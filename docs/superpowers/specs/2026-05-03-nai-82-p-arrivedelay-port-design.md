# NAI-82: Port P_ARRIVEDELAY opcode handler + lastMovement entity state

**Date**: 2026-05-03
**Cadence**: standard (spec + plan; upgraded from compressed because the
handler requires a new interface accessor + persisted entity field + per-tick
write site on both Player and Npc — out of the ≤15 LOC compressed bucket per
`compressed_cadence.md`)
**Predecessor**: NAI-81 (`ab37799` — LOC_COORD ported)
**Successor**: TBD (next NAI-N+1 cascade run; no further opcodes from NAI-80
smoke remain unported)

## 1. Problem

`P_ARRIVEDELAY` (opcode 2068) is declared at `pkg/script/opcode.go:168`
(`OpPArriveDelay`) but has no dispatch wiring. NAI-80's user-driven smoke
surfaced one consumer:

- `[oploc1, _bookcase]` pc=0

Pattern: `protocol_stub_not_completed`.

The handler also reads a Player field — `lastMovement` — that does not yet
exist on goscape's `*Player` (no field, no accessor on `ActivePlayer`). The
TS-faithful semantic of that field requires:
1. A persisted `int` field on `*Player`
2. A per-tick write site at the end of waypoint stepping when the player
   actually advanced (`stepsTaken > 0`)
3. Read access exposed via `ActivePlayer.LastMovement() int`

The symmetric NPC field `Npc.lastMovement` has the same TS-faithful contract
(written when the NPC's position changed within `updateMovement`, read by
`AI_ARRIVEDELAY` / `AI_TARGETMOVED`). Per user direction, NAI-82 also lands
the NPC-side **field + write site** — but defers the `ActiveNpc.LastMovement()`
accessor until an unhandled `AI_*` opcode surfaces (YAGNI on the read side;
TS-faithful entity state on the write side).

## 2. TS reference

### 2.1 Handler

`LostCityRS/Engine-TS/src/engine/script/handlers/PlayerOps.ts:357-366`:

```ts
// https://x.com/JagexAsh/status/1648254846686904321
[ScriptOpcode.P_ARRIVEDELAY]: checkedHandler(ProtectedActivePlayer, state => {
    if (state.activePlayer.lastMovement < World.currentTick) {
        return;
    }

    state.activePlayer.delayed = true;
    state.activePlayer.delayedUntil = World.currentTick + 1;
    state.execution = ScriptState.SUSPENDED;
}),
```

Pointer manifest: `ProtectedActivePlayer` (per `checkedHandler`'s first arg).

Behaviour: if the player has *not* moved within the past two ticks (per the
`lastMovement < currentTick` gate), return as a no-op. Otherwise mark the
player delayed for one tick (`delayedUntil = currentTick + 1`) and suspend
script execution.

The 2-tick window comes from how `lastMovement` is written: it is set to
`currentTick + 1`, so the gate accepts movement from this tick (`T+1 < T` ⇒
false) and last tick (`T < T` ⇒ false), but rejects movement from 2+ ticks ago
(`T-1 < T` ⇒ true, return).

### 2.2 lastMovement field — Player write site

`Engine-TS/src/engine/entity/Player.ts:670-680` (`Player.processMovement`):

```ts
if (!super.processMovement()) {
    // todo: this is running every idle tick
    this.tempRun = 0;
}

if (this.stepsTaken > 0) {
    this.lastMovement = World.currentTick + 1;
}

return this.stepsTaken > 0;
```

Field declared at `Engine-TS/src/engine/entity/PathingEntity.ts:56`
(`lastMovement: number = 0`).

### 2.3 lastMovement field — Npc write site

`Engine-TS/src/engine/entity/Npc.ts:337-368` (`Npc.updateMovement`):

```ts
updateMovement(): boolean {
    // ... [moverestrict + speed setup + walktrigger fire + super.processMovement]
    super.processMovement();
}

const moved = this.lastTickX !== this.x || this.lastTickZ !== this.z;
if (moved) {
    this.lastMovement = World.currentTick + 1;
    this.wanderCounter = 0;  // ← out of NAI-82 scope; see §6.2
}
return moved;
```

The TS NPC `moved` signal compares pre-/post-tick position rather than using
`stepsTaken` directly — equivalent for stepping but also captures any
position change (the goscape NPC `Teleport` path bypasses `updateMovement`
anyway, so the two formulations are observationally identical here).

### 2.4 Existing TS readers of `lastMovement`

| TS site | Opcode | goscape status |
|---|---|---|
| `PlayerOps.ts:359` | `P_ARRIVEDELAY` | **target of NAI-82** |
| `NpcOps.ts:543` | `AI_TARGETMOVED` | unported (not in any smoke yet) |
| `NpcOps.ts:548` | `AI_TARGETMOVED` (second branch) | unported |

NAI-82 wires the player-side reader. The NPC-side readers stay deferred; the
NPC-side **field + write site** still ship so that whichever future sub-spec
ports `AI_TARGETMOVED` only needs to add the accessor + interface method.

## 3. Existing goscape surface

| Concern | Location | Status |
|---|---|---|
| Opcode constant | `pkg/script/opcode.go:168` (`OpPArriveDelay = 2068`) | declared |
| Opcode → name | `pkg/script/opcode.go:743-744` | declared |
| Dispatch map | `pkg/script/handlers.go` (siblings to `OpPDelay`) | **missing** |
| `ActivePlayer.SetDelayed(ticks int)` | `pkg/script/active.go:13` (impl computes `currentTick + 1 + ticks`) | available — `SetDelayed(0)` ⇒ `delayedUntil = currentTick + 1` ✓ |
| `ScriptState.Execution = Suspended` | `pkg/script/handlers.go:668` (`handlePDelay` precedent) | available |
| `requireProtectedActivePlayer` gate | `pkg/script/handlers.go` (used by `handlePDelay`) | available |
| `s.World.CurrentTick()` | `pkg/script/state.go:60`; `s.World.CurrentTick()` consumers throughout | available |
| `ActivePlayer.LastMovement() int` | — | **missing** |
| `*Player.lastMovement` field | — | **missing** |
| `*Npc.lastMovement` field | — | **missing** |
| Player movement write site | `modules/world/movement.go:34-68` (`resolveMovement`); `p.stepsTaken` accumulates in `stepOnce` (movement.go:95) | **needs lastMovement write at tail** |
| NPC movement write site | `modules/world/npc_interaction.go:279-326` (`(n *Npc).updateMovement`); `n.lastTickX` snapshot at processMovementInteraction:162 | **needs lastMovement write at tail** |
| `s.currentTick` access from `*Player` | precedent: `modules/world/player_timer.go:29` (`p.client.server.currentTick`) | available, defensive guard convention applies |
| `s.currentTick` access from `*Npc` | direct via `s *Server` parameter to `updateMovement(s)` | available |
| Mock `ActivePlayer` | `pkg/script/runner_test.go:323` (`mockPlayer`) | **needs `LastMovement()`** |

## 4. Design

### 4.1 ActivePlayer interface extension

`pkg/script/active.go` — append a `LastMovement()` accessor adjacent to
`SetDelayed`:

```go
// LastMovement returns the absolute tick value stored on the player's
// lastMovement field. The field is written to currentTick + 1 at the end
// of any tick in which the player actually advanced (stepsTaken > 0),
// matching TS Player.processMovement at Engine-TS/.../Player.ts:675-677.
//
// Consumed by P_ARRIVEDELAY (PlayerOps.ts:359), which suspends the active
// script when the player moved within the past 2 ticks
// (lastMovement >= currentTick) and is a no-op otherwise.
//
// Returns 0 when the player has never moved (zero-value of the field).
LastMovement() int
```

### 4.2 *Player field + accessor

`modules/world/player.go` — add `lastMovement int` to the movement-state
field cluster (alongside `lastTickX/Z`, `lastStepX/Z`, `stepsTaken`). Default
zero-value matches TS (`PathingEntity.lastMovement: number = 0`); no
constructor write needed.

`modules/world/player_script.go` — append:

```go
// LastMovement returns the player's lastMovement field. See
// pkg/script.ActivePlayer.LastMovement docstring for semantics.
func (p *Player) LastMovement() int { return p.lastMovement }
```

### 4.3 *Player write site

`modules/world/movement.go` — at the tail of `(p *Player).resolveMovement()`,
after the run-step branch (movement.go:67) and before the closing brace:

```go
// NAI-82: TS Player.processMovement at Engine-TS/.../Player.ts:675-677
// writes lastMovement = World.currentTick + 1 whenever stepsTaken > 0
// after the tick's movement resolves. Read by P_ARRIVEDELAY's gate.
if p.stepsTaken > 0 && p.client != nil && p.client.server != nil {
    p.lastMovement = p.client.server.currentTick + 1
}
```

The defensive `client != nil && server != nil` guard mirrors the established
convention (e.g. movement.go:84 in `stepOnce`); it makes the write a silent
no-op for fixture tests that construct a bare `*Player` with no client.

### 4.4 *Npc field + write site

`modules/world/npc.go` — add `lastMovement int` to the NPC field cluster
(near `lastTickX/Z`/`stepsTaken`). Zero-value default; no constructor write.

`modules/world/npc_interaction.go` — at the tail of `(n *Npc).updateMovement(s
*Server)` (npc_interaction.go:325), replace the bare `return true` with:

```go
// NAI-82: TS Npc.updateMovement at Engine-TS/.../Npc.ts:362-366 writes
// lastMovement = World.currentTick + 1 when the NPC's position changed
// this tick. Read by AI_ARRIVEDELAY / AI_TARGETMOVED (deferred until a
// future sub-spec ports those handlers; field write ships here so the
// state is correct when consumers arrive).
if (n.x != n.lastTickX || n.z != n.lastTickZ) && s != nil {
    n.lastMovement = s.currentTick + 1
}
return true
```

The position-vs-snapshot check (rather than `stepsTaken > 0`) mirrors TS
exactly — it makes the write site invariant under any future change to NPC
movement that also mutates position outside `stepOnce` (e.g. a hypothetical
re-org that pushes teleport through `updateMovement`).

The `s != nil` guard handles the existing test fixture pattern at
`modules/world/npc_reorient_test.go:85`, where `npc.updateMovement` is
exercised with a nil server.

**Note (out-of-scope; not addressed here):** TS also resets `wanderCounter = 0`
inside the same `if (moved)` block (Npc.ts:365). goscape currently resets
`wanderCounter = 0` unconditionally at `aiMode` entry (npc_interaction.go:218),
which is a pre-existing TS divergence orthogonal to NAI-82. Tracked here as a
**deviation NAI-82-D1** for any future audit; not modified in this sub-spec.

### 4.5 Handler

`pkg/script/handlers.go` — append adjacent to `handlePDelay`:

```go
// handlePArriveDelay implements P_ARRIVEDELAY (opcode 2068): if the
// active player has moved within the past 2 ticks, mark them delayed for
// 1 tick and suspend the script; otherwise no-op. TS PlayerOps.ts:357-366.
//
// The 2-tick window arises from the TS lastMovement contract (written to
// currentTick + 1 after a moving tick): the gate accepts moves from this
// tick and last tick, rejects moves from 2+ ticks ago.
//
// Requires ProtectedActivePlayer pointer.
func handlePArriveDelay(s *ScriptState) error {
    if err := requireProtectedActivePlayer(s, "P_ARRIVEDELAY"); err != nil {
        return err
    }
    if s.Self.LastMovement() < s.World.CurrentTick() {
        return nil
    }
    s.Self.SetDelayed(0)
    s.Execution = Suspended
    return nil
}
```

### 4.6 Dispatch wiring

`pkg/script/handlers.go` — at the existing P_* dispatch cluster (lexical
neighbour to `OpPApRange`/`OpPClearPendingAction`):

```go
OpPArriveDelay:        handlePArriveDelay,
```

### 4.7 Mock updates

`pkg/script/runner_test.go` — extend `mockPlayer` with:

```go
func (m *mockPlayer) LastMovement() int { return m.lastMovement }
```

Add `lastMovement int` field to the `mockPlayer` struct so tests that need
to exercise the suspend branch can seed it. Existing call sites construct
`mockPlayer{}` — Go zero-value preserves current behaviour (no-op branch
fires on the gate).

No `mockActivePlayer` exists in `handlers_player_test.go`; the file uses
direct `mockPlayer` references (verified at brainstorm time via grep — only
`mockActiveNpc` lives there).

## 5. Tests

### 5.1 Handler tests (new in `pkg/script/handlers_test.go`, adjacent to the existing `TestPDelay*` cluster at lines 380-410)

All tests use the established `Init(sf, mp, protect, nil, nil)` + `Execute(state)`
shape from the `TestPDelay*` precedents. Tick wiring uses `mockWorld` —
`ScriptState.World` field is populated via the `Init` path; tests that need a
specific tick set `mw.tick` on the world stored in `state.World` (see
`handlers_vars_test.go:29` for the `mockWorld.CurrentTick()` shape).

**`TestPArriveDelaySuspendsWhenMovedThisTick`**

Setup: `mp.lastMovement = 101`, world tick `T = 100`. Gate: `101 < 100` is
false ⇒ suspend.

Run via `Init(sf, mp, true, nil, nil)` + `Execute`. Assert:
- Error is nil.
- `mp.setDelayedCalls == [0]` (one call, ticks=0).
- `state.Execution == Suspended`.

**`TestPArriveDelaySuspendsWhenMovedLastTick`**

Setup: `mp.lastMovement = 100`, `T = 100`. Gate: `100 < 100` is false ⇒
suspend. Same assertions as above. Pins the boundary semantic of the 2-tick
window.

**`TestPArriveDelayNoOpWhenMovedTwoTicksAgo`**

Setup: `mp.lastMovement = 99`, `T = 100`. Gate: `99 < 100` is true ⇒ no-op.
Assert:
- Error is nil.
- `mp.setDelayedCalls` length is 0.
- `state.Execution == Running` (unchanged from `Init`'s initial state — see
  state.go for the post-`Init` Execution value).

**`TestPArriveDelayNoOpWhenNeverMoved`**

Setup: `mp.lastMovement = 0` (zero-value), `T = 100`. Same gate / assertions
as the previous test. Pins zero-value behaviour.

**`TestPArriveDelayUnprotectedRejected`**

Mirror `TestPDelayUnprotectedRejected` at handlers_test.go:380. Setup:
`mp = &mockPlayer{}`, `Init(..., protect=false, ...)`, then `Execute`.
Assert error message exactly `"P_ARRIVEDELAY: script not protected"` and
`mp.setDelayedCalls` length is 0 (rejection must not mutate, per the
invariant pinned at handlers_test.go:806-808 for P_DELAY).

**`TestPArriveDelayRequiresActivePlayer`**

Mirror `TestPDelayRequiresActivePlayer` at handlers_test.go:392. Setup:
`Init(sf, nil, false, nil, nil)` (no Self). Assert error is non-nil and
matches the `requireProtectedActivePlayer` no-Self error message convention
(check handlers.go for the exact format string at NAI-82-implementation
time — implementer pins exact-match string against handler source).

### 5.2 Player write-site test (extend `modules/world/movement_test.go`)

**`TestResolveMovementWritesLastMovementOnStep`**

Construct a player with a wired `client.server`, seed `s.currentTick = 50`,
queue a single waypoint adjacent to the player so one step succeeds, call
`p.resolveMovement()`. Assert:
- `p.stepsTaken == 1` (sanity — pre-existing invariant).
- `p.lastMovement == 51` (= `currentTick + 1`).

**`TestResolveMovementSkipsLastMovementWhenIdle`**

Same setup but no waypoint queued (`p.waypointIndex = -1`). Call
`resolveMovement`. Assert:
- `p.stepsTaken == 0`.
- `p.lastMovement == 0` (unchanged from zero-value).

### 5.3 NPC write-site test (extend `modules/world/npc_interaction_test.go` or a new `npc_movement_test.go`)

**`TestNpcUpdateMovementWritesLastMovementOnStep`**

Construct an Npc + Server with `s.currentTick = 50`, wire `s.gamemap` so
`CanTravel` permits the step, snapshot `n.lastTickX/Z = n.x/n.z`, queue a
waypoint, call `n.updateMovement(s)`. Assert:
- Returns true.
- `n.x` or `n.z` advanced by one tile.
- `n.lastMovement == 51`.

**`TestNpcUpdateMovementSkipsLastMovementWhenStationary`**

Same setup but `n.waypointIndex = -1`. Call `n.updateMovement(s)`. Assert:
- Returns false.
- `n.lastMovement == 0` (unchanged).

### 5.4 Pre-flight verification (confirmed against HEAD `ab37799` at brainstorm time)

- `requireProtectedActivePlayer(s, op string) error` — verified
  precedent at `pkg/script/handlers.go:660` (`handlePDelay`).
- `s.Self.SetDelayed(n)` is the call shape used by `handlePDelay`
  (handlers.go:667). `SetDelayed(0)` ⇒ `delayedUntil = currentTick + 1` per
  the docstring at `pkg/script/active.go:10-12`.
- `s.World.CurrentTick()` is the established read pattern; consumers at
  `handlers_server.go:11`, `handlers_player.go:907`, `handlers_player.go:930`.
- `mockPlayer.setDelayedCalls` field shape verified at
  `pkg/script/runner_test.go:323-326`.
- `*Player.client.server.currentTick` access pattern verified at
  `modules/world/player_timer.go:29` and movement.go:84 (defensive nil-guard
  convention).
- `(n *Npc).updateMovement(s *Server)` signature already takes server —
  direct `s.currentTick` works (npc_interaction.go:279).
- `n.lastTickX/Z` are pre-snapshotted at `processMovementInteraction:162`
  (production tick path); test path at `npc_reorient_test.go:85` uses bare
  `npc.updateMovement` — `s != nil` guard makes the write a no-op there.
- No existing `lastMovement` field/identifier in goscape — verified by
  `grep -rn "lastMovement\|LastMovement" /home/owner/Code/github.com/zsrv/goscape/`
  returning zero matches.

## 6. TS-fidelity ledger

| TS construct | goscape mapping | Divergence? |
|---|---|---|
| `checkedHandler(ProtectedActivePlayer, ...)` | `requireProtectedActivePlayer(s, "P_ARRIVEDELAY")` | No — established sibling pattern (`handlePDelay`) |
| `state.activePlayer.lastMovement < World.currentTick` | `s.Self.LastMovement() < s.World.CurrentTick()` | No — direct port |
| `state.activePlayer.delayed = true` + `delayedUntil = currentTick + 1` | `s.Self.SetDelayed(0)` (impl computes `currentTick + 1 + 0`) | No — semantic-equivalent encapsulation, same as `handlePDelay` |
| `state.execution = SUSPENDED` | `s.Execution = Suspended` | No — direct port |
| `Player.lastMovement = currentTick + 1` after `stepsTaken > 0` | `p.lastMovement = currentTick + 1` at tail of `resolveMovement` when `stepsTaken > 0` | No — same condition, same arithmetic; `client != nil && server != nil` guard is goscape-only defensive parity (TS doesn't have a no-server fixture path) |
| `Npc.lastMovement = currentTick + 1` after `lastTickX != x || lastTickZ != z` | `n.lastMovement = s.currentTick + 1` at tail of `updateMovement(s)` when `n.x != n.lastTickX || n.z != n.lastTickZ` | No — direct port; `s != nil` guard is goscape-only defensive parity |
| `Npc.wanderCounter = 0` inside `if (moved)` block | (not modified by NAI-82) | **Pre-existing divergence — tracked as NAI-82-D1; goscape resets unconditionally at `aiMode` entry, npc_interaction.go:218** |

### 6.1 NPC-side accessor deferral

The `ActiveNpc.LastMovement() int` accessor is intentionally **not** added in
NAI-82. The field is written to support the future port of `AI_ARRIVEDELAY` /
`AI_TARGETMOVED` (NpcOps.ts:543,548), neither of which is currently
surfacing in any smoke. When that sub-spec lands, only the interface method
+ `(n *Npc).LastMovement()` accessor + `mockNpc` conformance need to be
added; the field state is already correct.

### 6.2 NAI-82-D1 (deviation tracker entry)

| Field | Value |
|---|---|
| **TS site** | `Engine-TS/src/engine/entity/Npc.ts:365` |
| **TS behaviour** | `wanderCounter = 0` only when NPC moved this tick, inside `if (moved)` block in `updateMovement` |
| **goscape behaviour** | `wanderCounter = 0` unconditionally at start of `aiMode` (npc_interaction.go:218) |
| **Why deferred** | Pre-existing divergence; observable only in a narrow case (NPC enters aiMode, fails to step, retains a stale wanderCounter under TS but not under goscape — affects NPC random-wander cadence at most). Out of NAI-82's narrowly-scoped P_ARRIVEDELAY port. |
| **Closure path** | Either a future NPC-AI sub-spec that audits all `wanderCounter` write sites, or surface-driven (smoke evidence of mis-wandered NPCs near aiMode boundaries). |

## 7. Out of scope

- **`AI_ARRIVEDELAY` / `AI_TARGETMOVED`** (NpcOps.ts:543,548) — neither in
  any current smoke; deferred. NAI-82 lands the NPC-side write so they're
  cheap to port when surfaced.
- **NPC `wanderCounter` reset semantics** — see NAI-82-D1 above.
- **Other unported P_* opcodes** — none surfaced in NAI-80 smoke beyond
  P_ARRIVEDELAY itself.

## 8. Tech stack

Go 1.26+ (per `go_version.md`).

## 9. Smoke / cascade routing

Stub-not-completed port. Cadence-blocker is `go test ./...` green +
`go vet ./...` clean. Cascade attribution ("P_ARRIVEDELAY silenced" —
specifically, the `[oploc1, _bookcase]` script no longer logs
`no handler for P_ARRIVEDELAY`) confirmed at the next NAI-N+1 user-driven
smoke if Tutorial Island bookcase content is exercised.

If the next smoke surfaces a *different* opcode-handler-missing error, that
becomes NAI-N+1's cascade target. If smoke comes back clean of opcode-missing
errors, the `protocol_stub_not_completed` line of work is exhausted at NAI-82.

## 10. Close protocol

On NAI-82 close commit (per `close_commit_memory_trailer.md`):

```
Closes memory: nai81_seed_loc_coord_p_arrivedelay.md
```

The seed memory carried both NAI-81 (LOC_COORD) and NAI-82 (P_ARRIVEDELAY).
NAI-81's close referenced it; NAI-82's close retires it (delete the file from
the memory directory in the same close step, since both seeded items are
landed).

If a fresh memory needs to be saved at NAI-82 close (e.g. the field-without-
accessor pattern from §4.4 proves novel enough to be reusable), save under
its own filename per `MEMORY.md` index conventions.
