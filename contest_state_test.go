package main

import (
	"context"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// TestContestStateIsWorkedOnBand exercises the dupe/worked-band test the
// index exposes for future analysis panels (roadmap Appendix B/C): a call
// logged on one band is "worked" only on that band, not others, and an
// unlogged call is never worked.
func TestContestStateIsWorkedOnBand(t *testing.T) {
	st, err := openStore(t.TempDir() + "/logger.db")
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	profile, err := st.activeStationProfile()
	if err != nil {
		t.Fatal(err)
	}

	mk := func(call, band string) qso {
		q := validTestQSO()
		q.call, q.band, q.contestID, q.profileID = call, band, "CW-OPEN-1", profile.ID
		return q
	}
	for _, q := range []qso{mk("W1AW", "20M"), mk("K1ABC", "40M")} {
		if _, err := st.insertQSO(q); err != nil {
			t.Fatal(err)
		}
	}

	state, err := buildContestState(context.Background(), profile.ID, "CW-OPEN-1", st)
	if err != nil {
		t.Fatalf("buildContestState: %v", err)
	}
	cases := []struct {
		call, band string
		want       bool
	}{
		{"W1AW", "20M", true},
		{"w1aw", "20m", true}, // case-insensitive, matching store.isDupe
		{"W1AW", "40M", false},
		{"K1ABC", "40M", true},
		{"N0CALL", "20M", false},
	}
	for _, c := range cases {
		if got := state.isWorkedOnBand(c.call, c.band); got != c.want {
			t.Errorf("isWorkedOnBand(%q, %q) = %v, want %v", c.call, c.band, got, c.want)
		}
	}
}

// TestContestStateRecomputesAfterEdit is Appendix E's hardest case: editing a
// QSO's call/band must be reflected by a fresh buildContestState call, since
// the roadmap's "correct any QSO, recompute the whole log instantly"
// invariant requires the index (and everything reading it, including score)
// to never serve stale data from before the edit.
func TestContestStateRecomputesAfterEdit(t *testing.T) {
	st, err := openStore(t.TempDir() + "/logger.db")
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	profile, err := st.activeStationProfile()
	if err != nil {
		t.Fatal(err)
	}

	q := validTestQSO()
	q.call, q.band, q.contestID, q.profileID = "W1AW", "20M", "CW-OPEN-1", profile.ID
	id, err := st.insertQSO(q)
	if err != nil {
		t.Fatal(err)
	}

	before, err := buildContestState(context.Background(), profile.ID, "CW-OPEN-1", st)
	if err != nil {
		t.Fatalf("buildContestState (before edit): %v", err)
	}
	if !before.isWorkedOnBand("W1AW", "20M") {
		t.Fatal("expected W1AW worked on 20M before edit")
	}
	if before.isWorkedOnBand("K1ABC", "40M") {
		t.Fatal("K1ABC/40M should not be worked before edit")
	}

	edited := q
	edited.call, edited.band = "K1ABC", "40M"
	if err := st.updateQSO(id, edited); err != nil {
		t.Fatalf("updateQSO: %v", err)
	}

	after, err := buildContestState(context.Background(), profile.ID, "CW-OPEN-1", st)
	if err != nil {
		t.Fatalf("buildContestState (after edit): %v", err)
	}
	if after.isWorkedOnBand("W1AW", "20M") {
		t.Fatal("W1AW/20M should no longer be worked after the edit moved it to K1ABC/40M")
	}
	if !after.isWorkedOnBand("K1ABC", "40M") {
		t.Fatal("expected K1ABC worked on 40M after edit")
	}

	score := after.score(cwOpenScoringEvent().Scoring)
	if score.qsoPoints != 1 || score.multipliers != 1 {
		t.Fatalf("score after edit = %d pts x %d mults, want 1 x 1", score.qsoPoints, score.multipliers)
	}
}

// TestContestStateScoreNilRulesIsZero guards score() against a nil
// scoringRules pointer, the same "no scoring configured" case
// computeContestScore short-circuits on.
func TestContestStateScoreNilRulesIsZero(t *testing.T) {
	state := newContestState()
	state.record(qso{call: "W1AW", band: "20M"})
	if got := state.score(nil).total(); got != 0 {
		t.Fatalf("score(nil).total() = %d, want 0", got)
	}
}

// TestContestIndexBuildsOnSelectAndUpdatesIncrementallyOnLog exercises the
// live model.contestIndex end to end: selecting a contest builds an (empty)
// index, and logging a QSO updates it incrementally, without a fresh
// buildContestState DB round-trip, in time to back an as-you-type Analysis
// panel.
func TestContestIndexBuildsOnSelectAndUpdatesIncrementallyOnLog(t *testing.T) {
	st, err := openStore(t.TempDir() + "/logger.db")
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	m := initialModel(st)
	cwopen := m.events[eventIndex(t, m.events, "CW-OPEN")]
	m.selectEvent(cwopen, cwopen.Sessions[0])
	contestID := m.contestFields[contestName].Value()

	if m.contestIndex == nil {
		t.Fatal("contestIndex is nil after selectEvent, want a built (empty) index")
	}
	if m.contestIndexID != contestID {
		t.Fatalf("contestIndexID = %q, want %q", m.contestIndexID, contestID)
	}
	if len(m.contestIndex.uniqueCalls) != 0 {
		t.Fatalf("uniqueCalls = %v, want empty before any QSO is logged", m.contestIndex.uniqueCalls)
	}

	m.fields[fieldCall].SetValue("W1AW")
	m.fields[fieldBand].SetValue("20M")
	m, _ = m.logCurrentQSO()
	if !strings.Contains(m.statusMsg, "logged") {
		t.Fatalf("QSO not logged: %q", m.statusMsg)
	}
	if !m.contestIndex.isWorkedOnBand("W1AW", "20M") {
		t.Fatal("contestIndex was not updated incrementally after logCurrentQSO")
	}
	if len(m.contestIndex.byCall["W1AW"]) != 1 {
		t.Fatalf("byCall[W1AW] = %d entries, want 1", len(m.contestIndex.byCall["W1AW"]))
	}
}

// TestContestIndexFullRecomputeOnEditWithinSameContest is Appendix E's
// hardest case applied to the live model: editing a QSO's band while the
// contest it belongs to stays active the whole time doesn't change
// contestIndexID, so checkDupe's lazy diff-check alone wouldn't rebuild —
// the edit-save path must force a full rebuildContestIndex regardless.
func TestContestIndexFullRecomputeOnEditWithinSameContest(t *testing.T) {
	st, err := openStore(t.TempDir() + "/logger.db")
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	m := initialModel(st)
	cwopen := m.events[eventIndex(t, m.events, "CW-OPEN")]
	m.selectEvent(cwopen, cwopen.Sessions[0])
	m.screen = qsoEntryScreen

	m.fields[fieldCall].SetValue("W1AW")
	m.fields[fieldBand].SetValue("20M")
	m, _ = m.logCurrentQSO()
	if !m.contestIndex.isWorkedOnBand("W1AW", "20M") {
		t.Fatal("W1AW/20M should be worked right after logging")
	}

	m.beginEditQSO(m.recentQSOs[0])
	m.fields[fieldBand].SetValue("40M")
	m.fields[fieldFrequency].SetValue("7.025")
	m, _ = m.logCurrentQSO()
	if !strings.Contains(m.statusMsg, "updated") {
		t.Fatalf("edit not saved: %q", m.statusMsg)
	}

	if m.contestIndex.isWorkedOnBand("W1AW", "20M") {
		t.Fatal("contestIndex still shows W1AW/20M worked after the edit moved it to 40M — stale index")
	}
	if !m.contestIndex.isWorkedOnBand("W1AW", "40M") {
		t.Fatal("contestIndex does not reflect the edited band 40M")
	}
}

// TestContestIndexRecomputesOnDelete confirms the delete path (Appendix C's
// "full recompute on edit/delete") also keeps the live index in sync.
func TestContestIndexRecomputesOnDelete(t *testing.T) {
	st, err := openStore(t.TempDir() + "/logger.db")
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	m := initialModel(st)
	cwopen := m.events[eventIndex(t, m.events, "CW-OPEN")]
	m.selectEvent(cwopen, cwopen.Sessions[0])
	m.screen = qsoEntryScreen
	m.fields[fieldCall].SetValue("W1AW")
	m.fields[fieldBand].SetValue("20M")
	m, _ = m.logCurrentQSO()
	if !m.contestIndex.isWorkedOnBand("W1AW", "20M") {
		t.Fatal("W1AW/20M should be worked right after logging")
	}

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyF9})
	m = updated.(model)
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("d")})
	m = updated.(model)
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("d")})
	m = updated.(model)

	if m.contestIndex.isWorkedOnBand("W1AW", "20M") {
		t.Fatal("contestIndex still shows W1AW/20M worked after delete — stale index")
	}
}

// TestContestIndexSwapsOnEditFromDifferentContestAndRestoresOnCancel checks
// the beginEditQSO/cancelEditQSO sync points: editing a QSO logged under a
// different (or no) contest than the one currently active must show that
// QSO's own contest analysis while editing, then restore the real active
// contest's index on cancel — mirroring
// TestEditQSOFromDifferentContestRestoresActiveContestOnSave's save-path
// coverage for the cancel path instead.
func TestContestIndexSwapsOnEditFromDifferentContestAndRestoresOnCancel(t *testing.T) {
	st, err := openStore(t.TempDir() + "/logger.db")
	if err != nil {
		t.Fatal(err)
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

	cwt := m.events[eventIndex(t, m.events, "CWT")]
	m.selectEvent(cwt, cwt.Sessions[0])
	activeContest := m.contestFields[contestName].Value()
	if m.contestIndexID != activeContest {
		t.Fatalf("contestIndexID = %q after selectEvent, want the active contest %q", m.contestIndexID, activeContest)
	}

	m.beginEditQSO(original)
	if m.contestIndexID != "" {
		t.Fatalf("contestIndexID while editing a no-contest QSO = %q, want blank (the edited QSO's own contest)", m.contestIndexID)
	}

	m.cancelEditQSO()
	if m.contestIndexID != activeContest {
		t.Fatalf("contestIndexID after cancelling the edit = %q, want the active contest %q restored", m.contestIndexID, activeContest)
	}
}
