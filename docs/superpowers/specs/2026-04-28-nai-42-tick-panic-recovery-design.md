# NAI-42 — Tick-wide panic-recovery convention

**Status:** open
**Date:** 2026-04-28
**HEAD at spec-write:** `e87ba81` (post-NAI-41)
**Tech Stack:** Go 1.26+

## 1. Purpose

Port TS's per-player try/catch convention to goscape's tick. Closes
`NAI-37-D-WORLDQUEUE-NO-PANIC-RECOVERY` and aligns five tick steps with
TS's panic-recovery semantics:

- A panic in one player's tick handling must NOT abort the tick goroutine
  for other players.
- The panicking player is force-disconnected (matches TS `player.logout()
  + player.client.close()` recovery action at `World.ts:651-657` and
  `World.ts:736-742`).
- A panic in the world script queue removes the offending entry without
  aborting the queue iteration (closes `NAI-37-D-WORLDQUEUE-NO-PANIC-RECOVERY`
  tagged at `world_script_queue.go:55-59`).

Without this convention, a single buggy script or malformed-packet bug
would crash the entire tick goroutine, dropping every connected player.

## 2. TS Reference

Two TS try/catch blocks define the convention:

**`World.processClients`** (input phase) — `World.ts:603-658`:

```ts
for (const player of this.playerLoop.all()) {
    try {
        player.playtime++;
        // ... afk + decodeIn + opcalled-path-branch + processInputTracking + logPublicChat
    } catch (err) {
        console.error(err);
        if (isClientConnected(player)) {
            player.logout();
            player.client.close();
        }
    }
}
```

**`World.processPlayers`** (script/timer/interaction phase) — `World.ts:703-743`:

```ts
for (const player of this.playerLoop.all()) {
    try {
        // ... delay-expiry + script-resume + processQueues + processTimers
        // ... + processEngineQueue + processInteraction + updateEnergy + validateDistanceWalked
    } catch (err) {
        console.error(err);
        if (isClientConnected(player)) {
            player.logout();
            player.client.close();
        }
    }
}
```

**World queue iteration** — `World.ts:534-559` (referenced in deviation tag):

```ts
for (let i = 0; i < this.queue.length; i++) {
    const entry = this.queue[i];
    // ...
    try {
        ScriptRunner.execute(entry.script);
    } catch (err) {
        console.error(err);
    }
}
```

The recovery action is structurally identical across the two player blocks:
log + force-disconnect. The world-queue catch logs only (no Player to
disconnect).

## 3. Goscape HEAD State

Per-player iteration sites in `modules/world/tick.go` (10 total):

| Line | Function | TS-counterpart in try/catch? |
|------|----------|-------|
| 70   | `processClientsIn` → `p.processIn` | ✅ TS `processClients` |
| 146  | `processLogouts` → idle/timeout flag | ❌ TS `processLogouts` not wrapped |
| 187  | `processPathing` → `p.resolveMovement` | ❌ TS pathing inside `processMovementDirections`; not wrapped |
| 202  | `processActiveScripts` → resume + `processPlayerQueue` | ✅ TS `processPlayers` (queue + script-resume) |
| 261  | `processPlayerTimers` → fire timers | ✅ TS `processPlayers` (timers) |
| 302  | `processClientsOut` → `p.processOut` | ❌ TS `World.processClientsOut` not wrapped |
| 318  | `processInfo` (appearance regen) | ❌ TS info path not wrapped |
| 342  | `processInfo` (rsbuf push) | ❌ TS info path not wrapped |
| 428  | `processInteractions` → `p.processInteraction` | ✅ TS `processPlayers` (`processInteraction` call) |
| 438  | `processCleanup` → `p.ResetMasks` | ❌ TS cleanup not wrapped |

World-queue iteration site:
- `modules/world/world_script_queue.go:60-86` — `processWorldQueue`
  body iterates `s.worldScriptQueue` and calls `s.resumeOrFinishWorld(state)`.
  Already self-tagged: `// DEVIATION NAI-37-D-WORLDQUEUE-NO-PANIC-RECOVERY`
  at lines 55-59.

Existing logout machinery (consumed by recovery action):
- `Player.requestLogout bool` (`player.go:180`) — set by recovery; read by
  `processLogouts` at `tick.go:155`.
- `Player.client.conn` (`net.Conn`) — closed by recovery; existing precedent
  at `tick.go:86, 169`.

Existing logger plumbing:
- `Server.log *slog.Logger` (`server.go:48`) — passed to handlers
  throughout. Recovery uses `s.log.Error(...)`.

## 4. Design

### 4.1 Recovery helpers

Two helpers in a new file `modules/world/tick_recovery.go`:

```go
package world

import (
    "log/slog"
    "runtime/debug"
)

// recoverPlayer recovers from panics during a per-player tick step.
// Mirrors TS World.ts:651-657 / 736-742 catch action: structured log +
// force-disconnect. Sets requestLogout so processLogouts picks the
// player up next tick, and closes the TCP connection immediately so
// subsequent decode attempts fail fast.
//
// Must be called from inside a `defer func() { ... }()` block; recover()
// must run in the deferred frame per Go semantics.
//
// op identifies the tick step ("processIn", "processInteraction", etc.)
// for log readability; pass a constant string per call site.
func recoverPlayer(p *Player, op string, log *slog.Logger) {
    r := recover()
    if r == nil {
        return
    }
    log.Error("panic in tick step",
        "op", op,
        "player", p.username,
        "level", p.level, "x", p.x, "z", p.z,
        "err", r,
        "stack", string(debug.Stack()))
    p.requestLogout = true
    if p.client != nil && p.client.conn != nil {
        _ = p.client.conn.Close()
    }
}

// recoverWorldScript recovers from panics during world-script-queue
// execution. The world queue has no Player to disconnect; the offending
// entry was already removed before fire (per processWorldQueue's
// remove-before-fire ordering at world_script_queue.go:75), so recovery
// only logs.
//
// Mirrors TS World.ts:534-559 catch action.
func recoverWorldScript(state *script.ScriptState, log *slog.Logger) {
    r := recover()
    if r == nil {
        return
    }
    var scriptName string
    if state != nil && state.Script != nil {
        scriptName = state.Script.Name
    }
    log.Error("panic in world script execution",
        "script", scriptName,
        "err", r,
        "stack", string(debug.Stack()))
}
```

### 4.2 Call-site pattern

Each TS-faithful site wraps its body in a closure with a `defer recoverPlayer(...)`:

```go
// Before:
for _, p := range players {
    p.processIn(s.currentTick)
}

// After:
for _, p := range players {
    func(p *Player) {
        defer recoverPlayer(p, "processIn", s.log)
        p.processIn(s.currentTick)
    }(p)
}
```

The `defer recoverPlayer(...)` evaluates `p`, `op`, `s.log` at registration
time — all are pointer/string/value, fine. The `recover()` call runs
inside `recoverPlayer` which is itself the deferred function, satisfying
the Go semantic requirement.

### 4.3 Sites to wrap

Five sites total:

1. `processClientsIn` (tick.go:70) — `op = "processIn"`
2. `processActiveScripts` (tick.go:202) — `op = "processActiveScripts"`. The
   loop body has 3 sub-actions (delay-expiry, script-resume, processPlayerQueue).
   Single wrapper covers all three (matches TS `processPlayers` granularity).
3. `processPlayerTimers` (tick.go:261) — `op = "processPlayerTimers"`. Wrapper
   covers the entire per-player body (id collection + sort + per-id fire loop).
4. `processInteractions` (tick.go:428) — `op = "processInteraction"`
5. `processWorldQueue` (world_script_queue.go:60) — wraps the
   `s.resumeOrFinishWorld(state)` call with `defer recoverWorldScript(state, s.log)`.

### 4.4 Sites NOT wrapped (TS-faithful asymmetries; not deviations)

These per-player iterations have no TS try/catch counterpart; leaving them
unwrapped is true-to-TS:

- `processLogouts` (tick.go:146) — TS `processLogouts` is unwrapped.
- `processPathing` (tick.go:187) — TS pathing runs inside multiple
  blocks; the closest wrapped analog is `processInteraction` already
  covered.
- `processClientsOut` (tick.go:302) — TS output writes are unwrapped.
- `processInfo` (tick.go:318, 342) — TS Player/Npc info encoding is
  unwrapped (a panic here would corrupt the ComputeShared output but
  rarely originates from per-player state).
- `processCleanup` (tick.go:438) — TS cleanup is unwrapped.
- NPC iterations (`processNpcs`, NPC parts of `processInfo`, etc.) —
  TS does not wrap NPC loops.

If a future incident shows one of these needs a wrapper, extend the
convention then; YAGNI applies (per `dead_api_polish.md`).

## 5. Test Strategy

`modules/world/tick_recovery_test.go`:

- **`TestRecoverPlayer_NoPanic`** — `recoverPlayer` is a no-op when
  `recover()` returns nil. Asserts `requestLogout == false` and the
  client connection unaffected after a clean run.
- **`TestRecoverPlayer_PanicSetsLogout`** — invoke a panic inside a
  deferred `recoverPlayer` block; assert `p.requestLogout == true`
  post-panic.
- **`TestRecoverPlayer_PanicClosesConn`** — same, plus assert
  `p.client.conn` is closed (use a `net.Pipe` or in-memory conn whose
  closed state is observable).
- **`TestRecoverPlayer_NilClientSafe`** — `p.client == nil` must not
  panic during recovery (defensive; no-client `Player` exists in tests).
- **`TestRecoverWorldScript_NoPanic`** — no-op when no panic.
- **`TestRecoverWorldScript_PanicSwallowed`** — panic swallowed; caller
  resumes; logging-side-effect assertion left to integration coverage.
- **`TestRunPerPlayer_OnePanicsOthersContinue`** — integration-style:
  a slice of three players; the middle one's `fn` panics; assert the
  first runs cleanly, the third runs cleanly, and only the middle is
  marked for logout.

A test logger (`slog.New(slog.DiscardHandler)`) is used to silence
panic-stack output during test runs.

The 5 production wrap sites are exercised by their existing tick-step
tests; the wrappers are pass-through when no panic occurs, so existing
green tests must remain green.

## 6. Tasks (provisional)

To be detailed in the plan doc. Sketch:

1. **T1** — Helpers + helper tests. Land `tick_recovery.go` and
   `tick_recovery_test.go`. Production wrap sites untouched.
2. **T2** — Wrap the 4 `tick.go` sites. Existing tests must stay green
   (wrappers are transparent). Add one integration-style test that
   injects a panic into one `processIn` and asserts other players run.
3. **T3** — Wrap `processWorldQueue`. Closes
   `NAI-37-D-WORLDQUEUE-NO-PANIC-RECOVERY`. Update the deviation comment
   at `world_script_queue.go:55-59` to retire the tag.

LOC estimate: ~40 helper + ~120 helper tests + ~30 wrap-site edits +
~30 integration test = ~220 LOC across 4 files.

## 7. Deviations

This sub-spec introduces no new deviations. Closes one tracked deviation:

- `NAI-37-D-WORLDQUEUE-NO-PANIC-RECOVERY` — retired at T3.

The 6 unwrapped sites listed in §4.4 are TS-faithful asymmetries (TS
also doesn't wrap them) and do not warrant deviation tags.

## 8. Out of scope

- Unification of `interactionFired` (goscape S6a-era flag) with TS
  `opcalled` semantics. That's `NAI-40-SB1`, deferred — its consumer
  (TS `World.ts:613-642` path-execution branch) is not yet ported.
- NPC-iteration panic recovery. TS doesn't wrap NPC loops.
- Top-level tick-goroutine recovery (panic in a non-wrapped site still
  crashes the goroutine). True-to-TS: TS lets non-wrapped panics propagate.
- Restart logic for the tick goroutine. Out of scope for both TS and goscape.
- Replacing existing `_test.go` panics with structured-error returns.
  Tests that intentionally `t.Fatal` on impossible state are unchanged.

## 9. Memory entries to apply

- `runescript_cadence.md` — full cadence (auto-mode collapse).
- `true_to_ts_gate.md` — drives the 5-site (not 10-site) scope.
- `dead_api_polish.md` — drives the §4.4 "don't wrap until needed" call.
- `verify_implementer_claims.md` + `implementer_commit_content_verify.md`
  — reviewer protocol.
- `controller_preflight.md` — pre-dispatch grep verification of
  `requestLogout` / `client.conn` / `s.log` references in plan code blocks.
- `plan_grep_helper_patterns.md` — confirm no existing panic-recovery
  helpers in goscape (verified at spec-write: `recover()` only at
  `pkg/io/packet/buffer.go:229`).
