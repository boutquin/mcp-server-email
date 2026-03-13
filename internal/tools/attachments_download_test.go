package tools_test

import (
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/boutquin/mcp-server-email/internal/models"
	"github.com/boutquin/mcp-server-email/internal/tools"
)

func TestAttachmentListTool(t *testing.T) {
	t.Parallel()

	tool := tools.AttachmentListTool()

	if tool.Name != "email_attachment_list" {
		t.Errorf("expected tool name 'email_attachment_list', got %q", tool.Name)
	}
}

func TestAttachmentGetTool(t *testing.T) {
	t.Parallel()

	tool := tools.AttachmentGetTool()

	if tool.Name != "email_attachment_get" {
		t.Errorf("expected tool name 'email_attachment_get', got %q", tool.Name)
	}
}

func TestAttachmentListHandler_SingleAttachment(t *testing.T) {
	t.Parallel()

	mock := &mockIMAPOps{
		defaultAccountID: "test",
		getAttachmentsResult: []models.AttachmentInfo{
			{Index: 0, Filename: "report.pdf", ContentType: "application/pdf", Size: 12345},
		},
	}

	handler := tools.AttachmentListHandler(mock)

	result, err := handler(t.Context(), toolReq(map[string]any{
		"id": "test:INBOX:42",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	text := resultText(t, result)

	if !strings.Contains(text, "report.pdf") {
		t.Error("expected filename in result")
	}

	if !strings.Contains(text, "application/pdf") {
		t.Error("expected content type in result")
	}

	if !strings.Contains(text, "12345") {
		t.Error("expected size in result")
	}
}

func TestAttachmentListHandler_MultipleAttachments(t *testing.T) {
	t.Parallel()

	mock := &mockIMAPOps{
		defaultAccountID: "test",
		getAttachmentsResult: []models.AttachmentInfo{
			{Index: 0, Filename: "a.pdf", ContentType: "application/pdf", Size: 100},
			{Index: 1, Filename: "b.txt", ContentType: "text/plain", Size: 200},
			{Index: 2, Filename: "c.png", ContentType: "image/png", Size: 300},
		},
	}

	handler := tools.AttachmentListHandler(mock)

	result, err := handler(t.Context(), toolReq(map[string]any{
		"id": "test:INBOX:42",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	text := resultText(t, result)

	var attachments []models.AttachmentInfo

	jsonErr := json.Unmarshal([]byte(text), &attachments)
	if jsonErr != nil {
		t.Fatalf("failed to parse result: %v", jsonErr)
	}

	if len(attachments) != 3 {
		t.Errorf("expected 3 attachments, got %d", len(attachments))
	}
}

func TestAttachmentListHandler_NoAttachments(t *testing.T) {
	t.Parallel()

	mock := &mockIMAPOps{
		defaultAccountID:     "test",
		getAttachmentsResult: nil,
	}

	handler := tools.AttachmentListHandler(mock)

	result, err := handler(t.Context(), toolReq(map[string]any{
		"id": "test:INBOX:42",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	text := resultText(t, result)

	if !strings.Contains(text, "null") && !strings.Contains(text, "[]") {
		t.Error("expected null or empty array for no attachments")
	}
}

func TestAttachmentGetHandler_ReturnsContent(t *testing.T) {
	t.Parallel()

	content := []byte("Hello, attachment content!")

	mock := &mockIMAPOps{
		defaultAccountID:      "test",
		getAttachmentData:     content,
		getAttachmentFilename: "report.pdf",
	}

	handler := tools.AttachmentGetHandler(mock, testLimits)

	result, err := handler(t.Context(), toolReq(map[string]any{
		"id":    "test:INBOX:42",
		"index": float64(0),
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	text := resultText(t, result)

	expected := base64.StdEncoding.EncodeToString(content)
	if !strings.Contains(text, expected) {
		t.Error("expected base64-encoded content in result")
	}

	if !strings.Contains(text, "report.pdf") {
		t.Error("expected filename in result")
	}
}

func TestAttachmentGetHandler_SaveToFile(t *testing.T) {
	t.Parallel()

	content := []byte("saved attachment content")
	dir := t.TempDir()
	savePath := filepath.Join(dir, "downloaded.pdf")

	mock := &mockIMAPOps{
		defaultAccountID:      "test",
		getAttachmentData:     content,
		getAttachmentFilename: "report.pdf",
	}

	handler := tools.AttachmentGetHandler(mock, testLimits)

	result, err := handler(t.Context(), toolReq(map[string]any{
		"id":     "test:INBOX:42",
		"index":  float64(0),
		"saveTo": savePath,
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	text := resultText(t, result)

	if !strings.Contains(text, savePath) {
		t.Error("expected save path in result")
	}

	// Verify file was written.
	data, readErr := os.ReadFile(savePath) //nolint:gosec // test reads from temp dir
	if readErr != nil {
		t.Fatalf("failed to read saved file: %v", readErr)
	}

	if string(data) != string(content) {
		t.Errorf("saved content mismatch: got %q, want %q", data, content)
	}
}

func TestAttachmentGetHandler_OversizedError(t *testing.T) {
	t.Parallel()

	// Create data larger than 25 MB.
	oversized := make([]byte, 26*1024*1024)

	mock := &mockIMAPOps{
		defaultAccountID:      "test",
		getAttachmentData:     oversized,
		getAttachmentFilename: "big.bin",
	}

	handler := tools.AttachmentGetHandler(mock, testLimits)

	result, err := handler(t.Context(), toolReq(map[string]any{
		"id":    "test:INBOX:42",
		"index": float64(0),
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !result.IsError {
		t.Error("expected error result for oversized attachment")
	}

	text := resultText(t, result)

	if !strings.Contains(text, "25 MB") {
		t.Error("expected size limit error message")
	}
}

func TestAttachmentGetHandler_InvalidIndex(t *testing.T) {
	t.Parallel()

	mock := &mockIMAPOps{
		defaultAccountID: "test",
		getAttachmentErr: models.ErrMessageNotFound,
	}

	handler := tools.AttachmentGetHandler(mock, testLimits)

	result, err := handler(t.Context(), toolReq(map[string]any{
		"id":    "test:INBOX:42",
		"index": float64(99),
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !result.IsError {
		t.Error("expected error result for invalid index")
	}
}

func TestAttachmentGetHandler_PathTraversal(t *testing.T) {
	t.Parallel()

	mock := &mockIMAPOps{
		defaultAccountID:      "test",
		getAttachmentData:     []byte("data"),
		getAttachmentFilename: "file.txt",
	}

	handler := tools.AttachmentGetHandler(mock, testLimits)

	result, err := handler(t.Context(), toolReq(map[string]any{
		"id":     "test:INBOX:42",
		"index":  float64(0),
		"saveTo": "/tmp/../etc/passwd",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !result.IsError {
		t.Error("expected error result for path traversal")
	}

	text := resultText(t, result)

	if !strings.Contains(text, "traversal") {
		t.Error("expected traversal error message")
	}
}

func TestAttachmentGetHandler_RelativePath(t *testing.T) {
	t.Parallel()

	mock := &mockIMAPOps{
		defaultAccountID:      "test",
		getAttachmentData:     []byte("data"),
		getAttachmentFilename: "file.txt",
	}

	handler := tools.AttachmentGetHandler(mock, testLimits)

	result, err := handler(t.Context(), toolReq(map[string]any{
		"id":     "test:INBOX:42",
		"index":  float64(0),
		"saveTo": "relative/path/file.txt",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !result.IsError {
		t.Error("expected error result for relative path")
	}
}

func TestAttachmentGetHandler_MissingIndex(t *testing.T) {
	t.Parallel()

	mock := &mockIMAPOps{
		defaultAccountID: "test",
	}

	handler := tools.AttachmentGetHandler(mock, testLimits)

	result, err := handler(t.Context(), toolReq(map[string]any{
		"id": "test:INBOX:42",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !result.IsError {
		t.Error("expected error result for missing index")
	}
}
