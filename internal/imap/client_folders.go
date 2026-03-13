package imap

import (
	"context"
	"fmt"
	"time"

	"github.com/boutquin/mcp-server-email/internal/models"
	"github.com/emersion/go-imap/v2"
)

// CreateFolder creates a new mailbox.
func (c *Client) CreateFolder(ctx context.Context, name string) error {
	return c.retryOp(ctx, func() error {
		c.mu.Lock()
		defer c.mu.Unlock()

		err := c.conn.Create(name, nil)
		if err != nil {
			return fmt.Errorf("create folder: %w", err)
		}

		c.lastActive = time.Now()

		return nil
	})
}

// ListFolders returns all mailboxes for the account.
// Uses STATUS-only (no Select) for O(n) roundtrips instead of O(2n).
func (c *Client) ListFolders(ctx context.Context) ([]models.Folder, error) {
	var folders []models.Folder

	err := c.retryOp(ctx, func() error {
		c.mu.Lock()
		defer c.mu.Unlock()

		if c.conn == nil {
			return errConnectionNil
		}

		mailboxes, err := c.conn.List("", "*", nil)
		if err != nil {
			return fmt.Errorf("list mailboxes: %w", err)
		}

		folders = nil // Reset on retry

		for _, mbox := range mailboxes {
			folder := models.Folder{Name: mbox.Mailbox}

			// Parse RFC 6154 special-use attributes.
			for _, attr := range mbox.Attrs {
				if role, ok := attrToRole[attr]; ok {
					folder.Role = role

					break
				}
			}

			// Use STATUS to get both counts in a single roundtrip per folder.
			// No Select needed — STATUS works on any mailbox without selecting it.
			statusData, err := c.conn.Status(mbox.Mailbox, &imap.StatusOptions{
				NumMessages: true,
				NumUnseen:   true,
			})
			if err == nil {
				if statusData.NumMessages != nil {
					folder.Total = int(*statusData.NumMessages)
				}

				if statusData.NumUnseen != nil {
					folder.Unread = int(*statusData.NumUnseen)
				}
			}
			// On STATUS error, folder is included with zero counts (not skipped).

			folders = append(folders, folder)
		}

		c.lastActive = time.Now()

		return nil
	})

	return folders, err
}

// getFolderByRole resolves a folder role to its mailbox name.
// It checks the cache first, then discovers roles via ListFolders (RFC 6154),
// and falls back to common folder names if no special-use attributes are found.
func (c *Client) getFolderByRole(ctx context.Context, role models.FolderRole) (string, error) {
	// Check cache first.
	c.folderRoleMu.Lock()
	if c.folderRoles != nil {
		if name, ok := c.folderRoles[role]; ok {
			c.folderRoleMu.Unlock()

			return name, nil
		}
	}
	c.folderRoleMu.Unlock()

	// Discover roles by listing folders (includes RFC 6154 attribute parsing).
	folders, err := c.ListFolders(ctx)
	if err != nil {
		return "", fmt.Errorf("discover folder roles: %w", err)
	}

	// Build role map and name set from results.
	roleMap := make(map[models.FolderRole]string)
	nameSet := make(map[string]struct{}, len(folders))

	for _, f := range folders {
		nameSet[f.Name] = struct{}{}

		if f.Role != "" {
			roleMap[f.Role] = f.Name
		}
	}

	// If RFC 6154 didn't provide the role, try common name fallback.
	if _, ok := roleMap[role]; !ok {
		if candidates, hasFallback := commonFolderNames[role]; hasFallback {
			for _, candidate := range candidates {
				if _, exists := nameSet[candidate]; exists {
					roleMap[role] = candidate

					break
				}
			}
		}
	}

	// Update cache with all discovered roles.
	c.folderRoleMu.Lock()
	c.folderRoles = roleMap
	c.folderRoleMu.Unlock()

	if name, ok := roleMap[role]; ok {
		return name, nil
	}

	return "", fmt.Errorf("%w: %s", models.ErrFolderRoleNotFound, role)
}
