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

func importADIF(ctx context.Context, reader io.Reader, profileID int64, st *store) (adifImportResult, error) {
	data, err := io.ReadAll(reader)
	if err != nil {
		return adifImportResult{}, fmt.Errorf("read ADIF: %w", err)
	}
	records, err := parseADIRecords(data)
	if err != nil {
		return adifImportResult{}, err
	}
	result := adifImportResult{}
	batch := make([]qso, 0, importBatchSize)
	flush := func() error {
		if len(batch) == 0 {
			return nil
		}
		if err := st.insertQSOBatch(ctx, batch); err != nil {
			return err
		}
		result.Imported += len(batch)
		batch = batch[:0]
		return nil
	}
	for _, record := range records {
		q, ok := qsoFromADI(record, profileID)
		if !ok {
			result.Skipped++
			continue
		}
		batch = append(batch, q)
		if len(batch) == importBatchSize {
			if err := flush(); err != nil {
				return result, err
			}
		}
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
	if call == "" || len(date) != 8 || len(timeOn) < 4 {
		return qso{}, false
	}
	if len(timeOn) == 4 {
		timeOn += "00"
	}
	start, err := time.Parse("20060102150405", date+timeOn[:6])
	if err != nil {
		return qso{}, false
	}
	dateOff, timeOff := strings.TrimSpace(record["QSO_DATE_OFF"]), strings.TrimSpace(record["TIME_OFF"])
	end := start
	if dateOff != "" && len(dateOff) == 8 && len(timeOff) >= 4 {
		if len(timeOff) == 4 {
			timeOff += "00"
		}
		if parsed, err := time.Parse("20060102150405", dateOff+timeOff[:6]); err == nil && !parsed.Before(start) {
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
		exchange:  record["SRX_STRING"],
		srxString: record["SRX_STRING"],
		time:      start.UTC(),
		timeOff:   end.UTC(),
		profileID: profileID,
	}, true
}

func parseADIRecords(data []byte) ([]map[string]string, error) {
	var records []map[string]string
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
			return nil, fmt.Errorf("ADIF tag at byte %d is unterminated", offset)
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
				records = append(records, record)
				record = make(map[string]string)
			}
			continue
		}
		parts := strings.Split(descriptor, ":")
		if len(parts) < 2 {
			return nil, fmt.Errorf("invalid ADIF field descriptor %q", descriptor)
		}
		length, err := parseADIFLength(parts[1])
		if err != nil || offset+length > len(data) {
			return nil, fmt.Errorf("invalid ADIF field length for %q", parts[0])
		}
		if inRecords {
			record[strings.ToUpper(strings.TrimSpace(parts[0]))] = string(data[offset : offset+length])
		}
		offset += length
	}
	return records, nil
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
