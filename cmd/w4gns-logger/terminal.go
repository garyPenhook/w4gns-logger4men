package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

const (
	terminalChildArg     = "--terminal-child"
	inCurrentTerminalArg = "--in-current-terminal"
)

type terminalInvocation struct {
	program string
	args    []string
}

// launchInOwnTerminal starts this executable in a separate terminal window.
// The child marker prevents the newly launched instance from opening another
// terminal window. If a terminal size was remembered from the last run (see
// windowsize.go) and the chosen emulator supports requesting one, the new
// window opens at that size instead of the emulator's own default.
func launchInOwnTerminal() error {
	if runtime.GOOS == "linux" && (os.Getenv("SSH_CONNECTION") != "" || (os.Getenv("DISPLAY") == "" && os.Getenv("WAYLAND_DISPLAY") == "")) {
		return fmt.Errorf("no local graphical session")
	}
	executable, err := os.Executable()
	if err != nil {
		return fmt.Errorf("locate application executable: %w", err)
	}
	invocation, err := findTerminalInvocation(executable, os.Getenv("TERMINAL"), loadWindowGeometry(), exec.LookPath)
	if err != nil {
		return err
	}
	cmd := exec.Command(invocation.program, invocation.args...)
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start %s: %w", filepath.Base(invocation.program), err)
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case err := <-done:
		if err != nil {
			return fmt.Errorf("terminal exited before launch: %w", err)
		}
		return nil
	case <-time.After(500 * time.Millisecond):
		return nil
	}
}

func findTerminalInvocation(executable, preferredTerminal, geometry string, lookPath func(string) (string, error)) (terminalInvocation, error) {
	childArgs := []string{executable, terminalChildArg}
	for _, terminal := range append(preferredTerminalCandidates(preferredTerminal, geometry), standardTerminalCandidates(geometry)...) {
		program, err := lookPath(terminal.program)
		if err != nil {
			continue
		}
		args := append(append([]string{}, terminal.args...), childArgs...)
		return terminalInvocation{program: program, args: args}, nil
	}
	return terminalInvocation{}, fmt.Errorf("no supported terminal emulator found; install one of gnome-terminal, konsole, xfce4-terminal, xterm, kitty, alacritty, wezterm, or foot, or run with %s", inCurrentTerminalArg)
}

func preferredTerminalCandidates(preferred, geometry string) []terminalInvocation {
	preferred = strings.TrimSpace(preferred)
	if preferred == "" || strings.ContainsAny(preferred, " \t") {
		return nil
	}
	return terminalCandidatesFor(filepath.Base(preferred), preferred, geometry)
}

func standardTerminalCandidates(geometry string) []terminalInvocation {
	var candidates []terminalInvocation
	for _, terminal := range []string{"gnome-terminal", "konsole", "xfce4-terminal", "xterm", "kitty", "alacritty", "wezterm", "foot"} {
		candidates = append(candidates, terminalCandidatesFor(terminal, terminal, geometry)...)
	}
	return candidates
}

// terminalCandidatesFor builds the invocation for one terminal emulator.
// geometry (an X11-style "COLSxROWS" string from windowsize.go), when
// non-empty, is only applied for the emulators confirmed (via their own
// --help output) to accept that exact character-cell syntax on the command
// line: xterm's -geometry and gnome-terminal's --geometry. Other emulators
// either have no equivalent (konsole's only documented --qwindowgeometry is
// pixel-based, not character cells) or weren't verified, so they're left at
// their own default size rather than guessing an unconfirmed flag.
func terminalCandidatesFor(name, program, geometry string) []terminalInvocation {
	switch name {
	case "gnome-terminal", "kgx", "ptyxis":
		args := []string{}
		if geometry != "" && name == "gnome-terminal" {
			args = append(args, "--geometry="+geometry)
		}
		return []terminalInvocation{{program: program, args: append(args, "--")}}
	case "xterm":
		args := []string{}
		if geometry != "" {
			args = append(args, "-geometry", geometry)
		}
		return []terminalInvocation{{program: program, args: append(args, "-e")}}
	case "konsole", "kitty", "alacritty", "foot":
		return []terminalInvocation{{program: program, args: []string{"-e"}}}
	case "xfce4-terminal":
		return []terminalInvocation{{program: program, args: []string{"--execute"}}}
	case "wezterm":
		return []terminalInvocation{{program: program, args: []string{"start", "--"}}}
	default:
		return []terminalInvocation{{program: program, args: []string{"-e"}}}
	}
}

func hasArg(args []string, want string) bool {
	for _, arg := range args {
		if arg == want {
			return true
		}
	}
	return false
}
