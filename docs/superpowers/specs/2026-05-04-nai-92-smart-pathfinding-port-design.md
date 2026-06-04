# NAI-92: Full SMART pathfinding port for `pathToTarget`

**Status:** Draft (brainstorm output)
**Date:** 2026-05-04
**Cadence:** Multi-bundle TDD sub-spec. ~7 implementer dispatches (B1-B7). Standard `runescript_cadence`: spec → plan → subagent-driven TDD with two-stage review per bundle.
**Predecessor:** NAI-91 (shape-aware `inOperableDistance` for Loc targets — closed the door re-click but left Survival Expert NPC unreachable per §10 deferral). NAI-90 mooted the full SMART port as option iii (B2-B6) but NAI-91 took the narrower reach-gate fix.
**Tech stack:** Go 1.26+ / TS reference at `LostCityRS/Engine-TS` per `ts_source_canonical_path` / Rust source `2004scape/rsbuf` branch 225 per `rust_source_canonical_path`.

---

## 1. Goal

Port TS `PathingEntity.pathToTarget` (Engine-TS/.../PathingEntity.ts:457-508) faithfully to goscape. The current `pathToTarget(tx, tz int)` packs a single waypoint and routes through `pathToMoveClick` → `FindPathDefault` (the shape-blind 1×1 wrapper at `pkg/pathfinder/routefinder/api.go:37`). All target-type information is lost before the pathfinder is called, so:

- Multi-tile Locs (banks, stalls, doors with non-trivial geometry) get pathed to as 1×1 tiles.
- NPC/Player targets are pathed to with `srcSize=1, destWidth=1, destLength=1, shape=-1` regardless of actual entity dimensions.
- Wall-blocked NPCs (e.g. Survival Expert behind cabin wall) are unreachable because the pathfinder uses the wrong dest-shape sentinel and never finds the door-routed path.

The port reshapes `pathToTarget` to read `p.target` directly (TS parity), type-switch on target, and call shape-aware findPath wrappers. Closes the long-standing NAI-11 deferral ("SMART pathfinding branch in pathToTarget"). Closes Survival Expert NPC reachability (smoke target). Touches but does not retire `NAI-91-D-OPERABLE-CHEB-FALLBACK` — `reachedEntity` / `reachedObj` ports remain their own future sub-specs.

## 2. Context: NAI-91 smoke binding

NAI-91 fixed the `inOperableDistance` reach-gate (player on the door tile passes the operable check); the door re-click is green. Smoke confirmed at commit `74d7e7d` (2026-05-04). Adjacent symptom logged in `nai_followups.md` "From NAI-91": Survival Expert NPC (typeId=943) at `(3104, 3093)` unreachable from player at `(3101, 3105)`, `cheb_dist=12`, repathed=true, steps_taken=0. The pathfinder runs but finds no route — different mechanism from NAI-91's reach-gate.

**Static binding (no Stage 1 instrumentation):**

1. `Player.pathToTarget(tx, tz)` (`interaction.go:566`) packs a single waypoint at `(tx, tz)` and calls `pathToMoveClick(packed, true)`.
2. `pathToMoveClick`'s SMART arm (`movement.go:142`) calls `gamemap.Pathfinder.FindPathDefault(level, p.x, p.z, dest.X, dest.Z)`.
3. `FindPathDefault` (`api.go:37`) hardcodes `srcSize=1, destWidth=1, destLength=1, angle=0, shape=-1`. The Survival Expert NPC's actual dest shape is the NPC's `width × length` (likely 1×1 here, but `shape=-1` and `srcSize=1` are the wrong sentinels — TS uses `shape=-2` for entity targets, which routes the rsmod search differently).
4. TS `PathingEntity.pathToTarget` (PathingEntity.ts:462-476) reads `this.target` and dispatches:
   - `*PathingEntity`: `findPathToEntity(... srcSize, destW, destL)` → `rsmod.findPath(... srcSize, destW, destL, 0, **-2**, true, 0, 25, NORMAL)`
   - `*Loc`: `findPathToLoc(... srcSize, locW, locL, locAngle, locShape, forceapproach)` → `rsmod.findPath(... srcSize, locW, locL, angle, shape, true, blockAccessFlags, 25, NORMAL)`
   - `*Obj` same tile: queue single waypoint (TS workaround comment at line 473)
   - else (`*Obj` different tile): `findPath(...)` → plain shape-blind
5. **Conclusion:** the goscape pathfinder API (`PathFinderAPI.FindPath` at `api.go:42`) already supports the full 14-arg surface; the gap is at the entity-side dispatch. Wrapping that into named helpers + restructuring `pathToTarget` is the entire port.

## 3. Cadence

Multi-bundle. Per `runescript_cadence`:

- Spec (this doc) → plan (subagent-driven TDD plan) → seven sequential implementer dispatches with two-stage review per bundle (implementer-side TDD review + controller-side `verify_implementer_claims` re-runs).
- Apply `controller_preflight` per dispatch.
- Apply `enumerate_all_sites` for the `pathToTarget` signature change (11 test sites + 2 production sites).
- End-of-spec smoke handoff after B7 close.

Compressed cadence is **inappropriate** here — production code surface is ~840 LOC across two pkgs, three modules. Stage-1 instrumentation is **not needed** because the diagnosis is static-confirmed at brainstorm time (no inferred binding remains).

## 4. Architecture

### 4.1 New named-wrapper layer in `pkg/pathfinder/routefinder/api.go`

TS GameMap.ts:378-391 parity. Each is a thin call to the existing `PathFinderAPI.FindPath`:

```go
// FindPathPlain mirrors TS findPath (GameMap.ts:378-380). Hardcodes the
// shape-blind 1×1 default search; equivalent to the prior FindPathDefault.
// Used by MOVE_CLICK pipeline and by SMART pathToTarget's else-branch
// (Obj different-tile fallback).
func (pf PathFinderAPI) FindPathPlain(level, srcX, srcZ, destX, destZ int) Route {
    return pf.FindPath(level, srcX, srcZ, destX, destZ, 1, 1, 1, 0, -1, true, 0, 25, collision.TypeNormal)
}

// FindPathToEntity mirrors TS findPathToEntity (GameMap.ts:382-384).
// shape=-2 is the entity-target sentinel for rsmod's reach search.
func (pf PathFinderAPI) FindPathToEntity(level, srcX, srcZ, destX, destZ, srcSize, destWidth, destLength int) Route {
    return pf.FindPath(level, srcX, srcZ, destX, destZ, srcSize, destWidth, destLength, 0, -2, true, 0, 25, collision.TypeNormal)
}

// FindPathToLoc mirrors TS findPathToLoc (GameMap.ts:386-388). Threads
// loc shape/angle/forceapproach into rsmod's reach search.
func (pf PathFinderAPI) FindPathToLoc(level, srcX, srcZ, destX, destZ, srcSize, destWidth, destLength, angle, shape, blockAccessFlags int) Route {
    return pf.FindPath(level, srcX, srcZ, destX, destZ, srcSize, destWidth, destLength, angle, shape, true, blockAccessFlags, 25, collision.TypeNormal)
}

// FindNaivePath mirrors TS findNaivePath (GameMap.ts:390-392). Uses the
// existing NaiveRouteFinder which takes width and length per side.
func (pf PathFinderAPI) FindNaivePath(level, srcX, srcZ, destX, destZ, srcWidth, srcLength, destWidth, destLength int, extraFlag collision.Flag, collisionType collision.Type) Route {
    return pf.NaiveRouteFinder.FindNaivePath(level, srcX, srcZ, destX, destZ, srcWidth, srcLength, destWidth, destLength, extraFlag, collisionType)
}
```

`FindPathDefault` is **renamed in place** to `FindPathPlain` in B1; the single production caller at `movement.go:142` is updated atomically. No deprecation alias — only one production site references it. (Spec doc references in `docs/superpowers/specs/` and `docs/superpowers/plans/` are historical and stay as-is.)

### 4.2 New `pkg/coordgrid.Intersects` helper

TS CoordGrid.intersects (CoordGrid.ts:144-150) parity:

```go
// Intersects reports whether two axis-aligned bounding boxes overlap.
// Mirrors TS CoordGrid.intersects.
func Intersects(srcX, srcZ, srcW, srcL, destX, destZ, destW, destL int) bool {
    srcHorizontal := srcX + srcW
    srcVertical := srcZ + srcL
    destHorizontal := destX + destW
    destVertical := destZ + destL
    return !(destX >= srcHorizontal || destHorizontal <= srcX || destZ >= srcVertical || destVertical <= srcZ)
}
```

### 4.3 New entity-side helpers (PathingEntity-base parity)

```go
// blockWalkFlag returns the CollisionFlag used to mark this entity's
// occupied tiles for path search. Mirrors TS Player.blockWalkFlag
// (Player.ts:706+) and Npc.blockWalkFlag (Npc.ts:381+). Returns
// collision.FlagNull when moveRestrict==NoMove (signals "no walking").
func (p *Player) blockWalkFlag() collision.Flag { ... }
func (n *Npc) blockWalkFlag() collision.Flag { ... }

// getCollisionStrategy returns the collision search type for this
// entity, or nil for moveRestrict==NoMove. Mirrors TS getCollisionStrategy.
func (p *Player) getCollisionStrategy() *collision.Type { ... }
func (n *Npc) getCollisionStrategy() *collision.Type { ... }
```

Exact return-value matrix (per MoveRestrict variant) is determined by reading TS PathingEntity.ts/Player.ts/Npc.ts in B1.

### 4.4 Player `pathToTarget()` reshape

Drop the `(tx, tz)` args. Read `p.target` directly. Type-switch matching TS:

```go
// pathToTarget queues waypoints from p.x/p.z to p.target, dispatched by
// target type with shape-aware findPath helpers. Mirrors TS
// PathingEntity.pathToTarget (PathingEntity.ts:457-508).
//
// Single point-of-entry; replaces NAI-11's naive (tx, tz int) signature.
func (p *Player) pathToTarget() {
    if p.target == nil {
        return
    }

    srv := p.client.server

    switch p.moveStrategy {
    case MoveStrategySmart:
        p.pathToTargetSmart(srv)
    case MoveStrategyNaive:
        p.pathToTargetNaive()
    default:
        p.pathToTargetNoStrategy()
    }
}
```

`pathToTargetSmart` dispatches by target type:

```go
func (p *Player) pathToTargetSmart(srv *Server) {
    pf := srv.gamemap.Pathfinder
    tx, tz, _ := p.target.Coords()

    switch t := p.target.(type) {
    case *Player, *Npc:
        // PathingEntity branch.
        tw, tl := pathingEntityDims(t)
        if srv.cfg.NodeClientRoutefinder && coordgrid.Intersects(p.x, p.z, p.width, p.length, tx, tz, tw, tl) {
            route := pf.FindNaivePath(p.level, p.x, p.z, tx, tz, p.width, p.length, tw, tl, 0, collision.TypeNormal)
            p.queueWaypoints(routeToPacked(route))
        } else {
            route := pf.FindPathToEntity(p.level, p.x, p.z, tx, tz, p.width, tw, tl)
            p.queueWaypoints(routeToPacked(route))
        }
    case *entitypkg.Loc:
        var fap int
        if cfg := srv.locTypeOrNil(t.Type()); cfg != nil {
            fap = cfg.ForceApproach
        }
        route := pf.FindPathToLoc(p.level, p.x, p.z, tx, tz, p.width, t.Width, t.Length, t.Angle(), t.Shape(), fap)
        p.queueWaypoints(routeToPacked(route))
    case *entitypkg.Obj:
        if p.x == tx && p.z == tz {
            // TS workaround: findPath returns 0,0 if src==dest; queue one waypoint.
            p.queueWaypoint(tx, tz)
        } else {
            route := pf.FindPathPlain(p.level, p.x, p.z, tx, tz)
            p.queueWaypoints(routeToPacked(route))
        }
    }
}

func (p *Player) pathToTargetNaive() {
    cs := p.getCollisionStrategy()
    if cs == nil {
        return
    }
    extraFlag := p.blockWalkFlag()
    if extraFlag == collision.FlagNull {
        return
    }

    tx, tz, _ := p.target.Coords()
    if t, ok := p.target.(pathingEntity); ok {
        tw, tl := t.Dims()
        pf := p.client.server.gamemap.Pathfinder
        route := pf.FindNaivePath(p.level, p.x, p.z, tx, tz, p.width, p.length, tw, tl, extraFlag, *cs)
        p.queueWaypoints(routeToPacked(route))
    } else {
        p.queueWaypoint(tx, tz)
    }
}

func (p *Player) pathToTargetNoStrategy() {
    cs := p.getCollisionStrategy()
    if cs == nil {
        return
    }
    extraFlag := p.blockWalkFlag()
    if extraFlag == collision.FlagNull {
        return
    }
    tx, tz, _ := p.target.Coords()
    p.queueWaypoint(tx, tz)
}
```

### 4.5 Npc `pathToTarget()` reshape

Currently naive single-waypoint (no pathfinder). TS Npc.pathToTarget overrides PathingEntity.pathToTarget (Npc.ts:319-335): non-PathingEntity targets delegate to base; PathingEntity targets check intersect-shortcut; otherwise delegate to base. Goscape inlines this since Go has no method override:

```go
// pathToTarget mirrors TS Npc.pathToTarget (Npc.ts:319-335). Override
// of PathingEntity.pathToTarget that short-circuits PathingEntity targets
// to FindNaivePath when bbox-intersect, otherwise delegates to the same
// SMART/NAIVE/else dispatch as Player.pathToTarget.
func (n *Npc) pathToTarget() {
    if n.target == nil {
        return
    }

    if t, ok := n.target.(pathingEntity); ok {
        tx, tz, _ := t.Coords()
        tw, tl := t.Dims()
        if coordgrid.Intersects(n.x, n.z, n.width, n.length, tx, tz, tw, tl) {
            pf := n.server.gamemap.Pathfinder
            route := pf.FindNaivePath(n.level, n.x, n.z, tx, tz, n.width, n.length, tw, tl, 0, collision.TypeNormal)
            n.queueWaypoints(routeToPacked(route))
            return
        }
    }

    n.pathToTargetBase()
}

// pathToTargetBase is the shared base dispatch consumed by Npc and Player.
func (n *Npc) pathToTargetBase() {
    switch n.moveStrategy {
    case MoveStrategySmart:
        n.pathToTargetSmart()
    case MoveStrategyNaive:
        n.pathToTargetNaive()
    default:
        n.pathToTargetNoStrategy()
    }
}
```

NPC variants of `pathToTargetSmart` / `pathToTargetNaive` / `pathToTargetNoStrategy` mirror Player's, with `n.server` instead of `p.client.server`. The dispatch logic is duplicated rather than factored into a shared package-level helper because the `self` type carries `server`/`client.server` access asymmetrically. **Risk register R2 mitigation:** add a side-by-side comment cross-reference at each Player/Npc method pair.

### 4.6 Caller updates

- `interaction.go:236-237` (Player): drop the `tx, tz, _ := p.target.Coords()` lookup; call `p.pathToTarget()` directly.
- `npc_interaction.go:227` (Npc): unchanged signature; behaviour now exercises full SMART dispatch.
- Test fixtures: ~11 sites in `interaction_test.go` / `npc_interaction_test.go` / `npc_player_modes_test.go` / `interaction_debug_test.go` need to set `p.target` before calling `pathToTarget()` instead of passing `(tx, tz)`. Enumerated in B2's plan-author preflight.

## 5. Bundle decomposition

Seven implementer dispatches. Each is its own commit and its own review cycle.

| Bundle | Scope | Production LOC | Test LOC | Files touched |
|---|---|---|---|---|
| **B1** | Wrapper-API layer (`FindPathPlain`/`FindPathToEntity`/`FindPathToLoc`/`FindNaivePath`); `coordgrid.Intersects`; `(*Player/*Npc).blockWalkFlag()`; `(*Player/*Npc).getCollisionStrategy()`. Pure-additive except for `FindPathDefault` → `FindPathPlain` rename + single caller update. | ~150 | ~250 | api.go, coordgrid.go, player.go, npc.go (+ tests) |
| **B2** | Player `pathToTarget()` SMART+`*Loc` arm. Drops `(tx, tz)` signature. Type-switch entry point + `*Loc` branch wired to `FindPathToLoc`. interaction.go:237 caller updated. ~5 Loc-target test fixtures migrated. | ~80 | ~200 | interaction.go, interaction_test.go, interaction_debug_test.go |
| **B3** | Player `pathToTarget()` SMART+`*Player`/`*Npc` arm with NODE_CLIENT_ROUTEFINDER+intersects gate. Survival Expert smoke fixture. | ~60 | ~180 | interaction.go, interaction_test.go |
| **B4** | Player `pathToTarget()` SMART+`*Obj` (same-tile workaround + fallback `FindPathPlain`). Closes the SMART branch. | ~40 | ~80 | interaction.go, interaction_test.go |
| **B5** | Player `pathToTarget()` NAIVE branch + nomove-else third branch. Wires `getCollisionStrategy` / `blockWalkFlag` early-returns. | ~80 | ~150 | interaction.go, interaction_test.go, movement_test.go |
| **B6** | Npc `pathToTarget()` override matching Npc.ts:319-335. Adds `pathToTargetBase` and the four-branch dispatch on the NPC side. | ~120 | ~220 | npc_interaction.go, npc_interaction_test.go, npc_player_modes_test.go |
| **B7** | Cleanup. Remove `pathToTarget(tx, tz)` legacy comments and `// SMART branch deferred` doc comments. Re-grep `FindPathDefault` and confirm zero occurrences. End-of-bundle smoke handoff. | ~30 | ~30 | interaction.go, npc_interaction.go |

**Sequencing constraints:**

- B2-B5 are sequential (each depends on the previous via the type-switch in `Player.pathToTarget`).
- B6 depends on B5 (consumes `pathToTargetBase` shape).
- B7 is the cleanup tail.
- B1 has no dependencies and unblocks all others.

Approximate total: ~560 LOC production + ~1,110 LOC test = ~1,670 LOC across 7 commits.

## 6. Tests (per-bundle TDD)

### B1
- `pkg/pathfinder/routefinder/api_test.go`:
  - `TestFindPathPlain_DelegatesToFindPath_WithDefaultArgs` — captures the 14-arg vector and asserts `srcSize=1, destW=1, destL=1, angle=0, shape=-1, moveNear=true, blockAccessFlags=0, maxWaypoints=25, type=Normal`.
  - `TestFindPathToEntity_DelegatesToFindPath_WithEntitySentinel` — asserts `shape=-2` and threaded `srcSize, destW, destL`.
  - `TestFindPathToLoc_DelegatesToFindPath_WithLocShapeAngleAccess` — asserts shape/angle/blockAccessFlags threaded through.
  - `TestFindNaivePath_DelegatesToNaiveRouteFinder` — asserts the 11-arg pass-through.
- `pkg/coordgrid/coordgrid_test.go`:
  - `TestIntersects_OverlappingBoxes` / `..._DisjointBoxes` / `..._TouchingEdges` (TS uses strict `>=` on horizontal/vertical) / `..._SrcContainsDest` / `..._DestContainsSrc`.
- `modules/world/player_test.go` / `npc_test.go`:
  - `TestPlayer_BlockWalkFlag_PerMoveRestrict` — parametrized `MoveRestrict ∈ {Normal, Blocked, Indoors, Outdoors, NoMove, Passthru}` with TS-extracted expected `CollisionFlag`.
  - `TestPlayer_GetCollisionStrategy_NoMoveReturnsNil` — and `Normal/Blocked/Indoors/Outdoors/Passthru` returns non-nil with TS-extracted `CollisionType`.

### B2 (Player Loc target)
- `TestPlayer_PathToTarget_LocTarget_ThreadsShapeAngle` — fixture: door (wall_straight, angle=west) at `(3098, 3107)`, player at `(3097, 3107)`. Assert `pathToTarget()` calls `FindPathToLoc` with `angle=loc.AngleWest, shape=loc.ShapeWallStraight, blockAccessFlags=0`. Capture via mock pathfinder.
- `TestPlayer_PathToTarget_LocTarget_ForceApproachThreaded` — fixture with non-zero `LocType.ForceApproach`. Assert `blockAccessFlags == cfg.ForceApproach`.
- `TestPlayer_PathToTarget_LocTarget_QueuesReturnedRoute` — mock returns 3-step route, assert `p.waypointIndex` advanced by 3.
- `TestPlayer_PathToTarget_LocTarget_NilLocTypeUsesZeroForceApproach` — `locTypeOrNil` returns nil; defensive guard label per `defensive_gate_doc_comment_label`.
- `TestPlayer_PathToTarget_NoTarget_NoOp` — guard at top.
- Migration of existing `pathToTarget(tx, tz)` test fixtures to `p.target = ... ; p.pathToTarget()`. Per `enumerate_all_sites`: list all 11 sites in the plan.

### B3 (Player PathingEntity target)
- `TestPlayer_PathToTarget_NpcTarget_NoIntersect_UsesFindPathToEntity` — fixture: Survival Expert (3104, 3093) + player (3101, 3105). Assert `FindPathToEntity` called with `srcSize=p.width, destW=npc.width, destL=npc.length`.
- `TestPlayer_PathToTarget_NpcTarget_NodeClientRoutefinder_Intersect_UsesNaivePath` — `cfg.NodeClientRoutefinder=true` + bbox-intersect → `FindNaivePath` called with `extraFlag=0, type=Normal`.
- `TestPlayer_PathToTarget_NpcTarget_NodeClientRoutefinder_NoIntersect_UsesFindPathToEntity` — `cfg.NodeClientRoutefinder=true` + bbox-disjoint → `FindPathToEntity` (intersects gate fails, fallthrough).
- `TestPlayer_PathToTarget_PlayerTarget_DispatchesSameAsNpc` — symmetry pin for `*Player` target.

### B4 (Player Obj target)
- `TestPlayer_PathToTarget_ObjTarget_SameTile_QueuesSingleWaypoint` — TS workaround pin.
- `TestPlayer_PathToTarget_ObjTarget_DifferentTile_UsesFindPathPlain` — pin shape-blind 1×1 fallback.

### B5 (NAIVE + nomove-else)
- `TestPlayer_PathToTarget_NaiveStrategy_PathingEntityTarget_UsesFindNaivePath` — NAIVE + Player target → `FindNaivePath` with `extraFlag=p.blockWalkFlag()`.
- `TestPlayer_PathToTarget_NaiveStrategy_LocTarget_QueuesSingleWaypoint` — non-PathingEntity → single waypoint.
- `TestPlayer_PathToTarget_NaiveStrategy_NoMove_NoOp` — `getCollisionStrategy()` nil → early return.
- `TestPlayer_PathToTarget_NaiveStrategy_NullBlockWalkFlag_NoOp` — `blockWalkFlag()==FlagNull` → early return.
- `TestPlayer_PathToTarget_NoStrategyBranch_QueuesSingleWaypoint` — `moveStrategy != Smart && != Naive` → else branch.

### B6 (Npc override)
- `TestNpc_PathToTarget_PlayerTarget_Intersect_UsesFindNaivePath` — `cfg.NodeClientRoutefinder` not gated on NPC side (TS Npc.pathToTarget intersects shortcut is unconditional).
- `TestNpc_PathToTarget_PlayerTarget_NoIntersect_DelegatesToBase` — calls `pathToTargetBase` → SMART → `FindPathToEntity`.
- `TestNpc_PathToTarget_LocTarget_NotPathingEntity_DelegatesToBase` — non-PathingEntity skips intersects shortcut, goes to base SMART/Loc arm.
- `TestNpc_PathToTarget_NoTarget_NoOp`.
- Plus full SMART/NAIVE/else parametrization mirroring B2-B5 for NPC self.

### B7 (cleanup smoke)
- `rg "FindPathDefault" pkg/ modules/` returns zero hits in production. Spec/plan doc references retained as historical.
- `rg "SMART branch deferred" pkg/ modules/` returns zero hits.
- `rg "pathToTarget\(.*int" modules/` returns zero hits (signature retired).

## 7. Smoke matrix (final user-launched, after B7)

User-launched Java client + goscape server (per `smoke_test_server_handoff`). Smoke targets:

1. **Survival Expert** (typeId=943, Tutorial Island cabin) — click NPC from outside cabin → player paths through door → reaches NPC → OP arm fires. **Primary NAI-92 binding.**
2. **Door re-click** (NAI-91 regression check) — doors still open on first and second click; chat-suppression coords avoided per `java_client_coord_chat_suppression`.
3. **Multi-tile Loc** — bank booth or stall (any 2x1 Loc) — pathing approaches from correct angle/shape.
4. **NPC follow** — basic `setInteraction(Npc, op)` interaction smoke.
5. **Obj pickup** — ground item walk-to-pickup for the Obj branch.

## 8. Deviations

**Closed by NAI-92:**
- NAI-11 deferral: "SMART pathfinding branch in pathToTarget" (`nai_followups.md` line 793). Fully closed.

**Untouched:**
- `NAI-91-D-OPERABLE-CHEB-FALLBACK` — Chebyshev≤1 fallback for `*Player` / `*Npc` / `*Obj` in `inOperableDistance`. Pending TS `reachedEntity` / `reachedObj` ports (separate sub-spec).
- `S6l-D4` — LOS-in-approach. Untouched.

**New (provisional, register only if surfaced in B1):**
- `NAI-92-D-BLOCKWALK-FLAG-MAP` — if goscape's collision-flag enum doesn't have parity with TS `CollisionFlag.BLOCK_PLAYERS` / `BLOCK_NPCS` / `WALK_BLOCKED` in `(*Entity).blockWalkFlag()`, register and stub. Verified at B1 plan-author time.

## 9. Risk register

- **R1 — `BlockWalk` flag mapping incomplete in goscape.** NPC `typ.BlockWalk` config bits may not be ported. **Mitigation:** B1 plan-author verifies `pkg/pathfinder/collision` flag set against TS `CollisionFlag` enum; defers via `NAI-92-D-BLOCKWALK-FLAG-MAP` if gap surfaces.
- **R2 — Player/Npc dispatch duplication.** `pathToTargetSmart` / `pathToTargetNaive` / `pathToTargetNoStrategy` are duplicated on Player and Npc. Drift risk over future maintenance. **Mitigation:** explicit side-by-side cross-reference comment at each method pair; B7 cleanup verifies parity.
- **R3 — Test fixture migration cost.** 11 test sites pass `(tx, tz)`; each needs `p.target = ... ; p.pathToTarget()`. **Mitigation:** B2 establishes the pattern; B3-B6 inherit. Plan-author enumerates all 11 sites per `enumerate_all_sites`.
- **R4 — `FindNaivePath` signature mismatch.** `NaiveRouteFinder.FindNaivePath` signature must accept `extraFlag CollisionFlag, collisionType CollisionType`. **Mitigation:** B1 plan-author verifies signature; if mismatch, B1 task adds the wrapper with adapter.
- **R5 — `pathingEntity` interface.** `Player` and `Npc` need a shared interface for the type-switch at the entry point and for `Dims()`. **Mitigation:** B1 introduces minimal `pathingEntity` interface in `modules/world/movement.go` or a new `modules/world/pathing.go`. Mirrors `interface_at_cyclic_import_boundary` memory pattern if needed.
- **R6 — `cfg.NodeClientRoutefinder` smoke not exercised.** Default smoke runs server-routefinder mode; the Npc-side intersects shortcut and Player-side NODE_CLIENT_ROUTEFINDER+intersects branch are not exercised. **Mitigation:** unit-test coverage in B3 + B6 pins both branches; smoke regression-pin only the server-routefinder path.

## 10. Out of scope

- `reachedEntity` / `reachedObj` ports (NAI-91-D-OPERABLE-CHEB-FALLBACK).
- LOS gating in `inApproachDistance` (NAI-12 closed for NPC; player side may have residuals — separate sub-spec).
- NPC face-persist after walking away from dialog (RuneScape Guide #945 long-standing symptom; routes to NAI-93+ per `nai_followups.md` "From NAI-91" carry-forward).
- `targetX/targetZ` fine-coord tracking refactor — TS PathingEntity uses these for sub-tile reorient; goscape uses `target.Coords()` snapshot. Untouched.
- `findpath` movePartial (`moveNear=false`) variants — TS uses `moveNear=true` everywhere relevant; not exercised.

## 11. References

- TS source: `LostCityRS/Engine-TS` per `ts_source_canonical_path` memory.
- Rust source (rsmod-equivalent for Go ports): `2004scape/rsbuf` branch 225.
- Memories applied: `runescript_cadence`, `controller_preflight`, `verify_implementer_claims`, `enumerate_all_sites`, `pathfinder_api_loc_aware`, `cascade_theory_smoke_binding`, `smoke_test_server_handoff`, `defensive_gate_doc_comment_label`, `interface_at_cyclic_import_boundary`, `true_to_ts_gate`, `flat_arg_signature_for_cross_lang_parity`.
- Predecessors closed: NAI-91 (door re-click reach gate). Predecessor moot: NAI-90 option iii (B2-B6 framing) — adopted with B1 prerequisite + B7 cleanup added.
