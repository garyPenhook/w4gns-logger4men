package main

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"
)

// The exchanged locations, persisted in STX_STRING/SRX_STRING, are the
// authoritative station-location snapshots. A callsign never determines state.
type qsoPartyRules struct {
	Periods                []partyPeriod     `json:"periods"`
	State                  string            `json:"state"`
	Exchange               string            `json:"exchange"` // location, serial_location, name_location
	CountyLineMax          int               `json:"county_line_max"`
	CountyAsState          bool              `json:"county_as_state,omitempty"`
	HomeMultipliers        []string          `json:"home_multipliers"` // county, state, dxcc
	MultiplierPer          string            `json:"multiplier_per"`
	MultiplierCap          int               `json:"multiplier_cap,omitempty"`
	Points                 int               `json:"points"`
	BonusCall              string            `json:"bonus_call,omitempty"`
	BonusPoints            int               `json:"bonus_points,omitempty"`
	MobileCountyMinimum    int               `json:"mobile_county_minimum,omitempty"`
	MobileCountyBonus      int               `json:"mobile_county_bonus,omitempty"`
	MobileCountyMultiplier bool              `json:"mobile_county_multiplier,omitempty"`
	AreaAliases            map[string]string `json:"area_aliases,omitempty"`
	OutsideCategory        string            `json:"outside_category,omitempty"`
	CategorySoapbox        bool              `json:"category_soapbox,omitempty"`
	CategoryMode           string            `json:"category_mode,omitempty"`
	PowerFactors           map[string]int    `json:"power_factors,omitempty"`
	QSOOnlyCalls           []string          `json:"qso_only_calls,omitempty"`
}

type partyPeriod struct {
	Start time.Time `json:"start"`
	End   time.Time `json:"end"`
}

func (e eventDefinition) partyInPeriod(at time.Time) bool {
	for _, p := range e.QSOParty.Periods {
		if !at.Before(p.Start) && at.Before(p.End) {
			return true
		}
	}
	return false
}

type partyExchange struct {
	serial, name, area, location string
	counties                     []string
}

func (e eventDefinition) prepareQSOParty() error {
	r := e.QSOParty
	if r == nil {
		if e.DupeScope == "call+band+location" {
			return fmt.Errorf("event %q location-aware dupes require QSO party rules", e.ID)
		}
		return nil
	}
	if r.CategoryMode != "" && r.CategoryMode != "CW" && r.CategoryMode != "MIXED" {
		return fmt.Errorf("event %q has invalid category mode", e.ID)
	}
	for category, factor := range r.PowerFactors {
		if !containsString([]string{"QRP", "LOW", "HIGH"}, category) || factor < 1 {
			return fmt.Errorf("event %q has invalid power factor", e.ID)
		}
	}
	if e.DupeScope != "call+band+location" {
		return fmt.Errorf("event %q QSO party rules require location-aware dupes", e.ID)
	}
	if e.Scoring == nil || e.Scoring.Points != nil || e.DXScoring != nil || e.Scoring.PointsPerQSO != r.Points {
		return fmt.Errorf("event %q QSO party points conflict with base scoring", e.ID)
	}
	mults := e.Scoring.effectiveMultipliers()
	if len(mults) != 1 || mults[0].Kind != "county" || mults[0].Per != r.MultiplierPer {
		return fmt.Errorf("event %q QSO party county multiplier conflicts with base scoring", e.ID)
	}
	if len(r.Periods) == 0 {
		return fmt.Errorf("event %q requires verified operating periods", e.ID)
	}
	for i, p := range r.Periods {
		if p.Start.IsZero() || !p.End.After(p.Start) || (i > 0 && p.Start.Before(r.Periods[i-1].End)) {
			return fmt.Errorf("event %q has invalid/overlapping operating periods", e.ID)
		}
	}
	if len(e.CountyOptions) == 0 || !exchangeAreaCodes[r.State] || len(r.State) != 2 {
		return fmt.Errorf("event %q requires a state and county table", e.ID)
	}
	if r.Exchange != "location" && r.Exchange != "serial_location" && r.Exchange != "name_location" {
		return fmt.Errorf("event %q has unsupported QSO party exchange", e.ID)
	}
	if e.SentSerial != (r.Exchange == "serial_location") || r.CountyLineMax < 1 || r.CountyLineMax > 4 || r.Points < 1 || (r.MultiplierPer != "band" && r.MultiplierPer != "contest") || r.MultiplierCap < 0 {
		return fmt.Errorf("event %q has inconsistent QSO party rules", e.ID)
	}
	for _, kind := range r.HomeMultipliers {
		if kind != "county" && kind != "state" && kind != "dxcc" && kind != "dx" && kind != "maritime" {
			return fmt.Errorf("event %q has invalid home multiplier %q", e.ID, kind)
		}
	}
	if len(r.HomeMultipliers) == 0 || r.BonusPoints < 0 || (r.BonusPoints > 0 && r.BonusCall == "") || r.MobileCountyMinimum < 0 || r.MobileCountyBonus < 0 || ((r.MobileCountyBonus > 0 || r.MobileCountyMultiplier) && r.MobileCountyMinimum == 0) {
		return fmt.Errorf("event %q has invalid QSO party multipliers or bonuses", e.ID)
	}
	for from, to := range r.AreaAliases {
		if len(from) < 2 || len(from) > 4 || strings.Trim(from, "ABCDEFGHIJKLMNOPQRSTUVWXYZ") != "" || !partyPostalArea(to) || from == to || r.AreaAliases[to] != "" {
			return fmt.Errorf("event %q has invalid area aliases", e.ID)
		}
	}
	if r.OutsideCategory != "" && r.OutsideCategory != "FIXED" {
		return fmt.Errorf("event %q has invalid outside station category", e.ID)
	}
	return nil
}

func (e eventDefinition) partyArea(area string) string {
	if target := e.QSOParty.AreaAliases[area]; target != "" {
		return target
	}
	return area
}

func (e eventDefinition) partyExchange(serial, text string) (partyExchange, error) {
	var out partyExchange
	for _, c := range serial + text {
		if c < ' ' || c > '~' {
			return out, fmt.Errorf("exchange must contain printable ASCII")
		}
	}
	fields := strings.Fields(strings.ToUpper(cabrilloExchange(serial, text)))
	switch e.QSOParty.Exchange {
	case "serial_location":
		if len(fields) != 2 || !positiveSerial(fields[0]) {
			return out, fmt.Errorf("exchange needs a positive serial and location")
		}
		out.serial, fields = fields[0], fields[1:]
	case "name_location":
		if len(fields) != 2 || !submissionName(fields[0]) {
			return out, fmt.Errorf("exchange needs a name and location")
		}
		out.name, fields = fields[0], fields[1:]
	}
	if len(fields) != 1 {
		return out, fmt.Errorf("exchange needs one location (join county-line codes with /)")
	}
	location := strings.ReplaceAll(fields[0], "+", "/")
	out.location = location
	if containsString(e.QSOParty.HomeMultipliers, "maritime") {
		region := strings.TrimPrefix(location, "R")
		if region == "1" || region == "2" || region == "3" {
			out.area = "R" + region
			return out, nil
		}
	}
	parts := strings.Split(location, "/")
	seen := map[string]bool{}
	for _, part := range parts {
		if code := e.countyCode(part); code != "" && !seen[code] {
			out.counties = append(out.counties, code)
			seen[code] = true
		} else {
			out.counties = nil
			break
		}
	}
	if len(out.counties) > 0 {
		if len(out.counties) > e.QSOParty.CountyLineMax {
			return partyExchange{}, fmt.Errorf("at most %d counties per exchange", e.QSOParty.CountyLineMax)
		}
		sort.Strings(out.counties)
		return out, nil
	}
	if len(parts) != 1 {
		return out, fmt.Errorf("county-line exchange contains an unknown or repeated county")
	}
	explicitDX := strings.HasPrefix(location, "DX:")
	dxToken := strings.TrimPrefix(location, "DX:")
	if dxToken == "" {
		return out, fmt.Errorf("DX entity prefix is missing")
	}
	for _, ch := range dxToken {
		if (ch < 'A' || ch > 'Z') && (ch < '0' || ch > '9') {
			return out, fmt.Errorf("invalid location %q", location)
		}
	}
	// Postal abbreviations and sponsor-specific aliases (such as DC -> MD).
	if !explicitDX && location != e.QSOParty.State && (partyPostalArea(location) || location == "DX" || e.QSOParty.AreaAliases[location] != "") {
		out.area = e.partyArea(location)
		return out, nil
	}
	// TN exchanges a DXCC entity. Accept a cty.dat prefix only if it
	// resolves outside the US/Canada/AK/HI, not arbitrary unknown text.
	if containsString(e.QSOParty.HomeMultipliers, "dxcc") {
		table, err := sharedDXCCTable()
		if err != nil {
			return out, err
		}
		for _, alias := range table.prefixByFirst[dxToken[0]] {
			if alias.prefix == dxToken {
				if entity, ok := partyDXEntity(dxToken); ok {
					out.area = "DX:" + entity
					return out, nil
				}
			}
		}
	}
	return out, fmt.Errorf("unknown county or outside state/province/DX location %q", location)
}

func containsString(values []string, value string) bool {
	for _, s := range values {
		if s == value {
			return true
		}
	}
	return false
}

func partyPostalArea(s string) bool {
	return len(s) == 2 && strings.Contains(" AL AK AZ AR CA CO CT DE DC FL GA HI ID IL IN IA KS KY LA ME MD MA MI MN MS MO MT NE NV NH NJ NM NY NC ND OH OK OR PA RI SC SD TN TX UT VT VA WA WV WI WY AB BC MB NB NL NS NT NU ON PE QC SK YT ", " "+s+" ")
}

func partyDXEntity(call string) (string, bool) {
	table, err := sharedDXCCTable()
	if err != nil {
		return "", false
	}
	entity, ok := table.lookup(call)
	if !ok || entity.Country == "United States" || entity.Country == "Canada" || entity.Country == "Alaska" || entity.Country == "Hawaii" {
		return "", false
	}
	return entity.Country, true
}

func (e eventDefinition) partyLocations(x partyExchange) []string {
	if len(x.counties) > 0 {
		return x.counties
	}
	return []string{x.location}
}

// Each county pair is one independently deduplicated credit. Expanding a
// two-county contact preserves partial credit when one county was worked before.
func (e eventDefinition) partyCredits(q qso) ([]qso, error) {
	sent, err := e.partyExchange(q.stx, q.stxString)
	if err != nil {
		return nil, fmt.Errorf("sent exchange: %w", err)
	}
	received, err := e.partyExchange(q.srx, q.srxString)
	if err != nil {
		return nil, fmt.Errorf("received exchange: %w", err)
	}
	if len(sent.counties) == 0 && len(received.counties) == 0 {
		return nil, nil
	}
	if q.mode != "" && !strings.EqualFold(q.mode, "CW") {
		return nil, nil
	}
	if !containsString(e.Bands, strings.ToUpper(q.band)) {
		return nil, nil
	}
	var credits []qso
	for _, home := range e.partyLocations(sent) {
		for _, away := range e.partyLocations(received) {
			credit := q
			credit.stx, credit.srx = sent.serial, received.serial
			credit.stxString, credit.srxString = home, away
			if sent.name != "" {
				credit.stxString = sent.name + " " + credit.stxString
			}
			if received.name != "" {
				credit.srxString = received.name + " " + credit.srxString
			}
			credits = append(credits, credit)
		}
	}
	return credits, nil
}

func (e eventDefinition) partyCreditKey(q qso) string {
	sent, _ := e.partyExchange(q.stx, q.stxString)
	received, _ := e.partyExchange(q.srx, q.srxString)
	location := func(x partyExchange, call string) string {
		if len(x.counties) > 0 {
			return strings.Join(x.counties, "/")
		}
		if x.area == "DX" {
			if entity, ok := partyDXEntity(call); ok {
				return "DX:" + entity
			}
		}
		return e.partyArea(x.area)
	}
	return strings.ToUpper(strings.TrimSpace(q.call)) + "|" + strings.ToUpper(q.band) + "|" + location(sent, q.stationCallsign) + "|" + location(received, q.call)
}

func (e eventDefinition) partyMultiplierKeys(q qso) []string {
	sent, err := e.partyExchange(q.stx, q.stxString)
	if err != nil {
		return nil
	}
	received, err := e.partyExchange(q.srx, q.srxString)
	if err != nil {
		return nil
	}
	r := e.QSOParty
	var keys []string
	if len(sent.counties) == 0 {
		for _, county := range received.counties {
			keys = append(keys, "county:"+county)
		}
	} else {
		for _, kind := range r.HomeMultipliers {
			switch kind {
			case "county":
				for _, county := range received.counties {
					keys = append(keys, "county:"+county)
				}
			case "state":
				area := received.area
				if len(received.counties) > 0 && r.CountyAsState {
					area = r.State
				}
				area = e.partyArea(area)
				if partyPostalArea(area) {
					keys = append(keys, "state:"+area)
				}
			case "dxcc":
				if strings.HasPrefix(received.area, "DX:") {
					keys = append(keys, received.area)
				} else if received.area == "DX" {
					if country, ok := partyDXEntity(q.call); ok {
						keys = append(keys, "DX:"+country)
					}
				}
			case "dx":
				if received.area == "DX" {
					keys = append(keys, "DX")
				}
			case "maritime":
				if containsString([]string{"R1", "R2", "R3"}, received.area) {
					keys = append(keys, "maritime:"+received.area)
				}
			}
		}
	}
	if r.MultiplierPer == "band" {
		for i := range keys {
			keys[i] += "|" + strings.ToUpper(q.band)
		}
	}
	return keys
}

func (c *contestState) partyScore() contestScore {
	r := c.event.QSOParty
	var out contestScore
	seen, mults, bonus := map[string]bool{}, map[string]bool{}, map[string]bool{}
	countyContacts := map[string]int{}
	allSpecial, hasCredit := true, false
	for _, q := range c.partyQSOs {
		if q.unscored || !c.event.partyInPeriod(q.time) {
			continue
		}
		credits, err := c.event.partyCredits(q)
		if err != nil {
			continue
		}
		for _, credit := range credits {
			key := c.event.partyCreditKey(credit)
			if seen[key] {
				continue
			}
			seen[key] = true
			hasCredit = true
			if !containsString(r.QSOOnlyCalls, strings.ToUpper(credit.stationCallsign)) {
				allSpecial = false
			}
			out.qsoPoints += r.Points
			for _, k := range c.event.partyMultiplierKeys(credit) {
				mults[k] = true
			}
			sent, _ := c.event.partyExchange(credit.stx, credit.stxString)
			for _, county := range sent.counties {
				countyContacts[county]++
			}
			if strings.EqualFold(q.call, r.BonusCall) {
				bonus[strings.ToUpper(q.band)] = true
			}
		}
	}
	out.bonusPoints = len(bonus) * r.BonusPoints
	if c.partyCategory == "MOBILE" || c.partyCategory == "ROVER" {
		for county, n := range countyContacts {
			if r.MobileCountyMinimum == 0 || n < r.MobileCountyMinimum {
				continue
			}
			out.bonusPoints += r.MobileCountyBonus
			if r.MobileCountyMultiplier {
				found := false
				for k := range mults {
					if k == "county:"+county || strings.HasPrefix(k, "county:"+county+"|") {
						found = true
						break
					}
				}
				if !found {
					mults["operated:"+county] = true
				}
			}
		}
	}
	out.multipliers = len(mults)
	if r.MultiplierCap > 0 && out.multipliers > r.MultiplierCap {
		out.multipliers = r.MultiplierCap
	}
	out.powerFactor = r.PowerFactors[cabrilloOrDefault(c.partyPower, "HIGH")]
	if hasCredit && allSpecial {
		out.qsoPoints /= r.Points
		out.multipliers = 1
		out.powerFactor = 1
		out.bonusPoints = 0
	}
	return out
}

func (c *contestState) partyNewMultiplier(q qso) (fresh, worked bool) {
	seen := map[string]bool{}
	for _, prior := range c.partyQSOs {
		if prior.unscored || !c.event.partyInPeriod(prior.time) {
			continue
		}
		credits, err := c.event.partyCredits(prior)
		if err != nil {
			continue
		}
		for _, credit := range credits {
			for _, k := range c.event.partyMultiplierKeys(credit) {
				seen[k] = true
			}
		}
	}
	credits, err := c.event.partyCredits(q)
	if err != nil {
		return false, false
	}
	for _, credit := range credits {
		for _, k := range c.event.partyMultiplierKeys(credit) {
			if seen[k] {
				worked = true
			} else {
				fresh = true
			}
		}
	}
	if cap := c.event.QSOParty.MultiplierCap; cap > 0 && len(seen) >= cap {
		fresh = false
	}
	return
}

func (s *store) isPartyDupe(candidate qso, event eventDefinition, excludeID int64, since time.Time) (bool, error) {
	credits, err := event.partyCredits(candidate)
	if err != nil || len(credits) == 0 {
		return false, nil
	} // incomplete entry is not a proved duplicate
	seen := map[string]bool{}
	err = s.forEachQSOForContest(context.Background(), candidate.profileID, candidate.contestID, func(prior qso) error {
		if !event.partyInPeriod(prior.time) {
			return nil
		}
		if prior.id == excludeID && excludeID != 0 || !since.IsZero() && prior.time.Before(since) {
			return nil
		}
		if !strings.EqualFold(prior.call, candidate.call) || !strings.EqualFold(prior.band, candidate.band) {
			return nil
		}
		old, parseErr := event.partyCredits(prior)
		if parseErr != nil {
			return nil
		}
		for _, credit := range old {
			seen[event.partyCreditKey(credit)] = true
		}
		return nil
	})
	if err != nil {
		return false, err
	}
	for _, credit := range credits {
		if !seen[event.partyCreditKey(credit)] {
			return false, nil
		}
	}
	return true, nil
}

func (m model) partyEntryQSO(call string) qso {
	return qso{call: call, band: m.qsoBand(), mode: "CW", profileID: m.activeStation.ID,
		stationCallsign: m.activeStation.Callsign,
		contestID:       strings.TrimSpace(m.contestFields[contestName].Value()),
		stx:             m.contestFields[contestSerialSent].Value(), stxString: m.contestFields[contestExchangeSent].Value(),
		srx: m.contestFields[contestSerialRcvd].Value(), srxString: m.contestFields[contestExchangeRcvd].Value()}
}

func (m model) entryDupe(call, contestID, eventID, scope string, now time.Time) (bool, error) {
	if event, ok := m.eventForContestID(); ok && event.QSOParty != nil {
		q := m.partyEntryQSO(call)
		q.contestID = contestID
		return m.store.isPartyDupe(q, event, m.editingQSOID, m.dupeBaselineAfter)
	}
	return m.store.isDupe(call, m.qsoBand(), contestID, eventID, scope, m.activeStation.ID, m.editingQSOID, now, m.dupeBaselineAfter)
}

func partySubmissionHeaders(ctx context.Context, st *store, profile stationProfile, event eventDefinition, contestID string) ([]string, error) {
	location := ""
	home, away := false, false
	err := st.forEachQSOForContest(ctx, profile.ID, contestID, func(q qso) error {
		if err := validateContestSubmission(q, event, profile); err != nil {
			return fmt.Errorf("%s: %w", q.call, err)
		}
		sent, _ := event.partyExchange(q.stx, q.stxString)
		if len(sent.counties) > 0 {
			home = true
			location = event.QSOParty.State
		} else {
			away = true
			if location != "" && location != sent.area {
				return fmt.Errorf("outside operating locations differ; export separate contest logs for each location")
			}
			location = sent.area
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	if home && away {
		return nil, fmt.Errorf("in-state and out-of-state contacts require separate submissions")
	}
	if strings.HasPrefix(location, "DX:") {
		location = strings.TrimPrefix(location, "DX:")
	}
	category := cabrilloOrDefault(profile.CategoryStation, "FIXED")
	if event.QSOParty.OutsideCategory != "" && !home {
		category = event.QSOParty.OutsideCategory
	}
	lines := []string{"LOCATION: " + cabrilloHeaderValue(location), "CATEGORY-STATION: " + category}
	if event.QSOParty.CategorySoapbox && category != "FIXED" {
		lines = append(lines, "SOAPBOX: "+category)
	}
	return lines, nil
}
