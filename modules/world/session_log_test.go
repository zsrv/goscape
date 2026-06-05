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
	b := NewSlogLoggerBridge(parent, 10, "main")

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

// TestProcessSessionLogsFlush pins the basic flush behaviour:
// non-empty buffer → bridge called once with snapshot, buffer cleared.
func TestProcessSessionLogsFlush(t *testing.T) {
	s := newTestServer(t)
	rec := installRecordingBridges(s)
	s.sessionLogs = []SessionLog{
		{SessionUUID: "a", Event: "first"},
		{SessionUUID: "b", Event: "second"},
	}
	s.currentTick = 5 // non-rate tick — only flush exercises here

	s.processSessionLogs()

	if len(rec.submittedSessionLogs) != 1 {
		t.Fatalf("bridge calls: got %d, want 1", len(rec.submittedSessionLogs))
	}
	if got := len(rec.submittedSessionLogs[0]); got != 2 {
		t.Errorf("batch size: got %d, want 2", got)
	}
	if s.sessionLogs != nil {
		t.Errorf("sessionLogs: must be nil after flush, got %v", s.sessionLogs)
	}
}

// TestProcessSessionLogsEmptyNoFlush pins the TS empty-buffer skip:
// World.ts:435 gates dispatch on length > 0. Bridge must NOT be called.
func TestProcessSessionLogsEmptyNoFlush(t *testing.T) {
	s := newTestServer(t)
	rec := installRecordingBridges(s)
	s.currentTick = 5

	s.processSessionLogs()

	if got := len(rec.submittedSessionLogs); got != 0 {
		t.Errorf("bridge calls: got %d, want 0 (empty buffer must skip flush)", got)
	}
}

// TestProcessSessionLogsCoordLog pins the periodic MODERATOR push:
// at tick == PlayerCoordLogRate, every player in s.players gets a
// "Server check in" record with their packed coord.
func TestProcessSessionLogsCoordLog(t *testing.T) {
	s := newTestServer(t)
	rec := installRecordingBridges(s)
	p1, _ := newTestPlayer(t)
	p1.client.server = s
	p1.session = "p1"
	p1.level, p1.x, p1.z = 0, 3200, 3200
	p2, _ := newTestPlayer(t)
	p2.client.server = s
	p2.session = "p2"
	p2.level, p2.x, p2.z = 0, 3210, 3215
	p1.pid = 1
	s.players.set(1, p1)
	p2.pid = 2
	s.players.set(2, p2)
	s.currentTick = PlayerCoordLogRate

	s.processSessionLogs()

	if len(rec.submittedSessionLogs) != 1 {
		t.Fatalf("bridge calls: got %d, want 1", len(rec.submittedSessionLogs))
	}
	batch := rec.submittedSessionLogs[0]
	if len(batch) != 2 {
		t.Fatalf("batch size: got %d, want 2", len(batch))
	}
	for i, want := range []struct {
		session string
		coord   int
	}{
		{"p1", coordgrid.PackCoord(0, 3200, 3200)},
		{"p2", coordgrid.PackCoord(0, 3210, 3215)},
	} {
		if batch[i].SessionUUID != want.session {
			t.Errorf("batch[%d].SessionUUID: got %q, want %q", i, batch[i].SessionUUID, want.session)
		}
		if batch[i].EventType != LoggerEventTypeModerator {
			t.Errorf("batch[%d].EventType: got %d, want MODERATOR(%d)", i, batch[i].EventType, LoggerEventTypeModerator)
		}
		if batch[i].Event != "Server check in" {
			t.Errorf("batch[%d].Event: got %q, want %q", i, batch[i].Event, "Server check in")
		}
		if batch[i].Coord != want.coord {
			t.Errorf("batch[%d].Coord: got %d, want %d", i, batch[i].Coord, want.coord)
		}
	}
}

// TestProcessSessionLogsCoordLogTickZeroSkip pins the TS tick > 0 guard
// (World.ts:428). At tick=0, no coord-log push.
func TestProcessSessionLogsCoordLogTickZeroSkip(t *testing.T) {
	s := newTestServer(t)
	rec := installRecordingBridges(s)
	p1, _ := newTestPlayer(t)
	p1.client.server = s
	p1.pid = 1
	s.players.set(1, p1)
	s.currentTick = 0 // tick 0 with tick % rate == 0 must NOT push

	s.processSessionLogs()

	if got := len(rec.submittedSessionLogs); got != 0 {
		t.Errorf("bridge calls: got %d, want 0 (tick=0 must skip coord-log)", got)
	}
	if got := len(s.sessionLogs); got != 0 {
		t.Errorf("sessionLogs: got %d, want 0 (no coord-log push at tick=0)", got)
	}
}

// TestProcessSessionLogsCoordLogPhaseOrder pins that the coord-log push
// happens BEFORE the flush — the Server-check-in entries land in the
// SAME tick's batch (not deferred to next tick).
func TestProcessSessionLogsCoordLogPhaseOrder(t *testing.T) {
	s := newTestServer(t)
	rec := installRecordingBridges(s)
	p1, _ := newTestPlayer(t)
	p1.client.server = s
	p1.pid = 1
	s.players.set(1, p1)
	s.currentTick = PlayerCoordLogRate
	// Pre-seed an unrelated entry to verify it precedes the coord-log
	// push in the batch (insertion order = slice append order).
	s.sessionLogs = []SessionLog{{Event: "preseeded"}}

	s.processSessionLogs()

	if len(rec.submittedSessionLogs) != 1 || len(rec.submittedSessionLogs[0]) != 2 {
		t.Fatalf("batch shape: %+v", rec.submittedSessionLogs)
	}
	batch := rec.submittedSessionLogs[0]
	if batch[0].Event != "preseeded" {
		t.Errorf("batch[0].Event: got %q, want %q", batch[0].Event, "preseeded")
	}
	if batch[1].Event != "Server check in" {
		t.Errorf("batch[1].Event: got %q, want %q", batch[1].Event, "Server check in")
	}
}

// TestProcessSessionLogsNonRateTickNoCoordLog pins that on a non-rate
// tick, no coord-log push happens; if buffer is empty, no flush either.
func TestProcessSessionLogsNonRateTickNoCoordLog(t *testing.T) {
	s := newTestServer(t)
	rec := installRecordingBridges(s)
	p1, _ := newTestPlayer(t)
	p1.client.server = s
	p1.pid = 1
	s.players.set(1, p1)
	s.currentTick = PlayerCoordLogRate + 1 // not a rate tick

	s.processSessionLogs()

	if got := len(rec.submittedSessionLogs); got != 0 {
		t.Errorf("bridge calls: got %d, want 0 (non-rate tick + empty buffer)", got)
	}
}
