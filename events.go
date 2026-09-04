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
	// Scoring, when present, lets the exporter compute a claimed score for the
	// contest instead of the informational 0 it emits for events with no rules
	// configured. Cabrillo export is per-session, so the computed score is one
	// session's score; the sponsor's robot recomputes the authoritative total.
	Scoring *scoringRules `json:"scoring"`
}

// scoringRules is a deliberately small, data-driven model of a contest's score
// formula: score = (sum of per-QSO points) × multipliers. It covers the
// "N points per non-duplicate QSO, one multiplier per unique callsign" shape
// used by CWops events; contests needing a different formula get their own
// multiplier kind rather than this being hardcoded per contest.
type scoringRules struct {
	// PointsPerQSO is awarded once per unique (callsign, band) worked in the
	// session; a same-band duplicate scores zero, matching CW Open's "once per
	// band, per session" rule.
	PointsPerQSO int `json:"points_per_qso"`
	// Multiplier selects how multipliers are counted. "unique_call" counts each
	// distinct callsign worked in the session once, regardless of band.
	Multiplier string `json:"multiplier"`
}

// validScoringMultiplier reports whether kind is a multiplier rule the scorer
// (see computeContestScore) understands, so a config typo fails loudly at
// startup instead of silently scoring zero multipliers.
func validScoringMultiplier(kind string) bool {
	switch strings.TrimSpace(kind) {
	case "unique_call":
		return true
	default:
		return false
	}
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

// receivedExchangeZoneKind reports which zone, if any, the operator is
// expected to log for the worked station's received exchange, inferred from
// the catalog's free-text RcvdExchangeHint (roadmap Appendix B.8 "auto data
// insert"). It drives autofillReceivedExchange (main.go), which prefills the
// zone from the resolved DXCC entity rather than making the operator type a
// value that's already knowable from the callsign. "itu_zone" is checked
// before "cq_zone" since a handful of hints mention both (e.g. IARU-style
// contests naming ITU zone alongside a CQ-zone-based multiplier note) and ITU
// is the one actually exchanged in those. Blank means no zone is inferable
// from the hint text — most contests exchange something the DXCC table can't
// derive (name, state, serial, RS(T) only).
func (e eventDefinition) receivedExchangeZoneKind() string {
	hint := strings.ToLower(e.RcvdExchangeHint)
	switch {
	case strings.Contains(hint, "itu zone"):
		return "itu_zone"
	case strings.Contains(hint, "cq zone"):
		return "cq_zone"
	default:
		return ""
	}
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
			if event.Scoring != nil {
				if event.Scoring.PointsPerQSO < 0 {
					return nil, fmt.Errorf("event %q has negative points_per_qso %d", event.ID, event.Scoring.PointsPerQSO)
				}
				if !validScoringMultiplier(event.Scoring.Multiplier) {
					return nil, fmt.Errorf("event %q has unsupported scoring multiplier %q", event.ID, event.Scoring.Multiplier)
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
