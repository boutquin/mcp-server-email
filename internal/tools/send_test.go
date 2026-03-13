package tools_test

import (
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/boutquin/mcp-server-email/internal/smtp"
	"github.com/boutquin/mcp-server-email/internal/tools"
)

func TestSendTool(t *testing.T) {
	t.Parallel()

	tool := tools.SendTool()

	if tool.Name != "email_send" {
		t.Errorf("expected tool name 'email_send', got %q", tool.Name)
	}

	if tool.Description == "" {
		t.Error("tool description should not be empty")
	}

	// Check required params
	required := map[string]bool{"to": false, "subject": false, "body": false}

	for _, req := range tool.InputSchema.Required {
		if _, ok := required[req]; ok {
			required[req] = true
		}
	}

	for param, found := range required {
		if !found {
			t.Errorf("'%s' should be a required parameter", param)
		}
	}
}

func TestDraftCreateTool(t *testing.T) {
	t.Parallel()

	tool := tools.DraftCreateTool()

	if tool.Name != "email_draft_create" {
		t.Errorf("expected tool name 'email_draft_create', got %q", tool.Name)
	}

	if tool.Description == "" {
		t.Error("tool description should not be empty")
	}
}

func TestDraftSendTool(t *testing.T) {
	t.Parallel()

	tool := tools.DraftSendTool()

	if tool.Name != "email_draft_send" {
		t.Errorf("expected tool name 'email_draft_send', got %q", tool.Name)
	}

	// Check that 'id' is required
	if !slices.Contains(tool.InputSchema.Required, "id") {
		t.Error("'id' should be a required parameter")
	}
}

func TestSplitAddresses(t *testing.T) {
	t.Parallel()

	tests := []struct {
		input string
		want  []string
	}{
		{"one@example.com", []string{"one@example.com"}},
		{"one@example.com, two@example.com", []string{"one@example.com", "two@example.com"}},
		{"one@example.com,two@example.com", []string{"one@example.com", "two@example.com"}},
		{"  one@example.com  ,  two@example.com  ", []string{"one@example.com", "two@example.com"}},
		{"", nil},
		{"   ", nil},
		{",,,", nil},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			t.Parallel()

			got := tools.SplitAddresses(tt.input)
			if len(got) != len(tt.want) {
				t.Errorf("tools.SplitAddresses(%q) = %v, want %v", tt.input, got, tt.want)

				return
			}

			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("tools.SplitAddresses(%q)[%d] = %q, want %q", tt.input, i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestDraftContentIsHTML(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		raw  string
		want bool
	}{
		{
			name: "CRLF_HTML",
			raw:  "Content-Type: text/html; charset=utf-8\r\n\r\n<h1>Hello</h1>",
			want: true,
		},
		{
			name: "CRLF_Plain",
			raw:  "Content-Type: text/plain\r\n\r\nHello",
			want: false,
		},
		{
			name: "LF_HTML",
			raw:  "Content-Type: text/html; charset=utf-8\n\nHello",
			want: true,
		},
		{
			name: "LF_Plain",
			raw:  "Content-Type: text/plain\n\nHello",
			want: false,
		},
		{
			name: "NoSeparator",
			raw:  "Content-Type: text/html",
			want: false,
		},
		{
			name: "NoContentType",
			raw:  "Subject: Test\r\n\r\nBody",
			want: false,
		},
		{
			name: "EmptyInput",
			raw:  "",
			want: false,
		},
		{
			name: "LF_HTML_WithBody",
			raw:  "Content-Type: text/html\n\n<p>paragraph</p>",
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := tools.DraftContentIsHTML([]byte(tt.raw))
			if got != tt.want {
				t.Errorf("DraftContentIsHTML(%q) = %v, want %v", tt.raw, got, tt.want)
			}
		})
	}
}

func TestBuildDraftMessage_PlainText(t *testing.T) {
	t.Parallel()

	msg, err := tools.BuildDraftMessage(
		"sender@example.com",
		[]string{"recipient@example.com"},
		nil,
		nil,
		"Test Subject",
		"Hello, World!",
		false,
		nil,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	content := string(msg)

	if !strings.Contains(content, "sender@example.com") {
		t.Error("message should contain From address")
	}

	if !strings.Contains(content, "Subject: Test Subject") {
		t.Error("message should contain Subject header")
	}

	if !strings.Contains(content, "text/plain") {
		t.Error("message should have text/plain content type")
	}

	if !strings.Contains(content, "Hello, World!") {
		t.Error("message should contain body")
	}
}

func TestBuildDraftMessage_HTML(t *testing.T) {
	t.Parallel()

	msg, err := tools.BuildDraftMessage(
		"sender@example.com",
		[]string{"recipient@example.com"},
		nil,
		nil,
		"HTML Email",
		"<h1>Hello</h1>",
		true,
		nil,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	content := string(msg)

	if !strings.Contains(content, "text/html") {
		t.Error("message should have text/html content type")
	}
}

func TestBuildDraftMessage_WithCCAndBCC(t *testing.T) {
	t.Parallel()

	msg, err := tools.BuildDraftMessage(
		"sender@example.com",
		[]string{"to@example.com"},
		[]string{"cc@example.com"},
		[]string{"bcc@example.com"},
		"Test",
		"Body",
		false,
		nil,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	content := string(msg)

	if !strings.Contains(content, "Cc:") {
		t.Error("message should contain Cc header")
	}

	// BCC headers are intentionally stripped from serialized MIME output by go-mail
	// (standard RFC 5322 behavior). The BCC addresses are set internally on the
	// message object but omitted from WriteTo output.
}

func TestBuildDraftMessage_MultipleRecipients(t *testing.T) {
	t.Parallel()

	msg, err := tools.BuildDraftMessage(
		"sender@example.com",
		[]string{"one@example.com", "two@example.com"},
		nil,
		nil,
		"Multiple Recipients",
		"Test body",
		false,
		nil,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	content := string(msg)

	if !strings.Contains(content, "To:") {
		t.Error("message should contain To header")
	}
}

func TestBuildDraftMessage_WithAttachments(t *testing.T) {
	t.Parallel()

	// Create a temp file for attachment.
	dir := t.TempDir()
	attachPath := dir + "/data.csv"

	err := os.WriteFile(attachPath, []byte("col1,col2\na,b"), 0o644)
	if err != nil {
		t.Fatalf("write temp file: %v", err)
	}

	msg, err := tools.BuildDraftMessage(
		"sender@example.com",
		[]string{"to@example.com"},
		nil,
		nil,
		"Subject with Attachment",
		"See attached.",
		false,
		[]smtp.SendAttachment{{Path: attachPath, Filename: "report.csv"}},
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	raw := string(msg)

	// Should be a multipart message.
	if !strings.Contains(raw, "multipart") {
		t.Error("expected multipart MIME structure")
	}

	// Should contain the attachment filename.
	if !strings.Contains(raw, "report.csv") {
		t.Error("expected attachment filename 'report.csv' in message")
	}

	// Should contain the subject.
	if !strings.Contains(raw, "Subject with Attachment") {
		t.Error("expected subject in message")
	}

	// Should contain the body text.
	if !strings.Contains(raw, "See attached.") {
		t.Error("expected body text in message")
	}
}
