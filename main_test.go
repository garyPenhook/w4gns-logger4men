package main

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestEnterLeavingCallStartsQSOTimer(t *testing.T) {
	st, err := openStore(filepath.Join(t.TempDir(), "logger.db"))
	if err != nil {
		t.Fatalf("openStore returned error: %v", err)
	}
	defer st.Close()

	m := initialModel(st)
	m.fields[fieldCall].SetValue("W1AW")
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	got := updated.(model)
	if got.qsoStartedAt.IsZero() {
		t.Fatal("Enter leaving Call did not start QSO timer")
	}
}

// TestF8IgnoresRepeatedPressesWhileBackupInProgress guards against launching
// concurrent backups (second-resolution filename collisions, uncoordinated
// with the mandatory exit backup): a second F8 while one is already running
// must not start another.
func TestF8IgnoresRepeatedPressesWhileBackupInProgress(t *testing.T) {
	st, err := openStore(filepath.Join(t.TempDir(), "logger.db"))
	if err != nil {
		t.Fatalf("openStore returned error: %v", err)
	}
	defer st.Close()

	m := initialModel(st)
	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyF8})
	m = updated.(model)
	if !m.backupInProgress {
		t.Fatal("first F8 press did not mark a backup in progress")
	}
	if cmd == nil {
		t.Fatal("first F8 press did not return a backup command")
	}

	updated, cmd = m.Update(tea.KeyMsg{Type: tea.KeyF8})
	m = updated.(model)
	if cmd != nil {
		t.Fatal("second F8 press while a backup is in progress returned a command")
	}
	if m.statusMsg != "backup already in progress…" {
		t.Fatalf("statusMsg = %q, want the already-in-progress message", m.statusMsg)
	}

	updated, _ = m.Update(backupCompletedMsg{})
	m = updated.(model)
	if m.backupInProgress {
		t.Fatal("backupInProgress stayed true after backupCompletedMsg")
	}
}

func TestEventDetailLineSurfacesPreviouslyUnusedFields(t *testing.T) {
	event := eventDefinition{
		Organizer:          "CWops",
		Bands:              []string{"20M", "15M"},
		RulesURL:           "https://example.com/rules",
		ScoreSubmissionURL: "https://example.com/scores",
	}
	line := eventDetailLine(event)
	for _, want := range []string{"CWops", "20M/15M", "https://example.com/rules", "https://example.com/scores"} {
		if !strings.Contains(line, want) {
			t.Errorf("eventDetailLine() = %q, want it to contain %q", line, want)
		}
	}
}

func TestLogCurrentQSOWarnsWhenBandOutsideEventAllowedBands(t *testing.T) {
	st, err := openStore(filepath.Join(t.TempDir(), "logger.db"))
	if err != nil {
		t.Fatalf("openStore returned error: %v", err)
	}
	defer st.Close()

	m := initialModel(st)
	tnqp := m.events[eventIndex(t, m.events, "TNQP")]
	m.selectEvent(tnqp, tnqp.Sessions[0])
	m.fields[fieldCall].SetValue("W1AW")
	m.fields[fieldBand].SetValue("60M")
	m.fields[fieldFrequency].SetValue("5.354")
	m, _ = m.logCurrentQSO()
	if !strings.Contains(m.statusMsg, "not in") {
		t.Fatalf("statusMsg = %q, want an allowed-bands warning", m.statusMsg)
	}
}

func TestReturningToCallResetsQSOTimer(t *testing.T) {
	st, err := openStore(filepath.Join(t.TempDir(), "logger.db"))
	if err != nil {
		t.Fatalf("openStore returned error: %v", err)
	}
	defer st.Close()

	m := initialModel(st)
	m.fields[fieldCall].SetValue("W1AW")
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyTab})
	m = updated.(model)
	if m.qsoStartedAt.IsZero() {
		t.Fatal("Tab leaving Call did not start QSO timer")
	}
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyShiftTab})
	if !updated.(model).qsoStartedAt.IsZero() {
		t.Fatal("Shift+Tab returning to Call did not reset QSO timer")
	}
}

func TestQSOEntryTabOrder(t *testing.T) {
	st, err := openStore(filepath.Join(t.TempDir(), "logger.db"))
	if err != nil {
		t.Fatalf("openStore returned error: %v", err)
	}
	defer st.Close()

	m := initialModel(st)
	want := []int{fieldRSTSent, fieldRSTRcvd, fieldBand, fieldFrequency, fieldMode}
	for _, field := range want {
		updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyTab})
		m = updated.(model)
		if m.focusIdx != field {
			t.Fatalf("focus after Tab = %d, want %d", m.focusIdx, field)
		}
	}
}

func TestBandSelectorSetsValidDefaultFrequency(t *testing.T) {
	st, err := openStore(filepath.Join(t.TempDir(), "logger.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	m := initialModel(st)
	m.focusField(fieldBand)
	m.selectBand(1)
	if m.qsoBand() != "17M" || m.qsoFrequency() != "18.080" {
		t.Fatalf("selected band/frequency = %s/%s, want 17M/18.080", m.qsoBand(), m.qsoFrequency())
	}
}

func TestADIFExportPath(t *testing.T) {
	path, ok := adifExportPath([]string{"--in-current-terminal", "--export-adif", "log.adi"})
	if !ok || path != "log.adi" {
		t.Fatalf("adifExportPath = %q, %t", path, ok)
	}
	if _, ok := adifExportPath([]string{"--export-adif"}); ok {
		t.Fatal("adifExportPath accepted a missing path")
	}
}

func TestPathsReferToSameFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "logger.db")
	if !pathsReferToSameFile(path, path) {
		t.Fatal("identical paths were not recognized")
	}
}

func TestScreenHotkeysSwitchBetweenQSOEntryAndStationSetup(t *testing.T) {
	st, err := openStore(filepath.Join(t.TempDir(), "logger.db"))
	if err != nil {
		t.Fatalf("openStore returned error: %v", err)
	}
	defer st.Close()

	m := initialModel(st)
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyF2})
	m = updated.(model)
	if m.screen != stationSetupScreen {
		t.Fatal("F2 did not open Station Setup")
	}
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyF1})
	if updated.(model).screen != qsoEntryScreen {
		t.Fatal("F1 did not return to QSO Entry")
	}
}

func TestF3OpensK3LRClusterScreen(t *testing.T) {
	st, err := openStore(filepath.Join(t.TempDir(), "logger.db"))
	if err != nil {
		t.Fatalf("openStore returned error: %v", err)
	}
	defer st.Close()

	m := initialModel(st)
	m.activeStation.Callsign = "W4GNS"
	updated, command := m.Update(tea.KeyMsg{Type: tea.KeyF3})
	got := updated.(model)
	if got.screen != clusterScreen {
		t.Fatal("F3 did not open Cluster screen")
	}
	if !got.clusterConnecting || command == nil {
		t.Fatal("F3 did not begin the K3LR connection")
	}
}

func TestClusterConnectionStateRejectsDuplicateAndStaleResults(t *testing.T) {
	st, err := openStore(filepath.Join(t.TempDir(), "logger.db"))
	if err != nil {
		t.Fatalf("openStore returned error: %v", err)
	}
	defer st.Close()

	m := initialModel(st)
	m.activeStation.Callsign = "W4GNS"
	m.openCluster()
	updated, _ := m.updateCluster(tea.KeyMsg{Type: tea.KeyF5})
	m = updated.(model)
	if !m.clusterConnecting || m.clusterGeneration != 1 {
		t.Fatalf("first connect state = connecting:%t generation:%d", m.clusterConnecting, m.clusterGeneration)
	}
	updated, _ = m.updateCluster(tea.KeyMsg{Type: tea.KeyF5})
	m = updated.(model)
	if m.clusterGeneration != 1 {
		t.Fatalf("duplicate F5 started another connection generation: %d", m.clusterGeneration)
	}
	m.clusterConnecting = false
	m.clusterStatus = "new connection stays active"
	m.clusterGeneration = 2
	updated, _ = m.updateCluster(clusterConnectedMsg{generation: 1, err: errors.New("old connection failed")})
	m = updated.(model)
	if m.clusterStatus != "new connection stays active" || m.clusterGeneration != 2 {
		t.Fatalf("stale connection result changed state: %#v", m)
	}
}
