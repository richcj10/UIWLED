"""Light platform — one light entity per configured switch."""
from __future__ import annotations

from typing import Any

from homeassistant.components.light import (
    ATTR_BRIGHTNESS,
    ATTR_EFFECT,
    ATTR_RGB_COLOR,
    ColorMode,
    LightEntity,
    LightEntityFeature,
)
from homeassistant.config_entries import ConfigEntry
from homeassistant.core import HomeAssistant, callback
from homeassistant.helpers.device_registry import DeviceInfo
from homeassistant.helpers.entity_platform import AddEntitiesCallback
from homeassistant.helpers.update_coordinator import CoordinatorEntity

from .const import DOMAIN
from .coordinator import UIWLEDCoordinator


async def async_setup_entry(
    hass: HomeAssistant, entry: ConfigEntry, async_add_entities: AddEntitiesCallback
) -> None:
    coordinator: UIWLEDCoordinator = hass.data[DOMAIN][entry.entry_id]

    known: set[str] = set()

    @callback
    def _add_new() -> None:
        new: list[UIWLEDLight] = []
        for name in (coordinator.data or {}):
            if name not in known:
                known.add(name)
                new.append(UIWLEDLight(coordinator, entry.entry_id, name))
        if new:
            async_add_entities(new)

    _add_new()
    entry.async_on_unload(coordinator.async_add_listener(_add_new))


class UIWLEDLight(CoordinatorEntity[UIWLEDCoordinator], LightEntity):
    """One `light.` entity per UIWLED switch."""

    _attr_has_entity_name = True
    _attr_name = None
    _attr_supported_color_modes = {ColorMode.RGB}
    _attr_color_mode = ColorMode.RGB
    _attr_supported_features = LightEntityFeature.EFFECT

    def __init__(self, coordinator: UIWLEDCoordinator, entry_id: str, switch_name: str) -> None:
        super().__init__(coordinator)
        self._switch_name = switch_name
        self._attr_unique_id = f"{entry_id}_{switch_name}"
        self._attr_device_info = DeviceInfo(
            identifiers={(DOMAIN, self._attr_unique_id)},
            manufacturer="Ubiquiti (via UIWLED)",
            name=f"UIWLED {switch_name}",
        )

    @property
    def _sw(self) -> dict[str, Any] | None:
        return (self.coordinator.data or {}).get(self._switch_name)

    @property
    def _state(self) -> dict[str, Any] | None:
        sw = self._sw
        return sw.get("state") if sw else None

    @property
    def available(self) -> bool:
        sw = self._sw
        return bool(sw and sw.get("online"))

    @property
    def is_on(self) -> bool | None:
        st = self._state
        return bool(st.get("On")) if st else None

    @property
    def brightness(self) -> int | None:
        st = self._state
        return int(st.get("Brightness", 0)) if st else None

    @property
    def rgb_color(self) -> tuple[int, int, int] | None:
        st = self._state
        if not st:
            return None
        colors = st.get("Colors") or []
        if not colors:
            return None
        c = colors[0]
        return int(c.get("R", 0)), int(c.get("G", 0)), int(c.get("B", 0))

    @property
    def effect_list(self) -> list[str]:
        return sorted(self.coordinator.effects_by_name.keys())

    @property
    def effect(self) -> str | None:
        st = self._state
        if not st:
            return None
        return self.coordinator.effects_by_id.get(int(st.get("EffectID", 0)))

    async def async_turn_on(self, **kwargs: Any) -> None:
        client = self.coordinator.client
        name = self._switch_name
        if ATTR_RGB_COLOR in kwargs:
            r, g, b = kwargs[ATTR_RGB_COLOR]
            await client.set_color(name, r, g, b)
        if ATTR_BRIGHTNESS in kwargs:
            await client.set_brightness(name, int(kwargs[ATTR_BRIGHTNESS]))
        if ATTR_EFFECT in kwargs:
            fx_id = self.coordinator.effects_by_name.get(kwargs[ATTR_EFFECT])
            if fx_id is not None:
                await client.set_effect(name, fx_id)
        await client.set_power(name, True)
        await self.coordinator.async_request_refresh()

    async def async_turn_off(self, **kwargs: Any) -> None:
        await self.coordinator.client.set_power(self._switch_name, False)
        await self.coordinator.async_request_refresh()
