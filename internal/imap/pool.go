package imap

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"

	"github.com/boutquin/mcp-server-email/internal/config"
	"github.com/boutquin/mcp-server-email/internal/models"
)

// inflight represents an in-progress client creation.
// Other goroutines requesting the same account wait on the channel.
type inflight struct {
	done   chan struct{}
	client *Client
	err    error
}

// clientFactory is the function signature for creating new IMAP clients.
type clientFactory func(ctx context.Context, account *config.Account, cfg *config.Config) (*Client, error)

// ErrPoolClosed is returned when Get is called on a closed pool.
var ErrPoolClosed = errors.New("pool is closed")

// Pool manages IMAP connections for multiple accounts.
//
// Every public method that performs I/O calls wg.Add(1)/defer wg.Done() to
// track in-flight operations. Close() sets the closing flag and waits on wg
// before tearing down connections — this ensures clean shutdown without
// interrupting active IMAP commands mid-operation.
type Pool struct {
	mu            sync.Mutex
	clients       map[string]*Client
	inflight      map[string]*inflight // per-account in-progress connections (singleflight)
	cfg           *config.Config
	accounts      map[string]*config.Account
	newClientFunc clientFactory  // injectable for testing; defaults to newClient
	closing       atomic.Bool    // set when Close is called; Get rejects new requests
	wg            sync.WaitGroup // tracks in-flight pool operations for graceful shutdown
}

// NewPool creates a new IMAP connection pool.
func NewPool(cfg *config.Config) *Pool {
	accounts := make(map[string]*config.Account)

	for i := range cfg.Accounts {
		accounts[cfg.Accounts[i].ID] = &cfg.Accounts[i]
	}

	return &Pool{
		clients:       make(map[string]*Client),
		inflight:      make(map[string]*inflight),
		cfg:           cfg,
		accounts:      accounts,
		newClientFunc: newClient,
	}
}

// Get returns an IMAP client for the given account, creating one if needed.
// The pool lock is NOT held during client creation (network I/O), so concurrent
// requests for different accounts do not block each other.
// Returns ErrPoolClosed if the pool is shutting down.
func (p *Pool) Get(ctx context.Context, accountID string) (*Client, error) {
	if p.closing.Load() {
		return nil, ErrPoolClosed
	}

	if accountID == "" {
		accountID = p.cfg.DefaultAccount
	}

	p.mu.Lock()

	// Check existing connected client
	if client, ok := p.clients[accountID]; ok {
		if client.IsConnected() {
			p.mu.Unlock()

			return client, nil
		}

		// Close stale connection
		_ = client.Close()

		delete(p.clients, accountID)
	}

	// Check if another goroutine is already creating a client for this account
	if inf, ok := p.inflight[accountID]; ok {
		p.mu.Unlock()

		// Wait for the in-progress creation to finish
		<-inf.done

		return inf.client, inf.err
	}

	// Validate account exists before releasing lock
	account, ok := p.accounts[accountID]
	if !ok {
		p.mu.Unlock()

		return nil, fmt.Errorf("%w: %s", models.ErrAccountNotFound, accountID)
	}

	// Register inflight creation so other goroutines wait on us
	inf := &inflight{done: make(chan struct{})}
	p.inflight[accountID] = inf
	p.mu.Unlock()

	// Create new client WITHOUT holding the pool lock (network I/O happens here)
	client, err := p.newClientFunc(ctx, account, p.cfg)

	// Re-lock to store result and clean up inflight
	p.mu.Lock()
	delete(p.inflight, accountID)

	if err == nil {
		p.clients[accountID] = client
	}

	// Publish result for any waiting goroutines
	inf.client = client
	inf.err = err

	close(inf.done)
	p.mu.Unlock()

	return client, err
}

// AccountStatus returns connection status for all accounts.
func (p *Pool) AccountStatus() []models.AccountStatus {
	p.mu.Lock()
	defer p.mu.Unlock()

	status := make([]models.AccountStatus, 0, len(p.cfg.Accounts))

	for _, acc := range p.cfg.Accounts {
		s := models.AccountStatus{
			ID:        acc.ID,
			Email:     acc.Email,
			IsDefault: acc.ID == p.cfg.DefaultAccount,
		}

		if client, ok := p.clients[acc.ID]; ok {
			s.Connected = client.IsConnected()
		}

		status = append(status, s)
	}

	return status
}

// DefaultAccountID returns the configured default account ID.
func (p *Pool) DefaultAccountID() string {
	return p.cfg.DefaultAccount
}

// GetFolderByRole resolves a folder role to a mailbox name for the given account.
func (p *Pool) GetFolderByRole(
	ctx context.Context,
	accountID string,
	role models.FolderRole,
) (string, error) {
	p.wg.Add(1)
	defer p.wg.Done()

	client, err := p.Get(ctx, accountID)
	if err != nil {
		return "", err
	}

	name, err := client.getFolderByRole(ctx, role)
	if err != nil {
		return "", wrapIMAPError("get folder by role", err)
	}

	return name, nil
}

// ListFolders returns all mailboxes for the given account.
func (p *Pool) ListFolders(ctx context.Context, accountID string) ([]models.Folder, error) {
	p.wg.Add(1)
	defer p.wg.Done()

	client, err := p.Get(ctx, accountID)
	if err != nil {
		return nil, err
	}

	folders, err := client.ListFolders(ctx)
	if err != nil {
		return nil, wrapIMAPError("list folders", err)
	}

	return folders, nil
}

// ListMessages returns messages from a mailbox for the given account.
func (p *Pool) ListMessages(
	ctx context.Context,
	accountID, folder string,
	limit, offset int,
	includeBody bool,
) ([]models.Email, error) {
	p.wg.Add(1)
	defer p.wg.Done()

	client, err := p.Get(ctx, accountID)
	if err != nil {
		return nil, err
	}

	msgs, err := client.ListMessages(ctx, folder, limit, offset, includeBody)
	if err != nil {
		return nil, wrapIMAPError("list messages", err)
	}

	return msgs, nil
}

// ListUnread returns unread messages from a mailbox for the given account.
func (p *Pool) ListUnread(
	ctx context.Context,
	accountID, folder string,
	limit int,
	includeBody bool,
) ([]models.Email, error) {
	p.wg.Add(1)
	defer p.wg.Done()

	client, err := p.Get(ctx, accountID)
	if err != nil {
		return nil, err
	}

	msgs, err := client.ListUnread(ctx, folder, limit, includeBody)
	if err != nil {
		return nil, wrapIMAPError("list unread", err)
	}

	return msgs, nil
}

// Search searches for messages matching criteria for the given account.
func (p *Pool) Search(
	ctx context.Context,
	accountID, mailbox, query, from, to, since, before string,
	limit int,
	includeBody bool,
) ([]models.Email, error) {
	p.wg.Add(1)
	defer p.wg.Done()

	client, err := p.Get(ctx, accountID)
	if err != nil {
		return nil, err
	}

	msgs, err := client.Search(ctx, mailbox, query, from, to, since, before, limit, includeBody)
	if err != nil {
		return nil, wrapIMAPError("search", err)
	}

	return msgs, nil
}

// SearchByMessageID searches for messages by Message-ID or References header.
func (p *Pool) SearchByMessageID(
	ctx context.Context,
	accountID, folder, messageID string,
) ([]models.Email, error) {
	p.wg.Add(1)
	defer p.wg.Done()

	client, err := p.Get(ctx, accountID)
	if err != nil {
		return nil, err
	}

	msgs, err := client.SearchByMessageID(ctx, folder, messageID)
	if err != nil {
		return nil, wrapIMAPError("search by message-id", err)
	}

	return msgs, nil
}

// GetMessage retrieves a single message by UID for the given account.
func (p *Pool) GetMessage(
	ctx context.Context,
	accountID, folder string,
	uid uint32,
) (*models.Email, error) {
	p.wg.Add(1)
	defer p.wg.Done()

	client, err := p.Get(ctx, accountID)
	if err != nil {
		return nil, err
	}

	msg, err := client.GetMessage(ctx, folder, uid)
	if err != nil {
		return nil, wrapIMAPError("get message", err)
	}

	return msg, nil
}

// GetAttachments returns attachment metadata for a message.
func (p *Pool) GetAttachments(
	ctx context.Context,
	accountID, folder string,
	uid uint32,
) ([]models.AttachmentInfo, error) {
	p.wg.Add(1)
	defer p.wg.Done()

	client, err := p.Get(ctx, accountID)
	if err != nil {
		return nil, err
	}

	attachments, err := client.GetAttachments(ctx, folder, uid)
	if err != nil {
		return nil, wrapIMAPError("get attachments", err)
	}

	return attachments, nil
}

// GetAttachment downloads a specific attachment by index, returning raw bytes and filename.
func (p *Pool) GetAttachment(
	ctx context.Context,
	accountID, folder string,
	uid uint32,
	index int,
) ([]byte, string, error) {
	p.wg.Add(1)
	defer p.wg.Done()

	client, err := p.Get(ctx, accountID)
	if err != nil {
		return nil, "", err
	}

	data, filename, err := client.GetAttachment(ctx, folder, uid, index)
	if err != nil {
		return nil, "", wrapIMAPError("get attachment", err)
	}

	return data, filename, nil
}

// MoveMessage moves a message to a different mailbox for the given account.
func (p *Pool) MoveMessage(
	ctx context.Context,
	accountID, folder string,
	uid uint32,
	dest string,
) error {
	p.wg.Add(1)
	defer p.wg.Done()

	client, err := p.Get(ctx, accountID)
	if err != nil {
		return err
	}

	err = client.MoveMessage(ctx, folder, uid, dest)
	if err != nil {
		return wrapIMAPError("move message", err)
	}

	return nil
}

// CopyMessage copies a message to a different mailbox for the given account.
func (p *Pool) CopyMessage(
	ctx context.Context,
	accountID, folder string,
	uid uint32,
	dest string,
) error {
	p.wg.Add(1)
	defer p.wg.Done()

	client, err := p.Get(ctx, accountID)
	if err != nil {
		return err
	}

	err = client.CopyMessage(ctx, folder, uid, dest)
	if err != nil {
		return wrapIMAPError("copy message", err)
	}

	return nil
}

// DeleteMessage deletes a message for the given account.
func (p *Pool) DeleteMessage(
	ctx context.Context,
	accountID, folder string,
	uid uint32,
	permanent bool,
) error {
	p.wg.Add(1)
	defer p.wg.Done()

	client, err := p.Get(ctx, accountID)
	if err != nil {
		return err
	}

	err = client.DeleteMessage(ctx, folder, uid, permanent)
	if err != nil {
		return wrapIMAPError("delete message", err)
	}

	return nil
}

// MarkRead marks a message as read or unread for the given account.
func (p *Pool) MarkRead(
	ctx context.Context,
	accountID, folder string,
	uid uint32,
	read bool,
) error {
	p.wg.Add(1)
	defer p.wg.Done()

	client, err := p.Get(ctx, accountID)
	if err != nil {
		return err
	}

	err = client.MarkRead(ctx, folder, uid, read)
	if err != nil {
		return wrapIMAPError("mark read", err)
	}

	return nil
}

// SetFlag sets or clears the flagged status for the given account.
func (p *Pool) SetFlag(
	ctx context.Context,
	accountID, folder string,
	uid uint32,
	flagged bool,
) error {
	p.wg.Add(1)
	defer p.wg.Done()

	client, err := p.Get(ctx, accountID)
	if err != nil {
		return err
	}

	err = client.SetFlag(ctx, folder, uid, flagged)
	if err != nil {
		return wrapIMAPError("set flag", err)
	}

	return nil
}

// CreateFolder creates a new mailbox for the given account.
func (p *Pool) CreateFolder(ctx context.Context, accountID, name string) error {
	p.wg.Add(1)
	defer p.wg.Done()

	client, err := p.Get(ctx, accountID)
	if err != nil {
		return err
	}

	err = client.CreateFolder(ctx, name)
	if err != nil {
		return wrapIMAPError("create folder", err)
	}

	return nil
}

// SaveDraft saves a message as a draft for the given account.
func (p *Pool) SaveDraft(ctx context.Context, accountID string, msg []byte) (uint32, error) {
	p.wg.Add(1)
	defer p.wg.Done()

	client, err := p.Get(ctx, accountID)
	if err != nil {
		return 0, err
	}

	uid, err := client.SaveDraft(ctx, msg)
	if err != nil {
		return 0, wrapIMAPError("save draft", err)
	}

	return uid, nil
}

// GetDraft retrieves a draft message for the given account.
func (p *Pool) GetDraft(ctx context.Context, accountID string, uid uint32) ([]byte, error) {
	p.wg.Add(1)
	defer p.wg.Done()

	client, err := p.Get(ctx, accountID)
	if err != nil {
		return nil, err
	}

	draft, err := client.GetDraft(ctx, uid)
	if err != nil {
		return nil, wrapIMAPError("get draft", err)
	}

	return draft, nil
}

// DeleteDraft deletes a draft for the given account.
func (p *Pool) DeleteDraft(ctx context.Context, accountID string, uid uint32) error {
	p.wg.Add(1)
	defer p.wg.Done()

	client, err := p.Get(ctx, accountID)
	if err != nil {
		return err
	}

	err = client.DeleteDraft(ctx, uid)
	if err != nil {
		return wrapIMAPError("delete draft", err)
	}

	return nil
}

// Close gracefully shuts down the pool. It rejects new Get calls, waits for
// in-flight operations to complete (or until ctx is cancelled), then closes
// all connections. On timeout, connections are force-closed to unblock hung
// operations.
func (p *Pool) Close(ctx context.Context) {
	p.closing.Store(true)

	// Wait for in-flight operations to complete or context deadline.
	done := make(chan struct{})

	go func() {
		p.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		// All in-flight ops completed — close connections cleanly.
	case <-ctx.Done():
		// Timeout — force-close raw connections to unblock hung operations.
		// This is best-effort; the process is about to exit.
		p.mu.Lock()

		for _, client := range p.clients {
			client.forceClose()
		}

		p.mu.Unlock()

		return
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	for _, client := range p.clients {
		_ = client.Close()
	}

	p.clients = make(map[string]*Client)
	p.inflight = make(map[string]*inflight)
}
