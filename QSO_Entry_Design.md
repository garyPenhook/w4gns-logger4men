# QSO Entry Design

## Purpose

Keep routine CW logging fast in a small terminal while making optional QSO and contest data available before a QSO is saved.

## Pages and navigation

| Key | Page | Purpose |
| --- | --- | --- |
| `F1` | QSO Entry | Fast, required QSO fields and prior-contact view. |
| `F6` | QSO Details | Optional station and operating details. |
| `F7` | Contest Entry | Optional contest identifiers, serials, and exchanges. |

`Esc` returns to QSO Entry. `Tab`, `Shift+Tab`, and `Enter` move among fields on the secondary pages. Saving remains an F1-only action: `Enter` on the final Mode field writes the QSO and begins a blank next QSO.

## F1 — QSO Entry

Field order is Call, RST Sent, RST Received, Band, Mode. This page displays UTC and local time, QSO timer state, 15-minute call-and-band dupe warning, and all prior contacts for the entered callsign. The active QSO never appears in the prior-contact list until saved.

## F6 — QSO Details

Optional fields are frequency, name, QTH, grid square, state/province, POTA reference, and notes. These map to the corresponding ADIF-shaped QSO columns: `FREQ`, `NAME`, `QTH`, `GRIDSQUARE`, `STATE`, `SIG`/`SIG_INFO`, and `COMMENT`. A POTA reference is populated only when a matching callsign appears in the previous 15 minutes in the local DX-cluster spot history or POTA active-spot feed. The network lookup has a bounded timeout and never blocks logging.

## F7 — Contest Entry

Optional fields are contest name, sent serial, sent exchange, received serial, and received exchange. They map to `CONTEST_ID`, `STX`, `STX_STRING`, `SRX`, and `SRX_STRING`. Future contest definitions determine required fields, validation, dupe scope, multiplier handling, scoring, and Cabrillo export without changing the general QSO-entry page.

## Data and reset behavior

All three pages edit one in-memory QSO. Data is written atomically only when F1’s final Mode field is submitted. After a successful save, every QSO, details, and contest field is cleared so details cannot carry accidentally into the next contact. A failed validation or database write retains all entered data and reports the error.
