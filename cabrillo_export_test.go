package main

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
)

func testStationProfile() stationProfile {
	return stationProfile{
		ID:               1,
		Callsign:         "W4GNS",
		OperatorName:     "Gary",
		Club:             "Test Radio Club",
		Address:          "123 Main St",
		CategoryOperator: "SINGLE-OP",
		CategoryAssisted: "NON-ASSISTED",
		CategoryPower:    "LOW",
	}
}

func testEventDefinition() eventDefinition {
	return eventDefinition{
		ID:    "CQ-WPX-CW",
		Name:  "CQ WW WPX Contest, CW",
		Bands: []string{"160M", "80M", "40M", "20M", "15M", "10M"},
	}
}

// TestSanitizeFilenameComponentBlocksPathTraversal guards the export
// filename against the Contest field's free-text input: an operator can
// type or paste anything into it, and a value like "CWT-../../../tmp/evil"
// still starts with a real catalog event ID ("CWT-"), so eventForContestID
// accepts it — the export path must not then let it escape the Downloads
// folder via filepath.Join's path-cleaning.
func TestSanitizeFilenameComponentBlocksPathTraversal(t *testing.T) {
	downloads := "/home/op/Downloads"
	contestID := "CWT-../../../tmp/evil"
	path := filepath.Join(downloads, sanitizeFilenameComponent("W4GNS")+"_"+sanitizeFilenameComponent(contestID)+".cbr")
	if !strings.HasPrefix(path, downloads+string(filepath.Separator)) {
		t.Fatalf("sanitized path %q escaped the Downloads folder %q", path, downloads)
	}
	if strings.ContainsAny(sanitizeFilenameComponent(contestID), "/.") {
		t.Fatalf("sanitizeFilenameComponent(%q) = %q, still contains a path separator or dot", contestID, sanitizeFilenameComponent(contestID))
	}
}

func TestSanitizeFilenameComponentFallsBackWhenFullyStripped(t *testing.T) {
	if got := sanitizeFilenameComponent("../.."); got == "" {
		t.Fatal("sanitizeFilenameComponent returned empty for an all-unsafe input")
	}
}

func TestCabrilloHeaderLinesUsesProfileAndEventFields(t *testing.T) {
	lines := cabrilloHeaderLines(testStationProfile(), testEventDefinition())
	joined := strings.Join(lines, "\n")
	for _, want := range []string{
		"START-OF-LOG: 3.0",
		"CONTEST: CQ-WPX-CW",
		"CALLSIGN: W4GNS",
		"CATEGORY-OPERATOR: SINGLE-OP",
		"CATEGORY-ASSISTED: NON-ASSISTED",
		"CATEGORY-BAND: ALL",
		"CATEGORY-POWER: LOW",
		"CATEGORY-MODE: CW",
		"CLUB: Test Radio Club",
		"NAME: Gary",
		"ADDRESS: 123 Main St",
		"OPERATORS: W4GNS",
	} {
		if !strings.Contains(joined, want) {
			t.Errorf("header missing %q, got:\n%s", want, joined)
		}
	}
}

// TestCabrilloHeaderLinesFallsBackToDefaultCategories guards against an
// invalid (blank) Cabrillo header when the operator never filled in the
// category fields on Station Setup.
func TestCabrilloHeaderLinesFallsBackToDefaultCategories(t *testing.T) {
	profile := testStationProfile()
	profile.CategoryOperator, profile.CategoryAssisted, profile.CategoryPower = "", "", ""
	lines := cabrilloHeaderLines(profile, testEventDefinition())
	joined := strings.Join(lines, "\n")
	for _, want := range []string{"CATEGORY-OPERATOR: SINGLE-OP", "CATEGORY-ASSISTED: NON-ASSISTED", "CATEGORY-POWER: LOW"} {
		if !strings.Contains(joined, want) {
			t.Errorf("header missing default %q, got:\n%s", want, joined)
		}
	}
}

func TestCabrilloCategoryBandSingleBandVsAll(t *testing.T) {
	if got := cabrilloCategoryBand([]string{"20M"}); got != "20M" {
		t.Errorf("cabrilloCategoryBand single band = %q, want 20M", got)
	}
	if got := cabrilloCategoryBand([]string{"160M", "80M", "40M"}); got != "ALL" {
		t.Errorf("cabrilloCategoryBand multi-band = %q, want ALL", got)
	}
}

func TestCabrilloQSOLineFormatsExpectedFields(t *testing.T) {
	q := validTestQSO()
	q.frequency = "14.025"
	q.rstSent, q.rstRcvd = "599", "579"
	q.stx, q.stxString = "001", "CA"
	q.srx, q.srxString = "042", "TX"
	q.stationCallsign = "W4GNS"

	line, err := cabrilloQSOLine(q, testStationProfile())
	if err != nil {
		t.Fatalf("cabrilloQSOLine: %v", err)
	}
	if !strings.HasPrefix(line, "QSO: 14025 CW 2026-08-31 1200 ") {
		t.Fatalf("cabrilloQSOLine = %q, unexpected prefix", line)
	}
	for _, want := range []string{"W4GNS", "599", "001 CA", "W1AW", "579", "042 TX"} {
		if !strings.Contains(line, want) {
			t.Errorf("cabrilloQSOLine missing %q, got %q", want, line)
		}
	}
}

// TestCabrilloQSOLineFallsBackToBandDefaultFrequency mirrors
// uploadQSOToWRL's fallback: frequency is optional for local logging but
// Cabrillo's QSO: line always needs a numeric frequency.
func TestCabrilloQSOLineFallsBackToBandDefaultFrequency(t *testing.T) {
	q := validTestQSO() // band 20M, blank frequency
	line, err := cabrilloQSOLine(q, testStationProfile())
	if err != nil {
		t.Fatalf("cabrilloQSOLine: %v", err)
	}
	if !strings.HasPrefix(line, "QSO: 14025 CW") {
		t.Fatalf("cabrilloQSOLine = %q, want the 20M band default frequency 14025 kHz", line)
	}
}

func TestCabrilloQSOLineFallsBackToProfileCallsignWhenStationSnapshotBlank(t *testing.T) {
	q := validTestQSO()
	q.frequency = "14.025"
	q.stationCallsign = ""
	line, err := cabrilloQSOLine(q, testStationProfile())
	if err != nil {
		t.Fatalf("cabrilloQSOLine: %v", err)
	}
	if !strings.Contains(line, "W4GNS") {
		t.Fatalf("cabrilloQSOLine = %q, want the profile callsign W4GNS as a fallback", line)
	}
}

func TestCabrilloExchangeJoinsSerialAndText(t *testing.T) {
	cases := []struct{ serial, text, want string }{
		{"001", "CA", "001 CA"},
		{"001", "", "001"},
		{"", "CA", "CA"},
		{"", "", ""},
	}
	for _, tc := range cases {
		if got := cabrilloExchange(tc.serial, tc.text); got != tc.want {
			t.Errorf("cabrilloExchange(%q, %q) = %q, want %q", tc.serial, tc.text, got, tc.want)
		}
	}
}

// TestExportCabrilloWritesHeaderFooterAndOnlyMatchingContestQSOs guards the
// database-scoping half of the feature: a profile logging QSOs in more than
// one contest must only export the one requested, and non-contest QSOs
// (blank contest_id) must never leak into a submission.
func TestExportCabrilloWritesHeaderFooterAndOnlyMatchingContestQSOs(t *testing.T) {
	st, err := openStore(t.TempDir() + "/logger.db")
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	profile, err := st.activeStationProfile()
	if err != nil {
		t.Fatal(err)
	}
	profile.Callsign = "W4GNS"

	for _, q := range []qso{
		func() qso {
			q := validTestQSO()
			q.call, q.contestID, q.profileID = "W1AW", "CQ-WPX-CW", profile.ID
			return q
		}(),
		func() qso {
			q := validTestQSO()
			q.call, q.contestID, q.profileID = "K1ABC", "CQ-WPX-CW", profile.ID
			return q
		}(),
		func() qso {
			q := validTestQSO()
			q.call, q.contestID, q.profileID = "N1MM", "OTHER-CONTEST", profile.ID
			return q
		}(),
		func() qso { q := validTestQSO(); q.call, q.profileID = "W9XYZ", profile.ID; return q }(), // not a contest QSO
	} {
		if _, err := st.insertQSO(q); err != nil {
			t.Fatal(err)
		}
	}

	var buf strings.Builder
	count, err := exportCabrillo(context.Background(), &buf, profile, testEventDefinition(), "CQ-WPX-CW", st)
	if err != nil {
		t.Fatalf("exportCabrillo: %v", err)
	}
	if count != 2 {
		t.Fatalf("exported %d QSOs, want 2", count)
	}
	out := buf.String()
	if !strings.HasPrefix(out, "START-OF-LOG: 3.0\r\n") {
		t.Fatalf("output does not start with the Cabrillo header: %q", out[:min(40, len(out))])
	}
	if !strings.HasSuffix(out, "END-OF-LOG:\r\n") {
		t.Fatalf("output does not end with END-OF-LOG:, got: %q", out[max(0, len(out)-40):])
	}
	if !strings.Contains(out, "W1AW") || !strings.Contains(out, "K1ABC") {
		t.Fatalf("output missing expected contest QSOs: %q", out)
	}
	if strings.Contains(out, "N1MM") || strings.Contains(out, "W9XYZ") {
		t.Fatalf("output leaked QSOs from a different contest or a non-contest QSO: %q", out)
	}
}
