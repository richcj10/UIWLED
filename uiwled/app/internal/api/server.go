// Package api hosts the built-in web UI and the REST endpoints that both the
// browser UI and the Home Assistant custom integration talk to.
//
// URL surface (all JSON, verbs mostly GET for simplicity):
//
//	GET    /                        embedded web UI
//	GET    /healthz                 liveness probe
//	GET    /api/switches            list all switches + state
//	GET    /api/state/{name}        one switch's state
//	GET    /api/color/{name}        ?slot=&r=&g=&b=&brightness=
//	GET    /api/brightness/{name}   ?value=0-255
//	GET    /api/effect/{name}       ?id=&speed=&intensity=
//	GET    /api/effects             list of {id,name}
//	GET    /api/power/{name}        ?on=0|1
//	POST   /api/identify/{name}     ?secs=5   (temporary flash pattern)
//	GET    /api/port/{name}         ?port=&r=&g=&b=&duration_ms=  set per-jack override
//	DELETE /api/port/{name}         ?port=                        clear one jack override
//	DELETE /api/ports/{name}                                      clear all overrides
//
// The API is intentionally read-tolerant: unknown/malformed values are clamped
// or ignored rather than returning 400s, so a slider drag storm never fails.
package api

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"sort"
	"time"

	"github.com/richcj10/uiwled/internal/device"
	"github.com/richcj10/uiwled/internal/engine"
	"github.com/richcj10/uiwled/internal/switches"
)

// Server bundles the HTTP mux with the engine and switch manager it drives.
type Server struct {
	port int
	mgr  *switches.Manager
	eng  *engine.Engine
	mux  *http.ServeMux
	log  *slog.Logger
}

// NewServer wires routes and returns a ready-to-Run server. The port is only
// bound in Run, not here.
func NewServer(port int, mgr *switches.Manager, eng *engine.Engine) *Server {
	s := &Server{
		port: port,
		mgr:  mgr,
		eng:  eng,
		mux:  http.NewServeMux(),
		log:  slog.Default().With("component", "api"),
	}
	s.routes()
	return s
}

func (s *Server) routes() {
	s.mux.HandleFunc("/api/switches", s.handleSwitches)
	s.mux.HandleFunc("/api/state/", s.handleState)     // /api/state/{name}
	s.mux.HandleFunc("/api/color/", s.handleColor)     // /api/color/{name}?r=&g=&b=&brightness=&slot=0
	s.mux.HandleFunc("/api/effect/", s.handleEffect)   // /api/effect/{name}?id=N&speed=X&intensity=Y
	s.mux.HandleFunc("/api/brightness/", s.handleBrightness) // /api/brightness/{name}?value=0-255
	s.mux.HandleFunc("/api/power/", s.handlePower)     // /api/power/{name}?on=1
	s.mux.HandleFunc("/api/effects", s.handleEffects)  // list
	s.mux.HandleFunc("/api/identify/", s.handleIdent)  // /api/identify/{name}?secs=5
	s.mux.HandleFunc("/api/port/", s.handlePortOverride) // /api/port/{name}?port=N&r=&g=&b=&duration_ms=
	s.mux.HandleFunc("/api/ports/", s.handleClearAll)    // /api/ports/{name} DELETE
	s.mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	s.mux.HandleFunc("/", s.handleIndex)
}

// Run binds :port and serves until ctx is cancelled. Returns nil on clean
// shutdown, or the underlying error from ListenAndServe.
func (s *Server) Run(ctx context.Context) error {
	srv := &http.Server{
		Addr:              fmt.Sprintf(":%d", s.port),
		Handler:           s.mux,
		ReadHeaderTimeout: 5 * time.Second,
	}
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdownCtx)
	}()
	s.log.Info("http listening", "port", s.port)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return err
	}
	return nil
}

func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(indexHTML))
}

// switchDTO is the JSON shape returned by /api/switches. Fields marked
// omitempty are absent until first successful device.Info discovery.
type switchDTO struct {
	Name     string               `json:"name"`
	Host     string               `json:"host"`
	Model    string               `json:"model,omitempty"`
	Hostname string               `json:"hostname,omitempty"`
	Layout   [][]int              `json:"layout,omitempty"`
	State    *engine.SegmentState `json:"state,omitempty"`
	Online   bool                 `json:"online"`
}

func (s *Server) handleSwitches(w http.ResponseWriter, r *http.Request) {
	out := make([]switchDTO, 0, len(s.mgr.Switches()))
	for _, sw := range s.mgr.Switches() {
		d := switchDTO{Name: sw.Name, Host: sw.Host, State: s.eng.State(sw.Name)}
		if info := sw.Info(); info != nil {
			d.Model = info.Model
			d.Hostname = info.Hostname
			d.Layout = info.Layout
			d.Online = true
		}
		out = append(out, d)
	}
	// Stable ordering so the UI doesn't reshuffle cards between polls.
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	writeJSON(w, out)
}

// handlePortOverride pins a jack to a color regardless of the running effect.
// POST/GET /api/port/{name}?port=N&r=&g=&b=&duration_ms=X    -> set override
// DELETE   /api/port/{name}?port=N                             -> clear one jack
func (s *Server) handlePortOverride(w http.ResponseWriter, r *http.Request) {
	name := r.URL.Path[len("/api/port/"):]
	if s.mgr.Find(name) == nil {
		http.Error(w, "no such switch", http.StatusNotFound)
		return
	}
	q := r.URL.Query()
	portStr := q.Get("port")
	if portStr == "" {
		http.Error(w, "port required", http.StatusBadRequest)
		return
	}
	port := atoiClamped(portStr, 1, 1024)
	if r.Method == http.MethodDelete {
		s.eng.ClearPortOverride(name, port)
		writeJSON(w, map[string]any{"ok": true, "cleared": port})
		return
	}
	rV := atoiClamped(q.Get("r"), 0, 255)
	gV := atoiClamped(q.Get("g"), 0, 255)
	bV := atoiClamped(q.Get("b"), 0, 255)
	durationMs := atoiClamped(q.Get("duration_ms"), 0, 24*3600*1000)
	s.eng.SetPortOverride(name, port, device.Color{R: uint8(rV), G: uint8(gV), B: uint8(bV)}, durationMs)
	writeJSON(w, map[string]any{"ok": true, "port": port, "duration_ms": durationMs})
}

// handleClearAll drops every per-jack override for a switch.
// DELETE /api/ports/{name}
func (s *Server) handleClearAll(w http.ResponseWriter, r *http.Request) {
	name := r.URL.Path[len("/api/ports/"):]
	if s.mgr.Find(name) == nil {
		http.Error(w, "no such switch", http.StatusNotFound)
		return
	}
	s.eng.ClearAllPortOverrides(name)
	writeJSON(w, map[string]any{"ok": true})
}

func (s *Server) handleIdent(w http.ResponseWriter, r *http.Request) {
	name := r.URL.Path[len("/api/identify/"):]
	if s.mgr.Find(name) == nil {
		http.Error(w, "no such switch", http.StatusNotFound)
		return
	}
	secs := atoiClamped(r.URL.Query().Get("secs"), 1, 60)
	if r.URL.Query().Get("secs") == "" {
		secs = 5
	}
	s.eng.Identify(name, time.Duration(secs)*time.Second)
	writeJSON(w, map[string]any{"ok": true, "seconds": secs})
}

// handleState returns the current engine state for one switch.
func (s *Server) handleState(w http.ResponseWriter, r *http.Request) {
	name := r.URL.Path[len("/api/state/"):]
	sw := s.mgr.Find(name)
	if sw == nil {
		http.Error(w, "no such switch", http.StatusNotFound)
		return
	}
	writeJSON(w, s.eng.State(name))
}

// handleColor writes a color into one of the three palette slots and
// optionally sets the master brightness. Turning the light on is implicit —
// receiving a color from the UI counts as an on-intent.
func (s *Server) handleColor(w http.ResponseWriter, r *http.Request) {
	name := r.URL.Path[len("/api/color/"):]
	sw := s.mgr.Find(name)
	if sw == nil {
		http.Error(w, "no such switch", http.StatusNotFound)
		return
	}
	q := r.URL.Query()
	rV := atoiClamped(q.Get("r"), 0, 255)
	gV := atoiClamped(q.Get("g"), 0, 255)
	bV := atoiClamped(q.Get("b"), 0, 255)
	brightness := atoiClamped(q.Get("brightness"), 0, 255)
	if q.Get("brightness") == "" {
		brightness = -1
	}
	slot := atoiClamped(q.Get("slot"), 0, 2)
	s.eng.SetState(name, func(st *engine.SegmentState) {
		st.On = true
		st.Colors[slot] = device.Color{R: uint8(rV), G: uint8(gV), B: uint8(bV)}
		if brightness >= 0 {
			st.Brightness = uint8(brightness)
		}
	})
	writeJSON(w, s.eng.State(name))
}

// handleEffect updates any combination of {id, speed, intensity}. Absent
// query params are left unchanged so callers can PATCH one field at a time.
func (s *Server) handleEffect(w http.ResponseWriter, r *http.Request) {
	name := r.URL.Path[len("/api/effect/"):]
	if s.mgr.Find(name) == nil {
		http.Error(w, "no such switch", http.StatusNotFound)
		return
	}
	q := r.URL.Query()
	s.eng.SetState(name, func(st *engine.SegmentState) {
		if q.Get("id") != "" {
			st.EffectID = atoiClamped(q.Get("id"), 0, 200)
		}
		if q.Get("speed") != "" {
			st.Speed = uint8(atoiClamped(q.Get("speed"), 0, 255))
		}
		if q.Get("intensity") != "" {
			st.Intensity = uint8(atoiClamped(q.Get("intensity"), 0, 255))
		}
	})
	writeJSON(w, s.eng.State(name))
}

// handleBrightness updates the master brightness (0-255).
func (s *Server) handleBrightness(w http.ResponseWriter, r *http.Request) {
	name := r.URL.Path[len("/api/brightness/"):]
	if s.mgr.Find(name) == nil {
		http.Error(w, "no such switch", http.StatusNotFound)
		return
	}
	v := atoiClamped(r.URL.Query().Get("value"), 0, 255)
	s.eng.SetState(name, func(st *engine.SegmentState) { st.Brightness = uint8(v) })
	writeJSON(w, s.eng.State(name))
}

// handlePower turns the switch's LEDs on or off. Off blacks out the whole
// strip without clearing state — the last effect resumes on power-on.
func (s *Server) handlePower(w http.ResponseWriter, r *http.Request) {
	name := r.URL.Path[len("/api/power/"):]
	if s.mgr.Find(name) == nil {
		http.Error(w, "no such switch", http.StatusNotFound)
		return
	}
	on := r.URL.Query().Get("on") == "1"
	s.eng.SetState(name, func(st *engine.SegmentState) { st.On = on })
	writeJSON(w, s.eng.State(name))
}

// handleEffects lists all registered effects as {id, name} pairs.
func (s *Server) handleEffects(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, engine.Effects())
}

// writeJSON is the one-liner encoder used by every handler. Errors from
// Encode are dropped intentionally — nothing sane to do with them mid-response.
func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

// atoiClamped parses a decimal query-string value and clamps to [lo, hi].
// Empty or malformed input returns lo. Used to make the API forgiving of
// slider-drag flooding without doing a strconv.Atoi + validation dance.
func atoiClamped(s string, lo, hi int) int {
	n := 0
	for _, ch := range s {
		if ch < '0' || ch > '9' {
			return lo
		}
		n = n*10 + int(ch-'0')
		if n > hi {
			return hi
		}
	}
	if n < lo {
		return lo
	}
	return n
}
