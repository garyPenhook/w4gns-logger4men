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
	// CabrilloContest overrides the Cabrillo CONTEST: token when the event's own
	// ID isn't the sponsor's identifier — e.g. a contest with distinct "home"
	// and "DX" side entries (different exchanges) that both submit under one
	// Cabrillo contest name. Blank means "use ID", preserving existing events.
	CabrilloContest  string   `json:"cabrillo_contest"`
	Organizer        string   `json:"organizer"`
	Kind             string   `json:"kind"`
	Schedule         string   `json:"schedule"`
	Bands            []string `json:"bands"`
	SentSerial       bool     `json:"sent_serial"`
	SentExchangeHint string   `json:"sent_exchange_hint"`
	RcvdExchangeHint string   `json:"received_exchange_hint"`
	DupeScope        string   `json:"dupe_scope"`
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
	// CabrilloLayout declares that this event's QSO-line shape has been checked
	// against the sponsor format. Blank means the catalog entry is useful for
	// selection/entry only and must not be exported as a purported submission.
	CabrilloLayout string `json:"cabrillo_layout"`
	// Scoring, when present, lets the exporter compute a claimed score for the
	// contest instead of the informational 0 it emits for events with no rules
	// configured. Cabrillo export is per-session, so the computed score is one
	// session's score; the sponsor's robot recomputes the authoritative total.
	Scoring *scoringRules `json:"scoring"`
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
// loudly at startup instead of silently scoring zero multipliers.
func validMultiplierKind(kind string) bool {
	switch strings.TrimSpace(kind) {
	case "unique_call", "dxcc", "cqzone", "ituzone":
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
// understands.
func validMultiplierPer(per string) bool {
	switch strings.TrimSpace(per) {
	case "band", "contest":
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
	case "", "cw_rst_exchange", "cw_exchange_only":
		return true
	default:
		return false
	}
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
// (see store.isDupe): blank means the casual 15-minute window, and the two
// named scopes select session- or contest-wide checking. Any other value
// would silently fall through to contest-wide scope, hiding a config typo.
func validDupeScope(scope string) bool {
	switch strings.TrimSpace(scope) {
	case "", "call+band", "call+band+session":
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
			if !validReceivedExchangeAutofill(event.ReceivedExchangeAutofill) {
				return nil, fmt.Errorf("event %q has unsupported received_exchange_autofill %q", event.ID, event.ReceivedExchangeAutofill)
			}
			if !validCabrilloLayout(event.CabrilloLayout) {
				return nil, fmt.Errorf("event %q has unsupported cabrillo_layout %q", event.ID, event.CabrilloLayout)
			}
			if event.CabrilloLayout != "" && event.CabrilloOmitRST != (event.CabrilloLayout == "cw_exchange_only") {
				return nil, fmt.Errorf("event %q has inconsistent cabrillo_omit_rst and cabrillo_layout", event.ID)
			}
			if event.Scoring != nil {
				if event.Scoring.PointsPerQSO < 0 {
					return nil, fmt.Errorf("event %q has negative points_per_qso %d", event.ID, event.Scoring.PointsPerQSO)
				}
				if len(event.Scoring.Multipliers) > 0 {
					for _, rule := range event.Scoring.Multipliers {
						if !validMultiplierKind(rule.Kind) {
							return nil, fmt.Errorf("event %q has unsupported multiplier kind %q", event.ID, rule.Kind)
						}
						if !validMultiplierPer(rule.Per) {
							return nil, fmt.Errorf("event %q multiplier %q has unsupported per scope %q", event.ID, rule.Kind, rule.Per)
						}
					}
				} else if !validScoringMultiplier(event.Scoring.Multiplier) {
					return nil, fmt.Errorf("event %q has unsupported scoring multiplier %q", event.ID, event.Scoring.Multiplier)
				}
				if p := event.Scoring.Points; p != nil {
					if p.SameCountry < 0 || p.SameContinent < 0 || p.OtherContinent < 0 {
						return nil, fmt.Errorf("event %q has a negative points value", event.ID)
					}
					for continent, value := range p.SameContinentOverrides {
						if !validContinentCode(continent) {
							return nil, fmt.Errorf("event %q has a same_continent_overrides entry for unsupported continent %q", event.ID, continent)
						}
						if value < 0 {
							return nil, fmt.Errorf("event %q has a negative same_continent_overrides value for %q", event.ID, continent)
						}
					}
				}
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
