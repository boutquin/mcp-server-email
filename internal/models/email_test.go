package models

import (
	"testing"
)

func TestParseMessageID(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		id          string
		wantAccount string
		wantMailbox string
		wantUID     uint32
		wantErr     bool
	}{
		{
			name:        "valid simple",
			id:          "account1:INBOX:12345",
			wantAccount: "account1",
			wantMailbox: "INBOX",
			wantUID:     12345,
		},
		{
			name:    "mailbox with colon - invalid format",
			id:      "acc:Folder:With:Colons:999",
			wantErr: true, // UID parsing fails because "With:Colons:999" is not a valid number
		},
		{
			name:        "uid 1",
			id:          "test:Sent:1",
			wantAccount: "test",
			wantMailbox: "Sent",
			wantUID:     1,
		},
		{
			name:        "large uid",
			id:          "test:INBOX:4294967295",
			wantAccount: "test",
			wantMailbox: "INBOX",
			wantUID:     4294967295,
		},
		{
			name:    "missing first colon",
			id:      "accountINBOX:123",
			wantErr: true,
		},
		{
			name:    "missing second colon",
			id:      "account:INBOX123",
			wantErr: true,
		},
		{
			name:    "empty string",
			id:      "",
			wantErr: true,
		},
		{
			name:    "non-numeric uid",
			id:      "acc:INBOX:abc",
			wantErr: true,
		},
		{
			name:    "uid zero",
			id:      "acc:INBOX:0",
			wantErr: true,
		},
		{
			name:    "uid overflow",
			id:      "acc:INBOX:4294967296",
			wantErr: true,
		},
		{
			name:    "negative uid",
			id:      "acc:INBOX:-1",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			account, mailbox, uid, err := ParseMessageID(tt.id)
			if tt.wantErr {
				if err == nil {
					t.Errorf("ParseMessageID(%q) expected error, got nil", tt.id)
				}

				return
			}

			if err != nil {
				t.Errorf("ParseMessageID(%q) unexpected error: %v", tt.id, err)

				return
			}

			if account != tt.wantAccount {
				t.Errorf("ParseMessageID(%q) account = %q, want %q", tt.id, account, tt.wantAccount)
			}

			if mailbox != tt.wantMailbox {
				t.Errorf("ParseMessageID(%q) mailbox = %q, want %q", tt.id, mailbox, tt.wantMailbox)
			}

			if uid != tt.wantUID {
				t.Errorf("ParseMessageID(%q) uid = %d, want %d", tt.id, uid, tt.wantUID)
			}
		})
	}
}

func TestFormatMessageID(t *testing.T) {
	t.Parallel()

	tests := []struct {
		account string
		mailbox string
		uid     uint32
		want    string
	}{
		{"acc1", "INBOX", 123, "acc1:INBOX:123"},
		{"test", "Sent", 1, "test:Sent:1"},
		{"x", "y", 4294967295, "x:y:4294967295"},
		{"account", "Folder:With:Colons", 42, "account:Folder:With:Colons:42"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			t.Parallel()

			got := FormatMessageID(tt.account, tt.mailbox, tt.uid)
			if got != tt.want {
				t.Errorf("FormatMessageID(%q, %q, %d) = %q, want %q", tt.account, tt.mailbox, tt.uid, got, tt.want)
			}
		})
	}
}

func TestFormatParseRoundTrip(t *testing.T) {
	t.Parallel()

	tests := []struct {
		account string
		mailbox string
		uid     uint32
	}{
		{"acc1", "INBOX", 123},
		{"test", "Sent", 1},
		{"x", "y", 4294967295},
	}

	for _, tt := range tests {
		id := FormatMessageID(tt.account, tt.mailbox, tt.uid)

		account, mailbox, uid, err := ParseMessageID(id)
		if err != nil {
			t.Errorf(
				"ParseMessageID(FormatMessageID(%q, %q, %d)) unexpected error: %v",
				tt.account, tt.mailbox, tt.uid, err,
			)

			continue
		}

		if account != tt.account || mailbox != tt.mailbox || uid != tt.uid {
			t.Errorf(
				"roundtrip failed: got (%q, %q, %d), want (%q, %q, %d)",
				account, mailbox, uid, tt.account, tt.mailbox, tt.uid,
			)
		}
	}
}

func TestUitoa(t *testing.T) {
	t.Parallel()

	tests := []struct {
		in   uint32
		want string
	}{
		{0, "0"},
		{1, "1"},
		{123, "123"},
		{4294967295, "4294967295"},
	}

	for _, tt := range tests {
		got := uitoa(tt.in)

		if got != tt.want {
			t.Errorf("uitoa(%d) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestEmailError(t *testing.T) {
	t.Parallel()

	if ErrInvalidMessageID.Error() != "invalid message ID format" {
		t.Errorf("ErrInvalidMessageID.Error() = %q", ErrInvalidMessageID.Error())
	}

	if ErrAccountNotFound.Error() != "account not found" {
		t.Errorf("ErrAccountNotFound.Error() = %q", ErrAccountNotFound.Error())
	}

	if ErrFolderNotFound.Error() != "folder not found" {
		t.Errorf("ErrFolderNotFound.Error() = %q", ErrFolderNotFound.Error())
	}

	if ErrMessageNotFound.Error() != "message not found" {
		t.Errorf("ErrMessageNotFound.Error() = %q", ErrMessageNotFound.Error())
	}

	if ErrAuthFailed.Error() != "authentication failed" {
		t.Errorf("ErrAuthFailed.Error() = %q", ErrAuthFailed.Error())
	}

	if ErrConnectionFailed.Error() != "connection failed" {
		t.Errorf("ErrConnectionFailed.Error() = %q", ErrConnectionFailed.Error())
	}

	if ErrTimeout.Error() != "operation timed out" {
		t.Errorf("ErrTimeout.Error() = %q", ErrTimeout.Error())
	}
}

func TestEmailStruct(t *testing.T) {
	t.Parallel()

	email := Email{
		ID:        "acc:INBOX:123",
		Subject:   "Test Subject",
		From:      "sender@example.com",
		To:        []string{"recipient@example.com"},
		CC:        []string{"cc@example.com"},
		BCC:       []string{"bcc@example.com"},
		Date:      "2026-02-04T10:00:00Z",
		Body:      "Hello, World!",
		Mailbox:   "INBOX",
		IsUnread:  true,
		IsFlagged: false,
		Attachments: []AttachmentInfo{
			{Filename: "file.pdf", ContentType: "application/pdf", Size: 1024},
		},
		Account: "acc",
	}

	if email.ID != "acc:INBOX:123" {
		t.Errorf("Email.ID = %q", email.ID)
	}

	if email.Subject != "Test Subject" {
		t.Errorf("Email.Subject = %q", email.Subject)
	}

	if len(email.To) != 1 || email.To[0] != "recipient@example.com" {
		t.Errorf("Email.To = %v", email.To)
	}

	if len(email.Attachments) != 1 || email.Attachments[0].Filename != "file.pdf" {
		t.Errorf("Email.Attachments = %v", email.Attachments)
	}
}

func TestFolderStruct(t *testing.T) {
	t.Parallel()

	folder := Folder{
		Name:   "INBOX",
		Unread: 5,
		Total:  100,
	}

	if folder.Name != "INBOX" {
		t.Errorf("Folder.Name = %q", folder.Name)
	}

	if folder.Unread != 5 {
		t.Errorf("Folder.Unread = %d", folder.Unread)
	}

	if folder.Total != 100 {
		t.Errorf("Folder.Total = %d", folder.Total)
	}
}

func TestAccountStatusStruct(t *testing.T) {
	t.Parallel()

	status := AccountStatus{
		ID:        "acc1",
		Email:     "test@example.com",
		Connected: true,
		IsDefault: true,
	}

	if status.ID != "acc1" {
		t.Errorf("AccountStatus.ID = %q", status.ID)
	}

	if !status.Connected {
		t.Error("AccountStatus.Connected should be true")
	}

	if !status.IsDefault {
		t.Error("AccountStatus.IsDefault should be true")
	}
}
