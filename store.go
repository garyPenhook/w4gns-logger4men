package main

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

// schemaTables creates every table if missing, but never an index: a
// genuinely old database (predating a column migrate() adds, e.g.
// profile_id) must have that column added before any index referencing it
// is created, or CREATE INDEX fails outright and openStore never reaches
// migrate() at all. See schemaIndexes, applied after migrate() in
// openStore.
const schemaTables = `
CREATE TABLE IF NOT EXISTS qso (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    call TEXT NOT NULL,
    qso_date TEXT NOT NULL,
    time_on TEXT NOT NULL,
	qso_date_off TEXT,
    time_off TEXT,
    band TEXT NOT NULL,
    freq REAL,
    mode TEXT DEFAULT 'CW',
    rst_sent TEXT,
    rst_rcvd TEXT,
    name TEXT,
    qth TEXT,
    gridsquare TEXT,
	my_gridsquare TEXT,
	station_callsign TEXT,
	operator_name TEXT,
	my_rig TEXT,
	my_antenna TEXT,
	tx_pwr TEXT,
    state TEXT,
    county TEXT,
    email TEXT,
    country TEXT,
    dxcc INTEGER,
    cqz INTEGER,
    ituz INTEGER,
	sig TEXT,
	sig_info TEXT,
	park_name TEXT,
    comment TEXT,
    qsl_sent TEXT DEFAULT 'N',
    qsl_rcvd TEXT DEFAULT 'N',

    contest_id TEXT,
    stx TEXT,
    stx_string TEXT,
    srx TEXT,
    srx_string TEXT,
    exchange_json TEXT,
    profile_id INTEGER
);

CREATE TABLE IF NOT EXISTS station_profile (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT NOT NULL UNIQUE,
    callsign TEXT,
    operator_name TEXT,
    my_gridsquare TEXT,
    latitude REAL,
    longitude REAL,
    timezone TEXT NOT NULL DEFAULT 'UTC',
    club TEXT,
    rig TEXT,
    antenna TEXT,
    power_watts REAL,
    category_operator TEXT,
    category_assisted TEXT,
    category_power TEXT,
    address TEXT,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);
`

// schemaIndexes must run after migrate() has added any column an index
// here references (profile_id in particular — see schemaTables).
const schemaIndexes = `
CREATE INDEX IF NOT EXISTS idx_call ON qso(call);
CREATE INDEX IF NOT EXISTS idx_call_band ON qso(call, band);
CREATE INDEX IF NOT EXISTS idx_dupe_window ON qso(call, band, qso_date, time_on);
CREATE INDEX IF NOT EXISTS idx_date ON qso(qso_date);
CREATE INDEX IF NOT EXISTS idx_date_time ON qso(qso_date, time_on);
CREATE INDEX IF NOT EXISTS idx_contest ON qso(contest_id);
CREATE INDEX IF NOT EXISTS idx_profile_date ON qso(profile_id, qso_date, time_on);
`

type store struct {
	db *sql.DB
}

const importBatchSize = 1_000
const dupeWindow = 15 * time.Minute

func potaSignal(reference string) string {
	if strings.TrimSpace(reference) == "" {
		return ""
	}
	return "POTA"
}

func openStore(path string) (*store, error) {
	// Pre-create the database file 0600 before any data is written. sql.Open
	// is lazy and the driver would otherwise create the .db/-wal/-shm files
	// under the process umask (often 0644) during schema/migrate, leaving a
	// window in which this app's private QSO data is group/world-readable.
	// tightenDBFilePermissions still runs at the end as a backstop for files
	// (e.g. sidecars) created after this point.
	precreateDBFile(path)
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open db: %w", err)
	}
	// This is a single-operator application. One writer connection avoids lock
	// contention while WAL keeps reads responsive as the log grows.
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	if err := configureSQLite(db); err != nil {
		db.Close()
		return nil, err
	}
	if _, err := db.Exec(schemaTables); err != nil {
		db.Close()
		return nil, fmt.Errorf("apply schema: %w", err)
	}
	s := &store{db: db}
	if err := s.migrate(); err != nil {
		db.Close()
		return nil, err
	}
	if _, err := db.Exec(schemaIndexes); err != nil {
		db.Close()
		return nil, fmt.Errorf("apply schema indexes: %w", err)
	}
	if _, err := db.Exec(uploadOutboxSchema); err != nil {
		db.Close()
		return nil, fmt.Errorf("apply upload outbox schema: %w", err)
	}
	if err := s.ensureDefaultProfile(); err != nil {
		db.Close()
		return nil, err
	}
	if err := s.backfillMissingProfileID(); err != nil {
		db.Close()
		return nil, err
	}
	tightenDBFilePermissions(path)
	return s, nil
}

// dbFilePermBits is the maximum permission bits a healthy database file (or
// its WAL/SHM sidecars) should have: owner read/write only. This app stores
// private QSO data, and the default file-create permissions (subject to the
// OS umask) can otherwise leave it group- or world-readable.
const dbFilePermBits = 0o600

// precreateDBFile best-effort creates the database file with owner-only
// permissions before the SQLite driver opens it, so the file never exists with
// looser (umask-derived) permissions even briefly. An in-memory or empty path,
// or an already-existing file, is left untouched. Any error is ignored: the
// driver and tightenDBFilePermissions handle the file from here.
func precreateDBFile(path string) {
	if path == "" || path == ":memory:" || strings.HasPrefix(path, "file::memory:") {
		return
	}
	if _, err := os.Stat(path); err == nil {
		return // already exists; don't disturb it
	}
	if f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY, dbFilePermBits); err == nil {
		f.Close()
	}
}

// tightenDBFilePermissions best-effort chmods the database file and its WAL/
// SHM sidecars, mirroring tightenKeyFilePermissions in qrz.go. Sidecar
// files that don't exist (e.g. WAL checkpointed away) are silently skipped.
func tightenDBFilePermissions(path string) {
	for _, p := range []string{path, path + "-wal", path + "-shm"} {
		info, err := os.Stat(p)
		if err != nil {
			continue
		}
		if info.Mode().Perm()&^dbFilePermBits != 0 {
			if err := os.Chmod(p, dbFilePermBits); err != nil {
				fmt.Fprintf(os.Stderr, "warning: could not restrict %s to owner-only permissions: %v\n", p, err)
			}
		}
	}
}

func configureSQLite(db *sql.DB) error {
	for _, statement := range []string{
		`PRAGMA journal_mode=WAL`,
		// FULL (not NORMAL) so a committed QSO and its upload_outbox rows are
		// fsynced at commit and survive an OS crash / power loss, not just a
		// process exit. The outbox advertises durable delivery across a crash
		// (see outbox.go); under WAL+NORMAL, un-checkpointed frames can be lost
		// on power loss, silently dropping the last logged QSOs and their
		// pending uploads. Commits are human-paced (or batched 1,000-at-a-time
		// on import), so the extra fsync per commit is not a hot path.
		`PRAGMA synchronous=FULL`,
		`PRAGMA foreign_keys=ON`,
		`PRAGMA busy_timeout=5000`,
		`PRAGMA temp_store=MEMORY`,
		`PRAGMA cache_size=-20000`, // approximately 20 MiB
	} {
		if _, err := db.Exec(statement); err != nil {
			return fmt.Errorf("configure SQLite (%s): %w", statement, err)
		}
	}
	return nil
}

func (s *store) Close() error {
	return s.db.Close()
}

// migrate keeps databases created by earlier versions compatible with the
// current schema. Each migration is additive so existing QSOs are preserved.
func (s *store) migrate() error {
	for _, column := range []struct {
		name       string
		definition string
	}{
		{name: "my_gridsquare", definition: "TEXT"},
		{name: "profile_id", definition: "INTEGER"},
		{name: "qso_date_off", definition: "TEXT"},
		{name: "time_off", definition: "TEXT"},
		{name: "sig", definition: "TEXT"},
		{name: "sig_info", definition: "TEXT"},
		{name: "station_callsign", definition: "TEXT"},
		{name: "operator_name", definition: "TEXT"},
		{name: "my_rig", definition: "TEXT"},
		{name: "my_antenna", definition: "TEXT"},
		{name: "tx_pwr", definition: "TEXT"},
		{name: "county", definition: "TEXT"},
		{name: "email", definition: "TEXT"},
		{name: "park_name", definition: "TEXT"},
	} {
		exists, err := s.columnExists("qso", column.name)
		if err != nil {
			return fmt.Errorf("inspect qso schema: %w", err)
		}
		if !exists {
			if _, err := s.db.Exec("ALTER TABLE qso ADD COLUMN " + column.name + " " + column.definition); err != nil {
				return fmt.Errorf("add qso.%s: %w", column.name, err)
			}
		}
	}
	for _, column := range []struct {
		name       string
		definition string
	}{
		{name: "category_operator", definition: "TEXT"},
		{name: "category_assisted", definition: "TEXT"},
		{name: "category_power", definition: "TEXT"},
		{name: "address", definition: "TEXT"},
	} {
		exists, err := s.columnExists("station_profile", column.name)
		if err != nil {
			return fmt.Errorf("inspect station_profile schema: %w", err)
		}
		if !exists {
			if _, err := s.db.Exec("ALTER TABLE station_profile ADD COLUMN " + column.name + " " + column.definition); err != nil {
				return fmt.Errorf("add station_profile.%s: %w", column.name, err)
			}
		}
	}
	return nil
}

// columnExists reports whether table has a column named name. table is
// always one of this file's own constant identifiers, never user input.
func (s *store) columnExists(table, name string) (bool, error) {
	rows, err := s.db.Query(`PRAGMA table_info(` + table + `)`)
	if err != nil {
		return false, err
	}
	defer rows.Close()
	for rows.Next() {
		var cid int
		var columnName, columnType string
		var notNull, primaryKey int
		var defaultValue sql.NullString
		if err := rows.Scan(&cid, &columnName, &columnType, &notNull, &defaultValue, &primaryKey); err != nil {
			return false, err
		}
		if columnName == name {
			return true, nil
		}
	}
	return false, rows.Err()
}

func (s *store) ensureDefaultProfile() error {
	var exists bool
	if err := s.db.QueryRow(`SELECT EXISTS(SELECT 1 FROM station_profile)`).Scan(&exists); err != nil {
		return fmt.Errorf("check station profiles: %w", err)
	}
	if exists {
		return nil
	}
	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := s.db.Exec(
		`INSERT INTO station_profile (name, timezone, created_at, updated_at) VALUES (?, ?, ?, ?)`,
		"Default", defaultTimezone(), now, now,
	); err != nil {
		return fmt.Errorf("create default station profile: %w", err)
	}
	return nil
}

// backfillMissingProfileID assigns every QSO left over from a
// pre-station-profile schema (profile_id IS NULL) to the oldest station
// profile. Without this, those rows stay visible in counts and call history
// but silently vanish from exportADIF/qrz uploads, which filter by
// profile_id.
func (s *store) backfillMissingProfileID() error {
	var hasOrphans bool
	if err := s.db.QueryRow(`SELECT EXISTS(SELECT 1 FROM qso WHERE profile_id IS NULL)`).Scan(&hasOrphans); err != nil {
		return fmt.Errorf("check for orphaned qso rows: %w", err)
	}
	if !hasOrphans {
		return nil
	}
	var defaultProfileID int64
	if err := s.db.QueryRow(`SELECT id FROM station_profile ORDER BY id LIMIT 1`).Scan(&defaultProfileID); err != nil {
		return fmt.Errorf("find default station profile for backfill: %w", err)
	}
	if _, err := s.db.Exec(`UPDATE qso SET profile_id = ? WHERE profile_id IS NULL`, defaultProfileID); err != nil {
		return fmt.Errorf("backfill qso.profile_id: %w", err)
	}
	return nil
}

// defaultTimezone returns an IANA time-zone identifier whenever the host
// exposes one. "Local" is not persisted because it is ambiguous and cannot
// correctly preserve historical daylight-saving rules. UTC is the safe,
// portable fallback.
func defaultTimezone() string {
	candidates := []string{validTimezoneIdentifier(os.Getenv("TZ")), validTimezoneIdentifier(time.Local.String())}
	if link, err := os.Readlink("/etc/localtime"); err == nil {
		if index := strings.Index(link, "/zoneinfo/"); index >= 0 {
			candidates = append(candidates, validTimezoneIdentifier(link[index+len("/zoneinfo/"):]))
		}
	}
	if contents, err := os.ReadFile("/etc/timezone"); err == nil {
		candidates = append(candidates, validTimezoneIdentifier(strings.TrimSpace(string(contents))))
	}
	for _, timezone := range candidates {
		if timezone == "" {
			continue
		}
		if _, err := time.LoadLocation(timezone); err == nil {
			return timezone
		}
	}
	return "UTC"
}

func validTimezoneIdentifier(timezone string) string {
	timezone = strings.TrimSpace(timezone)
	if timezone == "UTC" || strings.Contains(timezone, "/") {
		return strings.TrimPrefix(filepath.Clean(timezone), "./")
	}
	return ""
}

// dxccContext resolves the worked station's country/CQ-zone/ITU-zone/DXCC
// entity number from the embedded cty.dat and ARRL DXCC list, best-effort. A
// lookup miss (or a cty.dat load failure, which should not happen with the
// embedded copy) simply leaves these fields blank rather than failing the
// QSO. dxccNumber is nil both on a lookup miss and when the resolved entity
// has no known ARRL DXCC number (DXCCNumber == 0, e.g. an entity ARRL
// doesn't count as separate from its parent — see dxccEntity's doc comment).
func dxccContext(call string) (country string, cqZone, ituZone, dxccNumber any) {
	table, err := sharedDXCCTable()
	if err != nil {
		return "", nil, nil, nil
	}
	entity, ok := table.lookup(call)
	if !ok {
		return "", nil, nil, nil
	}
	if entity.DXCCNumber != 0 {
		dxccNumber = entity.DXCCNumber
	}
	return entity.Country, entity.CQZone, entity.ITUZone, dxccNumber
}

// resolveDXCC returns the country/CQ-zone/ITU-zone/DXCC-number to persist
// for q, preferring values already present on q (e.g. parsed from an
// imported ADIF record's COUNTRY/CQZ/ITUZ/DXCC fields) over a fresh cty.dat
// lookup, so accurate imported data isn't silently overwritten by a local
// guess.
func resolveDXCC(q qso) (country string, cqZone, ituZone, dxccNumber any) {
	country, cqZone, ituZone, dxccNumber = dxccContext(q.call)
	if c := strings.TrimSpace(q.country); c != "" {
		country = c
	}
	if z := strings.TrimSpace(q.cqZone); z != "" {
		if n, err := strconv.Atoi(z); err == nil {
			cqZone = n
		}
	}
	if z := strings.TrimSpace(q.ituZone); z != "" {
		if n, err := strconv.Atoi(z); err == nil {
			ituZone = n
		}
	}
	if z := strings.TrimSpace(q.dxccNumber); z != "" {
		if n, err := strconv.Atoi(z); err == nil {
			dxccNumber = n
		}
	}
	return country, cqZone, ituZone, dxccNumber
}

// insertQSO writes a general (non-contest) QSO and returns its id.
func (s *store) insertQSO(q qso) (int64, error) {
	if err := validateQSO(q); err != nil {
		return 0, fmt.Errorf("validate qso: %w", err)
	}
	utcTime := q.time.UTC()
	utcTimeOff := q.timeOff.UTC()
	country, cqZone, ituZone, dxccNumber := resolveDXCC(q)
	res, err := s.db.Exec(
		`INSERT INTO qso (call, qso_date, time_on, qso_date_off, time_off, band, freq, mode, rst_sent, rst_rcvd, name, qth, gridsquare, state, county, email, country, dxcc, cqz, ituz, sig, sig_info, park_name, comment, contest_id, stx, stx_string, srx, srx_string, profile_id, my_gridsquare, station_callsign, operator_name, my_rig, my_antenna, tx_pwr)
			 VALUES (?, ?, ?, ?, ?, ?, NULLIF(?, ''), ?, ?, ?, ?, ?, ?, ?, ?, ?, NULLIF(?, ''), ?, ?, ?, NULLIF(?, ''), NULLIF(?, ''), ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		q.call,
		utcTime.Format("20060102"),
		utcTime.Format("150405"),
		utcTimeOff.Format("20060102"),
		utcTimeOff.Format("150405"),
		q.band,
		q.frequency,
		q.mode,
		q.rstSent,
		q.rstRcvd,
		q.name,
		q.qth,
		q.grid,
		q.state,
		q.county,
		q.email,
		country,
		dxccNumber,
		cqZone,
		ituZone,
		potaSignal(q.potaRef),
		q.potaRef,
		q.parkName,
		q.comment,
		q.contestID,
		q.stx,
		q.stxString,
		q.srx,
		q.srxString,
		q.profileID,
		q.myGridSquare,
		q.stationCallsign,
		q.operatorName,
		q.myRig,
		q.myAntenna,
		q.txPower,
	)
	if err != nil {
		return 0, fmt.Errorf("insert qso: %w", err)
	}
	return res.LastInsertId()
}

// insertQSOBatch imports QSOs in bounded transactions, skipping any record
// that exactly matches one already on file (same call/band/qso_date/time_on/
// profile_id) so re-running an import after a mid-file failure doesn't
// duplicate the batches that already landed. It validates each record before
// persistence, rolls back the entire failing batch, and runs PRAGMA optimize
// once the import is complete. Callers can stream a large ADIF file through
// 1,000-record batches without loading it all into memory. Returns the
// number of records actually inserted (excluding skipped duplicates).
func (s *store) insertQSOBatch(ctx context.Context, qsos []qso) (int, error) {
	inserted := 0
	for start := 0; start < len(qsos); start += importBatchSize {
		end := start + importBatchSize
		if end > len(qsos) {
			end = len(qsos)
		}
		n, err := s.insertQSOChunk(ctx, qsos[start:end])
		inserted += n
		if err != nil {
			return inserted, fmt.Errorf("import QSOs %d-%d: %w", start+1, end, err)
		}
	}
	if _, err := s.db.ExecContext(ctx, `PRAGMA optimize`); err != nil {
		return inserted, fmt.Errorf("optimize database after import: %w", err)
	}
	return inserted, nil
}

func (s *store) insertQSOChunk(ctx context.Context, qsos []qso) (int, error) {
	for index, q := range qsos {
		if err := validateQSO(q); err != nil {
			return 0, fmt.Errorf("validate record %d: %w", index+1, err)
		}
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("begin import transaction: %w", err)
	}
	defer tx.Rollback()
	existsStatement, err := tx.PrepareContext(ctx, `SELECT 1 FROM qso WHERE call = ? AND band = ? AND qso_date = ? AND time_on = ? AND profile_id = ? LIMIT 1`)
	if err != nil {
		return 0, fmt.Errorf("prepare import dupe check: %w", err)
	}
	defer existsStatement.Close()
	statement, err := tx.PrepareContext(ctx, `INSERT INTO qso (call, qso_date, time_on, qso_date_off, time_off, band, freq, mode, rst_sent, rst_rcvd, name, qth, gridsquare, state, county, email, country, dxcc, cqz, ituz, sig, sig_info, park_name, comment, contest_id, stx, stx_string, srx, srx_string, profile_id, my_gridsquare, station_callsign, operator_name, my_rig, my_antenna, tx_pwr)
		VALUES (?, ?, ?, ?, ?, ?, NULLIF(?, ''), ?, ?, ?, ?, ?, ?, ?, ?, ?, NULLIF(?, ''), ?, ?, ?, NULLIF(?, ''), NULLIF(?, ''), ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return 0, fmt.Errorf("prepare import insert: %w", err)
	}
	defer statement.Close()
	inserted := 0
	for index, q := range qsos {
		start, end := q.time.UTC(), q.timeOff.UTC()
		qsoDate, timeOn := start.Format("20060102"), start.Format("150405")
		var exists int
		switch err := existsStatement.QueryRowContext(ctx, q.call, q.band, qsoDate, timeOn, q.profileID).Scan(&exists); {
		case err == nil:
			continue // already imported; skip to keep re-imports idempotent
		case err != sql.ErrNoRows:
			return inserted, fmt.Errorf("check existing record %d: %w", index+1, err)
		}
		country, cqZone, ituZone, dxccNumber := resolveDXCC(q)
		if _, err := statement.ExecContext(ctx, q.call, qsoDate, timeOn, end.Format("20060102"), end.Format("150405"), q.band, q.frequency, q.mode, q.rstSent, q.rstRcvd, q.name, q.qth, q.grid, q.state, q.county, q.email, country, dxccNumber, cqZone, ituZone, potaSignal(q.potaRef), q.potaRef, q.parkName, q.comment, q.contestID, q.stx, q.stxString, q.srx, q.srxString, q.profileID, q.myGridSquare, q.stationCallsign, q.operatorName, q.myRig, q.myAntenna, q.txPower); err != nil {
			return inserted, fmt.Errorf("insert record %d: %w", index+1, err)
		}
		inserted++
	}
	if err := tx.Commit(); err != nil {
		return inserted, fmt.Errorf("commit import transaction: %w", err)
	}
	return inserted, nil
}

// isDupe reports whether call has already been worked on band, per the
// dupe-check scope (call+band) described in the contest design doc.
// isDupe reports whether call has already been worked on band. Outside a
// contest (dupeScope blank) it uses the fixed 15-minute call+band window
// suited to casual/POTA logging, where re-working the same station later the
// same day is not a dupe. Inside a contest it honors the event's dupe_scope
// instead of the fixed window:
//   - "call+band+session": a dupe only within this exact session
//     (contestID, e.g. "CWT-1900"), unbounded in time — CWT/CW Open allow
//     working the same station again in a later session.
//   - anything else non-blank (almost every catalog entry uses
//     "call+band"): a dupe anywhere in this contest (any session of eventID),
//     unbounded in time.
//
// excludeID, when non-zero, omits that row's own id from the match — used
// when re-saving an edited QSO so it doesn't count as a dupe of itself.
func (s *store) isDupe(call, band, contestID, eventID, dupeScope string, profileID, excludeID int64, now time.Time) (bool, error) {
	var (
		query string
		args  []any
	)
	switch {
	case dupeScope == "":
		windowStart := now.UTC().Add(-dupeWindow).Format("20060102150405")
		windowEnd := now.UTC().Format("20060102150405")
		query = `SELECT COUNT(1) FROM qso WHERE call = ? AND band = ? AND profile_id = ? AND (qso_date || time_on) BETWEEN ? AND ?`
		args = []any{call, band, profileID, windowStart, windowEnd}
	case dupeScope == "call+band+session":
		query = `SELECT COUNT(1) FROM qso WHERE call = ? AND band = ? AND profile_id = ? AND contest_id = ?`
		args = []any{call, band, profileID, contestID}
	default:
		query = `SELECT COUNT(1) FROM qso WHERE call = ? AND band = ? AND profile_id = ? AND (contest_id = ? OR contest_id LIKE ?)`
		args = []any{call, band, profileID, eventID, eventID + "-%"}
	}
	if excludeID != 0 {
		query += ` AND id != ?`
		args = append(args, excludeID)
	}
	var n int
	if err := s.db.QueryRow(query, args...).Scan(&n); err != nil {
		return false, fmt.Errorf("dupe check: %w", err)
	}
	return n > 0, nil
}

// recentQSOs returns the most recent QSOs, newest first, for populating the
// log table. Each qso's id is populated so a selected table row can be
// looked up (for editing or deletion) without a second query.
func (s *store) recentQSOs(profileID int64, limit int) ([]qso, error) {
	rows, err := s.db.Query(
		`SELECT id, call, band, mode, rst_sent, rst_rcvd, srx_string, qso_date, time_on
		 FROM qso WHERE profile_id = ? ORDER BY id DESC LIMIT ?`,
		profileID, limit,
	)
	if err != nil {
		return nil, fmt.Errorf("recent qsos: %w", err)
	}
	defer rows.Close()

	var out []qso
	for rows.Next() {
		var q qso
		var qsoDate, timeOn string
		if err := rows.Scan(&q.id, &q.call, &q.band, &q.mode, &q.rstSent, &q.rstRcvd, &q.exchange, &qsoDate, &timeOn); err != nil {
			return nil, fmt.Errorf("scan qso: %w", err)
		}
		t, err := time.Parse("20060102150405", qsoDate+timeOn)
		if err == nil {
			q.time = t
		}
		out = append(out, q)
	}
	return out, rows.Err()
}

// qsoByID loads one QSO's editable fields plus its previously-resolved
// country/CQZ/ITUZ/DXCC context, so updateQSO can carry that context forward
// unchanged (via resolveDXCC's "prefer an already-set value" rule) when the
// callsign isn't part of the edit. It also loads the station-identity
// snapshot (my grid, station callsign, operator, rig, antenna, power) so
// callers that need a fully-formed QSO for external upload (see
// uploadBufferCmd) don't get blank ADIF station fields.
func (s *store) qsoByID(profileID, id int64) (qso, error) {
	var q qso
	var date, timeOn, dateOff, timeOff string
	var cqZone, ituZone, dxccNumber sql.NullString
	err := s.db.QueryRow(`SELECT id, call, qso_date, time_on, COALESCE(qso_date_off, ''), COALESCE(time_off, ''), band,
		COALESCE(freq, ''), mode, COALESCE(rst_sent, ''), COALESCE(rst_rcvd, ''), COALESCE(name, ''), COALESCE(qth, ''),
		COALESCE(gridsquare, ''), COALESCE(state, ''), COALESCE(county, ''), COALESCE(email, ''), COALESCE(country, ''), CAST(dxcc AS TEXT), CAST(cqz AS TEXT), CAST(ituz AS TEXT),
		COALESCE(sig_info, ''), COALESCE(park_name, ''), COALESCE(comment, ''), COALESCE(contest_id, ''),
		COALESCE(stx, ''), COALESCE(stx_string, ''), COALESCE(srx, ''), COALESCE(srx_string, ''), profile_id,
		COALESCE(my_gridsquare, ''), COALESCE(station_callsign, ''), COALESCE(operator_name, ''),
		COALESCE(my_rig, ''), COALESCE(my_antenna, ''), COALESCE(tx_pwr, '')
		FROM qso WHERE id = ? AND profile_id = ?`, id, profileID).Scan(
		&q.id, &q.call, &date, &timeOn, &dateOff, &timeOff, &q.band, &q.frequency, &q.mode, &q.rstSent, &q.rstRcvd,
		&q.name, &q.qth, &q.grid, &q.state, &q.county, &q.email, &q.country, &dxccNumber, &cqZone, &ituZone, &q.potaRef, &q.parkName, &q.comment, &q.contestID,
		&q.stx, &q.stxString, &q.srx, &q.srxString, &q.profileID,
		&q.myGridSquare, &q.stationCallsign, &q.operatorName, &q.myRig, &q.myAntenna, &q.txPower,
	)
	if err != nil {
		return qso{}, fmt.Errorf("load qso %d: %w", id, err)
	}
	q.cqZone, q.ituZone, q.dxccNumber = cqZone.String, ituZone.String, dxccNumber.String
	q.time, _ = time.Parse("20060102150405", date+timeOn)
	q.timeOff, _ = time.Parse("20060102150405", dateOff+timeOff)
	if q.timeOff.IsZero() {
		q.timeOff = q.time
	}
	return q, nil
}

// updateQSO overwrites an existing QSO's editable fields in place, keeping
// its original id, profile_id, and start/end times (editing corrects
// content, not when the contact happened). country/CQZ/ITUZ/DXCC are
// refreshed via resolveDXCC exactly as insertQSO does, so correcting a
// callsign also corrects the DXCC context resolved from it.
func (s *store) updateQSO(id int64, q qso) error {
	q.id = id
	if err := validateQSO(q); err != nil {
		return fmt.Errorf("validate qso: %w", err)
	}
	country, cqZone, ituZone, dxccNumber := resolveDXCC(q)
	_, err := s.db.Exec(
		`UPDATE qso SET call = ?, band = ?, freq = NULLIF(?, ''), rst_sent = ?, rst_rcvd = ?, name = ?, qth = ?,
			gridsquare = ?, state = ?, county = ?, email = ?, country = NULLIF(?, ''), dxcc = ?, cqz = ?, ituz = ?, sig = NULLIF(?, ''),
			sig_info = NULLIF(?, ''), park_name = ?, comment = ?, contest_id = ?, stx = ?, stx_string = ?, srx = ?, srx_string = ?
			WHERE id = ? AND profile_id = ?`,
		q.call, q.band, q.frequency, q.rstSent, q.rstRcvd, q.name, q.qth,
		q.grid, q.state, q.county, q.email, country, dxccNumber, cqZone, ituZone, potaSignal(q.potaRef),
		q.potaRef, q.parkName, q.comment, q.contestID, q.stx, q.stxString, q.srx, q.srxString,
		id, q.profileID,
	)
	if err != nil {
		return fmt.Errorf("update qso %d: %w", id, err)
	}
	return nil
}

// deleteQSO permanently removes one QSO. There is no undo; callers must
// confirm with the operator before calling this.
func (s *store) deleteQSO(profileID, id int64) error {
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("begin delete qso %d: %w", id, err)
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`DELETE FROM qso WHERE id = ? AND profile_id = ?`, id, profileID); err != nil {
		return fmt.Errorf("delete qso %d: %w", id, err)
	}
	// The upload_outbox has no foreign key onto qso (foreign_keys=ON therefore
	// can't cascade here), so its pending rows must be removed explicitly in the
	// same transaction. Otherwise the drain keeps trying to deliver a QSO that
	// no longer exists until it exhausts maxUploadAttempts and parks the row a
	// year out with a permanent stale error.
	if _, err := tx.Exec(`DELETE FROM upload_outbox WHERE qso_id = ?`, id); err != nil {
		return fmt.Errorf("delete outbox rows for qso %d: %w", id, err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit delete qso %d: %w", id, err)
	}
	return nil
}

// qsosByCall returns every previously logged contact for a callsign, newest
// first. The active entry is not in SQLite yet, so it is never shown here until
// the operator explicitly saves it.
// qsosByCall returns a callsign's prior contacts, newest first, with id
// populated so this list can also back the Recent QSOs table's F9
// browse/edit/delete selection while it's showing call history instead of
// the default recent list (see showWorkedCall).
func (s *store) qsosByCall(profileID int64, call string) ([]qso, error) {
	rows, err := s.db.Query(
		`SELECT id, call, band, mode, rst_sent, rst_rcvd, srx_string, qso_date, time_on
		 FROM qso WHERE call = ? AND profile_id = ? ORDER BY qso_date DESC, time_on DESC, id DESC`,
		call, profileID,
	)
	if err != nil {
		return nil, fmt.Errorf("query call history: %w", err)
	}
	defer rows.Close()
	var out []qso
	for rows.Next() {
		var q qso
		var qsoDate, timeOn string
		if err := rows.Scan(&q.id, &q.call, &q.band, &q.mode, &q.rstSent, &q.rstRcvd, &q.exchange, &qsoDate, &timeOn); err != nil {
			return nil, fmt.Errorf("scan call history: %w", err)
		}
		if timestamp, err := time.Parse("20060102150405", qsoDate+timeOn); err == nil {
			q.time = timestamp
		}
		out = append(out, q)
	}
	return out, rows.Err()
}

// count returns the total number of logged QSOs.
func (s *store) count(profileID int64) (int, error) {
	var n int
	err := s.db.QueryRow(`SELECT COUNT(1) FROM qso WHERE profile_id = ?`, profileID).Scan(&n)
	return n, err
}
