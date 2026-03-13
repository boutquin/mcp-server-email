// Package log provides structured logging via log/slog.
//
// Configuration is via environment variables:
//   - LOG_LEVEL: debug, info, warn, error (default: info)
//   - LOG_FORMAT: json, text (default: json)
package log

import (
	"io"
	"log/slog"
	"os"
	"strings"
)

// Init creates and returns a configured *slog.Logger based on environment variables.
// The returned logger should be stored and passed to subsystems that need logging.
func Init(w io.Writer) *slog.Logger {
	level := parseLevel(os.Getenv("LOG_LEVEL"))
	handler := newHandler(w, level, os.Getenv("LOG_FORMAT"))

	return slog.New(handler)
}

// With returns a child logger with the given key-value attributes.
func With(logger *slog.Logger, args ...any) *slog.Logger {
	return logger.With(args...)
}

// parseLevel converts a string level name to slog.Level.
func parseLevel(s string) slog.Level {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "debug":
		return slog.LevelDebug
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

// newHandler creates the appropriate slog handler based on format string.
func newHandler(w io.Writer, level slog.Level, format string) slog.Handler {
	opts := &slog.HandlerOptions{Level: level}

	if strings.EqualFold(strings.TrimSpace(format), "text") {
		return slog.NewTextHandler(w, opts)
	}

	return slog.NewJSONHandler(w, opts)
}
