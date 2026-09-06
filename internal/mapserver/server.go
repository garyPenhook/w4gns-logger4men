// Package mapserver serves the optional loopback-only companion map.
package mapserver

import (
	"crypto/rand"
	"embed"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/fs"
	"net"
	"net/http"
	"sync"
	"time"

	"w4gns-logger/internal/geo"
	"w4gns-logger/internal/mapfeed"
	"w4gns-logger/internal/spot"
)

//go:embed web/*
var assets embed.FS

type State struct {
	Status   string
	Callsign string
	Home     *geo.Location
	// FilterKey changes whenever the captured, immutable filter changes.
	FilterKey string
	Allows    func(spot.Report) bool `json:"-"`
}

type Server struct {
	mu      sync.Mutex
	store   *mapfeed.Store
	state   State
	server  *http.Server
	address string
	session string
	launch  map[string]bool
	closed  bool
}

func New(store *mapfeed.Store) *Server { return &Server{store: store, launch: make(map[string]bool)} }
func (s *Server) SetState(state State) { s.mu.Lock(); s.state = state; s.mu.Unlock() }

func token() string {
	b := make([]byte, 32)
	// crypto/rand.Read terminates on unrecoverable entropy failure.
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// OpenURL starts at most one server and creates a one-use browser launch URL.
func (s *Server) OpenURL() (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return "", fmt.Errorf("world map is closed")
	}
	if s.server == nil {
		listener, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			return "", err
		}
		s.address = listener.Addr().String()
		s.session = token()
		s.server = &http.Server{Handler: http.HandlerFunc(s.serve), ReadHeaderTimeout: 5 * time.Second, IdleTimeout: 30 * time.Second}
		go s.server.Serve(listener)
	}
	if len(s.launch) > 32 {
		clear(s.launch)
	}
	t := token()
	s.launch[t] = true
	return "http://" + s.address + "/launch?token=" + t, nil
}

func (s *Server) Close() error {
	s.mu.Lock()
	s.closed = true
	srv := s.server
	s.mu.Unlock()
	if srv != nil {
		return srv.Close()
	}
	return nil
}

func (s *Server) serve(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self'; img-src 'self'; connect-src 'self'; frame-ancestors 'none'; base-uri 'none'; form-action 'none'")
	s.mu.Lock()
	address, session := s.address, s.session
	s.mu.Unlock()
	if r.Host != address || (r.Header.Get("Origin") != "" && r.Header.Get("Origin") != "http://"+address) || r.Header.Get("Sec-Fetch-Site") == "cross-site" {
		http.Error(w, "Local map access only", http.StatusForbidden)
		return
	}
	if r.Method != "GET" {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if r.URL.Path == "/launch" {
		t := r.URL.Query().Get("token")
		s.mu.Lock()
		valid := s.launch[t]
		delete(s.launch, t)
		s.mu.Unlock()
		if !valid {
			http.Error(w, "Open the map again with Ctrl+L in the logger", http.StatusUnauthorized)
			return
		}
		http.SetCookie(w, &http.Cookie{Name: "w4gns_map_" + session[:12], Value: session, Path: "/", HttpOnly: true, SameSite: http.SameSiteStrictMode})
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	cookie, err := r.Cookie("w4gns_map_" + session[:12])
	if err != nil || cookie.Value != session {
		http.Error(w, "Open the map with Ctrl+L in the logger", http.StatusUnauthorized)
		return
	}
	if r.URL.Path == "/events" {
		s.events(w, r)
		return
	}
	root, _ := fs.Sub(assets, "web")
	http.FileServer(http.FS(root)).ServeHTTP(w, r)
}

type report struct {
	spot.Report
	MatchesLogger bool
}
type packet struct {
	Reset      bool
	Session    string
	Now        time.Time
	OldestID   int64
	AtCapacity bool
	State      State
	Reports    []report
}

// Each connection starts with a complete snapshot, then receives deltas from
// that cursor. A slow/reconnecting client is reset from retained state; no
// per-browser queue can block the cluster producer or grow without bound.
func (s *Server) events(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/event-stream")
	controller := http.NewResponseController(w)
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	var cursor uint64
	var filterKey string
	first := true
	for {
		now := time.Now().UTC()
		s.store.Expire(now.Add(-time.Hour))
		s.mu.Lock()
		state := s.state
		s.mu.Unlock()
		reset := first || state.FilterKey != filterKey
		if reset {
			cursor = 0
		}
		changes := s.store.ChangesSince(cursor)
		p := packet{Reset: reset, Session: s.store.SessionID(), Now: now, OldestID: changes.OldestID, AtCapacity: changes.AtCapacity, State: state, Reports: []report{}}
		for _, v := range changes.Reports {
			allowed := state.Allows == nil || state.Allows(v)
			p.Reports = append(p.Reports, report{v, allowed})
		}
		cursor = changes.Revision
		data, err := json.Marshal(p)
		if err != nil {
			return
		}
		_ = controller.SetWriteDeadline(time.Now().Add(5 * time.Second))
		if _, err = fmt.Fprintf(w, "data: %s\n\n", data); err != nil {
			return
		}
		if err = controller.Flush(); err != nil {
			return
		}
		first = false
		filterKey = state.FilterKey
		select {
		case <-r.Context().Done():
			return
		case <-ticker.C:
		}
	}
}
