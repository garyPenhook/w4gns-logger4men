package main

import (
	"fmt"
	"strings"
)

// GridSquare is a normalized Maidenhead locator and the centre point of the
// locator cell. Latitude and longitude are expressed in decimal degrees.
type GridSquare struct {
	Locator   string
	Latitude  float64
	Longitude float64
}

// ParseGridSquare validates and normalizes a standard 2, 4, 6, 8, or
// 10-character Maidenhead locator. The returned coordinates identify the
// centre of the locator cell, not the station's exact location.
func ParseGridSquare(locator string) (GridSquare, error) {
	grid := strings.ToUpper(strings.TrimSpace(locator))
	if len(grid) != 2 && len(grid) != 4 && len(grid) != 6 && len(grid) != 8 && len(grid) != 10 {
		return GridSquare{}, fmt.Errorf("grid square must contain 2, 4, 6, 8, or 10 characters")
	}
	if !inRange(grid[0], 'A', 'R') || !inRange(grid[1], 'A', 'R') {
		return GridSquare{}, fmt.Errorf("grid square field must use letters A through R")
	}

	lon := float64(grid[0]-'A')*20 - 180
	lat := float64(grid[1]-'A')*10 - 90
	lonWidth, latHeight := 20.0, 10.0

	if len(grid) >= 4 {
		if !inRange(grid[2], '0', '9') || !inRange(grid[3], '0', '9') {
			return GridSquare{}, fmt.Errorf("grid square must contain digits at positions 3 and 4")
		}
		lonWidth, latHeight = 2, 1
		lon += float64(grid[2]-'0') * lonWidth
		lat += float64(grid[3]-'0') * latHeight
	}
	if len(grid) >= 6 {
		if !inRange(grid[4], 'A', 'X') || !inRange(grid[5], 'A', 'X') {
			return GridSquare{}, fmt.Errorf("grid square subsquare must use letters A through X")
		}
		lonWidth, latHeight = 2.0/24, 1.0/24
		lon += float64(grid[4]-'A') * lonWidth
		lat += float64(grid[5]-'A') * latHeight
	}
	if len(grid) >= 8 {
		if !inRange(grid[6], '0', '9') || !inRange(grid[7], '0', '9') {
			return GridSquare{}, fmt.Errorf("extended grid square must contain digits at positions 7 and 8")
		}
		lonWidth, latHeight = (2.0/24)/10, (1.0/24)/10
		lon += float64(grid[6]-'0') * lonWidth
		lat += float64(grid[7]-'0') * latHeight
	}
	if len(grid) == 10 {
		if !inRange(grid[8], 'A', 'X') || !inRange(grid[9], 'A', 'X') {
			return GridSquare{}, fmt.Errorf("extended grid subsquare must use letters A through X")
		}
		lonWidth, latHeight = ((2.0/24)/10)/24, ((1.0/24)/10)/24
		lon += float64(grid[8]-'A') * lonWidth
		lat += float64(grid[9]-'A') * latHeight
	}

	return GridSquare{
		Locator:   grid,
		Latitude:  lat + latHeight/2,
		Longitude: lon + lonWidth/2,
	}, nil
}

func inRange(value, lower, upper byte) bool {
	return value >= lower && value <= upper
}
