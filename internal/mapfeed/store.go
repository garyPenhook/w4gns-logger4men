// Package mapfeed owns the companion map's in-memory report retention. It is
// the single synchronization boundary between the goroutine that observes
// cluster spots and any later consumer (browser snapshot/SSE) that reads
// them, so a producer never blocks on a slow or absent reader.
package mapfeed

import (
	"crypto/rand"
	"encoding/hex"
	"sync"
	"time"

	"w4gns-logger/internal/spot"
)

// Store is a bounded, concurrency-safe collection of spot.Report values. It
// assigns each added report an increasing EventID and a shared SessionID, and
// evicts the oldest report once Capacity is reached. The map server invokes
// Expire during streaming, including when the cluster is disconnected.
// Marker aggregation is performed by the browser, preserving raw reports.
type Store struct {
	mu        sync.Mutex
	capacity  int
	sessionID string
	nextEvent int64
	reports   []spot.Report
}

// Expire removes reports outside the retention window even when reception stops.
func (s *Store) Expire(before time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	kept := s.reports[:0]
	for _, r := range s.reports {
		if !r.ReceivedAtUTC.Before(before) {
			kept = append(kept, r)
		}
	}
	clear(s.reports[len(kept):])
	s.reports = kept
}

// NewStore creates a Store bounded to capacity reports. A new random
// SessionID is generated per Store so a client can tell a restarted feed
// apart from a continued one.
func NewStore(capacity int) *Store {
	if capacity < 1 {
		capacity = 1
	}
	return &Store{
		capacity:  capacity,
		sessionID: newSessionID(),
	}
}

// Add assigns report the next EventID and this Store's SessionID, appends it,
// and evicts the oldest report if the Store is over capacity. It returns the
// assigned EventID.
func (s *Store) Add(report spot.Report) int64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.nextEvent++
	report.SchemaVersion = spot.SchemaVersion
	report.SessionID = s.sessionID
	report.EventID = s.nextEvent
	s.reports = append(s.reports, report)
	if len(s.reports) > s.capacity {
		s.reports = s.reports[len(s.reports)-s.capacity:]
	}
	return report.EventID
}

// Snapshot returns a copy of the currently retained reports, oldest first.
// The returned slice is safe to read without further synchronization.
func (s *Store) Snapshot() []spot.Report {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]spot.Report, len(s.reports))
	copy(out, s.reports)
	return out
}

// Len reports how many reports are currently retained.
func (s *Store) Len() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.reports)
}

// SessionID returns this Store's per-process session identifier.
func (s *Store) SessionID() string {
	return s.sessionID
}

func newSessionID() string {
	buf := make([]byte, 8)
	if _, err := rand.Read(buf); err != nil {
		// crypto/rand.Read failing means the OS entropy source is broken;
		// a fixed fallback keeps the store usable rather than panicking on
		// a purely informational identifier.
		return "session-fallback"
	}
	return hex.EncodeToString(buf)
}
