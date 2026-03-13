package tools_test

import (
	"testing"

	"github.com/boutquin/mcp-server-email/internal/config"
	"github.com/boutquin/mcp-server-email/internal/tools"
	"github.com/mark3labs/mcp-go/server"
)

func TestLimitsFromConfig_CustomValues(t *testing.T) {
	t.Parallel()

	cfg := &config.Config{
		MaxAttachmentSizeMB:      25,
		MaxTotalAttachmentSizeMB: 25,
		MaxDownloadSizeMB:        50,
	}

	limits := tools.LimitsFromConfig(cfg)

	const mb = 1024 * 1024

	if limits.MaxFileSizeBytes != 25*mb {
		t.Errorf("MaxFileSizeBytes = %d, want %d", limits.MaxFileSizeBytes, int64(25*mb))
	}

	if limits.MaxTotalSizeBytes != 25*mb {
		t.Errorf("MaxTotalSizeBytes = %d, want %d", limits.MaxTotalSizeBytes, int64(25*mb))
	}

	if limits.MaxDownloadSizeBytes != 50*mb {
		t.Errorf("MaxDownloadSizeBytes = %d, want %d", limits.MaxDownloadSizeBytes, int64(50*mb))
	}
}

func TestLimitsFromConfig_Defaults(t *testing.T) {
	t.Parallel()

	cfg := &config.Config{
		MaxAttachmentSizeMB:      config.DefaultMaxAttachmentSizeMB,
		MaxTotalAttachmentSizeMB: config.DefaultMaxTotalAttachmentSizeMB,
		MaxDownloadSizeMB:        config.DefaultMaxDownloadSizeMB,
	}

	limits := tools.LimitsFromConfig(cfg)

	const mb = 1024 * 1024

	if limits.MaxFileSizeBytes != 18*mb {
		t.Errorf("MaxFileSizeBytes = %d, want %d", limits.MaxFileSizeBytes, int64(18*mb))
	}

	if limits.MaxTotalSizeBytes != 18*mb {
		t.Errorf("MaxTotalSizeBytes = %d, want %d", limits.MaxTotalSizeBytes, int64(18*mb))
	}

	if limits.MaxDownloadSizeBytes != 25*mb {
		t.Errorf("MaxDownloadSizeBytes = %d, want %d", limits.MaxDownloadSizeBytes, int64(25*mb))
	}
}

func TestRegisterAll(t *testing.T) {
	t.Parallel()

	imapMock := &mockIMAPOps{defaultAccountID: "acct1"}
	smtpMock := &mockSMTPOps{
		defaultAccountID: "acct1",
		accountEmails:    map[string]string{"acct1": "test@example.com"},
	}

	s := server.NewMCPServer("test", "0.0.0", server.WithToolCapabilities(true))

	// Should not panic
	tools.RegisterAll(s, imapMock, smtpMock, testLimits)

	// Verify expected tool count matches actual registrations.
	const expectedTools = 22

	registered := s.ListTools()
	if len(registered) != expectedTools {
		t.Errorf("expected %d tools registered, got %d", expectedTools, len(registered))
	}
}
