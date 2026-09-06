package main

import "time"

// potaGeoCacheCapacity bounds how many distinct POTA park references the map
// feed will cache per run, mirroring qrzGeoCacheCapacity.
const potaGeoCacheCapacity = 20000

// potaGeoTTL is how long a cached POTA park lookup is trusted. Park
// coordinates themselves are effectively permanent, so this only bounds how
// often a busy activation gets re-queried, not "freshness of activation" —
// that comes from the comment-reference match on each live spot, not this
// cache.
const potaGeoTTL = 12 * time.Hour

// newPOTAGeoCache creates the map feed's per-reference cache of POTA park
// locations (see geoCache).
func newPOTAGeoCache() *geoCache {
	return newGeoCache(potaGeoCacheCapacity, potaGeoTTL)
}
