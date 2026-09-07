package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Every advertised layout must have a working validator. Persist both valid
// and incomplete exchanges, then exercise the actual submission boundary.
func TestCheckedCatalogSubmissionExchanges(t *testing.T) {
	fixtures := map[string]string{
		"CA-QSO-PARTY": "001 ALAM",
		"MI-QSO-PARTY": "OAKL", "OH-QSO-PARTY": "CUYA",
		"GA-QSO-PARTY": "HARR", "FCG-FQP": "ALC",
		"AL-QSO-PARTY": "AUTA",
		"IAQP":         "STR",
		"CQ-WPX-CW":    "001", "DARC-WAEDC-CW": "001", "SAC-CW": "001",
		"OCEANIA-DX-CW": "001", "CQ-WW-CW": "05", "IARU-HF": "08",
		"STEW-PERRY": "EM75", "NAQP-CW": "GARY TN", "CQ-160-CW": "TN",
		"NA-SPRINT-CW": "001 GARY TN", "ARRL-DX-CW": "TN",
		"HELVETIA": "001", "RDXC": "001", "WAG": "001",
		"CWT": "GARY 1234", "CW-OPEN": "001 GARY",
		"K1USN-SST": "GARY TN", "TNQP": "HAMI", "TNQP-DX": "GA", "ARRL-SS-CW": "A 99 TN", "ICWC-MST": "GARY 001",
		"NCCC-SPRINT-CW":                   "001 GARY TN",
		"ARRL-RR-CW":                       "GARY 99 TN",
		"EA-MAJESTAD-CW":                   "001",
		"AADX-CW":                          "25",
		"EUHFC":                            "82",
		"NTC-QSO-PARTY":                    "GARY NM",
		"AGCW-HAPPY-NEW-YEAR-CONTEST":      "001/2583",
		"AGCW-QRP-CONTEST":                 "003 QRP NM",
		"KEYMAN-S-CLUB-OF-JAPAN-CONTEST":   "05",
		"KCJ-TOPBAND":                      "05",
		"40-80":                            "RM",
		"9A-DX":                            "08",
		"ALL-AUSTRIAN-160-METER-CONTEST":   "001",
		"HUNGARIAN-STRAIGHT-KEY-CONTEST":   "001 A",
		"UKEI-DX":                          "001",
		"A1CLUB-AWT":                       "GARY",
		"AGCW-YL-CW-PARTY":                 "001 OM GARY",
		"AGCW-STRAIGHT-KEY-PARTY":          "001 A GARY 39",
		"AGCW-QRP":                         "001 A",
		"AGCW-SEMI-AUTOMATIC-KEY-EVENING":  "001/89",
		"HIGH-SPEED-CLUB-CW-CONTEST":       "NM",
		"HA-DX":                            "001",
		"PACC":                             "001",
		"JIDX-CW":                          "05",
		"EUCW-160M":                        "GARY NM",
		"QRP-FOX-HUNT":                     "TN GARY 5W",
		"LZ-INTERNATIONAL-6-METER-CONTEST": "001 EM75AA",
		"REF-DDFM-6M-CONTEST":              "001 EM75",
		"NRAU-CW":                          "001 TA",
		"QRP-ARCI-HOLIDAY-SPIRITS-SPRINT":  "TN 5W",
		"QRP-ARCI-SUMMER-HOMEBREW-SPRINT":  "TN 5W",
		"QRP-ARCI-TOPBAND-SPRINT":          "TN 5W",
		"AZ-QSO-PARTY":                     "CT",
		"VA-QSO-PARTY":                     "001 CT",
		"IRTS-80M-COUNTIES":                "001",
		"NEQP":                             "CT",
		"DE-QSO-PARTY":                     "CT",
		"AR-QSO-PARTY":                     "CT",
		"MO-QSO-PARTY":                     "CT",
		"MS-QSO-PARTY":                     "CT",
		"OK-QSO-PARTY":                     "CT",
		"PA-QSO-PARTY":                     "001 CT",
		"NC-QSO-PARTY":                     "CT",
		"HI-QSO-PARTY":                     "CT",
		"ID-QSO-PARTY":                     "CT",
		"ND-QSO-PARTY":                     "CT",
		"KS-QSO-PARTY":                     "CT",
		"LA-QSO-PARTY":                     "CT",
		"MN-QSO-PARTY":                     "GARY CT",
		"NH-QSO-PARTY":                     "CT",
		"ME-QSO-PARTY":                     "CT",
		"WIQP":                             "CT",
		"KYQP":                             "CT",
		"BC-QSO-PARTY":                     "CT",
		"ON-QSO-PARTY":                     "CT",
		"CANADA-WINTER":                    "001",
		"CANADA-DAY":                       "001",
		"EA-QRP-CW-CONTEST":                "B M",
		"7QP":                              "CT",
		"NY-QSO-PARTY":                     "CT",
		"NE-QSO-PARTY":                     "CT",
		"LZ-DX":                            "08",
		"BALKAN-HF":                        "001",
		"ARI-DX":                           "001",
		"SACW":                             "08",
		"CVA-DX-CW":                        "NA",
		"MCD-QSO-PARTY":                    "001",
		"MDC-QSO-PARTY":                    "CT",
		"HA3NS-SPRINT":                     "NM",
		"DIG-QSO-PARTY":                    "001",
		"ARRL-160M":                        "CT",
		"GERMAN-TELEGRAPHY-CONTEST":        "",
		"CP-QSO-PARTY":                     "CT",
		"IL-QSO-PARTY":                     "CT",
		"IN-QSO-PARTY":                     "CT",
		"NM-QSO-PARTY":                     "CT",
		"OK-OM-DX":                         "001",
		"EUDXC":                            "08",
		"REF-CW":                           "001",
		"KENTUCKY-STATE-PARKS-ON-THE-AIR":  "CT",
		"ARSI-VU-DX":                       "001",
		"BEKASI-MERDEKA-CONTEST":           "001",
		"QC-QSO-PARTY":                     "CT",
		"AC-QSO-PARTY":                     "CT",
		"COQP":                             "CT",
		"NJQP":                             "CT",
		"ISLAND-QSO-PARTY":                 "CT",
		"TR-HF":                            "001",
		"REF-160-METER-CONTEST":            "001",
		"VENEZUELAN-IND-DAY-CONTEST":       "001",
		"DARC-CWA":                         "001",
		"DARC-10":                          "001",
		"EASTER":                           "001",
		"RSGB-IOTA":                        "001 EU-005",
		"AP-SPRINT":                        "001", "ASIA-PACIFIC-SPRING-SPRINT": "001", "BALTIC-CONTEST": "001",
		"MINITEST-40": "001", "MINITEST-80": "001", "MMC-HF-CW": "001",
		"RSGB-80M-AUT": "001", "RSGB-80M-CC": "001", "RSGB-AFS-CW": "001", "RSGB-NFD": "001",
		"IARU-REGION-1-FIELD-DAY": "001", "SEANET-CONTEST": "001", "SARL-HF-CW": "001", "TTC-SPCWC": "001",
		"SA10M": "05", "WWSA": "05",
		"RADIO-160": "EM75", "RADIO-YL-OM": "73",
		"SLOW-CW-QSO-PARTY": "001", "RAEM": "001 57N 85O",
		"SPDX": "001", "UKRAINIAN-DX": "001", "YO-DX-HF": "001", "XMAS": "001",
		"UBA-DX-CW": "001", "UBA-ON-6M": "001", "UBA-ON-CW": "001",
		"UBA-SPRING-CONTEST": "001", "UBA-SPRING-CONTEST-2": "001",
		"SC-QSO-PARTY": "TN", "SDQSOP": "TN", "TXQP": "TN", "VT-QSO-PARTY": "TN",
		"WA-SALMON-RUN": "TN", "WVQP": "TN",
		"ARS-SPARTAN-SPRINT": "TN 100", "ARS-FLIGHT-OF-THE-BUMBLEBEES": "TN 100",
		"MI-QRP-LABOR-DAY-CW-SPRINT": "TN 100", "NAQCC-CW-SPRINT": "TN 100",
		"4-STATES-QRP-GROUP-SECOND-SUNDAY": "TN 100", "QRP-ARCI-FALL-QSO-PARTY": "TN 100",
		"QRP-ARCI-HOOTOWL-SPRINT": "TN 100", "QRP-ARCI-SPRING-QSO-PARTY": "TN 100",
		"RUN-FOR-THE-BACON-QRP-CONTEST":  "TN 100",
		"WALK-FOR-THE-BACON-QRP-CONTEST": "TN GARY 100",
		"SKCC-SPRINT":                    "TN GARY 100", "SKCC-SPRINT-EUROPE": "TN GARY 100", "SKCC-WEEKEND-SPRINTATHON": "TN GARY 100",
		"PORTUGAL-DAY": "001", "RDAC": "001", "PRO-CW-CONTEST": "001",
		"RUSSIAN-RADIO-TEAM-CHAMPIONSHIP": "05", "RSGB-160": "001", "RSGB-LOW-POWER": "001 100",
	}
	catalog, err := loadEventCatalog()
	if err != nil {
		t.Fatal(err)
	}
	for _, event := range catalog {
		if !event.cabrilloReady() {
			continue
		}
		t.Run(event.ID, func(t *testing.T) {
			exchange, ok := fixtures[event.ID]
			if !ok {
				t.Fatal("checked catalog event lacks a submission fixture")
			}
			st, err := openStore(filepath.Join(t.TempDir(), "logger.db"))
			if err != nil {
				t.Fatal(err)
			}
			defer st.Close()
			profile, err := st.activeStationProfile()
			if err != nil {
				t.Fatal(err)
			}
			profile.Callsign = "W4GNS"
			q := validTestQSO()
			q.profileID, q.contestID = profile.ID, event.ID
			if len(event.Bands) > 0 && !bandAllowed(event.Bands, q.band) {
				q.band = event.Bands[0]
			}
			if event.QSOParty != nil {
				q.time = event.QSOParty.Periods[0].Start
				q.timeOff = q.time
			}
			q.call, q.stationCallsign = "W1AW", ""
			q.rstSent, q.rstRcvd = "599", "599"
			q.stx, q.srx, q.stxString, q.srxString = "", "", exchange, exchange
			if event.ID == "ARRL-SS-CW" {
				q.stx, q.srx = "001", "002"
			}
			if event.ID == "STEW-PERRY" {
				q.rstSent, q.rstRcvd = "", ""
			}
			id, err := st.insertQSO(q)
			if err != nil {
				t.Fatal(err)
			}
			var out bytes.Buffer
			count, _, err := exportCabrillo(context.Background(), &out, profile, event, event.ID, st)
			if err != nil || count != 1 || !strings.Contains(out.String(), "END-OF-LOG:") {
				t.Fatalf("valid export: count=%d err=%v", count, err)
			}
			for _, side := range []string{"sent", "received"} {
				invalidExchanges := []string{"", "ZZ!", exchange + " EXTRA", exchange + "\n"}
				if event.ID == "DIG-QSO-PARTY" || event.ID == "ARRL-160M" || event.ID == "GERMAN-TELEGRAPHY-CONTEST" {
					// Sponsor rules require no exchange beyond RST for non-members.
					invalidExchanges = invalidExchanges[1:]
				}
				for _, invalid := range invalidExchanges {
					changed := q
					if side == "sent" {
						changed.stxString = invalid
						if invalid == "" {
							changed.stx = ""
						}
					} else {
						changed.srxString = invalid
						if invalid == "" {
							changed.srx = ""
						}
					}
					_, err := st.db.Exec("UPDATE qso SET stx=?, stx_string=?, srx=?, srx_string=? WHERE id=?",
						changed.stx, changed.stxString, changed.srx, changed.srxString, id)
					if err != nil {
						t.Fatal(err)
					}
					out.Reset()
					_, _, err = exportCabrillo(context.Background(), &out, profile, event, event.ID, st)
					if err == nil || !strings.Contains(err.Error(), side) || strings.Contains(out.String(), "END-OF-LOG:") {
						t.Fatalf("%s invalid %q: err=%v", side, invalid, err)
					}
				}
			}
		})
	}
}

// TestQSOPartyOutOfStateExchange covers the case TestCheckedCatalogSubmissionExchanges
// misses: every state QSO party's own fixture there is an in-state county used
// for both sent and received, so an out-of-state operator's sent exchange (a
// bare state/province code, no county at all) was never exercised end to
// end. That gap is exactly what let a real TNQP session log three QSOs with
// an empty sent exchange before Cabrillo export caught it.
func TestQSOPartyOutOfStateExchange(t *testing.T) {
	catalog, err := loadEventCatalog()
	if err != nil {
		t.Fatal(err)
	}
	for _, event := range catalog {
		if !event.cabrilloReady() || event.QSOParty == nil {
			continue
		}
		t.Run(event.ID, func(t *testing.T) {
			outOfState := "VA"
			if event.QSOParty.State == outOfState {
				outOfState = "NC"
			}
			st, err := openStore(filepath.Join(t.TempDir(), "logger.db"))
			if err != nil {
				t.Fatal(err)
			}
			defer st.Close()
			profile, err := st.activeStationProfile()
			if err != nil {
				t.Fatal(err)
			}
			profile.Callsign = "W4GNS"
			q := validTestQSO()
			q.profileID, q.contestID = profile.ID, event.ID
			q.time = event.QSOParty.Periods[0].Start
			q.timeOff = q.time
			if len(event.Bands) > 0 && !bandAllowed(event.Bands, q.band) {
				q.band = event.Bands[0]
			}
			q.call, q.stationCallsign = "W1AW", ""
			q.rstSent, q.rstRcvd = "599", "599"
			homeCounty := event.CountyOptions[0].Code
			q.stx, q.srx = "", ""
			q.stxString, q.srxString = outOfState, homeCounty
			if event.QSOParty.Exchange == "serial_location" {
				q.stx, q.srx = "001", "002"
			}
			if _, err := st.insertQSO(q); err != nil {
				t.Fatal(err)
			}
			var out bytes.Buffer
			count, _, err := exportCabrillo(context.Background(), &out, profile, event, event.ID, st)
			if err != nil || count != 1 || !strings.Contains(out.String(), "END-OF-LOG:") {
				t.Fatalf("out-of-state sent exchange %q: count=%d err=%v\n%s", outOfState, count, err, out.String())
			}
		})
	}
}

func TestSubmissionTokenGrammar(t *testing.T) {
	for _, tt := range []struct {
		event, call, serial, text string
		valid                     bool
	}{
		{"CQ-WPX-CW", "W1AW", "001", "CA", false},
		{"CQ-WPX-CW", "W1AW", "", "000", false},
		{"CQ-WW-CW", "W1AW", "001", "05", false},
		{"CQ-WW-CW", "W1AW", "", "+5", false},
		{"CQ-WW-CW", "W1AW", "", "41", false},
		{"IARU-HF", "W1AW", "", "91", false},
		{"IARU-HF", "W1AW", "", "-1", false},
		{"IARU-HF", "W1AW", "", "BOGUS", false},
		{"IARU-HF", "W1AW", "", "arrl", true},
		{"IARU-HF", "W1AW", "001", "ARRL", false},
		{"IARU-HF", "W1AW", "", "R2", true},
		{"STEW-PERRY", "W1AW", "", "SS00", false},
		{"STEW-PERRY", "W1AW", "", "EM75AA", false},
		{"STEW-PERRY", "W1AW", "", "em75", true},
		{"ARRL-DX-CW", "W1AW", "", "ON", false},
		{"ARRL-DX-CW", "VE3ABC", "", "CA", false},
		{"ARRL-DX-CW", "VE3ABC", "", "ON", true},
		{"ARRL-DX-CW", "VE3ABC", "", "AB BC", false},
		{"ARRL-DX-CW", "DL1ABC", "", "100", true},
		{"ARRL-DX-CW", "KH6ABC", "", "KW", true},
		{"ARRL-DX-CW", "DL1ABC", "", "100W", true},
		{"ARRL-DX-CW", "DL1ABC", "", "ATT", true},
		{"ARRL-DX-CW", "DL1ABC", "", "1.5K", true},
		{"ARRL-DX-CW", "DL1ABC", "", "0", false},
		{"ARRL-DX-CW", "DL1ABC", "", "NaN", false},
		{"ARRL-DX-CW", "DL1ABC", "", "ZZ", false},
		{"CQ-160-CW", "DL1ABC", "", "14", true},
		{"CQ-160-CW", "DL1ABC", "", "41", false},
		{"CQ-160-CW", "W1AW", "", "05", false},
		{"NAQP-CW", "W1AW", "", "BOB", false},
		{"NAQP-CW", "W1AW", "", "123 TN", false},
		{"NAQP-CW", "W1AW", "", "BOB ZZ", false},
		{"NAQP-CW", "DL1ABC", "", "HANS", true},
		{"NAQP-CW", "DL1ABC", "", "HANS DX", true},
		{"NAQP-CW", "XE1ABC", "", "JOSE XE", true},
		{"NAQP-CW", "XE1ABC", "", "JOSE XENO", false},
		{"NAQP-CW", "KH6ABC", "", "BOB HI", true},
		{"NA-SPRINT-CW", "W1AW", "001", "BOB TN", true},
		{"NA-SPRINT-CW", "W1AW", "000", "BOB TN", false},
		{"NA-SPRINT-CW", "DL1ABC", "001", "HANS", false},
		{"NA-SPRINT-CW", "DL1ABC", "001", "HANS DL", true},
		{"CWT", "W1AW", "", "BOB 000", false},
		{"CWT", "DL1ABC", "", "HANS DL", true},
		{"K1USN-SST", "DL1ABC", "", "HANS DL", true},
		{"K1USN-SST", "W1AW", "001", "BOB TN", false},
		{"ARRL-RR-CW", "W1AW", "", "BOB 99 TN", true},
		{"ARRL-RR-CW", "VE3ABC", "", "AL 00 ON", true},
		{"ARRL-RR-CW", "XE1ABC", "", "JOSE 08 XE1", true},
		{"ARRL-RR-CW", "XE1ABC", "", "JOSE 08 XE", false},
		{"ARRL-RR-CW", "DL1ABC", "", "HANS 99 DX", true},
		{"ARRL-RR-CW", "DL1ABC", "", "HANS 1999 DX", false},
		{"AADX-CW", "W1AW", "", "01", true},
		{"AADX-CW", "W1AW", "", "1", false},
		{"AADX-CW", "W1AW", "", "101", false},
		{"EUHFC", "W1AW", "", "82", true},
		{"EUHFC", "W1AW", "", "1982", false},
		{"NTC-QSO-PARTY", "W1AW", "", "GARY NM", true},
		{"NTC-QSO-PARTY", "W1AW", "", "GARY 040", true},
		{"NTC-QSO-PARTY", "W1AW", "", "NM", false},
		{"AGCW-HAPPY-NEW-YEAR-CONTEST", "W1AW", "", "001", true},
		{"AGCW-HAPPY-NEW-YEAR-CONTEST", "W1AW", "", "001/2583", true},
		{"AGCW-HAPPY-NEW-YEAR-CONTEST", "W1AW", "", "001/", false},
		{"AGCW-QRP-CONTEST", "W1AW", "", "003 QRP NM", true},
		{"AGCW-QRP-CONTEST", "W1AW", "", "003 LP NM", false},
		{"EA-MAJESTAD-CW", "EA1ABC", "", "M", true},
		{"EA-MAJESTAD-CW", "EA8ABC", "", "GC", true},
		{"EA-MAJESTAD-CW", "EA1ABC", "", "001", false},
		{"EA-MAJESTAD-CW", "W1AW", "", "001", true},
		{"EA-MAJESTAD-CW", "W1AW", "", "M", false},
		{"TNQP", "W1AW", "", "ZZZZ", false},
		{"TNQP", "W1AW", "", "TN", false},
		{"TNQP", "DL1ABC", "", "DX", true},
		{"TNQP-DX", "W1AW", "", "ZZZZ", false},
		{"TNQP-DX", "W1AW", "", "TN", false},
		{"TNQP-DX", "DL1ABC", "", "DX", true},
		{"DIG-QSO-PARTY", "W1AW", "", "", true},
		{"DIG-QSO-PARTY", "W1AW", "", "NM", false},
		{"ARRL-160M", "DL1ABC", "", "", true},
		{"ARRL-160M", "W1AW", "", "", false},
		{"ARRL-160M", "W1AW", "", "ZZ", false},
		{"GERMAN-TELEGRAPHY-CONTEST", "DL1ABC", "", "MZ", true},
		{"GERMAN-TELEGRAPHY-CONTEST", "DL1ABC", "", "", false},
		{"CP-QSO-PARTY", "VE4ABC", "", "RGQ", true},
		{"CP-QSO-PARTY", "VE4ABC", "", "MB", false},
		{"CP-QSO-PARTY", "W1AW", "", "CT", true},
		{"CP-QSO-PARTY", "DL1ABC", "", "DX", true},
		{"CP-QSO-PARTY", "DL1ABC", "", "CT", false},
		{"IL-QSO-PARTY", "W9ABC", "", "COOK", true},
		{"IL-QSO-PARTY", "W1AW", "", "CT", true},
		{"IL-QSO-PARTY", "DL1ABC", "", "DX", true},
		{"IL-QSO-PARTY", "W1AW", "", "ZZZZ", false},
		{"IN-QSO-PARTY", "W9ABC", "", "INMRN", true},
		{"IN-QSO-PARTY", "W1AW", "", "CT", true},
		{"IN-QSO-PARTY", "DL1ABC", "", "DX", true},
		{"IN-QSO-PARTY", "W1AW", "", "INMAR", false},
		{"NM-QSO-PARTY", "W5ABC", "", "BER", true},
		{"NM-QSO-PARTY", "W1AW", "", "CT", true},
		{"NM-QSO-PARTY", "DL1ABC", "", "DX", true},
		{"NM-QSO-PARTY", "W1AW", "", "ABC", false},
		{"OK-OM-DX", "OK1ABC", "", "PRA", true},
		{"OK-OM-DX", "OK1ABC", "", "001", false},
		{"OK-OM-DX", "W1AW", "", "001", true},
		{"EUDXC", "W1AW", "", "DE17", false},
		{"EUDXC", "W1AW", "", "DE16", true},
		{"REF-CW", "F6ABC", "", "75", true},
		{"REF-CW", "F6ABC", "", "000", false},
		{"UNKNOWN", "W1AW", "001", "", false},
	} {
		t.Run(strings.Join([]string{tt.event, tt.call, tt.serial, tt.text}, "/"), func(t *testing.T) {
			err := validateSubmissionExchange(tt.event, tt.call, tt.serial, tt.text)
			if (err == nil) != tt.valid {
				t.Fatalf("valid=%v err=%v", tt.valid, err)
			}
		})
	}
}

func TestInvalidSubmissionPreservesExistingExport(t *testing.T) {
	dir := t.TempDir()
	st, err := openStore(filepath.Join(dir, "logger.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	profile, err := st.activeStationProfile()
	if err != nil {
		t.Fatal(err)
	}
	profile.Callsign = "W4GNS"
	q := validTestQSO()
	q.profileID, q.contestID = profile.ID, "CQ-WPX-CW"
	q.stx, q.srx, q.stxString, q.srxString = "001", "", "", ""
	if _, err := st.insertQSO(q); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "submission.cbr")
	if err := os.WriteFile(path, []byte("previous export"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := writeCabrilloAtomic(context.Background(), dir, path, profile, testEventDefinition(), q.contestID, st); err == nil {
		t.Fatal("incomplete exchange exported")
	}
	data, err := os.ReadFile(path)
	if err != nil || string(data) != "previous export" {
		t.Fatalf("previous export changed: %q, %v", data, err)
	}
	temps, err := filepath.Glob(filepath.Join(dir, ".w4gns-cabrillo-*"))
	if err != nil || len(temps) != 0 {
		t.Fatalf("temporary export left behind: %v, %v", temps, err)
	}
}
