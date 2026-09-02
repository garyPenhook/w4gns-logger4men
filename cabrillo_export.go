package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// cabrilloVersion is the Cabrillo specification version this exporter
// targets. See https://www.cabrillo.org/ for the current release.
const cabrilloVersion = "3.0"

// defaultDownloadsDir resolves the operator's Downloads folder, the
// conventional place a desktop OS surfaces newly created files for the
// operator to find and upload. $HOME/Downloads matches Windows, macOS, and
// most Linux desktops without needing XDG user-dirs parsing.
func defaultDownloadsDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("locate home directory: %w", err)
	}
	return filepath.Join(home, "Downloads"), nil
}

// sanitizeFilenameComponent strips everything but a conservative safe
// charset from a value bound for a filename. The Contest field is a free
// text input (an operator can type or paste anything, not just catalog
// event IDs), so contestID reaching cabrilloExportCmd's output path is not
// trustworthy: a value like "CWT-../../../tmp/evil" survives the "starts
// with a known event ID" check in eventForContestID but resolves (via
// filepath.Join's Clean) to a path outside the Downloads folder entirely.
func sanitizeFilenameComponent(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch {
		case r >= 'A' && r <= 'Z', r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == '-', r == '_':
			b.WriteRune(r)
		default:
			b.WriteRune('_')
		}
	}
	if b.Len() == 0 {
		return "LOG"
	}
	return b.String()
}

// cabrilloOrDefault falls back to def when value is blank, so an operator
// who never filled in a category field on Station Setup still gets a valid
// Cabrillo header instead of an empty (invalid) one.
func cabrilloOrDefault(value, def string) string {
	if strings.TrimSpace(value) == "" {
		return def
	}
	return value
}

// cabrilloCategoryBand reports the Cabrillo CATEGORY-BAND token for an
// event. A single-band event reports that band directly (this app's band
// names, e.g. "20M", already match Cabrillo's band tokens); anything else
// falls back to "ALL", which is always valid even when it undersells a
// contest that scores per-band.
func cabrilloCategoryBand(bands []string) string {
	if len(bands) == 1 {
		return strings.ToUpper(strings.TrimSpace(bands[0]))
	}
	return "ALL"
}

// cabrilloHeaderLines renders a Cabrillo v3 header for one contest
// submission. contestID is the event's own identifier (event.ID); Cabrillo's
// CONTEST: token is a fixed vocabulary defined by each contest sponsor, so
// this is a best-effort default the operator may need to edit before
// uploading if it doesn't match the sponsor's exact expected value.
func cabrilloHeaderLines(profile stationProfile, event eventDefinition) []string {
	return []string{
		"START-OF-LOG: " + cabrilloVersion,
		"CONTEST: " + event.ID,
		"CALLSIGN: " + profile.Callsign,
		"CATEGORY-OPERATOR: " + cabrilloOrDefault(profile.CategoryOperator, "SINGLE-OP"),
		"CATEGORY-ASSISTED: " + cabrilloOrDefault(profile.CategoryAssisted, "NON-ASSISTED"),
		"CATEGORY-BAND: " + cabrilloCategoryBand(event.Bands),
		"CATEGORY-POWER: " + cabrilloOrDefault(profile.CategoryPower, "LOW"),
		"CATEGORY-MODE: CW",
		// 0 rather than a computed total: contest robots recompute the score
		// from the QSO lines themselves and treat this as informational, so
		// an admittedly-approximate claim isn't worth the risk of a wrong
		// one appearing more authoritative than it is.
		"CLAIMED-SCORE: 0",
		"CLUB: " + profile.Club,
		"NAME: " + profile.OperatorName,
		"ADDRESS: " + profile.Address,
		"OPERATORS: " + profile.Callsign,
	}
}

// cabrilloFrequencyKHz resolves a QSO's frequency in whole kHz, the unit
// Cabrillo's QSO: line requires, falling back to the band's default
// frequency the same way uploadQSOToWRL does when the QSO's own frequency
// field was left blank.
func cabrilloFrequencyKHz(q qso) (int, error) {
	frequency := strings.TrimSpace(q.frequency)
	if frequency == "" {
		if index := bandIndex(q.band); index >= 0 {
			frequency = amateurBands[index].DefaultMHz
		}
	}
	mhz, err := strconv.ParseFloat(frequency, 64)
	if err != nil {
		return 0, fmt.Errorf("QSO frequency %q is not a number: %w", q.frequency, err)
	}
	return int(mhz*1000 + 0.5), nil
}

// cabrilloExchange joins a serial number with its free-text exchange, the
// shape both this app's sent/received exchange fields and most contests'
// Cabrillo exchanges take (e.g. "001 CA").
func cabrilloExchange(serial, text string) string {
	serial, text = strings.TrimSpace(serial), strings.TrimSpace(text)
	switch {
	case serial == "":
		return text
	case text == "":
		return serial
	default:
		return serial + " " + text
	}
}

// cabrilloQSOLine renders one Cabrillo QSO: line. callsign falls back to the
// profile's callsign when the QSO's own station-identity snapshot is blank
// (e.g. a QSO logged before Station Setup was filled in).
func cabrilloQSOLine(q qso, profile stationProfile) (string, error) {
	freqKHz, err := cabrilloFrequencyKHz(q)
	if err != nil {
		return "", err
	}
	callSent := q.stationCallsign
	if callSent == "" {
		callSent = profile.Callsign
	}
	// Cabrillo lines are CRLF-terminated per the v3 spec; callers join lines
	// with that separator rather than each line carrying its own.
	return fmt.Sprintf("QSO: %5d CW %s %s %-13s %-3s %-13s %-13s %-3s %-13s",
		freqKHz,
		q.time.UTC().Format("2006-01-02"),
		q.time.UTC().Format("1504"),
		callSent, q.rstSent, cabrilloExchange(q.stx, q.stxString),
		q.call, q.rstRcvd, cabrilloExchange(q.srx, q.srxString),
	), nil
}

// exportCabrillo writes a Cabrillo v3 submission for every QSO tagged with
// contestID under the active station profile, streaming one row at a time
// from the database like exportADIF does.
func exportCabrillo(ctx context.Context, writer io.Writer, profile stationProfile, event eventDefinition, contestID string, st *store) (int, error) {
	crlf := "\r\n"
	for _, line := range cabrilloHeaderLines(profile, event) {
		if _, err := io.WriteString(writer, line+crlf); err != nil {
			return 0, fmt.Errorf("write Cabrillo header: %w", err)
		}
	}
	count := 0
	err := st.forEachQSOForContest(ctx, profile.ID, contestID, func(q qso) error {
		line, err := cabrilloQSOLine(q, profile)
		if err != nil {
			return err
		}
		if _, err := io.WriteString(writer, line+crlf); err != nil {
			return fmt.Errorf("write Cabrillo QSO line: %w", err)
		}
		count++
		return nil
	})
	if err != nil {
		return 0, err
	}
	if _, err := io.WriteString(writer, "END-OF-LOG:"+crlf); err != nil {
		return 0, fmt.Errorf("write Cabrillo footer: %w", err)
	}
	return count, nil
}

// forEachQSOForContest streams every QSO tagged with contestID for one
// station profile in chronological order, the same streaming shape as
// forEachQSOForProfile but scoped to a single contest submission.
func (s *store) forEachQSOForContest(ctx context.Context, profileID int64, contestID string, fn func(qso) error) error {
	rows, err := s.db.QueryContext(ctx, `SELECT call, qso_date, time_on, COALESCE(qso_date_off, ''), COALESCE(time_off, ''), band,
		COALESCE(freq, ''), mode, COALESCE(rst_sent, ''), COALESCE(rst_rcvd, ''),
		COALESCE(stx, ''), COALESCE(stx_string, ''), COALESCE(srx, ''), COALESCE(srx_string, ''),
		COALESCE(station_callsign, '')
		FROM qso WHERE profile_id = ? AND contest_id = ? ORDER BY qso_date, time_on, id`, profileID, contestID)
	if err != nil {
		return fmt.Errorf("query QSOs for Cabrillo export: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var q qso
		var date, timeOn, dateOff, timeOff string
		if err := rows.Scan(&q.call, &date, &timeOn, &dateOff, &timeOff, &q.band, &q.frequency, &q.mode, &q.rstSent, &q.rstRcvd,
			&q.stx, &q.stxString, &q.srx, &q.srxString, &q.stationCallsign); err != nil {
			return fmt.Errorf("scan QSO for Cabrillo export: %w", err)
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
