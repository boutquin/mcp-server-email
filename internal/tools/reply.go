package tools

import (
	"context"
	"fmt"
	"strings"

	"github.com/boutquin/mcp-server-email/internal/imap"
	"github.com/boutquin/mcp-server-email/internal/models"
	"github.com/boutquin/mcp-server-email/internal/smtp"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// ReplyTool returns the email_reply tool definition.
func ReplyTool() mcp.Tool {
	return mcp.NewTool("email_reply",
		mcp.WithDescription("Reply to an email"),
		mcp.WithString("id", mcp.Description("Message ID to reply to"), mcp.Required()),
		mcp.WithString("body", mcp.Description("Reply body"), mcp.Required()),
		mcp.WithString("account", mcp.Description("Account ID (defaults to default account)")),
		mcp.WithString("cc", mcp.Description("CC recipients, comma-separated")),
		mcp.WithString("bcc", mcp.Description("BCC recipients, comma-separated")),
		mcp.WithBoolean("isHtml", mcp.Description("Body is HTML (default false)")),
		mcp.WithBoolean("all", mcp.Description("Reply all (default false)")),
	)
}

// ReplyHandler returns the handler for email_reply.
func ReplyHandler(imapOps imap.Operations, smtpOps smtp.Operations) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		id, err := req.RequireString("id")
		if err != nil {
			return mcp.NewToolResultError("id is required"), nil
		}

		body, err := req.RequireString("body")
		if err != nil {
			return mcp.NewToolResultError("body is required"), nil
		}

		account, mailbox, uid, parseErr := models.ParseMessageID(id)
		if parseErr != nil {
			return mcp.NewToolResultError(parseErr.Error()), nil
		}

		accountID := resolveAccountID(req.GetString("account", ""), account)

		// Fetch original message
		original, err := imapOps.GetMessage(ctx, accountID, mailbox, uid)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		// Get sender's email for reply-all filtering
		fromEmail, err := smtpOps.AccountEmail(accountID)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		replyAll := req.GetBool("all", false)

		sendReq := buildReplyRequest(original, body, fromEmail, replyAll)
		sendReq.IsHTML = req.GetBool("isHtml", false)

		if cc := req.GetString("cc", ""); cc != "" {
			sendReq.CC = append(sendReq.CC, SplitAddresses(cc)...)
		}

		if bcc := req.GetString("bcc", ""); bcc != "" {
			sendReq.BCC = SplitAddresses(bcc)
		}

		err = smtpOps.Send(ctx, accountID, sendReq)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		return jsonResult(map[string]any{
			"success": true,
			"account": accountID,
			"to":      sendReq.To,
		}), nil
	}
}

// buildReplyRequest constructs an smtp.SendRequest for replying to the original message.
func buildReplyRequest(original *models.Email, body, selfEmail string, replyAll bool) *smtp.SendRequest {
	sendReq := &smtp.SendRequest{
		To:      []string{original.From},
		Subject: addSubjectPrefix("Re: ", original.Subject),
		Body:    buildQuotedReply(body, original.Body),
	}

	// Set threading headers
	if original.MessageIDHeader != "" {
		sendReq.InReplyTo = original.MessageIDHeader

		refs := make([]string, 0, len(original.References)+1)
		refs = append(refs, original.References...)
		refs = append(refs, original.MessageIDHeader)
		sendReq.References = refs
	}

	if replyAll {
		var cc []string

		// Add original To recipients (minus self)
		for _, addr := range original.To {
			if !addressMatchesSelf(addr, selfEmail) {
				cc = append(cc, addr)
			}
		}

		// Add original CC recipients (minus self)
		for _, addr := range original.CC {
			if !addressMatchesSelf(addr, selfEmail) {
				cc = append(cc, addr)
			}
		}

		sendReq.CC = cc
	}

	return sendReq
}

// addSubjectPrefix adds a prefix (e.g., "Re: " or "Fwd: ") to a subject if not already present.
func addSubjectPrefix(prefix, subject string) string {
	if strings.HasPrefix(strings.ToLower(subject), strings.ToLower(prefix)) {
		return subject
	}

	return prefix + subject
}

// buildQuotedReply creates a reply body with the user's text followed by quoted original.
func buildQuotedReply(replyBody, originalBody string) string {
	var b strings.Builder

	b.WriteString(replyBody)
	b.WriteString("\n\n")

	for line := range strings.SplitSeq(originalBody, "\n") {
		b.WriteString("> ")
		b.WriteString(line)
		b.WriteString("\n")
	}

	return b.String()
}

// addressMatchesSelf checks if an address matches the sender's email.
// Handles both bare email and "Name <email>" formats.
func addressMatchesSelf(addr, selfEmail string) bool {
	normalized := extractEmailAddress(addr)

	return strings.EqualFold(normalized, selfEmail)
}

// extractEmailAddress extracts the bare email from an address that may be
// in "Name <email>" format or just a bare email.
func extractEmailAddress(addr string) string {
	if idx := strings.LastIndex(addr, "<"); idx >= 0 {
		end := strings.Index(addr[idx:], ">")
		if end > 0 {
			return addr[idx+1 : idx+end]
		}
	}

	return strings.TrimSpace(addr)
}

// ForwardTool returns the email_forward tool definition.
func ForwardTool() mcp.Tool {
	return mcp.NewTool("email_forward",
		mcp.WithDescription("Forward an email"),
		mcp.WithString("id", mcp.Description("Message ID to forward"), mcp.Required()),
		mcp.WithString("to", mcp.Description("Recipient email address(es), comma-separated"), mcp.Required()),
		mcp.WithString("body", mcp.Description("Optional note to include above forwarded content")),
		mcp.WithString("account", mcp.Description("Account ID (defaults to default account)")),
		mcp.WithString("cc", mcp.Description("CC recipients, comma-separated")),
		mcp.WithString("bcc", mcp.Description("BCC recipients, comma-separated")),
		mcp.WithBoolean("isHtml", mcp.Description("Body is HTML (default false)")),
	)
}

// ForwardHandler returns the handler for email_forward.
func ForwardHandler(imapOps imap.Operations, smtpOps smtp.Operations, limits AttachmentLimits) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		id, err := req.RequireString("id")
		if err != nil {
			return mcp.NewToolResultError("id is required"), nil
		}

		to, err := req.RequireString("to")
		if err != nil {
			return mcp.NewToolResultError("to is required"), nil
		}

		account, mailbox, uid, parseErr := models.ParseMessageID(id)
		if parseErr != nil {
			return mcp.NewToolResultError(parseErr.Error()), nil
		}

		accountID := resolveAccountID(req.GetString("account", ""), account)

		// Fetch original message
		original, err := imapOps.GetMessage(ctx, accountID, mailbox, uid)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		// Fetch original attachments and convert to in-memory SendAttachments.
		attachments, droppedNote := fetchForwardAttachments(
			ctx, imapOps, accountID, mailbox, uid, limits.MaxTotalSizeBytes,
		)

		note := req.GetString("body", "")
		if droppedNote != "" {
			note = appendDroppedNote(note, droppedNote)
		}

		sendReq := buildForwardRequest(original, SplitAddresses(to), note)
		sendReq.Attachments = attachments
		sendReq.IsHTML = req.GetBool("isHtml", false)

		if cc := req.GetString("cc", ""); cc != "" {
			sendReq.CC = SplitAddresses(cc)
		}

		if bcc := req.GetString("bcc", ""); bcc != "" {
			sendReq.BCC = SplitAddresses(bcc)
		}

		err = smtpOps.Send(ctx, accountID, sendReq)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		return jsonResult(map[string]any{
			"success": true,
			"account": accountID,
			"to":      sendReq.To,
		}), nil
	}
}

// buildForwardRequest constructs an smtp.SendRequest for forwarding a message.
func buildForwardRequest(original *models.Email, to []string, note string) *smtp.SendRequest {
	return &smtp.SendRequest{
		To:      to,
		Subject: addSubjectPrefix("Fwd: ", original.Subject),
		Body:    buildForwardBody(note, original),
	}
}

// buildForwardBody creates a forward body with optional note and forwarded content.
func buildForwardBody(note string, original *models.Email) string {
	var b strings.Builder

	if note != "" {
		b.WriteString(note)
		b.WriteString("\n\n")
	}

	b.WriteString("---------- Forwarded message ----------\n")
	b.WriteString("From: ")
	b.WriteString(original.From)
	b.WriteString("\n")
	b.WriteString("Date: ")
	b.WriteString(original.Date)
	b.WriteString("\n")
	b.WriteString("Subject: ")
	b.WriteString(original.Subject)
	b.WriteString("\n")

	if len(original.To) > 0 {
		b.WriteString("To: ")
		b.WriteString(strings.Join(original.To, ", "))
		b.WriteString("\n")
	}

	b.WriteString("\n")
	b.WriteString(original.Body)

	return b.String()
}

// fetchForwardAttachments retrieves the original message's attachments for forwarding.
// It skips inline parts and enforces a cumulative size limit. Returns the converted
// attachments and a note about any dropped attachments (empty if none were dropped).
func fetchForwardAttachments(
	ctx context.Context,
	imapOps imap.Operations,
	accountID, mailbox string,
	uid uint32,
	maxTotalBytes int64,
) ([]smtp.SendAttachment, string) {
	metas, err := imapOps.GetAttachments(ctx, accountID, mailbox, uid)
	if err != nil || len(metas) == 0 {
		return nil, ""
	}

	var (
		result  []smtp.SendAttachment
		dropped []string
		cumSize int64
	)

	for _, meta := range metas {
		// Check if adding this attachment would exceed the size limit.
		if cumSize+meta.Size > maxTotalBytes {
			dropped = append(dropped, fmt.Sprintf("%s (%s)", meta.Filename, formatBytes(meta.Size)))

			continue
		}

		data, filename, fetchErr := imapOps.GetAttachment(ctx, accountID, mailbox, uid, meta.Index)
		if fetchErr != nil {
			dropped = append(dropped, meta.Filename+" (fetch error)")

			continue
		}

		result = append(result, smtp.SendAttachment{
			Filename: filename,
			Data:     data,
		})

		cumSize += int64(len(data))
	}

	var droppedNote string

	if len(dropped) > 0 {
		droppedNote = "[Note: Some attachments were too large to forward: " +
			strings.Join(dropped, ", ") + "]"
	}

	return result, droppedNote
}

// appendDroppedNote appends a dropped-attachment note to the user's forward note.
func appendDroppedNote(note, droppedNote string) string {
	if note != "" {
		return note + "\n\n" + droppedNote
	}

	return droppedNote
}

// formatBytes returns a human-readable byte size string.
func formatBytes(b int64) string {
	const mb = 1024 * 1024

	if b >= mb {
		return fmt.Sprintf("%.1f MB", float64(b)/float64(mb))
	}

	const kb = 1024

	return fmt.Sprintf("%.1f KB", float64(b)/float64(kb))
}
