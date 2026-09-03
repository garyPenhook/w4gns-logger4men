package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// potaSpotAPI is a var (not const) so tests can point it at a local server.
var potaSpotAPI = "https://api.pota.app/spot/"

// maxPOTAResponseBytes bounds how much of the response this reads. The live
// POTA spot feed is at most a few thousand small JSON records; this is far
// larger than that, just enough to stop an unbounded read if the endpoint
// (or a MITM) ever returns something huge.
const maxPOTAResponseBytes = 5 << 20

// potaReferencePattern matches a POTA park reference (e.g. "K-0001",
// "VE-5082", "US-222") in free-form cluster comment text. The prefix is
// limited to 1-2 characters and the number to 3+ digits: the original
// 1-3-letter / 1-digit form matched common non-POTA tokens like "RST-599"
// (3-letter prefix) and "CQ-1" (single digit), tagging QSOs with bogus
// references.
var potaReferencePattern = regexp.MustCompile(`(?i)\b[A-Z0-9]{1,2}-\d{3,6}\b`)

type potaSpot struct {
	SpotTime  string `json:"spotTime"`
	Activator string `json:"activator"`
	Reference string `json:"reference"`
	Name      string `json:"name"`
}

type potaLookupMsg struct {
	call      string
	reference string
	parkName  string
	err       error
}

func lookupPOTASpot(call string, now time.Time) tea.Cmd {
	call = normalizeCall(call)
	return func() tea.Msg {
		request, err := http.NewRequest(http.MethodGet, potaSpotAPI, nil)
		if err != nil {
			return potaLookupMsg{call: call, err: fmt.Errorf("create POTA lookup: %w", err)}
		}
		client := &http.Client{Timeout: 10 * time.Second}
		response, err := client.Do(request)
		if err != nil {
			return potaLookupMsg{call: call, err: fmt.Errorf("query POTA spots: %w", err)}
		}
		defer response.Body.Close()
		if response.StatusCode != http.StatusOK {
			return potaLookupMsg{call: call, err: fmt.Errorf("query POTA spots: %s", response.Status)}
		}
		var spots []potaSpot
		if err := json.NewDecoder(io.LimitReader(response.Body, maxPOTAResponseBytes)).Decode(&spots); err != nil {
			return potaLookupMsg{call: call, err: fmt.Errorf("decode POTA spots: %w", err)}
		}
		reference, parkName, ok := recentPOTASpot(spots, call, now)
		if !ok {
			return potaLookupMsg{call: call}
		}
		return potaLookupMsg{call: call, reference: reference, parkName: parkName}
	}
}

// recentPOTASpot returns the reference and park name of call's most recent
// spot within the dupe window. A spot with a name but no reference (seen, in
// principle, from a malformed upstream record) still counts, so the park
// name can be filled in on its own rather than losing the match entirely.
func recentPOTASpot(spots []potaSpot, call string, now time.Time) (reference, parkName string, ok bool) {
	call = normalizeCall(call)
	cutoff := now.UTC().Add(-dupeWindow)
	var latest time.Time
	for _, spot := range spots {
		if !strings.EqualFold(strings.TrimSpace(spot.Activator), call) {
			continue
		}
		if strings.TrimSpace(spot.Reference) == "" && strings.TrimSpace(spot.Name) == "" {
			continue
		}
		spotTime, err := parsePOTASpotTime(spot.SpotTime)
		if err != nil || spotTime.Before(cutoff) || spotTime.After(now.UTC().Add(time.Minute)) {
			continue
		}
		if spotTime.After(latest) {
			latest = spotTime
			reference = strings.ToUpper(strings.TrimSpace(spot.Reference))
			parkName = strings.TrimSpace(spot.Name)
		}
	}
	return reference, parkName, reference != "" || parkName != ""
}

// parsePOTASpotTime parses a POTA spotTime, which the live feed currently
// emits as a bare "2006-01-02T15:04:05" in UTC. It also accepts an explicit
// trailing "Z", a timezone offset, and fractional seconds so a future format
// change (or a proxy that normalizes the timestamp) doesn't silently cause
// every spot to be skipped and POTA auto-fill to stop working. Values without
// a zone are interpreted as UTC.
func parsePOTASpotTime(value string) (time.Time, error) {
	value = strings.TrimSpace(value)
	for _, layout := range []string{
		"2006-01-02T15:04:05",
		"2006-01-02T15:04:05Z07:00",
		"2006-01-02T15:04:05.999999999",
		"2006-01-02T15:04:05.999999999Z07:00",
	} {
		if t, err := time.ParseInLocation(layout, value, time.UTC); err == nil {
			return t.UTC(), nil
		}
	}
	return time.Time{}, fmt.Errorf("unrecognized POTA spotTime %q", value)
}

// recentClusterPOTAReference scans spots (newest-first, per
// model.addClusterSpot which prepends each new spot) for the most recent
// spot for call within the dupe window, returning its POTA reference if
// its comment has one. A forward scan finds that newest match first.
func recentClusterPOTAReference(spots []clusterSpot, call string, now time.Time) (string, bool) {
	cutoff := now.UTC().Add(-dupeWindow)
	for _, spot := range spots {
		if !strings.EqualFold(spot.Callsign, call) || spot.Received.Before(cutoff) {
			continue
		}
		if reference := potaReferencePattern.FindString(spot.Comment); reference != "" {
			return strings.ToUpper(reference), true
		}
	}
	return "", false
}
