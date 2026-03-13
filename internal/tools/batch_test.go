package tools_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/boutquin/mcp-server-email/internal/models"
	"github.com/boutquin/mcp-server-email/internal/tools"
)

const testBadID = "bad-id"

var errMailboxNotFound = errors.New("mailbox not found")

// batchResponse holds the JSON structure returned by BatchHandler.
type batchResponse struct {
	Results []batchItemResult `json:"results"`
	Summary batchSummary      `json:"summary"`
}

type batchItemResult struct {
	ID      string `json:"id"`
	Success bool   `json:"success"`
	Error   string `json:"error,omitempty"`
}

type batchSummary struct {
	Total     int `json:"total"`
	Succeeded int `json:"succeeded"`
	Failed    int `json:"failed"`
}

func parseBatchResponse(t *testing.T, text string) batchResponse {
	t.Helper()

	var r batchResponse

	err := json.Unmarshal([]byte(text), &r)
	if err != nil {
		t.Fatalf("failed to parse batch response: %v", err)
	}

	return r
}

func batchReq(args map[string]any) map[string]any {
	return args
}

func TestBatch_MoveSuccess(t *testing.T) {
	t.Parallel()

	mock := &mockIMAPOps{}
	handler := tools.BatchHandler(mock)

	result, err := handler(context.Background(), toolReq(batchReq(map[string]any{
		"action":      "move",
		"ids":         []any{"hello:INBOX:1", "hello:INBOX:2", "hello:INBOX:3"},
		"destination": "Archive",
	})))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.IsError {
		t.Fatal("expected success result")
	}

	text := resultText(t, result)
	r := parseBatchResponse(t, text)

	if r.Summary.Total != 3 {
		t.Errorf("expected total 3, got %d", r.Summary.Total)
	}

	if r.Summary.Succeeded != 3 {
		t.Errorf("expected succeeded 3, got %d", r.Summary.Succeeded)
	}

	if r.Summary.Failed != 0 {
		t.Errorf("expected failed 0, got %d", r.Summary.Failed)
	}

	for _, item := range r.Results {
		if !item.Success {
			t.Errorf("expected success for %s", item.ID)
		}
	}
}

func TestBatch_DeleteSuccess(t *testing.T) {
	t.Parallel()

	mock := &mockIMAPOps{}
	handler := tools.BatchHandler(mock)

	result, err := handler(context.Background(), toolReq(batchReq(map[string]any{
		"action":    "delete",
		"ids":       []any{"hello:INBOX:1", "hello:INBOX:2"},
		"permanent": true,
	})))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	text := resultText(t, result)
	r := parseBatchResponse(t, text)

	if r.Summary.Total != 2 || r.Summary.Succeeded != 2 {
		t.Errorf("expected 2/2 succeeded, got %d/%d", r.Summary.Succeeded, r.Summary.Total)
	}
}

func TestBatch_PartialFailure(t *testing.T) {
	t.Parallel()

	mock := &mockIMAPOps{
		moveMessageErr: errMailboxNotFound,
	}

	handler := tools.BatchHandler(mock)

	// All will fail since mock returns same error for all calls.
	// Test with 1 valid + 1 invalid ID to get partial failure via parse error.
	result, err := handler(context.Background(), toolReq(batchReq(map[string]any{
		"action":      "move",
		"ids":         []any{"hello:INBOX:1", testBadID, "hello:INBOX:3"},
		"destination": "Archive",
	})))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	text := resultText(t, result)
	r := parseBatchResponse(t, text)

	assertBatchPartialFailure(t, r)
}

func assertBatchPartialFailure(t *testing.T, r batchResponse) {
	t.Helper()

	if r.Summary.Total != 3 {
		t.Errorf("expected total 3, got %d", r.Summary.Total)
	}

	if r.Summary.Failed == 0 {
		t.Error("expected at least one failure")
	}

	// The bad-id should have failed
	for _, item := range r.Results {
		if item.ID == testBadID && item.Success {
			t.Error("expected bad-id to fail")
		}

		if item.ID == testBadID && item.Error == "" {
			t.Error("expected error message for bad-id")
		}
	}
}

func TestBatch_MarkReadSuccess(t *testing.T) {
	t.Parallel()

	mock := &mockIMAPOps{}
	handler := tools.BatchHandler(mock)

	result, err := handler(context.Background(), toolReq(batchReq(map[string]any{
		"action": "mark_read",
		"ids":    []any{"hello:INBOX:1", "hello:INBOX:2"},
		"read":   true,
	})))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	text := resultText(t, result)
	r := parseBatchResponse(t, text)

	if r.Summary.Total != 2 || r.Summary.Succeeded != 2 {
		t.Errorf("expected 2/2 succeeded, got %d/%d", r.Summary.Succeeded, r.Summary.Total)
	}
}

func TestBatch_FlagSuccess(t *testing.T) {
	t.Parallel()

	mock := &mockIMAPOps{}
	handler := tools.BatchHandler(mock)

	result, err := handler(context.Background(), toolReq(batchReq(map[string]any{
		"action":  "flag",
		"ids":     []any{"hello:INBOX:1", "hello:INBOX:2"},
		"flagged": true,
	})))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	text := resultText(t, result)
	r := parseBatchResponse(t, text)

	if r.Summary.Total != 2 || r.Summary.Succeeded != 2 {
		t.Errorf("expected 2/2 succeeded, got %d/%d", r.Summary.Succeeded, r.Summary.Total)
	}
}

func TestBatch_InvalidAction(t *testing.T) {
	t.Parallel()

	mock := &mockIMAPOps{}
	handler := tools.BatchHandler(mock)

	result, err := handler(context.Background(), toolReq(batchReq(map[string]any{
		"action": "archive",
		"ids":    []any{"hello:INBOX:1"},
	})))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !result.IsError {
		t.Fatal("expected error result for invalid action")
	}

	text := resultText(t, result)
	if text == "" {
		t.Fatal("expected error message")
	}
}

func TestBatch_MissingDestination(t *testing.T) {
	t.Parallel()

	mock := &mockIMAPOps{}
	handler := tools.BatchHandler(mock)

	result, err := handler(context.Background(), toolReq(batchReq(map[string]any{
		"action": "move",
		"ids":    []any{"hello:INBOX:1"},
	})))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !result.IsError {
		t.Fatal("expected error result for missing destination")
	}
}

func TestBatch_EmptyIDs(t *testing.T) {
	t.Parallel()

	mock := &mockIMAPOps{}
	handler := tools.BatchHandler(mock)

	result, err := handler(context.Background(), toolReq(batchReq(map[string]any{
		"action": "delete",
		"ids":    []any{},
	})))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !result.IsError {
		t.Fatal("expected error result for empty IDs")
	}
}

func TestBatch_InvalidID(t *testing.T) {
	t.Parallel()

	mock := &mockIMAPOps{}
	handler := tools.BatchHandler(mock)

	result, err := handler(context.Background(), toolReq(batchReq(map[string]any{
		"action":    "delete",
		"ids":       []any{"hello:INBOX:1", testBadID, "hello:INBOX:3"},
		"permanent": false,
	})))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.IsError {
		t.Fatal("expected success result (batch should not fail entirely)")
	}

	text := resultText(t, result)
	r := parseBatchResponse(t, text)

	if r.Summary.Succeeded != 2 {
		t.Errorf("expected 2 succeeded, got %d", r.Summary.Succeeded)
	}

	if r.Summary.Failed != 1 {
		t.Errorf("expected 1 failed, got %d", r.Summary.Failed)
	}

	assertInvalidIDFailed(t, r)
}

func assertInvalidIDFailed(t *testing.T, r batchResponse) {
	t.Helper()

	for _, item := range r.Results {
		if item.ID == testBadID {
			if item.Success {
				t.Error("expected bad-id to fail")
			}

			if item.Error == "" {
				t.Error("expected error message for bad-id")
			}

			return
		}
	}

	t.Error("bad-id not found in results")
}

func TestBatch_MissingAction(t *testing.T) {
	t.Parallel()

	mock := &mockIMAPOps{}
	handler := tools.BatchHandler(mock)

	result, err := handler(context.Background(), toolReq(batchReq(map[string]any{
		"ids": []any{"hello:INBOX:1"},
	})))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !result.IsError {
		t.Fatal("expected error result for missing action")
	}
}

func TestBatch_PartialFailureWithMockError(t *testing.T) {
	t.Parallel()

	// Use a mock that fails on MoveMessage to get mixed results with valid IDs + one invalid ID.
	mock := &mockIMAPOps{}
	handler := tools.BatchHandler(mock)

	// Mix valid and invalid IDs — invalid ID fails at parse, valid IDs succeed.
	result, err := handler(context.Background(), toolReq(batchReq(map[string]any{
		"action":  "flag",
		"ids":     []any{"hello:INBOX:1", "nope", "hello:INBOX:3"},
		"flagged": true,
	})))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	text := resultText(t, result)
	r := parseBatchResponse(t, text)

	if r.Summary.Succeeded != 2 {
		t.Errorf("expected 2 succeeded, got %d", r.Summary.Succeeded)
	}

	if r.Summary.Failed != 1 {
		t.Errorf("expected 1 failed, got %d", r.Summary.Failed)
	}

	// Verify batch continued past the failure.
	foundLast := false

	for _, item := range r.Results {
		if item.ID == "hello:INBOX:3" {
			foundLast = true

			if !item.Success {
				t.Error("expected hello:INBOX:3 to succeed (batch should continue)")
			}
		}
	}

	if !foundLast {
		t.Error("hello:INBOX:3 not found in results — batch may have aborted early")
	}
}

func TestBatch_MixedTypeIDs(t *testing.T) {
	t.Parallel()

	mock := &mockIMAPOps{}
	handler := tools.BatchHandler(mock)

	// Mix string and non-string types in ids array.
	// Non-string items (42, true) should be silently skipped.
	result, err := handler(context.Background(), toolReq(batchReq(map[string]any{
		"action":  "flag",
		"ids":     []any{"hello:INBOX:1", 42, true, "hello:INBOX:2"},
		"flagged": true,
	})))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	text := resultText(t, result)
	r := parseBatchResponse(t, text)

	// Only the two string IDs should be processed.
	if r.Summary.Total != 2 {
		t.Errorf("expected total 2 (non-string items skipped), got %d", r.Summary.Total)
	}

	if r.Summary.Succeeded != 2 {
		t.Errorf("expected 2 succeeded, got %d", r.Summary.Succeeded)
	}
}

func TestBatch_NonArrayIDs(t *testing.T) {
	t.Parallel()

	mock := &mockIMAPOps{}
	handler := tools.BatchHandler(mock)

	// Pass ids as a string instead of an array — extractStringArray returns nil.
	result, err := handler(context.Background(), toolReq(batchReq(map[string]any{
		"action": "delete",
		"ids":    "not-an-array",
	})))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !result.IsError {
		t.Fatal("expected error result when ids is not an array")
	}
}

func TestBatch_DeleteOperationError(t *testing.T) {
	t.Parallel()

	mock := &mockIMAPOps{
		deleteMessageErr: models.ErrMessageNotFound,
	}

	handler := tools.BatchHandler(mock)

	result, err := handler(context.Background(), toolReq(batchReq(map[string]any{
		"action": "delete",
		"ids":    []any{"hello:INBOX:1", "hello:INBOX:2"},
	})))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	text := resultText(t, result)
	r := parseBatchResponse(t, text)

	if r.Summary.Failed != 2 {
		t.Errorf("expected 2 failed, got %d", r.Summary.Failed)
	}

	if r.Summary.Succeeded != 0 {
		t.Errorf("expected 0 succeeded, got %d", r.Summary.Succeeded)
	}
}
