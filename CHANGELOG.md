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
