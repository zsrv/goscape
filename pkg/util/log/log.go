package log

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"runtime"
	"time"
)

// Logger is the interface used throughout the project for logging.
type Logger interface {
	Debug(msg string, args ...any)
	Info(msg string, args ...any)
	Warn(msg string, args ...any)
	Error(msg string, args ...any)
}

func NewLogger(level slog.Level, format string) (Logger, error) {
	switch format {
	case "text":
		return NewStdLogger(level), nil
	case "json":
		return NewStructuredLogger(level), nil
	default:
		return nil, fmt.Errorf("invalid log format: %s", format)
	}
}

type StdLogger struct {
	logger *slog.Logger
}

// NewStdLogger creates a logger that logs messages in key-value format.
func NewStdLogger(level slog.Level) Logger {
	h := slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		AddSource: true,
		Level:     level,
	})

	return &StdLogger{
		logger: slog.New(h),
	}
}

func (l *StdLogger) Debug(msg string, args ...any) {
	slogSkipStackFrames(l.logger, slog.LevelDebug, msg, args...)
}

func (l *StdLogger) Info(msg string, args ...any) {
	slogSkipStackFrames(l.logger, slog.LevelInfo, msg, args...)
}

func (l *StdLogger) Warn(msg string, args ...any) {
	slogSkipStackFrames(l.logger, slog.LevelWarn, msg, args...)
}

func (l *StdLogger) Error(msg string, args ...any) {
	slogSkipStackFrames(l.logger, slog.LevelError, msg, args...)
}

// StructuredLogger writes log messages in JSON format.
type StructuredLogger struct {
	logger *slog.Logger
}

// NewStructuredLogger creates a logger that logs messages in JSON format.
func NewStructuredLogger(level slog.Level) Logger {
	h := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		AddSource: true,
		Level:     level,
	})

	return &StructuredLogger{
		logger: slog.New(h),
	}
}

func (l *StructuredLogger) Debug(msg string, args ...any) {
	slogSkipStackFrames(l.logger, slog.LevelDebug, msg, args...)
}

func (l *StructuredLogger) Info(msg string, args ...any) {
	slogSkipStackFrames(l.logger, slog.LevelInfo, msg, args...)
}

func (l *StructuredLogger) Warn(msg string, args ...any) {
	slogSkipStackFrames(l.logger, slog.LevelWarn, msg, args...)
}

func (l *StructuredLogger) Error(msg string, args ...any) {
	slogSkipStackFrames(l.logger, slog.LevelError, msg, args...)
}

func slogSkipStackFrames(logger *slog.Logger, level slog.Level, msg string, args ...any) {
	if !logger.Enabled(context.Background(), level) {
		return
	}
	var pcs [1]uintptr
	runtime.Callers(3, pcs[:])
	r := slog.NewRecord(time.Now(), level, msg, pcs[0])
	r.Add(args...)
	_ = logger.Handler().Handle(context.Background(), r)
}
