package auth //nolint:testpackage // tests unexported domainFromEmail

import "testing"

func TestDetectOAuthProvider_Gmail(t *testing.T) {
	t.Parallel()

	p := DetectOAuthProvider("user@gmail.com")
	if p == nil {
		t.Fatal("expected Gmail provider, got nil")
	}

	if p.Name != testProviderGmail {
		t.Errorf("Name = %q, want Gmail", p.Name)
	}
}

func TestDetectOAuthProvider_Googlemail(t *testing.T) {
	t.Parallel()

	p := DetectOAuthProvider("user@googlemail.com")
	if p == nil {
		t.Fatal("expected Gmail provider, got nil")
	}

	if p.Name != testProviderGmail {
		t.Errorf("Name = %q, want Gmail", p.Name)
	}
}

func TestDetectOAuthProvider_Outlook(t *testing.T) {
	t.Parallel()

	p := DetectOAuthProvider("user@outlook.com")
	if p == nil {
		t.Fatal("expected Outlook provider, got nil")
	}

	if p.Name != testProviderOutlook {
		t.Errorf("Name = %q, want Outlook", p.Name)
	}
}

func TestDetectOAuthProvider_Hotmail(t *testing.T) {
	t.Parallel()

	p := DetectOAuthProvider("user@hotmail.com")
	if p == nil {
		t.Fatal("expected Outlook provider, got nil")
	}

	if p.Name != testProviderOutlook {
		t.Errorf("Name = %q, want Outlook", p.Name)
	}
}

func TestDetectOAuthProvider_Live(t *testing.T) {
	t.Parallel()

	p := DetectOAuthProvider("user@live.com")
	if p == nil {
		t.Fatal("expected Outlook provider, got nil")
	}

	if p.Name != testProviderOutlook {
		t.Errorf("Name = %q, want Outlook", p.Name)
	}
}

func TestDetectOAuthProvider_Unknown(t *testing.T) {
	t.Parallel()

	p := DetectOAuthProvider("user@example.com")
	if p != nil {
		t.Errorf("expected nil for unknown domain, got %+v", p)
	}
}

func TestDetectOAuthProvider_InvalidEmail(t *testing.T) {
	t.Parallel()

	tests := []string{
		"noemail",
		"",
		"user@",
	}

	for _, email := range tests {
		p := DetectOAuthProvider(email)
		if p != nil {
			t.Errorf("DetectOAuthProvider(%q) = %+v, want nil", email, p)
		}
	}
}

func TestDetectOAuthProvider_CaseInsensitive(t *testing.T) {
	t.Parallel()

	p := DetectOAuthProvider("user@Gmail.COM")
	if p == nil {
		t.Fatal("expected Gmail provider for uppercase domain, got nil")
	}

	if p.Name != testProviderGmail {
		t.Errorf("Name = %q, want Gmail", p.Name)
	}
}

func TestProviderRegistry_Gmail(t *testing.T) {
	t.Parallel()

	p, ok := Providers["gmail"]
	if !ok {
		t.Fatal("gmail provider not in registry")
	}

	if p.AuthURL != "https://accounts.google.com/o/oauth2/auth" {
		t.Errorf("AuthURL = %q", p.AuthURL)
	}

	if p.TokenURL != "https://oauth2.googleapis.com/token" {
		t.Errorf("TokenURL = %q", p.TokenURL)
	}

	if p.DeviceAuthURL != "https://oauth2.googleapis.com/device/code" {
		t.Errorf("DeviceAuthURL = %q", p.DeviceAuthURL)
	}

	if len(p.Scopes) != 1 || p.Scopes[0] != "https://mail.google.com/" {
		t.Errorf("Scopes = %v", p.Scopes)
	}
}

func TestProviderRegistry_Outlook(t *testing.T) {
	t.Parallel()

	p, ok := Providers["outlook"]
	if !ok {
		t.Fatal("outlook provider not in registry")
	}

	if p.AuthURL != "https://login.microsoftonline.com/common/oauth2/v2.0/authorize" {
		t.Errorf("AuthURL = %q", p.AuthURL)
	}

	if p.TokenURL != "https://login.microsoftonline.com/common/oauth2/v2.0/token" {
		t.Errorf("TokenURL = %q", p.TokenURL)
	}

	if p.DeviceAuthURL != "https://login.microsoftonline.com/common/oauth2/v2.0/devicecode" {
		t.Errorf("DeviceAuthURL = %q", p.DeviceAuthURL)
	}

	if len(p.Scopes) != 3 {
		t.Fatalf("expected 3 scopes, got %d: %v", len(p.Scopes), p.Scopes)
	}
}

func TestDomainFromEmail(t *testing.T) {
	t.Parallel()

	tests := []struct {
		email string
		want  string
	}{
		{"user@gmail.com", "gmail.com"},
		{"user@Gmail.COM", "gmail.com"},
		{"noemail", ""},
		{"", ""},
		{"user@", ""},
	}

	for _, tt := range tests {
		got := domainFromEmail(tt.email)
		if got != tt.want {
			t.Errorf("domainFromEmail(%q) = %q, want %q", tt.email, got, tt.want)
		}
	}
}
