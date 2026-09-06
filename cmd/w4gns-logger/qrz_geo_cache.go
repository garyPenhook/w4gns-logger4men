package main

import "time"

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

// newQRZGeoCache creates the map feed's per-callsign cache of QRZ-derived
// locations (see geoCache). It is deliberately separate from the single
// in-flight QSO Entry auto-fill lookup (model.qrzLookups/qrzActiveLookup):
// many distinct callsigns can be in flight here at once, one per newly seen
// cluster call, unrelated to whatever call is currently being typed into
// QSO Entry.
func newQRZGeoCache() *geoCache {
	return newGeoCache(qrzGeoCacheCapacity, qrzGeoTTL)
}
