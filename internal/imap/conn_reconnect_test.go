package imap

import (
	"sync/atomic"
	"testing"
	"time"

	"github.com/boutquin/mcp-server-email/internal/config"
	"github.com/boutquin/mcp-server-email/internal/models"
	"github.com/boutquin/mcp-server-email/internal/retry"
)

// TestReconnect_CleansUpExistingConnection verifies that reconnect closes the
// old connector and clears conn/rawConn before attempting to re-establish.
func TestReconnect_CleansUpExistingConnection(t *testing.T) {
	t.Parallel()

	var closeCalled atomic.Bool

	mock := &mockConnector{
		closeFn: func() error {
			closeCalled.Store(true)

			return nil
		},
	}

	c := &Client{
		account: &config.Account{
			ID:       "test",
			IMAPHost: "localhost",
			IMAPPort: 993,
		},
		conn:    mock,
		rawConn: &mockNetConn{},
		timeout: 1 * time.Second,
		limiter: retry.NewLimiter(10000, time.Minute),
		folderRoles: map[models.FolderRole]string{
			models.RoleSent: "Sent",
		},
	}

	c.connInvalid.Store(true)

	// reconnect will fail on connect (no real server), but we verify cleanup first.
	_ = c.reconnect(t.Context())

	if !closeCalled.Load() {
		t.Error("expected old connector Close() to be called")
	}

	if c.connInvalid.Load() {
		t.Error("expected connInvalid flag to be cleared")
	}

	c.folderRoleMu.Lock()
	roles := c.folderRoles
	c.folderRoleMu.Unlock()

	if roles != nil {
		t.Error("expected folderRoles cache to be cleared")
	}
}

// TestReconnect_NilConnection verifies that reconnect handles the case where
// the existing connection is already nil (e.g., after a failed initial connect).
func TestReconnect_NilConnection(t *testing.T) {
	t.Parallel()

	c := &Client{
		account: &config.Account{
			ID:       "test",
			IMAPHost: "localhost",
			IMAPPort: 993,
		},
		conn:    nil,
		rawConn: nil,
		timeout: 1 * time.Second,
		limiter: retry.NewLimiter(10000, time.Minute),
		folderRoles: map[models.FolderRole]string{
			models.RoleDrafts: "Drafts",
		},
	}

	c.connInvalid.Store(true)

	// reconnect will fail on connect (no real server), but should not panic on nil conn.
	err := c.reconnect(t.Context())
	if err == nil {
		t.Fatal("expected error from connect failure")
	}

	// connInvalid should be cleared even though connect fails.
	if c.connInvalid.Load() {
		t.Error("expected connInvalid flag to be cleared before connect attempt")
	}

	// folderRoles should be cleared.
	c.folderRoleMu.Lock()
	roles := c.folderRoles
	c.folderRoleMu.Unlock()

	if roles != nil {
		t.Error("expected folderRoles cache to be cleared")
	}
}

// TestReconnect_ConnectFailureReturnsError verifies that reconnect propagates
// the connect error.
func TestReconnect_ConnectFailureReturnsError(t *testing.T) {
	t.Parallel()

	c := &Client{
		account: &config.Account{
			ID:       "test",
			IMAPHost: "localhost",
			IMAPPort: 993, // port 993 → TLS path → will fail (no server)
		},
		timeout: 1 * time.Second,
		limiter: retry.NewLimiter(10000, time.Minute),
	}

	err := c.reconnect(t.Context())
	if err == nil {
		t.Fatal("expected error when connect fails during reconnect")
	}
}
