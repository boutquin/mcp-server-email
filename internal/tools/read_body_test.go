package tools_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/boutquin/mcp-server-email/internal/models"
	"github.com/boutquin/mcp-server-email/internal/tools"
)

// readBodyResult holds the JSON structure returned by ReadBodyHandler.
type readBodyResult struct {
	Content     string `json:"content"`
	TotalLength int    `json:"total_length"`
	Offset      int    `json:"offset"`
	Limit       int    `json:"limit"`
	Remaining   int    `json:"remaining"`
	IsComplete  bool   `json:"is_complete"`
}

func parseReadBodyResult(t *testing.T, result string) readBodyResult {
	t.Helper()

	var r readBodyResult

	err := json.Unmarshal([]byte(result), &r)
	if err != nil {
		t.Fatalf("failed to parse read body result: %v", err)
	}

	return r
}

func TestReadBody_FullContent(t *testing.T) {
	t.Parallel()

	body := "Short email body"

	mock := &mockIMAPOps{
		getMessageResult: &models.Email{
			ID:          "hello:INBOX:42",
			Body:        body,
			ContentType: "text/plain",
		},
	}

	handler := tools.ReadBodyHandler(mock)

	result, err := handler(context.Background(), toolReq(map[string]any{
		"id": "hello:INBOX:42",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.IsError {
		t.Fatal("expected success result")
	}

	text := resultText(t, result)
	r := parseReadBodyResult(t, text)

	if r.Content != body {
		t.Errorf("expected content %q, got %q", body, r.Content)
	}

	if r.TotalLength != len(body) {
		t.Errorf("expected total_length %d, got %d", len(body), r.TotalLength)
	}

	if r.Remaining != 0 {
		t.Errorf("expected remaining 0, got %d", r.Remaining)
	}

	if !r.IsComplete {
		t.Error("expected is_complete=true")
	}
}

func TestReadBody_Pagination(t *testing.T) {
	t.Parallel()

	body := strings.Repeat("A", 500)

	mock := &mockIMAPOps{
		getMessageResult: &models.Email{
			ID:          "hello:INBOX:42",
			Body:        body,
			ContentType: "text/plain",
		},
	}

	handler := tools.ReadBodyHandler(mock)

	result, err := handler(context.Background(), toolReq(map[string]any{
		"id":    "hello:INBOX:42",
		"limit": float64(100),
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	text := resultText(t, result)
	r := parseReadBodyResult(t, text)

	if len(r.Content) != 100 {
		t.Errorf("expected content length 100, got %d", len(r.Content))
	}

	if r.TotalLength != 500 {
		t.Errorf("expected total_length 500, got %d", r.TotalLength)
	}

	if r.Remaining != 400 {
		t.Errorf("expected remaining 400, got %d", r.Remaining)
	}

	if r.IsComplete {
		t.Error("expected is_complete=false")
	}
}

func TestReadBody_Offset(t *testing.T) {
	t.Parallel()

	body := "ABCDEFGHIJKLMNOPQRSTUVWXYZ"

	mock := &mockIMAPOps{
		getMessageResult: &models.Email{
			ID:          "hello:INBOX:42",
			Body:        body,
			ContentType: "text/plain",
		},
	}

	handler := tools.ReadBodyHandler(mock)

	result, err := handler(context.Background(), toolReq(map[string]any{
		"id":     "hello:INBOX:42",
		"offset": float64(10),
		"limit":  float64(5),
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	text := resultText(t, result)
	r := parseReadBodyResult(t, text)

	if r.Content != "KLMNO" {
		t.Errorf("expected content 'KLMNO', got %q", r.Content)
	}

	if r.Offset != 10 {
		t.Errorf("expected offset 10, got %d", r.Offset)
	}

	if r.Remaining != 11 {
		t.Errorf("expected remaining 11, got %d", r.Remaining)
	}
}

func TestReadBody_HTMLToText(t *testing.T) {
	t.Parallel()

	mock := &mockIMAPOps{
		getMessageResult: &models.Email{
			ID:          "hello:INBOX:42",
			Body:        "<h1>Hello</h1><p>World</p>",
			ContentType: "text/html",
		},
	}

	handler := tools.ReadBodyHandler(mock)

	result, err := handler(context.Background(), toolReq(map[string]any{
		"id":     "hello:INBOX:42",
		"format": "text",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	text := resultText(t, result)
	r := parseReadBodyResult(t, text)

	if strings.Contains(r.Content, "<h1>") {
		t.Errorf("expected HTML tags stripped, got %q", r.Content)
	}

	if !strings.Contains(r.Content, "Hello") || !strings.Contains(r.Content, "World") {
		t.Errorf("expected text content preserved, got %q", r.Content)
	}
}

func TestReadBody_RawHTML(t *testing.T) {
	t.Parallel()

	htmlBody := "<h1>Hello</h1><p>World</p>"

	mock := &mockIMAPOps{
		getMessageResult: &models.Email{
			ID:          "hello:INBOX:42",
			Body:        htmlBody,
			ContentType: "text/html",
		},
	}

	handler := tools.ReadBodyHandler(mock)

	result, err := handler(context.Background(), toolReq(map[string]any{
		"id":     "hello:INBOX:42",
		"format": "raw_html",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	text := resultText(t, result)
	r := parseReadBodyResult(t, text)

	if !strings.Contains(r.Content, "<h1>") {
		t.Errorf("expected raw HTML preserved, got %q", r.Content)
	}
}

func TestReadBody_InvalidID(t *testing.T) {
	t.Parallel()

	mock := &mockIMAPOps{}

	handler := tools.ReadBodyHandler(mock)

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

func TestReadBody_MissingID(t *testing.T) {
	t.Parallel()

	mock := &mockIMAPOps{}

	handler := tools.ReadBodyHandler(mock)

	result, err := handler(context.Background(), toolReq(nil))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !result.IsError {
		t.Fatal("expected error result for missing ID")
	}
}

// --- GetHandler preview tests ---

type getPreviewResult struct {
	BodyPreview     string `json:"body_preview"`
	BodyTotalLength int    `json:"body_total_length"`
	Body            string `json:"body"`
}

func TestGet_Preview(t *testing.T) {
	t.Parallel()

	body := strings.Repeat("X", 1000)

	mock := &mockIMAPOps{
		getMessageResult: &models.Email{
			ID:          "hello:INBOX:42",
			Subject:     "Test",
			Body:        body,
			ContentType: "text/plain",
		},
	}

	handler := tools.GetHandler(mock)

	result, err := handler(context.Background(), toolReq(map[string]any{
		"id": "hello:INBOX:42",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	text := resultText(t, result)

	var r getPreviewResult

	err = json.Unmarshal([]byte(text), &r)
	if err != nil {
		t.Fatalf("failed to parse: %v", err)
	}

	if len(r.BodyPreview) != 500 {
		t.Errorf("expected preview length 500, got %d", len(r.BodyPreview))
	}

	if r.BodyTotalLength != 1000 {
		t.Errorf("expected body_total_length 1000, got %d", r.BodyTotalLength)
	}
}

func TestGet_PreviewCustomLength(t *testing.T) {
	t.Parallel()

	body := strings.Repeat("Y", 200)

	mock := &mockIMAPOps{
		getMessageResult: &models.Email{
			ID:          "hello:INBOX:42",
			Subject:     "Test",
			Body:        body,
			ContentType: "text/plain",
		},
	}

	handler := tools.GetHandler(mock)

	result, err := handler(context.Background(), toolReq(map[string]any{
		"id":            "hello:INBOX:42",
		"previewLength": float64(50),
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	text := resultText(t, result)

	var r getPreviewResult

	err = json.Unmarshal([]byte(text), &r)
	if err != nil {
		t.Fatalf("failed to parse: %v", err)
	}

	if len(r.BodyPreview) != 50 {
		t.Errorf("expected preview length 50, got %d", len(r.BodyPreview))
	}
}

func TestGet_PreviewDisabled(t *testing.T) {
	t.Parallel()

	mock := &mockIMAPOps{
		getMessageResult: &models.Email{
			ID:          "hello:INBOX:42",
			Subject:     "Test",
			Body:        "Some body",
			ContentType: "text/plain",
		},
	}

	handler := tools.GetHandler(mock)

	result, err := handler(context.Background(), toolReq(map[string]any{
		"id":            "hello:INBOX:42",
		"previewLength": float64(0),
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	text := resultText(t, result)

	if strings.Contains(text, "body_preview") {
		t.Error("expected no body_preview when previewLength=0")
	}
}

func TestGet_PreviewNoBody(t *testing.T) {
	t.Parallel()

	mock := &mockIMAPOps{
		getMessageResult: &models.Email{
			ID:          "hello:INBOX:42",
			Subject:     "Test",
			ContentType: "text/plain",
		},
	}

	handler := tools.GetHandler(mock)

	result, err := handler(context.Background(), toolReq(map[string]any{
		"id": "hello:INBOX:42",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	text := resultText(t, result)

	if strings.Contains(text, "body_preview") {
		t.Error("expected no body_preview when body is empty")
	}
}
