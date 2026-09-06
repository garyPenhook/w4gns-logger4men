package main

import (
	"sync"
	"time"

	"w4gns-logger/internal/geo"
)

// qrzGeoCacheCapacity bounds how many distinct callsigns' QRZ-derived
// locations the map feed will cache per run; beyond this, newly seen calls
// fall back to the country reference instead of ever being looked up, so a
// very high-cardinality contest can't grow this without bound (mirrors
// mapReportsCapacity's bound on the report store itself).
const qrzGeoCacheCapacity = 20000

// qrzGeoTTL is how long a cached QRZ result (hit or miss) is trusted before
// a later spot of the same call re-queries it. Long enough that a busy
// contest doesn't re-query a station spotted repeatedly within the hour;
// short enough that a stale profile or a transient lookup failure doesn't
// stick for the life of a long-running session.
const qrzGeoTTL = 12 * time.Hour

type qrzGeoEntry struct {
	// location is nil when QRZ was reached but had no usable coordinate for
	// this call (or the lookup failed) — still cached, so a bad/missing
	// record isn't re-queried on every subsequent spot within qrzGeoTTL.
	location  *geo.Location
	fetchedAt time.Time
}

// qrzGeoCache is the map feed's per-callsign cache of QRZ-derived locations.
// It is deliberately separate from the single in-flight QSO Entry auto-fill
// lookup (model.qrzLookups/qrzActiveLookup): many distinct callsigns can be
// in flight here at once, one per newly seen cluster call, unrelated to
// whatever call is currently being typed into QSO Entry.
type qrzGeoCache struct {
	mu      sync.Mutex
	entries map[string]qrzGeoEntry
	pending map[string]bool
}

func newQRZGeoCache() *qrzGeoCache {
	return &qrzGeoCache{entries: make(map[string]qrzGeoEntry), pending: make(map[string]bool)}
}

// lookup returns a cached, still-fresh result and whether an entry existed
// at all. A true, nil result means "known: QRZ had no coordinate for this
// call" — distinct from false, which means no cached answer exists yet.
func (c *qrzGeoCache) lookup(call string) (*geo.Location, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	entry, ok := c.entries[call]
	if !ok || time.Since(entry.fetchedAt) > qrzGeoTTL {
		return nil, false
	}
	return entry.location, true
}

// startIfNeeded marks call as in-flight and reports whether the caller
// should actually start a lookup command for it: false when a fresh cache
// entry already exists, a lookup for it is already pending, or the cache is
// at capacity (and call isn't already tracked).
func (c *qrzGeoCache) startIfNeeded(call string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	if entry, ok := c.entries[call]; ok && time.Since(entry.fetchedAt) <= qrzGeoTTL {
		return false
	}
	if c.pending[call] {
		return false
	}
	if _, tracked := c.entries[call]; !tracked && len(c.entries)+len(c.pending) >= qrzGeoCacheCapacity {
		return false
	}
	c.pending[call] = true
	return true
}

// store records a lookup result and clears the in-flight marker. A nil
// location (no coordinate, or the lookup failed) is still cached, so it is
// retried only after qrzGeoTTL rather than on every subsequent spot.
func (c *qrzGeoCache) store(call string, location *geo.Location) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.pending, call)
	if _, tracked := c.entries[call]; !tracked && len(c.entries) >= qrzGeoCacheCapacity {
		return // capacity reached while this lookup was in flight; drop it
	}
	c.entries[call] = qrzGeoEntry{location: location, fetchedAt: time.Now()}
}
