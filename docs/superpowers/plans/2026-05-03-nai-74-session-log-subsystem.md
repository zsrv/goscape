# NAI-74 — Session-log subsystem foundation Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Port the TS session-log subsystem (per-tick batched `SessionLog` buffer + bridge dispatch + periodic `MODERATOR "Server check in"` rate + `SESSION_LOG` script opcode) and activate the two carry-forward `addSessionLog` call sites that have been waiting on it — closing `NAI-71-D-OPHELD-NO-SESSION-LOG` and `NAI-73-D-INPUT-NO-SESSION-LOG-KICK`.

**Architecture:** Add `LoggerEventType` numeric constants + `SessionLog` struct + `Server.sessionLogs []SessionLog` buffer in a new `modules/world/session_log.go`. Extend `LoggerBridge` interface with `SubmitSessionLogs([]SessionLog)` (per-tick batch shape mirroring TS `LoggerThread 'session_log'` channel); `slogLoggerBridge` emits one structured slog record per entry; `noopBridges` no-ops; test fixture `recordingBridges` snapshots each tick's batch. Add `Player.AddSessionLog(eventType int, message string, args ...string)` (TS variadic shape, args joined with `' '`). Add `Server.processSessionLogs()` tick phase that runs after `processCleanup` and before `currentTick++`, performing two responsibilities: (a) periodic `MODERATOR "Server check in"` push every `PlayerCoordLogRate=50` ticks (and `tick > 0`); (b) flush the buffer if non-empty. Add `ActivePlayer.AddSessionLog` interface method + `handleSessionLog` opcode handler (TS `+2` script-side enum offset preserved). Activate the two carry-forward sites and retire their deviation comments.

**Tech Stack:** Go 1.26+, `log/slog`, `time`, `strings`. TS source canonical path: `LostCityRS/Engine-TS`.

**Predecessor:** NAI-73 (HEAD `b9ba7a6` after spec commit). Spec: `docs/superpowers/specs/2026-05-03-nai-74-session-log-subsystem-design.md` (commit `b9ba7a6`).

**Constants** (defined once in `modules/world/session_log.go`):
- `LoggerEventTypeEngine    = 0` — server-only (TS `LoggerEventType.ENGINE`)
- `LoggerEventTypeWealth    = 1` — wealth_log channel (separate buffer; not in NAI-74)
- `LoggerEventTypeModerator = 2` — session_log moderator channel
- `LoggerEventTypeAdventure = 3` — visible to players
- `PlayerCoordLogRate       = 50` — TS `World.PLAYER_COORDLOGRATE` (World.ts:125), 30s @ 600ms

**Premises verified at HEAD `b9ba7a6`** (per `controller_preflight.md`):

```
$ rg -n "addSessionLog|SessionLog|sessionLog|LoggerEventType|SESSION_LOG" --include="*.go" .
pkg/script/opcode.go:198: OpSessionLog Opcode = 2098                  (reserved, no handler)
pkg/script/opcode.go:803-804: case OpSessionLog: return "SESSION_LOG"
modules/world/handler_opheld.go:29-32, 129-132                        (NAI-71-D deviation comments)
modules/world/input_tracking.go:165-168, 179-182                      (NAI-73-D deviation comments)

$ rg -n "loggerBridge\b" modules/world/server.go
126: loggerBridge LoggerBridge
164: s.loggerBridge = NewSlogLoggerBridge(s.log)

$ grep -n "type LoggerBridge\|func .* SubmitInputTracking\|func .* NotifyPlayerReport" modules/world/bridges.go modules/world/logger_bridge.go
modules/world/bridges.go:29-40   (interface — needs 3rd method)
modules/world/logger_bridge.go:27-48 (slogLoggerBridge — needs 3rd method impl)
modules/world/bridges.go:53-54   (noopBridges — needs 3rd method)

$ rg -n "session string" modules/world/player.go
221-225: session string  (defaults "headless"; assignment gated by NAI-72-D-LOGIN-SERVER-BRIDGE-MOD)

$ rg -n "PackCoord" pkg/coordgrid/
158: func PackCoord(level, x, z int) int

$ rg -n "currentTick\b\|playerLoop\b" modules/world/server.go modules/world/tick.go | head
modules/world/server.go:57: currentTick int
modules/world/tick.go:47-48: s.processCleanup(); s.currentTick++   (insertion point: between these two lines)

$ grep -n "ComName\b" pkg/objtype/componenttype.go
50: ComName string
```

**Test infrastructure available:**
- `newTestServer(t)` (`server_test.go:311-324`) — minimal Server with noopBridges
- `newTestPlayer(t)` (`player_test.go:15`) — Player + paired conn
- `newTestPlayerAt(t, s, slot, x, z, level)` (`interaction_test.go:865`) — Player at given coord
- `installRecordingBridges(s)` (`bridges_test.go:76-82`) — wires recordingBridges and returns recorder
- `setupOpHeldServer(t)` / `setupOpHeldTServer(t)` (`handler_opheld_test.go:28-58, 323-332`) — pre-wired Server+Player for OPHELD tests
- `inputTrackingTestSetup(t)` (in `input_tracking_test.go`) — tt + p + cc + rec
- `mockPlayer` (`pkg/script/runner_test.go:99-228`) — ActivePlayer mock; needs new `addSessionLogCalls` field + `AddSessionLog` method

---

### Task 1: Foundation data shapes + bridge interface extension

**Files:**
- Create: `modules/world/session_log.go`
- Modify: `modules/world/server.go` (add `sessionLogs []SessionLog` field)
- Modify: `modules/world/bridges.go` (extend `LoggerBridge` interface + `noopBridges`)
- Modify: `modules/world/logger_bridge.go` (extend `slogLoggerBridge`)
- Modify: `modules/world/bridges_test.go` (extend `recordingBridges`)
- Test: `modules/world/session_log_test.go` (new) and `modules/world/bridges_test.go` (extension)

This is a foundation-only task: data types, interface signature, and capture fixture. No call sites yet (`Player.AddSessionLog` lands in T2; tick wiring lands in T3). Must compile + tests for the slog impl + no-op + recording fixture pass at the end.

- [ ] **Step 1: Write the failing test for the slog bridge impl + noop + recording capture**

Append to `modules/world/bridges_test.go`:

```go
// TestNoopBridgesSubmitSessionLogs exercises the noop SubmitSessionLogs.
func TestNoopBridgesSubmitSessionLogs(t *testing.T) {
	var b noopBridges
	b.SubmitSessionLogs([]SessionLog{{SessionUUID: "x"}})
	// Must not panic; nothing else to assert.
}

// TestRecordingBridgesCapturesSubmitSessionLogs verifies snapshot semantics.
func TestRecordingBridgesCapturesSubmitSessionLogs(t *testing.T) {
	rec := &recordingBridges{}
	caller := []SessionLog{
		{SessionUUID: "alice", Timestamp: 1000, Coord: 50, Event: "hi", EventType: LoggerEventTypeModerator},
		{SessionUUID: "bob", Timestamp: 2000, Coord: 60, Event: "ho", EventType: LoggerEventTypeEngine},
	}
	rec.SubmitSessionLogs(caller)
	if len(rec.submittedSessionLogs) != 1 {
		t.Fatalf("submittedSessionLogs: got %d batches, want 1", len(rec.submittedSessionLogs))
	}
	got := rec.submittedSessionLogs[0]
	if len(got) != 2 || got[0].SessionUUID != "alice" || got[1].SessionUUID != "bob" {
		t.Errorf("batch contents: %+v", got)
	}
	// Mutation defense: mutate caller's slice; recorded snapshot must be unaffected.
	caller[0].SessionUUID = "MUTATED"
	if rec.submittedSessionLogs[0][0].SessionUUID != "alice" {
		t.Error("snapshot must not alias caller slice")
	}
}
```

Create `modules/world/session_log_test.go`:

```go
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
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/... -run "TestNoopBridgesSubmitSessionLogs|TestRecordingBridgesCapturesSubmitSessionLogs|TestSlogLoggerBridgeSubmitSessionLogs" -count=1`
Expected: build failure — `SessionLog` undefined, `LoggerEventTypeModerator` undefined, `noopBridges.SubmitSessionLogs` undefined, etc.

- [ ] **Step 3: Create `modules/world/session_log.go`**

```go
package world

// LoggerEventType is the TS LoggerEventType numeric enum domain
// (LoggerEventType.ts:1-9). Untyped int alias keeps the script-side
// ActivePlayer interface signature simple; production callers use the
// named constants below.
type LoggerEventType = int

const (
	LoggerEventTypeEngine    LoggerEventType = 0 // server engine only
	LoggerEventTypeWealth    LoggerEventType = 1 // wealth_log (separate buffer; not in NAI-74)
	LoggerEventTypeModerator LoggerEventType = 2 // session_log moderator channel
	LoggerEventTypeAdventure LoggerEventType = 3 // visible to players
)

// PlayerCoordLogRate mirrors TS World.PLAYER_COORDLOGRATE = 50
// (World.ts:125). Every PlayerCoordLogRate ticks (with tick > 0), each
// active player emits a MODERATOR "Server check in" record.
const PlayerCoordLogRate = 50

// SessionLog mirrors TS SessionLog (SessionLog.ts:1-7). One entry per
// addSessionLog call; flushed batched per tick by Server.processSessionLogs.
type SessionLog struct {
	SessionUUID string          // TS session_uuid
	Timestamp   int64           // TS timestamp (ms since epoch via time.Now().UnixMilli())
	Coord       int             // TS coord (CoordGrid.packCoord(level,x,z))
	Event       string          // TS event (message + ' ' + args.join(' ') if args, else message)
	EventType   LoggerEventType // TS event_type
}
```

- [ ] **Step 4: Add `sessionLogs []SessionLog` field to `Server` struct**

Modify `modules/world/server.go` — add immediately after the `loggerBridge` field (~line 126):

```go
loggerBridge   LoggerBridge

// sessionLogs is the per-tick session-log accumulator. NAI-74. Pushed by
// Player.AddSessionLog; flushed via processSessionLogs in the tick loop.
sessionLogs    []SessionLog
```

- [ ] **Step 5: Extend `LoggerBridge` interface + `noopBridges` impl**

Modify `modules/world/bridges.go` — interface body (lines 29-40):

```go
type LoggerBridge interface {
	// NotifyPlayerReport posts an abuse report (TS World.notifyPlayerReport
	// at World.ts:2297-2313, channel 'report'). reason is the string label
	// of the ReportAbuseReason enum value (e.g. "MACROING").
	NotifyPlayerReport(player *Player, offender, reason string)

	// SubmitInputTracking posts a per-player input-recording blob from the
	// anti-cheat tracking subsystem (TS World.submitInputTracking at
	// World.ts:2314-2321, channel 'input_track'). blob is the raw bytes
	// from the EVENT_TRACKING client packet.
	SubmitInputTracking(player *Player, blob []byte)

	// SubmitSessionLogs posts the per-tick batch of session-log entries.
	// Mirrors TS LoggerThread 'session_log' channel (LoggerThread.ts:31-37,
	// dispatched from World.cycle at World.ts:435-442). Called once per
	// tick by Server.processSessionLogs when the buffer is non-empty.
	SubmitSessionLogs(logs []SessionLog)
}
```

Add the noop method (after the existing `noopBridges.SubmitInputTracking` line ~54):

```go
func (noopBridges) SubmitSessionLogs([]SessionLog)              {}
```

- [ ] **Step 6: Extend `slogLoggerBridge`**

Append to `modules/world/logger_bridge.go` (after `SubmitInputTracking`, before the compile-time interface assertion):

```go
// SubmitSessionLogs emits one structured slog record per entry. The
// per-tick batch shape is preserved by the call cadence (one
// SubmitSessionLogs call per tick); per-entry record emission is
// chosen for grep/filter friendliness — this is a dev/debug sink, not
// the production LoggerClient WS transport which would JSON-batch.
func (b *slogLoggerBridge) SubmitSessionLogs(logs []SessionLog) {
	for _, lg := range logs {
		b.log.Info("session_log",
			"type", "session_log",
			"session", lg.SessionUUID,
			"timestamp_ms", lg.Timestamp,
			"coord", lg.Coord,
			"event_type", lg.EventType,
			"event", lg.Event,
		)
	}
}
```

- [ ] **Step 7: Extend `recordingBridges`**

Modify `modules/world/bridges_test.go`:

Add field to the struct (after `inputTracks`):

```go
type recordingBridges struct {
	friends             []recordedFriendsCall
	loginMod            []recordedLoginModCall
	logger              []recordedLoggerCall
	inputTracks         []recordedInputTrackingCall // NAI-73
	submittedSessionLogs [][]SessionLog              // NAI-74 — one element per tick flush
}
```

Add the capture method (after `SubmitInputTracking`):

```go
func (r *recordingBridges) SubmitSessionLogs(logs []SessionLog) {
	// Snapshot: defends against caller mutation between the call and assertion.
	snap := make([]SessionLog, len(logs))
	copy(snap, logs)
	r.submittedSessionLogs = append(r.submittedSessionLogs, snap)
}
```

The pre-existing compile-time assertions (`var _ LoggerBridge = (*recordingBridges)(nil)` at lines 89/92) will catch any signature drift — no extra assertion needed.

- [ ] **Step 8: Run all 3 new tests + the full bridges + slog suite**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/... -run "TestNoopBridges|TestRecordingBridges|TestSlogLoggerBridge" -count=1 -race`
Expected: PASS for all the existing TestNoopBridgesAllMethods, TestRecordingBridgesCapturesAllCalls, TestRecordingBridgesCapturesSubmitInputTracking + the 3 new tests. No regressions.

- [ ] **Step 9: Run full module to verify compilation**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./...`
Expected: clean build.

- [ ] **Step 10: Commit**

```bash
git add modules/world/session_log.go modules/world/session_log_test.go modules/world/server.go modules/world/bridges.go modules/world/bridges_test.go modules/world/logger_bridge.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(world): NAI-74 T1 — SessionLog struct + LoggerBridge.SubmitSessionLogs

Adds the foundation data shapes for the session-log subsystem:
LoggerEventType constants, PlayerCoordLogRate, SessionLog struct,
Server.sessionLogs accumulator field, and the third LoggerBridge
interface method (per-tick batch dispatch). slogLoggerBridge emits
one structured record per entry; noopBridges no-ops; recordingBridges
snapshots each batch for test capture.

No call sites yet: Player.AddSessionLog lands in T2, processSessionLogs
tick wiring in T3.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

### Task 2: `Player.AddSessionLog`

**Files:**
- Modify: `modules/world/player.go` (add `AddSessionLog` method)
- Test: `modules/world/session_log_test.go` (extension)

- [ ] **Step 1: Write the failing tests**

Append to `modules/world/session_log_test.go`:

```go
import (
	// ... existing imports kept ...
	"time"

	"github.com/zsrv/goscape/pkg/coordgrid"
)

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
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/... -run "TestPlayerAddSessionLog" -count=1`
Expected: build failure — `(*Player).AddSessionLog` undefined.

- [ ] **Step 3: Add `Player.AddSessionLog` method**

Append to `modules/world/player.go` (anywhere after the `session` field declaration; place it near the existing per-player observability methods if a natural cluster exists, else at file end):

```go
// AddSessionLog mirrors TS Player.addSessionLog (Player.ts:629-631) +
// World.addSessionLog (World.ts:2222-2231). Pushes one SessionLog onto
// Server.sessionLogs; flushed per-tick by Server.processSessionLogs.
//
// Variadic-arg join preserves TS quirk (World.ts:2227):
//   event = len(args) > 0 ? message + " " + strings.Join(args, " ") : message
//
// goscape defensive: nil-client / nil-server short-circuit (TS Player
// has no equivalent gate; in TS the World reference is module-global).
func (p *Player) AddSessionLog(eventType LoggerEventType, message string, args ...string) {
	if p.client == nil || p.client.server == nil {
		return
	}
	s := p.client.server
	event := message
	if len(args) > 0 {
		event = message + " " + strings.Join(args, " ")
	}
	s.sessionLogs = append(s.sessionLogs, SessionLog{
		SessionUUID: p.session,
		Timestamp:   time.Now().UnixMilli(),
		Coord:       coordgrid.PackCoord(p.level, p.x, p.z),
		Event:       event,
		EventType:   eventType,
	})
}
```

If `player.go` doesn't yet import `strings`, add it (verify via the existing import block — `time` and `github.com/zsrv/goscape/pkg/coordgrid` are already in `player.go`'s imports per HEAD inspection).

- [ ] **Step 4: Run tests to verify they pass**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/... -run "TestPlayerAddSessionLog" -count=1 -race`
Expected: all 4 tests PASS.

- [ ] **Step 5: Commit**

```bash
git add modules/world/player.go modules/world/session_log_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(world): NAI-74 T2 — Player.AddSessionLog (variadic, TS-shape)

Ports TS Player.addSessionLog (Player.ts:629-631) + the World-side push
into Server.sessionLogs (World.ts:2222-2231). Variadic-arg join uses
single-space separator per TS quirk; no-args branch produces no
trailing space. Defensive nil-client short-circuit (TS has no
equivalent gate since World is module-global there).

Tests: 4 — single push, variadic join, no-args, nil-client.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

### Task 3: `Server.processSessionLogs` + tick wiring

**Files:**
- Modify: `modules/world/session_log.go` (add `processSessionLogs` method)
- Modify: `modules/world/tick.go` (insert call in `runTickLoopWithRate`)
- Test: `modules/world/session_log_test.go` (extension)

- [ ] **Step 1: Write the failing tests**

Append to `modules/world/session_log_test.go`:

```go
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
// at tick == PlayerCoordLogRate, every player in playerLoop gets a
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
	s.playerLoop = []*Player{p1, p2}
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
	s.playerLoop = []*Player{p1}
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
	s.playerLoop = []*Player{p1}
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
	s.playerLoop = []*Player{p1}
	s.currentTick = PlayerCoordLogRate + 1 // not a rate tick

	s.processSessionLogs()

	if got := len(rec.submittedSessionLogs); got != 0 {
		t.Errorf("bridge calls: got %d, want 0 (non-rate tick + empty buffer)", got)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/... -run "TestProcessSessionLogs" -count=1`
Expected: build failure — `(*Server).processSessionLogs` undefined.

- [ ] **Step 3: Add `processSessionLogs` method**

Append to `modules/world/session_log.go`:

```go
// processSessionLogs runs as the last tick phase (after processCleanup,
// before currentTick++). Mirrors TS World.cycle() session-log block at
// World.ts:428-442:
//  1. If currentTick > 0 && currentTick % PlayerCoordLogRate == 0,
//     push MODERATOR "Server check in" for every player in playerLoop.
//  2. If sessionLogs is non-empty, dispatch via loggerBridge then clear.
//
// Empty-buffer skip matches TS (World.ts:435 `if (sessionLogs.length > 0)`).
// Coord-log push runs BEFORE flush so server-check-in entries land in
// the SAME tick's batch (matches TS source ordering at World.ts:428-442).
func (s *Server) processSessionLogs() {
	if s.currentTick > 0 && s.currentTick%PlayerCoordLogRate == 0 {
		for _, p := range s.playerLoop {
			p.AddSessionLog(LoggerEventTypeModerator, "Server check in")
		}
	}
	if len(s.sessionLogs) > 0 {
		s.loggerBridge.SubmitSessionLogs(s.sessionLogs)
		s.sessionLogs = nil
	}
}
```

- [ ] **Step 4: Wire `processSessionLogs` into the tick loop**

Modify `modules/world/tick.go` — insert `s.processSessionLogs()` between `s.processCleanup()` (line 47) and `s.currentTick++` (line 48):

```go
		s.processClientsOut()
		s.processCleanup()
		s.processSessionLogs() // NAI-74: TS World.cycle session-log block (W.ts:428-442)
		s.currentTick++
```

- [ ] **Step 5: Run the new tests**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/... -run "TestProcessSessionLogs" -count=1 -race`
Expected: all 6 tests PASS.

- [ ] **Step 6: Run the full module suite to catch regressions**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/... -count=1 -race`
Expected: PASS — no existing test should regress (tick wiring adds a noop call when sessionLogs is empty + non-rate tick).

- [ ] **Step 7: Commit**

```bash
git add modules/world/session_log.go modules/world/session_log_test.go modules/world/tick.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(world): NAI-74 T3 — Server.processSessionLogs + tick-loop wiring

Adds the per-tick session-log housekeeping phase between processCleanup
and currentTick++. Two responsibilities (TS World.ts:428-442 ordering):
  1. tick > 0 && tick % PlayerCoordLogRate == 0 → push MODERATOR
     "Server check in" for every player in playerLoop
  2. Non-empty buffer → bridge.SubmitSessionLogs + clear

Empty-buffer skip preserves TS World.ts:435 gate. Coord-log push
precedes flush so server-check-in entries land in the same tick's
batch.

Tests: 6 — flush, empty skip, coord-log push, tick=0 skip, phase
order, non-rate tick.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

### Task 4: `SESSION_LOG` script opcode

**Files:**
- Modify: `pkg/script/active.go` (extend `ActivePlayer` interface)
- Modify: `pkg/script/handlers_player.go` (add `handleSessionLog`)
- Modify: `pkg/script/handlers.go` (register dispatch entry)
- Modify: `pkg/script/runner_test.go` (extend `mockPlayer`)
- Modify: `modules/world/player.go` (signature already satisfies — verify in step 5)
- Test: `pkg/script/handlers_player_test.go` (new function)

- [ ] **Step 1: Write the failing tests**

Find the existing handler test pattern in `pkg/script/handlers_player_test.go` (search for an `ActivePlayer`-gated test such as `TestHandleStat` or similar) and append the following tests. If a `mockSessionLogCall` type doesn't exist, define it inline in the test file or in `runner_test.go` (match local convention).

Append to `pkg/script/handlers_player_test.go`:

```go
// TestHandleSessionLog pins the SESSION_LOG opcode (TS PlayerOps.ts:1184-1189).
// Stack convention: pushString(event); pushInt(eventType_unshifted) →
// handler pops eventType+2, pops event, calls Self.AddSessionLog(eventType+2, event).
func TestHandleSessionLog(t *testing.T) {
	mp := &mockPlayer{}
	s := &ScriptState{
		IntStack:    make([]int, StackCapacity),
		StringStack: make([]string, StackCapacity),
		IntLocals:   []int{},
		StringLocals: []string{},
		Pointers:    PtrActivePlayer,
		Self:        mp,
	}
	// Push string first (deeper), then int (top of int stack).
	s.PushString("hello")
	s.PushInt(0) // script-side 0 → engine-side MODERATOR (2)

	if err := handleSessionLog(s); err != nil {
		t.Fatalf("handleSessionLog: %v", err)
	}
	if got := len(mp.addSessionLogCalls); got != 1 {
		t.Fatalf("AddSessionLog calls: got %d, want 1", got)
	}
	call := mp.addSessionLogCalls[0]
	if call.eventType != 2 {
		t.Errorf("eventType: got %d, want 2 (script 0 → MODERATOR via +2 shift)", call.eventType)
	}
	if call.message != "hello" {
		t.Errorf("message: got %q, want %q", call.message, "hello")
	}
	if len(call.args) != 0 {
		t.Errorf("args: got %v, want empty", call.args)
	}
}

// TestHandleSessionLogModeratorAdventureMapping pins both script-side
// values: 0 → 2 (MODERATOR), 1 → 3 (ADVENTURE).
func TestHandleSessionLogModeratorAdventureMapping(t *testing.T) {
	cases := []struct {
		scriptVal int
		wantType  int
	}{
		{0, 2}, // MODERATOR
		{1, 3}, // ADVENTURE
	}
	for _, tc := range cases {
		t.Run(fmt.Sprintf("script%d_eng%d", tc.scriptVal, tc.wantType), func(t *testing.T) {
			mp := &mockPlayer{}
			s := &ScriptState{
				IntStack:    make([]int, StackCapacity),
				StringStack: make([]string, StackCapacity),
				Pointers:    PtrActivePlayer,
				Self:        mp,
			}
			s.PushString("evt")
			s.PushInt(tc.scriptVal)

			if err := handleSessionLog(s); err != nil {
				t.Fatalf("handleSessionLog: %v", err)
			}
			if mp.addSessionLogCalls[0].eventType != tc.wantType {
				t.Errorf("eventType: got %d, want %d", mp.addSessionLogCalls[0].eventType, tc.wantType)
			}
		})
	}
}

// TestHandleSessionLogRequiresActivePlayer pins the gate.
func TestHandleSessionLogRequiresActivePlayer(t *testing.T) {
	s := &ScriptState{
		IntStack:    make([]int, StackCapacity),
		StringStack: make([]string, StackCapacity),
		Pointers:    0, // no PtrActivePlayer
		Self:        nil,
	}
	if err := handleSessionLog(s); err == nil {
		t.Fatal("handleSessionLog: want error on missing ActivePlayer, got nil")
	}
}
```

If `fmt` isn't already imported in `handlers_player_test.go`, add it.

- [ ] **Step 2: Run tests to verify they fail**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/... -run "TestHandleSessionLog" -count=1`
Expected: build failure — `mockPlayer.addSessionLogCalls` undefined and `handleSessionLog` undefined.

- [ ] **Step 3: Extend `ActivePlayer` interface**

Modify `pkg/script/active.go` — add inside the `ActivePlayer interface { ... }` block (a logical grouping is near the existing observability methods such as `MessageGame` / `Username` — look for clustered docs):

```go
	// AddSessionLog pushes a session-log entry onto the server-level
	// per-tick buffer. Mirrors TS Player.addSessionLog (Player.ts:629-631).
	// eventType is the LoggerEventType numeric value (0=ENGINE, 1=WEALTH,
	// 2=MODERATOR, 3=ADVENTURE — see modules/world/session_log.go for
	// the typed constants). Variadic args are space-joined per TS quirk.
	// Wired by NAI-74.
	AddSessionLog(eventType int, message string, args ...string)
```

- [ ] **Step 4: Add `handleSessionLog`**

Append to `pkg/script/handlers_player.go` (place it near other ActivePlayer-gated handlers — the file already clusters by trigger family; placement is flexible):

```go
// handleSessionLog ports TS PlayerOps.ts:1184-1189 (SESSION_LOG opcode).
// Pops eventType (with TS +2 offset — script-content domain collapses
// the engine-only ENGINE/WEALTH values out, leaving 0=MODERATOR and
// 1=ADVENTURE for content authors) and event string, then dispatches
// to ActivePlayer.AddSessionLog. NAI-74.
func handleSessionLog(s *ScriptState) error {
	if err := requireActivePlayer(s, "SESSION_LOG"); err != nil {
		return err
	}
	eventType := s.PopInt() + 2
	event := s.PopString()
	s.Self.AddSessionLog(eventType, event)
	return nil
}
```

- [ ] **Step 5: Register dispatch entry**

Modify `pkg/script/handlers.go` — add to the `handlers` map (placement near other player ops; the map is grouped by sub-spec marker comments):

```go
	// NAI-74: SESSION_LOG opcode → ActivePlayer.AddSessionLog dispatch.
	OpSessionLog: handleSessionLog,
```

- [ ] **Step 6: Extend `mockPlayer` to satisfy the new interface method**

Modify `pkg/script/runner_test.go`:

Add field group and type near other capture fields in the `mockPlayer` struct (after the existing `messages []string` cluster, anywhere logical):

```go
	// NAI-74: SESSION_LOG opcode + Player.AddSessionLog capture.
	addSessionLogCalls []mockSessionLogCall
```

Add the type definition near the other mock-call types (alongside `mockEnqueue`, `mockHintCoord`, etc.):

```go
type mockSessionLogCall struct {
	eventType int
	message   string
	args      []string
}
```

Add the method (near the other `mockPlayer` method definitions):

```go
func (m *mockPlayer) AddSessionLog(eventType int, message string, args ...string) {
	// Defensive copy of args (variadic slice may alias caller storage).
	cp := make([]string, len(args))
	copy(cp, args)
	m.addSessionLogCalls = append(m.addSessionLogCalls, mockSessionLogCall{
		eventType: eventType,
		message:   message,
		args:      cp,
	})
}
```

- [ ] **Step 7: Verify `*world.Player` still satisfies `script.ActivePlayer`**

`Player.AddSessionLog` was added in T2 with signature `func (p *Player) AddSessionLog(eventType LoggerEventType, message string, args ...string)`. Since `LoggerEventType = int` (type alias, not a new type), this signature is identical to `AddSessionLog(eventType int, ...)` and satisfies the interface. No change needed.

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./...`
Expected: clean build. If the build fails on an interface-satisfaction error for `*world.Player`, the alias-vs-new-type distinction has been violated — re-check `session_log.go` step 3 in T1.

- [ ] **Step 8: Run the new tests + the existing pkg/script suite**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/... -count=1 -race`
Expected: 3 new tests PASS + all existing PASS (mockPlayer additions are purely additive).

- [ ] **Step 9: Run the full project test suite to catch interface-satisfaction failures elsewhere**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./... -count=1 -race`
Expected: PASS.

- [ ] **Step 10: Commit**

```bash
git add pkg/script/active.go pkg/script/handlers.go pkg/script/handlers_player.go pkg/script/handlers_player_test.go pkg/script/runner_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(script): NAI-74 T4 — SESSION_LOG opcode (2098) port + ActivePlayer.AddSessionLog

Ports TS PlayerOps.ts:1184-1189: pops eventType (with +2 shift; script
content domain is 0=MODERATOR / 1=ADVENTURE, collapsing the engine-
only ENGINE/WEALTH values out) and event string, dispatches to
ActivePlayer.AddSessionLog.

ActivePlayer interface gains AddSessionLog(eventType int, message
string, args ...string). *world.Player satisfies via T2's method
(LoggerEventType is an int alias). mockPlayer captures via
addSessionLogCalls slice.

Tests: 3 — basic dispatch with +2 shift, MODERATOR/ADVENTURE
mapping, ActivePlayer gate.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

### Task 5: Activate NAI-71-D in `handler_opheld.go`

**Files:**
- Modify: `modules/world/handler_opheld.go` (add `fmt` import + 2 activation sites + retire deviation comments)
- Test: `modules/world/handler_opheld_test.go` (new tests for the activation)

- [ ] **Step 1: Write the failing tests**

Append to `modules/world/handler_opheld_test.go`:

```go
// TestHandleOpHeldSessionLogPushOp1Through4 pins NAI-71-D close: every
// successful op != 5 dispatch pushes one MODERATOR session-log record
// formatted as "<iop> <debugname>". Exercises ops 1..4.
func TestHandleOpHeldSessionLogPushOp1Through4(t *testing.T) {
	cases := []struct {
		op       int
		iop      string
		wantSkip bool
	}{
		{1, "op1", false},
		{2, "op2", false},
		{3, "op3", false},
		{4, "op4", false},
	}
	for _, tc := range cases {
		t.Run(fmt.Sprintf("op%d", tc.op), func(t *testing.T) {
			s, p := setupOpHeldServer(t)
			// Seed all 5 IOp slots so each op is allowed.
			s.objTypes.Configs[555].IOp = []string{"op1", "op2", "op3", "op4", "op5"}
			p.session = "test-sess"
			p.level, p.x, p.z = 0, 3200, 3200

			err := handleOpHeld(p, opHeldPayload(555, 3, 149), tc.op)
			if err != nil {
				t.Fatalf("handleOpHeld op=%d: %v", tc.op, err)
			}

			if got := len(s.sessionLogs); got != 1 {
				t.Fatalf("sessionLogs after op=%d: got %d, want 1", tc.op, got)
			}
			lg := s.sessionLogs[0]
			wantEvent := tc.iop + " test_held"
			if lg.Event != wantEvent {
				t.Errorf("Event: got %q, want %q", lg.Event, wantEvent)
			}
			if lg.EventType != LoggerEventTypeModerator {
				t.Errorf("EventType: got %d, want MODERATOR(%d)", lg.EventType, LoggerEventTypeModerator)
			}
			if lg.SessionUUID != "test-sess" {
				t.Errorf("SessionUUID: got %q, want test-sess", lg.SessionUUID)
			}
		})
	}
}

// TestHandleOpHeldOp5NoSessionLog pins the TS wealth-log carve-out:
// op == 5 must NOT push a session-log (TS OpHeldHandler.ts:63).
func TestHandleOpHeldOp5NoSessionLog(t *testing.T) {
	s, p := setupOpHeldServer(t)
	s.objTypes.Configs[555].IOp = []string{"op1", "op2", "op3", "op4", "op5"}

	if err := handleOpHeld(p, opHeldPayload(555, 3, 149), 5); err != nil {
		t.Fatalf("handleOpHeld op=5: %v", err)
	}

	if got := len(s.sessionLogs); got != 0 {
		t.Errorf("sessionLogs: got %d, want 0 (op=5 must skip session-log)", got)
	}
}

// TestHandleOpHeldSessionLogBeforeScript pins that the session-log push
// happens unconditionally on the gates-passed path — even when no script
// is registered for the trigger. Mirrors TS unconditional addSessionLog
// at OpHeldHandler.ts:62-65 (line 64 runs before the line-69 dispatch).
func TestHandleOpHeldSessionLogBeforeScript(t *testing.T) {
	s, p := setupOpHeldServer(t)
	s.objTypes.Configs[555].IOp = []string{"op1", "", "", "", ""}
	// Do NOT register any script for the OPHELD1 trigger — runScript(nil) is no-op.

	if err := handleOpHeld(p, opHeldPayload(555, 3, 149), 1); err != nil {
		t.Fatalf("handleOpHeld op=1: %v", err)
	}

	if got := len(s.sessionLogs); got != 1 {
		t.Errorf("sessionLogs: got %d, want 1 (push must fire regardless of script presence)", got)
	}
}

// TestHandleOpHeldTSessionLogPush pins NAI-71-D close for OPHELDT:
// successful dispatch pushes one MODERATOR record formatted as
// "Cast <comName> on <debugname>" (TS OpHeldTHandler.ts:61).
func TestHandleOpHeldTSessionLogPush(t *testing.T) {
	s, p := setupOpHeldTServer(t)
	// Set ComName on spell component 200.
	s.componentTypes.Configs[200].ComName = "spell_blast"
	p.session = "wizard-sess"

	if err := handleOpHeldT(p, opHeldTPayload(555, 3, 149, 200)); err != nil {
		t.Fatalf("handleOpHeldT: %v", err)
	}

	if got := len(s.sessionLogs); got != 1 {
		t.Fatalf("sessionLogs: got %d, want 1", got)
	}
	lg := s.sessionLogs[0]
	wantEvent := "Cast spell_blast on test_held"
	if lg.Event != wantEvent {
		t.Errorf("Event: got %q, want %q", lg.Event, wantEvent)
	}
	if lg.EventType != LoggerEventTypeModerator {
		t.Errorf("EventType: got %d, want MODERATOR(%d)", lg.EventType, LoggerEventTypeModerator)
	}
	if lg.SessionUUID != "wizard-sess" {
		t.Errorf("SessionUUID: got %q, want wizard-sess", lg.SessionUUID)
	}
}

// TestHandleOpHeldTSessionLogMissingObjType pins the goscape-defensive
// guard: when the obj has no registered ObjType, the session-log is
// skipped (no panic). TS would throw at ObjType.get(obj).debugname.
func TestHandleOpHeldTSessionLogMissingObjType(t *testing.T) {
	s, p := setupOpHeldTServer(t)
	s.componentTypes.Configs[200].ComName = "spell_blast"
	// Place a stack item in inv so HasAt passes, but use an obj id whose
	// ObjType is nil (default-zero slice slot).
	s.invs[93].Items[3] = &inventory.Item{Id: 999, Count: 1}

	if err := handleOpHeldT(p, opHeldTPayload(999, 3, 149, 200)); err != nil {
		t.Fatalf("handleOpHeldT: %v", err)
	}

	// No panic; no session-log pushed because ObjType is nil.
	if got := len(s.sessionLogs); got != 0 {
		t.Errorf("sessionLogs: got %d, want 0 (missing ObjType must skip session-log)", got)
	}
}
```

The first test references `setupOpHeldServer(t)` which already wires `s.objTypes` and the `999`-test depends on `inventory.Item` available — the existing handler_opheld_test.go imports already include `inventory` and `objtype`. The `componentTypes` access pattern (`s.componentTypes.Configs[200]`) needs verification — confirm via `grep -n "seedComponentTypes\|componentTypes" modules/world/handler_opheld_test.go` that the seeded component is reachable post-seed by index. If the seeded shape differs, swap to whatever helper `setupOpHeldTServer` uses to allow per-component mutation.

If `componentTypes` is not directly accessible, replace the `s.componentTypes.Configs[200].ComName = "spell_blast"` lines with a re-seed via `seedComponentTypes(t, s, map[int]*objtype.ComponentType{200: {RootLayer: 200, ActionTarget: objtype.ComActionTargetHeld, ComName: "spell_blast"}, 149: {RootLayer: 149, Operable: true, Usable: true}})`.

- [ ] **Step 2: Run tests to verify they fail**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/... -run "TestHandleOpHeldSessionLog|TestHandleOpHeldOp5NoSessionLog|TestHandleOpHeldTSessionLog" -count=1`
Expected: FAIL — `len(s.sessionLogs) == 0` for the success cases (no AddSessionLog wired yet).

- [ ] **Step 3: Add `fmt` to handler_opheld.go imports**

Modify `modules/world/handler_opheld.go` — extend the import block (currently `pkg/io/packet`, `pkg/objtype`, `pkg/script`):

```go
import (
	"fmt"

	"github.com/zsrv/goscape/pkg/io/packet"
	"github.com/zsrv/goscape/pkg/objtype"
	"github.com/zsrv/goscape/pkg/script"
)
```

- [ ] **Step 4: Activate NAI-71-D in `handleOpHeld`**

Modify `modules/world/handler_opheld.go`:

**(a)** Replace the deviation doc-comment block (current lines 29-32):

Old:
```
// DEVIATION NAI-71-D-OPHELD-NO-SESSION-LOG: TS OpHeldHandler.ts:62-65
// calls addSessionLog(MODERATOR, ...) for op != 5. Skipped — no
// session-log subsystem in goscape. Closure path: future moderator-
// logging sub-spec ports LoggerEventType + session-log buffer.
```

New:
```
// Per TS OpHeldHandler.ts:62-65: op != 5 emits a MODERATOR session
// log "<iop> <debugname>". (op == 5 is wealth-logged in content
// scripts, not here.) NAI-74 activates this; the prior
// NAI-71-D-OPHELD-NO-SESSION-LOG deviation is closed.
```

**(b)** Insert the activation between the `p.masks |= p.entitymask` line (currently line 93) and the `trigger := script.TriggerOpHeld1 + ...` line (currently line 95). Resulting shape:

```go
	p.masks |= p.entitymask

	// NAI-74: NAI-71-D close. TS OpHeldHandler.ts:62-65 — unconditional
	// at this point in the pipeline (before script lookup).
	if op != 5 {
		p.AddSessionLog(LoggerEventTypeModerator,
			fmt.Sprintf("%s %s", objType.IOp[op-1], objType.DebugName))
	}

	trigger := script.TriggerOpHeld1 + script.ServerTriggerType(op-1)
```

- [ ] **Step 5: Activate NAI-71-D in `handleOpHeldT`**

Modify `modules/world/handler_opheld.go`:

**(a)** Replace the deviation doc-comment block (current lines 129-132):

Old:
```
// DEVIATION NAI-71-D-OPHELD-NO-SESSION-LOG: TS OpHeldTHandler.ts:61
// addSessionLog skipped — no session-log subsystem in goscape. Closure
// path: future moderator-logging sub-spec ports LoggerEventType +
// session-log buffer.
```

New:
```
// Per TS OpHeldTHandler.ts:61: emits a MODERATOR session log
// "Cast <comName> on <debugname>" before script dispatch. NAI-74
// activates this; the prior NAI-71-D-OPHELD-NO-SESSION-LOG
// deviation is closed.
```

**(b)** Insert the activation between the `p.masks |= p.entitymask` line (currently line 187) and the `sf := s.scriptProvider.GetByTrigger(...)` line (currently line 189). The activation needs an inline ObjType lookup since `handleOpHeldT` doesn't currently resolve obj→ObjType (the script dispatch keys on spellComId). Resulting shape:

```go
	p.masks |= p.entitymask

	// NAI-74: NAI-71-D close. TS OpHeldTHandler.ts:61 — unconditional at
	// this point in the pipeline. Inline ObjType lookup is goscape-only
	// (TS uses ObjType.get(obj).debugname which would throw on missing
	// config; goscape skips the session-log on missing — defensive,
	// goscape behaviour-preserving since TS would have thrown).
	if s.objTypes != nil && obj >= 0 && obj < len(s.objTypes.Configs) {
		if objType := s.objTypes.Configs[obj]; objType != nil {
			p.AddSessionLog(LoggerEventTypeModerator,
				fmt.Sprintf("Cast %s on %s", spellCom.ComName, objType.DebugName))
		}
	}

	sf := s.scriptProvider.GetByTrigger(script.TriggerOpHeldT, spellComId, -1)
```

- [ ] **Step 6: Verify all `NAI-71-D-OPHELD-NO-SESSION-LOG` references are retired**

Run: `rg -n "NAI-71-D-OPHELD-NO-SESSION-LOG" pkg/ modules/ cmd/`
Expected: only commit-message-style references in retirement-context comments OR no hits at all (depending on whether the retirement comments retain the tag for grep-discoverability; current commit-trailer convention preserves it). Per memory `retire_deviation_grep_all_comments.md`, every active *production* doc-comment must be retired or rewritten. Grep is run again post-commit in T7.

- [ ] **Step 7: Run the new tests + full opheld suite**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/... -run "TestHandleOpHeld" -count=1 -race`
Expected: All 5 new NAI-74 tests PASS + all existing TestHandleOpHeld* tests PASS (activation is purely additive on the gates-passed path; gate-rejecting tests untouched).

- [ ] **Step 8: Run the full project suite**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./... -count=1 -race`
Expected: PASS.

- [ ] **Step 9: Commit**

```bash
git add modules/world/handler_opheld.go modules/world/handler_opheld_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(world): NAI-74 T5 — activate NAI-71-D session-log push in OPHELD/OPHELDT

handleOpHeld now emits the TS-mandated MODERATOR session-log
("<iop> <debugname>") on op != 5, positioned BEFORE the script
lookup to match TS OpHeldHandler.ts:62-65 ordering (unconditional
at this pipeline point; runScript(nil) is no-op so script presence
is irrelevant).

handleOpHeldT now emits "Cast <comName> on <debugname>" before
script dispatch (TS OpHeldTHandler.ts:61). Adds an inline ObjType
lookup with the standard goscape-defensive nil/bounds guard
(handler previously didn't resolve obj→ObjType because dispatch
keys on spellComId).

Both deviation comment blocks retired in favour of "ports TS …"
references. Tests: 5 — op1..4 push, op5 skip, push-before-script,
OPHELDT push, OPHELDT missing-ObjType defensive skip.

Closes: NAI-71-D-OPHELD-NO-SESSION-LOG.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

### Task 6: Activate NAI-73-D in `input_tracking.go`

**Files:**
- Modify: `modules/world/input_tracking.go` (kick-branch activation + retire deviation comments)
- Modify: `modules/world/input_tracking_test.go` (extend `TestInputTrackingSubmitEventsMatrix` with session-log assertions)

- [ ] **Step 1: Extend `TestInputTrackingSubmitEventsMatrix`**

Modify `modules/world/input_tracking_test.go` `TestInputTrackingSubmitEventsMatrix`:

Add a `wantSessionLogPush bool` field to the cases struct and populate per case (only the `!report+!debug→kick` case sets it true):

```go
		cases := []struct {
			name               string
			hasSeenReport      bool
			shouldSubmit       bool
			nodeDebug          bool
			blobsBefore        [][]byte
			wantBridgeCalls    int
			wantKick           bool
			wantSubmittedBlob  []byte
			wantSessionLogPush bool // NAI-74
		}{
			{
				name:              "report+submit→bridge",
				hasSeenReport:     true,
				shouldSubmit:      true,
				nodeDebug:         false,
				blobsBefore:       [][]byte{{0xAA}, {0xBB}, {0xCC}},
				wantBridgeCalls:   1,
				wantKick:          false,
				wantSubmittedBlob: []byte{0xAA},
				wantSessionLogPush: false,
			},
			{
				name:               "report+!submit→nothing",
				hasSeenReport:      true,
				shouldSubmit:       false,
				nodeDebug:          false,
				blobsBefore:        [][]byte{{0xAA}},
				wantBridgeCalls:    0,
				wantKick:           false,
				wantSessionLogPush: false,
			},
			{
				name:               "!report+!debug→kick",
				hasSeenReport:      false,
				shouldSubmit:       false,
				nodeDebug:          false,
				blobsBefore:        nil,
				wantBridgeCalls:    0,
				wantKick:           true,
				wantSessionLogPush: true, // NAI-74: TS InputTracking.ts:150
			},
			{
				name:               "!report+debug→nothing",
				hasSeenReport:      false,
				shouldSubmit:       false,
				nodeDebug:          true,
				blobsBefore:        nil,
				wantBridgeCalls:    0,
				wantKick:           false,
				wantSessionLogPush: false,
			},
		}
```

In the loop body, after the existing assertions, append:

```go
			// NAI-74: NAI-73-D close — kick branch must push one ENGINE
			// session-log "Client did not submit an input tracking report".
			if tc.wantSessionLogPush {
				if got := len(p.client.server.sessionLogs); got != 1 {
					t.Errorf("sessionLogs: got %d, want 1", got)
				} else {
					lg := p.client.server.sessionLogs[0]
					if lg.EventType != LoggerEventTypeEngine {
						t.Errorf("EventType: got %d, want ENGINE(%d)", lg.EventType, LoggerEventTypeEngine)
					}
					if lg.Event != "Client did not submit an input tracking report" {
						t.Errorf("Event: got %q, want %q", lg.Event,
							"Client did not submit an input tracking report")
					}
				}
			} else {
				if got := len(p.client.server.sessionLogs); got != 0 {
					t.Errorf("sessionLogs: got %d, want 0", got)
				}
			}
```

- [ ] **Step 2: Run the matrix test to verify it fails**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/... -run "TestInputTrackingSubmitEventsMatrix" -count=1`
Expected: FAIL on the `!report+!debug→kick` subcase — `sessionLogs: got 0, want 1`.

- [ ] **Step 3: Activate the kick branch**

Modify `modules/world/input_tracking.go` — replace the kick-branch block (currently lines 178-184):

Old:
```go
	} else if !s.cfg.NodeDebug {
		// NAI-73-D-INPUT-NO-SESSION-LOG-KICK: TS also calls
		// player.addSessionLog(LoggerEventType.ENGINE, ...) on this
		// branch (InputTracking.ts:150). Goscape has no session-log
		// subsystem yet; deferred to a future session-log NAI.
		t.player.requestIdleLogout = true
	}
```

New:
```go
	} else if !s.cfg.NodeDebug {
		// NAI-74: NAI-73-D close. Per TS InputTracking.ts:150 — emits
		// an ENGINE session log noting the missed report alongside the
		// idle-logout request.
		t.player.AddSessionLog(LoggerEventTypeEngine,
			"Client did not submit an input tracking report")
		t.player.requestIdleLogout = true
	}
```

- [ ] **Step 4: Update the function-doc-comment kick-branch entry**

Modify `modules/world/input_tracking.go` — in `submitEvents`'s function doc-comment, update the `!hasSeenReport && !cfg.NodeDebug` branch description (currently lines 164-168):

Old:
```
//   - !hasSeenReport && !cfg.NodeDebug → requestIdleLogout = true
//     (TS additionally calls addSessionLog(ENGINE, "Client did not submit
//     an input tracking report") which is deferred via
//     NAI-73-D-INPUT-NO-SESSION-LOG-KICK; structured-log entry is
//     missing in goscape until the session-log NAI lands).
```

New:
```
//   - !hasSeenReport && !cfg.NodeDebug → ENGINE session log "Client did
//     not submit an input tracking report" + requestIdleLogout = true
//     (TS InputTracking.ts:150; ported in NAI-74).
```

- [ ] **Step 5: Verify all `NAI-73-D-INPUT-NO-SESSION-LOG-KICK` references are retired**

Run: `rg -n "NAI-73-D-INPUT-NO-SESSION-LOG-KICK" pkg/ modules/ cmd/`
Expected: no hits in production code (all retired). The tag remains in spec/plan/memory files (those are histories and stay).

- [ ] **Step 6: Run the matrix test + full input_tracking suite**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/... -run "TestInputTracking" -count=1 -race`
Expected: All 4 subcases of TestInputTrackingSubmitEventsMatrix PASS + all existing TestInputTracking* PASS.

- [ ] **Step 7: Run the full project suite**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./... -count=1 -race`
Expected: PASS.

- [ ] **Step 8: Commit**

```bash
git add modules/world/input_tracking.go modules/world/input_tracking_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(world): NAI-74 T6 — activate NAI-73-D session-log push in InputTracking kick

InputTracking.submitEvents kick branch (!hasSeenReport && !NodeDebug)
now pushes the TS-mandated ENGINE session-log "Client did not submit
an input tracking report" alongside the existing requestIdleLogout
flag (TS InputTracking.ts:150). Function doc-comment updated to
reflect the ported branch description.

Test: TestInputTrackingSubmitEventsMatrix extended with a
wantSessionLogPush field — kick subcase now asserts the ENGINE
record presence + payload; other 3 subcases assert no push.

Closes: NAI-73-D-INPUT-NO-SESSION-LOG-KICK.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

### Task 7: Close commit (memory + tracker entries)

**Files:**
- Modify: `/home/owner/.claude/projects/-home-owner-Code-github-com-zsrv-goscape/memory/nai_followups.md` (append NAI-74 entry + tracker rows for deferred call sites)
- Modify: `/home/owner/.claude/projects/-home-owner-Code-github-com-zsrv-goscape/memory/MEMORY.md` (no new entries expected unless a novel learning surfaced)
- No code changes; this is a docs/memory close commit.

- [ ] **Step 1: Final post-implementation grep verification**

Per memory `retire_deviation_grep_all_comments.md` — re-grep both retired tags across production code:

```
rg -n "NAI-71-D-OPHELD-NO-SESSION-LOG" pkg/ modules/ cmd/
rg -n "NAI-73-D-INPUT-NO-SESSION-LOG-KICK" pkg/ modules/ cmd/
```

Expected: no production-code hits (retirement-context references in the commit-message-style comments are deliberate and grep-discoverable per `close_commit_memory_trailer.md`; verify only that no *active* deviation comments remain).

- [ ] **Step 2: Re-run the full suite for green confirmation**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./... -count=1 -race`
Expected: PASS.

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go vet ./...`
Expected: clean.

- [ ] **Step 3: Append the NAI-74 entry to `nai_followups.md`**

Append the standard NAI close entry to `/home/owner/.claude/projects/-home-owner-Code-github-com-zsrv-goscape/memory/nai_followups.md`. Use the NAI-73 entry (lines ~3770-3870 in the file at HEAD `b9ba7a6`) as the template. The entry must include:

- HEAD SHA + timestamp
- Closes: NAI-71-D-OPHELD-NO-SESSION-LOG, NAI-73-D-INPUT-NO-SESSION-LOG-KICK
- Opens: 0 deviations
- Net deviation tally: 14 → 12
- Tracker entries opened (deferred call sites — these are bookkeeping rows, not deviations):
  - World.ts:823, 873, 884, 896, 904, 906 — login flow ENGINE/MODERATOR (6 sites). Lands with future login-handler port.
  - World.ts:1210, 1606 — force-remove + logout (2 sites). Lands with future logout-flow audit.
  - Player.ts:1775, 1795, 1798, 1801 — `advanceStat` ADVENTURE (4 sites). Lands with future stat-advance port.
  - TcpServer.ts:48, 55, 63 — TCP socket close/error/timeout. Lands with future socket-teardown audit.
  - ClientCheatHandler.ts:53 — "Ran cheat" MODERATOR. Lands with future cheat-handler port.
  - web.ts:159 — WS close. N/A — goscape has no WS path.
- Carry-forwards (still open after NAI-74):
  - NAI-72-D-FRIENDS-SERVER-BRIDGE — friends-server module port
  - NAI-72-D-LOGIN-SERVER-BRIDGE-MOD — moderation IPC + Player.session UUID assignment
  - NAI-69-D-APPLAYER-SELF2-REVERSED-NO-SAMETICK-RETRY
  - NAI-67-D-PLAYER-UNFOCUS-DEFERRED
  - NAI-34-D4-NPC + NAI-34-D5-NPC
  - NAI-35-T3-D1
  - All other deferred carry-forwards from NAI-65/66/67/68 unchanged.
- Memories applied (cite each that surfaced during this NAI):
  - `controller_preflight.md` — pre-dispatch grep verified all 9 risk-register premises at HEAD.
  - `retire_deviation_grep_all_comments.md` — both retired tags grep-checked at spec-write and post-commit.
  - `defensive_gate_doc_comment_label.md` — OPHELDT inline ObjType lookup labelled as goscape-only defensive.
  - `flat_arg_signature_for_cross_lang_parity.md` — `Player.AddSessionLog` ships variadic now even though no scope-internal site uses it.
  - `verify_implementer_claims.md` — fresh `go test ./... -count=1 -race` after each task per protocol.
  - `close_commit_memory_trailer.md` — close commit carries the standard `Closes memory:` trailer pointer.

- [ ] **Step 4: (Conditional) Add a new memory entry only if a novel learning surfaced**

If during T1-T6 a behaviour, gotcha, or pattern was learned that's non-derivable from code/commits and would benefit a future NAI — write it to a new file under `/home/owner/.claude/projects/-home-owner-Code-github-com-zsrv-goscape/memory/<short_name>.md` and add a one-line index entry to `MEMORY.md`. Otherwise skip this step (no forced-addition).

- [ ] **Step 5: Commit the memory updates + close commit on the project**

Memory commit (in the memory directory if it's a git repo; otherwise just save):

```bash
# In the goscape working directory — create the close commit.
git add -A   # only memory files outside the worktree are unaffected
git status   # verify only memory files changed (or, if memory is outside the project tree, verify nothing in goscape changed)
```

If memory files live outside the project worktree (typical), then T7 has no goscape commit — the close commit is the **last code commit** which was already made in T6. In that case, the "close" formalisation is a tag-only commit on the project (optional) or a commit-trailer convention applied to T6's commit (per memory `close_commit_memory_trailer.md`).

Apply the following commit on the project to make the close formal (after memory updates are saved):

```bash
git commit --allow-empty --no-gpg-sign -m "$(cat <<'EOF'
chore(close): NAI-74 — session-log subsystem foundation                       (closes NAI-71-D + NAI-73-D; net deviation tally 14 → 12)

Foundation tasks T1-T3:
  T1: SessionLog struct + LoggerBridge.SubmitSessionLogs (3rd channel)
  T2: Player.AddSessionLog (variadic, TS-shape)
  T3: Server.processSessionLogs + tick-loop wiring (coord-log @ rate=50, flush)

Script opcode T4:
  T4: SESSION_LOG opcode (2098) port + ActivePlayer.AddSessionLog interface

Carry-forward activation T5-T6:
  T5: handleOpHeld op!=5 + handleOpHeldT — MODERATOR push before script
      dispatch (TS OpHeld[T]Handler.ts ordering)
  T6: InputTracking.submitEvents kick branch — ENGINE push alongside
      requestIdleLogout (TS InputTracking.ts:150)

Tests: 6 foundation + 6 process + 3 opcode + 5 OPHELD/OPHELDT + 4
matrix subcases = 24 new asserts. No smoke required (Java-client smoke
deferred until LoggerClient transport ships).

Tracker entries opened (bookkeeping, NOT deviations) for the 16
remaining TS addSessionLog call sites — they land with their host
ports (login flow, logout, advance-stat, TCP teardown, ClientCheat).

Closes: NAI-71-D-OPHELD-NO-SESSION-LOG, NAI-73-D-INPUT-NO-SESSION-LOG-KICK.
Closes memory: nai_followups.md NAI-74 entry.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Self-review

**1. Spec coverage.** Walking the spec section-by-section:
- §3.1 (data shapes) → T1 step 3 ✓
- §3.2 (Player.AddSessionLog) → T2 step 3 ✓
- §3.3 (processSessionLogs) → T3 steps 3-4 ✓
- §3.4 (LoggerBridge extension + impls) → T1 steps 5-7 ✓
- §3.5 (SESSION_LOG opcode + ActivePlayer) → T4 ✓
- §3.6 (carry-forward activation) → T5 (NAI-71-D), T6 (NAI-73-D) ✓
- §3.7 (tracker entries) → T7 step 3 ✓
- §5.1-5.3 (test plan) — F1 (T2), F2 (T2), F3 (T2), F4 (T2), F5-F11 (T3), A1-A5 (T5), A6-A7 (T6), S1-S3 (T4). ✓
- §6 (impl order sketch) — followed with the T1↔T1 merge for compile-cleanness ✓
- §7 (risk register) — all 9 premises verified pre-dispatch in plan header ✓

**2. Placeholder scan.** No "TBD", "implement later", "similar to". All step code blocks are concrete. No "add appropriate error handling" hand-waves. ✓

**3. Type consistency.** `LoggerEventType = int` alias used consistently — `Player.AddSessionLog` declares `eventType LoggerEventType`, `ActivePlayer.AddSessionLog` declares `eventType int`. The alias makes both signatures identical (T4 step 7 explicitly verifies). `mockSessionLogCall.eventType` is `int` (T4 step 6). All capture/assertion code uses the constants `LoggerEventTypeEngine`/`Moderator`/`Adventure`. `SessionLog.Coord` is `int` matching `coordgrid.PackCoord` return. `SessionLog.Timestamp` is `int64` (UnixMilli). ✓

**4. Cross-task references.** Each task's "what was added in earlier task" reference is concrete (T2 references the `Server.sessionLogs` field added in T1; T3 references the `Player.AddSessionLog` added in T2; T5/T6 reference the `LoggerEventTypeModerator`/`Engine` constants added in T1). ✓
