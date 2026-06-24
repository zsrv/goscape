package log

import (
	"fmt"
	"io"
	"log/slog"
	"path/filepath"
	"strconv"
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
	return slog.New(slog.NewTextHandler(w, handlerOptions(level)))
}

// NewStructuredLogger creates a logger that logs messages in JSON format.
func NewStructuredLogger(level slog.Level, w io.Writer) *slog.Logger {
	return slog.New(slog.NewJSONHandler(w, handlerOptions(level)))
}

// handlerOptions builds the shared slog.HandlerOptions: AddSource on, the
// given minimum level, and a ReplaceAttr that (1) renders LevelTrace as
// "TRACE" (slog would otherwise print "DEBUG-4") and (2) trims the source
// to "file.go:line" (the default prints the absolute compile path).
func handlerOptions(level slog.Level) *slog.HandlerOptions {
	return &slog.HandlerOptions{
		AddSource: true,
		Level:     level,
		ReplaceAttr: func(_ []string, a slog.Attr) slog.Attr {
			switch a.Key {
			case slog.LevelKey:
				if lvl, ok := a.Value.Any().(slog.Level); ok && lvl == LevelTrace {
					a.Value = slog.StringValue("TRACE")
				}
			case slog.SourceKey:
				if src, ok := a.Value.Any().(*slog.Source); ok && src != nil {
					a.Value = slog.StringValue(filepath.Base(src.File) + ":" + strconv.Itoa(src.Line))
				}
			}
			return a
		},
	}
}
