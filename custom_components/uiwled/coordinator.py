"""Polling coordinator and thin REST client for the UIWLED addon."""
from __future__ import annotations

from datetime import timedelta
from typing import Any

import aiohttp
from homeassistant.core import HomeAssistant
from homeassistant.helpers.aiohttp_client import async_get_clientsession
from homeassistant.helpers.update_coordinator import DataUpdateCoordinator, UpdateFailed

from .const import DOMAIN, UPDATE_INTERVAL_SECONDS


class UIWLEDClient:
    """Async REST client for the UIWLED addon's HTTP API."""

    def __init__(self, hass: HomeAssistant, host: str, port: int) -> None:
        self._session = async_get_clientsession(hass)
        self._base = f"http://{host}:{port}"

    async def _get(self, path: str, **params: Any) -> Any:
        url = f"{self._base}{path}"
        async with self._session.get(url, params=params, timeout=aiohttp.ClientTimeout(total=10)) as resp:
            resp.raise_for_status()
            if resp.content_type == "application/json":
                return await resp.json()
            return await resp.text()

    async def _delete(self, path: str, **params: Any) -> Any:
        url = f"{self._base}{path}"
        async with self._session.delete(url, params=params, timeout=aiohttp.ClientTimeout(total=10)) as resp:
            resp.raise_for_status()
            if resp.content_type == "application/json":
                return await resp.json()
            return await resp.text()

    async def list_switches(self) -> list[dict[str, Any]]:
        return await self._get("/api/switches")

    async def list_effects(self) -> list[dict[str, Any]]:
        return await self._get("/api/effects")

    async def set_power(self, name: str, on: bool) -> None:
        await self._get(f"/api/power/{name}", on="1" if on else "0")

    async def set_brightness(self, name: str, value: int) -> None:
        await self._get(f"/api/brightness/{name}", value=value)

    async def set_color(self, name: str, r: int, g: int, b: int, slot: int = 0) -> None:
        await self._get(f"/api/color/{name}", r=r, g=g, b=b, slot=slot)

    async def set_effect(self, name: str, effect_id: int) -> None:
        await self._get(f"/api/effect/{name}", id=effect_id)

    async def set_port_override(self, name: str, port: int, r: int, g: int, b: int, duration_ms: int = 0) -> None:
        await self._get(f"/api/port/{name}", port=port, r=r, g=g, b=b, duration_ms=duration_ms)

    async def clear_port_override(self, name: str, port: int) -> None:
        await self._delete(f"/api/port/{name}", port=port)

    async def clear_all_port_overrides(self, name: str) -> None:
        await self._delete(f"/api/ports/{name}")


class UIWLEDCoordinator(DataUpdateCoordinator[dict[str, dict[str, Any]]]):
    """Polls /api/switches on an interval; caches /api/effects once."""

    def __init__(self, hass: HomeAssistant, client: UIWLEDClient) -> None:
        super().__init__(
            hass,
            logger=hass.data.setdefault(DOMAIN, {}).get("_logger") or __import__("logging").getLogger(__name__),
            name=DOMAIN,
            update_interval=timedelta(seconds=UPDATE_INTERVAL_SECONDS),
        )
        self.client = client
        self.effects_by_id: dict[int, str] = {}
        self.effects_by_name: dict[str, int] = {}

    async def _async_update_data(self) -> dict[str, dict[str, Any]]:
        try:
            if not self.effects_by_id:
                effects = await self.client.list_effects()
                for e in effects or []:
                    self.effects_by_id[int(e["id"])] = e["name"]
                    self.effects_by_name[e["name"]] = int(e["id"])
            switches = await self.client.list_switches()
        except Exception as err:  # noqa: BLE001
            raise UpdateFailed(f"UIWLED API error: {err}") from err
        return {sw["name"]: sw for sw in switches}
