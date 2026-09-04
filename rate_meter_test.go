package main

import (
	"testing"
	"time"
)

// TestQSORateWindowsElapsedFromOldestInWindowToNow exercises the rate meter's
// core math (roadmap Appendix B/D): the Q/hr for a window is that window's
// QSO count divided by elapsed time from the *oldest* QSO in the window to
// now — so a longer idle gap since the last QSO drags the rate down, and a
// window smaller than the requested size (fewer than 10 QSOs logged so far)
// still computes over whatever's there.
func TestQSORateWindowsElapsedFromOldestInWindowToNow(t *testing.T) {
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	// Five QSOs one minute apart, so all five fall inside a "last 10" window.
	var times []time.Time
	for i := 0; i < 5; i++ {
		times = append(times, base.Add(time.Duration(i)*time.Minute))
	}
	now := base.Add(10 * time.Minute) // 10 minutes after the first QSO

	if got := qsoRate(times, 10, now); got != 30 { // 5 QSOs / (10min/60) = 30/hr
		t.Fatalf("qsoRate(window=10) = %v, want 30", got)
	}
	// Window of 2 only looks at the newest 2 QSOs (index 3,4 at 3m,4m), so
	// elapsed is now(10m) - 3m = 7m.
	want := 2 / (7.0 / 60.0)
	if got := qsoRate(times, 2, now); !floatsClose(got, want) {
		t.Fatalf("qsoRate(window=2) = %v, want %v", got, want)
	}
}

// TestQSORateEmptyOrNonPositiveElapsedIsZero guards the meter against
// dividing by zero or negative elapsed time (e.g. now equals the QSO's own
// timestamp right after logging it), which would otherwise render as +Inf.
func TestQSORateEmptyOrNonPositiveElapsedIsZero(t *testing.T) {
	if got := qsoRate(nil, 10, time.Now()); got != 0 {
		t.Fatalf("qsoRate(nil) = %v, want 0", got)
	}
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	if got := qsoRate([]time.Time{now}, 10, now); got != 0 {
		t.Fatalf("qsoRate(elapsed=0) = %v, want 0", got)
	}
}

// TestContestStateRateComputesQPerMultFromScoringRules exercises the Q/Mult
// figure: total QSOs logged divided by the multiplier count the event's
// scoring rules recognize (unique_call here, matching CW Open), not just raw
// unique calls, so it agrees with the same score() the Cabrillo export uses.
func TestContestStateRateComputesQPerMultFromScoringRules(t *testing.T) {
	state := newContestState()
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	state.record(qso{call: "W1AW", band: "20M", time: base})
	state.record(qso{call: "W1AW", band: "40M", time: base.Add(time.Minute)}) // same call, new band
	state.record(qso{call: "K1ABC", band: "20M", time: base.Add(2 * time.Minute)})

	rules := &scoringRules{PointsPerQSO: 1, Multiplier: "unique_call"}
	rm := state.rate(base.Add(10*time.Minute), rules)

	if rm.totalQSOs != 3 {
		t.Fatalf("totalQSOs = %d, want 3", rm.totalQSOs)
	}
	// 2 unique calls (W1AW, K1ABC) worked across those 3 QSOs.
	if want := 3.0 / 2.0; !floatsClose(rm.qPerMult, want) {
		t.Fatalf("qPerMult = %v, want %v", rm.qPerMult, want)
	}
}

func floatsClose(a, b float64) bool {
	const eps = 1e-9
	d := a - b
	if d < 0 {
		d = -d
	}
	return d < eps
}
