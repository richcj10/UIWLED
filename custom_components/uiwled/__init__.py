"""UIWLED integration — hub for the UIWLED addon's REST API."""
from __future__ import annotations

import logging
from typing import Any

import voluptuous as vol
from homeassistant.config_entries import ConfigEntry
from homeassistant.const import Platform
from homeassistant.core import HomeAssistant, ServiceCall
from homeassistant.helpers import config_validation as cv

from .const import (
    ATTR_DURATION_MS,
    ATTR_PORT,
    ATTR_RGB,
    ATTR_SWITCH,
    CONF_HOST,
    CONF_PORT,
    DOMAIN,
    SERVICE_CLEAR_ALL_PORTS,
    SERVICE_CLEAR_PORT,
    SERVICE_SET_PORT,
)
from .coordinator import UIWLEDClient, UIWLEDCoordinator

_LOGGER = logging.getLogger(__name__)

PLATFORMS: list[Platform] = [Platform.LIGHT]


async def async_setup_entry(hass: HomeAssistant, entry: ConfigEntry) -> bool:
    """Set up a UIWLED hub from a config entry."""
    client = UIWLEDClient(hass, entry.data[CONF_HOST], entry.data[CONF_PORT])
    coordinator = UIWLEDCoordinator(hass, client)
    await coordinator.async_config_entry_first_refresh()

    hass.data.setdefault(DOMAIN, {})[entry.entry_id] = coordinator

    await hass.config_entries.async_forward_entry_setups(entry, PLATFORMS)
    _register_services(hass)
    return True


async def async_unload_entry(hass: HomeAssistant, entry: ConfigEntry) -> bool:
    """Unload a config entry."""
    ok = await hass.config_entries.async_unload_platforms(entry, PLATFORMS)
    if ok:
        hass.data[DOMAIN].pop(entry.entry_id, None)
        if not hass.data[DOMAIN]:
            for svc in (SERVICE_SET_PORT, SERVICE_CLEAR_PORT, SERVICE_CLEAR_ALL_PORTS):
                hass.services.async_remove(DOMAIN, svc)
    return ok


def _coordinators(hass: HomeAssistant) -> list[UIWLEDCoordinator]:
    return list(hass.data.get(DOMAIN, {}).values())


def _pick_coordinator(hass: HomeAssistant, switch_name: str) -> UIWLEDCoordinator | None:
    for c in _coordinators(hass):
        if c.data and switch_name in c.data:
            return c
    return None


def _register_services(hass: HomeAssistant) -> None:
    if hass.services.has_service(DOMAIN, SERVICE_SET_PORT):
        return

    set_port_schema = vol.Schema(
        {
            vol.Required(ATTR_SWITCH): cv.string,
            vol.Required(ATTR_PORT): vol.All(int, vol.Range(min=1, max=1024)),
            vol.Required(ATTR_RGB): vol.All(list, vol.Length(min=3, max=3), [vol.All(int, vol.Range(min=0, max=255))]),
            vol.Optional(ATTR_DURATION_MS, default=0): vol.All(int, vol.Range(min=0)),
        }
    )
    clear_port_schema = vol.Schema(
        {
            vol.Required(ATTR_SWITCH): cv.string,
            vol.Required(ATTR_PORT): vol.All(int, vol.Range(min=1, max=1024)),
        }
    )
    clear_all_schema = vol.Schema({vol.Required(ATTR_SWITCH): cv.string})

    async def handle_set_port(call: ServiceCall) -> None:
        name = call.data[ATTR_SWITCH]
        coord = _pick_coordinator(hass, name)
        if coord is None:
            _LOGGER.warning("uiwled.set_port: unknown switch %s", name)
            return
        r, g, b = call.data[ATTR_RGB]
        await coord.client.set_port_override(name, call.data[ATTR_PORT], r, g, b, call.data[ATTR_DURATION_MS])

    async def handle_clear_port(call: ServiceCall) -> None:
        name = call.data[ATTR_SWITCH]
        coord = _pick_coordinator(hass, name)
        if coord is None:
            return
        await coord.client.clear_port_override(name, call.data[ATTR_PORT])

    async def handle_clear_all(call: ServiceCall) -> None:
        name = call.data[ATTR_SWITCH]
        coord = _pick_coordinator(hass, name)
        if coord is None:
            return
        await coord.client.clear_all_port_overrides(name)

    hass.services.async_register(DOMAIN, SERVICE_SET_PORT, handle_set_port, schema=set_port_schema)
    hass.services.async_register(DOMAIN, SERVICE_CLEAR_PORT, handle_clear_port, schema=clear_port_schema)
    hass.services.async_register(DOMAIN, SERVICE_CLEAR_ALL_PORTS, handle_clear_all, schema=clear_all_schema)
