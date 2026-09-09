package main

import (
	"fmt"
	"sort"
)

// awardProgress is the worked-vs-confirmed tally for one award (DXCC/WAS/
// WAZ/VUCC/IOTA), driven entirely from local data (qso + lotw_confirmation),
// so opening the stats panel never makes a network call. Needed lists the
// display labels of everything worked but not yet confirmed, sorted, so an
// operator can see what to chase next.
type awardProgress struct {
	Worked    int
	Confirmed int
	Needed    []string
}

// awardKeySet runs a "SELECT DISTINCT key, label" query scoped to profileID
// and returns it as a map for set comparison. key and label are the same
// column for awards with no secondary display name (WAS/WAZ/VUCC/IOTA); DXCC
// pairs its entity number with the country name.
func (s *store) awardKeySet(query string, profileID int64) (map[string]string, error) {
	rows, err := s.db.Query(query, profileID)
	if err != nil {
		return nil, fmt.Errorf("award query: %w", err)
	}
	defer rows.Close()
	set := make(map[string]string)
	for rows.Next() {
		var key, label string
		if err := rows.Scan(&key, &label); err != nil {
			return nil, fmt.Errorf("scan award row: %w", err)
		}
		set[key] = label
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate award rows: %w", err)
	}
	return set, nil
}

// awardProgressFor computes worked/confirmed/needed for one award from a pair
// of worked/confirmed key-set queries, both taking profileID as their sole
// parameter.
func (s *store) awardProgressFor(profileID int64, workedQuery, confirmedQuery string) (awardProgress, error) {
	worked, err := s.awardKeySet(workedQuery, profileID)
	if err != nil {
		return awardProgress{}, err
	}
	confirmed, err := s.awardKeySet(confirmedQuery, profileID)
	if err != nil {
		return awardProgress{}, err
	}
	needed := make([]string, 0, len(worked))
	for key, label := range worked {
		if _, ok := confirmed[key]; ok {
			continue
		}
		if label == "" {
			label = key
		}
		needed = append(needed, label)
	}
	sort.Strings(needed)
	return awardProgress{Worked: len(worked), Confirmed: len(confirmed), Needed: needed}, nil
}

// lotwAwardStats bundles worked-vs-confirmed progress for every award the
// stats panel displays.
type lotwAwardStats struct {
	DXCC awardProgress
	WAS  awardProgress
	WAZ  awardProgress
	VUCC awardProgress
	IOTA awardProgress
}

const (
	dxccWorkedQuery = `SELECT DISTINCT CAST(dxcc AS TEXT), TRIM(CAST(dxcc AS TEXT) || ' ' || COALESCE(country, ''))
		FROM qso WHERE profile_id = ? AND dxcc IS NOT NULL AND CAST(dxcc AS TEXT) != '0'`
	dxccConfirmedQuery = `SELECT DISTINCT CAST(q.dxcc AS TEXT), TRIM(CAST(q.dxcc AS TEXT) || ' ' || COALESCE(q.country, ''))
		FROM qso q JOIN lotw_confirmation c ON c.qso_id = q.id
		WHERE q.profile_id = ? AND q.dxcc IS NOT NULL AND CAST(q.dxcc AS TEXT) != '0'`

	wasWorkedQuery = `SELECT DISTINCT UPPER(TRIM(state)), UPPER(TRIM(state))
		FROM qso WHERE profile_id = ? AND state IS NOT NULL AND TRIM(state) != ''`
	wasConfirmedQuery = `SELECT DISTINCT UPPER(TRIM(q.state)), UPPER(TRIM(q.state))
		FROM qso q JOIN lotw_confirmation c ON c.qso_id = q.id
		WHERE q.profile_id = ? AND q.state IS NOT NULL AND TRIM(q.state) != ''`

	wazWorkedQuery = `SELECT DISTINCT CAST(cqz AS TEXT), CAST(cqz AS TEXT)
		FROM qso WHERE profile_id = ? AND cqz IS NOT NULL AND CAST(cqz AS TEXT) != '0'`
	wazConfirmedQuery = `SELECT DISTINCT CAST(q.cqz AS TEXT), CAST(q.cqz AS TEXT)
		FROM qso q JOIN lotw_confirmation c ON c.qso_id = q.id
		WHERE q.profile_id = ? AND q.cqz IS NOT NULL AND CAST(q.cqz AS TEXT) != '0'`

	// VUCC credits a 4-character grid square worked on 50 MHz and above; 6M is
	// the highest band this app's amateurBands table tracks (see
	// bandplan.go), so it is the only VHF+ filter needed here.
	vuccWorkedQuery = `SELECT DISTINCT UPPER(SUBSTR(gridsquare, 1, 4)), UPPER(SUBSTR(gridsquare, 1, 4))
		FROM qso WHERE profile_id = ? AND band = '6M' AND gridsquare IS NOT NULL AND LENGTH(TRIM(gridsquare)) >= 4`
	vuccConfirmedQuery = `SELECT DISTINCT UPPER(SUBSTR(q.gridsquare, 1, 4)), UPPER(SUBSTR(q.gridsquare, 1, 4))
		FROM qso q JOIN lotw_confirmation c ON c.qso_id = q.id
		WHERE q.profile_id = ? AND q.band = '6M' AND q.gridsquare IS NOT NULL AND LENGTH(TRIM(q.gridsquare)) >= 4`

	iotaWorkedQuery = `SELECT DISTINCT UPPER(TRIM(iota_ref)), UPPER(TRIM(iota_ref))
		FROM qso WHERE profile_id = ? AND iota_ref IS NOT NULL AND TRIM(iota_ref) != ''`
	iotaConfirmedQuery = `SELECT DISTINCT UPPER(TRIM(q.iota_ref)), UPPER(TRIM(q.iota_ref))
		FROM qso q JOIN lotw_confirmation c ON c.qso_id = q.id
		WHERE q.profile_id = ? AND q.iota_ref IS NOT NULL AND TRIM(q.iota_ref) != ''`
)

// loadLoTWAwardStats computes worked-vs-confirmed progress for every award
// the stats panel shows, entirely from local data.
func (s *store) loadLoTWAwardStats(profileID int64) (lotwAwardStats, error) {
	var stats lotwAwardStats
	var err error
	if stats.DXCC, err = s.awardProgressFor(profileID, dxccWorkedQuery, dxccConfirmedQuery); err != nil {
		return stats, err
	}
	if stats.WAS, err = s.awardProgressFor(profileID, wasWorkedQuery, wasConfirmedQuery); err != nil {
		return stats, err
	}
	if stats.WAZ, err = s.awardProgressFor(profileID, wazWorkedQuery, wazConfirmedQuery); err != nil {
		return stats, err
	}
	if stats.VUCC, err = s.awardProgressFor(profileID, vuccWorkedQuery, vuccConfirmedQuery); err != nil {
		return stats, err
	}
	if stats.IOTA, err = s.awardProgressFor(profileID, iotaWorkedQuery, iotaConfirmedQuery); err != nil {
		return stats, err
	}
	return stats, nil
}
