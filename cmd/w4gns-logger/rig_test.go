package main

import (
	"bufio"
	"net"
	"os"
	"path/filepath"
	"testing"
)

func TestBandForFrequencyHz(t *testing.T) {
	cases := []struct {
		name string
		hz   float64
		want string
	}{
		{"20M default", 14_025_000, "20M"},
		{"20M upper edge", 14_350_000, "20M"},
		{"160M lower edge", 1_800_000, "160M"},
		{"6M", 50_090_000, "6M"},
		{"between 40M and 30M", 9_500_000, ""},
		{"below 160M", 1_000_000, ""},
		{"above 6M", 60_000_000, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := bandForFrequencyHz(tc.hz); got != tc.want {
				t.Errorf("bandForFrequencyHz(%v) = %q, want %q", tc.hz, got, tc.want)
			}
		})
	}
}

func TestFormatRigFrequencyMHz(t *testing.T) {
	cases := []struct {
		hz   float64
		want string
	}{
		{14_025_000, "14.025"},
		{7_000_000, "7"},
		{28_300_500, "28.3005"},
	}
	for _, tc := range cases {
		if got := formatRigFrequencyMHz(tc.hz); got != tc.want {
			t.Errorf("formatRigFrequencyMHz(%v) = %q, want %q", tc.hz, got, tc.want)
		}
	}
}

func TestLoadRigctldAddrPrefersEnvOverride(t *testing.T) {
	t.Setenv("CWLOGGER_RIGCTLD_ADDR", "192.168.1.50:4532")
	if got := loadRigctldAddr(); got != "192.168.1.50:4532" {
		t.Fatalf("loadRigctldAddr() = %q, want 192.168.1.50:4532", got)
	}
}

func TestSaveRigctldAddrRoundTrip(t *testing.T) {
	t.Setenv("CWLOGGER_RIGCTLD_ADDR", "")
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	if err := saveRigctldAddr("localhost:4532"); err != nil {
		t.Fatal(err)
	}
	if got := loadRigctldAddr(); got != "localhost:4532" {
		t.Fatalf("loadRigctldAddr() = %q, want localhost:4532", got)
	}
	info, err := os.Stat(filepath.Join(dir, appDirName, "rigctld.addr"))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != qrzKeyFilePermBits {
		t.Fatalf("rigctld.addr permissions = %v, want %v", info.Mode().Perm(), qrzKeyFilePermBits)
	}
}

// fakeRigctld starts a one-shot TCP listener that mimics rigctld's "f"
// get_freq response (or an RPRT error line), returning its address.
func fakeRigctld(t *testing.T, response string) string {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { listener.Close() })
	go func() {
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		line, err := bufio.NewReader(conn).ReadString('\n')
		if err != nil || line != "f\n" {
			return
		}
		_, _ = conn.Write([]byte(response))
	}()
	return listener.Addr().String()
}

func TestRigctldFreqHzParsesSuccessResponse(t *testing.T) {
	addr := fakeRigctld(t, "14025000\n")
	hz, err := rigctldFreqHz(addr)
	if err != nil {
		t.Fatal(err)
	}
	if hz != 14_025_000 {
		t.Fatalf("rigctldFreqHz() = %v, want 14025000", hz)
	}
}

func TestRigctldFreqHzReportsRPRTError(t *testing.T) {
	addr := fakeRigctld(t, "RPRT -1\n")
	if _, err := rigctldFreqHz(addr); err == nil {
		t.Fatal("rigctldFreqHz() = nil error, want an error for an RPRT response")
	}
}

func TestRigctldFreqHzReportsUnreachableAddress(t *testing.T) {
	// Nothing listens on this port; dial must fail rather than hang, and
	// must not panic on the never-established connection.
	if _, err := rigctldFreqHz("127.0.0.1:1"); err == nil {
		t.Fatal("rigctldFreqHz() = nil error, want a connection error")
	}
}

func TestRigAutofillEligible(t *testing.T) {
	st, err := openStore(t.TempDir() + "/logger.db")
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	m := initialModel(st)
	m.screen = qsoEntryScreen
	if !m.rigAutofillEligible() {
		t.Fatal("rigAutofillEligible() = false at a fresh, idle QSO Entry screen, want true")
	}

	withCall := m
	withCall.fields[fieldCall].SetValue("W1AW")
	if withCall.rigAutofillEligible() {
		t.Fatal("rigAutofillEligible() = true with a callsign typed, want false")
	}

	editing := m
	editing.editingQSOID = 42
	if editing.rigAutofillEligible() {
		t.Fatal("rigAutofillEligible() = true while editing a QSO, want false")
	}

	focused := m
	focused.tableFocused = true
	if focused.rigAutofillEligible() {
		t.Fatal("rigAutofillEligible() = true with the Recent QSOs table focused, want false")
	}

	otherScreen := m
	otherScreen.screen = stationSetupScreen
	if otherScreen.rigAutofillEligible() {
		t.Fatal("rigAutofillEligible() = true on a screen other than QSO Entry, want false")
	}

	postMode := m
	postMode.postMode = true
	if postMode.rigAutofillEligible() {
		t.Fatal("rigAutofillEligible() = true in POST (backdated) mode, want false — Band/Frequency there describe a past contact, not the live rig")
	}
}

func TestUpdateRigStatusMsgAutofillsWhenEligibleAndIgnoresStaleGeneration(t *testing.T) {
	st, err := openStore(t.TempDir() + "/logger.db")
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	m := initialModel(st)
	m.screen = qsoEntryScreen
	m.rigctldAddr = "127.0.0.1:4532"
	m.rigGeneration = 5

	// A result stamped with an old generation (e.g. from an address that was
	// since reconfigured — see saveStationSetup) must be dropped without
	// mutating the entry fields or the displayed rig status.
	updated, _ := m.Update(rigStatusMsg{generation: 4, freqHz: 21_025_000, band: "15M"})
	stale := updated.(model)
	if stale.rigBand != "" || stale.fields[fieldBand].Value() == "15M" {
		t.Fatalf("stale-generation rigStatusMsg was applied: rigBand=%q band field=%q", stale.rigBand, stale.fields[fieldBand].Value())
	}

	updated, _ = m.Update(rigStatusMsg{generation: 5, freqHz: 14_025_000, band: "20M"})
	fresh := updated.(model)
	if fresh.rigBand != "20M" || fresh.rigFreqMHz != "14.025" {
		t.Fatalf("rig status not recorded: band=%q freq=%q", fresh.rigBand, fresh.rigFreqMHz)
	}
	if fresh.fields[fieldBand].Value() != "20M" || fresh.fields[fieldFrequency].Value() != "14.025" {
		t.Fatalf("Band/Frequency not autofilled from rig: band field=%q freq field=%q", fresh.fields[fieldBand].Value(), fresh.fields[fieldFrequency].Value())
	}
}

func TestUpdateRigStatusMsgDoesNotOverwriteFieldsMidQSO(t *testing.T) {
	st, err := openStore(t.TempDir() + "/logger.db")
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	m := initialModel(st)
	m.screen = qsoEntryScreen
	m.rigctldAddr = "127.0.0.1:4532"
	m.fields[fieldCall].SetValue("W1AW")
	m.fields[fieldBand].SetValue("40M")
	m.fields[fieldFrequency].SetValue("7.025")

	updated, _ := m.Update(rigStatusMsg{generation: 0, freqHz: 14_025_000, band: "20M"})
	next := updated.(model)
	if next.fields[fieldBand].Value() != "40M" || next.fields[fieldFrequency].Value() != "7.025" {
		t.Fatalf("rig poll overwrote an in-progress QSO's Band/Frequency: band=%q freq=%q", next.fields[fieldBand].Value(), next.fields[fieldFrequency].Value())
	}
	// The status line itself still reflects the live rig reading even though
	// the entry fields were left alone.
	if next.rigBand != "20M" {
		t.Fatalf("rigBand = %q, want 20M (status display should still update)", next.rigBand)
	}
}

func TestUpdateRigTickMsgDiesWhenConfigurationCleared(t *testing.T) {
	st, err := openStore(t.TempDir() + "/logger.db")
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	m := initialModel(st)
	m.rigctldAddr = ""
	m.rigGeneration = 3

	_, cmd := m.Update(rigTickMsg{generation: 3})
	if cmd != nil {
		t.Fatal("rigTickMsg with no configured address rescheduled another poll, want the chain to die")
	}
}

func TestUpdateRigTickMsgDiesOnStaleGeneration(t *testing.T) {
	st, err := openStore(t.TempDir() + "/logger.db")
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	m := initialModel(st)
	m.rigctldAddr = "127.0.0.1:4532"
	m.rigGeneration = 3

	_, cmd := m.Update(rigTickMsg{generation: 2})
	if cmd != nil {
		t.Fatal("rigTickMsg from a superseded generation rescheduled another poll, want the chain to die")
	}
}
