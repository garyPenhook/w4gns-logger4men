package main

import (
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"
)

// A session template repeats; @ identifies the actual operating occurrence.
// Weekly sessions use the UTC operating date; annual contests use the year.
func contestOccurrenceID(base string, event eventDefinition, at time.Time) string {
	base, _, _ = strings.Cut(strings.TrimSpace(base), "@")
	stamp := at.UTC().Format("2006")
	kind := strings.ToLower(event.Kind)
	if strings.Contains(kind, "weekly") || strings.Contains(kind, "daily") {
		stamp = at.UTC().Format("20060102")
	} else if strings.Contains(kind, "monthly") {
		stamp = at.UTC().Format("200601")
	}
	return base + "@" + stamp
}

// resolveOccurrenceForNow re-resolves a contest_id (freshly typed, restored
// from a previous session, or otherwise not necessarily current) against the
// occurrence it belongs to as of at. A weekly/monthly/annual session
// template rolls forward to today's stamp; an unresolved imported ID gets
// one assigned. Returns raw unchanged when it already names a fixed contest
// occurrence that doesn't need resolving.
func resolveOccurrenceForNow(raw string, event eventDefinition, at time.Time) string {
	switch {
	case strings.Contains(raw, "@"):
		return contestOccurrenceID(raw, event, at)
	case raw == event.ID || raw == event.ADIFContestID:
		return importedContestID(raw, at)
	default:
		return raw
	}
}

var importedCatalogOnce sync.Once
var importedCatalog []eventDefinition

// Use the timestamp only when the catalog gives an unambiguous session.
// Unknown IDs are retained. Ambiguous known multi-session imports get an
// occurrence suffix but no arbitrary session; they need operator mapping.
func importedContestID(id string, at time.Time) string {
	importedCatalogOnce.Do(func() { importedCatalog, _ = loadEventCatalog() })
	e, ok := resolveCatalogEvent(id, importedCatalog)
	if !ok || strings.Contains(id, "@") {
		return id
	}
	base := id
	if id == e.ID || id == e.ADIFContestID {
		base = e.ID
		switch {
		case len(e.Sessions) == 1:
			base += "-" + e.Sessions[0].ID
		case e.ID == "CWT":
			clock := at.UTC().Format("1500")
			for _, s := range e.Sessions {
				if s.ID == clock {
					base += "-" + s.ID
					break
				}
			}
		case e.ID == "CW-OPEN":
			h := at.UTC().Hour()
			switch {
			case h < 4:
				base += "-1"
			case h >= 12 && h < 16:
				base += "-2"
			case h >= 20:
				base += "-3"
			}
		case e.ID == "K1USN-SST":
			if at.UTC().Weekday() == time.Monday {
				base += "-MON"
			} else if at.UTC().Weekday() == time.Friday {
				base += "-FRI"
			}
		}
	}
	return contestOccurrenceID(base, e, at)
}

func resolveCatalogEvent(id string, events []eventDefinition) (eventDefinition, bool) {
	id, _, _ = strings.Cut(strings.TrimSpace(id), "@")
	if id == "" {
		return eventDefinition{}, false
	}
	var best eventDefinition
	for _, event := range events {
		if (id == event.ID || strings.HasPrefix(id, event.ID+"-") || id == event.ADIFContestID) && len(event.ID) > len(best.ID) {
			best = event
		}
	}
	return best, best.ID != ""
}

func (s *store) migrateContestOccurrences() error {
	events, err := loadEventCatalog()
	if err != nil {
		return err
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	rows, err := tx.Query(`SELECT id,contest_id,qso_date,time_on FROM qso WHERE COALESCE(contest_id,'') != '' AND instr(contest_id,'@') = 0`)
	if err != nil {
		return err
	}
	type change struct {
		id      int64
		contest string
	}
	var changes []change
	for rows.Next() {
		var id int64
		var contest, date, clock string
		if err := rows.Scan(&id, &contest, &date, &clock); err != nil {
			rows.Close()
			return err
		}
		_, ok := resolveCatalogEvent(contest, events)
		if !ok {
			continue
		}
		at, err := time.Parse("20060102150405", date+clock)
		if err != nil {
			continue
		}
		changes = append(changes, change{id, importedContestID(contest, at)})
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	for _, change := range changes {
		if _, err := tx.Exec(`UPDATE qso SET contest_id=? WHERE id=?`, change.contest, change.id); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *store) resumeSerial(profileID int64, contestID string) (int, error) {
	rows, err := s.db.Query(`SELECT COALESCE(stx,'') FROM qso WHERE profile_id=? AND contest_id=?`, profileID, contestID)
	if err != nil {
		return 0, err
	}
	defer rows.Close()
	next := 1
	for rows.Next() {
		var text string
		if err := rows.Scan(&text); err != nil {
			return 0, err
		}
		n, err := strconv.Atoi(text)
		if err == nil && n >= next && n < 999999999 {
			next = n + 1
		}
	}
	return next, rows.Err()
}

func (m *model) saveContestSelection() {
	if m.editingQSOID != 0 {
		return
	}
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := m.store.db.Exec(`INSERT INTO contest_selection(profile_id,contest_id,sent_exchange,updated_at) VALUES(?,?,?,?) ON CONFLICT(profile_id) DO UPDATE SET contest_id=excluded.contest_id,sent_exchange=excluded.sent_exchange,updated_at=excluded.updated_at`, m.activeStation.ID, m.contestFields[contestName].Value(), m.contestFields[contestExchangeSent].Value(), now)
	if err != nil {
		m.statusMsg = fmt.Sprintf("save contest selection: %v", err)
	}
}

// contestSelectionMaxAge bounds how long a persisted contest selection is
// trusted across restarts. Every catalog contest is a single weekend or
// shorter, recurring sessions included (contestOccurrenceID rolls those
// forward to today's slot on restore). A selection older than this was left
// behind by an operator who forgot to return to general logging when the
// contest ended — restoring it would otherwise leave dupe checks silently
// scoped to that stale contest (dupe_scope "call+band" has no time window)
// forever, rejecting a station worked in that contest as a dupe on any later
// date, even a general (non-contest) QSO a year on. See
// clearContestSelection's doc comment for the related startup-resurrection
// bug this guards against.
const contestSelectionMaxAge = 7 * 24 * time.Hour

// clearContestSelection returns to general logging: no active contest, no
// resumed serial, and the cleared selection is persisted immediately so a
// later restart honors it too — restoreContestSelection would otherwise
// resurrect the just-ended contest from contest_selection on the next
// startup, which is the "stuck in that event" bug this fixes.
func (m *model) clearContestSelection() {
	for index := range m.contestFields {
		m.contestFields[index].SetValue("")
	}
	m.contestFields[contestSerialSent].Placeholder = "001"
	m.contestFields[contestExchangeSent].Placeholder = "Sent exchange"
	m.contestFields[contestSerialRcvd].Placeholder = "001"
	m.contestFields[contestExchangeRcvd].Placeholder = "Received exchange"
	m.nextSerial = 0
	m.serialResumeError = ""
	m.contestExchangeRcvdEdited = false
	m.dupeBaselineAfter = time.Time{}
	m.exchangeChoiceFocus = -1
	m.statusMsg = "General logging — no contest selected"
	m.screen = qsoEntryScreen
	m.focusField(fieldCall)
	m.checkDupe()
	m.saveContestSelection()
}

func (m *model) restoreContestSelection() {
	var id, exchange, updatedAt string
	if err := m.store.db.QueryRow(`SELECT contest_id,sent_exchange,updated_at FROM contest_selection WHERE profile_id=?`, m.activeStation.ID).Scan(&id, &exchange, &updatedAt); err != nil {
		return
	}
	if id == "" {
		return
	}
	if at, err := time.Parse(time.RFC3339, updatedAt); err == nil && time.Since(at) > contestSelectionMaxAge {
		// The operator selected this contest more than a week ago and never
		// explicitly returned to general logging (F-key/clearContestSelection
		// would have refreshed updated_at). Don't resurrect it — see
		// contestSelectionMaxAge's doc comment for the unbounded-dupe bug
		// this avoids. An empty/pre-migration updated_at (err != nil) is
		// treated as fresh rather than stale, so upgrading doesn't
		// unexpectedly clear an active in-progress contest.
		m.clearContestSelection()
		m.statusMsg = fmt.Sprintf("contest %q selected over a week ago — returned to general logging", id)
		return
	}
	m.contestFields[contestName].SetValue(id)
	m.contestFields[contestExchangeSent].SetValue(exchange)
	if event, ok := m.eventForContestID(); ok {
		// The persisted id may be a stale occurrence from a previous
		// session (e.g. yesterday's weekly session date stamp). Roll it
		// forward to today's occurrence now, before the resumed serial is
		// ever shown to the operator — resolving it lazily at save time
		// would silently swap out a serial the operator already sent.
		id = resolveOccurrenceForNow(id, event, time.Now().UTC())
		m.contestFields[contestName].SetValue(id)
		if event.SentSerial {
			if n, err := m.store.resumeSerial(m.activeStation.ID, id); err == nil {
				m.nextSerial = n
				m.contestFields[contestSerialSent].SetValue(formatSerial(n))
			} else {
				m.serialResumeError = "cannot resume serial: " + err.Error()
			}
		}
	}
	m.rebuildContestIndex()
}
