package geo

import (
	"math"
	"testing"
)

func TestValidCoordinates(t *testing.T) {
	for _, p := range [][2]float64{{0, 0}, {-90, -180}, {90, 180}, {41.7, -72.7}} {
		if !ValidCoordinates(p[0], p[1]) {
			t.Errorf("rejected valid point %v", p)
		}
	}
	for _, p := range [][2]float64{{91, 0}, {-91, 0}, {0, 181}, {0, -181}, {math.NaN(), 0}, {0, math.NaN()}, {math.Inf(1), 0}, {0, math.Inf(-1)}} {
		if ValidCoordinates(p[0], p[1]) {
			t.Errorf("accepted invalid point %v", p)
		}
	}
}
