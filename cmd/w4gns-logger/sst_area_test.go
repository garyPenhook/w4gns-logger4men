package main

import "testing"

// TestSSTAreaCode covers the sst_area multiplier's value derivation (K1USN
// SST Rules, linked from k1usn.com/sst_rules.html): the same "Name Location"
// exchange shape and state/province table naqp_area.go uses, but with a
// worldwide DXCC fallback instead of naqpAreaCode's North-America-only one —
// the SST rules give a DXCC-country multiplier for "stations worked outside
// the USA lower 48 states and Canada", with no continent restriction.
func TestSSTAreaCode(t *testing.T) {
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
		// Unlike naqpAreaCode, a non-North-American entity still counts —
		// SST's DXCC multiplier is worldwide, not NA-only.
		{"DL", "Fed. Rep. of Germany"},
		{"BOB DL", "Fed. Rep. of Germany"},
		{"JA", "Japan"},
		{"US", "Ukraine"}, // US -> Ukraine in cty.dat; unlike naqpAreaCode, non-NA still counts here
		{"K", ""},         // resolves to United States, already handled by state codes
		{"VE", ""},        // resolves to Canada, already handled by province codes
		{"", ""},
		{"QQQ", ""}, // no cty.dat prefix match at all
	}
	for _, c := range cases {
		if got := sstAreaCode(c.text); got != c.want {
			t.Errorf("sstAreaCode(%q) = %q, want %q", c.text, got, c.want)
		}
	}
}
