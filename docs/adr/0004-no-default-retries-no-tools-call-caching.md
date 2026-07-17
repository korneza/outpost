# ADR-0004: No default retries; no `tools/call` caching

**Status:** Accepted · 2026-07-17

## Context

Generic proxies retry and cache. An MCP proxy that does either by default is dangerous:

- A `tools/call` may be a payment, an email, a deletion. If the call succeeded but the response was lost, a retry executes the side effect twice — attributed to Outpost.
- The MCP spec deliberately excludes `tools/call` from cacheable operations. `readOnlyHint` is advisory metadata supplied by the server — untrustworthy on exactly the malicious or sloppy servers Outpost exists to defend against.

## Decision

1. **Retries are never enabled by default, for any tool, under any configuration.** The config schema has no global retry setting (enforced by a reflection test in `internal/config`); retries exist only as an explicit per-tool opt-in for operators who know a specific tool is safe to retry.
2. **`tools/call` responses are never cached.** Caching applies only to `tools/list` and `resources/read`, honouring the spec's `cacheScope`. The cache layer rejects `tools/call` by construction (enforced by a permanent test when the cache package lands).

## Consequences

- Out of the box, Outpost never amplifies side effects and never serves stale tool results.
- Operators who opt a tool into retries own that judgement; the config stanza is deliberately verbose enough that it can't happen by accident.
