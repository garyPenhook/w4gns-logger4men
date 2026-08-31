package main

import (
	"path/filepath"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestLoadEventCatalogIncludesCWopsDefinitions(t *testing.T) {
	events, err := loadEventCatalog()
	if err != nil {
		t.Fatalf("loadEventCatalog: %v", err)
	}
	if len(events) != 3 {
		t.Fatalf("event count = %d, want 3", len(events))
	}
	if events[0].ID != "CW-OPEN" || events[1].ID != "CWT" || events[2].ID != "TNQP" {
		t.Fatalf("events = %#v", events)
	}
	if got := len(events[2].ReceivedExchangeOptions); got != 95 {
		t.Fatalf("TN county count = %d, want 95", got)
	}
}

func TestTNQPCountyTypeAheadInsertsOfficialCode(t *testing.T) {
	st, err := openStore(filepath.Join(t.TempDir(), "logger.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	m := initialModel(st)
	m.selectEvent(m.events[2], m.events[2].Sessions[0])
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
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
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
	m.eventFocus = 1 // CWT follows CW Open alphabetically.
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
