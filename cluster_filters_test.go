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

func TestClusterFiltersRejectPhoneSegmentSpots(t *testing.T) {
	filters := defaultClusterFilters()
	// 14.250 MHz is inside the enabled 20M band but well within the phone
	// segment (CW/data ends at 14.150), so a CW-only logger should reject it.
	if filters.allowsSpot(clusterSpot{Frequency: "14250.0", Callsign: "W4GNS"}) {
		t.Fatal("phone-segment spot was allowed through a CW-only filter")
	}
	if !filters.allowsSpot(clusterSpot{Frequency: "14025.0", Callsign: "W4GNS"}) {
		t.Fatal("CW-segment spot was rejected")
	}
}

// TestClusterFiltersRejectRTTYSpotInCWDataSubBand guards a real report: an
// RTTY spot appeared in a "CW only" filtered feed. RTTY (and other digital
// modes) share the same data sub-band as CW on most bands, so frequency
// range alone can't tell them apart — a spotter comment naming the mode is
// the only signal available.
func TestClusterFiltersRejectRTTYSpotInCWDataSubBand(t *testing.T) {
	filters := defaultClusterFilters()
	if filters.allowsSpot(clusterSpot{Frequency: "14080.0", Callsign: "W4GNS", Comment: "RTTY 21 WPM via RBN"}) {
		t.Fatal("RTTY-commented spot was allowed through a CW-only filter")
	}
	if !filters.allowsSpot(clusterSpot{Frequency: "14080.0", Callsign: "W4GNS", Comment: "CQ CQ"}) {
		t.Fatal("plain CW-segment spot was rejected")
	}
}

// TestClusterFiltersDoNotMisreadFromAbbreviationAsFMMode guards against a
// false positive this filter must not introduce: "FM" is common DX-cluster
// shorthand for "from" (e.g. "TNX FM VE3XYZ"), not the FM mode, and must
// not cause a real CW spot to be rejected.
func TestClusterFiltersDoNotMisreadFromAbbreviationAsFMMode(t *testing.T) {
	filters := defaultClusterFilters()
	if !filters.allowsSpot(clusterSpot{Frequency: "14025.0", Callsign: "W4GNS", Comment: "TNX FM VE3XYZ"}) {
		t.Fatal("a CW spot with ham-slang \"FM\" (from) in its comment was incorrectly rejected as FM-mode")
	}
}

func TestClusterFiltersRejectSpotsOutsideDXCountryFilter(t *testing.T) {
	filters := defaultClusterFilters()
	filters.DXCC = "Japan"
	if filters.allowsSpot(clusterSpot{Frequency: "14025.0", Callsign: "W4GNS"}) {
		t.Fatal("non-Japan DX spot was allowed through a Japan-only DX filter")
	}
	if !filters.allowsSpot(clusterSpot{Frequency: "14025.0", Callsign: "JA1ABC"}) {
		t.Fatal("Japan DX spot was rejected by a Japan-only DX filter")
	}
}

func TestClusterFiltersRejectSpottersOutsideDEContinentFilter(t *testing.T) {
	filters := defaultClusterFilters()
	filters.DEContinent = "EU"
	if !filters.allowsSpot(clusterSpot{Frequency: "14025.0", Callsign: "W4GNS", Spotter: "G4ABC"}) {
		t.Fatal("EU spotter was rejected by a EU-only DE filter")
	}
	if filters.allowsSpot(clusterSpot{Frequency: "14025.0", Callsign: "W4GNS", Spotter: "VK2ABC"}) {
		t.Fatal("non-EU spotter was allowed through a EU-only DE filter")
	}
}

func TestClusterFiltersRejectUnresolvableCallWhenFilterActive(t *testing.T) {
	filters := defaultClusterFilters()
	filters.DXCQZone = "5"
	if filters.allowsSpot(clusterSpot{Frequency: "14025.0", Callsign: ""}) {
		t.Fatal("spot with an unresolvable DX callsign should be rejected while a DX filter is active")
	}
}

func TestClusterFiltersMatchDECallAreaFilter(t *testing.T) {
	filters := defaultClusterFilters()
	filters.DECallArea = "2, 3, 4"
	if !filters.allowsSpot(clusterSpot{Frequency: "14025.0", Callsign: "W4GNS", Spotter: "K3ABC"}) {
		t.Fatal("call-area-3 spotter should pass a 2/3/4 DE call area filter")
	}
	if filters.allowsSpot(clusterSpot{Frequency: "14025.0", Callsign: "W4GNS", Spotter: "W1AW"}) {
		t.Fatal("call-area-1 spotter should be rejected by a 2/3/4 DE call area filter")
	}
	// Portable notation: the numeric slash segment overrides the base call's
	// own digit, so W1AW/4 is treated as operating from call area 4.
	if !filters.allowsSpot(clusterSpot{Frequency: "14025.0", Callsign: "W4GNS", Spotter: "W1AW/4"}) {
		t.Fatal("W1AW/4 should pass a 2/3/4 DE call area filter via its portable override")
	}
	if filters.allowsSpot(clusterSpot{Frequency: "14025.0", Callsign: "W4GNS", Spotter: ""}) {
		t.Fatal("spot with an unresolvable spotter callsign should be rejected while a DE call area filter is active")
	}
}

func TestClusterFiltersMatchZoneFilters(t *testing.T) {
	filters := defaultClusterFilters()
	filters.DXITUZone = "8"
	filters.DXCQZone = "5"
	if !filters.allowsSpot(clusterSpot{Frequency: "14025.0", Callsign: "W4GNS"}) {
		t.Fatal("W4GNS (ITU 8 / CQ 5) should pass matching ITU/CQ zone filters")
	}
	filters.DXCQZone = "14"
	if filters.allowsSpot(clusterSpot{Frequency: "14025.0", Callsign: "W4GNS"}) {
		t.Fatal("W4GNS should not pass a mismatched CQ zone filter")
	}
}

func TestCommentIndicatesNonCWMode(t *testing.T) {
	nonCW := []string{"RTTY 21 WPM", "psk31 qrp", "FT8 CQ", "ft4", "JS8Call", "SSB net"}
	for _, comment := range nonCW {
		if !commentIndicatesNonCWMode(comment) {
			t.Errorf("commentIndicatesNonCWMode(%q) = false, want true", comment)
		}
	}
	cw := []string{"CQ CQ", "TNX FM VE3XYZ", "599 UP 2", "", "5NN TU"}
	for _, comment := range cw {
		if commentIndicatesNonCWMode(comment) {
			t.Errorf("commentIndicatesNonCWMode(%q) = true, want false", comment)
		}
	}
}
