package device

import (
	"errors"
	"fmt"
	"strings"
)

// procfs command builders.
//
// Etherlighting exposes writable files under /proc/led/:
//
//   /proc/led/led_mode           — "0" (write to all ports) or "1" (write to connected only)
//   /proc/led/led_config         — set a firmware animation mode
//   /proc/led/led_all_port_code  — "RR GG BB brightness" (hex per channel, brightness 0-100)
//   /proc/led/led_color          — "<port> <r|g|b> <value*100>"  (per-jack, per-channel)
//
// Every custom-color write session must be preceded by `echo 0 > /proc/led/led_mode`
// to enable full-port control. led_config must NOT be written when running
// custom colors — it snaps the switch back into a firmware animation.

const (
	pathLEDMode    = "/proc/led/led_mode"
	pathLEDConfig  = "/proc/led/led_config"
	pathLEDAll     = "/proc/led/led_all_port_code"
	pathLEDPerPort = "/proc/led/led_color"
)

// CmdSetLEDMode returns the shell command to enable custom-color writes.
func CmdSetLEDMode(mode uint8) string {
	return fmt.Sprintf("echo %d > %s", mode, pathLEDMode)
}

// CmdSetAllPorts returns the shell command to set every jack to one color.
func CmdSetAllPorts(c Color, brightness uint8) string {
	if brightness > 100 {
		brightness = 100
	}
	return fmt.Sprintf("echo '%02X %02X %02X %d' > %s",
		c.R, c.G, c.B, brightness, pathLEDAll)
}

// CmdSetPortColor returns the three echo commands (r, g, b) for a single jack.
// Values are scaled by 100 as required by the procfs interface.
func CmdSetPortColor(port int, c Color) []string {
	return []string{
		fmt.Sprintf("echo %d r %d > %s", port, int(c.R)*100, pathLEDPerPort),
		fmt.Sprintf("echo %d g %d > %s", port, int(c.G)*100, pathLEDPerPort),
		fmt.Sprintf("echo %d b %d > %s", port, int(c.B)*100, pathLEDPerPort),
	}
}

// BuildFrameBatch produces the per-jack write commands for one frame.
// It intentionally does NOT touch /proc/led/led_mode — writing to led_mode
// resets all ports first, causing a visible off/on flicker on each frame.
// Call InitLEDMode() once after connect to put the switch into writable mode.
func BuildFrameBatch(colors []PortColor) []string {
	cmds := make([]string, 0, len(colors)*3)
	for _, pc := range colors {
		cmds = append(cmds, CmdSetPortColor(pc.Index, pc.Color)...)
	}
	return cmds
}

// InitLEDMode enables custom-color writes. Call once after connect; do not
// call per-frame (it resets all port colors and causes flicker).
func (c *Client) InitLEDMode() error {
	return c.ExecBatch([]string{CmdSetLEDMode(0)})
}

// SetAllPorts sets every jack to one color. Prefers the agent protocol
// ("A rr gg bb br" on stdin → one procfs write). Falls back to direct exec.
func (c *Client) SetAllPorts(color Color, brightness uint8) error {
	if brightness > 100 {
		brightness = 100
	}
	agentCmd := fmt.Sprintf("A %02X %02X %02X %d", color.R, color.G, color.B, brightness)
	if err := c.shellFF(agentCmd); err == nil {
		return nil
	}
	return c.ExecBatch([]string{CmdSetAllPorts(color, brightness)})
}

// SetPortColors sends per-jack colors. Emits pre-scaled decimals ("P n R G B"
// where each is r*100 etc.) so the agent's read loop is arithmetic-free.
// Falls back to direct exec if the agent isn't ready.
func (c *Client) SetPortColors(colors []PortColor) error {
	if len(colors) == 0 {
		return nil
	}
	var sb strings.Builder
	for _, pc := range colors {
		fmt.Fprintf(&sb, "P %d %d %d %d\n",
			pc.Index, int(pc.Color.R)*100, int(pc.Color.G)*100, int(pc.Color.B)*100)
	}
	agentMulti := strings.TrimSuffix(sb.String(), "\n")
	if err := c.shellFF(agentMulti); err == nil {
		return nil
	}
	return c.ExecBatch(BuildFrameBatch(colors))
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
