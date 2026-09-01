package main

import (
	"context"
	"encoding/xml"
	"fmt"
	"net/http"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

const (
	solarFetchTimeout    = 10 * time.Second
	solarRefreshInterval = 30 * time.Minute
)

// solarDataURL is a var (not const) so tests can point it at a local server.
var solarDataURL = "https://www.hamqsl.com/solarxml.php"

// solarIndices holds the propagation indices amateurs check before operating:
// Solar Flux Index (higher favors high-band DX), A-index and K-index (higher
// values indicate more geomagnetic disturbance, which degrades propagation).
type solarIndices struct {
	SFI     string
	AIndex  string
	KIndex  string
	Updated string
}

// solarXMLFeed maps the N0NBH solar-data XML feed's <solar><solardata>...
// fields actually used here. The feed carries many more (band conditions,
// aurora, sunspots) that this logger doesn't display.
type solarXMLFeed struct {
	Data struct {
		Updated   string `xml:"updated"`
		SolarFlux string `xml:"solarflux"`
		AIndex    string `xml:"aindex"`
		KIndex    string `xml:"kindex"`
	} `xml:"solardata"`
}

type solarIndicesMsg struct {
	indices solarIndices
	err     error
}

type solarTickMsg struct{}

// fetchSolarIndicesCmd runs asynchronously, matching the existing POTA-lookup
// and QRZ-upload commands, so the terminal UI never blocks on network I/O.
func fetchSolarIndicesCmd() tea.Cmd {
	return func() tea.Msg {
		indices, err := fetchSolarIndices(context.Background())
		return solarIndicesMsg{indices: indices, err: err}
	}
}

// solarTickCmd schedules the next periodic refresh. Solar indices are
// updated a few times a day by the source, so polling every 30 minutes is
// frequent enough without hammering the feed.
func solarTickCmd() tea.Cmd {
	return tea.Tick(solarRefreshInterval, func(time.Time) tea.Msg { return solarTickMsg{} })
}

// solarLine renders the SFI/A/K summary shown on the QSO-entry home screen.
func (m model) solarLine() string {
	if m.solar.SFI == "" && m.solar.AIndex == "" && m.solar.KIndex == "" {
		if m.solarErr != "" {
			return "Solar: unavailable (" + m.solarErr + ")"
		}
		return "Solar: loading…"
	}
	return fmt.Sprintf("Solar: SFI %s  A %s  K %s", m.solar.SFI, m.solar.AIndex, m.solar.KIndex)
}

func fetchSolarIndices(parent context.Context) (solarIndices, error) {
	ctx, cancel := context.WithTimeout(parent, solarFetchTimeout)
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, solarDataURL, nil)
	if err != nil {
		return solarIndices{}, fmt.Errorf("create solar data request: %w", err)
	}
	client := &http.Client{Timeout: solarFetchTimeout}
	response, err := client.Do(request)
	if err != nil {
		return solarIndices{}, fmt.Errorf("fetch solar data: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return solarIndices{}, fmt.Errorf("fetch solar data: %s", response.Status)
	}
	var feed solarXMLFeed
	if err := xml.NewDecoder(response.Body).Decode(&feed); err != nil {
		return solarIndices{}, fmt.Errorf("parse solar data: %w", err)
	}
	indices := solarIndices{
		SFI:     strings.TrimSpace(feed.Data.SolarFlux),
		AIndex:  strings.TrimSpace(feed.Data.AIndex),
		KIndex:  strings.TrimSpace(feed.Data.KIndex),
		Updated: strings.TrimSpace(feed.Data.Updated),
	}
	if indices.SFI == "" && indices.AIndex == "" && indices.KIndex == "" {
		return solarIndices{}, fmt.Errorf("solar data response missing SFI/A/K indices")
	}
	return indices, nil
}
