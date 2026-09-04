package main

import (
	"context"
	"sort"
	"strconv"
	"strings"
	"time"
)

// contestState is the in-memory index described in the roadmap (Appendix C):
// a single pass over one contest's QSOs that every scoring/analysis
// consumer reads, so they can never disagree with each other. It is built
// fresh from the store (buildContestState) rather than mutated incrementally
// today; the roadmap's "incremental on log, full recompute on edit" wiring
// belongs to the live TUI model once the as-you-type panels land, but a full
// rebuild is always correct and, at contest-log sizes (a few thousand QSOs),
// cheap enough to call on every score/export.
type contestState struct {
	// byCall lists every QSO worked with a given callsign (upper-cased), in
	// the order logged. Feeds Check Partial and the band/mode matrix.
	byCall map[string][]qso
	// workedCallBand is the set of "CALL|BAND" keys already logged, i.e. the
	// dupe/worked-this-band test.
	workedCallBand map[string]struct{}
	// uniqueCalls is the set of distinct callsigns worked in the contest,
	// regardless of band — the "unique_call" multiplier rule.
	uniqueCalls map[string]struct{}
	// scoredCallBand/scoredUniqueCalls mirror workedCallBand/uniqueCalls but
	// omit any QSO flagged /X (unscored). Dupe/Check-Partial/rate/continent
	// all still see an /X QSO as fully worked (it did happen; the flag only
	// says it shouldn't count) — only score() reads these two sets.
	scoredCallBand    map[string]struct{}
	scoredUniqueCalls map[string]struct{}
	// times holds every logged QSO's start time, in the same chronological
	// order they were recorded — the rate meter's only input (Appendix B/D
	// "Rate meter (Q/hr L10/L100/overall, Q/Mult)").
	times []time.Time
	// continentBand counts QSOs worked per continent per band — the roadmap's
	// "Worked/Needed by continent" panel (Appendix B.9). Keyed by the
	// two-letter continent code cty.dat/dxcc.go uses (NA, SA, EU, AF, AS, OC).
	continentBand map[string]map[string]int
	// dxccByBand/cqZoneByBand/ituZoneByBand hold the distinct DXCC entity
	// numbers / CQ zones / ITU zones worked on each band — the data-driven
	// multiplier schema's "per band" counters (Appendix C, multiplierRule
	// Per: "band"), e.g. CQ WW's countries-worked and zones-worked mults.
	// dxccAll/cqZoneAll/ituZoneAll mirror them but merge across every band,
	// for a Per: "contest" (counted once) multiplier. All four are built from
	// scored QSOs only (mirrors scoredCallBand/scoredUniqueCalls): an /X QSO
	// still counts as worked for dupe/Check-Partial/rate/continent, but not
	// for a multiplier.
	dxccByBand    map[string]map[int]struct{}
	dxccAll       map[int]struct{}
	cqZoneByBand  map[string]map[int]struct{}
	cqZoneAll     map[int]struct{}
	ituZoneByBand map[string]map[int]struct{}
	ituZoneAll    map[int]struct{}
	// stationDXCCNumber/stationContinent identify the operator's own station
	// (resolved from its callsign via cty.dat), the input a continent/country-
	// tiered points rule (pointsRule) needs to classify each worked QSO as
	// same-country/same-continent/other-continent. stationResolved is false
	// when setStation wasn't called or the callsign didn't resolve, in which
	// case pointCategory is never populated and a pointsRule scores 0 for every
	// QSO rather than guessing.
	stationDXCCNumber int
	stationContinent  string
	stationResolved   bool
	// pointCategory holds, per "CALL|BAND" key (mirrors scoredCallBand), which
	// bucket of a pointsRule the QSO falls into — populated by record() only
	// when both the worked entity and the station resolved, scored QSOs only
	// (same /X exclusion as scoredCallBand).
	pointCategory map[string]qsoPointCategory
	// pointCategoryContinent records the worked entity's continent for each
	// pointCategorySameContinent entry in pointCategory, so pointsTotal can
	// apply a pointsRule.SameContinentOverrides entry (e.g. CQ WW's North
	// America same-continent exception) instead of the flat SameContinent
	// value. Unset for any other category.
	pointCategoryContinent map[string]string
}

// qsoPointCategory classifies a worked station relative to the operator's own
// station for a continent/country-tiered points rule (pointsRule).
type qsoPointCategory int

// pointCategorySameCountry deliberately does not start at 0: a missing
// pointCategory map entry (unresolved station or worked entity) must read as
// "no category" rather than silently matching the zero value.
const (
	pointCategoryUnknown qsoPointCategory = iota
	pointCategorySameCountry
	pointCategorySameContinent
	pointCategoryOtherContinent
)

// newContestState returns an empty index, ready for QSOs to be recorded.
func newContestState() *contestState {
	return &contestState{
		byCall:                 make(map[string][]qso),
		workedCallBand:         make(map[string]struct{}),
		uniqueCalls:            make(map[string]struct{}),
		scoredCallBand:         make(map[string]struct{}),
		scoredUniqueCalls:      make(map[string]struct{}),
		continentBand:          make(map[string]map[string]int),
		dxccByBand:             make(map[string]map[int]struct{}),
		dxccAll:                make(map[int]struct{}),
		cqZoneByBand:           make(map[string]map[int]struct{}),
		cqZoneAll:              make(map[int]struct{}),
		ituZoneByBand:          make(map[string]map[int]struct{}),
		ituZoneAll:             make(map[int]struct{}),
		pointCategory:          make(map[string]qsoPointCategory),
		pointCategoryContinent: make(map[string]string),
	}
}

// setStation resolves callsign's DXCC entity and records its country/
// continent as the operator's own station, the input record() needs to
// classify worked QSOs for a continent/country-tiered pointsRule. A blank or
// unresolvable callsign leaves stationResolved false, so score() falls back
// to awarding 0 points under a pointsRule rather than guessing.
func (c *contestState) setStation(callsign string) {
	table, err := sharedDXCCTable()
	if err != nil {
		return
	}
	entity, found := table.lookup(strings.ToUpper(strings.TrimSpace(callsign)))
	if !found || entity.DXCCNumber == 0 {
		return
	}
	c.stationDXCCNumber = entity.DXCCNumber
	c.stationContinent = entity.Continent
	c.stationResolved = true
}

// record adds one QSO to the index. Safe to call repeatedly in chronological
// order while building, or once per newly logged QSO for an incremental
// update.
func (c *contestState) record(q qso) {
	call := strings.ToUpper(strings.TrimSpace(q.call))
	if call == "" {
		return
	}
	band := strings.ToUpper(strings.TrimSpace(q.band))
	c.byCall[call] = append(c.byCall[call], q)
	key := call + "|" + band
	c.workedCallBand[key] = struct{}{}
	c.uniqueCalls[call] = struct{}{}
	if !q.unscored {
		c.scoredCallBand[key] = struct{}{}
		c.scoredUniqueCalls[call] = struct{}{}
	}
	if !q.time.IsZero() {
		c.times = append(c.times, q.time)
	}
	if table, err := sharedDXCCTable(); err == nil {
		if entity, found := table.lookup(call); found {
			entity = entityWithPersistedContext(entity, q)
			if entity.Continent != "" {
				if c.continentBand[entity.Continent] == nil {
					c.continentBand[entity.Continent] = make(map[string]int)
				}
				c.continentBand[entity.Continent][band]++
			}
			if !q.unscored {
				recordMultiplierValue(c.dxccByBand, c.dxccAll, band, entity.DXCCNumber)
				recordMultiplierValue(c.cqZoneByBand, c.cqZoneAll, band, entity.CQZone)
				recordMultiplierValue(c.ituZoneByBand, c.ituZoneAll, band, entity.ITUZone)
				if c.stationResolved {
					switch {
					case entity.DXCCNumber != 0 && entity.DXCCNumber == c.stationDXCCNumber:
						c.pointCategory[key] = pointCategorySameCountry
					case entity.Continent != "" && entity.Continent == c.stationContinent:
						c.pointCategory[key] = pointCategorySameContinent
						c.pointCategoryContinent[key] = entity.Continent
					default:
						c.pointCategory[key] = pointCategoryOtherContinent
					}
				}
			}
		}
	}
}

// entityWithPersistedContext preserves imported/operator-corrected DXCC and
// zone values when scoring a stored QSO. The cty.dat lookup still supplies
// callsign-derived continent/coordinates, but its mutable reference data must
// not silently replace a QSO's recorded scoring context.
func entityWithPersistedContext(entity dxccEntity, q qso) dxccEntity {
	if country := strings.TrimSpace(q.country); country != "" {
		entity.Country = country
	}
	if value, err := strconv.Atoi(strings.TrimSpace(q.dxccNumber)); err == nil && value > 0 {
		entity.DXCCNumber = value
	}
	if value, err := strconv.Atoi(strings.TrimSpace(q.cqZone)); err == nil && value > 0 {
		entity.CQZone = value
	}
	if value, err := strconv.Atoi(strings.TrimSpace(q.ituZone)); err == nil && value > 0 {
		entity.ITUZone = value
	}
	return entity
}

// recordMultiplierValue adds value to byBand[band] and all, unless value is
// zero — cty.dat leaves DXCC number/CQ zone/ITU zone at zero when it couldn't
// resolve one, and a zero shouldn't be countable as a multiplier.
func recordMultiplierValue(byBand map[string]map[int]struct{}, all map[int]struct{}, band string, value int) {
	if value == 0 {
		return
	}
	if byBand[band] == nil {
		byBand[band] = make(map[int]struct{})
	}
	byBand[band][value] = struct{}{}
	all[value] = struct{}{}
}

// continents lists the standard continent codes cty.dat/dxcc.go use, in the
// fixed display order the Worked/Needed by Continent panel renders them —
// stable regardless of which continents happen to be worked yet.
var continents = []string{"NA", "SA", "EU", "AF", "AS", "OC"}

// validContinentCode reports whether continent is one of the six standard
// codes cty.dat/dxcc.go use — the same set continents lists — so a
// pointsRule.SameContinentOverrides typo fails loudly at load time.
func validContinentCode(continent string) bool {
	for _, c := range continents {
		if c == continent {
			return true
		}
	}
	return false
}

// continentSummary reports, for one continent on one band, whether it has
// been worked (and how many times) — the "needed" complement is simply
// !worked. Bands with a zero count are still "needed": the operator hasn't
// logged that continent there yet.
func (c *contestState) continentSummary(continent, band string) (worked bool, count int) {
	band = strings.ToUpper(strings.TrimSpace(band))
	byBand := c.continentBand[continent]
	if byBand == nil {
		return false, 0
	}
	count = byBand[band]
	return count > 0, count
}

// isWorkedOnBand reports whether call has already been logged on band —
// the same test as the dupe check, exposed here so future panels and
// scoring agree with the store-backed store.isDupe check used live.
func (c *contestState) isWorkedOnBand(call, band string) bool {
	_, ok := c.workedCallBand[strings.ToUpper(strings.TrimSpace(call))+"|"+strings.ToUpper(strings.TrimSpace(band))]
	return ok
}

// checkPartial returns the previously logged calls that contain fragment as
// a substring — the roadmap's Check Partial list (Appendix B.3): as the
// operator types a fragment of a call they half-caught, this surfaces
// candidates from their own log to complete it from. The exact fragment
// itself is excluded (that call already has its own "Worked:" line), matches
// are sorted for a stable display order, and the result is capped at limit
// so the panel can't grow unbounded on a fragment matching most of the log.
func (c *contestState) checkPartial(fragment string, limit int) []string {
	fragment = strings.ToUpper(strings.TrimSpace(fragment))
	if fragment == "" {
		return nil
	}
	var matches []string
	for call := range c.byCall {
		if call == fragment {
			continue
		}
		if strings.Contains(call, fragment) {
			matches = append(matches, call)
		}
	}
	sort.Strings(matches)
	if len(matches) > limit {
		matches = matches[:limit]
	}
	return matches
}

// score tallies a contestScore from the index per rules: PointsPerQSO once
// per unique (call, band) already recorded, multiplied by the sum of every
// rule in rules.effectiveMultipliers(). Mirrors the dedup behavior the
// previous per-call computeContestScore implementation had (a same-band
// duplicate still counts as a multiplier, since the callsign was still
// worked).
func (c *contestState) score(rules *scoringRules) contestScore {
	if rules == nil {
		return contestScore{}
	}
	var out contestScore
	if rules.Points != nil {
		out.qsoPoints = c.pointsTotal(rules.Points)
	} else {
		out.qsoPoints = len(c.scoredCallBand) * rules.PointsPerQSO
	}
	for _, rule := range rules.effectiveMultipliers() {
		out.multipliers += c.multiplierCount(rule)
	}
	return out
}

// pointsTotal sums a continent/country-tiered pointsRule over every scored
// (call, band) QSO. A QSO with no recorded category (station or worked
// entity didn't resolve — see setStation/record) contributes 0 rather than
// guessing a tier.
func (c *contestState) pointsTotal(rule *pointsRule) int {
	total := 0
	for key := range c.scoredCallBand {
		switch c.pointCategory[key] {
		case pointCategorySameCountry:
			total += rule.SameCountry
		case pointCategorySameContinent:
			if override, ok := rule.SameContinentOverrides[c.pointCategoryContinent[key]]; ok {
				total += override
			} else {
				total += rule.SameContinent
			}
		case pointCategoryOtherContinent:
			total += rule.OtherContinent
		}
	}
	return total
}

// multiplierCount returns how many multipliers rule contributes: the size of
// the relevant "all" set for Per: "contest", or the sum of the relevant
// per-band set sizes for Per: "band" (the same DXCC entity/zone counts again
// on each band it's worked, matching CQ WW-style scoring).
func (c *contestState) multiplierCount(rule multiplierRule) int {
	var byBand map[string]map[int]struct{}
	var all map[int]struct{}
	switch strings.TrimSpace(rule.Kind) {
	case "unique_call":
		return len(c.scoredUniqueCalls)
	case "dxcc":
		byBand, all = c.dxccByBand, c.dxccAll
	case "cqzone":
		byBand, all = c.cqZoneByBand, c.cqZoneAll
	case "ituzone":
		byBand, all = c.ituZoneByBand, c.ituZoneAll
	default:
		return 0
	}
	if strings.TrimSpace(rule.Per) == "band" {
		total := 0
		for _, set := range byBand {
			total += len(set)
		}
		return total
	}
	return len(all)
}

// wouldBeNewMultiplier reports whether logging call on band right now would
// add a new multiplier under rules (Appendix B.5 "advance multiplier flag"),
// and separately whether any rule it checked was already satisfied — the
// analysis panel uses newMult for the "NEW MULT" flag and workedBefore for
// the dimmer "not a new mult" line, and a rule set combining several kinds
// (e.g. CQ WW's DXCC-per-band + zone-per-band) can produce either, both, or
// neither depending on what's already logged. entityFound false (unresolved
// prefix) skips every dxcc/cqzone/ituzone rule — nothing to check yet.
func (c *contestState) wouldBeNewMultiplier(rules *scoringRules, call, band string, entity dxccEntity, entityFound bool) (newMult, workedBefore bool) {
	call = strings.ToUpper(strings.TrimSpace(call))
	band = strings.ToUpper(strings.TrimSpace(band))
	for _, rule := range rules.effectiveMultipliers() {
		switch strings.TrimSpace(rule.Kind) {
		case "unique_call":
			if _, worked := c.scoredUniqueCalls[call]; worked {
				workedBefore = true
			} else {
				newMult = true
			}
		case "dxcc", "cqzone", "ituzone":
			if !entityFound {
				continue
			}
			var value int
			var byBand map[string]map[int]struct{}
			var all map[int]struct{}
			switch strings.TrimSpace(rule.Kind) {
			case "dxcc":
				value, byBand, all = entity.DXCCNumber, c.dxccByBand, c.dxccAll
			case "cqzone":
				value, byBand, all = entity.CQZone, c.cqZoneByBand, c.cqZoneAll
			case "ituzone":
				value, byBand, all = entity.ITUZone, c.ituZoneByBand, c.ituZoneAll
			}
			if value == 0 {
				continue
			}
			var already bool
			if strings.TrimSpace(rule.Per) == "band" {
				_, already = byBand[band][value]
			} else {
				_, already = all[value]
			}
			if already {
				workedBefore = true
			} else {
				newMult = true
			}
		}
	}
	return newMult, workedBefore
}

// rebuildContestIndex rebuilds m.contestIndex from the store for whichever
// contest m.dupeCheckScope currently resolves to, and records that contest ID
// in m.contestIndexID. It is the single sync point that keeps the live index
// agreeing with the database: callers that change which contest is active
// (checkDupe, when contestIndexID no longer matches) or that mutate a QSO
// that could belong to the active contest (edit save, delete) call this
// directly. A blank contestID (no known contest) clears the index; a store
// error is treated as best-effort "no index," matching how sharedDXCCTable
// enrichment failures are already treated elsewhere.
func (m *model) rebuildContestIndex() {
	contestID, _, _ := m.dupeCheckScope()
	m.contestIndexID = contestID
	if contestID == "" {
		m.contestIndex = nil
		return
	}
	state, err := buildContestState(context.Background(), m.activeStation.ID, m.activeStation.Callsign, contestID, m.store)
	if err != nil {
		m.contestIndex = nil
		return
	}
	m.contestIndex = state
}

// buildContestState scans every QSO logged under contestID for profileID and
// returns the resulting index. Called on contest open and whenever a full
// recompute is needed (e.g. after an edit changes a call or band).
// stationCallsign resolves the operator's own DXCC country/continent (via
// setStation) so a pointsRule can classify each worked QSO; a blank
// callsign simply leaves a pointsRule scoring 0 for every QSO.
func buildContestState(ctx context.Context, profileID int64, stationCallsign, contestID string, st *store) (*contestState, error) {
	state := newContestState()
	state.setStation(stationCallsign)
	err := st.forEachQSOForContest(ctx, profileID, contestID, func(q qso) error {
		state.record(q)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return state, nil
}
