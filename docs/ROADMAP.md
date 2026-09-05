# w4gns-logger — App review backlog & contest roadmap

Single source of truth for everything we've changed and everything still to
build to make this a real contest logger, using SD (EI5DI) as the feature
benchmark. Part 1 is the tracker; the appendices hold the detailed design.

- **Scope:** the code-review backlog covers the entire app; the feature
  tracker and design appendices below it focus on the *contest* feature set.
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

## Shared state QSO party support (2026-09-05)

Design and rollout: [State QSO parties](State_QSO_Parties.md).
Shared parsing, persisted locations, county-aware duplicates, county-line credit,
entrant-side scoring, bonuses/power factors, and checked CW exports are implemented
for Tennessee, California, Michigan, Ohio, Georgia, Florida, Alabama, and Iowa.
See the linked document for verified years, sources, and remaining limitations.
Other catalog parties still require individual rule audits; regional and future
rule editions must not inherit these definitions without verification.

Completed: Station Setup category persistence, sponsor county tables, explicit
operating periods, county-pair duplicate and scoring credit, export expansion,
and shared live/export scoring. Validation passed: `go test ./...`,
`go vet ./...`, `go test -race -short ./...`, and `git diff --check`.

Remaining: audit each other catalog party, model multi-state regions, add
edition-specific changes without applying future rules to old logs, and extend
sponsor category/submission metadata where needed. Maximum operator on-time,
physical location eligibility, and category qualification are not adjudicated.
Earlier tracker entries below record the implementation at their stated dates;
this section and the linked support document supersede their state-party scope.

## 0A. Full-app code review (2026-09-05)

Reviewed commit `19f41e9`: application entry/edit workflows, SQLite schema and
migrations, import/export, contest configuration and scoring, network clients,
upload outbox, backups, terminal/platform helpers, tests, build and release
workflows, and documentation. The original review changed documentation only;
the follow-up implementation is recorded below. The
2026-09-04 backlog below remains historical; its completed labels do not close
the newly identified cases here.

Validation: `go test ./...`, `go vet ./...`, `go test -race -short ./...`, and
`govulncheck` all passed with Go 1.27.0; the vulnerability scan reported no
vulnerabilities. All five release targets built with CGO disabled:
Linux amd64/arm64, Windows amd64, macOS amd64/arm64. Temporary local probes
reproduced the specific results called out below and were removed afterward.
No production database, credentials, remote logbook, or Drive backup was
modified. Native Windows/macOS behavior, interactive terminal usability, live
service acceptance, and a sponsor-by-sponsor audit of every catalog record
remain unverified. Passing the existing tests does not establish those claims.

All 28 findings below have implementation changes and regression coverage or
documentation/CI changes, as applicable. R18 now validates exchange grammar
for every catalog event with a checked Cabrillo layout. Checked means the correction described
in the resolution summary landed, not that every broader acceptance scenario
or supported platform was manually certified. The original defect descriptions
and pre-fix line references are retained for audit history.
P1 means log integrity, delivery loss, or
incorrect contest submission; P2 means functional correctness, interoperability,
or reliability; P3 means documentation and verification gaps. Paths are relative
to the repository; function names identify the relevant implementation.

### Implementation resolution (2026-09-05, unreleased)

- R01/R06: unlimited edit input length, preservation of untouched multiline
  fields, and actual received-RST defaults; long-field edit/save regressions.
- R02/R03/R21: date-qualified occurrences, transactional legacy-ID migration,
  bare/standard event-ID resolution, profile-specific selection/exchange
  restoration, and serial resume from stored contacts. Annual, monthly, and
  weekly/daily suffixes are date-based, not a schedule engine; ambiguous
  multi-session third-party imports require operator mapping. Unknown IDs
  remain unchanged. Serial read failures block new saves rather than guessing.
- R04/R09/R10: persisted station grids, received CQ-zone multiplier context,
  and scoring duplicate keys aligned with event scope. Regression tests rebuild
  scores and export persisted contacts rather than testing only helpers.
- R05/R18: structured Sweepstakes output, explicit errors instead of clipping
  oversized calls/exchanges, and submission-time required-field, CW RST, serial,
  CQ-zone and IARU exchange validation, extended to every checked catalog layout.
  Submission validates the complete emitted exchange, with dated IARU society
  codes and country-aware location/power checks. Incomplete local logging remains
  allowed. Other layouts retain the generic 13-character exchange limit;
  token validation does not certify membership, station location, or eligibility.
- R07/R08/R25: missing/changed credentials pause rather than delete deliveries;
  only confirmed missing QSOs are discarded. Destination fingerprints, visible
  queue/error counts, a hard automatic retry cap, and explicit Ctrl+U retry/
  reassignment provide recovery. Active leases survive manual retry. Legacy
  unbound rows bind at first send. Delivery remains at-least-once: a crash after
  remote acceptance but before local acknowledgement can require reconciliation.
- R11–R15/R19/R20: refresh state after partial imports, count only committed
  batches, preserve app metadata, infer midnight TIME_OFF dates, stream ignored
  free text, check cancellation throughout parsing, reject nonregular inputs,
  split extended grids into ADIF fields, and encode all output fields as ASCII.
  ADIF is not lossless Unicode storage; database backups preserve original text.
- R16/R17/R24: exclusive filename reservations stop filesystem-error loops;
  ADIF/Cabrillo/CSV use consistent read snapshots, with a separate connection
  for file databases so slow writers do not occupy the logging connection.
  Synchronous contest-index rebuilds remain a possible large-log UI pause.
- R22/R23: stale QRZ credential sessions are ignored; POTA lookup results are
  correlated to the saved contact and park-reference matches.
- R26: headless/SSH fallback, detection of immediate terminal process failure,
  and embedded timezone data. There is no child-ready handshake, so delayed
  emulator failures remain a limitation; use `--in-current-terminal` if needed.
- R27/R28: README/changelog reconciled, stale capability counts removed, known
  scoring limitations shown in the catalog, persistence/UI/export regressions
  added, and native-platform smoke plus vulnerability gates wired into releases.

Verification limits: no production database or real uploads were exercised.
Local verification passed: full tests (including the 100,000-contact import),
`go vet`, short race suite, vulnerability scan (no vulnerabilities), and all
five CGO-disabled release builds. One full-test run exceeded the import timing
budget while five cross-builds competed for CPU; the isolated full rerun passed
in 6.6 seconds. Regression coverage is in `review_regression_test.go` and
`review_terminal_linux_test.go`, alongside existing updated tests.
Native Windows/macOS smoke runs must complete in CI; cross-compilation alone
does not establish their runtime correctness. No new sponsor-by-sponsor audit
or interactive terminal acceptance session was performed. Existing station-side
scoring gaps are explicitly documented rather than marked implemented.

### P1 — fix before relying on contest submissions or editing imported logs

- [x] **R01 — Editing silently truncates existing QSO data.**
  `cmd/w4gns-logger/main.go:459` (`newTextInput`, `beginEditQSO`,
  `logCurrentQSO`) gives every ordinary input a 20-character limit, including
  names, emails, park names, notes, and exchange text. `SetValue` truncates
  imported values when opening the editor, and saving any correction writes
  all these shortened values back. Probe: an 80-character note became 20
  characters on opening the edit form. Separate display width from field
  limits and preserve existing full values. Acceptance: import long fields,
  edit only RST, and verify every untouched field remains byte-for-byte intact.

- [x] **R02 — Recurring contests share one permanent session identity.**
  `main.go:1733` (`selectEvent`), `events/cwops.json`, and
  `store.go:623` (`isDupe`) under `cmd/w4gns-logger/` use IDs such as
  `CWT-1900` and `CW-OPEN-1` without an occurrence date/year. Next week's CWT
  reuses last week's dupes, score, and export selection. Non-session dupe
  queries additionally match every `eventID-%`, so merely adding a date suffix
  will not fix their scope. Introduce a persisted event occurrence/session
  identity and migrate existing contacts using timestamps. Acceptance: the
  same station can be worked in successive occurrences, and each export
  contains only its occurrence's contacts.

- [x] **R03 — Reselecting or resuming a contest restarts sent serials at 001.**
  `cmd/w4gns-logger/main.go:1733` (`selectEvent`) assigns `nextSerial = 1`
  without reading the log; startup also forgets the active contest. Probe:
  reselection after setting the next serial to 42 displayed `001`. Resume the
  persisted occurrence's sequence, accounting for manually sent numbers and
  edits. Acceptance: switch away/back and restart after logging serial 041;
  the next contact must send 042 without manual repair.

- [x] **R04 — Rebuilt/exported distance scores lose the station grid.**
  `cmd/w4gns-logger/cabrillo_export.go:335` (`forEachQSOForContest`) does not
  select `my_gridsquare`, while `contest_state.go:370` (`record`) requires
  `q.myGridSquare` for distance points. Probe: an EM75-to-FN31 Stew Perry QSO
  scored 3 points incrementally and 0 after rebuilding from SQLite. Load the
  complete scoring inputs, preserving the per-QSO station snapshot.
  Acceptance: live, rebuilt, and exported scores agree after logging, editing,
  importing, and restarting. Distance-based scoring is part of the
  [sponsor's published description](https://www.kkn.net/stew/).

- [x] **R05 — Generic Cabrillo exchange columns destroy valid exchanges.**
  `cmd/w4gns-logger/cabrillo_export.go:191` (`cabrilloQSOLine`) truncates the
  combined serial/text exchange to 13 characters for all promoted contests.
  Probe: `001 B W4GNS 76 TN` became `001 B W4GNS 7`, losing the section and
  part of the check. Sweepstakes also needs its own field arrangement: its
  catalog hint includes the callsign inside the exchange, while the generic
  writer emits a separate callsign column too. Implement structured per-event
  layouts and reject unsupported/overlong submissions instead of silently
  clipping them. Acceptance: golden fixtures preserve every field and match
  the [ARRL Sweepstakes example](https://lotw.arrl.org/lotw-help/pref-cab/).

- [x] **R06 — Contest fast entry saves blank received RST despite showing 599.**
  `cmd/w4gns-logger/main.go:467` (`initialModel`), `clearQSOForm`, and the
  contest Enter fast path use `599` only as the received field's placeholder.
  Its value is empty initially and after every save, while Enter skips it.
  Probe confirmed value `""` with placeholder `"599"`. RST-bearing Cabrillo
  exports consequently contain a blank report. Set a real default for events
  that exchange RST and validate required reports. Acceptance: log two
  consecutive contacts using the advertised fast path and verify both reports
  in SQLite and export.

- [x] **R07 — Missing credentials permanently discard pending uploads.**
  `cmd/w4gns-logger/main.go:2173` (`drainOutbox`) deletes a delivery when its
  upload command returns nil for a blank API key. A temporarily absent
  environment variable, unreadable key file, or different launch directory
  therefore erases durable work. Probe: a due QRZ row became zero pending rows
  with no credentials and no upload. Keep these deliveries paused, visibly
  explain the missing configuration, and resume when restored. Acceptance:
  restart without a key, then restore it; the original delivery still runs.

- [x] **R08 — Any QSO read error is treated as a deleted contact.**
  `cmd/w4gns-logger/main.go:2181` (`drainOutbox`) calls `markUploadDone` for
  every `qsoByID` error, including scan, locking, and I/O errors. Only a
  confirmed missing row justifies removing the delivery. Distinguish wrapped
  `sql.ErrNoRows` from recoverable failures, persist/report retry errors, and
  handle failed queue writes. Acceptance: inject a read failure while queue
  writes remain available and verify the row survives for retry.

- [x] **R09 — CQ-zone corrections do not affect the multiplier score.**
  `cmd/w4gns-logger/contest_state.go:386` (`record`) populates CQ-zone
  multipliers from stored/callsign-derived entity context rather than the
  copied `srxString` used in the submission. Editing the received zone never
  changes `q.cqZone`. Probe: W1AW with received exchange `04` still recorded
  zone 5. Define event-specific exchange precedence and use it consistently
  in scoring and NEW MULT previews. Acceptance: override an autofilled zone
  and verify the new value affects both live and rebuilt multiplier sets.

- [x] **R10 — Scoring ignores the event's duplicate key.**
  `cmd/w4gns-logger/contest_state.go:334` (`record`, `score`) always counts
  distinct call/band keys, even when `DupeScope` is `call`. Imported
  Sweepstakes contacts with the same call on two bands therefore score twice;
  the probe produced 4 points instead of the one-contact 2-point value.
  Duplicate rows can also add conflicting exchange multipliers because all
  rows populate multiplier sets even when points are deduplicated. Pass the
  event's eligibility/duplicate policy into scoring and choose the credited
  QSO deterministically. Acceptance: imported duplicates, corrected exchanges,
  and `/X` records follow one consistent scoring policy.

### P2 — interoperability and runtime correctness

- [x] **R11 — Import completion leaves contest analysis stale.**
  `cmd/w4gns-logger/main.go:2558` (`adifImportedMsg` handler) refreshes the
  table after success but never rebuilds the contest index. On partial failure
  it refreshes neither, despite already committed batches. Probe: a newly
  inserted second call left the index at one call after the completion
  message. Refresh count/table/index and dupe state after any committed work;
  report partial counts on both CLI and TUI errors. Acceptance: successful
  and mid-file-failing imports update all visible totals and scoring.

- [x] **R12 — ADIF round trips lose contest sessions and app metadata.**
  `cmd/w4gns-logger/adif_export.go:63` (`adifContestID`,
  `forEachQSOForProfile`) and `adif_import.go:79` (`qsoFromADI`) translate
  internal IDs on export but do not restore them on import. Probe:
  `CWT-1900` exports as `CWOPS-CWT`, then imports under that different ID;
  selecting CWT-1900 no longer finds it. `/X` and park-name metadata are also
  omitted. Preserve supported app metadata in application-defined ADIF fields
  alongside standard IDs, with an explicit mapping workflow for other logs.
  Acceptance: fresh-database export/import preserves occurrence membership,
  unscored flags, park names, and the corresponding score. SQLite backups
  remain the full-fidelity recovery format until then.

- [x] **R13 — Valid TIME_OFF without QSO_DATE_OFF is discarded.**
  `cmd/w4gns-logger/adif_import.go:105` (`qsoFromADI`) parses the end time
  only if an explicit end date exists. ADIF permits omission of that date,
  including a contact crossing midnight within 24 hours. Infer the end date
  from start/time-off and preserve valid durations. Acceptance: same-day and
  midnight-crossing fixtures match the
  [ADIF TIME_OFF definition](https://www.adif.org.uk/317/ADIF_317.htm#QSO_Field_TIME_OFF).

- [x] **R14 — The parser's tag limit also rejects ordinary free text.**
  `cmd/w4gns-logger/adif_import.go:233` (`parseADIRecords`) applies the
  256-byte tag limit while searching for the next `<`, so a long preamble or
  whitespace between records aborts an otherwise readable file. Probe: a
  350-byte plain-text preamble before `<EOH>` failed. Stream/discard text
  outside tags without retaining it, while retaining tag/field/record memory
  caps. Acceptance: long headers and inter-record whitespace use bounded
  memory and import successfully.

- [x] **R15 — ADIF import can prevent graceful shutdown indefinitely.**
  `cmd/w4gns-logger/main.go:984` (`importADIFFile`),
  `adif_import.go:29` (`importADIF`), and `main`'s `bgTasks.Wait` accept any
  filesystem object. Opening a FIFO without a writer blocks before the parser
  starts; a stalled reader or endless invalid records never reaches the
  database cancellation checks. Restrict interactive imports to regular files
  or make opening/reading cancellable, and check cancellation while parsing.
  Acceptance: quit during a blocked input and a long stream of skipped records;
  shutdown reaches its backup phase within a bounded time.

- [x] **R16 — Export filename selection loops forever on filesystem errors.**
  `cmd/w4gns-logger/main.go:961` (`nonCollidingPath`) treats every `Stat`
  error except not-exist as a name collision. EACCES, ENOTDIR, or a symlink
  loop therefore causes endless numbered probes; the tracked export never
  finishes and shutdown waits forever. Return unexpected errors and reserve
  the selected name safely. Acceptance: inaccessible/malformed destination
  paths produce a prompt error with no stuck background task.

- [x] **R17 — Cabrillo score and QSO lines use different database snapshots.**
  `cmd/w4gns-logger/cabrillo_export.go:257` (`exportCabrillo`) computes the
  score, releases that query, then reads the rows again. Logging or editing
  between these passes makes CLAIMED-SCORE disagree with the submitted rows.
  Export both from one read transaction or immutable snapshot. Acceptance:
  synchronize a write between the passes and verify a consistent submission.

- [x] **R18 — Invalid or missing exchanges can still be submitted as checked.**
  Implemented required-field, serial, CW RST, CQ-zone/IARU, and Sweepstakes
  validation plus overflow rejection. Follow-up: Helvetia, RDXC, and WAG now
  validate sent and received exchanges independently using the callsign's
  country (the stored station callsign takes precedence over the profile).
  Swiss cantons and Russian oblasts must match the existing allowed-code
  tables; German exchanges accept NM or alphanumeric DOK syntax, including
  digit-leading special DOKs. Other participants must supply a positive
  serial; Helvetia requires at least three digits. A code plus an extraneous
  serial fails instead of emitting both. Serial-only exchanges imported into
  the text field are accepted. Unknown callsign countries fail explicitly.
  Real-catalog SQLite-to-Cabrillo regressions cover both sides and rejection
  of invalid persisted exchanges in `contest_validation_test.go`.
  Follow-up verification passed: `go test ./... -count=1`, `go vet ./...`,
  `go test -race -short ./...`, and `git diff --check`.
  Sources: [USKA 2026 rules, section 2.5](https://uska.ch/wp-content/uploads/2026/03/2026-01-Rules-and-Regulations-for-Helvetia-Contest-issued-March-2026.pdf),
  [RDXC rules, sections 6 and 7.3](https://www.rdxc.org/rules_eng)
  (the live page currently identifies itself as 2027), and
  [DARC's published WAG rules, section 4](https://www.darc.de/fileadmin/filemounts/referate/conteste/contest/waedc/results/heft_erg_2016.pdf).
  The current WAG rules page returned HTTP 403; this follow-up does not certify
  current DOK assignments or membership. Callsign-country resolution retains
  the bundled prefix table's portable-call limitations.
  Completed follow-up: every checked catalog layout now validates the complete
  emitted exchange on both sides, including WPX/WAE/SAC/Oceania serials,
  CW Open serial/name, CWT name/member-number or location, SST name/location,
  TNQP county/state/province/DX, Stew Perry four-character grids, NAQP and
  Sprint names/locations, CQ 160 state/province or zone, and ARRL DX
  state/province or positive power (including power suffixes and CW cut digits).
  Extra serial/text tokens and non-printable/non-ASCII exchange input fail;
  signed or out-of-range zones cannot masquerade as valid zones. IARU HQ
  exchanges use an offline society/official-code allowlist dated 2026-09-05,
  rather than accepting arbitrary alphabetic text. Unknown event validators
  fail explicitly. Stew Perry permits omitted RST as its rules specify.
  `submission_regression_test.go` covers every checked catalog event through
  SQLite-to-Cabrillo export, invalid sent and received fields, token edge cases,
  and preservation of an existing export after rejection. Incomplete records
  remain locally editable. Grammar validation does not establish HQ authority,
  membership ownership, assigned DOKs, actual state/county, or contest eligibility;
  roster changes require updates, and non-Sweepstakes exchanges still have the
  existing 13-character output limit.
  Additional sources: [CW Open](https://cwops.org/cwops-tests/cw-open/),
  [CWT](https://cwops.org/cwops-tests/),
  [SST](https://www.k1usn.com/sst.html),
  [NAQP rules 10–11](https://ncjweb.com/NAQP-Rules.pdf),
  [Sprint rules 7–10](https://ncjweb.com/Sprint-Rules.pdf),
  [CQ 160 exchange rules](https://cq160.com/rules/index.htm),
  [ARRL DX exchange](https://www.arrl.org/arrl-dx),
  [Stew Perry rule 4](https://www.kkn.net/stew/stew.rules.txt), and
  [IARU member societies](https://www.iaru.org/reference/member-societies/).
  TNQP uses the existing catalog's county-code table; the live sponsor rules
  page presented a browser-verification challenge during this follow-up.
  Completion verification: `go test ./... -count=1`, `go vet ./...`,
  `go test -race -short ./...`, and `git diff --check` passed.
  `cmd/w4gns-logger/main.go:1970` (`logCurrentQSO`), `qso_validation.go:34`,
  and `cabrillo_export.go:257` validate general QSO fields but not the selected
  contest's required serial/exchange content. Blank serials and exchanges
  survive persistence and export; `iaru_zone.go` additionally treats `0` or
  `-1` as special HQ text and accepts arbitrarily large positive zones.
  Add event-specific validation, keeping incomplete records editable but
  identifying them before submission. Acceptance: missing required values and
  invalid zone/serial tokens cannot silently produce a checked submission.

- [x] **R19 — Supported long grids are emitted in the wrong ADIF fields.**
  `cmd/w4gns-logger/grid.go:19` accepts 10-character locators, but
  `adif_export.go:148` writes all ten characters into GRIDSQUARE or
  MY_GRIDSQUARE. ADIF splits characters beyond the first eight into the
  corresponding `_EXT` fields; the importer ignores these extension fields
  too. Split/reassemble them and preserve precision. Acceptance: a locator
  such as FN01MH42BQ survives a standards-compliant round trip. See
  [ADIF locator mapping](https://www.adif.org.uk/317/ADIF_317.htm).

- [x] **R20 — ASCII conversion is incomplete across ADIF output fields.**
  `cmd/w4gns-logger/adif_export.go:148` transliterates some free-text fields
  but writes COUNTRY, exchanges, and other strings directly. Unicode in a
  sent/received name exchange therefore bypasses the documented ASCII policy.
  `qso_validation.go:19` also allows Unicode letters/digits in callsigns,
  which Cabrillo later drops. Centralize field-type validation/encoding at
  export and constrain callsign characters appropriately. Acceptance: all
  emitted ADI fields satisfy their declared types with an explicit policy for
  unsupported characters. See the
  [ADIF data types and ADI format](https://www.adif.org.uk/317/ADIF_317.htm).

- [x] **R21 — Bare catalog event IDs fail event resolution.**
  `cmd/w4gns-logger/main.go:1771` (`eventForContestID`) accepts only
  `event.ID + "-"` prefixes, not equality or normalized surrounding whitespace.
  Probe: `CWT` did not resolve. Typing a known bare ID or editing an imported
  bare-ID contact falls back to casual dupes and loses contest-specific UI and
  export support. Normalize and resolve exact IDs, then explicitly select or
  derive an occurrence/session. Acceptance: exact, session-qualified, and
  standard imported IDs have predictable behavior without silent fallback.

- [x] **R22 — QRZ credential changes can restore a stale session.**
  `cmd/w4gns-logger/main.go:608` (`saveStationSetup`) clears the cached key,
  but the `qrzCallsignLookupMsg` handler assigns any returned session key
  before checking request validity. A lookup from the old account can arrive
  after saving new credentials and repopulate the old session. Associate
  requests/session keys with a credential generation and invalidate it on
  save. Acceptance: deliver the old account's result after credentials change;
  subsequent lookups authenticate using the new account only.

- [x] **R23 — POTA enrichment is not correlated to a particular contact.**
  `cmd/w4gns-logger/pota.go:41` (`potaLookupMsg`) and the corresponding
  `main.go` handler match only callsign. Results arriving after save are lost;
  an older lookup can instead fill a new/edit form for the same call. A park
  name can also be filled from a response whose reference differs from an
  already entered reference. Bind lookup results to the initiating QSO/form
  and keep reference/name as a consistent pair. Acceptance: delayed results,
  repeated calls, and operator-overridden park references do not cross-fill.

- [x] **R24 — Async exports still block interactive database operations.**
  `cmd/w4gns-logger/store.go:135` restricts SQLite to one connection;
  `adif_export.go:233` holds it while streaming the entire log to disk, while
  `main.go` performs dupe/history queries synchronously during input updates.
  A slow export stalls typing/logging despite running in a background command.
  Contest rebuilding is synchronous too. Use a carefully configured read
  connection/snapshot or move expensive work off the update loop. Acceptance:
  a deliberately slow export of a large log does not stall entry or quitting.

- [x] **R25 — Exhausted upload retries have no visible recovery workflow.**
  `cmd/w4gns-logger/outbox.go:168` (`uploadBackoff`) parks the twentieth
  failure a year ahead, but `main.go` still reports “will retry” and provides
  no queue inspection/reset action. Correcting credentials does not revive
  these rows promptly. Represent paused/exhausted states explicitly, show
  counts and last errors, and provide a retry action. Acceptance: reach the
  attempt limit, restart, correct configuration, and recover the delivery from
  the UI. Also bind queued destinations to the intended account/logbook so
  changing the global WRL logbook cannot silently reroute old queued contacts.

- [x] **R26 — Terminal launch can report success without starting the app.**
  `cmd/w4gns-logger/terminal.go:24` (`launchInOwnTerminal`) returns success
  as soon as `exec.Cmd.Start` succeeds. On a headless/SSH host an installed
  emulator can immediately exit because no graphical display is available;
  `main` has already returned and never reaches its current-terminal fallback.
  Detect unsuitable launch environments or verify startup failure before
  returning. Acceptance: a fake emulator that starts and immediately exits
  unsuccessfully leads to a visible error/current-terminal fallback.

### P3 — keep claims and verification aligned

- [x] **R27 — Existing documentation overstates completed behavior.**
  `README.md` and this roadmap's historical tracker disagree with current
  code and with each other: scoring counts/lists are stale, SAC is described
  both as not scored and scoring-ready, received RST is described as a default,
  retries are described as continuing until acceptance, and checked Cabrillo
  claims do not reflect R05. Reconcile these statements as the fixes land;
  generate catalog capability counts from the loaded catalog where practical.
  Acceptance: every advertised workflow and readiness claim has a matching
  implementation check, and known station-side/scoring limitations are visible
  to the operator rather than only buried in design prose.

- [x] **R28 — Release/platform tests miss the failing compositions above.**
  `.github/workflows/ci.yml`, `.github/workflows/release.yml`, and
  `cmd/w4gns-logger/*_test.go` run substantial tests, but predominantly on
  Linux and frequently against scoring helpers rather than persistence/UI/
  export combinations. The release verification job also omits the normal
  CI vulnerability scan, so an independently pushed tag bypasses that gate.
  Add the acceptance regressions above and native Windows/macOS smoke tests
  for filesystem replacement, startup, and timezone loading; apply the same
  required vulnerability gate to releases. Acceptance: release checks cover
  the actual supported targets and fail on the reproduced regressions.

Suggested implementation order: R01/R07/R08 (preserve data and deliveries),
R02/R03 (occurrence and serial model), R04–R06/R09/R10/R18 (submission
correctness), then the remaining P2 items. Fold each regression into the
permanent suite when implementing its fix; do not mark a whole subsystem done
because a helper-level test passes.

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

- 🔧 **Cabrillo/layout and scoring support remains event-specific.** The
  loaded catalog's capability labels and configuration are authoritative;
  historical static counts and lists have been removed because they drifted.
  SAC-CW now has both a layout and scoring configuration, with the unsupported
  non-Scandinavian European entrant branch noted in the UI and README.
  Section 0A adds strict supported-field validation and a dedicated Sweepstakes
  layout; this does not certify every sponsor's rules or all entrant branches.

- 🔧 **Audit scoring per contest before treating estimates as authoritative.**
  **SAC-CW's real scoring** (sactest.net's Sections 7-8) is
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
  (`events_test.go`). The initial TNQP implementation provided 3 points per
  CW QSO and outside-entrant county multipliers per band. That scope is now
  superseded by the shared state-party implementation above: both entrant sides,
  location-aware credit, K4TCG bonuses, and mobile/rover activation bonuses are
  configured for 2026. K4TCG's CW bonus is once per band for all entrants, not
  per QSO or restricted to Tennessee residents. The original **`tn_county`
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
  (`TestLoadEventCatalogTNQPHasRealScoringRules`). These test references describe
  the original implementation; current shared coverage is recorded above.
  Additional events have since gained scoring. Unaudited definitions must still
  be checked against sponsor rules before their capabilities are promoted.

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
  Blocked on open decision #2 (data licensing). A CWops-roster-based version
  of this was implemented and then reverted (2026-09-04): CWops does publish
  its own Member Roster as a public CSV export
  (cwops.org/membership/member-roster-2/), but the club's privacy policy is
  silent on redistribution/embedding rights for that public page and
  separately describes a members-only Groups.io channel for
  logger-integration data — a signal the club treats "publicly viewable"
  and "cleared for third-party software" differently. Given QRZ lookup
  already covers name/QTH/grid prefill for any callsign (not just the
  ~3,400-member CWops subset), the marginal value didn't justify committing
  static third-party personal data under unresolved terms. Revisit only
  with an explicitly cleared source.
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
- ✅ **Real per-contest wiring: North American Sprint, CW's actual scoring
  rules**, needing no new schema — Rule 10 (ncjweb.com/Sprint-Rules.pdf) is a
  flat 1 point per QSO times the existing `naqp_area` multiplier kind (the
  same US-state/DC/Canadian-province/other-North-America-DXCC table NAQP CW
  uses; Rule 10's 13-province list and its Newfoundland-Labrador combination
  match NAQP's Rule 11 exactly, and `naqpAreaCode`'s existing
  continent-must-be-NA check already excludes the non-NA "DX" exchange
  Sprint Rule 7 has non-North-American entrants send instead of a country),
  counted once for the whole contest (`per: "contest"`) rather than again
  per band the way NAQP counts it — already a generically supported
  `Per` value for this multiplier kind, so no `contest_state.go` change was
  needed, only the catalog config. Closing out this entry surfaced a
  pre-existing data bug: curated `NA-SPRINT-CW`
  (`events/contestcalendar.json`) had `sent_serial: false`, even though Rule
  7's exchange is explicitly "a sequential serial number" plus name and
  location (no RST) — the worked example in the rules text itself
  ("N6TR K7GM 154 RICK NC") shows the serial; fixed to `sent_serial: true`
  so the app's running-serial field and Rcv # column actually appear for
  this contest. `cabrillo_layout: cw_exchange_only` +
  `cabrillo_omit_rst: true` (no RST, matching CW Open/NAQP-CW/ARRL SS'
  shape) and `adif_contest_id: NA-SPRINT-CW` (confirmed against the ADIF
  Contest ID Enumeration) were added alongside, promoting `capability` to
  `scoring-ready`; the generated `SD-NASPRCW` duplicate is dropped via the
  existing curated-vs-generated de-dup (shared `NA-SPRINT-CW` Cabrillo
  token, already matching with no `cabrillo_contest` override needed on
  either side). Not addressed here (a documented limitation, not a rules
  gap, same class as `naqpAreaCode`'s own documented caveats): Rule 9 treats
  `4U1UN` (the Vienna International Centre special callsign) as a North
  American entity for this contest despite cty.dat classifying it under
  Europe's continent, so a Sprint contact with that one callsign would not
  register as a multiplier under the generic `naqp_area` lookup — a
  vanishingly rare edge case, not fixed with special-case code. Tests:
  `events_test.go` (`TestLoadEventCatalogNASprintCWHasRealScoringRules`).
- ✅ **Real per-contest wiring: WAE DX Contest, CW's actual QSO points and
  multiplier rules** (QTC traffic-list bonus points and Section 5's EU/non-EU
  QSO eligibility restriction deliberately out of scope — see below).
  Sourced from darc.de's WAEDC rules page (Sections 5, 6, 8): Section 8's
  score formula ("total QSOs ... multiplied by the sum of all multipliers
  weighted by the band bonus factor") is a flat 1 point per QSO, unlike CQ
  WW/ARRL DX/WPX's continent-tiered points — so no new points schema was
  needed. Section 6's multiplier is genuinely side-asymmetric, but in the
  *opposite* direction from every side-asymmetric event wired so far: a
  non-European entrant (this app's own station profile) counts distinct **WAE
  Country List** entities worked per band, while a European entrant counts
  distinct **non-European DXCC** entities worked per band — WAE's own
  "domestic" side is Europe, not this app's usual W/VE-station-is-domestic
  default, so `DomesticCountries` here is (unlike every prior use) the
  enumerable European side, and `Scoring`/`DXScoring` are assigned the
  reverse of ARRL-DX-CW's convention (`Scoring` = the European-entrant rules,
  `DXScoring` = this app's own non-European-entrant rules) — a labeling
  inversion documented inline in both `events.go` and the catalog entry so it
  isn't mistaken for a bug. **New `wae_country.go`** (`waeCountries`,
  `isWAECountry`, `waeCountryNames`) resolves the WAE Country List by
  cty.dat country name rather than re-deriving prefix matching: every WAE
  rules-page prefix token was resolved through this app's own embedded
  cty.dat to build the 69-country set (four tokens don't add a distinct
  entity under this app's data — 4U1V/Vienna Intl Ctr has no separate cty.dat
  entity and folds into Austria like OE, and the rules page's `GM/s`/`JW/b`
  sub-designators are marked non-DXCC in cty.dat and aren't resolvable from
  an ordinary callsign, so they fold into their parent entities Scotland/
  Svalbard — the same practical-approximation class as `wpxPrefix`'s
  non-exhaustive call handling). **New `wae_country`/`dxcc_non_wae`
  multiplier kinds** (`contest_state.go`: `waeCountryByBand`/`waeCountryAll`,
  `dxccNonWAEByBand`/`dxccNonWAEAll`, populated in `record()` by checking
  `isWAECountry` against the worked entity's resolved country) implement the
  two sides; `dxcc_non_wae` skips any worked entity whose country IS in the
  WAE list, so an accidental EU-EU contact under a European entrant's config
  doesn't spuriously add a multiplier even without an enforced eligibility
  check. **New `multiplierRule.Per: "band_weighted"` scope** (`events.go`
  `validMultiplierPer`; `contest_state.go` `multiplierCount`) implements
  Section 6's own band-bonus factor (`wae_country.go` `waeBandBonus`: 4x on
  80M, 3x on 40M, 2x on 20/15/10M) applied to whichever side's per-band
  count — a genuinely new multiplier-side capability (SAC's `CountryGroup`/
  `GroupPoints` and WPX's low-band tiering are both *points*-side weighting,
  not multiplier-side). Curated `DARC-WAEDC-CW`
  (`events/contestcalendar.json`) carries the real `scoring`/`dx_scoring`
  blocks plus `domestic_countries` (the 69-country WAE list, cross-checked
  1:1 against `wae_country.go`'s own set by test), `adif_contest_id:
  DARC-WAEDC-CW` (confirmed against the ADIF Contest ID Enumeration), and
  `cabrillo_layout: cw_rst_exchange` (RST + serial number on both sides,
  matching CQ WW/CQ 160/ARRL DX/WPX/IARU HF's shape), promoting `capability`
  to `scoring-ready`. Closing this out also surfaced a pre-existing dedup
  gap: the generated `SD-WAEDX` entry's own `cabrillo_contest` token
  (`DARC-WAEDC`, no `-CW` suffix) never matched the curated entry's
  ID-derived token (`DARC-WAEDC-CW`), so the two survived as an undetected
  duplicate pair — fixed by adding `"cabrillo_contest": "DARC-WAEDC"` to the
  curated entry, the same class of fix CWT needed. **Deliberately out of
  scope, not a rules gap:** Section 7's QTC (traffic list) bonus points need
  actual QTC send/receive/log functionality this app doesn't have yet (still
  tracked separately in §4, "WAE QTC send/receive/log"); Section 5's
  restriction that a contest QSO can only be between a European and a
  non-European station is unenforced, so — unlike the QSO-eligibility-clean
  contests wired so far — an operator who logs an invalid same-side contact
  under this event would still have it scored (though `dxcc_non_wae`'s own
  WAE-list exclusion means an accidental EU-EU contact can't inflate a
  European entrant's multiplier, only its QSO-point count). Tests:
  `wae_country_test.go` (`TestIsWAECountry`, `TestWAECountriesHas69Values`,
  `TestWAEBandBonus`), `contest_state_test.go`
  (`TestContestStateScoreWAECountryMultiplierBandWeighted`,
  `TestContestStateScoreDXCCNonWAEMultiplierBandWeighted`,
  `TestContestStateWouldBeNewMultiplierWAECountry`,
  `TestContestStateWouldBeNewMultiplierDXCCNonWAE`), `events_test.go`
  (`TestLoadEventCatalogWAEHasRealScoringRules`), `cabrillo_export_test.go`
  (`TestComputeContestScoreWAESideAsymmetric`).
- ✅ **Real per-contest wiring: Helvetia Contest's actual QSO points and
  multiplier rules**, the first curated event with a genuinely
  side-symmetric formula after SAC/ARRL-DX-CW/WAE's side-asymmetric wiring —
  every entrant, Swiss or not, scores under the same rule, so only one
  `Scoring` block was needed (no `DXScoring`/`DomesticCountries`). Sourced
  from uska.ch's "Rules and Regulations for Helvetia Contest" (issued March
  2026, §2.5, §2.7): a contact with a station in Switzerland scores 10
  points regardless of the operator's own location — reusing the existing
  `pointsRule.CountryGroup`/`GroupPoints` schema SAC-CW introduced, with
  `CountryGroup: ["Switzerland"]` — a same-continent contact scores 1, and a
  different-continent contact scores 3; the rules draw no separate
  same-country tier, so `SameCountry` is configured to match the flat
  `SameContinent` value (1) rather than left at CQ WW's 0. The multiplier is
  "Canton and DXCC country (including Switzerland) per band" (§2.7): the
  existing `dxcc` kind already covers the DXCC half unmodified (it has no
  Switzerland exclusion to begin with, unlike CQ 160/ARRL DX CW's own-country
  multiplier gap noted above). **New `canton` multiplier kind**
  (`canton.go`, `cantonCode`) resolves the Swiss half from the worked
  station's received-exchange text — only an HB9 station's exchange
  actually contains a canton (§2.5.1: RS(T) + 2-letter canton; §2.5.2's
  non-Swiss exchange is a plain running serial number, which never
  coincidentally matches one of the 26 canton codes) — the same
  exchange-is-authoritative, whole-text-match shape as `tn_county.go`'s
  `tnCountyCode`. `contest_state.go` extends the index with
  `cantonByBand`/`cantonAll` (recorded in `record()`, summed in
  `multiplierCount()`) and wires the as-you-type "NEW MULT" flag in
  `wouldBeNewMultiplier`. Curated `HELVETIA`
  (`events/contestcalendar.json`) carries the real `scoring` block plus
  `adif_contest_id: HELVETIA` (confirmed against the ADIF Contest ID
  Enumeration) and `cabrillo_layout: cw_rst_exchange` (RST + one free-text
  exchange field — canton or serial — the same shape CQ WW/CQ 160/ARRL DX/
  WPX/IARU HF/WAE already use), promoting `capability` to `scoring-ready`.
  No de-dup change was needed: the catalog already carried two generated
  `SD-*` entries sharing the curated entry's `HELVETIA` Cabrillo token
  (`Helvetia Contest - DX Side`/`- Home Side`) — the existing "token shared
  by two-or-more generated entries and one curated entry is added fidelity,
  not a duplicate" rule (§2) already keeps all three. Tests: `canton_test.go`
  (`TestCantonCode`, `TestCantonCodesHas26Values`), `contest_state_test.go`
  (`TestContestStateScorePointsRuleHelvetiaCountryGroup`,
  `TestContestStateScoreCantonMultiplierFromReceivedExchange`,
  `TestContestStateWouldBeNewMultiplierCanton`), `events_test.go`
  (`TestLoadEventCatalogHelvetiaHasRealScoringRules`).
- ✅ **Real per-contest wiring: Russian DX Contest's actual QSO points and
  multiplier rules**, side-asymmetric like ARRL-DX-CW/WAE/SAC-CW but the
  first contest wired here where the side-asymmetric split is a flat
  entity-group bonus (like Helvetia's `CountryGroup`) layered on top of an
  otherwise-uniform points formula shared by both sides, rather than either
  side needing its own distinct tier values or its own distinct multiplier
  kind. Sourced from rdxc.org/rules_eng (Sections 6, 7, 9) — the oblast code
  table itself (Section 6.3/9's "attached list") is not embedded in the
  rules page's own HTML (it renders as a client-side table WebFetch can't
  extract) and was supplied directly by the operator rather than guessed.
  Section 7: a Russian entrant scores 2/3/5 points for
  same-country/same-continent/other-continent; a non-Russian entrant (this
  app's own station profile) scores the same 2/3/5 tiers, *except* a QSO
  with any Russian-flagged station scores a flat 10 regardless of tier
  (§7.2 "QSO with Russian station – 10 points"). Section 7.3 ("Kaliningrad,
  Franz Josef Land, and Russian Antarctic stations each count as a separate
  DXCC entity") turns out to need no special-casing on the Russian-entrant
  side at all: this app's cty.dat already carries European Russia, Asiatic
  Russia, Kaliningrad, and Franz Josef Land as four distinct DXCC entities
  with the first two on different continents, so the existing
  country/continent classification alone reproduces every 7.1 case exactly
  (e.g. a European Russia operator working an Asiatic Russia station
  classifies as different-country-different-continent, landing on the
  existing `OtherContinent` value, which already equals the rule's own
  Russia-to-Russia-cross-continent figure). The non-Russian entrant's flat
  10-point bonus reuses the existing `pointsRule.CountryGroup`/`GroupPoints`
  schema (SAC-CW/Helvetia's mechanism) with
  `CountryGroup: ["European Russia", "Asiatic Russia", "Kaliningrad", "Franz
  Josef Land"]` — deliberately *not* including the generic cty.dat
  `Antarctica` entity, since that one DXCC entity is shared by every
  nation's Antarctic base, not just Russia's; a non-Russian entrant working
  a Russian Antarctic station therefore doesn't get the 10-point bonus under
  this config, a documented, narrow, out-of-scope gap rather than crediting
  every country's Antarctic operation. Section 9's two multipliers, both
  counted once per band: **new `oblast` multiplier kind**
  (`rdxc_oblast.go`, `rdxcOblastCode`) resolves the Russian side's own
  2-letter oblast code from the worked station's received-exchange text —
  the same exchange-is-authoritative, whole-text-match shape as
  `canton.go`/`tn_county.go` — against the contest's own 91-code table
  (deduplicated from the operator-supplied official list, which partitions
  some oblasts by prefix-number/suffix-letter range under the same code);
  Kaliningrad/Franz Josef Land/Antarctic stations are already ordinary rows
  in that table (`KA`/`FJ`/`AN`), not a separate special case, matching
  §7.3's "separate Oblast (double multiplier)" language. **New
  `dxcc_or_wae` multiplier kind** (`contest_state.go`
  `countryOrWAEByBand`/`countryOrWAEAll`) implements "DXCC entity list + WAE
  multipliers list" (§9) — a union wider than the existing `dxcc` kind,
  which skips any cty.dat entity with no assigned DXCC number
  (`recordMultiplierValue`); this app's own ARRL DXCC table
  (`loadARRLDXCCNumbers`) leaves a handful of WAE-listed entities (e.g.
  European Turkey) at zero, so the new kind reuses `wae_country.go`'s
  existing `isWAECountry` to still count them, keyed by country name
  (string) like `prefix`/`exchange_area` since not every qualifying entity
  has a DXCC number to key on. Curated `RDXC`
  (`events/contestcalendar.json`) carries the real `scoring`/`dx_scoring`
  blocks plus `domestic_countries` (the same four Russian-flagged entities
  as `CountryGroup`), `adif_contest_id: RDXC` (confirmed against the ADIF
  Contest ID Enumeration), and `cabrillo_layout: cw_rst_exchange` (RST +
  one free-text exchange field — oblast or serial number — the same shape
  CQ WW/CQ 160/ARRL DX/WPX/IARU HF/WAE/Helvetia already use), promoting
  `capability` to `scoring-ready`; the two generated `SD-*` entries sharing
  the curated entry's `RDXC` Cabrillo token (home/DX side splits, the same
  shape Helvetia's own generated duplicates take) survive de-dup unchanged
  under the existing "token shared by two-or-more generated entries and one
  curated entry is added fidelity" rule (§2). **Deliberately out of scope,
  documented limitations, not rules gaps:** §7.4's maritime-mobile ("/MM")
  flat-5-points/no-multiplier rule is unimplemented — this app already
  strips `/MM` as a non-DXCC-affecting portable suffix
  (`dxcc.go`'s `portableCallSuffixes`) before resolving the worked entity,
  so an `/MM` contact currently scores under the normal tiered rules and
  can count as a multiplier, the same class of gap as CQ 160/ARRL-DX-CW's
  own-country `dxcc`-multiplier exclusion noted above. Tests:
  `rdxc_oblast_test.go` (`TestRDXCOblastCode`,
  `TestRDXCOblastCodesHas91Values`), `contest_state_test.go`
  (`TestContestStateScorePointsRuleRDXCCountryGroup`,
  `TestContestStateScoreOblastMultiplierFromReceivedExchange`,
  `TestContestStateWouldBeNewMultiplierOblast`,
  `TestContestStateScoreDXCCOrWAEMultiplier`,
  `TestContestStateWouldBeNewMultiplierDXCCOrWAE`), `events_test.go`
  (`TestLoadEventCatalogRDXCHasRealScoringRules`).
- ✅ **Real per-contest wiring: Stew Perry Topband Distance Challenge's actual
  scoring rules**, a genuinely new points shape (continuous distance rather
  than a country/continent/zone tier) and the first curated event with no
  multiplier at all. Sourced from kkn.net/stew's rules page: "Count a
  minimum of one point per QSO and an additional point for every 500
  kilometers distance" between the two stations' 4-character grid squares
  (the contest's entire exchange — RST is explicitly optional and not
  scored), and "Final score equals the total number of QSO points. There is
  no multiplier for different grids worked." **New `pointsRule.Distance`**
  (`events.go` `distancePointsRule{PerKm}`, mutually exclusive with every
  other points field, enforced by `validateScoringRules`) is computed in
  `contest_state.go`'s `record()` from the worked station's grid as actually
  exchanged (`q.srxString`, the same exchange-is-authoritative precedent as
  `exchange_area.go`/`canton.go`) and the operator's own grid as snapshotted
  on the QSO at log time (`q.myGridSquare`, already persisted per-QSO —
  reading it here means a later station-profile edit can't retroactively
  change a logged QSO's score), reusing `grid.go`'s existing
  `ParseGridSquare` and `heading.go`'s `GreatCircleBearingDistance` rather
  than adding new geometry. New `distanceKmByKey map[string]float64`
  stores each scored QSO's distance (unset, so 0 points, if either grid
  doesn't parse); `distancePointsTotal`
  sums `1 + distanceKm/PerKm` over every scored QSO. **New `"none"`
  multiplier kind** (`validMultiplierKind`, `contestState.multiplierCount`)
  lets an event declare it genuinely has no multiplier: `contestScore.
  total()` multiplies QSO points by the summed multiplier count, so an empty
  `Multipliers` list (which `validateScoringRules` already rejects as "no
  multiplier configured" for every other event) can't simply mean zero here
  — `"none"` contributes a constant 1 instead, leaving the total equal to
  the QSO points sum. Curated `STEW-PERRY`
  (`events/contestcalendar.json`) carries the real `scoring` block plus
  `adif_contest_id: STEW-PERRY` (confirmed against the ADIF Contest ID
  Enumeration) and `cabrillo_layout: cw_rst_exchange` (RST + one free-text
  exchange field — the grid square — matching the Cabrillo QSO template
  `QSO: freq mo date time call rst exch call rst exch`, RST defaulting to
  599 even though the rules call it optional), promoting `capability` to
  `scoring-ready`; the generated `SD-STEW` duplicate is dropped via the
  existing curated-vs-generated de-dup (shared `STEW-PERRY` Cabrillo token).
  **Deliberately out of scope, a data gap rather than a formula gap:** the
  rules also multiply a QSO's points 2x/4x when the worked station declares
  itself Low Power/QRP, and multiply the operator's own final score 1.5x/3x
  for its own declared power category — neither is exchanged over the air
  (the exchange is only the grid square) or derivable from anything this
  app's QSO/Cabrillo model captures today, so this is left unimplemented
  rather than guessed, the same class of documented gap as WAE's QTC bonus
  or CQ 160/ARRL DX CW's own-country `dxcc`-multiplier exclusion. Tests:
  `contest_state_test.go`
  (`TestContestStateScorePointsRuleStewPerryDistance`,
  `TestContestStateScoreNoneMultiplierAlwaysCountsOne`), `events_test.go`
  (`TestLoadEventCatalogStewPerryHasRealScoringRules`).
- ✅ **Real per-contest wiring: Worked All Germany Contest's actual QSO points
  and multiplier rules**, side-asymmetric like ARRL-DX-CW/WAE/RDXC but the
  first contest wired here where the non-domestic entrant's points formula
  is genuinely flat (no country/continent tiering at all) while the domestic
  entrant's is tiered — the reverse pairing of shapes from every prior
  side-asymmetric event, which all gave the tiered formula (or a tiered
  formula plus a group bonus) to both sides. Sourced from darc.de's WAG
  rules (Sections 4-6) and the contest's own "Districts, DOKs and a
  mysterious multiplier" service page: Section 6 gives a German entrant 1/3/5
  points for same-country/same-continent/other-continent — the existing
  `pointsRule.SameCountry/SameContinent/OtherContinent` fields directly, no
  schema change — while "each complete exchange counts 3 points for
  non-German stations" (this app's own station profile) is a flat
  `PointsPerQSO: 3` with no `Points` block at all, the same shape NAQP-CW's
  own wiring used. Section 5's exchange format explains why: only a German
  station's exchange carries a multiplier-bearing value (RS(T) + DOK chapter
  code), while a non-German operator sends a plain running serial number, and
  a non-member German operator explicitly sends "NM" instead of a DOK
  ("will be no multiplier" per the rules) — the first letter of "NM" is
  itself the real district letter N (Nordrhein-Westfalen), so **new
  `dok_district.go`** (`dokDistrictCode`) excludes the literal "NM" token by
  name rather than relying on a letter/digit check alone. Unlike
  `canton.go`/`tn_county.go`'s fixed value sets, no district letter is
  actually excluded here: the WAG service page states "the regular DARC/VFDB
  districts allow for 25 multipliers per band" (every letter except J) plus a
  documented 26th "mysterious multiplier" from rare special-DOKs that do
  start with J, so `dokDistrictCode` accepts any letter A-Z. **New
  `dok_district` multiplier kind** (`contest_state.go`:
  `dokDistrictByBand`/`dokDistrictAll`, extending `multiplierCount`/
  `wouldBeNewMultiplier`) implements the non-German entrant's own per-band
  multiplier ("Stations outside Germany receive one multiplier point for
  each German district worked ... per band"); the German entrant's own
  multiplier ("each DXCC/WAE area plus IG9/IH9 ... per band") needed no new
  code at all, reusing RDXC's existing `dxcc_or_wae` kind unmodified. Curated
  `WAG` (`events/contestcalendar.json`) carries the real `scoring`
  (German-entrant) and `dx_scoring` (non-German-entrant, this app's own
  profile) blocks plus `domestic_countries: ["Fed. Rep. of Germany"]` (the
  embedded cty.dat's own country name for DL, confirmed by reading
  `data/cty.dat` directly — not the bare "Germany" the catalog's own
  `id`/`name` fields might suggest), `adif_contest_id: DARC-WAG` (confirmed
  against the ADIF Contest ID Enumeration — not the bare `WAG` the catalog's
  own event id might suggest),
  and `cabrillo_layout: cw_rst_exchange` (RST + one free-text exchange field
  — DOK, "NM", or serial number — the same shape every other
  `cw_rst_exchange` event already uses), promoting `capability` to
  `scoring-ready`; no de-dup change was needed since the catalog already
  carried two generated `SD-WAG*` entries (DL/DX side splits) sharing the
  curated entry's `WAG` Cabrillo token under the existing "token shared by
  two-or-more generated entries and one curated entry is added fidelity"
  rule (§2), the same shape Helvetia's own generated duplicates take.
  **Deliberately out of scope, documented limitations, not rules gaps:**
  this app logs CW only, and the rules' own per-band-per-mode counting
  ("once in CW and once in SSB") is moot under that standing limitation,
  the same class of non-gap as IARU HF's Phone/Mixed exclusion; the rules
  text found so far gives no separate same-country tier reference beyond
  Section 6's own wording, so no additional exception table (of the kind
  CQ WW's North America override or WPX's low-band tiering needed) was
  found to be missing. Tests: `dok_district_test.go` (`TestDOKDistrictCode`),
  `contest_state_test.go`
  (`TestContestStateScoreDOKDistrictMultiplierFromReceivedExchange`,
  `TestContestStateWouldBeNewMultiplierDOKDistrict`,
  `TestContestStateScorePointsRuleWAGSideAsymmetric`), `events_test.go`
  (`TestLoadEventCatalogWAGHasRealScoringRules`).
- ✅ **Real per-contest wiring: Oceania DX Contest, CW's actual scoring
  rules**, the first curated event with a points formula that ignores
  country/continent entirely — points depend solely on the band a QSO was
  worked on. Sourced from oceaniadxcontest.com's rules PDF: 20/10/5/1/2/3
  points on 160/80/40/20/15/10M respectively, times the number of distinct
  callsign prefixes worked, counted again on every band ("the same prefix
  may be counted once on each band for multiplier credit") — the existing
  CQ WPX-style `prefix` multiplier kind (`wpx.go`'s `wpxPrefix`) already
  covers this unmodified, just with `per: "band"` instead of WPX's own
  `per: "contest"`; `multiplierCount`/`wouldBeNewMultiplier` already handled
  both scopes generically, so no `contest_state.go` change was needed for
  the multiplier half. **New `pointsRule.PerBand` field**
  (`events.go`, `map[string]int` keyed by the same uppercase band string as
  `eventDefinition.Bands`) and `contestState.perBandPointsTotal`
  (`contest_state.go`) implement the points half — mutually exclusive with
  every other points field, enforced by `validateScoringRules`, the same
  pattern `Zone`/`Distance` already established. Curated `OCEANIA-DX-CW`
  (`events/contestcalendar.json`) carries the real `scoring` block plus
  `adif_contest_id: OCEANIA-DX-CW` (confirmed against the ADIF Contest ID
  Enumeration) and `cabrillo_layout: cw_rst_exchange` (RST + serial number
  on both sides), promoting `capability` to `scoring-ready`. Closing this
  out surfaced a pre-existing de-dup gap of the same class CWT/WAE needed
  fixed: the generated `SD-OCEANIA` entry's own `cabrillo_contest` token
  (`OCEANIA-DX`, missing the mode suffix) never matched the curated entry's
  ID-derived token (`OCEANIA-DX-CW`, the actual official Cabrillo name per
  LoTW's defined-contests list) — unlike CWT/WAE, the curated side was
  already correct here, so the fix corrected the generated file's token
  instead of adding a curated override, which would have broken this
  event's own Cabrillo `CONTEST:` header. Tests: `contest_state_test.go`
  (`TestContestStateScorePointsRulePerBand`), `events_test.go`
  (`TestLoadEventCatalogOceaniaDXHasRealScoringRules`, new `TestValidateScoringRules`
  cases for `per_band`).
- ✅ **Real per-contest wiring: K1USN Slow Speed Test (SST)'s actual scoring
  rules**, closing out the one curated event left without a `scoring` block.
  Sourced from the K1USN SST Rules (linked from k1usn.com/sst_rules.html; the
  rules text itself lives in a Google Doc embedded via iframe, which needed a
  direct fetch of the doc's own publish URL rather than the wrapper page,
  the same class of non-trivial-to-extract source as RDXC's oblast table):
  "SCORING 1 point for each QSO regardless of QTH. Multipliers are the sum
  of States, Provinces and DXCC Countries. No DXCC credit for the USA lower
  48 States or Canada ... DXCC Multiplier for stations worked outside the
  USA lower 48 states and Canada (applies to USA/Canada and all DX)" — a
  flat 1 point per QSO (no continent/country tiering, no schema change
  needed) times a multiplier counted once for the whole contest (the rules'
  own worked example sums a single flat "Total Multipliers" figure, not a
  per-band one). **New `sst_area` multiplier kind** (`sst_area.go`,
  `sstAreaCode`) reuses `naqp_area.go`'s existing 50-state/DC/13-province
  table and "Name Location" last-token exchange parsing unmodified (SST's
  own `sent_exchange_hint`, "First name + state/province/DX country", is the
  same shape) but drops naqpAreaCode's North-America-only restriction on its
  DXCC fallback: SST's DXCC multiplier is worldwide, not NA-only, so any
  cty.dat entity outside the United States/Canada counts, keyed by country
  name like `naqp_area`/`exchange_area`. `contest_state.go` extends the
  index with `sstAreaByBand`/`sstAreaAll` (recorded in `record()`, summed in
  `multiplierCount()`) and wires the as-you-type "NEW MULT" flag in
  `wouldBeNewMultiplier`. Curated `K1USN-SST` (`events/k1usn.json`) carries
  the real `scoring` block plus `adif_contest_id: K1USN-SST` (confirmed
  against the ADIF Contest ID Enumeration) and `cabrillo_layout:
  cw_exchange_only` (no RST, matching CW Open/NAQP-CW/ARRL SS/NA Sprint's
  shape — SST's `cabrillo_omit_rst: true` was already set), promoting
  `capability` to `scoring-ready`; the generated `SD-SST` duplicate remains
  dropped via the pre-existing curated-vs-generated de-dup (shared
  `SLOW-SPEED-TEST` Cabrillo token, from §2's "Real sessions/schedules"
  entry). Tests: `sst_area_test.go` (`TestSSTAreaCode`),
  `contest_state_test.go`
  (`TestContestStateScoreSSTAreaMultiplierCountsOncePerContest`,
  `TestContestStateWouldBeNewMultiplierSSTArea`), `events_test.go`
  (`TestLoadEventCatalogK1USNSSTHasRealScoringRules`).
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
- ⏳ **WAE QTC** send/receive/log · **Sked/reminder** (`.MMO`).
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
   treat keyer/rig/voice/band-map/WAE/skeds as Later.

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
| State-party county-pair duplicates and credit | `qso_party_rules.go`, `state_qso_party.go` | entrant-side eligibility, county lines, bonuses, power factors, checked export |
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

**Special contests** — WAE **QTC** — **Later**; RSGB IOTA Contest island-ref
mults and points — **Have** (`iota` multiplier kind, `pointsRule.IOTA`, own-
station My IOTA Ref in Station Setup); DXpedition/special-event templates —
**Core** (data-driven)

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
