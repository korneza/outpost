# 5-minute quickstart

## Prerequisites

- Go 1.26+ (to build from source — no released binary exists yet, see the main [README](../README.md)'s status note)
- An MCP server to proxy to. This guide uses a placeholder at `http://127.0.0.1:9000/mcp` — swap in your own.

## 1. Build

```bash
git clone https://github.com/korneza/outpost.git
cd outpost
go build -o outpost ./cmd/outpost
```

## 2. Configure

Copy the example config and edit the upstream URL:

```bash
cp example.outpost.yaml outpost.yaml
```

`example.outpost.yaml` as shipped:

```yaml
# Minimal Outpost configuration. Retries are OFF unless explicitly enabled
# per tool below — there is deliberately no global retry setting.
listen: "127.0.0.1:8100"
upstreams:
  - name: files
    url: "http://127.0.0.1:9000/mcp"
# tools:
#   files.read:            # opt-in, per tool only
#     retry:
#       max_attempts: 3
#       initial_backoff_ms: 100
```

Point `upstreams[0].url` at your real MCP server and change `name` to whatever you want the route to be called — it becomes the path segment clients connect to (see step 3).

Every config field is documented in [`docs/configuration.md`](configuration.md).

## 3. Run

```bash
./outpost serve -config outpost.yaml
```

Outpost listens on `listen` (`127.0.0.1:8100` in the example) and exposes one route per configured upstream, at `/{upstream.name}` — `/files` for the example config above — plus a fixed `GET /healthz`.

Point your MCP client at `http://127.0.0.1:8100/files` instead of your upstream server directly.

## 4. Verify

Health check:

```bash
curl http://127.0.0.1:8100/healthz
# {"status":"ok"}
```

A real MCP call, proxied through to your upstream:

```bash
curl -X POST http://127.0.0.1:8100/files \
  -H "Content-Type: application/json" \
  -d '{"jsonrpc":"2.0","id":1,"method":"tools/list"}'
```

You should get back your upstream's real `tools/list` response, unchanged — Outpost is transparent on the happy path. The first `tools/list` also pins every tool's definition hash (trust-on-first-sight); a later call with a changed definition gets logged as drift.

Try a malformed `tools/call` (once your upstream's schema has been learned from a `tools/list` call) to see T1 reject it before your upstream ever sees it:

```bash
curl -X POST http://127.0.0.1:8100/files \
  -H "Content-Type: application/json" \
  -d '{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"a-real-tool-name","arguments":{"wrong_field": true}}}'
```

## 5. What just happened

Every `tools/call` you send through Outpost passes through, in order: circuit breaker → drift-block check → T1 structural validation → (cache check, for `tools/list`/`resources/read` only) → the real upstream call → breaker/anomaly recording → (drift learning, for `tools/list`). None of this is visible on the happy path — that's the point. See [`docs/architecture.md`](architecture.md) for the full picture, and [`docs/if-outpost-goes-down.md`](if-outpost-goes-down.md) for what happens if the Outpost process itself dies.

## Other run modes

- **stdio wrapper mode** — run Outpost as a wrapper around a child MCP server that speaks stdio instead of HTTP: `outpost run -- <server-cmd> [args...]`. Same gate logic, different transport.
- **TypeScript / Python client shims** — if you're calling Outpost from code rather than curl, [`clients/typescript`](../clients/typescript) and [`clients/python`](../clients/python) both give you automatic fallback to a direct connection if the Outpost process is unreachable.
