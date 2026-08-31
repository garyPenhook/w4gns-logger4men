package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

const potaSpotAPI = "https://api.pota.app/spot/"

var potaReferencePattern = regexp.MustCompile(`(?i)\b[A-Z]{1,3}-\d{1,6}\b`)

type potaSpot struct {
	SpotTime  string `json:"spotTime"`
	Activator string `json:"activator"`
	Reference string `json:"reference"`
}

type potaLookupMsg struct {
	call      string
	reference string
	err       error
}

func lookupPOTASpot(call string, now time.Time) tea.Cmd {
	call = strings.ToUpper(strings.TrimSpace(call))
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
		if err := json.NewDecoder(response.Body).Decode(&spots); err != nil {
			return potaLookupMsg{call: call, err: fmt.Errorf("decode POTA spots: %w", err)}
		}
		reference, ok := recentPOTAReference(spots, call, now)
		if !ok {
			return potaLookupMsg{call: call}
		}
		return potaLookupMsg{call: call, reference: reference}
	}
}

func recentPOTAReference(spots []potaSpot, call string, now time.Time) (string, bool) {
	call = strings.ToUpper(strings.TrimSpace(call))
	cutoff := now.UTC().Add(-dupeWindow)
	var latest time.Time
	var reference string
	for _, spot := range spots {
		if !strings.EqualFold(strings.TrimSpace(spot.Activator), call) || strings.TrimSpace(spot.Reference) == "" {
			continue
		}
		spotTime, err := time.ParseInLocation("2006-01-02T15:04:05", spot.SpotTime, time.UTC)
		if err != nil || spotTime.Before(cutoff) || spotTime.After(now.UTC().Add(time.Minute)) {
			continue
		}
		if spotTime.After(latest) {
			latest = spotTime
			reference = strings.ToUpper(strings.TrimSpace(spot.Reference))
		}
	}
	return reference, reference != ""
}

func recentClusterPOTAReference(spots []clusterSpot, call string, now time.Time) (string, bool) {
	cutoff := now.UTC().Add(-dupeWindow)
	for index := len(spots) - 1; index >= 0; index-- {
		spot := spots[index]
		if !strings.EqualFold(spot.Callsign, call) || spot.Received.Before(cutoff) {
			continue
		}
		if reference := potaReferencePattern.FindString(spot.Comment); reference != "" {
			return strings.ToUpper(reference), true
		}
	}
	return "", false
}
