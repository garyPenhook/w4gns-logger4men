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
		"K1USN-SST": "GARY TN", "TNQP": "HAMI", "ARRL-SS-CW": "A 99 TN",
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
				for _, invalid := range []string{"", "ZZ!", exchange + " EXTRA", exchange + "\n"} {
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
		{"TNQP", "W1AW", "", "ZZZZ", false},
		{"TNQP", "W1AW", "", "TN", false},
		{"TNQP", "DL1ABC", "", "DX", true},
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
