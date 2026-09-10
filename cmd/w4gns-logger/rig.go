package main

import (
	"bufio"
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// rigctldDialTimeout/rigctldIOTimeout bound one poll of rigctld so a
// misconfigured address (nothing listening, a firewalled host) or a hung
// daemon can never stall the UI loop — this runs as an async tea.Cmd, same
// discipline as the solar-data and POTA/QRZ lookups.
const (
	rigctldDialTimeout = 2 * time.Second
	rigctldIOTimeout    = 2 * time.Second
	// rigPollInterval balances a responsive band/frequency indicator against
	// not hammering rigctld with short-lived connections. Hamlib's daemon is
	// designed for frequent polling from logging software; this is a coarse,
	// display-oriented rate, not a real-time tracking one.
	rigPollInterval = 2 * time.Second
)

// loadRigctldAddr returns the "host:port" of a running Hamlib rigctld to poll
// for the radio's live frequency, or "" if rig control isn't configured — the
// same env-var/file precedence as loadLoTWStation. rig control is entirely
// optional: an unset/unreachable rigctld never blocks or delays QSO entry.
func loadRigctldAddr() string {
	if addr := strings.TrimSpace(os.Getenv("CWLOGGER_RIGCTLD_ADDR")); addr != "" {
		return addr
	}
	return strings.TrimSpace(firstLine(readLoTWFile(defaultRigctldAddrPath())))
}

// saveRigctldAddr persists the rigctld address entered in Station Setup. See
// loadRigctldAddr for the env-var override this on-disk value yields to.
func saveRigctldAddr(value string) error {
	return writeLoTWFile(defaultRigctldAddrPath(), strings.TrimSpace(value))
}

// rigctldFreqHz queries a running Hamlib rigctld for the radio's current VFO
// frequency using rigctld's "simple" single-letter command protocol: writing
// "f\n" returns one line with the frequency in Hz, or an "RPRT <nonzero>"
// error line if the rig backend rejects the request. This is the same
// protocol other logging software (N1MM, fldigi, etc.) polls rigctld with —
// this app never talks to the radio's own CAT protocol directly, only to the
// already-running rigctld daemon.
func rigctldFreqHz(addr string) (float64, error) {
	conn, err := net.DialTimeout("tcp", addr, rigctldDialTimeout)
	if err != nil {
		return 0, fmt.Errorf("connect to rigctld at %s: %w", addr, err)
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(rigctldIOTimeout))
	if _, err := conn.Write([]byte("f\n")); err != nil {
		return 0, fmt.Errorf("query rigctld: %w", err)
	}
	line, err := bufio.NewReader(conn).ReadString('\n')
	if err != nil {
		return 0, fmt.Errorf("read rigctld response: %w", err)
	}
	line = strings.TrimSpace(line)
	if strings.HasPrefix(line, "RPRT") {
		return 0, fmt.Errorf("rigctld reported an error: %s", line)
	}
	hz, err := strconv.ParseFloat(line, 64)
	if err != nil {
		return 0, fmt.Errorf("unexpected rigctld response %q: %w", line, err)
	}
	return hz, nil
}

// bandForFrequencyHz resolves an amateur band name from a frequency in Hz
// using the same amateurBands table QSO-entry validation uses, so a
// rig-reported frequency and a hand-typed one are judged by identical
// allocation edges. Returns "" for a frequency outside every listed band
// (out-of-band, a general-coverage receive frequency, a WARC/broadcast gap).
func bandForFrequencyHz(hz float64) string {
	mhz := hz / 1_000_000
	for _, band := range amateurBands {
		if mhz >= band.LowMHz && mhz <= band.HighMHz {
			return band.Name
		}
	}
	return ""
}

// formatRigFrequencyMHz renders a rig-reported frequency the same way this
// app's own Frequency field expects to parse it (see validateBandFrequency):
// MHz, trimmed to no more than 6 decimal places (matching rigctld's Hz
// resolution) with no trailing zeros.
func formatRigFrequencyMHz(hz float64) string {
	return strconv.FormatFloat(hz/1_000_000, 'f', -1, 64)
}

type rigStatusMsg struct {
	generation uint64
	freqHz     float64
	band       string
	err        error
}

// fetchRigStatusCmd polls rigctld once. generation is captured at scheduling
// time and echoed back in the result; Update discards a result/tick whose
// generation doesn't match the model's current one — the same staleness
// guard connectK3LR uses for cluster reconnects. Without it, saving Station
// Setup with a changed rigctld address while a poll chain from the old
// address is already running would start a second, parallel chain: the old
// chain's own tick handler has no way to know it's obsolete (it only checks
// whether rig control is configured at all, which stays true), so it would
// keep rescheduling itself forever, doubling polling load on every
// reconfigure within one session.
func fetchRigStatusCmd(addr string, generation uint64) tea.Cmd {
	return func() tea.Msg {
		hz, err := rigctldFreqHz(addr)
		if err != nil {
			return rigStatusMsg{generation: generation, err: err}
		}
		return rigStatusMsg{generation: generation, freqHz: hz, band: bandForFrequencyHz(hz)}
	}
}

type rigTickMsg struct{ generation uint64 }

// rigTickCmd schedules the next poll. Only started/kept alive while rig
// control is configured — see Init and saveStationSetup.
func rigTickCmd(generation uint64) tea.Cmd {
	return tea.Tick(rigPollInterval, func(time.Time) tea.Msg { return rigTickMsg{generation: generation} })
}

// rigAutofillEligible reports whether it's safe to overwrite the Band/
// Frequency fields from the rig right now: only while the operator is idle
// at a blank Call field with no QSO in progress and nothing being edited.
// Once a callsign is typed or a QSO starts, the band/frequency are locked in
// for that contact — same "don't clobber operator input" rule the POTA/QRZ
// autofills already follow — and rig polling keeps running in the background
// so the fields resume tracking the rig again as soon as the form clears.
// POST (after-contest/backdated) mode is excluded entirely: it shares the
// same Call field, but its Band/Frequency describe what was worked at the
// typed historical Date/Time, not whatever the rig is tuned to right now.
func (m model) rigAutofillEligible() bool {
	return m.screen == qsoEntryScreen && !m.postMode && !m.tableFocused && m.editingQSOID == 0 &&
		m.qsoStartedAt.IsZero() && strings.TrimSpace(m.fields[fieldCall].Value()) == ""
}

// rigLine renders the rig-status summary shown under the solar-indices line
// on QSO Entry, matching solarLine's degrade-to-nothing behavior when rig
// control isn't configured at all.
func (m model) rigLine() string {
	if m.rigctldAddr == "" {
		return ""
	}
	if m.rigErr != "" {
		return fmt.Sprintf("Rig: unavailable (%s)", m.rigErr)
	}
	if m.rigFreqMHz == "" {
		return "Rig: connecting…"
	}
	if m.rigBand == "" {
		return fmt.Sprintf("Rig: %s MHz (outside a supported CW band)", m.rigFreqMHz)
	}
	return fmt.Sprintf("Rig: %s MHz (%s)", m.rigFreqMHz, m.rigBand)
}
