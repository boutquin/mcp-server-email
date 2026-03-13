package imap

import (
	"context"
	"fmt"
	"time"

	"github.com/boutquin/mcp-server-email/internal/models"
	"github.com/emersion/go-imap/v2"
)

// DeleteDraft deletes a draft after sending.
func (c *Client) DeleteDraft(ctx context.Context, uid uint32) error {
	draftsFolder, err := c.getFolderByRole(ctx, models.RoleDrafts)
	if err != nil {
		return err
	}

	return c.DeleteMessage(ctx, draftsFolder, uid, true)
}

// GetDraft retrieves a draft message.
func (c *Client) GetDraft(ctx context.Context, uid uint32) ([]byte, error) {
	draftsFolder, err := c.getFolderByRole(ctx, models.RoleDrafts)
	if err != nil {
		return nil, err
	}

	var literal []byte

	err = c.retryOp(ctx, func() error {
		c.mu.Lock()
		defer c.mu.Unlock()

		_, err := c.conn.Select(draftsFolder, nil)
		if err != nil {
			return fmt.Errorf("select %s: %w", draftsFolder, err)
		}

		var uidSet imap.UIDSet
		uidSet.AddNum(imap.UID(uid))

		fetchOptions := &imap.FetchOptions{
			BodySection: []*imap.FetchItemBodySection{{}},
		}

		buffers, err := c.conn.Fetch(uidSet, fetchOptions)
		if err != nil {
			return fmt.Errorf("fetch draft: %w", err)
		}

		if len(buffers) == 0 {
			return models.ErrMessageNotFound
		}

		// Find the body section in the first buffer
		for _, section := range buffers[0].BodySection {
			if len(section.Bytes) > 0 {
				literal = section.Bytes

				break
			}
		}

		c.lastActive = time.Now()

		return nil
	})

	return literal, err
}

// SaveDraft saves a message as a draft.
func (c *Client) SaveDraft(ctx context.Context, literal []byte) (uint32, error) {
	draftsFolder, err := c.getFolderByRole(ctx, models.RoleDrafts)
	if err != nil {
		return 0, err
	}

	var uid uint32

	err = c.retryOp(ctx, func() error {
		c.mu.Lock()
		defer c.mu.Unlock()

		// Check for nil connection (race condition protection)
		if c.conn == nil {
			return errConnectionNil
		}

		// Append to drafts folder (resolved via RFC 6154 / fallback)
		data, err := c.conn.Append(draftsFolder, literal, nil)
		if err != nil {
			return fmt.Errorf("append draft: %w", err)
		}

		if data.UID != 0 {
			uid = uint32(data.UID)
		}

		c.lastActive = time.Now()

		return nil
	})

	return uid, err
}
