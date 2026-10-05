# UIWLED

**WLED-style color, effects, and per-jack control for Ubiquiti Etherlighting switches — Home Assistant addon plus native integration.**

![UIWLED logo](uiwled/icon.png)

Every RJ45 jack on your Ubiquiti Pro Max switch has an addressable RGB LED around it. UIWLED turns those into an addressable LED strip you can drive from Home Assistant — solid colors, animated effects, per-jack overrides, presets, palettes, the works.

## Repository layout

```
UIWLED/
├── uiwled/                    # the Home Assistant addon (Go daemon in Docker)
│   ├── config.yaml            # addon manifest
│   ├── Dockerfile             # multi-stage build: Go builder → HA base
│   ├── run.sh                 # container entrypoint
│   ├── icon.png, logo.png     # addon store assets
│   └── app/                   # Go source
│       ├── cmd/uiwled/        # main entry
│       └── internal/
│           ├── api/           # HTTP server + built-in web UI
│           ├── config/        # /data/options.json parser
│           ├── device/        # SSH + procfs + agent-upload
│           ├── engine/        # effect ticker, state, persistence
│           └── switches/      # per-switch supervise goroutines
├── custom_components/
│   └── uiwled/                # Home Assistant custom integration (Python)
├── repository.json            # HA addon-store manifest (so this repo is addable)
└── LICENSE                    # MIT
```

## Install

### Addon

1. In Home Assistant, **Settings → Add-ons → Add-on Store → ⋮ → Repositories** → add `https://github.com/richcj10/UIWLED`.
2. Install the **UIWLED** addon.
3. Configuration tab: fill in your switch(es) with UniFi SSH creds:
   ```yaml
   switches:
     - name: rack_top
       host: 192.168.1.2
       user: admin
       password: yourpass
   listen_port: 8080
   target_fps: 15
   log_level: info
   ```
   Site-wide SSH creds are set in UniFi Network → Settings → System → Advanced → Device SSH Authentication.
4. Start it. **Open Web UI** — pick colors, effects, palettes.

### Home Assistant integration

Install via **HACS** (recommended):

1. HACS → Integrations → ⋮ → **Custom repositories**
2. Repository `https://github.com/richcj10/UIWLED`, Category **Integration** → Add
3. Install **UIWLED** from the HACS list, restart Home Assistant.
4. Settings → Devices & Services → **Add Integration** → **UIWLED** → host `localhost`, port `8080`.

Or manually: copy `custom_components/uiwled/` into `<config>/custom_components/` on your HA instance, restart, then add the integration as above.

Each switch shows up as a `light.` entity with RGB + brightness + effect select, plus three services (`uiwled.set_port`, `uiwled.clear_port`, `uiwled.clear_all_ports`) for per-jack alerts and automations.

## Supported hardware

- USW Pro Max 16 PoE
- USW Pro Max 24 PoE
- USW Pro Max 48 PoE

(Any USW Pro Max with Etherlighting should work — layout tables are trivially extendable.)

Requires UniFi firmware 7.4.x or later.

## Features

- **34 effects** — Solid, Breathe, Rainbow, Chase, Meteor, Fireworks, Pacifica, Sunrise, and many more, plus two hardware-breathe effects that run on the LED controller itself
- **Per-switch independent state** — different color/effect per switch
- **Per-jack override** — pin a jack to any color while an effect keeps running on the rest (great for alerts, port-locate)
- **Palettes and presets** — 8 built-in palettes, editable custom palettes, named presets per switch
- **HA integration** — every switch is a native `light.` entity; per-jack services drive automations
- **State persistence** — settings survive addon restart
- **Auto-recovery** — SSH connection death is detected and reconnected

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

The `/proc/led/*` command language was reverse-engineered by [robherley/etherlighter](https://github.com/robherley/etherlighter). UIWLED reuses that knowledge but is otherwise an independent implementation with a switch-side agent, WLED-style effects, and an HA integration.

## License

MIT — see [LICENSE](LICENSE).
