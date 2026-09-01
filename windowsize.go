package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// windowSizeMinDim is the smallest width/height worth remembering. A
// WindowSizeMsg can briefly report 0x0 during startup/teardown; saving that
// would make the next launch request a degenerate window size.
const windowSizeMinDim = 10

// defaultWindowSizePath resolves the path used to remember the terminal
// size across launches, alongside the QRZ key file under the user's XDG
// config directory (see paths.go). There's no legacy cwd-file case here —
// this is a new file, not one earlier versions ever wrote.
func defaultWindowSizePath() string {
	dir := xdgConfigDir()
	if dir == "" {
		return ""
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return ""
	}
	return filepath.Join(dir, "window-size")
}

// saveWindowSize best-effort remembers the terminal size (in character
// cells) so launchInOwnTerminal can ask the next spawned terminal window to
// open at the same size, instead of the operator resizing it by hand every
// time. Failures are silent: this is a convenience, not something worth
// failing shutdown over.
func saveWindowSize(width, height int) {
	if width < windowSizeMinDim || height < windowSizeMinDim {
		return
	}
	path := defaultWindowSizePath()
	if path == "" {
		return
	}
	_ = os.WriteFile(path, []byte(fmt.Sprintf("%dx%d\n", width, height)), 0o600)
}

// loadWindowGeometry returns the remembered terminal size as an X11-style
// "COLSxROWS" geometry string (what xterm's -geometry and gnome-terminal's
// --geometry both accept), or "" if none is saved or it's unreadable.
func loadWindowGeometry() string {
	path := defaultWindowSizePath()
	if path == "" {
		return ""
	}
	contents, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	width, height, ok := parseWindowSize(string(contents))
	if !ok {
		return ""
	}
	return fmt.Sprintf("%dx%d", width, height)
}

func parseWindowSize(s string) (width, height int, ok bool) {
	s = strings.TrimSpace(s)
	cols, rows, found := strings.Cut(s, "x")
	if !found {
		return 0, 0, false
	}
	width, errW := strconv.Atoi(cols)
	height, errH := strconv.Atoi(rows)
	if errW != nil || errH != nil || width < windowSizeMinDim || height < windowSizeMinDim {
		return 0, 0, false
	}
	return width, height, true
}
