# Changelog

### v1.11.0

- Logged QSOs are now also forwarded to [World Radio League](https://worldradioleague.com) alongside the existing QRZ Logbook upload. Configure it by saving an API key to `worldradioleague.comAPIkey` (same `.gitignore`/owner-only-permission handling as the QRZ key) or via `W4GNS_WRL_KEY`; a blank/missing key just disables it.
- The app version is now shown on every screen's hotkey line, and `w4gns-logger --version` prints it from the shell — makes a stale, not-yet-rebuilt binary obvious instead of silently missing recent features.

### v1.10.0

- QRZ callsign lookup now also auto-fills County and Email on the QSO Details (`F6`) screen, alongside the existing Name/QTH/Grid/State fields, and exports/imports them via ADIF's `CNTY`/`EMAIL` fields.

### v1.9.0

- Added QRZ XML callsign lookup: entering a call on QSO Entry and leaving the field (`Tab`/`Enter`) now looks it up against QRZ and auto-fills Name, QTH, Grid, and State on the QSO Details (`F6`) screen, the same way POTA Ref already auto-fills from recent spots. Existing values are never overwritten. Configure it by entering your QRZ.com username/password in Station Setup (`F2`), saved to a `qrz.comXMLlogin` file with the same `.gitignore`/owner-only-permission handling as the existing QRZ Logbook key, or via `W4GNS_QRZ_XML_USER`/`W4GNS_QRZ_XML_PASS`. This is a separate QRZ service and subscription from the existing Logbook upload.

### v1.8.0

- DX Cluster spots are now deduplicated: a station already shown on a given band is suppressed for 3 minutes instead of flooding the list every time another cluster node relays the same spot.

### v1.7.0

- Added a **DE Call Area** cluster filter: enter comma-separated digits (e.g. `2,3,4`) to only show spots from spotting stations in those US call areas. Matched directly against the spotter's callsign (including portable overrides like `W1AW/4`), independent of the existing country/ITU/CQ/continent filters.

### v1.6.0

- The terminal window size is now remembered across launches (for `xterm` and `gnome-terminal`, the two emulators confirmed to support requesting a size on their command line), instead of opening at the emulator's default size and needing to be resized by hand every time.

### v1.5.0

Addresses an external code review of data-correctness, reliability, and security issues.

**High severity**
- Fixed F9 browse/edit/delete acting on the wrong QSO while the table was showing a callsign's history instead of the default Recent QSOs list (typing a callsign into Call swaps the table's display without updating what F9 selects from).
- Fixed database startup failing outright on a genuinely old database: schema indexes were created before missing columns were migrated in, so `CREATE INDEX` on a not-yet-added column (e.g. `profile_id`) could fail before migration ever ran.
- Bounded ADIF import against a malformed or hostile file: a declared field length like `<CALL:1000000000>` no longer attempts a ~1GB allocation, and per-tag, per-field, and per-record limits stop unbounded memory use from a file with no `<`/`>` or from a record that never closes.
- The database and QRZ API key now default to stable, working-directory-independent paths (under `$XDG_DATA_HOME`/`$XDG_CONFIG_HOME`) instead of the current directory, so launching the installed command from a different directory than usual no longer silently starts a second, empty log or disables QRZ uploads. An existing `./w4gns.db` or `./qrz.comAPIkey` keeps being used unchanged.

**Medium severity**
- Fixed a real collision in the event catalog (e.g. `UBA-SPRING-CONTEST` vs `UBA-SPRING-CONTEST-2`) that could resolve a selected contest to the wrong, shorter event, using its bands/dupe_scope instead of the correct one's.
- A truncated ADIF file (cut off mid-record, no closing `<EOR>`) now reports an error instead of silently dropping the trailing record.
- Fixed the async ADIF import's result being silently lost if `Esc` left the Import ADIF screen before the import finished.
- Fixed the database and ADIF halves of one backup being able to represent different states: ADIF is now exported from the staged `VACUUM INTO` snapshot instead of the live database, which the UI doesn't block while a backup runs.
- DX cluster spot text (spotter, frequency, callsign, comment) is now stripped of ANSI escape/control characters before being stored or rendered — spots come from other operators on the cluster network and were previously rendered to the terminal unescaped.
- QRZ, POTA, and solar-data HTTP responses are now read through a size limit, guarding against an unbounded read from a misbehaving endpoint or a MITM.
- CLI ADIF export now writes to a temporary file and renames it into place, so a failure partway through no longer truncates/destroys an existing file at the target path.
- ADIF export now streams QSOs directly from the database instead of loading a station profile's entire history into memory first.
- The 100k-QSO import benchmark test no longer re-runs under `-race` in CI (the plain test run already covers it at full scale; race instrumentation doesn't test anything additional about a larger input).

**Lower priority**
- Fixed the cluster-spot POTA autofill preferring the oldest matching spot in the 15-minute window instead of the newest.
- The QSO-entry header's "Local" time now reflects the configured station profile's timezone instead of always the host machine's.
- Station callsigns are now validated the same way QSO callsigns are (letters/digits/`/` only), since a callsign is later sent as a raw line to the DX cluster's TCP connection.
- Unrecognized command-line flags and `--export-adif`/`--import-adif` missing their path argument now report a usage error instead of silently launching the TUI.

### v1.4.0

- Added an in-app way to view, edit, and delete logged QSOs: press `F9` to browse the Recent QSOs table, `Enter` to load one back into the entry fields for editing, and `d` `d` to delete it (with confirmation). The table's selection highlight — previously always shown on the most recent QSO with no interactive meaning — now only appears while actually browsing.

### v1.3.2

- Fixed the Recent QSOs table always rendering its top row (the most recent QSO) bold and pink: that was `bubbles/table`'s default cursor-highlight style leaking through even though the table isn't an interactive selector. All rows now render identically.

### v1.3.1

- Removed the Mode field from QSO entry and the header: this is a CW-only logger, so mode is always CW and no longer needs its own input. The initial entry sequence is now Call, RST Sent, RST Received, Band, then Frequency.

### v1.3.0

- ADIF export now populates the numeric `DXCC` entity code field, cross-referenced from the official ARRL DXCC List against the bundled `cty.dat` by primary callsign prefix (see the Export ADIF section) instead of being left blank.
- Expanded automated test coverage for Google Drive backups (real upload/retention/partial-failure paths against a fake `rclone`) and QRZ Logbook uploads (HTTP failures, unexpected API responses, the `qrzUploadCmd` command itself).

### v1.2.0

- ADIF `.adi` export is now ASCII-compliant: ADIF 3.1.7 restricts the IntlString data type (`_INTL` fields) to ADX/XML files, so non-ASCII text is transliterated to plain ASCII on export instead of being written under a `_INTL` field name.
- The station operator's name now exports to `MY_NAME` instead of `OPERATOR` — ADIF defines `OPERATOR` as the operator's *callsign*, not their name.
- Contest duplicate checking is re-verified against the database immediately before a QSO is logged, and the on-screen dupe indicator now updates whenever the selected contest changes, not just the callsign or band.
- Logging a QSO on a band outside the selected event's allowed bands is now rejected before it's saved, instead of being saved with a warning appended afterward.
- The solar indices line now shows the source's own "as of" timestamp, and keeps showing the last known-good values with a stale marker (instead of silently going quiet) if a refresh fails.
- Station power now rejects `NaN` and `Inf` instead of accepting them as valid wattage.
- ADIF import is now fully streamed — the source file is never read into memory all at once, only one field/batch at a time.
- DX cluster filter fields are labelled "DX/DE Country" instead of "DX/DE DXCC", matching what they actually match against (a country name, not a numeric DXCC entity code).
- CI now also runs `go test -race` and `govulncheck`.

### v1.1.1

- The solar indices line is now bold yellow instead of dim gray, so it's easier to spot at a glance.

### v1.1.0

- Live solar propagation indices (SFI, A-index, K-index) now display below the header on the QSO entry screen, sourced from N0NBH's solar-data feed and refreshed automatically every 30 minutes.

### v1.0.0

- Imported ADIF `COUNTRY`/`CQZ`/`ITUZ` fields are preserved instead of being silently overwritten by a local `cty.dat` guess.
- `OPERATOR`, `MY_RIG`, and `MY_ANTENNA` now export as `_INTL` fields when they contain non-ASCII characters, matching ADIF's String/IntlString rules.
- Dupe checking is scoped per station profile, so working the same call/band under a different profile is no longer flagged as a dupe.
- Re-running an ADIF import after a mid-file failure skips records that already landed instead of duplicating them.
- DX cluster and ADIF-import DXCC lookups use an indexed prefix match instead of a full linear scan.
- An unrecognized, free-typed contest name now surfaces a status message explaining that dupe checking fell back to the casual 15-minute window.
