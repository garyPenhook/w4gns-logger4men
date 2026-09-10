# Award-stats independent review — September 10, 2026

An independent review of the LoTW-derived award statistics (DXCC/WAS/WAZ/VUCC/IOTA panel) found no critical security vulnerability or data-loss defect, but concluded the totals are useful rough analytics, not authoritative sponsor-rule eligibility counts. It reported 2 high, 3 medium, and 3 low findings.

**The 2 high findings are fixed** (commit `0b43e13`): award totals now join confirmed rows to their matched QSO and drop unmatched confirmations (previously `Confirmed` could exceed `Worked`, contradicting the README), and every award query scopes to the active profile's `station_callsign` (blank/legacy rows still included) so a profile logged under more than one callsign no longer has incompatible identities silently combined. See `lotw_stats.go`, `lotw_stats_test.go`.

**4 of the remaining 6 findings are now fixed (2026-09-10, follow-up pass):**

- **Terminal control-sequence injection** — `stats_panel.go` now routes both `progress.Needed` labels and `m.statsSyncMsg` through `sanitizeClusterText` before rendering.
- **60m/WAS exclusion** — `wasWorkedQuery`/`wasConfirmedQuery` in `lotw_stats.go` now exclude `band = '60M'`.
- **Invalid award keys** — the WAZ confirmed-side query now normalizes `lotw_confirmation.cqz` (a TEXT column) the same way the worked-side `qso.cqz` (an INTEGER column) already was, so "05" and "5" collapse to one zone; the IOTA worked/confirmed queries now require the standard `AA-###` form (`iotaReferenceFilterSQL`), so free text no longer counts as a distinct IOTA entity. This was deliberately done as a SQL-side filter, not a `validateQSO` rejection: an earlier version of this fix added the check to `validateQSO`, which also gates the interactive edit path — a pre-existing row with a non-canonical `iota_ref` (e.g. backfilled from LoTW via `backfillQSOGeographyFromConfirmations`, which never calls `validateQSO`) would have permanently failed every edit to that row, even edits unrelated to IOTA. The `dxccNumber`-range half of this finding (a numeric-but-nonexistent DXCC entity number) is still open — see below.
- **Doc/workflow drift** — `.github/pull_request_template.md`'s build-verification step now targets `./cmd/w4gns-logger`; `docs/ROADMAP.md`'s SD-contest counts corrected from 271 (261 entry-aware) to the actual 269 (259 entry-aware).
- **LoTW sync memory bound** — `syncLoTWConfirmations` now caps buffered records at `maxLoTWReportRecords` (500,000) before validating/committing, guarding against unbounded memory growth from a runaway or malformed response.

Tests: `TestStatsPanelViewSanitizesUntrustedText`, `TestLoadLoTWAwardStatsWASExcludes60Meters`, `TestLoadLoTWAwardStatsWAZNormalizesConfirmedZonePadding`, `TestLoadLoTWAwardStatsIOTAExcludesMalformedReferences`.

The remaining 2 findings (1 medium remainder — malformed `dxccNumber` — plus the award-eligibility-modeling medium finding, and the 2 low findings about `main.go` concentration and floating CI tool versions) are **not yet fixed** and are recorded below so they aren't lost. None are security-critical; all affect correctness/accuracy of the award panel or maintainability.

## Open findings

### Medium — Award-specific eligibility is incompletely modeled

**Location:** `cmd/w4gns-logger/lotw_stats.go` (the `*WorkedQuery`/`*ConfirmedQuery` constants).

| Award | Correctly implemented | Missing or incorrect |
|---|---|---|
| DXCC | Distinct nonzero DXCC entities | Same-originating-entity rule, November 15 1945 cutoff, award category/mode/band, current-vs-deleted entity status |
| WAS | US/Alaska/Hawaii entity scope, valid state list, DC→MD fold | 60 m is counted even though ARRL excludes it from general WAS; 50-mile operating-area rule; Alaska/Hawaii historical cutoffs; specialty-award mode/band qualification |
| WAZ | Restricts numeric zones to 1–40 | Same-originating-DXCC-entity rule; category/account separation |
| VUCC | 6 m only, four-character grid count | January 1 1983 cutoff; 200 km operating-area rule |
| IOTA | Distinct nonblank references | November 15 1945 cutoff; same-entity/land-based rule; HF vs. VHF category separation; continent qualification |

The 60 m/WAS gap is now fixed (see above): the app supports 60 m in `bandplan.go`, and ARRL's WAS rules exclude it from the general award. Separately, `lotw_query.go`'s sync stores `CREDIT_GRANTED` (`credit_granted` column, populated at `lotw_query.go:432`) but no stat consults it, and the sync doesn't retain `APP_LoTW_MODEGROUP`, `APP_LoTW_2xQSL`, or entity status — so "LoTW-confirmed" here means "a QSL exists," not "award credit granted." This part is still open.

**Correction:** Either narrow the panel's framing (e.g. label it "rough progress, not sponsor-certified") or incrementally add the missing rules per award, consuming `credit_granted` before mode/mixed-category rules (which need more schema).

### Medium — Imported data can create invalid award keys (partially fixed)

**Location:** `cmd/w4gns-logger/adif_import.go:152` (`cqZone`) and `:158` (`iotaRef`) accept trimmed text with no format check.

~~Effects: any nonblank string can count as an IOTA reference; WAZ values like `"5"` and `"05"` both pass the existing `BETWEEN 1 AND 40` range check in `lotw_stats.go` but remain separate distinct keys~~ — fixed: `lotw_stats.go`'s IOTA worked/confirmed queries now require the standard `AA-###` form (`iotaReferenceFilterSQL`), and the WAZ confirmed-side query normalizes `lotw_confirmation.cqz` (TEXT) the same way `qso.cqz` (INTEGER) already was, so zero-padded zones collapse to one key. Deliberately fixed as a SQL-side filter rather than a `validateQSO` rejection — see the note in the summary above about why a hard rejection there would have broken editing pre-existing rows.

Still open: **a malformed but nonzero imported `dxccNumber` can become a spurious "entity."** Unlike `cqZone`/`iotaRef`, this can't be fixed with a simple format check — a garbage-but-numeric DXCC number (e.g. `"9999"`) is syntactically valid and would need a reverse lookup against the DXCC entity table to detect.

**Correction:** Validate `dxccNumber` against the known DXCC entity table (same table `dxcc.go`'s callsign lookup already loads) at import time, or filter `dxccWorkedQuery`/`dxccConfirmedQuery` to only entity numbers present in that table.

### ~~Medium — Terminal control-sequence injection in the stats panel~~ (fixed)

**Location:** `cmd/w4gns-logger/stats_panel.go` (the needed-list render loop and `m.statsSyncMsg`).

Fixed: both `progress.Needed` entries and `m.statsSyncMsg` are now routed through `sanitizeClusterText` before rendering, the same as the cluster spot panel. See `TestStatsPanelViewSanitizesUntrustedText`.

### ~~Low — Initial LoTW synchronization has no total response bound~~ (fixed)

**Location:** `cmd/w4gns-logger/lotw_query.go`.

Fixed: `syncLoTWConfirmations` now caps buffered records at `maxLoTWReportRecords` (500,000) — generous relative to any real account history, but bounds peak memory against a runaway or malformed response. (A full streaming-into-transaction rewrite was considered but rejected: the existing design deliberately validates the whole response, including the `APP_LoTW_NUMREC` count check, before committing anything, so a truncated transfer can't leave a bookmark advanced past confirmations it never delivered — a record cap preserves that invariant more simply than restructuring the transaction boundary.)

### Low — Review and workflow documentation has drifted (partially fixed)

- ~~`.github/pull_request_template.md:9` — the build-verification checklist item targets `.` instead of `./cmd/w4gns-logger`~~ — fixed.
- ~~`docs/ROADMAP.md:155` and `:856`/`:878` — claims `sd_contests.json` has 271 events; the current file has 269~~ — fixed (261/259 entry-aware breakdown corrected too).
- CI (`.github/workflows/*.yml`) runs `staticcheck@latest` and `govulncheck@latest` — GitHub Actions themselves are SHA-pinned, but these two tools float, so a CI run today can fail (or newly pass) for reasons unrelated to the diff being tested. Still open.

**Correction:** Consider pinning `staticcheck`/`govulncheck` to specific versions with a periodic manual bump, trading build reproducibility for currency.

### Low — Core application remains highly concentrated

**Location:** `cmd/w4gns-logger/main.go` — 5,060 lines, most of the production application living in one `main` package.

Test coverage (76.6% overall, per the review's measurement) substantially compensates, but UI state, orchestration, and service-lifecycle changes all share one large regression surface. Not a defect, but a standing maintainability cost worth tracking.

**Correction:** No specific fix required; consider incremental extraction (e.g. splitting message-handling groups into separate files, which is already partly done — `stats_panel.go`, `rig.go`, etc. — vs. further splitting `main.go` itself) opportunistically alongside unrelated feature work, not as a dedicated refactor.

## Contest-rule status (unchanged, not a defect)

Contest handling is already documented as non-certified: the README states scoring is an estimate and lists known omissions. The bundled SD catalog (269 definitions, corrected count above) has no scoring-ready entries, and many catalog events remain entry-only. Contest dates/exchanges/scoring cannot be treated as globally sponsor-compliant; only individually audited capability branches should be relied upon.

## Test-coverage gap noted by the review

`cmd/w4gns-logger/lotw_stats_test.go` had only two tests before this pass, covering none of: location/identity grouping, date cutoffs, the 60 m WAS exclusion, unmatched confirmations, official credit metadata (`credit_granted`), malformed identifiers, or IOTA category separation. The high-severity fixes added two more tests (`TestLoadLoTWAwardStatsExcludesUnmatchedConfirmations`, `TestLoadLoTWAwardStatsScopesToStationCallsign`); this pass added four more (`TestStatsPanelViewSanitizesUntrustedText`, `TestLoadLoTWAwardStatsWASExcludes60Meters`, `TestLoadLoTWAwardStatsWAZNormalizesConfirmedZonePadding`, `TestLoadLoTWAwardStatsIOTAExcludesMalformedReferences`). Remaining gaps (date cutoffs, `credit_granted`, DXCC entity-number validation, category/mode qualification) track the still-open findings above and should gain coverage alongside each fix.
