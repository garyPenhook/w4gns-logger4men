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

// TestWrapCommaListFitsWithinWidth guards against the DXCC needed list
// rendering as one line wider than the terminal (a real report: entries like
// "223 ENGLAND" joined 20-deep with no width awareness ran off the right
// edge of the screen).
func TestWrapCommaListFitsWithinWidth(t *testing.T) {
	items := []string{"223 ENGLAND", "339 JAPAN", "76 GUATEMALA", "245 IRELAND", "15 ASIATIC RUSSIA"}
	lines := wrapCommaList(items, 20)
	if len(lines) < 2 {
		t.Fatalf("wrapCommaList produced %d line(s) for width 20, want more than 1: %v", len(lines), lines)
	}
	for _, line := range lines {
		// A single item longer than width still gets its own line (matching
		// packFieldRows), so only a multi-item line overflowing is a bug.
		if len([]rune(line)) > 20 && strings.Count(line, ",") > 0 {
			t.Fatalf("line %q exceeds width 20 despite joining multiple items", line)
		}
	}
	// Every item must still appear, in order, once reassembled.
	joined := strings.Join(lines, ", ")
	for _, item := range items {
		if !strings.Contains(joined, item) {
			t.Fatalf("wrapped output missing item %q: %q", item, joined)
		}
	}
}

func TestWrapCommaListSingleOverwidthItemGetsOwnLine(t *testing.T) {
	lines := wrapCommaList([]string{"291 UNITED STATES OF AMERICA"}, 10)
	if len(lines) != 1 || lines[0] != "291 UNITED STATES OF AMERICA" {
		t.Fatalf("wrapCommaList = %v, want the single item on its own line even though it exceeds width", lines)
	}
}

func TestWrapCommaListEmpty(t *testing.T) {
	if lines := wrapCommaList(nil, 20); lines != nil {
		t.Fatalf("wrapCommaList(nil) = %v, want nil", lines)
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
