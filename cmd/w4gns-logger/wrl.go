package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

const (
	wrlProgramID     = "W4GNS-Logger"
	wrlUserAgent     = "W4GNS-Logger/1.0 (amateur radio contact logger)"
	wrlUploadTimeout = 15 * time.Second
	// maxWRLResponseBytes bounds how much of the response this reads: WRL's
	// error envelope is a short JSON object, so this is far larger than any
	// real response, just enough to stop an unbounded read if the endpoint
	// (or a MITM) ever returns something huge.
	maxWRLResponseBytes = 64 * 1024
)

// wrlContactsAPI is a var (not const) so tests can point it at a local server.
var wrlContactsAPI = "https://api.worldradioleague.com/v1/contacts"

// loadWRLAPIKey returns the World Radio League API key used to forward
// logged QSOs. W4GNS_WRL_KEY overrides the on-disk key file, mirroring
// loadQRZAPIKey. An empty return disables forwarding.
func loadWRLAPIKey() string {
	if key := strings.TrimSpace(os.Getenv("W4GNS_WRL_KEY")); key != "" {
		return key
	}
	return strings.TrimSpace(firstLine(readWRLKeyFile()))
}

// loadWRLLogbookID returns the destination logbook for forwarded QSOs.
// W4GNS_WRL_LOGBOOK_ID overrides the second line of the on-disk key file.
// WRL is documented to fall back to the account's only logbook when this is
// omitted, but that fallback has been observed to fail server-side with a
// 500 rather than resolving it, so an operator with a single logbook still
// needs to supply its ID explicitly (see World Radio League's GET
// /v1/logbooks) for uploads to succeed. An empty return omits logbookId from
// the request, relying on WRL's own default-resolution.
func loadWRLLogbookID() string {
	if id := strings.TrimSpace(os.Getenv("W4GNS_WRL_LOGBOOK_ID")); id != "" {
		return id
	}
	contents := readWRLKeyFile()
	lines := strings.SplitN(strings.TrimRight(contents, "\n"), "\n", 2)
	if len(lines) < 2 {
		return ""
	}
	return strings.TrimSpace(lines[1])
}

func readWRLKeyFile() string {
	keyFile := defaultWRLKeyPath()
	tightenKeyFilePermissions(keyFile)
	contents, err := os.ReadFile(keyFile)
	if err != nil {
		return ""
	}
	return string(contents)
}

func firstLine(s string) string {
	line, _, _ := strings.Cut(s, "\n")
	return line
}

type wrlUploadMsg struct {
	qsoID             int64
	call              string
	err               error
	deliveryPersisted bool
	queueErr          error
}

// wrlUploadCmd forwards one freshly logged QSO to World Radio League. It
// runs asynchronously, matching qrzUploadCmd, so the terminal UI never
// blocks on network I/O. A blank apiKey means WRL forwarding is not
// configured, so no command is returned.
func wrlUploadCmd(apiKey, logbookID string, q qso) tea.Cmd {
	if strings.TrimSpace(apiKey) == "" {
		return nil
	}
	call := q.call
	id := q.id
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), wrlUploadTimeout)
		defer cancel()
		err := uploadQSOToWRL(ctx, apiKey, logbookID, q)
		return wrlUploadMsg{qsoID: id, call: call, err: err}
	}
}

// wrlContact mirrors the required and commonly-available optional fields of
// World Radio League's ContactCreate schema (POST /v1/contacts). Field names
// follow the API's camelCase convention, not ADIF's.
type wrlContact struct {
	ProgramID       string  `json:"programId"`
	LogbookID       string  `json:"logbookId,omitempty"`
	Call            string  `json:"call"`
	Timestamp       string  `json:"timestamp"`
	Freq            float64 `json:"freq"`
	Band            string  `json:"band"`
	Mode            string  `json:"mode"`
	RSTSent         string  `json:"rstSent,omitempty"`
	RSTRcvd         string  `json:"rstRcvd,omitempty"`
	TXPwr           string  `json:"txPwr,omitempty"`
	Notes           string  `json:"notes,omitempty"`
	StationCallsign string  `json:"stationCallsign,omitempty"`
	MyGridsquare    string  `json:"myGridsquare,omitempty"`
	Name            string  `json:"name,omitempty"`
	Gridsquare      string  `json:"gridsquare,omitempty"`
	QTH             string  `json:"qth,omitempty"`
	State           string  `json:"state,omitempty"`
	Operator        string  `json:"operator,omitempty"`
}

type wrlErrorEnvelope struct {
	Error struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

// uploadQSOToWRL posts a single QSO to World Radio League's contact log. See
// https://worldradioleague.com/developer/
func uploadQSOToWRL(ctx context.Context, apiKey, logbookID string, q qso) error {
	frequency := strings.TrimSpace(q.frequency)
	if frequency == "" {
		// Frequency is optional for local logging (qso_validation.go only
		// checks it when non-blank) but required by WRL, so fall back to the
		// band's default rather than silently dropping the QSO from every
		// upload that omitted it.
		if index := bandIndex(q.band); index >= 0 {
			frequency = amateurBands[index].DefaultMHz
		}
	}
	freq, err := strconv.ParseFloat(frequency, 64)
	if err != nil {
		return fmt.Errorf("QSO frequency %q is not a number: %w", q.frequency, err)
	}
	contact := wrlContact{
		ProgramID:       wrlProgramID,
		LogbookID:       strings.TrimSpace(logbookID),
		Call:            q.call,
		Timestamp:       q.time.UTC().Format(time.RFC3339),
		Freq:            freq,
		Band:            strings.ToLower(q.band),
		Mode:            q.mode,
		RSTSent:         q.rstSent,
		RSTRcvd:         q.rstRcvd,
		TXPwr:           q.txPower,
		Notes:           q.comment,
		StationCallsign: q.stationCallsign,
		MyGridsquare:    q.myGridSquare,
		Name:            q.name,
		Gridsquare:      q.grid,
		QTH:             q.qth,
		State:           q.state,
		Operator:        q.operatorName,
	}
	body, err := json.Marshal(contact)
	if err != nil {
		return fmt.Errorf("encode WRL contact: %w", err)
	}

	request, err := http.NewRequestWithContext(ctx, http.MethodPost, wrlContactsAPI, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create WRL upload request: %w", err)
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-API-Key", apiKey)
	request.Header.Set("User-Agent", wrlUserAgent)

	client := &http.Client{Timeout: wrlUploadTimeout}
	response, err := client.Do(request)
	if err != nil {
		return fmt.Errorf("upload QSO to WRL: %w", err)
	}
	defer response.Body.Close()

	respBody, err := io.ReadAll(io.LimitReader(response.Body, maxWRLResponseBytes))
	if err != nil {
		return fmt.Errorf("read WRL response: %w", err)
	}
	if response.StatusCode == http.StatusCreated {
		return nil
	}

	var envelope wrlErrorEnvelope
	if err := json.Unmarshal(respBody, &envelope); err == nil && envelope.Error.Message != "" {
		return fmt.Errorf("WRL rejected QSO (%s): %s", response.Status, envelope.Error.Message)
	}
	return fmt.Errorf("upload QSO to WRL: %s", response.Status)
}
