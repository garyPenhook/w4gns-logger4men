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

// TestContestStateUsesPersistedMultiplierContext makes the scoring-context
// invariant explicit: an imported or operator-corrected DXCC/zone value is a
// historical fact recorded with the QSO, not a value to silently replace from
// a newer cty.dat prefix lookup.
func TestContestStateUsesPersistedMultiplierContext(t *testing.T) {
	state := newContestState()
	state.record(qso{
		call:       "W1AW", // Current cty.dat lookup is USA/CQ 5/ITU 8.
		band:       "20M",
		dxccNumber: "999",
		cqZone:     "88",
		ituZone:    "77",
	})

	rules := &scoringRules{
		PointsPerQSO: 1,
		Multipliers: []multiplierRule{
			{Kind: "dxcc", Per: "contest"},
			{Kind: "cqzone", Per: "contest"},
			{Kind: "ituzone", Per: "contest"},
		},
	}
	if score := state.score(rules); score.qsoPoints != 1 || score.multipliers != 3 {
		t.Fatalf("score with persisted DXCC/zone context = %d pts x %d mults, want 1 x 3", score.qsoPoints, score.multipliers)
	}
	if _, ok := state.dxccAll[999]; !ok {
		t.Fatalf("DXCC set = %v, want persisted value 999", state.dxccAll)
	}
	if _, ok := state.cqZoneAll[88]; !ok {
		t.Fatalf("CQ-zone set = %v, want persisted value 88", state.cqZoneAll)
	}
	if _, ok := state.ituZoneAll[77]; !ok {
		t.Fatalf("ITU-zone set = %v, want persisted value 77", state.ituZoneAll)
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

// TestContestStateScorePrefixMultiplierCountsOncePerContest exercises the CQ
// WPX-style "prefix" multiplier kind: the same WPX prefix worked on two
// different bands still counts once (Per: "contest", Rule V.C — "Each PREFIX
// is counted only once regardless of the band"), while a genuinely distinct
// prefix adds another.
func TestContestStateScorePrefixMultiplierCountsOncePerContest(t *testing.T) {
	state := newContestState()
	state.record(qso{call: "W1AW", band: "20M"})  // prefix W1
	state.record(qso{call: "W1XYZ", band: "40M"}) // prefix W1 again, different band
	state.record(qso{call: "K5ABC", band: "20M"}) // prefix K5, new

	rules := &scoringRules{
		PointsPerQSO: 1,
		Multipliers:  []multiplierRule{{Kind: "prefix", Per: "contest"}},
	}
	score := state.score(rules)
	if score.multipliers != 2 {
		t.Fatalf("multipliers = %d, want 2 (W1 once + K5)", score.multipliers)
	}
}

// TestContestStateScoreWAECountryMultiplierBandWeighted exercises the WAE DX
// Contest's non-European-entrant multiplier (Section 6): distinct WAE
// Country List entities worked per band, weighted by Section 6's band bonus
// (4x 80M, 3x 40M, 2x 20/15/10M) via the "band_weighted" Per scope. A second
// station from an already-counted WAE country on the same band doesn't add
// another; a non-WAE country (United States) contributes nothing to this
// side's multiplier.
func TestContestStateScoreWAECountryMultiplierBandWeighted(t *testing.T) {
	state := newContestState()
	state.record(qso{call: "DL1ABC", band: "80M"}) // Germany, new on 80M
	state.record(qso{call: "DL2XYZ", band: "80M"}) // Germany again, same band
	state.record(qso{call: "ON4ABC", band: "80M"}) // Belgium, new on 80M
	state.record(qso{call: "G3ABC", band: "40M"})  // England, new on 40M
	state.record(qso{call: "W1AW", band: "20M"})   // United States, not in WAE list

	rules := &scoringRules{
		PointsPerQSO: 1,
		Multipliers:  []multiplierRule{{Kind: "wae_country", Per: "band_weighted"}},
	}
	score := state.score(rules)
	want := 2*4 + 1*3 // 80M: Germany+Belgium x4, 40M: England x3
	if score.multipliers != want {
		t.Fatalf("multipliers = %d, want %d", score.multipliers, want)
	}
}

// TestContestStateScoreDXCCNonWAEMultiplierBandWeighted exercises the WAE DX
// Contest's European-entrant multiplier (Section 6): distinct non-European
// DXCC entities worked per band, weighted by the same band bonus. A WAE-list
// entity (Germany, Belgium, England) never contributes to this multiplier
// even though it was logged, since Section 5 restricts a European entrant to
// working only non-European stations.
func TestContestStateScoreDXCCNonWAEMultiplierBandWeighted(t *testing.T) {
	state := newContestState()
	state.record(qso{call: "DL1ABC", band: "80M"}) // Germany, WAE-list, excluded
	state.record(qso{call: "ON4ABC", band: "80M"}) // Belgium, WAE-list, excluded
	state.record(qso{call: "G3ABC", band: "40M"})  // England, WAE-list, excluded
	state.record(qso{call: "W1AW", band: "20M"})   // United States, new non-WAE on 20M
	state.record(qso{call: "K5ABC", band: "20M"})  // United States again, same band

	rules := &scoringRules{
		PointsPerQSO: 1,
		Multipliers:  []multiplierRule{{Kind: "dxcc_non_wae", Per: "band_weighted"}},
	}
	score := state.score(rules)
	want := 1 * 2 // 20M: United States x2, WAE-list contacts excluded
	if score.multipliers != want {
		t.Fatalf("multipliers = %d, want %d", score.multipliers, want)
	}
}

// TestContestStateWouldBeNewMultiplierWAECountry exercises the as-you-type
// "NEW MULT" flag for the wae_country kind: a repeated WAE-list country on
// the same band is workedBefore, a new one is newMult, and a non-WAE-list
// entity (United States) is neither, since it can never contribute to this
// side's multiplier.
func TestContestStateWouldBeNewMultiplierWAECountry(t *testing.T) {
	state := newContestState()
	state.record(qso{call: "DL1ABC", band: "20M"})

	rules := &scoringRules{
		Multipliers: []multiplierRule{{Kind: "wae_country", Per: "band_weighted"}},
	}
	germany := dxccEntity{Country: "Fed. Rep. of Germany"}
	if newMult, workedBefore := state.wouldBeNewMultiplier(rules, "DL2XYZ", "20M", "", germany, true); newMult || !workedBefore {
		t.Fatalf("Germany (already worked on 20M) = newMult=%v workedBefore=%v, want false/true", newMult, workedBefore)
	}
	if newMult, workedBefore := state.wouldBeNewMultiplier(rules, "ON4ABC", "20M", "", dxccEntity{Country: "Belgium"}, true); !newMult || workedBefore {
		t.Fatalf("Belgium (new WAE country) = newMult=%v workedBefore=%v, want true/false", newMult, workedBefore)
	}
	usa := dxccEntity{Country: "United States"}
	if newMult, workedBefore := state.wouldBeNewMultiplier(rules, "W1AW", "20M", "", usa, true); newMult || workedBefore {
		t.Fatalf("United States (not WAE) = newMult=%v workedBefore=%v, want false/false", newMult, workedBefore)
	}
}

// TestContestStateWouldBeNewMultiplierDXCCNonWAE exercises the as-you-type
// "NEW MULT" flag for the dxcc_non_wae kind: a repeated non-WAE entity on
// the same band is workedBefore, a new one is newMult, and a WAE-list entity
// (Germany) is neither, matching a European entrant's own multiplier scope.
func TestContestStateWouldBeNewMultiplierDXCCNonWAE(t *testing.T) {
	state := newContestState()
	state.record(qso{call: "W1AW", band: "20M"})

	rules := &scoringRules{
		Multipliers: []multiplierRule{{Kind: "dxcc_non_wae", Per: "band_weighted"}},
	}
	usa := dxccEntity{Country: "United States", DXCCNumber: 291}
	if newMult, workedBefore := state.wouldBeNewMultiplier(rules, "K5ABC", "20M", "", usa, true); newMult || !workedBefore {
		t.Fatalf("United States (already worked on 20M) = newMult=%v workedBefore=%v, want false/true", newMult, workedBefore)
	}
	japan := dxccEntity{Country: "Japan", DXCCNumber: 339}
	if newMult, workedBefore := state.wouldBeNewMultiplier(rules, "JA1ABC", "20M", "", japan, true); !newMult || workedBefore {
		t.Fatalf("Japan (new non-WAE entity) = newMult=%v workedBefore=%v, want true/false", newMult, workedBefore)
	}
	germany := dxccEntity{Country: "Fed. Rep. of Germany", DXCCNumber: 230}
	if newMult, workedBefore := state.wouldBeNewMultiplier(rules, "DL1ABC", "20M", "", germany, true); newMult || workedBefore {
		t.Fatalf("Germany (WAE-list) = newMult=%v workedBefore=%v, want false/false", newMult, workedBefore)
	}
}

// TestContestStateScoreExchangeAreaMultiplierFromReceivedExchange exercises
// CQ 160 Meter CW's "exchange_area" multiplier: a US state/DC/Canadian
// province parsed from the worked station's received exchange text (not
// resolvable from its callsign), counted once per contest alongside the
// DXCC-entity multiplier. A repeated area doesn't add another; unrecognized
// exchange text (a DX station's power report) contributes nothing.
func TestContestStateScoreExchangeAreaMultiplierFromReceivedExchange(t *testing.T) {
	state := newContestState()
	state.record(qso{call: "W1AW", band: "160M", srxString: "CT"})   // new area: CT
	state.record(qso{call: "K5ABC", band: "160M", srxString: "ct"})  // same area again, case-insensitive
	state.record(qso{call: "VE3ABC", band: "160M", srxString: "ON"}) // new area: ON
	state.record(qso{call: "DL1ABC", band: "160M", srxString: "5NN"})

	rules := &scoringRules{
		PointsPerQSO: 1,
		Multipliers:  []multiplierRule{{Kind: "exchange_area", Per: "contest"}},
	}
	score := state.score(rules)
	if score.multipliers != 2 {
		t.Fatalf("multipliers = %d, want 2 (CT + ON)", score.multipliers)
	}
}

// TestContestStateWouldBeNewMultiplierExchangeArea exercises the as-you-type
// "NEW MULT" flag for the exchange_area kind, which — unlike dxcc/cqzone/
// ituzone — has no callsign-derived fallback and reads only the operator's
// in-progress received-exchange field text.
func TestContestStateWouldBeNewMultiplierExchangeArea(t *testing.T) {
	state := newContestState()
	state.record(qso{call: "W1AW", band: "160M", srxString: "CT"})

	rules := &scoringRules{
		Multipliers: []multiplierRule{{Kind: "exchange_area", Per: "contest"}},
	}
	// Same area already worked: not a new mult.
	if newMult, workedBefore := state.wouldBeNewMultiplier(rules, "W1XYZ", "160M", "CT", dxccEntity{}, false); newMult || !workedBefore {
		t.Fatalf("CT (already worked) = newMult=%v workedBefore=%v, want false/true", newMult, workedBefore)
	}
	// A different area: new mult.
	if newMult, workedBefore := state.wouldBeNewMultiplier(rules, "K5ABC", "160M", "TX", dxccEntity{}, false); !newMult || workedBefore {
		t.Fatalf("TX (new area) = newMult=%v workedBefore=%v, want true/false", newMult, workedBefore)
	}
	// Not yet a recognizable area (still typing, or a DX power exchange):
	// no flag either way.
	if newMult, workedBefore := state.wouldBeNewMultiplier(rules, "DL1ABC", "160M", "5NN", dxccEntity{}, false); newMult || workedBefore {
		t.Fatalf("unrecognized exchange text = newMult=%v workedBefore=%v, want false/false", newMult, workedBefore)
	}
}

// TestContestStateScoreTNCountyMultiplierFromReceivedExchange exercises
// TNQP's "tn_county" multiplier: a Tennessee county parsed from the worked
// station's received exchange text, counted per band (tnqp.org/rules: "95
// maximum per band"). A repeated county on the same band doesn't add
// another; the same county on a different band does; unrecognized exchange
// text (a state/province abbreviation, from an out-of-state station's own
// sent exchange bleeding into the field) contributes nothing.
func TestContestStateScoreTNCountyMultiplierFromReceivedExchange(t *testing.T) {
	state := newContestState()
	state.record(qso{call: "K4TCG", band: "20M", srxString: "SHEL"}) // new: SHEL/20M
	state.record(qso{call: "W4ABC", band: "20M", srxString: "shel"}) // same county+band, case-insensitive
	state.record(qso{call: "W4DEF", band: "40M", srxString: "SHEL"}) // same county, new band
	state.record(qso{call: "K4XYZ", band: "20M", srxString: "DAVI"}) // new: DAVI/20M
	state.record(qso{call: "N4GHI", band: "20M", srxString: "TN"})   // not a county code

	rules := &scoringRules{
		PointsPerQSO: 3,
		Multipliers:  []multiplierRule{{Kind: "tn_county", Per: "band"}},
	}
	score := state.score(rules)
	if score.multipliers != 3 {
		t.Fatalf("multipliers = %d, want 3 (SHEL/20M + SHEL/40M + DAVI/20M)", score.multipliers)
	}
}

// TestContestStateWouldBeNewMultiplierTNCounty exercises the as-you-type
// "NEW MULT" flag for the tn_county kind, which — like exchange_area — has
// no callsign-derived fallback and reads only the operator's in-progress
// received-exchange field text.
func TestContestStateWouldBeNewMultiplierTNCounty(t *testing.T) {
	state := newContestState()
	state.record(qso{call: "K4TCG", band: "20M", srxString: "SHEL"})

	rules := &scoringRules{
		Multipliers: []multiplierRule{{Kind: "tn_county", Per: "band"}},
	}
	// Same county, same band, already worked: not a new mult.
	if newMult, workedBefore := state.wouldBeNewMultiplier(rules, "W4ABC", "20M", "SHEL", dxccEntity{}, false); newMult || !workedBefore {
		t.Fatalf("SHEL/20M (already worked) = newMult=%v workedBefore=%v, want false/true", newMult, workedBefore)
	}
	// Same county, different band: new mult.
	if newMult, workedBefore := state.wouldBeNewMultiplier(rules, "W4DEF", "40M", "SHEL", dxccEntity{}, false); !newMult || workedBefore {
		t.Fatalf("SHEL/40M (new band) = newMult=%v workedBefore=%v, want true/false", newMult, workedBefore)
	}
	// Unrecognized exchange text: no flag either way.
	if newMult, workedBefore := state.wouldBeNewMultiplier(rules, "K4XYZ", "20M", "TN", dxccEntity{}, false); newMult || workedBefore {
		t.Fatalf("unrecognized exchange text = newMult=%v workedBefore=%v, want false/false", newMult, workedBefore)
	}
}

// TestContestStateScoreNAQPAreaMultiplierFromReceivedExchange exercises
// NAQP's "naqp_area" multiplier (Rule 11, ncjweb.com/NAQP-Rules.pdf): a
// state/province or other-North-America-entity value parsed from the last
// token of the worked station's received exchange text ("Name Location"),
// counted per band ("Multipliers count again on each band"). A repeated
// value on the same band doesn't add another; the same value on a different
// band does; a non-North-America worked station contributes nothing.
func TestContestStateScoreNAQPAreaMultiplierFromReceivedExchange(t *testing.T) {
	state := newContestState()
	state.record(qso{call: "W4ABC", band: "20M", srxString: "BOB CA"})  // new: CA/20M
	state.record(qso{call: "K6XYZ", band: "20M", srxString: "JOE CA"})  // same area+band, no new mult
	state.record(qso{call: "W6DEF", band: "40M", srxString: "SUE CA"})  // same area, new band
	state.record(qso{call: "XE1GHI", band: "20M", srxString: "ANA XE"}) // other-NA entity: new mult
	state.record(qso{call: "JA1JKL", band: "20M", srxString: "KEN"})    // non-NA: no mult

	rules := &scoringRules{
		PointsPerQSO: 1,
		Multipliers:  []multiplierRule{{Kind: "naqp_area", Per: "band"}},
	}
	score := state.score(rules)
	if score.multipliers != 3 {
		t.Fatalf("multipliers = %d, want 3 (CA/20M + CA/40M + Mexico/20M)", score.multipliers)
	}
}

// TestContestStateWouldBeNewMultiplierNAQPArea exercises the as-you-type
// "NEW MULT" flag for the naqp_area kind, which — like exchange_area/
// tn_county — has no callsign-derived fallback and reads only the operator's
// in-progress received-exchange field text.
func TestContestStateWouldBeNewMultiplierNAQPArea(t *testing.T) {
	state := newContestState()
	state.record(qso{call: "W4ABC", band: "20M", srxString: "BOB CA"})

	rules := &scoringRules{
		Multipliers: []multiplierRule{{Kind: "naqp_area", Per: "band"}},
	}
	// Same area, same band, already worked: not a new mult.
	if newMult, workedBefore := state.wouldBeNewMultiplier(rules, "K6XYZ", "20M", "JOE CA", dxccEntity{}, false); newMult || !workedBefore {
		t.Fatalf("CA/20M (already worked) = newMult=%v workedBefore=%v, want false/true", newMult, workedBefore)
	}
	// Same area, different band: new mult.
	if newMult, workedBefore := state.wouldBeNewMultiplier(rules, "W6DEF", "40M", "SUE CA", dxccEntity{}, false); !newMult || workedBefore {
		t.Fatalf("CA/40M (new band) = newMult=%v workedBefore=%v, want true/false", newMult, workedBefore)
	}
	// Non-NA worked station: no flag either way.
	if newMult, workedBefore := state.wouldBeNewMultiplier(rules, "JA1JKL", "20M", "KEN", dxccEntity{}, false); newMult || workedBefore {
		t.Fatalf("non-NA exchange text = newMult=%v workedBefore=%v, want false/false", newMult, workedBefore)
	}
}

// TestContestStateScoreARRLSectionMultiplierCountsOncePerContest exercises
// ARRL Sweepstakes' "arrl_section" multiplier (Rule 5.2/5.3,
// contests.arrl.org/ContestRules/SS-Rules.pdf): the section parsed from the
// last token of the worked station's received exchange text ("Precedence
// Check Section"), counted once for the whole contest regardless of band —
// unlike naqp_area/exchange_area's per-band multipliers, since SS itself
// only allows working a station once regardless of band (dupe_scope "call").
func TestContestStateScoreARRLSectionMultiplierCountsOncePerContest(t *testing.T) {
	state := newContestState()
	state.record(qso{call: "W4ABC", band: "20M", srxString: "B 74 SCV"}) // new: SCV
	state.record(qso{call: "K6XYZ", band: "40M", srxString: "A 88 SCV"}) // same section, no new mult
	state.record(qso{call: "W1AW", band: "20M", srxString: "Q 79 CT"})   // new: CT

	rules := &scoringRules{
		PointsPerQSO: 2,
		Multipliers:  []multiplierRule{{Kind: "arrl_section", Per: "contest"}},
	}
	score := state.score(rules)
	if score.multipliers != 2 {
		t.Fatalf("multipliers = %d, want 2 (SCV + CT)", score.multipliers)
	}
}

// TestContestStateWouldBeNewMultiplierARRLSection exercises the as-you-type
// "NEW MULT" flag for the arrl_section kind, which — like naqp_area/
// exchange_area — has no callsign-derived fallback and reads only the
// operator's in-progress received-exchange field text.
func TestContestStateWouldBeNewMultiplierARRLSection(t *testing.T) {
	state := newContestState()
	state.record(qso{call: "W4ABC", band: "20M", srxString: "B 74 SCV"})

	rules := &scoringRules{
		Multipliers: []multiplierRule{{Kind: "arrl_section", Per: "contest"}},
	}
	// Same section, already worked (even on a different band): not a new mult.
	if newMult, workedBefore := state.wouldBeNewMultiplier(rules, "K6XYZ", "40M", "A 88 SCV", dxccEntity{}, false); newMult || !workedBefore {
		t.Fatalf("SCV (already worked) = newMult=%v workedBefore=%v, want false/true", newMult, workedBefore)
	}
	// New section: new mult.
	if newMult, workedBefore := state.wouldBeNewMultiplier(rules, "W1AW", "20M", "Q 79 CT", dxccEntity{}, false); !newMult || workedBefore {
		t.Fatalf("CT (new section) = newMult=%v workedBefore=%v, want true/false", newMult, workedBefore)
	}
	// Unrecognized exchange text: no flag either way.
	if newMult, workedBefore := state.wouldBeNewMultiplier(rules, "N0CALL", "20M", "garbage", dxccEntity{}, false); newMult || workedBefore {
		t.Fatalf("unrecognized exchange text = newMult=%v workedBefore=%v, want false/false", newMult, workedBefore)
	}
}

// TestContestStateScorePointsRuleWPXBandTiering exercises CQ WPX's
// band-tiered points (Rule V.B): high bands (10/15/20M) use the base
// same-continent/other-continent values, low bands (40/80/160M) double them,
// and North America's same-continent exception applies its own low-band
// value rather than doubling the base override.
func TestContestStateScorePointsRuleWPXBandTiering(t *testing.T) {
	state := newContestState()
	state.setStation("W1AW")                       // USA, NA
	state.record(qso{call: "VE3ABC", band: "20M"}) // NA same-continent, high band
	state.record(qso{call: "VE3ABC", band: "40M"}) // NA same-continent, low band
	state.record(qso{call: "JA1ABC", band: "20M"}) // other continent, high band
	state.record(qso{call: "JA1ABC", band: "80M"}) // other continent, low band

	rules := &scoringRules{
		Points: &pointsRule{
			SameContinent:                 1,
			OtherContinent:                3,
			SameContinentOverrides:        map[string]int{"NA": 2},
			LowBandSameContinent:          2,
			LowBandOtherContinent:         6,
			LowBandSameContinentOverrides: map[string]int{"NA": 4},
		},
	}
	score := state.score(rules)
	// VE3ABC/20M: NA override high = 2. VE3ABC/40M: NA override low = 4.
	// JA1ABC/20M: other-continent high = 3. JA1ABC/80M: other-continent low = 6.
	want := 2 + 4 + 3 + 6
	if score.qsoPoints != want {
		t.Fatalf("qsoPoints = %d, want %d", score.qsoPoints, want)
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
	if newMult, workedBefore := state.wouldBeNewMultiplier(rules, "W2XYZ", "20M", "", usEntity, true); newMult || !workedBefore {
		t.Fatalf("W2XYZ/20M (same country, same band) = newMult=%v workedBefore=%v, want false/true", newMult, workedBefore)
	}

	// A German call on the same band: new DXCC mult.
	dlEntity, found := table.lookup("DL1ABC")
	if !found {
		t.Fatal("expected DL1ABC to resolve to a DXCC entity")
	}
	if newMult, workedBefore := state.wouldBeNewMultiplier(rules, "DL1ABC", "20M", "", dlEntity, true); !newMult || workedBefore {
		t.Fatalf("DL1ABC/20M (new country) = newMult=%v workedBefore=%v, want true/false", newMult, workedBefore)
	}

	// Same USA country, but a fresh band: new DXCC mult again (Per:"band").
	if newMult, workedBefore := state.wouldBeNewMultiplier(rules, "W2XYZ", "40M", "", usEntity, true); !newMult || workedBefore {
		t.Fatalf("W2XYZ/40M (same country, new band) = newMult=%v workedBefore=%v, want true/false", newMult, workedBefore)
	}

	// Unresolved prefix: no dxcc/cqzone/ituzone rule can fire.
	if newMult, workedBefore := state.wouldBeNewMultiplier(rules, "ZZ1XYZ", "20M", "", dxccEntity{}, false); newMult || workedBefore {
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

// TestContestIndexRebuildFailureKeepsSameContestIndexVisible verifies the
// index cannot silently disappear after a read failure. The retained index is
// explicitly marked stale, so the operator can keep seeing the last known
// state without mistaking it for a fresh database recomputation.
func TestContestIndexRebuildFailureKeepsSameContestIndexVisible(t *testing.T) {
	st, err := openStore(t.TempDir() + "/logger.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })

	m := initialModel(st)
	cwopen := m.events[eventIndex(t, m.events, "CW-OPEN")]
	m.selectEvent(cwopen, cwopen.Sessions[0])
	original := m.contestIndex
	contestID := m.contestIndexID
	if original == nil {
		t.Fatal("expected initial contest index")
	}
	if err := st.Close(); err != nil {
		t.Fatal(err)
	}

	m.rebuildContestIndex()
	if m.contestIndex != original {
		t.Fatal("same-contest rebuild failure discarded the last known-good index")
	}
	if m.contestIndexID != contestID {
		t.Fatalf("contestIndexID = %q, want %q", m.contestIndexID, contestID)
	}
	if m.contestIndexError == "" {
		t.Fatal("rebuild failure did not mark contest analysis stale")
	}
	if !strings.Contains(m.continentPanelView(), "CONTEST ANALYSIS STALE") {
		t.Fatal("continent panel does not expose a stale contest index")
	}
}

// TestContestIndexRebuildFailureOnSwitchDoesNotReuseOldContest confirms a
// failed event switch clears the former event's index rather than presenting
// its dupe/multiplier state under the newly selected contest.
func TestContestIndexRebuildFailureOnSwitchDoesNotReuseOldContest(t *testing.T) {
	st, err := openStore(t.TempDir() + "/logger.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })

	m := initialModel(st)
	cwt := m.events[eventIndex(t, m.events, "CWT")]
	m.selectEvent(cwt, cwt.Sessions[0])
	if m.contestIndex == nil {
		t.Fatal("expected initial contest index")
	}
	if err := st.Close(); err != nil {
		t.Fatal(err)
	}
	cwopen := m.events[eventIndex(t, m.events, "CW-OPEN")]
	m.selectEvent(cwopen, cwopen.Sessions[0])
	selectedContestID := m.contestFields[contestName].Value()

	if m.contestIndex != nil {
		t.Fatal("failed contest switch retained the previous contest index")
	}
	if got, want := m.contestIndexID, selectedContestID; got != want {
		t.Fatalf("contestIndexID = %q, want selected contest %q", got, want)
	}
	if m.contestIndexError == "" {
		t.Fatal("failed contest switch did not expose an analysis error")
	}
	if !strings.Contains(m.continentPanelView(), "contest analysis unavailable") {
		t.Fatal("continent panel does not expose unavailable analysis after failed contest switch")
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

// TestContestStateScorePointsRuleCountryGroup exercises SAC's country-group
// points tier: a worked station's cty.dat country membership in
// pointsRule.CountryGroup takes precedence over the operator-relative
// same-country/same-continent/other-continent classification — the
// Scandinavian side's own rule (group points 0) means two Scandinavian
// stations working each other score 0 rather than the flat SameContinent
// value a plain continent match would otherwise give, while a
// non-Scandinavian European/non-European worked station still falls through
// to the ordinary continent classification.
func TestContestStateScorePointsRuleCountryGroup(t *testing.T) {
	state := newContestState()
	state.setStation("SM3ABC")                     // Sweden, EU
	state.record(qso{call: "LA1ABC", band: "20M"}) // Norway - in group, scores GroupPoints not SameContinent
	state.record(qso{call: "DL1ABC", band: "20M"}) // Germany - EU, not in group - SameContinent
	state.record(qso{call: "W1AW", band: "20M"})   // USA - NA, not in group - OtherContinent

	rules := &scoringRules{
		Points: &pointsRule{
			CountryGroup:   sacScandinavianCountries,
			GroupPoints:    0,
			SameContinent:  2,
			OtherContinent: 3,
		},
	}
	if score := state.score(rules); score.qsoPoints != 5 {
		t.Fatalf("qsoPoints = %d, want 5 (0 group + 2 same-continent + 3 other-continent)", score.qsoPoints)
	}
}

// TestContestStateScorePointsRuleCountryGroupLowBand exercises SAC's
// non-Scandinavian-entrant band tiering: a Scandinavian QSO scores
// GroupPoints on a high band and LowBandGroupPoints on a WPX-style low band
// (80M/40M — SAC's own "3.5 and 7 MHz" rule text).
func TestContestStateScorePointsRuleCountryGroupLowBand(t *testing.T) {
	state := newContestState()
	state.setStation("W1AW")                       // USA - not in the Scandinavian group
	state.record(qso{call: "SM3ABC", band: "20M"}) // high band
	state.record(qso{call: "OH2ABC", band: "80M"}) // low band

	rules := &scoringRules{
		Points: &pointsRule{
			CountryGroup:       sacScandinavianCountries,
			GroupPoints:        1,
			LowBandGroupPoints: 3,
		},
	}
	if score := state.score(rules); score.qsoPoints != 4 {
		t.Fatalf("qsoPoints = %d, want 4 (1 high band + 3 low band)", score.qsoPoints)
	}
}

// TestContestStateScoreSACAreaMultiplier exercises the "sac_area"
// multiplier: a Scandinavian-entity-plus-numeral value derived from the
// worked callsign, counted once per band (SAC rule 8.2). Multiple prefix
// variants from the same entity/numeral (SM3/SK3) count once; a different
// numeral is a new multiplier; a non-Scandinavian worked station contributes
// nothing.
func TestContestStateScoreSACAreaMultiplier(t *testing.T) {
	state := newContestState()
	state.record(qso{call: "SM3ABC", band: "20M"})
	state.record(qso{call: "SK3XYZ", band: "20M"}) // same entity+numeral, no new mult
	state.record(qso{call: "SM0ABC", band: "20M"}) // different numeral, new mult
	state.record(qso{call: "DL1ABC", band: "20M"}) // non-Scandinavian, no mult

	rules := &scoringRules{
		PointsPerQSO: 1,
		Multipliers:  []multiplierRule{{Kind: "sac_area", Per: "band"}},
	}
	if score := state.score(rules); score.multipliers != 2 {
		t.Fatalf("multipliers = %d, want 2 (Sweden-3 + Sweden-0)", score.multipliers)
	}
}

// TestContestStateWouldBeNewMultiplierSACArea exercises the as-you-type
// "NEW MULT" flag for the sac_area kind.
func TestContestStateWouldBeNewMultiplierSACArea(t *testing.T) {
	state := newContestState()
	state.record(qso{call: "SM3ABC", band: "20M"})

	rules := &scoringRules{
		Multipliers: []multiplierRule{{Kind: "sac_area", Per: "band"}},
	}
	sweden := dxccEntity{Country: "Sweden"}
	if newMult, workedBefore := state.wouldBeNewMultiplier(rules, "SK3XYZ", "20M", "", sweden, true); newMult || !workedBefore {
		t.Fatalf("Sweden-3 (already worked) = newMult=%v workedBefore=%v, want false/true", newMult, workedBefore)
	}
	if newMult, workedBefore := state.wouldBeNewMultiplier(rules, "SM0ABC", "20M", "", sweden, true); !newMult || workedBefore {
		t.Fatalf("Sweden-0 (new numeral) = newMult=%v workedBefore=%v, want true/false", newMult, workedBefore)
	}
	germany := dxccEntity{Country: "Fed. Rep. of Germany"}
	if newMult, workedBefore := state.wouldBeNewMultiplier(rules, "DL1ABC", "20M", "", germany, true); newMult || workedBefore {
		t.Fatalf("Germany (non-Scandinavian) = newMult=%v workedBefore=%v, want false/false", newMult, workedBefore)
	}
}

// TestContestStateScorePointsRuleZoneTiers exercises the IARU HF World
// Championship's zone-tiered points formula (pointsRule.Zone, Rule 5.1): a
// worked station whose exchanged ITU zone matches the operator's own scores
// SameZone regardless of country, a different zone on the operator's own
// continent scores SameContinentDifferentZone, a different continent scores
// OtherContinent, and an HQ/Official exchange (a non-numeric abbreviation)
// scores Special regardless of zone or continent (Rule 5.1.2). W1AW/K5ABC
// resolve to ITU zone 8 (NA), VE3ABC to zone 9 (NA), JA1ABC to zone 45 (AS).
func TestContestStateScorePointsRuleZoneTiers(t *testing.T) {
	state := newContestState()
	state.setStation("W1AW")                                          // ITU zone 8, NA
	state.record(qso{call: "K5ABC", band: "20M", srxString: "8"})     // same zone
	state.record(qso{call: "VE3ABC", band: "20M", srxString: "9"})    // same continent, different zone
	state.record(qso{call: "JA1ABC", band: "20M", srxString: "45"})   // other continent
	state.record(qso{call: "DL1ABC", band: "20M", srxString: "ARRL"}) // HQ/Official

	rules := &scoringRules{
		Points: &pointsRule{Zone: &zonePointsRule{
			SameZone:                   1,
			SameContinentDifferentZone: 3,
			OtherContinent:             5,
			Special:                    1,
		}},
	}
	score := state.score(rules)
	if score.qsoPoints != 10 {
		t.Fatalf("qsoPoints = %d, want 10 (1 + 3 + 5 + 1)", score.qsoPoints)
	}
}

// TestContestStateScoreIARUZoneAndHQMultipliers exercises the iaru_zone/
// iaru_hq multiplier kinds together (Rule 5.2.1): each distinct exchanged
// ITU zone and each distinct HQ/Official abbreviation counts once per band;
// a repeated value on the same band doesn't add another.
func TestContestStateScoreIARUZoneAndHQMultipliers(t *testing.T) {
	state := newContestState()
	state.record(qso{call: "K5ABC", band: "20M", srxString: "8"})
	state.record(qso{call: "K5DEF", band: "20M", srxString: "8"})    // same zone, same band: no new mult
	state.record(qso{call: "VE3ABC", band: "20M", srxString: "9"})   // new zone
	state.record(qso{call: "VE3XYZ", band: "40M", srxString: "9"})   // same zone, new band
	state.record(qso{call: "W1AW", band: "20M", srxString: "ARRL"})  // new HQ
	state.record(qso{call: "NU1AW", band: "20M", srxString: "IARU"}) // new HQ

	rules := &scoringRules{
		PointsPerQSO: 1,
		Multipliers: []multiplierRule{
			{Kind: "iaru_zone", Per: "band"},
			{Kind: "iaru_hq", Per: "band"},
		},
	}
	score := state.score(rules)
	if score.multipliers != 5 {
		t.Fatalf("multipliers = %d, want 5 (zone8/20M + zone9/20M + zone9/40M + ARRL/20M + IARU/20M)", score.multipliers)
	}
}

// TestContestStateWouldBeNewMultiplierIARUZoneAndHQ exercises the as-you-type
// "NEW MULT" flag for both new kinds, which — like exchange_area/tn_county —
// have no callsign-derived fallback and read only the operator's in-progress
// received-exchange field text.
func TestContestStateWouldBeNewMultiplierIARUZoneAndHQ(t *testing.T) {
	state := newContestState()
	state.record(qso{call: "K5ABC", band: "20M", srxString: "8"})
	state.record(qso{call: "W1AW", band: "20M", srxString: "ARRL"})

	rules := &scoringRules{
		Multipliers: []multiplierRule{
			{Kind: "iaru_zone", Per: "band"},
			{Kind: "iaru_hq", Per: "band"},
		},
	}
	if newMult, workedBefore := state.wouldBeNewMultiplier(rules, "K5DEF", "20M", "8", dxccEntity{}, false); newMult || !workedBefore {
		t.Fatalf("zone 8 (already worked) = newMult=%v workedBefore=%v, want false/true", newMult, workedBefore)
	}
	if newMult, workedBefore := state.wouldBeNewMultiplier(rules, "VE3ABC", "20M", "9", dxccEntity{}, false); !newMult || workedBefore {
		t.Fatalf("zone 9 (new zone) = newMult=%v workedBefore=%v, want true/false", newMult, workedBefore)
	}
	if newMult, workedBefore := state.wouldBeNewMultiplier(rules, "W2AW", "20M", "ARRL", dxccEntity{}, false); newMult || !workedBefore {
		t.Fatalf("ARRL (already worked) = newMult=%v workedBefore=%v, want false/true", newMult, workedBefore)
	}
	if newMult, workedBefore := state.wouldBeNewMultiplier(rules, "DL1ABC", "20M", "DARC", dxccEntity{}, false); !newMult || workedBefore {
		t.Fatalf("DARC (new HQ) = newMult=%v workedBefore=%v, want true/false", newMult, workedBefore)
	}
}

// TestContestStateScorePointsRuleHelvetiaCountryGroup exercises the Helvetia
// Contest's own points formula (uska.ch rules §2.7): a Swiss (HB9) contact
// scores the country-group value (10) regardless of the operator's own
// location, a same-country contact and a same-continent contact both score
// the flat 1-point value the rules don't otherwise distinguish, and a
// different-continent contact scores 3. W1AW is the operator (USA, NA);
// W2AZ is same-country, VE3ABC is same-continent (Canada, NA), JA1ABC is
// other-continent (Japan, AS), HB9AA is the Switzerland group hit.
func TestContestStateScorePointsRuleHelvetiaCountryGroup(t *testing.T) {
	state := newContestState()
	state.setStation("W1AW")
	state.record(qso{call: "HB9AA", band: "20M"})
	state.record(qso{call: "W2AZ", band: "20M"})
	state.record(qso{call: "VE3ABC", band: "20M"})
	state.record(qso{call: "JA1ABC", band: "20M"})

	rules := &scoringRules{
		Points: &pointsRule{
			CountryGroup:   []string{"Switzerland"},
			GroupPoints:    10,
			SameCountry:    1,
			SameContinent:  1,
			OtherContinent: 3,
		},
	}
	if score := state.score(rules); score.qsoPoints != 15 {
		t.Fatalf("qsoPoints = %d, want 15 (10 Switzerland + 1 same-country + 1 same-continent + 3 other-continent)", score.qsoPoints)
	}
}

// TestContestStateScoreCantonMultiplierFromReceivedExchange exercises the
// Helvetia Contest's "canton" multiplier: a Swiss canton parsed from the
// worked station's received exchange text, counted per band (uska.ch rules
// §2.7: "Canton and DXCC country ... per band"). A repeated canton on the
// same band doesn't add another; the same canton on a different band does;
// a non-Swiss station's own sent serial number contributes nothing.
func TestContestStateScoreCantonMultiplierFromReceivedExchange(t *testing.T) {
	state := newContestState()
	state.record(qso{call: "HB9AA", band: "20M", srxString: "ZH"}) // new: ZH/20M
	state.record(qso{call: "HB9BB", band: "20M", srxString: "zh"}) // same canton+band, case-insensitive
	state.record(qso{call: "HB9CC", band: "40M", srxString: "ZH"}) // same canton, new band
	state.record(qso{call: "HB9DD", band: "20M", srxString: "GE"}) // new: GE/20M
	state.record(qso{call: "W1AW", band: "20M", srxString: "042"}) // not a canton code

	rules := &scoringRules{
		PointsPerQSO: 1,
		Multipliers:  []multiplierRule{{Kind: "canton", Per: "band"}},
	}
	score := state.score(rules)
	if score.multipliers != 3 {
		t.Fatalf("multipliers = %d, want 3 (ZH/20M + ZH/40M + GE/20M)", score.multipliers)
	}
}

// TestContestStateWouldBeNewMultiplierCanton exercises the as-you-type "NEW
// MULT" flag for the canton kind, which — like tn_county — has no
// callsign-derived fallback and reads only the operator's in-progress
// received-exchange field text.
func TestContestStateWouldBeNewMultiplierCanton(t *testing.T) {
	state := newContestState()
	state.record(qso{call: "HB9AA", band: "20M", srxString: "ZH"})

	rules := &scoringRules{
		Multipliers: []multiplierRule{{Kind: "canton", Per: "band"}},
	}
	if newMult, workedBefore := state.wouldBeNewMultiplier(rules, "HB9BB", "20M", "ZH", dxccEntity{}, false); newMult || !workedBefore {
		t.Fatalf("ZH/20M (already worked) = newMult=%v workedBefore=%v, want false/true", newMult, workedBefore)
	}
	if newMult, workedBefore := state.wouldBeNewMultiplier(rules, "HB9CC", "40M", "ZH", dxccEntity{}, false); !newMult || workedBefore {
		t.Fatalf("ZH/40M (new band) = newMult=%v workedBefore=%v, want true/false", newMult, workedBefore)
	}
	if newMult, workedBefore := state.wouldBeNewMultiplier(rules, "W1AW", "20M", "042", dxccEntity{}, false); newMult || workedBefore {
		t.Fatalf("unrecognized exchange text = newMult=%v workedBefore=%v, want false/false", newMult, workedBefore)
	}
}

// TestContestStateScorePointsRuleRDXCCountryGroup exercises the Russian DX
// Contest's non-Russian-entrant points formula (rdxc.org rules, §7.2): a
// worked Russian station — European Russia, Asiatic Russia, Kaliningrad, or
// Franz Josef Land, each its own cty.dat DXCC entity per §7.3 — scores the
// flat 10-point group value regardless of which of the four it resolves to,
// while a same-country/same-continent/other-continent contact still falls
// through to the ordinary tiers. W1AW is the operator (USA, NA); K2AA is
// same-country, VE3ABC is same-continent (Canada, NA), JA1ABC is
// other-continent (Japan, AS).
func TestContestStateScorePointsRuleRDXCCountryGroup(t *testing.T) {
	state := newContestState()
	state.setStation("W1AW")
	state.record(qso{call: "RA3AA", band: "20M"})  // European Russia
	state.record(qso{call: "RA9AA", band: "20M"})  // Asiatic Russia
	state.record(qso{call: "UA2FF", band: "20M"})  // Kaliningrad
	state.record(qso{call: "RI1FJ", band: "20M"})  // Franz Josef Land
	state.record(qso{call: "K2AA", band: "20M"})   // same country
	state.record(qso{call: "VE3ABC", band: "20M"}) // same continent
	state.record(qso{call: "JA1ABC", band: "20M"}) // other continent

	rules := &scoringRules{
		Points: &pointsRule{
			CountryGroup:   []string{"European Russia", "Asiatic Russia", "Kaliningrad", "Franz Josef Land"},
			GroupPoints:    10,
			SameCountry:    2,
			SameContinent:  3,
			OtherContinent: 5,
		},
	}
	if score := state.score(rules); score.qsoPoints != 50 {
		t.Fatalf("qsoPoints = %d, want 50 (4x10 Russian + 2 same-country + 3 same-continent + 5 other-continent)", score.qsoPoints)
	}
}

// TestContestStateScoreOblastMultiplierFromReceivedExchange exercises the
// "oblast" multiplier: a Russian oblast code parsed from the worked
// station's received exchange text, counted per band (rdxc.org rules, §9).
// A repeated oblast on the same band doesn't add another; the same oblast on
// a different band does; a non-Russian station's own sent serial number
// contributes nothing.
func TestContestStateScoreOblastMultiplierFromReceivedExchange(t *testing.T) {
	state := newContestState()
	state.record(qso{call: "RA3AA", band: "20M", srxString: "MA"}) // new: MA/20M
	state.record(qso{call: "RA3BB", band: "20M", srxString: "ma"}) // same oblast+band, case-insensitive
	state.record(qso{call: "RA3CC", band: "40M", srxString: "MA"}) // same oblast, new band
	state.record(qso{call: "RA3DD", band: "20M", srxString: "SP"}) // new: SP/20M
	state.record(qso{call: "W1AW", band: "20M", srxString: "042"}) // not an oblast code

	rules := &scoringRules{
		PointsPerQSO: 1,
		Multipliers:  []multiplierRule{{Kind: "oblast", Per: "band"}},
	}
	score := state.score(rules)
	if score.multipliers != 3 {
		t.Fatalf("multipliers = %d, want 3 (MA/20M + MA/40M + SP/20M)", score.multipliers)
	}
}

// TestContestStateWouldBeNewMultiplierOblast exercises the as-you-type "NEW
// MULT" flag for the oblast kind, which — like canton — has no
// callsign-derived fallback and reads only the operator's in-progress
// received-exchange field text.
func TestContestStateWouldBeNewMultiplierOblast(t *testing.T) {
	state := newContestState()
	state.record(qso{call: "RA3AA", band: "20M", srxString: "MA"})

	rules := &scoringRules{
		Multipliers: []multiplierRule{{Kind: "oblast", Per: "band"}},
	}
	if newMult, workedBefore := state.wouldBeNewMultiplier(rules, "RA3BB", "20M", "MA", dxccEntity{}, false); newMult || !workedBefore {
		t.Fatalf("MA/20M (already worked) = newMult=%v workedBefore=%v, want false/true", newMult, workedBefore)
	}
	if newMult, workedBefore := state.wouldBeNewMultiplier(rules, "RA3CC", "40M", "MA", dxccEntity{}, false); !newMult || workedBefore {
		t.Fatalf("MA/40M (new band) = newMult=%v workedBefore=%v, want true/false", newMult, workedBefore)
	}
	if newMult, workedBefore := state.wouldBeNewMultiplier(rules, "W1AW", "20M", "042", dxccEntity{}, false); newMult || workedBefore {
		t.Fatalf("unrecognized exchange text = newMult=%v workedBefore=%v, want false/false", newMult, workedBefore)
	}
}

// TestContestStateScoreDXCCOrWAEMultiplier exercises the "dxcc_or_wae"
// multiplier: RDXC's country multiplier is "DXCC entity list + WAE
// multipliers list" (rdxc.org rules, §9), a union wider than the plain
// "dxcc" kind, which skips any cty.dat entity with no assigned DXCC number
// (recordMultiplierValue). RA3AA (European Russia) has a real DXCC number;
// European Turkey has none in this app's ARRL DXCC table (loadARRLDXCCNumbers)
// but is on the WAE Country List, so it still counts here — unlike the plain
// "dxcc" kind, which scores zero multipliers for it.
func TestContestStateScoreDXCCOrWAEMultiplier(t *testing.T) {
	state := newContestState()
	state.record(qso{call: "RA3AA", band: "20M"})  // European Russia: real DXCC number
	state.record(qso{call: "TA1ABC", band: "20M"}) // European Turkey: no DXCC number, on the WAE list
	state.record(qso{call: "TB1DEF", band: "20M"}) // same WAE entity, same band: no new mult

	rules := &scoringRules{
		PointsPerQSO: 1,
		Multipliers:  []multiplierRule{{Kind: "dxcc_or_wae", Per: "band"}},
	}
	score := state.score(rules)
	if score.multipliers != 2 {
		t.Fatalf("multipliers = %d, want 2 (European Russia + European Turkey)", score.multipliers)
	}

	plainDXCC := &scoringRules{
		PointsPerQSO: 1,
		Multipliers:  []multiplierRule{{Kind: "dxcc", Per: "band"}},
	}
	if score := state.score(plainDXCC); score.multipliers != 1 {
		t.Fatalf("plain dxcc multipliers = %d, want 1 (European Turkey has no DXCC number to count)", score.multipliers)
	}
}

// TestContestStateWouldBeNewMultiplierDXCCOrWAE exercises the as-you-type
// "NEW MULT" flag for the dxcc_or_wae kind, both for a real-DXCC entity and
// a WAE-only pseudo-entity with no DXCC number.
func TestContestStateWouldBeNewMultiplierDXCCOrWAE(t *testing.T) {
	state := newContestState()
	state.record(qso{call: "RA3AA", band: "20M"})
	state.record(qso{call: "TA1ABC", band: "20M"})

	rules := &scoringRules{
		Multipliers: []multiplierRule{{Kind: "dxcc_or_wae", Per: "band"}},
	}
	table, err := sharedDXCCTable()
	if err != nil {
		t.Fatal(err)
	}
	europeanRussia, found := table.lookup("RA3BB")
	if !found {
		t.Fatal("RA3BB did not resolve")
	}
	if newMult, workedBefore := state.wouldBeNewMultiplier(rules, "RA3BB", "20M", "", europeanRussia, true); newMult || !workedBefore {
		t.Fatalf("European Russia (already worked) = newMult=%v workedBefore=%v, want false/true", newMult, workedBefore)
	}
	europeanTurkey, found := table.lookup("TC1GHI")
	if !found {
		t.Fatal("TC1GHI did not resolve")
	}
	if newMult, workedBefore := state.wouldBeNewMultiplier(rules, "TC1GHI", "20M", "", europeanTurkey, true); newMult || !workedBefore {
		t.Fatalf("European Turkey (already worked) = newMult=%v workedBefore=%v, want false/true", newMult, workedBefore)
	}
	japan, found := table.lookup("JA1ABC")
	if !found {
		t.Fatal("JA1ABC did not resolve")
	}
	if newMult, workedBefore := state.wouldBeNewMultiplier(rules, "JA1ABC", "20M", "", japan, true); !newMult || workedBefore {
		t.Fatalf("Japan (new mult) = newMult=%v workedBefore=%v, want true/false", newMult, workedBefore)
	}
}
