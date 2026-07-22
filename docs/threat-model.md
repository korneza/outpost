# Threat model

## What Outpost defends against

**Rug pulls.** A tool your agent has already been using gets silently redefined into something dangerous. Outpost hashes every tool's *entire* definition (name, description, `inputSchema` — everything, not just the schema) the first time it sees it, and flags any later mismatch. Hashing the whole definition, not just `inputSchema`, is deliberate: a poisoned-description attack — the realistic prompt-injection vector, where hidden instructions get embedded in a tool's description — leaves the schema byte-identical, so a schema-only check would miss it entirely. Operators can opt individual tools into `block: true`, rejecting `tools/call` outright once drift is detected on that tool.

**Tool poisoning / prompt injection via tool metadata.** The same pinning/drift mechanism above is the primary defense — it catches the definition changing. The optional hosted scanner (Claude-based, `outpost-cloud`) is a second, alert-only layer on top: every drifted definition gets scanned once for signs of tampering, and a non-`safe` verdict dispatches an alert. The scanner's verdict is *never* a code path back into proxy behavior — the binary (`outpost`) and the control plane (`outpost-cloud`) are separate Go modules, and the binary never imports anything from the control plane. That's a structural guarantee, not a runtime check: there is no code path for a scanner verdict to block a call even if someone wanted it to.

**Structurally invalid or malicious `tools/call` arguments.** T1 validates every `tools/call`'s arguments against the schema learned from the upstream's own `tools/list`, synchronously, before the call reaches the upstream. Includes a recursion-depth guard against maliciously deeply-nested schemas (found by fuzzing a malicious-upstream scenario, not a hypothetical).

**Cascading failures from a misbehaving tool.** The circuit breaker trips after repeated consecutive failures on a specific tool, so a broken or hostile upstream tool can't be hammered indefinitely.

**Anomalous behavior that doesn't trip any explicit rule.** T2 statistical anomaly detection flags `tools/call` latency or error-rate outliers (>3 standard deviations from that tool's own history) — alert-only, catches things like a tool that suddenly starts taking 50x longer (possible sign of exfiltration, resource exhaustion, or a compromised upstream) without anyone having to define a rule for that specific case in advance.

## What Outpost explicitly does not do

- **Prevent, beyond structural/schema validation and opt-in drift blocking.** T2 anomaly detection and the scanner are detection-only by design (ADR-0003) — "detection, not prevention" is the product's positioning, not a current limitation to be fixed later.
- **Scan every call.** Scanning is scan-on-change only, deduplicated by definition hash — a given definition is scanned at most once, ever. There is no code path for per-call scanning to exist (spec §2.2's "impossible by construction" framing).
- **Broker, mint, or store credentials.** Outpost forwards opaque bearer tokens between agent and server unchanged. It never inspects, stores, or vaults them.
- **Let call payloads leave the customer's network.** The optional control-plane integration receives schema hashes, tool definitions, and aggregated statistics only — enforced by a reflection-based test on the wire types (ADR-0001), not just a design intent.
- **Retry by default.** No safe general retry rule exists for tools with side effects — retries are a per-tool, explicit opt-in, with no global retry setting in the config schema at all (enforced by a test).
- **Cache `tools/call` responses**, ever, under any configuration — the cache package structurally cannot mint a cache key for that method.

## Real findings from our own security review

A dedicated review (automated static checks + adversarial reasoning against Outpost's own controls, not just pattern-matching known-bad code) found and fixed two real gaps before this doc was written:

1. **Bearer-token forwarding was documented but not implemented.** The client-facing `Authorization` header was never actually read and forwarded to the upstream — a real functional/trust-boundary gap (fail-closed, not a leak: a real deployment requiring upstream auth would have simply broken, not exposed anything). Fixed and covered by an integration test proving the header survives the full proxy round-trip.
2. **Drift-block state didn't survive a process restart.** `Pinner`'s block state was purely in-memory; a restart silently un-blocked a previously-blocked tool until the next `tools/list` happened to re-detect the same, already-known drift — and many MCP clients only call `tools/list` once per session, so the bypass window could last an entire session. This was the higher-severity finding, and it came from adversarially attacking Outpost's own drift-blocking feature, not from automated scanning. Fixed: block state is now rehydrated from persisted drift history on every startup, verified with a real simulated-restart test (seed a store, close it, reopen fresh, confirm the very first call is blocked with zero `tools/list` calls in between).

Static scanning alone (dependency vulnerabilities, secret leaks, injection patterns, supply-chain pinning — all clean) would have missed both. The lesson we took from this, and apply to every review since: adversarially trying to defeat a security product's *own* controls finds different bugs than pattern-matching known-bad code, and both passes are necessary.

## What this threat model does not cover yet

- **A professional external penetration test.** This review, however thorough, is not a substitute for one — recommended before any GA security claim, noted here as a business/timeline decision rather than an engineering gap.
- **Production infrastructure security** (the hosted control plane's real deployment, once it exists — currently code-only, nothing deployed).
- **Supply-chain attacks on Outpost's own dependencies** beyond `govulncheck` (checks known CVEs) and pinned CI action SHAs — no deeper dependency-provenance verification yet.
