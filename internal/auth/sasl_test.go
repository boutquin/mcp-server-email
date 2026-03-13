package auth //nolint:testpackage // tests verify unexported struct fields

import (
	"testing"

	"github.com/emersion/go-sasl"
)

func TestBuildXOAuth2String(t *testing.T) {
	t.Parallel()

	got := BuildXOAuth2String("user@example.com", "test-access-token")
	want := "user=user@example.com\x01auth=Bearer test-access-token\x01\x01"

	if string(got) != want {
		t.Errorf("BuildXOAuth2String() = %q, want %q", got, want)
	}
}

func TestBuildXOAuth2String_Empty(t *testing.T) {
	t.Parallel()

	got := BuildXOAuth2String("", "")
	want := "user=\x01auth=Bearer \x01\x01"

	if string(got) != want {
		t.Errorf("BuildXOAuth2String() = %q, want %q", got, want)
	}
}

func TestXOAuth2Client_Start(t *testing.T) {
	t.Parallel()

	client := NewXOAuth2Client("user@gmail.com", "access-token-xyz")

	mech, ir, err := client.Start()
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	if mech != "XOAUTH2" {
		t.Errorf("mechanism = %q, want XOAUTH2", mech)
	}

	wantIR := "user=user@gmail.com\x01auth=Bearer access-token-xyz\x01\x01"
	if string(ir) != wantIR {
		t.Errorf("initial response = %q, want %q", ir, wantIR)
	}
}

func TestXOAuth2Client_Next_Challenge(t *testing.T) {
	t.Parallel()

	client := NewXOAuth2Client("user@gmail.com", "token")

	// In XOAUTH2, any server challenge means authentication failed.
	resp, err := client.Next([]byte("error data from server"))
	if err == nil {
		t.Fatal("expected error from Next() with challenge")
	}

	// Response should be empty (protocol requirement).
	if len(resp) != 0 {
		t.Errorf("response = %q, want empty", resp)
	}
}

func TestXOAuth2Client_Next_Empty(t *testing.T) {
	t.Parallel()

	client := NewXOAuth2Client("user@gmail.com", "token")

	// Empty challenge (should still indicate server error in XOAUTH2).
	resp, err := client.Next([]byte{})
	if err == nil {
		t.Fatal("expected error from Next() with empty challenge")
	}

	if len(resp) != 0 {
		t.Errorf("response = %q, want empty", resp)
	}
}

func TestXOAuth2Client_Interface(t *testing.T) {
	t.Parallel()

	// Compile-time interface check.
	var _ sasl.Client = (*XOAuth2Client)(nil)
}
