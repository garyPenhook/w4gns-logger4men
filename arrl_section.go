package main

import "strings"

// arrlSectionCodes is the "arrl_section" multiplier kind's canonical value
// set: the 85 ARRL/RAC section abbreviations from the ARRL/RAC Section
// Abbreviation List (arrl.org/files/file/Field-Day/Generic/ARRL-RAC%20Section%20List.pdf,
// revised 2025) that ARRL Sweepstakes Rule 5.2 scores as multipliers ("ARRL
// and RAC Sections you contacted").
var arrlSectionCodes = map[string]bool{
	// Call Area 1
	"CT": true, "RI": true, "EMA": true, "VT": true, "ME": true, "WMA": true, "NH": true,
	// Call Area 2
	"ENY": true, "NNY": true, "NLI": true, "SNJ": true, "NNJ": true, "WNY": true,
	// Call Area 3
	"DE": true, "MDC": true, "EPA": true, "WPA": true,
	// Call Area 4
	"AL": true, "SFL": true, "GA": true, "TN": true, "KY": true, "VA": true,
	"NC": true, "WCF": true, "NFL": true, "PR": true, "SC": true, "VI": true,
	// Call Area 5
	"AR": true, "NTX": true, "LA": true, "OK": true, "MS": true, "STX": true,
	"NM": true, "WTX": true,
	// Call Area 6
	"EB": true, "SDG": true, "LAX": true, "SF": true, "ORG": true, "SJV": true,
	"SB": true, "SV": true, "SCV": true, "PAC": true,
	// Call Area 7
	"AK": true, "NV": true, "AZ": true, "OR": true, "EWA": true, "UT": true,
	"ID": true, "WWA": true, "MT": true, "WY": true,
	// Call Area 8
	"MI": true, "WV": true, "OH": true,
	// Call Area 9
	"IL": true, "WI": true, "IN": true,
	// Call Area 0
	"CO": true, "MO": true, "IA": true, "NE": true, "KS": true, "ND": true,
	"MN": true, "SD": true,
	// Canada
	"AB": true, "ONE": true, "BC": true, "ONN": true, "GH": true, "ONS": true,
	"MB": true, "PE": true, "NB": true, "QC": true, "NL": true, "SK": true,
	"NS": true, "TER": true,
}

// arrlSectionAliases maps a received-exchange spelling variant to its
// canonical arrlSectionCodes key: "GTA" (Greater Toronto Area) was renamed
// "Golden Horseshoe"/GH and "NT" (Northwest Territories) was folded into the
// combined "Territories"/TER section, but both older abbreviations remain in
// common contest-logger use.
var arrlSectionAliases = map[string]string{
	"GTA": "GH",
	"NT":  "TER",
}

// arrlSectionCode extracts the arrl_section multiplier value from a received-
// exchange text. ARRL Sweepstakes' full exchange is serial + precedence +
// call + check + section (contest_state.go's srx already holds the serial
// separately, same as every other serial-number contest here), so — mirroring
// naqp_area.go's "single free-text field holds more than one exchange
// element" precedent — only the last whitespace-separated token is checked,
// matching the rules' own required send order ("Serial ... Precedence ...
// Call ... Check ... Section"). Anything else — no token or a code outside
// the 85-section table — contributes no multiplier.
func arrlSectionCode(text string) string {
	fields := strings.Fields(text)
	if len(fields) == 0 {
		return ""
	}
	code := strings.ToUpper(fields[len(fields)-1])
	if canonical, ok := arrlSectionAliases[code]; ok {
		code = canonical
	}
	if arrlSectionCodes[code] {
		return code
	}
	return ""
}
