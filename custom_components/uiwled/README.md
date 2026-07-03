# UIWLED — Home Assistant custom integration

Bridges the [UIWLED addon](../../addons/uiwled/) into Home Assistant as native `light.` entities plus per-jack services.

## Install

1. Copy this folder to `<config>/custom_components/uiwled/` on your HA instance.
2. Restart Home Assistant (Developer Tools → Restart).
3. Settings → Devices & Services → **Add Integration** → search **UIWLED** → **Add**.
4. Host: `localhost` (or the addon's LAN IP), Port: `8080` → Submit.

Two entities appear per configured switch (name derived from your addon config), one `light.` each.

## Light entity

Supports:

- On/off
- Brightness (0–255)
- RGB color
- Effect selection (all 32 UIWLED effects show up in the effect dropdown)

## Services

### `uiwled.set_port`
Pin a single RJ45 jack to a color while any effect keeps running on the other jacks.

```yaml
service: uiwled.set_port
data:
  switch: rack_top
  port: 5
  rgb_color: [255, 0, 0]
  duration_ms: 3000   # optional; omit or 0 = persist until cleared
```

### `uiwled.clear_port`
Remove the override on one jack (returns it to the effect).

```yaml
service: uiwled.clear_port
data:
  switch: rack_top
  port: 5
```

### `uiwled.clear_all_ports`
Clear every per-jack override on a switch.

```yaml
service: uiwled.clear_all_ports
data:
  switch: rack_top
```

## Example: motion-triggered alert

```yaml
automation:
  - alias: "Flash port 5 red on motion"
    trigger:
      - platform: state
        entity_id: binary_sensor.hallway_motion
        to: "on"
    action:
      - service: uiwled.set_port
        data:
          switch: rack_top
          port: 5
          rgb_color: [255, 0, 0]
          duration_ms: 5000
```
