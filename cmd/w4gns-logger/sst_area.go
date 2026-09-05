package main

import "strings"

// sstAreaCode extracts the "sst_area" multiplier kind's value from a
// received-exchange text for K1USN's Slow Speed Test (SST). Sourced from the
// K1USN SST Rules (linked from k1usn.com/sst_rules.html, the rules document
// itself is a Google Doc embedded in that page): "SCORING 1 point for each
// QSO regardless of QTH. Multipliers are the sum of States, Provinces and
// DXCC Countries. No DXCC credit for the USA lower 48 States or Canada —
// QSO's just [get] State or Province (applies to USA/Canada/all DX). DXCC
// Multiplier for stations worked outside the USA lower 48 states and Canada
// (applies to USA/Canada and all DX)." Unlike naqpAreaCode's own DXCC
// fallback (which credits only North-American entities beyond the
// US/Canada state table, per NAQP Rule 11), SST's DXCC multiplier is
// worldwide — any entity outside the USA/Canada counts — so the fallback
// here has no continent restriction. SST's sent_exchange_hint ("First name +
// state/province/DX country") is the same "Name + location" single
// free-text shape NAQP CW uses, so — like naqpAreaCode — only the last
// whitespace-separated token is checked, and the same 50-state/DC/
// 13-province table (naqpAreaCodes/naqpAreaAliases, naqp_area.go) is reused
// since SST's own rules give no separate state/province table of their own.
func sstAreaCode(text string) string {
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
	if !found {
		return ""
	}
	switch entity.Country {
	case "", "United States", "Canada":
		return ""
	}
	return entity.Country
}
