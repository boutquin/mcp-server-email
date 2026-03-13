# mcp-server-email

Go MCP server for email (IMAP/SMTP). 22 tools, multi-account, connection pooling, retry, rate limiting.

Module: `github.com/boutquin/mcp-server-email` | Go 1.24 | MIT

## Package Layout

```
cmd/mcp-server-email/main.go     → Entry point, slog init, pool init, signal handling, stdio serve
internal/
├── auth/                        → OAuth2 device-code flow, SASL XOAUTH2, token store, provider detection
├── config/                      → ENV/file-based multi-account config loading + provider auto-detection
├── models/                      → Email/Folder/AttachmentInfo structs, message ID codec, error sentinels
├── imap/                        → Client (split: client.go, client_*.go) + Pool + Connector + auth + MIME
├── log/                         → Structured logging (slog) initialization from environment
├── smtp/                        → Client + Pool (lightweight, connect-per-send) + OAuth2 auth retry
├── retry/                       → Token-bucket rate limiter + exponential backoff with jitter
├── tools/                       → All 22 MCP tool handlers + registration
└── resources/                   → email://status resource
```

## Architecture

**Data flow:** MCP client → stdio → `mcp-go` router → tool handler → `imap.Operations`/`smtp.Operations` interface → pool → client → IMAP/SMTP server.

**Key patterns:**
- **Interface-driven DI** — `imap.Operations` (20 methods) and `smtp.Operations` (3 methods) decouple handlers from protocol clients. Compile-time checks enforce conformance.
- **Connection pooling with singleflight** — IMAP Pool coalesces concurrent requests for the same account via inflight map. Prevents thundering herd.
- **Force-close with reconnect** — `connInvalid` atomic flag + raw socket close unblocks hung IMAP operations.
- **Connector adapter** — `imapclientAdapter` collapses go-imap's two-step cmd/wait API into single-call interface for mocking.
- **Composite message IDs** — `{account}:{mailbox}:{uid}` format. All CRUD tools extract routing info from ID alone.
- **Retry with exponential backoff + jitter** — Shared `retry.WithRetry` with `PermanentError` short-circuit.
- **Token-bucket rate limiting** — Per-account, per-protocol (IMAP req/min, SMTP sends/hour).

**Dependencies flow inward:** `tools → imap.Operations/smtp.Operations → models`. No circular deps. `config` consumed at init time only.

## Dependencies (4 direct)

`go-imap/v2` (beta.8), `mcp-go` v0.43.2, `go-mail` v0.6.2, `x/net` v0.50.0

Notable indirect: `x/oauth2`, `go-message`, `go-sasl`

## Build & Test

```bash
make test              # go test -race -count=1 ./...
make lint              # golangci-lint run ./...
make test-integration  # Spins up Greenmail Docker, runs integration tests, tears down
make docker-build      # Multi-stage Docker build
```

- Integration tests require `integration` build tag + Greenmail container (IMAP:3993, SMTP:3465)
- Fuzz tests: 6 targets, run in CI for 2min each
- Linter: golangci-lint v2 with `default: all`, 11 linters disabled (each documented in `.golangci.yml`)

## Test Profile

| Metric | Value |
|--------|-------|
| Source lines | 7,003 (35 files) |
| Test lines | 20,926 (~55 files) |
| Test:source ratio | 2.99:1 |
| Overall coverage | 91.1% |
| CI jobs | 4 (test+codecov, lint, fuzz, integration) |

## CI & Release

- **CI:** `.github/workflows/ci.yml` — 4 jobs: unit tests with codecov upload, golangci-lint v2, fuzz (6 targets), integration (Greenmail service container)
- **Release:** `.github/workflows/release.yml` — GoReleaser on tag push → Homebrew, Docker, binaries, MCP bundle

## Lint Pitfalls

These golangci-lint v2 rules cause the most rework when caught late:

| Rule | What It Wants | Common Mistake |
|------|---------------|----------------|
| `wsl_v5` | Blank line before assignments preceded by other statements | Missing blank line before `handler := ...` after `mock := ...` |
| `funcorder` | Unexported methods after all exported methods | Adding `log()` between exported methods |
| `funlen` | Max 60 lines per function (including tests) | Long test setup — extract helpers with `t.Helper()` |
| `nolintlint` | No stale `//nolint:` directives | Leftover nolint after fixing the issue |
| `gofmt` | Aligned struct field tags | Misaligned fields after adding/removing a field |

## Tools (22)

accounts, folders, folder_create, list, unread, search, get, read_body, move, copy, delete, mark_read, flag, send, draft_create, draft_send, reply, forward, attachment_list, attachment_get, batch, thread
