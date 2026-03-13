package smtp

import (
	"testing"

	"github.com/boutquin/mcp-server-email/internal/auth"
	gosmtp "github.com/wneessen/go-mail/smtp"
)

func TestXOAuth2Auth_Start(t *testing.T) {
	t.Parallel()

	a := &xoauth2Auth{email: "user@gmail.com", token: "access-token-xyz"}

	mech, resp, err := a.Start(&gosmtp.ServerInfo{Name: "smtp.gmail.com", TLS: true})
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	if mech != "XOAUTH2" {
		t.Errorf("mechanism = %q, want XOAUTH2", mech)
	}

	want := auth.BuildXOAuth2String("user@gmail.com", "access-token-xyz")
	if string(resp) != string(want) {
		t.Errorf("response = %q, want %q", resp, want)
	}
}

func TestXOAuth2Auth_Next_NoMore(t *testing.T) {
	t.Parallel()

	a := &xoauth2Auth{email: "user@gmail.com", token: "tok"}

	resp, err := a.Next(nil, false)
	if err != nil {
		t.Fatalf("Next(more=false) error = %v", err)
	}

	if resp != nil {
		t.Errorf("response = %q, want nil", resp)
	}
}

func TestXOAuth2Auth_Next_MoreIsError(t *testing.T) {
	t.Parallel()

	a := &xoauth2Auth{email: "user@gmail.com", token: "tok"}

	_, err := a.Next([]byte("server challenge"), true)
	if err == nil {
		t.Fatal("expected error when more=true")
	}
}

func TestIsSMTPAuthFailure(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		err  error
		want bool
	}{
		{"nil error", nil, false},
		{"535 auth failure", &testError{msg: "535 5.7.8 Authentication credentials invalid"}, true},
		{"535 plain", &testError{msg: "535 Authentication failed"}, true},
		{"421 not auth", &testError{msg: "421 service unavailable"}, false},
		{"connection reset", &testError{msg: "connection reset by peer"}, false},
		{"empty error", &testError{msg: ""}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := isSMTPAuthFailure(tt.err); got != tt.want {
				t.Errorf("isSMTPAuthFailure() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestXOAuth2Auth_Interface(t *testing.T) {
	t.Parallel()

	// Compile-time interface check.
	var _ gosmtp.Auth = (*xoauth2Auth)(nil)
}
