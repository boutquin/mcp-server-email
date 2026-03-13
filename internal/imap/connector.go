package imap

import (
	"fmt"

	"github.com/emersion/go-imap/v2"
	"github.com/emersion/go-imap/v2/imapclient"
	"github.com/emersion/go-sasl"
)

// Connector abstracts the imapclient.Client methods used by Client.
// This enables mock injection for unit testing without a live IMAP server.
//
// Design decision: Uses a high-level interface that collapses the two-step
// `cmd := conn.Method(); result, err := cmd.Wait()` pattern into single calls.
// This is necessary because *imapclient.XxxCommand types require an active
// connection for their Wait()/Next()/Collect() methods to function — they
// cannot be meaningfully constructed in test code.
//
//nolint:interfacebloat,dupl // 14 methods required: maps 1:1 to imapclient.Client methods used by Client
type Connector interface {
	// Login performs IMAP LOGIN command (password authentication).
	Login(username, password string) error

	// Authenticate performs SASL authentication (e.g., XOAUTH2).
	Authenticate(saslClient sasl.Client) error

	// Select selects a mailbox.
	Select(mailbox string, opts *imap.SelectOptions) (*imap.SelectData, error)

	// List lists mailboxes matching a pattern and collects all results.
	List(ref, pattern string, opts *imap.ListOptions) ([]*imap.ListData, error)

	// Status gets mailbox status.
	Status(mailbox string, opts *imap.StatusOptions) (*imap.StatusData, error)

	// Fetch fetches messages and collects all results into buffers.
	Fetch(seqSet imap.NumSet, opts *imap.FetchOptions) ([]*imapclient.FetchMessageBuffer, error)

	// Search searches for messages matching criteria.
	Search(criteria *imap.SearchCriteria, opts *imap.SearchOptions) (*imap.SearchData, error)

	// Move moves messages to a destination mailbox.
	Move(uidSet imap.UIDSet, dest string) error

	// Copy copies messages to a destination mailbox.
	Copy(uidSet imap.UIDSet, dest string) error

	// Store modifies message flags, draining all results.
	Store(seqSet imap.NumSet, flags *imap.StoreFlags, opts *imap.StoreOptions) error

	// Expunge permanently removes messages marked as deleted.
	Expunge() error

	// Append appends a message literal to a mailbox.
	Append(mailbox string, literal []byte, opts *imap.AppendOptions) (*imap.AppendData, error)

	// Create creates a new mailbox.
	Create(name string, opts *imap.CreateOptions) error

	// Close closes the underlying connection.
	Close() error
}

// Compile-time check: imapclientAdapter implements Connector.
var _ Connector = (*imapclientAdapter)(nil)

// imapclientAdapter wraps *imapclient.Client to implement Connector.
// In production, this is a thin pass-through that collapses the two-step
// command pattern (create command + wait/collect) into single calls.
type imapclientAdapter struct {
	client *imapclient.Client
}

func (a *imapclientAdapter) Login(username, password string) error {
	err := a.client.Login(username, password).Wait()
	if err != nil {
		return fmt.Errorf("login: %w", err)
	}

	return nil
}

func (a *imapclientAdapter) Authenticate(saslClient sasl.Client) error {
	err := a.client.Authenticate(saslClient)
	if err != nil {
		return fmt.Errorf("authenticate: %w", err)
	}

	return nil
}

func (a *imapclientAdapter) Select(
	mailbox string, opts *imap.SelectOptions,
) (*imap.SelectData, error) {
	data, err := a.client.Select(mailbox, opts).Wait()
	if err != nil {
		return nil, fmt.Errorf("select: %w", err)
	}

	return data, nil
}

func (a *imapclientAdapter) List(
	ref, pattern string, opts *imap.ListOptions,
) ([]*imap.ListData, error) {
	data, err := a.client.List(ref, pattern, opts).Collect()
	if err != nil {
		return nil, fmt.Errorf("list: %w", err)
	}

	return data, nil
}

func (a *imapclientAdapter) Status(
	mailbox string, opts *imap.StatusOptions,
) (*imap.StatusData, error) {
	data, err := a.client.Status(mailbox, opts).Wait()
	if err != nil {
		return nil, fmt.Errorf("status: %w", err)
	}

	return data, nil
}

func (a *imapclientAdapter) Fetch(
	seqSet imap.NumSet, opts *imap.FetchOptions,
) ([]*imapclient.FetchMessageBuffer, error) {
	data, err := a.client.Fetch(seqSet, opts).Collect()
	if err != nil {
		return nil, fmt.Errorf("fetch: %w", err)
	}

	return data, nil
}

func (a *imapclientAdapter) Search(
	criteria *imap.SearchCriteria, opts *imap.SearchOptions,
) (*imap.SearchData, error) {
	data, err := a.client.Search(criteria, opts).Wait()
	if err != nil {
		return nil, fmt.Errorf("search: %w", err)
	}

	return data, nil
}

func (a *imapclientAdapter) Move(uidSet imap.UIDSet, dest string) error {
	_, err := a.client.Move(uidSet, dest).Wait()
	if err != nil {
		return fmt.Errorf("move: %w", err)
	}

	return nil
}

func (a *imapclientAdapter) Copy(uidSet imap.UIDSet, dest string) error {
	_, err := a.client.Copy(uidSet, dest).Wait()
	if err != nil {
		return fmt.Errorf("copy: %w", err)
	}

	return nil
}

func (a *imapclientAdapter) Store(
	seqSet imap.NumSet, flags *imap.StoreFlags, opts *imap.StoreOptions,
) error {
	cmd := a.client.Store(seqSet, flags, opts)
	// Drain all results
	for cmd.Next() != nil {
	}

	err := cmd.Close()
	if err != nil {
		return fmt.Errorf("store: %w", err)
	}

	return nil
}

func (a *imapclientAdapter) Expunge() error {
	cmd := a.client.Expunge()
	// Drain all results
	for cmd.Next() != 0 {
	}

	err := cmd.Close()
	if err != nil {
		return fmt.Errorf("expunge: %w", err)
	}

	return nil
}

func (a *imapclientAdapter) Append(
	mailbox string, literal []byte, opts *imap.AppendOptions,
) (*imap.AppendData, error) {
	cmd := a.client.Append(mailbox, int64(len(literal)), opts)

	_, err := cmd.Write(literal)
	if err != nil {
		return nil, fmt.Errorf("write literal: %w", err)
	}

	err = cmd.Close()
	if err != nil {
		return nil, fmt.Errorf("close literal: %w", err)
	}

	data, err := cmd.Wait()
	if err != nil {
		return nil, fmt.Errorf("append: %w", err)
	}

	return data, nil
}

func (a *imapclientAdapter) Create(name string, opts *imap.CreateOptions) error {
	err := a.client.Create(name, opts).Wait()
	if err != nil {
		return fmt.Errorf("create: %w", err)
	}

	return nil
}

func (a *imapclientAdapter) Close() error {
	err := a.client.Close()
	if err != nil {
		return fmt.Errorf("close: %w", err)
	}

	return nil
}
