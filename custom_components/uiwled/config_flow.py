"""Config flow for UIWLED."""
from __future__ import annotations

import logging
from typing import Any

import aiohttp
import voluptuous as vol
from homeassistant import config_entries
from homeassistant.data_entry_flow import FlowResult
from homeassistant.helpers.aiohttp_client import async_get_clientsession

from .const import CONF_HOST, CONF_PORT, DEFAULT_PORT, DOMAIN

_LOGGER = logging.getLogger(__name__)


async def _probe(hass, host: str, port: int) -> bool:
    """Return True if the UIWLED addon answers on this host/port."""
    url = f"http://{host}:{port}/api/switches"
    session = async_get_clientsession(hass)
    try:
        async with session.get(url, timeout=aiohttp.ClientTimeout(total=5)) as resp:
            if resp.status != 200:
                return False
            payload = await resp.json()
            return isinstance(payload, list)
    except Exception as err:
        _LOGGER.debug("UIWLED probe failed for %s:%s — %s", host, port, err)
        return False


class UiwledConfigFlow(config_entries.ConfigFlow, domain=DOMAIN):
    """Handle the config flow for UIWLED."""

    VERSION = 1

    async def async_step_user(self, user_input: dict[str, Any] | None = None) -> FlowResult:
        errors: dict[str, str] = {}
        if user_input is not None:
            host = user_input[CONF_HOST].strip()
            port = user_input.get(CONF_PORT, DEFAULT_PORT)

            # Prevent duplicate entries for the same addon.
            await self.async_set_unique_id(f"{host}:{port}")
            self._abort_if_unique_id_configured()

            if await _probe(self.hass, host, port):
                return self.async_create_entry(
                    title=f"UIWLED @ {host}",
                    data={CONF_HOST: host, CONF_PORT: port},
                )
            errors["base"] = "cannot_connect"

        schema = vol.Schema(
            {
                vol.Required(CONF_HOST, default="localhost"): str,
                vol.Optional(CONF_PORT, default=DEFAULT_PORT): int,
            }
        )
        return self.async_show_form(step_id="user", data_schema=schema, errors=errors)
