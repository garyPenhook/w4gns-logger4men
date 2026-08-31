package main

import (
	"fmt"
	"strconv"
	"strings"
)

// amateurBands uses internationally recognized amateur-band limits for the
// CW-only bands supported by the logger. National licences and band plans may
// be narrower, so they remain the operator's controlling authority.
var amateurBands = []struct {
	Name       string
	LowMHz     float64
	HighMHz    float64
	DefaultMHz string
}{
	{"160M", 1.800, 2.000, "1.810"},
	{"80M", 3.500, 4.000, "3.550"},
	{"60M", 5.3515, 5.3665, "5.354"},
	{"40M", 7.000, 7.300, "7.025"},
	{"30M", 10.100, 10.150, "10.110"},
	{"20M", 14.000, 14.350, "14.025"},
	{"17M", 18.068, 18.168, "18.080"},
	{"15M", 21.000, 21.450, "21.025"},
	{"12M", 24.890, 24.990, "24.905"},
	{"10M", 28.000, 29.700, "28.025"},
	{"6M", 50.000, 54.000, "50.090"},
}

func bandIndex(name string) int {
	for index, band := range amateurBands {
		if strings.EqualFold(strings.TrimSpace(name), band.Name) {
			return index
		}
	}
	return -1
}

func validateBandFrequency(band, frequency string) error {
	index := bandIndex(band)
	if index < 0 {
		return fmt.Errorf("unsupported amateur band %q", band)
	}
	value, err := strconv.ParseFloat(strings.TrimSpace(frequency), 64)
	if err != nil || value <= 0 {
		return fmt.Errorf("frequency must be entered in MHz")
	}
	allocation := amateurBands[index]
	if value < allocation.LowMHz || value > allocation.HighMHz {
		return fmt.Errorf("%.6g MHz is outside the %s amateur allocation (%.4g–%.4g MHz)", value, allocation.Name, allocation.LowMHz, allocation.HighMHz)
	}
	return nil
}
