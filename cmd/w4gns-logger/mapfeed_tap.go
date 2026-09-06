package main

import (
	"strings"
	"time"

	"w4gns-logger/internal/geo"
	"w4gns-logger/internal/mapfeed"
	"w4gns-logger/internal/spot"
)

// mapFeedBands is the baseline band set used by the map feed tap. Unlike the
// terminal's clusterFilters.Bands (which the operator can narrow), the map
// feed always evaluates every CW band so that narrowing the terminal's band
// filter doesn't silently narrow what the map ever saw — see
// isBaselineCWEligible.
var mapFeedBands = defaultClusterFilters().Bands

// buildMapReport converts an already-baseline-eligible cluster spot into a
// spot.Report. SpotterLocation resolves via resolveMapLocation (a cached QRZ
// profile coordinate when available, otherwise the DXCC country/prefix
// reference). DXLocation additionally prefers a POTA park coordinate when
// the spot's comment names a reference — see resolveDXLocation. A callsign
// that resolves via none of these gets a nil Location rather than a guessed
// one.
func buildMapReport(cspot clusterSpot, band string, freqMHz float64, qrzCache, potaCache *geoCache) spot.Report {
	return spot.Report{
		ReceivedAtUTC:   cspot.Received.UTC(),
		DXCall:          cspot.Callsign,
		SpotterCall:     cspot.Spotter,
		FrequencyHz:     spot.FrequencyHzFromMHz(freqMHz),
		Band:            band,
		Comment:         cspot.Comment,
		DXLocation:      resolveDXLocation(cspot, qrzCache, potaCache),
		SpotterLocation: resolveMapLocation(cspot.Spotter, qrzCache),
	}
}

// resolveDXLocation prefers a cached POTA park coordinate when cspot's
// comment names a POTA reference — the DX station's current activation
// site, more relevant and more precise than its permanent QRZ home address
// for this report — falling back to resolveMapLocation's QRZ/country-
// reference precedence otherwise. POTA applies only to the DX/activator
// side of a spot: the spotter is presumably at their own location, not the
// activator's park, matching how recentClusterPOTAReference (pota.go) is
// likewise only ever applied to the worked/spotted call.
func resolveDXLocation(cspot clusterSpot, qrzCache, potaCache *geoCache) *geo.Location {
	if potaCache != nil {
		if reference, ok := potaReferenceFromComment(cspot.Comment); ok {
			if loc, ok := potaCache.lookup(reference); ok && loc != nil {
				return loc
			}
		}
	}
	return resolveMapLocation(cspot.Callsign, qrzCache)
}

// potaReferenceFromComment extracts a POTA park reference from a single
// cluster spot's comment, applying the same "skip an IOTA-shaped match"
// exclusion recentClusterPOTAReference (pota.go) uses when scanning spot
// history — potaReferencePattern's shape also matches IOTA island-group
// references (e.g. "EU-005"), so a comment carrying both must not report the
// IOTA reference as if it were a POTA one.
func potaReferenceFromComment(comment string) (string, bool) {
	for _, candidate := range potaReferencePattern.FindAllString(comment, -1) {
		if !iotaReferencePattern.MatchString(candidate) {
			return strings.ToUpper(candidate), true
		}
	}
	return "", false
}

// resolveMapLocation prefers a fresh, cached QRZ profile coordinate for call
// (see qrzGeoCache) — more precise than the country reference — falling
// back to the country/prefix reference table when the cache has no entry or
// a cached miss (QRZ reached but had no usable coordinate).
func resolveMapLocation(call string, qrzCache *geoCache) *geo.Location {
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
	if !record.hasLatLon || !geo.ValidCoordinates(record.latitude, record.longitude) {
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

// potaRecordLocation converts a POTA park lookup result into a map
// Location, or nil when the record carried no usable coordinate (see
// potaParkRecord.hasCoordinates) — POTA already uses the standard north/
// east-positive convention, so no sign conversion is needed here. Country is
// normalized to the exact string "United States" when EntityName names it
// (POTA's "United States of America" wouldn't otherwise match the DX/USA map
// filter, which compares against the same exact string cty.dat and QRZ
// already produce); other entities keep POTA's own EntityName as a
// best-effort display label, at the cost of not participating in that
// filter — not worth a full prefix-to-country table for.
func potaRecordLocation(record potaParkRecord) *geo.Location {
	if !record.hasCoordinates() {
		return nil
	}
	country := record.EntityName
	if strings.Contains(country, "United States") {
		country = "United States"
	}
	return &geo.Location{
		Country:    country,
		Latitude:   record.Latitude,
		Longitude:  record.Longitude,
		Source:     geo.SourcePOTAPark,
		Precision:  geo.PrecisionPOTAPark,
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

// pendingPOTASpot is a map-eligible cluster spot held back because its
// comment names a POTA reference not yet in potaGeoCache — see
// model.pendingPOTASpots.
type pendingPOTASpot struct {
	cspot   clusterSpot
	band    string
	freqMHz float64
}

// potaPendingSpotCap bounds how many spots can queue behind a single
// unresolved POTA reference before the map feed gives up waiting and adds
// them with whatever fallback location resolveDXLocation finds instead —
// protects against unbounded growth if a park lookup hangs near its
// potaParkLookupTimeout during a busy pileup on one activation.
const potaPendingSpotCap = 25

// Bound all waiting references together, including a burst of distinct parks.
const potaPendingTotalCap = 500

// enrichQRZReports replaces only fallback/profile locations. A park, explicit
// override, or report-attributed grid remains more authoritative than QRZ.
func enrichQRZReports(reports *mapfeed.Store, call string, loc *geo.Location) {
	if reports == nil || loc == nil {
		return
	}
	call = normalizeCall(call)
	eligible := func(old *geo.Location) bool {
		return old == nil || old.Source == geo.SourceCountryReference || old.Source == geo.SourceQRZProfile
	}
	reports.UpdateLocations(func(r spot.Report) (*geo.Location, *geo.Location) {
		dx, spotter := r.DXLocation, r.SpotterLocation
		if normalizeCall(r.DXCall) == call && eligible(dx) {
			dx = loc
		}
		if normalizeCall(r.SpotterCall) == call && eligible(spotter) {
			spotter = loc
		}
		return dx, spotter
	})
}

// Late park results also enrich spots published as fallbacks when the pending
// queue filled or timed out. Match the reference on each report, not the call:
// one station can activate different parks during the retained history.
func enrichPOTAReports(reports *mapfeed.Store, reference string, loc *geo.Location) {
	if reports == nil || loc == nil {
		return
	}
	reports.UpdateLocations(func(r spot.Report) (*geo.Location, *geo.Location) {
		dx := r.DXLocation
		if ref, ok := potaReferenceFromComment(r.Comment); ok && ref == reference &&
			(dx == nil || dx.Source == geo.SourceCountryReference || dx.Source == geo.SourceQRZProfile || dx.Source == geo.SourcePOTAPark) {
			dx = loc
		}
		return dx, r.SpotterLocation
	})
}
