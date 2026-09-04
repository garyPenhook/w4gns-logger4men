package main

import (
	"context"
	"fmt"
	"io"
	"os"
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
	"CWT":       "CWOPS-CWT",
	"CW-OPEN":   "CWOPS-CW-OPEN",
	"CQ-WW-CW":  "CQ-WW-CW",
	"CQ-160-CW": "CQ-160-CW",
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

// exportADIF writes the active station profile's QSOs as ADIF records,
// streaming one row at a time from the database rather than materializing
// the whole profile in memory first — a large log otherwise held its
// entire QSO history as a []qso for the duration of every export, including
// the shutdown backup, which must complete promptly.
func exportADIF(ctx context.Context, writer io.Writer, profileID int64, st *store) (int, error) {
	if _, err := io.WriteString(writer, "W4GNS Logger ADIF export\n<ADIF_VER:"+strconv.Itoa(len(adifVersion))+">"+adifVersion+"<PROGRAMID:12>W4GNS Logger<EOH>\n"); err != nil {
		return 0, fmt.Errorf("write ADIF header: %w", err)
	}
	count := 0
	err := st.forEachQSOForProfile(ctx, profileID, func(q qso) error {
		for _, field := range adifQSOFields(q) {
			if strings.TrimSpace(field.value) == "" {
				continue
			}
			if err := writeADIFField(writer, field.name, field.value); err != nil {
				return err
			}
		}
		if _, err := io.WriteString(writer, "<EOR>\n"); err != nil {
			return fmt.Errorf("write ADIF record terminator: %w", err)
		}
		count++
		return nil
	})
	if err != nil {
		return 0, err
	}
	return count, nil
}

// writeADIFAtomic writes profileID's full ADIF export to path, used by both
// the CLI `--export-adif` flag and the in-app Ctrl+O export. It writes to a
// temporary file in dir first and renames it into place only after a full,
// successful export — os.Create(path) alone would truncate any existing
// file at path immediately, so a failure partway through (a DB read error,
// the process being killed) would otherwise destroy it and leave nothing
// usable behind.
func writeADIFAtomic(ctx context.Context, dir, path string, profileID int64, st *store) (int, error) {
	tempFile, err := os.CreateTemp(dir, ".w4gns-export-*.adi.tmp")
	if err != nil {
		return 0, fmt.Errorf("create temporary export file: %w", err)
	}
	tempPath := tempFile.Name()
	cleanup := func() { os.Remove(tempPath) }

	count, err := exportADIF(ctx, tempFile, profileID, st)
	if err != nil {
		tempFile.Close()
		cleanup()
		return 0, err
	}
	if err := tempFile.Sync(); err != nil {
		tempFile.Close()
		cleanup()
		return 0, fmt.Errorf("sync ADIF file: %w", err)
	}
	if err := tempFile.Close(); err != nil {
		cleanup()
		return 0, fmt.Errorf("close ADIF file: %w", err)
	}
	if err := replaceFileAtomic(tempPath, path); err != nil {
		cleanup()
		return 0, fmt.Errorf("finalize ADIF export: %w", err)
	}
	return count, nil
}

// adifQSOFields lists the ADIF fields for one QSO in export order. Shared by
// the bulk ADIF export and the single-record QRZ Logbook upload so both stay
// in sync.
func adifQSOFields(q qso) []struct{ name, value string } {
	return []struct{ name, value string }{
		{"CALL", q.call}, {"QSO_DATE", q.time.UTC().Format("20060102")}, {"TIME_ON", q.time.UTC().Format("150405")},
		{"QSO_DATE_OFF", q.timeOff.UTC().Format("20060102")}, {"TIME_OFF", q.timeOff.UTC().Format("150405")},
		{"BAND", q.band}, {"FREQ", q.frequency}, {"MODE", q.mode}, {"RST_SENT", q.rstSent}, {"RST_RCVD", q.rstRcvd},
		{"NAME", asciiField(q.name)}, {"QTH", asciiField(q.qth)},
		{"GRIDSQUARE", q.grid}, {"STATE", q.state}, {"CNTY", asciiField(q.county)}, {"EMAIL", q.email},
		{"COUNTRY", q.country}, {"DXCC", q.dxccNumber}, {"CQZ", q.cqZone}, {"ITUZ", q.ituZone},
		{"SIG", potaSignal(q.potaRef)}, {"SIG_INFO", asciiField(q.potaRef)}, {"POTA_REF", q.potaRef},
		{"COMMENT", asciiField(q.comment)},
		{"CONTEST_ID", adifContestID(q.contestID)}, integerOnlyField("STX", q.stx), {"STX_STRING", q.stxString},
		integerOnlyField("SRX", q.srx), {"SRX_STRING", q.srxString},
		// OPERATOR is the operator's *callsign* per the ADIF field table; the
		// human-readable name belongs in MY_NAME. STATION_CALLSIGN already
		// covers the callsign, and this app has no separate operator-callsign
		// concept, so OPERATOR is intentionally left unset rather than
		// populated with the wrong kind of value.
		{"MY_GRIDSQUARE", q.myGridSquare}, {"STATION_CALLSIGN", q.stationCallsign}, {"MY_NAME", asciiField(q.operatorName)},
		{"MY_RIG", asciiField(q.myRig)}, {"MY_ANTENNA", asciiField(q.myAntenna)}, {"TX_PWR", q.txPower},
	}
}

// diacriticToASCII maps common Latin letters with diacritics to their plain
// ASCII base letter for asciiField's transliteration.
var diacriticToASCII = map[rune]rune{
	'á': 'a', 'à': 'a', 'â': 'a', 'ä': 'a', 'ã': 'a', 'å': 'a', 'ā': 'a', 'ă': 'a', 'ą': 'a',
	'Á': 'A', 'À': 'A', 'Â': 'A', 'Ä': 'A', 'Ã': 'A', 'Å': 'A', 'Ā': 'A', 'Ă': 'A', 'Ą': 'A',
	'é': 'e', 'è': 'e', 'ê': 'e', 'ë': 'e', 'ē': 'e', 'ĕ': 'e', 'ę': 'e',
	'É': 'E', 'È': 'E', 'Ê': 'E', 'Ë': 'E', 'Ē': 'E', 'Ĕ': 'E', 'Ę': 'E',
	'í': 'i', 'ì': 'i', 'î': 'i', 'ï': 'i', 'ī': 'i',
	'Í': 'I', 'Ì': 'I', 'Î': 'I', 'Ï': 'I', 'Ī': 'I',
	'ó': 'o', 'ò': 'o', 'ô': 'o', 'ö': 'o', 'õ': 'o', 'ø': 'o', 'ō': 'o',
	'Ó': 'O', 'Ò': 'O', 'Ô': 'O', 'Ö': 'O', 'Õ': 'O', 'Ø': 'O', 'Ō': 'O',
	'ú': 'u', 'ù': 'u', 'û': 'u', 'ü': 'u', 'ū': 'u',
	'Ú': 'U', 'Ù': 'U', 'Û': 'U', 'Ü': 'U', 'Ū': 'U',
	'ý': 'y', 'ÿ': 'y', 'Ý': 'Y',
	'ñ': 'n', 'Ñ': 'N',
	'ç': 'c', 'Ç': 'C', 'ć': 'c', 'Ć': 'C', 'č': 'c', 'Č': 'C',
	'ß': 's',
	'ł': 'l', 'Ł': 'L',
	'ś': 's', 'Ś': 'S', 'š': 's', 'Š': 'S',
	'ž': 'z', 'Ž': 'Z', 'ź': 'z', 'Ź': 'Z', 'ż': 'z', 'Ż': 'Z',
	'đ': 'd', 'Đ': 'D',
}

// asciiField makes value ADIF-compliant for an ASCII String field. ADIF
// 3.1.7 restricts the IntlString data type (and its paired "_INTL" field
// names, e.g. NAME_INTL) to ADX/XML files; a .adi file — what this exporter
// and the QRZ Logbook upload both produce — must stay ASCII. Common accented
// Latin letters are transliterated to their plain base letter; any other
// non-ASCII rune becomes "?" so the file stays valid instead of silently
// carrying an invalid field or a fabricated one.
func asciiField(value string) string {
	var b strings.Builder
	for _, r := range value {
		switch {
		case r <= unicode.MaxASCII:
			b.WriteRune(r)
		case diacriticToASCII[r] != 0:
			b.WriteRune(diacriticToASCII[r])
		default:
			b.WriteByte('?')
		}
	}
	return b.String()
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

// forEachQSOForProfile streams every QSO for one station profile in
// chronological order, invoking fn for each one instead of materializing
// them all in memory. fn's error, if any, stops iteration and is returned.
func (s *store) forEachQSOForProfile(ctx context.Context, profileID int64, fn func(qso) error) error {
	rows, err := s.db.QueryContext(ctx, `SELECT call, qso_date, time_on, COALESCE(qso_date_off, ''), COALESCE(time_off, ''), band,
		COALESCE(freq, ''), mode, COALESCE(rst_sent, ''), COALESCE(rst_rcvd, ''), COALESCE(name, ''), COALESCE(qth, ''),
		COALESCE(gridsquare, ''), COALESCE(state, ''), COALESCE(county, ''), COALESCE(email, ''), COALESCE(country, ''), COALESCE(CAST(dxcc AS TEXT), ''), COALESCE(CAST(cqz AS TEXT), ''), COALESCE(CAST(ituz AS TEXT), ''),
		COALESCE(sig_info, ''), COALESCE(comment, ''), COALESCE(contest_id, ''),
		COALESCE(stx, ''), COALESCE(stx_string, ''), COALESCE(srx, ''), COALESCE(srx_string, ''),
		COALESCE(my_gridsquare, ''), COALESCE(station_callsign, ''), COALESCE(operator_name, ''),
		COALESCE(my_rig, ''), COALESCE(my_antenna, ''), COALESCE(tx_pwr, '')
		FROM qso WHERE profile_id = ? ORDER BY qso_date, time_on, id`, profileID)
	if err != nil {
		return fmt.Errorf("query QSOs for ADIF export: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var q qso
		var date, timeOn, dateOff, timeOff string
		if err := rows.Scan(&q.call, &date, &timeOn, &dateOff, &timeOff, &q.band, &q.frequency, &q.mode, &q.rstSent, &q.rstRcvd,
			&q.name, &q.qth, &q.grid, &q.state, &q.county, &q.email, &q.country, &q.dxccNumber, &q.cqZone, &q.ituZone, &q.potaRef, &q.comment, &q.contestID,
			&q.stx, &q.stxString, &q.srx, &q.srxString,
			&q.myGridSquare, &q.stationCallsign, &q.operatorName, &q.myRig, &q.myAntenna, &q.txPower); err != nil {
			return fmt.Errorf("scan QSO for ADIF export: %w", err)
		}
		q.time, _ = time.Parse("20060102150405", date+timeOn)
		q.timeOff, _ = time.Parse("20060102150405", dateOff+timeOff)
		if q.timeOff.IsZero() {
			q.timeOff = q.time
		}
		if err := fn(q); err != nil {
			return err
		}
	}
	return rows.Err()
}

// qsosForProfile returns every QSO for one station profile in chronological
// order. Prefer forEachQSOForProfile for anything that doesn't specifically
// need every row in memory at once (e.g. ADIF export).
func (s *store) qsosForProfile(ctx context.Context, profileID int64) ([]qso, error) {
	var qsos []qso
	err := s.forEachQSOForProfile(ctx, profileID, func(q qso) error {
		qsos = append(qsos, q)
		return nil
	})
	return qsos, err
}
