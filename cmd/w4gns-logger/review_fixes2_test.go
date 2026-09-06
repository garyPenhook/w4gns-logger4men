package main

import (
	"context"
	"strings"
	"testing"
	"time"
)

// TestForEachQSOForContestPreservesIOTAFields reproduces the Cabrillo
// export/scoring bug: iota_ref and my_iota_ref weren't in
// forEachQSOForContest's SELECT, so both the multiplier index rebuild and
// the Cabrillo writer saw a QSO's IOTA reference as blank even though it was
// logged and persisted. A same-island contact would score its multiplier
// only until the index (or an export) reread the QSO from the database.
func TestForEachQSOForContestPreservesIOTAFields(t *testing.T) {
	m := reviewModel(t)
	q := reviewQSO(m)
	q.contestID = "IOTA-TEST"
	q.iotaRef = "EU-005"
	q.myIotaRef = "EU-005"
	reviewInsert(t, m, q)

	var got qso
	found := false
	err := m.store.forEachQSOForContest(context.Background(), m.activeStation.ID, "IOTA-TEST", func(row qso) error {
		got = row
		found = true
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if !found {
		t.Fatal("forEachQSOForContest returned no rows for the inserted QSO")
	}
	if got.iotaRef != "EU-005" {
		t.Errorf("iotaRef = %q, want EU-005 (worked station's IOTA multiplier would be lost on reload)", got.iotaRef)
	}
	if got.myIotaRef != "EU-005" {
		t.Errorf("myIotaRef = %q, want EU-005 (island-to-island multiplier would be lost on reload)", got.myIotaRef)
	}
}

// TestCancelEditRestoresNextSerialAcrossContestSwitch reproduces the serial
// corruption bug: opening the Event Catalog while editing an older QSO and
// picking a different serial-based session overwrites m.nextSerial for that
// other contest. Cancelling the edit put the contest-name/exchange fields
// back but left m.nextSerial (and the displayed Sent Serial field) at the
// other contest's value instead of the active contest's own running count.
func TestCancelEditRestoresNextSerialAcrossContestSwitch(t *testing.T) {
	m := reviewModel(t)
	cwOpen := m.events[eventIndex(t, m.events, "CW-OPEN")]
	k1usn := m.events[eventIndex(t, m.events, "K1USN-SST")]

	m.selectEvent(cwOpen, cwOpen.Sessions[0])
	m.fields[fieldCall].SetValue("W1AW")
	m, _ = m.logCurrentQSO()
	if m.nextSerial != 2 {
		t.Fatalf("nextSerial after logging the first CW-OPEN QSO = %d, want 2", m.nextSerial)
	}

	recent, err := m.store.recentQSOs(m.activeStation.ID, 1)
	if err != nil || len(recent) != 1 {
		t.Fatalf("recentQSOs: %v (%d rows)", err, len(recent))
	}
	editID := recent[0].id

	m.beginEditQSO(qso{id: editID})
	// Simulate opening the Event Catalog mid-edit and picking a different
	// serial-based session, exactly like openContestOrCatalog/openEventCatalog
	// would land on from Contest Entry (F7) while a QSO is loaded for editing.
	m.selectEvent(k1usn, k1usn.Sessions[0])
	m.cancelEditQSO()

	if m.nextSerial != 2 {
		t.Fatalf("nextSerial after cancelling the edit = %d, want 2 (CW-OPEN's own running serial)", m.nextSerial)
	}
	if got := m.contestFields[contestSerialSent].Value(); got != formatSerial(2) {
		t.Fatalf("Sent Serial field after cancel = %q, want %q", got, formatSerial(2))
	}
}

// TestEditingOldQSODoesNotDupeAgainstTodaysContact reproduces the duplicate
// false-positive: logCurrentQSO's re-check used time.Now() as the dupe
// window's anchor even while editing an old record, so a same-call contact
// worked today made an unrelated edit to yesterday's contact (e.g. a notes
// correction) look like a dupe of today's QSO.
func TestEditingOldQSODoesNotDupeAgainstTodaysContact(t *testing.T) {
	m := reviewModel(t)

	yesterday := reviewQSO(m)
	yesterday.time = time.Now().UTC().Add(-24 * time.Hour)
	yesterday.timeOff = yesterday.time.Add(time.Minute)
	yesterday.comment = "original notes"
	yesterdayID := reviewInsert(t, m, yesterday)

	today := reviewQSO(m)
	today.time = time.Now().UTC()
	today.timeOff = today.time.Add(time.Minute)
	reviewInsert(t, m, today)

	m.beginEditQSO(qso{id: yesterdayID})
	m.detailFields[detailNotes].SetValue("corrected notes")
	m, _ = m.logCurrentQSO()

	if m.dupeWarning {
		t.Fatalf("editing yesterday's QSO was rejected as a dupe of today's contact: %s", m.statusMsg)
	}
	got, err := m.store.qsoByID(m.activeStation.ID, yesterdayID)
	if err != nil {
		t.Fatal(err)
	}
	if got.comment != "corrected notes" {
		t.Fatalf("comment = %q, want %q (edit was not saved)", got.comment, "corrected notes")
	}
}

// TestEditingOldQSOIgnoresStalePostModeTimestamp covers a variant of the
// same false-dupe bug: POST mode can be left on from before an edit began
// (Ctrl+P is only blocked from toggling while editing, not from being
// already on when an edit starts), leaving a stale, unrelated postTime in
// postFields. logCurrentQSO must anchor the dupe check on the edited
// record's own original time, not that leftover postTime, or the same
// false-positive dupe this fix targets reappears whenever POST mode happens
// to be on during an edit.
func TestEditingOldQSOIgnoresStalePostModeTimestamp(t *testing.T) {
	m := reviewModel(t)

	yesterday := reviewQSO(m)
	yesterday.time = time.Now().UTC().Add(-24 * time.Hour)
	yesterday.timeOff = yesterday.time.Add(time.Minute)
	yesterday.comment = "original notes"
	yesterdayID := reviewInsert(t, m, yesterday)

	today := reviewQSO(m)
	today.time = time.Now().UTC()
	today.timeOff = today.time.Add(time.Minute)
	reviewInsert(t, m, today)

	// Simulate POST mode having been left on (Ctrl+P) before the operator
	// opened this edit, with its Date/Time field still holding whatever was
	// last typed there — here, the current instant, which is exactly what
	// would make this look like a dupe of today's contact if it won.
	m.postMode = true
	m.postFields[postTimestamp].SetValue(time.Now().UTC().Format(postTimestampLayout))

	m.beginEditQSO(qso{id: yesterdayID})
	m.detailFields[detailNotes].SetValue("corrected notes")
	m, _ = m.logCurrentQSO()

	if m.dupeWarning {
		t.Fatalf("editing yesterday's QSO with stale POST mode on was rejected as a dupe: %s", m.statusMsg)
	}
	got, err := m.store.qsoByID(m.activeStation.ID, yesterdayID)
	if err != nil {
		t.Fatal(err)
	}
	if got.comment != "corrected notes" {
		t.Fatalf("comment = %q, want %q (edit was not saved)", got.comment, "corrected notes")
	}
}

// TestRecentClusterPOTAReferenceIgnoresIOTAToken reproduces the POTA
// autofill bug: potaReferencePattern's generic 1-2-letter-prefix/3+-digit
// shape also matches IOTA island-group references like "EU-005", so an
// IOTA-only spot comment got its reference copied into the POTA field too.
func TestRecentClusterPOTAReferenceIgnoresIOTAToken(t *testing.T) {
	spots := []clusterSpot{{
		Callsign: "W1AW",
		Comment:  "IOTA EU-005",
		Received: time.Now().UTC(),
	}}
	if ref, ok := recentClusterPOTAReference(spots, "W1AW", time.Now()); ok {
		t.Fatalf("recentClusterPOTAReference returned %q for an IOTA-only comment, want no match", ref)
	}
	if ref, ok := recentClusterIOTAReference(spots, "W1AW", time.Now()); !ok || ref != "EU-005" {
		t.Fatalf("recentClusterIOTAReference = %q, %v, want EU-005, true", ref, ok)
	}
}

// TestRecentClusterPOTAReferenceFindsRealRefPastIOTAToken guards the fix's
// own edge case: potaReferencePattern.FindString only ever returns the
// leftmost match, so a comment carrying both an IOTA token and a genuine
// POTA reference must not have the real reference missed just because the
// IOTA token happens to appear first in the text.
func TestRecentClusterPOTAReferenceFindsRealRefPastIOTAToken(t *testing.T) {
	spots := []clusterSpot{{
		Callsign: "W1AW",
		Comment:  "EU-005 IOTA also POTA K-1234",
		Received: time.Now().UTC(),
	}}
	if ref, ok := recentClusterPOTAReference(spots, "W1AW", time.Now()); !ok || ref != "K-1234" {
		t.Fatalf("recentClusterPOTAReference = %q, %v, want K-1234, true", ref, ok)
	}
}

// TestQSOEntryReservesAnalysisPanelSpace reproduces the layout bug: the DX
// Spots panel was sized against all width left over after Recent QSOs,
// before the analysis panel got a chance to claim its own share. A long spot
// comment could grow the DX Spots line enough to consume every remaining
// column, silently dropping the analysis panel on an unchanged terminal
// size.
func TestQSOEntryReservesAnalysisPanelSpace(t *testing.T) {
	m := reviewModel(t)
	m.screen = qsoEntryScreen
	m.termWidth = 240
	m.termHeight = 40
	m.fields[fieldCall].SetValue("W1AW")
	m.clusterSpots = []clusterSpot{{
		Callsign:  "W1AW",
		Frequency: "14025.0",
		Comment:   "this is an unusually long spot comment that used to consume all the remaining terminal width and then some more padding here to be sure",
		Received:  time.Now().UTC(),
	}}
	view := m.View()
	if view == "" {
		t.Fatal("qsoEntryView returned empty output")
	}
	if !strings.Contains(view, "Analysis") {
		t.Fatalf("analysis panel missing from QSO Entry view with a long DX spot comment on a %d-column terminal", m.termWidth)
	}
}
