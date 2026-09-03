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

	// clusterIdleTimeout bounds how long a single read may block. A cluster
	// node (or an intervening NAT) can silently black-hole the TCP flow
	// without sending a RST, which would otherwise leave readNext blocked
	// forever with the UI still showing "connected". If no line arrives within
	// this window the read returns a timeout error, surfacing the dead peer so
	// the auto-reconnect path can recover. It is generous because a quiet CW
	// cluster can legitimately go minutes between spots.
	clusterIdleTimeout = 10 * time.Minute
	// clusterWriteTimeout bounds the login write so a server that accepts the
	// connection but never drains its receive buffer can't block the dial Cmd.
	clusterWriteTimeout = 15 * time.Second

	// Auto-reconnect backoff: cluster nodes cycle/restart routinely, so a
	// dropped feed is reconnected automatically with exponential backoff
	// (guarded by clusterGeneration so a user disconnect cancels it) rather
	// than silently freezing the DX Spots panel until a manual re-connect.
	clusterReconnectBase = 2 * time.Second
	clusterReconnectMax  = 60 * time.Second
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
	return connectK3LRAfter(0, callsign, generation)
}

// connectK3LRAfter dials the cluster after an optional backoff delay, used by
// the auto-reconnect path. A stale attempt (the user disconnected or a newer
// attempt superseded it while this one was waiting) is dropped by the
// generation check on clusterConnectedMsg, so the delay need not be cancelled.
func connectK3LRAfter(delay time.Duration, callsign string, generation uint64) tea.Cmd {
	return func() tea.Msg {
		if delay > 0 {
			time.Sleep(delay)
		}
		conn, err := net.DialTimeout("tcp", k3lrClusterAddr, 10*time.Second)
		if err != nil {
			return clusterConnectedMsg{generation: generation, err: fmt.Errorf("connect to %s: %w", k3lrClusterName, err)}
		}
		_ = conn.SetWriteDeadline(time.Now().Add(clusterWriteTimeout))
		if _, err := fmt.Fprintf(conn, "%s\n", strings.ToUpper(strings.TrimSpace(callsign))); err != nil {
			conn.Close()
			return clusterConnectedMsg{generation: generation, err: fmt.Errorf("send cluster callsign: %w", err)}
		}
		_ = conn.SetWriteDeadline(time.Time{}) // clear; no further writes are made
		scanner := bufio.NewScanner(conn)
		scanner.Buffer(make([]byte, 4*1024), 64*1024)
		return clusterConnectedMsg{generation: generation, client: &clusterClient{conn: conn, scanner: scanner, generation: generation}}
	}
}

func (c *clusterClient) readNext() tea.Cmd {
	return func() tea.Msg {
		// Refresh the idle deadline before each line so a live-but-quiet feed
		// stays connected while a truly dead peer trips the timeout.
		if c.conn != nil {
			_ = c.conn.SetReadDeadline(time.Now().Add(clusterIdleTimeout))
		}
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
