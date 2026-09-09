package main

import (
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// lotwUploadTimeout bounds one tqsl sign-and-upload invocation. It covers a
// whole drain batch (potentially several QSOs signed and uploaded in one
// call), so it is longer than the single-QSO QRZ/WRL upload timeouts.
const lotwUploadTimeout = 60 * time.Second

// lotwBackfillTimeout bounds the CLI --upload-lotw / in-app backfill path,
// which can sign and upload an entire log (not just one drain batch's worth
// of freshly logged QSOs) in a single tqsl call.
const lotwBackfillTimeout = 10 * time.Minute

// loadLoTWStation returns the TQSL station-location name used to sign
// outgoing LoTW uploads. CWLOGGER_LOTW_STATION overrides the on-disk file,
// mirroring loadQRZAPIKey/loadWRLAPIKey. An empty return disables LoTW
// forwarding: uploadDestinations only offers uploadDestLoTW when this is
// non-empty.
func loadLoTWStation() string {
	if station := strings.TrimSpace(os.Getenv("CWLOGGER_LOTW_STATION")); station != "" {
		return station
	}
	return strings.TrimSpace(firstLine(readLoTWFile(defaultLoTWStationPath())))
}

// loadLoTWPass returns the signing passphrase for the TQSL Callsign
// Certificate, if the operator's key requires one. CWLOGGER_LOTW_PASS overrides
// the on-disk file. An empty return omits -p from the tqsl invocation, which
// is correct for an unprotected key (see docs/LoTW_Integration_Design.md) and
// would hang/fail under -x for a protected one.
func loadLoTWPass() string {
	if pass := strings.TrimSpace(os.Getenv("CWLOGGER_LOTW_PASS")); pass != "" {
		return pass
	}
	return strings.TrimSpace(firstLine(readLoTWFile(defaultLoTWPassPath())))
}

// loadLoTWLogin returns the LoTW website login used to query confirmation
// (QSL) data from lotwreport.adi. This is the operator's LoTW web-account
// username, distinct from the TQSL Callsign Certificate used to sign
// uploads. CWLOGGER_LOTW_LOGIN overrides the on-disk file. An empty return
// disables confirmation sync.
func loadLoTWLogin() string {
	if login := strings.TrimSpace(os.Getenv("CWLOGGER_LOTW_LOGIN")); login != "" {
		return login
	}
	return strings.TrimSpace(firstLine(readLoTWFile(defaultLoTWLoginPath())))
}

// loadLoTWWebPass returns the LoTW website password paired with
// loadLoTWLogin. CWLOGGER_LOTW_WEBPASS overrides the on-disk file.
func loadLoTWWebPass() string {
	if pass := strings.TrimSpace(os.Getenv("CWLOGGER_LOTW_WEBPASS")); pass != "" {
		return pass
	}
	return strings.TrimSpace(firstLine(readLoTWFile(defaultLoTWWebPassPath())))
}

func readLoTWFile(path string) string {
	tightenKeyFilePermissions(path)
	contents, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return string(contents)
}

// findTQSL resolves the tqsl binary from PATH. CWLOGGER_TQSL overrides it — the
// test seam, pointed at a fake script that exits with a chosen code and
// echoes a "Final Status" line.
func findTQSL() (string, error) {
	if override := strings.TrimSpace(os.Getenv("CWLOGGER_TQSL")); override != "" {
		return override, nil
	}
	path, err := exec.LookPath("tqsl")
	if err != nil {
		return "", fmt.Errorf("tqsl not found in PATH: %w", err)
	}
	return path, nil
}

// lotwExitDelivered classifies a tqsl exit status as a delivered outcome per
// the authoritative TQSL_EXIT_* table in docs/LoTW_Integration_Design.md.
// Codes 9 (some QSOs suppressed, e.g. duplicates) and 14 (already uploaded)
// are delivered on purpose: TQSL's own upload-tracking database will never
// accept those QSOs again, so re-queuing them would retry forever. Every
// other code (including unrecognized ones) is treated as a failure and goes
// through the normal outbox backoff.
func lotwExitDelivered(code int) bool {
	switch code {
	case 0, 8, 9, 14:
		return true
	default:
		return false
	}
}

// tqslFinalStatusText extracts the human-readable status TQSL prints on its
// last line ("... Final Status: <text>(<n>)") for storage in last_error /
// upload_log.detail. Falls back to the raw combined output (trimmed) when the
// expected marker isn't found, so a malformed or unexpected tqsl output still
// leaves something useful for the operator to see rather than nothing.
func tqslFinalStatusText(output string) string {
	const marker = "Final Status: "
	idx := strings.LastIndex(output, marker)
	if idx < 0 {
		return strings.TrimSpace(output)
	}
	rest := output[idx+len(marker):]
	if paren := strings.LastIndexByte(rest, '('); paren >= 0 {
		rest = rest[:paren]
	}
	rest = strings.TrimSpace(rest)
	if rest == "" {
		return strings.TrimSpace(output)
	}
	return rest
}

// runTQSL invokes tqsl to sign and upload adifPath under station, returning
// its exit code and the "Final Status" text it printed. err is only set for
// failures to run the process at all (not found, killed, non-exit-status
// failure); a normal nonzero exit is reported via exitCode, not err, since
// every documented tqsl exit code (including failures) is meaningful and
// handled by the caller.
func runTQSL(ctx context.Context, tqslPath, station, pass, adifPath string) (exitCode int, statusText string, err error) {
	args := []string{"-x", "-d", "-a", "compliant", "-l", station}
	if pass != "" {
		args = append(args, "-p", pass)
	}
	args = append(args, "-u", adifPath)
	cmd := exec.CommandContext(ctx, tqslPath, args...)
	output, runErr := cmd.CombinedOutput()
	statusText = tqslFinalStatusText(string(output))
	var exitErr *exec.ExitError
	switch {
	case runErr == nil:
		return 0, statusText, nil
	case errors.As(runErr, &exitErr):
		return exitErr.ExitCode(), statusText, nil
	default:
		return -1, statusText, fmt.Errorf("run tqsl: %w", runErr)
	}
}

// writeLoTWBatchADIF writes qsos to a fresh ADIF file at path for tqsl to
// sign and upload in one call, reusing adifQSOFields so the fields sent to
// LoTW stay in sync with the bulk ADIF export and QRZ's single-record upload.
func writeLoTWBatchADIF(path string, qsos []qso) error {
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return fmt.Errorf("create LoTW batch ADIF: %w", err)
	}
	defer file.Close()
	if _, err := io.WriteString(file, adifProgramID+" LoTW batch\n<ADIF_VER:"+strconv.Itoa(len(adifVersion))+">"+adifVersion+"<PROGRAMID:"+strconv.Itoa(len(adifProgramID))+">"+adifProgramID+"<EOH>\n"); err != nil {
		return fmt.Errorf("write LoTW batch ADIF header: %w", err)
	}
	for _, q := range qsos {
		for _, field := range adifQSOFields(q) {
			if strings.TrimSpace(field.value) == "" {
				continue
			}
			if err := writeADIFField(file, field.name, field.value); err != nil {
				return err
			}
		}
		if _, err := io.WriteString(file, "<EOR>\n"); err != nil {
			return fmt.Errorf("write LoTW batch ADIF record terminator: %w", err)
		}
	}
	return nil
}

// signAndUploadLoTW writes qsos to a temp ADIF file and hands it to tqsl for
// signing and upload under station, removing the temp file before returning
// regardless of outcome.
func signAndUploadLoTW(ctx context.Context, tqslPath, station, pass string, qsos []qso) (exitCode int, statusText string, err error) {
	tempFile, err := os.CreateTemp("", "w4gns-lotw-*.adi")
	if err != nil {
		return -1, "", fmt.Errorf("create LoTW batch temp file: %w", err)
	}
	tempPath := tempFile.Name()
	tempFile.Close()
	defer os.Remove(tempPath)

	if err := writeLoTWBatchADIF(tempPath, qsos); err != nil {
		return -1, "", err
	}
	return runTQSL(ctx, tqslPath, station, pass, tempPath)
}

// lotwUpdateCheckInterval is how often this app asks tqsl to check for a new
// TQSL version, a new Configuration Data file, and expiring/pending Callsign
// Certificates, per ARRL's developer guidance for programs that drive tqsl on
// the operator's behalf (docs/LoTW_Integration_Design.md, "ARRL developer
// guidance"; <https://lotw.arrl.org/lotw-help/cmdline/?lang=en>). ARRL's docs
// don't specify a cadence; once a day is enough for something that changes at
// most a few times a year.
const lotwUpdateCheckInterval = 24 * time.Hour

// lotwUpdateCheckTimeout bounds the tqsl -n invocation, which only checks a
// version file and certificate expiry (no QSO signing), so it needs far less
// time than a sign-and-upload batch.
const lotwUpdateCheckTimeout = 15 * time.Second

// tqslCertStatus is the shape of ~/.tqsl/cert_status.xml, which tqsl -n
// (re)writes with the revocation status of every locally installed Callsign
// Certificate.
type tqslCertStatus struct {
	Certs []struct {
		Serial string `xml:"serial,attr"`
		Status string `xml:"status"`
	} `xml:"Cert"`
}

// tqslCertStatusPath and tqslAppStatePath locate the two files tqsl -n
// actually writes its findings to. Traced with strace against the installed
// tqsl 2.8.6: ARRL's command-line reference says `-n` "displays the above
// information (if any) to stderr, then exits" with no GUI, but on this
// build/platform it writes stdout/stderr nothing at all, performs the real
// network checks (a curl request to lotw.arrl.org's CRL endpoint for each
// installed cert's serial, plus a TQSL/Configuration Data version check),
// and instead persists the results to these two files before unconditionally
// calling exit(-1) regardless of outcome (a code path that never got wired
// up to return 0 on success on this build) — so neither stdout/stderr nor
// the exit code carry any information here, but these files reliably do.
// Both live under $HOME, which tqsl itself resolves the same way (confirmed
// via strace: it opens exactly $HOME/.tqsl/... and $HOME/.tqslapp), so tests
// redirect them the same way tqsl itself would follow — by overriding HOME.
func tqslCertStatusPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".tqsl", "cert_status.xml"), nil
}

func tqslAppStatePath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".tqslapp"), nil
}

// tqslAppStateValue reads one `Key=Value` line from the ~/.tqslapp
// preferences file tqsl maintains (a flat key=value format, not INI
// sections, until the `[wxHtmlWindow]` marker near the end that this app
// never needs to read past).
func tqslAppStateValue(contents, key string) string {
	prefix := key + "="
	for _, line := range strings.Split(contents, "\n") {
		if rest, ok := strings.CutPrefix(strings.TrimRight(line, "\r"), prefix); ok {
			return strings.TrimSpace(rest)
		}
	}
	return ""
}

// tqslUpdateNoticeFromFiles reads the two structured signals tqsl -n leaves
// behind (see tqslCertStatusPath/tqslAppStatePath) and joins any actionable
// findings into one operator-facing line. A missing/unparseable file (e.g.
// the check has never run) is treated the same as "nothing to report", not
// an error — this is a best-effort informational surface, not something QSO
// delivery depends on.
func tqslUpdateNoticeFromFiles() string {
	var notices []string
	if path, err := tqslCertStatusPath(); err == nil {
		if data, err := os.ReadFile(path); err == nil {
			var status tqslCertStatus
			if xml.Unmarshal(data, &status) == nil {
				for _, cert := range status.Certs {
					if s := strings.TrimSpace(cert.Status); s != "" && !strings.EqualFold(s, "Unrevoked") {
						notices = append(notices, fmt.Sprintf("certificate %s: %s", cert.Serial, s))
					}
				}
			}
		}
	}
	if path, err := tqslAppStatePath(); err == nil {
		if data, err := os.ReadFile(path); err == nil {
			if pending := tqslAppStateValue(string(data), "RequestPending"); pending != "" {
				notices = append(notices, "pending Callsign Certificate request: "+pending)
			}
		}
	}
	return strings.Join(notices, "; ")
}

// checkTQSLUpdates runs `tqsl -n` (alone, per ARRL's TQSL command-line
// reference: "-n... should therefore not be used with any other command line
// option") to make it refresh cert_status.xml/.tqslapp, then reads the
// findings from those files rather than from the process's output or exit
// code — see tqslCertStatusPath for why. Both are ignored deliberately.
func checkTQSLUpdates(ctx context.Context, tqslPath string) string {
	cmd := exec.CommandContext(ctx, tqslPath, "-n")
	_ = cmd.Run()
	return tqslUpdateNoticeFromFiles()
}

// lotwUpdateCheckTickMsg wakes the periodic tqsl -n check.
type lotwUpdateCheckTickMsg struct{}

func lotwUpdateCheckTickCmd() tea.Cmd {
	return tea.Tick(lotwUpdateCheckInterval, func(time.Time) tea.Msg { return lotwUpdateCheckTickMsg{} })
}

// lotwUpdateCheckMsg reports the result of one tqsl -n check. notice is
// empty in the common case (tqsl found nothing to report, or the check
// couldn't run at all) — only non-empty output is surfaced to the operator.
type lotwUpdateCheckMsg struct {
	notice string
}

// lotwUpdateCheckCmd runs the periodic tqsl -n check in the background. It is
// a no-op when LoTW isn't configured, since there's no reason to shell out to
// tqsl for an integration the operator hasn't set up.
func (m model) lotwUpdateCheckCmd() tea.Cmd {
	if strings.TrimSpace(m.lotwStation) == "" {
		return nil
	}
	tqslPath, err := findTQSL()
	if err != nil {
		return nil
	}
	parent := m.bgCtx
	if parent == nil {
		parent = context.Background()
	}
	return runBgCmd(m.bgTasks, func() tea.Msg {
		ctx, cancel := context.WithTimeout(parent, lotwUpdateCheckTimeout)
		defer cancel()
		return lotwUpdateCheckMsg{notice: checkTQSLUpdates(ctx, tqslPath)}
	}, func(recovered any) tea.Msg {
		return lotwUpdateCheckMsg{}
	})
}

// enqueueLoTWBackfill queues every one of profileID's existing QSOs for LoTW
// delivery through the normal outbox drain, in one statement rather than a
// per-QSO round trip. INSERT OR IGNORE keeps it idempotent with rows already
// queued (or already delivered and cleared, which simply get skipped here —
// TQSL's own upload-tracking database would no-op them as duplicates anyway
// if they were re-queued). Returns the number of QSOs newly queued.
func (s *store) enqueueLoTWBackfill(profileID int64) (int64, error) {
	now := time.Now().UTC().Format(time.RFC3339)
	result, err := s.db.Exec(
		`INSERT OR IGNORE INTO upload_outbox (qso_id, profile_id, destination, attempts, next_attempt_at, created_at)
		 SELECT id, profile_id, ?, 0, ?, ? FROM qso WHERE profile_id = ?`,
		uploadDestLoTW, now, now, profileID,
	)
	if err != nil {
		return 0, fmt.Errorf("enqueue LoTW backfill for profile %d: %w", profileID, err)
	}
	return result.RowsAffected()
}

// lotwUploadResult identifies one QSO in a completed batch, for the status
// message summarizing the batch outcome.
type lotwUploadResult struct {
	qsoID int64
	call  string
}

// lotwUploadMsg reports the outcome of one LoTW batch drain. Every QSO in the
// batch shares the same outcome, since they were signed and uploaded together
// in a single tqsl invocation (see docs/LoTW_Integration_Design.md).
type lotwUploadMsg struct {
	results    []lotwUploadResult
	delivered  bool
	statusText string
	err        error
	queueErr   error
}

// lotwOutboxUploadCmd signs and uploads every claimed LoTW QSO in one batch
// tqsl call, then resolves each outbox row from the shared exit code.
// bgTasks makes shutdown wait for this closure while the store is still
// open, matching qrzOutboxUploadCmd/wrlOutboxUploadCmd.
func (m model) lotwOutboxUploadCmd(qsos []qso) tea.Cmd {
	if len(qsos) == 0 {
		return nil
	}
	station := strings.TrimSpace(m.lotwStation)
	if station == "" {
		return nil
	}
	tqslPath, err := findTQSL()
	if err != nil {
		return nil
	}
	pass := m.lotwPass
	parent := m.bgCtx
	if parent == nil {
		parent = context.Background()
	}
	st := m.store
	results := make([]lotwUploadResult, len(qsos))
	for i, q := range qsos {
		results[i] = lotwUploadResult{qsoID: q.id, call: q.call}
	}
	return runBgCmd(m.bgTasks, func() tea.Msg {
		ctx, cancel := context.WithTimeout(parent, lotwUploadTimeout)
		defer cancel()
		exitCode, statusText, runErr := signAndUploadLoTW(ctx, tqslPath, station, pass, qsos)
		if runErr != nil {
			var queueErr error
			for _, q := range qsos {
				if err := st.recordUploadFailure(q.id, uploadDestLoTW, runErr.Error(), time.Now()); err != nil {
					queueErr = err
				}
				_ = st.logUploadEvent(q.id, uploadDestLoTW, q.call, uploadLogFailed, "", runErr.Error())
			}
			return lotwUploadMsg{results: results, err: runErr, queueErr: queueErr}
		}
		delivered := lotwExitDelivered(exitCode)
		var queueErr error
		for _, q := range qsos {
			if delivered {
				if err := st.markUploadDone(q.id, uploadDestLoTW); err != nil {
					queueErr = err
				}
				_ = st.logUploadEvent(q.id, uploadDestLoTW, q.call, uploadLogSent, "", statusText)
				continue
			}
			if err := st.recordUploadFailure(q.id, uploadDestLoTW, statusText, time.Now()); err != nil {
				queueErr = err
			}
			_ = st.logUploadEvent(q.id, uploadDestLoTW, q.call, uploadLogFailed, "", statusText)
		}
		return lotwUploadMsg{results: results, delivered: delivered, statusText: statusText, queueErr: queueErr}
	}, func(r any) tea.Msg {
		return lotwUploadMsg{results: results, err: fmt.Errorf("panic during LoTW upload: %v", r)}
	})
}
