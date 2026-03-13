package resources_test

import (
	"testing"

	"github.com/boutquin/mcp-server-email/internal/resources"
)

func TestStatusResource(t *testing.T) {
	t.Parallel()

	resource := resources.StatusResource()

	if resource.URI != "email://status" {
		t.Errorf("expected URI 'email://status', got %q", resource.URI)
	}

	if resource.Name != "Email MCP Server Status" {
		t.Errorf("expected name 'Email MCP Server Status', got %q", resource.Name)
	}

	if resource.MIMEType != "application/json" {
		t.Errorf("expected MIME type 'application/json', got %q", resource.MIMEType)
	}
}

func TestVersion(t *testing.T) {
	t.Parallel()

	if resources.Version == "" {
		t.Error("Version should not be empty")
	}
}
