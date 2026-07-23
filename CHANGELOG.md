# Changelog

All notable changes to Outpost are documented here. The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and versions follow [SemVer](https://semver.org/) (pre-1.0: minor bumps may break).

## [Unreleased]

## [0.9.0-alpha] - 2026-07-23

### Added

- Scan-on-change pipeline (`outpost-cloud/internal/scanner`): drifted tool definitions are scanned once (dedup by hash) via a swappable `Scanner` interface; `ClaudeScanner` calls the Anthropic Messages API and rejects free-text or out-of-enum model responses rather than coercing them into a verdict. Alert dispatch (`internal/alerting`) via `SlackAlerter` + a rate-limiting decorator, both off by default
- Repos pushed to GitHub (`korneza/outpost`, `korneza/outpost-cloud`, `korneza/internal`); CI is live and green across all 9 jobs (lint, race-enabled tests, 4× cross-compile, govulncheck, gitleaks, license check) — the project's first fully-green CI run
- `GET /healthz` on the proxy (liveness/readiness probes)
- `outpost run -- <server-cmd> [args...]`: stdio wrapper mode — spawns a child MCP server and proxies its stdio through the same T1/breaker/pinning/cache/anomaly/reporter gate as HTTP mode, via a new `caller` interface and transport-agnostic `handle()` (both behavior-preserving refactors, verified by the unchanged existing test suite)
- `internal/stdioupstream`: a `caller` backed by a spawned child process speaking newline-delimited JSON-RPC over stdio
- TypeScript client shim (`clients/typescript/`, `@korneza/outpost-client`): direct-connection fallback if the proxy is unreachable, visible (console warning) and bounded (timeout), verified with a test that kills the proxy mid-run — zero runtime dependencies
- `internal/chaos` + `cmd/soaktest`: a deterministic, seeded chaotic upstream (slow responses, malformed bodies, dropped connections) for resilience testing; a bounded 30-second proof runs in the normal test suite, `cmd/soaktest` is the real tool for an actual long-duration soak
- Concurrent load tests published: T1's `Check` p99 stays sub-millisecond under 50-goroutine/100k-call concurrency; the reporter's buffer stays bounded under sustained concurrent load against an unreachable control plane
- Deploy-ready (not deployed) packaging: a Kubernetes manifest (`deploy/k8s/`) and a Homebrew formula (`packaging/homebrew/`) — both explicitly documented as not-yet-usable pending a real GitHub Release and, for Homebrew, a separate tap repo
- Python client shim (`clients/python/`, `korneza-outpost-client`): mirrors the TypeScript shim exactly — direct-connection fallback, visible warning, bounded timeout, stdlib only
- Optional API-key auth on the control plane's `/v1/ingest/*` endpoints (`CONTROL_PLANE_API_KEY`, off by default; `/healthz` always stays open for probes) — a real step toward, not the final state of, "safe to expose publicly"
- Docs: [quickstart](docs/quickstart.md), [architecture](docs/architecture.md), [configuration reference](docs/configuration.md), [threat model](docs/threat-model.md), [what happens if Outpost goes down](docs/if-outpost-goes-down.md) — the five pages named explicitly in the 30-day plan's Week-3 scope
- `outpost` CLI: extracted a testable `run()` dispatcher (`cmd/outpost/main.go`) and made `runServe`'s shutdown context injectable — coverage 37.6% → 78.4%
- `outpost run`'s stdio path hardened against a non-compliant child: adversarial test fixtures covering a child that exits without responding and one that writes malformed JSON; fixed a real JSON-RPC 2.0 spec violation in the test fixture (notifications must never receive a response)
- Control plane reporting: `ControlPlaneAPIKey` config field, sent as `Authorization: Bearer` on every report — fixes a real gap where a control plane with auth enabled silently 401'd and dropped every report from a binary that had no way to send a key

### Fixed

- Security: hardened `outpost-cloud`'s scanner against prompt injection via the tool definition it's analyzing — scanning instructions now live in Anthropic's dedicated `system` field (not concatenated with untrusted content), with explicit anti-injection language and delimiters (prompt version bumped to `v2`)
- Security: closed a metadata-boundary bypass in `outpost-cloud`'s ingest validator — banned fields hidden inside JSON arrays (e.g. `{"items":[{"password":"x"}]}`) previously sailed through undetected; validation now recurses into arrays as well as objects
- Security: `outpost-cloud`'s control-plane API key check now uses a constant-time comparison (`crypto/subtle`) instead of `!=`
- Security: `outpost-cloud`'s `/v1/ingest/*` endpoints now cap request bodies at 1 MiB and rate-limit at 20 req/s (burst 40), gating the API-key check too so key-guessing traffic is throttled as well
- Security: `outpost-cloud`'s HTTP server now sets `ReadHeaderTimeout`/`ReadTimeout`/`WriteTimeout`/`IdleTimeout` — it previously ran with none at all
- `outpost-cloud` now has a CI pipeline (lint, race-tested tests, govulncheck, gitleaks, license check) and a `.gitignore` guard against committing `*.db` state files — it previously had neither
- A genuine data race in `runServe`'s graceful-shutdown path, caught by `-race` in CI: the "listening on..." log line moved to before the server goroutine starts
- TypeScript client's `package.json` was missing a `files` allowlist, so `npm publish` would have shipped a package without `dist/`

## [0.8.0-alpha] - 2026-07-20

### Added

- Fuzz testing for the JSON-RPC parser and schema validator (zero crashes across 5.9M+ combined executions); recursion-depth guard on schema validation against maliciously nested tool schemas
- Per-tool circuit breaker on `tools/call`: consecutive-failure tripping, cooldown, half-open trial, `CircuitOpen` (-32001) rejection before the upstream is attempted
- `state_db` config option (default `outpost.db`) for the shared SQLite state store
- Tool-definition pinning and drift detection: SHA-256 hash of the entire tool definition (not just inputSchema) on first sight; drift is logged, and blocks `tools/call` when `block: true` is configured (`DriftBlocked`, -32002)
- T2 statistical anomaly detection: Welford's online algorithm on per-tool `tools/call` latency and error rate, 3-stddev threshold with a zero-variance special case, detection-only (never blocks)
- Dedicated security review (automated static checks + adversarial reasoning against Outpost's own controls): 12/12 static checks passed, 2 real findings fixed — bearer-token forwarding was documented but not implemented, and drift-block state didn't survive a process restart. Full report in `internal/docs/security/2026-07-19-security-review.md`
- List-op cache for `tools/list` and `resources/read`, per-upstream `cache_ttl_seconds` (off by default); `tools/call` is structurally uncacheable — the cache package itself refuses to mint a key for it, so a wiring mistake degrades to a cache miss, never a stale tool-call result
- OpenTelemetry tracing: one span per proxied call (upstream/method/tool/duration/success attributes only, never arguments or results), exported via a swappable `io.Writer` (stdout in production; no real OTel collector exists yet)
- Fail-silent, buffered reporter (`internal/reporter`) sending drift events to an optional hosted control plane (`control_plane_url`, off by default); a down or unreachable control plane never blocks or delays MCP traffic
- **All 8 Week-2 binary-side features are now complete** (circuit breaker, list-op cache, drift differ, pinning, T2 anomaly detection, OTel tracing, plus Week-1's proxy core and T1 validation) — tagged `v0.8.0-alpha`

## [0.7.0-alpha] - 2026-07-19

### Added

- CLI skeleton: `outpost version`, `outpost serve` (stub)
- YAML config loader with per-tool-only retry opt-in — no global retry setting exists, by design (ADR-0004)
- `outpost serve` is a working MCP reverse proxy: JSON-RPC 2.0 over Streamable HTTP, dual protocol-version negotiation (`MCP-Protocol-Version` header), one route per configured upstream, graceful shutdown on SIGINT/SIGTERM
- Founding ADRs: metadata-only boundary, dual protocol support, fail-open tiers, no-default-retries / no-`tools/call`-caching
- CI: lint, race tests, cross-compile matrix (linux/darwin × amd64/arm64), govulncheck, secret scanning, dependency-license checks
- Release pipeline: goreleaser with signed checksums (cosign), SBOM (syft), distroless container image
- Embedded SQLite state store (`tool_pins`, `drift_events`, `breaker_state`, `anomaly_aggregates`) — pure Go, no cgo
- Control-plane wire-contract types (`PinEvent`, `DriftEvent`, `StatSnapshot`) with a reflection-based boundary test enforcing ADR-0001's metadata-only rule
- T1 structural validation: `outpost serve` learns tool schemas from `tools/list` and rejects invalid `tools/call` arguments before forwarding, fail-open for unknown tools, ~1.5us/op measured
