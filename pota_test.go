package main

import (
	"testing"
	"time"
)

func TestRecentPOTAReferenceUsesOnlyLastFifteenMinutes(t *testing.T) {
	now := time.Date(2026, time.August, 31, 22, 30, 0, 0, time.UTC)
	spots := []potaSpot{
		{Activator: "W4GNS", Reference: "US-100", SpotTime: "2026-08-31T22:14:59"},
		{Activator: "W4GNS", Reference: "US-200", SpotTime: "2026-08-31T22:20:00"},
		{Activator: "K1ABC", Reference: "US-300", SpotTime: "2026-08-31T22:29:00"},
	}
	if reference, ok := recentPOTAReference(spots, "w4gns", now); !ok || reference != "US-200" {
		t.Fatalf("recentPOTAReference() = %q, %t; want US-200, true", reference, ok)
	}
}

func TestRecentClusterPOTAReferenceFindsCommentReference(t *testing.T) {
	now := time.Date(2026, time.August, 31, 22, 30, 0, 0, time.UTC)
	spots := []clusterSpot{
		{Callsign: "W4GNS", Comment: "POTA US-111", Received: now.Add(-16 * time.Minute)},
		{Callsign: "W4GNS", Comment: "cq POTA us-222", Received: now.Add(-time.Minute)},
	}
	if reference, ok := recentClusterPOTAReference(spots, "W4GNS", now); !ok || reference != "US-222" {
		t.Fatalf("recentClusterPOTAReference() = %q, %t; want US-222, true", reference, ok)
	}
}
