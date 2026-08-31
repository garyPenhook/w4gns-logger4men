package main

import (
	"context"
	"strings"
	"testing"
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
