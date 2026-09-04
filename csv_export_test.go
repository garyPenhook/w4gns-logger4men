package main

import (
	"context"
	"strings"
	"testing"
)

func TestCSVFieldQuotesCommaAndQuoteAndStripsControlChars(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"W1AW", "W1AW"},
		{"599 CA", "599 CA"},
		{"Smith, John", `"Smith, John"`},
		{`He said "hi"`, `"He said ""hi"""`},
		{"line\r\nbreak", "linebreak"},
	}
	for _, tc := range cases {
		if got := csvField(tc.in); got != tc.want {
			t.Errorf("csvField(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

// TestExportCSVWritesHeaderAndOnlyMatchingContestQSOs mirrors
// TestExportCabrilloWritesHeaderFooterAndOnlyMatchingContestQSOs: a profile
// logging QSOs in more than one contest must only export the one requested,
// and non-contest QSOs (blank contest_id) must never leak into the CSV.
func TestExportCSVWritesHeaderAndOnlyMatchingContestQSOs(t *testing.T) {
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
	count, err := exportCSV(context.Background(), &buf, profile, "CQ-WPX-CW", st)
	if err != nil {
		t.Fatalf("exportCSV: %v", err)
	}
	if count != 2 {
		t.Fatalf("exported %d QSOs, want 2", count)
	}
	out := buf.String()
	if !strings.HasPrefix(out, strings.Join(csvHeader, ",")+"\r\n") {
		t.Fatalf("output does not start with the CSV header: %q", out[:min(80, len(out))])
	}
	if !strings.Contains(out, "W1AW") || !strings.Contains(out, "K1ABC") {
		t.Fatalf("output missing expected contest QSOs: %q", out)
	}
	if strings.Contains(out, "N1MM") || strings.Contains(out, "W9XYZ") {
		t.Fatalf("output leaked QSOs from a different contest or a non-contest QSO: %q", out)
	}
}
