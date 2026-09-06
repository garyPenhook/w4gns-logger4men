package mapserver

import (
	"bufio"
	"encoding/json"
	"net/http"
	"net/http/cookiejar"
	"strings"
	"testing"
	"time"

	"w4gns-logger/internal/mapfeed"
	"w4gns-logger/internal/spot"
)

func browser(t *testing.T, s *Server) (*http.Client, string) {
	t.Helper()
	u, err := s.OpenURL()
	if err != nil {
		t.Fatal(err)
	}
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar, Timeout: 8 * time.Second}
	res, err := client.Get(u)
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != 200 {
		t.Fatalf("launch: %d", res.StatusCode)
	}
	return client, strings.Split(u, "/launch")[0]
}

func TestLocalAccessAndLaunchLifecycle(t *testing.T) {
	s := New(mapfeed.NewStore(20))
	defer s.Close()
	client, base := browser(t, s)
	for _, tc := range []struct {
		name, path, origin, host string
		auth                     bool
		want                     int
	}{
		{"asset", "/map.js", "", "", true, 200},
		{"anonymous", "/events", "", "", false, 401},
		{"foreign origin", "/events", "https://example.org", "", true, 403},
		{"foreign host", "/events", "", "example.org", true, 403},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req, _ := http.NewRequest("GET", base+tc.path, nil)
			req.Header.Set("Origin", tc.origin)
			if tc.host != "" {
				req.Host = tc.host
			}
			c := client
			if !tc.auth {
				c = &http.Client{Timeout: time.Second}
			}
			res, err := c.Do(req)
			if err != nil {
				t.Fatal(err)
			}
			defer res.Body.Close()
			if res.StatusCode != tc.want {
				t.Fatalf("got %d want %d", res.StatusCode, tc.want)
			}
			if res.Header.Get("Content-Security-Policy") == "" {
				t.Fatal("missing CSP")
			}
		})
	}
	u, _ := s.OpenURL()
	res, err := client.Get(u)
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	res, err = client.Get(u)
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != 401 {
		t.Fatal("launch token reused")
	}
	s.Close()
	if _, err = s.OpenURL(); err == nil {
		t.Fatal("reopened after close")
	}
}

func nextPacket(t *testing.T, reader *bufio.Scanner) packet {
	t.Helper()
	for reader.Scan() {
		line := reader.Text()
		if strings.HasPrefix(line, "data: ") {
			var p packet
			if err := json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &p); err != nil {
				t.Fatal(err)
			}
			return p
		}
	}
	t.Fatalf("stream ended: %v", reader.Err())
	return packet{}
}

func TestSnapshotDeltaFilterResetAndExpiry(t *testing.T) {
	store := mapfeed.NewStore(20)
	store.Add(spot.Report{DXCall: "EXPIRED", ReceivedAtUTC: time.Now().Add(-2 * time.Hour)})
	first := store.Add(spot.Report{DXCall: "W1AW", SpotterCall: "G3AAA", ReceivedAtUTC: time.Now()})
	s := New(store)
	defer s.Close()
	s.SetState(State{FilterKey: "one", Allows: func(r spot.Report) bool { return r.DXCall == "W1AW" }})
	client, base := browser(t, s)
	res, err := client.Get(base + "/events")
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	reader := bufio.NewScanner(res.Body)
	reader.Buffer(make([]byte, 4096), 16<<20)
	p := nextPacket(t, reader)
	if !p.Reset || len(p.Reports) != 1 || p.Reports[0].EventID != first || !p.Reports[0].MatchesLogger {
		t.Fatalf("snapshot: %+v", p)
	}
	id := store.Add(spot.Report{DXCall: "JA1AAA", ReceivedAtUTC: time.Now()})
	p = nextPacket(t, reader)
	if p.Reset || len(p.Reports) != 1 || p.Reports[0].EventID != id || p.Reports[0].MatchesLogger {
		t.Fatalf("delta: %+v", p)
	}
	s.SetState(State{FilterKey: "two"})
	p = nextPacket(t, reader)
	if !p.Reset || len(p.Reports) != 2 || !p.Reports[1].MatchesLogger {
		t.Fatalf("filter reset: %+v", p)
	}
	store.Expire(time.Now().Add(time.Second))
	p = nextPacket(t, reader)
	if len(p.Reports) != 0 || store.Len() != 0 {
		t.Fatal("expiry failed")
	}
	res.Body.Close()
	res, err = client.Get(base + "/events")
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	reader = bufio.NewScanner(res.Body)
	if p = nextPacket(t, reader); !p.Reset || len(p.Reports) != 0 {
		t.Fatal("reconnection must replace snapshot")
	}
}
