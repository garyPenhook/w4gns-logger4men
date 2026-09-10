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

func TestRecentPOTASpotUsesOnlyLastFifteenMinutes(t *testing.T) {
	now := time.Date(2026, time.August, 31, 22, 30, 0, 0, time.UTC)
	spots := []potaSpot{
		{Activator: "W4GNS", Reference: "US-100", Name: "Old Park", SpotTime: "2026-08-31T22:14:59"},
		{Activator: "W4GNS", Reference: "US-200", Name: "New Park", SpotTime: "2026-08-31T22:20:00"},
		{Activator: "K1ABC", Reference: "US-300", Name: "Other Park", SpotTime: "2026-08-31T22:29:00"},
	}
	reference, parkName, ok := recentPOTASpot(spots, "w4gns", now)
	if !ok || reference != "US-200" || parkName != "New Park" {
		t.Fatalf("recentPOTASpot() = %q, %q, %t; want US-200, New Park, true", reference, parkName, ok)
	}
}

// TestRecentPOTASpotFillsParkNameWithoutAReference guards the "prefer the
// name if there's no park number" behavior: a spot record missing Reference
// but carrying Name must still resolve instead of being skipped entirely.
func TestRecentPOTASpotFillsParkNameWithoutAReference(t *testing.T) {
	now := time.Date(2026, time.August, 31, 22, 30, 0, 0, time.UTC)
	spots := []potaSpot{
		{Activator: "W4GNS", Reference: "", Name: "Nameless Number Park", SpotTime: "2026-08-31T22:29:00"},
	}
	reference, parkName, ok := recentPOTASpot(spots, "w4gns", now)
	if !ok || reference != "" || parkName != "Nameless Number Park" {
		t.Fatalf("recentPOTASpot() = %q, %q, %t; want \"\", Nameless Number Park, true", reference, parkName, ok)
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

// TestAutoFillPOTAReferenceSkippedInPostMode guards a real bug found in a
// domain review: autoFillPOTAReference read live cluster spots and the live
// POTA activation API against time.Now() with no regard for POST
// (after-contest/backdated) mode, so logging a historical contact for a
// callsign that happens to be POTA-spotted right now silently stamped
// today's park reference onto a days-old QSO. It must now no-op entirely —
// not even start the async live-lookup command — whenever POST mode is on.
func TestAutoFillPOTAReferenceSkippedInPostMode(t *testing.T) {
	st, err := openStore(t.TempDir() + "/logger.db")
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	m := initialModel(st)
	m.postMode = true
	m.fields[fieldCall].SetValue("W4GNS")
	m.clusterSpots = []clusterSpot{
		{Callsign: "W4GNS", Comment: "POTA US-222", Received: time.Now().Add(-time.Minute)},
	}
	if cmd := m.autoFillPOTAReference(); cmd != nil {
		t.Fatal("autoFillPOTAReference() returned a non-nil command in POST mode, want nil — no live lookup should start for a backdated entry")
	}
	if got := m.fields[fieldPOTARef].Value(); got != "" {
		t.Fatalf("POTA Ref = %q after autoFillPOTAReference in POST mode, want unchanged/blank", got)
	}
}

// TestAutoFillPOTAReferenceFillsOutsidePostMode is the baseline this guards
// against regressing: the same recent cluster spot still autofills normally
// when POST mode is off.
func TestAutoFillPOTAReferenceFillsOutsidePostMode(t *testing.T) {
	st, err := openStore(t.TempDir() + "/logger.db")
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	m := initialModel(st)
	m.fields[fieldCall].SetValue("W4GNS")
	m.clusterSpots = []clusterSpot{
		{Callsign: "W4GNS", Comment: "POTA US-222", Received: time.Now().Add(-time.Minute)},
	}
	m.autoFillPOTAReference()
	if got := m.fields[fieldPOTARef].Value(); got != "US-222" {
		t.Fatalf("POTA Ref = %q, want US-222 from the recent cluster spot", got)
	}
}
