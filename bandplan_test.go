package main

import "testing"

func TestValidateBandFrequency(t *testing.T) {
	for _, test := range []struct {
		band      string
		frequency string
		valid     bool
	}{
		{"20M", "14.025", true},
		{"60M", "5.354", true},
		{"20M", "7.025", false},
		{"UNKNOWN", "14.025", false},
		{"20M", "not-a-frequency", false},
	} {
		if err := validateBandFrequency(test.band, test.frequency); (err == nil) != test.valid {
			t.Errorf("validateBandFrequency(%q, %q) error = %v, valid = %t", test.band, test.frequency, err, test.valid)
		}
	}
}
