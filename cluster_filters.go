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
	// DECallArea is a comma-separated list of call-area digits (e.g. "2,3,4")
	// matched against the spotting station's own callsign, not the DXCC
	// table — call areas aren't part of the cty.dat entity data.
	DECallArea string
	Bands      map[string]bool
}

func defaultClusterFilters() clusterFilters {
	filters := clusterFilters{Bands: make(map[string]bool, len(cwBands))}
	for _, band := range cwBands {
		filters.Bands[band] = true
	}
	return filters
}

// allowsSpot reports whether a cluster spot passes the band filter and the
// DX (worked station, spot.Callsign) / DE (spotting station, spot.Spotter)
// country/ITU-zone/CQ-zone/continent filters. Any filter field left blank is
// not applied. When a filter field is set but the corresponding callsign
// can't be resolved against the DXCC table, the spot is rejected rather than
// let through unfiltered.
func (f clusterFilters) allowsSpot(spot clusterSpot) bool {
	band, freqMHz, ok := bandForFrequency(spot.Frequency)
	if !ok || !f.Bands[band] {
		return false
	}
	// This is a CW-only logger, so spots inside the conventional phone/digital
	// portion of the band are not relevant even if the band itself is enabled.
	if !isLikelyCWFrequency(band, freqMHz) {
		return false
	}
	if !f.matchesEntityFilters(spot.Callsign, f.DXCC, f.DXITUZone, f.DXCQZone, f.DXContinent) {
		return false
	}
	if !f.matchesEntityFilters(spot.Spotter, f.DECC, f.DEITUZone, f.DECQZone, f.DEContinent) {
		return false
	}
	if !matchesCallArea(spot.Spotter, f.DECallArea) {
		return false
	}
	return true
}

// matchesCallArea reports whether call's call-area digit is one of the
// comma-separated digits in areas. A blank areas filter always matches.
// When areas is set but no call-area digit can be found in call, the spot
// is rejected rather than let through unfiltered.
func matchesCallArea(call, areas string) bool {
	areas = strings.TrimSpace(areas)
	if areas == "" {
		return true
	}
	digit, ok := callAreaDigit(call)
	if !ok {
		return false
	}
	for _, want := range strings.Split(areas, ",") {
		want = strings.TrimSpace(want)
		if len(want) == 1 && want[0] == digit {
			return true
		}
	}
	return false
}

// callAreaDigit extracts the call-area digit from a callsign. A portable
// suffix that is purely numeric (e.g. "W1AW/4") overrides the base call's
// own digit, matching standard amateur radio portable-operation notation.
// Otherwise it returns the first digit found in the (longest, if the call
// has slash segments) base callsign.
func callAreaDigit(call string) (byte, bool) {
	call = strings.ToUpper(strings.TrimSpace(call))
	if call == "" {
		return 0, false
	}
	parts := strings.Split(call, "/")
	for _, part := range parts {
		if part != "" && isAllDigits(part) {
			return part[len(part)-1], true
		}
	}
	base := parts[0]
	for _, part := range parts[1:] {
		if len(part) > len(base) {
			base = part
		}
	}
	for i := 0; i < len(base); i++ {
		if base[i] >= '0' && base[i] <= '9' {
			return base[i], true
		}
	}
	return 0, false
}

func isAllDigits(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}

// matchesEntityFilters resolves call against the DXCC table only if at least
// one of country/ituZone/cqZone/continent is non-blank, then checks each
// non-blank filter against the resolved entity.
func (f clusterFilters) matchesEntityFilters(call, country, ituZone, cqZone, continent string) bool {
	country, ituZone, cqZone, continent = strings.TrimSpace(country), strings.TrimSpace(ituZone), strings.TrimSpace(cqZone), strings.TrimSpace(continent)
	if country == "" && ituZone == "" && cqZone == "" && continent == "" {
		return true
	}
	table, err := sharedDXCCTable()
	if err != nil {
		return false
	}
	entity, ok := table.lookup(call)
	if !ok {
		return false
	}
	if country != "" && !strings.Contains(strings.ToUpper(entity.Country), strings.ToUpper(country)) {
		return false
	}
	if ituZone != "" {
		zone, err := strconv.Atoi(ituZone)
		if err != nil || zone != entity.ITUZone {
			return false
		}
	}
	if cqZone != "" {
		zone, err := strconv.Atoi(cqZone)
		if err != nil || zone != entity.CQZone {
			return false
		}
	}
	if continent != "" && !strings.EqualFold(continent, entity.Continent) {
		return false
	}
	return true
}

// bandForFrequency normalizes frequency (which cluster nodes report in
// either kHz or MHz) and returns its band name and the normalized MHz value.
func bandForFrequency(frequency string) (string, float64, bool) {
	freq, err := strconv.ParseFloat(strings.TrimSpace(frequency), 64)
	if err != nil || freq <= 0 {
		return "", 0, false
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
			return allocation.Name, freq, true
		}
	}
	return "", 0, false
}
