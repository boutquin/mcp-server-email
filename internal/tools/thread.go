package tools

import (
	"context"
	"slices"
	"strings"

	"github.com/boutquin/mcp-server-email/internal/imap"
	"github.com/boutquin/mcp-server-email/internal/models"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// ThreadTool returns the email_thread tool definition.
func ThreadTool() mcp.Tool {
	return mcp.NewTool("email_thread",
		mcp.WithDescription(
			"Get all messages in a conversation thread. "+
				"Searches across INBOX, Sent, Archive, and All Mail for complete conversations.",
		),
		mcp.WithString("id", mcp.Description("Message ID (any message in the thread)"), mcp.Required()),
	)
}

// ThreadHandler returns the handler for email_thread.
func ThreadHandler(ops imap.Operations) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		id, err := req.RequireString("id")
		if err != nil {
			return mcp.NewToolResultError("id is required"), nil
		}

		account, mailbox, uid, errResult := parseMessageParams(id)
		if errResult != nil {
			return errResult, nil
		}

		// Fetch the target message to get threading headers.
		target, err := ops.GetMessage(ctx, account, mailbox, uid)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		// Collect all Message-IDs to search for.
		searchIDs := collectThreadIDs(target)

		// Build list of folders to search.
		folders := threadFolders(ctx, ops, account, mailbox)

		// Search for all related messages across folders.
		thread := searchThread(ctx, ops, account, folders, searchIDs)

		// Include the original message.
		thread = ensureTargetIncluded(thread, target)

		// Sort by date ascending (oldest first).
		slices.SortFunc(thread, func(a, b models.Email) int {
			return strings.Compare(a.Date, b.Date)
		})

		return jsonResult(map[string]any{
			"thread": thread,
			"count":  len(thread),
		}), nil
	}
}

// threadFolders builds the list of folders to search for thread messages.
// Always includes the original folder, plus Sent (if available), INBOX
// (if the original message is from Sent), Archive, and Gmail All Mail.
func threadFolders(
	ctx context.Context,
	ops imap.Operations,
	account, originalFolder string,
) []string {
	seen := map[string]bool{originalFolder: true}
	folders := []string{originalFolder}

	// Try to add Sent folder.
	sentFolder, err := ops.GetFolderByRole(ctx, account, models.RoleSent)
	if err == nil && sentFolder != "" && !seen[sentFolder] {
		seen[sentFolder] = true
		folders = append(folders, sentFolder)
	}

	// If original is from Sent, also search INBOX.
	const inbox = "INBOX"

	if originalFolder == sentFolder && !seen[inbox] {
		seen[inbox] = true
		folders = append(folders, inbox)
	}

	// Try to add Archive folder.
	archiveFolder, err := ops.GetFolderByRole(ctx, account, models.RoleArchive)
	if err == nil && archiveFolder != "" && !seen[archiveFolder] {
		seen[archiveFolder] = true
		folders = append(folders, archiveFolder)
	}

	// Try Gmail All Mail (common path, not an RFC 6154 role).
	folders = appendAllMail(ctx, ops, account, seen, folders)

	return folders
}

// appendAllMail adds "[Gmail]/All Mail" to the folder list if it exists.
func appendAllMail(
	ctx context.Context,
	ops imap.Operations,
	account string,
	seen map[string]bool,
	folders []string,
) []string {
	const allMail = "[Gmail]/All Mail"

	if seen[allMail] {
		return folders
	}

	allFolders, err := ops.ListFolders(ctx, account)
	if err != nil {
		return folders
	}

	for _, f := range allFolders {
		if f.Name == allMail {
			seen[allMail] = true
			folders = append(folders, allMail)

			break
		}
	}

	return folders
}

// collectThreadIDs extracts all Message-IDs from a message's threading headers.
func collectThreadIDs(msg *models.Email) []string {
	seen := make(map[string]struct{})

	var ids []string

	add := func(id string) {
		id = strings.TrimSpace(id)
		if id == "" {
			return
		}

		if _, ok := seen[id]; ok {
			return
		}

		seen[id] = struct{}{}
		ids = append(ids, id)
	}

	add(msg.MessageIDHeader)
	add(msg.InReplyTo)

	for _, ref := range msg.References {
		add(ref)
	}

	return ids
}

// searchThread searches for all messages matching any of the given Message-IDs
// across multiple folders. Results are deduplicated by MessageIDHeader to handle
// the same message appearing in multiple folders (e.g., INBOX and Sent).
func searchThread(
	ctx context.Context,
	ops imap.Operations,
	account string,
	folders []string,
	messageIDs []string,
) []models.Email {
	seenMsgID := make(map[string]struct{})

	var result []models.Email

	for _, folder := range folders {
		for _, msgID := range messageIDs {
			msgs, err := ops.SearchByMessageID(ctx, account, folder, msgID)
			if err != nil {
				continue
			}

			for _, m := range msgs {
				dedupKey := m.MessageIDHeader
				if dedupKey == "" {
					dedupKey = m.ID // fallback for messages without Message-ID header
				}

				if _, ok := seenMsgID[dedupKey]; ok {
					continue
				}

				seenMsgID[dedupKey] = struct{}{}

				result = append(result, m)
			}
		}
	}

	return result
}

// ensureTargetIncluded adds the target message to the thread if not already present.
func ensureTargetIncluded(thread []models.Email, target *models.Email) []models.Email {
	dedupKey := target.MessageIDHeader
	if dedupKey == "" {
		dedupKey = target.ID
	}

	for _, m := range thread {
		mKey := m.MessageIDHeader
		if mKey == "" {
			mKey = m.ID
		}

		if mKey == dedupKey {
			return thread
		}
	}

	return append(thread, *target)
}
