package main

import "strings"

// rdxcOblastCodes is the "oblast" multiplier kind's canonical value set: the
// Russian DX Contest's own official oblast abbreviation table (rdxc.org,
// "RULES ENG", the "Name/Abbr/Px/Sx/CQ/ITU/Cont/Lat/Lon/Sunrise/Sunset" list
// referenced by Section 6.3's "signal report + oblast code as per attached
// list" and Section 9's "one multiplier for each different oblast contacted
// on each band"). Keyed by the 2-letter abbreviation only — several oblasts
// span more than one prefix-number/suffix-letter partition in the source
// table (e.g. Moskovskaya obl. "MO" appears under both R2,3,5/D,H and
// R3,5/F), which collapse to one code here since the multiplier is per
// oblast, not per partition. Section 7.3's Kaliningrad ("KA"), Franz Josef
// Land ("FJ"), and (Russian) Antarctic ("AN") stations are already ordinary
// rows in the source table, not a separate special-case list.
var rdxcOblastCodes = map[string]bool{
	"SP": true, "LO": true, "KO": true, "KL": true, "AR": true,
	"NO": true, "VO": true, "NV": true, "PS": true, "MU": true,
	"KA": true, "MA": true, "MO": true, "OR": true, "LP": true,
	"TV": true, "VR": true, "SM": true, "YR": true, "KS": true,
	"TL": true, "TB": true, "RA": true, "NN": true, "IV": true,
	"VL": true, "KU": true, "KG": true, "BR": true, "BO": true,
	"VG": true, "SA": true, "PE": true, "SR": true, "UL": true,
	"KI": true, "TA": true, "MR": true, "MD": true, "UD": true,
	"CU": true, "KR": true, "KC": true, "ST": true, "KM": true,
	"SO": true, "RK": true, "RO": true, "CN": true, "IN": true,
	"SE": true, "AO": true, "DA": true, "KB": true, "AD": true,
	"DO": true, "HE": true, "LU": true, "ZP": true, "CB": true,
	"SV": true, "PM": true, "TO": true, "HM": true, "YN": true,
	"TN": true, "OM": true, "NS": true, "KN": true, "OB": true,
	"KE": true, "BA": true, "AL": true, "GA": true, "KK": true,
	"HK": true, "EA": true, "SL": true, "MG": true, "AM": true,
	"CK": true, "PK": true, "BU": true, "YA": true, "IR": true,
	"ZK": true, "HA": true, "TU": true, "KT": true, "FJ": true,
	"AN": true,
}

// rdxcOblastCode extracts the "oblast" multiplier value from a received-
// exchange text, or "" if it doesn't match one of the RDXC oblast codes —
// mirrors canton.go's cantonCode (whole-text match, since the oblast is the
// only content of a Russian station's exchange after RS(T); a non-Russian
// station's exchange is a running serial number, digits only, which never
// coincidentally matches a 2-letter code).
func rdxcOblastCode(text string) string {
	code := strings.ToUpper(strings.TrimSpace(text))
	if rdxcOblastCodes[code] {
		return code
	}
	return ""
}
