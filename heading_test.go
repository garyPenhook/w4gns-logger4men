package main

import (
	"math"
	"testing"
)

func TestGreatCircleBearingDistance(t *testing.T) {
	cases := []struct {
		name                 string
		lat1, lon1           float64
		lat2, lon2           float64
		wantBearing, wantDst float64
		distTol              float64
	}{
		{
			name: "zero distance",
			lat1: 40.0, lon1: -75.0,
			lat2: 40.0, lon2: -75.0,
			wantBearing: 0, wantDst: 0, distTol: 0,
		},
		{
			// 10 degrees due east along the equator: pure longitude delta.
			name: "due east along equator",
			lat1: 0, lon1: 0,
			lat2: 0, lon2: 10,
			wantBearing: 90, wantDst: earthRadiusKm * (10 * math.Pi / 180), distTol: 1,
		},
		{
			// 10 degrees due north along a meridian.
			name: "due north along meridian",
			lat1: 0, lon1: 0,
			lat2: 10, lon2: 0,
			wantBearing: 0, wantDst: earthRadiusKm * (10 * math.Pi / 180), distTol: 1,
		},
		{
			// Reverse of the above: due south.
			name: "due south along meridian",
			lat1: 10, lon1: 0,
			lat2: 0, lon2: 0,
			wantBearing: 180, wantDst: earthRadiusKm * (10 * math.Pi / 180), distTol: 1,
		},
		{
			// Antipodal points: half the great circle, exactly half the
			// Earth's circumference away.
			name: "antipode",
			lat1: 0, lon1: 0,
			lat2: 0, lon2: 180,
			wantBearing: 90, wantDst: math.Pi * earthRadiusKm, distTol: 1,
		},
		{
			// Southern/western hemisphere signs: Sydney to Cape Town, a
			// known real-world pair (negative lat and lon on both ends).
			name: "southern/western hemisphere signs (Sydney to Cape Town)",
			lat1: -33.8688, lon1: 151.2093,
			lat2: -33.9249, lon2: 18.4241,
			wantBearing: 218, wantDst: 11020, distTol: 50,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			bearing, dist := GreatCircleBearingDistance(c.lat1, c.lon1, c.lat2, c.lon2)
			if math.Abs(dist-c.wantDst) > c.distTol {
				t.Errorf("distance = %.2f km, want %.2f km (tol %.2f)", dist, c.wantDst, c.distTol)
			}
			if bearingDiff(bearing, c.wantBearing) > 2 {
				t.Errorf("bearing = %.2f deg, want %.2f deg", bearing, c.wantBearing)
			}
		})
	}
}

// bearingDiff returns the absolute angular difference between two bearings,
// accounting for the 0/360 wraparound.
func bearingDiff(a, b float64) float64 {
	d := math.Abs(a - b)
	if d > 180 {
		d = 360 - d
	}
	return d
}

func TestKmToMiles(t *testing.T) {
	got := KmToMiles(1.609344)
	if math.Abs(got-1) > 1e-9 {
		t.Errorf("KmToMiles(1.609344) = %v, want 1", got)
	}
	if KmToMiles(0) != 0 {
		t.Errorf("KmToMiles(0) = %v, want 0", KmToMiles(0))
	}
}
