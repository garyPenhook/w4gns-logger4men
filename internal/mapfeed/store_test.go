package mapfeed

import (
	"sync"
	"testing"

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
