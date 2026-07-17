## What & why

<!-- One or two sentences. Link the issue if there is one. -->

## Checklist

- [ ] Commits are signed off (`git commit -s`, DCO)
- [ ] Tests added/updated; `make test` is green (race detector)
- [ ] No tool-call arguments/results/credentials in logs, errors, or telemetry
- [ ] Change respects the product invariants in `docs/adr/` (no `tools/call` caching, no default retries, no payload egress, scanner output never blocks) — or updates the relevant ADR first
- [ ] ≤ ~400 changed lines, or an issue explains why not
