package main

import (
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestAddClusterSpotSuppressesSameBandDupeWithinWindow(t *testing.T) {
	var m model
	base := time.Date(2026, time.August, 31, 20, 0, 0, 0, time.UTC)

	m.addClusterSpot(clusterSpot{Callsign: "JA1ABC", Frequency: "14025.0", Received: base})
	if len(m.clusterSpots) != 1 {
		t.Fatalf("first spot should be shown, got %d spots", len(m.clusterSpots))
	}

	// Same station, same band, 2 minutes later (inside the 3-minute window): suppressed.
	m.addClusterSpot(clusterSpot{Callsign: "ja1abc", Frequency: "14026.0", Received: base.Add(2 * time.Minute)})
	if len(m.clusterSpots) != 1 {
		t.Fatalf("dupe within the window should be suppressed, got %d spots", len(m.clusterSpots))
	}

	// Same station, different band: not a dupe.
	m.addClusterSpot(clusterSpot{Callsign: "JA1ABC", Frequency: "7025.0", Received: base.Add(2 * time.Minute)})
	if len(m.clusterSpots) != 2 {
		t.Fatalf("same station on a different band should be shown, got %d spots", len(m.clusterSpots))
	}

	// Same station, same band, past the 3-minute window: not a dupe.
	m.addClusterSpot(clusterSpot{Callsign: "JA1ABC", Frequency: "14027.0", Received: base.Add(4 * time.Minute)})
	if len(m.clusterSpots) != 3 {
		t.Fatalf("spot outside the dupe window should be shown, got %d spots", len(m.clusterSpots))
	}
}

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

// TestMapFeedTapSurvivesTerminalDedup is the Phase 1 completion gate from
// docs/World_Map_Design_Plan.md: two distinct spotters reporting the same DX
// call on the same band within the terminal's 3-minute dupe window must both
// reach the map feed, even though the terminal's own DX Spots panel
// (m.clusterSpots) suppresses the second as a duplicate.
func TestMapFeedTapSurvivesTerminalDedup(t *testing.T) {
	st, err := openStore(filepath.Join(t.TempDir(), "logger.db"))
	if err != nil {
		t.Fatalf("openStore returned error: %v", err)
	}
	defer st.Close()
	m := initialModel(st)

	updated, _ := m.Update(clusterLineMsg{line: "DX de K3LR-1:  14025.0 JA1ABC CQ CQ 2000Z"})
	m = updated.(model)
	updated, _ = m.Update(clusterLineMsg{line: "DX de W1AW:    14025.0 JA1ABC CQ CQ 2001Z"})
	m = updated.(model)

	if got := m.mapReports.Len(); got != 2 {
		t.Fatalf("mapReports.Len() = %d, want 2 (both spotters retained)", got)
	}
	if len(m.clusterSpots) != 1 {
		t.Fatalf("clusterSpots = %d entries, want 1 (terminal dedup unaffected)", len(m.clusterSpots))
	}

	snap := m.mapReports.Snapshot()
	spotters := map[string]bool{}
	for _, r := range snap {
		if r.DXCall != "JA1ABC" {
			t.Errorf("report DXCall = %q, want JA1ABC", r.DXCall)
		}
		spotters[r.SpotterCall] = true
	}
	if !spotters["K3LR-1"] || !spotters["W1AW"] {
		t.Errorf("mapReports spotters = %v, want both K3LR-1 and W1AW", spotters)
	}
}
