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

# Generous headroom for a real tool-call result (including a modest
# embedded file or image) while still bounding the worst case. This
# matters most on the direct-fallback path specifically: that path
# talks to the upstream with none of Outpost's own protections in
# effect, so a malicious or compromised upstream there is unconstrained
# except by what this client does itself.
_DEFAULT_MAX_RESPONSE_BYTES = 10 * 1024 * 1024


class OutpostClient:
    def __init__(
        self,
        proxy_url: str,
        direct_url: str,
        timeout_seconds: float = 3.0,
        max_response_bytes: int = _DEFAULT_MAX_RESPONSE_BYTES,
    ) -> None:
        self.proxy_url = proxy_url
        self.direct_url = direct_url
        self.timeout_seconds = timeout_seconds
        self.max_response_bytes = max_response_bytes

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
            raw = _read_bounded(resp, self.max_response_bytes)
            return json.loads(raw.decode("utf-8"))


def _read_bounded(resp: Any, max_bytes: int) -> bytes:
    """Reads resp in chunks, raising rather than buffering past max_bytes.

    resp.read() with no argument reads the whole body in one call,
    trusting the far end not to send something absurd. The upstream
    (direct or through the proxy) is untrusted by this client's own
    threat model, so this reads in bounded chunks instead and stops the
    moment the total would exceed max_bytes.
    """
    chunks: list[bytes] = []
    total = 0
    while True:
        chunk = resp.read(65536)
        if not chunk:
            break
        total += len(chunk)
        if total > max_bytes:
            raise ValueError(f"response exceeds {max_bytes} byte limit")
        chunks.append(chunk)
    return b"".join(chunks)
