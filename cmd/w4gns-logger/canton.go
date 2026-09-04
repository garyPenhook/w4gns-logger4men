package main

import "strings"

// cantonCodes is the "canton" multiplier kind's canonical value set: the
// 26 official two-letter Swiss canton abbreviations from the Helvetia
// Contest rules (uska.ch, "Rules and Regulations for Helvetia Contest",
// issued March 2026, §2.11). Only an HB9 (Swiss) station sends a canton in
// its exchange (§2.5.1: RS(T) + canton); every other participant sends a
// running serial number (§2.5.2, digits only), which never coincidentally
// matches one of these two-letter codes — the same "safe to apply
// unconditionally" property tn_county.go's tnCountyCode documents.
var cantonCodes = map[string]bool{
	"AG": true, "AI": true, "AR": true, "BE": true, "BL": true,
	"BS": true, "FR": true, "GE": true, "GL": true, "GR": true,
	"JU": true, "LU": true, "NE": true, "NW": true, "OW": true,
	"SG": true, "SH": true, "SO": true, "SZ": true, "TG": true,
	"TI": true, "UR": true, "VD": true, "VS": true, "ZG": true,
	"ZH": true,
}

// cantonCode extracts the "canton" multiplier value from a received-exchange
// text, or "" if it doesn't match one of the 26 official canton codes —
// mirrors tn_county.go's tnCountyCode (whole-text match, since the canton is
// the only content of a Swiss station's exchange after RS(T)).
func cantonCode(text string) string {
	code := strings.ToUpper(strings.TrimSpace(text))
	if cantonCodes[code] {
		return code
	}
	return ""
}
