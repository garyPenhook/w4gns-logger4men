package main

import (
	"testing"
	"time"
)

func TestIOTAReferenceCode(t *testing.T) {
	cases := []struct {
		text string
		want string
	}{
		{"59 378 EU115", ""}, // three-digit continent-number check: EU115 has no hyphen, not a valid reference
		{"59 378 EU-115", "EU-115"},
		{"eu-005", "EU-005"},
		{"599 254", ""}, // plain world-station exchange, no IOTA reference
		{"NA-065 QRP", "NA-065"},
		{"prefix ZZ-999 unknown", ""}, // ZZ isn't a valid continent prefix
		{"", ""},
	}
	for _, c := range cases {
		if got := iotaReferenceCode(c.text); got != c.want {
			t.Errorf("iotaReferenceCode(%q) = %q, want %q", c.text, got, c.want)
		}
	}
}

func TestIOTAMultiplierValuePrefersExchangeThenFallsBackToField(t *testing.T) {
	q := qso{srxString: "599 001 OC-001", iotaRef: "EU-005"}
	if got := iotaMultiplierValue(q); got != "OC-001" {
		t.Fatalf("iotaMultiplierValue() = %q, want OC-001 (exchange text wins)", got)
	}
	q = qso{srxString: "599 001", iotaRef: "eu-005"}
	if got := iotaMultiplierValue(q); got != "EU-005" {
		t.Fatalf("iotaMultiplierValue() = %q, want EU-005 (falls back to iotaRef field)", got)
	}
	q = qso{srxString: "599 001"}
	if got := iotaMultiplierValue(q); got != "" {
		t.Fatalf("iotaMultiplierValue() = %q, want empty when neither source has a reference", got)
	}
}

// TestRecentClusterIOTAReferenceFindsCommentReference mirrors
// TestRecentClusterPOTAReferenceFindsCommentReference: spots are scanned in
// the newest-first order model.addClusterSpot actually builds.
func TestRecentClusterIOTAReferenceFindsCommentReference(t *testing.T) {
	now := time.Date(2026, time.August, 31, 22, 30, 0, 0, time.UTC)
	spots := []clusterSpot{
		{Callsign: "K1C", Comment: "cq iota eu-115", Received: now.Add(-time.Minute)},
		{Callsign: "K1C", Comment: "IOTA EU-005", Received: now.Add(-10 * time.Minute)},
	}
	if reference, ok := recentClusterIOTAReference(spots, "K1C", now); !ok || reference != "EU-115" {
		t.Fatalf("recentClusterIOTAReference() = %q, %t; want the newest spot's EU-115, true", reference, ok)
	}
}

func TestRecentClusterIOTAReferenceIgnoresOldSpots(t *testing.T) {
	now := time.Date(2026, time.August, 31, 22, 30, 0, 0, time.UTC)
	spots := []clusterSpot{
		{Callsign: "K1C", Comment: "IOTA EU-005", Received: now.Add(-20 * time.Minute)},
	}
	if _, ok := recentClusterIOTAReference(spots, "K1C", now); ok {
		t.Fatal("recentClusterIOTAReference() matched a spot outside the dupe window")
	}
}
