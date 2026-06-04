# NAI-86: Port LOC_CHANGE / LOC_ADD / LOC_DEL / LOC_ANIM + lifecycle revert tick processor

**Date**: 2026-05-04
**Cadence**: full sub-spec, 4 bundles, per-bundle Sonnet review (per
`runescript_cadence.md`; estimated ~250 production LOC + ~300 test LOC,
above the `compressed_cadence.md` ≤15 LOC band).
**Predecessor**: NAI-85 (HEAD `4df135a` — LOC_PARAM/NAME/TYPE/SHAPE
active-loc readers ported; door-click LOC_PARAM cascade-blocker
silenced).
**Smoke binding (post-NAI-85)**: door-click smoke at HEAD `4df135a`
confirmed LOC_PARAM silenced; new cascade-blocker surfaced at the same
site as `no handler for LOC_CHANGE (opcode 3004) at pc=50` inside
`[proc,open_and_close_door]`.
**Successor**: TBD (drained by NAI-N+1 user-driven door-click smoke).

## 1. Problem

Tutorial Island door-click smoke at HEAD `4df135a` reveals
`[proc,open_and_close_door]` aborts at pc=50:

```
WARN script="[proc,open_and_close_door]" err="no handler for LOC_CHANGE (opcode 3004) at pc=50"
```

The proc body (`Content/scripts/doors/scripts/open_and_close_doors.rs2`,
lines 9-40) calls `loc_change(inviswall, 3)` immediately followed by
`loc_add(movecoord(...), $replacement, modulo(add($angle, 1), 4), $shape, 3)`.
LOC_CHANGE is the cascade-trigger; LOC_ADD will be the next
cascade-blocker once LOC_CHANGE lands. Door does not animate, never
becomes walkable, never auto-reverts.

Pattern: `protocol_stub_not_completed` (per `nai_followups.md` and the
NAI-83/NAI-85 routing rule). Family of 4 mutator opcodes declared in
`pkg/script/opcode.go` (3000/3002/3004/3006) but with no dispatch
wiring:

| Opcode | Constant decl | Name decl | Handler |
|---|---|---|---|
| LOC_ADD (3000) | `pkg/script/opcode.go:289` | `pkg/script/opcode.go:969-970` | **missing** |
| LOC_ANIM (3002) | `pkg/script/opcode.go:291` | `pkg/script/opcode.go:973-974` | **missing** |
| LOC_CHANGE (3004) | `pkg/script/opcode.go:293` | `pkg/script/opcode.go:977-978` | **missing** |
| LOC_DEL (3006) | `pkg/script/opcode.go:295` | `pkg/script/opcode.go:981-982` | **missing** |

Per Q1 brainstorm (option C confirmed), the bundle drains the full
`LocOps.ts` mutator family even though only LOC_CHANGE + LOC_ADD have
content-script consumers in the smoke path. Opens the file once.

LOC_CHANGE alone is **not** sufficient. TS `World.changeLoc`
(`World.ts:1350-1386`) hinges on:
- `Loc.change(type, shape, angle)` → mutates `currentInfo` (TS bitfield
  is `baseInfo` immutable + `currentInfo` mutable; goscape has only
  `Info` — single bitfield, no change/revert support).
- `changeLocCollision(...)` → updates pathfinder collision flags
  (`gamemap.ChangeLocCollision` exists in goscape but is **not** wired
  into `Server.AddLoc/ChangeLoc/RemoveLoc`).
- `loc.setLifeCycle(duration)` → registers in `World.locObjTracker` for
  per-tick revert. **Goscape has zero per-tick Loc/Obj despawn
  driver.**
- `World.processZones` → iterates `locObjTracker`, calls
  `entity.turn()` → DESPAWN+active→Remove,
  RESPAWN+changed+active→Revert, RESPAWN+!active→Add.
- `World.revertLoc` (`World.ts:1427-1448`) → snap to baseInfo + collision
  swap + zone wire event.

Without the lifecycle revert tick processor, the door opens and never
closes (smoke residual unacceptable per Q2 brainstorm).

## 2. TS reference

**`LostCityRS/Engine-TS/src/engine/script/handlers/LocOps.ts:18-77`**:

```ts
[ScriptOpcode.LOC_ADD]: state => {
    const [coord, type, angle, shape, duration] = state.popInts(5);
    const position: CoordGrid = check(coord, CoordValid);
    const locType: LocType = check(type, LocTypeValid);
    const locAngle: LocAngle = check(angle, LocAngleValid);
    const locShape: LocShape = check(shape, LocShapeValid);
    const locLayer = locShapeLayer(locShape);
    check(duration, DurationValid);

    // Search through zone and change a loc if it's on the same layer
    const locs = World.gameMap.getZone(position.x, position.z, position.level)
        .getLocsUnsafe(CoordGrid.packZoneCoord(position.x, position.z));
    for (const loc of locs) {
        if (loc.layer === locLayer) {
            World.changeLoc(loc, type, locShape, locAngle, duration);
            state.activeLoc = loc;
            state.pointerAdd(ActiveLoc[state.intOperand]);
            return;
        }
    }

    const created: Loc = new Loc(position.level, position.x, position.z,
        locType.width, locType.length, EntityLifeCycle.DESPAWN,
        locType.id, locShape, locAngle);
    World.addLoc(created, duration);
    state.activeLoc = created;
    state.pointerAdd(ActiveLoc[state.intOperand]);
},

[ScriptOpcode.LOC_ANIM]: checkedHandler(ActiveLoc, state => {
    const seqType: SeqType = check(state.popInt(), SeqTypeValid);
    World.animLoc(state.activeLoc, seqType.id);
}),

[ScriptOpcode.LOC_CHANGE]: checkedHandler(ActiveLoc, state => {
    const [id, duration] = state.popInts(2);
    check(duration, DurationValid);
    check(id, LocTypeValid);
    World.changeLoc(state.activeLoc, id, state.activeLoc.shape, state.activeLoc.angle, duration);
}),

[ScriptOpcode.LOC_DEL]: checkedHandler(ActiveLoc, state => {
    const duration: number = check(state.popInt(), DurationValid);
    World.removeLoc(state.activeLoc, duration);
}),
```

**`LostCityRS/Engine-TS/src/engine/World.ts:1337-1448`**: `addLoc`,
`changeLoc`, `removeLoc`, `revertLoc` — collision wiring + lifecycle
scheduling.

**`LostCityRS/Engine-TS/src/engine/entity/Loc.ts`**: `baseInfo` /
`currentInfo` split + `change` / `revert` / `isChanged` / `layer`. Note
`layer` reads from **baseInfo** (set at construction; immutable).

**`LostCityRS/Engine-TS/src/engine/entity/NonPathingEntity.ts:11-25`**:
`setLifeCycle` registers/unlinks in `World.locObjTracker`.

**`LostCityRS/Engine-TS/src/engine/World.ts:961-986`** (`processZones`):
iterates `locObjTracker`, calls `event.entity.turn()`, then computes
shared per zone.

**`LostCityRS/Engine-TS/src/engine/entity/Loc.ts:54-74`** (`Loc.turn`):
the four-branch dispatch above.

**Validators (`ScriptValidators.ts`)**:
- `DurationValid` — `[1, 2147483647]`
- `LocTypeValid`, `LocAngleValid`, `LocShapeValid`, `SeqTypeValid` —
  nil-checks / range checks already familiar from NAI-83/NAI-85.

## 3. Existing goscape surface

| Concern | Location | Status |
|---|---|---|
| `Loc.Info` (single bitfield) | `pkg/entity/loc.go:8` | **needs split into `BaseInfo` + `CurrentInfo`** |
| `Loc.Type/Shape/Angle` accessors | `pkg/entity/loc.go:28-34` | rewire to read `CurrentInfo` |
| `Loc.IsValid` | `pkg/entity/loc.go:57-59` | keep — intrinsic; new `IsActive bool` field added separately |
| `NonPathing` base | `pkg/entity/nonpathing.go:6-9` | **needs `SetLifeCycle(duration, currentTick, tracker) ` override + tracker hook** |
| `Server.AddLoc/ChangeLoc/RemoveLoc/AnimLoc` | `modules/world/world_zone.go:8-44` | **needs collision wiring + signature change to take `duration`** |
| `Server.locObjTracker` | — | **new** — DLL of `*entity.NonPathing` |
| `Server.processZones` step | — | **new** — slot in `tick.go` between `processNpcs` and `processInfo` |
| `Server.RevertLoc` | — | **new** |
| `gamemap.ChangeLocCollision` | `pkg/gamemap/gamemap.go:50-61` | exists; ready to call |
| `LocType.BlockWalk` | `pkg/objtype/loctype.go:29` | exists (`bool`) |
| `LocType.BlockRange` | `pkg/objtype/loctype.go:30` | exists |
| `Zone.LocsAtCoord(packedZoneCoord)` | — | **new** — needed for LOC_ADD's same-layer search |
| `ActiveLoc` interface | `pkg/script/active.go:698-703` | already gained `Shape()` in NAI-85; needs **no** further additions for handlers themselves |
| `ScriptState.World` | tbd at plan-write | **needs `ChangeLoc/AddLoc/RemoveLoc/AnimLoc/LocsAtCoord` methods** |
| `requireActiveLoc` / `requireConfigs` gates | `pkg/script/handlers_loc.go:12-17` | available |
| `checkLocAngle`, `checkLocShape` validators | `pkg/script/handlers_player.go:87-102` | available (NAI-83 era) |
| `checkDuration` validator | — | **new** — `[1, 2147483647]` |
| `checkLocType` / `checkSeqType` Configs lookup | — | **new** helper (one for each); `checkLocType` is reused inside Bundle 3 handlers |
| Loc tick processor | — | **new** — `Server.turnLoc(loc, tick)` |

## 4. Solution

### 4.1 Bundle 1 — Loc entity + collision foundation

**4.1.1** `pkg/entity/loc.go`: split `Info int` into `BaseInfo int` (immutable
post-construction) + `CurrentInfo int` (mutable). Both packed by
`packLocInfo(typ, shape, angle)`. Layer bits 21-22 added to packing
(per TS `Loc.ts:20-24`):

```go
func packLocInfo(typ, shape, angle int) int {
    layer := loc.LayerOf(loc.Shape(shape))  // pkg/objtype/loc.LayerOf
    return (typ & 0x3FFF) |
        (shape&0x1F)<<14 |
        (angle&0x3)<<19 |
        (int(layer)&0x3)<<21
}
```

Add methods:
```go
func (l *Loc) Type() int  { return l.CurrentInfo & 0x3FFF }       // rewire from Info
func (l *Loc) Shape() int { return (l.CurrentInfo >> 14) & 0x1F } // rewire from Info
func (l *Loc) Angle() int { return (l.CurrentInfo >> 19) & 0x3 }  // rewire from Info
func (l *Loc) Layer() int { return (l.BaseInfo >> 21) & 0x3 }     // baseInfo per TS
func (l *Loc) IsChanged() bool { return l.CurrentInfo != l.BaseInfo }
func (l *Loc) Change(typ, shape, angle int) {
    l.CurrentInfo = packLocInfo(typ, shape, angle)
}
func (l *Loc) Revert() { l.CurrentInfo = l.BaseInfo }
```

Add `IsActive bool` field. Pre-existing `Loc.IsValid()` keeps its
intrinsic-validity meaning — `IsActive` is the "in world right now"
flag that gates `Loc.Turn` branches.

`NewLoc` sets `BaseInfo = CurrentInfo = packLocInfo(typ,shape,angle)`,
`IsActive = false`. Server.AddLoc sets `IsActive = true` after the zone
wire.

**4.1.2** `modules/world/world_zone.go`: rewrite `Server.AddLoc`,
`Server.ChangeLoc`, `Server.RemoveLoc` per Section 2 of the brainstorm:

```go
func (s *Server) AddLoc(loc *entitypkg.Loc, duration int) {
    if s.gamemap != nil && s.configs != nil {
        if lt := s.configs.LocType(loc.Type()); lt != nil && lt.BlockWalk {
            s.gamemap.ChangeLocCollision(loc.Shape(), loc.Angle(), lt.BlockRange,
                loc.Length, loc.Width, loc.X, loc.Z, loc.Level, true)
        }
    }
    z := s.zoneMap.Get(loc.Level, loc.X, loc.Z)
    z.AddLoc(loc)
    loc.IsActive = true
    s.TrackZone(z)
    loc.SetLifeCycle(duration, s.currentTick, s.locObjTracker)
}

func (s *Server) ChangeLoc(loc *entitypkg.Loc, typ, shape, angle, duration int) {
    if loc.Lifecycle == entitypkg.LifecycleDespawn && !loc.IsActive {
        return  // TS guard: don't return inactive DESPAWN to game world
    }
    // remove old collision
    if loc.IsActive && s.gamemap != nil && s.configs != nil {
        if oldLt := s.configs.LocType(loc.Type()); oldLt != nil && oldLt.BlockWalk {
            s.gamemap.ChangeLocCollision(loc.Shape(), loc.Angle(), oldLt.BlockRange,
                loc.Length, loc.Width, loc.X, loc.Z, loc.Level, false)
        }
    }
    loc.Change(typ, shape, angle)
    // add new collision
    if s.gamemap != nil && s.configs != nil {
        if newLt := s.configs.LocType(typ); newLt != nil && newLt.BlockWalk {
            s.gamemap.ChangeLocCollision(loc.Shape(), loc.Angle(), newLt.BlockRange,
                loc.Length, loc.Width, loc.X, loc.Z, loc.Level, true)
        }
    }
    z := s.zoneMap.Get(loc.Level, loc.X, loc.Z)
    z.ChangeLoc(loc)
    s.TrackZone(z)
    if loc.IsChanged() || loc.Lifecycle == entitypkg.LifecycleDespawn {
        loc.SetLifeCycle(duration, s.currentTick, s.locObjTracker)
    } else {
        loc.SetLifeCycle(-1, s.currentTick, nil)  // no-op change to static; untrack
    }
}

func (s *Server) RemoveLoc(loc *entitypkg.Loc, duration int) {
    if !loc.IsActive { return }
    if s.gamemap != nil && s.configs != nil {
        if lt := s.configs.LocType(loc.Type()); lt != nil && lt.BlockWalk {
            s.gamemap.ChangeLocCollision(loc.Shape(), loc.Angle(), lt.BlockRange,
                loc.Length, loc.Width, loc.X, loc.Z, loc.Level, false)
        }
    }
    z := s.zoneMap.Get(loc.Level, loc.X, loc.Z)
    z.RemoveLoc(loc)
    loc.IsActive = false
    s.TrackZone(z)
    if loc.Lifecycle == entitypkg.LifecycleRespawn {
        loc.SetLifeCycle(duration, s.currentTick, s.locObjTracker)
    } else {
        loc.SetLifeCycle(-1, s.currentTick, nil)
    }
}
```

**Signature changes** (call-site enumeration):
- `Server.AddLoc(loc)` → `Server.AddLoc(loc, duration)` — touch
  `gamemap.StaticLocs()` loop call site (passes `0`); test fixtures
  `addLocToZone` and friends.
- `Server.ChangeLoc(loc)` → `Server.ChangeLoc(loc, typ, shape, angle,
  duration)` — currently has zero call sites outside of itself.
- `Server.RemoveLoc(loc)` → `Server.RemoveLoc(loc, duration)` —
  enumerate at plan-write.

Per `enumerate_all_sites.md` + `plan_enumerate_struct_literals.md`:
plan-write greps every `s.AddLoc(`, `s.ChangeLoc(`, `s.RemoveLoc(` call
site and lists them with required edit; controller pre-flights at
dispatch.

**Bundle 1 tests**: `pkg/entity/loc_test.go` — `TestLocChange`,
`TestLocRevert`, `TestLocIsChanged`, `TestLocLayerReadsFromBaseInfo`.
`modules/world/world_zone_test.go` — `TestServerAddLocAddsCollision`,
`TestServerChangeLocSwapsCollision`, `TestServerRemoveLocClearsCollision`.

### 4.2 Bundle 2 — Lifecycle revert tick processor

**4.2.1** `pkg/entity/nonpathing.go`: add `LifecycleTracker` interface +
`SetLifeCycle(duration, currentTick, tracker)` override:

```go
type LifecycleTracker interface {
    Register(np *NonPathing)
    Unregister(np *NonPathing)
}

type NonPathing struct {
    Entity
    parent  any  // back-pointer to *Loc / *Obj (set by NewLoc / NewObj)
    tracker LifecycleTracker
    // trackerNode opaque — tracker owns the DLL element
}

func (np *NonPathing) Parent() any { return np.parent }

func (np *NonPathing) SetLifeCycle(duration, currentTick int, tracker LifecycleTracker) {
    if np.tracker != nil {
        np.tracker.Unregister(np)
        np.tracker = nil
    }
    if duration > 0 {
        tracker.Register(np)
        np.tracker = tracker
        np.SetLifecycle(currentTick+duration, currentTick)  // schedule transition tick
    } else {
        np.SetLifecycle(-1, currentTick)
    }
}
```

`NewLoc` does `l.parent = l` (and same for `NewObj` though Obj.Turn
ports later — keeping the back-pointer wired now means Bundle 2 doesn't
get re-opened to retrofit it).

**4.2.2** `modules/world/server_loc_tracker.go` (new):

```go
type locObjTracker struct {
    list *zone.DoublyLinkList[*entity.NonPathing]
    nodes map[*entity.NonPathing]*zone.Element[*entity.NonPathing]
}

func newLocObjTracker() *locObjTracker { ... }

func (t *locObjTracker) Register(np *entity.NonPathing) {
    if existing, ok := t.nodes[np]; ok {
        existing.Unlink()
    }
    t.nodes[np] = t.list.AddTail(np)
}

func (t *locObjTracker) Unregister(np *entity.NonPathing) {
    if e, ok := t.nodes[np]; ok {
        e.Unlink()
        delete(t.nodes, np)
    }
}

func (t *locObjTracker) All() iter.Seq[*entity.NonPathing] { return t.list.All(false) }
```

`Server` embeds `locObjTracker = newLocObjTracker()` in `New`.

**4.2.3** `modules/world/server_zone_tick.go` (new):

```go
func (s *Server) processZones() {
    // Iterate tracker. Snapshot to avoid mid-iteration mutation
    // (turnLoc may call SetLifeCycle which Unregisters).
    snap := make([]*entity.NonPathing, 0)
    for np := range s.locObjTracker.All() {
        snap = append(snap, np)
    }
    for _, np := range snap {
        switch p := np.Parent().(type) {
        case *entity.Loc:
            s.turnLoc(p, s.currentTick)
        case *entity.Obj:
            // Obj.Turn ports later; no-op for now
        }
    }
    for z := range s.zonesTracking {
        z.ComputeShared()
    }
}

func (s *Server) turnLoc(l *entity.Loc, now int) {
    if l.LifecycleTick != now { return }
    switch {
    case l.Lifecycle == entity.LifecycleDespawn && l.IsActive:
        s.RemoveLoc(l, 0)
    case l.Lifecycle == entity.LifecycleRespawn && l.IsChanged() && l.IsActive:
        s.RevertLoc(l)
    case l.Lifecycle == entity.LifecycleRespawn && !l.IsActive:
        s.AddLoc(l, 0)
    default:
        s.log.Error("loc tracked but no event matched",
            "type", l.Type(), "x", l.X, "z", l.Z)
        l.SetLifeCycle(-1, now, nil)
    }
}

func (s *Server) RevertLoc(l *entity.Loc) {
    if s.gamemap != nil && s.configs != nil {
        if oldLt := s.configs.LocType(l.Type()); oldLt != nil && oldLt.BlockWalk {
            s.gamemap.ChangeLocCollision(l.Shape(), l.Angle(), oldLt.BlockRange,
                l.Length, l.Width, l.X, l.Z, l.Level, false)
        }
    }
    l.Revert()
    if s.gamemap != nil && s.configs != nil {
        if newLt := s.configs.LocType(l.Type()); newLt != nil && newLt.BlockWalk {
            s.gamemap.ChangeLocCollision(l.Shape(), l.Angle(), newLt.BlockRange,
                l.Length, l.Width, l.X, l.Z, l.Level, true)
        }
    }
    z := s.zoneMap.Get(l.Level, l.X, l.Z)
    z.ChangeLoc(l)
    s.TrackZone(z)
    l.SetLifeCycle(-1, s.currentTick, nil)  // untrack
}
```

**4.2.4** `modules/world/tick.go`: insert `s.processZones()` between
`s.processNpcs()` and `s.processInfo()`. Mirrors TS `World.cycle` order
at `World.ts:365` block.

**Bundle 2 tests**: `modules/world/loc_turn_test.go` — drive
`s.currentTick` forward, assert revert fires at `ChangeTick+duration`.
Cases:
- `TestLocTurnDespawnRemoves` — DESPAWN+active+tick==0 → loc removed
- `TestLocTurnRespawnChangedReverts` — RESPAWN+isChanged+active+tick==0 → loc reverts
- `TestLocTurnRespawnInactiveReadds` — RESPAWN+!active+tick==0 → loc re-added
- `TestLocTurnNoOpChangeUntracks` — change static to same type → untracked
- `TestLocChangeSchedulesRevertAtCorrectTick` — duration=3, change at tick 100, revert fires tick 103

### 4.3 Bundle 3 — Script handlers

**4.3.1** Validators (`pkg/script/handlers_player.go` siblings, or new
`handlers_loc_validators.go`):

```go
func checkDuration(v int) error {
    if v < 1 || v > 2147483647 {
        return fmt.Errorf("duration out of range [1, 2147483647]: %d", v)
    }
    return nil
}

func checkLocTypeID(s *ScriptState, op string, id int) (*objtype.LocType, error) {
    if s.Configs == nil {
        return nil, fmt.Errorf("%s: configs unavailable", op)
    }
    lt := s.Configs.LocType(id)
    if lt == nil {
        return nil, fmt.Errorf("%s: unknown loc id %d", op, id)
    }
    return lt, nil
}

func checkSeqTypeID(s *ScriptState, op string, id int) (*objtype.SeqType, error) {
    // mirror checkLocTypeID
}
```

Per `plan_grep_helper_patterns.md`: plan-write greps for any existing
`checkDuration`/`checkLocType`/`checkSeqType` helpers before codifying.

**4.3.2** Handlers (`pkg/script/handlers_loc.go`):

```go
func handleLocChange(s *ScriptState) error {
    if err := requireActiveLoc(s, "LOC_CHANGE"); err != nil { return err }
    duration := s.PopInt()
    id := s.PopInt()
    if err := checkDuration(duration); err != nil {
        return fmt.Errorf("LOC_CHANGE: %w", err)
    }
    if _, err := checkLocTypeID(s, "LOC_CHANGE", id); err != nil { return err }
    return s.World.ChangeLoc(s.ActiveLoc, id,
        s.ActiveLoc.Shape(), s.ActiveLoc.Angle(), duration)
}

func handleLocAdd(s *ScriptState) error {
    duration := s.PopInt()
    shape := s.PopInt()
    angle := s.PopInt()
    typ := s.PopInt()
    coord := s.PopInt()
    level, x, z, err := coordgrid.UnpackCoord(coord)
    if err != nil { return fmt.Errorf("LOC_ADD: %w", err) }
    if _, err := checkLocTypeID(s, "LOC_ADD", typ); err != nil { return err }
    if err := checkLocAngle(angle); err != nil { return fmt.Errorf("LOC_ADD: %w", err) }
    if err := checkLocShape(shape); err != nil { return fmt.Errorf("LOC_ADD: %w", err) }
    if err := checkDuration(duration); err != nil { return fmt.Errorf("LOC_ADD: %w", err) }
    layer := loc.LayerOf(loc.Shape(shape))
    // search same-layer existing loc at this coord
    for existing := range s.World.LocsAtCoord(level, x, z) {
        if existing.Layer() == int(layer) {
            if err := s.World.ChangeLoc(existing, typ, shape, angle, duration); err != nil {
                return err
            }
            s.ActiveLoc = existing
            s.PointerAdd(PointerActiveLoc[s.IntOperand])  // tbd at plan-write
            return nil
        }
    }
    created, err := s.World.AddLoc(level, x, z, typ, shape, angle, duration)
    if err != nil { return err }
    s.ActiveLoc = created
    s.PointerAdd(PointerActiveLoc[s.IntOperand])
    return nil
}

func handleLocDel(s *ScriptState) error {
    if err := requireActiveLoc(s, "LOC_DEL"); err != nil { return err }
    duration := s.PopInt()
    if err := checkDuration(duration); err != nil {
        return fmt.Errorf("LOC_DEL: %w", err)
    }
    return s.World.RemoveLoc(s.ActiveLoc, duration)
}

func handleLocAnim(s *ScriptState) error {
    if err := requireActiveLoc(s, "LOC_ANIM"); err != nil { return err }
    seq := s.PopInt()
    if _, err := checkSeqTypeID(s, "LOC_ANIM", seq); err != nil { return err }
    return s.World.AnimLoc(s.ActiveLoc, seq)
}
```

**4.3.3** `ScriptState.World` interface extension. Plan-write greps for
the actual interface location and adds:

```go
ChangeLoc(loc ActiveLoc, typ, shape, angle, duration int) error
AddLoc(level, x, z, typ, shape, angle, duration int) (ActiveLoc, error)
RemoveLoc(loc ActiveLoc, duration int) error
AnimLoc(loc ActiveLoc, seq int) error
LocsAtCoord(level, x, z int) iter.Seq[ActiveLoc]
```

Adapter implementations on `modules/world.Server` type-assert
`ActiveLoc` to `*entitypkg.Loc` then call the bare `Server.AddLoc`/etc.

**4.3.4** `pkg/zone.LocsAtCoord(packedZoneCoord int) iter.Seq[*Loc]` —
new helper. Plan-write confirms zone-coord packing matches TS
`CoordGrid.packZoneCoord`.

**4.3.5** Dispatch wiring in `pkg/script/handlers.go`. Rename "LOC
active-loc reads" sub-block to "LOC active-loc reads + mutations".
Lexical insertion:

```go
OpLocAdd:    handleLocAdd,
OpLocAngle:  handleLocAngle,
OpLocAnim:   handleLocAnim,
OpLocChange: handleLocChange,
OpLocDel:    handleLocDel,
OpLocName:   handleLocName,
OpLocOp:     handleLocOp,
OpLocParam:  handleLocParam,
OpLocShape:  handleLocShape,
OpLocType:   handleLocType,
```

**Bundle 3 tests**: `pkg/script/handlers_loc_test.go` per-handler unit
tests. Mock `World` records `ChangeLoc/AddLoc/RemoveLoc/AnimLoc` calls.
`&ScriptState{}` fixture per `scriptstate_test_fixture_idioms.md` —
StackCapacity init, push-order, `Pointers` set with `ActiveLoc` flag
where `checkedHandler(ActiveLoc, ...)`.

Test cases per handler:
- LOC_CHANGE: `TestLocChangeCallsWorld`, `TestLocChangeRequiresActiveLoc`,
  `TestLocChangeRejectsZeroDuration`, `TestLocChangeRejectsUnknownType`.
- LOC_ADD: `TestLocAddSameLayerCallsChange`, `TestLocAddNewLocCallsAdd`,
  `TestLocAddRejectsBadCoord`, `TestLocAddRejectsBadAngle/Shape/Type/Duration`.
- LOC_DEL: `TestLocDelCallsWorld`, `TestLocDelRequiresActiveLoc`,
  `TestLocDelRejectsZeroDuration`.
- LOC_ANIM: `TestLocAnimCallsWorld`, `TestLocAnimRequiresActiveLoc`,
  `TestLocAnimRejectsUnknownSeq`.

### 4.4 Bundle 4 — Door-click smoke + sub-spec close

User-launched server per `smoke_test_server_handoff.md`. Smoke
expectations:
- Player clicks door (`newbie_door1` Tutorial Island).
- Server log shows `LOC_CHANGE` + `LOC_ADD` opcodes silently complete
  (no `no handler for ...` warnings).
- Door visually animates open (Java client renders the swap).
- Door tile becomes walkable; player walks through.
- After 3 ticks, door auto-reverts to closed; tile becomes blocking
  again.

Acceptance: silent on the LOC_CHANGE/LOC_ADD warnings AND door-close
auto-revert observed. Either residual = open follow-up.

Sub-spec close: retire `nai_followups.md` LOC_CHANGE entry + add new
follow-up entries for any uncovered cascade-blockers surfaced by the
smoke. `Closes memory:` trailer per `close_commit_memory_trailer.md`.

## 5. Tracked deviations

- **D-N86-1** — `LocObjEvent.check()` skipped. TS uses
  `LocObjEvent.check()` to validate the tracker entry against the live
  entity (catches "loc removed externally; tracker still has stale
  node"). Goscape entry-points are exclusively `Server.AddLoc`/
  `ChangeLoc`/`RemoveLoc` — same code path that registers the tracker.
  No external invalidation source. Revisit if external loc-mutation
  entry points appear (e.g., a `Loc.Remove()` outside Server).
  **Pin**: comment + this spec section.

- **D-N86-2** — `Loc.IsValid()` keeps returning `true` (intrinsic
  always-valid). TS `World.changeLoc` early-returns on
  `lifecycle === DESPAWN && !loc.isValid()` — goscape uses
  `!loc.IsActive` (Section 4.1.2) which is the closer
  semantics-equivalent. **Pin**: doc-comment in Server.ChangeLoc per
  `defensive_gate_doc_comment_label.md`.

- **D-N86-3** — `Obj.Turn` not implemented. The `processZones`
  type-switch falls through to a no-op for `*entity.Obj`. Future Obj
  despawn work uses the same tracker. **Pin**: comment in
  `processZones`.

- **D-N86-4** (TS-asymmetry-dual-pin candidate per
  `ts_asymmetry_dual_pin.md`) — TS `Loc.turn` decrements
  `lifecycleTick--` first; goscape's `turnLoc` reads
  `LifecycleTick` directly. The decrement model is needed when
  iteration drives the clock; goscape uses `Server.currentTick` as the
  authoritative clock and stores the **target** transition tick
  absolute. Equivalent observable behavior; pin both presence
  (currentTick comparison) and absence (no per-tick decrement) with
  tests.

## 6. Risks / mitigations

| Risk | Mitigation |
|---|---|
| `enumerate_all_sites.md` — Server.AddLoc/ChangeLoc/RemoveLoc signature changes blast-radius | Plan-write greps every call site; controller pre-flights at dispatch per `controller_preflight.md`; Bundle 1 lists call-site fixups inline |
| `plan_helper_coverage.md` — `addLocToZone` test helper in `modules/world` doesn't carry a duration arg | Plan-write extends signature OR adds variant; enumerate test fixture call sites in plan |
| `plan_var_name_collision.md` — `loc` is both a package import (`pkg/objtype/loc`) and a likely local var name | Plan code blocks use `l` for *Loc local var to avoid collision |
| `gamemap_nil_in_tests` — many fixtures bypass gamemap | Server.AddLoc/etc. nil-check `s.gamemap != nil && s.configs != nil` (Section 4.1.2 already encodes); existing `IsZoneAllocated` precedent |
| Loc.IsActive default for static spawn loop | `gamemap.StaticLocs()` → `s.AddLoc(loc, 0)`. Inside AddLoc, `loc.IsActive = true` runs unconditionally — statics get IsActive=true on first tick, then duration=0 calls `SetLifeCycle(-1, ...)` which untracks. Net effect: static is active and untracked, matches TS. |
| `parent any` field type-assertion failure | Default branch in turnLoc logs + untracks. Tests cover unknown parent type. |
| `dispatch_order_audit_blind_spot.md` — order of collision-update vs zone wire vs lifecycle scheduling | Section 4.1.2 mirrors TS exactly: `(1) remove old collision → (2) Loc.Change → (3) add new collision → (4) zone.ChangeLoc → (5) trackZone → (6) SetLifeCycle`. Plan-write annotates each step with its TS line number. |
| `risk_register_premise_grep.md` — claim "currently has zero call sites outside of itself" for Server.ChangeLoc | Plan-write re-greps `s.ChangeLoc(` and pre-flights at dispatch. |

## 7. Out of scope

- Obj despawn / `Obj.Turn` (deferred; tracker + processZones are
  ready for it).
- LOC_FIND real implementation (still stub from NAI-85 era).
- LOC_FINDALLZONE / LOC_FINDNEXT (separate iterator family).
- LOC_CATEGORY / LOC_COORD (already shipped, NAI-85).
- TS `World.locObjTracker` LinkList → goscape DLL — using existing
  `pkg/zone.DoublyLinkList` if signatures permit; plan-write decides.
- Loc spawn from script (`Loc.NewLoc` constructor signature changes for
  Bundle 3 LOC_ADD code path may require touching).

## 8. Success criteria

- Bundle 1: `go test ./pkg/entity/... ./modules/world/... -run "Loc"`
  green; collision-after-Change integration test passes.
- Bundle 2: `go test ./modules/world/... -run "LocTurn|LifecycleRevert"`
  green; revert fires at correct tick.
- Bundle 3: `go test ./pkg/script/... -run "LocChange|LocAdd|LocDel|LocAnim"`
  green; per-handler validator + dispatch wiring covered.
- Bundle 4: door-click smoke at HEAD `<post-Bundle-3-SHA>` shows
  silent LOC_CHANGE/LOC_ADD AND auto-revert observed.
- Full repo: `go test ./...` and `go vet ./...` green at sub-spec
  close.

## 9. Tech stack

- Go 1.26+ (per `go_version.md`)
- Existing deps only — no new modules
- TS reference: `LostCityRS/Engine-TS` (per `ts_source_canonical_path.md`)
