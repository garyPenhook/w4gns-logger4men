package main

import (
	"math"
	"testing"
)

func TestParseGridSquareNormalizesAndFindsCellCentre(t *testing.T) {
	grid, err := ParseGridSquare(" fn31pr ")
	if err != nil {
		t.Fatalf("ParseGridSquare returned error: %v", err)
	}
	if grid.Locator != "FN31PR" {
		t.Fatalf("Locator = %q, want FN31PR", grid.Locator)
	}
	if math.Abs(grid.Latitude-41.7291666667) > 0.000001 {
		t.Errorf("Latitude = %.10f", grid.Latitude)
	}
	if math.Abs(grid.Longitude-(-72.7083333333)) > 0.000001 {
		t.Errorf("Longitude = %.10f", grid.Longitude)
	}
}

func TestParseGridSquareAcceptsStandardPrecisions(t *testing.T) {
	for _, locator := range []string{"FN", "FN31", "FN31PR", "FN31PR42", "FN31PR42AA"} {
		if _, err := ParseGridSquare(locator); err != nil {
			t.Errorf("ParseGridSquare(%q) returned error: %v", locator, err)
		}
	}
}

func TestParseGridSquareRejectsInvalidLocators(t *testing.T) {
	for _, locator := range []string{"", "F", "FN3", "SN31", "FN3A", "FN31PZ", "FN31PR4A", "FN31PR42AZ"} {
		if _, err := ParseGridSquare(locator); err == nil {
			t.Errorf("ParseGridSquare(%q) succeeded, want error", locator)
		}
	}
}
