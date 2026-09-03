package main

import (
	"fmt"
	"strings"
	"unicode"
)

// normalizeCall upper-cases and trims a callsign for comparison/lookup
// (worked-call matching, DXCC prefix lookup, spot filtering). Shared so every
// call site treats "w4gns", "W4GNS", and " W4GNS " as the same station.
func normalizeCall(call string) string {
	return strings.ToUpper(strings.TrimSpace(call))
}

// validateCallsignChars rejects characters a callsign never legitimately
// contains (only letters, digits, and "/" for portable/multi-prefix
// operation) and enforces a generous length cap. A callsign can end up sent
// as a raw line to the DX cluster's TCP connection (see connectK3LR in
// cluster.go); without this check, an embedded CR/LF or other control
// character could inject extra lines into that session.
func validateCallsignChars(call string) error {
	if len(call) > 20 {
		return fmt.Errorf("callsign exceeds 20 characters")
	}
	for _, r := range call {
		if !unicode.IsLetter(r) && !unicode.IsDigit(r) && r != '/' {
			return fmt.Errorf("callsign contains an unsupported character %q", r)
		}
	}
	return nil
}

// validateQSO protects the database boundary. UI validation improves the
// operator experience, but every caller must pass through this check before a
// QSO is persisted or sent to a future external service.
func validateQSO(q qso) error {
	call := strings.TrimSpace(q.call)
	if call == "" {
		return fmt.Errorf("callsign is required")
	}
	if err := validateCallsignChars(call); err != nil {
		return err
	}
	// The station (operator) callsign is re-emitted to ADIF as STATION_CALLSIGN
	// and, on the live path, drives the cluster login line, so it needs the
	// same character check as the worked call to keep control characters
	// (CR/LF injection) out of both. It's optional, so only checked when set.
	if sc := strings.TrimSpace(q.stationCallsign); sc != "" {
		if err := validateCallsignChars(sc); err != nil {
			return fmt.Errorf("station callsign: %w", err)
		}
	}
	if strings.TrimSpace(q.band) == "" {
		return fmt.Errorf("band is required")
	}
	// Validate the band even when no frequency is present. An imported record
	// with an unsupported band but a blank FREQ would otherwise be accepted
	// here and only fail much later at WRL upload or Cabrillo export
	// (cabrilloFrequencyKHz/uploadQSOToWRL both fall back to the band's
	// default frequency, which doesn't exist for an unknown band).
	if bandIndex(q.band) < 0 {
		return fmt.Errorf("unsupported amateur band %q", strings.TrimSpace(q.band))
	}
	if strings.TrimSpace(q.frequency) != "" {
		if err := validateBandFrequency(q.band, q.frequency); err != nil {
			return err
		}
	}
	// The grid is optional, but when present it must be a real Maidenhead
	// locator: an unchecked value (e.g. an imported GRIDSQUARE of "ZZ99" or
	// free text) would otherwise be stored, re-exported verbatim, and uploaded
	// to WRL as if it were valid.
	if strings.TrimSpace(q.grid) != "" {
		if _, err := ParseGridSquare(q.grid); err != nil {
			return fmt.Errorf("invalid grid square %q: %w", strings.TrimSpace(q.grid), err)
		}
	}
	if !strings.EqualFold(strings.TrimSpace(q.mode), cwMode) {
		return fmt.Errorf("mode must be CW")
	}
	if q.time.IsZero() {
		return fmt.Errorf("QSO time is required")
	}
	if q.timeOff.IsZero() {
		return fmt.Errorf("QSO end time is required")
	}
	if q.timeOff.Before(q.time) {
		return fmt.Errorf("QSO end time is before its start time")
	}
	return nil
}
