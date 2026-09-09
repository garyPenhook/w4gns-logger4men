package main

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/mattn/go-isatty"
)

// appDirName names this application's directory under the user's XDG data
// and config roots.
const appDirName = "cwlogger"

// legacyAppDirName is the XDG directory name this app used before being
// renamed from w4gns-logger to cwlogger. Checked once as a migration
// fallback (see legacyStableDirFor/legacyXDGDataDir below) so an existing
// install's database and credential files are found automatically rather
// than orphaned by the rename.
const legacyAppDirName = "w4gns-logger"

// maxCallsignFilenameLen bounds the sanitized callsign used to name the
// database file — generous relative to any real amateur radio callsign, just
// enough to stop a pathological prompt answer from producing an unwieldy
// filename.
const maxCallsignFilenameLen = 32

// promptForCallsign is a var (not a plain function) so tests can override it
// and never touch the real terminal/stdin — reading from stdin during `go
// test` would otherwise block indefinitely if a developer happens to run the
// suite from an interactive terminal. Returns "" without prompting when
// stdin isn't a terminal (a script or cron job running e.g. --export-adif
// with CWLOGGER_DB unset and no database created yet) — there is no one to
// answer, and defaultDBPath treats a blank answer as "fail with a clear
// error" rather than silently inventing a placeholder database.
var promptForCallsign = func() string {
	if !isatty.IsTerminal(os.Stdin.Fd()) {
		return ""
	}
	fmt.Fprint(os.Stderr, "No existing database found. Enter your callsign to name your log database: ")
	line, _ := bufio.NewReader(os.Stdin).ReadString('\n')
	return strings.TrimSpace(line)
}

// sanitizeCallsignForFilename reduces a callsign to a safe, portable
// filename component: lowercase ASCII letters and digits only. Amateur radio
// callsigns are alphanumeric — a "/" portable-operation suffix, if any,
// belongs on individual QSOs' station_callsign, not the operator's own
// database name — so anything else here is simply dropped rather than
// rejected outright.
func sanitizeCallsignForFilename(callsign string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(strings.TrimSpace(callsign)) {
		if b.Len() >= maxCallsignFilenameLen {
			break
		}
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// existingDBIn reports the one *.db file already present in dir, if any.
// This app is single-operator (see store.go's openStore), so "a database
// file already exists here" is unambiguous — no need to track which
// filename was chosen beyond finding it again.
func existingDBIn(dir string) (string, bool) {
	matches, err := filepath.Glob(filepath.Join(dir, "*.db"))
	if err != nil || len(matches) == 0 {
		return "", false
	}
	return matches[0], true
}

// defaultDBPath resolves the database path used when CWLOGGER_DB is unset.
// A "./w4gns.db" already present in the current directory is preferred (so
// an existing install that's always launched from one directory keeps using
// that exact file, unchanged); otherwise an existing database under the
// user's XDG data directory is reused, falling back to the pre-rename
// w4gns-logger XDG directory for an install that predates this app being
// renamed from w4gns-logger to cwlogger. If no database exists anywhere
// yet, this is a first run: the operator is prompted for their callsign
// (see promptForCallsign) and the database is named after it (e.g.
// "w1aw.db") rather than after this app's own author, since the file lives
// on that operator's own machine — no database is created until a callsign
// is actually provided, so a script or cron job invoking --export-adif or
// --upload-lotw with CWLOGGER_DB unset and no database yet gets a clear
// error instead of silently starting a fresh, empty "placeholder" database
// (promptForCallsign returns "" without blocking when stdin isn't a
// terminal, so this is the outcome for any non-interactive first run).
// Every subsequent run just finds the file already created — no
// re-prompting. Resolving to a stable, working-directory-independent path
// also matters because the installed command is on PATH and can be launched
// from anywhere: without it, running from an unfamiliar directory would
// silently open or create an unrelated, empty database.
func defaultDBPath() (string, error) {
	if _, err := os.Stat("w4gns.db"); err == nil {
		return "w4gns.db", nil
	}
	dataDir := xdgDataDir()
	if dataDir == "" {
		return "w4gns.db", nil
	}
	if path, ok := existingDBIn(dataDir); ok {
		return path, nil
	}
	if legacyDir := legacyXDGDataDir(); legacyDir != "" {
		if legacyPath := filepath.Join(legacyDir, "w4gns.db"); fileExists(legacyPath) {
			return legacyPath, nil
		}
	}
	filename := sanitizeCallsignForFilename(promptForCallsign())
	if filename == "" {
		return "", fmt.Errorf("no database found and no callsign provided; set CWLOGGER_DB to an explicit path, or run the app in a terminal and enter a callsign when prompted")
	}
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		return "", fmt.Errorf("create %s: %w", dataDir, err)
	}
	return filepath.Join(dataDir, filename+".db"), nil
}

// defaultQRZKeyPath resolves the QRZ Logbook API key file path used when
// CWLOGGER_QRZ_KEY is unset, with the same legacy-cwd-file preference as
// defaultDBPath — otherwise the same cwd-dependence could silently disable
// QRZ uploads after launching from a different directory.
func defaultQRZKeyPath() string {
	return legacyOrStablePath("qrz.comAPIkey", xdgConfigDir())
}

// defaultQRZXMLCredPath resolves the QRZ XML (callsign lookup) credentials
// file path used when CWLOGGER_QRZ_XML_USER/CWLOGGER_QRZ_XML_PASS are
// unset, with the same legacy-cwd-file preference as defaultQRZKeyPath.
// This is a separate credential from the Logbook API key: the XML lookup
// API authenticates with a QRZ.com username/password, not an API key.
func defaultQRZXMLCredPath() string {
	return legacyOrStablePath("qrz.comXMLlogin", xdgConfigDir())
}

// defaultWRLKeyPath resolves the World Radio League API key file path used
// when CWLOGGER_WRL_KEY is unset, with the same legacy-cwd-file preference
// as defaultQRZKeyPath.
func defaultWRLKeyPath() string {
	return legacyOrStablePath("worldradioleague.comAPIkey", xdgConfigDir())
}

// defaultLoTWStationPath resolves the LoTW/TQSL station-location name file
// path used when CWLOGGER_LOTW_STATION is unset, with the same
// legacy-cwd-file preference as defaultQRZKeyPath.
func defaultLoTWStationPath() string {
	return legacyOrStablePath("lotw.station", xdgConfigDir())
}

// defaultLoTWPassPath resolves the LoTW/TQSL signing-passphrase file path
// used when CWLOGGER_LOTW_PASS is unset, with the same legacy-cwd-file
// preference as defaultQRZKeyPath.
func defaultLoTWPassPath() string {
	return legacyOrStablePath("lotw.pass", xdgConfigDir())
}

// defaultLoTWLoginPath resolves the LoTW website login (username) file path
// used when CWLOGGER_LOTW_LOGIN is unset, with the same legacy-cwd-file
// preference as defaultQRZKeyPath. This is the operator's LoTW web-account
// login used by the lotwreport.adi confirmation query, a separate credential
// from the TQSL Callsign Certificate/passphrase used to sign uploads.
func defaultLoTWLoginPath() string {
	return legacyOrStablePath("lotw.login", xdgConfigDir())
}

// defaultLoTWWebPassPath resolves the LoTW website password file path used
// when CWLOGGER_LOTW_WEBPASS is unset, with the same legacy-cwd-file
// preference as defaultQRZKeyPath.
func defaultLoTWWebPassPath() string {
	return legacyOrStablePath("lotw.webpass", xdgConfigDir())
}

func legacyOrStablePath(legacyName, stableDir string) string {
	if _, err := os.Stat(legacyName); err == nil {
		return legacyName
	}
	if stableDir == "" {
		return legacyName
	}
	if fileExists(filepath.Join(stableDir, legacyName)) {
		return filepath.Join(stableDir, legacyName)
	}
	// Migration: this app's config/data directory was renamed from
	// w4gns-logger to cwlogger; an existing install's file under the old
	// directory name is used as-is rather than treating it as newly unset.
	if oldDir := legacyStableDirFor(stableDir); oldDir != "" {
		if oldPath := filepath.Join(oldDir, legacyName); fileExists(oldPath) {
			return oldPath
		}
	}
	if err := os.MkdirAll(stableDir, 0o700); err != nil {
		return legacyName
	}
	return filepath.Join(stableDir, legacyName)
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// legacyStableDirFor maps a new stable config/data directory (ending in
// appDirName) to its pre-rename equivalent (ending in legacyAppDirName), so
// legacyOrStablePath can fall back to it without needing to know whether
// stableDir came from xdgConfigDir or xdgDataDir.
func legacyStableDirFor(stableDir string) string {
	if stableDir == "" {
		return ""
	}
	return filepath.Join(filepath.Dir(stableDir), legacyAppDirName)
}

func xdgDataDir() string {
	if base := os.Getenv("XDG_DATA_HOME"); base != "" {
		return filepath.Join(base, appDirName)
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".local", "share", appDirName)
}

func xdgConfigDir() string {
	if base := os.Getenv("XDG_CONFIG_HOME"); base != "" {
		return filepath.Join(base, appDirName)
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".config", appDirName)
}

// legacyXDGDataDir is the pre-rename equivalent of xdgDataDir, checked once
// by defaultDBPath as a migration fallback.
func legacyXDGDataDir() string {
	if base := os.Getenv("XDG_DATA_HOME"); base != "" {
		return filepath.Join(base, legacyAppDirName)
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".local", "share", legacyAppDirName)
}
