package imap

import (
	"context"
	"errors"
	"log/slog"
	"testing"
	"time"

	"github.com/boutquin/mcp-server-email/internal/config"
	"github.com/boutquin/mcp-server-email/internal/models"
	"github.com/boutquin/mcp-server-email/internal/retry"
	"github.com/emersion/go-imap/v2"
	"github.com/emersion/go-imap/v2/imapclient"
	"github.com/emersion/go-sasl"
)

// --- Test Constants ---

const (
	testMailboxINBOX  = "INBOX"
	testMailboxDrafts = "Drafts"
	testContentPlain  = "text/plain"
	testContentHTML   = "text/html"
)

// --- Test Sentinel Errors ---

var (
	errTestAuth       = errors.New("authentication failed")
	errTestMailbox    = errors.New("mailbox not found")
	errTestSearch     = errors.New("search failed")
	errTestStore      = errors.New("store failed")
	errTestMove       = errors.New("move failed")
	errTestCopy       = errors.New("copy failed")
	errTestAppend     = errors.New("append failed")
	errTestCreate     = errors.New("folder already exists")
	errTestConnReset  = errors.New("connection reset by peer")
	errTestTempFail   = errors.New("temporary failure")
	errTestBrokenPipe = errors.New("broken pipe")
	errTestClose      = errors.New("close failed")
)

// --- Mock Connector ---

// mockConnector implements Connector for unit testing.
// Each method has a configurable function field. If nil, it returns zero values.
type mockConnector struct {
	loginFn        func(username, password string) error
	authenticateFn func(saslClient sasl.Client) error
	selectFn       func(mailbox string, opts *imap.SelectOptions) (*imap.SelectData, error)
	listFn         func(ref, pattern string, opts *imap.ListOptions) ([]*imap.ListData, error)
	statusFn       func(mailbox string, opts *imap.StatusOptions) (*imap.StatusData, error)
	fetchFn        func(seqSet imap.NumSet, opts *imap.FetchOptions) ([]*imapclient.FetchMessageBuffer, error)
	searchFn       func(criteria *imap.SearchCriteria, opts *imap.SearchOptions) (*imap.SearchData, error)
	moveFn         func(uidSet imap.UIDSet, dest string) error
	copyFn         func(uidSet imap.UIDSet, dest string) error
	storeFn        func(seqSet imap.NumSet, flags *imap.StoreFlags, opts *imap.StoreOptions) error
	expungeFn      func() error
	appendFn       func(mailbox string, literal []byte, opts *imap.AppendOptions) (*imap.AppendData, error)
	createFn       func(name string, opts *imap.CreateOptions) error
	closeFn        func() error
}

// Compile-time check.
var _ Connector = (*mockConnector)(nil)

func (m *mockConnector) Login(username, password string) error {
	if m.loginFn != nil {
		return m.loginFn(username, password)
	}

	return nil
}

func (m *mockConnector) Authenticate(saslClient sasl.Client) error {
	if m.authenticateFn != nil {
		return m.authenticateFn(saslClient)
	}

	return nil
}

func (m *mockConnector) Select(mailbox string, opts *imap.SelectOptions) (*imap.SelectData, error) {
	if m.selectFn != nil {
		return m.selectFn(mailbox, opts)
	}

	return &imap.SelectData{}, nil
}

func (m *mockConnector) List(ref, pattern string, opts *imap.ListOptions) ([]*imap.ListData, error) {
	if m.listFn != nil {
		return m.listFn(ref, pattern, opts)
	}

	return nil, nil
}

func (m *mockConnector) Status(mailbox string, opts *imap.StatusOptions) (*imap.StatusData, error) {
	if m.statusFn != nil {
		return m.statusFn(mailbox, opts)
	}

	return &imap.StatusData{}, nil
}

func (m *mockConnector) Fetch(
	seqSet imap.NumSet, opts *imap.FetchOptions,
) ([]*imapclient.FetchMessageBuffer, error) {
	if m.fetchFn != nil {
		return m.fetchFn(seqSet, opts)
	}

	return nil, nil
}

func (m *mockConnector) Search(
	criteria *imap.SearchCriteria, opts *imap.SearchOptions,
) (*imap.SearchData, error) {
	if m.searchFn != nil {
		return m.searchFn(criteria, opts)
	}

	return &imap.SearchData{}, nil
}

func (m *mockConnector) Move(uidSet imap.UIDSet, dest string) error {
	if m.moveFn != nil {
		return m.moveFn(uidSet, dest)
	}

	return nil
}

func (m *mockConnector) Copy(uidSet imap.UIDSet, dest string) error {
	if m.copyFn != nil {
		return m.copyFn(uidSet, dest)
	}

	return nil
}

func (m *mockConnector) Store(
	seqSet imap.NumSet, flags *imap.StoreFlags, opts *imap.StoreOptions,
) error {
	if m.storeFn != nil {
		return m.storeFn(seqSet, flags, opts)
	}

	return nil
}

func (m *mockConnector) Expunge() error {
	if m.expungeFn != nil {
		return m.expungeFn()
	}

	return nil
}

func (m *mockConnector) Append(
	mailbox string, literal []byte, opts *imap.AppendOptions,
) (*imap.AppendData, error) {
	if m.appendFn != nil {
		return m.appendFn(mailbox, literal, opts)
	}

	return &imap.AppendData{}, nil
}

func (m *mockConnector) Create(name string, opts *imap.CreateOptions) error {
	if m.createFn != nil {
		return m.createFn(name, opts)
	}

	return nil
}

func (m *mockConnector) Close() error {
	if m.closeFn != nil {
		return m.closeFn()
	}

	return nil
}

// --- Test Helpers ---

// seqSetFrom creates a SeqSet from a list of sequence numbers.
func seqSetFrom(nums ...uint32) imap.SeqSet {
	var ss imap.SeqSet

	for _, n := range nums {
		ss.AddNum(n)
	}

	return ss
}

// newMockClient creates a Client with a mock connector for testing.
// The client has rate limiting disabled (high token count) and a generous timeout.
func newMockClient(mock *mockConnector) *Client {
	return &Client{
		account: &config.Account{
			ID:    "test-account",
			Email: "test@example.com",
		},
		conn:    mock,
		timeout: 30 * time.Second,
		limiter: retry.NewLimiter(1000, time.Minute),
		retryCfg: retry.Config{
			MaxRetries: retry.DefaultMaxRetries,
			BaseDelay:  retry.DefaultBaseDelay,
			MaxDelay:   retry.DefaultMaxDelay,
		},
	}
}

// standardListFn returns a listFn that reports standard folder names so
// getFolderByRole's common-name fallback succeeds in tests.
func standardListFn() func(string, string, *imap.ListOptions) ([]*imap.ListData, error) {
	return func(_, _ string, _ *imap.ListOptions) ([]*imap.ListData, error) {
		return []*imap.ListData{
			{Mailbox: testMailboxINBOX},
			{Mailbox: testMailboxDrafts},
			{Mailbox: "Sent"},
			{Mailbox: "Trash"},
		}, nil
	}
}

// standardStatusFn returns a statusFn that reports zero counts (sufficient for role discovery).
func standardStatusFn() func(string, *imap.StatusOptions) (*imap.StatusData, error) {
	zero := uint32(0)

	return func(_ string, _ *imap.StatusOptions) (*imap.StatusData, error) {
		return &imap.StatusData{NumMessages: &zero, NumUnseen: &zero}, nil
	}
}

// --- Unit Tests ---

// ==================== ListFolders ====================

func TestClient_ListFolders_Happy(t *testing.T) {
	t.Parallel()

	inboxTotal := uint32(42)
	inboxUnseen := uint32(3)
	sentTotal := uint32(10)
	sentUnseen := uint32(0)

	mock := &mockConnector{
		listFn: func(_, _ string, _ *imap.ListOptions) ([]*imap.ListData, error) {
			return []*imap.ListData{
				{Mailbox: testMailboxINBOX},
				{Mailbox: "Sent"},
			}, nil
		},
		statusFn: func(mailbox string, opts *imap.StatusOptions) (*imap.StatusData, error) {
			// Verify both fields are requested
			if !opts.NumMessages || !opts.NumUnseen {
				t.Errorf("STATUS should request NumMessages+NumUnseen, got messages=%v unseen=%v",
					opts.NumMessages, opts.NumUnseen)
			}

			switch mailbox {
			case testMailboxINBOX:
				return &imap.StatusData{NumMessages: &inboxTotal, NumUnseen: &inboxUnseen}, nil
			case "Sent":
				return &imap.StatusData{NumMessages: &sentTotal, NumUnseen: &sentUnseen}, nil
			default:
				return &imap.StatusData{}, nil
			}
		},
	}

	client := newMockClient(mock)

	folders, err := client.ListFolders(context.Background())
	if err != nil {
		t.Fatalf("ListFolders() error = %v", err)
	}

	if len(folders) != 2 {
		t.Fatalf("expected 2 folders, got %d", len(folders))
	}

	if folders[0].Name != testMailboxINBOX || folders[0].Total != 42 || folders[0].Unread != 3 {
		t.Errorf("INBOX folder = %+v", folders[0])
	}

	if folders[1].Name != "Sent" || folders[1].Total != 10 || folders[1].Unread != 0 {
		t.Errorf("Sent folder = %+v", folders[1])
	}
}

func TestClient_ListFolders_ListError(t *testing.T) {
	t.Parallel()

	mock := &mockConnector{
		listFn: func(_, _ string, _ *imap.ListOptions) ([]*imap.ListData, error) {
			return nil, errTestAuth
		},
	}

	client := newMockClient(mock)

	_, err := client.ListFolders(context.Background())
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestClient_ListFolders_Empty(t *testing.T) {
	t.Parallel()

	mock := &mockConnector{
		listFn: func(_, _ string, _ *imap.ListOptions) ([]*imap.ListData, error) {
			return nil, nil
		},
	}

	client := newMockClient(mock)

	folders, err := client.ListFolders(context.Background())
	if err != nil {
		t.Fatalf("ListFolders() error = %v", err)
	}

	if len(folders) != 0 {
		t.Errorf("expected 0 folders, got %d", len(folders))
	}
}

func TestClient_ListFolders_PartialStatus(t *testing.T) {
	t.Parallel()

	// STATUS returns NumMessages but not NumUnseen — Unread should be 0
	total := uint32(7)

	mock := &mockConnector{
		listFn: func(_, _ string, _ *imap.ListOptions) ([]*imap.ListData, error) {
			return []*imap.ListData{{Mailbox: testMailboxINBOX}}, nil
		},
		statusFn: func(_ string, _ *imap.StatusOptions) (*imap.StatusData, error) {
			return &imap.StatusData{NumMessages: &total, NumUnseen: nil}, nil
		},
	}

	client := newMockClient(mock)

	folders, err := client.ListFolders(context.Background())
	if err != nil {
		t.Fatalf("ListFolders() error = %v", err)
	}

	if len(folders) != 1 {
		t.Fatalf("expected 1 folder, got %d", len(folders))
	}

	if folders[0].Total != 7 {
		t.Errorf("expected Total=7, got %d", folders[0].Total)
	}

	if folders[0].Unread != 0 {
		t.Errorf("expected Unread=0 for nil NumUnseen, got %d", folders[0].Unread)
	}
}

func TestClient_ListFolders_StatusError(t *testing.T) {
	t.Parallel()

	// STATUS errors for one folder → folder included with zeros, others unaffected
	total := uint32(5)
	unseen := uint32(2)

	mock := &mockConnector{
		listFn: func(_, _ string, _ *imap.ListOptions) ([]*imap.ListData, error) {
			return []*imap.ListData{
				{Mailbox: testMailboxINBOX},
				{Mailbox: "Broken"},
				{Mailbox: "Sent"},
			}, nil
		},
		statusFn: func(mailbox string, _ *imap.StatusOptions) (*imap.StatusData, error) {
			if mailbox == "Broken" {
				return nil, errTestMailbox
			}

			return &imap.StatusData{NumMessages: &total, NumUnseen: &unseen}, nil
		},
	}

	client := newMockClient(mock)

	folders, err := client.ListFolders(context.Background())
	if err != nil {
		t.Fatalf("ListFolders() error = %v", err)
	}

	if len(folders) != 3 {
		t.Fatalf("expected 3 folders, got %d", len(folders))
	}

	// INBOX should have correct counts
	if folders[0].Total != 5 || folders[0].Unread != 2 {
		t.Errorf("INBOX = %+v, expected Total=5 Unread=2", folders[0])
	}

	// Broken should have zero counts (not skipped)
	if folders[1].Name != "Broken" || folders[1].Total != 0 || folders[1].Unread != 0 {
		t.Errorf("Broken folder = %+v, expected zero counts", folders[1])
	}

	// Sent should have correct counts
	if folders[2].Total != 5 || folders[2].Unread != 2 {
		t.Errorf("Sent = %+v, expected Total=5 Unread=2", folders[2])
	}
}

func TestClient_ListFolders_NoSelect(t *testing.T) {
	t.Parallel()

	// Verify that ListFolders does NOT call Select
	total := uint32(1)

	mock := &mockConnector{
		listFn: func(_, _ string, _ *imap.ListOptions) ([]*imap.ListData, error) {
			return []*imap.ListData{{Mailbox: testMailboxINBOX}}, nil
		},
		selectFn: func(_ string, _ *imap.SelectOptions) (*imap.SelectData, error) {
			t.Fatal("Select should not be called by ListFolders")

			return nil, errTestMailbox // unreachable; t.Fatal stops execution
		},
		statusFn: func(_ string, _ *imap.StatusOptions) (*imap.StatusData, error) {
			return &imap.StatusData{NumMessages: &total}, nil
		},
	}

	client := newMockClient(mock)

	_, err := client.ListFolders(context.Background())
	if err != nil {
		t.Fatalf("ListFolders() error = %v", err)
	}
}

// ==================== GetMessage ====================

func TestClient_GetMessage_Happy(t *testing.T) {
	t.Parallel()

	mock := &mockConnector{
		fetchFn: func(_ imap.NumSet, _ *imap.FetchOptions) ([]*imapclient.FetchMessageBuffer, error) {
			return []*imapclient.FetchMessageBuffer{
				{
					UID:   42,
					Flags: []imap.Flag{imap.FlagSeen},
					Envelope: &imap.Envelope{
						Subject: "Test Subject",
						From:    []imap.Address{{Mailbox: "sender", Host: "example.com"}},
						Date:    time.Date(2026, 1, 15, 10, 0, 0, 0, time.UTC),
					},
					BodyStructure: &imap.BodyStructureSinglePart{
						Type:    "TEXT",
						Subtype: "PLAIN",
					},
				},
			}, nil
		},
	}

	client := newMockClient(mock)

	email, err := client.GetMessage(context.Background(), testMailboxINBOX, 42)
	if err != nil {
		t.Fatalf("GetMessage() error = %v", err)
	}

	if email.Subject != "Test Subject" {
		t.Errorf("Subject = %q, want %q", email.Subject, "Test Subject")
	}

	if email.From != "sender@example.com" {
		t.Errorf("From = %q", email.From)
	}

	if email.IsUnread {
		t.Error("expected message to be read (has \\Seen flag)")
	}

	if email.ContentType != testContentPlain {
		t.Errorf("ContentType = %q", email.ContentType)
	}
}

func TestClient_GetMessage_NotFound(t *testing.T) {
	t.Parallel()

	mock := &mockConnector{
		fetchFn: func(_ imap.NumSet, _ *imap.FetchOptions) ([]*imapclient.FetchMessageBuffer, error) {
			return nil, nil // Empty result
		},
	}

	client := newMockClient(mock)

	_, err := client.GetMessage(context.Background(), testMailboxINBOX, 999)
	if !errors.Is(err, models.ErrMessageNotFound) {
		t.Errorf("expected ErrMessageNotFound, got: %v", err)
	}
}

func TestClient_GetMessage_SelectError(t *testing.T) {
	t.Parallel()

	mock := &mockConnector{
		selectFn: func(_ string, _ *imap.SelectOptions) (*imap.SelectData, error) {
			return nil, errTestMailbox
		},
	}

	client := newMockClient(mock)

	_, err := client.GetMessage(context.Background(), testMailboxINBOX, 42)
	if err == nil {
		t.Fatal("expected error")
	}
}

// ==================== ListMessages ====================

func TestClient_ListMessages_Happy(t *testing.T) {
	t.Parallel()

	mock := &mockConnector{
		selectFn: func(_ string, _ *imap.SelectOptions) (*imap.SelectData, error) {
			return &imap.SelectData{NumMessages: 10}, nil
		},
		fetchFn: func(_ imap.NumSet, _ *imap.FetchOptions) ([]*imapclient.FetchMessageBuffer, error) {
			return []*imapclient.FetchMessageBuffer{
				{
					UID:      8,
					Flags:    []imap.Flag{imap.FlagSeen},
					Envelope: &imap.Envelope{Subject: "Message 8"},
				},
				{
					UID:      9,
					Flags:    []imap.Flag{},
					Envelope: &imap.Envelope{Subject: "Message 9"},
				},
			}, nil
		},
	}

	client := newMockClient(mock)

	emails, err := client.ListMessages(context.Background(), testMailboxINBOX, 5, 0, false)
	if err != nil {
		t.Fatalf("ListMessages() error = %v", err)
	}

	if len(emails) != 2 {
		t.Fatalf("expected 2 emails, got %d", len(emails))
	}
}

func TestClient_ListMessages_EmptyMailbox(t *testing.T) {
	t.Parallel()

	mock := &mockConnector{
		selectFn: func(_ string, _ *imap.SelectOptions) (*imap.SelectData, error) {
			return &imap.SelectData{NumMessages: 0}, nil
		},
	}

	client := newMockClient(mock)

	emails, err := client.ListMessages(context.Background(), testMailboxINBOX, 50, 0, false)
	if err != nil {
		t.Fatalf("ListMessages() error = %v", err)
	}

	if len(emails) != 0 {
		t.Errorf("expected 0 emails, got %d", len(emails))
	}
}

func TestClient_ListMessages_OffsetBeyondTotal(t *testing.T) {
	t.Parallel()

	mock := &mockConnector{
		selectFn: func(_ string, _ *imap.SelectOptions) (*imap.SelectData, error) {
			return &imap.SelectData{NumMessages: 5}, nil
		},
	}

	client := newMockClient(mock)

	emails, err := client.ListMessages(context.Background(), testMailboxINBOX, 10, 10, false)
	if err != nil {
		t.Fatalf("ListMessages() error = %v", err)
	}

	if len(emails) != 0 {
		t.Errorf("expected 0 emails for offset > total, got %d", len(emails))
	}
}

// ==================== Search ====================

func TestClient_Search_Happy(t *testing.T) {
	t.Parallel()

	mock := &mockConnector{
		searchFn: func(_ *imap.SearchCriteria, _ *imap.SearchOptions) (*imap.SearchData, error) {
			return &imap.SearchData{
				All: seqSetFrom(1, 2, 3),
			}, nil
		},
		fetchFn: func(_ imap.NumSet, _ *imap.FetchOptions) ([]*imapclient.FetchMessageBuffer, error) {
			return []*imapclient.FetchMessageBuffer{
				{UID: 1, Envelope: &imap.Envelope{Subject: "Match 1"}},
				{UID: 2, Envelope: &imap.Envelope{Subject: "Match 2"}},
				{UID: 3, Envelope: &imap.Envelope{Subject: "Match 3"}},
			}, nil
		},
	}

	client := newMockClient(mock)

	emails, err := client.Search(
		context.Background(), testMailboxINBOX, "test", "", "", "", "", 50, false,
	)
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}

	if len(emails) != 3 {
		t.Fatalf("expected 3 emails, got %d", len(emails))
	}
}

func TestClient_Search_NoResults(t *testing.T) {
	t.Parallel()

	mock := &mockConnector{
		searchFn: func(_ *imap.SearchCriteria, _ *imap.SearchOptions) (*imap.SearchData, error) {
			return &imap.SearchData{}, nil
		},
	}

	client := newMockClient(mock)

	emails, err := client.Search(
		context.Background(), testMailboxINBOX, "nonexistent", "", "", "", "", 50, false,
	)
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}

	if len(emails) != 0 {
		t.Errorf("expected 0 emails, got %d", len(emails))
	}
}

func TestClient_Search_Error(t *testing.T) {
	t.Parallel()

	mock := &mockConnector{
		searchFn: func(_ *imap.SearchCriteria, _ *imap.SearchOptions) (*imap.SearchData, error) {
			return nil, errTestSearch
		},
	}

	client := newMockClient(mock)

	_, err := client.Search(
		context.Background(), testMailboxINBOX, "test", "", "", "", "", 50, false,
	)
	if err == nil {
		t.Fatal("expected error")
	}
}

// ==================== ListUnread ====================

func TestClient_ListUnread_Happy(t *testing.T) {
	t.Parallel()

	mock := &mockConnector{
		searchFn: func(_ *imap.SearchCriteria, _ *imap.SearchOptions) (*imap.SearchData, error) {
			return &imap.SearchData{All: seqSetFrom(2, 5)}, nil
		},
		fetchFn: func(_ imap.NumSet, _ *imap.FetchOptions) ([]*imapclient.FetchMessageBuffer, error) {
			return []*imapclient.FetchMessageBuffer{
				{UID: 2, Envelope: &imap.Envelope{Subject: "Unread 1"}},
				{UID: 5, Envelope: &imap.Envelope{Subject: "Unread 2"}},
			}, nil
		},
	}

	client := newMockClient(mock)

	emails, err := client.ListUnread(context.Background(), testMailboxINBOX, 50, false)
	if err != nil {
		t.Fatalf("ListUnread() error = %v", err)
	}

	if len(emails) != 2 {
		t.Fatalf("expected 2 emails, got %d", len(emails))
	}
}

func TestClient_ListUnread_NoUnread(t *testing.T) {
	t.Parallel()

	mock := &mockConnector{
		searchFn: func(_ *imap.SearchCriteria, _ *imap.SearchOptions) (*imap.SearchData, error) {
			return &imap.SearchData{}, nil
		},
	}

	client := newMockClient(mock)

	emails, err := client.ListUnread(context.Background(), testMailboxINBOX, 50, false)
	if err != nil {
		t.Fatalf("ListUnread() error = %v", err)
	}

	if len(emails) != 0 {
		t.Errorf("expected 0 emails, got %d", len(emails))
	}
}

// ==================== DeleteMessage ====================

func TestClient_DeleteMessage_Permanent(t *testing.T) {
	t.Parallel()

	storeCalled := false
	expungeCalled := false

	mock := &mockConnector{
		storeFn: func(_ imap.NumSet, flags *imap.StoreFlags, _ *imap.StoreOptions) error {
			storeCalled = true

			if flags.Op != imap.StoreFlagsAdd {
				t.Errorf("expected StoreFlagsAdd, got %v", flags.Op)
			}

			return nil
		},
		expungeFn: func() error {
			expungeCalled = true

			return nil
		},
	}

	client := newMockClient(mock)

	err := client.DeleteMessage(context.Background(), testMailboxINBOX, 42, true)
	if err != nil {
		t.Fatalf("DeleteMessage() error = %v", err)
	}

	if !storeCalled {
		t.Error("Store was not called")
	}

	if !expungeCalled {
		t.Error("Expunge was not called")
	}
}

func TestClient_DeleteMessage_MoveToTrash(t *testing.T) {
	t.Parallel()

	moveCalled := false

	mock := &mockConnector{
		listFn:   standardListFn(),
		statusFn: standardStatusFn(),
		moveFn: func(_ imap.UIDSet, dest string) error {
			moveCalled = true

			if dest != "Trash" {
				t.Errorf("expected destination %q, got %q", "Trash", dest)
			}

			return nil
		},
	}

	client := newMockClient(mock)

	err := client.DeleteMessage(context.Background(), testMailboxINBOX, 42, false)
	if err != nil {
		t.Fatalf("DeleteMessage() error = %v", err)
	}

	if !moveCalled {
		t.Error("Move was not called")
	}
}

func TestClient_DeleteMessage_StoreError(t *testing.T) {
	t.Parallel()

	mock := &mockConnector{
		storeFn: func(_ imap.NumSet, _ *imap.StoreFlags, _ *imap.StoreOptions) error {
			return errTestStore
		},
	}

	client := newMockClient(mock)

	err := client.DeleteMessage(context.Background(), testMailboxINBOX, 42, true)
	if err == nil {
		t.Fatal("expected error")
	}
}

// ==================== MoveMessage ====================

func TestClient_MoveMessage_Happy(t *testing.T) {
	t.Parallel()

	mock := &mockConnector{
		moveFn: func(_ imap.UIDSet, dest string) error {
			if dest != "Archive" {
				t.Errorf("expected destination %q, got %q", "Archive", dest)
			}

			return nil
		},
	}

	client := newMockClient(mock)

	err := client.MoveMessage(context.Background(), testMailboxINBOX, 42, "Archive")
	if err != nil {
		t.Fatalf("MoveMessage() error = %v", err)
	}
}

func TestClient_MoveMessage_Error(t *testing.T) {
	t.Parallel()

	mock := &mockConnector{
		moveFn: func(_ imap.UIDSet, _ string) error {
			return errTestMove
		},
	}

	client := newMockClient(mock)

	err := client.MoveMessage(context.Background(), testMailboxINBOX, 42, "Archive")
	if err == nil {
		t.Fatal("expected error")
	}
}

// ==================== CopyMessage ====================

func TestClient_CopyMessage_Happy(t *testing.T) {
	t.Parallel()

	mock := &mockConnector{
		copyFn: func(_ imap.UIDSet, dest string) error {
			if dest != "Backup" {
				t.Errorf("expected destination %q, got %q", "Backup", dest)
			}

			return nil
		},
	}

	client := newMockClient(mock)

	err := client.CopyMessage(context.Background(), testMailboxINBOX, 42, "Backup")
	if err != nil {
		t.Fatalf("CopyMessage() error = %v", err)
	}
}

func TestClient_CopyMessage_Error(t *testing.T) {
	t.Parallel()

	mock := &mockConnector{
		copyFn: func(_ imap.UIDSet, _ string) error {
			return errTestCopy
		},
	}

	client := newMockClient(mock)

	err := client.CopyMessage(context.Background(), testMailboxINBOX, 42, "Backup")
	if err == nil {
		t.Fatal("expected error")
	}
}

// ==================== MarkRead ====================

func TestClient_MarkRead_Happy(t *testing.T) {
	t.Parallel()

	mock := &mockConnector{
		storeFn: func(_ imap.NumSet, flags *imap.StoreFlags, _ *imap.StoreOptions) error {
			if flags.Op != imap.StoreFlagsAdd {
				t.Errorf("expected StoreFlagsAdd for read=true, got %v", flags.Op)
			}

			if len(flags.Flags) != 1 || flags.Flags[0] != imap.FlagSeen {
				t.Errorf("expected [\\Seen], got %v", flags.Flags)
			}

			return nil
		},
	}

	client := newMockClient(mock)

	err := client.MarkRead(context.Background(), testMailboxINBOX, 42, true)
	if err != nil {
		t.Fatalf("MarkRead() error = %v", err)
	}
}

func TestClient_MarkRead_Unread(t *testing.T) {
	t.Parallel()

	mock := &mockConnector{
		storeFn: func(_ imap.NumSet, flags *imap.StoreFlags, _ *imap.StoreOptions) error {
			if flags.Op != imap.StoreFlagsDel {
				t.Errorf("expected StoreFlagsDel for read=false, got %v", flags.Op)
			}

			return nil
		},
	}

	client := newMockClient(mock)

	err := client.MarkRead(context.Background(), testMailboxINBOX, 42, false)
	if err != nil {
		t.Fatalf("MarkRead() error = %v", err)
	}
}

func TestClient_MarkRead_Error(t *testing.T) {
	t.Parallel()

	mock := &mockConnector{
		storeFn: func(_ imap.NumSet, _ *imap.StoreFlags, _ *imap.StoreOptions) error {
			return errTestStore
		},
	}

	client := newMockClient(mock)

	err := client.MarkRead(context.Background(), testMailboxINBOX, 42, true)
	if err == nil {
		t.Fatal("expected error")
	}
}

// ==================== SetFlag ====================

func TestClient_SetFlag_Happy(t *testing.T) {
	t.Parallel()

	mock := &mockConnector{
		storeFn: func(_ imap.NumSet, flags *imap.StoreFlags, _ *imap.StoreOptions) error {
			if len(flags.Flags) != 1 || flags.Flags[0] != imap.FlagFlagged {
				t.Errorf("expected [\\Flagged], got %v", flags.Flags)
			}

			return nil
		},
	}

	client := newMockClient(mock)

	err := client.SetFlag(context.Background(), testMailboxINBOX, 42, true)
	if err != nil {
		t.Fatalf("SetFlag() error = %v", err)
	}
}

func TestClient_SetFlag_Unflag(t *testing.T) {
	t.Parallel()

	mock := &mockConnector{
		storeFn: func(_ imap.NumSet, flags *imap.StoreFlags, _ *imap.StoreOptions) error {
			if flags.Op != imap.StoreFlagsDel {
				t.Errorf("expected StoreFlagsDel for unflag, got %v", flags.Op)
			}

			return nil
		},
	}

	client := newMockClient(mock)

	err := client.SetFlag(context.Background(), testMailboxINBOX, 42, false)
	if err != nil {
		t.Fatalf("SetFlag() error = %v", err)
	}
}

// ==================== getFolderByRole ====================

func TestClient_GetFolderByRole_RFC6154(t *testing.T) {
	t.Parallel()

	// Server returns RFC 6154 special-use attributes → role resolved via attributes.
	mock := &mockConnector{
		listFn: func(_, _ string, _ *imap.ListOptions) ([]*imap.ListData, error) {
			return []*imap.ListData{
				{Mailbox: testMailboxINBOX},
				{Mailbox: "MyDrafts", Attrs: []imap.MailboxAttr{imap.MailboxAttrDrafts}},
				{Mailbox: "Rubbish", Attrs: []imap.MailboxAttr{imap.MailboxAttrTrash}},
				{Mailbox: "Sent Messages", Attrs: []imap.MailboxAttr{imap.MailboxAttrSent}},
			}, nil
		},
		statusFn: standardStatusFn(),
	}

	client := newMockClient(mock)

	name, err := client.getFolderByRole(context.Background(), models.RoleDrafts)
	if err != nil {
		t.Fatalf("getFolderByRole(Drafts) error = %v", err)
	}

	if name != "MyDrafts" {
		t.Errorf("expected %q, got %q", "MyDrafts", name)
	}

	// Trash should also be resolved from attributes.
	name, err = client.getFolderByRole(context.Background(), models.RoleTrash)
	if err != nil {
		t.Fatalf("getFolderByRole(Trash) error = %v", err)
	}

	if name != "Rubbish" {
		t.Errorf("expected %q, got %q", "Rubbish", name)
	}
}

func TestClient_GetFolderByRole_Gmail(t *testing.T) {
	t.Parallel()

	// Gmail uses [Gmail]/Drafts, [Gmail]/Trash etc. with RFC 6154 attributes.
	mock := &mockConnector{
		listFn: func(_, _ string, _ *imap.ListOptions) ([]*imap.ListData, error) {
			return []*imap.ListData{
				{Mailbox: testMailboxINBOX},
				{Mailbox: "[Gmail]/Drafts", Attrs: []imap.MailboxAttr{imap.MailboxAttrDrafts}},
				{Mailbox: "[Gmail]/Trash", Attrs: []imap.MailboxAttr{imap.MailboxAttrTrash}},
				{Mailbox: "[Gmail]/Sent Mail", Attrs: []imap.MailboxAttr{imap.MailboxAttrSent}},
				{Mailbox: "[Gmail]/Spam", Attrs: []imap.MailboxAttr{imap.MailboxAttrJunk}},
				{Mailbox: "[Gmail]/All Mail", Attrs: []imap.MailboxAttr{imap.MailboxAttrArchive}},
			}, nil
		},
		statusFn: standardStatusFn(),
	}

	client := newMockClient(mock)

	tests := []struct {
		role models.FolderRole
		want string
	}{
		{models.RoleDrafts, "[Gmail]/Drafts"},
		{models.RoleTrash, "[Gmail]/Trash"},
		{models.RoleSent, "[Gmail]/Sent Mail"},
		{models.RoleJunk, "[Gmail]/Spam"},
		{models.RoleArchive, "[Gmail]/All Mail"},
	}

	for _, tt := range tests {
		name, err := client.getFolderByRole(context.Background(), tt.role)
		if err != nil {
			t.Fatalf("getFolderByRole(%s) error = %v", tt.role, err)
		}

		if name != tt.want {
			t.Errorf("getFolderByRole(%s) = %q, want %q", tt.role, name, tt.want)
		}
	}
}

func TestClient_GetFolderByRole_StandardNames(t *testing.T) {
	t.Parallel()

	// Server has NO RFC 6154 attributes → falls back to common folder names.
	mock := &mockConnector{
		listFn: func(_, _ string, _ *imap.ListOptions) ([]*imap.ListData, error) {
			return []*imap.ListData{
				{Mailbox: testMailboxINBOX},
				{Mailbox: testMailboxDrafts}, // "Drafts" matches common name
				{Mailbox: "Trash"},           // "Trash" matches common name
				{Mailbox: "Sent"},
			}, nil
		},
		statusFn: standardStatusFn(),
	}

	client := newMockClient(mock)

	name, err := client.getFolderByRole(context.Background(), models.RoleDrafts)
	if err != nil {
		t.Fatalf("getFolderByRole(Drafts) error = %v", err)
	}

	if name != testMailboxDrafts {
		t.Errorf("expected %q, got %q", testMailboxDrafts, name)
	}

	name, err = client.getFolderByRole(context.Background(), models.RoleTrash)
	if err != nil {
		t.Fatalf("getFolderByRole(Trash) error = %v", err)
	}

	if name != "Trash" {
		t.Errorf("expected %q, got %q", "Trash", name)
	}
}

func TestClient_GetFolderByRole_NoMatch(t *testing.T) {
	t.Parallel()

	// Server has no Drafts folder (no attribute, no common name) → error.
	mock := &mockConnector{
		listFn: func(_, _ string, _ *imap.ListOptions) ([]*imap.ListData, error) {
			return []*imap.ListData{
				{Mailbox: testMailboxINBOX},
				{Mailbox: "CustomFolder"},
			}, nil
		},
		statusFn: standardStatusFn(),
	}

	client := newMockClient(mock)

	_, err := client.getFolderByRole(context.Background(), models.RoleDrafts)
	if !errors.Is(err, models.ErrFolderRoleNotFound) {
		t.Errorf("expected ErrFolderRoleNotFound, got: %v", err)
	}
}

func TestClient_GetFolderByRole_Caching(t *testing.T) {
	t.Parallel()

	// Verify that the second call uses the cache and doesn't call ListFolders again.
	listCallCount := 0

	mock := &mockConnector{
		listFn: func(_, _ string, _ *imap.ListOptions) ([]*imap.ListData, error) {
			listCallCount++

			return []*imap.ListData{
				{Mailbox: testMailboxINBOX},
				{Mailbox: testMailboxDrafts},
				{Mailbox: "Trash"},
			}, nil
		},
		statusFn: standardStatusFn(),
	}

	client := newMockClient(mock)

	// First call: discovers and caches.
	name1, err := client.getFolderByRole(context.Background(), models.RoleDrafts)
	if err != nil {
		t.Fatalf("first call error = %v", err)
	}

	if listCallCount != 1 {
		t.Fatalf("expected 1 list call, got %d", listCallCount)
	}

	// Second call: should use cache (no additional List call).
	name2, err := client.getFolderByRole(context.Background(), models.RoleDrafts)
	if err != nil {
		t.Fatalf("second call error = %v", err)
	}

	if listCallCount != 1 {
		t.Errorf("expected still 1 list call (cached), got %d", listCallCount)
	}

	if name1 != name2 {
		t.Errorf("cached result mismatch: %q vs %q", name1, name2)
	}
}

func TestClient_GetFolderByRole_ListFoldersPopulatesRoles(t *testing.T) {
	t.Parallel()

	// ListFolders populates Role field on Folder structs from RFC 6154 attrs.
	mock := &mockConnector{
		listFn: func(_, _ string, _ *imap.ListOptions) ([]*imap.ListData, error) {
			return []*imap.ListData{
				{Mailbox: testMailboxINBOX},
				{Mailbox: "MyDrafts", Attrs: []imap.MailboxAttr{imap.MailboxAttrDrafts}},
				{Mailbox: "Junk", Attrs: []imap.MailboxAttr{imap.MailboxAttrJunk}},
			}, nil
		},
		statusFn: standardStatusFn(),
	}

	client := newMockClient(mock)

	folders, err := client.ListFolders(context.Background())
	if err != nil {
		t.Fatalf("ListFolders() error = %v", err)
	}

	if len(folders) != 3 {
		t.Fatalf("expected 3 folders, got %d", len(folders))
	}

	// INBOX has no role.
	if folders[0].Role != "" {
		t.Errorf("INBOX role = %q, want empty", folders[0].Role)
	}

	// MyDrafts has \Drafts attribute.
	if folders[1].Role != models.RoleDrafts {
		t.Errorf("MyDrafts role = %q, want %q", folders[1].Role, models.RoleDrafts)
	}

	// Junk has \Junk attribute.
	if folders[2].Role != models.RoleJunk {
		t.Errorf("Junk role = %q, want %q", folders[2].Role, models.RoleJunk)
	}
}

// ==================== CreateFolder ====================

func TestClient_CreateFolder_Happy(t *testing.T) {
	t.Parallel()

	mock := &mockConnector{
		createFn: func(name string, _ *imap.CreateOptions) error {
			if name != "NewFolder" {
				t.Errorf("expected folder name %q, got %q", "NewFolder", name)
			}

			return nil
		},
	}

	client := newMockClient(mock)

	err := client.CreateFolder(context.Background(), "NewFolder")
	if err != nil {
		t.Fatalf("CreateFolder() error = %v", err)
	}
}

func TestClient_CreateFolder_Error(t *testing.T) {
	t.Parallel()

	mock := &mockConnector{
		createFn: func(_ string, _ *imap.CreateOptions) error {
			return errTestCreate
		},
	}

	client := newMockClient(mock)

	err := client.CreateFolder(context.Background(), "ExistingFolder")
	if err == nil {
		t.Fatal("expected error")
	}
}

// ==================== SaveDraft ====================

func TestClient_SaveDraft_Happy(t *testing.T) {
	t.Parallel()

	mock := &mockConnector{
		listFn:   standardListFn(),
		statusFn: standardStatusFn(),
		appendFn: func(mailbox string, literal []byte, _ *imap.AppendOptions) (*imap.AppendData, error) {
			if mailbox != testMailboxDrafts {
				t.Errorf("expected mailbox %q, got %q", testMailboxDrafts, mailbox)
			}

			if string(literal) != "draft content" {
				t.Errorf("expected literal %q, got %q", "draft content", string(literal))
			}

			return &imap.AppendData{UID: 100}, nil
		},
	}

	client := newMockClient(mock)

	uid, err := client.SaveDraft(context.Background(), []byte("draft content"))
	if err != nil {
		t.Fatalf("SaveDraft() error = %v", err)
	}

	if uid != 100 {
		t.Errorf("expected UID 100, got %d", uid)
	}
}

func TestClient_SaveDraft_NilConn(t *testing.T) {
	t.Parallel()

	// With nil conn, getFolderByRole → ListFolders fails before SaveDraft's inner logic.
	// The exact error path depends on retry/reconnect behavior, but an error must occur.
	client := newMockClient(&mockConnector{})
	client.conn = nil

	_, err := client.SaveDraft(context.Background(), []byte("data"))
	if err == nil {
		t.Error("expected error with nil connection")
	}
}

func TestClient_SaveDraft_Error(t *testing.T) {
	t.Parallel()

	mock := &mockConnector{
		listFn:   standardListFn(),
		statusFn: standardStatusFn(),
		appendFn: func(_ string, _ []byte, _ *imap.AppendOptions) (*imap.AppendData, error) {
			return nil, errTestAppend
		},
	}

	client := newMockClient(mock)

	_, err := client.SaveDraft(context.Background(), []byte("data"))
	if err == nil {
		t.Fatal("expected error")
	}
}

// ==================== GetDraft ====================

func TestClient_GetDraft_Happy(t *testing.T) {
	t.Parallel()

	mock := &mockConnector{
		listFn:   standardListFn(),
		statusFn: standardStatusFn(),
		selectFn: func(mailbox string, _ *imap.SelectOptions) (*imap.SelectData, error) {
			if mailbox != testMailboxDrafts {
				t.Errorf("expected Drafts, got %q", mailbox)
			}

			return &imap.SelectData{}, nil
		},
		fetchFn: func(_ imap.NumSet, _ *imap.FetchOptions) ([]*imapclient.FetchMessageBuffer, error) {
			return []*imapclient.FetchMessageBuffer{
				{
					UID: 50,
					BodySection: []imapclient.FetchBodySectionBuffer{
						{Bytes: []byte("draft body content")},
					},
				},
			}, nil
		},
	}

	client := newMockClient(mock)

	draft, err := client.GetDraft(context.Background(), 50)
	if err != nil {
		t.Fatalf("GetDraft() error = %v", err)
	}

	if string(draft) != "draft body content" {
		t.Errorf("draft = %q, want %q", string(draft), "draft body content")
	}
}

func TestClient_GetDraft_NotFound(t *testing.T) {
	t.Parallel()

	mock := &mockConnector{
		listFn:   standardListFn(),
		statusFn: standardStatusFn(),
		fetchFn: func(_ imap.NumSet, _ *imap.FetchOptions) ([]*imapclient.FetchMessageBuffer, error) {
			return nil, nil
		},
	}

	client := newMockClient(mock)

	_, err := client.GetDraft(context.Background(), 999)
	if !errors.Is(err, models.ErrMessageNotFound) {
		t.Errorf("expected ErrMessageNotFound, got: %v", err)
	}
}

// ==================== DeleteDraft ====================

func TestClient_DeleteDraft_Happy(t *testing.T) {
	t.Parallel()

	selectMailbox := ""

	mock := &mockConnector{
		listFn:   standardListFn(),
		statusFn: standardStatusFn(),
		selectFn: func(mailbox string, _ *imap.SelectOptions) (*imap.SelectData, error) {
			selectMailbox = mailbox

			return &imap.SelectData{}, nil
		},
	}

	client := newMockClient(mock)

	err := client.DeleteDraft(context.Background(), 42)
	if err != nil {
		t.Fatalf("DeleteDraft() error = %v", err)
	}

	if selectMailbox != testMailboxDrafts {
		t.Errorf("expected Drafts mailbox, got %q", selectMailbox)
	}
}

// ==================== Retry on transient error ====================

func TestClient_MoveMessage_RetryOnTransient(t *testing.T) {
	t.Parallel()

	callCount := 0

	mock := &mockConnector{
		moveFn: func(_ imap.UIDSet, _ string) error {
			callCount++

			if callCount == 1 {
				return errTestConnReset
			}

			return nil
		},
	}

	client := newMockClient(mock)

	err := client.MoveMessage(context.Background(), testMailboxINBOX, 42, "Archive")
	if err != nil {
		t.Fatalf("MoveMessage() error = %v (after retry)", err)
	}

	if callCount < 2 {
		t.Errorf("expected at least 2 calls (retry), got %d", callCount)
	}
}

func TestClient_ListFolders_RetryOnTransient(t *testing.T) {
	t.Parallel()

	listCallCount := 0
	total := uint32(5)

	mock := &mockConnector{
		listFn: func(_, _ string, _ *imap.ListOptions) ([]*imap.ListData, error) {
			listCallCount++

			if listCallCount == 1 {
				return nil, errTestTempFail
			}

			return []*imap.ListData{{Mailbox: testMailboxINBOX}}, nil
		},
		statusFn: func(_ string, _ *imap.StatusOptions) (*imap.StatusData, error) {
			return &imap.StatusData{NumMessages: &total}, nil
		},
	}

	client := newMockClient(mock)

	folders, err := client.ListFolders(context.Background())
	if err != nil {
		t.Fatalf("ListFolders() error = %v (after retry)", err)
	}

	if len(folders) != 1 {
		t.Errorf("expected 1 folder after retry, got %d", len(folders))
	}

	if listCallCount < 2 {
		t.Errorf("expected at least 2 list calls, got %d", listCallCount)
	}
}

func TestClient_Search_RetryOnTransient(t *testing.T) {
	t.Parallel()

	selectCallCount := 0

	mock := &mockConnector{
		selectFn: func(_ string, _ *imap.SelectOptions) (*imap.SelectData, error) {
			selectCallCount++

			if selectCallCount == 1 {
				return nil, errTestBrokenPipe
			}

			return &imap.SelectData{}, nil
		},
		searchFn: func(_ *imap.SearchCriteria, _ *imap.SearchOptions) (*imap.SearchData, error) {
			return &imap.SearchData{All: seqSetFrom(1)}, nil
		},
		fetchFn: func(_ imap.NumSet, _ *imap.FetchOptions) ([]*imapclient.FetchMessageBuffer, error) {
			return []*imapclient.FetchMessageBuffer{
				{UID: 1, Envelope: &imap.Envelope{Subject: "Found"}},
			}, nil
		},
	}

	client := newMockClient(mock)

	emails, err := client.Search(
		context.Background(), testMailboxINBOX, "test", "", "", "", "", 50, false,
	)
	if err != nil {
		t.Fatalf("Search() error = %v (after retry)", err)
	}

	if len(emails) != 1 {
		t.Errorf("expected 1 email, got %d", len(emails))
	}
}

// ==================== parseMessageBuffer ====================

func TestClient_ParseMessageBuffer_WithBody(t *testing.T) {
	t.Parallel()

	client := newMockClient(&mockConnector{})

	buf := &imapclient.FetchMessageBuffer{
		UID:   42,
		Flags: []imap.Flag{imap.FlagFlagged},
		Envelope: &imap.Envelope{
			Subject: "Test",
			From:    []imap.Address{{Name: "Alice", Mailbox: "alice", Host: "example.com"}},
			To:      []imap.Address{{Mailbox: "bob", Host: "example.com"}},
			Date:    time.Date(2026, 1, 15, 10, 0, 0, 0, time.UTC),
		},
		BodyStructure: &imap.BodyStructureSinglePart{
			Type:    "TEXT",
			Subtype: "HTML",
		},
		BodySection: []imapclient.FetchBodySectionBuffer{
			{
				Section: &imap.FetchItemBodySection{
					Specifier: imap.PartSpecifierText,
				},
				Bytes: []byte("<h1>Hello</h1>"),
			},
			{
				Section: &imap.FetchItemBodySection{
					Specifier:    imap.PartSpecifierHeader,
					HeaderFields: []string{"References"},
					Peek:         true,
				},
				Bytes: []byte("References: <ref1@example.com>\r\n"),
			},
		},
	}

	email := client.parseMessageBuffer(buf, testMailboxINBOX, true)

	if email.Subject != "Test" {
		t.Errorf("Subject = %q", email.Subject)
	}

	if email.From != "Alice <alice@example.com>" {
		t.Errorf("From = %q", email.From)
	}

	if email.IsFlagged != true {
		t.Error("expected IsFlagged = true")
	}

	if email.IsUnread != true {
		t.Error("expected IsUnread = true (no \\Seen)")
	}

	if email.Body != "<h1>Hello</h1>" {
		t.Errorf("Body = %q", email.Body)
	}

	if email.ContentType != testContentHTML {
		t.Errorf("ContentType = %q", email.ContentType)
	}

	if len(email.References) != 1 || email.References[0] != "<ref1@example.com>" {
		t.Errorf("References = %v", email.References)
	}
}

func TestClient_ParseMessageBuffer_NoBody(t *testing.T) {
	t.Parallel()

	client := newMockClient(&mockConnector{})

	buf := &imapclient.FetchMessageBuffer{
		UID:      10,
		Flags:    []imap.Flag{imap.FlagSeen},
		Envelope: &imap.Envelope{Subject: "No body"},
		BodySection: []imapclient.FetchBodySectionBuffer{
			{
				Section: &imap.FetchItemBodySection{
					Specifier: imap.PartSpecifierText,
				},
				Bytes: []byte("body text here"),
			},
		},
	}

	email := client.parseMessageBuffer(buf, testMailboxINBOX, false)

	if email.Body != "" {
		t.Errorf("expected empty body when includeBody=false, got %q", email.Body)
	}

	if email.IsUnread {
		t.Error("expected read message (has \\Seen)")
	}
}

// ==================== Close ====================

func TestClient_Close_Happy(t *testing.T) {
	t.Parallel()

	closeCalled := false

	mock := &mockConnector{
		closeFn: func() error {
			closeCalled = true

			return nil
		},
	}

	client := newMockClient(mock)

	err := client.Close()
	if err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	if !closeCalled {
		t.Error("Close was not called on connector")
	}
}

func TestClient_Close_Error(t *testing.T) {
	t.Parallel()

	mock := &mockConnector{
		closeFn: func() error {
			return errTestClose
		},
	}

	client := newMockClient(mock)

	err := client.Close()
	if err == nil {
		t.Fatal("expected error")
	}
}

// ==================== SearchByMessageID ====================

func TestClient_SearchByMessageID_Success(t *testing.T) {
	t.Parallel()

	mc := &mockConnector{
		searchFn: func(_ *imap.SearchCriteria, _ *imap.SearchOptions) (*imap.SearchData, error) {
			return &imap.SearchData{All: seqSetFrom(1, 3)}, nil
		},
		fetchFn: func(_ imap.NumSet, _ *imap.FetchOptions) ([]*imapclient.FetchMessageBuffer, error) {
			return []*imapclient.FetchMessageBuffer{
				{
					UID:      1,
					Envelope: &imap.Envelope{Subject: "Original", Date: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)},
				},
				{
					UID:      3,
					Envelope: &imap.Envelope{Subject: "Reply", Date: time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)},
				},
			}, nil
		},
	}

	client := newMockClient(mc)

	emails, err := client.SearchByMessageID(context.Background(), testMailboxINBOX, "<abc@example.com>")
	if err != nil {
		t.Fatalf("SearchByMessageID() error = %v", err)
	}

	if len(emails) != 2 {
		t.Fatalf("expected 2 emails, got %d", len(emails))
	}
}

func TestClient_SearchByMessageID_NoResults(t *testing.T) {
	t.Parallel()

	mc := &mockConnector{
		searchFn: func(_ *imap.SearchCriteria, _ *imap.SearchOptions) (*imap.SearchData, error) {
			return &imap.SearchData{}, nil
		},
	}

	client := newMockClient(mc)

	emails, err := client.SearchByMessageID(context.Background(), testMailboxINBOX, "<none@example.com>")
	if err != nil {
		t.Fatalf("SearchByMessageID() error = %v", err)
	}

	if len(emails) != 0 {
		t.Errorf("expected 0 emails, got %d", len(emails))
	}
}

func TestClient_SearchByMessageID_SearchError(t *testing.T) {
	t.Parallel()

	mc := &mockConnector{
		searchFn: func(_ *imap.SearchCriteria, _ *imap.SearchOptions) (*imap.SearchData, error) {
			return nil, errTestSearch
		},
	}

	client := newMockClient(mc)

	_, err := client.SearchByMessageID(context.Background(), testMailboxINBOX, "<abc@example.com>")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestClient_SearchByMessageID_SelectError(t *testing.T) {
	t.Parallel()

	mc := &mockConnector{
		selectFn: func(_ string, _ *imap.SelectOptions) (*imap.SelectData, error) {
			return nil, errTestMailbox
		},
	}

	client := newMockClient(mc)

	_, err := client.SearchByMessageID(context.Background(), testMailboxINBOX, "<abc@example.com>")
	if err == nil {
		t.Fatal("expected error")
	}
}

// ==================== GetAttachments ====================

func TestClient_GetAttachments_HasAttachments(t *testing.T) {
	t.Parallel()

	mc := &mockConnector{
		fetchFn: func(_ imap.NumSet, _ *imap.FetchOptions) ([]*imapclient.FetchMessageBuffer, error) {
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
										Params: map[string]string{"filename": "report.pdf"},
									},
								},
							},
						},
					},
				},
			}, nil
		},
	}

	client := newMockClient(mc)

	atts, err := client.GetAttachments(context.Background(), testMailboxINBOX, 42)
	if err != nil {
		t.Fatalf("GetAttachments() error = %v", err)
	}

	if len(atts) != 1 {
		t.Fatalf("expected 1 attachment, got %d", len(atts))
	}

	if atts[0].Filename != "report.pdf" {
		t.Errorf("filename = %q, want %q", atts[0].Filename, "report.pdf")
	}
}

func TestClient_GetAttachments_NoAttachments(t *testing.T) {
	t.Parallel()

	mc := &mockConnector{
		fetchFn: func(_ imap.NumSet, _ *imap.FetchOptions) ([]*imapclient.FetchMessageBuffer, error) {
			return []*imapclient.FetchMessageBuffer{
				{
					UID: 42,
					BodyStructure: &imap.BodyStructureSinglePart{
						Type:    "TEXT",
						Subtype: "PLAIN",
					},
				},
			}, nil
		},
	}

	client := newMockClient(mc)

	atts, err := client.GetAttachments(context.Background(), testMailboxINBOX, 42)
	if err != nil {
		t.Fatalf("GetAttachments() error = %v", err)
	}

	if len(atts) != 0 {
		t.Errorf("expected 0 attachments, got %d", len(atts))
	}
}

func TestClient_GetAttachments_FetchError(t *testing.T) {
	t.Parallel()

	mc := &mockConnector{
		fetchFn: func(_ imap.NumSet, _ *imap.FetchOptions) ([]*imapclient.FetchMessageBuffer, error) {
			return nil, errTestSearch
		},
	}

	client := newMockClient(mc)

	_, err := client.GetAttachments(context.Background(), testMailboxINBOX, 42)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestClient_GetAttachments_NotFound(t *testing.T) {
	t.Parallel()

	mc := &mockConnector{
		fetchFn: func(_ imap.NumSet, _ *imap.FetchOptions) ([]*imapclient.FetchMessageBuffer, error) {
			return nil, nil
		},
	}

	client := newMockClient(mc)

	_, err := client.GetAttachments(context.Background(), testMailboxINBOX, 999)
	if !errors.Is(err, models.ErrMessageNotFound) {
		t.Errorf("expected ErrMessageNotFound, got: %v", err)
	}
}

// ==================== GetAttachment ====================

func TestClient_GetAttachment_IndexOutOfBounds(t *testing.T) {
	t.Parallel()

	mc := &mockConnector{
		fetchFn: func(_ imap.NumSet, _ *imap.FetchOptions) ([]*imapclient.FetchMessageBuffer, error) {
			return []*imapclient.FetchMessageBuffer{
				{
					UID: 42,
					BodyStructure: &imap.BodyStructureSinglePart{
						Type:    "TEXT",
						Subtype: "PLAIN",
					},
				},
			}, nil
		},
	}

	client := newMockClient(mc)

	_, _, err := client.GetAttachment(context.Background(), testMailboxINBOX, 42, 5)
	if err == nil {
		t.Fatal("expected error for out-of-bounds index")
	}
}

func TestClient_GetAttachment_NegativeIndex(t *testing.T) {
	t.Parallel()

	mc := &mockConnector{
		fetchFn: func(_ imap.NumSet, _ *imap.FetchOptions) ([]*imapclient.FetchMessageBuffer, error) {
			return []*imapclient.FetchMessageBuffer{
				{
					UID: 42,
					BodyStructure: &imap.BodyStructureSinglePart{
						Type:    "TEXT",
						Subtype: "PLAIN",
					},
				},
			}, nil
		},
	}

	client := newMockClient(mc)

	_, _, err := client.GetAttachment(context.Background(), testMailboxINBOX, 42, -1)
	if err == nil {
		t.Fatal("expected error for negative index")
	}
}

func TestClient_GetAttachment_FetchBodyError(t *testing.T) {
	t.Parallel()

	callCount := 0

	mc := &mockConnector{
		fetchFn: func(_ imap.NumSet, _ *imap.FetchOptions) ([]*imapclient.FetchMessageBuffer, error) {
			callCount++

			// First call: GetAttachments succeeds.
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

			// Second call: fetchAttachmentBody fails.
			return nil, errTestSearch
		},
	}

	client := newMockClient(mc)

	_, _, err := client.GetAttachment(context.Background(), testMailboxINBOX, 42, 0)
	if err == nil {
		t.Fatal("expected error from fetch body")
	}
}

// ==================== log() ====================

func TestClient_Log_NilLogger(t *testing.T) {
	t.Parallel()

	client := newMockClient(&mockConnector{})
	client.logger = nil

	logger := client.log()
	if logger == nil {
		t.Fatal("expected non-nil logger")
	}

	if logger != nopLogger {
		t.Error("expected nopLogger fallback")
	}
}

func TestClient_Log_WithLogger(t *testing.T) {
	t.Parallel()

	custom := slog.New(slog.NewTextHandler(&nopWriter{}, nil))

	client := newMockClient(&mockConnector{})
	client.logger = custom

	logger := client.log()
	if logger != custom {
		t.Error("expected custom logger to be returned")
	}
}

// ==================== nopWriter ====================

func TestNopWriter_Write(t *testing.T) {
	t.Parallel()

	w := nopWriter{}

	n, err := w.Write([]byte("test data"))
	if err != nil {
		t.Fatalf("Write() error = %v", err)
	}

	if n != 9 {
		t.Errorf("expected 9 bytes written, got %d", n)
	}
}

// ==================== fetchMessages error path ====================

func TestClient_FetchMessages_Error(t *testing.T) {
	t.Parallel()

	mc := &mockConnector{
		fetchFn: func(_ imap.NumSet, _ *imap.FetchOptions) ([]*imapclient.FetchMessageBuffer, error) {
			return nil, errTestSearch
		},
	}

	client := newMockClient(mc)

	_, err := client.fetchMessages(seqSetFrom(1, 2), testMailboxINBOX, false)
	if err == nil {
		t.Fatal("expected error")
	}
}

// ==================== DeleteDraft error from getFolderByRole ====================

func TestClient_DeleteDraft_NoDraftsFolder(t *testing.T) {
	t.Parallel()

	mc := &mockConnector{
		listFn: func(_, _ string, _ *imap.ListOptions) ([]*imap.ListData, error) {
			return []*imap.ListData{{Mailbox: testMailboxINBOX}}, nil
		},
		statusFn: standardStatusFn(),
	}

	client := newMockClient(mc)

	err := client.DeleteDraft(context.Background(), 42)
	if err == nil {
		t.Fatal("expected error when Drafts folder not found")
	}
}

// ==================== DeleteMessage additional error paths ====================

func TestClient_DeleteMessage_SelectError(t *testing.T) {
	t.Parallel()

	mc := &mockConnector{
		listFn:   standardListFn(),
		statusFn: standardStatusFn(),
		selectFn: func(_ string, _ *imap.SelectOptions) (*imap.SelectData, error) {
			return nil, errTestMailbox
		},
	}

	client := newMockClient(mc)

	err := client.DeleteMessage(context.Background(), testMailboxINBOX, 42, false)
	if err == nil {
		t.Fatal("expected error from Select in non-permanent delete")
	}
}

func TestClient_DeleteMessage_MoveError(t *testing.T) {
	t.Parallel()

	mc := &mockConnector{
		listFn:   standardListFn(),
		statusFn: standardStatusFn(),
		moveFn: func(_ imap.UIDSet, _ string) error {
			return errTestMove
		},
	}

	client := newMockClient(mc)

	err := client.DeleteMessage(context.Background(), testMailboxINBOX, 42, false)
	if err == nil {
		t.Fatal("expected error from Move in trash delete")
	}
}

func TestClient_DeleteMessage_ExpungeError(t *testing.T) {
	t.Parallel()

	mc := &mockConnector{
		expungeFn: func() error { return errTestClose },
	}

	client := newMockClient(mc)

	err := client.DeleteMessage(context.Background(), testMailboxINBOX, 42, true)
	if err == nil {
		t.Fatal("expected error from Expunge in permanent delete")
	}
}

func TestClient_DeleteMessage_NoTrashFolder(t *testing.T) {
	t.Parallel()

	mc := &mockConnector{
		listFn: func(_, _ string, _ *imap.ListOptions) ([]*imap.ListData, error) {
			return []*imap.ListData{{Mailbox: testMailboxINBOX}}, nil
		},
		statusFn: standardStatusFn(),
	}

	client := newMockClient(mc)

	err := client.DeleteMessage(context.Background(), testMailboxINBOX, 42, false)
	if err == nil {
		t.Fatal("expected error when trash folder not found")
	}
}

// ==================== fetchAttachmentBody additional paths ====================

func TestClient_FetchAttachmentBody_NotFound(t *testing.T) {
	t.Parallel()

	mc := &mockConnector{
		fetchFn: func(_ imap.NumSet, _ *imap.FetchOptions) ([]*imapclient.FetchMessageBuffer, error) {
			return nil, nil // empty result
		},
	}

	client := newMockClient(mc)

	_, err := client.fetchAttachmentBody(context.Background(), testMailboxINBOX, 999, 0)
	if !errors.Is(err, models.ErrMessageNotFound) {
		t.Errorf("expected ErrMessageNotFound, got: %v", err)
	}
}

// ==================== CopyMessage select error ====================

func TestClient_CopyMessage_SelectError(t *testing.T) {
	t.Parallel()

	mc := &mockConnector{
		selectFn: func(_ string, _ *imap.SelectOptions) (*imap.SelectData, error) {
			return nil, errTestMailbox
		},
	}

	client := newMockClient(mc)

	err := client.CopyMessage(context.Background(), testMailboxINBOX, 42, "Backup")
	if err == nil {
		t.Fatal("expected error from Select")
	}
}

// ==================== MoveMessage select error ====================

func TestClient_MoveMessage_SelectError(t *testing.T) {
	t.Parallel()

	mc := &mockConnector{
		selectFn: func(_ string, _ *imap.SelectOptions) (*imap.SelectData, error) {
			return nil, errTestMailbox
		},
	}

	client := newMockClient(mc)

	err := client.MoveMessage(context.Background(), testMailboxINBOX, 42, "Archive")
	if err == nil {
		t.Fatal("expected error from Select")
	}
}

// ==================== GetAttachments select error ====================

func TestClient_GetAttachments_SelectError(t *testing.T) {
	t.Parallel()

	mc := &mockConnector{
		selectFn: func(_ string, _ *imap.SelectOptions) (*imap.SelectData, error) {
			return nil, errTestMailbox
		},
	}

	client := newMockClient(mc)

	_, err := client.GetAttachments(context.Background(), testMailboxINBOX, 42)
	if err == nil {
		t.Fatal("expected error from Select")
	}
}

// ==================== SaveDraft append error path ====================

func TestClient_SaveDraft_NoDraftsFolder(t *testing.T) {
	t.Parallel()

	mc := &mockConnector{
		listFn: func(_, _ string, _ *imap.ListOptions) ([]*imap.ListData, error) {
			return []*imap.ListData{{Mailbox: testMailboxINBOX}}, nil
		},
		statusFn: standardStatusFn(),
	}

	client := newMockClient(mc)

	_, err := client.SaveDraft(context.Background(), []byte("draft"))
	if err == nil {
		t.Fatal("expected error when no Drafts folder")
	}
}

// ==================== Search with date filters ====================

func TestClient_Search_WithDateFilters(t *testing.T) {
	t.Parallel()

	mc := &mockConnector{
		searchFn: func(_ *imap.SearchCriteria, _ *imap.SearchOptions) (*imap.SearchData, error) {
			return &imap.SearchData{}, nil
		},
	}

	client := newMockClient(mc)

	emails, err := client.Search(
		context.Background(), testMailboxINBOX, "",
		"sender@test.com", "to@test.com",
		"2026-01-01", "2026-02-01",
		50, false,
	)
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}

	if len(emails) != 0 {
		t.Errorf("expected 0 emails, got %d", len(emails))
	}
}

// ==================== ListMessages with body ====================

func TestClient_ListMessages_WithBody(t *testing.T) {
	t.Parallel()

	mc := &mockConnector{
		selectFn: func(_ string, _ *imap.SelectOptions) (*imap.SelectData, error) {
			return &imap.SelectData{NumMessages: 1}, nil
		},
		fetchFn: func(_ imap.NumSet, _ *imap.FetchOptions) ([]*imapclient.FetchMessageBuffer, error) {
			return []*imapclient.FetchMessageBuffer{
				{
					UID:      1,
					Envelope: &imap.Envelope{Subject: "With Body"},
					BodySection: []imapclient.FetchBodySectionBuffer{
						{
							Section: &imap.FetchItemBodySection{
								Specifier: imap.PartSpecifierText,
							},
							Bytes: []byte("body content"),
						},
					},
				},
			}, nil
		},
	}

	client := newMockClient(mc)

	emails, err := client.ListMessages(context.Background(), testMailboxINBOX, 10, 0, true)
	if err != nil {
		t.Fatalf("ListMessages() error = %v", err)
	}

	if len(emails) != 1 {
		t.Fatalf("expected 1 email, got %d", len(emails))
	}

	if emails[0].Body != "body content" {
		t.Errorf("Body = %q", emails[0].Body)
	}
}

// ==================== applyEnvelope with BCC ====================

func TestClient_ParseMessageBuffer_WithBCC(t *testing.T) {
	t.Parallel()

	client := newMockClient(&mockConnector{})

	buf := &imapclient.FetchMessageBuffer{
		UID: 1,
		Envelope: &imap.Envelope{
			Subject: "BCC test",
			From:    []imap.Address{{Mailbox: "a", Host: "b.com"}},
			To:      []imap.Address{{Mailbox: "c", Host: "d.com"}},
			Cc:      []imap.Address{{Mailbox: "e", Host: "f.com"}},
			Bcc:     []imap.Address{{Mailbox: "g", Host: "h.com"}},
			Date:    time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		},
	}

	email := client.parseMessageBuffer(buf, testMailboxINBOX, false)

	if len(email.BCC) != 1 || email.BCC[0] != "g@h.com" {
		t.Errorf("BCC = %v", email.BCC)
	}
}

// ==================== ListUnread additional paths ====================

func TestClient_ListUnread_SelectError(t *testing.T) {
	t.Parallel()

	mc := &mockConnector{
		selectFn: func(_ string, _ *imap.SelectOptions) (*imap.SelectData, error) {
			return nil, errTestMailbox
		},
	}

	client := newMockClient(mc)

	_, err := client.ListUnread(context.Background(), testMailboxINBOX, 50, false)
	if err == nil {
		t.Fatal("expected error from Select")
	}
}

func TestClient_ListUnread_LimitResults(t *testing.T) {
	t.Parallel()

	mc := &mockConnector{
		searchFn: func(_ *imap.SearchCriteria, _ *imap.SearchOptions) (*imap.SearchData, error) {
			return &imap.SearchData{All: seqSetFrom(1, 2, 3, 4, 5)}, nil
		},
		fetchFn: func(_ imap.NumSet, _ *imap.FetchOptions) ([]*imapclient.FetchMessageBuffer, error) {
			return []*imapclient.FetchMessageBuffer{
				{UID: 4, Envelope: &imap.Envelope{Subject: "Unread 4"}},
				{UID: 5, Envelope: &imap.Envelope{Subject: "Unread 5"}},
			}, nil
		},
	}

	client := newMockClient(mc)

	emails, err := client.ListUnread(context.Background(), testMailboxINBOX, 2, false)
	if err != nil {
		t.Fatalf("ListUnread() error = %v", err)
	}

	if len(emails) != 2 {
		t.Errorf("expected 2 emails (limit=2), got %d", len(emails))
	}
}

// ==================== GetDraft fetch error ====================

func TestClient_GetDraft_FetchError(t *testing.T) {
	t.Parallel()

	mc := &mockConnector{
		listFn:   standardListFn(),
		statusFn: standardStatusFn(),
		fetchFn: func(_ imap.NumSet, _ *imap.FetchOptions) ([]*imapclient.FetchMessageBuffer, error) {
			return nil, errTestSearch
		},
	}

	client := newMockClient(mc)

	_, err := client.GetDraft(context.Background(), 42)
	if err == nil {
		t.Fatal("expected error from fetch")
	}
}

// ==================== storeFlag select error ====================

func TestClient_SetFlag_SelectError(t *testing.T) {
	t.Parallel()

	mc := &mockConnector{
		selectFn: func(_ string, _ *imap.SelectOptions) (*imap.SelectData, error) {
			return nil, errTestMailbox
		},
	}

	client := newMockClient(mc)

	err := client.SetFlag(context.Background(), testMailboxINBOX, 42, true)
	if err == nil {
		t.Fatal("expected error from Select")
	}
}

func TestClient_FetchAttachmentBody_EmptyBody(t *testing.T) {
	t.Parallel()

	mc := &mockConnector{
		fetchFn: func(_ imap.NumSet, _ *imap.FetchOptions) ([]*imapclient.FetchMessageBuffer, error) {
			return []*imapclient.FetchMessageBuffer{
				{UID: 42, BodySection: nil},
			}, nil
		},
	}

	client := newMockClient(mc)

	_, err := client.fetchAttachmentBody(context.Background(), testMailboxINBOX, 42, 0)
	if !errors.Is(err, models.ErrMessageNotFound) {
		t.Errorf("expected ErrMessageNotFound, got: %v", err)
	}
}
