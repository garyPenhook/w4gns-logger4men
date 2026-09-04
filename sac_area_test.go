package main

import "testing"

// TestSACAreaCode covers the sac_area multiplier's value derivation (SAC
// rule 8.2): every prefix variant from the same Scandinavian entity and
// numeral resolves to the same value (SI3/SK3/SL3/SM3 all "Sweden-3"), a
// call with no digit gets the rule's own "0" convention, and a
// non-Scandinavian entity never contributes a value regardless of its own
// digit.
func TestSACAreaCode(t *testing.T) {
	sweden := dxccEntity{Country: "Sweden"}
	germany := dxccEntity{Country: "Germany"}
	cases := []struct {
		entity dxccEntity
		call   string
		want   string
	}{
		{sweden, "SM3XYZ", "Sweden-3"},
		{sweden, "SK3ABC", "Sweden-3"},
		{sweden, "SL3ZZZ", "Sweden-3"},
		{sweden, "SI3AAA", "Sweden-3"},
		{sweden, "SM0ABC", "Sweden-0"},
		{sweden, "SMABC", "Sweden-0"},
		{germany, "DL3ABC", ""},
		{germany, "DL0ABC", ""},
	}
	for _, c := range cases {
		if got := sacAreaCode(c.entity, c.call); got != c.want {
			t.Errorf("sacAreaCode(%+v, %q) = %q, want %q", c.entity, c.call, got, c.want)
		}
	}
}

// TestSACScandinavianCountriesHas11Values guards the exact entity count SAC
// rule 2 lists: the 5 Nordic countries plus 6 associated territories
// (Svalbard, Jan Mayen, Aland Islands, Market Reef, Greenland, Faroe
// Islands).
func TestSACScandinavianCountriesHas11Values(t *testing.T) {
	if len(sacScandinavianCountries) != 11 {
		t.Fatalf("len(sacScandinavianCountries) = %d, want 11", len(sacScandinavianCountries))
	}
}
