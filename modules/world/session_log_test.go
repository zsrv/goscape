package world

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"
)

// TestSlogLoggerBridgeSubmitSessionLogs verifies one slog record per entry
// with the expected attribute keys and values.
func TestSlogLoggerBridgeSubmitSessionLogs(t *testing.T) {
	var buf bytes.Buffer
	parent := slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelInfo}))
	b := NewSlogLoggerBridge(parent)

	logs := []SessionLog{
		{SessionUUID: "sess-a", Timestamp: 111, Coord: 222, Event: "foo", EventType: LoggerEventTypeModerator},
		{SessionUUID: "sess-b", Timestamp: 333, Coord: 444, Event: "bar", EventType: LoggerEventTypeEngine},
	}
	b.SubmitSessionLogs(logs)

	out := buf.String()
	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	if len(lines) != 2 {
		t.Fatalf("got %d slog lines, want 2: %s", len(lines), out)
	}
	for i, want := range []struct {
		session string
		event   string
	}{
		{"sess-a", "foo"},
		{"sess-b", "bar"},
	} {
		if !strings.Contains(lines[i], `"session":"`+want.session+`"`) {
			t.Errorf("line %d missing session=%q: %s", i, want.session, lines[i])
		}
		if !strings.Contains(lines[i], `"event":"`+want.event+`"`) {
			t.Errorf("line %d missing event=%q: %s", i, want.event, lines[i])
		}
		if !strings.Contains(lines[i], `"msg":"session_log"`) {
			t.Errorf("line %d missing msg=session_log: %s", i, lines[i])
		}
	}
}
