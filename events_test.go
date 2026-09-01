package main

import (
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// eventIndex finds an event by ID rather than assuming a fixed catalog
// position, since the alphabetical sort order shifts as events/*.json grows.
func eventIndex(t *testing.T, events []eventDefinition, id string) int {
	t.Helper()
	for i, event := range events {
		if event.ID == id {
			return i
		}
	}
	t.Fatalf("event %q not found in catalog", id)
	return -1
}

func TestLoadEventCatalogIncludesCWopsDefinitions(t *testing.T) {
	events, err := loadEventCatalog()
	if err != nil {
		t.Fatalf("loadEventCatalog: %v", err)
	}
	if len(events) < 41 {
		t.Fatalf("event count = %d, want at least 41", len(events))
	}
	seen := map[string]bool{}
	for _, event := range events {
		if seen[event.ID] {
			t.Fatalf("duplicate event id %q", event.ID)
		}
		seen[event.ID] = true
	}
	for _, id := range []string{"CW-OPEN", "CWT", "TNQP"} {
		if !seen[id] {
			t.Fatalf("expected event id %q in catalog", id)
		}
	}
	tnqp := events[eventIndex(t, events, "TNQP")]
	if got := len(tnqp.ReceivedExchangeOptions); got != 95 {
		t.Fatalf("TN county count = %d, want 95", got)
	}
}

// TestEventSelectionIDsFitContestField guards against silently truncating
// "event.ID-session.ID" (see selectEvent) in the Contest Name field: every
// catalog entry's longest generated value must fit maxEventSelectionLength.
func TestEventSelectionIDsFitContestField(t *testing.T) {
	events, err := loadEventCatalog()
	if err != nil {
		t.Fatalf("loadEventCatalog: %v", err)
	}
	for _, event := range events {
		for _, session := range event.Sessions {
			value := event.ID + "-" + session.ID
			if len(value) > maxEventSelectionLength {
				t.Errorf("event %q session %q generates %q (%d chars), exceeds maxEventSelectionLength (%d)",
					event.ID, session.ID, value, len(value), maxEventSelectionLength)
			}
		}
	}
}

// TestLoadEventCatalogHasNoLeftoverScraperArtifacts guards against the
// " and / " glue text a prior scrape left in several multi-session
// schedules (e.g. "0600Z-0629Z, Sep 5 and / 0630Z-0659Z, Sep 5").
func TestLoadEventCatalogHasNoLeftoverScraperArtifacts(t *testing.T) {
	events, err := loadEventCatalog()
	if err != nil {
		t.Fatalf("loadEventCatalog: %v", err)
	}
	for _, event := range events {
		if strings.Contains(event.Schedule, "and / ") {
			t.Errorf("event %q schedule has a leftover scraper artifact: %q", event.ID, event.Schedule)
		}
		for _, session := range event.Sessions {
			if strings.Contains(session.Schedule, "and / ") {
				t.Errorf("event %q session %q schedule has a leftover scraper artifact: %q", event.ID, session.ID, session.Schedule)
			}
		}
	}
}

func TestTNQPCountyTypeAheadInsertsOfficialCode(t *testing.T) {
	st, err := openStore(filepath.Join(t.TempDir(), "logger.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	m := initialModel(st)
	tnqp := m.events[eventIndex(t, m.events, "TNQP")]
	m.selectEvent(tnqp, tnqp.Sessions[0])
	m.contestFocusIdx = contestExchangeRcvd
	focusTextFields(m.contestFields, m.contestFocusIdx)
	m.contestFields[contestExchangeRcvd].SetValue("shel")
	updated, _ := m.updateQSOContest(tea.KeyMsg{Type: tea.KeyDown})
	m = updated.(model)
	updated, _ = m.updateQSOContest(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(model)
	if got := m.contestFields[contestExchangeRcvd].Value(); got != "SHEL" {
		t.Fatalf("county choice = %q, want SHEL", got)
	}
}

func TestEventCatalogSelectsCWOpenDefaults(t *testing.T) {
	st, err := openStore(filepath.Join(t.TempDir(), "logger.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	m := initialModel(st)
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyF7})
	m = updated.(model)
	if m.screen != eventCatalogScreen {
		t.Fatalf("F7 screen = %v, want event catalog", m.screen)
	}
	m.eventFocus = eventIndex(t, m.events, "CW-OPEN")
	updated, _ = m.updateEventCatalog(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(model)
	if m.screen != qsoContestScreen || m.contestFields[contestName].Value() != "CW-OPEN-1" || m.contestFields[contestSerialSent].Value() != "001" {
		t.Fatalf("selected event state = %#v", m)
	}
}

func TestEventCatalogCyclesCWTSessions(t *testing.T) {
	st, err := openStore(filepath.Join(t.TempDir(), "logger.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	m := initialModel(st)
	m.openEventCatalog()
	m.eventFocus = eventIndex(t, m.events, "CWT")
	updated, _ := m.updateEventCatalog(tea.KeyMsg{Type: tea.KeyRight})
	m = updated.(model)
	if m.eventSessionFocus != 1 {
		t.Fatalf("CWT session focus = %d, want 1", m.eventSessionFocus)
	}
	updated, _ = m.updateEventCatalog(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(model)
	if m.contestFields[contestName].Value() != "CWT-1900" {
		t.Fatalf("contest ID = %q", m.contestFields[contestName].Value())
	}
}
