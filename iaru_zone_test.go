package main

import "testing"

// TestIARUExchangeZone covers iaruExchangeZone's parsing of the IARU HF
// World Championship exchange (Rule 4.2): a plain positive integer is the
// worked station's ITU zone; anything else (blank, a Member Society
// abbreviation, an Official code, or a non-positive number) isn't a zone.
func TestIARUExchangeZone(t *testing.T) {
	cases := []struct {
		text string
		want int
	}{
		{"14", 14},
		{"  14  ", 14},
		{"08", 8},
		{"ARRL", 0},
		{"AC", 0},
		{"R1", 0},
		{"", 0},
		{"0", 0},
		{"-3", 0},
	}
	for _, c := range cases {
		if got := iaruExchangeZone(c.text); got != c.want {
			t.Errorf("iaruExchangeZone(%q) = %d, want %d", c.text, got, c.want)
		}
	}
}

// TestIARUExchangeSpecial covers iaruExchangeSpecial's parsing of the same
// exchange text into a Member Society (Rule 4.2.1) or Official (Rule 4.2.2)
// abbreviation: anything that isn't a plain ITU zone number, normalized to
// upper case, or "" for a blank exchange or an actual zone number.
func TestIARUExchangeSpecial(t *testing.T) {
	cases := []struct {
		text string
		want string
	}{
		{"ARRL", "ARRL"},
		{"arrl", "ARRL"},
		{"  darc  ", "DARC"},
		{"IARU", "IARU"},
		{"AC", "AC"},
		{"R1", "R1"},
		{"R2", "R2"},
		{"R3", "R3"},
		{"14", ""},
		{"", ""},
	}
	for _, c := range cases {
		if got := iaruExchangeSpecial(c.text); got != c.want {
			t.Errorf("iaruExchangeSpecial(%q) = %q, want %q", c.text, got, c.want)
		}
	}
}
