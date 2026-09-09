package main

import (
	"os"
	"path/filepath"
	"testing"
)

// chdir switches the test's working directory and restores it on cleanup.
func chdir(t *testing.T, dir string) {
	t.Helper()
	old, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chdir(old) })
}

// TestDefaultDBPathPrefersExistingLegacyFile guards backward compatibility:
// an existing install that always launches from one directory must keep
// using its "./w4gns.db" unchanged, not silently switch to a new location.
func TestDefaultDBPathPrefersExistingLegacyFile(t *testing.T) {
	dir := t.TempDir()
	chdir(t, dir)
	if err := os.WriteFile("w4gns.db", []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("XDG_DATA_HOME", filepath.Join(dir, "xdg-data"))
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(dir, "xdg-config"))

	got, err := defaultDBPath()
	if err != nil {
		t.Fatal(err)
	}
	if got != "w4gns.db" {
		t.Errorf("defaultDBPath() = %q, want the existing legacy w4gns.db", got)
	}
}

// withPromptForCallsign overrides the promptForCallsign seam for the
// duration of one test, so no test ever blocks on or reads real stdin.
func withPromptForCallsign(t *testing.T, callsign string) {
	t.Helper()
	old := promptForCallsign
	promptForCallsign = func() string { return callsign }
	t.Cleanup(func() { promptForCallsign = old })
}

// TestDefaultDBPathIsStableAcrossWorkingDirectories guards the actual bug:
// without a legacy file present, the default must resolve to the same path
// regardless of which directory the command was launched from, or an
// operator running the installed (PATH-based) command from a different
// directory than usual would silently get a second, empty database.
func TestDefaultDBPathIsStableAcrossWorkingDirectories(t *testing.T) {
	dataHome := t.TempDir()
	t.Setenv("XDG_DATA_HOME", dataHome)
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	withPromptForCallsign(t, "W1AW")

	dirA, dirB := t.TempDir(), t.TempDir()
	chdir(t, dirA)
	pathFromA, err := defaultDBPath()
	if err != nil {
		t.Fatal(err)
	}
	chdir(t, dirB)
	pathFromB, err := defaultDBPath()
	if err != nil {
		t.Fatal(err)
	}

	if pathFromA != pathFromB {
		t.Fatalf("defaultDBPath() differs by working directory: %q (dir A) vs %q (dir B)", pathFromA, pathFromB)
	}
	if filepath.Dir(pathFromA) != filepath.Join(dataHome, appDirName) {
		t.Errorf("defaultDBPath() = %q, want it under XDG_DATA_HOME/%s", pathFromA, appDirName)
	}
}

// TestDefaultDBPathNamesDatabaseAfterPromptedCallsign covers the actual
// first-run feature: with no database anywhere (no cwd file, nothing in the
// new or legacy stable dir), the operator is prompted and the database is
// named after their callsign, sanitized to a safe filename.
func TestDefaultDBPathNamesDatabaseAfterPromptedCallsign(t *testing.T) {
	dataHome := t.TempDir()
	t.Setenv("XDG_DATA_HOME", dataHome)
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	chdir(t, t.TempDir())
	withPromptForCallsign(t, "  W1AW/4  ")

	got, err := defaultDBPath()
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(dataHome, appDirName, "w1aw4.db")
	if got != want {
		t.Fatalf("defaultDBPath() = %q, want %q (sanitized callsign, stripping the portable-op slash)", got, want)
	}
}

// TestDefaultDBPathReusesExistingCallsignNamedDBWithoutRePrompting guards
// against re-prompting on every launch: once a callsign-named database
// exists in the stable dir, later calls must find it directly.
func TestDefaultDBPathReusesExistingCallsignNamedDBWithoutRePrompting(t *testing.T) {
	dataHome := t.TempDir()
	t.Setenv("XDG_DATA_HOME", dataHome)
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	chdir(t, t.TempDir())
	existing := filepath.Join(dataHome, appDirName)
	if err := os.MkdirAll(existing, 0o700); err != nil {
		t.Fatal(err)
	}
	dbPath := filepath.Join(existing, "w1aw.db")
	if err := os.WriteFile(dbPath, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	old := promptForCallsign
	promptForCallsign = func() string {
		t.Fatal("promptForCallsign was called even though a database already exists")
		return ""
	}
	t.Cleanup(func() { promptForCallsign = old })

	got, err := defaultDBPath()
	if err != nil {
		t.Fatal(err)
	}
	if got != dbPath {
		t.Fatalf("defaultDBPath() = %q, want the existing %q", got, dbPath)
	}
}

// TestDefaultDBPathFallsBackToPreRenameXDGDir guards the migration path for
// an install that predates this app being renamed from w4gns-logger to
// cwlogger: its database under the old XDG directory name must be found
// automatically, not orphaned by a prompt for a "new" database.
func TestDefaultDBPathFallsBackToPreRenameXDGDir(t *testing.T) {
	dataHome := t.TempDir()
	t.Setenv("XDG_DATA_HOME", dataHome)
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	chdir(t, t.TempDir())
	legacyDir := filepath.Join(dataHome, legacyAppDirName)
	if err := os.MkdirAll(legacyDir, 0o700); err != nil {
		t.Fatal(err)
	}
	legacyPath := filepath.Join(legacyDir, "w4gns.db")
	if err := os.WriteFile(legacyPath, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	old := promptForCallsign
	promptForCallsign = func() string {
		t.Fatal("promptForCallsign was called even though a pre-rename database exists")
		return ""
	}
	t.Cleanup(func() { promptForCallsign = old })

	got, err := defaultDBPath()
	if err != nil {
		t.Fatal(err)
	}
	if got != legacyPath {
		t.Fatalf("defaultDBPath() = %q, want the pre-rename %q", got, legacyPath)
	}
}

// TestDefaultDBPathErrorsWhenNoCallsignProvided covers the "or not create
// the db until a callsign is provided" requirement: a blank prompt answer
// (whether a human pressed Enter with nothing, or — as promptForCallsign
// itself guarantees — stdin isn't a terminal at all, e.g. a script or cron
// job) must fail clearly rather than inventing a placeholder database.
func TestDefaultDBPathErrorsWhenNoCallsignProvided(t *testing.T) {
	dataHome := t.TempDir()
	t.Setenv("XDG_DATA_HOME", dataHome)
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	chdir(t, t.TempDir())
	withPromptForCallsign(t, "   ")

	if _, err := defaultDBPath(); err == nil {
		t.Fatal("defaultDBPath() succeeded with a blank callsign, want an error")
	}
}

// TestDefaultDBPathReusesRememberedPathOutsideStandardLocations guards the
// actual incident this hardening fixes: a database that was only ever found
// via the cwd-relative "./w4gns.db" check (or an explicit CWLOGGER_DB) on a
// previous run — so it lives somewhere none of the standard XDG locations
// would ever discover — must still be found on a run from a different
// working directory, instead of silently starting a second, empty database.
func TestDefaultDBPathReusesRememberedPathOutsideStandardLocations(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	chdir(t, t.TempDir())

	elsewhere := filepath.Join(t.TempDir(), "w4gns.db")
	if err := os.WriteFile(elsewhere, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	rememberLastDBPath(elsewhere)
	old := promptForCallsign
	promptForCallsign = func() string {
		t.Fatal("promptForCallsign was called even though a remembered database still exists")
		return ""
	}
	t.Cleanup(func() { promptForCallsign = old })

	got, err := defaultDBPath()
	if err != nil {
		t.Fatal(err)
	}
	if got != elsewhere {
		t.Fatalf("defaultDBPath() = %q, want the remembered %q", got, elsewhere)
	}
}

// TestDefaultDBPathErrorsWhenRememberedPathMissing guards the other half of
// the hardening: if the remembered database has disappeared (wrong working
// directory, unmounted drive, changed XDG_DATA_HOME), defaultDBPath must
// report a clear error rather than silently prompting for a callsign and
// starting a brand-new database next to the operator's real, just
// temporarily unreachable one.
func TestDefaultDBPathErrorsWhenRememberedPathMissing(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	chdir(t, t.TempDir())

	gone := filepath.Join(t.TempDir(), "w4gns.db")
	if err := os.WriteFile(gone, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	rememberLastDBPath(gone)
	if err := os.Remove(gone); err != nil {
		t.Fatal(err)
	}
	old := promptForCallsign
	promptForCallsign = func() string {
		t.Fatal("promptForCallsign was called even though the remembered database is missing")
		return ""
	}
	t.Cleanup(func() { promptForCallsign = old })

	if _, err := defaultDBPath(); err == nil {
		t.Fatal("defaultDBPath() succeeded with the remembered database missing, want an error")
	}
}

func TestSanitizeCallsignForFilename(t *testing.T) {
	cases := map[string]string{
		"W1AW":          "w1aw",
		"  w1aw/4  ":    "w1aw4",
		"K1ZZ-portable": "k1zzportable",
		"":              "",
		"   ":           "",
	}
	for input, want := range cases {
		if got := sanitizeCallsignForFilename(input); got != want {
			t.Errorf("sanitizeCallsignForFilename(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestDefaultQRZKeyPathPrefersExistingLegacyFile(t *testing.T) {
	dir := t.TempDir()
	chdir(t, dir)
	if err := os.WriteFile("qrz.comAPIkey", []byte("KEY"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(dir, "xdg-config"))

	if got := defaultQRZKeyPath(); got != "qrz.comAPIkey" {
		t.Errorf("defaultQRZKeyPath() = %q, want the existing legacy qrz.comAPIkey", got)
	}
}

func TestDefaultQRZKeyPathIsStableAcrossWorkingDirectories(t *testing.T) {
	configHome := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", configHome)

	dirA, dirB := t.TempDir(), t.TempDir()
	chdir(t, dirA)
	pathFromA := defaultQRZKeyPath()
	chdir(t, dirB)
	pathFromB := defaultQRZKeyPath()

	if pathFromA != pathFromB {
		t.Fatalf("defaultQRZKeyPath() differs by working directory: %q (dir A) vs %q (dir B)", pathFromA, pathFromB)
	}
	if filepath.Dir(pathFromA) != filepath.Join(configHome, appDirName) {
		t.Errorf("defaultQRZKeyPath() = %q, want it under XDG_CONFIG_HOME/%s", pathFromA, appDirName)
	}
}

// TestDefaultQRZKeyPathFallsBackToPreRenameXDGDir guards the same migration
// concern as TestDefaultDBPathFallsBackToPreRenameXDGDir for credential
// files: an existing install's qrz.comAPIkey under the old w4gns-logger XDG
// config directory must still be found after the rename to cwlogger.
func TestDefaultQRZKeyPathFallsBackToPreRenameXDGDir(t *testing.T) {
	configHome := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", configHome)
	chdir(t, t.TempDir())
	legacyDir := filepath.Join(configHome, legacyAppDirName)
	if err := os.MkdirAll(legacyDir, 0o700); err != nil {
		t.Fatal(err)
	}
	legacyPath := filepath.Join(legacyDir, "qrz.comAPIkey")
	if err := os.WriteFile(legacyPath, []byte("KEY"), 0o600); err != nil {
		t.Fatal(err)
	}

	if got := defaultQRZKeyPath(); got != legacyPath {
		t.Fatalf("defaultQRZKeyPath() = %q, want the pre-rename %q", got, legacyPath)
	}
}
