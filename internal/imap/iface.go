package imap

import (
	"context"

	"github.com/boutquin/mcp-server-email/internal/models"
)

// Operations defines the IMAP operations interface.
// Tool handlers accept this interface instead of concrete *Pool,
// enabling mock injection for unit tests.
//
//nolint:interfacebloat // 20 methods required: 18 IMAP operations + AccountStatus + DefaultAccountID
type Operations interface {
	ListFolders(ctx context.Context, accountID string) ([]models.Folder, error)
	GetFolderByRole(ctx context.Context, accountID string, role models.FolderRole) (string, error)
	ListMessages(
		ctx context.Context, accountID, folder string,
		limit, offset int, includeBody bool,
	) ([]models.Email, error)
	ListUnread(
		ctx context.Context, accountID, folder string,
		limit int, includeBody bool,
	) ([]models.Email, error)
	Search(
		ctx context.Context,
		accountID, mailbox, query, from, to, since, before string,
		limit int, includeBody bool,
	) ([]models.Email, error)
	GetMessage(ctx context.Context, accountID, folder string, uid uint32) (*models.Email, error)
	SearchByMessageID(ctx context.Context, accountID, folder, messageID string) ([]models.Email, error)
	GetAttachments(ctx context.Context, accountID, folder string, uid uint32) ([]models.AttachmentInfo, error)
	GetAttachment(ctx context.Context, accountID, folder string, uid uint32, index int) ([]byte, string, error)
	MoveMessage(ctx context.Context, accountID, folder string, uid uint32, dest string) error
	CopyMessage(ctx context.Context, accountID, folder string, uid uint32, dest string) error
	DeleteMessage(
		ctx context.Context, accountID, folder string,
		uid uint32, permanent bool,
	) error
	MarkRead(ctx context.Context, accountID, folder string, uid uint32, read bool) error
	SetFlag(ctx context.Context, accountID, folder string, uid uint32, flagged bool) error
	CreateFolder(ctx context.Context, accountID, name string) error
	SaveDraft(ctx context.Context, accountID string, msg []byte) (uint32, error)
	GetDraft(ctx context.Context, accountID string, uid uint32) ([]byte, error)
	DeleteDraft(ctx context.Context, accountID string, uid uint32) error
	AccountStatus() []models.AccountStatus
	DefaultAccountID() string
}

// Compile-time check: Pool implements Operations.
var _ Operations = (*Pool)(nil)
