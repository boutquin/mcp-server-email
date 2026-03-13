package imap

import (
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"golang.org/x/oauth2"

	"github.com/boutquin/mcp-server-email/internal/config"
	"github.com/boutquin/mcp-server-email/internal/retry"
	"github.com/emersion/go-sasl"
)

// --- Mock TokenSource ---

type mockTokenSource struct {
	tokens []*oauth2.Token
	index  atomic.Int32
	err    error
}

func (m *mockTokenSource) Token() (*oauth2.Token, error) {
	if m.err != nil {
		return nil, m.err
	}

	idx := int(m.index.Add(1) - 1)
	if idx >= len(m.tokens) {
		idx = len(m.tokens) - 1
	}

	return m.tokens[idx], nil
}

// --- Auth Branching Tests ---

func TestAuthenticateConn_OAuth2(t *testing.T) {
	t.Parallel()

	var authCalled bool

	var mechName string

	mock := &mockConnector{
		authenticateFn: func(saslClient sasl.Client) error {
			authCalled = true
			mechName, _, _ = saslClient.Start()

			return nil
		},
	}

	ts := &mockTokenSource{
		tokens: []*oauth2.Token{{AccessToken: "test-token"}},
	}

	client := &Client{
		account: &config.Account{
			ID:         "test",
			Email:      "user@gmail.com",
			AuthMethod: "oauth2",
		},
		conn:        mock,
		tokenSource: ts,
		timeout:     30 * time.Second,
		limiter:     retry.NewLimiter(1000, time.Minute),
	}

	err := client.authenticateConn(mock)
	if err != nil {
		t.Fatalf("authenticateConn() error = %v", err)
	}

	if !authCalled {
		t.Error("Authenticate was not called")
	}

	if mechName != "XOAUTH2" {
		t.Errorf("mechanism = %q, want XOAUTH2", mechName)
	}
}

func TestAuthenticateConn_Password(t *testing.T) {
	t.Parallel()

	var loginCalled bool

	mock := &mockConnector{
		loginFn: func(_, _ string) error {
			loginCalled = true

			return nil
		},
	}

	client := &Client{
		account: &config.Account{
			ID:       "test",
			Email:    "user@gmail.com",
			Username: "user@gmail.com",
			Password: "password123",
		},
		conn:    mock,
		timeout: 30 * time.Second,
		limiter: retry.NewLimiter(1000, time.Minute),
	}

	err := client.authenticateConn(mock)
	if err != nil {
		t.Fatalf("authenticateConn() error = %v", err)
	}

	if !loginCalled {
		t.Error("Login was not called")
	}
}

func TestAuthenticateConn_DefaultIsPassword(t *testing.T) {
	t.Parallel()

	var loginCalled bool

	mock := &mockConnector{
		loginFn: func(_, _ string) error {
			loginCalled = true

			return nil
		},
	}

	// AuthMethod == "" should default to password.
	client := &Client{
		account: &config.Account{
			ID:       "test",
			Email:    "user@gmail.com",
			Username: "user@gmail.com",
			Password: "pass",
		},
		conn:    mock,
		timeout: 30 * time.Second,
		limiter: retry.NewLimiter(1000, time.Minute),
	}

	err := client.authenticateConn(mock)
	if err != nil {
		t.Fatalf("authenticateConn() error = %v", err)
	}

	if !loginCalled {
		t.Error("Login was not called for default auth method")
	}
}

func TestAuthenticateOAuth2_RetryOnAuthFail(t *testing.T) {
	t.Parallel()

	var authCount atomic.Int32

	mock := &mockConnector{
		authenticateFn: func(_ sasl.Client) error {
			count := authCount.Add(1)
			if count == 1 {
				return errTestAuth // AUTHENTICATIONFAILED equivalent
			}

			return nil // second attempt succeeds
		},
	}

	ts := &mockTokenSource{
		tokens: []*oauth2.Token{
			{AccessToken: "old-token"},
			{AccessToken: "new-token"},
		},
	}

	client := &Client{
		account: &config.Account{
			ID:         "test",
			Email:      "user@gmail.com",
			AuthMethod: "oauth2",
		},
		conn:        mock,
		tokenSource: ts,
		timeout:     30 * time.Second,
		limiter:     retry.NewLimiter(1000, time.Minute),
	}

	err := client.authenticateOAuth2(mock)
	if err != nil {
		t.Fatalf("authenticateOAuth2() error = %v", err)
	}

	if authCount.Load() != 2 {
		t.Errorf("Authenticate call count = %d, want 2", authCount.Load())
	}
}

func TestAuthenticateOAuth2_NoRetryOnNetworkError(t *testing.T) {
	t.Parallel()

	var authCount atomic.Int32

	mock := &mockConnector{
		authenticateFn: func(_ sasl.Client) error {
			authCount.Add(1)

			return errTestConnReset
		},
	}

	ts := &mockTokenSource{
		tokens: []*oauth2.Token{{AccessToken: "token"}},
	}

	client := &Client{
		account: &config.Account{
			ID:         "test",
			Email:      "user@gmail.com",
			AuthMethod: "oauth2",
		},
		conn:        mock,
		tokenSource: ts,
		timeout:     30 * time.Second,
		limiter:     retry.NewLimiter(1000, time.Minute),
	}

	err := client.authenticateOAuth2(mock)
	if err == nil {
		t.Fatal("expected error for network failure")
	}

	if authCount.Load() != 1 {
		t.Errorf("Authenticate call count = %d, want 1 (no retry)", authCount.Load())
	}
}

func TestAuthenticateOAuth2_SameTokenNoRetry(t *testing.T) {
	t.Parallel()

	mock := &mockConnector{
		authenticateFn: func(_ sasl.Client) error {
			return errTestAuth
		},
	}

	// Token source always returns the same token.
	ts := &mockTokenSource{
		tokens: []*oauth2.Token{{AccessToken: "same-token"}},
	}

	client := &Client{
		account: &config.Account{
			ID:         "test",
			Email:      "user@gmail.com",
			AuthMethod: "oauth2",
		},
		conn:        mock,
		tokenSource: ts,
		timeout:     30 * time.Second,
		limiter:     retry.NewLimiter(1000, time.Minute),
	}

	err := client.authenticateOAuth2(mock)
	if err == nil {
		t.Fatal("expected error when token is unchanged")
	}

	if !errors.Is(err, ErrTokenUnchanged) {
		t.Errorf("error = %v, want ErrTokenUnchanged", err)
	}
}

func TestIsAuthenticationFailed(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		err  error
		want bool
	}{
		{"nil", nil, false},
		{"auth error", errTestAuth, true},
		{"network error", errTestConnReset, false},
	}

	for _, tt := range tests {
		got := isAuthenticationFailed(tt.err)
		if got != tt.want {
			t.Errorf("isAuthenticationFailed(%v) = %v, want %v", tt.err, got, tt.want)
		}
	}
}
