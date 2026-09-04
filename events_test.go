package main

import (
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// eventIndex finds an event by ID rather than assuming a fixed catalog
// position, since the alphabetical sort order shifts as events/*.json grows.
func eventIndex(t *testing.T, events []eventDefinition, id string) int {
	t.Helper()
	for i, event := range events {
		if event.ID == id {
			return i
		}
	}
	t.Fatalf("event %q not found in catalog", id)
	return -1
}

// TestReceivedExchangeZoneKindInfersFromHintText guards the heuristic
// autofillReceivedExchange (main.go) relies on: it must read "cq_zone" or
// "itu_zone" out of a catalog entry's free-text hint without any per-event
// JSON tagging, so every existing and future zone-exchange contest gets
// autofill for free, and it must not false-positive on unrelated hints.
func TestReceivedExchangeZoneKindInfersFromHintText(t *testing.T) {
	cases := []struct {
		hint string
		want string
	}{
		{"RST + CQ Zone No.", "cq_zone"},
		{"Your ITU Zone", "itu_zone"},
		{"CWSP Members: RST + \"M\" All Others: RST + ITU Zone No.", "itu_zone"},
		{"599 + Name + State", ""},
		{"", ""},
	}
	for _, tc := range cases {
		event := eventDefinition{RcvdExchangeHint: tc.hint}
		if got := event.receivedExchangeZoneKind(); got != tc.want {
			t.Errorf("receivedExchangeZoneKind(%q) = %q, want %q", tc.hint, got, tc.want)
		}
	}
}

func TestLoadEventCatalogIncludesCWopsDefinitions(t *testing.T) {
	events, err := loadEventCatalog()
	if err != nil {
		t.Fatalf("loadEventCatalog: %v", err)
	}
	if len(events) < 41 {
		t.Fatalf("event count = %d, want at least 41", len(events))
	}
	seen := map[string]bool{}
	for _, event := range events {
		if seen[event.ID] {
			t.Fatalf("duplicate event id %q", event.ID)
		}
		seen[event.ID] = true
	}
	for _, id := range []string{"CW-OPEN", "CWT", "TNQP"} {
		if !seen[id] {
			t.Fatalf("expected event id %q in catalog", id)
		}
	}
	tnqp := events[eventIndex(t, events, "TNQP")]
	if got := len(tnqp.ReceivedExchangeOptions); got != 95 {
		t.Fatalf("TN county count = %d, want 95", got)
	}
}

// TestSDContestCatalogLoadsWithDistinctSideVariants guards the imported SD
// template catalog: the many contests load alongside the curated events with
// unique IDs, and side-variant entries that submit under one Cabrillo contest
// name (e.g. ARRL DX CW "home" and "DX" sides) keep distinct IDs while sharing
// a cabrillo_contest token — the reason eventDefinition separates the two.
func TestSDContestCatalogLoadsWithDistinctSideVariants(t *testing.T) {
	events, err := loadEventCatalog()
	if err != nil {
		t.Fatalf("loadEventCatalog: %v", err)
	}
	if len(events) < 250 {
		t.Fatalf("event count = %d, want at least 250 after importing SD templates", len(events))
	}
	dx := events[eventIndex(t, events, "SD-ARDXDXC")]
	home := events[eventIndex(t, events, "SD-ARDXWVC")]
	if dx.ID == home.ID {
		t.Fatal("side variants must have distinct IDs")
	}
	if dx.CabrilloContest != "ARRL-DX-CW" || home.CabrilloContest != "ARRL-DX-CW" {
		t.Fatalf("both ARRL DX CW sides must map to CONTEST ARRL-DX-CW, got %q / %q",
			dx.CabrilloContest, home.CabrilloContest)
	}
}

// TestLoadEventCatalogPrefersCuratedOverGeneratedDuplicate guards the
// curated-vs-generated de-dup (docs/ROADMAP.md "curated vs generated
// duplicates"): CW-OPEN is both a hand-curated event (events/cwops.json) and
// an SD-generated one (events/sd_contests.json, cabrillo_contest "CW-OPEN")
// with no side-variant split — a straight 1:1 duplicate. The generator can't
// know what this app already curates, so the loader is the one place that
// can catch it; the curated copy must win and the generated one must not
// appear in the catalog at all.
func TestLoadEventCatalogPrefersCuratedOverGeneratedDuplicate(t *testing.T) {
	events, err := loadEventCatalog()
	if err != nil {
		t.Fatalf("loadEventCatalog: %v", err)
	}
	for _, event := range events {
		if event.ID == "SD-CWOPEN" {
			t.Fatal("SD-generated CW-OPEN duplicate should be dropped in favor of the curated CW-OPEN event")
		}
	}
	cwOpen := events[eventIndex(t, events, "CW-OPEN")]
	if !cwOpen.CabrilloOmitRST {
		t.Fatal("the surviving CW-OPEN event should be the curated one (cabrillo_omit_rst set)")
	}
}

// TestLoadEventCatalogKeepsGeneratedSideVariantsDespiteCuratedOverlap guards
// against the de-dup being too aggressive: ARRL DX CW has one generic curated
// entry (events/contestcalendar.json, ID "ARRL-DX-CW") but the SD catalog
// splits it into "home"/"DX" sides with distinct exchanges sharing that same
// cabrillo_contest token. That's added fidelity, not a duplicate, so both
// generated sides must survive alongside the curated entry.
func TestLoadEventCatalogKeepsGeneratedSideVariantsDespiteCuratedOverlap(t *testing.T) {
	events, err := loadEventCatalog()
	if err != nil {
		t.Fatalf("loadEventCatalog: %v", err)
	}
	for _, id := range []string{"ARRL-DX-CW", "SD-ARDXDXC", "SD-ARDXWVC"} {
		eventIndex(t, events, id) // fatals if missing
	}
}

// TestCabrilloHeaderUsesCabrilloContestOverride guards that the Cabrillo
// CONTEST: line follows cabrillo_contest when set (so an SD side-variant with
// ID "SD-ARDXDXC" still submits under "ARRL-DX-CW"), and falls back to ID when
// the override is blank (every curated event's existing behavior).
func TestCabrilloHeaderUsesCabrilloContestOverride(t *testing.T) {
	withOverride := eventDefinition{ID: "SD-ARDXDXC", CabrilloContest: "ARRL-DX-CW", Bands: []string{"20M"}}
	lines := cabrilloHeaderLines(testStationProfile(), withOverride, 0)
	if !strings.Contains(strings.Join(lines, "\n"), "CONTEST: ARRL-DX-CW") {
		t.Fatalf("header should use cabrillo_contest override, got:\n%s", strings.Join(lines, "\n"))
	}
	noOverride := eventDefinition{ID: "CW-OPEN", Bands: []string{"20M"}}
	lines = cabrilloHeaderLines(testStationProfile(), noOverride, 0)
	if !strings.Contains(strings.Join(lines, "\n"), "CONTEST: CW-OPEN") {
		t.Fatalf("header should fall back to ID when override blank, got:\n%s", strings.Join(lines, "\n"))
	}
}

// TestEventSelectionIDsFitContestField guards against silently truncating
// "event.ID-session.ID" (see selectEvent) in the Contest Name field: every
// catalog entry's longest generated value must fit maxEventSelectionLength.
func TestEventSelectionIDsFitContestField(t *testing.T) {
	events, err := loadEventCatalog()
	if err != nil {
		t.Fatalf("loadEventCatalog: %v", err)
	}
	for _, event := range events {
		for _, session := range event.Sessions {
			value := event.ID + "-" + session.ID
			if len(value) > maxEventSelectionLength {
				t.Errorf("event %q session %q generates %q (%d chars), exceeds maxEventSelectionLength (%d)",
					event.ID, session.ID, value, len(value), maxEventSelectionLength)
			}
		}
	}
}

// TestLoadEventCatalogHasNoLeftoverScraperArtifacts guards against the
// " and / " glue text a prior scrape left in several multi-session
// schedules (e.g. "0600Z-0629Z, Sep 5 and / 0630Z-0659Z, Sep 5").
func TestLoadEventCatalogHasNoLeftoverScraperArtifacts(t *testing.T) {
	events, err := loadEventCatalog()
	if err != nil {
		t.Fatalf("loadEventCatalog: %v", err)
	}
	for _, event := range events {
		if strings.Contains(event.Schedule, "and / ") {
			t.Errorf("event %q schedule has a leftover scraper artifact: %q", event.ID, event.Schedule)
		}
		for _, session := range event.Sessions {
			if strings.Contains(session.Schedule, "and / ") {
				t.Errorf("event %q session %q schedule has a leftover scraper artifact: %q", event.ID, session.ID, session.Schedule)
			}
		}
	}
}

func TestTNQPCountyTypeAheadInsertsOfficialCode(t *testing.T) {
	st, err := openStore(filepath.Join(t.TempDir(), "logger.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	m := initialModel(st)
	tnqp := m.events[eventIndex(t, m.events, "TNQP")]
	m.selectEvent(tnqp, tnqp.Sessions[0])
	m.contestFocusIdx = contestExchangeRcvd
	focusTextFields(m.contestFields, m.contestFocusIdx)
	m.contestFields[contestExchangeRcvd].SetValue("shel")
	updated, _ := m.updateQSOContest(tea.KeyMsg{Type: tea.KeyDown})
	m = updated.(model)
	updated, _ = m.updateQSOContest(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(model)
	if got := m.contestFields[contestExchangeRcvd].Value(); got != "SHEL" {
		t.Fatalf("county choice = %q, want SHEL", got)
	}
}

func TestEventCatalogSelectsCWOpenDefaults(t *testing.T) {
	st, err := openStore(filepath.Join(t.TempDir(), "logger.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	m := initialModel(st)
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyF7})
	m = updated.(model)
	if m.screen != eventCatalogScreen {
		t.Fatalf("F7 screen = %v, want event catalog", m.screen)
	}
	m.eventFocus = eventIndex(t, m.events, "CW-OPEN")
	updated, _ = m.updateEventCatalog(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(model)
	if m.screen != qsoContestScreen || m.contestFields[contestName].Value() != "CW-OPEN-1" || m.contestFields[contestSerialSent].Value() != "001" {
		t.Fatalf("selected event state = %#v", m)
	}
}

func TestEventCatalogCyclesCWTSessions(t *testing.T) {
	st, err := openStore(filepath.Join(t.TempDir(), "logger.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	m := initialModel(st)
	m.openEventCatalog()
	m.eventFocus = eventIndex(t, m.events, "CWT")
	updated, _ := m.updateEventCatalog(tea.KeyMsg{Type: tea.KeyRight})
	m = updated.(model)
	if m.eventSessionFocus != 1 {
		t.Fatalf("CWT session focus = %d, want 1", m.eventSessionFocus)
	}
	updated, _ = m.updateEventCatalog(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(model)
	if m.contestFields[contestName].Value() != "CWT-1900" {
		t.Fatalf("contest ID = %q", m.contestFields[contestName].Value())
	}
}
