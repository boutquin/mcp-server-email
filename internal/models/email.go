// Package models defines shared data structures for email operations.
package models

// Email represents an email message.
type Email struct {
	ID              string           `json:"id"`
	Subject         string           `json:"subject"`
	From            string           `json:"from"`
	To              []string         `json:"to"`
	CC              []string         `json:"cc,omitempty"`
	BCC             []string         `json:"bcc,omitempty"`
	Date            string           `json:"date"`
	Body            string           `json:"body,omitempty"`
	ContentType     string           `json:"contentType,omitempty"`
	Mailbox         string           `json:"mailbox"`
	IsUnread        bool             `json:"isUnread"`
	IsFlagged       bool             `json:"isFlagged"`
	Attachments     []AttachmentInfo `json:"attachments,omitempty"`
	Account         string           `json:"account,omitempty"`
	MessageIDHeader string           `json:"messageIdHeader,omitempty"`
	InReplyTo       string           `json:"inReplyTo,omitempty"`
	References      []string         `json:"references,omitempty"`
}

// AttachmentInfo represents email attachment metadata.
type AttachmentInfo struct {
	Index       int    `json:"index"`
	Filename    string `json:"filename"`
	ContentType string `json:"contentType"`
	Size        int64  `json:"size"`
}

// FolderRole represents the special-use purpose of a folder (RFC 6154).
type FolderRole string

const (
	RoleDrafts  FolderRole = "\\Drafts"
	RoleTrash   FolderRole = "\\Trash"
	RoleSent    FolderRole = "\\Sent"
	RoleJunk    FolderRole = "\\Junk"
	RoleArchive FolderRole = "\\Archive"
)

// Folder represents an email folder/mailbox.
type Folder struct {
	Name   string     `json:"name"`
	Unread int        `json:"unread"`
	Total  int        `json:"total"`
	Role   FolderRole `json:"role,omitempty"`
}

// AccountStatus represents the status of an email account.
type AccountStatus struct {
	ID        string `json:"id"`
	Email     string `json:"email"`
	Connected bool   `json:"connected"`
	IsDefault bool   `json:"isDefault"`
}

// findColon returns the index of the first ':' in id starting at offset, or -1.
func findColon(id string, offset int) int {
	for i := offset; i < len(id); i++ {
		if id[i] == ':' {
			return i
		}
	}

	return -1
}

// parseUID parses a UID string into a uint32, returning 0 on invalid input.
func parseUID(s string) (uint32, error) {
	var uidVal uint64

	for _, c := range s {
		if c < '0' || c > '9' {
			return 0, ErrInvalidMessageID
		}

		uidVal = uidVal*base10 + uint64(c-'0')
	}

	if uidVal == 0 || uidVal > 0xFFFFFFFF {
		return 0, ErrInvalidMessageID
	}

	return uint32(uidVal), nil
}

// ParseMessageID extracts account, mailbox, and UID from a message ID.
// Format: {account}:{mailbox}:{uid}
// Parsing splits on first two colons only (mailbox names may contain colons).
func ParseMessageID(id string) (string, string, uint32, error) {
	firstColon := findColon(id, 0)
	if firstColon == -1 {
		return "", "", 0, ErrInvalidMessageID
	}

	secondColon := findColon(id, firstColon+1)
	if secondColon == -1 {
		return "", "", 0, ErrInvalidMessageID
	}

	account := id[:firstColon]
	mailbox := id[firstColon+1 : secondColon]

	uid, err := parseUID(id[secondColon+1:])
	if err != nil {
		return "", "", 0, err
	}

	return account, mailbox, uid, nil
}

// FormatMessageID creates a message ID from components.
func FormatMessageID(account, mailbox string, uid uint32) string {
	return account + ":" + mailbox + ":" + uitoa(uid)
}

const (
	base10          = 10
	maxUint32Digits = 10
)

func uitoa(u uint32) string {
	if u == 0 {
		return "0"
	}

	var buf [maxUint32Digits]byte

	i := len(buf)
	for u > 0 {
		i--
		buf[i] = byte(u%base10) + '0'
		u /= base10
	}

	return string(buf[i:])
}

// EmailError is a string-based error type for email operations.
type EmailError string

func (e EmailError) Error() string { return string(e) }

const (
	ErrInvalidMessageID   EmailError = "invalid message ID format"
	ErrAccountNotFound    EmailError = "account not found"
	ErrFolderNotFound     EmailError = "folder not found"
	ErrMessageNotFound    EmailError = "message not found"
	ErrAuthFailed         EmailError = "authentication failed"
	ErrConnectionFailed   EmailError = "connection failed"
	ErrTimeout            EmailError = "operation timed out"
	ErrFolderRoleNotFound EmailError = "folder role not found"
)
