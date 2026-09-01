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

- CW QSOs are logged with callsign, band, mode, and sent/received RST.
- The initial entry sequence is Call, RST Sent, RST Received, Band, Frequency, then Mode.
- Band is a selector: use `Left`/`Right` or `Up`/`Down` while it is selected. Selecting a band supplies a valid default CW frequency; enter frequency in MHz.
- Frequency is checked against the selected band’s international amateur allocation. Always follow the narrower rules of your licence and country.
- Callsigns are normalized to uppercase.
- While a callsign is being entered, the **Stations Worked** area shows that station's earlier contacts from the log. The QSO being entered is excluded until it is saved.
- Duplicate callsign-and-band contacts are flagged before logging.
- The first `Tab` or `Enter` after entering a callsign starts the QSO timer.
- The final `Enter` saves UTC start and end times, then clears the form for the next QSO.
- UTC and local time are displayed in the logger header.

## QSO Details and Events & Contests

Press `F6` for optional QSO details: operator name, QTH, grid square, state or province, POTA reference, and notes. Press `F7` for the Events & Contests page. Select an event with `Up`/`Down`, select its UTC session with `Left`/`Right`, then press `Enter`; the session-specific ID and exchange templates populate Contest Entry. The catalog shows a scrollable window, so it accommodates hundreds of definitions. The built-in CWops definitions cover all four weekly CWT sessions and the three CW Open sessions. Each definition can include a score-submission URL; CWT points to 3830scores.com. Events are JSON files embedded from `events/`, so definitions can be added without modifying Go code. These pages retain their values until the QSO is logged from the main QSO Entry screen.

`events/contestcalendar.json` adds the next occurrence of every CW-inclusive contest and QSO party found on the [WA7BNM Contest Calendar](https://www.contestcalendar.com/) — worldwide majors, US and Canadian state/province QSO parties, and national/regional contests from Europe, Asia, South America, Africa, and Oceania. A contest is included only when its own Contest Calendar detail page confirms CW as one of its modes; entries whose only confirmed mode is something else (SSB-only, RTTY-only, etc.) or whose status is reported `Inactive` are left out. For contests that also allow other modes, the exchange hint is scoped to the CW leg specifically, and says so. Every entry's exchange hint, band list, and `rules_url` are read from that same detail page rather than recalled from memory; where no confirmed upcoming date is published, the entry says so instead of guessing one.

TNQP is configured for an out-of-state operator. Its received-exchange field offers Tennessee county codes as you type; use `Up`/`Down` and `Enter` to insert the official four-letter county abbreviation.

<details>
<summary>Full list of 186 built-in event/contest definitions (click to expand)</summary>

Selected directly with `F7`; grouped by region below. CWops (CWT, CW Open) and the Tennessee QSO Party ship as hand-curated definitions with typeahead exchange support; the rest come from `events/contestcalendar.json`, sourced from the [WA7BNM Contest Calendar](https://www.contestcalendar.com/) and its per-contest detail pages.

**CWops / hand-curated:**
- CWops Test (CWT)
- CW Open
- Tennessee QSO Party

**Major & club contests (worldwide/US) (42):**
- 4 States QRP Group Second Sunday Sprint
- A1Club AWT
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

**Europe (70):**
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
- LZ DX Contest
- LZ International 6-Meter Contest
- Marconi Club ARI Loano QSO Party Day
- Marconi Club ARI Loano Slow CW QSO Party
- Marconi Memorial HF Contest
- NAQCC CW Sprint
- NRAU-Baltic Contest, CW
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

## Cluster Filters

Press `F4` to open Cluster Filters.

- DX and DE criteria are available for DXCC, ITU zone, CQ zone, and continent.
- Only CW bands from 160M through 6M are available.
- Use `Up` and `Down` to select a band, then `Space` to enable or disable it.
- Press `Enter` to apply the selected filters and return to the DX Cluster screen.
- The selected bands are applied to incoming spots immediately.

## Log data

- QSO timestamps are stored in UTC.
- A local-time display is shown alongside UTC.
- Station profiles and QSOs remain on your computer.
- The logger is suitable for large logs, including ADIF imports of 100,000 QSOs.

## Import ADIF

From QSO Entry, press `F5`, enter the ADIF file path, and press `Enter` to import.

Import an ADIF file directly into the active station profile:

```bash
./bin/w4gns-logger --import-adif path/to/log.adi
```

The importer accepts CW records, preserves QSO times when present, and reports skipped records. Non-CW or incomplete records are skipped.

## Export ADIF

Export every QSO from the active station profile as ADIF 3 records:

```bash
./bin/w4gns-logger --export-adif path/to/log.adi
```

The export preserves the QSO's CW fields, frequency, details, POTA metadata, contest fields, and UTC start/end times. The export path must not be the SQLite database file.

## QRZ Logbook upload

Every QSO logged from QSO Entry is uploaded to your [QRZ Logbook](https://www.qrz.com/docs/logbook/QRZLogbookAPI.html) in the background as soon as it saves locally.

- Put your QRZ Logbook API key in a file named `qrz.comAPIkey` in the directory you run the app from (one line, no quotes), or set the `W4GNS_QRZ_KEY` environment variable. The key file is git-ignored so it is never committed.
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

## License

Copyright © 2026 Gary Penhook. W4GNS Logger 4 Men is licensed under the GNU General Public License, version 3 or any later version (GPL-3.0-or-later). See [LICENSE](LICENSE).
