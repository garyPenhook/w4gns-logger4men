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
	qrzKeyFile       = "qrz.comAPIkey"
	qrzUserAgent     = "W4GNS-Logger/1.0 (amateur radio contact logger)"
	qrzUploadTimeout = 15 * time.Second
)

// qrzLogbookAPI is a var (not const) so tests can point it at a local server.
var qrzLogbookAPI = "https://logbook.qrz.com/api"

// loadQRZAPIKey returns the QRZ Logbook API key used to upload logged QSOs.
// W4GNS_QRZ_KEY overrides the on-disk key file, mirroring how W4GNS_DB
// overrides the database path. An empty return disables uploads.
func loadQRZAPIKey() string {
	if key := strings.TrimSpace(os.Getenv("W4GNS_QRZ_KEY")); key != "" {
		return key
	}
	contents, err := os.ReadFile(qrzKeyFile)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(contents))
}

type qrzUploadMsg struct {
	call  string
	logID string
	err   error
}

// qrzUploadCmd uploads one freshly logged QSO to the operator's QRZ Logbook.
// It runs asynchronously, matching the existing POTA-lookup and backup
// commands, so the terminal UI never blocks on network I/O. A blank apiKey
// means QRZ upload is not configured, so no command is returned.
func qrzUploadCmd(apiKey, stationCallsign string, q qso) tea.Cmd {
	if strings.TrimSpace(apiKey) == "" {
		return nil
	}
	call := q.call
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), qrzUploadTimeout)
		defer cancel()
		logID, err := uploadQSOToQRZ(ctx, apiKey, stationCallsign, q)
		return qrzUploadMsg{call: call, logID: logID, err: err}
	}
}

// uploadQSOToQRZ posts a single QSO to the QRZ Logbook API and returns the
// assigned LOGID on success. See
// https://www.qrz.com/docs/logbook/QRZLogbookAPI.html
func uploadQSOToQRZ(ctx context.Context, apiKey, stationCallsign string, q qso) (string, error) {
	form := url.Values{
		"KEY":    {apiKey},
		"ACTION": {"INSERT"},
		"ADIF":   {singleQSOADIF(q, stationCallsign)},
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
	body, err := io.ReadAll(response.Body)
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

// singleQSOADIF renders one QSO as a single ADIF record for the QRZ upload.
func singleQSOADIF(q qso, stationCallsign string) string {
	var b strings.Builder
	fields := adifQSOFields(q)
	if strings.TrimSpace(stationCallsign) != "" {
		fields = append(fields, struct{ name, value string }{"STATION_CALLSIGN", stationCallsign})
	}
	for _, field := range fields {
		if strings.TrimSpace(field.value) == "" {
			continue
		}
		fmt.Fprintf(&b, "<%s:%d>%s", field.name, len([]byte(field.value)), field.value)
	}
	b.WriteString("<EOR>")
	return b.String()
}
