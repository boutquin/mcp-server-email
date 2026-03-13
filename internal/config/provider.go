package config

import "strings"

type providerPreset struct {
	IMAPHost string
	IMAPPort int
	SMTPHost string
	SMTPPort int
}

var providers = map[string]providerPreset{ //nolint:gochecknoglobals // static lookup table
	"gmail.com":      {"imap.gmail.com", 993, "smtp.gmail.com", 587},
	"googlemail.com": {"imap.gmail.com", 993, "smtp.gmail.com", 587},
	"outlook.com":    {"outlook.office365.com", 993, "smtp.office365.com", 587},
	"hotmail.com":    {"outlook.office365.com", 993, "smtp.office365.com", 587},
	"live.com":       {"outlook.office365.com", 993, "smtp.office365.com", 587},
	"yahoo.com":      {"imap.mail.yahoo.com", 993, "smtp.mail.yahoo.com", 587},
	"icloud.com":     {"imap.mail.me.com", 993, "smtp.mail.me.com", 587},
	"me.com":         {"imap.mail.me.com", 993, "smtp.mail.me.com", 587},
	"mac.com":        {"imap.mail.me.com", 993, "smtp.mail.me.com", 587},
	"fastmail.com":   {"imap.fastmail.com", 993, "smtp.fastmail.com", 587},
	"fastmail.fm":    {"imap.fastmail.com", 993, "smtp.fastmail.com", 587},
	"zoho.com":       {"imap.zoho.com", 993, "smtp.zoho.com", 587},
	"zohomail.com":   {"imap.zoho.com", 993, "smtp.zoho.com", 587},
}

// ProviderResult holds the detected IMAP/SMTP settings for a provider.
type ProviderResult struct {
	IMAPHost string
	IMAPPort int
	SMTPHost string
	SMTPPort int
}

// DetectProvider returns IMAP/SMTP settings for well-known email providers.
// Returns nil if the domain is not recognized.
func DetectProvider(email string) *ProviderResult {
	at := strings.LastIndex(email, "@")
	if at < 0 || at == len(email)-1 {
		return nil
	}

	domain := strings.ToLower(email[at+1:])

	p, ok := providers[domain]
	if !ok {
		return nil
	}

	return &ProviderResult{
		IMAPHost: p.IMAPHost,
		IMAPPort: p.IMAPPort,
		SMTPHost: p.SMTPHost,
		SMTPPort: p.SMTPPort,
	}
}
