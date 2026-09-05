package main

import (
	"fmt"
	"strings"
)

// countyCode uses only the exchanged code and this event's county table.
// Names are entry suggestions; arbitrary extra exchange tokens must not earn
// a multiplier. Serial numbers belong in the existing separate serial field.
func (e eventDefinition) countyCode(text string) string {
	code := strings.ToUpper(strings.TrimSpace(text))
	for _, option := range e.CountyOptions {
		if code == option.Code {
			return code
		}
	}
	return ""
}

func (e *eventDefinition) prepareCountyOptions() error {
	seen := make(map[string]bool)
	for i := range e.CountyOptions {
		option := &e.CountyOptions[i]
		option.Code = strings.ToUpper(strings.TrimSpace(option.Code))
		if option.Code == "" || seen[option.Code] {
			return fmt.Errorf("event %q has empty or duplicate county code %q", e.ID, option.Code)
		}
		for _, ch := range option.Code {
			if (ch < 'A' || ch > 'Z') && (ch < '0' || ch > '9') {
				return fmt.Errorf("event %q has invalid county code %q", e.ID, option.Code)
			}
		}
		seen[option.Code] = true
	}
	for _, rules := range []*scoringRules{e.Scoring, e.DXScoring} {
		if rules == nil {
			continue
		}
		for _, rule := range rules.effectiveMultipliers() {
			if strings.TrimSpace(rule.Kind) == "county" {
				if len(seen) == 0 {
					return fmt.Errorf("event %q county scoring requires county_options", e.ID)
				}
				if rule.Per != "band" && rule.Per != "contest" {
					return fmt.Errorf("event %q county multiplier scope must be band or contest", e.ID)
				}
			}
		}
	}
	if len(e.ReceivedExchangeOptions) == 0 && len(e.CountyOptions) > 0 {
		e.ReceivedExchangeOptions = append([]exchangeOption(nil), e.CountyOptions...)
	}
	return nil
}
