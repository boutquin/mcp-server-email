package tools_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/boutquin/mcp-server-email/internal/models"
	"github.com/boutquin/mcp-server-email/internal/smtp"
	"github.com/mark3labs/mcp-go/mcp"
)

// mockIMAPOps implements imap.Operations for handler-level tests.
type mockIMAPOps struct {
	defaultAccountID string

	// Return values for each method.
	listFoldersResult     []models.Folder
	listFoldersErr        error
	getFolderByRoleResult string
	getFolderByRoleErr    error
	listMessagesResult    []models.Email
	listMessagesErr       error
	listUnreadResult      []models.Email
	listUnreadErr         error
	searchResult          []models.Email
	searchErr             error
	getMessageResult      *models.Email
	getMessageErr         error
	moveMessageErr        error
	copyMessageErr        error
	deleteMessageErr      error
	markReadErr           error
	setFlagErr            error
	createFolderErr       error
	saveDraftUID          uint32
	saveDraftErr          error
	getDraftResult        []byte
	getDraftErr           error
	deleteDraftErr        error
	getAttachmentsResult  []models.AttachmentInfo
	getAttachmentsErr     error
	getAttachmentData     []byte
	getAttachmentFilename string
	getAttachmentErr      error
	getAttachmentByIndex  map[int]struct {
		Data     []byte
		Filename string
	} // per-index overrides
	searchByMessageIDResult []models.Email
	searchByMessageIDErr    error
	searchByMessageIDFunc   func(accountID, folder, msgID string) ([]models.Email, error)
	getFolderByRoleFunc     func(accountID string, role models.FolderRole) (string, error)
	accountStatuses         []models.AccountStatus

	// Captured arguments for verification.
	lastListFoldersAccountID string
	lastListMessagesArgs     listMessagesArgs
	lastListUnreadArgs       listUnreadArgs
	lastSearchArgs           searchArgs
	lastGetMessageArgs       getMessageArgs
	lastMoveMessageArgs      moveMessageArgs
	lastCopyMessageArgs      copyMessageArgs
	lastDeleteMessageArgs    deleteMessageArgs
	lastMarkReadArgs         markReadArgs
	lastSetFlagArgs          setFlagArgs
	lastCreateFolderArgs     createFolderArgs
	lastSaveDraftArgs        saveDraftArgs
	lastDeleteDraftArgs      deleteDraftArgs
	lastGetAttachmentsArgs   getAttachmentsArgs
	lastGetAttachmentArgs    getAttachmentArgs

	// Call tracking.
	accountStatusCalled bool
}

type listMessagesArgs struct {
	accountID   string
	folder      string
	limit       int
	offset      int
	includeBody bool
}

type listUnreadArgs struct {
	accountID   string
	folder      string
	limit       int
	includeBody bool
}

type searchArgs struct {
	accountID   string
	mailbox     string
	query       string
	from        string
	to          string
	since       string
	before      string
	limit       int
	includeBody bool
}

type getMessageArgs struct {
	accountID string
	folder    string
	uid       uint32
}

type moveMessageArgs struct {
	accountID string
	folder    string
	uid       uint32
	dest      string
}

type copyMessageArgs struct {
	accountID string
	folder    string
	uid       uint32
	dest      string
}

type deleteMessageArgs struct {
	accountID string
	folder    string
	uid       uint32
	permanent bool
}

type markReadArgs struct {
	accountID string
	folder    string
	uid       uint32
	read      bool
}

type setFlagArgs struct {
	accountID string
	folder    string
	uid       uint32
	flagged   bool
}

type createFolderArgs struct {
	accountID string
	name      string
}

type saveDraftArgs struct {
	accountID string
	msg       []byte
}

type deleteDraftArgs struct {
	accountID string
	uid       uint32
}

type getAttachmentsArgs struct {
	accountID string
	folder    string
	uid       uint32
}

type getAttachmentArgs struct {
	accountID string
	folder    string
	uid       uint32
	index     int
}

func (m *mockIMAPOps) ListFolders(_ context.Context, accountID string) ([]models.Folder, error) {
	m.lastListFoldersAccountID = accountID

	return m.listFoldersResult, m.listFoldersErr
}

func (m *mockIMAPOps) GetFolderByRole(
	_ context.Context, accountID string, role models.FolderRole,
) (string, error) {
	if m.getFolderByRoleFunc != nil {
		return m.getFolderByRoleFunc(accountID, role)
	}

	return m.getFolderByRoleResult, m.getFolderByRoleErr
}

func (m *mockIMAPOps) ListMessages(
	_ context.Context, accountID, folder string,
	limit, offset int, includeBody bool,
) ([]models.Email, error) {
	m.lastListMessagesArgs = listMessagesArgs{accountID, folder, limit, offset, includeBody}

	return m.listMessagesResult, m.listMessagesErr
}

func (m *mockIMAPOps) ListUnread(
	_ context.Context, accountID, folder string,
	limit int, includeBody bool,
) ([]models.Email, error) {
	m.lastListUnreadArgs = listUnreadArgs{accountID, folder, limit, includeBody}

	return m.listUnreadResult, m.listUnreadErr
}

func (m *mockIMAPOps) Search(
	_ context.Context,
	accountID, mailbox, query, from, to, since, before string,
	limit int, includeBody bool,
) ([]models.Email, error) {
	m.lastSearchArgs = searchArgs{accountID, mailbox, query, from, to, since, before, limit, includeBody}

	return m.searchResult, m.searchErr
}

func (m *mockIMAPOps) GetMessage(_ context.Context, accountID, folder string, uid uint32) (*models.Email, error) {
	m.lastGetMessageArgs = getMessageArgs{accountID, folder, uid}

	return m.getMessageResult, m.getMessageErr
}

func (m *mockIMAPOps) MoveMessage(_ context.Context, accountID, folder string, uid uint32, dest string) error {
	m.lastMoveMessageArgs = moveMessageArgs{accountID, folder, uid, dest}

	return m.moveMessageErr
}

func (m *mockIMAPOps) CopyMessage(_ context.Context, accountID, folder string, uid uint32, dest string) error {
	m.lastCopyMessageArgs = copyMessageArgs{accountID, folder, uid, dest}

	return m.copyMessageErr
}

func (m *mockIMAPOps) DeleteMessage(
	_ context.Context, accountID, folder string,
	uid uint32, permanent bool,
) error {
	m.lastDeleteMessageArgs = deleteMessageArgs{accountID, folder, uid, permanent}

	return m.deleteMessageErr
}

func (m *mockIMAPOps) MarkRead(_ context.Context, accountID, folder string, uid uint32, read bool) error {
	m.lastMarkReadArgs = markReadArgs{accountID, folder, uid, read}

	return m.markReadErr
}

func (m *mockIMAPOps) SetFlag(_ context.Context, accountID, folder string, uid uint32, flagged bool) error {
	m.lastSetFlagArgs = setFlagArgs{accountID, folder, uid, flagged}

	return m.setFlagErr
}

func (m *mockIMAPOps) CreateFolder(_ context.Context, accountID, name string) error {
	m.lastCreateFolderArgs = createFolderArgs{accountID, name}

	return m.createFolderErr
}

func (m *mockIMAPOps) SaveDraft(_ context.Context, accountID string, msg []byte) (uint32, error) {
	m.lastSaveDraftArgs = saveDraftArgs{accountID, msg}

	return m.saveDraftUID, m.saveDraftErr
}

func (m *mockIMAPOps) GetDraft(_ context.Context, accountID string, uid uint32) ([]byte, error) {
	m.lastDeleteDraftArgs = deleteDraftArgs{accountID, uid} // reuse for tracking

	return m.getDraftResult, m.getDraftErr
}

func (m *mockIMAPOps) DeleteDraft(_ context.Context, accountID string, uid uint32) error {
	m.lastDeleteDraftArgs = deleteDraftArgs{accountID, uid}

	return m.deleteDraftErr
}

func (m *mockIMAPOps) GetAttachments(
	_ context.Context, accountID, folder string, uid uint32,
) ([]models.AttachmentInfo, error) {
	m.lastGetAttachmentsArgs = getAttachmentsArgs{accountID, folder, uid}

	return m.getAttachmentsResult, m.getAttachmentsErr
}

func (m *mockIMAPOps) GetAttachment(
	_ context.Context, accountID, folder string, uid uint32, index int,
) ([]byte, string, error) {
	m.lastGetAttachmentArgs = getAttachmentArgs{accountID, folder, uid, index}

	if m.getAttachmentByIndex != nil {
		if entry, ok := m.getAttachmentByIndex[index]; ok {
			return entry.Data, entry.Filename, m.getAttachmentErr
		}
	}

	return m.getAttachmentData, m.getAttachmentFilename, m.getAttachmentErr
}

func (m *mockIMAPOps) SearchByMessageID(
	_ context.Context, accountID, folder, messageID string,
) ([]models.Email, error) {
	if m.searchByMessageIDFunc != nil {
		return m.searchByMessageIDFunc(accountID, folder, messageID)
	}

	return m.searchByMessageIDResult, m.searchByMessageIDErr
}

func (m *mockIMAPOps) AccountStatus() []models.AccountStatus {
	m.accountStatusCalled = true

	return m.accountStatuses
}

func (m *mockIMAPOps) DefaultAccountID() string {
	return m.defaultAccountID
}

// mockSMTPOps implements smtp.Operations for handler-level tests.
type mockSMTPOps struct {
	defaultAccountID string
	accountEmails    map[string]string
	accountEmailErr  error

	// Return values.
	sendErr error

	// Captured arguments.
	lastSendAccountID string
	lastSendReq       *smtp.SendRequest

	// Call tracking.
	sendCalled bool
}

func (m *mockSMTPOps) Send(_ context.Context, accountID string, req *smtp.SendRequest) error {
	m.sendCalled = true
	m.lastSendAccountID = accountID
	m.lastSendReq = req

	return m.sendErr
}

func (m *mockSMTPOps) AccountEmail(accountID string) (string, error) {
	if m.accountEmailErr != nil {
		return "", m.accountEmailErr
	}

	if m.accountEmails != nil {
		if email, ok := m.accountEmails[accountID]; ok {
			return email, nil
		}
	}

	return "", fmt.Errorf("%w: %s", models.ErrAccountNotFound, accountID)
}

func (m *mockSMTPOps) DefaultAccountID() string {
	return m.defaultAccountID
}

// toolReq creates a CallToolRequest with the given arguments.
func toolReq(args map[string]any) mcp.CallToolRequest {
	req := mcp.CallToolRequest{}
	req.Params.Arguments = args

	return req
}

// resultText extracts the text content from a CallToolResult.
func resultText(t *testing.T, result *mcp.CallToolResult) string {
	t.Helper()

	if len(result.Content) == 0 {
		t.Fatal("result has no content")
	}

	tc, ok := result.Content[0].(mcp.TextContent)
	if !ok {
		t.Fatalf("expected TextContent, got %T", result.Content[0])
	}

	return tc.Text
}
