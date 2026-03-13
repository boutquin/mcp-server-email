package tools

import (
	"context"

	"github.com/boutquin/mcp-server-email/internal/imap"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// FoldersTool returns the email_folders tool definition.
func FoldersTool() mcp.Tool {
	return mcp.NewTool("email_folders",
		mcp.WithDescription("List all folders for account"),
		mcp.WithString("account", mcp.Description("Account ID (defaults to default account)")),
	)
}

// FoldersHandler returns the handler for email_folders.
func FoldersHandler(ops imap.Operations) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		accountID := resolveAccountID(req.GetString("account", ""), ops.DefaultAccountID())

		folders, err := ops.ListFolders(ctx, accountID)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		return jsonResult(map[string]any{
			"account": accountID,
			"folders": folders,
		}), nil
	}
}

// FolderCreateTool returns the email_folder_create tool definition.
func FolderCreateTool() mcp.Tool {
	return mcp.NewTool("email_folder_create",
		mcp.WithDescription("Create new folder"),
		mcp.WithString("name", mcp.Description("Folder name"), mcp.Required()),
		mcp.WithString("account", mcp.Description("Account ID (defaults to default account)")),
	)
}

// FolderCreateHandler returns the handler for email_folder_create.
func FolderCreateHandler(ops imap.Operations) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		name, err := req.RequireString("name")
		if err != nil {
			return mcp.NewToolResultError("name is required"), nil
		}

		accountID := resolveAccountID(req.GetString("account", ""), ops.DefaultAccountID())

		err = ops.CreateFolder(ctx, accountID, name)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		return jsonResult(map[string]any{
			"success": true,
			"account": accountID,
			"folder":  name,
		}), nil
	}
}
