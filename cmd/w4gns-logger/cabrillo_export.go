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
func cabrilloHeaderLines(profile stationProfile, event eventDefinition, claimedScore int) []string {
	return []string{
		"START-OF-LOG: " + cabrilloVersion,
		"CONTEST: " + cabrilloHeaderValue(event.cabrilloToken()),
		"CALLSIGN: " + cabrilloHeaderValue(profile.Callsign),
		"CATEGORY-OPERATOR: " + cabrilloHeaderValue(cabrilloOrDefault(profile.CategoryOperator, "SINGLE-OP")),
		"CATEGORY-ASSISTED: " + cabrilloHeaderValue(cabrilloOrDefault(profile.CategoryAssisted, "NON-ASSISTED")),
		"CATEGORY-BAND: " + cabrilloCategoryBand(event.Bands),
		"CATEGORY-POWER: " + cabrilloHeaderValue(cabrilloOrDefault(profile.CategoryPower, "LOW")),
		"CATEGORY-MODE: CW",
		// The sponsor's robot recomputes the authoritative score from the QSO
		// lines and treats this as an informational claim. Events with a
		// scoring rule get a real computed total here; events without one keep
		// 0 rather than have a wrong claim look more authoritative than it is.
		"CLAIMED-SCORE: " + strconv.Itoa(claimedScore),
		"CLUB: " + cabrilloHeaderValue(profile.Club),
		"NAME: " + cabrilloHeaderValue(profile.OperatorName),
		"ADDRESS: " + cabrilloHeaderValue(profile.Address),
		"OPERATORS: " + cabrilloHeaderValue(profile.Callsign),
	}
}

// cabrilloHeaderValue strips CR/LF, other control characters, and non-ASCII
// runes from a free-text header value (callsign, club, operator name,
// address). Without this, a value containing a newline would inject a forged
// header line — or, worse, a spoofed QSO: line — into the submission. Header
// values aren't fixed-width, so unlike cabrilloText this doesn't truncate.
func cabrilloHeaderValue(value string) string {
	var b strings.Builder
	for _, r := range value {
		if r >= 0x20 && r <= 0x7e {
			b.WriteRune(r)
		}
	}
	return b.String()
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

// cabrilloText makes a field value safe for a fixed-column Cabrillo QSO
// line. Imported data reaches here unmodified, so a value carrying a CR/LF
// (which would split one QSO into two lines, or inject a forged line) or an
// over-long value (which would shift every column after it) must be
// neutralized: non-printable and non-ASCII runes are dropped, and the result
// is truncated to the column's fixed width.
func cabrilloText(value string, width int) string {
	var b strings.Builder
	for _, r := range value {
		// Keep only printable ASCII (0x20–0x7e); this drops CR, LF, tabs,
		// other control characters, and any non-ASCII rune.
		if r >= 0x20 && r <= 0x7e {
			b.WriteRune(r)
		}
	}
	cleaned := b.String()
	if len(cleaned) > width {
		cleaned = cleaned[:width]
	}
	return cleaned
}

// cabrilloQSOLine renders one Cabrillo QSO: line for the given event. callsign
// falls back to the profile's callsign when the QSO's own station-identity
// snapshot is blank (e.g. a QSO logged before Station Setup was filled in).
// The checked event.CabrilloLayout selects the QSO-line shape. The generic
// RST-bearing and exchange-only layouts cover only events explicitly marked
// ready in the catalog; an unclassified event is rejected rather than being
// exported in a format its sponsor may misparse.
func cabrilloQSOLine(q qso, profile stationProfile, event eventDefinition) (string, error) {
	if !event.cabrilloReady() {
		return "", fmt.Errorf("event %q has no verified Cabrillo QSO layout", event.ID)
	}
	freqKHz, err := cabrilloFrequencyKHz(q)
	if err != nil {
		return "", err
	}
	callSent := q.stationCallsign
	if callSent == "" {
		callSent = profile.Callsign
	}
	// Cabrillo lines are CRLF-terminated per the v3 spec; callers join lines
	// with that separator rather than each line carrying its own. Every
	// field is passed through cabrilloText to strip line-breaking/control
	// characters and enforce the column width the format string assumes.
	sentCall := cabrilloText(callSent, 13)
	sentExch := cabrilloText(cabrilloExchange(q.stx, q.stxString), 13)
	rcvdCall := cabrilloText(q.call, 13)
	rcvdExch := cabrilloText(cabrilloExchange(q.srx, q.srxString), 13)
	// The Cabrillo v3 spec's X-QSO: line type keeps a /X-flagged QSO visible
	// in the submission (an auditor can see it happened) while telling the
	// scoring committee not to count it — the export-side half of "logged
	// but unscored".
	label := "QSO:"
	if q.unscored {
		label = "X-QSO:"
	}
	switch event.CabrilloLayout {
	case "cw_exchange_only":
		return fmt.Sprintf("%s %5d CW %s %s %-13s %-13s %-13s %-13s",
			label, freqKHz,
			q.time.UTC().Format("2006-01-02"),
			q.time.UTC().Format("1504"),
			sentCall, sentExch,
			rcvdCall, rcvdExch,
		), nil
	case "cw_rst_exchange":
		return fmt.Sprintf("%s %5d CW %s %s %-13s %-3s %-13s %-13s %-3s %-13s",
			label, freqKHz,
			q.time.UTC().Format("2006-01-02"),
			q.time.UTC().Format("1504"),
			sentCall, cabrilloText(q.rstSent, 3), sentExch,
			rcvdCall, cabrilloText(q.rstRcvd, 3), rcvdExch,
		), nil
	default:
		return "", fmt.Errorf("event %q has unsupported Cabrillo QSO layout %q", event.ID, event.CabrilloLayout)
	}
}

// contestScore is one session's claimed score: PointsPerQSO awarded per unique
// (callsign, band) QSO, multiplied by the number of unique callsigns worked.
type contestScore struct {
	qsoPoints   int
	multipliers int
}

func (c contestScore) total() int { return c.qsoPoints * c.multipliers }

// computeContestScore tallies the claimed score for one contest session from
// the QSOs tagged with contestID, applying event.effectiveScoring(...) for
// profile's own station — Scoring, unless the event is side-asymmetric
// (DXScoring set) and profile's callsign resolves to a non-domestic country.
// It returns a zero score when that resolves to no scoring rule, which the
// header renders as the informational "CLAIMED-SCORE: 0". A same-band
// duplicate is counted once for points (matching CW Open's "once per band,
// per session") but its callsign still counts as a multiplier, since a dupe
// is still a callsign worked. Scoring reads the same contestState index
// (roadmap Appendix C) that will back the live analysis panels, so the
// exported CLAIMED-SCORE and whatever the UI shows can never disagree.
func computeContestScore(ctx context.Context, profile stationProfile, event eventDefinition, contestID string, st *store) (contestScore, error) {
	rules := event.effectiveScoring(stationCountry(profile.Callsign))
	if rules == nil {
		return contestScore{}, nil
	}
	state, err := buildContestState(ctx, profile.ID, profile.Callsign, contestID, st)
	if err != nil {
		return contestScore{}, err
	}
	return state.score(rules), nil
}

// exportCabrillo writes a Cabrillo v3 submission for every QSO tagged with
// contestID under the active station profile, streaming one row at a time
// from the database like exportADIF does. It makes a first pass to compute the
// claimed score for the header (contest logs are a few hundred QSOs at most,
// so the extra scan is cheap) before streaming the QSO lines.
func exportCabrillo(ctx context.Context, writer io.Writer, profile stationProfile, event eventDefinition, contestID string, st *store) (int, contestScore, error) {
	if !event.cabrilloReady() {
		return 0, contestScore{}, fmt.Errorf("event %q has no verified Cabrillo QSO layout", event.ID)
	}
	score, err := computeContestScore(ctx, profile, event, contestID, st)
	if err != nil {
		return 0, contestScore{}, err
	}
	crlf := "\r\n"
	for _, line := range cabrilloHeaderLines(profile, event, score.total()) {
		if _, err := io.WriteString(writer, line+crlf); err != nil {
			return 0, contestScore{}, fmt.Errorf("write Cabrillo header: %w", err)
		}
	}
	count := 0
	err = st.forEachQSOForContest(ctx, profile.ID, contestID, func(q qso) error {
		line, err := cabrilloQSOLine(q, profile, event)
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
		return 0, contestScore{}, err
	}
	if _, err := io.WriteString(writer, "END-OF-LOG:"+crlf); err != nil {
		return 0, contestScore{}, fmt.Errorf("write Cabrillo footer: %w", err)
	}
	return count, score, nil
}

// writeCabrilloAtomic writes the contest submission to a temporary file in
// dir and renames it into place only after a full, successful export. Opening
// path with O_TRUNC directly (as the caller used to) truncates any existing
// submission immediately, so a mid-export failure — a DB read error, an
// unexportable QSO, the process being killed — would destroy the previous,
// valid submission and leave nothing usable behind. Mirrors writeADIFAtomic.
func writeCabrilloAtomic(ctx context.Context, dir, path string, profile stationProfile, event eventDefinition, contestID string, st *store) (int, contestScore, error) {
	tempFile, err := os.CreateTemp(dir, ".w4gns-cabrillo-*.cbr.tmp")
	if err != nil {
		return 0, contestScore{}, fmt.Errorf("create temporary Cabrillo file: %w", err)
	}
	tempPath := tempFile.Name()
	cleanup := func() { os.Remove(tempPath) }

	count, score, err := exportCabrillo(ctx, tempFile, profile, event, contestID, st)
	if err != nil {
		tempFile.Close()
		cleanup()
		return 0, contestScore{}, err
	}
	if err := tempFile.Sync(); err != nil {
		tempFile.Close()
		cleanup()
		return 0, contestScore{}, fmt.Errorf("sync Cabrillo file: %w", err)
	}
	if err := tempFile.Close(); err != nil {
		cleanup()
		return 0, contestScore{}, fmt.Errorf("close Cabrillo file: %w", err)
	}
	if err := replaceFileAtomic(tempPath, path); err != nil {
		cleanup()
		return 0, contestScore{}, fmt.Errorf("finalize Cabrillo export: %w", err)
	}
	return count, score, nil
}

// forEachQSOForContest streams every QSO tagged with contestID for one
// station profile in chronological order, the same streaming shape as
// forEachQSOForProfile but scoped to a single contest submission.
func (s *store) forEachQSOForContest(ctx context.Context, profileID int64, contestID string, fn func(qso) error) error {
	rows, err := s.db.QueryContext(ctx, `SELECT call, qso_date, time_on, COALESCE(qso_date_off, ''), COALESCE(time_off, ''), band,
		COALESCE(freq, ''), mode, COALESCE(rst_sent, ''), COALESCE(rst_rcvd, ''),
		COALESCE(stx, ''), COALESCE(stx_string, ''), COALESCE(srx, ''), COALESCE(srx_string, ''),
		COALESCE(station_callsign, ''), unscored, COALESCE(country, ''),
		COALESCE(CAST(dxcc AS TEXT), ''), COALESCE(CAST(cqz AS TEXT), ''), COALESCE(CAST(ituz AS TEXT), '')
		FROM qso WHERE profile_id = ? AND contest_id = ? ORDER BY qso_date, time_on, id`, profileID, contestID)
	if err != nil {
		return fmt.Errorf("query QSOs for Cabrillo export: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var q qso
		var date, timeOn, dateOff, timeOff string
		if err := rows.Scan(&q.call, &date, &timeOn, &dateOff, &timeOff, &q.band, &q.frequency, &q.mode, &q.rstSent, &q.rstRcvd,
			&q.stx, &q.stxString, &q.srx, &q.srxString, &q.stationCallsign, &q.unscored,
			&q.country, &q.dxccNumber, &q.cqZone, &q.ituZone); err != nil {
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
