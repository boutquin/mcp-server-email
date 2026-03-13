package log_test

import (
	"bytes"
	"strings"
	"testing"

	applog "github.com/boutquin/mcp-server-email/internal/log"
)

func TestInit_DefaultJSON(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer

	logger := applog.Init(&buf)

	logger.Info("test message")

	output := buf.String()
	if !strings.Contains(output, `"msg":"test message"`) {
		t.Errorf("expected JSON output with msg field, got: %s", output)
	}
}

func TestInit_TextFormat(t *testing.T) {
	t.Setenv("LOG_FORMAT", "text")

	var buf bytes.Buffer

	logger := applog.Init(&buf)

	logger.Info("hello")

	output := buf.String()
	if !strings.Contains(output, "hello") {
		t.Errorf("expected text output containing 'hello', got: %s", output)
	}

	// Text format should NOT produce JSON braces.
	if strings.Contains(output, "{") {
		t.Errorf("text format should not produce JSON, got: %s", output)
	}
}

func TestInit_DebugLevel(t *testing.T) {
	t.Setenv("LOG_LEVEL", "debug")

	var buf bytes.Buffer

	logger := applog.Init(&buf)

	logger.Debug("debug msg")

	output := buf.String()
	if !strings.Contains(output, "debug msg") {
		t.Errorf("debug level should output debug messages, got: %s", output)
	}
}

func TestInit_ErrorLevel(t *testing.T) {
	t.Setenv("LOG_LEVEL", "error")

	var buf bytes.Buffer

	logger := applog.Init(&buf)

	logger.Info("info msg")

	output := buf.String()
	if output != "" {
		t.Errorf("error level should suppress info messages, got: %s", output)
	}
}

func TestWith(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer

	logger := applog.Init(&buf)
	child := applog.With(logger, "account_id", "test123")

	child.Info("child msg")

	output := buf.String()
	if !strings.Contains(output, "test123") {
		t.Errorf("child logger should include account_id, got: %s", output)
	}
}
