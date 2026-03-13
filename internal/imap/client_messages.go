package imap

import (
	"context"
	"fmt"
	"time"

	"github.com/boutquin/mcp-server-email/internal/models"
	"github.com/emersion/go-imap/v2"
)

// GetAttachment downloads a specific attachment by index, returning raw bytes and filename.
func (c *Client) GetAttachment(
	ctx context.Context,
	mailbox string,
	uid uint32,
	index int,
) ([]byte, string, error) {
	attachments, err := c.GetAttachments(ctx, mailbox, uid)
	if err != nil {
		return nil, "", err
	}

	if index < 0 || index >= len(attachments) {
		return nil, "", fmt.Errorf("%w: %d (have %d)", errAttachmentNotFound, index, len(attachments))
	}

	att := attachments[index]

	data, err := c.fetchAttachmentBody(ctx, mailbox, uid, index)
	if err != nil {
		return nil, "", err
	}

	return data, att.Filename, nil
}

// GetAttachments returns attachment metadata for a message by parsing its BODYSTRUCTURE.
func (c *Client) GetAttachments(
	ctx context.Context,
	mailbox string,
	uid uint32,
) ([]models.AttachmentInfo, error) {
	var attachments []models.AttachmentInfo

	err := c.retryOp(ctx, func() error {
		c.mu.Lock()
		defer c.mu.Unlock()

		_, err := c.conn.Select(mailbox, nil)
		if err != nil {
			return fmt.Errorf("select %s: %w", mailbox, err)
		}

		var uidSet imap.UIDSet
		uidSet.AddNum(imap.UID(uid))

		fetchOpts := &imap.FetchOptions{
			UID:           true,
			BodyStructure: &imap.FetchItemBodyStructure{Extended: true},
		}

		buffers, err := c.conn.Fetch(uidSet, fetchOpts)
		if err != nil {
			return fmt.Errorf("fetch bodystructure: %w", err)
		}

		if len(buffers) == 0 {
			return models.ErrMessageNotFound
		}

		if buffers[0].BodyStructure != nil {
			attachments = extractAttachments(buffers[0].BodyStructure)
		}

		c.lastActive = time.Now()

		return nil
	})

	return attachments, err
}

// GetMessage retrieves a single message by UID.
func (c *Client) GetMessage(
	ctx context.Context,
	mailbox string,
	uid uint32,
) (*models.Email, error) {
	var email *models.Email

	err := c.retryOp(ctx, func() error {
		c.mu.Lock()
		defer c.mu.Unlock()

		// Select mailbox
		_, err := c.conn.Select(mailbox, nil)
		if err != nil {
			return fmt.Errorf("select %s: %w", mailbox, err)
		}

		var uidSet imap.UIDSet
		uidSet.AddNum(imap.UID(uid))

		emails, err := c.fetchMessagesByUID(uidSet, mailbox, true)
		if err != nil {
			return err
		}

		if len(emails) == 0 {
			return models.ErrMessageNotFound
		}

		email = &emails[0]
		c.lastActive = time.Now()

		return nil
	})

	return email, err
}

// ListMessages returns messages from a mailbox.
func (c *Client) ListMessages(
	ctx context.Context,
	mailbox string,
	limit, offset int,
	includeBody bool,
) ([]models.Email, error) {
	var emails []models.Email

	err := c.retryOp(ctx, func() error {
		c.mu.Lock()
		defer c.mu.Unlock()

		// Select mailbox
		data, err := c.conn.Select(mailbox, nil)
		if err != nil {
			return fmt.Errorf("select %s: %w", mailbox, err)
		}

		if data.NumMessages == 0 {
			return nil
		}

		// Calculate range (IMAP uses 1-based sequence numbers, newest last)
		total := int(data.NumMessages)
		start := total - offset - limit + 1
		end := total - offset

		if start < 1 {
			start = 1
		}

		if end < 1 {
			return nil
		}

		// Create sequence set for range
		var seqSet imap.SeqSet
		seqSet.AddRange(uint32(start), uint32(end)) //nolint:gosec // start and end are bounds-checked ≥ 1 above

		emails, err = c.fetchMessages(seqSet, mailbox, includeBody)
		if err != nil {
			return err
		}

		c.lastActive = time.Now()

		return nil
	})

	return emails, err
}

// ListUnread returns unread messages from a mailbox.
func (c *Client) ListUnread(
	ctx context.Context,
	mailbox string,
	limit int,
	includeBody bool,
) ([]models.Email, error) {
	var emails []models.Email

	err := c.retryOp(ctx, func() error {
		c.mu.Lock()
		defer c.mu.Unlock()

		// Select mailbox
		_, err := c.conn.Select(mailbox, nil)
		if err != nil {
			return fmt.Errorf("select %s: %w", mailbox, err)
		}

		// Search for unseen messages
		criteria := &imap.SearchCriteria{
			NotFlag: []imap.Flag{imap.FlagSeen},
		}

		searchData, err := c.conn.Search(criteria, nil)
		if err != nil {
			return fmt.Errorf("search unseen: %w", err)
		}

		seqNums := searchData.AllSeqNums()
		if len(seqNums) == 0 {
			return nil
		}

		// Limit results
		if len(seqNums) > limit {
			seqNums = seqNums[len(seqNums)-limit:]
		}

		var seqSet imap.SeqSet
		for _, num := range seqNums {
			seqSet.AddNum(num)
		}

		emails, err = c.fetchMessages(seqSet, mailbox, includeBody)
		if err != nil {
			return err
		}

		c.lastActive = time.Now()

		return nil
	})

	return emails, err
}

// Search searches for messages matching criteria.
func (c *Client) Search(
	ctx context.Context,
	mailbox, query, from, to, since, before string,
	limit int,
	includeBody bool,
) ([]models.Email, error) {
	var emails []models.Email

	err := c.retryOp(ctx, func() error {
		c.mu.Lock()
		defer c.mu.Unlock()

		// Select mailbox
		_, err := c.conn.Select(mailbox, nil)
		if err != nil {
			return fmt.Errorf("select %s: %w", mailbox, err)
		}

		criteria := buildSearchCriteria(query, from, to, since, before)

		searchData, err := c.conn.Search(criteria, nil)
		if err != nil {
			return fmt.Errorf("search: %w", err)
		}

		seqNums := searchData.AllSeqNums()
		if len(seqNums) == 0 {
			return nil
		}

		// Limit results
		if len(seqNums) > limit {
			seqNums = seqNums[len(seqNums)-limit:]
		}

		var seqSet imap.SeqSet
		for _, num := range seqNums {
			seqSet.AddNum(num)
		}

		emails, err = c.fetchMessages(seqSet, mailbox, includeBody)
		if err != nil {
			return err
		}

		c.lastActive = time.Now()

		return nil
	})

	return emails, err
}

// SearchByMessageID searches for messages whose Message-ID or References header
// contains the given message ID. Results are deduplicated and sorted by date ascending.
func (c *Client) SearchByMessageID(
	ctx context.Context,
	mailbox, messageID string,
) ([]models.Email, error) {
	var emails []models.Email

	err := c.retryOp(ctx, func() error {
		c.mu.Lock()
		defer c.mu.Unlock()

		_, err := c.conn.Select(mailbox, nil)
		if err != nil {
			return fmt.Errorf("select %s: %w", mailbox, err)
		}

		// Search Message-ID header.
		msgIDCriteria := &imap.SearchCriteria{
			Header: []imap.SearchCriteriaHeaderField{
				{Key: "Message-ID", Value: messageID},
			},
		}

		// Search References header.
		refsCriteria := &imap.SearchCriteria{
			Header: []imap.SearchCriteriaHeaderField{
				{Key: "References", Value: messageID},
			},
		}

		// OR the two criteria.
		orCriteria := &imap.SearchCriteria{
			Or: [][2]imap.SearchCriteria{
				{*msgIDCriteria, *refsCriteria},
			},
		}

		searchData, err := c.conn.Search(orCriteria, nil)
		if err != nil {
			return fmt.Errorf("search: %w", err)
		}

		seqNums := searchData.AllSeqNums()
		if len(seqNums) == 0 {
			return nil
		}

		var seqSet imap.SeqSet
		for _, num := range seqNums {
			seqSet.AddNum(num)
		}

		emails, err = c.fetchMessages(seqSet, mailbox, false)
		if err != nil {
			return err
		}

		c.lastActive = time.Now()

		return nil
	})

	return emails, err
}

// fetchAttachmentBody fetches the full message and extracts an attachment part by index.
func (c *Client) fetchAttachmentBody(
	ctx context.Context, mailbox string, uid uint32, index int,
) ([]byte, error) {
	var data []byte

	err := c.retryOp(ctx, func() error {
		c.mu.Lock()
		defer c.mu.Unlock()

		_, err := c.conn.Select(mailbox, nil)
		if err != nil {
			return fmt.Errorf("select %s: %w", mailbox, err)
		}

		var uidSet imap.UIDSet
		uidSet.AddNum(imap.UID(uid))

		fetchOpts := &imap.FetchOptions{
			UID: true,
			BodySection: []*imap.FetchItemBodySection{
				{}, // Empty section = entire message
			},
		}

		buffers, err := c.conn.Fetch(uidSet, fetchOpts)
		if err != nil {
			return fmt.Errorf("fetch message: %w", err)
		}

		if len(buffers) == 0 {
			return models.ErrMessageNotFound
		}

		rawMsg := extractRawBody(buffers[0].BodySection)
		if rawMsg == nil {
			return models.ErrMessageNotFound
		}

		data, err = extractAttachmentByIndex(rawMsg, index)
		if err != nil {
			return fmt.Errorf("extract attachment: %w", err)
		}

		c.lastActive = time.Now()

		return nil
	})

	return data, err
}
