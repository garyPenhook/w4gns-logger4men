package main

import (
	"bytes"
	"context"
	"errors"
	"io"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func reviewModel(t *testing.T) model {
	t.Helper()
	st, err := openStore(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	m := initialModel(st)
	t.Cleanup(m.bgCancel)
	return m
}
func reviewQSO(m model) qso {
	q := validTestQSO()
	q.profileID = m.activeStation.ID
	q.call = "W1AW"
	q.band = "20M"
	q.frequency = "14.025"
	q.rstSent = "599"
	q.rstRcvd = "599"
	return q
}
func reviewInsert(t *testing.T, m model, q qso) int64 {
	t.Helper()
	id, err := m.store.insertQSO(q)
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func TestReviewEditPreservesLongFields(t *testing.T) {
	m := reviewModel(t)
	q := reviewQSO(m)
	q.comment = strings.Repeat("long notes ", 15)
	q.comment = strings.TrimSpace(q.comment)
	q.comment += "\nsecond line\twith a tab"
	q.email = strings.Repeat("x", 40) + "@example.test"
	q.parkName = strings.Repeat("park", 20)
	q.name = strings.Repeat("name", 15)
	id := reviewInsert(t, m, q)
	m.beginEditQSO(qso{id: id})
	m.fields[fieldRSTRcvd].SetValue("579")
	m, _ = m.logCurrentQSO()
	got, err := m.store.qsoByID(q.profileID, id)
	if err != nil {
		t.Fatal(err)
	}
	if got.comment != q.comment || got.email != q.email || got.parkName != q.parkName || got.name != q.name || got.rstRcvd != "579" {
		t.Fatalf("edit changed unrelated data: %+v", got)
	}
}

func TestReviewOccurrencesMigrationAndSerialResume(t *testing.T) {
	path := filepath.Join(t.TempDir(), "old.db")
	st, err := openStore(path)
	if err != nil {
		t.Fatal(err)
	}
	m := initialModel(st)
	defer m.bgCancel()
	q := reviewQSO(m)
	q.contestID = "CWT-1900"
	q.time = time.Date(2026, 9, 2, 19, 0, 0, 0, time.UTC)
	q.timeOff = q.time
	reviewInsert(t, m, q)
	q.time = q.time.AddDate(0, 0, 7)
	q.timeOff = q.time
	reviewInsert(t, m, q)
	st.Close()
	st, err = openStore(path)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	m = initialModel(st)
	defer m.bgCancel()
	var distinct int
	if err := st.db.QueryRow(`SELECT COUNT(DISTINCT contest_id) FROM qso`).Scan(&distinct); err != nil || distinct != 2 {
		t.Fatalf("occurrences=%d err=%v", distinct, err)
	}
	dupe, err := st.isDupe("W1AW", "20M", "CWT-1900@20260916", "CWT", "call+band+session", q.profileID, 0, q.time, time.Time{})
	if err != nil || dupe {
		t.Fatalf("previous week dupe=%v err=%v", dupe, err)
	}
	e := m.events[eventIndex(t, m.events, "CW-OPEN")]
	m.selectEvent(e, e.Sessions[0])
	q.contestID = m.contestFields[contestName].Value()
	q.stx = "041"
	reviewInsert(t, m, q)
	m.selectEvent(e, e.Sessions[0])
	if m.contestFields[contestSerialSent].Value() != "042" {
		t.Fatal("serial reset")
	}
	restarted := initialModel(st)
	defer restarted.bgCancel()
	if restarted.contestFields[contestSerialSent].Value() != "042" || restarted.contestFields[contestName].Value() != q.contestID {
		t.Fatal("restart lost contest or serial")
	}
	q.stx = "099"
	q.call = "K2ABC"
	reviewInsert(t, m, q)
	updated, _ := restarted.Update(adifImportedMsg{result: adifImportResult{Imported: 1}})
	restarted = updated.(model)
	if restarted.contestFields[contestSerialSent].Value() != "100" {
		t.Fatal("import did not advance stored serial")
	}
}

func TestReviewDistanceAndZonePersistedScores(t *testing.T) {
	m := reviewModel(t)
	q := reviewQSO(m)
	q.contestID = "STEW-PERRY-test@2026"
	q.band = "160M"
	q.frequency = "1.810"
	q.myGridSquare = "EM75"
	q.srxString = "FN31"
	reviewInsert(t, m, q)
	live := newContestState()
	live.record(q)
	rebuilt, err := buildContestState(context.Background(), q.profileID, "W4GNS", q.contestID, m.store)
	if err != nil {
		t.Fatal(err)
	}
	e := m.events[eventIndex(t, m.events, "STEW-PERRY")]
	if live.score(e.Scoring).total() == 0 || live.score(e.Scoring) != rebuilt.score(e.Scoring) {
		t.Fatal("distance lost on rebuild")
	}
	q.contestID = "CQ-WW-CW-test@2026"
	q.srxString = "04"
	reviewInsert(t, m, q)
	rebuilt, err = buildContestState(context.Background(), q.profileID, "W4GNS", q.contestID, m.store)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := rebuilt.cqZoneAll[4]; !ok {
		t.Fatalf("copied zone ignored: %v", rebuilt.cqZoneAll)
	}
	if _, ok := rebuilt.cqZoneAll[5]; ok {
		t.Fatal("guessed zone retained")
	}
}

func TestReviewSweepstakesExportAndDuplicates(t *testing.T) {
	m := reviewModel(t)
	e := m.events[eventIndex(t, m.events, "ARRL-SS-CW")]
	q := reviewQSO(m)
	q.contestID = "ARRL-SS-CW-test@2026"
	q.stx = "001"
	q.stxString = "B W4GNS 76 TN"
	q.srx = "002"
	q.srxString = "A W1AW 38 CT"
	q.stationCallsign = "W4GNS"
	reviewInsert(t, m, q)
	q.band = "40M"
	q.frequency = "7.025"
	q.srxString = "A W1AW 38 EMA"
	q.time = q.time.Add(time.Minute)
	q.timeOff = q.time
	reviewInsert(t, m, q)
	state, err := buildContestState(context.Background(), q.profileID, "W4GNS", q.contestID, m.store)
	if err != nil {
		t.Fatal(err)
	}
	if score := state.score(e.Scoring); score.qsoPoints != 2 || score.multipliers != 1 {
		t.Fatalf("duplicates scored: %+v", score)
	}
	line, err := cabrilloQSOLine(q, testStationProfile(), e)
	if err != nil {
		t.Fatal(err)
	}
	fields := strings.Fields(line)
	if len(fields) != 15 || fields[10] != "W1AW" || fields[9] != "TN" || fields[14] != "EMA" {
		t.Fatalf("bad Sweepstakes layout: %q", line)
	}
	q.srxString = strings.Repeat("X", 30)
	if _, err := cabrilloQSOLine(q, testStationProfile(), testEventDefinition()); err == nil {
		t.Fatal("oversize exchange silently clipped")
	}
}

func TestReviewDefaultRSTAndImportRefresh(t *testing.T) {
	m := reviewModel(t)
	e := m.events[eventIndex(t, m.events, "CQ-WW-CW")]
	m.selectEvent(e, e.Sessions[0])
	for _, call := range []string{"W1AW", "K1ABC"} {
		m.fields[fieldCall].SetValue(call)
		m, _ = m.logCurrentQSO()
	}
	var missing int
	if err := m.store.db.QueryRow(`SELECT COUNT(*) FROM qso WHERE COALESCE(rst_rcvd,'')=''`).Scan(&missing); err != nil || missing != 0 {
		t.Fatalf("missing RST=%d err=%v", missing, err)
	}
	q := reviewQSO(m)
	q.call = "K2ABC"
	q.contestID = m.contestFields[contestName].Value()
	reviewInsert(t, m, q)
	updated, _ := m.Update(adifImportedMsg{result: adifImportResult{Imported: 1}, err: errors.New("trailing bad record")})
	m = updated.(model)
	if len(m.contestIndex.uniqueCalls) != 3 || m.qsoCount != 3 || !strings.Contains(m.statusMsg, "1 committed") {
		t.Fatalf("partial import not reflected: %s", m.statusMsg)
	}
}

func TestReviewPendingUploadsSurviveConfigurationAndReadErrors(t *testing.T) {
	m := reviewModel(t)
	q := reviewQSO(m)
	id, err := m.store.insertQSOWithUploads(q, []string{uploadDestQRZ}, time.Now().Add(-time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if cmds := m.drainOutbox(); len(cmds) != 0 {
		t.Fatal("unconfigured upload ran")
	}
	var count int
	if err := m.store.db.QueryRow(`SELECT COUNT(*) FROM upload_outbox`).Scan(&count); err != nil || count != 1 {
		t.Fatal("pending delivery lost")
	}
	m.qrzAPIKey = "test-key"
	if _, err := m.store.db.Exec(`UPDATE upload_outbox SET next_attempt_at=?`, time.Now().Add(-time.Minute).UTC().Format(time.RFC3339)); err != nil {
		t.Fatal(err)
	}
	if _, err := m.store.db.Exec(`UPDATE qso SET mode=NULL WHERE id=?`, id); err != nil {
		t.Fatal(err)
	}
	if cmds := m.drainOutbox(); len(cmds) != 0 {
		t.Fatal("unreadable QSO uploaded")
	}
	var attempts int
	if err := m.store.db.QueryRow(`SELECT attempts FROM upload_outbox`).Scan(&attempts); err != nil || attempts != 1 {
		t.Fatalf("read error lost delivery: attempts=%d err=%v", attempts, err)
	}
}

func TestReviewADIFMetadataAndGridRoundTrip(t *testing.T) {
	m := reviewModel(t)
	q := reviewQSO(m)
	q.contestID = "CW-OPEN-1@2026"
	q.unscored = true
	q.grid = "FN01MH42BQ"
	q.myGridSquare = "EM75AB12CD"
	q.parkName = "A long park name"
	q.stxString = "José"
	reviewInsert(t, m, q)
	var output bytes.Buffer
	if _, err := exportADIF(context.Background(), &output, q.profileID, m.store); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(output.String(), "José") || !strings.Contains(output.String(), "<GRIDSQUARE_EXT:2>BQ") {
		t.Fatalf("bad ADIF encoding: %s", output.String())
	}
	other := reviewModel(t)
	result, err := importADIF(context.Background(), &output, other.activeStation.ID, other.store)
	if err != nil || result.Imported != 1 {
		t.Fatalf("import: %+v %v", result, err)
	}
	got, err := other.store.qsoByID(other.activeStation.ID, 1)
	if err != nil {
		t.Fatal(err)
	}
	if got.contestID != q.contestID || !got.unscored || got.parkName != q.parkName || got.grid != q.grid || got.myGridSquare != q.myGridSquare {
		t.Fatalf("round trip lost metadata: %+v", got)
	}
}

func TestReviewADIFLongHeaderTimeOffAndCancellation(t *testing.T) {
	m := reviewModel(t)
	record := "<CALL:4>W1AW<MODE:2>CW<BAND:3>20M<QSO_DATE:8>20260905<TIME_ON:4>2355<TIME_OFF:4>0100<EOR>"
	result, err := importADIF(context.Background(), strings.NewReader(strings.Repeat("header ", 1000)+"<EOH>"+record), m.activeStation.ID, m.store)
	if err != nil || result.Imported != 1 {
		t.Fatalf("long header: %+v %v", result, err)
	}
	q, err := m.store.qsoByID(m.activeStation.ID, 1)
	if err != nil {
		t.Fatal(err)
	}
	if q.timeOff.Sub(q.time) != 65*time.Minute {
		t.Fatalf("time off lost: %v", q.timeOff)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := importADIF(ctx, strings.NewReader(strings.Repeat("text ", 100000)), m.activeStation.ID, m.store); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancel ignored: %v", err)
	}
	if _, err := openADIFInput(t.TempDir()); err == nil {
		t.Fatal("directory accepted as import")
	}
}

func TestReviewExportPathErrorsAndReservations(t *testing.T) {
	dir := t.TempDir()
	path, err := reserveExportPath(filepath.Join(dir, "log.adi"))
	if err != nil {
		t.Fatal(err)
	}
	second, err := reserveExportPath(filepath.Join(dir, "log.adi"))
	if err != nil || path == second {
		t.Fatal("reservation collided")
	}
	if _, err := reserveExportPath(filepath.Join(path, "child.adi")); err == nil {
		t.Fatal("non-directory path accepted")
	}
}

type reviewHookWriter struct {
	io.Writer
	hook func()
}

func (w *reviewHookWriter) Write(p []byte) (int, error) {
	if w.hook != nil {
		hook := w.hook
		w.hook = nil
		hook()
	}
	return w.Writer.Write(p)
}

func TestReviewExportSnapshotAllowsConcurrentLogging(t *testing.T) {
	m := reviewModel(t)
	q := reviewQSO(m)
	q.contestID = "CW-OPEN-test@2026"
	q.stx = "001"
	q.srx = "001"
	q.stxString = "GARY"
	q.srxString = "BOB"
	reviewInsert(t, m, q)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	var buf bytes.Buffer
	writer := &reviewHookWriter{Writer: &buf, hook: func() {
		q.call = "K1ABC"
		done := make(chan error, 1)
		go func() { _, err := m.store.insertQSO(q); done <- err }()
		select {
		case err := <-done:
			if err != nil {
				t.Error(err)
			}
		case <-ctx.Done():
			t.Error("export blocked logging")
		}
	}}
	e := m.events[eventIndex(t, m.events, "CW-OPEN")]
	count, score, err := exportCabrillo(ctx, writer, testStationProfile(), e, q.contestID, m.store)
	if err != nil || count != 1 || score.total() != 1 || strings.Contains(buf.String(), "K1ABC") {
		t.Fatalf("inconsistent snapshot: count=%d score=%+v err=%v", count, score, err)
	}
}

func TestReviewLookupCorrelation(t *testing.T) {
	t.Chdir(t.TempDir())
	m := reviewModel(t)
	m.fields[fieldCall].SetValue("W1AW")
	m.qrzLookups = map[uint64]qrzLookupPending{1: {call: "W1AW"}}
	m.qrzActiveLookup = 1
	m.openStationSetup()
	m.stationFields[stationQRZXMLUserField].SetValue("new-user")
	m.stationFields[stationQRZXMLPassField].SetValue("new-password")
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	m.saveStationSetup()
	updated, _ := m.Update(qrzCallsignLookupMsg{requestID: 1, call: "W1AW", sessionKey: "old-key"})
	m = updated.(model)
	if m.qrzXMLSessionKey != "" {
		t.Fatal("old credential session restored")
	}
	q := reviewQSO(m)
	id := reviewInsert(t, m, q)
	m.potaLookups = map[uint64]qrzLookupPending{1: {call: q.call, qsoID: id}}
	updated, _ = m.Update(potaLookupMsg{requestID: 1, call: q.call, reference: "US-1234", parkName: "Park"})
	m = updated.(model)
	got, err := m.store.qsoByID(q.profileID, id)
	if err != nil || got.potaRef != "US-1234" || got.parkName != "Park" {
		t.Fatal("late POTA result lost")
	}
	m.detailFields[detailPOTARef].SetValue("US-9999")
	updated, _ = m.Update(potaLookupMsg{call: q.call, reference: "US-1234", parkName: "Wrong"})
	m = updated.(model)
	if m.detailFields[detailParkName].Value() != "" {
		t.Fatal("mismatched park name filled")
	}
}

func TestReviewUploadRecoveryAndBinding(t *testing.T) {
	m := reviewModel(t)
	q := reviewQSO(m)
	m.wrlAPIKey = "old-key"
	m.wrlLogbookID = "old-book"
	_, err := m.store.insertQSOWithUploads(q, []string{uploadDestWRL}, time.Now().Add(-time.Minute), m.uploadBindings())
	if err != nil {
		t.Fatal(err)
	}
	m.wrlLogbookID = "new-book"
	if len(m.drainOutbox()) != 0 {
		t.Fatal("pending contact rerouted")
	}
	if _, err := m.store.db.Exec(`UPDATE upload_outbox SET attempts=20`); err != nil {
		t.Fatal(err)
	}
	t.Setenv("W4GNS_WRL_KEY", "old-key")
	t.Setenv("W4GNS_WRL_LOGBOOK_ID", "new-book")
	t.Setenv("W4GNS_QRZ_KEY", "unused-test-key")
	m.retryFailedUploads()
	var attempts int
	var binding string
	if err := m.store.db.QueryRow(`SELECT attempts,binding FROM upload_outbox`).Scan(&attempts, &binding); err != nil || attempts != 0 || binding != uploadBinding("old-key", "new-book") {
		t.Fatalf("retry did not reset/rebind: %d %v", attempts, err)
	}
	if _, err := m.store.db.Exec(`UPDATE upload_outbox SET last_error='previous failure'`); err != nil {
		t.Fatal(err)
	}
	entries, err := m.store.claimDueUploads(time.Now().Add(time.Second), time.Hour, 10)
	if err != nil || len(entries) != 1 {
		t.Fatalf("claim: %v %v", entries, err)
	}
	m.retryFailedUploads()
	entries, err = m.store.claimDueUploads(time.Now().Add(time.Second), time.Hour, 10)
	if err != nil || len(entries) != 0 {
		t.Fatalf("manual retry broke in-flight lease: %v %v", entries, err)
	}
}

func TestReviewNativeSmoke(t *testing.T) {
	m := reviewModel(t)
	q := reviewQSO(m)
	reviewInsert(t, m, q)
	dir := t.TempDir()
	path := filepath.Join(dir, "log.adi")
	for i := 0; i < 2; i++ {
		if _, err := writeADIFAtomic(context.Background(), dir, path, q.profileID, m.store); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := time.LoadLocation("America/New_York"); err != nil {
		t.Fatal(err)
	}
	if err := validateCallsignChars("W１AW"); err == nil {
		t.Fatal("non-ASCII call accepted")
	}
	if e, ok := m.eventForContestID(); ok {
		t.Fatalf("empty ID matched %s", e.ID)
	}
	m.contestFields[contestName].SetValue(" CWT ")
	if e, ok := m.eventForContestID(); !ok || e.ID != "CWT" {
		t.Fatal("bare event not resolved")
	}
	for _, bad := range []string{"0", "-1", "999"} {
		if iaruExchangeSpecial(bad) != "" || iaruExchangeZone(bad) != 0 {
			t.Fatalf("bad IARU exchange %q accepted", bad)
		}
	}
}
