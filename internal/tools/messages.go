package tools

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/boutquin/mcp-server-email/internal/imap"
	"github.com/boutquin/mcp-server-email/internal/models"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"golang.org/x/net/html"
)

const (
	defaultLimit     = 50
	maxLimit         = 500
	defaultPreview   = 500
	defaultBodyLimit = 10000
)

// ListTool returns the email_list tool definition.
func ListTool() mcp.Tool {
	return mcp.NewTool("email_list",
		mcp.WithDescription("List messages in folder"),
		mcp.WithString("account", mcp.Description("Account ID (defaults to default account)")),
		mcp.WithString("folder", mcp.Description("Folder name (default: INBOX)")),
		mcp.WithNumber("limit", mcp.Description("Max results (default 50, max 500)")),
		mcp.WithNumber("offset", mcp.Description("Offset for pagination")),
		mcp.WithBoolean("includeBody", mcp.Description("Include message body (default false)")),
	)
}

// ListHandler returns the handler for email_list.
func ListHandler(ops imap.Operations) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		accountID := resolveAccountID(req.GetString("account", ""), ops.DefaultAccountID())
		folder := req.GetString("folder", "INBOX")
		limit := req.GetInt("limit", defaultLimit)
		offset := req.GetInt("offset", 0)
		includeBody := req.GetBool("includeBody", false)

		if limit > maxLimit {
			limit = maxLimit
		}

		if limit < 1 {
			limit = defaultLimit
		}

		emails, err := ops.ListMessages(ctx, accountID, folder, limit, offset, includeBody)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		if includeBody {
			convertHTMLBodies(emails)
		}

		return jsonResult(map[string]any{
			"account":  accountID,
			"folder":   folder,
			"messages": emails,
			"count":    len(emails),
		}), nil
	}
}

// UnreadTool returns the email_unread tool definition.
func UnreadTool() mcp.Tool {
	return mcp.NewTool("email_unread",
		mcp.WithDescription("List unread messages"),
		mcp.WithString("account", mcp.Description("Account ID (defaults to default account)")),
		mcp.WithString("folder", mcp.Description("Folder name (default: INBOX)")),
		mcp.WithNumber("limit", mcp.Description("Max results (default 50, max 500)")),
		mcp.WithBoolean("includeBody", mcp.Description("Include message body (default false)")),
	)
}

// UnreadHandler returns the handler for email_unread.
func UnreadHandler(ops imap.Operations) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		accountID := resolveAccountID(req.GetString("account", ""), ops.DefaultAccountID())
		folder := req.GetString("folder", "INBOX")
		limit := req.GetInt("limit", defaultLimit)
		includeBody := req.GetBool("includeBody", false)

		if limit > maxLimit {
			limit = maxLimit
		}

		if limit < 1 {
			limit = defaultLimit
		}

		emails, err := ops.ListUnread(ctx, accountID, folder, limit, includeBody)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		if includeBody {
			convertHTMLBodies(emails)
		}

		return jsonResult(map[string]any{
			"account":  accountID,
			"folder":   folder,
			"messages": emails,
			"count":    len(emails),
		}), nil
	}
}

// GetTool returns the email_get tool definition.
func GetTool() mcp.Tool {
	return mcp.NewTool("email_get",
		mcp.WithDescription("Get full message by ID"),
		mcp.WithString("id", mcp.Description("Message ID"), mcp.Required()),
		mcp.WithNumber("previewLength", mcp.Description("Max chars for body_preview (default 500, 0 to disable)")),
	)
}

// GetHandler returns the handler for email_get.
func GetHandler(ops imap.Operations) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		id, err := req.RequireString("id")
		if err != nil {
			return mcp.NewToolResultError("id is required"), nil
		}

		account, mailbox, uid, err := models.ParseMessageID(id)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		email, err := ops.GetMessage(ctx, account, mailbox, uid)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		if email.Body != "" && strings.Contains(email.ContentType, "text/html") {
			email.Body = htmlToText(email.Body)
		}

		previewLength := req.GetInt("previewLength", defaultPreview)

		return marshalGetResult(email, previewLength)
	}
}

// marshalGetResult marshals the email with optional body preview fields.
func marshalGetResult(email *models.Email, previewLength int) (*mcp.CallToolResult, error) {
	result := make(map[string]any)

	b, err := json.Marshal(email)
	if err != nil {
		return mcp.NewToolResultError("internal: failed to marshal message"), nil
	}

	err = json.Unmarshal(b, &result)
	if err != nil {
		return mcp.NewToolResultError("internal: failed to process message"), nil
	}

	if email.Body != "" && previewLength > 0 {
		result["body_total_length"] = len(email.Body)

		preview := email.Body
		if len(preview) > previewLength {
			preview = preview[:previewLength]
		}

		result["body_preview"] = preview
	}

	out, _ := json.MarshalIndent(result, "", "  ") //nolint:errchkjson // map values are JSON-safe types

	return mcp.NewToolResultText(string(out)), nil
}

// MoveTool returns the email_move tool definition.
func MoveTool() mcp.Tool {
	return mcp.NewTool("email_move",
		mcp.WithDescription("Move message to folder"),
		mcp.WithString("id", mcp.Description("Message ID"), mcp.Required()),
		mcp.WithString("destination", mcp.Description("Destination folder"), mcp.Required()),
	)
}

// parseMessageParams extracts message ID components.
func parseMessageParams(id string) (string, string, uint32, *mcp.CallToolResult) {
	account, mailbox, uid, err := models.ParseMessageID(id)
	if err != nil {
		return "", "", 0, mcp.NewToolResultError(err.Error())
	}

	return account, mailbox, uid, nil
}

// jsonResult marshals the result map to a tool result.
// Map values are JSON-safe types (strings, bools, ints, model structs) that cannot fail marshaling.
func jsonResult(result map[string]any) *mcp.CallToolResult {
	b, _ := json.MarshalIndent(result, "", "  ") //nolint:errchkjson // map values are JSON-safe types

	return mcp.NewToolResultText(string(b))
}

// htmlToText converts HTML content to plain text by stripping tags.
// Returns the original string unchanged if parsing fails.
func htmlToText(s string) string {
	doc, err := html.Parse(strings.NewReader(s))
	if err != nil {
		return s
	}

	var b strings.Builder

	extractText(doc, &b)

	return strings.TrimSpace(b.String())
}

// extractText recursively walks the HTML node tree and extracts text content.
// Block-level elements get newline separation for readability.
func extractText(n *html.Node, b *strings.Builder) {
	if n.Type == html.TextNode {
		text := strings.TrimSpace(n.Data)
		if text != "" {
			if b.Len() > 0 {
				b.WriteByte(' ')
			}

			b.WriteString(text)
		}

		return
	}

	// Skip script and style elements
	if n.Type == html.ElementNode && (n.Data == "script" || n.Data == "style") {
		return
	}

	// Add newlines before block-level elements
	isBlock := n.Type == html.ElementNode && isBlockElement(n.Data)
	if isBlock && b.Len() > 0 {
		b.WriteByte('\n')
	}

	for c := n.FirstChild; c != nil; c = c.NextSibling {
		extractText(c, b)
	}

	if isBlock && b.Len() > 0 {
		b.WriteByte('\n')
	}
}

func isBlockElement(tag string) bool {
	switch tag {
	case "div", "p", "h1", "h2", "h3", "h4", "h5", "h6",
		"br", "hr", "li", "ol", "ul", "table", "tr",
		"blockquote", "pre", "section", "article", "header", "footer":
		return true
	default:
		return false
	}
}

// convertHTMLBodies converts HTML bodies to plain text in a slice of emails.
func convertHTMLBodies(emails []models.Email) {
	for i := range emails {
		if emails[i].Body != "" && strings.Contains(emails[i].ContentType, "text/html") {
			emails[i].Body = htmlToText(emails[i].Body)
		}
	}
}

// resolveAccountID returns accountID if non-empty, otherwise returns defaultID.
func resolveAccountID(accountID, defaultID string) string {
	if accountID != "" {
		return accountID
	}

	return defaultID
}

// MoveHandler returns the handler for email_move.
func MoveHandler(ops imap.Operations) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		id, err := req.RequireString("id")
		if err != nil {
			return mcp.NewToolResultError("id is required"), nil
		}

		destination, err := req.RequireString("destination")
		if err != nil {
			return mcp.NewToolResultError("destination is required"), nil
		}

		account, mailbox, uid, errResult := parseMessageParams(id)
		if errResult != nil {
			return errResult, nil
		}

		err = ops.MoveMessage(ctx, account, mailbox, uid, destination)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		return jsonResult(map[string]any{
			"success":     true,
			"id":          id,
			"destination": destination,
		}), nil
	}
}

// CopyTool returns the email_copy tool definition.
func CopyTool() mcp.Tool {
	return mcp.NewTool("email_copy",
		mcp.WithDescription("Copy message to folder"),
		mcp.WithString("id", mcp.Description("Message ID"), mcp.Required()),
		mcp.WithString("destination", mcp.Description("Destination folder"), mcp.Required()),
	)
}

// CopyHandler returns the handler for email_copy.
func CopyHandler(ops imap.Operations) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		id, err := req.RequireString("id")
		if err != nil {
			return mcp.NewToolResultError("id is required"), nil
		}

		destination, err := req.RequireString("destination")
		if err != nil {
			return mcp.NewToolResultError("destination is required"), nil
		}

		account, mailbox, uid, errResult := parseMessageParams(id)
		if errResult != nil {
			return errResult, nil
		}

		err = ops.CopyMessage(ctx, account, mailbox, uid, destination)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		return jsonResult(map[string]any{
			"success":     true,
			"id":          id,
			"destination": destination,
		}), nil
	}
}

// DeleteTool returns the email_delete tool definition.
func DeleteTool() mcp.Tool {
	return mcp.NewTool("email_delete",
		mcp.WithDescription("Delete message (trash or expunge)"),
		mcp.WithString("id", mcp.Description("Message ID"), mcp.Required()),
		mcp.WithBoolean("permanent", mcp.Description("Permanently delete (default: move to Trash)")),
	)
}

// DeleteHandler returns the handler for email_delete.
func DeleteHandler(ops imap.Operations) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		id, err := req.RequireString("id")
		if err != nil {
			return mcp.NewToolResultError("id is required"), nil
		}

		permanent := req.GetBool("permanent", false)

		account, mailbox, uid, errResult := parseMessageParams(id)
		if errResult != nil {
			return errResult, nil
		}

		err = ops.DeleteMessage(ctx, account, mailbox, uid, permanent)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		return jsonResult(map[string]any{
			"success":   true,
			"id":        id,
			"permanent": permanent,
		}), nil
	}
}

// MarkReadTool returns the email_mark_read tool definition.
func MarkReadTool() mcp.Tool {
	return mcp.NewTool("email_mark_read",
		mcp.WithDescription("Mark message as read/unread"),
		mcp.WithString("id", mcp.Description("Message ID"), mcp.Required()),
		mcp.WithBoolean("read", mcp.Description("Mark as read (true) or unread (false)"), mcp.Required()),
	)
}

// MarkReadHandler returns the handler for email_mark_read.
func MarkReadHandler(ops imap.Operations) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		id, err := req.RequireString("id")
		if err != nil {
			return mcp.NewToolResultError("id is required"), nil
		}

		read := req.GetBool("read", true)

		account, mailbox, uid, errResult := parseMessageParams(id)
		if errResult != nil {
			return errResult, nil
		}

		err = ops.MarkRead(ctx, account, mailbox, uid, read)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		return jsonResult(map[string]any{
			"success": true,
			"id":      id,
			"read":    read,
		}), nil
	}
}

// FlagTool returns the email_flag tool definition.
func FlagTool() mcp.Tool {
	return mcp.NewTool("email_flag",
		mcp.WithDescription("Flag/unflag message"),
		mcp.WithString("id", mcp.Description("Message ID"), mcp.Required()),
		mcp.WithBoolean("flagged", mcp.Description("Set flagged status"), mcp.Required()),
	)
}

// FlagHandler returns the handler for email_flag.
func FlagHandler(ops imap.Operations) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		id, err := req.RequireString("id")
		if err != nil {
			return mcp.NewToolResultError("id is required"), nil
		}

		flagged := req.GetBool("flagged", true)

		account, mailbox, uid, errResult := parseMessageParams(id)
		if errResult != nil {
			return errResult, nil
		}

		err = ops.SetFlag(ctx, account, mailbox, uid, flagged)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		return jsonResult(map[string]any{
			"success": true,
			"id":      id,
			"flagged": flagged,
		}), nil
	}
}

// ReadBodyTool returns the email_read_body tool definition.
func ReadBodyTool() mcp.Tool {
	return mcp.NewTool("email_read_body",
		mcp.WithDescription("Read email body content with pagination"),
		mcp.WithString("id", mcp.Description("Message ID"), mcp.Required()),
		mcp.WithNumber("offset", mcp.Description("Character offset into body (default 0)")),
		mcp.WithNumber("limit", mcp.Description("Max characters to return (default 10000)")),
		mcp.WithString("format", mcp.Description("Output format: text or raw_html (default text)")),
	)
}

// ReadBodyHandler returns the handler for email_read_body.
func ReadBodyHandler(ops imap.Operations) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		id, err := req.RequireString("id")
		if err != nil {
			return mcp.NewToolResultError("id is required"), nil
		}

		account, mailbox, uid, errResult := parseMessageParams(id)
		if errResult != nil {
			return errResult, nil
		}

		email, err := ops.GetMessage(ctx, account, mailbox, uid)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		return buildReadBodyResult(req, email), nil
	}
}

// buildReadBodyResult extracts, converts, and slices the email body.
func buildReadBodyResult(req mcp.CallToolRequest, email *models.Email) *mcp.CallToolResult {
	format := req.GetString("format", "text")
	offset := req.GetInt("offset", 0)
	limit := req.GetInt("limit", defaultBodyLimit)

	body := email.Body
	if format == "text" && strings.Contains(email.ContentType, "text/html") {
		body = htmlToText(body)
	}

	totalLength := len(body)

	if offset > totalLength {
		offset = totalLength
	}

	if offset < 0 {
		offset = 0
	}

	end := min(offset+limit, totalLength)

	content := body[offset:end]

	remaining := totalLength - offset - len(content)

	return jsonResult(map[string]any{
		"content":      content,
		"total_length": totalLength,
		"offset":       offset,
		"limit":        limit,
		"remaining":    remaining,
		"is_complete":  remaining == 0,
	})
}
