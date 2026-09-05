package main

import (
	"bytes"
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func partyEvent(t *testing.T, id string) eventDefinition {
	t.Helper()
	events, err := loadEventCatalog()
	if err != nil {
		t.Fatal(err)
	}
	return events[eventIndex(t, events, id)]
}

func partyQSO(call, sent, received, band string) qso {
	q := validTestQSO()
	q.call, q.stxString, q.srxString, q.band = call, sent, received, band
	q.rstSent, q.rstRcvd = "599", "599"
	q.stationCallsign = "W4GNS"
	q.time = time.Date(2026, 9, 6, 18, 0, 0, 0, time.UTC)
	if len(sent) > 0 && sent[0] >= '0' && sent[0] <= '9' {
		q.time = time.Date(2026, 10, 3, 18, 0, 0, 0, time.UTC)
	}
	q.timeOff = q.time.Add(time.Minute)
	return q
}

func TestPartyScoringSidesAndCountyLines(t *testing.T) {
	for _, tc := range []struct {
		id                   string
		qs                   []qso
		points, mults, bonus int
	}{
		{"CA-QSO-PARTY", []qso{
			partyQSO("W6AW", "001 TN", "101 ALAM", "20M"),
			partyQSO("W6AW", "002 TN", "102 ALAM/CCOS", "20M"), // only CCOS is new
			partyQSO("W6AW", "003 TN", "103 CCOS", "40M"),
			partyQSO("W1AW", "004 TN", "004 CT", "20M"), // neither end in CA
		}, 9, 2, 0},
		{"CA-QSO-PARTY", []qso{
			partyQSO("W1AW", "001 ALAM", "101 CT", "20M"),
			partyQSO("W6AW", "002 ALAM", "102 CCOS", "20M"),
			partyQSO("DL1AW", "003 ALAM", "103 DX", "20M"), // DX has points but no multiplier
			partyQSO("W3AW", "004 ALAM", "104 DC", "20M"),
			partyQSO("W3ZZ", "005 ALAM", "105 MD", "20M"), // DC and MD one mult
		}, 15, 3, 0},
		{"TNQP", []qso{
			partyQSO("K4TCG", "CA", "SHEL", "20M"),
			partyQSO("K4TCG", "CA", "DAVI", "20M"), // another county; bonus once per band
			partyQSO("K4TCG", "CA", "SHEL", "40M"),
			partyQSO("W1AW", "CA", "CT", "20M"),
		}, 9, 3, 200},
		{"TNQP", []qso{
			partyQSO("W1AW", "SHEL", "CT", "20M"),
			partyQSO("W4AW", "SHEL", "DAVI", "20M"),
			partyQSO("VE3AW", "SHEL", "ON", "20M"),
			partyQSO("DL1AW", "SHEL", "DL", "20M"),
			partyQSO("DL2AW", "SHEL", "DX", "20M"),
			partyQSO("KL7AW", "SHEL", "AK", "20M"),
		}, 18, 5, 0},
	} {
		t.Run(tc.id+fmt.Sprint(tc.points), func(t *testing.T) {
			state := newContestState()
			state.event = partyEvent(t, tc.id)
			for _, q := range tc.qs {
				if _, err := state.event.partyCredits(q); err != nil {
					t.Errorf("%s: %v", q.call, err)
				}
				state.record(q)
			}
			got := state.score(state.event.Scoring)
			if got.qsoPoints != tc.points || got.multipliers != tc.mults || got.bonusPoints != tc.bonus {
				t.Fatalf("score = %+v; want %d/%d/%d", got, tc.points, tc.mults, tc.bonus)
			}
		})
	}
}

func TestPartyMobileBonusAndExclusions(t *testing.T) {
	state := newContestState()
	state.event = partyEvent(t, "TNQP")
	state.partyCategory = "MOBILE"
	for i := 0; i < 10; i++ {
		state.record(partyQSO(fmt.Sprintf("W1A%d", i), "SHEL", "CT", "20M"))
	}
	got := state.score(state.event.Scoring)
	if got.total() != 560 || got.bonusPoints != 500 || got.multipliers != 2 {
		t.Fatalf("mobile score = %+v", got)
	}
	state.partyCategory = "FIXED"
	if got := state.score(state.event.Scoring); got.total() != 30 {
		t.Fatalf("fixed score = %+v", got)
	}
	state.partyCategory = "ROVER"
	state.partyQSOs[9].unscored = true
	if got := state.score(state.event.Scoring); got.total() != 27 {
		t.Fatalf("unscored contact must not qualify activation: %+v", got)
	}
	q := partyQSO("W4AW", "SHEL", "DAVI", "30M")
	state.record(q)
	q.band, q.mode = "20M", "FT8"
	state.record(q)
	q.mode, q.stxString = "CW", ""
	state.record(q)
	if got := state.score(state.event.Scoring); got.total() != 27 {
		t.Fatalf("invalid contacts scored: %+v", got)
	}
}

func TestPartyCaliforniaMultiplierCap(t *testing.T) {
	state := newContestState()
	state.event = partyEvent(t, "CA-QSO-PARTY")
	areas := strings.Fields("AL AK AZ AR CO CT DE FL GA HI ID IL IN IA KS KY LA ME MD MA MI MN MS MO MT NE NV NH NJ NM NY NC ND OH OK OR PA RI SC SD TN TX UT VT VA WA WV WI WY AB BC MB NB NL NS NT NU ON PE QC SK YT")
	for i, area := range areas {
		state.record(partyQSO(fmt.Sprintf("W1A%d", i), "001 ALAM", "002 "+area, "20M"))
	}
	state.record(partyQSO("W6AW", "001 ALAM", "002 CCOS", "20M"))
	got := state.score(state.event.Scoring)
	if got.multipliers != 58 || got.qsoPoints != 189 {
		t.Fatalf("cap score = %+v", got)
	}
}

func TestMichiganOhioPartyRules(t *testing.T) {
	for _, tc := range []struct {
		id, county   string
		count, mults int
	}{{"MI-QSO-PARTY", "OAKL", 83, 5}, {"OH-QSO-PARTY", "CUYA", 88, 4}} {
		e := partyEvent(t, tc.id)
		if len(e.CountyOptions) != tc.count {
			t.Fatalf("%s county count = %d", tc.id, len(e.CountyOptions))
		}
		state := newContestState()
		state.event = e
		for i, area := range []string{"DC", "MD", "DX", "DX", "YT", "NU"} {
			q := partyQSO(fmt.Sprintf("W1A%d", i), tc.county, area, "20M")
			q.time = e.QSOParty.Periods[0].Start
			state.record(q)
		}
		if got := state.score(e.Scoring); got.qsoPoints != 12 || got.multipliers != tc.mults {
			t.Errorf("%s score = %+v", tc.id, got)
		}
		if _, err := e.partyExchange("", tc.county+"/"+e.CountyOptions[0].Code); err == nil {
			t.Errorf("%s permits simultaneous counties", tc.id)
		}
	}
}

func TestGeorgiaAlabamaFloridaScoring(t *testing.T) {
	for _, tc := range []struct {
		id, county, power string
		received          []string
		total             int
	}{
		{"GA-QSO-PARTY", "HARR", "LOW", []string{"MUSC", "CT", "DX"}, 12},
		{"AL-QSO-PARTY", "AUTA", "LOW", []string{"BALD", "MDC", "DC", "DL"}, 32},
		{"FCG-FQP", "POL", "QRP", []string{"DAD", "CT", "DC", "MD", "DL", "R1", "2"}, 294},
	} {
		state := newContestState()
		state.event = partyEvent(t, tc.id)
		state.partyPower = tc.power
		for i, received := range tc.received {
			q := partyQSO(fmt.Sprintf("W1A%d", i), tc.county, received, "20M")
			q.time = state.event.QSOParty.Periods[0].Start
			state.record(q)
		}
		if got := state.score(state.event.Scoring); got.total() != tc.total {
			t.Errorf("%s score=%+v total=%d want=%d", tc.id, got, got.total(), tc.total)
		}
	}
	state := newContestState()
	state.event = partyEvent(t, "FCG-FQP")
	state.setStation("N4U")
	state.partyPower = "QRP"
	for i := 0; i < 2; i++ {
		q := partyQSO(fmt.Sprintf("W1A%d", i), "CIT", "CT", "20M")
		q.stationCallsign = "N4U"
		q.time = state.event.QSOParty.Periods[0].Start
		state.record(q)
	}
	if got := state.score(state.event.Scoring); got.total() != 2 {
		t.Fatalf("special station QSO-only score = %+v", got)
	}
}

func TestIowaCountyLineAndBands(t *testing.T) {
	e := partyEvent(t, "IAQP")
	if len(e.CountyOptions) != 99 || containsString(e.Bands, "60M") || !containsString(e.Bands, "160M") {
		t.Fatal("incorrect Iowa counties or eligible bands")
	}
	counties := make([]string, 4)
	for i := range counties {
		counties[i] = e.CountyOptions[i].Code
	}
	state := newContestState()
	state.event = e
	q := partyQSO("W0AW", "TN", strings.Join(counties, "/"), "20M")
	q.time = e.QSOParty.Periods[0].Start
	state.record(q)
	q.srxString = counties[0]
	state.record(q) // Repeating one county must not earn another credit.
	if got := state.score(e.Scoring); got.qsoPoints != 8 || got.multipliers != 4 {
		t.Fatalf("Iowa four-county score = %+v", got)
	}
	if _, err := e.partyExchange("", strings.Join(counties, "/")+"/"+e.CountyOptions[4].Code); err == nil {
		t.Fatal("Iowa accepted five simultaneous counties")
	}
}

func TestPartyExchangeValidation(t *testing.T) {
	for _, id := range []string{"TNQP", "CA-QSO-PARTY"} {
		event := partyEvent(t, id)
		invalid := []string{"", "ZZ!", "UNKNOWN", "SHEL/SHEL", "ALAM/ALAM", "AA/BB", "DL EXTRA", "SHEL\n"}
		for _, value := range invalid {
			if id == "CA-QSO-PARTY" {
				value = "001 " + value
			}
			if _, err := event.partyExchange("", value); err == nil {
				t.Errorf("%s accepted %q", id, value)
			}
		}
	}
	tn := partyEvent(t, "TNQP")
	if _, err := tn.partyExchange("", "SHEL/DAVI/HAMI"); err == nil {
		t.Fatal("TN accepted three-county line")
	}
	ca := partyEvent(t, "CA-QSO-PARTY")
	if _, err := ca.partyExchange("001", "ALAM+CCOS"); err != nil {
		t.Fatal(err)
	}
	if _, err := ca.partyExchange("", "000 ALAM"); err == nil {
		t.Fatal("zero serial accepted")
	}
	if x, err := tn.partyExchange("", "DX:LA"); err != nil || !strings.HasPrefix(x.area, "DX:") {
		t.Fatalf("explicit DX entity: %+v %v", x, err)
	}
}

func TestPartyPeriodBoundariesAndConfig(t *testing.T) {
	event := partyEvent(t, "CA-QSO-PARTY")
	p := event.QSOParty.Periods[0]
	for _, tc := range []struct {
		at    time.Time
		valid bool
	}{{p.Start, true}, {p.End.Add(-time.Second), true}, {p.Start.Add(-time.Second), false}, {p.End, false}} {
		q := partyQSO("W6AW", "001 TN", "101 ALAM", "20M")
		q.time = tc.at
		state := newContestState()
		state.event = event
		state.record(q)
		if (state.score(event.Scoring).total() > 0) != tc.valid {
			t.Errorf("period boundary %v", tc.at)
		}
	}
	copyRules := *event.QSOParty
	event.QSOParty = &copyRules
	event.QSOParty.Periods = nil
	if event.prepareQSOParty() == nil {
		t.Fatal("missing operating periods accepted")
	}
}

func TestPartyEntryMobileCountyChange(t *testing.T) {
	st, err := openStore(filepath.Join(t.TempDir(), "logger.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	m := initialModel(st)
	event := partyEvent(t, "CA-QSO-PARTY")
	m.postMode = true
	m.postFields[postTimestamp].SetValue("2026-10-03 18:00")
	m.selectEvent(event, event.Sessions[0])
	enter := func(county string) {
		m.fields[fieldCall].SetValue("W6AW")
		m.fields[fieldBand].SetValue("20M")
		m.fields[fieldFrequency].SetValue("14.025")
		m.contestFields[contestExchangeSent].SetValue("TN")
		m.contestFields[contestSerialRcvd].SetValue("101")
		m.contestFields[contestExchangeRcvd].SetValue(county)
		m, _ = m.logCurrentQSO()
	}
	enter("ALAM")
	enter("CCOS")
	enter("CCOS")
	if n, err := st.count(m.activeStation.ID); err != nil || n != 2 {
		t.Fatalf("logged=%d err=%v status=%s", n, err, m.statusMsg)
	}
	if !m.dupeWarning {
		t.Fatal("same county repeat was not flagged")
	}
	m.contestFocusIdx = contestExchangeRcvd
	m.contestFields[contestExchangeRcvd].SetValue("ALAM/contra")
	choices := m.exchangeChoices()
	if len(choices) != 1 || choices[0].Code != "ALAM/CCOS" {
		t.Fatalf("county-line autocomplete: %+v", choices)
	}
	m.contestFocusIdx = contestExchangeSent
	m.contestFields[contestExchangeSent].SetValue("Alameda")
	choices = m.exchangeChoices()
	if len(choices) != 1 || choices[0].Code != "ALAM" {
		t.Fatalf("sent county autocomplete: %+v", choices)
	}
}

func TestPartyDatabaseDupeEditExportAndProfile(t *testing.T) {
	st, err := openStore(filepath.Join(t.TempDir(), "logger.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	profile, err := st.activeStationProfile()
	if err != nil {
		t.Fatal(err)
	}
	profile.Callsign, profile.CategoryStation = "W4GNS", "MOBILE"
	profile, err = st.saveStationProfile(profile)
	if err != nil {
		t.Fatal(err)
	}
	reloaded, err := st.activeStationProfile()
	if err != nil || reloaded.CategoryStation != "MOBILE" {
		t.Fatalf("category persistence: %+v %v", reloaded, err)
	}
	event := partyEvent(t, "CA-QSO-PARTY")
	q := partyQSO("W6AW", "001 TN", "101 ALAM", "20M")
	q.profileID, q.contestID = profile.ID, event.ID+"@2026"
	id, err := st.insertQSO(q)
	if err != nil {
		t.Fatal(err)
	}
	check := func(candidate qso, exclude int64, since time.Time, want bool) {
		t.Helper()
		got, err := st.isPartyDupe(candidate, event, exclude, since)
		if err != nil || got != want {
			t.Fatalf("dupe=%v want=%v err=%v", got, want, err)
		}
	}
	q.stx, q.srx, q.stxString, q.srxString = "002", "102", "TN", "ALAM"
	check(q, 0, time.Time{}, true) // serial does not change duplicate identity
	check(q, id, time.Time{}, false)
	check(q, 0, q.time.Add(time.Second), false)
	q.srxString = "ALAM/CCOS"
	check(q, 0, time.Time{}, false)
	if _, err := st.insertQSO(q); err != nil {
		t.Fatal(err)
	}
	check(q, 0, time.Time{}, true)
	q.profileID++
	check(q, 0, time.Time{}, false)
	q.profileID--
	q.contestID = event.ID + "@2025"
	check(q, 0, time.Time{}, false)
	q.contestID = event.ID + "@2026"
	var buf bytes.Buffer
	n, score, err := exportCabrillo(context.Background(), &buf, profile, event, q.contestID, st)
	if err != nil || n != 2 || score.total() != 12 {
		t.Fatalf("export: n=%d score=%+v err=%v", n, score, err)
	}
	if strings.Count(buf.String(), "\r\nQSO:") != 3 || strings.Contains(buf.String(), "ALAM/CCOS") {
		t.Fatalf("county-line QSO was not expanded: %s", buf.String())
	}
	if !strings.Contains(buf.String(), "CLAIMED-SCORE: 12") {
		t.Fatal(buf.String())
	}
	state, err := buildContestState(context.Background(), profile.ID, profile.Callsign, q.contestID, st)
	if err != nil {
		t.Fatal(err)
	}
	if state.score(event.Scoring) != score {
		t.Fatal("live/rebuilt and exported scores disagree")
	}
	q.srxString = "ORAN"
	fresh, worked := state.partyNewMultiplier(q)
	if !fresh || worked {
		t.Fatal("new county not indicated")
	}
	q.srxString = "ALAM/ORAN"
	fresh, worked = state.partyNewMultiplier(q)
	if !fresh || !worked {
		t.Fatal("partial county-line multiplier not indicated")
	}
}
