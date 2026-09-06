package main

import (
	"bufio"
	"encoding/json"
	"net/http"
	"net/http/cookiejar"
	"strings"
	"testing"
	"time"

	"w4gns-logger/internal/mapfeed"
	"w4gns-logger/internal/mapserver"
	"w4gns-logger/internal/spot"
)

func TestMapFilterFrequencyUnits(t *testing.T) {
	filters := defaultClusterFilters()
	r := spot.Report{FrequencyHz: 14025000, DXCall: "W1AW", SpotterCall: "G3AAA", Band: "20M", ReceivedAtUTC: time.Now()}
	store := mapfeed.NewStore(20)
	store.Add(r)
	server := mapserver.New(store)
	defer server.Close()
	m := model{clusterFilters: filters, mapServer: server}
	m.activeStation.MyGridSquare = "EM76"
	m.activeStation.Callsign = "W4GNS"
	m.publishMapState()
	// Mutating the UI's band map must not mutate the published filter closure.
	m.clusterFilters.Bands["20M"] = false
	u, err := server.OpenURL()
	if err != nil {
		t.Fatal(err)
	}
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar, Timeout: 5 * time.Second}
	res, err := client.Get(u)
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	res, err = client.Get(strings.Split(u, "/launch")[0] + "/events")
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	scanner := bufio.NewScanner(res.Body)
	if !scanner.Scan() {
		t.Fatal("missing snapshot")
	}
	var p struct {
		Reports []struct{ MatchesLogger bool }
		State   mapserver.State
	}
	if err = json.Unmarshal([]byte(strings.TrimPrefix(scanner.Text(), "data: ")), &p); err != nil {
		t.Fatal(err)
	}
	if len(p.Reports) != 1 || !p.Reports[0].MatchesLogger {
		t.Fatal("Hz conversion or immutable filter capture failed")
	}
	if p.State.Home == nil || p.State.Home.Locator != "EM76" || p.State.Callsign != "W4GNS" {
		t.Fatal("missing home profile")
	}
}
