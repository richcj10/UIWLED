"""UIWLED integration — bridges the UIWLED addon's REST API into Home Assistant.

Each Ubiquiti Etherlighting switch known to the addon becomes a `light.` entity.
Per-jack overrides are exposed via services (uiwled.set_port etc.).
"""
from __future__ import annotations

import logging
from typing import Any

import aiohttp
import voluptuous as vol
from homeassistant.config_entries import ConfigEntry
from homeassistant.const import Platform
from homeassistant.core import HomeAssistant, ServiceCall
from homeassistant.helpers.aiohttp_client import async_get_clientsession
import homeassistant.helpers.config_validation as cv

from .const import CONF_HOST, CONF_PORT, DEFAULT_PORT, DOMAIN

_LOGGER = logging.getLogger(__name__)

PLATFORMS: list[Platform] = [Platform.LIGHT]

# Service schemas — 'host' is optional and only needed when multiple UIWLED
# addons are configured; otherwise we pick the first entry.
_HOST = vol.Optional("host")
_SWITCH = vol.Required("switch")
_PORT = vol.Required("port")
_RGB = vol.Required("rgb_color")
_DURATION = vol.Optional("duration_ms", default=0)

SERVICE_SET_PORT = "set_port"
SERVICE_CLEAR_PORT = "clear_port"
SERVICE_CLEAR_ALL_PORTS = "clear_all_ports"

SCHEMA_SET_PORT = vol.Schema(
    {
        _HOST: cv.string,
        _SWITCH: cv.string,
        _PORT: vol.All(vol.Coerce(int), vol.Range(min=1, max=1024)),
        _RGB: vol.All(list, vol.Length(min=3, max=3), [vol.All(vol.Coerce(int), vol.Range(min=0, max=255))]),
        _DURATION: vol.All(vol.Coerce(int), vol.Range(min=0, max=3_600_000)),
    }
)
SCHEMA_CLEAR_PORT = vol.Schema(
    {
        _HOST: cv.string,
        _SWITCH: cv.string,
        _PORT: vol.All(vol.Coerce(int), vol.Range(min=1, max=1024)),
    }
)
SCHEMA_CLEAR_ALL = vol.Schema(
    {
        _HOST: cv.string,
        _SWITCH: cv.string,
    }
)


def _resolve_base_url(hass: HomeAssistant, requested_host: str | None) -> str | None:
    """Pick which UIWLED addon this service call targets."""
    domain_data = hass.data.get(DOMAIN, {})
    if not domain_data:
        return None
    if requested_host:
        for cfg in domain_data.values():
            if cfg.get(CONF_HOST) == requested_host:
                return f"http://{cfg[CONF_HOST]}:{cfg.get(CONF_PORT, DEFAULT_PORT)}"
    # No host specified (or no match) — use the first configured addon.
    cfg = next(iter(domain_data.values()))
    return f"http://{cfg[CONF_HOST]}:{cfg.get(CONF_PORT, DEFAULT_PORT)}"


async def async_setup_entry(hass: HomeAssistant, entry: ConfigEntry) -> bool:
    """Set up UIWLED from a config entry."""
    hass.data.setdefault(DOMAIN, {})
    hass.data[DOMAIN][entry.entry_id] = {
        CONF_HOST: entry.data[CONF_HOST],
        CONF_PORT: entry.data.get(CONF_PORT, DEFAULT_PORT),
    }
    await hass.config_entries.async_forward_entry_setups(entry, PLATFORMS)
    _register_services(hass)
    return True


async def async_unload_entry(hass: HomeAssistant, entry: ConfigEntry) -> bool:
    """Unload a UIWLED config entry."""
    unload_ok = await hass.config_entries.async_unload_platforms(entry, PLATFORMS)
    if unload_ok:
        hass.data[DOMAIN].pop(entry.entry_id, None)
    # Services are cheap; leave them registered across reloads. Only remove if
    # no config entries remain.
    if not hass.data.get(DOMAIN):
        for svc in (SERVICE_SET_PORT, SERVICE_CLEAR_PORT, SERVICE_CLEAR_ALL_PORTS):
            hass.services.async_remove(DOMAIN, svc)
    return unload_ok


def _register_services(hass: HomeAssistant) -> None:
    """Idempotent service registration."""

    async def _call_addon(path: str, method: str = "GET") -> None:
        session = async_get_clientsession(hass)
        try:
            async with session.request(method, path, timeout=aiohttp.ClientTimeout(total=5)) as resp:
                if resp.status >= 400:
                    _LOGGER.warning("UIWLED %s %s -> %s", method, path, resp.status)
        except Exception as err:
            _LOGGER.warning("UIWLED call to %s failed: %s", path, err)

    async def handle_set_port(call: ServiceCall) -> None:
        base = _resolve_base_url(hass, call.data.get("host"))
        if not base:
            _LOGGER.error("UIWLED set_port: no configured addon")
            return
        sw = call.data["switch"]
        port = int(call.data["port"])
        r, g, b = (int(v) for v in call.data["rgb_color"])
        duration_ms = int(call.data.get("duration_ms", 0))
        await _call_addon(
            f"{base}/api/port/{sw}?port={port}&r={r}&g={g}&b={b}&duration_ms={duration_ms}"
        )

    async def handle_clear_port(call: ServiceCall) -> None:
        base = _resolve_base_url(hass, call.data.get("host"))
        if not base:
            return
        sw = call.data["switch"]
        port = int(call.data["port"])
        await _call_addon(f"{base}/api/port/{sw}?port={port}", method="DELETE")

    async def handle_clear_all(call: ServiceCall) -> None:
        base = _resolve_base_url(hass, call.data.get("host"))
        if not base:
            return
        sw = call.data["switch"]
        await _call_addon(f"{base}/api/ports/{sw}", method="DELETE")

    if not hass.services.has_service(DOMAIN, SERVICE_SET_PORT):
        hass.services.async_register(DOMAIN, SERVICE_SET_PORT, handle_set_port, schema=SCHEMA_SET_PORT)
    if not hass.services.has_service(DOMAIN, SERVICE_CLEAR_PORT):
        hass.services.async_register(DOMAIN, SERVICE_CLEAR_PORT, handle_clear_port, schema=SCHEMA_CLEAR_PORT)
    if not hass.services.has_service(DOMAIN, SERVICE_CLEAR_ALL_PORTS):
        hass.services.async_register(DOMAIN, SERVICE_CLEAR_ALL_PORTS, handle_clear_all, schema=SCHEMA_CLEAR_ALL)
