package main

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

func TestRegionalExchangeSubmission(t *testing.T) {
	for _, tt := range []struct {
		name, event, call, serial, text string
		valid                           bool
	}{
		{"Swiss canton", "HELVETIA", "HB9ABC", "", "ZH", true},
		{"Swiss invalid canton", "HELVETIA", "HB9ABC", "", "ZZ", false},
		{"Swiss serial instead of canton", "HELVETIA", "HB9ABC", "001", "", false},
		{"Swiss double exchange", "HELVETIA", "HB9ABC", "001", "ZH", false},
		{"foreign Helvetia serial", "HELVETIA", "W4GNS", "001", "", true},
		{"imported serial in text", "HELVETIA", "W4GNS", "", "001", true},
		{"short Helvetia serial", "HELVETIA", "W4GNS", "1", "", false},
		{"foreign canton rejected", "HELVETIA", "W4GNS", "", "ZH", false},
		{"Russian oblast", "RDXC", "UA3ABC", "", "mo", true},
		{"Kaliningrad", "RDXC", "UA2ABC", "", "KA", true},
		{"Russian Antarctic", "RDXC", "RI1ANB", "", "AN", true},
		{"invalid oblast", "RDXC", "UA3ABC", "", "ZZ", false},
		{"missing oblast", "RDXC", "UA3ABC", "", "", false},
		{"foreign RDXC serial", "RDXC", "W4GNS", "001", "", true},
		{"foreign RDXC zero", "RDXC", "W4GNS", "000", "", false},
		{"German DOK", "WAG", "DL1ABC", "", "A01", true},
		{"German nonmember", "WAG", "DL1ABC", "", "NM", true},
		{"special DOK", "WAG", "DL1ABC", "", "20W17", true},
		{"DOK punctuation", "WAG", "DL1ABC", "", "A!1", false},
		{"DOK whitespace", "WAG", "DL1ABC", "", "A 01", false},
		{"German serial rejected", "WAG", "DL1ABC", "001", "", false},
		{"foreign WAG serial", "WAG", "W4GNS", "001", "", true},
		{"unknown country", "WAG", "123", "001", "", false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			err := validateRegionalExchange(tt.event, tt.call, tt.serial, tt.text)
			if (err == nil) != tt.valid {
				t.Fatalf("valid = %v, err = %v", tt.valid, err)
			}
		})
	}
}

// Exercise real catalog definitions and persisted rows through the submission
// boundary, including the sent-call snapshot/profile fallback used by the writer.
func TestExportRegionalExchanges(t *testing.T) {
	catalog, err := loadEventCatalog()
	if err != nil {
		t.Fatal(err)
	}
	for _, tt := range []struct{ event, call, code string }{
		{"HELVETIA", "HB9ABC", "ZH"},
		{"RDXC", "UA3ABC", "MO"},
		{"WAG", "DL1ABC", "A01"},
	} {
		for _, domesticSent := range []bool{false, true} {
			name := tt.event + "/received"
			if domesticSent {
				name = tt.event + "/sent"
			}
			t.Run(name, func(t *testing.T) {
				st, err := openStore(t.TempDir() + "/logger.db")
				if err != nil {
					t.Fatal(err)
				}
				defer st.Close()
				profile, err := st.activeStationProfile()
				if err != nil {
					t.Fatal(err)
				}
				profile.Callsign = "W4GNS"
				var event eventDefinition
				for _, e := range catalog {
					if e.ID == tt.event {
						event = e
						break
					}
				}
				if event.ID == "" {
					t.Fatal("missing catalog event")
				}
				q := validTestQSO()
				q.rstSent, q.rstRcvd = "599", "599"
				q.profileID, q.contestID = profile.ID, tt.event
				q.call, q.stationCallsign = tt.call, ""
				q.stx, q.stxString, q.srx, q.srxString = "001", "", "", tt.code
				if domesticSent {
					q.stationCallsign, q.call = tt.call, "W1AW"
					q.stx, q.stxString, q.srx, q.srxString = "", tt.code, "002", ""
				}
				id, err := st.insertQSO(q)
				if err != nil {
					t.Fatal(err)
				}
				var out bytes.Buffer
				count, _, err := exportCabrillo(context.Background(), &out, profile, event, tt.event, st)
				if err != nil || count != 1 {
					t.Fatalf("count = %d, err = %v", count, err)
				}
				if !strings.Contains(out.String(), "QSO:") || !strings.Contains(out.String(), tt.code) {
					t.Fatalf("missing exchange in export: %s", out.String())
				}
				// A stored invalid token must fail submission, not prevent local editing.
				field := "srx_string"
				if domesticSent {
					field = "stx_string"
				}
				if _, err := st.db.Exec("UPDATE qso SET "+field+" = ? WHERE id = ?", "!INVALID!", id); err != nil {
					t.Fatal(err)
				}
				out.Reset()
				if _, _, err := exportCabrillo(context.Background(), &out, profile, event, tt.event, st); err == nil {
					t.Fatal("export accepted invalid persisted regional exchange")
				}
			})
		}
	}
}
