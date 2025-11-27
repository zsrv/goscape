package log

import (
	"fmt"
	"log/slog"
	"os"
	"strings"
)

func NewLogger(level slog.Level, format string) (*slog.Logger, error) {
	switch strings.ToLower(format) {
	case "text":
		return NewStdLogger(level), nil
	case "json":
		return NewStructuredLogger(level), nil
	default:
		return nil, fmt.Errorf("invalid log format: %s", format)
	}
}

// NewStdLogger creates a logger that logs messages in key-value format.
func NewStdLogger(level slog.Level) *slog.Logger {
	h := slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		AddSource: true,
		Level:     level,
	})

	return slog.New(h)
}

// NewStructuredLogger creates a logger that logs messages in JSON format.
func NewStructuredLogger(level slog.Level) *slog.Logger {
	h := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		AddSource: true,
		Level:     level,
	})

	return slog.New(h)
}
