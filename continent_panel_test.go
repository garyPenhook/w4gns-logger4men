package main

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// TestContinentPanelOpenBuildsIndexAndShowsWorkedContinent exercises the
// Continents Worked screen (roadmap §3 Phase 2, Appendix B.9)
// end to end: opening it with an active contest lands on QSO Entry's
// current band, and a call logged in that contest/band shows its continent
// as worked while an unworked continent shows needed.
func TestContinentPanelOpenBuildsIndexAndShowsWorkedContinent(t *testing.T) {
	m := analysisTestModel(t)
	m.fields[fieldBand].SetValue("20M")
	m.contestIndex = newContestState()
	m.contestIndexID = m.contestFields[contestName].Value()
	m.contestIndex.record(qso{call: "DL1ABC", band: "20M"}) // Germany — EU

	m.openContinentPanel()
	if m.screen != continentScreen {
		t.Fatalf("screen = %v, want continentScreen", m.screen)
	}
	bands := m.continentPanelBands()
	if bands[m.continentBandFocus] != "20M" {
		t.Fatalf("continentBandFocus band = %q, want 20M (QSO Entry's current band)", bands[m.continentBandFocus])
	}

	view := m.continentPanelView()
	if !strings.Contains(view, "EU") {
		t.Fatalf("view missing EU line:\n%s", view)
	}
	if !strings.Contains(view, "worked (1)") {
		t.Fatalf("view should show EU worked once on 20M:\n%s", view)
	}
	if !strings.Contains(view, "NA") || !strings.Contains(view, "needed") {
		t.Fatalf("view should show an un-worked continent as needed:\n%s", view)
	}
}

// TestContinentPanelPagesBandsAndEscReturns exercises Left/Right band
// paging and Esc returning to QSO Entry with Call refocused.
func TestContinentPanelPagesBandsAndEscReturns(t *testing.T) {
	m := analysisTestModel(t)
	m.openContinentPanel()
	bands := m.continentPanelBands()
	start := m.continentBandFocus

	updated, _ := m.updateContinentPanel(tea.KeyMsg{Type: tea.KeyRight})
	m = updated.(model)
	if want := (start + 1) % len(bands); m.continentBandFocus != want {
		t.Fatalf("continentBandFocus after Right = %d, want %d", m.continentBandFocus, want)
	}

	updated, _ = m.updateContinentPanel(tea.KeyMsg{Type: tea.KeyEsc})
	m = updated.(model)
	if m.screen != qsoEntryScreen {
		t.Fatalf("screen after Esc = %v, want qsoEntryScreen", m.screen)
	}
	if m.focusIdx != fieldCall {
		t.Fatalf("focusIdx after Esc = %v, want fieldCall", m.focusIdx)
	}
}

// TestContinentPanelNoActiveContestShowsNotice covers opening the screen
// with no contest selected — same "set one on Events (F7)" fallback the
// Analysis panel uses.
func TestContinentPanelNoActiveContestShowsNotice(t *testing.T) {
	st, err := openStore(t.TempDir() + "/logger.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	m := initialModel(st)
	m.openContinentPanel()
	view := m.continentPanelView()
	if !strings.Contains(view, "no active contest") {
		t.Fatalf("expected no-active-contest notice, got:\n%s", view)
	}
}
