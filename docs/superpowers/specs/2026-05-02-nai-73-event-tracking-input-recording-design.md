# NAI-73 — EventTracking handler + InputTracking subsystem + LoggerBridge realisation

**Status:** Spec written 2026-05-02.
**Predecessor:** NAI-72 (HEAD `c33a2c2`). Net deviation tally entering: 18.
**Closes:** 2 deviations:
- `NAI-72-D-LOGGER-BRIDGE` — interface gains its second method and a real default impl (`slogLoggerBridge`) replaces the `noopBridges` binding for the `LoggerBridge` field.
- `NAI-72-D-INPUT-RECORDING-NOT-PORTED` — `Player.input` (`*InputTracking`) and `Player.submitInput` ship; the EVENT_TRACKING handler (op 81) wires up; the per-tick `OnCycle` scheduler runs.

**Opens:** 1 new deviation:
- `NAI-73-D-INPUT-NO-SESSION-LOG-KICK` — InputTracking's `submitEvents()` no-report kick branch ships `requestIdleLogout = true` only; the TS `addSessionLog(LoggerEventType.ENGINE, 'Client did not submit an input tracking report')` call is deferred until the session-log NAI lands. Net behavior preserved (player still logs out); only the structured-log audit entry is missing.

**Net deviation tally projection: 18 → 17 (−2 closes, +1 opens).**

**Retroactive close (not a numbered deviation):** `handler_reportabuse.go:23-28`'s TODO doc-comment ("MACROING/BUG_ABUSE submitInput=true branch intentionally omitted — input-recording subsystem not ported") is retired. The MACROING/BUG_ABUSE branch wires `offenderPlayer.submitInput = true` per TS `World.notifyPlayerReport` at `World.ts:2298-2304`.

**Tech stack:** Go 1.26+. TS source canonical path: `LostCityRS/Engine-TS`. Rust source N/A.

## 1. Background

The EVENT_TRACKING client opcode (81, RESTRICTED_EVENT, -2 prefix) is declared at the wire level (`pkg/io/protocol/game/client/prot.go:32`) and exercised by the `processIn` reader at `modules/world/player_test.go:138-167`, but `gameHandlers[81]` is unbound — the read path silently discards every packet. The opcode is the client's submission channel for an anti-cheat input-recording subsystem: when the server enables tracking on a per-player tick schedule, the client streams blobs of recorded input events back, and the server submits them to an external logger sink for offline analysis.

The TS subsystem comprises:

| TS file | Role |
|---|---|
| `network/game/client/handler/EventTrackingHandler.ts` (29 LOC) | Handler — len-gate, IsActive-gate, hasSeenReport=true, conditional record |
| `network/game/client/codec/EventTrackingDecoder.ts` (16 LOC) | Decoder — `gdata(bytes, 0, len)` |
| `network/game/client/model/EventTracking.ts` (13 LOC) | Model — wraps `bytes: Uint8Array` |
| `network/game/server/codec/EnableTrackingEncoder.ts` | Server packet (opcode 226, payload 0) |
| `network/game/server/codec/FinishTrackingEncoder.ts` | Server packet (opcode 133, payload 0) |
| `engine/entity/tracking/InputTracking.ts` (163 LOC) | Per-player state machine — schedules tracking on/off cycles, records blobs, submits at end |
| `engine/entity/Player.ts:305,422,1271-1273` | `input: InputTracking` field, constructor, `processInputTracking()` method |
| `engine/World.ts:646` | Per-tick call site (end of client-input phase loop) |
| `engine/World.ts:2298-2304,2314-2321` | `notifyPlayerReport` MACROING/BUG_ABUSE branch + `submitInputTracking` logger call |

The schedule cadence (TS `InputTracking.ts:10-14`):
- `TRACKING_RATE` = 200 ticks (~120s) — interval between tracking sessions.
- `TRACKING_TIME` = 150 ticks (~90s) — duration of each tracking window.
- `REMAINING_DATA_UPLOAD_LEEWAY` = 16 ticks (~10s) — grace period after window closes for the client to flush remaining data.
- Per-player jitter `offset(15)` = uniform `[-15, +15]` ticks added to first scheduled start.

Submission gate (`shouldSubmitTrackingDetails`, line 126-128): `player.submitInput || Environment.NODE_SUBMIT_INPUT`. Goscape already has `cfg.NodeSubmitInput` (`config.go:42`); the per-player flag is the new field.

Idle-logout gate (line 146-152): if no report was seen and `!Environment.NODE_DEBUG`, set `requestIdleLogout = true` and emit a session log. Goscape has `cfg.NodeDebug` (`config.go:41`) and `Player.requestIdleLogout` (`player.go:213`); only the session-log call is deferred.

## 2. Current state at HEAD (`c33a2c2`)

### 2.1 Dependencies (verified present)

| Dependency | goscape location | Notes |
|---|---|---|
| `LoggerBridge` interface (one method) | `modules/world/bridges.go:28-32` | NAI-72; gains second method in NAI-73 T2 |
| `LoggerBridge.NotifyPlayerReport` consumer | `handler_reportabuse.go:61` | Wired; default `noopBridges{}` is the impl |
| `noopBridges` default | `bridges.go:36-45` | NAI-73 keeps the noop binding for `friendsBridge` and `loginBridgeMod`; replaces it for `loggerBridge` specifically |
| `Server.loggerBridge LoggerBridge` field | `server.go:126` | Already an interface field; default-initialised at `server.go:164` |
| `cfg.NodeSubmitInput bool` | `config.go:42` | TS analog of `Environment.NODE_SUBMIT_INPUT` |
| `cfg.NodeDebug bool` | `config.go:41` | TS analog of `Environment.NODE_DEBUG` |
| `Player.requestIdleLogout bool` | `player.go:213` | Honored by `processLogouts` at `tick.go:155-163` |
| `Player.client.server *Server` | confirmed by handler usage | Provides bridge access from InputTracking |
| `Player.MessageGame` etc. | existing | Pattern reference |
| `EVENT_TRACKING` opcode size table | `prot.go:32` (`set(81, "EVENT_TRACKING", -2, r)`) | Declared, accepted, currently no-op |
| Op-81 wire-test | `player_test.go:138-238` | Reader path verified; handler dispatch is the gap |
| `processIn` dispatch table | `handlers_game.go:15` (`gameHandlers [256]func(*Player, []byte) error`) | Add 1 entry in `init()` |
| `slog.Logger` | used throughout — `server.go:48,136`, `app.go:23,37` | Goscape's logger of choice; `slogLoggerBridge` wraps a child logger |

### 2.2 Goscape conventions (confirmed via grep)

- **`*slog.Logger` is goscape's logger** — not `zerolog`. The default `LoggerBridge` impl is `slogLoggerBridge` (lowercase exported only as needed; constructor `NewSlogLoggerBridge`).
- **Bridge fields on Server are interface-typed and default to `noopBridges{}`** (`server.go:162-164`). NAI-73 changes only the `loggerBridge` line: `s.loggerBridge = NewSlogLoggerBridge(s.log)`.
- **Per-cycle player state lives on `*Player` directly or as a sub-struct held by pointer**. NAI-73 follows the latter: `Player.input *InputTracking` initialised in the same `processLogins` block that today sets `p.invs`, `p.varps`, etc. (`tick.go:96-110`).
- **Per-tick player-phase methods are on `*Player` and called from a `*Server` loop in `tick.go`**, e.g. `processIn` (`player.go:683`, called from `processClientsIn` at `tick.go:64-76`). NAI-73 adds a single line at the **end** of `processIn` (mirrors TS World.ts:646 placement: after the packet-read loop, in the same per-player iteration of the client-input phase). No new top-level pass.
- **Server-prot opcodes go in `pkg/io/protocol/game/server/prot.go`** as `Op{opcode, payloadSize}` constants. Two-line append.
- **Encoder helpers are methods on `*Player` calling `p.writeOut(opcode, payload)`** (`data_map.go`, `inv_update.go`, `interaction.go`, etc.). For 0-payload signals, the helper is a one-liner: `p.writeOut(gameserver.OpEnableTracking, nil)`.
- **Test conventions** — handler tests use `newTestPlayer(t)` patterns from existing files (e.g. `handler_reportabuse_test.go`). Per-tick state-machine tests are table-driven (e.g. `npc_script_test.go`).
- **Config-flag pattern** — `f.IntVar(&c.NodeLimitBytesPerTrackingSession, ...)` mirrors the existing `NodeSubmitInput` registration at `config.go:73`.

### 2.3 Verified-absent claims (premise grep evidence)

Per `risk_register_premise_grep.md` and `controller_preflight.md`:

```
$ rg -n "InputTracking|p\.input\b|input\.OnCycle|input\.Record" pkg/ modules/
(no hits — confirms entity is absent)

$ rg -n "submitInput\b|SubmitInput\b" pkg/ modules/
modules/world/config.go:42:	NodeSubmitInput                  bool          `yaml:"node_submit_input"`
modules/world/config.go:73:	f.BoolVar(&c.NodeSubmitInput, "world.node-submit-input", false, "...")
(only the global config knob — no per-player Player.submitInput field)

$ rg -n "OpEnableTracking|OpFinishTracking|ENABLE_TRACKING|FINISH_TRACKING" pkg/ modules/
(no hits — server-prot opcodes 226/133 are absent)

$ rg -n "gameHandlers\[81\]" modules/
(no hits — op-81 dispatch is unbound)

$ rg -n "handleEventTracking|HandleEventTracking" modules/
(no hits — handler is absent)

$ rg -n "Player\.session\b|p\.session\b|sessionUUID" modules/
(no hits — Player.session field is absent; LOGIN-SERVER-BRIDGE-MOD owns its eventual init)

$ rg -n "SubmitInputTracking|submitInputTracking" pkg/ modules/
(no hits — second LoggerBridge method is absent)

$ rg -n "NodeLimitBytesPerTrackingSession|node_limit_bytes_per_tracking_session" modules/
(no hits — config knob is absent)

$ rg -n "slogLoggerBridge|NewSlogLoggerBridge" modules/
(no hits — default impl is absent)
```

### 2.4 Out-of-scope dependencies (tracked as deviations)

- **Session-log subsystem.** TS `InputTracking.submitEvents()` calls `this.player.addSessionLog(LoggerEventType.ENGINE, 'Client did not submit an input tracking report')` (line 150) on the no-report kick path. Goscape has no `addSessionLog`. Deferred via new `NAI-73-D-INPUT-NO-SESSION-LOG-KICK`. The kick itself fires (`requestIdleLogout = true`); only the audit-log entry is missing. This is the same subsystem deferred by `NAI-71-D-OPHELD-NO-SESSION-LOG`; a future session-log NAI sweeps both.
- **`Player.session` UUID.** TS `Player.session: string = 'headless'` is overridden by login-server bridge integration (`LOGIN-SERVER-BRIDGE-MOD`). Goscape's `Player.session` defaults to `"headless"` in NAI-73; LOGIN-SERVER-BRIDGE-MOD's eventual closure sets a real UUID. Already-tracked, no new deviation.
- **`Environment.NODE_LIMIT_BYTES_PER_TRACKING_SESSION` env var.** TS reads from `process.env.NODE_MAX_BYTES_PER_TRACKING_SESSION` with default `50_000`. Goscape ports as a config knob (`world.node-limit-bytes-per-tracking-session`) with default `50000`. Not a deviation — exact-equivalent port.

## 3. Scope

### 3.1 Out-of-scope (explicitly deferred)

| Item | Why | Defer-tag / closure |
|---|---|---|
| `addSessionLog` infrastructure | Session-log subsystem is its own NAI candidate (#4 from NAI-73 brainstorm) | `NAI-73-D-INPUT-NO-SESSION-LOG-KICK`; future session-log NAI |
| Real `Player.session` UUID generation | Owned by login-server bridge integration | `NAI-72-D-LOGIN-SERVER-BRIDGE-MOD` (no-op for NAI-73; default `"headless"`) |
| `varp setVarPlayer submitInput=true` branch (TS World.ts:2033) | Varp-system wiring is a separate subsystem | Tracked as N-note `NAI-73-N-SUBMITINPUT-VARP-SETTER`; minor, no behavioral pin |
| Friends-server / login-bridge-mod real impls | Not blocked by NAI-73; out of scope | `NAI-72-D-FRIENDS-SERVER-BRIDGE`, `NAI-72-D-LOGIN-SERVER-BRIDGE-MOD` |
| MessagePrivate handler (op 148) | Needs WordPack port | Future "MessagePrivate + WordPack" sub-spec |

### 3.2 In-scope

1. Extend `LoggerBridge` interface with `SubmitInputTracking(player *Player, blob []byte)`.
2. Add `slogLoggerBridge` real default impl wrapping `*slog.Logger`; replace `noopBridges{}` with `NewSlogLoggerBridge(s.log)` for the `loggerBridge` field at `server.go:164`. The other two bridges (`friendsBridge`, `loginBridgeMod`) keep `noopBridges{}` — their deviations are unchanged.
3. Update `noopBridges.SubmitInputTracking` for tests / interface satisfaction (still a no-op).
4. Add `Player.input *InputTracking` field; initialise in `processLogins` after the existing per-player state init block.
5. Add `Player.submitInput bool` field; default false.
6. Add `Player.session string` field; default `"headless"` (TS-faithful).
7. Add `cfg.NodeLimitBytesPerTrackingSession int` config knob; default `50000`; flag `world.node-limit-bytes-per-tracking-session`.
8. Add server-prot opcodes `OpEnableTracking = Op{226, 0}` and `OpFinishTracking = Op{133, 0}` to `pkg/io/protocol/game/server/prot.go` plus `prot_test.go` entries.
9. Add `(*Player).WriteEnableTracking()` and `(*Player).WriteFinishTracking()` one-liner helpers (in new file `modules/world/input_tracking_packets.go` or folded into `input_tracking.go` — plan-author chooses).
10. Add `InputTracking` entity (`modules/world/input_tracking.go`) — full state-machine port of `InputTracking.ts:1-163` with TDD pinning every TS branch.
11. Add EVENT_TRACKING handler (`modules/world/handler_event_tracking.go`) — line-by-line port of `EventTrackingHandler.ts:7-28`, registered in `gameHandlers[81]` via `init()`.
12. Add `(*Player).processInputTracking(currentTick int)` method calling `p.input.OnCycle(currentTick)`; invoke as the last line of `processIn` (`player.go:683-730`).
13. Wire `s.loggerBridge.SubmitInputTracking(...)` from `InputTracking.submitEvents()` (via `p.client.server.loggerBridge`).
14. Retroactive REPORT_ABUSE close: `handler_reportabuse.go` — wire MACROING/BUG_ABUSE → `offenderPlayer.submitInput = true`. Remove the `NAI-72-D-INPUT-RECORDING-NOT-PORTED` doc-comment block at lines 23-25 and the "logger bridges all stubbed" wording at lines 27-28; rewrite as TS-faithful narrative reflecting the now-real LoggerBridge and the MACROING/BUG_ABUSE branch.
15. Retire deviation tags: `NAI-72-D-LOGGER-BRIDGE` and `NAI-72-D-INPUT-RECORDING-NOT-PORTED` from all docstring sites and the `bridges.go:18-19,25-27` interface comments. New deviation tag `NAI-73-D-INPUT-NO-SESSION-LOG-KICK` opens at `input_tracking.go` (the `submitEvents` kick branch).

## 4. Architecture

### 4.1 Components

```
┌─────────────────────────────────────────────────────────────┐
│ Server (modules/world/server.go)                            │
│   loggerBridge LoggerBridge ─── NewSlogLoggerBridge(s.log)  │
│   friendsBridge FriendsBridge ─── noopBridges{}             │
│   loginBridgeMod LoginBridgeMod ─── noopBridges{}           │
└──────────┬──────────────────────────────────────────────────┘
           │
           │ (per-tick)
           ▼
┌─────────────────────────────────────────────────────────────┐
│ Player (modules/world/player.go)                            │
│   input *InputTracking ──────┐                              │
│   submitInput bool           │                              │
│   session string ("headless")│                              │
│   requestIdleLogout bool ────┼──── (existing)               │
└──────────────────────────────┼──────────────────────────────┘
                               │
                               │
┌──────────────────────────────▼──────────────────────────────┐
│ InputTracking (modules/world/input_tracking.go)             │
│   player *Player (back-pointer)                             │
│   rng *rand.Rand                                            │
│   enabled bool                                              │
│   hasSeenReport bool                                        │
│   waitingForRemainingData bool                              │
│   startTrackingAt int                                       │
│   endTrackingAt int                                         │
│   recordedBlobs [][]byte                                    │
│   recordedBlobsSizeTotal int                                │
│                                                             │
│   OnCycle(currentTick int)                                  │
│   IsActive(currentTick int) bool                            │
│   ShouldSubmitTrackingDetails() bool                        │
│   Record(rawData []byte)                                    │
│   enable() / disable() / submitEvents()                     │
└─────────────────────────────────────────────────────────────┘
                               │
                               │ submitEvents calls
                               ▼
┌─────────────────────────────────────────────────────────────┐
│ LoggerBridge (modules/world/bridges.go)                     │
│   NotifyPlayerReport(player, offender, reason)  ◄ NAI-72   │
│   SubmitInputTracking(player, blob []byte)      ◄ NAI-73   │
│                                                             │
│ slogLoggerBridge default impl (modules/world/logger_bridge.go) │
│   logger *slog.Logger                                       │
│   emits structured records: type=report|input_track,        │
│     session, coord/offender/reason/blob_len                 │
└─────────────────────────────────────────────────────────────┘
```

### 4.2 Data flow

**Scheduling/recording (per tick, per player):**

```
processClientsIn (tick.go:64)
  └── processIn (player.go:683)
        ├── packet read loop (existing)
        │     └── on op 81 → handleEventTracking
        │           ├── len ∈ (0, 500] gate (else return false)
        │           ├── input.IsActive(currentTick) gate (else return false)
        │           ├── input.hasSeenReport = true
        │           ├── if !input.shouldSubmitTrackingDetails() → return true (no record, no cap check)
        │           ├── recordedBlobsSizeTotal cap gate (else return false)
        │           └── input.Record(bytes); return true
        └── processInputTracking(currentTick)  ◄ NEW (last line of processIn)
              └── input.OnCycle(currentTick)
                    ├── if waitingForRemainingData and (endTrackingAt + LEEWAY < currentTick) → submitEvents
                    ├── if shouldStartTracking and !enabled → enable() → WriteEnableTracking
                    └── if shouldEndTracking and enabled → disable() → WriteFinishTracking + waitingForRemainingData=true
```

**`submitEvents()` branches** (TS `InputTracking.ts:140-158`):

| `hasSeenReport` | `shouldSubmit` | `cfg.NodeDebug` | Action |
|:-:|:-:|:-:|:--|
| true | true | × | `loggerBridge.SubmitInputTracking(player, recordedBlobs[0])` |
| true | false | × | (no submission) |
| false | × | true | (no kick — debug mode) |
| false | × | false | `requestIdleLogout = true` |

State always reset post-branch: `waitingForRemainingData = false`, `recordedBlobs = nil`, `recordedBlobsSizeTotal = 0`, `hasSeenReport = false`.

**REPORT_ABUSE retroactive flow:**

```
handleReportAbuse (handler_reportabuse.go:29)
  ├── ... existing branches ...
  ├── moderator-mute branch (existing)
  ├── if reason ∈ {ReportAbuseMacroing, ReportAbuseBugAbuse}:  ◄ NEW
  │     offenderPlayer := s.findPlayerByUsername(util.FromBase37(offender))
  │     if offenderPlayer != nil:
  │         offenderPlayer.submitInput = true
  ├── s.loggerBridge.NotifyPlayerReport(...) (existing)
  ├── p.MessageGame(...) (existing)
  └── p.reportAbuseProtect = true (existing)
```

(The `findPlayerByUsername` lookup is a goscape gap — TS uses `World.getPlayerByUsername`; plan-author greps `findPlayerByUsername`/`getPlayerByUsername` in goscape and uses the existing helper. If absent, plan-author chooses a small in-scope addition.)

### 4.3 Component boundaries

**`InputTracking`** — pure-state struct, depends only on `*Player` (back-pointer for `submitInput` read, `requestIdleLogout` write, `WriteEnableTracking/WriteFinishTracking`, `client.server.loggerBridge` access, `client.server.cfg.NodeDebug`/`NodeSubmitInput`/`NodeLimitBytesPerTrackingSession` reads). Does not import Server or World types directly. Random source is per-player `*rand.Rand` allocated in `processLogins` from `math/rand/v2.NewPCG` seeded by something stable but non-deterministic per session (plan-author chooses; existing player-level RNG patterns are the reference).

**`LoggerBridge` (interface)** — gains second method:
```go
type LoggerBridge interface {
    NotifyPlayerReport(player *Player, offender, reason string)
    SubmitInputTracking(player *Player, blob []byte)
}
```
**`slogLoggerBridge`** (new, `modules/world/logger_bridge.go`):
```go
type slogLoggerBridge struct {
    log *slog.Logger
}

func NewSlogLoggerBridge(parent *slog.Logger) *slogLoggerBridge {
    return &slogLoggerBridge{log: parent.With("component", "logger_bridge")}
}

func (b *slogLoggerBridge) NotifyPlayerReport(p *Player, offender, reason string) {
    b.log.Info("player_report",
        "type", "report",
        "session", p.session,
        "coord", packCoord(p),
        "offender", offender,
        "reason", reason,
    )
}

func (b *slogLoggerBridge) SubmitInputTracking(p *Player, blob []byte) {
    b.log.Info("input_track",
        "type", "input_track",
        "session", p.session,
        "blob_len", len(blob),
        "blob_b64", base64.StdEncoding.EncodeToString(blob),
    )
}
```

`packCoord(p)` mirrors TS `CoordGrid.packCoord(level, x, z)`. If goscape lacks this helper, plan-author either greps an existing one (likely in `pkg/grid/` or `pkg/zone/`) or inlines the bit-pack.

**`noopBridges`** — gains `SubmitInputTracking(*Player, []byte) {}` for `friendsBridge`/`loginBridgeMod` interface satisfaction (they all share `noopBridges` today). Tests can still bind `noopBridges{}` to `Server.loggerBridge` if they want a recording-free baseline; production swaps in `slogLoggerBridge`.

**`mockLoggerBridge`** (test fixture) — recording impl capturing call args for assertion.

## 5. Testing strategy

### 5.1 InputTracking state machine (`input_tracking_test.go`)

Table-driven over OnCycle's branch matrix. Each case sets `currentTick`, prior `enabled`, prior `waitingForRemainingData`, prior `startTrackingAt`, prior `endTrackingAt`; asserts post-state and which packet (if any) was written via a captured `*mockClient`.

Branches to pin:
- enable on tick boundary (currentTick == startTrackingAt, !enabled) → enabled=true, endTrackingAt=startTrackingAt+TRACKING_TIME, EnableTracking written, startTrackingAt set to currentTick.
- disable on tick boundary (currentTick == endTrackingAt, enabled) → enabled=false, waitingForRemainingData=true, FinishTracking written, startTrackingAt rescheduled (next interval).
- waiting + grace expired → submitEvents fires; waitingForRemainingData=false; recordedBlobs cleared.
- waiting + grace not yet expired → no-op.
- pre-window (currentTick < startTrackingAt) → no-op.
- mid-window (currentTick > startTrackingAt && < endTrackingAt && enabled) → no-op.

`submitEvents` branches (5 cases per matrix above):
- (hasSeenReport=true, shouldSubmit=true) → `loggerBridge.SubmitInputTracking(player, blobs[0])` called once with first blob; remaining blobs ignored (TS quirk preserved — pin via test on a 3-blob fixture).
- (hasSeenReport=true, shouldSubmit=false) → no bridge call.
- (hasSeenReport=false, NodeDebug=false) → `requestIdleLogout=true`; no bridge call.
- (hasSeenReport=false, NodeDebug=true) → `requestIdleLogout` unchanged; no bridge call.
- All branches: state reset (recordedBlobs nil, recordedBlobsSizeTotal=0, hasSeenReport=false, waitingForRemainingData=false).

`shouldSubmitTrackingDetails`: (player.submitInput=false, cfg.NodeSubmitInput=false) → false; (true, false) → true; (false, true) → true; (true, true) → true.

`Record`: appends blob, increments size; multi-blob accumulation.

`IsActive`: `(currentTick >= startTrackingAt && <= endTrackingAt) || waitingForRemainingData`. Pin all 4 corners.

### 5.2 EVENT_TRACKING handler (`handler_event_tracking_test.go`)

5-gate matrix (mirrors TS `EventTrackingHandler.ts:7-28`):
- len=0 → returns false (no record, no hasSeenReport).
- len=501 → returns false (no record).
- len=1, !IsActive → returns false (no hasSeenReport).
- len=1, IsActive, !shouldSubmit → returns true; hasSeenReport=true; **no Record call** (TS line 18-20 short-circuits return-true).
- len=1, IsActive, shouldSubmit, recordedBlobsSizeTotal already > limit → returns false (no record).
- len=1, IsActive, shouldSubmit, under cap → Record called; hasSeenReport=true; returns true.

Test fixture builds a Player with `input` initialised and a configurable mockClient capturing writes.

### 5.3 LoggerBridge (`logger_bridge_test.go`)

For each method, build `slog.New(slog.NewTextHandler(buf, ...))`, call the bridge method, parse the buf, assert keys/values:
- `NotifyPlayerReport`: `type=report`, `session=...`, `offender=...`, `reason=...`, `coord=...`.
- `SubmitInputTracking`: `type=input_track`, `session=...`, `blob_len=...`, `blob_b64=...` (base64 encoding round-trips).

### 5.4 REPORT_ABUSE retroactive (`handler_reportabuse_test.go` extension)

Add 3 cases:
- reason=Macroing(6), offender online → `offenderPlayer.submitInput = true`.
- reason=BugAbuse(3), offender online → `offenderPlayer.submitInput = true`.
- reason=OffensiveLanguage(0), offender online → `offenderPlayer.submitInput` unchanged (false).

Plus existing report flow assertions (mockLoggerBridge.NotifyPlayerReport called) preserved.

### 5.5 Config (`config_test.go` extension)

`NodeLimitBytesPerTrackingSession` default = 50000; flag `world.node-limit-bytes-per-tracking-session` overrides.

### 5.6 Server-prot opcode wiring (`prot_test.go` extension — server side)

Two new entries: `{OpEnableTracking, 226, 0}`, `{OpFinishTracking, 133, 0}`.

### 5.7 Per-tick wiring (`player_test.go` or `input_tracking_test.go`)

Invoke `processIn(currentTick)` on a Player with `input.startTrackingAt = currentTick` and `!input.enabled`; assert that after the call, `input.enabled = true` and an EnableTracking packet is buffered in the client out-stream.

## 6. Deviation ledger

**Closing in NAI-73:**
- `NAI-72-D-LOGGER-BRIDGE` — interface gains second method; real default impl ships and is bound.
- `NAI-72-D-INPUT-RECORDING-NOT-PORTED` — full subsystem ports.

**Retroactive doc-comment retirements (not numbered tags):**
- `handler_reportabuse.go:23-25` TODO block — MACROING/BUG_ABUSE submitInput=true wired.
- `handler_reportabuse.go:27-28` "logger bridges all stubbed" wording — narrowed to friends/loginBridgeMod only.
- `bridges.go:25-27` LoggerBridge interface "real impl deferred" comment — replaced with "ships with `slogLoggerBridge` default impl".

**Opening in NAI-73:**
- `NAI-73-D-INPUT-NO-SESSION-LOG-KICK` — InputTracking.submitEvents kick branch ships `requestIdleLogout=true` only; `addSessionLog(ENGINE, ...)` deferred to future session-log NAI.

**Tracker N-notes (not numbered deviations):**
- `NAI-73-N-SUBMITINPUT-VARP-SETTER` — TS World.ts:2033 `setVarPlayer submitInput=true` branch is varp-system wiring (a different code path than REPORT_ABUSE; allows mods to flip the flag via varp manipulation without filing a report). Out of scope for NAI-73; tracked for future varp-system NAI when `setVarPlayer` ports.

**Net deviation tally: 18 → 17.**

## 7. Task breakdown

7 tasks, TDD per task. Plan-author elaborates each in NAI-73 plan with grep evidence and code blocks per `controller_preflight.md` and `plan_runnable_test_fixtures.md`.

| # | Task | Touches | Depends on |
|:-:|---|---|---|
| T1 | Foundation: Player.input pointer field, Player.submitInput, Player.session, NodeLimitBytesPerTrackingSession config knob, server-prot opcodes 226+133 with `prot_test.go` entries | `player.go`, `config.go`, `pkg/io/protocol/game/server/prot.go`, `pkg/io/protocol/game/server/prot_test.go` | — |
| T2 | LoggerBridge extension + real default impl: extend interface with `SubmitInputTracking`; implement `slogLoggerBridge`; bind in `NewServer`; update `noopBridges` to satisfy extended interface | `bridges.go`, new `logger_bridge.go`, `logger_bridge_test.go`, `server.go` | T1 (for `Player.session` reference in tests) |
| T3 | InputTracking entity: full state-machine port with TDD pinning every TS branch; `WriteEnableTracking`/`WriteFinishTracking` helpers | new `input_tracking.go`, new `input_tracking_test.go`, possibly new `input_tracking_packets.go` | T1, T2 |
| T4 | EVENT_TRACKING handler (op 81): wire into `gameHandlers[81]` via `init()`; line-by-line TDD against `EventTrackingHandler.ts` | new `handler_event_tracking.go`, new `handler_event_tracking_test.go`, `handlers_game.go` | T3 |
| T5 | Per-tick wiring: `processInputTracking` on Player; call from end of `processIn` | `player.go`, `input_tracking_test.go` (extension) | T3 |
| T6 | InputTracking init at login: `processLogins` allocates `p.input = NewInputTracking(p, currentTick, rng)` | `tick.go` | T3 |
| T7 | REPORT_ABUSE retroactive close: wire MACROING/BUG_ABUSE submitInput=true; rewrite doc-comment narrative; retire deviation tags `NAI-72-D-LOGGER-BRIDGE` and `NAI-72-D-INPUT-RECORDING-NOT-PORTED` from all docstring sites | `handler_reportabuse.go`, `handler_reportabuse_test.go`, `bridges.go` (interface comment) | T2 |

T1+T2 are independent of T3+. T3 strictly precedes T4/T5/T6. T7 only depends on T2.

## 8. Risk register

| Risk | Likelihood | Mitigation |
|---|---|---|
| `findPlayerByUsername`/`getPlayerByUsername` helper missing in goscape — needed by REPORT_ABUSE retroactive submitInput=true wiring | Medium | T7 plan-author greps; if absent, adds a minimal helper in scope; otherwise picks the existing one |
| `packCoord(p)` helper missing — needed by `slogLoggerBridge.NotifyPlayerReport` | Low | T2 plan-author greps `pkg/grid/`, `pkg/zone/`; if absent, inlines the bit-pack |
| TS `submitEvents` always submits `recordedBlobs[0]` only (TS quirk — slice-index-0, ignoring blobs[1+]). Easy to "fix" to TS-divergent submit-all loop | High | §5.1 explicitly pins single-blob[0] submission with a 3-blob fixture; deviation comment in `submitEvents()` reads "TS-faithful: submits blob 0 only, even if multiple blobs were recorded" |
| Per-player RNG init pattern unclear in goscape — InputTracking needs jitter `[-15, 15]` | Low | T6 plan-author greps existing `*rand.Rand` allocation in `tick.go` `processLogins`; if a shared `s.rng` exists, uses it; otherwise allocates one per player at init time |
| `processInputTracking` placement: end of `processIn` (TS World.ts:646 puts it inside the same per-player loop iteration AFTER packet reads, BEFORE `logPublicChat`) | Low | §3.2 #12 specifies "last line of processIn"; matches TS placement to within one peer call (logPublicChat doesn't exist in goscape's `processIn` yet, so end-of-method is the unambiguous mirror) |
| Bound `loggerBridge` swap from `noopBridges` to `slogLoggerBridge` could surface latent test breakage if any existing test asserts no log output during `handleReportAbuse` | Medium | T2 runs full test suite after the swap; if breakage, isolate via `mockLoggerBridge{}` binding in affected tests |
| `cfg.NodeDebug` default is `true` (config.go:76) — InputTracking kick branch never fires in dev/test config | Low | Tests explicitly set `cfg.NodeDebug = false` for the kick-path assertions; doc-comment notes that production runs override the default |

## 9. Premise re-checks before plan dispatch

Plan-author MUST re-grep at HEAD (not at NAI-72 close) before T1 dispatch:

```
$ rg -n "Player\.session|p\.session\b|sessionUUID" modules/    # confirm absence
$ rg -n "submitInput\b" modules/                               # confirm only config knob present
$ rg -n "InputTracking|p\.input\b" modules/                    # confirm subsystem absent
$ rg -n "OpEnableTracking|OpFinishTracking" pkg/               # confirm opcodes absent
$ rg -n "gameHandlers\[81\]" modules/                          # confirm handler unbound
$ rg -n "loggerBridge\b" modules/                              # confirm field exists at server.go:126
$ rg -n "findPlayerByUsername|getPlayerByUsername" modules/    # T7: confirm helper status
$ rg -n "packCoord\b|PackCoord\b" pkg/ modules/                # T2: confirm helper status
$ rg -n "math/rand/v2\b|rand\.New" modules/world/              # T6: confirm RNG conventions
```

Per `controller_preflight.md`: any premise that surfaces wrong gets fixed in the spec via inline erratum (matches NAI-72's `9854ac5` STAFFMODLEVEL retraction pattern) before plan-author dispatch.

## 10. Out-of-scope follow-ups

Already noted above; consolidated for memory grep:

- **NAI-73-D-INPUT-NO-SESSION-LOG-KICK** → closes in future session-log NAI (candidate #4 from NAI-73 brainstorm). Sweeps OPHELD / REPORT_ABUSE addSessionLog gaps simultaneously.
- **NAI-73-N-SUBMITINPUT-VARP-SETTER** → closes in future varp-system NAI when `setVarPlayer` ports.
- LoggerBridge `session_log` channel — deferred; lands with the session-log NAI as a third method.

---

**Closes memory:** NAI-72-D-LOGGER-BRIDGE, NAI-72-D-INPUT-RECORDING-NOT-PORTED.
**Opens memory:** NAI-73-D-INPUT-NO-SESSION-LOG-KICK.
