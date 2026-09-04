package main

import "math"

// earthRadiusKm is the mean Earth radius used for great-circle distance
// (WGS-84 mean radius, the same constant ham radio beam-heading tools use).
const earthRadiusKm = 6371.0

const kmPerMile = 1.609344

// GreatCircleBearingDistance returns the initial great-circle bearing (degrees
// true, 0-360) and distance (km) from (lat1, lon1) to (lat2, lon2), all in
// decimal degrees with the standard signed convention (north/east positive).
// When the two points coincide, bearing is 0 and distance is 0 — there is no
// well-defined direction to a point at zero distance.
func GreatCircleBearingDistance(lat1, lon1, lat2, lon2 float64) (bearingDeg, distanceKm float64) {
	phi1 := degToRad(lat1)
	phi2 := degToRad(lat2)
	deltaPhi := degToRad(lat2 - lat1)
	deltaLambda := degToRad(lon2 - lon1)

	a := math.Sin(deltaPhi/2)*math.Sin(deltaPhi/2) +
		math.Cos(phi1)*math.Cos(phi2)*math.Sin(deltaLambda/2)*math.Sin(deltaLambda/2)
	c := 2 * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))
	distanceKm = earthRadiusKm * c

	if distanceKm == 0 {
		return 0, 0
	}

	y := math.Sin(deltaLambda) * math.Cos(phi2)
	x := math.Cos(phi1)*math.Sin(phi2) - math.Sin(phi1)*math.Cos(phi2)*math.Cos(deltaLambda)
	theta := math.Atan2(y, x)
	bearingDeg = math.Mod(radToDeg(theta)+360, 360)

	return bearingDeg, distanceKm
}

// KmToMiles converts a great-circle distance from kilometers to statute
// miles, for stations preferring imperial units (SD's DISTUNit).
func KmToMiles(km float64) float64 {
	return km / kmPerMile
}

func degToRad(deg float64) float64 { return deg * math.Pi / 180 }
func radToDeg(rad float64) float64 { return rad * 180 / math.Pi }
