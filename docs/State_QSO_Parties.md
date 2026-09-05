# Shared state QSO party support

Updated September 5, 2026. State parties share the entry interface and a
configurable CW rules engine, but exchanges are not their only difference.
Points, entrant-side eligibility, multiplier lists and scope, county-line
credit, duplicate rules, bands, operating periods, bonuses, and submission
requirements also differ. Sponsor rules remain authoritative.

Related documentation: [operator guide](../README.md),
[entry behavior](QSO_Entry_Design.md), [implementation tracker](ROADMAP.md),
and [release notes](../CHANGELOG.md).

## Implemented behavior

The event's `county_options` supplies validated official codes and county-name
suggestions in both sent and received exchange fields. Type a name or code,
select with Up/Down, and press Enter. For county-line contacts, join codes with
`/`; autocomplete preserves previously entered counties. Enter California's
serial in the separate serial field. RST is omitted only where configured.

Sent and received exchanges are persisted with each QSO. Enter your operated
county when inside the party's state, or your state/province/DX location when
outside. Location is never inferred from your callsign. County changes therefore
survive edits, imports, score rebuilds, and profile changes. DXCC multipliers use
the bundled prefix table; use `DX:LA` to distinguish Norway from Louisiana where
DX entity exchanges are accepted. Sponsor aliases such as DC/MD are configured
per event.

The shared `qso_party` configuration drives parsing, eligible CW bands and
verified operating periods, both entrant sides, scoring, live new-multiplier
indication, location-aware duplicate checks, and Cabrillo validation. Duplicates
include call, band, operated location, and worked location, independently of
serials or names. County-line contacts expand into independently credited county
pairs: a previously worked county does not discard a new county in the same
contact. Duplicate checks respect station profile, contest log, edit exclusion,
and the existing duplicate-baseline setting.

Scoring excludes duplicates, contacts marked unscored, malformed exchanges,
outside/outside contacts, and contacts outside configured periods/bands. Bonuses,
multiplier caps, and power factors are applied by the same engine used for the
exported claimed score. Station Setup now persists `Cat-Station`; choose MOBILE
or ROVER for applicable Tennessee county activation bonuses. Florida uses the
declared power category (blank defaults to HIGH, factor one).

Cabrillo export validates exchanges and event periods, derives LOCATION from
saved sent exchanges, emits CATEGORY-STATION, and expands county-line contacts
into separate QSO records. Ineligible outside/outside contacts are X-QSO records.
Mixed in-state/out-of-state logs and changing outside locations require separate
submissions. Exported contact count refers to saved contacts, which can produce
more QSO lines after county expansion.

The optional County field on QSO Details does not replace the received contest
exchange. The sent exchange carries forward after saving, so a mobile operator
must update it when changing counties. Contest occurrence IDs keep each edition
or recurring session separate; only the sponsor token goes in Cabrillo's
CONTEST header. Ordinary party submissions use CW mode; Michigan and Ohio use
the configured MIXED category header even though all logged contacts are CW.

## Configured parties and sources

These eight definitions have county tables, scoring, and checked CW submission
paths. Periods are explicit; this does not automatically certify future years.
All dates below describe the configured rules, not a claim of sponsor approval.

| Party / catalog ID | Counties | CW differences implemented | Verified editions / sponsor sources |
| --- | ---: | --- | --- |
| Tennessee / TNQP | 95 | 3 points; counties, states/provinces and DXCC for home entrants; multipliers per band; two-county lines; K4TCG bonus and mobile/rover activation bonuses | [2026 rules](https://tnqp.org/wp-content/uploads/2026/08/Tennessee-QSO-Party-2026-Rules.pdf) |
| California / CA-QSO-PARTY | 58 | Serial/location, no RST; 3 points; home state/province multipliers and outside county multipliers; 58 cap; two-county lines | [2026 rules](https://www.cqp.org/Rules.html), [multipliers](https://www.cqp.org/cqp_multipliers.html), [county-line logging](https://www.cqp.org/files/cqp_how_to_log_a_county_line_qso.pdf) |
| Michigan / MI-QSO-PARTY | 83 | RST/location; 2 points; home counties, states/provinces and one DX multiplier; DC separate; one county per exchange | [2026–2027 rules](https://miqp.org/index.php/rules/), [multipliers](https://miqp.org/index.php/official-list-of-mults/), [Cabrillo](https://miqp.org/index.php/cabrillo-information/) |
| Ohio / OH-QSO-PARTY | 88 | RST/location; 2 points; home counties, states/provinces and one DX multiplier; NT/YT/NU grouped; one county per exchange | [2026–2027 rules](https://www.ohqp.org/index.php/rules/), [multipliers](https://www.ohqp.org/index.php/official-list-of-mults-for-ohio-stations/), [Cabrillo](https://www.ohqp.org/index.php/cabrillo-information/) |
| Georgia / GA-QSO-PARTY | 159 | 2 points; home state/province multipliers, including GA; no DX multiplier; two operating sessions and two-county lines | [2026 rules](https://gaqsoparty.com/georgia-qso-party-rules/), [counties](https://gaqsoparty.com/county-list/) |
| Florida / FCG-FQP | 67 | 2 points; home states/provinces, DXCC and maritime regions; HIGH/LOW/QRP factors 1/2/3; special stations score QSO count; two sessions and two-county lines | [2026 rules](https://floridaqsoparty.org/rules/), [counties](https://floridaqsoparty.org/counties/counties-list/), [special calls](https://floridaqsoparty.org/fqp-2026-special-calls/), [Cabrillo](https://floridaqsoparty.org/wp-content/uploads/Cabrillo-Specification-V3-FQP-1.pdf) |
| Alabama / AL-QSO-PARTY | 67 | 2 points; home counties, states/provinces and DXCC; DC/MDC normalize to MD; one county per exchange | [2026 rules](https://alabamacontestgroup.org/aqp/rules/), [counties](https://alabamacontestgroup.org/aqp/counties/) |
| Iowa / IAQP | 99 | 2 points; home counties plus states/provinces; DC/MD grouped; up to four simultaneous counties; corrected imported 60M-only band list | [2026 rules](https://www.w0yl.com/IAQP), [counties](https://w0yl.com/sites/default/files/IAQP-County-List-2018-06-08.pdf), [Cabrillo example](https://w0yl.com/sites/default/files/iaqp-cab-tpl-2021-05-19.txt) |

Outside entrants earn worked county multipliers. Unless the table says per band,
configured multipliers are contest-wide for this CW-only logger. Michigan and
Ohio require MIXED in the submission category even for a CW-only log.

## Limits and remaining catalog work

The remaining state/province/regional catalog entries retain their existing
capabilities; they have **not** all been promoted to verified scoring and export.
Each needs a sponsor-source audit and submission fixture. Multi-state regions
such as New England and 7QP need explicit regional county/state modeling.
Minnesota's published [2027 rules](https://www.w0aa.org/wp-content/docs/mnqp/MNQP_2027_Contest_Rules_A.pdf)
change CW points and multiplier treatment; those rules must not be applied to
2026 logs. Minnesota is not promoted by this change.

Scores remain estimates. This implementation does not adjudicate maximum
operator on-time, mandatory off-time, physical county-line eligibility, travel,
power measurements, category qualification, or all sponsor-specific DXCC
exceptions. Supported band coverage stops at the logger's available bands.
Special category declarations and sponsor-specific submission metadata may need
completion on the sponsor's upload form or in the exported file. A checked QSO
layout is not certification of every competitive category or submission field.

## Maintenance and validation

`qso_party_rules.go` owns the shared rules; event JSON owns verified differences.
The original generic county multiplier and legacy Tennessee multiplier remain
compatible. Invalid county tables, rule combinations, or overlapping periods
fail catalog loading. `tools/import_party_counties.py` converts saved sponsor
HTML or extracted PDF text into county JSON with duplicate/count checks; it does
not fetch sources or silently replace event definitions.

Regression tests cover entrant sides, partial county-line duplicates, serial
independence, aliases and DX prefixes, caps, bonuses, power factors, operating
period boundaries, invalid/unscored contacts, profile persistence, edit and log
isolation, actual entry/autocomplete, and Cabrillo expansion/score parity. Every
checked catalog definition must have a valid and invalid submission fixture.

Implementation validation passed on September 5, 2026: `go test ./...`,
`go vet ./...`, and `go test -race -short ./...`. Documentation updates also
check local links and `git diff --check`. These checks verify the implementation
and documentation; they do not establish sponsor acceptance of every category.
