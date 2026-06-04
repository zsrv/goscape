# Friends-server bridge — slice 5b design: RELAY_* world-state action handlers

**Date:** 2026-05-20
**Slice:** 5b of 7 (friends-server bridge arc; slice 5 decomposed into 5a/5b)
**Predecessor:** slice 5a (close commit `d3946e9b`, opened `NAI-S5A-D-DISPATCHER-NO-ACTION` + 3 other permanent tags; see `[[friends-server-slice5a-close]]`)
**Closes:** `NAI-S5A-D-DISPATCHER-NO-ACTION` (piecewise — one bullet per wired opcode)
**Opens:** `NAI-S5B-D-CLEARLOGOUTS-NO-GOSCAPE-QUEUE`, `NAI-S5B-D-NO-RUNESCRIPT-RUNTIME`, optionally `NAI-S5B-D-NO-INGAME-EMIT-BROADCAST` and/or `NAI-S5B-D-NO-INGAME-EMIT-TRACK` (gated on plan-time investigation)

## 1. Forward map (what ships in this slice)

| File | New / changed | Notes |
|---|---|---|
| `modules/world/bridges.go` | **changed** | New `WorldStateOps` interface (6 methods, possibly 7-8 if BROADCAST/TRACK flip wireable at plan time — see §6.3); inline doc-comment retires per-opcode bullets of `NAI-S5A-D-DISPATCHER-NO-ACTION` |
| `modules/world/world_events_dispatcher.go` | **new** | `actionWorldEventsDispatcher` impl — wraps inner `slogWorldEventsDispatcher`, then calls into `WorldStateOps`. Composition (Option B from resume Q1). |
| `modules/world/world_events_dispatcher_test.go` | **changed** (extends slice-5a file) | Per-opcode unit tests on a recording `WorldStateOps` fake: assert dispatch → method called with right args; assert slog inner still fires |
| `modules/world/world_state_ops.go` | **new** | `*Server` implements `WorldStateOps`. Six methods (per §3); fused lookup inline per §5. Extracted to its own file to keep `server.go` focused. |
| `modules/world/world_state_ops_test.go` | **new** | `*Server`-backed integration tests: SHUTDOWN advances `shutdownTick`; CLEARLOGINS empties `newPlayers`; KICK flips `loggingOut`; MUTE sets `mutedUntil`; RELOAD enqueues on `rebuildReq`. |
| `modules/world/server.go` | **changed** | `NewServer` wires `actionWorldEventsDispatcher` in place of `slogWorldEventsDispatcher` at the single existing call site (~line 284). No new fields beyond what's needed to expose `WorldStateOps`. |
| `modules/world/friends_smoke_test.go` | **changed** | Extend `TestFriendsClient_E2E_RelayWorldEventsRoundTrip` (or new sibling) — boot real friends-server, world subscribes, issue `RelayShutdown`, assert world's `shutdownTick` advanced after dispatch. One additional opcode round-trip (RelayReload or RelayClearLogins). |

LOC estimate: ~600-700 added, ~5 deleted. No proto changes. No DB changes. World-side only.

## 2. Composition vs. replacement (Q1)

**Decision: composition.** New `actionWorldEventsDispatcher` wraps an inner `slogWorldEventsDispatcher` and applies the action after logging. The slog impl is unchanged and remains the test-helper baseline for tests that want pure observability without world-state side effects.

```go
type actionWorldEventsDispatcher struct {
    inner WorldEventsDispatcher // slog or recording
    ops   WorldStateOps
    log   *slog.Logger          // for action-path warnings (lookup misses, etc.)
}

func (d *actionWorldEventsDispatcher) OnMute(username37 uint64, mutedUntilMs int64) {
    d.inner.OnMute(username37, mutedUntilMs)
    d.ops.SetPlayerMute(username37, mutedUntilMs)
}
// ... eight more
```

Pros: logging stays uniform across all events; tests can pin slog output independently of action assertions; the action layer doesn't have to duplicate Info-level lines.

Rejected alternatives:
- **Replace** `slogWorldEventsDispatcher` entirely with a single new impl that does both — loses the cleanly testable slog-only baseline, makes slice-5a's `recordingWorldEventsDispatcher` test helper less natural to reuse.
- **Extend in place** — conflates concerns; harder to add a no-action test impl later. **Reject.**

## 3. `WorldStateOps` interface (Q2)

```go
// WorldStateOps is the world-side action surface invoked by
// actionWorldEventsDispatcher on inbound RELAY_* events. *Server
// implements it (world_state_ops.go). Tests bind a recording fake.
//
// Methods correspond 1:1 to wired RELAY_* opcodes. Deferred opcodes
// (BROADCAST, TRACK, QUEUESCRIPT — see §6) are NOT on this interface;
// their dispatcher methods stay slog-warn until their gates retire.
type WorldStateOps interface {
    SetPlayerMute(username37 uint64, mutedUntilMs int64)
    KickPlayer(username37 uint64)
    Shutdown(durationTicks int32)
    Reload()
    ClearLogins()
    ClearLogouts() // tagged no-op — see NAI-S5B-D-CLEARLOGOUTS-NO-GOSCAPE-QUEUE
}
```

Six methods today. If TRACK/BROADCAST flip from deferred to wireable at plan time (§6), they extend this surface — small interface delta, no architecture change.

The interface mirrors slice-2 `LoginBridgeMod` + slice-4a `FriendsDispatcher` shape. The dispatcher consumes the interface; `*Server` is the production binding; a recording fake (`recordingWorldStateOps` in `world_events_dispatcher_test.go`) is the test binding.

## 4. KICK threading model (Q3)

A RELAY_KICK arrives on the dispatcher goroutine (per-world subscriber Recv loop). The action target — flipping `Player.loggingOut = true` — is also done by the `::kick` cheat handler from the tick goroutine. The existing pattern (NAI-186-D1, pinned by `handler_cheats_supermod_test.go:401`) is "set loggingOut, defer teardown to processLogouts in the tick loop."

**Decision:** the dispatcher goroutine acquires `playersMu.Lock()`, sets `p.loggingOut = true`, releases. The tick loop sees it on its next iteration and calls `removePlayerOnTick`. Single-writer semantics on player state are preserved because `loggingOut` is the handoff signal — any goroutine may flip it under the mutex, only the tick goroutine reads it for teardown.

This is safer than queueing the kick into a new channel: `loggingOut` is already the canonical "this player wants out" flag, used by `::kick`, by mute-driven logout-on-mute paths, and by removePlayerOnDisconnect. A new queue would duplicate state without adding safety.

## 5. Player lookup (Q4)

Add `LookupPlayerByUsername37(uint64) *Player` adjacent to existing `LookupPlayerByUsername(string)` at `server.go:1106`. Implementation: iterate `s.playerLoop` once, compare each active player's username via `jstring.ToBase37(p.username) == username37` (matches the existing string-based lookup's iteration pattern; one base37 encode per slot is cheap relative to the network event arrival rate).

**Thread safety divergence from `LookupPlayerByUsername`.** The existing string-based lookup is documented "intended to be called from the tick goroutine (playerLoop is unguarded there)." The dispatcher path runs on the per-world subscriber goroutine — not the tick. `LookupPlayerByUsername37` MUST acquire `playersMu.RLock()` for the duration of the iteration. The tick goroutine is the only writer to `playerLoop` and never reenters the dispatcher path, so RLock is sufficient.

`WorldStateOps` methods that follow the lookup (SetPlayerMute, KickPlayer) also need the write lock on `playersMu` to mutate the located player's fields — so the lookup helper returns `*Player` under RLock, the caller upgrades by releasing the RLock and reacquiring as `Lock()` for the mutation. **Or** (simpler): the lookup is fused into each `WorldStateOps` method — `SetPlayerMute` acquires `playersMu.Lock()` once, iterates inline, mutates, unlocks. No separate helper needed for the dispatcher path. Existing `LookupPlayerByUsername(string)` stays as the tick-side helper, untouched. **Recommend the fused variant** — cleaner lock discipline.

Lookup-miss is a normal occurrence (the friends-server fanned the relay to every world; the target may live on a different world). `WorldStateOps` methods log lookup misses at Debug — not Warn — so cross-world relays don't spam logs.

## 6. Per-opcode actions

### 6.1 Wireable (6 opcodes — wire in this slice)

| TS source | Opcode | `WorldStateOps` method | Action |
|---|---|---|---|
| World.ts:2001-2007 | RELAY_MUTE | `SetPlayerMute(u37, mutedUntilMs)` | lookup player; if found, `p.mutedUntil = time.UnixMilli(mutedUntilMs)` under `playersMu.Lock`. (Goscape stores `mutedUntil` as `time.Time`; existing field at handler_message_private.go:124-127 gates chat on it.) |
| World.ts:2008-2019 | RELAY_KICK | `KickPlayer(u37)` | lookup; if found, `p.loggingOut = true` under `playersMu.Lock`. (NAI-186-D1 pattern — defer to processLogouts.) |
| World.ts:2024-2027 | RELAY_SHUTDOWN | `Shutdown(durationTicks)` | `s.rebootTimer(int(durationTicks))` — existing function at `reboot.go:17`, sets `shutdownTick = currentTick + duration`. |
| World.ts:2035-2036 | RELAY_RELOAD | `Reload()` | Non-blocking send on `s.rebuildReq` channel. Drop + log Warn if full (the rebuild is already queued). |
| World.ts:2037-2038 | RELAY_CLEARLOGINS | `ClearLogins()` | `s.playersMu.Lock(); s.newPlayers = nil; s.playersMu.Unlock()`. TS clears `loginRequests`; goscape's analogue is `newPlayers`, drained by processLogins each tick. |
| World.ts:2039-2040 | RELAY_CLEARLOGOUTS | `ClearLogouts()` | **Tagged no-op.** See §6.3. |

### 6.2 Deferred (1 opcode — guaranteed deferred)

| Opcode | Tag opened | Reason |
|---|---|---|
| RELAY_QUEUESCRIPT | `NAI-S5B-D-NO-RUNESCRIPT-RUNTIME` | Runescript runtime cannot yet dispatch `[queue,<name>]` to a player. Compiler is done through NAI-211; runtime is partial ([[runescript-Run-observability-close]]). Dispatcher stays slog-warn for this opcode. Retires when runtime can resolve a `[queue,<name>]` trigger by name and enqueue it onto `player.activeScript`. |

### 6.3 Conditional — investigate at plan time

**BROADCAST.** TS L2020-2023: `this.broadcastMes(message)`. Goscape has `Server.BroadcastMes(args)` at `handlers_game.go:1071`. **Plan-time question:** does `BroadcastMes` already write `MESSAGE_GAME` to every connected client (i.e., does it cross the NAI-182-D5 social-cluster ServerGameProt gate)? If **yes and wired**, BROADCAST is wireable — add `BroadcastMessage(message string)` to `WorldStateOps`. If **gated/no-op today**, open `NAI-S5B-D-NO-INGAME-EMIT-BROADCAST` and keep slog-warn.

**TRACK.** TS L2028-2034: `player.submitInput = state`. The `submitInput` field already exists on `Player` and is flipped by `::report MACROING` (handler_reportabuse.go:69) — purely server-internal, no ServerGameProt write. **Recommendation:** wire via `SetPlayerInputTracking(u37 uint64, state int32)` on `WorldStateOps`. **Plan-time verification:** confirm the field is never crossed to client packet flow on its own (the *output* — tracking submissions — already goes through `LoggerBridge.SubmitInputTracking`, which is wired). If verified clean, add to interface; otherwise open `NAI-S5B-D-NO-INGAME-EMIT-TRACK`.

### 6.4 The CLEARLOGOUTS gap

TS L2039: `this.logoutRequests.clear()`. Goscape has **no separate logout-request queue**. The logout pattern is "flip `loggingOut=true` (anywhere), the tick goroutine drains via `removePlayerOnTick`, or the disconnect goroutine drains via `removePlayerOnDisconnect`." There is no third pending-list.

Synthesizing such a list would be unsafe: clearing it would mean either un-flipping `loggingOut` (race: the player may already be mid-teardown) or rolling back a tick-loop side effect (worse). The semantic doesn't map.

**Decision:** `ClearLogouts()` is a tagged no-op. The dispatcher still calls it (preserves the interface symmetry); the impl logs Info "RELAY_CLEARLOGOUTS received (no-op: goscape has no logout-request queue)" and returns. Tag `NAI-S5B-D-CLEARLOGOUTS-NO-GOSCAPE-QUEUE` is **permanent** — goscape's logout architecture differs intentionally from TS.

## 7. Test strategy

### 7.1 Unit (dispatcher → ops)
- `world_events_dispatcher_test.go` per-opcode tests against a recording `WorldStateOps` fake.
- Assert (a) the right ops method fired with the right args, (b) the inner slog dispatcher also fired (composition).
- Deferred opcodes (QUEUESCRIPT + any of BROADCAST/TRACK that end up gated): assert NO ops method fired; assert slog-warn log line emitted.

### 7.2 Integration (ops on *Server)
- `world_state_ops_test.go` — boot a `*Server` via existing `newTestServer` (or equivalent test harness from slice 5a's `world_test_util_test.go`); invoke each `WorldStateOps` method; assert world-state effect.
  - `SetPlayerMute` → `mutedUntil` field updated on online player.
  - `KickPlayer` → `loggingOut` flipped (mirror existing `::kick` cheat assertion).
  - `Shutdown(60)` → `shutdownTick == currentTick + 60`.
  - `ClearLogins` → `newPlayers` slice empty.
  - `Reload` → one value drained from `rebuildReq` non-blocking.
  - `ClearLogouts` → tagged no-op; assert no crash + log line.

### 7.3 E2E (round-trip)
Extend slice-5a's `TestFriendsClient_E2E_RelayWorldEventsRoundTrip`:
- Bring up real friends-server gRPC + two worlds.
- Issue `RelayShutdown` from world A targeting world B.
- World B subscriber receives, dispatcher applies, assert world B's `shutdownTick` advanced within 250ms.
- One additional opcode (recommend `RelayReload` or `RelayClearLogins` — they need no player setup).

## 8. Tag movements (precise)

### Retires
- **`NAI-S5A-D-DISPATCHER-NO-ACTION`** — piecewise. The umbrella tag's doc-comment in `bridges.go` lists 9 sub-bullets (one per opcode). Each opcode that wires deletes its bullet. When all 6 wireable + 3 deferred-tagged opcodes are accounted for (either wired or covered by a NAI-S5B-D-* gate), the umbrella tag is fully retired by deleting the doc-comment block on `slogWorldEventsDispatcher`.

### Opens
- **`NAI-S5B-D-CLEARLOGOUTS-NO-GOSCAPE-QUEUE`** — permanent (architectural divergence). §6.4.
- **`NAI-S5B-D-NO-RUNESCRIPT-RUNTIME`** — retires when runescript runtime can dispatch named scripts to players. §6.2.
- **`NAI-S5B-D-NO-INGAME-EMIT-BROADCAST`** — conditional (open only if BROADCAST is gated). Retires alongside NAI-182-D5. §6.3.
- **`NAI-S5B-D-NO-INGAME-EMIT-TRACK`** — conditional (open only if TRACK turns out to need a packet emit). Retires alongside NAI-182-D5. §6.3.

## 9. Out of scope

- **NAI-S4A-D-PERPLAYER-NOT-PERWORLD-STREAM** unification of `SubscribeUpdates` + `SubscribeWorldEvents` — out of scope; future slice.
- **NAI-S5A-D-NO-ADMIN-AUTH-AT-SERVER** — admin RPC auth still TODO; not 5b territory.
- **NAI-S5A-D-WORLDEVENTS-DROP-ON-FULL** — backpressure tuning; permanent registry behavior.
- **NAI-S5A-D-PERWORLD-EVENTS-STREAM-SEPARATE** — permanent.
- **Admin CLI / web UI** to issue Relay* RPCs — the outbound bridge surface ships in 5a; whoever calls it is downstream of slice 5b.

## 10. Plan-execution discipline (carried from slice 5a)

- **Forbid `git checkout` / `git restore` on tracked files in implementer prompts.** Slice 5a T0 lost the user's in-flight Makefile edits this way; the right escape hatch is BLOCKED-status reporting.
- **Test helper files use `_test.go` suffix.** If 5b introduces a new test helper, name it `world_state_ops_test_util_test.go` (NOT `world_state_ops_test_util.go`) so it doesn't compile into the production binary.
- **Plan-writer must grep for cited test helpers before referencing them.** Slice 5a T3 cited helpers that didn't exist; the implementer adapted on the fly. The 5b plan should cite `createTestDB`, `newTestServer`, slice-5a's `world_test_util_test.go` helpers — all verified by grep at plan-write time.
- **Whole-slice reviewer pass at the end.** 5b's blast radius is comparable to 5a (single-package touches with cross-opcode consistency); reviewer pass justified.
- **Per-task test scope:** structural changes (new interface, new dispatcher impl, NewServer rewire) need `go test -race ./...`, not just the immediate package.
