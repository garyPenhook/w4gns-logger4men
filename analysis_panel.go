package main

import (
	"fmt"
	"sort"
	"strings"
)

// analysisPanelMinWidth is the floor below which there's no point rendering
// the panel at all, matching dxSpotsPanelMinWidth's role for DX Spots.
const analysisPanelMinWidth = 30

// analysisPanel renders the contest analysis column shown beside QSO Entry
// (roadmap docs/ROADMAP.md §3 Phase 1 / Appendix B, C): the callsign's
// country/CQ/ITU/continent, beam heading + distance from the station's own
// coordinates, whether it would be a new multiplier, and which bands it's
// already been worked on in the active contest. It renders "" — and the
// caller falls back to the pre-panel layout — when there's no active
// contest, no callsign typed yet, or not enough width, exactly like
// dxSpotsPanel degrades on narrow terminals.
func (m model) analysisPanel(width int) string {
	if width < analysisPanelMinWidth {
		return ""
	}
	event, ok := m.eventForContestID()
	if !ok {
		return ""
	}
	call := normalizeCall(m.fields[fieldCall].Value())
	if call == "" {
		return ""
	}

	var lines []string
	lines = append(lines, helpStyle.Render(truncateToWidth("Analysis: "+event.Name, width)))

	if table, err := sharedDXCCTable(); err == nil {
		if entity, found := table.lookup(call); found {
			loc := fmt.Sprintf("%s  CQ%d ITU%d %s", entity.Country, entity.CQZone, entity.ITUZone, entity.Continent)
			lines = append(lines, truncateToWidth(loc, width))
			if m.activeStation.Latitude != nil && m.activeStation.Longitude != nil &&
				(entity.Latitude != 0 || entity.Longitude != 0) {
				bearing, distanceKm := GreatCircleBearingDistance(
					*m.activeStation.Latitude, *m.activeStation.Longitude,
					entity.Latitude, entity.Longitude,
				)
				lines = append(lines, truncateToWidth(fmt.Sprintf("Bearing %.0f°  %.0f km", bearing, distanceKm), width))
			}
		} else {
			lines = append(lines, truncateToWidth("Country: unknown prefix", width))
		}
	}

	switch {
	case m.dupeWarning:
		lines = append(lines, truncateToWidth(dupeStyle.Render("DUPE — worked before on this band"), width))
	case m.contestIndex != nil:
		if _, worked := m.contestIndex.uniqueCalls[call]; worked {
			lines = append(lines, truncateToWidth("Worked before — not a new mult", width))
		} else if event.Scoring != nil && event.Scoring.Multiplier == "unique_call" {
			lines = append(lines, truncateToWidth(newMultStyle.Render("NEW MULT"), width))
		}
	}

	if m.contestIndex != nil {
		if worked := m.contestIndex.byCall[call]; len(worked) > 0 {
			lines = append(lines, truncateToWidth("Worked: "+strings.Join(workedBands(worked), " "), width))
		}
	}

	return strings.Join(lines, "\n")
}

// workedBands returns the de-duplicated, band-plan-ordered list of bands a
// callsign has been worked on, from a contestState.byCall entry.
func workedBands(qsos []qso) []string {
	seen := make(map[string]bool, len(qsos))
	bands := make([]string, 0, len(qsos))
	for _, q := range qsos {
		band := strings.ToUpper(strings.TrimSpace(q.band))
		if band == "" || seen[band] {
			continue
		}
		seen[band] = true
		bands = append(bands, band)
	}
	sort.Slice(bands, func(i, j int) bool { return bandIndex(bands[i]) < bandIndex(bands[j]) })
	return bands
}
