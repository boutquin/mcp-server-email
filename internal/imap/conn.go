package imap

import (
	"context"
	"crypto/tls"
	"fmt"
	"log/slog"
	"net"
	"strconv"
	"time"

	"github.com/boutquin/mcp-server-email/internal/config"
	"github.com/boutquin/mcp-server-email/internal/retry"
	"github.com/emersion/go-imap/v2/imapclient"
)

// connResult holds the result of a connection attempt.
type connResult struct {
	conn    *imapclient.Client
	rawConn net.Conn
	err     error
}

// connect establishes a new IMAP connection and logs in.
func (c *Client) connect(ctx context.Context) error {
	start := time.Now()
	addr := net.JoinHostPort(c.account.IMAPHost, strconv.Itoa(c.account.IMAPPort))

	c.log().Debug("connecting", slog.String("addr", addr))

	// Apply timeout to the entire connect+login sequence
	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	done := make(chan connResult, 1)

	go func() {
		dial := c.dialConn
		if dial == nil {
			dial = c.dialConnection
		}

		dialResult := dial(ctx, addr)

		adapter := &imapclientAdapter{client: dialResult.conn}

		done <- c.authenticateResult(dialResult, adapter)
	}()

	select {
	case result := <-done:
		if result.err != nil {
			return result.err
		}

		c.conn = &imapclientAdapter{client: result.conn}
		c.rawConn = result.rawConn
		c.lastActive = time.Now()

		c.log().Info("connected", slog.Duration("duration", time.Since(start)))

		return nil
	case <-ctx.Done():
		c.log().Warn("connect timeout", slog.Duration("timeout", c.timeout))

		return fmt.Errorf("connect timed out after %v: %w", c.timeout, ctx.Err())
	}
}

// dialSTARTTLS connects plain then upgrades via STARTTLS.
func (c *Client) dialSTARTTLS(addr string) connResult {
	dialer := &net.Dialer{Timeout: c.timeout}

	plainConn, err := dialer.Dial("tcp", addr)
	if err != nil {
		return connResult{nil, nil, fmt.Errorf("connect to %s: %w", addr, err)}
	}

	conn, err := imapclient.NewStartTLS(plainConn, &imapclient.Options{
		TLSConfig: &tls.Config{
			ServerName:         c.account.IMAPHost,
			MinVersion:         tls.VersionTLS12,
			InsecureSkipVerify: c.account.InsecureSkipVerify, //nolint:gosec // configurable for testing/dev
		},
	})
	if err != nil {
		_ = plainConn.Close()

		return connResult{nil, nil, fmt.Errorf("STARTTLS: %w", err)}
	}

	return connResult{conn, plainConn, nil}
}

// dialTLS connects using implicit TLS (port 993).
func (c *Client) dialTLS(ctx context.Context, addr string) connResult {
	tlsDialer := &tls.Dialer{
		NetDialer: &net.Dialer{Timeout: c.timeout},
		Config: &tls.Config{
			ServerName:         c.account.IMAPHost,
			MinVersion:         tls.VersionTLS12,
			InsecureSkipVerify: c.account.InsecureSkipVerify, //nolint:gosec // configurable for testing/dev
		},
	}

	tlsConn, err := tlsDialer.DialContext(ctx, "tcp", addr)
	if err != nil {
		return connResult{nil, nil, fmt.Errorf("TLS connect to %s: %w", addr, err)}
	}

	return connResult{imapclient.New(tlsConn, nil), tlsConn, nil}
}

// forceClose closes the raw connection without waiting for the mutex.
// This is used to unblock hung operations on timeout.
// The operation holding the mutex will get an I/O error and release it.
// Sets connInvalid flag to signal that reconnection is needed.
func (c *Client) forceClose() {
	c.log().Warn("force-closing connection")

	// Mark connection as invalid BEFORE closing - this signals to retry logic
	// that a reconnect is needed before the next attempt
	c.connInvalid.Store(true)

	// Close raw connection directly - this is safe to call concurrently
	// and will cause any pending I/O to fail immediately
	if c.rawConn != nil {
		_ = c.rawConn.Close()
	}
}

// reconnect closes any existing connection and establishes a new one.
// Called when connInvalid flag is set, indicating the connection was force-closed.
func (c *Client) reconnect(ctx context.Context) error {
	c.log().Info("reconnecting")

	c.mu.Lock()
	// Clean up old connection state
	if c.conn != nil {
		_ = c.conn.Close()
		c.conn = nil
		c.rawConn = nil
	}
	c.mu.Unlock()

	// Clear the invalid flag and folder role cache
	c.connInvalid.Store(false)

	c.folderRoleMu.Lock()
	c.folderRoles = nil
	c.folderRoleMu.Unlock()

	// Establish new connection
	return c.connect(ctx)
}

// retryOp runs an IMAP operation with retry, rate limiting, and reconnect logic.
//
// The retry flow: rate limit token → check connInvalid → reconnect if needed → run op with timeout.
// If a timeout occurs, withTimeout calls forceClose() which sets connInvalid=true and closes
// the raw socket. On the next retry attempt, connInvalid is detected here and reconnect is
// called before retrying the operation. If reconnect itself fails, it's wrapped as a
// PermanentError to abort the retry loop immediately — no point retrying if we can't connect.
//
//nolint:wrapcheck // retryOp is a thin delegation; callers wrap the error with operation context.
func (c *Client) retryOp(ctx context.Context, op func() error) error {
	return retry.WithRetry(ctx, c.retryCfg, c.limiter, retry.IsRetryable, func() error {
		if c.connInvalid.Load() {
			err := c.reconnect(ctx)
			if err != nil {
				return &retry.PermanentError{Err: fmt.Errorf("reconnect failed: %w", err)}
			}
		}

		return c.withTimeout(ctx, op)
	})
}

// withTimeout runs an operation with the client's configured timeout.
// If the operation doesn't complete within the timeout, it returns context.DeadlineExceeded.
// On timeout, the raw connection is force-closed to unblock the hung operation,
// and connInvalid flag is set to signal that reconnection is needed before retry.
func (c *Client) withTimeout(ctx context.Context, op func() error) error {
	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	done := make(chan error, 1)

	go func() {
		done <- op()
	}()

	select {
	case err := <-done:
		return err
	case <-ctx.Done():
		// Force-close the raw connection to unblock any hung I/O.
		// This sets connInvalid flag and closes the underlying socket.
		// The operation holding the mutex will get an I/O error and release it.
		c.forceClose()

		// Wait for the operation to finish - it will fail due to closed socket.
		// This ensures we don't have concurrent access to connection state.
		<-done

		return fmt.Errorf("operation timed out after %v: %w", c.timeout, ctx.Err())
	}
}

// dialConnection dispatches to the TLS or STARTTLS dial path based on account config.
func (c *Client) dialConnection(ctx context.Context, addr string) connResult {
	if c.account.IMAPUseTLS() {
		return c.dialTLS(ctx, addr)
	}

	return c.dialSTARTTLS(addr)
}

// authenticateResult authenticates a dial result. If the dial already failed,
// it returns the error immediately. On auth failure, it closes the connection
// and returns a wrapped error.
func (c *Client) authenticateResult(result connResult, conn Connector) connResult {
	if result.err != nil {
		return result
	}

	err := c.authenticateConn(conn)
	if err != nil {
		_ = conn.Close()

		return connResult{nil, nil, fmt.Errorf("IMAP auth: %w", err)}
	}

	return result
}

func newClient(
	ctx context.Context,
	account *config.Account,
	cfg *config.Config,
) (*Client, error) {
	client := &Client{
		account: account,
		timeout: time.Duration(cfg.IMAPTimeoutMS) * time.Millisecond,
		limiter: retry.NewLimiter(cfg.IMAPRateLimitRPM, time.Minute),
		retryCfg: retry.Config{
			MaxRetries: retry.DefaultMaxRetries,
			BaseDelay:  retry.DefaultBaseDelay,
			MaxDelay:   retry.DefaultMaxDelay,
		},
		debug:  cfg.Debug,
		logger: slog.Default().With(slog.String("component", "imap"), slog.String("account_id", account.ID)),
	}

	err := client.connect(ctx)
	if err != nil {
		return nil, err
	}

	return client, nil
}
