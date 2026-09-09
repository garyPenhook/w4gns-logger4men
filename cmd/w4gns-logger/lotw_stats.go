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

// usStateAndDCCodesSQL is a literal SQL IN-list of the 50 US state postal
// codes plus DC (folded into MD by wasWorkedQuery/wasConfirmedQuery above,
// but still needed here so a DC row passes the filter in the first place).
// Built from a fixed, non-user-controlled Go slice, not user input, so
// inlining it into the query string is safe.
const usStateAndDCCodesSQL = `'AL','AK','AZ','AR','CA','CO','CT','DE','FL','GA','HI','ID','IL','IN','IA','KS','KY','LA','ME','MD','MA','MI','MN','MS','MO','MT','NE','NV','NH','NJ','NM','NY','NC','ND','OH','OK','OR','PA','RI','SC','SD','TN','TX','UT','VT','VA','WA','WV','WI','WY','DC'`

// The *ConfirmedQuery constants read geography (dxcc/state/cqz/gridsquare/
// iota_ref) straight from lotw_confirmation, not the locally matched qso
// row: those columns are populated from the QSLing station's own confirmed
// data (requested via qso_qsldetail=yes, see buildLoTWReportURL), which is
// authoritative over this app's local QRZ/prefix-table-derived guess and can
// legitimately differ from it (e.g. a station portable in a different DXCC
// entity than its callsign prefix implies). A confirmation counts here
// whether or not it could be matched to a local QSO — an unmatched
// confirmation is still a real LoTW confirmation of that entity/state/zone.
const (
	dxccWorkedQuery = `SELECT DISTINCT CAST(dxcc AS TEXT), TRIM(CAST(dxcc AS TEXT) || ' ' || COALESCE(country, ''))
		FROM qso WHERE profile_id = ? AND dxcc IS NOT NULL AND CAST(dxcc AS TEXT) != '0'`
	dxccConfirmedQuery = `SELECT DISTINCT dxcc, TRIM(dxcc || ' ' || country)
		FROM lotw_confirmation WHERE profile_id = ? AND dxcc != '' AND dxcc != '0'`

	// WAS (Worked All States, ARRL rules at arrl.org/was) credits the 50 US
	// states, not the ADIF STATE field verbatim: STATE is a per-DXCC-entity
	// "primary administrative subdivision" code (ADIF spec ??3.6.24) reused by
	// many countries — Canadian provinces, Australian states, Russian
	// oblasts, etc. can collide with a US state's own two-letter code (e.g.
	// "AR" is both Arkansas and a European Russia oblast code), so counting
	// every non-blank STATE worldwide overcounts WAS, which is exactly the
	// bug this fixes (a real log showed 61 "confirmed" states, more than the
	// 50 that exist). The fix requires two things together, not just a state
	// allowlist: scoping to the DXCC entities WAS actually draws from --
	// mainland United States (291) plus Alaska (6) and Hawaii (110), which
	// ARRL's own DXCC FAQ confirms are separate DXCC entities yet still count
	// as 2 of the 50 states for WAS -- and restricting to the 50 real state
	// codes. The District of Columbia (DC) is folded into Maryland (MD) per
	// ARRL's WAS rules ("the District of Columbia may be counted for
	// Maryland"), so a DC contact/confirmation counts as an MD credit rather
	// than a 51st, nonexistent "state".
	wasWorkedQuery = `SELECT DISTINCT
			CASE WHEN UPPER(TRIM(state)) = 'DC' THEN 'MD' ELSE UPPER(TRIM(state)) END,
			CASE WHEN UPPER(TRIM(state)) = 'DC' THEN 'MD' ELSE UPPER(TRIM(state)) END
		FROM qso WHERE profile_id = ? AND dxcc IN (291, 6, 110)
			AND UPPER(TRIM(state)) IN (` + usStateAndDCCodesSQL + `)`
	wasConfirmedQuery = `SELECT DISTINCT
			CASE WHEN UPPER(TRIM(state)) = 'DC' THEN 'MD' ELSE UPPER(TRIM(state)) END,
			CASE WHEN UPPER(TRIM(state)) = 'DC' THEN 'MD' ELSE UPPER(TRIM(state)) END
		FROM lotw_confirmation WHERE profile_id = ? AND dxcc IN (291, 6, 110)
			AND UPPER(TRIM(state)) IN (` + usStateAndDCCodesSQL + `)`

	// WAZ (CQ's Worked All Zones) covers exactly 40 CQ zones (cq-amateur-
	// radio.com's WAZ rules, Section 1): bounding to 1-40 rejects a corrupt or
	// out-of-range cqz value (e.g. bad ADIF import data) as a new "zone"
	// instead of trusting it at face value.
	wazWorkedQuery = `SELECT DISTINCT CAST(cqz AS TEXT), CAST(cqz AS TEXT)
		FROM qso WHERE profile_id = ? AND cqz IS NOT NULL AND CAST(cqz AS INTEGER) BETWEEN 1 AND 40`
	wazConfirmedQuery = `SELECT DISTINCT cqz, cqz
		FROM lotw_confirmation WHERE profile_id = ? AND cqz != '' AND CAST(cqz AS INTEGER) BETWEEN 1 AND 40`

	// VUCC credits a 4-character grid square worked on 50 MHz and above; 6M is
	// the highest band this app's amateurBands table tracks (see
	// bandplan.go), so it is the only VHF+ filter needed here.
	vuccWorkedQuery = `SELECT DISTINCT UPPER(SUBSTR(gridsquare, 1, 4)), UPPER(SUBSTR(gridsquare, 1, 4))
		FROM qso WHERE profile_id = ? AND band = '6M' AND gridsquare IS NOT NULL AND LENGTH(TRIM(gridsquare)) >= 4`
	vuccConfirmedQuery = `SELECT DISTINCT UPPER(SUBSTR(gridsquare, 1, 4)), UPPER(SUBSTR(gridsquare, 1, 4))
		FROM lotw_confirmation WHERE profile_id = ? AND band = '6M' AND LENGTH(TRIM(gridsquare)) >= 4`

	iotaWorkedQuery = `SELECT DISTINCT UPPER(TRIM(iota_ref)), UPPER(TRIM(iota_ref))
		FROM qso WHERE profile_id = ? AND iota_ref IS NOT NULL AND TRIM(iota_ref) != ''`
	iotaConfirmedQuery = `SELECT DISTINCT UPPER(TRIM(iota_ref)), UPPER(TRIM(iota_ref))
		FROM lotw_confirmation WHERE profile_id = ? AND TRIM(iota_ref) != ''`
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
