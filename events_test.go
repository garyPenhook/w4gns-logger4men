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

// TestScoringRulesEffectiveMultipliers guards the translation between the
// legacy scalar Multiplier field (existing CW Open/CWops configs) and the
// new data-driven Multipliers list (Appendix C): a config using only the old
// field still scores via one Per:"contest" rule, a config using the new list
// takes it verbatim, and nil rules produce no rules to sum.
func TestScoringRulesEffectiveMultipliers(t *testing.T) {
	var nilRules *scoringRules
	if got := nilRules.effectiveMultipliers(); got != nil {
		t.Fatalf("nil rules effectiveMultipliers() = %v, want nil", got)
	}

	legacy := &scoringRules{Multiplier: "unique_call"}
	got := legacy.effectiveMultipliers()
	want := []multiplierRule{{Kind: "unique_call", Per: "contest"}}
	if len(got) != 1 || got[0] != want[0] {
		t.Fatalf("legacy effectiveMultipliers() = %v, want %v", got, want)
	}

	dataDriven := &scoringRules{
		Multiplier:  "unique_call", // should be ignored: Multipliers takes precedence
		Multipliers: []multiplierRule{{Kind: "dxcc", Per: "band"}, {Kind: "cqzone", Per: "band"}},
	}
	got = dataDriven.effectiveMultipliers()
	if len(got) != 2 || got[0].Kind != "dxcc" || got[1].Kind != "cqzone" {
		t.Fatalf("data-driven effectiveMultipliers() = %v, want dxcc+cqzone", got)
	}
}

// TestValidMultiplierKindAndPer guards the config-typo guardrails
// loadEventCatalog relies on when validating scoring.multipliers.
func TestValidMultiplierKindAndPer(t *testing.T) {
	for _, kind := range []string{"unique_call", "dxcc", "cqzone", "ituzone"} {
		if !validMultiplierKind(kind) {
			t.Errorf("validMultiplierKind(%q) = false, want true", kind)
		}
	}
	if validMultiplierKind("areacode") {
		t.Error("validMultiplierKind(\"areacode\") = true, want false (not implemented yet)")
	}
	for _, per := range []string{"band", "contest"} {
		if !validMultiplierPer(per) {
			t.Errorf("validMultiplierPer(%q) = false, want true", per)
		}
	}
	if validMultiplierPer("") || validMultiplierPer("once") {
		t.Error("validMultiplierPer accepted an unsupported scope")
	}
}

// TestReceivedExchangeZoneKindRequiresExplicitConfig guards against deriving
// an exchange rule from descriptive hint prose: asymmetric contests can name
// a DX zone and a domestic state/province in the same hint.
func TestReceivedExchangeZoneKindRequiresExplicitConfig(t *testing.T) {
	cases := []struct {
		event eventDefinition
		want  string
	}{
		{eventDefinition{RcvdExchangeHint: "RST + CQ Zone No."}, ""},
		{eventDefinition{RcvdExchangeHint: "W/VE: RST + state/province DX: RST + CQ Zone"}, ""},
		{eventDefinition{ReceivedExchangeAutofill: "cq_zone"}, "cq_zone"},
		{eventDefinition{ReceivedExchangeAutofill: "itu_zone"}, "itu_zone"},
	}
	for _, tc := range cases {
		if got := tc.event.receivedExchangeZoneKind(); got != tc.want {
			t.Errorf("receivedExchangeZoneKind(%+v) = %q, want %q", tc.event, got, tc.want)
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

func TestEventCapabilityValidationAndCatalogStatus(t *testing.T) {
	valid := []eventDefinition{
		{ID: "SELECT", Capability: catalogCapabilitySelectionOnly},
		{ID: "ENTRY", Capability: catalogCapabilityEntryAware, Bands: []string{"20M"}, RcvdExchangeHint: "RST + serial"},
		{ID: "CAB", Capability: catalogCapabilityCabrilloReady, CabrilloLayout: "cw_rst_exchange"},
		{ID: "SCORE", Capability: catalogCapabilityScoringReady, CabrilloLayout: "cw_rst_exchange", Scoring: &scoringRules{}},
	}
	for _, event := range valid {
		if err := event.validateCapability(); err != nil {
			t.Errorf("validateCapability(%q): %v", event.Capability, err)
		}
	}
	for _, event := range []eventDefinition{
		{ID: "MISSING", Capability: ""},
		{ID: "BAD-ENTRY", Capability: catalogCapabilityEntryAware, Bands: []string{"20M"}, RcvdExchangeHint: "RST", CabrilloLayout: "cw_rst_exchange"},
		{ID: "BAD-CAB", Capability: catalogCapabilityCabrilloReady},
		{ID: "BAD-SCORE", Capability: catalogCapabilityScoringReady, CabrilloLayout: "cw_rst_exchange"},
	} {
		if err := event.validateCapability(); err == nil {
			t.Errorf("validateCapability(%+v) succeeded, want error", event)
		}
	}

	events, err := loadEventCatalog()
	if err != nil {
		t.Fatalf("loadEventCatalog: %v", err)
	}
	for _, tc := range []struct{ id, capability string }{
		{"SD-GENERAL", catalogCapabilitySelectionOnly},
		{"TNQP", catalogCapabilityEntryAware},
		{"CWT", catalogCapabilityCabrilloReady},
		{"CW-OPEN", catalogCapabilityScoringReady},
		{"CQ-WW-CW", catalogCapabilityScoringReady},
		{"CQ-160-CW", catalogCapabilityScoringReady},
	} {
		if got := events[eventIndex(t, events, tc.id)].Capability; got != tc.capability {
			t.Errorf("%s capability = %q, want %q", tc.id, got, tc.capability)
		}
	}
}

// TestLoadEventCatalogCQWWHasRealScoringRules guards the curated CQ-WW-CW
// entry's actual scoring config (roadmap §3 Phase 3 "real per-contest
// wiring"), sourced from cqww.com/rules.htm rather than guessed: 0/1/3
// points for same-country/same-continent/other-continent with a North
// America same-continent exception (2 points), plus DXCC-entity and CQ-zone
// multipliers counted per band.
func TestLoadEventCatalogCQWWHasRealScoringRules(t *testing.T) {
	events, err := loadEventCatalog()
	if err != nil {
		t.Fatalf("loadEventCatalog: %v", err)
	}
	cqww := events[eventIndex(t, events, "CQ-WW-CW")]
	if cqww.Scoring == nil || cqww.Scoring.Points == nil {
		t.Fatal("CQ-WW-CW must have a Points scoring rule")
	}
	points := cqww.Scoring.Points
	if points.SameCountry != 0 || points.SameContinent != 1 || points.OtherContinent != 3 {
		t.Fatalf("CQ-WW-CW points = %+v, want {0,1,3,...}", points)
	}
	if points.SameContinentOverrides["NA"] != 2 {
		t.Fatalf("CQ-WW-CW NA same-continent override = %d, want 2", points.SameContinentOverrides["NA"])
	}
	mults := cqww.Scoring.effectiveMultipliers()
	if len(mults) != 2 {
		t.Fatalf("CQ-WW-CW multiplier count = %d, want 2 (dxcc + cqzone)", len(mults))
	}
	for _, kind := range []string{"dxcc", "cqzone"} {
		found := false
		for _, rule := range mults {
			if rule.Kind == kind && rule.Per == "band" {
				found = true
			}
		}
		if !found {
			t.Fatalf("CQ-WW-CW missing %q multiplier per band", kind)
		}
	}
}

func TestLoadEventCatalogCarriesCheckedADIFContestIDs(t *testing.T) {
	events, err := loadEventCatalog()
	if err != nil {
		t.Fatal(err)
	}
	for id, want := range map[string]string{
		"CWT":       "CWOPS-CWT",
		"CW-OPEN":   "CWOPS-CW-OPEN",
		"CQ-WW-CW":  "CQ-WW-CW",
		"CQ-160-CW": "CQ-160-CW",
	} {
		if got := events[eventIndex(t, events, id)].ADIFContestID; got != want {
			t.Errorf("event %q adif_contest_id = %q, want %q", id, got, want)
		}
	}
}

func TestValidADIFContestID(t *testing.T) {
	for _, id := range []string{"", "CQ-WW-CW", "CWOPS-CWT", "7QP"} {
		if !validADIFContestID(id) {
			t.Errorf("validADIFContestID(%q) = false, want true", id)
		}
	}
	for _, id := range []string{"cq-ww-cw", "CQ WW CW", "CQ_WW_CW", "CQ/WW"} {
		if validADIFContestID(id) {
			t.Errorf("validADIFContestID(%q) = true, want false", id)
		}
	}
}

// TestLoadEventCatalogCQ160HasRealScoringRules guards the curated
// CQ-160-CW entry's actual scoring config (roadmap §3 Phase 3 "real
// per-contest wiring"), sourced from cq160.com/rules/index.htm rather than
// guessed: 2/5/10 points for same-country/same-continent/other-continent,
// plus a DXCC-entity multiplier counted once per contest. The rules also
// award US-state and Canadian-province multipliers to DX stations, which
// the schema can't express yet (states/provinces come from the received
// exchange text, not the worked callsign) — deliberately left out rather
// than guessed at.
func TestLoadEventCatalogCQ160HasRealScoringRules(t *testing.T) {
	events, err := loadEventCatalog()
	if err != nil {
		t.Fatalf("loadEventCatalog: %v", err)
	}
	cq160 := events[eventIndex(t, events, "CQ-160-CW")]
	if cq160.Scoring == nil || cq160.Scoring.Points == nil {
		t.Fatal("CQ-160-CW must have a Points scoring rule")
	}
	points := cq160.Scoring.Points
	if points.SameCountry != 2 || points.SameContinent != 5 || points.OtherContinent != 10 {
		t.Fatalf("CQ-160-CW points = %+v, want {2,5,10}", points)
	}
	mults := cq160.Scoring.effectiveMultipliers()
	if len(mults) != 1 || mults[0].Kind != "dxcc" || mults[0].Per != "contest" {
		t.Fatalf("CQ-160-CW multipliers = %+v, want [{dxcc contest}]", mults)
	}
	if cq160.receivedExchangeZoneKind() != "cq_zone" {
		t.Fatalf("CQ-160-CW received_exchange_autofill = %q, want cq_zone", cq160.receivedExchangeZoneKind())
	}
	if !cq160.receivedExchangeAutofillExcluded("United States") || !cq160.receivedExchangeAutofillExcluded("Canada") {
		t.Fatal("CQ-160-CW must exclude United States and Canada from zone autofill (they send state/province, not a zone)")
	}
	if cq160.receivedExchangeAutofillExcluded("England") {
		t.Fatal("CQ-160-CW must not exclude a DX entity from zone autofill")
	}
}

// TestReceivedExchangeAutofillExcludedIsCaseInsensitiveAndSafeOnBlank guards
// the helper autofillReceivedExchange calls on every keystroke: matching
// must not depend on cty.dat's exact letter casing, and an unresolved
// worked-station country (blank) must never match an exclusion list.
func TestReceivedExchangeAutofillExcludedIsCaseInsensitiveAndSafeOnBlank(t *testing.T) {
	event := eventDefinition{ReceivedExchangeAutofillDomestic: []string{"United States", "Canada"}}
	if !event.receivedExchangeAutofillExcluded("united states") {
		t.Error("expected case-insensitive match for 'united states'")
	}
	if event.receivedExchangeAutofillExcluded("") {
		t.Error("blank country must never be excluded")
	}
	if event.receivedExchangeAutofillExcluded("Germany") {
		t.Error("Germany is not on the exclusion list")
	}
}

// TestLoadEventCatalogARRLDXCWHasRealScoringRules guards the curated
// ARRL-DX-CW entry's actual scoring config (roadmap §3 Phase 3 "real
// per-contest wiring"), sourced from
// contests.arrl.org/ContestRules/DX-Rules.pdf rather than guessed: a flat 3
// points per QSO (§5.1) and a DXCC-entity multiplier counted once per band
// (§5.2.1/§5.2.2). The rules are asymmetric — DX entrants count US
// states/DC/Canadian provinces (§5.2.3) as their multiplier instead of DXCC
// entities — but that side needs an exchange-derived multiplier kind the
// schema doesn't have yet (states and provinces come from the received
// exchange text, not the worked callsign), the same gap CQ-160-CW's DX-side
// multiplier left open. This config is therefore only correct for a W/VE-side
// entrant, which matches this app's station profile.
func TestLoadEventCatalogARRLDXCWHasRealScoringRules(t *testing.T) {
	events, err := loadEventCatalog()
	if err != nil {
		t.Fatalf("loadEventCatalog: %v", err)
	}
	arrl := events[eventIndex(t, events, "ARRL-DX-CW")]
	if arrl.Scoring == nil {
		t.Fatal("ARRL-DX-CW must have a Scoring rule")
	}
	if arrl.Scoring.PointsPerQSO != 3 {
		t.Fatalf("ARRL-DX-CW points_per_qso = %d, want 3", arrl.Scoring.PointsPerQSO)
	}
	mults := arrl.Scoring.effectiveMultipliers()
	if len(mults) != 1 || mults[0].Kind != "dxcc" || mults[0].Per != "band" {
		t.Fatalf("ARRL-DX-CW multipliers = %+v, want [{dxcc band}]", mults)
	}
	if arrl.ADIFContestID != "ARRL-DX-CW" {
		t.Fatalf("ARRL-DX-CW adif_contest_id = %q, want ARRL-DX-CW", arrl.ADIFContestID)
	}
	if !arrl.cabrilloReady() {
		t.Fatal("ARRL-DX-CW must have a checked Cabrillo layout")
	}
}

// TestLoadEventCatalogCQWPXHasRealScoringRules guards the curated CQ-WPX-CW
// entry's actual scoring config, sourced from cqwpx.com/rules.htm rather than
// guessed: band-tiered QSO points (1/1/3 same-country/same-continent/other-
// continent on 10-20M, doubled to 1/2/6 on 40/80/160M, with a North America
// same-continent exception of 2/4) and a "prefix" multiplier counted once per
// contest (Rule V.C: "Each PREFIX is counted only once regardless of the band
// ... it is worked").
func TestLoadEventCatalogCQWPXHasRealScoringRules(t *testing.T) {
	events, err := loadEventCatalog()
	if err != nil {
		t.Fatalf("loadEventCatalog: %v", err)
	}
	wpx := events[eventIndex(t, events, "CQ-WPX-CW")]
	if wpx.Scoring == nil {
		t.Fatal("CQ-WPX-CW must have a Scoring rule")
	}
	points := wpx.Scoring.Points
	if points == nil {
		t.Fatal("CQ-WPX-CW must use the Points rule (band-tiered), not a flat PointsPerQSO")
	}
	if points.SameCountry != 1 || points.SameContinent != 1 || points.OtherContinent != 3 {
		t.Fatalf("CQ-WPX-CW high-band points = %+v, want same_country=1 same_continent=1 other_continent=3", points)
	}
	if points.LowBandSameContinent != 2 || points.LowBandOtherContinent != 6 {
		t.Fatalf("CQ-WPX-CW low-band points = %+v, want low_band_same_continent=2 low_band_other_continent=6", points)
	}
	if points.SameContinentOverrides["NA"] != 2 || points.LowBandSameContinentOverrides["NA"] != 4 {
		t.Fatalf("CQ-WPX-CW NA overrides = high:%d low:%d, want high:2 low:4", points.SameContinentOverrides["NA"], points.LowBandSameContinentOverrides["NA"])
	}
	mults := wpx.Scoring.effectiveMultipliers()
	if len(mults) != 1 || mults[0].Kind != "prefix" || mults[0].Per != "contest" {
		t.Fatalf("CQ-WPX-CW multipliers = %+v, want [{prefix contest}]", mults)
	}
	if wpx.ADIFContestID != "CQ-WPX-CW" {
		t.Fatalf("CQ-WPX-CW adif_contest_id = %q, want CQ-WPX-CW", wpx.ADIFContestID)
	}
	if !wpx.cabrilloReady() {
		t.Fatal("CQ-WPX-CW must have a checked Cabrillo layout")
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

// TestLoadEventCatalogPrefersCuratedSSTOverGeneratedDuplicate mirrors
// TestLoadEventCatalogPrefersCuratedOverGeneratedDuplicate for K1USN-SST
// (events/k1usn.json): the SD-generated SD-SST entry collapses both weekly
// slots into one synthetic "ALL" session, while the curated entry carries the
// real Friday/Monday session schedule, so the curated copy must win.
func TestLoadEventCatalogPrefersCuratedSSTOverGeneratedDuplicate(t *testing.T) {
	events, err := loadEventCatalog()
	if err != nil {
		t.Fatalf("loadEventCatalog: %v", err)
	}
	for _, event := range events {
		if event.ID == "SD-SST" {
			t.Fatal("SD-generated SST duplicate should be dropped in favor of the curated K1USN-SST event")
		}
	}
	sst := events[eventIndex(t, events, "K1USN-SST")]
	if len(sst.Sessions) != 2 {
		t.Fatalf("K1USN-SST session count = %d, want 2", len(sst.Sessions))
	}
	if sst.DupeScope != "call+band+session" {
		t.Fatalf("K1USN-SST dupe_scope = %q, want call+band+session", sst.DupeScope)
	}
}

// TestLoadEventCatalogPrefersCuratedCWTOverGeneratedDuplicate mirrors
// TestLoadEventCatalogPrefersCuratedOverGeneratedDuplicate for CWT
// (events/cwops.json): the SD-generated SD-CWOPS entry ("CWOPS Mini-CWT",
// cabrillo_contest "CW-OPS") collapses the four weekly slots into one
// synthetic "ALL" session, while the curated CWT entry carries the real
// Wed/Thu session schedule and now shares the same "CW-OPS" cabrillo_contest
// token (the real-world Cabrillo tag for this contest), so the curated copy
// must win.
func TestLoadEventCatalogPrefersCuratedCWTOverGeneratedDuplicate(t *testing.T) {
	events, err := loadEventCatalog()
	if err != nil {
		t.Fatalf("loadEventCatalog: %v", err)
	}
	for _, event := range events {
		if event.ID == "SD-CWOPS" {
			t.Fatal("SD-generated CWOPS Mini-CWT duplicate should be dropped in favor of the curated CWT event")
		}
	}
	cwt := events[eventIndex(t, events, "CWT")]
	if len(cwt.Sessions) != 4 {
		t.Fatalf("CWT session count = %d, want 4", len(cwt.Sessions))
	}
	if cwt.DupeScope != "call+band+session" {
		t.Fatalf("CWT dupe_scope = %q, want call+band+session", cwt.DupeScope)
	}
}

func TestEventCatalogCyclesSSTSessions(t *testing.T) {
	st, err := openStore(filepath.Join(t.TempDir(), "logger.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	m := initialModel(st)
	m.openEventCatalog()
	m.eventFocus = eventIndex(t, m.events, "K1USN-SST")
	updated, _ := m.updateEventCatalog(tea.KeyMsg{Type: tea.KeyRight})
	m = updated.(model)
	if m.eventSessionFocus != 1 {
		t.Fatalf("K1USN-SST session focus = %d, want 1", m.eventSessionFocus)
	}
	updated, _ = m.updateEventCatalog(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(model)
	if m.contestFields[contestName].Value() != "K1USN-SST-MON" {
		t.Fatalf("contest ID = %q", m.contestFields[contestName].Value())
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
