package main

import (
	"embed"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

//go:embed events/*.json
var eventConfigFiles embed.FS

// eventDefinition is deliberately data-only so new events and contests can be
// added without changing the application. IDs are persisted in ADIF CONTEST_ID.
type eventDefinition struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	// Capability tells the operator how far this catalog record has been
	// verified. It is deliberately explicit rather than inferred in the UI:
	// an event can be useful for entry while still being unsafe to submit as
	// Cabrillo or unable to produce a trustworthy claimed score.
	Capability string `json:"capability"`
	// CabrilloContest overrides the Cabrillo CONTEST: token when the event's own
	// ID isn't the sponsor's identifier — e.g. a contest with distinct "home"
	// and "DX" side entries (different exchanges) that both submit under one
	// Cabrillo contest name. Blank means "use ID", preserving existing events.
	CabrilloContest string `json:"cabrillo_contest"`
	// ADIFContestID is the interoperable ADIF CONTEST_ID value. It is separate
	// from ID because ID is intentionally application/session-oriented (for
	// example, CWT-1900), while ADIF has one standard identifier for the whole
	// event (CWOPS-CWT). Blank leaves the stored internal ID untouched in ADIF
	// output, which is appropriate until an event has been checked against the
	// ADIF Contest ID Enumeration.
	ADIFContestID    string   `json:"adif_contest_id"`
	Organizer        string   `json:"organizer"`
	Kind             string   `json:"kind"`
	Schedule         string   `json:"schedule"`
	Bands            []string `json:"bands"`
	SentSerial       bool     `json:"sent_serial"`
	SentExchangeHint string   `json:"sent_exchange_hint"`
	RcvdExchangeHint string   `json:"received_exchange_hint"`
	DupeScope        string   `json:"dupe_scope"`
	// CountyOptions is the sponsor's canonical county list for shared QSO
	// party scoring. Kept separate from exchange suggestions, which can also
	// contain states, provinces, or other non-county exchange values.
	CountyOptions []exchangeOption `json:"county_options,omitempty"`
	QSOParty      *qsoPartyRules   `json:"qso_party,omitempty"`
	// CabrilloOmitRST drops the RST columns from the Cabrillo QSO: line for
	// contests whose exchange carries no signal report — e.g. CW Open, whose
	// exchange is a serial number plus the operator's name. The default (false)
	// keeps the RST-bearing generic layout every other contest here uses.
	CabrilloOmitRST         bool             `json:"cabrillo_omit_rst"`
	RulesURL                string           `json:"rules_url"`
	ScoreSubmissionURL      string           `json:"score_submission_url"`
	Sessions                []eventSession   `json:"sessions"`
	ReceivedExchangeOptions []exchangeOption `json:"received_exchange_options"`
	// ReceivedExchangeAutofill explicitly opts an event into callsign-derived
	// receive-exchange filling. It deliberately does not infer a rule from the
	// display hint: asymmetric contests routinely mention both a domestic
	// state/province exchange and a DX zone exchange in that text. An empty
	// value is the safe default.
	ReceivedExchangeAutofill string `json:"received_exchange_autofill"`
	// ReceivedExchangeAutofillDomestic lists cty.dat country names (exact
	// match, e.g. "United States", "Canada") for which ReceivedExchangeAutofill
	// must NOT fire. It exists for contests whose exchange is genuinely
	// side-dependent by the worked station's own entity rather than uniform —
	// CQ 160 CW has DX stations send a CQ zone but W/VE stations send a
	// state/province/DC that cty.dat has no way to derive. Autofilling a
	// guessed zone for a worked W/VE station would silently contradict the
	// actual (unresolvable-from-callsign) required exchange, so those entities
	// are excluded rather than guessed. Empty means the kind applies uniformly
	// (e.g. CQ WW CW, worked on both sides as a CQ zone).
	ReceivedExchangeAutofillDomestic []string `json:"received_exchange_autofill_domestic,omitempty"`
	// CabrilloLayout declares that this event's QSO-line shape has been checked
	// against the sponsor format. Blank means the catalog entry is useful for
	// selection/entry only and must not be exported as a purported submission.
	CabrilloLayout string `json:"cabrillo_layout"`
	// Scoring, when present, lets the exporter compute a claimed score for the
	// contest instead of the informational 0 it emits for events with no rules
	// configured. Cabrillo export is per-session, so the computed score is one
	// session's score; the sponsor's robot recomputes the authoritative total.
	// For an event whose rules are side-asymmetric (DXScoring is set), Scoring
	// is specifically the rule set for an operator on the "domestic" side
	// (DomesticCountries) — e.g. ARRL DX CW's W/VE-side DXCC-entity
	// multiplier, matching this app's own station profile.
	Scoring *scoringRules `json:"scoring"`
	// DXScoring, when set, is the scoring rule set for an operator whose own
	// station does NOT resolve to one of DomesticCountries — the other half
	// of a side-asymmetric contest (e.g. ARRL DX CW Rule 5.2.2's DX-side
	// US-state/DC/Canadian-province multiplier, which replaces rather than
	// adds to the W/VE-side DXCC-entity multiplier in Scoring). Blank/nil
	// (the default) means the contest's scoring doesn't depend on which side
	// the operator is on, and every entrant uses Scoring — the shape every
	// event configured before this field existed already has.
	DXScoring *scoringRules `json:"dx_scoring,omitempty"`
	// DomesticCountries lists cty.dat country names (exact match, e.g.
	// "United States", "Canada") that make the operator's own station
	// "domestic" for effectiveScoring's side selection. Required whenever
	// DXScoring is set — there is no meaningful side split otherwise. Reuses
	// the same shape as ReceivedExchangeAutofillDomestic (a different
	// side-detection need: which entities to exclude from zone autofill).
	DomesticCountries []string `json:"domestic_countries,omitempty"`
}

// effectiveScoring picks which scoring rule set applies to an operator whose
// own station resolves to stationCountry (a cty.dat dxccEntity.Country
// value, or "" if unresolved): Scoring unless DXScoring is configured and
// stationCountry does NOT match one of DomesticCountries, in which case the
// DX-side rules apply instead. An unresolved station's own country
// conservatively falls back to Scoring (this app's own station profile is
// domestic/W-VE for every side-asymmetric event configured so far) rather
// than guessing DX-side rules for a station that couldn't be resolved.
func (e eventDefinition) effectiveScoring(stationCountry string) *scoringRules {
	if e.DXScoring == nil {
		return e.Scoring
	}
	if strings.TrimSpace(stationCountry) == "" || countryInList(e.DomesticCountries, stationCountry) {
		return e.Scoring
	}
	return e.DXScoring
}

// validateScoringRules checks one scoringRules block (an event's Scoring or
// DXScoring) for internal consistency, the same checks the loader ran
// inline before DXScoring existed — extracted so both blocks get identical
// validation. label identifies which field a failure is in (e.g. "scoring"
// vs "dx_scoring"). A nil rules is valid (no scoring configured for that
// side) and returns nil immediately.
func validateScoringRules(eventID, label string, rules *scoringRules) error {
	if rules == nil {
		return nil
	}
	if rules.PointsPerQSO < 0 {
		return fmt.Errorf("event %q has negative %s.points_per_qso %d", eventID, label, rules.PointsPerQSO)
	}
	if len(rules.Multipliers) > 0 {
		for _, rule := range rules.Multipliers {
			if !validMultiplierKind(rule.Kind) {
				return fmt.Errorf("event %q has unsupported %s multiplier kind %q", eventID, label, rule.Kind)
			}
			if !validMultiplierPer(rule.Per) {
				return fmt.Errorf("event %q %s multiplier %q has unsupported per scope %q", eventID, label, rule.Kind, rule.Per)
			}
		}
	} else if !validScoringMultiplier(rules.Multiplier) {
		return fmt.Errorf("event %q has unsupported %s multiplier %q", eventID, label, rules.Multiplier)
	}
	p := rules.Points
	if p == nil {
		return nil
	}
	if p.SameCountry < 0 || p.SameContinent < 0 || p.OtherContinent < 0 ||
		p.LowBandSameContinent < 0 || p.LowBandOtherContinent < 0 ||
		p.GroupPoints < 0 || p.LowBandGroupPoints < 0 {
		return fmt.Errorf("event %q has a negative %s points value", eventID, label)
	}
	if z := p.Zone; z != nil {
		if z.SameZone < 0 || z.SameContinentDifferentZone < 0 || z.OtherContinent < 0 || z.Special < 0 {
			return fmt.Errorf("event %q has a negative %s.zone points value", eventID, label)
		}
		if p.SameCountry != 0 || p.SameContinent != 0 || p.OtherContinent != 0 || len(p.CountryGroup) > 0 {
			return fmt.Errorf("event %q must not combine %s.zone with country/continent-based points fields", eventID, label)
		}
	}
	if d := p.Distance; d != nil {
		if d.PerKm <= 0 {
			return fmt.Errorf("event %q has a non-positive %s.distance.per_km value %d", eventID, label, d.PerKm)
		}
		if p.SameCountry != 0 || p.SameContinent != 0 || p.OtherContinent != 0 || len(p.CountryGroup) > 0 || p.Zone != nil {
			return fmt.Errorf("event %q must not combine %s.distance with any other points field", eventID, label)
		}
	}
	if len(p.PerBand) > 0 {
		for band, value := range p.PerBand {
			if strings.TrimSpace(band) == "" {
				return fmt.Errorf("event %q has a blank %s.per_band band key", eventID, label)
			}
			if value < 0 {
				return fmt.Errorf("event %q has a negative %s.per_band value for band %q", eventID, label, band)
			}
		}
		if p.SameCountry != 0 || p.SameContinent != 0 || p.OtherContinent != 0 || len(p.CountryGroup) > 0 || p.Zone != nil || p.Distance != nil {
			return fmt.Errorf("event %q must not combine %s.per_band with any other points field", eventID, label)
		}
	}
	if iotaRule := p.IOTA; iotaRule != nil {
		if iotaRule.IslandWorksWorld < 0 || iotaRule.IslandWorksSameReference < 0 || iotaRule.IslandWorksOtherIsland < 0 ||
			iotaRule.WorldWorksWorld < 0 || iotaRule.WorldWorksIsland < 0 {
			return fmt.Errorf("event %q has a negative %s.iota points value", eventID, label)
		}
		if p.SameCountry != 0 || p.SameContinent != 0 || p.OtherContinent != 0 || len(p.CountryGroup) > 0 || p.Zone != nil || p.Distance != nil || len(p.PerBand) > 0 {
			return fmt.Errorf("event %q must not combine %s.iota with any other points field", eventID, label)
		}
	}
	for continent, value := range p.SameContinentOverrides {
		if !validContinentCode(continent) {
			return fmt.Errorf("event %q has a %s.same_continent_overrides entry for unsupported continent %q", eventID, label, continent)
		}
		if value < 0 {
			return fmt.Errorf("event %q has a negative %s.same_continent_overrides value for %q", eventID, label, continent)
		}
	}
	for continent, value := range p.LowBandSameContinentOverrides {
		if !validContinentCode(continent) {
			return fmt.Errorf("event %q has a %s.low_band_same_continent_overrides entry for unsupported continent %q", eventID, label, continent)
		}
		if value < 0 {
			return fmt.Errorf("event %q has a negative %s.low_band_same_continent_overrides value for %q", eventID, label, continent)
		}
	}
	return nil
}

const (
	// catalogCapabilitySelectionOnly is a discoverable catalog placeholder;
	// it may be selected but makes no promise about exchange-aware entry.
	catalogCapabilitySelectionOnly = "selection-only"
	// catalogCapabilityEntryAware has reviewed bands and exchange hints, but
	// no verified Cabrillo line format or scoring implementation.
	catalogCapabilityEntryAware = "entry-aware"
	// catalogCapabilityCabrilloReady additionally has a checked Cabrillo QSO
	// line layout. It may still lack contest-specific scoring rules.
	catalogCapabilityCabrilloReady = "cabrillo-ready"
	// catalogCapabilityScoringReady has both a checked Cabrillo layout and
	// implemented scoring rules.
	catalogCapabilityScoringReady = "scoring-ready"
)

func validEventCapability(capability string) bool {
	switch strings.TrimSpace(capability) {
	case catalogCapabilitySelectionOnly, catalogCapabilityEntryAware,
		catalogCapabilityCabrilloReady, catalogCapabilityScoringReady:
		return true
	default:
		return false
	}
}

// capabilityLabel is intentionally operator-facing (rather than exposing
// JSON tokens) for the Events screen.
func capabilityLabel(capability string) string {
	switch strings.TrimSpace(capability) {
	case catalogCapabilitySelectionOnly:
		return "selection only"
	case catalogCapabilityEntryAware:
		return "entry aware"
	case catalogCapabilityCabrilloReady:
		return "Cabrillo ready"
	case catalogCapabilityScoringReady:
		return "scoring ready"
	default:
		return "unverified"
	}
}

// validateCapability makes the advertised catalog state mechanically true.
// Keep the requirements intentionally conservative: a status may never imply
// that the exporter or scorer knows more about an event than its data proves.
func (e eventDefinition) validateCapability() error {
	capability := strings.TrimSpace(e.Capability)
	if !validEventCapability(capability) {
		return fmt.Errorf("event %q has unsupported capability %q", e.ID, e.Capability)
	}
	hasLayout := strings.TrimSpace(e.CabrilloLayout) != ""
	hasScoring := e.Scoring != nil
	switch capability {
	case catalogCapabilitySelectionOnly:
		if hasLayout || hasScoring {
			return fmt.Errorf("event %q capability %q must not declare Cabrillo layout or scoring", e.ID, capability)
		}
	case catalogCapabilityEntryAware:
		if len(e.Bands) == 0 || strings.TrimSpace(e.RcvdExchangeHint) == "" {
			return fmt.Errorf("event %q capability %q requires bands and a received exchange hint", e.ID, capability)
		}
		if hasLayout || hasScoring {
			return fmt.Errorf("event %q capability %q must not declare Cabrillo layout or scoring", e.ID, capability)
		}
	case catalogCapabilityCabrilloReady:
		if !hasLayout || hasScoring {
			return fmt.Errorf("event %q capability %q requires Cabrillo layout and no scoring", e.ID, capability)
		}
	case catalogCapabilityScoringReady:
		if !hasLayout || !hasScoring {
			return fmt.Errorf("event %q capability %q requires Cabrillo layout and scoring", e.ID, capability)
		}
	}
	return nil
}

// scoringRules is a deliberately small, data-driven model of a contest's score
// formula: score = (sum of per-QSO points) × multipliers. It covers the
// "N points per non-duplicate QSO, one multiplier per unique callsign" shape
// used by CWops events, plus (Appendix C) the DXCC-entity/CQ-zone/ITU-zone
// per-band multiplier shape used by contests like CQ WW — contests needing a
// different formula still get their own multiplier kind rather than this
// being hardcoded per contest.
type scoringRules struct {
	// PointsPerQSO is awarded once per unique (callsign, band) worked in the
	// session; a same-band duplicate scores zero, matching CW Open's "once per
	// band, per session" rule.
	PointsPerQSO int `json:"points_per_qso"`
	// Multiplier is the legacy single-kind field: "unique_call" counts each
	// distinct callsign worked in the session once, regardless of band. Kept
	// so existing event configs (CW Open, CWops) don't need editing; a config
	// listing Multipliers instead takes precedence (see effectiveMultipliers).
	Multiplier string `json:"multiplier"`
	// Multipliers is the data-driven multiplier list (Appendix C): each rule
	// contributes its own count and the counts sum, matching contests that
	// award more than one kind of multiplier (e.g. CQ WW's countries + zones).
	// When non-empty it replaces Multiplier entirely rather than adding to it,
	// so a config can't double-count "unique_call" by accident.
	Multipliers []multiplierRule `json:"multipliers,omitempty"`
	// Points is the continent/country-tiered points shape used by contests
	// like CQ WW (e.g. 0 points same country, 1 same continent/different
	// country, 3 different continent) instead of a flat PointsPerQSO. When
	// set it replaces PointsPerQSO entirely for that event (see score()).
	Points *pointsRule `json:"points,omitempty"`
}

// pointsRule is a continent/country-tiered points formula (Appendix C's
// "points": {"same_country":0,"same_continent":1,"other_continent":3}):
// how many points a QSO is worth, classified by the worked station's
// relationship to the operator's own station (contestState.setStation
// resolves the operator's side; record() classifies each worked QSO).
type pointsRule struct {
	SameCountry    int `json:"same_country"`
	SameContinent  int `json:"same_continent"`
	OtherContinent int `json:"other_continent"`
	// SameContinentOverrides replaces SameContinent for a specific worked
	// continent, keyed by the two-letter code contest_state.go's continents
	// list uses (NA, SA, EU, AF, AS, OC). CQ WW is the motivating case: same
	// continent is 1 point everywhere except North America, where a
	// different-country QSO is worth 2 ("Exception: Contacts between stations
	// in different countries within the North American boundaries count two
	// (2) points" — cqww.com/rules.htm).
	SameContinentOverrides map[string]int `json:"same_continent_overrides,omitempty"`
	// LowBandSameContinent/LowBandOtherContinent/LowBandSameContinentOverrides
	// apply instead of the corresponding non-low-band field when the QSO's
	// band is one of the WPX-defined "low bands" (160M/80M/40M) — CQ WPX
	// awards double points on those bands relative to 20M/15M/10M (e.g. 1/2
	// same-continent, 3/6 other-continent). Zero (unset) falls back to the
	// base field, so a contest without band-tiered points (CQ WW, CQ 160)
	// doesn't need to repeat every value.
	LowBandSameContinent          int            `json:"low_band_same_continent,omitempty"`
	LowBandOtherContinent         int            `json:"low_band_other_continent,omitempty"`
	LowBandSameContinentOverrides map[string]int `json:"low_band_same_continent_overrides,omitempty"`
	// CountryGroup, when non-empty, adds a group-membership tier that
	// pointsTotal checks before SameCountry/SameContinent/OtherContinent: a
	// worked station whose cty.dat country is in CountryGroup scores
	// GroupPoints (LowBandGroupPoints on a WPX-style low band) instead of the
	// country/continent-relative value, regardless of the operator's own
	// location. A worked station outside CountryGroup falls through to the
	// existing country/continent classification unchanged. This is for
	// contests scored around a fixed geographic group rather than the
	// operator's own position — SAC's "Scandinavian" side/DX split, where
	// e.g. two Scandinavian stations working each other isn't the
	// same-continent case the flat SameContinent value describes.
	CountryGroup       []string `json:"country_group,omitempty"`
	GroupPoints        int      `json:"group_points,omitempty"`
	LowBandGroupPoints int      `json:"low_band_group_points,omitempty"`
	// Zone, when set, replaces the country/continent tiers above entirely
	// with a zone-based formula (the IARU HF World Championship's shape,
	// Rule 5.1) — mutually exclusive with SameCountry/SameContinent/
	// OtherContinent/CountryGroup, enforced by validateScoringRules.
	Zone *zonePointsRule `json:"zone,omitempty"`
	// Distance, when set, replaces every tier above with a continuous
	// distance-based formula (the Stew Perry Topband Distance Challenge's
	// shape, kkn.net/stew rules: "Count a minimum of one point per QSO and
	// an additional point for every 500 kilometers distance") — mutually
	// exclusive with every other field, enforced by validateScoringRules.
	Distance *distancePointsRule `json:"distance,omitempty"`
	// PerBand, when set, replaces every tier above with a flat points value
	// looked up solely by the QSO's own band — no country/continent
	// classification at all. The Oceania DX Contest's shape
	// (oceaniadxcontest.com rules: 20/10/5/1/2/3 points on 160/80/40/20/15/
	// 10M) is the motivating case: unlike WPX's LowBand fields, which double
	// an existing country/continent tier, OCDX's points depend on the band
	// alone. Keyed by the same uppercase band string as
	// eventDefinition.Bands (e.g. "160M"); a band missing from the map
	// scores 0. Mutually exclusive with every other field, enforced by
	// validateScoringRules.
	PerBand map[string]int `json:"per_band,omitempty"`
	// IOTA, when set, replaces every tier above with the RSGB IOTA Contest's
	// shape (rsgbcc.org/hf/rules "Scoring"): points depend on whether the
	// operator's own station is declared an island station for the QSO
	// (qso.myIotaRef, snapshotted from the station profile's My IOTA Ref at
	// log time — see stationProfile.MyIOTARef) and whether the worked station
	// exchanged an IOTA reference (iotaMultiplierValue) — mutually exclusive
	// with every other field, enforced by validateScoringRules.
	IOTA *iotaPointsRule `json:"iota,omitempty"`
}

// iotaPointsRule is the RSGB IOTA Contest's points table. The contest
// distinguishes "Island stations" (operating from a qualifying island) from
// "World stations" (everyone else); which one the operator's own station is
// for a given QSO comes from qso.myIotaRef, not the worked callsign.
type iotaPointsRule struct {
	// IslandWorksWorld is scored when the operator is an island station and
	// works a world station (rules: "World Stations: 5 points").
	IslandWorksWorld int `json:"island_works_world"`
	// IslandWorksSameReference is scored when the operator is an island
	// station and works another station on the *same* IOTA reference (rules:
	// "Island Stations having the same IOTA reference: 5 points").
	IslandWorksSameReference int `json:"island_works_same_reference"`
	// IslandWorksOtherIsland is scored when the operator is an island
	// station and works a *different* island station (rules: "Other Island
	// Stations: 15 points").
	IslandWorksOtherIsland int `json:"island_works_other_island"`
	// WorldWorksWorld is scored when the operator is a world station and
	// works another world station (rules: "World Stations: 2 points").
	WorldWorksWorld int `json:"world_works_world"`
	// WorldWorksIsland is scored when the operator is a world station and
	// works an island station (rules: "Island Stations: 15 points").
	WorldWorksIsland int `json:"world_works_island"`
}

// distancePointsRule is the Stew Perry Topband Distance Challenge's points
// formula: 1 point minimum, plus 1 more point per PerKm kilometers of
// great-circle distance between the two stations' 4-character Maidenhead
// grid squares (the contest's entire exchange — RST is optional and not
// scored). Distance is computed from the worked station's received-exchange
// text (contestState.record, mirroring exchange_area.go/canton.go's
// exchange-is-authoritative precedent) and the operator's own grid square as
// snapshotted on the QSO at log time (qso.myGridSquare), not a live station
// profile lookup, so a later profile edit can't retroactively change a
// logged QSO's score. Deliberately not implemented (out of scope, a data
// gap rather than a formula gap): the rules also multiply a QSO's points 2x/
// 4x when the worked station declares itself Low Power/QRP, and multiply
// the operator's own final score 1.5x/3x for its own declared power class —
// none of that is exchanged over the air (the exchange is only the grid
// square) or derivable from anything this app's QSO/Cabrillo model
// captures, so a QSO with no resolvable grid on either side simply
// contributes 0 points rather than guessing.
type distancePointsRule struct {
	PerKm int `json:"per_km"`
}

// zonePointsRule is the IARU HF World Championship's points formula (Rule
// 5.1): a worked station's ITU zone, as actually exchanged (iaru_zone.go's
// iaruExchangeZone, not resolved from its callsign) determines whether it
// matches the operator's own zone, a different zone on the same continent,
// or a different continent — except a worked IARU Member Society HQ or
// Official station (iaruExchangeSpecial), which always scores Special
// regardless of zone/continent (Rule 5.1.2).
type zonePointsRule struct {
	SameZone                   int `json:"same_zone"`
	SameContinentDifferentZone int `json:"same_continent_different_zone"`
	OtherContinent             int `json:"other_continent"`
	Special                    int `json:"special"`
}

// multiplierRule is one entry in scoringRules.Multipliers: what to count
// (Kind) and over what scope (Per). "band" mirrors SD's MULTSCOUNT=Band (the
// same DXCC entity/zone counts again on every band it's worked on); "contest"
// mirrors MULTSCOUNT=Once (counted a single time no matter how many bands).
type multiplierRule struct {
	Kind string `json:"kind"`
	Per  string `json:"per"`
}

// validMultiplierKind reports whether kind is a multiplier the scorer (see
// contestState.multiplierCount) knows how to count, so a config typo fails
// loudly at startup instead of silently scoring zero multipliers. "none" is
// the explicit multiplier-free declaration (Stew Perry Topband's own rules:
// "There is no multiplier for different grids worked" — final score is
// simply the QSO points total): contestScore.total() multiplies qsoPoints by
// the summed multiplier count, so an event with genuinely no multiplier
// concept still needs a rule contributing a constant 1 rather than an empty
// Multipliers list, which validateScoringRules would otherwise reject as
// "no multiplier configured" the same way it does for every other event.
func validMultiplierKind(kind string) bool {
	switch strings.TrimSpace(kind) {
	case "unique_call", "dxcc", "cqzone", "ituzone", "prefix", "exchange_area", "county", "tn_county", "sac_area", "naqp_area", "sst_area", "arrl_section", "iaru_zone", "iaru_hq", "wae_country", "dxcc_non_wae", "canton", "oblast", "dxcc_or_wae", "dok_district", "iota", "none":
		return true
	default:
		return false
	}
}

// validScoringMultiplier is validMultiplierKind under its original name, kept
// for the legacy Multiplier field's validation call site.
func validScoringMultiplier(kind string) bool {
	return validMultiplierKind(kind)
}

// validMultiplierPer reports whether per is a scope multiplierCount
// understands. "band_weighted" is WAE's own per-band multiplier bonus
// (Section 6: countries worked on 80M count 4x, 40M 3x, 20/15/10M 2x) —
// distinct from "band" (an unweighted per-band count).
func validMultiplierPer(per string) bool {
	switch strings.TrimSpace(per) {
	case "band", "contest", "band_weighted":
		return true
	default:
		return false
	}
}

// effectiveMultipliers returns the multiplier rules score() should sum,
// translating the legacy scalar Multiplier field into the equivalent single
// rule when Multipliers wasn't configured. "unique_call" via the legacy field
// has always counted once across the whole contest regardless of band, so it
// maps to Per: "contest".
func (r *scoringRules) effectiveMultipliers() []multiplierRule {
	if r == nil {
		return nil
	}
	if len(r.Multipliers) > 0 {
		return r.Multipliers
	}
	if kind := strings.TrimSpace(r.Multiplier); kind != "" {
		return []multiplierRule{{Kind: kind, Per: "contest"}}
	}
	return nil
}

// cabrilloToken returns the effective Cabrillo CONTEST: token for the event:
// CabrilloContest when set, otherwise the event's own ID. Shared by the
// exporter (the actual header value) and the catalog loader's curated-vs-
// generated de-dup (two events sharing a token are the same real-world
// contest, however differently they're keyed in this catalog).
func (e eventDefinition) cabrilloToken() string {
	if token := strings.TrimSpace(e.CabrilloContest); token != "" {
		return token
	}
	return e.ID
}

// receivedExchangeZoneKind reports the explicitly configured callsign-derived
// zone type for the worked station's received exchange. Do not infer this from
// RcvdExchangeHint: hints are descriptive prose and cannot safely encode
// side-dependent exchanges such as CQ 160's W/VE state-versus-DX-zone rule.
func (e eventDefinition) receivedExchangeZoneKind() string {
	return strings.TrimSpace(e.ReceivedExchangeAutofill)
}

// receivedExchangeAutofillExcluded reports whether country (a cty.dat
// dxccEntity.Country value) is on this event's autofill-domestic exclusion
// list — i.e. the worked station's real exchange isn't the callsign-derived
// zone ReceivedExchangeAutofill names, so autofillReceivedExchange must leave
// the field blank rather than prefill a wrong guess.
func (e eventDefinition) receivedExchangeAutofillExcluded(country string) bool {
	return countryInList(e.ReceivedExchangeAutofillDomestic, country)
}

// countryInList reports whether country (a cty.dat dxccEntity.Country value)
// case-insensitively matches an entry in list, trimming both sides — the
// matching rule shared by receivedExchangeAutofillExcluded's zone-autofill
// exclusion and effectiveScoring's domestic/DX side selection. A blank
// country never matches, so an unresolved worked/own station is never
// mistaken for a specific listed entity.
func countryInList(list []string, country string) bool {
	country = strings.TrimSpace(country)
	if country == "" {
		return false
	}
	for _, candidate := range list {
		if strings.EqualFold(strings.TrimSpace(candidate), country) {
			return true
		}
	}
	return false
}

func validReceivedExchangeAutofill(kind string) bool {
	switch strings.TrimSpace(kind) {
	case "", "cq_zone", "itu_zone":
		return true
	default:
		return false
	}
}

func validCabrilloLayout(layout string) bool {
	switch strings.TrimSpace(layout) {
	case "", "cw_rst_exchange", "cw_exchange_only", "cw_sweepstakes":
		return true
	default:
		return false
	}
}

// validADIFContestID accepts the deliberately narrow token shape used by the
// ADIF Contest ID Enumeration. We do not attempt to duplicate that external
// enumeration here: a catalog entry may name a newly added official ID before
// this application updates. Rejecting internal whitespace, punctuation, and
// lowercase still catches configuration mistakes that would create an invalid
// ADIF field.
func validADIFContestID(id string) bool {
	id = strings.TrimSpace(id)
	if id == "" {
		return true
	}
	for _, r := range id {
		if !(r >= 'A' && r <= 'Z') && !(r >= '0' && r <= '9') && r != '-' {
			return false
		}
	}
	return true
}

// cabrilloReady is intentionally strict: a catalog record without a checked
// line layout is not safe to export as a contest submission.
func (e eventDefinition) cabrilloReady() bool {
	return strings.TrimSpace(e.CabrilloLayout) != ""
}

type exchangeOption struct {
	Code string `json:"code"`
	Name string `json:"name"`
}

type eventSession struct {
	ID       string `json:"id"`
	Label    string `json:"label"`
	Schedule string `json:"schedule"`
}

// validDupeScope reports whether scope is one the dupe checker understands
// (see store.isDupe): blank means the casual 15-minute window, "call+band"/
// "call+band+session" select contest-/session-wide checking scoped to a
// band, and "call" drops the band scope entirely — ARRL Sweepstakes Rule 2.2
// ("Each station may be contacted only once, regardless of band") is the
// motivating case: every other configured event allows re-working the same
// callsign once per band. Any other value would silently fall through to
// contest-wide scope, hiding a config typo.
func validDupeScope(scope string) bool {
	switch strings.TrimSpace(scope) {
	case "", "call", "call+band", "call+band+session", "call+band+location":
		return true
	default:
		return false
	}
}

// generatedEventCatalogFile is the SD-template-derived catalog (see
// docs/ROADMAP.md "Contest catalog from SD templates"): every other file
// under events/ is hand-curated with a real rules URL and hand-checked
// exchange format. The SD generator runs independently of this app and has
// no way to know what's already hand-curated here, so loadEventCatalog drops
// a generated entry that's a straight duplicate of a curated one — same
// cabrilloToken, exactly one entry on each side — rather than showing the
// operator the same real-world contest twice (roadmap: "curated vs generated
// duplicates", prefer curated). A token shared by *two or more* generated
// entries and one curated entry is left alone: that's the SD catalog
// splitting a contest's "home"/"DX" sides (distinct exchanges) that the
// curated entry only covers generically, which is additional fidelity worth
// keeping, not a duplicate.
const generatedEventCatalogFile = "sd_contests.json"

func loadEventCatalog() ([]eventDefinition, error) {
	entries, err := eventConfigFiles.ReadDir("events")
	if err != nil {
		return nil, fmt.Errorf("read event configs: %w", err)
	}
	// Token counts are collected in a pass over every file before any event
	// is considered for the catalog, so the de-dup below doesn't depend on
	// directory read order.
	curatedTokenCount := make(map[string]int)
	generatedTokenCount := make(map[string]int)
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		data, err := eventConfigFiles.ReadFile("events/" + entry.Name())
		if err != nil {
			return nil, fmt.Errorf("read event config %s: %w", entry.Name(), err)
		}
		var configured []eventDefinition
		if err := json.Unmarshal(data, &configured); err != nil {
			return nil, fmt.Errorf("parse event config %s: %w", entry.Name(), err)
		}
		counts := curatedTokenCount
		if entry.Name() == generatedEventCatalogFile {
			counts = generatedTokenCount
		}
		for _, event := range configured {
			counts[strings.ToUpper(event.cabrilloToken())]++
		}
	}
	var events []eventDefinition
	ids := make(map[string]struct{})
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		data, err := eventConfigFiles.ReadFile("events/" + entry.Name())
		if err != nil {
			return nil, fmt.Errorf("read event config %s: %w", entry.Name(), err)
		}
		var configured []eventDefinition
		if err := json.Unmarshal(data, &configured); err != nil {
			return nil, fmt.Errorf("parse event config %s: %w", entry.Name(), err)
		}
		for _, event := range configured {
			event.ID = strings.TrimSpace(event.ID)
			event.Name = strings.TrimSpace(event.Name)
			// ADIF contest IDs are standardized uppercase ASCII tokens. Normalize
			// surrounding whitespace here, while validation below keeps malformed
			// values from becoming silent, non-interoperable export data.
			event.ADIFContestID = strings.TrimSpace(event.ADIFContestID)
			if event.ID == "" || event.Name == "" {
				return nil, fmt.Errorf("event config %s has an event without id or name", entry.Name())
			}
			if entry.Name() == generatedEventCatalogFile {
				token := strings.ToUpper(event.cabrilloToken())
				if curatedTokenCount[token] == 1 && generatedTokenCount[token] == 1 {
					continue
				}
			}
			if len(event.Sessions) == 0 {
				return nil, fmt.Errorf("event config %s has no sessions for %q", entry.Name(), event.ID)
			}
			if _, exists := ids[event.ID]; exists {
				return nil, fmt.Errorf("duplicate event id %q", event.ID)
			}
			if !validDupeScope(event.DupeScope) {
				return nil, fmt.Errorf("event %q has unsupported dupe_scope %q", event.ID, event.DupeScope)
			}
			if err := event.prepareCountyOptions(); err != nil {
				return nil, err
			}
			if err := event.prepareQSOParty(); err != nil {
				return nil, err
			}
			if !validReceivedExchangeAutofill(event.ReceivedExchangeAutofill) {
				return nil, fmt.Errorf("event %q has unsupported received_exchange_autofill %q", event.ID, event.ReceivedExchangeAutofill)
			}
			if len(event.ReceivedExchangeAutofillDomestic) > 0 && strings.TrimSpace(event.ReceivedExchangeAutofill) == "" {
				return nil, fmt.Errorf("event %q has received_exchange_autofill_domestic without received_exchange_autofill", event.ID)
			}
			if !validCabrilloLayout(event.CabrilloLayout) {
				return nil, fmt.Errorf("event %q has unsupported cabrillo_layout %q", event.ID, event.CabrilloLayout)
			}
			if !validADIFContestID(event.ADIFContestID) {
				return nil, fmt.Errorf("event %q has invalid adif_contest_id %q", event.ID, event.ADIFContestID)
			}
			if err := event.validateCapability(); err != nil {
				return nil, err
			}
			if event.CabrilloLayout != "" && event.CabrilloOmitRST != (event.CabrilloLayout == "cw_exchange_only" || event.CabrilloLayout == "cw_sweepstakes") {
				return nil, fmt.Errorf("event %q has inconsistent cabrillo_omit_rst and cabrillo_layout", event.ID)
			}
			if err := validateScoringRules(event.ID, "scoring", event.Scoring); err != nil {
				return nil, err
			}
			if err := validateScoringRules(event.ID, "dx_scoring", event.DXScoring); err != nil {
				return nil, err
			}
			if event.DXScoring != nil && len(event.DomesticCountries) == 0 {
				return nil, fmt.Errorf("event %q has dx_scoring without domestic_countries", event.ID)
			}
			if len(event.DomesticCountries) > 0 && event.DXScoring == nil {
				return nil, fmt.Errorf("event %q has domestic_countries without dx_scoring", event.ID)
			}
			for _, band := range event.Bands {
				if bandIndex(band) < 0 {
					return nil, fmt.Errorf("event %q lists unsupported band %q", event.ID, band)
				}
			}
			sessionIDs := make(map[string]struct{}, len(event.Sessions))
			for _, session := range event.Sessions {
				sessionID := strings.TrimSpace(session.ID)
				if sessionID == "" {
					return nil, fmt.Errorf("event %q has a session without an id", event.ID)
				}
				if _, exists := sessionIDs[sessionID]; exists {
					return nil, fmt.Errorf("event %q has duplicate session id %q", event.ID, sessionID)
				}
				sessionIDs[sessionID] = struct{}{}
				// selectEvent writes "event.ID-session.ID" into the Contest
				// field, whose CharLimit is maxEventSelectionLength; a longer
				// generated value would be silently truncated and then fail to
				// resolve back to its event (eventForContestID).
				if selection := event.ID + "-" + sessionID; len(selection) > maxEventSelectionLength {
					return nil, fmt.Errorf("event %q session %q generates a %d-character contest id, exceeding the %d-character limit", event.ID, sessionID, len(selection), maxEventSelectionLength)
				}
			}
			ids[event.ID] = struct{}{}
			events = append(events, event)
		}
	}
	if len(events) == 0 {
		return nil, fmt.Errorf("event catalog is empty")
	}
	sort.Slice(events, func(i, j int) bool { return events[i].Name < events[j].Name })
	return events, nil
}
