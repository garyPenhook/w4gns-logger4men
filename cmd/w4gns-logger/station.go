package main

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"
	_ "time/tzdata"
)

type stationProfile struct {
	ID           int64
	Name         string
	Callsign     string
	OperatorName string
	MyGridSquare string
	// MyIOTARef, when set, declares the station is operating as an IOTA
	// island station (e.g. for the RSGB IOTA Contest) rather than a "world"
	// station — see iotaPointsRule. Empty means "not an island station";
	// there is no separate boolean, so clearing this field is how an
	// operator returns to world-station status after an activation.
	MyIOTARef  string
	Latitude   *float64
	Longitude  *float64
	Timezone   string
	Club       string
	Rig        string
	Antenna    string
	PowerWatts string

	// Contest-submission (Cabrillo) header fields. Free text: contest
	// sponsors' accepted values (e.g. CATEGORY-OPERATOR: SINGLE-OP,
	// CHECKLOG; CATEGORY-POWER: QRP, LOW, HIGH) vary enough between contests
	// that validating against a fixed list would reject legitimate values.
	CategoryOperator string
	CategoryAssisted string
	CategoryPower    string
	CategoryStation  string
	Address          string
}

func (s *store) activeStationProfile() (stationProfile, error) {
	var profile stationProfile
	var latitude, longitude sqlNullFloat64
	err := s.db.QueryRow(`SELECT id, name, COALESCE(callsign, ''), COALESCE(operator_name, ''), COALESCE(my_gridsquare, ''), latitude, longitude, timezone, COALESCE(club, ''), COALESCE(rig, ''), COALESCE(antenna, ''), COALESCE(CAST(power_watts AS TEXT), ''), COALESCE(category_operator, ''), COALESCE(category_assisted, ''), COALESCE(category_power, ''), COALESCE(address, ''), COALESCE(category_station, ''), COALESCE(my_iota_ref, '')
		FROM station_profile ORDER BY id LIMIT 1`).Scan(
		&profile.ID, &profile.Name, &profile.Callsign, &profile.OperatorName, &profile.MyGridSquare,
		&latitude, &longitude, &profile.Timezone, &profile.Club, &profile.Rig, &profile.Antenna, &profile.PowerWatts,
		&profile.CategoryOperator, &profile.CategoryAssisted, &profile.CategoryPower, &profile.Address,
		&profile.CategoryStation, &profile.MyIOTARef,
	)
	if err != nil {
		return stationProfile{}, fmt.Errorf("load active station profile: %w", err)
	}
	profile.Latitude = latitude.pointer()
	profile.Longitude = longitude.pointer()
	if profile.PowerWatts != "" {
		value, err := strconv.ParseFloat(profile.PowerWatts, 64)
		if err != nil || math.IsNaN(value) || math.IsInf(value, 0) || value < 0 {
			return stationProfile{}, fmt.Errorf("read station power: invalid stored value %q", profile.PowerWatts)
		}
		profile.PowerWatts = strconv.FormatFloat(value, 'f', -1, 64)
	}
	return profile, nil
}

// saveStationProfile validates all station settings before replacing the active
// profile. It keeps the original grid-square precision while storing the cell
// centre only as derived location data.
func (s *store) saveStationProfile(profile stationProfile) (stationProfile, error) {
	profile.Name = strings.TrimSpace(profile.Name)
	profile.Callsign = strings.ToUpper(strings.TrimSpace(profile.Callsign))
	profile.OperatorName = strings.TrimSpace(profile.OperatorName)
	profile.MyGridSquare = strings.ToUpper(strings.TrimSpace(profile.MyGridSquare))
	profile.MyIOTARef = strings.ToUpper(strings.TrimSpace(profile.MyIOTARef))
	profile.Timezone = strings.TrimSpace(profile.Timezone)
	profile.Club = strings.TrimSpace(profile.Club)
	profile.Rig = strings.TrimSpace(profile.Rig)
	profile.Antenna = strings.TrimSpace(profile.Antenna)
	profile.PowerWatts = strings.TrimSpace(profile.PowerWatts)
	profile.CategoryOperator = strings.ToUpper(strings.TrimSpace(profile.CategoryOperator))
	profile.CategoryAssisted = strings.ToUpper(strings.TrimSpace(profile.CategoryAssisted))
	profile.CategoryPower = strings.ToUpper(strings.TrimSpace(profile.CategoryPower))
	profile.CategoryStation = strings.ToUpper(strings.TrimSpace(profile.CategoryStation))
	if !containsString([]string{"", "FIXED", "MOBILE", "ROVER", "EXPEDITION", "PORTABLE", "EOC", "SCHOOL"}, profile.CategoryStation) {
		return stationProfile{}, fmt.Errorf("station category must be FIXED, MOBILE, ROVER, EXPEDITION, PORTABLE, EOC or SCHOOL")
	}
	profile.Address = strings.TrimSpace(profile.Address)
	if profile.Name == "" {
		return stationProfile{}, fmt.Errorf("station profile name is required")
	}
	if profile.Callsign != "" {
		if err := validateCallsignChars(profile.Callsign); err != nil {
			return stationProfile{}, fmt.Errorf("invalid station callsign: %w", err)
		}
	}
	if profile.Timezone == "" {
		return stationProfile{}, fmt.Errorf("station timezone is required")
	}
	if profile.Timezone == "Local" {
		return stationProfile{}, fmt.Errorf("station timezone must be an IANA identifier or UTC, not Local")
	}
	if _, err := time.LoadLocation(profile.Timezone); err != nil {
		return stationProfile{}, fmt.Errorf("invalid IANA timezone %q: %w", profile.Timezone, err)
	}
	var powerWatts any
	if profile.PowerWatts != "" {
		value, err := strconv.ParseFloat(profile.PowerWatts, 64)
		if err != nil || math.IsNaN(value) || math.IsInf(value, 0) || value < 0 {
			return stationProfile{}, fmt.Errorf("power must be a non-negative, finite number of watts")
		}
		powerWatts = value
		profile.PowerWatts = strconv.FormatFloat(value, 'f', -1, 64)
	}
	var latitude, longitude any
	if profile.MyGridSquare != "" {
		grid, err := ParseGridSquare(profile.MyGridSquare)
		if err != nil {
			return stationProfile{}, fmt.Errorf("invalid station grid square: %w", err)
		}
		profile.MyGridSquare = grid.Locator
		profile.Latitude, profile.Longitude = &grid.Latitude, &grid.Longitude
		latitude, longitude = grid.Latitude, grid.Longitude
	}
	if profile.MyIOTARef != "" && iotaReferenceCode(profile.MyIOTARef) != profile.MyIOTARef {
		return stationProfile{}, fmt.Errorf("invalid station IOTA reference %q: must look like EU-005", profile.MyIOTARef)
	}
	if profile.ID == 0 {
		return stationProfile{}, fmt.Errorf("station profile is missing an identifier")
	}
	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := s.db.Exec(`UPDATE station_profile SET name=?, callsign=?, operator_name=?, my_gridsquare=?, latitude=?, longitude=?, timezone=?, club=?, rig=?, antenna=?, power_watts=?, category_operator=?, category_assisted=?, category_power=?, address=?, category_station=?, my_iota_ref=?, updated_at=? WHERE id=?`,
		profile.Name, profile.Callsign, profile.OperatorName, profile.MyGridSquare, latitude, longitude, profile.Timezone, profile.Club, profile.Rig, profile.Antenna, powerWatts,
		profile.CategoryOperator, profile.CategoryAssisted, profile.CategoryPower, profile.Address, profile.CategoryStation, profile.MyIOTARef, now, profile.ID,
	); err != nil {
		return stationProfile{}, fmt.Errorf("save station profile: %w", err)
	}
	return profile, nil
}

// sqlNullFloat64 avoids exposing database/sql details to the UI model.
type sqlNullFloat64 struct {
	value float64
	valid bool
}

func (n *sqlNullFloat64) Scan(value any) error {
	if value == nil {
		n.valid = false
		return nil
	}
	switch v := value.(type) {
	case float64:
		n.value, n.valid = v, true
		return nil
	case int64:
		n.value, n.valid = float64(v), true
		return nil
	default:
		return fmt.Errorf("scan station coordinate %T", value)
	}
}

func (n sqlNullFloat64) pointer() *float64 {
	if !n.valid {
		return nil
	}
	value := n.value
	return &value
}
