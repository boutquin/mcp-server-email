# Architecture

This document explains the design decisions behind mcp-server-email — why things are built the way they are, not just what they do.

## Data Flow

```
LLM Client (Claude, Cursor, etc.)
    │
    │  stdio (JSON-RPC)
    ▼
mcp-go router
    │
    │  dispatches to tool handler
    ▼
Tool Handler (tools/)
    │
    │  calls interface methods
    ▼
imap.Operations / smtp.Operations    ◄── interface boundary (DI seam)
    │
    │  delegates to pool
    ▼
Pool (imap/pool.go, smtp/pool.go)
    │
    │  manages connections
    ▼
Client (imap/client.go, smtp/client.go)
    │
    │  protocol operations
    ▼
IMAP/SMTP Server
```

Every layer adds exactly one concern. Tool handlers parse MCP arguments and format responses. The Operations interface provides the DI seam for testing. The pool manages connection lifecycle. The client speaks the protocol.

## Package Responsibilities

| Package | Concern | Key Invariant |
|---------|---------|---------------|
| `cmd` | Entry point, signal handling | Wires everything together; no business logic |
| `config` | Config loading, provider detection | Consumed at init time only; immutable after startup |
| `auth` | OAuth2 device-code flow, token storage | Tokens persisted atomically (temp file + rename) |
| `imap` | IMAP client, pool, connector | Pool lock never held during network I/O |
| `smtp` | SMTP client, pool | One connection per send (no persistent connections) |
| `retry` | Rate limiting, backoff | Shared by both IMAP and SMTP; per-account isolation |
| `tools` | 22 MCP tool handlers | Pure request→response; no persistent state |
| `models` | Data structures, error sentinels | Leaf package; no imports from other internal packages |
| `resources` | MCP resources (`email://status`) | Read-only status reporting |
| `log` | slog initialization | One-time setup from environment variables |

**Dependency direction:** `tools → imap/smtp (via Operations) → models`. No circular imports. `config` flows inward at init. `auth` is consumed by `imap` and `smtp` for OAuth2.

## Design Decisions

### Why Interface-Driven DI

`imap.Operations` (20 methods) and `smtp.Operations` (3 methods) are the central abstraction. Tool handlers depend only on these interfaces, never on concrete clients or pools.

**Why:** Testing. Every tool handler test uses a mock that implements the interface. No network connections, no containers, no flaky tests. The 95.2% coverage in `tools/` exists because mocking is trivial.

**Compile-time check:** Each concrete type has a `var _ Operations = (*Pool)(nil)` line to ensure the interface is satisfied. If a method signature changes, the build fails immediately.

### Why Singleflight in the IMAP Pool

When multiple tool calls arrive concurrently for the same account, naive pooling would create multiple connections. The pool uses an in-flight map to coalesce these:

```
Goroutine A: Get("gmail") → no client, no inflight → creates inflight{done: chan}
                                                       → releases lock
                                                       → dials IMAP (slow)
Goroutine B: Get("gmail") → no client, sees inflight → releases lock
                                                       → waits on <-inflight.done
Goroutine A: dial completes → stores client → closes inflight.done
Goroutine B: ← receives client from inflight
```

**Why not `sync.Once` or `golang.org/x/sync/singleflight`?** The pool needs per-account coalescing with error propagation and connection caching. The inflight map provides this with ~20 lines of code. A generic singleflight library would need adaptation for the pool's lifecycle (stale connection eviction, graceful shutdown).

**Critical detail:** The pool lock is released before dialing. Network I/O under a lock would serialize all accounts. Instead, only the inflight map update is locked; the actual connection happens lock-free.

### Why Force-Close with Raw Socket

IMAP operations can hang indefinitely — the server stops responding, the network drops, or TLS negotiation stalls. Go's `context.Context` cancellation doesn't interrupt a blocked `Read()` on a TCP socket.

The solution:

1. `withTimeout` wraps every IMAP operation in a context with deadline
2. On timeout, `forceClose()` calls `rawConn.Close()` directly on the underlying `net.Conn`
3. This causes the blocked `Read()` to return an I/O error immediately
4. The `connInvalid` atomic flag signals that the next retry should reconnect
5. `withTimeout` waits for the operation goroutine to finish before returning, preventing concurrent access to connection state

**Why not just use `context.WithTimeout`?** go-imap's client doesn't propagate context cancellation to the underlying socket read. The context expires, but the goroutine running the IMAP command stays blocked until the TCP timeout (which can be minutes). Force-close unblocks it in milliseconds.

**Why atomic flag instead of mutex?** The force-close must happen while another goroutine holds the connection mutex (it's doing the IMAP operation that's hanging). Using a mutex for the invalid flag would deadlock. The atomic flag is safe to set from any goroutine.

### Why Token-Bucket Rate Limiting

LLM agents can fire tool calls rapidly — an aggressive agent might search, list, and fetch dozens of messages in seconds. Without rate limiting, this would hit provider rate limits (Gmail: ~15 IMAP connections/account, Outlook: variable) and cause account lockouts.

The rate limiter uses a sliding-window token bucket per account, per protocol:
- IMAP: 60 requests/minute (configurable via `EMAIL_IMAP_RATE_LIMIT`)
- SMTP: 100 sends/hour (configurable via `EMAIL_SMTP_RATE_LIMIT`)

**Why per-account?** A multi-account server shouldn't let one account's traffic starve another. Each account gets its own token bucket.

**Why not per-connection?** Connections are pooled and reused. Rate limiting at the connection level would reset on reconnect, defeating the purpose.

### Why Retry with PermanentError Short-Circuit

Transient failures (network blips, server busy) should retry with exponential backoff. Permanent failures (auth rejected, mailbox not found) should fail immediately.

`retry.WithRetry` accepts an `IsRetryableFunc` that classifies errors. But some errors are unconditionally permanent regardless of classification — for example, a failed reconnect attempt. Wrapping these in `PermanentError` bypasses the classifier and aborts the retry loop immediately.

**Backoff formula:** `min(baseDelay * 2^attempt + jitter, maxDelay)` where jitter is ±25% of the delay. Defaults: 1s base, 30s max, 3 attempts.

### Why Composite Message IDs

Every message ID in the system is `{accountID}:{mailbox}:{uid}` — for example, `gmail:INBOX:42`. This single string encodes everything needed to route a CRUD operation: which account to connect to, which mailbox to select, and which message to operate on.

**Why not separate parameters?** MCP tool calls pass arguments as a flat JSON object. Having `id` as a single string is simpler for the LLM to handle than three separate parameters (`account`, `mailbox`, `uid`) on every operation. The LLM gets an ID from `email_list` and passes it directly to `email_get`, `email_move`, etc.

### Why Connector Adapter (go-imap Isolation)

go-imap v2 uses a two-step API: `cmd := client.Fetch(...)` then `data, err := cmd.Wait()`. The `Connector` interface collapses this into single calls (`Fetch(ctx, ...) ([]FetchResult, error)`).

**Why:** go-imap is still in beta (v2.0.0-beta.8). The adapter isolates the rest of the codebase from API changes. If go-imap's API changes in a future beta or stable release, only `connector.go` needs updating — the 20-method `Operations` interface and all tool handlers remain unchanged.

**Testing benefit:** The adapter's 14 methods are thin delegation (no logic to test). Unit tests mock the `Connector` interface instead of the raw go-imap client, keeping test setup simple.

### Why SMTP Pool is Lightweight

IMAP connections are long-lived (select mailbox, fetch messages, search). SMTP connections are short-lived (connect, authenticate, send, disconnect). The SMTP "pool" is really just a factory that creates a client per account with shared configuration.

**Why not connection pooling for SMTP?** SMTP servers (especially Gmail) often close connections after a single send or after a short idle timeout. Keeping SMTP connections open adds complexity (keepalive, stale detection) for minimal benefit. Connect-per-send with retry is simpler and more reliable.

### Why wg.Add(1)/Done() in Pool Methods

Every public `Pool` method that performs I/O starts with `p.wg.Add(1)` and defers `p.wg.Done()`. This tracks in-flight operations for graceful shutdown.

When `Pool.Close()` is called:
1. `closing` atomic flag is set — new `Get()` calls return `ErrPoolClosed`
2. `p.wg.Wait()` blocks until all in-flight operations complete
3. After drain, all client connections are closed

**Why not just close connections immediately?** An in-progress IMAP FETCH would get an I/O error mid-transfer, potentially leaving the connection in an inconsistent state. Waiting for drain ensures clean shutdown. The configurable timeout (`EMAIL_POOL_CLOSE_TIMEOUT_MS`) prevents indefinite waiting.

## What Doesn't Belong in an MCP Server

MCP is request-response: the client calls a tool, the server returns a result. There is no persistent event channel, no background daemon, and no UI access. Features that require these don't fit:

| Feature | Why Not |
|---------|---------|
| IMAP IDLE / push | No push channel in MCP — the server can't notify the client of new mail |
| Email scheduling | Requires a 24/7 daemon; MCP servers are started/stopped by the client |
| AI triage presets | Classification belongs in the LLM/prompt layer, not transport |
| Desktop notifications | MCP servers have no UI access; that's the client's job |
| Background sync | Same issue as scheduling — assumes persistent uptime |

**What does fit:** Connection pooling (reuse across sequential tool calls), rate limiting (protect against aggressive agents), retry with backoff (transparent transient failure recovery), provider auto-detection (simplify user config).
