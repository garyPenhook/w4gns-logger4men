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
