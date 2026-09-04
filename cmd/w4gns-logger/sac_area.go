package main

import "strings"

// sacScandinavianCountries is SAC's own definition of "Scandinavia" (rules
// section 2, sactest.net): the five Nordic countries plus the territories
// the rules list by their own callsign prefix block — Svalbard & Bear
// Island (JW), Jan Mayen (JX), the Åland Islands and Market Reef (both
// under Finland's OH-prefix block), Greenland (OX/XP), and the Faroe
// Islands (OW/OY) — given here as the cty.dat dxccEntity.Country names
// those prefixes resolve to (data/cty.dat). Shared by the "sac_area"
// multiplier kind (which worked stations count at all for a non-Scandinavian
// entrant) and events/contestcalendar.json's SAC-CW domestic_countries
// (which side of SAC's asymmetric scoring an operator's own station uses).
var sacScandinavianCountries = []string{
	"Norway", "Finland", "Sweden", "Iceland", "Denmark",
	"Svalbard", "Jan Mayen", "Aland Islands", "Market Reef",
	"Greenland", "Faroe Islands",
}

// sacAreaCode derives the "sac_area" multiplier kind's value for a worked
// call resolved to entity: SAC rule 8.2 ("Each worked prefix-number (Ø-9) in
// each Scandinavian DXCC entity is valid for one multiplier on each band")
// — every prefix variant from the same entity and numeral counts as one
// multiplier (SI3/SK3/SL3/SM3 all resolve to the same "Sweden-3" value per
// the rule's own examples), and a call with no digit is the 0th area per the
// rule's own convention. Returns "" for a call that doesn't resolve to one
// of the Scandinavian entities — a non-Scandinavian worked station is never
// a multiplier under this kind.
func sacAreaCode(entity dxccEntity, call string) string {
	if !countryInList(sacScandinavianCountries, entity.Country) {
		return ""
	}
	numeral := "0"
	for _, r := range strings.ToUpper(strings.TrimSpace(call)) {
		if r >= '0' && r <= '9' {
			numeral = string(r)
			break
		}
	}
	return entity.Country + "-" + numeral
}
