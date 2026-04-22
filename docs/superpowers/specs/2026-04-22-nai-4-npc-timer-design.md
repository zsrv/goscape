# NAI-4 — NPC Timer + `ai_timer` Trigger

Add the NPC-side single-slot timer: `timerInterval` / `timerClock`
fields on `*Npc`, `SetTimer` method, `processNpcTimer` pass inside
`Npc.turn()` between resume and queue, and `OpNpcSetTimer` opcode
wiring so scripts can configure the tick interval between `ai_timer`
trigger fires.

Part of the NPC AI tick decomposition roadmap
(`docs/superpowers/specs/2026-04-22-npc-ai-tick-decomposition-design.md`).
Depends on NAI-2 (NPC script infrastructure).

## Goal

After NAI-4 ships:

1. NPCs whose `NpcType.Timer > 0` tick a clock each turn; when it
   hits the interval, the `ai_timer` trigger script fires.
2. RuneScript `npc_settimer <interval>` opcode adjusts the interval
   at runtime. `interval == -1` is a silent no-op (matches TS).
3. Timer does NOT tick while the NPC is `delayed`.
4. `timerClock` resets to 0 only after a successful script fire;
   if no `ai_timer` script is registered for the NPC's type, the
   clock stays at threshold and retries each tick (matches TS).

## Scope

Implements the NAI-4 row of the NAI roadmap. No folded-in follow-ups
this time (nai_followups.md has no NAI-4 items).

## Non-goals

1. **`NumberNotNull` check on interval.** TS `NpcOps.ts:279` wraps
   the popped interval in `check(popInt, NumberNotNull)` (rejects
   negatives). Go accepts any int — matches the deferred audit item
   already tracked in `nai_followups.md`.
2. **Soft vs normal timer distinction.** Player-side has `TimerNormal`
   vs `TimerSoft` with different delayed-gating semantics. NPC-side
   has only one timer per `*Npc`; no type distinction in TS either.
3. **Multiple timers per NPC.** Single-slot per TS. If scripts want
   cascade timing, they use `ai_queueN` (NAI-3).
4. **Integration test that actually fires an `ai_timer` script and
   verifies side effects.** Requires a scriptProvider fixture that
   can register a real script for a trigger; deferred to the same
   audit pass as NAI-3's "speedup quirk" follow-up. Tests instead
   verify observable state (timerClock, timerInterval) plus the
   handler-level enqueue through `mockNpc.SetTimer` recording.

## TS reference

- `Engine-TS/src/engine/entity/Npc.ts:60-61` (field declarations)
- `Engine-TS/src/engine/entity/Npc.ts:210-214` (setTimer method with
  `-1` guard)
- `Engine-TS/src/engine/entity/Npc.ts:424` (constructor seeds
  `timerInterval = type.timer`)
- `Engine-TS/src/engine/entity/Npc.ts:527-536` (processTimers: the
  fire + `timerClock = 0` only on successful script fire)
- `Engine-TS/src/engine/entity/Npc.ts:176-181` (turn order: regen →
  timer → queue → movement-interaction)
- `Engine-TS/src/engine/script/handlers/NpcOps.ts:278-280`
  (NPC_SETTIMER opcode)

## Architecture

### File layout

**Modified:**
- `pkg/script/active.go` — `+SetTimer` on `ActiveNpc` interface
- `pkg/script/handlers_npc.go` — `+handleNpcSetTimer`
- `pkg/script/handlers.go` — `+OpNpcSetTimer` registration
- `pkg/script/handlers_npc_test.go` — `+setTimerCalls` on `mockNpc`
  + 2 tests (happy path + defensive)
- `pkg/script/handlers_player_test.go` — `+SetTimer` no-op stub on
  `mockActiveNpc`
- `modules/world/npc.go` — `+timerInterval`, `+timerClock` fields;
  `+SetTimer` method; `NewNpc` seeds `timerInterval = int(typ.Timer)`
- `modules/world/npc_script.go` — `+processNpcTimer` helper
- `modules/world/npc_ai.go` — `+s.processNpcTimer(n)` call inside
  existing `!n.dead` block, BEFORE `processNpcQueue`
- `modules/world/npc_script_test.go` — 5 tests (2 unit, 2 handler,
  1 integration for delayed-gate)

### Types

No new types. `timerInterval` and `timerClock` are both plain `int`
fields on `*Npc`.

### `ActiveNpc` interface extension (`pkg/script/active.go`)

Appended after NAI-3's `EnqueueScriptForTrigger`, before the closing
`}`:

```go
	// SetTimer sets the tick interval between ai_timer trigger fires
	// on the active NPC. interval == -1 is a silent no-op, matching
	// TS Npc.setTimer at Engine-TS/.../Npc.ts:210-214. Called by the
	// NPC_SETTIMER opcode.
	SetTimer(interval int)
```

### `*Npc` additions (`modules/world/npc.go`)

Fields added to the `// === script state ===` block:

```go
	// === script state ===
	server        *Server
	activeScript  *script.ScriptState
	delayed       bool
	delayedUntil  int
	queue         []script.NpcQueueRequest
	timerInterval int
	timerClock    int
```

**NewNpc update:** seed `timerInterval` from `typ.Timer`. The current
`NewNpc` body does not reference `typ.Timer`; NAI-4 adds one line.
`NpcType.Timer` defaults to `-1` (see `pkg/objtype/npctype.go:246`)
so `timerInterval <= 0` is the disabled state. The `processNpcTimer`
guard handles the `<= 0` case as no-op.

Implementation: in `NewNpc`, set:

```go
timerInterval: int(typ.Timer),
```

(verify location — the existing struct literal in `NewNpc` has no
`timerInterval` field, so this is an addition alongside `nid`,
`typeId`, `typ` etc.)

**SetTimer method** appended (after `ClearActiveScript` /
`SetDelayed` / `EnqueueScriptForTrigger`):

```go
// SetTimer sets the tick interval between ai_timer trigger fires.
// interval == -1 is a silent no-op, matching TS Npc.setTimer at
// Engine-TS/.../Npc.ts:210-214. Implements script.ActiveNpc.SetTimer.
func (n *Npc) SetTimer(interval int) {
	if interval == -1 {
		return
	}
	n.timerInterval = interval
}
```

### `handleNpcSetTimer` (`pkg/script/handlers_npc.go`)

Appended (after `handleNpcQueue`):

```go
// handleNpcSetTimer (NPC_SETTIMER, opcode 2536) sets the active
// NPC's ai_timer tick interval. Pop order: interval. Mirrors TS
// NpcOps.ts:278-280. No NumberNotNull check — tracked as future
// fidelity-audit item in nai_followups memory.
func handleNpcSetTimer(s *ScriptState) error {
	if err := requireActiveNpc(s, "NPC_SETTIMER"); err != nil {
		return err
	}
	interval := s.PopInt()
	s.ActiveNpc.SetTimer(interval)
	return nil
}
```

**Registration** in `pkg/script/handlers.go` in alphabetical position
within the NPC-mutating-ops block. With NAI-3's `OpNpcQueue` already
registered, `OpNpcSetTimer` slots after it and before
`OpNpcSay`/`OpNpcType`/etc. (whatever exists — let gofmt align
columns).

### `processNpcTimer` helper (`modules/world/npc_script.go`)

Appended (after `processNpcQueue`):

```go
// processNpcTimer fires the ai_timer trigger script when timerClock
// reaches timerInterval. Matches TS Npc.processTimers at
// Engine-TS/.../Npc.ts:527-536.
//
// Behaviour:
//   - No-op while delayed (TS gates via the isValid return in
//     turn(); Go gates internally).
//   - No-op when timerInterval <= 0 (unset or explicitly disabled
//     via SetTimer with a non-positive value).
//   - timerClock increments once per call when conditions pass.
//   - timerClock resets to 0 ONLY after a successful script fire.
//     If no ai_timer trigger script is registered for the NPC's
//     type, timerClock stays at threshold and retries every tick —
//     matches TS's "script may be registered later" semantics.
func (s *Server) processNpcTimer(n *Npc) {
	if n.delayed || n.timerInterval <= 0 {
		return
	}
	n.timerClock++
	if n.timerClock < n.timerInterval {
		return
	}
	if n.typ == nil || s.scriptProvider == nil {
		return
	}
	sf := s.scriptProvider.GetByTrigger(script.TriggerAiTimer, n.typeId, n.typ.Category)
	if sf == nil {
		return
	}
	s.runNpcScript(sf, n, nil, nil)
	n.timerClock = 0
}
```

### `Npc.turn()` wire (`modules/world/npc_ai.go`)

Inside the existing `!n.dead` prefix block, the final state after
NAI-4 looks like:

```go
if !n.dead {
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
	// Timer pass. Matches TS Npc.ts:178 (turn calls processTimers).
	s.processNpcTimer(n)
	// Queue pass. Matches TS Npc.ts:180.
	s.processNpcQueue(n)
}
```

The new call is one line between the existing resume block and the
existing `processNpcQueue` call.

## Test strategy

### Unit tests (`modules/world/npc_script_test.go`)

1. **`TestNpcSetTimer`** — construct bare `*Npc`, call
   `n.SetTimer(5)`, assert `n.timerInterval == 5`. Then
   `n.SetTimer(-1)`, assert `n.timerInterval` still `5`
   (no-op semantics per TS).

2. **`TestNewNpcSeedsTimerIntervalFromType`** — build `*NpcType`
   with `Timer: 7`, call `NewNpc`, assert `n.timerInterval == 7`.

3. **`TestNpcTurnDoesNotTickTimerWhileDelayed`** — set
   `n.timerInterval = 3`, `n.delayed = true`, `n.delayedUntil =
   s.currentTick + 100`; call `n.turn(s)` 3 times; assert
   `n.timerClock == 0` (no tick while delayed).

### Handler tests (`pkg/script/handlers_npc_test.go`)

4. **`TestHandleNpcSetTimer`** — push `interval=42`, execute
   `OpNpcSetTimer` on a mockNpc, assert `mockNpc.setTimerCalls`
   records one call with `interval=42`.

5. **`TestHandleNpcSetTimerWithoutActiveNpcErrors`** — state with
   nil ActiveNpc, execute OpNpcSetTimer, assert error
   `"NPC_SETTIMER: no active npc"`.

### Mock updates (`pkg/script/handlers_npc_test.go` + `handlers_player_test.go`)

- `mockNpc`: add `setTimerCalls []int` field recording every
  `SetTimer` invocation. Method records the interval parameter.
- `mockActiveNpc`: no-op stub.

## Fidelity notes

1. **`SetTimer(-1)` is a no-op.** TS `Npc.setTimer` at lines 210-214
   has `if (interval !== -1) { this.timerInterval = interval }`.
   Go mirrors exactly.

2. **`timerClock` reset only on successful fire.** TS line 533
   (`this.timerClock = 0` inside the `if (script)` block). If no
   script is registered for `ai_timer` trigger on this NPC type,
   `timerClock` stays at threshold and each subsequent tick
   increments it further. This is intentional — scripts can be
   registered at any point and the NPC will fire them as soon as
   they appear. Go preserves exactly.

3. **Delayed-gating via internal check.** TS's equivalent gate is
   the `!isValid` early-return in `turn()` at line 154. Go has no
   `isValid` concept (uses explicit `!n.dead` and `!n.delayed`);
   `processNpcTimer` gates internally on `n.delayed`.

4. **`NumberNotNull` divergence.** TS rejects negative intervals via
   `check(popInt, NumberNotNull)`. Go accepts any int. Tracked in
   `nai_followups.md` fidelity-audit bucket.

5. **`NpcType.Timer` default is `-1`** (see
   `pkg/objtype/npctype.go:246`). NPCs without a timer config have
   `timerInterval = -1`, which the `timerInterval <= 0` gate
   treats as disabled. Matches TS `NpcType.timer = -1` default
   behaviour.

## Rough LOC

- `pkg/script/active.go`: +6 (interface method + doc)
- `pkg/script/handlers_npc.go`: +12 (handleNpcSetTimer)
- `pkg/script/handlers.go`: +1 (registration)
- `pkg/script/handlers_npc_test.go`: +45 (mock field + 2 tests)
- `pkg/script/handlers_player_test.go`: +1 (mock stub)
- `modules/world/npc.go`: +12 (2 fields + NewNpc + SetTimer)
- `modules/world/npc_script.go`: +25 (processNpcTimer)
- `modules/world/npc_ai.go`: +2 (call line + comment)
- `modules/world/npc_script_test.go`: +70 (3 tests)

Total ≈ 175 LOC — higher than the roadmap's ~70 estimate because the
test coverage includes an integration test for the delayed-gate and
the NewNpc-seed test, neither of which the roadmap accounted for.
Still within the project's established cadence band.

## Dependencies

- **Blocks:** NAI-5 (lifecycle) will reset `timerClock` on
  revertType. NAI-6 (regen) adds a separate regen clock that sits
  alongside but doesn't interact.
- **Blocked by:** NAI-2 (`runNpcScript`, `ActiveNpc` interface
  script-lifecycle methods, `*Npc.server`/`delayed` fields). NAI-3
  is technically independent but NAI-4 reuses `scriptProvider.GetByTrigger`
  pattern established there.

## Verifications resolved during spec-write

1. `NpcType.Timer` field exists at `pkg/objtype/npctype.go:81`, loaded
   from cache opcode 203, default `-1`.
2. `OpNpcSetTimer` opcode constant exists at `pkg/script/opcode.go:273`
   (value 2536). Disassembler entry at opcode.go:945.
3. `TriggerAiTimer` exists at `pkg/script/trigger.go:140` (value 139).
4. TS `setTimer` `-1`-guard semantics confirmed at `Engine-TS/.../Npc.ts:210-214`.
5. TS `processTimers` `timerClock = 0` only-on-fire confirmed at
   `Engine-TS/.../Npc.ts:533`.
