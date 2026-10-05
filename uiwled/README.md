# UIWLED

WLED-style color, effects, and per-jack control for Ubiquiti Etherlighting switches — packaged as a Home Assistant addon plus a native HA integration.

![UIWLED logo](icon.png)

## What it does

Every RJ45 jack on your Ubiquiti Pro Max switch has an RGB LED around it. UniFi's own UI treats them as passive info displays (link speed, PoE, port locate). UIWLED turns them into an addressable LED strip you can drive from Home Assistant — solid colors, animated effects, per-jack overrides, presets, palettes, the works.

- **22+ WLED-style effects** — Solid, Breathe, Rainbow, Chase, Meteor, Fireworks, Pacifica, Sunrise, and many more, running at 5–15 FPS depending on effect type
- **Independent per-switch state** — different color/effect per switch
- **Per-jack override** — pin a single jack to any color while an effect keeps running on the rest (great for alerts, port-locate)
- **Palettes & presets** — 8 built-in palettes, editable custom palettes, named presets per switch
- **Home Assistant integration** — every switch appears as a `light.` entity with RGB + brightness + effect select
- **HA services** — `uiwled.set_port`, `uiwled.clear_port`, `uiwled.clear_all_ports` for scriptable per-jack control

## Supported hardware

- USW Pro Max 16 PoE
- USW Pro Max 24 PoE
- USW Pro Max 48 PoE
- (any USW Pro Max variant with Etherlighting should work — layout tables are trivially extendable)

Requires UniFi firmware 7.4.x or later.

## How it works

1. **On start**, the addon SSHes into each configured switch (creds are the site-wide UniFi SSH user).
2. **Discovers the model** via `mca-cli-op info` and pulls the port layout from a small local table.
3. **Uploads a tiny shell agent** to `/tmp/uiwled_agent.sh` and starts it as a persistent process.
4. **Streams frame commands** over the agent's stdin at the configured FPS (default 15).
5. **Effects run in Go** — an internal ticker computes each frame, diffs against the previous frame, and only pushes changed jacks. Uniform frames use `/proc/led/led_all_port_code` (one write per frame).
6. **HA integration** polls the addon's REST API to expose each switch as a `light.` entity.

## Configuration

In the addon's Configuration tab:

```yaml
switches:
  - name: rack_top       # anything you want, used in HA entity names
    host: 192.168.1.2    # switch IP
    user: admin          # UniFi site-wide SSH user
    password: yourpass   # UniFi site-wide SSH password
    ssh_key: ""          # optional PEM private key instead of password
listen_port: 8080        # web UI + REST
log_level: info          # debug, info, warn, error
target_fps: 15           # 5-20 recommended
```

**SSH credentials** are set in UniFi Network → Settings → System → Advanced → Device SSH Authentication.

## Home Assistant integration

Copy `custom_components/uiwled/` from the `config/` example (or install from HACS once packaged), restart Home Assistant, then:

1. Settings → Devices & Services → Add Integration → **UIWLED**
2. Host: `localhost`, Port: `8080` → Submit
3. Two entities per configured switch appear: one `light.` and the shared `uiwled.*` services

## Performance notes

Measured on a USW Pro Max 16 PoE (single-core MIPS 34Kc, firmware 7.5.15): every `/proc/led/led_color` write (one channel of one jack) costs **~1.9 ms**, mostly kernel time talking to the Etherlighting controller (a separate Nuvoton M031 MCU). A `led_all_port_color` write costs ~14 ms. The shell agent itself is cheap (~0.2 ms per line), so a native binary agent would gain little — the way to go faster is fewer writes:

- **Per-channel diff** — only the R/G/B channels that changed are written, and changes under ~1.5% of the channel level are deferred.
- **Write budget** — each frame uses at most ~75% of the frame interval in writes (the switch CPU also runs its management plane); the biggest changes go first and the rest land next frame.
- **All-ports base** — when one `led_all_port_color` write plus a few fix-ups is cheaper than the diff, that's used instead.
- **Frame acks** — the agent acks every frame; with 2 frames already in flight, new frames are dropped instead of queueing, so a slow switch never lags behind.
- **Trickle reassert** — one jack per frame is re-sent (or the all-ports color every 5 s for uniform frames) to heal resets from the controller, instead of a full-frame stall every 2 s.

Per-jack values use the controller's full 0–65535 range, so multi-color effects are as bright as uniform ones.

### Hardware breathe

`/proc/led/led_behavior` makes the LED controller itself breathe whatever colors the jacks show — perfectly smooth and zero CPU. `0`/`1` are solid, `2`–`11` breathe (faster as the value rises; it visibly tops out around 6). UIWLED exposes this as two effects:

- **Breathe (HW)** — color 1 on every jack.
- **Breathe Mix (HW)** — colors 1/2/3 repeated across the jacks.

Speed maps onto behavior 2–6. Per-jack overrides breathe along with the rest.

Starting a breathe is picky (fw 7.5.15): the controller only starts on behavior `2` followed by an all-ports color write; writing `3`–`11` from solid leaves the jacks frozen. Once breathing, `2`–`11` change the rate live. UIWLED sends `2`, the all-ports base, the per-jack fix-ups, then the target rate.

## Credits

The `/proc/led/*` command language was reverse-engineered by [robherley/etherlighter](https://github.com/robherley/etherlighter). UIWLED reuses that knowledge but is otherwise an independent implementation.

## License

MIT — see LICENSE.
