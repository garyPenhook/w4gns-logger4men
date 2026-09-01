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
		{"20M", "NaN", false},
		{"20M", "Inf", false},
		{"20M", "-Inf", false},
	} {
		if err := validateBandFrequency(test.band, test.frequency); (err == nil) != test.valid {
			t.Errorf("validateBandFrequency(%q, %q) error = %v, valid = %t", test.band, test.frequency, err, test.valid)
		}
	}
}

func TestIsLikelyCWFrequency(t *testing.T) {
	for _, test := range []struct {
		band   string
		freq   float64
		wantCW bool
	}{
		{"20M", 14.025, true},
		{"20M", 14.250, false},
		{"30M", 10.140, true}, // whole band is CW/data-only, no phone
		{"6M", 50.150, true},  // no regulatory mode split above 30 MHz
		{"UNKNOWN", 14.025, false},
	} {
		if got := isLikelyCWFrequency(test.band, test.freq); got != test.wantCW {
			t.Errorf("isLikelyCWFrequency(%q, %v) = %t, want %t", test.band, test.freq, got, test.wantCW)
		}
	}
}

func TestBandAllowed(t *testing.T) {
	allowed := []string{"160M", "80M", "40M"}
	if !bandAllowed(allowed, "80m") {
		t.Error("bandAllowed should be case-insensitive")
	}
	if bandAllowed(allowed, "20M") {
		t.Error("20M should not be allowed by this list")
	}
}
