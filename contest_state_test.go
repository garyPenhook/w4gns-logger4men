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

	state, err := buildContestState(context.Background(), profile.ID, profile.Callsign, "CW-OPEN-1", st)
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

// TestContestStateCheckPartial exercises the Check Partial candidate list
// (roadmap Appendix B.3): a substring fragment should surface every other
// logged call containing it, excluding an exact match to the fragment
// itself, sorted, and capped at the caller's limit.
func TestContestStateCheckPartial(t *testing.T) {
	state := newContestState()
	for _, call := range []string{"W1AW", "K1AW", "N1AWX", "W4GNS"} {
		state.record(qso{call: call, band: "20M"})
	}

	if got := state.checkPartial("", 10); got != nil {
		t.Fatalf("checkPartial(\"\") = %v, want nil", got)
	}
	if got := state.checkPartial("ZZZZZ", 10); got != nil {
		t.Fatalf("checkPartial with no matches = %v, want nil", got)
	}

	got := state.checkPartial("1AW", 10)
	want := []string{"K1AW", "N1AWX", "W1AW"}
	if len(got) != len(want) {
		t.Fatalf("checkPartial(1AW) = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("checkPartial(1AW) = %v, want %v", got, want)
		}
	}

	// An exact match to the fragment doesn't list itself.
	if got := state.checkPartial("W1AW", 10); len(got) != 0 {
		t.Fatalf("checkPartial(W1AW) = %v, want no self-match", got)
	}

	if got := state.checkPartial("1AW", 2); len(got) != 2 {
		t.Fatalf("checkPartial(1AW) with limit 2 = %v, want 2 entries", got)
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

	before, err := buildContestState(context.Background(), profile.ID, profile.Callsign, "CW-OPEN-1", st)
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

	after, err := buildContestState(context.Background(), profile.ID, profile.Callsign, "CW-OPEN-1", st)
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

// TestContestStateContinentSummary exercises the Worked/Needed by Continent
// index (roadmap Appendix B.9): a call resolves to its DXCC entity's
// continent, tallied per band, and an un-worked continent/band combination
// reports needed with a zero count.
func TestContestStateContinentSummary(t *testing.T) {
	state := newContestState()
	state.record(qso{call: "W1AW", band: "20M"})   // USA — NA
	state.record(qso{call: "DL1ABC", band: "20M"}) // Germany — EU
	state.record(qso{call: "W2XYZ", band: "20M"})  // USA — NA, same continent/band again

	if worked, count := state.continentSummary("NA", "20M"); !worked || count != 2 {
		t.Fatalf("NA/20M summary = worked=%v count=%d, want worked=true count=2", worked, count)
	}
	if worked, count := state.continentSummary("EU", "20M"); !worked || count != 1 {
		t.Fatalf("EU/20M summary = worked=%v count=%d, want worked=true count=1", worked, count)
	}
	if worked, count := state.continentSummary("NA", "40M"); worked || count != 0 {
		t.Fatalf("NA/40M summary = worked=%v count=%d, want worked=false count=0 (not logged on that band)", worked, count)
	}
	if worked, count := state.continentSummary("OC", "20M"); worked || count != 0 {
		t.Fatalf("OC/20M summary = worked=%v count=%d, want worked=false count=0 (never worked)", worked, count)
	}
}

// TestContestStateScoreSumsDXCCAndZoneMultipliersPerBand exercises the
// data-driven multiplier schema (roadmap Appendix C, e.g. CQ WW's countries
// + zones): each rule's per-band count is summed into the total multiplier
// count, and a callsign worked again on a different band counts again for
// each rule (SD's MULTSCOUNT=Band), matching what the real contest awards.
func TestContestStateScoreSumsDXCCAndZoneMultipliersPerBand(t *testing.T) {
	state := newContestState()
	state.record(qso{call: "W1AW", band: "20M"})   // USA, CQ zone 5
	state.record(qso{call: "DL1ABC", band: "20M"}) // Germany, CQ zone 14
	state.record(qso{call: "W2XYZ", band: "40M"})  // USA again, but a new band

	rules := &scoringRules{
		PointsPerQSO: 1,
		Multipliers: []multiplierRule{
			{Kind: "dxcc", Per: "band"},
			{Kind: "cqzone", Per: "band"},
		},
	}
	score := state.score(rules)
	// qsoPoints: 3 unique (call, band) pairs x 1 point.
	if score.qsoPoints != 3 {
		t.Fatalf("qsoPoints = %d, want 3", score.qsoPoints)
	}
	// dxcc mults: 20M has USA+Germany (2), 40M has USA (1) = 3.
	// cqzone mults: 20M has zone 5+14 (2), 40M has zone 5 (1) = 3.
	if score.multipliers != 6 {
		t.Fatalf("multipliers = %d, want 6 (3 dxcc + 3 cqzone)", score.multipliers)
	}
	if score.total() != 18 {
		t.Fatalf("total() = %d, want 18", score.total())
	}
}

// TestContestStateScorePerContestMultiplierCountsOnce mirrors the previous
// test's log but with Per:"contest" rules, confirming the same DXCC entity
// or zone worked on multiple bands only counts once toward the multiplier
// total (SD's MULTSCOUNT=Once).
func TestContestStateScorePerContestMultiplierCountsOnce(t *testing.T) {
	state := newContestState()
	state.record(qso{call: "W1AW", band: "20M"})
	state.record(qso{call: "DL1ABC", band: "20M"})
	state.record(qso{call: "W2XYZ", band: "40M"})

	rules := &scoringRules{
		PointsPerQSO: 1,
		Multipliers: []multiplierRule{
			{Kind: "dxcc", Per: "contest"},
			{Kind: "cqzone", Per: "contest"},
		},
	}
	score := state.score(rules)
	// dxcc: USA + Germany = 2 (regardless of band). cqzone: 5 + 14 = 2.
	if score.multipliers != 4 {
		t.Fatalf("multipliers = %d, want 4 (2 dxcc + 2 cqzone, deduped across bands)", score.multipliers)
	}
}

// TestContestStateUnscoredQSOExcludedFromMultiplierCount extends the /X
// (unscored) invariant already covered for unique_call scoring to the
// dxcc/cqzone/ituzone multiplier kinds: an unscored QSO still happened (it
// would show as worked in Check Partial/continent), but must not contribute
// a multiplier toward CLAIMED-SCORE.
func TestContestStateUnscoredQSOExcludedFromMultiplierCount(t *testing.T) {
	state := newContestState()
	state.record(qso{call: "W1AW", band: "20M", unscored: true})

	rules := &scoringRules{
		PointsPerQSO: 1,
		Multipliers:  []multiplierRule{{Kind: "dxcc", Per: "band"}},
	}
	score := state.score(rules)
	if score.qsoPoints != 0 || score.multipliers != 0 {
		t.Fatalf("score of a single unscored QSO = %d pts x %d mults, want 0 x 0", score.qsoPoints, score.multipliers)
	}
}

// TestContestStateScorePointsRuleTiersByCountryAndContinent exercises the
// continent/country-tiered points shape (roadmap §3 Phase 3, e.g. CQ WW):
// a QSO with the operator's own country scores SameCountry, one with a
// different country on the operator's own continent scores SameContinent,
// and one on another continent scores OtherContinent.
func TestContestStateScorePointsRuleTiersByCountryAndContinent(t *testing.T) {
	state := newContestState()
	state.setStation("W1AW")                       // USA, NA
	state.record(qso{call: "W2XYZ", band: "20M"})  // USA - same country
	state.record(qso{call: "VE3ABC", band: "20M"}) // Canada - same continent
	state.record(qso{call: "JA1ABC", band: "20M"}) // Japan - other continent

	rules := &scoringRules{
		Points: &pointsRule{SameCountry: 0, SameContinent: 1, OtherContinent: 3},
	}
	score := state.score(rules)
	if score.qsoPoints != 4 {
		t.Fatalf("qsoPoints = %d, want 4 (0 + 1 + 3)", score.qsoPoints)
	}
}

// TestContestStateScorePointsRulePerContinentOverride exercises CQ WW's own
// exception to its 1-point same-continent tier: a same-continent,
// different-country QSO within North America is worth 2 points under
// SameContinentOverrides, while a same-continent QSO on any other continent
// still uses the flat SameContinent value.
func TestContestStateScorePointsRulePerContinentOverride(t *testing.T) {
	state := newContestState()
	state.setStation("W1AW")                       // USA, NA
	state.record(qso{call: "VE3ABC", band: "20M"}) // Canada - NA override applies

	otherState := newContestState()
	otherState.setStation("JA1ABC")                     // Japan, AS
	otherState.record(qso{call: "HL1ABC", band: "20M"}) // South Korea - same continent, no override configured

	rules := &scoringRules{
		Points: &pointsRule{
			SameCountry:            0,
			SameContinent:          1,
			OtherContinent:         3,
			SameContinentOverrides: map[string]int{"NA": 2},
		},
	}
	if score := state.score(rules); score.qsoPoints != 2 {
		t.Fatalf("NA qsoPoints = %d, want 2 (override applied)", score.qsoPoints)
	}
	if score := otherState.score(rules); score.qsoPoints != 1 {
		t.Fatalf("AS qsoPoints = %d, want 1 (flat SameContinent, no override for AS)", score.qsoPoints)
	}
}

// TestContestStateScorePointsRuleUnresolvedStationScoresZero confirms a
// pointsRule can't guess a tier when the operator's own callsign never
// resolved (setStation never called, or an unresolvable callsign) — every
// QSO scores 0 rather than silently defaulting to same-country.
func TestContestStateScorePointsRuleUnresolvedStationScoresZero(t *testing.T) {
	state := newContestState()
	state.record(qso{call: "JA1ABC", band: "20M"})

	rules := &scoringRules{Points: &pointsRule{SameCountry: 0, SameContinent: 1, OtherContinent: 3}}
	score := state.score(rules)
	if score.qsoPoints != 0 {
		t.Fatalf("qsoPoints = %d, want 0 (station never resolved)", score.qsoPoints)
	}
}

// TestContestStateScorePointsRuleTakesPrecedenceOverPointsPerQSO confirms
// Points, when set, replaces PointsPerQSO entirely rather than adding to it
// (mirrors Multipliers-over-Multiplier precedence in effectiveMultipliers).
func TestContestStateScorePointsRuleTakesPrecedenceOverPointsPerQSO(t *testing.T) {
	state := newContestState()
	state.setStation("W1AW")
	state.record(qso{call: "W2XYZ", band: "20M"}) // same country -> 0 under Points

	rules := &scoringRules{
		PointsPerQSO: 10,
		Points:       &pointsRule{SameCountry: 0, SameContinent: 1, OtherContinent: 3},
	}
	score := state.score(rules)
	if score.qsoPoints != 0 {
		t.Fatalf("qsoPoints = %d, want 0 (Points should override PointsPerQSO)", score.qsoPoints)
	}
}

// TestContestStateWouldBeNewMultiplier exercises the advance multiplier flag
// (roadmap Appendix B.5) for a rule set combining unique_call-independent
// dxcc/cqzone kinds: a callsign from an already-worked country on the same
// band isn't a new mult even though the callsign itself is new, and a
// callsign from a new country is.
func TestContestStateWouldBeNewMultiplier(t *testing.T) {
	state := newContestState()
	state.record(qso{call: "W1AW", band: "20M"}) // USA, CQ zone 5

	table, err := sharedDXCCTable()
	if err != nil {
		t.Fatalf("sharedDXCCTable: %v", err)
	}
	rules := &scoringRules{
		Multipliers: []multiplierRule{{Kind: "dxcc", Per: "band"}},
	}

	// Another USA call on the same band: not a new DXCC mult.
	usEntity, found := table.lookup("W2XYZ")
	if !found {
		t.Fatal("expected W2XYZ to resolve to a DXCC entity")
	}
	if newMult, workedBefore := state.wouldBeNewMultiplier(rules, "W2XYZ", "20M", usEntity, true); newMult || !workedBefore {
		t.Fatalf("W2XYZ/20M (same country, same band) = newMult=%v workedBefore=%v, want false/true", newMult, workedBefore)
	}

	// A German call on the same band: new DXCC mult.
	dlEntity, found := table.lookup("DL1ABC")
	if !found {
		t.Fatal("expected DL1ABC to resolve to a DXCC entity")
	}
	if newMult, workedBefore := state.wouldBeNewMultiplier(rules, "DL1ABC", "20M", dlEntity, true); !newMult || workedBefore {
		t.Fatalf("DL1ABC/20M (new country) = newMult=%v workedBefore=%v, want true/false", newMult, workedBefore)
	}

	// Same USA country, but a fresh band: new DXCC mult again (Per:"band").
	if newMult, workedBefore := state.wouldBeNewMultiplier(rules, "W2XYZ", "40M", usEntity, true); !newMult || workedBefore {
		t.Fatalf("W2XYZ/40M (same country, new band) = newMult=%v workedBefore=%v, want true/false", newMult, workedBefore)
	}

	// Unresolved prefix: no dxcc/cqzone/ituzone rule can fire.
	if newMult, workedBefore := state.wouldBeNewMultiplier(rules, "ZZ1XYZ", "20M", dxccEntity{}, false); newMult || workedBefore {
		t.Fatalf("unresolved prefix = newMult=%v workedBefore=%v, want false/false", newMult, workedBefore)
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
