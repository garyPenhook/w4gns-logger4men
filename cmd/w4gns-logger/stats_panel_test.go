package main

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestCtrlAOpensStatsPanelWithLocalStatsOnly(t *testing.T) {
	m := reviewModel(t)
	q := reviewQSO(m)
	q.dxccNumber = "291"
	q.country = "UNITED STATES"
	reviewInsert(t, m, q)

	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlA})
	m = updated.(model)
	if cmd != nil {
		t.Fatal("opening the stats panel must not issue a command — it's computed from local data only")
	}
	if m.screen != statsScreen {
		t.Fatalf("screen = %v, want statsScreen", m.screen)
	}
	if m.lotwStats == nil {
		t.Fatal("lotwStats not populated on open")
	}
	if m.lotwStats.DXCC.Worked != 1 {
		t.Fatalf("DXCC.Worked = %d, want 1", m.lotwStats.DXCC.Worked)
	}

	view := m.statsPanelView()
	if !strings.Contains(view, "DXCC") || !strings.Contains(view, "WAS") {
		t.Fatalf("stats panel view missing award labels: %q", view)
	}
}

func TestStatsPanelEscReturnsToQSOEntry(t *testing.T) {
	m := reviewModel(t)
	m.openStatsPanel()

	updated, _ := m.updateStatsPanel(tea.KeyMsg{Type: tea.KeyEsc})
	m = updated.(model)
	if m.screen != qsoEntryScreen {
		t.Fatalf("screen = %v, want qsoEntryScreen after Esc", m.screen)
	}
}

func TestStatsPanelSyncRequiresLoTWLoginConfigured(t *testing.T) {
	m := reviewModel(t)
	m.openStatsPanel()
	m.lotwLogin, m.lotwWebPass = "", ""

	updated, cmd := m.updateStatsPanel(tea.KeyMsg{Runes: []rune("s"), Type: tea.KeyRunes})
	m = updated.(model)
	if cmd != nil {
		t.Fatal("sync without configured credentials must not issue a command")
	}
	if !strings.Contains(m.statsSyncMsg, "not configured") {
		t.Fatalf("statsSyncMsg = %q, want a not-configured notice", m.statsSyncMsg)
	}
}
