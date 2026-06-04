# NAI-97 — GroundDecor Over-Blocking + Reach Abandonment Stage 1 Diagnosis

**Spec:** `docs/superpowers/specs/2026-05-05-nai-97-grounddecor-reach-investigation-design.md`
**Plan:** `docs/superpowers/plans/2026-05-05-nai-97-grounddecor-reach-investigation.md`
**Audit date:** 2026-05-05
**HEAD at audit start:** e31c26b (NAI-97 plan commit)

## Summary

Static audit (H1-H4) eliminates all code-divergence hypotheses: goscape's
`LocType` decoder + PostDecode + `ChangeLocCollision` dispatch + boot-time
`populateStaticLocsIntoZones` gate + `interaction.go` SMART pathToTarget arms
match TS Engine-TS byte-for-byte against `LostCityRS/Engine-TS` HEAD.
H5 (tickloop waypoint discard) is **DIAGNOSIS-CEILING** — call-graph
mapped (CanTravel-fail at `movement.go:96`, "I can't reach that" at
`interaction.go:255-258`, repathed-once-per-interaction gate at
`interaction.go:236-239`), but no static-readable mutator surfaces as a
smoking gun. The empty-FlagMap reproducers (Repro A, Repro B) hit the
known degenerate symptom (`{Waypoints:[] Alternative:true Success:true}`
via `routefinder.go:117-124` moveNear+findClosestApproachPoint
short-circuit on the source tile when BFS can't expand) and were skip-
pinned. Real root cause needs runtime instrumentation OR a real-cache
integration test (NAI-98 first task).

## Audit baseline (Bundle 0 controller pre-flight)

```
NAI-97 Bundle 0 pre-flight at HEAD e31c26b:
- Step 1.2 ChangeLocCollision sig (gamemap.go:61): match — LayerGroundDecor branch at :72 with active==1 gate at :73
- Step 1.3 PostDecode lines (loctype.go:166-176): match — Active=-1 → 0 default, →1 if (Shapes==[10] || Op != nil); :183 default BlockWalk:true
- Step 1.4 pathfinder API names (api.go): match — FindPathPlain:40, FindPathToEntity:47, FindPathToLoc:54, FindNaivePath:62, FindPath:70
- Step 1.5 interaction.go arms: match — Loc→FindPathToLoc:623, PathingEntity→FindPathToEntity:642 or FindNaivePath:639 on bbox-intersect, Obj→FindPathPlain:659, default→FindPathPlain:670
- Step 1.6 server.go BlockWalk gate (:327): confirmed — `if lt := s.locTypeOrNil(loc.Type()); lt != nil && lt.BlockWalk` gates ChangeLocCollision at boot
- Step 1.7 fixture: available (m48_50, loc.dat, client/config all present)
```

## Reproducer test results

| Test | Result | Disposition |
|---|---|---|
| `TestNAI97_LocWalkDump_Lumbridge` | PASS (dump-only, 100 lines `-v` output captured) | always passes; output captured at Stage 1.1 (commit `5d08c71`) |
| `TestNAI97_NPC943_PathAroundFountain` | SKIP (anomaly reproduces on empty FlagMap; `{Waypoints:[] Alternative:true Success:true}` pinned) | skipped per plan §8.4; commit `9c88e3b` |
| `TestNAI97_NPC3_MidRouteAbandonment` | SKIP (same shape, same pin) | skipped per plan §8.4; commit `9c88e3b` |

**Important caveat on the SKIP tests:** the empty-FlagMap setup is degenerate
per memory `empty_flagmap_degenerate_routefinder`. The observed
`{Waypoints:[] Alternative:true Success:true}` does NOT prove a pathfinder
shape bug — it's the documented degenerate symptom for `moveNear=true`
(via `findClosestApproachPoint` short-circuiting to source tile when no
BFS expansion succeeds). NAI-98 must lift these skips against a real-cache
fixture (production loader + populateStaticLocsIntoZones replay), NOT
against `NewPathFinderAPI()` alone.

## Per-hypothesis verdicts

### H1 — TS-divergent over-blocking via LocType.BlockWalk

**Verdict:** ELIMINATED.

**Goscape decoder (`pkg/objtype/loctype.go`):**
- `:29` field declaration: `BlockWalk bool // code 17 sets false; default true`
- `:79-80` opcode 17: `lt.BlockWalk = false`
- `:183` `NewLocType` default: `BlockWalk: true`
- `:166-176` `PostDecode` does NOT touch BlockWalk

**TS decoder (`LostCityRS/Engine-TS/src/cache/config/LocType.ts`):**
- `:79` field declaration: `blockwalk = true;`
- `:127` opcode 17: `this.blockwalk = false;`
- `:202-214` `postDecode` does NOT touch blockwalk

**Cross-reference table** (LocTypes from Stage 1.1 dump, restricted to
those that actually write FlagMap entries; non-writing GroundDecor with
Active=0 omitted as their BlockWalk value is irrelevant):

| LocTypeID | Name | Layer | goscape BlockWalk | goscape Active | TS blockwalk | TS active | Divergent? |
|---|---|---|---|---|---|---|---|
| 1124 | bush1 | LayerGround (2) | true | 1 | true (default) | 1 (Op-driven) | match |
| 1516 | openbankdoor_l | LayerWall (0) | true | 1 | true | 1 | match |
| 1519 | openthickpoordoor | LayerWall (0) | true | 1 | true | 1 | match |
| 1536 | poordooropen | LayerWall (0) | true | 1 | true | 1 | match |
| 1911 | castlewall | LayerWall (0) | true | 0 | true | 0 | match |
| 196 | torch | LayerWallDecor (1) | true | 0 | true | 0 | match (LayerWallDecor no-op in both) |
| 840 | wallshield | LayerWallDecor (1) | true | 0 | true | 0 | match (no-op) |
| 1938 | castlearrowslit | LayerWallDecor (1) | true | 0 | true | 0 | match (no-op) |

(GroundDecor IDs 320, 925, 940, 941, 1248, 1257-1260, 2771 had Active=0
in the dump and the LayerGroundDecor branch only fires on `active == 1`,
so their BlockWalk values do not contribute to the FlagMap regardless of
goscape-vs-TS parity.)

**Smoking-gun candidates:** none. Same input cache → same BlockWalk
values for both implementations.

**Evidence:**
- goscape `pkg/objtype/loctype.go:29, 79-80, 166-176, 183` (read at audit time)
- TS `LostCityRS/Engine-TS/src/cache/config/LocType.ts:79, 127, 202-214`
- Stage 1.1 dump (commit `5d08c71`, captured at `$TMPDIR/nai97_dump.log` at audit time)

### H2 — LocType.Active PostDecode coercion

**Verdict:** ELIMINATED.

**Coercion rule comparison:**

- TS rule (`LocType.ts:202-214`): `if (active === -1) { active = 0; if (shapes.length === 1 && shapes[0] === 10) active = 1; if (op !== null) active = 1; }`
- goscape rule (`loctype.go:166-176`): `if lt.Active == -1 { lt.Active = 0; if len(lt.Shapes) == 1 && lt.Shapes[0] == 10 { lt.Active = 1 }; if lt.Op != nil { lt.Active = 1 } }`

Byte-identical control flow.

**Per-LocType table** (restricted to bbox-relevant IDs; both
implementations produce the same Active value because the coercion logic
is identical and Shapes/Op fields are decoded identically):

| LocTypeID | Name | shapes-set | op-set | goscape Active | TS active | Divergent? |
|---|---|---|---|---|---|---|
| 1124 | bush1 | yes | yes (Examine) | 1 | 1 | match |
| 1516 | openbankdoor_l | yes | yes | 1 | 1 | match |
| 1519 | openthickpoordoor | yes | yes | 1 | 1 | match |
| 1536 | poordooropen | yes | yes | 1 | 1 | match |
| 1911 | castlewall | yes | no | 0 | 0 | match |
| 1257-1260 | grass_light_* | yes | no | 0 | 0 | match |
| 320 | dugupsoil2_light | yes | no | 0 | 0 | match |
| 925, 940-941 | rugcorner / bluerugcorner / bluerugside | yes | no | 0 | 0 | match |
| 1248 | sticks_twigs | yes | no | 0 | 0 | match |
| 2771 | water_source_icon | yes | no | 0 | 0 | match |

**Evidence:**
- goscape `pkg/objtype/loctype.go:166-176` (verbatim at audit time)
- TS `LostCityRS/Engine-TS/src/cache/config/LocType.ts:202-214`
- Stage 1.1 dump observed Active values

### H3 — LayerGroundDecor write-side divergence

**Verdict:** ELIMINATED.

**Divergence register:**

| Aspect | TS GameMap.ts:326-341 | goscape gamemap.go:61-78 | Divergent? |
|---|---|---|---|
| Layer dispatch | `rsmod.locShapeLayer(shape)` switch on WALL/GROUND/GROUND_DECOR | `loc.LayerOf(loc.Shape(shape))` switch on LayerWall/LayerGround/LayerGroundDecor | match |
| LayerWall handling | `rsmod.changeWall(x,z,level,angle,shape,blockrange,false,add)` | `gm.Pathfinder.ChangeWall(x,z,level,angle,shape,blocksRange,false,add)` | match |
| LayerGround angle-aware swap | NORTH/SOUTH → `(length,width)`; else → `(width,length)` | NORTH/SOUTH → `(length,width)`; else → `(width,length)` | match (NAI-96-fixed) |
| GroundDecor active gate | `if (active === 1)` | `if active == 1` | match |
| GroundDecor flag written | `rsmod.changeFloor` (FlagBlockWalk) | `gm.Pathfinder.ChangeFloor` (FlagBlockWalk via api.go:74-80) | match |
| Tile count written for GroundDecor | 1 (single-tile via changeFloor) | 1 (single-tile via ChangeFloor) | match |
| LayerWallDecor branch | absent (TS skips) | absent (per goscape comment "TS skips: GameMap.ts:326-340 has no WALL_DECOR branch") | match |
| Boot-time BlockWalk gate | `if (type.blockwalk) changeLocCollision(...)` (GameMap.ts:259-260) | `if lt.BlockWalk { ChangeLocCollision(...) }` (server.go:327-330) | match |

**Evidence:**
- goscape `pkg/gamemap/gamemap.go:52-78` (read at audit time)
- TS `LostCityRS/Engine-TS/src/engine/GameMap.ts:259-263, 326-341`
- goscape `pkg/pathfinder/routefinder/api.go:74-99, 135-146`
- Stage 1.1 FlagMap dump (commit `5d08c71`) shows expected per-layer flag writes

### H4 — Pathfinder API call-site dispatch routing

**Verdict:** ELIMINATED for the SMART arm vs TS PathingEntity.ts. The
empty-grid reproducers FAILED but **NOT for shape-dispatch reasons** —
they hit the FlagNull-degenerate symptom for `moveNear=true`
(`findClosestApproachPoint` falls back to source tile in
`routefinder.go:117-124`). The implementer's report described this as
"entity-reach considers source already reached"; controller verification
shows it is the documented degenerate-empty-grid behavior, not a
pathfinder shape bug. Repro A/B do NOT escalate H4.

**Dispatch-arm register:**

| Target class | TS arm (`PathingEntity.ts:457-475`) | goscape arm (`interaction.go:600-672`) | TS args | goscape args | Divergent? |
|---|---|---|---|---|---|
| player→Loc (SMART) | `findPathToLoc(level, x, z, tx, tz, this.width, target.width, target.length, target.angle, target.shape, forceapproach)` (L471) | `pf.FindPathToLoc(p.level, p.x, p.z, tx, tz, p.Width(), t.Width, t.Length, t.Angle(), t.Shape(), fap)` (:623) | identical | identical | match |
| player→Player/Npc (SMART, intersect) | `findNaivePath(level, x, z, tx, tz, this.width, this.length, target.width, target.length, 0, NORMAL)` (L465) | `pf.FindNaivePath(p.level, p.x, p.z, tx, tz, p.Width(), p.Length(), tw, tl, 0, collision.TypeNormal)` (:639) | identical | identical | match |
| player→Player/Npc (SMART, no intersect) | `findPathToEntity(level, x, z, tx, tz, this.width, target.width, target.length)` (L467) | `pf.FindPathToEntity(p.level, p.x, p.z, tx, tz, p.Width(), tw, tl)` (:642) | identical | identical | match |
| player→Obj same-tile | `queueWaypoint(target.x, target.z)` (L473) | `p.queueWaypoint(tx, tz)` (:652) | identical | identical | match |
| player→Obj different-tile | `findPath(level, x, z, tx, tz)` (L475 — TS plain findPath) | `pf.FindPathPlain(p.level, p.x, p.z, tx, tz)` (:659) | identical | identical | match |

**Empty-grid reproducer outcome:**
- Both Repro A (`TestNAI97_NPC943_PathAroundFountain`) and Repro B
  (`TestNAI97_NPC3_MidRouteAbandonment`) returned
  `{Waypoints:[] Alternative:true Success:true}`.
- Mechanism: empty `NewPathFinderAPI()` FlagMap returns FlagNull=-1 for
  all unallocated zones → `CanMove` returns false everywhere → BFS
  doesn't expand past source → `pathFound=false` →
  `findClosestApproachPoint` evaluates only the source tile (the only
  tile appended in `routeFindSize1`) → currLocalX/Z stays at source →
  waypoint construction loop at `routefinder.go:130-153` immediately
  breaks on `currLocalX == localSrcX` → empty waypoints, `Alternative=true`,
  `Success=true`.
- Verified at `pkg/pathfinder/routefinder/routefinder.go:107, 117-124, 126-158`
  and `routeFindSize1` BFS at `:165-180`.
- This is NOT proof that `findPathToEntity` mishandles entity-reach
  on the smoke geometry; the test setup is too degenerate to falsify
  H4 against real cache.

**Evidence:**
- goscape `modules/world/interaction.go:600-672`
- TS `LostCityRS/Engine-TS/src/engine/entity/PathingEntity.ts:457-476`
- `pkg/pathfinder/routefinder/routefinder.go:107, 117-153, 165-180`
- Repro A/B commit `9c88e3b`, captured Route value pinned in `t.Skip`

### H5 — Post-FindPath waypoint discard / interaction-state reset

**Verdict:** PARTIAL — DIAGNOSIS-CEILING.

**Tickloop call-graph (path-produce → step-consume):**

- Path producer (per-interaction, once): `interaction.go:236-239` —
  `if !p.repathed { p.pathToTarget(); p.repathed = true }`. Routes to
  `pathToTargetSmart` (`:604-672`) → `pf.FindPathToEntity` /
  `FindPathToLoc` / `FindPathPlain` per target class.
- Click-walk producer: `movement.go:133-153` `pathToMoveClick` →
  `FindPathPlain` (shape-blind 1×1) when MOVE_GAMECLICK fires; queued
  separately from interaction's pathToTarget output.
- Path consumer: `movement.go:34-78` `resolveMovement` → `stepOnce`
  (`:81-115`). Per-step gate: `gamemap.CanTravel` (`gamemap.go:97-101`)
  via `StepValidator.CanTravel(...,srcSize=1, extraFlag=0, TypeNormal)`
  — hardcoded srcSize=1 regardless of player size.
- Step-consume → waypointIndex transitions:
  - `dir == -1` (already at waypoint coord) → decrement waypointIndex
    (`movement.go:88`)
  - CanTravel false → `waypointIndex = -1` (`movement.go:96`)
  - Reached final dest → decrement to -1 (`movement.go:112`)
- `repathed` flag (`interaction.go:87, 135, 238`) gates `pathToTarget`
  to ONCE per interaction (set false on SetInteraction/ClearInteraction;
  set true after pathToTarget). Subsequent ticks rely on the originally
  queued waypoints; no automatic re-pathing on path exhaustion.
- "I can't reach that" mutator (`interaction.go:255-258`) fires when
  `!interacted && !p.hasWaypoints() && p.stepsTaken == 0`, then calls
  `ClearInteraction()` which nulls target + waypointIndex.
- defaultOp NIH fallback (`interaction.go:454`) — fires inside contact
  range when no [op…] script registered; clears waypointIndex via the
  "Nothing interesting happens." path.

**Smoking-gun mutators identified:** none in static read.
- The repathed-once-per-interaction gate is intentional and matches TS
  PathingEntity.ts behavior.
- The CanTravel-fail abandonment (`movement.go:96`) requires that the
  step that the BFS-built path expects fails the StepValidator's
  CanTravel check on the actual FlagMap. This can happen if the BFS
  `routeFindSize1` and `StepValidator.CanTravel` have semantically
  divergent CanMove predicates (e.g., differing handling of FlagLoc /
  FlagWall* combinations). Static reading of both code paths shows they
  use the same `collision.CanMove` helper from `strategies.go:25-33`,
  but the BFS expansion's per-direction `clipFlag` (e.g., FlagBlockWest,
  FlagBlockEast at `routefinder.go:187, 197`) vs StepValidator's
  per-step direction-derived flag may select different bit-patterns
  for diagonal moves — this is investigation-cost-prohibitive without
  runtime instrumentation and was not pursued in this audit.
- The "I can't reach that" + ClearInteraction path is reachable on a
  legitimate exhausted-path-out-of-range smoke (player walked the path
  but pathfinder's reach predicate didn't consider the final tile
  in operable distance). This is consistent with the smoke trace
  "waypoint_idx=1 → -1 in two ticks, target_still_set=false."

**Smoke-trace mapping:**
- "tick N: waypoint_idx=1, repathed=true, steps_taken=0" — pathToTarget
  ran THIS tick (repathed=true), produced a path of length 2
  (waypointIndex points at index 1), but resolveMovement ran BEFORE
  pathToTarget so no step taken yet.
- "tick N+1: waypoint_idx=-1, target_still_set=false" — resolveMovement
  consumed the path (either CanTravel-failed mid-route OR walked to the
  final tile and decremented past -1); processInteraction's
  "I can't reach that" branch fired because reach predicate evaluated
  false at the new player tile + no remaining waypoints + stepsTaken==0
  on that tick (post-step interact arm).

**Why DIAGNOSIS-CEILING:** the actual mechanism is one of:
1. CanTravel/CanMove predicate divergence between BFS and StepValidator
   (sub-H6 candidate; vs `2004scape/rsmod-pathfinder` Rust source).
2. Pathfinder produced a 0- or 1-step path that doesn't reach operable
   distance, and the repathed-once gate prevents re-pathing.
3. Real-cache FlagMap has unallocated zones that block pathfinder
   expansion at production time (per `empty_flagmap_degenerate_routefinder`).

Distinguishing (1) vs (2) vs (3) requires either runtime instrumentation
of `pf.FindPathToEntity` return values + per-step CanTravel results, or
a real-cache integration test that loads m48_50 + replays
populateStaticLocsIntoZones + calls FindPathToEntity for the smoke
geometry and asserts a path.

**Evidence:**
- `modules/world/interaction.go:87, 135, 171-294, 454`
- `modules/world/movement.go:34-115, 133-153, 181-190`
- `pkg/gamemap/gamemap.go:97-101`
- `pkg/pathfinder/routefinder/routefinder.go:107, 165-180, 165+`
- `pkg/pathfinder/collision/strategies.go:25-33`

## Root cause

**Diagnosis ceiling.** Static audit eliminates H1-H4 as code-divergence
sources (decoder, PostDecode, ChangeLocCollision, dispatch arms all
match TS byte-for-byte). H5 maps the call-graph but no static-readable
mutator surfaces as a smoking gun. The smoke trace is consistent with
multiple residual mechanisms: pathfinder/StepValidator predicate
divergence, repathed-once-per-interaction gate combined with a too-short
BFS path, or rsmod-pathfinder port divergence vs the Rust source.
NAI-98 needs (a) a real-cache integration test that exercises the smoke
geometry against the production loader, and (b) runtime instrumentation
of `pf.FindPathToEntity` return values + per-step CanTravel results to
break through to a definitive root cause.

## Stage 2 (NAI-98) handoff

- **Root cause:** UNDETERMINED — diagnosis ceiling at H5 static audit;
  static-divergence hypotheses H1-H4 ruled out. Sub-hypotheses for
  NAI-98 to narrow:
  - **Sub-H6** — `pkg/pathfinder/routefinder/routefinder.go` BFS expansion
    or `pkg/pathfinder/reach.Reached` predicate divergent from the Rust
    `2004scape/rsmod-pathfinder` source (verify against
    `rsmod-pathfinder` branch 225 per `rust_source_canonical_path`).
  - **Sub-H7** — StepValidator's per-step CanTravel predicate disagrees
    with the BFS's CanMove predicate, causing mid-route abandonment
    when a path-tile-reachable-in-BFS fails CanTravel.
  - **Sub-H8** — repathed-once-per-interaction (`interaction.go:236-239`)
    is correct vs TS but combined with a short-or-empty initial path
    produces "walked partial path then abandons" smoke shape; needs
    runtime path-length capture.
- **Repro tests to lift skip on:**
  - `TestNAI97_NPC943_PathAroundFountain` (`pkg/pathfinder/routefinder/nai97_repro_test.go`)
    — currently SKIP on empty-grid degenerate symptom. Lift requires
    re-pointing the test at a real-cache fixture (load m48_50 +
    populateStaticLocsIntoZones replay) so the FlagMap contains real
    bush/wall/floor flags around the smoke geometry. Expected post-fix
    behavior: `Route.Success=true`, `Route.Waypoints` non-empty, last
    waypoint within cheb=1 of (3218, 3216).
  - `TestNAI97_NPC3_MidRouteAbandonment` — same, but for (3218,3213) →
    (3223,3216) geometry; last waypoint within cheb=1 of (3223,3216).
- **Files NAI-98 will touch:** depends on which sub-hypothesis fires:
  - If Sub-H6: `pkg/pathfinder/routefinder/routefinder.go` and/or
    `pkg/pathfinder/reach/`
  - If Sub-H7: `pkg/pathfinder/stepvalidator.go` and the BFS predicate
    in `routefinder.go`
  - If Sub-H8: `modules/world/interaction.go:236-239` repathed gate
  - In all cases: `pkg/pathfinder/routefinder/nai97_repro_test.go`
    (lift skip with real-cache fixture)
- **Estimated LOC for fix:** unknown; ranges from ~5 LOC (predicate
  bit-flag fix) to ~50+ LOC (BFS expansion semantics rework).
- **Residual hypotheses for NAI-99+:** none beyond the Sub-H6/7/8
  scoped above.
- **Stale memory to update during NAI-98 close:**
  - `pathfinder_api_loc_aware.md` references `FindPathDefault` which
    was renamed to `FindPathPlain` (per Bundle 0 Step 1.4).
- **Smoke spec:** post-fix smoke must confirm both Repro A (player at
  (3221,3218) reaches adjacent to NPC 943 at (3218,3216)) and Repro B
  (player at (3218,3213) reaches NPC 3 at (3223,3216)) without "I can't
  reach that" abandonment.
- **Adjacent finding (not Stage 2 work):** Stage 1.1 dump shows
  `loc x=3217 z=3218 layer=0 locTypeID=1516 name="openbankdoor_l" BlockWalk=true Active=1`.
  Per memory `nai_followups`, openbankdoor_l symptom traces to parked
  DISPLAYNAME opcode 2016 residual (out-of-band), not collision.
  No NAI-98 action.
- **Diagnostic note:** `routefinder.go:110` shows informational
  staticcheck QF1003 ("could use tagged switch on srcSize"). Not a
  bug; not in NAI-97 scope (investigation only); flagged here for
  awareness. NAI-98 may or may not address depending on whether it
  touches the file.
