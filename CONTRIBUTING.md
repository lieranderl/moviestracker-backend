# Contributing

## Development Workflow
1. Create a feature branch.
2. Keep changes scoped to one module when possible.
3. Run formatting, tests, and checks locally before pushing.
4. Open PR with clear summary and risk notes.

## Required Local Checks
- `make fmt`
- `make vet`
- `make test`
- `make race`
- `make precommit-run`

For deeper quality/security checks:
- `make lint`
- `make staticcheck`
- `make gosec`

## Testing Standards
- Prefer table-driven unit tests for pure logic.
- Add integration tests only when behavior depends on external systems.
- Avoid flaky tests (time/network dependent) unless explicitly tagged and optional.
- Target at least:
  - overall coverage: 70%+
  - critical paths (HTTP handlers + core logic): 85%+

## Code Standards
- Use idiomatic Go.
- Wrap errors with `%w` when returning upstream.
- Validate inputs at boundaries (HTTP handlers / CLI args).
- Use `context.Context` for cancellation/timeouts in I/O and long-running paths.
- Avoid logging secrets or full credential-bearing payloads.

## Multi-Module Notes
This repository has four separate Go modules. Run commands from module directories for ad-hoc work, or use root `Makefile` targets to run across all modules.

## Git Hooks
- Install hooks once: `make precommit-install`
- Hooks run on commit and include formatting + targeted module vet/test checks.

## CI Expectations
PRs are expected to pass `.github/workflows/ci.yml` checks:
- format check
- vet
- tests + race
- staticcheck, gosec, golangci-lint
- build + Docker build

## Security & Secrets
- Never commit real credentials.
- Keep `.env` local only.
- Use secret manager integrations in deploy scripts.
- If a secret is leaked, rotate immediately.
