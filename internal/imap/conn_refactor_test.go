package imap

import (
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/boutquin/mcp-server-email/internal/config"
	"github.com/boutquin/mcp-server-email/internal/retry"
)

// Test sentinel errors for refactoring tests.
var errDialFailed = errors.New("dial failed")

// --- dialConnection tests ---

// TestDialConnection_TLS verifies that dialConnection uses the TLS path
// when IMAPUseTLS() returns true.
func TestDialConnection_TLS(t *testing.T) {
	t.Parallel()

	c := &Client{
		account: &config.Account{
			ID:       "test",
			IMAPHost: "localhost",
			IMAPPort: 993, // port 993 → IMAPUseTLS() returns true
		},
		timeout: 2 * time.Second,
		limiter: retry.NewLimiter(10000, time.Minute),
	}

	result := c.dialConnection(t.Context(), "localhost:1")

	// Expect a TLS connection error (not STARTTLS)
	if result.err == nil {
		t.Fatal("expected error from dialConnection with invalid address")
	}

	// TLS dial errors contain "TLS connect" in the error message
	if !strings.Contains(result.err.Error(), "TLS connect") {
		t.Errorf("expected TLS connect error, got: %v", result.err)
	}
}

// TestDialConnection_STARTTLS verifies that dialConnection uses the STARTTLS path
// when IMAPUseTLS() returns false.
func TestDialConnection_STARTTLS(t *testing.T) {
	t.Parallel()

	c := &Client{
		account: &config.Account{
			ID:       "test",
			IMAPHost: "localhost",
			IMAPPort: 143, // port 143 → IMAPUseTLS() returns false (STARTTLS)
		},
		timeout: 2 * time.Second,
		limiter: retry.NewLimiter(10000, time.Minute),
	}

	result := c.dialConnection(t.Context(), "localhost:1")

	// Expect a STARTTLS connection error (plain TCP connect fails)
	if result.err == nil {
		t.Fatal("expected error from dialConnection with invalid address")
	}

	// STARTTLS dial errors contain "connect to" in the error message
	if !strings.Contains(result.err.Error(), "connect to") {
		t.Errorf("expected 'connect to' error for STARTTLS path, got: %v", result.err)
	}
}

// --- authenticateResult tests ---

// TestAuthenticateResult_Success verifies successful authentication returns the
// original connResult with no error.
func TestAuthenticateResult_Success(t *testing.T) {
	t.Parallel()

	mock := &mockConnector{}

	c := &Client{
		account: &config.Account{
			ID:       "test",
			Username: "user",
			Password: "pass",
		},
	}

	input := connResult{
		conn:    nil, // not used directly — adapter wraps it
		rawConn: &mockNetConn{},
		err:     nil,
	}

	result := c.authenticateResult(input, mock)

	if result.err != nil {
		t.Fatalf("expected nil error, got: %v", result.err)
	}

	if result.rawConn != input.rawConn {
		t.Error("expected rawConn to be preserved")
	}
}

// TestAuthenticateResult_AuthError verifies that when authentication fails,
// the connector is closed and the error is wrapped.
func TestAuthenticateResult_AuthError(t *testing.T) {
	t.Parallel()

	closeCalled := false

	mock := &mockConnector{
		loginFn: func(_, _ string) error {
			return fmt.Errorf("login: %w", errTestAuth)
		},
		closeFn: func() error {
			closeCalled = true

			return nil
		},
	}

	c := &Client{
		account: &config.Account{
			ID:       "test",
			Username: "user",
			Password: "pass",
		},
	}

	input := connResult{
		conn:    nil,
		rawConn: &mockNetConn{},
		err:     nil,
	}

	result := c.authenticateResult(input, mock)

	if result.err == nil {
		t.Fatal("expected error from authentication failure")
	}

	if !strings.Contains(result.err.Error(), "IMAP auth") {
		t.Errorf("expected 'IMAP auth' in error, got: %v", result.err)
	}

	if !closeCalled {
		t.Error("expected connector to be closed on auth failure")
	}

	if result.conn != nil {
		t.Error("expected conn to be nil after auth failure")
	}

	if result.rawConn != nil {
		t.Error("expected rawConn to be nil after auth failure")
	}
}

// TestAuthenticateResult_DialError verifies that when the input connResult
// already has an error, authenticateResult returns it immediately without
// attempting authentication.
func TestAuthenticateResult_DialError(t *testing.T) {
	t.Parallel()

	loginCalled := false

	mock := &mockConnector{
		loginFn: func(_, _ string) error {
			loginCalled = true

			return nil
		},
	}

	c := &Client{
		account: &config.Account{ID: "test"},
	}

	input := connResult{
		conn:    nil,
		rawConn: nil,
		err:     errDialFailed,
	}

	result := c.authenticateResult(input, mock)

	if !errors.Is(result.err, errDialFailed) {
		t.Errorf("expected dial error, got: %v", result.err)
	}

	if loginCalled {
		t.Error("login should not be called when dial already failed")
	}
}
