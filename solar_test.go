package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

const sampleSolarXML = `<?xml version="1.0" encoding="UTF-8" ?>
<solar>
	<solardata>
		<source url="http://www.hamqsl.com/solar.html">N0NBH</source>
		<updated> 01 Sep 2026 1300 GMT</updated>
		<solarflux>107</solarflux>
		<aindex> 12</aindex>
		<kindex> 2</kindex>
		<kindexnt>No Report</kindexnt>
		<xray>B8.2</xray>
		<sunspots>61</sunspots>
	</solardata>
</solar>`

func TestFetchSolarIndicesParsesSFIAAndK(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(sampleSolarXML))
	}))
	defer srv.Close()

	old := solarDataURL
	solarDataURL = srv.URL
	defer func() { solarDataURL = old }()

	indices, err := fetchSolarIndices(context.Background())
	if err != nil {
		t.Fatalf("fetchSolarIndices: %v", err)
	}
	if indices.SFI != "107" {
		t.Errorf("SFI = %q, want 107", indices.SFI)
	}
	if indices.AIndex != "12" {
		t.Errorf("AIndex = %q, want 12", indices.AIndex)
	}
	if indices.KIndex != "2" {
		t.Errorf("KIndex = %q, want 2", indices.KIndex)
	}
}

func TestFetchSolarIndicesRejectsEmptyFeed(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`<solar><solardata></solardata></solar>`))
	}))
	defer srv.Close()

	old := solarDataURL
	solarDataURL = srv.URL
	defer func() { solarDataURL = old }()

	if _, err := fetchSolarIndices(context.Background()); err == nil {
		t.Fatal("fetchSolarIndices returned no error for a feed with no SFI/A/K data")
	}
}

func TestFetchSolarIndicesReportsHTTPFailure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	old := solarDataURL
	solarDataURL = srv.URL
	defer func() { solarDataURL = old }()

	if _, err := fetchSolarIndices(context.Background()); err == nil {
		t.Fatal("fetchSolarIndices returned no error for a 503 response")
	}
}

func TestSolarLineFormatsIndicesOrFallsBack(t *testing.T) {
	var m model
	if got := m.solarLine(); got != "Solar: loading…" {
		t.Errorf("solarLine() with no data = %q, want the loading placeholder", got)
	}

	m.solarErr = "network error"
	if got := m.solarLine(); !strings.Contains(got, "network error") {
		t.Errorf("solarLine() with an error and no prior data = %q, want it to mention the error", got)
	}

	m.solarErr = ""
	m.solar = solarIndices{SFI: "107", AIndex: "12", KIndex: "2", Updated: "01 Sep 2026 1300 GMT"}
	if got := m.solarLine(); got != "Solar: SFI 107  A 12  K 2  as of 01 Sep 2026 1300 GMT" {
		t.Errorf("solarLine() = %q, want the formatted indices with the source timestamp", got)
	}
}

// TestSolarLineMarksStaleDataAfterFailedRefresh guards against a failed
// refresh silently keeping old data on screen with no indication anything
// went wrong: once indices have been fetched at least once, a later failure
// must still show the last known-good values (better than blanking them)
// but flag them as stale.
func TestSolarLineMarksStaleDataAfterFailedRefresh(t *testing.T) {
	m := model{
		solar:    solarIndices{SFI: "107", AIndex: "12", KIndex: "2", Updated: "01 Sep 2026 1300 GMT"},
		solarErr: "fetch solar data: 503 Service Unavailable",
	}
	got := m.solarLine()
	if !strings.Contains(got, "SFI 107") {
		t.Errorf("solarLine() = %q, want it to keep showing the last known-good indices", got)
	}
	if !strings.Contains(got, "STALE") || !strings.Contains(got, "503 Service Unavailable") {
		t.Errorf("solarLine() = %q, want a stale marker mentioning the refresh error", got)
	}
}
