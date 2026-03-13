package tools

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"mime"
	"net/textproto"
	"strings"

	"github.com/boutquin/mcp-server-email/internal/imap"
	"github.com/boutquin/mcp-server-email/internal/models"
	"github.com/boutquin/mcp-server-email/internal/smtp"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/wneessen/go-mail"
)

// Send parameter validation errors.
var (
	errToRequired      = errors.New("to is required")
	errSubjectRequired = errors.New("subject is required")
	errBodyRequired    = errors.New("body is required")
)

// SendTool returns the email_send tool definition.
func SendTool() mcp.Tool {
	return mcp.NewTool("email_send",
		mcp.WithDescription("Send email via SMTP"),
		mcp.WithString("to", mcp.Description("Recipient email address(es), comma-separated"), mcp.Required()),
		mcp.WithString("subject", mcp.Description("Email subject"), mcp.Required()),
		mcp.WithString("body", mcp.Description("Email body"), mcp.Required()),
		mcp.WithString("account", mcp.Description("Account ID (defaults to default account)")),
		mcp.WithString("cc", mcp.Description("CC recipients, comma-separated")),
		mcp.WithString("bcc", mcp.Description("BCC recipients, comma-separated")),
		mcp.WithString("replyTo", mcp.Description("Reply-To address")),
		mcp.WithBoolean("isHtml", mcp.Description("Body is HTML (default false)")),
		mcp.WithArray("attachments",
			mcp.Description("File attachments (array of {path, filename?, content_type?})"),
			mcp.Items(map[string]any{
				"type": "object",
				"properties": map[string]any{
					"path":         map[string]any{"type": "string", "description": "Absolute file path"},
					"filename":     map[string]any{"type": "string", "description": "Override display filename"},
					"content_type": map[string]any{"type": "string", "description": "MIME type (auto-detected if omitted)"},
				},
				"required": []string{"path"},
			}),
		),
	)
}

// buildSendRequest constructs an smtp.SendRequest from tool call parameters.
func buildSendRequest(req mcp.CallToolRequest, limits AttachmentLimits) (*smtp.SendRequest, error) {
	to, err := req.RequireString("to")
	if err != nil {
		return nil, errToRequired
	}

	subject, err := req.RequireString("subject")
	if err != nil {
		return nil, errSubjectRequired
	}

	body, err := req.RequireString("body")
	if err != nil {
		return nil, errBodyRequired
	}

	attachments, err := parseAttachments(req, limits.MaxFileSizeBytes, limits.MaxTotalSizeBytes)
	if err != nil {
		return nil, err
	}

	sendReq := &smtp.SendRequest{
		To:          SplitAddresses(to),
		Subject:     subject,
		Body:        body,
		IsHTML:      req.GetBool("isHtml", false),
		ReplyTo:     req.GetString("replyTo", ""),
		Attachments: attachments,
	}

	if cc := req.GetString("cc", ""); cc != "" {
		sendReq.CC = SplitAddresses(cc)
	}

	if bcc := req.GetString("bcc", ""); bcc != "" {
		sendReq.BCC = SplitAddresses(bcc)
	}

	return sendReq, nil
}

// SendHandler returns the handler for email_send.
func SendHandler(ops smtp.Operations, limits AttachmentLimits) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		sendReq, err := buildSendRequest(req, limits)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		accountID := resolveAccountID(req.GetString("account", ""), ops.DefaultAccountID())

		fromEmail, err := ops.AccountEmail(accountID)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		err = ops.Send(ctx, accountID, sendReq)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		return jsonResult(map[string]any{
			"success": true,
			"account": accountID,
			"from":    fromEmail,
			"to":      sendReq.To,
		}), nil
	}
}

// DraftCreateTool returns the email_draft_create tool definition.
func DraftCreateTool() mcp.Tool {
	return mcp.NewTool("email_draft_create",
		mcp.WithDescription("Save as draft"),
		mcp.WithString("account", mcp.Description("Account ID (defaults to default account)")),
		mcp.WithString("to", mcp.Description("Recipient email address(es), comma-separated")),
		mcp.WithString("subject", mcp.Description("Email subject")),
		mcp.WithString("body", mcp.Description("Email body")),
		mcp.WithString("cc", mcp.Description("CC recipients, comma-separated")),
		mcp.WithString("bcc", mcp.Description("BCC recipients, comma-separated")),
		mcp.WithBoolean("isHtml", mcp.Description("Body is HTML (default false)")),
		mcp.WithArray("attachments",
			mcp.Description("File attachments (array of {path, filename?, content_type?})"),
			mcp.Items(map[string]any{
				"type": "object",
				"properties": map[string]any{
					"path":         map[string]any{"type": "string", "description": "Absolute file path"},
					"filename":     map[string]any{"type": "string", "description": "Override display filename"},
					"content_type": map[string]any{"type": "string", "description": "MIME type (auto-detected if omitted)"},
				},
				"required": []string{"path"},
			}),
		),
	)
}

// DraftCreateHandler returns the handler for email_draft_create.
func DraftCreateHandler(
	imapOps imap.Operations, smtpOps smtp.Operations, limits AttachmentLimits,
) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		accountID := resolveAccountID(req.GetString("account", ""), imapOps.DefaultAccountID())
		to := req.GetString("to", "")
		subject := req.GetString("subject", "")
		body := req.GetString("body", "")
		cc := req.GetString("cc", "")
		bcc := req.GetString("bcc", "")
		isHTML := req.GetBool("isHtml", false)

		attachments, err := parseAttachments(req, limits.MaxFileSizeBytes, limits.MaxTotalSizeBytes)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		// Get the from email address for the account
		fromEmail, err := smtpOps.AccountEmail(accountID)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		// Build the message
		var toAddrs, ccAddrs, bccAddrs []string
		if to != "" {
			toAddrs = SplitAddresses(to)
		}

		if cc != "" {
			ccAddrs = SplitAddresses(cc)
		}

		if bcc != "" {
			bccAddrs = SplitAddresses(bcc)
		}

		literal, err := buildDraftMessage(fromEmail, toAddrs, ccAddrs, bccAddrs, subject, body, isHTML, attachments)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		// Save the draft via IMAP
		uid, err := imapOps.SaveDraft(ctx, accountID, literal)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		draftID := models.FormatMessageID(accountID, "Drafts", uid)

		return jsonResult(map[string]any{
			"success": true,
			"account": accountID,
			"id":      draftID,
		}), nil
	}
}

// DraftSendTool returns the email_draft_send tool definition.
func DraftSendTool() mcp.Tool {
	return mcp.NewTool("email_draft_send",
		mcp.WithDescription("Send existing draft"),
		mcp.WithString("id", mcp.Description("Draft message ID"), mcp.Required()),
	)
}

// DraftSendHandler returns the handler for email_draft_send.
func DraftSendHandler(imapOps imap.Operations, smtpOps smtp.Operations) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		id, err := req.RequireString("id")
		if err != nil {
			return mcp.NewToolResultError("id is required"), nil
		}

		account, mailbox, uid, err := models.ParseMessageID(id)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		if mailbox != "Drafts" {
			return mcp.NewToolResultError("message is not a draft"), nil
		}

		// Get the draft message
		email, err := imapOps.GetMessage(ctx, account, mailbox, uid)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		// Detect HTML from draft Content-Type by reading the raw message.
		isHTML := false

		raw, draftErr := imapOps.GetDraft(ctx, account, uid)
		if draftErr == nil {
			isHTML = draftContentIsHTML(raw)
		}

		// Send via SMTP
		sendReq := &smtp.SendRequest{
			To:      email.To,
			CC:      email.CC,
			BCC:     email.BCC,
			Subject: email.Subject,
			Body:    email.Body,
			IsHTML:  isHTML,
		}

		err = smtpOps.Send(ctx, account, sendReq)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		// Delete the draft — non-fatal if it fails (message already sent)
		_ = imapOps.DeleteDraft(ctx, account, uid)

		return jsonResult(map[string]any{
			"success": true,
			"account": account,
			"to":      email.To,
		}), nil
	}
}

// SplitAddresses splits a comma-separated string of email addresses into a
// trimmed slice, discarding empty entries.
func SplitAddresses(s string) []string {
	var addrs []string

	for addr := range strings.SplitSeq(s, ",") {
		addr = strings.TrimSpace(addr)
		if addr != "" {
			addrs = append(addrs, addr)
		}
	}

	return addrs
}

// draftContentIsHTML parses the Content-Type header from a raw RFC 5322 message
// and returns true if the media type is text/html.
func draftContentIsHTML(raw []byte) bool {
	// Split headers from body at the first blank line.
	// Try CRLF first, then fall back to bare LF.
	sep := "\r\n\r\n"

	idx := bytes.Index(raw, []byte(sep))
	if idx < 0 {
		sep = "\n\n"
		idx = bytes.Index(raw, []byte(sep))
	}

	if idx < 0 {
		return false
	}

	// Include the full blank line separator so textproto parses without error.
	reader := textproto.NewReader(bufio.NewReader(bytes.NewReader(raw[:idx+len(sep)])))

	headers, err := reader.ReadMIMEHeader()
	if err != nil {
		return false
	}

	ct := headers.Get("Content-Type")
	if ct == "" {
		return false
	}

	mediaType, _, _ := mime.ParseMediaType(ct)

	return strings.EqualFold(mediaType, "text/html")
}

// buildDraftMessage creates an RFC 5322 message literal with optional file attachments.
// Handles both simple (no attachments) and multipart (with attachments) cases using go-mail.
func buildDraftMessage(
	from string,
	to, cc, bcc []string,
	subject, body string,
	isHTML bool,
	attachments []smtp.SendAttachment,
) ([]byte, error) {
	m := mail.NewMsg()

	err := m.From(from)
	if err != nil {
		return nil, fmt.Errorf("set from: %w", err)
	}

	if len(to) > 0 {
		err = m.To(to...)
		if err != nil {
			return nil, fmt.Errorf("set to: %w", err)
		}
	}

	if len(cc) > 0 {
		err = m.Cc(cc...)
		if err != nil {
			return nil, fmt.Errorf("set cc: %w", err)
		}
	}

	if len(bcc) > 0 {
		err = m.Bcc(bcc...)
		if err != nil {
			return nil, fmt.Errorf("set bcc: %w", err)
		}
	}

	m.Subject(subject)

	if isHTML {
		m.SetBodyString(mail.TypeTextHTML, body)
	} else {
		m.SetBodyString(mail.TypeTextPlain, body)
	}

	for _, att := range attachments {
		m.AttachFile(att.Path, mail.WithFileName(att.Filename))
	}

	var buf bytes.Buffer

	_, err = m.WriteTo(&buf)
	if err != nil {
		return nil, fmt.Errorf("serialize message: %w", err)
	}

	return buf.Bytes(), nil
}
