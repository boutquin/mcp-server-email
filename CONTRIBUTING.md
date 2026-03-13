# Contributing

Thanks for your interest in contributing to mcp-server-email.

## Getting Started

```bash
git clone https://github.com/boutquin/mcp-server-email.git
cd mcp-server-email
make test
make lint
```

**Requirements:** Go 1.24+, golangci-lint v2.11+

## Development Workflow

1. Fork and clone the repository
2. Create a feature branch from `main`
3. Make your changes
4. Run the validation triple before committing:
   ```bash
   go build ./...
   go test -race -count=1 ./...
   golangci-lint run ./...
   ```
5. Commit with a descriptive message (see below)
6. Open a pull request against `main`

## Testing

### Unit Tests

```bash
make test
```

All tests run with the race detector (`-race`). New code should include tests — the project maintains a 2.99:1 test-to-source ratio and 91%+ coverage.

### Integration Tests

Integration tests require Docker (Greenmail container for real IMAP/SMTP):

```bash
make test-integration
```

This starts a Greenmail container on ports 3993 (IMAP) and 3465 (SMTP), runs tests with the `integration` build tag, and tears down the container.

### Fuzz Tests

Six fuzz targets run in CI for 2 minutes each. To run locally:

```bash
go test -fuzz=FuzzParseMessageID -fuzztime=30s ./internal/models/
go test -fuzz=FuzzHtmlToText -fuzztime=30s ./internal/tools/
```

## Code Style

The project uses golangci-lint v2 with `default: all` and 11 explicitly disabled linters (documented in `.golangci.yml`). Key rules that cause the most rework:

| Rule | What It Wants |
|------|---------------|
| `wsl_v5` | Blank line before assignments preceded by other statements |
| `funcorder` | Unexported methods after all exported methods |
| `funlen` | Max 60 lines per function (extract helpers with `t.Helper()`) |
| `nolintlint` | No stale `//nolint:` directives |
| `gofmt` | Aligned struct field tags |

**Run lint early** — after every few files, not at the end.

## Commit Messages

Follow [Conventional Commits](https://www.conventionalcommits.org/):

```
type: short description

Longer explanation if needed.
```

Types: `feat`, `fix`, `refactor`, `test`, `chore`, `docs`, `ci`

Examples:
- `feat: add email_thread tool for cross-folder thread search`
- `fix: handle nil attachment in forward handler`
- `test: add coverage for SMTP OAuth2 retry path`
- `docs: refresh CLAUDE.md key files table`

## Architecture

See [ARCHITECTURE.md](ARCHITECTURE.md) for design decisions, data flow, and package responsibilities.

Key patterns to understand before contributing:
- **Interface-driven DI** — `imap.Operations` and `smtp.Operations` decouple handlers from protocol clients
- **Connection pooling with singleflight** — IMAP Pool coalesces concurrent requests per account
- **Composite message IDs** — `{account}:{mailbox}:{uid}` format routes all CRUD operations
- **Retry with backoff** — shared `retry.WithRetry` with `PermanentError` short-circuit

## Pull Request Checklist

- [ ] Tests pass: `go test -race -count=1 ./...`
- [ ] Lint clean: `golangci-lint run ./...`
- [ ] New public functions have GoDoc comments
- [ ] Coverage maintained (check `go tool cover -func=coverage.out`)
- [ ] No sensitive data (passwords, tokens) in test fixtures

## Reporting Issues

Open an issue on [GitHub](https://github.com/boutquin/mcp-server-email/issues) with:
- Steps to reproduce
- Expected vs actual behavior
- Server version (`mcp-server-email --version` or `go version -m $(which mcp-server-email)`)
- Mail provider (Gmail, Outlook, etc.) if relevant

## License

By contributing, you agree that your contributions will be licensed under the [MIT License](LICENSE).
