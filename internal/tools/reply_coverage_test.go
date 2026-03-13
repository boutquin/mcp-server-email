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

// errIMAPConn and errSMTPSend are sentinel errors for handler error-path tests.
var (
	errIMAPConn   = fmt.Errorf("imap: %w", models.ErrAccountNotFound)
	errSMTPSend   = fmt.Errorf("smtp: %w", models.ErrAccountNotFound)
	errAcctLookup = fmt.Errorf("lookup: %w", models.ErrAccountNotFound)
)

// --- ReplyHandler error path tests ---

func TestReplyHandler_MissingID(t *testing.T) {
	t.Parallel()

	handler := tools.ReplyHandler(&mockIMAPOps{}, &mockSMTPOps{})

	result, err := handler(context.Background(), toolReq(map[string]any{
		"body": "reply text",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !result.IsError {
		t.Fatal("expected error result for missing id")
	}

	text := resultText(t, result)
	if !strings.Contains(text, "id is required") {
		t.Errorf("expected 'id is required', got %q", text)
	}
}

func TestReplyHandler_MissingBody(t *testing.T) {
	t.Parallel()

	handler := tools.ReplyHandler(&mockIMAPOps{}, &mockSMTPOps{})

	result, err := handler(context.Background(), toolReq(map[string]any{
		"id": "acct1:INBOX:42",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !result.IsError {
		t.Fatal("expected error result for missing body")
	}

	text := resultText(t, result)
	if !strings.Contains(text, "body is required") {
		t.Errorf("expected 'body is required', got %q", text)
	}
}

func TestReplyHandler_InvalidMessageID(t *testing.T) {
	t.Parallel()

	handler := tools.ReplyHandler(&mockIMAPOps{}, &mockSMTPOps{})

	result, err := handler(context.Background(), toolReq(map[string]any{
		"id":   "bad-id",
		"body": "reply",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !result.IsError {
		t.Fatal("expected error result for invalid message ID")
	}
}

func TestReplyHandler_GetMessageError(t *testing.T) {
	t.Parallel()

	imapMock := &mockIMAPOps{
		defaultAccountID: "acct1",
		getMessageErr:    errIMAPConn,
	}
	smtpMock := &mockSMTPOps{
		defaultAccountID: "acct1",
		accountEmails:    map[string]string{"acct1": "me@example.com"},
	}

	handler := tools.ReplyHandler(imapMock, smtpMock)

	result, err := handler(context.Background(), toolReq(map[string]any{
		"id":   "acct1:INBOX:42",
		"body": "reply",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !result.IsError {
		t.Fatal("expected error result for GetMessage failure")
	}

	text := resultText(t, result)
	if !strings.Contains(text, "account not found") {
		t.Errorf("expected error message, got %q", text)
	}
}

func TestReplyHandler_AccountEmailError(t *testing.T) {
	t.Parallel()

	imapMock := &mockIMAPOps{
		defaultAccountID: "acct1",
		getMessageResult: &models.Email{
			ID:      "acct1:INBOX:42",
			Subject: "Hello",
			From:    "alice@example.com",
			To:      []string{"me@example.com"},
			Body:    "Original body",
			Mailbox: "INBOX",
		},
	}
	smtpMock := &mockSMTPOps{
		defaultAccountID: "acct1",
		accountEmailErr:  errAcctLookup,
	}

	handler := tools.ReplyHandler(imapMock, smtpMock)

	result, err := handler(context.Background(), toolReq(map[string]any{
		"id":   "acct1:INBOX:42",
		"body": "reply",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !result.IsError {
		t.Fatal("expected error result for AccountEmail failure")
	}

	text := resultText(t, result)
	if !strings.Contains(text, "account not found") {
		t.Errorf("expected error message, got %q", text)
	}
}

func TestReplyHandler_SendError(t *testing.T) {
	t.Parallel()

	imapMock := &mockIMAPOps{
		defaultAccountID: "acct1",
		getMessageResult: &models.Email{
			ID:      "acct1:INBOX:42",
			Subject: "Hello",
			From:    "alice@example.com",
			To:      []string{"me@example.com"},
			Body:    "Original body",
			Mailbox: "INBOX",
		},
	}
	smtpMock := &mockSMTPOps{
		defaultAccountID: "acct1",
		accountEmails:    map[string]string{"acct1": "me@example.com"},
		sendErr:          errSMTPSend,
	}

	handler := tools.ReplyHandler(imapMock, smtpMock)

	result, err := handler(context.Background(), toolReq(map[string]any{
		"id":   "acct1:INBOX:42",
		"body": "reply",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !result.IsError {
		t.Fatal("expected error result for Send failure")
	}

	text := resultText(t, result)
	if !strings.Contains(text, "account not found") {
		t.Errorf("expected error message, got %q", text)
	}
}

func TestReplyHandler_WithHTMLAndCCBCC(t *testing.T) {
	t.Parallel()

	imapMock := &mockIMAPOps{
		defaultAccountID: "acct1",
		getMessageResult: &models.Email{
			ID:              "acct1:INBOX:42",
			Subject:         "Hello",
			From:            "alice@example.com",
			To:              []string{"me@example.com"},
			Body:            "Original body",
			Mailbox:         "INBOX",
			MessageIDHeader: "<msg-001@example.com>",
		},
	}
	smtpMock := &mockSMTPOps{
		defaultAccountID: "acct1",
		accountEmails:    map[string]string{"acct1": "me@example.com"},
	}

	handler := tools.ReplyHandler(imapMock, smtpMock)

	result, err := handler(context.Background(), toolReq(map[string]any{
		"id":     "acct1:INBOX:42",
		"body":   "<p>HTML reply</p>",
		"isHtml": true,
		"cc":     "extra-cc@example.com",
		"bcc":    "secret@example.com",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	text := resultText(t, result)

	var res map[string]any

	err = json.Unmarshal([]byte(text), &res)
	if err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	if res["success"] != true {
		t.Fatalf("expected success=true, got %v", res["success"])
	}

	if !smtpMock.lastSendReq.IsHTML {
		t.Error("expected IsHTML=true")
	}

	// CC should include the extra cc.
	ccFound := false

	for _, addr := range smtpMock.lastSendReq.CC {
		if addr == "extra-cc@example.com" {
			ccFound = true
		}
	}

	if !ccFound {
		t.Errorf("CC should include extra-cc@example.com, got %v", smtpMock.lastSendReq.CC)
	}

	if len(smtpMock.lastSendReq.BCC) != 1 || smtpMock.lastSendReq.BCC[0] != "secret@example.com" {
		t.Errorf("BCC = %v, want [secret@example.com]", smtpMock.lastSendReq.BCC)
	}
}

// --- ForwardHandler error path tests ---

func TestForwardHandler_MissingID(t *testing.T) {
	t.Parallel()

	handler := tools.ForwardHandler(&mockIMAPOps{}, &mockSMTPOps{}, testLimits)

	result, err := handler(context.Background(), toolReq(map[string]any{
		"to": "bob@example.com",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !result.IsError {
		t.Fatal("expected error result for missing id")
	}

	text := resultText(t, result)
	if !strings.Contains(text, "id is required") {
		t.Errorf("expected 'id is required', got %q", text)
	}
}

func TestForwardHandler_MissingTo(t *testing.T) {
	t.Parallel()

	handler := tools.ForwardHandler(&mockIMAPOps{}, &mockSMTPOps{}, testLimits)

	result, err := handler(context.Background(), toolReq(map[string]any{
		"id": "acct1:INBOX:42",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !result.IsError {
		t.Fatal("expected error result for missing to")
	}

	text := resultText(t, result)
	if !strings.Contains(text, "to is required") {
		t.Errorf("expected 'to is required', got %q", text)
	}
}

func TestForwardHandler_InvalidMessageID(t *testing.T) {
	t.Parallel()

	handler := tools.ForwardHandler(&mockIMAPOps{}, &mockSMTPOps{}, testLimits)

	result, err := handler(context.Background(), toolReq(map[string]any{
		"id": "bad-id",
		"to": "bob@example.com",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !result.IsError {
		t.Fatal("expected error result for invalid message ID")
	}
}

func TestForwardHandler_GetMessageError(t *testing.T) {
	t.Parallel()

	imapMock := &mockIMAPOps{
		defaultAccountID: "acct1",
		getMessageErr:    errIMAPConn,
	}
	smtpMock := &mockSMTPOps{
		defaultAccountID: "acct1",
		accountEmails:    map[string]string{"acct1": "me@example.com"},
	}

	handler := tools.ForwardHandler(imapMock, smtpMock, testLimits)

	result, err := handler(context.Background(), toolReq(map[string]any{
		"id": "acct1:INBOX:42",
		"to": "bob@example.com",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !result.IsError {
		t.Fatal("expected error result for GetMessage failure")
	}

	text := resultText(t, result)
	if !strings.Contains(text, "account not found") {
		t.Errorf("expected error message, got %q", text)
	}
}

func TestForwardHandler_SendError(t *testing.T) {
	t.Parallel()

	imapMock := &mockIMAPOps{
		defaultAccountID: "acct1",
		getMessageResult: &models.Email{
			ID:      "acct1:INBOX:42",
			Subject: "FYI",
			From:    "alice@example.com",
			To:      []string{"me@example.com"},
			Date:    "2026-02-22T10:00:00Z",
			Body:    "Check this out.",
			Mailbox: "INBOX",
		},
	}
	smtpMock := &mockSMTPOps{
		defaultAccountID: "acct1",
		accountEmails:    map[string]string{"acct1": "me@example.com"},
		sendErr:          errSMTPSend,
	}

	handler := tools.ForwardHandler(imapMock, smtpMock, testLimits)

	result, err := handler(context.Background(), toolReq(map[string]any{
		"id": "acct1:INBOX:42",
		"to": "bob@example.com",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !result.IsError {
		t.Fatal("expected error result for Send failure")
	}

	text := resultText(t, result)
	if !strings.Contains(text, "account not found") {
		t.Errorf("expected error message, got %q", text)
	}
}

func TestForwardHandler_WithCCBCCAndHTML(t *testing.T) {
	t.Parallel()

	imapMock := &mockIMAPOps{
		defaultAccountID: "acct1",
		getMessageResult: &models.Email{
			ID:      "acct1:INBOX:42",
			Subject: "Important",
			From:    "alice@example.com",
			To:      []string{"me@example.com"},
			Date:    "2026-02-22T10:00:00Z",
			Body:    "Important content.",
			Mailbox: "INBOX",
		},
	}
	smtpMock := &mockSMTPOps{
		defaultAccountID: "acct1",
		accountEmails:    map[string]string{"acct1": "me@example.com"},
	}

	handler := tools.ForwardHandler(imapMock, smtpMock, testLimits)

	result, err := handler(context.Background(), toolReq(map[string]any{
		"id":     "acct1:INBOX:42",
		"to":     "bob@example.com",
		"cc":     "carol@example.com",
		"bcc":    "hidden@example.com",
		"isHtml": true,
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	text := resultText(t, result)

	var res map[string]any

	err = json.Unmarshal([]byte(text), &res)
	if err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	if res["success"] != true {
		t.Fatalf("expected success=true, got %v", res["success"])
	}

	if !smtpMock.lastSendReq.IsHTML {
		t.Error("expected IsHTML=true")
	}

	if len(smtpMock.lastSendReq.CC) != 1 || smtpMock.lastSendReq.CC[0] != "carol@example.com" {
		t.Errorf("CC = %v, want [carol@example.com]", smtpMock.lastSendReq.CC)
	}

	if len(smtpMock.lastSendReq.BCC) != 1 || smtpMock.lastSendReq.BCC[0] != "hidden@example.com" {
		t.Errorf("BCC = %v, want [hidden@example.com]", smtpMock.lastSendReq.BCC)
	}
}

func TestForwardHandler_WithAccountOverride(t *testing.T) {
	t.Parallel()

	imapMock := &mockIMAPOps{
		defaultAccountID: "acct1",
		getMessageResult: &models.Email{
			ID:      "acct1:INBOX:42",
			Subject: "FYI",
			From:    "alice@example.com",
			To:      []string{"me@example.com"},
			Date:    "2026-02-22T10:00:00Z",
			Body:    "Content",
			Mailbox: "INBOX",
		},
	}
	smtpMock := &mockSMTPOps{
		defaultAccountID: "acct1",
		accountEmails:    map[string]string{"acct1": "me@example.com", "acct2": "other@example.com"},
	}

	handler := tools.ForwardHandler(imapMock, smtpMock, testLimits)

	result, err := handler(context.Background(), toolReq(map[string]any{
		"id":      "acct1:INBOX:42",
		"to":      "bob@example.com",
		"account": "acct2",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	text := resultText(t, result)

	var res map[string]any

	err = json.Unmarshal([]byte(text), &res)
	if err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	if res["success"] != true {
		t.Fatalf("expected success=true, got %v", res["success"])
	}

	if smtpMock.lastSendAccountID != "acct2" {
		t.Errorf("expected send via acct2, got %q", smtpMock.lastSendAccountID)
	}
}
