# soaktest

The real Week-4 "24-hour chaos soak" tool — runs a live `outpost` proxy
against `internal/chaos`'s deterministic chaotic upstream (random slow
responses, malformed bodies, dropped connections) for a configurable
duration.

Not run automatically by `go test` or CI — a 24-hour test would make the
normal suite unusable. `internal/chaos/soak_proof_test.go` runs a bounded
30-second version of the same idea as part of the real test suite instead;
this tool is for the actual long-duration run.

## Usage

```bash
go run ./cmd/soaktest -duration 24h -seed 42
```

Logs a running call/error count once per second to stderr and exits
cleanly at the deadline. A non-zero, growing error count is *expected*
(that's the chaos upstream's dropped-connection and malformed-body
behaviors) — what to actually watch for is the process itself: it should
never panic, and `calls` should keep incrementing throughout the run
without long stalls.
