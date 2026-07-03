package device

// agentScript is uploaded to each switch and run as a long-lived process.
// It reads compact commands from stdin and writes /proc/led/* directly.
//
// Protocol (one command per line, space-separated):
//   A RR GG BB br          — all ports to hex color RR GG BB, brightness 0-100
//   P n RR GG BB           — port n to hex color RR GG BB (via led_code)
//   L n r|g|b val          — port n single channel decimal val (via led_color fallback)
//   Q                      — quit
//
// Kept as a single 'sh' script so we don't need to cross-compile a binary.
const agentScript = `#!/bin/sh
# UIWLED agent (v3)
# Reads LED frame commands from stdin, drives /proc/led/*.
#
# Protocol (HA pre-scales all values so this loop does zero arithmetic):
#   A rr gg bb br      hex RGB + decimal brightness, one write to led_all_port_code
#   P n R100 G100 B100 decimal values (already scaled *100), 3 writes to led_color
#   Q                  quit
#
# NOTE: /proc/led/led_code (combined RGB per port) is broken on UniFi 7.4.1 —
# it always renders reddish. We use per-channel led_color which works.

echo 0 > /proc/led/led_mode 2>/dev/null

while IFS=' ' read -r op a b c d; do
  case "$op" in
    P)
      echo "$a r $b" > /proc/led/led_color 2>/dev/null
      echo "$a g $c" > /proc/led/led_color 2>/dev/null
      echo "$a b $d" > /proc/led/led_color 2>/dev/null
      ;;
    A)
      echo "$a $b $c $d" > /proc/led/led_all_port_code 2>/dev/null
      ;;
    Q) exit 0 ;;
    "") ;;
    *) ;;
  esac
done
`

// agentPath is where we upload the script on the switch. /tmp is writable
// on UniFi firmware and cleared on reboot, which is fine — we redeploy on
// every connect anyway.
const agentPath = "/tmp/uiwled_agent.sh"
