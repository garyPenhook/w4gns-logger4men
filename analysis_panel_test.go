package main

import (
	"strings"
	"testing"
)

// analysisTestModel builds a model with the CW Open event selected (the one
// catalog entry with Scoring configured, see events/cwops.json) and a
// station lat/lon set, so tests can exercise country/zone, mult-flag, and
// beam-heading lines without a DB round-trip per case.
func analysisTestModel(t *testing.T) model {
	t.Helper()
	st, err := openStore(t.TempDir() + "/logger.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })

	m := initialModel(st)
	cwopen := m.events[eventIndex(t, m.events, "CW-OPEN")]
	m.selectEvent(cwopen, cwopen.Sessions[0])
	m.screen = qsoEntryScreen
	m.termWidth = 200

	// W4GNS's own cty.dat coordinates (dxcc_test.go), used as the station's
	// origin for beam heading — avoids a saveStationProfile/grid round-trip.
	lat, lon := 37.60, -91.87
	m.activeStation.Latitude = &lat
	m.activeStation.Longitude = &lon

	return m
}

func TestAnalysisPanelWidthAndPreconditionGating(t *testing.T) {
	m := analysisTestModel(t)
	m.fields[fieldCall].SetValue("1A0KM")

	if got := m.analysisPanel(analysisPanelMinWidth - 1); got != "" {
		t.Fatalf("analysisPanel below min width = %q, want empty", got)
	}
	if got := m.analysisPanel(200); got == "" {
		t.Fatal("analysisPanel with an active contest, a call, and enough width = empty, want content")
	}

	m.fields[fieldCall].SetValue("")
	if got := m.analysisPanel(200); got != "" {
		t.Fatalf("analysisPanel with no callsign typed = %q, want empty", got)
	}

	m.fields[fieldCall].SetValue("1A0KM")
	m.contestFields[contestName].SetValue("")
	if got := m.analysisPanel(200); got != "" {
		t.Fatalf("analysisPanel with no active contest = %q, want empty", got)
	}
}

func TestAnalysisPanelCountryAndHeading(t *testing.T) {
	m := analysisTestModel(t)
	// Sov Mil Order of Malta (Rome): CQ15 ITU28 EU, lat 41.90/lon 12.43 —
	// unambiguously east of and closer than antipodal to the W4GNS origin
	// used by analysisTestModel, so bearing/distance are well-defined.
	m.fields[fieldCall].SetValue("1A0KM")

	panel := m.analysisPanel(200)
	if !strings.Contains(panel, "Sov Mil Order of Malta") {
		t.Errorf("panel = %q, want the resolved country name", panel)
	}
	if !strings.Contains(panel, "CQ15") || !strings.Contains(panel, "ITU28") {
		t.Errorf("panel = %q, want CQ/ITU zone", panel)
	}
	if !strings.Contains(panel, "Bearing") || !strings.Contains(panel, "km") {
		t.Errorf("panel = %q, want a bearing/distance line (station lat/lon is set)", panel)
	}

	// Without station coordinates, there's no origin to compute a bearing
	// from — the line must be omitted, not fabricated from zero values.
	m.activeStation.Latitude = nil
	m.activeStation.Longitude = nil
	panel = m.analysisPanel(200)
	if strings.Contains(panel, "Bearing") {
		t.Errorf("panel = %q, want no Bearing line without station lat/lon", panel)
	}
}

// TestAnalysisPanelUsesEnteredGridBeforeEntityCentroid verifies that an
// operator-entered (or QRZ-returned) grid wins over the much broader country
// centroid when calculating a short-path bearing. This matters for countries
// with territory spread over thousands of kilometres.
func TestAnalysisPanelUsesEnteredGridBeforeEntityCentroid(t *testing.T) {
	m := analysisTestModel(t)
	m.fields[fieldCall].SetValue("1A0KM")

	withoutGrid := analysisBearingLine(m.analysisPanel(200))
	if withoutGrid == "" {
		t.Fatal("analysis panel without a grid has no bearing line")
	}
	m.detailFields[detailGrid].SetValue("FN31") // Connecticut, not Rome.
	withGrid := analysisBearingLine(m.analysisPanel(200))
	if withGrid == "" {
		t.Fatal("analysis panel with a valid grid has no bearing line")
	}
	if withGrid == withoutGrid {
		t.Fatalf("bearing with grid = %q, want it to differ from entity-centroid bearing %q", withGrid, withoutGrid)
	}
}

func analysisBearingLine(panel string) string {
	for _, line := range strings.Split(panel, "\n") {
		if strings.HasPrefix(line, "Bearing ") {
			return line
		}
	}
	return ""
}

func TestAnalysisPanelUnknownPrefix(t *testing.T) {
	m := analysisTestModel(t)
	m.fields[fieldCall].SetValue("!!!!!")

	panel := m.analysisPanel(200)
	if !strings.Contains(panel, "unknown prefix") {
		t.Errorf("panel = %q, want an unknown-prefix line for an unresolvable call", panel)
	}
}

// TestAnalysisPanelNewMultFlag exercises the mult-flag priority order: a
// call not yet worked under CW Open's unique_call rule shows NEW MULT; once
// it's logged (on any band), it shows "worked before" instead — matching
// contestState.score()'s "one multiplier per unique callsign" rule.
func TestAnalysisPanelNewMultFlag(t *testing.T) {
	m := analysisTestModel(t)
	m.fields[fieldCall].SetValue("W1AW")

	panel := m.analysisPanel(200)
	if !strings.Contains(panel, "NEW MULT") {
		t.Errorf("panel before working W1AW = %q, want NEW MULT", panel)
	}

	m.fields[fieldBand].SetValue("20M")
	m, _ = m.logCurrentQSO()
	m.fields[fieldCall].SetValue("W1AW")

	panel = m.analysisPanel(200)
	if strings.Contains(panel, "NEW MULT") {
		t.Errorf("panel after working W1AW = %q, want no NEW MULT", panel)
	}
	if !strings.Contains(panel, "Worked before") {
		t.Errorf("panel after working W1AW = %q, want a worked-before line", panel)
	}
	if !strings.Contains(panel, "Worked: 20M") {
		t.Errorf("panel after working W1AW on 20M = %q, want a Worked: 20M line", panel)
	}
}

// TestAnalysisPanelNoMultFlagWithoutScoringRules confirms the panel doesn't
// assert a multiplier claim for an event with no configured Scoring — CWT
// has none (see events/cwops.json) — rather than guessing.
func TestAnalysisPanelNoMultFlagWithoutScoringRules(t *testing.T) {
	m := analysisTestModel(t)
	cwt := m.events[eventIndex(t, m.events, "CWT")]
	m.selectEvent(cwt, cwt.Sessions[0])
	m.screen = qsoEntryScreen
	m.fields[fieldCall].SetValue("W1AW")

	panel := m.analysisPanel(200)
	if strings.Contains(panel, "MULT") {
		t.Errorf("panel for a no-scoring event = %q, want no multiplier claim", panel)
	}
}

func TestAnalysisPanelDupeLine(t *testing.T) {
	m := analysisTestModel(t)
	m.fields[fieldCall].SetValue("W1AW")
	m.fields[fieldBand].SetValue("20M")
	m, _ = m.logCurrentQSO()

	m.fields[fieldCall].SetValue("W1AW")
	m.fields[fieldBand].SetValue("20M")
	m.checkDupe()
	if !m.dupeWarning {
		t.Fatal("re-entering the same call/band did not set dupeWarning")
	}

	panel := m.analysisPanel(200)
	if !strings.Contains(panel, "DUPE") {
		t.Errorf("panel while dupeWarning is set = %q, want a DUPE line", panel)
	}
}

// TestAnalysisPanelCheckPartial exercises the roadmap's Check Partial list
// (Appendix B.3): a fragment typed so far should surface other logged calls
// containing it, styled by whether logging that candidate on the currently
// selected band would be new (bold) or a dupe (dim) — not by the in-progress
// call's own worked state, which the "Worked:"/"NEW MULT" lines already
// cover.
func TestAnalysisPanelCheckPartial(t *testing.T) {
	m := analysisTestModel(t)

	m.fields[fieldCall].SetValue("W1AW")
	m.fields[fieldBand].SetValue("20M")
	m, _ = m.logCurrentQSO()

	m.fields[fieldCall].SetValue("K1AW")
	m.fields[fieldBand].SetValue("40M")
	m.fields[fieldFrequency].SetValue("7.025")
	m, _ = m.logCurrentQSO()

	// "1AW" fragments both W1AW and K1AW; on 20M, W1AW is a dupe and K1AW is
	// new (not yet worked on that band).
	m.fields[fieldCall].SetValue("1AW")
	m.fields[fieldBand].SetValue("20M")
	m.fields[fieldFrequency].SetValue("14.025")

	panel := m.analysisPanel(200)
	if !strings.Contains(panel, "Partial:") {
		t.Fatalf("panel = %q, want a Partial: line", panel)
	}
	if !strings.Contains(panel, "W1AW") || !strings.Contains(panel, "K1AW") {
		t.Errorf("panel = %q, want both W1AW and K1AW as candidates", panel)
	}
}

// TestAnalysisPanelCheckPartialExcludesExactMatch confirms a fragment that
// exactly equals an already-logged call doesn't list itself as a candidate
// — that call already has its own "Worked:" line.
func TestAnalysisPanelCheckPartialExcludesExactMatch(t *testing.T) {
	m := analysisTestModel(t)

	m.fields[fieldCall].SetValue("W1AW")
	m.fields[fieldBand].SetValue("20M")
	m, _ = m.logCurrentQSO()

	m.fields[fieldCall].SetValue("W1AW")
	panel := m.analysisPanel(200)
	if strings.Contains(panel, "Partial:") {
		t.Errorf("panel = %q, want no Partial: line for an exact-match fragment with no other candidates", panel)
	}
}
