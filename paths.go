package main

import (
	"os"
	"path/filepath"
)

// appDirName names this application's directory under the user's XDG data
// and config roots.
const appDirName = "w4gns-logger"

// defaultDBPath resolves the database path used when W4GNS_DB is unset. A
// "./w4gns.db" already present in the current directory is preferred (so an
// existing install that's always launched from one directory keeps using
// that exact file, unchanged); otherwise a stable, working-directory-
// independent path under the user's XDG data directory is used. Without
// this, running the installed command (on PATH, so it starts from whatever
// directory the shell happens to be in) from a different directory than
// usual would silently open or create an unrelated, empty database instead
// of the operator's real log.
func defaultDBPath() string {
	return legacyOrStablePath("w4gns.db", xdgDataDir())
}

// defaultQRZKeyPath resolves the QRZ Logbook API key file path used when
// W4GNS_QRZ_KEY is unset, with the same legacy-cwd-file preference as
// defaultDBPath — otherwise the same cwd-dependence could silently disable
// QRZ uploads after launching from a different directory.
func defaultQRZKeyPath() string {
	return legacyOrStablePath("qrz.comAPIkey", xdgConfigDir())
}

// defaultQRZXMLCredPath resolves the QRZ XML (callsign lookup) credentials
// file path used when W4GNS_QRZ_XML_USER/W4GNS_QRZ_XML_PASS are unset, with
// the same legacy-cwd-file preference as defaultQRZKeyPath. This is a
// separate credential from the Logbook API key: the XML lookup API
// authenticates with a QRZ.com username/password, not an API key.
func defaultQRZXMLCredPath() string {
	return legacyOrStablePath("qrz.comXMLlogin", xdgConfigDir())
}

func legacyOrStablePath(legacyName, stableDir string) string {
	if _, err := os.Stat(legacyName); err == nil {
		return legacyName
	}
	if stableDir == "" {
		return legacyName
	}
	if err := os.MkdirAll(stableDir, 0o700); err != nil {
		return legacyName
	}
	return filepath.Join(stableDir, legacyName)
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
