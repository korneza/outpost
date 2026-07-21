# @korneza/outpost-client

TypeScript client for MCP agents that talk to servers through [Outpost](https://github.com/korneza/outpost). Sends every call through your local Outpost proxy; if the proxy is unreachable, falls back to a direct connection to the upstream MCP server — visibly (a console warning naming the failure) and with a bounded timeout, never a silent hang.

## Install

```bash
npm install @korneza/outpost-client
```

## Usage

```typescript
import { OutpostClient } from "@korneza/outpost-client";

const client = new OutpostClient({
  proxyUrl: "http://127.0.0.1:8080/my-upstream",
  directUrl: "https://my-mcp-server.example.com",
  timeoutMs: 3000, // optional, defaults to 3000
});

const response = await client.call({
  jsonrpc: "2.0",
  id: 1,
  method: "tools/list",
});
```

## Why the fallback exists

Outpost is a reliability and security layer, not a single point of failure. If the Outpost process itself goes down, agents using this client keep working — just without T1 validation, drift detection, or anomaly monitoring until Outpost comes back. That trade-off is deliberate (see the main repo's fail-open design principles) and this client makes the trade-off visible rather than hiding it.

## Status

Pre-`v0.9.0` — this package tracks the main `outpost` repo's pre-release status. Not yet published to npm (needs the org's npm account, an open item — see the 30-day build plan).
