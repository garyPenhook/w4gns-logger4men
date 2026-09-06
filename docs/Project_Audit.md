# Project audit — September 6, 2026

Reviewed the Go application, database schema and migrations, QSO editing and
profile boundaries, ADIF parsing, export paths, upload queue, network clients,
background work, terminal rendering, contest/scoring code, release workflows,
and the Python county-import utility. Fixed the reproducible defects below.

## Findings and fixes

| Priority | Finding | Fix and regression coverage |
| --- | --- | --- |
| High | Atomic export functions could replace the live database or its WAL/SHM files. The CLI's existing guard compared a filesystem path against the raw SQLite DSN, missing URI forms. | All three atomic exporters check SQLite's actual open filename before creating output. The CLI also resolves URI escaping and driver options. `TestAuditExportCannotReplaceDatabase` covers ADIF, CSV, and Cabrillo; a subprocess smoke test verifies CLI refusal and an intact database. |
| High | Terminal control sequences in remote status/error text, POTA park names, solar text, and imported signal reports reached the terminal renderer. | Sanitize external text before applying application styles, including Recent QSOs and call history. Stored QSO values remain intact. `TestAuditTerminalOutputTreatsExternalTextAsData` reproduces an OSC clipboard-control sequence across the affected views. |
| Medium | Deleting a QSO using the wrong profile ID deleted its upload queue rows even though the QSO itself remained. Updates to absent or mismatched records also reported success. | Require a matching QSO deletion before removing outbox rows, within the same transaction. QSO and station-profile updates report missing records. Covered by `TestAuditWrongProfileCannotRemoveUploads` and `TestAuditClearStationGrid`. |
| Medium | Database precreation and permission checks treated SQLite URI/options text as a filesystem name; named memory databases could create stray files. | Resolve the filesystem component before precreation and use SQLite's authoritative filename after opening. Respect URI modes requiring an existing database. `TestAuditSQLiteDSNFileHandling` checks memory databases, escaped names, driver options, owner-only file permissions, and absent `mode=rw`/`mode=ro` databases. URI behavior follows [SQLite's URI documentation](https://www.sqlite.org/uri.html). |
| Medium | NULL optional reports/exchanges in older database rows prevented Recent QSOs and call history from loading. | Coalesce optional display fields when reading table rows. `TestAuditLegacyOptionalFields` inserts a minimally populated legacy row and checks both readers. Uploads still reject unreadable records, preserving the existing regression test. |
| Medium | ADIF import memory accounting omitted park names, allowing large names to bypass the byte-based batch limit. Completed batch entries also retained their string references. | Include park names in the estimate and clear completed batches. `TestAuditLargeParkNamesFlushImportBatch` verifies that a large import commits a bounded batch before encountering a malformed trailing record. |
| Low | Clearing a grid on a loaded station-profile value retained its previous coordinate pointers in the returned value. | Reset derived coordinates before resolving the new grid. `TestAuditClearStationGrid` verifies the cleared result. |
| Low | Extra operands, repeated actions, an empty filename, and conflicting `--version` actions were silently accepted by argument validation. | Consume operands explicitly and require one unambiguous action. `TestAuditRejectAmbiguousCLIArguments` and the CLI subprocess smoke test cover rejection. |
| Low | Floating-point rounding at some antipodal coordinate pairs produced NaN distances. | Clamp the haversine intermediate to its mathematical range. `TestAuditAntipodalDistanceStaysFinite` checks 1,799 antipodal pairs. |

Also removed an unused DXCC field, replaced a deprecated style-copy call,
simplified a redundant prefix check, and corrected six analyzer-reported error
messages. Added the audit regression tests to the native platform smoke workflow.

## Verification

- `go test ./...`: passed, including the large-import test and all nine new audit tests.
- `go vet ./...`: passed.
- `go test -race -short ./...`: passed. The original baseline also passed the full race suite, including the large-import test.
- `go run honnef.co/go/tools/cmd/staticcheck@v0.8.1 ./...`: passed. The older installed analyzer could not read Go 1.27 export data; the compatible version was run without replacing the installed tool.
- `go run golang.org/x/vuln/cmd/govulncheck@latest ./...`: no vulnerabilities found. This checks known dependency vulnerabilities using the [Go vulnerability database](https://go.dev/doc/security/vuln/).
- `go mod verify`: all modules verified.
- Formatting and `git diff --check`: passed.
- `make build`: passed; refreshed `bin/w4gns-logger`.
- Release builds: Linux amd64/arm64, Windows amd64, and macOS amd64/arm64 all compiled with `CGO_ENABLED=0`.
- Linux CLI subprocess smoke: version reporting, SQLite URI database creation, ADIF import, duplicate reimport, ADIF export, database/sidecar overwrite refusal, and invalid argument rejection passed using a temporary database.
- Python utility: syntax and representative inputs for all four extraction modes passed.

## Limits

The work ran on Linux with Go 1.27.0. Windows and macOS binaries were cross-compiled;
their native runtime checks require the existing GitHub Actions runners. The
updated native smoke workflow was not executed remotely during this audit.

Live QRZ/WRL delivery, Google Drive backup/retention, and graphical terminal
launching were not verified end to end against the operator's accounts or desktop.
Existing local mock/regression tests cover those integration paths. Contest
catalog validation and scoring tests passed; this code audit does not recertify
every sponsor's current rules or event dates. Existing documented scoring
limitations remain applicable.
