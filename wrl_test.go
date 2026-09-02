package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadWRLAPIKeyTightensLoosePermissions(t *testing.T) {
	dir := t.TempDir()
	oldWD, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(oldWD)
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	const wrlKeyFile = "worldradioleague.comAPIkey"
	if err := os.WriteFile(wrlKeyFile, []byte("wrl_live_abc123\n"), 0o664); err != nil {
		t.Fatal(err)
	}

	key := loadWRLAPIKey()
	if key != "wrl_live_abc123" {
		t.Fatalf("key = %q, want wrl_live_abc123", key)
	}
	info, err := os.Stat(filepath.Join(dir, wrlKeyFile))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != qrzKeyFilePermBits {
		t.Fatalf("permissions = %o, want %o", info.Mode().Perm(), qrzKeyFilePermBits)
	}
}

func TestLoadWRLAPIKeyPrefersEnvOverride(t *testing.T) {
	t.Setenv("W4GNS_WRL_KEY", "ENV-KEY")
	if got := loadWRLAPIKey(); got != "ENV-KEY" {
		t.Fatalf("loadWRLAPIKey() = %q, want ENV-KEY", got)
	}
}

// TestLoadWRLLogbookIDReadsSecondLine covers the fallback needed because
// WRL's own "use my only logbook" default-resolution has been observed to
// fail server-side (INTERNAL_ERROR) rather than actually applying it, so an
// operator must be able to pin a logbookId explicitly.
func TestLoadWRLLogbookIDReadsSecondLine(t *testing.T) {
	dir := t.TempDir()
	oldWD, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(oldWD)
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile("worldradioleague.comAPIkey", []byte("wrl_live_abc123\nlogbook-uuid\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	if got := loadWRLAPIKey(); got != "wrl_live_abc123" {
		t.Fatalf("loadWRLAPIKey() = %q, want wrl_live_abc123", got)
	}
	if got := loadWRLLogbookID(); got != "logbook-uuid" {
		t.Fatalf("loadWRLLogbookID() = %q, want logbook-uuid", got)
	}
}

func TestLoadWRLLogbookIDPrefersEnvOverride(t *testing.T) {
	t.Setenv("W4GNS_WRL_LOGBOOK_ID", "ENV-LOGBOOK")
	if got := loadWRLLogbookID(); got != "ENV-LOGBOOK" {
		t.Fatalf("loadWRLLogbookID() = %q, want ENV-LOGBOOK", got)
	}
}

func TestUploadQSOToWRLSendsLogbookID(t *testing.T) {
	var gotLogbookID string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var contact wrlContact
		if err := json.NewDecoder(r.Body).Decode(&contact); err != nil {
			t.Fatal(err)
		}
		gotLogbookID = contact.LogbookID
		w.WriteHeader(http.StatusCreated)
	}))
	defer srv.Close()

	old := wrlContactsAPI
	wrlContactsAPI = srv.URL
	defer func() { wrlContactsAPI = old }()

	q := validTestQSO()
	q.frequency = "14.025"
	if err := uploadQSOToWRL(context.Background(), "testkey", "logbook-uuid", q); err != nil {
		t.Fatalf("uploadQSOToWRL: %v", err)
	}
	if gotLogbookID != "logbook-uuid" {
		t.Fatalf("LogbookID = %q, want logbook-uuid", gotLogbookID)
	}
}

func TestUploadQSOToWRLPostsExpectedContact(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("X-API-Key"); got != "testkey" {
			t.Fatalf("X-API-Key = %q, want testkey", got)
		}
		var contact wrlContact
		if err := json.NewDecoder(r.Body).Decode(&contact); err != nil {
			t.Fatal(err)
		}
		if contact.Call != "W1AW" || contact.Band != "20m" || contact.Mode != "CW" {
			t.Fatalf("unexpected contact: %+v", contact)
		}
		if contact.Freq != 14.025 {
			t.Fatalf("Freq = %v, want 14.025", contact.Freq)
		}
		w.WriteHeader(http.StatusCreated)
		w.Write([]byte(`{"data":{},"meta":{},"error":null}`))
	}))
	defer srv.Close()

	old := wrlContactsAPI
	wrlContactsAPI = srv.URL
	defer func() { wrlContactsAPI = old }()

	q := validTestQSO()
	q.call = "W1AW"
	q.frequency = "14.025"
	if err := uploadQSOToWRL(context.Background(), "testkey", "", q); err != nil {
		t.Fatalf("uploadQSOToWRL: %v", err)
	}
}

func TestUploadQSOToWRLReportsErrorEnvelope(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnprocessableEntity)
		w.Write([]byte(`{"data":null,"meta":null,"error":{"code":"VALIDATION_ERROR","message":"bad band"}}`))
	}))
	defer srv.Close()

	old := wrlContactsAPI
	wrlContactsAPI = srv.URL
	defer func() { wrlContactsAPI = old }()

	q := validTestQSO()
	q.call = "W1AW"
	q.frequency = "14.025"
	err := uploadQSOToWRL(context.Background(), "testkey", "", q)
	if err == nil || !strings.Contains(err.Error(), "bad band") {
		t.Fatalf("expected a bad band error, got %v", err)
	}
}

func TestUploadQSOToWRLFallsBackToBandDefaultFrequency(t *testing.T) {
	var gotFreq float64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var contact wrlContact
		if err := json.NewDecoder(r.Body).Decode(&contact); err != nil {
			t.Fatal(err)
		}
		gotFreq = contact.Freq
		w.WriteHeader(http.StatusCreated)
	}))
	defer srv.Close()

	old := wrlContactsAPI
	wrlContactsAPI = srv.URL
	defer func() { wrlContactsAPI = old }()

	q := validTestQSO() // band 20M, blank frequency
	if err := uploadQSOToWRL(context.Background(), "testkey", "", q); err != nil {
		t.Fatalf("uploadQSOToWRL: %v", err)
	}
	if gotFreq != 14.025 {
		t.Fatalf("Freq = %v, want the 20M band default 14.025", gotFreq)
	}
}

func TestUploadQSOToWRLRejectsUnparsableFrequency(t *testing.T) {
	q := validTestQSO()
	q.band = "not-a-band"
	q.frequency = ""
	if err := uploadQSOToWRL(context.Background(), "testkey", "", q); err == nil {
		t.Fatal("uploadQSOToWRL returned no error for a blank frequency and unrecognized band")
	}
}

func TestWRLUploadCmdSkipsWhenAPIKeyBlank(t *testing.T) {
	q := validTestQSO()
	q.frequency = "14.025"
	if cmd := wrlUploadCmd("", "", q); cmd != nil {
		t.Fatal("wrlUploadCmd(\"\", ...) returned a non-nil command")
	}
	if cmd := wrlUploadCmd("   ", "", q); cmd != nil {
		t.Fatal("wrlUploadCmd(\"   \", ...) returned a non-nil command")
	}
}

func TestWRLUploadCmdReturnsResultMessage(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
		w.Write([]byte(`{"data":{},"meta":{},"error":null}`))
	}))
	defer srv.Close()

	old := wrlContactsAPI
	wrlContactsAPI = srv.URL
	defer func() { wrlContactsAPI = old }()

	q := validTestQSO()
	q.call = "W1AW"
	q.frequency = "14.025"
	cmd := wrlUploadCmd("testkey", "", q)
	if cmd == nil {
		t.Fatal("wrlUploadCmd returned a nil command for a non-blank API key")
	}
	msg, ok := cmd().(wrlUploadMsg)
	if !ok {
		t.Fatalf("wrlUploadCmd()() = %T, want wrlUploadMsg", msg)
	}
	if msg.err != nil {
		t.Fatalf("wrlUploadMsg.err = %v, want nil", msg.err)
	}
	if msg.call != "W1AW" {
		t.Fatalf("wrlUploadMsg.call = %q, want W1AW", msg.call)
	}
}
