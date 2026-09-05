package main

import "strings"

// dokDistrictCode extracts the "dok_district" multiplier value — the
// district letter (first letter of a German DOK) — from a received-exchange
// text, or "" if it isn't a German DOK. Sourced from the WAG rules'
// districts/DOKs service page (darc.de, "Districts, DOKs and a mysterious
// multiplier"): "the regular DARC/VFDB districts allow for 25 multipliers
// per band" (every letter A-Z except J), plus a documented 26th "mysterious
// multiplier" from rare special DOKs that do start with J — so unlike
// canton.go/tn_county.go's fixed value sets, no letter is actually excluded
// here; any letter A-Z is a valid district code.
//
// A non-German station's exchange is a plain running serial number
// (digits only, WAG rules §4), and a non-member German station explicitly
// sends "NM" rather than a DOK ("will be no multiplier" per the rules) — the
// first character of "NM" happens to be the real district letter N
// (Nordrhein-Westfalen), so "NM" must be excluded by name rather than
// relying on the digit-vs-letter check alone.
func dokDistrictCode(text string) string {
	code := strings.ToUpper(strings.TrimSpace(text))
	if code == "" || code == "NM" {
		return ""
	}
	first := rune(code[0])
	if first < 'A' || first > 'Z' {
		return ""
	}
	return string(first)
}
