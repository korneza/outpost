# Contributing to Outpost

Thanks for considering a contribution. Outpost is early and moving fast toward `v0.9.0`; small, focused PRs land quickest.

## Development setup

- Go **1.25 or newer** (pure Go — never enable cgo in product builds).
- `make build` — static binary with version ldflags.
- `make test` — full suite with the race detector (needs cgo at test time; on Windows that means a mingw-w64 gcc, or use `make test-norace` locally and let CI run the race build).
- `make lint` — [golangci-lint](https://golangci-lint.run/).

## Ground rules

1. **Sign your commits (DCO).** Every commit needs a `Signed-off-by:` line (`git commit -s`), certifying the [Developer Certificate of Origin](https://developercertificate.org/). No CLA.
2. **Conventional commits.** `feat:`, `fix:`, `docs:`, `chore:`, `ci:`, `test:` — these feed the changelog.
3. **Keep PRs reviewable.** Aim for ≤400 changed lines. Bigger changes: open an issue first and we'll split it together.
4. **Tests are not optional.** New behaviour ships with tests; CI runs the race detector and must be green.
5. **Never log payloads.** Tool-call arguments and results must not appear in logs, error strings, or telemetry. Reviewers block on this.

## Things we will not merge

These are product invariants, each with an Architecture Decision Record in [`docs/adr/`](docs/adr/). PRs that violate them are closed with a pointer to the ADR — it isn't personal:

- Caching of `tools/call` responses (ADR-0004)
- Retries on by default, or any global retry setting (ADR-0004)
- Payload data crossing to hosted components (ADR-0001)
- Scanner/LLM output directly driving blocking behaviour (ADR-0003)
- Credential storage or OAuth brokering of any kind

## Security issues

Never open a public issue for a vulnerability. See [SECURITY.md](SECURITY.md).
