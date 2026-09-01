package main

import (
	"bytes"
	"context"
	"database/sql"
	"fmt"
	"strings"
	"testing"
	"time"
)

func TestImportADIFImportsCWAndSkipsOtherModes(t *testing.T) {
	st, err := openStore(t.TempDir() + "/logger.db")
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	profile, err := st.activeStationProfile()
	if err != nil {
		t.Fatal(err)
	}
	adi := `<ADIF_VER:5>3.1.7<EOH><CALL:3>W1A<QSO_DATE:8>20260831<TIME_ON:6>120000<QSO_DATE_OFF:8>20260831<TIME_OFF:6>120030<BAND:3>20M<MODE:2>CW<RST_SENT:3>599<RST_RCVD:3>599<EOR><CALL:3>K1A<QSO_DATE:8>20260831<TIME_ON:6>120000<BAND:3>20M<MODE:3>FT8<EOR>`
	result, err := importADIF(context.Background(), strings.NewReader(adi), profile.ID, st)
	if err != nil {
		t.Fatalf("importADIF: %v", err)
	}
	if result.Imported != 1 || result.Skipped != 1 {
		t.Fatalf("result = %#v", result)
	}
	if count, _ := st.count(); count != 1 {
		t.Fatalf("count = %d", count)
	}
}

func TestExportADIFRoundTripPreservesQSOFields(t *testing.T) {
	source, err := openStore(t.TempDir() + "/source.db")
	if err != nil {
		t.Fatal(err)
	}
	defer source.Close()
	profile, err := source.activeStationProfile()
	if err != nil {
		t.Fatal(err)
	}
	q := qso{
		call: "W1AW", band: "20M", frequency: "14.025", mode: "CW", rstSent: "599", rstRcvd: "579",
		name: "José", qth: "Newington", grid: "FN31", state: "CT", potaRef: "K-0001", comment: "test contact",
		contestID: "ARRL-DX-CW", stx: "12", stxString: "CT", srx: "34", srxString: "MA",
		time: time.Date(2026, 8, 31, 12, 0, 0, 0, time.UTC), timeOff: time.Date(2026, 8, 31, 12, 1, 30, 0, time.UTC), profileID: profile.ID,
	}
	if _, err := source.insertQSO(q); err != nil {
		t.Fatalf("insertQSO: %v", err)
	}
	var adif bytes.Buffer
	count, err := exportADIF(context.Background(), &adif, profile.ID, source)
	if err != nil {
		t.Fatalf("exportADIF: %v", err)
	}
	// "José" is non-ASCII; ADIF 3.1.7 restricts IntlString "_INTL" fields to
	// ADX/XML files, so ADI-compliant output must transliterate it into the
	// ASCII-only NAME field ("Jose") instead of emitting NAME_INTL.
	if count != 1 || !strings.Contains(adif.String(), "<NAME:4>Jose") || strings.Contains(adif.String(), "NAME_INTL") {
		t.Fatalf("export = %q, count = %d", adif.String(), count)
	}

	destination, err := openStore(t.TempDir() + "/destination.db")
	if err != nil {
		t.Fatal(err)
	}
	defer destination.Close()
	destinationProfile, err := destination.activeStationProfile()
	if err != nil {
		t.Fatal(err)
	}
	result, err := importADIF(context.Background(), bytes.NewReader(adif.Bytes()), destinationProfile.ID, destination)
	if err != nil {
		t.Fatalf("importADIF: %v", err)
	}
	if result.Imported != 1 || result.Skipped != 0 {
		t.Fatalf("round-trip result = %#v", result)
	}
	var got qso
	var date, timeOn, dateOff, timeOff string
	var dxcc sql.NullString
	err = destination.db.QueryRow(`SELECT call, band, freq, mode, rst_sent, rst_rcvd, name, qth, gridsquare, state, CAST(dxcc AS TEXT), sig_info, comment, contest_id, stx, stx_string, srx, srx_string, qso_date, time_on, qso_date_off, time_off FROM qso`).Scan(
		&got.call, &got.band, &got.frequency, &got.mode, &got.rstSent, &got.rstRcvd, &got.name, &got.qth, &got.grid, &got.state, &dxcc, &got.potaRef, &got.comment, &got.contestID, &got.stx, &got.stxString, &got.srx, &got.srxString, &date, &timeOn, &dateOff, &timeOff,
	)
	if err != nil {
		t.Fatal(err)
	}
	got.dxccNumber = dxcc.String
	got.time, _ = time.Parse("20060102150405", date+timeOn)
	got.timeOff, _ = time.Parse("20060102150405", dateOff+timeOff)
	got.profileID = destinationProfile.ID
	// name is expected to come back transliterated ("Jose", not "José"): ADI
	// export cannot round-trip non-ASCII losslessly under the ADIF 3.1.7
	// ASCII-only String rule, so this is intentional lossy behavior, not a bug.
	// dxccNumber is expected to come back populated (291, United States) even
	// though the source q didn't set it explicitly: insertQSO resolves it
	// automatically from the callsign, and that resolved value round-trips
	// through the DXCC ADIF field on export/import.
	want := q
	want.name = "Jose"
	want.dxccNumber = "291"
	if got != want {
		t.Fatalf("round-trip QSO = %#v, want %#v", got, want)
	}
}

// TestImportADIFRejectsMalformedFiveCharTime guards against a regression of a
// panic: a 5-character TIME_ON (invalid ADIF Time, which must be HHMM or
// HHMMSS) used to reach a fixed 6-byte slice and panic with
// slice-bounds-out-of-range instead of being skipped as an unparsable record.
func TestImportADIFRejectsMalformedFiveCharTime(t *testing.T) {
	st, err := openStore(t.TempDir() + "/logger.db")
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	profile, err := st.activeStationProfile()
	if err != nil {
		t.Fatal(err)
	}
	adi := `<CALL:3>W1A<QSO_DATE:8>20260831<TIME_ON:5>12003<BAND:3>20M<MODE:2>CW<EOR>`
	result, err := importADIF(context.Background(), strings.NewReader(adi), profile.ID, st)
	if err != nil {
		t.Fatalf("importADIF returned error instead of skipping: %v", err)
	}
	if result.Imported != 0 || result.Skipped != 1 {
		t.Fatalf("result = %#v, want 0 imported / 1 skipped", result)
	}
}

// TestImportADIFStreamsAcrossMultipleBatches exercises the streaming
// parser/insert path across several importBatchSize-sized batches (rather
// than one giant in-memory slice of every parsed record) and confirms every
// record still lands correctly.
func TestImportADIFStreamsAcrossMultipleBatches(t *testing.T) {
	st, err := openStore(t.TempDir() + "/logger.db")
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	profile, err := st.activeStationProfile()
	if err != nil {
		t.Fatal(err)
	}

	const total = importBatchSize*3 + 250
	var b strings.Builder
	b.WriteString("<ADIF_VER:5>3.1.7<EOH>")
	for i := 0; i < total; i++ {
		call := fmt.Sprintf("W%dTEST", i)
		fmt.Fprintf(&b, "<CALL:%d>%s<QSO_DATE:8>20260831<TIME_ON:6>120000<BAND:3>20M<MODE:2>CW<EOR>", len(call), call)
	}
	result, err := importADIF(context.Background(), strings.NewReader(b.String()), profile.ID, st)
	if err != nil {
		t.Fatalf("importADIF: %v", err)
	}
	if result.Imported != total || result.Skipped != 0 {
		t.Fatalf("result = %#v, want %d imported / 0 skipped", result, total)
	}
	count, err := st.count()
	if err != nil {
		t.Fatal(err)
	}
	if count != total {
		t.Fatalf("stored count = %d, want %d", count, total)
	}
}

// TestImportADIFStopsOnErrorButKeepsPriorBatches documents the actual
// partial-import behavior: a parse error partway through a file still leaves
// the batches inserted before the bad record committed, so a fixed re-import
// doesn't lose already-imported QSOs.
func TestImportADIFStopsOnErrorButKeepsPriorBatches(t *testing.T) {
	st, err := openStore(t.TempDir() + "/logger.db")
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	profile, err := st.activeStationProfile()
	if err != nil {
		t.Fatal(err)
	}

	var b strings.Builder
	for i := 0; i < importBatchSize+5; i++ {
		call := fmt.Sprintf("W%dTEST", i)
		fmt.Fprintf(&b, "<CALL:%d>%s<QSO_DATE:8>20260831<TIME_ON:6>120000<BAND:3>20M<MODE:2>CW<EOR>", len(call), call)
	}
	b.WriteString("<BADFIELD:notanumber>x<EOR>")

	result, err := importADIF(context.Background(), strings.NewReader(b.String()), profile.ID, st)
	if err == nil {
		t.Fatal("expected a parse error from the malformed trailing record")
	}
	if result.Imported != importBatchSize {
		t.Fatalf("Imported = %d, want the first full batch (%d) preserved", result.Imported, importBatchSize)
	}
	count, err := st.count()
	if err != nil {
		t.Fatal(err)
	}
	if count != importBatchSize {
		t.Fatalf("stored count = %d, want %d", count, importBatchSize)
	}
}

func TestImportADIFAcceptsFileWithoutHeader(t *testing.T) {
	st, err := openStore(t.TempDir() + "/logger.db")
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	profile, err := st.activeStationProfile()
	if err != nil {
		t.Fatal(err)
	}
	adi := `<CALL:3>W1A<QSO_DATE:8>20260831<TIME_ON:6>120000<BAND:3>20M<MODE:2>CW<EOR>`
	result, err := importADIF(context.Background(), strings.NewReader(adi), profile.ID, st)
	if err != nil {
		t.Fatalf("importADIF: %v", err)
	}
	if result.Imported != 1 {
		t.Fatalf("result = %#v", result)
	}
}
