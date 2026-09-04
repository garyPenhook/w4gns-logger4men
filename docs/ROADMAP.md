# w4gns-logger — Contest features roadmap & design

Single source of truth for everything we've changed and everything still to
build to make this a real contest logger, using SD (EI5DI) as the feature
benchmark. Part 1 is the tracker; the appendices hold the detailed design.

- **Scope:** CW contest logging. The app logs CW only today.
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
- ⏳ **Real sessions/schedules** for multi-session contests needing per-session
  dupe scope and per-session Cabrillo files. Start with the ones people run.
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
- ⏳ **Check Partial** panel (prefix + `.`/`,` suffix), ↑ to pull a call.
- ⏳ **Super Check Partial** highlight (known-good calls).
- ⏳ **Worked/Needed by continent** panel; per-band paging.
- ⏳ **Rate meter** (Q/hr last-10 / last-100 / overall, Q/Mult).

**Phase 3 — corrections, mult data, output parity**
- ⏳ Log-wide **recompute on any edit** (dupes/mults/points) — SD's differentiator.
- ⏳ `/Z` (mark old for delete), `/X` (logged-but-unscored), `ZAP`, `SETDUPE`.
- ⏳ Area-mult `.mlt` tables + data-driven **multiplier/points schema** in `events.go`
  (`MULTSCOUNT`, `MULTSBOTH`, `POINTSAREA`, `MAXBAND/MINBAND`, `TIMES`, …).
- ⏳ SDCHECK parity: **CSV export**, per-session Cabrillo filenames (`CALL1.log`),
  **POST** (after-contest) entry mode.
- ⏳ In-app **HELP** for the new commands.

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
- SDCHECK: **Cabrillo `.LOG`**, **ADIF**, **CSV** — **Have** (Cab/ADIF) / **Core** (CSV, per-session names)
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
