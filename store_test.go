package main

import (
	"context"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestOpenStoreCreatesStationProfileAndCurrentColumns(t *testing.T) {
	st, err := openStore(filepath.Join(t.TempDir(), "logger.db"))
	if err != nil {
		t.Fatalf("openStore returned error: %v", err)
	}
	defer st.Close()

	var profiles int
	if err := st.db.QueryRow(`SELECT COUNT(*) FROM station_profile`).Scan(&profiles); err != nil {
		t.Fatalf("count station profiles: %v", err)
	}
	if profiles != 1 {
		t.Fatalf("station profiles = %d, want 1", profiles)
	}
	for _, column := range []string{"my_gridsquare", "profile_id"} {
		exists, err := st.qsoColumnExists(column)
		if err != nil {
			t.Fatalf("qsoColumnExists(%q) returned error: %v", column, err)
		}
		if !exists {
			t.Errorf("qso column %q is missing", column)
		}
	}
}

func TestDefaultTimezoneIsConcrete(t *testing.T) {
	timezone := defaultTimezone()
	if timezone == "" || timezone == "Local" {
		t.Errorf("defaultTimezone() = %q, want a concrete IANA zone or UTC", timezone)
	}
	if _, err := time.LoadLocation(timezone); err != nil {
		t.Errorf("defaultTimezone() = %q cannot be loaded: %v", timezone, err)
	}
}

func TestInsertQSOBatchHandlesMultipleTransactions(t *testing.T) {
	st, err := openStore(filepath.Join(t.TempDir(), "logger.db"))
	if err != nil {
		t.Fatalf("openStore returned error: %v", err)
	}
	defer st.Close()

	qsos := make([]qso, importBatchSize*2+1)
	for i := range qsos {
		q := validTestQSO()
		q.call = "W1" + strconv.Itoa(i)
		q.time = q.time.Add(time.Duration(i) * time.Second)
		q.timeOff = q.time.Add(time.Second)
		qsos[i] = q
	}
	if err := st.insertQSOBatch(context.Background(), qsos); err != nil {
		t.Fatalf("insertQSOBatch returned error: %v", err)
	}
	count, err := st.count()
	if err != nil {
		t.Fatalf("count returned error: %v", err)
	}
	if count != len(qsos) {
		t.Errorf("QSO count = %d, want %d", count, len(qsos))
	}
}

func TestDupeCheckUsesFifteenMinuteWindow(t *testing.T) {
	st, err := openStore(filepath.Join(t.TempDir(), "logger.db"))
	if err != nil {
		t.Fatalf("openStore returned error: %v", err)
	}
	defer st.Close()
	now := time.Date(2026, time.August, 31, 18, 0, 0, 0, time.UTC)
	q := validTestQSO()
	q.call, q.band = "W4GNS", "20M"
	q.time = now.Add(-14 * time.Minute)
	q.timeOff = q.time.Add(time.Minute)
	if _, err := st.insertQSO(q); err != nil {
		t.Fatalf("insert current-window QSO: %v", err)
	}
	dupe, err := st.isDupe("W4GNS", "20M", now)
	if err != nil || !dupe {
		t.Fatalf("dupe inside window = %t, err = %v", dupe, err)
	}
	q.call = "K1ABC"
	q.time = now.Add(-16 * time.Minute)
	q.timeOff = q.time.Add(time.Minute)
	if _, err := st.insertQSO(q); err != nil {
		t.Fatalf("insert older QSO: %v", err)
	}
	dupe, err = st.isDupe("K1ABC", "20M", now)
	if err != nil || dupe {
		t.Fatalf("dupe outside window = %t, err = %v", dupe, err)
	}
}

func TestQSOsByCallReturnsOnlyPriorContacts(t *testing.T) {
	st, err := openStore(filepath.Join(t.TempDir(), "logger.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	for _, call := range []string{"W4GNS", "K1ABC", "W4GNS"} {
		q := validTestQSO()
		q.call = call
		if _, err := st.insertQSO(q); err != nil {
			t.Fatal(err)
		}
	}
	contacts, err := st.qsosByCall("W4GNS")
	if err != nil {
		t.Fatal(err)
	}
	if len(contacts) != 2 {
		t.Fatalf("contacts = %d, want 2", len(contacts))
	}
}

func TestInsertQSOPersistsDetailAndContestFields(t *testing.T) {
	st, err := openStore(filepath.Join(t.TempDir(), "logger.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	q := validTestQSO()
	q.frequency, q.name, q.qth, q.grid, q.state, q.comment = "14.025", "Pat", "Raleigh", "FM05", "NC", "Great signal"
	q.contestID, q.stx, q.stxString, q.srx, q.srxString = "NAQP", "001", "NC", "014", "VA"
	if _, err := st.insertQSO(q); err != nil {
		t.Fatal(err)
	}
	var frequency, name, qth, grid, state, comment, contest, stx, stxString, srx, srxString string
	if err := st.db.QueryRow(`SELECT freq, name, qth, gridsquare, state, comment, contest_id, stx, stx_string, srx, srx_string FROM qso`).Scan(&frequency, &name, &qth, &grid, &state, &comment, &contest, &stx, &stxString, &srx, &srxString); err != nil {
		t.Fatal(err)
	}
	if got, want := []string{frequency, name, qth, grid, state, comment, contest, stx, stxString, srx, srxString}, []string{"14.025", "Pat", "Raleigh", "FM05", "NC", "Great signal", "NAQP", "001", "NC", "014", "VA"}; strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("stored detail fields = %#v, want %#v", got, want)
	}
}
