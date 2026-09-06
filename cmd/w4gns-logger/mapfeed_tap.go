package main

import (
	"time"

	"w4gns-logger/internal/geo"
	"w4gns-logger/internal/spot"
)

// mapFeedBands is the baseline band set used by the map feed tap. Unlike the
// terminal's clusterFilters.Bands (which the operator can narrow), the map
// feed always evaluates every CW band so that narrowing the terminal's band
// filter doesn't silently narrow what the map ever saw — see
// isBaselineCWEligible.
var mapFeedBands = defaultClusterFilters().Bands

// buildMapReport converts an already-baseline-eligible cluster spot into a
// spot.Report, resolving both endpoints' locations via the bundled DXCC
// country/prefix reference table. A callsign that doesn't resolve, or
// resolves to an entity with no reference coordinate (see
// dxccEntity.HasCoordinates), gets a nil Location rather than a guessed one.
func buildMapReport(cspot clusterSpot, band string, freqMHz float64) spot.Report {
	return spot.Report{
		ReceivedAtUTC:   cspot.Received.UTC(),
		DXCall:          cspot.Callsign,
		SpotterCall:     cspot.Spotter,
		FrequencyHz:     spot.FrequencyHzFromMHz(freqMHz),
		Band:            band,
		Comment:         cspot.Comment,
		DXLocation:      countryReferenceLocation(cspot.Callsign),
		SpotterLocation: countryReferenceLocation(cspot.Spotter),
	}
}

// countryReferenceLocation resolves call to its DXCC entity's reference
// coordinate, labeled as an approximate country/prefix reference per the
// design's location-honesty requirement. It returns nil when the call can't
// be resolved or the resolved entity carries no coordinate.
func countryReferenceLocation(call string) *geo.Location {
	table, err := sharedDXCCTable()
	if err != nil {
		return nil
	}
	entity, ok := table.lookup(call)
	if !ok || !entity.HasCoordinates() {
		return nil
	}
	return &geo.Location{
		Country:    entity.Country,
		Latitude:   entity.Latitude,
		Longitude:  entity.Longitude,
		Source:     geo.SourceCountryReference,
		Precision:  geo.PrecisionCountryReference,
		ResolvedAt: time.Now().UTC(),
	}
}
