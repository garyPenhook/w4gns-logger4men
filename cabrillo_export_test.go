package main

import (
	"bytes"
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
		ID:             "CQ-WPX-CW",
		Name:           "CQ WW WPX Contest, CW",
		Bands:          []string{"160M", "80M", "40M", "20M", "15M", "10M"},
		CabrilloLayout: "cw_rst_exchange",
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
	lines := cabrilloHeaderLines(testStationProfile(), testEventDefinition(), 0)
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
	lines := cabrilloHeaderLines(profile, testEventDefinition(), 0)
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

	line, err := cabrilloQSOLine(q, testStationProfile(), testEventDefinition())
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

// TestExportCabrilloRejectsUnverifiedEventEvenWithNoQSOs makes the catalog
// gate apply to every export, not only a log with a QSO (where the per-line
// formatter would eventually reject it). An empty unverified event must not
// produce a deceptively valid-looking header-only submission.
func TestExportCabrilloRejectsUnverifiedEventEvenWithNoQSOs(t *testing.T) {
	st, err := openStore(t.TempDir() + "/logger.db")
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	profile, err := st.activeStationProfile()
	if err != nil {
		t.Fatal(err)
	}
	_, _, err = exportCabrillo(context.Background(), &bytes.Buffer{}, profile, eventDefinition{ID: "UNVERIFIED"}, "UNVERIFIED", st)
	if err == nil || !strings.Contains(err.Error(), "no verified Cabrillo") {
		t.Fatalf("exportCabrillo unverified event error = %v, want verification failure", err)
	}
}

// TestCabrilloQSOLineOmitsRSTForCWOpen guards the CW Open Cabrillo format,
// whose exchange is a serial number plus name and carries no signal report:
// the QSO: line must not emit the RST columns (which default to "599" in the
// logger), because an extra field there misaligns the exchange the sponsor's
// log checker parses by position.
func TestCabrilloQSOLineOmitsRSTForCWOpen(t *testing.T) {
	q := validTestQSO()
	q.frequency = "14.025"
	q.rstSent, q.rstRcvd = "599", "599"
	q.stx, q.stxString = "001", "Bud"
	q.srx, q.srxString = "042", "Joe"
	q.stationCallsign = "W4GNS"

	event := eventDefinition{ID: "CW-OPEN", Name: "CW Open", CabrilloOmitRST: true, CabrilloLayout: "cw_exchange_only"}
	line, err := cabrilloQSOLine(q, testStationProfile(), event)
	if err != nil {
		t.Fatalf("cabrilloQSOLine: %v", err)
	}
	// Whitespace-collapsed tokens must be exactly the CW Open layout:
	// freq mode date time sentcall sentserial sentname rcvdcall rcvdserial rcvdname.
	got := strings.Join(strings.Fields(line), " ")
	want := "QSO: 14025 CW 2026-08-31 1200 W4GNS 001 Bud W1AW 042 Joe"
	if got != want {
		t.Fatalf("cabrilloQSOLine (CW Open) = %q, want %q", got, want)
	}
	if strings.Contains(got, "599") {
		t.Fatalf("cabrilloQSOLine (CW Open) leaked an RST report: %q", line)
	}
}

// TestCabrilloQSOLineRejectsLineInjectionAndOversizedFields guards the
// fixed-column QSO line against imported data that carries CR/LF (which would
// split or forge a line) or an over-long value (which would shift every
// following column).
func TestCabrilloQSOLineRejectsLineInjectionAndOversizedFields(t *testing.T) {
	q := validTestQSO()
	q.frequency = "14.025"
	q.stationCallsign = "W4GNS"
	q.call = "W1AW\r\nQSO: 14025 CW 2026-08-31 1200 FORGED"
	q.srxString = strings.Repeat("X", 100)

	line, err := cabrilloQSOLine(q, testStationProfile(), testEventDefinition())
	if err != nil {
		t.Fatalf("cabrilloQSOLine: %v", err)
	}
	if strings.ContainsAny(line, "\r\n") {
		t.Fatalf("cabrilloQSOLine leaked a line break: %q", line)
	}
	if strings.Contains(line, "FORGED") {
		t.Fatalf("cabrilloQSOLine let a forged line through: %q", line)
	}
	// The received exchange column is 13 wide; an over-long value is truncated.
	if strings.Contains(line, strings.Repeat("X", 14)) {
		t.Fatalf("cabrilloQSOLine did not enforce the field width: %q", line)
	}
}

// TestCabrilloHeaderValueStripsLineBreaks guards header values (free-text
// profile fields) against the same line-injection vector.
func TestCabrilloHeaderValueStripsLineBreaks(t *testing.T) {
	profile := testStationProfile()
	profile.Club = "Test Club\r\nX-QSO: forged"
	lines := cabrilloHeaderLines(profile, testEventDefinition(), 0)
	for _, line := range lines {
		if strings.ContainsAny(line, "\r\n") {
			t.Fatalf("header line leaked a break: %q", line)
		}
	}
	joined := strings.Join(lines, "|")
	if strings.Contains(joined, "forged") == false {
		// The text is kept, just flattened onto one line — sanity that we
		// didn't drop the content entirely.
		t.Fatalf("expected sanitized club text to survive on one line: %v", lines)
	}
}

// TestCabrilloQSOLineFallsBackToBandDefaultFrequency mirrors
// uploadQSOToWRL's fallback: frequency is optional for local logging but
// Cabrillo's QSO: line always needs a numeric frequency.
func TestCabrilloQSOLineFallsBackToBandDefaultFrequency(t *testing.T) {
	q := validTestQSO() // band 20M, blank frequency
	line, err := cabrilloQSOLine(q, testStationProfile(), testEventDefinition())
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
	line, err := cabrilloQSOLine(q, testStationProfile(), testEventDefinition())
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

func cwOpenScoringEvent() eventDefinition {
	return eventDefinition{
		ID:              "CW-OPEN",
		Name:            "CW Open",
		Bands:           []string{"160M", "80M", "40M", "20M", "15M", "10M"},
		CabrilloOmitRST: true,
		CabrilloLayout:  "cw_exchange_only",
		Scoring:         &scoringRules{PointsPerQSO: 1, Multiplier: "unique_call"},
	}
}

// TestComputeContestScoreCWOpen exercises the CW Open score formula: one point
// per unique (callsign, band) worked in the session, times the number of unique
// callsigns. A same-band duplicate must add no point but still not remove the
// callsign's multiplier, and a QSO on a second band must add a point without a
// second multiplier.
func TestComputeContestScoreCWOpen(t *testing.T) {
	st, err := openStore(t.TempDir() + "/logger.db")
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	profile, err := st.activeStationProfile()
	if err != nil {
		t.Fatal(err)
	}

	mk := func(call, band string) qso {
		q := validTestQSO()
		q.call, q.band, q.contestID, q.profileID = call, band, "CW-OPEN-1", profile.ID
		return q
	}
	for _, q := range []qso{
		mk("W1AW", "20M"),  // point + mult
		mk("W1AW", "40M"),  // point (new band), no new mult
		mk("K1ABC", "20M"), // point + mult
		mk("W1AW", "20M"),  // same-band dupe: no point, no new mult
	} {
		if _, err := st.insertQSO(q); err != nil {
			t.Fatal(err)
		}
	}

	score, err := computeContestScore(context.Background(), profile, cwOpenScoringEvent(), "CW-OPEN-1", st)
	if err != nil {
		t.Fatalf("computeContestScore: %v", err)
	}
	if score.qsoPoints != 3 || score.multipliers != 2 || score.total() != 6 {
		t.Fatalf("score = %d pts x %d mults = %d, want 3 x 2 = 6", score.qsoPoints, score.multipliers, score.total())
	}
}

// TestComputeContestScoreNoRulesIsZero guards the default: an event without a
// scoring rule scores zero, so its Cabrillo header keeps CLAIMED-SCORE: 0.
func TestComputeContestScoreNoRulesIsZero(t *testing.T) {
	st, err := openStore(t.TempDir() + "/logger.db")
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	profile, err := st.activeStationProfile()
	if err != nil {
		t.Fatal(err)
	}
	q := validTestQSO()
	q.call, q.contestID, q.profileID = "W1AW", "CQ-WPX-CW", profile.ID
	if _, err := st.insertQSO(q); err != nil {
		t.Fatal(err)
	}
	score, err := computeContestScore(context.Background(), profile, testEventDefinition(), "CQ-WPX-CW", st)
	if err != nil {
		t.Fatalf("computeContestScore: %v", err)
	}
	if score.total() != 0 {
		t.Fatalf("score.total() = %d, want 0 for an event with no scoring rule", score.total())
	}
}

// TestExportCabrilloWritesComputedClaimedScore ties the scorer to the header:
// a CW Open session log must carry the computed CLAIMED-SCORE, not 0.
func TestExportCabrilloWritesComputedClaimedScore(t *testing.T) {
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

	mk := func(call, band string) qso {
		q := validTestQSO()
		q.call, q.band, q.contestID, q.profileID = call, band, "CW-OPEN-1", profile.ID
		return q
	}
	for _, q := range []qso{mk("W1AW", "20M"), mk("K1ABC", "20M")} { // 2 pts x 2 mults = 4
		if _, err := st.insertQSO(q); err != nil {
			t.Fatal(err)
		}
	}

	var buf strings.Builder
	count, score, err := exportCabrillo(context.Background(), &buf, profile, cwOpenScoringEvent(), "CW-OPEN-1", st)
	if err != nil {
		t.Fatalf("exportCabrillo: %v", err)
	}
	if count != 2 || score.total() != 4 {
		t.Fatalf("exported %d QSOs, score %d; want 2 QSOs, score 4", count, score.total())
	}
	if !strings.Contains(buf.String(), "CLAIMED-SCORE: 4\r\n") {
		t.Fatalf("Cabrillo output missing computed CLAIMED-SCORE: 4, got:\n%s", buf.String())
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
	count, _, err := exportCabrillo(context.Background(), &buf, profile, testEventDefinition(), "CQ-WPX-CW", st)
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
