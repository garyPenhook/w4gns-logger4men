package main

import (
	"strings"
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

// TestParseClusterSpotStripsControlCharacters guards against a spot's
// comment (or other fields) smuggling an ANSI escape/OSC sequence into the
// terminal: cluster spots come from other operators on the network, and
// this app renders them directly, unescaped.
func TestParseClusterSpotStripsControlCharacters(t *testing.T) {
	when := time.Date(2026, time.August, 31, 20, 0, 0, 0, time.UTC)
	line := "DX de K3LR-1:  14025.0 ea8abc CQ\x1b]52;c;AAAA\x07 CQ\x1b[2J evil"
	spot, ok := parseClusterSpot(line, when)
	if !ok {
		t.Fatalf("parseClusterSpot did not parse a DX spot")
	}
	if strings.ContainsAny(spot.Comment, "\x1b\x07") {
		t.Errorf("Comment = %q, want control characters stripped", spot.Comment)
	}
	if !strings.Contains(spot.Comment, "CQ") || !strings.Contains(spot.Comment, "evil") {
		t.Errorf("Comment = %q, want the surrounding readable text preserved", spot.Comment)
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
