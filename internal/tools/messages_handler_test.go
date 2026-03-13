package tools_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/boutquin/mcp-server-email/internal/models"
	"github.com/boutquin/mcp-server-email/internal/tools"
)

const testInbox = "INBOX"
const testSent = "Sent"

// --- List handler tests ---

func TestListHandler_HappyPath(t *testing.T) {
	t.Parallel()

	mock := &mockIMAPOps{
		defaultAccountID: testDefaultAccount,
		listMessagesResult: []models.Email{
			{ID: "default:INBOX:1", Subject: "Hello"},
			{ID: "default:INBOX:2", Subject: "World"},
		},
	}

	handler := tools.ListHandler(mock)

	result, err := handler(context.Background(), toolReq(map[string]any{
		"folder":      testInbox,
		"limit":       float64(10),
		"offset":      float64(5),
		"includeBody": true,
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.IsError {
		t.Fatal("expected success result")
	}

	args := mock.lastListMessagesArgs
	if args.accountID != testDefaultAccount {
		t.Errorf("expected account 'default', got %q", args.accountID)
	}

	if args.folder != testInbox {
		t.Errorf("expected folder 'INBOX', got %q", args.folder)
	}

	if args.limit != 10 {
		t.Errorf("expected limit 10, got %d", args.limit)
	}

	if args.offset != 5 {
		t.Errorf("expected offset 5, got %d", args.offset)
	}

	if !args.includeBody {
		t.Error("expected includeBody=true")
	}

	text := resultText(t, result)

	var resp struct {
		Count    int            `json:"count"`
		Messages []models.Email `json:"messages"`
	}

	err = json.Unmarshal([]byte(text), &resp)
	if err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}

	if resp.Count != 2 {
		t.Errorf("expected count 2, got %d", resp.Count)
	}
}

func TestListHandler_LimitClampHigh(t *testing.T) {
	t.Parallel()

	mock := &mockIMAPOps{
		defaultAccountID:   testDefaultAccount,
		listMessagesResult: []models.Email{},
	}

	handler := tools.ListHandler(mock)

	_, err := handler(context.Background(), toolReq(map[string]any{
		"limit": float64(999),
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if mock.lastListMessagesArgs.limit != 500 {
		t.Errorf("expected limit clamped to 500, got %d", mock.lastListMessagesArgs.limit)
	}
}

func TestListHandler_LimitClampLow(t *testing.T) {
	t.Parallel()

	mock := &mockIMAPOps{
		defaultAccountID:   testDefaultAccount,
		listMessagesResult: []models.Email{},
	}

	handler := tools.ListHandler(mock)

	_, err := handler(context.Background(), toolReq(map[string]any{
		"limit": float64(0),
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if mock.lastListMessagesArgs.limit != 50 {
		t.Errorf("expected limit clamped to 50 (default), got %d", mock.lastListMessagesArgs.limit)
	}
}

func TestListHandler_OffsetDefault(t *testing.T) {
	t.Parallel()

	mock := &mockIMAPOps{
		defaultAccountID:   testDefaultAccount,
		listMessagesResult: []models.Email{},
	}

	handler := tools.ListHandler(mock)

	_, err := handler(context.Background(), toolReq(nil))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if mock.lastListMessagesArgs.offset != 0 {
		t.Errorf("expected default offset 0, got %d", mock.lastListMessagesArgs.offset)
	}
}

func TestListHandler_IncludeBodyDefault(t *testing.T) {
	t.Parallel()

	mock := &mockIMAPOps{
		defaultAccountID:   testDefaultAccount,
		listMessagesResult: []models.Email{},
	}

	handler := tools.ListHandler(mock)

	_, err := handler(context.Background(), toolReq(nil))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if mock.lastListMessagesArgs.includeBody {
		t.Error("expected default includeBody=false")
	}
}

// --- Unread handler tests ---

func TestUnreadHandler_HappyPath(t *testing.T) {
	t.Parallel()

	mock := &mockIMAPOps{
		defaultAccountID: testDefaultAccount,
		listUnreadResult: []models.Email{
			{ID: "default:INBOX:1", Subject: "Unread 1"},
		},
	}

	handler := tools.UnreadHandler(mock)

	result, err := handler(context.Background(), toolReq(map[string]any{
		"folder": testSent,
		"limit":  float64(25),
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.IsError {
		t.Fatal("expected success result")
	}

	args := mock.lastListUnreadArgs
	if args.folder != testSent {
		t.Errorf("expected folder 'Sent', got %q", args.folder)
	}

	if args.limit != 25 {
		t.Errorf("expected limit 25, got %d", args.limit)
	}
}

func TestUnreadHandler_LimitClamping(t *testing.T) {
	t.Parallel()

	mock := &mockIMAPOps{
		defaultAccountID: testDefaultAccount,
		listUnreadResult: []models.Email{},
	}

	handler := tools.UnreadHandler(mock)

	_, err := handler(context.Background(), toolReq(map[string]any{
		"limit": float64(1000),
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if mock.lastListUnreadArgs.limit != 500 {
		t.Errorf("expected limit clamped to 500, got %d", mock.lastListUnreadArgs.limit)
	}
}

func TestUnreadHandler_FolderDefault(t *testing.T) {
	t.Parallel()

	mock := &mockIMAPOps{
		defaultAccountID: testDefaultAccount,
		listUnreadResult: []models.Email{},
	}

	handler := tools.UnreadHandler(mock)

	_, err := handler(context.Background(), toolReq(nil))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if mock.lastListUnreadArgs.folder != testInbox {
		t.Errorf("expected default folder 'INBOX', got %q", mock.lastListUnreadArgs.folder)
	}
}

// --- Get handler tests ---

func TestGetHandler_HappyPath(t *testing.T) {
	t.Parallel()

	mock := &mockIMAPOps{
		getMessageResult: &models.Email{
			ID:      "hello:INBOX:42",
			Subject: "Test Email",
			From:    "sender@example.com",
		},
	}

	handler := tools.GetHandler(mock)

	result, err := handler(context.Background(), toolReq(map[string]any{
		"id": "hello:INBOX:42",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.IsError {
		t.Fatal("expected success result")
	}

	args := mock.lastGetMessageArgs
	if args.accountID != testAccountHello {
		t.Errorf("expected account 'hello', got %q", args.accountID)
	}

	if args.folder != testInbox {
		t.Errorf("expected folder 'INBOX', got %q", args.folder)
	}

	if args.uid != 42 {
		t.Errorf("expected uid 42, got %d", args.uid)
	}
}

func TestGetHandler_InvalidID(t *testing.T) {
	t.Parallel()

	mock := &mockIMAPOps{}

	handler := tools.GetHandler(mock)

	result, err := handler(context.Background(), toolReq(map[string]any{
		"id": "invalid-no-colons",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !result.IsError {
		t.Fatal("expected error result for invalid ID")
	}
}

func TestGetHandler_MessageNotFound(t *testing.T) {
	t.Parallel()

	mock := &mockIMAPOps{
		getMessageErr: models.ErrMessageNotFound,
	}

	handler := tools.GetHandler(mock)

	result, err := handler(context.Background(), toolReq(map[string]any{
		"id": "hello:INBOX:999",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !result.IsError {
		t.Fatal("expected error result")
	}

	text := resultText(t, result)
	if !strings.Contains(text, "message not found") {
		t.Errorf("expected 'message not found' error, got %q", text)
	}
}

// --- Move handler tests ---

func TestMoveHandler_HappyPath(t *testing.T) {
	t.Parallel()

	mock := &mockIMAPOps{}

	handler := tools.MoveHandler(mock)

	result, err := handler(context.Background(), toolReq(map[string]any{
		"id":          "hello:INBOX:10",
		"destination": "Archive",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.IsError {
		t.Fatal("expected success result")
	}

	args := mock.lastMoveMessageArgs
	if args.accountID != testAccountHello || args.folder != testInbox || args.uid != 10 || args.dest != "Archive" {
		t.Errorf("unexpected args: %+v", args)
	}
}

func TestMoveHandler_MissingDestination(t *testing.T) {
	t.Parallel()

	mock := &mockIMAPOps{}

	handler := tools.MoveHandler(mock)

	result, err := handler(context.Background(), toolReq(map[string]any{
		"id": "hello:INBOX:10",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !result.IsError {
		t.Fatal("expected error result for missing destination")
	}

	text := resultText(t, result)
	if !strings.Contains(text, "destination is required") {
		t.Errorf("expected 'destination is required' error, got %q", text)
	}
}

func TestMoveHandler_InvalidID(t *testing.T) {
	t.Parallel()

	mock := &mockIMAPOps{}

	handler := tools.MoveHandler(mock)

	result, err := handler(context.Background(), toolReq(map[string]any{
		"id":          "bad-id",
		"destination": "Archive",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !result.IsError {
		t.Fatal("expected error result for invalid ID")
	}
}

// --- Copy handler tests ---

func TestCopyHandler_HappyPath(t *testing.T) {
	t.Parallel()

	mock := &mockIMAPOps{}

	handler := tools.CopyHandler(mock)

	result, err := handler(context.Background(), toolReq(map[string]any{
		"id":          "hello:INBOX:10",
		"destination": "Backup",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.IsError {
		t.Fatal("expected success result")
	}

	args := mock.lastCopyMessageArgs
	if args.accountID != testAccountHello || args.folder != testInbox || args.uid != 10 || args.dest != "Backup" {
		t.Errorf("unexpected args: %+v", args)
	}
}

func TestCopyHandler_MissingDestination(t *testing.T) {
	t.Parallel()

	mock := &mockIMAPOps{}

	handler := tools.CopyHandler(mock)

	result, err := handler(context.Background(), toolReq(map[string]any{
		"id": "hello:INBOX:10",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !result.IsError {
		t.Fatal("expected error result")
	}
}

func TestCopyHandler_InvalidID(t *testing.T) {
	t.Parallel()

	mock := &mockIMAPOps{}

	handler := tools.CopyHandler(mock)

	result, err := handler(context.Background(), toolReq(map[string]any{
		"id":          "bad",
		"destination": "Backup",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !result.IsError {
		t.Fatal("expected error result")
	}
}

// --- Delete handler tests ---

func TestDeleteHandler_TrashDefault(t *testing.T) {
	t.Parallel()

	mock := &mockIMAPOps{}

	handler := tools.DeleteHandler(mock)

	result, err := handler(context.Background(), toolReq(map[string]any{
		"id": "hello:INBOX:10",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.IsError {
		t.Fatal("expected success result")
	}

	if mock.lastDeleteMessageArgs.permanent {
		t.Error("expected permanent=false by default")
	}
}

func TestDeleteHandler_Permanent(t *testing.T) {
	t.Parallel()

	mock := &mockIMAPOps{}

	handler := tools.DeleteHandler(mock)

	result, err := handler(context.Background(), toolReq(map[string]any{
		"id":        "hello:INBOX:10",
		"permanent": true,
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.IsError {
		t.Fatal("expected success result")
	}

	if !mock.lastDeleteMessageArgs.permanent {
		t.Error("expected permanent=true")
	}
}

func TestDeleteHandler_InvalidID(t *testing.T) {
	t.Parallel()

	mock := &mockIMAPOps{}

	handler := tools.DeleteHandler(mock)

	result, err := handler(context.Background(), toolReq(map[string]any{
		"id": "nope",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !result.IsError {
		t.Fatal("expected error result")
	}
}

// --- MarkRead handler tests ---

func TestMarkReadHandler_MarkRead(t *testing.T) {
	t.Parallel()

	mock := &mockIMAPOps{}

	handler := tools.MarkReadHandler(mock)

	result, err := handler(context.Background(), toolReq(map[string]any{
		"id":   "hello:INBOX:10",
		"read": true,
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.IsError {
		t.Fatal("expected success result")
	}

	if !mock.lastMarkReadArgs.read {
		t.Error("expected read=true")
	}

	if mock.lastMarkReadArgs.uid != 10 {
		t.Errorf("expected uid 10, got %d", mock.lastMarkReadArgs.uid)
	}
}

func TestMarkReadHandler_MarkUnread(t *testing.T) {
	t.Parallel()

	mock := &mockIMAPOps{}

	handler := tools.MarkReadHandler(mock)

	result, err := handler(context.Background(), toolReq(map[string]any{
		"id":   "hello:INBOX:10",
		"read": false,
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.IsError {
		t.Fatal("expected success result")
	}

	if mock.lastMarkReadArgs.read {
		t.Error("expected read=false")
	}
}

func TestMarkReadHandler_MissingReadParam(t *testing.T) {
	t.Parallel()

	// When "read" is not provided, GetBool defaults to true.
	mock := &mockIMAPOps{}

	handler := tools.MarkReadHandler(mock)

	result, err := handler(context.Background(), toolReq(map[string]any{
		"id": "hello:INBOX:10",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.IsError {
		t.Fatal("expected success result (read defaults to true)")
	}

	if !mock.lastMarkReadArgs.read {
		t.Error("expected read to default to true")
	}
}

func TestMarkReadHandler_InvalidID(t *testing.T) {
	t.Parallel()

	mock := &mockIMAPOps{}

	handler := tools.MarkReadHandler(mock)

	result, err := handler(context.Background(), toolReq(map[string]any{
		"id":   "bad-id",
		"read": true,
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !result.IsError {
		t.Fatal("expected error result")
	}
}

// --- Flag handler tests ---

func TestFlagHandler_Flag(t *testing.T) {
	t.Parallel()

	mock := &mockIMAPOps{}

	handler := tools.FlagHandler(mock)

	result, err := handler(context.Background(), toolReq(map[string]any{
		"id":      "hello:INBOX:10",
		"flagged": true,
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.IsError {
		t.Fatal("expected success result")
	}

	if !mock.lastSetFlagArgs.flagged {
		t.Error("expected flagged=true")
	}
}

func TestFlagHandler_Unflag(t *testing.T) {
	t.Parallel()

	mock := &mockIMAPOps{}

	handler := tools.FlagHandler(mock)

	result, err := handler(context.Background(), toolReq(map[string]any{
		"id":      "hello:INBOX:10",
		"flagged": false,
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.IsError {
		t.Fatal("expected success result")
	}

	if mock.lastSetFlagArgs.flagged {
		t.Error("expected flagged=false")
	}
}

func TestFlagHandler_MissingFlaggedParam(t *testing.T) {
	t.Parallel()

	// When "flagged" is not provided, GetBool defaults to true.
	mock := &mockIMAPOps{}

	handler := tools.FlagHandler(mock)

	result, err := handler(context.Background(), toolReq(map[string]any{
		"id": "hello:INBOX:10",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.IsError {
		t.Fatal("expected success (flagged defaults to true)")
	}

	if !mock.lastSetFlagArgs.flagged {
		t.Error("expected flagged to default to true")
	}
}

func TestFlagHandler_InvalidID(t *testing.T) {
	t.Parallel()

	mock := &mockIMAPOps{}

	handler := tools.FlagHandler(mock)

	result, err := handler(context.Background(), toolReq(map[string]any{
		"id":      "bad",
		"flagged": true,
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !result.IsError {
		t.Fatal("expected error result")
	}
}

// --- HTML-to-text tests ---

func TestHTMLToText(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "simple paragraph",
			input: "<p>Hello, World!</p>",
			want:  "Hello, World!",
		},
		{
			name:  "nested tags",
			input: "<div><h1>Title</h1><p>Body text</p></div>",
			want:  "Body text",
		},
		{
			name:  "plain text passthrough",
			input: "Just plain text",
			want:  "Just plain text",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := tools.HTMLToText(tt.input)

			if !strings.Contains(got, tt.want) {
				t.Errorf("htmlToText(%q)\n  got:  %q\n  want contains: %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestGetHandler_HTMLBody(t *testing.T) {
	t.Parallel()

	mock := &mockIMAPOps{
		getMessageResult: &models.Email{
			ID:          "hello:INBOX:42",
			Subject:     "HTML Email",
			From:        "sender@example.com",
			Body:        "<h1>Hello</h1><p>This is HTML</p>",
			ContentType: "text/html",
		},
	}

	handler := tools.GetHandler(mock)

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

	// Should not contain raw HTML tags
	if strings.Contains(text, "<h1>") || strings.Contains(text, "<p>") {
		t.Errorf("expected HTML to be converted to plain text, got %q", text)
	}

	// Should contain the text content
	if !strings.Contains(text, "Hello") || !strings.Contains(text, "This is HTML") {
		t.Errorf("expected text content to be preserved, got %q", text)
	}
}

func TestListHandler_HTMLBody(t *testing.T) {
	t.Parallel()

	mock := &mockIMAPOps{
		defaultAccountID: testDefaultAccount,
		listMessagesResult: []models.Email{
			{
				ID:          "default:INBOX:1",
				Subject:     "HTML Email",
				Body:        "<p>HTML content</p>",
				ContentType: "text/html",
			},
		},
	}

	handler := tools.ListHandler(mock)

	result, err := handler(context.Background(), toolReq(map[string]any{
		"includeBody": true,
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	text := resultText(t, result)

	if strings.Contains(text, "<p>") {
		t.Errorf("expected HTML to be converted, got %q", text)
	}

	if !strings.Contains(text, "HTML content") {
		t.Errorf("expected text content preserved, got %q", text)
	}
}

func TestUnreadHandler_HTMLBody(t *testing.T) {
	t.Parallel()

	mock := &mockIMAPOps{
		defaultAccountID: testDefaultAccount,
		listUnreadResult: []models.Email{
			{
				ID:          "default:INBOX:1",
				Subject:     "HTML Unread",
				Body:        "<b>Bold text</b>",
				ContentType: "text/html",
			},
		},
	}

	handler := tools.UnreadHandler(mock)

	result, err := handler(context.Background(), toolReq(map[string]any{
		"includeBody": true,
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	text := resultText(t, result)

	if strings.Contains(text, "<b>") {
		t.Errorf("expected HTML to be converted, got %q", text)
	}
}

func TestGetHandler_PlainTextBody(t *testing.T) {
	t.Parallel()

	mock := &mockIMAPOps{
		getMessageResult: &models.Email{
			ID:          "hello:INBOX:42",
			Subject:     "Plain Email",
			From:        "sender@example.com",
			Body:        "Just plain text",
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

	if !strings.Contains(text, "Just plain text") {
		t.Errorf("expected plain text body preserved, got %q", text)
	}
}
