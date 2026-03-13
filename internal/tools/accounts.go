package tools

import (
	"context"

	"github.com/boutquin/mcp-server-email/internal/imap"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// AccountsTool returns the email_accounts tool definition.
func AccountsTool() mcp.Tool {
	return mcp.NewTool("email_accounts",
		mcp.WithDescription("List configured accounts with connection status"),
	)
}

// AccountsHandler returns the handler for email_accounts.
func AccountsHandler(ops imap.Operations) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		status := ops.AccountStatus()

		result := map[string]any{
			"accounts": status,
		}

		return jsonResult(result), nil
	}
}
