package tools_test

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/boutquin/mcp-server-email/internal/models"
	"github.com/boutquin/mcp-server-email/internal/tools"
	"github.com/mark3labs/mcp-go/mcp"
)

var errConnectionReset = errors.New("connection reset")

func TestForwardHandler_HappyPath(t *testing.T) {
	t.Parallel()

	imapMock := &mockIMAPOps{
		defaultAccountID: "acct1",
		getMessageResult: &models.Email{
			ID:      "acct1:INBOX:42",
			Subject: "Important Update",
			From:    "alice@example.com",
			To:      []string{"me@example.com"},
			Date:    "2026-02-22T10:00:00Z",
			Body:    "Here is the update.",
			Mailbox: "INBOX",
		},
	}

	smtpMock := &mockSMTPOps{
		defaultAccountID: "acct1",
		accountEmails:    map[string]string{"acct1": "me@example.com"},
	}

	handler := tools.ForwardHandler(imapMock, smtpMock, testLimits)
	req := toolReq(map[string]any{
		"id": "acct1:INBOX:42",
		"to": "bob@example.com",
	})

	result, err := handler(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	text := resultText(t, result)

	var res map[string]any

	err = json.Unmarshal([]byte(text), &res)
	if err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	if res["success"] != true {
		t.Fatalf("expected success=true, got %v", res["success"])
	}

	// Verify To = specified recipient, not original sender
	if len(smtpMock.lastSendReq.To) != 1 || smtpMock.lastSendReq.To[0] != "bob@example.com" {
		t.Errorf("To = %v, want [bob@example.com]", smtpMock.lastSendReq.To)
	}

	// Verify Subject = "Fwd: Important Update"
	if smtpMock.lastSendReq.Subject != "Fwd: Important Update" {
		t.Errorf("Subject = %q, want %q", smtpMock.lastSendReq.Subject, "Fwd: Important Update")
	}

	// Verify forwarded content has separator
	if !strings.Contains(smtpMock.lastSendReq.Body, "---------- Forwarded message ----------") {
		t.Errorf("body should contain forwarded message separator")
	}

	// Verify forwarded content includes original headers
	if !strings.Contains(smtpMock.lastSendReq.Body, "From: alice@example.com") {
		t.Errorf("body should contain original From header")
	}

	if !strings.Contains(smtpMock.lastSendReq.Body, "Subject: Important Update") {
		t.Errorf("body should contain original Subject header")
	}

	// Verify forwarded content includes original body
	if !strings.Contains(smtpMock.lastSendReq.Body, "Here is the update.") {
		t.Errorf("body should contain original message body")
	}
}

func TestForwardHandler_WithNote(t *testing.T) {
	t.Parallel()

	imapMock := &mockIMAPOps{
		defaultAccountID: "acct1",
		getMessageResult: &models.Email{
			ID:      "acct1:INBOX:42",
			Subject: "FYI",
			From:    "alice@example.com",
			To:      []string{"me@example.com"},
			Date:    "2026-02-22T10:00:00Z",
			Body:    "Check this out.",
			Mailbox: "INBOX",
		},
	}

	smtpMock := &mockSMTPOps{
		defaultAccountID: "acct1",
		accountEmails:    map[string]string{"acct1": "me@example.com"},
	}

	handler := tools.ForwardHandler(imapMock, smtpMock, testLimits)
	req := toolReq(map[string]any{
		"id":   "acct1:INBOX:42",
		"to":   "bob@example.com",
		"body": "Take a look at this",
	})

	result, err := handler(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	text := resultText(t, result)

	var res map[string]any

	err = json.Unmarshal([]byte(text), &res)
	if err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	// Verify user's note appears above forwarded content
	noteIdx := strings.Index(smtpMock.lastSendReq.Body, "Take a look at this")
	fwdIdx := strings.Index(smtpMock.lastSendReq.Body, "---------- Forwarded message ----------")

	if noteIdx < 0 {
		t.Fatal("body should contain user's note")
	}

	if fwdIdx < 0 {
		t.Fatal("body should contain forwarded message separator")
	}

	if noteIdx > fwdIdx {
		t.Errorf("user's note should appear above forwarded content (note at %d, fwd at %d)", noteIdx, fwdIdx)
	}
}

func TestForwardHandler_AlreadyHasFwdPrefix(t *testing.T) {
	t.Parallel()

	imapMock := &mockIMAPOps{
		defaultAccountID: "acct1",
		getMessageResult: &models.Email{
			ID:      "acct1:INBOX:42",
			Subject: "Fwd: Already forwarded",
			From:    "alice@example.com",
			To:      []string{"me@example.com"},
			Date:    "2026-02-22T10:00:00Z",
			Body:    "Forwarded content",
			Mailbox: "INBOX",
		},
	}

	smtpMock := &mockSMTPOps{
		defaultAccountID: "acct1",
		accountEmails:    map[string]string{"acct1": "me@example.com"},
	}

	handler := tools.ForwardHandler(imapMock, smtpMock, testLimits)
	req := toolReq(map[string]any{
		"id": "acct1:INBOX:42",
		"to": "bob@example.com",
	})

	result, err := handler(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	text := resultText(t, result)

	var res map[string]any

	err = json.Unmarshal([]byte(text), &res)
	if err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	// Subject stays "Fwd: Already forwarded", no double "Fwd: Fwd:"
	if smtpMock.lastSendReq.Subject != "Fwd: Already forwarded" {
		t.Errorf("Subject = %q, want %q", smtpMock.lastSendReq.Subject, "Fwd: Already forwarded")
	}
}

func TestForwardHandler_WithSingleAttachment(t *testing.T) {
	t.Parallel()

	imapMock := &mockIMAPOps{
		defaultAccountID: "acct1",
		getMessageResult: &models.Email{
			ID: "acct1:INBOX:42", Subject: "With attachment",
			From: "alice@example.com", To: []string{"me@example.com"},
			Date: "2026-02-22T10:00:00Z", Body: "See attached.", Mailbox: "INBOX",
		},
		getAttachmentsResult: []models.AttachmentInfo{
			{Index: 0, Filename: "report.pdf", ContentType: "application/pdf", Size: 1024},
		},
		getAttachmentData:     []byte("PDF-content"),
		getAttachmentFilename: "report.pdf",
	}

	smtpMock := &mockSMTPOps{
		defaultAccountID: "acct1",
		accountEmails:    map[string]string{"acct1": "me@example.com"},
	}

	handler := tools.ForwardHandler(imapMock, smtpMock, testLimits)

	result, err := handler(context.Background(), toolReq(map[string]any{
		"id": "acct1:INBOX:42", "to": "bob@example.com",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	assertSuccess(t, result)

	if len(smtpMock.lastSendReq.Attachments) != 1 {
		t.Fatalf("Attachments count = %d, want 1", len(smtpMock.lastSendReq.Attachments))
	}

	att := smtpMock.lastSendReq.Attachments[0]
	if att.Filename != "report.pdf" {
		t.Errorf("Attachment filename = %q, want %q", att.Filename, "report.pdf")
	}

	if string(att.Data) != "PDF-content" {
		t.Errorf("Attachment data = %q, want %q", string(att.Data), "PDF-content")
	}
}

func TestForwardHandler_WithMultipleAttachments(t *testing.T) {
	t.Parallel()

	imapMock := &mockIMAPOps{
		defaultAccountID: "acct1",
		getMessageResult: &models.Email{
			ID: "acct1:INBOX:42", Subject: "Multi-attach",
			From: "alice@example.com", To: []string{"me@example.com"},
			Date: "2026-02-22T10:00:00Z", Body: "Files attached.", Mailbox: "INBOX",
		},
		getAttachmentsResult: []models.AttachmentInfo{
			{Index: 0, Filename: "a.txt", ContentType: "text/plain", Size: 100},
			{Index: 1, Filename: "b.txt", ContentType: "text/plain", Size: 200},
			{Index: 2, Filename: "c.txt", ContentType: "text/plain", Size: 300},
		},
		getAttachmentByIndex: map[int]struct {
			Data     []byte
			Filename string
		}{
			0: {Data: []byte("aaa"), Filename: "a.txt"},
			1: {Data: []byte("bbb"), Filename: "b.txt"},
			2: {Data: []byte("ccc"), Filename: "c.txt"},
		},
	}

	smtpMock := &mockSMTPOps{
		defaultAccountID: "acct1",
		accountEmails:    map[string]string{"acct1": "me@example.com"},
	}

	handler := tools.ForwardHandler(imapMock, smtpMock, testLimits)

	result, err := handler(context.Background(), toolReq(map[string]any{
		"id": "acct1:INBOX:42", "to": "bob@example.com",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	assertSuccess(t, result)

	if len(smtpMock.lastSendReq.Attachments) != 3 {
		t.Fatalf("Attachments count = %d, want 3", len(smtpMock.lastSendReq.Attachments))
	}

	names := []string{
		smtpMock.lastSendReq.Attachments[0].Filename,
		smtpMock.lastSendReq.Attachments[1].Filename,
		smtpMock.lastSendReq.Attachments[2].Filename,
	}
	if names[0] != "a.txt" || names[1] != "b.txt" || names[2] != "c.txt" {
		t.Errorf("Attachment filenames = %v, want [a.txt b.txt c.txt]", names)
	}
}

func TestForwardHandler_NoAttachments(t *testing.T) {
	t.Parallel()

	imapMock := &mockIMAPOps{
		defaultAccountID: "acct1",
		getMessageResult: &models.Email{
			ID: "acct1:INBOX:42", Subject: "Plain text",
			From: "alice@example.com", To: []string{"me@example.com"},
			Date: "2026-02-22T10:00:00Z", Body: "No attachments here.", Mailbox: "INBOX",
		},
		// getAttachmentsResult defaults to nil — no attachments
	}

	smtpMock := &mockSMTPOps{
		defaultAccountID: "acct1",
		accountEmails:    map[string]string{"acct1": "me@example.com"},
	}

	handler := tools.ForwardHandler(imapMock, smtpMock, testLimits)

	result, err := handler(context.Background(), toolReq(map[string]any{
		"id": "acct1:INBOX:42", "to": "bob@example.com",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	assertSuccess(t, result)

	if len(smtpMock.lastSendReq.Attachments) != 0 {
		t.Errorf("Attachments count = %d, want 0", len(smtpMock.lastSendReq.Attachments))
	}

	// Body should not contain dropped-attachment note
	if strings.Contains(smtpMock.lastSendReq.Body, "[Note:") {
		t.Errorf("body should not contain dropped note when no attachments")
	}
}

func TestForwardHandler_OversizedAttachments(t *testing.T) {
	t.Parallel()

	// 18 MB limit; first attachment is 10 MB, second is 10 MB → second should be dropped.
	const tenMB = 10 * 1024 * 1024

	imapMock := &mockIMAPOps{
		defaultAccountID: "acct1",
		getMessageResult: &models.Email{
			ID: "acct1:INBOX:42", Subject: "Big files",
			From: "alice@example.com", To: []string{"me@example.com"},
			Date: "2026-02-22T10:00:00Z", Body: "Large attachments.", Mailbox: "INBOX",
		},
		getAttachmentsResult: []models.AttachmentInfo{
			{Index: 0, Filename: "small.bin", ContentType: "application/octet-stream", Size: tenMB},
			{Index: 1, Filename: "big.bin", ContentType: "application/octet-stream", Size: tenMB},
		},
		getAttachmentByIndex: map[int]struct {
			Data     []byte
			Filename string
		}{
			0: {Data: make([]byte, tenMB), Filename: "small.bin"},
		},
	}

	smtpMock := &mockSMTPOps{
		defaultAccountID: "acct1",
		accountEmails:    map[string]string{"acct1": "me@example.com"},
	}

	handler := tools.ForwardHandler(imapMock, smtpMock, testLimits)

	result, err := handler(context.Background(), toolReq(map[string]any{
		"id": "acct1:INBOX:42", "to": "bob@example.com",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	assertSuccess(t, result)

	// Only the first attachment should be included.
	if len(smtpMock.lastSendReq.Attachments) != 1 {
		t.Fatalf("Attachments count = %d, want 1", len(smtpMock.lastSendReq.Attachments))
	}

	if smtpMock.lastSendReq.Attachments[0].Filename != "small.bin" {
		t.Errorf("Attachment = %q, want small.bin", smtpMock.lastSendReq.Attachments[0].Filename)
	}

	// Body should contain dropped note
	if !strings.Contains(smtpMock.lastSendReq.Body, "[Note:") {
		t.Error("body should contain dropped-attachment note")
	}

	if !strings.Contains(smtpMock.lastSendReq.Body, "big.bin") {
		t.Error("dropped note should mention big.bin")
	}
}

func TestForwardHandler_AttachmentFetchError(t *testing.T) {
	t.Parallel()

	imapMock := &mockIMAPOps{
		defaultAccountID: "acct1",
		getMessageResult: &models.Email{
			ID: "acct1:INBOX:42", Subject: "Fetch fail",
			From: "alice@example.com", To: []string{"me@example.com"},
			Date: "2026-02-22T10:00:00Z", Body: "Error on fetch.", Mailbox: "INBOX",
		},
		getAttachmentsResult: []models.AttachmentInfo{
			{Index: 0, Filename: "broken.zip", ContentType: "application/zip", Size: 500},
		},
		getAttachmentErr: errConnectionReset,
	}

	smtpMock := &mockSMTPOps{
		defaultAccountID: "acct1",
		accountEmails:    map[string]string{"acct1": "me@example.com"},
	}

	handler := tools.ForwardHandler(imapMock, smtpMock, testLimits)

	result, err := handler(context.Background(), toolReq(map[string]any{
		"id": "acct1:INBOX:42", "to": "bob@example.com",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Forward should still succeed (graceful degradation)
	assertSuccess(t, result)

	// No attachments included
	if len(smtpMock.lastSendReq.Attachments) != 0 {
		t.Errorf("Attachments count = %d, want 0 (fetch failed)", len(smtpMock.lastSendReq.Attachments))
	}

	// Body should note the failed attachment
	if !strings.Contains(smtpMock.lastSendReq.Body, "broken.zip") {
		t.Error("body should mention broken.zip in dropped note")
	}

	if !strings.Contains(smtpMock.lastSendReq.Body, "fetch error") {
		t.Error("body should mention fetch error")
	}
}

// assertSuccess verifies the handler result is {success: true}.
func assertSuccess(t *testing.T, result *mcp.CallToolResult) {
	t.Helper()

	text := resultText(t, result)

	var res map[string]any

	err := json.Unmarshal([]byte(text), &res)
	if err != nil {
		t.Fatalf("invalid JSON: %v (%s)", err, text)
	}

	if res["success"] != true {
		t.Fatalf("expected success=true, got %v", res["success"])
	}
}
