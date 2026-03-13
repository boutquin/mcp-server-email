package imap

import (
	"bytes"
	"fmt"
	"io"
	"slices"
	"strings"
	"time"

	"github.com/boutquin/mcp-server-email/internal/models"
	"github.com/emersion/go-imap/v2"
	"github.com/emersion/go-imap/v2/imapclient"
)

// extractRawBody finds the first non-empty body section from a FETCH response.
func extractRawBody(sections []imapclient.FetchBodySectionBuffer) []byte {
	for _, section := range sections {
		if len(section.Bytes) > 0 {
			return section.Bytes
		}
	}

	return nil
}

// buildFetchOptions returns the standard fetch options used for message listing.
func buildFetchOptions(includeBody bool) *imap.FetchOptions {
	opts := &imap.FetchOptions{
		UID:         true,
		Flags:       true,
		Envelope:    true,
		BodySection: []*imap.FetchItemBodySection{},
	}

	if includeBody {
		opts.BodySection = append(opts.BodySection, &imap.FetchItemBodySection{
			Specifier: imap.PartSpecifierText,
		})
	}

	opts.BodyStructure = &imap.FetchItemBodyStructure{Extended: true}

	// Fetch References header for reply/forward threading
	opts.BodySection = append(opts.BodySection, &imap.FetchItemBodySection{
		Specifier:    imap.PartSpecifierHeader,
		HeaderFields: []string{"References"},
		Peek:         true,
	})

	return opts
}

// fetchMessages fetches messages by sequence set.
func (c *Client) fetchMessages(
	seqSet imap.SeqSet,
	mailbox string,
	includeBody bool,
) ([]models.Email, error) {
	buffers, err := c.conn.Fetch(seqSet, buildFetchOptions(includeBody))
	if err != nil {
		return nil, fmt.Errorf("fetch messages: %w", err)
	}

	return c.convertBuffers(buffers, mailbox, includeBody), nil
}

// fetchMessagesByUID fetches messages by UID set.
func (c *Client) fetchMessagesByUID(
	uidSet imap.UIDSet,
	mailbox string,
	includeBody bool,
) ([]models.Email, error) {
	buffers, err := c.conn.Fetch(uidSet, buildFetchOptions(includeBody))
	if err != nil {
		return nil, fmt.Errorf("fetch messages: %w", err)
	}

	return c.convertBuffers(buffers, mailbox, includeBody), nil
}

// convertBuffers converts fetched message buffers to email models.
func (c *Client) convertBuffers(
	buffers []*imapclient.FetchMessageBuffer,
	mailbox string,
	includeBody bool,
) []models.Email {
	emails := make([]models.Email, 0, len(buffers))

	for _, buf := range buffers {
		emails = append(emails, c.parseMessageBuffer(buf, mailbox, includeBody))
	}

	return emails
}

// parseMessageBuffer extracts email data from a fetch message buffer.
func (c *Client) parseMessageBuffer(
	buf *imapclient.FetchMessageBuffer,
	mailbox string,
	includeBody bool,
) models.Email {
	email := models.Email{
		Mailbox: mailbox,
		Account: c.account.ID,
	}

	email.ID = models.FormatMessageID(c.account.ID, mailbox, uint32(buf.UID))
	applyFlags(&email, buf.Flags)

	if buf.Envelope != nil {
		applyEnvelope(&email, buf.Envelope)
	}

	if buf.BodyStructure != nil {
		email.Attachments = extractAttachments(buf.BodyStructure)
		email.ContentType = extractContentType(buf.BodyStructure)
	}

	// Process body sections
	for _, section := range buf.BodySection {
		if section.Section != nil && section.Section.Specifier == imap.PartSpecifierHeader {
			applyHeaderFields(&email, bytes.NewReader(section.Bytes))
		} else if includeBody && len(section.Bytes) > 0 {
			email.Body = string(section.Bytes)
		}
	}

	return email
}

// buildSearchCriteria constructs IMAP search criteria from the provided filters.
func buildSearchCriteria(
	query, from, to, since, before string,
) *imap.SearchCriteria {
	criteria := &imap.SearchCriteria{}

	if query != "" {
		criteria.Or = append(criteria.Or, [2]imap.SearchCriteria{
			{Header: []imap.SearchCriteriaHeaderField{{Key: "Subject", Value: query}}},
			{Body: []string{query}},
		})
	}

	if from != "" {
		criteria.Header = append(
			criteria.Header,
			imap.SearchCriteriaHeaderField{Key: "From", Value: from},
		)
	}

	if to != "" {
		criteria.Header = append(
			criteria.Header,
			imap.SearchCriteriaHeaderField{Key: "To", Value: to},
		)
	}

	if since != "" {
		t, err := time.Parse("2006-01-02", since)
		if err == nil {
			criteria.Since = t
		}
	}

	if before != "" {
		t, err := time.Parse("2006-01-02", before)
		if err == nil {
			criteria.Before = t
		}
	}

	return criteria
}

// wrapIMAPError wraps raw IMAP protocol errors with user-friendly messages.
// It maps known IMAP error patterns to sentinel errors and strips protocol details
// that should not be exposed to LLMs.
func wrapIMAPError(op string, err error) error {
	if err == nil {
		return nil
	}

	errStr := err.Error()

	switch {
	case strings.Contains(errStr, "AUTHENTICATIONFAILED"):
		return fmt.Errorf("%s: %w", op, models.ErrAuthFailed)
	case strings.Contains(errStr, "NONEXISTENT"):
		return fmt.Errorf("%s: %w", op, models.ErrFolderNotFound)
	case strings.Contains(errStr, "connection refused") ||
		strings.Contains(errStr, "no such host") ||
		strings.Contains(errStr, "TLS connect"):
		return fmt.Errorf("%s: %w", op, models.ErrConnectionFailed)
	default:
		return fmt.Errorf("%s: %w", op, err)
	}
}

// applyFlags sets flag-related fields on the email from IMAP flag data.
func applyFlags(email *models.Email, flags []imap.Flag) {
	for _, flag := range flags {
		if flag == imap.FlagFlagged {
			email.IsFlagged = true
		}
	}
	// Default to unread if \Seen not present
	email.IsUnread = !slices.Contains(flags, imap.FlagSeen)
}

// applyEnvelope sets envelope fields on the email.
func applyEnvelope(email *models.Email, env *imap.Envelope) {
	email.Subject = env.Subject
	email.Date = env.Date.Format(time.RFC3339)

	if len(env.From) > 0 {
		email.From = formatAddress(env.From[0])
	}

	for _, addr := range env.To {
		email.To = append(email.To, formatAddress(addr))
	}

	for _, addr := range env.Cc {
		email.CC = append(email.CC, formatAddress(addr))
	}

	for _, addr := range env.Bcc {
		email.BCC = append(email.BCC, formatAddress(addr))
	}

	email.MessageIDHeader = env.MessageID

	if len(env.InReplyTo) > 0 {
		email.InReplyTo = env.InReplyTo[0]
	}
}

// applyHeaderFields parses fetched header fields (e.g., References) and applies
// them to the email model. The literal contains RFC 5322 formatted headers.
func applyHeaderFields(email *models.Email, literal io.Reader) {
	if literal == nil {
		return
	}

	raw, err := io.ReadAll(literal)
	if err != nil || len(raw) == 0 {
		return
	}

	// Parse "References: <id1> <id2> ..." header from raw RFC 5322 header block.
	// The header may span multiple lines (folded with leading whitespace).
	header := string(raw)

	for line := range strings.SplitSeq(header, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(strings.ToLower(trimmed), "references:") {
			value := strings.TrimSpace(trimmed[len("references:"):])
			email.References = parseMessageIDList(value)
		}
	}
}

// parseMessageIDList splits a space-separated list of message IDs (e.g., "<id1> <id2>").
func parseMessageIDList(s string) []string {
	var ids []string

	for field := range strings.SplitSeq(s, " ") {
		field = strings.TrimSpace(field)
		if field != "" {
			ids = append(ids, field)
		}
	}

	return ids
}

func formatAddress(addr imap.Address) string {
	if addr.Name != "" {
		return fmt.Sprintf("%s <%s@%s>", addr.Name, addr.Mailbox, addr.Host)
	}

	return fmt.Sprintf("%s@%s", addr.Mailbox, addr.Host)
}
