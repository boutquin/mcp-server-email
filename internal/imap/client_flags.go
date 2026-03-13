package imap

import (
	"context"
	"fmt"
	"time"

	"github.com/boutquin/mcp-server-email/internal/models"
	"github.com/emersion/go-imap/v2"
)

// CopyMessage copies a message to a different mailbox.
func (c *Client) CopyMessage(
	ctx context.Context,
	mailbox string,
	uid uint32,
	destination string,
) error {
	return c.retryOp(ctx, func() error {
		c.mu.Lock()
		defer c.mu.Unlock()

		_, err := c.conn.Select(mailbox, nil)
		if err != nil {
			return fmt.Errorf("select %s: %w", mailbox, err)
		}

		var uidSet imap.UIDSet
		uidSet.AddNum(imap.UID(uid))

		err = c.conn.Copy(uidSet, destination)
		if err != nil {
			return fmt.Errorf("copy message: %w", err)
		}

		c.lastActive = time.Now()

		return nil
	})
}

// DeleteMessage deletes a message (moves to Trash or expunges).
func (c *Client) DeleteMessage(
	ctx context.Context,
	mailbox string,
	uid uint32,
	permanent bool,
) error {
	// Resolve trash folder name before entering retry+mutex block.
	var trashFolder string

	if !permanent {
		var err error

		trashFolder, err = c.getFolderByRole(ctx, models.RoleTrash)
		if err != nil {
			return err
		}
	}

	return c.retryOp(ctx, func() error {
		c.mu.Lock()
		defer c.mu.Unlock()

		_, err := c.conn.Select(mailbox, nil)
		if err != nil {
			return fmt.Errorf("select %s: %w", mailbox, err)
		}

		var uidSet imap.UIDSet
		uidSet.AddNum(imap.UID(uid))

		if permanent {
			// Mark as deleted and expunge
			err = c.conn.Store(uidSet, &imap.StoreFlags{
				Op:    imap.StoreFlagsAdd,
				Flags: []imap.Flag{imap.FlagDeleted},
			}, nil)
			if err != nil {
				return fmt.Errorf("mark deleted: %w", err)
			}

			err = c.conn.Expunge()
			if err != nil {
				return fmt.Errorf("expunge: %w", err)
			}
		} else {
			// Move to trash folder (resolved via RFC 6154 / fallback)
			err = c.conn.Move(uidSet, trashFolder)
			if err != nil {
				return fmt.Errorf("move to trash: %w", err)
			}
		}

		c.lastActive = time.Now()

		return nil
	})
}

// MarkRead marks a message as read or unread.
func (c *Client) MarkRead(
	ctx context.Context,
	mailbox string,
	uid uint32,
	read bool,
) error {
	return c.storeFlag(ctx, mailbox, uid, imap.FlagSeen, read, "mark read")
}

// MoveMessage moves a message to a different mailbox.
func (c *Client) MoveMessage(
	ctx context.Context,
	mailbox string,
	uid uint32,
	destination string,
) error {
	return c.retryOp(ctx, func() error {
		c.mu.Lock()
		defer c.mu.Unlock()

		_, err := c.conn.Select(mailbox, nil)
		if err != nil {
			return fmt.Errorf("select %s: %w", mailbox, err)
		}

		var uidSet imap.UIDSet
		uidSet.AddNum(imap.UID(uid))

		err = c.conn.Move(uidSet, destination)
		if err != nil {
			return fmt.Errorf("move message: %w", err)
		}

		c.lastActive = time.Now()

		return nil
	})
}

// SetFlag sets or clears the flagged status.
func (c *Client) SetFlag(
	ctx context.Context,
	mailbox string,
	uid uint32,
	flagged bool,
) error {
	return c.storeFlag(ctx, mailbox, uid, imap.FlagFlagged, flagged, "set flag")
}

// storeFlag adds or removes a flag on a message.
func (c *Client) storeFlag(
	ctx context.Context,
	mailbox string,
	uid uint32,
	flag imap.Flag,
	add bool,
	opName string,
) error {
	return c.retryOp(ctx, func() error {
		c.mu.Lock()
		defer c.mu.Unlock()

		_, err := c.conn.Select(mailbox, nil)
		if err != nil {
			return fmt.Errorf("select %s: %w", mailbox, err)
		}

		var uidSet imap.UIDSet
		uidSet.AddNum(imap.UID(uid))

		op := imap.StoreFlagsAdd
		if !add {
			op = imap.StoreFlagsDel
		}

		err = c.conn.Store(uidSet, &imap.StoreFlags{
			Op:    op,
			Flags: []imap.Flag{flag},
		}, nil)
		if err != nil {
			return fmt.Errorf("%s: %w", opName, err)
		}

		c.lastActive = time.Now()

		return nil
	})
}
