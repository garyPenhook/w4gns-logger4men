package main

import (
	"fmt"
	"time"
)

// rateMeter is the roadmap's rate meter (Appendix B/D "Rate meter (Q/hr
// L10/L100/overall, Q/Mult)"): how fast the operator is working stations,
// updated live from the same contestState index that backs scoring and the
// analysis panel, so it can never disagree with them.
type rateMeter struct {
	totalQSOs int
	last10    float64 // Q/hr computed over the most recent 10 QSOs (or fewer)
	last100   float64 // Q/hr computed over the most recent 100 QSOs (or fewer)
	overall   float64 // Q/hr computed over the whole contest so far
	qPerMult  float64 // total QSOs / current multiplier count
}

// rate computes the rate meter as of now. Each Q/hr figure covers the most
// recent window of QSOs (10, 100, or all of them): the rate is the window's
// QSO count divided by the elapsed time from the window's oldest QSO to now
// — not to the window's newest QSO — so the rate visibly decays if the
// operator stops calling CQ, matching what an operator expects from a live
// rate meter (SD's behaves the same way).
func (c *contestState) rate(now time.Time, rules *scoringRules) rateMeter {
	total := len(c.times)
	meter := rateMeter{totalQSOs: total}
	if total == 0 {
		return meter
	}
	meter.last10 = qsoRate(c.times, 10, now)
	meter.last100 = qsoRate(c.times, 100, now)
	meter.overall = qsoRate(c.times, total, now)

	mults := 0
	if rules != nil {
		mults = c.score(rules).multipliers
	}
	if mults == 0 {
		mults = len(c.uniqueCalls)
	}
	if mults > 0 {
		meter.qPerMult = float64(total) / float64(mults)
	}
	return meter
}

// qsoRate returns the Q/hr rate for the most recent min(n, len(times))
// entries in times (chronologically ordered), measured from the oldest QSO
// in that window through now.
func qsoRate(times []time.Time, n int, now time.Time) float64 {
	if len(times) == 0 {
		return 0
	}
	if n > len(times) {
		n = len(times)
	}
	start := times[len(times)-n]
	elapsedHours := now.Sub(start).Hours()
	if elapsedHours <= 0 {
		return 0
	}
	return float64(n) / elapsedHours
}

// rateMeterLine renders the rate meter status line shown beneath Recent
// QSOs/DX Spots on QSO Entry (roadmap Appendix C mockup: "Rate: L10 42.0
// L100 38.5 All 39.2 Q/Mult 3.6"). Returns "" when there's no active
// contest index or nothing logged yet, so callers can omit the line and its
// spacing entirely, matching how analysisPanel/dxSpotsPanel degrade.
func (m model) rateMeterLine() string {
	if m.contestIndex == nil || len(m.contestIndex.times) == 0 {
		return ""
	}
	var rules *scoringRules
	if event, ok := m.eventForContestID(); ok {
		rules = event.Scoring
	}
	rm := m.contestIndex.rate(time.Now(), rules)
	line := fmt.Sprintf("Rate: L10 %.1f  L100 %.1f  All %.1f", rm.last10, rm.last100, rm.overall)
	if rm.qPerMult > 0 {
		line += fmt.Sprintf("  Q/Mult %.1f", rm.qPerMult)
	}
	return line
}
