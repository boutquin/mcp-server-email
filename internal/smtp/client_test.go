package smtp

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"testing"
	"time"

	"golang.org/x/oauth2"

	"github.com/boutquin/mcp-server-email/internal/config"
	"github.com/boutquin/mcp-server-email/internal/retry"
	"github.com/wneessen/go-mail"
)

func TestNewPool(t *testing.T) {
	t.Parallel()

	cfg := &config.Config{
		Accounts: []config.Account{
			{ID: "acc1", Email: "one@example.com", SMTPHost: "smtp.example.com", SMTPPort: 587},
			{ID: "acc2", Email: "two@example.com", SMTPHost: "smtp.example.com", SMTPPort: 465},
		},
		DefaultAccount:   "acc1",
		SMTPRateLimitRPH: 100,
		SMTPTimeoutMS:    30000,
	}

	pool := NewPool(cfg)

	if pool == nil {
		t.Fatal("NewPool returned nil")
	}

	if len(pool.accounts) != 2 {
		t.Errorf("expected 2 accounts, got %d", len(pool.accounts))
	}

	if pool.cfg != cfg {
		t.Error("pool.cfg not set correctly")
	}
}

func TestPool_Get(t *testing.T) {
	t.Parallel()

	cfg := &config.Config{
		Accounts: []config.Account{
			{ID: "acc1", Email: "one@example.com", SMTPHost: "smtp.example.com", SMTPPort: 587},
			{ID: "acc2", Email: "two@example.com", SMTPHost: "smtp.example.com", SMTPPort: 465},
		},
		DefaultAccount:   "acc1",
		SMTPRateLimitRPH: 100,
		SMTPTimeoutMS:    30000,
	}

	pool := NewPool(cfg)

	t.Run("get by ID", func(t *testing.T) {
		t.Parallel()

		client, err := pool.Get("acc2")
		if err != nil {
			t.Fatalf("Get() error = %v", err)
		}

		if client.AccountID() != "acc2" {
			t.Errorf("expected account ID 'acc2', got %q", client.AccountID())
		}
	})

	t.Run("get default", func(t *testing.T) {
		t.Parallel()

		client, err := pool.Get("")
		if err != nil {
			t.Fatalf("Get() error = %v", err)
		}

		if client.AccountID() != "acc1" {
			t.Errorf("expected default account 'acc1', got %q", client.AccountID())
		}
	})

	t.Run("reuse cached client", func(t *testing.T) {
		t.Parallel()

		client1, _ := pool.Get("acc1")
		client2, _ := pool.Get("acc1")

		if client1 != client2 {
			t.Error("expected same client instance to be reused")
		}
	})

	t.Run("account not found", func(t *testing.T) {
		t.Parallel()

		_, err := pool.Get("nonexistent")
		if err == nil {
			t.Error("expected error for nonexistent account")
		}

		if !strings.Contains(err.Error(), "account not found") {
			t.Errorf("expected 'account not found' error, got: %v", err)
		}
	})
}

func TestClient_RateLimitInfo(t *testing.T) {
	t.Parallel()

	client := &Client{
		account: &config.Account{ID: "test"},
		limiter: retry.NewLimiter(100, time.Hour),
	}

	// Consume some tokens to test remaining count
	for range 25 {
		_ = retry.WaitForToken(t.Context(), client.limiter)
	}

	remaining, limit, resetAt := client.RateLimitInfo()

	if remaining != 75 {
		t.Errorf("expected remaining 75, got %d", remaining)
	}

	if limit != 100 {
		t.Errorf("expected limit 100, got %d", limit)
	}

	if resetAt.Before(time.Now()) {
		t.Error("resetAt should be in the future")
	}
}

func TestClient_RateLimitInfo_TokenReset(t *testing.T) {
	t.Parallel()

	// Use a very short window so it expires immediately
	client := &Client{
		account: &config.Account{ID: "test"},
		limiter: retry.NewLimiter(100, 1*time.Millisecond),
	}

	// Drain some tokens
	for range 90 {
		_ = retry.WaitForToken(t.Context(), client.limiter)
	}

	time.Sleep(5 * time.Millisecond) // window expires

	remaining, limit, _ := client.RateLimitInfo()

	// Tokens should be reset to limit
	if remaining != 100 {
		t.Errorf("expected remaining to be reset to 100, got %d", remaining)
	}

	if limit != 100 {
		t.Errorf("expected limit 100, got %d", limit)
	}
}

func TestClient_AccountID(t *testing.T) {
	t.Parallel()

	client := &Client{
		account: &config.Account{ID: "test-account"},
	}

	if client.AccountID() != "test-account" {
		t.Errorf("AccountID() = %q, want %q", client.AccountID(), "test-account")
	}
}

func TestClient_Email(t *testing.T) {
	t.Parallel()

	client := &Client{
		account: &config.Account{Email: "test@example.com"},
	}

	if client.Email() != "test@example.com" {
		t.Errorf("Email() = %q, want %q", client.Email(), "test@example.com")
	}
}

func TestSmtpIsRetryable(t *testing.T) {
	t.Parallel()

	tests := []struct {
		errMsg string
		want   bool
	}{
		{"connection reset by peer", true},
		{"timeout waiting for response", true},
		{"operation timed out", true},
		{"temporary failure", true},
		{"try again later", true},
		{"broken pipe", true},
		{"connection refused", true},
		{"421 service unavailable", true}, // 4xx error
		{"451 temporary error", true},     // 4xx error
		{"authentication failed", false},
		{"550 mailbox not found", false}, // 5xx error
		{"", false},
	}

	for _, tt := range tests {
		t.Run(tt.errMsg, func(t *testing.T) {
			t.Parallel()

			var err error
			if tt.errMsg != "" {
				err = &testError{msg: tt.errMsg}
			}

			if got := smtpIsRetryable(err); got != tt.want {
				t.Errorf("smtpIsRetryable(%q) = %v, want %v", tt.errMsg, got, tt.want)
			}
		})
	}

	// Test nil error
	if smtpIsRetryable(nil) {
		t.Error("smtpIsRetryable(nil) should return false")
	}

	// Test context.DeadlineExceeded
	if !smtpIsRetryable(context.DeadlineExceeded) {
		t.Error("smtpIsRetryable(context.DeadlineExceeded) should return true")
	}
}

type testError struct {
	msg string
}

func (e *testError) Error() string {
	return e.msg
}

func TestSmtpIsRetryable_FalsePositives(t *testing.T) {
	t.Parallel()

	// These error messages contain the digit "4" but are NOT 4xx SMTP errors.
	falsePositives := []struct {
		errMsg string
	}{
		{"connection to port 443 failed"},
		{"IPv4 address not reachable"},
		{"retry after 4 seconds"},
	}

	for _, tt := range falsePositives {
		t.Run(tt.errMsg, func(t *testing.T) {
			t.Parallel()

			err := &testError{msg: tt.errMsg}
			if smtpIsRetryable(err) {
				t.Errorf("smtpIsRetryable(%q) = true, want false (false positive)", tt.errMsg)
			}
		})
	}
}

func TestClient_WithTimeout_Success(t *testing.T) {
	t.Parallel()

	client := &Client{
		timeout: 5 * time.Second,
	}

	err := client.withTimeout(context.Background(), func() error {
		return nil
	})
	if err != nil {
		t.Errorf("expected nil error, got: %v", err)
	}
}

func TestClient_WithTimeout_OpError(t *testing.T) {
	t.Parallel()

	client := &Client{
		timeout: 5 * time.Second,
	}

	err := client.withTimeout(context.Background(), func() error {
		return &testError{msg: "operation failed"}
	})
	if err == nil {
		t.Error("expected error, got nil")
	}

	if !strings.Contains(err.Error(), "operation failed") {
		t.Errorf("expected 'operation failed' in error, got: %v", err)
	}
}

func TestClient_WithTimeout_Exceeded(t *testing.T) {
	t.Parallel()

	client := &Client{
		timeout: 50 * time.Millisecond,
	}

	err := client.withTimeout(context.Background(), func() error {
		time.Sleep(2 * time.Second)

		return nil
	})
	if err == nil {
		t.Error("expected timeout error, got nil")
	}

	if !strings.Contains(err.Error(), "timed out") {
		t.Errorf("expected 'timed out' in error, got: %v", err)
	}
}

func TestClient_BuildMessage_WithReplyHeaders(t *testing.T) {
	t.Parallel()

	client := &Client{
		account: &config.Account{Email: "sender@example.com"},
	}

	req := &SendRequest{
		To:         []string{"recipient@example.com"},
		Subject:    "Re: Test",
		Body:       "Reply body",
		InReplyTo:  "<original@example.com>",
		References: []string{"<original@example.com>"},
	}

	msg, err := client.buildMessage(req)
	if err != nil {
		t.Fatalf("buildMessage() error = %v", err)
	}

	if msg == nil {
		t.Fatal("buildMessage() returned nil")
	}
}

func TestSendRequest(t *testing.T) {
	t.Parallel()

	req := &SendRequest{
		To:      []string{"recipient@example.com"},
		CC:      []string{"cc@example.com"},
		BCC:     []string{"bcc@example.com"},
		Subject: "Test Subject",
		Body:    "Test body",
		IsHTML:  false,
		ReplyTo: "reply@example.com",
	}

	if len(req.To) != 1 || req.To[0] != "recipient@example.com" {
		t.Errorf("To = %v", req.To)
	}

	if req.Subject != "Test Subject" {
		t.Errorf("Subject = %q", req.Subject)
	}

	if req.IsHTML {
		t.Error("IsHTML should be false")
	}

	if req.ReplyTo != "reply@example.com" {
		t.Errorf("ReplyTo = %q", req.ReplyTo)
	}
}

// ==================== Pool.Close ====================

func TestPool_Close(t *testing.T) {
	t.Parallel()

	cfg := &config.Config{
		Accounts: []config.Account{
			{ID: "acc1", Email: "a@example.com", SMTPHost: "smtp.example.com", SMTPPort: 587},
		},
		DefaultAccount:   "acc1",
		SMTPRateLimitRPH: 100,
		SMTPTimeoutMS:    30000,
	}

	pool := NewPool(cfg)
	pool.Close()

	if !pool.closing.Load() {
		t.Error("closing flag should be true after Close()")
	}

	_, err := pool.Get("acc1")
	if !errors.Is(err, ErrPoolClosed) {
		t.Errorf("Get after Close: expected ErrPoolClosed, got %v", err)
	}
}

// ==================== Client.log ====================

func TestClient_Log_NilFallback(t *testing.T) {
	t.Parallel()

	client := &Client{
		account: &config.Account{ID: "test"},
	}

	logger := client.log()
	if logger == nil {
		t.Fatal("log() returned nil")
	}

	if logger != nopLogger {
		t.Error("expected nopLogger for nil logger field")
	}
}

func TestClient_Log_WithLogger(t *testing.T) {
	t.Parallel()

	custom := slog.Default()

	client := &Client{
		account: &config.Account{ID: "test"},
		logger:  custom,
	}

	logger := client.log()
	if logger != custom {
		t.Error("expected custom logger to be returned")
	}
}

// ==================== nopWriter ====================

func TestNopWriter_Write(t *testing.T) {
	t.Parallel()

	w := nopWriter{}

	n, err := w.Write([]byte("test data"))
	if err != nil {
		t.Errorf("Write() error = %v", err)
	}

	if n != 9 {
		t.Errorf("Write() n = %d, want 9", n)
	}
}

// ==================== attachFiles ====================

func TestAttachFiles_FileBased(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := dir + "/test.txt"

	err := writeTestFile(t, path, "file content")
	if err != nil {
		t.Fatalf("write test file: %v", err)
	}

	m := mail.NewMsg()

	err = attachFiles(m, []SendAttachment{
		{Path: path, Filename: "test.txt"},
	})
	if err != nil {
		t.Errorf("attachFiles() error = %v", err)
	}
}

func TestAttachFiles_InMemory(t *testing.T) {
	t.Parallel()

	m := mail.NewMsg()

	err := attachFiles(m, []SendAttachment{
		{Filename: "data.bin", Data: []byte("binary content")},
	})
	if err != nil {
		t.Errorf("attachFiles() error = %v", err)
	}
}

func TestAttachFiles_Empty(t *testing.T) {
	t.Parallel()

	m := mail.NewMsg()

	err := attachFiles(m, nil)
	if err != nil {
		t.Errorf("attachFiles(nil) error = %v", err)
	}
}

func writeTestFile(t *testing.T, path, content string) error {
	t.Helper()

	err := os.WriteFile(path, []byte(content), 0o600)
	if err != nil {
		return fmt.Errorf("write test file: %w", err)
	}

	return nil
}

// ==================== Test Sentinels ====================

var (
	errTestTokenExpired  = errors.New("token expired")
	errTestConnRefused   = errors.New("connection refused")
	errTest550Mailbox    = errors.New("550 mailbox not found")
	errTestRefreshFailed = errors.New("refresh failed")
	errTest535Auth       = errors.New("535 Authentication failed")
	errTest535Retry      = errors.New("535 auth failed again")
	errTestDialerCreate  = errors.New("dialer creation failed")
)

// ==================== Mock Dialer ====================

type mockDialer struct {
	err error
}

func (m *mockDialer) DialAndSend(_ ...*mail.Msg) error {
	return m.err
}

// ==================== Mock Token Source ====================

type mockTokenSource struct {
	tokens []*oauth2.Token
	index  int
	err    error
}

func (m *mockTokenSource) Token() (*oauth2.Token, error) {
	if m.err != nil {
		return nil, m.err
	}

	if m.index >= len(m.tokens) {
		return m.tokens[len(m.tokens)-1], nil
	}

	tok := m.tokens[m.index]
	m.index++

	return tok, nil
}

// ==================== Test Helpers ====================

func newDialTestClient(t *testing.T, account *config.Account) *Client {
	t.Helper()

	return &Client{
		account: account,
		timeout: 5 * time.Second,
		limiter: retry.NewLimiter(100, time.Hour),
		logger:  nopLogger,
	}
}

func setMockDialer(t *testing.T, c *Client, err error) {
	t.Helper()

	c.newDialer = func(_ ...mail.Option) (dialer, error) {
		return &mockDialer{err: err}, nil
	}
}

// ==================== tlsOptions Tests ====================

func TestClient_TlsOptions(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name               string
		port               int
		insecureSkipVerify bool
		wantLen            int
	}{
		{"implicit TLS port 465", 465, false, 1},
		{"STARTTLS port 587", 587, false, 1},
		{"implicit TLS with insecure", 465, true, 2},
		{"STARTTLS with insecure", 587, true, 2},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			c := newDialTestClient(t, &config.Account{
				SMTPPort:           tt.port,
				InsecureSkipVerify: tt.insecureSkipVerify,
			})

			opts := c.tlsOptions()
			if len(opts) != tt.wantLen {
				t.Errorf("tlsOptions() returned %d opts, want %d", len(opts), tt.wantLen)
			}
		})
	}
}

// ==================== dialAndSend Tests ====================

func TestClient_DialAndSend_HappyPath(t *testing.T) {
	t.Parallel()

	c := newDialTestClient(t, &config.Account{
		Email:    "test@example.com",
		SMTPPort: 587,
		Username: "user",
		Password: "pass",
	})
	setMockDialer(t, c, nil)

	m := mail.NewMsg()
	_ = m.From("test@example.com")
	_ = m.To("recipient@example.com")

	err := c.dialAndSend(m)
	if err != nil {
		t.Errorf("dialAndSend() error = %v", err)
	}
}

func TestClient_DialAndSend_PasswordAuth(t *testing.T) {
	t.Parallel()

	c := newDialTestClient(t, &config.Account{
		Email:    "test@example.com",
		SMTPPort: 587,
		Username: "user",
		Password: "pass",
	})
	setMockDialer(t, c, nil)

	m := mail.NewMsg()
	_ = m.From("test@example.com")
	_ = m.To("recipient@example.com")

	err := c.dialAndSend(m)
	if err != nil {
		t.Errorf("dialAndSend() error = %v", err)
	}
}

func TestClient_DialAndSend_OAuth2Auth(t *testing.T) {
	t.Parallel()

	c := newDialTestClient(t, &config.Account{
		Email:      "test@example.com",
		SMTPPort:   587,
		AuthMethod: "oauth2",
	})

	c.tokenSource = &mockTokenSource{
		tokens: []*oauth2.Token{{AccessToken: "test-token"}},
	}
	setMockDialer(t, c, nil)

	m := mail.NewMsg()
	_ = m.From("test@example.com")
	_ = m.To("recipient@example.com")

	err := c.dialAndSend(m)
	if err != nil {
		t.Errorf("dialAndSend() error = %v", err)
	}
}

func TestClient_DialAndSend_OAuth2TokenError(t *testing.T) {
	t.Parallel()

	c := newDialTestClient(t, &config.Account{
		Email:      "test@example.com",
		SMTPPort:   587,
		AuthMethod: "oauth2",
	})

	c.tokenSource = &mockTokenSource{
		err: errTestTokenExpired,
	}

	m := mail.NewMsg()
	_ = m.From("test@example.com")
	_ = m.To("recipient@example.com")

	err := c.dialAndSend(m)
	if err == nil {
		t.Fatal("expected error")
	}

	if !strings.Contains(err.Error(), "get OAuth2 token") {
		t.Errorf("expected OAuth2 token error, got: %v", err)
	}
}

func TestClient_DialAndSend_DialError(t *testing.T) {
	t.Parallel()

	c := newDialTestClient(t, &config.Account{
		Email:    "test@example.com",
		SMTPPort: 587,
		Username: "user",
		Password: "pass",
	})

	c.newDialer = func(_ ...mail.Option) (dialer, error) {
		return nil, errTestConnRefused
	}

	m := mail.NewMsg()
	_ = m.From("test@example.com")
	_ = m.To("recipient@example.com")

	err := c.dialAndSend(m)
	if err == nil {
		t.Fatal("expected error")
	}

	if !strings.Contains(err.Error(), "create SMTP client") {
		t.Errorf("expected create SMTP client error, got: %v", err)
	}
}

func TestClient_DialAndSend_SendError(t *testing.T) {
	t.Parallel()

	c := newDialTestClient(t, &config.Account{
		Email:    "test@example.com",
		SMTPPort: 587,
		Username: "user",
		Password: "pass",
	})

	setMockDialer(t, c, errTest550Mailbox)

	m := mail.NewMsg()
	_ = m.From("test@example.com")
	_ = m.To("recipient@example.com")

	err := c.dialAndSend(m)
	if err == nil {
		t.Fatal("expected error")
	}

	if !strings.Contains(err.Error(), "send email") {
		t.Errorf("expected send email error, got: %v", err)
	}
}

// ==================== buildTokenSource Tests ====================

func TestPool_BuildTokenSource_MissingOAuthConfig(t *testing.T) {
	t.Parallel()

	pool := NewPool(&config.Config{
		SMTPRateLimitRPH: 100,
		SMTPTimeoutMS:    30000,
	})

	// Account without OAuth2 config should fail.
	account := &config.Account{
		ID:         "test",
		AuthMethod: "oauth2",
	}

	_, err := pool.buildTokenSource(account)
	if err == nil {
		t.Fatal("expected error for missing OAuth2 config")
	}
}

// ==================== retryWithRefreshedToken Tests (P49) ====================

func TestClient_RetryWithRefreshedToken_Success(t *testing.T) {
	t.Parallel()

	c := newDialTestClient(t, &config.Account{
		Email:      "test@example.com",
		SMTPPort:   587,
		AuthMethod: "oauth2",
	})

	c.tokenSource = &mockTokenSource{
		tokens: []*oauth2.Token{{AccessToken: "refreshed-token"}},
	}
	setMockDialer(t, c, nil)

	m := mail.NewMsg()
	_ = m.From("test@example.com")
	_ = m.To("recipient@example.com")

	err := c.retryWithRefreshedToken(m)
	if err != nil {
		t.Errorf("retryWithRefreshedToken() error = %v", err)
	}
}

func TestClient_RetryWithRefreshedToken_TokenError(t *testing.T) {
	t.Parallel()

	c := newDialTestClient(t, &config.Account{
		Email:      "test@example.com",
		SMTPPort:   587,
		AuthMethod: "oauth2",
	})

	c.tokenSource = &mockTokenSource{
		err: errTestRefreshFailed,
	}

	m := mail.NewMsg()
	_ = m.From("test@example.com")
	_ = m.To("recipient@example.com")

	err := c.retryWithRefreshedToken(m)
	if err == nil {
		t.Fatal("expected error")
	}

	if !strings.Contains(err.Error(), "SMTP token refresh") {
		t.Errorf("expected token refresh error, got: %v", err)
	}
}

func TestClient_RetryWithRefreshedToken_RetrySendFails(t *testing.T) {
	t.Parallel()

	c := newDialTestClient(t, &config.Account{
		Email:      "test@example.com",
		SMTPPort:   587,
		AuthMethod: "oauth2",
	})

	c.tokenSource = &mockTokenSource{
		tokens: []*oauth2.Token{{AccessToken: "refreshed-token"}},
	}

	setMockDialer(t, c, errTest535Retry)

	m := mail.NewMsg()
	_ = m.From("test@example.com")
	_ = m.To("recipient@example.com")

	err := c.retryWithRefreshedToken(m)
	if err == nil {
		t.Fatal("expected error")
	}

	if !strings.Contains(err.Error(), "SMTP retry send") {
		t.Errorf("expected retry send error, got: %v", err)
	}
}

func TestClient_RetryWithRefreshedToken_DialerError(t *testing.T) {
	t.Parallel()

	c := newDialTestClient(t, &config.Account{
		Email:      "test@example.com",
		SMTPPort:   587,
		AuthMethod: "oauth2",
	})

	c.tokenSource = &mockTokenSource{
		tokens: []*oauth2.Token{{AccessToken: "refreshed-token"}},
	}

	c.newDialer = func(_ ...mail.Option) (dialer, error) {
		return nil, errTestDialerCreate
	}

	m := mail.NewMsg()
	_ = m.From("test@example.com")
	_ = m.To("recipient@example.com")

	err := c.retryWithRefreshedToken(m)
	if err == nil {
		t.Fatal("expected error")
	}

	if !strings.Contains(err.Error(), "SMTP retry client") {
		t.Errorf("expected retry client error, got: %v", err)
	}
}

// ==================== 535 Retry Flow in dialAndSend (P49) ====================

func TestClient_DialAndSend_535TriggersRetry(t *testing.T) {
	t.Parallel()

	callCount := 0

	c := newDialTestClient(t, &config.Account{
		Email:      "test@example.com",
		SMTPPort:   587,
		AuthMethod: "oauth2",
	})

	c.tokenSource = &mockTokenSource{
		tokens: []*oauth2.Token{
			{AccessToken: "old-token"},
			{AccessToken: "new-token"},
		},
	}

	c.newDialer = func(_ ...mail.Option) (dialer, error) {
		callCount++
		if callCount == 1 {
			return &mockDialer{err: errTest535Auth}, nil
		}

		return &mockDialer{err: nil}, nil
	}

	m := mail.NewMsg()
	_ = m.From("test@example.com")
	_ = m.To("recipient@example.com")

	err := c.dialAndSend(m)
	if err != nil {
		t.Errorf("dialAndSend() should succeed after retry, got: %v", err)
	}

	if callCount != 2 {
		t.Errorf("expected 2 dialer calls (original + retry), got %d", callCount)
	}
}

func TestClient_DialAndSend_550NoRetry(t *testing.T) {
	t.Parallel()

	c := newDialTestClient(t, &config.Account{
		Email:      "test@example.com",
		SMTPPort:   587,
		AuthMethod: "oauth2",
	})

	c.tokenSource = &mockTokenSource{
		tokens: []*oauth2.Token{{AccessToken: "token"}},
	}

	setMockDialer(t, c, errTest550Mailbox)

	m := mail.NewMsg()
	_ = m.From("test@example.com")
	_ = m.To("recipient@example.com")

	err := c.dialAndSend(m)
	if err == nil {
		t.Fatal("expected error")
	}

	if !strings.Contains(err.Error(), "send email") {
		t.Errorf("expected send email error (no retry), got: %v", err)
	}
}

func TestClient_DialAndSend_535WithPasswordNoRetry(t *testing.T) {
	t.Parallel()

	c := newDialTestClient(t, &config.Account{
		Email:    "test@example.com",
		SMTPPort: 587,
		Username: "user",
		Password: "pass",
	})

	setMockDialer(t, c, errTest535Auth)

	m := mail.NewMsg()
	_ = m.From("test@example.com")
	_ = m.To("recipient@example.com")

	err := c.dialAndSend(m)
	if err == nil {
		t.Fatal("expected error")
	}

	if !strings.Contains(err.Error(), "send email") {
		t.Errorf("expected send email error (no retry for password auth), got: %v", err)
	}
}
