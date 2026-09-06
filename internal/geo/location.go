// Package geo holds location-provenance types shared by the logger and its
// companion map: a resolved coordinate is only as trustworthy as the method
// used to find it, so that method travels with the coordinate rather than
// being implied by context.
package geo

import "time"

// Source identifies how a Location's coordinates were determined.
type Source int

const (
	// SourceUnknown means no coordinates could be attached to the endpoint.
	SourceUnknown Source = iota
	// SourceOverride is an explicitly configured callsign/grid override.
	SourceOverride
	// SourceLocator is a Maidenhead locator explicitly attributed to the
	// endpoint by a supported spot format.
	SourceLocator
	// SourceCountryReference is the bundled callsign-prefix country/DXCC
	// entity reference coordinate — an approximation, not a station location.
	SourceCountryReference
	// SourceQRZProfile is the station's QRZ.com XML profile coordinate — a
	// self-reported address, more precise than a country reference but not
	// an operator-confirmed override or a spot-attributed locator.
	SourceQRZProfile
)

// Precision categorizes how tightly a Location bounds the true station
// position, independent of Source (an override could be exact or a grid
// square; a country reference is always approximate).
type Precision int

const (
	// PrecisionUnknown applies when there is no Location at all.
	PrecisionUnknown Precision = iota
	// PrecisionExact is a specific station coordinate (e.g. an override).
	PrecisionExact
	// PrecisionLocatorCell is the center of a Maidenhead locator cell.
	PrecisionLocatorCell
	// PrecisionCountryReference is a country/prefix reference point, far
	// coarser than the operator's actual location.
	PrecisionCountryReference
	// PrecisionQRZProfile is a QRZ.com profile's self-reported address
	// coordinate — station/address-level, but not verified.
	PrecisionQRZProfile
)

// Location is a resolved endpoint coordinate with its provenance attached.
// Latitude and Longitude use the standard signed convention (north/east
// positive). Locator is set only when Source is SourceLocator or the
// coordinate happens to derive from one; it is empty otherwise.
type Location struct {
	Country    string
	Latitude   float64
	Longitude  float64
	Source     Source
	Precision  Precision
	Locator    string
	ResolvedAt time.Time
}
