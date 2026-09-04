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
- ✅ **Worked/Needed by continent** panel; per-band paging.
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
- ⏳ Area-mult `.mlt` tables + data-driven **multiplier/points schema** in `events.go`
  (`MULTSCOUNT`, `MULTSBOTH`, `POINTSAREA`, `MAXBAND/MINBAND`, `TIMES`, …).
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
