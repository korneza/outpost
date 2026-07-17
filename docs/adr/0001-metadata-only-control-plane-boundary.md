# ADR-0001: Metadata-only control-plane boundary

**Status:** Accepted · 2026-07-17

## Context

Outpost's binary runs inside the customer's network. Optional hosted features (fleet dashboard, tool-definition scanning, alerting) run elsewhere. Anything that crosses that boundary is a trust decision: production tool-call traffic contains customer data, credentials, and business logic.

## Decision

Only three kinds of data may ever cross from the binary to any hosted component:

1. SHA-256 hashes of tool definitions
2. Tool definitions themselves (near-public metadata, not customer data)
3. Aggregated numeric statistics — call counts, latency percentiles, error rates

Tool-call **arguments**, **results**, and **credentials** never cross, in any form, at any sampling rate. If a future feature appears to need payload data at a hosted component, the feature moves into the binary instead — the boundary does not widen.

## Consequences

- The reporter's wire types are an allowlist; a CI boundary test fails if a payload-shaped field is ever added (test ships with the reporter package).
- Hosted components can be operated with a far smaller compliance surface, and self-hosting customers can verify the claim by reading one package.
- Some fleet features are harder to build than they would be with raw payloads. That trade is intentional and permanent.
