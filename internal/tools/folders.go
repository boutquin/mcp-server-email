package tools

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/boutquin/mcp-server-email/internal/imap"
	"github.com/boutquin/mcp-server-email/internal/models"
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

func FolderRenameTool() mcp.Tool {
	return mcp.NewTool("email_folder_rename",
		mcp.WithDescription("Rename an email folder"),
		mcp.WithString("name", mcp.Description("Current folder name"), mcp.Required()),
		mcp.WithString("newName", mcp.Description("New folder name"), mcp.Required()),
		mcp.WithString("account", mcp.Description("Account ID (defaults to default account)")),
	)
}

func FolderRenameHandler(ops imap.Operations) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		name, err := req.RequireString("name")
		if err != nil {
			return mcp.NewToolResultError("name is required"), nil
		}
		newName, err := req.RequireString("newName")
		if err != nil {
			return mcp.NewToolResultError("newName is required"), nil
		}
		accountID := resolveAccountID(req.GetString("account", ""), ops.DefaultAccountID())
		if err := ops.RenameFolder(ctx, accountID, name, newName); err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		return jsonResult(map[string]any{"success": true, "account": accountID, "folder": newName, "previous_folder": name}), nil
	}
}

func FolderDeleteTool() mcp.Tool {
	return mcp.NewTool("email_folder_delete",
		mcp.WithDescription("Delete an email folder"),
		mcp.WithString("name", mcp.Description("Folder name"), mcp.Required()),
		mcp.WithString("account", mcp.Description("Account ID (defaults to default account)")),
	)
}

func FolderDeleteHandler(ops imap.Operations) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		name, err := req.RequireString("name")
		if err != nil {
			return mcp.NewToolResultError("name is required"), nil
		}
		accountID := resolveAccountID(req.GetString("account", ""), ops.DefaultAccountID())
		if err := protectSystemFolder(ctx, ops, accountID, name); err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		if err := ops.DeleteFolder(ctx, accountID, name); err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		return jsonResult(map[string]any{"success": true, "account": accountID, "folder": name}), nil
	}
}

func protectSystemFolder(ctx context.Context, ops imap.Operations, accountID, name string) error {
	requested := strings.TrimSpace(name)
	for _, protected := range []string{"INBOX", "Drafts", "Sent", "Trash"} {
		if strings.EqualFold(requested, protected) {
			return fmt.Errorf("cannot delete protected folder %q", name)
		}
	}

	for _, role := range []models.FolderRole{models.RoleDrafts, models.RoleSent, models.RoleTrash} {
		resolved, err := ops.GetFolderByRole(ctx, accountID, role)
		if err != nil {
			if errors.Is(err, models.ErrFolderRoleNotFound) {
				continue
			}
			return fmt.Errorf("resolve protected folder %s: %w", role, err)
		}
		if strings.EqualFold(requested, strings.TrimSpace(resolved)) {
			return fmt.Errorf("cannot delete protected folder %q (%s)", name, role)
		}
	}
	return nil
}
