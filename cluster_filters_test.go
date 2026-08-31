package main

import "testing"

func TestClusterFiltersDefaultToCWBands160Through6Metres(t *testing.T) {
	filters := defaultClusterFilters()
	if len(filters.Bands) != len(cwBands) {
		t.Fatalf("enabled band count = %d, want %d", len(filters.Bands), len(cwBands))
	}
	for _, band := range cwBands {
		if !filters.Bands[band] {
			t.Errorf("%s is not enabled by default", band)
		}
	}
}

func TestClusterFiltersAllowOnlySelectedAmateurBands(t *testing.T) {
	filters := defaultClusterFilters()
	filters.Bands["20M"] = false
	if filters.allowsSpot(clusterSpot{Frequency: "14025.0"}) {
		t.Fatal("disabled 20M spot was allowed")
	}
	if !filters.allowsSpot(clusterSpot{Frequency: "7025.0"}) {
		t.Fatal("enabled 40M spot was rejected")
	}
	if filters.allowsSpot(clusterSpot{Frequency: "27.185"}) {
		t.Fatal("non-amateur-band spot was allowed")
	}
}
