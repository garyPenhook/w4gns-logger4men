package main

import "strings"

// naqpAreaCodes is the "naqp_area" multiplier kind's canonical value set:
// NAQP Rule 11's (ncjweb.com/NAQP-Rules.pdf) list of all 50 US states
// (including Alaska and Hawaii), the District of Columbia, and the 13
// Canadian provinces/territories — unlike exchange_area.go's CQ 160/ARRL DX
// table, NAQP includes Alaska/Hawaii and combines Newfoundland and Labrador
// into a single "Newfoundland-Labrador" entry rather than counting them
// separately.
var naqpAreaCodes = map[string]bool{
	"AL": true, "AK": true, "AZ": true, "AR": true, "CA": true, "CO": true,
	"CT": true, "DE": true, "FL": true, "GA": true, "HI": true, "ID": true,
	"IL": true, "IN": true, "IA": true, "KS": true, "KY": true, "LA": true,
	"ME": true, "MD": true, "MA": true, "MI": true, "MN": true, "MS": true,
	"MO": true, "MT": true, "NE": true, "NV": true, "NH": true, "NJ": true,
	"NM": true, "NY": true, "NC": true, "ND": true, "OH": true, "OK": true,
	"OR": true, "PA": true, "RI": true, "SC": true, "SD": true, "TN": true,
	"TX": true, "UT": true, "VT": true, "VA": true, "WA": true, "WV": true,
	"WI": true, "WY": true, "DC": true,
	"AB": true, "BC": true, "MB": true, "NB": true, "NL": true, "NS": true,
	"NT": true, "NU": true, "ON": true, "PE": true, "QC": true, "SK": true,
	"YT": true,
}

// naqpAreaAliases maps a received-exchange spelling variant to its canonical
// naqpAreaCodes key. "NF" is the traditional abbreviation for Newfoundland
// and "LB" for Labrador; NAQP Rule 11 lists them as one combined
// "Newfoundland-Labrador" multiplier, unlike CQ 160/ARRL DX's exchange_area
// table, which counts them separately.
var naqpAreaAliases = map[string]string{
	"NF": "NL",
	"LB": "NL",
}

// naqpAreaCode extracts the naqp_area multiplier value from a received-
// exchange text. NAQP's exchange is "Name + location" typed into this app's
// single free-text exchange field (events/contestcalendar.json's
// received_exchange_hint), so — unlike exchange_area.go/tn_county.go, whose
// contests exchange location alone — only the last whitespace-separated
// token is checked, matching the conventional "name, then location" typing
// order. A hit against the 50-state/DC/13-province table above is returned
// directly. Otherwise, per Rule 11's "for other North American entities,
// please use the standard DXCC prefix ... in the received location field,"
// the token is resolved as a DXCC prefix (dxccTable.lookup does plain
// string-prefix matching, so a bare prefix like "XE" or "KP4" resolves the
// same way a full callsign would); a hit that is itself in North America but
// not the United States or Canada (already handled above) counts as a
// multiplier keyed by its country name. Anything else — no token, an
// unresolved prefix, or a non-NA entity — contributes no multiplier.
func naqpAreaCode(text string) string {
	fields := strings.Fields(text)
	if len(fields) == 0 {
		return ""
	}
	code := strings.ToUpper(fields[len(fields)-1])
	if canonical, ok := naqpAreaAliases[code]; ok {
		code = canonical
	}
	if naqpAreaCodes[code] {
		return code
	}
	table, err := sharedDXCCTable()
	if err != nil {
		return ""
	}
	entity, found := table.lookup(code)
	if !found || entity.Continent != "NA" {
		return ""
	}
	switch entity.Country {
	case "", "United States", "Canada":
		return ""
	}
	return entity.Country
}
