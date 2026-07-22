"""Python client for MCP agents that talk to servers through Outpost.

Sends every call through a local Outpost proxy; if the proxy is
unreachable, falls back to a direct connection to the upstream MCP
server -- spec's v1 requirement that a dead Outpost process must never
take an agent down with it. The fallback is visible (a warnings.warn
naming the failure and the fallback target) and bounded
(timeout_seconds per attempt), matching clients/typescript's design
exactly. Standard library only -- no third-party dependencies.
"""
from __future__ import annotations

import json
import urllib.request
import warnings
from typing import Any

__all__ = ["OutpostClient"]


class OutpostClient:
    def __init__(self, proxy_url: str, direct_url: str, timeout_seconds: float = 3.0) -> None:
        self.proxy_url = proxy_url
        self.direct_url = direct_url
        self.timeout_seconds = timeout_seconds

    def call(self, request: dict[str, Any]) -> dict[str, Any]:
        try:
            return self._post_json(self.proxy_url, request)
        except Exception as err:  # noqa: BLE001 - any transport failure triggers the fallback
            warnings.warn(
                f"[outpost-client] proxy at {self.proxy_url} unreachable ({err}); "
                f"falling back to direct connection at {self.direct_url}",
                stacklevel=2,
            )
            return self._post_json(self.direct_url, request)

    def _post_json(self, url: str, request: dict[str, Any]) -> dict[str, Any]:
        body = json.dumps(request).encode("utf-8")
        req = urllib.request.Request(
            url, data=body, headers={"Content-Type": "application/json"}, method="POST"
        )
        with urllib.request.urlopen(req, timeout=self.timeout_seconds) as resp:
            return json.loads(resp.read().decode("utf-8"))
