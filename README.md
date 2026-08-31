# W4GNS Logger 4 Men

CW only logger, life is too short for QRM.

## Start

Run the application:

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
| `F7` | Contest Entry |
| `Tab` / `Shift+Tab` | Move between entry fields |
| `Enter` | Move to the next field; save a QSO from the final field |
| `Esc` | Quit from QSO Entry; cancel Station Setup; return to QSO Entry from DX Cluster |

## QSO entry

- CW QSOs are logged with callsign, band, mode, and sent/received RST.
- The initial entry sequence is Call, RST Sent, RST Received, Band, then Mode.
- Callsigns are normalized to uppercase.
- While a callsign is being entered, the **Stations Worked** area shows that station's earlier contacts from the log. The QSO being entered is excluded until it is saved.
- Duplicate callsign-and-band contacts are flagged before logging.
- The first `Tab` or `Enter` after entering a callsign starts the QSO timer.
- The final `Enter` saves UTC start and end times, then clears the form for the next QSO.
- UTC and local time are displayed in the logger header.

## QSO Details and Contest Entry

Press `F6` for optional QSO details: frequency, operator name, QTH, grid square, state or province, and notes. Press `F7` for contest fields: contest name, sent and received serial numbers, and sent and received exchanges. These pages retain their values until the QSO is logged from the main QSO Entry screen.

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
