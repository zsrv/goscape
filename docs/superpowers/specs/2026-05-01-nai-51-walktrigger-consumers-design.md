# NAI-51 — Walktrigger consumers (Player + NPC)

**Date:** 2026-05-01
**Tech stack:** Go 1.26+
**Closes deviations:** `NAI-37-D-WALKTRIGGER-NOREADER`, `NAI-44-D-PLAYER-WALKTRIGGER-NOOP`

## Problem

Two walktrigger-consumer hooks exist as TS-shape stubs but never fire:

- **Player side** — `(p *Player).processWalktrigger()` at
  `modules/world/interaction.go:241` is an empty no-op. Already invoked at the
  two TS-faithful call sites in `processInteraction` (lines 169 + 183). Tracked as
  `NAI-44-D-PLAYER-WALKTRIGGER-NOOP`.
- **NPC side** — `Npc.walktrigger int` field at `modules/world/npc.go:97`
  (default `-1`) is written by the existing `NPC_WALKTRIGGER` opcode handler
  (opcode 2545, `pkg/script/handlers_npc.go:407`) but never read. No consumer in
  `processMovementInteraction`. Tracked as `NAI-37-D-WALKTRIGGER-NOREADER`.

Both gaps prevent `walktrigger`-driven RuneScript content from firing. Wire them.

## Scope

Two parallel consumers, one per entity. Independent infra (separate fields,
separate triggers, separate providers). Bundled because they share the
"walktrigger" concept and were tagged together for joint closure since NAI-37.

### In scope

- **Bundle 1 (Player):** add `Player.walktrigger int` field, port
  `P_WALKTRIGGER` (opcode 2128) + `GETWALKTRIGGER` (opcode 2023) opcode
  handlers, implement `(p *Player).processWalktrigger()` body.
- **Bundle 2 (NPC):** insert walktrigger consumer in
  `(*Npc).updateMovement` (`modules/world/npc_interaction.go:277`) before step
  consumption.

### Out of scope (explicitly deferred)

- **`MoveClickHandler` PLAYERPACKET-gated processWalktrigger call** — TS site
  `network/.../MoveClickHandler.ts:53`, gated on
  `NODE_WALKTRIGGER_SETTING == PLAYERPACKET`. Goscape `NodeWalktriggerSetting`
  config field exists (`config.go:25`) but is unused at HEAD; activating it
  is a separate concern (config-enum port + behavioral-mode validation).
- **`World.ts:639` PLAYERSETUP-gated call** — inside the unported
  `World.ts:613-642` userPath/opcalled branch. Tracked as `NAI-40-SB1`,
  BLOCKED on userPath/routefinder port per memory.

## Goals

1. Close `NAI-37-D-WALKTRIGGER-NOREADER` and `NAI-44-D-PLAYER-WALKTRIGGER-NOOP`.
2. Match TS dispatch order and field-clearing semantics exactly.
3. Land one new tracked deviation (Player `protect` flag absent — see
   "Deviations introduced") rather than silently diverging.

## Non-goals

- No new tracked deviations beyond the one named below.
- No reshape of `processInteraction` (the two call sites at lines 169 + 183 are
  already correct).
- No reshape of `updateMovement`'s existing structure beyond a single insertion.

## TS reference

### Player consumer — `Engine-TS/src/engine/entity/Player.ts:1057-1070`

```ts
// to allow p_walk (sets player destination tile) during walktriggers
// we process walktriggers from regular movement in client input,
// and for each interaction.
processWalktrigger() {
    if (this.walktrigger !== -1 && !this.protect && !this.delayed) {
        const trigger = ScriptProvider.get(this.walktrigger);
        this.walktrigger = -1;
        if (trigger) {
            const script = ScriptRunner.init(trigger, this);
            this.runScript(script, true);
        }
    }
}
```

### Player opcode handlers — `Engine-TS/src/engine/script/handlers/PlayerOps.ts:1035-1042`

```ts
[ScriptOpcode.WALKTRIGGER]: state => {
    state.activePlayer.walktrigger = state.popInt();
},

[ScriptOpcode.GETWALKTRIGGER]: state => {
    state.pushInt(state.activePlayer.walktrigger);
},
```

### NPC consumer — `Engine-TS/src/engine/entity/Npc.ts:343-360`

```ts
if (this.moveSpeed !== MoveSpeed.INSTANT) {
    this.moveSpeed = this.defaultMoveSpeed();
}

if (this.waypointIndex !== -1) {
    if (this.walktrigger !== -1) {
        const type = NpcType.get(this.type);
        const script = ScriptProvider.getByTrigger(
            ServerTriggerType.AI_QUEUE1 + this.walktrigger,
            type.id, type.category);
        this.walktrigger = -1;

        if (script) {
            const state = ScriptRunner.init(script, this, null, [this.walktriggerArg]);
            ScriptRunner.execute(state);
        }
    }

    super.processMovement();
}
```

## Architecture

### Bundle 1 — Player walktrigger

**Files touched:**

- `modules/world/player.go` — add `walktrigger int` field (default `-1` set in
  player constructor; locate the existing default-init block).
- `pkg/script/active.go` — add `WalkTrigger() int` and `SetWalkTrigger(scriptID int)`
  to the `ActivePlayer` interface (alongside the analogous `ActiveNpc` methods at
  `active.go:546-556`).
- `modules/world/script_active.go` (or wherever `*Player` adapts to
  `ActivePlayer`) — implement the two methods.
- `pkg/script/handlers_player.go` — add `handleWalkTrigger` and
  `handleGetWalkTrigger`.
- `pkg/script/handlers.go` — register both in the dispatch table:
  `OpWalkTrigger: handleWalkTrigger` and `OpGetWalkTrigger: handleGetWalkTrigger`.
- `pkg/script/handlers_player_test.go` — extend mock `ActivePlayer` with the two
  methods and round-trip tests.
- `modules/world/interaction.go:232-241` — replace the empty-stub body of
  `(p *Player).processWalktrigger()`.
- `modules/world/interaction_test.go:646-674` — rewrite `TestProcessWalktriggerNoOp`;
  it currently asserts the stub-no-op contract, which will no longer hold.

**`processWalktrigger` body (proposed):**

```go
func (p *Player) processWalktrigger() {
    if p.walktrigger == -1 || p.delayed {
        return
    }
    if p.client == nil || p.client.server == nil {
        return
    }
    s := p.client.server
    sf := s.scriptProvider.GetByID(p.walktrigger)
    p.walktrigger = -1
    if sf == nil {
        return
    }
    s.runScript(sf, p, nil, true, nil, nil)
}
```

Notes on the `runScript` call: `(s *Server).runScript(sf, p, target=nil, autorelease=true, intArgs=nil, stringArgs=nil)` — the existing pattern used by `handler_inv_button.go:54`, `handler_interface.go:42`, `tick.go:138`, etc. The `true` for `autorelease` matches TS `runScript(script, true)`.

**`handleWalkTrigger` (opcode 2128):**

```go
func handleWalkTrigger(s *ScriptState) error {
    p, ok := s.ActivePlayer.(ActivePlayer)
    if !ok || p == nil {
        return errNoActivePlayer
    }
    p.SetWalkTrigger(s.PopInt())
    return nil
}
```

**`handleGetWalkTrigger` (opcode 2023):** symmetric — calls `p.WalkTrigger()`,
pushes via `s.PushInt`.

(Spec authors: verify the exact `errNoActivePlayer` + `requireActivePlayer`
helper conventions in `handlers_player.go` at plan-write time per memory entry
`plan_grep_helper_patterns.md`. Use the existing helper if one exists.)

### Bundle 2 — NPC walktrigger

**Files touched:**

- `modules/world/npc_interaction.go:277-308` — insert consumer block at the top
  of `updateMovement`, after the `MoveRestrictNoMove` and `waypointIndex < 0`
  early-returns.

**Insertion (proposed):**

```go
func (n *Npc) updateMovement(s *Server) bool {
    if n.moveRestrict == MoveRestrictNoMove {
        n.walkDir = -1
        n.runDir = -1
        return false
    }
    if n.waypointIndex < 0 {
        n.walkDir = -1
        n.runDir = -1
        return false
    }

    // NAI-51: walktrigger fire BEFORE step consumption.
    // Mirrors TS Npc.ts:347-357.
    if n.walktrigger != -1 && n.typ != nil {
        trigger := script.TriggerAiQueue1 + script.ServerTriggerType(n.walktrigger)
        sf := s.scriptProvider.GetByTrigger(trigger, n.typeId, n.typ.Category)
        wtArg := n.walktriggerArg
        n.walktrigger = -1
        if sf != nil {
            s.runNpcScript(sf, n, nil, []int{wtArg}, nil)
        }
    }

    advanced1, dir1 := n.stepOnce(s)
    // ... (rest unchanged)
}
```

Notes:

- TS clears `walktrigger` BEFORE the script-fire-or-skip check, so a missing
  script still resets the field. Mirrored above (assignment placed before the
  `if sf != nil`).
- `walktriggerArg` is captured to a local before the field could potentially
  be mutated by the script's side effects (defensive — TS reads it after the
  walktrigger reset, but goscape's plan-write convention prefers explicit
  capture).
- `runNpcScript` signature: `(s *Server) runNpcScript(sf, npc, target, intArgs, stringArgs)`
  per `npc_script.go:278-290`. `target=nil` matches TS `null`.

## Test strategy

### Bundle 1 tests

| Test | Location | Asserts |
|------|----------|---------|
| `TestNewPlayer_WalkTriggerDefault` | `modules/world/player_test.go` | `NewPlayer(...).walktrigger == -1` |
| `TestHandleWalkTrigger_PopsAndWrites` | `pkg/script/handlers_player_test.go` | `pushInt(42)` → handler → mock records `SetWalkTrigger(42)` |
| `TestHandleGetWalkTrigger_ReadsAndPushes` | `pkg/script/handlers_player_test.go` | mock returns 99 → handler → `popInt() == 99` |
| `TestProcessWalktrigger_UnsetNoOp` | `modules/world/interaction_test.go` (rewrite of existing) | `walktrigger=-1` → no script lookup, no field write |
| `TestProcessWalktrigger_DelayedNoOp` | `modules/world/interaction_test.go` | `walktrigger=N + p.delayed=true` → no script lookup, field unchanged |
| `TestProcessWalktrigger_FiresAndClears` | `modules/world/interaction_test.go` | walktrigger=N + script registered → script fires once + field cleared to -1 |
| `TestProcessWalktrigger_MissingScriptStillClears` | `modules/world/interaction_test.go` | walktrigger=N + no script → field cleared to -1 anyway (TS semantics) |
| `TestProcessInteraction_PreStepWalktriggerFires` | `modules/world/interaction_test.go` | walktrigger set + interaction in range → fires before tryInteract |
| `TestProcessInteraction_PostStepWalktriggerFires` | `modules/world/interaction_test.go` | walktrigger set + waypoints + out of range → fires after pre-step skip |

### Bundle 2 tests

| Test | Location | Asserts |
|------|----------|---------|
| `TestNpcUpdateMovement_WalktriggerFiresThenSteps` | `modules/world/npc_interaction_test.go` | `walktrigger=0` + waypoint + script registered at `(TriggerAiQueue1, typeId, category)` → script fires + field reset + step still consumed |
| `TestNpcUpdateMovement_WalktriggerSentinelSkipsLookup` | `modules/world/npc_interaction_test.go` | `walktrigger=-1` → no provider call, step proceeds |
| `TestNpcUpdateMovement_WalktriggerMissingScriptStillClears` | `modules/world/npc_interaction_test.go` | walktrigger=N + no script registered → field cleared, no fire, step proceeds |
| `TestNpcUpdateMovement_WalktriggerArgPassthrough` | `modules/world/npc_interaction_test.go` | `walktriggerArg=42` + script registered → script fires with `intArgs=[42]` (assert via mock-script handler that reads arg and side-effects observable state) |
| `TestNpcUpdateMovement_WalktriggerNilTypNoOp` | `modules/world/npc_interaction_test.go` | `walktrigger=N + n.typ == nil` → no provider call, step proceeds (defends the `n.typ != nil` guard) |

## Deviations introduced

### `NAI-51-D-PLAYER-WALKTRIGGER-NO-PROTECT-CHECK`

**Where:** `modules/world/interaction.go::(p *Player).processWalktrigger`.

**TS:** `Player.ts:1062` gates on `!this.protect && !this.delayed`.

**Goscape:** gates on `!p.delayed` only.

**Why:** `Player` has no boolean `protect` field at HEAD. Goscape has
anim-protect tracking in the `// === anim-protect (S7b) ===` block at
`player.go:166`, but no equivalent boolean. Adding the field requires understanding
where TS sets/clears `protect` and porting the full lifecycle, which is out of
scope for a walktrigger-consumer sub-spec.

**Closure:** future protect / anim-protect convergence sub-spec. Tag the call
site with a `// DEVIATION NAI-51-D-PLAYER-WALKTRIGGER-NO-PROTECT-CHECK` comment.

## Deviations retired

- `NAI-37-D-WALKTRIGGER-NOREADER` — NPC consumer wired (Bundle 2). Remove the
  deviation comment at `modules/world/npc.go:88-96`.
- `NAI-44-D-PLAYER-WALKTRIGGER-NOOP` — Player consumer wired (Bundle 1). Remove
  the deviation comment at `modules/world/interaction.go:235-240`.

## Net deviation tally

22 (post-NAI-50) → 23 (introduce NAI-51-D-PLAYER-WALKTRIGGER-NO-PROTECT-CHECK)
→ 21 (retire NAI-37-D-WALKTRIGGER-NOREADER + NAI-44-D-PLAYER-WALKTRIGGER-NOOP).

Net change: **−1**.

## Bundle ordering

Bundles are independent (separate files, separate test surfaces, separate
deviation tags). Suggested order:

1. **Bundle 1 (Player)** — larger surface (field + 2 opcode handlers + interface
   methods + body). Land first so review attention captures the broader change.
2. **Bundle 2 (NPC)** — single insertion + tests. Mechanical wrap.

Per-bundle review cadence per `runescript_cadence.md`. Compressed-cadence
candidate per `compressed_cadence.md` if Bundle 2's task ends up ≤~15 LOC.

## Open audit items for plan-write phase

Per `controller_preflight.md`, plan-author should re-grep + Read at HEAD:

1. `requireActivePlayer` / `errNoActivePlayer` helper conventions in
   `handlers_player.go` — use existing helpers, do not inline.
2. `*Player` → `ActivePlayer` adapter location — find the file that implements the
   `ActiveNpc` adapter and add the analogous `WalkTrigger` / `SetWalkTrigger`
   methods there.
3. `Player` constructor / `NewPlayer` location — confirm where to add the
   `walktrigger: -1` default. Per `plan_enumerate_struct_literals.md`, grep for
   `Player{` literals across tests; non-zero default `-1` means existing literals
   that omit `walktrigger` will silently default to `0` (a valid script ID),
   which would fire phantom scripts. Test files MUST be audited.
4. `npc_interaction_test.go` existing `&Npc{...}` literals — same audit applies
   for any test that builds an Npc fixture with default-zero `walktrigger`. The
   `NewNpc` path already sets `-1` (verified) but raw struct literals don't.
5. `runScript` vs `runNpcScript` autorelease parameter difference — confirm by
   reading both signatures at plan-write time.
