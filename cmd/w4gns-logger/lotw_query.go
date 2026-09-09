package main

import (
	"bufio"
	"context"
	"database/sql"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// lotwConfirmationSchema stores inbound LoTW QSL confirmation data queried
// from lotwreport.adi (see fetchLoTWConfirmations below). This is separate
// from upload_outbox/upload_log (outbound delivery records) since it tracks
// what ARRL has confirmed back, not what this app has sent.
const lotwConfirmationSchema = `
CREATE TABLE IF NOT EXISTS lotw_confirmation (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    profile_id INTEGER NOT NULL,
    qso_id INTEGER,
    call TEXT NOT NULL,
    band TEXT NOT NULL DEFAULT '',
    mode TEXT NOT NULL DEFAULT '',
    qso_date TEXT NOT NULL DEFAULT '',
    time_on TEXT NOT NULL DEFAULT '',
    dxcc TEXT NOT NULL DEFAULT '',
    country TEXT NOT NULL DEFAULT '',
    gridsquare TEXT NOT NULL DEFAULT '',
    state TEXT NOT NULL DEFAULT '',
    cqz TEXT NOT NULL DEFAULT '',
    credit_granted TEXT NOT NULL DEFAULT '',
    synced_at TEXT NOT NULL,
    UNIQUE (profile_id, call, band, mode, qso_date, time_on)
);
CREATE INDEX IF NOT EXISTS idx_lotw_confirmation_qso ON lotw_confirmation(qso_id);
CREATE INDEX IF NOT EXISTS idx_lotw_confirmation_profile ON lotw_confirmation(profile_id);

CREATE TABLE IF NOT EXISTS lotw_sync_state (
    profile_id INTEGER PRIMARY KEY,
    last_qsl TEXT NOT NULL DEFAULT '',
    last_qsorx TEXT NOT NULL DEFAULT '',
    synced_at TEXT NOT NULL DEFAULT ''
);
`

// lotwReportURL is the ARRL LoTW confirmation query endpoint. A var (not
// const) so tests can point it at a local server.
var lotwReportURL = "https://lotw.arrl.org/lotwuser/lotwreport.adi"

// lotwQueryTimeout bounds one confirmation-sync HTTP round trip. Generous
// relative to lotwUploadTimeout because an initial (non-incremental) sync can
// return an operator's entire confirmed history.
const lotwQueryTimeout = 5 * time.Minute

// lotwMatchWindow mirrors the ±30-minute LoTW dupe-matching window documented
// in docs/LoTW_Integration_Design.md's ARRL developer guidance table: a
// confirmation is joined to a local QSO whose logged time falls within this
// window of the confirmed QSO's TIME_ON, not an exact match.
const lotwMatchWindow = 30 * time.Minute

// lotwSyncSummary reports the outcome of one confirmation sync for display in
// the stats panel's status line.
type lotwSyncSummary struct {
	Fetched int // confirmation records received from LoTW
	Matched int // of those, how many joined to a local QSO
}

// lotwSyncState is the incremental-sync bookmark for one profile: the most
// recent QSL/QSO-received timestamps LoTW reported, echoed back as
// qso_qslsince/qso_qsorxsince on the next sync so it asks "what's new since
// last time" rather than re-fetching the whole confirmed history every run —
// the same "should not be routine" principle the upload side follows (see
// docs/LoTW_Integration_Design.md).
type lotwSyncState struct {
	LastQSL   string
	LastQSORX string
}

func (s *store) loadLoTWSyncState(profileID int64) (lotwSyncState, error) {
	var state lotwSyncState
	err := s.db.QueryRow(
		`SELECT last_qsl, last_qsorx FROM lotw_sync_state WHERE profile_id = ?`, profileID,
	).Scan(&state.LastQSL, &state.LastQSORX)
	if err == sql.ErrNoRows {
		return lotwSyncState{}, nil
	}
	if err != nil {
		return lotwSyncState{}, fmt.Errorf("load LoTW sync state for profile %d: %w", profileID, err)
	}
	return state, nil
}

// lotwSyncExecer is the subset of *sql.DB / *sql.Tx the confirmation-sync
// read/write helpers need, so the same query logic can run standalone (the
// direct-call test/legacy path) or inside the one transaction that makes a
// sync's records-plus-bookmark commit atomic (see syncLoTWConfirmations).
type lotwSyncExecer interface {
	Exec(query string, args ...any) (sql.Result, error)
	Query(query string, args ...any) (*sql.Rows, error)
	QueryRow(query string, args ...any) *sql.Row
}

func saveLoTWSyncState(exec lotwSyncExecer, profileID int64, state lotwSyncState) error {
	_, err := exec.Exec(
		`INSERT INTO lotw_sync_state (profile_id, last_qsl, last_qsorx, synced_at) VALUES (?, ?, ?, ?)
		 ON CONFLICT(profile_id) DO UPDATE SET last_qsl = excluded.last_qsl, last_qsorx = excluded.last_qsorx, synced_at = excluded.synced_at`,
		profileID, state.LastQSL, state.LastQSORX, time.Now().UTC().Format(time.RFC3339),
	)
	if err != nil {
		return fmt.Errorf("save LoTW sync state for profile %d: %w", profileID, err)
	}
	return nil
}

// buildLoTWReportURL constructs the lotwreport.adi query. login/password are
// the operator's LoTW website credentials (loadLoTWLogin/loadLoTWWebPass) —
// distinct from the TQSL Callsign Certificate used to sign uploads.
// qso_qsl=yes scopes the response to confirmed (QSL'd) records, per ARRL's
// documented default. qso_qsldetail=yes additionally requests the QSLing
// station's location data (DXCC/COUNTRY/CQZ/GRIDSQUARE/STATE/IOTA) — without
// it ARRL does not promise those fields, yet upsertLoTWConfirmation stores
// them. qso_qslsince, when state.LastQSL is non-empty, makes the request
// incremental. ownCall, when non-empty, sets qso_owncall so a multi-callsign
// LoTW account only returns this profile's confirmations.
func buildLoTWReportURL(login, password, ownCall string, state lotwSyncState) string {
	values := url.Values{}
	values.Set("login", login)
	values.Set("password", password)
	values.Set("qso_query", "1")
	values.Set("qso_qsl", "yes")
	values.Set("qso_qsldetail", "yes")
	if strings.TrimSpace(state.LastQSL) != "" {
		values.Set("qso_qslsince", state.LastQSL)
	}
	if strings.TrimSpace(ownCall) != "" {
		values.Set("qso_owncall", ownCall)
	}
	return lotwReportURL + "?" + values.Encode()
}

// redactLoTWCredentials strips login/password (and their URL-encoded forms)
// out of err's text. Go's net/url.Error.Error() embeds the full request URL,
// and buildLoTWReportURL puts both credentials in that URL's query string —
// without this, a network failure (DNS, TLS, connection refused, timeout)
// would surface the operator's LoTW password in plain text in the status
// line (stats_panel.go) or on the CLI's stderr.
func redactLoTWCredentials(err error, login, password string) string {
	msg := err.Error()
	for _, secret := range []string{login, password} {
		if secret == "" {
			continue
		}
		msg = strings.ReplaceAll(msg, secret, "[redacted]")
		if encoded := url.QueryEscape(secret); encoded != secret {
			msg = strings.ReplaceAll(msg, encoded, "[redacted]")
		}
	}
	return msg
}

// maxLoTWHeaderFields bounds how many fields parseLoTWReportHeader reads
// before giving up on ever finding <EOH>. ARRL's documented lotwreport.adi
// response always carries a short header (a comment line plus a handful of
// APP_LoTW_* fields) terminated by <EOH>, so this cap is far larger than any
// real header. Without it, a response that omitted <EOH> entirely — which
// bare ADIF permits, even though this specific endpoint always sends one —
// would make the loop below silently consume every QSO record in the
// response as a "header field" and discard it, leaving parseADIRecords
// nothing to read and the sync reporting zero confirmations with no error.
const maxLoTWHeaderFields = 64

// parseLoTWReportHeader reads ADIF header fields (everything before <EOH>)
// from br, reusing the same tag/field primitives as parseADIRecords
// (discardUntil/readUntil/parseADIFLength) since the wire format is
// identical. found reports whether <EOH> was actually seen: ARRL's
// documentation is explicit that "if the query fails, an HTML page
// containing an explanation will be returned; the absence of an ADIF end of
// header tag can be used to detect this outcome" — an expired password,
// revoked login, or server error returned as HTTP 200 looks exactly like a
// response with no header, and the caller must treat that as a failure, not
// as "zero confirmations".
func parseLoTWReportHeader(br *bufio.Reader) (header map[string]string, found bool, err error) {
	header = make(map[string]string)
	fieldsSeen := 0
	for {
		if fieldsSeen > maxLoTWHeaderFields {
			return nil, false, fmt.Errorf("LoTW report header exceeds %d fields without an <EOH> — response may be malformed or missing its header", maxLoTWHeaderFields)
		}
		if err := discardUntil(br, '<'); err != nil {
			if err == io.EOF {
				return header, false, nil
			}
			return nil, false, fmt.Errorf("read LoTW report header: %w", err)
		}
		tag, err := readUntil(br, '>', maxADIFTagLength)
		if err != nil {
			return nil, false, fmt.Errorf("LoTW report header tag is unterminated or too long: %w", err)
		}
		descriptor := strings.TrimSpace(string(tag[:len(tag)-1]))
		if strings.EqualFold(descriptor, "EOH") {
			return header, true, nil
		}
		parts := strings.SplitN(descriptor, ":", 2)
		if len(parts) < 2 {
			continue // plain text or a non-field tag ahead of the first real header field
		}
		length, err := parseADIFLength(parts[1])
		if err != nil {
			continue
		}
		if length > maxADIFFieldBytes {
			return nil, false, fmt.Errorf("LoTW report header field %q declares length %d, exceeding the %d-byte limit", parts[0], length, maxADIFFieldBytes)
		}
		value := make([]byte, length)
		if _, err := io.ReadFull(br, value); err != nil {
			return nil, false, fmt.Errorf("LoTW report header field %q: %w", parts[0], err)
		}
		header[strings.ToUpper(strings.TrimSpace(parts[0]))] = string(value)
		fieldsSeen++
	}
}

// parseLoTWReportRecords reads QSO/QSL records from br (positioned just past
// <EOH>), invoking onRecord for each <EOR>-terminated record, until EOF.
// sawEOF reports whether ARRL's documented APP_LoTW_EOF trailing marker was
// seen — a field ARRL's docs describe as appearing after all QSO records and
// deliberately "not followed by <EOR>", specifically so a client "can verify
// that the file was completely received". This app doesn't reuse the
// stricter, general-purpose parseADIRecords (adif_import.go) here because
// that parser treats a trailing field with no closing <EOR> as an error
// (correctly, for a plain ADIF file, which has no such trailing marker) —
// this endpoint's response format is close to but not the same as bare ADIF.
func parseLoTWReportRecords(br *bufio.Reader, onRecord func(map[string]string) error) (sawEOF bool, err error) {
	record := make(map[string]string)
	for {
		if err := discardUntil(br, '<'); err != nil {
			if err == io.EOF {
				return false, nil
			}
			return false, fmt.Errorf("read LoTW report records: %w", err)
		}
		tag, err := readUntil(br, '>', maxADIFTagLength)
		if err != nil {
			return false, fmt.Errorf("LoTW report record tag is unterminated or too long: %w", err)
		}
		descriptor := strings.TrimSpace(string(tag[:len(tag)-1]))
		if strings.EqualFold(descriptor, "EOR") {
			if len(record) > 0 {
				if err := onRecord(record); err != nil {
					return false, err
				}
				record = make(map[string]string)
			}
			continue
		}
		// APP_LoTW_EOF is a bare tag with no ":length" suffix (confirmed
		// against live lotwreport.adi responses) — same shape as <eor>/<eoh>,
		// not a NAME:LENGTH field. It must be recognized here, before the
		// colon split below discards it as "not a field" and this loop reads
		// straight past it to genuine end-of-stream, permanently unable to
		// reach the length-prefixed check that used to sit after
		// io.ReadFull.
		if strings.EqualFold(descriptor, "APP_LoTW_EOF") {
			return true, nil
		}
		parts := strings.SplitN(descriptor, ":", 2)
		if len(parts) < 2 {
			continue
		}
		length, err := parseADIFLength(parts[1])
		if err != nil {
			continue
		}
		if length > maxADIFFieldBytes {
			return false, fmt.Errorf("LoTW report field %q declares length %d, exceeding the %d-byte limit", parts[0], length, maxADIFFieldBytes)
		}
		value := make([]byte, length)
		if _, err := io.ReadFull(br, value); err != nil {
			return false, fmt.Errorf("LoTW report field %q: %w", parts[0], err)
		}
		name := strings.ToUpper(strings.TrimSpace(parts[0]))
		if len(record) >= maxADIFFieldsPerRecord {
			return false, fmt.Errorf("LoTW report record exceeds %d fields", maxADIFFieldsPerRecord)
		}
		record[name] = string(value)
	}
}

// syncLoTWConfirmations queries lotwreport.adi for QSL confirmations new
// since the profile's last sync, upserts each into lotw_confirmation
// (matched to a local QSO where possible), and advances the sync bookmark
// from the response header. login/password are the LoTW website credentials;
// ownCall scopes the query to one callsign on a multi-callsign LoTW account.
//
// The whole response is validated (header present, APP_LoTW_EOF marker seen,
// APP_LoTW_NUMREC matching the records actually parsed) before anything is
// written, and every confirmation plus the advanced bookmark commit together
// in one transaction — a response truncated mid-transfer must not leave a
// bookmark advanced past confirmations it never delivered.
func syncLoTWConfirmations(ctx context.Context, st *store, profileID int64, login, password, ownCall string) (lotwSyncSummary, error) {
	if strings.TrimSpace(login) == "" || strings.TrimSpace(password) == "" {
		return lotwSyncSummary{}, fmt.Errorf("LoTW login/password not configured (set lotw.login/lotw.webpass or CWLOGGER_LOTW_LOGIN/CWLOGGER_LOTW_WEBPASS)")
	}
	state, err := st.loadLoTWSyncState(profileID)
	if err != nil {
		return lotwSyncSummary{}, err
	}

	request, err := http.NewRequestWithContext(ctx, http.MethodGet, buildLoTWReportURL(login, password, ownCall, state), nil)
	if err != nil {
		return lotwSyncSummary{}, fmt.Errorf("build LoTW confirmation query: %s", redactLoTWCredentials(err, login, password))
	}
	client := &http.Client{Timeout: lotwQueryTimeout}
	response, err := client.Do(request)
	if err != nil {
		return lotwSyncSummary{}, fmt.Errorf("query LoTW confirmations: %s", redactLoTWCredentials(err, login, password))
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		snippet, _ := io.ReadAll(io.LimitReader(response.Body, 4096))
		return lotwSyncSummary{}, fmt.Errorf("LoTW confirmation query failed (%s): %s", response.Status, strings.TrimSpace(string(snippet)))
	}

	br := bufio.NewReaderSize(response.Body, 64*1024)
	header, foundEOH, err := parseLoTWReportHeader(br)
	if err != nil {
		return lotwSyncSummary{}, err
	}
	if !foundEOH {
		// Per ARRL: a failed query (bad login/password, server error) comes
		// back as an HTML explanation page instead of a non-200 status, and
		// its absent <EOH> is the documented way to detect that.
		return lotwSyncSummary{}, fmt.Errorf("LoTW confirmation query failed: response had no ADIF header (check lotw.login/lotw.webpass, or LoTW may be reporting an error)")
	}

	var records []map[string]string
	sawEOF, err := parseLoTWReportRecords(br, func(record map[string]string) error {
		records = append(records, record)
		return nil
	})
	if err != nil {
		return lotwSyncSummary{}, err
	}
	if !sawEOF {
		return lotwSyncSummary{}, fmt.Errorf("LoTW confirmation query failed: response ended without the documented end-of-file marker (possibly a truncated download) — nothing was recorded")
	}
	if numRecStr := strings.TrimSpace(header["APP_LOTW_NUMREC"]); numRecStr != "" {
		if numRec, convErr := strconv.Atoi(numRecStr); convErr == nil && numRec != len(records) {
			return lotwSyncSummary{}, fmt.Errorf("LoTW confirmation query failed: response declared %d record(s) but %d were received — nothing was recorded", numRec, len(records))
		}
	}

	newState := state
	if v := strings.TrimSpace(header["APP_LOTW_LASTQSL"]); v != "" {
		newState.LastQSL = v
	}
	if v := strings.TrimSpace(header["APP_LOTW_LASTQSORX"]); v != "" {
		newState.LastQSORX = v
	}

	tx, err := st.db.Begin()
	if err != nil {
		return lotwSyncSummary{}, fmt.Errorf("begin LoTW confirmation sync: %w", err)
	}
	defer tx.Rollback()

	summary := lotwSyncSummary{}
	now := time.Now().UTC().Format(time.RFC3339)
	for _, record := range records {
		summary.Fetched++
		matched, err := upsertLoTWConfirmation(tx, profileID, record, now)
		if err != nil {
			return lotwSyncSummary{}, err
		}
		if matched {
			summary.Matched++
		}
	}
	if newState != state {
		if err := saveLoTWSyncState(tx, profileID, newState); err != nil {
			return lotwSyncSummary{}, err
		}
	}
	if err := tx.Commit(); err != nil {
		return lotwSyncSummary{}, fmt.Errorf("commit LoTW confirmation sync: %w", err)
	}
	return summary, nil
}

// upsertLoTWConfirmation records one lotwreport.adi record for profileID,
// matching it to a local QSO by call/band and time window (see
// matchLoTWConfirmation), then upserting it keyed by
// (profile_id, call, band, mode, qso_date, time_on) so a re-sync over an
// overlapping date range updates rather than duplicates the row. exec runs
// standalone (the direct-call test path) or inside syncLoTWConfirmations's
// transaction.
func upsertLoTWConfirmation(exec lotwSyncExecer, profileID int64, record map[string]string, syncedAt string) (matched bool, err error) {
	call := strings.ToUpper(strings.TrimSpace(record["CALL"]))
	if call == "" {
		return false, nil
	}
	band := strings.ToUpper(strings.TrimSpace(record["BAND"]))
	// ARRL's docs: "a QSO's mode may be mapped to a different value... a
	// future ADIF version may not include the MODE field, using instead the
	// APP_LoTW_MODE field" — fall back to that when MODE itself is absent.
	mode := strings.ToUpper(strings.TrimSpace(firstNonEmpty(record["MODE"], record["APP_LOTW_MODE"])))
	qsoDate := strings.TrimSpace(record["QSO_DATE"])
	timeOn := strings.TrimSpace(record["TIME_ON"])
	if len(timeOn) == 4 {
		timeOn += "00"
	}
	credit := firstNonEmpty(record["CREDIT_GRANTED"], record["APP_LOTW_CREDIT_GRANTED"])
	dxcc := strings.TrimSpace(record["DXCC"])
	country := strings.TrimSpace(record["COUNTRY"])
	gridsquare := strings.ToUpper(strings.TrimSpace(record["GRIDSQUARE"]))
	state := strings.ToUpper(strings.TrimSpace(record["STATE"]))
	cqz := strings.TrimSpace(record["CQZ"])
	// IOTA/CQZ/GRIDSQUARE/STATE/DXCC/COUNTRY are only guaranteed present when
	// qso_qsldetail=yes is requested (see buildLoTWReportURL) — this is the
	// QSLing station's own confirmed location data, distinct from (and
	// authoritative over) this app's locally resolved qso.dxcc/state/etc.,
	// which lotw_stats.go's *ConfirmedQuery constants read from this table
	// rather than the joined qso row for exactly that reason.
	iotaRef := strings.ToUpper(strings.TrimSpace(record["IOTA"]))

	var qsoID sql.NullInt64
	if band != "" && len(qsoDate) == 8 && len(timeOn) == 6 {
		id, ok, matchErr := matchLoTWConfirmation(exec, profileID, call, band, mode, qsoDate, timeOn)
		if matchErr != nil {
			return false, matchErr
		}
		if ok {
			qsoID = sql.NullInt64{Int64: id, Valid: true}
			matched = true
		}
	}

	_, err = exec.Exec(
		`INSERT INTO lotw_confirmation (profile_id, qso_id, call, band, mode, qso_date, time_on, dxcc, country, gridsquare, state, cqz, credit_granted, iota_ref, synced_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(profile_id, call, band, mode, qso_date, time_on) DO UPDATE SET
		   qso_id = excluded.qso_id, dxcc = excluded.dxcc, country = excluded.country,
		   gridsquare = excluded.gridsquare, state = excluded.state, cqz = excluded.cqz,
		   credit_granted = excluded.credit_granted, iota_ref = excluded.iota_ref, synced_at = excluded.synced_at`,
		profileID, qsoID, call, band, mode, qsoDate, timeOn, dxcc, country, gridsquare, state, cqz, credit, iotaRef, syncedAt,
	)
	if err != nil {
		return false, fmt.Errorf("upsert LoTW confirmation for %s: %w", call, err)
	}
	return matched, nil
}

func (s *store) upsertLoTWConfirmation(profileID int64, record map[string]string, syncedAt string) (bool, error) {
	return upsertLoTWConfirmation(s.db, profileID, record, syncedAt)
}

// matchLoTWConfirmation finds the local QSO a confirmation record belongs to:
// same profile, call, and band, logged within lotwMatchWindow of the
// confirmed QSO's start time (see docs/LoTW_Integration_Design.md's ARRL
// developer guidance table — LoTW itself matches within a ±30-minute
// window). Per ARRL's guidance ("it may be best to leave the mode out of the
// comparison except in the case where a downloaded record matches multiple
// QSO records") mode is not part of the primary filter — TQSL can remap an
// uploaded mode to a different value than what's logged locally — and is
// only used to break a tie when more than one local QSO falls in the window.
func matchLoTWConfirmation(exec lotwSyncExecer, profileID int64, call, band, mode, qsoDate, timeOn string) (int64, bool, error) {
	confirmedAt, err := time.ParseInLocation("20060102150405", qsoDate+timeOn, time.UTC)
	if err != nil {
		return 0, false, nil
	}
	windowStart := confirmedAt.Add(-lotwMatchWindow).Format("20060102150405")
	windowEnd := confirmedAt.Add(lotwMatchWindow).Format("20060102150405")
	rows, err := exec.Query(
		`SELECT id, UPPER(COALESCE(mode, '')), qso_date, time_on FROM qso
		 WHERE profile_id = ? AND UPPER(call) = ? AND UPPER(band) = ?
		   AND (qso_date || time_on) BETWEEN ? AND ?`,
		profileID, call, band, windowStart, windowEnd,
	)
	if err != nil {
		return 0, false, fmt.Errorf("match LoTW confirmation for %s: %w", call, err)
	}
	defer rows.Close()

	type candidate struct {
		id    int64
		mode  string
		delta time.Duration
	}
	var candidates []candidate
	for rows.Next() {
		var id int64
		var qMode, qDate, qTime string
		if err := rows.Scan(&id, &qMode, &qDate, &qTime); err != nil {
			return 0, false, fmt.Errorf("match LoTW confirmation for %s: %w", call, err)
		}
		at, parseErr := time.ParseInLocation("20060102150405", qDate+qTime, time.UTC)
		if parseErr != nil {
			continue
		}
		delta := at.Sub(confirmedAt)
		if delta < 0 {
			delta = -delta
		}
		candidates = append(candidates, candidate{id: id, mode: qMode, delta: delta})
	}
	if err := rows.Err(); err != nil {
		return 0, false, fmt.Errorf("match LoTW confirmation for %s: %w", call, err)
	}
	if len(candidates) == 0 {
		return 0, false, nil
	}
	if len(candidates) == 1 {
		return candidates[0].id, true, nil
	}
	// Multiple local QSOs fall in the same window: use mode only now, to
	// disambiguate, per ARRL's guidance quoted above.
	best := candidates[0]
	bestModeMatch := mode != "" && strings.EqualFold(best.mode, mode)
	for _, c := range candidates[1:] {
		modeMatch := mode != "" && strings.EqualFold(c.mode, mode)
		switch {
		case modeMatch && !bestModeMatch:
			best, bestModeMatch = c, true
		case modeMatch == bestModeMatch && c.delta < best.delta:
			best = c
		}
	}
	return best.id, true, nil
}
