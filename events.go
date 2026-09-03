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
	ID                      string           `json:"id"`
	Name                    string           `json:"name"`
	Organizer               string           `json:"organizer"`
	Kind                    string           `json:"kind"`
	Schedule                string           `json:"schedule"`
	Bands                   []string         `json:"bands"`
	SentSerial              bool             `json:"sent_serial"`
	SentExchangeHint        string           `json:"sent_exchange_hint"`
	RcvdExchangeHint        string           `json:"received_exchange_hint"`
	DupeScope               string           `json:"dupe_scope"`
	RulesURL                string           `json:"rules_url"`
	ScoreSubmissionURL      string           `json:"score_submission_url"`
	Sessions                []eventSession   `json:"sessions"`
	ReceivedExchangeOptions []exchangeOption `json:"received_exchange_options"`
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

func loadEventCatalog() ([]eventDefinition, error) {
	entries, err := eventConfigFiles.ReadDir("events")
	if err != nil {
		return nil, fmt.Errorf("read event configs: %w", err)
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
			if len(event.Sessions) == 0 {
				return nil, fmt.Errorf("event config %s has no sessions for %q", entry.Name(), event.ID)
			}
			if _, exists := ids[event.ID]; exists {
				return nil, fmt.Errorf("duplicate event id %q", event.ID)
			}
			if !validDupeScope(event.DupeScope) {
				return nil, fmt.Errorf("event %q has unsupported dupe_scope %q", event.ID, event.DupeScope)
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
