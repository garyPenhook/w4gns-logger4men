package main

import (
	"strings"
	"testing"
	"time"
)

func validTestQSO() qso {
	return qso{
		call:    "W1AW",
		band:    "20M",
		mode:    "CW",
		time:    time.Date(2026, time.August, 31, 12, 0, 0, 0, time.UTC),
		timeOff: time.Date(2026, time.August, 31, 12, 1, 0, 0, time.UTC),
	}
}

func TestValidateQSORejectsInvalidInput(t *testing.T) {
	tests := []struct {
		name string
		edit func(*qso)
	}{
		{"missing callsign", func(q *qso) { q.call = " " }},
		{"unsupported callsign character", func(q *qso) { q.call = "W1@W" }},
		{"missing band", func(q *qso) { q.band = "" }},
		{"non-CW mode", func(q *qso) { q.mode = "FT8" }},
		{"missing time", func(q *qso) { q.time = time.Time{} }},
		{"missing end time", func(q *qso) { q.timeOff = time.Time{} }},
		{"end before start", func(q *qso) { q.timeOff = q.time.Add(-time.Second) }},
		{"malformed grid square", func(q *qso) { q.grid = "ZZ99" }},
		{"non-grid text in grid", func(q *qso) { q.grid = "hello" }},
		{"bad station callsign character", func(q *qso) { q.stationCallsign = "W4@GNS" }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			q := validTestQSO()
			tt.edit(&q)
			if err := validateQSO(q); err == nil {
				t.Fatal("validateQSO succeeded, want error")
			}
		})
	}
}

func TestInsertQSOValidatesAndStoresUTC(t *testing.T) {
	st, err := openStore(t.TempDir() + "/logger.db")
	if err != nil {
		t.Fatalf("openStore returned error: %v", err)
	}
	defer st.Close()

	invalid := validTestQSO()
	invalid.mode = "FT8"
	if _, err := st.insertQSO(invalid); err == nil || !strings.Contains(err.Error(), "mode must be CW") {
		t.Fatalf("insertQSO invalid mode error = %v", err)
	}

	q := validTestQSO()
	q.time = time.Date(2026, time.August, 31, 8, 30, 45, 0, time.FixedZone("EDT", -4*60*60))
	q.timeOff = q.time.Add(time.Minute)
	if _, err := st.insertQSO(q); err != nil {
		t.Fatalf("insertQSO returned error: %v", err)
	}
	var date, timeOn, dateOff, timeOff string
	if err := st.db.QueryRow(`SELECT qso_date, time_on, qso_date_off, time_off FROM qso WHERE call = ?`, q.call).Scan(&date, &timeOn, &dateOff, &timeOff); err != nil {
		t.Fatalf("read stored QSO: %v", err)
	}
	if date != "20260831" || timeOn != "123045" {
		t.Errorf("stored UTC timestamp = %s %s, want 20260831 123045", date, timeOn)
	}
	if dateOff != "20260831" || timeOff != "123145" {
		t.Errorf("stored UTC end timestamp = %s %s, want 20260831 123145", dateOff, timeOff)
	}
}
