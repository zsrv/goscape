package log

import (
	"context"
	"log/slog"
	"strings"
)

// LevelTrace is one step below slog.LevelDebug — the firehose level for
// per-packet / per-tick output. slog has no built-in trace level, so we
// define one. It is an ordinary slog.Level value (-8); handlers and the
// Trace helper use it directly.
const LevelTrace = slog.LevelDebug - 4

// Level is a config-only wrapper around slog.Level that additionally
// understands the name "trace". Config structs use Level so YAML and
// flags accept "trace"; it is converted back to slog.Level at the
// logger-construction boundary (NewLogger keeps an slog.Level signature).
type Level slog.Level

// UnmarshalText parses "trace" (case-insensitive) as LevelTrace and
// delegates everything else to slog.Level (debug/info/warn/error plus
// their +/-N offset forms).
func (l *Level) UnmarshalText(b []byte) error {
	if strings.EqualFold(strings.TrimSpace(string(b)), "trace") {
		*l = Level(LevelTrace)
		return nil
	}
	var sl slog.Level
	if err := sl.UnmarshalText(b); err != nil {
		return err
	}
	*l = Level(sl)
	return nil
}

// MarshalText renders LevelTrace as "TRACE" and delegates otherwise.
func (l Level) MarshalText() ([]byte, error) {
	if slog.Level(l) == LevelTrace {
		return []byte("TRACE"), nil
	}
	return slog.Level(l).MarshalText()
}

// String renders LevelTrace as "TRACE" and delegates otherwise.
func (l Level) String() string {
	if slog.Level(l) == LevelTrace {
		return "TRACE"
	}
	return slog.Level(l).String()
}

// Trace logs at LevelTrace. slog.Logger has no Trace method; this helper
// fills the gap for the handful of firehose call sites.
func Trace(l *slog.Logger, msg string, args ...any) {
	l.Log(context.Background(), LevelTrace, msg, args...)
}
