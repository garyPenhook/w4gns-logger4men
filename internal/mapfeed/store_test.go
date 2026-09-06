package mapfeed

import (
	"sync"
	"testing"
	"time"

	"w4gns-logger/internal/geo"
	"w4gns-logger/internal/spot"
)

func TestStoreAddAssignsIncreasingEventIDs(t *testing.T) {
	s := NewStore(10)
	id1 := s.Add(spot.Report{DXCall: "W1AW"})
	id2 := s.Add(spot.Report{DXCall: "K1ABC"})
	if id1 != 1 || id2 != 2 {
		t.Fatalf("event IDs = %d, %d; want 1, 2", id1, id2)
	}
	snap := s.Snapshot()
	if len(snap) != 2 {
		t.Fatalf("Snapshot len = %d, want 2", len(snap))
	}
	if snap[0].SessionID == "" || snap[0].SessionID != snap[1].SessionID {
		t.Errorf("expected shared non-empty session ID, got %q and %q", snap[0].SessionID, snap[1].SessionID)
	}
	if snap[0].SchemaVersion != spot.SchemaVersion {
		t.Errorf("SchemaVersion = %d, want %d", snap[0].SchemaVersion, spot.SchemaVersion)
	}
}

func TestChangesIncludeEnrichmentAndRetainReportIdentity(t *testing.T) {
	s := NewStore(2)
	at := time.Now()
	id := s.Add(spot.Report{DXCall: "W1AW", ReceivedAtUTC: at})
	s.Add(spot.Report{DXCall: "K1ABC", ReceivedAtUTC: at})
	before := s.ChangesSince(0)
	loc := &geo.Location{Latitude: 41.7, Longitude: -72.7, Source: geo.SourceQRZProfile}
	s.UpdateLocations(func(r spot.Report) (*geo.Location, *geo.Location) {
		if r.DXCall == "W1AW" {
			return loc, r.SpotterLocation
		}
		return r.DXLocation, r.SpotterLocation
	})
	after := s.ChangesSince(before.Revision)
	if len(after.Reports) != 1 || after.Reports[0].EventID != id || after.Reports[0].ReceivedAtUTC != at || after.Reports[0].DXLocation != loc || !after.AtCapacity || after.OldestID != id {
		t.Fatalf("enrichment delta: %+v", after)
	}
	if before.Reports[0].DXLocation != nil || len(s.ChangesSince(after.Revision).Reports) != 0 {
		t.Fatal("snapshot mutated or delta repeated")
	}
	s.Add(spot.Report{DXCall: "W3LPL", ReceivedAtUTC: at})
	evicted := s.ChangesSince(after.Revision)
	if evicted.OldestID != id+1 || len(evicted.Reports) != 1 || evicted.Reports[0].DXCall != "W3LPL" {
		t.Fatalf("eviction delta: %+v", evicted)
	}
	s.Expire(at.Add(time.Second))
	empty := s.ChangesSince(evicted.Revision)
	if len(empty.Reports) != 0 || empty.AtCapacity || empty.OldestID <= evicted.Reports[0].EventID {
		t.Fatalf("empty store did not retire browser IDs: %+v", empty)
	}
}

func TestConcurrentEnrichmentAndSnapshots(t *testing.T) {
	s := NewStore(20)
	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 30; j++ {
				s.Add(spot.Report{DXCall: "W1AW", ReceivedAtUTC: time.Now()})
				loc := &geo.Location{Latitude: float64(j), Longitude: 1}
				s.UpdateLocations(func(r spot.Report) (*geo.Location, *geo.Location) { return loc, r.SpotterLocation })
				for _, r := range s.Snapshot() {
					if r.DXLocation != nil {
						_ = r.DXLocation.Latitude
					}
				}
			}
		}()
	}
	wg.Wait()
	if s.Len() != 20 {
		t.Fatal("concurrent updates changed retention capacity")
	}
}

func TestStoreEvictsOldestOverCapacity(t *testing.T) {
	s := NewStore(2)
	s.Add(spot.Report{DXCall: "A"})
	s.Add(spot.Report{DXCall: "B"})
	s.Add(spot.Report{DXCall: "C"})
	snap := s.Snapshot()
	if len(snap) != 2 {
		t.Fatalf("Snapshot len = %d, want 2", len(snap))
	}
	if snap[0].DXCall != "B" || snap[1].DXCall != "C" {
		t.Errorf("Snapshot = %+v, want oldest evicted (B, C)", snap)
	}
}

func TestStoreConcurrentAdd(t *testing.T) {
	s := NewStore(1000)
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			s.Add(spot.Report{DXCall: "W1AW"})
		}()
	}
	wg.Wait()
	if got := s.Len(); got != 100 {
		t.Fatalf("Len() = %d, want 100", got)
	}
}
