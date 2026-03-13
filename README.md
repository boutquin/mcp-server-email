# mcp-server-email

[![CI](https://github.com/boutquin/mcp-server-email/actions/workflows/ci.yml/badge.svg)](https://github.com/boutquin/mcp-server-email/actions/workflows/ci.yml)
[![codecov](https://codecov.io/gh/boutquin/mcp-server-email/branch/main/graph/badge.svg?token=Y3EWPR8X6K)](https://codecov.io/gh/boutquin/mcp-server-email)
[![Go](https://img.shields.io/badge/Go-1.24+-00ADD8?logo=go&logoColor=white)](https://go.dev)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)
[![MCP](https://img.shields.io/badge/MCP-compatible-blueviolet)](https://modelcontextprotocol.io)

Multi-account email server for the [Model Context Protocol](https://modelcontextprotocol.io). Gives LLMs full email access — read, send, search, organize — over IMAP/SMTP with connection pooling, rate limiting, and retry. Designed as the remote-email counterpart to [apple-bridge](https://github.com/boutquin/apple-bridge) (local Mail.app access).

## Quick Start

1. **Install**

   ```bash
   go install github.com/boutquin/mcp-server-email/cmd/mcp-server-email@latest
   ```

2. **Create a config file** (`~/.config/mcp-email/accounts.json`)

   For well-known providers (Gmail, Outlook, Yahoo, iCloud, Fastmail, Zoho), host and port are auto-detected from the email domain — just provide credentials:

   ```json
   [
     {
       "id": "hello",
       "email": "hello@gmail.com",
       "username": "hello@gmail.com",
       "password": "app-password-here"
     }
   ]
   ```

   For custom mail servers, specify host and port explicitly:

   ```json
   [
     {
       "id": "work",
       "email": "hello@example.com",
       "imap_host": "mail.example.com",
       "imap_port": 993,
       "smtp_host": "mail.example.com",
       "smtp_port": 465,
       "username": "hello@example.com",
       "password": "app-password-here"
     }
   ]
   ```

   ```bash
   chmod 600 ~/.config/mcp-email/accounts.json
   ```

3. **Add to Claude Code** (`~/.claude.json`)

   ```json
   {
     "mcpServers": {
       "email": {
         "command": "mcp-server-email",
         "env": {
           "EMAIL_CONFIG_FILE": "~/.config/mcp-email/accounts.json"
         }
       }
     }
   }
   ```

4. **Restart Claude Code** — the `email_*` tools are now available.

## Installation

### Go Install (recommended for Go developers)

```bash
go install github.com/boutquin/mcp-server-email/cmd/mcp-server-email@latest
```

### Homebrew (macOS/Linux)

```bash
brew install boutquin/tap/mcp-server-email
```

### Binary Download

Download pre-built binaries for your platform from [GitHub Releases](https://github.com/boutquin/mcp-server-email/releases).

Available for: Linux (amd64, arm64), macOS (amd64, arm64), Windows (amd64, arm64).

### Docker

```bash
docker run --rm \
  -e EMAIL_ACCOUNTS='[{"id":"main","email":"user@example.com","imap_host":"mail.example.com","imap_port":993,"smtp_host":"mail.example.com","smtp_port":465,"username":"user@example.com","password":"app-password"}]' \
  ghcr.io/boutquin/mcp-server-email:latest
```

### MCP Bundle (Claude Desktop)

Download the `.mcpb` file from [Releases](https://github.com/boutquin/mcp-server-email/releases) and open in Claude Desktop.

### Build from source

```bash
git clone https://github.com/boutquin/mcp-server-email.git
cd mcp-server-email
go build -o mcp-server-email ./cmd/mcp-server-email
```

## Configuration

Accounts are loaded once at startup. Changes require a server restart.

### Config file vs environment variable

| Approach | Best for |
|----------|----------|
| `EMAIL_CONFIG_FILE` — path to a JSON file | Production use. File can be permission-locked (`chmod 600`) |
| `EMAIL_ACCOUNTS` — inline JSON in env var | Testing, CI, or containerized deployments |

If both are set, `EMAIL_ACCOUNTS` takes precedence.

### Account JSON schema

```json
[
  {
    "id": "hello",
    "email": "hello@example.com",
    "imap_host": "mail.example.com",
    "imap_port": 993,
    "smtp_host": "mail.example.com",
    "smtp_port": 465,
    "username": "hello@example.com",
    "password": "app-password-here"
  }
]
```

| Field | Required | Description |
|-------|----------|-------------|
| `id` | Yes | Unique account identifier |
| `email` | Yes | Email address |
| `imap_host` | No* | IMAP server hostname |
| `imap_port` | No* | IMAP port (993 = implicit TLS, 143 = STARTTLS) |
| `smtp_host` | No* | SMTP server hostname |
| `smtp_port` | No* | SMTP port (465 = implicit TLS, 587 = STARTTLS) |
| `username` | Yes | Login username |
| `password` | Yes** | App password or account password |
| `use_starttls` | No | Override TLS auto-detection (`true`/`false`) |
| `insecure_skip_verify` | No | Skip TLS certificate verification (dev/testing) |
| `auth_method` | No | `"password"` (default) or `"oauth2"` |
| `oauth_client_id` | No | OAuth2 client ID (required when `auth_method` is `"oauth2"`) |
| `oauth_client_secret` | No | OAuth2 client secret |
| `oauth_token_file` | No | Override token file path |

*Host and port are auto-detected for well-known providers (see below). Required for custom servers.
**Not required when using OAuth2 authentication.

### Provider auto-detection

When `imap_host`/`smtp_host` are omitted, the server detects settings from the email domain:

| Provider | Domains | IMAP | SMTP |
|----------|---------|------|------|
| Gmail | `gmail.com`, `googlemail.com` | `imap.gmail.com:993` | `smtp.gmail.com:587` |
| Outlook | `outlook.com`, `hotmail.com`, `live.com` | `outlook.office365.com:993` | `smtp.office365.com:587` |
| Yahoo | `yahoo.com` | `imap.mail.yahoo.com:993` | `smtp.mail.yahoo.com:587` |
| iCloud | `icloud.com`, `me.com`, `mac.com` | `imap.mail.me.com:993` | `smtp.mail.me.com:587` |
| Fastmail | `fastmail.com`, `fastmail.fm` | `imap.fastmail.com:993` | `smtp.fastmail.com:587` |
| Zoho | `zoho.com`, `zohomail.com` | `imap.zoho.com:993` | `smtp.zoho.com:587` |

Explicit host/port in the config always takes precedence over auto-detection.

### TLS modes

TLS mode is auto-detected from port:

| Port | Protocol | Mode |
|------|----------|------|
| 993 | IMAP | Implicit TLS |
| 143 | IMAP | STARTTLS |
| 465 | SMTP | Implicit TLS |
| 587 | SMTP | STARTTLS |

Override with `"use_starttls": true` or `"use_starttls": false` in the account object. Omit for auto-detection (recommended).

### OAuth2 authentication

For providers that support it (Gmail, Outlook), you can use OAuth2 instead of app passwords. This uses the device code flow (RFC 8628) — no browser redirect needed.

1. **Create OAuth2 credentials** in the provider's developer console (Google Cloud Console or Azure AD)

2. **Configure the account** with `auth_method: "oauth2"`:

   ```json
   [
     {
       "id": "gmail",
       "email": "user@gmail.com",
       "username": "user@gmail.com",
       "auth_method": "oauth2",
       "oauth_client_id": "your-client-id.apps.googleusercontent.com",
       "oauth_client_secret": "your-client-secret"
     }
   ]
   ```

3. **On first connection**, the server initiates the device code flow — printing a verification URL and code to stderr. Visit the URL and enter the code to authorize.

4. **Tokens are persisted** in `~/.config/mcp-email/tokens/` and automatically refreshed. Subsequent connections reuse the stored token without re-authorization.

Supported OAuth2 providers: **Gmail** (`gmail.com`, `googlemail.com`) and **Outlook** (`outlook.com`, `hotmail.com`, `live.com`).

### Environment variables

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `EMAIL_CONFIG_FILE` | Yes* | — | Path to JSON config file |
| `EMAIL_ACCOUNTS` | Yes* | — | JSON array of account configs (inline) |
| `EMAIL_DEFAULT_ACCOUNT` | No | First account | Default account ID |
| `EMAIL_IMAP_TIMEOUT_MS` | No | `30000` | IMAP operation timeout (ms) |
| `EMAIL_SMTP_TIMEOUT_MS` | No | `30000` | SMTP operation timeout (ms) |
| `EMAIL_IMAP_RATE_LIMIT` | No | `60` | IMAP requests/minute/account |
| `EMAIL_SMTP_RATE_LIMIT` | No | `100` | SMTP sends/hour/account |
| `MAX_ATTACHMENT_SIZE_MB` | No | `18` | Max size per attachment (MB) |
| `MAX_TOTAL_ATTACHMENT_SIZE_MB` | No | `18` | Max total attachment size per message (MB) |
| `MAX_DOWNLOAD_SIZE_MB` | No | `25` | Max attachment download size (MB) |
| `EMAIL_POOL_CLOSE_TIMEOUT_MS` | No | `5000` | Pool close timeout (ms) |
| `EMAIL_DEBUG` | No | `false` | Debug logging to stderr |
| `LOG_LEVEL` | No | `info` | Log level: `debug`, `info`, `warn`, `error` |
| `LOG_FORMAT` | No | `json` | Log format: `json` or `text` |

*One of `EMAIL_CONFIG_FILE` or `EMAIL_ACCOUNTS` is required.

### Config file permissions

The config file contains account passwords. Always restrict access:

```bash
chmod 600 ~/.config/mcp-email/accounts.json
```

## Tools (22)

### Account & folder tools

| Tool | Description | Key params |
|------|-------------|------------|
| `email_accounts` | List configured accounts with connection status | — |
| `email_folders` | List all folders with unread/total counts | `account?` |
| `email_folder_create` | Create new folder | `name`, `account?` |

### Message listing

| Tool | Description | Key params |
|------|-------------|------------|
| `email_list` | List messages in folder | `folder?`, `limit?`, `offset?`, `includeBody?`, `account?` |
| `email_unread` | List unread messages | `folder?`, `limit?`, `includeBody?`, `account?` |
| `email_search` | Search subject and body | `query`, `from?`, `to?`, `since?`, `before?`, `folder?`, `limit?`, `includeBody?`, `account?` |

### Message operations

| Tool | Description | Key params |
|------|-------------|------------|
| `email_get` | Get full message by ID | `id` |
| `email_read_body` | Read email body with pagination | `id`, `offset?`, `limit?`, `format?` |
| `email_move` | Move message to folder | `id`, `destination` |
| `email_copy` | Copy message to folder | `id`, `destination` |
| `email_delete` | Delete message (trash or permanent expunge) | `id`, `permanent?` |
| `email_mark_read` | Mark as read/unread | `id`, `read` |
| `email_flag` | Flag/unflag message | `id`, `flagged` |
| `email_reply` | Reply to a message (sets In-Reply-To, References, quotes body) | `id`, `body`, `all?`, `cc?`, `bcc?`, `isHtml?`, `account?` |
| `email_forward` | Forward a message (re-attaches original attachments) | `id`, `to`, `body?`, `cc?`, `bcc?`, `isHtml?`, `account?` |
| `email_batch` | Batch operations on multiple messages | `action`, `ids`, `destination?`, `permanent?`, `read?`, `flagged?` |

### Attachments & threads

| Tool | Description | Key params |
|------|-------------|------------|
| `email_attachment_list` | List attachments on a message | `id` |
| `email_attachment_get` | Download attachment by index | `id`, `index`, `saveTo?` |
| `email_thread` | Get conversation thread (searches across INBOX, Sent, Archive, and All Mail) | `id` |

### Send & drafts

| Tool | Description | Key params |
|------|-------------|------------|
| `email_send` | Send via SMTP with optional attachments | `to`, `subject`, `body`, `cc?`, `bcc?`, `replyTo?`, `isHtml?`, `attachments?`, `account?` |
| `email_draft_create` | Save draft with optional attachments | `to?`, `subject?`, `body?`, `cc?`, `bcc?`, `isHtml?`, `attachments?`, `account?` |
| `email_draft_send` | Send existing draft | `id` |

All optional `account` params default to the configured default account.

## Search

`email_search` searches both **subject and body** using IMAP `SEARCH OR (SUBJECT "q") (BODY "q")`.

Optional filters narrow the candidate set server-side before body scanning:

| Filter | Format | Example |
|--------|--------|---------|
| `from` | Email address or name | `"alice@example.com"` |
| `to` | Email address or name | `"bob@example.com"` |
| `since` | `YYYY-MM-DD` | `"2026-01-01"` |
| `before` | `YYYY-MM-DD` | `"2026-02-01"` |

The existing operation timeout (default 30s) prevents hung body searches on large mailboxes.

## Attachments

`email_send` and `email_draft_create` accept an `attachments` parameter — an array of file references on the server host:

```json
{
  "attachments": [
    {"path": "/tmp/report.pdf"},
    {"path": "/tmp/data.csv", "filename": "Q1-data.csv", "content_type": "text/csv"}
  ]
}
```

| Parameter | Required | Description |
|-----------|----------|-------------|
| `path` | Yes | Absolute file path on the server host |
| `filename` | No | Override display filename (defaults to basename of `path`) |
| `content_type` | No | MIME type (auto-detected from file extension if omitted) |

**Limits (defaults):** 18 MB per file, 18 MB total (pre-base64 encoding; stays under 25 MB SMTP cap after encoding). Configurable via `MAX_ATTACHMENT_SIZE_MB` and `MAX_TOTAL_ATTACHMENT_SIZE_MB` environment variables.

**Download limit:** Attachment downloads (`email_attachment_get`) are capped at 25 MB by default, configurable via `MAX_DOWNLOAD_SIZE_MB`.

Validation failures (missing file, non-absolute path, size exceeded) return `INVALID_ARGUMENT`.

## Message IDs

Message IDs are composite strings encoding account, mailbox, and UID:

```
{account}:{mailbox}:{uid}
```

Example: `hello:INBOX:12345`

All CRUD tools (`email_get`, `email_move`, `email_copy`, `email_delete`, `email_mark_read`, `email_flag`, `email_draft_send`) extract the account and folder from the ID — no separate params needed.

## Error Codes

All errors are returned as MCP tool errors with a structured code prefix:

| Code | Meaning |
|------|---------|
| `AUTH_FAILED` | IMAP/SMTP authentication failed |
| `CONNECTION_FAILED` | Cannot connect to server |
| `ACCOUNT_NOT_FOUND` | Unknown account ID |
| `FOLDER_NOT_FOUND` | Mailbox doesn't exist |
| `MESSAGE_NOT_FOUND` | UID not found in mailbox |
| `INVALID_ARGUMENT` | Missing/invalid parameter (including attachment validation) |
| `TIMEOUT` | Operation timed out |
| `INTERNAL` | Unexpected server error |

## Resources

| URI | Description |
|-----|-------------|
| `email://status` | Server version, account connection state, rate limit configuration |

## Comparison with apple-bridge

This server and [apple-bridge](https://github.com/boutquin/apple-bridge) share an `Email` model and parameter semantics (`limit`, `includeBody`, `folder`, `query`) so LLMs can work with both interchangeably. Key differences:

| Aspect | mcp-server-email | apple-bridge |
|--------|------------------|--------------|
| Transport | IMAP/SMTP (remote) | Mail.app (local) |
| Tool prefix | `email_*` | `mail_*` |
| Message ID | `{account}:{mailbox}:{uid}` | RFC 5322 Message-ID |
| Folder create | Supported | Not supported (Mail.app requires UI) |
| Copy message | Supported | Not supported |
| Draft send | Supported | Not supported (Mail.app uses compose UI) |
| Attachments (send) | File path on server host | Not yet supported |

## Development

### Prerequisites

- Go 1.24+
- [golangci-lint](https://golangci-lint.run/)
- [Docker](https://docs.docker.com/get-docker/) (for integration tests only)

### Build

```bash
go build ./...
```

### Unit tests

```bash
make test
# or: go test -race -count=1 ./...
```

Unit tests use mock implementations of the `imap.Operations` and `smtp.Operations` interfaces — no live mail server needed.

### Benchmarks

```bash
go test -bench=. -benchmem ./...
```

| Benchmark | Package | What it measures |
|-----------|---------|------------------|
| `BenchmarkPoolGetRelease` | `imap` | Connection pool acquire/release cycle |
| `BenchmarkExtractAttachments` | `imap` | MIME attachment extraction |
| `BenchmarkHtmlToText` | `tools` | HTML-to-plain-text conversion |
| `BenchmarkLimiterAllow` | `retry` | Rate limiter (sequential) |
| `BenchmarkLimiterAllow_Parallel` | `retry` | Rate limiter (concurrent) |

### Fuzz testing

Fuzz targets ship with seed corpora in `testdata/fuzz/` directories. Run a specific target:

```bash
go test -fuzz=FuzzParseMessageID ./internal/models/ -fuzztime=30s
```

| Target | Package | What it fuzzes |
|--------|---------|----------------|
| `FuzzBuildSearchCriteria` | `imap` | IMAP search query builder |
| `FuzzExtractAttachmentByIndex` | `imap` | Attachment index boundary handling |
| `FuzzExtractContentType` | `imap` | MIME content-type parser |
| `FuzzParseMessageID` | `models` | Composite message ID codec |
| `FuzzHtmlToText` | `tools` | HTML-to-text sanitizer |
| `FuzzSplitAddresses` | `tools` | Email address list splitter |

### Lint

```bash
make lint
# or: golangci-lint run ./...
```

### Integration tests

Integration tests exercise the full IMAP/SMTP stack against a real mail server. They are isolated behind the `integration` build tag and **never run** during `go test ./...`.

#### Mail server: Greenmail

Tests use [Greenmail](https://greenmail-mail-test.github.io/greenmail/), a lightweight Java mail server packaged as a Docker image. Key details that affect how you run it:

| Setting | Value | Why it matters |
|---------|-------|----------------|
| IMAPS port | **3993** | Greenmail's SSL IMAP port (not 993). The code auto-detects TLS from port number, so tests explicitly set `UseStartTLS=false` to force implicit TLS on this non-standard port. |
| SMTPS port | **3465** | Greenmail's SSL SMTP port (not 465). Same `UseStartTLS=false` override. |
| Bind address | **`0.0.0.0`** | Greenmail defaults to `127.0.0.1` *inside the container*, which makes Docker port-mapping silently fail (connections get EOF). You **must** pass `-Dgreenmail.hostname=0.0.0.0`. |
| Username | **`test`** | Greenmail uses the *local part only* (before `@`) as the login username — not the full email address. If the user is `test@example.com`, the IMAP/SMTP username is `test`. |
| TLS certificates | **Self-signed** | Greenmail generates self-signed certs. Tests set `InsecureSkipVerify: true` in the account config to accept them. |

#### Quick start (one command)

```bash
make test-integration
```

This starts a Greenmail container, runs all integration tests, then tears down the container — regardless of pass/fail.

#### Manual step-by-step

If you need to iterate on tests without restarting the container each time:

1. **Start Greenmail**

   ```bash
   docker run -d --name greenmail \
     -p 3465:3465 -p 3993:3993 \
     -e "GREENMAIL_OPTS=-Dgreenmail.setup.test.all -Dgreenmail.users=test:password@example.com -Dgreenmail.hostname=0.0.0.0" \
     greenmail/standalone:2.1.0
   ```

   Wait ~3 seconds for the JVM to start.

2. **Run integration tests**

   ```bash
   TEST_IMAP_HOST=localhost TEST_IMAP_PORT=3993 \
   TEST_SMTP_HOST=localhost TEST_SMTP_PORT=3465 \
   TEST_EMAIL=test@example.com TEST_PASSWORD=password \
     go test -tags=integration -race -v ./...
   ```

3. **Tear down** when done

   ```bash
   docker stop greenmail && docker rm greenmail
   ```

#### Test environment variables

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `TEST_IMAP_HOST` | Yes* | — | IMAP server hostname. Tests skip if unset. |
| `TEST_IMAP_PORT` | No | `3993` | IMAPS port |
| `TEST_SMTP_HOST` | Yes* | — | SMTP server hostname. Tests skip if unset. |
| `TEST_SMTP_PORT` | No | `3465` | SMTPS port |
| `TEST_EMAIL` | No | `test@example.com` | Email address for the test account |
| `TEST_USERNAME` | No | Local part of `TEST_EMAIL` | IMAP/SMTP login username (Greenmail uses local part only) |
| `TEST_PASSWORD` | No | `password` | Account password |

*If the corresponding `HOST` variable is unset, that test file's tests are skipped with a message (not failed).

#### What the tests cover

**IMAP** (`internal/imap/integration_test.go` — 7 tests):

| Test | What it verifies |
|------|------------------|
| `ConnectAndListFolders` | TLS connection, authentication, folder listing, INBOX exists |
| `SendAndListMessages` | SMTP send → IMAP receive round-trip, body content match |
| `SearchBySubject` | IMAP SEARCH by subject string |
| `DeleteMessagePermanent` | Flag as deleted + expunge, verify message is gone |
| `MoveMessage` | IMAP MOVE to another folder (skips if server lacks MOVE extension) |
| `DraftWorkflow` | SaveDraft → GetDraft → DeleteDraft lifecycle (skips if no APPENDUID) |
| `MarkReadAndFlag` | Set read/flagged flags, verify via GetMessage |

**SMTP** (`internal/smtp/integration_test.go` — 4 tests):

| Test | What it verifies |
|------|------------------|
| `SendPlainText` | Plain-text email delivery, body content verified via IMAP |
| `SendHTML` | HTML email delivery, Content-Type verified as `text/html` |
| `SendWithAttachment` | Multipart MIME with attachment, filename verified in metadata |
| `RateLimitTokenConsumption` | Sending consumes a rate-limit token |

#### Troubleshooting

| Symptom | Cause | Fix |
|---------|-------|-----|
| `EOF` or `connection reset` on connect | Greenmail bound to `127.0.0.1` inside container | Add `-Dgreenmail.hostname=0.0.0.0` to `GREENMAIL_OPTS` |
| `TLS handshake failure` / certificate error | Self-signed certs rejected | Test configs already set `InsecureSkipVerify: true` — if writing new tests, do the same |
| `Invalid login/password` | Using full email as username | Greenmail expects the local part only (`test`, not `test@example.com`). Set `TEST_USERNAME` or let it default. |
| `STARTTLS` error on port 3993 | Using STARTTLS on an implicit-TLS port | Test configs set `UseStartTLS=false`. Don't use ports 3143/3025 (plain, no TLS). |
| Tests skip with "not set" | `TEST_IMAP_HOST` / `TEST_SMTP_HOST` not exported | Export the env vars or use the `make test-integration` target |
| `MoveMessage` test skips | Greenmail may not support MOVE | Expected — test uses `t.Skip()` |

#### CI

Integration tests run automatically in GitHub Actions via the `integration` job in `.github/workflows/ci.yml`. The job uses a Greenmail service container — no manual Docker setup needed. See the workflow file for the exact configuration.

### Coverage

To generate a combined unit + integration coverage report:

```bash
# With Greenmail running (see above):
TEST_IMAP_HOST=localhost TEST_IMAP_PORT=3993 \
TEST_SMTP_HOST=localhost TEST_SMTP_PORT=3465 \
TEST_EMAIL=test@example.com TEST_PASSWORD=password \
  go test -tags=integration -race -coverprofile=coverage.out ./...

go tool cover -func=coverage.out | tail -1   # total percentage
go tool cover -html=coverage.out             # open in browser
```

### Architecture

```
mcp-server-email/
├── cmd/mcp-server-email/     # Entry point
└── internal/
    ├── auth/                 # OAuth2 device code flow, XOAUTH2 SASL, token store
    ├── config/               # Multi-account configuration, provider auto-detection
    ├── imap/                 # IMAP client (split by concern), connection pool, Operations interface
    │   ├── client.go         # Client struct, lifecycle, shared helpers
    │   ├── client_messages.go # List, search, get, attachments
    │   ├── client_folders.go  # Folder ops, role cache
    │   ├── client_drafts.go   # Draft save/get/delete
    │   ├── client_flags.go    # Flags, move, copy, delete
    │   └── pool.go           # Connection pool with configurable close timeout
    ├── log/                  # Structured logging (slog) initialization
    ├── models/               # Email model, message ID codec, error types
    ├── resources/            # email://status resource
    ├── retry/                # Token-bucket rate limiter
    ├── smtp/                 # SMTP client, Operations interface
    └── tools/                # 22 MCP tool handlers + registration
```

Tool handlers are decoupled from IMAP/SMTP clients via the `imap.Operations` and `smtp.Operations` interfaces, enabling comprehensive unit testing with mocks.

### Dependencies

This project uses [go-imap v2](https://github.com/emersion/go-imap) (currently v2.0.0-beta.8).
The v2 API is not yet stable — breaking changes may occur before the v2.0.0 release.
We pin the exact version in `go.mod` and will upgrade promptly when stable is released.

## License

[MIT](LICENSE)
