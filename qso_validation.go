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

// validateQSO protects the database boundary. UI validation improves the
// operator experience, but every caller must pass through this check before a
// QSO is persisted or sent to a future external service.
func validateQSO(q qso) error {
	call := strings.TrimSpace(q.call)
	if call == "" {
		return fmt.Errorf("callsign is required")
	}
	if len(call) > 20 {
		return fmt.Errorf("callsign exceeds 20 characters")
	}
	for _, r := range call {
		if !unicode.IsLetter(r) && !unicode.IsDigit(r) && r != '/' {
			return fmt.Errorf("callsign contains an unsupported character %q", r)
		}
	}
	if strings.TrimSpace(q.band) == "" {
		return fmt.Errorf("band is required")
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
