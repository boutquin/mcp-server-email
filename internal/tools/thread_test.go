package tools_test

import (
	"encoding/json"
	"strconv"
	"strings"
	"testing"

	"github.com/boutquin/mcp-server-email/internal/models"
	"github.com/boutquin/mcp-server-email/internal/tools"
	"github.com/mark3labs/mcp-go/mcp"
)

func TestThreadTool(t *testing.T) {
	t.Parallel()

	tool := tools.ThreadTool()

	if tool.Name != "email_thread" {
		t.Errorf("expected tool name 'email_thread', got %q", tool.Name)
	}

	if !strings.Contains(tool.Description, "thread") {
		t.Error("tool description should mention thread")
	}
}

func TestThreadHandler_TwoMessages(t *testing.T) {
	t.Parallel()

	msgA := models.Email{
		ID:              "test:INBOX:1",
		Subject:         "Original",
		Date:            "2026-01-01T10:00:00Z",
		MessageIDHeader: "<a@example.com>",
	}

	msgB := models.Email{
		ID:              "test:INBOX:2",
		Subject:         "Re: Original",
		Date:            "2026-01-01T11:00:00Z",
		MessageIDHeader: "<b@example.com>",
		InReplyTo:       "<a@example.com>",
		References:      []string{"<a@example.com>"},
	}

	mock := &mockIMAPOps{
		defaultAccountID:   "test",
		getMessageResult:   &msgA,
		getFolderByRoleErr: models.ErrFolderRoleNotFound, // No Sent folder
		searchByMessageIDFunc: func(_, _, msgID string) ([]models.Email, error) {
			switch msgID {
			case "<a@example.com>":
				return []models.Email{msgA, msgB}, nil
			default:
				return nil, nil
			}
		},
	}

	handler := tools.ThreadHandler(mock)

	result, err := handler(t.Context(), toolReq(map[string]any{
		"id": "test:INBOX:1",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	text := resultText(t, result)

	var parsed struct {
		Thread []models.Email `json:"thread"`
		Count  int            `json:"count"`
	}

	jsonErr := json.Unmarshal([]byte(text), &parsed)
	if jsonErr != nil {
		t.Fatalf("parse result: %v", jsonErr)
	}

	if parsed.Count != 2 {
		t.Errorf("expected 2 messages in thread, got %d", parsed.Count)
	}

	// Verify sorted by date (oldest first).
	if len(parsed.Thread) >= 2 {
		if parsed.Thread[0].Date > parsed.Thread[1].Date {
			t.Error("thread should be sorted by date ascending")
		}
	}
}

func TestThreadHandler_FiveMessageChain(t *testing.T) {
	t.Parallel()

	msgs := []models.Email{
		{
			ID: "test:INBOX:1", Subject: "A",
			Date: "2026-01-01T10:00:00Z", MessageIDHeader: "<a@ex.com>",
		},
		{
			ID: "test:INBOX:2", Subject: "B",
			Date: "2026-01-01T11:00:00Z", MessageIDHeader: "<b@ex.com>",
			InReplyTo: "<a@ex.com>", References: []string{"<a@ex.com>"},
		},
		{
			ID: "test:INBOX:3", Subject: "C",
			Date: "2026-01-01T12:00:00Z", MessageIDHeader: "<c@ex.com>",
			InReplyTo:  "<b@ex.com>",
			References: []string{"<a@ex.com>", "<b@ex.com>"},
		},
		{
			ID: "test:INBOX:4", Subject: "D",
			Date: "2026-01-01T13:00:00Z", MessageIDHeader: "<d@ex.com>",
			InReplyTo:  "<c@ex.com>",
			References: []string{"<a@ex.com>", "<b@ex.com>", "<c@ex.com>"},
		},
		{
			ID: "test:INBOX:5", Subject: "E",
			Date: "2026-01-01T14:00:00Z", MessageIDHeader: "<e@ex.com>",
			InReplyTo:  "<d@ex.com>",
			References: []string{"<a@ex.com>", "<b@ex.com>", "<c@ex.com>", "<d@ex.com>"},
		},
	}

	mock := &mockIMAPOps{
		defaultAccountID:   "test",
		getMessageResult:   &msgs[2], // Start from message C
		getFolderByRoleErr: models.ErrFolderRoleNotFound,
		searchByMessageIDFunc: func(_, _, msgID string) ([]models.Email, error) {
			// Return all messages for any thread-related search.
			switch msgID {
			case "<a@ex.com>":
				return msgs, nil
			default:
				return nil, nil
			}
		},
	}

	handler := tools.ThreadHandler(mock)

	result, err := handler(t.Context(), toolReq(map[string]any{
		"id": "test:INBOX:3",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	text := resultText(t, result)

	var parsed struct {
		Count int `json:"count"`
	}

	jsonErr := json.Unmarshal([]byte(text), &parsed)
	if jsonErr != nil {
		t.Fatalf("parse result: %v", jsonErr)
	}

	if parsed.Count != 5 {
		t.Errorf("expected 5 messages in chain, got %d", parsed.Count)
	}
}

func TestThreadHandler_ForkMerge(t *testing.T) {
	t.Parallel()

	msgA := models.Email{
		ID: "test:INBOX:1", Subject: "A", Date: "2026-01-01T10:00:00Z",
		MessageIDHeader: "<a@ex.com>",
	}

	msgB := models.Email{
		ID: "test:INBOX:2", Subject: "B", Date: "2026-01-01T11:00:00Z",
		MessageIDHeader: "<b@ex.com>", InReplyTo: "<a@ex.com>",
		References: []string{"<a@ex.com>"},
	}

	msgC := models.Email{
		ID: "test:INBOX:3", Subject: "C", Date: "2026-01-01T12:00:00Z",
		MessageIDHeader: "<c@ex.com>", InReplyTo: "<a@ex.com>",
		References: []string{"<a@ex.com>"},
	}

	mock := &mockIMAPOps{
		defaultAccountID:   "test",
		getMessageResult:   &msgA,
		getFolderByRoleErr: models.ErrFolderRoleNotFound,
		searchByMessageIDFunc: func(_, _, msgID string) ([]models.Email, error) {
			if msgID == "<a@ex.com>" {
				return []models.Email{msgA, msgB, msgC}, nil
			}

			return nil, nil
		},
	}

	handler := tools.ThreadHandler(mock)

	result, err := handler(t.Context(), toolReq(map[string]any{
		"id": "test:INBOX:1",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	text := resultText(t, result)

	var parsed struct {
		Count int `json:"count"`
	}

	jsonErr := json.Unmarshal([]byte(text), &parsed)
	if jsonErr != nil {
		t.Fatalf("parse result: %v", jsonErr)
	}

	if parsed.Count != 3 {
		t.Errorf("expected 3 messages (A + B + C), got %d", parsed.Count)
	}
}

func TestThreadHandler_SingleMessage(t *testing.T) {
	t.Parallel()

	msg := models.Email{
		ID:              "test:INBOX:99",
		Subject:         "Standalone",
		Date:            "2026-01-01T10:00:00Z",
		MessageIDHeader: "<standalone@ex.com>",
	}

	mock := &mockIMAPOps{
		defaultAccountID:   "test",
		getMessageResult:   &msg,
		getFolderByRoleErr: models.ErrFolderRoleNotFound,
		searchByMessageIDFunc: func(_, _, msgID string) ([]models.Email, error) {
			if msgID == "<standalone@ex.com>" {
				return []models.Email{msg}, nil
			}

			return nil, nil
		},
	}

	handler := tools.ThreadHandler(mock)

	result, err := handler(t.Context(), toolReq(map[string]any{
		"id": "test:INBOX:99",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	text := resultText(t, result)

	var parsed struct {
		Count int `json:"count"`
	}

	jsonErr := json.Unmarshal([]byte(text), &parsed)
	if jsonErr != nil {
		t.Fatalf("parse result: %v", jsonErr)
	}

	if parsed.Count != 1 {
		t.Errorf("expected 1 message for standalone, got %d", parsed.Count)
	}
}

func TestThreadHandler_InvalidID(t *testing.T) {
	t.Parallel()

	mock := &mockIMAPOps{
		defaultAccountID: "test",
	}

	handler := tools.ThreadHandler(mock)

	result, err := handler(t.Context(), toolReq(map[string]any{
		"id": "invalid",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !result.IsError {
		t.Error("expected error result for invalid ID")
	}
}

func TestThreadHandler_GetMessageError(t *testing.T) {
	t.Parallel()

	mock := &mockIMAPOps{
		defaultAccountID: "test",
		getMessageErr:    models.ErrMessageNotFound,
	}

	handler := tools.ThreadHandler(mock)

	result, err := handler(t.Context(), toolReq(map[string]any{
		"id": "test:INBOX:42",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !result.IsError {
		t.Error("expected error result when GetMessage fails")
	}
}

// --- Cross-Folder Thread Tests (P30) ---

func TestThreadHandler_CrossFolder_InboxAndSent(t *testing.T) {
	t.Parallel()

	// Message A is in INBOX (received), message B is in Sent (our reply).
	msgA := models.Email{
		ID: "test:INBOX:1", Subject: "Question",
		Date: "2026-01-01T10:00:00Z", MessageIDHeader: "<q@ex.com>",
	}

	msgB := models.Email{
		ID: "test:Sent:5", Subject: "Re: Question",
		Date: "2026-01-01T11:00:00Z", MessageIDHeader: "<r@ex.com>",
		InReplyTo: "<q@ex.com>", References: []string{"<q@ex.com>"},
	}

	mock := &mockIMAPOps{
		defaultAccountID:      "test",
		getMessageResult:      &msgA,
		getFolderByRoleResult: testSent,
		searchByMessageIDFunc: func(_, folder, msgID string) ([]models.Email, error) {
			if folder == testInbox && msgID == "<q@ex.com>" {
				return []models.Email{msgA}, nil
			}

			if folder == testSent && msgID == "<q@ex.com>" {
				return []models.Email{msgB}, nil
			}

			return nil, nil
		},
	}

	handler := tools.ThreadHandler(mock)

	result, err := handler(t.Context(), toolReq(map[string]any{
		"id": "test:INBOX:1",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	parsed := parseThreadResult(t, result)

	if parsed.Count != 2 {
		t.Errorf("expected 2 messages (INBOX + Sent), got %d", parsed.Count)
	}
}

func TestThreadHandler_CrossFolder_SentToInbox(t *testing.T) {
	t.Parallel()

	// Starting from a message in Sent, should also search INBOX.
	msgSent := models.Email{
		ID: "test:Sent:1", Subject: "Hello",
		Date: "2026-01-01T10:00:00Z", MessageIDHeader: "<hello@ex.com>",
	}

	msgInbox := models.Email{
		ID: "test:INBOX:5", Subject: "Re: Hello",
		Date: "2026-01-01T11:00:00Z", MessageIDHeader: "<reply@ex.com>",
		InReplyTo: "<hello@ex.com>", References: []string{"<hello@ex.com>"},
	}

	mock := &mockIMAPOps{
		defaultAccountID:      "test",
		getMessageResult:      &msgSent,
		getFolderByRoleResult: testSent,
		searchByMessageIDFunc: func(_, folder, msgID string) ([]models.Email, error) {
			if folder == testSent && msgID == "<hello@ex.com>" {
				return []models.Email{msgSent}, nil
			}

			if folder == testInbox && msgID == "<hello@ex.com>" {
				return []models.Email{msgInbox}, nil
			}

			return nil, nil
		},
	}

	handler := tools.ThreadHandler(mock)

	result, err := handler(t.Context(), toolReq(map[string]any{
		"id": "test:Sent:1",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	parsed := parseThreadResult(t, result)

	if parsed.Count != 2 {
		t.Errorf("expected 2 messages (Sent + INBOX), got %d", parsed.Count)
	}
}

func TestThreadHandler_CrossFolder_SentNotFound(t *testing.T) {
	t.Parallel()

	// Sent folder doesn't exist — should still find messages in INBOX.
	msg := models.Email{
		ID: "test:INBOX:1", Subject: "Solo",
		Date: "2026-01-01T10:00:00Z", MessageIDHeader: "<solo@ex.com>",
	}

	mock := &mockIMAPOps{
		defaultAccountID:   "test",
		getMessageResult:   &msg,
		getFolderByRoleErr: models.ErrFolderRoleNotFound,
		searchByMessageIDFunc: func(_, _, msgID string) ([]models.Email, error) {
			if msgID == "<solo@ex.com>" {
				return []models.Email{msg}, nil
			}

			return nil, nil
		},
	}

	handler := tools.ThreadHandler(mock)

	result, err := handler(t.Context(), toolReq(map[string]any{
		"id": "test:INBOX:1",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	parsed := parseThreadResult(t, result)

	if parsed.Count != 1 {
		t.Errorf("expected 1 message (Sent not found), got %d", parsed.Count)
	}
}

func TestThreadHandler_CrossFolder_Deduplication(t *testing.T) {
	t.Parallel()

	// Same message appears in both INBOX and Sent (e.g., BCC to self).
	// Should be deduplicated by MessageIDHeader.
	msgInbox := models.Email{
		ID: "test:INBOX:1", Subject: "Dup",
		Date: "2026-01-01T10:00:00Z", MessageIDHeader: "<dup@ex.com>",
	}

	msgSent := models.Email{
		ID: "test:Sent:1", Subject: "Dup",
		Date: "2026-01-01T10:00:00Z", MessageIDHeader: "<dup@ex.com>",
	}

	mock := &mockIMAPOps{
		defaultAccountID:      "test",
		getMessageResult:      &msgInbox,
		getFolderByRoleResult: testSent,
		searchByMessageIDFunc: func(_, folder, msgID string) ([]models.Email, error) {
			if msgID != "<dup@ex.com>" {
				return nil, nil
			}

			if folder == testInbox {
				return []models.Email{msgInbox}, nil
			}

			if folder == testSent {
				return []models.Email{msgSent}, nil
			}

			return nil, nil
		},
	}

	handler := tools.ThreadHandler(mock)

	result, err := handler(t.Context(), toolReq(map[string]any{
		"id": "test:INBOX:1",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	parsed := parseThreadResult(t, result)

	if parsed.Count != 1 {
		t.Errorf("expected 1 message (deduped by Message-ID), got %d", parsed.Count)
	}
}

func TestThreadHandler_CrossFolder_LargeThread(t *testing.T) {
	t.Parallel()

	// 12-message thread spanning INBOX and Sent.
	inboxMsgs := make([]models.Email, 0, 6)
	sentMsgs := make([]models.Email, 0, 6)

	for i := range 6 {
		inboxMsgs = append(inboxMsgs, models.Email{
			ID:              "test:INBOX:" + itoa(i+1),
			Subject:         "Thread",
			Date:            "2026-01-01T" + itoa(10+i) + ":00:00Z",
			MessageIDHeader: "<thread-" + itoa(i*2) + "@ex.com>",
			References:      []string{"<thread-0@ex.com>"},
		})

		sentMsgs = append(sentMsgs, models.Email{
			ID:              "test:Sent:" + itoa(i+1),
			Subject:         "Re: Thread",
			Date:            "2026-01-01T" + itoa(10+i) + ":30:00Z",
			MessageIDHeader: "<thread-" + itoa(i*2+1) + "@ex.com>",
			References:      []string{"<thread-0@ex.com>"},
		})
	}

	mock := &mockIMAPOps{
		defaultAccountID:      "test",
		getMessageResult:      &inboxMsgs[0],
		getFolderByRoleResult: testSent,
		searchByMessageIDFunc: func(_, folder, msgID string) ([]models.Email, error) {
			if msgID == "<thread-0@ex.com>" {
				if folder == testInbox {
					return inboxMsgs, nil
				}

				if folder == testSent {
					return sentMsgs, nil
				}
			}

			return nil, nil
		},
	}

	handler := tools.ThreadHandler(mock)

	result, err := handler(t.Context(), toolReq(map[string]any{
		"id": "test:INBOX:1",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	parsed := parseThreadResult(t, result)

	if parsed.Count != 12 {
		t.Errorf("expected 12 messages across folders, got %d", parsed.Count)
	}
}

// --- Helpers ---

type threadResult struct {
	Thread []models.Email `json:"thread"`
	Count  int            `json:"count"`
}

func parseThreadResult(t *testing.T, result *mcp.CallToolResult) threadResult {
	t.Helper()

	text := resultText(t, result)

	var parsed threadResult

	err := json.Unmarshal([]byte(text), &parsed)
	if err != nil {
		t.Fatalf("parse thread result: %v (%s)", err, text)
	}

	return parsed
}

func itoa(n int) string {
	return strconv.Itoa(n)
}

// --- Thread Folder Expansion Tests (P57) ---

func TestThreadHandler_IncludesArchive(t *testing.T) {
	t.Parallel()

	msg := models.Email{
		ID: "test:INBOX:1", Subject: "Archived thread",
		Date: "2026-01-01T10:00:00Z", MessageIDHeader: "<arch@ex.com>",
	}

	archiveMsg := models.Email{
		ID: "test:Archive:5", Subject: "Re: Archived thread",
		Date: "2026-01-01T11:00:00Z", MessageIDHeader: "<arch-reply@ex.com>",
		InReplyTo: "<arch@ex.com>", References: []string{"<arch@ex.com>"},
	}

	mock := newArchiveMock(&msg, map[string][]models.Email{
		"INBOX:<arch@ex.com>":   {msg},
		"Archive:<arch@ex.com>": {archiveMsg},
	})

	handler := tools.ThreadHandler(mock)

	result, err := handler(t.Context(), toolReq(map[string]any{
		"id": "test:INBOX:1",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	parsed := parseThreadResult(t, result)

	if parsed.Count != 2 {
		t.Errorf("expected 2 messages (INBOX + Archive), got %d", parsed.Count)
	}
}

func TestThreadHandler_IncludesAllMail(t *testing.T) {
	t.Parallel()

	msg := models.Email{
		ID: "test:INBOX:1", Subject: "Gmail thread",
		Date: "2026-01-01T10:00:00Z", MessageIDHeader: "<gmail@ex.com>",
	}

	allMailMsg := models.Email{
		ID: "test:[Gmail]/All Mail:99", Subject: "Re: Gmail thread",
		Date: "2026-01-01T12:00:00Z", MessageIDHeader: "<gmail-all@ex.com>",
		InReplyTo: "<gmail@ex.com>", References: []string{"<gmail@ex.com>"},
	}

	mock := newAllMailMock(&msg, map[string][]models.Email{
		testInbox + ":<gmail@ex.com>":         {msg},
		"[Gmail]/All Mail:<gmail@ex.com>":     {allMailMsg},
		testSent + ":<gmail@ex.com>":          {},
		"[Gmail]/All Mail:<gmail-all@ex.com>": {},
	})

	handler := tools.ThreadHandler(mock)

	result, err := handler(t.Context(), toolReq(map[string]any{
		"id": "test:INBOX:1",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	parsed := parseThreadResult(t, result)

	if parsed.Count != 2 {
		t.Errorf("expected 2 messages (INBOX + All Mail), got %d", parsed.Count)
	}
}

func TestThreadHandler_IgnoresMissingRoles(t *testing.T) {
	t.Parallel()

	msg := models.Email{
		ID: "test:INBOX:1", Subject: "No roles",
		Date: "2026-01-01T10:00:00Z", MessageIDHeader: "<norole@ex.com>",
	}

	mock := &mockIMAPOps{
		defaultAccountID: "test",
		getMessageResult: &msg,
		getFolderByRoleFunc: func(_ string, _ models.FolderRole) (string, error) {
			return "", models.ErrFolderRoleNotFound
		},
		searchByMessageIDFunc: func(_, folder, msgID string) ([]models.Email, error) {
			if folder == testInbox && msgID == "<norole@ex.com>" {
				return []models.Email{msg}, nil
			}

			return nil, nil
		},
	}

	handler := tools.ThreadHandler(mock)

	result, err := handler(t.Context(), toolReq(map[string]any{
		"id": "test:INBOX:1",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	parsed := parseThreadResult(t, result)

	if parsed.Count != 1 {
		t.Errorf("expected 1 message (all roles missing), got %d", parsed.Count)
	}

	if result.IsError {
		t.Error("missing roles should not produce an error")
	}
}

func TestThreadHandler_NoDuplicateFolders(t *testing.T) {
	t.Parallel()

	msg := models.Email{
		ID: "test:INBOX:1", Subject: "Dedup test",
		Date: "2026-01-01T10:00:00Z", MessageIDHeader: "<dedup@ex.com>",
	}

	// Archive role returns "INBOX" — same as original folder.
	// Should not search INBOX twice.
	searchCount := 0

	mock := &mockIMAPOps{
		defaultAccountID: "test",
		getMessageResult: &msg,
		getFolderByRoleFunc: func(_ string, role models.FolderRole) (string, error) {
			switch role { //nolint:exhaustive // test only uses Sent and Archive
			case models.RoleSent:
				return testSent, nil
			case models.RoleArchive:
				return testInbox, nil // Archive maps to INBOX (dedup scenario)
			default:
				return "", models.ErrFolderRoleNotFound
			}
		},
		searchByMessageIDFunc: func(_, folder, msgID string) ([]models.Email, error) {
			if folder == testInbox {
				searchCount++
			}

			if msgID == "<dedup@ex.com>" {
				return []models.Email{msg}, nil
			}

			return nil, nil
		},
	}

	handler := tools.ThreadHandler(mock)

	result, err := handler(t.Context(), toolReq(map[string]any{
		"id": "test:INBOX:1",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	parsed := parseThreadResult(t, result)

	if parsed.Count != 1 {
		t.Errorf("expected 1 message (dedup), got %d", parsed.Count)
	}

	if searchCount > 1 {
		t.Errorf("INBOX searched %d times, expected at most 1 (dedup failed)", searchCount)
	}
}

func TestThreadHandler_ArchiveMessages(t *testing.T) {
	t.Parallel()

	// Integration-style: thread with messages in INBOX, Sent, and Archive.
	msgInbox := models.Email{
		ID: "test:INBOX:1", Subject: "Start",
		Date: "2026-01-01T10:00:00Z", MessageIDHeader: "<start@ex.com>",
	}

	msgSent := models.Email{
		ID: "test:Sent:2", Subject: "Re: Start",
		Date: "2026-01-01T11:00:00Z", MessageIDHeader: "<reply@ex.com>",
		InReplyTo: "<start@ex.com>", References: []string{"<start@ex.com>"},
	}

	msgArchive := models.Email{
		ID: "test:Archive:3", Subject: "Fwd: Start",
		Date: "2026-01-01T12:00:00Z", MessageIDHeader: "<archived@ex.com>",
		InReplyTo: "<reply@ex.com>", References: []string{"<start@ex.com>", "<reply@ex.com>"},
	}

	mock := newArchiveMock(&msgInbox, map[string][]models.Email{
		"INBOX:<start@ex.com>":      {msgInbox},
		"Sent:<start@ex.com>":       {msgSent},
		"Archive:<start@ex.com>":    {msgArchive},
		"INBOX:<reply@ex.com>":      {},
		"Sent:<reply@ex.com>":       {},
		"Archive:<reply@ex.com>":    {},
		"INBOX:<archived@ex.com>":   {},
		"Sent:<archived@ex.com>":    {},
		"Archive:<archived@ex.com>": {},
	})

	handler := tools.ThreadHandler(mock)

	result, err := handler(t.Context(), toolReq(map[string]any{
		"id": "test:INBOX:1",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	parsed := parseThreadResult(t, result)

	if parsed.Count != 3 {
		t.Errorf("expected 3 messages (INBOX + Sent + Archive), got %d", parsed.Count)
	}
}

// --- P57 Test Helpers ---

func newArchiveMock(
	target *models.Email,
	searchResults map[string][]models.Email,
) *mockIMAPOps {
	return &mockIMAPOps{
		defaultAccountID: "test",
		getMessageResult: target,
		getFolderByRoleFunc: func(_ string, role models.FolderRole) (string, error) {
			switch role { //nolint:exhaustive // test only uses Sent and Archive
			case models.RoleSent:
				return testSent, nil
			case models.RoleArchive:
				return "Archive", nil
			default:
				return "", models.ErrFolderRoleNotFound
			}
		},
		searchByMessageIDFunc: func(_, folder, msgID string) ([]models.Email, error) {
			key := folder + ":" + msgID

			if msgs, ok := searchResults[key]; ok {
				return msgs, nil
			}

			return nil, nil
		},
	}
}

func newAllMailMock(
	target *models.Email,
	searchResults map[string][]models.Email,
) *mockIMAPOps {
	return &mockIMAPOps{
		defaultAccountID: "test",
		getMessageResult: target,
		getFolderByRoleFunc: func(_ string, role models.FolderRole) (string, error) {
			switch role { //nolint:exhaustive // test only uses Sent
			case models.RoleSent:
				return testSent, nil
			default:
				return "", models.ErrFolderRoleNotFound
			}
		},
		listFoldersResult: []models.Folder{
			{Name: testInbox},
			{Name: testSent},
			{Name: "[Gmail]/All Mail"},
			{Name: "[Gmail]/Trash"},
		},
		searchByMessageIDFunc: func(_, folder, msgID string) ([]models.Email, error) {
			key := folder + ":" + msgID

			if msgs, ok := searchResults[key]; ok {
				return msgs, nil
			}

			return nil, nil
		},
	}
}
