# What happens if Outpost goes down

This is the first question worth answering honestly, because it's the first objection anyone evaluating a proxy sitting in front of production agent traffic will raise. Two different failure modes, two different answers.

## A feature inside Outpost fails

T1, the circuit breaker, pinning, anomaly detection, tracing, and control-plane reporting are all **fail-open** by design (ADR-0003). If any of them errors internally — a hydration failure on startup, a broken control-plane connection, a scanner outage — the proxy keeps serving traffic. The specific behaviors:

- Pinning hydration failure on startup: logged, not fatal. The proxy starts anyway, with drift-block state as if no drift had ever been seen.
- Control plane unreachable: the reporter drops events (oldest first, once its buffer fills) rather than blocking or retrying indefinitely. MCP traffic is never affected.
- Scanner outage: drift ingestion still succeeds; scanning is skipped for that event, best-effort.
- T1 sees a tool it's never learned: fails open (the call proceeds unvalidated) rather than rejecting a legitimate call it just doesn't have data on yet.

None of these ever result in blocked traffic due to an internal error. Detection features degrade to "not currently detecting," never to "currently blocking everything."

## The Outpost process itself goes down

This is the harder case, and the honest answer depends on which transport you're using.

**HTTP mode, with a client shim.** If you're using [`clients/typescript`](../clients/typescript) or [`clients/python`](../clients/python), the shim tries the proxy first and falls back to a direct connection to your upstream MCP server if the proxy is unreachable — visibly (a logged warning naming the failure and the fallback target) and with a bounded timeout, never a silent indefinite hang. Your agent keeps working. What it loses for the duration: T1 validation, drift detection, breaker protection, anomaly monitoring, and tracing — none of that runs on the direct path, because none of it exists outside Outpost.

**HTTP mode, without a shim** (calling Outpost's HTTP endpoint directly, e.g. from curl or a client that doesn't use ours): a dead Outpost process means connection refused. No fallback exists at the protocol level — the fallback is a client-side behavior, not something the proxy itself can provide once it's the thing that's down.

**stdio wrapper mode** (`outpost run -- <server-cmd>`): there is currently no fallback at all. If the wrapper process dies, whatever launched it loses its connection to the child MCP server entirely — the wrapper's whole point is standing between a caller and the child's stdio, so its own death breaks that connection by construction. This is a real, current gap, not a soft edge case: stdio mode should be considered a case where an Outpost failure is a hard failure for now.

## What this means for you

| Scenario | Agent keeps working? | Security/reliability coverage during the outage |
|---|---|---|
| Internal feature fails inside a running Outpost | Yes (fail-open) | Degraded per-feature, traffic unaffected |
| Outpost process dies, HTTP + client shim | Yes (direct fallback) | None — you're running unprotected until Outpost comes back |
| Outpost process dies, HTTP + no shim | No (connection refused) | N/A |
| Outpost process dies, stdio wrapper mode | No | N/A |

The design choice here is deliberate: Outpost is a reliability and security *layer*, not a hard dependency your agents can't function without. We'd rather your agent run briefly unprotected than be unable to run at all because a sidecar process crashed. But "unprotected" is the honest word for that window — this page exists so nobody discovers that trade-off for the first time during an actual incident.
