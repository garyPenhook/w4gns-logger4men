package main

import (
	"sync"
	"time"

	"w4gns-logger/internal/geo"
)

type geoCacheEntry struct {
	// location is nil when the lookup succeeded but had no usable
	// coordinate for this key (or the lookup itself failed) — still cached,
	// so a bad/missing record isn't re-queried on every subsequent spot
	// within the cache's ttl.
	location  *geo.Location
	fetchedAt time.Time
}

// geoCache is a bounded, TTL'd, dedup-on-in-flight cache from an arbitrary
// string key (a callsign or a POTA park reference) to a resolved map
// Location. Shared by the QRZ callsign cache (qrz_geo_cache.go) and the POTA
// reference cache (pota_geo_cache.go): both need identical mechanics — cache
// a lookup result (including a negative "no coordinate" result) so a busy
// contest doesn't re-query the same key for every repeat spot, dedup
// concurrent lookups for the same key, and cap total memory use over a
// long-running session.
type geoCache struct {
	mu       sync.Mutex
	capacity int
	ttl      time.Duration
	entries  map[string]geoCacheEntry
	pending  map[string]bool
}

func newGeoCache(capacity int, ttl time.Duration) *geoCache {
	return &geoCache{capacity: capacity, ttl: ttl, entries: make(map[string]geoCacheEntry), pending: make(map[string]bool)}
}

// lookup returns a cached, still-fresh result and whether an entry existed
// at all. A true, nil result means "known: the lookup had no coordinate for
// this key" — distinct from false, which means no cached answer exists yet.
func (c *geoCache) lookup(key string) (*geo.Location, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	entry, ok := c.entries[key]
	if !ok || time.Since(entry.fetchedAt) > c.ttl {
		return nil, false
	}
	return entry.location, true
}

// startIfNeeded marks key as in-flight and reports whether the caller should
// actually start a lookup command for it: false when a fresh cache entry
// already exists, a lookup for it is already pending, or the cache is at
// capacity (and key isn't already tracked).
func (c *geoCache) startIfNeeded(key string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	if entry, ok := c.entries[key]; ok && time.Since(entry.fetchedAt) <= c.ttl {
		return false
	}
	if c.pending[key] {
		return false
	}
	if _, tracked := c.entries[key]; !tracked && len(c.entries)+len(c.pending) >= c.capacity {
		return false
	}
	c.pending[key] = true
	return true
}

// store records a lookup result and clears the in-flight marker. A nil
// location (no coordinate, or the lookup failed) is still cached, so it is
// retried only after ttl rather than on every subsequent spot.
func (c *geoCache) store(key string, location *geo.Location) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.pending, key)
	if _, tracked := c.entries[key]; !tracked && len(c.entries) >= c.capacity {
		return // capacity reached while this lookup was in flight; drop it
	}
	c.entries[key] = geoCacheEntry{location: location, fetchedAt: time.Now()}
}
