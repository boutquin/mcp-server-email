package tools_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/boutquin/mcp-server-email/internal/models"
	"github.com/boutquin/mcp-server-email/internal/tools"
)

func TestReplyTool(t *testing.T) {
	t.Parallel()

	tool := tools.ReplyTool()

	if tool.Name != "email_reply" {
		t.Errorf("expected tool name 'email_reply', got %q", tool.Name)
	}

	if tool.Description == "" {
		t.Error("tool description should not be empty")
	}

	// Check required params
	required := map[string]bool{"id": false, "body": false}

	for _, req := range tool.InputSchema.Required {
		if _, ok := required[req]; ok {
			required[req] = true
		}
	}

	for param, found := range required {
		if !found {
			t.Errorf("expected %q to be required", param)
		}
	}
}

func TestForwardTool(t *testing.T) {
	t.Parallel()

	tool := tools.ForwardTool()

	if tool.Name != "email_forward" {
		t.Errorf("expected tool name 'email_forward', got %q", tool.Name)
	}

	if tool.Description == "" {
		t.Error("tool description should not be empty")
	}

	// Check required params
	required := map[string]bool{"id": false, "to": false}

	for _, req := range tool.InputSchema.Required {
		if _, ok := required[req]; ok {
			required[req] = true
		}
	}

	for param, found := range required {
		if !found {
			t.Errorf("expected %q to be required", param)
		}
	}
}

func TestExtractEmailAddress(t *testing.T) {
	t.Parallel()

	tests := []struct {
		input string
		want  string
	}{
		{"alice@example.com", "alice@example.com"},
		{"Alice <alice@example.com>", "alice@example.com"},
		{"  alice@example.com  ", "alice@example.com"},
		{"\"Alice Smith\" <alice@example.com>", "alice@example.com"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			t.Parallel()

			got := tools.ExtractEmailAddress(tt.input)
			if got != tt.want {
				t.Errorf("ExtractEmailAddress(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestReplyHandler_HappyPath(t *testing.T) {
	t.Parallel()

	imapMock := &mockIMAPOps{
		defaultAccountID: "acct1",
		getMessageResult: &models.Email{
			ID:              "acct1:INBOX:42",
			Subject:         "Hello",
			From:            "alice@example.com",
			To:              []string{"me@example.com"},
			Body:            "Original body\nSecond line",
			Mailbox:         "INBOX",
			MessageIDHeader: "<msg-001@example.com>",
			References:      []string{"<ref-001@example.com>"},
		},
	}

	smtpMock := &mockSMTPOps{
		defaultAccountID: "acct1",
		accountEmails:    map[string]string{"acct1": "me@example.com"},
	}

	handler := tools.ReplyHandler(imapMock, smtpMock)
	req := toolReq(map[string]any{
		"id":   "acct1:INBOX:42",
		"body": "Thanks for the message!",
	})

	result, err := handler(context.Background(), req)
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

	// Verify SMTP was called
	if !smtpMock.sendCalled {
		t.Fatal("expected SMTP Send to be called")
	}

	// Verify In-Reply-To
	if smtpMock.lastSendReq.InReplyTo != "<msg-001@example.com>" {
		t.Errorf("In-Reply-To = %q, want %q", smtpMock.lastSendReq.InReplyTo, "<msg-001@example.com>")
	}

	// Verify References chain = original References + original Message-ID
	wantRefs := []string{"<ref-001@example.com>", "<msg-001@example.com>"}
	if len(smtpMock.lastSendReq.References) != len(wantRefs) {
		t.Fatalf("References = %v, want %v", smtpMock.lastSendReq.References, wantRefs)
	}

	for i, ref := range wantRefs {
		if smtpMock.lastSendReq.References[i] != ref {
			t.Errorf("References[%d] = %q, want %q", i, smtpMock.lastSendReq.References[i], ref)
		}
	}

	// Verify To = original From
	if len(smtpMock.lastSendReq.To) != 1 || smtpMock.lastSendReq.To[0] != "alice@example.com" {
		t.Errorf("To = %v, want [alice@example.com]", smtpMock.lastSendReq.To)
	}

	// Verify Subject = "Re: Hello"
	if smtpMock.lastSendReq.Subject != "Re: Hello" {
		t.Errorf("Subject = %q, want %q", smtpMock.lastSendReq.Subject, "Re: Hello")
	}

	// Verify body includes quoted original
	if !strings.Contains(smtpMock.lastSendReq.Body, "> Original body") {
		t.Errorf("body should contain quoted original, got %q", smtpMock.lastSendReq.Body)
	}

	if !strings.Contains(smtpMock.lastSendReq.Body, "> Second line") {
		t.Errorf("body should contain quoted second line, got %q", smtpMock.lastSendReq.Body)
	}

	// Verify reply body appears before quoted original
	idx := strings.Index(smtpMock.lastSendReq.Body, "Thanks for the message!")
	quoteIdx := strings.Index(smtpMock.lastSendReq.Body, "> Original body")

	if idx < 0 || quoteIdx < 0 || idx > quoteIdx {
		t.Errorf("reply body should appear before quoted original")
	}
}

func TestReplyHandler_ReplyAll(t *testing.T) {
	t.Parallel()

	imapMock := &mockIMAPOps{
		defaultAccountID: "acct1",
		getMessageResult: &models.Email{
			ID:              "acct1:INBOX:42",
			Subject:         "Team discussion",
			From:            "alice@example.com",
			To:              []string{"me@example.com", "bob@example.com"},
			CC:              []string{"carol@example.com", "me@example.com"},
			Body:            "Let's discuss",
			Mailbox:         "INBOX",
			MessageIDHeader: "<msg-002@example.com>",
		},
	}

	smtpMock := &mockSMTPOps{
		defaultAccountID: "acct1",
		accountEmails:    map[string]string{"acct1": "me@example.com"},
	}

	handler := tools.ReplyHandler(imapMock, smtpMock)
	req := toolReq(map[string]any{
		"id":   "acct1:INBOX:42",
		"body": "Agreed!",
		"all":  true,
	})

	result, err := handler(context.Background(), req)
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

	// To = original From
	if len(smtpMock.lastSendReq.To) != 1 || smtpMock.lastSendReq.To[0] != "alice@example.com" {
		t.Errorf("To = %v, want [alice@example.com]", smtpMock.lastSendReq.To)
	}

	// CC = original To + original CC, minus self
	cc := smtpMock.lastSendReq.CC
	if len(cc) != 2 {
		t.Fatalf("CC = %v, want 2 entries (bob + carol)", cc)
	}

	ccSet := make(map[string]bool)
	for _, addr := range cc {
		ccSet[addr] = true
	}

	if !ccSet["bob@example.com"] {
		t.Errorf("CC should include bob@example.com, got %v", cc)
	}

	if !ccSet["carol@example.com"] {
		t.Errorf("CC should include carol@example.com, got %v", cc)
	}

	if ccSet["me@example.com"] {
		t.Errorf("CC should NOT include self (me@example.com), got %v", cc)
	}
}

func TestReplyHandler_AlreadyHasRePrefix(t *testing.T) {
	t.Parallel()

	imapMock := &mockIMAPOps{
		defaultAccountID: "acct1",
		getMessageResult: &models.Email{
			ID:              "acct1:INBOX:42",
			Subject:         "Re: Hello",
			From:            "alice@example.com",
			To:              []string{"me@example.com"},
			Body:            "Reply to your reply",
			Mailbox:         "INBOX",
			MessageIDHeader: "<msg-003@example.com>",
		},
	}

	smtpMock := &mockSMTPOps{
		defaultAccountID: "acct1",
		accountEmails:    map[string]string{"acct1": "me@example.com"},
	}

	handler := tools.ReplyHandler(imapMock, smtpMock)
	req := toolReq(map[string]any{
		"id":   "acct1:INBOX:42",
		"body": "Got it!",
	})

	result, err := handler(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	text := resultText(t, result)

	var res map[string]any

	err = json.Unmarshal([]byte(text), &res)
	if err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	// Subject stays "Re: Hello", no double "Re: Re: Hello"
	if smtpMock.lastSendReq.Subject != "Re: Hello" {
		t.Errorf("Subject = %q, want %q", smtpMock.lastSendReq.Subject, "Re: Hello")
	}
}

func TestReplyHandler_NoMessageID(t *testing.T) {
	t.Parallel()

	imapMock := &mockIMAPOps{
		defaultAccountID: "acct1",
		getMessageResult: &models.Email{
			ID:              "acct1:INBOX:42",
			Subject:         "No ID",
			From:            "alice@example.com",
			To:              []string{"me@example.com"},
			Body:            "Message without ID",
			Mailbox:         "INBOX",
			MessageIDHeader: "", // empty
		},
	}

	smtpMock := &mockSMTPOps{
		defaultAccountID: "acct1",
		accountEmails:    map[string]string{"acct1": "me@example.com"},
	}

	handler := tools.ReplyHandler(imapMock, smtpMock)
	req := toolReq(map[string]any{
		"id":   "acct1:INBOX:42",
		"body": "Reply anyway",
	})

	result, err := handler(context.Background(), req)
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

	// In-Reply-To should be empty
	if smtpMock.lastSendReq.InReplyTo != "" {
		t.Errorf("InReplyTo = %q, want empty", smtpMock.lastSendReq.InReplyTo)
	}

	// References should be empty (no message ID to add)
	if len(smtpMock.lastSendReq.References) != 0 {
		t.Errorf("References = %v, want empty", smtpMock.lastSendReq.References)
	}
}
