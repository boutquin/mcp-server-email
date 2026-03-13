// Package config handles multi-account email configuration.
package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strconv"

	"github.com/boutquin/mcp-server-email/internal/models"
)

// Sentinel errors for configuration loading and validation.
var (
	ErrNoConfig           = errors.New("EMAIL_ACCOUNTS or EMAIL_CONFIG_FILE must be set")
	ErrNoAccounts         = errors.New("no accounts configured")
	ErrOAuthMissingClient = errors.New("oauth2 requires oauth_client_id")
	ErrUnknownAuthMethod  = errors.New("unknown auth_method")
)

const (
	DefaultIMAPRateLimitRPM = 60
	DefaultSMTPRateLimitRPH = 100
	DefaultIMAPTimeoutMS    = 30000
	DefaultSMTPTimeoutMS    = 30000

	DefaultMaxAttachmentSizeMB      = 18
	DefaultMaxTotalAttachmentSizeMB = 18
	DefaultMaxDownloadSizeMB        = 25
	DefaultPoolCloseTimeoutMS       = 5000

	imapTLSPort = 993
	smtpTLSPort = 465
)

// Account represents a single email account configuration.
type Account struct {
	ID                 string `json:"id"`
	Email              string `json:"email"`
	IMAPHost           string `json:"imap_host"`
	IMAPPort           int    `json:"imap_port"`
	SMTPHost           string `json:"smtp_host"`
	SMTPPort           int    `json:"smtp_port"`
	Username           string `json:"username"`
	Password           string `json:"password"`
	UseStartTLS        *bool  `json:"use_starttls,omitempty"`         // nil = auto-detect based on port
	InsecureSkipVerify bool   `json:"insecure_skip_verify,omitempty"` // skip TLS cert verify
	AuthMethod         string `json:"auth_method,omitempty"`          // "password" (default) or "oauth2"
	OAuthClientID      string `json:"oauth_client_id,omitempty"`      // OAuth2 client ID
	OAuthClientSecret  string `json:"oauth_client_secret,omitempty"`  // OAuth2 client secret
	OAuthTokenFile     string `json:"oauth_token_file,omitempty"`     // override token file path
}

// IMAPUseTLS returns true if IMAP should use implicit TLS (port 993).
// Returns false if STARTTLS should be used (port 143 or explicit config).
func (a *Account) IMAPUseTLS() bool {
	if a.UseStartTLS != nil {
		return !*a.UseStartTLS // explicit config
	}
	// Auto-detect: port 993 = implicit TLS, others = STARTTLS
	return a.IMAPPort == imapTLSPort
}

// SMTPUseTLS returns true if SMTP should use implicit TLS (port 465).
// Returns false if STARTTLS should be used (port 587 or explicit config).
func (a *Account) SMTPUseTLS() bool {
	if a.UseStartTLS != nil {
		return !*a.UseStartTLS // explicit config
	}
	// Auto-detect: port 465 = implicit TLS, others = STARTTLS
	return a.SMTPPort == smtpTLSPort
}

// Config holds the full email server configuration.
type Config struct {
	Accounts                 []Account
	DefaultAccount           string
	IMAPRateLimitRPM         int
	SMTPRateLimitRPH         int
	IMAPTimeoutMS            int
	SMTPTimeoutMS            int
	MaxAttachmentSizeMB      int
	MaxTotalAttachmentSizeMB int
	MaxDownloadSizeMB        int
	PoolCloseTimeoutMS       int
	Debug                    bool
}

// LoadFromEnv loads configuration from environment variables.
func LoadFromEnv() (*Config, error) {
	cfg := &Config{
		IMAPRateLimitRPM:         DefaultIMAPRateLimitRPM,
		SMTPRateLimitRPH:         DefaultSMTPRateLimitRPH,
		IMAPTimeoutMS:            DefaultIMAPTimeoutMS,
		SMTPTimeoutMS:            DefaultSMTPTimeoutMS,
		MaxAttachmentSizeMB:      DefaultMaxAttachmentSizeMB,
		MaxTotalAttachmentSizeMB: DefaultMaxTotalAttachmentSizeMB,
		MaxDownloadSizeMB:        DefaultMaxDownloadSizeMB,
		PoolCloseTimeoutMS:       DefaultPoolCloseTimeoutMS,
		Debug:                    os.Getenv("EMAIL_DEBUG") == "true",
	}

	// Load accounts from env var or file
	accountsJSON := os.Getenv("EMAIL_ACCOUNTS")
	configFile := os.Getenv("EMAIL_CONFIG_FILE")

	switch {
	case accountsJSON != "":
		err := json.Unmarshal([]byte(accountsJSON), &cfg.Accounts)
		if err != nil {
			return nil, fmt.Errorf("parse EMAIL_ACCOUNTS: %w", err)
		}
	case configFile != "":
		data, err := os.ReadFile(configFile)
		if err != nil {
			return nil, fmt.Errorf("read EMAIL_CONFIG_FILE: %w", err)
		}

		err = json.Unmarshal(data, &cfg.Accounts)
		if err != nil {
			return nil, fmt.Errorf("parse EMAIL_CONFIG_FILE: %w", err)
		}
	default:
		return nil, ErrNoConfig
	}

	if len(cfg.Accounts) == 0 {
		return nil, ErrNoAccounts
	}

	// Auto-detect provider settings for accounts missing host configuration.
	for i := range cfg.Accounts {
		applyProviderDefaults(&cfg.Accounts[i])
	}

	// Validate auth method configuration.
	for _, acct := range cfg.Accounts {
		err := validateAuthMethod(&acct)
		if err != nil {
			return nil, err
		}
	}

	// Default account
	cfg.DefaultAccount = os.Getenv("EMAIL_DEFAULT_ACCOUNT")
	if cfg.DefaultAccount == "" {
		cfg.DefaultAccount = cfg.Accounts[0].ID
	}

	applyEnvOverrides(cfg)

	return cfg, nil
}

// GetAccount returns an account by ID, or the default account if id is empty.
func (c *Config) GetAccount(id string) (*Account, error) {
	if id == "" {
		id = c.DefaultAccount
	}

	for i := range c.Accounts {
		if c.Accounts[i].ID == id {
			return &c.Accounts[i], nil
		}
	}

	return nil, fmt.Errorf("%w: %s", models.ErrAccountNotFound, id)
}

// applyProviderDefaults fills in missing IMAP/SMTP host and port from
// well-known provider presets. Explicit configuration always wins.
func applyProviderDefaults(a *Account) {
	if a.IMAPHost != "" && a.SMTPHost != "" {
		return
	}

	p := DetectProvider(a.Email)
	if p == nil {
		return
	}

	if a.IMAPHost == "" {
		a.IMAPHost = p.IMAPHost
		a.IMAPPort = p.IMAPPort
	}

	if a.SMTPHost == "" {
		a.SMTPHost = p.SMTPHost
		a.SMTPPort = p.SMTPPort
	}
}

func applyEnvOverrides(cfg *Config) {
	if v, _ := strconv.Atoi(os.Getenv("EMAIL_IMAP_RATE_LIMIT")); v > 0 {
		cfg.IMAPRateLimitRPM = v
	}

	if v, _ := strconv.Atoi(os.Getenv("EMAIL_SMTP_RATE_LIMIT")); v > 0 {
		cfg.SMTPRateLimitRPH = v
	}

	if v, _ := strconv.Atoi(os.Getenv("EMAIL_IMAP_TIMEOUT_MS")); v > 0 {
		cfg.IMAPTimeoutMS = v
	}

	if v, _ := strconv.Atoi(os.Getenv("EMAIL_SMTP_TIMEOUT_MS")); v > 0 {
		cfg.SMTPTimeoutMS = v
	}

	if v, _ := strconv.Atoi(os.Getenv("MAX_ATTACHMENT_SIZE_MB")); v > 0 {
		cfg.MaxAttachmentSizeMB = v
	}

	if v, _ := strconv.Atoi(os.Getenv("MAX_TOTAL_ATTACHMENT_SIZE_MB")); v > 0 {
		cfg.MaxTotalAttachmentSizeMB = v
	}

	if v, _ := strconv.Atoi(os.Getenv("MAX_DOWNLOAD_SIZE_MB")); v > 0 {
		cfg.MaxDownloadSizeMB = v
	}

	if v, _ := strconv.Atoi(os.Getenv("EMAIL_POOL_CLOSE_TIMEOUT_MS")); v > 0 {
		cfg.PoolCloseTimeoutMS = v
	}
}

// validateAuthMethod checks that the account's auth method is valid
// and that required OAuth2 fields are present.
func validateAuthMethod(acct *Account) error {
	switch acct.AuthMethod {
	case "", "password":
		return nil
	case "oauth2":
		if acct.OAuthClientID == "" {
			return fmt.Errorf("account %s: %w", acct.ID, ErrOAuthMissingClient)
		}

		return nil
	default:
		return fmt.Errorf(
			"account %s: %w: %q (valid: password, oauth2)",
			acct.ID, ErrUnknownAuthMethod, acct.AuthMethod,
		)
	}
}
