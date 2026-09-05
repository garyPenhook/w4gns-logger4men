package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestReviewTerminalFallback(t *testing.T) {
	t.Setenv("SSH_CONNECTION", "test-session")
	if err := launchInOwnTerminal(); err == nil {
		t.Fatal("SSH launch should stay in current terminal")
	}
	t.Setenv("SSH_CONNECTION", "")
	t.Setenv("DISPLAY", "")
	t.Setenv("WAYLAND_DISPLAY", "")
	if err := launchInOwnTerminal(); err == nil {
		t.Fatal("headless launch should stay in current terminal")
	}
	// A harmless executable models an emulator that exists but fails at once.
	falsePath, err := exec.LookPath("false")
	if err != nil {
		t.Skip("false executable unavailable")
	}
	dir := t.TempDir()
	terminal := filepath.Join(dir, "xterm")
	if err := os.Symlink(falsePath, terminal); err != nil {
		t.Fatal(err)
	}
	t.Chdir(dir)
	t.Setenv("XDG_CONFIG_HOME", dir)
	t.Setenv("DISPLAY", ":review-test")
	t.Setenv("TERMINAL", terminal)
	if err := launchInOwnTerminal(); err == nil {
		t.Fatal("failed emulator incorrectly reported success")
	}
}
