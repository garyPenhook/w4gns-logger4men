package main

import "testing"

// TestRDXCOblastCode covers the oblast multiplier's value recognition: a
// valid two-letter oblast code (case-insensitive, trimmed) resolves to its
// canonical form, and non-matching text (a non-Russian station's own sent
// serial number, an RST-shaped value, an unrecognized token, blank) yields
// no multiplier value.
func TestRDXCOblastCode(t *testing.T) {
	cases := []struct {
		text string
		want string
	}{
		{"MA", "MA"},   // Moscow city
		{"ma", "MA"},
		{"  SP  ", "SP"}, // Saint Petersburg
		{"KA", "KA"},     // Kaliningrad (rule 7.3's "double multiplier" entity)
		{"FJ", "FJ"},     // Franz Josef Land
		{"AN", "AN"},     // Russian Antarctic stations
		{"042", ""},      // a non-Russian station's own sent serial number
		{"599", ""},      // an RST-shaped value, never an oblast
		{"XX", ""},
		{"", ""},
		{"   ", ""},
	}
	for _, c := range cases {
		if got := rdxcOblastCode(c.text); got != c.want {
			t.Errorf("rdxcOblastCode(%q) = %q, want %q", c.text, got, c.want)
		}
	}
}

// TestRDXCOblastCodesHas91Values guards the exact multiplier count the
// Russian DX Contest's own official oblast table (rdxc.org rules, "RUSSIAN
// OBLASTS ABBREVIATIONS") specifies, deduplicated by 2-letter code (a few
// oblasts span more than one prefix-number/suffix-letter partition in the
// source table under the same code).
func TestRDXCOblastCodesHas91Values(t *testing.T) {
	if len(rdxcOblastCodes) != 91 {
		t.Fatalf("len(rdxcOblastCodes) = %d, want 91", len(rdxcOblastCodes))
	}
}
