package main

import (
	"fmt"
	"strconv"
	"strings"
)

func positiveSerial(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	n, err := strconv.Atoi(s)
	return err == nil && n > 0
}

// Incomplete contacts may be retained locally; submission must not silently
// turn them into a purportedly checked exchange.
func validateContestSubmission(q qso, event eventDefinition, profile stationProfile) error {
	if event.QSOParty != nil {
		_, err := event.partyCredits(q)
		if err != nil {
			return err
		}
		if !event.partyInPeriod(q.time) {
			return fmt.Errorf("QSO is outside this event's verified operating periods")
		}
		if !bandAllowed(event.Bands, q.band) {
			return fmt.Errorf("band %q is not allowed", q.band)
		}
		if q.mode != "" && !strings.EqualFold(q.mode, "CW") {
			return fmt.Errorf("QSO party export supports CW only")
		}
		if !event.CabrilloOmitRST {
			for _, rst := range []string{q.rstSent, q.rstRcvd} {
				if len(rst) != 3 || rst[0] < '1' || rst[0] > '5' || rst[1] < '1' || rst[1] > '9' || rst[2] < '1' || rst[2] > '9' {
					return fmt.Errorf("CW RST must be three valid digits")
				}
			}
		}
		return nil
	}
	if strings.TrimSpace(q.stxString) == "" && strings.TrimSpace(q.stx) == "" {
		return fmt.Errorf("missing sent exchange")
	}
	if strings.TrimSpace(q.srxString) == "" && strings.TrimSpace(q.srx) == "" {
		return fmt.Errorf("missing received exchange")
	}
	callSent := q.stationCallsign
	if callSent == "" {
		callSent = profile.Callsign
	}
	if err := validateSubmissionExchange(event.ID, callSent, q.stx, q.stxString); err != nil {
		return fmt.Errorf("sent exchange: %w", err)
	}
	if err := validateSubmissionExchange(event.ID, q.call, q.srx, q.srxString); err != nil {
		return fmt.Errorf("received exchange: %w", err)
	}
	if !event.CabrilloOmitRST && event.ID != "STEW-PERRY" && (strings.TrimSpace(q.rstSent) == "" || strings.TrimSpace(q.rstRcvd) == "") {
		return fmt.Errorf("missing RST report")
	}
	if !event.CabrilloOmitRST {
		for _, rst := range []string{q.rstSent, q.rstRcvd} {
			if event.ID == "STEW-PERRY" && rst == "" {
				continue // The sponsor makes RST optional.
			}
			if len(rst) != 3 || rst[0] < '1' || rst[0] > '5' || rst[1] < '1' || rst[1] > '9' || rst[2] < '1' || rst[2] > '9' {
				return fmt.Errorf("CW RST must be three digits (readability 1–5, strength/tone 1–9)")
			}
		}
	}
	return nil
}

// Validate the combined value used by the writer, including imported serials
// stored in the text field. Never discard extra tokens to make a value valid.
func validateSubmissionExchange(eventID, call, serial, text string) error {
	token := strings.ToUpper(cabrilloExchange(serial, text))
	for _, r := range serial + text {
		if r < ' ' || r > '~' {
			return fmt.Errorf("exchange must contain printable ASCII only")
		}
	}
	switch eventID {
	case "HELVETIA", "RDXC", "WAG":
		return validateRegionalExchange(eventID, call, serial, text)
	case "ARRL-SS-CW":
		_, err := sweepstakesExchange(serial, text, call)
		return err
	case "CW-OPEN":
		fields := strings.Fields(token)
		if len(fields) != 2 || !positiveSerial(fields[0]) || !submissionName(fields[1]) {
			return fmt.Errorf("CW Open exchange needs a positive serial and name")
		}
	case "CQ-WPX-CW", "DARC-WAEDC-CW", "SAC-CW", "OCEANIA-DX-CW":
		if !positiveSerial(token) {
			return fmt.Errorf("exchange must be one positive decimal serial")
		}
	case "CQ-WW-CW":
		if !submissionZone(token, 40) {
			return fmt.Errorf("CQ zone must be 1–40")
		}
	case "IARU-HF":
		if !submissionZone(token, 90) && !iaruSubmissionCodes[token] {
			return fmt.Errorf("exchange must be ITU zone 1–90 or a recognized IARU society/official code")
		}
	case "STEW-PERRY":
		if len(token) != 4 || token[0] < 'A' || token[0] > 'R' || token[1] < 'A' || token[1] > 'R' || !isAllDigits(token[2:]) {
			return fmt.Errorf("exchange must be a four-character Maidenhead grid (e.g. EM75)")
		}
	case "ARRL-DX-CW", "CQ-160-CW", "NAQP-CW", "NA-SPRINT-CW", "CWT", "K1USN-SST", "TNQP":
		return validateLocationSubmission(eventID, call, token)
	default:
		return fmt.Errorf("event %q has no checked exchange validator", eventID)
	}
	return nil
}

func submissionZone(token string, max int) bool {
	n, err := strconv.Atoi(token)
	return positiveSerial(token) && err == nil && n <= max
}

const canadianSubmissionAreas = " AB BC MB NB NL NF LB NS NT NU ON PE QC SK YT "

func submissionArea(token, country string, includeAKHI bool) bool {
	canadian := len(token) == 2 && strings.Contains(canadianSubmissionAreas, " "+token+" ")
	if country == "Canada" {
		return canadian
	}
	if country == "Alaska" {
		return includeAKHI && token == "AK"
	}
	if country == "Hawaii" {
		return includeAKHI && token == "HI"
	}
	return country == "United States" && exchangeAreaCodes[token] && !canadian
}

func validateLocationSubmission(eventID, call, token string) error {
	table, err := sharedDXCCTable()
	if err != nil {
		return err
	}
	entity, ok := table.lookup(call)
	if !ok {
		return fmt.Errorf("cannot determine exchange country for %q", call)
	}
	if eventID == "TNQP" {
		if entity.Country == "United States" && tnCountyCodes[token] {
			return nil
		}
		if token != "TN" && submissionArea(token, entity.Country, true) {
			return nil
		}
		if entity.Country != "United States" && entity.Country != "Canada" && entity.Country != "Alaska" && entity.Country != "Hawaii" && token == "DX" {
			return nil
		}
		return fmt.Errorf("TNQP needs a TN county code, other state/province, or DX")
	}
	if eventID == "ARRL-DX-CW" || eventID == "CQ-160-CW" {
		if entity.Country == "United States" || entity.Country == "Canada" {
			if !submissionArea(token, entity.Country, false) {
				return fmt.Errorf("exchange must be a valid state/province for %s", entity.Country)
			}
		} else if eventID == "CQ-160-CW" {
			if !submissionZone(token, 40) {
				return fmt.Errorf("DX exchange must be CQ zone 1–40")
			}
		} else if !validSubmissionPower(token) {
			return fmt.Errorf("DX exchange must be positive power in watts or a power abbreviation (e.g. 100, 100W, KW)")
		}
		return nil
	}
	fields := strings.Fields(token)
	if eventID == "NA-SPRINT-CW" {
		if len(fields) == 0 || !positiveSerial(fields[0]) {
			return fmt.Errorf("Sprint exchange needs a positive serial, name and location")
		}
		fields = fields[1:]
	}
	if len(fields) < 1 || len(fields) > 2 || !submissionName(fields[0]) {
		return fmt.Errorf("exchange needs one alphabetic name and location")
	}
	northAmerican := entity.Continent == "NA" || entity.Country == "Hawaii"
	if len(fields) == 1 {
		if !northAmerican && eventID == "NAQP-CW" {
			return nil
		}
		return fmt.Errorf("missing exchange location")
	}
	location := fields[1]
	if eventID == "CWT" && positiveSerial(location) {
		return nil // Syntax only; membership ownership is not inferred.
	}
	if entity.Country == "United States" || entity.Country == "Canada" || entity.Country == "Alaska" || entity.Country == "Hawaii" {
		if submissionArea(location, entity.Country, true) {
			return nil
		}
	} else if !northAmerican && location == "DX" && (eventID == "NAQP-CW" || eventID == "NA-SPRINT-CW") {
		return nil
	} else if northAmerican || eventID == "NA-SPRINT-CW" || eventID == "CWT" || eventID == "K1USN-SST" {
		// Require an actual prefix token, not an arbitrary string that merely
		// starts with one. Match the station's country as well.
		for _, alias := range table.prefixByFirst[location[0]] {
			if alias.prefix == location && alias.entity.Country == entity.Country {
				return nil
			}
		}
	}
	return fmt.Errorf("invalid exchange location %q for %s", location, entity.Country)
}

func submissionName(s string) bool {
	for _, r := range s {
		if r < 'A' || r > 'Z' {
			return false
		}
	}
	return s != ""
}

func validSubmissionPower(s string) bool {
	if s == "NAN" || s == "INF" {
		return false
	}
	if s == "KW" {
		return true
	}
	// CW cut digits are commonly copied as ATT (100) or AK (1 kW).
	s = strings.NewReplacer("A", "1", "N", "9", "T", "0", "O", "0").Replace(s)
	s = strings.TrimSuffix(s, "W")
	s = strings.TrimSuffix(s, "K")
	if s == "" {
		return false
	}
	dot := false
	for _, r := range s {
		if r == '.' && !dot {
			dot = true
		} else if r < '0' || r > '9' {
			return false
		}
	}
	n, err := strconv.ParseFloat(s, 64)
	return err == nil && n > 0
}

// Validate the exact combined token that Cabrillo will emit. These contests
// exchange either a regional code or a serial, never both. Consult the station
// callsign snapshot (with the same profile fallback as the writer), rather than
// potentially stale QSO country enrichment.
func validateRegionalExchange(eventID, call, serial, text string) error {
	table, err := sharedDXCCTable()
	if err != nil {
		return fmt.Errorf("resolve exchange country: %w", err)
	}
	entity, ok := table.lookup(call)
	if !ok {
		return fmt.Errorf("cannot determine exchange country for %q", call)
	}
	token := strings.ToUpper(cabrilloExchange(serial, text))
	switch eventID {
	case "HELVETIA":
		if entity.Country == "Switzerland" {
			if cantonCode(token) == "" {
				return fmt.Errorf("Swiss exchange must be one valid canton code")
			}
			return nil
		}
	case "RDXC":
		russian := entity.Country == "European Russia" || entity.Country == "Asiatic Russia" || entity.Country == "Kaliningrad" || entity.Country == "Franz Josef Land"
		// RDXC section 7.3 also includes Russian Antarctic stations.
		russian = russian || (entity.Country == "Antarctica" && strings.HasPrefix(normalizeCall(call), "RI1AN"))
		if russian {
			if rdxcOblastCode(token) == "" {
				return fmt.Errorf("Russian exchange must be one valid oblast code")
			}
			return nil
		}
	case "WAG":
		if entity.Country == "Fed. Rep. of Germany" {
			if !validWAGDOK(token) {
				return fmt.Errorf("German exchange must be NM or one alphanumeric DOK")
			}
			return nil
		}
	}
	if !positiveSerial(token) {
		return fmt.Errorf("exchange must be a positive decimal serial")
	}
	if eventID == "HELVETIA" && len(token) < 3 {
		return fmt.Errorf("Helvetia serial must contain at least three digits")
	}
	return nil
}

// Special DOKs may begin with digits and do not follow the ordinary A01
// pattern. Check syntax only: assignment/membership needs a dated DARC roster.
func validWAGDOK(token string) bool {
	if len(token) < 2 {
		return false
	}
	hasLetter := false
	for _, r := range token {
		if r >= 'A' && r <= 'Z' {
			hasLetter = true
		} else if r < '0' || r > '9' {
			return false
		}
	}
	return hasLetter
}

func sweepstakesExchange(serial, text, call string) (string, error) {
	fields := strings.Fields(strings.ToUpper(text))
	// The entry hint includes the station's callsign; Cabrillo already has it.
	if len(fields) == 4 && fields[1] == normalizeCall(call) {
		fields = append(fields[:1], fields[2:]...)
	}
	if !positiveSerial(serial) || len(serial) > 4 || len(fields) != 3 {
		return "", fmt.Errorf("Sweepstakes needs serial, precedence, check and section")
	}
	if len(fields[0]) != 1 || !strings.Contains("QABUMS", fields[0]) || len(fields[1]) != 2 || !isAllDigits(fields[1]) || arrlSectionCode(fields[2]) == "" {
		return "", fmt.Errorf("invalid Sweepstakes precedence/check/section")
	}
	return fmt.Sprintf("%4s %s %2s %-3s", serial, fields[0], fields[1], arrlSectionCode(fields[2])), nil
}
