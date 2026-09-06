package main

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"testing"
	"time"

	"w4gns-logger/internal/geo"
	"w4gns-logger/internal/spot"
)

// Regression coverage for the independent September 6 code review.
func TestIndependentAuditChangedCallClearsAwardReferences(t *testing.T) {
	m := reviewModel(t)
	m.fields[fieldCall].SetValue("W1AW")
	m.resetDetailsForCall("W1AW")
	next, _ := m.Update(potaLookupMsg{call: "W1AW", reference: "US-1234", parkName: "First park"})
	m = next.(model)
	m.fields[fieldIOTARef].SetValue("NA-013")
	m.fields[fieldCall].SetValue("K2ABC")
	m.resetDetailsForCall("K2ABC")
	next, _ = m.Update(potaLookupMsg{call: "K2ABC", reference: "US-5678", parkName: "Second park"})
	m = next.(model)
	m, _ = m.logCurrentQSO()
	qs, err := m.store.qsosForProfile(context.Background(), m.activeStation.ID)
	if err != nil || len(qs) != 1 {
		t.Fatalf("save: %v, %s", err, m.statusMsg)
	}
	if qs[0].potaRef != "US-5678" || qs[0].iotaRef != "" {
		t.Fatalf("saved %s with previous station's references: POTA=%q IOTA=%q", qs[0].call, qs[0].potaRef, qs[0].iotaRef)
	}
}

func TestIndependentAuditAwardReferencesPreservedForSameCallAndEdits(t *testing.T) {
	m := reviewModel(t)
	m.detailsCall = "W1AW"
	m.fields[fieldPOTARef].SetValue("US-1234")
	m.fields[fieldIOTARef].SetValue("NA-013")
	m.resetDetailsForCall("W1AW")
	if m.fields[fieldPOTARef].Value() != "US-1234" || m.fields[fieldIOTARef].Value() != "NA-013" {
		t.Fatal("returning to the same station discarded its award references")
	}
	m.editingQSOID = 1
	m.resetDetailsForCall("K2ABC")
	if m.fields[fieldPOTARef].Value() != "US-1234" || m.fields[fieldIOTARef].Value() != "NA-013" {
		t.Fatal("correcting an existing QSO discarded its award references")
	}
}

func TestIndependentAuditClusterAwardReferencesFollowChangedCall(t *testing.T) {
	m := reviewModel(t)
	m.clusterSpots = []clusterSpot{
		{Callsign: "W1AW", Comment: "US-1234 NA-013", Received: time.Now()},
		{Callsign: "K2ABC", Comment: "US-5678 EU-005", Received: time.Now()},
	}
	for _, call := range []string{"W1AW", "K2ABC", "W1AW"} {
		m.fields[fieldCall].SetValue(call)
		m.resetDetailsForCall(call)
		_ = m.autoFillPOTAReference() // synchronous cluster autofill; no network
		park, island := "US-1234", "NA-013"
		if call == "K2ABC" {
			park, island = "US-5678", "EU-005"
		}
		if m.fields[fieldPOTARef].Value() != park || m.fields[fieldIOTARef].Value() != island {
			t.Fatalf("%s references: %s / %s", call, m.fields[fieldPOTARef].Value(), m.fields[fieldIOTARef].Value())
		}
	}
}

func TestIndependentAuditPendingParkQueueBoundsAndLateEnrichment(t *testing.T) {
	m := reviewModel(t)
	for i := 0; i < potaPendingTotalCap+5; i++ {
		next, _ := m.Update(clusterLineMsg{generation: m.clusterGeneration, line: fmt.Sprintf("DX de W3LPL: 14025.0 W1AW CW US-%06d", i)})
		m = next.(model)
	}
	total := 0
	for _, pending := range m.pendingPOTASpots {
		total += len(pending)
	}
	if total != potaPendingTotalCap || m.mapReports.Len() != 5 {
		t.Fatalf("queued=%d published=%d", total, m.mapReports.Len())
	}
	for _, pending := range m.pendingPOTASpots {
		for i := range pending {
			pending[i].cspot.Received = time.Now().Add(-potaParkLookupTimeout - time.Second)
		}
	}
	next, _ := m.Update(uploadDrainMsg{})
	m = next.(model)
	if len(m.pendingPOTASpots) != 0 || m.mapReports.Len() != potaPendingTotalCap+5 {
		t.Fatal("timed-out queued spots were not published")
	}
	next, _ = m.Update(potaGeoMsg{reference: "US-000000", record: potaParkRecord{Latitude: 35, Longitude: -85}})
	m = next.(model)
	matched := 0
	for _, r := range m.mapReports.Snapshot() {
		if r.DXLocation != nil && r.DXLocation.Source == geo.SourcePOTAPark {
			if r.Comment != "CW US-000000" {
				t.Fatal("park enrichment changed a different activation")
			}
			matched++
		}
	}
	if matched != 1 {
		t.Fatalf("late park enrichment matched %d reports", matched)
	}
}

func TestIndependentAuditSingleParkQueueLimitAndSharedLookup(t *testing.T) {
	m := reviewModel(t)
	for i := 0; i < potaPendingSpotCap+3; i++ {
		next, _ := m.Update(clusterLineMsg{generation: m.clusterGeneration, line: "DX de W3LPL: 14025.0 W1AW CW US-1234"})
		m = next.(model)
	}
	if len(m.potaGeoCache.pending) != 1 || len(m.pendingPOTASpots["US-1234"]) != potaPendingSpotCap || m.mapReports.Len() != 3 {
		t.Fatal("repeated park spots did not share a bounded lookup queue")
	}
	next, _ := m.Update(potaGeoMsg{reference: "US-1234", record: potaParkRecord{Latitude: 35, Longitude: -85}})
	m = next.(model)
	if m.mapReports.Len() != potaPendingSpotCap+3 || len(m.pendingPOTASpots) != 0 {
		t.Fatal("completed lookup lost waiting reports")
	}
	for _, r := range m.mapReports.Snapshot() {
		if r.DXLocation == nil || r.DXLocation.Source != geo.SourcePOTAPark {
			t.Fatal("park result did not enrich both waiting and overflow spots")
		}
	}
}

func TestIndependentAuditQRZEnrichesSpottersWithoutMovingParks(t *testing.T) {
	m := reviewModel(t)
	park := &geo.Location{Latitude: 35, Longitude: -85, Source: geo.SourcePOTAPark}
	id := m.mapReports.Add(spot.Report{DXCall: "W1AW", SpotterCall: "W1AW", DXLocation: park, ReceivedAtUTC: time.Now()})
	before := m.mapReports.Snapshot()[0]
	next, _ := m.Update(qrzMapGeoMsg{call: "W1AW", record: qrzCallsignRecord{hasLatLon: true, latitude: 41.7, longitude: -72.7}})
	m = next.(model)
	after := m.mapReports.Snapshot()[0]
	if after.EventID != id || after.ReceivedAtUTC != before.ReceivedAtUTC || after.DXLocation != park || after.SpotterLocation == nil || after.SpotterLocation.Source != geo.SourceQRZProfile {
		t.Fatalf("bad enrichment: %+v", after)
	}
	if before.SpotterLocation != nil {
		t.Fatal("enrichment mutated an earlier snapshot")
	}
}

func TestIndependentAuditInvalidUpstreamLocationsBecomeUnavailable(t *testing.T) {
	for _, pair := range [][2]float64{{math.NaN(), 1}, {1, math.NaN()}, {math.Inf(1), 1}, {1, math.Inf(-1)}, {91, 1}, {-91, 1}, {1, 181}, {1, -181}} {
		qrz := qrzRecordLocation(qrzCallsignRecord{hasLatLon: true, latitude: pair[0], longitude: pair[1]})
		pota := potaRecordLocation(potaParkRecord{Latitude: pair[0], Longitude: pair[1]})
		if qrz != nil || pota != nil {
			t.Fatalf("invalid pair accepted: %v", pair)
		}
	}
}

func TestIndependentAuditCSVFormulaIsText(t *testing.T) {
	m := reviewModel(t)
	q := reviewQSO(m)
	q.contestID = "AUDIT"
	q.srx, q.srxString = "", "=1+1"
	reviewInsert(t, m, q)
	var output strings.Builder
	_, err := exportCSV(context.Background(), &output, m.activeStation, q.contestID, m.store)
	if err != nil {
		t.Fatal(err)
	}
	rows, err := csv.NewReader(strings.NewReader(output.String())).ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	if rows[1][9] == "=1+1" {
		t.Fatalf("CSV received exchange remains an executable formula: %q", rows[1][9])
	}
}

func TestIndependentAuditFullParkCacheDoesNotStrandSpots(t *testing.T) {
	m := reviewModel(t)
	m.potaGeoCache = newGeoCache(1, time.Hour)
	m.potaGeoCache.store("US-0001", nil)
	next, _ := m.Update(clusterLineMsg{generation: m.clusterGeneration, line: "DX de W3LPL: 14025.0 W1AW CW POTA US-0002"})
	m = next.(model)
	if m.mapReports.Len() != 1 || len(m.pendingPOTASpots) != 0 || m.potaGeoCache.isPending("US-0002") {
		t.Fatal("rejected park lookup must publish the spot immediately without queueing it")
	}
}

func TestIndependentAuditQRZResultUpdatesFirstReport(t *testing.T) {
	m := reviewModel(t)
	next, _ := m.Update(clusterLineMsg{generation: m.clusterGeneration, line: "DX de W3LPL: 14025.0 W1AW CW"})
	m = next.(model)
	if m.mapReports.Len() != 1 {
		t.Fatal("fixture spot not accepted")
	}
	next, _ = m.Update(qrzMapGeoMsg{call: "W1AW", record: qrzCallsignRecord{hasLatLon: true, latitude: 41.7, longitude: -72.7, country: "United States"}})
	m = next.(model)
	got := m.mapReports.Snapshot()[0].DXLocation
	if got == nil || got.Source != geo.SourceQRZProfile {
		t.Fatalf("completed QRZ lookup left the existing map report at country-reference location: %+v", got)
	}
}

func TestIndependentAuditNonFiniteQRZCoordinateRejected(t *testing.T) {
	lat, lon, ok := parseQRZLatLon("NaN", "-72.7")
	if !ok {
		return
	}
	m := reviewModel(t)
	m.qrzGeoCache.store("W1AW", qrzRecordLocation(qrzCallsignRecord{hasLatLon: true, latitude: lat, longitude: lon}))
	next, _ := m.Update(clusterLineMsg{generation: m.clusterGeneration, line: "DX de W3LPL: 14025.0 W1AW CW"})
	m = next.(model)
	if _, err := json.Marshal(m.mapReports.Snapshot()); err != nil {
		t.Fatalf("accepted QRZ coordinate poisons SSE report JSON: %v", err)
	}
}
