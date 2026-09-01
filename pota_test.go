package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// TestLookupPOTASpotBoundsResponseSize guards against an unbounded read
// from the POTA endpoint (or a MITM) returning an excessively large
// response: json.Decode must fail on the truncated body from
// io.LimitReader rather than buffering the whole thing.
func TestLookupPOTASpotBoundsResponseSize(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("["))
		one := `{"spotTime":"2026-08-31T22:20:00","activator":"W4GNS","reference":"US-100"},`
		for n := 0; n < maxPOTAResponseBytes/len(one)+10; n++ {
			w.Write([]byte(one))
		}
		w.Write([]byte(`{"spotTime":"2026-08-31T22:20:00","activator":"W4GNS","reference":"US-999"}]`))
	}))
	defer srv.Close()

	old := potaSpotAPI
	potaSpotAPI = srv.URL
	defer func() { potaSpotAPI = old }()

	cmd := lookupPOTASpot("W4GNS", time.Now())
	msg, ok := cmd().(potaLookupMsg)
	if !ok {
		t.Fatalf("lookupPOTASpot()() = %T, want potaLookupMsg", msg)
	}
	if msg.err == nil {
		t.Fatal("lookupPOTASpot returned no error for a response exceeding maxPOTAResponseBytes")
	}
}

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

// TestRecentClusterPOTAReferenceFindsCommentReference uses spots in the
// same newest-first order model.addClusterSpot actually builds (each new
// spot prepended to index 0) — a prior version of this test used the
// opposite (oldest-first) order, which happened to still pass despite
// recentClusterPOTAReference scanning backwards and returning the oldest
// match instead of the newest in real use.
func TestRecentClusterPOTAReferenceFindsCommentReference(t *testing.T) {
	now := time.Date(2026, time.August, 31, 22, 30, 0, 0, time.UTC)
	// Both spots are within the 15-minute dupe window, so only scan order
	// distinguishes which reference wins.
	spots := []clusterSpot{
		{Callsign: "W4GNS", Comment: "cq POTA us-222", Received: now.Add(-time.Minute)},
		{Callsign: "W4GNS", Comment: "POTA US-111", Received: now.Add(-10 * time.Minute)},
	}
	if reference, ok := recentClusterPOTAReference(spots, "W4GNS", now); !ok || reference != "US-222" {
		t.Fatalf("recentClusterPOTAReference() = %q, %t; want the newest spot's US-222, true", reference, ok)
	}
}
