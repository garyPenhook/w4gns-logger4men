package main

import (
	"strconv"
	"strings"
)

var cwBands = []string{"160M", "80M", "60M", "40M", "30M", "20M", "17M", "15M", "12M", "10M", "6M"}

type clusterFilters struct {
	DXCC        string
	DXITUZone   string
	DXCQZone    string
	DXContinent string
	DECC        string
	DEITUZone   string
	DECQZone    string
	DEContinent string
	Bands       map[string]bool
}

func defaultClusterFilters() clusterFilters {
	filters := clusterFilters{Bands: make(map[string]bool, len(cwBands))}
	for _, band := range cwBands {
		filters.Bands[band] = true
	}
	return filters
}

func (f clusterFilters) allowsSpot(spot clusterSpot) bool {
	band, ok := bandForFrequency(spot.Frequency)
	return ok && f.Bands[band]
}

func bandForFrequency(frequency string) (string, bool) {
	freq, err := strconv.ParseFloat(strings.TrimSpace(frequency), 64)
	if err != nil || freq <= 0 {
		return "", false
	}
	// DX clusters commonly report HF frequencies in kHz (for example 14025.0)
	// while some nodes use MHz. Normalize both forms before band matching.
	if freq >= 1000 {
		freq /= 1000
	}
	// Band edges come from amateurBands (bandplan.go) so the cluster filter
	// and QSO-entry validation always agree on what belongs to a band.
	for _, allocation := range amateurBands {
		if freq >= allocation.LowMHz && freq <= allocation.HighMHz {
			return allocation.Name, true
		}
	}
	return "", false
}
