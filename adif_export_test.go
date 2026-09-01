package main

import (
	"context"
	"strings"
	"testing"
)

func fieldValue(fields []struct{ name, value string }, name string) (string, bool) {
	for _, field := range fields {
		if field.name == name {
			return field.value, true
		}
	}
	return "", false
}

func TestAdifQSOFieldsExportsIntegerSTXSRXOnly(t *testing.T) {
	q := validTestQSO()
	q.stx, q.srx = "12", "not-a-number"
	fields := adifQSOFields(q)
	if value, _ := fieldValue(fields, "STX"); value != "12" {
		t.Errorf("STX = %q, want %q", value, "12")
	}
	if value, _ := fieldValue(fields, "SRX"); value != "" {
		t.Errorf("SRX = %q, want empty for non-numeric input", value)
	}
}

func TestAdifQSOFieldsUsesIntlFieldForNonASCII(t *testing.T) {
	q := validTestQSO()
	q.name = "José"
	fields := adifQSOFields(q)
	if _, ok := fieldValue(fields, "NAME"); ok {
		t.Error("NAME should not be present for non-ASCII values")
	}
	if value, ok := fieldValue(fields, "NAME_INTL"); !ok || value != "José" {
		t.Errorf("NAME_INTL = %q, ok=%v, want %q", value, ok, "José")
	}
}

func TestAdifQSOFieldsUsesIntlFieldForNonASCIIStationFields(t *testing.T) {
	q := validTestQSO()
	q.operatorName, q.myRig, q.myAntenna = "José", "FTδ-891", "Δ-loop"
	fields := adifQSOFields(q)
	for _, tc := range []struct{ base, want string }{
		{"OPERATOR", "José"}, {"MY_RIG", "FTδ-891"}, {"MY_ANTENNA", "Δ-loop"},
	} {
		if _, ok := fieldValue(fields, tc.base); ok {
			t.Errorf("%s should not be present for non-ASCII values", tc.base)
		}
		if value, ok := fieldValue(fields, tc.base+"_INTL"); !ok || value != tc.want {
			t.Errorf("%s_INTL = %q, ok=%v, want %q", tc.base, value, ok, tc.want)
		}
	}
}

func TestAdifQSOFieldsUsesPlainFieldForASCII(t *testing.T) {
	q := validTestQSO()
	q.name = "Pat"
	fields := adifQSOFields(q)
	if value, ok := fieldValue(fields, "NAME"); !ok || value != "Pat" {
		t.Errorf("NAME = %q, ok=%v, want %q", value, ok, "Pat")
	}
	if _, ok := fieldValue(fields, "NAME_INTL"); ok {
		t.Error("NAME_INTL should not be present for ASCII values")
	}
}

func TestAdifQSOFieldsExportsPotaRefAlongsideSig(t *testing.T) {
	q := validTestQSO()
	q.potaRef = "US-1234"
	fields := adifQSOFields(q)
	if value, _ := fieldValue(fields, "POTA_REF"); value != "US-1234" {
		t.Errorf("POTA_REF = %q, want %q", value, "US-1234")
	}
	if value, _ := fieldValue(fields, "SIG"); value != "POTA" {
		t.Errorf("SIG = %q, want POTA", value)
	}
}

func TestAdifQSOFieldsExportsStationSnapshot(t *testing.T) {
	q := validTestQSO()
	q.myGridSquare, q.stationCallsign, q.operatorName = "FN31PR", "W4GNS", "Gary"
	fields := adifQSOFields(q)
	for name, want := range map[string]string{
		"MY_GRIDSQUARE":    "FN31PR",
		"STATION_CALLSIGN": "W4GNS",
		"OPERATOR":         "Gary",
	} {
		if value, _ := fieldValue(fields, name); value != want {
			t.Errorf("%s = %q, want %q", name, value, want)
		}
	}
}

func TestAdifContestIDMapsCWTAndCWOpenToStandardIDs(t *testing.T) {
	cases := map[string]string{
		"CWT-1900":      "CWOPS-CWT",
		"CWT-0300":      "CWOPS-CWT",
		"CW-OPEN-1":     "CWOPS-CW-OPEN",
		"CW-OPEN-3":     "CWOPS-CW-OPEN",
		"ARRL-DX-CW":    "ARRL-DX-CW",
		"TNQP-DAVIDSON": "TNQP-DAVIDSON",
	}
	for internal, want := range cases {
		if got := adifContestID(internal); got != want {
			t.Errorf("adifContestID(%q) = %q, want %q", internal, got, want)
		}
	}
}

func TestExportADIFDeclaresCurrentADIFVersion(t *testing.T) {
	st, err := openStore(t.TempDir() + "/logger.db")
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	var buf strings.Builder
	if _, err := exportADIF(context.Background(), &buf, 1, st); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "<ADIF_VER:5>3.1.7") {
		t.Fatalf("export header = %q, want ADIF_VER 3.1.7", buf.String())
	}
}
