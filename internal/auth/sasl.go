package auth

import (
	"errors"
	"fmt"

	"github.com/emersion/go-sasl"
)

// ErrXOAuth2Failed indicates the server rejected XOAUTH2 authentication.
var ErrXOAuth2Failed = errors.New("XOAUTH2 authentication failed")

// Compile-time interface check.
var _ sasl.Client = (*XOAuth2Client)(nil)

// xoauth2Mechanism is the SASL mechanism name for XOAUTH2.
const xoauth2Mechanism = "XOAUTH2"

// XOAuth2Client implements the go-sasl Client interface for XOAUTH2 authentication.
// The XOAUTH2 mechanism sends the user's email and OAuth2 bearer token
// in a single initial response.
type XOAuth2Client struct {
	email string
	token string
}

// NewXOAuth2Client creates a new XOAUTH2 SASL client.
func NewXOAuth2Client(email, accessToken string) *XOAuth2Client {
	return &XOAuth2Client{
		email: email,
		token: accessToken,
	}
}

// Start returns the XOAUTH2 mechanism name and initial response.
func (c *XOAuth2Client) Start() (string, []byte, error) {
	return xoauth2Mechanism, BuildXOAuth2String(c.email, c.token), nil
}

// Next handles server challenges. In XOAUTH2, any server challenge
// indicates authentication failure. Returns an empty response (required
// by protocol) and the challenge as an error.
func (c *XOAuth2Client) Next(challenge []byte) ([]byte, error) {
	return nil, fmt.Errorf("%w: %s", ErrXOAuth2Failed, challenge)
}

// BuildXOAuth2String builds the XOAUTH2 initial response per RFC 7628.
func BuildXOAuth2String(email, token string) []byte {
	return []byte("user=" + email + "\x01" + "auth=Bearer " + token + "\x01\x01")
}
