package device

// agentScript is uploaded to each switch and run as a long-lived process.
// It reads compact commands from stdin and writes /proc/led/* directly.
//
// Protocol (one command per line, space-separated; values are pre-scaled by
// the daemon so the loop does no arithmetic):
//
//	C n ch val   port n, channel ch (r|g|b), value 0-65535 -> led_color
//	A r g b      every port to 16-bit r g b -> led_all_port_color
//	B n          controller behavior: 0 = solid, 2-11 = hardware breathe
//	S            end of frame: reply "K" on stdout (frame ack / backpressure)
//	Q            quit
//
// Kept as a single 'sh' script so we don't need to cross-compile a binary.
// Measured on fw 7.5.15: the shell costs ~0.2 ms per line while the driver
// costs ~1.9 ms per led_color write, so a native agent would gain little.
const agentScript = `#!/bin/sh
# UIWLED agent (v4)
# Reads LED frame commands from stdin, drives /proc/led/*.
#
#   C n ch val   one channel of port n, 0-65535      (led_color)
#   A r g b      all ports, 16-bit per channel       (led_all_port_color)
#   B n          0 = solid, 2-11 = controller breathe (led_behavior)
#   S            frame done -> echo K
#   Q            quit
#
# The /proc files are opened once and kept open (re-opening per write costs
# ~20% more). /proc/led/led_code only accepts a fixed set of color codes, so
# per-port color goes through led_color one channel at a time.

echo 0 > /proc/led/led_mode 2>/dev/null
echo 0 > /proc/led/led_behavior 2>/dev/null
exec 3>/proc/led/led_color 4>/proc/led/led_all_port_color 2>/dev/null

while IFS=' ' read -r op a b c; do
  case "$op" in
    C) echo "$a $b $c" >&3 ;;
    A) echo "$a $b $c 0" >&4 ;;
    B) echo "$a" > /proc/led/led_behavior ;;
    S) echo K ;;
    Q) exit 0 ;;
  esac
done
`

// agentPath is where we upload the script on the switch. /tmp is writable
// on UniFi firmware and cleared on reboot, which is fine — we redeploy on
// every connect anyway.
const agentPath = "/tmp/uiwled_agent.sh"
