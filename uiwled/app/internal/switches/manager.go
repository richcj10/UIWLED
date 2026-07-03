// Package switches owns the lifecycle of every configured switch: SSH
// connection, info discovery, reconnect on drop, and a per-switch mailbox
// for the engine to push frames into.
package switches

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/richcj10/uiwled/internal/config"
	"github.com/richcj10/uiwled/internal/device"
)

// Switch is the runtime representation of one configured UniFi switch —
// its identity, current SSH client, cached device info, and the last frame
// we pushed (used by the diff path). All fields except Name and Host are
// mutated by supervise() from the manager goroutine and guarded by mu.
type Switch struct {
	Name string
	Host string

	mu           sync.Mutex
	client       *device.Client
	info         *device.Info
	lastSent     []device.Color // 1-indexed; index 0 unused (ports are 1-based)
	lastFullPush time.Time
	cfg          device.Config
	log          *slog.Logger
}

// reassertInterval controls how often we re-send the full frame even when no
// jack has changed. Some switch firmware and the UniFi controller periodically
// reset LED state; a cheap full re-push snaps our colors back.
const reassertInterval = 2 * time.Second

// Info returns the last discovered device info (model, hostname, layout, etc.)
// or nil if the switch hasn't finished its first connect yet.
func (s *Switch) Info() *device.Info {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.info
}

// Client returns the active SSH client for this switch, or nil if disconnected.
func (s *Switch) Client() *device.Client {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.client
}

// PushFrame sends only the changed jacks vs. the last frame, or the full frame
// if reassertInterval has elapsed since the last full push.
// `frame` is 1-indexed: frame[1] = port 1's target color, frame[0] ignored.
//
// Fast path: when every jack in the target frame is the same color, we use
// /proc/led/led_all_port_code (one shell write) instead of N*3 per-jack writes.
// For a 48-port switch that's 144x fewer writes per frame.
func (s *Switch) PushFrame(frame []device.Color) error {
	s.mu.Lock()
	client := s.client
	last := s.lastSent
	lastFull := s.lastFullPush
	s.mu.Unlock()
	if client == nil {
		return nil
	}

	forceFull := time.Since(lastFull) >= reassertInterval

	// Fast path: uniform frame across all jacks.
	if uniform, c := frameIsUniform(frame); uniform {
		// Only skip if the frame is identical to last-sent AND we're not forcing.
		if !forceFull && lastMatches(last, frame) {
			return nil
		}
		if err := client.SetAllPorts(c, 100); err != nil {
			return err
		}
		s.mu.Lock()
		s.lastSent = append([]device.Color(nil), frame...)
		s.lastFullPush = time.Now()
		s.mu.Unlock()
		return nil
	}

	// Slow path: per-jack diff.
	toSend := make([]device.PortColor, 0)
	for i := 1; i < len(frame); i++ {
		if forceFull || i >= len(last) || last[i] != frame[i] {
			toSend = append(toSend, device.PortColor{Index: i, Color: frame[i]})
		}
	}
	if len(toSend) == 0 {
		return nil
	}
	if err := client.SetPortColors(toSend); err != nil {
		return err
	}

	s.mu.Lock()
	s.lastSent = append([]device.Color(nil), frame...)
	if forceFull {
		s.lastFullPush = time.Now()
	}
	s.mu.Unlock()
	return nil
}

// frameIsUniform returns true and the shared color if every jack (index 1+)
// in the frame is the same color. Ports are 1-based; index 0 is ignored.
func frameIsUniform(frame []device.Color) (bool, device.Color) {
	if len(frame) < 2 {
		return false, device.Color{}
	}
	first := frame[1]
	for i := 2; i < len(frame); i++ {
		if frame[i] != first {
			return false, device.Color{}
		}
	}
	return true, first
}

// lastMatches reports whether the last-sent buffer equals the target frame.
func lastMatches(last, frame []device.Color) bool {
	if len(last) != len(frame) {
		return false
	}
	for i := 1; i < len(frame); i++ {
		if last[i] != frame[i] {
			return false
		}
	}
	return true
}

// Manager owns every Switch: creates one supervise-goroutine per configured
// switch, exposes lookups by name, and coordinates clean shutdown.
type Manager struct {
	switches []*Switch
	log      *slog.Logger
}

// NewManager constructs a Manager from parsed addon config. Nothing is dialed
// until Start is called.
func NewManager(cfgs []config.Switch) *Manager {
	log := slog.Default()
	m := &Manager{log: log}
	for _, c := range cfgs {
		s := &Switch{
			Name: c.Name,
			Host: c.Host,
			cfg: device.Config{
				Name:     c.Name,
				Addr:     c.Host,
				User:     c.User,
				Password: c.Password,
				KeyPEM:   c.SSHKey,
			},
			log: log.With("switch", c.Name),
		}
		m.switches = append(m.switches, s)
	}
	return m
}

// Switches returns the list in configured order. Callers should not modify it.
func (m *Manager) Switches() []*Switch { return m.switches }

// Find returns the Switch with the given configured name, or nil.
func (m *Manager) Find(name string) *Switch {
	for _, s := range m.switches {
		if s.Name == name {
			return s
		}
	}
	return nil
}

// Start launches one supervise-goroutine per switch. Returns immediately;
// each goroutine dials, discovers, keeps the SSH connection alive, and
// automatically reconnects on failure.
func (m *Manager) Start(ctx context.Context) error {
	for _, s := range m.switches {
		go m.supervise(ctx, s)
	}
	return nil
}

// Stop force-closes every open SSH connection. Safe to call after context
// cancellation; a no-op if never Started.
func (m *Manager) Stop() {
	for _, s := range m.switches {
		if c := s.Client(); c != nil {
			_ = c.Close()
		}
	}
}

// supervise is the per-switch main loop. It Dials, discovers info, uploads
// and starts the agent script, then blocks on either context cancellation
// or SSH connection death. On death it closes the client and loops back to
// redial (with exponential backoff on repeated Dial failures).
func (m *Manager) supervise(ctx context.Context, s *Switch) {
	backoff := 2 * time.Second
	for {
		if ctx.Err() != nil {
			return
		}
		c, err := device.Dial(s.cfg)
		if err != nil {
			s.log.Warn("ssh dial failed", "err", err, "retry_in", backoff)
			select {
			case <-time.After(backoff):
			case <-ctx.Done():
				return
			}
			if backoff < 30*time.Second {
				backoff *= 2
			}
			continue
		}
		backoff = 2 * time.Second

		info, err := c.Info()
		if err != nil {
			s.log.Warn("failed to fetch device info", "err", err)
			_ = c.Close()
			time.Sleep(2 * time.Second)
			continue
		}
		if err := c.InitLEDMode(); err != nil {
			s.log.Warn("failed to init LED mode", "err", err)
		}
		if err := c.StartFrameShell(); err != nil {
			s.log.Warn("failed to start frame shell (falling back to session-per-exec)", "err", err)
		}
		s.mu.Lock()
		s.client = c
		s.info = info
		s.lastSent = nil
		s.lastFullPush = time.Time{}
		s.mu.Unlock()
		s.log.Info("connected", "model", info.Model, "hostname", info.Hostname, "version", info.Version)

		// Watch for SSH connection death (peer reset, network drop, etc.)
		// so we can drop the corpse and redial instead of hammering error
		// messages on every frame push.
		sshDied := make(chan error, 1)
		go func() { sshDied <- c.SSHWait() }()

		select {
		case <-ctx.Done():
			_ = c.Close()
			return
		case sshErr := <-sshDied:
			s.log.Warn("ssh connection died, reconnecting", "err", sshErr)
			_ = c.Close()
			s.mu.Lock()
			s.client = nil
			s.lastSent = nil
			s.lastFullPush = time.Time{}
			s.mu.Unlock()
			// small pause so we don't slam the switch during transient issues
			select {
			case <-time.After(1 * time.Second):
			case <-ctx.Done():
				return
			}
			continue
		}
	}
}

