# W4GNS Logger 4 Men

CW only logger, life is too short for QRM.

## Start

Build and install the application into your user-local command path:

```bash
make install
```

This creates `~/.local/bin/w4gns-logger` as a symlink to the repository build. `~/.local/bin` is on the configured Bash `PATH`, so run the application from any directory with:

```bash
w4gns-logger
```

Every `make build` or `make install` refreshes `bin/w4gns-logger`; the installed command immediately uses that rebuilt version. No `sudo` or system-wide installation is used.

You can also run the repository build directly:

```bash
./bin/w4gns-logger
```

It opens in its own terminal window. To run it in the terminal you launched it from:

```bash
./bin/w4gns-logger --in-current-terminal
```

Your log is stored locally in `w4gns.db` by default. Set `W4GNS_DB` to use another database path.

## Screens and controls

| Key | Action |
| --- | --- |
| `F1` | QSO Entry |
| `F2` | Station Setup |
| `F3` | DX Cluster |
| `F4` | Cluster Filters |
| `F5` | Import ADIF from QSO Entry |
| `F6` | QSO Details |
| `F7` | Events & Contests |
| `F8` | Back up now to Google Drive |
| `Tab` / `Shift+Tab` | Move between entry fields |
| `Enter` | Move to the next field; save a QSO from the final field |
| `Esc` | Quit from QSO Entry; cancel Station Setup; return to QSO Entry from DX Cluster |

## QSO entry

- CW QSOs are logged with callsign, band, and sent/received RST. Mode is always CW — there's no Mode field to fill in, since this logger doesn't support anything else.
- The initial entry sequence is Call, RST Sent, RST Received, Band, then Frequency.
- Band is a selector: use `Left`/`Right` or `Up`/`Down` while it is selected. Selecting a band supplies a valid default CW frequency; enter frequency in MHz.
- Frequency is checked against the selected band's amateur allocation edges. These follow long-stable US Amateur Extra-class limits (47 CFR §97.301); 160m, 80/75m, 40m, and 6m allocations vary by ITU region and national authority, so always follow the narrower rules of your licence and country.
- Callsigns are normalized to uppercase.
- While a callsign is being entered, the **Stations Worked** area shows that station's earlier contacts from the log. The QSO being entered is excluded until it is saved.
- Duplicate callsign-and-band contacts are flagged before logging. Outside a contest this uses a 15-minute window (so re-working the same station later the same day for a rag-chew or POTA activation isn't flagged). Inside a contest selected from Events & Contests (`F7`), the flag instead follows that event's own `dupe_scope`: most contests treat any repeat contact on the same band anywhere in the contest as a dupe, while CWT and CW Open scope the check to the current session only, since working the same station again in a later session is allowed.
- The first `Tab` or `Enter` after entering a callsign starts the QSO timer.
- The final `Enter` saves UTC start and end times, then clears the form for the next QSO.
- UTC and local time are displayed in the logger header.
- Current solar-weather propagation indices (SFI, A-index, K-index) are shown below the header, sourced from [N0NBH's solar-data feed](https://www.hamqsl.com/solar.html) and refreshed automatically every 30 minutes. If the feed is unreachable, the line reports why and logging is unaffected.

## Browse, edit, and delete QSOs

Press `F9` to move keyboard focus into the Recent QSOs table (the row under your cursor is highlighted only while browsing — it doesn't imply anything the rest of the time).

- `Up`/`Down` (and the table's other built-in navigation) move the selection.
- `Enter` loads the selected QSO back into the entry fields for editing — Call, RST Sent/Rcvd, Band, Frequency, and everything on the QSO Details (`F6`) and Events & Contests (`F7`) screens. An **EDITING #&lt;id&gt;** banner replaces the DUPE warning area as a reminder. The QSO's original start/end time and the station-identity snapshot it was logged with are left untouched; only what you actually change is saved. The final `Enter` saves the edit in place (no duplicate row); `Esc` discards the edit and returns to a blank entry instead of quitting the app (a plain `Esc` still quits as usual everywhere else, including a second press after cancelling an edit).
- `d` arms a delete of the selected QSO — the status bar asks for a second `d` to confirm; any other key cancels it. Deletion is permanent; there is no undo.
- `Esc` or `F9` again leaves the table and returns focus to the entry fields.

## QSO Details and Events & Contests

Press `F6` for optional QSO details: operator name, QTH, grid square, state or province, POTA reference, and notes. Press `F7` for the Events & Contests page. Select an event with `Up`/`Down`, select its UTC session with `Left`/`Right`, then press `Enter`; the session-specific ID and exchange templates populate Contest Entry. The catalog shows a scrollable window, so it accommodates hundreds of definitions. The built-in CWops definitions cover all four weekly CWT sessions and the three CW Open sessions. Each definition can include a score-submission URL; CWT points to 3830scores.com. Events are JSON files embedded from `events/`, so definitions can be added without modifying Go code. These pages retain their values until the QSO is logged from the main QSO Entry screen.

`events/contestcalendar.json` adds the next occurrence of CW-inclusive contests and QSO parties found on the [WA7BNM Contest Calendar](https://www.contestcalendar.com/) — worldwide majors, US and Canadian state/province QSO parties, and national/regional contests from Europe, Asia, South America, Africa, and Oceania. A contest is included only when its own Contest Calendar detail page confirms CW as one of its modes; entries whose only confirmed mode is something else (SSB-only, RTTY-only, etc.) or whose status is reported `Inactive` are left out. For contests that also allow other modes, the exchange hint is scoped to the CW leg specifically, and says so. Every entry's exchange hint, band list, and `rules_url` are read from that same detail page rather than recalled from memory; where no confirmed upcoming date is published, the entry says so instead of guessing one. This list is not guaranteed exhaustive — the Contest Calendar adds and retires entries over time, and gaps found during review are filled in as they're noticed rather than through a scheduled resync.

TNQP is configured for an out-of-state operator. Its received-exchange field offers Tennessee county codes as you type; use `Up`/`Down` and `Enter` to insert the official four-letter county abbreviation.

<details>
<summary>Full list of 190 built-in event/contest definitions (click to expand)</summary>

Selected directly with `F7`; grouped by region below. CWops (CWT, CW Open) and the Tennessee QSO Party ship as hand-curated definitions with typeahead exchange support; the rest come from `events/contestcalendar.json`, sourced from the [WA7BNM Contest Calendar](https://www.contestcalendar.com/) and its per-contest detail pages.

**CWops / hand-curated:**
- CWops Test (CWT)
- CW Open
- Tennessee QSO Party

**Major & club contests (worldwide/US) (44):**
- 4 States QRP Group Second Sunday Sprint
- A1Club AWT
- ARRL 160-Meter Contest
- ARRL Inter. DX Contest, CW
- ARRL Rookie Roundup, CW
- ARRL Sweepstakes Contest, CW
- ARS Flight of the Bumblebees
- ARS Spartan Sprint
- AWA Bruce Kelley 1929 QSO Party
- CQ 160-Meter Contest, CW
- CQ WW WPX Contest, CW
- CQ Worldwide DX Contest, CW
- Classic Exchange, CW
- High Speed Club CW Contest
- Homebrew and Oldtime Equipment Party
- IARU Region 1 Field Day, CW
- ICWC Medium Speed Test
- K1USN SST Open
- K1USN Slow Speed Test
- MI QRP Labor Day CW Sprint
- Mini-Test 40
- Mini-Test 80
- NCCC Sprint Ladder
- North American QSO Party, CW
- North American Sprint, CW
- Novice Rig Roundup
- PRO CW Contest
- QCX Challenge
- QRP ARCI Fall QSO Party
- QRP ARCI Holiday Spirits Sprint
- QRP ARCI Hootowl Sprint
- QRP ARCI Spring QSO Party
- QRP ARCI Summer Homebrew Sprint
- QRP ARCI Topband Sprint
- QRP Fox Hunt
- QRP to the Field
- Run for the Bacon QRP Contest
- SKCC Sprint
- SKCC Sprint Europe
- SKCC Weekend Sprintathon
- Stew Perry Topband Challenge
- Wake-Up! QRP Sprint
- Walk for the Bacon QRP Contest
- Zombie Shuffle

**United States state QSO parties (45):**
- 7th Call Area QSO Party
- Alabama QSO Party
- Arizona QSO Party
- Arkansas QSO Party
- California QSO Party
- Collegiate QSO Party
- Colorado QSO Party
- Delaware QSO Party
- Florida QSO Party
- Georgia QSO Party
- Hawaii QSO Party
- Idaho QSO Party
- Illinois QSO Party
- Indiana QSO Party
- Iowa QSO Party
- Kansas QSO Party
- Kentucky QSO Party
- Kentucky State Parks on the Air
- Louisiana QSO Party
- Maine QSO Party
- Maryland-DC QSO Party
- Michigan QSO Party
- Minnesota QSO Party
- Mississippi QSO Party
- Missouri QSO Party
- Nebraska QSO Party
- New England QSO Party
- New Hampshire QSO Party
- New Jersey QSO Party
- New Mexico QSO Party
- New York QSO Party
- North Carolina QSO Party
- North Dakota QSO Party
- Ohio QSO Party
- Oklahoma QSO Party
- Pennsylvania QSO Party
- South Carolina QSO Party
- South Dakota QSO Party
- Texas QSO Party
- U.S. Islands QSO Party
- Vermont QSO Party
- Virginia QSO Party
- Washington State Salmon Run
- West Virginia QSO Party
- Wisconsin QSO Party

**Canada (8):**
- Atlantic Canada QSO Party
- British Columbia QSO Party
- Canadian Prairies QSO Party
- NSARA Contest
- Ontario QSO Party
- Quebec QSO Party
- RAC Canada Day Contest
- RAC Winter Contest

**Europe (72):**
- AGCW Happy New Year Contest
- AGCW QRP Contest
- AGCW QRP/QRP Party
- AGCW Semi-Automatic Key Evening
- AGCW Straight Key Party
- AGCW YL-CW Party
- ARI 40/80 Contest
- ARI International DX Contest
- All Austrian 160-Meter Contest
- Balkan HF Contest
- Baltic Contest
- Croatian DX Contest
- DARC 10-Meter Contest
- DARC CW-Training Contest
- DARC Christmas Contest
- DARC Easter Contest
- DIG QSO Party, CW
- Dutch PACC Contest
- EA-QRP CW Contest
- EUCW 160m Contest
- European HF Championship
- European Union DX Contest
- GACW WWSA CW DX Contest
- German Telegraphy Contest
- HA3NS Sprint Memorial Contest
- Helvetia Contest
- His Maj. King of Spain Contest, CW
- Hungarian DX Contest
- Hungarian Straight Key Contest
- IRTS 80m Counties Contest
- LZ DX Contest
- LZ International 6-Meter Contest
- Marconi Club ARI Loano QSO Party Day
- Marconi Club ARI Loano Slow CW QSO Party
- Marconi Memorial HF Contest
- NAQCC CW Sprint
- NRAU-Baltic Contest, CW
- NTC QSO Party
- OK/OM DX Contest, CW
- Portugal Day Contest
- Portuguese Navy Day Contest - CT1DBS Memorial
- RAEM Contest
- REF 160-Meter Contest
- REF Contest, CW
- REF DDFM 6m Contest
- RSGB 1.8 MHz Contest
- RSGB 80m Autumn Series, CW
- RSGB 80m Club Championship, CW
- RSGB AFS Contest, CW
- RSGB IOTA Contest
- RSGB International Low Power Contest
- RSGB National Field Day
- Russian 160-Meter DX Contest
- Russian DX Contest
- Russian District Award Contest
- Russian Radio Team Championship
- Russian YL/OM Contest
- SP DX Contest
- Scandinavian Activity Contest, CW
- Tuesday's Telegraphy Contest
- Turkiye HF Contest
- UBA DX Contest, CW
- UBA ON Contest, 6m
- UBA ON Contest, CW
- UBA Spring Contest, 6m
- UBA Spring Contest, CW
- UK/EI DX Contest, CW
- Ukrainian DX Contest
- WAE DX Contest, CW
- Worked All Germany Contest
- YO DX HF Contest
- YU DX Contest

**Asia (10):**
- ARSI VU DX Contest
- All Asian DX Contest, CW
- Asia-Pacific Fall Sprint, CW
- Asia-Pacific Spring Sprint, CW
- EurAsia HF Championship
- JIDX CW Contest
- KCJ Topband Contest
- Keyman's Club of Japan Contest
- SEANET Contest
- YB Bekasi Merdeka Contest

**South America (6):**
- CVA DX Contest, CW
- LABRE DX Contest
- South America 10 Meter Contest
- South American Integration Contest CW
- Venezuelan Ind. Day Contest
- World Wide Argentina DX Contest

**Africa (1):**
- SARL HF CW Contest

**Oceania (1):**
- Oceania DX Contest, CW

</details>

## Station Setup

Press `F2` to maintain the active station profile:

- Profile name
- Callsign and operator name
- Maidenhead grid square
- Timezone
- Club, rig, antenna, and power

Maidenhead locators support 2, 4, 6, 8, and 10-character values. New QSOs use the active station profile.

## DX Cluster

Press `F3` to open the DX Cluster screen and connect to K3LR automatically when the active station has a callsign.

- The current source is **K3LR DX Cluster**.
- Save your callsign in Station Setup before connecting.
- Press `F5` to retry a connection and `F6` to disconnect.
- Standard `DX de` spots are shown with UTC time, spotter, frequency, callsign, and comment.
- Only spots inside the conventional CW/data segment of an enabled band are shown; phone and digital-mode activity elsewhere in the band is filtered out. The CW/data segment edges follow the same US Amateur Extra-class defaults as QSO-entry frequency validation — see [QSO entry](#qso-entry) — so operators under a different license class or country should treat this as a starting point, not a guarantee.

## Cluster Filters

Press `F4` to open Cluster Filters.

- DX (the worked/spotted station) and DE (the spotting station) criteria are available for country, ITU zone, CQ zone, and continent. Country matching is a case-insensitive substring against the country name resolved from the bundled `data/cty.dat` prefix table (e.g. typing "Germany" matches "Fed. Rep. of Germany"); ITU zone, CQ zone, and continent require an exact match. A filter field left blank is not applied. If a filter is set but the DX or DE callsign can't be resolved to a country, the spot is rejected rather than let through unfiltered.
- Only CW bands from 160M through 6M are available.
- Use `Up` and `Down` to select a band, then `Space` to enable or disable it.
- Press `Enter` to apply the selected filters and return to the DX Cluster screen.
- The selected bands and DX/DE criteria are applied to incoming spots immediately.

## Log data

- QSO timestamps are stored in UTC.
- A local-time display is shown alongside UTC.
- Station profiles and QSOs remain on your computer.
- Each QSO also stores a snapshot of the active station profile at log time (grid square, callsign, operator, rig, antenna, power), so editing the station profile later never rewrites the operating context of a past contact.
- Verified with an automated test: importing 100,000 QSOs from a single ADIF file completes in a few seconds on a typical dev machine and every record lands correctly. Exact throughput is hardware-dependent; the test only asserts correctness and a generous time budget, not a specific rate.

## Import ADIF

From QSO Entry, press `F5`, enter the ADIF file path, and press `Enter` to import.

Import an ADIF file directly into the active station profile:

```bash
./bin/w4gns-logger --import-adif path/to/log.adi
```

The importer accepts CW records and reports skipped records. Non-CW records, and records missing `CALL`/`QSO_DATE`/`BAND`, or with a malformed `TIME_ON` (ADIF Time must be 4 or 6 digits), are skipped entirely. A missing or malformed `TIME_OFF`/`QSO_DATE_OFF` does *not* skip the record — it falls back to using the start time as the end time, since many programs don't export an end time at all. The file is streamed rather than read into memory up front, and records are inserted in batches of 1,000, so peak memory is bounded by one batch, not by file size. If an import fails partway through (a malformed record later in the file, a database error), the batches inserted before the failure stay committed, and re-running the import after fixing the file skips records that already landed instead of duplicating them.

## Export ADIF

Export every QSO from the active station profile as ADIF records (targeting ADIF 3.1.7):

```bash
./bin/w4gns-logger --export-adif path/to/log.adi
```

The export preserves the QSO's CW fields, frequency, details, POTA metadata (both the legacy `SIG`/`SIG_INFO` convention and the modern `POTA_REF` field), contest fields, the station-identity snapshot (`MY_GRIDSQUARE`, `STATION_CALLSIGN`, `MY_NAME`, `MY_RIG`, `MY_ANTENNA`, `TX_PWR`), country/DXCC-entity/CQ-zone/ITU-zone context resolved from the worked callsign, and UTC start/end times. `STX`/`SRX` are only written when they parse as ADIF's integer type; non-numeric contest exchanges stay in `STX_STRING`/`SRX_STRING`. `MY_NAME` (not `OPERATOR`) carries the station operator's name — ADIF defines `OPERATOR` as the operator's *callsign*, and this app has no separate operator-callsign concept from `STATION_CALLSIGN`, so `OPERATOR` is intentionally left unset. The output is a `.adi` file, which ADIF 3.1.7 restricts to ASCII String fields; the IntlString data type (the `_INTL` field suffix, e.g. `NAME_INTL`) is only valid in ADX/XML files, so this exporter never emits it. Non-ASCII text (an accented name, for example) is transliterated to its plain ASCII base letter where a common mapping exists (e.g. "José" → "Jose"), and any other non-ASCII character becomes `?`. CWT and CW Open contest IDs are mapped to the official ADIF Contest ID List values (`CWOPS-CWT`, `CWOPS-CW-OPEN`) on export, even though the database keeps its own session-specific IDs (e.g. `CWT-1900`) for dupe checking. The export path must not be the SQLite database file.

The numeric ADIF `DXCC` field (the entity code) is populated from `data/arrl_dxcc.dat`, a table generated from the [ARRL DXCC List](https://www.arrl.org/files/file/DXCC/Current_Deleted.txt) and cross-referenced against the bundled `data/cty.dat` by primary callsign prefix — an exact identifier both lists key on, not a free-text country-name match, which would be unreliable given how differently the two sources spell the same entity's name. `cty.dat` entities ARRL doesn't recognize as a separate DXCC entity (marked with a leading `*` on their primary prefix, e.g. Sicily, African Italy, Shetland Islands — see `data/arrl_dxcc.dat`'s header comment) are left with no `DXCC` value rather than a guessed one. `COUNTRY`, `CQZ`, and `ITUZ` continue to come from `cty.dat` directly.

## QRZ Logbook upload

Every QSO logged from QSO Entry is uploaded to your [QRZ Logbook](https://www.qrz.com/docs/logbook/QRZLogbookAPI.html) in the background as soon as it saves locally.

- Put your QRZ Logbook API key in a file named `qrz.comAPIkey` in the directory you run the app from (one line, no quotes), or set the `W4GNS_QRZ_KEY` environment variable. The key file is listed in `.gitignore`, which keeps it out of `git add -A`/`git status` by default — but `.gitignore` is only a convention respected by Git; it doesn't stop a `git add -f`, doesn't restrict which local accounts can read the file, and doesn't help if the key ends up in a screenshot or shared archive. On every startup the app also checks the key file's permissions and tightens them to owner-read/write only (`0600`) if it finds the default umask left it group- or world-readable.
- If neither is set, QRZ upload is silently skipped — local logging is unaffected.
- The upload runs asynchronously and never blocks or delays logging the next QSO.
- The status bar reports `QRZ upload OK for <call> (LOGID ...)` on success or `QRZ upload failed for <call>: ...` on failure (invalid key, no active subscription, duplicate QSO, network error, etc.). A failed upload never removes the QSO from your local log.
- Requires an active QRZ XML/Logbook Data subscription; the QRZ API rejects `ACTION=INSERT` without one.

## Backups

Press `F8` at any time to back up immediately, and every shutdown backs up automatically before the app exits — whether that's `Esc`/`Ctrl+C` from the keyboard, closing the terminal window (`SIGHUP`), or `kill`/`kill -INT` from another process. A backup always finishes (or fails and reports why) before the database closes and the process exits.

- A backup takes a consistent snapshot of the database (SQLite `VACUUM INTO`, safe even while logging continues) plus a fresh full ADIF export.
- Both files are uploaded to Google Drive with [rclone](https://rclone.org/) under the remote `gdrive:W4GNS_Logger_Backups`.
- Only the 5 most recent database backups and 5 most recent ADIF backups are kept; older ones are deleted automatically after each backup.
- Requires an `rclone` binary in `PATH` with a working `gdrive` remote already configured (`rclone config`). If rclone or the remote is unavailable, the status bar (or terminal output on shutdown) reports the failure and logging continues unaffected — a failed backup never blocks or loses QSO data.
- Backups are serialized: pressing `F8` again while one is already running is ignored (the status bar shows "backup already in progress…"), and the mandatory backup-on-exit waits for any backup still in flight instead of racing it. This avoids two backups writing to the same second-resolution filenames or running `VACUUM INTO` concurrently.

## Changelog

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

## License

Copyright © 2026 Gary Penhook. W4GNS Logger 4 Men is licensed under the GNU General Public License, version 3 or any later version (GPL-3.0-or-later). See [LICENSE](LICENSE).
