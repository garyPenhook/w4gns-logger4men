package main

import (
	"testing"
	"time"

	"w4gns-logger/internal/geo"
)

func TestQRZGeoCacheStartIfNeededDedupsPending(t *testing.T) {
	c := newQRZGeoCache()
	if !c.startIfNeeded("W1AW") {
		t.Fatal("first startIfNeeded(W1AW) = false, want true")
	}
	if c.startIfNeeded("W1AW") {
		t.Fatal("second startIfNeeded(W1AW) while pending = true, want false (already in flight)")
	}
	if _, ok := c.lookup("W1AW"); ok {
		t.Fatal("lookup(W1AW) while pending = found, want not yet cached")
	}
}

func TestQRZGeoCacheStoreThenLookup(t *testing.T) {
	c := newQRZGeoCache()
	c.startIfNeeded("W1AW")
	loc := &geo.Location{Latitude: 41.7, Longitude: -72.7, Source: geo.SourceQRZProfile}
	c.store("W1AW", loc)

	got, ok := c.lookup("W1AW")
	if !ok || got != loc {
		t.Fatalf("lookup(W1AW) = %v, %v; want %v, true", got, ok, loc)
	}
	// A fresh entry means no further lookup should be started.
	if c.startIfNeeded("W1AW") {
		t.Fatal("startIfNeeded(W1AW) with a fresh cached entry = true, want false")
	}
}

// TestQRZGeoCacheStoreNilIsStillCached guards a QRZ miss (reached, but no
// coordinate, or the lookup failed) being cached too — otherwise every spot
// of a station QRZ can't locate would retry the lookup forever.
func TestQRZGeoCacheStoreNilIsStillCached(t *testing.T) {
	c := newQRZGeoCache()
	c.startIfNeeded("N0CALL")
	c.store("N0CALL", nil)

	got, ok := c.lookup("N0CALL")
	if !ok || got != nil {
		t.Fatalf("lookup(N0CALL) = %v, %v; want nil, true (cached miss)", got, ok)
	}
	if c.startIfNeeded("N0CALL") {
		t.Fatal("startIfNeeded(N0CALL) with a fresh cached miss = true, want false")
	}
}

func TestQRZGeoCacheTTLExpires(t *testing.T) {
	c := newQRZGeoCache()
	c.entries["W1AW"] = geoCacheEntry{
		location:  &geo.Location{Latitude: 41.7, Longitude: -72.7},
		fetchedAt: time.Now().Add(-(qrzGeoTTL + time.Minute)),
	}
	if _, ok := c.lookup("W1AW"); ok {
		t.Fatal("lookup(W1AW) with an expired entry = found, want expired (not fresh)")
	}
	if !c.startIfNeeded("W1AW") {
		t.Fatal("startIfNeeded(W1AW) with an expired entry = false, want true (should re-query)")
	}
}

func TestQRZGeoCacheCapacityCapsNewEntries(t *testing.T) {
	c := newQRZGeoCache()
	for i := 0; i < qrzGeoCacheCapacity; i++ {
		call := formatSerial(i + 1)
		c.entries[call] = geoCacheEntry{fetchedAt: time.Now()}
	}
	if c.startIfNeeded("OVERFLOW") {
		t.Fatal("startIfNeeded at capacity for a new call = true, want false")
	}
	c.store("OVERFLOW", &geo.Location{})
	if _, ok := c.lookup("OVERFLOW"); ok {
		t.Fatal("store() at capacity for a never-tracked call should not add it")
	}
}
