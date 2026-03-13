// Package imap provides IMAP client with connection pooling, rate limiting, and retry.
package imap

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"github.com/boutquin/mcp-server-email/internal/config"
	"github.com/boutquin/mcp-server-email/internal/models"
	"github.com/boutquin/mcp-server-email/internal/retry"
	"github.com/emersion/go-imap/v2"
	"golang.org/x/oauth2"
)

// Static errors for IMAP connection operations.
var errConnectionNil = errors.New("connection is nil - may have been closed by timeout")

// nopLogger is a discarding logger used when no logger is configured.
//
//nolint:gochecknoglobals // package-level singleton, read-only
var nopLogger = slog.New(slog.NewTextHandler(nopWriter{}, &slog.HandlerOptions{Level: slog.LevelError + 1}))

// nopWriter discards all writes.
type nopWriter struct{}

func (nopWriter) Write(p []byte) (int, error) { return len(p), nil }

// Client wraps an IMAP connection with rate limiting and retry.
type Client struct {
	account     *config.Account
	conn        Connector
	rawConn     net.Conn // stored separately for force-close on timeout
	mu          sync.Mutex
	connInvalid atomic.Bool // set when connection force-closed, signals need for reconnect
	timeout     time.Duration
	limiter     *retry.Limiter
	retryCfg    retry.Config
	tokenSource oauth2.TokenSource // nil for password auth
	lastActive  time.Time
	debug       bool
	logger      *slog.Logger

	// dialConn overrides the default dial function for testing.
	// When nil, dialConnection is used (TLS or STARTTLS based on config).
	dialConn func(ctx context.Context, addr string) connResult

	// folderRoles caches RFC 6154 / common-name folder role → name mappings.
	// Protected by folderRoleMu (separate from conn mu to avoid deadlock).
	folderRoleMu sync.Mutex
	folderRoles  map[models.FolderRole]string
}

// commonFolderNames maps folder roles to common mailbox names used by popular
// providers when RFC 6154 special-use attributes are not available.
//
//nolint:gochecknoglobals // read-only lookup table, package-internal
var commonFolderNames = map[models.FolderRole][]string{
	models.RoleDrafts:  {"Drafts", "INBOX.Drafts", "[Gmail]/Drafts"},
	models.RoleTrash:   {"Trash", "INBOX.Trash", "[Gmail]/Trash", "Deleted Items"},
	models.RoleSent:    {"Sent", "Sent Items", "INBOX.Sent", "[Gmail]/Sent Mail"},
	models.RoleJunk:    {"Junk", "Spam", "INBOX.Junk", "[Gmail]/Spam"},
	models.RoleArchive: {"Archive", "INBOX.Archive", "[Gmail]/All Mail"},
}

// attrToRole maps go-imap v2 special-use mailbox attributes to FolderRole.
//
//nolint:gochecknoglobals // read-only lookup table, package-internal
var attrToRole = map[imap.MailboxAttr]models.FolderRole{
	imap.MailboxAttrDrafts:  models.RoleDrafts,
	imap.MailboxAttrTrash:   models.RoleTrash,
	imap.MailboxAttrSent:    models.RoleSent,
	imap.MailboxAttrJunk:    models.RoleJunk,
	imap.MailboxAttrArchive: models.RoleArchive,
}

// AccountID returns the account ID for this client.
func (c *Client) AccountID() string {
	return c.account.ID
}

// Close closes the IMAP connection.
func (c *Client) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.conn != nil {
		err := c.conn.Close()
		c.conn = nil
		c.rawConn = nil

		if err != nil {
			return fmt.Errorf("close connection: %w", err)
		}
	}

	return nil
}

// IsConnected checks if the connection is still alive.
func (c *Client) IsConnected() bool {
	c.mu.Lock()
	defer c.mu.Unlock()

	return c.conn != nil
}

// LastActivity returns the time of the last IMAP activity.
func (c *Client) LastActivity() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()

	return c.lastActive
}

// RateLimitInfo returns current rate limit state.
func (c *Client) RateLimitInfo() (int, int, time.Time) {
	return c.limiter.Info()
}

// log returns the client's logger, falling back to nopLogger if nil.
func (c *Client) log() *slog.Logger {
	if c.logger != nil {
		return c.logger
	}

	return nopLogger
}
