# NAI-94 — Pathfinder Reach Stage 1 Diagnosis

**Spec:** `docs/superpowers/specs/2026-05-05-nai-94-pathfinder-reach-investigation-design.md`
**Plan:** `docs/superpowers/plans/2026-05-05-nai-94-pathfinder-reach-investigation.md`
**Audit date:** 2026-05-05

## Summary

Stage 1 audit ELIMINATES the routefinder algorithm itself as the source of the NAI-92-surfaced `waypoint_idx=-1` symptom. The unit-level pathfinder produces correct paths for both anomaly shapes (Hans cheb=2 straight-line, Survival Expert cheb=8) when supplied a FlagMap whose relevant zones are allocated (the real-world default). The empty-FlagMap probes the spec proposed reproduce the symptom but for a DEGENERATE reason (`FlagMap.Get()` returns `FlagNull=-1` for unallocated zones; `CanMove(-1, mask, TypeNormal) = (-1 & mask) == 0` is false for any non-zero mask, so BFS cannot expand any direction). H2 confirms the `useRouteBlockerFlags` field at `pkg/pathfinder/routefinder/routefinder.go:26` is a dead stub but is NOT the NAI-92 bug (AS has no equivalent runtime flag; under `TypeNormal` neither AS nor goscape checks route-blocker flags). Diagnosis ceiling: NAI-95 must investigate upstream — production FlagMap allocation around the relevant tiles, the caller's coord/srcSize/destWidth/destLength threading, or the consumer layer's interpretation of `Route.Alternative`.

## Reproducer test results

| Test | Result | Disposition |
|---|---|---|
| `TestNAI94_HansCheb2_StraightLineMustReach` | FAIL on empty FlagMap (`{Waypoints:[] Alternative:true Success:true}`) | t.Skip with degenerate-case pin |
| `TestNAI94_RouteBlockerFlag_Consulted/BlockerHonored_RefusesToCross` | FAIL (got Success=true, want false; field unconsulted) | t.Skip with H2-confirmed pin |
| `TestNAI94_RouteBlockerFlag_Consulted/BlockerIgnored_PassesThrough` | PASS structurally (Success=true reaches dest) | t.Skip (paired with sibling for differential probe) |
| `TestNAI94_SurvivalExpert_BlockedPassage/EmptyFlagMap_MustReach` | FAIL on empty FlagMap (same shape as H1) | t.Skip with degenerate-case pin |
| `TestNAI94_SurvivalExpert_BlockedPassage/SyntheticCabinWall_MoveNearReports` | logs `Success=true Alternative=true len=5 last=(3103,3096)` | (diagnostic, never asserts) |
| `TestNAI94_AllocatedZones_PathfinderWorks/HansCheb2` | PASS (Success=true, Alternative=false, last=(3219,3222)) | passing — positive elimination evidence |
| `TestNAI94_AllocatedZones_PathfinderWorks/SurvivalExpertCheb8` | PASS (Success=true, Alternative=false, last=(3103,3095)) | passing — positive elimination evidence |

## Per-hypothesis verdicts

### H1 — BFS / waypoint return broken at minimum-distance paths

**Verdict:** ELIMINATED at allocated-zone unit level.

**Evidence:**
- `pkg/pathfinder/routefinder/nai94_repro_test.go::TestNAI94_AllocatedZones_PathfinderWorks/HansCheb2` PASSES: src=(3219,3224)→dst=(3219,3222) with `internal.BuildCollisionMap(3216,3220,3222,3226)` returns `{Waypoints:[(3219,3222,0)] Alternative:false Success:true}`.
- The empty-FlagMap reproducer fails for a DEGENERATE reason: `pkg/pathfinder/collision/flag.go:4` `FlagNull = -1`; `pkg/pathfinder/collision/flagmap.go:30-48` returns `FlagNull` for unallocated zones; `pkg/pathfinder/collision/strategies.go:25-26` `TypeNormal` returns `tileFlag & blockFlag == 0`. With `tileFlag = -1` (all bits set in two's complement), the AND with any non-zero `blockFlag` yields a non-zero result, so `CanMove` returns false for every direction expansion. Source's BFS expansion at `routefinder.go:184-274` therefore appends zero neighbors. Loop exits, `routeFindSize1` returns false, `pathFound=false`, moveNear branch fires `findClosestApproachPoint` which selects the source itself as the only tile with finite distance → reconstruction breaks immediately on the first iteration's `currLocalX==localSrcX` check → empty Waypoints, `Alternative=true`.
- T1 implementer's original skip pin claimed `Alternative:false`. Verification probe (`TestNAI94_ProbeFullRoute`, ephemeral) showed actual `Alternative:true`. Pin corrected at commit `4a289a4`.

### H2 — `useRouteBlockerFlags` declared but unconsulted

**Verdict:** CONFIRMED as dead/stub field. **NOT the NAI-92 bug.**

**Diff register:**

| AS site | Goscape site | Status |
|---|---|---|
| `rsmod-pathfinder/src/rsmod/PathFinder.ts` constructor + class — no `useRouteBlockerFlags` field; `CollisionStrategy` is the per-call polymorphism | `pkg/pathfinder/routefinder/routefinder.go:26` field declaration; `:39-44` constructor argument; `:44` carries `// TODO: unused - funcs written as if false` | DIVERGENT (goscape adds field; AS has no equivalent) |
| `rsmod-pathfinder/src/rsmod/collision/CollisionStrategies.ts:52-61` `LineOfSight.BLOCK_ROUTE` static const (9 RouteBlocker flags OR'd) | `pkg/pathfinder/collision/strategies.go:13-21` `lineOfSightBlockRoute` constant | MATCH (semantically identical) |
| `CollisionStrategies.ts:64-69` `LineOfSight.canMove`: `(blockFlag & BLOCK_ROUTE) >> 13` static mask transform; only fires under `LineOfSight` strategy | `collision/strategies.go:34-38` `TypeLineOfSight` case mirrors the same `>> 13` transform | MATCH |
| `PathFinder.findPath1` step expansion calls `collision.canMove(tileFlag, blockFlag)` via the per-call CollisionStrategy object | `routefinder.go:184-274` `routeFindSize1` calls `collision.CanMove(flag, clipFlag, collisionType)` — static dispatch on `Type` enum | MATCH (same effect under TypeNormal: route-blocker flags not checked) |
| `useRouteBlockerFlags` consumed in `findPath*` body | `useRouteBlockerFlags` consumed in `routeFindSize1`/`Size2`/`Big` body | DIVERGENT (goscape: never read; AS: no such field) |

**Evidence:**
- `rg "useRouteBlockerFlags" pkg/ /home/owner/Code/github.com/2004scape/rsmod-pathfinder/src/`: only goscape references, all in `routefinder.go` constructor; never read in any `routeFind*` body.
- `TestNAI94_RouteBlockerFlag_Consulted` differential subtests behave IDENTICALLY: both `useRouteBlockerFlags=true` and `=false` produce `Success=true Waypoints=[(3004,3000,0)]` despite the planted `FlagWallWestRouteBlocker` at (3002, 3000). The field is not consulted in any expansion branch.
- AS root-cause analysis: route-blocker flag honoring is fully governed by which `CollisionStrategy` is passed at call time. `TypeNormal` (the default for `FindPathPlain`/`FindPathToEntity`/`FindPathToLoc`) ignores route-blocker flags in BOTH AS and goscape. Hans cheb=2 doesn't involve route-blocker tiles at all, so this hypothesis is structurally incompatible with the NAI-92 anomaly.

### H3 — Ring buffer wrap or `maxWaypoints` truncation

**Verdict:** ELIMINATED.

**Evidence:**
- `TestNAI94_AllocatedZones_PathfinderWorks/SurvivalExpertCheb8` PASSES with cheb=8 unobstructed: `{Waypoints:[(3101,3097,0),(3103,3095,0)] Alternative:false Success:true}`. No truncation observed.
- `routefinderDefaultRingBufferSize = 4096` (`routefinder.go:14`). Cheb=8 is far below ring buffer capacity. Hans cheb=2 is trivially within capacity.
- Default `maxWaypoints=25` (per `FindRouteDefault` at `routefinder.go:161-163`). Reconstruction at `routefinder.go:130-153` only appends a waypoint on direction CHANGES, not per-tile. For a 2-tile straight-line move (one direction throughout), only 1 waypoint is recorded. For cheb=8 unobstructed (one direction throughout), ditto — 2 waypoints suffice (intermediate corner + dest). Neither shape approaches `maxWaypoints`.
- `SyntheticCabinWall_MoveNearReports` diagnostic: `Success=true Alternative=true len=5 last=(3103,3096)` confirms moveNear closest-approach fires correctly when blocked, returning 5 waypoints (last 1 tile north of the synthetic wall).

### H4 — `findPath1` step-expansion divergence

**Verdict:** ELIMINATED for `routeFindSize1`. PARTIAL divergence in `routeFindSize2` (out of H1/H3 scope; tracked as residual).

**Diff register (8-direction step expansion, srcSize=1):**

| Direction block | AS line | Goscape line | Divergence |
|---|---|---|---|
| east-to-west (clip=BLOCK_WEST, dir=East) | `PathFinder.ts:172-179` | `routefinder.go:184-192` | match |
| west-to-east (clip=BLOCK_EAST, dir=West) | `PathFinder.ts:181-188` | `routefinder.go:194-202` | match |
| north-to-south (clip=BLOCK_SOUTH, dir=North) | `PathFinder.ts:190-197` | `routefinder.go:204-212` | match |
| south-to-north (clip=BLOCK_NORTH, dir=South) | `PathFinder.ts:199-206` | `routefinder.go:214-222` | match |
| northeast-to-southwest (3 canMove: BLOCK_SW + BLOCK_W + BLOCK_S, dir=NE) | `PathFinder.ts:208-220` | `routefinder.go:224-235` | match |
| northwest-to-southeast (3 canMove: BLOCK_SE + BLOCK_E + BLOCK_S, dir=NW) | `PathFinder.ts:222-234` | `routefinder.go:237-248` | match |
| southeast-to-northwest (3 canMove: BLOCK_NW + BLOCK_W + BLOCK_N, dir=SE) | `PathFinder.ts:236-248` | `routefinder.go:250-261` | match |
| southwest-to-northeast (3 canMove: BLOCK_NE + BLOCK_E + BLOCK_N, dir=SW) | `PathFinder.ts:250-262` | `routefinder.go:263-274` | match |

**AS↔Rust spot-check:** not performed. H1 was eliminated at the allocated-zone unit level (TestNAI94_AllocatedZones_PathfinderWorks PASSES) without needing to broaden the AS↔Rust comparison. The line-for-line AS↔goscape match for `routeFindSize1` is sufficient to eliminate H4 for the H1/H3 anomaly shapes.

**Residual divergence found in `routeFindSize2`:** `pkg/pathfinder/routefinder/routefinder.go:315` reads
```go
collision.CanMove(pf.collisionFlag(baseZ, baseZ, pf.currLocalX+2, pf.currLocalZ+1, level), collision.FlagBlockNorthEast, collisionType)
```
The first arg should be `baseX, baseZ` not `baseZ, baseZ`. This is a typo specific to the size=2 path; H1 and H3 use srcSize=1 so this typo does not affect the NAI-92 anomaly. Track as NAI-95+ follow-up (likely never fired in production because srcSize=2 is rare for player movement; would surface for size-2 NPCs or large-creature pathing).

### H5 — Closest-approach / moveNear divergence

**Verdict:** ELIMINATED.

**Diff register:**

| Aspect | AS behavior | Goscape behavior | Divergent? |
|---|---|---|---|
| Iteration bounds | `[localDestX-MAX_ALTERNATIVE_ROUTE_DISTANCE_FROM_DESTINATION, localDestX+10]` × `[localDestZ-10, localDestZ+10]` (constant=10) | `routefinder.go:597-598` same — `routefinderMaxAlternativeRouteDistanceFromDestination=10` (`:19`) | match |
| Skip condition | OOB OR `distances[idx] >= MAX_ALTERNATIVE_ROUTE_SEEK_RANGE=100` | `routefinder.go:599` same — `routefinderMaxAlternativeRouteSeekRange=100` (`:18`) | match |
| Cost function | `cost = dx² + dz²` with dx/dz computed against dest box (width/length offsets) | `routefinder.go:603-617` same | match |
| Selection ordering | `cost < lowestCost OR (cost==lowestCost AND maxAlternativePath > distances[idx])` | `routefinder.go:618` same | match |
| Tie-breaking | by `distances[idx]` (lower wins on equal cost) | same | match |
| Bound constants | `MAX_ALTERNATIVE_ROUTE_LOWEST_COST=1000`, `_SEEK_RANGE=100`, `_DISTANCE_FROM_DESTINATION=10` | `routefinder.go:17-19` same values | match |
| Return condition | `lowestCost != MAX_ALTERNATIVE_ROUTE_LOWEST_COST` | `routefinder.go:626` same | match |

`PathFinder.ts:633-672` ↔ `routefinder.go:593-627` is line-for-line equivalent.

## Root cause

**Diagnosis ceiling.** The unit-level routefinder algorithm is correct (allocated-zone passing tests confirm cheb=2 and cheb=8 anomaly shapes return correct paths). The NAI-92-surfaced `waypoint_idx=-1` is therefore upstream of `pkg/pathfinder/routefinder/`. Stage 2 (NAI-95) must determine which of these is the actual production failure mode:

1. **Production FlagMap allocation gap.** If the relevant mapsquare (Lumbridge / m48_50 for Hans, or m48_48-area for Survival Expert) has unallocated zones at the time `RouteFinder.FindRoute` is invoked, `FlagMap.Get()` returns `FlagNull=-1` for those tiles. `CanMove(-1, mask, TypeNormal)` is false for any non-zero mask, blocking all BFS expansion (the same degenerate case the empty-FlagMap probes hit). Probe to confirm/refute: in production, log `pf.Flags.Get(srcX, srcZ, level)` at the moment of the failing call; if `-1`, this is the gap.

2. **Consumer's interpretation of `Route.Alternative`.** When BFS fails and moveNear closest-approach selects the source as the best candidate (because no other tile has finite distance), reconstruction returns `{Waypoints:[] Alternative:true Success:true}`. If the consumer (e.g., `pathToMoveClick`, `pathToTarget` post-NAI-92) treats `Alternative=true` as "no path → set waypoint_idx=-1," the symptom matches even when the algorithm is functioning correctly. Probe: trace how `Route.Alternative` is consumed in `modules/world/.../movement.go` and the player tick flow.

3. **Caller-side coord / srcSize / destWidth / destLength threading.** Less likely (NAI-92 already verified `pathToTarget` dispatch), but worth confirming the args supplied at the production call site match what `TestNAI94_AllocatedZones_PathfinderWorks` uses.

The ELIMINATION is binding: the routefinder algorithm itself does not need NAI-95 fixes. NAI-95 is an upstream investigation, not a routefinder change.

## Stage 2 (NAI-95) handoff

- **Root cause:** ROUTEFINDER ELIMINATED. Bug is upstream of `pkg/pathfinder/routefinder/`. NAI-95 is an upstream investigation (allocation / consumer / caller-threading).
- **Repro tests to lift skip on:** the H1 and H3 empty-FlagMap reproducers should NOT be lifted as-is — they probe a degenerate case, not the production bug. After NAI-95 identifies the actual upstream failure mode, write a NEW reproducer that pins the correct shape (e.g., loaded mapsquare + production-style call site + assertion against waypoint_idx). The H2 differential subtests can be lifted IF NAI-95 wires the `useRouteBlockerFlags` field (or removes it for YAGNI). The TestNAI94_AllocatedZones_PathfinderWorks tests should remain as a regression guard.
- **Files NAI-95 will touch:** likely outside `pkg/pathfinder/routefinder/`. Candidates: `modules/world/.../movement.go` (`pathToMoveClick`), `modules/world/.../<player|npc>.go` (Route.Alternative consumption), the FlagMap-population path (likely `modules/world/.../<map|zone>.go`), `pkg/pathfinder/routefinder/api.go::ChangeFloor`/`ChangeLoc`/etc. (might surface a missing zone-allocation step on world load).
- **Estimated LOC for fix:** unknown until upstream is probed. If the issue is production-side mapsquare zone allocation, fix is likely ~10-30 LOC in the world-load path. If the issue is consumer interpretation of `Route.Alternative`, fix is likely <10 LOC. If the issue is multi-layered, fix could span several files.
- **Residual hypotheses for NAI-96+:**
  - `pkg/pathfinder/routefinder/routefinder.go:315` — `routeFindSize2` `baseZ, baseZ` typo (should be `baseX, baseZ`). Affects size=2 source pathing. Out of NAI-95 scope unless smoke surfaces it.
  - `useRouteBlockerFlags` dead-stub field at `routefinder.go:26`. Either remove (YAGNI per existing memory pattern) or implement an NPC-aware collision strategy (port AS's `LineOfSight` polymorphism more fully). Out of NAI-95 scope unless smoke surfaces a route-blocker scenario.
  - The empty-FlagMap reproducers (H1 EmptyFlagMap, H3 EmptyFlagMap_MustReach) document a degenerate case. If desired, NAI-96+ could harden `RouteFinder.FindRoute` to short-circuit early when source's tile read returns `FlagNull` rather than silently returning `Waypoints=[] Alternative=true Success=true`.
