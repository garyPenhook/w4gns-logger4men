package main

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestFetchPOTAParkParsesCoordinates covers the happy path against a real
// recorded shape from https://api.pota.app/park/US-7614.
func TestFetchPOTAParkParsesCoordinates(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"reference":"US-7614","name":"Pea Ridge","latitude":35.9307,"longitude":-85.9401,"entityName":"United States of America"}`)
	}))
	defer srv.Close()

	old := potaParkAPI
	potaParkAPI = srv.URL + "/"
	defer func() { potaParkAPI = old }()

	record, err := fetchPOTAPark(context.Background(), "US-7614")
	if err != nil {
		t.Fatalf("fetchPOTAPark returned error: %v", err)
	}
	if !record.hasCoordinates() {
		t.Fatal("hasCoordinates() = false, want true")
	}
	if record.Latitude != 35.9307 || record.Longitude != -85.9401 {
		t.Fatalf("lat,lon = %v,%v, want 35.9307,-85.9401", record.Latitude, record.Longitude)
	}
	if record.EntityName != "United States of America" {
		t.Fatalf("EntityName = %q, want %q", record.EntityName, "United States of America")
	}
}

// TestFetchPOTAParkNullBodyHasNoCoordinates covers an unknown/inactive
// reference: POTA's API returns bare JSON null (not an HTTP error), which
// must decode to a zero-value record rather than failing the lookup.
func TestFetchPOTAParkNullBodyHasNoCoordinates(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "null")
	}))
	defer srv.Close()

	old := potaParkAPI
	potaParkAPI = srv.URL + "/"
	defer func() { potaParkAPI = old }()

	record, err := fetchPOTAPark(context.Background(), "US-99999999")
	if err != nil {
		t.Fatalf("fetchPOTAPark returned error: %v", err)
	}
	if record.hasCoordinates() {
		t.Fatalf("hasCoordinates() = true for a null response, want false: %+v", record)
	}
}

func TestFetchPOTAParkNonOKStatusIsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	old := potaParkAPI
	potaParkAPI = srv.URL + "/"
	defer func() { potaParkAPI = old }()

	_, err := fetchPOTAPark(context.Background(), "US-7614")
	if err == nil || !strings.Contains(err.Error(), "500") {
		t.Fatalf("err = %v, want a 500-status error", err)
	}
}

func TestPotaGeoLookupCmdSkipsBlankReference(t *testing.T) {
	if cmd := potaGeoLookupCmd("  "); cmd != nil {
		t.Fatal("potaGeoLookupCmd with a blank reference returned a non-nil command")
	}
}
