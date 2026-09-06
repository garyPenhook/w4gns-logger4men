# Independent code review — September 6, 2026

Audited commit: `ecc3b1f61302938fd66504c928e078fde7b9e824`.

The initial review found five reproducible **P2 / medium** issues. No critical or high-severity defect was confirmed. All five are now fixed in the working tree, with permanent regression coverage. The descriptions below preserve the original findings against the audited commit; line references refer to that revision.

## Remediation

- Callsign changes clear POTA/IOTA references belonging to the previous unsaved station. Repeated navigation on the same call and edits to saved QSOs preserve their references.
- CSV formula-like cells receive an apostrophe prefix and CSV quoting, including formula prefixes after whitespace and their full-width forms. Ordinary numeric fields and stored QSO values remain unchanged.
- POTA spots wait only for admitted lookups. Rejected admissions publish fallbacks immediately; waiting spots are capped at 25 per park and 500 overall. The periodic tick expires old waits, and expired cache entries can be reclaimed.
- QRZ and late POTA results enrich retained reports. The store tracks revisions separately from stable report IDs, so connected browsers receive location updates without duplicates or changed receipt timestamps. QRZ cannot replace a park, explicit override, or report-attributed grid.
- A shared coordinate validator rejects NaN, infinity, and out-of-range latitude/longitude in QRZ parsing, QRZ/POTA location construction, and cache admission.

The original five reproductions and additional cases now live in [`independent_audit_test.go`](../cmd/w4gns-logger/independent_audit_test.go). Additional tests cover CSV prefixes, cache expiry, geographic bounds, concurrent enrichment, and real loopback SSE delivery/reconnection.

Post-fix verification passed: `go test ./...`, `go vet ./...`, `go test -race -short ./...`, the final review regression group under the race detector, staticcheck v0.8.1, and govulncheck (no known vulnerabilities). `make build` refreshed `bin/w4gns-logger`; Windows amd64 and macOS arm64 cross-builds passed. Spreadsheet GUI and live external-service checks remain outside the verified scope below.

## Findings

### 1. Changing an unsaved callsign retains the previous station's award references

**Location:** [`main.go:2012`](../cmd/w4gns-logger/main.go), `resetDetailsForCall`; related autofill at lines 1898–1923 and 2638–2659.

Enter station A and let POTA autofill its reference. Return to Call, replace A with B, and leave Call again. `resetDetailsForCall` clears the details widgets but leaves `fieldPOTARef` and `fieldIOTARef` intact. Both the cluster autofill and POTA response handler only populate an empty reference, so B's actual park cannot replace A's value. Saving commits the incorrect reference and can subsequently export/upload it.

**Reproduction:** A=`W1AW`, POTA=`US-1234`, IOTA=`NA-013`; replace with `K2ABC` and deliver its lookup result for `US-5678`. The saved K2ABC record still contains `US-1234` and `NA-013`.

**Correction:** Bind award references to the callsign that supplied them and clear them when an unsaved form changes stations. Preserve the deliberate existing-QSO editing behavior. Cover both cluster-derived and API-derived references and a change back to a previously entered callsign.

### 2. CSV export leaves untrusted values executable as spreadsheet formulas

**Location:** [`csv_export.go:15`](../cmd/w4gns-logger/csv_export.go), `csvField`; exported reports/exchanges at lines 67–79.

`csvField` strips controls and handles CSV delimiters but does not neutralize formula prefixes. Imported ADIF reports/exchanges and locally entered free text reach these columns without contest-submission validation. A received exchange of `=1+1` survives persistence and CSV parsing unchanged. CSV quoting alone does not make such values spreadsheet text. Spreadsheet interpretation of untrusted cells is documented by [OWASP's CSV Injection guidance](https://owasp.org/www-community/attacks/CSV_Injection).

**Impact:** Opening a supplied log's export in a spreadsheet can evaluate attacker-supplied formulas. More serious effects depend on the spreadsheet, its settings, and user interaction; this review did not attempt code execution or data exfiltration.

**Correction:** Define a spreadsheet-safe export policy for textual fields and test it in the supported spreadsheet applications. Neutralize formula-leading values after normalization, while retaining ordinary numeric columns. If exact machine-readable CSV is required too, distinguish that purpose explicitly; there is no universal CSV escaping policy for every spreadsheet and downstream consumer.

### 3. Full POTA cache strands map spots and bypasses the intended memory bound

**Location:** [`main.go:2941`](../cmd/w4gns-logger/main.go), unresolved-POTA branch; [`geo_cache.go:56`](../cmd/w4gns-logger/geo_cache.go), `startIfNeeded`; pending queue at `main.go:1959`.

The cluster handler queues a spot before checking whether its park lookup can actually start. Once the park cache reaches its 20,000-key capacity, `startIfNeeded` rejects a new key. No command runs and no `potaGeoMsg` can flush that key's pending spots. Cache entries are not evicted, including expired entries for unrelated keys.

**Reproduction:** Set cache capacity to one and fill it, then feed a CW spot mentioning a different park. There are zero published map reports, one retained pending spot, and no lookup marked pending. This is the same admission branch used at the production limit.

**Impact:** The first 25 spots for each newly encountered reference remain invisible indefinitely. The cap is per reference, so further distinct references continue growing `pendingPOTASpots` despite the cache and report-store limits.

**Correction:** Distinguish cache hits, already-pending lookups, newly admitted lookups, and rejected admissions. Immediately publish fallback locations on rejection. Add a global pending-spot bound and an expiry/eviction policy.

### 4. Completed QRZ lookups do not update the spot that triggered them

**Location:** [`main.go:2668`](../cmd/w4gns-logger/main.go), `qrzMapGeoMsg` handler; initial publication at lines 2950–2952.

A non-POTA spot is published using the current cache before its QRZ lookup starts. The result handler subsequently updates only `qrzGeoCache`; retained reports keep their original `DXLocation` and `SpotterLocation`. The browser receives immutable report values and has no enrichment-update path. A station spotted only once therefore stays at its country-reference location even after QRZ returns precise coordinates. Repeated stations can retain a mixture of old and new locations.

**Reproduction:** Publish W1AW's first CW spot, then deliver a successful QRZ result at 41.7, -72.7. Its retained report remains at the US reference coordinate 37.6, -91.87 with `SourceCountryReference`.

**Correction:** Update affected retained reports and notify connected browsers through an explicit update/reset mechanism, or delay initial publication until bounded resolution completes. Preserve the stronger POTA-location precedence when enriching DX positions.

### 5. Non-finite QRZ coordinates can stop the entire map stream

**Location:** [`qrz_lookup.go:240`](../cmd/w4gns-logger/qrz_lookup.go), `parseQRZLatLon`; stream serialization at [`server.go:183`](../internal/mapserver/server.go).

Coordinate validation checks only whether `strconv.ParseFloat` returns an error. `NaN` and infinities parse successfully. `qrzRecordLocation` accepts them, and a subsequent report embeds the invalid location. JSON cannot serialize these values; `events` responds to the marshal error by returning, closing that browser's stream. A reconnect's complete snapshot encounters the same invalid report and fails again.

**Reproduction:** Parse latitude `NaN` and longitude `-72.7`, cache the resulting location, and feed a W1AW spot. Serializing the retained reports fails with `json: unsupported value: NaN`.

**Impact:** A malformed upstream coordinate can make the whole map repeatedly disconnect while a poisoned report is retained. The bad cached location can also poison subsequent spots until it is refreshed. Out-of-range finite coordinates are accepted by this parser too.

**Correction:** Require finite latitude/longitude and geographic bounds before constructing a location. Apply the same checks to POTA coordinates. Treat an invalid coordinate as an unavailable location, retaining the rest of the report.

## Verification and reproducibility

The following checks passed on Linux amd64 with Go 1.27.0:

- `go test ./...` — including the large ADIF import test.
- `go vet ./...`.
- `go test -race -short ./...`.
- `go run honnef.co/go/tools/cmd/staticcheck@v0.8.1 ./...`.
- `go run golang.org/x/vuln/cmd/govulncheck@latest ./...` — no known vulnerabilities reported.
- `go mod verify`.
- Native Linux amd64 build and `--version` smoke check (1.42.0); Windows amd64 and macOS arm64 cross-compilation.
- Go source formatting and `git diff --check`.
- Python utility syntax and representative extraction inputs for all four supported formats.

The five original reproductions failed on the audited revision with the symptoms described above. They have been promoted to normal regression tests and now pass with the fixes. Run the review regression group from this checkout:

```sh
go test ./cmd/w4gns-logger -run '^TestIndependentAudit' -count=1 -v
```

The regression tests use temporary databases and synthetic messages; they do not read operator credentials or contact external services.

## Scope and limits

Review covered database opening/migrations, profile and QSO persistence, import/export boundaries, durable upload scheduling/retries, backup staging/retention, UI entry/edit state, background-result handling, network clients, terminal sanitization, contest occurrence/scoring/submission paths, map retention/server/browser code, the county-import utility, and CI/release configuration. Existing regression coverage and the earlier `Project_Audit.md` were consulted; its prior fixes are not counted as new findings.

Positive controls include transactional QSO/outbox insertion, export snapshots and atomic replacement, database/sidecar collision guards, bounded ADIF parsing, upload destination bindings, terminal control filtering, and authenticated loopback map access.

Live QRZ/WRL account delivery, Google Drive backup/retention, graphical terminal launch, and interactive browser/spreadsheet behavior were not exercised end to end. Native Windows/macOS runtime behavior and hosted CI execution were not verified. Contest logic was reviewed against the bundled catalog and tests; every sponsor's current rules, dates, and historical applicability were not independently recertified. Passing tests and vulnerability scans do not establish the absence of other defects.
