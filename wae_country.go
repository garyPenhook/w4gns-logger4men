package main

import (
	"sort"
	"strings"
)

// waeCountries is the WAE Country List (darc.de WAEDC rules, Section 6 and
// the rules page's own "WAE Country List" appendix): the set of DXCC
// entities counted as "Europe" for the WAE DX Contest, keyed by this app's
// cty.dat dxccEntity.Country name rather than the rules page's own prefix
// tokens. Resolving by country name (via the same dxccTable.lookup every
// other multiplier kind already uses) means portable/alias call handling
// stays consistent with the rest of the app instead of a second parser.
//
// Built by resolving every WAE Country List prefix token through this app's
// embedded cty.dat: most tokens map 1:1 to a distinct entity, but a few fold
// together under this app's data — 4U1V (Vienna Intl Ctr) has no separate
// cty.dat entity and resolves to Austria (same as OE), so it contributes no
// additional country beyond OE's. The rules page's "GM/s" (Shetland Islands)
// and "JW/b" (Bear Island) sub-designators are marked non-DXCC ("*") in
// cty.dat and are not separately resolvable from an ordinary callsign, so
// they are treated as their parent entities (Scotland, Svalbard) — the same
// practical-approximation class as wpxPrefix's non-exhaustive call handling.
var waeCountries = map[string]struct{}{
	"Sov Mil Order of Malta": {},
	"Monaco":                 {},
	"Montenegro":             {},
	"ITU HQ":                 {},
	"Austria":                {},
	"Croatia":                {},
	"Malta":                  {},
	"Andorra":                {},
	"Portugal":               {},
	"Azores":                 {},
	"Fed. Rep. of Germany":   {},
	"Bosnia-Herzegovina":     {},
	"Spain":                  {},
	"Balearic Islands":       {},
	"Ireland":                {},
	"Moldova":                {},
	"Estonia":                {},
	"Belarus":                {},
	"France":                 {},
	"England":                {},
	"Isle of Man":            {},
	"Northern Ireland":       {},
	"Jersey":                 {},
	"Scotland":               {},
	"Guernsey":               {},
	"Wales":                  {},
	"Hungary":                {},
	"Switzerland":            {},
	"Liechtenstein":          {},
	"Vatican City":           {},
	"Italy":                  {},
	"Sardinia":               {},
	"Svalbard":               {},
	"Jan Mayen":              {},
	"Norway":                 {},
	"Luxembourg":             {},
	"Lithuania":              {},
	"Bulgaria":               {},
	"Finland":                {},
	"Aland Islands":          {},
	"Market Reef":            {},
	"Czech Republic":         {},
	"Slovak Republic":        {},
	"Belgium":                {},
	"Faroe Islands":          {},
	"Denmark":                {},
	"Netherlands":            {},
	"European Russia":        {},
	"Kaliningrad":            {},
	"Slovenia":               {},
	"Sweden":                 {},
	"Poland":                 {},
	"Greece":                 {},
	"Dodecanese":             {},
	"Crete":                  {},
	"San Marino":             {},
	"European Turkey":        {},
	"Iceland":                {},
	"Corsica":                {},
	"Ukraine":                {},
	"Latvia":                 {},
	"Romania":                {},
	"Serbia":                 {},
	"Republic of Kosovo":     {},
	"North Macedonia":        {},
	"Albania":                {},
	"Gibraltar":              {},
	"Franz Josef Land":       {},
	"Mount Athos":            {},
}

// isWAECountry reports whether country (a cty.dat dxccEntity.Country value)
// is a member of the WAE Country List, case-insensitively.
func isWAECountry(country string) bool {
	country = strings.TrimSpace(country)
	if country == "" {
		return false
	}
	for candidate := range waeCountries {
		if strings.EqualFold(candidate, country) {
			return true
		}
	}
	return false
}

// waeCountryNames returns the WAE Country List's country names, sorted, for
// use as an eventDefinition.DomesticCountries value — WAE's own "domestic"
// side (Section 5's "European station") from the contest's own perspective,
// not this app's usual W/VE-station-is-domestic convention (see
// events/contestcalendar.json's DARC-WAEDC-CW comment).
func waeCountryNames() []string {
	names := make([]string, 0, len(waeCountries))
	for name := range waeCountries {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// waeBandBonus is Section 6's per-band multiplier weighting ("Multiply the
// number of countries worked on 3.5 MHz by four, on 7 MHz by three, and on
// 14/21/28 MHz by two") applied uniformly to whichever side's multiplier
// count (WAE Country List for a non-European entrant, DXCC entities for a
// European entrant). Any band outside WAE's own band plan (80/40/20/15/10M)
// contributes zero rather than guessing a weight.
func waeBandBonus(band string) int {
	switch strings.ToUpper(strings.TrimSpace(band)) {
	case "80M":
		return 4
	case "40M":
		return 3
	case "20M", "15M", "10M":
		return 2
	default:
		return 0
	}
}
