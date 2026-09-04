package main

import "testing"

// TestCantonCode covers the canton multiplier's value recognition: a valid
// two-letter canton code (case-insensitive, trimmed) resolves to its
// canonical form, and non-matching text (a plain serial number, an
// unrecognized token, blank) yields no multiplier value.
func TestCantonCode(t *testing.T) {
	cases := []struct {
		text string
		want string
	}{
		{"ZH", "ZH"},
		{"zh", "ZH"},
		{"  GE  ", "GE"},
		{"TI", "TI"},
		{"042", ""}, // a non-Swiss station's own sent serial number
		{"599", ""}, // an RST-shaped value, never a canton
		{"XX", ""},
		{"", ""},
		{"   ", ""},
	}
	for _, c := range cases {
		if got := cantonCode(c.text); got != c.want {
			t.Errorf("cantonCode(%q) = %q, want %q", c.text, got, c.want)
		}
	}
}

// TestCantonCodesHas26Values guards the exact multiplier count the Helvetia
// Contest rules specify (uska.ch, §2.11): all 26 Swiss cantons.
func TestCantonCodesHas26Values(t *testing.T) {
	if len(cantonCodes) != 26 {
		t.Fatalf("len(cantonCodes) = %d, want 26", len(cantonCodes))
	}
}
