"""Light platform for UIWLED — one entity per configured switch."""
from __future__ import annotations

import logging
from datetime import timedelta
from typing import Any

import aiohttp
from homeassistant.components.light import (
    ATTR_BRIGHTNESS,
    ATTR_EFFECT,
    ATTR_RGB_COLOR,
    ColorMode,
    LightEntity,
    LightEntityFeature,
)
from homeassistant.config_entries import ConfigEntry
from homeassistant.core import HomeAssistant
from homeassistant.helpers.aiohttp_client import async_get_clientsession
from homeassistant.helpers.entity_platform import AddEntitiesCallback

from .const import CONF_HOST, CONF_PORT, DEFAULT_PORT, DOMAIN, POLL_INTERVAL_SECONDS

_LOGGER = logging.getLogger(__name__)

SCAN_INTERVAL = timedelta(seconds=POLL_INTERVAL_SECONDS)

# WLED-style effect list. Falls back to whatever /api/effects returns from the addon.
_DEFAULT_EFFECTS: list[dict] = [{"id": 0, "name": "Solid"}]


async def _get_json(session: aiohttp.ClientSession, url: str) -> Any:
    async with session.get(url, timeout=aiohttp.ClientTimeout(total=5)) as resp:
        resp.raise_for_status()
        return await resp.json()


async def async_setup_entry(
    hass: HomeAssistant,
    entry: ConfigEntry,
    add_entities: AddEntitiesCallback,
) -> None:
    """Fetch the addon's switch list and create one light per switch."""
    data = hass.data[DOMAIN][entry.entry_id]
    host: str = data[CONF_HOST]
    port: int = data.get(CONF_PORT, DEFAULT_PORT)
    base_url = f"http://{host}:{port}"

    session = async_get_clientsession(hass)
    try:
        switches = await _get_json(session, f"{base_url}/api/switches")
        effects = await _get_json(session, f"{base_url}/api/effects")
    except Exception as err:
        _LOGGER.error("Failed to fetch switches/effects from %s: %s", base_url, err)
        switches = []
        effects = _DEFAULT_EFFECTS

    entities = [UiwledLight(base_url, sw, effects) for sw in switches]
    add_entities(entities, update_before_add=True)


class UiwledLight(LightEntity):
    """One switch = one light. RGB + brightness + effect selection."""

    _attr_supported_color_modes = {ColorMode.RGB}
    _attr_color_mode = ColorMode.RGB
    _attr_supported_features = LightEntityFeature.EFFECT
    _attr_should_poll = True

    def __init__(self, base_url: str, sw_dto: dict, effects: list[dict]) -> None:
        self._base_url = base_url
        self._sw_name: str = sw_dto["name"]
        self._attr_unique_id = f"uiwled_{self._sw_name}"
        self._attr_name = f"UIWLED {self._sw_name}"
        self._effects_by_id: dict[int, str] = {e["id"]: e["name"] for e in effects}
        self._effects_by_name: dict[str, int] = {e["name"]: e["id"] for e in effects}
        self._attr_effect_list = [e["name"] for e in effects]

        # Attribution: seed from initial DTO if present.
        st = sw_dto.get("state") or {}
        self._is_on = bool(st.get("On", True))
        self._brightness = int(st.get("Brightness", 128))
        colors = st.get("Colors") or []
        c0 = colors[0] if colors else {}
        self._rgb = (
            int(c0.get("R", 255)),
            int(c0.get("G", 160)),
            int(c0.get("B", 0)),
        )
        self._effect = self._effects_by_id.get(int(st.get("EffectID", 0)), "Solid")

    @property
    def is_on(self) -> bool:
        return self._is_on

    @property
    def brightness(self) -> int | None:
        return self._brightness

    @property
    def rgb_color(self) -> tuple[int, int, int] | None:
        return self._rgb

    @property
    def effect(self) -> str | None:
        return self._effect

    async def async_turn_on(self, **kwargs: Any) -> None:
        session = async_get_clientsession(self.hass)
        # Order matters slightly: turn on first so subsequent writes land on an active state.
        await self._call(session, f"/api/power/{self._sw_name}?on=1")

        if ATTR_BRIGHTNESS in kwargs:
            b = int(kwargs[ATTR_BRIGHTNESS])
            await self._call(session, f"/api/brightness/{self._sw_name}?value={b}")
            self._brightness = b

        if ATTR_RGB_COLOR in kwargs:
            r, g, b = kwargs[ATTR_RGB_COLOR]
            await self._call(
                session,
                f"/api/color/{self._sw_name}?slot=0&r={int(r)}&g={int(g)}&b={int(b)}",
            )
            self._rgb = (int(r), int(g), int(b))

        if ATTR_EFFECT in kwargs:
            eff_name = kwargs[ATTR_EFFECT]
            eff_id = self._effects_by_name.get(eff_name)
            if eff_id is not None:
                await self._call(session, f"/api/effect/{self._sw_name}?id={eff_id}")
                self._effect = eff_name

        self._is_on = True
        self.async_write_ha_state()

    async def async_turn_off(self, **kwargs: Any) -> None:
        session = async_get_clientsession(self.hass)
        await self._call(session, f"/api/power/{self._sw_name}?on=0")
        self._is_on = False
        self.async_write_ha_state()

    async def async_update(self) -> None:
        """Poll the addon so external UI changes reflect in HA."""
        session = async_get_clientsession(self.hass)
        try:
            switches = await _get_json(session, f"{self._base_url}/api/switches")
        except Exception as err:
            _LOGGER.debug("UIWLED poll failed for %s: %s", self._sw_name, err)
            return
        for sw in switches:
            if sw.get("name") != self._sw_name:
                continue
            self._attr_available = bool(sw.get("online", False))
            st = sw.get("state") or {}
            self._is_on = bool(st.get("On", True))
            self._brightness = int(st.get("Brightness", 128))
            colors = st.get("Colors") or []
            if colors:
                c0 = colors[0]
                self._rgb = (
                    int(c0.get("R", 0)),
                    int(c0.get("G", 0)),
                    int(c0.get("B", 0)),
                )
            self._effect = self._effects_by_id.get(int(st.get("EffectID", 0)), self._effect)
            return

    async def _call(self, session: aiohttp.ClientSession, path: str) -> None:
        url = f"{self._base_url}{path}"
        try:
            async with session.get(url, timeout=aiohttp.ClientTimeout(total=5)) as resp:
                if resp.status >= 400:
                    _LOGGER.warning("UIWLED %s returned %s", url, resp.status)
        except Exception as err:
            _LOGGER.warning("UIWLED call to %s failed: %s", url, err)
