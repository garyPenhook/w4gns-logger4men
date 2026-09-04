package main

import (
	"context"
	"strings"
)

// contestState is the in-memory index described in the roadmap (Appendix C):
// a single pass over one contest's QSOs that every scoring/analysis
// consumer reads, so they can never disagree with each other. It is built
// fresh from the store (buildContestState) rather than mutated incrementally
// today; the roadmap's "incremental on log, full recompute on edit" wiring
// belongs to the live TUI model once the as-you-type panels land, but a full
// rebuild is always correct and, at contest-log sizes (a few thousand QSOs),
// cheap enough to call on every score/export.
type contestState struct {
	// byCall lists every QSO worked with a given callsign (upper-cased), in
	// the order logged. Feeds Check Partial and the band/mode matrix.
	byCall map[string][]qso
	// workedCallBand is the set of "CALL|BAND" keys already logged, i.e. the
	// dupe/worked-this-band test.
	workedCallBand map[string]struct{}
	// uniqueCalls is the set of distinct callsigns worked in the contest,
	// regardless of band — the "unique_call" multiplier rule.
	uniqueCalls map[string]struct{}
}

// newContestState returns an empty index, ready for QSOs to be recorded.
func newContestState() *contestState {
	return &contestState{
		byCall:         make(map[string][]qso),
		workedCallBand: make(map[string]struct{}),
		uniqueCalls:    make(map[string]struct{}),
	}
}

// record adds one QSO to the index. Safe to call repeatedly in chronological
// order while building, or once per newly logged QSO for an incremental
// update.
func (c *contestState) record(q qso) {
	call := strings.ToUpper(strings.TrimSpace(q.call))
	if call == "" {
		return
	}
	band := strings.ToUpper(strings.TrimSpace(q.band))
	c.byCall[call] = append(c.byCall[call], q)
	c.workedCallBand[call+"|"+band] = struct{}{}
	c.uniqueCalls[call] = struct{}{}
}

// isWorkedOnBand reports whether call has already been logged on band —
// the same test as the dupe check, exposed here so future panels and
// scoring agree with the store-backed store.isDupe check used live.
func (c *contestState) isWorkedOnBand(call, band string) bool {
	_, ok := c.workedCallBand[strings.ToUpper(strings.TrimSpace(call))+"|"+strings.ToUpper(strings.TrimSpace(band))]
	return ok
}

// score tallies a contestScore from the index per rules: PointsPerQSO once
// per unique (call, band) already recorded, multiplied by the multiplier
// count rules.Multiplier selects. Mirrors the dedup behavior the previous
// per-call computeContestScore implementation had (a same-band duplicate
// still counts as a multiplier, since the callsign was still worked).
func (c *contestState) score(rules *scoringRules) contestScore {
	if rules == nil {
		return contestScore{}
	}
	var out contestScore
	out.qsoPoints = len(c.workedCallBand) * rules.PointsPerQSO
	if rules.Multiplier == "unique_call" {
		out.multipliers = len(c.uniqueCalls)
	}
	return out
}

// buildContestState scans every QSO logged under contestID for profileID and
// returns the resulting index. Called on contest open and whenever a full
// recompute is needed (e.g. after an edit changes a call or band).
func buildContestState(ctx context.Context, profileID int64, contestID string, st *store) (*contestState, error) {
	state := newContestState()
	err := st.forEachQSOForContest(ctx, profileID, contestID, func(q qso) error {
		state.record(q)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return state, nil
}
