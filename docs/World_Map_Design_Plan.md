# W4GNS World Map — companion application design

Status: design with first companion implementation, September 6, 2026.

## Implementation notes

Ctrl+L (or F10 when passed through by the terminal) opens the embedded browser map through `internal/mapserver`. The existing
`internal/spot`, `internal/geo`, and `internal/mapfeed` foundation supplies normalized
reports before terminal filtering/deduplication. The logger remains the sole cluster
connection owner. Country reference positions, home-grid location, band/age/search
controls, follow/independent filtering, station inspection, selected/all/off paths,
zoom/pan/reset/fullscreen, and a paged report list are implemented.

Implementation choices relative to the proposal below:

- Both geography and dynamic layers use one canvas, ensuring identical transforms;
  the HTML report list provides keyboard station selection.
- Every SSE connection begins with a replacement snapshot; subsequent packets are
  deltas. Filter changes and retention gaps force another replacement snapshot.
  This avoids a separate snapshot/live handoff and does not require replaying a
  disconnected client's old event queue.
- Retention is bounded to 20,000 reports; streaming expires records after one hour.
  Marker aggregation counts distinct spotters, while the report list keeps raw
  reports rather than suppressing identical relays. All-path rendering is capped
  at 500 distinct DX/band/spotter paths.
- View preferences use browser local storage and therefore follow the current
  local origin; an assigned port change after restart can reset them.
- Manual location overrides, Pacific centering, home-to-DX drawn lines, day/night
  overlays, logger entry handoff, and a standalone executable remain extensions.
  Home-to-DX distance/bearing is already shown as information in station details.

Validation includes Go tests/vet, targeted server/feed race checks, and Chromium
fixture checks for selection, distinct spotters, filtering, search, safe text,
refresh, zoom/reset, and narrow layout. The opt-in fixture can be launched with
`W4GNS_MAP_BROWSER=1 go test ./internal/mapserver -run TestBrowserFixture -v`;
it prints a one-use local URL and runs for three minutes without touching a
database or establishing a cluster login. Live cluster acceptance and native
Windows/macOS browser launching require checks on those environments. The load
targets below remain targets rather than measured performance guarantees.

## Purpose and recommended direction

Add a companion graphical window displaying live CW cluster activity on a flat world map. Keep the terminal logger as the operating interface; the map can sit on another monitor and update continuously.

Recommended first release: a locally served browser application, opened from the logger, with its own window/tab and embedded map assets. The logger owns the existing cluster connection and supplies live data. This provides a secondary application experience without initially introducing a second cluster login or a desktop GUI runtime. A separate `w4gns-map` executable with standalone reception is a later phase if operating without the logger is desired.

This is an observed cluster-activity map. A report indicates that a spotter reported a DX station; it does not prove that the operator can hear or work that station. Country reference coordinates also do not establish the actual radio path. Avoid presenting activity density as signal strength or a propagation prediction.

## Existing code and integration implications

| Existing component | Reuse or required adjustment |
| --- | --- |
| `cmd/w4gns-logger/cluster.go` | Existing K3LR connection, reconnection support, spot parser, and text sanitation. Spots contain spotter, frequency string, DX call, comment, and local UTC receipt time. |
| `main.go`: `clusterLineMsg`, `addClusterSpot` | Publish parsed spots before display filtering and duplicate suppression. The terminal currently keeps only 100 spots and suppresses the same DX call/band for three minutes; that loses distinct spotter reports needed for paths. |
| `cluster_filters.go`, `bandplan.go` | Reuse CW classification and DX/DE filters. Separate baseline CW eligibility from optional display filters so map controls can narrow or broaden the retained CW feed. |
| `dxcc.go`, `data/cty.dat` | Resolve both endpoint callsigns to country/continent and approximate reference coordinates. Audit alias coordinate overrides: the current code describes ignoring these, so do not claim station-level precision. |
| `grid.go`, `station.go` | Home marker from the active station grid. Update when station profile changes. |
| `heading.go` | Reuse great-circle distance and initial bearing calculations. |
| `solar.go` | Optional later display of the existing solar indices with their age. |

Most existing functionality is in `package main` and cannot simply be imported by another executable. Extract reusable domain code gradually, with existing tests preserved, when implementing the shared packages below.

## Window layout and behavior

```text
+--------------------------------------------------------------------------------+
| W4GNS World Map    Cluster: connected    Last spot: 8s ago    18:42 UTC           |
| Bands: All / 160 ... 6m    Age: 15m    Filters: Follow logger    Search: [      ] |
+-----------------------------------------------------------+--------------------+
|                                                           | Selected station   |
|             FLAT WORLD MAP                                | Call / country     |
|                                                           | Band / frequency   |
|    Home star       Colored DX markers                     | Latest report age  |
|                    Spotter-to-DX paths on selection       | Location source    |
|                                                           | Spotter / comment  |
|    Zoom +/-   Reset view   Paths: selected/all/off         | Bearing / distance |
+-----------------------------------------------------------+--------------------+
| Recent reports: UTC | DX call | frequency | spotter | location quality          |
| Band legend    Visible: 124    Unlocated: 7    Approximate locations: 98         |
+--------------------------------------------------------------------------------+
```

- Dark ocean, muted land, readable borders and restrained latitude/longitude grid. Band colors remain consistent in markers, list, and legend; labels and shape also convey meaning for users who cannot distinguish colors.
- Use an equirectangular projection for a rectangular whole-world view. Default longitude center is Greenwich; add a Pacific-centered option. Explain that this projection distorts area and that straight screen lines are not radio paths.
- Start at the whole-world extent. Support zoom, drag, reset, and fullscreen. Keep a usable report list and details panel on smaller windows.
- Group markers by DX callsign and band, retaining all contributing reports. Size indicates distinct reporting stations, not signal strength. Fade by report age; avoid continuous flashing.
- Default to the last 15 minutes; offer 5, 15, 30, and 60 minutes. Use local receipt time consistently because the current parser does not preserve a reliable source timestamp.
- Click a marker or report row to select the station, highlight its reporting stations and paths, and show frequency, comment, first/latest seen, report count, and endpoint location sources. Overlapping country-reference markers open a selectable group; never randomly offset coordinates and imply precision.
- Draw sampled great-circle paths between spotter and DX, splitting at the map seam. Show selected paths by default to avoid clutter. Approximate endpoints produce dashed paths. If either endpoint cannot be located, retain the report without drawing that path.
- A separate optional home-to-DX line displays bearing/distance and is labeled "From your station — not a reception report." No home grid means no home marker or home bearing.
- Follow logger filters by default. An explicit Independent mode uses the same filter rules against the retained CW feed, with map-specific band/age controls. Display which mode is active; map adjustments do not silently change F4.
- Initial version supports inspection and copying calls/frequency. Later "Use in logger" must send an explicit command through the logger event loop and protect an entry already in progress; it must never log a QSO automatically.

## Location resolution and honesty

Resolve DX and spotter independently and attach provenance to each result:

1. An explicitly configured callsign/grid override, with source and optional expiry.
2. A valid locator explicitly attributed to that endpoint by a supported spot format. Do not treat an arbitrary grid-like comment token as the DX or spotter location.
3. Bundled callsign-prefix country reference coordinates, labeled "Approximate: country/prefix reference."
4. Unknown: keep in the report list and unlocated count, with no geographic marker.

For the MVP, implement manual overrides and country references; format-specific grid extraction can follow once fixtures establish endpoint attribution. Maidenhead positions are cell centers, not exact station coordinates. Preserve locator precision. Use explicit missing-coordinate state rather than treating every numeric zero as absent; the current DXCC representation uses `(0,0)` as unavailable and needs careful conversion at the boundary.

Portable and mobile calls may not match a home location. Keep the reported callsign, apply the existing resolver's documented behavior, and label unresolved/approximate results. Do not bulk-query QRZ or infer current positions from old QSOs in the initial release. Any later lookup cache needs source age, permitted usage, rate limits, and clear handling of portable operation.

## Data flow and ownership

```text
Existing cluster connection
          |
    Parse and validate
          |
          +--> Existing logger filters + terminal dedup --> terminal panels
          |
          +--> Shared CW eligibility --> bounded map report store
                                            |
                                   resolve both locations
                                            |
                                  snapshot + live events
                                            |
                                   browser map and list
```

The map branch must receive distinct spotter reports before terminal deduplication. Normalize frequency to integer Hz with explicit input units while retaining the displayed original value. Reject malformed/out-of-band records before map storage. Mode remains "likely CW" under the logger's existing heuristic, not independently verified CW.

Suggested internal report contract: schema version, session ID, increasing event ID, received-at UTC, DX call, spotter call, frequency Hz, band, comment, and two optional location objects. Each location carries latitude, longitude, source, precision category, optional locator, and resolution timestamp. Keep reported source time nullable until parsed reliably.

Retain up to 60 minutes and 20,000 reports in memory, whichever limit is reached first; tune from a captured high-volume fixture. Show when a cap truncates the selected time range. Expire records even while the cluster is disconnected. No spot-history database is required for the first release.

Collapse identical relayed reports using normalized DX, spotter, frequency, and comment within a short proposed 30-second window; maintain last-seen and duplicate count. Preserve different spotters and frequency changes. Marker aggregation is separate from report deduplication.

Keep mutations in one map-store owner or behind a narrow synchronization boundary. Publishing must be bounded and nonblocking for the terminal event loop. If a consumer falls behind, signal a resnapshot rather than silently lose reports. Browser clients get immutable data and never access the logger database or credentials.

## Local serving and packaging

- Embed HTML, CSS, JavaScript, and simplified world geometry in the Go build. Use SVG for base geography and a canvas overlay for dynamic markers/paths, with synchronized transforms and an accessible HTML list. No frontend framework is needed for the first version.
- Bundle low-resolution Natural Earth geometry with a recorded source version and build/conversion procedure. Its map data is public domain; retain an attribution/source note for traceability. [Natural Earth terms](https://www.naturalearthdata.com/about/terms-of-use/).
- Start an optional Go HTTP server bound to `127.0.0.1` on an OS-assigned port when the user invokes a new, conflict-checked "Open World Map" action. Open the URL using platform launch helpers; show the URL if automatic opening fails. The map assets work without internet; new spots require the cluster connection.
- Serve an initial snapshot and Server-Sent Events (SSE) for report updates, status, active station, and filter changes. Browser `EventSource` supports this server-to-browser stream. [MDN EventSource](https://developer.mozilla.org/en-US/docs/Web/API/EventSource).
- Make snapshot/live handoff atomic using a snapshot cursor and bounded event replay. On reconnect, replay after the last event ID; if unavailable or the server session changed, send reset and replace the snapshot. Include heartbeat and clear stale/disconnected status.
- Restrict Host and Origin to the local application, avoid permissive CORS, and use a per-run authenticated session established through a one-use launch token. Never include cluster credentials or external-service keys in snapshots. Render all cluster strings as text and use a restrictive content-security policy.
- Closing the browser leaves logging running. Quitting the logger stops the local server and the page visibly becomes disconnected. Reopening the map reuses the running server and restores view preferences. Persist only harmless display preferences in browser storage in the MVP.
- Browser refresh, a slow client, or a map error must not interrupt logging or cluster reception. Support multiple tabs through one feed owner and bounded subscriptions.

Suggested package boundaries, to finalize during implementation:

```text
internal/spot/          normalized reports, CW eligibility, filters
internal/geo/           DXCC resolution, grids, bearings, location provenance
internal/mapfeed/       retention, aggregation, snapshots and replay
internal/mapserver/     local HTTP lifecycle and authentication
internal/mapserver/web/ embedded browser UI and world geometry
cmd/w4gns-logger/       terminal UI, cluster owner, map launch integration
cmd/w4gns-map/          future standalone host; not needed for MVP
```

Do not extract station credentials/storage into shared web-facing types. If standalone operation is added, reuse shared packages and define explicit attached versus standalone modes; never silently establish another cluster session while attached to the logger.

## Delivery sequence and completion gates

| Phase | Deliverable | Completion gate |
| --- | --- | --- |
| 1. Shared data foundation | Report types, location provenance, reusable geo/filter boundaries, feed tap before terminal dedup | Existing logger checks pass; two spotters reporting one DX survive in map data; terminal behavior remains covered. |
| 2. Useful companion map | Local launch, embedded whole-world map, CW spots, country fallback, home marker, age/band filters, details/list | A parsed fixture and a live feed place reports correctly; approximate/unknown locations are explicit; logger remains responsive. |
| 3. Reliable activity paths | Selected spotter-to-DX paths, seam handling, aggregation, SSE replay/reset, bounded retention | Reconnect has no unexplained gaps/duplicates; disconnected spots age out; load and seam fixtures pass. |
| 4. Operating polish | Independent filters, grid overrides, persisted view, accessibility, platform launch checks | Filter behavior is predictable, overlapping spots are selectable, and launch/close behavior works on supported platforms. |
| Later | Day/night terminator, gray-line overlay, solar indices, optional logger handoff, standalone executable, explicit history/replay | Scope separately; prediction models and RF-strength heatmaps are outside the initial design. |

MVP release requires phases 1–3 plus basic keyboard access, usable contrast, and error states. Phase 4 enhances the operating workflow; do not defer security or lifecycle correctness to it.

## Verification and acceptance

- Unit and integration fixtures: longitude sign conversion, portable/unresolved calls, absent coordinates, locator centers, identical reports versus distinct spotters, integer frequency conversion, time expiry, filter parity, session resets, and snapshot/live races.
- Map fixtures: known locations in both hemispheres, stations adjacent to the date line, near-polar paths, coincident endpoints, and multiple calls at the same country reference point. No path spans incorrectly across the screen seam.
- Browser checks: select from map/list, zoom/pan alignment, follow/independent filters, missing home grid, empty feed, lost cluster connection, closed logger, reconnect, and untrusted comments rendered harmlessly as text.
- Proposed load target: 20,000 retained reports, 100 incoming reports/second in bursts, and two browser clients. Aim for ordinary updates visible within one second and no perceptible delay in QSO entry on the target machine. Measure before claiming this target is achieved.
- Regression checks during implementation: `go test ./...`, `go vet ./...`, and relevant race checks; build supported release targets and manually exercise browser launching where those platforms are available.

The initial implementation should make one workflow work well: open the companion map, see recent CW activity worldwide, select a DX station, and understand who reported it, when, and how accurately either endpoint is located.
