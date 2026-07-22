# Architecture

Outpost is a reverse proxy for MCP traffic. Every configured upstream gets its own route (`/{upstream.name}`); each route is served by an `upstreamHandler` (`internal/proxy/proxy.go`) wired with one instance of every subsystem below. The core logic lives in `upstreamHandler.handle` — transport-independent, called by both the HTTP path (`ServeHTTP`) and the stdio wrapper mode's read loop.

## Request flow

For every `tools/call`, in this exact order (see `handle` in `internal/proxy/proxy.go`):

```
tools/call request
      │
      ▼
 1. circuit breaker (internal/breaker)  ──open──▶ reject: CircuitOpen (-32001)
      │ closed/half-open
      ▼
 2. drift-block check (internal/pinning) ──drifted + block:true──▶ reject: DriftBlocked (-32002)
      │
      ▼
 3. T1 structural validation (internal/t1) ──violation──▶ reject: InvalidParams
      │ OK (or unknown tool — fail-open)
      ▼
 4. list-op cache check (internal/cache) — tools/call is never cacheable, this step
      is always a structural no-op for it (Cache.Key refuses to mint a key)
      ▼
 5. the real upstream call (internal/upstream or internal/stdioupstream)
      │
      ▼
 6. record result: breaker.RecordResult + anomaly.Observe (internal/anomaly)
      │
      ▼
 7. response
```

`tools/list` calls skip steps 1–3 (the breaker/pinning/T1 gate is `tools/call`-only, D19) and instead run step 8 after the upstream call: `t1.LearnFromToolsList` + `pinning.LearnFromToolsList`, which is how T1's schemas and pinning's hashes get learned in the first place — from the upstream's own responses, never a separate registration step.

## Subsystems

### Proxy core (`internal/proxy`, `internal/mcp`, `internal/upstream`, `internal/logging`)
JSON-RPC 2.0 over Streamable HTTP, dual protocol-version negotiation (`2025-11-25` current, `2026-07-28` next, via the `MCP-Protocol-Version` header — `internal/mcp.NegotiateVersion`). Structured, no-payload logging (`internal/logging.LogCall` logs method/tool/duration/error only, never arguments or results). ADR: [0002-dual-protocol-support](adr/0002-dual-protocol-support.md).

### T1 — structural validation (`internal/t1`, `internal/schema`)
Synchronous JSON-Schema validation of `tools/call` arguments against the schema learned from the upstream's own `tools/list`. Fail-open for any tool it hasn't learned yet. Sub-microsecond per call even under 50-goroutine concurrent load (`internal/t1/loadtest_test.go` — real measured p99, not a single-threaded benchmark claim). The validator itself (`internal/schema`) is hand-written rather than a third-party library, with a recursion-depth guard against maliciously nested schemas (found via fuzzing, not speculation).

### Circuit breaker (`internal/breaker`)
Per-tool `tools/call` failure tripping: 5 consecutive failures (default) opens the circuit, a cooldown period, then a single half-open trial call before fully re-closing. In-memory hot path; state transitions (not every call) persisted to `internal/store` for restart visibility.

### Tool-definition pinning + drift detection (`internal/pinning`)
The rug-pull defense. Hashes the **entire** tool definition (not just `inputSchema`) on first sight — trust-on-first-sight — and flags drift on any later mismatch. Hashing the whole definition matters because a poisoned-description attack (the realistic prompt-injection vector) leaves `inputSchema` byte-identical; T1 alone would never see it. Drift is always logged; `block: true` per tool additionally rejects `tools/call` for that tool once drifted. Block state is hydrated from persisted drift history on every startup (`Pinner.Hydrate`) — without this, a process restart would silently un-block a previously-blocked tool.

### Statistical anomaly detection (`internal/anomaly`)
T2 detection-only (never blocks, ADR-0003). Welford's online algorithm tracks per-tool `tools/call` latency and error rate; flags calls more than 3 standard deviations from that tool's own history, with a zero-variance special case that catches the *first* failure after a 100%-clean streak (a naive stddev check would never fire on it, since stddev is 0 until then).

### List-op cache (`internal/cache`)
TTL cache for `tools/list`/`resources/read`, off by default (`cache_ttl_seconds` per upstream). `tools/call` is uncacheable **by construction** — `Cache.Key` structurally refuses to mint a key for that method, so a wiring mistake degrades to a cache miss, never a stale cached tool-call result.

### OpenTelemetry tracing (`internal/tracing`)
One span per proxied call, metadata-only attributes (upstream/method/tool/duration/success — never arguments or results). Exports to a swappable `io.Writer` (stdout in production via `outpost serve`) — no real OTel collector exists yet, so this is the interim export target.

### Control-plane reporter (`internal/reporter`, `internal/report`)
Fail-silent, bounded-buffer client that ships drift events to an optional hosted control plane (`control_plane_url`, off by default). A down or unreachable control plane never blocks or delays MCP traffic — proven with a real test pointing the reporter at a closed port. See [ADR-0001](adr/0001-metadata-only-control-plane-boundary.md) for what's allowed to cross this boundary (metadata only, enforced by a reflection-based test on the wire types).

### stdio wrapper mode (`internal/stdioupstream`, `cmd/outpost/run.go`)
`outpost run -- <server-cmd> [args...]` spawns the given command as a child process and speaks newline-delimited JSON-RPC to its stdin/stdout, running every request through the exact same gate as HTTP mode via `proxy.NewSingle` + `Handler.Handle` — no duplicated logic between the two transports. First-cut simplification: `Call` serializes concurrent requests with a mutex rather than correlating responses by JSON-RPC id, which is enough for one child process backing one `outpost run` invocation but not a general-purpose concurrent stdio multiplexer.

### Client shims (`clients/typescript`, `clients/python`)
Language clients that call through the proxy and fall back to a direct connection to the upstream if the proxy is unreachable — visibly (a logged warning naming the failure) and with a bounded timeout. See [`docs/if-outpost-goes-down.md`](if-outpost-goes-down.md).

### Control plane (`outpost-cloud`, separate private repo)
A one-way metadata sink: ingests pin/drift events, runs scan-on-change against a Claude-based scanner behind a swappable `Scanner` interface, and dispatches alerts on non-safe verdicts. Entirely optional — the binary works fully without it. The binary and control plane are separate Go modules; the control plane never has a code path back into proxy behavior, so a scanner verdict can never directly block traffic (spec §6) — that's a structural property of the module boundary, not a runtime check.
