# w4gns-logger — Contest features roadmap & design

Single source of truth for everything we've changed and everything still to
build to make this a real contest logger, using SD (EI5DI) as the feature
benchmark. Part 1 is the tracker; the appendices hold the detailed design.

- **Scope:** this document tracks the *contest* feature set specifically.
  The app itself is a general-purpose CW logger first — QSO Entry, QSO
  Details (name/QTH/grid/POTA/notes), ADIF import/export, QRZ/WRL lookups,
  and the DX cluster all work identically with no contest selected. Contest
  mode (event catalog, dupe/mult scoring, Cabrillo export, the as-you-type
  analysis tools) is a layer on top, active only while a contest is selected
  on the Events (F7) screen — everything below is about deepening that
  layer, not a description of the whole app. The app logs CW only today.
- **SD manual reference:** `~/Downloads/sditalia.pdf` (27 pp., Italian ed., 2025);
  its full feature set is cataloged **in English** in Appendix D.
- **Status legend:** ✅ done · 🔧 in progress · ⏳ planned · 🧪 needs test · ❓ needs a decision

---

# Part 1 — Tracker

## 0. Code-review backlog (2026-09-04)

The items below came from a full repository review. They are defects or gaps in
already-described behavior, not new feature ideas. Severity reflects risk to
the log, submitted contest results, or external services.

### P1 — correctness / data-delivery risks

- ✅ **QSO persistence and initial upload enqueueing are one transaction.**
  `store.insertQSOWithUploads` now inserts the QSO plus all configured QRZ/WRL
  outbox rows through one SQLite transaction; logging fails rather than leaving
  a saved contact without its required initial deliveries. Regression coverage
  verifies the committed QSO/outbox set. Statement-boundary fault injection and
  a user-facing per-delivery status view remain future hardening.

- 🔧 **Cabrillo export is gated by checked per-event layouts.** The catalog now
  has validated `cabrillo_layout` values, and export refuses any event without
  one while leaving CSV available. The checked CW layouts currently cover CWT,
  CW Open, CQ WW CW, CQ 160 CW, ARRL DX CW, CQ WPX CW, and (Cabrillo shape
  only, no scoring yet) **SAC-CW**; every other catalog entry is intentionally
  selection/entry-only until its sponsor-specific schema is verified.
  Scandinavian Activity Contest, CW (`events/contestcalendar.json`) now
  carries `cabrillo_layout: cw_rst_exchange` and `adif_contest_id: SAC-CW`,
  promoting its `capability` from `entry-aware` to `cabrillo-ready` (the
  distinct capability tier for "Cabrillo line shape checked, scoring not yet
  audited") — sourced from sactest.net's Cabrillo 3.0 spec (`QSO: freq mo
  date time call rst exch call rst exch t`: RS(T) + a single free-text
  serial-number exchange field on both sides, the same shape the existing
  `cw_rst_exchange` layout already renders) and confirmed against the ADIF
  Contest ID Enumeration. No `scoring` block was added — SAC's real
  points/multiplier rules (serial-number dupe/zero-point handling, per-band
  multiplier shape) still need their own audit before this entry can move to
  `scoring-ready`. Test: `events_test.go`
  (`TestLoadEventCatalogSACCWHasCheckedCabrilloLayout`).

- 🔧 **Audit and implement scoring per contest before presenting the catalog as
  correct.** Every one of the 429 event records now declares and is validated
  against an explicit `capability`: 9 intentionally generic templates are
  `selection-only`, 409 are `entry-aware`, and `CW-OPEN`, `CWT`, `CQ-WW-CW`,
  `CQ-160-CW`, `ARRL-DX-CW`, `CQ-WPX-CW`, `TNQP`, `SAC-CW`, `NAQP-CW`,
  `ARRL-SS-CW`, and `IARU-HF` are `scoring-ready`. **SAC-CW's real scoring** (sactest.net's Sections 7-8) is
  side-asymmetric around a fixed "Scandinavian" country group — Norway,
  Finland, Sweden, Iceland, Denmark, and the territories the rules list by
  their own prefix block (Svalbard, Jan Mayen, Åland Islands, Market Reef,
  Greenland, Faroe Islands) — rather than by the operator's own continent, a
  shape the existing `pointsRule` (same-country/same-continent/other-
  continent, relative to the operator) couldn't express: a Scandinavian
  station working another Scandinavian station is neither "same country" nor
  meaningfully "same continent" under this rule's own point scale, so a plain
  continent match would have silently mis-scored it. New **`pointsRule.
  CountryGroup`** (`events.go`, plus `GroupPoints`/`LowBandGroupPoints`) adds
  a group-membership tier `pointsTotal` (`contest_state.go`) checks before
  the country/continent classification: a worked station in `CountryGroup`
  scores the group value regardless of the operator's own location, and a
  station outside it falls through to the unchanged existing logic. `record()`
  now tracks `pointCategoryCountry` (the worked entity's country per scored
  QSO) unconditionally, since a group check doesn't need the operator's own
  station to resolve. Wired as `Scoring` for a Scandinavian entrant (§7.1:
  0 for a Scandinavian-Scandinavian QSO the rules don't otherwise address,
  2 for European-non-Scandinavian, 3 for non-European; DXCC-entity multiplier
  per band, §8.1) and `DXScoring` for a non-Scandinavian entrant (§7.2's
  non-European-entrant formula only — 1 point on 20/15/10M Scandinavian QSOs,
  3 on 80/40M, scored only for Scandinavian contacts; §7.2's flat 1-point
  European-entrant case is out of scope, matching this app's own
  non-European station profile, the same "only the branch that applies to
  this app's station" scoping TNQP's out-of-state multiplier used). New
  **`sac_area` multiplier kind** (`sac_area.go`, `sacAreaCode`) implements
  §8.2's "prefix-number in each Scandinavian DXCC entity" — SI3/SK3/SL3/SM3
  all resolve to the same `Sweden-3` value, matching the rule's own examples;
  a call with no digit is the 0th area. Tests: `sac_area_test.go`
  (`TestSACAreaCode`, `TestSACScandinavianCountriesHas11Values`),
  `contest_state_test.go`
  (`TestContestStateScorePointsRuleCountryGroup`,
  `TestContestStateScorePointsRuleCountryGroupLowBand`,
  `TestContestStateScoreSACAreaMultiplier`,
  `TestContestStateWouldBeNewMultiplierSACArea`), `events_test.go`
  (`TestLoadEventCatalogSACCWHasRealScoringRules`, replacing the earlier
  cabrillo-ready-only guard). The
  Events screen shows this status, so an operator can tell an entry-only
  template from a checked submission before export. CWT's real scoring
  (cwops.org/cwops-tests/: 1 point per QSO, multiplied by unique callsigns
  worked — the same points/multiplier shape as CW Open, scoped per session by
  its existing `call+band+session` dupe_scope) is now wired
  (`events/cwops.json`); test `TestLoadEventCatalogCWTHasRealScoringRules`
  (`events_test.go`). TNQP's real scoring (tnqp.org/rules/: flat 3 points per
  QSO, plus a Tennessee-county multiplier counted once per band — "95
  maximum per band") is also wired (`events/tnqp.json`). Only the
  out-of-state entrant's multiplier category is configured, matching this
  catalog entry's existing out-of-state scope (README.md): the rules also
  describe a state/province/DXCC-entity multiplier set and a 100-point
  bonus per QSO with sponsor station K4TCG, both of which apply only to a
  Tennessee-resident entrant and so are out of scope here. New **`tn_county`
  multiplier kind** (`tn_county.go`, mirroring `exchange_area.go`'s shape)
  resolves the county from the worked station's received-exchange text
  (`qso.srxString`) against the 95 official four-letter county codes;
  `contest_state.go` extends the index with `tnCountyByBand`/`tnCountyAll`.
  ADIF export maps every `TNQP-*` session id to `TN-QSO-PARTY` (confirmed
  against the ADIF Contest ID Enumeration) and Cabrillo export uses the
  existing `cw_rst_exchange` layout (RST + one free-text exchange field).
  Tests: `tn_county_test.go` (`TestTNCountyCode`,
  `TestTNCountyCodesHas95Values`), `contest_state_test.go`
  (`TestContestStateScoreTNCountyMultiplierFromReceivedExchange`,
  `TestContestStateWouldBeNewMultiplierTNCounty`), `events_test.go`
  (`TestLoadEventCatalogTNQPHasRealScoringRules`). The actual scoring audit
  remains: every event except those seven still has no `scoring` block and
  must not be promoted until its rules are tested against authoritative
  examples.

- ✅ **Zone autofill no longer infers rules from prose hints, and is now
  side-aware by the worked entity.** It requires an explicit
  `received_exchange_autofill` catalog value (`events.go`
  `receivedExchangeZoneKind`). For contests where the exchange genuinely
  differs by which side the *worked* station is on — CQ 160 CW's DX stations
  send a CQ zone, but W/VE stations send a state/province cty.dat has no way
  to derive — a new `received_exchange_autofill_domestic` list of exact
  cty.dat country names (`ReceivedExchangeAutofillDomestic`,
  `receivedExchangeAutofillExcluded`) excludes those entities from the
  callsign-derived guess instead of prefilling a value the station will never
  actually send. `autofillReceivedExchange` (`main.go`) checks the exclusion
  before filling the zone. Wired into curated `CQ-160-CW`
  (`received_exchange_autofill: cq_zone`, domestic `["United States",
  "Canada"]`); `CQ-WW-CW` needed no change since its exchange is a uniform CQ
  zone on both sides. ARRL DX CW's non-W/VE exchange is power, not a zone, so
  it's out of scope for this schema — its state/province-vs-DXCC multiplier
  asymmetry was the separate "exchange-derived multiplier kind" gap, since
  resolved (§3 Phase 3, `exchange_area` multiplier kind + `DXScoring`/
  `DomesticCountries`/`effectiveScoring`).
  The loader rejects a domestic list configured without an autofill kind.
  Tests: `events_test.go`
  (`TestLoadEventCatalogCQ160HasRealScoringRules`,
  `TestReceivedExchangeAutofillExcludedIsCaseInsensitiveAndSafeOnBlank`),
  `main_test.go` (`TestAutofillReceivedExchangeZoneExcludesDomesticEntities`).

### P2 — stale state, interoperability, and retry bugs

- ✅ **Station identity changes rebuild dependent state and rotate cluster
  login.** Saving a changed callsign or grid rebuilds the live contest index;
  a changed callsign disconnects an active/pending cluster session before the
  normal reconnect path runs.

- ✅ **QRZ late enrichment is correlated by request ID.** A lookup is bound to
  a QSO only while that exact request remains outstanding; obsolete, error, or
  already-consumed results are discarded rather than patching a later QSO with
  the same callsign. Regression coverage includes result-before-save and
  repeated-same-call ordering.

- ✅ **Upload completion/failure is persisted before Tea receives the result,
  and shutdown waits for it.** QRZ/WRL outbox commands are registered with the
  background-task lifecycle and acknowledge/reschedule their delivery in the
  command itself. This closes the successful-request/exit race; network
  ambiguity still gives the overall external-delivery contract at-least-once.

- ✅ **Repeated atomic exports replace existing files on Windows.** Exporters
  use a platform replacement helper: POSIX uses rename and Windows uses
  `MoveFileEx` with replace-existing/write-through flags. The Windows target is
  cross-built; native Windows filesystem regression coverage remains desirable.

- ✅ **Map internal contest/session ids to standard ADIF identifiers.** Every
  currently promoted event (CWT, CW Open, CQ WW CW, and CQ 160 CW) declares a
  validated per-event `adif_contest_id`; ADIF export resolves session-specific
  internal ids through that catalog metadata while preserving the database's
  internal id for scoring and dupe scope. The values are checked against the
  ADIF Contest ID Enumeration.

- ✅ **Scoring prefers persisted entity context.** Contest iteration loads each
  QSO's country/DXCC/CQZ/ITUZ fields and `contestState` uses valid recorded
  values before the current cty.dat lookup. An explicit operator-driven
  re-resolution workflow remains future work.

- ✅ **POST duplicate checks use the entered timestamp.** POST time is parsed
  before duplicate detection and drives both the casual 15-minute window and
  stored start/end timestamps. Rate-meter treatment for deliberately
  out-of-order paper-log entry remains a product decision.

### P3 — roadmap/UI accuracy and hardening

- ✅ **QSO Entry no longer prompts for RST on a contest that doesn't exchange
  it.** `entrySlots` always rendered RST Sent/Rcvd as fixed base fields
  regardless of the active event, so CW Open and NAQP CW — both
  `cabrillo_omit_rst: true`, confirmed against cwops.org/cwops-tests/cw-open/
  ("Sequential Serial Number and Name") and ncjweb.com/NAQP-Rules.pdf (Rule
  10, name and location only) — still showed two RST input boxes with an
  unused "599" default on screen, even though Cabrillo export already
  omitted the columns for these events (`cw_exchange_only` layout): the
  on-screen form disagreed with what the loaded event actually exchanges.
  `entrySlots` (`main.go`) now drops RST Sent/Rcvd entirely while the active
  event's `CabrilloOmitRST` is set. Because base fields no longer always
  occupy fixed positions 0..`fieldCount`-1, every direct
  `m.focusIdx == fieldBand` comparison was replaced with a new
  position-independent `focusedBaseFieldIndex()` helper, and the contest
  Enter-fast-path's jump target (previously a hardcoded `fieldCount`) is now
  the resolved index of the first contest slot. `fieldCall` comparisons were
  left as direct constants since Call is never hidden. Tests:
  `TestEntrySlotsHideRSTForCabrilloOmitRSTEvent` (also guards that an event
  that does exchange RST, e.g. CQ WW CW, is unaffected),
  `TestEnterAfterCallFastPathsPastAutoFilledFieldsDuringContest` and
  `TestContestReceivedExchangeLoggedInlineOnEntryScreen` updated for the new
  slot count/positions.

- ✅ **Bearing/distance prefers the worked station's entered grid.** The
  analysis panel uses a valid entered/QRZ-filled grid before falling back to
  DXCC coordinates; a regression test verifies the bearing changes accordingly.

- ✅ **The continent-level panel is accurately named `Continents Worked`.** The
  current `continentBand` index counts QSOs per continent and the panel reports
  only six continent-level worked/not-worked states, not countries worked or
  wanted within each continent. The screen title, Help entry, and hotkey label
  now say `Continents Worked`; entity-level totals/denominators remain future
  feature work rather than an implied capability.

- ✅ **Contest-index rebuild failures preserve known-good state and are visible.**
  `rebuildContestIndex` retains the prior index only when the failed rebuild is
  for that same contest, marks it stale in the QSO Entry analysis and
  Continents panel, and clears it on a failed switch to a different contest so
  one event's dupes/multipliers can never appear under another. Regression
  coverage closes the database after a successful build and verifies both the
  retained state and visible stale warning.

- 🔧 **Increase integration coverage around risky asynchronous and file paths.**
  This pass adds deterministic coverage for atomic initial enqueueing,
  persisted upload acknowledgement, QRZ request correlation, identity rotation,
  POST duplicate timing, persisted scoring context, and grid bearings. Native
  Windows replacement, TUI screens, and reconnect scheduling still need
  dedicated tests; keep the existing `go test`, `go vet`, race, cross-build,
  and vulnerability gates.
- ✅ **Full `drainOutbox`/shutdown-drain lifecycle, verified with Go 1.27's
  new `goroutineleak` profiler.** `TestShutdownDrainLeavesNoLeakedGoroutines`
  (`goroutineleak_test.go`) reproduces `main()`'s exact shutdown sequence
  (cancel `bgCtx`, then `bgTasks.Wait()`) with real QRZ/WRL outbox uploads
  (`httptest` servers, `qrzLogbookAPI`/`wrlContactsAPI` override) and an ADIF
  import all in flight at once — the three `bgTasks`-tracked job kinds that
  exist today. `waitForBgTasksOrDumpLeaks` bounds the wait so a regression
  (a job that fails to call `wg.Done()` on some return path, or never
  registered `wg.Add(1)` before its goroutine started) fails the test in
  seconds with the exact stuck stack from `runtime/pprof`'s `goroutineleak`
  profile, instead of hanging until `go test`'s own timeout with no
  diagnostic. `assertNoGoroutineLeaks` then confirms the profile is empty
  after drain — sanity-checked against a deliberately-never-`Done()`
  `sync.WaitGroup` in a throwaway program, which the profile correctly
  reported with a full stack trace.

## 1. Done (already shipped)

### CW Open correctness
- ✅ **Cabrillo QSO line omits RST for CW Open.** `event.cabrillo_omit_rst` drops
  the RST columns so the exchange is serial+name only (CW Open sends no RST).
  `cabrillo_export.go`, `events/cwops.json`.
- ✅ **Per-session scoring.** `scoringRules` (`points_per_qso` × `unique_call`
  multipliers); real `CLAIMED-SCORE` written to the header and shown in the UI
  status line. `events.go`, `cabrillo_export.go`, `main.go`.

### Running serial number
- ✅ **Auto-incrementing sent serial.** Seeds `001` on event select, advances
  past the number actually sent (carries a manual correction forward), survives
  the between-QSO reset. `main.go` (`nextSerial`, `formatSerial`, `clearQSOForm`).
- ✅ **Serial visible on both screens** — Contest Entry (F7) field *and* mirrored
  into the QSO Entry header (`Sending # NNN`).

### Inline received exchange (log the worked station on one screen)
- ✅ **Received exchange captured on the main QSO Entry screen.** When a contest
  is active the field row grows to include the worked station's exchange
  (`Rcv #` + `Rcv Exch` for serial contests) — no trip to F7. Modeled as
  `entrySlot`s so base fields keep positions 0–4 and existing focus logic is
  untouched. `main.go` (`entrySlots`, `focusedInput`, `focusField`, `renderSlot`).

### Contest catalog from SD templates
- ✅ **271 SD contests imported** into `events/sd_contests.json` (factual params
  only: name, bands, serial/RST shape, exchange hints; 1 synthetic session each).
  Generator: `~/Downloads/sd_gen.py`. Proprietary SD data (`.MLT/.DTA/.LST/.CTY`)
  was **not** copied.
- ✅ **`cabrillo_contest` field** so side-variant entries (e.g. ARRL DX CW home/DX)
  keep unique IDs but export one shared Cabrillo `CONTEST:` token.

### Edit correctness
- ✅ **Editing a QSO from a different (or no) contest no longer clobbers the
  active contest session.** `beginEditQSO` loads the edited row's own contest
  fields for editing; previously nothing restored the operator's actual active
  contest afterward, silently mis-tagging every QSO logged post-edit. Fixed
  with a pre-edit snapshot/restore. `main.go` (`preEditContestName` et al.,
  `restorePreEditContestSelection`).

### Tests covering the above
- ✅ CW Open RST omission, scoring, serial increment, header mirror, inline
  received exchange, SD catalog load, Cabrillo-token override, Enter fast-path
  (with/without an active contest), edit-from-different-contest restore.

## 2. Near-term — finish the contests we already list

Make the 271 imported contests *correct*, not just selectable.

- ✅ **Ergonomic entry order.** Enter-after-Call fast-paths to the received
  exchange, skipping auto-filled RST/Band/Freq, only while a contest is active
  (no exchange to jump to otherwise); Tab still visits every field. `main.go`
  (the "enter" key case in `updateQSOEntry`).
- ✅ **Curated-vs-generated de-dup.** `loadEventCatalog` drops a generated
  (`events/sd_contests.json`) entry when its `cabrillo_contest` token is a
  straight 1:1 duplicate of a curated event (`CW-OPEN` vs `SD-CWOPEN`) —
  curated wins. A token shared by *two or more* generated entries and one
  curated entry (e.g. `ARRL-DX-CW`'s SD home/DX split vs one generic curated
  entry) is left alone: that's added fidelity, not a duplicate. `events.go`
  (`eventDefinition.cabrilloToken`, `generatedEventCatalogFile`).
- ✅ **Real sessions/schedules** for multi-session contests needing per-session
  dupe scope and per-session Cabrillo files. The plumbing (schema, `dupe_scope`,
  per-session Cabrillo export) already exists generically — the gap was real
  per-contest session data replacing the SD catalog's synthetic single "ALL"
  session. Most major DX contests (ARRL DX, CQ WW, IARU HF) run one continuous
  block and don't need this; the real candidates are weekly sprints with
  genuinely distinct time slots. **K1USN Slow Speed Test (SST)** added as
  `events/k1usn.json` (`K1USN-SST`, 2 real sessions: Fri 2000-2100 UTC / Mon
  0000-0100 UTC, `dupe_scope: call+band+session`) — de-dups the generated
  `SD-SST` entry via the existing `cabrillo_contest: SLOW-SPEED-TEST` token
  match. Close-out audit found one more real duplicate the token-based de-dup
  was missing: curated `CWT` (`events/cwops.json`, 4 real Wed/Thu sessions)
  had no `cabrillo_contest` override, so it fell back to token `CWT` instead
  of the contest's actual Cabrillo tag `CW-OPS` (confirmed against WA7BNM's
  Cabrillo names reference) — that mismatch meant the generated `SD-CWOPS`
  entry ("CWOPS Mini-CWT", already tagged `CW-OPS`) survived de-dup as a
  spurious single-session duplicate of the same real-world contest, and the
  curated entry's own Cabrillo exports carried the wrong `CONTEST:` value.
  Fixed by adding `"cabrillo_contest": "CW-OPS"` to curated `CWT`; test
  `TestLoadEventCatalogPrefersCuratedCWTOverGeneratedDuplicate` guards both
  the de-dup and the 4-session count. Other weekly sprints in the catalog
  (AP/SA/NA Sprint, RSGB sprints, Russian Mini-Test 40/80, SCAG Sprint) run a
  single hour/period once a week and are already correctly single-session —
  verified no other generated entry shares a token with a curated one without
  already being caught by the existing de-dup.
- ⏳ **Mode handling.** `CATEGORY-MODE` is hard-coded `CW`; for mixed events either
  mark CW-only in-app or add SSB logging (larger — §4). ❓ Decision.

## 3. Core real-time engine (SD headline features)

Design detail in Appendix A–C. Shared backend is a **`contestState` index** on
the model (built on open, incremental on log, full recompute on edit) that feeds
every panel *and* scoring so they always agree.

**Phase 1 — analysis engine + always-on panels**
- ✅ `dxcc.go`: capture the per-entity **lat/lon** (header fields 5/6 and the
  per-alias `<lat/lon>` override), normalizing cty.dat's west-positive
  longitude to standard east-positive. `dxccEntity.Latitude/Longitude`,
  `parseAliasLatLon`; test `TestDXCCLookupNormalizesLongitudeEastPositive`.
- ✅ `heading.go`: great-circle **bearing + distance**. `GreatCircleBearingDistance`
  (haversine + initial bearing, km) and `KmToMiles`; wiring to a prefs unit
  toggle is deferred to the analysis-panel UI work below. Tested against
  equator/meridian exact cases, antipode, zero-distance, and a real
  southern/western-hemisphere city pair.
- ✅ `contestState` index; refactor `computeContestScore` to read it.
  `contest_state.go` (`byCall`, `workedCallBand`, `uniqueCalls`,
  `buildContestState`, `.score()`, `.isWorkedOnBand()`); full rebuild per call
  today (correct and fast enough at contest-log sizes), incremental-on-log
  wiring deferred to the live TUI model when the as-you-type panels land.
  Tested: worked-band lookups, full recompute after a call/band edit
  (Appendix E's hardest case), nil-rules score.
- ✅ **As-you-type analysis panel**: dupe, country/CQ/ITU/continent, beam
  heading+distance, new-multiplier flag (`unique_call` rule only — double-mult
  and area-code mults wait for the Phase 3 data-driven multiplier schema),
  band-worked matrix — right-hand column beside QSO Entry, gated by
  `analysisPanelMinWidth` the same way `dxSpotsPanel` degrades on narrow
  terminals. `analysis_panel.go` (`analysisPanel`), `main.go`
  (`model.contestIndex`/`contestIndexID`), `contest_state.go`
  (`rebuildContestIndex`, the sync point wired into `checkDupe`,
  `beginEditQSO`/`cancelEditQSO`, edit-save, insert, and delete — full
  recompute on edit/delete, incremental on log, per Appendix C).
- ✅ **Auto-fill zones** with operator-override carried forward. For a contest
  whose `received_exchange_hint` names a CQ or ITU zone,
  `eventDefinition.receivedExchangeZoneKind` (`events.go`) infers which one
  from the hint text (no per-event JSON tagging needed) and
  `autofillReceivedExchange` (`main.go`) prefills the resolved DXCC entity's
  zone into the inline Rcv Exch field as the operator types the call,
  sharpening with each keystroke. `contestExchangeRcvdEdited` tracks the same
  autofill-until-overridden shape as `nextSerial`: once the operator edits the
  field's *content* (not just moves the cursor) or an edited QSO's real stored
  exchange loads for editing, autofill stops touching it until the next QSO
  (`clearQSOForm`) or contest (`selectEvent`). Area codes are deferred — they
  need `.mlt` data that doesn't exist in this repo yet (Phase 3).

**Phase 2 — databases & partials**
- ⏳ `roster.go`: `.LST` club rosters (bidirectional call↔name↔number), prefill.
  Blocked on open decision #2 (data licensing).
- ✅ **Check Partial** list. `contestState.checkPartial` (`contest_state.go`)
  returns prior-logged calls in the active contest containing the in-progress
  fragment (substring match, self-match excluded, sorted, capped at 5); shown
  as a `Partial: ...` row in the Analysis panel (`analysis_panel.go`,
  `checkPartialLine`), bold for a candidate not yet worked on the currently
  selected band, dim if it would be a dupe there. Scoped to the operator's
  own log (no `.LST`/`MASTER.DTA` dependency, so it isn't blocked by #2);
  prefix/suffix mode toggle (`.`/`,`) and ↑-to-pull-into-field are deferred —
  substring match covers the common case display-only.
- ⏳ **Super Check Partial** highlight (known-good calls).
- ✅ **Continents Worked** panel; per-band paging.
  `contestState.continentBand` (`contest_state.go`) tallies each logged QSO's
  continent (resolved via `sharedDXCCTable` at `record()` time, the same
  lookup the Analysis panel already uses) per band; `continentSummary`
  exposes worked/needed + count for a continent/band pair. New full-screen
  `continentScreen` (`Ctrl+W`, `main.go` `openContinentPanel`,
  `updateContinentPanel`, `continentPanelView`) lists the six standard
  continents against the currently paged-to band — Left/Right page through
  the active event's allowed bands, or every supported band with no event
  selected (SD binds this to F1/F2, but F1 is already this app's own
  "QSO Entry" hotkey everywhere else, so it stays bound to its usual meaning).
- ✅ **Rate meter** (Q/hr last-10 / last-100 / overall, Q/Mult).
  `contestState.times`/`.rate()` (`rate_meter.go`) extend the index with each
  logged QSO's timestamp (rebuild and incremental-on-log paths both feed it
  through the existing `record()` sync point) and compute Q/hr per window as
  window-QSO-count ÷ elapsed-time-to-now from the window's oldest QSO — so
  the rate visibly decays if the operator stops calling CQ. Q/Mult reads the
  same `score()` multiplier count Cabrillo export uses, falling back to raw
  unique calls for non-`unique_call` scoring. Rendered as a status line under
  Recent QSOs/DX Spots on QSO Entry when a contest is active and something's
  been logged (`main.go` `View()`, `rateMeterLine`).

**Phase 3 — corrections, mult data, output parity**
- ✅ Log-wide **recompute on any edit** (dupes/mults/points) — SD's differentiator.
  `rebuildContestIndex` (full re-read from the store) already fires on every
  mutation path: edit-save (`logCurrentQSO`), table delete, `ZAP`, `/Z`,
  `/X`, and contest-switch; `score()`/`rate()` are always derived fresh from
  `contestState`, never cached. No remaining gap — verified no stale-score
  field exists on `model`.
- ✅ `/Z` (mark old for delete), `/X` (logged-but-unscored), `ZAP`, `SETDUPE`.
  Typed into the Call field and confirmed with Enter, intercepted in
  `handleCallFieldCommand` (`main.go`) before the normal Enter-to-advance/
  Enter-to-save handling. **ZAP** (`zapLastQSO`) deletes the most recently
  logged QSO while entering a new one. **`/Z`** (`deleteEditingQSO`) and
  **`/X`** (`toggleEditingQSOUnscored`) only fire while a QSO is loaded for
  editing (`beginEditQSO` leaves focus on Call): `/Z` deletes it instead of
  saving; `/X` flips a new `qso.unscored` / `store.setQSOUnscored` DB flag
  that keeps the QSO logged and in Cabrillo output (as an `X-QSO:` line,
  `cabrilloQSOLine`) but excludes it from `contestState.score()` via new
  `scoredCallBand`/`scoredUniqueCalls` sets kept alongside the existing
  worked/dupe sets — so an X-QSO still counts for dupe/Check-Partial/rate/
  continent (it happened) but not for `CLAIMED-SCORE`. **SETDUPE**
  (`setDupeBaseline`) resets `model.dupeBaselineAfter` to now; `store.isDupe`
  gained a `since` parameter so a station worked earlier no longer blocks
  re-working it — for multi-period sprints the event catalog doesn't model
  as distinct sessions. Reset on contest switch (`selectEvent`). In-app Help
  documents all four. Tests: `main_test.go` (`TestZapDeletesMostRecently…`,
  `TestZapWithNoQSOsIsANoop`, `TestSlashZDeletes…`,
  `TestSlashXTogglesUnscoredFlagAndExcludesFromScore`,
  `TestSetDupeResetsBaseline…`), `store_test.go`
  (`TestSetQSOUnscoredTogglesFlagAndPersists`,
  `TestSetQSOUnscoredRejectsWrongProfile`).
- ✅ **Data-driven multiplier schema** (`MULTSCOUNT` Band/Once). `events.go`
  (`multiplierRule{Kind, Per}`, `scoringRules.Multipliers`,
  `scoringRules.effectiveMultipliers` — the legacy scalar `Multiplier` field
  still works, translated to one `Per:"contest"` rule, so existing CW
  Open/CWops configs needed no edits) lets an event award several multiplier
  kinds that sum together — `dxcc`/`cqzone`/`ituzone`, each either `"band"`
  (the same entity/zone counts again on every band worked, CQ WW-style) or
  `"contest"` (counted once). `contest_state.go` extends the index with
  `dxccByBand`/`dxccAll`, `cqZoneByBand`/`cqZoneAll`,
  `ituZoneByBand`/`ituZoneAll` (scored-QSOs-only, same `/X` exclusion as
  `scoredUniqueCalls`), and `score()` sums `multiplierCount` over
  `effectiveMultipliers()`. The Analysis panel's "NEW MULT" flag (Appendix
  B.5 "advance multiplier flag," previously `unique_call`-only) now uses a
  new `wouldBeNewMultiplier` that checks every configured rule, so a
  DXCC/zone-mult contest gets an accurate live flag too — and as a side
  effect now reads `scoredUniqueCalls` instead of `uniqueCalls` for the
  "worked before" line, so a QSO logged `/X` no longer wrongly suppresses
  the flag. Area-mult `.mlt` tables remain blocked on data licensing
  (decision #2). Tests: `contest_state_test.go`
  (`TestContestStateScoreSumsDXCCAndZoneMultipliersPerBand`,
  `TestContestStateScorePerContestMultiplierCountsOnce`,
  `TestContestStateUnscoredQSOExcludedFromMultiplierCount`,
  `TestContestStateWouldBeNewMultiplier`), `events_test.go`
  (`TestScoringRulesEffectiveMultipliers`, `TestValidMultiplierKindAndPer`).
- ✅ **Continent/country-tiered points schema.** `events.go` (`pointsRule`,
  `scoringRules.Points` — set, it replaces `PointsPerQSO` entirely, mirroring
  `Multipliers`-over-`Multiplier` precedence) lets an event score a QSO by its
  relationship to the operator's own station instead of a flat per-QSO value
  (CQ WW-style: 0 same country, 1 same continent, 3 other continent).
  `contest_state.go` adds `contestState.setStation` (resolves the operator's
  callsign to its own DXCC country/continent via cty.dat) and classifies each
  scored QSO into `pointCategory` (`record()`, scored QSOs only, same `/X`
  exclusion as the multiplier sets); `score()` sums `pointsTotal()` when
  `Points` is set. An unresolved station (or worked entity) contributes 0
  rather than guessing a tier. `buildContestState`/`computeContestScore`/
  `rebuildContestIndex` thread the station's callsign through. The `.mlt`
  area-mult tables remain deferred/blocked on data licensing (decision #2).
  Tests: `contest_state_test.go`
  (`TestContestStateScorePointsRuleTiersByCountryAndContinent`,
  `TestContestStateScorePointsRuleUnresolvedStationScoresZero`,
  `TestContestStateScorePointsRuleTakesPrecedenceOverPointsPerQSO`).
- ✅ **Real per-contest wiring: CQ WW CW's actual scoring rules.** Closes the
  gap the two schema items above deliberately left open — curated
  `CQ-WW-CW` (`events/contestcalendar.json`) now carries a real `scoring`
  block sourced from cqww.com/rules.htm rather than a guess: `points` of
  0/1/3 for same-country/same-continent/other-continent, plus `dxcc` and
  `cqzone` multipliers counted per band. The rules include one exception the
  existing `pointsRule` schema couldn't express — a same-continent,
  different-country QSO *within North America* is worth 2 points, not the
  contest's general same-continent value — so `pointsRule` gained
  `SameContinentOverrides map[string]int` (`events.go`, keyed by the same
  continent codes `contest_state.go`'s `continents` list uses, validated by
  new `validContinentCode`) and `contestState` gained a parallel
  `pointCategoryContinent` map recording which continent a
  `pointCategorySameContinent` QSO resolved to, so `pointsTotal` can look up
  an override before falling back to the flat `SameContinent` value. Tests:
  `contest_state_test.go` (`TestContestStateScorePointsRulePerContinentOverride`),
  `events_test.go` (`TestLoadEventCatalogCQWWHasRealScoringRules`).
- ✅ **Real per-contest wiring: CQ 160-Meter CW's actual scoring rules,**
  including the exchange-derived multiplier schema gap this entry originally
  left open. Curated `CQ-160-CW` (`events/contestcalendar.json`) carries a
  real `scoring` block sourced from cq160.com/rules/: flat 2/5/10 points for
  same-country/same-continent/other-continent (no NA-style exception here,
  unlike CQ WW), a DXCC-entity multiplier, and — re-reading the current
  rules text closely — a second, *uniformly awarded* multiplier for each
  distinct US state/DC/Canadian province worked ("MULTIPLIER: U.S. States...
  Canadian Provinces..."), both `per: "contest"`. Unlike what this entry
  first assumed, that multiplier is not DX-side-only: CQ 160's rule counts
  it for every entrant. **New `exchange_area` multiplier kind**
  (`exchange_area.go`) resolves it from the worked station's *received
  exchange text* (`qso.srxString`) rather than a cty.dat callsign lookup —
  the first multiplier kind with no callsign-derived fallback — against a
  canonical 63-value table (48 contiguous US states + DC + the 14 Canadian
  provinces/territories both CQ 160 and ARRL DX CW's rules enumerate,
  Newfoundland/Labrador counted separately "for reasons of tradition"; an
  `NF`→`NL` alias covers ARRL DX's own alternate spelling). `contest_state.go`
  extends the index with `exchangeAreaByBand`/`exchangeAreaAll` (recorded in
  `record()`, summed in `multiplierCount()`) and threads the operator's
  in-progress received-exchange field text into `wouldBeNewMultiplier`
  (new `exchangeText` parameter, wired from `analysisPanel`) so the as-you-
  type "NEW MULT" flag works for this kind too. **Not wired into ARRL DX CW
  here**: that contest's version of this multiplier (Rule 5.2.2) is
  genuinely side-asymmetric — it replaces the DXCC-entity multiplier for a
  DX-side entrant rather than adding to it, unlike CQ 160's uniform award, so
  adding it to ARRL DX CW's single `scoring` block as configured here would
  double-count for this app's own (W/VE-side) station profile. Wiring it
  correctly needed the app to know which side the *operator's own station*
  is on and pick one multiplier rule set or the other — done in the very
  next entry below (`DXScoring`/`DomesticCountries`/`effectiveScoring`).
  Separately noted, not fixed here (out of scope for this change): CQ 160's
  `dxcc` multiplier
  as configured doesn't exclude the operator's own country, so a domestic
  QSO can be miscounted as a spurious 1-multiplier "DXCC entity" that isn't
  actually in the rule's DXCC/WAE country list — pre-existing, not
  introduced by this change. Tests: `exchange_area_test.go`
  (`TestExchangeAreaCode`, `TestExchangeAreaCodesHas63Values`),
  `contest_state_test.go`
  (`TestContestStateScoreExchangeAreaMultiplierFromReceivedExchange`,
  `TestContestStateWouldBeNewMultiplierExchangeArea`), `events_test.go`
  (`TestLoadEventCatalogCQ160HasRealScoringRules`).
- ✅ **Real per-contest wiring: ARRL International DX Contest, CW's actual
  scoring rules, including the side-asymmetric DX-side multiplier this entry
  originally left unwired.** Curated `ARRL-DX-CW`
  (`events/contestcalendar.json`) carries a real `scoring` block sourced from
  `contests.arrl.org/ContestRules/DX-Rules.pdf`: a flat 3 points per QSO on
  both sides (§5.1), a DXCC-entity multiplier counted once per band for a
  W/VE-side entrant (§5.2.1, `scoring`), and — now that `exchange_area`
  exists — the DX-side entrant's own US-state/DC/Canadian-province
  multiplier counted once per band (§5.2.2, new `dx_scoring`). **New
  side-asymmetric scoring schema** (`events.go`): `eventDefinition.DXScoring`
  is an alternate `scoringRules` block that applies instead of `Scoring`
  when the operator's own station does *not* resolve to one of the new
  `DomesticCountries` (`["United States", "Canada"]` here); blank/nil
  `DXScoring` (every event configured before this field existed) always uses
  `Scoring` regardless of country, so no other event needed changes.
  `eventDefinition.effectiveScoring(stationCountry)` implements the
  selection (falling back to `Scoring` for an unresolved station rather than
  guessing DX-side rules), and `contest_state.go`'s new `stationCountry`
  helper resolves an operator's callsign to feed it. Every consumer that
  used to read `event.Scoring` directly now goes through
  `effectiveScoring`: `computeContestScore` (`cabrillo_export.go`, so
  Cabrillo `CLAIMED-SCORE` is correct for whichever side actually submits),
  `rateMeterLine`'s Q/Mult (`rate_meter.go`), and the as-you-type "NEW MULT"
  flag (`analysisPanel`, `analysis_panel.go`) — so a DX-side entrant now
  gets a correct claimed score and live multiplier flag instead of the
  W/VE-side rules silently misapplying to their log. The loader validates
  `DXScoring`/`Scoring` identically (extracted `validateScoringRules`,
  reused for both) and requires `DomesticCountries` whenever `DXScoring` is
  set (and vice versa) so the two fields can't be configured
  inconsistently. `adif_contest_id` (`ARRL-DX-CW`, confirmed against the
  ADIF Contest ID Enumeration) and `cabrillo_layout: cw_rst_exchange` (the
  exchange is one free-text field after RST on both sides — state/province
  for W/VE, power for DX — the same shape CQ WW/CQ 160 already use) were
  added alongside the scoring block, promoting the event's `capability` to
  `scoring-ready`. Separately noted, not fixed here (out of scope for this
  change, same caveat as CQ-160-CW's `dxcc` multiplier above): the W/VE-side
  `dxcc` multiplier doesn't itself exclude US/Canada, so it only stays
  correct because §1.1 structurally forbids a W/VE entrant from logging a
  domestic QSO under this contest in the first place — an operator error
  that logged one anyway would still be miscounted. Tests: `events_test.go`
  (`TestLoadEventCatalogARRLDXCWHasRealScoringRules`,
  `TestEventDefinitionEffectiveScoring`, `TestValidateScoringRules`),
  `cabrillo_export_test.go`
  (`TestComputeContestScoreARRLDXCWSideAsymmetric`).
- ✅ **Real per-contest wiring: CQ WW WPX Contest, CW's actual scoring
  rules**, the multiplier/points schema gap the CQ-160-CW and ARRL-DX-CW
  entries above left open ("prefix mult + band-tiered points"). Sourced from
  cqwpx.com/rules.htm (Rule V.B points, Rule V.C prefix multiplier):
  - **New `prefix` multiplier kind** (`events.go` `validMultiplierKind`;
    `contest_state.go` `prefixByBand`/`prefixAll`, extending
    `multiplierCount`/`wouldBeNewMultiplier`) counted once per contest
    regardless of band ("Each PREFIX is counted only once regardless of the
    band ... it is worked"), unlike `dxcc`/`cqzone`/`ituzone` this counts
    from the callsign directly rather than a cty.dat resolution, so it never
    skips an unresolved call. `wpx.go` (`wpxPrefix`) implements Rule V.C's
    "letter/numeral combination which forms the first part of the call":
    portable-designator handling (a full alternate prefix after `/` replaces
    the home prefix; a numeral-only designator like `/4` swaps in for the
    home prefix's own numeral; non-qualifying suffixes `/P /M /MM /AM /A /E
    /J /QRP` are ignored) and the rule's own no-numeral examples
    (`PA/N8BJQ`→`PA0`, `XEFTJW`→`XE0`) — a practical implementation, not
    exhaustive for exotic call formats, the same class of documented
    approximation as `dxccTable.lookup`.
  - **Band-tiered `pointsRule`** (`events.go` `LowBandSameContinent`/
    `LowBandOtherContinent`/`LowBandSameContinentOverrides`;
    `contest_state.go` `pointsTotal` reading the QSO's own band via
    `bandFromCallBandKey`, `wpxLowBand` for the 160/80/40M vs. 20/15/10M
    split) expresses WPX's double points on 40/80/160M relative to
    10/15/20M (1/1/3 same-country/same-continent/other-continent, doubled to
    1/2/6), including a North America same-continent exception (2 points
    high band, 4 low) distinct from the flat override. Zero (unset) falls
    back to the base field so CQ WW/CQ 160's existing non-tiered configs
    needed no changes.
  - Curated `CQ-WPX-CW` (`events/contestcalendar.json`) now carries the real
    `scoring` block plus `adif_contest_id: CQ-WPX-CW` (ADIF Contest ID
    Enumeration) and `cabrillo_layout: cw_rst_exchange` (RST + serial number
    on both sides), promoting `capability` to `scoring-ready`. Tests:
    `wpx_test.go` (`TestWPXPrefix`), `contest_state_test.go`
    (`TestContestStateScorePrefixMultiplierCountsOncePerContest`,
    `TestContestStateScorePointsRuleWPXBandTiering`), `events_test.go`
    (`TestLoadEventCatalogCQWPXHasRealScoringRules`).
- ✅ **Real per-contest wiring: NAQP CW's actual scoring rules**, sourced from
  ncjweb.com/NAQP-Rules.pdf. Rule 13 ("Multiply total valid contacts by the
  sum of the number of multipliers worked on each band") is a flat 1 point
  per QSO — no continent/country tiering, unlike CQ WW/CQ 160/ARRL DX/WPX —
  times a Rule 11 multiplier: all 50 US states (including Alaska/Hawaii),
  DC, the 13 Canadian provinces/territories (Newfoundland and Labrador
  combined into one value, unlike the existing `exchange_area` table's
  separate NL/LB), and "other North American entities" identified by DXCC
  prefix, counted again on every band. This is exactly the existing
  `PointsPerQSO`/`multiplierRule{Per:"band"}` shape (`contest_state.go`
  `score()`, `total()`), so no new scoring schema was needed. **New
  `naqp_area` multiplier kind** (`naqp_area.go`, `naqpAreaCode`) reads the
  worked station's received-exchange text: unlike `exchange_area`/
  `tn_county` (whose contests exchange location alone), NAQP's exchange is
  "Name + location" typed into the same single free-text field, so only the
  last whitespace-separated token is checked against the 64-value state/
  province table; a miss falls back to a `dxccTable.lookup` on that token
  as a bare DXCC prefix (Rule 11: "please use the standard DXCC prefix ...
  in the received location field"), counting a hit outside the US/Canada as
  a multiplier keyed by its country name. `contest_state.go` extends the
  index with `naqpAreaByBand`/`naqpAreaAll` (recorded in `record()`, summed
  in `multiplierCount()`) and wires the as-you-type "NEW MULT" flag in
  `wouldBeNewMultiplier`. Curated `NAQP-CW` (`events/contestcalendar.json`)
  carries the real `scoring` block plus `adif_contest_id: NAQP-CW` (ADIF
  Contest ID Enumeration) and `cabrillo_layout: cw_exchange_only` +
  `cabrillo_omit_rst: true` (Rule 10: name and location only, no RST — the
  same shape CW Open uses), promoting `capability` to `scoring-ready`. Not
  addressed here (a documented limitation, not a rules gap): the
  last-token heuristic assumes the conventional "name, then location"
  typing order, matching the practical-approximation class of `wpxPrefix`.
  Tests: `naqp_area_test.go` (`TestNAQPAreaCode`,
  `TestNAQPAreaCodesHas64Values`), `contest_state_test.go`
  (`TestContestStateScoreNAQPAreaMultiplierFromReceivedExchange`,
  `TestContestStateWouldBeNewMultiplierNAQPArea`), `events_test.go`
  (`TestLoadEventCatalogNAQPCWHasRealScoringRules`).
- ✅ **Real per-contest wiring: ARRL November Sweepstakes, CW's actual scoring
  rules**, including a dupe-scope gap this entry uncovered. Sourced from
  contests.arrl.org/ContestRules/SS-Rules.pdf: Rule 5.1 is a flat 2 points per
  QSO (no continent/country tiering, unlike CQ WW/CQ 160/ARRL DX/WPX) times a
  Rule 5.2/5.3 multiplier — every ARRL/RAC section worked, counted once for
  the whole contest (not per band, unlike every multiplier kind wired so
  far). **New `arrl_section` multiplier kind** (`arrl_section.go`,
  `arrlSectionCode`) resolves it from the worked station's received-exchange
  text: SS's full exchange is serial + precedence + call + check + section,
  which — following naqp_area.go's "cram more than one exchange element into
  the single free-text field" precedent — the operator types as "precedence
  check section" after the serial (already its own field, `q.srx`), so only
  the last whitespace-separated token is checked. The canonical table is the
  85-value ARRL/RAC Section Abbreviation List (arrl.org/files/file/Field-Day/
  Generic/ARRL-RAC%20Section%20List.pdf, revised 2025); `GTA`→`GH` and
  `NT`→`TER` aliases cover the two sections' prior abbreviations, which
  remain common in contest-logger use after the official rename.
  `contest_state.go` extends the index with `arrlSectionByBand`/
  `arrlSectionAll` (recorded in `record()`, summed in `multiplierCount()`)
  and wires the as-you-type "NEW MULT" flag in `wouldBeNewMultiplier`. Rule
  2.2 ("Each station may be contacted only once, regardless of band") is
  genuinely different from every other configured event, which allows
  re-working a station once per band: the existing `dupe_scope` schema only
  offered `call+band`/`call+band+session`, both of which filter on band in
  `store.isDupe`'s query, so neither could express a whole-contest,
  band-agnostic dupe check. **New `dupe_scope: "call"`** (`events.go`
  `validDupeScope`; `store.go` `isDupe`) drops the band filter entirely for
  this one case, leaving Check Partial's per-band worked/dupe styling
  (`contestState.isWorkedOnBand`, cosmetic only — the real gate is
  `store.isDupe`) as a documented, out-of-scope display nuance, the same
  class of approximation as NAQP's last-token exchange heuristic. Curated
  `ARRL-SS-CW` (`events/contestcalendar.json`) carries the real `scoring`
  block plus `adif_contest_id: ARRL-SS-CW` (ADIF Contest ID Enumeration),
  `dupe_scope: "call"`, and `cabrillo_layout: cw_exchange_only` +
  `cabrillo_omit_rst: true` (no RST is exchanged), promoting `capability` to
  `scoring-ready`; the generated `SD-ARRLSSC` duplicate already falls out via
  the existing curated-vs-generated de-dup (shared `ARRL-SS-CW` token). Tests:
  `arrl_section_test.go` (`TestARRLSectionCode`,
  `TestARRLSectionCodesHas85Values`), `contest_state_test.go`
  (`TestContestStateScoreARRLSectionMultiplierCountsOncePerContest`,
  `TestContestStateWouldBeNewMultiplierARRLSection`), `store_test.go`
  (`TestDupeCheckHonorsCallScopeRegardlessOfBand`), `events_test.go`
  (`TestLoadEventCatalogARRLSSCWHasRealScoringRules`).
- ✅ **Real per-contest wiring: IARU HF World Championship, CW's actual
  scoring rules**, a genuinely new points/multiplier shape (zone-tiered
  rather than country/continent-tiered, and scored from the worked station's
  actually-exchanged value rather than a cty.dat callsign lookup). Sourced
  from `contests.arrl.org/ContestRules/IARU-HF-Rules.pdf` (Rules 4-5): every
  station sends RS(T) plus either its ITU zone, an IARU Member Society HQ
  abbreviation (Rule 4.2.1), or an Official code `AC`/`R1`/`R2`/`R3` (Rule
  4.2.2) — **`iaru_zone.go`**'s `iaruExchangeZone`/`iaruExchangeSpecial`
  parse the worked station's received-exchange text (`qso.srxString`) into
  whichever it sent, following the same exchange-is-authoritative precedent
  as `exchange_area.go`/`tn_county.go`/`naqp_area.go`/`arrl_section.go`
  rather than trusting a cty.dat-derived zone the operator might not
  actually be in. **New `pointsRule.Zone` schema** (`events.go`
  `zonePointsRule`) replaces the country/continent tiers entirely (validated
  mutually exclusive with `SameCountry`/`SameContinent`/`OtherContinent`/
  `CountryGroup`) with Rule 5.1's own tiers: 1 point for the worked station's
  own ITU zone or an HQ/Official contact (5.1.1/5.1.2 collapse to the same
  value), 3 for a different zone on the operator's own continent (5.1.4), 5
  for a different continent (5.1.5). `contest_state.go` adds
  `stationITUZone`/`stationZoneResolved` (the operator's own zone,
  `setStation`) and a parallel `pointCategoryZone` index (independent of the
  existing country-based `pointCategory`, since a zone-tiered event
  classifies QSOs on a different basis entirely) and `zonePointsTotal`.
  **New `iaru_zone`/`iaru_hq` multiplier kinds** (Rule 5.2.1: "the total
  number of ITU zones worked on each band ... plus IARU member society HQ
  stations worked on each band"; officials are naturally capped at 4 since
  only `AC`/`R1`/`R2`/`R3` exist, needing no special-case cap logic) extend
  `contestState` with `iaruZoneByBand`/`iaruZoneAll` and
  `iaruSpecialByBand`/`iaruSpecialAll`, wired into `multiplierCount` and the
  as-you-type `wouldBeNewMultiplier` flag. Curated `IARU-HF`
  (`events/contestcalendar.json`) carries the real `scoring` block plus
  `adif_contest_id: IARU-HF` (confirmed against the ADIF Contest ID
  Enumeration) and `cabrillo_layout: cw_rst_exchange` (RST + one free-text
  exchange field, the same shape CQ WW/CQ 160/ARRL DX/WPX already use),
  promoting `capability` to `scoring-ready`; the generated `SD-IARUHF`
  duplicate is dropped via the existing curated-vs-generated de-dup (shared
  `IARU-HF` Cabrillo token). Not addressed here (a documented limitation,
  not a rules gap): unlike `arrl_section.go`/`tn_county.go`'s fixed
  abbreviation tables, IARU has roughly 160 member societies with no single
  canonical machine-readable list in this repo, so `iaruExchangeSpecial`
  accepts any non-numeric exchange token rather than validating it against a
  known-society list — the same practical-approximation class as
  `wpxPrefix`'s non-exhaustive call-format handling. This app logs CW only,
  so the contest's own Phone/Mixed-Mode categories are out of scope, the
  same standing limitation as every other configured event (roadmap intro).
  Tests: `iaru_zone_test.go` (`TestIARUExchangeZone`,
  `TestIARUExchangeSpecial`), `contest_state_test.go`
  (`TestContestStateScorePointsRuleZoneTiers`,
  `TestContestStateScoreIARUZoneAndHQMultipliers`,
  `TestContestStateWouldBeNewMultiplierIARUZoneAndHQ`), `events_test.go`
  (`TestLoadEventCatalogIARUHFHasRealScoringRules`,
  `TestValidateScoringRules` new zone-rule cases).
- ✅ **CSV export** (`Ctrl+R`). `csv_export.go` (`exportCSV`, `writeCSVAtomic`,
  `csvField`/`csvRow` — RFC 4180 quoting, CRLF rows) streams the active
  contest's QSOs (same `contest_id` scoping as Cabrillo/ADIF export) to
  `{CALL}_{contestID}.csv` in Downloads; wired in `main.go`
  (`csvExportCmd`, `csvExportedMsg`, `model.csvExportInProgress`) with the
  same atomic-write-then-rename and `bgTasks` shutdown-drain shape as
  `cabrilloExportCmd`/`adifExportCmd`. Deliberately a plain QSO listing (no
  per-row points) — a correct per-row score needs the same dupe/once-per-band
  logic `contestState.score()` applies for `CLAIMED-SCORE`, which the
  Cabrillo header already surfaces. **Per-session Cabrillo filenames** were
  already effectively done: the export filename embeds the literal
  `contest_id` (`main.go` `cabrilloExportCmd`), which is session-granular
  (e.g. `K1USN-SST-MON`) whenever the operator selects a session-specific
  contest ID from the Events (F7) screen — no `CALL1.log`-style renaming
  needed since the existing naming already disambiguates by session.
- ✅ SDCHECK parity: **POST** (after-contest) entry mode. `Ctrl+P` toggles
  `model.postMode`; while on, `entrySlots()` appends a trailing "Date/Time
  UTC" field (`postFields`, format `2006-01-02 15:04`) to the QSO Entry row,
  and `logCurrentQSO` uses the operator-typed value for both time-on/time-off
  instead of `time.Now()` — refusing to save (no QSO logged) on an unparsable
  value. The field's value persists across QSOs (only the operator edits the
  minutes for consecutive paper-log entries) but is hidden while editing an
  existing QSO, since `logCurrentQSO`'s edit branch never rewrites a QSO's
  stored timestamp. Blocked from toggling mid-edit. The pre-existing Enter
  fast-path (skip to received exchange when a contest is active) is gated on
  the contest itself, not slot count, since POST mode's own trailing slot
  also grows `entrySlots()` past `fieldCount`. `main.go` (`postMode`,
  `postFields`, `postTimestamp`, `entrySlot.post`, the Ctrl+P handler,
  `logCurrentQSO`). Tests: `main_test.go`
  (`TestCtrlPTogglesPostModeAndAddsDateTimeSlot`,
  `TestCtrlPBlockedWhileEditingQSO`,
  `TestPostModeLogsQSOWithTypedTimestampInsteadOfNow`,
  `TestPostModeRejectsUnparsableTimestamp`,
  `TestPostModeEnterFastPathStillVisitsRSTBandFreqWithoutAContest`,
  `TestPostModeSlotHiddenWhileEditingQSO`).
- ✅ **In-app HELP** (`Ctrl+G`, reachable from any screen). `helpScreen`
  (`main.go`), `openHelpPanel`/`updateHelpPanel`/`helpPanelView` — static
  reference covering every screen hotkey, QSO Entry field/editing keys, and
  the as-you-type contest tools (analysis panel, Check Partial, rate meter,
  zone auto-fill); Esc/F1/Ctrl+G returns to whichever screen Ctrl+G was
  pressed from (`model.helpReturnScreen`), not always QSO Entry, since Help
  is global rather than contest-scoped like the other panels.

## 4. Later / hardware-bound / niche

- ⏳ **Band map** (seed from `cluster.go`; full value needs rig control).
- ⏳ **Rig control (CAT)** — band/mode sync, frequency to log, F11/F12, QSY memory.
- ⏳ **CW keyer + ESM + WinKey** — 8 memories w/ tokens, Enter-Sends-Message
  Run/S&P, speed/weight/QRS, cut numbers. Hardware I/O.
- ⏳ **Voice keyer** (`.WAV` F1–F8) — out of scope for a TUI; noted only.
- ⏳ **WAE QTC** send/receive/log · **IOTA** island-ref mults · **Sked/reminder** (`.MMO`).
- ⏳ **CTY refresh tooling** (cty.dat is embedded today).

Not applicable (SD is a Windows console app on wine; this app is native Go):
install / Legacy-Console-Host / colors / codepage / `SD.MAP` keyboard sections.

## 5. Open decisions (need your call)

1. ✅ **Curated vs generated duplicates** — resolved: curated wins on a
   straight 1:1 token duplicate; generated side-variant splits of one curated
   entry are kept (see §2 above).
2. ❓ **Data licensing** — which roster / SCP / master-call lists may be bundled
   (CWops, FOC, `MASTER.DTA`) and under what terms?
3. ❓ **SSB / mixed** — mark mixed events CW-only, or invest in SSB logging?
4. ❓ **Prefs surface** — extend the station profile vs a new app-prefs store for
   distance units, autofill on/off, dupe beep, etc.?
5. ❓ **Scope of "all SD features"** — recommend Phases 1–3 as the real target;
   treat keyer/rig/voice/band-map/WAE/IOTA/skeds as Later.

## 6. Cross-cutting engineering rules

- **Concurrency:** contest and entry state are mutated only in the Bubble Tea
  `Update` loop; async enrichment posts a `tea.Msg` applied on the loop — never
  from a goroutine.
- **Correctness on edit:** anything caching worked/mult/score state must fully
  recompute on QSO edit/delete (the SD "intelligent corrections" invariant).
- **Data hygiene:** no proprietary SD reference data in the repo; only factual
  contest parameters and independently-sourced/licensed reference data.
- **Testing:** pure math (heading, scoring, roster parse) table-driven; `Update`
  driven per-keystroke for as-you-type panels; edit-recompute covered hardest.

---

# Part 2 — Design detail (appendices)

## Appendix A — What already exists (reuse, don't rebuild)

| Capability | Where | Reuse for |
|---|---|---|
| DXCC / CQ / ITU / continent by prefix (longest-match, `=CALL`, `/` handling) | `dxcc.go` `dxccTable.lookup` | country analysis, mult flag, zone auto-fill |
| Per-entity **lat/lon** in cty.dat — *parsed but discarded* (`dxcc.go:210`) | `dxcc.go` | beam heading + distance |
| Maidenhead → lat/lon | `grid.go` `ParseGridSquare` | beam heading from received grid |
| Station's own lat/lon + grid | `station.go` | beam heading origin |
| Per-keystroke hook on Call field | `main.go` `checkDupe()` on `focusIdx==fieldCall` | analysis-engine trigger |
| Dupe check w/ contest/session scope | `store.isDupe`, `events.go` `dupe_scope` | real-time + partial dupe |
| Event catalog (bands, sessions, scoring, serial, omit-RST, cabrillo_contest) | `events.go`, `events/*.json` | contest rules (SD `.TPL`) |
| Cabrillo export + per-session scoring + running serial | `cabrillo_export.go`, `main.go` | SDCHECK-equiv output, score seed |
| Cluster spots (CW, all bands) | `cluster.go`, DX Spots panel | band map seed |
| QRZ lookup (async, as-you-type) | `qrz_lookup.go` | name/QTH prefill alongside `.LST` |
| ADIF import/export | `adif_*.go` | SDCHECK ADIF/CSV export |

**Takeaway:** the country/zone/continent brain and the geographic inputs already
exist. Most work is (a) a resolver that also returns lat/lon, (b) the in-memory
`contestState` index, and (c) TUI panels — not new radio science.

## Appendix B — Requested real-time features → design

1. **Real-time dupe (per char).** Fire from 2 chars; distinguish confirmed dupe
   (color + optional beep) vs partial/worked-before (feeds Check Partial). Ignore
   `/P /M /A /MM /AM /QRP` (reuse `portableCallSuffixes` in `dxcc.go`). `SETDUPE`
   resets baseline for multi-period contests.
2. **Country/Band analysis.** On prefix resolve show country + CQ/ITU + continent
   and worked/needed bands; black out non-workable entities for the event.
3. **Check Partial (prefix + suffix).** Live list of prior calls containing the
   fragment; bold=new-this-band, dim=dupe, distinct color=multiplier. `.`=suffix
   mode, `,`=back. ↑ into panel, Enter pulls a non-dupe call.
4. **Beam heading + distance.** `heading.go` great-circle from station lat/lon to
   target lat/lon (received grid, else cty.dat entity lat/lon). Units via prefs
   (`DISTUNit` km/mi).
5. **Advance multiplier flag.** Before the call completes, flag would-be new mult
   per mult type and the double-mult case (CQ WW zone+country). Backed by the index.
6. **Band/Mode matrix by call.** One-line per-band (per-mode in mixed) worked map.
7. **Linked database (`.LST`).** `roster.go` bidirectional (call→#, #→call),
   prefill received exchange; auto-load bundled roster per event. Superset of SCP.
8. **Auto data insert (zones/area codes).** Prefill from `dxcc.go` (+ `.mlt` for
   area contests); operator override carried forward (`AUTOFILL`/`NOFILL`).
9. **Countries Worked/Wanted by Continent.** Continent×band worked/needed panel.

## Appendix C — Core component: the `contestState` index

In-memory, rebuilt from the DB when a contest opens, updated on log/edit/delete.
Shared backend for every panel and for scoring; must stay consistent under SD's
"correct any QSO, recompute the whole log instantly" rule.

```
type contestState struct {
    byCall     map[string][]qsoRef              // Check Partial, band/mode matrix
    dxccByBand map[band]set[dxccNumber]          // mult/needed
    zoneByBand map[band]set[zone]
    areaByBand map[band]set[areaCode]
    worked     set[workedKey]                    // dupe + "worked this band"
    contByBand map[continent]map[band]counts     // worked/needed by continent
}
```

Rules: rebuild-on-open, incremental-on-log, **full recompute on edit/delete**
(logs are ≤ a few thousand QSOs → milliseconds). Lives on the `model`; mutated
only in `Update`. `computeContestScore` reads the index so header score, rate
meter, and panels agree.

**Multiplier/points schema (extend `events.go`)** mirrors SD `.TPL` params:
```json
"multipliers": [{"kind":"dxcc","per":"band"}, {"kind":"cqzone","per":"band"}],
"points": {"same_country":0,"same_continent":1,"other_continent":3}
```
covering `MULTSCOUNT` (Band/Once), `MULTSBOTH` (per-mode), `NON-AREA-MULTS`,
`POINTSCW/POINTSSSB`, `POINTSAREA`, `MAXBAND/MINBAND`, `MIXED`, `TIMES`,
`WORKBOTH`, `QSOPARTY`. Area codes from `data/mults/<event>.mlt` (SD `.MLT`).

**Enabling change in `dxcc.go`:** add `Latitude, Longitude` to `dxccEntity`,
parse header fields 5/6 and the per-alias `<lat/lon>` override, normalize
cty.dat's west-positive longitude to east-positive (with a test). Additive;
zero-value lat/lon → skip heading.

**UI:** keep QSO Entry; add a right-hand analysis column that renders only when a
contest is active and there's room (reuse the `dxSpotsPanel` width-gating that
already degrades on narrow terminals). Everything display-only and
keystroke-driven — no new modal for the core flow.

```
┌ QSO Entry ──────────────────────────────┐ ┌ Analysis (contest) ─────────┐
│ CW-OPEN-1 | 20M | UTC .. | Sending # 007 │ │ DL Germany  CQ14 ITU28 EU   │
│ Call [DL1AB_] RST[599] Rcv#[042] Name[..] │ │ Bearing 034° 6390 km        │
│ DUPE / NEW MULT (DXCC)                    │ │ Worked:20✓ Need:15 10 40    │
│ Recent QSOs        DX Spots               │ │ Partial: DL1ABC DL1AB* ...  │
└──────────────────────────────────────────┘ └ Roster: Hans #1423 ─────────┘
Rate: L10 42.0  L100 38.5  All 39.2  Q/Mult 3.6         Worked/Needed by cont → F-key
```

## Appendix D — Full SD manual feature catalog (English)

Legend: **Have** · **Core** (Phases 1–3) · **Later** · **Skip**.

**Logging & entry model**
- Enter accepts field/advances; Tab accepts without logging — **Have/Core**
- Serial send + running number; cut numbers (`CWZERO`) — **Have** / **Core** (cut nums at send)
- RST default 599/59; edit rcvd RST via empty-serial Enter — **Have** / **Core**
- POST (after-contest) entry with date prompt — **Core**
- Insert/Overwrite + AutoInsert (INS/OVR/AI) — **Core**
- `"` repeat previous call; quick same-call QSY — **Later**

**Real-time analysis (headline)** — all **Core** (Appendix B): dupe, partial/suffix
check, country/zone, beam heading+distance, advance mult (incl. double), band/mode
matrix, roster `.LST`, zone/area auto-fill, worked/needed by continent, SCP highlight.

**Editing / corrections**
- Correct **any** prior QSO with instant log-wide recompute — **Core**
- `ZAP` last; `/Z` mark older; `/X` X-QSO (logged, unscored) — **Core**
- ↑/↓ navigate log; `call+F9` list a call's QSOs — **Have** / **Core**

**Multipliers & score**
- Area (`.MLT`) + country/zone (`.CTY`) mults; per-band/per-mode; `TIMES` bonus — **Core**
- Live worked/needed grids; F1/F2 across bands — **Core**
- Rate meter (Q/hr L10/L100/overall, Q/Mult, 5-s refresh) — **Core**

**Band map** — per-freq call memory; worked vs new color; rig proximity ≤300 Hz
(`THRESHOLD`); F10 show; double-F10 return to run freq — **Later**

**Callsign databases** — `.LST` rosters (bidirectional, prefill) incl.
FOC/CWOPS/RDA/INORC — **Core**; Super Check Partial `MASTER.DTA` highlight,
prefix/suffix/all views — **Core**

**Skeds/reminders** — `.MMO` timed alerts + notes; F7 set, F8 next 7 — **Later**

**CW / keyer** — 8 memories w/ tokens (`#R #C #S #T #P #N #E #B`, `<`/`>`, `^`);
ESM Run/S&P + `'` toggle + auto-CQ (`CQTIMER`); internal/WinKey; PTT lead/tail;
QRS; calibrate; cut numbers — **Later/Skip** (hardware)

**Rig control** — CAT band/mode sync; freq to log; F11/F12; QSY memory — **Later**

**Voice keyer** — `.WAV` F1–F8, DVR — **Skip** (hardware)

**Special contests** — WAE **QTC** — **Later**; IOTA island-ref mults — **Later**;
DXpedition/special-event templates — **Core** (data-driven)

**Files/config/output**
- Text log (`.ALL`), audit/backup (`.AUD`), power-loss safe — **Have** (SQLite +
  atomic writes + backup); document equivalence
- SDCHECK: **Cabrillo `.LOG`**, **ADIF**, **CSV** — **Have** (Cab/ADIF/CSV, per-session names)
- Update CTY files — **Later**
- `.TPL` templates / 290+ contests — **Have** pattern (`events/*.json`); grow — **Core/ongoing**
- SD.INI prefs (colors, units, autofill, beep, ESM) — **Core** (map to prefs)
- Colors / `EXPAND` / `SD.MAP` / codepage — **Skip/Later** (terminal handles most)
- Backups `DUMP`/`BACKUP` — **Have** · Help (`HELP.TXT`) — **Core**

**Platform note (informational):** SD is a Windows console app run on Linux via
wine; this app is native cross-platform Go, so the manual's Windows-console /
Legacy-Console-Host / install sections are **not applicable**.

## Appendix E — Testing strategy

- **Pure functions:** `heading.go` (known city pairs, antipode, zero-distance,
  hemisphere signs); cty.dat lat/lon parse (incl. west→east); roster bidirectional
  parse; mult/points per event.
- **`contestState` correctness:** table-driven — log a scripted set, assert
  worked/needed/dupe/mult/double-mult and score; then **edit** a QSO's
  call/band/zone and assert the *entire* index + score recompute (hardest case).
- **As-you-type:** drive `Update` with per-character `KeyMsg`s and assert panel
  state at 2, 3, N chars (extends existing serial/header/inline-exchange tests).
- **Concurrency:** index mutated only in `Update`; async enrichment via `tea.Msg`.
