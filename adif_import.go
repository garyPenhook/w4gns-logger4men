package main

import (
	"bufio"
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

// importADIF streams reader through parseADIRecords and inserts records one
// batch at a time, so peak memory is bounded by one importBatchSize-sized
// batch of qso values plus a small read buffer — not by the size of the
// source file, which is never read into memory all at once.
//
// A failure partway through still leaves the batches inserted so far
// committed and the rest un-imported (result.Imported reports how much
// landed); this mirrors insertQSOBatch's per-batch transaction boundaries
// and lets an operator re-run the import after fixing the file without
// losing prior progress. insertQSOBatch skips records that exactly match one
// already on file, so re-running the same file doesn't double-insert the
// batches that already landed.
func importADIF(ctx context.Context, reader io.Reader, profileID int64, st *store) (adifImportResult, error) {
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
	parseErr := parseADIRecords(reader, func(record map[string]string) error {
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
		call:       strings.ToUpper(call),
		band:       band,
		mode:       "CW",
		rstSent:    record["RST_SENT"],
		rstRcvd:    record["RST_RCVD"],
		frequency:  strings.TrimSpace(record["FREQ"]),
		name:       strings.TrimSpace(firstNonEmpty(record["NAME_INTL"], record["NAME"])),
		qth:        strings.TrimSpace(firstNonEmpty(record["QTH_INTL"], record["QTH"])),
		grid:       strings.TrimSpace(record["GRIDSQUARE"]),
		state:      strings.TrimSpace(record["STATE"]),
		country:    strings.TrimSpace(record["COUNTRY"]),
		dxccNumber: strings.TrimSpace(record["DXCC"]),
		cqZone:     strings.TrimSpace(record["CQZ"]),
		ituZone:    strings.TrimSpace(record["ITUZ"]),
		comment:    strings.TrimSpace(firstNonEmpty(record["COMMENT_INTL"], record["COMMENT"])),
		potaRef:    adifPOTAReference(record),
		contestID:  strings.TrimSpace(record["CONTEST_ID"]),
		stx:        strings.TrimSpace(record["STX"]),
		stxString:  strings.TrimSpace(record["STX_STRING"]),
		srx:        strings.TrimSpace(record["SRX"]),
		exchange:   record["SRX_STRING"],
		srxString:  record["SRX_STRING"],
		time:       start.UTC(),
		timeOff:    end.UTC(),
		profileID:  profileID,

		myGridSquare:    strings.TrimSpace(record["MY_GRIDSQUARE"]),
		stationCallsign: strings.ToUpper(strings.TrimSpace(record["STATION_CALLSIGN"])),
		// MY_NAME is the logging operator's name per the ADIF field table;
		// OPERATOR means the operator's *callsign*. OPERATOR/OPERATOR_INTL
		// are still accepted as a fallback for logs this app exported before
		// that distinction was fixed here, which wrote the name into OPERATOR.
		operatorName: strings.TrimSpace(firstNonEmpty(record["MY_NAME_INTL"], record["MY_NAME"], record["OPERATOR_INTL"], record["OPERATOR"])),
		myRig:        strings.TrimSpace(firstNonEmpty(record["MY_RIG_INTL"], record["MY_RIG"])),
		myAntenna:    strings.TrimSpace(firstNonEmpty(record["MY_ANTENNA_INTL"], record["MY_ANTENNA"])),
		txPower:      strings.TrimSpace(record["TX_PWR"]),
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

// Bounds on a single ADIF field/tag, generous relative to any real ADIF
// record but well short of what an attacker could use to exhaust memory: a
// declared field length like <CALL:1000000000> would otherwise attempt a
// ~1GB allocation, and an unterminated or field-less blob of input would
// otherwise buffer without limit while scanning for '<' or '>'.
const (
	maxADIFTagLength       = 256      // "FIELDNAME:12345:T" is a handful of bytes in practice
	maxADIFFieldBytes      = 10 << 20 // 10 MiB; far larger than any real ADIF field
	maxADIFFieldsPerRecord = 2000     // a real ADIF record has a few dozen fields at most
)

// parseADIRecords streams reader for ADIF records and invokes onRecord for
// each one as soon as its <EOR> is seen. Memory use is bounded by one
// buffered tag/field at a time (plus whatever onRecord itself retains, e.g.
// importADIF's batch), not by the size of the source file: ADIF's explicit
// byte-length-prefixed fields let each field be read with a single
// io.ReadFull instead of requiring the whole file in memory up front. See
// the maxADIF* constants for the per-field/per-record limits that keep a
// malformed or hostile file from exhausting memory despite the streaming
// design.
func parseADIRecords(reader io.Reader, onRecord func(map[string]string) error) error {
	br := bufio.NewReaderSize(reader, 64*1024)
	record := make(map[string]string)
	// ADIF permits an omitted header, so records are accepted from the first
	// field unless an explicit <EOH> resets the accumulated header fields.
	inRecords := true
	for {
		if _, err := readUntil(br, '<', maxADIFTagLength); err != nil {
			if err == io.EOF {
				if inRecords && len(record) > 0 {
					// Fields were parsed but the file ended before a
					// closing <EOR>: a truncated download or a file cut
					// off mid-write. Silently succeeding here would drop
					// the trailing record with no indication it ever
					// existed.
					return fmt.Errorf("ADIF file ends with an unterminated record (missing <EOR>)")
				}
				return nil
			}
			return fmt.Errorf("read ADIF: %w", err)
		}
		tag, err := readUntil(br, '>', maxADIFTagLength)
		if err != nil {
			return fmt.Errorf("ADIF tag is unterminated or exceeds %d bytes: %w", maxADIFTagLength, err)
		}
		descriptor := strings.TrimSpace(string(tag[:len(tag)-1]))
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
		if err != nil {
			return fmt.Errorf("invalid ADIF field length for %q", parts[0])
		}
		if length > maxADIFFieldBytes {
			return fmt.Errorf("ADIF field %q declares length %d, exceeding the %d-byte limit", parts[0], length, maxADIFFieldBytes)
		}
		value := make([]byte, length)
		if _, err := io.ReadFull(br, value); err != nil {
			return fmt.Errorf("invalid ADIF field length for %q", parts[0])
		}
		if inRecords {
			if len(record) >= maxADIFFieldsPerRecord {
				return fmt.Errorf("ADIF record exceeds %d fields", maxADIFFieldsPerRecord)
			}
			record[strings.ToUpper(strings.TrimSpace(parts[0]))] = string(value)
		}
	}
}

// readUntil reads from br up to and including delim, erroring out if delim
// isn't found within max bytes. bufio.Reader.ReadBytes has no such limit
// and will buffer arbitrarily large input (e.g. a huge file with no '<' at
// all, or a single absurdly long tag), which readUntil exists to prevent.
func readUntil(br *bufio.Reader, delim byte, max int) ([]byte, error) {
	buf := make([]byte, 0, 32)
	for {
		b, err := br.ReadByte()
		if err != nil {
			return buf, err
		}
		buf = append(buf, b)
		if b == delim {
			return buf, nil
		}
		if len(buf) >= max {
			return buf, fmt.Errorf("no %q found within %d bytes", delim, max)
		}
	}
}

func parseADIFLength(value string) (int, error) {
	var length int
	if _, err := fmt.Sscan(value, &length); err != nil || length < 0 {
		return 0, fmt.Errorf("invalid length")
	}
	return length, nil
}
