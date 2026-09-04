package main

import (
	"strconv"
	"strings"
)

// iaruExchangeZone parses the IARU HF World Championship's received-exchange
// text (Rule 4.2: "Signal report" + "ITU Zone, IARU Society Abbreviation, or
// IARU Official") into the ITU zone number the worked station actually sent,
// or 0 if the text isn't a plain positive integer — i.e. it's a Member
// Society or Official abbreviation instead (iaruExchangeSpecial). Reads the
// exchanged value directly rather than resolving the worked callsign's zone
// via cty.dat, matching the exchange-driven precedent set by
// exchange_area.go/tn_county.go/naqp_area.go/arrl_section.go: the contest
// scores what was actually copied, not a callsign-derived guess.
func iaruExchangeZone(text string) int {
	text = strings.ToUpper(strings.TrimSpace(text))
	if text == "" {
		return 0
	}
	value, err := strconv.Atoi(text)
	if err != nil || value <= 0 {
		return 0
	}
	return value
}

// iaruExchangeSpecial parses the same exchange text into an IARU Member
// Society abbreviation (Rule 4.2.1, e.g. "ARRL", or "IARU" for the
// International Secretariat's NU1AW) or Official code (Rule 4.2.2: "AC",
// "R1", "R2", "R3") — anything that isn't a plain ITU zone number. Unlike
// arrl_section.go/tn_county.go's fixed abbreviation tables, IARU has roughly
// 160 member societies with no single canonical machine-readable list in
// this repo, so any non-numeric token is accepted as a practical
// approximation, the same class as wpxPrefix's non-exhaustive call-format
// handling.
func iaruExchangeSpecial(text string) string {
	text = strings.ToUpper(strings.TrimSpace(text))
	if text == "" || iaruExchangeZone(text) > 0 {
		return ""
	}
	return text
}
