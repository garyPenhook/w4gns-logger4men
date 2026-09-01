package main

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"
)

// exportADIF writes the active station profile's QSOs as ADIF 3 records.
func exportADIF(ctx context.Context, writer io.Writer, profileID int64, st *store) (int, error) {
	qsos, err := st.qsosForProfile(ctx, profileID)
	if err != nil {
		return 0, err
	}
	if _, err := io.WriteString(writer, "W4GNS Logger ADIF export\n<ADIF_VER:5>3.1.5<PROGRAMID:12>W4GNS Logger<EOH>\n"); err != nil {
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
		{"NAME", q.name}, {"QTH", q.qth}, {"GRIDSQUARE", q.grid}, {"STATE", q.state},
		{"SIG", potaSignal(q.potaRef)}, {"SIG_INFO", q.potaRef}, {"COMMENT", q.comment},
		{"CONTEST_ID", q.contestID}, {"STX", q.stx}, {"STX_STRING", q.stxString}, {"SRX", q.srx}, {"SRX_STRING", q.srxString},
	}
}

func writeADIFField(writer io.Writer, name, value string) error {
	if _, err := fmt.Fprintf(writer, "<%s:%d>%s", name, len([]byte(value)), value); err != nil {
		return fmt.Errorf("write ADIF %s: %w", name, err)
	}
	return nil
}

// qsosForProfile returns every QSO for one station profile in chronological order.
func (s *store) qsosForProfile(ctx context.Context, profileID int64) ([]qso, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT call, qso_date, time_on, qso_date_off, time_off, band,
		COALESCE(freq, ''), mode, rst_sent, rst_rcvd, name, qth, gridsquare, state,
		COALESCE(sig_info, ''), comment, contest_id, stx, stx_string, srx, srx_string
		FROM qso WHERE profile_id = ? ORDER BY qso_date, time_on, id`, profileID)
	if err != nil {
		return nil, fmt.Errorf("query QSOs for ADIF export: %w", err)
	}
	defer rows.Close()
	var qsos []qso
	for rows.Next() {
		var q qso
		var date, timeOn, dateOff, timeOff string
		if err := rows.Scan(&q.call, &date, &timeOn, &dateOff, &timeOff, &q.band, &q.frequency, &q.mode, &q.rstSent, &q.rstRcvd, &q.name, &q.qth, &q.grid, &q.state, &q.potaRef, &q.comment, &q.contestID, &q.stx, &q.stxString, &q.srx, &q.srxString); err != nil {
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
