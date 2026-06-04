# NAI-99 Diagnosis — Multi-tile Loc Footprint Coverage Investigation

**Stage 1 of `investigation_subspec_cadence`.** Stage 2 routes to NAI-100.

**Spec:** `docs/superpowers/specs/2026-05-05-nai-99-multi-tile-loc-footprint-investigation-design.md`
**Plan:** `docs/superpowers/plans/2026-05-05-nai-99-multi-tile-loc-footprint-investigation.md`

---

## Summary

Stage 1.1 dump surfaced a smoking-gun hypothesis (**H5**) not in the original H1–H4 register: `pkg/gamemap/load.go:190` constructs every static `*entity.Loc` with hardcoded `width=1, length=1`, ignoring the LocType's `Width`/`Length` fields. The TODO at `pkg/gamemap/load.go:135-136` (`// Footprint hardcoded to 1x1 until LocType config loading lands. // TODO(loctype): use LocType.Width/Length.`) explicitly acknowledges the deferred work. H1–H4 are all ELIMINATED by the dump alone. NAI-100 is a small, mechanical fix.

---

## Audit baseline (Bundle 0 controller pre-flight)

```
NAI-99 Bundle 0 pre-flight at HEAD 3c9e3f9:
- Step 1.2 LayerGroundDecor branch: match (gamemap.go:73 — case loc.LayerGroundDecor → ChangeFloor when active==1)
- Step 1.3 ChangeFloor single-tile: confirmed (api.go:74-80, no W×L loop)
- Step 1.4 ChangeLoc W×L: confirmed (api.go:91 — for index := 0; index < width*length; index++)
- Step 1.5 server.go BlockWalk gate: confirmed (server.go:318 — `if lt := s.locTypeOrNil(loc.Type()); lt != nil && lt.BlockWalk`)
- Step 1.6 Loc accessors: confirmed (loc.go:49-59 Type/Shape/Angle/Layer)
- Step 1.7 FlagMap.Get: confirmed (flagmap.go:30)
- Step 1.8 fixture: available (m48_50 + loc.dat present)
```

---

## Reproducer test results

| Test | Path | Disposition | Notes |
|---|---|---|---|
| `TestNAI99_FountainFootprintDump_Lumbridge` | `pkg/gamemap/nai99_fountain_dump_test.go` | PASS — dump captured | bbox widened to z∈[3214,3228] in commit `00d1b74` after the initial bbox missed the fountain (NAI-99 T2-followup). 64 loc instances; fountain at (3221, 3226) confirmed. Only 1 tile flagged of 4-tile expected footprint. |
| `TestNAI99_FountainCoverage_Lumbridge` | same | SKIP-pinned (commit `3a15435`) | Reproduces — global StaticLocs scan finds 39 fountain instances of typeID=879; first instance asserted is at (2556,3113); identical divergence at Lumbridge (3221,3226) confirmed in instance 34 of the same run. |

Skip-pin (verbatim from Step 6.3 run, copied into the test body):

```
NAI-99 instance 0: typeID=879 origin=(2556,3113,0) shape=10 angle=3 W=2 L=2 (rotated W=2 L=2) flagged=[(2556,3113)=0x100] unflagged=[(2557,3113)=0x0 (2556,3114)=0x0 (2557,3114)=0x0]
NAI-99: instance 0 footprint coverage divergence — flagged=[(2556,3113)=0x100] unflagged=[(2557,3113)=0x0 (2556,3114)=0x0 (2557,3114)=0x0] expected all 4 tiles flagged
```

---

## Per-hypothesis verdicts

### H1 — adjacent-loc / BlockWalk-gating

**Verdict:** ELIMINATED.

**Evidence:**
- Fountain LocType has `BlockWalk=true, BlockRange=false, Active=1` per dump line 9 (`loc x=3221 z=3226 ... locTypeID=879 name="fountain" W=2 L=2 BlockWalk=true BlockRange=false Active=1`).
- The `populateStaticLocsIntoZones` BlockWalk gate at `modules/world/server.go:318` is satisfied; collision write fires.
- Stuck-tile candidate inside fountain footprint: only `(3221, 3226) = 0x100` (FlagLoc) flagged in `$TMPDIR/nai99_dump.log` lines 90 (only 1 of 4 expected). The producing loc IS the fountain itself, not an adjacent loc.

**Implication:** The fountain's own collision write is firing but only writes 1 tile. Not an adjacent-loc problem. Closed.

### H2 — single multi-tile GroundDecor + `ChangeFloor` single-tile

**Verdict:** ELIMINATED (mechanical; no Rust rsmod-pathfinder cross-check needed).

**Evidence:**
- Fountain `Shape() = 10 = ShapeCentrepieceStraight` (per `pkg/pathfinder/loc/shape.go:16`), which `LayerOf` maps to `LayerGround` (per `pkg/pathfinder/loc/layer.go:27-40`). The dump confirms `layer=2` (LayerGround = iota value 2 in `pkg/pathfinder/loc/layer.go:10`).
- `LayerGround` routes through `gm.Pathfinder.ChangeLoc` at `pkg/gamemap/gamemap.go:67-71`, which IS W×L-aware (`for index := 0; index < width*length; index++` at `pkg/pathfinder/routefinder/api.go:91`).
- The fountain does NOT route through the single-tile `ChangeFloor` path (which is `LayerGroundDecor` only).
- The bug is therefore upstream of `ChangeFloor`/`ChangeLoc` — the W×L values being passed to `ChangeLoc` are wrong (1, 1 instead of 2, 2).

**Implication:** The `ChangeFloor` single-tile question is moot for this fountain. Rust rsmod-pathfinder cross-check skipped per spec §3 short-circuit policy. Closed.

### H3 — N adjacent single-tile loc placements

**Verdict:** ELIMINATED.

**Evidence:**
- Fountain typeID=879 has a SINGLE static-loc instance in the bbox: `(3221, 3226, level=0)`. Globally, 39 instances of typeID=879, but each is a single multi-tile placement at distinct coords (controller scratch global scan).
- LocType has `W=2, L=2` (`pkg/objtype/loctype.go:181-182` defaults; per-LocType decoded values from `loc.dat`).

**Implication:** Not a content-side composition. Closed.

### H4 — l-pack per-instance Shape decode divergence

**Verdict:** ELIMINATED (mechanical; no l-pack decoder diff needed).

**Evidence:**
- Per-instance `Shape() = 10` matches expected centrepiece shape for a fountain LocType (`ShapeCentrepieceStraight` → `LayerGround`).
- The decoded shape correctly routes through `LayerGround` W×L path. The shape decode is not the problem.
- Per-instance Width/Length is the actual divergence — but those fields aren't in the l-pack stream; they come from `entity.NewLoc(..., width, length, ...)` at `pkg/gamemap/load.go:190`, which hardcodes `1, 1`.

**Implication:** Shape decode is fine. Closed.

### H5 (NEW) — static-loc loader hardcodes per-instance Width/Length to 1,1

**Verdict:** CONFIRMED — root cause.

**Evidence (file:line):**
- `pkg/gamemap/load.go:190` — `loc := entity.NewLoc(actualLevel, absX, absZ, 1, 1, entity.LifecycleRespawn, locID, shape, angle)` — passes hardcoded `1, 1` for `width, length`.
- `pkg/gamemap/load.go:135-136` — explicit `// Footprint hardcoded to 1x1 until LocType config loading lands. // TODO(loctype): use LocType.Width/Length.` acknowledges the deferred work.
- `modules/world/server.go:319-320` — `populateStaticLocsIntoZones` calls `s.gamemap.ChangeLocCollision(loc.Shape(), loc.Angle(), lt.BlockRange, loc.Length, loc.Width, lt.Active, loc.X, loc.Z, loc.Level, true)` — `loc.Length` and `loc.Width` are entity fields hardcoded to 1 by the loader, NOT `lt.Length`/`lt.Width`.
- `pkg/pathfinder/routefinder/api.go:82-99` — `ChangeLoc` loops `width*length`. With `width=1, length=1`, only 1 tile is written regardless of the LocType's actual footprint.
- Reproducer assertion: `TestNAI99_FountainCoverage_Lumbridge` shows 39 fountain instances ALL exhibit single-tile coverage of their declared 2×2 footprint.

**Implication:** NAI-100 fix is mechanical. Lift the TODO at `load.go:135-136` and pass `lt.Width, lt.Length` to `entity.NewLoc`. The loader needs the LocTypes registry (currently loaded by a different module); the fix may need to either:
1. Thread `*objtype.LocTypeConfigs` into `gamemap.Init` / `loadLocs` so per-instance dimensions can be looked up at load time, OR
2. Reorder boot-time so static-loc footprints are corrected after LocTypes load (e.g., a post-load fixup pass), OR
3. Defer footprint resolution: change `populateStaticLocsIntoZones` to read `lt.Width, lt.Length` directly from `s.locTypes` instead of trusting `loc.Width, loc.Length`. Lower-impact, but leaves entity fields wrong for later consumers (zone management, AddLoc/RemoveLoc paths). NAI-100 will weigh.

---

## Root cause

`pkg/gamemap/load.go:190` constructs every static `*entity.Loc` with hardcoded `width=1, length=1`. The LocType registry has the correct `W=2, L=2` for multi-tile features (verified for fountain typeID=879), but those values never reach the entity. Production callers (`modules/world/server.go:319-320`, zone collision-write paths) read `loc.Width`/`loc.Length` from the entity and pass `1, 1` to `ChangeLocCollision` → `ChangeLoc`, which loops only 1×1=1 iteration regardless of the W×L pathfinder API support.

---

## Stage 2 (NAI-100) handoff

- **Root cause:** `pkg/gamemap/load.go:190` hardcodes `entity.NewLoc(..., 1, 1, ...)`. TODO at `pkg/gamemap/load.go:135-136` acknowledges.
- **Repro tests to lift skip on:** `pkg/gamemap/nai99_fountain_dump_test.go::TestNAI99_FountainCoverage_Lumbridge` — expected post-fix behavior: instance 0 `unflagged=[]`; all `width*length` (typically 4 for the fountain) tiles carry FlagLoc=0x100.
- **Files NAI-100 will touch:** at minimum `pkg/gamemap/load.go` (consume `lt.Width`/`lt.Length`); likely `pkg/gamemap/gamemap.go` (`Init` signature/wiring) to thread `*objtype.LocTypeConfigs`; possibly `modules/world/server.go` if the boot order needs adjustment. Decision between approaches 1/2/3 above is for NAI-100 brainstorm.
- **Estimated LOC for fix:** small — ~10 LOC for the loader change, plus wiring to thread LocTypes into `gamemap.Init` (~30 LOC across files). Approach 3 (server.go reads `lt.Width/Length` directly) is even smaller (~5 LOC) but leaves entity-field divergence as a known limitation.
- **Residual hypotheses for NAI-101+:** none surfaced. The diagnosis is clean.
- **Smoke spec:** walk NW from Lumbridge spawn (3221, 3218) into the fountain footprint at (3221..3222, 3226..3227); verify the player can no longer walk onto any of those 4 tiles; verify path-around routes correctly to NPCs on the far side of the fountain.

---

## Notes on plan deviations

- Tasks 4 (H2 Rust rsmod cross-check) and 5 (H4 l-pack decoder diff) were skipped per the spec's §3 short-circuit policy. Both hypotheses were mechanically eliminable from the Stage 1.1 dump alone (fountain shape=10 → LayerGround route, not GroundDecor; shape decoded correctly). The Rust audit and TS l-pack diff would have added cost without changing the verdict.
- Task 2's initial bbox (`x∈[3217,3225] z∈[3214,3220]`) missed the fountain entirely (z=3226 is north of zHi=3220). A controller scratch global-name scan (uncommitted) located the fountain at (3221, 3226), and Task 2 was followed up with a bbox widen (commit `00d1b74`) to z∈[3214,3228].
- Task 6's coverage test asserts on the FIRST static-loc instance of typeID=879 (which is at `(2556,3113)`, not Lumbridge's `(3221,3226)`). The bug is identical at both — instance 34 of the same scan exhibits the Lumbridge symptom. Skip-pin uses instance 0's verbatim values per `skip_pin_full_struct_capture`.
