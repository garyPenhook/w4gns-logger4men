package main

import "testing"

// TestTNCountyCode covers the tn_county multiplier's value recognition: a
// valid four-letter county code (case-insensitive, trimmed) resolves to its
// canonical form, and non-matching text (a state/province abbreviation, an
// unrecognized token, blank) yields no multiplier value.
func TestTNCountyCode(t *testing.T) {
	cases := []struct {
		text string
		want string
	}{
		{"SHEL", "SHEL"},
		{"shel", "SHEL"},
		{"  DAVI  ", "DAVI"},
		{"WILS", "WILS"},
		{"TN", ""},   // an out-of-state station's own sent exchange, not a county
		{"100W", ""}, // a DX station's power exchange
		{"", ""},
		{"   ", ""},
	}
	for _, c := range cases {
		if got := tnCountyCode(c.text); got != c.want {
			t.Errorf("tnCountyCode(%q) = %q, want %q", c.text, got, c.want)
		}
	}
}

// TestTNCountyCodesHas95Values guards the exact multiplier count tnqp.org's
// rules specify: all 95 Tennessee counties, matching
// events/tnqp.json's received_exchange_options typeahead list.
func TestTNCountyCodesHas95Values(t *testing.T) {
	if len(tnCountyCodes) != 95 {
		t.Fatalf("len(tnCountyCodes) = %d, want 95", len(tnCountyCodes))
	}
}
