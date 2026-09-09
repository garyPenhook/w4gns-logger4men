# Changelog

### v1.52.3

- Fix the stats panel's (`Ctrl+A`) DXCC/WAS/WAZ/VUCC/IOTA "needed" list running off the right edge of the screen: up to 20 entries (labels like "223 ENGLAND" can run long) were joined into a single comma-separated line with no regard for terminal width. The list now wraps across as many lines as needed to fit the actual terminal width.

### v1.52.2

- Add a startup backfill (`store.backfillQSOGeographyFromConfirmations`) that copies state/gridsquare/IOTA-reference from a QSO's own matched LoTW confirmation onto the local QSO row whenever that row never had it — e.g. logged without a QRZ lookup or ADIF import carrying that data. Unlike country/DXCC (backfillMissingDXCC), these fields can't be derived from a callsign, so a confirmed QSO could still be invisible to WAS/VUCC/IOTA "worked" counts even after LoTW proved it happened. Only fills a blank, never overwrites a locally-known value. Verified against a real log: WAS worked rows went from 21 to 674, bringing worked in line with the 50 confirmed states from v1.52.1's fix.

### v1.52.1

- Fix the WAS award stat counting foreign administrative subdivisions as US states: ADIF's STATE field is reused by many countries for their own "primary administrative subdivision" (Canadian provinces, Australian states, Russian oblasts, etc.), and some of those two-letter codes collide with a real US state's code (e.g. "AR" is both Arkansas and a European Russia oblast) — a real log showed 61 "confirmed" states, more than the 50 that exist. WAS worked/confirmed are now scoped to the DXCC entities WAS actually draws from (mainland United States, Alaska, Hawaii — the latter two are separate DXCC entities per ARRL's own DXCC FAQ but still count as 2 of the 50 states), restricted to the 50 real state codes, with DC folded into Maryland per ARRL's WAS rules.
- Fix WAZ counting any non-zero `cqz` value as a worked/confirmed zone, including out-of-range data (CQ's WAZ award covers exactly 40 zones); both sides are now bounded to 1-40.

### v1.52.0

- Add a one-time startup backfill (`store.backfillMissingDXCC`) that resolves country/CQ-zone/ITU-zone/DXCC-number for any QSO that never had them set — e.g. rows from a database carried over from before this app's DXCC resolution existed. Without it, the DXCC/WAS/WAZ award stats (`Ctrl+A`) undercounted "worked" for anyone whose log predates that feature, sometimes showing more confirmed than worked and needed-award lists missing most unconfirmed entities. Only ever fills a blank field; never overwrites a value already present, so it's safe on every startup.

### v1.51.3

- Fix a first LoTW confirmation sync silently returning only the last few confirmations instead of the operator's whole history: this app omitted `qso_qslsince` when no prior sync bookmark existed, assuming ARRL would then return everything, but live testing showed ARRL substitutes its own recent "system supplied default" instead — hiding years of older confirmations (e.g. DXCC entities confirmed long before switching to this app) with no error. A first sync now explicitly sends a pre-2003 `qso_qslsince` to force the real full history.

### v1.51.2

- Fix LoTW confirmation sync always failing with "response ended without the documented end-of-file marker," even on a complete, successful response: `parseLoTWReportRecords` only recognized `APP_LoTW_EOF` inside its `NAME:LENGTH>value` field-parsing branch, but ARRL's live `lotwreport.adi` sends it as a bare tag with no length prefix (confirmed against the real endpoint), so the check was unreachable and every sync fell through to genuine end-of-stream first.

### v1.51.1

- Surface a specific, actionable message when the configured LoTW Station doesn't match any TQSL Station Location, instead of `tqsl`'s generic "Command Syntax Error": `tqsl` uses the same exit code (10) for a genuinely malformed command line and for an unrecognized `-l` value, but its stdout names the real cause, which is now detected and reported (e.g. after entering a callsign instead of a Station Location name into Station Setup's LoTW Station field).

### v1.51.0

- Add LoTW Station, LoTW Cert Pass, LoTW Login, and LoTW Web Pass fields to Station Setup (`F2`), so the TQSL station-location name, certificate passphrase, and LoTW website login/password used for upload and confirmation sync can be entered in-app instead of only via `lotw.station`/`lotw.pass`/`lotw.login`/`lotw.webpass` files or `CWLOGGER_LOTW_*` environment variables. All three configuration methods remain interchangeable and env vars still take precedence.

### v1.50.1

- Harden database-path resolution against silently starting a second, empty database when the operator's real one can't be found in any of the usual locations (working directory, XDG data dir, pre-rename legacy dir): the path last successfully opened is now remembered and reused if it's still there, or reported as a clear error if it's gone, instead of quietly prompting for a callsign and creating a new log next to the real one.

### v1.50.0

- Fix the QSO Entry screen losing its Call/RST Sent/RST Rcvd field row off the top of a short terminal window: the Recent QSOs table (and the DX Spots panel beside it) now shrinks to fit the actual terminal height instead of staying pinned at a fixed 10 rows, so the hotkey bar, header, and field-entry grid above it always fit on screen.

### v1.49.0

- Add ARRL Logbook of the World (LoTW) integration: automatic per-QSO upload signed by your locally installed `tqsl` (alongside the existing QRZ/WRL delivery), a manual/CLI full-log backfill (`Ctrl+Y` / `--upload-lotw`), incremental confirmation (QSL) sync, and a `Ctrl+A` award/analytics stats panel (DXCC/WAS/WAZ/VUCC/IOTA worked vs. confirmed). See the "ARRL LoTW upload" and "LoTW confirmation sync & award stats" sections of the README.
- Enforce a one-hour cooldown between full-log LoTW backfills (`Ctrl+Y` / `--upload-lotw`), per ARRL's guidance that resubmitting a whole log should not be routine; `--upload-lotw --force` overrides it for a deliberate recovery re-run.
- Fix several LoTW confirmation-sync correctness gaps found in an external review and verified against ARRL's own documentation: a failed query (bad login/password) no longer reads as "zero new confirmations"; a response truncated in transit is now detected instead of silently advancing the sync bookmark; matching a confirmation to a logged QSO no longer requires an exact mode match (TQSL may remap the uploaded mode); and confirmed-award geography (DXCC/state/CQ zone/grid/IOTA) is now read from LoTW's own confirmed data rather than this app's local guess.
- Stop recording a LoTW upload batch as cleanly "sent" when `tqsl` reports it as all-duplicate or out-of-certificate-date-range; those are now logged as a distinct "suppressed" outcome, visible in the upload log and status line.
- Redact the LoTW password out of any network-error text before it can reach the screen or logs.
- Rename internal env vars, config/data directory names, and the ADIF/User-Agent product identifier so a downloaded copy of the app no longer carries the original author's callsign into another operator's config, uploads, or exports; existing installs keep working via an automatic fallback to the old paths. A first run with no existing database now prompts for the operator's own callsign and names the database file after it, instead of a fixed filename.
- Add a periodic (daily) check of TQSL Callsign Certificate status and pending certificate requests, per ARRL's guidance for programs that automate `tqsl`; findings surface as a `TQSL: ...` line in the upload-status panel.

### v1.48.0

- Reword the contest Cabrillo exchange-validation errors so they read correctly where they surface: each is wrapped as `<call> <time>: sent/received exchange: <message>`, so the former proper-noun-leading, self-repeating text (e.g. `Kentucky QSO Party DX exchange must be DX`) now reads `DX exchange must be DX` / `must be a French department code`. No validation behavior changes.
- Run `staticcheck` in CI (built with the job's Go toolchain so it always matches the go.mod version), and clear its findings.
- Pin every GitHub Actions dependency to a full commit SHA across all workflows.
- Sign each release's `SHA256SUMS` manifest with Sigstore keyless (cosign) signing and publish the signature and certificate alongside it; README documents how to verify a download.

### v1.47.0

- Add Cabrillo exchange validators for 78 more contests (plain-serial, CQ-zone/grid, country-conditional, county-or-area, and QRP/sprint location-based shapes), moving them from `entry-aware` to a tested `cabrillo-ready`/`scoring-ready` capability with fixtures in `TestCheckedCatalogSubmissionExchanges`.
- Correct 8 stale/incomplete exchange hints in the contest catalog (`HI-QSO-PARTY`, `ID-QSO-PARTY`, `ISLAND-QSO-PARTY`, `TR-HF`, `WALK-FOR-THE-BACON-QRP-CONTEST`, `QCX-CHALLENGE`, `QRP-ARCI-SPRING-QSO-PARTY`, `NTC-QSO-PARTY`).
- Allow a blank sent/received exchange for the small set of contests (ARRL 160M, DIG QSO Party, German Telegraphy Contest) whose sponsor rules specify an RST-only exchange for some stations, instead of always requiring one.

### v1.46.0

- Add regression coverage for an out-of-state operator's sent exchange in every US state QSO party (TN, OH, AL, CA, FL, GA, IA, MI): the prior catalog test only exercised an in-state fixture for both sides, missing the exact shape of contact that produced a blank sent exchange in the field.

### v1.45.0

- Block logging a US state QSO party contact with no sent exchange set (Contest Entry's Exchange Sent field): a blank value used to save silently and only surface as a Cabrillo export failure, potentially after an entire session's worth of QSOs.

### v1.44.0

- Record every QRZ/WRL upload's terminal outcome (sent/failed, QRZ's LOGID, and any error) in a persistent upload log, and show the most recent delivery in the footer status line, so an operator can confirm after the fact whether a QSO actually reached a destination.
- Hide the POTA Ref/IOTA Ref fields from the QSO Entry form during a US state QSO party (e.g. Tennessee QSO Party): those contests exchange serial/county/state, never a park or island reference.

### v1.43.0

- Clear the previous station's POTA/IOTA references when replacing an unsaved callsign, while preserving references during an existing-QSO edit.
- Export formula-like CSV fields as spreadsheet text instead of executable formulas.
- Publish map spots with fallback locations when a park lookup cannot be admitted; bound the total pending queue and expire waits, with late park results enriching those fallbacks.
- Update existing DX and spotter locations when QRZ results arrive, sending browser deltas with stable report IDs and preserving more precise park/grid/override locations.
- Reject non-finite and out-of-range QRZ/POTA coordinates so malformed locations cannot break the map's JSON stream.

### v1.42.0

- Fix World Map POTA locations: a spot naming a POTA park reference not yet resolved is now held until the park lookup completes instead of being permanently stranded at the coarser DXCC country reference.
- Draw POTA-resolved World Map markers in red to distinguish a park's location from a station's own location.

### v1.41.0

- Add a spotter call-area filter (checkboxes 0-9) to the World Map toolbar, so spots reported by stations in a given US call area (e.g. area 6 covers California) can be excluded from view.

### v1.40.0

- Darken the World Map's US state outlines for better contrast against the land fill.

### v1.39.0

- Draw US state outlines on the World Map (bundled Natural Earth admin-1 boundaries, 50 states + DC), as cartographic context under the country borders/markers/paths.

### v1.38.0

- Resolve POTA activation locations on the World Map from POTA's public API (api.pota.app), taking priority over both the QRZ profile and DXCC country reference when a spot's comment names a park reference.

### v1.37.0

- Show which band is currently most active on the World Map, live off the current report set and labeled as subject to change.

### v1.36.0

- Use QRZ profile lat/lon for World Map DX/spotter locations when available, falling back to the DXCC country/prefix reference otherwise. Requires QRZ XML credentials; looks up each newly seen callsign once (cached, rate-limited).

### v1.35.0

- Add a "General logging" indicator/status line, shown on every screen, reporting the active contest by name or that no contest is selected.
- Add a 'c' key on the Events (F7) catalog screen to clear the active contest and return to general logging, persisted so a restart doesn't resurrect an ended contest.

### v1.34.0

- Add a Location filter (DX + USA / DX only / USA only) to the World Map toolbar, so USA-located DX activity can be shown alongside or separated from foreign DX.
- Fix portable-call DXCC resolution: a callsign like "F/VE3ABC" now resolves to the shorter, operating-location side (France) instead of letting the home call's longer table prefix (Canada) win.
- Fix Station Setup so an active `W4GNS_QRZ_XML_USER`/`PASS` environment override keeps applying to the running session after a save, instead of being silently overridden by the just-typed form values until the next restart.

### v1.33.0

- Add Ctrl+L as the primary World Map shortcut because some consoles intercept F10 for their menu; retain F10 as an alias.

- Add F10 World Map: a local browser companion sharing the logger's CW cluster feed, with bundled world geography, band/age/search controls, logger-filter following, selected spotter paths, home-grid bearings, and approximate location labels.
- Preserve distinct spotter reports before terminal deduplication in a bounded map feed. Stream snapshots and deltas with reconnect resets, expiry, and authenticated loopback-only access.
- Add map server, feed integration, and opt-in real-browser fixture tests. Existing terminal cluster filtering and duplicate suppression remain in place.

### v1.32.3

- Refuse to overwrite the live SQLite database or its `-wal`/`-shm` sidecars from every atomic exporter (ADIF, CSV, Cabrillo), checking SQLite's actual open filename; the CLI export guard now also resolves URI escaping and driver options instead of comparing the raw DSN.
- Sanitize external text (remote status/error messages, POTA park names, solar text, imported signal reports, Recent QSOs and call history) before applying terminal styles, so a smuggled ANSI/OSC control sequence can't reposition the cursor, spoof UI text, or trigger a clipboard write. Stored QSO values stay intact.
- Require a matching QSO deletion before removing its upload-queue rows, within the same transaction, so a wrong-profile delete can no longer strip outbox rows while leaving the contact; QSO and station-profile updates now report a missing/mismatched record instead of falsely succeeding.
- Resolve the SQLite filesystem path before precreating and permission-checking the database, and use SQLite's authoritative filename after opening, so a named in-memory database no longer creates stray files and URI `mode=ro`/`mode=rw` databases are respected.
- Coalesce optional reports/exchanges when reading Recent QSOs and call history, so legacy rows with NULL fields load instead of failing.
- Include park names in the ADIF-import byte estimate and clear completed batches, so a large park name can't bypass the batch limit and batch strings are released promptly.
- Reset derived station coordinates before resolving a new grid, so clearing the grid no longer retains the previous latitude/longitude.
- Reject extra operands, repeated actions, an empty export/import filename, and conflicting actions in CLI argument validation; require exactly one unambiguous action.
- Clamp the haversine intermediate to its mathematical range so near-antipodal coordinate pairs no longer produce NaN distances/bearings.
- Housekeeping: removed an unused DXCC field, replaced a deprecated lipgloss style-copy call, simplified a redundant prefix check, and corrected six analyzer-reported error strings. Added the new audit regression tests to the native smoke workflow. See `docs/Project_Audit.md`.

### v1.32.2

- Wrap the QSO Entry fields onto multiple rows sized to the terminal width instead of stretching every field (Call, RST, Band, Freq, POTA/IOTA Ref, plus contest and POST slots) onto one line that could reach ~246 columns and overflow the display.
- Preserve a manually typed, not-yet-logged Sent Serial override across an edit detour: cancelling or saving an edit no longer reformats the field from the running counter and silently drops the operator's typed serial.
- Include `iota_ref`/`my_iota_ref` in the contest QSO SELECT/scan so IOTA multipliers survive an export or multiplier-index rebuild instead of reading back blank.
- Map the unresolved-session occurrence form (`EVENTID@stamp`) to the catalog's ADIF contest ID on export, matching the bare and `EVENTID-session` forms.
- Enforce each event's catalog band list for every contest submission (not just QSO parties) so an imported contact on an excluded band is rejected before it is exported and scored.
- Resolve portable calls symmetrically: the operating location now wins for both `F/W4GNS` and `W4GNS/F`, instead of the suffix form falling back to the home-call prefix.
- Skip IOTA island-group references (e.g. `EU-005`) when auto-filling POTA references from DX-cluster comments, and find a genuine POTA reference later in a comment that also carries an IOTA token.
- Track the pending delete confirmation by QSO identity, so an async table refresh between the two `d` presses can no longer delete whichever row now occupies the cursor.
- Start background export/import/upload work synchronously and recover panics in it, so quitting in the gap before Bubble Tea dispatches a batched command can no longer leave shutdown's task drain blocked or crash the program.
- Anchor an edited contact's duplicate re-check on its own original time, so correcting an old QSO is no longer rejected as a duplicate of a same-call contact worked today.
- Reserve the analysis panel's minimum width up front so a wide DX Spots line can no longer starve it to nothing on an unchanged terminal size.

### v1.32.1

- Fix two tests (`TestSaveStationSetupRetriesClusterConnectionWhenCallsignAdded`, `TestSaveStationSetupRotatesClusterAndContestStateOnIdentityChange`) that called `saveStationSetup()` without isolating `XDG_CONFIG_HOME`, silently overwriting a developer's real `~/.config/w4gns-logger/qrz.comXMLlogin` with blank QRZ XML credentials on every `go test` run. No app runtime behavior changed.

### v1.32.0

- Add IOTA (Islands on the Air) tracking: an `iotaRef`/`islandName` pair on every QSO, ADIF `IOTA`/`APP_W4GNS_LOGGER_ISLAND_NAME` export and import, and DX-Cluster-comment auto-fill (no equivalent third-party spot API exists for IOTA, unlike POTA).
- Wire the RSGB IOTA Contest's real scoring rules: a new `iotaPointsRule` implements all five official point tiers (island/world station × same/other/no reference), and a new `iota` per-band multiplier kind counts distinct references worked. A new Station Setup "My IOTA Ref" field declares your own station an island station for the QSOs logged while it's set, snapshotted per QSO so a later profile edit never rescoring already-logged contacts.
- Move POTA Ref and the new IOTA Ref onto the main QSO Entry screen (after Frequency), both optional and clearing after each QSO like Call; Park Name and the new Island Name stay on the QSO Details screen as free-text labels for whichever reference was entered.
- Add a live "POTA SPOTTED" indicator to the QSO Entry analysis panel, sourced from the real POTA spot feed (api.pota.app) for whatever callsign is currently being typed — shown even if a reference was already entered by hand. The analysis panel (country/zone/bearing plus this indicator) now also renders with no contest active, since POTA hunting is typically not a contest QSO.

### v1.31.0

- Synchronize the README, entry design, state-party guide, and roadmap with the eight implemented parties, category defaults, county-line export, retained sent exchanges, and remaining audit work; correct obsolete Tennessee bonus/scope notes.
- Implement shared state QSO party parsing, county autocomplete, location-aware duplicates, county-line credit, entrant-side scoring, bonuses/power factors, station categories, and checked CW exports for TN, CA, MI, OH, GA, FL, AL, and IA. Document verified editions and remaining catalog limitations in `docs/State_QSO_Parties.md`.
- Complete R18 exchange validation across all checked Cabrillo catalog layouts; reject extra tokens, invalid locations/grids/power, and unknown IARU society codes while keeping incomplete contacts editable locally.
- Validate Helvetia canton/serial, RDXC oblast/serial, and WAG DOK/NM/serial exchanges independently for each station during Cabrillo export; accept valid regional exchanges without requiring an unrelated serial number.
- Corrected the September 5 review findings; see `docs/ROADMAP.md` section 0A for scope and remaining verification limits.
- Preserve long and untouched multiline QSO fields on edit, default received CW reports to 599, and correlate delayed QRZ/POTA enrichment with the original contact and credentials.
- Separate recurring contest occurrences, migrate recognized legacy IDs by QSO date, restore contest selection/exchange, and resume serials from stored contacts.
- Correct persisted grid/zone scoring and event-scoped duplicate scoring. Add structured Sweepstakes Cabrillo exchanges and reject missing, invalid, or oversized supported submission fields instead of clipping them.
- Preserve app metadata and extended grids through ADIF, handle TIME_OFF without a separate date, stream long free text, cancel imports promptly, report partial results, and refresh contest analysis after import.
- Use consistent read snapshots on a separate database connection for file-backed exports; reserve ADIF filenames without indefinite retries on filesystem errors.
- Retain paused/failed uploads, bind queued destinations to credentials/logbooks, show queue errors/counts, and add explicit Ctrl+U recovery. Automatic retries stop after 20 failures.
- Fall back from headless/failed Linux terminal launches, embed timezone data, and add native-platform smoke tests plus release vulnerability checks. Native Windows/macOS execution remains a CI gate, not locally verified.

### v1.30.0

- Wired the K1USN Slow Speed Test (SST)'s real scoring rules: a flat 1 point per QSO times a multiplier that's the sum of distinct US states, Canadian provinces, and worldwide DXCC countries worked (no DXCC credit for the USA/Canada themselves), counted once for the whole contest, sourced from the K1USN SST Rules. New `sst_area` multiplier kind reuses NAQP CW's existing state/province table and exchange parsing but drops its North-America-only restriction on the DXCC fallback, since SST's own DXCC multiplier is worldwide.

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
