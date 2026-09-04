package main

import "strings"

// wpxNonQualifyingSuffixes are the portable/mobile/license-class tokens CQ
// WPX Rule V.C says never form a prefix on their own ("Maritime mobile,
// mobile, /A, /E, /J, /P, or other license class identifiers do not count as
// prefixes"): when one of these appears after a "/", it's dropped and the
// prefix is derived from the call's other side instead.
var wpxNonQualifyingSuffixes = map[string]bool{
	"M": true, "MM": true, "AM": true, "A": true, "E": true, "J": true, "P": true,
	"QRP": true, "QRPP": true,
}

// wpxPrefix derives the CQ WPX contest prefix multiplier value for call,
// applying CQ WPX Rule V.C: "the letter/numeral combination which forms the
// first part of the amateur call", with a portable operation's designator
// (an authorized prefix of the country/call area actually being operated
// from) replacing the home call's prefix, a numeral-only designator (e.g.
// "/4") swapping in for the home prefix's own numeral, and a no-numeral call
// or designator getting a trailing "0" per the rule's own examples
// (PA/N8BJQ -> PA0, XEFTJW -> XE0). This is a practical implementation of
// the rule, not exhaustive for exotic combinations (e.g. two full calls
// joined by "/", or nonstandard call formats) — the same class of edge case
// dxccTable.lookup already documents.
func wpxPrefix(call string) string {
	call = strings.ToUpper(strings.TrimSpace(call))
	if call == "" {
		return ""
	}
	segments := strings.Split(call, "/")
	var kept []string
	for _, seg := range segments {
		if seg == "" || wpxNonQualifyingSuffixes[seg] {
			continue
		}
		kept = append(kept, seg)
	}
	if len(kept) == 0 {
		// Every segment was a non-qualifying suffix (shouldn't happen for a
		// real call, but fall back to the raw first segment rather than
		// returning nothing).
		kept = []string{segments[0]}
	}
	if len(kept) == 1 {
		return wpxPrefixFromSegment(kept[0])
	}
	// More than one candidate segment remains (e.g. "W3ABC/VP9" or
	// "VP9/W3ABC"): the shorter segment is the portable designator per the
	// rule's own examples (PA/N8BJQ -> the 2-character "PA" side, not the
	// 5-character home call). Prefer the later segment on a length tie,
	// matching the common "home/location" call ordering.
	portable, home := kept[0], kept[1]
	if len(kept[1]) < len(kept[0]) {
		portable, home = kept[1], kept[0]
	}
	if isDigitsOnly(portable) {
		return wpxPrefixLetters(home) + portable
	}
	return wpxPrefixFromSegment(portable)
}

// wpxPrefixFromSegment applies the base (no-slash) prefix rule to one call
// segment: the leading run through the last digit group that's still
// followed by a trailing letter suffix (e.g. "4X1AB" -> "4X1",
// "WD8ABC" -> "WD8"), or the first two characters plus "0" when the segment
// has no digit at all (e.g. "XEFTJW" -> "XE0", the rule's own example).
func wpxPrefixFromSegment(seg string) string {
	trimmed := strings.TrimRight(seg, "ABCDEFGHIJKLMNOPQRSTUVWXYZ")
	if trimmed == seg {
		// Nothing trimmed: seg already ends in a digit, so it's already in
		// prefix form (no trailing letter suffix to strip).
		return seg
	}
	if trimmed != "" {
		return trimmed
	}
	letters := seg
	if len(letters) > 2 {
		letters = letters[:2]
	}
	return letters + "0"
}

// wpxPrefixLetters returns just the leading letters of a home call's prefix
// (e.g. "W3ABC" -> "W"), the part a numeral-only portable designator (e.g.
// "/4") replaces the numeral of.
func wpxPrefixLetters(seg string) string {
	for i, r := range seg {
		if r >= '0' && r <= '9' {
			return seg[:i]
		}
	}
	return seg
}

func isDigitsOnly(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}
