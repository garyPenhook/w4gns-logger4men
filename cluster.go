package main

import (
	"bufio"
	"fmt"
	"io"
	"net"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

const (
	k3lrClusterName = "K3LR DX Cluster"
	k3lrClusterAddr = "dx.k3lr.com:23"
)

type clusterSpot struct {
	Spotter   string
	Frequency string
	Callsign  string
	Comment   string
	Received  time.Time
}

type clusterClient struct {
	conn       net.Conn
	scanner    *bufio.Scanner
	generation uint64
}

type clusterConnectedMsg struct {
	client     *clusterClient
	generation uint64
	err        error
}

type clusterLineMsg struct {
	line       string
	generation uint64
	err        error
}

func connectK3LR(callsign string, generation uint64) tea.Cmd {
	return func() tea.Msg {
		conn, err := net.DialTimeout("tcp", k3lrClusterAddr, 10*time.Second)
		if err != nil {
			return clusterConnectedMsg{generation: generation, err: fmt.Errorf("connect to %s: %w", k3lrClusterName, err)}
		}
		if _, err := fmt.Fprintf(conn, "%s\n", strings.ToUpper(strings.TrimSpace(callsign))); err != nil {
			conn.Close()
			return clusterConnectedMsg{generation: generation, err: fmt.Errorf("send cluster callsign: %w", err)}
		}
		scanner := bufio.NewScanner(conn)
		scanner.Buffer(make([]byte, 4*1024), 64*1024)
		return clusterConnectedMsg{generation: generation, client: &clusterClient{conn: conn, scanner: scanner, generation: generation}}
	}
}

func (c *clusterClient) readNext() tea.Cmd {
	return func() tea.Msg {
		if c.scanner.Scan() {
			return clusterLineMsg{generation: c.generation, line: c.scanner.Text()}
		}
		if err := c.scanner.Err(); err != nil {
			return clusterLineMsg{generation: c.generation, err: err}
		}
		return clusterLineMsg{generation: c.generation, err: io.EOF}
	}
}

func (c *clusterClient) close() error {
	if c == nil || c.conn == nil {
		return nil
	}
	return c.conn.Close()
}

// parseClusterSpot accepts the standard "DX de" format emitted by AR-Cluster
// and DXSpider nodes. Non-spot banner and login lines are intentionally ignored.
func parseClusterSpot(line string, received time.Time) (clusterSpot, bool) {
	fields := strings.Fields(line)
	if len(fields) < 5 || !strings.EqualFold(fields[0], "DX") || !strings.EqualFold(fields[1], "de") {
		return clusterSpot{}, false
	}
	spotter := strings.TrimSuffix(fields[2], ":")
	if spotter == "" {
		return clusterSpot{}, false
	}
	return clusterSpot{
		Spotter:   sanitizeClusterText(spotter),
		Frequency: sanitizeClusterText(fields[3]),
		Callsign:  strings.ToUpper(sanitizeClusterText(fields[4])),
		Comment:   sanitizeClusterText(strings.Join(fields[5:], " ")),
		Received:  received.UTC(),
	}, true
}

// sanitizeClusterText strips ASCII control characters (C0, DEL) and Unicode
// C1 controls from cluster-sourced text before it's stored or rendered.
// Spots originate from other operators on the DX cluster network (or
// anyone able to reach it) and are rendered directly to the terminal;
// without this, an ANSI escape or OSC sequence smuggled into a callsign or
// comment could reposition the cursor, spoof UI text, or trigger terminal
// features such as an OSC 52 clipboard write.
func sanitizeClusterText(s string) string {
	return strings.Map(func(r rune) rune {
		switch {
		case r == '\t':
			return ' '
		case r < 0x20, r == 0x7f, r >= 0x80 && r <= 0x9f:
			return -1
		default:
			return r
		}
	}, s)
}
