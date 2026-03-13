package tools

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/boutquin/mcp-server-email/internal/imap"
	"github.com/boutquin/mcp-server-email/internal/models"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

const saveFilePerms = 0o644

// Attachment download errors.
var (
	errSavePathRequired  = errors.New("saveTo must be an absolute path")
	errSavePathTraversal = errors.New("saveTo must not contain path traversal (..)")
	errSaveParentMissing = errors.New("saveTo parent directory does not exist")
	errIndexRequired     = errors.New("index is required")
)

// AttachmentListTool returns the email_attachment_list tool definition.
func AttachmentListTool() mcp.Tool {
	return mcp.NewTool("email_attachment_list",
		mcp.WithDescription("List attachments for an email message"),
		mcp.WithString("id", mcp.Description("Message ID"), mcp.Required()),
	)
}

// AttachmentListHandler returns the handler for email_attachment_list.
func AttachmentListHandler(ops imap.Operations) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		id, err := req.RequireString("id")
		if err != nil {
			return mcp.NewToolResultError("id is required"), nil
		}

		account, mailbox, uid, errResult := parseMessageParams(id)
		if errResult != nil {
			return errResult, nil
		}

		attachments, err := ops.GetAttachments(ctx, account, mailbox, uid)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		return attachmentListResult(attachments), nil
	}
}

// attachmentListResult marshals attachment metadata to a tool result.
func attachmentListResult(attachments []models.AttachmentInfo) *mcp.CallToolResult {
	b, _ := json.MarshalIndent(attachments, "", "  ") //nolint:errchkjson // model structs are JSON-safe

	return mcp.NewToolResultText(string(b))
}

// AttachmentGetTool returns the email_attachment_get tool definition.
func AttachmentGetTool() mcp.Tool {
	return mcp.NewTool("email_attachment_get",
		mcp.WithDescription("Download an email attachment by index"),
		mcp.WithString("id", mcp.Description("Message ID"), mcp.Required()),
		mcp.WithNumber("index", mcp.Description("Attachment index (from attachment list)"), mcp.Required()),
		mcp.WithString("saveTo", mcp.Description("Absolute file path to save attachment (optional)")),
	)
}

// AttachmentGetHandler returns the handler for email_attachment_get.
func AttachmentGetHandler(ops imap.Operations, limits AttachmentLimits) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		id, err := req.RequireString("id")
		if err != nil {
			return mcp.NewToolResultError("id is required"), nil
		}

		indexFloat, ok := req.GetArguments()["index"].(float64)
		if !ok {
			return mcp.NewToolResultError(errIndexRequired.Error()), nil
		}

		index := int(indexFloat)

		account, mailbox, uid, errResult := parseMessageParams(id)
		if errResult != nil {
			return errResult, nil
		}

		data, filename, err := ops.GetAttachment(ctx, account, mailbox, uid, index)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		if int64(len(data)) > limits.MaxDownloadSizeBytes {
			return mcp.NewToolResultError(fmt.Sprintf(
				"attachment size %d bytes exceeds %d MB download limit",
				len(data), limits.MaxDownloadSizeBytes/bytesPerMB,
			)), nil
		}

		saveTo := req.GetString("saveTo", "")
		if saveTo != "" {
			return handleSaveAttachment(data, filename, saveTo)
		}

		return handleReturnAttachment(data, filename)
	}
}

// handleSaveAttachment validates the save path and writes the attachment to disk.
func handleSaveAttachment(data []byte, filename, saveTo string) (*mcp.CallToolResult, error) {
	err := validateSavePath(saveTo)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	err = os.WriteFile(saveTo, data, saveFilePerms)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("write file: %s", err)), nil
	}

	return jsonResult(map[string]any{
		"saved":    saveTo,
		"filename": filename,
		"size":     len(data),
	}), nil
}

// handleReturnAttachment base64-encodes the attachment and returns it inline.
func handleReturnAttachment(data []byte, filename string) (*mcp.CallToolResult, error) {
	encoded := base64.StdEncoding.EncodeToString(data)

	return jsonResult(map[string]any{
		"filename": filename,
		"size":     len(data),
		"content":  encoded,
	}), nil
}

// validateSavePath checks that the save path is absolute, has no traversal, and parent exists.
func validateSavePath(saveTo string) error {
	if !filepath.IsAbs(saveTo) {
		return errSavePathRequired
	}

	if strings.Contains(saveTo, "..") {
		return errSavePathTraversal
	}

	parent := filepath.Dir(saveTo)

	info, err := os.Stat(parent)
	if err != nil || !info.IsDir() {
		return errSaveParentMissing
	}

	return nil
}
