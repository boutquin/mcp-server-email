package tools_test

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/boutquin/mcp-server-email/internal/models"
	"github.com/boutquin/mcp-server-email/internal/tools"
)

func TestSearchHandler_QueryOnly(t *testing.T) {
	t.Parallel()

	mock := &mockIMAPOps{
		defaultAccountID: testDefaultAccount,
		searchResult: []models.Email{
			{ID: "default:INBOX:1", Subject: "Match"},
		},
	}

	handler := tools.SearchHandler(mock)

	result, err := handler(context.Background(), toolReq(map[string]any{
		"query": "test search",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.IsError {
		t.Fatal("expected success result")
	}

	args := mock.lastSearchArgs
	if args.query != "test search" {
		t.Errorf("expected query 'test search', got %q", args.query)
	}

	if args.accountID != testDefaultAccount {
		t.Errorf("expected default account, got %q", args.accountID)
	}

	if args.mailbox != testInbox {
		t.Errorf("expected default mailbox 'INBOX', got %q", args.mailbox)
	}

	text := resultText(t, result)

	var resp struct {
		Count    int            `json:"count"`
		Query    string         `json:"query"`
		Messages []models.Email `json:"messages"`
	}

	err = json.Unmarshal([]byte(text), &resp)
	if err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}

	if resp.Count != 1 {
		t.Errorf("expected count 1, got %d", resp.Count)
	}

	if resp.Query != "test search" {
		t.Errorf("expected query in response, got %q", resp.Query)
	}
}

func TestSearchHandler_AllFilters(t *testing.T) {
	t.Parallel()

	mock := &mockIMAPOps{
		defaultAccountID: testDefaultAccount,
		searchResult:     []models.Email{},
	}

	handler := tools.SearchHandler(mock)

	_, err := handler(context.Background(), toolReq(map[string]any{
		"query":       "important",
		"account":     testAccountHello,
		"folder":      testSent,
		"from":        "sender@example.com",
		"to":          "recipient@example.com",
		"since":       "2024-01-01",
		"before":      "2024-12-31",
		"limit":       float64(25),
		"includeBody": true,
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	args := mock.lastSearchArgs
	if args.accountID != testAccountHello {
		t.Errorf("expected account 'hello', got %q", args.accountID)
	}

	if args.mailbox != testSent {
		t.Errorf("expected mailbox 'Sent', got %q", args.mailbox)
	}

	if args.query != "important" {
		t.Errorf("expected query 'important', got %q", args.query)
	}

	if args.from != "sender@example.com" {
		t.Errorf("expected from 'sender@example.com', got %q", args.from)
	}

	if args.to != "recipient@example.com" {
		t.Errorf("expected to 'recipient@example.com', got %q", args.to)
	}

	if args.since != "2024-01-01" {
		t.Errorf("expected since '2024-01-01', got %q", args.since)
	}

	if args.before != "2024-12-31" {
		t.Errorf("expected before '2024-12-31', got %q", args.before)
	}

	if args.limit != 26 {
		t.Errorf("expected requested page plus one lookahead result (26), got %d", args.limit)
	}

	if !args.includeBody {
		t.Error("expected includeBody=true")
	}
}

func TestSearchHandler_MissingQuery(t *testing.T) {
	t.Parallel()

	mock := &mockIMAPOps{defaultAccountID: testDefaultAccount}

	handler := tools.SearchHandler(mock)

	result, err := handler(context.Background(), toolReq(nil))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !result.IsError {
		t.Fatal("expected error result for missing query")
	}

	text := resultText(t, result)
	if !strings.Contains(text, "query is required") {
		t.Errorf("expected 'query is required' error, got %q", text)
	}
}

func TestSearchHandler_InvalidSinceDate(t *testing.T) {
	t.Parallel()

	mock := &mockIMAPOps{defaultAccountID: testDefaultAccount}

	handler := tools.SearchHandler(mock)

	result, err := handler(context.Background(), toolReq(map[string]any{
		"query": "test",
		"since": "not-a-date",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !result.IsError {
		t.Fatal("expected error result for invalid since date")
	}

	text := resultText(t, result)
	if !strings.Contains(text, "invalid 'since' date") {
		t.Errorf("expected invalid since date error, got %q", text)
	}
}

func TestSearchHandler_InvalidBeforeDate(t *testing.T) {
	t.Parallel()

	mock := &mockIMAPOps{defaultAccountID: testDefaultAccount}

	handler := tools.SearchHandler(mock)

	result, err := handler(context.Background(), toolReq(map[string]any{
		"query":  "test",
		"before": "yesterday",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !result.IsError {
		t.Fatal("expected error result for invalid before date")
	}

	text := resultText(t, result)
	if !strings.Contains(text, "invalid 'before' date") {
		t.Errorf("expected invalid before date error, got %q", text)
	}
}

func TestSearchHandler_SinceAfterBefore(t *testing.T) {
	t.Parallel()

	mock := &mockIMAPOps{defaultAccountID: testDefaultAccount}

	handler := tools.SearchHandler(mock)

	result, err := handler(context.Background(), toolReq(map[string]any{
		"query":  "test",
		"since":  "2026-03-01",
		"before": "2026-01-01",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !result.IsError {
		t.Fatal("expected error result when since is after before")
	}

	text := resultText(t, result)
	if !strings.Contains(text, "since") && !strings.Contains(text, "before") {
		t.Errorf("expected error mentioning since/before, got %q", text)
	}
}

func TestSearchHandler_LimitClamping(t *testing.T) {
	t.Parallel()

	mock := &mockIMAPOps{
		defaultAccountID: testDefaultAccount,
		searchResult:     []models.Email{},
	}

	handler := tools.SearchHandler(mock)

	_, err := handler(context.Background(), toolReq(map[string]any{
		"query": "test",
		"limit": float64(999),
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if mock.lastSearchArgs.limit != 500 {
		t.Errorf("expected limit clamped to 500, got %d", mock.lastSearchArgs.limit)
	}
}

func TestSearchHandler_Pagination(t *testing.T) {
	t.Parallel()

	mock := &mockIMAPOps{defaultAccountID: testDefaultAccount}
	for i := range 5 {
		mock.searchResult = append(mock.searchResult, models.Email{ID: fmt.Sprintf("default:INBOX:%d", i+1)})
	}

	result, err := tools.SearchHandler(mock)(t.Context(), toolReq(map[string]any{
		"query": "test", "limit": float64(2), "offset": float64(2),
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if mock.lastSearchArgs.limit != 5 {
		t.Fatalf("expected backend lookahead limit 5, got %d", mock.lastSearchArgs.limit)
	}

	var parsed struct {
		Messages []models.Email `json:"messages"`
		Offset   int            `json:"offset"`
		HasMore  bool           `json:"has_more"`
	}
	if err := json.Unmarshal([]byte(resultText(t, result)), &parsed); err != nil {
		t.Fatalf("parse result: %v", err)
	}
	if len(parsed.Messages) != 2 || parsed.Messages[0].ID != "default:INBOX:3" || !parsed.HasMore || parsed.Offset != 2 {
		t.Fatalf("unexpected page: %+v", parsed)
	}
}

func TestSearchHandler_RejectsInvalidOffset(t *testing.T) {
	t.Parallel()

	result, err := tools.SearchHandler(&mockIMAPOps{defaultAccountID: testDefaultAccount})(
		t.Context(), toolReq(map[string]any{"query": "test", "offset": float64(-1)}),
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError || !strings.Contains(resultText(t, result), "offset") {
		t.Fatalf("expected offset validation error, got %s", resultText(t, result))
	}
}
