package imap

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/boutquin/mcp-server-email/internal/config"
	"github.com/boutquin/mcp-server-email/internal/models"
	"github.com/emersion/go-imap/v2"
	"github.com/emersion/go-imap/v2/imapclient"
)

// --- Pool Test Helpers ---

const testAccountID = "test-account"

// newTestPool creates a Pool with a pre-populated mock client for the given account.
// The mockConnector controls all IMAP responses without real network I/O.
func newTestPool(t *testing.T, mc *mockConnector) *Pool {
	t.Helper()

	cfg := &config.Config{
		DefaultAccount: testAccountID,
		Accounts: []config.Account{{
			ID:       testAccountID,
			IMAPHost: "localhost",
			IMAPPort: 993,
			Username: "test",
			Password: "test",
			Email:    "test@example.com",
		}},
	}

	pool := NewPool(cfg)
	pool.clients[testAccountID] = newMockClient(mc)

	return pool
}

// errTestPool is a sentinel error for pool-level tests.
var errTestPool = errors.New("pool test error")

// ==================== Pool.Get ====================

func TestPool_Get_ExistingClient(t *testing.T) {
	t.Parallel()

	pool := newTestPool(t, &mockConnector{})

	client, err := pool.Get(context.Background(), testAccountID)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}

	if client == nil {
		t.Fatal("expected non-nil client")
	}
}

func TestPool_Get_DefaultAccount(t *testing.T) {
	t.Parallel()

	pool := newTestPool(t, &mockConnector{})

	client, err := pool.Get(context.Background(), "")
	if err != nil {
		t.Fatalf("Get('') error = %v", err)
	}

	if client == nil {
		t.Fatal("expected non-nil client for default account")
	}
}

func TestPool_Get_UnknownAccount(t *testing.T) {
	t.Parallel()

	pool := newTestPool(t, &mockConnector{})

	_, err := pool.Get(context.Background(), "nonexistent")
	if !errors.Is(err, models.ErrAccountNotFound) {
		t.Errorf("expected ErrAccountNotFound, got: %v", err)
	}
}

func TestPool_Get_AfterClose(t *testing.T) {
	t.Parallel()

	pool := newTestPool(t, &mockConnector{})
	pool.Close(context.Background())

	_, err := pool.Get(context.Background(), testAccountID)
	if !errors.Is(err, ErrPoolClosed) {
		t.Errorf("expected ErrPoolClosed, got: %v", err)
	}
}

func TestPool_Get_CreatesNewClient(t *testing.T) {
	t.Parallel()

	cfg := &config.Config{
		DefaultAccount: testAccountID,
		Accounts: []config.Account{{
			ID:       testAccountID,
			IMAPHost: "localhost",
			IMAPPort: 993,
			Username: "test",
			Password: "test",
		}},
	}

	pool := NewPool(cfg)
	pool.newClientFunc = func(
		_ context.Context, _ *config.Account, _ *config.Config,
	) (*Client, error) {
		return newMockClient(&mockConnector{}), nil
	}

	client, err := pool.Get(context.Background(), testAccountID)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}

	if client == nil {
		t.Fatal("expected non-nil client from factory")
	}
}

func TestPool_Get_FactoryError(t *testing.T) {
	t.Parallel()

	cfg := &config.Config{
		DefaultAccount: testAccountID,
		Accounts: []config.Account{{
			ID: testAccountID, IMAPHost: "localhost", IMAPPort: 993,
			Username: "test", Password: "test",
		}},
	}

	pool := NewPool(cfg)
	pool.newClientFunc = func(
		_ context.Context, _ *config.Account, _ *config.Config,
	) (*Client, error) {
		return nil, errTestPool
	}

	_, err := pool.Get(context.Background(), testAccountID)
	if !errors.Is(err, errTestPool) {
		t.Errorf("expected errTestPool, got: %v", err)
	}
}

// ==================== Pool.AccountStatus ====================

func TestPool_AccountStatus_Connected(t *testing.T) {
	t.Parallel()

	pool := newTestPool(t, &mockConnector{})

	statuses := pool.AccountStatus()
	if len(statuses) != 1 {
		t.Fatalf("expected 1 status, got %d", len(statuses))
	}

	if !statuses[0].Connected {
		t.Error("expected Connected=true for pre-populated client")
	}

	if !statuses[0].IsDefault {
		t.Error("expected IsDefault=true")
	}
}

func TestPool_AccountStatus_NotConnected(t *testing.T) {
	t.Parallel()

	cfg := &config.Config{
		DefaultAccount: testAccountID,
		Accounts: []config.Account{{
			ID:    testAccountID,
			Email: "test@example.com",
		}},
	}

	pool := NewPool(cfg)

	statuses := pool.AccountStatus()
	if len(statuses) != 1 {
		t.Fatalf("expected 1 status, got %d", len(statuses))
	}

	if statuses[0].Connected {
		t.Error("expected Connected=false with no client in pool")
	}
}

// ==================== Pool.DefaultAccountID ====================

func TestPool_DefaultAccountID(t *testing.T) {
	t.Parallel()

	pool := newTestPool(t, &mockConnector{})

	if got := pool.DefaultAccountID(); got != testAccountID {
		t.Errorf("DefaultAccountID() = %q, want %q", got, testAccountID)
	}
}

// ==================== Pool.ListFolders ====================

func TestPool_ListFolders_Success(t *testing.T) {
	t.Parallel()

	total := uint32(5)

	mc := &mockConnector{
		listFn: func(_, _ string, _ *imap.ListOptions) ([]*imap.ListData, error) {
			return []*imap.ListData{{Mailbox: testMailboxINBOX}}, nil
		},
		statusFn: func(_ string, _ *imap.StatusOptions) (*imap.StatusData, error) {
			return &imap.StatusData{NumMessages: &total}, nil
		},
	}

	pool := newTestPool(t, mc)

	folders, err := pool.ListFolders(context.Background(), testAccountID)
	if err != nil {
		t.Fatalf("ListFolders() error = %v", err)
	}

	if len(folders) != 1 {
		t.Fatalf("expected 1 folder, got %d", len(folders))
	}
}

func TestPool_ListFolders_Error(t *testing.T) {
	t.Parallel()

	mc := &mockConnector{
		listFn: func(_, _ string, _ *imap.ListOptions) ([]*imap.ListData, error) {
			return nil, errTestPool
		},
	}

	pool := newTestPool(t, mc)

	_, err := pool.ListFolders(context.Background(), testAccountID)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestPool_ListFolders_UnknownAccount(t *testing.T) {
	t.Parallel()

	pool := newTestPool(t, &mockConnector{})

	_, err := pool.ListFolders(context.Background(), "bad-account")
	if !errors.Is(err, models.ErrAccountNotFound) {
		t.Errorf("expected ErrAccountNotFound, got: %v", err)
	}
}

// ==================== Pool.ListMessages ====================

func TestPool_ListMessages_Success(t *testing.T) {
	t.Parallel()

	mc := &mockConnector{
		selectFn: func(_ string, _ *imap.SelectOptions) (*imap.SelectData, error) {
			return &imap.SelectData{NumMessages: 5}, nil
		},
		fetchFn: func(_ imap.NumSet, _ *imap.FetchOptions) ([]*imapclient.FetchMessageBuffer, error) {
			return []*imapclient.FetchMessageBuffer{
				{UID: 1, Envelope: &imap.Envelope{Subject: "Msg"}},
			}, nil
		},
	}

	pool := newTestPool(t, mc)

	msgs, err := pool.ListMessages(context.Background(), testAccountID, "INBOX", 10, 0, false)
	if err != nil {
		t.Fatalf("ListMessages() error = %v", err)
	}

	if len(msgs) != 1 {
		t.Fatalf("expected 1 message, got %d", len(msgs))
	}
}

func TestPool_ListMessages_Error(t *testing.T) {
	t.Parallel()

	mc := &mockConnector{
		selectFn: func(_ string, _ *imap.SelectOptions) (*imap.SelectData, error) {
			return nil, errTestPool
		},
	}

	pool := newTestPool(t, mc)

	_, err := pool.ListMessages(context.Background(), testAccountID, "INBOX", 10, 0, false)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestPool_ListMessages_UnknownAccount(t *testing.T) {
	t.Parallel()

	pool := newTestPool(t, &mockConnector{})

	_, err := pool.ListMessages(context.Background(), "bad", "INBOX", 10, 0, false)
	if !errors.Is(err, models.ErrAccountNotFound) {
		t.Errorf("expected ErrAccountNotFound, got: %v", err)
	}
}

// ==================== Pool.ListUnread ====================

func TestPool_ListUnread_Success(t *testing.T) {
	t.Parallel()

	mc := &mockConnector{
		searchFn: func(_ *imap.SearchCriteria, _ *imap.SearchOptions) (*imap.SearchData, error) {
			return &imap.SearchData{All: seqSetFrom(1)}, nil
		},
		fetchFn: func(_ imap.NumSet, _ *imap.FetchOptions) ([]*imapclient.FetchMessageBuffer, error) {
			return []*imapclient.FetchMessageBuffer{
				{UID: 1, Envelope: &imap.Envelope{Subject: "Unread"}},
			}, nil
		},
	}

	pool := newTestPool(t, mc)

	msgs, err := pool.ListUnread(context.Background(), testAccountID, "INBOX", 10, false)
	if err != nil {
		t.Fatalf("ListUnread() error = %v", err)
	}

	if len(msgs) != 1 {
		t.Fatalf("expected 1 message, got %d", len(msgs))
	}
}

func TestPool_ListUnread_Error(t *testing.T) {
	t.Parallel()

	mc := &mockConnector{
		searchFn: func(_ *imap.SearchCriteria, _ *imap.SearchOptions) (*imap.SearchData, error) {
			return nil, errTestPool
		},
	}

	pool := newTestPool(t, mc)

	_, err := pool.ListUnread(context.Background(), testAccountID, "INBOX", 10, false)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestPool_ListUnread_UnknownAccount(t *testing.T) {
	t.Parallel()

	pool := newTestPool(t, &mockConnector{})

	_, err := pool.ListUnread(context.Background(), "bad", "INBOX", 10, false)
	if !errors.Is(err, models.ErrAccountNotFound) {
		t.Errorf("expected ErrAccountNotFound, got: %v", err)
	}
}

// ==================== Pool.Search ====================

func TestPool_Search_Success(t *testing.T) {
	t.Parallel()

	mc := &mockConnector{
		searchFn: func(_ *imap.SearchCriteria, _ *imap.SearchOptions) (*imap.SearchData, error) {
			return &imap.SearchData{All: seqSetFrom(1)}, nil
		},
		fetchFn: func(_ imap.NumSet, _ *imap.FetchOptions) ([]*imapclient.FetchMessageBuffer, error) {
			return []*imapclient.FetchMessageBuffer{
				{UID: 1, Envelope: &imap.Envelope{Subject: "Found"}},
			}, nil
		},
	}

	pool := newTestPool(t, mc)

	msgs, err := pool.Search(
		context.Background(), testAccountID, "INBOX", "test", "", "", "", "", 50, false,
	)
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}

	if len(msgs) != 1 {
		t.Fatalf("expected 1 message, got %d", len(msgs))
	}
}

func TestPool_Search_Error(t *testing.T) {
	t.Parallel()

	mc := &mockConnector{
		searchFn: func(_ *imap.SearchCriteria, _ *imap.SearchOptions) (*imap.SearchData, error) {
			return nil, errTestPool
		},
	}

	pool := newTestPool(t, mc)

	_, err := pool.Search(
		context.Background(), testAccountID, "INBOX", "test", "", "", "", "", 50, false,
	)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestPool_Search_UnknownAccount(t *testing.T) {
	t.Parallel()

	pool := newTestPool(t, &mockConnector{})

	_, err := pool.Search(
		context.Background(), "bad", "INBOX", "test", "", "", "", "", 50, false,
	)
	if !errors.Is(err, models.ErrAccountNotFound) {
		t.Errorf("expected ErrAccountNotFound, got: %v", err)
	}
}

// ==================== Pool.SearchByMessageID ====================

func TestPool_SearchByMessageID_Success(t *testing.T) {
	t.Parallel()

	mc := &mockConnector{
		searchFn: func(_ *imap.SearchCriteria, _ *imap.SearchOptions) (*imap.SearchData, error) {
			return &imap.SearchData{All: seqSetFrom(1)}, nil
		},
		fetchFn: func(_ imap.NumSet, _ *imap.FetchOptions) ([]*imapclient.FetchMessageBuffer, error) {
			return []*imapclient.FetchMessageBuffer{
				{UID: 1, Envelope: &imap.Envelope{Subject: "Threaded"}},
			}, nil
		},
	}

	pool := newTestPool(t, mc)

	msgs, err := pool.SearchByMessageID(
		context.Background(), testAccountID, "INBOX", "<abc@example.com>",
	)
	if err != nil {
		t.Fatalf("SearchByMessageID() error = %v", err)
	}

	if len(msgs) != 1 {
		t.Fatalf("expected 1 message, got %d", len(msgs))
	}
}

func TestPool_SearchByMessageID_Error(t *testing.T) {
	t.Parallel()

	mc := &mockConnector{
		searchFn: func(_ *imap.SearchCriteria, _ *imap.SearchOptions) (*imap.SearchData, error) {
			return nil, errTestPool
		},
	}

	pool := newTestPool(t, mc)

	_, err := pool.SearchByMessageID(
		context.Background(), testAccountID, "INBOX", "<abc@example.com>",
	)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestPool_SearchByMessageID_UnknownAccount(t *testing.T) {
	t.Parallel()

	pool := newTestPool(t, &mockConnector{})

	_, err := pool.SearchByMessageID(
		context.Background(), "bad", "INBOX", "<abc@example.com>",
	)
	if !errors.Is(err, models.ErrAccountNotFound) {
		t.Errorf("expected ErrAccountNotFound, got: %v", err)
	}
}

// ==================== Pool.GetMessage ====================

func TestPool_GetMessage_Success(t *testing.T) {
	t.Parallel()

	mc := &mockConnector{
		fetchFn: func(_ imap.NumSet, _ *imap.FetchOptions) ([]*imapclient.FetchMessageBuffer, error) {
			return []*imapclient.FetchMessageBuffer{
				{
					UID:      42,
					Envelope: &imap.Envelope{Subject: "Test"},
				},
			}, nil
		},
	}

	pool := newTestPool(t, mc)

	msg, err := pool.GetMessage(context.Background(), testAccountID, "INBOX", 42)
	if err != nil {
		t.Fatalf("GetMessage() error = %v", err)
	}

	if msg.Subject != "Test" {
		t.Errorf("Subject = %q, want %q", msg.Subject, "Test")
	}
}

func TestPool_GetMessage_Error(t *testing.T) {
	t.Parallel()

	mc := &mockConnector{
		fetchFn: func(_ imap.NumSet, _ *imap.FetchOptions) ([]*imapclient.FetchMessageBuffer, error) {
			return nil, errTestPool
		},
	}

	pool := newTestPool(t, mc)

	_, err := pool.GetMessage(context.Background(), testAccountID, "INBOX", 42)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestPool_GetMessage_UnknownAccount(t *testing.T) {
	t.Parallel()

	pool := newTestPool(t, &mockConnector{})

	_, err := pool.GetMessage(context.Background(), "bad", "INBOX", 42)
	if !errors.Is(err, models.ErrAccountNotFound) {
		t.Errorf("expected ErrAccountNotFound, got: %v", err)
	}
}

// ==================== Pool.GetAttachments ====================

func TestPool_GetAttachments_Success(t *testing.T) {
	t.Parallel()

	mc := &mockConnector{
		fetchFn: func(_ imap.NumSet, _ *imap.FetchOptions) ([]*imapclient.FetchMessageBuffer, error) {
			return []*imapclient.FetchMessageBuffer{
				{
					UID: 42,
					BodyStructure: &imap.BodyStructureMultiPart{
						Children: []imap.BodyStructure{
							&imap.BodyStructureSinglePart{
								Type:    "TEXT",
								Subtype: "PLAIN",
							},
							&imap.BodyStructureSinglePart{
								Type:    "APPLICATION",
								Subtype: "PDF",
								Extended: &imap.BodyStructureSinglePartExt{
									Disposition: &imap.BodyStructureDisposition{
										Value:  "attachment",
										Params: map[string]string{"filename": "doc.pdf"},
									},
								},
							},
						},
					},
				},
			}, nil
		},
	}

	pool := newTestPool(t, mc)

	atts, err := pool.GetAttachments(context.Background(), testAccountID, "INBOX", 42)
	if err != nil {
		t.Fatalf("GetAttachments() error = %v", err)
	}

	if len(atts) != 1 {
		t.Fatalf("expected 1 attachment, got %d", len(atts))
	}
}

func TestPool_GetAttachments_Error(t *testing.T) {
	t.Parallel()

	mc := &mockConnector{
		fetchFn: func(_ imap.NumSet, _ *imap.FetchOptions) ([]*imapclient.FetchMessageBuffer, error) {
			return nil, errTestPool
		},
	}

	pool := newTestPool(t, mc)

	_, err := pool.GetAttachments(context.Background(), testAccountID, "INBOX", 42)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestPool_GetAttachments_UnknownAccount(t *testing.T) {
	t.Parallel()

	pool := newTestPool(t, &mockConnector{})

	_, err := pool.GetAttachments(context.Background(), "bad", "INBOX", 42)
	if !errors.Is(err, models.ErrAccountNotFound) {
		t.Errorf("expected ErrAccountNotFound, got: %v", err)
	}
}

// ==================== Pool.GetAttachment ====================

func TestPool_GetAttachment_Success(t *testing.T) {
	t.Parallel()

	callCount := 0
	mimeMsg := "MIME-Version: 1.0\r\n" +
		"Content-Type: multipart/mixed; boundary=boundary42\r\n\r\n" +
		"--boundary42\r\nContent-Type: text/plain\r\n\r\nHello\r\n" +
		"--boundary42\r\nContent-Type: application/pdf\r\n" +
		"Content-Disposition: attachment; filename=\"doc.pdf\"\r\n\r\n" +
		"pdf-bytes\r\n--boundary42--\r\n"

	mc := &mockConnector{
		fetchFn: func(_ imap.NumSet, _ *imap.FetchOptions) ([]*imapclient.FetchMessageBuffer, error) {
			callCount++

			if callCount == 1 {
				return []*imapclient.FetchMessageBuffer{
					{
						UID: 42,
						BodyStructure: &imap.BodyStructureMultiPart{
							Children: []imap.BodyStructure{
								&imap.BodyStructureSinglePart{Type: "TEXT", Subtype: "PLAIN"},
								&imap.BodyStructureSinglePart{
									Type:    "APPLICATION",
									Subtype: "PDF",
									Extended: &imap.BodyStructureSinglePartExt{
										Disposition: &imap.BodyStructureDisposition{
											Value:  "attachment",
											Params: map[string]string{"filename": "doc.pdf"},
										},
									},
								},
							},
						},
					},
				}, nil
			}

			return []*imapclient.FetchMessageBuffer{
				{
					UID: 42,
					BodySection: []imapclient.FetchBodySectionBuffer{
						{Bytes: []byte(mimeMsg)},
					},
				},
			}, nil
		},
	}

	pool := newTestPool(t, mc)

	data, filename, err := pool.GetAttachment(
		context.Background(), testAccountID, "INBOX", 42, 0,
	)
	if err != nil {
		t.Fatalf("GetAttachment() error = %v", err)
	}

	if filename != "doc.pdf" {
		t.Errorf("filename = %q, want %q", filename, "doc.pdf")
	}

	if len(data) == 0 {
		t.Error("expected non-empty data")
	}
}

func TestPool_GetAttachment_Error(t *testing.T) {
	t.Parallel()

	mc := &mockConnector{
		fetchFn: func(_ imap.NumSet, _ *imap.FetchOptions) ([]*imapclient.FetchMessageBuffer, error) {
			return nil, errTestPool
		},
	}

	pool := newTestPool(t, mc)

	_, _, err := pool.GetAttachment(
		context.Background(), testAccountID, "INBOX", 42, 0,
	)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestPool_GetAttachment_UnknownAccount(t *testing.T) {
	t.Parallel()

	pool := newTestPool(t, &mockConnector{})

	_, _, err := pool.GetAttachment(
		context.Background(), "bad", "INBOX", 42, 0,
	)
	if !errors.Is(err, models.ErrAccountNotFound) {
		t.Errorf("expected ErrAccountNotFound, got: %v", err)
	}
}

// ==================== Pool.MoveMessage ====================

func TestPool_MoveMessage_Success(t *testing.T) {
	t.Parallel()

	mc := &mockConnector{
		moveFn: func(_ imap.UIDSet, dest string) error {
			if dest != "Archive" {
				t.Errorf("dest = %q, want %q", dest, "Archive")
			}

			return nil
		},
	}

	pool := newTestPool(t, mc)

	err := pool.MoveMessage(context.Background(), testAccountID, "INBOX", 42, "Archive")
	if err != nil {
		t.Fatalf("MoveMessage() error = %v", err)
	}
}

func TestPool_MoveMessage_Error(t *testing.T) {
	t.Parallel()

	mc := &mockConnector{
		moveFn: func(_ imap.UIDSet, _ string) error {
			return errTestPool
		},
	}

	pool := newTestPool(t, mc)

	err := pool.MoveMessage(context.Background(), testAccountID, "INBOX", 42, "Archive")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestPool_MoveMessage_UnknownAccount(t *testing.T) {
	t.Parallel()

	pool := newTestPool(t, &mockConnector{})

	err := pool.MoveMessage(context.Background(), "bad", "INBOX", 42, "Archive")
	if !errors.Is(err, models.ErrAccountNotFound) {
		t.Errorf("expected ErrAccountNotFound, got: %v", err)
	}
}

// ==================== Pool.CopyMessage ====================

func TestPool_CopyMessage_Success(t *testing.T) {
	t.Parallel()

	mc := &mockConnector{
		copyFn: func(_ imap.UIDSet, dest string) error {
			if dest != "Backup" {
				t.Errorf("dest = %q, want %q", dest, "Backup")
			}

			return nil
		},
	}

	pool := newTestPool(t, mc)

	err := pool.CopyMessage(context.Background(), testAccountID, "INBOX", 42, "Backup")
	if err != nil {
		t.Fatalf("CopyMessage() error = %v", err)
	}
}

func TestPool_CopyMessage_Error(t *testing.T) {
	t.Parallel()

	mc := &mockConnector{
		copyFn: func(_ imap.UIDSet, _ string) error {
			return errTestPool
		},
	}

	pool := newTestPool(t, mc)

	err := pool.CopyMessage(context.Background(), testAccountID, "INBOX", 42, "Backup")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestPool_CopyMessage_UnknownAccount(t *testing.T) {
	t.Parallel()

	pool := newTestPool(t, &mockConnector{})

	err := pool.CopyMessage(context.Background(), "bad", "INBOX", 42, "Backup")
	if !errors.Is(err, models.ErrAccountNotFound) {
		t.Errorf("expected ErrAccountNotFound, got: %v", err)
	}
}

// ==================== Pool.DeleteMessage ====================

func TestPool_DeleteMessage_Success(t *testing.T) {
	t.Parallel()

	mc := &mockConnector{
		storeFn: func(_ imap.NumSet, _ *imap.StoreFlags, _ *imap.StoreOptions) error {
			return nil
		},
		expungeFn: func() error { return nil },
	}

	pool := newTestPool(t, mc)

	err := pool.DeleteMessage(context.Background(), testAccountID, "INBOX", 42, true)
	if err != nil {
		t.Fatalf("DeleteMessage() error = %v", err)
	}
}

func TestPool_DeleteMessage_Error(t *testing.T) {
	t.Parallel()

	mc := &mockConnector{
		storeFn: func(_ imap.NumSet, _ *imap.StoreFlags, _ *imap.StoreOptions) error {
			return errTestPool
		},
	}

	pool := newTestPool(t, mc)

	err := pool.DeleteMessage(context.Background(), testAccountID, "INBOX", 42, true)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestPool_DeleteMessage_UnknownAccount(t *testing.T) {
	t.Parallel()

	pool := newTestPool(t, &mockConnector{})

	err := pool.DeleteMessage(context.Background(), "bad", "INBOX", 42, true)
	if !errors.Is(err, models.ErrAccountNotFound) {
		t.Errorf("expected ErrAccountNotFound, got: %v", err)
	}
}

// ==================== Pool.MarkRead ====================

func TestPool_MarkRead_Success(t *testing.T) {
	t.Parallel()

	mc := &mockConnector{
		storeFn: func(_ imap.NumSet, _ *imap.StoreFlags, _ *imap.StoreOptions) error {
			return nil
		},
	}

	pool := newTestPool(t, mc)

	err := pool.MarkRead(context.Background(), testAccountID, "INBOX", 42, true)
	if err != nil {
		t.Fatalf("MarkRead() error = %v", err)
	}
}

func TestPool_MarkRead_Error(t *testing.T) {
	t.Parallel()

	mc := &mockConnector{
		storeFn: func(_ imap.NumSet, _ *imap.StoreFlags, _ *imap.StoreOptions) error {
			return errTestPool
		},
	}

	pool := newTestPool(t, mc)

	err := pool.MarkRead(context.Background(), testAccountID, "INBOX", 42, true)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestPool_MarkRead_UnknownAccount(t *testing.T) {
	t.Parallel()

	pool := newTestPool(t, &mockConnector{})

	err := pool.MarkRead(context.Background(), "bad", "INBOX", 42, true)
	if !errors.Is(err, models.ErrAccountNotFound) {
		t.Errorf("expected ErrAccountNotFound, got: %v", err)
	}
}

// ==================== Pool.SetFlag ====================

func TestPool_SetFlag_Success(t *testing.T) {
	t.Parallel()

	mc := &mockConnector{
		storeFn: func(_ imap.NumSet, _ *imap.StoreFlags, _ *imap.StoreOptions) error {
			return nil
		},
	}

	pool := newTestPool(t, mc)

	err := pool.SetFlag(context.Background(), testAccountID, "INBOX", 42, true)
	if err != nil {
		t.Fatalf("SetFlag() error = %v", err)
	}
}

func TestPool_SetFlag_Error(t *testing.T) {
	t.Parallel()

	mc := &mockConnector{
		storeFn: func(_ imap.NumSet, _ *imap.StoreFlags, _ *imap.StoreOptions) error {
			return errTestPool
		},
	}

	pool := newTestPool(t, mc)

	err := pool.SetFlag(context.Background(), testAccountID, "INBOX", 42, true)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestPool_SetFlag_UnknownAccount(t *testing.T) {
	t.Parallel()

	pool := newTestPool(t, &mockConnector{})

	err := pool.SetFlag(context.Background(), "bad", "INBOX", 42, true)
	if !errors.Is(err, models.ErrAccountNotFound) {
		t.Errorf("expected ErrAccountNotFound, got: %v", err)
	}
}

// ==================== Pool.CreateFolder ====================

func TestPool_CreateFolder_Success(t *testing.T) {
	t.Parallel()

	mc := &mockConnector{
		createFn: func(name string, _ *imap.CreateOptions) error {
			if name != "NewFolder" {
				t.Errorf("name = %q, want %q", name, "NewFolder")
			}

			return nil
		},
	}

	pool := newTestPool(t, mc)

	err := pool.CreateFolder(context.Background(), testAccountID, "NewFolder")
	if err != nil {
		t.Fatalf("CreateFolder() error = %v", err)
	}
}

func TestPool_CreateFolder_Error(t *testing.T) {
	t.Parallel()

	mc := &mockConnector{
		createFn: func(_ string, _ *imap.CreateOptions) error {
			return errTestPool
		},
	}

	pool := newTestPool(t, mc)

	err := pool.CreateFolder(context.Background(), testAccountID, "NewFolder")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestPool_CreateFolder_UnknownAccount(t *testing.T) {
	t.Parallel()

	pool := newTestPool(t, &mockConnector{})

	err := pool.CreateFolder(context.Background(), "bad", "NewFolder")
	if !errors.Is(err, models.ErrAccountNotFound) {
		t.Errorf("expected ErrAccountNotFound, got: %v", err)
	}
}

// ==================== Pool.SaveDraft ====================

func TestPool_SaveDraft_Success(t *testing.T) {
	t.Parallel()

	mc := &mockConnector{
		listFn:   standardListFn(),
		statusFn: standardStatusFn(),
		appendFn: func(_ string, _ []byte, _ *imap.AppendOptions) (*imap.AppendData, error) {
			return &imap.AppendData{UID: 100}, nil
		},
	}

	pool := newTestPool(t, mc)

	uid, err := pool.SaveDraft(context.Background(), testAccountID, []byte("draft"))
	if err != nil {
		t.Fatalf("SaveDraft() error = %v", err)
	}

	if uid != 100 {
		t.Errorf("uid = %d, want 100", uid)
	}
}

func TestPool_SaveDraft_Error(t *testing.T) {
	t.Parallel()

	mc := &mockConnector{
		listFn:   standardListFn(),
		statusFn: standardStatusFn(),
		appendFn: func(_ string, _ []byte, _ *imap.AppendOptions) (*imap.AppendData, error) {
			return nil, errTestPool
		},
	}

	pool := newTestPool(t, mc)

	_, err := pool.SaveDraft(context.Background(), testAccountID, []byte("draft"))
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestPool_SaveDraft_UnknownAccount(t *testing.T) {
	t.Parallel()

	pool := newTestPool(t, &mockConnector{})

	_, err := pool.SaveDraft(context.Background(), "bad", []byte("draft"))
	if !errors.Is(err, models.ErrAccountNotFound) {
		t.Errorf("expected ErrAccountNotFound, got: %v", err)
	}
}

// ==================== Pool.GetDraft ====================

func TestPool_GetDraft_Success(t *testing.T) {
	t.Parallel()

	mc := &mockConnector{
		listFn:   standardListFn(),
		statusFn: standardStatusFn(),
		fetchFn: func(_ imap.NumSet, _ *imap.FetchOptions) ([]*imapclient.FetchMessageBuffer, error) {
			return []*imapclient.FetchMessageBuffer{
				{
					UID: 50,
					BodySection: []imapclient.FetchBodySectionBuffer{
						{Bytes: []byte("draft body")},
					},
				},
			}, nil
		},
	}

	pool := newTestPool(t, mc)

	data, err := pool.GetDraft(context.Background(), testAccountID, 50)
	if err != nil {
		t.Fatalf("GetDraft() error = %v", err)
	}

	if string(data) != "draft body" {
		t.Errorf("data = %q, want %q", string(data), "draft body")
	}
}

func TestPool_GetDraft_Error(t *testing.T) {
	t.Parallel()

	mc := &mockConnector{
		listFn:   standardListFn(),
		statusFn: standardStatusFn(),
		fetchFn: func(_ imap.NumSet, _ *imap.FetchOptions) ([]*imapclient.FetchMessageBuffer, error) {
			return nil, errTestPool
		},
	}

	pool := newTestPool(t, mc)

	_, err := pool.GetDraft(context.Background(), testAccountID, 50)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestPool_GetDraft_UnknownAccount(t *testing.T) {
	t.Parallel()

	pool := newTestPool(t, &mockConnector{})

	_, err := pool.GetDraft(context.Background(), "bad", 50)
	if !errors.Is(err, models.ErrAccountNotFound) {
		t.Errorf("expected ErrAccountNotFound, got: %v", err)
	}
}

// ==================== Pool.DeleteDraft ====================

func TestPool_DeleteDraft_Success(t *testing.T) {
	t.Parallel()

	mc := &mockConnector{
		listFn:   standardListFn(),
		statusFn: standardStatusFn(),
	}

	pool := newTestPool(t, mc)

	err := pool.DeleteDraft(context.Background(), testAccountID, 42)
	if err != nil {
		t.Fatalf("DeleteDraft() error = %v", err)
	}
}

func TestPool_DeleteDraft_Error(t *testing.T) {
	t.Parallel()

	mc := &mockConnector{
		listFn:   standardListFn(),
		statusFn: standardStatusFn(),
		storeFn: func(_ imap.NumSet, _ *imap.StoreFlags, _ *imap.StoreOptions) error {
			return errTestPool
		},
	}

	pool := newTestPool(t, mc)

	err := pool.DeleteDraft(context.Background(), testAccountID, 42)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestPool_DeleteDraft_UnknownAccount(t *testing.T) {
	t.Parallel()

	pool := newTestPool(t, &mockConnector{})

	err := pool.DeleteDraft(context.Background(), "bad", 42)
	if !errors.Is(err, models.ErrAccountNotFound) {
		t.Errorf("expected ErrAccountNotFound, got: %v", err)
	}
}

// ==================== Pool.GetFolderByRole ====================

func TestPool_GetFolderByRole_Success(t *testing.T) {
	t.Parallel()

	mc := &mockConnector{
		listFn:   standardListFn(),
		statusFn: standardStatusFn(),
	}

	pool := newTestPool(t, mc)

	name, err := pool.GetFolderByRole(
		context.Background(), testAccountID, models.RoleDrafts,
	)
	if err != nil {
		t.Fatalf("GetFolderByRole() error = %v", err)
	}

	if name != "Drafts" {
		t.Errorf("name = %q, want %q", name, "Drafts")
	}
}

func TestPool_GetFolderByRole_Error(t *testing.T) {
	t.Parallel()

	mc := &mockConnector{
		listFn: func(_, _ string, _ *imap.ListOptions) ([]*imap.ListData, error) {
			return []*imap.ListData{{Mailbox: "INBOX"}}, nil
		},
		statusFn: standardStatusFn(),
	}

	pool := newTestPool(t, mc)

	_, err := pool.GetFolderByRole(
		context.Background(), testAccountID, models.RoleDrafts,
	)
	if err == nil {
		t.Fatal("expected error for missing role")
	}
}

func TestPool_GetFolderByRole_UnknownAccount(t *testing.T) {
	t.Parallel()

	pool := newTestPool(t, &mockConnector{})

	_, err := pool.GetFolderByRole(
		context.Background(), "bad", models.RoleDrafts,
	)
	if !errors.Is(err, models.ErrAccountNotFound) {
		t.Errorf("expected ErrAccountNotFound, got: %v", err)
	}
}

// ==================== Pool.Close ====================

func TestPool_Close_Graceful(t *testing.T) {
	t.Parallel()

	closeCalled := false

	mc := &mockConnector{
		closeFn: func() error {
			closeCalled = true

			return nil
		},
	}

	pool := newTestPool(t, mc)
	pool.Close(context.Background())

	if !closeCalled {
		t.Error("expected Close to be called on client connector")
	}

	// Verify pool is now closed.
	_, err := pool.Get(context.Background(), testAccountID)
	if !errors.Is(err, ErrPoolClosed) {
		t.Errorf("expected ErrPoolClosed after Close, got: %v", err)
	}
}

func TestPool_Close_Timeout(t *testing.T) {
	t.Parallel()

	pool := newTestPool(t, &mockConnector{})

	// Add an in-flight operation that blocks forever.
	pool.wg.Add(1)

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	// Close should return after timeout without hanging.
	pool.Close(ctx)

	// Cleanup: balance the wg.
	pool.wg.Done()
}
