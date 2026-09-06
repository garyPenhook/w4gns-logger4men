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
// spot.Report, resolving both endpoints' locations via resolveMapLocation —
// a cached QRZ profile coordinate when one is available (see qrzGeoCache),
// otherwise the bundled DXCC country/prefix reference table. A callsign
// that resolves via neither gets a nil Location rather than a guessed one.
func buildMapReport(cspot clusterSpot, band string, freqMHz float64, qrzCache *qrzGeoCache) spot.Report {
	return spot.Report{
		ReceivedAtUTC:   cspot.Received.UTC(),
		DXCall:          cspot.Callsign,
		SpotterCall:     cspot.Spotter,
		FrequencyHz:     spot.FrequencyHzFromMHz(freqMHz),
		Band:            band,
		Comment:         cspot.Comment,
		DXLocation:      resolveMapLocation(cspot.Callsign, qrzCache),
		SpotterLocation: resolveMapLocation(cspot.Spotter, qrzCache),
	}
}

// resolveMapLocation prefers a fresh, cached QRZ profile coordinate for call
// (see qrzGeoCache) — more precise than the country reference — falling
// back to the country/prefix reference table when the cache has no entry or
// a cached miss (QRZ reached but had no usable coordinate).
func resolveMapLocation(call string, qrzCache *qrzGeoCache) *geo.Location {
	if qrzCache != nil {
		if loc, ok := qrzCache.lookup(normalizeCall(call)); ok && loc != nil {
			return loc
		}
	}
	return countryReferenceLocation(call)
}

// qrzRecordLocation converts a QRZ XML lookup result into a map Location,
// or nil when the record carried no usable coordinate (see
// qrzCallsignRecord.hasLatLon) — QRZ already uses the standard north/east-
// positive convention, so no sign conversion is needed here.
func qrzRecordLocation(record qrzCallsignRecord) *geo.Location {
	if !record.hasLatLon {
		return nil
	}
	return &geo.Location{
		Country:    record.country,
		Latitude:   record.latitude,
		Longitude:  record.longitude,
		Source:     geo.SourceQRZProfile,
		Precision:  geo.PrecisionQRZProfile,
		ResolvedAt: time.Now().UTC(),
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
