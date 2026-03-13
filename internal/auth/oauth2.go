package auth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"golang.org/x/oauth2"

	"github.com/boutquin/mcp-server-email/internal/config"
)

// Sentinel errors for device code flow and token operations.
var (
	ErrDeviceCodeExpired = errors.New("device code expired — please re-run authorization")
	ErrDeviceCodeDenied  = errors.New("authorization denied by user")
	ErrProviderUnknown   = errors.New("no OAuth2 provider for email domain")
	ErrDeviceCodeHTTP    = errors.New("device code request failed")
	ErrTokenEndpoint     = errors.New("token endpoint error")
)

const (
	defaultPollInterval = 5 * time.Second
	slowDownIncrement   = 5 * time.Second
	dirPermissions      = 0700
	filePermissions     = 0600
	formContentType     = "application/x-www-form-urlencoded"
	deviceCodeGrantType = "urn:ietf:params:oauth:grant-type:device_code"
)

// OAuthConfig builds an oauth2.Config from an account and its detected provider.
func OAuthConfig(account *config.Account) (*oauth2.Config, *Provider, error) {
	provider := DetectOAuthProvider(account.Email)
	if provider == nil {
		return nil, nil, fmt.Errorf(
			"%w: %q (supported: gmail.com, outlook.com)",
			ErrProviderUnknown, domainFromEmail(account.Email),
		)
	}

	return &oauth2.Config{
		ClientID:     account.OAuthClientID,
		ClientSecret: account.OAuthClientSecret,
		Endpoint: oauth2.Endpoint{
			AuthURL:  provider.AuthURL,
			TokenURL: provider.TokenURL,
		},
		Scopes: provider.Scopes,
	}, provider, nil
}

// NewTokenSource creates a token source that auto-refreshes and persists tokens.
func NewTokenSource(
	store *TokenStore,
	accountID string,
	oauthCfg *oauth2.Config,
) (oauth2.TokenSource, error) {
	token, err := store.Load(accountID)
	if err != nil {
		return nil, fmt.Errorf("load token for %s: %w", accountID, err)
	}

	ctx := context.Background()
	inner := oauth2.ReuseTokenSource(token, oauthCfg.TokenSource(ctx, token))

	return &persistingTokenSource{
		inner:     inner,
		store:     store,
		accountID: accountID,
		lastToken: token,
	}, nil
}

// persistingTokenSource wraps an oauth2.TokenSource and saves new tokens.
type persistingTokenSource struct {
	inner     oauth2.TokenSource
	store     *TokenStore
	accountID string
	lastToken *oauth2.Token
}

// Token returns a valid token, saving to disk if it was refreshed.
func (s *persistingTokenSource) Token() (*oauth2.Token, error) {
	token, err := s.inner.Token()
	if err != nil {
		return nil, err //nolint:wrapcheck // passthrough from oauth2
	}

	if s.lastToken == nil || token.AccessToken != s.lastToken.AccessToken {
		_ = s.store.Save(s.accountID, token) // best-effort persist
		s.lastToken = token
	}

	return token, nil
}

// deviceCodeResponse is the JSON response from the device authorization endpoint.
type deviceCodeResponse struct {
	DeviceCode      string `json:"device_code"`
	UserCode        string `json:"user_code"`
	VerificationURI string `json:"verification_uri"`
	Interval        int    `json:"interval"`
	ExpiresIn       int    `json:"expires_in"`
}

// tokenResponse is the JSON response from the token endpoint during device code polling.
type tokenResponse struct {
	AccessToken  string `json:"access_token"`
	TokenType    string `json:"token_type"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int    `json:"expires_in"`
	Error        string `json:"error"`
}

// DeviceCodeAuth runs the OAuth2 device authorization grant (RFC 8628).
func DeviceCodeAuth(
	ctx context.Context,
	oauthCfg *oauth2.Config,
	deviceAuthURL string,
) (*oauth2.Token, error) {
	dcResp, err := requestDeviceCode(ctx, oauthCfg, deviceAuthURL)
	if err != nil {
		return nil, err
	}

	// Print instructions to stderr (avoid polluting MCP stdout).
	fmt.Fprintf(os.Stderr, "\nVisit %s and enter code: %s\n\n",
		dcResp.VerificationURI, dcResp.UserCode)

	return pollForToken(ctx, oauthCfg, dcResp)
}

// requestDeviceCode posts to the device authorization endpoint.
func requestDeviceCode(
	ctx context.Context,
	oauthCfg *oauth2.Config,
	deviceAuthURL string,
) (*deviceCodeResponse, error) {
	data := url.Values{
		"client_id": {oauthCfg.ClientID},
		"scope":     {strings.Join(oauthCfg.Scopes, " ")},
	}

	body := strings.NewReader(data.Encode())

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, deviceAuthURL, body)
	if err != nil {
		return nil, fmt.Errorf("create device code request: %w", err)
	}

	req.Header.Set("Content-Type", formContentType)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("device code request: %w", err)
	}

	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read device code response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%w (HTTP %d): %s",
			ErrDeviceCodeHTTP, resp.StatusCode, respBody)
	}

	var dcResp deviceCodeResponse

	err = json.Unmarshal(respBody, &dcResp)
	if err != nil {
		return nil, fmt.Errorf("parse device code response: %w", err)
	}

	return &dcResp, nil
}

// pollForToken polls the token endpoint until success, expiry, or cancellation.
// Implements RFC 8628 §3.5: the server may respond with "authorization_pending"
// (user hasn't acted yet — keep polling) or "slow_down" (increase interval by 5s
// per RFC). Any other error (expired_token, access_denied) terminates the loop.
func pollForToken(
	ctx context.Context,
	oauthCfg *oauth2.Config,
	dcResp *deviceCodeResponse,
) (*oauth2.Token, error) {
	interval := time.Duration(dcResp.Interval) * time.Second
	if interval == 0 {
		interval = defaultPollInterval
	}

	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err() //nolint:wrapcheck // context cancellation
		case <-time.After(interval):
		}

		token, pollErr := pollTokenEndpoint(ctx, oauthCfg, dcResp.DeviceCode)
		if pollErr == nil {
			return token, nil
		}

		if errors.Is(pollErr, errAuthorizationPending) {
			continue
		}

		if errors.Is(pollErr, errSlowDown) {
			interval += slowDownIncrement

			continue
		}

		return nil, pollErr
	}
}

// Internal sentinel errors for poll loop control.
var (
	errAuthorizationPending = errors.New("authorization_pending")
	errSlowDown             = errors.New("slow_down")
)

// pollTokenEndpoint makes one poll request to the token endpoint.
func pollTokenEndpoint(
	ctx context.Context,
	oauthCfg *oauth2.Config,
	deviceCode string,
) (*oauth2.Token, error) {
	data := url.Values{
		"client_id":     {oauthCfg.ClientID},
		"client_secret": {oauthCfg.ClientSecret},
		"device_code":   {deviceCode},
		"grant_type":    {deviceCodeGrantType},
	}

	body := strings.NewReader(data.Encode())

	req, err := http.NewRequestWithContext(
		ctx, http.MethodPost, oauthCfg.Endpoint.TokenURL, body,
	)
	if err != nil {
		return nil, fmt.Errorf("create token request: %w", err)
	}

	req.Header.Set("Content-Type", formContentType)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("token request: %w", err)
	}

	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read token response: %w", err)
	}

	var tokenResp tokenResponse

	err = json.Unmarshal(respBody, &tokenResp)
	if err != nil {
		return nil, fmt.Errorf("parse token response: %w", err)
	}

	return handleTokenResponse(&tokenResp)
}

// handleTokenResponse interprets the token endpoint response.
func handleTokenResponse(tokenResp *tokenResponse) (*oauth2.Token, error) {
	switch tokenResp.Error {
	case "":
		return &oauth2.Token{
			AccessToken:  tokenResp.AccessToken,
			TokenType:    tokenResp.TokenType,
			RefreshToken: tokenResp.RefreshToken,
			Expiry:       time.Now().Add(time.Duration(tokenResp.ExpiresIn) * time.Second),
		}, nil
	case "authorization_pending":
		return nil, errAuthorizationPending
	case "slow_down":
		return nil, errSlowDown
	case "expired_token":
		return nil, ErrDeviceCodeExpired
	case "access_denied":
		return nil, ErrDeviceCodeDenied
	default:
		return nil, fmt.Errorf("%w: %s", ErrTokenEndpoint, tokenResp.Error)
	}
}
