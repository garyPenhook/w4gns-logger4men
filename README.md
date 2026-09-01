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

`events/contestcalendar.json` adds the next occurrence of 79 CW-only contests from the [WA7BNM Contest Calendar](https://www.contestcalendar.com/contestcal.php) — every listing whose Contest Calendar detail page reports `Mode: CW` exactly, from the majors (CQ WW CW, CQ WPX CW, ARRL DX CW, ARRL Sweepstakes CW, NAQP CW, WAE CW, Stew Perry Topband) down to club sprints (SKCC, NAQCC, K1USN, ICWC, A1 Club, AGCW, and more). Each entry's exchange hint, band list, and `rules_url` come from that same detail page rather than from memory. Contests whose detail page lists more than one mode (e.g. CW+SSB QSO parties) are left out, since this logger is CW-only.

TNQP is configured for an out-of-state operator. Its received-exchange field offers Tennessee county codes as you type; use `Up`/`Down` and `Enter` to insert the official four-letter county abbreviation.

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

## Backups

Press `F8` at any time to back up immediately, and every shutdown backs up automatically before the app exits — whether that's `Esc`/`Ctrl+C` from the keyboard, closing the terminal window (`SIGHUP`), or `kill`/`kill -INT` from another process. A backup always finishes (or fails and reports why) before the database closes and the process exits.

- A backup takes a consistent snapshot of the database (SQLite `VACUUM INTO`, safe even while logging continues) plus a fresh full ADIF export.
- Both files are uploaded to Google Drive with [rclone](https://rclone.org/) under the remote `gdrive:W4GNS_Logger_Backups`.
- Only the 5 most recent database backups and 5 most recent ADIF backups are kept; older ones are deleted automatically after each backup.
- Requires an `rclone` binary in `PATH` with a working `gdrive` remote already configured (`rclone config`). If rclone or the remote is unavailable, the status bar (or terminal output on shutdown) reports the failure and logging continues unaffected — a failed backup never blocks or loses QSO data.

## License

Copyright © 2026 Gary Penhook. W4GNS Logger 4 Men is licensed under the GNU General Public License, version 3 or any later version (GPL-3.0-or-later). See [LICENSE](LICENSE).
