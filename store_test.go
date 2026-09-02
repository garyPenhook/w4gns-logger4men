package main

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

// TestOpenStoreTightensDatabaseFilePermissions guards against the database
// file being left group- or world-readable by the OS's default create
// permissions (subject to umask): this app stores private QSO data, and
// openStore should self-heal a too-loose mode the same way the QRZ API key
// file does.
func TestOpenStoreTightensDatabaseFilePermissions(t *testing.T) {
	oldUmask := setUmask(0o022) // deliberately loose, like a typical default
	defer setUmask(oldUmask)

	dbPath := filepath.Join(t.TempDir(), "logger.db")
	st, err := openStore(dbPath)
	if err != nil {
		t.Fatalf("openStore returned error: %v", err)
	}
	defer st.Close()

	info, err := os.Stat(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != dbFilePermBits {
		t.Errorf("database file permissions = %o, want %o", info.Mode().Perm(), dbFilePermBits)
	}
}

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
		exists, err := st.columnExists("qso", column)
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

// TestOpenStoreMigratesDatabaseMissingIndexedColumn guards against a
// regression where schema application created indexes (including one on
// profile_id) before migrate() had a chance to add columns a genuinely old
// database predates: CREATE INDEX on a nonexistent column fails outright,
// so openStore would error out before migrate() ever ran. This builds a
// qso table shaped like the pre-profile_id schema directly (bypassing
// openStore) and confirms opening it through openStore still succeeds.
func TestOpenStoreMigratesDatabaseMissingIndexedColumn(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "logger.db")
	raw, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := raw.Exec(`CREATE TABLE qso (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		call TEXT NOT NULL,
		qso_date TEXT NOT NULL,
		time_on TEXT NOT NULL,
		band TEXT NOT NULL,
		mode TEXT DEFAULT 'CW',
		rst_sent TEXT,
		rst_rcvd TEXT,
		contest_id TEXT
	)`); err != nil {
		t.Fatal(err)
	}
	if _, err := raw.Exec(`INSERT INTO qso (call, qso_date, time_on, band, mode) VALUES (?, ?, ?, ?, ?)`,
		"W4GNS", "20260101", "120000", "20M", "CW"); err != nil {
		t.Fatal(err)
	}
	if err := raw.Close(); err != nil {
		t.Fatal(err)
	}

	st, err := openStore(dbPath)
	if err != nil {
		t.Fatalf("openStore on a database predating profile_id returned error: %v", err)
	}
	defer st.Close()

	exists, err := st.columnExists("qso", "profile_id")
	if err != nil {
		t.Fatal(err)
	}
	if !exists {
		t.Fatal("profile_id column was not added by migrate()")
	}
	if count, err := st.count(); err != nil || count != 1 {
		t.Fatalf("count = %d, err = %v, want 1 (the pre-existing legacy row)", count, err)
	}
}

// TestBackfillMissingProfileIDSkipsWriteWhenNoOrphans guards the steady-state
// startup path: once every row has a profile_id, backfillMissingProfileID
// must not issue an UPDATE at all (it should short-circuit on the EXISTS
// check), verified indirectly by confirming a clean store's rows are
// untouched and openStore succeeds with no station_profile rows to backfill
// against beyond the default profile.
func TestBackfillMissingProfileIDSkipsWriteWhenNoOrphans(t *testing.T) {
	st, err := openStore(filepath.Join(t.TempDir(), "logger.db"))
	if err != nil {
		t.Fatalf("openStore returned error: %v", err)
	}
	defer st.Close()

	if err := st.backfillMissingProfileID(); err != nil {
		t.Fatalf("backfillMissingProfileID on an already-clean store returned error: %v", err)
	}

	var orphans int
	if err := st.db.QueryRow(`SELECT COUNT(*) FROM qso WHERE profile_id IS NULL`).Scan(&orphans); err != nil {
		t.Fatalf("count orphans: %v", err)
	}
	if orphans != 0 {
		t.Fatalf("orphan count = %d, want 0", orphans)
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

// TestForEachQSOForProfileStopsOnCallbackError guards the streaming
// contract exportADIF relies on: forEachQSOForProfile must invoke fn once
// per row (not accumulate them into a slice first) and stop as soon as fn
// returns an error, propagating it.
func TestForEachQSOForProfileStopsOnCallbackError(t *testing.T) {
	st, err := openStore(filepath.Join(t.TempDir(), "logger.db"))
	if err != nil {
		t.Fatalf("openStore returned error: %v", err)
	}
	defer st.Close()
	profile, err := st.activeStationProfile()
	if err != nil {
		t.Fatal(err)
	}
	for _, call := range []string{"W1AW", "K1ABC", "N4XYZ"} {
		q := validTestQSO()
		q.call, q.profileID = call, profile.ID
		if _, err := st.insertQSO(q); err != nil {
			t.Fatal(err)
		}
	}

	sentinel := fmt.Errorf("stop after first row")
	seen := 0
	err = st.forEachQSOForProfile(context.Background(), profile.ID, func(qso) error {
		seen++
		return sentinel
	})
	if err != sentinel {
		t.Fatalf("forEachQSOForProfile returned %v, want the callback's sentinel error", err)
	}
	if seen != 1 {
		t.Fatalf("callback was invoked %d times, want 1 (iteration should stop on the first error)", seen)
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
	inserted, err := st.insertQSOBatch(context.Background(), qsos)
	if err != nil {
		t.Fatalf("insertQSOBatch returned error: %v", err)
	}
	if inserted != len(qsos) {
		t.Errorf("insertQSOBatch inserted = %d, want %d", inserted, len(qsos))
	}
	count, err := st.count()
	if err != nil {
		t.Fatalf("count returned error: %v", err)
	}
	if count != len(qsos) {
		t.Errorf("QSO count = %d, want %d", count, len(qsos))
	}
}

// TestInsertQSOBatchSkipsExactDuplicatesOnReimport documents that re-running
// insertQSOBatch with records that already landed (same call/band/qso_date/
// time_on/profile_id) does not double-insert them, so retrying an ADIF import
// after a mid-file failure is safe.
func TestInsertQSOBatchSkipsExactDuplicatesOnReimport(t *testing.T) {
	st, err := openStore(filepath.Join(t.TempDir(), "logger.db"))
	if err != nil {
		t.Fatalf("openStore returned error: %v", err)
	}
	defer st.Close()

	qsos := []qso{validTestQSO(), validTestQSO()}
	qsos[1].call = "K1ABC"

	first, err := st.insertQSOBatch(context.Background(), qsos)
	if err != nil {
		t.Fatalf("first insertQSOBatch returned error: %v", err)
	}
	if first != 2 {
		t.Fatalf("first insertQSOBatch inserted = %d, want 2", first)
	}

	second, err := st.insertQSOBatch(context.Background(), qsos)
	if err != nil {
		t.Fatalf("second insertQSOBatch returned error: %v", err)
	}
	if second != 0 {
		t.Errorf("second insertQSOBatch inserted = %d, want 0 (all duplicates)", second)
	}

	count, err := st.count()
	if err != nil {
		t.Fatalf("count returned error: %v", err)
	}
	if count != 2 {
		t.Errorf("QSO count after reimport = %d, want 2", count)
	}
}

func TestResolveDXCCPrefersImportedValues(t *testing.T) {
	q := validTestQSO() // call = W1AW, resolvable via the embedded cty.dat
	q.country, q.cqZone, q.ituZone, q.dxccNumber = "Imported Land", "5", "9", "777"
	country, cqZone, ituZone, dxccNumber := resolveDXCC(q)
	if country != "Imported Land" {
		t.Errorf("country = %q, want %q (imported value should win over cty.dat lookup)", country, "Imported Land")
	}
	if cqZone != 5 {
		t.Errorf("cqZone = %v, want 5", cqZone)
	}
	if ituZone != 9 {
		t.Errorf("ituZone = %v, want 9", ituZone)
	}
	if dxccNumber != 777 {
		t.Errorf("dxccNumber = %v, want 777", dxccNumber)
	}
}

func TestResolveDXCCFallsBackToLookupWhenNotImported(t *testing.T) {
	q := validTestQSO() // call = W1AW, resolvable via the embedded cty.dat
	country, _, _, dxccNumber := resolveDXCC(q)
	if country == "" {
		t.Error("resolveDXCC() country is empty, want a cty.dat lookup result when q.country is blank")
	}
	if dxccNumber != 291 {
		t.Errorf("dxccNumber = %v, want 291 (United States, resolved from the embedded ARRL DXCC table)", dxccNumber)
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
	dupe, err := st.isDupe("W4GNS", "20M", "", "", "", 0, 0, now)
	if err != nil || !dupe {
		t.Fatalf("dupe inside window = %t, err = %v", dupe, err)
	}
	q.call = "K1ABC"
	q.time = now.Add(-16 * time.Minute)
	q.timeOff = q.time.Add(time.Minute)
	if _, err := st.insertQSO(q); err != nil {
		t.Fatalf("insert older QSO: %v", err)
	}
	dupe, err = st.isDupe("K1ABC", "20M", "", "", "", 0, 0, now)
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

	dupe, err := st.isDupe("W4GNS", "20M", "CWT-1900", "CWT", "call+band+session", 0, 0, now)
	if err != nil || !dupe {
		t.Fatalf("same-session dupe = %t, err = %v", dupe, err)
	}
	dupe, err = st.isDupe("W4GNS", "20M", "CWT-0300", "CWT", "call+band+session", 0, 0, now)
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

	dupe, err := st.isDupe("W4GNS", "20M", "ARRL-DX-CW", "ARRL-DX-CW", "call+band", 0, 0, now)
	if err != nil || !dupe {
		t.Fatalf("whole-contest dupe (3h later) = %t, err = %v", dupe, err)
	}
	dupe, err = st.isDupe("W4GNS", "15M", "ARRL-DX-CW", "ARRL-DX-CW", "call+band", 0, 0, now)
	if err != nil || dupe {
		t.Fatalf("different-band dupe = %t, want false, err = %v", dupe, err)
	}
}

// TestDupeCheckIsScopedToProfile ensures working the same call/band under a
// different station profile is not reported as a dupe: separate profiles
// (e.g. home vs. portable) are logically separate logs.
func TestDupeCheckIsScopedToProfile(t *testing.T) {
	st, err := openStore(filepath.Join(t.TempDir(), "logger.db"))
	if err != nil {
		t.Fatalf("openStore returned error: %v", err)
	}
	defer st.Close()
	now := time.Date(2026, time.August, 31, 18, 0, 0, 0, time.UTC)
	q := validTestQSO()
	q.call, q.band, q.profileID = "W4GNS", "20M", 1
	q.time = now.Add(-time.Minute)
	q.timeOff = q.time.Add(time.Second)
	if _, err := st.insertQSO(q); err != nil {
		t.Fatalf("insert QSO: %v", err)
	}
	dupe, err := st.isDupe("W4GNS", "20M", "", "", "", 1, 0, now)
	if err != nil || !dupe {
		t.Fatalf("same-profile dupe = %t, err = %v", dupe, err)
	}
	dupe, err = st.isDupe("W4GNS", "20M", "", "", "", 2, 0, now)
	if err != nil || dupe {
		t.Fatalf("cross-profile dupe = %t, want false, err = %v", dupe, err)
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

func TestQSOByIDLoadsEditableFields(t *testing.T) {
	st, err := openStore(filepath.Join(t.TempDir(), "logger.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	q := validTestQSO()
	q.name, q.qth = "Pat", "Raleigh"
	id, err := st.insertQSO(q)
	if err != nil {
		t.Fatal(err)
	}

	got, err := st.qsoByID(id)
	if err != nil {
		t.Fatalf("qsoByID returned error: %v", err)
	}
	if got.id != id || got.call != q.call || got.name != "Pat" || got.qth != "Raleigh" {
		t.Fatalf("qsoByID(%d) = %+v, want call=%q name=Pat qth=Raleigh", id, got, q.call)
	}
	// W1AW resolves via the embedded cty.dat/ARRL table; qsoByID must surface
	// that resolved context too, so updateQSO can carry it forward unchanged
	// when the callsign isn't part of an edit.
	if got.country == "" || got.dxccNumber == "" {
		t.Errorf("qsoByID(%d) country/dxccNumber = %q/%q, want both resolved", id, got.country, got.dxccNumber)
	}

	if _, err := st.qsoByID(id + 1000); err == nil {
		t.Fatal("qsoByID returned no error for a nonexistent id")
	}
}

// TestUpdateQSOOverwritesEditableFieldsOnly guards the edit workflow's core
// promise: only the fields shown for editing change; the id, profile_id,
// start/end time, and station-identity snapshot are untouched.
func TestUpdateQSOOverwritesEditableFieldsOnly(t *testing.T) {
	st, err := openStore(filepath.Join(t.TempDir(), "logger.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	q := validTestQSO()
	q.stationCallsign, q.operatorName = "W4GNS", "Gary"
	id, err := st.insertQSO(q)
	if err != nil {
		t.Fatal(err)
	}
	original, err := st.qsoByID(id)
	if err != nil {
		t.Fatal(err)
	}

	edited := original
	edited.call = "K1ABC"
	edited.name = "New Name"
	if err := st.updateQSO(id, edited); err != nil {
		t.Fatalf("updateQSO returned error: %v", err)
	}

	got, err := st.qsoByID(id)
	if err != nil {
		t.Fatal(err)
	}
	if got.call != "K1ABC" || got.name != "New Name" {
		t.Fatalf("updateQSO did not persist edited fields: %+v", got)
	}
	if !got.time.Equal(original.time) || !got.timeOff.Equal(original.timeOff) {
		t.Errorf("updateQSO changed start/end time: got %v/%v, want %v/%v", got.time, got.timeOff, original.time, original.timeOff)
	}
	if got.profileID != original.profileID {
		t.Errorf("updateQSO changed profile_id: got %d, want %d", got.profileID, original.profileID)
	}
	var stationCallsign, operatorName string
	if err := st.db.QueryRow(`SELECT station_callsign, operator_name FROM qso WHERE id = ?`, id).Scan(&stationCallsign, &operatorName); err != nil {
		t.Fatal(err)
	}
	if stationCallsign != "W4GNS" || operatorName != "Gary" {
		t.Errorf("updateQSO touched the station-identity snapshot: callsign=%q operator=%q", stationCallsign, operatorName)
	}
	if count, err := st.count(); err != nil || count != 1 {
		t.Fatalf("count after update = %d, err = %v, want 1 (update must not insert a new row)", count, err)
	}
}

func TestDeleteQSORemovesRow(t *testing.T) {
	st, err := openStore(filepath.Join(t.TempDir(), "logger.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	q := validTestQSO()
	id, err := st.insertQSO(q)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.deleteQSO(id); err != nil {
		t.Fatalf("deleteQSO returned error: %v", err)
	}
	if count, err := st.count(); err != nil || count != 0 {
		t.Fatalf("count after delete = %d, err = %v, want 0", count, err)
	}
	if _, err := st.qsoByID(id); err == nil {
		t.Fatal("qsoByID found a row after deleteQSO")
	}
}

// TestIsDupeExcludesGivenID covers the parameter the edit workflow relies
// on: re-saving an edited QSO must not flag itself as a dupe of its own
// prior state.
func TestIsDupeExcludesGivenID(t *testing.T) {
	st, err := openStore(filepath.Join(t.TempDir(), "logger.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	q := validTestQSO()
	q.call, q.band = "W4GNS", "20M"
	id, err := st.insertQSO(q)
	if err != nil {
		t.Fatal(err)
	}

	dupe, err := st.isDupe("W4GNS", "20M", "", "", "", 0, 0, q.time)
	if err != nil || !dupe {
		t.Fatalf("dupe without exclusion = %t, err = %v, want true", dupe, err)
	}
	dupe, err = st.isDupe("W4GNS", "20M", "", "", "", 0, id, q.time)
	if err != nil || dupe {
		t.Fatalf("dupe excluding its own id = %t, err = %v, want false", dupe, err)
	}
}
