# QSO Entry Design

## Purpose

Keep routine CW logging fast in a small terminal while making optional QSO and contest data available before a QSO is saved.

## Build and installation

`make install` builds the executable at `bin/w4gns-logger` and creates the user-local command `~/.local/bin/w4gns-logger` as a symlink to it. Because `~/.local/bin` is on the configured Bash `PATH`, the logger can be started as `w4gns-logger` from any directory. The symlink follows every subsequent `make build` or `make install`, so it always runs the most recently rebuilt repository binary. This installation is entirely user-owned and requires no `sudo`.

Persistent files use stable per-user locations so launching from another directory does not create a second log. Unless `W4GNS_DB` overrides it or an existing legacy `./w4gns.db` is found, the database lives under `$XDG_DATA_HOME/w4gns-logger/` (normally `~/.local/share/w4gns-logger/`). Configuration lives under `$XDG_CONFIG_HOME/w4gns-logger/` (normally `~/.config/w4gns-logger/`). When the application launches its own `xterm` or `gnome-terminal` window, it saves the last character-cell dimensions in the configuration directory and reapplies them at the next launch; other supported terminal emulators use their own default size because no equivalent command-line option has been confirmed. Running with `--in-current-terminal` does not create a new window.

## Pages and navigation

| Key | Page | Purpose |
| --- | --- | --- |
| `F1` | QSO Entry | Fast, required QSO fields and prior-contact view. |
| `F6` | QSO Details | Optional station and operating details. |
| `F7` | Events & Contests | Select a configured event or contest, then enter its exchange. |
| `F9` | Browse/Edit (from QSO Entry) | Select a logged QSO from Recent QSOs to edit or delete. |

`Esc` returns to QSO Entry. `Tab`, `Shift+Tab`, and `Enter` move among fields on the secondary pages. Saving remains an F1-only action: `Enter` on the final Frequency field writes the QSO (or, while editing an existing one via `F9`, saves the edit in place) and begins a blank next QSO. There is no Mode field — this logger is CW-only, so mode is always "CW" internally with nothing to enter.

## F1 — QSO Entry

Field order is Call, RST Sent, RST Received, Band, Frequency. Band is a closed selector; `Left`/`Right` or `Up`/`Down` chooses one of the supported CW bands and supplies a valid default frequency. Frequency is entered in MHz beside Band and is validated against US Amateur Extra-class allocation edges. Some allocations vary by ITU region and national authority, so operators remain responsible for following the narrower limits of their licence, country, and local band plan. This page displays UTC and station-profile local time, QSO timer state, 15-minute (or contest-scoped) call-and-band dupe warning, and all prior contacts for the entered callsign. The active QSO never appears in the prior-contact list until saved.

## F9 — Browse, edit, and delete QSOs

Pressing `F9` from QSO Entry moves keyboard focus into the Recent QSOs table instead of the entry fields; the currently selected row is highlighted only while this mode is active. `Up`/`Down` move the selection, `Enter` loads the selected QSO back into the F1/F6/F7 fields for editing (its original start/end time and station-identity snapshot are preserved; only what's actually changed on screen is saved), and `d` `d` deletes it after a confirmation prompt. `Esc` or `F9` again returns focus to the entry fields without leaving the table's selection changed.

## F6 — QSO Details

Optional fields are name, QTH, grid square, state/province, POTA reference, and notes. These map to the corresponding ADIF-shaped QSO columns: `NAME`, `QTH`, `GRIDSQUARE`, `STATE`, `SIG`/`SIG_INFO`, and `COMMENT` (Frequency itself is an F1 entry-row field, not part of F6). A POTA reference is populated only when a matching callsign appears in the previous 15 minutes in the local DX-cluster spot history or POTA active-spot feed. The network lookup has a bounded timeout and never blocks logging.

## F7 — Events & Contests

The Events & Contests page is backed by embedded JSON configuration files in `events/`; it is designed to scale to the hundreds of on-air activities without a code change for each one. An event definition supplies its stable ADIF `CONTEST_ID`, organizer, schedule, allowed bands, exchange prompts, serial requirement, dupe scope, rules URL, score-submission URL, and one or more UTC sessions. The operator selects both the event and its session; this produces a session-specific contest ID such as `CWT-1900` or `CW-OPEN-2`. The initial catalog includes all four weekly CWops Test (CWT) sessions and the three CW Open sessions. Contest Entry stores `CONTEST_ID`, `STX`, `STX_STRING`, `SRX`, and `SRX_STRING`.

The Tennessee QSO Party (TNQP) definition is configured for an out-of-state operator: the sent exchange is the operator's state, province, or DX prefix, while the received exchange supports type-ahead against all 95 official Tennessee county abbreviations. When the received-exchange field is focused, type a county name or code, use `Up`/`Down` to select a match, then press `Enter` to insert its four-letter abbreviation.

## Data and reset behavior

All three entry pages (F1, F6, F7) edit one in-memory QSO. Data is written atomically only when F1's final Frequency field is submitted — either as a new insert, or, when a QSO was loaded via `F9`, as an update to that same row. After a successful save, every QSO, details, and contest field is cleared (except Band, Frequency, and RST Sent, which persist as defaults for the next contact) so details cannot carry accidentally into the next contact. A failed validation or database write retains all entered data and reports the error; cancelling an edit with `Esc` discards the in-progress changes without touching the database.
