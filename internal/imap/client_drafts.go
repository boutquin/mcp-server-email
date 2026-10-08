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
	if !c.conn.SupportsUIDExpunge() {
		trashFolder, err := c.getFolderByRole(ctx, models.RoleTrash)
		if err != nil {
			return fmt.Errorf("cannot safely delete draft without UID EXPUNGE and Trash: %w", err)
		}
		return c.MoveMessage(ctx, draftsFolder, uid, trashFolder)
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

// UpdateDraft appends a replacement RFC 5322 message and marks only the old
// draft UID as deleted. It never issues a mailbox-wide EXPUNGE.
func (c *Client) UpdateDraft(ctx context.Context, uid uint32, literal []byte) (uint32, error) {
	draftsFolder, err := c.getFolderByRole(ctx, models.RoleDrafts)
	if err != nil {
		return 0, err
	}
	uidExpunge := c.conn.SupportsUIDExpunge()
	trashFolder := ""
	if !uidExpunge {
		trashFolder, err = c.getFolderByRole(ctx, models.RoleTrash)
		if err != nil {
			return 0, fmt.Errorf("cannot safely replace draft without UID EXPUNGE and Trash: %w", err)
		}
	}
	var newUID uint32
	err = c.retryOp(ctx, func() error {
		c.mu.Lock()
		defer c.mu.Unlock()
		if c.conn == nil {
			return errConnectionNil
		}
		if _, err := c.conn.Select(draftsFolder, nil); err != nil {
			return fmt.Errorf("select %s: %w", draftsFolder, err)
		}
		data, err := c.conn.Append(draftsFolder, literal, nil)
		if err != nil {
			return fmt.Errorf("append replacement draft: %w", err)
		}
		if data.UID == 0 {
			return fmt.Errorf("append replacement draft: server did not return UID")
		}
		newUID = uint32(data.UID)
		var uidSet imap.UIDSet
		uidSet.AddNum(imap.UID(uid))
		if !uidExpunge {
			if err := c.conn.Move(uidSet, trashFolder); err != nil {
				return fmt.Errorf("move old draft to Trash after replacement UID %d: %w", newUID, err)
			}
			c.lastActive = time.Now()
			return nil
		}
		if err := c.conn.Store(uidSet, &imap.StoreFlags{
			Op: imap.StoreFlagsAdd, Flags: []imap.Flag{imap.FlagDeleted},
		}, nil); err != nil {
			return fmt.Errorf("mark old draft deleted after replacement UID %d: %w", newUID, err)
		}
		if err := c.conn.UIDExpunge(uidSet); err != nil {
			return fmt.Errorf("UID EXPUNGE old draft after replacement UID %d: %w", newUID, err)
		}
		c.lastActive = time.Now()
		return nil
	})
	return newUID, err
}

// AppendSent stores a copy of a successfully sent message in the mailbox
// advertised for the RFC 6154 Sent role.
func (c *Client) AppendSent(ctx context.Context, literal []byte) error {
	sentFolder, err := c.getFolderByRole(ctx, models.RoleSent)
	if err != nil {
		return err
	}

	return c.retryOp(ctx, func() error {
		c.mu.Lock()
		defer c.mu.Unlock()

		if c.conn == nil {
			return errConnectionNil
		}

		if _, appendErr := c.conn.Append(sentFolder, literal, nil); appendErr != nil {
			return fmt.Errorf("append sent message: %w", appendErr)
		}

		c.lastActive = time.Now()
		return nil
	})
}
