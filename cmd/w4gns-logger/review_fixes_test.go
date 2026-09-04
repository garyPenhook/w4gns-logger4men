package main

import (
	"errors"
	"net/url"
	"strings"
	"testing"
	"time"
)

// TestParseADIFLengthIsDecimal guards against fmt.Sscan's Go-literal rules:
// an ADIF length prefix is plain base-10, so a zero-padded "010" must be 10
// (ten bytes), not octal 8, and a "0x10" must be rejected outright rather than
// read as hex 16. Either misparse silently reads the wrong number of bytes and
// desyncs the rest of the record.
func TestParseADIFLengthIsDecimal(t *testing.T) {
	for _, tt := range []struct {
		in      string
		want    int
		wantErr bool
	}{
		{"10", 10, false},
		{"010", 10, false}, // decimal ten, not octal eight
		{"0", 0, false},
		{"0x10", 0, true},
		{"4abc", 0, true},
		{"-1", 0, true},
		{"", 0, true},
	} {
		got, err := parseADIFLength(tt.in)
		if tt.wantErr {
			if err == nil {
				t.Errorf("parseADIFLength(%q) = %d, want error", tt.in, got)
			}
			continue
		}
		if err != nil || got != tt.want {
			t.Errorf("parseADIFLength(%q) = %d, %v; want %d, nil", tt.in, got, err, tt.want)
		}
	}
}

// TestValidateQSOAcceptsValidGrid confirms the new grid check doesn't reject a
// well-formed locator (the negative cases live in TestValidateQSORejectsInvalidInput).
func TestValidateQSOAcceptsValidGrid(t *testing.T) {
	q := validTestQSO()
	q.grid = "FN31pr"
	if err := validateQSO(q); err != nil {
		t.Fatalf("validateQSO with valid grid returned error: %v", err)
	}
	q.grid = ""
	if err := validateQSO(q); err != nil {
		t.Fatalf("validateQSO with empty grid returned error: %v", err)
	}
}

// TestDeleteQSORemovesPendingOutboxRows guards the fix for orphaned outbox
// rows: deleting a QSO with a pending upload must also remove its outbox
// entry, or the drain keeps trying to deliver a QSO that no longer exists.
func TestDeleteQSORemovesPendingOutboxRows(t *testing.T) {
	st := openTestStore(t)
	q := validTestQSO()
	id, err := st.insertQSO(q)
	if err != nil {
		t.Fatalf("insertQSO: %v", err)
	}
	if err := st.enqueueUpload(id, q.profileID, uploadDestQRZ, time.Now().Add(-time.Minute)); err != nil {
		t.Fatalf("enqueueUpload: %v", err)
	}
	if err := st.deleteQSO(q.profileID, id); err != nil {
		t.Fatalf("deleteQSO: %v", err)
	}
	entries, err := st.claimDueUploads(time.Now().Add(time.Hour), time.Minute, 10)
	if err != nil {
		t.Fatalf("claimDueUploads: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("claimed %d outbox entries after deleting the QSO, want 0", len(entries))
	}
}

// TestRedactQRZURLErrorStripsCredentials confirms the credential scrubber
// removes the request URL (which carries the QRZ username/password or session
// key in its query string) from a *url.Error, keeping only the operation and
// underlying cause.
func TestRedactQRZURLErrorStripsCredentials(t *testing.T) {
	secret := "s3cr3t-password"
	urlErr := &url.Error{
		Op:  "Get",
		URL: "https://xmldata.qrz.com/xml/?username=w4gns&password=" + secret,
		Err: errors.New("dial tcp: connection refused"),
	}
	got := redactQRZURLError(urlErr).Error()
	if got == "" {
		t.Fatal("redactQRZURLError returned an empty error")
	}
	if contains := errors.As(redactQRZURLError(urlErr), new(*url.Error)); contains {
		t.Fatal("redactQRZURLError still wraps the *url.Error (URL leaks)")
	}
	if strings.Contains(got, secret) || strings.Contains(got, "username=") {
		t.Fatalf("redactQRZURLError leaked credentials: %q", got)
	}
	// A non-URL error passes through unchanged.
	plain := errors.New("some other failure")
	if redactQRZURLError(plain) != plain {
		t.Fatal("redactQRZURLError altered a non-url.Error")
	}
}

// TestParsePOTASpotTimeAcceptsVariants confirms the tolerant parser handles a
// trailing Z, a timezone offset, and fractional seconds in addition to the
// bare layout the feed currently emits, so a format change doesn't silently
// stop POTA auto-fill.
func TestParsePOTASpotTimeAcceptsVariants(t *testing.T) {
	want := time.Date(2026, time.September, 3, 14, 30, 0, 0, time.UTC)
	for _, in := range []string{
		"2026-09-03T14:30:00",
		"2026-09-03T14:30:00Z",
		"2026-09-03T14:30:00+00:00",
		"2026-09-03T14:30:00.000Z",
	} {
		got, err := parsePOTASpotTime(in)
		if err != nil {
			t.Errorf("parsePOTASpotTime(%q) returned error: %v", in, err)
			continue
		}
		if !got.Equal(want) {
			t.Errorf("parsePOTASpotTime(%q) = %v, want %v", in, got, want)
		}
	}
	if _, err := parsePOTASpotTime("not-a-time"); err == nil {
		t.Error("parsePOTASpotTime accepted a malformed value")
	}
}

// TestPOTAReferencePatternIgnoresNonPOTATokens guards the tightened regex:
// common CW-comment tokens like RST-599 and CQ-1 must no longer be mistaken
// for POTA references, while real references still match.
func TestPOTAReferencePatternIgnoresNonPOTATokens(t *testing.T) {
	for _, comment := range []string{"TU RST-599", "CQ-1 up", "5NN TU"} {
		if got := potaReferencePattern.FindString(comment); got != "" {
			t.Errorf("potaReferencePattern matched %q in %q, want no match", got, comment)
		}
	}
	for _, comment := range []string{"POTA US-1234", "at K-0001 now", "us-222 pota"} {
		if got := potaReferencePattern.FindString(comment); got == "" {
			t.Errorf("potaReferencePattern found no reference in %q, want one", comment)
		}
	}
}

// TestDXCCAddAliasesToleratesOverrideTokens guards the fix that a routine
// cty.dat refresh carrying AD1C <lat/lon>, {continent}, or ~UTC~ per-alias
// override tokens no longer fails the whole load (which silently disabled all
// DXCC enrichment). The overrides this app uses are applied; the rest are
// ignored; the bare prefix still resolves.
func TestDXCCAddAliasesToleratesOverrideTokens(t *testing.T) {
	base := dxccEntity{Country: "Testland", CQZone: 5, ITUZone: 8, Continent: "NA"}
	table := &dxccTable{}
	// A prefix carrying every override kind, plus a plain one and an exact match.
	list := "TL<47.0/-8.0>{EU}~-1.0~(14)[28],T2A,=T3EXACT;"
	if err := table.addAliases(base, list); err != nil {
		t.Fatalf("addAliases rejected AD1C override tokens: %v", err)
	}
	entity, ok := table.lookup("TL7ABC")
	if !ok {
		t.Fatal("prefix with override tokens did not resolve")
	}
	if entity.CQZone != 14 || entity.ITUZone != 28 {
		t.Errorf("CQ/ITU overrides not applied: got CQ=%d ITU=%d, want 14/28", entity.CQZone, entity.ITUZone)
	}
	if entity.Continent != "EU" {
		t.Errorf("continent override not applied: got %q, want EU", entity.Continent)
	}
	if _, ok := table.lookup("T3EXACT"); !ok {
		t.Error("exact-match alias did not resolve")
	}
}
