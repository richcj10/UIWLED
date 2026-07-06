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

- **32 effects** — Solid, Breathe, Rainbow, Chase, Meteor, Fireworks, Pacifica, Sunrise, and many more
- **Per-switch independent state** — different color/effect per switch
- **Per-jack override** — pin a jack to any color while an effect keeps running on the rest (great for alerts, port-locate)
- **Palettes and presets** — 8 built-in palettes, editable custom palettes, named presets per switch
- **HA integration** — every switch is a native `light.` entity; per-jack services drive automations
- **State persistence** — settings survive addon restart
- **Auto-recovery** — SSH connection death is detected and reconnected

## Performance notes

Uniform-color effects (Solid, Breathe, Fade) run smoothly at 15+ FPS via a single `/proc/led/led_all_port_code` write per frame. Per-jack effects (Rainbow, Chase) top out around **5 FPS on 48-port switches** — the switch's shell just can't process procfs writes faster. Higher framerates would need a cross-compiled native binary agent.

## Credits

The `/proc/led/*` command language was reverse-engineered by [robherley/etherlighter](https://github.com/robherley/etherlighter). UIWLED reuses that knowledge but is otherwise an independent implementation with a switch-side agent, WLED-style effects, and an HA integration.

## License

MIT — see [LICENSE](LICENSE).
