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

type sentAppender interface {
	AppendSent(ctx context.Context, accountID string, message []byte) error
}

// ErrSentButNotArchived indicates SMTP accepted the message but its Sent copy
// could not be stored. Retrying the send may deliver a duplicate.
var ErrSentButNotArchived = errors.New("message sent successfully but could not be saved to Sent; do not resend")

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
func SendHandler(ops smtp.Operations, limits AttachmentLimits, imapOps ...imap.Operations) server.ToolHandlerFunc {
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
		literal, err := ops.SendWithCopy(ctx, accountID, sendReq)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		if len(imapOps) > 0 {
			if err := appendSentCopy(ctx, imapOps[0], accountID, literal); err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
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
		draftsFolder, err := imapOps.GetFolderByRole(ctx, accountID, models.RoleDrafts)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		// Save the draft via IMAP
		uid, err := imapOps.SaveDraft(ctx, accountID, literal)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		draftID := models.FormatMessageID(accountID, draftsFolder, uid)

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

		draftsFolder, err := imapOps.GetFolderByRole(ctx, account, models.RoleDrafts)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		if mailbox != draftsFolder {
			return mcp.NewToolResultError("message is not a draft"), nil
		}

		// Get the draft message
		email, err := imapOps.GetMessage(ctx, account, mailbox, uid)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		raw, err := imapOps.GetDraft(ctx, account, uid)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		isHTML := draftContentIsHTML(raw)

		// Send via SMTP
		sendReq := &smtp.SendRequest{
			To:      email.To,
			CC:      email.CC,
			BCC:     email.BCC,
			Subject: email.Subject,
			Body:    email.Body,
			IsHTML:  isHTML,
		}
		if attachments, attachErr := fetchDraftAttachments(ctx, imapOps, account, mailbox, uid, email.Attachments); attachErr != nil {
			return mcp.NewToolResultError(attachErr.Error()), nil
		} else {
			sendReq.Attachments = attachments
		}

		sentCopy, err := smtpOps.SendWithCopy(ctx, account, sendReq)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		if err := appendSentCopy(ctx, imapOps, account, sentCopy); err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		// A sent draft remains available for recovery if cleanup fails.
		if err := imapOps.DeleteDraft(ctx, account, uid); err != nil {
			return mcp.NewToolResultError("message sent and archived; do not resend; draft cleanup failed: " + err.Error()), nil
		}

		return jsonResult(map[string]any{
			"success": true,
			"account": account,
			"to":      email.To,
		}), nil
	}
}

func appendSentCopy(ctx context.Context, imapOps imap.Operations, accountID string, raw []byte) error {
	appender, ok := any(imapOps).(sentAppender)
	if !ok {
		return fmt.Errorf("%w: IMAP Sent append is unavailable", ErrSentButNotArchived)
	}
	if err := appender.AppendSent(ctx, accountID, raw); err != nil {
		return fmt.Errorf("%w: %v", ErrSentButNotArchived, err)
	}
	return nil
}

func fetchDraftAttachments(
	ctx context.Context, ops imap.Operations, account, mailbox string, uid uint32,
	metadata []models.AttachmentInfo,
) ([]smtp.SendAttachment, error) {
	attachments := make([]smtp.SendAttachment, 0, len(metadata))
	for _, item := range metadata {
		data, filename, err := ops.GetAttachment(ctx, account, mailbox, uid, item.Index)
		if err != nil {
			return nil, fmt.Errorf("read draft attachment %q: %w", item.Filename, err)
		}
		if filename == "" {
			filename = item.Filename
		}
		attachments = append(attachments, smtp.SendAttachment{Filename: filename, Data: data})
	}
	return attachments, nil
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
	return buildSendMessage(from, &smtp.SendRequest{
		To: to, CC: cc, BCC: bcc, Subject: subject, Body: body, IsHTML: isHTML, Attachments: attachments,
	})
}

func buildSendMessage(from string, req *smtp.SendRequest) ([]byte, error) {
	m := mail.NewMsg()

	err := m.From(from)
	if err != nil {
		return nil, fmt.Errorf("set from: %w", err)
	}

	if len(req.To) > 0 {
		err = m.To(req.To...)
		if err != nil {
			return nil, fmt.Errorf("set to: %w", err)
		}
	}

	if len(req.CC) > 0 {
		err = m.Cc(req.CC...)
		if err != nil {
			return nil, fmt.Errorf("set cc: %w", err)
		}
	}

	if len(req.BCC) > 0 {
		err = m.Bcc(req.BCC...)
		if err != nil {
			return nil, fmt.Errorf("set bcc: %w", err)
		}
	}

	if req.ReplyTo != "" {
		if err := m.ReplyTo(req.ReplyTo); err != nil {
			return nil, fmt.Errorf("set reply-to: %w", err)
		}
	}
	if req.InReplyTo != "" {
		m.SetGenHeader(mail.HeaderInReplyTo, req.InReplyTo)
	}
	if len(req.References) > 0 {
		m.SetGenHeader(mail.HeaderReferences, strings.Join(req.References, " "))
	}
	m.Subject(req.Subject)

	if req.IsHTML {
		m.SetBodyString(mail.TypeTextHTML, req.Body)
	} else {
		m.SetBodyString(mail.TypeTextPlain, req.Body)
	}

	for _, att := range req.Attachments {
		if len(att.Data) != 0 {
			err = m.AttachReader(att.Filename, bytes.NewReader(att.Data))
		} else {
			m.AttachFile(att.Path, mail.WithFileName(att.Filename))
		}
		if err != nil {
			return nil, fmt.Errorf("attach %s: %w", att.Filename, err)
		}
	}

	var buf bytes.Buffer

	_, err = m.WriteTo(&buf)
	if err != nil {
		return nil, fmt.Errorf("serialize message: %w", err)
	}

	return buf.Bytes(), nil
}
