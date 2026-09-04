package main

import "testing"

// TestExchangeAreaCode covers the exchange_area multiplier's value
// recognition: a valid US state/DC/Canadian province code (case-insensitive,
// trimmed) resolves to its canonical form, the ARRL DX CW rule text's
// alternate "NF" spelling for Newfoundland/Labrador maps to the postal code
// this table otherwise uses, and non-matching text (a DX station's power
// exchange, an unrecognized token, blank) yields no multiplier value.
func TestExchangeAreaCode(t *testing.T) {
	cases := []struct {
		text string
		want string
	}{
		{"CT", "CT"},
		{"ct", "CT"},
		{"  TX  ", "TX"},
		{"DC", "DC"},
		{"ON", "ON"},
		{"nf", "NL"},
		{"NF", "NL"},
		{"NL", "NL"},
		{"AK", ""},   // Alaska: excluded (not one of the 48 contiguous states)
		{"HI", ""},   // Hawaii: same
		{"100W", ""}, // a DX station's power exchange, not a state/province
		{"5", ""},
		{"", ""},
		{"   ", ""},
	}
	for _, c := range cases {
		if got := exchangeAreaCode(c.text); got != c.want {
			t.Errorf("exchangeAreaCode(%q) = %q, want %q", c.text, got, c.want)
		}
	}
}

// TestExchangeAreaCodesHas63Values guards the exact multiplier count both
// CQ 160 Meter CW and ARRL DX CW's rules specify (48 contiguous states + DC
// + 14 Canadian provinces/territories = 63; ARRL DX Rules 5.2.2's own
// "maximum of 63 per band").
func TestExchangeAreaCodesHas63Values(t *testing.T) {
	if len(exchangeAreaCodes) != 63 {
		t.Fatalf("len(exchangeAreaCodes) = %d, want 63", len(exchangeAreaCodes))
	}
}
