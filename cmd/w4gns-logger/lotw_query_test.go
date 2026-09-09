package main

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// lotwReportFixture builds a lotwreport.adi response matching ARRL's
// documented shape: a header carrying APP_LoTW_LASTQSL/LASTQSORX/NUMREC,
// followed by the given records, followed by the documented APP_LoTW_EOF
// trailing marker (not followed by <EOR>).
func lotwReportFixture(lastQSL, lastQSORX string, records []map[string]string) string {
	var b strings.Builder
	b.WriteString("LoTW QSO/QSL query response\n")
	writeADIField := func(name, value string) {
		b.WriteString("<")
		b.WriteString(name)
		b.WriteString(":")
		b.WriteString(itoa(len(value)))
		b.WriteString(">")
		b.WriteString(value)
	}
	if lastQSL != "" {
		writeADIField("APP_LoTW_LASTQSL", lastQSL)
	}
	if lastQSORX != "" {
		writeADIField("APP_LoTW_LASTQSORX", lastQSORX)
	}
	writeADIField("APP_LoTW_NUMREC", itoa(len(records)))
	b.WriteString("<EOH>\n")
	for _, record := range records {
		for name, value := range record {
			writeADIField(name, value)
		}
		b.WriteString("<EOR>\n")
	}
	// Bare tag, no ":length" suffix — matches live lotwreport.adi responses
	// (verified manually against the real endpoint), unlike every other
	// field here.
	b.WriteString("<APP_LoTW_EOF>")
	return b.String()
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	digits := ""
	for n > 0 {
		digits = string(rune('0'+n%10)) + digits
		n /= 10
	}
	return digits
}

func TestSyncLoTWConfirmationsMatchesLocalQSOAndAdvancesBookmark(t *testing.T) {
	st, err := openStore(t.TempDir() + "/logger.db")
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	q := qso{
		call:      "W1AW",
		band:      "20M",
		mode:      "CW",
		time:      time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC),
		timeOff:   time.Date(2026, 3, 1, 12, 1, 0, 0, time.UTC),
		profileID: 1,
	}
	qsoID, err := st.insertQSO(q)
	if err != nil {
		t.Fatal(err)
	}

	body := lotwReportFixture("2026-03-02 00:00:00", "2026-03-02 00:00:00", []map[string]string{
		{"CALL": "W1AW", "BAND": "20M", "MODE": "CW", "QSO_DATE": "20260301", "TIME_ON": "1205", "CREDIT_GRANTED": "DXCC"},
		{"CALL": "K1ZZ", "BAND": "40M", "MODE": "CW", "QSO_DATE": "20260301", "TIME_ON": "1300"},
	})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("login") != "myuser" || r.URL.Query().Get("password") != "mypass" {
			t.Errorf("unexpected credentials in query: %s", r.URL.RawQuery)
		}
		w.Write([]byte(body))
	}))
	defer srv.Close()
	old := lotwReportURL
	lotwReportURL = srv.URL
	defer func() { lotwReportURL = old }()

	summary, err := syncLoTWConfirmations(context.Background(), st, 1, "myuser", "mypass", "")
	if err != nil {
		t.Fatal(err)
	}
	if summary.Fetched != 2 {
		t.Fatalf("Fetched = %d, want 2", summary.Fetched)
	}
	if summary.Matched != 1 {
		t.Fatalf("Matched = %d, want 1", summary.Matched)
	}

	var gotQSOID int64
	if err := st.db.QueryRow(`SELECT qso_id FROM lotw_confirmation WHERE call = 'W1AW'`).Scan(&gotQSOID); err != nil {
		t.Fatal(err)
	}
	if gotQSOID != qsoID {
		t.Fatalf("matched qso_id = %d, want %d", gotQSOID, qsoID)
	}
	var unmatchedQSOID any
	if err := st.db.QueryRow(`SELECT qso_id FROM lotw_confirmation WHERE call = 'K1ZZ'`).Scan(&unmatchedQSOID); err != nil {
		t.Fatal(err)
	}
	if unmatchedQSOID != nil {
		t.Fatalf("unmatched confirmation qso_id = %v, want nil", unmatchedQSOID)
	}

	state, err := st.loadLoTWSyncState(1)
	if err != nil {
		t.Fatal(err)
	}
	if state.LastQSL != "2026-03-02 00:00:00" || state.LastQSORX != "2026-03-02 00:00:00" {
		t.Fatalf("sync state = %+v, want both bookmarks advanced", state)
	}
}

func TestSyncLoTWConfirmationsSendsIncrementalSinceOnSecondSync(t *testing.T) {
	st, err := openStore(t.TempDir() + "/logger.db")
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	var lastQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		lastQuery = r.URL.RawQuery
		w.Write([]byte(lotwReportFixture("2026-03-02 00:00:00", "", nil)))
	}))
	defer srv.Close()
	old := lotwReportURL
	lotwReportURL = srv.URL
	defer func() { lotwReportURL = old }()

	if _, err := syncLoTWConfirmations(context.Background(), st, 1, "myuser", "mypass", ""); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(lastQuery, "qso_qslsince") {
		t.Fatalf("first sync query %q should not carry qso_qslsince yet", lastQuery)
	}

	if _, err := syncLoTWConfirmations(context.Background(), st, 1, "myuser", "mypass", ""); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(lastQuery, "qso_qslsince=2026-03-02") {
		t.Fatalf("second sync query %q should carry the bookmark from the first sync", lastQuery)
	}
}

// TestSyncLoTWConfirmationsErrorsOnMissingEOH guards against silently
// swallowing every record: bare ADIF permits an omitted header, but if
// lotwreport.adi ever returned a response with no <EOH> at all, the header
// scanner must fail loudly (see maxLoTWHeaderFields) instead of consuming
// every record as a discarded "header field" and reporting a clean zero.
func TestSyncLoTWConfirmationsErrorsOnMissingEOH(t *testing.T) {
	st, err := openStore(t.TempDir() + "/logger.db")
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	var body strings.Builder
	for i := 0; i < maxLoTWHeaderFields+5; i++ {
		body.WriteString("<CALL:4>W1AW<BAND:3>20M<MODE:2>CW<QSO_DATE:8>20260301<TIME_ON:6>120000<EOR>\n")
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(body.String()))
	}))
	defer srv.Close()
	old := lotwReportURL
	lotwReportURL = srv.URL
	defer func() { lotwReportURL = old }()

	if _, err := syncLoTWConfirmations(context.Background(), st, 1, "myuser", "mypass", ""); err == nil {
		t.Fatal("expected an error for a response with no <EOH>, got nil")
	}
}

// TestSyncLoTWConfirmationsErrorsOnHTMLAuthFailure guards the exact case
// ARRL's docs describe: "If the query fails, an HTML page containing an
// explanation will be returned; the absence of an ADIF end of header tag can
// be used to detect this outcome." A short HTML error page (unlike the
// synthetic all-EOR-records body in the MissingEOH test above) must still be
// rejected rather than parsed as "zero confirmations, all good".
func TestSyncLoTWConfirmationsErrorsOnHTMLAuthFailure(t *testing.T) {
	st, err := openStore(t.TempDir() + "/logger.db")
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte("<html><head><title>Error</title></head><body>Invalid login or password</body></html>"))
	}))
	defer srv.Close()
	old := lotwReportURL
	lotwReportURL = srv.URL
	defer func() { lotwReportURL = old }()

	if _, err := syncLoTWConfirmations(context.Background(), st, 1, "myuser", "wrongpass", ""); err == nil {
		t.Fatal("expected an error for an HTML auth-failure response with no <EOH>, got nil")
	}
}

// TestSyncLoTWConfirmationsErrorsOnMissingEOFMarker guards against a
// response truncated in transit (e.g. a proxy or connection cut mid-stream)
// landing at a clean record boundary and being mistaken for a complete,
// successful sync: ARRL's documented APP_LoTW_EOF trailing marker exists
// specifically so a client can tell the two apart, and its absence must
// abort the sync with nothing committed.
func TestSyncLoTWConfirmationsErrorsOnMissingEOFMarker(t *testing.T) {
	st, err := openStore(t.TempDir() + "/logger.db")
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	// Built by hand (not lotwReportFixture) to omit the trailing APP_LoTW_EOF
	// field that a genuine complete response always carries.
	body := "LoTW QSO/QSL query response\n<APP_LoTW_NUMREC:1>1<EOH>\n" +
		"<CALL:4>W1AW<BAND:3>20M<MODE:2>CW<QSO_DATE:8>20260301<TIME_ON:6>120000<EOR>\n"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(body))
	}))
	defer srv.Close()
	old := lotwReportURL
	lotwReportURL = srv.URL
	defer func() { lotwReportURL = old }()

	if _, err := syncLoTWConfirmations(context.Background(), st, 1, "myuser", "mypass", ""); err == nil {
		t.Fatal("expected an error for a response missing the APP_LoTW_EOF marker, got nil")
	}
	var count int
	if err := st.db.QueryRow(`SELECT COUNT(*) FROM lotw_confirmation`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("lotw_confirmation rows = %d, want 0 (nothing committed for an incomplete response)", count)
	}
}

// TestSyncLoTWConfirmationsErrorsOnNumRecMismatch guards against a response
// that declares one record count in its header but delivers a different
// number of records — a case a naive implementation would happily accept and
// commit as if it were complete.
func TestSyncLoTWConfirmationsErrorsOnNumRecMismatch(t *testing.T) {
	st, err := openStore(t.TempDir() + "/logger.db")
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	body := "LoTW QSO/QSL query response\n<APP_LoTW_NUMREC:1>2<EOH>\n" +
		"<CALL:4>W1AW<BAND:3>20M<MODE:2>CW<QSO_DATE:8>20260301<TIME_ON:6>120000<EOR>\n" +
		"<APP_LoTW_EOF>"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(body))
	}))
	defer srv.Close()
	old := lotwReportURL
	lotwReportURL = srv.URL
	defer func() { lotwReportURL = old }()

	if _, err := syncLoTWConfirmations(context.Background(), st, 1, "myuser", "mypass", ""); err == nil {
		t.Fatal("expected an error for a NUMREC/records-received mismatch, got nil")
	}
	var count int
	if err := st.db.QueryRow(`SELECT COUNT(*) FROM lotw_confirmation`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("lotw_confirmation rows = %d, want 0 (nothing committed on a NUMREC mismatch)", count)
	}
}

// TestUpsertLoTWConfirmationMatchesDespiteModeMismatch guards ARRL's
// documented guidance that TQSL may remap an uploaded mode to a different
// value than what's logged locally, so mode must not gate the match when
// only one local QSO falls in the confirmation's time window.
func TestUpsertLoTWConfirmationMatchesDespiteModeMismatch(t *testing.T) {
	st, err := openStore(t.TempDir() + "/logger.db")
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	qsoID, err := st.insertQSO(qso{
		call: "W1AW", band: "20M", mode: "CW",
		time: time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC), timeOff: time.Date(2026, 3, 1, 12, 1, 0, 0, time.UTC),
		profileID: 1,
	})
	if err != nil {
		t.Fatal(err)
	}

	// LoTW reports a remapped mode ("RTTY") the operator logged as "CW".
	record := map[string]string{"CALL": "W1AW", "BAND": "20M", "MODE": "RTTY", "QSO_DATE": "20260301", "TIME_ON": "120000"}
	matched, err := st.upsertLoTWConfirmation(1, record, "2026-03-02T00:00:00Z")
	if err != nil {
		t.Fatal(err)
	}
	if !matched {
		t.Fatal("expected a match despite the mode mismatch (single candidate in the time window)")
	}
	var gotQSOID int64
	if err := st.db.QueryRow(`SELECT qso_id FROM lotw_confirmation WHERE call = 'W1AW'`).Scan(&gotQSOID); err != nil {
		t.Fatal(err)
	}
	if gotQSOID != qsoID {
		t.Fatalf("matched qso_id = %d, want %d", gotQSOID, qsoID)
	}
}

// TestUpsertLoTWConfirmationDisambiguatesByMode guards the other half of
// ARRL's guidance: mode should still be used to break a tie when multiple
// local QSOs fall within the confirmation's time window. w4gns-logger only
// ever logs CW contacts (validateQSO rejects any other mode), so the second
// QSO here is inserted directly via insertQSOInto, bypassing that
// restriction, purely to exercise matchLoTWConfirmation's general-purpose
// disambiguation logic against a mixed-mode candidate set.
func TestUpsertLoTWConfirmationDisambiguatesByMode(t *testing.T) {
	st, err := openStore(t.TempDir() + "/logger.db")
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	cwID, err := st.insertQSO(qso{
		call: "W1AW", band: "20M", mode: "CW",
		time: time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC), timeOff: time.Date(2026, 3, 1, 12, 1, 0, 0, time.UTC),
		profileID: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	tx, err := st.db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := insertQSOInto(tx, qso{
		call: "W1AW", band: "20M", mode: "SSB",
		time: time.Date(2026, 3, 1, 12, 5, 0, 0, time.UTC), timeOff: time.Date(2026, 3, 1, 12, 6, 0, 0, time.UTC),
		profileID: 1,
	}); err != nil {
		tx.Rollback()
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}

	record := map[string]string{"CALL": "W1AW", "BAND": "20M", "MODE": "CW", "QSO_DATE": "20260301", "TIME_ON": "120000"}
	matched, err := st.upsertLoTWConfirmation(1, record, "2026-03-02T00:00:00Z")
	if err != nil {
		t.Fatal(err)
	}
	if !matched {
		t.Fatal("expected a match")
	}
	var gotQSOID int64
	if err := st.db.QueryRow(`SELECT qso_id FROM lotw_confirmation WHERE call = 'W1AW'`).Scan(&gotQSOID); err != nil {
		t.Fatal(err)
	}
	if gotQSOID != cwID {
		t.Fatalf("matched qso_id = %d, want the CW QSO %d (mode disambiguates between two same-window candidates)", gotQSOID, cwID)
	}
}

// TestRedactLoTWCredentialsStripsLoginAndPassword guards against a network
// error (DNS, TLS, connection refused, timeout) leaking the operator's LoTW
// password: Go's net/url.Error.Error() embeds the full request URL, and
// buildLoTWReportURL puts both credentials in that URL's query string.
func TestRedactLoTWCredentialsStripsLoginAndPassword(t *testing.T) {
	err := fmt.Errorf(`Get "https://lotw.arrl.org/lotwuser/lotwreport.adi?login=myuser&password=hunter2": dial tcp: lookup failed`)
	redacted := redactLoTWCredentials(err, "myuser", "hunter2")
	if strings.Contains(redacted, "hunter2") {
		t.Fatalf("redacted error still contains the password: %q", redacted)
	}
	if strings.Contains(redacted, "myuser") {
		t.Fatalf("redacted error still contains the login: %q", redacted)
	}
}

func TestSyncLoTWConfirmationsRequiresCredentials(t *testing.T) {
	st, err := openStore(t.TempDir() + "/logger.db")
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	if _, err := syncLoTWConfirmations(context.Background(), st, 1, "", "", ""); err == nil {
		t.Fatal("expected an error for missing credentials")
	}
}

func TestUpsertLoTWConfirmationIsIdempotentAcrossOverlappingSyncs(t *testing.T) {
	st, err := openStore(t.TempDir() + "/logger.db")
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	record := map[string]string{"CALL": "W1AW", "BAND": "20M", "MODE": "CW", "QSO_DATE": "20260301", "TIME_ON": "120000"}
	if _, err := st.upsertLoTWConfirmation(1, record, "2026-03-02T00:00:00Z"); err != nil {
		t.Fatal(err)
	}
	if _, err := st.upsertLoTWConfirmation(1, record, "2026-03-03T00:00:00Z"); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := st.db.QueryRow(`SELECT COUNT(1) FROM lotw_confirmation WHERE call = 'W1AW'`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("row count = %d, want 1 (re-sync over an overlapping window must not duplicate)", count)
	}
}
