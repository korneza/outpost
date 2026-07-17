# Security Policy

Outpost is a security-adjacent product; we take reports seriously and we publish our detector accuracy honestly. Please extend us the courtesy of private disclosure.

## Reporting a vulnerability

Email **security@korneza.com**. You'll get an acknowledgement within **48 hours** and a substantive response within **7 days**. We follow coordinated disclosure with a **90-day** default window, negotiable if a fix needs longer.

Please include: affected version/commit, platform, MCP protocol version in use, reproduction steps, and impact as you understand it. Redact any real credentials or payloads from reproductions.

## Scope — reports we especially want

- Proxy bypass: any way traffic reaches an upstream without passing configured checks
- Payload egress: any path by which tool-call arguments, results, or credentials could leave the local process boundary (logs, telemetry, hosted reporting)
- Validation bypass: malformed `tools/call` traffic that T1 should reject but passes
- Pinning/drift evasion: tool-definition changes that escape hash pinning or the drift differ
- Scanner prompt injection: tool definitions crafted to manipulate scan verdicts

## Supported versions

Pre-1.0: the latest minor release only. From 1.0: the latest minor plus the previous one.

## No public issues for vulnerabilities

Public issues describing exploitable behaviour will be minimised and redirected here.
