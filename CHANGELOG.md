# Changelog

### v1.29.0

- Wired the Oceania DX Contest, CW's real scoring rules: a flat points value per QSO looked up solely by band (20/10/5/1/2/3 points on 160/80/40/20/15/10M), times the existing CQ WPX-style prefix multiplier counted per band instead of once per contest, sourced from oceaniadxcontest.com's official rules. New `pointsRule.PerBand` schema is the first points formula with no country/continent classification at all. Also fixed a pre-existing catalog de-dup gap: the generated duplicate's Cabrillo token was missing the contest's mode suffix, so it never matched the curated entry's own (correct) token.

### v1.28.0

- Wired the Worked All Germany Contest's real scoring rules: a German entrant scores same-country/continent/other-continent tiers plus a DXCC/WAE country multiplier; a non-German entrant (this app's own profile) scores a flat 3 points per QSO plus a new district multiplier parsed from the worked station's DOK, sourced from darc.de's WAG rules.

### v1.27.0

- Wired the Stew Perry Topband Distance Challenge's real scoring rules: 1 point minimum plus 1 more point for every 500 km of great-circle distance between the two stations' grid squares, with no multiplier ("There is no multiplier for different grids worked") — sourced from kkn.net/stew's official rules. New `pointsRule.Distance` schema is the first continuous (non-tiered) points formula in the catalog; new `"none"` multiplier kind lets a contest declare it genuinely has no multiplier without `scoringRules` rejecting the config. Deliberately out of scope: the rules' 2x/4x per-QSO bonus for working a Low Power/QRP station and the operator's own 1.5x/3x final-score bonus for its own declared power class — neither is exchanged over the air (only the grid square is) or captured anywhere in this app's QSO model.

### v1.26.0

- Wired the Russian DX Contest's real scoring rules: same-country/same-continent/other-continent points (2/3/5) for every entrant, except a non-Russian entrant scores a flat 10 for any Russian-flagged contact (European Russia, Asiatic Russia, Kaliningrad, or Franz Josef Land), times an oblast multiplier (the contest's own 91-code table) and a "DXCC + WAE" country multiplier, both counted per band — sourced from rdxc.org's official rules.

### v1.25.0

- Wired the Helvetia Contest's real scoring rules: a Switzerland contact scores 10 points regardless of the operator's own location, a same-continent contact 1, and a different-continent contact 3, times a DXCC-country and Swiss-canton multiplier both counted per band — sourced from uska.ch's official rules (issued March 2026). Unlike SAC/WAE/ARRL DX CW, this contest's formula is the same for every entrant, so no side-asymmetric DX-side rules were needed.

### v1.24.0

- Wired the WAE DX Contest, CW's real scoring rules: a flat 1 point per QSO, plus a side-asymmetric multiplier (WAE Country List entities for a non-European entrant, non-European DXCC entities for a European entrant) weighted by a per-band bonus factor, sourced from darc.de's WAEDC rules.

### v1.23.0

- Wired the North American Sprint, CW's real scoring rules: flat 1 point per QSO times a state/province/other-North-America-entity multiplier (the same table NAQP CW uses) counted once for the whole contest rather than per band, sourced from ncjweb.com's Sprint rules. Also fixed a pre-existing data bug where this contest was configured with no sent serial number, despite the rules requiring one in every exchange.

### v1.22.0

- Wired the IARU HF World Championship, CW's real scoring rules: a zone-tiered points formula (1 point for the worked station's own ITU zone or an IARU HQ/Official contact, 3 for a different zone on your own continent, 5 for a different continent) times a multiplier for every distinct ITU zone and HQ/Official station worked per band, sourced from contests.arrl.org's official rules. New `pointsRule.Zone` schema and `iaru_zone`/`iaru_hq` multiplier kinds read the worked station's actually-exchanged zone/abbreviation rather than a callsign lookup, since IARU's exchange (not geography) is what's scored.

### v1.21.0

- Wired the ARRL November Sweepstakes, CW's real scoring rules: flat 2 points per QSO times an ARRL/RAC-section multiplier counted once for the whole contest, sourced from contests.arrl.org's official rules. Also adds a dupe check that spans every band rather than one per band, matching Sweepstakes' "each station may be contacted only once, regardless of band" rule.

### v1.20.0

- Wired the North American QSO Party, CW (NAQP CW)'s real scoring rules: flat 1 point per QSO times a state/province/other-North-America-entity multiplier counted again on every band, sourced from ncjweb.com's NAQP rules.
- QSO Entry no longer shows RST Sent/Rcvd fields for a contest that doesn't exchange RST (CW Open, NAQP CW) — previously both fields were always shown with an unused "599" default, disagreeing with what the contest actually exchanges and wasting keystrokes tabbing past them.

### v1.19.0

- Fixed selecting an event on the Events screen leaving you unable to log: it landed on the Contest Entry screen, which has no Call field, instead of QSO Entry. Selecting an event now returns to QSO Entry with Call focused so you can start logging immediately.
- `F7` from QSO Entry/QSO Details now opens Contest Entry (to set the one-time sent-exchange, e.g. your name for CW Open) when a contest is active, instead of always reopening the Events catalog; the catalog is still one `F7` away from Contest Entry.
- Wired the Scandinavian Activity Contest, CW (SAC-CW)'s real scoring rules: side-asymmetric points/multipliers for a Scandinavian vs. non-Scandinavian entrant, sourced from sactest.net's rules.

### v1.18.0

- Added POST (after-contest) entry mode for re-logging QSOs from a paper log. `Ctrl+P` toggles it on QSO Entry, adding a Date/Time (UTC) field to the entry row; logging a QSO uses that typed timestamp instead of the live clock, and refuses to log (with an explanatory message) if it can't be parsed. The field keeps its value between QSOs so only the time needs editing for consecutive entries, and is hidden while editing an existing QSO since edits don't rewrite a QSO's stored time.

### v1.17.0

- Added an in-app Help screen (`Ctrl+G`, from any screen) listing every hotkey, QSO Entry editing key, and contest-active tool (analysis panel, Check Partial, rate meter, zone auto-fill) so you don't need `docs/ROADMAP.md` open to find a command. Esc/`Ctrl+G` return to whichever screen you opened it from; `F1` still always goes to QSO Entry.

### v1.16.1

- Logged QSOs and their pending uploads are now durable across an OS crash or power loss, not just a clean process exit (the database is fsynced on every commit). This closes a gap where the last few QSOs logged before a power drop — and their queued QRZ/WRL uploads — could be lost silently.
- The DX cluster feed now reconnects automatically with exponential backoff when the connection drops (cluster nodes restart routinely), instead of silently freezing the DX Spots panel until you manually reconnect. A dead-but-not-closed connection is now also detected via an idle read timeout rather than hanging indefinitely while still showing "connected".
- ADIF import is more robust: field length prefixes are parsed strictly as decimal (a zero-padded length like `010` was previously misread as octal, corrupting the field), and a single unimportable record is now skipped rather than aborting the entire import.
- QSO validation now rejects malformed grid squares before they can be stored, re-exported, or uploaded to WRL.
- Deleting a QSO now also removes its pending upload-queue entries, so the uploader no longer keeps retrying a contact that no longer exists.
- DXCC country/zone enrichment no longer breaks if the bundled `cty.dat` is refreshed with a file that carries the standard lat/lon, continent, or UTC per-alias override tokens.
- POTA auto-fill is more resilient: park references are matched more precisely in cluster comments (common tokens like `RST-599` are no longer mistaken for a reference), and spot timestamps are parsed tolerantly so a feed format change doesn't quietly stop it working.
- Assorted hardening: the database file is created owner-only from the start, edits and the imported station callsign are validated more strictly, in-flight ADIF/Cabrillo exports are drained on exit so they can't be cut off by shutdown, and the "Stations Worked" header no longer lingers after the table is refreshed. Release downloads now ship a `SHA256SUMS` file.

### v1.16.0

- QRZ/WRL uploads are now durable. Every logged QSO is recorded in a persistent upload outbox in the database — one entry per destination — that survives a crash, quit, or transient upload failure. Pending deliveries are retried automatically on the next launch and on a periodic timer with exponential backoff until each destination accepts them (a QSO deleted before it's accepted is dropped from the queue rather than sent). Previously an upload lived only in a single in-memory 60-second timer and was lost entirely if the app closed, crashed, or the upload failed before it fired.
- macOS and Windows (and minimal Linux hosts) now start correctly. When no supported terminal emulator can be launched, the app runs in the current terminal instead of exiting — previously it quit with an error unless you knew to pass `--in-current-terminal`.
- Recent QSOs, call history, the QSO count, and edit/delete are now scoped to the active station profile, matching the dupe check and exports. A multi-profile database no longer shows or lets you modify QSOs belonging to another profile.
- QRZ callsign-lookup credentials (and session keys) can no longer leak into the status bar: transport-error messages that used to embed the full request URL — which carries your username/password or session key — are now redacted.
- Cabrillo export is now written atomically (a temporary file is renamed into place) so a failure partway through can't destroy a previously exported submission, and imported QSO/header fields are sanitized so control characters or over-long values can't inject or shift lines in the output.
- A QRZ callsign lookup that resolves after you've already logged the QSO now patches the correct row even when two same-callsign QSOs are logged in quick succession.
- ADIF import is hardened against malformed/hostile files (bounded per-record and per-batch memory) and now rejects unsupported bands even when the frequency field is blank. A second import can no longer be started while one is running, and an in-flight import is cancelled and drained cleanly on exit.
- Cluster Filters band changes now only take effect when you press Enter — pressing Esc discards them — and applying filters immediately drops already-buffered spots that no longer match.
- Assorted robustness fixes: CLI export refuses to overwrite the database through a symlink, timestamped ADIF exports no longer collide within the same second, backup retention still runs after a partial upload failure, dead DX-cluster sockets are closed explicitly, conflicting/incomplete command-line flags are rejected, and the event catalog is validated more strictly at load.

### v1.15.6

- The DX Spots panel can now also be scrolled with the mouse wheel, in addition to `PgUp`/`PgDn`. Mouse support (`tea.WithMouseCellMotion`) is now enabled app-wide. This is the more reliable option on setups where PgUp/PgDn gets captured by the terminal emulator, a multiplexer (tmux/screen), or the window manager before it reaches the app.
- The status bar now shows the visible range while scrolling the DX Spots panel (e.g. `DX Spots 11-20 of 37`), making it obvious whether a scroll input registered.

### v1.15.5

- The DX Spots panel on QSO Entry can now be scrolled: `PgUp`/`PgDn` page through all buffered spots (up to 100), not just the most recent 10. The panel title shows a `(PgUp/PgDn)` hint whenever there are more spots than fit on screen.

### v1.15.4

- QRZ Logbook and WRL uploads now wait 60 seconds after a QSO is logged before sending it, instead of firing immediately. This gives a window to catch a mistyped call or other field and correct it (`F9`, then `Enter` on the QSO) before it goes out — the upload picks up whatever the QSO looks like when the buffer expires, so an edit or delete made within that window is what actually gets sent (or not sent, if deleted).

### v1.15.3

- Filtered RTTY (and other digital-mode) spots out of the "CW only" DX Cluster/DX Spots feed. RTTY shares the same data sub-band as CW on most bands, so the existing frequency-range filter alone couldn't tell them apart; spots whose comment names a non-CW mode (`RTTY`, `PSK31`, `FT8`, `FT4`, `JS8`, `JT65`, `JT9`, `SSB`, etc.) are now rejected too.

### v1.15.2

- Fixed the DX cluster getting stuck showing "connecting to dx.k3lr.com:23…" forever: the connection result was only handled while on the DX Cluster (`F3`) screen, but the connection now starts at app launch while the operator is on QSO Entry. The TCP connection was actually succeeding in the background the whole time — the success message just had nowhere to land, so `clusterConnecting` never cleared and no spots ever populated the DX Spots panel.

### v1.15.1

- Fixed Station Setup, QSO Details, Contest Entry, and Cluster Filters rendering one field per row instead of two: writing two multi-line bordered field boxes to the screen back to back had always just stacked them vertically, not placed them side by side. With Station Setup's field count grown by Cabrillo's category/address fields, this pushed the page to 64 lines — tall enough that Callsign scrolled out of view with no way to scroll back in alt-screen mode. Fixed by joining each pair of fields properly; Station Setup is now 39 lines with Callsign in the first row.
- The DX cluster connection now also retries when Station Setup is saved with a callsign, not only at app startup — an operator who fills in Station Setup after launch (e.g. on first run) previously had no way to trigger the connection short of manually visiting the DX Cluster (`F3`) screen.

### v1.15.0

- Added a DX Spots panel filling the empty space beside Recent QSOs on QSO Entry: live CW spots across all bands, from the same feed and Cluster Filters (`F4`) as the full DX Cluster (`F3`) screen. Hidden automatically on terminals too narrow to fit it.
- The app now connects to K3LR automatically at launch (once a station callsign is configured), not only when visiting the DX Cluster screen, so the new panel has spots to show right away.

### v1.14.0

- Added a Park Name field to QSO Details (`F6`), auto-filled from recent POTA spots alongside the existing POTA Ref field — including when a spot has a name but no reference code, so the park is still recorded even without its number. Local-only: there's no standard ADIF field for it, so it isn't exported/imported or sent to QRZ/WRL.
- Fixed editing an existing QSO (`F9` → `Enter`) silently discarding any change to County or Email — they were missing from the fields the save actually carried forward, a gap left over from when those two fields were added.

### v1.13.0

- Added an in-app ADIF export (`Ctrl+O`): writes the active station profile's full log to your Downloads folder as a timestamped `.adi` file, without needing to quit and use the `--export-adif` CLI flag. Each run gets its own file, so repeated exports never overwrite an earlier one.

### v1.12.4

- The version/keybinding rows at the top of every screen are now yellow instead of dim gray, which was hard to read.

### v1.12.3

- Rebound Cabrillo export from `F11` to `Ctrl+X`: F11 is a near-universal fullscreen/maximize toggle in terminal emulators and window managers, so it never reached the app either, the same problem F10 had.

### v1.12.2

- The hotkey line at the top of every screen now wraps across two rows instead of one, which had grown too wide to fit most terminal widths after F11 (Cabrillo export) was added.

### v1.12.1

- Rebound Cabrillo export from `F10` to `F11`: GNOME Terminal and other GTK-based terminals reserve `F10` to toggle their own menu bar, so the keypress never reached the app there.

### v1.12.0

- Added a Cabrillo export (`F10`, whenever a contest is loaded on the Contest Entry field): writes a Cabrillo v3 submission for the active contest's QSOs to your Downloads folder, named `<CALLSIGN>_<CONTEST>.cbr`. New Station Setup (`F2`) fields — Cat-Operator, Cat-Assisted, Cat-Power, Address — feed the Cabrillo header; each falls back to a sane default (SINGLE-OP/NON-ASSISTED/LOW) if left blank.

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
