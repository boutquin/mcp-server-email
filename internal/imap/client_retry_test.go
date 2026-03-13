package imap

import (
	"context"
	"errors"
	"net"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/boutquin/mcp-server-email/internal/config"
	"github.com/boutquin/mcp-server-email/internal/models"
	"github.com/boutquin/mcp-server-email/internal/retry"
	"github.com/emersion/go-imap/v2"
)

// --- Test sentinel errors (err113) ---

var (
	errMock       = errors.New("mock")
	errConnReset  = errors.New("connection reset by peer")
	errConnReset2 = errors.New("connection reset")
	errAuthFailed = errors.New("authentication failed")
	errClosedConn = errors.New("use of closed connection")
)

// --- Test helpers ---

// mockNetConn implements net.Conn for testing forceClose behavior.
type mockNetConn struct {
	closed atomic.Bool
}

func (m *mockNetConn) Read([]byte) (int, error)  { return 0, errMock }
func (m *mockNetConn) Write([]byte) (int, error) { return 0, errMock }

func (m *mockNetConn) Close() error {
	m.closed.Store(true)

	return nil
}

func (m *mockNetConn) LocalAddr() net.Addr              { return &net.TCPAddr{} }
func (m *mockNetConn) RemoteAddr() net.Addr             { return &net.TCPAddr{} }
func (m *mockNetConn) SetDeadline(time.Time) error      { return nil }
func (m *mockNetConn) SetReadDeadline(time.Time) error  { return nil }
func (m *mockNetConn) SetWriteDeadline(time.Time) error { return nil }

// newTestClient creates a Client with mock connections for testing retry/timeout orchestration.
// Uses high rate limit and long timeout to avoid interfering with retry/timeout tests.
func newTestClient(timeout time.Duration) *Client {
	return &Client{
		account: &config.Account{ID: "test"},
		timeout: timeout,
		limiter: retry.NewLimiter(10000, time.Minute),
		retryCfg: retry.Config{
			MaxRetries: retry.DefaultMaxRetries,
			BaseDelay:  retry.DefaultBaseDelay,
			MaxDelay:   retry.DefaultMaxDelay,
		},
		rawConn: &mockNetConn{},
	}
}

// --- withRetry tests ---

func TestRetry_SuccessFirstAttempt(t *testing.T) {
	t.Parallel()

	c := newTestClient(30 * time.Second)

	var attempts int

	err := c.retryOp(t.Context(), func() error {
		attempts++

		return nil
	})
	if err != nil {
		t.Fatalf("expected nil error, got: %v", err)
	}

	if attempts != 1 {
		t.Errorf("expected 1 attempt, got %d", attempts)
	}
}

func TestRetry_TransientThenSuccess(t *testing.T) {
	t.Parallel()

	c := newTestClient(30 * time.Second)

	var attempts int

	err := c.retryOp(t.Context(), func() error {
		attempts++
		if attempts == 1 {
			return errConnReset
		}

		return nil
	})
	if err != nil {
		t.Fatalf("expected nil error after retry, got: %v", err)
	}

	if attempts != 2 {
		t.Errorf("expected 2 attempts, got %d", attempts)
	}
}

func TestRetry_PermanentError(t *testing.T) {
	t.Parallel()

	c := newTestClient(30 * time.Second)

	var attempts int

	err := c.retryOp(t.Context(), func() error {
		attempts++

		return errAuthFailed
	})
	if err == nil {
		t.Fatal("expected error for permanent failure")
	}

	if attempts != 1 {
		t.Errorf("expected 1 attempt (no retry for permanent error), got %d", attempts)
	}

	if !strings.Contains(err.Error(), "authentication failed") {
		t.Errorf("expected 'authentication failed' in error, got: %v", err)
	}
}

func TestRetry_MaxRetriesExceeded(t *testing.T) {
	t.Parallel()

	c := newTestClient(30 * time.Second)

	var attempts int

	err := c.retryOp(t.Context(), func() error {
		attempts++

		return errConnReset
	})
	if err == nil {
		t.Fatal("expected error after max retries")
	}

	// maxRetries=3, so attempts = initial(0) + 3 retries = 4 total
	if attempts != 4 {
		t.Errorf("expected 4 attempts (initial + 3 retries), got %d", attempts)
	}

	if !strings.Contains(err.Error(), "max retries exceeded") {
		t.Errorf("expected 'max retries exceeded' in error, got: %v", err)
	}

	if !strings.Contains(err.Error(), "connection reset") {
		t.Errorf("expected original error wrapped, got: %v", err)
	}
}

func TestRetry_ContextCancelledDuringBackoff(t *testing.T) {
	t.Parallel()

	c := newTestClient(30 * time.Second)

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	var attempts int

	err := c.retryOp(ctx, func() error {
		attempts++
		if attempts == 1 {
			// Cancel context shortly after — during the 1s+ backoff
			go func() {
				time.Sleep(100 * time.Millisecond)
				cancel()
			}()

			return errConnReset2
		}

		return nil
	})
	if err == nil {
		t.Fatal("expected error when context cancelled during backoff")
	}

	if !errors.Is(err, context.Canceled) {
		t.Errorf("expected context.Canceled, got: %v", err)
	}

	if attempts != 1 {
		t.Errorf("expected 1 attempt before context cancel, got %d", attempts)
	}
}

// --- withTimeout tests ---

func TestTimeout_CompletesWithinTimeout(t *testing.T) {
	t.Parallel()

	c := newTestClient(5 * time.Second)

	result := "not set"

	err := c.withTimeout(t.Context(), func() error {
		result = "completed"

		return nil
	})
	if err != nil {
		t.Fatalf("expected nil error, got: %v", err)
	}

	if result != "completed" {
		t.Errorf("expected result 'completed', got %q", result)
	}
}

func TestTimeout_ExceedsTimeout(t *testing.T) {
	t.Parallel()

	mockConn := &mockNetConn{}
	c := newTestClient(100 * time.Millisecond)
	c.rawConn = mockConn

	err := c.withTimeout(t.Context(), func() error {
		// Simulate hung I/O that unblocks when connection is force-closed
		for !mockConn.closed.Load() {
			time.Sleep(5 * time.Millisecond)
		}

		return errClosedConn
	})
	if err == nil {
		t.Fatal("expected timeout error")
	}

	if !strings.Contains(err.Error(), "timed out") {
		t.Errorf("expected 'timed out' in error, got: %v", err)
	}

	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("expected context.DeadlineExceeded wrapped, got: %v", err)
	}

	if !c.connInvalid.Load() {
		t.Error("expected connInvalid to be true after timeout")
	}

	if !mockConn.closed.Load() {
		t.Error("expected rawConn to be closed after timeout")
	}
}

func TestTimeout_ContextCancelledBeforeTimeout(t *testing.T) {
	t.Parallel()

	mockConn := &mockNetConn{}
	c := newTestClient(30 * time.Second) // long timeout — parent cancel fires first
	c.rawConn = mockConn

	ctx, cancel := context.WithCancel(t.Context())

	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()

	err := c.withTimeout(ctx, func() error {
		// Simulate hung I/O that unblocks when connection is force-closed
		for !mockConn.closed.Load() {
			time.Sleep(5 * time.Millisecond)
		}

		return errClosedConn
	})
	if err == nil {
		t.Fatal("expected error when parent context cancelled")
	}

	if !errors.Is(err, context.Canceled) {
		t.Errorf("expected context.Canceled, got: %v", err)
	}
}

// --- Rate limit tests ---

func TestRateLimit_TokenRefillAtWindowBoundary(t *testing.T) {
	t.Parallel()

	// Create a limiter with exhausted tokens and expired window
	lim := retry.NewLimiter(5, time.Minute)

	// Drain all tokens
	for range 5 {
		err := retry.WaitForToken(t.Context(), lim)
		if err != nil {
			t.Fatalf("drain: unexpected error: %v", err)
		}
	}

	// Manually expire the window by creating a new limiter that simulates expired state
	lim = retry.NewLimiter(5, 1*time.Millisecond)
	// Drain and wait for window to expire
	for range 5 {
		_ = retry.WaitForToken(t.Context(), lim)
	}

	time.Sleep(5 * time.Millisecond) // window expires

	err := retry.WaitForToken(t.Context(), lim)
	if err != nil {
		t.Fatalf("expected nil error after token refill, got: %v", err)
	}

	// Tokens should have been reset to limit (5) then decremented by 1
	remaining, _, _ := lim.Info()
	if remaining != 4 {
		t.Errorf("expected 4 remaining tokens (5 refilled - 1 consumed), got %d", remaining)
	}
}

// --- Coverage: reconnect failure path ---

func TestRetry_ReconnectFailure(t *testing.T) {
	t.Parallel()

	c := newTestClient(500 * time.Millisecond)
	c.connInvalid.Store(true) // triggers reconnect path in withRetry
	c.account = &config.Account{
		ID:       "test",
		IMAPHost: "localhost",
		IMAPPort: 1, // will get connection refused immediately
	}

	err := c.retryOp(t.Context(), func() error {
		return nil
	})
	if err == nil {
		t.Fatal("expected error from failed reconnect")
	}

	if !strings.Contains(err.Error(), "reconnect failed") {
		t.Errorf("expected 'reconnect failed', got: %v", err)
	}
}

// --- Coverage: helper functions ---

func TestApplyFlags_FlaggedAndUnread(t *testing.T) {
	t.Parallel()

	email := &models.Email{}
	applyFlags(email, []imap.Flag{imap.FlagFlagged})

	if !email.IsFlagged {
		t.Error("expected IsFlagged to be true")
	}

	if !email.IsUnread {
		t.Error("expected IsUnread to be true when Seen flag is absent")
	}
}

func TestApplyFlags_SeenNotFlagged(t *testing.T) {
	t.Parallel()

	email := &models.Email{}
	applyFlags(email, []imap.Flag{imap.FlagSeen})

	if email.IsFlagged {
		t.Error("expected IsFlagged to be false")
	}

	if email.IsUnread {
		t.Error("expected IsUnread to be false when Seen flag is present")
	}
}

func TestApplyEnvelope(t *testing.T) {
	t.Parallel()

	email := &models.Email{}
	now := time.Now()

	env := &imap.Envelope{
		Subject: "Test Subject",
		Date:    now,
		From:    []imap.Address{{Name: "Sender", Mailbox: "sender", Host: "example.com"}},
		To:      []imap.Address{{Mailbox: "to", Host: "example.com"}},
		Cc:      []imap.Address{{Mailbox: "cc", Host: "example.com"}},
		Bcc:     []imap.Address{{Mailbox: "bcc", Host: "example.com"}},
	}

	applyEnvelope(email, env)

	if email.Subject != "Test Subject" {
		t.Errorf("Subject = %q, want %q", email.Subject, "Test Subject")
	}

	if email.From != "Sender <sender@example.com>" {
		t.Errorf("From = %q, want %q", email.From, "Sender <sender@example.com>")
	}

	if len(email.To) != 1 || email.To[0] != "to@example.com" {
		t.Errorf("To = %v, want [to@example.com]", email.To)
	}

	if len(email.CC) != 1 || email.CC[0] != "cc@example.com" {
		t.Errorf("CC = %v, want [cc@example.com]", email.CC)
	}

	if len(email.BCC) != 1 || email.BCC[0] != "bcc@example.com" {
		t.Errorf("BCC = %v, want [bcc@example.com]", email.BCC)
	}

	if email.Date != now.Format(time.RFC3339) {
		t.Errorf("Date = %q, want %q", email.Date, now.Format(time.RFC3339))
	}
}

func TestFormatAddress_WithName(t *testing.T) {
	t.Parallel()

	result := formatAddress(imap.Address{Name: "John Doe", Mailbox: "john", Host: "example.com"})

	if result != "John Doe <john@example.com>" {
		t.Errorf("formatAddress() = %q, want %q", result, "John Doe <john@example.com>")
	}
}

func TestFormatAddress_WithoutName(t *testing.T) {
	t.Parallel()

	result := formatAddress(imap.Address{Mailbox: "john", Host: "example.com"})

	if result != "john@example.com" {
		t.Errorf("formatAddress() = %q, want %q", result, "john@example.com")
	}
}

func TestDefaultAccountID(t *testing.T) {
	t.Parallel()

	pool := NewPool(&config.Config{DefaultAccount: "main"})

	if pool.DefaultAccountID() != "main" {
		t.Errorf("DefaultAccountID() = %q, want %q", pool.DefaultAccountID(), "main")
	}
}

// --- Coverage: extractAttachments ---

func TestExtractAttachments_SinglePartAttachment(t *testing.T) {
	t.Parallel()

	bs := &imap.BodyStructureSinglePart{
		Type:    "application",
		Subtype: "pdf",
		Size:    2048,
		Extended: &imap.BodyStructureSinglePartExt{
			Disposition: &imap.BodyStructureDisposition{
				Value:  "attachment",
				Params: map[string]string{"filename": "report.pdf"},
			},
		},
	}

	atts := extractAttachments(bs)

	if len(atts) != 1 {
		t.Fatalf("expected 1 attachment, got %d", len(atts))
	}

	if atts[0].Filename != "report.pdf" {
		t.Errorf("Filename = %q, want %q", atts[0].Filename, "report.pdf")
	}

	if atts[0].ContentType != "application/pdf" {
		t.Errorf("ContentType = %q, want %q", atts[0].ContentType, "application/pdf")
	}

	if atts[0].Size != 2048 {
		t.Errorf("Size = %d, want %d", atts[0].Size, 2048)
	}
}

func TestExtractAttachments_SinglePartNoDisposition(t *testing.T) {
	t.Parallel()

	bs := &imap.BodyStructureSinglePart{
		Type:    "text",
		Subtype: "plain",
		Size:    100,
	}

	atts := extractAttachments(bs)

	if len(atts) != 0 {
		t.Errorf("expected 0 attachments for inline part, got %d", len(atts))
	}
}

func TestExtractAttachments_MultiPart(t *testing.T) {
	t.Parallel()

	bs := &imap.BodyStructureMultiPart{
		Subtype: "mixed",
		Children: []imap.BodyStructure{
			&imap.BodyStructureSinglePart{
				Type:    "text",
				Subtype: "plain",
				Size:    50,
			},
			&imap.BodyStructureSinglePart{
				Type:    "image",
				Subtype: "png",
				Size:    4096,
				Extended: &imap.BodyStructureSinglePartExt{
					Disposition: &imap.BodyStructureDisposition{
						Value:  "attachment",
						Params: map[string]string{"filename": "photo.png"},
					},
				},
			},
		},
	}

	atts := extractAttachments(bs)

	if len(atts) != 1 {
		t.Fatalf("expected 1 attachment, got %d", len(atts))
	}

	if atts[0].Filename != "photo.png" {
		t.Errorf("Filename = %q, want %q", atts[0].Filename, "photo.png")
	}
}

func TestExtractAttachments_FallbackToNameParam(t *testing.T) {
	t.Parallel()

	bs := &imap.BodyStructureSinglePart{
		Type:    "application",
		Subtype: "octet-stream",
		Size:    1024,
		Params:  map[string]string{"name": "data.bin"},
		Extended: &imap.BodyStructureSinglePartExt{
			Disposition: &imap.BodyStructureDisposition{
				Value: "attachment",
			},
		},
	}

	atts := extractAttachments(bs)

	if len(atts) != 1 {
		t.Fatalf("expected 1 attachment, got %d", len(atts))
	}

	if atts[0].Filename != "data.bin" {
		t.Errorf("Filename = %q, want %q (should fall back to Params[name])", atts[0].Filename, "data.bin")
	}
}

// --- Coverage: Pool delegate error paths ---

func TestPool_ListFolders_AccountNotFound(t *testing.T) {
	t.Parallel()

	pool := NewPool(&config.Config{
		Accounts:       []config.Account{{ID: "acc1"}},
		DefaultAccount: "acc1",
	})

	_, err := pool.ListFolders(t.Context(), "nonexistent")
	if err == nil {
		t.Error("expected error for nonexistent account")
	}
}

func TestPool_ListMessages_AccountNotFound(t *testing.T) {
	t.Parallel()

	pool := NewPool(&config.Config{
		Accounts:       []config.Account{{ID: "acc1"}},
		DefaultAccount: "acc1",
	})

	_, err := pool.ListMessages(t.Context(), "nonexistent", "INBOX", 10, 0, false)
	if err == nil {
		t.Error("expected error for nonexistent account")
	}
}

func TestPool_ListUnread_AccountNotFound(t *testing.T) {
	t.Parallel()

	pool := NewPool(&config.Config{
		Accounts:       []config.Account{{ID: "acc1"}},
		DefaultAccount: "acc1",
	})

	_, err := pool.ListUnread(t.Context(), "nonexistent", "INBOX", 10, false)
	if err == nil {
		t.Error("expected error for nonexistent account")
	}
}

func TestPool_Search_AccountNotFound(t *testing.T) {
	t.Parallel()

	pool := NewPool(&config.Config{
		Accounts:       []config.Account{{ID: "acc1"}},
		DefaultAccount: "acc1",
	})

	_, err := pool.Search(t.Context(), "nonexistent", "INBOX", "test", "", "", "", "", 10, false)
	if err == nil {
		t.Error("expected error for nonexistent account")
	}
}

func TestPool_GetMessage_AccountNotFound(t *testing.T) {
	t.Parallel()

	pool := NewPool(&config.Config{
		Accounts:       []config.Account{{ID: "acc1"}},
		DefaultAccount: "acc1",
	})

	_, err := pool.GetMessage(t.Context(), "nonexistent", "INBOX", 1)
	if err == nil {
		t.Error("expected error for nonexistent account")
	}
}

func TestPool_MoveMessage_AccountNotFound(t *testing.T) {
	t.Parallel()

	pool := NewPool(&config.Config{
		Accounts:       []config.Account{{ID: "acc1"}},
		DefaultAccount: "acc1",
	})

	err := pool.MoveMessage(t.Context(), "nonexistent", "INBOX", 1, "Archive")
	if err == nil {
		t.Error("expected error for nonexistent account")
	}
}

func TestPool_CopyMessage_AccountNotFound(t *testing.T) {
	t.Parallel()

	pool := NewPool(&config.Config{
		Accounts:       []config.Account{{ID: "acc1"}},
		DefaultAccount: "acc1",
	})

	err := pool.CopyMessage(t.Context(), "nonexistent", "INBOX", 1, "Archive")
	if err == nil {
		t.Error("expected error for nonexistent account")
	}
}

func TestPool_DeleteMessage_AccountNotFound(t *testing.T) {
	t.Parallel()

	pool := NewPool(&config.Config{
		Accounts:       []config.Account{{ID: "acc1"}},
		DefaultAccount: "acc1",
	})

	err := pool.DeleteMessage(t.Context(), "nonexistent", "INBOX", 1, false)
	if err == nil {
		t.Error("expected error for nonexistent account")
	}
}

func TestPool_MarkRead_AccountNotFound(t *testing.T) {
	t.Parallel()

	pool := NewPool(&config.Config{
		Accounts:       []config.Account{{ID: "acc1"}},
		DefaultAccount: "acc1",
	})

	err := pool.MarkRead(t.Context(), "nonexistent", "INBOX", 1, true)
	if err == nil {
		t.Error("expected error for nonexistent account")
	}
}

func TestPool_SetFlag_AccountNotFound(t *testing.T) {
	t.Parallel()

	pool := NewPool(&config.Config{
		Accounts:       []config.Account{{ID: "acc1"}},
		DefaultAccount: "acc1",
	})

	err := pool.SetFlag(t.Context(), "nonexistent", "INBOX", 1, true)
	if err == nil {
		t.Error("expected error for nonexistent account")
	}
}

func TestPool_CreateFolder_AccountNotFound(t *testing.T) {
	t.Parallel()

	pool := NewPool(&config.Config{
		Accounts:       []config.Account{{ID: "acc1"}},
		DefaultAccount: "acc1",
	})

	err := pool.CreateFolder(t.Context(), "nonexistent", "TestFolder")
	if err == nil {
		t.Error("expected error for nonexistent account")
	}
}

func TestPool_SaveDraft_AccountNotFound(t *testing.T) {
	t.Parallel()

	pool := NewPool(&config.Config{
		Accounts:       []config.Account{{ID: "acc1"}},
		DefaultAccount: "acc1",
	})

	_, err := pool.SaveDraft(t.Context(), "nonexistent", []byte("draft"))
	if err == nil {
		t.Error("expected error for nonexistent account")
	}
}

func TestPool_GetDraft_AccountNotFound(t *testing.T) {
	t.Parallel()

	pool := NewPool(&config.Config{
		Accounts:       []config.Account{{ID: "acc1"}},
		DefaultAccount: "acc1",
	})

	_, err := pool.GetDraft(t.Context(), "nonexistent", 1)
	if err == nil {
		t.Error("expected error for nonexistent account")
	}
}

func TestPool_DeleteDraft_AccountNotFound(t *testing.T) {
	t.Parallel()

	pool := NewPool(&config.Config{
		Accounts:       []config.Account{{ID: "acc1"}},
		DefaultAccount: "acc1",
	})

	err := pool.DeleteDraft(t.Context(), "nonexistent", 1)
	if err == nil {
		t.Error("expected error for nonexistent account")
	}
}
