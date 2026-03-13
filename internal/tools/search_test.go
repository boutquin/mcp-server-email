package tools_test

import (
	"slices"
	"strings"
	"testing"

	"github.com/boutquin/mcp-server-email/internal/tools"
)

func TestSearchTool(t *testing.T) {
	t.Parallel()

	tool := tools.SearchTool()

	if tool.Name != "email_search" {
		t.Errorf("expected tool name 'email_search', got %q", tool.Name)
	}

	if tool.Description == "" {
		t.Error("tool description should not be empty")
	}

	// Check that 'query' is required
	if !slices.Contains(tool.InputSchema.Required, "query") {
		t.Error("'query' should be a required parameter")
	}
}

func TestSearchTool_SubjectAndBody(t *testing.T) {
	t.Parallel()

	tool := tools.SearchTool()

	// Tool description must indicate search covers both subject and body
	if !strings.Contains(strings.ToLower(tool.Description), "subject") ||
		!strings.Contains(strings.ToLower(tool.Description), "body") {
		t.Errorf("tool description should mention subject and body search, got %q", tool.Description)
	}
}
