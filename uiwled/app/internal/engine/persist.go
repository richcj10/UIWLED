package engine

import (
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// State persistence: engine states are serialized to a JSON file in the HA
// addon's /data directory (which HA preserves across restarts). Writes are
// debounced — many rapid UI changes are coalesced into one disk write.

const (
	persistDefaultPath = "/data/state.json"
	persistDebounce    = 500 * time.Millisecond
)

type persistedState struct {
	Version  int                       `json:"version"`
	Switches map[string]*SegmentState `json:"switches"`
}

type persister struct {
	path    string
	engine  *Engine
	mu      sync.Mutex
	pending *time.Timer
	log     *slog.Logger
}

func newPersister(e *Engine, path string) *persister {
	if path == "" {
		path = persistDefaultPath
	}
	return &persister{
		path:   path,
		engine: e,
		log:    slog.Default().With("component", "persist"),
	}
}

// load populates the engine's states from disk. Missing/unreadable file is not
// an error — first-run just starts with defaults.
func (p *persister) load() {
	b, err := os.ReadFile(p.path)
	if err != nil {
		if !os.IsNotExist(err) {
			p.log.Warn("failed to read state file", "path", p.path, "err", err)
		}
		return
	}
	var ps persistedState
	if err := json.Unmarshal(b, &ps); err != nil {
		p.log.Warn("failed to parse state file", "path", p.path, "err", err)
		return
	}
	p.engine.mu.Lock()
	for name, st := range ps.Switches {
		if st == nil {
			continue
		}
		p.engine.states[name] = st
	}
	p.engine.mu.Unlock()
	p.log.Info("state restored", "path", p.path, "switches", len(ps.Switches))
}

// schedule debounces disk writes so a slider drag doesn't fsync 30 times/sec.
func (p *persister) schedule() {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.pending != nil {
		p.pending.Stop()
	}
	p.pending = time.AfterFunc(persistDebounce, p.flush)
}

func (p *persister) flush() {
	p.engine.mu.Lock()
	snap := make(map[string]*SegmentState, len(p.engine.states))
	for k, v := range p.engine.states {
		cp := *v
		snap[k] = &cp
	}
	p.engine.mu.Unlock()

	ps := persistedState{Version: 1, Switches: snap}
	b, err := json.MarshalIndent(&ps, "", "  ")
	if err != nil {
		p.log.Warn("marshal state failed", "err", err)
		return
	}
	tmp := p.path + ".tmp"
	if err := os.MkdirAll(filepath.Dir(p.path), 0o755); err != nil {
		p.log.Warn("mkdir failed", "err", err)
		return
	}
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		p.log.Warn("write state failed", "err", err)
		return
	}
	if err := os.Rename(tmp, p.path); err != nil {
		p.log.Warn("rename state failed", "err", err)
		return
	}
}
