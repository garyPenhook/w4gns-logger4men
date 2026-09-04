package main

import "testing"

// TestNAQPAreaCode covers the naqp_area multiplier's value derivation (NAQP
// Rule 11, ncjweb.com/NAQP-Rules.pdf): a bare state/province code, a
// "Name Location" exchange (only the last token is checked), the
// Newfoundland/Labrador alias combining into one value, Alaska/Hawaii
// (present here unlike exchange_area.go's CQ 160/ARRL DX table), and the
// "other North American entities" DXCC-prefix fallback for a non-US/Canada
// NA station.
func TestNAQPAreaCode(t *testing.T) {
	cases := []struct {
		text string
		want string
	}{
		{"CA", "CA"},
		{"BOB CA", "CA"},
		{"  bob   tn  ", "TN"},
		{"AK", "AK"},
		{"HI", "HI"},
		{"DC", "DC"},
		{"NF", "NL"},
		{"LB", "NL"},
		{"JOE NF", "NL"},
		{"XE", "Mexico"},
		{"BOB XE", "Mexico"},
		{"KP4", "Puerto Rico"},
		{"US", ""}, // US -> Ukraine in cty.dat, not a domestic code
		{"K", ""},  // resolves to United States, already handled by state codes
		{"VE", ""}, // resolves to Canada, already handled by province codes
		{"", ""},
		{"BOB", ""},
	}
	for _, c := range cases {
		if got := naqpAreaCode(c.text); got != c.want {
			t.Errorf("naqpAreaCode(%q) = %q, want %q", c.text, got, c.want)
		}
	}
}

// TestNAQPAreaCodesHas64Values guards the exact multiplier-table size NAQP
// Rule 11 specifies: 50 US states + DC (51) + 13 Canadian
// provinces/territories.
func TestNAQPAreaCodesHas64Values(t *testing.T) {
	if len(naqpAreaCodes) != 64 {
		t.Fatalf("len(naqpAreaCodes) = %d, want 64 (50 states + DC + 13 provinces/territories)", len(naqpAreaCodes))
	}
}
