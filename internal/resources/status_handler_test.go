package resources_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/boutquin/mcp-server-email/internal/config"
	"github.com/boutquin/mcp-server-email/internal/models"
	"github.com/boutquin/mcp-server-email/internal/resources"
	"github.com/mark3labs/mcp-go/mcp"
)

var errNotImplemented = errors.New("not implemented")

// mockIMAPOps implements imap.Operations for the status handler test.
type mockIMAPOps struct {
	statuses         []models.AccountStatus
	defaultAccountID string
}

func (m *mockIMAPOps) ListFolders(_ context.Context, _ string) ([]models.Folder, error) {
	return nil, errNotImplemented
}

func (m *mockIMAPOps) GetFolderByRole(
	_ context.Context, _ string, _ models.FolderRole,
) (string, error) {
	return "", errNotImplemented
}

func (m *mockIMAPOps) ListMessages(
	_ context.Context, _, _ string, _, _ int, _ bool,
) ([]models.Email, error) {
	return nil, errNotImplemented
}

func (m *mockIMAPOps) ListUnread(
	_ context.Context, _, _ string, _ int, _ bool,
) ([]models.Email, error) {
	return nil, errNotImplemented
}

func (m *mockIMAPOps) Search(
	_ context.Context, _, _, _, _, _, _, _ string, _ int, _ bool,
) ([]models.Email, error) {
	return nil, errNotImplemented
}

func (m *mockIMAPOps) GetMessage(_ context.Context, _, _ string, _ uint32) (*models.Email, error) {
	return nil, errNotImplemented
}

func (m *mockIMAPOps) MoveMessage(_ context.Context, _, _ string, _ uint32, _ string) error {
	return nil
}

func (m *mockIMAPOps) CopyMessage(_ context.Context, _, _ string, _ uint32, _ string) error {
	return nil
}

func (m *mockIMAPOps) DeleteMessage(_ context.Context, _, _ string, _ uint32, _ bool) error {
	return nil
}

func (m *mockIMAPOps) MarkRead(_ context.Context, _, _ string, _ uint32, _ bool) error {
	return nil
}

func (m *mockIMAPOps) SetFlag(_ context.Context, _, _ string, _ uint32, _ bool) error {
	return nil
}

func (m *mockIMAPOps) CreateFolder(_ context.Context, _, _ string) error {
	return nil
}

func (m *mockIMAPOps) SaveDraft(_ context.Context, _ string, _ []byte) (uint32, error) {
	return 0, errNotImplemented
}

func (m *mockIMAPOps) GetDraft(_ context.Context, _ string, _ uint32) ([]byte, error) {
	return nil, errNotImplemented
}

func (m *mockIMAPOps) DeleteDraft(_ context.Context, _ string, _ uint32) error {
	return nil
}

func (m *mockIMAPOps) GetAttachments(
	_ context.Context, _, _ string, _ uint32,
) ([]models.AttachmentInfo, error) {
	return nil, nil
}

func (m *mockIMAPOps) GetAttachment(
	_ context.Context, _, _ string, _ uint32, _ int,
) ([]byte, string, error) {
	return nil, "", nil
}

func (m *mockIMAPOps) SearchByMessageID(
	_ context.Context, _, _, _ string,
) ([]models.Email, error) {
	return nil, nil
}

func (m *mockIMAPOps) AccountStatus() []models.AccountStatus {
	return m.statuses
}

func (m *mockIMAPOps) DefaultAccountID() string {
	return m.defaultAccountID
}

func TestStatusHandler_AccountsAndVersion(t *testing.T) {
	t.Parallel()

	cfg := &config.Config{
		DefaultAccount: "hello",
	}

	mock := &mockIMAPOps{
		statuses: []models.AccountStatus{
			{ID: "hello", Email: "hello@example.com", Connected: true},
			{ID: "info", Email: "info@example.com", Connected: false},
		},
	}

	handler := resources.StatusHandler(cfg, mock)

	contents, err := handler(context.Background(), mcp.ReadResourceRequest{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(contents) != 1 {
		t.Fatalf("expected 1 resource content, got %d", len(contents))
	}

	textContent, ok := contents[0].(mcp.TextResourceContents)
	if !ok {
		t.Fatal("expected TextResourceContents")
	}

	if textContent.URI != "email://status" {
		t.Errorf("expected URI 'email://status', got %q", textContent.URI)
	}

	if textContent.MIMEType != "application/json" {
		t.Errorf("expected MIME type 'application/json', got %q", textContent.MIMEType)
	}

	var status struct {
		Server         string `json:"server"`
		Version        string `json:"version"`
		DefaultAccount string `json:"defaultAccount"`
		Runtime        string `json:"runtime"`
		Accounts       []struct {
			ID        string `json:"id"`
			Email     string `json:"email"`
			Connected bool   `json:"connected"`
		} `json:"accounts"`
	}

	err = json.Unmarshal([]byte(textContent.Text), &status)
	if err != nil {
		t.Fatalf("failed to parse status JSON: %v", err)
	}

	if status.Server != "mcp-server-email" {
		t.Errorf("expected server 'mcp-server-email', got %q", status.Server)
	}

	if status.Version != resources.Version {
		t.Errorf("expected version %q, got %q", resources.Version, status.Version)
	}

	if status.DefaultAccount != "hello" {
		t.Errorf("expected default account 'hello', got %q", status.DefaultAccount)
	}

	if status.Runtime == "" {
		t.Error("expected non-empty runtime")
	}

	if len(status.Accounts) != 2 {
		t.Fatalf("expected 2 accounts, got %d", len(status.Accounts))
	}

	if status.Accounts[0].ID != "hello" || !status.Accounts[0].Connected {
		t.Errorf("unexpected first account: %+v", status.Accounts[0])
	}

	if status.Accounts[1].ID != "info" || status.Accounts[1].Connected {
		t.Errorf("unexpected second account: %+v", status.Accounts[1])
	}
}

func TestStatusHandler_RateLimit(t *testing.T) {
	t.Parallel()

	cfg := &config.Config{
		DefaultAccount:   "hello",
		IMAPRateLimitRPM: 60,
		SMTPRateLimitRPH: 100,
	}

	mock := &mockIMAPOps{
		statuses: []models.AccountStatus{
			{ID: "hello", Email: "hello@example.com", Connected: true},
		},
	}

	handler := resources.StatusHandler(cfg, mock)

	contents, err := handler(context.Background(), mcp.ReadResourceRequest{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(contents) != 1 {
		t.Fatalf("expected 1 resource content, got %d", len(contents))
	}

	textContent, ok := contents[0].(mcp.TextResourceContents)
	if !ok {
		t.Fatal("expected TextResourceContents")
	}

	var status struct {
		RateLimit struct {
			IMAP struct {
				RequestsPerMinute int `json:"requestsPerMinute"`
			} `json:"imap"`
			SMTP struct {
				SendsPerHour int `json:"sendsPerHour"`
			} `json:"smtp"`
		} `json:"rateLimit"`
	}

	err = json.Unmarshal([]byte(textContent.Text), &status)
	if err != nil {
		t.Fatalf("failed to parse status JSON: %v", err)
	}

	if status.RateLimit.IMAP.RequestsPerMinute != 60 {
		t.Errorf("expected IMAP rate limit 60, got %d", status.RateLimit.IMAP.RequestsPerMinute)
	}

	if status.RateLimit.SMTP.SendsPerHour != 100 {
		t.Errorf("expected SMTP rate limit 100, got %d", status.RateLimit.SMTP.SendsPerHour)
	}
}
