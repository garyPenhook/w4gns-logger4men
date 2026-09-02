package main

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// TestEventForContestIDPrefersLongestMatchingEventID guards against a real
// collision in the live event catalog: "UBA-SPRING-CONTEST" is itself a
// prefix-match of "UBA-SPRING-CONTEST-2" (and there are several other such
// pairs — AGCW-QRP/AGCW-QRP-CONTEST, K1USNSST/K1USNSST-OPEN,
// SKCC-SPRINT/SKCC-SPRINT-EUROPE). Selecting the longer event must resolve
// back to itself, not the shorter ancestor, or the wrong bands/dupe_scope
// get used.
func TestEventForContestIDPrefersLongestMatchingEventID(t *testing.T) {
	st, err := openStore(filepath.Join(t.TempDir(), "logger.db"))
	if err != nil {
		t.Fatalf("openStore returned error: %v", err)
	}
	defer st.Close()

	m := initialModel(st)
	longer := m.events[eventIndex(t, m.events, "UBA-SPRING-CONTEST-2")]
	m.selectEvent(longer, longer.Sessions[0])

	resolved, ok := m.eventForContestID()
	if !ok {
		t.Fatal("eventForContestID did not resolve a match")
	}
	if resolved.ID != "UBA-SPRING-CONTEST-2" {
		t.Fatalf("eventForContestID resolved %q, want %q (the exact event selected, not the shorter prefix ancestor)", resolved.ID, "UBA-SPRING-CONTEST-2")
	}
}

// TestADIFImportResultSurfacesAfterLeavingImportScreen guards against
// losing the async import's result: pressing Esc to leave the Import ADIF
// screen while the import is still running must not prevent its eventual
// adifImportedMsg from updating the status bar and Recent QSOs table once
// it arrives, even though the screen has already changed.
func TestADIFImportResultSurfacesAfterLeavingImportScreen(t *testing.T) {
	st, err := openStore(filepath.Join(t.TempDir(), "logger.db"))
	if err != nil {
		t.Fatalf("openStore returned error: %v", err)
	}
	defer st.Close()

	m := initialModel(st)
	m.screen = adifImportScreen
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = updated.(model)
	if m.screen != qsoEntryScreen {
		t.Fatalf("screen after Esc = %v, want qsoEntryScreen", m.screen)
	}

	updated, _ = m.Update(adifImportedMsg{result: adifImportResult{Imported: 3, Skipped: 1}})
	m = updated.(model)
	if !strings.Contains(m.statusMsg, "3 CW QSOs") {
		t.Fatalf("statusMsg = %q, want it to report the import result even after leaving the import screen", m.statusMsg)
	}
}

// TestQSOEntryHeaderUsesStationProfileTimezone guards against the header's
// "Local" time always reflecting the host machine's timezone instead of
// the configured station profile's — an operator running on a
// differently-configured host still wants their station's own local time.
func TestQSOEntryHeaderUsesStationProfileTimezone(t *testing.T) {
	st, err := openStore(filepath.Join(t.TempDir(), "logger.db"))
	if err != nil {
		t.Fatalf("openStore returned error: %v", err)
	}
	defer st.Close()

	m := initialModel(st)
	m.activeStation.Timezone = "Pacific/Kiritimati" // UTC+14, unlikely to be the host's zone
	view := m.View()
	if !strings.Contains(view, "Pacific/Kiritimati") {
		t.Fatalf("View() header does not mention the configured station timezone Pacific/Kiritimati:\n%s", view)
	}
}

// TestWindowSizeMsgIsRememberedOnModel guards the mechanism saveWindowSize
// relies on at shutdown: the terminal size from the most recent
// tea.WindowSizeMsg must land on the model so it can be persisted.
func TestWindowSizeMsgIsRememberedOnModel(t *testing.T) {
	st, err := openStore(filepath.Join(t.TempDir(), "logger.db"))
	if err != nil {
		t.Fatalf("openStore returned error: %v", err)
	}
	defer st.Close()

	m := initialModel(st)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 132, Height: 43})
	m = updated.(model)
	if m.termWidth != 132 || m.termHeight != 43 {
		t.Fatalf("termWidth/termHeight = %d/%d, want 132/43", m.termWidth, m.termHeight)
	}
}

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

// TestF9TogglesTableFocusAndCursorSurvivesEmptyToNonEmptyTransition guards a
// real regression: bubbles/table's SetRows leaves the cursor at -1 the
// first time it's called with zero rows (which happens once at startup,
// before any QSO exists), and never recovers on its own once real rows
// exist — so without refreshTableRows correcting it, F9 + Enter could never
// select anything.
func TestF9TogglesTableFocusAndCursorSurvivesEmptyToNonEmptyTransition(t *testing.T) {
	st, err := openStore(filepath.Join(t.TempDir(), "logger.db"))
	if err != nil {
		t.Fatalf("openStore returned error: %v", err)
	}
	defer st.Close()

	m := initialModel(st)
	m.refreshTableRows() // mirrors the startup call with zero QSOs logged yet
	if m.table.Cursor() != -1 && m.table.Cursor() != 0 {
		t.Fatalf("cursor on an empty table = %d, want -1 or 0", m.table.Cursor())
	}

	m.fields[fieldCall].SetValue("W1AW")
	m, _ = m.logCurrentQSO()
	if m.table.Cursor() < 0 || m.table.Cursor() >= len(m.recentQSOs) {
		t.Fatalf("cursor after logging the first QSO = %d, want a valid row index into %d rows", m.table.Cursor(), len(m.recentQSOs))
	}

	if m.tableFocused {
		t.Fatal("table should not start focused")
	}
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyF9})
	m = updated.(model)
	if !m.tableFocused || !m.table.Focused() {
		t.Fatal("F9 did not focus the Recent QSOs table")
	}
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyF9})
	m = updated.(model)
	if m.tableFocused || m.table.Focused() {
		t.Fatal("second F9 did not unfocus the Recent QSOs table")
	}
}

// TestEditQSOFlowSavesChangesWithoutInsertingANewRow drives the full
// F9 -> Enter -> edit -> save cycle through Update, the way a real
// keystroke sequence would, and confirms the existing row is updated in
// place (not duplicated) with its original timestamp preserved.
func TestEditQSOFlowSavesChangesWithoutInsertingANewRow(t *testing.T) {
	st, err := openStore(filepath.Join(t.TempDir(), "logger.db"))
	if err != nil {
		t.Fatalf("openStore returned error: %v", err)
	}
	defer st.Close()

	m := initialModel(st)
	m.fields[fieldCall].SetValue("W1AW")
	m, _ = m.logCurrentQSO()
	original, err := st.qsoByID(m.recentQSOs[0].id)
	if err != nil {
		t.Fatal(err)
	}

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyF9})
	m = updated.(model)
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(model)
	if m.editingQSOID != original.id {
		t.Fatalf("editingQSOID = %d, want %d", m.editingQSOID, original.id)
	}
	if m.tableFocused {
		t.Fatal("entering edit mode should release table focus")
	}
	if m.fields[fieldCall].Value() != "W1AW" {
		t.Fatalf("Call field = %q after beginEditQSO, want W1AW", m.fields[fieldCall].Value())
	}

	m.fields[fieldRSTSent].SetValue("579")
	m, _ = m.logCurrentQSO()
	if m.editingQSOID != 0 {
		t.Fatalf("editingQSOID = %d after save, want 0", m.editingQSOID)
	}
	if !strings.Contains(m.statusMsg, "updated") {
		t.Fatalf("statusMsg = %q, want an updated confirmation", m.statusMsg)
	}

	got, err := st.qsoByID(original.id)
	if err != nil {
		t.Fatal(err)
	}
	if got.rstSent != "579" {
		t.Errorf("rstSent = %q, want 579", got.rstSent)
	}
	if !got.time.Equal(original.time) {
		t.Errorf("time = %v, want unchanged %v", got.time, original.time)
	}
	if count, err := st.count(); err != nil || count != 1 {
		t.Fatalf("count = %d, err = %v, want 1 (edit must not insert a new row)", count, err)
	}
}

// TestEscCancelsEditWithoutQuitting covers the contextual Esc behavior: it
// must cancel an in-progress edit (discarding changes) rather than quitting
// the whole app, which is Esc's normal meaning on this screen.
func TestEscCancelsEditWithoutQuitting(t *testing.T) {
	st, err := openStore(filepath.Join(t.TempDir(), "logger.db"))
	if err != nil {
		t.Fatalf("openStore returned error: %v", err)
	}
	defer st.Close()

	m := initialModel(st)
	m.fields[fieldCall].SetValue("W1AW")
	m, _ = m.logCurrentQSO()
	m.beginEditQSO(m.recentQSOs[0])
	m.fields[fieldRSTSent].SetValue("579")

	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = updated.(model)
	if cmd != nil {
		t.Fatal("Esc while editing returned a command (expected no tea.Quit)")
	}
	if m.editingQSOID != 0 {
		t.Fatal("Esc did not cancel the in-progress edit")
	}
	got, err := st.qsoByID(m.recentQSOs[0].id)
	if err != nil {
		t.Fatal(err)
	}
	if got.rstSent == "579" {
		t.Fatal("Esc-cancelled edit was persisted to the database")
	}
}

// TestDeleteRequiresSecondDConfirmation covers the delete-arm/confirm/
// cancel-on-other-key flow, driven through Update the way real keystrokes
// would arrive.
func TestDeleteRequiresSecondDConfirmation(t *testing.T) {
	st, err := openStore(filepath.Join(t.TempDir(), "logger.db"))
	if err != nil {
		t.Fatalf("openStore returned error: %v", err)
	}
	defer st.Close()

	m := initialModel(st)
	m.fields[fieldCall].SetValue("W1AW")
	m, _ = m.logCurrentQSO()

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyF9})
	m = updated.(model)
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("d")})
	m = updated.(model)
	if !m.deleteArmed {
		t.Fatal("first d press did not arm delete")
	}
	if count, _ := st.count(); count != 1 {
		t.Fatal("first d press deleted the row instead of arming confirmation")
	}

	// Any other key cancels the armed delete.
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m = updated.(model)
	if m.deleteArmed {
		t.Fatal("a non-d key did not cancel the armed delete")
	}
	if count, _ := st.count(); count != 1 {
		t.Fatal("delete happened despite being cancelled")
	}

	// Arm again and confirm this time.
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("d")})
	m = updated.(model)
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("d")})
	m = updated.(model)
	if count, err := st.count(); err != nil || count != 0 {
		t.Fatalf("count after confirmed delete = %d, err = %v, want 0", count, err)
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
	want := []int{fieldRSTSent, fieldRSTRcvd, fieldBand, fieldFrequency, fieldCall}
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

// TestAdifExportCmdWritesTimestampedFileToDownloads covers the in-app ADIF
// export end to end: the tea.Cmd it returns must write a real file under
// the operator's Downloads folder and report its path and QSO count via
// adifExportedMsg, not just format a filename.
func TestAdifExportCmdWritesTimestampedFileToDownloads(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	st, err := openStore(filepath.Join(t.TempDir(), "logger.db"))
	if err != nil {
		t.Fatalf("openStore returned error: %v", err)
	}
	defer st.Close()

	m := initialModel(st)
	m.activeStation.Callsign = "W4GNS"
	logged := validTestQSO()
	logged.profileID = m.activeStation.ID
	if _, err := st.insertQSO(logged); err != nil {
		t.Fatalf("insertQSO returned error: %v", err)
	}

	msg, ok := m.adifExportCmd()().(adifExportedMsg)
	if !ok {
		t.Fatalf("adifExportCmd()() = %T, want adifExportedMsg", msg)
	}
	if msg.err != nil {
		t.Fatalf("adifExportedMsg.err = %v, want nil", msg.err)
	}
	if msg.count != 1 {
		t.Fatalf("adifExportedMsg.count = %d, want 1", msg.count)
	}
	wantDir := filepath.Join(home, "Downloads")
	if filepath.Dir(msg.path) != wantDir {
		t.Fatalf("export path = %q, want it under %q", msg.path, wantDir)
	}
	if !strings.HasPrefix(filepath.Base(msg.path), "W4GNS_") || !strings.HasSuffix(msg.path, ".adi") {
		t.Fatalf("export filename = %q, want a W4GNS_<timestamp>.adi shape", filepath.Base(msg.path))
	}
	if _, err := os.Stat(msg.path); err != nil {
		t.Fatalf("exported file does not exist: %v", err)
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

// TestValidateArgsRejectsUnrecognizedAndIncompleteFlags guards against a
// typo'd flag (e.g. --export-adiff) or a dropped path argument silently
// falling through to launching the TUI instead of reporting a usage error.
func TestValidateArgsRejectsUnrecognizedAndIncompleteFlags(t *testing.T) {
	for _, args := range [][]string{
		{"--export-adiff", "log.adi"},
		{"--exprot-adif", "log.adi"},
		{"--export-adif"},
		{"--import-adif"},
	} {
		if err := validateArgs(args); err == nil {
			t.Errorf("validateArgs(%v) returned no error", args)
		}
	}
	for _, args := range [][]string{
		nil,
		{"--export-adif", "log.adi"},
		{"--import-adif", "log.adi"},
		{"--in-current-terminal"},
		{"--terminal-child"},
		{"--version"},
	} {
		if err := validateArgs(args); err != nil {
			t.Errorf("validateArgs(%v) returned error: %v", args, err)
		}
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

// TestQRZCallsignLookupMsgIgnoresStaleResult guards against a slow QRZ XML
// lookup for one callsign landing after the operator has already moved on to
// a different call: the result must be discarded, not written into the
// current QSO's details.
func TestQRZCallsignLookupMsgIgnoresStaleResult(t *testing.T) {
	st, err := openStore(filepath.Join(t.TempDir(), "logger.db"))
	if err != nil {
		t.Fatalf("openStore returned error: %v", err)
	}
	defer st.Close()

	m := initialModel(st)
	m.fields[fieldCall].SetValue("K1ABC")

	updated, _ := m.Update(qrzCallsignLookupMsg{
		call:   "AA7BQ",
		record: qrzCallsignRecord{name: "Fred Lloyd", qth: "Phoenix", state: "AZ", grid: "DM43"},
	})
	m = updated.(model)
	if m.detailFields[detailName].Value() != "" {
		t.Fatalf("detailFields[detailName] = %q, want blank for a stale lookup result", m.detailFields[detailName].Value())
	}
}

// TestQRZCallsignLookupMsgFillsOnlyBlankFields guards against clobbering
// details the operator already typed (or that were loaded from an existing
// logged QSO for editing) with a QRZ lookup result for the same call.
func TestQRZCallsignLookupMsgFillsOnlyBlankFields(t *testing.T) {
	st, err := openStore(filepath.Join(t.TempDir(), "logger.db"))
	if err != nil {
		t.Fatalf("openStore returned error: %v", err)
	}
	defer st.Close()

	m := initialModel(st)
	m.fields[fieldCall].SetValue("AA7BQ")
	m.detailFields[detailQTH].SetValue("operator-entered QTH")

	updated, _ := m.Update(qrzCallsignLookupMsg{
		call:       "AA7BQ",
		sessionKey: "session123",
		record:     qrzCallsignRecord{name: "Fred Lloyd", qth: "Phoenix", state: "AZ", grid: "DM43"},
	})
	m = updated.(model)
	if m.detailFields[detailName].Value() != "Fred Lloyd" {
		t.Fatalf("detailFields[detailName] = %q, want Fred Lloyd", m.detailFields[detailName].Value())
	}
	if m.detailFields[detailQTH].Value() != "operator-entered QTH" {
		t.Fatalf("detailFields[detailQTH] = %q, want the operator-entered value left untouched", m.detailFields[detailQTH].Value())
	}
	if m.detailFields[detailState].Value() != "AZ" || m.detailFields[detailGrid].Value() != "DM43" {
		t.Fatalf("state/grid = %q/%q, want AZ/DM43", m.detailFields[detailState].Value(), m.detailFields[detailGrid].Value())
	}
	if m.qrzXMLSessionKey != "session123" {
		t.Fatalf("qrzXMLSessionKey = %q, want session123 to be cached for the next lookup", m.qrzXMLSessionKey)
	}
}

// TestSaveStationSetupPersistsQRZXMLCredentials covers entering QRZ XML
// login in Station Setup: saving must write it to the credentials file (so
// it survives a restart), update the in-memory creds used by the next
// lookup, and drop any cached session key since it belonged to whichever
// account was previously configured.
func TestSaveStationSetupPersistsQRZXMLCredentials(t *testing.T) {
	st, err := openStore(filepath.Join(t.TempDir(), "logger.db"))
	if err != nil {
		t.Fatalf("openStore returned error: %v", err)
	}
	defer st.Close()
	oldWD, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(oldWD)
	// legacyOrStablePath prefers a "qrz.comXMLlogin" already present in the
	// cwd; chdir into an empty tempdir first so this test's outcome doesn't
	// depend on whatever the invoking process's working directory contains.
	if err := os.Chdir(t.TempDir()); err != nil {
		t.Fatal(err)
	}
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	m := initialModel(st)
	m.qrzXMLSessionKey = "stale-session"
	m.openStationSetup()
	m.stationFields[stationQRZXMLUserField].SetValue("newuser")
	m.stationFields[stationQRZXMLPassField].SetValue("newpass")

	m.saveStationSetup()

	if m.qrzXMLCreds.username != "newuser" || m.qrzXMLCreds.password != "newpass" {
		t.Fatalf("qrzXMLCreds = %+v, want newuser/newpass", m.qrzXMLCreds)
	}
	if m.qrzXMLSessionKey != "" {
		t.Fatalf("qrzXMLSessionKey = %q, want cleared after credentials changed", m.qrzXMLSessionKey)
	}
	if m.screen != qsoEntryScreen {
		t.Fatalf("screen after save = %v, want qsoEntryScreen", m.screen)
	}
	if got := loadQRZXMLCredentials(); got.username != "newuser" || got.password != "newpass" {
		t.Fatalf("loadQRZXMLCredentials() after save = %+v, want newuser/newpass", got)
	}
}
