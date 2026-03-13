package tools_test

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/boutquin/mcp-server-email/internal/models"
	"github.com/boutquin/mcp-server-email/internal/tools"
)

// errOps is a sentinel error for handler operation failures.
var errOps = fmt.Errorf("op failed: %w", models.ErrAccountNotFound)

// ==================== formatBytes ====================

func TestFormatBytes_KB(t *testing.T) {
	t.Parallel()

	got := tools.FormatBytes(512 * 1024)
	if got != "512.0 KB" {
		t.Errorf("FormatBytes(512KB) = %q, want %q", got, "512.0 KB")
	}
}

func TestFormatBytes_MB(t *testing.T) {
	t.Parallel()

	got := tools.FormatBytes(5 * 1024 * 1024)
	if got != "5.0 MB" {
		t.Errorf("FormatBytes(5MB) = %q, want %q", got, "5.0 MB")
	}
}

func TestFormatBytes_Zero(t *testing.T) {
	t.Parallel()

	got := tools.FormatBytes(0)
	if got != "0.0 KB" {
		t.Errorf("FormatBytes(0) = %q, want %q", got, "0.0 KB")
	}
}

func TestFormatBytes_SmallBytes(t *testing.T) {
	t.Parallel()

	got := tools.FormatBytes(100)
	if !strings.Contains(got, "KB") {
		t.Errorf("FormatBytes(100) = %q, expected KB suffix", got)
	}
}

func TestFormatBytes_ExactMB(t *testing.T) {
	t.Parallel()

	got := tools.FormatBytes(1024 * 1024)
	if got != "1.0 MB" {
		t.Errorf("FormatBytes(1MB) = %q, want %q", got, "1.0 MB")
	}
}

// ==================== appendDroppedNote ====================

func TestAppendDroppedNote_EmptyNote(t *testing.T) {
	t.Parallel()

	got := tools.AppendDroppedNote("", "dropped attachment X")
	if got != "dropped attachment X" {
		t.Errorf("got %q, want %q", got, "dropped attachment X")
	}
}

func TestAppendDroppedNote_NonEmptyNote(t *testing.T) {
	t.Parallel()

	got := tools.AppendDroppedNote("existing note", "dropped attachment X")

	want := "existing note\n\ndropped attachment X"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// ==================== Handler IMAP operation errors ====================

func TestMoveHandler_MoveError(t *testing.T) {
	t.Parallel()

	mock := &mockIMAPOps{moveMessageErr: errOps}
	handler := tools.MoveHandler(mock)

	result, err := handler(context.Background(), toolReq(map[string]any{
		"id":          "hello:INBOX:10",
		"destination": "Archive",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !result.IsError {
		t.Fatal("expected error result for MoveMessage failure")
	}

	text := resultText(t, result)
	if !strings.Contains(text, "account not found") {
		t.Errorf("expected error message, got %q", text)
	}
}

func TestCopyHandler_CopyError(t *testing.T) {
	t.Parallel()

	mock := &mockIMAPOps{copyMessageErr: errOps}
	handler := tools.CopyHandler(mock)

	result, err := handler(context.Background(), toolReq(map[string]any{
		"id":          "hello:INBOX:10",
		"destination": "Backup",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !result.IsError {
		t.Fatal("expected error result for CopyMessage failure")
	}

	text := resultText(t, result)
	if !strings.Contains(text, "account not found") {
		t.Errorf("expected error message, got %q", text)
	}
}

func TestDeleteHandler_DeleteError(t *testing.T) {
	t.Parallel()

	mock := &mockIMAPOps{deleteMessageErr: errOps}
	handler := tools.DeleteHandler(mock)

	result, err := handler(context.Background(), toolReq(map[string]any{
		"id": "hello:INBOX:10",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !result.IsError {
		t.Fatal("expected error result for DeleteMessage failure")
	}

	text := resultText(t, result)
	if !strings.Contains(text, "account not found") {
		t.Errorf("expected error message, got %q", text)
	}
}

func TestMarkReadHandler_MarkReadError(t *testing.T) {
	t.Parallel()

	mock := &mockIMAPOps{markReadErr: errOps}
	handler := tools.MarkReadHandler(mock)

	result, err := handler(context.Background(), toolReq(map[string]any{
		"id":   "hello:INBOX:10",
		"read": true,
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !result.IsError {
		t.Fatal("expected error result for MarkRead failure")
	}

	text := resultText(t, result)
	if !strings.Contains(text, "account not found") {
		t.Errorf("expected error message, got %q", text)
	}
}

func TestFlagHandler_SetFlagError(t *testing.T) {
	t.Parallel()

	mock := &mockIMAPOps{setFlagErr: errOps}
	handler := tools.FlagHandler(mock)

	result, err := handler(context.Background(), toolReq(map[string]any{
		"id":      "hello:INBOX:10",
		"flagged": true,
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !result.IsError {
		t.Fatal("expected error result for SetFlag failure")
	}

	text := resultText(t, result)
	if !strings.Contains(text, "account not found") {
		t.Errorf("expected error message, got %q", text)
	}
}

// ==================== SendHandler error paths ====================

func TestSendHandler_SendError(t *testing.T) {
	t.Parallel()

	smtpMock := &mockSMTPOps{
		defaultAccountID: testDefaultAccount,
		accountEmails:    map[string]string{testDefaultAccount: "hello@example.com"},
		sendErr:          errOps,
	}

	handler := tools.SendHandler(smtpMock, testLimits)

	result, err := handler(context.Background(), toolReq(map[string]any{
		"to":      "to@example.com",
		"subject": "Subject",
		"body":    "Body",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !result.IsError {
		t.Fatal("expected error result for Send failure")
	}

	text := resultText(t, result)
	if !strings.Contains(text, "account not found") {
		t.Errorf("expected error message, got %q", text)
	}
}

func TestSendHandler_AccountEmailError(t *testing.T) {
	t.Parallel()

	smtpMock := &mockSMTPOps{
		defaultAccountID: testDefaultAccount,
		accountEmailErr:  errOps,
	}

	handler := tools.SendHandler(smtpMock, testLimits)

	result, err := handler(context.Background(), toolReq(map[string]any{
		"to":      "to@example.com",
		"subject": "Subject",
		"body":    "Body",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !result.IsError {
		t.Fatal("expected error result for AccountEmail failure")
	}

	text := resultText(t, result)
	if !strings.Contains(text, "account not found") {
		t.Errorf("expected error message, got %q", text)
	}
}

// ==================== DraftCreateHandler error paths ====================

func TestDraftCreateHandler_AccountEmailError(t *testing.T) {
	t.Parallel()

	imapMock := &mockIMAPOps{defaultAccountID: testDefaultAccount}
	smtpMock := &mockSMTPOps{
		defaultAccountID: testDefaultAccount,
		accountEmailErr:  errOps,
	}

	handler := tools.DraftCreateHandler(imapMock, smtpMock, testLimits)

	result, err := handler(context.Background(), toolReq(map[string]any{
		"body": "Draft body",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !result.IsError {
		t.Fatal("expected error result for AccountEmail failure")
	}

	text := resultText(t, result)
	if !strings.Contains(text, "account not found") {
		t.Errorf("expected error message, got %q", text)
	}
}

func TestDraftCreateHandler_SaveDraftError(t *testing.T) {
	t.Parallel()

	imapMock := &mockIMAPOps{
		defaultAccountID: testDefaultAccount,
		saveDraftErr:     errOps,
	}
	smtpMock := &mockSMTPOps{
		defaultAccountID: testDefaultAccount,
		accountEmails:    map[string]string{testDefaultAccount: "hello@example.com"},
	}

	handler := tools.DraftCreateHandler(imapMock, smtpMock, testLimits)

	result, err := handler(context.Background(), toolReq(map[string]any{
		"body": "Draft body",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !result.IsError {
		t.Fatal("expected error result for SaveDraft failure")
	}

	text := resultText(t, result)
	if !strings.Contains(text, "account not found") {
		t.Errorf("expected error message, got %q", text)
	}
}

func TestDraftCreateHandler_WithCCAndBCC(t *testing.T) {
	t.Parallel()

	imapMock := &mockIMAPOps{
		defaultAccountID: testDefaultAccount,
		saveDraftUID:     500,
	}
	smtpMock := &mockSMTPOps{
		defaultAccountID: testDefaultAccount,
		accountEmails:    map[string]string{testDefaultAccount: "hello@example.com"},
	}

	handler := tools.DraftCreateHandler(imapMock, smtpMock, testLimits)

	result, err := handler(context.Background(), toolReq(map[string]any{
		"to":      "to@example.com",
		"subject": "Subject",
		"body":    "Body",
		"cc":      "cc@example.com",
		"bcc":     "bcc@example.com",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.IsError {
		text := resultText(t, result)
		t.Fatalf("expected success, got error: %s", text)
	}
}

func TestDraftCreateHandler_InvalidAttachment(t *testing.T) {
	t.Parallel()

	imapMock := &mockIMAPOps{defaultAccountID: testDefaultAccount}
	smtpMock := &mockSMTPOps{
		defaultAccountID: testDefaultAccount,
		accountEmails:    map[string]string{testDefaultAccount: "hello@example.com"},
	}

	handler := tools.DraftCreateHandler(imapMock, smtpMock, testLimits)

	result, err := handler(context.Background(), toolReq(map[string]any{
		"body": "Body",
		"attachments": []any{
			map[string]any{"path": "/tmp/nonexistent-p36-test-file.txt"},
		},
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !result.IsError {
		t.Fatal("expected error result for invalid attachment")
	}
}

// ==================== AttachmentListHandler error path ====================

func TestAttachmentListHandler_GetAttachmentsError(t *testing.T) {
	t.Parallel()

	mock := &mockIMAPOps{
		defaultAccountID:  "test",
		getAttachmentsErr: errOps,
	}

	handler := tools.AttachmentListHandler(mock)

	result, err := handler(context.Background(), toolReq(map[string]any{
		"id": "test:INBOX:42",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !result.IsError {
		t.Fatal("expected error result for GetAttachments failure")
	}

	text := resultText(t, result)
	if !strings.Contains(text, "account not found") {
		t.Errorf("expected error message, got %q", text)
	}
}

func TestAttachmentListHandler_InvalidID(t *testing.T) {
	t.Parallel()

	mock := &mockIMAPOps{defaultAccountID: "test"}
	handler := tools.AttachmentListHandler(mock)

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

// ==================== ensureTargetIncluded edge cases ====================

func TestEnsureTargetIncluded_AlreadyPresent(t *testing.T) {
	t.Parallel()

	target := &models.Email{
		ID:              "test:INBOX:1",
		MessageIDHeader: "<msg1@ex.com>",
	}

	thread := []models.Email{
		{ID: "test:INBOX:1", MessageIDHeader: "<msg1@ex.com>"},
		{ID: "test:INBOX:2", MessageIDHeader: "<msg2@ex.com>"},
	}

	result := tools.EnsureTargetIncluded(thread, target)
	if len(result) != 2 {
		t.Errorf("expected 2 messages (target already present), got %d", len(result))
	}
}

func TestEnsureTargetIncluded_NotPresent(t *testing.T) {
	t.Parallel()

	target := &models.Email{
		ID:              "test:INBOX:3",
		MessageIDHeader: "<msg3@ex.com>",
	}

	thread := []models.Email{
		{ID: "test:INBOX:1", MessageIDHeader: "<msg1@ex.com>"},
		{ID: "test:INBOX:2", MessageIDHeader: "<msg2@ex.com>"},
	}

	result := tools.EnsureTargetIncluded(thread, target)
	if len(result) != 3 {
		t.Errorf("expected 3 messages (target appended), got %d", len(result))
	}
}

func TestEnsureTargetIncluded_FallbackToID(t *testing.T) {
	t.Parallel()

	// Target has no MessageIDHeader — should fall back to ID for dedup.
	target := &models.Email{
		ID:              "test:INBOX:1",
		MessageIDHeader: "",
	}

	thread := []models.Email{
		{ID: "test:INBOX:1", MessageIDHeader: ""},
	}

	result := tools.EnsureTargetIncluded(thread, target)
	if len(result) != 1 {
		t.Errorf("expected 1 message (deduped by ID fallback), got %d", len(result))
	}
}

func TestEnsureTargetIncluded_EmptyThread(t *testing.T) {
	t.Parallel()

	target := &models.Email{
		ID:              "test:INBOX:1",
		MessageIDHeader: "<msg1@ex.com>",
	}

	result := tools.EnsureTargetIncluded(nil, target)
	if len(result) != 1 {
		t.Errorf("expected 1 message (appended to empty), got %d", len(result))
	}
}

// ==================== ListHandler error path ====================

func TestListHandler_ListMessagesError(t *testing.T) {
	t.Parallel()

	mock := &mockIMAPOps{
		defaultAccountID: testDefaultAccount,
		listMessagesErr:  errOps,
	}

	handler := tools.ListHandler(mock)

	result, err := handler(context.Background(), toolReq(nil))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !result.IsError {
		t.Fatal("expected error result for ListMessages failure")
	}
}

// ==================== UnreadHandler error path ====================

func TestUnreadHandler_ListUnreadError(t *testing.T) {
	t.Parallel()

	mock := &mockIMAPOps{
		defaultAccountID: testDefaultAccount,
		listUnreadErr:    errOps,
	}

	handler := tools.UnreadHandler(mock)

	result, err := handler(context.Background(), toolReq(nil))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !result.IsError {
		t.Fatal("expected error result for ListUnread failure")
	}
}

// ==================== SearchHandler error path ====================

func TestSearchHandler_SearchError(t *testing.T) {
	t.Parallel()

	mock := &mockIMAPOps{
		defaultAccountID: testDefaultAccount,
		searchErr:        errOps,
	}

	handler := tools.SearchHandler(mock)

	result, err := handler(context.Background(), toolReq(map[string]any{
		"query": "test",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !result.IsError {
		t.Fatal("expected error result for Search failure")
	}
}
