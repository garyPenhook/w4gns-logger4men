package main

import (
	"context"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"
	"unicode"
)

// adifVersion is the ADIF specification version this exporter targets. See
// https://www.adif.org/ for the current release.
const adifVersion = "3.1.7"

// standardContestIDs maps this application's internal, session-granular
// contest_id values (e.g. "CWT-1900", used for per-session dupe checking and
// display) onto the interoperable identifiers from the official ADIF Contest
// ID List, so exported logs match what other software and contest robots
// expect.
var standardContestIDs = map[string]string{
	"CWT":     "CWOPS-CWT",
	"CW-OPEN": "CWOPS-CW-OPEN",
}

// adifContestID maps an internal contest_id to its ADIF-standard equivalent
// when one is known, leaving session/serial tracking in the database
// untouched.
func adifContestID(internal string) string {
	for prefix, standard := range standardContestIDs {
		if internal == prefix || strings.HasPrefix(internal, prefix+"-") {
			return standard
		}
	}
	return internal
}

// exportADIF writes the active station profile's QSOs as ADIF records.
func exportADIF(ctx context.Context, writer io.Writer, profileID int64, st *store) (int, error) {
	qsos, err := st.qsosForProfile(ctx, profileID)
	if err != nil {
		return 0, err
	}
	if _, err := io.WriteString(writer, "W4GNS Logger ADIF export\n<ADIF_VER:"+strconv.Itoa(len(adifVersion))+">"+adifVersion+"<PROGRAMID:12>W4GNS Logger<EOH>\n"); err != nil {
		return 0, fmt.Errorf("write ADIF header: %w", err)
	}
	for _, q := range qsos {
		for _, field := range adifQSOFields(q) {
			if strings.TrimSpace(field.value) == "" {
				continue
			}
			if err := writeADIFField(writer, field.name, field.value); err != nil {
				return 0, err
			}
		}
		if _, err := io.WriteString(writer, "<EOR>\n"); err != nil {
			return 0, fmt.Errorf("write ADIF record terminator: %w", err)
		}
	}
	return len(qsos), nil
}

// adifQSOFields lists the ADIF fields for one QSO in export order. Shared by
// the bulk ADIF export and the single-record QRZ Logbook upload so both stay
// in sync.
func adifQSOFields(q qso) []struct{ name, value string } {
	return []struct{ name, value string }{
		{"CALL", q.call}, {"QSO_DATE", q.time.UTC().Format("20060102")}, {"TIME_ON", q.time.UTC().Format("150405")},
		{"QSO_DATE_OFF", q.timeOff.UTC().Format("20060102")}, {"TIME_OFF", q.timeOff.UTC().Format("150405")},
		{"BAND", q.band}, {"FREQ", q.frequency}, {"MODE", q.mode}, {"RST_SENT", q.rstSent}, {"RST_RCVD", q.rstRcvd},
		asciiOrIntlField("NAME", q.name), asciiOrIntlField("QTH", q.qth),
		{"GRIDSQUARE", q.grid}, {"STATE", q.state}, {"COUNTRY", q.country}, {"CQZ", q.cqZone}, {"ITUZ", q.ituZone},
		{"SIG", potaSignal(q.potaRef)}, asciiOrIntlField("SIG_INFO", q.potaRef), {"POTA_REF", q.potaRef},
		asciiOrIntlField("COMMENT", q.comment),
		{"CONTEST_ID", adifContestID(q.contestID)}, integerOnlyField("STX", q.stx), {"STX_STRING", q.stxString},
		integerOnlyField("SRX", q.srx), {"SRX_STRING", q.srxString},
		{"MY_GRIDSQUARE", q.myGridSquare}, {"STATION_CALLSIGN", q.stationCallsign}, asciiOrIntlField("OPERATOR", q.operatorName),
		asciiOrIntlField("MY_RIG", q.myRig), asciiOrIntlField("MY_ANTENNA", q.myAntenna), {"TX_PWR", q.txPower},
	}
}

// asciiOrIntlField returns the field under its normal ADIF name when value is
// pure ASCII (the ADIF String data type), or under the paired "_INTL" field
// name (the ADIF IntlString data type) when it contains non-ASCII characters
// such as accented names. Writing arbitrary UTF-8 into an ASCII String field
// is not ADIF-compliant.
func asciiOrIntlField(base, value string) struct{ name, value string } {
	if isASCII(value) {
		return struct{ name, value string }{base, value}
	}
	return struct{ name, value string }{base + "_INTL", value}
}

func isASCII(value string) bool {
	for _, r := range value {
		if r > unicode.MaxASCII {
			return false
		}
	}
	return true
}

// integerOnlyField exports STX/SRX only when the stored value is a valid
// ADIF Number (the QSO-entry UI accepts free text, but STX/SRX are typed as
// integers in the ADIF spec); non-numeric input is dropped here and remains
// available in STX_STRING/SRX_STRING, which are free-form by design.
func integerOnlyField(name, value string) struct{ name, value string } {
	if _, err := strconv.Atoi(strings.TrimSpace(value)); err != nil {
		return struct{ name, value string }{name, ""}
	}
	return struct{ name, value string }{name, strings.TrimSpace(value)}
}

func writeADIFField(writer io.Writer, name, value string) error {
	if _, err := fmt.Fprintf(writer, "<%s:%d>%s", name, len([]byte(value)), value); err != nil {
		return fmt.Errorf("write ADIF %s: %w", name, err)
	}
	return nil
}

// qsosForProfile returns every QSO for one station profile in chronological order.
func (s *store) qsosForProfile(ctx context.Context, profileID int64) ([]qso, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT call, qso_date, time_on, COALESCE(qso_date_off, ''), COALESCE(time_off, ''), band,
		COALESCE(freq, ''), mode, COALESCE(rst_sent, ''), COALESCE(rst_rcvd, ''), COALESCE(name, ''), COALESCE(qth, ''),
		COALESCE(gridsquare, ''), COALESCE(state, ''), COALESCE(country, ''), COALESCE(CAST(cqz AS TEXT), ''), COALESCE(CAST(ituz AS TEXT), ''),
		COALESCE(sig_info, ''), COALESCE(comment, ''), COALESCE(contest_id, ''),
		COALESCE(stx, ''), COALESCE(stx_string, ''), COALESCE(srx, ''), COALESCE(srx_string, ''),
		COALESCE(my_gridsquare, ''), COALESCE(station_callsign, ''), COALESCE(operator_name, ''),
		COALESCE(my_rig, ''), COALESCE(my_antenna, ''), COALESCE(tx_pwr, '')
		FROM qso WHERE profile_id = ? ORDER BY qso_date, time_on, id`, profileID)
	if err != nil {
		return nil, fmt.Errorf("query QSOs for ADIF export: %w", err)
	}
	defer rows.Close()
	var qsos []qso
	for rows.Next() {
		var q qso
		var date, timeOn, dateOff, timeOff string
		if err := rows.Scan(&q.call, &date, &timeOn, &dateOff, &timeOff, &q.band, &q.frequency, &q.mode, &q.rstSent, &q.rstRcvd,
			&q.name, &q.qth, &q.grid, &q.state, &q.country, &q.cqZone, &q.ituZone, &q.potaRef, &q.comment, &q.contestID,
			&q.stx, &q.stxString, &q.srx, &q.srxString,
			&q.myGridSquare, &q.stationCallsign, &q.operatorName, &q.myRig, &q.myAntenna, &q.txPower); err != nil {
			return nil, fmt.Errorf("scan QSO for ADIF export: %w", err)
		}
		q.time, _ = time.Parse("20060102150405", date+timeOn)
		q.timeOff, _ = time.Parse("20060102150405", dateOff+timeOff)
		if q.timeOff.IsZero() {
			q.timeOff = q.time
		}
		qsos = append(qsos, q)
	}
	return qsos, rows.Err()
}
