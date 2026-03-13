package tools_test

import "github.com/boutquin/mcp-server-email/internal/tools"

// testLimits provides default attachment limits for tests matching pre-v6 defaults.
var testLimits = tools.AttachmentLimits{ //nolint:gochecknoglobals // test helper
	MaxFileSizeBytes:     18 * 1024 * 1024,
	MaxTotalSizeBytes:    18 * 1024 * 1024,
	MaxDownloadSizeBytes: 25 * 1024 * 1024,
}
