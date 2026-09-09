package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// lotwReportFixture builds a minimal lotwreport.adi response: a header
// carrying APP_LoTW_LASTQSL/LASTQSORX followed by the given records.
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
	b.WriteString("<EOH>\n")
	for _, record := range records {
		for name, value := range record {
			writeADIField(name, value)
		}
		b.WriteString("<EOR>\n")
	}
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
