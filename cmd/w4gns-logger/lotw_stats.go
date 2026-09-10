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
// and stationCallsign and returns it as a map for set comparison. key and
// label are the same column for awards with no secondary display name (WAS/
// WAZ/VUCC/IOTA); DXCC pairs its entity number with the country name.
func (s *store) awardKeySet(query string, profileID int64, stationCallsign string) (map[string]string, error) {
	rows, err := s.db.Query(query, profileID, stationCallsign)
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
// of worked/confirmed key-set queries, each taking (profileID,
// stationCallsign) as its parameters. stationCallsign scopes both queries to
// rows whose own station_callsign either matches it or is blank/unset (older
// QSOs logged before that column existed): sponsor award rules (DXCC/WAS/WAZ/
// VUCC/IOTA) require all contributing contacts to come from one operating
// identity, so a profile that has logged under more than one callsign must
// not have those callsigns' totals silently combined.
func (s *store) awardProgressFor(profileID int64, stationCallsign, workedQuery, confirmedQuery string) (awardProgress, error) {
	worked, err := s.awardKeySet(workedQuery, profileID, stationCallsign)
	if err != nil {
		return awardProgress{}, err
	}
	confirmed, err := s.awardKeySet(confirmedQuery, profileID, stationCallsign)
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
// entity than its callsign prefix implies). Each still joins to its matched
// qso row (qso_id IS NOT NULL, enforced by the join) for two reasons: the
// README documents that an unmatched confirmation (e.g. for a QSO logged
// elsewhere, or under a different callsign LoTW happens to report) is
// recorded but not counted toward these totals, and the join is also how
// stationCallsignFilter reaches the confirmation's own operating identity,
// since lotw_confirmation itself has no station_callsign column.
//
// stationCallsignFilter is appended to both the worked and confirmed queries
// (against qso.station_callsign directly, or q.station_callsign through the
// join) to scope every award to one operating identity — see
// awardProgressFor's comment for why that matters for sponsor rule
// compliance. A blank/NULL station_callsign (QSOs logged before that column
// existed) is always included rather than excluded, so pre-existing logs
// don't silently lose their award progress.
const stationCallsignFilter = `(%[1]s.station_callsign IS NULL OR TRIM(%[1]s.station_callsign) = '' OR UPPER(TRIM(%[1]s.station_callsign)) = UPPER(TRIM(?)))`

// iotaReferenceFilterSQL restricts an already upper-cased/trimmed IOTA
// reference column expression (%[1]s) to the standard "AA-###"
// continent/sequence form (matching iotaReferenceCode in iota.go): a
// two-letter continent code from a fixed list, a literal hyphen, and three
// digits. iota_ref is stored from imported ADIF data with no format check
// (adif_import.go), so an unchecked free-text value would otherwise count as
// a spurious distinct IOTA entity here — filtering it out of the award
// query, rather than rejecting it at the database boundary, keeps a
// malformed value from blocking an otherwise-unrelated edit to the same row
// (see validateQSO, which intentionally does not enforce this format).
const iotaReferenceFilterSQL = `LENGTH(%[1]s) = 6 AND SUBSTR(%[1]s, 3, 1) = '-' AND SUBSTR(%[1]s, 1, 2) IN ('AF','AN','AS','EU','NA','OC','SA') AND SUBSTR(%[1]s, 4, 3) GLOB '[0-9][0-9][0-9]'`

var (
	dxccWorkedQuery = `SELECT DISTINCT CAST(dxcc AS TEXT), TRIM(CAST(dxcc AS TEXT) || ' ' || COALESCE(country, ''))
		FROM qso WHERE profile_id = ? AND dxcc IS NOT NULL AND CAST(dxcc AS TEXT) != '0'
		AND ` + fmt.Sprintf(stationCallsignFilter, "qso")
	dxccConfirmedQuery = `SELECT DISTINCT lc.dxcc, TRIM(lc.dxcc || ' ' || lc.country)
		FROM lotw_confirmation lc JOIN qso q ON q.id = lc.qso_id
		WHERE lc.profile_id = ? AND lc.dxcc != '' AND lc.dxcc != '0'
		AND ` + fmt.Sprintf(stationCallsignFilter, "q")

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
	// 60M is excluded: ARRL's WAS rules (arrl.org/was) don't include the 60m
	// channelized allocation in the general award, even though this app
	// supports logging on it (see bandplan.go).
	wasWorkedQuery = `SELECT DISTINCT
			CASE WHEN UPPER(TRIM(state)) = 'DC' THEN 'MD' ELSE UPPER(TRIM(state)) END,
			CASE WHEN UPPER(TRIM(state)) = 'DC' THEN 'MD' ELSE UPPER(TRIM(state)) END
		FROM qso WHERE profile_id = ? AND dxcc IN (291, 6, 110) AND UPPER(TRIM(band)) != '60M'
			AND UPPER(TRIM(state)) IN (` + usStateAndDCCodesSQL + `)
			AND ` + fmt.Sprintf(stationCallsignFilter, "qso")
	wasConfirmedQuery = `SELECT DISTINCT
			CASE WHEN UPPER(TRIM(lc.state)) = 'DC' THEN 'MD' ELSE UPPER(TRIM(lc.state)) END,
			CASE WHEN UPPER(TRIM(lc.state)) = 'DC' THEN 'MD' ELSE UPPER(TRIM(lc.state)) END
		FROM lotw_confirmation lc JOIN qso q ON q.id = lc.qso_id
		WHERE lc.profile_id = ? AND lc.dxcc IN ('291', '6', '110') AND UPPER(TRIM(lc.band)) != '60M'
			AND UPPER(TRIM(lc.state)) IN (` + usStateAndDCCodesSQL + `)
			AND ` + fmt.Sprintf(stationCallsignFilter, "q")

	// WAZ (CQ's Worked All Zones) covers exactly 40 CQ zones (cq-amateur-
	// radio.com's WAZ rules, Section 1): bounding to 1-40 rejects a corrupt or
	// out-of-range cqz value (e.g. bad ADIF import data) as a new "zone"
	// instead of trusting it at face value.
	wazWorkedQuery = `SELECT DISTINCT CAST(cqz AS TEXT), CAST(cqz AS TEXT)
		FROM qso WHERE profile_id = ? AND cqz IS NOT NULL AND CAST(cqz AS INTEGER) BETWEEN 1 AND 40
			AND ` + fmt.Sprintf(stationCallsignFilter, "qso")
	wazConfirmedQuery = `SELECT DISTINCT CAST(CAST(lc.cqz AS INTEGER) AS TEXT), CAST(CAST(lc.cqz AS INTEGER) AS TEXT)
		FROM lotw_confirmation lc JOIN qso q ON q.id = lc.qso_id
		WHERE lc.profile_id = ? AND lc.cqz != '' AND CAST(lc.cqz AS INTEGER) BETWEEN 1 AND 40
			AND ` + fmt.Sprintf(stationCallsignFilter, "q")

	// VUCC credits a 4-character grid square worked on 50 MHz and above; 6M is
	// the highest band this app's amateurBands table tracks (see
	// bandplan.go), so it is the only VHF+ filter needed here.
	vuccWorkedQuery = `SELECT DISTINCT UPPER(SUBSTR(gridsquare, 1, 4)), UPPER(SUBSTR(gridsquare, 1, 4))
		FROM qso WHERE profile_id = ? AND band = '6M' AND gridsquare IS NOT NULL AND LENGTH(TRIM(gridsquare)) >= 4
			AND ` + fmt.Sprintf(stationCallsignFilter, "qso")
	vuccConfirmedQuery = `SELECT DISTINCT UPPER(SUBSTR(lc.gridsquare, 1, 4)), UPPER(SUBSTR(lc.gridsquare, 1, 4))
		FROM lotw_confirmation lc JOIN qso q ON q.id = lc.qso_id
		WHERE lc.profile_id = ? AND lc.band = '6M' AND LENGTH(TRIM(lc.gridsquare)) >= 4
			AND ` + fmt.Sprintf(stationCallsignFilter, "q")

	iotaWorkedQuery = `SELECT DISTINCT UPPER(TRIM(iota_ref)), UPPER(TRIM(iota_ref))
		FROM qso WHERE profile_id = ? AND iota_ref IS NOT NULL AND TRIM(iota_ref) != ''
			AND ` + fmt.Sprintf(iotaReferenceFilterSQL, "UPPER(TRIM(iota_ref))") + `
			AND ` + fmt.Sprintf(stationCallsignFilter, "qso")
	iotaConfirmedQuery = `SELECT DISTINCT UPPER(TRIM(lc.iota_ref)), UPPER(TRIM(lc.iota_ref))
		FROM lotw_confirmation lc JOIN qso q ON q.id = lc.qso_id
		WHERE lc.profile_id = ? AND TRIM(lc.iota_ref) != ''
			AND ` + fmt.Sprintf(iotaReferenceFilterSQL, "UPPER(TRIM(lc.iota_ref))") + `
			AND ` + fmt.Sprintf(stationCallsignFilter, "q")
)

// loadLoTWAwardStats computes worked-vs-confirmed progress for every award
// the stats panel shows, entirely from local data. stationCallsign is the
// active profile's own callsign, used to scope every award to one operating
// identity (see stationCallsignFilter).
func (s *store) loadLoTWAwardStats(profileID int64, stationCallsign string) (lotwAwardStats, error) {
	var stats lotwAwardStats
	var err error
	if stats.DXCC, err = s.awardProgressFor(profileID, stationCallsign, dxccWorkedQuery, dxccConfirmedQuery); err != nil {
		return stats, err
	}
	if stats.WAS, err = s.awardProgressFor(profileID, stationCallsign, wasWorkedQuery, wasConfirmedQuery); err != nil {
		return stats, err
	}
	if stats.WAZ, err = s.awardProgressFor(profileID, stationCallsign, wazWorkedQuery, wazConfirmedQuery); err != nil {
		return stats, err
	}
	if stats.VUCC, err = s.awardProgressFor(profileID, stationCallsign, vuccWorkedQuery, vuccConfirmedQuery); err != nil {
		return stats, err
	}
	if stats.IOTA, err = s.awardProgressFor(profileID, stationCallsign, iotaWorkedQuery, iotaConfirmedQuery); err != nil {
		return stats, err
	}
	return stats, nil
}
