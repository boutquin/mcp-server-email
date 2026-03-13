package tools_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/boutquin/mcp-server-email/internal/models"
	"github.com/boutquin/mcp-server-email/internal/tools"
)

const testAccountHello = "hello"

func TestAccountsTool(t *testing.T) {
	t.Parallel()

	tool := tools.AccountsTool()

	if tool.Name != "email_accounts" {
		t.Errorf("expected tool name 'email_accounts', got %q", tool.Name)
	}

	if tool.Description == "" {
		t.Error("tool description should not be empty")
	}
}

func TestAccountsHandler_TwoAccounts(t *testing.T) {
	t.Parallel()

	mock := &mockIMAPOps{
		accountStatuses: []models.AccountStatus{
			{ID: testAccountHello, Email: "hello@example.com", Connected: true, IsDefault: true},
			{ID: "info", Email: "info@example.com", Connected: false, IsDefault: false},
		},
	}

	handler := tools.AccountsHandler(mock)

	result, err := handler(context.Background(), toolReq(nil))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.IsError {
		t.Fatal("expected success result")
	}

	if !mock.accountStatusCalled {
		t.Fatal("mock AccountStatus() was not called")
	}

	// Parse the JSON response.
	text := resultText(t, result)

	var resp struct {
		Accounts []models.AccountStatus `json:"accounts"`
	}

	err = json.Unmarshal([]byte(text), &resp)
	if err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}

	if len(resp.Accounts) != 2 {
		t.Fatalf("expected 2 accounts, got %d", len(resp.Accounts))
	}

	if resp.Accounts[0].ID != testAccountHello {
		t.Errorf("expected first account ID 'hello', got %q", resp.Accounts[0].ID)
	}

	if resp.Accounts[1].ID != "info" {
		t.Errorf("expected second account ID 'info', got %q", resp.Accounts[1].ID)
	}
}

func TestAccountsHandler_EmptyAccounts(t *testing.T) {
	t.Parallel()

	mock := &mockIMAPOps{
		accountStatuses: []models.AccountStatus{},
	}

	handler := tools.AccountsHandler(mock)

	result, err := handler(context.Background(), toolReq(nil))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.IsError {
		t.Fatal("expected success result")
	}

	text := resultText(t, result)

	var resp struct {
		Accounts []models.AccountStatus `json:"accounts"`
	}

	err = json.Unmarshal([]byte(text), &resp)
	if err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}

	if len(resp.Accounts) != 0 {
		t.Errorf("expected 0 accounts, got %d", len(resp.Accounts))
	}
}
