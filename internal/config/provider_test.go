package config

import (
	"testing"
)

func TestDetectProvider_AllDomains(t *testing.T) {
	t.Parallel()

	tests := []struct {
		email    string
		wantIMAP string
		wantSMTP string
		wantPort int
	}{
		{"user@gmail.com", "imap.gmail.com", "smtp.gmail.com", 587},
		{"user@googlemail.com", "imap.gmail.com", "smtp.gmail.com", 587},
		{"user@outlook.com", "outlook.office365.com", "smtp.office365.com", 587},
		{"user@hotmail.com", "outlook.office365.com", "smtp.office365.com", 587},
		{"user@live.com", "outlook.office365.com", "smtp.office365.com", 587},
		{"user@yahoo.com", "imap.mail.yahoo.com", "smtp.mail.yahoo.com", 587},
		{"user@icloud.com", "imap.mail.me.com", "smtp.mail.me.com", 587},
		{"user@me.com", "imap.mail.me.com", "smtp.mail.me.com", 587},
		{"user@mac.com", "imap.mail.me.com", "smtp.mail.me.com", 587},
		{"user@fastmail.com", "imap.fastmail.com", "smtp.fastmail.com", 587},
		{"user@fastmail.fm", "imap.fastmail.com", "smtp.fastmail.com", 587},
		{"user@zoho.com", "imap.zoho.com", "smtp.zoho.com", 587},
		{"user@zohomail.com", "imap.zoho.com", "smtp.zoho.com", 587},
	}

	for _, tt := range tests {
		t.Run(tt.email, func(t *testing.T) {
			t.Parallel()

			p := DetectProvider(tt.email)
			if p == nil {
				t.Fatalf("expected provider detection for %s", tt.email)
			}

			if p.IMAPHost != tt.wantIMAP {
				t.Errorf("IMAP host = %q, want %q", p.IMAPHost, tt.wantIMAP)
			}

			if p.IMAPPort != 993 {
				t.Errorf("IMAP port = %d, want 993", p.IMAPPort)
			}

			if p.SMTPHost != tt.wantSMTP {
				t.Errorf("SMTP host = %q, want %q", p.SMTPHost, tt.wantSMTP)
			}

			if p.SMTPPort != tt.wantPort {
				t.Errorf("SMTP port = %d, want %d", p.SMTPPort, tt.wantPort)
			}
		})
	}
}

func TestDetectProvider_Unknown(t *testing.T) {
	t.Parallel()

	p := DetectProvider("user@example.com")
	if p != nil {
		t.Error("expected nil for unknown domain")
	}
}

func TestDetectProvider_InvalidEmail(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		email string
	}{
		{"no at sign", "userexample.com"},
		{"empty string", ""},
		{"trailing at", "user@"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			p := DetectProvider(tt.email)
			if p != nil {
				t.Errorf("expected nil for %q", tt.email)
			}
		})
	}
}

func TestDetectProvider_CaseInsensitive(t *testing.T) {
	t.Parallel()

	p := DetectProvider("User@Gmail.COM")
	if p == nil {
		t.Fatal("expected provider detection for mixed-case domain")
	}

	if p.IMAPHost != "imap.gmail.com" {
		t.Errorf("IMAP host = %q, want %q", p.IMAPHost, "imap.gmail.com")
	}
}

func TestDetectProvider_MultipleAtSigns(t *testing.T) {
	t.Parallel()

	p := DetectProvider("user@middle@gmail.com")
	if p == nil {
		t.Fatal("expected provider detection using last @ segment")
	}
}

func TestApplyProviderDefaults_ExplicitOverrides(t *testing.T) {
	t.Parallel()

	a := &Account{
		Email:    "user@gmail.com",
		IMAPHost: "custom-imap.example.com",
		IMAPPort: 143,
		SMTPHost: "custom-smtp.example.com",
		SMTPPort: 465,
	}

	applyProviderDefaults(a)

	if a.IMAPHost != "custom-imap.example.com" {
		t.Errorf("IMAP host overwritten: got %q", a.IMAPHost)
	}

	if a.SMTPHost != "custom-smtp.example.com" {
		t.Errorf("SMTP host overwritten: got %q", a.SMTPHost)
	}
}

func TestApplyProviderDefaults_PartialOverride(t *testing.T) {
	t.Parallel()

	a := &Account{
		Email:    "user@gmail.com",
		IMAPHost: "custom-imap.example.com",
		IMAPPort: 143,
	}

	applyProviderDefaults(a)

	if a.IMAPHost != "custom-imap.example.com" {
		t.Errorf("IMAP host overwritten: got %q", a.IMAPHost)
	}

	if a.SMTPHost != "smtp.gmail.com" {
		t.Errorf("SMTP host not auto-detected: got %q", a.SMTPHost)
	}

	if a.SMTPPort != 587 {
		t.Errorf("SMTP port not auto-detected: got %d", a.SMTPPort)
	}
}
