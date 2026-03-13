//go:build integration

package imap_test

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

// testConfig returns a *config.Config wired to the Greenmail container
// using environment variables set by CI or the Makefile target.
func testConfig(t *testing.T) *config.Config {
	t.Helper()

	host := os.Getenv("TEST_IMAP_HOST")
	if host == "" {
		t.Skip("TEST_IMAP_HOST not set — skipping integration test")
	}

	port, _ := strconv.Atoi(os.Getenv("TEST_IMAP_PORT"))
	if port == 0 {
		port = 3993
	}

	smtpHost := os.Getenv("TEST_SMTP_HOST")
	if smtpHost == "" {
		smtpHost = host
	}

	smtpPort, _ := strconv.Atoi(os.Getenv("TEST_SMTP_PORT"))
	if smtpPort == 0 {
		smtpPort = 3465
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
			IMAPHost:           host,
			IMAPPort:           port,
			SMTPHost:           smtpHost,
			SMTPPort:           smtpPort,
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

// sendTestEmail sends a plain-text email via SMTP.
func sendTestEmail(t *testing.T, cfg *config.Config, subject, body string) {
	t.Helper()

	pool := smtppkg.NewPool(cfg)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	err := pool.Send(ctx, "test", &smtppkg.SendRequest{
		To:      []string{cfg.Accounts[0].Email},
		Subject: subject,
		Body:    body,
	})
	if err != nil {
		t.Fatalf("sendTestEmail: %v", err)
	}
}

// waitForMessage polls INBOX until a message with the given subject appears, or times out.
func waitForMessage(t *testing.T, pool *imappkg.Pool, subject string) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	for {
		msgs, err := pool.ListMessages(ctx, "test", "INBOX", 50, 0, false)
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

// TestIntegration_ConnectAndListFolders verifies IMAP connection, TLS, auth, and folder listing.
func TestIntegration_ConnectAndListFolders(t *testing.T) {
	cfg := testConfig(t)
	pool := imappkg.NewPool(cfg)
	defer pool.Close(context.Background())

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	folders, err := pool.ListFolders(ctx, "test")
	if err != nil {
		t.Fatalf("ListFolders: %v", err)
	}

	if len(folders) == 0 {
		t.Fatal("expected at least one folder, got 0")
	}

	// INBOX must exist
	found := false
	for _, f := range folders {
		if f.Name == "INBOX" {
			found = true
			break
		}
	}

	if !found {
		t.Errorf("INBOX not found in folders: %v", folders)
	}
}

// TestIntegration_SendAndListMessages sends via SMTP and verifies the round-trip via IMAP.
func TestIntegration_SendAndListMessages(t *testing.T) {
	cfg := testConfig(t)
	pool := imappkg.NewPool(cfg)
	defer pool.Close(context.Background())

	subject := fmt.Sprintf("integration-roundtrip-%d", time.Now().UnixNano())
	body := "Round-trip test body"

	sendTestEmail(t, cfg, subject, body)
	waitForMessage(t, pool, subject)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	msgs, err := pool.ListMessages(ctx, "test", "INBOX", 50, 0, true)
	if err != nil {
		t.Fatalf("ListMessages: %v", err)
	}

	var found bool
	for _, m := range msgs {
		if m.Subject == subject {
			found = true
			if !strings.Contains(m.Body, body) {
				t.Errorf("body mismatch: got %q, want contains %q", m.Body, body)
			}
			break
		}
	}

	if !found {
		t.Fatalf("sent message with subject %q not found in INBOX", subject)
	}
}

// TestIntegration_SearchBySubject sends a message and searches for it by subject.
func TestIntegration_SearchBySubject(t *testing.T) {
	cfg := testConfig(t)
	pool := imappkg.NewPool(cfg)
	defer pool.Close(context.Background())

	subject := fmt.Sprintf("integration-search-%d", time.Now().UnixNano())
	sendTestEmail(t, cfg, subject, "Searchable body content")
	waitForMessage(t, pool, subject)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	results, err := pool.Search(ctx, "test", "INBOX", subject, "", "", "", "", 50, false)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}

	if len(results) == 0 {
		t.Fatal("expected at least 1 search result, got 0")
	}

	var found bool
	for _, m := range results {
		if m.Subject == subject {
			found = true
			break
		}
	}

	if !found {
		t.Errorf("search did not return message with subject %q", subject)
	}
}

// TestIntegration_DeleteMessagePermanent sends a message and permanently deletes it.
func TestIntegration_DeleteMessagePermanent(t *testing.T) {
	cfg := testConfig(t)
	pool := imappkg.NewPool(cfg)
	defer pool.Close(context.Background())

	subject := fmt.Sprintf("integration-delete-%d", time.Now().UnixNano())
	sendTestEmail(t, cfg, subject, "Delete me permanently")
	waitForMessage(t, pool, subject)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Find the message UID
	msgs, err := pool.Search(ctx, "test", "INBOX", subject, "", "", "", "", 50, false)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}

	if len(msgs) == 0 {
		t.Fatal("message not found for deletion")
	}

	// Parse UID from message ID
	_, _, uid, err := parseTestMessageID(msgs[0].ID)
	if err != nil {
		t.Fatalf("parse message ID: %v", err)
	}

	// Delete permanently
	err = pool.DeleteMessage(ctx, "test", "INBOX", uid, true)
	if err != nil {
		t.Fatalf("DeleteMessage: %v", err)
	}

	// Verify message is gone
	msgs2, err := pool.Search(ctx, "test", "INBOX", subject, "", "", "", "", 50, false)
	if err != nil {
		t.Fatalf("Search after delete: %v", err)
	}

	for _, m := range msgs2 {
		if m.Subject == subject {
			t.Error("message still found after permanent deletion")
		}
	}
}

// TestIntegration_MoveMessage sends a message and moves it to another folder.
func TestIntegration_MoveMessage(t *testing.T) {
	cfg := testConfig(t)
	pool := imappkg.NewPool(cfg)
	defer pool.Close(context.Background())

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Create target folder (ignore error if already exists)
	_ = pool.CreateFolder(ctx, "test", "TestMove")

	subject := fmt.Sprintf("integration-move-%d", time.Now().UnixNano())
	sendTestEmail(t, cfg, subject, "Move me")
	waitForMessage(t, pool, subject)

	// Find the message
	msgs, err := pool.Search(ctx, "test", "INBOX", subject, "", "", "", "", 50, false)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}

	if len(msgs) == 0 {
		t.Fatal("message not found for move")
	}

	_, _, uid, err := parseTestMessageID(msgs[0].ID)
	if err != nil {
		t.Fatalf("parse message ID: %v", err)
	}

	// Move to TestMove
	err = pool.MoveMessage(ctx, "test", "INBOX", uid, "TestMove")
	if err != nil {
		// Greenmail may not support MOVE — skip if so
		if strings.Contains(err.Error(), "MOVE") || strings.Contains(err.Error(), "Unknown command") {
			t.Skip("greenmail does not support MOVE extension")
		}

		t.Fatalf("MoveMessage: %v", err)
	}

	// Verify message is in TestMove
	moved, err := pool.Search(ctx, "test", "TestMove", subject, "", "", "", "", 50, false)
	if err != nil {
		t.Fatalf("Search TestMove: %v", err)
	}

	var found bool
	for _, m := range moved {
		if m.Subject == subject {
			found = true
			break
		}
	}

	if !found {
		t.Error("message not found in TestMove after move")
	}
}

// TestIntegration_DraftWorkflow tests save, get, and delete draft.
func TestIntegration_DraftWorkflow(t *testing.T) {
	cfg := testConfig(t)
	pool := imappkg.NewPool(cfg)
	defer pool.Close(context.Background())

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Ensure Drafts folder exists
	_ = pool.CreateFolder(ctx, "test", "Drafts")

	// Build a draft message
	draftLiteral := buildDraftLiteral(cfg.Accounts[0].Email, "Draft Subject", "Draft body content")

	// Save draft
	uid, err := pool.SaveDraft(ctx, "test", draftLiteral)
	if err != nil {
		t.Fatalf("SaveDraft: %v", err)
	}

	if uid == 0 {
		// Some servers don't return APPENDUID — that's OK, but we can't test get/delete
		t.Skip("server did not return APPENDUID, cannot verify get/delete")
	}

	// Get draft
	raw, err := pool.GetDraft(ctx, "test", uid)
	if err != nil {
		t.Fatalf("GetDraft: %v", err)
	}

	if len(raw) == 0 {
		t.Fatal("GetDraft returned empty content")
	}

	if !strings.Contains(string(raw), "Draft body content") {
		t.Errorf("draft body missing expected content, got %q", string(raw))
	}

	// Delete draft
	err = pool.DeleteDraft(ctx, "test", uid)
	if err != nil {
		t.Fatalf("DeleteDraft: %v", err)
	}
}

// TestIntegration_MarkReadAndFlag tests marking a message as read and flagged.
func TestIntegration_MarkReadAndFlag(t *testing.T) {
	cfg := testConfig(t)
	pool := imappkg.NewPool(cfg)
	defer pool.Close(context.Background())

	subject := fmt.Sprintf("integration-flags-%d", time.Now().UnixNano())
	sendTestEmail(t, cfg, subject, "Flag test body")
	waitForMessage(t, pool, subject)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Find the message
	msgs, err := pool.Search(ctx, "test", "INBOX", subject, "", "", "", "", 50, false)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}

	if len(msgs) == 0 {
		t.Fatal("message not found")
	}

	_, _, uid, err := parseTestMessageID(msgs[0].ID)
	if err != nil {
		t.Fatalf("parse message ID: %v", err)
	}

	// Mark as read
	err = pool.MarkRead(ctx, "test", "INBOX", uid, true)
	if err != nil {
		t.Fatalf("MarkRead: %v", err)
	}

	// Set flagged
	err = pool.SetFlag(ctx, "test", "INBOX", uid, true)
	if err != nil {
		t.Fatalf("SetFlag: %v", err)
	}

	// Verify via GetMessage
	msg, err := pool.GetMessage(ctx, "test", "INBOX", uid)
	if err != nil {
		t.Fatalf("GetMessage: %v", err)
	}

	if msg.IsUnread {
		t.Error("expected message to be read after MarkRead(true)")
	}

	if !msg.IsFlagged {
		t.Error("expected message to be flagged after SetFlag(true)")
	}
}

// --- attachment tests ---

func TestIntegration_AttachmentRoundtrip(t *testing.T) {
	cfg := testConfig(t)
	pool := imappkg.NewPool(cfg)

	defer pool.Close(context.Background())

	// Send email with attachment via SMTP.
	smtpPool := smtppkg.NewPool(cfg)

	tmpFile, err := os.CreateTemp(t.TempDir(), "att-roundtrip-*.txt")
	if err != nil {
		t.Fatalf("create temp file: %v", err)
	}

	attachContent := "roundtrip attachment data"

	_, err = tmpFile.WriteString(attachContent)
	if err != nil {
		t.Fatalf("write temp file: %v", err)
	}

	if closeErr := tmpFile.Close(); closeErr != nil {
		t.Fatalf("close temp file: %v", closeErr)
	}

	subject := fmt.Sprintf("att-roundtrip-%d", time.Now().UnixNano())

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	err = smtpPool.Send(ctx, "test", &smtppkg.SendRequest{
		To:      []string{cfg.Accounts[0].Email},
		Subject: subject,
		Body:    "See attachment.",
		Attachments: []smtppkg.SendAttachment{{
			Path:     tmpFile.Name(),
			Filename: "roundtrip.txt",
		}},
	})
	if err != nil {
		t.Fatalf("send with attachment: %v", err)
	}

	waitForMessage(t, pool, subject)

	// Find the message.
	msgs, err := pool.Search(ctx, "test", "INBOX", subject, "", "", "", "", 10, false)
	if err != nil {
		t.Fatalf("search: %v", err)
	}

	if len(msgs) == 0 {
		t.Fatal("message not found after send")
	}

	_, mailbox, uid, parseErr := parseTestMessageID(msgs[0].ID)
	if parseErr != nil {
		t.Fatalf("parse message ID: %v", parseErr)
	}

	// List attachments.
	attachments, err := pool.GetAttachments(ctx, "test", mailbox, uid)
	if err != nil {
		t.Fatalf("get attachments: %v", err)
	}

	if len(attachments) == 0 {
		t.Skip("Greenmail did not preserve attachment parts — skipping roundtrip verification")
	}

	if attachments[0].Filename != "roundtrip.txt" {
		t.Errorf("attachment filename = %q, want %q", attachments[0].Filename, "roundtrip.txt")
	}

	// Download attachment.
	data, filename, err := pool.GetAttachment(ctx, "test", mailbox, uid, 0)
	if err != nil {
		t.Fatalf("get attachment: %v", err)
	}

	if filename != "roundtrip.txt" {
		t.Errorf("download filename = %q, want %q", filename, "roundtrip.txt")
	}

	if !strings.Contains(string(data), attachContent) {
		t.Errorf("downloaded content does not contain expected data")
	}
}

func TestIntegration_NoAttachments(t *testing.T) {
	cfg := testConfig(t)
	pool := imappkg.NewPool(cfg)

	defer pool.Close(context.Background())

	subject := fmt.Sprintf("no-att-%d", time.Now().UnixNano())
	sendTestEmail(t, cfg, subject, "Plain text, no attachments.")
	waitForMessage(t, pool, subject)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	msgs, err := pool.Search(ctx, "test", "INBOX", subject, "", "", "", "", 10, false)
	if err != nil {
		t.Fatalf("search: %v", err)
	}

	if len(msgs) == 0 {
		t.Fatal("message not found")
	}

	_, mailbox, uid, parseErr := parseTestMessageID(msgs[0].ID)
	if parseErr != nil {
		t.Fatalf("parse message ID: %v", parseErr)
	}

	attachments, err := pool.GetAttachments(ctx, "test", mailbox, uid)
	if err != nil {
		t.Fatalf("get attachments: %v", err)
	}

	if len(attachments) != 0 {
		t.Errorf("expected 0 attachments for plain text email, got %d", len(attachments))
	}
}

// --- helpers ---

// parseTestMessageID splits "account:mailbox:uid" into components.
func parseTestMessageID(id string) (string, string, uint32, error) {
	parts := strings.SplitN(id, ":", 3)
	if len(parts) != 3 {
		return "", "", 0, fmt.Errorf("invalid message ID: %s", id)
	}

	uid, err := strconv.ParseUint(parts[2], 10, 32)
	if err != nil {
		return "", "", 0, fmt.Errorf("invalid UID in message ID %s: %w", id, err)
	}

	return parts[0], parts[1], uint32(uid), nil
}

// buildDraftLiteral creates a minimal RFC 5322 message for draft saving.
func buildDraftLiteral(from, subject, body string) []byte {
	msg := fmt.Sprintf("From: %s\r\nSubject: %s\r\nContent-Type: text/plain; charset=utf-8\r\n\r\n%s",
		from, subject, body)
	return []byte(msg)
}
