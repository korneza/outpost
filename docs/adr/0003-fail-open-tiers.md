# ADR-0003: Fail-open tiers

**Status:** Accepted · 2026-07-17

## Context

A reliability proxy that causes outages has negative value. Every feature must have an explicit answer to "what happens when this feature itself breaks?"

## Decision

| Tier | Features | Failure behaviour |
|---|---|---|
| T0 | list-op cache, circuit breaker | Fail-open, no override |
| T1 | structural validation | Fail-open, no override |
| T2 | anomaly detection, definition scanning | Fail-open; per-tool fail-closed reserved for a later phase, only for tools declaring `destructiveHint: true` |

Additionally: client integrations must fall back to a **direct connection** to the MCP server if the Outpost process is unreachable, with a bounded timeout and a visible log line. Scanner/LLM output is untrusted input and never directly drives blocking behaviour — it produces alerts through a strictly validated verdict schema.

## Consequences

- Outpost is positioned as detection with published accuracy, not "prevention" — and the failure semantics match the positioning.
- Blocking behaviour, where configured (e.g. pinning's optional block mode), is driven only by deterministic checks, never by model output.
