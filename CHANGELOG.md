# Changelog

All notable changes to Outpost are documented here. The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and versions follow [SemVer](https://semver.org/) (pre-1.0: minor bumps may break).

## [Unreleased]

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
- Scan-on-change pipeline (`outpost-cloud/internal/scanner`): drifted tool definitions are scanned once (dedup by hash) via a swappable `Scanner` interface; `ClaudeScanner` calls the Anthropic Messages API and rejects free-text or out-of-enum model responses rather than coercing them into a verdict. Alert dispatch (`internal/alerting`) via `SlackAlerter` + a rate-limiting decorator, both off by default
- Repos pushed to GitHub (`korneza/outpost`, `korneza/outpost-cloud`, `korneza/internal`); CI is live and green across all 9 jobs (lint, race-enabled tests, 4× cross-compile, govulncheck, gitleaks, license check) — the project's first fully-green CI run
- `GET /healthz` on the proxy (liveness/readiness probes)
- `outpost run -- <server-cmd> [args...]`: stdio wrapper mode — spawns a child MCP server and proxies its stdio through the same T1/breaker/pinning/cache/anomaly/reporter gate as HTTP mode, via a new `caller` interface and transport-agnostic `handle()` (both behavior-preserving refactors, verified by the unchanged existing test suite)
- `internal/stdioupstream`: a `caller` backed by a spawned child process speaking newline-delimited JSON-RPC over stdio
- TypeScript client shim (`clients/typescript/`, `@korneza/outpost-client`): direct-connection fallback if the proxy is unreachable, visible (console warning) and bounded (timeout), verified with a test that kills the proxy mid-run — zero runtime dependencies
- `internal/chaos` + `cmd/soaktest`: a deterministic, seeded chaotic upstream (slow responses, malformed bodies, dropped connections) for resilience testing; a bounded 30-second proof runs in the normal test suite, `cmd/soaktest` is the real tool for an actual long-duration soak
- Concurrent load tests published: T1's `Check` p99 stays sub-millisecond under 50-goroutine/100k-call concurrency; the reporter's buffer stays bounded under sustained concurrent load against an unreachable control plane
- Deploy-ready (not deployed) packaging: a Kubernetes manifest (`deploy/k8s/`) and a Homebrew formula (`packaging/homebrew/`) — both explicitly documented as not-yet-usable pending a real GitHub Release and, for Homebrew, a separate tap repo
