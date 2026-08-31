package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
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
// terminal window.
func launchInOwnTerminal() error {
	executable, err := os.Executable()
	if err != nil {
		return fmt.Errorf("locate application executable: %w", err)
	}
	invocation, err := findTerminalInvocation(executable, os.Getenv("TERMINAL"), exec.LookPath)
	if err != nil {
		return err
	}
	if err := exec.Command(invocation.program, invocation.args...).Start(); err != nil {
		return fmt.Errorf("start %s: %w", filepath.Base(invocation.program), err)
	}
	return nil
}

func findTerminalInvocation(executable, preferredTerminal string, lookPath func(string) (string, error)) (terminalInvocation, error) {
	childArgs := []string{executable, terminalChildArg}
	for _, terminal := range append(preferredTerminalCandidates(preferredTerminal), standardTerminalCandidates()...) {
		program, err := lookPath(terminal.program)
		if err != nil {
			continue
		}
		args := append(append([]string{}, terminal.args...), childArgs...)
		return terminalInvocation{program: program, args: args}, nil
	}
	return terminalInvocation{}, fmt.Errorf("no supported terminal emulator found; install one of gnome-terminal, konsole, xfce4-terminal, xterm, kitty, alacritty, wezterm, or foot, or run with %s", inCurrentTerminalArg)
}

func preferredTerminalCandidates(preferred string) []terminalInvocation {
	preferred = strings.TrimSpace(preferred)
	if preferred == "" || strings.ContainsAny(preferred, " \t") {
		return nil
	}
	return terminalCandidatesFor(filepath.Base(preferred), preferred)
}

func standardTerminalCandidates() []terminalInvocation {
	var candidates []terminalInvocation
	for _, terminal := range []string{"gnome-terminal", "konsole", "xfce4-terminal", "xterm", "kitty", "alacritty", "wezterm", "foot"} {
		candidates = append(candidates, terminalCandidatesFor(terminal, terminal)...)
	}
	return candidates
}

func terminalCandidatesFor(name, program string) []terminalInvocation {
	switch name {
	case "gnome-terminal", "kgx", "ptyxis":
		return []terminalInvocation{{program: program, args: []string{"--"}}}
	case "konsole", "xterm", "kitty", "alacritty", "foot":
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
