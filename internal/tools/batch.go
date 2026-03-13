package tools

import (
	"context"
	"errors"
	"fmt"

	"github.com/boutquin/mcp-server-email/internal/imap"
	"github.com/boutquin/mcp-server-email/internal/models"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

const (
	actionMove     = "move"
	actionDelete   = "delete"
	actionMarkRead = "mark_read"
	actionFlag     = "flag"
)

// errUnsupportedAction is returned when an unknown batch action is requested.
var errUnsupportedAction = errors.New("unsupported batch action")

// BatchTool returns the email_batch tool definition.
func BatchTool() mcp.Tool {
	return mcp.NewTool("email_batch",
		mcp.WithDescription("Batch operations on multiple messages (move, delete, mark_read, flag)"),
		mcp.WithString("action", mcp.Description("Action: move, delete, mark_read, flag"), mcp.Required()),
		mcp.WithArray("ids",
			mcp.Description("Array of message IDs"),
			mcp.Required(),
			mcp.Items(map[string]any{"type": "string"}),
		),
		mcp.WithString("destination", mcp.Description("Target folder for move action")),
		mcp.WithBoolean("permanent", mcp.Description("Permanently delete (default: move to Trash)")),
		mcp.WithBoolean("read", mcp.Description("Mark as read (true) or unread (false, default true)")),
		mcp.WithBoolean("flagged", mcp.Description("Set flagged status (default true)")),
	)
}

// BatchHandler returns the handler for email_batch.
func BatchHandler(ops imap.Operations) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		action, err := req.RequireString("action")
		if err != nil {
			return mcp.NewToolResultError("action is required"), nil
		}

		if errResult := validateBatchAction(action); errResult != nil {
			return errResult, nil
		}

		ids := extractStringArray(req)
		if len(ids) == 0 {
			return mcp.NewToolResultError("ids must be a non-empty array"), nil
		}

		if errResult := validateBatchParams(action, req); errResult != nil {
			return errResult, nil
		}

		results := executeBatch(ctx, ops, action, ids, req)

		return buildBatchResponse(results), nil
	}
}

// batchResult holds the outcome of a single batch item.
type batchResult struct {
	ID      string `json:"id"`
	Success bool   `json:"success"`
	Error   string `json:"error,omitempty"`
}

func validateBatchAction(action string) *mcp.CallToolResult {
	switch action {
	case actionMove, actionDelete, actionMarkRead, actionFlag:
		return nil
	default:
		msg := fmt.Sprintf("invalid action %q: must be move, delete, mark_read, or flag", action)

		return mcp.NewToolResultError(msg)
	}
}

func validateBatchParams(action string, req mcp.CallToolRequest) *mcp.CallToolResult {
	if action == actionMove {
		dest := req.GetString("destination", "")
		if dest == "" {
			return mcp.NewToolResultError("destination is required for move action")
		}
	}

	return nil
}

func extractStringArray(req mcp.CallToolRequest) []string {
	args, ok := req.Params.Arguments.(map[string]any)
	if !ok {
		return nil
	}

	raw, ok := args["ids"]
	if !ok {
		return nil
	}

	arr, ok := raw.([]any)
	if !ok {
		return nil
	}

	result := make([]string, 0, len(arr))

	for _, item := range arr {
		s, ok := item.(string)
		if ok {
			result = append(result, s)
		}
	}

	return result
}

func executeBatch(
	ctx context.Context, ops imap.Operations,
	action string, ids []string, req mcp.CallToolRequest,
) []batchResult {
	results := make([]batchResult, 0, len(ids))

	for _, id := range ids {
		err := dispatchAction(ctx, ops, action, id, req)

		br := batchResult{ID: id, Success: err == nil}
		if err != nil {
			br.Error = err.Error()
		}

		results = append(results, br)
	}

	return results
}

func dispatchAction(
	ctx context.Context, ops imap.Operations,
	action, id string, req mcp.CallToolRequest,
) error {
	account, mailbox, uid, err := models.ParseMessageID(id)
	if err != nil {
		return fmt.Errorf("parse ID: %w", err)
	}

	return dispatchParsed(ctx, ops, action, account, mailbox, uid, req)
}

func dispatchParsed(
	ctx context.Context, ops imap.Operations,
	action, account, mailbox string, uid uint32, req mcp.CallToolRequest,
) error {
	switch action {
	case actionMove:
		err := ops.MoveMessage(ctx, account, mailbox, uid, req.GetString("destination", ""))
		if err != nil {
			return fmt.Errorf("move: %w", err)
		}

		return nil
	case actionDelete:
		err := ops.DeleteMessage(ctx, account, mailbox, uid, req.GetBool("permanent", false))
		if err != nil {
			return fmt.Errorf("delete: %w", err)
		}

		return nil
	case actionMarkRead:
		err := ops.MarkRead(ctx, account, mailbox, uid, req.GetBool("read", true))
		if err != nil {
			return fmt.Errorf("mark_read: %w", err)
		}

		return nil
	case actionFlag:
		err := ops.SetFlag(ctx, account, mailbox, uid, req.GetBool("flagged", true))
		if err != nil {
			return fmt.Errorf("flag: %w", err)
		}

		return nil
	default:
		return fmt.Errorf("%w: %s", errUnsupportedAction, action)
	}
}

func buildBatchResponse(results []batchResult) *mcp.CallToolResult {
	succeeded := 0
	failed := 0

	for _, r := range results {
		if r.Success {
			succeeded++
		} else {
			failed++
		}
	}

	return jsonResult(map[string]any{
		"results": results,
		"summary": map[string]any{
			"total":     len(results),
			"succeeded": succeeded,
			"failed":    failed,
		},
	})
}
