package main

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// TestHelpPanelOpenReturnsToPriorScreen exercises Ctrl+G opening the Help
// screen from an arbitrary screen and Esc returning to that same screen
// (roadmap §3 Phase 3: "in-app HELP for the new commands").
func TestHelpPanelOpenReturnsToPriorScreen(t *testing.T) {
	m := analysisTestModel(t)
	m.screen = clusterScreen

	m.openHelpPanel()
	if m.screen != helpScreen {
		t.Fatalf("screen = %v, want helpScreen", m.screen)
	}
	if m.helpReturnScreen != clusterScreen {
		t.Fatalf("helpReturnScreen = %v, want clusterScreen", m.helpReturnScreen)
	}

	updated, _ := m.updateHelpPanel(tea.KeyMsg{Type: tea.KeyEsc})
	m = updated.(model)
	if m.screen != clusterScreen {
		t.Fatalf("screen after Esc = %v, want clusterScreen", m.screen)
	}
}

// TestHelpPanelOpenFromQSOEntryRefocusesCall covers the common case: Ctrl+G
// from QSO Entry, Esc back to QSO Entry with Call refocused, same as the
// other single-purpose panels.
func TestHelpPanelOpenFromQSOEntryRefocusesCall(t *testing.T) {
	m := analysisTestModel(t)
	m.screen = qsoEntryScreen

	m.openHelpPanel()
	updated, _ := m.updateHelpPanel(tea.KeyMsg{Type: tea.KeyEsc})
	m = updated.(model)
	if m.screen != qsoEntryScreen {
		t.Fatalf("screen after Esc = %v, want qsoEntryScreen", m.screen)
	}
	if m.focusIdx != fieldCall {
		t.Fatalf("focusIdx after Esc = %v, want fieldCall", m.focusIdx)
	}
}

// TestHelpPanelViewListsKeyCommands is a smoke check that the static
// reference actually mentions the headline commands, so a future rename
// silently drifting out of the help text is caught.
func TestHelpPanelViewListsKeyCommands(t *testing.T) {
	m := analysisTestModel(t)
	m.screen = qsoEntryScreen
	m.openHelpPanel()

	view := m.helpPanelView()
	for _, want := range []string{
		"Ctrl+W", "Worked/Needed by Continent",
		"Ctrl+G",
		"Ctrl+X", "Cabrillo",
		"Ctrl+R", "CSV",
		"Check Partial",
		"Rate meter",
		"Zone auto-fill",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("help view missing %q:\n%s", want, view)
		}
	}
}
