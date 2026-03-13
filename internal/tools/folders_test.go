package tools_test

import (
	"context"
	"encoding/json"
	"slices"
	"strings"
	"testing"

	"github.com/boutquin/mcp-server-email/internal/models"
	"github.com/boutquin/mcp-server-email/internal/tools"
)

const (
	testDefaultAccount = "default"
	testArchiveFolder  = "Archive"
)

func TestFoldersTool(t *testing.T) {
	t.Parallel()

	tool := tools.FoldersTool()

	if tool.Name != "email_folders" {
		t.Errorf("expected tool name 'email_folders', got %q", tool.Name)
	}

	if tool.Description == "" {
		t.Error("tool description should not be empty")
	}
}

func TestFolderCreateTool(t *testing.T) {
	t.Parallel()

	tool := tools.FolderCreateTool()

	if tool.Name != "email_folder_create" {
		t.Errorf("expected tool name 'email_folder_create', got %q", tool.Name)
	}

	if tool.Description == "" {
		t.Error("tool description should not be empty")
	}

	if !slices.Contains(tool.InputSchema.Required, "name") {
		t.Error("'name' should be a required parameter")
	}
}

// --- Folders handler tests ---

func TestFoldersHandler_WithAccount(t *testing.T) {
	t.Parallel()

	mock := &mockIMAPOps{
		defaultAccountID: testDefaultAccount,
		listFoldersResult: []models.Folder{
			{Name: "INBOX", Unread: 5, Total: 100},
			{Name: "Sent", Unread: 0, Total: 50},
		},
	}

	handler := tools.FoldersHandler(mock)

	result, err := handler(context.Background(), toolReq(map[string]any{
		"account": testAccountHello,
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.IsError {
		t.Fatal("expected success result")
	}

	if mock.lastListFoldersAccountID != testAccountHello {
		t.Errorf("expected account 'hello', got %q", mock.lastListFoldersAccountID)
	}

	text := resultText(t, result)

	var resp struct {
		Account string          `json:"account"`
		Folders []models.Folder `json:"folders"`
	}

	err = json.Unmarshal([]byte(text), &resp)
	if err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}

	if resp.Account != testAccountHello {
		t.Errorf("expected account 'hello' in response, got %q", resp.Account)
	}

	if len(resp.Folders) != 2 {
		t.Fatalf("expected 2 folders, got %d", len(resp.Folders))
	}
}

func TestFoldersHandler_AccountNotFound(t *testing.T) {
	t.Parallel()

	mock := &mockIMAPOps{
		defaultAccountID: testDefaultAccount,
		listFoldersErr:   models.ErrAccountNotFound,
	}

	handler := tools.FoldersHandler(mock)

	result, err := handler(context.Background(), toolReq(map[string]any{
		"account": "nonexistent",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !result.IsError {
		t.Fatal("expected error result")
	}

	text := resultText(t, result)
	if !strings.Contains(text, "account not found") {
		t.Errorf("expected 'account not found' in error, got %q", text)
	}
}

func TestFoldersHandler_DefaultAccount(t *testing.T) {
	t.Parallel()

	mock := &mockIMAPOps{
		defaultAccountID:  "default-acct",
		listFoldersResult: []models.Folder{},
	}

	handler := tools.FoldersHandler(mock)

	_, err := handler(context.Background(), toolReq(nil))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if mock.lastListFoldersAccountID != "default-acct" {
		t.Errorf("expected default account 'default-acct', got %q", mock.lastListFoldersAccountID)
	}
}

// --- FolderCreate handler tests ---

func TestFolderCreateHandler_HappyPath(t *testing.T) {
	t.Parallel()

	mock := &mockIMAPOps{
		defaultAccountID: testDefaultAccount,
	}

	handler := tools.FolderCreateHandler(mock)

	result, err := handler(context.Background(), toolReq(map[string]any{
		"name": testArchiveFolder,
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.IsError {
		t.Fatal("expected success result")
	}

	if mock.lastCreateFolderArgs.accountID != testDefaultAccount {
		t.Errorf("expected account 'default', got %q", mock.lastCreateFolderArgs.accountID)
	}

	if mock.lastCreateFolderArgs.name != testArchiveFolder {
		t.Errorf("expected folder name 'Archive', got %q", mock.lastCreateFolderArgs.name)
	}

	text := resultText(t, result)

	var resp struct {
		Success bool   `json:"success"`
		Folder  string `json:"folder"`
	}

	err = json.Unmarshal([]byte(text), &resp)
	if err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}

	if !resp.Success {
		t.Error("expected success=true")
	}

	if resp.Folder != testArchiveFolder {
		t.Errorf("expected folder 'Archive', got %q", resp.Folder)
	}
}

func TestFolderCreateHandler_MissingName(t *testing.T) {
	t.Parallel()

	mock := &mockIMAPOps{defaultAccountID: testDefaultAccount}

	handler := tools.FolderCreateHandler(mock)

	result, err := handler(context.Background(), toolReq(nil))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !result.IsError {
		t.Fatal("expected error result for missing name")
	}

	text := resultText(t, result)
	if !strings.Contains(text, "name is required") {
		t.Errorf("expected 'name is required' error, got %q", text)
	}
}

func TestFolderCreateHandler_AccountNotFound(t *testing.T) {
	t.Parallel()

	mock := &mockIMAPOps{
		defaultAccountID: testDefaultAccount,
		createFolderErr:  models.ErrAccountNotFound,
	}

	handler := tools.FolderCreateHandler(mock)

	result, err := handler(context.Background(), toolReq(map[string]any{
		"name":    testArchiveFolder,
		"account": "bad",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !result.IsError {
		t.Fatal("expected error result")
	}

	text := resultText(t, result)
	if !strings.Contains(text, "account not found") {
		t.Errorf("expected 'account not found' error, got %q", text)
	}
}
