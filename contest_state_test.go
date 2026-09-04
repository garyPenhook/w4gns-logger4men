package main

import (
	"context"
	"testing"
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
