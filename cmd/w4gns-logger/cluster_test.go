package main

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"w4gns-logger/internal/geo"
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

// TestMapFeedTapUsesCachedQRZLocationOverCountryReference covers the QRZ
// map-location feature end to end through Update: once qrzGeoCache has a
// QRZ-derived location cached for a call (as qrzMapGeoMsg populates it), a
// later spot for that call must use it instead of the coarser country/
// prefix reference, even though the call also resolves there.
func TestMapFeedTapUsesCachedQRZLocationOverCountryReference(t *testing.T) {
	st, err := openStore(filepath.Join(t.TempDir(), "logger.db"))
	if err != nil {
		t.Fatalf("openStore returned error: %v", err)
	}
	defer st.Close()
	m := initialModel(st)

	updated, _ := m.Update(qrzMapGeoMsg{
		call:   "JA1ABC",
		record: qrzCallsignRecord{hasLatLon: true, latitude: 35.6, longitude: 139.7, country: "Japan"},
	})
	m = updated.(model)

	updated, _ = m.Update(clusterLineMsg{line: "DX de K3LR-1:  14025.0 JA1ABC CQ CQ 2000Z"})
	m = updated.(model)

	snap := m.mapReports.Snapshot()
	if len(snap) != 1 {
		t.Fatalf("mapReports.Len() = %d, want 1", len(snap))
	}
	loc := snap[0].DXLocation
	if loc == nil || loc.Source != geo.SourceQRZProfile || loc.Latitude != 35.6 || loc.Longitude != 139.7 {
		t.Fatalf("DXLocation = %+v, want the cached QRZ location (source QRZProfile, lat 35.6/lon 139.7)", loc)
	}
}

// TestQrzMapGeoMsgIgnoresSessionKeyFromSupersededCredentials guards a real
// bug found in a domain review: qrzMapGeoMsg's handler restored
// m.qrzXMLSessionKey from any inbound response unconditionally, unlike the
// QSO Entry auto-fill flow's qrzCallsignLookupMsg handler, which already got
// a request-ID-based version of this same protection (R22 in
// docs/ROADMAP.md). A map geo-lookup issued under old credentials could
// therefore silently resurrect the old account's session after Station
// Setup changed credentials and cleared it.
func TestQrzMapGeoMsgIgnoresSessionKeyFromSupersededCredentials(t *testing.T) {
	st, err := openStore(filepath.Join(t.TempDir(), "logger.db"))
	if err != nil {
		t.Fatalf("openStore returned error: %v", err)
	}
	defer st.Close()
	m := initialModel(st)
	m.qrzCredGeneration = 5 // credentials already changed once since startup

	// A result stamped with a superseded generation (the lookup started
	// before the most recent credential change) must not resurrect the
	// session key.
	updated, _ := m.Update(qrzMapGeoMsg{call: "W1AW", sessionKey: "OLD-SESSION", credGeneration: 4})
	stale := updated.(model)
	if stale.qrzXMLSessionKey != "" {
		t.Fatalf("qrzXMLSessionKey = %q after a superseded-generation result, want unchanged (blank)", stale.qrzXMLSessionKey)
	}

	// A result stamped with the current generation is trusted normally.
	updated, _ = stale.Update(qrzMapGeoMsg{call: "W1AW", sessionKey: "NEW-SESSION", credGeneration: 5})
	fresh := updated.(model)
	if fresh.qrzXMLSessionKey != "NEW-SESSION" {
		t.Fatalf("qrzXMLSessionKey = %q, want NEW-SESSION from a current-generation result", fresh.qrzXMLSessionKey)
	}
}

// TestMapFeedTapUsesCachedPOTALocationOverQRZ covers the POTA activation
// location feature end to end through Update: once potaGeoCache has a park
// location cached for a reference named in a spot's comment, that spot's
// DXLocation must use it even when a QRZ profile location is also cached
// for the same callsign — the activation site takes priority.
func TestMapFeedTapUsesCachedPOTALocationOverQRZ(t *testing.T) {
	st, err := openStore(filepath.Join(t.TempDir(), "logger.db"))
	if err != nil {
		t.Fatalf("openStore returned error: %v", err)
	}
	defer st.Close()
	m := initialModel(st)

	updated, _ := m.Update(qrzMapGeoMsg{
		call:   "W4GNS",
		record: qrzCallsignRecord{hasLatLon: true, latitude: 1, longitude: 1, country: "United States"},
	})
	m = updated.(model)
	updated, _ = m.Update(potaGeoMsg{
		reference: "K-1234",
		record:    potaParkRecord{Latitude: 35.9307, Longitude: -85.9401, EntityName: "United States of America"},
	})
	m = updated.(model)

	updated, _ = m.Update(clusterLineMsg{line: "DX de K3LR-1:  14025.0 W4GNS QRP FROM K-1234 2000Z"})
	m = updated.(model)

	snap := m.mapReports.Snapshot()
	if len(snap) != 1 {
		t.Fatalf("mapReports.Len() = %d, want 1", len(snap))
	}
	loc := snap[0].DXLocation
	if loc == nil || loc.Source != geo.SourcePOTAPark || loc.Latitude != 35.9307 || loc.Longitude != -85.9401 {
		t.Fatalf("DXLocation = %+v, want the cached POTA location (source POTAPark, lat 35.9307/lon -85.9401)", loc)
	}
}

// TestMapFeedTapHoldsFirstSpotUntilPOTALookupResolves covers the case
// TestMapFeedTapUsesCachedPOTALocationOverQRZ doesn't: a spot naming a POTA
// reference that has never been looked up yet. Adding it immediately would
// permanently strand it at the coarser DXCC country reference, since the
// map store has no way to revisit an already-added report once the async
// lookup completes. It must instead be held back and only added once
// potaGeoMsg arrives, carrying the precise park location.
func TestMapFeedTapHoldsFirstSpotUntilPOTALookupResolves(t *testing.T) {
	st, err := openStore(filepath.Join(t.TempDir(), "logger.db"))
	if err != nil {
		t.Fatalf("openStore returned error: %v", err)
	}
	defer st.Close()
	m := initialModel(st)

	updated, _ := m.Update(clusterLineMsg{line: "DX de K3LR-1:  14025.0 W4GNS QRP FROM K-1234 2000Z"})
	m = updated.(model)

	if got := m.mapReports.Len(); got != 0 {
		t.Fatalf("mapReports.Len() = %d before the POTA lookup resolves, want 0 (spot must be held, not stranded at the country reference)", got)
	}

	updated, _ = m.Update(potaGeoMsg{
		reference: "K-1234",
		record:    potaParkRecord{Latitude: 35.9307, Longitude: -85.9401, EntityName: "United States of America"},
	})
	m = updated.(model)

	snap := m.mapReports.Snapshot()
	if len(snap) != 1 {
		t.Fatalf("mapReports.Len() = %d after the POTA lookup resolves, want 1", len(snap))
	}
	loc := snap[0].DXLocation
	if loc == nil || loc.Source != geo.SourcePOTAPark || loc.Latitude != 35.9307 || loc.Longitude != -85.9401 {
		t.Fatalf("DXLocation = %+v, want the freshly resolved POTA location (source POTAPark, lat 35.9307/lon -85.9401)", loc)
	}
}

// TestMapFeedTapAddsHeldSpotWithFallbackWhenPOTALookupFails covers the
// negative path: a held spot whose POTA lookup fails (or the reference
// turns out to have no coordinate) must still be added — with whatever
// fallback location resolveDXLocation finds — rather than being silently
// dropped forever.
func TestMapFeedTapAddsHeldSpotWithFallbackWhenPOTALookupFails(t *testing.T) {
	st, err := openStore(filepath.Join(t.TempDir(), "logger.db"))
	if err != nil {
		t.Fatalf("openStore returned error: %v", err)
	}
	defer st.Close()
	m := initialModel(st)

	updated, _ := m.Update(clusterLineMsg{line: "DX de K3LR-1:  14025.0 W4GNS QRP FROM K-9999 2000Z"})
	m = updated.(model)
	if got := m.mapReports.Len(); got != 0 {
		t.Fatalf("mapReports.Len() = %d before the POTA lookup resolves, want 0", got)
	}

	updated, _ = m.Update(potaGeoMsg{reference: "K-9999", err: errors.New("lookup failed")})
	m = updated.(model)

	if got := m.mapReports.Len(); got != 1 {
		t.Fatalf("mapReports.Len() = %d after a failed POTA lookup, want 1 (held spot must still be added)", got)
	}
}
