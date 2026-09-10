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

	stats, err := st.loadLoTWAwardStats(1, "")
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

// TestLoadLoTWAwardStatsWASScopesToUSAAlaskaHawaiiAndFoldsDCIntoMaryland
// guards the ARRL WAS rule fix: the ADIF STATE field is reused by many
// countries for their own primary administrative subdivisions (Canadian
// provinces, Russian oblasts, etc.), and some of those codes collide with a
// real US state's two-letter code (e.g. "AR" is both Arkansas and a European
// Russia oblast) — a naive "any non-blank STATE" count previously let a
// foreign confirmation masquerade as a US state, which is how a real log
// showed 61 "confirmed" states, more than the 50 that exist. WAS must also
// still credit Alaska/Hawaii (separate DXCC entities from the mainland, per
// ARRL's DXCC FAQ) and fold DC into Maryland (per ARRL's WAS rules).
func TestLoadLoTWAwardStatsWASScopesToUSAAlaskaHawaiiAndFoldsDCIntoMaryland(t *testing.T) {
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

	// Arkansas (a real US state, dxcc 291) vs. a European Russia oblast that
	// happens to share the "AR" code (dxcc 15) — only the former may count.
	arkansasID := insert(qso{
		call: "W5AR", band: "20M", mode: "CW", time: time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC),
		profileID: 1, dxccNumber: "291", state: "AR", cqZone: "5",
	})
	insert(qso{
		call: "R1ABC", band: "20M", mode: "CW", time: time.Date(2026, 3, 1, 13, 0, 0, 0, time.UTC),
		profileID: 1, dxccNumber: "15", state: "AR", cqZone: "16",
	})
	// Alaska and Hawaii are separate DXCC entities (6 and 110) yet still
	// count as 2 of the 50 states.
	alaskaID := insert(qso{
		call: "KL7ABC", band: "20M", mode: "CW", time: time.Date(2026, 3, 1, 14, 0, 0, 0, time.UTC),
		profileID: 1, dxccNumber: "6", state: "AK", cqZone: "1",
	})
	hawaiiID := insert(qso{
		call: "KH6ABC", band: "20M", mode: "CW", time: time.Date(2026, 3, 1, 15, 0, 0, 0, time.UTC),
		profileID: 1, dxccNumber: "110", state: "HI", cqZone: "31",
	})
	// DC folds into Maryland, not a 51st "state".
	dcID := insert(qso{
		call: "W3DC", band: "20M", mode: "CW", time: time.Date(2026, 3, 1, 16, 0, 0, 0, time.UTC),
		profileID: 1, dxccNumber: "291", state: "DC", cqZone: "5",
	})

	now := time.Now().UTC().Format(time.RFC3339)
	for _, c := range []struct {
		id                int64
		call, dxcc, state string
	}{
		{arkansasID, "W5AR", "291", "AR"},
		{alaskaID, "KL7ABC", "6", "AK"},
		{hawaiiID, "KH6ABC", "110", "HI"},
		{dcID, "W3DC", "291", "DC"},
	} {
		if _, err := st.db.Exec(
			`INSERT INTO lotw_confirmation (profile_id, qso_id, call, band, dxcc, country, state, cqz, synced_at) VALUES (1, ?, ?, '20M', ?, 'X', ?, '5', ?)`,
			c.id, c.call, c.dxcc, c.state, now,
		); err != nil {
			t.Fatal(err)
		}
	}
	// A Russia oblast confirmation reusing the "AR" code must not count as an
	// Arkansas confirmation.
	if _, err := st.db.Exec(
		`INSERT INTO lotw_confirmation (profile_id, call, band, dxcc, country, state, cqz, synced_at) VALUES (1, 'R1ABC', '20M', '15', 'ASIATIC RUSSIA', 'AR', '16', ?)`,
		now,
	); err != nil {
		t.Fatal(err)
	}

	stats, err := st.loadLoTWAwardStats(1, "")
	if err != nil {
		t.Fatal(err)
	}

	// Worked: AR (from W5AR only, not the Russia "AR"), AK, HI, and DC folded
	// into MD = 4 distinct states (AR, AK, HI, MD).
	if stats.WAS.Worked != 4 {
		t.Fatalf("WAS.Worked = %d, want 4 (AR/AK/HI/MD, excluding the Russia oblast's colliding \"AR\" code)", stats.WAS.Worked)
	}
	if stats.WAS.Confirmed != 4 {
		t.Fatalf("WAS.Confirmed = %d, want 4 (AR/AK/HI/MD, excluding the Russia oblast confirmation)", stats.WAS.Confirmed)
	}
	if len(stats.WAS.Needed) != 0 {
		t.Fatalf("WAS.Needed = %v, want none (everything worked is confirmed)", stats.WAS.Needed)
	}
}

// TestLoadLoTWAwardStatsExcludesUnmatchedConfirmations guards the README's
// documented behavior ("An unmatched confirmation ... is still recorded,
// just not counted in the stats above"): a lotw_confirmation row with no
// matching local QSO (qso_id NULL) must not inflate Confirmed, since a
// Confirmed count that can exceed Worked contradicts the worked/needed set
// semantics awardProgressFor relies on.
func TestLoadLoTWAwardStatsExcludesUnmatchedConfirmations(t *testing.T) {
	st, err := openStore(t.TempDir() + "/logger.db")
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	id, err := st.insertQSO(qso{
		call: "W1AW", band: "20M", mode: "CW", time: time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC),
		timeOff:   time.Date(2026, 3, 1, 12, 1, 0, 0, time.UTC),
		profileID: 1, dxccNumber: "291", country: "UNITED STATES", cqZone: "5",
	})
	if err != nil {
		t.Fatal(err)
	}

	now := time.Now().UTC().Format(time.RFC3339)
	// Matched: qso_id set, joins to the QSO above.
	if _, err := st.db.Exec(
		`INSERT INTO lotw_confirmation (profile_id, qso_id, call, band, dxcc, country, cqz, synced_at) VALUES (1, ?, 'W1AW', '20M', '291', 'UNITED STATES', '5', ?)`,
		id, now,
	); err != nil {
		t.Fatal(err)
	}
	// Unmatched: no local QSO for this confirmation (e.g. logged elsewhere).
	if _, err := st.db.Exec(
		`INSERT INTO lotw_confirmation (profile_id, call, band, dxcc, country, cqz, synced_at) VALUES (1, 'K1ABC', '20M', '223', 'ENGLAND', '5', ?)`,
		now,
	); err != nil {
		t.Fatal(err)
	}

	stats, err := st.loadLoTWAwardStats(1, "")
	if err != nil {
		t.Fatal(err)
	}
	if stats.DXCC.Worked != 1 || stats.DXCC.Confirmed != 1 {
		t.Fatalf("DXCC = %+v, want worked=1 confirmed=1 (unmatched ENGLAND confirmation must not count)", stats.DXCC)
	}
}

// TestLoadLoTWAwardStatsScopesToStationCallsign guards the fix for combining
// two different operating identities logged under one profile (e.g. a
// callsign change, or a second operator sharing the profile): only QSOs
// whose station_callsign matches the active profile's current callsign, or
// have no station_callsign snapshot at all (pre-existing QSOs logged before
// that column existed), may count toward award totals.
func TestLoadLoTWAwardStatsScopesToStationCallsign(t *testing.T) {
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

	// Logged under the profile's current callsign: must count.
	currentID := insert(qso{
		call: "W1AW", band: "20M", mode: "CW", time: time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC),
		profileID: 1, dxccNumber: "291", country: "UNITED STATES", cqZone: "5",
		stationCallsign: "N0CALL",
	})
	// Logged under a different callsign (e.g. profile reused by another
	// operator, or a vanity callsign change): must not count.
	insert(qso{
		call: "G4ABC", band: "20M", mode: "CW", time: time.Date(2026, 3, 1, 13, 0, 0, 0, time.UTC),
		profileID: 1, dxccNumber: "223", country: "ENGLAND", cqZone: "5",
		stationCallsign: "OTHERCALL",
	})
	// No station_callsign snapshot at all (pre-existing data): must still
	// count, so older logs don't lose their award progress.
	legacyID := insert(qso{
		call: "VK2ABC", band: "15M", mode: "CW", time: time.Date(2026, 3, 1, 14, 0, 0, 0, time.UTC),
		profileID: 1, dxccNumber: "150", country: "AUSTRALIA", cqZone: "30",
	})

	now := time.Now().UTC().Format(time.RFC3339)
	for _, c := range []struct {
		id               int64
		call, dxcc, ctry string
	}{
		{currentID, "W1AW", "291", "UNITED STATES"},
		{legacyID, "VK2ABC", "150", "AUSTRALIA"},
	} {
		if _, err := st.db.Exec(
			`INSERT INTO lotw_confirmation (profile_id, qso_id, call, band, dxcc, country, cqz, synced_at) VALUES (1, ?, ?, '20M', ?, ?, '5', ?)`,
			c.id, c.call, c.dxcc, c.ctry, now,
		); err != nil {
			t.Fatal(err)
		}
	}

	stats, err := st.loadLoTWAwardStats(1, "N0CALL")
	if err != nil {
		t.Fatal(err)
	}
	if stats.DXCC.Worked != 2 {
		t.Fatalf("DXCC.Worked = %d, want 2 (N0CALL + legacy, excluding OTHERCALL)", stats.DXCC.Worked)
	}
	if stats.DXCC.Confirmed != 2 {
		t.Fatalf("DXCC.Confirmed = %d, want 2 (N0CALL + legacy, excluding OTHERCALL)", stats.DXCC.Confirmed)
	}
	if len(stats.DXCC.Needed) != 0 {
		t.Fatalf("DXCC.Needed = %v, want none", stats.DXCC.Needed)
	}
}
