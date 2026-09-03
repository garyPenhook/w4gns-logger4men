package main

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

const (
	qrzUserAgent     = "W4GNS-Logger/1.0 (amateur radio contact logger)"
	qrzUploadTimeout = 15 * time.Second
	// maxQRZResponseBytes bounds how much of the response this reads: QRZ's
	// reply is a short "RESULT=OK&LOGID=...&COUNT=1" line, so this is far
	// larger than any real response, just enough to stop an unbounded read
	// if the endpoint (or a MITM) ever returns something huge.
	maxQRZResponseBytes = 64 * 1024
)

// qrzLogbookAPI is a var (not const) so tests can point it at a local server.
var qrzLogbookAPI = "https://logbook.qrz.com/api"

// qrzKeyFilePermBits is the maximum permission bits a healthy key file
// should have: owner read/write only. Anything looser leaks the key to every
// other local account able to read the working directory.
const qrzKeyFilePermBits = 0o600

// loadQRZAPIKey returns the QRZ Logbook API key used to upload logged QSOs.
// W4GNS_QRZ_KEY overrides the on-disk key file, mirroring how W4GNS_DB
// overrides the database path. An empty return disables uploads.
func loadQRZAPIKey() string {
	if key := strings.TrimSpace(os.Getenv("W4GNS_QRZ_KEY")); key != "" {
		return key
	}
	keyFile := defaultQRZKeyPath()
	tightenKeyFilePermissions(keyFile)
	contents, err := os.ReadFile(keyFile)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(contents))
}

// tightenKeyFilePermissions best-effort chmods a credentials file (QRZ
// Logbook key or QRZ XML login) to owner-only read/write. .gitignore keeps
// such files out of version control, but it does nothing about other local
// accounts reading the file directly, so this self-heals a too-permissive
// mode (e.g. the default umask leaving it group/world readable) on every
// startup.
func tightenKeyFilePermissions(keyFile string) {
	info, err := os.Stat(keyFile)
	if err != nil {
		return
	}
	if info.Mode().Perm()&^qrzKeyFilePermBits != 0 {
		if err := os.Chmod(keyFile, qrzKeyFilePermBits); err != nil {
			fmt.Fprintf(os.Stderr, "warning: could not restrict %s to owner-only permissions: %v\n", keyFile, err)
		}
	}
}

type qrzUploadMsg struct {
	qsoID int64
	call  string
	logID string
	err   error
}

// qrzUploadCmd uploads one freshly logged QSO to the operator's QRZ Logbook.
// It runs asynchronously, matching the existing POTA-lookup and backup
// commands, so the terminal UI never blocks on network I/O. A blank apiKey
// means QRZ upload is not configured, so no command is returned.
func qrzUploadCmd(apiKey string, q qso) tea.Cmd {
	if strings.TrimSpace(apiKey) == "" {
		return nil
	}
	call := q.call
	id := q.id
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), qrzUploadTimeout)
		defer cancel()
		logID, err := uploadQSOToQRZ(ctx, apiKey, q)
		return qrzUploadMsg{qsoID: id, call: call, logID: logID, err: err}
	}
}

// uploadQSOToQRZ posts a single QSO to the QRZ Logbook API and returns the
// assigned LOGID on success. See
// https://www.qrz.com/docs/logbook/QRZLogbookAPI.html
func uploadQSOToQRZ(ctx context.Context, apiKey string, q qso) (string, error) {
	form := url.Values{
		"KEY":    {apiKey},
		"ACTION": {"INSERT"},
		"ADIF":   {singleQSOADIF(q)},
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, qrzLogbookAPI, strings.NewReader(form.Encode()))
	if err != nil {
		return "", fmt.Errorf("create QRZ upload request: %w", err)
	}
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("User-Agent", qrzUserAgent)

	client := &http.Client{Timeout: qrzUploadTimeout}
	response, err := client.Do(request)
	if err != nil {
		return "", fmt.Errorf("upload QSO to QRZ: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return "", fmt.Errorf("upload QSO to QRZ: %s", response.Status)
	}
	// QRZ's response is a short key=value line; a cap far larger than any
	// real response guards against an unbounded read if the endpoint (or a
	// MITM) ever returns something huge.
	body, err := io.ReadAll(io.LimitReader(response.Body, maxQRZResponseBytes))
	if err != nil {
		return "", fmt.Errorf("read QRZ response: %w", err)
	}

	// The API responds with the same key=value&key=value encoding as the
	// request, e.g. "RESULT=OK&LOGID=123456789&COUNT=1".
	values, err := url.ParseQuery(strings.TrimSpace(string(body)))
	if err != nil {
		return "", fmt.Errorf("parse QRZ response %q: %w", body, err)
	}
	switch values.Get("RESULT") {
	case "OK", "REPLACE":
		return values.Get("LOGID"), nil
	case "FAIL":
		return "", fmt.Errorf("QRZ rejected QSO: %s", values.Get("REASON"))
	default:
		return "", fmt.Errorf("unexpected QRZ response: %s", strings.TrimSpace(string(body)))
	}
}

// singleQSOADIF renders one QSO as a single ADIF record for the QRZ upload,
// sharing field construction with the bulk exporter via adifQSOFields.
func singleQSOADIF(q qso) string {
	var b strings.Builder
	for _, field := range adifQSOFields(q) {
		if strings.TrimSpace(field.value) == "" {
			continue
		}
		if err := writeADIFField(&b, field.name, field.value); err != nil {
			// writeADIFField only fails if the writer errors; strings.Builder
			// never does, so this is unreachable in practice.
			continue
		}
	}
	b.WriteString("<EOR>")
	return b.String()
}
