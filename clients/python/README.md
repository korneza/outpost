# korneza-outpost-client

Python client for MCP agents that talk to servers through [Outpost](https://github.com/korneza/outpost). Sends every call through your local Outpost proxy; if the proxy is unreachable, falls back to a direct connection to the upstream MCP server — visibly (a `warnings.warn` naming the failure) and with a bounded timeout, never a silent hang. Mirrors the [TypeScript shim](../typescript) exactly.

## Install

```bash
pip install korneza-outpost-client
```

## Usage

```python
from outpost_client import OutpostClient

client = OutpostClient(
    proxy_url="http://127.0.0.1:8080/my-upstream",
    direct_url="https://my-mcp-server.example.com",
    timeout_seconds=3.0,  # optional, defaults to 3.0
)

response = client.call({
    "jsonrpc": "2.0",
    "id": 1,
    "method": "tools/list",
})
```

## Why the fallback exists

Outpost is a reliability and security layer, not a single point of failure. If the Outpost process itself goes down, agents using this client keep working — just without T1 validation, drift detection, or anomaly monitoring until Outpost comes back. See [`docs/if-outpost-goes-down.md`](../../docs/if-outpost-goes-down.md) in the main repo for the full picture.

## Status

Pre-`v0.9.0` — this package tracks the main `outpost` repo's pre-release status. Not yet published to PyPI (needs the org's PyPI account, an open item — see the 30-day build plan).
