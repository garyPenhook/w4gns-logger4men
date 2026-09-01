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

// TestCheckDupeWarnsOnceForUnrecognizedContestName covers the case where an
// operator free-types a contest name that isn't in the event catalog:
// dupeScope silently falls back to the casual 15-minute window, so checkDupe
// must surface that instead of failing silently — but only once per distinct
// value, not on every keystroke.
func TestCheckDupeWarnsOnceForUnrecognizedContestName(t *testing.T) {
	st, err := openStore(filepath.Join(t.TempDir(), "logger.db"))
	if err != nil {
		t.Fatalf("openStore returned error: %v", err)
	}
	defer st.Close()

	m := initialModel(st)
	m.contestFields[contestName].SetValue("Some Local Sprint")
	m.fields[fieldCall].SetValue("W1AW")
	m.checkDupe()
	if !strings.Contains(m.statusMsg, "Some Local Sprint") {
		t.Fatalf("statusMsg = %q, want it to mention the unrecognized contest name", m.statusMsg)
	}

	m.statusMsg = ""
	m.checkDupe()
	if m.statusMsg != "" {
		t.Fatalf("checkDupe re-warned for the same contest name: statusMsg = %q", m.statusMsg)
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

// TestLogCurrentQSORejectsBandOutsideEventAllowedBands guards against
// committing a QSO on a band outside the selected event's allowed bands: the
// check must block the insert (there's no in-app way to fix a bad record
// afterward), not just append a warning once it's already in the database.
func TestLogCurrentQSORejectsBandOutsideEventAllowedBands(t *testing.T) {
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
	if count, err := st.count(); err != nil || count != 0 {
		t.Fatalf("count = %d, err = %v, want 0 — the QSO must not be committed", count, err)
	}
	if m.fields[fieldCall].Value() != "W1AW" {
		t.Fatal("Call field was cleared even though the QSO was rejected")
	}
}

// TestLogCurrentQSORejectsContestDupeEvenIfCachedWarningIsStale guards
// against relying solely on the cached dupeWarning indicator: it is only
// recomputed by checkDupe, which runs on the call/band/contest-name Update
// paths, not on every possible way a field can change (e.g. SetValue calls
// from tests, or future code paths). logCurrentQSO must re-verify against
// the database immediately before insert regardless of what dupeWarning
// currently holds.
func TestLogCurrentQSORejectsContestDupeEvenIfCachedWarningIsStale(t *testing.T) {
	st, err := openStore(filepath.Join(t.TempDir(), "logger.db"))
	if err != nil {
		t.Fatalf("openStore returned error: %v", err)
	}
	defer st.Close()

	m := initialModel(st)
	arrl := m.events[eventIndex(t, m.events, "ARRL-DX-CW")]
	m.selectEvent(arrl, arrl.Sessions[0])
	contestID := m.contestFields[contestName].Value()

	prior := validTestQSO()
	prior.call, prior.band, prior.contestID, prior.profileID = "W4GNS", "20M", contestID, m.activeStation.ID
	if _, err := st.insertQSO(prior); err != nil {
		t.Fatalf("insert prior QSO: %v", err)
	}

	// Set the callsign/band directly (bypassing Update, which is what
	// recalculates dupeWarning) so the cached indicator is left stale.
	m.fields[fieldCall].SetValue("W4GNS")
	m.fields[fieldBand].SetValue("20M")
	if m.dupeWarning {
		t.Fatal("test setup invalid: dupeWarning should still be stale (false) at this point")
	}

	m, _ = m.logCurrentQSO()
	if !strings.Contains(m.statusMsg, "DUPE") {
		t.Fatalf("statusMsg = %q, want a DUPE rejection despite the stale cached indicator", m.statusMsg)
	}
	if count, err := st.count(); err != nil || count != 1 {
		t.Fatalf("count = %d, err = %v, want 1 (only the pre-existing QSO)", count, err)
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
