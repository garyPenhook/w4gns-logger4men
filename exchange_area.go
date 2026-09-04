package main

import "strings"

// exchangeAreaCodes is the "exchange_area" multiplier kind's canonical value
// set: the 48 contiguous US states, the District of Columbia, and the 14
// Canadian provinces/territories (Newfoundland and Labrador counted
// separately "for reasons of tradition"). This is the multiplier list CQ 160
// Meter CW's rules (cq160.com/rules, "MULTIPLIER") award uniformly to every
// entrant — not just DX stations — for each distinct US state/DC/province
// worked, alongside the DXCC-entity multiplier already wired for that event.
// It is also the same 63-value list ARRL DX CW Rule 5.2.2 awards to DX-side
// entrants specifically (as opposed to W/VE-side entrants, who instead count
// DXCC entities per 5.2.1) — that asymmetric wiring is a separate, still-open
// gap (docs/ROADMAP.md) because this app's own station profile is W/VE, and
// applying this multiplier to ARRL DX CW's existing W/VE-side scoring block
// would incorrectly award mults for a rule that doesn't apply to that side.
// Alaska and Hawaii are deliberately absent: both contests' state/province
// side excludes them (ARRL DX Rules 1.2.1.1; CQ 160's "48 contiguous
// states").
var exchangeAreaCodes = map[string]bool{
	// US states (48 contiguous) + DC.
	"AL": true, "AZ": true, "AR": true, "CA": true, "CO": true, "CT": true,
	"DE": true, "FL": true, "GA": true, "ID": true, "IL": true, "IN": true,
	"IA": true, "KS": true, "KY": true, "LA": true, "ME": true, "MD": true,
	"MA": true, "MI": true, "MN": true, "MS": true, "MO": true, "MT": true,
	"NE": true, "NV": true, "NH": true, "NJ": true, "NM": true, "NY": true,
	"NC": true, "ND": true, "OH": true, "OK": true, "OR": true, "PA": true,
	"RI": true, "SC": true, "SD": true, "TN": true, "TX": true, "UT": true,
	"VT": true, "VA": true, "WA": true, "WV": true, "WI": true, "WY": true,
	"DC": true,
	// Canadian provinces/territories (14 — NL and LB counted separately).
	"AB": true, "BC": true, "MB": true, "NB": true, "NL": true, "LB": true,
	"NS": true, "NT": true, "NU": true, "ON": true, "PE": true, "QC": true,
	"SK": true, "YT": true,
}

// exchangeAreaAliases maps a received-exchange spelling variant to its
// canonical exchangeAreaCodes key. ARRL DX CW's own rule text spells
// Newfoundland/Labrador's traditional abbreviation "NF" rather than the
// postal code "NL" this table otherwise uses.
var exchangeAreaAliases = map[string]string{
	"NF": "NL",
}

// exchangeAreaCode extracts the exchange_area multiplier value from a
// received-exchange text, or "" if it doesn't match a known US
// state/DC/Canadian province code — e.g. a DX station's power exchange
// ("100W", "5") never coincidentally matches one of these two-letter codes,
// so this is safe to apply unconditionally to any received exchange text
// rather than needing the contest's own side-detection.
func exchangeAreaCode(text string) string {
	code := strings.ToUpper(strings.TrimSpace(text))
	if canonical, ok := exchangeAreaAliases[code]; ok {
		code = canonical
	}
	if exchangeAreaCodes[code] {
		return code
	}
	return ""
}
