package auth //nolint:testpackage // tests access internal helpers

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"golang.org/x/oauth2"

	"github.com/boutquin/mcp-server-email/internal/config"
)

const (
	testProviderGmail   = "Gmail"
	testProviderOutlook = "Outlook"
)

func TestOAuthConfig_Gmail(t *testing.T) {
	t.Parallel()

	account := &config.Account{
		Email:             "user@gmail.com",
		OAuthClientID:     "client-id",
		OAuthClientSecret: "client-secret",
	}

	cfg, provider, err := OAuthConfig(account)
	if err != nil {
		t.Fatalf("OAuthConfig() error = %v", err)
	}

	if provider.Name != testProviderGmail {
		t.Errorf("provider.Name = %q, want Gmail", provider.Name)
	}

	if cfg.ClientID != "client-id" {
		t.Errorf("ClientID = %q, want client-id", cfg.ClientID)
	}

	if cfg.Endpoint.TokenURL != "https://oauth2.googleapis.com/token" {
		t.Errorf("TokenURL = %q", cfg.Endpoint.TokenURL)
	}
}

func TestOAuthConfig_Outlook(t *testing.T) {
	t.Parallel()

	account := &config.Account{
		Email:             "user@outlook.com",
		OAuthClientID:     "cid",
		OAuthClientSecret: "csec",
	}

	cfg, provider, err := OAuthConfig(account)
	if err != nil {
		t.Fatalf("OAuthConfig() error = %v", err)
	}

	if provider.Name != testProviderOutlook {
		t.Errorf("provider.Name = %q, want Outlook", provider.Name)
	}

	if len(cfg.Scopes) != 3 {
		t.Errorf("Scopes len = %d, want 3", len(cfg.Scopes))
	}
}

func TestOAuthConfig_UnknownDomain(t *testing.T) {
	t.Parallel()

	account := &config.Account{
		Email:         "user@custom-domain.org",
		OAuthClientID: "cid",
	}

	_, _, err := OAuthConfig(account)
	if err == nil {
		t.Fatal("expected error for unknown domain")
	}
}

func TestNewTokenSource_LoadExisting(t *testing.T) {
	t.Parallel()

	store, err := NewTokenStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewTokenStore() error = %v", err)
	}

	original := &oauth2.Token{
		AccessToken:  "valid-token",
		TokenType:    "Bearer",
		RefreshToken: "refresh",
		Expiry:       time.Now().Add(time.Hour),
	}

	err = store.Save("acct", original)
	if err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	oauthCfg := &oauth2.Config{
		ClientID:     "cid",
		ClientSecret: "csec",
		Endpoint: oauth2.Endpoint{ //nolint:gosec // G101: test fixture URL, not credentials
			TokenURL: "https://example.com/token",
		},
	}

	ts, err := NewTokenSource(store, "acct", oauthCfg)
	if err != nil {
		t.Fatalf("NewTokenSource() error = %v", err)
	}

	token, err := ts.Token()
	if err != nil {
		t.Fatalf("Token() error = %v", err)
	}

	if token.AccessToken != "valid-token" {
		t.Errorf("AccessToken = %q, want valid-token", token.AccessToken)
	}
}

func TestNewTokenSource_NoExistingToken(t *testing.T) {
	t.Parallel()

	store, err := NewTokenStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewTokenStore() error = %v", err)
	}

	oauthCfg := &oauth2.Config{ClientID: "cid"}

	_, err = NewTokenSource(store, "missing", oauthCfg)
	if err == nil {
		t.Fatal("expected error when no token file exists")
	}

	if !errors.Is(err, ErrTokenNotFound) {
		t.Errorf("error = %v, want ErrTokenNotFound wrapped", err)
	}
}

func TestTokenSource_RefreshSavesToStore(t *testing.T) {
	t.Parallel()

	store, err := NewTokenStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewTokenStore() error = %v", err)
	}

	// Seed with an expired token.
	expired := &oauth2.Token{
		AccessToken:  "old-access",
		RefreshToken: "refresh-token",
		Expiry:       time.Now().Add(-time.Hour),
	}

	err = store.Save("acct", expired)
	if err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	// Mock token server that returns a new token on refresh.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"access_token":  "new-access",
			"token_type":    "Bearer",
			"refresh_token": "refresh-token",
			"expires_in":    3600,
		})
	}))
	defer srv.Close()

	oauthCfg := &oauth2.Config{
		ClientID:     "cid",
		ClientSecret: "csec",
		Endpoint: oauth2.Endpoint{
			TokenURL: srv.URL,
		},
	}

	ts, err := NewTokenSource(store, "acct", oauthCfg)
	if err != nil {
		t.Fatalf("NewTokenSource() error = %v", err)
	}

	token, err := ts.Token()
	if err != nil {
		t.Fatalf("Token() error = %v", err)
	}

	if token.AccessToken != "new-access" {
		t.Errorf("AccessToken = %q, want new-access", token.AccessToken)
	}

	// Verify the refreshed token was persisted.
	loaded, err := store.Load("acct")
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if loaded.AccessToken != "new-access" {
		t.Errorf("persisted AccessToken = %q, want new-access", loaded.AccessToken)
	}
}

func TestDeviceCodeAuth_Success(t *testing.T) {
	t.Parallel()

	var pollCount atomic.Int32

	mux := http.NewServeMux()
	mux.HandleFunc("/device", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"device_code":      "dev-code",
			"user_code":        "USER-1234",
			"verification_uri": "https://example.com/verify",
			"interval":         1,
			"expires_in":       300,
		})
	})
	mux.HandleFunc("/token", func(w http.ResponseWriter, _ *http.Request) {
		count := pollCount.Add(1)

		w.Header().Set("Content-Type", "application/json")

		if count < 2 {
			_ = json.NewEncoder(w).Encode(map[string]string{
				"error": "authorization_pending",
			})

			return
		}

		_ = json.NewEncoder(w).Encode(map[string]any{
			"access_token":  "device-access",
			"token_type":    "Bearer",
			"refresh_token": "device-refresh",
			"expires_in":    3600,
		})
	})

	srv := httptest.NewServer(mux)
	defer srv.Close()

	oauthCfg := &oauth2.Config{
		ClientID:     "cid",
		ClientSecret: "csec",
		Endpoint:     oauth2.Endpoint{TokenURL: srv.URL + "/token"},
		Scopes:       []string{"mail"},
	}

	token, err := DeviceCodeAuth(context.Background(), oauthCfg, srv.URL+"/device")
	if err != nil {
		t.Fatalf("DeviceCodeAuth() error = %v", err)
	}

	if token.AccessToken != "device-access" {
		t.Errorf("AccessToken = %q, want device-access", token.AccessToken)
	}

	if token.RefreshToken != "device-refresh" {
		t.Errorf("RefreshToken = %q, want device-refresh", token.RefreshToken)
	}
}

func TestDeviceCodeAuth_Expired(t *testing.T) {
	t.Parallel()

	mux := http.NewServeMux()
	mux.HandleFunc("/device", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"device_code":      "dev-code",
			"user_code":        "CODE",
			"verification_uri": "https://example.com/verify",
			"interval":         1,
		})
	})
	mux.HandleFunc("/token", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{
			"error": "expired_token",
		})
	})

	srv := httptest.NewServer(mux)
	defer srv.Close()

	oauthCfg := &oauth2.Config{
		ClientID: "cid",
		Endpoint: oauth2.Endpoint{TokenURL: srv.URL + "/token"},
	}

	_, err := DeviceCodeAuth(context.Background(), oauthCfg, srv.URL+"/device")
	if !errors.Is(err, ErrDeviceCodeExpired) {
		t.Errorf("error = %v, want ErrDeviceCodeExpired", err)
	}
}

func TestDeviceCodeAuth_Denied(t *testing.T) {
	t.Parallel()

	mux := http.NewServeMux()
	mux.HandleFunc("/device", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"device_code":      "dev-code",
			"user_code":        "CODE",
			"verification_uri": "https://example.com/verify",
			"interval":         1,
		})
	})
	mux.HandleFunc("/token", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{
			"error": "access_denied",
		})
	})

	srv := httptest.NewServer(mux)
	defer srv.Close()

	oauthCfg := &oauth2.Config{
		ClientID: "cid",
		Endpoint: oauth2.Endpoint{TokenURL: srv.URL + "/token"},
	}

	_, err := DeviceCodeAuth(context.Background(), oauthCfg, srv.URL+"/device")
	if !errors.Is(err, ErrDeviceCodeDenied) {
		t.Errorf("error = %v, want ErrDeviceCodeDenied", err)
	}
}

func TestDeviceCodeAuth_SlowDown(t *testing.T) {
	t.Parallel()

	var pollCount atomic.Int32

	mux := http.NewServeMux()
	mux.HandleFunc("/device", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"device_code":      "dev-code",
			"user_code":        "CODE",
			"verification_uri": "https://example.com/verify",
			"interval":         1,
		})
	})
	mux.HandleFunc("/token", func(w http.ResponseWriter, _ *http.Request) {
		count := pollCount.Add(1)

		w.Header().Set("Content-Type", "application/json")

		if count == 1 {
			_ = json.NewEncoder(w).Encode(map[string]string{
				"error": "slow_down",
			})

			return
		}

		_ = json.NewEncoder(w).Encode(map[string]any{
			"access_token": "slowed-access",
			"token_type":   "Bearer",
			"expires_in":   3600,
		})
	})

	srv := httptest.NewServer(mux)
	defer srv.Close()

	oauthCfg := &oauth2.Config{
		ClientID: "cid",
		Endpoint: oauth2.Endpoint{TokenURL: srv.URL + "/token"},
	}

	token, err := DeviceCodeAuth(context.Background(), oauthCfg, srv.URL+"/device")
	if err != nil {
		t.Fatalf("DeviceCodeAuth() error = %v", err)
	}

	if token.AccessToken != "slowed-access" {
		t.Errorf("AccessToken = %q, want slowed-access", token.AccessToken)
	}
}

func TestDeviceCodeAuth_CtxCancel(t *testing.T) {
	t.Parallel()

	mux := http.NewServeMux()
	mux.HandleFunc("/device", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"device_code":      "dev-code",
			"user_code":        "CODE",
			"verification_uri": "https://example.com/verify",
			"interval":         60, // long interval
		})
	})
	mux.HandleFunc("/token", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{
			"error": "authorization_pending",
		})
	})

	srv := httptest.NewServer(mux)
	defer srv.Close()

	oauthCfg := &oauth2.Config{
		ClientID: "cid",
		Endpoint: oauth2.Endpoint{TokenURL: srv.URL + "/token"},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	_, err := DeviceCodeAuth(ctx, oauthCfg, srv.URL+"/device")
	if err == nil {
		t.Fatal("expected error from context cancellation")
	}

	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("error = %v, want context.DeadlineExceeded", err)
	}
}
