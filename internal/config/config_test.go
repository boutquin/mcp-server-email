package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestAccountIMAPUseTLS(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		port        int
		useStartTLS *bool
		want        bool
	}{
		{"port 993 implicit TLS", 993, nil, true},
		{"port 143 STARTTLS", 143, nil, false},
		{"port 993 explicit STARTTLS", 993, boolPtr(true), false},
		{"port 143 explicit TLS", 143, boolPtr(false), true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			a := &Account{IMAPPort: tt.port, UseStartTLS: tt.useStartTLS}

			if got := a.IMAPUseTLS(); got != tt.want {
				t.Errorf("IMAPUseTLS() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestAccountSMTPUseTLS(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		port        int
		useStartTLS *bool
		want        bool
	}{
		{"port 465 implicit TLS", 465, nil, true},
		{"port 587 STARTTLS", 587, nil, false},
		{"port 465 explicit STARTTLS", 465, boolPtr(true), false},
		{"port 587 explicit TLS", 587, boolPtr(false), true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			a := &Account{SMTPPort: tt.port, UseStartTLS: tt.useStartTLS}

			if got := a.SMTPUseTLS(); got != tt.want {
				t.Errorf("SMTPUseTLS() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestLoadFromEnv_Accounts(t *testing.T) {
	// Test with EMAIL_ACCOUNTS env var
	t.Run("valid JSON accounts", func(t *testing.T) {
		t.Setenv("EMAIL_ACCOUNTS", `[{"id":"test","email":"test@example.com",`+
			`"imap_host":"imap.example.com","imap_port":993,`+
			`"smtp_host":"smtp.example.com","smtp_port":465,`+
			`"username":"user","password":"pass"}]`)

		cfg, err := LoadFromEnv()
		if err != nil {
			t.Fatalf("LoadFromEnv() error = %v", err)
		}

		if len(cfg.Accounts) != 1 {
			t.Errorf("expected 1 account, got %d", len(cfg.Accounts))
		}

		if cfg.Accounts[0].ID != "test" {
			t.Errorf("expected account ID 'test', got %q", cfg.Accounts[0].ID)
		}

		if cfg.DefaultAccount != "test" {
			t.Errorf("expected default account 'test', got %q", cfg.DefaultAccount)
		}
	})

	t.Run("invalid JSON", func(t *testing.T) {
		t.Setenv("EMAIL_ACCOUNTS", `not valid json`)

		_, err := LoadFromEnv()
		if err == nil {
			t.Error("expected error for invalid JSON")
		}
	})

	t.Run("empty accounts", func(t *testing.T) {
		t.Setenv("EMAIL_ACCOUNTS", `[]`)

		_, err := LoadFromEnv()
		if err == nil {
			t.Error("expected error for empty accounts")
		}
	})

	t.Run("no config", func(t *testing.T) {
		os.Unsetenv("EMAIL_ACCOUNTS")
		os.Unsetenv("EMAIL_CONFIG_FILE")

		_, err := LoadFromEnv()
		if err == nil {
			t.Error("expected error when no config provided")
		}
	})
}

func TestLoadFromEnv_ConfigFile(t *testing.T) {
	// Create temp config file
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.json")
	configData := `[{"id":"file-test","email":"test@example.com",` +
		`"imap_host":"imap.example.com","imap_port":993,` +
		`"smtp_host":"smtp.example.com","smtp_port":465,` +
		`"username":"user","password":"pass"}]`

	err := os.WriteFile(configPath, []byte(configData), 0644)
	if err != nil {
		t.Fatalf("failed to write temp config: %v", err)
	}

	os.Unsetenv("EMAIL_ACCOUNTS")
	t.Setenv("EMAIL_CONFIG_FILE", configPath)

	cfg, err := LoadFromEnv()
	if err != nil {
		t.Fatalf("LoadFromEnv() error = %v", err)
	}

	if cfg.Accounts[0].ID != "file-test" {
		t.Errorf("expected account ID 'file-test', got %q", cfg.Accounts[0].ID)
	}
}

func TestLoadFromEnv_RateLimits(t *testing.T) {
	t.Setenv("EMAIL_ACCOUNTS", `[{"id":"test","email":"test@example.com",`+
		`"imap_host":"imap.example.com","imap_port":993,`+
		`"smtp_host":"smtp.example.com","smtp_port":465,`+
		`"username":"user","password":"pass"}]`)

	t.Run("defaults", func(t *testing.T) {
		os.Unsetenv("EMAIL_IMAP_RATE_LIMIT")
		os.Unsetenv("EMAIL_SMTP_RATE_LIMIT")

		cfg, err := LoadFromEnv()
		if err != nil {
			t.Fatalf("LoadFromEnv() error = %v", err)
		}

		if cfg.IMAPRateLimitRPM != DefaultIMAPRateLimitRPM {
			t.Errorf("expected IMAP rate limit %d, got %d", DefaultIMAPRateLimitRPM, cfg.IMAPRateLimitRPM)
		}

		if cfg.SMTPRateLimitRPH != DefaultSMTPRateLimitRPH {
			t.Errorf("expected SMTP rate limit %d, got %d", DefaultSMTPRateLimitRPH, cfg.SMTPRateLimitRPH)
		}
	})

	t.Run("custom values", func(t *testing.T) {
		t.Setenv("EMAIL_IMAP_RATE_LIMIT", "120")
		t.Setenv("EMAIL_SMTP_RATE_LIMIT", "200")

		cfg, err := LoadFromEnv()
		if err != nil {
			t.Fatalf("LoadFromEnv() error = %v", err)
		}

		if cfg.IMAPRateLimitRPM != 120 {
			t.Errorf("expected IMAP rate limit 120, got %d", cfg.IMAPRateLimitRPM)
		}

		if cfg.SMTPRateLimitRPH != 200 {
			t.Errorf("expected SMTP rate limit 200, got %d", cfg.SMTPRateLimitRPH)
		}
	})
}

func TestLoadFromEnv_Timeouts(t *testing.T) {
	t.Setenv("EMAIL_ACCOUNTS", `[{"id":"test","email":"test@example.com",`+
		`"imap_host":"imap.example.com","imap_port":993,`+
		`"smtp_host":"smtp.example.com","smtp_port":465,`+
		`"username":"user","password":"pass"}]`)

	t.Run("defaults", func(t *testing.T) {
		os.Unsetenv("EMAIL_IMAP_TIMEOUT_MS")
		os.Unsetenv("EMAIL_SMTP_TIMEOUT_MS")

		cfg, err := LoadFromEnv()
		if err != nil {
			t.Fatalf("LoadFromEnv() error = %v", err)
		}

		if cfg.IMAPTimeoutMS != DefaultIMAPTimeoutMS {
			t.Errorf("expected IMAP timeout %d, got %d", DefaultIMAPTimeoutMS, cfg.IMAPTimeoutMS)
		}

		if cfg.SMTPTimeoutMS != DefaultSMTPTimeoutMS {
			t.Errorf("expected SMTP timeout %d, got %d", DefaultSMTPTimeoutMS, cfg.SMTPTimeoutMS)
		}
	})

	t.Run("custom values", func(t *testing.T) {
		t.Setenv("EMAIL_IMAP_TIMEOUT_MS", "60000")
		t.Setenv("EMAIL_SMTP_TIMEOUT_MS", "45000")

		cfg, err := LoadFromEnv()
		if err != nil {
			t.Fatalf("LoadFromEnv() error = %v", err)
		}

		if cfg.IMAPTimeoutMS != 60000 {
			t.Errorf("expected IMAP timeout 60000, got %d", cfg.IMAPTimeoutMS)
		}

		if cfg.SMTPTimeoutMS != 45000 {
			t.Errorf("expected SMTP timeout 45000, got %d", cfg.SMTPTimeoutMS)
		}
	})
}

func TestLoadFromEnv_Debug(t *testing.T) {
	t.Setenv("EMAIL_ACCOUNTS", `[{"id":"test","email":"test@example.com",`+
		`"imap_host":"imap.example.com","imap_port":993,`+
		`"smtp_host":"smtp.example.com","smtp_port":465,`+
		`"username":"user","password":"pass"}]`)

	t.Run("debug off by default", func(t *testing.T) {
		os.Unsetenv("EMAIL_DEBUG")

		cfg, err := LoadFromEnv()
		if err != nil {
			t.Fatalf("LoadFromEnv() error = %v", err)
		}

		if cfg.Debug {
			t.Error("expected Debug to be false")
		}
	})

	t.Run("debug enabled", func(t *testing.T) {
		t.Setenv("EMAIL_DEBUG", "true")

		cfg, err := LoadFromEnv()
		if err != nil {
			t.Fatalf("LoadFromEnv() error = %v", err)
		}

		if !cfg.Debug {
			t.Error("expected Debug to be true")
		}
	})
}

func TestConfig_GetAccount(t *testing.T) {
	t.Parallel()

	cfg := &Config{
		Accounts: []Account{
			{ID: "acc1", Email: "one@example.com"},
			{ID: "acc2", Email: "two@example.com"},
		},
		DefaultAccount: "acc1",
	}

	t.Run("get by ID", func(t *testing.T) {
		t.Parallel()

		acc, err := cfg.GetAccount("acc2")
		if err != nil {
			t.Fatalf("GetAccount() error = %v", err)
		}

		if acc.ID != "acc2" {
			t.Errorf("expected account ID 'acc2', got %q", acc.ID)
		}
	})

	t.Run("get default", func(t *testing.T) {
		t.Parallel()

		acc, err := cfg.GetAccount("")
		if err != nil {
			t.Fatalf("GetAccount() error = %v", err)
		}

		if acc.ID != "acc1" {
			t.Errorf("expected default account 'acc1', got %q", acc.ID)
		}
	})

	t.Run("not found", func(t *testing.T) {
		t.Parallel()

		_, err := cfg.GetAccount("nonexistent")
		if err == nil {
			t.Error("expected error for nonexistent account")
		}
	})
}

func TestLoadFromEnv_AttachmentLimits(t *testing.T) {
	t.Setenv("EMAIL_ACCOUNTS", `[{"id":"test","email":"test@example.com",`+
		`"imap_host":"imap.example.com","imap_port":993,`+
		`"smtp_host":"smtp.example.com","smtp_port":465,`+
		`"username":"user","password":"pass"}]`)

	t.Run("defaults", func(t *testing.T) {
		os.Unsetenv("MAX_ATTACHMENT_SIZE_MB")
		os.Unsetenv("MAX_TOTAL_ATTACHMENT_SIZE_MB")
		os.Unsetenv("MAX_DOWNLOAD_SIZE_MB")

		cfg, err := LoadFromEnv()
		if err != nil {
			t.Fatalf("LoadFromEnv() error = %v", err)
		}

		if cfg.MaxAttachmentSizeMB != DefaultMaxAttachmentSizeMB {
			t.Errorf("MaxAttachmentSizeMB = %d, want %d", cfg.MaxAttachmentSizeMB, DefaultMaxAttachmentSizeMB)
		}

		if cfg.MaxTotalAttachmentSizeMB != DefaultMaxTotalAttachmentSizeMB {
			t.Errorf("MaxTotalAttachmentSizeMB = %d, want %d",
				cfg.MaxTotalAttachmentSizeMB, DefaultMaxTotalAttachmentSizeMB)
		}

		if cfg.MaxDownloadSizeMB != DefaultMaxDownloadSizeMB {
			t.Errorf("MaxDownloadSizeMB = %d, want %d", cfg.MaxDownloadSizeMB, DefaultMaxDownloadSizeMB)
		}
	})

	t.Run("custom values", func(t *testing.T) {
		t.Setenv("MAX_ATTACHMENT_SIZE_MB", "50")
		t.Setenv("MAX_TOTAL_ATTACHMENT_SIZE_MB", "100")
		t.Setenv("MAX_DOWNLOAD_SIZE_MB", "75")

		cfg, err := LoadFromEnv()
		if err != nil {
			t.Fatalf("LoadFromEnv() error = %v", err)
		}

		if cfg.MaxAttachmentSizeMB != 50 {
			t.Errorf("MaxAttachmentSizeMB = %d, want 50", cfg.MaxAttachmentSizeMB)
		}

		if cfg.MaxTotalAttachmentSizeMB != 100 {
			t.Errorf("MaxTotalAttachmentSizeMB = %d, want 100", cfg.MaxTotalAttachmentSizeMB)
		}

		if cfg.MaxDownloadSizeMB != 75 {
			t.Errorf("MaxDownloadSizeMB = %d, want 75", cfg.MaxDownloadSizeMB)
		}
	})
}

func TestLoadFromEnv_ProviderAutoDetection(t *testing.T) {
	t.Run("auto-detect gmail", func(t *testing.T) {
		t.Setenv("EMAIL_ACCOUNTS", `[{"id":"gmail","email":"user@gmail.com",`+
			`"username":"user","password":"pass"}]`)

		cfg, err := LoadFromEnv()
		if err != nil {
			t.Fatalf("LoadFromEnv() error = %v", err)
		}

		acc := cfg.Accounts[0]

		if acc.IMAPHost != "imap.gmail.com" {
			t.Errorf("IMAPHost = %q, want imap.gmail.com", acc.IMAPHost)
		}

		if acc.SMTPHost != "smtp.gmail.com" {
			t.Errorf("SMTPHost = %q, want smtp.gmail.com", acc.SMTPHost)
		}
	})

	t.Run("explicit overrides auto-detect", func(t *testing.T) {
		t.Setenv("EMAIL_ACCOUNTS", `[{"id":"gmail","email":"user@gmail.com",`+
			`"imap_host":"custom.example.com","imap_port":993,`+
			`"smtp_host":"custom-smtp.example.com","smtp_port":587,`+
			`"username":"user","password":"pass"}]`)

		cfg, err := LoadFromEnv()
		if err != nil {
			t.Fatalf("LoadFromEnv() error = %v", err)
		}

		acc := cfg.Accounts[0]

		if acc.IMAPHost != "custom.example.com" {
			t.Errorf("IMAPHost = %q, want custom.example.com", acc.IMAPHost)
		}

		if acc.SMTPHost != "custom-smtp.example.com" {
			t.Errorf("SMTPHost = %q, want custom-smtp.example.com", acc.SMTPHost)
		}
	})
}

func TestLoadFromEnv_OAuthFields(t *testing.T) {
	t.Run("oauth fields parsed", func(t *testing.T) {
		t.Setenv("EMAIL_ACCOUNTS", `[{
			"id":"oauth-test",
			"email":"user@gmail.com",
			"imap_host":"imap.gmail.com","imap_port":993,
			"smtp_host":"smtp.gmail.com","smtp_port":587,
			"auth_method":"oauth2",
			"oauth_client_id":"my-client-id",
			"oauth_client_secret":"my-secret"
		}]`)

		cfg, err := LoadFromEnv()
		if err != nil {
			t.Fatalf("LoadFromEnv() error = %v", err)
		}

		acc := cfg.Accounts[0]

		if acc.AuthMethod != "oauth2" {
			t.Errorf("AuthMethod = %q, want oauth2", acc.AuthMethod)
		}

		if acc.OAuthClientID != "my-client-id" {
			t.Errorf("OAuthClientID = %q, want my-client-id", acc.OAuthClientID)
		}

		if acc.OAuthClientSecret != "my-secret" {
			t.Errorf("OAuthClientSecret = %q, want my-secret", acc.OAuthClientSecret)
		}
	})

	t.Run("default password auth", func(t *testing.T) {
		t.Setenv("EMAIL_ACCOUNTS", `[{"id":"test","email":"test@example.com",`+
			`"imap_host":"imap.example.com","imap_port":993,`+
			`"smtp_host":"smtp.example.com","smtp_port":465,`+
			`"username":"user","password":"pass"}]`)

		cfg, err := LoadFromEnv()
		if err != nil {
			t.Fatalf("LoadFromEnv() error = %v", err)
		}

		if cfg.Accounts[0].AuthMethod != "" {
			t.Errorf("AuthMethod = %q, want empty (default)", cfg.Accounts[0].AuthMethod)
		}
	})

	t.Run("invalid auth method", func(t *testing.T) {
		t.Setenv("EMAIL_ACCOUNTS", `[{"id":"test","email":"test@example.com",`+
			`"imap_host":"imap.example.com","imap_port":993,`+
			`"smtp_host":"smtp.example.com","smtp_port":465,`+
			`"auth_method":"bogus"}]`)

		_, err := LoadFromEnv()
		if err == nil {
			t.Error("expected error for invalid auth_method")
		}
	})

	t.Run("oauth2 missing client_id", func(t *testing.T) {
		t.Setenv("EMAIL_ACCOUNTS", `[{"id":"test","email":"test@example.com",`+
			`"imap_host":"imap.example.com","imap_port":993,`+
			`"smtp_host":"smtp.example.com","smtp_port":465,`+
			`"auth_method":"oauth2"}]`)

		_, err := LoadFromEnv()
		if err == nil {
			t.Error("expected error for oauth2 without client_id")
		}
	})

	t.Run("oauth token file override", func(t *testing.T) {
		t.Setenv("EMAIL_ACCOUNTS", `[{
			"id":"test","email":"user@gmail.com",
			"imap_host":"imap.gmail.com","imap_port":993,
			"smtp_host":"smtp.gmail.com","smtp_port":587,
			"auth_method":"oauth2",
			"oauth_client_id":"cid",
			"oauth_token_file":"/custom/path/token.json"
		}]`)

		cfg, err := LoadFromEnv()
		if err != nil {
			t.Fatalf("LoadFromEnv() error = %v", err)
		}

		if cfg.Accounts[0].OAuthTokenFile != "/custom/path/token.json" {
			t.Errorf("OAuthTokenFile = %q, want /custom/path/token.json",
				cfg.Accounts[0].OAuthTokenFile)
		}
	})
}

func TestLoadFromEnv_PoolCloseTimeoutDefault(t *testing.T) {
	t.Setenv("EMAIL_ACCOUNTS", `[{"id":"test","email":"test@example.com",`+
		`"imap_host":"imap.example.com","imap_port":993,`+
		`"smtp_host":"smtp.example.com","smtp_port":465,`+
		`"username":"user","password":"pass"}]`)

	cfg, err := LoadFromEnv()
	if err != nil {
		t.Fatalf("LoadFromEnv() error = %v", err)
	}

	if cfg.PoolCloseTimeoutMS != DefaultPoolCloseTimeoutMS {
		t.Errorf("PoolCloseTimeoutMS = %d, want %d",
			cfg.PoolCloseTimeoutMS, DefaultPoolCloseTimeoutMS)
	}
}

func TestLoadFromEnv_PoolCloseTimeoutCustom(t *testing.T) {
	t.Setenv("EMAIL_ACCOUNTS", `[{"id":"test","email":"test@example.com",`+
		`"imap_host":"imap.example.com","imap_port":993,`+
		`"smtp_host":"smtp.example.com","smtp_port":465,`+
		`"username":"user","password":"pass"}]`)
	t.Setenv("EMAIL_POOL_CLOSE_TIMEOUT_MS", "10000")

	cfg, err := LoadFromEnv()
	if err != nil {
		t.Fatalf("LoadFromEnv() error = %v", err)
	}

	if cfg.PoolCloseTimeoutMS != 10000 {
		t.Errorf("PoolCloseTimeoutMS = %d, want 10000",
			cfg.PoolCloseTimeoutMS)
	}
}

func boolPtr(b bool) *bool {
	return &b
}
