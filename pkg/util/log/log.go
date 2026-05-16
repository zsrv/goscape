package log

import (
	"fmt"
	"io"
	"log/slog"
	"strings"
)

// NewLogger creates a slog.Logger writing to w in the given format.
// format is "text" (key-value) or "json".
func NewLogger(level slog.Level, format string, w io.Writer) (*slog.Logger, error) {
	switch strings.ToLower(format) {
	case "text":
		return NewStdLogger(level, w), nil
	case "json":
		return NewStructuredLogger(level, w), nil
	default:
		return nil, fmt.Errorf("invalid log format: %s", format)
	}
}

// NewStdLogger creates a logger that logs messages in key-value format.
func NewStdLogger(level slog.Level, w io.Writer) *slog.Logger {
	h := slog.NewTextHandler(w, &slog.HandlerOptions{
		AddSource: true,
		Level:     level,
	})

	return slog.New(h)
}

// NewStructuredLogger creates a logger that logs messages in JSON format.
func NewStructuredLogger(level slog.Level, w io.Writer) *slog.Logger {
	h := slog.NewJSONHandler(w, &slog.HandlerOptions{
		AddSource: true,
		Level:     level,
	})

	return slog.New(h)
}
