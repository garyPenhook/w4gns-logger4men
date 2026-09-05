package main

import "testing"

// TestDOKDistrictCode covers the dok_district multiplier's value
// recognition: a German station's DOK (letter + local-chapter digits)
// resolves to its district letter, a non-member's explicit "NM" exchange
// yields no multiplier (even though "N" is itself a real district letter),
// and a non-German station's plain serial number (digits only) also yields
// no multiplier.
func TestDOKDistrictCode(t *testing.T) {
	cases := []struct {
		text string
		want string
	}{
		{"F41", "F"},
		{"f41", "F"},
		{"  P03  ", "P"},
		{"G51", "G"},
		{"J99", "J"}, // rare special-DOK: WAG's own "mysterious 26th multiplier"
		{"NM", ""},   // non-member German station: rules say "no multiplier"
		{"nm", ""},
		{"042", ""}, // a non-German station's own sent serial number
		{"599", ""}, // an RST-shaped value, never a DOK
		{"", ""},
		{"   ", ""},
	}
	for _, c := range cases {
		if got := dokDistrictCode(c.text); got != c.want {
			t.Errorf("dokDistrictCode(%q) = %q, want %q", c.text, got, c.want)
		}
	}
}
