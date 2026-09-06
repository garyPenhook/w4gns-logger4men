package main

import (
	"testing"

	"w4gns-logger/internal/geo"
)

// TestResolveMapLocationPrefersCachedQRZOverCountryReference is the crux of
// the "QRZ location unless something more precise" requirement: a fresh,
// non-nil cached QRZ location must win over the country/prefix reference,
// even though the call resolves fine there too.
func TestResolveMapLocationPrefersCachedQRZOverCountryReference(t *testing.T) {
	cache := newQRZGeoCache()
	cache.startIfNeeded("W4GNS")
	qrzLoc := &geo.Location{Latitude: 35.1, Longitude: -85.2, Source: geo.SourceQRZProfile}
	cache.store("W4GNS", qrzLoc)

	got := resolveMapLocation("W4GNS", cache)
	if got != qrzLoc {
		t.Fatalf("resolveMapLocation = %+v, want the cached QRZ location %+v", got, qrzLoc)
	}
}

// TestResolveMapLocationFallsBackToCountryReference covers both "no cache"
// and "cached miss" — either must fall through to the country reference,
// not silently produce no location for a resolvable call.
func TestResolveMapLocationFallsBackToCountryReference(t *testing.T) {
	if got := resolveMapLocation("W4GNS", nil); got == nil || got.Source != geo.SourceCountryReference {
		t.Fatalf("resolveMapLocation with nil cache = %+v, want a country-reference location", got)
	}

	cache := newQRZGeoCache()
	cache.startIfNeeded("W4GNS")
	cache.store("W4GNS", nil) // cached miss: QRZ reached, no coordinate
	if got := resolveMapLocation("W4GNS", cache); got == nil || got.Source != geo.SourceCountryReference {
		t.Fatalf("resolveMapLocation with a cached miss = %+v, want a country-reference location", got)
	}
}

func TestQRZRecordLocationRequiresLatLon(t *testing.T) {
	if loc := qrzRecordLocation(qrzCallsignRecord{name: "Fred"}); loc != nil {
		t.Fatalf("qrzRecordLocation without hasLatLon = %+v, want nil", loc)
	}
	record := qrzCallsignRecord{hasLatLon: true, latitude: 35.1, longitude: -85.2, country: "United States"}
	loc := qrzRecordLocation(record)
	if loc == nil || loc.Latitude != 35.1 || loc.Longitude != -85.2 || loc.Source != geo.SourceQRZProfile || loc.Country != "United States" {
		t.Fatalf("qrzRecordLocation(%+v) = %+v, want lat 35.1/lon -85.2/SourceQRZProfile/United States", record, loc)
	}
}
