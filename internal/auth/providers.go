// Package auth provides OAuth2 authentication for email providers.
package auth

import "strings"

// Provider holds OAuth2 configuration for an email provider.
type Provider struct {
	Name          string
	AuthURL       string
	TokenURL      string
	DeviceAuthURL string
	Scopes        []string
}

// Providers is the registry of known OAuth2 email providers.
//
//nolint:gochecknoglobals,gosec // static lookup table; G101: URLs contain "token" but are not credentials
var Providers = map[string]Provider{
	"gmail": {
		Name:          "Gmail",
		AuthURL:       "https://accounts.google.com/o/oauth2/auth",
		TokenURL:      "https://oauth2.googleapis.com/token",
		DeviceAuthURL: "https://oauth2.googleapis.com/device/code",
		Scopes:        []string{"https://mail.google.com/"},
	},
	"outlook": {
		Name:          "Outlook",
		AuthURL:       "https://login.microsoftonline.com/common/oauth2/v2.0/authorize",
		TokenURL:      "https://login.microsoftonline.com/common/oauth2/v2.0/token",
		DeviceAuthURL: "https://login.microsoftonline.com/common/oauth2/v2.0/devicecode",
		Scopes: []string{
			"https://outlook.office365.com/IMAP.AccessAsUser.All",
			"https://outlook.office365.com/SMTP.Send",
			"offline_access",
		},
	},
}

// domainToProvider maps email domains to provider registry keys.
//
//nolint:gochecknoglobals // static lookup table
var domainToProvider = map[string]string{
	"gmail.com":      "gmail",
	"googlemail.com": "gmail",
	"outlook.com":    "outlook",
	"hotmail.com":    "outlook",
	"live.com":       "outlook",
}

// DetectOAuthProvider returns the OAuth2 provider for the given email address.
// Returns nil if the domain is not recognized.
func DetectOAuthProvider(email string) *Provider {
	domain := domainFromEmail(email)
	if domain == "" {
		return nil
	}

	key, ok := domainToProvider[domain]
	if !ok {
		return nil
	}

	p := Providers[key]

	return &p
}

// domainFromEmail extracts and lowercases the domain from an email address.
func domainFromEmail(email string) string {
	at := strings.LastIndex(email, "@")
	if at < 0 || at == len(email)-1 {
		return ""
	}

	return strings.ToLower(email[at+1:])
}
