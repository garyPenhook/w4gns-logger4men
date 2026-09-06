// Package spot defines the normalized cluster-report contract shared between
// the terminal logger and its companion map. A Report is deliberately a
// plain, storage-agnostic value: it carries no database, credential, or UI
// state, so it is safe to hand to a browser-facing consumer.
package spot

import (
	"math"
	"time"

	"w4gns-logger/internal/geo"
)

// SchemaVersion is the version of the Report contract below. Bump it when
// the field set or meaning changes so a future standalone consumer can tell
// snapshots apart.
const SchemaVersion = 1

// Report is one normalized DX-cluster report. FrequencyHz is the normalized
// value; the original as-reported frequency string is kept by the caller's
// display layer, not here. SourceTimeUTC stays nil until the cluster line
// format reliably carries its own timestamp — ReceivedAtUTC (local receipt
// time) is otherwise the only trustworthy time for a report.
type Report struct {
	SchemaVersion   int
	SessionID       string
	EventID         int64
	ReceivedAtUTC   time.Time
	DXCall          string
	SpotterCall     string
	FrequencyHz     int64
	Band            string
	Comment         string
	DXLocation      *geo.Location
	SpotterLocation *geo.Location
	SourceTimeUTC   *time.Time
}

// FrequencyHzFromMHz converts a frequency already normalized to MHz (as
// produced by the cluster band parser) into an integer Hz value.
func FrequencyHzFromMHz(mhz float64) int64 {
	return int64(math.Round(mhz * 1_000_000))
}
