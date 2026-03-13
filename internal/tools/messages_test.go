package tools_test

import (
	"slices"
	"testing"

	"github.com/boutquin/mcp-server-email/internal/tools"
)

func TestListTool(t *testing.T) {
	t.Parallel()

	tool := tools.ListTool()

	if tool.Name != "email_list" {
		t.Errorf("expected tool name 'email_list', got %q", tool.Name)
	}

	if tool.Description == "" {
		t.Error("tool description should not be empty")
	}
}

func TestUnreadTool(t *testing.T) {
	t.Parallel()

	tool := tools.UnreadTool()

	if tool.Name != "email_unread" {
		t.Errorf("expected tool name 'email_unread', got %q", tool.Name)
	}

	if tool.Description == "" {
		t.Error("tool description should not be empty")
	}
}

func TestGetTool(t *testing.T) {
	t.Parallel()

	tool := tools.GetTool()

	if tool.Name != "email_get" {
		t.Errorf("expected tool name 'email_get', got %q", tool.Name)
	}

	if tool.Description == "" {
		t.Error("tool description should not be empty")
	}

	// Check that 'id' is required
	if !slices.Contains(tool.InputSchema.Required, "id") {
		t.Error("'id' should be a required parameter")
	}
}

func TestMoveTool(t *testing.T) {
	t.Parallel()

	tool := tools.MoveTool()

	if tool.Name != "email_move" {
		t.Errorf("expected tool name 'email_move', got %q", tool.Name)
	}

	// Check required params
	required := map[string]bool{"id": false, "destination": false}

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

func TestCopyTool(t *testing.T) {
	t.Parallel()

	tool := tools.CopyTool()

	if tool.Name != "email_copy" {
		t.Errorf("expected tool name 'email_copy', got %q", tool.Name)
	}

	// Check required params
	required := map[string]bool{"id": false, "destination": false}

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

func TestDeleteTool(t *testing.T) {
	t.Parallel()

	tool := tools.DeleteTool()

	if tool.Name != "email_delete" {
		t.Errorf("expected tool name 'email_delete', got %q", tool.Name)
	}

	// Check that 'id' is required
	if !slices.Contains(tool.InputSchema.Required, "id") {
		t.Error("'id' should be a required parameter")
	}
}

func TestMarkReadTool(t *testing.T) {
	t.Parallel()

	tool := tools.MarkReadTool()

	if tool.Name != "email_mark_read" {
		t.Errorf("expected tool name 'email_mark_read', got %q", tool.Name)
	}

	// Check required params
	required := map[string]bool{"id": false, "read": false}

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

func TestFlagTool(t *testing.T) {
	t.Parallel()

	tool := tools.FlagTool()

	if tool.Name != "email_flag" {
		t.Errorf("expected tool name 'email_flag', got %q", tool.Name)
	}

	// Check required params
	required := map[string]bool{"id": false, "flagged": false}

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
