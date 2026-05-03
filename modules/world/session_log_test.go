package world

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/zsrv/goscape/pkg/coordgrid"
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

// TestPlayerAddSessionLog pins single-message push: SessionUUID/Coord/Event/EventType
// exact; Timestamp within ±5s of test-start (per goscape convention,
// see handler_reportabuse_test.go:102-107).
func TestPlayerAddSessionLog(t *testing.T) {
	s := newTestServer(t)
	p, _ := newTestPlayer(t)
	p.client.server = s
	p.session = "sess-uuid-1"
	p.level, p.x, p.z = 0, 3200, 3200

	startMs := time.Now().UnixMilli()
	p.AddSessionLog(LoggerEventTypeModerator, "Hello world")
	endMs := time.Now().UnixMilli()

	if got := len(s.sessionLogs); got != 1 {
		t.Fatalf("sessionLogs: got %d, want 1", got)
	}
	lg := s.sessionLogs[0]
	if lg.SessionUUID != "sess-uuid-1" {
		t.Errorf("SessionUUID: got %q, want sess-uuid-1", lg.SessionUUID)
	}
	if lg.Event != "Hello world" {
		t.Errorf("Event: got %q, want %q", lg.Event, "Hello world")
	}
	if lg.EventType != LoggerEventTypeModerator {
		t.Errorf("EventType: got %d, want %d", lg.EventType, LoggerEventTypeModerator)
	}
	wantCoord := coordgrid.PackCoord(0, 3200, 3200)
	if lg.Coord != wantCoord {
		t.Errorf("Coord: got %d, want %d", lg.Coord, wantCoord)
	}
	if lg.Timestamp < startMs || lg.Timestamp > endMs+5_000 {
		t.Errorf("Timestamp: got %d, want within [%d, %d+5s]", lg.Timestamp, startMs, endMs)
	}
}

// TestPlayerAddSessionLogVariadic pins TS variadic-arg join semantics:
// args.length ? message + ' ' + args.join(' ') : message.
func TestPlayerAddSessionLogVariadic(t *testing.T) {
	s := newTestServer(t)
	p, _ := newTestPlayer(t)
	p.client.server = s

	p.AddSessionLog(LoggerEventTypeModerator, "Logged", "alice", "uuid-x")

	if got := len(s.sessionLogs); got != 1 {
		t.Fatalf("sessionLogs: got %d, want 1", got)
	}
	if got := s.sessionLogs[0].Event; got != "Logged alice uuid-x" {
		t.Errorf("Event: got %q, want %q", got, "Logged alice uuid-x")
	}
}

// TestPlayerAddSessionLogNoArgsNoTrailingSpace pins the no-args branch:
// no trailing space when args is empty (TS args.length === 0 path).
func TestPlayerAddSessionLogNoArgsNoTrailingSpace(t *testing.T) {
	s := newTestServer(t)
	p, _ := newTestPlayer(t)
	p.client.server = s

	p.AddSessionLog(LoggerEventTypeAdventure, "Logged in")

	if got := s.sessionLogs[0].Event; got != "Logged in" {
		t.Errorf("Event: got %q, want %q (no trailing space)", got, "Logged in")
	}
}

// TestPlayerAddSessionLogNilClient pins the goscape-defensive guard:
// nil client returns without panic; buffer untouched.
func TestPlayerAddSessionLogNilClient(t *testing.T) {
	p := &Player{client: nil}
	// Must not panic.
	p.AddSessionLog(LoggerEventTypeEngine, "ignored")
	// Nothing to assert on a nil-client Player; absence of panic is the test.
}
