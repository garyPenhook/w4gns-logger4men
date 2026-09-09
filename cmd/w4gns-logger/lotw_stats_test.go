package main

import (
	"testing"
	"time"
)

func TestLoadLoTWAwardStatsCountsWorkedAndConfirmed(t *testing.T) {
	st, err := openStore(t.TempDir() + "/logger.db")
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	insert := func(q qso) int64 {
		q.timeOff = q.time.Add(time.Minute)
		id, err := st.insertQSO(q)
		if err != nil {
			t.Fatal(err)
		}
		return id
	}

	// Two DXCC entities worked, one confirmed. One US state worked and
	// confirmed. One CQ zone worked, not confirmed. One 6M grid worked and
	// confirmed (VUCC); a non-6M grid must not count toward VUCC. One IOTA
	// reference worked, not confirmed.
	// The QSOs below pin dxccNumber/cqZone to the same values on every insert
	// except where a test explicitly varies them (VK2ABC's zone, G4ABC's
	// entity): resolveDXCC always resolves *some* DXCC entity/zone from the
	// callsign otherwise, which would inflate the tallies this test isolates.
	confirmedID := insert(qso{
		call: "W1AW", band: "20M", mode: "CW", time: time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC),
		profileID: 1, dxccNumber: "291", country: "UNITED STATES", state: "CT", cqZone: "5",
	})
	insert(qso{
		call: "G4ABC", band: "20M", mode: "CW", time: time.Date(2026, 3, 1, 13, 0, 0, 0, time.UTC),
		profileID: 1, dxccNumber: "223", country: "ENGLAND", cqZone: "5",
	})
	insert(qso{
		call: "VK2ABC", band: "15M", mode: "CW", time: time.Date(2026, 3, 1, 14, 0, 0, 0, time.UTC),
		profileID: 1, dxccNumber: "291", cqZone: "30",
	})
	vuccID := insert(qso{
		call: "W6ABC", band: "6M", mode: "CW", time: time.Date(2026, 3, 1, 15, 0, 0, 0, time.UTC),
		profileID: 1, dxccNumber: "291", grid: "CM87xx", cqZone: "5",
	})
	insert(qso{
		call: "W6DEF", band: "20M", mode: "CW", time: time.Date(2026, 3, 1, 16, 0, 0, 0, time.UTC),
		profileID: 1, dxccNumber: "291", grid: "CM88xx", cqZone: "5",
	})
	insert(qso{
		call: "DL1ABC", band: "20M", mode: "CW", time: time.Date(2026, 3, 1, 17, 0, 0, 0, time.UTC),
		profileID: 1, dxccNumber: "291", iotaRef: "EU-005", cqZone: "5",
	})

	// lotw_confirmation's own dxcc/country/state/cqz/band/gridsquare columns
	// (not the joined qso row) drive the confirmed side of each award tally —
	// see lotw_stats.go's *ConfirmedQuery comment — so they're populated here
	// the same way a real qso_qsldetail=yes sync would.
	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := st.db.Exec(
		`INSERT INTO lotw_confirmation (profile_id, qso_id, call, band, dxcc, country, state, cqz, synced_at) VALUES (1, ?, 'W1AW', '20M', '291', 'UNITED STATES', 'CT', '5', ?)`,
		confirmedID, now,
	); err != nil {
		t.Fatal(err)
	}
	if _, err := st.db.Exec(
		`INSERT INTO lotw_confirmation (profile_id, qso_id, call, band, dxcc, country, cqz, gridsquare, synced_at) VALUES (1, ?, 'W6ABC', '6M', '291', 'UNITED STATES', '5', 'CM87XX', ?)`,
		vuccID, now,
	); err != nil {
		t.Fatal(err)
	}

	stats, err := st.loadLoTWAwardStats(1)
	if err != nil {
		t.Fatal(err)
	}

	if stats.DXCC.Worked != 2 || stats.DXCC.Confirmed != 1 {
		t.Fatalf("DXCC = %+v, want worked=2 confirmed=1", stats.DXCC)
	}
	if len(stats.DXCC.Needed) != 1 || stats.DXCC.Needed[0] != "223 ENGLAND" {
		t.Fatalf("DXCC.Needed = %v, want [\"223 ENGLAND\"]", stats.DXCC.Needed)
	}

	if stats.WAS.Worked != 1 || stats.WAS.Confirmed != 1 || len(stats.WAS.Needed) != 0 {
		t.Fatalf("WAS = %+v, want worked=1 confirmed=1 needed=[]", stats.WAS)
	}

	if stats.WAZ.Worked != 2 || stats.WAZ.Confirmed != 1 || len(stats.WAZ.Needed) != 1 || stats.WAZ.Needed[0] != "30" {
		t.Fatalf("WAZ = %+v, want worked=2 confirmed=1 needed=[\"30\"]", stats.WAZ)
	}

	// W6DEF's grid is worked on 20M, not 6M, so it must not count toward
	// VUCC — only W6ABC's 6M contact does, and it's confirmed.
	if stats.VUCC.Worked != 1 || stats.VUCC.Confirmed != 1 || len(stats.VUCC.Needed) != 0 {
		t.Fatalf("VUCC = %+v, want worked=1 confirmed=1 needed=[] (only 6M grids count)", stats.VUCC)
	}

	if stats.IOTA.Worked != 1 || stats.IOTA.Confirmed != 0 || len(stats.IOTA.Needed) != 1 {
		t.Fatalf("IOTA = %+v, want worked=1 confirmed=0 needed=1", stats.IOTA)
	}
}
