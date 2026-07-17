# ADR-0002: Dual MCP protocol support

**Status:** Accepted · 2026-07-17

## Context

MCP `2025-11-25` is the current stable protocol. The `2026-07-28` release removes session-affinity requirements, mandates `Mcp-Method` / `Mcp-Name` routing headers on every request, and standardises W3C Trace Context in `_meta`. Real fleets will straddle both for a long time.

## Decision

Support `2025-11-25` and `2026-07-28` concurrently for a minimum 12-month deprecation window. `2026-07-28` is the **primary** target: it needs no shared session store and scales behind a plain load balancer. `2025-11-25` clients are served through a compatibility shim, not a parallel implementation.

## Consequences

- A version-negotiation layer sits at the proxy edge; feature code is written against the `2026-07-28` shapes.
- Until the final `2026-07-28` text ships (28 Jul 2026), we track the release candidate and re-run conformance on final publication.
- Dropping `2025-11-25` before mid-2027 requires revisiting this ADR.
