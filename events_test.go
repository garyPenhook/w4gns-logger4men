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

// TestEventDefinitionEffectiveScoring guards the side-selection an
// asymmetric-contest event (DXScoring + DomesticCountries set) needs: a
// domestic-country station uses Scoring, a non-domestic one uses DXScoring
// (case-insensitively), an unresolved station conservatively falls back to
// Scoring, and an event with no DXScoring configured always uses Scoring
// regardless of country — the shape every event predating this field has.
func TestEventDefinitionEffectiveScoring(t *testing.T) {
	weRules := &scoringRules{PointsPerQSO: 3}
	dxRules := &scoringRules{PointsPerQSO: 1}
	asymmetric := eventDefinition{
		Scoring:           weRules,
		DXScoring:         dxRules,
		DomesticCountries: []string{"United States", "Canada"},
	}
	if got := asymmetric.effectiveScoring("United States"); got != weRules {
		t.Error("a domestic country must use Scoring")
	}
	if got := asymmetric.effectiveScoring("canada"); got != weRules {
		t.Error("a domestic country match must be case-insensitive")
	}
	if got := asymmetric.effectiveScoring("Germany"); got != dxRules {
		t.Error("a non-domestic country must use DXScoring")
	}
	if got := asymmetric.effectiveScoring(""); got != weRules {
		t.Error("an unresolved station must fall back to Scoring, not guess DXScoring")
	}

	symmetric := eventDefinition{Scoring: weRules}
	if got := symmetric.effectiveScoring("Germany"); got != weRules {
		t.Error("an event with no DXScoring must always use Scoring")
	}
}

// TestValidateScoringRules guards the loader's per-scoring-block checks
// (shared by an event's Scoring and DXScoring via the label parameter): nil
// is valid (no rules configured for that side), and each of the checks the
// loader used to run inline still fires with the field it's reporting named
// in the error.
func TestValidateScoringRules(t *testing.T) {
	if err := validateScoringRules("EVT", "scoring", nil); err != nil {
		t.Errorf("nil rules must be valid: %v", err)
	}
	cases := []struct {
		name  string
		rules *scoringRules
	}{
		{"negative points_per_qso", &scoringRules{PointsPerQSO: -1}},
		{"unsupported multiplier kind", &scoringRules{Multipliers: []multiplierRule{{Kind: "bogus", Per: "band"}}}},
		{"unsupported multiplier per", &scoringRules{Multipliers: []multiplierRule{{Kind: "dxcc", Per: "bogus"}}}},
		{"unsupported legacy multiplier", &scoringRules{Multiplier: "bogus"}},
		{"negative points value", &scoringRules{Points: &pointsRule{SameCountry: -1}}},
		{"unsupported same_continent_overrides continent", &scoringRules{Points: &pointsRule{SameContinentOverrides: map[string]int{"ZZ": 1}}}},
		{"negative same_continent_overrides value", &scoringRules{Points: &pointsRule{SameContinentOverrides: map[string]int{"NA": -1}}}},
	}
	for _, tc := range cases {
		if err := validateScoringRules("EVT", "dx_scoring", tc.rules); err == nil {
			t.Errorf("%s: want error, got nil", tc.name)
		} else if !strings.Contains(err.Error(), "dx_scoring") {
			t.Errorf("%s: error %q doesn't name the dx_scoring field", tc.name, err)
		}
	}
	if err := validateScoringRules("EVT", "scoring", &scoringRules{PointsPerQSO: 1, Multiplier: "unique_call"}); err != nil {
		t.Errorf("a valid rules block must not error: %v", err)
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
		{"TNQP", catalogCapabilityScoringReady},
		{"CWT", catalogCapabilityScoringReady},
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
// plus a DXCC-entity multiplier and a US-state/DC/Canadian-province
// ("exchange_area") multiplier, each counted once per contest — cq160.com's
// rules award both uniformly to every entrant (not just DX stations), unlike
// ARRL DX CW's side-asymmetric state/province multiplier (still unwired: see
// exchange_area.go).
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
	want := []multiplierRule{{Kind: "dxcc", Per: "contest"}, {Kind: "exchange_area", Per: "contest"}}
	if len(mults) != len(want) || mults[0] != want[0] || mults[1] != want[1] {
		t.Fatalf("CQ-160-CW multipliers = %+v, want %+v", mults, want)
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

// TestLoadEventCatalogTNQPHasRealScoringRules guards the curated TNQP
// entry's actual scoring config, sourced from tnqp.org/rules/: a flat 3
// points per QSO (mode-independent; this app logs CW only so the mode split
// the rules describe doesn't apply) and a "tn_county" multiplier counted
// once per band ("95 maximum per band"). The rules also describe a
// state/province/DXCC multiplier set and a K4TCG working bonus, but both
// only apply to a Tennessee-resident entrant — this catalog entry is
// configured for an out-of-state operator (README.md), for whom TN counties
// are the only multiplier category, so neither is wired here.
func TestLoadEventCatalogTNQPHasRealScoringRules(t *testing.T) {
	events, err := loadEventCatalog()
	if err != nil {
		t.Fatalf("loadEventCatalog: %v", err)
	}
	tnqp := events[eventIndex(t, events, "TNQP")]
	if tnqp.Capability != catalogCapabilityScoringReady {
		t.Fatalf("TNQP capability = %q, want %q", tnqp.Capability, catalogCapabilityScoringReady)
	}
	if tnqp.Scoring == nil || tnqp.Scoring.PointsPerQSO != 3 {
		t.Fatalf("TNQP scoring = %+v, want PointsPerQSO 3", tnqp.Scoring)
	}
	mults := tnqp.Scoring.effectiveMultipliers()
	want := []multiplierRule{{Kind: "tn_county", Per: "band"}}
	if len(mults) != len(want) || mults[0] != want[0] {
		t.Fatalf("TNQP multipliers = %+v, want %+v", mults, want)
	}
	if tnqp.ADIFContestID != "TN-QSO-PARTY" {
		t.Fatalf("TNQP adif_contest_id = %q, want TN-QSO-PARTY", tnqp.ADIFContestID)
	}
	if tnqp.CabrilloLayout != "cw_rst_exchange" {
		t.Fatalf("TNQP cabrillo_layout = %q, want cw_rst_exchange", tnqp.CabrilloLayout)
	}
}

// TestLoadEventCatalogNAQPCWHasRealScoringRules guards the curated NAQP-CW
// entry's actual scoring config, sourced from ncjweb.com/NAQP-Rules.pdf:
// Rule 13 ("Multiply total valid contacts by the sum of the number of
// multipliers worked on each band") is a flat 1 point per QSO — not a
// distance/tier scale — times a Rule 11 "naqp_area" multiplier (US
// states/DC/Canadian provinces, plus other North American DXCC entities)
// counted again on every band. No RST is exchanged (Rule 10: name and
// location only), matching CW Open's cw_exchange_only/cabrillo_omit_rst
// shape.
func TestLoadEventCatalogNAQPCWHasRealScoringRules(t *testing.T) {
	events, err := loadEventCatalog()
	if err != nil {
		t.Fatalf("loadEventCatalog: %v", err)
	}
	naqp := events[eventIndex(t, events, "NAQP-CW")]
	if naqp.Capability != catalogCapabilityScoringReady {
		t.Fatalf("NAQP-CW capability = %q, want %q", naqp.Capability, catalogCapabilityScoringReady)
	}
	if naqp.Scoring == nil || naqp.Scoring.PointsPerQSO != 1 {
		t.Fatalf("NAQP-CW scoring = %+v, want PointsPerQSO 1", naqp.Scoring)
	}
	mults := naqp.Scoring.effectiveMultipliers()
	want := []multiplierRule{{Kind: "naqp_area", Per: "band"}}
	if len(mults) != len(want) || mults[0] != want[0] {
		t.Fatalf("NAQP-CW multipliers = %+v, want %+v", mults, want)
	}
	if naqp.ADIFContestID != "NAQP-CW" {
		t.Fatalf("NAQP-CW adif_contest_id = %q, want NAQP-CW", naqp.ADIFContestID)
	}
	if naqp.CabrilloLayout != "cw_exchange_only" {
		t.Fatalf("NAQP-CW cabrillo_layout = %q, want cw_exchange_only", naqp.CabrilloLayout)
	}
	if !naqp.CabrilloOmitRST {
		t.Fatalf("NAQP-CW cabrillo_omit_rst = false, want true")
	}
}

// TestLoadEventCatalogARRLSSCWHasRealScoringRules guards the curated
// ARRL-SS-CW entry's actual scoring config, sourced from
// contests.arrl.org/ContestRules/SS-Rules.pdf: Rule 5.1 is a flat 2 points
// per QSO (no continent/country tiering, unlike CQ WW/CQ 160/ARRL DX/WPX)
// times a Rule 5.2/5.3 "arrl_section" multiplier — every ARRL/RAC section
// worked, counted once for the whole contest rather than per band. Rule 2.2
// ("Each station may be contacted only once, regardless of band") makes this
// the one configured event whose dupe_scope drops the band scope entirely.
// No RST is exchanged (the exchange is serial/precedence/call/check/section),
// matching CW Open/NAQP-CW's cw_exchange_only/cabrillo_omit_rst shape.
func TestLoadEventCatalogARRLSSCWHasRealScoringRules(t *testing.T) {
	events, err := loadEventCatalog()
	if err != nil {
		t.Fatalf("loadEventCatalog: %v", err)
	}
	ss := events[eventIndex(t, events, "ARRL-SS-CW")]
	if ss.Capability != catalogCapabilityScoringReady {
		t.Fatalf("ARRL-SS-CW capability = %q, want %q", ss.Capability, catalogCapabilityScoringReady)
	}
	if ss.Scoring == nil || ss.Scoring.PointsPerQSO != 2 {
		t.Fatalf("ARRL-SS-CW scoring = %+v, want PointsPerQSO 2", ss.Scoring)
	}
	mults := ss.Scoring.effectiveMultipliers()
	want := []multiplierRule{{Kind: "arrl_section", Per: "contest"}}
	if len(mults) != len(want) || mults[0] != want[0] {
		t.Fatalf("ARRL-SS-CW multipliers = %+v, want %+v", mults, want)
	}
	if ss.ADIFContestID != "ARRL-SS-CW" {
		t.Fatalf("ARRL-SS-CW adif_contest_id = %q, want ARRL-SS-CW", ss.ADIFContestID)
	}
	if ss.CabrilloLayout != "cw_exchange_only" {
		t.Fatalf("ARRL-SS-CW cabrillo_layout = %q, want cw_exchange_only", ss.CabrilloLayout)
	}
	if !ss.CabrilloOmitRST {
		t.Fatalf("ARRL-SS-CW cabrillo_omit_rst = false, want true")
	}
	if ss.DupeScope != "call" {
		t.Fatalf("ARRL-SS-CW dupe_scope = %q, want call", ss.DupeScope)
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
// points per QSO on both sides (§5.1), a DXCC-entity multiplier counted once
// per band for a W/VE-side entrant (§5.2.1), and an exchange_area multiplier
// counted once per band for a DX-side entrant (§5.2.2) — the schema's
// side-asymmetric scoring (Scoring/DXScoring/DomesticCountries,
// effectiveScoring), replacing the W/VE-only limitation the original version
// of this config had.
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
	if arrl.DXScoring == nil {
		t.Fatal("ARRL-DX-CW must have a DXScoring rule for DX-side entrants")
	}
	if arrl.DXScoring.PointsPerQSO != 3 {
		t.Fatalf("ARRL-DX-CW dx_scoring.points_per_qso = %d, want 3", arrl.DXScoring.PointsPerQSO)
	}
	dxMults := arrl.DXScoring.effectiveMultipliers()
	if len(dxMults) != 1 || dxMults[0].Kind != "exchange_area" || dxMults[0].Per != "band" {
		t.Fatalf("ARRL-DX-CW dx_scoring multipliers = %+v, want [{exchange_area band}]", dxMults)
	}
	if !countryInList(arrl.DomesticCountries, "United States") || !countryInList(arrl.DomesticCountries, "Canada") {
		t.Fatalf("ARRL-DX-CW domestic_countries = %v, want United States and Canada", arrl.DomesticCountries)
	}
	if got := arrl.effectiveScoring("United States"); got != arrl.Scoring {
		t.Fatal("a W/VE-side station (United States) must use Scoring")
	}
	if got := arrl.effectiveScoring("Canada"); got != arrl.Scoring {
		t.Fatal("a W/VE-side station (Canada) must use Scoring")
	}
	if got := arrl.effectiveScoring("Germany"); got != arrl.DXScoring {
		t.Fatal("a DX-side station (Germany) must use DXScoring")
	}
	if got := arrl.effectiveScoring(""); got != arrl.Scoring {
		t.Fatal("an unresolved station must conservatively fall back to Scoring")
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

// TestLoadEventCatalogCWTHasRealScoringRules guards the curated CWT entry's
// actual scoring config, sourced from cwops.org/cwops-tests/ rather than
// guessed: 1 point per QSO, multiplied by the count of unique callsigns
// worked ("If you had 75 QSOs with 40 different callsigns, your score is
// 75 x 40 = 3000 points") — the same points/multiplier shape as CW Open,
// scoped per session by CWT's existing call+band+session dupe_scope.
func TestLoadEventCatalogCWTHasRealScoringRules(t *testing.T) {
	events, err := loadEventCatalog()
	if err != nil {
		t.Fatalf("loadEventCatalog: %v", err)
	}
	cwt := events[eventIndex(t, events, "CWT")]
	if cwt.Scoring == nil {
		t.Fatal("CWT must have a Scoring rule")
	}
	if cwt.Scoring.PointsPerQSO != 1 {
		t.Fatalf("CWT points_per_qso = %d, want 1", cwt.Scoring.PointsPerQSO)
	}
	mults := cwt.Scoring.effectiveMultipliers()
	if len(mults) != 1 || mults[0].Kind != "unique_call" {
		t.Fatalf("CWT multipliers = %+v, want [{unique_call ...}]", mults)
	}
	if cwt.ADIFContestID != "CWOPS-CWT" {
		t.Fatalf("CWT adif_contest_id = %q, want CWOPS-CWT", cwt.ADIFContestID)
	}
	if !cwt.cabrilloReady() {
		t.Fatal("CWT must have a checked Cabrillo layout")
	}
}

// TestLoadEventCatalogSACCWHasRealScoringRules guards the curated SAC-CW
// entry's actual scoring config, sourced from
// sactest.net/blog/scandinavian-activity-contest-2025-rules/ Sections 7-8
// rather than guessed. SAC is side-asymmetric around a fixed "Scandinavian"
// country group (Norway, Finland, Sweden, Iceland, Denmark and their listed
// territories) rather than the operator's own continent: a Scandinavian
// entrant (Scoring) scores 2 points for a European-non-Scandinavian QSO and
// 3 for a non-European QSO (a Scandinavian-Scandinavian QSO the rules don't
// address scores 0 via the pointsRule.CountryGroup override), with a DXCC-
// entity multiplier per band (§8.1); a non-Scandinavian entrant (DXScoring)
// scores only for Scandinavian QSOs — 1 point on 20/15/10M, 3 on 80/40M
// (§7.2's non-European-entrant formula; the flat 1-point European-entrant
// case is out of scope, matching this app's non-European station profile) —
// with the "sac_area" multiplier (§8.2: one Scandinavian-entity-plus-numeral
// combination per band).
func TestLoadEventCatalogSACCWHasRealScoringRules(t *testing.T) {
	events, err := loadEventCatalog()
	if err != nil {
		t.Fatalf("loadEventCatalog: %v", err)
	}
	sac := events[eventIndex(t, events, "SAC-CW")]
	if sac.Capability != catalogCapabilityScoringReady {
		t.Fatalf("SAC-CW capability = %q, want %q", sac.Capability, catalogCapabilityScoringReady)
	}
	if !sac.cabrilloReady() {
		t.Fatal("SAC-CW must have a checked Cabrillo layout")
	}
	if sac.ADIFContestID != "SAC-CW" {
		t.Fatalf("SAC-CW adif_contest_id = %q, want SAC-CW", sac.ADIFContestID)
	}
	if !countryInList(sac.DomesticCountries, "Sweden") || !countryInList(sac.DomesticCountries, "Norway") {
		t.Fatalf("SAC-CW domestic_countries = %v, want the Scandinavian country group", sac.DomesticCountries)
	}
	if sac.Scoring == nil || sac.Scoring.Points == nil {
		t.Fatal("SAC-CW must have a Points-based Scoring rule")
	}
	p := sac.Scoring.Points
	if p.GroupPoints != 0 || p.SameContinent != 2 || p.OtherContinent != 3 {
		t.Fatalf("SAC-CW scoring.points = %+v, want group 0 / same-continent 2 / other-continent 3", p)
	}
	if !countryInList(p.CountryGroup, "Sweden") {
		t.Fatalf("SAC-CW scoring.points.country_group = %v, want the Scandinavian country group", p.CountryGroup)
	}
	mults := sac.Scoring.effectiveMultipliers()
	if len(mults) != 1 || mults[0].Kind != "dxcc" || mults[0].Per != "band" {
		t.Fatalf("SAC-CW scoring multipliers = %+v, want [{dxcc band}]", mults)
	}
	if sac.DXScoring == nil || sac.DXScoring.Points == nil {
		t.Fatal("SAC-CW must have a Points-based DXScoring rule")
	}
	dp := sac.DXScoring.Points
	if dp.GroupPoints != 1 || dp.LowBandGroupPoints != 3 {
		t.Fatalf("SAC-CW dx_scoring.points = %+v, want group 1 / low-band group 3", dp)
	}
	dxMults := sac.DXScoring.effectiveMultipliers()
	if len(dxMults) != 1 || dxMults[0].Kind != "sac_area" || dxMults[0].Per != "band" {
		t.Fatalf("SAC-CW dx_scoring multipliers = %+v, want [{sac_area band}]", dxMults)
	}
	if got := sac.effectiveScoring("Sweden"); got != sac.Scoring {
		t.Fatal("a Scandinavian station (Sweden) must use Scoring")
	}
	if got := sac.effectiveScoring("Germany"); got != sac.DXScoring {
		t.Fatal("a non-Scandinavian station (Germany) must use DXScoring")
	}
	if got := sac.effectiveScoring(""); got != sac.Scoring {
		t.Fatal("an unresolved station must conservatively fall back to Scoring")
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
	// selectEvent lands back on QSO Entry with Call focused (not Contest
	// Entry, which has no Call field) so the operator can start logging
	// immediately; the contest fields it configured are still set underneath.
	if m.screen != qsoEntryScreen || m.focusIdx != fieldCall {
		t.Fatalf("selected event screen = %v focusIdx = %v, want qsoEntryScreen/fieldCall", m.screen, m.focusIdx)
	}
	if m.contestFields[contestName].Value() != "CW-OPEN-1" || m.contestFields[contestSerialSent].Value() != "001" {
		t.Fatalf("selected event contest fields = %#v", m.contestFields)
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
