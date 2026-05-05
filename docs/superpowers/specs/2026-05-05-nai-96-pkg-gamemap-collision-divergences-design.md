# NAI-96 — `pkg/gamemap/` collision-write divergences (NAI-95 smoke residuals)

**Status:** spec — bundled sub-spec, two parallel tasks under one plan.
**Cadence:** standard `runescript_cadence` — brainstorm → spec → plan → subagent-driven TDD → user-launched smoke at close.
**Tech stack:** Go 1.26+.
**Parent:** NAI-95 — `docs/superpowers/specs/2026-05-05-nai-95-static-loc-collision-fix-design.md` (closed `cae1ad8`); user-launched smoke 2026-05-05 surfaced these residuals.
**TS reference:** `LostCityRS/Engine-TS` (per `ts_source_canonical_path`). Primary: `src/engine/GameMap.ts:24-26` constants, `:182-217` `loadGround` two-pass, `:220-267` `loadLocations` LINK_BELOW level adjustment, `:326-341` `changeLocCollision` layer dispatch.

---

## 1. Context & motivation

NAI-95 wired static-loc collision at world init (`populateStaticLocsIntoZones`), unblocking the NAI-92/NAI-94 root cause. User-launched smoke (2026-05-05) confirmed NPC dispatch surface works (clicked NPC at (3219, 3230), reached interaction range, chat dialog opened). The smoke surfaced two adjacent `pkg/gamemap/` divergences not in NAI-95's scope:

1. **Hans path detour** — User reported "very difficult to move" and "Pathing directly to Hans was not possible." Tile (3219, 3223) on Hans's straight-line-south path got spurious `FlagBlockWalk`, forcing a 3-waypoint detour. Root cause: `pkg/gamemap/load.go:9` defines `gameMapBlockMapSquare = 0x2`. TS `GameMap.ts:24-25` defines `BLOCK_MAP_SQUARE = 0x1` and `LINK_BELOW = 0x2`. Goscape mis-marks `LINK_BELOW` (bridge) tiles as floor-blocked AND misses real `BLOCK_MAP_SQUARE` tiles.

2. **Lumbridge fountain walk-through** — User walked into the Lumbridge fountain (a GroundDecor loc with `Active=1`, `BlockWalk=true`). Root cause: `pkg/gamemap/gamemap.go:60` `ChangeLocCollision` switch silently skips `LayerGroundDecor`. TS `GameMap.ts:336-340` writes `changeFloor` for `GROUND_DECOR` when `active === 1`. Goscape's switch has no `LayerGroundDecor` case.

Per `audit_full_method_against_ts`, end-to-end TS comparison of the two functions surfaced two adjacent divergences in the same code being touched, both folded in:

3. **`REMOVE_ROOFS=0x4` roof-collision write missing** in `loadGround`. TS `GameMap.ts:200-202` writes roof collision before the BLOCK_MAP_SQUARE check. Free to add once `loadGround` is two-pass-restructured for LINK_BELOW.

4. **Angle-aware width/length swap missing** in `ChangeLocCollision` LayerGround branch. TS `GameMap.ts:331-334` swaps `(length, width)` for N/S vs `(width, length)` for E/W. Goscape unconditionally passes `(width, length)`. Currently masked for static locs (1×1 hardcode at `load.go:128`); live for any script-driven `AddLoc` with a multi-tile non-square loc.

---

## 2. Scope

### In scope

**Task 1 — `loadGround` two-pass restructure** (`pkg/gamemap/load.go`, `pkg/gamemap/gamemap.go`):

- Flip `gameMapBlockMapSquare` from `0x2` to `0x1`. Add `gameMapLinkBelow = 0x2` and `gameMapRemoveRoofs = 0x4`.
- Restructure `loadGround` into two passes per TS `GameMap.ts:182-217`:
  - **Pass 1:** parse all 4 levels × 64 × 64 land bytes into a `[mapLevels * mapSquareSize * mapSquareSize]int8` buffer indexed by `packCoord(x,z,level) = (z & 0x3F) | ((x & 0x3F) << 6) | ((level & 0x3) << 12)`.
  - **Pass 2:** iterate every tile; if `land & REMOVE_ROOFS != 0` call `Pathfinder.ChangeRoof(absX, absZ, level, true)`; if `land & BLOCK_MAP_SQUARE == 0` continue; compute `bridged := (level==1 ? land : lands[packCoord(x,z,1)]) & LINK_BELOW != 0`; `actualLevel := bridged ? level-1 : level`; skip if `actualLevel < 0`; `Pathfinder.ChangeFloor(absX, absZ, actualLevel, true)`.
- Persist the per-mapsquare `lands` buffer on `GameMap` so `loadLocs` can read it. Add field `landsByMapSquare map[uint16][]int8` (key `(mapX<<8)|mapZ`, mirroring `mData`/`lData` convention at `gamemap.go:26-27`).
- `loadLocs` reads `lands := gm.landsByMapSquare[key]` and applies the same LINK_BELOW level adjustment per TS `GameMap.ts:242-246`: `bridged := (level==1 ? lands[coord] : lands[packCoord(x,z,1)]) & LINK_BELOW != 0`; `actualLevel := bridged ? level-1 : level`; skip if `actualLevel < 0`; persist the loc with `actualLevel`.
- Remove `TODO(bridged-levels)` at `load.go:91`.

**Task 2 — `ChangeLocCollision` layer dispatch** (`pkg/gamemap/gamemap.go` + 7 caller sites):

- Add `active int` parameter to `ChangeLocCollision` between `width` and `x` (TS arg-order parity per `flat_arg_signature_for_cross_lang_parity`). New signature:

  ```go
  func (gm *GameMap) ChangeLocCollision(shape, angle int, blocksRange bool, length, width, active, x, z, level int, add bool)
  ```

- Add `case loc.LayerGroundDecor:` branch: `if active == 1 { gm.Pathfinder.ChangeFloor(x, z, level, add) }`.
- Fix LayerGround branch to swap `(length, width)` for N/S vs `(width, length)` for E/W per TS `GameMap.ts:331-334`. Use `loc.LocAngle` constants (verify exact constant names at plan-write).
- Keep `LayerWallDecor` skip; doc-comment as `// LayerWallDecor: TS skips (GameMap.ts:326-340 has no WALL_DECOR branch)` per `defensive_gate_doc_comment_label`.
- Update 7 caller sites to pass `lt.Active` (or `oldLt.Active` / `newLt.Active`):
  - `modules/world/world_zone.go:20` (`AddLoc`)
  - `modules/world/world_zone.go:49` (`ChangeLoc` remove old, `oldLt.Active`)
  - `modules/world/world_zone.go:56` (`ChangeLoc` add new, `newLt.Active`)
  - `modules/world/world_zone.go:82` (`RemoveLoc`)
  - `modules/world/loc_turn.go:42` (`RevertLoc` remove old, `oldLt.Active`)
  - `modules/world/loc_turn.go:49` (`RevertLoc` add new, `newLt.Active`)
  - `modules/world/server.go:328` (`populateStaticLocsIntoZones`)

### Out of scope

- DISPLAYNAME opcode handler (separate NAI-95 residual; deferred).
- Survival Expert reach (continues on existing NAI-92/NAI-94 followup tracker; pre-existing pathfinder reach issue, not a `pkg/gamemap/` divergence).
- LocType-aware `Width`/`Length` for static locs (existing `TODO(loctype)` at `load.go:90`; multi-tile static loc footprints will become correct once that lands AND Task 2's angle-swap is in place — interlocked but separable).
- Free-to-play / membership filtering at TS `GameMap.ts:189-191, 238-240` (goscape doesn't enforce; existing untracked TS-fidelity gap, surface separately if smoke surfaces it).
- Roof-removal LOS consumer changes — Task 1 writes the flags; consumer audit (whether anything reads `FlagRoof`) is out of scope.

---

## 3. Task 1 — `loadGround` two-pass + LINK_BELOW + REMOVE_ROOFS

### 3.1 Production changes (`pkg/gamemap/load.go`, `pkg/gamemap/gamemap.go`)

Constants block at `load.go:8-13`:

```go
const (
    gameMapBlockMapSquare = 0x1 // BLOCK_MAP_SQUARE — marks a tile as blocked floor (TS GameMap.ts:24)
    gameMapLinkBelow      = 0x2 // LINK_BELOW — bridge tile; collision drops to level-1 (TS GameMap.ts:25)
    gameMapRemoveRoofs    = 0x4 // REMOVE_ROOFS — write roof collision (TS GameMap.ts:26)

    mapSquareSize = 64
    mapLevels     = 4
)
```

`GameMap` struct at `gamemap.go:22-31` adds:

```go
landsByMapSquare map[uint16][]int8 // (mapX<<8)|mapZ -> 4*64*64 land bytes, populated by loadGround, consumed by loadLocs
```

initialised in `New(log)` alongside `mData`/`lData`.

`loadGround` rewritten as two-pass. Reference structure (mentally-compiled per `plan_runnable_test_fixtures`):

```go
func (gm *GameMap) loadGround(data []byte, mapSquareX, mapSquareZ int) {
    p := packet.NewPacket(data)
    lands := make([]int8, mapLevels*mapSquareSize*mapSquareSize)

    // Pass 1: parse land bytes into lands[packCoord].
    for level := 0; level < mapLevels; level++ {
        for x := 0; x < mapSquareSize; x++ {
            for z := 0; z < mapSquareSize; z++ {
                for {
                    if p.Len() == 0 {
                        gm.landsByMapSquare[uint16((mapSquareX<<8)|mapSquareZ)] = lands
                        return
                    }
                    op := p.G1()
                    if op == 0 {
                        break
                    }
                    if op == 1 {
                        if p.Len() >= 1 {
                            _ = p.G1() // height
                        }
                        break
                    }
                    if op <= 49 {
                        if p.Len() >= 3 {
                            _ = p.Next(3) // overlay
                        }
                        continue
                    }
                    lands[packCoord(x, z, level)] = int8(op) - 49
                    break
                }
            }
        }
    }
    gm.landsByMapSquare[uint16((mapSquareX<<8)|mapSquareZ)] = lands

    // Pass 2: write collision per TS GameMap.ts:182-217.
    for level := 0; level < mapLevels; level++ {
        for x := 0; x < mapSquareSize; x++ {
            absX := mapSquareX*mapSquareSize + x
            for z := 0; z < mapSquareSize; z++ {
                absZ := mapSquareZ*mapSquareSize + z
                land := lands[packCoord(x, z, level)]

                if land&gameMapRemoveRoofs != 0 {
                    gm.Pathfinder.ChangeRoof(absX, absZ, level, true)
                }
                if land&gameMapBlockMapSquare == 0 {
                    continue
                }

                var bridgeLand int8
                if level == 1 {
                    bridgeLand = land
                } else {
                    bridgeLand = lands[packCoord(x, z, 1)]
                }
                bridged := bridgeLand&gameMapLinkBelow != 0
                actualLevel := level
                if bridged {
                    actualLevel = level - 1
                }
                if actualLevel < 0 {
                    continue
                }
                gm.Pathfinder.ChangeFloor(absX, absZ, actualLevel, true)
            }
        }
    }
}

func packCoord(x, z, level int) int {
    return (z & 0x3F) | ((x & 0x3F) << 6) | ((level & 0x3) << 12)
}
```

Plan-author verifies `Pathfinder.ChangeRoof` signature matches the call shape (audit `pkg/pathfinder/routefinder/api.go` at plan-write).

`loadLocs` updated to consume `lands`:

```go
func (gm *GameMap) loadLocs(data []byte, mapSquareX, mapSquareZ int) {
    p := packet.NewPacket(data)
    lands := gm.landsByMapSquare[uint16((mapSquareX<<8)|mapSquareZ)]
    locID := -1
    for {
        // ... existing locID/coord delta loop ...
        coord := 0
        for {
            // ... read coordDelta, info ...
            level := (coord >> 12) & 0x3

            actualLevel := level
            if lands != nil {
                var bridgeLand int8
                if level == 1 {
                    bridgeLand = lands[coord]
                } else {
                    bridgeLand = lands[packCoord(localX, localZ, 1)]
                }
                if bridgeLand&gameMapLinkBelow != 0 {
                    actualLevel = level - 1
                }
            }
            if actualLevel < 0 {
                continue
            }

            loc := entity.NewLoc(actualLevel, absX, absZ, 1, 1,
                entity.LifecycleRespawn,
                locID, shape, angle)
            gm.staticLocs = append(gm.staticLocs, loc)
        }
    }
}
```

`lands == nil` fallback preserves correctness when `loadLocs` is called before `loadGround` (currently impossible per `Init` ordering, but defensive — and matches TS implicit behavior where `lands` is always populated by the same `decode` pass).

Remove `TODO(bridged-levels): honour LINK_BELOW` at `load.go:91` (closed by this task).

### 3.2 Tests (new file `pkg/gamemap/load_test.go`)

Each test builds a synthetic m-file byte stream and calls `loadGround` directly on a fresh `GameMap`. Helpers:

```go
// mFileWithLand(level, x, z, land) builds a minimal m-file where the named tile
// has the given land value (encoded as opcode = land + 49) and all other tiles
// terminate with opcode 0. Returns the full mapLevels*mapSquareSize*mapSquareSize
// stream length.
func mFileWithLand(level, x, z int, land byte) []byte { ... }
```

Tests:

| Test | Setup | Assertion |
|---|---|---|
| `TestLoadGround_BlockMapSquare_WritesFloorBlock` | tile (1,1,0) with land=0x1 | `gm.Pathfinder.flag(1,1,0)` includes FlagFloorBlock; (1,2,0) does not |
| `TestLoadGround_LinkBelowOnly_DoesNotBlock` | tile (1,1,0) with land=0x2 (LINK_BELOW only, no BLOCK_MAP_SQUARE) | no FlagFloorBlock written at level 0 — confirms constant flip |
| `TestLoadGround_RemoveRoofs_WritesRoof` | tile (1,1,0) with land=0x4 | `gm.Pathfinder.flag(1,1,0)` includes FlagRoof; FlagFloorBlock unset |
| `TestLoadGround_BlockAndRemoveRoofs_BothWritten` | land=0x5 | both FlagFloorBlock and FlagRoof |
| `TestLoadGround_BridgedLevel0_DropsToLevelMinus1_Skipped` | tile (1,1,0) land=0x1 (BLOCK); tile (1,1,1) land=0x2 (LINK_BELOW set at level 1) | no FlagFloorBlock at level 0 (actualLevel = -1 skipped) |
| `TestLoadGround_BridgedLevel1_WritesAtLevel0` | tile (1,1,1) land=0x3 (BLOCK \| LINK_BELOW) | FlagFloorBlock at level 0; level 1 unchanged |
| `TestLoadGround_NonBridgedLevel1_WritesAtLevel1` | tile (1,1,1) land=0x1 (BLOCK only); level 1 of (1,1,1) does NOT have LINK_BELOW | FlagFloorBlock at level 1; level 0 unchanged |
| `TestLoadLocs_BridgedLoc_PlacedAtActualLevel` | l-file places a loc with `BlockWalk=true` at level 1, with `lands[(1,1,1)] = LINK_BELOW` set | `staticLocs[i].Level == 0`; collision write happens via `populateStaticLocsIntoZones` (separate test or assert level on the entity) |

Pre-allocate the `Pathfinder` zones touched by each scenario via the existing `Pathfinder.AllocateIfAbsent`-equivalent path before flag reads (per `empty_flagmap_degenerate_routefinder`). Plan-author confirms the exact API name at plan-write.

Hand-crafted byte stream sample (BLOCK_MAP_SQUARE at (1,1,0)):

```
(level 0, x=0, z=0..63):
  for each tile: opcode 0 (terminator)               → 64 bytes
(level 0, x=1, z=0..0):
  tile (1,0): opcode 0
(level 0, x=1, z=1):
  opcode (1 + 49) = 50  (land = 0x1)
(level 0, x=1, z=2..63): opcode 0
(level 0, x=2..63, all z): opcode 0
(level 1..3, all x, all z): opcode 0
```

Total: `4 levels * 64 * 64 = 16384` opcode-0 bytes plus one opcode-50 byte. Helper builds this programmatically.

---

## 4. Task 2 — `ChangeLocCollision` layer dispatch + active + angle-swap

### 4.1 Production changes

`pkg/gamemap/gamemap.go::ChangeLocCollision`:

```go
// ChangeLocCollision updates collision for a loc based on its layer.
// Mirrors TS Engine-TS/src/engine/GameMap.ts:326-341 changeLocCollision.
//
// LayerWall:        ChangeWall.
// LayerGround:      ChangeLoc with angle-aware (length,width) swap.
// LayerGroundDecor: ChangeFloor when active==1 (TS-faithful: GameMap.ts:336-340).
// LayerWallDecor:   no-op (TS skips: GameMap.ts:326-340 has no WALL_DECOR branch).
func (gm *GameMap) ChangeLocCollision(shape, angle int, blocksRange bool, length, width, active, x, z, level int, add bool) {
    layer := loc.LayerOf(loc.Shape(shape))
    switch layer {
    case loc.LayerWall:
        gm.Pathfinder.ChangeWall(x, z, level, angle, shape, blocksRange, false, add)
    case loc.LayerGround:
        if angle == int(loc.AngleNorth) || angle == int(loc.AngleSouth) {
            gm.Pathfinder.ChangeLoc(x, z, level, length, width, blocksRange, false, add)
        } else {
            gm.Pathfinder.ChangeLoc(x, z, level, width, length, blocksRange, false, add)
        }
    case loc.LayerGroundDecor:
        if active == 1 {
            gm.Pathfinder.ChangeFloor(x, z, level, add)
        }
    }
}
```

Plan-author confirms `loc.AngleNorth` / `loc.AngleSouth` constant names by reading `pkg/pathfinder/loc/` at plan-write (per `plan_grep_helper_patterns` — likely `LocAngle` constants exist; if named differently, use the canonical names).

### 4.2 Caller updates (7 sites)

All sites already have an `*objtype.LocType` in scope. Threading:

| File:line | Variable | Change |
|---|---|---|
| `modules/world/world_zone.go:20` | `lt` | add `lt.Active` between `loc.Width` and `loc.X` |
| `modules/world/world_zone.go:49` | `oldLt` | add `oldLt.Active` |
| `modules/world/world_zone.go:56` | `newLt` | add `newLt.Active` |
| `modules/world/world_zone.go:82` | `lt` | add `lt.Active` |
| `modules/world/loc_turn.go:42` | `oldLt` | add `oldLt.Active` |
| `modules/world/loc_turn.go:49` | `newLt` | add `newLt.Active` |
| `modules/world/server.go:328` | `lt` | add `lt.Active` |

Plan-author re-greps these line numbers at HEAD pre-dispatch per `controller_preflight` (NAI-95 close commit `cae1ad8` may have shifted them; the current spec captures them as of post-NAI-95).

### 4.3 Tests (extend `modules/world/static_loc_collision_test.go`)

| Test | Setup | Assertion |
|---|---|---|
| `TestChangeLocCollision_GroundDecor_Active1_WritesFloor` | static loc with shape in GroundDecor range, `LocType.Active=1`, `BlockWalk=true`, at (3220, 3220, 0) | post-`populateStaticLocsIntoZones`: FlagFloorBlock set at (3220, 3220, 0) |
| `TestChangeLocCollision_GroundDecor_Active0_NoWrite` | same but `Active=0` | no FlagFloorBlock at that tile |
| `TestChangeLocCollision_WallDecor_NoWrite` | loc with WallDecor shape, `Active=1`, `BlockWalk=true` | no flag write at that tile (TS skips WallDecor) |
| `TestChangeLocCollision_AngleSwap_North_2x3` | dynamic `s.AddLoc` with width=2, length=3, angle=North, LayerGround shape, `BlockWalk=true` | FlagLoc set at the 3-along-X × 2-along-Z footprint (per N/S → `length, width` swap) |
| `TestChangeLocCollision_AngleSwap_East_2x3` | same loc, angle=East | FlagLoc set at the 2-along-X × 3-along-Z footprint (per E/W → `width, length`) |

Plan-author confirms shape ranges for GroundDecor and WallDecor by reading `pkg/pathfinder/loc/layer.go::LayerOf` at plan-write. Pin one shape value per layer that survives any future refactor (use named constants if available).

For angle-swap tests, assert footprint by reading `Pathfinder.flag(x,z,0)` at each of the 6 covered tiles plus the tiles outside the expected footprint. Use `internal.BuildCollisionMap` or pre-allocate the touched zones.

---

## 5. Deviations from TS

None. All four fixes converge on TS behavior. Existing deviations (`D-N86-2` DESPAWN+!IsActive early-return in `ChangeLoc`) are unrelated and untouched.

The `lands == nil` defensive fallback in `loadLocs` is goscape-only but unreachable in production (Init orders `loadGround` before `loadLocs` for each mapsquare per `gamemap.go:124-129`); doc-comment per `defensive_gate_doc_comment_label`.

---

## 6. Risks & cross-checks

- **`latent_bug_at_migration_boundary`** — flipping `BLOCK_MAP_SQUARE` may surface tiles that were previously incorrectly blocked-as-floor (because their land had bit 0x2 set, mistakenly interpreted as BLOCK). Mitigation: smoke at close (Hans path, Lumbridge fountain).
- **`enumerate_all_sites`** — 7 caller sites for `ChangeLocCollision`; spec lists them verbatim §4.2; controller re-greps at HEAD pre-dispatch.
- **Angle-swap regression coverage** — no goscape test currently pins a multi-tile non-square loc with N-angle. The new `TestChangeLocCollision_AngleSwap_*` tests are the only guards. Plan-author runs a focused grep `rg "AddLoc|s.AddLoc" --type go` for any test that would surface incidentally.
- **`plan_runnable_test_fixtures`** — hand-crafted m-file byte streams need mental dry-run. Spec §3.2 provides the byte-layout pattern; plan-author writes the full helper function and dry-runs one scenario before implementer dispatch.
- **`packet_rw_pointer_gotcha`** — m-file parsing uses `packet.G1`/`packet.GSmart` (read-only); no Pos/Data swap concerns.
- **`controller_preflight`** — controller verifies before each implementer dispatch: (1) `pkg/gamemap/load.go:9` still has `0x2`; (2) the 7 `ChangeLocCollision` call sites are at the line numbers listed; (3) `loc.LayerGroundDecor` is the canonical constant name.

---

## 7. Test strategy summary

- **Task 1**: new `pkg/gamemap/load_test.go`, ~7 unit tests with hand-crafted m-file byte streams. Pure-Go fixtures, no cache files.
- **Task 2**: extend `modules/world/static_loc_collision_test.go` with ~5 cases (GroundDecor active=0/1, WallDecor, angle-swap N/E). Re-uses NAI-95 pattern of asserting `Pathfinder` flags after server boot.
- **Smoke at close** (user-launched per `smoke_test_server_handoff`): Hans straight-line path probe (3219, 3230 → 3219, 3222) — expect ≤2 waypoints (no detour); Lumbridge fountain walk-into — expect blocked.

---

## 8. Out-of-scope followups (parked)

- `LayerWallDecor` write — TS skips, goscape skips. If a future content port needs it (unlikely), separate sub-spec.
- F2P / membership filtering at TS `GameMap.ts:189-191, 238-240` — untracked TS-fidelity gap. Surface separately if smoke surfaces it.
- `FlagRoof` consumer (does anything in goscape read it?) — Task 1 writes; consumer audit deferred.
- DISPLAYNAME opcode handler — separate NAI-95 residual.
- Survival Expert reach — continues on existing NAI-92/NAI-94 followup tracker.
