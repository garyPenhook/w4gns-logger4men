package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestLoadQRZAPIKeyTightensLoosePermissions guards against the key file
// being left group/world readable (e.g. the default umask), which would let
// every other local account read it even though .gitignore keeps it out of
// version control.
func TestLoadQRZAPIKeyTightensLoosePermissions(t *testing.T) {
	dir := t.TempDir()
	oldWD, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(oldWD)
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	const qrzKeyFile = "qrz.comAPIkey"
	if err := os.WriteFile(qrzKeyFile, []byte("ABCD-1234\n"), 0o664); err != nil {
		t.Fatal(err)
	}

	key := loadQRZAPIKey()
	if key != "ABCD-1234" {
		t.Fatalf("key = %q, want ABCD-1234", key)
	}
	info, err := os.Stat(filepath.Join(dir, qrzKeyFile))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != qrzKeyFilePermBits {
		t.Fatalf("permissions = %o, want %o", info.Mode().Perm(), qrzKeyFilePermBits)
	}
}

func TestLoadQRZAPIKeyPrefersEnvOverride(t *testing.T) {
	t.Setenv("W4GNS_QRZ_KEY", "ENV-KEY")
	if got := loadQRZAPIKey(); got != "ENV-KEY" {
		t.Fatalf("loadQRZAPIKey() = %q, want ENV-KEY", got)
	}
}

func TestUploadQSOToQRZParsesSuccessAndFailureResponses(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Fatal(err)
		}
		if r.Form.Get("ACTION") != "INSERT" || r.Form.Get("KEY") != "testkey" {
			t.Fatalf("unexpected form: %+v", r.Form)
		}
		if !strings.Contains(r.Form.Get("ADIF"), "<CALL:4>W1AW") {
			t.Fatalf("unexpected ADIF: %s", r.Form.Get("ADIF"))
		}
		if strings.Contains(r.Form.Get("ADIF"), "FAIL_ME") {
			w.Write([]byte("RESULT=FAIL&REASON=Duplicate QSO"))
			return
		}
		w.Write([]byte("RESULT=OK&LOGID=42&COUNT=1"))
	}))
	defer srv.Close()

	old := qrzLogbookAPI
	qrzLogbookAPI = srv.URL
	defer func() { qrzLogbookAPI = old }()

	q := validTestQSO()
	q.call = "W1AW"
	logID, err := uploadQSOToQRZ(context.Background(), "testkey", q)
	if err != nil {
		t.Fatalf("uploadQSOToQRZ: %v", err)
	}
	if logID != "42" {
		t.Fatalf("logID = %q, want 42", logID)
	}

	q.comment = "FAIL_ME"
	_, err = uploadQSOToQRZ(context.Background(), "testkey", q)
	if err == nil || !strings.Contains(err.Error(), "Duplicate QSO") {
		t.Fatalf("expected a Duplicate QSO error, got %v", err)
	}
}

// TestUploadQSOToQRZReportsHTTPFailure covers a non-200 response (e.g. QRZ's
// service being down or rate-limiting), which previously had no test
// coverage of its own.
func TestUploadQSOToQRZReportsHTTPFailure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	old := qrzLogbookAPI
	qrzLogbookAPI = srv.URL
	defer func() { qrzLogbookAPI = old }()

	q := validTestQSO()
	q.call = "W1AW"
	if _, err := uploadQSOToQRZ(context.Background(), "testkey", q); err == nil {
		t.Fatal("uploadQSOToQRZ returned no error for a 503 response")
	}
}

// TestUploadQSOToQRZReportsUnexpectedResult covers a response whose RESULT
// is neither OK/REPLACE nor FAIL (e.g. a QRZ API change or an unexpected
// body), which must surface as an error rather than being silently treated
// as success.
func TestUploadQSOToQRZReportsUnexpectedResult(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("RESULT=MAYBE&SOMETHING=ELSE"))
	}))
	defer srv.Close()

	old := qrzLogbookAPI
	qrzLogbookAPI = srv.URL
	defer func() { qrzLogbookAPI = old }()

	q := validTestQSO()
	q.call = "W1AW"
	if _, err := uploadQSOToQRZ(context.Background(), "testkey", q); err == nil {
		t.Fatal("uploadQSOToQRZ returned no error for an unrecognized RESULT value")
	}
}

// TestQRZUploadCmdSkipsWhenAPIKeyBlank covers the "not configured" path:
// qrzUploadCmd must return a nil command (no upload attempt, no QRZ status
// message) when no API key is set, rather than a command that fails at
// request time.
func TestQRZUploadCmdSkipsWhenAPIKeyBlank(t *testing.T) {
	if cmd := qrzUploadCmd("", validTestQSO()); cmd != nil {
		t.Fatal("qrzUploadCmd(\"\", ...) returned a non-nil command")
	}
	if cmd := qrzUploadCmd("   ", validTestQSO()); cmd != nil {
		t.Fatal("qrzUploadCmd(\"   \", ...) returned a non-nil command")
	}
}

// TestQRZUploadCmdReturnsResultMessage covers qrzUploadCmd's happy path end
// to end: the tea.Cmd it returns must actually perform the upload and
// report the call and LOGID via qrzUploadMsg.
func TestQRZUploadCmdReturnsResultMessage(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("RESULT=OK&LOGID=99&COUNT=1"))
	}))
	defer srv.Close()

	old := qrzLogbookAPI
	qrzLogbookAPI = srv.URL
	defer func() { qrzLogbookAPI = old }()

	q := validTestQSO()
	q.call = "W1AW"
	cmd := qrzUploadCmd("testkey", q)
	if cmd == nil {
		t.Fatal("qrzUploadCmd returned a nil command for a non-blank API key")
	}
	msg, ok := cmd().(qrzUploadMsg)
	if !ok {
		t.Fatalf("qrzUploadCmd()() = %T, want qrzUploadMsg", msg)
	}
	if msg.err != nil {
		t.Fatalf("qrzUploadMsg.err = %v, want nil", msg.err)
	}
	if msg.call != "W1AW" || msg.logID != "99" {
		t.Fatalf("qrzUploadMsg = %+v, want call=W1AW logID=99", msg)
	}
}
