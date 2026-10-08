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
	errSaveExists        = errors.New("saveTo target already exists")
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
	resolvedSaveTo, err := resolveSavePath(saveTo)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	file, err := os.OpenFile(resolvedSaveTo, os.O_WRONLY|os.O_CREATE|os.O_EXCL, saveFilePerms)
	if err != nil {
		if os.IsExist(err) {
			return mcp.NewToolResultError(errSaveExists.Error()), nil
		}
		return mcp.NewToolResultError(fmt.Sprintf("write file: %s", err)), nil
	}
	_, writeErr := file.Write(data)
	closeErr := file.Close()
	if writeErr != nil {
		_ = os.Remove(resolvedSaveTo)
		return mcp.NewToolResultError(fmt.Sprintf("write file: %s", writeErr)), nil
	}
	if closeErr != nil {
		_ = os.Remove(resolvedSaveTo)
		return mcp.NewToolResultError(fmt.Sprintf("write file: %s", closeErr)), nil
	}

	return jsonResult(map[string]any{
		"saved":    resolvedSaveTo,
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
	_, err := resolveSavePath(saveTo)
	return err
}

func resolveSavePath(saveTo string) (string, error) {
	if !filepath.IsAbs(saveTo) {
		return "", errSavePathRequired
	}

	// Check both slash styles regardless of the host OS so Windows-style
	// traversal cannot bypass validation when separators are mixed.
	portablePath := strings.ReplaceAll(saveTo, `\`, "/")
	for _, component := range strings.Split(portablePath, "/") {
		if component == ".." {
			return "", errSavePathTraversal
		}
	}
	clean := filepath.Clean(saveTo)
	parent := filepath.Dir(clean)
	if err := rejectSymlinkPath(parent, false); err != nil {
		return "", err
	}
	realParent, err := filepath.EvalSymlinks(parent)
	if err != nil {
		return "", errSaveParentMissing
	}
	info, err := os.Stat(realParent)
	if err != nil || !info.IsDir() {
		return "", errSaveParentMissing
	}
	if err := rejectSymlinkPath(clean, true); err != nil {
		return "", err
	}
	realTarget := filepath.Join(realParent, filepath.Base(clean))
	return realTarget, nil
}
