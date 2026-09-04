package main

import "testing"

// TestARRLSectionCode covers the arrl_section multiplier's value derivation
// (ARRL Sweepstakes Rule 5.2, contests.arrl.org/ContestRules/SS-Rules.pdf): a
// bare section code, a "Precedence Check Section" exchange (only the last
// token is checked, matching the exchange's own required send order), and
// both spelling-variant aliases (GTA -> GH, NT -> TER).
func TestARRLSectionCode(t *testing.T) {
	cases := []struct {
		text string
		want string
	}{
		{"SCV", "SCV"},
		{"B 74 SCV", "SCV"},
		{"  a   79   ct  ", "CT"},
		{"A 79 GTA", "GH"},
		{"M 88 NT", "TER"},
		{"", ""},
		{"BOB", ""},
	}
	for _, c := range cases {
		if got := arrlSectionCode(c.text); got != c.want {
			t.Errorf("arrlSectionCode(%q) = %q, want %q", c.text, got, c.want)
		}
	}
}

// TestARRLSectionCodesHas85Values guards the exact multiplier-table size the
// ARRL/RAC Section Abbreviation List specifies: 71 US sections across the
// ARRL divisions plus 14 Canadian RAC sections.
func TestARRLSectionCodesHas85Values(t *testing.T) {
	if len(arrlSectionCodes) != 85 {
		t.Fatalf("len(arrlSectionCodes) = %d, want 85 (71 US sections + 14 Canadian sections)", len(arrlSectionCodes))
	}
}
