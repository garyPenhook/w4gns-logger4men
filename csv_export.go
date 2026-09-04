package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"
)

// csvField makes a value safe for one comma-separated column: control
// characters (which would split a row or inject one) are dropped, and the
// result is quoted (doubling any embedded quote) whenever it contains a
// comma or quote, per RFC 4180.
func csvField(value string) string {
	var b strings.Builder
	for _, r := range value {
		if r >= 0x20 && r != 0x7f {
			b.WriteRune(r)
		}
	}
	cleaned := b.String()
	if !strings.ContainsAny(cleaned, ",\"") {
		return cleaned
	}
	return `"` + strings.ReplaceAll(cleaned, `"`, `""`) + `"`
}

// csvRow joins fields with commas and a trailing CRLF, matching Cabrillo's
// line ending so both exports round-trip the same way through Windows tools.
func csvRow(fields ...string) string {
	escaped := make([]string, len(fields))
	for i, f := range fields {
		escaped[i] = csvField(f)
	}
	return strings.Join(escaped, ",") + "\r\n"
}

// csvHeader lists the columns exportCSV writes, in order. Kept alongside the
// writer (rather than inlined) so a test can assert the two never drift.
var csvHeader = []string{
	"Date", "Time", "Call", "Band", "Mode", "Freq(MHz)",
	"RST Sent", "Sent Exch", "RST Rcvd", "Rcvd Exch",
}

// exportCSV writes every QSO tagged with contestID under profile as a CSV
// SDCHECK-equivalent (docs/ROADMAP.md "SDCHECK parity: CSV export"), one row
// per QSO in chronological order, streamed like exportCabrillo/exportADIF
// rather than materialized in memory first. This is a plain QSO listing, not
// a scored one — a per-row point total would need the same dupe/once-per-band
// logic contestState.score() already applies for CLAIMED-SCORE, and getting
// that right per-row is more machinery than a CSV listing needs; operators
// wanting the claimed score already get it from the Cabrillo header.
func exportCSV(ctx context.Context, writer io.Writer, profile stationProfile, contestID string, st *store) (int, error) {
	if _, err := io.WriteString(writer, csvRow(csvHeader...)); err != nil {
		return 0, fmt.Errorf("write CSV header: %w", err)
	}
	count := 0
	err := st.forEachQSOForContest(ctx, profile.ID, contestID, func(q qso) error {
		row := csvRow(
			q.time.UTC().Format("2006-01-02"),
			q.time.UTC().Format("1504"),
			q.call,
			q.band,
			q.mode,
			q.frequency,
			q.rstSent,
			cabrilloExchange(q.stx, q.stxString),
			q.rstRcvd,
			cabrilloExchange(q.srx, q.srxString),
		)
		if _, err := io.WriteString(writer, row); err != nil {
			return fmt.Errorf("write CSV row: %w", err)
		}
		count++
		return nil
	})
	if err != nil {
		return 0, err
	}
	return count, nil
}

// writeCSVAtomic mirrors writeCabrilloAtomic: write to a temp file in dir,
// fsync, then rename into place, so a mid-export failure never truncates a
// previous, valid CSV export.
func writeCSVAtomic(ctx context.Context, dir, path string, profile stationProfile, contestID string, st *store) (int, error) {
	tempFile, err := os.CreateTemp(dir, ".w4gns-csv-*.csv.tmp")
	if err != nil {
		return 0, fmt.Errorf("create temporary CSV file: %w", err)
	}
	tempPath := tempFile.Name()
	cleanup := func() { os.Remove(tempPath) }

	count, err := exportCSV(ctx, tempFile, profile, contestID, st)
	if err != nil {
		tempFile.Close()
		cleanup()
		return 0, err
	}
	if err := tempFile.Sync(); err != nil {
		tempFile.Close()
		cleanup()
		return 0, fmt.Errorf("sync CSV file: %w", err)
	}
	if err := tempFile.Close(); err != nil {
		cleanup()
		return 0, fmt.Errorf("close CSV file: %w", err)
	}
	if err := os.Rename(tempPath, path); err != nil {
		cleanup()
		return 0, fmt.Errorf("finalize CSV export: %w", err)
	}
	return count, nil
}
