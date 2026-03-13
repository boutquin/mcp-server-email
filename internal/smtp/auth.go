package smtp

import (
	"errors"
	"strings"

	"github.com/boutquin/mcp-server-email/internal/auth"
	gosmtp "github.com/wneessen/go-mail/smtp"
)

// ErrXOAuth2Challenge indicates an unexpected server challenge during XOAUTH2.
var ErrXOAuth2Challenge = errors.New("unexpected XOAUTH2 server challenge")

// xoauth2Auth implements go-mail's smtp.Auth for the XOAUTH2 mechanism.
type xoauth2Auth struct {
	email string
	token string
}

// Compile-time interface check.
var _ gosmtp.Auth = (*xoauth2Auth)(nil)

// Start returns the XOAUTH2 mechanism name and initial response.
func (a *xoauth2Auth) Start(_ *gosmtp.ServerInfo) (string, []byte, error) {
	return "XOAUTH2", auth.BuildXOAuth2String(a.email, a.token), nil
}

// Next handles server challenges. In XOAUTH2, any further challenge means failure.
func (a *xoauth2Auth) Next(_ []byte, more bool) ([]byte, error) {
	if more {
		return nil, ErrXOAuth2Challenge
	}

	return nil, nil
}

// isSMTPAuthFailure checks if an SMTP error is a 535 authentication failure.
func isSMTPAuthFailure(err error) bool {
	if err == nil {
		return false
	}

	return strings.HasPrefix(err.Error(), "535 ")
}
