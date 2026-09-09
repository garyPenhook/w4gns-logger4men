package main

import (
	"context"
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

// statsAwardCount is the number of awards the stats panel pages through, and
// the length of lotwAwardStats' fields in display order.
const statsAwardCount = 5

// statsAwardLabels names the awards in the order statsPanelAward returns
// them, matching lotwAwardStats' field order.
var statsAwardLabels = [statsAwardCount]string{"DXCC", "WAS", "WAZ", "VUCC", "IOTA"}

// openStatsPanel opens the award/analytics stats screen (Ctrl+A): DXCC/WAS/
// WAZ/VUCC/IOTA worked-vs-confirmed progress, computed from local data only
// (qso + lotw_confirmation) so it's instant even offline — no network call on
// open, matching the design in docs/LoTW_Integration_Design.md's "Phase 2"
// stats panel.
func (m *model) openStatsPanel() {
	m.setTableFocused(false)
	m.screen = statsScreen
	m.statsSyncMsg = ""
	m.refreshLoTWStats()
}

// refreshLoTWStats recomputes m.lotwStats for the active profile. Cheap
// (local queries only), so it's safe to call on every panel open and after
// every sync.
func (m *model) refreshLoTWStats() {
	stats, err := m.store.loadLoTWAwardStats(m.activeStation.ID)
	if err != nil {
		m.statusMsg = "stats: " + err.Error()
		return
	}
	m.lotwStats = &stats
}

// statsPanelAward returns the awardProgress for statsAwardFocus's position
// (0=DXCC .. 4=IOTA, per statsAwardLabels) plus the label to display, or
// false if stats haven't loaded yet.
func (m model) statsPanelAward(index int) (string, awardProgress, bool) {
	if m.lotwStats == nil {
		return "", awardProgress{}, false
	}
	switch index {
	case 0:
		return statsAwardLabels[0], m.lotwStats.DXCC, true
	case 1:
		return statsAwardLabels[1], m.lotwStats.WAS, true
	case 2:
		return statsAwardLabels[2], m.lotwStats.WAZ, true
	case 3:
		return statsAwardLabels[3], m.lotwStats.VUCC, true
	case 4:
		return statsAwardLabels[4], m.lotwStats.IOTA, true
	default:
		return "", awardProgress{}, false
	}
}

// lotwStatsSyncMsg reports the outcome of a manual confirmation sync
// triggered from the stats panel.
type lotwStatsSyncMsg struct {
	summary lotwSyncSummary
	err     error
}

// lotwStatsSyncCmd runs syncLoTWConfirmations in the background so the sync's
// network round trip never blocks the terminal UI. Wrapped in runBgCmd,
// matching lotwOutboxUploadCmd, so shutdown waits for an in-flight sync
// rather than racing the store's Close.
func (m model) lotwStatsSyncCmd() tea.Cmd {
	login, webpass := m.lotwLogin, m.lotwWebPass
	if strings.TrimSpace(login) == "" || strings.TrimSpace(webpass) == "" {
		return nil
	}
	ownCall := m.activeStation.Callsign
	profileID := m.activeStation.ID
	parent := m.bgCtx
	if parent == nil {
		parent = context.Background()
	}
	st := m.store
	return runBgCmd(m.bgTasks, func() tea.Msg {
		ctx, cancel := context.WithTimeout(parent, lotwQueryTimeout)
		defer cancel()
		summary, err := syncLoTWConfirmations(ctx, st, profileID, login, webpass, ownCall)
		return lotwStatsSyncMsg{summary: summary, err: err}
	}, func(r any) tea.Msg {
		return lotwStatsSyncMsg{err: fmt.Errorf("panic during LoTW confirmation sync: %v", r)}
	})
}

// updateStatsPanel drives the stats screen: Up/Down pages between the five
// awards (statsAwardFocus), 's' triggers a manual confirmation sync (mirrors
// the "operator-triggered manual sync" option in
// docs/LoTW_Integration_Design.md — this app has no background scheduler for
// it), Esc/Ctrl+A returns to QSO Entry.
func (m model) updateStatsPanel(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch message := msg.(type) {
	case lotwStatsSyncMsg:
		m.statsSyncing = false
		if message.err != nil {
			m.statsSyncMsg = "sync failed: " + message.err.Error()
			return m, nil
		}
		m.statsSyncMsg = fmt.Sprintf("synced: %d confirmation(s) fetched, %d matched to log", message.summary.Fetched, message.summary.Matched)
		m.refreshLoTWStats()
		return m, nil
	case tea.KeyMsg:
		switch message.String() {
		case "ctrl+c":
			return m, tea.Quit
		case "esc", "ctrl+a":
			m.screen = qsoEntryScreen
			m.focusField(fieldCall)
			return m, nil
		case "up", "k":
			m.statsAwardFocus = (m.statsAwardFocus - 1 + statsAwardCount) % statsAwardCount
			return m, nil
		case "down", "j":
			m.statsAwardFocus = (m.statsAwardFocus + 1) % statsAwardCount
			return m, nil
		case "s":
			if m.statsSyncing {
				return m, nil
			}
			if strings.TrimSpace(m.lotwLogin) == "" || strings.TrimSpace(m.lotwWebPass) == "" {
				m.statsSyncMsg = "LoTW login not configured (set lotw.login/lotw.webpass or W4GNS_LOTW_LOGIN/W4GNS_LOTW_WEBPASS)"
				return m, nil
			}
			cmd := m.lotwStatsSyncCmd()
			if cmd == nil {
				return m, nil
			}
			m.statsSyncing = true
			m.statsSyncMsg = "syncing…"
			return m, cmd
		}
	}
	return m, nil
}

// statsPanelView renders the award/analytics stats screen: a summary line
// per award (worked/confirmed/needed count) and the focused award's
// drill-down need list, so an operator knows what to chase next.
func (m model) statsPanelView() string {
	var b strings.Builder
	b.WriteString(screenHotkeys(m))
	b.WriteString("\n")
	b.WriteString(headerStyle.Render("Award/Analytics Stats — worked vs. LoTW-confirmed"))
	b.WriteString("\n\n")

	if m.lotwStats == nil {
		b.WriteString(helpStyle.Render("no stats available"))
		b.WriteString("\n\n")
		b.WriteString(helpStyle.Render("Esc/Ctrl+A: QSO Entry"))
		return b.String()
	}

	for index := 0; index < statsAwardCount; index++ {
		label, progress, _ := m.statsPanelAward(index)
		line := fmt.Sprintf("%-5s worked %3d  confirmed %3d  needed %3d", label, progress.Worked, progress.Confirmed, len(progress.Needed))
		if index == m.statsAwardFocus {
			b.WriteString(focusedFieldBoxStyle.Render(line))
		} else {
			b.WriteString(line)
		}
		b.WriteString("\n")
	}
	b.WriteString("\n")

	label, progress, _ := m.statsPanelAward(m.statsAwardFocus)
	b.WriteString(statusBarStyle.Render(label + " needed"))
	b.WriteString("\n")
	if len(progress.Needed) == 0 {
		b.WriteString(newMultStyle.Render("none — every worked " + label + " is confirmed"))
		b.WriteString("\n")
	} else {
		const maxShown = 20
		shown := progress.Needed
		truncated := false
		if len(shown) > maxShown {
			shown = shown[:maxShown]
			truncated = true
		}
		b.WriteString(strings.Join(shown, ", "))
		b.WriteString("\n")
		if truncated {
			b.WriteString(helpStyle.Render(fmt.Sprintf("…and %d more", len(progress.Needed)-maxShown)))
			b.WriteString("\n")
		}
	}

	b.WriteString("\n")
	if m.statsSyncMsg != "" {
		b.WriteString(dupeStyle.Render(m.statsSyncMsg))
		b.WriteString("\n\n")
	}
	b.WriteString(helpStyle.Render("Up/Down: page award  •  s: sync LoTW confirmations  •  Esc/Ctrl+A: QSO Entry"))
	return b.String()
}
