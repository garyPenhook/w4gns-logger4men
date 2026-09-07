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
	// Applies to every contest submission, not just QSO parties: an imported
	// contact bypasses the interactive entry screen's band restriction, so
	// this is the only place left to enforce the event's catalog band list
	// before the contact is exported and scored.
	if len(event.Bands) > 0 && !bandAllowed(event.Bands, q.band) {
		return fmt.Errorf("band %q is not allowed", q.band)
	}
	if event.QSOParty != nil {
		_, err := event.partyCredits(q)
		if err != nil {
			return err
		}
		if !event.partyInPeriod(q.time) {
			return fmt.Errorf("QSO is outside this event's verified operating periods")
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
	if strings.TrimSpace(q.stxString) == "" && strings.TrimSpace(q.stx) == "" && !submissionAllowsBlankExchange(event.ID) {
		return fmt.Errorf("missing sent exchange")
	}
	if strings.TrimSpace(q.srxString) == "" && strings.TrimSpace(q.srx) == "" && !submissionAllowsBlankExchange(event.ID) {
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

// These sponsor rules explicitly prescribe RST-only QSOs for a class of
// stations, so the Cabrillo exchange column legitimately remains blank.
func submissionAllowsBlankExchange(eventID string) bool {
	switch eventID {
	case "ARRL-160M", "DIG-QSO-PARTY", "GERMAN-TELEGRAPHY-CONTEST":
		return true
	default:
		return false
	}
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
	case "HELVETIA", "RDXC", "WAG", "SPDX", "UKRAINIAN-DX", "YO-DX-HF", "XMAS", "PORTUGAL-DAY", "RDAC", "EA-MAJESTAD-CW", "9A-DX":
		return validateRegionalExchange(eventID, call, serial, text)
	case "PRO-CW-CONTEST":
		if strings.HasSuffix(token, "/M") {
			token = strings.TrimSuffix(token, "/M")
		}
		if !positiveSerial(token) {
			return fmt.Errorf(`exchange must be a positive serial, optionally suffixed "/M" for members`)
		}
	case "RUSSIAN-RADIO-TEAM-CHAMPIONSHIP":
		// Whether the exchanged code is an RRTC member's 3-character club code
		// or a plain ITU zone depends on club membership, not the caller's
		// location — not resolvable from a callsign, so both forms are
		// accepted; the club code is syntax-only (no membership roster here).
		if !submissionZone(token, 90) && !isThreeCharAlnumCode(token) {
			return fmt.Errorf("exchange must be ITU zone 1–90 or a three-character RRTC club code")
		}
	case "RSGB-160":
		return validateUKDistrictExchange(call, token)
	case "ALL-AUSTRIAN-160-METER-CONTEST":
		return validateAustrianDistrictExchange(call, token)
	case "RSGB-LOW-POWER":
		fields := strings.Fields(token)
		if len(fields) != 2 || !positiveSerial(fields[0]) || !validSubmissionPower(fields[1]) {
			return fmt.Errorf("exchange must be a serial and power")
		}
	case "ARRL-SS-CW":
		_, err := sweepstakesExchange(serial, text, call)
		return err
	case "CW-OPEN":
		fields := strings.Fields(token)
		if len(fields) != 2 || !positiveSerial(fields[0]) || !submissionName(fields[1]) {
			return fmt.Errorf("CW Open exchange needs a positive serial and name")
		}
	case "ICWC-MST":
		// MST exchanges are the operator name followed by that operator's
		// running QSO number; the number is not necessarily our serial.
		fields := strings.Fields(token)
		if len(fields) != 2 || !submissionName(fields[0]) || !positiveSerial(fields[1]) {
			return fmt.Errorf("MST exchange needs one alphabetic name and a positive QSO number")
		}
	case "AADX-CW", "EUHFC":
		// Both contests exchange an exactly two-digit personal value: age for
		// All Asian DX and first-license year for the European HF Championship.
		if len(token) != 2 || !isAllDigits(token) {
			return fmt.Errorf("exchange must be exactly two decimal digits")
		}
	case "40-80":
		if len(token) != 2 || !submissionName(token) {
			return fmt.Errorf("exchange must be a two-letter Italian province code")
		}
	case "ARI-DX":
		return validateItalianProvinceOrSerialExchange(call, token)
	case "CVA-DX-CW":
		return validateCVAExchange(call, token)
	case "MCD-QSO-PARTY":
		if strings.HasPrefix(token, "MC") {
			if len(token) != 5 || !positiveSerial(token[2:]) {
				return fmt.Errorf("Marconi Club membership exchange must be MC followed by a three-digit number")
			}
			return nil
		}
		if !positiveSerial(token) {
			return fmt.Errorf("non-member exchange must be a positive serial")
		}
	case "HA3NS-SPRINT":
		if token != "NM" && !positiveSerial(token) {
			return fmt.Errorf("HA3NS Sprint exchange must be a HACWG member number or NM")
		}
	case "DIG-QSO-PARTY":
		// DIG non-members send only RST; members send their positive DIG number.
		if token != "" && !positiveSerial(token) {
			return fmt.Errorf("DIG QSO Party exchange must be a DIG member number or blank for a non-member")
		}
	case "ARRL-160M":
		return validateARRL160Exchange(call, token)
	case "GERMAN-TELEGRAPHY-CONTEST":
		return validateDTCExchange(call, token)
	case "CP-QSO-PARTY":
		return validateCPQPExchange(call, token)
	case "IL-QSO-PARTY":
		return validateILQPExchange(call, token)
	case "IN-QSO-PARTY":
		return validateINQPExchange(call, token)
	case "NM-QSO-PARTY":
		return validateNMQPExchange(call, token)
	case "OK-OM-DX":
		return validateOKOMDXExchange(call, token)
	case "EUDXC":
		if !validEURegionCode(token) && !submissionZone(token, 90) {
			return fmt.Errorf("EUDX exchange must be a published EU region code or ITU zone 1–90")
		}
	case "KENTUCKY-STATE-PARKS-ON-THE-AIR":
		return validateKentuckyParksExchange(call, token)
	case "MDC-QSO-PARTY":
		return validateMDCExchange(call, token)
	case "ARSI-VU-DX":
		return validateARSIExchange(call, token)
	case "HUNGARIAN-STRAIGHT-KEY-CONTEST":
		fields := strings.Fields(token)
		if len(fields) != 2 || !positiveSerial(fields[0]) || (fields[1] != "A" && fields[1] != "B") {
			return fmt.Errorf("exchange must be a serial and A/B power category")
		}
	case "A1CLUB-AWT":
		if !submissionName(token) {
			return fmt.Errorf("AWT exchange must be one alphabetic CW name")
		}
	case "AGCW-YL-CW-PARTY":
		fields := strings.Fields(token)
		if len(fields) != 3 || !positiveSerial(fields[0]) ||
			(fields[1] != "YL" && fields[1] != "OM") || !submissionName(fields[2]) {
			return fmt.Errorf("YL CW Party exchange needs serial, YL/OM, and name")
		}
	case "AGCW-STRAIGHT-KEY-PARTY":
		fields := strings.Fields(token)
		if len(fields) != 4 || len(fields[0]) != 3 || !positiveSerial(fields[0]) ||
			(fields[1] != "A" && fields[1] != "B" && fields[1] != "C") ||
			!submissionName(fields[2]) || (fields[3] != "XX" && !isAllDigits(fields[3])) {
			return fmt.Errorf("Straight Key Party exchange needs a 3-digit serial, A/B/C class, name, and age or XX")
		}
	case "AGCW-QRP":
		fields := strings.Fields(token)
		if len(fields) != 2 || len(fields[0]) != 3 || !positiveSerial(fields[0]) ||
			(fields[1] != "A" && fields[1] != "B") {
			return fmt.Errorf("QRP/QRP Party exchange needs a 3-digit serial and A/B category")
		}
	case "AGCW-SEMI-AUTOMATIC-KEY-EVENING":
		parts := strings.Split(token, "/")
		if len(parts) != 2 || len(parts[0]) != 3 || !positiveSerial(parts[0]) ||
			len(parts[1]) != 2 || !isAllDigits(parts[1]) {
			return fmt.Errorf("Semi-Automatic Key Evening exchange needs a 3-digit serial and two-digit year")
		}
	case "HIGH-SPEED-CLUB-CW-CONTEST":
		if token != "NM" && !positiveSerial(token) {
			return fmt.Errorf("HSC exchange must be a membership number or NM")
		}
	case "UKEI-DX":
		return validateUKEIDistrictExchange(call, token)
	case "HA-DX":
		return validateHungarianCountyExchange(call, token)
	case "PACC":
		return validateDutchProvinceExchange(call, token)
	case "JIDX-CW":
		return validateJIDXExchange(call, token)
	case "EUCW-160M":
		fields := strings.Fields(token)
		if len(fields) == 2 && submissionName(fields[0]) && fields[1] == "NM" {
			break
		}
		if len(fields) != 3 || !submissionName(fields[0]) || !submissionName(fields[1]) || !positiveSerial(fields[2]) {
			return fmt.Errorf("EUCW 160m exchange needs name plus NM, or name, club, and membership number")
		}
	case "LZ-INTERNATIONAL-6-METER-CONTEST":
		fields := strings.Fields(token)
		if len(fields) != 2 || !positiveSerial(fields[0]) || len(fields[1]) != 6 {
			return fmt.Errorf("LZ 6m exchange needs a positive serial and 6-character grid")
		}
		if _, err := ParseGridSquare(fields[1]); err != nil {
			return fmt.Errorf("LZ 6m exchange grid: %w", err)
		}
	case "REF-DDFM-6M-CONTEST":
		fields := strings.Fields(token)
		if len(fields) != 2 || !positiveSerial(fields[0]) || len(fields[1]) != 4 {
			return fmt.Errorf("REF DDFM 6m exchange needs a positive serial and 4-character grid")
		}
		if _, err := ParseGridSquare(fields[1]); err != nil {
			return fmt.Errorf("REF DDFM 6m exchange grid: %w", err)
		}
	case "NRAU-CW":
		fields := strings.Fields(token)
		if len(fields) != 2 || !positiveSerial(fields[0]) || len(fields[1]) != 2 || !submissionName(fields[1]) {
			return fmt.Errorf("NRAU-Baltic exchange needs a positive serial and two-letter region")
		}
	case "QRP-FOX-HUNT":
		return validateQRPFoxHuntExchange(call, token)
	case "NTC-QSO-PARTY":
		fields := strings.Fields(token)
		if len(fields) != 2 || !submissionName(fields[0]) || (fields[1] != "NM" && !positiveSerial(fields[1])) {
			return fmt.Errorf("NTC QSO Party exchange needs a name and NTC number or NM")
		}
	case "AGCW-HAPPY-NEW-YEAR-CONTEST":
		parts := strings.Split(token, "/")
		if len(parts) < 1 || len(parts) > 2 || !positiveSerial(parts[0]) || (len(parts) == 2 && !positiveSerial(parts[1])) {
			return fmt.Errorf("Happy New Year exchange needs a serial, optionally followed by an AGCW member number")
		}
	case "AGCW-QRP-CONTEST":
		fields := strings.Fields(token)
		if len(fields) != 3 || !positiveSerial(fields[0]) ||
			(fields[1] != "VLP" && fields[1] != "QRP" && fields[1] != "MP" && fields[1] != "QRO") ||
			(fields[2] != "NM" && !positiveSerial(fields[2])) {
			return fmt.Errorf("AGCW QRP exchange needs serial, VLP/QRP/MP/QRO category, and member number or NM")
		}
	case "KEYMAN-S-CLUB-OF-JAPAN-CONTEST", "KCJ-TOPBAND":
		return validateJapanZoneExchange(call, token)
	case "ARRL-RR-CW":
		return validateRookieRoundupExchange(call, token)
	case "CQ-WPX-CW", "DARC-WAEDC-CW", "SAC-CW", "OCEANIA-DX-CW",
		"AP-SPRINT", "ASIA-PACIFIC-SPRING-SPRINT", "BALTIC-CONTEST", "MINITEST-40", "MINITEST-80",
		"MMC-HF-CW", "RSGB-80M-AUT", "RSGB-80M-CC", "RSGB-AFS-CW", "RSGB-NFD",
		"IARU-REGION-1-FIELD-DAY", "SEANET-CONTEST", "SARL-HF-CW", "TTC-SPCWC", "BALKAN-HF", "BEKASI-MERDEKA-CONTEST", "VENEZUELAN-IND-DAY-CONTEST":
		if !positiveSerial(token) {
			return fmt.Errorf("exchange must be one positive decimal serial")
		}
	case "CQ-WW-CW", "SA10M", "WWSA":
		if !submissionZone(token, 40) {
			return fmt.Errorf("CQ zone must be 1–40")
		}
	case "SACW":
		if token != "M" && token != "QRP" && token != "YL" && !submissionZone(token, 90) {
			return fmt.Errorf("exchange must be M, QRP, YL, or ITU zone 1–90")
		}
	case "IARU-HF":
		if !submissionZone(token, 90) && !iaruSubmissionCodes[token] {
			return fmt.Errorf("exchange must be ITU zone 1–90 or a recognized IARU society/official code")
		}
	case "STEW-PERRY", "RADIO-160":
		if len(token) != 4 || token[0] < 'A' || token[0] > 'R' || token[1] < 'A' || token[1] > 'R' || !isAllDigits(token[2:]) {
			return fmt.Errorf("exchange must be a four-character Maidenhead grid (e.g. EM75)")
		}
	case "ARRL-DX-CW", "CQ-160-CW", "NAQP-CW", "NA-SPRINT-CW", "NCCC-SPRINT-CW", "CWT", "K1USN-SST", "TNQP", "TNQP-DX":
		return validateLocationSubmission(eventID, call, token)
	case "RADIO-YL-OM":
		if token != "88" && token != "73" {
			return fmt.Errorf(`exchange must be "88" (YL) or "73" (OM)`)
		}
	case "SLOW-CW-QSO-PARTY":
		if strings.HasPrefix(token, "MC") && positiveSerial(strings.TrimPrefix(token, "MC")) {
			return nil
		}
		if !positiveSerial(token) {
			return fmt.Errorf(`exchange must be a positive serial, optionally prefixed "MC" for members`)
		}
	case "RAEM":
		fields := strings.Fields(token)
		if len(fields) != 3 || !positiveSerial(fields[0]) || !validRAEMCoordinate(fields[1], "NS") || !validRAEMCoordinate(fields[2], "WO") {
			return fmt.Errorf("RAEM exchange needs a serial, then latitude and longitude as digits + N/S/W/O")
		}
	case "UBA-DX-CW", "UBA-ON-6M", "UBA-ON-CW", "UBA-SPRING-CONTEST", "UBA-SPRING-CONTEST-2":
		return validateBelgianSectionExchange(call, token)
	case "AR-QSO-PARTY", "DE-QSO-PARTY", "SC-QSO-PARTY", "SDQSOP", "TXQP", "VT-QSO-PARTY", "WA-SALMON-RUN", "WVQP":
		return validateCountyOrAreaExchange(call, token)
	case "AZ-QSO-PARTY":
		return validateAZQPExchange(call, token)
	case "VA-QSO-PARTY":
		return validateSerialCountyOrAreaExchange(call, token, "Virginia QSO Party")
	case "IRTS-80M-COUNTIES":
		return validateIRTSCountyExchange(call, token)
	case "NEQP":
		return validateAreaOrDXExchange(call, token, "NEQP")
	case "MO-QSO-PARTY":
		return validateUSCountyOrDXExchange(call, token, "Missouri QSO Party")
	case "COQP":
		return validateUSCountyOrDXExchange(call, token, "Colorado QSO Party")
	case "NJQP":
		return validateUSCountyOrDXExchange(call, token, "New Jersey QSO Party")
	case "MS-QSO-PARTY":
		return validateUSCountyOrCountryExchange(call, token, "Mississippi QSO Party")
	case "OK-QSO-PARTY":
		return validateUSCountyOrCountryExchange(call, token, "Oklahoma QSO Party")
	case "PA-QSO-PARTY":
		return validatePAQPExchange(token)
	case "NC-QSO-PARTY":
		return validateUSCountyOrDXExchange(call, token, "North Carolina QSO Party")
	case "HI-QSO-PARTY":
		return validateUSCountyOrDXExchange(call, token, "Hawaii QSO Party")
	case "ID-QSO-PARTY":
		return validateUSCountyOrCountryExchange(call, token, "Idaho QSO Party")
	case "ND-QSO-PARTY":
		return validateUSCountyOrCountryExchange(call, token, "North Dakota QSO Party")
	case "KS-QSO-PARTY":
		return validateUSCountyOrDXExchange(call, token, "Kansas QSO Party")
	case "LA-QSO-PARTY":
		return validateUSCountyOrCountryExchange(call, token, "Louisiana QSO Party")
	case "MN-QSO-PARTY":
		return validateMinnesotaExchange(call, token)
	case "NH-QSO-PARTY":
		return validateUSCountyOrDXExchange(call, token, "New Hampshire QSO Party")
	case "ME-QSO-PARTY":
		return validateUSCountyOrDXExchange(call, token, "Maine QSO Party")
	case "WIQP":
		return validateUSCountyOrDXExchange(call, token, "Wisconsin QSO Party")
	case "KYQP":
		return validateKentuckyExchange(call, token)
	case "BC-QSO-PARTY":
		return validateUSCountyOrDXExchange(call, token, "British Columbia QSO Party")
	case "ON-QSO-PARTY":
		return validateUSCountyOrCountryOrDXExchange(call, token, "Ontario QSO Party")
	case "CANADA-DAY", "CANADA-WINTER":
		return validateRACCanadaExchange(call, token)
	case "EA-QRP-CW-CONTEST":
		return validateEAQRPExchange(token)
	case "7QP":
		return validate7QPExchange(call, token)
	case "NY-QSO-PARTY":
		return validateNewYorkExchange(call, token)
	case "NE-QSO-PARTY":
		return validateUSCountyOrCountryExchange(call, token, "Nebraska QSO Party")
	case "LZ-DX":
		return validateLZDXExchange(call, token)
	case "QC-QSO-PARTY":
		return validateQuebecExchange(call, token)
	case "AC-QSO-PARTY":
		return validateAtlanticCanadaExchange(call, token)
	case "DARC-CWA":
		return validateRegionalExchange("XMAS", call, serial, text)
	case "EASTER":
		return validateRegionalExchange("XMAS", call, serial, text)
	case "DARC-10":
		return validateDARC10Exchange(call, token)
	case "ISLAND-QSO-PARTY":
		return validateUSIslandExchange(call, token)
	case "TR-HF":
		return validateTurkiyeExchange(call, token)
	case "REF-160-METER-CONTEST":
		return validateREF160Exchange(call, token)
	case "REF-CW":
		return validateREFCWExchange(call, token)
	case "ARS-SPARTAN-SPRINT", "ARS-FLIGHT-OF-THE-BUMBLEBEES", "MI-QRP-LABOR-DAY-CW-SPRINT",
		"NAQCC-CW-SPRINT", "4-STATES-QRP-GROUP-SECOND-SUNDAY", "QRP-ARCI-FALL-QSO-PARTY",
		"QRP-ARCI-HOOTOWL-SPRINT", "QRP-ARCI-SPRING-QSO-PARTY", "RUN-FOR-THE-BACON-QRP-CONTEST":
		return validateSPCExchange(call, token, false)
	case "QRP-ARCI-HOLIDAY-SPIRITS-SPRINT", "QRP-ARCI-SUMMER-HOMEBREW-SPRINT", "QRP-ARCI-TOPBAND-SPRINT":
		return validateHolidaySpiritsExchange(call, token)
	case "WALK-FOR-THE-BACON-QRP-CONTEST", "SKCC-SPRINT", "SKCC-SPRINT-EUROPE", "SKCC-WEEKEND-SPRINTATHON":
		return validateSPCExchange(call, token, true)
	case "RSGB-IOTA":
		// "RS(T) + Serial No. + IOTA No. (if operating from a qualifying
		// island)" (rsgbcc.org/hf/rules): the IOTA reference is optional
		// (world stations send only a serial), but a second field must be a
		// valid reference, not arbitrary trailing text.
		fields := strings.Fields(token)
		if len(fields) < 1 || len(fields) > 2 || !positiveSerial(fields[0]) {
			return fmt.Errorf("exchange must be a positive serial, optionally followed by an IOTA reference")
		}
		if len(fields) == 2 && iotaReferenceCode(fields[1]) != fields[1] {
			return fmt.Errorf("IOTA reference must look like EU-005")
		}
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
	if eventID == "TNQP" || eventID == "TNQP-DX" {
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
	if eventID == "NA-SPRINT-CW" || eventID == "NCCC-SPRINT-CW" {
		if len(fields) == 0 || !positiveSerial(fields[0]) {
			return fmt.Errorf("sprint exchange needs a positive serial, name and location")
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
	} else if !northAmerican && location == "DX" && (eventID == "NAQP-CW" || eventID == "NA-SPRINT-CW" || eventID == "NCCC-SPRINT-CW") {
		return nil
	} else if northAmerican || eventID == "NA-SPRINT-CW" || eventID == "NCCC-SPRINT-CW" || eventID == "CWT" || eventID == "K1USN-SST" {
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

// validateRookieRoundupExchange checks ARRL Rookie Roundup's CW exchange:
// operator name, a two-digit first-license year, then the station location.
// US and Canadian stations send their state/province, Mexican stations their
// XE call area, and other stations send DX.
func validateRookieRoundupExchange(call, token string) error {
	table, err := sharedDXCCTable()
	if err != nil {
		return fmt.Errorf("resolve exchange country: %w", err)
	}
	entity, ok := table.lookup(call)
	if !ok {
		return fmt.Errorf("cannot determine exchange country for %q", call)
	}
	fields := strings.Fields(token)
	if len(fields) != 3 || !submissionName(fields[0]) || len(fields[1]) != 2 || !isAllDigits(fields[1]) {
		return fmt.Errorf("Rookie Roundup exchange needs name, two-digit first-license year, and location")
	}
	location := fields[2]
	if entity.Country == "Mexico" {
		if len(location) == 3 && strings.HasPrefix(location, "XE") && location[2] >= '0' && location[2] <= '9' {
			return nil
		}
		return fmt.Errorf("exchange for Mexico must use an XE call area")
	}
	if entity.Country == "United States" || entity.Country == "Canada" || entity.Country == "Alaska" || entity.Country == "Hawaii" {
		if submissionArea(location, entity.Country, true) {
			return nil
		}
		return fmt.Errorf("exchange must use a valid state/province for %s", entity.Country)
	}
	if location != "DX" {
		return fmt.Errorf("DX exchange location must be DX")
	}
	return nil
}

func validateJapanZoneExchange(call, token string) error {
	table, err := sharedDXCCTable()
	if err != nil {
		return fmt.Errorf("resolve exchange country: %w", err)
	}
	entity, ok := table.lookup(call)
	if !ok {
		return fmt.Errorf("cannot determine exchange country for %q", call)
	}
	if entity.Country == "Japan" {
		if len(token) != 2 || !submissionName(token) {
			return fmt.Errorf("exchange for Japan must be a two-letter prefecture/district code")
		}
		return nil
	}
	if !submissionZone(token, 40) {
		return fmt.Errorf("DX exchange must be CQ zone 1–40")
	}
	return nil
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
				return fmt.Errorf("exchange for Switzerland must be one valid canton code")
			}
			return nil
		}
	case "RDXC", "RDAC":
		russian := entity.Country == "European Russia" || entity.Country == "Asiatic Russia" || entity.Country == "Kaliningrad" || entity.Country == "Franz Josef Land"
		// RDXC section 7.3 also includes Russian Antarctic stations.
		russian = russian || (entity.Country == "Antarctica" && strings.HasPrefix(normalizeCall(call), "RI1AN"))
		if russian {
			if eventID == "RDAC" {
				if token == "" {
					return fmt.Errorf("exchange for Russia must be one district code")
				}
				return nil
			}
			if rdxcOblastCode(token) == "" {
				return fmt.Errorf("exchange for Russia must be one valid oblast code")
			}
			return nil
		}
	case "EA-MAJESTAD-CW":
		// The organizer classifies EA, EA6, EA8, and EA9 as Spanish
		// stations. cty.dat represents those as four distinct entities.
		spanish := entity.Country == "Spain" || entity.Country == "Balearic Islands" || entity.Country == "Canary Islands" || entity.Country == "Ceuta & Melilla"
		if spanish {
			if !validSpanishProvinceCode(token) {
				return fmt.Errorf("exchange for a Spanish station must be a province abbreviation")
			}
			return nil
		}
	case "9A-DX":
		if entity.Country == "Croatia" {
			if len(token) != 2 || !submissionName(token) {
				return fmt.Errorf("exchange for Croatia must be a two-letter county code")
			}
			return nil
		}
		if !submissionZone(token, 90) {
			return fmt.Errorf("DX exchange must be ITU zone 1–90")
		}
		return nil
	case "PORTUGAL-DAY":
		if entity.Country == "Portugal" {
			if token == "" {
				return fmt.Errorf("exchange for Portugal must be one district code")
			}
			return nil
		}
	case "SPDX":
		if entity.Country == "Poland" {
			if token == "" {
				return fmt.Errorf("exchange for Poland must be one province code")
			}
			return nil
		}
	case "UKRAINIAN-DX":
		if entity.Country == "Ukraine" {
			if len(token) != 2 {
				return fmt.Errorf("exchange for Ukraine must be a two-letter oblast code")
			}
			return nil
		}
	case "YO-DX-HF":
		if entity.Country == "Romania" {
			if token == "" {
				return fmt.Errorf("exchange for Romania must be one county code")
			}
			return nil
		}
	case "XMAS", "WAG":
		if entity.Country == "Fed. Rep. of Germany" {
			if !validWAGDOK(token) {
				return fmt.Errorf("exchange for Germany must be NM or one alphanumeric DOK")
			}
			return nil
		}
	}
	if !positiveSerial(token) {
		return fmt.Errorf("exchange must be a positive decimal serial")
	}
	if eventID == "HELVETIA" && len(token) < 3 {
		return fmt.Errorf("serial for Helvetia must contain at least three digits")
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
		return "", fmt.Errorf("exchange for Sweepstakes needs serial, precedence, check and section")
	}
	if len(fields[0]) != 1 || !strings.Contains("QABUMS", fields[0]) || len(fields[1]) != 2 || !isAllDigits(fields[1]) || arrlSectionCode(fields[2]) == "" {
		return "", fmt.Errorf("invalid Sweepstakes precedence/check/section")
	}
	return fmt.Sprintf("%4s %s %2s %-3s", serial, fields[0], fields[1], arrlSectionCode(fields[2])), nil
}

// isThreeCharAlnumCode is a syntax-only check for RUSSIAN-RADIO-TEAM-CHAMPIONSHIP's
// member club code — three uppercase letters/digits, no membership roster here.
func isThreeCharAlnumCode(s string) bool {
	if len(s) != 3 {
		return false
	}
	for _, r := range s {
		if !((r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9')) {
			return false
		}
	}
	return true
}

// validRAEMCoordinate checks RAEM's own degrees+hemisphere shape (e.g. "57N",
// "85O" for East per the contest's rules) — digits followed by exactly one of
// the given hemisphere letters.
func validRAEMCoordinate(s, hemispheres string) bool {
	if len(s) < 2 {
		return false
	}
	if !strings.ContainsRune(hemispheres, rune(s[len(s)-1])) {
		return false
	}
	return isAllDigits(s[:len(s)-1])
}

// validSpanishProvinceCode is a syntax check for the one- or two-letter
// province abbreviations (and EF0F's special SMR code) sent by EA stations
// in the King of Spain contest. A complete province roster is deliberately
// not claimed here; catalog validation rejects serials and arbitrary text
// while preserving valid sponsor abbreviations.
func validSpanishProvinceCode(s string) bool {
	if len(s) < 1 || len(s) > 3 {
		return false
	}
	for _, r := range s {
		if r < 'A' || r > 'Z' {
			return false
		}
	}
	return true
}

// validateUKDistrictExchange validates RSGB 160m's shape: a UK home-nation
// station sends a serial plus district code, everyone else sends a bare
// serial. "UK" here is England/Scotland/Wales/Northern Ireland — cty.dat's
// own DXCC entities for the home nations.
func validateUKDistrictExchange(call, token string) error {
	table, err := sharedDXCCTable()
	if err != nil {
		return fmt.Errorf("resolve exchange country: %w", err)
	}
	entity, ok := table.lookup(call)
	if !ok {
		return fmt.Errorf("cannot determine exchange country for %q", call)
	}
	fields := strings.Fields(token)
	uk := entity.Country == "England" || entity.Country == "Scotland" || entity.Country == "Wales" || entity.Country == "Northern Ireland"
	if uk {
		if len(fields) != 2 || !positiveSerial(fields[0]) || fields[1] == "" {
			return fmt.Errorf("exchange for the UK must be a serial and district code")
		}
	}
	if len(fields) != 1 || !positiveSerial(fields[0]) {
		return fmt.Errorf("exchange must be a single positive decimal serial")
	}
	return nil
}

func validateAustrianDistrictExchange(call, token string) error {
	table, err := sharedDXCCTable()
	if err != nil {
		return fmt.Errorf("resolve exchange country: %w", err)
	}
	entity, ok := table.lookup(call)
	if !ok {
		return fmt.Errorf("cannot determine exchange country for %q", call)
	}
	fields := strings.Fields(token)
	if entity.Country == "Austria" {
		if len(fields) != 2 || !positiveSerial(fields[0]) || !submissionName(fields[1]) {
			return fmt.Errorf("exchange for Austria must be a serial and district code")
		}
		return nil
	}
	if len(fields) != 1 || !positiveSerial(fields[0]) {
		return fmt.Errorf("exchange must be a single positive decimal serial")
	}
	return nil
}

func validateUKEIDistrictExchange(call, token string) error {
	table, err := sharedDXCCTable()
	if err != nil {
		return fmt.Errorf("resolve exchange country: %w", err)
	}
	entity, ok := table.lookup(call)
	if !ok {
		return fmt.Errorf("cannot determine exchange country for %q", call)
	}
	fields := strings.Fields(token)
	ukEI := entity.Country == "England" || entity.Country == "Scotland" || entity.Country == "Wales" || entity.Country == "Northern Ireland" || entity.Country == "Ireland" || entity.Country == "Isle of Man" || entity.Country == "Jersey" || entity.Country == "Guernsey"
	if ukEI {
		if len(fields) != 2 || !nonNegativeSerial(fields[0]) || len(fields[1]) != 2 || !submissionName(fields[1]) {
			return fmt.Errorf("UK/EI exchange must be a serial and two-letter district code")
		}
		return nil
	}
	if len(fields) != 1 || !nonNegativeSerial(fields[0]) {
		return fmt.Errorf("exchange must be one serial")
	}
	return nil
}

func nonNegativeSerial(s string) bool { return s == "0" || positiveSerial(s) }

var hungarianCountyCodes = map[string]bool{"BA": true, "BE": true, "BN": true, "BO": true, "BP": true, "CS": true, "FE": true, "GY": true, "HB": true, "HE": true, "SZ": true, "KO": true, "NG": true, "PE": true, "SO": true, "SA": true, "TO": true, "VA": true, "VE": true, "ZA": true}

func validateHungarianCountyExchange(call, token string) error {
	table, err := sharedDXCCTable()
	if err != nil {
		return fmt.Errorf("resolve exchange country: %w", err)
	}
	entity, ok := table.lookup(call)
	if !ok {
		return fmt.Errorf("cannot determine exchange country for %q", call)
	}
	if entity.Country == "Hungary" {
		if !hungarianCountyCodes[token] {
			return fmt.Errorf("exchange for Hungary must be a valid county code")
		}
		return nil
	}
	if !positiveSerial(token) {
		return fmt.Errorf("DX exchange must be a positive serial")
	}
	return nil
}

var dutchProvinceCodes = map[string]bool{
	"DR": true, "FL": true, "FR": true, "GD": true, "GR": true, "LB": true,
	"NB": true, "NH": true, "OV": true, "UT": true, "ZH": true, "ZL": true,
}

// validateDutchProvinceExchange implements PACC's PA/non-PA split: Dutch
// stations send a province code, while all other DXCC entities send a QSO
// serial number.
func validateDutchProvinceExchange(call, token string) error {
	table, err := sharedDXCCTable()
	if err != nil {
		return fmt.Errorf("resolve exchange country: %w", err)
	}
	entity, ok := table.lookup(call)
	if !ok {
		return fmt.Errorf("cannot determine exchange country for %q", call)
	}
	if entity.Country == "Netherlands" {
		if !dutchProvinceCodes[token] {
			return fmt.Errorf("PA exchange must be a valid Dutch province code")
		}
		return nil
	}
	if !positiveSerial(token) {
		return fmt.Errorf("non-PA exchange must be a positive serial")
	}
	return nil
}

// validateItalianProvinceOrSerialExchange implements the ARI International
// split: Italian stations send their two-letter province abbreviation, while
// all other stations send their QSO serial number. Province abbreviations are
// syntax-checked here because no authoritative ARI province roster is bundled.
func validateItalianProvinceOrSerialExchange(call, token string) error {
	table, err := sharedDXCCTable()
	if err != nil {
		return fmt.Errorf("resolve exchange country: %w", err)
	}
	entity, ok := table.lookup(call)
	if !ok {
		return fmt.Errorf("cannot determine exchange country for %q", call)
	}
	if entity.Country == "Italy" {
		if len(token) != 2 || !submissionName(token) {
			return fmt.Errorf("Italian exchange must be a two-letter province abbreviation")
		}
		return nil
	}
	if !positiveSerial(token) {
		return fmt.Errorf("non-Italian exchange must be a positive serial")
	}
	return nil
}

// validateCVAExchange implements CVA DX's 2026 exchange. Military stations
// send MIL irrespective of country; that status cannot be inferred from a
// callsign, so it is accepted as an explicit rule-defined alternative.
func validateCVAExchange(call, token string) error {
	if token == "MIL" {
		return nil
	}
	table, err := sharedDXCCTable()
	if err != nil {
		return fmt.Errorf("resolve exchange country: %w", err)
	}
	entity, ok := table.lookup(call)
	if !ok {
		return fmt.Errorf("cannot determine exchange country for %q", call)
	}
	if entity.Country == "Brazil" {
		if len(token) != 2 || !submissionName(token) {
			return fmt.Errorf("Brazilian exchange must be a two-letter state abbreviation")
		}
		return nil
	}
	if !map[string]bool{"AF": true, "AN": true, "AS": true, "EU": true, "NA": true, "OC": true, "SA": true}[token] {
		return fmt.Errorf("non-Brazilian exchange must be a continent abbreviation")
	}
	return nil
}

var arsiStateCodes = map[string]bool{
	"AP": true, "AR": true, "AS": true, "BR": true, "CH": true, "CG": true, "DD": true,
	"DL": true, "DN": true, "GA": true, "GJ": true, "HR": true, "HP": true, "JK": true,
	"JH": true, "KA": true, "KL": true, "LA": true, "LD": true, "MP": true, "MH": true,
	"MN": true, "ML": true, "MZ": true, "NL": true, "OD": true, "PY": true, "PB": true,
	"RJ": true, "SK": true, "TN": true, "TG": true, "TR": true, "UP": true, "UK": true,
	"WB": true, "AN": true,
}

func validateARSIExchange(call, token string) error {
	table, err := sharedDXCCTable()
	if err != nil {
		return fmt.Errorf("resolve exchange country: %w", err)
	}
	entity, ok := table.lookup(call)
	if !ok {
		return fmt.Errorf("cannot determine exchange country for %q", call)
	}
	if entity.Country == "India" {
		if !arsiStateCodes[token] {
			return fmt.Errorf("Indian exchange must be a listed state or UT code")
		}
		return nil
	}
	if !positiveSerial(token) {
		return fmt.Errorf("non-Indian exchange must be a positive serial")
	}
	return nil
}

func validateJIDXExchange(call, token string) error {
	table, err := sharedDXCCTable()
	if err != nil {
		return fmt.Errorf("resolve exchange country: %w", err)
	}
	entity, ok := table.lookup(call)
	if !ok {
		return fmt.Errorf("cannot determine exchange country for %q", call)
	}
	if entity.Country == "Japan" {
		if len(token) != 2 || !isAllDigits(token) || token == "00" || token > "50" {
			return fmt.Errorf("JA exchange must be a prefecture number from 01 through 50")
		}
		return nil
	}
	if !submissionZone(token, 40) {
		return fmt.Errorf("non-JA exchange must be a CQ zone from 1 through 40")
	}
	return nil
}

func validateQRPFoxHuntExchange(call, token string) error {
	table, err := sharedDXCCTable()
	if err != nil {
		return fmt.Errorf("resolve exchange country: %w", err)
	}
	entity, ok := table.lookup(call)
	if !ok {
		return fmt.Errorf("cannot determine exchange country for %q", call)
	}
	fields := strings.Fields(token)
	if len(fields) != 3 || !submissionName(fields[1]) || !validSubmissionPower(fields[2]) {
		return fmt.Errorf("QRP Fox Hunt exchange needs location, name, and positive power")
	}
	location := fields[0]
	if entity.Country == "United States" || entity.Country == "Canada" || entity.Country == "Alaska" || entity.Country == "Hawaii" {
		if !submissionArea(location, entity.Country, true) {
			return fmt.Errorf("North American exchange must use a valid state or province")
		}
		return nil
	}
	if !submissionName(location) {
		return fmt.Errorf("DX exchange must use a country abbreviation")
	}
	return nil
}

// validateBelgianSectionExchange validates the UBA contests' shape: a
// Belgian station sends a serial plus UBA section, everyone else sends a
// bare serial.
func validateBelgianSectionExchange(call, token string) error {
	table, err := sharedDXCCTable()
	if err != nil {
		return fmt.Errorf("resolve exchange country: %w", err)
	}
	entity, ok := table.lookup(call)
	if !ok {
		return fmt.Errorf("cannot determine exchange country for %q", call)
	}
	fields := strings.Fields(token)
	if entity.Country == "Belgium" {
		if len(fields) != 2 || !positiveSerial(fields[0]) || fields[1] == "" {
			return fmt.Errorf("exchange for Belgium must be a serial and UBA section")
		}
		return nil
	}
	if len(fields) != 1 || !positiveSerial(fields[0]) {
		return fmt.Errorf("exchange must be a single positive decimal serial")
	}
	return nil
}

// looksLikeCountyToken is a syntax-only check for the many state QSO
// parties' in-state county/parish/borough abbreviation — this app has no
// enumerated county roster for most states (unlike TNQP's tnCountyCodes), so
// it only rejects values that couldn't plausibly be one, mirroring
// validWAGDOK's own "syntax only" precedent.
func looksLikeCountyToken(s string) bool {
	if len(s) < 2 || len(s) > 6 {
		return false
	}
	for _, r := range s {
		if r < 'A' || r > 'Z' {
			return false
		}
	}
	return true
}

// validateCountyOrAreaExchange validates the widely used "in-state county;
// out-of-state state/province; DX" state QSO party shape shared by several
// catalog events with no per-state county roster.
func validateCountyOrAreaExchange(call, token string) error {
	table, err := sharedDXCCTable()
	if err != nil {
		return fmt.Errorf("resolve exchange country: %w", err)
	}
	entity, ok := table.lookup(call)
	if !ok {
		return fmt.Errorf("cannot determine exchange country for %q", call)
	}
	if token == "DX" || submissionArea(token, entity.Country, true) || looksLikeCountyToken(token) {
		return nil
	}
	return fmt.Errorf("exchange must be a county code, state/province, or DX")
}

var azqpCountyCodes = map[string]bool{
	"APH": true, "CHS": true, "CNO": true, "GLA": true, "GHM": true,
	"GLE": true, "LPZ": true, "MCP": true, "MHV": true, "NVO": true,
	"PMA": true, "PNL": true, "SCZ": true, "YVP": true, "YMA": true,
}

func validateAZQPExchange(call, token string) error {
	table, err := sharedDXCCTable()
	if err != nil {
		return fmt.Errorf("resolve exchange country: %w", err)
	}
	entity, ok := table.lookup(call)
	if !ok {
		return fmt.Errorf("cannot determine exchange country for %q", call)
	}
	if azqpCountyCodes[token] || submissionArea(token, entity.Country, true) {
		return nil
	}
	if token != "" && entity.Country != "United States" && entity.Country != "Canada" && entity.Country != "Alaska" && entity.Country != "Hawaii" {
		for _, alias := range table.prefixByFirst[token[0]] {
			if alias.prefix == token && alias.entity.Country == entity.Country {
				return nil
			}
		}
	}
	return fmt.Errorf("AZQP exchange must be an AZ county, state/province, or DXCC prefix")
}

func validateSerialCountyOrAreaExchange(call, token, contest string) error {
	table, err := sharedDXCCTable()
	if err != nil {
		return fmt.Errorf("resolve exchange country: %w", err)
	}
	entity, ok := table.lookup(call)
	if !ok {
		return fmt.Errorf("cannot determine exchange country for %q", call)
	}
	fields := strings.Fields(token)
	if len(fields) != 2 || !positiveSerial(fields[0]) {
		return fmt.Errorf("%s exchange needs a positive serial and location", contest)
	}
	location := fields[1]
	if location == "DX" || submissionArea(location, entity.Country, true) || looksLikeCountyToken(location) {
		return nil
	}
	return fmt.Errorf("%s location must be a county, state/province, or DX", contest)
}

func validateIRTSCountyExchange(call, token string) error {
	table, err := sharedDXCCTable()
	if err != nil {
		return fmt.Errorf("resolve exchange country: %w", err)
	}
	entity, ok := table.lookup(call)
	if !ok {
		return fmt.Errorf("cannot determine exchange country for %q", call)
	}
	fields := strings.Fields(token)
	if entity.Country == "Ireland" || entity.Country == "Northern Ireland" {
		if len(fields) != 2 || !positiveSerial(fields[0]) || !submissionName(fields[1]) {
			return fmt.Errorf("EI/GI exchange must be a serial and county code")
		}
		return nil
	}
	if len(fields) != 1 || !positiveSerial(fields[0]) {
		return fmt.Errorf("non-EI/GI exchange must be one serial")
	}
	return nil
}

func validateAreaOrDXExchange(call, token, contest string) error {
	table, err := sharedDXCCTable()
	if err != nil {
		return fmt.Errorf("resolve exchange country: %w", err)
	}
	entity, ok := table.lookup(call)
	if !ok {
		return fmt.Errorf("cannot determine exchange country for %q", call)
	}
	if entity.Country == "United States" || entity.Country == "Canada" || entity.Country == "Alaska" || entity.Country == "Hawaii" {
		if submissionArea(token, entity.Country, true) {
			return nil
		}
		return fmt.Errorf("%s exchange must be a valid state/province", contest)
	}
	if token != "DX" {
		return fmt.Errorf("%s DX exchange must be DX", contest)
	}
	return nil
}

func validateUSCountyOrDXExchange(call, token, contest string) error {
	table, err := sharedDXCCTable()
	if err != nil {
		return fmt.Errorf("resolve exchange country: %w", err)
	}
	entity, ok := table.lookup(call)
	if !ok {
		return fmt.Errorf("cannot determine exchange country for %q", call)
	}
	if entity.Country == "United States" || entity.Country == "Canada" || entity.Country == "Alaska" || entity.Country == "Hawaii" {
		if submissionArea(token, entity.Country, true) || looksLikeCountyToken(token) {
			return nil
		}
		return fmt.Errorf("%s exchange must be a county or state/province", contest)
	}
	if token != "DX" {
		return fmt.Errorf("%s DX exchange must be DX", contest)
	}
	return nil
}

func validateUSCountyOrCountryExchange(call, token, contest string) error {
	table, err := sharedDXCCTable()
	if err != nil {
		return fmt.Errorf("resolve exchange country: %w", err)
	}
	entity, ok := table.lookup(call)
	if !ok {
		return fmt.Errorf("cannot determine exchange country for %q", call)
	}
	if entity.Country == "United States" || entity.Country == "Canada" || entity.Country == "Alaska" || entity.Country == "Hawaii" {
		if submissionArea(token, entity.Country, true) || looksLikeCountyToken(token) {
			return nil
		}
		return fmt.Errorf("%s exchange must be a county or state/province", contest)
	}
	if token != "" {
		for _, alias := range table.prefixByFirst[token[0]] {
			if alias.prefix == token && alias.entity.Country == entity.Country {
				return nil
			}
		}
	}
	return fmt.Errorf("%s DX exchange must be a matching DXCC prefix", contest)
}

// The MDC QSO Party publishes a finite county/city roster. cty.dat cannot
// distinguish a Maryland/DC station from another US/Canadian station, so this
// validates the sponsor's tokens while leaving that location relationship to
// the submitted log.
var mdcCountyCityCodes = map[string]bool{
	"ALY": true, "ANA": true, "BAL": true, "BCT": true, "CLV": true,
	"CLN": true, "CRL": true, "CEC": true, "CHS": true, "DRC": true,
	"FRD": true, "GAR": true, "HFD": true, "HWD": true, "KEN": true,
	"MON": true, "PGE": true, "QAN": true, "STM": true, "SMR": true,
	"TAL": true, "WAS": true, "WIC": true, "WRC": true, "WDC": true,
}

func validateMDCExchange(call, token string) error {
	table, err := sharedDXCCTable()
	if err != nil {
		return fmt.Errorf("resolve exchange country: %w", err)
	}
	entity, ok := table.lookup(call)
	if !ok {
		return fmt.Errorf("cannot determine exchange country for %q", call)
	}
	if entity.Country == "United States" || entity.Country == "Canada" || entity.Country == "Alaska" || entity.Country == "Hawaii" {
		if mdcCountyCityCodes[token] || submissionArea(token, entity.Country, true) {
			return nil
		}
		return fmt.Errorf("MDC QSO Party exchange must be an MDC county/city or state/province")
	}
	if token != "" {
		for _, alias := range table.prefixByFirst[token[0]] {
			if alias.prefix == token && alias.entity.Country == entity.Country {
				return nil
			}
		}
	}
	return fmt.Errorf("MDC QSO Party DX exchange must be a matching DXCC prefix")
}

// KYPOTA's 2026 rules publish these 60 Kentucky park and national-site IDs.
// cty.dat has no state-level location for a US callsign, so a listed park ID
// is accepted for any call whose reported operating location cannot be known
// from its callsign alone.
var kentuckyParkCodes = map[string]bool{
	"BRL": true, "KL": true, "BBL": true, "KC": true, "BLB": true,
	"LB": true, "BI": true, "LCR": true, "BLR": true, "LM": true,
	"CC": true, "LJW": true, "CCR": true, "LH": true, "CB": true,
	"MM": true, "CF": true, "MKH": true, "DH": true, "NB": true,
	"DLR": true, "NL": true, "DTW": true, "OFH": true, "TS": true,
	"OMM": true, "FB": true, "PL": true, "GBI": true, "PF": true,
	"GB": true, "PB": true, "GL": true, "PMR": true, "GRL": true,
	"PMT": true, "GLR": true, "RRD": true, "ISC": true, "TL": true,
	"JD": true, "WSH": true, "JW": true, "WH": true, "JJA": true,
	"WM": true, "KLR": true, "WWH": true, "KDV": true, "YL": true,
	"ALB": true, "BSF": true, "CN": true, "CG": true, "FD": true,
	"LAC": true, "MC": true, "MSB": true, "TT": true, "BL": true,
}

func validateKentuckyParksExchange(call, token string) error {
	table, err := sharedDXCCTable()
	if err != nil {
		return fmt.Errorf("resolve exchange country: %w", err)
	}
	entity, ok := table.lookup(call)
	if !ok {
		return fmt.Errorf("cannot determine exchange country for %q", call)
	}
	if kentuckyParkCodes[token] || token == "KENTUCKY" {
		return nil
	}
	if entity.Country == "United States" || entity.Country == "Canada" {
		if submissionArea(token, entity.Country, true) {
			return nil
		}
		return fmt.Errorf("KYPOTA exchange must be a Kentucky park ID, Kentucky, or state/province")
	}
	if token == "DX" {
		return nil
	}
	return fmt.Errorf("KYPOTA exchange must be DX outside the continental US and Canada")
}

// validEURegionCode validates the contiguous country-region ranges published
// in the EUDX Contest 2026 rules. A callsign alone cannot distinguish all EU
// overseas operating locations, so the caller is intentionally not used to
// select between this form and the non-EU ITU-zone form.
var euRegionMaximum = map[string]int{
	"AT": 9, "BE": 11, "BG": 6, "CZ": 14, "CY": 5, "HR": 5,
	"DK": 5, "EE": 5, "FI": 19, "FR": 20, "DE": 16, "GR": 13,
	"HU": 7, "IE": 4, "IT": 21, "LV": 6, "LT": 5, "LX": 1,
	"MT": 5, "NL": 13, "PL": 16, "PT": 7, "RO": 8, "SK": 8,
	"SI": 6, "ES": 19, "SE": 21,
}

func validEURegionCode(token string) bool {
	if len(token) != 4 || !submissionName(token[:2]) || !isAllDigits(token[2:]) {
		return false
	}
	maximum, ok := euRegionMaximum[token[:2]]
	if !ok {
		return false
	}
	value, err := strconv.Atoi(token[2:])
	return err == nil && value >= 1 && value <= maximum
}

var arrl160WVEEntities = map[string]bool{
	"United States": true, "Canada": true, "Alaska": true, "Hawaii": true,
	"Puerto Rico": true, "Virgin Islands": true, "Guam": true,
	"Northern Mariana Islands": true, "American Samoa": true,
	"Wake Island": true, "Midway Island": true, "Palmyra & Jarvis Islands": true,
}

func validateARRL160Exchange(call, token string) error {
	table, err := sharedDXCCTable()
	if err != nil {
		return fmt.Errorf("resolve exchange country: %w", err)
	}
	entity, ok := table.lookup(call)
	if !ok {
		return fmt.Errorf("cannot determine exchange country for %q", call)
	}
	if arrl160WVEEntities[entity.Country] {
		if arrlSectionCode(token) == "" {
			return fmt.Errorf("ARRL 160m W/VE exchange must be an ARRL/RAC section")
		}
		return nil
	}
	if token != "" {
		return fmt.Errorf("ARRL 160m DX exchange must be blank after RST")
	}
	return nil
}

func validateDTCExchange(call, token string) error {
	table, err := sharedDXCCTable()
	if err != nil {
		return fmt.Errorf("resolve exchange country: %w", err)
	}
	entity, ok := table.lookup(call)
	if !ok {
		return fmt.Errorf("cannot determine exchange country for %q", call)
	}
	if entity.Country == "Fed. Rep. of Germany" {
		if len(token) < 1 || len(token) > 3 || !submissionName(token) {
			return fmt.Errorf("DTC German exchange must be a one- to three-letter district code")
		}
		return nil
	}
	if token != "" {
		return fmt.Errorf("DTC non-German exchange must be blank after RST")
	}
	return nil
}

func validateUSCountyOrCountryOrDXExchange(call, token, contest string) error {
	if err := validateUSCountyOrCountryExchange(call, token, contest); err == nil {
		return nil
	}
	table, err := sharedDXCCTable()
	if err != nil {
		return fmt.Errorf("resolve exchange country: %w", err)
	}
	entity, ok := table.lookup(call)
	if !ok {
		return fmt.Errorf("cannot determine exchange country for %q", call)
	}
	if token == "DX" && entity.Country != "United States" && entity.Country != "Canada" && entity.Country != "Alaska" && entity.Country != "Hawaii" {
		return nil
	}
	return fmt.Errorf("%s exchange must be a county, state/province, matching DXCC prefix, or DX", contest)
}

func validatePAQPExchange(token string) error {
	fields := strings.Fields(token)
	if len(fields) != 2 || !positiveSerial(fields[0]) {
		return fmt.Errorf("PA QSO Party exchange needs a positive serial and location")
	}
	location := fields[1]
	if location == "DX" || arrlSectionCode(location) != "" || (len(location) == 3 && submissionName(location)) {
		return nil
	}
	return fmt.Errorf("PA QSO Party location must be a PA county, ARRL/Canadian section, or DX")
}

func validateMinnesotaExchange(call, token string) error {
	table, err := sharedDXCCTable()
	if err != nil {
		return fmt.Errorf("resolve exchange country: %w", err)
	}
	entity, ok := table.lookup(call)
	if !ok {
		return fmt.Errorf("cannot determine exchange country for %q", call)
	}
	fields := strings.Fields(token)
	if entity.Country != "United States" && entity.Country != "Canada" && entity.Country != "Alaska" && entity.Country != "Hawaii" {
		if len(fields) == 1 && submissionName(fields[0]) {
			return nil
		}
		return fmt.Errorf("Minnesota QSO Party DX exchange must be one name")
	}
	if len(fields) != 2 || !submissionName(fields[0]) {
		return fmt.Errorf("Minnesota QSO Party exchange needs a name and location")
	}
	if submissionArea(fields[1], entity.Country, true) || looksLikeCountyToken(fields[1]) {
		return nil
	}
	return fmt.Errorf("Minnesota QSO Party location must be a state/province or county code")
}

func validateKentuckyExchange(call, token string) error {
	if err := validateUSCountyOrDXExchange(call, token, "Kentucky QSO Party"); err == nil {
		return nil
	}
	table, err := sharedDXCCTable()
	if err != nil {
		return fmt.Errorf("resolve exchange country: %w", err)
	}
	entity, ok := table.lookup(call)
	if !ok {
		return fmt.Errorf("cannot determine exchange country for %q", call)
	}
	if entity.Country != "United States" && entity.Country != "Canada" && entity.Country != "Alaska" && entity.Country != "Hawaii" {
		return fmt.Errorf("Kentucky QSO Party DX exchange must be DX")
	}
	parts := strings.Split(token, "/")
	if len(parts) == 2 && looksLikeCountyToken(parts[0]) && looksLikeCountyToken(parts[1]) {
		return nil
	}
	return fmt.Errorf("Kentucky QSO Party exchange must be a county, state/province, DX, or two county codes")
}

func validateRACCanadaExchange(call, token string) error {
	if province, ok := racProvinceForCall(call); ok {
		if token == province {
			return nil
		}
		return fmt.Errorf("RAC Canada contest exchange for %q must be %s", call, province)
	}
	if !positiveSerial(token) {
		return fmt.Errorf("RAC Canada contest non-Canadian exchange must be a positive serial")
	}
	return nil
}

func validateEAQRPExchange(token string) error {
	fields := strings.Fields(token)
	if len(fields) < 1 || len(fields) > 2 || len(fields[0]) != 1 || !strings.ContainsRune("ABCD", rune(fields[0][0])) {
		return fmt.Errorf("EA-QRP CW Contest exchange must start with category A, B, C, or D")
	}
	if len(fields) == 2 && fields[1] != "M" {
		return fmt.Errorf("EA-QRP CW Contest optional second exchange field must be M")
	}
	return nil
}

func validate7QPExchange(call, token string) error {
	table, err := sharedDXCCTable()
	if err != nil {
		return fmt.Errorf("resolve exchange country: %w", err)
	}
	entity, ok := table.lookup(call)
	if !ok {
		return fmt.Errorf("cannot determine exchange country for %q", call)
	}
	if token == "DX" || submissionArea(token, entity.Country, true) {
		return nil
	}
	if len(token) == 5 && submissionName(token) {
		return nil
	}
	return fmt.Errorf("7QP exchange must be a state/province, DX, or five-character state/county code")
}

func validateNewYorkExchange(call, token string) error {
	table, err := sharedDXCCTable()
	if err != nil {
		return fmt.Errorf("resolve exchange country: %w", err)
	}
	entity, ok := table.lookup(call)
	if !ok {
		return fmt.Errorf("cannot determine exchange country for %q", call)
	}
	if entity.Country != "United States" && entity.Country != "Canada" && entity.Country != "Alaska" && entity.Country != "Hawaii" {
		if token == "DX" {
			return nil
		}
		return fmt.Errorf("New York QSO Party DX exchange must be DX")
	}
	if submissionArea(token, entity.Country, true) || (len(token) == 3 && submissionName(token)) || (len(token) == 6 && submissionName(token)) {
		return nil
	}
	parts := strings.Split(token, "/")
	if len(parts) == 2 && len(parts[0]) == 3 && len(parts[1]) == 3 && submissionName(parts[0]) && submissionName(parts[1]) {
		return nil
	}
	return fmt.Errorf("New York QSO Party exchange must be a state/province or one/two three-letter county codes")
}

func validateLZDXExchange(call, token string) error {
	table, err := sharedDXCCTable()
	if err != nil {
		return fmt.Errorf("resolve exchange country: %w", err)
	}
	entity, ok := table.lookup(call)
	if !ok {
		return fmt.Errorf("cannot determine exchange country for %q", call)
	}
	if entity.Country == "Bulgaria" {
		if len(token) == 2 && submissionName(token) {
			return nil
		}
		return fmt.Errorf("LZ DX Contest Bulgarian exchange must be a two-letter district")
	}
	zone, err := strconv.Atoi(token)
	if err != nil || zone < 1 || zone > 90 {
		return fmt.Errorf("LZ DX Contest non-LZ exchange must be an ITU zone from 1 to 90")
	}
	return nil
}

var quebecRegionCodes = map[string]bool{
	"BSA": true, "SLS": true, "QUE": true, "MAU": true, "ETE": true, "MTL": true,
	"OTS": true, "ATE": true, "CND": true, "NDQ": true, "GIM": true, "CAS": true,
	"LVL": true, "LDE": true, "LNS": true, "MEE": true, "CDQ": true,
}

var quebecNonQuebecCanadaAreas = map[string]bool{
	"NF": true, "LB": true, "PE": true, "NB": true, "NS": true, "ON": true,
	"MB": true, "SK": true, "AB": true, "BC": true, "NT": true, "YT": true,
	"NU": true, "NWT": true,
}

func quebecCall(call string) bool {
	call = strings.ToUpper(call)
	if strings.HasSuffix(call, "/VE2") {
		return true
	}
	for _, prefix := range []string{"VA2", "VB2", "VE2", "VX2"} {
		if strings.HasPrefix(call, prefix) {
			return true
		}
	}
	return false
}

// validateQuebecExchange follows the sponsor's explicit physical-Quebec call
// rule. A station from Quebec may use a non-Quebec prefix only with /VE2.
func validateQuebecExchange(call, token string) error {
	if quebecCall(call) {
		if !quebecRegionCodes[token] {
			return fmt.Errorf("Quebec exchange must be one of the listed administrative-region codes")
		}
		return nil
	}
	table, err := sharedDXCCTable()
	if err != nil {
		return fmt.Errorf("resolve exchange country: %w", err)
	}
	entity, ok := table.lookup(call)
	if !ok {
		return fmt.Errorf("cannot determine exchange country for %q", call)
	}
	if entity.Country == "Canada" {
		if !quebecNonQuebecCanadaAreas[token] {
			return fmt.Errorf("non-Quebec Canadian exchange must be a listed province or territory")
		}
		return nil
	}
	if entity.Country == "United States" || entity.Country == "Alaska" || entity.Country == "Hawaii" {
		if !submissionArea(token, entity.Country, true) && token != "DC" {
			return fmt.Errorf("US exchange must be a state abbreviation")
		}
		return nil
	}
	if token != "DX" {
		return fmt.Errorf("non-US/Canadian exchange must be DX")
	}
	return nil
}

// validateAtlanticCanadaExchange uses RAC's published call-area mapping for
// the in-region provinces. The final three county/division letters are
// syntax-checked because the ACQP roster is not bundled with the application.
func validateAtlanticCanadaExchange(call, token string) error {
	if province, ok := racProvinceForCall(call); ok && (province == "NS" || province == "NB" || province == "NL" || province == "PE") {
		if len(token) != 5 || !strings.HasPrefix(token, province) || !submissionName(token[2:]) {
			return fmt.Errorf("Atlantic Canada exchange must be province plus a three-letter county or division code")
		}
		return nil
	}
	table, err := sharedDXCCTable()
	if err != nil {
		return fmt.Errorf("resolve exchange country: %w", err)
	}
	entity, ok := table.lookup(call)
	if !ok {
		return fmt.Errorf("cannot determine exchange country for %q", call)
	}
	if entity.Country == "United States" || entity.Country == "Canada" || entity.Country == "Alaska" || entity.Country == "Hawaii" {
		if !submissionArea(token, entity.Country, true) {
			return fmt.Errorf("out-of-region W/VE exchange must be a state or province abbreviation")
		}
		return nil
	}
	if token != "DX" {
		return fmt.Errorf("out-of-region DX exchange must be DX")
	}
	return nil
}

// validateCPQPExchange applies the Canadian Prairies QSO Party exchange.
// Its sponsor publishes the current 62 district abbreviations as a map rather
// than a text table, so the in-Prairie form is checked to the published
// three-letter grammar. The VA/VE prefix mapping is sufficient to distinguish
// the three Prairie provinces from every other Canadian call area.
func validateCPQPExchange(call, token string) error {
	if province, ok := racProvinceForCall(call); ok && (province == "MB" || province == "SK" || province == "AB") {
		if len(token) != 3 || !submissionName(token) {
			return fmt.Errorf("Canadian Prairies exchange must be a three-letter federal district abbreviation")
		}
		return nil
	}
	table, err := sharedDXCCTable()
	if err != nil {
		return fmt.Errorf("resolve exchange country: %w", err)
	}
	entity, ok := table.lookup(call)
	if !ok {
		return fmt.Errorf("cannot determine exchange country for %q", call)
	}
	if entity.Country == "United States" || entity.Country == "Canada" || entity.Country == "Alaska" || entity.Country == "Hawaii" {
		if submissionArea(token, entity.Country, true) {
			return nil
		}
		return fmt.Errorf("out-of-Prairie Canadian or US exchange must be a state or province abbreviation")
	}
	if token != "DX" {
		return fmt.Errorf("out-of-Prairie DX exchange must be DX")
	}
	return nil
}

// ILQP's sponsor publishes this exact county-abbreviation chart. A callsign
// cannot determine a US operator's state, so a published Illinois county and
// the documented out-of-state area forms are both accepted here.
const ilqpCountyCodes = " ADAM ALEX BOND BOON BROW BURO CALH CARR CASS CHAM CHRS CLRK CLAY CLNT COLE COOK CRAW CUMB DEKA DEWT DOUG DUPG EDGR EDWA EFFG FAYE FORD FRNK FULT GALL GREE GRUN HAML HANC HARD HNDR HENR IROQ JACK JASP JEFF JERS JOHN KANE KANK KEND KNOX LAKE LASA LAWR LEE LIVG LOGN MACN MCPN MADN MARI MSHL MASN MSSC MCHE MCDN MCLN MNRD MRCR MNRO MNTG MORG MOUL MARN MARP MEND MERC MODO MONO MONT NAPA OGLE PEOR PERR PIAT PIKE POPE PULA PUTN RAND RICH ROCK SALI SANG SCHY SCOT SHEL SIER SISK SCLA SOLA STAR STEP TAZW UNIO VERM WABA WARR WASH WAYN WHIT WTSD WILL WMSN WBGO WOOD "

func validateILQPExchange(call, token string) error {
	if strings.Contains(ilqpCountyCodes, " "+token+" ") {
		return nil
	}
	table, err := sharedDXCCTable()
	if err != nil {
		return fmt.Errorf("resolve exchange country: %w", err)
	}
	entity, ok := table.lookup(call)
	if !ok {
		return fmt.Errorf("cannot determine exchange country for %q", call)
	}
	if entity.Country == "United States" || entity.Country == "Canada" || entity.Country == "Alaska" || entity.Country == "Hawaii" {
		if submissionArea(token, entity.Country, true) {
			return nil
		}
		return fmt.Errorf("Illinois QSO Party exchange must be an Illinois county or state/province abbreviation")
	}
	if token != "DX" {
		return fmt.Errorf("Illinois QSO Party DX exchange must be DX")
	}
	return nil
}

// INQP's 2026 county chart is the source for these exact five-character forms.
const inqpCountyCodes = " INADA INALL INBAR INBEN INBLA INBOO INBRO INCAR INCAS INCLR INCLY INCLI INCRA INDAV INDEA INDEC INDEK INDEL INDUB INELK INFAY INFLO INFOU INFRA INFUL INGIB INGRA INGRE INHAM INHAN INHAR INHND INHNR INHOW INHUN INJAC INJAS INJAY INJEF INJEN INJOH INKNO INKOS INLAG INLAK INLAP INLAW INMAD INMRN INMRS INMRT INMIA INMNR INMNT INMOR INNEW INNOB INOHI INORA INOWE INPAR INPER INPIK INPOR INPOS INPUL INPUT INRAN INRIP INRUS INSCO INSHE INSPE INSTA INSTE INSTJ INSUL INSWI INTPP INTPT INUNI INVAN INVER INVIG INWAB INWRN INWRK INWAS INWAY INWEL INWHT INWHL "

func validateINQPExchange(call, token string) error {
	if strings.Contains(inqpCountyCodes, " "+token+" ") {
		return nil
	}
	table, err := sharedDXCCTable()
	if err != nil {
		return fmt.Errorf("resolve exchange country: %w", err)
	}
	entity, ok := table.lookup(call)
	if !ok {
		return fmt.Errorf("cannot determine exchange country for %q", call)
	}
	if entity.Country == "United States" || entity.Country == "Canada" || entity.Country == "Alaska" || entity.Country == "Hawaii" {
		if submissionArea(token, entity.Country, true) {
			return nil
		}
		return fmt.Errorf("Indiana QSO Party exchange must be an Indiana county or state/province abbreviation")
	}
	if token != "DX" {
		return fmt.Errorf("Indiana QSO Party DX exchange must be DX")
	}
	return nil
}

const nmqpCountyCodes = " BER CAT CHA CIB COL CUR DEB DON EDD GRA GUA HAR HID LEA LIN LOS LUN MCK MOR OTE QUA RIO ROO SJU SMI SAN SFE SIE SOC TAO TOR UNI VAL "

func validateNMQPExchange(call, token string) error {
	if strings.Contains(nmqpCountyCodes, " "+token+" ") {
		return nil
	}
	table, err := sharedDXCCTable()
	if err != nil {
		return fmt.Errorf("resolve exchange country: %w", err)
	}
	entity, ok := table.lookup(call)
	if !ok {
		return fmt.Errorf("cannot determine exchange country for %q", call)
	}
	if entity.Country == "United States" || entity.Country == "Canada" || entity.Country == "Alaska" || entity.Country == "Hawaii" {
		if submissionArea(token, entity.Country, true) {
			return nil
		}
		return fmt.Errorf("New Mexico QSO Party exchange must be a New Mexico county or state/province abbreviation")
	}
	if token != "DX" {
		return fmt.Errorf("New Mexico QSO Party DX exchange must be DX")
	}
	return nil
}

func validateOKOMDXExchange(call, token string) error {
	table, err := sharedDXCCTable()
	if err != nil {
		return fmt.Errorf("resolve exchange country: %w", err)
	}
	entity, ok := table.lookup(call)
	if !ok {
		return fmt.Errorf("cannot determine exchange country for %q", call)
	}
	if entity.Country == "Czech Republic" || entity.Country == "Slovak Republic" {
		if len(token) != 3 || !submissionName(token) {
			return fmt.Errorf("OK/OM DX station exchange must be a three-letter district code")
		}
		return nil
	}
	if !positiveSerial(token) {
		return fmt.Errorf("OK/OM DX outside station exchange must be a positive serial")
	}
	return nil
}

func usIslandReference(token string) bool {
	if len(token) == 5 && strings.HasSuffix(token, "NEW") && submissionName(token[:2]) {
		return true
	}
	if len(token) != 5 && len(token) != 6 {
		return false
	}
	if !submissionName(token[:2]) || !isAllDigits(token[2:5]) {
		return false
	}
	return len(token) == 5 || token[5] == 'S' || token[5] == 'L'
}

func validateUSIslandExchange(call, token string) error {
	table, err := sharedDXCCTable()
	if err != nil {
		return fmt.Errorf("resolve exchange country: %w", err)
	}
	entity, ok := table.lookup(call)
	if !ok {
		return fmt.Errorf("cannot determine exchange country for %q", call)
	}
	if entity.Country == "United States" || entity.Country == "Alaska" || entity.Country == "Hawaii" {
		if submissionArea(token, entity.Country, true) || usIslandReference(token) {
			return nil
		}
		return fmt.Errorf("US Islands exchange must be a state or USI island reference")
	}
	if token != "DX" {
		return fmt.Errorf("non-US Islands exchange must be DX")
	}
	return nil
}

func validateTurkiyeExchange(call, token string) error {
	table, err := sharedDXCCTable()
	if err != nil {
		return fmt.Errorf("resolve exchange country: %w", err)
	}
	entity, ok := table.lookup(call)
	if !ok {
		return fmt.Errorf("cannot determine exchange country for %q", call)
	}
	if entity.Country == "European Turkey" || entity.Country == "Asiatic Turkey" {
		if len(token) != 2 || !isAllDigits(token) || !submissionZone(token, 81) {
			return fmt.Errorf("Turkiye exchange must be a province code from 01 through 81")
		}
		return nil
	}
	if !positiveSerial(token) {
		return fmt.Errorf("non-Turkiye exchange must be a positive serial")
	}
	return nil
}

func validateDARC10Exchange(call, token string) error {
	table, err := sharedDXCCTable()
	if err != nil {
		return fmt.Errorf("resolve exchange country: %w", err)
	}
	entity, ok := table.lookup(call)
	if !ok {
		return fmt.Errorf("cannot determine exchange country for %q", call)
	}
	if entity.Country == "Fed. Rep. of Germany" {
		fields := strings.Fields(token)
		if len(fields) != 2 || !positiveSerial(fields[0]) || !validWAGDOK(fields[1]) {
			return fmt.Errorf("German exchange must be a serial and DOK or NM")
		}
		return nil
	}
	if !positiveSerial(token) {
		return fmt.Errorf("non-German exchange must be a positive serial")
	}
	return nil
}

func frenchExchangeEntity(country string) bool {
	switch country {
	case "France", "Corsica", "Guadeloupe", "Martinique", "French Guiana", "Reunion", "Mayotte", "Saint Pierre & Miquelon":
		return true
	default:
		return false
	}
}

func frenchDepartmentCode(token string) bool {
	if token == "2A" || token == "2B" {
		return true
	}
	return len(token) >= 1 && len(token) <= 3 && submissionZone(token, 999)
}

// validateREF160Exchange follows REF's French-station department versus
// foreign-station serial exchange. Department values are syntax-checked: the
// live REF rules define the field but do not publish a compact roster here.
func validateREF160Exchange(call, token string) error {
	table, err := sharedDXCCTable()
	if err != nil {
		return fmt.Errorf("resolve exchange country: %w", err)
	}
	entity, ok := table.lookup(call)
	if !ok {
		return fmt.Errorf("cannot determine exchange country for %q", call)
	}
	if frenchExchangeEntity(entity.Country) {
		if !frenchDepartmentCode(token) {
			return fmt.Errorf("French exchange must be a department code")
		}
		return nil
	}
	if !positiveSerial(token) {
		return fmt.Errorf("foreign exchange must be a positive serial")
	}
	return nil
}

var refOverseasPrefix = map[string]string{
	"Guadeloupe": "FG", "Martinique": "FM", "French Guiana": "FY",
	"Reunion": "FR", "Mayotte": "FH", "Saint Pierre & Miquelon": "FP",
}

// validateREFCWExchange follows Coupe du REF's department/prefix exchange for
// French stations and positive serial exchange for foreign stations.
func validateREFCWExchange(call, token string) error {
	table, err := sharedDXCCTable()
	if err != nil {
		return fmt.Errorf("resolve exchange country: %w", err)
	}
	entity, ok := table.lookup(call)
	if !ok {
		return fmt.Errorf("cannot determine exchange country for %q", call)
	}
	if prefix, overseas := refOverseasPrefix[entity.Country]; overseas {
		if token != prefix {
			return fmt.Errorf("French overseas exchange must be prefix %s", prefix)
		}
		return nil
	}
	if entity.Country == "France" || entity.Country == "Corsica" {
		if !frenchDepartmentCode(token) {
			return fmt.Errorf("French exchange must be a department code")
		}
		return nil
	}
	if !positiveSerial(token) {
		return fmt.Errorf("foreign exchange must be a positive serial")
	}
	return nil
}

// racProvinceForCall is the RAC-published prefix table used by its Canada Day
// and Canada Winter contest rules. VE0 is deliberately absent: RAC requires a
// serial for its maritime-mobile stations.
func racProvinceForCall(call string) (string, bool) {
	call = strings.ToUpper(strings.TrimSpace(call))
	for prefix, province := range map[string]string{
		"VE1": "NS", "VA1": "NS", "CY9": "NS", "CY0": "NS",
		"VE2": "QC", "VA2": "QC",
		"VE3": "ON", "VA3": "ON",
		"VE4": "MB", "VA4": "MB",
		"VE5": "SK", "VA5": "SK",
		"VE6": "AB", "VA6": "AB",
		"VE7": "BC", "VA7": "BC",
		"VE8": "NT", "VE9": "NB",
		"VO1": "NL", "VO2": "NL",
		"VY0": "NU", "VY1": "YT", "VY2": "PE",
	} {
		if strings.HasPrefix(call, prefix) {
			return province, true
		}
	}
	return "", false
}

// validateSPCExchange validates the "location [+ name] [+ member no./power]"
// shape shared by several QRP/sprint contests: location must be a real
// state/province/prefix match or "DX"; an optional name field only needs to
// look alphabetic; an optional trailing field accepts a positive member
// number or power value, or an explicit non-member marker ("NONE"/"NM").
func validateSPCExchange(call, token string, hasName bool) error {
	table, err := sharedDXCCTable()
	if err != nil {
		return fmt.Errorf("resolve exchange country: %w", err)
	}
	entity, ok := table.lookup(call)
	if !ok {
		return fmt.Errorf("cannot determine exchange country for %q", call)
	}
	fields := strings.Fields(token)
	if len(fields) == 0 {
		return fmt.Errorf("missing exchange location")
	}
	location := fields[0]
	if !(location == "DX" || submissionArea(location, entity.Country, true) || looksLikeCountyToken(location)) {
		return fmt.Errorf("invalid exchange location %q", location)
	}
	fields = fields[1:]
	if hasName {
		if len(fields) == 0 || !submissionName(fields[0]) {
			return fmt.Errorf("exchange needs one alphabetic name after the location")
		}
		fields = fields[1:]
	}
	switch {
	case len(fields) == 0:
		return nil
	case len(fields) > 1:
		return fmt.Errorf("exchange has unexpected trailing fields")
	case fields[0] == "NONE" || fields[0] == "NM" || validSubmissionPower(fields[0]):
		return nil
	default:
		return fmt.Errorf("trailing field must be a member number, power, or %q/%q", "NONE", "NM")
	}
}

func validateHolidaySpiritsExchange(call, token string) error {
	if err := validateSPCExchange(call, token, false); err != nil {
		return err
	}
	fields := strings.Fields(token)
	if len(fields) != 2 || (fields[1] != "NM" && !positiveSerial(fields[1]) && !validSubmissionPower(fields[1])) {
		return fmt.Errorf("Holiday Spirits exchange needs location and ARCI number or power")
	}
	return nil
}
