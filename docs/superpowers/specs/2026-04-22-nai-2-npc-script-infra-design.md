# NAI-2 — NPC Script Infrastructure

Extend the NPC entity and the RuneScript runtime so NPC-anchored scripts
can suspend via `npc_delay`, store on the NPC, and resume from the tick
loop on delay expiry. Port the TS `Npc.executeScript` /
`Npc.turn()`-prefix pattern at `Engine-TS/src/engine/entity/Npc.ts:110-119`
and `:216-239` into Go.

Part of the NPC AI tick decomposition roadmap
(`docs/superpowers/specs/2026-04-22-npc-ai-tick-decomposition-design.md`).
Blocks NAI-3 through NAI-12 (every behavioural sub-spec needs NPC
script anchoring and resumption).

## Goal

After NAI-2 ships, a RuneScript program can:

1. Be invoked with an NPC as the active entity via `runNpcScript`.
2. Call `npc_delay N` to suspend for N ticks.
3. Be resumed automatically at the top of `Npc.turn()` when the delay
   expires.

## Scope extension from the roadmap

The roadmap stated NAI-2 as *"activeScript, delayed, runNpcScript;
suspended-script resume in tick"*. This spec adds `OpNpcDelay` wiring +
`SetDelayed` on the `ActiveNpc` interface (+~30 LOC). Rationale:
`OpNpcDelay` is the only opcode that transitions a script to
`NpcSuspended`; without it the resume machinery has zero producers.
Per the *"Dead-API YAGNI polish pattern"* memory, infrastructure
without a live consumer is a documented anti-pattern on this project.
Including the handler here gives NAI-2 an end-to-end vertical slice
testable with a single `npc_delay` call.

## Non-goals

1. **NPC queue processing** — `OpNpcQueue`, `ai_queue{1..20}` dispatch.
   NAI-3 owns this.
2. **NPC timers** — `OpNpcSetTimer`, `ai_timer`. NAI-4.
3. **`Self = ActiveNpc` unification** — current `script.Init` takes
   `ActivePlayer` only. `runNpcScript` calls `Init(sf, nil, ...)` and
   then manually sets `state.ActiveNpc = npc`. This matches the
   existing test-code pattern and avoids changing the `script.Init`
   signature (out of scope for NAI-2; would cascade across every
   player-side caller).
4. **Protected-pointer cleanup** (TS `Npc.ts:230-238`). See Fidelity
   Notes below — likely already handled downstream; NAI-2 documents
   the concern but does not add code unless verification shows a gap.
5. **`Npc.executeScript` -> world helper refactor.** TS has a method
   on `Npc`. Go puts the helper on `*Server`, matching the existing
   `runScript` / `resumeOrFinish` layout in `modules/world/script.go`.
   Not a behavioural divergence.
6. **Corrupt-state panic recovery in `runNpcScript`.** Per NAI-1's
   course correction (see memory: *"Match spec tests to library
   capabilities"*), no project loader or runner wraps `panic` into
   returned errors. `runNpcScript` matches the plain `runScript`
   shape — no `defer recover()`.

## TS reference

- `Engine-TS/src/engine/entity/Npc.ts:110-119` — turn-prefix: delayed
  expiration + suspended-script resume.
- `Engine-TS/src/engine/entity/Npc.ts:216-239` — `executeScript`: run
  + route by execution state + protected-pointer cleanup.

## Architecture

### File layout

**New:**
- `modules/world/npc_script.go` — `runNpcScript`, `resumeOrFinishNpc`
- `modules/world/npc_script_test.go` — unit + integration tests

**Modified:**
- `modules/world/npc.go` — add `activeScript`, `delayed`, `delayedUntil`
  fields; add `StoreActiveScript`, `ClearActiveScript`, `SetDelayed`
  methods
- `modules/world/npc_ai.go` — prepend delayed-expiration + resume block
  to `Npc.turn()`
- `pkg/script/active.go` — extend `ActiveNpc` interface with three new
  methods
- `pkg/script/handlers_npc.go` — add `handleNpcDelay`
- `pkg/script/handlers.go` — register `OpNpcDelay: handleNpcDelay`

The existing `modules/world/script.go:resumeOrFinish` is not changed;
its `default:` branch that currently warns on NpcSuspended still works
as a defensive fallback, but NPC scripts never reach it because they
route through `resumeOrFinishNpc` instead.

### Runtime topology

```
handleNpcDelay (OpNpcDelay)
├─ ticks := s.PopInt()
├─ s.ActiveNpc.SetDelayed(ticks)
└─ s.Execution = NpcSuspended

runNpcScript(sf, npc, intArgs, stringArgs)  [new, world-side]
├─ script.Init(sf, nil, false, intArgs, stringArgs)
├─ state.ActiveNpc = npc
└─ resumeOrFinishNpc(state, npc)

resumeOrFinishNpc(state, npc)               [new, world-side]
├─ script.Execute(state)
└─ switch state.Execution {
      case Finished, Aborted: npc.ClearActiveScript()
      case NpcSuspended:      npc.StoreActiveScript(state)
      default:                log.Warn(...); npc.ClearActiveScript()
   }

Npc.turn(s)                                 [prepended block]
├─ if delayed && s.currentTick >= delayedUntil { delayed = false }
├─ if !delayed && activeScript != nil && activeScript.Execution == NpcSuspended {
│     state := activeScript
│     state.Execution = Running
│     s.resumeOrFinishNpc(state, n)
│  }
└─ ... existing turn() body ...
```

## Type + method definitions

### `Npc` struct additions (`modules/world/npc.go`)

Grouped under the existing `// === AI ===` section (currently holds
`targetOp`, `wanderCounter`, etc.):

```go
// === script state ===
server        *Server          // back-reference for SetDelayed → currentTick
activeScript  *script.ScriptState
delayed       bool
delayedUntil  int
```

The `server` field is set by `Server.addNpc` when the NPC enters the
registry. Any path that constructs an `*Npc` outside the registry
(tests for opcode-level behaviour) leaves it nil — only `SetDelayed`
touches it, so construction-time tests remain safe.

### `Npc` methods (`modules/world/npc.go`)

Appended near the bottom of the file, after `Slot()`:

```go
// StoreActiveScript saves a Suspended ScriptState so Npc.turn() can
// resume it when the NPC's delay expires. Part of the ActiveNpc
// interface; mirrors *Player.StoreActiveScript.
func (n *Npc) StoreActiveScript(state *script.ScriptState) {
    n.activeScript = state
}

// ClearActiveScript discards any stored ScriptState. Called after
// Finished/Aborted runs. Part of the ActiveNpc interface; mirrors
// *Player.ClearActiveScript.
func (n *Npc) ClearActiveScript() {
    n.activeScript = nil
}

// SetDelayed marks the NPC as suspended for `ticks` more ticks starting
// next tick. delayedUntil = currentTick + 1 + ticks, matching the TS
// Npc.delay() convention and the ActivePlayer.SetDelayed semantics.
func (n *Npc) SetDelayed(ticks int) {
    n.delayed = true
    n.delayedUntil = n.server.currentTick + 1 + ticks
}
```

**Server back-reference on `*Npc`.** `*Player.SetDelayed` reaches
`currentTick` via `p.client.server.currentTick` (see
`modules/world/player_script.go:34-39`). `*Npc` has no `client`, so
NAI-2 adds a `server *Server` field to `*Npc` (single back-reference,
not exported). It is set at NPC-registration time inside
`Server.addNpc` (analogous to how `*Player.client` gets wired at
login). The field is NOT set in `NewNpc(...)` because that constructor
is called from tests that don't have a `*Server`; existing tests that
build an `*Npc` without going through `addNpc` only use it for
opcode-level script tests that never invoke `SetDelayed`.

### `ActiveNpc` interface extensions (`pkg/script/active.go`)

Appended to the `ActiveNpc` interface at line 310:

```go
// StoreActiveScript saves a Suspended ScriptState so the tick loop
// can resume it when the NPC's delay expires. Mirrors ActivePlayer.
StoreActiveScript(state *ScriptState)

// ClearActiveScript discards any stored ScriptState. Called after
// Finished/Aborted runs.
ClearActiveScript()

// SetDelayed marks the NPC as suspended for `ticks` more ticks
// starting next tick. Implementation computes delayedUntil =
// currentTick + 1 + ticks.
SetDelayed(ticks int)
```

### `handleNpcDelay` (`pkg/script/handlers_npc.go`)

Appended to the existing handlers_npc.go file:

```go
// handleNpcDelay (NPC_DELAY, opcode 2511) suspends the active NPC's
// script for N ticks. Transitions the script to NpcSuspended and
// records the wake tick on the NPC itself via SetDelayed. The tick
// loop resumes the script from Npc.turn() when delayedUntil expires.
// Mirrors TS NpcOps.ts NPC_DELAY.
func handleNpcDelay(s *ScriptState) error {
    if s.ActiveNpc == nil {
        return fmt.Errorf("NPC_DELAY: no active npc")
    }
    ticks := s.PopInt()
    s.ActiveNpc.SetDelayed(ticks)
    s.Execution = NpcSuspended
    return nil
}
```

Registered in `pkg/script/handlers.go` adjacent to other NPC handlers:

```go
OpNpcDelay: handleNpcDelay,
```

### `runNpcScript` + `resumeOrFinishNpc` (`modules/world/npc_script.go`)

```go
package world

import (
    "github.com/zsrv/goscape/pkg/script"
)

// runNpcScript initialises a ScriptState anchored on npc (not a
// player) and routes the result via resumeOrFinishNpc. Safe to call
// with a nil scriptFile (no-op). Mirrors runScript at script.go:14.
//
// If the script suspends (Execution == NpcSuspended), the state is
// stored on the NPC and Npc.turn() resumes it when the NPC's delay
// expires.
func (s *Server) runNpcScript(sf *script.ScriptFile, npc script.ActiveNpc, intArgs []int, stringArgs []string) {
    if sf == nil {
        return
    }
    state := script.Init(sf, nil, false, intArgs, stringArgs)
    state.ActiveNpc = npc
    state.Provider = s.scriptProvider
    state.World = s.worldVars
    state.Configs = s.configsView
    state.Inv = s.invLookup
    s.resumeOrFinishNpc(state, npc)
}

// resumeOrFinishNpc is the shared post-Execute handler for both fresh
// NPC-anchored runs (from runNpcScript) and resumed runs (from
// Npc.turn()). Mirrors resumeOrFinish at script.go:30 but routes via
// the ActiveNpc interface instead of ActivePlayer.
func (s *Server) resumeOrFinishNpc(state *script.ScriptState, npc script.ActiveNpc) {
    if err := script.Execute(state); err != nil {
        s.log.Warn("npc script execute error",
            "script", state.Script.Name, "err", err)
        npc.ClearActiveScript()
        return
    }
    switch state.Execution {
    case script.Finished, script.Aborted:
        npc.ClearActiveScript()
    case script.NpcSuspended:
        npc.StoreActiveScript(state)
    default:
        // Suspended / PauseButton / CountDialog / WorldSuspended —
        // not reachable via npc_delay alone, but defensively clear.
        s.log.Warn("npc script in unexpected execution state",
            "script", state.Script.Name, "execution", state.Execution)
        npc.ClearActiveScript()
    }
}
```

### `Npc.turn` prefix (`modules/world/npc_ai.go`)

Prepended at the very top of the existing `turn(s *Server)` method,
before the `if n.dead { ... }` check:

```go
// Delayed expiration. Matches TS Npc.ts:113.
if n.delayed && s.currentTick >= n.delayedUntil {
    n.delayed = false
}
// Resume suspended script. Matches TS Npc.ts:116-118.
if !n.delayed && n.activeScript != nil &&
    n.activeScript.Execution == script.NpcSuspended {
    state := n.activeScript
    state.Execution = script.Running
    s.resumeOrFinishNpc(state, n)
}
```

## Test strategy

Seven tests. Files:

- `modules/world/npc_script_test.go` — tests 1-6 (world-side)
- `pkg/script/handlers_npc_test.go` — test 7 (handler-side, appended
  to existing file)

1. **`TestNpcStoreAndClearActiveScript`** — unit: after
   `n.StoreActiveScript(state)` assert `n.activeScript == state`;
   after `n.ClearActiveScript()` assert `n.activeScript == nil`.
2. **`TestNpcSetDelayed`** — unit: build a test server with
   `currentTick = 100`, attach npc, call `n.SetDelayed(5)`, assert
   `n.delayed == true`, `n.delayedUntil == 106` (100 + 1 + 5).
3. **`TestRunNpcScriptFiresAndFinishes`** — integration: scriptfile
   with just `OpReturn`, invoke `s.runNpcScript(sf, n, nil, nil)`,
   assert `n.activeScript == nil` (cleared on Finished).
4. **`TestRunNpcScriptSuspendsOnNpcDelay`** — integration: scriptfile
   = `[OpPushConstantInt 3, OpNpcDelay, OpReturn]`, invoke
   `runNpcScript` at `s.currentTick = 100`. Assert: `n.activeScript
   != nil`, `n.activeScript.Execution == script.NpcSuspended`,
   `n.delayed == true`, `n.delayedUntil == 104`.
5. **`TestNpcTurnResumesSuspendedScriptAfterDelay`** — end-to-end:
   suspend via npc_delay 3 at tick 100 (so `delayedUntil = 104`),
   advance `s.currentTick = 104`, call `n.turn(s)`, assert
   `n.activeScript == nil` (ran to completion), `n.delayed == false`.
6. **`TestNpcTurnDoesNotResumeWhileDelayed`** — end-to-end: suspend
   at tick 100 (`delayedUntil = 104`), advance `s.currentTick = 103`,
   call `n.turn(s)`, assert `n.activeScript` still non-nil and still
   `NpcSuspended`, `n.delayed` still true.
7. **`TestHandleNpcDelayWithoutActiveNpcErrors`** — unit: a
   `ScriptState` with `ActiveNpc == nil`, call runner for
   `OpNpcDelay`, assert error message `"NPC_DELAY: no active npc"`.
   Matches the defensive-error pattern at `handlers_npc.go:12`.

**Test-fixture helpers needed:** the existing `modules/world` tests
have a pattern for building a minimal `*Server` for NPC-only work
(see `script_test.go:939-952`). NAI-2 reuses this pattern; no new
helpers required.

## Fidelity notes

1. **Protected-pointer cleanup** (TS `Npc.ts:230-238` — drops
   `ProtectedActivePlayer` / `ProtectedActivePlayer2` pointers after
   every run). Not added in this spec. Verification step during the
   implementation plan: inspect `script.Execute` + OpReturn to confirm
   whether protected-pointer state is cleaned up downstream. If a gap
   is found, it is a separate fix (not NAI-2 scope creep).
2. **`Self = nil` for NPC-anchored scripts.** `script.Init` at
   `runner.go:12` takes `ActivePlayer`. `runNpcScript` passes `nil`;
   this matches TS semantics (NPC-only scripts have no active player
   pointer by default) and is what existing test code does (e.g.
   `handlers_npc_test.go:89`: `state := newState(t, sf, nil)` then
   `state.ActiveNpc = npc`).
3. **Go-idiom divergence on file layout.** TS has
   `Npc.executeScript` as a method on the Npc class. Go's
   `resumeOrFinishNpc` is a world-side function because the existing
   Go precedent (`resumeOrFinish`) already made this split for
   *Player. Not a behavioural divergence.
4. **No `defer recover()`** — matches NAI-1's course correction. If
   a script runtime panic occurs in `script.Execute`, it propagates
   to the caller of `runNpcScript` (usually `Npc.turn()`), which
   does not recover. Consistent with the player-side `runScript`.

## Rough LOC

- `modules/world/npc.go`: +3 fields + 3 methods ≈ 25 lines
- `modules/world/npc_script.go`: ~45 lines (two helper functions)
- `modules/world/npc_script_test.go`: ~150 lines (6 tests)
- `modules/world/npc_ai.go`: +10 lines (turn prefix)
- `pkg/script/active.go`: +3 interface methods ≈ 15 lines
- `pkg/script/handlers_npc.go`: +12 lines (handleNpcDelay)
- `pkg/script/handlers.go`: +1 line (opcode registration)
- `pkg/script/handlers_npc_test.go`: +25 lines (test 7)

Total ≈ 280 LOC — slightly over the roadmap's ~120 prod+test estimate
because the scope extension (OpNpcDelay + SetDelayed) and the extra
tests for the vertical slice both add weight. Still well within the
100-400 LOC band the project's cadence supports.

## Dependencies

- **Blocks:** NAI-3 (NPC queue) — shares the `delayed` bool in the
  queue gate, shares `activeScript` for queue-dispatched scripts.
  NAI-4 (NPC timer) — similar. NAI-5 (lifecycle) — needs `delayed`
  to gate `lifecycleTick` decrement. NAI-6 through NAI-12 — all
  consume suspended-script resumption implicitly.
- **Blocked by:** nothing. NAI-1 (HuntType loader) already landed
  but is orthogonal — NAI-2 does not depend on it.

## Verifications resolved during implementation

1. **Protected-pointer cleanup (TS `Npc.ts:230-238`).** Not needed
   within NAI-2's scope. `runNpcScript` passes `protect=false` to
   `script.Init`, which is followed by `state.ActiveNpc = npc` but
   NO `state.Self` (nil) and NO protected-player anchoring. The TS
   cleanup at `:230-238` exists for scripts that carry both an
   active player *and* an NPC-suspend transition — a scenario only
   reachable once player-triggered OpNpc paths (NAI-8+) route
   player-anchored scripts through NPC suspension. This is tracked
   as a **NAI-8 prerequisite**: the first sub-spec that exposes a
   player-anchored script to NPC suspension must add the cleanup to
   `resumeOrFinishNpc` *and* extend `runNpcScript`'s signature to
   accept an active player. No action needed in NAI-2.
2. **`handleNpcDelay` does not need a `Protect` check.** TS's
   `NumberNotNull` check on the popped tick count (rejects negatives)
   is a separate concern; tracked as a low-priority follow-up for a
   future fidelity-audit sub-spec (neither runtime-critical nor
   blocking — scripts don't pass negatives in practice).
3. **`Server.addNpc` is the single entry point.** No alternate
   registration path exists. Task 1 wired `n.server = s` here.
