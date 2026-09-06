package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"w4gns-logger/internal/geo"
)

// potaParkAPI is a var (not const) so tests can point it at a local server,
// matching potaSpotAPI's pattern in pota.go.
var potaParkAPI = "https://api.pota.app/park/"

const potaParkLookupTimeout = 15 * time.Second

// maxPOTAParkResponseBytes bounds how much of the response this reads: a
// POTA park record is well under a KB, so this is far larger than any real
// response, just enough to stop an unbounded read if the endpoint (or a
// MITM) ever returns something huge — mirrors maxPOTAResponseBytes' intent
// at a smaller scale appropriate to a single-record lookup.
const maxPOTAParkResponseBytes = 64 * 1024

// potaParkConcurrency bounds how many POTA park lookups the map feed runs at
// once, mirroring qrzGeoConcurrency — a separate channel so POTA and QRZ
// lookups don't compete for the same budget.
const potaParkConcurrency = 2

var potaParkSemaphore = make(chan struct{}, potaParkConcurrency)

// potaParkRecord holds the fields the map feed reads from a POTA park
// lookup. See https://api.pota.app/park/{reference} — an unknown or
// inactive reference returns bare JSON null, which decodes to a zero value
// here rather than an error.
type potaParkRecord struct {
	Latitude   float64 `json:"latitude"`
	Longitude  float64 `json:"longitude"`
	Name       string  `json:"name"`
	EntityName string  `json:"entityName"`
}

// hasCoordinates reports whether r carries a usable coordinate, treating an
// all-zero pair as absent — the same explicit-absence convention
// dxccEntity.HasCoordinates uses, since a bare JSON null response decodes to
// exactly this zero value.
func (r potaParkRecord) hasCoordinates() bool {
	return (r.Latitude != 0 || r.Longitude != 0) && geo.ValidCoordinates(r.Latitude, r.Longitude)
}

// potaGeoMsg carries a map-feed POTA park lookup result, keyed by reference
// rather than a request ID — many of these can be in flight concurrently,
// one per newly seen POTA reference in a cluster spot's comment.
type potaGeoMsg struct {
	reference string
	record    potaParkRecord
	err       error
}

// potaGeoLookupCmd looks up reference's park location for the map feed's
// POTA cache. No credentials are needed (POTA's park API is public), unlike
// the QRZ equivalent.
func potaGeoLookupCmd(reference string) tea.Cmd {
	reference = strings.ToUpper(strings.TrimSpace(reference))
	if reference == "" {
		return nil
	}
	return func() tea.Msg {
		potaParkSemaphore <- struct{}{}
		defer func() { <-potaParkSemaphore }()
		ctx, cancel := context.WithTimeout(context.Background(), potaParkLookupTimeout)
		defer cancel()
		record, err := fetchPOTAPark(ctx, reference)
		return potaGeoMsg{reference: reference, record: record, err: err}
	}
}

func fetchPOTAPark(ctx context.Context, reference string) (potaParkRecord, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, potaParkAPI+url.PathEscape(reference), nil)
	if err != nil {
		return potaParkRecord{}, fmt.Errorf("create POTA park lookup: %w", err)
	}
	client := &http.Client{Timeout: potaParkLookupTimeout}
	response, err := client.Do(request)
	if err != nil {
		return potaParkRecord{}, fmt.Errorf("query POTA park: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return potaParkRecord{}, fmt.Errorf("query POTA park: %s", response.Status)
	}
	var record potaParkRecord
	if err := json.NewDecoder(io.LimitReader(response.Body, maxPOTAParkResponseBytes)).Decode(&record); err != nil {
		return potaParkRecord{}, fmt.Errorf("decode POTA park: %w", err)
	}
	return record, nil
}
