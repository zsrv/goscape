# NAI-11 — NPC Movement-Interaction

Port TS `Npc.processMovementInteraction` (`Engine-TS/src/engine/entity/Npc.ts:562-603` +
helpers) and the full `PathingEntity.setInteraction` (`PathingEntity.ts:510-548`) into
the Go implementation. NPC paths toward `target`, fires `TriggerAiApNpc{1..5}` /
`TriggerAiOpNpc{1..5}` / `TriggerAiApPlayer{1..5}` / `TriggerAiOpPlayer{1..5}` /
`TriggerAiApLoc{1..5}` / `TriggerAiOpLoc{1..5}` / `TriggerAiApObj{1..5}` /
`TriggerAiOpObj{1..5}` per the AP/OP range-gating matrix. Also closes NAI-10's seven
deferred `setInteraction` fields by introducing the full method.

Part of the NPC AI tick decomposition roadmap
(`docs/superpowers/specs/2026-04-22-npc-ai-tick-decomposition-design.md`). Blockers:
NAI-2 (activeScript + `runNpcScript`), NAI-10 (consumeHuntTarget populates
`n.target` / `n.targetOp` — now migrates to call the new `SetInteraction`).
Roadmap fidelity risk: **High** — the AP/OP range-gating matrix + trigger-type
selection across 4 target categories × (AP, OP) × 5 ops, plus the `setInteraction`
side-effect closure, are where subtle divergences from TS hide.

**Tech Stack:** Go 1.26+, existing `pkg/script` (all `TriggerAi{Op,Ap}{Npc,Player,Loc,Obj}{1..5}`
already defined), existing `(*Server).runNpcScript` (NAI-2 — signature extended by this
spec), existing `pkg/coordgrid` (`PackCoord`/`UnpackCoord`/`Fine`), existing
`pkg/objtype.NpcType.AttackRange`/`.MaxRange`/`.GiveChase`/`.DefaultMode`.

## Goal

After NAI-11 ships:

1. `(*Npc).processMovementInteraction(s *Server)` exists and replaces `npc_ai.go:79-102`
   as the sole dispatcher for NPC movement-and-interaction in `Npc.turn()`.
2. The full TS `NpcMode` enum (NULL=-1 through QUEUE20=66) lives in `pkg/objtype/npctype.go`,
   superseding Go's ad-hoc `NpcModeNone`/`NpcModeWander`/`NpcModePatrol` constants in
   `modules/world/npc.go`. NAI-10's latent bug (TS-space `FindNewMode` written into a
   field whose switch used Go-ad-hoc values) closes.
3. `(*Npc).SetInteraction(kind InteractionKind, target entity, op, com int) bool` exists,
   mirrors player-side `(*Player).SetInteraction`, and closes all seven NAI-10 deferrals
   (apRange, apRangeCalled, targetSubject.com/typ, faceAngleX/Z, faceEntity + masks,
   targetX/Z, isValid pre-check).
4. `consumeHuntTarget`'s interaction branch migrates from direct field writes to
   `n.SetInteraction(InteractionScript, n.huntTarget, hunt.FindNewMode, -1)` — a
   one-line replacement that closes the NAI-10 DEVIATION block.
5. The AP/OP trigger matrix fires for **all four** target categories (Player, Npc, Loc,
   Obj) × (AP, OP) × 1..5 ops via 8 fire helpers in new file
   `modules/world/npc_interaction_trigger.go`.
6. NPCs with `moveSpeed == MoveSpeedRun` consume up to 2 waypoint steps per tick
   (`walkDir` from first step, `runDir` from second). Walk-only NPCs behave unchanged.
7. `entity` interface gains `IsValid() bool`. `*Npc`/`*Player`/`*Loc`/`*Obj` implement
   it. Zone-membership lifecycle checks stay as concrete `locStillValid`/`objStillValid`
   helpers in `modules/world` (module layering constraint).
8. `runNpcScript` signature extends to accept a generic `target entity` with internal
   type-dispatch of `ActivePlayer`/`ActiveNpc`/`ActiveLoc`/`ActiveObj` pointers on the
   `ScriptState`.

## Scope — what's IN

### Core dispatcher

1. **`(*Npc).processMovementInteraction(s *Server)`** in new file
   `modules/world/npc_interaction.go`. Entry point, called from `Npc.turn()`.
   Mirrors TS `Npc.ts:562-603`.

2. **Targetless mode dispatch** — `NONE`/`WANDER`/`PATROL` branches of the switch.
   Relocated `wanderMode`/`patrolMode` from `npc_ai.go`.

3. **Targeted-mode prelude** — `validateTarget` + `targetWithinMaxRange` + `resetDefaults`.

4. **`aiMode` branch** — the AP/OP trigger-firing path. TS `Npc.ts:832-858`.
   Calls `tryInteract` before AND after `updateMovement` (the "try twice" pattern).

5. **`updateMovement`** — replaces `advanceWaypoint`. Walk = 1 step; Run = 2 steps.
   Returns `moved bool` consumed by `aiMode` for the givechase clause.

### Full `setInteraction` port

6. **`(*Npc).SetInteraction(kind, target, op, com)`** — closes NAI-10 deferrals #1–#7.
   New fields on `*Npc`: `apRange`, `apRangeCalled`, `targetSubject npcTargetSubject`,
   `targetX`, `targetZ`, `faceAngleX`, `faceAngleZ`.

7. **`(*Npc).focus(fx, fz int, instant bool)`** — small helper mirroring TS
   `PathingEntity.focus`. Stores fine-grained coord; `instant` is stored write-only for
   a future wire-protocol divergence.

8. **`(*Npc).clearInteraction()`** — resets `target`/`targetOp`/`apRange`/`apRangeCalled`
   without touching `faceEntity`/`masks` (which are cleared by a subsequent mask-frame
   pass, not here — matches TS).

### AP/OP trigger matrix

9. **`(*Npc).tryInteract(s, allowOpScenery bool) bool`** — top-level AP/OP dispatcher.
   Mirrors TS `Npc.ts:861-883`.

10. **`checkApTrigger` / `checkOpTrigger`** — range checks against the `NPCMode` enum.
    TS `Npc.ts:1064-1080`.

11. **`(*Npc).inOperableDistance(target entity) bool`** — contact-range check. For
    PathingEntity targets (Player/Npc) uses Chebyshev<=1 excluding same tile (matches
    current `interaction.go:128-141`). For Loc/Obj: same Chebyshev; full `reachedLoc`/
    `reachedObj` integration is DEFERRED (tracked deviation).

12. **`(*Npc).inApproachDistance(range int, target entity) bool`** — AP-range check.
    Range is `n.typ.AttackRange` for NPC-side (fixed per-type, unlike player-side's
    mutable `apRange`). Matches `interaction.go:148-164` pattern; LoS DEFERRED.

13. **Eight fire helpers** in new file `modules/world/npc_interaction_trigger.go`:
    `fireAiApTriggerPlayer`, `fireAiOpTriggerPlayer`, `fireAiApTriggerNpc`,
    `fireAiOpTriggerNpc`, `fireAiApTriggerLoc`, `fireAiOpTriggerLoc`,
    `fireAiApTriggerObj`, `fireAiOpTriggerObj`. Two dispatchers
    (`fireAiApTrigger`, `fireAiOpTrigger`) type-switch on `n.target` and route.

14. **Eight mode→trigger map helpers**: `aiApNpcTriggerForOp`,
    `aiOpNpcTriggerForOp`, `aiApPlayerTriggerForOp`, `aiOpPlayerTriggerForOp`,
    `aiApLocTriggerForOp`, `aiOpLocTriggerForOp`, `aiApObjTriggerForOp`,
    `aiOpObjTriggerForOp`. Each returns the `ServerTriggerType` for a given
    `NpcMode`-space targetOp, or `0` if out-of-range.

### Constant unification

15. **Full `NpcMode` enum** in `pkg/objtype/npctype.go` — all 68 values
    (NULL=-1 through QUEUE20=66) per TS `NpcMode.ts`. Supersedes Go's ad-hoc
    `NpcModeNone`/`NpcModeWander`/`NpcModePatrol` in `modules/world/npc.go`.

16. **`NewNpc` + `npc_ai.go` + tests migrated** to the unified constants. Grep-and-list
    required for every referencing site (memory: *Enumerate ALL call sites when
    propagating through a shared file*).

### `runNpcScript` signature extension

17. **`runNpcScript(sf, n, target entity, req)`** — new signature. Type-switch on
    `target`: `*Player`→`ActivePlayer`+`PtrActivePlayer`; `*Npc`→`OtherActiveNpc`+
    `PtrOtherActiveNpc`; `*Loc`→`ActiveLoc`+`PtrActiveLoc`; `*Obj`→`ActiveObj`+
    `PtrActiveObj`; `nil`→no secondary pointer.

18. **`ScriptState` additions** if missing: `OtherActiveNpc *Npc` field +
    `PtrOtherActiveNpc` flag constant. `ActiveObj`/`PtrActiveObj` may already exist;
    verify during implementation.

### Entity interface expansion

19. **`IsValid() bool`** added to the `entity` interface in `modules/world/`.
    Concrete implementations:
    - `*Player.IsValid()` → `p.online && p.client != nil`
    - `*Npc.IsValid()` → `!n.dead`
    - `*entitypkg.Loc.IsValid()` → intrinsic check (no zone lookup); true for
      lifecycle-alive loc.
    - `*entitypkg.Obj.IsValid()` → intrinsic check; true for lifecycle-alive obj.

20. **`validateTarget` uses layered validity**:
    - `target.IsValid()` — intrinsic cheap check (in-interface).
    - For `*Loc`/`*Obj`: additionally call `locStillValid` / `objStillValid`
      (world-module helpers with zone-map access).

### Integration

21. **`npc_ai.go:79-102` replacement** — single call `n.processMovementInteraction(s)`.
    Old `advanceWaypoint`/`wanderMode`/`patrolMode` removed (migrate to `npc_interaction.go`).
    `queueWaypoint` stays in `npc_ai.go` as shared helper.

22. **`consumeHuntTarget` interaction-branch migration** to `SetInteraction`. Closes
    NAI-10's DEVIATION block.

23. **Tests** across the matrix — see "Test strategy" below.

## Scope — what's OUT (non-goals)

1. **PLAYERESCAPE / PLAYERFOLLOW / PLAYERFACE / PLAYERFACECLOSE modes.** No runtime
   content paths depend on these. The dispatcher switch lands a default case that calls
   `resetDefaults` for any PLAYER* targetOp — a one-line sentinel. Tracked deviation
   with follow-up.

2. **SMART pathfinding branch of `pathToTarget`.** Go uses NAIVE-only. TS's `findPath`/
   `findPathToEntity`/`findPathToLoc` are a massive orthogonal porting effort.
   Tracked deviation.

3. **Full `reachedEntity`/`reachedLoc`/`reachedObj` reach helpers.** NAI-11 uses
   Chebyshev distance only (matches current player-side behaviour at
   `interaction.go:128-164`). Loc shape/angle/forceapproach reach logic and Obj width/
   length reach logic are deferred. Tracked deviation.

4. **Line-of-sight (LoS) gating on AP checks.** TS `inApproachDistance` calls
   `isApproached` which walks the collision map. NAI-11 does range-only. Inherits
   player-side's S6l-D4 deviation posture. Tracked deviation.

5. **Run-direction wire-protocol wire-up.** `runDir` field exists on `*Npc` and is
   emitted by the mask path. NAI-11 writes it correctly from `updateMovement`'s
   second-step consumption. No protocol changes.

6. **`moveSpeed` mutation.** NAI-11 reads `n.moveSpeed` but does not set it — setting
   is scripted via handlers already in place.

7. **`interacted` tracking on `*Npc`.** TS has an `interacted` field on NPC tracking
   per-tick observability. NAI-11 does not need it (the try-twice pattern reads moved
   from updateMovement, not interacted). Defer until combat sub-specs surface a need.

8. **New opcode handlers** for `ai_aprange`, `p_aprange`-for-NPC, etc. NAI-11 stores
   `apRangeCalled` but no opcode sets it yet. Future opcode sub-spec wires.

9. **Any player-side interaction work.** Player-side `processInteraction` remains as-is.

## Architecture

### Call placement in `Npc.turn()`

Before NAI-11 (current HEAD after NAI-10):
```
[script prefix / events / isValid gate]
s.processNpcHunt(n)       // NAI-7..9
s.consumeHuntTarget(n)    // NAI-10
s.processNpcRegen(n)      // NAI-6
s.processNpcTimer(n)      // NAI-4
s.processNpcQueue(n)      // NAI-3
[moveRestrict gate]
[lastTick update + tele=false]
[waypoint advance OR wander/patrol + 500-tick teleport-to-spawn]
```

After NAI-11:
```
[script prefix / events / isValid gate]
s.processNpcHunt(n)
s.consumeHuntTarget(n)
s.processNpcRegen(n)
s.processNpcTimer(n)
s.processNpcQueue(n)
n.processMovementInteraction(s)   // NAI-11 — replaces lines 79-102
// (NAI-12 adds validateDistanceWalked after)
```

Exact TS line mapping (Npc.ts:110-185):
- `:112-118` → script prefix (NAI-2)
- `:121-151` → events (NAI-5)
- `:154` → isValid gate (NAI-5)
- `:158-171` → processHunt (NAI-7)
- `:174` → consumeHuntTarget (NAI-10)
- `:176` → processRegen (NAI-6)
- `:178` → processTimers (NAI-4)
- `:180` → processQueue (NAI-3)
- `:182` → processMovementInteraction (**NAI-11**)
- `:184` → validateDistanceWalked (NAI-12)

### `processMovementInteraction` control flow

```
1. Guard:
     return if n.delayed || n.dead

2. Failsafe (TS :568-571):
     if n.targetOp == NPCModeNull: n.targetOp = n.defaultMode()

3. Targetless-mode dispatch (TS :574-582):
     switch n.targetOp:
       NPCModeNone:   n.noMode(s);   return
       NPCModeWander: n.wanderMode(s); return
       NPCModePatrol: n.patrolMode(s); return

4. Targeted-mode prelude (TS :585-589):
     if n.target == nil || !n.validateTarget():
       n.resetDefaults(); return

5. Targeted-mode dispatch (TS :591-602):
     switch n.targetOp:
       PLAYERESCAPE | PLAYERFOLLOW | PLAYERFACE | PLAYERFACECLOSE:
         // DEVIATION: PLAYER* modes deferred — tracked
         n.resetDefaults()
       default:
         n.aiMode(s)
```

**`aiMode` flow** (TS :832-858):
```
1. n.wanderCounter = 0                        // reset teleport-to-spawn timer
2. if n.tryInteract(s, allowOpScenery=true): return   // OP or AP fire, pre-move
3. n.pathToTarget()                           // naive-only
4. moved := n.updateMovement()                // walk 1 step or run 2 steps
5. if moved && !n.typ.GiveChase:
     n.resetDefaults(); return                // clear interaction mid-chase
6. if n.target != nil:
     n.tryInteract(s, allowOpScenery=false)   // post-move retry (no scenery OP)
```

**`tryInteract` flow** (TS :861-883):
```
1. return false if n.target == nil || n.typ == nil
2. if checkOpTrigger(n.targetOp) && n.inOperableDistance(n.target):
     isPathing := n.target ∈ {*Player, *Npc}
     if isPathing || allowOpScenery:
       n.fireAiOpTrigger(s); return true
3. else if checkApTrigger(n.targetOp) && n.inApproachDistance(n.typ.AttackRange, n.target):
     n.fireAiApTrigger(s); return true
4. return false
```

### Mode→trigger dispatch matrix

| targetOp range | Category | AP/OP | Go trigger constant        | Go fire helper             |
|----------------|----------|-------|----------------------------|----------------------------|
| 7..11          | Player   | OP    | `TriggerAiOpPlayer1..5`    | `fireAiOpTriggerPlayer`    |
| 12..16         | Player   | AP    | `TriggerAiApPlayer1..5`    | `fireAiApTriggerPlayer`    |
| 17..21         | Loc      | OP    | `TriggerAiOpLoc1..5`       | `fireAiOpTriggerLoc`       |
| 22..26         | Loc      | AP    | `TriggerAiApLoc1..5`       | `fireAiApTriggerLoc`       |
| 27..31         | Obj      | OP    | `TriggerAiOpObj1..5`       | `fireAiOpTriggerObj`       |
| 32..36         | Obj      | AP    | `TriggerAiApObj1..5`       | `fireAiApTriggerObj`       |
| 37..41         | Npc      | OP    | `TriggerAiOpNpc1..5`       | `fireAiOpTriggerNpc`       |
| 42..46         | Npc      | AP    | `TriggerAiApNpc1..5`       | `fireAiApTriggerNpc`       |

Note TS NpcMode layout ordering: OP comes BEFORE AP in each category — verified against
`Engine-TS/src/engine/entity/NpcMode.ts:7-73`.

### `SetInteraction` full port

```
SetInteraction(kind InteractionKind, target entity, op, com int) bool:
1. if !target.IsValid(): return false             // deferral #7 closure
2. n.target         = target
3. n.targetOp       = op
4. n.apRange        = 10                          // deferral #1 closure
5. n.apRangeCalled  = false
6. n.targetSubject.com = com if com != 0 else -1  // TS "com ? com : -1" quirk
7. switch target.(type):                          // deferral #2 closure
     *Npc:  n.targetSubject.typ = t.typeId
     *Loc:  n.targetSubject.typ = t.Type()
     *Obj:  n.targetSubject.typ = t.Type()
     else:  n.targetSubject.typ = -1
8. focus with fine-grained coord (deferral #3 closure):
     tx, tz, _ := target.Coords()
     tw, tl := targetWidthLength(target)          // (1,1) for PathingEntity; LocType for Loc; (1,1) for Obj
     fx := coordgrid.Fine(tx, tw)
     fz := coordgrid.Fine(tz, tl)
     _, isNonPathing := target.(nonPathingEntity)
     n.focus(fx, fz, isNonPathing && kind == InteractionEngine)
9. faceEntity + masks + targetX/Z dispatch (deferrals #4, #5 closure):
     switch target.(type):
       *Player: slot := t.slot + 32768
                if n.faceEntity != slot:
                  n.faceEntity = slot
                  n.masks |= n.entitymask
       *Npc:    if n.faceEntity != t.nid:
                  n.faceEntity = t.nid
                  n.masks |= n.entitymask
       else:    n.targetX = coordgrid.Fine(tx, tw)
                n.targetZ = coordgrid.Fine(tz, tl)
10. return true
```

**`com == 0 → -1` quirk:** TS `com ? com : -1` treats 0 as falsy. Go port preserves this.
Inline comment documents.

### `updateMovement` walk/run

```
updateMovement() (moved bool):
1. if n.moveRestrict == MoveRestrictNoMove:
     n.walkDir = -1; n.runDir = -1; return false
2. if n.waypointIndex < 0: return false  // nothing to consume

3. stepOnce() returns (advanced bool, dir int):
     dest := coordgrid.UnpackCoord(n.waypoints[n.waypointIndex])
     dir := coordgrid.Face(n.x, n.z, dest.X, dest.Z)
     if dir == -1: n.waypointIndex--; return false, -1
     if !s.gamemap.CanTravel(n.level, n.x, n.z, coordgrid.DeltaX(dir), coordgrid.DeltaZ(dir)):
       n.waypointIndex = -1; return false, -1
     n.x += DeltaX(dir); n.z += DeltaZ(dir)
     n.stepsTaken++
     if n.x == dest.X && n.z == dest.Z: n.waypointIndex--
     return true, int(dir)

4. advanced1, dir1 := stepOnce()
   if !advanced1:
     n.walkDir = -1; n.runDir = -1; return false
   n.walkDir = dir1

5. if n.moveSpeed == MoveSpeedRun && n.waypointIndex >= 0:
     advanced2, dir2 := stepOnce()
     if advanced2:
       n.runDir = dir2
     else:
       n.runDir = -1
   else:
     n.runDir = -1

6. return true
```

**`lastTickX/Z` update + `tele = false`** happen at the start of
`processMovementInteraction` (before dispatch), matching the current `npc_ai.go:83-84`
placement. The targetless noMode/wanderMode/patrolMode branches also walk (they call
updateMovement), so the lastTick update belongs before the switch.

### `validateTarget` gates

```
validateTarget() bool:
1. tx, tz, tlevel := n.target.Coords()
   if tlevel != n.level: return false

2. if !n.targetWithinMaxRange(): return false

3. Type-changed check (TS :618):
     switch t := n.target.(type):
       *Npc: if n.targetSubject.typ != t.typeId: return false
       *Loc: if n.targetSubject.typ != t.Type():  return false
       (Obj/Player skip — Obj type is ephemeral; Player has no targetSubject.typ usage)

4. Lifecycle check (TS :622-626):
     switch t := n.target.(type):
       *Npc: return !t.dead && !t.delayed   // TS isActive = !dead && !delayed
       *Loc: return locStillValid(s, t, n.targetSubject.typ, tx, tz, tlevel)
       *Obj: return objStillValid(s, t, tx, tz, tlevel)
       *Player: return t.IsValid()
```

### `targetWithinMaxRange`

Three branches (PLAYERESCAPE dropped per scope decision; tracked deviation):

```
targetWithinMaxRange() bool:
1. if n.target == nil: return true

2. maxrange  := int(n.typ.MaxRange)
   attackrng := int(n.typ.AttackRange)
   tx, tz, _ := n.target.Coords()
   dx := abs(tx - n.startX)
   dz := abs(tz - n.startZ)

3. if checkOpTrigger(n.targetOp):
     // TS :640-648 — corner removal quirk
     if max(dx, dz) > maxrange+1: return false
     if dx == maxrange+1 && dz == maxrange+1: return false
     return true

4. else if checkApTrigger(n.targetOp):
     // TS :651-654 — SW-distance up to maxrange + attackrange
     if distanceToSW(tx, tz, n.startX, n.startZ) > maxrange + attackrng:
       return false
     return true

5. else:  // default (e.g. follow/face modes)
     // TS :676 — SW-distance up to maxrange + 1
     if distanceToSW(tx, tz, n.startX, n.startZ) > maxrange + 1:
       return false
     return true
```

`distanceToSW` mirrors TS `CoordGrid.distanceToSW` — Chebyshev max(dx, dz) of the
entity's SW corner to target's SW corner; for 1x1 entities this collapses to
`max(|tx-sx|, |tz-sz|)`. Helper lives in `pkg/coordgrid` (add if missing).

### `resetDefaults` + `defaultMode`

```
resetDefaults():
1. n.target   = nil
2. n.targetOp = n.defaultMode()
   // Does NOT clear apRange/apRangeCalled/faceEntity/masks — those are
   // stamped only by the NEXT SetInteraction call. Matches TS.

defaultMode() int:
  if len(n.typ.PatrolCoord) > 0: return NPCModePatrol
  if n.typ.WanderRange > 0:      return NPCModeWander
  return NPCModeNone
```

### Entity interface + validity layering

Current `entity` interface (in `modules/world/`) has `Coords()` + `Slot()`. NAI-11 adds:

```go
type entity interface {
    Coords() (x, z, level int)
    Slot() int
    IsValid() bool   // NAI-11
}
```

**Validity semantics by concrete type:**

| Type       | `IsValid()` body                          | Additional world-module check (if target is this type) |
|------------|-------------------------------------------|--------------------------------------------------------|
| `*Player`  | `p.online && p.client != nil`             | none                                                   |
| `*Npc`     | `!n.dead`                                 | `!n.delayed` (at validateTarget call site only)        |
| `*Loc`     | intrinsic: `true` (stateless, no dead flag)| `locStillValid(s, loc, typ, x, z, level)` (zone-membership) |
| `*Obj`     | intrinsic: `true`                         | `objStillValid(s, obj, x, z, level)` (zone-membership) |

**Why layered:** `*Loc`/`*Obj` live in `pkg/entity/` which cannot import
`modules/world` (module layering). Zone-membership checks need the world's zoneMap, so
they stay as world-module helpers. `IsValid()` on Loc/Obj is an intrinsic cheap check
(no server reference needed). `validateTarget`'s call site has the server pointer so it
adds the zone-lookup helper call as a separate gate.

`objStillValid` may need to be introduced; `locStillValid` already exists at
`modules/world/interaction_trigger.go:218-224`. If it's not already exported from its
file, refactor to share between player-side and NPC-side.

### `runNpcScript` signature extension

Current signature (NAI-2):
```go
func (s *Server) runNpcScript(sf *script.ScriptFile, n *Npc, activePlayer *Player, req *script.NpcQueueRequest)
```

New signature:
```go
func (s *Server) runNpcScript(sf *script.ScriptFile, n *Npc, target entity, req *script.NpcQueueRequest)
```

Internal type-dispatch sets `ScriptState` fields/flags based on `target` concrete type:

| target concrete type      | State field set       | Pointer flag            |
|---------------------------|-----------------------|-------------------------|
| `*Player`                 | `ActivePlayer`        | `PtrActivePlayer`       |
| `*Npc`                    | `OtherActiveNpc`      | `PtrOtherActiveNpc`     |
| `*entitypkg.Loc`          | `ActiveLoc`           | `PtrActiveLoc`          |
| `*entitypkg.Obj`          | `ActiveObj`           | `PtrActiveObj`          |
| `nil`                     | (none)                | (none)                  |

**Pre-existing callers** (established via grep pass; must be updated):
1. `processNpcTimer` (NAI-4) — currently passes `activePlayer=nil`. New signature:
   `target=nil`. Mechanical.
2. `processNpcQueue` (NAI-3) — same as timer. Mechanical.
3. `processNpcEventQueue` (NAI-5, ai_despawn) — same. Mechanical.
4. `consumeHuntTarget` QUEUE1..20 branch (NAI-10) — currently `activePlayer=nil`. New
   signature: `target=nil`. Mechanical.
5. `resumeOrFinishNpc` (NAI-2) — verify at implementation time. If it calls
   `runNpcScript`, migrate.

**Tests touching `runNpcScript` signature** — expected ~10-15 sites across
`npc_script_test.go`, `npc_timer_test.go`, `npc_queue_test.go`, `npc_event_queue_test.go`,
`npc_hunt_test.go`. Mechanical substitution. **Grep-and-list required in the plan.**

### `ScriptState` additions (if missing)

Verify during implementation:
- `OtherActiveNpc *Npc` field and `PtrOtherActiveNpc` flag constant — may need to be
  added. TS uses this for NPC-target AI triggers (`_other_activeNpc` in script opcode
  context).
- `ActiveObj *entitypkg.Obj` and `PtrActiveObj` — may already be partially wired from
  player-side. Verify before adding.

### File layout summary

**New files (2):**
- `modules/world/npc_interaction.go` (~320 LOC): `processMovementInteraction`,
  `noMode`, `wanderMode`, `patrolMode`, `aiMode`, `tryInteract`, `pathToTarget`,
  `updateMovement`, `validateTarget`, `targetWithinMaxRange`, `resetDefaults`,
  `defaultMode`, `SetInteraction`, `focus`, `clearInteraction`, `inOperableDistance`,
  `inApproachDistance`, `checkApTrigger`, `checkOpTrigger`, `targetWidthLength`,
  `nonPathingEntity` marker interface.
- `modules/world/npc_interaction_trigger.go` (~220 LOC): `fireAiOpTrigger` +
  `fireAiApTrigger` dispatchers; 8 fire helpers; 8 mode→trigger map helpers;
  `objStillValid` if not yet present.

**Modified files (~9):**
- `modules/world/npc_ai.go` — `turn()` lines 79-102 collapse; `advanceWaypoint`,
  `wanderMode`, `patrolMode` deleted (migrate to new file). `queueWaypoint` stays.
- `modules/world/npc.go` — new fields: `apRange`, `apRangeCalled`, `targetSubject`,
  `targetX`, `targetZ`, `faceAngleX`, `faceAngleZ`. Delete `NpcModeNone`/
  `NpcModeWander`/`NpcModePatrol`. `NewNpc` calls `defaultMode()`.
- `modules/world/npc_hunt.go` — `consumeHuntTarget` interaction branch → `SetInteraction`.
- `modules/world/interaction.go` — add `IsValid()` to `entity` interface.
- `modules/world/player.go` — add `IsValid()` method on `*Player`.
- `pkg/entity/loc.go` (or wherever Loc lives) — add `IsValid()` on `*Loc`.
- `pkg/entity/obj.go` (or wherever Obj lives) — add `IsValid()` on `*Obj`.
- `pkg/objtype/npctype.go` — full `NPCMode*` enum replacement.
- `pkg/script/npc_script.go` — `runNpcScript` signature extension + type-dispatch.
- `pkg/script/state.go` (or wherever `ScriptState` lives) — `OtherActiveNpc` +
  `PtrOtherActiveNpc` if missing; verify `ActiveObj`/`PtrActiveObj`.

**Test files modified or created (~5):**
- `modules/world/npc_interaction_test.go` — new, ~27 tests.
- `modules/world/npc_interaction_trigger_test.go` — new, ~16 tests + 8 helper tests.
- `modules/world/npc_hunt_test.go` — amended with 2 handoff integration tests.
- `pkg/script/npc_script_test.go` — amended for target-dispatch verification.
- All NAI-1..NAI-10 test files touching old `NpcMode*` constants or `runNpcScript`
  signature (grep-and-list during plan phase).

## Test strategy

Established S6/NAI pattern: table-driven for enumerable matrices, discrete unit tests
for each branch of a conditional, one integration test per cross-layer handoff.

### `modules/world/npc_interaction_test.go` — dispatcher + helpers (~29 test functions)

**`processMovementInteraction` dispatcher (7):**
1. `TestProcessMovementInteractionDelayedBails`
2. `TestProcessMovementInteractionDeadBails`
3. `TestProcessMovementInteractionNullTargetOpFallsToDefaultMode`
4. `TestProcessMovementInteractionNoneCallsUpdateMovement`
5. `TestProcessMovementInteractionWanderDispatchesWanderMode`
6. `TestProcessMovementInteractionPatrolDispatchesPatrolMode`
7. `TestProcessMovementInteractionPlayerModesResetToDefault` — sentinel for deferred PLAYER* modes.

**Targeted prelude (3):**
8. `TestProcessMovementInteractionNilTargetResetsDefaults`
9. `TestProcessMovementInteractionValidateTargetFailureResetsDefaults` (level mismatch)
10. `TestProcessMovementInteractionValidateTargetPassThrough` (asserts `aiMode` called)

**`validateTarget` (5):**
11. Level mismatch.
12. Maxrange exceeded, OP-trigger branch.
13. Maxrange exceeded, AP-trigger branch.
14. `targetSubject.typ` mismatch (NPC changetyped mid-interaction).
15. `*Npc` target with `dead==true`.

**`targetWithinMaxRange` (1 table-driven test, 4 rows):**
16. Rows cover: OP corner-removal quirk (maxrange+1, maxrange+1) → false; OP
    non-corner edge (maxrange+1, 0) → true; AP branch uses maxrange+attackrange;
    default branch uses maxrange+1 SW-distance.

**`defaultMode` + `resetDefaults` (4):**
17. `defaultMode` with PatrolCoord → `NPCModePatrol`.
18. `defaultMode` with WanderRange>0 → `NPCModeWander`.
19. `defaultMode` with neither → `NPCModeNone`.
20. `resetDefaults` clears target, writes `defaultMode()`, does NOT touch faceEntity/masks/apRange.

**`SetInteraction` (1 table-driven test, 6 rows):**
21. Rows cover: 4 concrete target types (Player/Npc/Loc/Obj), each asserting
    `target`, `targetOp`, `apRange=10`, `apRangeCalled=false`, `targetSubject.com`,
    `targetSubject.typ`, `faceAngleX/Z`, `faceEntity` (Player/Npc only) or
    `targetX/Z` (Loc/Obj only), `masks |= entitymask` (Player/Npc only); plus a
    `com == 0` row asserting `targetSubject.com == -1` (TS truthy quirk); plus a
    `target.IsValid() == false` row asserting returns `false` with no state changes.

**`updateMovement` walk/run (4):**
22. Walk with 1 waypoint: `walkDir` set, `runDir=-1`.
23. Run with 2+ waypoints: `walkDir` = dir1, `runDir` = dir2.
24. Run with 1 waypoint: `walkDir` set, `runDir=-1`.
25. `MoveRestrictNoMove` → no steps, both `-1`.

**`aiMode` try-twice (2):**
26. Target starts out-of-range, in-range after updateMovement → OP fires on second `tryInteract`.
27. Target starts in-range → OP fires on first `tryInteract`; second call not observed.

**Givechase clause (2):**
28. `GiveChase=false` + moved → target cleared post-updateMovement.
29. `GiveChase=true` + moved → target persists; second tryInteract call observed.

### `modules/world/npc_interaction_trigger_test.go` — fire matrix (~24 test functions)

**Sparse 4×4 matrix (16 test functions):**

For each target category (Player, Npc, Loc, Obj), for each case
(AP happy, OP happy, no-script, lifecycle-invalid): assert correct
`ServerTriggerType` fired via `scriptProvider.GetByTrigger` call args,
correct `category` resolution, and correct post-fire state
(`clearInteraction` called on no-script/lifecycle-invalid; target preserved on happy).

**Mode→trigger map helpers (8 table-driven test functions):**
30–37. One per helper (`aiApNpcTriggerForOp`, `aiOpNpcTriggerForOp`,
`aiApPlayerTriggerForOp`, `aiOpPlayerTriggerForOp`, `aiApLocTriggerForOp`,
`aiOpLocTriggerForOp`, `aiApObjTriggerForOp`, `aiOpObjTriggerForOp`). Each verifies:
in-range values 1..5 → correct trigger + offset; out-of-range → `0`.

### `modules/world/npc_hunt_test.go` — handoff integration (+2 tests)

38. `TestConsumeHuntTargetInteractionBranchCallsSetInteraction` — post-call state
    shows all seven NAI-10 deferrals closed: `apRange=10`, `apRangeCalled=false`,
    `targetSubject.com`, `targetSubject.typ`, `faceEntity`, `masks` updated.
39. `TestConsumeHuntTargetQueueBranchSignatureUnchanged` — verifies QUEUE1..20 branch
    still fires via `runNpcScript` with `target=nil`; ensures NAI-10 behaviour
    preserved through the signature migration.

### `pkg/script/npc_script_test.go` — signature dispatch (+4 tests)

40. Target=`*Player` → `state.ActivePlayer` set + `state.Pointers & PtrActivePlayer`.
41. Target=`*Npc` → `state.OtherActiveNpc` set + `state.Pointers & PtrOtherActiveNpc`.
42. Target=`*Loc` → `state.ActiveLoc` set + `state.Pointers & PtrActiveLoc`.
43. Target=`*Obj` → `state.ActiveObj` set + `state.Pointers & PtrActiveObj`.

**Total test functions:** ~59 (29 in npc_interaction_test.go + 24 in
npc_interaction_trigger_test.go + 2 in npc_hunt_test.go + 4 in npc_script_test.go).

**Test-count justification:** The AP/OP matrix is the highest-risk component
(fidelity risk: High per roadmap). A sparse 4-case × 4-target matrix proves every
branch of `fireAiOpTrigger`/`fireAiApTrigger` without the combinatorial blowup of a
dense 5-op × 4-target matrix. The 8 mode→trigger tests are cheap (table-driven, ~5
lines each) and guard the trigger-constant offset arithmetic where fidelity bugs
hide. setInteraction table-driven is the ONLY test that proves all seven NAI-10
deferrals close in one place.

## Deviations tracked

Per memory *"True-to-TS fidelity gate"*, behavioural divergences are recorded with
rationale + follow-up.

1. **PLAYERESCAPE / PLAYERFOLLOW / PLAYERFACE / PLAYERFACECLOSE modes deferred.**
   **Rationale:** Movement-only modes with no AP/OP trigger dispatch; orthogonal to
   NAI-11's fidelity-risk focus on the AP/OP matrix. **Follow-up:** Separate sub-spec
   for NPC player-only movement modes when runtime content surfaces a need. Inline
   breadcrumb at the dispatcher's default case.

2. **SMART pathfinding branch of `pathToTarget` deferred.** **Rationale:** Go's
   naive-only pathing is sufficient for every NAI sub-spec through NAI-12; SMART
   requires a full `findPath`/`findPathToEntity`/`findPathToLoc` port which is a
   massive orthogonal effort. **Follow-up:** Audit sub-spec or a dedicated
   pathfinding sub-spec.

3. **`reachedEntity` / `reachedLoc` / `reachedObj` reach helpers deferred.**
   **Rationale:** NAI-11 uses Chebyshev distance; full reach logic (loc shape/angle/
   forceapproach, obj width/length) needs the pathfinder package's reach helpers
   integrated. **Follow-up:** Same audit sub-spec as #2.

4. **Line-of-sight gating on AP checks deferred.** **Rationale:** Inherits player-side
   S6l-D4 deviation posture. LoS integration is an orthogonal pathfinder-package
   concern. **Follow-up:** Same audit as #2/#3; this is the NPC-side analogue of
   CheckVis deferred in NAI-8.

5. **`interacted` field on `*Npc` deferred.** **Rationale:** NAI-11 gets by with
   `moved` from updateMovement; `interacted` is mostly observability. **Follow-up:**
   Combat sub-specs that need per-tick NPC interaction tracking.

6. **`Interaction.ENGINE` vs `Interaction.SCRIPT` `instant` flag write-only.**
   **Rationale:** `focus()` stores the `instant` flag but no current consumer branches
   on it. **Follow-up:** If/when the wire protocol diverges for engine-initiated
   vs script-initiated face.

7. **`com == 0 → -1` quirk preserved.** **Not a deviation — preserved from TS.**
   Inline comment documents.

8. **NAI-10 DEVIATION block closes:** seven deferrals (apRange, apRangeCalled,
   targetSubject.com/typ, focus+faceAngleX/Z, faceEntity+masks, targetX/Z, isValid
   pre-check) all resolve with this spec. The `consumeHuntTarget` DEVIATION doc
   comment shrinks to a reference pointing at `SetInteraction`.

## TS reference

**Primary:**
- `Engine-TS/src/engine/entity/Npc.ts:182` — call site in `turn()`.
- `Engine-TS/src/engine/entity/Npc.ts:562-603` — `processMovementInteraction`.
- `Engine-TS/src/engine/entity/Npc.ts:606-627` — `validateTarget`.
- `Engine-TS/src/engine/entity/Npc.ts:629-680` — `targetWithinMaxRange`.
- `Engine-TS/src/engine/entity/Npc.ts:832-858` — `aiMode`.
- `Engine-TS/src/engine/entity/Npc.ts:861-883` — `tryInteract`.
- `Engine-TS/src/engine/entity/Npc.ts:989-1080` — `getTrigger`, `getTriggerForMode`,
  `checkApTrigger`, `checkOpTrigger`.
- `Engine-TS/src/engine/entity/PathingEntity.ts:378-406` — `inOperableDistance`,
  `inApproachDistance`.
- `Engine-TS/src/engine/entity/PathingEntity.ts:510-548` — `setInteraction`.
- `Engine-TS/src/engine/entity/PathingEntity.ts:550-556` — `clearInteraction`.
- `Engine-TS/src/engine/entity/NpcMode.ts` — full enum.

**Supporting:**
- `Engine-TS/src/engine/entity/PathingEntity.ts:422-508` — `pathToPathingTarget`,
  `pathToTarget` (SMART/NAIVE branches; NAIVE-only ported).

## Files touched

**Created (4):**
- `modules/world/npc_interaction.go`
- `modules/world/npc_interaction_trigger.go`
- `modules/world/npc_interaction_test.go`
- `modules/world/npc_interaction_trigger_test.go`

**Modified (core, ~10):**
- `modules/world/npc_ai.go`
- `modules/world/npc.go`
- `modules/world/npc_hunt.go`
- `modules/world/interaction.go` (entity interface + any needed helpers)
- `modules/world/player.go` (IsValid)
- `pkg/entity/loc.go` (IsValid; path may differ)
- `pkg/entity/obj.go` (IsValid; path may differ)
- `pkg/objtype/npctype.go` (full NPCMode enum)
- `pkg/script/npc_script.go` (runNpcScript signature)
- `pkg/script/state.go` (OtherActiveNpc field; path may differ)

**Modified (test-only migrations for `NpcMode*` constant renames + `runNpcScript`
signature, ~10-15 files):** Enumerate during plan phase via grep.

## Post-commit verification

Per memory *"Verify implementer claims with fresh independent runs"* and *"Enumerate
ALL call sites when propagating through a shared file"*:

1. `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...` at HEAD (not
   package-scoped) — catches cross-package breakage masked by local green.
2. Grep `NpcModeNone|NpcModeWander|NpcModePatrol` across the repo — expect ZERO
   matches (fully migrated to `objtype.NPCMode*`).
3. Grep `runNpcScript\(` across the repo — every call site uses the new 4-arg
   signature with a valid target (entity or nil).
4. Grep `processMovementInteraction` — expect exactly ONE production call site
   (inside `Npc.turn()`).
5. Grep `consumeHuntTarget` + manual inspection of its body — the DEVIATION
   comment block has shrunk to a reference-only comment pointing at
   `SetInteraction`.
6. Grep `checkOpTrigger|checkApTrigger` — NPC-side helpers live in
   `npc_interaction.go`; player-side helpers at `interaction.go` if any (verify no
   symbol collision).

## Rough LOC estimate

- Production: ~570 LOC
  - `npc_interaction.go`: ~320
  - `npc_interaction_trigger.go`: ~220
  - `npc.go` additions (fields): ~15
  - `npc_ai.go` shrinkage: -25 (net subtraction)
  - `npc_hunt.go` interaction-branch simplification: -5 (net subtraction)
  - `npctype.go` constants: ~70 (all 68 values + grouping comments)
  - `runNpcScript` + ScriptState: ~30
  - `IsValid` implementations × 4: ~15

- Tests: ~520 LOC
  - `npc_interaction_test.go`: ~300
  - `npc_interaction_trigger_test.go`: ~180
  - `npc_hunt_test.go` amendments: ~30
  - `npc_script_test.go` amendments: ~40
  - Test-only migrations for constant/signature renames: ~ -30 (net near-zero; the
    rewrites replace existing test code)

- Docs: ~40 LOC of doc comments.

**Total: ~1,130 LOC.** Exceeds the roadmap's "~200 LOC" estimate by ~5.6×. The
roadmap estimate pre-dated:
- The constant-unification scope decision (option A on Q2, adds the full 68-value
  enum + migration).
- Run-support wire-up (option B on Q5, adds ~30 LOC of updateMovement + 4 tests).
- All-four-target-categories scope (option A on Q4, adds ~160 LOC of 8 fire helpers
  + 8 mode→trigger helpers vs the minimal 2-category case).
- Full `setInteraction` port closing seven NAI-10 deferrals atomically.

Noting up-front so the plan's test-coverage crosscheck (memory
*"Plan-test-coverage crosscheck"*) anchors against the real scope rather than the
roadmap stub.
