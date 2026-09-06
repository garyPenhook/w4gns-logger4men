package main

import (
	"context"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// TestDeleteConfirmationTracksQSOIdentity reproduces the wrong-row-deleted
// bug: the "press d again" confirmation used to be a bare bool, so a table
// refresh between the two presses (e.g. from an async import) that shifted
// which QSO occupied the selected row caused the second "d" to delete
// whatever now sat there instead of the QSO the operator actually confirmed.
func TestDeleteConfirmationTracksQSOIdentity(t *testing.T) {
	m := reviewModel(t)
	w1aw := reviewQSO(m)
	w1aw.call = "W1AW"
	w1awID := reviewInsert(t, m, w1aw)
	k2abc := reviewQSO(m)
	k2abc.call = "K2ABC"
	k2abc.time = w1aw.time.Add(time.Second)
	k2abcID := reviewInsert(t, m, k2abc)

	m.refreshTableRows()
	m.setTableFocused(true)
	m.table.SetCursor(0) // most recent first: K2ABC
	if q, ok := m.selectedRecentQSO(); !ok || q.id != k2abcID {
		t.Fatalf("selected QSO = %+v, want K2ABC", q)
	}
	updated, _ := m.updateRecentQSOsTable(keyRune("d"))
	m = updated.(model)
	if m.deleteArmedID != k2abcID {
		t.Fatalf("deleteArmedID = %d, want %d (K2ABC)", m.deleteArmedID, k2abcID)
	}

	// An async refresh (e.g. an import completing) repopulates the table and
	// shifts the row order, but the cursor position stays the same, so row 0
	// now backs a different QSO than the one that was armed.
	older := reviewQSO(m)
	older.call = "N0CALL"
	older.time = w1aw.time.Add(-time.Hour)
	reviewInsert(t, m, older)
	m.refreshTableRows()
	m.table.SetCursor(0)
	if q, ok := m.selectedRecentQSO(); !ok || q.id == k2abcID {
		t.Fatalf("row 0 after refresh should no longer be K2ABC, got %+v", q)
	}

	// The confirmation must not have survived the refresh: this second "d"
	// arms whatever now occupies row 0, it does not delete K2ABC.
	updated, _ = m.updateRecentQSOsTable(keyRune("d"))
	m = updated.(model)
	if count, err := m.store.count(m.activeStation.ID); err != nil || count != 3 {
		t.Fatalf("count = %d, err = %v; want 3 (nothing deleted by the stale confirmation)", count, err)
	}
	if _, err := m.store.qsoByID(m.activeStation.ID, w1awID); err != nil {
		t.Fatalf("W1AW was deleted instead of the confirmed row: %v", err)
	}
}

// TestRestoreContestSelectionRollsOverStaleOccurrence reproduces the serial
// corruption bug: restoreContestSelection used to resume the serial for the
// literal persisted contest_id (a previous session's dated occurrence)
// without first rolling it forward to today's occurrence, so it could
// present a serial (e.g. 050, carried over from an old session with QSOs
// logged under it) that had nothing to do with the occurrence the operator
// was about to log into — and logCurrentQSO would then silently swap in a
// different value at save time.
func TestRestoreContestSelectionRollsOverStaleOccurrence(t *testing.T) {
	m := reviewModel(t)
	cwOpen := m.events[eventIndex(t, m.events, "CW-OPEN")]

	stale := "CW-OPEN@2020" // a long-past annual occurrence
	for i := 0; i < 49; i++ {
		q := reviewQSO(m)
		q.call = "OLD" + string(rune('A'+i%26))
		q.contestID = stale
		q.stx = formatSerial(i + 1)
		q.time = time.Date(2020, 9, 5, 12, i, 0, 0, time.UTC)
		reviewInsert(t, m, q)
	}
	if _, err := m.store.db.Exec(`INSERT INTO contest_selection(profile_id,contest_id,sent_exchange) VALUES(?,?,?)`, m.activeStation.ID, stale, "GARY"); err != nil {
		t.Fatal(err)
	}

	m.restoreContestSelection()

	got := m.contestFields[contestName].Value()
	if strings.Contains(got, "2020") {
		t.Fatalf("contestName after restore = %q, still carries the stale occurrence year", got)
	}
	want := contestOccurrenceID("CW-OPEN", cwOpen, time.Now().UTC())
	if got != want {
		t.Fatalf("contestName after restore = %q, want %q (today's occurrence)", got, want)
	}
	if m.nextSerial != 1 {
		t.Fatalf("nextSerial after restore = %d, want 1 (no QSOs logged under today's occurrence yet)", m.nextSerial)
	}
	if sent := m.contestFields[contestSerialSent].Value(); sent != formatSerial(1) {
		t.Fatalf("Sent Serial field after restore = %q, want %q", sent, formatSerial(1))
	}
}

// TestDXCCPortableLocationWinsOverHomeCallPrefix reproduces the asymmetric
// portable-call resolution bug: "F/W4GNS" (location prefix first) correctly
// resolved to France, but "W4GNS/F" (location suffix last) resolved to the
// United States instead, because the unsplit call and the home-call half
// were tried before the explicit location half and a tie never overrides an
// earlier match.
func TestDXCCPortableLocationWinsOverHomeCallPrefix(t *testing.T) {
	table, err := loadDXCCTable()
	if err != nil {
		t.Fatal(err)
	}
	prefixFirst, ok := table.lookup("F/W4GNS")
	if !ok {
		t.Fatal("lookup(\"F/W4GNS\") = not found")
	}
	suffixLast, ok := table.lookup("W4GNS/F")
	if !ok {
		t.Fatal("lookup(\"W4GNS/F\") = not found")
	}
	if prefixFirst.Country != "France" {
		t.Fatalf("lookup(\"F/W4GNS\").Country = %q, want France", prefixFirst.Country)
	}
	if suffixLast.Country != prefixFirst.Country {
		t.Fatalf("lookup(\"W4GNS/F\").Country = %q, want %q (same portable location as \"F/W4GNS\")", suffixLast.Country, prefixFirst.Country)
	}
}

// TestExportCabrilloRejectsExcludedBandOutsideQSOParty reproduces the
// band-validation gap: validateContestSubmission only enforced the event's
// catalog band list inside the QSO-party branch, so an imported contact on a
// band excluded from a regular contest (like CQ WPX CW's 30M exclusion)
// bypassed the interactive entry screen's band restriction and exported —
// and scored — successfully.
func TestExportCabrilloRejectsExcludedBandOutsideQSOParty(t *testing.T) {
	m := reviewModel(t)
	wpx := m.events[eventIndex(t, m.events, "CQ-WPX-CW")]
	if bandAllowed(wpx.Bands, "30M") {
		t.Fatal("test assumes 30M is excluded from CQ-WPX-CW's catalog bands")
	}

	q := reviewQSO(m)
	q.contestID = wpx.ID
	q.band = "30M"
	q.frequency = "10.125"
	q.stx, q.srx = "001", "002"
	reviewInsert(t, m, q)

	profile, err := m.store.activeStationProfile()
	if err != nil {
		t.Fatal(err)
	}
	var out strings.Builder
	count, _, err := exportCabrillo(context.Background(), &out, profile, wpx, wpx.ID, m.store)
	if err == nil {
		t.Fatalf("exportCabrillo accepted a 30M contact for CQ-WPX-CW: count=%d", count)
	}
}

// TestADIFContestIDMapsAmbiguousSessionOccurrence reproduces the mapping
// gap: adifContestID recognized "CWT" and "CWT-<session>" but not the
// unresolved-session occurrence form "CWT@<stamp>" that importedContestID
// leaves behind for an ambiguous import, so an ADIF export of such a QSO
// wrote the internal ID as CONTEST_ID instead of the catalog's ADIF ID.
func TestADIFContestIDMapsAmbiguousSessionOccurrence(t *testing.T) {
	got := adifContestID("CWT@20260902")
	if got != "CWOPS-CWT" {
		t.Fatalf("adifContestID(%q) = %q, want %q", "CWT@20260902", got, "CWOPS-CWT")
	}
}

// TestCancelEditPreservesManualSerialOverride reproduces the serial-override
// loss bug: with a serial-based contest active, an operator can type a manual
// Sent Serial (e.g. "010") that hasn't been logged yet, so the field diverges
// from the running nextSerial counter. Detouring to edit another QSO and then
// cancelling used to reformat the field from nextSerial, silently dropping the
// typed override; the restore must put back the exact pre-edit field text.
func TestCancelEditPreservesManualSerialOverride(t *testing.T) {
	m := reviewModel(t)
	cwOpen := m.events[eventIndex(t, m.events, "CW-OPEN")]
	m.selectEvent(cwOpen, cwOpen.Sessions[0])

	m.fields[fieldCall].SetValue("W1AW")
	m, _ = m.logCurrentQSO()
	if m.nextSerial != 2 {
		t.Fatalf("nextSerial after logging = %d, want 2", m.nextSerial)
	}

	// Operator types a manual serial override that diverges from the counter
	// and has not been logged yet.
	m.contestFields[contestSerialSent].SetValue("010")

	recent, err := m.store.recentQSOs(m.activeStation.ID, 1)
	if err != nil || len(recent) != 1 {
		t.Fatalf("recentQSOs: %v (%d rows)", err, len(recent))
	}
	m.beginEditQSO(qso{id: recent[0].id})
	m.cancelEditQSO()

	if got := m.contestFields[contestSerialSent].Value(); got != "010" {
		t.Fatalf("Sent Serial field after cancel = %q, want %q (manual override preserved)", got, "010")
	}
	if m.nextSerial != 2 {
		t.Fatalf("nextSerial after cancel = %d, want 2 (running counter restored)", m.nextSerial)
	}
}

// TestPackFieldRowsWrapsWithinBudget verifies the QSO Entry form no longer
// stretches every field onto one very wide line: packFieldRows must keep each
// row within the width budget by wrapping into multiple rows, while never
// dropping a box that is individually wider than the budget.
func TestPackFieldRowsWrapsWithinBudget(t *testing.T) {
	boxes := []string{
		lipgloss.NewStyle().Width(20).Render("a"),
		lipgloss.NewStyle().Width(20).Render("b"),
		lipgloss.NewStyle().Width(20).Render("c"),
		lipgloss.NewStyle().Width(20).Render("d"),
	}
	packed := packFieldRows(boxes, 45) // fits ~2 boxes per row
	if w := lipgloss.Width(packed); w > 45 {
		t.Fatalf("packed block width = %d, want <= 45 (rows must wrap within budget)", w)
	}
	if h := lipgloss.Height(packed); h < 2 {
		t.Fatalf("packed block height = %d, want >= 2 rows (fields did not wrap)", h)
	}

	// A single box wider than the budget still gets its own row, not dropped.
	wide := lipgloss.NewStyle().Width(80).Render("x")
	packedWide := packFieldRows([]string{wide}, 45)
	if !strings.Contains(packedWide, "x") {
		t.Fatal("packFieldRows dropped a box wider than the budget")
	}
}

// TestQSOEntryFieldsWrapInsteadOfOneWideLine drives the full View and asserts
// the entry field boxes wrap rather than rendering as one line far wider than
// the terminal (previously ~246 columns for a serial contest in POST mode).
func TestQSOEntryFieldsWrapInsteadOfOneWideLine(t *testing.T) {
	m := reviewModel(t)
	cwOpen := m.events[eventIndex(t, m.events, "CW-OPEN")]
	m.selectEvent(cwOpen, cwOpen.Sessions[0])
	m.screen = qsoEntryScreen
	m.postMode = true
	m.fields[fieldCall].SetValue("W1AW")

	slots := m.entrySlots()
	views := make([]string, len(slots))
	for i, s := range slots {
		views[i] = m.renderSlot(i, s)
	}
	oneLine := lipgloss.Width(lipgloss.JoinHorizontal(lipgloss.Top, views...))

	m.termWidth = 120
	m.termHeight = 40
	if view := m.View(); view == "" {
		t.Fatal("qsoEntryView returned empty output")
	}
	packed := packFieldRows(views, m.termWidth)
	if w := lipgloss.Width(packed); w >= oneLine {
		t.Fatalf("packed field block width = %d, want narrower than the %d-column single line", w, oneLine)
	}
	if lipgloss.Height(packed) <= lipgloss.Height(views[0]) {
		t.Fatalf("field block did not wrap into multiple rows (height %d)", lipgloss.Height(packed))
	}
}

func keyRune(s string) tea.KeyMsg {
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}
}
