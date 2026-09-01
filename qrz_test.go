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
