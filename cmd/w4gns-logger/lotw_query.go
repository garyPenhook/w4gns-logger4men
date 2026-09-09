package main

import (
	"bufio"
	"context"
	"database/sql"
	"fmt"
	"io"
	"net/http"
	"net/url"
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

func (s *store) saveLoTWSyncState(profileID int64, state lotwSyncState) error {
	_, err := s.db.Exec(
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
// documented default. qso_qslsince, when state.LastQSL is non-empty, makes
// the request incremental. ownCall, when non-empty, sets qso_owncall so a
// multi-callsign LoTW account only returns this profile's confirmations.
func buildLoTWReportURL(login, password, ownCall string, state lotwSyncState) string {
	values := url.Values{}
	values.Set("login", login)
	values.Set("password", password)
	values.Set("qso_query", "1")
	values.Set("qso_qsl", "yes")
	if strings.TrimSpace(state.LastQSL) != "" {
		values.Set("qso_qslsince", state.LastQSL)
	}
	if strings.TrimSpace(ownCall) != "" {
		values.Set("qso_owncall", ownCall)
	}
	return lotwReportURL + "?" + values.Encode()
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
// identical. parseADIRecords itself discards header fields (ADIF permits an
// omitted header, so it can't distinguish "header field" from "record field
// before any EOH" and treats both as record data) — this is the only way to
// recover APP_LoTW_LASTQSL/APP_LoTW_LASTQSORX, which arrive only in the
// header, never as a per-record field.
func parseLoTWReportHeader(br *bufio.Reader) (map[string]string, error) {
	header := make(map[string]string)
	fieldsSeen := 0
	for {
		if fieldsSeen > maxLoTWHeaderFields {
			return nil, fmt.Errorf("LoTW report header exceeds %d fields without an <EOH> — response may be malformed or missing its header", maxLoTWHeaderFields)
		}
		if err := discardUntil(br, '<'); err != nil {
			if err == io.EOF {
				return header, nil
			}
			return nil, fmt.Errorf("read LoTW report header: %w", err)
		}
		tag, err := readUntil(br, '>', maxADIFTagLength)
		if err != nil {
			return nil, fmt.Errorf("LoTW report header tag is unterminated or too long: %w", err)
		}
		descriptor := strings.TrimSpace(string(tag[:len(tag)-1]))
		if strings.EqualFold(descriptor, "EOH") {
			return header, nil
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
			return nil, fmt.Errorf("LoTW report header field %q declares length %d, exceeding the %d-byte limit", parts[0], length, maxADIFFieldBytes)
		}
		value := make([]byte, length)
		if _, err := io.ReadFull(br, value); err != nil {
			return nil, fmt.Errorf("LoTW report header field %q: %w", parts[0], err)
		}
		header[strings.ToUpper(strings.TrimSpace(parts[0]))] = string(value)
		fieldsSeen++
	}
}

// syncLoTWConfirmations queries lotwreport.adi for QSL confirmations new
// since the profile's last sync, upserts each into lotw_confirmation
// (matched to a local QSO where possible), and advances the sync bookmark
// from the response header. login/password are the LoTW website credentials;
// ownCall scopes the query to one callsign on a multi-callsign LoTW account.
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
		return lotwSyncSummary{}, fmt.Errorf("build LoTW confirmation query: %w", err)
	}
	client := &http.Client{Timeout: lotwQueryTimeout}
	response, err := client.Do(request)
	if err != nil {
		return lotwSyncSummary{}, fmt.Errorf("query LoTW confirmations: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		snippet, _ := io.ReadAll(io.LimitReader(response.Body, 4096))
		return lotwSyncSummary{}, fmt.Errorf("LoTW confirmation query failed (%s): %s", response.Status, strings.TrimSpace(string(snippet)))
	}

	br := bufio.NewReaderSize(response.Body, 64*1024)
	header, err := parseLoTWReportHeader(br)
	if err != nil {
		return lotwSyncSummary{}, err
	}
	// LoTW reports an error (bad login, etc.) as an ADIF comment/text response
	// rather than a non-200 status; a header with no fields at all and no
	// records to follow is the signal something other than a report came
	// back. Not treated as fatal here — parseADIRecords below simply finds no
	// <EOR> and returns cleanly with zero records, which is indistinguishable
	// from "nothing new since last sync" and reported as such.

	summary := lotwSyncSummary{}
	now := time.Now().UTC().Format(time.RFC3339)
	parseErr := parseADIRecords(br, func(record map[string]string) error {
		summary.Fetched++
		matched, err := st.upsertLoTWConfirmation(profileID, record, now)
		if err != nil {
			return err
		}
		if matched {
			summary.Matched++
		}
		return nil
	})
	if parseErr != nil {
		return summary, parseErr
	}

	newState := state
	if v := strings.TrimSpace(header["APP_LOTW_LASTQSL"]); v != "" {
		newState.LastQSL = v
	}
	if v := strings.TrimSpace(header["APP_LOTW_LASTQSORX"]); v != "" {
		newState.LastQSORX = v
	}
	if newState != state {
		if err := st.saveLoTWSyncState(profileID, newState); err != nil {
			return summary, err
		}
	}
	return summary, nil
}

// upsertLoTWConfirmation records one lotwreport.adi record for profileID,
// matching it to a local QSO by call/band/mode within lotwMatchWindow of the
// confirmed time (see lotwMatchWindow). ON CONFLICT keeps a re-run over an
// overlapping date range idempotent rather than duplicating rows.
func (s *store) upsertLoTWConfirmation(profileID int64, record map[string]string, syncedAt string) (matched bool, err error) {
	call := strings.ToUpper(strings.TrimSpace(record["CALL"]))
	if call == "" {
		return false, nil
	}
	band := strings.ToUpper(strings.TrimSpace(record["BAND"]))
	mode := strings.ToUpper(strings.TrimSpace(record["MODE"]))
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

	var qsoID sql.NullInt64
	if band != "" && mode != "" && len(qsoDate) == 8 && len(timeOn) == 6 {
		id, ok, matchErr := s.matchLoTWConfirmation(profileID, call, band, mode, qsoDate, timeOn)
		if matchErr != nil {
			return false, matchErr
		}
		if ok {
			qsoID = sql.NullInt64{Int64: id, Valid: true}
			matched = true
		}
	}

	_, err = s.db.Exec(
		`INSERT INTO lotw_confirmation (profile_id, qso_id, call, band, mode, qso_date, time_on, dxcc, country, gridsquare, state, cqz, credit_granted, synced_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(profile_id, call, band, mode, qso_date, time_on) DO UPDATE SET
		   qso_id = excluded.qso_id, dxcc = excluded.dxcc, country = excluded.country,
		   gridsquare = excluded.gridsquare, state = excluded.state, cqz = excluded.cqz,
		   credit_granted = excluded.credit_granted, synced_at = excluded.synced_at`,
		profileID, qsoID, call, band, mode, qsoDate, timeOn, dxcc, country, gridsquare, state, cqz, credit, syncedAt,
	)
	if err != nil {
		return false, fmt.Errorf("upsert LoTW confirmation for %s: %w", call, err)
	}
	return matched, nil
}

// matchLoTWConfirmation finds the local QSO a confirmation record belongs to:
// same profile, call, band, and mode, logged within lotwMatchWindow of the
// confirmed QSO's start time. See docs/LoTW_Integration_Design.md's ARRL
// developer guidance table — LoTW itself matches within a ±30-minute window,
// so this mirrors the server's own rule rather than requiring an exact
// TIME_ON match that would miss a confirmation over a rounding difference.
func (s *store) matchLoTWConfirmation(profileID int64, call, band, mode, qsoDate, timeOn string) (int64, bool, error) {
	confirmedAt, err := time.ParseInLocation("20060102150405", qsoDate+timeOn, time.UTC)
	if err != nil {
		return 0, false, nil
	}
	windowStart := confirmedAt.Add(-lotwMatchWindow).Format("20060102150405")
	windowEnd := confirmedAt.Add(lotwMatchWindow).Format("20060102150405")
	var id int64
	err = s.db.QueryRow(
		`SELECT id FROM qso
		 WHERE profile_id = ? AND UPPER(call) = ? AND UPPER(band) = ? AND UPPER(COALESCE(mode, '')) = ?
		   AND (qso_date || time_on) BETWEEN ? AND ?
		 ORDER BY ABS(strftime('%s', substr(qso_date,1,4)||'-'||substr(qso_date,5,2)||'-'||substr(qso_date,7,2)||' '||substr(time_on,1,2)||':'||substr(time_on,3,2)||':'||substr(time_on,5,2)) - strftime('%s', ?))
		 LIMIT 1`,
		profileID, call, band, mode, windowStart, windowEnd, confirmedAt.Format("2006-01-02 15:04:05"),
	).Scan(&id)
	if err == sql.ErrNoRows {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, fmt.Errorf("match LoTW confirmation for %s: %w", call, err)
	}
	return id, true, nil
}
