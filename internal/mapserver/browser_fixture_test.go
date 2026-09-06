package mapserver

import (
	"fmt"
	"os"
	"testing"
	"time"

	"w4gns-logger/internal/geo"
	"w4gns-logger/internal/mapfeed"
	"w4gns-logger/internal/spot"
)

// Opt-in fixture for real browser checks, with no database or cluster login.
// W4GNS_MAP_BROWSER=1 go test ./internal/mapserver -run TestBrowserFixture -v
func TestBrowserFixture(t *testing.T) {
	if os.Getenv("W4GNS_MAP_BROWSER") != "1" {
		t.Skip("opt-in browser fixture")
	}
	store := mapfeed.NewStore(20000)
	loc := func(lat, lon float64) *geo.Location {
		return &geo.Location{Latitude: lat, Longitude: lon, Source: geo.SourceCountryReference, Precision: geo.PrecisionCountryReference}
	}
	for i, r := range []spot.Report{
		{DXCall: "JA1AAA", SpotterCall: "W4GNS", FrequencyHz: 14025000, Band: "20M", DXLocation: loc(36, 138), SpotterLocation: loc(36, -84)},
		{DXCall: "JA1AAA", SpotterCall: "G3AAA", FrequencyHz: 14025000, Band: "20M", DXLocation: loc(36, 138), SpotterLocation: loc(52, 0)},
		{DXCall: "JA2BBB", SpotterCall: "ZL1AAA", FrequencyHz: 21025000, Band: "15M", DXLocation: loc(36, 138), SpotterLocation: loc(-41, 174)},
		{DXCall: "VK2AAA", SpotterCall: "W1AW", FrequencyHz: 7025000, Band: "40M", DXLocation: loc(-33, 151), SpotterLocation: loc(42, -72)},
		{DXCall: "PY1AAA", SpotterCall: "G3AAA", FrequencyHz: 28025000, Band: "10M", DXLocation: loc(-22, -43), SpotterLocation: loc(52, 0)},
		{DXCall: "UNKNOWN", SpotterCall: "W4GNS", FrequencyHz: 14025000, Band: "20M", Comment: "<img src=x onerror=alert(1)>"},
	} {
		r.ReceivedAtUTC = time.Now().Add(-time.Duration(i) * time.Minute)
		store.Add(r)
	}
	s := New(store)
	defer s.Close()
	s.SetState(State{Status: "Fixture cluster · connected", Callsign: "W4GNS", Home: &geo.Location{Latitude: 36, Longitude: -84, Locator: "EM76", Source: geo.SourceLocator}, FilterKey: "fixture", Allows: func(r spot.Report) bool { return r.Band != "40M" }})
	u, err := s.OpenURL()
	if err != nil {
		t.Fatal(err)
	}
	fmt.Println("BROWSER_URL=" + u)
	<-time.After(3 * time.Minute)
}
