# NAI-9 — `huntNpcs` + `huntObjs` + `huntLocs` Variant Fill

Fill the three remaining `huntXxx` variant stubs (left by NAI-7) with
zone-iteration + type/category/distance filter chains. Also close the
NAI-7-era PAUSEHUNT gate stub (`observers := 1`) by adding a real
per-NPC observer counter maintained by `pkg/rsbuf`'s NpcInfo encoder.

Part of the NPC AI tick decomposition roadmap. Blocker: NAI-7 (hunt
skeleton + variant stubs). Roadmap fidelity risk: Medium (three
structurally-similar variants) + a small infrastructure item
(observer counter) that NAI-7 deferred.

**Tech Stack:** Go 1.26+, existing `pkg/grid.NearbyNpcs`, existing
`pkg/zone.ZoneMap`, existing `pkg/rsbuf.NpcSource` interface, existing
`pkg/entity.Obj` / `pkg/entity.Loc`.

## Goal

After NAI-9 ships:

1. `huntNpcs(s, hunt)`, `huntObjs(s, hunt)`, and `huntLocs(s, hunt)`
   return `[]entity` of in-range candidates passing type + category
   filters, for `huntAll` to random-pick from.
2. The NAI-7 PAUSEHUNT gate (`observers <= 0` branch in
   `processNpcHunt`) runs against a real observer counter maintained
   by the NpcInfo subscription path in `pkg/rsbuf`, not the `:= 1`
   placeholder.
3. `*entity.Obj` satisfies the `world.entity` interface so Objs can be
   assigned to `n.huntTarget`.
4. `nai_followups.md` gets the NAI-8 CheckVis-gap entry that the NAI-8
   audit surfaced, plus an amendment to the checkNotCombat/Self
   entries flagging the outer `target !== player && !isMulti(...)`
   guard at TS `Npc.ts:942`.

## Scope — what's IN

1. **Three variant bodies** in a new file `modules/world/npc_hunt_entities.go`:
   - `huntNpcs`: iterates `s.grid.NearbyNpcs(x, z, level, zoneRadius)`,
     applies `CheckNpc` (type), `CheckCategory`, Chebyshev distance.
   - `huntObjs`: iterates `s.zoneMap.NearbyZones(level, x, z, zoneRadius)`,
     for each zone walks `zn.Objs`, applies `CheckObj`, `CheckCategory`,
     distance.
   - `huntLocs`: same shape as `huntObjs` over `zn.Locs` with
     `CheckLoc`, `CheckCategory`, distance.

2. **Zone-iteration helper** `pkg/zone.(*ZoneMap).NearbyZones(level, x, z, zoneRadius int) []*Zone`
   — returns all materialised zones in a zoneRadius Chebyshev
   neighbourhood of the zone containing (x, z) at the given level.
   Unmaterialised zones skipped (read-path, avoids lazy creation).

3. **rsbuf-owned observer counter**, keyed by nid, exposed as
   `rsbuf.GetNpcObservers(nid int) int` — mirrors the
   `@2004scape/rsbuf` public API at
   `Engine-TS/node_modules/@2004scape/rsbuf/dist/rsbuf.d.ts:13`
   (`export function getNpcObservers(nid: number): number`). Storage
   is a new file `pkg/rsbuf/npc_observers.go` holding a package-level
   `map[int]int` + accessor functions. Maintained at three sites in
   `pkg/rsbuf/npcinfo.go` (one subscription-add, two
   subscription-remove).

4. **rsbuf-owned logout cleanup**, exposed as
   `rsbuf.RemovePlayer(pid int, subscribedNpcs map[int]struct{})` —
   mirrors the `@2004scape/rsbuf` public API at
   `rsbuf.d.ts:6` (`export function removePlayer(pid: number): void`).
   Iterates the player's subscription set and decrements each NPC's
   observer count. Called from `modules/world/tick.go`'s
   `processLogouts` with `p.buildArea.Npcs` as the subscription set.
   The `subscribedNpcs` parameter is goscape-specific (TS's
   removePlayer accesses `PLAYERS[pid].build.npcs` internally;
   goscape's rsbuf doesn't own the player-build-area state, so the
   caller supplies it — see D5).

5. **`*entity.Obj` entity-interface satisfaction** — add `Slot()` and
   `Coords()` methods on `*Obj`, mirroring `*entity.Loc`'s existing
   methods. Required so `n.huntTarget = o` type-checks.

6. **Test assertion flip** in
   `modules/world/npc_event_queue_test.go`:
   - Rename `TestProcessNpcHuntPauseHuntRunsWithObserverStub` →
     `TestProcessNpcHuntPauseHuntBailsWithNoObservers` and invert:
     expect `huntClock == 0` (gate short-circuits).
   - Add companion `TestProcessNpcHuntPauseHuntRunsWithObservers`
     that seeds `rsbuf.SetObserverForTest(n.nid, 1)` via the
     test-only export, expects `huntClock == 1`.

7. **`nai_followups.md` updates** — fold in the NAI-8 fidelity-audit
   findings surfaced during this brainstorm:
   - New bullet under "From NAI-8": "Deferred: CheckVis (LoS/LoW)
     gate on all four hunt variants (TS:ScriptIterators.ts:88-94,
     :113-118, :137-142, :160-165)". Symmetric with NAI-9's own
     CheckVis deferral below.
   - Amend the NAI-8 checkNotCombat and checkNotCombatSelf entries
     to call out the outer `this.target !== player &&
     !World.gameMap.isMulti(...)` guard at TS `Npc.ts:942` that
     conditionally wraps both filters. Future filter-audit
     implementer must port the guard alongside the filters.
   - Mark the NAI-7 "observer counting" blocker resolved, citing
     this spec.

## Scope — explicit non-goals (deferred to future audit)

1. **`CheckVis` (LoS / LoW) gate** — TS `ScriptIterators.ts:88-94`,
   `:113-118`, `:137-142`, `:160-165` applies `HuntVis.LINEOFSIGHT`
   and `HuntVis.LINEOFWALK` gates inside each HuntIterator branch.
   NAI-9 does NOT port these gates for any of the four variants
   (huntPlayers silent gap from NAI-8 remains; huntNpcs/Objs/Locs
   follow the same pattern). Each variant carries an inline
   `// TODO: CheckVis gate` breadcrumb pointing at the TS line.
   Added to `nai_followups.md` for the same audit pass that picks up
   NAI-8's six deferred filters. The `pkg/pathfinder/routefinder`
   package has `LineOfSight` / `LineOfWalk` routines available for
   when the audit pass runs.

2. **Multi-tile NPC / Obj iteration** — deferred per roadmap
   non-goal #4. Not a NAI-9 concern.

3. **Script dispatch from hunt** — TS variants do zero script
   dispatch; NAI-9 matches. No `runNpcScript` calls.

4. **`HuntType.CheckVis` config-value exposure** — already loaded by
   NAI-1; NAI-9 reads but doesn't act on it (see non-goal #1).

## TS reference

- `Engine-TS/src/engine/entity/Npc.ts:975-985` — `huntNpcs`,
  `huntObjs`, `huntLocs` bodies.
- `Engine-TS/src/engine/script/ScriptIterators.ts:36-172` — the
  `HuntIterator` class whose four per-mode branches are the source of
  the filter logic each variant inlines.
- `Engine-TS/src/engine/entity/Npc.ts:162` — `processHunt` call site
  invoking `rsbuf.getNpcObservers(this.nid)`; the contract NAI-9's
  observer counter implements.
- `Engine-TS/src/engine/CoordGrid.ts:75-80` — `distanceToSW` is
  Chebyshev: `Math.max(|dx|, |dz|)`. Matched by the inline `|dx| >
  range || |dz| > range` predicate in each variant.
- `Engine-TS/src/engine/World.ts:581` — second `getNpcObservers` call
  site (world-wide hunt pass); same contract.

## Architecture

### Files modified / created

| Path | Change |
|---|---|
| `modules/world/npc_hunt_entities.go` | **New** — three variant bodies (~140 LOC prod) |
| `modules/world/npc_hunt.go` | Modify — delete the three one-line stubs; replace `observers := 1` with `observers := rsbuf.GetNpcObservers(n.nid)` |
| `modules/world/tick.go` | Modify — in `processLogouts`, call `rsbuf.RemovePlayer(p.slot, p.buildArea.Npcs)` before build-area cleanup |
| `modules/world/npc_event_queue_test.go` | Modify — flip `TestProcessNpcHuntPauseHuntRunsWithObserverStub` + add companion |
| `pkg/rsbuf/npc_observers.go` | **New** — package-level `map[int]int` + `GetNpcObservers`, `RemovePlayer`, unexported add/remove helpers. Matches `@2004scape/rsbuf` public API |
| `pkg/rsbuf/npc_observers_test.go` | **New** — direct tests for inc/dec/floor/clear semantics |
| `pkg/rsbuf/npcinfo.go` | Modify — 3 new call-sites (inc on add, dec on two removes) |
| `pkg/zone/map.go` | Modify — add `NearbyZones(level, x, z, zoneRadius int) []*Zone` helper |
| `pkg/zone/map_test.go` | Modify — tests for `NearbyZones` |
| `pkg/entity/obj.go` | Modify — add `Slot() int` and `Coords() (x, z, level int)` |
| `pkg/entity/obj_test.go` | Modify — entity-interface satisfaction test |
| `.../memory/nai_followups.md` | Modify — 3 edits (CheckVis entry, checkNotCombat amendment, observer-blocker resolution) |

### `huntNpcs` body

```go
// huntNpcs iterates NPCs in the grid within huntRange and returns
// those passing type + category + distance filters. Matches TS
// Npc.huntNpcs at Engine-TS/.../Npc.ts:975-977, which delegates to
// HuntIterator's NPC branch at ScriptIterators.ts:98-120.
//
// Filter coverage (NAI-9):
//   - Range: Chebyshev distance <= n.huntRange
//   - Level match: NearbyNpcs is level-filtered internally
//   - CheckNpc: type-ID filter (-1 == allow all)
//   - CheckCategory: NpcType.Category filter (-1 == allow all)
//
// DEFERRED to audit pass:
//   - CheckVis (TS ScriptIterators.ts:113-118) — LoS/LoW gate.
//
// Does NOT exclude self (TS doesn't either — NPC can appear in its
// own zone's NPC list and pass all filters). Preserves TS quirk.
func (n *Npc) huntNpcs(s *Server, hunt *objtype.HuntType) []entity {
    if s.grid == nil || s.npcTypes == nil {
        return nil
    }
    zoneRadius := 1 + n.huntRange/8
    nids := s.grid.NearbyNpcs(n.x, n.z, n.level, zoneRadius)
    var hunted []entity
    for _, nid := range nids {
        if nid < 0 || nid >= len(s.npcs) {
            continue
        }
        other := s.npcs[nid]
        if other == nil {
            continue
        }
        if hunt.CheckNpc != -1 && other.typeId != hunt.CheckNpc {
            continue
        }
        if hunt.CheckCategory != -1 {
            if other.typeId < 0 || other.typeId >= len(s.npcTypes.Configs) {
                continue
            }
            ot := s.npcTypes.Configs[other.typeId]
            if ot == nil || ot.Category != hunt.CheckCategory {
                continue
            }
        }
        dx := other.x - n.x
        if dx < 0 {
            dx = -dx
        }
        dz := other.z - n.z
        if dz < 0 {
            dz = -dz
        }
        if dx > n.huntRange || dz > n.huntRange {
            continue
        }
        // TODO: CheckVis gate — TS ScriptIterators.ts:113-118.
        // Deferred; see nai_followups.md.
        hunted = append(hunted, other)
    }
    return hunted
}
```

### `huntObjs` body

```go
// huntObjs iterates dynamic objs in zone-radius and returns those
// passing the filter chain. Matches TS Npc.huntObjs at
// Engine-TS/.../Npc.ts:979-981 (HuntIterator OBJ branch at
// ScriptIterators.ts:121-144).
//
// goscape Zone.Objs contains only LifecycleDespawn (dynamic) objs
// by construction (pkg/zone/zone.go:221). Matches TS comment
// "scripting only cares about dynamic objs??" at
// ScriptIterators.ts:122.
func (n *Npc) huntObjs(s *Server, hunt *objtype.HuntType) []entity {
    if s.zoneMap == nil || s.objTypes == nil {
        return nil
    }
    zoneRadius := 1 + n.huntRange/8
    var hunted []entity
    for _, zn := range s.zoneMap.NearbyZones(n.level, n.x, n.z, zoneRadius) {
        for _, o := range zn.Objs {
            if o == nil {
                continue
            }
            if hunt.CheckObj != -1 && o.Type != hunt.CheckObj {
                continue
            }
            if hunt.CheckCategory != -1 {
                if o.Type < 0 || o.Type >= len(s.objTypes.Configs) {
                    continue
                }
                ot := s.objTypes.Configs[o.Type]
                if ot == nil || ot.Category != hunt.CheckCategory {
                    continue
                }
            }
            dx := o.X - n.x
            if dx < 0 {
                dx = -dx
            }
            dz := o.Z - n.z
            if dz < 0 {
                dz = -dz
            }
            if dx > n.huntRange || dz > n.huntRange {
                continue
            }
            // TODO: CheckVis gate — TS ScriptIterators.ts:137-142.
            hunted = append(hunted, o)
        }
    }
    return hunted
}
```

### `huntLocs` body

```go
// huntLocs iterates locs in zone-radius and returns those passing
// the filter chain. Matches TS Npc.huntLocs at
// Engine-TS/.../Npc.ts:983-985 (HuntIterator SCENERY branch at
// ScriptIterators.ts:145-167).
//
// Zone.Locs contains both static (Respawn) and dynamic (Despawn)
// locs — matches TS getAllLocsSafe(true).
//
// Multi-tile locs use SW corner for distance (l.X/l.Z ARE the SW
// corner by goscape entity.Entity convention, matching TS which
// passes {x: loc.x, z: loc.z} to distanceToSW).
func (n *Npc) huntLocs(s *Server, hunt *objtype.HuntType) []entity {
    if s.zoneMap == nil || s.locTypes == nil {
        return nil
    }
    zoneRadius := 1 + n.huntRange/8
    var hunted []entity
    for _, zn := range s.zoneMap.NearbyZones(n.level, n.x, n.z, zoneRadius) {
        for _, l := range zn.Locs {
            if l == nil {
                continue
            }
            if hunt.CheckLoc != -1 && l.Type() != hunt.CheckLoc {
                continue
            }
            if hunt.CheckCategory != -1 {
                if l.Type() < 0 || l.Type() >= len(s.locTypes.Configs) {
                    continue
                }
                lt := s.locTypes.Configs[l.Type()]
                if lt == nil || lt.Category != hunt.CheckCategory {
                    continue
                }
            }
            dx := l.X - n.x
            if dx < 0 {
                dx = -dx
            }
            dz := l.Z - n.z
            if dz < 0 {
                dz = -dz
            }
            if dx > n.huntRange || dz > n.huntRange {
                continue
            }
            // TODO: CheckVis gate — TS ScriptIterators.ts:160-165.
            hunted = append(hunted, l)
        }
    }
    return hunted
}
```

### `ZoneMap.NearbyZones`

```go
// NearbyZones returns all materialised zones whose zone-coord is
// within zoneRadius Chebyshev distance of the zone containing
// (x, z) at the given level. Unmaterialised zones are skipped —
// callers need not guard against nil *Zone entries.
//
// Used by hunt-variant iteration in modules/world/npc_hunt_entities.go.
func (m *ZoneMap) NearbyZones(level, x, z, zoneRadius int) []*Zone {
    zoneX := x >> 3
    zoneZ := z >> 3
    out := make([]*Zone, 0, (2*zoneRadius+1)*(2*zoneRadius+1))
    for dx := -zoneRadius; dx <= zoneRadius; dx++ {
        for dz := -zoneRadius; dz <= zoneRadius; dz++ {
            zx := zoneX + dx
            zz := zoneZ + dz
            if zx < 0 || zz < 0 {
                continue
            }
            idx := coordgrid.ZoneIndex(zx<<3, zz<<3, level)
            if z, ok := m.zones[idx]; ok {
                out = append(out, z)
            }
        }
    }
    return out
}
```

### Observer counter — rsbuf-owned storage

`pkg/rsbuf/npc_observers.go` (new file):

```go
package rsbuf

// npcObservers counts per-NPC subscribers across all player
// BuildAreas. Mirrors the @2004scape/rsbuf WASM internal state
// that backs the getNpcObservers(nid) public API.
//
// Maintained at three sites in npcinfo.go:
//   - incNpcObserver on subscription-add
//   - decNpcObserver on subscription-remove (inactive path)
//   - decNpcObserver on subscription-remove (out-of-range path)
// And by RemovePlayer for bulk-decrement on player logout.
//
// Read by consumers via GetNpcObservers (public API; currently
// called from modules/world/npc_hunt.go's PAUSEHUNT gate).
var npcObservers = map[int]int{}

// GetNpcObservers returns the number of players currently subscribed
// to this NPC via NpcInfo. Returns 0 for any nid never observed or
// whose count floored at zero. Public API; mirrors
// @2004scape/rsbuf's getNpcObservers(nid) at rsbuf.d.ts:13.
func GetNpcObservers(nid int) int { return npcObservers[nid] }

// RemovePlayer performs bulk-decrement of observer counts for every
// NPC in the player's subscription set. Mirrors @2004scape/rsbuf's
// removePlayer(pid) at rsbuf.d.ts:6, whose WASM internals iterate
// the player's build.npcs set and decrement each NPC's observer
// count. Caller supplies the subscription set because goscape's
// pkg/rsbuf doesn't own the per-player BuildArea.
//
// Safe to call with a nil or empty set.
func RemovePlayer(pid int, subscribedNpcs map[int]struct{}) {
    for nid := range subscribedNpcs {
        decNpcObserver(nid)
    }
}

// incNpcObserver increments nid's observer count. Unexported;
// called only from EncodeNpc.
func incNpcObserver(nid int) { npcObservers[nid]++ }

// decNpcObserver decrements nid's observer count, flooring at 0.
// Matches @2004scape/rsbuf semantics (Math.max(x - 1, 0) in the TS
// shim wrapper). Unexported; called from EncodeNpc remove sites
// and RemovePlayer.
func decNpcObserver(nid int) {
    if v := npcObservers[nid]; v > 0 {
        npcObservers[nid] = v - 1
    }
}
```

Why package-level state, not per-NPC field:
- The `@2004scape/rsbuf` public API takes an `nid` (not an Npc pointer),
  so the storage is nid-keyed internally. Mirroring the API shape forces
  nid-keyed storage on the goscape side too.
- Keeps pkg/rsbuf self-contained (no interface extension, no cross-package
  field writes).
- Per-test state reset via `cleanup` helper in `npc_observers_test.go`
  that does `clear(npcObservers)` — matches the `@2004scape/rsbuf`
  `cleanup()` function at `rsbuf.d.ts:14`.

`pkg/rsbuf/npcinfo.go` hook sites — 3 additions:

- At the first delete (`:39`, inside the `!ok || !n.Active()` branch):
  `decNpcObserver(nid)` before `delete(ba.Npcs, nid)`. Fires for both
  the `!ok` (server-side NPC gone) and `!n.Active()` (despawning)
  sub-cases — both are observer-losing transitions. The nid is known
  regardless of whether the NpcSource lookup succeeded.
- At the second delete (`:46`, level-mismatch / out-of-zone-range
  branch): `decNpcObserver(nid)` before `delete(ba.Npcs, nid)`.
- At the add (`:108`): `incNpcObserver(nid)` after
  `ba.Npcs[nid] = struct{}{}`.

Because storage is nid-keyed rather than pointer-keyed, the nil-guard
concern from the earlier draft evaporates — we use the nid directly,
which is always available in the encode loop regardless of whether
the server-side NpcSource exists.

### `processLogouts` hook

```go
// Inside processLogouts, before tearing down the player:
if p.buildArea != nil {
    rsbuf.RemovePlayer(p.slot, p.buildArea.Npcs)
}
```

Lives in `modules/world/tick.go` next to the existing `processLogouts`
body. Mirrors the TS call shape (`rsbuf.removePlayer(pid)`); the
BuildArea set is a goscape-specific second argument because goscape's
rsbuf doesn't own per-player build-area state (see D5).

### `*entity.Obj` interface methods

```go
// Slot returns -1 because objs are not slot-indexed (unlike Players
// and Npcs). Mirrors *entity.Loc.Slot. Required for the world.entity
// interface so objs can be assigned to Npc.huntTarget.
func (o *Obj) Slot() int { return -1 }

// Coords returns the obj's tile position. Required for the
// world.entity interface.
func (o *Obj) Coords() (x, z, level int) {
    return o.X, o.Z, o.Level
}
```

## Data flow

### Observer-count lifecycle

```
Tick N:
  processNpcs
    processNpcHunt(n)
      observers := rsbuf.GetNpcObservers(n.nid)   ← reads counter maintained in tick N-1
      ... PAUSEHUNT gate ...
  ...
  processInfo
    EncodeNpc(p, ...)                             ← writes counter for tick N+1's read
      phase-1 remove branches: decNpcObserver(nid)
      phase-2 add branch:      incNpcObserver(nid)
```

One-tick lag is inherent to `processNpcs` running before `processInfo`
in the tick order (`tick.go:41` vs `:44`). TS has the same ordering
shape, so this is fidelity-preserving, not a divergence.

### Hunt-variant control flow

```
Npc.turn()
  processNpcHunt(n)                       ← observer gate + throttle
    n.huntAll(s, hunt)                    ← mode dispatch
      case HuntModeNpc:     huntNpcs
      case HuntModeObj:     huntObjs
      case HuntModeScenery: huntLocs
        ↓
      []entity (filtered candidates)
        ↓
      hunted[rand.IntN(len(hunted))]  →  n.huntTarget
```

## Error handling

1. **Nil registries** (`s.grid == nil`, `s.zoneMap == nil`,
   `s.npcTypes == nil`, etc.) — each variant returns `nil` early.
   Matches NAI-8's defensive pattern.
2. **Out-of-range type IDs** on `CheckNpc`/`CheckObj`/`CheckLoc` — type
   equality simply fails, loop skips. No special branch.
3. **Out-of-range type IDs on the iterated entity** (e.g., a zone-loc
   with `l.Type() >= len(s.locTypes.Configs)`) when `CheckCategory`
   filter is active — skip the entity. Matches TS `NpcType.get(type)`
   returning a default if missing, which then fails category compare.
4. **Observer-count underflow** — floored at 0 in `decNpcObserver`.
   Matches `@2004scape/rsbuf` semantics observable via the shim's
   `Math.max(x-1, 0)` pattern.
5. **Re-subscription during mid-tick** — not possible; NpcInfo encode
   is monotonic within a single `processInfo` pass per player.

## Deviations (tracked)

- **D1** (retroactively legitimate via Q3 decision, and carried from
  NAI-8): `huntNpcs` iterates `s.grid.NearbyNpcs` rather than
  zone-iterating. Set-equivalent with TS (both are zone-bucketed);
  iteration order differs (grid walks dx/dz ascending, TS walks
  maxX→minX, maxZ→minZ). `huntAll` feeds the result to `rand.IntN`
  uniformly, so the selected target's distribution is identical.
- **D2**: `CheckVis` (LoS/LoW) gate deferred for **all four** hunt
  variants — huntPlayers (NAI-8 silent gap, now tracked), huntNpcs,
  huntObjs, huntLocs. Added to `nai_followups.md` for future audit
  pass alongside NAI-8's six deferred filters.
- **D3**: TS HuntIterator's stale-tick throw (`currentTick > this.tick`
  at ScriptIterators.ts:81, :100, :124, :147) not ported. Go
  iteration is synchronous per-tick; the TS premise (lazy generator
  outliving the tick) doesn't apply. No-op divergence.
- **D4**: Observer-counter storage matches the `@2004scape/rsbuf`
  public API shape (nid-keyed, rsbuf-owned) via
  `rsbuf.GetNpcObservers(nid) int`. Internal representation is a
  package-level `map[int]int` rather than the WASM's internal
  per-NPC field (not source-available from
  `Engine-TS/node_modules/@2004scape/rsbuf/dist/`). Semantically
  identical; storage-representation divergence is an
  implementation-language artefact.
- **D5**: Logout cleanup API signature diverges from
  `@2004scape/rsbuf`. TS public API is `removePlayer(pid: number): void`
  (rsbuf.d.ts:6) — the WASM accesses `PLAYERS[pid].build.npcs`
  internally. goscape's `pkg/rsbuf` doesn't own per-player BuildArea
  state (the stateless-encoder pattern established by prior ports
  pre-dates this sub-spec), so `rsbuf.RemovePlayer` takes the
  subscription set as a second argument:
  `rsbuf.RemovePlayer(pid int, subscribedNpcs map[int]struct{})`.
  Called from `modules/world.processLogouts` with
  `p.buildArea.Npcs`. Observable behaviour matches TS; API shape
  differs only in the caller-supplied BuildArea reference.
- **D6**: `huntObjs` iterates `Zone.Objs` which by construction
  contains only `LifecycleDespawn` (dynamic) objs
  (`pkg/zone/zone.go:221`). Matches TS intent (inline comment
  "scripting only cares about dynamic objs??" at
  `ScriptIterators.ts:122`).

## Test strategy

### `pkg/zone/map_test.go` additions

1. `TestNearbyZonesRadius0ReturnsOnlyCenter` — zoneRadius 0 returns a
   slice of 1 zone (the center, if materialised).
2. `TestNearbyZonesRadius1ReturnsUpTo9Zones` — zoneRadius 1 returns
   all materialised zones in the 3×3 neighbourhood.
3. `TestNearbyZonesSkipsUnmaterialisedZones` — of a 3×3 neighbourhood
   with only 2 materialised zones, exactly those 2 are returned.
4. `TestNearbyZonesLevelFilter` — same (x, z) at different levels
   yields disjoint zone sets.
5. `TestNearbyZonesBoundaryClampsNegativeZoneCoords` — center near
   (0, 0) does not produce negative-zone lookups.

### `pkg/entity/obj_test.go` additions

1. `TestObjSatisfiesEntityInterface` — compile-time assertion
   `var _ someInterface = (*Obj)(nil)` where `someInterface` has
   `Slot() int` + `Coords() (x, z, level int)`.
2. `TestObjSlotReturnsNegativeOne` and `TestObjCoords`.

### `pkg/rsbuf/npc_observers_test.go` (new file)

Unit tests for the counter in isolation:

1. `TestGetNpcObserversDefaultZero` — fresh map; any nid returns 0.
2. `TestIncNpcObserverIncrements` — after two inc calls for same
   nid, `GetNpcObservers` returns 2.
3. `TestDecNpcObserverFloorsAtZero` — dec with zero count stays at
   0 (no underflow, no negative).
4. `TestDecNpcObserverDecrementsPositiveCount` — inc twice, dec
   once, expect 1.
5. `TestRemovePlayerDecrementsEachSubscribedNid` — seed counts;
   call `RemovePlayer(pid, {nidA, nidB})`; assert both decremented
   by 1.
6. `TestRemovePlayerEmptySetIsNoOp` — empty map; call RemovePlayer;
   assert no panic and no counts changed.
7. `TestRemovePlayerNilSetIsNoOp` — nil map; call RemovePlayer;
   assert no panic.

All tests must `clear(npcObservers)` at top (either via a helper or
direct access since they're in the same package).

### `pkg/rsbuf/npcinfo_test.go` additions

Three tests exercising the counter through the real EncodeNpc path:

1. `TestEncodeNpcAddIncrementsObservers` — empty subscription; one
   nearby active NPC; assert `GetNpcObservers(nid) == 1` after
   encode.
2. `TestEncodeNpcRemoveOnInactiveDecrementsObservers` — pre-seed
   subscription + observer count = 1; NPC becomes
   `Active() == false`; assert count decrements to 0 after encode.
3. `TestEncodeNpcRemoveOnOutOfRangeDecrementsObservers` — pre-seed
   subscription + count = 1; NPC moves to zone-distance > 15;
   assert count decrements to 0 after encode.

Tests must reset package state (`clear(npcObservers)`) before each
run to avoid cross-test pollution. Add a `TestMain` helper if not
already present.

### `modules/world/npc_hunt_entities_test.go` (new file)

Per variant, nine tests (×3 variants = 27; bundled in one file
since shape is identical):

1. `TestHunt<X>InRangeSameLevel` — one candidate in range, one out;
   assert only in-range returned.
2. `TestHunt<X>DifferentLevelExcluded` — same (x, z) as NPC, different
   level; assert excluded.
3. `TestHunt<X>CheckTypeFilter` — two candidates different types;
   with `CheckNpc/Obj/Loc == t1`, assert only type-t1 returned.
4. `TestHunt<X>CheckTypeNegativeOneAllowsAll` — with `CheckNpc/Obj/Loc ==
   -1`, all candidates returned regardless of type.
5. `TestHunt<X>CheckCategoryFilter` — two candidates different
   categories; with `CheckCategory == c1`, assert only cat-c1
   returned.
6. `TestHunt<X>CheckCategoryNegativeOneAllowsAll`.
7. `TestHunt<X>ChebyshevDistance` — candidate at
   `(x+huntRange, z+huntRange)` included (boundary), at
   `(x+huntRange+1, z)` excluded.
8. `TestHunt<X>MissingTypeConfigDoesNotPanic` — candidate with
   `Type >= len(s.<X>Types.Configs)` and `CheckCategory != -1` —
   silently skipped.
9. `TestHunt<X>NilRegistriesReturnsNil`.

Plus one integration:

- `TestHuntAllPicksFromVariantResult` — seeded `rand.IntN` stub via
  existing test pattern; verifies `n.huntTarget` is one of the
  variant's returned candidates.

### `modules/world/npc_event_queue_test.go` modifications

1. Rename `TestProcessNpcHuntPauseHuntRunsWithObserverStub` →
   `TestProcessNpcHuntPauseHuntBailsWithNoObservers`. Change
   assertion from `huntClock == 1` to `huntClock == 0`. Rationale:
   no observers seeded; PAUSEHUNT gate short-circuits.
2. Add `TestProcessNpcHuntPauseHuntRunsWithObservers` — seed the
   rsbuf package state (`rsbuf.SetObserverForTest(n.nid, 1)` — a
   `_test.go`-only export helper lives in
   `pkg/rsbuf/npc_observers_test_export_test.go` with the
   `export_test.go` suffix convention), run tick, assert
   `huntClock == 1`.
3. Add `TestProcessLogoutsDecrementsSubscribedNpcObservers` —
   set up a player with `p.buildArea.Npcs = {nid1: {}, nid2: {}}`
   and seed `rsbuf.SetObserverForTest(nid1, 1)` +
   `rsbuf.SetObserverForTest(nid2, 1)`; run `processLogouts`;
   assert `rsbuf.GetNpcObservers(nid1) == 0` and
   `rsbuf.GetNpcObservers(nid2) == 0`.

### Fixtures

- `newTestServer` / `addPlayerToServer` helpers from NAI-8 reused.
- New helper `addNpcToServer(s, nid, typeId, x, z, level)` — sets
  `s.npcs[nid]`, `s.grid.AddNpc`, and registers a minimal
  `NpcType` in `s.npcTypes.Configs[typeId]`.
- New helper `addLocToZone(s, level, x, z, typeId)` —
  `s.zoneMap.Get(level, x, z).Locs = append(...)`, registers
  `s.locTypes.Configs[typeId]`.
- Similar for objs.

## Rough LOC

| Area | Prod | Test |
|---|---|---|
| `pkg/rsbuf/npc_observers.go` + 3 hook sites in npcinfo.go | ~50 | ~120 |
| `pkg/zone/map.go` NearbyZones + helper test | ~20 | ~60 |
| `pkg/entity/obj.go` Slot/Coords + test | ~10 | ~20 |
| `modules/world/npc_hunt_entities.go` (3 variants) | ~140 | ~280 |
| `modules/world/npc_hunt.go` edits (stub deletion + gate read) | ~2 | — |
| `processLogouts` hook (single rsbuf.RemovePlayer call) | ~3 | ~40 |
| `npc_event_queue_test.go` flip + 2 companions | — | ~60 |
| `nai_followups.md` edits | — (prose) | — |
| **Total** | **~205** | **~520** |

Combined ~725 LOC. Over the roadmap's ~180 estimate because the
observer-counter infrastructure was underscoped in the roadmap —
NAI-7 inlined the stub, NAI-9 was charged with the fix, but the
fix is a real subsystem: a new `pkg/rsbuf/npc_observers.go` file
that mirrors `@2004scape/rsbuf`'s public observer API, three hook
sites in `EncodeNpc`, a logout cleanup API, and tests for each of
those edges.

## Dependencies

- **Blocks:** NAI-10 (consumeHuntTarget) — requires `n.huntTarget`
  populated from all four variants.
- **Blocked by:** NAI-7 (hunt skeleton + variant stubs + HuntType
  infrastructure).

## Verifications to resolve during plan-write

1. Confirm `processLogouts` exact location and cleanup order — the
   `rsbuf.RemovePlayer` call must run BEFORE `p.buildArea` is
   cleared or the player struct is invalidated.
2. Confirm `NpcType.Category`, `ObjType.Category`, `LocType.Category`
   all use `-1` as the "uncategorised" sentinel (already verified for
   NpcType/ObjType/LocType defaults in brainstorm).
3. Confirm whether `objtype.NPCTypeConfigs` is the correct type name
   (spec-reference is case-inconsistent elsewhere in the codebase:
   `NPCTypeConfigs` vs `NpcTypeConfigs`).
4. Confirm `Zone.Locs` iteration is safe during a hunt pass (no
   concurrent zone mutation; NPC hunt runs inside `processNpcs` —
   zone events fire in `processZones` later in the tick).
5. Package-state leak between test runs in `pkg/rsbuf` — existing
   tests may or may not run in `-parallel`. `clear(npcObservers)` in
   a per-test setup helper is the fix; plan must include it.
