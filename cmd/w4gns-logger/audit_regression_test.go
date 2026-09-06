package main

import (
	"bytes"
	"context"
	"math"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestAuditLegacyOptionalFields(t *testing.T) {
	st := openTestStore(t)
	if _, err := st.db.Exec(`INSERT INTO qso(call,band,qso_date,time_on,profile_id) VALUES('W1AW','20M','20260906','120000',1)`); err != nil {
		t.Fatal(err)
	}
	for _, read := range []func() ([]qso, error){
		func() ([]qso, error) { return st.recentQSOs(1, 50) },
		func() ([]qso, error) { return st.qsosByCall(1, "W1AW") },
	} {
		rows, err := read()
		if err != nil || len(rows) != 1 {
			t.Fatalf("read legacy QSO: rows=%d, err=%v", len(rows), err)
		}
	}
}

func TestAuditClearStationGrid(t *testing.T) {
	st := openTestStore(t)
	p, err := st.activeStationProfile()
	if err != nil {
		t.Fatal(err)
	}
	p.MyGridSquare = "FN31PR"
	p, err = st.saveStationProfile(p)
	if err != nil {
		t.Fatal(err)
	}
	p.MyGridSquare = ""
	p, err = st.saveStationProfile(p)
	if err != nil {
		t.Fatal(err)
	}
	if p.Latitude != nil || p.Longitude != nil {
		t.Fatal("clearing the grid retained coordinates in the saved profile")
	}
	p.ID++
	if _, err := st.saveStationProfile(p); err == nil {
		t.Error("saving a nonexistent profile reported success")
	}
}

func TestAuditWrongProfileCannotRemoveUploads(t *testing.T) {
	st := openTestStore(t)
	q := validTestQSO()
	id, err := st.insertQSOWithUploads(q, []string{uploadDestQRZ}, time.Now().Add(-time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if err := st.deleteQSO(q.profileID+1, id); err == nil {
		t.Error("deleting another profile's QSO reported success")
	}
	if _, err := st.qsoByID(q.profileID, id); err != nil {
		t.Fatal(err)
	}
	entries, err := st.claimDueUploads(time.Now(), time.Minute, 10)
	if err != nil || len(entries) != 1 {
		t.Fatalf("other profile's upload was lost: entries=%v, err=%v", entries, err)
	}
	q.profileID++
	if err := st.updateQSO(id, q); err == nil {
		t.Error("updating another profile's QSO reported success")
	}
}

func TestAuditExportCannotReplaceDatabase(t *testing.T) {
	for _, uri := range []bool{false, true} {
		dir := t.TempDir()
		path := filepath.Join(dir, "log with spaces.db")
		dsn := path
		if uri {
			dsn = (&url.URL{Scheme: "file", Path: filepath.ToSlash(path), RawQuery: "mode=rwc"}).String()
		}
		st, err := openStore(dsn)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { st.Close() })
		if !exportTargetCollidesWithDB(path, dsn) {
			t.Errorf("collision check missed database DSN %q", dsn)
		}
		profile, err := st.activeStationProfile()
		if err != nil {
			t.Fatal(err)
		}
		events, err := loadEventCatalog()
		if err != nil {
			t.Fatal(err)
		}
		event := events[eventIndex(t, events, "CWT")]
		for _, target := range []string{path, path + "-wal", path + "-shm"} {
			if _, err := writeADIFAtomic(context.Background(), dir, target, profile.ID, st); err == nil {
				t.Errorf("ADIF export overwrote %s", target)
			}
			if _, err := writeCSVAtomic(context.Background(), dir, target, profile, "CWT", st); err == nil {
				t.Errorf("CSV export overwrote %s", target)
			}
			if _, _, err := writeCabrilloAtomic(context.Background(), dir, target, profile, event, "CWT", st); err == nil || !strings.Contains(err.Error(), "SQLite database") {
				t.Errorf("Cabrillo export did not reject database target %s: %v", target, err)
			}
		}
		contents, err := os.ReadFile(path)
		if err != nil || !bytes.HasPrefix(contents, []byte("SQLite format 3\x00")) {
			t.Fatalf("database was replaced: %v", err)
		}
	}
}

func TestAuditSQLiteDSNFileHandling(t *testing.T) {
	t.Chdir(t.TempDir())
	for _, dsn := range []string{":memory:", "file::memory:", "file:audit-memory?mode=memory&cache=shared"} {
		st, err := openStore(dsn)
		if err != nil {
			t.Fatal(err)
		}
		if st.readDB != nil {
			t.Errorf("memory database %q opened a separate read connection", dsn)
		}
		st.Close()
	}
	for _, mode := range []string{"rw", "ro"} {
		if st, err := openStore("file:missing.db?mode=" + mode); err == nil {
			st.Close()
			t.Errorf("mode=%s created a missing database", mode)
		}
	}
	entries, err := os.ReadDir(".")
	if err != nil || len(entries) != 0 {
		t.Fatalf("memory/missing database DSNs created unexpected files: %v, %v", entries, err)
	}
	for _, dsn := range []string{"file:escaped%20log.db?mode=rwc", "plain.db?_pragma=busy_timeout(5000)"} {
		st, err := openStore(dsn)
		if err != nil {
			t.Fatal(err)
		}
		if st.readDB == nil {
			t.Error("file database has no read connection")
		}
		if !exportTargetCollidesWithDB(sqliteFilePath(dsn), dsn) {
			t.Errorf("missed DSN collision for %q", dsn)
		}
		info, err := os.Stat(sqliteFilePath(dsn))
		if err != nil {
			t.Fatal(err)
		}
		if runtime.GOOS != "windows" && info.Mode().Perm() != 0o600 {
			t.Errorf("database permissions = %o", info.Mode().Perm())
		}
		st.Close()
	}
	if _, err := os.Stat("plain.db?_pragma=busy_timeout(5000)"); !os.IsNotExist(err) {
		t.Fatalf("driver options became part of a filename: %v", err)
	}
}

func TestAuditRejectAmbiguousCLIArguments(t *testing.T) {
	for _, args := range [][]string{
		{"-version"}, {"unexpected"}, {"--export-adif", ""},
		{"--export-adif", "one.adi", "two.adi"},
		{"--export-adif", "one.adi", "--export-adif", "two.adi"},
		{"--import-adif", "one.adi", "--version"},
	} {
		if err := validateArgs(args); err == nil {
			t.Errorf("accepted ambiguous arguments %q", args)
		}
	}
}

func TestAuditLargeParkNamesFlushImportBatch(t *testing.T) {
	st := openTestStore(t)
	var input strings.Builder
	for i := range 20 {
		q := validTestQSO()
		q.time = q.time.Add(time.Duration(i) * time.Minute)
		q.timeOff = q.time
		q.parkName = strings.Repeat("P", 900000)
		input.WriteString(singleQSOADIF(q))
	}
	input.WriteString("<CALL:bad>")
	result, err := importADIF(context.Background(), strings.NewReader(input.String()), 1, st)
	if err == nil {
		t.Fatal("expected malformed trailing record to fail")
	}
	if result.Imported == 0 {
		t.Fatal("large park names bypassed the import batch byte limit")
	}
}

func TestAuditAntipodalDistanceStaysFinite(t *testing.T) {
	for i := -899; i <= 899; i++ {
		lat := float64(i) / 10
		_, distance := GreatCircleBearingDistance(lat, 0, -lat, 180)
		if math.IsNaN(distance) || math.IsInf(distance, 0) || math.Abs(distance-math.Pi*earthRadiusKm) > 0.001 {
			t.Fatalf("antipodal distance at latitude %v = %v", lat, distance)
		}
	}
}

func TestAuditTerminalOutputTreatsExternalTextAsData(t *testing.T) {
	m := reviewModel(t)
	control := "\x1b]52;c;YXVkaXQ=\x07"
	m.statusMsg = "remote error: " + control
	m.clusterStatus = control
	m.solar.SFI = control
	for _, screen := range []screen{qsoEntryScreen, stationSetupScreen, clusterScreen, clusterFiltersScreen, adifImportScreen, qsoDetailsScreen, qsoContestScreen} {
		m.screen = screen
		if strings.Contains(m.View(), "\x1b]52;") {
			t.Errorf("screen %v emitted terminal controls from status text", screen)
		}
	}
	m.fields[fieldCall].SetValue("W1AW")
	m.potaSpottedCall, m.potaSpottedRef, m.potaSpottedPark = "W1AW", "US-0001", control
	if strings.Contains(m.analysisPanel(100), "\x1b]52;") {
		t.Error("POTA park name emitted terminal controls")
	}
	q := reviewQSO(m)
	q.rstSent = control + "599"
	reviewInsert(t, m, q)
	m.refreshTableRows()
	if strings.Contains(m.table.Rows()[0][3], "\x1b") {
		t.Error("recent QSO report retained terminal controls")
	}
	m.showWorkedCall(q.call)
	if strings.Contains(m.table.Rows()[0][3], "\x1b") {
		t.Error("call history report retained terminal controls")
	}
	stored, err := m.store.qsoByID(q.profileID, m.recentQSOs[0].id)
	if err != nil || stored.rstSent != q.rstSent {
		t.Fatalf("display sanitization changed stored data: %v", err)
	}
}
