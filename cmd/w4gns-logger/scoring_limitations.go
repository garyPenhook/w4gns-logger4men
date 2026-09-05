package main

// Capability describes implemented machinery, not a sponsor-certified score.
func eventScoringLimitation(id string) string {
	switch id {
	case "SAC-CW":
		return "non-Scandinavian European entrant scoring is not supported"
	case "TNQP":
		return "CW scoring covers the logger's supported bands through 6M; choose MOBILE/ROVER in Station Setup for county activation bonuses"
	case "FCG-FQP":
		return "power factor uses declared Station Setup category (blank means HIGH); physical/category eligibility is not adjudicated"
	case "CA-QSO-PARTY", "MI-QSO-PARTY", "OH-QSO-PARTY", "GA-QSO-PARTY", "AL-QSO-PARTY", "IAQP":
		return "CW score uses verified event periods; operator on-time and physical/category eligibility are not adjudicated"
	case "STEW-PERRY":
		return "power-class bonuses are not included"
	case "DARC-WAEDC-CW":
		return "QTC bonuses and opposite-continent QSO eligibility are not included; country matching is approximate"
	case "RDXC", "WAG":
		return "country entities use the bundled prefix table; sponsor-specific entity distinctions may differ"
	}
	return ""
}
