package main

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"
)

type adifImportResult struct {
	Imported int
	Skipped  int
}

// importADIF parses and inserts records one at a time as parseADIRecords
// scans them, so peak memory is bounded by one importBatchSize-sized batch
// of qso values rather than every parsed record in the file (previously a
// []map[string]string per record, for the whole file, before any insert
// began). The underlying byte read is still proportional to file size:
// ADIF's explicit byte-length-prefixed fields must be read as one contiguous
// buffer to parse correctly, so this does not bound memory by file size, only
// by parsed-record overhead.
//
// A failure partway through still leaves the batches inserted so far
// committed and the rest un-imported (result.Imported reports how much
// landed); this mirrors insertQSOBatch's per-batch transaction boundaries
// and lets an operator re-run the import after fixing the file without
// losing prior progress. insertQSOBatch skips records that exactly match one
// already on file, so re-running the same file doesn't double-insert the
// batches that already landed.
func importADIF(ctx context.Context, reader io.Reader, profileID int64, st *store) (adifImportResult, error) {
	data, err := io.ReadAll(reader)
	if err != nil {
		return adifImportResult{}, fmt.Errorf("read ADIF: %w", err)
	}
	result := adifImportResult{}
	batch := make([]qso, 0, importBatchSize)
	flush := func() error {
		if len(batch) == 0 {
			return nil
		}
		n, err := st.insertQSOBatch(ctx, batch)
		result.Imported += n
		result.Skipped += len(batch) - n
		batch = batch[:0]
		return err
	}
	parseErr := parseADIRecords(data, func(record map[string]string) error {
		q, ok := qsoFromADI(record, profileID)
		if !ok {
			result.Skipped++
			return nil
		}
		batch = append(batch, q)
		if len(batch) == importBatchSize {
			return flush()
		}
		return nil
	})
	if parseErr != nil {
		return result, parseErr
	}
	if err := flush(); err != nil {
		return result, err
	}
	return result, nil
}

func qsoFromADI(record map[string]string, profileID int64) (qso, bool) {
	if !strings.EqualFold(strings.TrimSpace(record["MODE"]), "CW") {
		return qso{}, false
	}
	call, date, timeOn := strings.TrimSpace(record["CALL"]), strings.TrimSpace(record["QSO_DATE"]), strings.TrimSpace(record["TIME_ON"])
	// ADIF Time is HHMM or HHMMSS: any other length (e.g. a malformed 5-char
	// value) is not a valid time and must be rejected here, not sliced blindly
	// below, or a short string panics with a slice-out-of-range.
	if call == "" || len(date) != 8 || (len(timeOn) != 4 && len(timeOn) != 6) {
		return qso{}, false
	}
	if len(timeOn) == 4 {
		timeOn += "00"
	}
	start, err := time.Parse("20060102150405", date+timeOn)
	if err != nil {
		return qso{}, false
	}
	dateOff, timeOff := strings.TrimSpace(record["QSO_DATE_OFF"]), strings.TrimSpace(record["TIME_OFF"])
	end := start
	if dateOff != "" && len(dateOff) == 8 && (len(timeOff) == 4 || len(timeOff) == 6) {
		if len(timeOff) == 4 {
			timeOff += "00"
		}
		if parsed, err := time.Parse("20060102150405", dateOff+timeOff); err == nil && !parsed.Before(start) {
			end = parsed
		}
	}
	band := strings.ToUpper(strings.TrimSpace(record["BAND"]))
	if band == "" {
		return qso{}, false
	}
	return qso{
		call:      strings.ToUpper(call),
		band:      band,
		mode:      "CW",
		rstSent:   record["RST_SENT"],
		rstRcvd:   record["RST_RCVD"],
		frequency: strings.TrimSpace(record["FREQ"]),
		name:      strings.TrimSpace(firstNonEmpty(record["NAME_INTL"], record["NAME"])),
		qth:       strings.TrimSpace(firstNonEmpty(record["QTH_INTL"], record["QTH"])),
		grid:      strings.TrimSpace(record["GRIDSQUARE"]),
		state:     strings.TrimSpace(record["STATE"]),
		country:   strings.TrimSpace(record["COUNTRY"]),
		cqZone:    strings.TrimSpace(record["CQZ"]),
		ituZone:   strings.TrimSpace(record["ITUZ"]),
		comment:   strings.TrimSpace(firstNonEmpty(record["COMMENT_INTL"], record["COMMENT"])),
		potaRef:   adifPOTAReference(record),
		contestID: strings.TrimSpace(record["CONTEST_ID"]),
		stx:       strings.TrimSpace(record["STX"]),
		stxString: strings.TrimSpace(record["STX_STRING"]),
		srx:       strings.TrimSpace(record["SRX"]),
		exchange:  record["SRX_STRING"],
		srxString: record["SRX_STRING"],
		time:      start.UTC(),
		timeOff:   end.UTC(),
		profileID: profileID,

		myGridSquare:    strings.TrimSpace(record["MY_GRIDSQUARE"]),
		stationCallsign: strings.ToUpper(strings.TrimSpace(record["STATION_CALLSIGN"])),
		operatorName:    strings.TrimSpace(firstNonEmpty(record["OPERATOR_INTL"], record["OPERATOR"])),
		myRig:           strings.TrimSpace(firstNonEmpty(record["MY_RIG_INTL"], record["MY_RIG"])),
		myAntenna:       strings.TrimSpace(firstNonEmpty(record["MY_ANTENNA_INTL"], record["MY_ANTENNA"])),
		txPower:         strings.TrimSpace(record["TX_PWR"]),
	}, true
}

// firstNonEmpty returns the first argument with non-blank content, preferring
// an ADIF "_INTL" field (present when the source value contains non-ASCII
// text) over its plain ASCII counterpart.
func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

// adifPOTAReference prefers the modern POTA_REF field and falls back to the
// legacy SIG=POTA/SIG_INFO convention used by older exports (including this
// application's own exports prior to POTA_REF support).
func adifPOTAReference(record map[string]string) string {
	if ref := strings.TrimSpace(record["POTA_REF"]); ref != "" {
		return ref
	}
	if !strings.EqualFold(strings.TrimSpace(record["SIG"]), "POTA") {
		return ""
	}
	return strings.TrimSpace(firstNonEmpty(record["SIG_INFO_INTL"], record["SIG_INFO"]))
}

// parseADIRecords scans data for ADIF records and invokes onRecord for each
// one as soon as its <EOR> is seen, rather than accumulating every record in
// memory before the caller can act on any of them.
func parseADIRecords(data []byte, onRecord func(map[string]string) error) error {
	record := make(map[string]string)
	// ADIF permits an omitted header, so records are accepted from the first
	// field unless an explicit <EOH> resets the accumulated header fields.
	inRecords := true
	for offset := 0; offset < len(data); {
		if data[offset] != '<' {
			offset++
			continue
		}
		endTag := bytesIndexByte(data, '>', offset+1)
		if endTag < 0 {
			return fmt.Errorf("ADIF tag at byte %d is unterminated", offset)
		}
		descriptor := strings.TrimSpace(string(data[offset+1 : endTag]))
		offset = endTag + 1
		switch strings.ToUpper(descriptor) {
		case "EOH":
			inRecords = true
			record = make(map[string]string)
			continue
		case "EOR":
			if inRecords && len(record) > 0 {
				if err := onRecord(record); err != nil {
					return err
				}
				record = make(map[string]string)
			}
			continue
		}
		parts := strings.Split(descriptor, ":")
		if len(parts) < 2 {
			return fmt.Errorf("invalid ADIF field descriptor %q", descriptor)
		}
		length, err := parseADIFLength(parts[1])
		if err != nil || offset+length > len(data) {
			return fmt.Errorf("invalid ADIF field length for %q", parts[0])
		}
		if inRecords {
			record[strings.ToUpper(strings.TrimSpace(parts[0]))] = string(data[offset : offset+length])
		}
		offset += length
	}
	return nil
}

func bytesIndexByte(data []byte, want byte, start int) int {
	for i := start; i < len(data); i++ {
		if data[i] == want {
			return i
		}
	}
	return -1
}

func parseADIFLength(value string) (int, error) {
	var length int
	if _, err := fmt.Sscan(value, &length); err != nil || length < 0 {
		return 0, fmt.Errorf("invalid length")
	}
	return length, nil
}
