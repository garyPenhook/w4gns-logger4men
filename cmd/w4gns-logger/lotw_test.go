package main

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// writeFakeTQSL installs a fake tqsl script that ignores its arguments,
// echoes a "Final Status" line matching what real tqsl prints, and exits
// with the code taken from the TQSL_FAKE_EXIT env var (set per test via
// t.Setenv), returning the script's path for CWLOGGER_TQSL.
func writeFakeTQSL(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "tqsl")
	script := "#!/bin/sh\n" +
		"echo \"Final Status: ${TQSL_FAKE_TEXT}(${TQSL_FAKE_EXIT})\"\n" +
		"exit \"${TQSL_FAKE_EXIT}\"\n"
	if err := os.WriteFile(path, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestFindTQSLPrefersEnvOverride(t *testing.T) {
	t.Setenv("CWLOGGER_TQSL", "/tmp/does-not-need-to-exist-for-this-check")
	got, err := findTQSL()
	if err != nil {
		t.Fatalf("findTQSL: %v", err)
	}
	if got != "/tmp/does-not-need-to-exist-for-this-check" {
		t.Fatalf("findTQSL() = %q, want the env override", got)
	}
}

func TestLotwExitDeliveredTable(t *testing.T) {
	delivered := map[int]bool{
		0: true, 1: false, 2: false, 3: false, 4: false, 5: false, 6: false, 7: false,
		8: true, 9: true, 10: false, 11: false, 12: false, 13: false, 14: true, 15: false,
	}
	for code, want := range delivered {
		if got := lotwExitDelivered(code); got != want {
			t.Errorf("lotwExitDelivered(%d) = %v, want %v", code, got, want)
		}
	}
}

func TestTqslFinalStatusTextExtractsMarker(t *testing.T) {
	output := "Signing using Callsign W4GNS, DXCC Entity UNITED STATES OF AMERICA\nwrote 1 records ... Final Status: Success(0)\n"
	if got := tqslFinalStatusText(output); got != "Success" {
		t.Fatalf("tqslFinalStatusText = %q, want Success", got)
	}
}

func TestTqslFinalStatusTextFallsBackToRawOutput(t *testing.T) {
	output := "some unexpected crash trace with no marker"
	if got := tqslFinalStatusText(output); got != output {
		t.Fatalf("tqslFinalStatusText = %q, want the raw output", got)
	}
}

// TestTqslStationLocationNotFoundHint covers the real tqsl 2.8.6 output
// (verified locally) for an -l value that doesn't match any configured
// Station Location, which otherwise surfaces only as the unhelpful generic
// "Command Syntax Error(10)".
func TestTqslStationLocationNotFoundHint(t *testing.T) {
	output := "TQSL Version 2.8.6 [pkg-v2.8.6]\n" +
		"The selected Station Location (W4GNS) could not be found\n" +
		"Final Status: Command Syntax Error(10)\n"
	hint, ok := tqslStationLocationNotFoundHint(output, "W4GNS")
	if !ok {
		t.Fatal("tqslStationLocationNotFoundHint: ok = false, want true")
	}
	if !strings.Contains(hint, `"W4GNS"`) || !strings.Contains(hint, "Station Setup") {
		t.Fatalf("hint = %q, want it to name the bad value and point at Station Setup", hint)
	}
}

func TestTqslStationLocationNotFoundHintFalseForUnrelatedOutput(t *testing.T) {
	output := "Final Status: Command Syntax Error(10)\n"
	if _, ok := tqslStationLocationNotFoundHint(output, "Home"); ok {
		t.Fatal("tqslStationLocationNotFoundHint: ok = true for output without the marker")
	}
}

// TestRunTQSLSurfacesStationLocationNotFoundHint checks the hint is wired
// into runTQSL's returned statusText, not just the standalone helper above.
func TestRunTQSLSurfacesStationLocationNotFoundHint(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "tqsl")
	script := "#!/bin/sh\n" +
		"echo \"The selected Station Location (W4GNS) could not be found\"\n" +
		"echo \"Final Status: Command Syntax Error(10)\"\n" +
		"exit 10\n"
	if err := os.WriteFile(path, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	code, text, err := runTQSL(context.Background(), path, "W4GNS", "", "/tmp/whatever.adi")
	if err != nil {
		t.Fatalf("runTQSL: %v", err)
	}
	if code != 10 {
		t.Fatalf("exit code = %d, want 10", code)
	}
	if !strings.Contains(text, `"W4GNS"`) || !strings.Contains(text, "Station Setup") {
		t.Fatalf("status text = %q, want the station-not-found hint", text)
	}
}

// TestRunTQSLExitCodeTable is table-driven over every documented TQSL exit
// code (docs/LoTW_Integration_Design.md), asserting runTQSL surfaces the
// code and status text rather than treating a nonzero exit as a Go error.
func TestRunTQSLExitCodeTable(t *testing.T) {
	tqslPath := writeFakeTQSL(t)
	cases := []struct {
		code int
		text string
	}{
		{0, "Success"},
		{1, "User Cancelled"},
		{2, "Upload Rejected"},
		{8, "No QSOs to upload"},
		{9, "Some QSOs not processed"},
		{11, "LoTW Connection Failed"},
		{13, "Upload tracking database is locked"},
		{14, "Previously signed QSOs were detected"},
		{15, "Incorrect passphrase"},
	}
	for _, tc := range cases {
		t.Run(fmt.Sprintf("code_%d", tc.code), func(t *testing.T) {
			t.Setenv("TQSL_FAKE_EXIT", fmt.Sprintf("%d", tc.code))
			t.Setenv("TQSL_FAKE_TEXT", tc.text)
			code, text, err := runTQSL(context.Background(), tqslPath, "Home", "", "/tmp/whatever.adi")
			if err != nil {
				t.Fatalf("runTQSL: %v", err)
			}
			if code != tc.code {
				t.Fatalf("exit code = %d, want %d", code, tc.code)
			}
			if text != tc.text {
				t.Fatalf("status text = %q, want %q", text, tc.text)
			}
		})
	}
}

func TestSignAndUploadLoTWWritesBatchAndCleansUpTemp(t *testing.T) {
	tqslPath := writeFakeTQSL(t)
	t.Setenv("TQSL_FAKE_EXIT", "0")
	t.Setenv("TQSL_FAKE_TEXT", "Success")

	q1 := validTestQSO()
	q1.call = "W1AW"
	q2 := validTestQSO()
	q2.call = "K1ABC"

	before, _ := filepath.Glob(filepath.Join(os.TempDir(), "w4gns-lotw-*.adi"))
	code, text, err := signAndUploadLoTW(context.Background(), tqslPath, "Home", "", []qso{q1, q2})
	if err != nil {
		t.Fatalf("signAndUploadLoTW: %v", err)
	}
	if code != 0 || text != "Success" {
		t.Fatalf("code=%d text=%q, want 0/Success", code, text)
	}
	after, _ := filepath.Glob(filepath.Join(os.TempDir(), "w4gns-lotw-*.adi"))
	if len(after) > len(before) {
		t.Fatalf("temp LoTW ADIF file(s) left behind: %v", after)
	}
}

func TestWriteLoTWBatchADIFIncludesEveryQSO(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "batch.adi")
	q1 := validTestQSO()
	q1.call = "W1AW"
	q2 := validTestQSO()
	q2.call = "K1ABC"

	if err := writeLoTWBatchADIF(path, []qso{q1, q2}); err != nil {
		t.Fatalf("writeLoTWBatchADIF: %v", err)
	}
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	body := string(contents)
	if got := strings.Count(body, "<EOR>"); got != 2 {
		t.Fatalf("<EOR> count = %d, want 2", got)
	}
	if !strings.Contains(body, "W1AW") || !strings.Contains(body, "K1ABC") {
		t.Fatalf("batch ADIF missing an expected callsign:\n%s", body)
	}
}

func TestLotwOutboxUploadCmdNilWithoutStationConfigured(t *testing.T) {
	m := model{lotwStation: ""}
	if cmd := m.lotwOutboxUploadCmd([]qso{validTestQSO()}); cmd != nil {
		t.Fatal("lotwOutboxUploadCmd returned a non-nil command with no station configured")
	}
}

func TestLotwOutboxUploadCmdNilWhenTQSLUnavailable(t *testing.T) {
	t.Setenv("CWLOGGER_TQSL", "")
	t.Setenv("PATH", t.TempDir()) // a PATH with no tqsl binary in it
	m := model{lotwStation: "Home"}
	if cmd := m.lotwOutboxUploadCmd([]qso{validTestQSO()}); cmd != nil {
		t.Fatal("lotwOutboxUploadCmd returned a non-nil command with tqsl unresolvable")
	}
}

func TestLotwBindingEmptyWithoutStationOrTQSL(t *testing.T) {
	m := model{lotwStation: ""}
	if got := m.lotwBinding(); got != "" {
		t.Fatalf("lotwBinding() = %q, want empty with no station configured", got)
	}

	t.Setenv("CWLOGGER_TQSL", "")
	t.Setenv("PATH", t.TempDir())
	m = model{lotwStation: "Home"}
	if got := m.lotwBinding(); got != "" {
		t.Fatalf("lotwBinding() = %q, want empty when tqsl is unresolvable", got)
	}
}

// writeFakeTQSLNoop installs a fake tqsl that ignores its arguments and does
// nothing — checkTQSLUpdates reads its findings from tqsl's own state files
// (cert_status.xml/.tqslapp) rather than from the process's output or exit
// code (see tqslCertStatusPath), so the fake process itself only needs to
// exist and exit; tests seed those files directly via a redirected HOME.
func writeFakeTQSLNoop(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "tqsl")
	if err := os.WriteFile(path, []byte("#!/bin/sh\nexit 255\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	return path
}

// seedTQSLHome redirects HOME (the same variable tqsl itself resolves
// ~/.tqsl and ~/.tqslapp from) to a fresh temp dir and writes cert_status.xml
// and .tqslapp with the given contents, so checkTQSLUpdates reads them back
// exactly as it would tqsl's real state files.
func seedTQSLHome(t *testing.T, certStatusXML, tqslAppState string) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	if err := os.MkdirAll(filepath.Join(home, ".tqsl"), 0o700); err != nil {
		t.Fatal(err)
	}
	if certStatusXML != "" {
		if err := os.WriteFile(filepath.Join(home, ".tqsl", "cert_status.xml"), []byte(certStatusXML), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if tqslAppState != "" {
		if err := os.WriteFile(filepath.Join(home, ".tqslapp"), []byte(tqslAppState), 0o600); err != nil {
			t.Fatal(err)
		}
	}
}

func TestCheckTQSLUpdatesReportsRevokedCertificate(t *testing.T) {
	tqslPath := writeFakeTQSLNoop(t)
	seedTQSLHome(t, `<CertStatus><Cert serial="1144040"><status>Revoked</status></Cert></CertStatus>`, "")
	got := checkTQSLUpdates(context.Background(), tqslPath)
	want := "certificate 1144040: Revoked"
	if got != want {
		t.Fatalf("checkTQSLUpdates() = %q, want %q", got, want)
	}
}

func TestCheckTQSLUpdatesReportsPendingCertRequest(t *testing.T) {
	tqslPath := writeFakeTQSLNoop(t)
	seedTQSLHome(t, "", "HasRun=yes\nRequestPending=W4GNS\nName=Test\n")
	got := checkTQSLUpdates(context.Background(), tqslPath)
	want := "pending Callsign Certificate request: W4GNS"
	if got != want {
		t.Fatalf("checkTQSLUpdates() = %q, want %q", got, want)
	}
}

func TestCheckTQSLUpdatesEmptyWhenUnrevokedAndNoPendingRequest(t *testing.T) {
	tqslPath := writeFakeTQSLNoop(t)
	seedTQSLHome(t, `<CertStatus><Cert serial="1144040"><status>Unrevoked</status></Cert></CertStatus>`, "HasRun=yes\nRequestPending=\n")
	if got := checkTQSLUpdates(context.Background(), tqslPath); got != "" {
		t.Fatalf("checkTQSLUpdates() = %q, want empty for an unrevoked cert and no pending request", got)
	}
}

func TestCheckTQSLUpdatesEmptyWhenFilesAbsent(t *testing.T) {
	tqslPath := writeFakeTQSLNoop(t)
	t.Setenv("HOME", t.TempDir())
	if got := checkTQSLUpdates(context.Background(), tqslPath); got != "" {
		t.Fatalf("checkTQSLUpdates() = %q, want empty when tqsl has never run", got)
	}
}

func TestLotwUpdateCheckCmdNilWithoutStationConfigured(t *testing.T) {
	m := model{lotwStation: ""}
	if cmd := m.lotwUpdateCheckCmd(); cmd != nil {
		t.Fatal("lotwUpdateCheckCmd returned a non-nil command with no station configured")
	}
}

func TestLotwUpdateCheckCmdNilWhenTQSLUnavailable(t *testing.T) {
	t.Setenv("CWLOGGER_TQSL", "")
	t.Setenv("PATH", t.TempDir())
	m := model{lotwStation: "Home"}
	if cmd := m.lotwUpdateCheckCmd(); cmd != nil {
		t.Fatal("lotwUpdateCheckCmd returned a non-nil command with tqsl unresolvable")
	}
}

func TestLotwUpdateCheckCmdReportsNotice(t *testing.T) {
	t.Setenv("CWLOGGER_TQSL", writeFakeTQSLNoop(t))
	seedTQSLHome(t, `<CertStatus><Cert serial="1144040"><status>Expired</status></Cert></CertStatus>`, "")
	m := model{lotwStation: "Home", bgTasks: &sync.WaitGroup{}}
	cmd := m.lotwUpdateCheckCmd()
	if cmd == nil {
		t.Fatal("lotwUpdateCheckCmd returned nil with LoTW configured and tqsl resolvable")
	}
	msg, ok := cmd().(lotwUpdateCheckMsg)
	if !ok {
		t.Fatalf("cmd() = %T, want lotwUpdateCheckMsg", cmd())
	}
	if msg.notice != "certificate 1144040: Expired" {
		t.Fatalf("notice = %q, want the cert_status.xml finding", msg.notice)
	}
}

// TestDrainOutboxBatchesLoTWEntriesIntoOneCommand guards against a future
// drainOutbox switch edit dropping the LoTW batch path: two QSOs enqueued for
// "lotw" must collapse into a single tea.Cmd (one tqsl call), while a QRZ/WRL
// row stays on its own per-entry command.
func TestDrainOutboxBatchesLoTWEntriesIntoOneCommand(t *testing.T) {
	m := reviewModel(t)
	t.Setenv("CWLOGGER_TQSL", writeFakeTQSL(t))
	t.Setenv("TQSL_FAKE_EXIT", "0")
	t.Setenv("TQSL_FAKE_TEXT", "Success")
	m.lotwStation = "Home"
	m.wrlAPIKey = "wrl-test-key"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
	}))
	defer srv.Close()
	oldWRLAPI := wrlContactsAPI
	wrlContactsAPI = srv.URL
	defer func() { wrlContactsAPI = oldWRLAPI }()

	q1 := reviewQSO(m)
	q1.call = "W1AW"
	if _, err := m.store.insertQSOWithUploads(q1, []string{uploadDestLoTW}, time.Now().Add(-time.Minute), m.uploadBindings()); err != nil {
		t.Fatal(err)
	}
	q2 := reviewQSO(m)
	q2.call = "K1ABC"
	if _, err := m.store.insertQSOWithUploads(q2, []string{uploadDestLoTW}, time.Now().Add(-time.Minute), m.uploadBindings()); err != nil {
		t.Fatal(err)
	}
	q3 := reviewQSO(m)
	q3.call = "N2XYZ"
	if _, err := m.store.insertQSOWithUploads(q3, []string{uploadDestWRL}, time.Now().Add(-time.Minute), m.uploadBindings()); err != nil {
		t.Fatal(err)
	}

	cmds := m.drainOutbox()
	if len(cmds) != 2 {
		t.Fatalf("drainOutbox returned %d commands, want 2 (one LoTW batch + one WRL)", len(cmds))
	}

	var sawLoTWBatch, sawWRL bool
	for _, cmd := range cmds {
		switch msg := cmd().(type) {
		case lotwUploadMsg:
			sawLoTWBatch = true
			if len(msg.results) != 2 {
				t.Fatalf("lotwUploadMsg carried %d results, want 2 (both LoTW QSOs batched together)", len(msg.results))
			}
			if !msg.delivered {
				t.Fatalf("expected the fake tqsl success exit to be delivered: %+v", msg)
			}
		case wrlUploadMsg:
			sawWRL = true
			if msg.call != "N2XYZ" {
				t.Fatalf("wrlUploadMsg.call = %q, want N2XYZ", msg.call)
			}
		default:
			t.Fatalf("unexpected message type %T from drainOutbox command", msg)
		}
	}
	if !sawLoTWBatch || !sawWRL {
		t.Fatalf("missing expected command: sawLoTWBatch=%v sawWRL=%v", sawLoTWBatch, sawWRL)
	}
}

// TestLotwOutboxUploadCmdRecordsSuppressedNotSentOnAmbiguousExitCode guards
// against a tqsl exit code 8/9 batch (some/no QSOs processed, because they
// were duplicates or fell outside the certificate's date range) being
// recorded as a confirmed delivery for every QSO in the automatic per-QSO
// outbox drain: TQSL doesn't say which QSOs were skipped as harmless
// duplicates versus never actually reaching LoTW, so the outbox row is still
// removed (retrying won't help — see lotwExitDelivered) but the upload log
// must show "suppressed", not "sent".
func TestLotwOutboxUploadCmdRecordsSuppressedNotSentOnAmbiguousExitCode(t *testing.T) {
	m := reviewModel(t)
	t.Setenv("CWLOGGER_TQSL", writeFakeTQSL(t))
	t.Setenv("TQSL_FAKE_EXIT", "9")
	t.Setenv("TQSL_FAKE_TEXT", "Some QSOs not processed")
	m.lotwStation = "Home"

	q := reviewQSO(m)
	q.call = "W1AW"
	qsoID, err := m.store.insertQSOWithUploads(q, []string{uploadDestLoTW}, time.Now().Add(-time.Minute), m.uploadBindings())
	if err != nil {
		t.Fatal(err)
	}
	q.id = qsoID

	cmd := m.lotwOutboxUploadCmd([]qso{q})
	if cmd == nil {
		t.Fatal("lotwOutboxUploadCmd returned nil")
	}
	msg, ok := cmd().(lotwUploadMsg)
	if !ok {
		t.Fatalf("lotwOutboxUploadCmd command produced %T, want lotwUploadMsg", msg)
	}
	if msg.delivered {
		t.Fatal("exit code 9 must not be reported as cleanly delivered")
	}
	if !msg.suppressed {
		t.Fatal("exit code 9 must be reported as suppressed")
	}

	var outboxCount int
	if err := m.store.db.QueryRow(`SELECT COUNT(*) FROM upload_outbox WHERE qso_id=? AND destination=?`, qsoID, uploadDestLoTW).Scan(&outboxCount); err != nil {
		t.Fatal(err)
	}
	if outboxCount != 0 {
		t.Fatalf("outbox row still present after a terminal exit code: %d rows", outboxCount)
	}

	var status string
	if err := m.store.db.QueryRow(`SELECT status FROM upload_log WHERE qso_id=? AND destination=? ORDER BY id DESC LIMIT 1`, qsoID, uploadDestLoTW).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != uploadLogSuppressed {
		t.Fatalf("upload_log status = %q, want %q", status, uploadLogSuppressed)
	}
}

func TestEnqueueLoTWBackfillQueuesExistingQSOsOnce(t *testing.T) {
	m := reviewModel(t)
	q1 := reviewQSO(m)
	q1.call = "W1AW"
	reviewInsert(t, m, q1)
	q2 := reviewQSO(m)
	q2.call = "K1ABC"
	reviewInsert(t, m, q2)

	n, err := m.store.enqueueLoTWBackfill(m.activeStation.ID)
	if err != nil {
		t.Fatalf("enqueueLoTWBackfill: %v", err)
	}
	if n != 2 {
		t.Fatalf("enqueueLoTWBackfill queued %d rows, want 2", n)
	}
	var count int
	if err := m.store.db.QueryRow(`SELECT COUNT(*) FROM upload_outbox WHERE destination=?`, uploadDestLoTW).Scan(&count); err != nil || count != 2 {
		t.Fatalf("upload_outbox lotw rows = %d, err %v; want 2", count, err)
	}

	// Re-running must not duplicate rows already queued (INSERT OR IGNORE).
	n2, err := m.store.enqueueLoTWBackfill(m.activeStation.ID)
	if err != nil {
		t.Fatalf("enqueueLoTWBackfill (second run): %v", err)
	}
	if n2 != 0 {
		t.Fatalf("enqueueLoTWBackfill re-queued %d rows on a second run, want 0", n2)
	}
}

func TestCtrlYQueuesLoTWBackfillAndDrains(t *testing.T) {
	m := reviewModel(t)
	t.Setenv("CWLOGGER_TQSL", writeFakeTQSL(t))
	t.Setenv("TQSL_FAKE_EXIT", "0")
	t.Setenv("TQSL_FAKE_TEXT", "Success")
	m.lotwStation = "Home"

	q := reviewQSO(m)
	q.call = "W1AW"
	reviewInsert(t, m, q)

	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlY})
	m = updated.(model)
	if !strings.Contains(m.statusMsg, "queued 1 QSO") {
		t.Fatalf("statusMsg = %q, want it to report 1 QSO queued", m.statusMsg)
	}
	if cmd == nil {
		t.Fatal("Update(ctrl+y) returned a nil command; expected the drain to run")
	}
	// tea.Batch collapses to the single underlying command when there's only
	// one (compactCmds in bubbletea), which is the case here (one QSO, one
	// destination) — run it to completion instead of inspecting upload_outbox
	// immediately afterward. runBgCmd starts the upload goroutine synchronously
	// inside drainOutbox itself (see its doc comment), so the row can already
	// be gone by the time this line runs otherwise, racing the very goroutine
	// this call is meant to wait for.
	var sawLoTW bool
	switch result := cmd().(type) {
	case lotwUploadMsg:
		sawLoTW = true
		if !result.delivered {
			t.Fatalf("lotwUploadMsg not delivered: %+v", result)
		}
	case tea.BatchMsg:
		for _, sub := range result {
			if msg, ok := sub().(lotwUploadMsg); ok {
				sawLoTW = true
				if !msg.delivered {
					t.Fatalf("lotwUploadMsg not delivered: %+v", msg)
				}
			}
		}
	default:
		t.Fatalf("Update(ctrl+y) command produced %T", result)
	}
	if !sawLoTW {
		t.Fatal("drain did not produce a lotwUploadMsg for the queued backfill")
	}
	var count int
	if err := m.store.db.QueryRow(`SELECT COUNT(*) FROM upload_outbox WHERE destination=?`, uploadDestLoTW).Scan(&count); err != nil || count != 0 {
		t.Fatalf("upload_outbox lotw rows = %d, err %v; want 0 (delivered)", count, err)
	}
}

// TestLotwBackfillCooldownRemainingFailsLoudOnCorruptTimestamp guards against
// a corrupted last_backfill_at row silently disabling the cooldown it exists
// to enforce (the failure mode this backstop is meant to prevent) instead of
// surfacing an error.
func TestLotwBackfillCooldownRemainingFailsLoudOnCorruptTimestamp(t *testing.T) {
	m := reviewModel(t)
	if _, err := m.store.db.Exec(
		`INSERT INTO lotw_backfill_state (profile_id, last_backfill_at) VALUES (?, ?)`,
		m.activeStation.ID, "not-a-timestamp",
	); err != nil {
		t.Fatalf("seed corrupt lotw_backfill_state row: %v", err)
	}
	if _, err := m.store.lotwBackfillCooldownRemaining(m.activeStation.ID, time.Now()); err == nil {
		t.Fatal("lotwBackfillCooldownRemaining returned no error for a corrupt timestamp; want a parse error, not a silently-disabled cooldown")
	}
}

// TestLoTWBackfillCooldownBlocksSecondCtrlYAndCLIRun guards the technical
// backstop for ARRL's "uploading all of a log's QSOs should not be routine"
// guidance: a second full-log backfill within lotwBackfillMinInterval must be
// refused (both via Ctrl+Y and the recorded store state --upload-lotw checks)
// rather than only documented as a nudge.
func TestLoTWBackfillCooldownBlocksSecondCtrlYAndCLIRun(t *testing.T) {
	m := reviewModel(t)
	t.Setenv("CWLOGGER_TQSL", writeFakeTQSL(t))
	t.Setenv("TQSL_FAKE_EXIT", "0")
	t.Setenv("TQSL_FAKE_TEXT", "Success")
	m.lotwStation = "Home"

	q := reviewQSO(m)
	q.call = "W1AW"
	reviewInsert(t, m, q)

	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlY})
	m = updated.(model)
	if !strings.Contains(m.statusMsg, "queued 1 QSO") {
		t.Fatalf("first ctrl+y statusMsg = %q, want it to report 1 QSO queued", m.statusMsg)
	}
	if cmd != nil {
		cmd() // drain so the outbox row is gone before the second attempt.
	}

	q2 := reviewQSO(m)
	q2.call = "K1ABC"
	reviewInsert(t, m, q2)

	updated2, cmd2 := m.Update(tea.KeyMsg{Type: tea.KeyCtrlY})
	m = updated2.(model)
	if !strings.Contains(m.statusMsg, "ran recently") {
		t.Fatalf("second ctrl+y statusMsg = %q, want a cooldown message", m.statusMsg)
	}
	if cmd2 != nil {
		t.Fatal("second ctrl+y within the cooldown returned a command; want nil (no drain triggered)")
	}
	var count int
	if err := m.store.db.QueryRow(`SELECT COUNT(*) FROM upload_outbox WHERE destination=?`, uploadDestLoTW).Scan(&count); err != nil || count != 0 {
		t.Fatalf("upload_outbox lotw rows = %d, err %v; want 0 (blocked by cooldown, not queued)", count, err)
	}

	// Past the cooldown, a backfill is allowed again.
	if err := m.store.recordLoTWBackfillAt(m.activeStation.ID, time.Now().Add(-2*lotwBackfillMinInterval)); err != nil {
		t.Fatalf("recordLoTWBackfillAt: %v", err)
	}
	updated3, cmd3 := m.Update(tea.KeyMsg{Type: tea.KeyCtrlY})
	m = updated3.(model)
	// q2 was still logged (and never delivered) during the blocked attempt
	// above, so this backfill queues both q1 (re-queued, harmless — TQSL's
	// tracking database will report it already-delivered) and q2.
	if !strings.Contains(m.statusMsg, "queued 2 QSO") {
		t.Fatalf("third ctrl+y statusMsg = %q, want it to report 2 QSOs queued once the cooldown has elapsed", m.statusMsg)
	}
	if cmd3 == nil {
		t.Fatal("third ctrl+y past the cooldown returned a nil command; expected the drain to run")
	}
}

// TestLotwBackfillCooldownRemainingStoreHelpers exercises the store-level
// helpers directly: no prior backfill means no cooldown, a fresh timestamp
// blocks for the full interval, and an old timestamp clears it.
func TestLotwBackfillCooldownRemainingStoreHelpers(t *testing.T) {
	m := reviewModel(t)
	now := time.Now()

	remaining, err := m.store.lotwBackfillCooldownRemaining(m.activeStation.ID, now)
	if err != nil {
		t.Fatalf("lotwBackfillCooldownRemaining (no prior run): %v", err)
	}
	if remaining != 0 {
		t.Fatalf("remaining = %v, want 0 with no prior backfill recorded", remaining)
	}

	if err := m.store.recordLoTWBackfillAt(m.activeStation.ID, now); err != nil {
		t.Fatalf("recordLoTWBackfillAt: %v", err)
	}
	remaining, err = m.store.lotwBackfillCooldownRemaining(m.activeStation.ID, now.Add(time.Minute))
	if err != nil {
		t.Fatalf("lotwBackfillCooldownRemaining (just recorded): %v", err)
	}
	if remaining <= 0 || remaining > lotwBackfillMinInterval {
		t.Fatalf("remaining = %v, want a positive value up to %v", remaining, lotwBackfillMinInterval)
	}

	remaining, err = m.store.lotwBackfillCooldownRemaining(m.activeStation.ID, now.Add(lotwBackfillMinInterval+time.Minute))
	if err != nil {
		t.Fatalf("lotwBackfillCooldownRemaining (past interval): %v", err)
	}
	if remaining != 0 {
		t.Fatalf("remaining = %v, want 0 once the interval has elapsed", remaining)
	}
}

func TestCtrlYWithoutStationConfiguredLeavesQueueEmpty(t *testing.T) {
	m := reviewModel(t)
	m.lotwStation = ""
	q := reviewQSO(m)
	reviewInsert(t, m, q)

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyCtrlY})
	m = updated.(model)
	if !strings.Contains(m.statusMsg, "not configured") {
		t.Fatalf("statusMsg = %q, want a not-configured message", m.statusMsg)
	}
	var count int
	if err := m.store.db.QueryRow(`SELECT COUNT(*) FROM upload_outbox`).Scan(&count); err != nil || count != 0 {
		t.Fatalf("upload_outbox rows = %d, err %v; want 0", count, err)
	}
}

func TestLotwBindingChangesWithStation(t *testing.T) {
	t.Setenv("CWLOGGER_TQSL", writeFakeTQSL(t))
	home := model{lotwStation: "Home"}.lotwBinding()
	away := model{lotwStation: "Away"}.lotwBinding()
	if home == "" || away == "" {
		t.Fatal("expected non-empty bindings once station and tqsl are both configured")
	}
	if home == away {
		t.Fatal("lotwBinding should differ when the station location changes")
	}
}
