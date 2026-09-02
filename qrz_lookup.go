package main

import (
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// qrzXMLAPI is a var (not const) so tests can point it at a local server.
// This is QRZ's XML callsign-lookup API, a separate service (and
// subscription) from the Logbook upload API in qrz.go.
var qrzXMLAPI = "https://xmldata.qrz.com/xml/current/"

const (
	qrzXMLUserAgent     = "W4GNS-Logger/1.0 (amateur radio contact logger)"
	qrzXMLLookupTimeout = 15 * time.Second
	// maxQRZXMLResponseBytes bounds how much of the response this reads: a
	// QRZ XML record is a few hundred bytes to a few KB, so this is far
	// larger than any real response, just enough to stop an unbounded read
	// if the endpoint (or a MITM) ever returns something huge.
	maxQRZXMLResponseBytes = 64 * 1024
)

// qrzXMLCreds holds the QRZ.com website login used by the XML lookup API,
// distinct from the Logbook API key in qrz.go.
type qrzXMLCreds struct {
	username string
	password string
}

func (c qrzXMLCreds) empty() bool {
	return strings.TrimSpace(c.username) == "" || strings.TrimSpace(c.password) == ""
}

// loadQRZXMLCredentials returns the QRZ.com username/password used for
// callsign lookups. W4GNS_QRZ_XML_USER/W4GNS_QRZ_XML_PASS override the
// on-disk credentials file, mirroring how W4GNS_QRZ_KEY overrides the
// Logbook key file. Empty credentials disable lookups.
func loadQRZXMLCredentials() qrzXMLCreds {
	if user := strings.TrimSpace(os.Getenv("W4GNS_QRZ_XML_USER")); user != "" {
		return qrzXMLCreds{username: user, password: strings.TrimSpace(os.Getenv("W4GNS_QRZ_XML_PASS"))}
	}
	credFile := defaultQRZXMLCredPath()
	tightenKeyFilePermissions(credFile)
	contents, err := os.ReadFile(credFile)
	if err != nil {
		return qrzXMLCreds{}
	}
	lines := strings.SplitN(strings.TrimRight(string(contents), "\n"), "\n", 2)
	creds := qrzXMLCreds{username: strings.TrimSpace(lines[0])}
	if len(lines) > 1 {
		creds.password = strings.TrimSpace(lines[1])
	}
	return creds
}

// saveQRZXMLCredentials writes username/password to the same file
// loadQRZXMLCredentials reads (username on line 1, password on line 2),
// so credentials entered in Station Setup persist across restarts without
// the operator having to create the file by hand. It always overwrites the
// full file; a blank creds value disables lookup on the next load, the same
// as if the file had never been created.
func saveQRZXMLCredentials(creds qrzXMLCreds) error {
	path := defaultQRZXMLCredPath()
	content := creds.username + "\n" + creds.password + "\n"
	if err := os.WriteFile(path, []byte(content), qrzKeyFilePermBits); err != nil {
		return fmt.Errorf("write QRZ XML credentials: %w", err)
	}
	// os.WriteFile only applies qrzKeyFilePermBits to a newly created file;
	// an existing file keeps whatever mode it already had, so this self-heals
	// it the same way loadQRZXMLCredentials does on read.
	tightenKeyFilePermissions(path)
	return nil
}

// qrzCallsignRecord holds the fields this app auto-fills from a QRZ XML
// callsign lookup.
type qrzCallsignRecord struct {
	name   string
	qth    string
	grid   string
	state  string
	county string
	email  string
}

type qrzCallsignLookupMsg struct {
	call       string
	record     qrzCallsignRecord
	sessionKey string
	err        error
}

// lookupQRZCallsignCmd looks up call against the QRZ XML API, logging in
// first if sessionKey is empty and retrying once with a fresh login if the
// session has expired. A qrzXMLCreds zero value means lookup isn't
// configured, so no command is returned (mirrors qrzUploadCmd's blank-key
// guard in qrz.go).
func lookupQRZCallsignCmd(creds qrzXMLCreds, sessionKey, call string) tea.Cmd {
	if creds.empty() || strings.TrimSpace(call) == "" {
		return nil
	}
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), qrzXMLLookupTimeout)
		defer cancel()

		key := sessionKey
		if key == "" {
			var err error
			key, err = qrzXMLLogin(ctx, creds)
			if err != nil {
				return qrzCallsignLookupMsg{call: call, err: err}
			}
		}

		record, err := qrzXMLLookupCallsign(ctx, key, call)
		if isQRZSessionExpired(err) {
			key, err = qrzXMLLogin(ctx, creds)
			if err != nil {
				return qrzCallsignLookupMsg{call: call, err: err}
			}
			record, err = qrzXMLLookupCallsign(ctx, key, call)
		}
		if err != nil {
			return qrzCallsignLookupMsg{call: call, sessionKey: key, err: err}
		}
		return qrzCallsignLookupMsg{call: call, record: record, sessionKey: key}
	}
}

// qrzXMLSessionError signals an expired/invalid QRZ XML session, distinct
// from a login failure or a not-found callsign, so lookupQRZCallsignCmd
// knows to retry once with a fresh login.
type qrzXMLSessionError struct {
	message string
}

func (e *qrzXMLSessionError) Error() string { return e.message }

func isQRZSessionExpired(err error) bool {
	_, ok := err.(*qrzXMLSessionError)
	return ok
}

// qrzXMLResponse mirrors the subset of QRZ's XML schema this app reads. See
// https://www.qrz.com/XML/current_spec.html
type qrzXMLResponse struct {
	Session struct {
		Key   string `xml:"Key"`
		Error string `xml:"Error"`
	} `xml:"Session"`
	Callsign struct {
		FirstName string `xml:"fname"`
		LastName  string `xml:"name"`
		City      string `xml:"addr2"`
		State     string `xml:"state"`
		Grid      string `xml:"grid"`
		County    string `xml:"county"`
		Email     string `xml:"email"`
	} `xml:"Callsign"`
}

func qrzXMLLogin(ctx context.Context, creds qrzXMLCreds) (string, error) {
	values := url.Values{"username": {creds.username}, "password": {creds.password}, "agent": {qrzXMLUserAgent}}
	response, err := fetchQRZXML(ctx, values)
	if err != nil {
		return "", fmt.Errorf("QRZ XML login: %w", err)
	}
	if response.Session.Key == "" {
		return "", fmt.Errorf("QRZ XML login failed: %s", response.Session.Error)
	}
	return response.Session.Key, nil
}

func qrzXMLLookupCallsign(ctx context.Context, sessionKey, call string) (qrzCallsignRecord, error) {
	values := url.Values{"s": {sessionKey}, "callsign": {call}}
	response, err := fetchQRZXML(ctx, values)
	if err != nil {
		return qrzCallsignRecord{}, fmt.Errorf("QRZ XML lookup: %w", err)
	}
	if response.Session.Error != "" {
		if strings.Contains(strings.ToLower(response.Session.Error), "session") {
			return qrzCallsignRecord{}, &qrzXMLSessionError{message: response.Session.Error}
		}
		return qrzCallsignRecord{}, fmt.Errorf("QRZ XML lookup: %s", response.Session.Error)
	}
	name := strings.TrimSpace(strings.TrimSpace(response.Callsign.FirstName) + " " + strings.TrimSpace(response.Callsign.LastName))
	return qrzCallsignRecord{
		name:   name,
		qth:    strings.TrimSpace(response.Callsign.City),
		grid:   strings.TrimSpace(response.Callsign.Grid),
		state:  strings.TrimSpace(response.Callsign.State),
		county: strings.TrimSpace(response.Callsign.County),
		email:  strings.TrimSpace(response.Callsign.Email),
	}, nil
}

func fetchQRZXML(ctx context.Context, values url.Values) (qrzXMLResponse, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, qrzXMLAPI+"?"+values.Encode(), nil)
	if err != nil {
		return qrzXMLResponse{}, fmt.Errorf("create request: %w", err)
	}
	request.Header.Set("User-Agent", qrzXMLUserAgent)

	client := &http.Client{Timeout: qrzXMLLookupTimeout}
	response, err := client.Do(request)
	if err != nil {
		return qrzXMLResponse{}, fmt.Errorf("request: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return qrzXMLResponse{}, fmt.Errorf("request: %s", response.Status)
	}

	body, err := io.ReadAll(io.LimitReader(response.Body, maxQRZXMLResponseBytes))
	if err != nil {
		return qrzXMLResponse{}, fmt.Errorf("read response: %w", err)
	}
	var parsed qrzXMLResponse
	if err := xml.Unmarshal(body, &parsed); err != nil {
		return qrzXMLResponse{}, fmt.Errorf("parse response: %w", err)
	}
	return parsed, nil
}
