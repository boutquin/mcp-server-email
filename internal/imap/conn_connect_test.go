package imap

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/boutquin/mcp-server-email/internal/config"
	"github.com/boutquin/mcp-server-email/internal/retry"
)

// ==================== connect() via dialConn injection ====================

func TestConnect_DialError(t *testing.T) {
	t.Parallel()

	c := &Client{
		account: &config.Account{
			ID:       "test",
			IMAPHost: "imap.example.com",
			IMAPPort: 993,
			Username: "user",
			Password: "pass",
		},
		timeout: 5 * time.Second,
		limiter: retry.NewLimiter(10000, time.Minute),
		retryCfg: retry.Config{
			MaxRetries: 0,
			BaseDelay:  10 * time.Millisecond,
			MaxDelay:   50 * time.Millisecond,
		},
		dialConn: func(_ context.Context, _ string) connResult {
			return connResult{conn: nil, rawConn: nil, err: errDialFailed}
		},
	}

	err := c.connect(t.Context())
	if err == nil {
		t.Fatal("expected error from failed dial")
	}

	if !strings.Contains(err.Error(), "dial failed") {
		t.Errorf("expected dial error, got: %v", err)
	}

	// Verify connection state is not set on failure.
	if c.conn != nil {
		t.Error("expected c.conn to be nil after dial failure")
	}

	if c.rawConn != nil {
		t.Error("expected c.rawConn to be nil after dial failure")
	}
}

func TestConnect_Timeout(t *testing.T) {
	t.Parallel()

	c := &Client{
		account: &config.Account{
			ID:       "test",
			IMAPHost: "imap.example.com",
			IMAPPort: 993,
		},
		timeout: 50 * time.Millisecond,
		limiter: retry.NewLimiter(10000, time.Minute),
		retryCfg: retry.Config{
			MaxRetries: 0,
			BaseDelay:  10 * time.Millisecond,
			MaxDelay:   50 * time.Millisecond,
		},
		dialConn: func(ctx context.Context, _ string) connResult {
			// Block until context is cancelled (simulating slow connection).
			<-ctx.Done()

			return connResult{conn: nil, rawConn: nil, err: ctx.Err()}
		},
	}

	err := c.connect(t.Context())
	if err == nil {
		t.Fatal("expected timeout error")
	}

	if !strings.Contains(err.Error(), "timed out") {
		t.Errorf("expected timeout error, got: %v", err)
	}
}

func TestConnect_DialAddr(t *testing.T) {
	t.Parallel()

	var gotAddr string

	c := &Client{
		account: &config.Account{
			ID:       "test",
			IMAPHost: "mail.example.com",
			IMAPPort: 993,
			Username: "user",
			Password: "pass",
		},
		timeout: 5 * time.Second,
		limiter: retry.NewLimiter(10000, time.Minute),
		retryCfg: retry.Config{
			MaxRetries: 0,
			BaseDelay:  10 * time.Millisecond,
			MaxDelay:   50 * time.Millisecond,
		},
		dialConn: func(_ context.Context, addr string) connResult {
			gotAddr = addr

			return connResult{conn: nil, rawConn: nil, err: errDialFailed}
		},
	}

	_ = c.connect(t.Context())

	wantAddr := "mail.example.com:993"
	if gotAddr != wantAddr {
		t.Errorf("expected addr %q, got %q", wantAddr, gotAddr)
	}
}
