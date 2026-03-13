package auth //nolint:testpackage // tests access internal helpers

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"golang.org/x/oauth2"
)

// --- requestDeviceCode error paths ---

func TestRequestDeviceCode_HTTPNon200(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("server error"))
	}))
	defer srv.Close()

	oauthCfg := &oauth2.Config{ClientID: "cid", Scopes: []string{"mail"}}

	_, err := requestDeviceCode(context.Background(), oauthCfg, srv.URL)
	if err == nil {
		t.Fatal("expected error for HTTP 500")
	}

	if !errors.Is(err, ErrDeviceCodeHTTP) {
		t.Errorf("error = %v, want ErrDeviceCodeHTTP", err)
	}

	if !strings.Contains(err.Error(), "500") {
		t.Errorf("error should contain status code, got: %v", err)
	}
}

func TestRequestDeviceCode_InvalidJSON(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte("not json{{{"))
	}))
	defer srv.Close()

	oauthCfg := &oauth2.Config{ClientID: "cid", Scopes: []string{"mail"}}

	_, err := requestDeviceCode(context.Background(), oauthCfg, srv.URL)
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}

	if !strings.Contains(err.Error(), "parse device code response") {
		t.Errorf("error = %q, want 'parse device code response'", err.Error())
	}
}

func TestRequestDeviceCode_InvalidURL(t *testing.T) {
	t.Parallel()

	oauthCfg := &oauth2.Config{ClientID: "cid"}

	// A URL with a control character triggers NewRequestWithContext error.
	_, err := requestDeviceCode(context.Background(), oauthCfg, "http://invalid\x00url")
	if err == nil {
		t.Fatal("expected error for invalid URL")
	}

	if !strings.Contains(err.Error(), "create device code request") {
		t.Errorf("error = %q, want 'create device code request'", err.Error())
	}
}

func TestRequestDeviceCode_NetworkError(t *testing.T) {
	t.Parallel()

	oauthCfg := &oauth2.Config{ClientID: "cid"}

	// Connect to a port that's not listening.
	_, err := requestDeviceCode(context.Background(), oauthCfg, "http://127.0.0.1:1")
	if err == nil {
		t.Fatal("expected error for network failure")
	}

	if !strings.Contains(err.Error(), "device code request") {
		t.Errorf("error = %q, want 'device code request'", err.Error())
	}
}

func TestRequestDeviceCode_ContextCancelled(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		// Block forever
		select {}
	}))
	defer srv.Close()

	oauthCfg := &oauth2.Config{ClientID: "cid"}

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	_, err := requestDeviceCode(ctx, oauthCfg, srv.URL)
	if err == nil {
		t.Fatal("expected error for cancelled context")
	}
}

// --- pollTokenEndpoint error paths ---

func TestPollTokenEndpoint_InvalidURL(t *testing.T) {
	t.Parallel()

	oauthCfg := &oauth2.Config{
		ClientID: "cid",
		Endpoint: oauth2.Endpoint{TokenURL: "http://invalid\x00url"}, //nolint:gosec // G101: test URL, not credentials
	}

	_, err := pollTokenEndpoint(context.Background(), oauthCfg, "dev-code")
	if err == nil {
		t.Fatal("expected error for invalid URL")
	}

	if !strings.Contains(err.Error(), "create token request") {
		t.Errorf("error = %q, want 'create token request'", err.Error())
	}
}

func TestPollTokenEndpoint_NetworkError(t *testing.T) {
	t.Parallel()

	oauthCfg := &oauth2.Config{
		ClientID: "cid",
		Endpoint: oauth2.Endpoint{TokenURL: "http://127.0.0.1:1"}, //nolint:gosec // G101: test URL, not credentials
	}

	_, err := pollTokenEndpoint(context.Background(), oauthCfg, "dev-code")
	if err == nil {
		t.Fatal("expected error for network failure")
	}

	if !strings.Contains(err.Error(), "token request") {
		t.Errorf("error = %q, want 'token request'", err.Error())
	}
}

func TestPollTokenEndpoint_InvalidJSON(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte("{{not json"))
	}))
	defer srv.Close()

	oauthCfg := &oauth2.Config{
		ClientID: "cid",
		Endpoint: oauth2.Endpoint{TokenURL: srv.URL},
	}

	_, err := pollTokenEndpoint(context.Background(), oauthCfg, "dev-code")
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}

	if !strings.Contains(err.Error(), "parse token response") {
		t.Errorf("error = %q, want 'parse token response'", err.Error())
	}
}

func TestPollTokenEndpoint_UnknownError(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"error":"server_error"}`))
	}))
	defer srv.Close()

	oauthCfg := &oauth2.Config{
		ClientID: "cid",
		Endpoint: oauth2.Endpoint{TokenURL: srv.URL},
	}

	_, err := pollTokenEndpoint(context.Background(), oauthCfg, "dev-code")
	if err == nil {
		t.Fatal("expected error for unknown error response")
	}

	if !errors.Is(err, ErrTokenEndpoint) {
		t.Errorf("error = %v, want ErrTokenEndpoint", err)
	}

	if !strings.Contains(err.Error(), "server_error") {
		t.Errorf("error should contain 'server_error', got: %v", err)
	}
}

// --- DeviceCodeAuth error propagation ---

func TestDeviceCodeAuth_DeviceEndpointError(t *testing.T) {
	t.Parallel()

	oauthCfg := &oauth2.Config{ClientID: "cid"}

	// Use unreachable address to trigger device endpoint failure.
	_, err := DeviceCodeAuth(context.Background(), oauthCfg, "http://127.0.0.1:1")
	if err == nil {
		t.Fatal("expected error when device endpoint is unreachable")
	}

	if !strings.Contains(err.Error(), "device code request") {
		t.Errorf("error = %q, want 'device code request'", err.Error())
	}
}

func TestHandleTokenResponse_UnknownError(t *testing.T) {
	t.Parallel()

	resp := &tokenResponse{Error: "invalid_grant"}

	_, err := handleTokenResponse(resp)
	if err == nil {
		t.Fatal("expected error for unknown error type")
	}

	if !errors.Is(err, ErrTokenEndpoint) {
		t.Errorf("error = %v, want ErrTokenEndpoint", err)
	}

	if !strings.Contains(err.Error(), "invalid_grant") {
		t.Errorf("error should contain error type, got: %v", err)
	}
}
