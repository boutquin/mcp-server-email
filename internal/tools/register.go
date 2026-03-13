package tools

import (
	"github.com/boutquin/mcp-server-email/internal/config"
	"github.com/boutquin/mcp-server-email/internal/imap"
	"github.com/boutquin/mcp-server-email/internal/smtp"
	"github.com/mark3labs/mcp-go/server"
)

// AttachmentLimits holds byte-level attachment size limits derived from config.
type AttachmentLimits struct {
	MaxFileSizeBytes     int64
	MaxTotalSizeBytes    int64
	MaxDownloadSizeBytes int64
}

// LimitsFromConfig converts MB-based config values to byte-level limits.
func LimitsFromConfig(cfg *config.Config) AttachmentLimits {
	const mb = 1024 * 1024

	return AttachmentLimits{
		MaxFileSizeBytes:     int64(cfg.MaxAttachmentSizeMB) * mb,
		MaxTotalSizeBytes:    int64(cfg.MaxTotalAttachmentSizeMB) * mb,
		MaxDownloadSizeBytes: int64(cfg.MaxDownloadSizeMB) * mb,
	}
}

// RegisterAll registers all email tools with the MCP server.
func RegisterAll(
	s *server.MCPServer, imapOps imap.Operations, smtpOps smtp.Operations, limits AttachmentLimits,
) {
	// Account tools (1)
	s.AddTool(AccountsTool(), AccountsHandler(imapOps))

	// Folder tools (2)
	s.AddTool(FoldersTool(), FoldersHandler(imapOps))
	s.AddTool(FolderCreateTool(), FolderCreateHandler(imapOps))

	// Message list tools (3)
	s.AddTool(ListTool(), ListHandler(imapOps))
	s.AddTool(UnreadTool(), UnreadHandler(imapOps))
	s.AddTool(SearchTool(), SearchHandler(imapOps))

	// Message CRUD tools (6)
	s.AddTool(GetTool(), GetHandler(imapOps))
	s.AddTool(ReadBodyTool(), ReadBodyHandler(imapOps))
	s.AddTool(MoveTool(), MoveHandler(imapOps))
	s.AddTool(CopyTool(), CopyHandler(imapOps))
	s.AddTool(DeleteTool(), DeleteHandler(imapOps))
	s.AddTool(MarkReadTool(), MarkReadHandler(imapOps))

	// Flag tool (1)
	s.AddTool(FlagTool(), FlagHandler(imapOps))

	// Send tools (3)
	s.AddTool(SendTool(), SendHandler(smtpOps, limits))
	s.AddTool(DraftCreateTool(), DraftCreateHandler(imapOps, smtpOps, limits))
	s.AddTool(DraftSendTool(), DraftSendHandler(imapOps, smtpOps))

	// Reply/Forward tools (2)
	s.AddTool(ReplyTool(), ReplyHandler(imapOps, smtpOps))
	s.AddTool(ForwardTool(), ForwardHandler(imapOps, smtpOps, limits))

	// Attachment download tools (2)
	s.AddTool(AttachmentListTool(), AttachmentListHandler(imapOps))
	s.AddTool(AttachmentGetTool(), AttachmentGetHandler(imapOps, limits))

	// Batch tool (1)
	s.AddTool(BatchTool(), BatchHandler(imapOps))

	// Thread tool (1)
	s.AddTool(ThreadTool(), ThreadHandler(imapOps))
}
