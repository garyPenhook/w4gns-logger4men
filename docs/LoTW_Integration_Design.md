# ARRL Logbook of the World (LoTW) integration — design

Status: implemented. LoTW is a third durable upload destination alongside QRZ
Logbook and World Radio League: QSOs are signed with the locally installed TQSL
and uploaded to LoTW automatically as they are logged, plus a manual/CLI
backfill path (`Ctrl+Y` / `--upload-lotw`) for an existing log. See
`cmd/w4gns-logger/lotw.go` and `cmd/w4gns-logger/lotw_test.go`; the "ARRL LoTW
upload" section of `README.md` documents the operator-facing setup and
behavior. The design below reflects what shipped, not a proposal.

Reference: ARRL LoTW help — <https://lotw.arrl.org/lotw-help/>.

## Goals

- Every logged QSO flows to LoTW the same way it already flows to QRZ/WRL:
  enqueued durably on log, signed and uploaded by the periodic outbox drain,
  with per-QSO status visible in the upload panel and audit log.
- A manual/CLI batch path to backfill an existing log into LoTW.
- No QSO is lost on crash, quit, or transient failure; duplicates are never a
  problem because TQSL maintains its own upload-tracking database.

## Non-goals

- No re-implementation of TQSL signing. LoTW requires a Callsign Certificate;
  signing is delegated entirely to the installed `tqsl` binary.
- No certificate/station-location management UI. The operator configures those
  in TQSL as they do today; the app only *selects* an existing station location
  by name.
- No download/sync of LoTW confirmations (QSLs) in this phase. Upload only.

## Environment (verified on this machine)

- `tqsl` resolves to `~/.local/bin/tqsl`, **TQSL Version 2.8.6 [pkg-v2.8.6]**.
- `~/.tqsl/` already holds a `W4GNS` Callsign Certificate and a station location
  named **`Home`** (`<StationData name="Home">` in `~/.tqsl/station_data`).
- TQSL keeps its own duplicate database at `~/.tqsl/uploaded.db`.

## Background: how LoTW/TQSL upload works

LoTW only accepts QSOs digitally signed by a Callsign Certificate. TQSL performs
the signing and the upload. Relevant `tqsl` command-line options (from
`tqsl --help` on 2.8.6):

| Option | Meaning |
| --- | --- |
| `-l, --location <name>` | Select the station location to sign under |
| `-u, --upload` | Upload after signing instead of saving a `.tq8` |
| `-x, --batch` | Exit after processing the log (no interactive startup) |
| `-d, --nodate` | Suppress the date-range dialog |
| `-a, --action <str>` | Non-interactive dialog action: `abort`, `all`, `compliant`, `ask` |
| `-p, --password <str>` | Passphrase for the signing key |
| `-o, --output <file>` | Output file (defaults to input basename + `.tq8`) |
| `-q, --quiet` | Quiet mode (same behavior as `-x`) |

Planned invocation for a headless sign-and-upload of one temp ADIF file:

```
tqsl -x -d -a compliant -l <station> [-p <passphrase>] -u <input.adi>
```

`-x -d -a compliant` guarantees no dialog blocks the process: `-a compliant`
signs the QSOs that are compliant and drops the rest rather than prompting.
`-p` is supplied only when the operator configured a passphrase (see Config).

### TQSL exit-status codes (authoritative)

Verified against the TQSL source array `errors[]` in `apps/tqsl.cpp`
(SourceForge `trustedqsl/tqsl` master) and cross-checked against the installed
binary, which prints `Final Status: <text>(<n>)` — a deliberate syntax error
produced `Command Syntax Error(10)`, matching index 10 below, confirming the
enum is 0-based and sequential.

| Code | Enum | Text | App treatment |
| ---: | --- | --- | --- |
| 0 | `TQSL_EXIT_SUCCESS` | Success | **delivered** |
| 1 | `TQSL_EXIT_CANCEL` | User Cancelled | failure (should not happen headless) |
| 2 | `TQSL_EXIT_REJECTED` | Upload Rejected | failure (permanent — server rejected) |
| 3 | `TQSL_EXIT_UNEXP_RESP` | Unexpected LoTW Response | failure (retry) |
| 4 | `TQSL_EXIT_TQSL_ERROR` | TQSL Error | failure |
| 5 | `TQSL_EXIT_LIB_ERROR` | TQSLLib Error | failure |
| 6 | `TQSL_EXIT_ERR_OPEN_INPUT` | Error opening input file | failure (bug — our temp file) |
| 7 | `TQSL_EXIT_ERR_OPEN_OUTPUT` | Error opening output file | failure |
| 8 | `TQSL_EXIT_NO_QSOS` | No QSOs to upload | **delivered** (nothing left to send) |
| 9 | `TQSL_EXIT_SUPPRESSED` | Some QSOs not processed | **delivered** (dupes/out-of-range dropped) |
| 10 | `TQSL_EXIT_COMMAND_ERROR` | Command Syntax Error | failure (bug in our argv) |
| 11 | `TQSL_EXIT_CONNECTION_FAILED` | LoTW Connection Failed | failure (**retry** — transient) |
| 12 | `TQSL_EXIT_UNKNOWN` | Unknown | failure (retry) |
| 13 | `TQSL_EXIT_BUSY` | Upload tracking database is locked | failure (**retry** — transient) |
| 14 | `TQSL_EXIT_UPLOADED_ALREADY` | Previously signed QSOs were detected | **delivered** (already at LoTW) |
| 15 | `TQSL_EXIT_BAD_PASSPHRASE` | Incorrect passphrase | failure (permanent — config) |

Delivered set = `{0, 8, 9, 14}`. Codes `9` (some suppressed) and `14`
(already uploaded) are treated as delivered on purpose: the suppressed/duplicate
QSOs are ones LoTW will never accept again, so re-queuing them would retry
forever. The trade-off — a QSO suppressed for a *non*-duplicate reason (e.g. it
falls outside the certificate's validity dates) is marked delivered without
reaching LoTW — is acceptable because such a QSO can never be signed under the
current certificate anyway, and the outcome is recorded in `upload_log` for the
operator to see. Transient failures `{11, 13}` and the ambiguous `{3, 12}` use
normal exponential backoff; permanent config failures `{2, 15}` back off and
eventually park with `last_error` visible, exactly like a rejected QRZ upload.

## ARRL developer guidance & compliance

ARRL publishes developer guidance for logging applications that automate LoTW
signing/upload. Sources: [Integrating Logbook of the World with Logging
Applications (PDF)](https://www.arrl.org/files/file/LoTW_Developer/DeveloperIntro.pdf),
[Developer Information](https://lotw.arrl.org/lotw-help/developer-information/?lang=en),
[Submitting QSOs](https://lotw.arrl.org/lotw-help/developer-submit-qsos/?lang=en),
[TQSL Command Line Reference](https://lotw.arrl.org/lotw-help/cmdline/?lang=en).

| ARRL guidance | How this app complies |
| --- | --- |
| *"Uploading all of a log's QSOs should not be routine."* Applications must not resubmit QSOs already at LoTW unless the operator believes they never arrived or were lost. | Automatic delivery enqueues each QSO **once**, on log, and removes its outbox row once delivered — never a routine full-log resend. `Ctrl+Y` / `--upload-lotw` *do* resubmit the whole log, but are framed in the UI/README as a one-off backfill for initial setup or recovery, not something to run on a schedule (see below). |
| *"Exploit the duplicate QSO detection and removal facilities provided in TrustedQSL and TQSL's command line interface"* rather than reimplementing dedup. | Dedup is delegated entirely to TQSL's own `~/.tqsl/uploaded.db`. Exit code `14` (`TQSL_EXIT_UPLOADED_ALREADY`) is treated as delivered — this app never tracks "already sent to LoTW" state itself, so a backfill re-run costs a `tqsl` invocation but not a redundant server-side submission. |
| A QSO is a duplicate when `CALL`/`BAND`/`MODE`/`PROP_MODE`/`SAT_NAME` and the station location are unchanged. | Not reimplemented here (see above) — this is exactly TQSL's own matching logic, which this app relies on rather than duplicates. |
| `TIME_ON` should be the time "one would be satisfied with had it been written on a QSL card"; LoTW matches within a ±30-minute window. | `adifQSOFields` (shared with the ADIF export and QRZ upload) emits `q.time.UTC()` directly — the same start time recorded at logging, with no separate LoTW-specific time handling to drift out of sync. |
| Drive `tqsl` non-interactively via `-x`/`-q` (batch, no menu), `-l` (station location), `-p` (passphrase), `-d` (suppress date-range dialog), `-a [abort\|compliant\|all\|ask]` (duplicate/out-of-range handling), `-u` (upload). Without these, TQSL falls back to interactive dialogs. | `runTQSL` invokes exactly `tqsl -x -d -a compliant -l <station> [-p <pass>] -u <file>` — the full documented non-interactive switch set. |
| `-n` checks for TQSL version/critical-file updates; called out for programs that drive `tqsl` on the user's behalf. | **Not implemented.** Low-stakes (affects TQSL's own trust/cert data freshness, not QSO delivery) — tracked in Open questions below. |

## Architecture fit

The existing durable upload machinery is reused wholesale:

- `outbox.go` — `upload_outbox` (one row per `(qso_id, destination)`, with
  `attempts`, `next_attempt_at`, `last_error`, `binding`) and `upload_log`
  (terminal audit rows). Enqueue/claim/complete/fail helpers already exist.
- `main.go` `drainOutbox()` (`~2463`) — claims due rows and dispatches upload
  commands; `runBgCmd` + `bgTasks` make signing wait for a clean shutdown.
- `upload_status.go` — status panel and `Ctrl+U` retry.
- `adif_export.go` — `adifQSOFields(q qso)` builds the ADIF field list shared by
  bulk export and the single-record QRZ upload; reused to produce the ADIF that
  TQSL signs.

LoTW differs from QRZ/WRL in one way that requires a new code path: it is
**batch-oriented**. Signing is a subprocess spawn, so signing one QSO per TQSL
invocation is wasteful. Instead, all LoTW rows claimed in a single drain are
written to **one** temp ADIF and signed/uploaded with **one** `tqsl` call, then
each QSO's outbox row is resolved from the shared exit code. QRZ/WRL keep their
existing per-QSO HTTP path unchanged.

## Detailed design

### New destination constant

`outbox.go`:

```go
const uploadDestLoTW = "lotw"
```

### Configuration (`paths.go` + new `lotw.go`)

Two new credential inputs, loaded with the same legacy-cwd-then-XDG precedence
as `loadQRZAPIKey`/`loadWRLAPIKey`, files kept `0600`:

- **Station location name** — file `lotw.station` (env `W4GNS_LOTW_STATION`).
  Required; must match a TQSL station-location name (e.g. `Home`).
- **Signing passphrase** — file `lotw.pass` (env `W4GNS_LOTW_PASS`). Optional;
  passed as `-p` only when non-empty. On this machine the W4GNS key is **not**
  passphrase-protected (see below), so it stays empty here — but the option is
  kept for portability: a protected key would otherwise block on a prompt under
  `-x` and the upload would hang/fail.

`findTQSL()` resolves the binary from `PATH`, overridable via `W4GNS_TQSL`
(the test seam — points at a fake script in tests).

### Enablement guard (`main.go` `uploadDestinations()`)

Append `uploadDestLoTW` only when **both** a station location is configured
**and** `findTQSL()` succeeds. This mirrors the existing rule that a destination
with no usable credentials is never enqueued, so LoTW rows can't pile up
unsendable on a machine without TQSL.

### Binding (`upload_status.go` `uploadBindings()`)

`binding = uploadBinding(stationLocation, tqslPathFingerprint)`. Changing the
station location (or the resolved TQSL binary) invalidates in-flight rows the
same way a changed QRZ key does — the drain pauses them with a "missing or
changed credentials" `last_error` rather than silently signing under a
different location.

### Enqueue on log

No new code beyond the destination list: `logCurrentQSO` already enqueues one
outbox row per entry in `uploadDestinations()` inside the same transaction that
inserts the QSO, honoring the post-log edit window via `next_attempt_at`.

### Batch drain (`main.go` `drainOutbox` + `lotw.go`)

`drainOutbox` currently loops claimed entries and builds one cmd each via
`switch e.destination`. Change: **partition** claimed entries by destination.
QRZ/WRL keep the per-entry loop. All `lotw` entries are collected and handed to
a single batch command:

1. Re-read each QSO fresh (`qsoByID`); drop rows whose QSO was deleted
   (`markUploadDone`), fail-and-retain on read error — same semantics as today.
2. Write the surviving QSOs to a temp `.adi` in the export temp dir using
   `adifQSOFields` (atomic temp-file pattern from `writeADIFAtomic`).
3. `signAndUploadLoTW(ctx, tqslPath, station, pass, adifPath)` runs the `tqsl`
   invocation above, captures the exit code, and removes the temp file.
4. Map the exit code (table above). On a delivered code, `markUploadDone` +
   `logUploadEvent(uploadLogSent)` for **every** QSO in the batch. On a failure
   code, `recordUploadFailure` + `logUploadEvent(uploadLogFailed)` for every
   QSO, storing the TQSL status text as `last_error`.

Because success/failure is shared across the batch, one bad QSO can bounce the
whole drain's batch back to retry; TQSL's duplicate DB makes the retry harmless
(already-signed QSOs return code 9/14 = delivered). This is the standard,
safe LoTW workflow.

Wrapped in `runBgCmd(m.bgTasks, …)` so a shutdown mid-sign is awaited, matching
`qrzOutboxUploadCmd`/`wrlOutboxUploadCmd`.

### Manual + CLI backfill

- In-app hotkey action "Upload log to LoTW" that enqueues LoTW rows for the
  active profile's existing QSOs (or a date range), then triggers a drain.
- `--upload-lotw` CLI flag: export the profile's ADIF and run the same
  `signAndUploadLoTW`, for scripted/backfill use. Parallels `--export-adif`.
  Safe against dupes via TQSL's tracking DB.
- Per ARRL's "should not be routine" guidance (see ARRL developer guidance
  above), both paths are documented as an occasional/one-off backfill, not a
  scheduled job — nothing currently *enforces* that (no rate-limit or
  last-run tracking); it's a documentation-level nudge, not a technical
  backstop. A cron job that runs `--upload-lotw` every few minutes would
  still work (TQSL's own tracking DB keeps it harmless to LoTW's server) but
  defeats the point of the durable per-QSO outbox and burns a `tqsl`
  subprocess signing the whole log each time.

### Concurrency & shutdown notes

- `signAndUploadLoTW` honors the passed `ctx` (derived from `m.bgCtx` with a
  timeout) so a hung TQSL is killed on shutdown; the subprocess is started with
  a context so `cmd.Cancel`/kill applies.
- Only the outbox drain spawns TQSL. The drain already leases claimed rows
  (`claimDueUploads` pushes `next_attempt_at` forward) so two overlapping drain
  ticks cannot double-submit the same batch.
- TQSL's `uploaded.db` lock surfaces as exit `13` (BUSY) → retry, which covers
  the case where an operator runs the TQSL GUI concurrently.

## Testing

- **Fake `tqsl`** shell script injected via `W4GNS_LOTW`/`W4GNS_TQSL` that exits
  with a chosen code and echoes a `Final Status` line. Table-driven test over
  every exit code asserting the correct outbox transition (delivered set →
  `markUploadDone`; retryable → `recordUploadFailure` reschedules; permanent →
  parks after `maxUploadAttempts`).
- Batch ADIF generation: multiple QSOs → one file with the expected `<EOR>`
  count and fields, reusing `adifQSOFields`.
- Binding invalidation when the station location changes mid-flight.
- Regression: `drainOutbox` partition reaches the batch path for `lotw` and the
  per-entry path for `qrz`/`wrl` (guards against a future `switch` edit dropping
  LoTW).
- `go test ./... `, `go vet`, `-race -short`, staticcheck, govulncheck — the
  project's standard gates.

## Files touched

| File | Change |
| --- | --- |
| `cmd/w4gns-logger/lotw.go` (new) | config loaders, `findTQSL`, `signAndUploadLoTW`, exit-code mapping, batch command |
| `cmd/w4gns-logger/outbox.go` | `uploadDestLoTW` constant |
| `cmd/w4gns-logger/paths.go` | `lotw.station` / `lotw.pass` path resolvers |
| `cmd/w4gns-logger/main.go` | `uploadDestinations()` guard, `drainOutbox` partitioning, hotkey + `--upload-lotw` |
| `cmd/w4gns-logger/upload_status.go` | `uploadBindings()` entry, status-panel label |
| `cmd/w4gns-logger/lotw_test.go` (new) | fake-tqsl exit-code table, ADIF batch, binding, drain-partition regression |
| `README.md` | LoTW setup section |

### Verified: key is not passphrase-protected

A non-interactive test-sign confirmed no passphrase is needed on this machine:

```
tqsl -x -d -a compliant -l Home -z -o out.tq8 probe.adi </dev/null
→ Signing using Callsign W4GNS, DXCC Entity UNITED STATES OF AMERICA
→ wrote 1 records ... Final Status: Success(0)
```

Success with stdin closed and no `-p` proves the `W4GNS` key container
(`~/.tqsl/keys/W4GNS`, whose `<PRIVATE_KEY:916>` field carries no `ENCRYPTED`
marker) is unprotected. `lotw.pass` therefore stays unset here; `-p` is omitted
from the invocation when empty. The same command with `-u` in place of `-z`
is the production upload call.

## Open questions / follow-ups

- Phase 2 (out of scope here): download LoTW QSL confirmations and mark matched
  QSOs as confirmed.
- Call `tqsl -n` (update/critical-file check) periodically — ARRL calls this
  out for programs driving `tqsl` on the user's behalf; not implemented yet
  (see ARRL developer guidance above).
- Consider a technical backstop (rate-limit or last-run timestamp) for
  `Ctrl+Y`/`--upload-lotw` so the "not routine" guidance is enforced rather
  than only documented, if operators are observed scripting it on a schedule.
