package main

import (
	"bytes"
	"context"
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
	if count != 1 || !strings.Contains(adif.String(), "<NAME:5>José") {
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
	err = destination.db.QueryRow(`SELECT call, band, freq, mode, rst_sent, rst_rcvd, name, qth, gridsquare, state, sig_info, comment, contest_id, stx, stx_string, srx, srx_string, qso_date, time_on, qso_date_off, time_off FROM qso`).Scan(
		&got.call, &got.band, &got.frequency, &got.mode, &got.rstSent, &got.rstRcvd, &got.name, &got.qth, &got.grid, &got.state, &got.potaRef, &got.comment, &got.contestID, &got.stx, &got.stxString, &got.srx, &got.srxString, &date, &timeOn, &dateOff, &timeOff,
	)
	if err != nil {
		t.Fatal(err)
	}
	got.time, _ = time.Parse("20060102150405", date+timeOn)
	got.timeOff, _ = time.Parse("20060102150405", dateOff+timeOff)
	got.profileID = destinationProfile.ID
	if got != q {
		t.Fatalf("round-trip QSO = %#v, want %#v", got, q)
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
