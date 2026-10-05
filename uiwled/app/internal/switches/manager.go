// Package switches owns the lifecycle of every configured switch: SSH
// connection, info discovery, reconnect on drop, and a per-switch mailbox
// for the engine to push frames into.
package switches

import (
	"context"
	"log/slog"
	"sort"
	"sync"
	"time"

	"github.com/richcj10/uiwled/internal/config"
	"github.com/richcj10/uiwled/internal/device"
)

// Switch is the runtime representation of one configured UniFi switch —
// its identity, current SSH client, cached device info, and what we believe
// the LEDs currently show (used by the diff path). All fields except Name
// and Host are guarded by mu.
type Switch struct {
	Name string
	Host string

	mu         sync.Mutex
	client     *device.Client
	info       *device.Info
	hw         []device.Color // what the LEDs show, 1-indexed; nil = unknown
	lastAll    time.Time      // last led_all_port_color write
	reassertAt int            // round-robin cursor for the trickle reassert
	behavior   int            // wanted led_behavior; -1 = unknown
	breatheOn  bool           // controller confirmed breathing (start sequence sent)
	cfg        device.Config
	log        *slog.Logger
}

// Write-cost model, measured on a USW-Pro-Max-16-PoE (MIPS 34Kc, fw 7.5.15):
// one led_color write (one channel of one port) costs ~1.9 ms, mostly kernel
// time talking to the LED controller, and one led_all_port_color write costs
// ~14 ms. The driver, not the shell, is the bottleneck, so PushFrame's job is
// to send as few writes as possible.
const (
	channelWriteCost = 1900 * time.Microsecond
	allPortCost      = 7 // an all-ports write, in channel-write units

	// budgetDuty is the share of each frame interval LED writes may use. The
	// switch has one CPU that also runs its management plane; leave headroom.
	budgetDuty = 0.75

	// Channel changes no bigger than 1/minDeltaFrac of the channel's level are
	// deferred; the trickle reassert settles them later. Relative, so dim
	// effects (low brightness) don't lose their fade steps. Changes to 0 are
	// always sent.
	minDeltaFrac = 64

	// Some firmware / the UniFi controller occasionally reset LED state. Rather
	// than re-sending the whole frame at once (a ~275 ms stall on 48 ports),
	// re-send one port per frame when budget allows, or the all-ports color
	// every uniformReassert when the frame is a single color.
	reassertPortsPerFrame = 1
	uniformReassert       = 5 * time.Second
)

var channels = [3]byte{'r', 'g', 'b'}

func chanVal(c device.Color, i int) uint8 {
	switch i {
	case 0:
		return c.R
	case 1:
		return c.G
	}
	return c.B
}

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

// SetBehavior sets the LED controller behavior (0 = solid, 2-11 = hardware
// breathe rate), sending only changes. Starting a breathe needs a special
// sequence wrapped around an all-ports write, so that is left to the next
// PushFrame; rate changes and stopping are sent directly.
func (s *Switch) SetBehavior(b int) error {
	s.mu.Lock()
	client, cur, on := s.client, s.behavior, s.breatheOn
	s.mu.Unlock()
	if client == nil || b == cur {
		return nil
	}
	if b <= 0 || on {
		if err := client.SetBehavior(b); err != nil {
			return err
		}
	}
	s.mu.Lock()
	s.behavior = b
	if b <= 0 {
		s.breatheOn = false
	}
	s.mu.Unlock()
	return nil
}

// PushFrame moves the LEDs toward `frame` (1-indexed: frame[1] = port 1,
// frame[0] ignored) using at most a frame interval's worth of writes.
//
//   - If maxInFlight earlier frames are still unacked by the agent, this one
//     is skipped: the switch drops frames instead of building a backlog.
//   - Writes go per channel, only where the value changed by more than
//     1/minDeltaFrac of its level.
//   - When one all-ports write plus fix-ups beats the plain diff (e.g. a
//     mostly-uniform frame), that is used instead.
//   - Over budget, the biggest changes go first; the rest land next frame.
func (s *Switch) PushFrame(frame []device.Color, interval time.Duration) error {
	s.mu.Lock()
	client := s.client
	hw := s.hw
	lastAll := s.lastAll
	cursor := s.reassertAt
	// Under hardware breathe, rewriting a port may restart its breathe cycle,
	// so don't reassert; only real changes are sent.
	reassert := s.behavior <= 0
	startBreathe := 0
	if s.behavior > 0 && !s.breatheOn {
		startBreathe = s.behavior
	}
	s.mu.Unlock()
	if client == nil || len(frame) < 2 || client.Busy() {
		return nil
	}
	n := len(frame) - 1
	budget := int(budgetDuty * float64(interval) / float64(channelWriteCost))
	if budget < 3 {
		budget = 3
	}

	next := make([]device.Color, len(frame))
	if len(hw) == len(frame) {
		copy(next, hw)
	}
	base, baseCount := dominantColor(frame)
	uniform := baseCount == n

	diffCost := 3 * n
	if len(hw) == len(frame) {
		diffCost = countDiff(frame, hw)
	}
	baseCost := allPortCost + countDiffFrom(frame, base)

	var all *device.Color
	if startBreathe > 0 || len(hw) != len(frame) || baseCost < diffCost {
		all = &base
		budget -= allPortCost
		for i := 1; i <= n; i++ {
			next[i] = base
		}
	} else if reassert && uniform && diffCost == 0 && time.Since(lastAll) >= uniformReassert {
		all = &base
		budget -= allPortCost
	}

	type cand struct {
		w     device.ChannelWrite
		delta int
	}
	var cands []cand
	for i := 1; i <= n; i++ {
		for ch := 0; ch < 3; ch++ {
			want, have := chanVal(frame[i], ch), chanVal(next[i], ch)
			d := int(want) - int(have)
			if d < 0 {
				d = -d
			}
			level := int(want)
			if int(have) > level {
				level = int(have)
			}
			if d == 0 || (want != 0 && d*minDeltaFrac <= level) {
				continue
			}
			cands = append(cands, cand{device.ChannelWrite{Port: i, Ch: channels[ch], Value: want}, d})
		}
	}
	sort.SliceStable(cands, func(a, b int) bool { return cands[a].delta > cands[b].delta })

	writes := make([]device.ChannelWrite, 0, budget)
	for _, c := range cands {
		if len(writes) >= budget {
			break
		}
		writes = append(writes, c.w)
	}

	// Trickle reassert with whatever budget is left (not needed when this
	// frame already rewrote every port via the all-ports write).
	if reassert && all == nil && !uniform {
		for k := 0; k < reassertPortsPerFrame && len(writes)+3 <= budget; k++ {
			port := cursor%n + 1
			cursor++
			for ch := 0; ch < 3; ch++ {
				writes = append(writes, device.ChannelWrite{Port: port, Ch: channels[ch], Value: chanVal(frame[port], ch)})
			}
		}
	}

	if all == nil && len(writes) == 0 {
		return nil
	}
	if err := client.SendFrame(all, writes, startBreathe); err != nil {
		return err
	}

	for _, w := range writes {
		c := &next[w.Port]
		switch w.Ch {
		case 'r':
			c.R = w.Value
		case 'g':
			c.G = w.Value
		default:
			c.B = w.Value
		}
	}
	s.mu.Lock()
	if s.client == client {
		s.hw = next
		s.reassertAt = cursor
		if all != nil {
			s.lastAll = time.Now()
		}
		if startBreathe > 0 && s.behavior == startBreathe {
			s.breatheOn = true
		}
	}
	s.mu.Unlock()
	return nil
}

// dominantColor returns the most common color among ports 1..n and its count.
func dominantColor(frame []device.Color) (device.Color, int) {
	counts := make(map[device.Color]int, 8)
	var best device.Color
	bestN := 0
	for i := 1; i < len(frame); i++ {
		counts[frame[i]]++
		if c := counts[frame[i]]; c > bestN {
			best, bestN = frame[i], c
		}
	}
	return best, bestN
}

// countDiff counts channels that differ between two frames of equal length.
func countDiff(a, b []device.Color) int {
	n := 0
	for i := 1; i < len(a); i++ {
		for ch := 0; ch < 3; ch++ {
			if chanVal(a[i], ch) != chanVal(b[i], ch) {
				n++
			}
		}
	}
	return n
}

// countDiffFrom counts channels in frame that differ from a single color.
func countDiffFrom(frame []device.Color, c device.Color) int {
	n := 0
	for i := 1; i < len(frame); i++ {
		for ch := 0; ch < 3; ch++ {
			if chanVal(frame[i], ch) != chanVal(c, ch) {
				n++
			}
		}
	}
	return n
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
			Name:     c.Name,
			Host:     c.Host,
			behavior: -1,
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

// resetLEDState forgets what we believe the LEDs show, forcing a full push
// (and a behavior resend) on the next frame.
func (s *Switch) resetLEDState() {
	s.hw = nil
	s.lastAll = time.Time{}
	s.behavior = -1
	s.breatheOn = false
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
		s.resetLEDState()
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
			s.resetLEDState()
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
