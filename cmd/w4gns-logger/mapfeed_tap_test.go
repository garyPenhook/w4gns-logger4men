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

func TestPotaReferenceFromComment(t *testing.T) {
	if ref, ok := potaReferenceFromComment("QRP FROM K-1234 TNX"); !ok || ref != "K-1234" {
		t.Fatalf("potaReferenceFromComment = %q, %v, want K-1234, true", ref, ok)
	}
	// potaReferencePattern also matches IOTA-shaped tokens; a comment naming
	// both must report the genuine POTA reference, not the IOTA one.
	if ref, ok := potaReferenceFromComment("EU-005 also POTA K-1234"); !ok || ref != "K-1234" {
		t.Fatalf("potaReferenceFromComment(IOTA+POTA) = %q, %v, want K-1234, true", ref, ok)
	}
	if _, ok := potaReferenceFromComment("EU-005 IOTA only"); ok {
		t.Fatal("potaReferenceFromComment matched an IOTA-only comment")
	}
	if _, ok := potaReferenceFromComment("CQ CQ CW"); ok {
		t.Fatal("potaReferenceFromComment matched plain text")
	}
}

func TestPotaRecordLocationRequiresCoordinates(t *testing.T) {
	if loc := potaRecordLocation(potaParkRecord{Name: "Pea Ridge"}); loc != nil {
		t.Fatalf("potaRecordLocation without coordinates = %+v, want nil", loc)
	}
	record := potaParkRecord{Latitude: 35.9307, Longitude: -85.9401, EntityName: "United States of America"}
	loc := potaRecordLocation(record)
	if loc == nil || loc.Latitude != 35.9307 || loc.Longitude != -85.9401 || loc.Source != geo.SourcePOTAPark || loc.Country != "United States" {
		t.Fatalf("potaRecordLocation(%+v) = %+v, want lat 35.9307/lon -85.9401/SourcePOTAPark/United States", record, loc)
	}
}

func TestPotaRecordLocationKeepsForeignEntityName(t *testing.T) {
	record := potaParkRecord{Latitude: 51.5, Longitude: -0.1, EntityName: "England"}
	loc := potaRecordLocation(record)
	if loc == nil || loc.Country != "England" {
		t.Fatalf("potaRecordLocation(%+v).Country = %q, want %q", record, loc.Country, "England")
	}
}

// TestResolveDXLocationPrefersPOTAOverQRZ is the crux of this feature: a
// cached POTA park location for a reference named in the spot's comment must
// win over a cached QRZ profile location for the same callsign.
func TestResolveDXLocationPrefersPOTAOverQRZ(t *testing.T) {
	qrzCache := newQRZGeoCache()
	qrzCache.startIfNeeded("W4GNS")
	qrzCache.store("W4GNS", &geo.Location{Latitude: 1, Longitude: 1, Source: geo.SourceQRZProfile})

	potaCache := newPOTAGeoCache()
	potaCache.startIfNeeded("K-1234")
	potaLoc := &geo.Location{Latitude: 35.9, Longitude: -85.9, Source: geo.SourcePOTAPark}
	potaCache.store("K-1234", potaLoc)

	cspot := clusterSpot{Callsign: "W4GNS", Comment: "QRP FROM K-1234"}
	got := resolveDXLocation(cspot, qrzCache, potaCache)
	if got != potaLoc {
		t.Fatalf("resolveDXLocation = %+v, want the cached POTA location %+v", got, potaLoc)
	}
}

// TestResolveDXLocationFallsBackWithoutPOTAReference covers the common case
// (no POTA reference in the comment) still using the QRZ/country-reference
// precedence resolveMapLocation provides.
func TestResolveDXLocationFallsBackWithoutPOTAReference(t *testing.T) {
	potaCache := newPOTAGeoCache()
	cspot := clusterSpot{Callsign: "W4GNS", Comment: "CQ CQ CW"}
	got := resolveDXLocation(cspot, nil, potaCache)
	if got == nil || got.Source != geo.SourceCountryReference {
		t.Fatalf("resolveDXLocation without a POTA reference = %+v, want a country-reference location", got)
	}
}
