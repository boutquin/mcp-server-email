package tools_test

import (
	"os"
	"strings"
	"testing"

	"github.com/boutquin/mcp-server-email/internal/smtp"
	"github.com/boutquin/mcp-server-email/internal/tools"
)

func TestBuildDraftMessage_WithCCAndBCCAndAttachments(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	attachPath := dir + "/file.txt"

	err := os.WriteFile(attachPath, []byte("content"), 0o644)
	if err != nil {
		t.Fatalf("write temp file: %v", err)
	}

	msg, err := tools.BuildDraftMessage(
		"sender@example.com",
		[]string{"to@example.com"},
		[]string{"cc@example.com"},
		[]string{"bcc@example.com"},
		"Subject",
		"Body text",
		false,
		[]smtp.SendAttachment{{Path: attachPath, Filename: "file.txt"}},
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(msg) == 0 {
		t.Fatal("expected non-empty message")
	}
}

func TestBuildDraftMessage_HTMLBodyWithAttachment(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	attachPath := dir + "/file.txt"

	err := os.WriteFile(attachPath, []byte("content"), 0o644)
	if err != nil {
		t.Fatalf("write temp file: %v", err)
	}

	msg, err := tools.BuildDraftMessage(
		"sender@example.com",
		[]string{"to@example.com"},
		nil,
		nil,
		"HTML Subject",
		"<h1>Hello</h1>",
		true,
		[]smtp.SendAttachment{{Path: attachPath, Filename: "file.txt"}},
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(msg) == 0 {
		t.Fatal("expected non-empty message")
	}
}

func TestBuildDraftMessage_MultipleAttachments(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path1 := dir + "/a.txt"
	path2 := dir + "/b.txt"

	for _, p := range []string{path1, path2} {
		err := os.WriteFile(p, []byte("data"), 0o644)
		if err != nil {
			t.Fatalf("write temp file: %v", err)
		}
	}

	msg, err := tools.BuildDraftMessage(
		"sender@example.com",
		[]string{"to@example.com"},
		nil,
		nil,
		"Multi Attach",
		"Body",
		false,
		[]smtp.SendAttachment{
			{Path: path1, Filename: "a.txt"},
			{Path: path2, Filename: "b.txt"},
		},
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(msg) == 0 {
		t.Fatal("expected non-empty message")
	}
}

func TestBuildDraftMessage_InvalidFrom(t *testing.T) {
	t.Parallel()

	_, err := tools.BuildDraftMessage(
		"not-an-email",
		[]string{"to@example.com"},
		nil,
		nil,
		"Subject",
		"Body",
		false,
		nil,
	)
	if err == nil {
		t.Fatal("expected error for invalid from address")
	}

	if !strings.Contains(err.Error(), "set from") {
		t.Errorf("expected 'set from' error, got %q", err.Error())
	}
}

func TestBuildDraftMessage_InvalidTo(t *testing.T) {
	t.Parallel()

	_, err := tools.BuildDraftMessage(
		"sender@example.com",
		[]string{"not-valid"},
		nil,
		nil,
		"Subject",
		"Body",
		false,
		nil,
	)
	if err == nil {
		t.Fatal("expected error for invalid to address")
	}

	if !strings.Contains(err.Error(), "set to") {
		t.Errorf("expected 'set to' error, got %q", err.Error())
	}
}

func TestBuildDraftMessage_InvalidCC(t *testing.T) {
	t.Parallel()

	_, err := tools.BuildDraftMessage(
		"sender@example.com",
		[]string{"to@example.com"},
		[]string{"not-valid"},
		nil,
		"Subject",
		"Body",
		false,
		nil,
	)
	if err == nil {
		t.Fatal("expected error for invalid cc address")
	}

	if !strings.Contains(err.Error(), "set cc") {
		t.Errorf("expected 'set cc' error, got %q", err.Error())
	}
}

func TestBuildDraftMessage_InvalidBCC(t *testing.T) {
	t.Parallel()

	_, err := tools.BuildDraftMessage(
		"sender@example.com",
		[]string{"to@example.com"},
		nil,
		[]string{"not-valid"},
		"Subject",
		"Body",
		false,
		nil,
	)
	if err == nil {
		t.Fatal("expected error for invalid bcc address")
	}

	if !strings.Contains(err.Error(), "set bcc") {
		t.Errorf("expected 'set bcc' error, got %q", err.Error())
	}
}

func TestBuildDraftMessage_EmptyTo(t *testing.T) {
	t.Parallel()

	// Empty to is valid (drafts may have no recipients).
	msg, err := tools.BuildDraftMessage(
		"sender@example.com",
		nil,
		nil,
		nil,
		"Draft Subject",
		"Draft Body",
		false,
		nil,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(msg) == 0 {
		t.Fatal("expected non-empty message")
	}
}
