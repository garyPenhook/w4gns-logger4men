package main

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"runtime"
	"strconv"

	tea "github.com/charmbracelet/bubbletea"
	"w4gns-logger/internal/geo"
	"w4gns-logger/internal/mapserver"
	"w4gns-logger/internal/spot"
)

type mapOpenedMsg string

func openMapCmd(server *mapserver.Server) tea.Cmd {
	return func() tea.Msg {
		url, err := server.OpenURL()
		if err != nil {
			return mapOpenedMsg("World map: " + err.Error())
		}
		var command *exec.Cmd
		switch runtime.GOOS {
		case "windows":
			command = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
		case "darwin":
			command = exec.Command("open", url)
		default:
			command = exec.Command("xdg-open", url)
		}
		// Browser output must never corrupt the terminal UI. Reap the short
		// launcher asynchronously; the URL remains available on launch failure.
		if err = command.Start(); err != nil {
			return mapOpenedMsg(fmt.Sprintf("Open map in browser: %s (%v)", url, err))
		}
		go command.Wait()
		return mapOpenedMsg("World map: " + url + " (Ctrl+L reopens)")
	}
}

func (m *model) publishMapState() {
	if m.mapServer == nil {
		return
	}
	filters := m.clusterFilters
	filters.Bands = make(map[string]bool, len(m.clusterFilters.Bands))
	for band, on := range m.clusterFilters.Bands {
		filters.Bands[band] = on
	}
	encoded, _ := json.Marshal(filters)
	state := mapserver.State{Status: m.clusterStatus, Callsign: m.activeStation.Callsign, FilterKey: string(encoded)}
	if grid, err := ParseGridSquare(m.activeStation.MyGridSquare); err == nil {
		state.Home = &geo.Location{Latitude: grid.Latitude, Longitude: grid.Longitude, Locator: grid.Locator, Source: geo.SourceLocator, Precision: geo.PrecisionLocatorCell}
	}
	state.Allows = func(r spot.Report) bool {
		return filters.allowsSpot(clusterSpot{Callsign: r.DXCall, Spotter: r.SpotterCall, Frequency: strconv.FormatFloat(float64(r.FrequencyHz)/1000, 'f', 3, 64), Comment: r.Comment, Received: r.ReceivedAtUTC})
	}
	m.mapServer.SetState(state)
}
