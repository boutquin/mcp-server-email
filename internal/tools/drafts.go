package tools

import (
	"context"

	"github.com/boutquin/mcp-server-email/internal/imap"
	"github.com/boutquin/mcp-server-email/internal/models"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

func DraftListTool() mcp.Tool {
	return mcp.NewTool("email_draft_list",
		mcp.WithDescription("List drafts from the account's special-use Drafts folder"),
		mcp.WithString("account", mcp.Description("Account ID (defaults to default account)")),
		mcp.WithNumber("limit", mcp.Description("Max results (default 50, max 500)")),
		mcp.WithNumber("offset", mcp.Description("Offset for pagination")),
		mcp.WithBoolean("includeBody", mcp.Description("Include message body (default false)")),
	)
}

func DraftListHandler(ops imap.Operations) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		accountID := resolveAccountID(req.GetString("account", ""), ops.DefaultAccountID())
		folder, err := ops.GetFolderByRole(ctx, accountID, models.RoleDrafts)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		limit := req.GetInt("limit", defaultLimit)
		if limit < 1 {
			limit = defaultLimit
		}
		if limit > maxLimit {
			limit = maxLimit
		}
		offset := req.GetInt("offset", 0)
		if offset < 0 {
			return mcp.NewToolResultError("offset must be non-negative"), nil
		}
		includeBody := req.GetBool("includeBody", false)
		messages, err := ops.ListMessages(ctx, accountID, folder, limit, offset, includeBody)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		if includeBody {
			convertHTMLBodies(messages)
		}
		return jsonResult(map[string]any{"account": accountID, "folder": folder, "messages": messages, "count": len(messages)}), nil
	}
}

func DraftUpdateTool() mcp.Tool {
	return mcp.NewTool("email_draft_update",
		mcp.WithDescription("Replace a draft with a complete RFC 5322/MIME message; returns a new message ID"),
		mcp.WithString("id", mcp.Description("Existing draft message ID"), mcp.Required()),
		mcp.WithString("message", mcp.Description("Complete replacement RFC 5322/MIME message"), mcp.Required()),
	)
}

func DraftUpdateHandler(ops imap.Operations) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		id, err := req.RequireString("id")
		if err != nil {
			return mcp.NewToolResultError("id is required"), nil
		}
		message, err := req.RequireString("message")
		if err != nil {
			return mcp.NewToolResultError("message is required"), nil
		}
		account, mailbox, uid, err := models.ParseMessageID(id)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		draftsFolder, err := ops.GetFolderByRole(ctx, account, models.RoleDrafts)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		if mailbox != draftsFolder {
			return mcp.NewToolResultError("message is not a draft"), nil
		}
		newUID, err := ops.UpdateDraft(ctx, account, uid, []byte(message))
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		return jsonResult(map[string]any{
			"success": true, "account": account,
			"id": models.FormatMessageID(account, draftsFolder, newUID), "replaced_id": id,
		}), nil
	}
}

func DraftDeleteTool() mcp.Tool {
	return mcp.NewTool("email_draft_delete",
		mcp.WithDescription("Delete a draft by message ID"),
		mcp.WithString("id", mcp.Description("Draft message ID"), mcp.Required()),
	)
}

func DraftDeleteHandler(ops imap.Operations) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		id, err := req.RequireString("id")
		if err != nil {
			return mcp.NewToolResultError("id is required"), nil
		}
		account, mailbox, uid, err := models.ParseMessageID(id)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		draftsFolder, err := ops.GetFolderByRole(ctx, account, models.RoleDrafts)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		if mailbox != draftsFolder {
			return mcp.NewToolResultError("message is not a draft"), nil
		}
		if err := ops.DeleteDraft(ctx, account, uid); err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		return jsonResult(map[string]any{"success": true, "account": account, "id": id}), nil
	}
}
