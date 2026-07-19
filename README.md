# Outpost

An MCP-native proxy that sits between your agents and their MCP servers: deterministic reliability controls, schema-drift and tool-tampering detection, and statistical anomaly flags — all inside your own network.

> **Status: pre-release.** We are building in the open toward `v0.9.0` (early August 2026). APIs, config format, and behaviour may change without notice until then. Watch releases if you want the launch.

## Why

If you run MCP agents in production today, three failure modes are waiting for you:

- **Silent schema breaks.** An upstream server changes a tool's definition and your agent starts failing — or worse, quietly misbehaving — with nothing in your logs that says why.
- **Tool poisoning.** Malicious instructions embedded in MCP tool metadata are demonstrated at scale: across 45 live servers and 353 tools, published research measured a 36.5% average attack success rate against evaluated agents. Model alignment does not solve this; even the best-aligned model tested complied with poisoned instructions over 97% of the time. This needs a control *outside* the model.
- **Rug pulls.** A previously-vetted tool gets silently redefined into something dangerous. If you pinned nothing, you'll never know it changed.

## What Outpost does

- **Structural validation (T1)** — synchronous, sub-millisecond schema checks on every `tools/call`, learned automatically from the upstream's own `tools/list` responses. Fail-open: a tool Outpost hasn't seen yet is never blocked. No LLM in the request path, ever.
- **Circuit breaking** — per-tool failure-rate tripping with deterministic thresholds.
- **List-operation caching** — in-process cache for `tools/list` and `resources/read`, honouring the spec's `cacheScope`. Explicitly **not** `tools/call`.
- **Schema-drift detection** — diffs tool definitions across calls and flags changes.
- **Tool-definition pinning** — SHA-256 hash of every tool definition on first sight; alert (optionally block) on an unexplained change.
- **Statistical anomaly detection (T2)** — streaming statistics (t-digest, EWMA) on per-tool latency, error rate, argument shape, and call frequency. No machine learning, no LLM.
- **OpenTelemetry export** — native trace export using the MCP spec's `_meta` trace fields.

Both MCP protocol versions `2025-11-25` and `2026-07-28` are supported concurrently.

## What Outpost will never do

We think a security tool should be explicit about its boundaries:

- **Never cache `tools/call` responses.** The MCP spec excludes them from cacheable operations, and `readOnlyHint` is not trustworthy on exactly the servers this product exists to defend against.
- **Never retry by default.** No safe general retry rule exists for tools with side effects. Retries are a per-tool, explicit opt-in — there is deliberately no global retry setting in the config schema (a test enforces this).
- **Never store or broker credentials.** Outpost forwards opaque bearer tokens between agent and server. It never mints, stores, or vaults anything.
- **Never let call payloads leave your network.** Optional hosted features receive schema hashes, tool definitions, and aggregated statistics — metadata only. Arguments, results, and credentials stay inside your boundary, always.
- **Never scan per call.** LLM-based scanning of tool definitions happens on definition *change* only, and its output is treated as untrusted input — it alerts; it does not block.

## Design principles

- **Fail-open by default.** Reliability and detection features must never take your traffic down. Clients keep a direct-connection fallback if the proxy itself is unreachable.
- **Detection with a published accuracy rate, not "prevention."** Every release ships with a false-positive/false-negative report against public corpora. We'd rather tell you our real numbers than market a shield.
- **Single static binary.** Pure Go, embedded state, no runtime dependencies. Runs as a sidecar, a standalone process, or in front of your server fleet.

## Quickstart

```bash
go build -o outpost ./cmd/outpost
cp example.outpost.yaml outpost.yaml   # edit the upstream URL(s) to match your MCP servers
./outpost serve -config outpost.yaml
```

Outpost listens on the configured address and exposes one route per upstream, at `/{upstream-name}`. Point your MCP client at `http://<listen-addr>/<upstream-name>` instead of the upstream directly.

This is pre-`v0.9.0` — drift detection, pinning, circuit breaking, and anomaly detection aren't wired in yet. Today's binary proxies, negotiates protocol version, and structurally validates `tools/call` arguments against schemas it's learned; it doesn't yet detect tampering or anomalies.

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md). Security reports: [SECURITY.md](SECURITY.md) — please don't open public issues for vulnerabilities.

## License

[Apache-2.0](LICENSE) © Korneza Solutions Private Limited
