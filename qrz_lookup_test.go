package main

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadQRZXMLCredentialsPrefersEnvOverride(t *testing.T) {
	t.Setenv("W4GNS_QRZ_XML_USER", "envuser")
	t.Setenv("W4GNS_QRZ_XML_PASS", "envpass")
	got := loadQRZXMLCredentials()
	if got.username != "envuser" || got.password != "envpass" {
		t.Fatalf("loadQRZXMLCredentials() = %+v, want envuser/envpass", got)
	}
}

func TestLoadQRZXMLCredentialsReadsFileAndTightensPermissions(t *testing.T) {
	dir := t.TempDir()
	oldWD, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(oldWD)
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	const credFile = "qrz.comXMLlogin"
	if err := os.WriteFile(credFile, []byte("myuser\nmypass\n"), 0o664); err != nil {
		t.Fatal(err)
	}

	creds := loadQRZXMLCredentials()
	if creds.username != "myuser" || creds.password != "mypass" {
		t.Fatalf("loadQRZXMLCredentials() = %+v, want myuser/mypass", creds)
	}
	info, err := os.Stat(filepath.Join(dir, credFile))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != qrzKeyFilePermBits {
		t.Fatalf("permissions = %o, want %o", info.Mode().Perm(), qrzKeyFilePermBits)
	}
}

// TestSaveQRZXMLCredentialsRoundTripsAndTightensPermissions covers the
// in-app login path (Station Setup writes here): what's saved must be what
// loadQRZXMLCredentials reads back, and the file must end up owner-only
// (0600) even though os.WriteFile only applies that mode to a freshly
// created file.
func TestSaveQRZXMLCredentialsRoundTripsAndTightensPermissions(t *testing.T) {
	dir := t.TempDir()
	oldWD, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(oldWD)
	// legacyOrStablePath prefers a "qrz.comXMLlogin" already present in the
	// cwd; chdir into an empty tempdir first so this test's outcome doesn't
	// depend on whatever the invoking process's working directory contains.
	if err := os.Chdir(t.TempDir()); err != nil {
		t.Fatal(err)
	}
	t.Setenv("XDG_CONFIG_HOME", dir)

	if err := saveQRZXMLCredentials(qrzXMLCreds{username: "myuser", password: "mypass"}); err != nil {
		t.Fatalf("saveQRZXMLCredentials: %v", err)
	}

	got := loadQRZXMLCredentials()
	if got.username != "myuser" || got.password != "mypass" {
		t.Fatalf("loadQRZXMLCredentials() = %+v, want myuser/mypass", got)
	}

	path := filepath.Join(dir, "w4gns-logger", "qrz.comXMLlogin")
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != qrzKeyFilePermBits {
		t.Fatalf("permissions = %o, want %o", info.Mode().Perm(), qrzKeyFilePermBits)
	}
}

// TestLookupQRZCallsignCmdSkipsWhenCredsBlank covers the "not configured"
// path: lookupQRZCallsignCmd must return a nil command (no request attempted)
// when no username/password is set, mirroring qrzUploadCmd's blank-key guard.
func TestLookupQRZCallsignCmdSkipsWhenCredsBlank(t *testing.T) {
	if cmd := lookupQRZCallsignCmd(qrzXMLCreds{}, "", "W1AW"); cmd != nil {
		t.Fatal("lookupQRZCallsignCmd with blank creds returned a non-nil command")
	}
	if cmd := lookupQRZCallsignCmd(qrzXMLCreds{username: "u"}, "", "W1AW"); cmd != nil {
		t.Fatal("lookupQRZCallsignCmd with blank password returned a non-nil command")
	}
}

func qrzXMLLoginResponse(key string) string {
	return fmt.Sprintf(`<?xml version="1.0"?><QRZDatabase><Session><Key>%s</Key></Session></QRZDatabase>`, key)
}

func qrzXMLCallsignResponse(fname, name, city, state, grid string) string {
	return qrzXMLCallsignResponseWithCountyEmail(fname, name, city, state, grid, "", "")
}

func qrzXMLCallsignResponseWithCountyEmail(fname, name, city, state, grid, county, email string) string {
	return fmt.Sprintf(`<?xml version="1.0"?><QRZDatabase><Session><Key>somekey</Key></Session>`+
		`<Callsign><fname>%s</fname><name>%s</name><addr2>%s</addr2><state>%s</state><grid>%s</grid><county>%s</county><email>%s</email></Callsign></QRZDatabase>`,
		fname, name, city, state, grid, county, email)
}

// TestLookupQRZCallsignCmdReturnsResultMessage covers the happy path end to
// end: login (no session key yet) followed by a callsign lookup, both
// against the same qrzXMLAPI URL, distinguished by query parameters like the
// real API.
func TestLookupQRZCallsignCmdReturnsResultMessage(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("callsign") != "" {
			w.Write([]byte(qrzXMLCallsignResponseWithCountyEmail("Fred", "Lloyd", "Phoenix", "AZ", "DM43", "Maricopa", "fred@example.com")))
			return
		}
		w.Write([]byte(qrzXMLLoginResponse("session123")))
	}))
	defer srv.Close()

	old := qrzXMLAPI
	qrzXMLAPI = srv.URL
	defer func() { qrzXMLAPI = old }()

	cmd := lookupQRZCallsignCmd(qrzXMLCreds{username: "u", password: "p"}, "", "AA7BQ")
	if cmd == nil {
		t.Fatal("lookupQRZCallsignCmd returned a nil command for configured creds")
	}
	msg, ok := cmd().(qrzCallsignLookupMsg)
	if !ok {
		t.Fatalf("lookupQRZCallsignCmd()() = %T, want qrzCallsignLookupMsg", msg)
	}
	if msg.err != nil {
		t.Fatalf("qrzCallsignLookupMsg.err = %v, want nil", msg.err)
	}
	if msg.record.name != "Fred Lloyd" || msg.record.qth != "Phoenix" || msg.record.state != "AZ" || msg.record.grid != "DM43" ||
		msg.record.county != "Maricopa" || msg.record.email != "fred@example.com" {
		t.Fatalf("record = %+v, want Fred Lloyd/Phoenix/AZ/DM43/Maricopa/fred@example.com", msg.record)
	}
	if msg.sessionKey != "session123" {
		t.Fatalf("sessionKey = %q, want session123", msg.sessionKey)
	}
}

// TestLookupQRZCallsignCmdRetriesOnceOnExpiredSession covers a session that
// expired between calls: the first lookup attempt with a stale key must
// trigger exactly one fresh login and one retry, not an unbounded loop.
func TestLookupQRZCallsignCmdRetriesOnceOnExpiredSession(t *testing.T) {
	loginCount := 0
	lookupAttempts := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("callsign") != "" {
			lookupAttempts++
			if r.URL.Query().Get("s") == "stale-key" {
				w.Write([]byte(`<?xml version="1.0"?><QRZDatabase><Session><Error>Session Timeout</Error></Session></QRZDatabase>`))
				return
			}
			w.Write([]byte(qrzXMLCallsignResponse("Fred", "Lloyd", "Phoenix", "AZ", "DM43")))
			return
		}
		loginCount++
		w.Write([]byte(qrzXMLLoginResponse("fresh-key")))
	}))
	defer srv.Close()

	old := qrzXMLAPI
	qrzXMLAPI = srv.URL
	defer func() { qrzXMLAPI = old }()

	cmd := lookupQRZCallsignCmd(qrzXMLCreds{username: "u", password: "p"}, "stale-key", "AA7BQ")
	msg, ok := cmd().(qrzCallsignLookupMsg)
	if !ok {
		t.Fatalf("lookupQRZCallsignCmd()() = %T, want qrzCallsignLookupMsg", msg)
	}
	if msg.err != nil {
		t.Fatalf("qrzCallsignLookupMsg.err = %v, want nil", msg.err)
	}
	if loginCount != 1 {
		t.Fatalf("loginCount = %d, want exactly 1 fresh login", loginCount)
	}
	if lookupAttempts != 2 {
		t.Fatalf("lookupAttempts = %d, want exactly 2 (stale then retry)", lookupAttempts)
	}
	if msg.sessionKey != "fresh-key" {
		t.Fatalf("sessionKey = %q, want fresh-key", msg.sessionKey)
	}
}

func TestQRZXMLLoginReportsFailure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`<?xml version="1.0"?><QRZDatabase><Session><Error>Username/password incorrect</Error></Session></QRZDatabase>`))
	}))
	defer srv.Close()

	old := qrzXMLAPI
	qrzXMLAPI = srv.URL
	defer func() { qrzXMLAPI = old }()

	cmd := lookupQRZCallsignCmd(qrzXMLCreds{username: "bad", password: "bad"}, "", "AA7BQ")
	msg := cmd().(qrzCallsignLookupMsg)
	if msg.err == nil || !strings.Contains(msg.err.Error(), "incorrect") {
		t.Fatalf("err = %v, want an incorrect-credentials error", msg.err)
	}
}
