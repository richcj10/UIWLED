// Package engine computes LED frames and pushes them to switches at the
// configured tick rate. Approach A: every frame is computed on HA and shipped
// over SSH. See README for the tradeoffs and the path to a switch-side agent.
package engine

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/richcj10/uiwled/internal/device"
	"github.com/richcj10/uiwled/internal/switches"
)

// SegmentState is the per-switch runtime state — the full "what should this
// switch look like right now" snapshot. Effect functions read it every tick;
// the API writes it in response to user actions; persist saves/restores it
// across restarts.
//
// Field names use exported PascalCase so they JSON-marshal cleanly and can
// be persisted round-tripped.
type SegmentState struct {
	EffectID   int             // effect id, one of engine.Fx* constants
	Speed      uint8           // 0-255 — most effects treat higher = faster
	Intensity  uint8           // 0-255 — effect-specific (density, tail length, etc.)
	Palette    int             // reserved for future WLED palette compat
	Colors     [3]device.Color // three "slots" that effects sample from
	Brightness uint8           // 0-255 master brightness applied to effect output
	On         bool            // when false the switch shows all-black frames
}

// Engine is the animation runtime — it holds per-switch SegmentState,
// per-jack overrides and identify timers, and ticks at fps to render and
// push frames via switches.Manager.
type Engine struct {
	mgr *switches.Manager
	fps int

	mu        sync.Mutex
	states    map[string]*SegmentState        // switch name -> state
	identifyU map[string]time.Time            // switch name -> identify-until timestamp
	overrides map[string]map[int]portOverride // switch name -> port -> override
	log       *slog.Logger
	persist   *persister
}

// portOverride pins a single jack to a color regardless of the running effect.
// If ExpiresAt is zero, the override persists until explicitly cleared.
type portOverride struct {
	Color     device.Color
	ExpiresAt time.Time
}

// New constructs an Engine and immediately loads any persisted state from
// /data/state.json. Call Start to begin ticking.
func New(mgr *switches.Manager, fps int) *Engine {
	e := &Engine{
		mgr:       mgr,
		fps:       fps,
		states:    make(map[string]*SegmentState),
		identifyU: make(map[string]time.Time),
		overrides: make(map[string]map[int]portOverride),
		log:       slog.Default().With("component", "engine"),
	}
	e.persist = newPersister(e, "")
	e.persist.load()
	return e
}

// SetPortOverride pins the given jack to a color regardless of the running
// effect. If durationMs > 0, the override auto-clears after that many ms.
func (e *Engine) SetPortOverride(name string, port int, c device.Color, durationMs int) {
	e.mu.Lock()
	m, ok := e.overrides[name]
	if !ok {
		m = make(map[int]portOverride)
		e.overrides[name] = m
	}
	ov := portOverride{Color: c}
	if durationMs > 0 {
		ov.ExpiresAt = time.Now().Add(time.Duration(durationMs) * time.Millisecond)
	}
	m[port] = ov
	e.mu.Unlock()
}

// ClearPortOverride removes the override for a specific jack. No-op if unset.
func (e *Engine) ClearPortOverride(name string, port int) {
	e.mu.Lock()
	if m, ok := e.overrides[name]; ok {
		delete(m, port)
	}
	e.mu.Unlock()
}

// ClearAllPortOverrides removes every jack override for a switch.
func (e *Engine) ClearAllPortOverrides(name string) {
	e.mu.Lock()
	delete(e.overrides, name)
	e.mu.Unlock()
}

// Overrides returns a snapshot of the current overrides for a switch, useful
// for the API to expose current state.
func (e *Engine) Overrides(name string) map[int]device.Color {
	e.mu.Lock()
	defer e.mu.Unlock()
	m, ok := e.overrides[name]
	if !ok {
		return nil
	}
	out := make(map[int]device.Color, len(m))
	now := time.Now()
	for port, ov := range m {
		if !ov.ExpiresAt.IsZero() && now.After(ov.ExpiresAt) {
			delete(m, port)
			continue
		}
		out[port] = ov.Color
	}
	return out
}

// Identify triggers a distinctive attention-grabbing pattern on the named
// switch for `duration`, then normal state resumes.
func (e *Engine) Identify(name string, duration time.Duration) {
	e.mu.Lock()
	e.identifyU[name] = time.Now().Add(duration)
	e.mu.Unlock()
}

// State returns (or lazily creates) the SegmentState for a switch. The
// returned pointer is the live state — read-only for callers.
func (e *Engine) State(name string) *SegmentState {
	e.mu.Lock()
	defer e.mu.Unlock()
	s, ok := e.states[name]
	if !ok {
		s = &SegmentState{
			On:         true,
			Brightness: 128,
			Speed:      128,
			Intensity:  128,
			Colors: [3]device.Color{
				{R: 255, G: 160, B: 0},  // amber
				{R: 0, G: 60, B: 255},   // blue (used by wipe/fade)
				{R: 0, G: 255, B: 60},   // green (spare)
			},
		}
		e.states[name] = s
	}
	return s
}

// SetState atomically mutates the state for a switch via the supplied
// callback and schedules a persistence flush. Prefer this over reading
// State() and writing fields directly so persistence stays consistent.
func (e *Engine) SetState(name string, mutate func(*SegmentState)) {
	e.mu.Lock()
	s, ok := e.states[name]
	if !ok {
		s = &SegmentState{On: true, Brightness: 128}
		e.states[name] = s
	}
	mutate(s)
	e.mu.Unlock()
	if e.persist != nil {
		e.persist.schedule()
	}
}

// Start launches the ticker goroutine. Cancels when ctx does.
func (e *Engine) Start(ctx context.Context) {
	go e.run(ctx)
}

// run is the ticker loop. Each tick renders a frame for every configured
// switch and hands it off to switches.Switch.PushFrame (which does the diff
// and SSH write). Effects see `elapsed` measured from Start(), so animations
// stay phase-coherent across switches.
func (e *Engine) run(ctx context.Context) {
	if e.fps <= 0 {
		e.fps = 15
	}
	tick := time.NewTicker(time.Second / time.Duration(e.fps))
	defer tick.Stop()

	tStart := time.Now()
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-tick.C:
			elapsed := now.Sub(tStart)
			for _, sw := range e.mgr.Switches() {
				info := sw.Info()
				if info == nil || info.Layout == nil {
					continue
				}
				var frame []device.Color
				e.mu.Lock()
				until, identifying := e.identifyU[sw.Name]
				if identifying && now.Before(until) {
					e.mu.Unlock()
					frame = renderIdentify(info, elapsed)
				} else {
					if identifying {
						delete(e.identifyU, sw.Name)
					}
					state := e.states[sw.Name]
					e.mu.Unlock()
					if state == nil {
						state = e.State(sw.Name)
					}
					frame = renderFrame(info, state, elapsed)
				}
				// Apply per-jack overrides on top of the effect frame. Identify
				// mode intentionally ignores overrides so the flash pattern is
				// unambiguous.
				if !identifying {
					e.applyOverrides(sw.Name, frame, now)
				}
				if err := sw.PushFrame(frame); err != nil {
					e.log.Warn("push frame failed", "switch", sw.Name, "err", err)
				}
			}
		}
	}
}

// portCount returns the number of jacks known for this layout.
func portCount(layout [][]int) int {
	max := 0
	for _, row := range layout {
		for _, p := range row {
			if p > max {
				max = p
			}
		}
	}
	return max
}

// renderFrame produces a 1-indexed color buffer sized to fit the layout's
// highest port index. Index 0 is unused (ports are 1-based on the switch).
func renderFrame(info *device.Info, s *SegmentState, elapsed time.Duration) []device.Color {
	n := portCount(info.Layout)
	frame := make([]device.Color, n+1)
	if !s.On {
		return frame
	}
	pixels := renderEffect(s, elapsed, n)
	for i := 0; i < n && i < len(pixels); i++ {
		frame[i+1] = pixels[i]
	}
	return frame
}

// applyOverrides paints per-jack overrides on top of a rendered effect frame.
// Expired overrides are pruned as we go.
func (e *Engine) applyOverrides(name string, frame []device.Color, now time.Time) {
	e.mu.Lock()
	defer e.mu.Unlock()
	m, ok := e.overrides[name]
	if !ok || len(m) == 0 {
		return
	}
	for port, ov := range m {
		if !ov.ExpiresAt.IsZero() && now.After(ov.ExpiresAt) {
			delete(m, port)
			continue
		}
		if port >= 1 && port < len(frame) {
			frame[port] = ov.Color
		}
	}
	if len(m) == 0 {
		delete(e.overrides, name)
	}
}

// renderIdentify draws a bright cyan chase across all jacks. Impossible to
// miss visually — used to physically locate a specific switch in a rack.
func renderIdentify(info *device.Info, elapsed time.Duration) []device.Color {
	n := portCount(info.Layout)
	frame := make([]device.Color, n+1)
	head := int(elapsed.Milliseconds()/250) % (n + 4)
	for i := 1; i <= n; i++ {
		if i == head || i == head-1 || i == head-2 {
			frame[i] = device.Color{R: 0, G: 255, B: 255}
		} else {
			frame[i] = device.Color{R: 0, G: 20, B: 20}
		}
	}
	return frame
}
