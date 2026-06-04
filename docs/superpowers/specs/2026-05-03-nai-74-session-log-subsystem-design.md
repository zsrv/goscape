# NAI-74 — Session-log subsystem foundation + carry-forward closes

**Status:** Spec written 2026-05-03.
**Predecessor:** NAI-73 (HEAD `a09d11b`). Net deviation tally entering: 14.
**Closes:** 2 deviations:
- `NAI-71-D-OPHELD-NO-SESSION-LOG` — `handleOpHeld` (op != 5) + `handleOpHeldT` activate the deferred `LoggerEventType.MODERATOR` push (TS `OpHeldHandler.ts:62-65`, `OpHeldTHandler.ts:61`).
- `NAI-73-D-INPUT-NO-SESSION-LOG-KICK` — `InputTracking.submitEvents` no-report kick branch activates the deferred `LoggerEventType.ENGINE` push (TS `InputTracking.ts:150`).

**Opens:** 0 new deviations. Deferred TS `addSessionLog` call sites (login flow, logout, advance-stat, TCP teardown, ClientCheat) get **tracker entries** referencing this NAI as their foundation — they ship with their host code in later NAIs.

**Net deviation tally projection: 14 → 12 (−2 closes, +0 opens).**

**Tech stack:** Go 1.26+. TS source canonical path: `LostCityRS/Engine-TS`. Rust source N/A.

## 1. Background

The TypeScript engine ships a per-tick batched **session-log** subsystem that records moderator/engine/adventure-class events tagged by player session UUID, packed coordinate, wall-clock timestamp, and a free-form event string. The buffer flushes once per tick to a worker thread that forwards to an external logger server (`LoggerThread.ts:31-37`, `LoggerClient.ts:9-24`).

In TS, the entry points are:
- `Player.addSessionLog(event_type, message, ...args)` (`Player.ts:629-631`) — variadic; joins args with `' '` if non-empty.
- `World.addSessionLog(event_type, session_uuid, coord, message, ...args)` (`World.ts:2222-2231`) — pushes one entry to `World.sessionLogs[]`.
- Per-tick flush at end of `World.cycle()` (`World.ts:435-442`): if `sessionLogs.length > 0`, `loggerThread.postMessage({type:'session_log', logs:...})` then clear.
- Periodic `MODERATOR "Server check in"` per player at `tick % PLAYER_COORDLOGRATE === 0` and `tick > 0` (`World.ts:428-432`, `PLAYER_COORDLOGRATE = 50`, `World.ts:125`).
- Script opcode `SESSION_LOG` (`PlayerOps.ts:1184-1189`) — pops `eventType+2` (script-side enum offsets ENGINE/WEALTH out, leaving MODERATOR/ADVENTURE for content) and message, calls `activePlayer.addSessionLog`.

`LoggerEventType` (`LoggerEventType.ts:1-9`) is a 4-value numeric enum:
| Value | Name | Description |
|---|---|---|
| 0 | ENGINE | server engine only (not visible) |
| 1 | WEALTH | wealth log channel (separate buffer; not in NAI-74 scope) |
| 2 | MODERATOR | session log channel — moderator visibility |
| 3 | ADVENTURE | visible to players (e.g. level-up announcements) |

In goscape, two prior NAIs deferred their `addSessionLog` calls because the subsystem was unported:
- **NAI-71-D** (`handler_opheld.go:29-32, 129-132`) — OPHELD ops 1..4 + OPHELDT
- **NAI-73-D** (`input_tracking.go:165-168, 179-182`) — InputTracking kick branch

Both were tagged with the same closure path: "future moderator-logging sub-spec ports `LoggerEventType` + session-log buffer." NAI-74 is that sub-spec.

The script opcode `OpSessionLog` (2098) is reserved at `pkg/script/opcode.go:198` with no handler yet — NAI-74 ports the dispatch.

## 2. Current state at HEAD (`a09d11b`)

### 2.1 Dependencies (verified present)

| Dependency | goscape location | Notes |
|---|---|---|
| `LoggerBridge` interface | `modules/world/bridges.go:29-40` | Has `NotifyPlayerReport` + `SubmitInputTracking`; needs third method. |
| `slogLoggerBridge` default impl | `modules/world/logger_bridge.go:15-51` | Already wired in `NewServer`; needs `SubmitSessionLogs` method. |
| `noopBridges` zero-impl | `modules/world/bridges.go:42-54` | Needs `SubmitSessionLogs` no-op. |
| `recordingBridges` test fixture | `modules/world/bridges_test.go` | Needs `submittedSessionLogs [][]SessionLog` capture field. |
| `Player.session string` | `modules/world/player.go:222-225` | Defaults `"headless"` (UUID assignment gated by NAI-72-D-LOGIN-SERVER-BRIDGE-MOD; bridge accepts `"headless"` correctly). |
| `Player.client *client` + `client.server *Server` | `modules/world/player.go` | Used to reach the server-level buffer. |
| `coordgrid.PackCoord(level, x, z) int` | `pkg/coordgrid/coordgrid.go:158` | TS `CoordGrid.packCoord` analogue. |
| `Server.currentTick int` | `modules/world/server.go:57` | Tick counter; advanced post-flush. |
| `Server.playerLoop []*Player` | `modules/world/server.go` | Iteration source for periodic coord-log. |
| `OpSessionLog Opcode = 2098` | `pkg/script/opcode.go:198` | Reserved; opcode `String()` returns `"SESSION_LOG"`. |
| `ScriptState.PopInt() int` / `PopString() string` | `pkg/script/state.go:259, 279` | Stack pop methods. |
| `ScriptState.Self ActivePlayer` | `pkg/script/state.go:187` | Active-player surface for handler dispatch. |
| `requireActivePlayer(s, op)` gate | `pkg/script/handlers_player.go:35-40` | Standard ActivePlayer validator. |
| `tick.runTickLoopWithRate` ordered phases | `modules/world/tick.go:25-61` | Phase list ends with `processCleanup` then `currentTick++`. |
| `objType.IOp []string` + `objType.DebugName string` + `objType.Category int` | `pkg/objtype/configtype.go` | OpHeld activation reads. |
| `spellCom.ComName string` | `pkg/objtype/componenttype.go:50` | OpHeldT activation reads. |
| `s.objTypes.Configs []*ObjType` | `modules/world/server.go` | OpHeld[T] objType lookup pattern (see `handler_opheld.go:71-77`). |
| `Player.requestIdleLogout bool` | `modules/world/player.go:213` | InputTracking kick activation reads. |
| `cfg.NodeDebug bool` | `modules/world/config.go:41` | InputTracking kick gate. |

### 2.2 Symbols added by NAI-74

| Symbol | Location (new or modified) |
|---|---|
| `LoggerEventType = int` (untyped const group) | new: `modules/world/session_log.go` |
| `LoggerEventTypeEngine = 0`, `…Wealth = 1`, `…Moderator = 2`, `…Adventure = 3` | new: `modules/world/session_log.go` |
| `PlayerCoordLogRate = 50` | new: `modules/world/session_log.go` |
| `SessionLog` struct | new: `modules/world/session_log.go` |
| `Server.sessionLogs []SessionLog` | modify: `modules/world/server.go` |
| `Server.processSessionLogs()` | new: `modules/world/session_log.go` |
| `Player.AddSessionLog(eventType int, message string, args ...string)` | new method: `modules/world/player.go` (or new file `player_session_log.go`) |
| `LoggerBridge.SubmitSessionLogs([]SessionLog)` interface method | modify: `modules/world/bridges.go` |
| `noopBridges.SubmitSessionLogs(...)` | modify: `modules/world/bridges.go` |
| `slogLoggerBridge.SubmitSessionLogs(...)` | modify: `modules/world/logger_bridge.go` |
| `recordingBridges.submittedSessionLogs [][]SessionLog` + capture in `SubmitSessionLogs` | modify: `modules/world/bridges_test.go` |
| `ActivePlayer.AddSessionLog(eventType int, message string, args ...string)` | modify: `pkg/script/active.go` |
| `handleSessionLog(s *ScriptState) error` | new: `pkg/script/handlers_player.go` |
| Dispatch entry `OpSessionLog: handleSessionLog` | modify: `pkg/script/handlers.go` |
| `processSessionLogs` invocation | modify: `modules/world/tick.go` |
| Activated session-log calls | modify: `modules/world/handler_opheld.go` (×2 sites) + `modules/world/input_tracking.go` (×1 site) |

## 3. Design

### 3.1 Data shapes (`modules/world/session_log.go`)

```go
package world

import (
	"strings"
	"time"

	"github.com/zsrv/goscape/pkg/coordgrid"
)

// LoggerEventType is the TS LoggerEventType numeric enum domain
// (LoggerEventType.ts:1-9). Untyped int aliases keep the script-side
// ActivePlayer interface signature simple; all production callers use
// the named constants below.
type LoggerEventType = int

const (
	LoggerEventTypeEngine    = 0 // server engine only
	LoggerEventTypeWealth    = 1 // wealth_log (separate buffer; not in NAI-74)
	LoggerEventTypeModerator = 2 // session_log moderator channel
	LoggerEventTypeAdventure = 3 // visible to players
)

// PlayerCoordLogRate is the TS World.PLAYER_COORDLOGRATE = 50
// (World.ts:125). Every PlayerCoordLogRate ticks (and tick > 0), each
// active player emits a MODERATOR "Server check in" record.
const PlayerCoordLogRate = 50

// SessionLog mirrors TS SessionLog (SessionLog.ts:1-7). One entry per
// addSessionLog call; flushed batched per tick.
type SessionLog struct {
	SessionUUID string             // TS session_uuid
	Timestamp   int64              // TS timestamp (ms since epoch via time.Now().UnixMilli())
	Coord       int                // TS coord (CoordGrid.packCoord(level,x,z))
	Event       string             // TS event (message + ' ' + args.join(' ') if args, else message)
	EventType   LoggerEventType    // TS event_type
}
```

### 3.2 `Player.AddSessionLog`

```go
// AddSessionLog mirrors TS Player.addSessionLog (Player.ts:629-631) +
// World.addSessionLog (World.ts:2222-2231). Pushes one SessionLog onto
// Server.sessionLogs; flushed per-tick by processSessionLogs.
//
// Variadic-arg join preserves TS quirk: 'message' + ' ' + args.join(' ')
// when len(args) > 0, else just 'message' (no trailing space).
//
// goscape defensive: nil-client / nil-server short-circuit (TS Player
// has no equivalent gate; in TS the server reference is module-global).
func (p *Player) AddSessionLog(eventType int, message string, args ...string) {
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

### 3.3 `Server.processSessionLogs`

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
// the same tick's batch (matches TS source ordering).
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

Tick-loop wiring (`modules/world/tick.go`): insert `s.processSessionLogs()` between `s.processCleanup()` and `s.currentTick++`.

### 3.4 `LoggerBridge` extension

```go
// modules/world/bridges.go
type LoggerBridge interface {
	NotifyPlayerReport(player *Player, offender, reason string)
	SubmitInputTracking(player *Player, blob []byte)
	SubmitSessionLogs(logs []SessionLog) // new — TS LoggerThread 'session_log' channel
}

func (noopBridges) SubmitSessionLogs([]SessionLog) {}
```

```go
// modules/world/logger_bridge.go — slogLoggerBridge gains:
//
// SubmitSessionLogs emits one structured slog record per entry. The
// per-tick batch shape is preserved by the call cadence; per-entry
// records are vastly more useful for grep/filter than a single
// JSON-blob-in-record (this is a dev/debug sink, not the production
// LoggerClient WS transport).
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

```go
// modules/world/bridges_test.go — recordingBridges gains:
type recordingBridges struct {
	// ... existing fields ...
	submittedSessionLogs [][]SessionLog // one element per processSessionLogs flush
}

func (r *recordingBridges) SubmitSessionLogs(logs []SessionLog) {
	// Snapshot: callers may reuse/clear the slice header after the call.
	snap := make([]SessionLog, len(logs))
	copy(snap, logs)
	r.submittedSessionLogs = append(r.submittedSessionLogs, snap)
}
```

### 3.5 `SESSION_LOG` script opcode

```go
// pkg/script/active.go — extend ActivePlayer interface
type ActivePlayer interface {
	// ... existing methods ...

	// AddSessionLog pushes a session-log entry onto the server-level
	// buffer. Mirrors TS Player.addSessionLog (Player.ts:629-631).
	// eventType is the LoggerEventType numeric value (0=ENGINE,
	// 1=WEALTH, 2=MODERATOR, 3=ADVENTURE). Variadic args join with ' '.
	AddSessionLog(eventType int, message string, args ...string)
}
```

```go
// pkg/script/handlers_player.go
//
// handleSessionLog ports TS PlayerOps.ts:1184-1189. Pops eventType
// (with TS +2 offset — script-content enum collapses ENGINE/WEALTH out,
// leaving 0=MODERATOR, 1=ADVENTURE for content authors) and event string,
// then calls activePlayer.AddSessionLog.
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

```go
// pkg/script/handlers.go — dispatch entry
OpSessionLog: handleSessionLog,
```

### 3.6 Carry-forward activation

#### NAI-71-D-OPHELD-NO-SESSION-LOG (`modules/world/handler_opheld.go`)

**handleOpHeld** — insert BEFORE `s.scriptProvider.GetByTrigger` (TS ordering: `OpHeldHandler.ts:62-65` runs before line 68 dispatch):

```go
// Replace the deviation comment block (lines 29-32) with:
// Per TS OpHeldHandler.ts:62-65, op != 5 emits a MODERATOR session log
// recording the iop string and obj debugname (op == 5 is wealth-logged
// in content scripts, not here).

// Activation site: AFTER `p.masks |= p.entitymask` (line 93), BEFORE
// the trigger lookup (line 95). objType + op-1 already validated by
// gates 7-9 (lines 71-80).
if op != 5 {
    p.AddSessionLog(LoggerEventTypeModerator,
        fmt.Sprintf("%s %s", objType.IOp[op-1], objType.DebugName))
}
```

**handleOpHeldT** — insert BEFORE `s.scriptProvider.GetByTrigger` (TS ordering: `OpHeldTHandler.ts:61` runs before line 63 dispatch). Requires inline objType lookup (current goscape handler doesn't resolve obj→objType because the script dispatch keys on spellComId):

```go
// Replace the deviation comment block (lines 129-132) with:
// Per TS OpHeldTHandler.ts:61, emits a MODERATOR session log recording
// "Cast <comName> on <debugname>".

// Activation site: AFTER `p.masks |= p.entitymask` (line 187), BEFORE
// the GetByTrigger lookup (line 189). ObjType lookup is goscape-only
// (TS uses `ObjType.get(obj).debugname` which would throw on missing
// config); goscape skips the session-log on missing rather than panic.
//
// goscape defensive: TS skips this nil-check (would throw).
if s.objTypes != nil && obj >= 0 && obj < len(s.objTypes.Configs) {
    if objType := s.objTypes.Configs[obj]; objType != nil {
        p.AddSessionLog(LoggerEventTypeModerator,
            fmt.Sprintf("Cast %s on %s", spellCom.ComName, objType.DebugName))
    }
}
```

#### NAI-73-D-INPUT-NO-SESSION-LOG-KICK (`modules/world/input_tracking.go`)

```go
// submitEvents kick branch — replace deviation comment (lines 179-182):
// Per TS InputTracking.ts:150, also emits an ENGINE session log noting
// the missed report, in addition to setting requestIdleLogout.
} else if !s.cfg.NodeDebug {
    t.player.AddSessionLog(LoggerEventTypeEngine,
        "Client did not submit an input tracking report")
    t.player.requestIdleLogout = true
}
```

The other input_tracking comment site (lines 165-168, in the function-level doc-comment listing branches) gets its parenthetical "(TS additionally calls addSessionLog(ENGINE, ...) which is deferred via NAI-73-D…)" rewritten to "(TS InputTracking.ts:150 — ported in NAI-74)".

#### Deviation-tag retirement (memory `retire_deviation_grep_all_comments.md`)

At spec-write, ran:
```
$ rg "NAI-71-D-OPHELD-NO-SESSION-LOG" pkg/ modules/ cmd/
modules/world/handler_opheld.go:29
modules/world/handler_opheld.go:130

$ rg "NAI-73-D-INPUT-NO-SESSION-LOG-KICK" pkg/ modules/ cmd/
modules/world/input_tracking.go:167
modules/world/input_tracking.go:179
```

Plan-author re-runs both greps before dispatching the activation tasks; every hit must be retired (either deletion or rewording).

### 3.7 Tracker entries opened (not deviations)

These call sites are **not** deviations — they're unported handlers/code paths whose `addSessionLog` calls land naturally with their host port. Each gets a tracker entry referencing NAI-74 as the foundation:

| TS site | Description | Lands with |
|---|---|---|
| `World.ts:823, 873, 884, 896, 904, 906` | Login flow ENGINE/MODERATOR (6 sites) | Future login-handler port (currently stubbed) |
| `World.ts:1210, 1606` | Force-remove + logout (2 sites) | Future logout-flow audit |
| `Player.ts:1775, 1795, 1798, 1801` | `advanceStat` ADVENTURE messages (4 sites) | Future stat-advance port |
| `TcpServer.ts:48, 55, 63` | TCP socket close/error/timeout | Future socket-teardown audit |
| `ClientCheatHandler.ts:53` | "Ran cheat" MODERATOR | Future cheat-handler port |
| `web.ts:159` | WS socket close | N/A — goscape has no WS path |

Tracker file convention follows existing carry-forward bookkeeping (memory `nai_followups.md`).

## 4. Non-obvious details / risks

### 4.1 `Player.session = "headless"` correlation key

The `Player.session` UUID is still defaulted to `"headless"` because NAI-72-D-LOGIN-SERVER-BRIDGE-MOD owns the per-login UUID assignment. This is fine for the foundation: records flow correctly with `"headless"` as the session_uuid, and the slog backend / future LoggerClient will see `session=headless` until that bridge ships. Do **not** treat this as a NAI-74 blocker.

### 4.2 OPHELD activation ordering vs goscape current dispatch

TS dispatches `runScript` on potentially-nil `script` (line 69 `if (script)` guard — does nothing on nil). goscape's current `handleOpHeld` uses `s.runScript(sf, p, nil, true, nil, nil)` which is also nil-safe (returns early). The session-log call must be positioned BEFORE the trigger lookup so it fires regardless of script presence — matches TS line 62-65 which is unconditional at that point.

### 4.3 `handleOpHeldT` lacks pre-existing objType lookup

Current `handleOpHeldT` doesn't resolve `obj` → `*ObjType` because the script dispatch keys on `spellComId`. The session-log MODERATOR record needs `objType.DebugName`, so the lookup is added inline at the activation site, with the standard goscape-defensive nil/bounds guard. Label the guard per memory `defensive_gate_doc_comment_label.md`.

### 4.4 Timestamp deterministic-test concern

No clock injection. Tests use the established goscape pattern (see `handler_reportabuse_test.go:102-107` — "5-second window for test timing") — assert `Timestamp` is within `±5s` of test-start; assert all other fields exactly.

### 4.5 Per-tick buffer slice ownership

`Server.sessionLogs` is reset to `nil` (not `s.sessionLogs[:0]`) after flush so the bridge implementation owns the slice it received and can store it without aliasing the next tick's accumulator. `recordingBridges.SubmitSessionLogs` snapshots defensively anyway.

### 4.6 Empty-buffer flush skip

TS `World.ts:435` gates the postMessage on `sessionLogs.length > 0`. Goscape mirrors with `if len(s.sessionLogs) > 0`. The bridge is **not** called on empty ticks. The slog default would still be quiet (no records emitted), but the noop bridge doesn't see calls — this matters for tests asserting "bridge not called on empty tick".

### 4.7 Variadic arg formatting — TS quirk

TS: `args.length ? message + ' ' + args.join(' ') : message`. Note the single space between message and the first arg (no comma, no separator chosen by caller). Go must mirror exactly: `event = message + " " + strings.Join(args, " ")`. Within NAI-74's scope no call site uses the variadic form — but the signature ships per memory `flat_arg_signature_for_cross_lang_parity.md` for forward-compat with deferred sites (login flow, TCP error, ClientCheat).

### 4.8 SESSION_LOG opcode's `+2` offset is a TS feature, not a bug

Script-content domain is a 2-value enum (0=MODERATOR, 1=ADVENTURE) because content has no business writing to ENGINE or WEALTH channels. The `+2` shift in `PlayerOps.ts:1185` collapses those engine-only values out. Mirror exactly — this is wire-format-equivalent for runescript bytecode portability.

## 5. Test plan

### 5.1 Foundation (`modules/world/session_log_test.go` — new file)

| # | Test | Asserts |
|---|---|---|
| F1 | `TestPlayerAddSessionLog` | Single-message push: SessionUUID/Coord/Event/EventType exact; Timestamp within ±5s of test-start. |
| F2 | `TestPlayerAddSessionLogVariadic` | `AddSessionLog(MODERATOR, "Logged", "alice", "uuid-x")` → Event = `"Logged alice uuid-x"`. |
| F3 | `TestPlayerAddSessionLogNoArgs` | No trailing space when args empty. |
| F4 | `TestPlayerAddSessionLogNilClient` | Defensive: `p.client = nil` returns without panic; buffer untouched. |
| F5 | `TestProcessSessionLogsFlush` | Two pushes → `processSessionLogs` calls bridge once with snapshot; `Server.sessionLogs == nil` after. |
| F6 | `TestProcessSessionLogsEmptyNoFlush` | Empty buffer → bridge **not** called. `len(rec.submittedSessionLogs) == 0`. |
| F7 | `TestProcessSessionLogsCoordLog` | At `currentTick = PlayerCoordLogRate`, every player in `playerLoop` gets a MODERATOR "Server check in" record (Coord matches each player's position). |
| F8 | `TestProcessSessionLogsCoordLogTickZeroSkip` | At `currentTick = 0`, no coord-log push (TS `tick > 0` guard). |
| F9 | `TestProcessSessionLogsCoordLogPhaseOrder` | Coord-log pushes happen BEFORE bridge flush (server-check-in entries land in the same tick's batch). |
| F10 | `TestProcessSessionLogsNonRateTickNoFlushIfEmpty` | At `currentTick = 1`, no coord-log; if no other entries exist, bridge not called. |
| F11 | `TestSlogLoggerBridgeSubmitSessionLogs` | N entries → N slog records emitted via the slog handler under message `"session_log"` with expected attrs. |

### 5.2 Carry-forward activation

| # | Test | Asserts |
|---|---|---|
| A1 | `TestOpHeldSessionLogPushOp1Through4` | For op ∈ {1,2,3,4}, `handleOpHeld` pushes one MODERATOR record `"<iop> <debugname>"` with player's coord/session. |
| A2 | `TestOpHeldOp5NoSessionLog` | `handleOpHeld5` does NOT push (TS wealth-log carve-out). |
| A3 | `TestOpHeldSessionLogBeforeScript` | Session-log push happens whether or not the script trigger resolves (use a fixture with no matching script — record still pushed). |
| A4 | `TestOpHeldTSessionLogPush` | `handleOpHeldT` pushes one MODERATOR `"Cast <comName> on <debugname>"`. |
| A5 | `TestOpHeldTSessionLogMissingObjType` | `objType == nil` → no panic, no record (defensive). |
| A6 | `TestInputTrackingSubmitEventsKickPushesEngineLog` | Kick branch (`!hasSeenReport && !cfg.NodeDebug`) pushes ENGINE record AND sets `requestIdleLogout = true`. |
| A7 | `TestInputTrackingSubmitEventsDebugSkipsKick` | `cfg.NodeDebug = true` → no record, no logout (existing test may already pin no-logout — extend with no-record assertion). |

### 5.3 Script opcode (`pkg/script/handlers_player_test.go`)

| # | Test | Asserts |
|---|---|---|
| S1 | `TestHandleSessionLog` | Stack: push `"hello"` then `0` → handler calls `Self.AddSessionLog(2, "hello")` (verifies `+2` shift). Use a mock ActivePlayer recorder. |
| S2 | `TestHandleSessionLogModeratorAdventureMapping` | eventType arg `0 → 2 (MODERATOR)`, `1 → 3 (ADVENTURE)`. |
| S3 | `TestHandleSessionLogRequiresActivePlayer` | No ActivePlayer pointer → returns the standard requireActivePlayer error. |

### 5.4 No smoke test

End-to-end behaviour is exercised by tests F1-F11 + A1-A7 + S1-S3. Java-client smoke is deferred until session-log records can be observed via the future LoggerClient transport — `slogLoggerBridge` records can be eyeballed in dev logs but require no client interaction.

## 6. Implementation order (sketch — plan-author refines)

1. **T1** — Add `session_log.go` with constants/struct/`processSessionLogs` skeleton + `Server.sessionLogs` field + tick-loop wiring. No callers yet.
2. **T2** — Add `Player.AddSessionLog` + `LoggerBridge.SubmitSessionLogs` interface method + `noopBridges.SubmitSessionLogs` no-op + `slogLoggerBridge.SubmitSessionLogs` impl + `recordingBridges.SubmitSessionLogs` capture. Foundation tests F1-F11 land here.
3. **T3** — Add `ActivePlayer.AddSessionLog` interface method + `handleSessionLog` + dispatch entry. Tests S1-S3 land here. Mock-Self recorder needed in `pkg/script/state_test.go` style.
4. **T4** — Activate NAI-71-D in `handler_opheld.go` (handleOpHeld + handleOpHeldT). Retire deviation comments. Tests A1-A5 land here.
5. **T5** — Activate NAI-73-D in `input_tracking.go` (`submitEvents` kick branch). Retire deviation comments. Tests A6-A7 land here.
6. **T6** — Close commit (memory `close_commit_memory_trailer.md`): summarise + memory: trailers + tracker-entry diffs for the deferred call sites. Net deviation tally 14 → 12.

## 7. Risk register (verified at spec-write per memory `risk_register_premise_grep.md`)

| Risk | Status |
|---|---|
| Player.session UUID always "headless" until login bridge ships | Verified at `modules/world/tick.go:146` (set on player bind) and `player.go:222-225` (default). Records flow correctly. Documented §4.1. |
| OPHELD ordering: addSessionLog before vs after script dispatch | Verified at TS `OpHeldHandler.ts:62-68` — addSessionLog is line 64, script dispatch is line 68. NAI-74 mirrors order. Documented §4.2. |
| handleOpHeldT lacks current objType lookup | Verified by reading `modules/world/handler_opheld.go:133-196` — confirmed. NAI-74 adds inline lookup with defensive guard. Documented §4.3. |
| `objType.IOp[op-1]` access bound | Verified — pre-existing gate at `handler_opheld.go:78` already validates `len(objType.IOp) >= op && objType.IOp[op-1] != ""` before reaching activation site. |
| `spellCom.ComName` exists | Verified at `pkg/objtype/componenttype.go:50`. |
| `Server.playerLoop` shape | Verified at `modules/world/server.go` — `[]*Player`. Iterable. |
| `OpSessionLog` opcode reserved with no handler | Verified at `pkg/script/opcode.go:198, 803-804`. No dispatch entry in `handlers.go`. |
| `requireActivePlayer` available | Verified at `pkg/script/handlers_player.go:35-40`. |
| Tick-phase ordering: `processCleanup` then `currentTick++` | Verified at `modules/world/tick.go:47-48`. Insert point clear. |

## 8. Out of scope

- Real `LoggerClient` WS/HTTP transport (TS `LoggerClient.ts`/`LoggerThread.ts` worker thread). Future NAI when an external logger backend is selected.
- Wealth-event subsystem (TS `World.addWealthEvent`, `wealthTransactions`, `wealthTransactionGroup`). Separate buffer + bridge channel; orthogonal.
- All deferred call sites listed in §3.7. They land with their host code.
- `Player.session` UUID assignment at login (NAI-72-D-LOGIN-SERVER-BRIDGE-MOD).
- Per-tick rate `PLAYER_SAVERATE` autosave (different rate, different responsibility).
