package smtp

import (
	"context"
	"errors"
	"testing"
	"time"

	"golang.org/x/oauth2"

	"github.com/boutquin/mcp-server-email/internal/config"
	"github.com/boutquin/mcp-server-email/internal/retry"
	"github.com/wneessen/go-mail"
)

// ==================== Pool.Get OAuth2 tests (via buildTokenSourceFunc) ====================

var errTestTokenBuild = errors.New("token source build failed")

func TestPool_Get_OAuth2Success(t *testing.T) {
	t.Parallel()

	cfg := &config.Config{
		Accounts: []config.Account{
			{
				ID:         "oauth-acc",
				Email:      "oauth@example.com",
				SMTPHost:   "smtp.example.com",
				SMTPPort:   587,
				AuthMethod: "oauth2",
			},
		},
		DefaultAccount:   "oauth-acc",
		SMTPRateLimitRPH: 100,
		SMTPTimeoutMS:    30000,
	}

	pool := NewPool(cfg)
	pool.buildTokenSourceFunc = func(_ *config.Account) (oauth2.TokenSource, error) {
		return &mockTokenSource{
			tokens: []*oauth2.Token{{AccessToken: "test-token"}},
		}, nil
	}

	client, err := pool.Get("oauth-acc")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}

	if client.tokenSource == nil {
		t.Error("expected tokenSource to be set for OAuth2 account")
	}

	if client.AccountID() != "oauth-acc" {
		t.Errorf("expected account ID 'oauth-acc', got %q", client.AccountID())
	}
}

func TestPool_Get_OAuth2Error(t *testing.T) {
	t.Parallel()

	cfg := &config.Config{
		Accounts: []config.Account{
			{
				ID:         "oauth-fail",
				Email:      "fail@example.com",
				SMTPHost:   "smtp.example.com",
				SMTPPort:   587,
				AuthMethod: "oauth2",
			},
		},
		DefaultAccount:   "oauth-fail",
		SMTPRateLimitRPH: 100,
		SMTPTimeoutMS:    30000,
	}

	pool := NewPool(cfg)
	pool.buildTokenSourceFunc = func(_ *config.Account) (oauth2.TokenSource, error) {
		return nil, errTestTokenBuild
	}

	_, err := pool.Get("oauth-fail")
	if err == nil {
		t.Fatal("expected error from failing buildTokenSourceFunc")
	}

	if !errors.Is(err, errTestTokenBuild) {
		t.Errorf("expected errTestTokenBuild, got: %v", err)
	}
}

func TestPool_Send_OAuth2Success(t *testing.T) {
	t.Parallel()

	cfg := &config.Config{
		Accounts: []config.Account{
			{
				ID:         "oauth-send",
				Email:      "send@example.com",
				SMTPHost:   "smtp.example.com",
				SMTPPort:   587,
				AuthMethod: "oauth2",
			},
		},
		DefaultAccount:   "oauth-send",
		SMTPRateLimitRPH: 100,
		SMTPTimeoutMS:    30000,
	}

	pool := NewPool(cfg)
	pool.buildTokenSourceFunc = func(_ *config.Account) (oauth2.TokenSource, error) {
		return &mockTokenSource{
			tokens: []*oauth2.Token{{AccessToken: "send-token"}},
		}, nil
	}

	// Get the client first so we can override the dialer.
	client, err := pool.Get("oauth-send")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}

	client.retryCfg = retry.Config{
		MaxRetries: 0,
		BaseDelay:  10 * time.Millisecond,
		MaxDelay:   50 * time.Millisecond,
	}
	client.newDialer = func(_ ...mail.Option) (dialer, error) {
		return &mockDialer{err: nil}, nil
	}

	err = pool.Send(context.Background(), "oauth-send", &SendRequest{
		To:      []string{"to@example.com"},
		Subject: "OAuth2 Test",
		Body:    "Hello from OAuth2",
	})
	if err != nil {
		t.Fatalf("Pool.Send() error = %v", err)
	}
}
