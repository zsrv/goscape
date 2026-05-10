# NAI-151 — static map ground items (loadObjs un-stub)

## 1. Scope

Un-stub `pkg/gamemap/load.go:243` `loadObjs` so static ground items (bones, knives, pots, rings, etc. encoded in `o{X}_{Z}` cache files) are parsed at gamemap-init, routed into their owning zones at server boot, and replayed to clients via the existing `writeFullFollows` path on zone entry.

Closes the user-reported "ground items do not spawn anywhere" symptom. Pickup-respawn cycling is verified post-smoke; if broken, route to NAI-152.

## 2. Tech stack

Go 1.26+. Standard goscape packages: `pkg/gamemap`, `pkg/zone`, `modules/world`, `pkg/entity`, `pkg/objtype`. No new dependencies.

## 3. TS source — `Engine-TS/src/engine/GameMap.ts:139-159`

```ts
private loadObjs(packet: Packet, mapsquareX: number, mapsquareZ: number): void {
    while (packet.available > 0) {
        const { x, z, level } = this.unpackCoord(packet.g2());
        const absoluteX: number = mapsquareX + x;
        const absoluteZ: number = mapsquareZ + z;
        const count: number = packet.g1();
        for (let index: number = 0; index < count; index++) {
            const id: number = packet.g2();
            const count: number = packet.g1();
            if (!this.members && !this.isFreeToPlay(absoluteX, absoluteZ)) {
                continue;
            }
            const objType: ObjType = ObjType.get(id);
            if ((objType.members && this.members) || !objType.members) {
                const obj: Obj = new Obj(level, absoluteX, absoluteZ, EntityLifeCycle.RESPAWN, objType.id, count);
                this.getZone(obj.x, obj.z, obj.level).addStaticObj(obj);
            }
        }
    }
}
```

Wire layout per record: position(G2, packed `level<<12 | localX<<6 | localZ`) + tile-count(G1) + tile-count × (typeID(G2) + count(G1)). Layout-faithful to the existing `loadNPCs` parser at `pkg/gamemap/load.go:219-242`.

Two gates:
- **Tile gate**: skip tile when `!members && !isFreeToPlay(x, z)` — drops members-only tiles in F2P-only servers.
- **ObjType gate**: include when `(objType.Members && members) || !objType.Members` — drops members-only objs in F2P-only servers; non-members objs always pass; members-only objs pass only when members.

TS calls `getZone().addStaticObj(obj)` inline because `GameMap` owns the zone registry. Goscape's zone registry lives on `modules/world.Server`, so the parser collects records and lets the server route them — paralleling the existing `npcSpawns` / `staticLocs` handoff.

## 4. Existing surface (no change)

- `pkg/entity/obj.go:30-36` `entity.NewObj(level, x, z, lc, typ, count)` — constructor already handles `LifecycleRespawn` via the existing `lc` parameter; `IsActive` is the embedded `Entity` field set by `Zone.AddStaticObj`.
- `pkg/zone/zone.go:146-153` `Zone.AddStaticLoc` — pattern to mirror for `AddStaticObj`.
- `pkg/zone/zone.go:253` `Zone.AddObj` — already supports `LifecycleRespawn` (skips the dynamic-list append if Respawn), but **queues an OBJ_ADD event**, which is wrong for boot-time statics. We need the silent-append `AddStaticObj` variant so static spawns aren't double-delivered (event would fire pre-zone-tracking and would-be-delivered alongside the FullFollows replay on zone entry).
- `modules/world/player_zone.go:42-58` `writeFullFollows` — already iterates `z.Objs`, gates on `LastLifecycleTick == currentTick`, `ReceiverID` filter, and `CheckLifecycle(currentTick)`. For `LifecycleRespawn` static objs with `ReceiverID = PublicReceiver`, this already produces the correct OBJ_ADD on zone entry. **No change.**
- `pkg/gamemap/multimap.go:21-23` `gm.IsFreeToPlay(x, z)` — F2P tile lookup, already populated.
- `pkg/objtype/objtype.go:107` `ObjType.Members bool` — already loaded.
- `modules/world/config.go:36` `cfg.NodeMembers bool` — already loaded.
- `pkg/gamemap/gamemap.go:31` `gm.npcSpawns []NpcSpawn` — pattern to mirror for `objSpawns`.
- `pkg/gamemap/gamemap.go:50-58` `SetLocTypes` — pattern to mirror for `SetMembers` and `SetObjTypes`.
- `modules/world/server.go:303-318` NPC-spawn loop and `populateStaticLocsIntoZones` — pattern to mirror for `populateStaticObjsIntoZones`.
- `pkg/io/packet.Packet` — `G1`, `G2`, `Len()` already used by the sibling `loadNPCs` parser.

## 5. New surface

### 5.1. `pkg/gamemap` extensions

#### 5.1.1. New struct + field + accessor (`pkg/gamemap/gamemap.go`)

```go
// ObjSpawn records a ground-obj spawn position from a mapsquare's o-file.
// Mirrors NpcSpawn (pkg/gamemap/gamemap.go:17). NAI-151.
type ObjSpawn struct {
    TypeID, Count int
    X, Z, Level   int
}
```

Add fields to `GameMap`:

```go
type GameMap struct {
    // ... existing fields ...
    objSpawns []ObjSpawn
    members   bool                    // NodeMembers flag — set via SetMembers before Init. NAI-151.
    objTypes  *objtype.ObjTypeConfigs // optional; consumed by loadObjs gate. NAI-151.
}
```

Accessor:

```go
// ObjSpawns returns the list of static-obj spawn records collected
// during Init. Returned slice is internal — do not mutate. NAI-151.
func (gm *GameMap) ObjSpawns() []ObjSpawn { return gm.objSpawns }
```

#### 5.1.2. New setters (`pkg/gamemap/gamemap.go`)

Both must be called BEFORE `Init`. Default-zero values (`members=false`, `objTypes=nil`) preserve `t.TempDir()` test fixtures whose o-files are absent — `loadObjs` no-ops on empty input, and the `objTypes==nil` guard inside `loadObjs` skips the ObjType gate (test fixtures with hand-built o-files but no objTypes are invalid by construction; spec the parser to skip those tiles silently).

```go
// SetMembers registers the world's NodeMembers flag for use by loadObjs.
// Must be called BEFORE Init; calling later has no effect on already-
// loaded static objs. Default false. Mirrors TS GameMap.members.
// NAI-151.
func (gm *GameMap) SetMembers(m bool) {
    gm.members = m
}

// SetObjTypes registers the ObjType configs used by loadObjs to gate
// per-obj members visibility. Must be called BEFORE Init. nil-OK:
// when unset, loadObjs records no objs (preserves test fixtures with
// empty caches). Mirrors TS ObjType.get() inside GameMap.loadObjs.
// NAI-151.
func (gm *GameMap) SetObjTypes(cfgs *objtype.ObjTypeConfigs) {
    gm.objTypes = cfgs
}
```

#### 5.1.3. Replace stub body of `loadObjs` (`pkg/gamemap/load.go:243-249`)

```go
// loadObjs records ground-object positions from the o{X}_{Z} file.
// Mirrors LostCityRS/Engine-TS/src/engine/GameMap.ts:139-159.
//
// Wire layout per record: position(G2, packed level<<12|localX<<6|localZ)
// + tile-count(G1) + tile-count × (typeID(G2) + count(G1)).
//
// Two gates mirror TS:
//   - tile gate: skip when !members && !isFreeToPlay(absX, absZ)
//   - objtype gate: include when (objType.Members && members) || !objType.Members
//
// nil objTypes (test fixtures without registered configs) → skip all
// records silently. NAI-151.
func (gm *GameMap) loadObjs(data []byte, mapSquareX, mapSquareZ int) {
    p := packet.NewPacket(data)
    for p.Len() >= 3 {
        packed := int(p.G2())
        tileCount := int(p.G1())
        level := (packed >> 12) & 0x3
        localX := (packed >> 6) & 0x3F
        localZ := packed & 0x3F
        absX := mapSquareX*mapSquareSize + localX
        absZ := mapSquareZ*mapSquareSize + localZ
        for i := 0; i < tileCount && p.Len() >= 3; i++ {
            typeID := int(p.G2())
            count := int(p.G1())
            // Tile gate: members-only world OR F2P tile.
            if !gm.members && !gm.IsFreeToPlay(absX, absZ) {
                continue
            }
            if gm.objTypes == nil {
                continue
            }
            if typeID < 0 || typeID >= len(gm.objTypes.Configs) {
                continue
            }
            ot := gm.objTypes.Configs[typeID]
            if ot == nil {
                continue
            }
            // ObjType gate: ((Members && members) || !Members) — drops
            // members-only objs in F2P-only servers, otherwise include.
            if !((ot.Members && gm.members) || !ot.Members) {
                continue
            }
            gm.objSpawns = append(gm.objSpawns, ObjSpawn{
                TypeID: typeID, Count: count, X: absX, Z: absZ, Level: level,
            })
        }
    }
}
```

Plan-author preflight (mandatory): re-read `pkg/gamemap/load.go` `loadNPCs` parser to confirm the bit-packing constants (`>>12 & 0x3` for level, `>>6 & 0x3F` for x, `& 0x3F` for z) and the `Len() >= 3` outer-loop guard match the parser's actual idiom. Confirm `mapSquareSize` constant value (existing reference in `loadNPCs`).

Plan-author preflight: confirm `objtype.ObjTypeConfigs.Configs` is the slice-shaped accessor (vs map-shaped); confirm field names `Members` (bool) on the loaded ObjType. Re-read `pkg/objtype/objtype.go:107` and the `LoadObjTypes` return type at `pkg/objtype/objtype.go:19`.

### 5.2. `pkg/zone` extension — `Zone.AddStaticObj`

Append to `pkg/zone/zone.go` adjacent to `AddStaticLoc`:

```go
// AddStaticObj appends a static (LifecycleRespawn) obj to z.Objs WITHOUT
// queuing a zone event. Statics are delivered to clients via the
// FullFollows replay on zone entry (modules/world/player_zone.go:42-58),
// not via Enclosed/Follows events. Called once per obj during world init.
// Mirrors LostCityRS/Engine-TS/src/engine/zone/Zone.ts:211-215. NAI-151.
func (z *Zone) AddStaticObj(obj *entity.Obj) {
    z.Objs = append(z.Objs, obj)
    obj.IsActive = true
}
```

Plan-author preflight: confirm `obj.IsActive` is a settable bool on the embedded `Entity` (mirrors loc) — re-grep `pkg/entity/entity.go` and `pkg/entity/obj.go`.

### 5.3. `modules/world` server-boot reorder + new wiring

#### 5.3.1. Move `LoadParams` + `LoadObjTypes` ABOVE `gm.Init` (server.go ~lines 180-216)

Current order:

```
locTypes   ← LoadLocTypes
gm.New + SetLocTypes + Init           ← parses now
params     ← LoadParams                  ← needed by ObjTypes
objTypes   ← LoadObjTypes
```

New order:

```
locTypes   ← LoadLocTypes
params     ← LoadParams                  ← moved up
objTypes   ← LoadObjTypes                ← moved up
gm.New + SetLocTypes + SetMembers(cfg.NodeMembers) + SetObjTypes(objTypes) + Init
```

`LoadParams` and `LoadObjTypes` only read cache files; they do not depend on gamemap state. Reordering is safe. Plan-author preflight: re-read `modules/world/server.go:180-220` at HEAD to confirm no intermediate state depends on the existing order.

#### 5.3.2. New helper `populateStaticObjsIntoZones` (`modules/world/server.go`)

Placed adjacent to `populateStaticLocsIntoZones`:

```go
// populateStaticObjsIntoZones constructs an *entity.Obj per parsed
// ObjSpawn and routes it to its owning Zone via Zone.AddStaticObj.
// Called once at server startup, adjacent to populateStaticLocsIntoZones.
// Mirrors TS GameMap.loadObjs's inline getZone().addStaticObj() call;
// goscape splits the parse (gamemap.loadObjs) from the zone-routing
// (here) because the zone registry lives on Server, not GameMap.
// NAI-151.
func (s *Server) populateStaticObjsIntoZones() {
    for _, spawn := range s.gamemap.ObjSpawns() {
        obj := entitypkg.NewObj(spawn.Level, spawn.X, spawn.Z,
            entitypkg.LifecycleRespawn, spawn.TypeID, spawn.Count)
        z := s.zoneAt(spawn.X, spawn.Z, spawn.Level)
        z.AddStaticObj(obj)
    }
    s.log.Info("static objs loaded", "count", len(s.gamemap.ObjSpawns()))
}
```

Call site: append after `s.populateStaticLocsIntoZones()` in `Server.New`.

Plan-author preflight: confirm the existing zone-resolver helper name (`zoneAt`? `getZone`? `zoneFor`?) at `modules/world/world_zone.go` and the import alias for `entity` (`entitypkg` per `world_zone.go:128`). Confirm `s.log.Info` matches the existing logging shape at `populateStaticLocsIntoZones`.

## 6. Test strategy

### 6.1. Parser tests (`pkg/gamemap/load_test.go` extension)

Hand-crafted o-file bytes using the same `packet.Packet.P*` helpers as existing `loadNPCs` tests. Test cases:

1. `TestLoadObjs_Empty` — zero-byte input → empty `ObjSpawns()`.
2. `TestLoadObjs_SingleTileSingleObj` — 1 tile, 1 obj, `Members=false`, F2P tile, `gm.members=false` → spawn recorded with correct `(X, Z, Level, TypeID, Count)`.
3. `TestLoadObjs_SingleTileMultiObj` — 1 tile, 3 objs at same coord → 3 spawns recorded in order.
4. `TestLoadObjs_MultiTile` — 2 tiles, 1 obj each → 2 spawns recorded.
5. `TestLoadObjs_LevelEncoding` — packed-coord bits: assert level=0,1,2,3 each decode correctly.
6. `TestLoadObjs_TruncatedTrailing` — record header present but obj-loop body cut mid-record → no spawn appended (mid-record bytes consumed safely by `Len() >= 3` guard).
7. `TestLoadObjs_TileGate_MembersWorld_F2PTile` — `members=true`, F2P tile → included.
8. `TestLoadObjs_TileGate_F2POnlyServer_MembersTile` — `members=false`, **non-F2P tile** → record dropped, spawns empty.
9. `TestLoadObjs_TileGate_F2POnlyServer_F2PTile` — `members=false`, F2P tile → included.
10. `TestLoadObjs_ObjTypeGate_F2POnlyServer_MembersObj` — `ObjType.Members=true, gm.members=false`, F2P tile → record dropped (ObjType-gate fails).
11. `TestLoadObjs_ObjTypeGate_MembersWorld_MembersObj` — `ObjType.Members=true, gm.members=true`, F2P tile → included.
12. `TestLoadObjs_ObjTypeGate_NonMembersObj_F2POnlyServer` — `ObjType.Members=false, gm.members=false` → included.
13. `TestLoadObjs_ObjTypesNil` — `SetObjTypes` not called → no spawns recorded (nil-guard preserves test fixtures).
14. `TestLoadObjs_TypeIDOutOfRange` — typeID exceeds `len(Configs)` → skipped.
15. `TestLoadObjs_TypeIDNilEntry` — `Configs[id] == nil` → skipped.

Test-helper builder for hand-crafted gamemaps: extend the existing `loadNPCs` test setup (`pkg/gamemap/load_test.go` if present, else `pkg/gamemap/gamemap_test.go`). Use `packet.Packet`'s P1/P2 to build records; pass the bytes through a wrapper that calls `gm.loadObjs(data, mapSquareX, mapSquareZ)` directly (not via `Init`).

Plan-author preflight: re-grep `pkg/gamemap/*_test.go` for the existing parser-test idiom (likely `gm.loadNPCs(data, ...)` direct call); replicate for symmetry.

### 6.2. `Zone.AddStaticObj` tests (`pkg/zone/zone_test.go` extension)

Two tests, paralleling the existing `TestAddStaticLoc...` pattern (or `TestAddObjPublicIsEnclosed` shape with the inverse assertion):

1. `TestAddStaticObjAppendsToObjsAndActivates` — construct empty zone, `obj := entity.NewObj(0, 5, 5, entity.LifecycleRespawn, 1234, 1)`, call `z.AddStaticObj(obj)`. Assert: `len(z.Objs) == 1`, `z.Objs[0] == obj`, `obj.IsActive == true`.
2. `TestAddStaticObjQueuesNoEvent` — same setup. Assert: `len(z.Events()) == 0` (or whatever accessor returns the queued event slice; mirror `TestAddObjPublicIsEnclosed` shape's event-inspection accessor — see `pkg/zone/zone_test.go:193-207` for current `e := z.Events()[0]` idiom). The zero-event assertion is the load-bearing pin distinguishing static from dynamic add.

Plan-author preflight: re-grep `pkg/zone/zone_test.go` for `AddStaticLoc` tests (if present) — mirror the assertion shape exactly. Confirm `z.Events()` accessor name.

### 6.3. Server-boot wiring tests (`modules/world/world_zone_test.go` extension)

Tests use the existing `newTestServer(t)` fixture extended with a hand-built `gamemap` carrying ≥1 ObjSpawn record.

1. `TestPopulateStaticObjsIntoZones_RoutesByCoord` — register ObjSpawns at two distinct zones; call `s.populateStaticObjsIntoZones()`; assert each zone's `Objs` contains exactly the spawn(s) for its coords. Mirrors `TestServerAddObjRoutesByCoord` (existing, `world_zone_test.go:30`).
2. `TestPopulateStaticObjsIntoZones_LifecycleRespawn` — single spawn; assert routed obj has `Lifecycle == entity.LifecycleRespawn` and `IsActive == true`.
3. `TestPopulateStaticObjsIntoZones_LogsCount` — pin the boot-log message contains `count=N`. Mirrors the existing `populateStaticLocsIntoZones` log idiom.

### 6.4. End-to-end zone-entry replay test (`modules/world/player_zone_test.go` extension)

Single test, leveraging the existing `writeFullFollows` test infrastructure:

`TestStaticObjReplaysOnZoneEntry`:
- `s := newTestServer(t)`.
- Construct `obj := entity.NewObj(0, p.X(), p.Z(), entity.LifecycleRespawn, 1234, 5)`.
- `s.zoneAt(p.X(), p.Z(), 0).AddStaticObj(obj)`.
- Player joins zone (existing helper). Capture player's outgoing packet stream.
- Assert stream contains `OpUpdateZoneFullFollows` + `OpUpdateZonePartialFollows` + 1 OBJ_ADD with the encoded `(coord, type=1234, count=5)` payload.

Plan-author preflight: re-grep `modules/world/player_zone_test.go` for the existing `TestWriteFullFollows_ObjAdd_RespawnEmits` test (referenced at `:419` per the `writeFullFollows` review) — it ALREADY tests the Respawn-lifecycle replay path. The new e2e test is a wiring pin, not a re-test of `writeFullFollows`; assert it passes through the boot helper `populateStaticObjsIntoZones` rather than direct `AddStaticObj`.

### 6.5. Cache-data smoke (post-merge, user-driven)

Per `smoke_test_server_handoff.md`, ask user to launch the server and confirm:
- Boot log: `static objs loaded count=N` with N > 0 against the production cache.
- Java client: walk to a known static-obj location (e.g., bones at `(3221, 3219)` Lumbridge area) and confirm visible.

## 7. Cadence

Per `runescript_cadence.md` mid-band. Total LOC estimate:

- Production: ~70 LOC (parser body 35 + `AddStaticObj` 5 + setters 10 + `populateStaticObjsIntoZones` 12 + reorder 8).
- Tests: ~250 LOC (15 parser cases + 2 zone cases + 3 wiring cases + 1 e2e case + helper extension).
- **Total: ~320 LOC.**

Workflow per `execution_mode_default.md` and `runescript_cadence.md`: separate spec + plan, subagent-driven impl with TDD red→green per task tier, single end-of-impl reviewer subagent on Sonnet (per `superpowers_code_reviewer_model.md`). `/clear` between plan and impl per `superpowers_clear_between_spec_and_impl.md`.

Plan tasks (preview):
1. T1 — `pkg/gamemap`: add `ObjSpawn` struct, `objSpawns/members/objTypes` fields, `SetMembers/SetObjTypes/ObjSpawns` methods. Compile-check only.
2. T2 — write 15 parser tests RED (new test file or extend existing).
3. T3 — implement `loadObjs` body → tests GREEN.
4. T4 — `pkg/zone`: write 2 `AddStaticObj` tests RED → implement → GREEN.
5. T5 — `modules/world`: reorder LoadParams/LoadObjTypes above gm.Init; wire SetMembers/SetObjTypes; add `populateStaticObjsIntoZones` + call site. Write 3 wiring tests + 1 e2e test RED → impl → GREEN.
6. T6 — `go test ./...` + `go vet ./...` repo-wide.
7. T7 — final reviewer pass (Sonnet).
8. T8 — close commit + `Closes memory:` trailer per `close_commit_memory_trailer.md`.
9. T9 — user-driven smoke per §6.5; if static objs visible, close NAI-151. If pickup-respawn breaks, open NAI-152.

## 8. TS-fidelity deviations

- **NAI-151-D1: gating-call-site** — TS gates inline at parse time using `World.members` reachable from `GameMap`; goscape gates inline at parse time using `gm.members` set explicitly via `SetMembers` (mirrors `SetLocTypes` pattern). Net record set is identical when gates run on equal inputs. **Why:** goscape's `gamemap.New` doesn't take a server-config view; runtime config flag enters via setter. **Risk:** server boot must call `SetMembers` BEFORE `Init` — same invariant as `SetLocTypes` and `SetObjTypes`. Pinned by §6.1 tests #7-12 (gate-matrix coverage).
- **NAI-151-D2: parse vs route split** — TS calls `getZone().addStaticObj(obj)` inline inside `loadObjs`; goscape collects records into `gm.objSpawns` and routes them at server boot via `populateStaticObjsIntoZones`. **Why:** zone registry lives on `modules/world.Server`, not `pkg/gamemap`. **Risk:** the resulting `z.Objs` content is identical, but the call-graph differs from TS by one indirection layer. Mirrors the existing `npcSpawns` precedent.

If the implementer surfaces an additional divergence during T2-T5, open as `NAI-151-D-<TAG>` and add to spec §8 before the close commit.

## 9. Risk register

- **R1 (low):** o-file format drift between cache versions. Mitigated by `loadNPCs` precedent — both files use the same `unpackCoord` shape per TS GameMap.ts. If parser fails on production cache, the boot log will show `count=0` rather than crashing (parser is `Len() >= 3`-guarded).
- **R2 (low):** `cfg.NodeMembers` value at boot. Default is config-file driven; absence defaults to `false`. Smoke validates against the production config.
- **R3 (med):** Pickup-respawn cycling. Existing `Zone.RemoveObj` does NOT remove `LifecycleRespawn` objs from `z.Objs` (zone.go:293), so the entry survives — but no "respawn-after-N-ticks" timer wiring is verified. **Mitigation:** explicitly out-of-scope for this sub-spec; verified post-smoke. If first pickup leaves obj absent permanently (vs reappearing after `ObjType.respawnrate`), open NAI-152.
- **R4 (low):** Test fixtures with `t.TempDir()` empty caches break if `SetObjTypes` becomes mandatory. Mitigated by nil-guard in `loadObjs` (skip records when `objTypes == nil`).
- **R5 (low):** `populateStaticObjsIntoZones` must run AFTER zone registry is initialized. Plan-author preflight: confirm `s.zoneAt` (or whatever the helper is called) is available before this point — mirror placement of `populateStaticLocsIntoZones`.

## 10. Out of scope / non-goals

- **Pickup-respawn cycling.** Verified post-smoke; routed to NAI-152 if broken.
- **Per-zone obj cap (TS `OBJS = 129`).** Existing `TODO(beyond-4b)` at `pkg/zone/zone.go:251`.
- **Tradeability gating on reveal.** Existing `TODO(beyond-4b)` at `pkg/zone/zone.go:323`.
- **Retiring the `loadObjs` "Sub-spec 2 discards these" doc-comment as a separate cleanup.** Replaced inline as part of T3.
- Any other entity types parsed from cache — only `o{X}_{Z}` is in scope.

## 11. Cascade attribution

Closes the user-reported "ground items do not spawn anywhere" symptom (2026-05-10 conversation). Symptom-cause chain:

```
o{X}_{Z} cache files present
  → loadObjs stub (gamemap/load.go:243) discards
  → no entries in z.Objs for any zone
  → writeFullFollows iterates z.Objs (player_zone.go:42), zero hits
  → client renders no static ground items anywhere
```

Fix at the parser layer is the load-bearing edit; the rest (Zone.AddStaticObj, server wiring) is downstream plumbing for that fix.
