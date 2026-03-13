package smtp

import (
	"bytes"
	"context"
	"mime"
	"mime/multipart"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/boutquin/mcp-server-email/internal/config"
)

// newTestClient returns a Client suitable for calling buildMessage
// (no network connection needed).
func newTestClient() *Client {
	return &Client{
		account: &config.Account{
			Email: "sender@example.com",
		},
	}
}

// buildAndCapture calls buildMessage and returns the raw MIME output.
func buildAndCapture(t *testing.T, c *Client, req *SendRequest) string {
	t.Helper()

	m, err := c.buildMessage(req)
	if err != nil {
		t.Fatalf("buildMessage() error = %v", err)
	}

	var buf bytes.Buffer

	_, err = m.WriteTo(&buf)
	if err != nil {
		t.Fatalf("WriteTo() error = %v", err)
	}

	return buf.String()
}

// requireContains is a test helper that fails if raw does not contain substr.
func requireContains(t *testing.T, raw, substr, ctx string) {
	t.Helper()

	if !strings.Contains(raw, substr) {
		t.Errorf("expected %s to contain %q", ctx, substr)
	}
}

func TestBuildMessage_PlainText(t *testing.T) {
	t.Parallel()

	raw := buildAndCapture(t, newTestClient(), &SendRequest{
		To:      []string{"to@example.com"},
		Subject: "Plain test",
		Body:    "Hello, world!",
		IsHTML:  false,
	})

	requireContains(t, raw, "Content-Type: text/plain", "MIME output")
	requireContains(t, raw, "Subject: Plain test", "MIME output")
	requireContains(t, raw, "sender@example.com", "From header")
	requireContains(t, raw, "to@example.com", "To header")
	requireContains(t, raw, "Hello, world!", "body")
}

func TestBuildMessage_HTML(t *testing.T) {
	t.Parallel()

	raw := buildAndCapture(t, newTestClient(), &SendRequest{
		To:      []string{"to@example.com"},
		Subject: "HTML test",
		Body:    "<h1>Hello</h1>",
		IsHTML:  true,
	})

	requireContains(t, raw, "Content-Type: text/html", "MIME output")
	requireContains(t, raw, "<h1>Hello</h1>", "body")
}

func TestBuildMessage_WithCC(t *testing.T) {
	t.Parallel()

	raw := buildAndCapture(t, newTestClient(), &SendRequest{
		To:      []string{"to@example.com"},
		CC:      []string{"cc1@example.com", "cc2@example.com"},
		Subject: "CC test",
		Body:    "body",
	})

	// go-mail uses "Cc:" header with angle-bracketed addresses.
	requireContains(t, raw, "Cc:", "MIME output")
	requireContains(t, raw, "cc1@example.com", "Cc header")
	requireContains(t, raw, "cc2@example.com", "Cc header")
}

func TestBuildMessage_WithBCC(t *testing.T) {
	t.Parallel()

	raw := buildAndCapture(t, newTestClient(), &SendRequest{
		To:      []string{"to@example.com"},
		BCC:     []string{"bcc@example.com"},
		Subject: "BCC test",
		Body:    "body",
	})

	// BCC must NOT appear in the message headers (only in the SMTP envelope).
	if strings.Contains(raw, "bcc@example.com") {
		t.Error("BCC address must not appear in raw MIME headers")
	}

	// The message should still be valid.
	requireContains(t, raw, "Subject: BCC test", "MIME output")
}

func TestBuildMessage_WithReplyTo(t *testing.T) {
	t.Parallel()

	raw := buildAndCapture(t, newTestClient(), &SendRequest{
		To:      []string{"to@example.com"},
		Subject: "ReplyTo test",
		Body:    "body",
		ReplyTo: "replyto@example.com",
	})

	requireContains(t, raw, "Reply-To:", "MIME output")
	requireContains(t, raw, "replyto@example.com", "Reply-To header")
}

func TestBuildMessage_WithAttachments(t *testing.T) {
	t.Parallel()

	// Create a temporary file to attach.
	tmpDir := t.TempDir()

	attachPath := filepath.Join(tmpDir, "report.csv")

	err := os.WriteFile(attachPath, []byte("col1,col2\na,b\n"), 0o644)
	if err != nil {
		t.Fatalf("failed to create temp attachment: %v", err)
	}

	raw := buildAndCapture(t, newTestClient(), &SendRequest{
		To:      []string{"to@example.com"},
		Subject: "Attachment test",
		Body:    "See attached.",
		Attachments: []SendAttachment{
			{Path: attachPath, Filename: "report.csv"},
		},
	})

	// Must be multipart.
	requireContains(t, raw, "Content-Type: multipart/", "MIME output")

	// Verify the attachment filename appears in the raw output.
	requireContains(t, raw, "report.csv", "attachment filename")

	// Extract boundary and verify multipart structure.
	boundary := extractBoundary(t, raw)

	// Find the body after the top-level headers.
	_, mimeBody, found := strings.Cut(raw, "\r\n\r\n")
	if !found {
		t.Fatal("could not find header/body separator")
	}

	reader := multipart.NewReader(strings.NewReader(mimeBody), boundary)

	var partCount int

	for {
		part, partErr := reader.NextPart()
		if partErr != nil {
			break
		}

		partCount++

		part.Close() //nolint:errcheck // test helper, close error irrelevant
	}

	if partCount < 2 {
		t.Errorf("expected at least 2 multipart parts (body + attachment), got %d", partCount)
	}
}

func TestBuildMessage_EmptySubject(t *testing.T) {
	t.Parallel()

	c := newTestClient()

	m, err := c.buildMessage(&SendRequest{
		To:      []string{"to@example.com"},
		Subject: "",
		Body:    "no subject",
	})
	if err != nil {
		t.Fatalf("buildMessage() with empty subject should not error, got: %v", err)
	}

	var buf bytes.Buffer

	_, err = m.WriteTo(&buf)
	if err != nil {
		t.Fatalf("WriteTo() error = %v", err)
	}

	raw := buf.String()

	// Message must still be valid MIME even with empty subject.
	requireContains(t, raw, "sender@example.com", "From header")
	requireContains(t, raw, "no subject", "body")
}

func TestBuildMessage_InvalidFrom(t *testing.T) {
	t.Parallel()

	c := &Client{
		account: &config.Account{
			Email: "not-an-email",
		},
	}

	_, err := c.buildMessage(&SendRequest{
		To:   []string{"to@example.com"},
		Body: "body",
	})
	if err == nil {
		t.Error("expected error for invalid From address")
	}
}

func TestBuildMessage_InvalidTo(t *testing.T) {
	t.Parallel()

	_, err := newTestClient().buildMessage(&SendRequest{
		To:   []string{"not-an-email"},
		Body: "body",
	})
	if err == nil {
		t.Error("expected error for invalid To address")
	}
}

func TestBuildMessage_InvalidCC(t *testing.T) {
	t.Parallel()

	_, err := newTestClient().buildMessage(&SendRequest{
		To:   []string{"to@example.com"},
		CC:   []string{"not-an-email"},
		Body: "body",
	})
	if err == nil {
		t.Error("expected error for invalid CC address")
	}
}

func TestBuildMessage_InvalidBCC(t *testing.T) {
	t.Parallel()

	_, err := newTestClient().buildMessage(&SendRequest{
		To:   []string{"to@example.com"},
		BCC:  []string{"not-an-email"},
		Body: "body",
	})
	if err == nil {
		t.Error("expected error for invalid BCC address")
	}
}

func TestBuildMessage_InvalidReplyTo(t *testing.T) {
	t.Parallel()

	_, err := newTestClient().buildMessage(&SendRequest{
		To:      []string{"to@example.com"},
		ReplyTo: "not-an-email",
		Body:    "body",
	})
	if err == nil {
		t.Error("expected error for invalid ReplyTo address")
	}
}

func TestPool_AccountEmail(t *testing.T) {
	t.Parallel()

	pool := NewPool(&config.Config{
		Accounts: []config.Account{
			{ID: "acc1", Email: "one@example.com", SMTPHost: "smtp.example.com", SMTPPort: 587},
		},
		DefaultAccount:   "acc1",
		SMTPRateLimitRPH: 100,
		SMTPTimeoutMS:    30000,
	})

	email, err := pool.AccountEmail("acc1")
	if err != nil {
		t.Fatalf("AccountEmail() error = %v", err)
	}

	if email != "one@example.com" {
		t.Errorf("AccountEmail() = %q, want %q", email, "one@example.com")
	}

	_, err = pool.AccountEmail("nonexistent")
	if err == nil {
		t.Error("expected error for nonexistent account")
	}
}

func TestPool_DefaultAccountID(t *testing.T) {
	t.Parallel()

	pool := NewPool(&config.Config{
		DefaultAccount: "default-acc",
	})

	if got := pool.DefaultAccountID(); got != "default-acc" {
		t.Errorf("DefaultAccountID() = %q, want %q", got, "default-acc")
	}
}

func TestPool_Send_AccountNotFound(t *testing.T) {
	t.Parallel()

	pool := NewPool(&config.Config{
		Accounts:       []config.Account{},
		DefaultAccount: "acc1",
	})

	err := pool.Send(context.Background(), "nonexistent", &SendRequest{
		To:   []string{"to@example.com"},
		Body: "body",
	})
	if err == nil {
		t.Error("expected error for nonexistent account")
	}
}

// extractBoundary finds the Content-Type header line in raw MIME output
// and returns the boundary parameter.
func extractBoundary(t *testing.T, raw string) string {
	t.Helper()

	// Find the Content-Type header value. It may be folded across lines,
	// so collect everything from "Content-Type:" up to the next unfolded header.
	lines := strings.Split(raw, "\r\n")
	ctParts := make([]string, 0, len(lines))

	for i, line := range lines {
		value, found := strings.CutPrefix(line, "Content-Type:")
		if !found {
			continue
		}

		ctParts = append(ctParts, value)

		// Collect continuation lines (start with whitespace).
		for j := i + 1; j < len(lines); j++ {
			if len(lines[j]) > 0 && (lines[j][0] == ' ' || lines[j][0] == '\t') {
				ctParts = append(ctParts, strings.TrimSpace(lines[j]))
			} else {
				break
			}
		}

		break
	}

	ctValue := strings.TrimSpace(strings.Join(ctParts, " "))
	if ctValue == "" {
		t.Fatal("no Content-Type header found")
	}

	_, params, err := mime.ParseMediaType(ctValue)
	if err != nil {
		t.Fatalf("failed to parse Content-Type %q: %v", ctValue, err)
	}

	boundary := params["boundary"]
	if boundary == "" {
		t.Fatal("no boundary parameter in Content-Type")
	}

	return boundary
}
