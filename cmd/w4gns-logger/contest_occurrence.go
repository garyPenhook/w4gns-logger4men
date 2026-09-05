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
	_, err := m.store.db.Exec(`INSERT INTO contest_selection(profile_id,contest_id,sent_exchange) VALUES(?,?,?) ON CONFLICT(profile_id) DO UPDATE SET contest_id=excluded.contest_id,sent_exchange=excluded.sent_exchange`, m.activeStation.ID, m.contestFields[contestName].Value(), m.contestFields[contestExchangeSent].Value())
	if err != nil {
		m.statusMsg = fmt.Sprintf("save contest selection: %v", err)
	}
}

func (m *model) restoreContestSelection() {
	var id, exchange string
	if err := m.store.db.QueryRow(`SELECT contest_id,sent_exchange FROM contest_selection WHERE profile_id=?`, m.activeStation.ID).Scan(&id, &exchange); err != nil {
		return
	}
	m.contestFields[contestName].SetValue(id)
	m.contestFields[contestExchangeSent].SetValue(exchange)
	if event, ok := m.eventForContestID(); ok && event.SentSerial {
		if n, err := m.store.resumeSerial(m.activeStation.ID, id); err == nil {
			m.nextSerial = n
			m.contestFields[contestSerialSent].SetValue(formatSerial(n))
		} else {
			m.serialResumeError = "cannot resume serial: " + err.Error()
		}
	}
	m.rebuildContestIndex()
}
