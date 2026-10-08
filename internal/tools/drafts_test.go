package tools_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/boutquin/mcp-server-email/internal/models"
	"github.com/boutquin/mcp-server-email/internal/tools"
)

func TestDraftListHandlerUsesDraftRole(t *testing.T) {
	t.Parallel()
	mock := &mockIMAPOps{
		defaultAccountID:      "acct",
		getFolderByRoleResult: "[Gmail]/Drafts",
		listMessagesResult:    []models.Email{{ID: "acct:[Gmail]/Drafts:7", Subject: "draft"}},
	}
	result, err := tools.DraftListHandler(mock)(t.Context(), toolReq(map[string]any{"limit": float64(10)}))
	if err != nil || result.IsError {
		t.Fatalf("result=%v err=%v", result, err)
	}
	if mock.lastListMessagesArgs.folder != "[Gmail]/Drafts" || mock.lastListMessagesArgs.accountID != "acct" {
		t.Fatalf("unexpected list args: %+v", mock.lastListMessagesArgs)
	}
}

func TestDraftUpdateHandlerReturnsReplacementID(t *testing.T) {
	t.Parallel()
	mock := &mockIMAPOps{
		getFolderByRoleResult: "[Gmail]/Drafts",
		updateDraftUID:        99,
	}
	raw := "From: me@example.com\r\nContent-Type: multipart/mixed; boundary=x\r\n\r\n--x--\r\n"
	result, err := tools.DraftUpdateHandler(mock)(t.Context(), toolReq(map[string]any{
		"id": "acct:[Gmail]/Drafts:42", "message": raw,
	}))
	if err != nil || result.IsError {
		t.Fatalf("result=%v err=%v", result, err)
	}
	if mock.lastUpdateDraftArgs.uid != 42 || string(mock.lastUpdateDraftArgs.msg) != raw {
		t.Fatalf("unexpected update args: %+v", mock.lastUpdateDraftArgs)
	}
	var response struct {
		ID         string `json:"id"`
		ReplacedID string `json:"replaced_id"`
	}
	if err := json.Unmarshal([]byte(resultText(t, result)), &response); err != nil {
		t.Fatal(err)
	}
	if response.ID != "acct:[Gmail]/Drafts:99" || response.ReplacedID != "acct:[Gmail]/Drafts:42" {
		t.Fatalf("unexpected response: %+v", response)
	}
}

func TestDraftUpdateHandlerRejectsNonDraft(t *testing.T) {
	t.Parallel()
	mock := &mockIMAPOps{getFolderByRoleResult: "Drafts"}
	result, err := tools.DraftUpdateHandler(mock)(t.Context(), toolReq(map[string]any{
		"id": "acct:INBOX:42", "message": "Subject: replacement\r\n\r\nbody",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError || !strings.Contains(resultText(t, result), "not a draft") {
		t.Fatalf("unexpected result: %s", resultText(t, result))
	}
}

func TestDraftDeleteHandlerUsesIDAccountAndUID(t *testing.T) {
	t.Parallel()
	mock := &mockIMAPOps{getFolderByRoleResult: "Drafts"}
	result, err := tools.DraftDeleteHandler(mock)(t.Context(), toolReq(map[string]any{"id": "work:Drafts:17"}))
	if err != nil || result.IsError {
		t.Fatalf("result=%v err=%v", result, err)
	}
	if mock.lastDeleteDraftArgs.accountID != "work" || mock.lastDeleteDraftArgs.uid != 17 {
		t.Fatalf("unexpected delete args: %+v", mock.lastDeleteDraftArgs)
	}
}

func TestDraftToolsRequiredParameters(t *testing.T) {
	t.Parallel()
	if got := tools.DraftUpdateTool().InputSchema.Required; len(got) != 2 {
		t.Fatalf("update required params = %v", got)
	}
	if got := tools.DraftDeleteTool().InputSchema.Required; len(got) != 1 || got[0] != "id" {
		t.Fatalf("delete required params = %v", got)
	}
}
