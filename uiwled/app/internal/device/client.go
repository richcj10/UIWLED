// Package device speaks to a single Ubiquiti Etherlighting switch over SSH.
//
// The Client wraps a long-lived ssh.Client plus a "frame shell" — a persistent
// session running the UIWLED agent script (see agent.go). Frame commands go
// via the agent's stdin for high throughput; one-shot commands (info,
// mode-init) go via a fresh session each call.
//
// The /proc/led/* command language is based on reverse engineering by
// robherley/etherlighter (github.com/robherley/etherlighter). See procfs.go
// for the command format details.
package device

import (
	"errors"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"golang.org/x/crypto/ssh"
)

// Color is an 8-bit RGB triple. Used as the frame-buffer element type.
type Color struct{ R, G, B uint8 }

// Info is the subset of `mca-cli-op info` we care about, plus the derived
// jack layout (see LayoutFor).
type Info struct {
	Hostname string
	IP       string
	MAC      string
	Model    string
	Version  string
	Uptime   string
	Layout   [][]int
}

// Config carries per-switch SSH connection details. Either Password or
// KeyPEM must be set. Addr may omit :22.
type Config struct {
	Name     string
	Addr     string // host or host:port
	User     string
	Password string
	KeyPEM   string // PEM-encoded private key (optional)
}

// Client owns one SSH connection to a switch and (once StartFrameShell is
// called) a persistent agent process. All methods are safe to call from
// multiple goroutines.
type Client struct {
	cfg Config

	mu   sync.Mutex
	ssh  *ssh.Client
	info *Info

	// Persistent shell for high-rate frame writes. Avoids the ~50ms cost of
	// opening a new SSH session per push. Fire-and-forget: we write frame
	// commands and don't wait for output (agent writes produce none).
	shellMu      sync.Mutex // serializes writes to shellIn
	shellSess    *ssh.Session
	shellIn      io.WriteCloser
	shellStdout  io.Reader // drained by shellDrainer goroutine
	shellReady   bool
	shellStopper chan struct{}

	// Frame acks: SendFrame bumps pending, the agent answers each frame's
	// trailing "S" with "K", and shellDrainer counts those back down. While
	// a frame is outstanding the pusher skips new frames instead of queueing
	// them in the SSH pipe, so a slow switch drops frames rather than lagging.
	pending atomic.Int32
	sentAt  atomic.Int64 // unix nanos of the last SendFrame
}

const (
	// ackTimeout bounds how long we wait for a frame ack before assuming it
	// was lost (agent restarted, shell reopened) and sending again.
	ackTimeout = time.Second
	// maxInFlight frames may be sent before the oldest is acked. One frame of
	// slack absorbs ack jitter (an ack landing just after the next tick would
	// otherwise skip a frame) while still bounding the backlog.
	maxInFlight = 2
)

// Busy reports whether maxInFlight frames sent via the agent are unacked.
func (c *Client) Busy() bool {
	if c.pending.Load() < maxInFlight {
		return false
	}
	if time.Since(time.Unix(0, c.sentAt.Load())) > ackTimeout {
		c.pending.Store(0)
		return false
	}
	return true
}

// Dial opens a fresh SSH connection using cfg. Returns a Client with the
// connection established but no frame shell yet — call StartFrameShell
// after fetching Info to prepare the agent.
func Dial(cfg Config) (*Client, error) {
	if !strings.Contains(cfg.Addr, ":") {
		cfg.Addr = cfg.Addr + ":22"
	}
	c := &Client{cfg: cfg}
	if err := c.connect(); err != nil {
		return nil, err
	}
	return c, nil
}

func (c *Client) connect() error {
	sshCfg, err := c.buildSSHConfig()
	if err != nil {
		return err
	}
	conn, err := ssh.Dial("tcp", c.cfg.Addr, sshCfg)
	if err != nil {
		return fmt.Errorf("ssh dial %s: %w", c.cfg.Addr, err)
	}
	c.ssh = conn
	return nil
}

func (c *Client) buildSSHConfig() (*ssh.ClientConfig, error) {
	var auth []ssh.AuthMethod
	if c.cfg.KeyPEM != "" {
		signer, err := ssh.ParsePrivateKey([]byte(c.cfg.KeyPEM))
		if err != nil {
			return nil, fmt.Errorf("parse ssh key: %w", err)
		}
		auth = append(auth, ssh.PublicKeys(signer))
	}
	if c.cfg.Password != "" {
		auth = append(auth, ssh.Password(c.cfg.Password))
	}
	if len(auth) == 0 {
		return nil, errors.New("no ssh auth configured (password or ssh_key required)")
	}
	return &ssh.ClientConfig{
		User: c.cfg.User,
		Auth: auth,
		// Ubiquiti device host keys aren't distributed. Accepting any key is
		// acceptable here because switch IPs are on a private LAN and the
		// addon config already trusts a password/key for that host.
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         10 * time.Second,
	}, nil
}

// Close shuts down the frame shell (if running) and the underlying SSH
// connection. Idempotent.
func (c *Client) Close() error {
	c.stopShell()
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.ssh != nil {
		err := c.ssh.Close()
		c.ssh = nil
		return err
	}
	return nil
}

// SSHWait blocks until the underlying SSH client closes (peer reset, network
// drop, remote sshd death, etc.). Returns the reason the connection ended.
// Used by the switch supervisor to notice a dead connection and reconnect
// the whole client rather than uselessly retrying frame pushes on a corpse.
func (c *Client) SSHWait() error {
	c.mu.Lock()
	sshc := c.ssh
	c.mu.Unlock()
	if sshc == nil {
		return errors.New("ssh not connected")
	}
	return sshc.Wait()
}

// StartFrameShell uploads the UIWLED agent script (if needed) and runs it as
// a long-lived process on the switch. Frame commands are written to the
// agent's stdin. Replaces the previous "interactive shell parsing echoes"
// design — the agent's while-read loop parses far fewer tokens per frame.
func (c *Client) StartFrameShell() error {
	c.shellMu.Lock()
	if c.shellReady {
		c.shellMu.Unlock()
		return nil
	}
	c.shellMu.Unlock()

	c.mu.Lock()
	sshc := c.ssh
	c.mu.Unlock()
	if sshc == nil {
		return errors.New("ssh not connected")
	}

	// Always re-upload — /tmp gets wiped on switch reboot, and the script is
	// tiny (~500B). One-shot exec via heredoc.
	if err := c.uploadAgent(); err != nil {
		return fmt.Errorf("upload agent: %w", err)
	}

	sess, err := sshc.NewSession()
	if err != nil {
		return fmt.Errorf("agent session: %w", err)
	}
	stdin, err := sess.StdinPipe()
	if err != nil {
		_ = sess.Close()
		return fmt.Errorf("stdin pipe: %w", err)
	}
	stdout, err := sess.StdoutPipe()
	if err != nil {
		_ = sess.Close()
		return fmt.Errorf("stdout pipe: %w", err)
	}
	// Start the agent directly — no interactive shell in the middle.
	if err := sess.Start("sh " + agentPath); err != nil {
		_ = sess.Close()
		return fmt.Errorf("start agent: %w", err)
	}

	c.shellMu.Lock()
	c.shellSess = sess
	c.shellIn = stdin
	c.shellStdout = stdout
	c.shellStopper = make(chan struct{})
	c.shellReady = true
	c.shellMu.Unlock()
	c.pending.Store(0)

	go c.shellDrainer()
	slog.Info("uiwled agent started on switch", "addr", c.cfg.Addr)
	return nil
}

// uploadAgent writes the script to /tmp/uiwled_agent.sh via a heredoc exec.
// The delimiter is quoted so $ and backticks inside the script aren't
// expanded by the receiving shell.
func (c *Client) uploadAgent() error {
	cmd := "cat > " + agentPath + " << 'UIWLED_EOF_MARKER'\n" + agentScript + "UIWLED_EOF_MARKER\n"
	_, err := c.exec(cmd)
	return err
}

// stopShell tears the persistent shell down. Called by Close and by
// shellFF's auto-recovery path when a write fails.
func (c *Client) stopShell() {
	c.shellMu.Lock()
	defer c.shellMu.Unlock()
	if !c.shellReady {
		return
	}
	c.shellReady = false
	close(c.shellStopper)
	if c.shellIn != nil {
		_ = c.shellIn.Close()
	}
	if c.shellSess != nil {
		_ = c.shellSess.Close()
	}
	c.shellSess = nil
	c.shellIn = nil
	c.shellStdout = nil
}

// shellDrainer keeps the SSH channel unblocked by continuously reading the
// agent's stdout, and counts the "K" frame acks it finds there (see Busy).
// Without the drain, the SSH flow-control window would fill and our stdin
// writes would eventually block.
func (c *Client) shellDrainer() {
	buf := make([]byte, 4096)
	for {
		select {
		case <-c.shellStopper:
			return
		default:
		}
		// Read blocks until data arrives or the pipe closes; no busy loop.
		// No logging in this hot path — a debug log per read floods stdout and
		// backpressures the whole shell channel.
		n, err := c.shellStdout.Read(buf)
		for _, b := range buf[:n] {
			if b == 'K' && c.pending.Add(-1) < 0 {
				c.pending.Store(0)
			}
		}
		if err != nil {
			return
		}
	}
}

// shellFF writes cmd + newline to the persistent shell's stdin. Fire and forget.
// If the shell isn't ready or the write fails, tries to reopen once before
// returning an error (letting the caller fall back to session-per-exec).
func (c *Client) shellFF(cmd string) error {
	c.shellMu.Lock()
	ready := c.shellReady
	stdin := c.shellIn
	c.shellMu.Unlock()

	if ready && stdin != nil {
		if _, err := io.WriteString(stdin, cmd+"\n"); err == nil {
			return nil
		}
		// Write failed → shell probably died. Fall through to reopen.
		slog.Warn("frame shell write failed, reopening", "addr", c.cfg.Addr)
		c.stopShell()
	}
	// Try to reopen the shell in-line so the next frame can use it.
	if err := c.StartFrameShell(); err != nil {
		return fmt.Errorf("shell reopen: %w", err)
	}
	c.shellMu.Lock()
	stdin = c.shellIn
	c.shellMu.Unlock()
	if stdin == nil {
		return errors.New("frame shell not ready after reopen")
	}
	_, err := io.WriteString(stdin, cmd+"\n")
	return err
}

// exec runs a single command on a fresh SSH session and returns combined
// output. Use this for one-shot commands (info, mode changes). For per-frame
// writes, use shellFF (via SendFrame / SetBehavior) to hit the agent.
func (c *Client) exec(cmd string) (string, error) {
	return c.execOpts(cmd, false)
}

// execPty is exec with a PTY allocated. Needed for tools like mca-cli-op
// which on some UniFi firmware write to /dev/tty rather than stdout.
func (c *Client) execPty(cmd string) (string, error) {
	return c.execOpts(cmd, true)
}

// execOpts is the shared implementation of exec and execPty. Uses
// session.CombinedOutput internally rather than a raw shared bytes.Buffer to
// avoid a stdout/stderr race in older crypto/ssh versions.
func (c *Client) execOpts(cmd string, pty bool) (string, error) {
	c.mu.Lock()
	sshc := c.ssh
	c.mu.Unlock()
	if sshc == nil {
		return "", errors.New("ssh connection closed")
	}
	session, err := sshc.NewSession()
	if err != nil {
		return "", fmt.Errorf("new session: %w", err)
	}
	defer session.Close()

	if pty {
		modes := ssh.TerminalModes{
			ssh.ECHO:          0,
			ssh.TTY_OP_ISPEED: 14400,
			ssh.TTY_OP_OSPEED: 14400,
		}
		if err := session.RequestPty("xterm", 80, 40, modes); err != nil {
			return "", fmt.Errorf("request pty: %w", err)
		}
	}

	// CombinedOutput uses an internal locked writer — safer than sharing a raw
	// bytes.Buffer between stdout and stderr goroutines inside crypto/ssh.
	raw, runErr := session.CombinedOutput(cmd)
	out := string(raw)
	slog.Debug("ssh exec", "cmd", cmd, "pty", pty, "bytes", len(out), "err", runErr)
	if runErr != nil {
		return out, runErr
	}
	return out, nil
}

// ExecBatch is the fallback path for frame writes when the persistent agent
// isn't available. Joins commands with ' ; ' and runs them in one fresh
// SSH session. Uses ';' rather than '&&' so a single-port failure doesn't
// abort the whole frame.
func (c *Client) ExecBatch(cmds []string) error {
	if len(cmds) == 0 {
		return nil
	}
	_, err := c.exec(strings.Join(cmds, " ; "))
	return err
}

// Info fetches hostname/model/etc via `mca-cli-op info` and populates the
// jack layout from the model name.
//
// The PTY retry is retained because some observed UniFi firmware versions had
// mca-cli-op writing to /dev/tty rather than stdout — non-PTY sessions saw
// zero output. CombinedOutput fixed the issue on our test firmware but the
// fallback is cheap insurance for future firmware variants.
func (c *Client) Info() (*Info, error) {
	out, err := c.exec("mca-cli-op info")
	if err != nil {
		return nil, fmt.Errorf("mca-cli-op info: %w", err)
	}
	if strings.TrimSpace(out) == "" {
		out, err = c.execPty("mca-cli-op info")
		if err != nil {
			return nil, fmt.Errorf("mca-cli-op info (pty): %w", err)
		}
	}
	if strings.TrimSpace(out) == "" {
		return nil, fmt.Errorf("mca-cli-op info returned empty output — reconnecting")
	}
	info := &Info{}
	for _, line := range strings.Split(out, "\n") {
		k, v, ok := splitKV(line, ":")
		if !ok {
			continue
		}
		switch k {
		case "Hostname":
			info.Hostname = v
		case "IP Address":
			info.IP = v
		case "MAC Address":
			info.MAC = v
		case "Model":
			info.Model = v
		case "Version":
			info.Version = v
		case "Uptime":
			info.Uptime = v
		}
	}
	info.Layout = LayoutFor(info.Model)
	c.mu.Lock()
	c.info = info
	c.mu.Unlock()
	return info, nil
}

// CachedInfo returns the last Info populated by a successful Info() call,
// or nil if Info hasn't yet succeeded.
func (c *Client) CachedInfo() *Info {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.info
}

// splitKV cuts a "key<sep>value" line at the first separator and trims
// surrounding whitespace off each half. Returns false if no separator found.
func splitKV(line, sep string) (string, string, bool) {
	i := strings.Index(line, sep)
	if i < 0 {
		return "", "", false
	}
	return strings.TrimSpace(line[:i]), strings.TrimSpace(line[i+len(sep):]), true
}

