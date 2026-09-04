package main

import "strings"

// tnCountyCodes is the "tn_county" multiplier kind's canonical value set: the
// official four-letter abbreviation for each of Tennessee's 95 counties, the
// same codes events/tnqp.json's received_exchange_options offers as typeahead
// on the received-exchange field. This is the multiplier tnqp.org/rules
// awards an out-of-state entrant for each distinct county worked (up to 95
// per band) — the only multiplier category that applies to this app's own
// station profile, which the catalog documents as out-of-state (README.md).
var tnCountyCodes = map[string]bool{
	"ANDE": true, "BEDF": true, "BENT": true, "BLED": true, "BLOU": true,
	"BRAD": true, "CAMP": true, "CANN": true, "CARR": true, "CART": true,
	"CHEA": true, "CHES": true, "CLAI": true, "CLAY": true, "COCK": true,
	"COFF": true, "CROC": true, "CUMB": true, "DAVI": true, "DECA": true,
	"DEKA": true, "DICK": true, "DYER": true, "FAYE": true, "FENT": true,
	"FRAN": true, "GIBS": true, "GILE": true, "GRAI": true, "GREE": true,
	"GRUN": true, "HAMB": true, "HAMI": true, "HANC": true, "HARD": true,
	"HARN": true, "HAWK": true, "HAYW": true, "HEND": true, "HENR": true,
	"HICK": true, "HOUS": true, "HUMP": true, "JACK": true, "JEFF": true,
	"JOHN": true, "KNOX": true, "LAKE": true, "LAUD": true, "LAWR": true,
	"LEWI": true, "LINC": true, "LOUD": true, "MACO": true, "MADI": true,
	"MARI": true, "MARS": true, "MAUR": true, "MCMI": true, "MCNA": true,
	"MEIG": true, "MONR": true, "MONT": true, "MOOR": true, "MORG": true,
	"OBIO": true, "OVER": true, "PERR": true, "PICK": true, "POLK": true,
	"PUTN": true, "RHEA": true, "ROAN": true, "ROBE": true, "RUTH": true,
	"SCOT": true, "SEQU": true, "SEVI": true, "SHEL": true, "SMIT": true,
	"STEW": true, "SULL": true, "SUMN": true, "TIPT": true, "TROU": true,
	"UNIC": true, "UNIO": true, "VANB": true, "WARR": true, "WASH": true,
	"WAYN": true, "WEAK": true, "WHIT": true, "WILL": true, "WILS": true,
}

// tnCountyCode extracts the tn_county multiplier value from a received-
// exchange text, or "" if it doesn't match one of the 95 official county
// codes — an out-of-state station's own sent exchange (a state/province
// abbreviation) never coincidentally matches one of these, so this is safe
// to apply unconditionally to any received exchange text, the same shape as
// exchange_area.go's exchangeAreaCode.
func tnCountyCode(text string) string {
	code := strings.ToUpper(strings.TrimSpace(text))
	if tnCountyCodes[code] {
		return code
	}
	return ""
}
