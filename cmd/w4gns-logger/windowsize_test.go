package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSaveAndLoadWindowSizeRoundTrips(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	saveWindowSize(132, 43)
	if got := loadWindowGeometry(); got != "132x43" {
		t.Fatalf("loadWindowGeometry() = %q, want 132x43", got)
	}
}

func TestSaveWindowSizeIgnoresDegenerateDimensions(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	saveWindowSize(0, 0)
	if got := loadWindowGeometry(); got != "" {
		t.Fatalf("loadWindowGeometry() after saving 0x0 = %q, want empty (nothing should have been written)", got)
	}
}

func TestLoadWindowGeometryReturnsEmptyWhenNothingSaved(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	if got := loadWindowGeometry(); got != "" {
		t.Fatalf("loadWindowGeometry() with nothing saved = %q, want empty", got)
	}
}

func TestLoadWindowGeometryRejectsCorruptFile(t *testing.T) {
	configHome := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", configHome)

	dir := filepath.Join(configHome, appDirName)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "window-size"), []byte("not-a-size"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := loadWindowGeometry(); got != "" {
		t.Fatalf("loadWindowGeometry() with a corrupt file = %q, want empty", got)
	}
}
