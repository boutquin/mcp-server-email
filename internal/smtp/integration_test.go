//go:build integration

package smtp_test

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/boutquin/mcp-server-email/internal/config"
	imappkg "github.com/boutquin/mcp-server-email/internal/imap"
	smtppkg "github.com/boutquin/mcp-server-email/internal/smtp"
)

// smtpTestConfig returns a *config.Config for integration tests.
func smtpTestConfig(t *testing.T) *config.Config {
	t.Helper()

	host := os.Getenv("TEST_SMTP_HOST")
	if host == "" {
		t.Skip("TEST_SMTP_HOST not set — skipping integration test")
	}

	port, _ := strconv.Atoi(os.Getenv("TEST_SMTP_PORT"))
	if port == 0 {
		port = 3465
	}

	imapHost := os.Getenv("TEST_IMAP_HOST")
	if imapHost == "" {
		imapHost = host
	}

	imapPort, _ := strconv.Atoi(os.Getenv("TEST_IMAP_PORT"))
	if imapPort == 0 {
		imapPort = 3993
	}

	email := os.Getenv("TEST_EMAIL")
	if email == "" {
		email = "test@example.com"
	}

	username := os.Getenv("TEST_USERNAME")
	if username == "" {
		// Greenmail uses the local part (before @) as the login username
		username = strings.SplitN(email, "@", 2)[0]
	}

	password := os.Getenv("TEST_PASSWORD")
	if password == "" {
		password = "password"
	}

	// UseStartTLS=false → implicit TLS (required for Greenmail SSL ports 3993/3465)
	useStartTLS := false

	return &config.Config{
		Accounts: []config.Account{{
			ID:                 "test",
			Email:              email,
			IMAPHost:           imapHost,
			IMAPPort:           imapPort,
			SMTPHost:           host,
			SMTPPort:           port,
			Username:           username,
			Password:           password,
			UseStartTLS:        &useStartTLS,
			InsecureSkipVerify: true,
		}},
		DefaultAccount:   "test",
		IMAPRateLimitRPM: 120,
		SMTPRateLimitRPH: 120,
		IMAPTimeoutMS:    10000,
		SMTPTimeoutMS:    10000,
	}
}

// waitForSubject polls INBOX via IMAP until a message with the given subject appears.
func waitForSubject(t *testing.T, imapPool *imappkg.Pool, subject string) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	for {
		msgs, err := imapPool.ListMessages(ctx, "test", "INBOX", 50, 0, false)
		if err == nil {
			for _, m := range msgs {
				if m.Subject == subject {
					return
				}
			}
		}

		select {
		case <-ctx.Done():
			t.Fatalf("timed out waiting for message with subject %q", subject)
		case <-time.After(500 * time.Millisecond):
		}
	}
}

// TestIntegration_SendPlainText sends a plain-text email and verifies delivery via IMAP.
func TestIntegration_SendPlainText(t *testing.T) {
	cfg := smtpTestConfig(t)
	pool := smtppkg.NewPool(cfg)
	imapPool := imappkg.NewPool(cfg)
	defer imapPool.Close(context.Background())

	subject := fmt.Sprintf("smtp-plain-%d", time.Now().UnixNano())

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	err := pool.Send(ctx, "test", &smtppkg.SendRequest{
		To:      []string{cfg.Accounts[0].Email},
		Subject: subject,
		Body:    "Plain text body",
	})
	if err != nil {
		t.Fatalf("Send plain text: %v", err)
	}

	waitForSubject(t, imapPool, subject)

	// Verify the message content via IMAP
	msgs, err := imapPool.Search(ctx, "test", "INBOX", subject, "", "", "", "", 50, true)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}

	if len(msgs) == 0 {
		t.Fatal("sent message not found")
	}

	if !strings.Contains(msgs[0].Body, "Plain text body") {
		t.Errorf("body = %q, want contains %q", msgs[0].Body, "Plain text body")
	}
}

// TestIntegration_SendHTML sends an HTML email and verifies Content-Type.
func TestIntegration_SendHTML(t *testing.T) {
	cfg := smtpTestConfig(t)
	pool := smtppkg.NewPool(cfg)
	imapPool := imappkg.NewPool(cfg)
	defer imapPool.Close(context.Background())

	subject := fmt.Sprintf("smtp-html-%d", time.Now().UnixNano())

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	err := pool.Send(ctx, "test", &smtppkg.SendRequest{
		To:      []string{cfg.Accounts[0].Email},
		Subject: subject,
		Body:    "<h1>Hello</h1><p>HTML body</p>",
		IsHTML:  true,
	})
	if err != nil {
		t.Fatalf("Send HTML: %v", err)
	}

	waitForSubject(t, imapPool, subject)

	msgs, err := imapPool.Search(ctx, "test", "INBOX", subject, "", "", "", "", 50, true)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}

	if len(msgs) == 0 {
		t.Fatal("sent message not found")
	}

	// Verify Content-Type is HTML
	if !strings.Contains(msgs[0].ContentType, "html") {
		t.Errorf("ContentType = %q, want contains 'html'", msgs[0].ContentType)
	}
}

// TestIntegration_SendWithAttachment sends an email with an attachment and verifies
// the multipart MIME structure is received correctly.
func TestIntegration_SendWithAttachment(t *testing.T) {
	cfg := smtpTestConfig(t)
	pool := smtppkg.NewPool(cfg)
	imapPool := imappkg.NewPool(cfg)
	defer imapPool.Close(context.Background())

	// Create a temporary file for the attachment
	tmpFile, err := os.CreateTemp("", "test-attachment-*.txt")
	if err != nil {
		t.Fatalf("create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())

	_, err = tmpFile.WriteString("attachment content here")
	if err != nil {
		t.Fatalf("write temp file: %v", err)
	}
	tmpFile.Close()

	subject := fmt.Sprintf("smtp-attach-%d", time.Now().UnixNano())

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	err = pool.Send(ctx, "test", &smtppkg.SendRequest{
		To:      []string{cfg.Accounts[0].Email},
		Subject: subject,
		Body:    "Email with attachment",
		Attachments: []smtppkg.SendAttachment{{
			Path:     tmpFile.Name(),
			Filename: "test-file.txt",
		}},
	})
	if err != nil {
		t.Fatalf("Send with attachment: %v", err)
	}

	waitForSubject(t, imapPool, subject)

	msgs, err := imapPool.Search(ctx, "test", "INBOX", subject, "", "", "", "", 50, false)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}

	if len(msgs) == 0 {
		t.Fatal("sent message not found")
	}

	// Verify attachment metadata
	if len(msgs[0].Attachments) == 0 {
		t.Error("expected at least 1 attachment, got 0")
	} else {
		att := msgs[0].Attachments[0]
		if att.Filename != "test-file.txt" {
			t.Errorf("attachment filename = %q, want %q", att.Filename, "test-file.txt")
		}
	}
}

// TestIntegration_RateLimitTokenConsumption verifies that sending consumes rate limit tokens.
func TestIntegration_RateLimitTokenConsumption(t *testing.T) {
	cfg := smtpTestConfig(t)
	pool := smtppkg.NewPool(cfg)

	// Get client to check tokens
	client, err := pool.Get("test")
	if err != nil {
		t.Fatalf("Get client: %v", err)
	}

	tokensBefore, limit, _ := client.RateLimitInfo()
	if tokensBefore != limit {
		t.Fatalf("expected fresh tokens = limit (%d), got %d", limit, tokensBefore)
	}

	// Send one email
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	subject := fmt.Sprintf("smtp-ratelimit-%d", time.Now().UnixNano())

	err = pool.Send(ctx, "test", &smtppkg.SendRequest{
		To:      []string{cfg.Accounts[0].Email},
		Subject: subject,
		Body:    "Rate limit test",
	})
	if err != nil {
		t.Fatalf("Send: %v", err)
	}

	tokensAfter, _, _ := client.RateLimitInfo()
	if tokensAfter >= tokensBefore {
		t.Errorf("tokens not consumed: before=%d, after=%d", tokensBefore, tokensAfter)
	}
}
