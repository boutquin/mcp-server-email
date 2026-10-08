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

	var saved struct {
		Saved string `json:"saved"`
	}
	if err := json.Unmarshal([]byte(text), &saved); err != nil {
		t.Fatalf("failed to parse save result: %v", err)
	}
	if saved.Saved != savePath {
		t.Errorf("saved path = %q, want %q", saved.Saved, savePath)
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

func TestAttachmentGetHandler_DoesNotOverwriteExistingFile(t *testing.T) {
	dir := t.TempDir()
	savePath := filepath.Join(dir, "existing.txt")
	if err := os.WriteFile(savePath, []byte("original"), 0o600); err != nil {
		t.Fatal(err)
	}
	mock := &mockIMAPOps{defaultAccountID: "test", getAttachmentData: []byte("new"), getAttachmentFilename: "file.txt"}
	result, err := tools.AttachmentGetHandler(mock, testLimits)(t.Context(), toolReq(map[string]any{
		"id": "test:INBOX:42", "index": float64(0), "saveTo": savePath,
	}))
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError {
		t.Fatal("expected existing target to be rejected")
	}
	got, err := os.ReadFile(savePath)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "original" {
		t.Fatalf("existing file changed: %q", got)
	}
}

func TestValidateSavePath_RejectsSymlinkParent(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "real")
	if err := os.Mkdir(target, 0o700); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "link")
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	mock := &mockIMAPOps{defaultAccountID: "test", getAttachmentData: []byte("data"), getAttachmentFilename: "file.txt"}
	result, err := tools.AttachmentGetHandler(mock, testLimits)(t.Context(), toolReq(map[string]any{
		"id": "test:INBOX:42", "index": float64(0), "saveTo": filepath.Join(link, "out.txt"),
	}))
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError {
		t.Fatal("expected symlink parent to be rejected")
	}
}

func TestAttachmentGetHandler_AllowsSaveToOutsideWorkingDirectory(t *testing.T) {
	outside := t.TempDir()
	savePath := filepath.Join(outside, "not-allowed.txt")
	mock := &mockIMAPOps{defaultAccountID: "test", getAttachmentData: []byte("data"), getAttachmentFilename: "file.txt"}
	result, err := tools.AttachmentGetHandler(mock, testLimits)(t.Context(), toolReq(map[string]any{
		"id": "test:INBOX:42", "index": float64(0), "saveTo": savePath,
	}))
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatalf("expected save outside working directory to succeed: %s", resultText(t, result))
	}
	if _, err := os.Stat(savePath); err != nil {
		t.Fatalf("expected target file to be saved: %v", err)
	}
}

func TestAttachmentGetHandler_RejectsPortableTraversalSeparators(t *testing.T) {
	dir := t.TempDir()
	mock := &mockIMAPOps{defaultAccountID: "test", getAttachmentData: []byte("data"), getAttachmentFilename: "file.txt"}
	for _, savePath := range []string{
		dir + "/child/../escape.txt",
		dir + `\\child\\..\\escape.txt`,
	} {
		result, err := tools.AttachmentGetHandler(mock, testLimits)(t.Context(), toolReq(map[string]any{
			"id": "test:INBOX:42", "index": float64(0), "saveTo": savePath,
		}))
		if err != nil {
			t.Fatal(err)
		}
		if !result.IsError || !strings.Contains(resultText(t, result), "traversal") {
			t.Fatalf("expected traversal rejection for %q", savePath)
		}
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
	traversalPath := filepath.Join(t.TempDir(), "child") + string(filepath.Separator) + ".." +
		string(filepath.Separator) + "etc" + string(filepath.Separator) + "passwd"

	mock := &mockIMAPOps{
		defaultAccountID:      "test",
		getAttachmentData:     []byte("data"),
		getAttachmentFilename: "file.txt",
	}

	handler := tools.AttachmentGetHandler(mock, testLimits)

	result, err := handler(t.Context(), toolReq(map[string]any{
		"id":     "test:INBOX:42",
		"index":  float64(0),
		"saveTo": traversalPath,
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
