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
		Spotter:   spotter,
		Frequency: fields[3],
		Callsign:  strings.ToUpper(fields[4]),
		Comment:   strings.Join(fields[5:], " "),
		Received:  received.UTC(),
	}, true
}
