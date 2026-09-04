package main

import "testing"

// TestIsWAECountry covers isWAECountry's membership check: a WAE Country
// List entry matches case-insensitively, a non-European DXCC entity never
// matches, and a blank country never matches (an unresolved worked entity
// must not be silently treated as a WAE-list member).
func TestIsWAECountry(t *testing.T) {
	cases := []struct {
		country string
		want    bool
	}{
		{"Fed. Rep. of Germany", true},
		{"fed. rep. of germany", true},
		{"England", true},
		{"Balearic Islands", true},
		{"United States", false},
		{"Japan", false},
		{"", false},
	}
	for _, c := range cases {
		if got := isWAECountry(c.country); got != c.want {
			t.Errorf("isWAECountry(%q) = %v, want %v", c.country, got, c.want)
		}
	}
}

// TestWAECountriesHas69Values guards the WAE Country List's exact size, as
// resolved through this app's own cty.dat (darc.de's rules page lists 79
// prefix tokens; several fold into one country here — see wae_country.go's
// comment on 4U1V/GM-s/JW-b).
func TestWAECountriesHas69Values(t *testing.T) {
	if len(waeCountries) != 69 {
		t.Fatalf("len(waeCountries) = %d, want 69", len(waeCountries))
	}
}

// TestWAEBandBonus covers Section 6's per-band multiplier weighting: 4x on
// 80M, 3x on 40M, 2x on 20/15/10M, and 0 for a band outside WAE's own band
// plan rather than guessing a weight.
func TestWAEBandBonus(t *testing.T) {
	cases := []struct {
		band string
		want int
	}{
		{"80M", 4},
		{"40m", 3},
		{"40M", 3},
		{"20M", 2},
		{"15M", 2},
		{"10M", 2},
		{"160M", 0},
		{"", 0},
	}
	for _, c := range cases {
		if got := waeBandBonus(c.band); got != c.want {
			t.Errorf("waeBandBonus(%q) = %d, want %d", c.band, got, c.want)
		}
	}
}
