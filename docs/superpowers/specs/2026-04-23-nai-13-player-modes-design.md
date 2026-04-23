# NAI-13 — PLAYER* NPC Modes + `entitymask` Plumbing

Port the four PLAYER\* NPC modes (`PLAYERESCAPE`, `PLAYERFOLLOW`, `PLAYERFACE`,
`PLAYERFACECLOSE`) from TS `Npc.ts:746-830`, and wire the `entitymask`
construction-time assignment that makes `SetInteraction`'s `faceEntity` mask bit
fire correctly. Closes the NAI-11 PLAYER\* deferral in one slice, restoring the
NAI-11-memorialized flipped assertion in
`TestNpcTurnHuntAndConsumeSetsTarget`.

Part of the NPC AI tick decomposition roadmap
(`docs/superpowers/specs/2026-04-22-npc-ai-tick-decomposition-design.md`).
Blockers: NAI-11 (dispatch skeleton at
`modules/world/npc_interaction.go:176-184` routes all four PLAYER\* modes to
`resetDefaults()` as a scope stub; `targetWithinMaxRange` at
`modules/world/npc_interaction.go:397-447` has an explicit DEVIATION note
pointing at the PLAYERESCAPE retreat branch). Roadmap fidelity risk: **Medium**
— the PLAYERESCAPE algorithm has five subtle quirks (SW-distance abandon gate,
quadrant-direction tiebreakers, wall-flag pair per quadrant, axis-fallback
branching, corner-removal in maxrange); each is preserved exactly. See
`nai_followups.md` entry "Deferred: PLAYER\* mode implementations".

**Tech Stack:** Go 1.26+, existing `pkg/pathfinder/collision`
(`FlagMap.IsFlagged`, `FlagWallNorth/South/East/West`), existing
`pkg/coordgrid` (`DistanceToSW`, Chebyshev distance), existing
`modules/world` NPC infrastructure (`pathToTarget`, `queueWaypoint`,
`updateMovement`, `validateTarget`, `targetWithinMaxRange`, `resetDefaults`,
`SetInteraction`), existing `pkg/rsbuf` (`NpcMaskFaceEntity`), existing
`pkg/objtype` NPC-mode constants
(`NPCModePlayerEscape/PlayerFollow/PlayerFace/PlayerFaceClose`,
`NpcType.MaxRange`).

## Goal

After NAI-13 ships:

1. `(*Npc).processMovementInteraction` at
   `modules/world/npc_interaction.go:176-184` dispatches each of the four
   PLAYER\* modes to its own method instead of routing them all to
   `resetDefaults()`. The stub DEVIATION comment at lines 141-143 is removed.
2. Four new per-tick mode methods live on `*Npc`:
   - `playerEscapeMode()` — TS `Npc.ts:746-799`
   - `playerFollowMode()` — TS `Npc.ts:801-812`
   - `playerFaceMode()`   — TS `Npc.ts:815-819`
   - `playerFaceCloseMode()` — TS `Npc.ts:821-829`
3. `NewNpc` at `modules/world/npc.go:108` sets `n.entitymask =
   rsbuf.NpcMaskFaceEntity`, mirroring TS `PathingEntity.ts:107`. This makes
   the previously-no-op `n.masks |= n.entitymask` lines in
   `SetInteraction` and (newly added) `resetDefaults` actually emit the
   face-entity mask bit.
4. `(*Npc).resetDefaults` at `modules/world/npc_interaction.go:36` adds
   `n.masks |= n.entitymask` mirroring TS `Npc.ts:416`.
5. `(*Npc).targetWithinMaxRange` at
   `modules/world/npc_interaction.go:397-447` gains two new branches: a
   PLAYERFOLLOW early-return of `true` (TS `Npc.ts:633-635`) and a
   PLAYERESCAPE retreat-maxrange branch with the corner-removal quirk (TS
   `Npc.ts:657-673`). The DEVIATION note at lines 402-404 is removed.
6. The NAI-11 flipped-test memorial at `npc_hunt_test.go`
   `TestNpcTurnHuntAndConsumeSetsTarget` is restored: the assertion flips
   from `n.target == nil` back to `n.target == huntedNpc`. The inline
   forwarding comment pointing at `nai_followups.md` is removed.
7. ~22-25 new unit tests assert each mode's behavior + guard the five
   PLAYERESCAPE quirks explicitly.
8. Memory entry `nai_followups.md` § "From NAI-11 (2026-04-22) →
   Deferred: PLAYER\* mode implementations" is updated to "Resolved
   2026-04-23 (NAI-13)" with the resolution pointer.

## Scope — what's IN

### Mask plumbing (3 sites)

1. **`NewNpc` entitymask assignment** at `modules/world/npc.go:108`. Adds
   `n.entitymask = rsbuf.NpcMaskFaceEntity` at the tail of the constructor.
   Single line. Mirrors TS `PathingEntity.ts:107`.
2. **`resetDefaults` mask emission** at
   `modules/world/npc_interaction.go:36`. Adds `n.masks |= n.entitymask`
   after the existing `n.faceEntity = -1` line. Single line. Mirrors TS
   `Npc.ts:416`.
3. **`SetInteraction` mask emission activation** (no code change). The
   existing `n.masks |= n.entitymask` line becomes functional by virtue of
   site 1. Verified via the new `TestSetInteractionEmitsEntityMask` test.

### Dispatch wiring (1 site)

4. **`processMovementInteraction` switch** at
   `modules/world/npc_interaction.go:176-184`. Expand the single fall-through
   `n.resetDefaults()` stub into four individual cases, each calling the
   corresponding `playerXxxMode()` method. Remove the DEVIATION comment at
   lines 141-143.

### `targetWithinMaxRange` branches (1 site)

5. **Two new branches** at `modules/world/npc_interaction.go:397-447`. Add a
   PLAYERFOLLOW early-return immediately after the `n.typ == nil` guard (TS
   `Npc.ts:633-635`), and a PLAYERESCAPE branch mirroring the OP-trigger
   branch shape (Chebyshev `maxAxis > maxrange + 1` + corner-removal quirk)
   per TS `Npc.ts:657-673`. Remove the DEVIATION note at lines 402-404.

### Four PLAYER\* mode methods (1 new file)

6. **`modules/world/npc_player_modes.go`** (new file). Hosts the four methods:

   **`playerEscapeMode()`** — TS `Npc.ts:746-799`. ~55 Go LOC.
   - Type guard: if `n.target` is not `*Player`, log and return (TS throws; Go
     logs + returns per project Go-idiom convention, tracked as a deviation
     from the TS throw-and-abort contract).
   - Abandon gate: `coordgrid.DistanceToSW(n.x, n.z, tx, tz) > 25` →
     `resetDefaults()`; return.
   - Quadrant pick: one of four SW/NW/SE/NE directions based on the sign of
     `(tx - n.x, tz - n.z)`. Each direction carries a specific wall-flag pair.
   - Candidate tile: `mx = n.x + direction.dx`, `mz = n.z + direction.dz`.
   - Wall check: `s.gamemap.Pathfinder.FlagMap.IsFlagged(mx, mz, n.level,
     flagPair)` → `resetDefaults()`; return.
   - Maxrange check: if `coordgrid.DistanceToSW(mx, mz, n.startX, n.startZ) <
     n.typ.MaxRange` → `queueWaypoint(mx, mz)`; `updateMovement()`; return.
   - Axis fallback: for NE/NW → `queueWaypoint(n.x, mz)`; for SE/SW →
     `queueWaypoint(mx, n.z)`; then `updateMovement()`.

   **`playerFollowMode()`** — TS `Npc.ts:801-812`. ~10 Go LOC.
   - Type guard: non-Player → log and return.
   - `n.pathToTarget()`; `n.updateMovement()`.

   **`playerFaceMode()`** — TS `Npc.ts:815-819`. ~5 Go LOC.
   - Type guard only. No body. The face-entity bit is already emitted by
     `SetInteraction`'s `n.masks |= n.entitymask` (now functional per site 1).

   **`playerFaceCloseMode()`** — TS `Npc.ts:821-829`. ~10 Go LOC.
   - Type guard: non-Player → log and return.
   - If Chebyshev distance to target > 1 → `resetDefaults()`; return.

### NAI-11 flipped-test restoration (1 site)

7. **`TestNpcTurnHuntAndConsumeSetsTarget`** at
   `modules/world/npc_hunt_test.go`. Flip the assertion from `n.target ==
   nil` back to `n.target == huntedNpc` (the original pre-NAI-11 shape). The
   NAI-11-memorialized inline comment pointing at `nai_followups.md` is
   removed. This is the concrete visible-regression repair proving PLAYERFOLLOW
   dispatch works.

## Scope — what's OUT (non-goals)

1. **Size-aware `inApproachDistance` LoS**. Tracked as the NAI-12 deferral
   ("From NAI-12 — Deferred: size-aware inApproachDistance LoS" in
   `nai_followups.md`). Orthogonal: PLAYER\* modes do not call
   `inApproachDistance`.
2. **SMART pathfinding branch in `pathToTarget`**. Tracked as NAI-11
   deferral. PLAYERFOLLOW uses the naive branch (queues a waypoint at the
   target's current tile) — inherits the existing DEVIATION at
   `npc_interaction.go:332`. No new deferral entry is created; NAI-13 is
   not the closer for that deferral.
3. **Reach helpers (`reachedEntity` / `reachedLoc` / `reachedObj`)**. Tracked
   as NAI-11 deferral. Not touched; PLAYER\* modes don't dispatch through
   reach helpers.
4. **`focus()` `instant` flag wire-protocol**. Tracked as NAI-11 deferral.
   Face modes rely on the `entitymask`-emitted faceEntity bit, not on
   `focus()` / `faceAngleX/Z` plumbing.
5. **Other `PathingEntity.ts` `entitymask` sites** (TS lines 534, 540, 612).
   Per the Q3 scope decision: those sites live inside TS
   `setInteraction` / `updateMovement` portions with known partial Go
   ports; touching them risks fidelity drift in unrelated directions. Land
   whenever the surrounding TS method gets a dedicated port sub-spec.
6. **`Npc.interacted` field**. Tracked as NAI-11 deferral. Not needed for
   PLAYER\* modes; the modes don't require a cross-tick "did we interact"
   flag.
7. **Hunt-filter backfill (`checkNotBusy`, `checkNotTooStrong`,
   `checkNotCombat`, `checkNotCombatSelf`, `checkVars`, `checkInv`)**.
   Tracked as NAI-8 deferrals. Orthogonal — hunt filters select a target;
   PLAYER\* modes act on an already-selected target.
8. **`NumberNotNull` opcode gate sweep**. Tracked as an unassigned fidelity
   audit. Orthogonal.

## Architecture

### File additions

- **`modules/world/npc_player_modes.go`** (new). Four `(*Npc).playerXxxMode`
  methods + a four-value `escapeDirection` block (local to the file) carrying
  `(dx, dz, fallbackAxis, wallFlagPair)` tuples. No new exports. No new
  types escape the package. Estimated ~110 LOC.

- **`modules/world/npc_player_modes_test.go`** (new). All per-mode unit
  tests + quirk guards. Reuses existing fixtures (`newTestServer`,
  `newTestNpc`, `newTestPlayer`, wall-flag seeding helpers from NAI-12).
  Estimated ~300 LOC.

### File modifications

- **`modules/world/npc.go`** — 1 line added in `NewNpc` (entitymask
  assignment).
- **`modules/world/npc_interaction.go`**:
  - 1 line added in `resetDefaults` (mask emission).
  - ~15 line delta in `processMovementInteraction` (switch expansion +
    DEVIATION comment removal).
  - ~20 line delta in `targetWithinMaxRange` (two new branches + DEVIATION
    comment removal).
- **`modules/world/npc_hunt_test.go`** — 1 test assertion flipped + NAI-11
  memorial comment removed.
- **`modules/world/npc_masks_test.go`** — 1 new test for the mask emission
  from `resetDefaults`. (Alternative location: `npc_test.go` — pick
  whichever already has the closer NPC-construction fixture surface.)

### Type additions — none

- `n.entitymask` already exists on `*Npc` (referenced by `SetInteraction`'s
  no-op line today).
- `n.faceSquareX/Z` already exist (NAI-11 scaffolding).
- `pathToTarget()`, `queueWaypoint()`, `updateMovement()`, `resetDefaults()`,
  `validateTarget()`, `targetWithinMaxRange()` all exist from prior NAI
  sub-specs.
- The `escapeDirection` struct is local to `npc_player_modes.go` — a
  file-scoped literal table, not exported.

### Helpers consumed (all exist)

- `(*collision.FlagMap).IsFlagged(x, z, level, flags int) bool` at
  `pkg/pathfinder/collision/flagmap.go:146`.
- `collision.FlagWallNorth / FlagWallSouth / FlagWallEast / FlagWallWest` at
  `pkg/pathfinder/collision/flag.go:9-16`.
- `coordgrid.DistanceToSW(x1, z1, x2, z2 int) int` (used at
  `npc_interaction.go:439`).
- `n.typ.MaxRange` (`objtype.NpcType.MaxRange`).
- `rsbuf.NpcMaskFaceEntity` (already used at `npc_masks.go:36`).

### Chebyshev distance helper (audit)

TS `playerFaceCloseMode` uses `CoordGrid.distanceTo` (Chebyshev); TS
`playerEscapeMode`'s abandon gate uses `CoordGrid.distanceToSW`. The Go side
has `coordgrid.DistanceToSW`; if a `coordgrid.DistanceToChebyshev` (or
equivalent name — `DistanceTo`, `ChebyshevDistance`) helper exists, reuse it.
If it doesn't exist, inline the calculation as `max(abs(dx), abs(dz))` in
`playerFaceCloseMode` — a one-liner not worth promoting to a new
package-level helper until a second consumer appears.

## Error handling

### Type-guard mismatch (`*Player` expected, something else received)

TS throws `new Error('[Npc] Target must be a Player for playerXxx mode.')`
— a hard crash in that tick, caller recovers at the tick-loop boundary.

Go cannot follow the throw-and-recover pattern without restructuring the
tick loop. NAI-13 takes the project's established Go-idiom posture: each
mode method type-guards via a type-switch on `n.target.(type)`. On a
non-Player branch:

1. Emit a structured `slog.Warn` with `npc_id`, `target_kind`, `mode`.
2. Return without mutating state.

This is a minor deviation from TS (which aborts the tick entirely on
mismatch). Tracked as a DEVIATION comment on each of the four methods. The
divergence is defensive-only: `validateTarget` + `targetWithinMaxRange` +
the `consumeHuntTarget` / `SetInteraction` call sites only ever route
Player targets into the PLAYER\* modes under correct operation. A non-Player
reaching a PLAYER\* mode is a state-corruption bug, not a recoverable
runtime case — logging preserves forward progress; throwing would stall the
whole tick.

### `n.target == nil` guard

Already handled by `processMovementInteraction`'s prelude (line 170). PLAYER\*
methods do NOT re-check for nil — mirrors TS structure (the modes assume
prelude-validated presence).

### `n.typ == nil` guard

Existing pattern in `targetWithinMaxRange` (line 409-411) — preserved. The
PLAYERESCAPE retreat branch also needs `n.typ != nil` to read `MaxRange`; the
existing typ guard protects both new branches.

### `s.gamemap == nil` guard (test fixture compatibility)

Matches NAI-12 convention: `playerEscapeMode` must check
`n.server.gamemap != nil` before dereferencing `FlagMap`. On nil, treat the
wall-flag check as "pass" (`!IsFlagged` equivalent). This keeps tests that
don't wire a gamemap green — same pattern NAI-12 Task 5 established for
`inApproachDistance`.

## Data flow

### Per-tick dispatch path

```
Npc.turn()
  └─ processMovementInteraction(s)           // npc_interaction.go:144
       ├─ [delayed / dead bail]
       ├─ [bookkeeping: lastTickX/Z/level, tele = false]
       ├─ [null-targetOp failsafe → defaultMode]
       ├─ [targetless: none / wander / patrol]
       ├─ [targeted prelude: target == nil || !validateTarget() → resetDefaults]
       └─ switch targetOp:
            ├─ NPCModePlayerEscape     → playerEscapeMode()
            ├─ NPCModePlayerFollow     → playerFollowMode()
            ├─ NPCModePlayerFace       → playerFaceMode()
            ├─ NPCModePlayerFaceClose  → playerFaceCloseMode()
            └─ default                  → aiMode(s)
```

### Mask emission path (face modes rely on this)

```
[target-setter, e.g. consumeHuntTarget]
  └─ n.SetInteraction(target, op)
       ├─ n.target     = target
       ├─ n.targetOp   = op
       ├─ n.faceEntity = target.pid/nid   (existing branches)
       └─ n.masks     |= n.entitymask     (existing line — becomes functional)

[on abandon / invalidation]
  └─ n.resetDefaults()
       ├─ n.target     = nil
       ├─ n.targetOp   = defaultMode
       ├─ n.faceEntity = -1
       └─ n.masks     |= n.entitymask     (NEW — mirrors TS Npc.ts:416)
```

On the wire side, `rsbuf.EncodeNpc` reads `n.masks` and `n.faceEntity` and
emits the face-entity info-block update. **No rsbuf change required** — the
mask bit and field are already consumed by the encoder. NAI-13 just makes
the mask-set lines fire correctly by virtue of `n.entitymask` being
non-zero at construction.

### PLAYERESCAPE quadrant table

| Target position rel. NPC | Direction | `dx, dz` | Wall flags for candidate tile |
|---|---|---|---|
| `tx >= nx && tz >= nz` (SW of NPC) | SOUTH_WEST | `-1, -1` | `FlagWallSouth \| FlagWallWest` |
| `tx >= nx && tz <  nz` (NW of NPC) | NORTH_WEST | `-1, +1` | `FlagWallNorth \| FlagWallWest` |
| `tx <  nx && tz >= nz` (SE of NPC) | SOUTH_EAST | `+1, -1` | `FlagWallSouth \| FlagWallEast` |
| `tx <  nx && tz <  nz` (NE of NPC) | NORTH_EAST | `+1, +1` | `FlagWallNorth \| FlagWallEast` |

Axis fallback: NE/NW → `(n.x, mz)` (Z-axis only); SE/SW → `(mx, n.z)`
(X-axis only).

### `targetWithinMaxRange` new structure

```go
func (n *Npc) targetWithinMaxRange() bool {
    if n.target == nil { return true }
    if n.typ == nil    { return false }

    // NEW (TS :633-635): PLAYERFOLLOW has no maxrange bound.
    if n.targetOp == objtype.NPCModePlayerFollow {
        return true
    }

    maxrng := int(n.typ.MaxRange)
    attackrng := int(n.typ.AttackRange)

    tx, tz, _ := n.target.Coords()
    dx := abs(tx - n.startX)
    dz := abs(tz - n.startZ)

    // NEW (TS :657-673): PLAYERESCAPE retreat maxrange.
    if n.targetOp == objtype.NPCModePlayerEscape {
        maxAxis := max(dx, dz)
        if maxAxis > maxrng+1            { return false }
        if dx == maxrng+1 && dz == maxrng+1 { return false }
        return true
    }

    switch {
    case checkOpTrigger(n.targetOp):  /* unchanged */
    case checkApTrigger(n.targetOp):  /* unchanged */
    default:                          /* unchanged */
    }
}
```

## Testing strategy

### Test file layout

- **`modules/world/npc_player_modes_test.go`** (new) — per-mode behavior +
  quirk guards + `targetWithinMaxRange` new branches.
- **`modules/world/npc_masks_test.go`** or **`npc_test.go`** — mask
  plumbing tests (pick the file that already has the closer NPC-construction
  fixture surface).
- **`modules/world/npc_hunt_test.go`** — flip one existing assertion in
  `TestNpcTurnHuntAndConsumeSetsTarget`.

### Test inventory (25 tests: 24 new + 1 modified)

**Mask plumbing (3 new tests)**

- `TestNewNpcSetsEntityMaskToFaceEntity` — `NewNpc(...).entitymask ==
  rsbuf.NpcMaskFaceEntity`.
- `TestResetDefaultsEmitsEntityMask` — post-`resetDefaults`,
  `n.masks & rsbuf.NpcMaskFaceEntity != 0`.
- `TestSetInteractionEmitsEntityMask` — post-`SetInteraction`,
  `n.masks & rsbuf.NpcMaskFaceEntity != 0`. Proves the previously-no-op line
  now fires.

**Dispatch routing (4 new tests)**

- `TestProcessMovementInteractionDispatchPlayerEscape` — targetOp =
  PlayerEscape, target player 26+ tiles SW → `n.target == nil` (abandon gate
  fired, proving method reached).
- `TestProcessMovementInteractionDispatchPlayerFollow` — targetOp =
  PlayerFollow, player at (5,5), NPC at (0,0) → `n.waypoints` contains
  (5,5).
- `TestProcessMovementInteractionDispatchPlayerFace` — targetOp =
  PlayerFace → `n.target != nil` AND `n.masks & NpcMaskFaceEntity != 0`.
- `TestProcessMovementInteractionDispatchPlayerFaceClose` — targetOp =
  PlayerFaceClose, player 3 tiles away → `n.target == nil` (distance gate
  fired).

**`playerEscapeMode` (10 new tests)**

- `TestPlayerEscapeTargetSW_FleesNE` — target at `(nx+1, nz+1)` → waypoint
  at `(nx-1, nz-1)`. Proves SW-quadrant direction pick.
- `TestPlayerEscapeTargetNW_FleesSE` — `(nx+1, nz-1)` → `(nx-1, nz+1)`.
- `TestPlayerEscapeTargetSE_FleesNW` — `(nx-1, nz+1)` → `(nx+1, nz-1)`.
- `TestPlayerEscapeTargetNE_FleesSW` — `(nx-1, nz-1)` → `(nx+1, nz+1)`.
- `TestPlayerEscapeDistanceGateAbandons` — target SW-distance 26 →
  `resetDefaults`; no waypoint.
- `TestPlayerEscapeBlockedByWallResetsDefaults` — candidate tile pre-seeded
  with the matching wall-flag pair → `resetDefaults`; no waypoint.
- `TestPlayerEscapeBeyondMaxRangeNorthAxisFallback` — candidate outside
  `typ.MaxRange` + direction NE or NW → waypoint at `(n.x, mz)`.
- `TestPlayerEscapeBeyondMaxRangeSouthAxisFallback` — direction SE or SW
  outside MaxRange → waypoint at `(mx, n.z)`.
- `TestPlayerEscapeWithinMaxRangeQueuesDiagonal` — candidate within
  MaxRange → waypoint at `(mx, mz)` diagonal.
- `TestPlayerEscapeNonPlayerTargetLogsAndReturns` — target is `*Npc` → no
  state mutation, no waypoint. Verifies the log-and-return deviation from
  TS throw.

**`playerFollowMode` (3 new tests)**

- `TestPlayerFollowQueuesWaypointAtTarget` — player at (10,10), NPC at
  (0,0) → `n.waypoints` contains (10,10).
- `TestPlayerFollowAdvancesOneTile` — after mode fires, NPC is one tile
  closer. Proves `updateMovement()` actually runs (separates
  pathToTarget-only from pathToTarget+move bugs).
- `TestPlayerFollowNonPlayerTargetLogsAndReturns` — target is `*Npc` →
  no-op.

**`playerFaceMode` (2 new tests)**

- `TestPlayerFaceMethodIsNoop` — snapshot NPC state before/after call →
  identical (no waypoint, no position delta). Specifically DOES NOT assert
  on `masks` inside this method — the mask was already set by
  `SetInteraction`.
- `TestPlayerFaceNonPlayerTargetLogsAndReturns` — log-and-return on wrong
  type.

**`playerFaceCloseMode` (3 new tests)**

- `TestPlayerFaceCloseWithinRangeNoops` — player 1 tile away (Chebyshev)
  → state unchanged, no waypoint, target preserved.
- `TestPlayerFaceCloseBeyondRangeResetsDefaults` — player 2 tiles away →
  `n.target == nil`, `n.targetOp == defaultMode`.
- `TestPlayerFaceCloseUsesChebyshevNotSWDistance` — player at `(+2, 0)`
  (SW-distance 1, Chebyshev 2) → resets. Quirk-guard separating this from
  `playerEscape`'s `distanceToSW` abandon gate.

**`targetWithinMaxRange` additions (4 new tests)**

- `TestTargetWithinMaxRangePlayerFollowAlwaysTrue` — player 100 tiles from
  start + `typ.MaxRange = 2` → still true.
- `TestTargetWithinMaxRangePlayerEscapeRetreatBound` — PLAYERESCAPE, target
  at `maxrange+1` on one axis → true; target at `maxrange+2` → false.
- `TestTargetWithinMaxRangePlayerEscapeCornerQuirk` — PLAYERESCAPE, target
  at `(maxrange+1, maxrange+1)` → false (corner-removal, mirrors OP-trigger
  branch at lines 432-434).
- `TestTargetWithinMaxRangeOpTriggerUnchanged` — regression guard: existing
  OP / AP / default branches behave identically.

**NAI-11 flipped-test restoration (1 modified test)**

- `TestNpcTurnHuntAndConsumeSetsTarget` at
  `modules/world/npc_hunt_test.go` — assertion flipped from `n.target ==
  nil` back to `n.target == huntedNpc`. Inline forwarding comment pointing
  at `nai_followups.md` removed.

### Fixtures

All reuse NAI-11 / NAI-12 patterns. No new mocks.

- `newTestServer(t)` — `*Server` with a valid `gamemap.Pathfinder` and
  `FlagMap`.
- `newTestNpc(t, server, typ, x, z, level)` — NPC construction.
- `newTestPlayer(t, server, x, z, level)` — Player target.
- Wall-flag seeding — NAI-12 Task 1 introduced `withBlockingWall`
  (`modules/world/npc_hunt_entities_test.go` scope). If it's reusable
  directly: reuse. If it's too opinionated about flag composition: add a
  small `withWallFlag(m, x, z, level, flag int)` helper local to
  `npc_player_modes_test.go`.

### Plan ordering

The plan doc should sequence tasks as:

1. Mask plumbing (`NewNpc` entitymask + `resetDefaults` emit) — smallest.
   Unblocks face-mode tests.
2. Dispatch wiring (`processMovementInteraction` switch expansion) —
   smallest production change. Unblocks per-mode dispatch tests.
3. `playerFaceMode` — trivial, validates mask-plumbing end-to-end.
4. `playerFaceCloseMode` — trivial + Chebyshev quirk test.
5. `targetWithinMaxRange` new branches — unblocks movement modes'
   maxrange tests.
6. `playerFollowMode` — simplest movement; lands the NAI-11 flipped-test
   restoration as part of this task.
7. `playerEscapeMode` — largest. All five quirk-guard tests land here.
8. NAI-13 close commit: `nai_followups.md` memory update + final verification
   sweep.

## Files changed

**New:**
- `modules/world/npc_player_modes.go` (~110 LOC production)
- `modules/world/npc_player_modes_test.go` (~300 LOC test)

**Modified:**
- `modules/world/npc.go` (+1 line — entitymask assignment in `NewNpc`)
- `modules/world/npc_interaction.go` (~40 line delta — `resetDefaults`,
  `processMovementInteraction`, `targetWithinMaxRange`)
- `modules/world/npc_hunt_test.go` (~3 line delta — flipped assertion +
  removed NAI-11 comment)
- `modules/world/npc_masks_test.go` or `npc_test.go` (3 new tests, ~40 LOC)

**No changes:**
- `pkg/rsbuf` — mask bit already consumed by `EncodeNpc`.
- `pkg/pathfinder/collision` — `IsFlagged` + wall flags already in place.
- `pkg/coordgrid` — `DistanceToSW` already in use; Chebyshev inlined if
  no helper exists.
- `pkg/objtype` — NPC-mode constants already defined.
- `pkg/entity` — no new interface methods (size-aware LoS stays deferred).

## References

### TS source
- `Engine-TS/src/engine/entity/Npc.ts:416` — `masks |= entitymask` in TS
  `resetDefaults`.
- `Engine-TS/src/engine/entity/Npc.ts:592-599` — PLAYER\* dispatch switch.
- `Engine-TS/src/engine/entity/Npc.ts:633-635` — PLAYERFOLLOW maxrange
  early-return.
- `Engine-TS/src/engine/entity/Npc.ts:657-673` — PLAYERESCAPE retreat
  maxrange.
- `Engine-TS/src/engine/entity/Npc.ts:746-799` — `playerEscapeMode`.
- `Engine-TS/src/engine/entity/Npc.ts:801-812` — `playerFollowMode`.
- `Engine-TS/src/engine/entity/Npc.ts:815-819` — `playerFaceMode`.
- `Engine-TS/src/engine/entity/Npc.ts:821-829` — `playerFaceCloseMode`.
- `Engine-TS/src/engine/entity/PathingEntity.ts:107` — `entitymask`
  construction-time assignment on base class.

### Go code
- `modules/world/npc.go:108` — `NewNpc` constructor.
- `modules/world/npc_interaction.go:36` — `resetDefaults`.
- `modules/world/npc_interaction.go:144-185` — `processMovementInteraction`.
- `modules/world/npc_interaction.go:397-447` — `targetWithinMaxRange`.
- `modules/world/npc_masks.go:36` — existing `NpcMaskFaceEntity` consumer
  (`SetFaceEntity`).
- `modules/world/npc_hunt_test.go` — `TestNpcTurnHuntAndConsumeSetsTarget`
  (NAI-11 flipped assertion).
- `pkg/pathfinder/collision/flagmap.go:146` — `FlagMap.IsFlagged`.
- `pkg/pathfinder/collision/flag.go:9-16` — `FlagWallNorth/South/East/West`.

### Memory / prior specs
- `~/.claude/projects/-home-owner-Code-github-com-zsrv-goscape/memory/nai_followups.md`
  — § "From NAI-11 → Deferred: PLAYER\* mode implementations".
- `docs/superpowers/specs/2026-04-23-nai-12-checkvis-unified-design.md` —
  NAI-12 LoS unification (source of `withBlockingWall` test fixture
  pattern).
- `docs/superpowers/specs/2026-04-22-npc-ai-tick-decomposition-design.md` —
  NPC AI tick decomposition roadmap.
