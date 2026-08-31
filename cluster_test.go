package main

import (
	"testing"
	"time"
)

func TestParseClusterSpot(t *testing.T) {
	when := time.Date(2026, time.August, 31, 20, 0, 0, 0, time.UTC)
	spot, ok := parseClusterSpot("DX de K3LR-1:  14025.0 ea8abc CQ CQ 2000Z", when)
	if !ok {
		t.Fatalf("parseClusterSpot did not parse a DX spot")
	}
	if spot.Spotter != "K3LR-1" || spot.Frequency != "14025.0" || spot.Callsign != "EA8ABC" {
		t.Errorf("spot = %#v", spot)
	}
	if spot.Received != when {
		t.Errorf("Received = %v, want %v", spot.Received, when)
	}
}

func TestK3LRDefaultEndpoint(t *testing.T) {
	if k3lrClusterAddr != "dx.k3lr.com:23" {
		t.Errorf("K3LR endpoint = %q, want dx.k3lr.com:23", k3lrClusterAddr)
	}
}

func TestParseClusterSpotRejectsNonSpot(t *testing.T) {
	if _, ok := parseClusterSpot("Welcome to the K3LR DX Cluster", time.Now()); ok {
		t.Fatal("parseClusterSpot accepted a banner line")
	}
}
