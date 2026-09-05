package main

import (
	"regexp"
	"strings"
	"time"
)

// iotaReferencePattern matches an IOTA island-group reference (e.g. "EU-005",
// "NA-065", "OC-001") in free-form text such as a cluster spot comment or a
// contest exchange. The continent prefix is always exactly two letters
// (AF/AN/AS/EU/NA/OC/SA), and the number is always exactly three digits per
// the RSGB IOTA directory, so this is tighter than POTA's pattern and won't
// false-positive on things like "RS-599" or a two-digit serial number.
var iotaReferencePattern = regexp.MustCompile(`(?i)\b(AF|AN|AS|EU|NA|OC|SA)-\d{3}\b`)

// recentClusterIOTAReference scans spots (newest-first, per
// model.addClusterSpot which prepends each new spot) for the most recent spot
// for call within the dupe window, returning its IOTA reference if its
// comment has one. Mirrors recentClusterPOTAReference in pota.go; there is no
// equivalent free public IOTA spot API to poll, so cluster-comment scanning is
// the only auto-fill source.
func recentClusterIOTAReference(spots []clusterSpot, call string, now time.Time) (string, bool) {
	cutoff := now.UTC().Add(-dupeWindow)
	for _, spot := range spots {
		if !strings.EqualFold(spot.Callsign, call) || spot.Received.Before(cutoff) {
			continue
		}
		if reference := iotaReferencePattern.FindString(spot.Comment); reference != "" {
			return strings.ToUpper(reference), true
		}
	}
	return "", false
}

// iotaReferenceCode extracts and canonicalizes an IOTA reference (e.g.
// "eu-005" -> "EU-005") from free-form exchange text, such as a contest's
// received exchange string. Returns "" if no valid reference is found, so
// callers can use it directly as a multiplier key without a second check.
func iotaReferenceCode(text string) string {
	return strings.ToUpper(iotaReferencePattern.FindString(text))
}

// iotaMultiplierValue determines the IOTA multiplier value for a scored QSO.
// The received exchange text is authoritative (the RSGB IOTA Contest
// exchange is "RS(T) + Serial No. + IOTA No."), but the operator's own
// dedicated IOTA Ref field is accepted as a fallback so a reference entered
// or auto-filled there (rather than typed into the exchange) still counts.
func iotaMultiplierValue(q qso) string {
	if code := iotaReferenceCode(q.srxString); code != "" {
		return code
	}
	return iotaReferenceCode(q.iotaRef)
}

// iotaCategory classifies a QSO for pointsRule.IOTA (contest_state.go's
// iotaPointCategory): the operator's own island status comes from the QSO's
// snapshotted myIotaRef, never a live station-profile lookup, so a later
// profile edit can't retroactively change a logged QSO's score.
func iotaCategory(q qso) iotaPointCategory {
	myRef := iotaReferenceCode(q.myIotaRef)
	workedRef := iotaMultiplierValue(q)
	switch {
	case myRef == "" && workedRef == "":
		return iotaCategoryWorldWorksWorld
	case myRef == "" && workedRef != "":
		return iotaCategoryWorldWorksIsland
	case myRef != "" && workedRef == "":
		return iotaCategoryIslandWorksWorld
	case myRef == workedRef:
		return iotaCategoryIslandWorksSameReference
	default:
		return iotaCategoryIslandWorksOtherIsland
	}
}
