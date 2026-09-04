package main

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime/pprof"
	"sync"
	"testing"
	"time"
)

// waitForBgTasksOrDumpLeaks waits for wg with a bounded timeout instead of
// letting a stuck shutdown-drain hang the test suite until go test's own
// (much longer, much less specific) timeout. On timeout it captures Go
// 1.27's "goroutineleak" profile — precise stacks of every goroutine
// permanently blocked on a channel or sync primitive — so a real regression
// here fails fast with the exact stuck call site instead of an opaque hang.
func waitForBgTasksOrDumpLeaks(t *testing.T, wg *sync.WaitGroup, timeout time.Duration) {
	t.Helper()
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(timeout):
		var buf bytes.Buffer
		if p := pprof.Lookup("goroutineleak"); p != nil {
			_ = p.WriteTo(&buf, 1)
		}
		t.Fatalf("bgTasks.Wait() did not return within %s — shutdown would hang; leaked goroutines:\n%s", timeout, buf.String())
	}
}

// assertNoGoroutineLeaks reports a test failure with full stacks if the
// goroutineleak profile finds anything, and does nothing while there's no
// support (older Go, or the profile omitted from the build). Call after
// draining background work so the assertion is "shutdown truly left nothing
// blocked," not just "the one WaitGroup we happened to track reached zero."
func assertNoGoroutineLeaks(t *testing.T) {
	t.Helper()
	p := pprof.Lookup("goroutineleak")
	if p == nil {
		return
	}
	if count := p.Count(); count > 0 {
		var buf bytes.Buffer
		_ = p.WriteTo(&buf, 1)
		t.Fatalf("goroutineleak profile reports %d leaked goroutine(s) after shutdown drain:\n%s", count, buf.String())
	}
}

// TestShutdownDrainLeavesNoLeakedGoroutines exercises the exact sequence
// main() runs on every exit path (Esc, Ctrl+C, SIGTERM, SIGHUP, kill -INT —
// see main.go's post-p.Run() block): cancel bgCtx, then bgTasks.Wait().
// Real background work is in flight when shutdown fires — a QRZ outbox
// upload, a WRL outbox upload, and an ADIF import — the same three
// bgTasks-tracked jobs the roadmap (docs/ROADMAP.md §0 P3) flags as needing
// "full drainOutbox lifecycle" coverage. If any of them failed to register
// with bgTasks before its goroutine started, or failed to call wg.Done() on
// every return path, this either hangs (caught by the bounded wait above) or
// leaves a goroutine parked past drain (caught by the goroutineleak
// assertion below) instead of silently passing.
func TestShutdownDrainLeavesNoLeakedGoroutines(t *testing.T) {
	qrzSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("RESULT=OK&LOGID=1&COUNT=1"))
	}))
	defer qrzSrv.Close()
	oldQRZAPI := qrzLogbookAPI
	qrzLogbookAPI = qrzSrv.URL
	t.Cleanup(func() { qrzLogbookAPI = oldQRZAPI })

	wrlSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
	}))
	defer wrlSrv.Close()
	oldWRLAPI := wrlContactsAPI
	wrlContactsAPI = wrlSrv.URL
	t.Cleanup(func() { wrlContactsAPI = oldWRLAPI })

	st := openTestStore(t)
	profile, err := st.activeStationProfile()
	if err != nil {
		t.Fatal(err)
	}
	q := validTestQSO()
	q.profileID = profile.ID
	id, err := st.insertQSO(q)
	if err != nil {
		t.Fatal(err)
	}
	q.id = id

	m := initialModel(st)
	m.qrzAPIKey = "test-key"
	m.wrlAPIKey = "test-key"
	m.wrlLogbookID = "test-logbook"

	qrzCmd := m.qrzOutboxUploadCmd(q)
	if qrzCmd == nil {
		t.Fatal("qrzOutboxUploadCmd returned nil with a configured API key")
	}
	wrlCmd := m.wrlOutboxUploadCmd(q)
	if wrlCmd == nil {
		t.Fatal("wrlOutboxUploadCmd returned nil with a configured API key")
	}

	adiPath := filepath.Join(t.TempDir(), "import.adi")
	adi := `<ADIF_VER:5>3.1.7<EOH><CALL:3>W1A<QSO_DATE:8>20260831<TIME_ON:6>120000<QSO_DATE_OFF:8>20260831<TIME_OFF:6>120030<BAND:3>20M<MODE:2>CW<RST_SENT:3>599<RST_RCVD:3>599<EOR>`
	if err := os.WriteFile(adiPath, []byte(adi), 0o600); err != nil {
		t.Fatal(err)
	}
	m.bgTasks.Add(1) // mirrors updateADIFImport registering the job before dispatch
	importCmd := m.importADIFFile(adiPath)

	// Dispatch every command the same way Bubble Tea does: each runs in its
	// own goroutine, and Bubble Tea does not wait for them before returning
	// from p.Run() — bgTasks is what stands in for that wait on shutdown.
	go qrzCmd()
	go wrlCmd()
	go importCmd()

	// Mirror main()'s shutdown sequence exactly.
	m.bgCancel()
	waitForBgTasksOrDumpLeaks(t, m.bgTasks, 5*time.Second)

	assertNoGoroutineLeaks(t)
}
