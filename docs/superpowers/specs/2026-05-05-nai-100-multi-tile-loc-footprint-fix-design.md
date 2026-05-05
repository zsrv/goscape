# NAI-100 — Multi-Tile Loc Footprint Fix (Stage 2 of NAI-99)

**Stage 2 of `investigation_subspec_cadence`.** Closes NAI-99 H5 root cause.

**Predecessor:** `docs/superpowers/specs/2026-05-05-nai-99-multi-tile-loc-footprint-investigation-design.md` + diagnosis at `docs/superpowers/investigations/2026-05-05-nai-99-diagnosis.md`.

**Tech Stack:** Go 1.26+; goscape engine; LostCityRS/Engine-TS canonical reference.

---

## 1. Problem

`pkg/gamemap/load.go:190` constructs every static `*entity.Loc` with hardcoded `width=1, length=1`, ignoring the LocType's `Width`/`Length`. The TODO at `pkg/gamemap/load.go:135-136` (`Footprint hardcoded to 1x1 until LocType config loading lands. TODO(loctype): use LocType.Width/Length.`) acknowledges the deferred work. Downstream:

- `modules/world/server.go:319-320` (`populateStaticLocsIntoZones`) reads `loc.Length, loc.Width` from the entity and passes them to `ChangeLocCollision`. With 1, 1 the collision write loop iterates only one tile regardless of the LocType's actual W×L.
- `modules/world/world_zone.go:21,50,57,83` (runtime AddLoc/ChangeLoc/RemoveLoc paths) read the same entity fields.
- `modules/world/interaction.go:489` and `modules/world/npc_interaction.go:691` pass `loc.Width, loc.Length` to `findApproachPoint` for player→loc and NPC→loc pathfinding; with 1, 1 the approach calculation underestimates the footprint.

Symptom (NAI-99 user smoke 2026-05-05): "fountain treated like 1 tile wide; player walks partway in then stuck." The Lumbridge fountain is `typeID=879, W=2, L=2, BlockWalk=true, Active=1, Shape=10 (LayerGround)`; only `(3221, 3226) = 0x100 (FlagLoc)` is flagged of the expected 4-tile footprint `(3221..3222, 3226..3227)`.

## 2. TS reference

`Engine-TS/src/engine/GameMap.ts:248-263` — within `loadLocations`:

```ts
const type: LocType = LocType.get(locId);
if (!type) {
    printFatalError(`Invalid loc type ${locId} in map m${...}_${...}.jm2`);
    continue;
}

const width: number = type.width;
const length: number = type.length;
const shape: number = info >> 2;
const angle: number = info & 0x3;

if (type.blockwalk) {
    changeLocCollision(shape, angle, type.blockrange, length, width, type.active, absoluteX, absoluteZ, actualLevel, true);
}

this.getZone(absoluteX, absoluteZ, actualLevel).addStaticLoc(new Loc(actualLevel, absoluteX, absoluteZ, width, length, EntityLifeCycle.RESPAWN, locId, shape, angle));
```

TS aborts on missing/invalid locID via `printFatalError`. goscape has no equivalent fatal-error infra; this spec adopts log-warn + 1×1 fallback, tracked as a deviation (D1, §7).

## 3. Architecture

One change point + one wiring change. No new modules.

### 3.1 `pkg/gamemap/gamemap.go` — LocTypes accessor

- Add private field `locTypes *objtype.LocTypeConfigs` to `GameMap` (initialized nil).
- Add method `func (gm *GameMap) SetLocTypes(cfgs *objtype.LocTypeConfigs)` storing into the field.
- Setter MUST be called before `Init` for static-loc footprint correctness; otherwise `loadLocs` falls back to `1, 1` (preserves today's behavior for empty-cache test fixtures — ~14 callers of `gm.Init(t.TempDir())`).

### 3.2 `pkg/gamemap/load.go` — consume LocType.Width/Length

In `loadLocs`, replace the line-190 `entity.NewLoc(actualLevel, absX, absZ, 1, 1, ...)` with a per-instance lookup:

```go
width, length := 1, 1
if gm.locTypes != nil {
    if locID >= 0 && locID < len(gm.locTypes.Configs) {
        if lt := gm.locTypes.Configs[locID]; lt != nil {
            width, length = lt.Width, lt.Length
        } else {
            // (goscape defensive; TS calls printFatalError on missing LocType — see GameMap.ts:249-252)
            gm.log.Warn("loadLocs: nil LocType for locID; using 1x1 fallback",
                "locID", locID, "mapSquareX", mapSquareX, "mapSquareZ", mapSquareZ)
        }
    } else {
        // (goscape defensive; TS calls printFatalError — see GameMap.ts:249-252)
        gm.log.Warn("loadLocs: locID out of range; using 1x1 fallback",
            "locID", locID, "mapSquareX", mapSquareX, "mapSquareZ", mapSquareZ)
    }
}
loc := entity.NewLoc(actualLevel, absX, absZ, width, length,
    entity.LifecycleRespawn,
    locID, shape, angle)
```

Drop the `Footprint hardcoded to 1x1 until LocType config loading lands. TODO(loctype): use LocType.Width/Length.` doc-comment lines (load.go:135-136). Update `loadLocs`'s preceding doc-comment to describe the new lookup behavior.

### 3.3 `modules/world/server.go` — boot reorder

Hoist `objtype.LoadLocTypes(cfg.CachePath)` (currently lines 235-238) above `gm.Init` (currently lines 178-182). Concretely the new order is:

```go
locTypes, err := objtype.LoadLocTypes(cfg.CachePath)
if err != nil {
    return nil, fmt.Errorf("load loc types: %w", err)
}
s.locTypes = locTypes

gm := gamemap.New(logger)
gm.SetLocTypes(locTypes)
if err := gm.Init(cfg.CachePath); err != nil {
    return nil, fmt.Errorf("failed to load game map: %w", err)
}
s.gamemap = gm
```

Verified safe: `LoadLocTypes(dir string)` (`pkg/objtype/loctype.go:204`) depends only on the cache path — no cross-type dependencies on params/objTypes/invTypes/etc. loaded between current lines 184-234.

The follow-on `s.locTypes = locTypes` at the original site (line 241) is replaced by deletion (the assignment moves to the new earlier site).

## 4. Data flow

```
boot:
  LoadLocTypes(cachePath) ──┐
                            ├──► gm.SetLocTypes ──► gm.Init
                            │                         │
                            │                         └─► loadLocs (each l-file)
                            │                                  │
                            │                                  └─► entity.NewLoc(..., lt.Width, lt.Length, ...)
                            ▼
                         StaticLocs() with correct W/L
                            │
                            └─► populateStaticLocsIntoZones
                                  └─► ChangeLocCollision(..., loc.Length, loc.Width, ...)
                                        └─► ChangeLoc loops W×L iterations ✓
```

Runtime sites (`world_zone.go`, `interaction.go:489`, `npc_interaction.go:691`) auto-fix because they read `loc.Width`/`loc.Length` from the now-correct entity.

## 5. Test strategy

### 5.1 Lift skip on `TestNAI99_FountainCoverage_Lumbridge`

`pkg/gamemap/nai99_fountain_dump_test.go:163-176` — drop the `t.Skip(...)` block. The test then exercises:

1. `gm.Init(cacheDir)` with real pack data — but the test does NOT call `gm.SetLocTypes` before `Init`. To make the test exercise the production path, add `gm.SetLocTypes(cfgs)` BEFORE `gm.Init` (currently the test calls Init then loads cfgs separately — reorder).
2. The collision-write replay loop (lines 189-200) reads `l.Length, l.Width` from the entity; after the fix these are `lt.Length, lt.Width` and the replay iterates the correct footprint.
3. The footprint coverage assertion (lines 216 onward) expects every tile in the rotated footprint to be flagged.

Expected post-fix output for instance 0: `unflagged=[]`; all `width × length` tiles flagged with `0x100` (FlagLoc).

### 5.2 New unit test: `TestLoadLocs_UsesLocTypeWidthLength`

New file `pkg/gamemap/load_test.go` (or extend `gamemap_test.go`). Seed:

- A synthetic `*objtype.LocTypeConfigs` with one entry: `Configs[42] = &LocType{Width: 2, Length: 3}` (the rest nil).
- An l-byte stream encoding a single placement of locID=42 at known coords. Use the GSmart format from `loadLocs` (locID delta=43 = `42+1`; coord delta=1; info byte arbitrary).
- An m-byte stream OR rely on `landsByMapSquare` being nil (the lands-nil branch at load.go:175 short-circuits the bridge check — actualLevel stays at the decoded level).

Assertions:
- `gm.StaticLocs()[0].Width == 2`
- `gm.StaticLocs()[0].Length == 3`
- `gm.StaticLocs()[0].Type() == 42`

### 5.3 New unit test: `TestLoadLocs_NilLocTypesFallback`

Same setup as 5.2 but skip the `SetLocTypes` call. Assertions:
- `gm.StaticLocs()[0].Width == 1`
- `gm.StaticLocs()[0].Length == 1`
- (Optional) capture `gm.log` output — should NOT include the "nil LocType" or "locID out of range" warnings (these only fire when `gm.locTypes != nil`).

This pins the test-fixture compat for the ~14 callers of `gm.Init(t.TempDir())`.

### 5.4 Smoke (post-merge, user-driven)

Per `smoke_test_server_handoff`: user runs the server with the goscape binary and the Java client, walks NW from Lumbridge spawn `(3221, 3218)` toward fountain footprint `(3221..3222, 3226..3227)`.

**Expected:**
- Player cannot walk onto any of the 4 footprint tiles.
- Pathfinder routes around the fountain.

**Cascade theory binding** (per `cascade_theory_smoke_binding`): if symptom unchanged, brainstorm under-diagnosed; if other Lumbridge multi-tile features still misbehave, residual to NAI-101+ (root cause of NAI-100 closes, but adjacent untracked divergences route forward).

## 6. Risks

- **R1:** pkg/gamemap import cycle on objtype. Mitigated — NAI-99 dump test already imports `objtype` from `pkg/gamemap` cleanly; no cycle.
- **R2:** Test fixture (`gm.Init(t.TempDir())`) regression. Mitigated by nil-locTypes fallback; pinned by §5.3.
- **R3:** Boot-order assumption that LoadLocTypes has no deps on already-loaded types. Verified by grep at spec-write (§3.3).
- **R4:** NAI-99 dump test format change. The test asserts on `lt.Width, lt.Length` (loaded separately) for the `(rotated W=… L=…)` portion — unaffected. The coverage test reads entity `l.Length, l.Width` — now correct, so passes.
- **R5:** Subagent might forget the `SetLocTypes` call ordering in §5.1 reorder. Mitigated by explicit ordering in plan task.

## 7. Deviations register

- **D1 (NAI-100):** TS `printFatalError` on missing/invalid locID at GameMap.ts:249-252 → goscape log-warn + 1×1 fallback. Rationale: no fatal-error infra in goscape; soft-degrade matches `loadLocs`'s existing tolerance for missing l-files (gamemap.go:144-147). Follow-up: NAI-101+ if production smoke surfaces unknown-locID warnings (none expected with full LocType registry already loaded).

## 8. Success criteria

1. `t.Skip` lifted on `TestNAI99_FountainCoverage_Lumbridge`; test passes with `unflagged=[]` for fountain instance 0.
2. New `TestLoadLocs_UsesLocTypeWidthLength` unit test passes.
3. New `TestLoadLocs_NilLocTypesFallback` unit test passes.
4. All existing tests in `pkg/gamemap/...` and `modules/world/...` remain green (no regressions in `gm.Init(t.TempDir())` callers).
5. `go test ./...` passes.
6. User smoke (§5.4) confirms fountain blocks pathing on all 4 tiles.

## 9. Out of scope

- Fixing other content-side multi-tile placement bugs surfaced by the smoke (route to NAI-101+ per cascade theory).
- Adding TS-equivalent `printFatalError` infra (D1 follow-up; only if smoke warrants).
- Updating `loadGround`/`loadNPCs`/`loadObjs` paths (none use LocType width/length).

## 10. LOC estimate

- pkg/gamemap/gamemap.go: +5 (field + setter)
- pkg/gamemap/load.go: +12 / -3 (lookup + fallback + drop TODO)
- modules/world/server.go: +3 / -3 (move LoadLocTypes block; the LoadLocTypes line itself moves intact)
- pkg/gamemap/load_test.go (new): +50 (2 sub-tests; the synthetic l-byte stream is a small GSmart-encoded buffer)
- pkg/gamemap/nai99_fountain_dump_test.go: -16 (drop t.Skip block); +1 (`gm.SetLocTypes(cfgs)` call before `gm.Init`)

Total prod: ~17 LOC; tests: ~35 LOC.
