package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

// TestEventForContestIDPrefersLongestMatchingEventID guards against a real
// collision in the live event catalog: "UBA-SPRING-CONTEST" is itself a
// prefix-match of "UBA-SPRING-CONTEST-2" (and there are several other such
// pairs — AGCW-QRP/AGCW-QRP-CONTEST, K1USNSST/K1USNSST-OPEN,
// SKCC-SPRINT/SKCC-SPRINT-EUROPE). Selecting the longer event must resolve
// back to itself, not the shorter ancestor, or the wrong bands/dupe_scope
// get used.
func TestEventForContestIDPrefersLongestMatchingEventID(t *testing.T) {
	st, err := openStore(filepath.Join(t.TempDir(), "logger.db"))
	if err != nil {
		t.Fatalf("openStore returned error: %v", err)
	}
	defer st.Close()

	m := initialModel(st)
	longer := m.events[eventIndex(t, m.events, "UBA-SPRING-CONTEST-2")]
	m.selectEvent(longer, longer.Sessions[0])

	resolved, ok := m.eventForContestID()
	if !ok {
		t.Fatal("eventForContestID did not resolve a match")
	}
	if resolved.ID != "UBA-SPRING-CONTEST-2" {
		t.Fatalf("eventForContestID resolved %q, want %q (the exact event selected, not the shorter prefix ancestor)", resolved.ID, "UBA-SPRING-CONTEST-2")
	}
}

// TestADIFImportResultSurfacesAfterLeavingImportScreen guards against
// losing the async import's result: pressing Esc to leave the Import ADIF
// screen while the import is still running must not prevent its eventual
// adifImportedMsg from updating the status bar and Recent QSOs table once
// it arrives, even though the screen has already changed.
func TestADIFImportResultSurfacesAfterLeavingImportScreen(t *testing.T) {
	st, err := openStore(filepath.Join(t.TempDir(), "logger.db"))
	if err != nil {
		t.Fatalf("openStore returned error: %v", err)
	}
	defer st.Close()

	m := initialModel(st)
	m.screen = adifImportScreen
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = updated.(model)
	if m.screen != qsoEntryScreen {
		t.Fatalf("screen after Esc = %v, want qsoEntryScreen", m.screen)
	}

	updated, _ = m.Update(adifImportedMsg{result: adifImportResult{Imported: 3, Skipped: 1}})
	m = updated.(model)
	if !strings.Contains(m.statusMsg, "3 CW QSOs") {
		t.Fatalf("statusMsg = %q, want it to report the import result even after leaving the import screen", m.statusMsg)
	}
}

// TestQSOEntryHeaderUsesStationProfileTimezone guards against the header's
// "Local" time always reflecting the host machine's timezone instead of
// the configured station profile's — an operator running on a
// differently-configured host still wants their station's own local time.
func TestQSOEntryHeaderUsesStationProfileTimezone(t *testing.T) {
	st, err := openStore(filepath.Join(t.TempDir(), "logger.db"))
	if err != nil {
		t.Fatalf("openStore returned error: %v", err)
	}
	defer st.Close()

	m := initialModel(st)
	m.activeStation.Timezone = "Pacific/Kiritimati" // UTC+14, unlikely to be the host's zone
	view := m.View()
	if !strings.Contains(view, "Pacific/Kiritimati") {
		t.Fatalf("View() header does not mention the configured station timezone Pacific/Kiritimati:\n%s", view)
	}
}

// TestWindowSizeMsgIsRememberedOnModel guards the mechanism saveWindowSize
// relies on at shutdown: the terminal size from the most recent
// tea.WindowSizeMsg must land on the model so it can be persisted.
func TestWindowSizeMsgIsRememberedOnModel(t *testing.T) {
	st, err := openStore(filepath.Join(t.TempDir(), "logger.db"))
	if err != nil {
		t.Fatalf("openStore returned error: %v", err)
	}
	defer st.Close()

	m := initialModel(st)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 132, Height: 43})
	m = updated.(model)
	if m.termWidth != 132 || m.termHeight != 43 {
		t.Fatalf("termWidth/termHeight = %d/%d, want 132/43", m.termWidth, m.termHeight)
	}
}

func TestEnterLeavingCallStartsQSOTimer(t *testing.T) {
	st, err := openStore(filepath.Join(t.TempDir(), "logger.db"))
	if err != nil {
		t.Fatalf("openStore returned error: %v", err)
	}
	defer st.Close()

	m := initialModel(st)
	m.fields[fieldCall].SetValue("W1AW")
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	got := updated.(model)
	if got.qsoStartedAt.IsZero() {
		t.Fatal("Enter leaving Call did not start QSO timer")
	}
}

// TestEnterAfterCallFastPathsPastAutoFilledFieldsDuringContest guards the
// ergonomic entry order: with a contest active, Enter leaving Call should
// jump straight to the received exchange, skipping RST/Band/Freq (which are
// auto-filled and rarely need touching mid-QSO). Tab must still visit every
// field for the rare correction.
func TestEnterAfterCallFastPathsPastAutoFilledFieldsDuringContest(t *testing.T) {
	st, err := openStore(filepath.Join(t.TempDir(), "logger.db"))
	if err != nil {
		t.Fatalf("openStore returned error: %v", err)
	}
	defer st.Close()

	m := initialModel(st)
	cwopen := m.events[eventIndex(t, m.events, "CW-OPEN")]
	m.selectEvent(cwopen, cwopen.Sessions[0])
	m.screen = qsoEntryScreen
	m.focusField(fieldCall)

	m.fields[fieldCall].SetValue("W1AW")
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	got := updated.(model)

	slots := got.entrySlots()
	if want := fieldCount; got.focusIdx != want || !slots[got.focusIdx].contest {
		t.Fatalf("Enter after Call with a contest active focused slot %d (%+v), want the first received-exchange slot at %d", got.focusIdx, slots[got.focusIdx], want)
	}

	// Tab still moves one field at a time, unaffected by the fast path.
	got.focusField(fieldCall)
	updated, _ = got.Update(tea.KeyMsg{Type: tea.KeyTab})
	got = updated.(model)
	if got.focusIdx != fieldRSTSent {
		t.Fatalf("Tab after Call focused slot %d, want %d (RST Sent)", got.focusIdx, fieldRSTSent)
	}
}

// TestEnterAfterCallAdvancesOneFieldOutsideContest guards against the fast
// path firing when no contest is active — there is no received exchange to
// jump to, so Enter must keep its normal one-field-at-a-time advance.
func TestEnterAfterCallAdvancesOneFieldOutsideContest(t *testing.T) {
	st, err := openStore(filepath.Join(t.TempDir(), "logger.db"))
	if err != nil {
		t.Fatalf("openStore returned error: %v", err)
	}
	defer st.Close()

	m := initialModel(st)
	m.fields[fieldCall].SetValue("W1AW")
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	got := updated.(model)
	if got.focusIdx != fieldRSTSent {
		t.Fatalf("Enter after Call outside a contest focused slot %d, want %d (RST Sent)", got.focusIdx, fieldRSTSent)
	}
}

// TestF8IgnoresRepeatedPressesWhileBackupInProgress guards against launching
// concurrent backups (second-resolution filename collisions, uncoordinated
// with the mandatory exit backup): a second F8 while one is already running
// must not start another.
func TestF8IgnoresRepeatedPressesWhileBackupInProgress(t *testing.T) {
	st, err := openStore(filepath.Join(t.TempDir(), "logger.db"))
	if err != nil {
		t.Fatalf("openStore returned error: %v", err)
	}
	defer st.Close()

	m := initialModel(st)
	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyF8})
	m = updated.(model)
	if !m.backupInProgress {
		t.Fatal("first F8 press did not mark a backup in progress")
	}
	if cmd == nil {
		t.Fatal("first F8 press did not return a backup command")
	}

	updated, cmd = m.Update(tea.KeyMsg{Type: tea.KeyF8})
	m = updated.(model)
	if cmd != nil {
		t.Fatal("second F8 press while a backup is in progress returned a command")
	}
	if m.statusMsg != "backup already in progress…" {
		t.Fatalf("statusMsg = %q, want the already-in-progress message", m.statusMsg)
	}

	updated, _ = m.Update(backupCompletedMsg{})
	m = updated.(model)
	if m.backupInProgress {
		t.Fatal("backupInProgress stayed true after backupCompletedMsg")
	}
}

// TestCheckDupeWarnsOnceForUnrecognizedContestName covers the case where an
// operator free-types a contest name that isn't in the event catalog:
// dupeScope silently falls back to the casual 15-minute window, so checkDupe
// must surface that instead of failing silently — but only once per distinct
// value, not on every keystroke.
func TestCheckDupeWarnsOnceForUnrecognizedContestName(t *testing.T) {
	st, err := openStore(filepath.Join(t.TempDir(), "logger.db"))
	if err != nil {
		t.Fatalf("openStore returned error: %v", err)
	}
	defer st.Close()

	m := initialModel(st)
	m.contestFields[contestName].SetValue("Some Local Sprint")
	m.fields[fieldCall].SetValue("W1AW")
	m.checkDupe()
	if !strings.Contains(m.statusMsg, "Some Local Sprint") {
		t.Fatalf("statusMsg = %q, want it to mention the unrecognized contest name", m.statusMsg)
	}

	m.statusMsg = ""
	m.checkDupe()
	if m.statusMsg != "" {
		t.Fatalf("checkDupe re-warned for the same contest name: statusMsg = %q", m.statusMsg)
	}
}

func TestEventDetailLineSurfacesPreviouslyUnusedFields(t *testing.T) {
	event := eventDefinition{
		Capability:         catalogCapabilityScoringReady,
		Organizer:          "CWops",
		Bands:              []string{"20M", "15M"},
		RulesURL:           "https://example.com/rules",
		ScoreSubmissionURL: "https://example.com/scores",
	}
	line := eventDetailLine(event)
	for _, want := range []string{"scoring ready", "CWops", "20M/15M", "https://example.com/rules", "https://example.com/scores"} {
		if !strings.Contains(line, want) {
			t.Errorf("eventDetailLine() = %q, want it to contain %q", line, want)
		}
	}
}

// TestLogCurrentQSORejectsBandOutsideEventAllowedBands guards against
// committing a QSO on a band outside the selected event's allowed bands: the
// check must block the insert (there's no in-app way to fix a bad record
// afterward), not just append a warning once it's already in the database.
func TestLogCurrentQSORejectsBandOutsideEventAllowedBands(t *testing.T) {
	st, err := openStore(filepath.Join(t.TempDir(), "logger.db"))
	if err != nil {
		t.Fatalf("openStore returned error: %v", err)
	}
	defer st.Close()

	m := initialModel(st)
	tnqp := m.events[eventIndex(t, m.events, "TNQP")]
	m.selectEvent(tnqp, tnqp.Sessions[0])
	m.fields[fieldCall].SetValue("W1AW")
	m.fields[fieldBand].SetValue("60M")
	m.fields[fieldFrequency].SetValue("5.354")
	m, _ = m.logCurrentQSO()
	if !strings.Contains(m.statusMsg, "not in") {
		t.Fatalf("statusMsg = %q, want an allowed-bands warning", m.statusMsg)
	}
	if count, err := st.count(m.activeStation.ID); err != nil || count != 0 {
		t.Fatalf("count = %d, err = %v, want 0 — the QSO must not be committed", count, err)
	}
	if m.fields[fieldCall].Value() != "W1AW" {
		t.Fatal("Call field was cleared even though the QSO was rejected")
	}
}

// TestLogCurrentQSORejectsContestDupeEvenIfCachedWarningIsStale guards
// against relying solely on the cached dupeWarning indicator: it is only
// recomputed by checkDupe, which runs on the call/band/contest-name Update
// paths, not on every possible way a field can change (e.g. SetValue calls
// from tests, or future code paths). logCurrentQSO must re-verify against
// the database immediately before insert regardless of what dupeWarning
// currently holds.
func TestLogCurrentQSORejectsContestDupeEvenIfCachedWarningIsStale(t *testing.T) {
	st, err := openStore(filepath.Join(t.TempDir(), "logger.db"))
	if err != nil {
		t.Fatalf("openStore returned error: %v", err)
	}
	defer st.Close()

	m := initialModel(st)
	arrl := m.events[eventIndex(t, m.events, "ARRL-DX-CW")]
	m.selectEvent(arrl, arrl.Sessions[0])
	contestID := m.contestFields[contestName].Value()

	prior := validTestQSO()
	prior.call, prior.band, prior.contestID, prior.profileID = "W4GNS", "20M", contestID, m.activeStation.ID
	if _, err := st.insertQSO(prior); err != nil {
		t.Fatalf("insert prior QSO: %v", err)
	}

	// Set the callsign/band directly (bypassing Update, which is what
	// recalculates dupeWarning) so the cached indicator is left stale.
	m.fields[fieldCall].SetValue("W4GNS")
	m.fields[fieldBand].SetValue("20M")
	if m.dupeWarning {
		t.Fatal("test setup invalid: dupeWarning should still be stale (false) at this point")
	}

	m, _ = m.logCurrentQSO()
	if !strings.Contains(m.statusMsg, "DUPE") {
		t.Fatalf("statusMsg = %q, want a DUPE rejection despite the stale cached indicator", m.statusMsg)
	}
	if count, err := st.count(m.activeStation.ID); err != nil || count != 1 {
		t.Fatalf("count = %d, err = %v, want 1 (only the pre-existing QSO)", count, err)
	}
}

// TestF9TogglesTableFocusAndCursorSurvivesEmptyToNonEmptyTransition guards a
// real regression: bubbles/table's SetRows leaves the cursor at -1 the
// first time it's called with zero rows (which happens once at startup,
// before any QSO exists), and never recovers on its own once real rows
// exist — so without refreshTableRows correcting it, F9 + Enter could never
// select anything.
func TestF9TogglesTableFocusAndCursorSurvivesEmptyToNonEmptyTransition(t *testing.T) {
	st, err := openStore(filepath.Join(t.TempDir(), "logger.db"))
	if err != nil {
		t.Fatalf("openStore returned error: %v", err)
	}
	defer st.Close()

	m := initialModel(st)
	m.refreshTableRows() // mirrors the startup call with zero QSOs logged yet
	if m.table.Cursor() != -1 && m.table.Cursor() != 0 {
		t.Fatalf("cursor on an empty table = %d, want -1 or 0", m.table.Cursor())
	}

	m.fields[fieldCall].SetValue("W1AW")
	m, _ = m.logCurrentQSO()
	if m.table.Cursor() < 0 || m.table.Cursor() >= len(m.recentQSOs) {
		t.Fatalf("cursor after logging the first QSO = %d, want a valid row index into %d rows", m.table.Cursor(), len(m.recentQSOs))
	}

	if m.tableFocused {
		t.Fatal("table should not start focused")
	}
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyF9})
	m = updated.(model)
	if !m.tableFocused || !m.table.Focused() {
		t.Fatal("F9 did not focus the Recent QSOs table")
	}
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyF9})
	m = updated.(model)
	if m.tableFocused || m.table.Focused() {
		t.Fatal("second F9 did not unfocus the Recent QSOs table")
	}
}

// TestEditQSOFlowSavesChangesWithoutInsertingANewRow drives the full
// F9 -> Enter -> edit -> save cycle through Update, the way a real
// keystroke sequence would, and confirms the existing row is updated in
// place (not duplicated) with its original timestamp preserved.
// TestSerialSentAutoIncrementsAcrossContestQSOs guards the running serial: a
// serial-exchange contest (CW Open) must show 001 on selection, advance to the
// next number after each logged QSO, keep the operator "in the event" (contest
// name and their own sent exchange survive the between-QSO reset), and carry a
// manual correction to the Sent Serial field forward.
func TestSerialSentAutoIncrementsAcrossContestQSOs(t *testing.T) {
	st, err := openStore(filepath.Join(t.TempDir(), "logger.db"))
	if err != nil {
		t.Fatalf("openStore returned error: %v", err)
	}
	defer st.Close()

	m := initialModel(st)
	cwopen := m.events[eventIndex(t, m.events, "CW-OPEN")]
	m.selectEvent(cwopen, cwopen.Sessions[0])
	contestID := m.contestFields[contestName].Value()

	if got := m.contestFields[contestSerialSent].Value(); got != "001" {
		t.Fatalf("initial Sent Serial = %q, want 001", got)
	}

	m.contestFields[contestExchangeSent].SetValue("Gary") // the operator's own name, constant all session
	m.fields[fieldCall].SetValue("W1AW")
	m.fields[fieldBand].SetValue("20M")
	m.fields[fieldFrequency].SetValue("14.025")
	m.contestFields[contestSerialRcvd].SetValue("045")
	m, _ = m.logCurrentQSO()
	if !strings.Contains(m.statusMsg, "logged") {
		t.Fatalf("first QSO not logged: %q", m.statusMsg)
	}
	if got := m.contestFields[contestSerialSent].Value(); got != "002" {
		t.Fatalf("Sent Serial after 1st QSO = %q, want 002", got)
	}
	if got := m.contestFields[contestName].Value(); got != contestID {
		t.Fatalf("contest name cleared between QSOs: %q, want %q", got, contestID)
	}
	if got := m.contestFields[contestExchangeSent].Value(); got != "Gary" {
		t.Fatalf("sent exchange (name) cleared between QSOs: %q, want Gary", got)
	}
	if got := m.contestFields[contestSerialRcvd].Value(); got != "" {
		t.Fatalf("received serial not cleared between QSOs: %q", got)
	}

	m.fields[fieldCall].SetValue("K1ABC")
	m, _ = m.logCurrentQSO()
	if got := m.contestFields[contestSerialSent].Value(); got != "003" {
		t.Fatalf("Sent Serial after 2nd QSO = %q, want 003", got)
	}

	// A manual correction to the Sent Serial field must carry forward: the next
	// serial follows the number actually sent, not an internal blind counter.
	m.contestFields[contestSerialSent].SetValue("010")
	m.fields[fieldCall].SetValue("K2XYZ")
	m, _ = m.logCurrentQSO()
	if got := m.contestFields[contestSerialSent].Value(); got != "011" {
		t.Fatalf("Sent Serial after manual correction to 010 = %q, want 011", got)
	}

	// The serials actually stored on the QSOs must match what was displayed.
	wantSerials := map[string]string{"W1AW": "001", "K1ABC": "002", "K2XYZ": "010"}
	err = st.forEachQSOForContest(context.Background(), m.activeStation.ID, contestID, func(q qso) error {
		if want, ok := wantSerials[q.call]; ok && q.stx != want {
			t.Errorf("stored serial for %s = %q, want %q", q.call, q.stx, want)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("forEachQSOForContest: %v", err)
	}
}

// TestQSOEntryHeaderShowsSendingSerial guards the mirror of the running serial
// onto the main QSO Entry screen: a serial contest must surface the number the
// operator will send next there (not only on Contest Entry/F7), and it must
// track the advance after a QSO is logged.
func TestQSOEntryHeaderShowsSendingSerial(t *testing.T) {
	st, err := openStore(filepath.Join(t.TempDir(), "logger.db"))
	if err != nil {
		t.Fatalf("openStore returned error: %v", err)
	}
	defer st.Close()

	m := initialModel(st)
	cwopen := m.events[eventIndex(t, m.events, "CW-OPEN")]
	m.selectEvent(cwopen, cwopen.Sessions[0])
	m.screen = qsoEntryScreen // selectEvent opens Contest Entry; go look at QSO Entry

	if view := m.View(); !strings.Contains(view, "Sending # 001") {
		t.Fatalf("QSO Entry view missing 'Sending # 001', got:\n%s", view)
	}

	m.fields[fieldCall].SetValue("W1AW")
	m.fields[fieldBand].SetValue("20M")
	m.fields[fieldFrequency].SetValue("14.025")
	m, _ = m.logCurrentQSO()
	if view := m.View(); !strings.Contains(view, "Sending # 002") {
		t.Fatalf("QSO Entry view after 1st QSO missing 'Sending # 002', got:\n%s", view)
	}
}

// TestQSOEntryHeaderOmitsSendingSerialOutsideSerialContest guards the gate: a
// non-serial context must not show a "Sending #" label.
func TestQSOEntryHeaderOmitsSendingSerialOutsideSerialContest(t *testing.T) {
	st, err := openStore(filepath.Join(t.TempDir(), "logger.db"))
	if err != nil {
		t.Fatalf("openStore returned error: %v", err)
	}
	defer st.Close()

	m := initialModel(st)
	if view := m.View(); strings.Contains(view, "Sending #") {
		t.Fatalf("QSO Entry view showed a serial with no serial contest active:\n%s", view)
	}
}

// TestContestReceivedExchangeLoggedInlineOnEntryScreen guards the core contest
// workflow: while a contest is active, the worked station's received exchange
// (serial + name for CW Open) is captured on the main QSO Entry screen and
// logged in one place — no switching to Contest Entry (F7). It verifies the
// inline fields appear, keystrokes route to them, and the values reach the QSO.
func TestContestReceivedExchangeLoggedInlineOnEntryScreen(t *testing.T) {
	st, err := openStore(filepath.Join(t.TempDir(), "logger.db"))
	if err != nil {
		t.Fatalf("openStore returned error: %v", err)
	}
	defer st.Close()

	m := initialModel(st)
	cwopen := m.events[eventIndex(t, m.events, "CW-OPEN")]
	m.selectEvent(cwopen, cwopen.Sessions[0])
	contestID := m.contestFields[contestName].Value()
	m.screen = qsoEntryScreen
	m.focusField(fieldCall)

	// The entry row now carries the received exchange inline (Rcv # + Rcv Exch),
	// and the last field is the exchange so a final Enter logs from it.
	slots := m.entrySlots()
	if len(slots) != fieldCount+2 {
		t.Fatalf("entrySlots len = %d, want %d (base fields + Rcv#/RcvExch)", len(slots), fieldCount+2)
	}
	if !slots[len(slots)-1].contest {
		t.Fatalf("last entry slot should be the received exchange, got %+v", slots[len(slots)-1])
	}
	if view := m.View(); !strings.Contains(view, "Rcv #") || !strings.Contains(view, "Rcv Exch") {
		t.Fatalf("QSO Entry view missing inline received-exchange fields:\n%s", view)
	}

	// Focus the received-exchange field and type: keystrokes must route to the
	// contest field, not a base field.
	m.focusField(len(slots) - 1)
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("Joe")})
	m = updated.(model)
	if got := m.contestFields[contestExchangeRcvd].Value(); got != "Joe" {
		t.Fatalf("typing into the inline exchange field set %q, want Joe", got)
	}

	// Fill call + received serial and log — without ever opening F7.
	m.fields[fieldCall].SetValue("W1AW")
	m.fields[fieldBand].SetValue("20M")
	m.fields[fieldFrequency].SetValue("14.025")
	m.contestFields[contestSerialRcvd].SetValue("042")
	m, _ = m.logCurrentQSO()
	if !strings.Contains(m.statusMsg, "logged") {
		t.Fatalf("QSO not logged: %q", m.statusMsg)
	}

	var got qso
	found := false
	if err := st.forEachQSOForContest(context.Background(), m.activeStation.ID, contestID, func(q qso) error {
		got, found = q, true
		return nil
	}); err != nil {
		t.Fatalf("forEachQSOForContest: %v", err)
	}
	if !found {
		t.Fatal("logged QSO not found for contest")
	}
	if got.call != "W1AW" || got.srx != "042" || got.srxString != "Joe" {
		t.Fatalf("stored QSO = %s rcv %q/%q, want W1AW 042/Joe", got.call, got.srx, got.srxString)
	}
}

// TestAutofillReceivedExchangeZonePrefillsAndSharpens guards roadmap Appendix
// B.8 "auto data insert (zones)": for a contest whose exchange the catalog
// hint identifies as a CQ zone (CQ-WW-CW), typing a callsign should prefill
// the received-exchange field with that entity's CQ zone from the DXCC
// table, and the guess should update (sharpen) as more of the call is typed.
func TestAutofillReceivedExchangeZonePrefillsAndSharpens(t *testing.T) {
	st, err := openStore(filepath.Join(t.TempDir(), "logger.db"))
	if err != nil {
		t.Fatalf("openStore returned error: %v", err)
	}
	defer st.Close()

	m := initialModel(st)
	cqww := m.events[eventIndex(t, m.events, "CQ-WW-CW")]
	m.selectEvent(cqww, cqww.Sessions[0])
	m.screen = qsoEntryScreen
	m.focusField(fieldCall)

	// 1A0KM is the Sov Mil Order of Malta entity, CQ zone 15 (see dxcc_test.go).
	m.fields[fieldCall].SetValue("1A0KM")
	m.checkDupe()
	if got := m.contestFields[contestExchangeRcvd].Value(); got != "15" {
		t.Fatalf("autofill for 1A0KM = %q, want CQ zone 15", got)
	}

	// Backspacing the call back to blank must clear the guess too, since it
	// hasn't been overridden by the operator and no longer resolves to
	// anything.
	m.fields[fieldCall].SetValue("")
	m.checkDupe()
	if got := m.contestFields[contestExchangeRcvd].Value(); got != "" {
		t.Fatalf("autofill after clearing the call = %q, want blank", got)
	}
}

// TestAutofillReceivedExchangeZoneDoesNotClobberManualEdit guards the
// override half of Appendix B.8: once the operator types their own value
// into the received-exchange field, the autofill must stop touching it even
// as the callsign keeps changing — the operator's typed value always wins.
func TestAutofillReceivedExchangeZoneDoesNotClobberManualEdit(t *testing.T) {
	st, err := openStore(filepath.Join(t.TempDir(), "logger.db"))
	if err != nil {
		t.Fatalf("openStore returned error: %v", err)
	}
	defer st.Close()

	m := initialModel(st)
	cqww := m.events[eventIndex(t, m.events, "CQ-WW-CW")]
	m.selectEvent(cqww, cqww.Sessions[0])
	m.screen = qsoEntryScreen
	m.focusField(fieldCall)

	m.fields[fieldCall].SetValue("1A0KM")
	m.checkDupe()
	if got := m.contestFields[contestExchangeRcvd].Value(); got != "15" {
		t.Fatalf("autofill for 1A0KM = %q, want CQ zone 15", got)
	}

	slots := m.entrySlots()
	m.focusField(len(slots) - 1) // the received-exchange slot
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("9")})
	m = updated.(model)
	if got := m.contestFields[contestExchangeRcvd].Value(); got != "159" {
		t.Fatalf("manual keystroke into exchange field = %q, want 159", got)
	}

	m.focusField(fieldCall)
	m.fields[fieldCall].SetValue("W1AW") // a different entity, CQ zone 5
	m.checkDupe()
	if got := m.contestFields[contestExchangeRcvd].Value(); got != "159" {
		t.Fatalf("autofill clobbered a manual edit: got %q, want 159 unchanged", got)
	}

	// clearQSOForm (the between-QSO reset) must clear the override too, so
	// the next QSO starts fresh.
	m.clearQSOForm()
	if m.contestExchangeRcvdEdited {
		t.Fatal("clearQSOForm did not reset the manual-edit override flag")
	}
}

// TestAutofillReceivedExchangeZoneIgnoresCursorMovement guards a subtler
// case: moving the cursor within the received-exchange field (left/right —
// textinput handles these but Value() doesn't change) must not be mistaken
// for the operator overriding the autofilled value, or a single stray arrow
// key would permanently disable autofill for the rest of the QSO.
func TestAutofillReceivedExchangeZoneIgnoresCursorMovement(t *testing.T) {
	st, err := openStore(filepath.Join(t.TempDir(), "logger.db"))
	if err != nil {
		t.Fatalf("openStore returned error: %v", err)
	}
	defer st.Close()

	m := initialModel(st)
	cqww := m.events[eventIndex(t, m.events, "CQ-WW-CW")]
	m.selectEvent(cqww, cqww.Sessions[0])
	m.screen = qsoEntryScreen
	m.focusField(fieldCall)

	m.fields[fieldCall].SetValue("1A0KM")
	m.checkDupe()
	if got := m.contestFields[contestExchangeRcvd].Value(); got != "15" {
		t.Fatalf("autofill for 1A0KM = %q, want CQ zone 15", got)
	}

	slots := m.entrySlots()
	m.focusField(len(slots) - 1) // the received-exchange slot
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyLeft})
	m = updated.(model)
	if m.contestExchangeRcvdEdited {
		t.Fatal("a cursor-movement key set the manual-edit override flag")
	}

	// Autofill must still track a new callsign after the cursor move.
	m.focusField(fieldCall)
	m.fields[fieldCall].SetValue("W1AW")
	m.checkDupe()
	if got := m.contestFields[contestExchangeRcvd].Value(); got == "15" {
		t.Fatal("autofill did not update for the new callsign after a cursor-movement key")
	}
}

// TestAutofillReceivedExchangeZoneExcludesDomesticEntities guards the
// side-aware exchange schema (received_exchange_autofill_domestic): CQ 160 CW
// has DX stations send a CQ zone but W/VE stations send a state/province that
// cty.dat can't derive, so autofill must fire for a DX call and stay blank
// for a worked United States/Canada call rather than guess a zone the
// station never actually sends.
func TestAutofillReceivedExchangeZoneExcludesDomesticEntities(t *testing.T) {
	st, err := openStore(filepath.Join(t.TempDir(), "logger.db"))
	if err != nil {
		t.Fatalf("openStore returned error: %v", err)
	}
	defer st.Close()

	m := initialModel(st)
	cq160 := m.events[eventIndex(t, m.events, "CQ-160-CW")]
	m.selectEvent(cq160, cq160.Sessions[0])
	m.screen = qsoEntryScreen
	m.focusField(fieldCall)

	m.fields[fieldCall].SetValue("1A0KM")
	m.checkDupe()
	if got := m.contestFields[contestExchangeRcvd].Value(); got != "15" {
		t.Fatalf("autofill for DX call 1A0KM = %q, want CQ zone 15", got)
	}

	m.fields[fieldCall].SetValue("W1AW")
	m.checkDupe()
	if got := m.contestFields[contestExchangeRcvd].Value(); got != "" {
		t.Fatalf("autofill for domestic call W1AW = %q, want blank (state/province isn't derivable)", got)
	}
}

func TestEditQSOFlowSavesChangesWithoutInsertingANewRow(t *testing.T) {
	st, err := openStore(filepath.Join(t.TempDir(), "logger.db"))
	if err != nil {
		t.Fatalf("openStore returned error: %v", err)
	}
	defer st.Close()

	m := initialModel(st)
	m.fields[fieldCall].SetValue("W1AW")
	m, _ = m.logCurrentQSO()
	original, err := st.qsoByID(m.activeStation.ID, m.recentQSOs[0].id)
	if err != nil {
		t.Fatal(err)
	}

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyF9})
	m = updated.(model)
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(model)
	if m.editingQSOID != original.id {
		t.Fatalf("editingQSOID = %d, want %d", m.editingQSOID, original.id)
	}
	if m.tableFocused {
		t.Fatal("entering edit mode should release table focus")
	}
	if m.fields[fieldCall].Value() != "W1AW" {
		t.Fatalf("Call field = %q after beginEditQSO, want W1AW", m.fields[fieldCall].Value())
	}

	m.fields[fieldRSTSent].SetValue("579")
	m, _ = m.logCurrentQSO()
	if m.editingQSOID != 0 {
		t.Fatalf("editingQSOID = %d after save, want 0", m.editingQSOID)
	}
	if !strings.Contains(m.statusMsg, "updated") {
		t.Fatalf("statusMsg = %q, want an updated confirmation", m.statusMsg)
	}

	got, err := st.qsoByID(m.activeStation.ID, original.id)
	if err != nil {
		t.Fatal(err)
	}
	if got.rstSent != "579" {
		t.Errorf("rstSent = %q, want 579", got.rstSent)
	}
	if !got.time.Equal(original.time) {
		t.Errorf("time = %v, want unchanged %v", got.time, original.time)
	}
	if count, err := st.count(m.activeStation.ID); err != nil || count != 1 {
		t.Fatalf("count = %d, err = %v, want 1 (edit must not insert a new row)", count, err)
	}
}

// TestEditQSOFlowSavesCountyEmailAndParkName guards against a real
// regression: the edit-merge in logCurrentQSO once carried forward name/qth/
// grid/state/potaRef/comment but silently dropped any County or Email edit
// (added in a later change) because they were never added to that same
// merge list. Park Name is new alongside them, so this covers all three
// together rather than risking the same omission again.
func TestEditQSOFlowSavesCountyEmailAndParkName(t *testing.T) {
	st, err := openStore(filepath.Join(t.TempDir(), "logger.db"))
	if err != nil {
		t.Fatalf("openStore returned error: %v", err)
	}
	defer st.Close()

	m := initialModel(st)
	m.fields[fieldCall].SetValue("W1AW")
	m, _ = m.logCurrentQSO()
	original, err := st.qsoByID(m.activeStation.ID, m.recentQSOs[0].id)
	if err != nil {
		t.Fatal(err)
	}

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyF9})
	m = updated.(model)
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(model)
	if m.editingQSOID != original.id {
		t.Fatalf("editingQSOID = %d, want %d", m.editingQSOID, original.id)
	}

	m.detailFields[detailCounty].SetValue("Davidson")
	m.detailFields[detailEmail].SetValue("w1aw@arrl.org")
	m.detailFields[detailParkName].SetValue("Fake State Park")
	m, _ = m.logCurrentQSO()
	if m.editingQSOID != 0 {
		t.Fatalf("editingQSOID = %d after save, want 0", m.editingQSOID)
	}

	got, err := st.qsoByID(m.activeStation.ID, original.id)
	if err != nil {
		t.Fatal(err)
	}
	if got.county != "Davidson" {
		t.Errorf("county = %q, want Davidson", got.county)
	}
	if got.email != "w1aw@arrl.org" {
		t.Errorf("email = %q, want w1aw@arrl.org", got.email)
	}
	if got.parkName != "Fake State Park" {
		t.Errorf("parkName = %q, want Fake State Park", got.parkName)
	}
}

// TestEditQSOFromDifferentContestRestoresActiveContestOnSave guards against a
// real regression: beginEditQSO loads the edited row's own contest fields
// (contestID/stx/stxString) into m.contestFields so they're editable, but a
// QSO logged before the operator selected today's contest has no contestID —
// editing it must not leave the active contest session blanked out for every
// QSO logged afterward, mis-tagging them for dupe checks and Cabrillo export.
func TestEditQSOFromDifferentContestRestoresActiveContestOnSave(t *testing.T) {
	st, err := openStore(filepath.Join(t.TempDir(), "logger.db"))
	if err != nil {
		t.Fatalf("openStore returned error: %v", err)
	}
	defer st.Close()

	m := initialModel(st)
	// Log a QSO with no contest active.
	m.fields[fieldCall].SetValue("W1AW")
	m, _ = m.logCurrentQSO()
	original, err := st.qsoByID(m.activeStation.ID, m.recentQSOs[0].id)
	if err != nil {
		t.Fatal(err)
	}

	// Now select a contest — this is the active session for every QSO logged
	// from here on.
	cwt := m.events[eventIndex(t, m.events, "CWT")]
	m.selectEvent(cwt, cwt.Sessions[0])
	m.screen = qsoEntryScreen
	activeContest := m.contestFields[contestName].Value()
	if activeContest == "" {
		t.Fatal("selectEvent did not set an active contest selection")
	}

	// Edit the earlier, no-contest QSO and save.
	m.beginEditQSO(original)
	if got := m.contestFields[contestName].Value(); got != "" {
		t.Fatalf("contestFields[contestName] while editing = %q, want blank (the edited QSO's own, contest-less value)", got)
	}
	m.fields[fieldRSTSent].SetValue("579")
	m, _ = m.logCurrentQSO()

	if got := m.contestFields[contestName].Value(); got != activeContest {
		t.Fatalf("contestFields[contestName] after saving an edit = %q, want the active contest %q restored", got, activeContest)
	}

	// The next QSO logged must be tagged with the active contest, not blank.
	m.fields[fieldCall].SetValue("K1ABC")
	m, _ = m.logCurrentQSO()
	var next qso
	if err := st.forEachQSOForContest(context.Background(), m.activeStation.ID, activeContest, func(q qso) error {
		if q.call == "K1ABC" {
			next = q
		}
		return nil
	}); err != nil {
		t.Fatalf("forEachQSOForContest: %v", err)
	}
	if next.call != "K1ABC" {
		t.Fatal("QSO logged after the edit was not tagged with the active contest")
	}
}

// TestEscCancelsEditWithoutQuitting covers the contextual Esc behavior: it
// must cancel an in-progress edit (discarding changes) rather than quitting
// the whole app, which is Esc's normal meaning on this screen.
func TestEscCancelsEditWithoutQuitting(t *testing.T) {
	st, err := openStore(filepath.Join(t.TempDir(), "logger.db"))
	if err != nil {
		t.Fatalf("openStore returned error: %v", err)
	}
	defer st.Close()

	m := initialModel(st)
	m.fields[fieldCall].SetValue("W1AW")
	m, _ = m.logCurrentQSO()
	m.beginEditQSO(m.recentQSOs[0])
	m.fields[fieldRSTSent].SetValue("579")

	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = updated.(model)
	if cmd != nil {
		t.Fatal("Esc while editing returned a command (expected no tea.Quit)")
	}
	if m.editingQSOID != 0 {
		t.Fatal("Esc did not cancel the in-progress edit")
	}
	got, err := st.qsoByID(m.activeStation.ID, m.recentQSOs[0].id)
	if err != nil {
		t.Fatal(err)
	}
	if got.rstSent == "579" {
		t.Fatal("Esc-cancelled edit was persisted to the database")
	}
}

// TestDeleteRequiresSecondDConfirmation covers the delete-arm/confirm/
// cancel-on-other-key flow, driven through Update the way real keystrokes
// would arrive.
func TestDeleteRequiresSecondDConfirmation(t *testing.T) {
	st, err := openStore(filepath.Join(t.TempDir(), "logger.db"))
	if err != nil {
		t.Fatalf("openStore returned error: %v", err)
	}
	defer st.Close()

	m := initialModel(st)
	m.fields[fieldCall].SetValue("W1AW")
	m, _ = m.logCurrentQSO()

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyF9})
	m = updated.(model)
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("d")})
	m = updated.(model)
	if !m.deleteArmed {
		t.Fatal("first d press did not arm delete")
	}
	if count, _ := st.count(m.activeStation.ID); count != 1 {
		t.Fatal("first d press deleted the row instead of arming confirmation")
	}

	// Any other key cancels the armed delete.
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m = updated.(model)
	if m.deleteArmed {
		t.Fatal("a non-d key did not cancel the armed delete")
	}
	if count, _ := st.count(m.activeStation.ID); count != 1 {
		t.Fatal("delete happened despite being cancelled")
	}

	// Arm again and confirm this time.
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("d")})
	m = updated.(model)
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("d")})
	m = updated.(model)
	if count, err := st.count(m.activeStation.ID); err != nil || count != 0 {
		t.Fatalf("count after confirmed delete = %d, err = %v, want 0", count, err)
	}
}

// TestZapDeletesMostRecentlyLoggedQSO covers the ZAP typed command: typing
// ZAP into the Call field and pressing Enter while entering a new QSO
// deletes the single most recent QSO instead of logging "ZAP" as a callsign.
func TestZapDeletesMostRecentlyLoggedQSO(t *testing.T) {
	st, err := openStore(filepath.Join(t.TempDir(), "logger.db"))
	if err != nil {
		t.Fatalf("openStore returned error: %v", err)
	}
	defer st.Close()

	m := initialModel(st)
	m.fields[fieldCall].SetValue("W1AW")
	m, _ = m.logCurrentQSO()
	if count, _ := st.count(m.activeStation.ID); count != 1 {
		t.Fatalf("count after logging = %d, want 1", count)
	}

	m.focusField(fieldCall)
	m.fields[fieldCall].SetValue("ZAP")
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(model)

	if count, err := st.count(m.activeStation.ID); err != nil || count != 0 {
		t.Fatalf("count after ZAP = %d, err = %v, want 0", count, err)
	}
	if !strings.Contains(m.statusMsg, "ZAP") || !strings.Contains(m.statusMsg, "W1AW") {
		t.Fatalf("statusMsg = %q, want a ZAP confirmation naming W1AW", m.statusMsg)
	}
	if m.fields[fieldCall].Value() != "" {
		t.Fatalf("Call field = %q after ZAP, want cleared", m.fields[fieldCall].Value())
	}
}

// TestZapWithNoQSOsIsANoop guards against ZAP panicking or erroring when
// there's nothing to delete yet.
func TestZapWithNoQSOsIsANoop(t *testing.T) {
	st, err := openStore(filepath.Join(t.TempDir(), "logger.db"))
	if err != nil {
		t.Fatalf("openStore returned error: %v", err)
	}
	defer st.Close()

	m := initialModel(st)
	m.focusField(fieldCall)
	m.fields[fieldCall].SetValue("ZAP")
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(model)

	if !strings.Contains(m.statusMsg, "no QSO to delete") {
		t.Fatalf("statusMsg = %q, want a no-op confirmation", m.statusMsg)
	}
}

// TestSlashZDeletesQSOCurrentlyLoadedForEditing covers the /Z typed command:
// typing /Z into the Call field and pressing Enter while an existing QSO is
// loaded for editing deletes it instead of saving whatever's on screen.
func TestSlashZDeletesQSOCurrentlyLoadedForEditing(t *testing.T) {
	st, err := openStore(filepath.Join(t.TempDir(), "logger.db"))
	if err != nil {
		t.Fatalf("openStore returned error: %v", err)
	}
	defer st.Close()

	m := initialModel(st)
	m.fields[fieldCall].SetValue("W1AW")
	m, _ = m.logCurrentQSO()
	target := m.recentQSOs[0]
	m.beginEditQSO(target)
	if m.editingQSOID != target.id {
		t.Fatalf("editingQSOID = %d, want %d", m.editingQSOID, target.id)
	}

	m.fields[fieldCall].SetValue("/Z")
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(model)

	if m.editingQSOID != 0 {
		t.Fatalf("editingQSOID = %d after /Z, want 0", m.editingQSOID)
	}
	if count, err := st.count(m.activeStation.ID); err != nil || count != 0 {
		t.Fatalf("count after /Z = %d, err = %v, want 0", count, err)
	}
	if !strings.Contains(m.statusMsg, "/Z") {
		t.Fatalf("statusMsg = %q, want a /Z confirmation", m.statusMsg)
	}
}

// TestSlashXTogglesUnscoredFlagAndExcludesFromScore covers the /X typed
// command end to end: it flips the flag on the recalled QSO without
// otherwise touching it, and a QSO so flagged is excluded from
// contestState's score while still appearing in Cabrillo output as an
// X-QSO: line rather than being dropped entirely.
func TestSlashXTogglesUnscoredFlagAndExcludesFromScore(t *testing.T) {
	st, err := openStore(filepath.Join(t.TempDir(), "logger.db"))
	if err != nil {
		t.Fatalf("openStore returned error: %v", err)
	}
	defer st.Close()

	m := initialModel(st)
	m.contestFields[contestName].SetValue("TEST-CONTEST")
	m.fields[fieldCall].SetValue("W1AW")
	m.contestFields[contestSerialSent].SetValue("001")
	m.contestFields[contestExchangeRcvd].SetValue("599")
	m, _ = m.logCurrentQSO()
	target := m.recentQSOs[0]

	event := eventDefinition{ID: "TEST-CONTEST", CabrilloLayout: "cw_rst_exchange", Scoring: &scoringRules{PointsPerQSO: 1, Multiplier: "unique_call"}}
	before, err := computeContestScore(context.Background(), m.activeStation, event, "TEST-CONTEST", st)
	if err != nil {
		t.Fatal(err)
	}
	if before.total() != 1 {
		t.Fatalf("score before /X = %d, want 1", before.total())
	}

	m.beginEditQSO(target)
	m.fields[fieldCall].SetValue("/X")
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(model)

	if m.editingQSOID != 0 {
		t.Fatalf("editingQSOID = %d after /X, want 0", m.editingQSOID)
	}
	if !strings.Contains(m.statusMsg, "/X") || !strings.Contains(m.statusMsg, "unscored") {
		t.Fatalf("statusMsg = %q, want an /X unscored confirmation", m.statusMsg)
	}
	if count, err := st.count(m.activeStation.ID); err != nil || count != 1 {
		t.Fatalf("count after /X = %d, err = %v, want 1 (still logged, not deleted)", count, err)
	}

	after, err := computeContestScore(context.Background(), m.activeStation, event, "TEST-CONTEST", st)
	if err != nil {
		t.Fatal(err)
	}
	if after.total() != 0 {
		t.Fatalf("score after /X = %d, want 0 (unscored QSO must not count)", after.total())
	}

	var buf strings.Builder
	if _, _, err := exportCabrillo(context.Background(), &buf, m.activeStation, event, "TEST-CONTEST", st); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "X-QSO:") {
		t.Fatalf("Cabrillo output missing X-QSO: line for unscored QSO:\n%s", buf.String())
	}
	if strings.Contains(buf.String(), "\nQSO:") {
		t.Fatalf("Cabrillo output has a plain QSO: line, want only X-QSO: for the unscored contact:\n%s", buf.String())
	}

	// Toggling /X again restores it to scored.
	m.beginEditQSO(target)
	m.fields[fieldCall].SetValue("/X")
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(model)
	if !strings.Contains(m.statusMsg, "restored to scored") {
		t.Fatalf("statusMsg = %q, want a restored-to-scored confirmation", m.statusMsg)
	}
	restored, err := computeContestScore(context.Background(), m.activeStation, event, "TEST-CONTEST", st)
	if err != nil {
		t.Fatal(err)
	}
	if restored.total() != 1 {
		t.Fatalf("score after restoring /X = %d, want 1", restored.total())
	}
}

// TestSetDupeResetsBaselineSoEarlierQSOIsNoLongerADupe covers the SETDUPE
// typed command: after resetting the baseline to now, a station worked
// earlier no longer blocks re-working it, but a station worked after the
// reset still does.
func TestSetDupeResetsBaselineSoEarlierQSOIsNoLongerADupe(t *testing.T) {
	st, err := openStore(filepath.Join(t.TempDir(), "logger.db"))
	if err != nil {
		t.Fatalf("openStore returned error: %v", err)
	}
	defer st.Close()

	m := initialModel(st)
	m.fields[fieldCall].SetValue("W1AW")
	m, _ = m.logCurrentQSO()
	// Backdate the first contact well before the SETDUPE reset below: real
	// QSOs are seconds apart, but this test logs several in the same
	// instant, and time_on's one-second resolution would otherwise make the
	// "since" floor's >= inclusive of a same-second QSO it's meant to
	// exclude.
	if _, err := st.db.Exec(`UPDATE qso SET qso_date = '20200101', time_on = '000000' WHERE id = ?`, m.recentQSOs[0].id); err != nil {
		t.Fatal(err)
	}

	m.focusField(fieldCall)
	m.fields[fieldCall].SetValue("SETDUPE")
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(model)
	if m.dupeBaselineAfter.IsZero() {
		t.Fatal("SETDUPE did not set dupeBaselineAfter")
	}
	if !strings.Contains(m.statusMsg, "SETDUPE") {
		t.Fatalf("statusMsg = %q, want a SETDUPE confirmation", m.statusMsg)
	}

	m.fields[fieldCall].SetValue("W1AW")
	m, _ = m.logCurrentQSO()
	if count, err := st.count(m.activeStation.ID); err != nil || count != 2 {
		t.Fatalf("count after re-working W1AW post-SETDUPE = %d, err = %v, want 2 (not blocked as a dupe)", count, err)
	}

	// A QSO logged after the reset is still a real dupe.
	m.fields[fieldCall].SetValue("W1AW")
	m, _ = m.logCurrentQSO()
	if count, err := st.count(m.activeStation.ID); err != nil || count != 2 {
		t.Fatalf("count after a genuine post-reset dupe = %d, err = %v, want 2 (should have been rejected)", count, err)
	}
	if !strings.Contains(m.statusMsg, "DUPE") {
		t.Fatalf("statusMsg = %q, want a DUPE rejection for the post-reset repeat", m.statusMsg)
	}
}

func TestReturningToCallResetsQSOTimer(t *testing.T) {
	st, err := openStore(filepath.Join(t.TempDir(), "logger.db"))
	if err != nil {
		t.Fatalf("openStore returned error: %v", err)
	}
	defer st.Close()

	m := initialModel(st)
	m.fields[fieldCall].SetValue("W1AW")
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyTab})
	m = updated.(model)
	if m.qsoStartedAt.IsZero() {
		t.Fatal("Tab leaving Call did not start QSO timer")
	}
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyShiftTab})
	if !updated.(model).qsoStartedAt.IsZero() {
		t.Fatal("Shift+Tab returning to Call did not reset QSO timer")
	}
}

func TestQSOEntryTabOrder(t *testing.T) {
	st, err := openStore(filepath.Join(t.TempDir(), "logger.db"))
	if err != nil {
		t.Fatalf("openStore returned error: %v", err)
	}
	defer st.Close()

	m := initialModel(st)
	want := []int{fieldRSTSent, fieldRSTRcvd, fieldBand, fieldFrequency, fieldCall}
	for _, field := range want {
		updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyTab})
		m = updated.(model)
		if m.focusIdx != field {
			t.Fatalf("focus after Tab = %d, want %d", m.focusIdx, field)
		}
	}
}

func TestBandSelectorSetsValidDefaultFrequency(t *testing.T) {
	st, err := openStore(filepath.Join(t.TempDir(), "logger.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	m := initialModel(st)
	m.focusField(fieldBand)
	m.selectBand(1)
	if m.qsoBand() != "17M" || m.qsoFrequency() != "18.080" {
		t.Fatalf("selected band/frequency = %s/%s, want 17M/18.080", m.qsoBand(), m.qsoFrequency())
	}
}

// TestAdifExportCmdWritesTimestampedFileToDownloads covers the in-app ADIF
// export end to end: the tea.Cmd it returns must write a real file under
// the operator's Downloads folder and report its path and QSO count via
// adifExportedMsg, not just format a filename.
func TestAdifExportCmdWritesTimestampedFileToDownloads(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	st, err := openStore(filepath.Join(t.TempDir(), "logger.db"))
	if err != nil {
		t.Fatalf("openStore returned error: %v", err)
	}
	defer st.Close()

	m := initialModel(st)
	m.activeStation.Callsign = "W4GNS"
	logged := validTestQSO()
	logged.profileID = m.activeStation.ID
	if _, err := st.insertQSO(logged); err != nil {
		t.Fatalf("insertQSO returned error: %v", err)
	}

	msg, ok := m.adifExportCmd()().(adifExportedMsg)
	if !ok {
		t.Fatalf("adifExportCmd()() = %T, want adifExportedMsg", msg)
	}
	if msg.err != nil {
		t.Fatalf("adifExportedMsg.err = %v, want nil", msg.err)
	}
	if msg.count != 1 {
		t.Fatalf("adifExportedMsg.count = %d, want 1", msg.count)
	}
	wantDir := filepath.Join(home, "Downloads")
	if filepath.Dir(msg.path) != wantDir {
		t.Fatalf("export path = %q, want it under %q", msg.path, wantDir)
	}
	if !strings.HasPrefix(filepath.Base(msg.path), "W4GNS_") || !strings.HasSuffix(msg.path, ".adi") {
		t.Fatalf("export filename = %q, want a W4GNS_<timestamp>.adi shape", filepath.Base(msg.path))
	}
	if _, err := os.Stat(msg.path); err != nil {
		t.Fatalf("exported file does not exist: %v", err)
	}
}

func TestADIFExportPath(t *testing.T) {
	path, ok := adifExportPath([]string{"--in-current-terminal", "--export-adif", "log.adi"})
	if !ok || path != "log.adi" {
		t.Fatalf("adifExportPath = %q, %t", path, ok)
	}
	if _, ok := adifExportPath([]string{"--export-adif"}); ok {
		t.Fatal("adifExportPath accepted a missing path")
	}
}

// TestValidateArgsRejectsUnrecognizedAndIncompleteFlags guards against a
// typo'd flag (e.g. --export-adiff) or a dropped path argument silently
// falling through to launching the TUI instead of reporting a usage error.
func TestValidateArgsRejectsUnrecognizedAndIncompleteFlags(t *testing.T) {
	for _, args := range [][]string{
		{"--export-adiff", "log.adi"},
		{"--exprot-adif", "log.adi"},
		{"--export-adif"},
		{"--import-adif"},
		// A recognized flag must not be consumed as another flag's path operand.
		{"--export-adif", "--version"},
		{"--export-adif", "--import-adif", "log.adi"},
		// Conflicting actions must be rejected outright.
		{"--export-adif", "a.adi", "--import-adif", "b.adi"},
	} {
		if err := validateArgs(args); err == nil {
			t.Errorf("validateArgs(%v) returned no error", args)
		}
	}
	for _, args := range [][]string{
		nil,
		{"--export-adif", "log.adi"},
		{"--import-adif", "log.adi"},
		{"--in-current-terminal"},
		{"--terminal-child"},
		{"--version"},
	} {
		if err := validateArgs(args); err != nil {
			t.Errorf("validateArgs(%v) returned error: %v", args, err)
		}
	}
}

func TestPathsReferToSameFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "logger.db")
	if !pathsReferToSameFile(path, path) {
		t.Fatal("identical paths were not recognized")
	}
}

// TestRenderFieldGridPlacesFieldsSideBySide guards the actual bug behind the
// "can't scroll up to find Callsign" report: writing two multi-line
// bordered field boxes to a strings.Builder back to back (the previous
// approach in every caller of this grid) stacks them vertically instead of
// side by side, since plain string concatenation has no notion of "these
// two belong on the same row." Two boxes on the same row must both start
// their top border on the grid's very first output line.
func TestRenderFieldGridPlacesFieldsSideBySide(t *testing.T) {
	labels := []string{"One", "Two"}
	fields := []textinput.Model{newStationTextInput("a", 10), newStationTextInput("b", 10)}
	out := renderFieldGrid(labels, fields, -1)
	lines := strings.Split(out, "\n")
	if len(lines) == 0 || strings.Count(lines[0], "╭") != 2 {
		t.Fatalf("renderFieldGrid did not place two fields on the same row, first line = %q", firstOr(lines, ""))
	}
}

func firstOr(lines []string, def string) string {
	if len(lines) == 0 {
		return def
	}
	return lines[0]
}

// TestStationSetupViewShowsCallsignNearTop is a direct regression guard for
// the reported symptom: with the field-grid bug (see
// TestRenderFieldGridPlacesFieldsSideBySide) Station Setup rendered one
// field per row instead of two, and after Cabrillo's category/address
// fields were added, that pushed the whole page to 64 lines — tall enough
// that Callsign (the second field) scrolled out of view on an ordinary
// terminal, with no way to scroll back in alt-screen mode.
func TestStationSetupViewShowsCallsignNearTop(t *testing.T) {
	st, err := openStore(filepath.Join(t.TempDir(), "logger.db"))
	if err != nil {
		t.Fatalf("openStore returned error: %v", err)
	}
	defer st.Close()

	m := initialModel(st)
	m.openStationSetup()
	view := m.stationSetupView()
	lines := strings.Split(view, "\n")
	found := -1
	for i, line := range lines {
		if strings.Contains(line, "Callsign") {
			found = i
			break
		}
	}
	if found == -1 {
		t.Fatal("Callsign field not found anywhere in the Station Setup view")
	}
	const wantWithinLines = 10
	if found >= wantWithinLines {
		t.Fatalf("Callsign appears at line %d, want it within the first %d lines so it's visible without scrolling", found, wantWithinLines)
	}
}

func TestScreenHotkeysSwitchBetweenQSOEntryAndStationSetup(t *testing.T) {
	st, err := openStore(filepath.Join(t.TempDir(), "logger.db"))
	if err != nil {
		t.Fatalf("openStore returned error: %v", err)
	}
	defer st.Close()

	m := initialModel(st)
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyF2})
	m = updated.(model)
	if m.screen != stationSetupScreen {
		t.Fatal("F2 did not open Station Setup")
	}
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyF1})
	if updated.(model).screen != qsoEntryScreen {
		t.Fatal("F1 did not return to QSO Entry")
	}
}

func TestF3OpensK3LRClusterScreen(t *testing.T) {
	st, err := openStore(filepath.Join(t.TempDir(), "logger.db"))
	if err != nil {
		t.Fatalf("openStore returned error: %v", err)
	}
	defer st.Close()

	m := initialModel(st)
	m.activeStation.Callsign = "W4GNS"
	updated, command := m.Update(tea.KeyMsg{Type: tea.KeyF3})
	got := updated.(model)
	if got.screen != clusterScreen {
		t.Fatal("F3 did not open Cluster screen")
	}
	if !got.clusterConnecting || command == nil {
		t.Fatal("F3 did not begin the K3LR connection")
	}
}

// TestConnectClusterIfNeededSkipsWhenAlreadyConnecting guards the startup
// auto-connect added for the DX Spots panel: main() pre-flags
// clusterConnecting before the program starts so Init() can fire the actual
// connectK3LR command. If the operator then presses F3 while that connect
// is still in flight, openCluster (via connectClusterIfNeeded) must not
// start a second, redundant connection.
func TestConnectClusterIfNeededSkipsWhenAlreadyConnecting(t *testing.T) {
	st, err := openStore(filepath.Join(t.TempDir(), "logger.db"))
	if err != nil {
		t.Fatalf("openStore returned error: %v", err)
	}
	defer st.Close()

	m := initialModel(st)
	m.activeStation.Callsign = "W4GNS"
	if cmd := m.connectClusterIfNeeded(); cmd == nil {
		t.Fatal("connectClusterIfNeeded returned nil on the first call")
	}
	if !m.clusterConnecting {
		t.Fatal("connectClusterIfNeeded did not set clusterConnecting")
	}
	generationAfterFirst := m.clusterGeneration
	if cmd := m.connectClusterIfNeeded(); cmd != nil {
		t.Fatal("connectClusterIfNeeded started a second connection while one was already in flight")
	}
	if m.clusterGeneration != generationAfterFirst {
		t.Fatal("connectClusterIfNeeded bumped clusterGeneration on the skipped second call")
	}
}

// TestClusterConnectedMsgHandledOnNonClusterScreen is a direct regression
// guard for a real bug: clusterConnectedMsg/clusterLineMsg were only
// handled inside updateCluster, which Update only calls when
// m.screen == clusterScreen. Once the DX cluster started connecting at app
// startup (while the operator is on QSO Entry, not the DX Cluster screen —
// see connectClusterIfNeeded), that message arrived while off the cluster
// screen and was silently dropped: clusterConnecting stayed true and
// clusterClient stayed nil forever, so the DX Spots panel got stuck showing
// "connecting…" even though the TCP connection had actually succeeded.
func TestClusterConnectedMsgHandledOnNonClusterScreen(t *testing.T) {
	st, err := openStore(filepath.Join(t.TempDir(), "logger.db"))
	if err != nil {
		t.Fatalf("openStore returned error: %v", err)
	}
	defer st.Close()

	m := initialModel(st)
	m.activeStation.Callsign = "W4GNS"
	m.connectClusterIfNeeded()
	if m.screen != qsoEntryScreen {
		t.Fatalf("screen = %v, want qsoEntryScreen (the bug only reproduces off the DX Cluster screen)", m.screen)
	}

	updated, _ := m.Update(clusterConnectedMsg{generation: m.clusterGeneration, client: &clusterClient{}})
	got := updated.(model)
	if got.clusterConnecting {
		t.Fatal("clusterConnecting is still true after clusterConnectedMsg — the message was dropped")
	}
	if got.clusterClient == nil {
		t.Fatal("clusterClient is still nil after a successful clusterConnectedMsg — the message was dropped")
	}
}

func TestClusterConnectionStateRejectsDuplicateAndStaleResults(t *testing.T) {
	st, err := openStore(filepath.Join(t.TempDir(), "logger.db"))
	if err != nil {
		t.Fatalf("openStore returned error: %v", err)
	}
	defer st.Close()

	m := initialModel(st)
	m.activeStation.Callsign = "W4GNS"
	m.openCluster()
	updated, _ := m.updateCluster(tea.KeyMsg{Type: tea.KeyF5})
	m = updated.(model)
	if !m.clusterConnecting || m.clusterGeneration != 1 {
		t.Fatalf("first connect state = connecting:%t generation:%d", m.clusterConnecting, m.clusterGeneration)
	}
	updated, _ = m.updateCluster(tea.KeyMsg{Type: tea.KeyF5})
	m = updated.(model)
	if m.clusterGeneration != 1 {
		t.Fatalf("duplicate F5 started another connection generation: %d", m.clusterGeneration)
	}
	m.clusterConnecting = false
	m.clusterStatus = "new connection stays active"
	m.clusterGeneration = 2
	updated, _ = m.updateCluster(clusterConnectedMsg{generation: 1, err: errors.New("old connection failed")})
	m = updated.(model)
	if m.clusterStatus != "new connection stays active" || m.clusterGeneration != 2 {
		t.Fatalf("stale connection result changed state: %#v", m)
	}
}

// TestQRZCallsignLookupMsgIgnoresStaleResult guards against a slow QRZ XML
// lookup for one callsign landing after the operator has already moved on to
// a different call: the result must be discarded, not written into the
// current QSO's details.
func TestQRZCallsignLookupMsgIgnoresStaleResult(t *testing.T) {
	st, err := openStore(filepath.Join(t.TempDir(), "logger.db"))
	if err != nil {
		t.Fatalf("openStore returned error: %v", err)
	}
	defer st.Close()

	m := initialModel(st)
	m.fields[fieldCall].SetValue("K1ABC")

	updated, _ := m.Update(qrzCallsignLookupMsg{
		call:   "AA7BQ",
		record: qrzCallsignRecord{name: "Fred Lloyd", qth: "Phoenix", state: "AZ", grid: "DM43"},
	})
	m = updated.(model)
	if m.detailFields[detailName].Value() != "" {
		t.Fatalf("detailFields[detailName] = %q, want blank for a stale lookup result", m.detailFields[detailName].Value())
	}
}

// TestQRZCallsignLookupMsgFillsOnlyBlankFields guards against clobbering
// details the operator already typed (or that were loaded from an existing
// logged QSO for editing) with a QRZ lookup result for the same call.
func TestQRZCallsignLookupMsgFillsOnlyBlankFields(t *testing.T) {
	st, err := openStore(filepath.Join(t.TempDir(), "logger.db"))
	if err != nil {
		t.Fatalf("openStore returned error: %v", err)
	}
	defer st.Close()

	m := initialModel(st)
	m.fields[fieldCall].SetValue("AA7BQ")
	m.detailFields[detailQTH].SetValue("operator-entered QTH")

	updated, _ := m.Update(qrzCallsignLookupMsg{
		call:       "AA7BQ",
		sessionKey: "session123",
		record:     qrzCallsignRecord{name: "Fred Lloyd", qth: "Phoenix", state: "AZ", grid: "DM43"},
	})
	m = updated.(model)
	if m.detailFields[detailName].Value() != "Fred Lloyd" {
		t.Fatalf("detailFields[detailName] = %q, want Fred Lloyd", m.detailFields[detailName].Value())
	}
	if m.detailFields[detailQTH].Value() != "operator-entered QTH" {
		t.Fatalf("detailFields[detailQTH] = %q, want the operator-entered value left untouched", m.detailFields[detailQTH].Value())
	}
	if m.detailFields[detailState].Value() != "AZ" || m.detailFields[detailGrid].Value() != "DM43" {
		t.Fatalf("state/grid = %q/%q, want AZ/DM43", m.detailFields[detailState].Value(), m.detailFields[detailGrid].Value())
	}
	if m.qrzXMLSessionKey != "session123" {
		t.Fatalf("qrzXMLSessionKey = %q, want session123 to be cached for the next lookup", m.qrzXMLSessionKey)
	}
}

// TestQRZLookupRequestIDsDoNotLeaveStaleSameCallBindings covers the ordering
// that a callsign FIFO cannot represent: one lookup resolves before its QSO is
// saved, then another lookup for the same call resolves after save. The latter
// must enrich the second row, not consume a stale first-row id.
func TestQRZLookupRequestIDsDoNotLeaveStaleSameCallBindings(t *testing.T) {
	st, err := openStore(filepath.Join(t.TempDir(), "logger.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	m := initialModel(st)
	m.qrzXMLCreds = qrzXMLCreds{username: "user", password: "pass"}
	m.fields[fieldCall].SetValue("W1AW")
	if cmd := m.autoFillFromQRZ(); cmd == nil {
		t.Fatal("configured QRZ lookup returned nil")
	}
	firstRequest := m.qrzActiveLookup
	updated, _ := m.Update(qrzCallsignLookupMsg{requestID: firstRequest, call: "W1AW", record: qrzCallsignRecord{name: "Early"}})
	m = updated.(model)
	if m.qrzActiveLookup != 0 || len(m.qrzLookups) != 0 {
		t.Fatal("pre-save QRZ result left a pending lookup binding")
	}
	m, _ = m.logCurrentQSO()

	// Work the same call on another band so the normal duplicate guard permits
	// a second QSO while exercising same-callsign correlation.
	m.fields[fieldCall].SetValue("W1AW")
	m.fields[fieldBand].SetValue("40M")
	m.fields[fieldFrequency].SetValue("7.025")
	if cmd := m.autoFillFromQRZ(); cmd == nil {
		t.Fatal("second configured QRZ lookup returned nil")
	}
	secondRequest := m.qrzActiveLookup
	m, _ = m.logCurrentQSO()
	secondID := m.recentQSOs[0].id

	updated, _ = m.Update(qrzCallsignLookupMsg{requestID: secondRequest, call: "W1AW", record: qrzCallsignRecord{name: "Late"}})
	m = updated.(model)
	second, err := st.qsoByID(m.activeStation.ID, secondID)
	if err != nil {
		t.Fatal(err)
	}
	if second.name != "Late" {
		t.Fatalf("late QRZ result enriched %q, want second QSO name Late", second.name)
	}
}

// TestSaveStationSetupPersistsQRZXMLCredentials covers entering QRZ XML
// login in Station Setup: saving must write it to the credentials file (so
// it survives a restart), update the in-memory creds used by the next
// lookup, and drop any cached session key since it belonged to whichever
// account was previously configured.
func TestSaveStationSetupPersistsQRZXMLCredentials(t *testing.T) {
	st, err := openStore(filepath.Join(t.TempDir(), "logger.db"))
	if err != nil {
		t.Fatalf("openStore returned error: %v", err)
	}
	defer st.Close()
	oldWD, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(oldWD)
	// legacyOrStablePath prefers a "qrz.comXMLlogin" already present in the
	// cwd; chdir into an empty tempdir first so this test's outcome doesn't
	// depend on whatever the invoking process's working directory contains.
	if err := os.Chdir(t.TempDir()); err != nil {
		t.Fatal(err)
	}
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	m := initialModel(st)
	m.qrzXMLSessionKey = "stale-session"
	m.openStationSetup()
	m.stationFields[stationQRZXMLUserField].SetValue("newuser")
	m.stationFields[stationQRZXMLPassField].SetValue("newpass")

	m.saveStationSetup()

	if m.qrzXMLCreds.username != "newuser" || m.qrzXMLCreds.password != "newpass" {
		t.Fatalf("qrzXMLCreds = %+v, want newuser/newpass", m.qrzXMLCreds)
	}
	if m.qrzXMLSessionKey != "" {
		t.Fatalf("qrzXMLSessionKey = %q, want cleared after credentials changed", m.qrzXMLSessionKey)
	}
	if m.screen != qsoEntryScreen {
		t.Fatalf("screen after save = %v, want qsoEntryScreen", m.screen)
	}
	if got := loadQRZXMLCredentials(); got.username != "newuser" || got.password != "newpass" {
		t.Fatalf("loadQRZXMLCredentials() after save = %+v, want newuser/newpass", got)
	}
}

func TestTruncateToWidth(t *testing.T) {
	if got := truncateToWidth("hello", 10); got != "hello" {
		t.Fatalf("truncateToWidth(short) = %q, want unchanged", got)
	}
	if got := truncateToWidth("hello world", 5); got != "hello" {
		t.Fatalf("truncateToWidth(long) = %q, want hello", got)
	}
	if got := truncateToWidth("anything", 0); got != "" {
		t.Fatalf("truncateToWidth(width=0) = %q, want empty", got)
	}
}

// TestDxSpotsPanelHidesBelowMinWidth covers the narrow-terminal fallback:
// the caller (View) relies on "" meaning "render Recent QSOs alone", so a
// width too small for even "HH:MM freq call" must return exactly that.
func TestDxSpotsPanelHidesBelowMinWidth(t *testing.T) {
	st, err := openStore(filepath.Join(t.TempDir(), "logger.db"))
	if err != nil {
		t.Fatalf("openStore returned error: %v", err)
	}
	defer st.Close()

	m := initialModel(st)
	if got := m.dxSpotsPanel(dxSpotsPanelMinWidth - 1); got != "" {
		t.Fatalf("dxSpotsPanel(too narrow) = %q, want empty", got)
	}
}

// TestDxSpotsPanelOmitsCommentWhenNarrow guards the mid-width case: wide
// enough to show spots at all, but not wide enough for a comment to be
// appended without risking a wrap.
func TestDxSpotsPanelOmitsCommentWhenNarrow(t *testing.T) {
	st, err := openStore(filepath.Join(t.TempDir(), "logger.db"))
	if err != nil {
		t.Fatalf("openStore returned error: %v", err)
	}
	defer st.Close()

	m := initialModel(st)
	m.addClusterSpot(clusterSpot{Spotter: "K1ABC", Frequency: "14025.0", Callsign: "W1AW", Comment: "this comment should not appear", Received: time.Now()})
	got := m.dxSpotsPanel(dxSpotsPanelCommentMinWidth - 1)
	if strings.Contains(got, "this comment") {
		t.Fatalf("dxSpotsPanel(narrow) included the comment, want it omitted: %q", got)
	}
	if !strings.Contains(got, "W1AW") {
		t.Fatalf("dxSpotsPanel(narrow) = %q, want it to still show the callsign", got)
	}
}

// TestDxSpotsPanelShowsCommentWhenWide is the companion case: wide enough,
// the comment must actually appear.
func TestDxSpotsPanelShowsCommentWhenWide(t *testing.T) {
	st, err := openStore(filepath.Join(t.TempDir(), "logger.db"))
	if err != nil {
		t.Fatalf("openStore returned error: %v", err)
	}
	defer st.Close()

	m := initialModel(st)
	m.addClusterSpot(clusterSpot{Spotter: "K1ABC", Frequency: "14025.0", Callsign: "W1AW", Comment: "cq test", Received: time.Now()})
	got := m.dxSpotsPanel(80)
	if !strings.Contains(got, "cq test") {
		t.Fatalf("dxSpotsPanel(wide) = %q, want the comment included", got)
	}
}

// TestDxSpotsPanelLimitsToVisibleRows guards against the panel growing
// unbounded and pushing the layout below Recent QSOs' own row count.
func TestDxSpotsPanelLimitsToVisibleRows(t *testing.T) {
	st, err := openStore(filepath.Join(t.TempDir(), "logger.db"))
	if err != nil {
		t.Fatalf("openStore returned error: %v", err)
	}
	defer st.Close()

	m := initialModel(st)
	for i := 0; i < recentQSOsVisibleRows+5; i++ {
		m.addClusterSpot(clusterSpot{Spotter: "K1ABC", Frequency: "14025.0", Callsign: fmt.Sprintf("W%dAW", i), Received: time.Now()})
	}
	got := m.dxSpotsPanel(80)
	if lines := strings.Count(got, "\n") + 1; lines != recentQSOsVisibleRows+1 { // +1 for the title line
		t.Fatalf("dxSpotsPanel line count = %d, want %d (title + %d rows)", lines, recentQSOsVisibleRows+1, recentQSOsVisibleRows)
	}
}

// TestSaveStationSetupRetriesClusterConnectionWhenCallsignAdded guards
// against a real gap: connectClusterIfNeeded previously only ran once, at
// app startup. An operator who launches the app before filling in Station
// Setup (e.g. a first run) would have no callsign yet at that moment, so
// the auto-connect would silently no-op — and, without this, saving
// Station Setup afterward would never retry it, leaving no path to a
// connection short of manually visiting the DX Cluster (F3) screen.
func TestSaveStationSetupRetriesClusterConnectionWhenCallsignAdded(t *testing.T) {
	st, err := openStore(filepath.Join(t.TempDir(), "logger.db"))
	if err != nil {
		t.Fatalf("openStore returned error: %v", err)
	}
	defer st.Close()

	m := initialModel(st)
	if m.activeStation.Callsign != "" {
		t.Fatalf("test assumes no callsign configured yet, got %q", m.activeStation.Callsign)
	}
	m.openStationSetup()
	m.stationFields[stationCallsignField].SetValue("W4GNS")
	m.stationFields[stationTimezoneField].SetValue("UTC")

	cmd := m.saveStationSetup()
	if cmd == nil {
		t.Fatal("saveStationSetup returned a nil command after a callsign was added; the DX cluster connect never gets retried")
	}
	if !m.clusterConnecting {
		t.Fatal("saveStationSetup did not mark the cluster connection as in progress")
	}
}

// TestSaveStationSetupRotatesClusterAndContestStateOnIdentityChange ensures
// changing an already-configured station callsign cannot leave the DX cluster
// logged in as the old call or scoring against the old station entity.
func TestSaveStationSetupRotatesClusterAndContestStateOnIdentityChange(t *testing.T) {
	st, err := openStore(filepath.Join(t.TempDir(), "logger.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	m := initialModel(st)
	m.activeStation.Callsign = "W4GNS"
	m.activeStation.MyGridSquare = "EM12"
	cqww := m.events[eventIndex(t, m.events, "CQ-WW-CW")]
	m.selectEvent(cqww, cqww.Sessions[0])
	if m.contestIndex == nil {
		t.Fatal("selectEvent did not build the active contest index")
	}
	oldIndex := m.contestIndex
	m.clusterClient = &clusterClient{}
	m.clusterReconnect = true
	m.openStationSetup()
	m.stationFields[stationCallsignField].SetValue("DL1ABC")
	m.stationFields[stationGridField].SetValue("JO62")
	m.stationFields[stationTimezoneField].SetValue("UTC")

	cmd := m.saveStationSetup()
	if cmd == nil || !m.clusterConnecting {
		t.Fatal("saving a changed callsign did not start a replacement cluster connection")
	}
	if m.clusterClient != nil {
		t.Fatal("old cluster client survived a callsign change")
	}
	if m.contestIndex == nil || m.contestIndex == oldIndex {
		t.Fatal("saving a changed station identity did not rebuild the contest index")
	}
	if m.contestIndex.stationContinent != "EU" {
		t.Fatalf("rebuilt contest index continent = %q, want EU for DL1ABC", m.contestIndex.stationContinent)
	}
}

// TestCtrlPTogglesPostModeAndAddsDateTimeSlot covers the Ctrl+P toggle: it
// flips model.postMode, prefills the Date/Time field with the current UTC
// time on enable and clears it on disable, and the extra entrySlot only
// exists while postMode is true.
func TestCtrlPTogglesPostModeAndAddsDateTimeSlot(t *testing.T) {
	st, err := openStore(filepath.Join(t.TempDir(), "logger.db"))
	if err != nil {
		t.Fatalf("openStore returned error: %v", err)
	}
	defer st.Close()

	m := initialModel(st)
	baseSlots := len(m.entrySlots())

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyCtrlP})
	m = updated.(model)
	if !m.postMode {
		t.Fatal("Ctrl+P did not enable POST mode")
	}
	if len(m.entrySlots()) != baseSlots+1 {
		t.Fatalf("entrySlots count = %d after enabling POST mode, want %d", len(m.entrySlots()), baseSlots+1)
	}
	if m.postFields[postTimestamp].Value() == "" {
		t.Fatal("enabling POST mode did not prefill the Date/Time field")
	}

	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlP})
	m = updated.(model)
	if m.postMode {
		t.Fatal("second Ctrl+P did not disable POST mode")
	}
	if len(m.entrySlots()) != baseSlots {
		t.Fatalf("entrySlots count = %d after disabling POST mode, want %d", len(m.entrySlots()), baseSlots)
	}
	if m.postFields[postTimestamp].Value() != "" {
		t.Fatal("disabling POST mode did not clear the Date/Time field")
	}
}

// TestCtrlPBlockedWhileEditingQSO guards against toggling POST mode mid-edit,
// which would silently reshuffle the entry-slot layout under the operator
// while a different save path (the edit path, which doesn't consult
// postMode) is in flight.
func TestCtrlPBlockedWhileEditingQSO(t *testing.T) {
	st, err := openStore(filepath.Join(t.TempDir(), "logger.db"))
	if err != nil {
		t.Fatalf("openStore returned error: %v", err)
	}
	defer st.Close()

	m := initialModel(st)
	m.fields[fieldCall].SetValue("W1AW")
	m, _ = m.logCurrentQSO()
	m.beginEditQSO(m.recentQSOs[0])

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyCtrlP})
	m = updated.(model)
	if m.postMode {
		t.Fatal("Ctrl+P enabled POST mode while a QSO was loaded for editing")
	}
	if !strings.Contains(m.statusMsg, "edit") {
		t.Fatalf("statusMsg = %q, want a message explaining POST mode is blocked while editing", m.statusMsg)
	}
}

// TestPostModeLogsQSOWithTypedTimestampInsteadOfNow is the core behavior:
// with POST mode on, logCurrentQSO must use the operator-typed Date/Time
// field, not time.Now(), for both time-on and time-off.
func TestPostModeLogsQSOWithTypedTimestampInsteadOfNow(t *testing.T) {
	st, err := openStore(filepath.Join(t.TempDir(), "logger.db"))
	if err != nil {
		t.Fatalf("openStore returned error: %v", err)
	}
	defer st.Close()

	m := initialModel(st)
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyCtrlP})
	m = updated.(model)

	want := time.Date(2023, 6, 10, 14, 30, 0, 0, time.UTC)
	m.postFields[postTimestamp].SetValue(want.Format(postTimestampLayout))
	m.fields[fieldCall].SetValue("W1AW")

	m, _ = m.logCurrentQSO()
	if len(m.recentQSOs) == 0 {
		t.Fatal("logCurrentQSO in POST mode did not log a QSO")
	}
	if !m.recentQSOs[0].time.Equal(want) {
		t.Fatalf("logged time = %v, want %v", m.recentQSOs[0].time, want)
	}

	// recentQSOs doesn't select time_off, so check the stored end time directly.
	var qsoDate, timeOn, dateOff, timeOff string
	if err := st.db.QueryRow(`SELECT qso_date, time_on, qso_date_off, time_off FROM qso WHERE call = 'W1AW'`).Scan(&qsoDate, &timeOn, &dateOff, &timeOff); err != nil {
		t.Fatal(err)
	}
	if qsoDate+timeOn != "20230610143000" || dateOff+timeOff != "20230610143000" {
		t.Fatalf("stored time_on = %s %s, time_off = %s %s, want both 2023-06-10 14:30:00", qsoDate, timeOn, dateOff, timeOff)
	}
}

// TestPostModeRejectsUnparsableTimestamp guards against silently logging a
// QSO with a garbage or empty Date/Time value in POST mode — it must refuse
// to save and explain the expected format instead.
func TestPostModeRejectsUnparsableTimestamp(t *testing.T) {
	st, err := openStore(filepath.Join(t.TempDir(), "logger.db"))
	if err != nil {
		t.Fatalf("openStore returned error: %v", err)
	}
	defer st.Close()

	m := initialModel(st)
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyCtrlP})
	m = updated.(model)
	m.postFields[postTimestamp].SetValue("not a date")
	m.fields[fieldCall].SetValue("W1AW")

	m, _ = m.logCurrentQSO()
	if count, err := st.count(m.activeStation.ID); err != nil || count != 0 {
		t.Fatalf("count after bad POST-mode timestamp = %d, err = %v, want 0 (not logged)", count, err)
	}
	if !strings.Contains(m.statusMsg, "POST mode") {
		t.Fatalf("statusMsg = %q, want a POST-mode format error", m.statusMsg)
	}
}

// TestPostModeUsesTypedTimeForCasualDupeCheck ensures paper-log entry checks
// the contact's actual timestamp, not the wall-clock time when it is typed.
func TestPostModeUsesTypedTimeForCasualDupeCheck(t *testing.T) {
	st, err := openStore(filepath.Join(t.TempDir(), "logger.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	m := initialModel(st)
	prior := validTestQSO()
	prior.profileID = m.activeStation.ID
	if _, err := st.insertQSO(prior); err != nil {
		t.Fatal(err)
	}
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyCtrlP})
	m = updated.(model)
	m.postFields[postTimestamp].SetValue(prior.time.Add(5 * time.Minute).Format(postTimestampLayout))
	m.fields[fieldCall].SetValue(prior.call)

	m, _ = m.logCurrentQSO()
	if !strings.Contains(m.statusMsg, "DUPE") {
		t.Fatalf("POST duplicate status = %q, want DUPE", m.statusMsg)
	}
	if count, err := st.count(m.activeStation.ID); err != nil || count != 1 {
		t.Fatalf("count after duplicate POST entry = %d, err = %v, want 1", count, err)
	}
}

// TestPostModeEnterFastPathStillVisitsRSTBandFreqWithoutAContest guards a
// regression where enabling POST mode alone (no contest active) made the
// Call field's Enter fast-path — meant only for "a contest is active, so
// RST/Band/Freq are auto-filled and rarely need touching" — misfire, because
// it used to key off entrySlots() growing past fieldCount, which POST mode's
// trailing Date/Time slot also does. Enter from Call must still land on
// RST Sent (fieldRSTSent), not jump straight to the Date/Time slot.
func TestPostModeEnterFastPathStillVisitsRSTBandFreqWithoutAContest(t *testing.T) {
	st, err := openStore(filepath.Join(t.TempDir(), "logger.db"))
	if err != nil {
		t.Fatalf("openStore returned error: %v", err)
	}
	defer st.Close()

	m := initialModel(st)
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyCtrlP})
	m = updated.(model)
	if !m.postMode {
		t.Fatal("Ctrl+P did not enable POST mode")
	}
	m.focusField(fieldCall)
	m.fields[fieldCall].SetValue("W1AW")

	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(model)
	if m.focusIdx != fieldRSTSent {
		t.Fatalf("focusIdx after Enter from Call in POST mode (no contest) = %d, want %d (fieldRSTSent, the normal next field) — it should not fast-path to the Date/Time slot", m.focusIdx, fieldRSTSent)
	}
}

// TestPostModeSlotHiddenWhileEditingQSO guards a regression where the
// Date/Time slot stayed visible and editable during an edit (F9) that began
// while POST mode was already on, even though logCurrentQSO's edit branch
// never reads postFields — a QSO's timestamp isn't rewritten by an edit. A
// visible-but-inert field is misleading, so the slot must not appear at all
// while editingQSOID != 0.
func TestPostModeSlotHiddenWhileEditingQSO(t *testing.T) {
	st, err := openStore(filepath.Join(t.TempDir(), "logger.db"))
	if err != nil {
		t.Fatalf("openStore returned error: %v", err)
	}
	defer st.Close()

	m := initialModel(st)
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyCtrlP})
	m = updated.(model)
	baseSlotsWithPost := len(m.entrySlots())

	m.fields[fieldCall].SetValue("W1AW")
	m, _ = m.logCurrentQSO()
	m.beginEditQSO(m.recentQSOs[0])

	if got := len(m.entrySlots()); got != baseSlotsWithPost-1 {
		t.Fatalf("entrySlots count while editing with POST mode on = %d, want %d (Date/Time slot hidden)", got, baseSlotsWithPost-1)
	}
	for _, s := range m.entrySlots() {
		if s.post {
			t.Fatal("entrySlots still includes the POST Date/Time slot while editing a QSO")
		}
	}
}
