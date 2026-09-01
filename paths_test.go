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

	if got := defaultDBPath(); got != "w4gns.db" {
		t.Errorf("defaultDBPath() = %q, want the existing legacy w4gns.db", got)
	}
}

// TestDefaultDBPathIsStableAcrossWorkingDirectories guards the actual bug:
// without a legacy file present, the default must resolve to the same path
// regardless of which directory the command was launched from, or an
// operator running the installed (PATH-based) command from a different
// directory than usual would silently get a second, empty database.
func TestDefaultDBPathIsStableAcrossWorkingDirectories(t *testing.T) {
	dataHome := t.TempDir()
	t.Setenv("XDG_DATA_HOME", dataHome)

	dirA, dirB := t.TempDir(), t.TempDir()
	chdir(t, dirA)
	pathFromA := defaultDBPath()
	chdir(t, dirB)
	pathFromB := defaultDBPath()

	if pathFromA != pathFromB {
		t.Fatalf("defaultDBPath() differs by working directory: %q (dir A) vs %q (dir B)", pathFromA, pathFromB)
	}
	if filepath.Dir(pathFromA) != filepath.Join(dataHome, appDirName) {
		t.Errorf("defaultDBPath() = %q, want it under XDG_DATA_HOME/%s", pathFromA, appDirName)
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
