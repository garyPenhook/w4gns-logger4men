package main

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// writeFakeTQSL installs a fake tqsl script that ignores its arguments,
// echoes a "Final Status" line matching what real tqsl prints, and exits
// with the code taken from the TQSL_FAKE_EXIT env var (set per test via
// t.Setenv), returning the script's path for W4GNS_TQSL.
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
	t.Setenv("W4GNS_TQSL", "/tmp/does-not-need-to-exist-for-this-check")
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
	t.Setenv("W4GNS_TQSL", "")
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

	t.Setenv("W4GNS_TQSL", "")
	t.Setenv("PATH", t.TempDir())
	m = model{lotwStation: "Home"}
	if got := m.lotwBinding(); got != "" {
		t.Fatalf("lotwBinding() = %q, want empty when tqsl is unresolvable", got)
	}
}

// TestDrainOutboxBatchesLoTWEntriesIntoOneCommand guards against a future
// drainOutbox switch edit dropping the LoTW batch path: two QSOs enqueued for
// "lotw" must collapse into a single tea.Cmd (one tqsl call), while a QRZ/WRL
// row stays on its own per-entry command.
func TestDrainOutboxBatchesLoTWEntriesIntoOneCommand(t *testing.T) {
	m := reviewModel(t)
	t.Setenv("W4GNS_TQSL", writeFakeTQSL(t))
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
	t.Setenv("W4GNS_TQSL", writeFakeTQSL(t))
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
	t.Setenv("W4GNS_TQSL", writeFakeTQSL(t))
	home := model{lotwStation: "Home"}.lotwBinding()
	away := model{lotwStation: "Away"}.lotwBinding()
	if home == "" || away == "" {
		t.Fatal("expected non-empty bindings once station and tqsl are both configured")
	}
	if home == away {
		t.Fatal("lotwBinding should differ when the station location changes")
	}
}
