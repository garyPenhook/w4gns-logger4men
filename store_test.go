package main

import (
	"context"
	"database/sql"
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

// TestBackfillAssignsLegacyQSOsToDefaultProfile simulates a database created
// before station_profile/profile_id existed: a QSO row with a NULL
// profile_id must still be assigned to a profile on open, or it silently
// disappears from ADIF exports (which filter WHERE profile_id = ?).
func TestBackfillAssignsLegacyQSOsToDefaultProfile(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "logger.db")
	st, err := openStore(dbPath)
	if err != nil {
		t.Fatalf("openStore returned error: %v", err)
	}
	if _, err := st.db.Exec(`INSERT INTO qso (call, qso_date, time_on, band, mode) VALUES (?, ?, ?, ?, ?)`,
		"W4GNS", "20260101", "120000", "20M", "CW"); err != nil {
		t.Fatalf("insert legacy row: %v", err)
	}
	st.Close()

	// Reopen so backfillMissingProfileID runs against the row inserted above.
	st, err = openStore(dbPath)
	if err != nil {
		t.Fatalf("reopen returned error: %v", err)
	}
	defer st.Close()

	var profileID sql.NullInt64
	if err := st.db.QueryRow(`SELECT profile_id FROM qso WHERE call = 'W4GNS'`).Scan(&profileID); err != nil {
		t.Fatalf("scan profile_id: %v", err)
	}
	if !profileID.Valid {
		t.Fatal("legacy QSO still has a NULL profile_id after reopen")
	}

	exported, err := st.qsosForProfile(context.Background(), profileID.Int64)
	if err != nil {
		t.Fatalf("qsosForProfile returned error: %v", err)
	}
	if len(exported) != 1 {
		t.Fatalf("qsosForProfile returned %d QSOs, want 1", len(exported))
	}
}

// TestQSOsForProfileToleratesNullEndTime covers a row from an
// intermediate schema where qso_date_off/time_off exist but were never
// populated (NULL), which previously failed to scan into a plain string.
func TestQSOsForProfileToleratesNullEndTime(t *testing.T) {
	st, err := openStore(filepath.Join(t.TempDir(), "logger.db"))
	if err != nil {
		t.Fatalf("openStore returned error: %v", err)
	}
	defer st.Close()

	if _, err := st.db.Exec(`INSERT INTO qso (call, qso_date, time_on, band, mode, profile_id) VALUES (?, ?, ?, ?, ?, ?)`,
		"W4GNS", "20260101", "120000", "20M", "CW", 1); err != nil {
		t.Fatalf("insert row with NULL end time: %v", err)
	}

	exported, err := st.qsosForProfile(context.Background(), 1)
	if err != nil {
		t.Fatalf("qsosForProfile returned error: %v", err)
	}
	if len(exported) != 1 {
		t.Fatalf("qsosForProfile returned %d QSOs, want 1", len(exported))
	}
	if exported[0].timeOff.Before(exported[0].time) {
		t.Fatalf("timeOff %v is before time %v", exported[0].timeOff, exported[0].time)
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
	dupe, err := st.isDupe("W4GNS", "20M", "", "", "", now)
	if err != nil || !dupe {
		t.Fatalf("dupe inside window = %t, err = %v", dupe, err)
	}
	q.call = "K1ABC"
	q.time = now.Add(-16 * time.Minute)
	q.timeOff = q.time.Add(time.Minute)
	if _, err := st.insertQSO(q); err != nil {
		t.Fatalf("insert older QSO: %v", err)
	}
	dupe, err = st.isDupe("K1ABC", "20M", "", "", "", now)
	if err != nil || dupe {
		t.Fatalf("dupe outside window = %t, err = %v", dupe, err)
	}
}

// TestDupeCheckHonorsCallBandSessionScope covers CWT/CW Open-style contests,
// where dupe_scope is "call+band+session": working the same station again in
// a *different* session is not a dupe, even seconds after the first QSO.
func TestDupeCheckHonorsCallBandSessionScope(t *testing.T) {
	st, err := openStore(filepath.Join(t.TempDir(), "logger.db"))
	if err != nil {
		t.Fatalf("openStore returned error: %v", err)
	}
	defer st.Close()
	now := time.Date(2026, time.August, 31, 18, 0, 0, 0, time.UTC)
	q := validTestQSO()
	q.call, q.band, q.contestID = "W4GNS", "20M", "CWT-1900"
	q.time, q.timeOff = now, now.Add(time.Minute)
	if _, err := st.insertQSO(q); err != nil {
		t.Fatalf("insert QSO: %v", err)
	}

	dupe, err := st.isDupe("W4GNS", "20M", "CWT-1900", "CWT", "call+band+session", now)
	if err != nil || !dupe {
		t.Fatalf("same-session dupe = %t, err = %v", dupe, err)
	}
	dupe, err = st.isDupe("W4GNS", "20M", "CWT-0300", "CWT", "call+band+session", now)
	if err != nil || dupe {
		t.Fatalf("different-session dupe = %t, want false, err = %v", dupe, err)
	}
}

// TestDupeCheckHonorsCallBandContestScope covers the majority (call+band)
// dupe_scope: a dupe spans the whole contest (any session), and is not
// bounded by the casual-logging 15-minute window.
func TestDupeCheckHonorsCallBandContestScope(t *testing.T) {
	st, err := openStore(filepath.Join(t.TempDir(), "logger.db"))
	if err != nil {
		t.Fatalf("openStore returned error: %v", err)
	}
	defer st.Close()
	now := time.Date(2026, time.August, 31, 18, 0, 0, 0, time.UTC)
	q := validTestQSO()
	q.call, q.band, q.contestID = "W4GNS", "20M", "ARRL-DX-CW"
	q.time = now.Add(-3 * time.Hour)
	q.timeOff = q.time.Add(time.Minute)
	if _, err := st.insertQSO(q); err != nil {
		t.Fatalf("insert QSO: %v", err)
	}

	dupe, err := st.isDupe("W4GNS", "20M", "ARRL-DX-CW", "ARRL-DX-CW", "call+band", now)
	if err != nil || !dupe {
		t.Fatalf("whole-contest dupe (3h later) = %t, err = %v", dupe, err)
	}
	dupe, err = st.isDupe("W4GNS", "15M", "ARRL-DX-CW", "ARRL-DX-CW", "call+band", now)
	if err != nil || dupe {
		t.Fatalf("different-band dupe = %t, want false, err = %v", dupe, err)
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
	q.potaRef = "US-1234"
	q.contestID, q.stx, q.stxString, q.srx, q.srxString = "NAQP", "001", "NC", "014", "VA"
	if _, err := st.insertQSO(q); err != nil {
		t.Fatal(err)
	}
	var frequency, name, qth, grid, state, sig, sigInfo, comment, contest, stx, stxString, srx, srxString string
	if err := st.db.QueryRow(`SELECT freq, name, qth, gridsquare, state, sig, sig_info, comment, contest_id, stx, stx_string, srx, srx_string FROM qso`).Scan(&frequency, &name, &qth, &grid, &state, &sig, &sigInfo, &comment, &contest, &stx, &stxString, &srx, &srxString); err != nil {
		t.Fatal(err)
	}
	if got, want := []string{frequency, name, qth, grid, state, sig, sigInfo, comment, contest, stx, stxString, srx, srxString}, []string{"14.025", "Pat", "Raleigh", "FM05", "NC", "POTA", "US-1234", "Great signal", "NAQP", "001", "NC", "014", "VA"}; strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("stored detail fields = %#v, want %#v", got, want)
	}
}
