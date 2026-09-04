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
	if m.contestIndexError != "" {
		lines = append(lines, truncateToWidth(dupeStyle.Render("STALE — "+m.contestIndexError), width))
	}

	var entity dxccEntity
	var entityFound bool
	if table, err := sharedDXCCTable(); err == nil {
		if e, found := table.lookup(call); found {
			entity, entityFound = e, true
			loc := fmt.Sprintf("%s  CQ%d ITU%d %s", entity.Country, entity.CQZone, entity.ITUZone, entity.Continent)
			lines = append(lines, truncateToWidth(loc, width))
		} else {
			lines = append(lines, truncateToWidth("Country: unknown prefix", width))
		}
	}
	if m.activeStation.Latitude != nil && m.activeStation.Longitude != nil {
		// A received grid is a station-specific coordinate and therefore more
		// accurate than the broad DXCC entity centroid. Fall back to the entity
		// only when no valid grid has been entered or returned by QRZ.
		targetLat, targetLon := entity.Latitude, entity.Longitude
		haveTarget := entityFound && (targetLat != 0 || targetLon != 0)
		if grid, err := ParseGridSquare(m.detailFields[detailGrid].Value()); err == nil {
			targetLat, targetLon, haveTarget = grid.Latitude, grid.Longitude, true
		}
		if haveTarget {
			bearing, distanceKm := GreatCircleBearingDistance(
				*m.activeStation.Latitude, *m.activeStation.Longitude,
				targetLat, targetLon,
			)
			lines = append(lines, truncateToWidth(fmt.Sprintf("Bearing %.0f°  %.0f km", bearing, distanceKm), width))
		}
	}

	rules := event.effectiveScoring(stationCountry(m.activeStation.Callsign))
	switch {
	case m.dupeWarning:
		lines = append(lines, truncateToWidth(dupeStyle.Render("DUPE — worked before on this band"), width))
	case m.contestIndex != nil && rules != nil:
		band := m.fields[fieldBand].Value()
		exchangeText := m.contestFields[contestExchangeRcvd].Value()
		newMult, workedBefore := m.contestIndex.wouldBeNewMultiplier(rules, call, band, exchangeText, entity, entityFound)
		switch {
		case newMult:
			lines = append(lines, truncateToWidth(newMultStyle.Render("NEW MULT"), width))
		case workedBefore:
			lines = append(lines, truncateToWidth("Worked before — not a new mult", width))
		}
	}

	if m.contestIndex != nil {
		if worked := m.contestIndex.byCall[call]; len(worked) > 0 {
			lines = append(lines, truncateToWidth("Worked: "+strings.Join(workedBands(worked), " "), width))
		}
		if partial := m.checkPartialLine(call, width); partial != "" {
			lines = append(lines, partial)
		}
	}

	return strings.Join(lines, "\n")
}

// checkPartialLimit caps how many Check Partial candidates are shown, so a
// short fragment matching most of the log doesn't blow out the panel width.
const checkPartialLimit = 5

// checkPartialLine renders the Check Partial row (roadmap Appendix B.3):
// prior-logged calls containing the in-progress fragment, so the operator
// can complete a call they only half-caught. Each candidate is colored by
// what logging it *now* — on the band currently selected — would mean:
// newMultStyle (bold) if that call hasn't been worked on this band yet,
// helpStyle (dim) if it's a dupe on this band already.
func (m model) checkPartialLine(fragment string, width int) string {
	matches := m.contestIndex.checkPartial(fragment, checkPartialLimit)
	if len(matches) == 0 {
		return ""
	}
	band := m.qsoBand()
	rendered := make([]string, len(matches))
	for i, candidate := range matches {
		if m.contestIndex.isWorkedOnBand(candidate, band) {
			rendered[i] = helpStyle.Render(candidate)
		} else {
			rendered[i] = newMultStyle.Render(candidate)
		}
	}
	return truncateToWidth("Partial: "+strings.Join(rendered, " "), width)
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
