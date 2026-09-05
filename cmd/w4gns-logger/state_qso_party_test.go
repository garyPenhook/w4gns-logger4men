package main

import "testing"

func TestSharedCountyScoring(t *testing.T) {
	event := eventDefinition{CountyOptions: []exchangeOption{{Code: "AAAA", Name: "First"}, {Code: "BBBB", Name: "Second"}}, DupeScope: "call+band"}
	if err := event.prepareCountyOptions(); err != nil {
		t.Fatal(err)
	}
	state := newContestState()
	state.event = event
	for _, q := range []qso{
		{call: "W1AW", band: "20M", srxString: "aaaa"},
		{call: "W1AW", band: "20M", srxString: "BBBB"}, // duplicate cannot add a county
		{call: "W1AW", band: "40M", srxString: "AAAA"},
		{call: "W2AW", band: "20M", srxString: "BBBB", unscored: true},
	} {
		state.record(q)
	}
	for _, tc := range []struct {
		per  string
		want int
	}{{"band", 2}, {"contest", 1}} {
		rules := &scoringRules{PointsPerQSO: 3, Multipliers: []multiplierRule{{Kind: "county", Per: tc.per}}}
		got := state.score(rules)
		if got.qsoPoints != 6 || got.multipliers != tc.want {
			t.Fatalf("%s: score = %+v", tc.per, got)
		}
		fresh, worked := state.wouldBeNewMultiplier(rules, "W3AW", "10M", "AAAA", dxccEntity{}, false)
		if fresh != (tc.per == "band") || worked != (tc.per == "contest") {
			t.Fatalf("%s: new=%v worked=%v", tc.per, fresh, worked)
		}
	}
	for _, invalid := range []string{"First", "CA", "AAAA BBBB", "001 AAAA", ""} {
		if event.countyCode(invalid) != "" {
			t.Errorf("accepted %q", invalid)
		}
	}
}

func TestCountyCatalogValidation(t *testing.T) {
	for _, event := range []eventDefinition{
		{Scoring: &scoringRules{Multipliers: []multiplierRule{{Kind: "county", Per: "band"}}}},
		{CountyOptions: []exchangeOption{{Code: "AAA"}, {Code: "aaa"}}},
		{CountyOptions: []exchangeOption{{Code: "A B"}}},
		{CountyOptions: []exchangeOption{{Code: "AAA"}}, Scoring: &scoringRules{Multipliers: []multiplierRule{{Kind: "county", Per: "band_weighted"}}}},
	} {
		if event.prepareCountyOptions() == nil {
			t.Errorf("accepted invalid event %+v", event)
		}
	}
	events, err := loadEventCatalog()
	if err != nil {
		t.Fatal(err)
	}
	tn := events[eventIndex(t, events, "TNQP")]
	if len(tn.CountyOptions) != 95 || len(tn.ReceivedExchangeOptions) != 95 || tn.countyCode("shel") != "SHEL" {
		t.Fatal("Tennessee county scoring and autocomplete must share the 95 county table")
	}
	ca := events[eventIndex(t, events, "CA-QSO-PARTY")]
	if !ca.SentSerial || ca.Scoring == nil || ca.QSOParty == nil || len(ca.CountyOptions) != 58 {
		t.Fatal("California requires serial entry, scoring and all 58 counties")
	}
}
