package log_test

import (
	"bytes"
	"testing"

	applog "github.com/boutquin/mcp-server-email/internal/log"
)

func TestInit_UnknownLevel(t *testing.T) {
	t.Setenv("LOG_LEVEL", "TRACE")

	var buf bytes.Buffer

	logger := applog.Init(&buf)

	// With an unrecognized level, parseLevel defaults to Info.
	// Debug messages should be suppressed.
	logger.Debug("should be suppressed")

	if buf.Len() != 0 {
		t.Errorf("unknown LOG_LEVEL should default to Info (suppress Debug), got: %s", buf.String())
	}

	// Info messages should still appear.
	logger.Info("visible")

	if buf.Len() == 0 {
		t.Error("Info messages should appear at default level")
	}
}
