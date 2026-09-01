package main

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

// amateurBands uses internationally recognized amateur-band limits for the
// CW-only bands supported by the logger. National licences and band plans may
// be narrower, so they remain the operator's controlling authority.
//
// CWUpperMHz is the top of the conventional CW/data-only segment used to
// classify DX cluster spots as CW vs. phone/digital (see
// cluster_filters.go's allowsSpot). Below 30 MHz these follow the long-stable
// US Amateur Extra-class privileges (47 CFR §97.305), where FCC rules
// codify a CW/data-only sub-band on every HF band except 60M (channelized,
// no continuous sub-band) and 30M (no phone permitted anywhere in the band,
// so the whole allocation is CW/data). Above 30 MHz (6M) there is no
// regulatory mode split; CWUpperMHz is set to HighMHz so the cluster filter
// does not reject anything on that band. Other license classes and other
// countries' band plans differ — this is a best-effort default for a
// CW-focused station, not a substitute for the operator's own authority.
var amateurBands = []struct {
	Name       string
	LowMHz     float64
	HighMHz    float64
	DefaultMHz string
	CWUpperMHz float64
}{
	{"160M", 1.800, 2.000, "1.810", 1.840},
	{"80M", 3.500, 4.000, "3.550", 3.600},
	{"60M", 5.3515, 5.3665, "5.354", 5.3665},
	{"40M", 7.000, 7.300, "7.025", 7.125},
	{"30M", 10.100, 10.150, "10.110", 10.150},
	{"20M", 14.000, 14.350, "14.025", 14.150},
	{"17M", 18.068, 18.168, "18.080", 18.110},
	{"15M", 21.000, 21.450, "21.025", 21.200},
	{"12M", 24.890, 24.990, "24.905", 24.930},
	{"10M", 28.000, 29.700, "28.025", 28.300},
	{"6M", 50.000, 54.000, "50.090", 54.000},
}

// isLikelyCWFrequency reports whether freqMHz falls within the conventional
// CW/data-only segment of the given band (see amateurBands.CWUpperMHz).
// Returns false for an unknown band.
func isLikelyCWFrequency(band string, freqMHz float64) bool {
	index := bandIndex(band)
	if index < 0 {
		return false
	}
	allocation := amateurBands[index]
	return freqMHz >= allocation.LowMHz && freqMHz <= allocation.CWUpperMHz
}

// bandAllowed reports whether band is in an event's allowed-bands list
// (case-insensitive). An empty list is treated as "no restriction" by
// callers, not by this function.
func bandAllowed(allowed []string, band string) bool {
	for _, candidate := range allowed {
		if strings.EqualFold(strings.TrimSpace(candidate), band) {
			return true
		}
	}
	return false
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
	// NaN and Inf pass strconv.ParseFloat (e.g. the literal "NaN") and compare
	// false against every bound below, which would otherwise let them slip
	// through as "in band."
	if err != nil || math.IsNaN(value) || math.IsInf(value, 0) || value <= 0 {
		return fmt.Errorf("frequency must be entered in MHz")
	}
	allocation := amateurBands[index]
	if value < allocation.LowMHz || value > allocation.HighMHz {
		return fmt.Errorf("%.6g MHz is outside the %s amateur allocation (%.4g–%.4g MHz)", value, allocation.Name, allocation.LowMHz, allocation.HighMHz)
	}
	return nil
}
