package tools_test

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/boutquin/mcp-server-email/internal/models"
	"github.com/boutquin/mcp-server-email/internal/tools"
)

// --- Send handler tests ---

func TestSendHandler_HappyPath(t *testing.T) {
	t.Parallel()

	smtpMock := &mockSMTPOps{
		defaultAccountID: testDefaultAccount,
		accountEmails:    map[string]string{testDefaultAccount: "hello@example.com"},
	}

	handler := tools.SendHandler(smtpMock, testLimits)

	result, err := handler(context.Background(), toolReq(map[string]any{
		"to":      "recipient@example.com",
		"subject": "Test Subject",
		"body":    "Test Body",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.IsError {
		t.Fatal("expected success result")
	}

	if !smtpMock.sendCalled {
		t.Fatal("mock Send() was not called")
	}

	if smtpMock.lastSendAccountID != testDefaultAccount {
		t.Errorf("expected account 'default', got %q", smtpMock.lastSendAccountID)
	}

	req := smtpMock.lastSendReq
	if len(req.To) != 1 || req.To[0] != "recipient@example.com" {
		t.Errorf("unexpected To: %v", req.To)
	}

	if req.Subject != "Test Subject" {
		t.Errorf("expected subject 'Test Subject', got %q", req.Subject)
	}

	if req.Body != "Test Body" {
		t.Errorf("expected body 'Test Body', got %q", req.Body)
	}

	// Check response includes success and from.
	text := resultText(t, result)

	var resp struct {
		Success bool   `json:"success"`
		From    string `json:"from"`
	}

	err = json.Unmarshal([]byte(text), &resp)
	if err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}

	if !resp.Success {
		t.Error("expected success=true in response")
	}

	if resp.From != "hello@example.com" {
		t.Errorf("expected from 'hello@example.com', got %q", resp.From)
	}
}

func TestSendHandler_WithCCBCCReplyTo(t *testing.T) {
	t.Parallel()

	smtpMock := &mockSMTPOps{
		defaultAccountID: testDefaultAccount,
		accountEmails:    map[string]string{testDefaultAccount: "hello@example.com"},
	}

	handler := tools.SendHandler(smtpMock, testLimits)

	result, err := handler(context.Background(), toolReq(map[string]any{
		"to":      "to@example.com",
		"subject": "Subject",
		"body":    "Body",
		"cc":      "cc1@example.com, cc2@example.com",
		"bcc":     "bcc@example.com",
		"replyTo": "reply@example.com",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.IsError {
		t.Fatal("expected success result")
	}

	req := smtpMock.lastSendReq
	if len(req.CC) != 2 {
		t.Errorf("expected 2 CC addresses, got %d", len(req.CC))
	}

	if len(req.BCC) != 1 || req.BCC[0] != "bcc@example.com" {
		t.Errorf("unexpected BCC: %v", req.BCC)
	}

	if req.ReplyTo != "reply@example.com" {
		t.Errorf("expected replyTo 'reply@example.com', got %q", req.ReplyTo)
	}
}

func TestSendHandler_IsHTML(t *testing.T) {
	t.Parallel()

	smtpMock := &mockSMTPOps{
		defaultAccountID: testDefaultAccount,
		accountEmails:    map[string]string{testDefaultAccount: "hello@example.com"},
	}

	handler := tools.SendHandler(smtpMock, testLimits)

	result, err := handler(context.Background(), toolReq(map[string]any{
		"to":      "to@example.com",
		"subject": "Subject",
		"body":    "<h1>HTML</h1>",
		"isHtml":  true,
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.IsError {
		t.Fatal("expected success result")
	}

	if !smtpMock.lastSendReq.IsHTML {
		t.Error("expected IsHTML=true")
	}
}

func TestSendHandler_MissingTo(t *testing.T) {
	t.Parallel()

	smtpMock := &mockSMTPOps{defaultAccountID: testDefaultAccount}

	handler := tools.SendHandler(smtpMock, testLimits)

	result, err := handler(context.Background(), toolReq(map[string]any{
		"subject": "Subject",
		"body":    "Body",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !result.IsError {
		t.Fatal("expected error result for missing 'to'")
	}

	text := resultText(t, result)
	if !strings.Contains(text, "to is required") {
		t.Errorf("expected 'to is required' error, got %q", text)
	}
}

func TestSendHandler_MissingSubject(t *testing.T) {
	t.Parallel()

	smtpMock := &mockSMTPOps{defaultAccountID: testDefaultAccount}

	handler := tools.SendHandler(smtpMock, testLimits)

	result, err := handler(context.Background(), toolReq(map[string]any{
		"to":   "to@example.com",
		"body": "Body",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !result.IsError {
		t.Fatal("expected error result for missing 'subject'")
	}

	text := resultText(t, result)
	if !strings.Contains(text, "subject is required") {
		t.Errorf("expected 'subject is required' error, got %q", text)
	}
}

func TestSendHandler_MissingBody(t *testing.T) {
	t.Parallel()

	smtpMock := &mockSMTPOps{defaultAccountID: testDefaultAccount}

	handler := tools.SendHandler(smtpMock, testLimits)

	result, err := handler(context.Background(), toolReq(map[string]any{
		"to":      "to@example.com",
		"subject": "Subject",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !result.IsError {
		t.Fatal("expected error result for missing 'body'")
	}

	text := resultText(t, result)
	if !strings.Contains(text, "body is required") {
		t.Errorf("expected 'body is required' error, got %q", text)
	}
}

// --- DraftCreate handler tests ---

func TestDraftCreateHandler_HappyPath(t *testing.T) {
	t.Parallel()

	imapMock := &mockIMAPOps{
		defaultAccountID: testDefaultAccount,
		saveDraftUID:     123,
	}

	smtpMock := &mockSMTPOps{
		defaultAccountID: testDefaultAccount,
		accountEmails:    map[string]string{testDefaultAccount: "hello@example.com"},
	}

	handler := tools.DraftCreateHandler(imapMock, smtpMock, testLimits)

	result, err := handler(context.Background(), toolReq(map[string]any{
		"to":      "to@example.com",
		"subject": "Draft Subject",
		"body":    "Draft Body",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.IsError {
		text := resultText(t, result)
		t.Fatalf("expected success result, got error: %s", text)
	}

	if imapMock.lastSaveDraftArgs.accountID != testDefaultAccount {
		t.Errorf("expected account 'default', got %q", imapMock.lastSaveDraftArgs.accountID)
	}

	if len(imapMock.lastSaveDraftArgs.msg) == 0 {
		t.Error("expected non-empty message literal")
	}

	text := resultText(t, result)

	var resp struct {
		Success bool   `json:"success"`
		ID      string `json:"id"`
	}

	err = json.Unmarshal([]byte(text), &resp)
	if err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}

	if !resp.Success {
		t.Error("expected success=true")
	}

	// ID should be formatted as account:Drafts:uid.
	if !strings.Contains(resp.ID, "default:Drafts:123") {
		t.Errorf("expected draft ID containing 'default:Drafts:123', got %q", resp.ID)
	}
}

func TestDraftCreateHandler_WithIsHTML(t *testing.T) {
	t.Parallel()

	imapMock := &mockIMAPOps{
		defaultAccountID: testDefaultAccount,
		saveDraftUID:     456,
	}

	smtpMock := &mockSMTPOps{
		defaultAccountID: testDefaultAccount,
		accountEmails:    map[string]string{testDefaultAccount: "hello@example.com"},
	}

	handler := tools.DraftCreateHandler(imapMock, smtpMock, testLimits)

	result, err := handler(context.Background(), toolReq(map[string]any{
		"body":   "<p>HTML draft</p>",
		"isHtml": true,
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.IsError {
		text := resultText(t, result)
		t.Fatalf("expected success result, got error: %s", text)
	}

	// The message literal should contain HTML content-type.
	msg := string(imapMock.lastSaveDraftArgs.msg)
	if !strings.Contains(msg, "text/html") {
		t.Error("expected HTML content-type in message literal")
	}
}

// --- DraftSend handler tests ---

func TestDraftSendHandler_HappyPath(t *testing.T) {
	t.Parallel()

	imapMock := &mockIMAPOps{
		getMessageResult: &models.Email{
			ID:      "hello:Drafts:10",
			Subject: "Draft to Send",
			To:      []string{"recipient@example.com"},
			Body:    "Hello from draft",
		},
	}

	smtpMock := &mockSMTPOps{
		accountEmails: map[string]string{testAccountHello: "hello@example.com"},
	}

	handler := tools.DraftSendHandler(imapMock, smtpMock)

	result, err := handler(context.Background(), toolReq(map[string]any{
		"id": "hello:Drafts:10",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.IsError {
		text := resultText(t, result)
		t.Fatalf("expected success result, got error: %s", text)
	}

	// Verify mock was called to get the draft.
	if imapMock.lastGetMessageArgs.accountID != testAccountHello {
		t.Errorf("expected GetMessage for account 'hello', got %q", imapMock.lastGetMessageArgs.accountID)
	}

	if imapMock.lastGetMessageArgs.folder != "Drafts" {
		t.Errorf("expected GetMessage for folder 'Drafts', got %q", imapMock.lastGetMessageArgs.folder)
	}

	// Verify SMTP send was called.
	if !smtpMock.sendCalled {
		t.Fatal("mock Send() was not called")
	}

	if smtpMock.lastSendReq.Subject != "Draft to Send" {
		t.Errorf("expected subject 'Draft to Send', got %q", smtpMock.lastSendReq.Subject)
	}

	// Verify delete draft was called.
	if imapMock.lastDeleteDraftArgs.accountID != testAccountHello || imapMock.lastDeleteDraftArgs.uid != 10 {
		t.Errorf("unexpected delete draft args: %+v", imapMock.lastDeleteDraftArgs)
	}
}

func TestDraftSendHandler_InvalidID(t *testing.T) {
	t.Parallel()

	imapMock := &mockIMAPOps{}
	smtpMock := &mockSMTPOps{}

	handler := tools.DraftSendHandler(imapMock, smtpMock)

	result, err := handler(context.Background(), toolReq(map[string]any{
		"id": "bad-id",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !result.IsError {
		t.Fatal("expected error result for invalid ID")
	}
}

func TestDraftSendHandler_NotADraft(t *testing.T) {
	t.Parallel()

	imapMock := &mockIMAPOps{}
	smtpMock := &mockSMTPOps{}

	handler := tools.DraftSendHandler(imapMock, smtpMock)

	result, err := handler(context.Background(), toolReq(map[string]any{
		"id": "hello:INBOX:10",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !result.IsError {
		t.Fatal("expected error result for non-draft message")
	}

	text := resultText(t, result)
	if !strings.Contains(text, "not a draft") {
		t.Errorf("expected 'not a draft' error, got %q", text)
	}
}

func TestDraftSendHandler_DraftNotFound(t *testing.T) {
	t.Parallel()

	imapMock := &mockIMAPOps{
		getMessageErr: models.ErrMessageNotFound,
	}

	smtpMock := &mockSMTPOps{}

	handler := tools.DraftSendHandler(imapMock, smtpMock)

	result, err := handler(context.Background(), toolReq(map[string]any{
		"id": "hello:Drafts:999",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !result.IsError {
		t.Fatal("expected error result for draft not found")
	}

	text := resultText(t, result)
	if !strings.Contains(text, "message not found") {
		t.Errorf("expected 'message not found' error, got %q", text)
	}
}

func TestDraftSendHandler_HTMLDetection(t *testing.T) {
	t.Parallel()

	// Simulate a draft that was created with HTML content.
	// GetDraft returns the raw RFC 5322 message with Content-Type: text/html.
	htmlDraft := []byte("From: hello@example.com\r\n" +
		"To: recipient@example.com\r\n" +
		"Subject: HTML Draft\r\n" +
		"Content-Type: text/html; charset=utf-8\r\n" +
		"\r\n" +
		"<h1>Hello</h1>")

	imapMock := &mockIMAPOps{
		getMessageResult: &models.Email{
			ID:      "hello:Drafts:20",
			Subject: "HTML Draft",
			To:      []string{"recipient@example.com"},
			Body:    "<h1>Hello</h1>",
		},
		getDraftResult: htmlDraft,
	}

	smtpMock := &mockSMTPOps{
		accountEmails: map[string]string{testAccountHello: "hello@example.com"},
	}

	handler := tools.DraftSendHandler(imapMock, smtpMock)

	result, err := handler(context.Background(), toolReq(map[string]any{
		"id": "hello:Drafts:20",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.IsError {
		text := resultText(t, result)
		t.Fatalf("expected success result, got error: %s", text)
	}

	// The key assertion: Send must be called with IsHTML=true because the
	// draft's Content-Type is text/html.
	if !smtpMock.sendCalled {
		t.Fatal("mock Send() was not called")
	}

	if !smtpMock.lastSendReq.IsHTML {
		t.Error("expected IsHTML=true when draft Content-Type is text/html")
	}
}

func TestDraftSendHandler_PlainTextDetection(t *testing.T) {
	t.Parallel()

	// Simulate a draft that was created with plain text content.
	plainDraft := []byte("From: hello@example.com\r\n" +
		"To: recipient@example.com\r\n" +
		"Subject: Plain Draft\r\n" +
		"Content-Type: text/plain; charset=utf-8\r\n" +
		"\r\n" +
		"Hello, world!")

	imapMock := &mockIMAPOps{
		getMessageResult: &models.Email{
			ID:      "hello:Drafts:30",
			Subject: "Plain Draft",
			To:      []string{"recipient@example.com"},
			Body:    "Hello, world!",
		},
		getDraftResult: plainDraft,
	}

	smtpMock := &mockSMTPOps{
		accountEmails: map[string]string{testAccountHello: "hello@example.com"},
	}

	handler := tools.DraftSendHandler(imapMock, smtpMock)

	result, err := handler(context.Background(), toolReq(map[string]any{
		"id": "hello:Drafts:30",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.IsError {
		text := resultText(t, result)
		t.Fatalf("expected success result, got error: %s", text)
	}

	if !smtpMock.sendCalled {
		t.Fatal("mock Send() was not called")
	}

	if smtpMock.lastSendReq.IsHTML {
		t.Error("expected IsHTML=false when draft Content-Type is text/plain")
	}
}

// --- Send with attachments tests ---

func TestSendHandler_WithAttachments(t *testing.T) {
	t.Parallel()

	// Create a real temp file for attachment.
	dir := t.TempDir()
	attachPath := dir + "/report.pdf"

	err := os.WriteFile(attachPath, []byte("fake pdf content"), 0o644)
	if err != nil {
		t.Fatalf("write temp file: %v", err)
	}

	smtpMock := &mockSMTPOps{
		defaultAccountID: testDefaultAccount,
		accountEmails:    map[string]string{testDefaultAccount: "hello@example.com"},
	}

	handler := tools.SendHandler(smtpMock, testLimits)

	result, err := handler(context.Background(), toolReq(map[string]any{
		"to":      "recipient@example.com",
		"subject": "Report",
		"body":    "See attached.",
		"attachments": []any{
			map[string]any{"path": attachPath},
		},
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.IsError {
		text := resultText(t, result)
		t.Fatalf("expected success result, got error: %s", text)
	}

	if !smtpMock.sendCalled {
		t.Fatal("mock Send() was not called")
	}

	req := smtpMock.lastSendReq
	if len(req.Attachments) != 1 {
		t.Fatalf("expected 1 attachment, got %d", len(req.Attachments))
	}

	if req.Attachments[0].Path != attachPath {
		t.Errorf("expected attachment path %q, got %q", attachPath, req.Attachments[0].Path)
	}

	if req.Attachments[0].Filename != "report.pdf" {
		t.Errorf("expected filename 'report.pdf', got %q", req.Attachments[0].Filename)
	}
}

func TestSendHandler_AttachmentTooLarge(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	attachPath := dir + "/huge.bin"

	f, fErr := os.Create(attachPath) //nolint:gosec // test file in temp dir
	if fErr != nil {
		t.Fatalf("create temp file: %v", fErr)
	}

	// Sparse file just over 18 MB.
	truncErr := f.Truncate(18*1024*1024 + 1)
	if truncErr != nil {
		_ = f.Close()

		t.Fatalf("truncate: %v", truncErr)
	}

	_ = f.Close()

	smtpMock := &mockSMTPOps{
		defaultAccountID: testDefaultAccount,
		accountEmails:    map[string]string{testDefaultAccount: "hello@example.com"},
	}

	handler := tools.SendHandler(smtpMock, testLimits)

	result, err := handler(context.Background(), toolReq(map[string]any{
		"to":      "to@example.com",
		"subject": "Subject",
		"body":    "Body",
		"attachments": []any{
			map[string]any{"path": attachPath},
		},
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !result.IsError {
		t.Fatal("expected error result for too-large attachment")
	}

	text := resultText(t, result)
	if !strings.Contains(text, "18 MB") {
		t.Errorf("expected '18 MB' in error, got %q", text)
	}
}

func TestSendHandler_AttachmentNotFound(t *testing.T) {
	t.Parallel()

	smtpMock := &mockSMTPOps{
		defaultAccountID: testDefaultAccount,
		accountEmails:    map[string]string{testDefaultAccount: "hello@example.com"},
	}

	handler := tools.SendHandler(smtpMock, testLimits)

	result, err := handler(context.Background(), toolReq(map[string]any{
		"to":      "to@example.com",
		"subject": "Subject",
		"body":    "Body",
		"attachments": []any{
			map[string]any{"path": "/tmp/nonexistent-attachment-file.txt"},
		},
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !result.IsError {
		t.Fatal("expected error result for non-existent attachment")
	}

	text := resultText(t, result)
	if !strings.Contains(text, "file not found") {
		t.Errorf("expected 'file not found' in error, got %q", text)
	}
}

// --- DraftCreate with attachments tests ---

func TestDraftCreateHandler_WithAttachments(t *testing.T) {
	t.Parallel()

	// Create a real temp file for attachment.
	dir := t.TempDir()
	attachPath := dir + "/notes.txt"

	err := os.WriteFile(attachPath, []byte("attachment content"), 0o644)
	if err != nil {
		t.Fatalf("write temp file: %v", err)
	}

	imapMock := &mockIMAPOps{
		defaultAccountID: testDefaultAccount,
		saveDraftUID:     789,
	}

	smtpMock := &mockSMTPOps{
		defaultAccountID: testDefaultAccount,
		accountEmails:    map[string]string{testDefaultAccount: "hello@example.com"},
	}

	handler := tools.DraftCreateHandler(imapMock, smtpMock, testLimits)

	result, err := handler(context.Background(), toolReq(map[string]any{
		"to":      "to@example.com",
		"subject": "Draft with Attachment",
		"body":    "See attached.",
		"attachments": []any{
			map[string]any{"path": attachPath, "filename": "my-notes.txt"},
		},
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.IsError {
		text := resultText(t, result)
		t.Fatalf("expected success result, got error: %s", text)
	}

	// Verify the draft was saved.
	if imapMock.lastSaveDraftArgs.accountID != testDefaultAccount {
		t.Errorf("expected account %q, got %q", testDefaultAccount, imapMock.lastSaveDraftArgs.accountID)
	}

	// The saved message should contain the attachment filename.
	msg := string(imapMock.lastSaveDraftArgs.msg)
	if !strings.Contains(msg, "my-notes.txt") {
		t.Error("expected draft message to contain attachment filename 'my-notes.txt'")
	}

	// The saved message should contain the attachment content (base64 encoded).
	if !strings.Contains(msg, "multipart") && !strings.Contains(msg, "YXR0YWNobWVudCBjb250ZW50") {
		t.Error("expected draft message to contain attachment content or multipart structure")
	}

	// Verify response.
	text := resultText(t, result)

	var resp struct {
		Success bool   `json:"success"`
		ID      string `json:"id"`
	}

	err = json.Unmarshal([]byte(text), &resp)
	if err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}

	if !resp.Success {
		t.Error("expected success=true")
	}

	if !strings.Contains(resp.ID, "default:Drafts:789") {
		t.Errorf("expected draft ID containing 'default:Drafts:789', got %q", resp.ID)
	}
}
