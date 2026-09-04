package main

import "testing"

// TestWPXPrefix covers CQ WPX Rule V.C's own worked examples plus the
// standard portable-operation cases the rule text describes.
func TestWPXPrefix(t *testing.T) {
	cases := []struct {
		call string
		want string
	}{
		// Plain calls: letter/numeral combination through the last digit
		// group that precedes the trailing suffix letters.
		{"N8ABC", "N8"},
		{"W8ABC", "W8"},
		{"WD8ABC", "WD8"},
		{"KC2ABC", "KC2"},
		{"4X1AB", "4X1"},
		// No numeral at all: first two letters + "0" (rule's own example).
		{"XEFTJW", "XE0"},
		// Portable designator without a numeral: rule's own example.
		{"PA/N8BJQ", "PA0"},
		{"N8BJQ/PA", "PA0"},
		// Numeral-only portable designator swaps in for the home call's
		// numeral.
		{"W3ABC/4", "W4"},
		// A full alternate prefix after the slash (operating from another
		// country/call area) becomes the prefix, regardless of side.
		{"VP9/W3ABC", "VP9"},
		{"W3ABC/VP9", "VP9"},
		// Non-qualifying license-class/mobile suffixes are ignored; the
		// prefix comes from the home call.
		{"N8BJQ/P", "N8"},
		{"N8BJQ/QRP", "N8"},
		{"KH6XXX/MM", "KH6"},
		{"KH6XXX/M", "KH6"},
		// Blank input.
		{"", ""},
	}
	for _, c := range cases {
		if got := wpxPrefix(c.call); got != c.want {
			t.Errorf("wpxPrefix(%q) = %q, want %q", c.call, got, c.want)
		}
	}
}
