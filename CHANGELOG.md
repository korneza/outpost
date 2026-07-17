# Changelog

All notable changes to Outpost are documented here. The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and versions follow [SemVer](https://semver.org/) (pre-1.0: minor bumps may break).

## [Unreleased]

### Added

- CLI skeleton: `outpost version`, `outpost serve` (stub)
- YAML config loader with per-tool-only retry opt-in — no global retry setting exists, by design (ADR-0004)
- Founding ADRs: metadata-only boundary, dual protocol support, fail-open tiers, no-default-retries / no-`tools/call`-caching
- CI: lint, race tests, cross-compile matrix (linux/darwin × amd64/arm64), govulncheck, secret scanning, dependency-license checks
- Release pipeline: goreleaser with signed checksums (cosign), SBOM (syft), distroless container image
