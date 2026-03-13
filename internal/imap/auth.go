package imap

import (
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/boutquin/mcp-server-email/internal/auth"
)

// ErrTokenUnchanged indicates token refresh returned the same token.
var ErrTokenUnchanged = errors.New("OAuth2 token unchanged after refresh")

// authenticateConn performs either password Login or OAuth2 SASL Authenticate.
func (c *Client) authenticateConn(conn Connector) error {
	if c.account.AuthMethod == "oauth2" {
		return c.authenticateOAuth2(conn)
	}

	return c.authenticatePassword(conn)
}

// authenticatePassword performs IMAP LOGIN command.
func (c *Client) authenticatePassword(conn Connector) error {
	err := conn.Login(c.account.Username, c.account.Password)
	if err != nil {
		return fmt.Errorf("IMAP login: %w", err)
	}

	return nil
}

// authenticateOAuth2 performs XOAUTH2 SASL authentication with one retry on failure.
func (c *Client) authenticateOAuth2(conn Connector) error {
	token, err := c.tokenSource.Token()
	if err != nil {
		return fmt.Errorf("get OAuth2 token: %w", err)
	}

	saslClient := auth.NewXOAuth2Client(c.account.Email, token.AccessToken)

	err = conn.Authenticate(saslClient)
	if err == nil {
		return nil
	}

	if !isAuthenticationFailed(err) {
		return fmt.Errorf("IMAP OAuth2 authenticate: %w", err)
	}

	// One retry: the token may have been cached but expired server-side.
	c.log().Warn("OAuth2 auth failed, refreshing token and retrying",
		slog.String("error", err.Error()))

	newToken, refreshErr := c.tokenSource.Token()
	if refreshErr != nil {
		return fmt.Errorf("token refresh after auth failure: %w", refreshErr)
	}

	// If refresh returned the same token, don't retry (would fail again).
	if newToken.AccessToken == token.AccessToken {
		return fmt.Errorf("%w: %w", ErrTokenUnchanged, err)
	}

	saslClient2 := auth.NewXOAuth2Client(c.account.Email, newToken.AccessToken)

	err = conn.Authenticate(saslClient2)
	if err != nil {
		return fmt.Errorf("IMAP OAuth2 retry authenticate: %w", err)
	}

	return nil
}

// isAuthenticationFailed checks if an IMAP error indicates an authentication failure
// (as opposed to a network error or protocol error).
func isAuthenticationFailed(err error) bool {
	if err == nil {
		return false
	}

	errStr := strings.ToUpper(err.Error())

	return strings.Contains(errStr, "AUTHENTICATIONFAILED") ||
		strings.Contains(errStr, "AUTHENTICATION FAILED") ||
		strings.Contains(errStr, "INVALID CREDENTIALS")
}
