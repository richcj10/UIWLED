package device

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

// procfs command builders.
//
// Etherlighting exposes writable files under /proc/led/:
//
//   /proc/led/led_mode            — "0" (write to all ports) or "1" (write to connected only)
//   /proc/led/led_config          — set a firmware animation mode
//   /proc/led/led_all_port_color  — "R G B W" (each 0-65535)
//   /proc/led/led_color           — "<port> <r|g|b|w> <0-65535>"  (per-jack, per-channel)
//   /proc/led/led_behavior        — 0 = solid, 2-11 = controller-side breathe (faster as it rises)
//
// Every custom-color write session must be preceded by `echo 0 > /proc/led/led_mode`
// to enable full-port control. led_config must NOT be written when running
// custom colors — it snaps the switch back into a firmware animation.
//
// Measured cost on fw 7.5.15 (MIPS 34Kc): ~1.9 ms per led_color write and
// ~14 ms per led_all_port_color write, mostly kernel time talking to the LED
// controller. Fewer writes is the only way to go faster.

const (
	pathLEDMode     = "/proc/led/led_mode"
	pathLEDConfig   = "/proc/led/led_config"
	pathLEDAll      = "/proc/led/led_all_port_color"
	pathLEDPerPort  = "/proc/led/led_color"
	pathLEDBehavior = "/proc/led/led_behavior"
)

// ChannelWrite is one led_color write: a single color channel of one port.
type ChannelWrite struct {
	Port  int
	Ch    byte  // 'r', 'g' or 'b'
	Value uint8 // 0-255; scaled to the driver's 0-65535 range on send
}

// scale16 maps an 8-bit channel onto the driver's 16-bit range (255 -> 65535).
func scale16(v uint8) int { return int(v) * 257 }

// CmdSetLEDMode returns the shell command to enable custom-color writes.
func CmdSetLEDMode(mode uint8) string {
	return fmt.Sprintf("echo %d > %s", mode, pathLEDMode)
}

// CmdSetAllPorts returns the shell command to set every jack to one color.
func CmdSetAllPorts(c Color) string {
	return fmt.Sprintf("echo '%d %d %d 0' > %s", scale16(c.R), scale16(c.G), scale16(c.B), pathLEDAll)
}

// CmdSetChannel returns the shell command for a single led_color write.
func CmdSetChannel(w ChannelWrite) string {
	return fmt.Sprintf("echo '%d %c %d' > %s", w.Port, w.Ch, scale16(w.Value), pathLEDPerPort)
}

// InitLEDMode enables custom-color writes. Call once after connect; do not
// call per-frame (it resets all port colors and causes flicker).
func (c *Client) InitLEDMode() error {
	return c.ExecBatch([]string{CmdSetLEDMode(0)})
}

// SendFrame applies one frame: an optional all-ports base color first, then
// individual channel writes. Over the agent it ends with an "S" so the agent
// acks with "K" once the writes are done (see Busy). Falls back to a
// synchronous exec if the agent isn't available.
//
// startBreathe > 0 starts hardware breathe at that rate. Observed on fw
// 7.5.15: the controller only starts breathing on led_behavior 2 followed by
// an all-ports write (writing 3-11 from solid, or 2 without the all-ports
// write, leaves the jacks frozen); once breathing, 2-11 change the rate. So
// the frame becomes "B 2", the all-ports write (all must be non-nil), the
// fix-ups, then "B rate".
func (c *Client) SendFrame(all *Color, writes []ChannelWrite, startBreathe int) error {
	var sb strings.Builder
	if startBreathe > 0 {
		sb.WriteString("B 2\n")
	}
	if all != nil {
		fmt.Fprintf(&sb, "A %d %d %d\n", scale16(all.R), scale16(all.G), scale16(all.B))
	}
	for _, w := range writes {
		fmt.Fprintf(&sb, "C %d %c %d\n", w.Port, w.Ch, scale16(w.Value))
	}
	if startBreathe > 2 {
		fmt.Fprintf(&sb, "B %d\n", startBreathe)
	}
	sb.WriteString("S")

	c.pending.Add(1)
	c.sentAt.Store(time.Now().UnixNano())
	if err := c.shellFF(sb.String()); err == nil {
		return nil
	}
	c.pending.Store(0)

	cmds := make([]string, 0, len(writes)+3)
	if startBreathe > 0 {
		cmds = append(cmds, fmt.Sprintf("echo 2 > %s", pathLEDBehavior))
	}
	if all != nil {
		cmds = append(cmds, CmdSetAllPorts(*all))
	}
	for _, w := range writes {
		cmds = append(cmds, CmdSetChannel(w))
	}
	if startBreathe > 2 {
		cmds = append(cmds, fmt.Sprintf("echo %d > %s", startBreathe, pathLEDBehavior))
	}
	return c.ExecBatch(cmds)
}

// SetBehavior sets the LED controller's behavior: 0 = solid, 2-11 = hardware
// breathe rate (faster as it rises; visibly saturates around 6). Only changes
// the rate of a breathe that is already running — starting one needs
// SendFrame's startBreathe. Out-of-range values are rejected by the driver.
func (c *Client) SetBehavior(b int) error {
	if err := c.shellFF(fmt.Sprintf("B %d", b)); err == nil {
		return nil
	}
	return c.ExecBatch([]string{fmt.Sprintf("echo %d > %s", b, pathLEDBehavior)})
}

// FirmwareMode is one of the built-in Etherlighting animations.
type FirmwareMode string

const (
	FirmwareRainbow  FirmwareMode = "0"    // cold reset — back-and-forth rainbow
	FirmwareBreathe  FirmwareMode = "1"    // warm reset — white breathe
	FirmwareOperate  FirmwareMode = "2"    // boot done — normal operation
	FirmwareSpeed    FirmwareMode = "10 0" // speed indicator
	FirmwareNetwork  FirmwareMode = "10 1"
	FirmwarePOE      FirmwareMode = "10 2"
	FirmwareLocateOn FirmwareMode = "10 4"
)

// SetFirmwareMode hands control back to the switch firmware for its own
// animation. Any custom colors are lost.
func (c *Client) SetFirmwareMode(m FirmwareMode) error {
	if strings.TrimSpace(string(m)) == "" {
		return errors.New("empty firmware mode")
	}
	return c.ExecBatch([]string{
		CmdSetLEDMode(1),
		fmt.Sprintf("echo %s > %s", m, pathLEDConfig),
	})
}
