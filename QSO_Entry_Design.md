# QSO Entry Design

## Purpose

Keep routine CW logging fast in a small terminal while making optional QSO and contest data available before a QSO is saved.

## Pages and navigation

| Key | Page | Purpose |
| --- | --- | --- |
| `F1` | QSO Entry | Fast, required QSO fields and prior-contact view. |
| `F6` | QSO Details | Optional station and operating details. |
| `F7` | Events & Contests | Select a configured event or contest, then enter its exchange. |

`Esc` returns to QSO Entry. `Tab`, `Shift+Tab`, and `Enter` move among fields on the secondary pages. Saving remains an F1-only action: `Enter` on the final Mode field writes the QSO and begins a blank next QSO.

## F1 — QSO Entry

Field order is Call, RST Sent, RST Received, Band, Frequency, Mode. Band is a closed selector; `Left`/`Right` or `Up`/`Down` chooses one of the supported CW bands and supplies a valid default frequency. Frequency is entered in MHz beside Band and is validated against conservative international band edges. Operators remain responsible for their national licence, regional allocation, and band-plan limits. This page displays UTC and local time, QSO timer state, 15-minute call-and-band dupe warning, and all prior contacts for the entered callsign. The active QSO never appears in the prior-contact list until saved.

## F6 — QSO Details

Optional fields are name, QTH, grid square, state/province, POTA reference, and notes. Frequency is an entry-row field. These map to the corresponding ADIF-shaped QSO columns: `FREQ`, `NAME`, `QTH`, `GRIDSQUARE`, `STATE`, `SIG`/`SIG_INFO`, and `COMMENT`. A POTA reference is populated only when a matching callsign appears in the previous 15 minutes in the local DX-cluster spot history or POTA active-spot feed. The network lookup has a bounded timeout and never blocks logging.

## F7 — Events & Contests

The Events & Contests page is backed by embedded JSON configuration files in `events/`; it is designed to scale to the hundreds of on-air activities without a code change for each one. An event definition supplies its stable ADIF `CONTEST_ID`, organizer, schedule, allowed bands, exchange prompts, serial requirement, dupe scope, rules URL, score-submission URL, and one or more UTC sessions. The operator selects both the event and its session; this produces a session-specific contest ID such as `CWT-1900` or `CW-OPEN-2`. The initial catalog includes all four weekly CWops Test (CWT) sessions and the three CW Open sessions. Contest Entry stores `CONTEST_ID`, `STX`, `STX_STRING`, `SRX`, and `SRX_STRING`.

The Tennessee QSO Party (TNQP) definition is configured for an out-of-state operator: the sent exchange is the operator's state, province, or DX prefix, while the received exchange supports type-ahead against all 95 official Tennessee county abbreviations. When the received-exchange field is focused, type a county name or code, use `Up`/`Down` to select a match, then press `Enter` to insert its four-letter abbreviation.

## Data and reset behavior

All three pages edit one in-memory QSO. Data is written atomically only when F1’s final Mode field is submitted. After a successful save, every QSO, details, and contest field is cleared so details cannot carry accidentally into the next contact. A failed validation or database write retains all entered data and reports the error.
