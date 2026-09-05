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

Field order is Call, RST Sent, RST Received, Band, Frequency, POTA Ref, IOTA Ref. Band is a closed selector; `Left`/`Right` or `Up`/`Down` chooses one of the supported CW bands and supplies a valid default frequency. Frequency is entered in MHz beside Band and is validated against US Amateur Extra-class allocation edges. Some allocations vary by ITU region and national authority, so operators remain responsible for following the narrower limits of their licence, country, and local band plan. POTA Ref and IOTA Ref are both optional, free-text (POTA Ref auto-fills from a live POTA spot lookup or a matching DX Cluster comment; IOTA Ref auto-fills from a matching DX Cluster comment only — POTA has no equivalent third-party spot API for IOTA), and clear after each QSO like Call. This page displays UTC and station-profile local time, QSO timer state, 15-minute (or contest-scoped) call-and-band dupe warning, all prior contacts for the entered callsign, and — while a callsign is typed and the terminal is wide enough — an analysis panel showing DXCC country/zone/bearing and a `POTA SPOTTED` indicator when the live POTA feed currently has a spot for that callsign. The active QSO never appears in the prior-contact list until saved.

## F9 — Browse, edit, and delete QSOs

Pressing `F9` from QSO Entry moves keyboard focus into the Recent QSOs table instead of the entry fields; the currently selected row is highlighted only while this mode is active. `Up`/`Down` move the selection, `Enter` loads the selected QSO back into the F1/F6/F7 fields for editing (its original start/end time and station-identity snapshot are preserved; only what's actually changed on screen is saved), and `d` `d` deletes it after a confirmation prompt. `Esc` or `F9` again returns focus to the entry fields without leaving the table's selection changed.

## F6 — QSO Details

Optional fields are name, QTH, grid square, state/province, county, email, park name, island name, and notes. These map to `NAME`, `QTH`, `GRIDSQUARE`, `STATE`, `CNTY`, `EMAIL`, `APP_W4GNS_LOGGER_PARK_NAME`, `APP_W4GNS_LOGGER_ISLAND_NAME`, and `COMMENT`. POTA Ref and IOTA Ref themselves are F1 fields, not F6; park name and island name are just the free-text labels for whatever reference was entered on F1. Frequency is also an F1 field. County here is optional contact metadata; the contest exchange remains authoritative for QSO-party credit. QRZ lookup and recent POTA spots can fill blank fields asynchronously. Results are tied to the contact that initiated the lookup and preserve operator-entered values; see the README for lookup configuration.

## F7 — Events & Contests

The Events & Contests page is backed by embedded JSON configuration files in `events/`; it is designed to scale to the hundreds of on-air activities without a code change for each one. An event definition supplies its stable ADIF `CONTEST_ID`, organizer, schedule, allowed bands, exchange prompts, serial requirement, dupe scope, rules URL, score-submission URL, and one or more UTC sessions. The operator selects both the event and its session; this produces a session-specific contest ID such as `CWT-1900` or `CW-OPEN-2`. The initial catalog includes all four weekly CWops Test (CWT) sessions and the three CW Open sessions. Contest Entry stores `CONTEST_ID`, `STX`, `STX_STRING`, `SRX`, and `SRX_STRING`.

Verified QSO parties use saved sent/received locations for both entrant sides and county-aware duplicates. Enter your county when inside the event state, otherwise your state/province or DX location. County names and official codes autocomplete in both exchange fields: select with `Up`/`Down`, then `Enter`. Join county-line codes with `/`; sponsor limits apply. California serials use the separate serial fields. See [State QSO party support](State_QSO_Parties.md) for configured events and limitations.

Actual contest logs include an occurrence suffix, for example `CWT-1900@20260909` or `CW-OPEN-2@2026`, so repeated sessions do not share duplicate history or scores. The sponsor's Cabrillo contest token is exported separately. For configured QSO parties, duplicate checks include both exchanged locations; a partially new county-line contact remains loggable. Missing exchanges can be saved for later correction, but checked export rejects them.

Station Setup (`F2`) supplies the declared station and power categories. Changing those categories rebuilds contest scoring; changing the profile does not rewrite saved exchanged locations. Category eligibility and physical county-line requirements remain the operator's responsibility.

## Data and reset behavior

All three entry pages (F1, F6, F7) edit one in-memory QSO. Data is written atomically when F1's final Frequency field is submitted, as a new insert or an update to the QSO loaded through `F9`. After saving, contact details and received exchange/serial clear. Band, frequency, sent report, selected contest, and sent exchange carry forward; the sent serial advances where required, and the received report returns to 599. Update the sent county when moving to a new operating location. A failed validation or database write retains the entered data. Cancelling an edit restores the previous contest-entry context without changing the database.
