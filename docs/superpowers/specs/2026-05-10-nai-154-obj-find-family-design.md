# NAI-154 — OBJ_FIND family port (3505 + 3506 + 3507 + 3508 + 3509)

**Cadence:** Mid-band (per `runescript_cadence` / `compressed_cadence`) — separate spec + plan; 5 sequential T-tasks dispatched as Sonnet implementer subagents; combined Sonnet reviewer at end (NOT per-task two-stage).

**Tech stack:** Go 1.26+ (`pkg/script`, `modules/world`).

---

## §1. Background

Successor to NAI-152 / NAI-153 (`a47e9cf`) which closed the mindrune-pickup chain by extending `ActiveObj` with `ObjCount`/`IsValidFor`/`ObjType`/`Coords` and porting `OBJ_TYPE`/`OBJ_COUNT`/`OBJ_TAKEITEM`. The cascade-tail audit (per `missing_handler_audit.md`) at `a47e9cf` reports **34 undispatched opcodes**; the OBJ family (3505–3509) is the largest cohesive sibling cluster on the existing ActiveObj surface.

The five opcodes:

| # | Opcode | TS reference | Shape |
|---|---|---|---|
| 3505 | `OBJ_FIND` | `ObjOps.ts:168-183` | Pops `[coord, objId]`; calls `World.getObj(...)`; on hit sets `state.activeObj` (slot-routed via `IntOperand`) + pushes 1; else pushes 0. |
| 3506 | `OBJ_FINDALLZONE` | `ObjOps.ts:185-189` | Pops `coord`; installs `ObjIterator(currentTick, level, x, z)` into `state.objIterator`. |
| 3507 | `OBJ_FINDNEXT` | `ObjOps.ts:191-201` | Pulls next from `state.objIterator?.next()`; on hit sets `state.activeObj` (slot-routed) + pushes 1; else pushes 0. |
| 3508 | `OBJ_NAME` | `ObjOps.ts:106-110` | Pushes `objType.name ?? objType.debugname ?? 'null'` for `state.activeObj.type`. |
| 3509 | `OBJ_PARAM` | `ObjOps.ts:95-104` | Pops `paramId`; delegates to `ParamHelper.getIntParam`/`getStringParam` on `state.activeObj.type`. |

The iterator pair (3506/3507) directly parallels NAI-119's LOC iterator (`LocIterator` / `LOC_FINDALLZONE` / `LOC_FINDNEXT`); the helper template (`setActiveLocSlot`) and the iterator template (`loc_iterator.go`) are drop-in references. `OBJ_FIND` wires through an existing production-side surface (`Server.GetObj` at `modules/world/obj_lookup.go:13`). `OBJ_NAME` mirrors `handleOcName` exactly. `OBJ_PARAM` mirrors `handleOcParam` minus the explicit objId pop (the active obj's type id is read instead).

The dual-slot pattern (`OtherActiveObj` + `PtrActiveObj2`) is in scope per the brainstorm — `PtrActiveObj2` already exists at `pkg/script/pointer.go:15` but no `OtherActiveObj` field exists, mirroring the NAI-119 starting state.

---

## §2. TS source (verified at Engine-TS HEAD)

### §2.1 `Engine-TS/src/engine/script/handlers/ObjOps.ts:95-201`

```typescript
[ScriptOpcode.OBJ_PARAM]: state => {
    const paramType: ParamType = check(state.popInt(), ParamTypeValid);
    const objType: ObjType = check(state.activeObj.type, ObjTypeValid);
    if (paramType.isString()) {
        state.pushString(ParamHelper.getStringParam(paramType.id, objType, paramType.defaultString));
    } else {
        state.pushInt(ParamHelper.getIntParam(paramType.id, objType, paramType.defaultInt));
    }
},

[ScriptOpcode.OBJ_NAME]: state => {
    const objType: ObjType = check(state.activeObj.type, ObjTypeValid);
    state.pushString(objType.name ?? objType.debugname ?? 'null');
},

[ScriptOpcode.OBJ_FIND]: state => {
    const [coord, objId] = state.popInts(2);
    const objType: ObjType = check(objId, ObjTypeValid);
    const position: CoordGrid = check(coord, CoordValid);
    const obj = World.getObj(position.x, position.z, position.level, objType.id, state.activePlayer.hash64);
    if (!obj) {
        state.pushInt(0);
        return;
    }
    state.activeObj = obj;
    state.pointerAdd(ActiveObj[state.intOperand]);
    state.pushInt(1);
},

[ScriptOpcode.OBJ_FINDALLZONE]: state => {
    const coord: CoordGrid = check(state.popInt(), CoordValid);
    state.objIterator = new ObjIterator(World.currentTick, coord.level, coord.x, coord.z);
},

[ScriptOpcode.OBJ_FINDNEXT]: state => {
    const result = state.objIterator?.next();
    if (!result || result.done) {
        state.pushInt(0);
        return;
    }
    state.activeObj = result.value;
    state.pointerAdd(ActiveObj[state.intOperand]);
    state.pushInt(1);
}
```

### §2.2 `Engine-TS/src/engine/script/ScriptIterators.ts:387-407` (ObjIterator)

```typescript
export class ObjIterator extends ScriptIterator<Obj> {
    private readonly level: number;
    private readonly x: number;
    private readonly z: number;

    constructor(tick: number, level: number, x: number, z: number) {
        super(tick);
        this.level = level;
        this.x = x;
        this.z = z;
    }

    protected *generator(): IterableIterator<Obj> {
        for (const Obj of World.gameMap.getZone(this.x, this.z, this.level).getAllObjsSafe(true)) {
            if (World.currentTick > this.tick) {
                throw new Error('[ObjIterator] tried to use an old iterator. Create a new iterator instead.');
            }
            yield Obj;
        }
    }
}
```

Single-zone, no filter, stale-check on each yield. Structurally identical to `LocIterator` at `ScriptIterators.ts:365-385`.

### §2.3 TS `World.getObj` (per NAI-153 surface)

TS `World.getObj(x, z, level, type, hash64)` returns the first ground `Obj` at the exact tile whose `type === objId` and which is visible to the caller (private-receiver gate). Goscape's `Server.GetObj(level, x, z, objId, receiverID)` already implements this contract at `modules/world/obj_lookup.go:13` — used at three sites in `modules/world/handler_opobj.go`.

---

## §3. Goscape mapping

### §3.1 Existing infra (verified at HEAD `a47e9cf`)

- **`pkg/script/active.go:910-915`** — `ActiveObj` interface with `ObjType() int`, `Coords()`, `ObjCount()`, `IsValidFor(playerUID int)`. No extension needed for NAI-154.
- **`pkg/script/state.go:306`** — `ActiveObj ActiveObj` field exists; `OtherActiveObj` does NOT.
- **`pkg/script/pointer.go:14-15`** — `PtrActiveObj` and `PtrActiveObj2` flags both exist; `PtrActiveObj2` is currently unused.
- **`pkg/script/opcode.go:313-317`** — `OpObjFind`, `OpObjFindAllZone`, `OpObjFindNext`, `OpObjName`, `OpObjParam` constants all declared.
- **`pkg/script/loc_iterator.go`** — full reference template: `LocIterator` struct, `NewZoneLocIterator(ops, tick, level, x, z)`, `Stale(currentTick)`, `Next() (ActiveLoc, bool)`. NAI-119.
- **`pkg/script/handlers_loc.go:20-39`** — `setActiveLocSlot` helper template. NAI-119.
- **`pkg/script/handlers_loc.go` (LOC_FINDALLZONE / LOC_FINDNEXT handlers)** — full reference for the two iterator handlers.
- **`pkg/script/handlers_config.go:425-460`** — `handleOcName` and `handleOcParam` reference templates (the only difference: OBJ_NAME/OBJ_PARAM read the type id from `s.ActiveObj.ObjType()` instead of popping it).
- **`pkg/script/handlers_config.go:9-58`** — `paramLookup(s, params, paramID)` shared path; OBJ_PARAM delegates to it.
- **`modules/world/obj_lookup.go:13`** — `Server.GetObj(level, x, z, objId, receiverID int) *entity.Obj` already exists and is consumed by `modules/world/handler_opobj.go:56,149,226`. Drop-in source for the WorldVars seam.
- **`pkg/zone.Zone.Objs []*entity.Obj`** at `pkg/zone/zone.go:43` — direct slice the ObjIterator's snapshot reads from.
- **`pkg/script/state.go:54-142`** — `WorldVars` interface; OBJ-related methods (`RemoveObj`, `AddObj`, `EnqueueObjDelayed`) already live here. `GetObj` and `ZoneObjs` extensions go here.

### §3.2 What's missing

1. `ObjIterator` type (no analogue exists for Obj). New file `pkg/script/obj_iterator.go`.
2. `OtherActiveObj ActiveObj` field on `ScriptState`.
3. `objIterator *ObjIterator` field on `ScriptState`.
4. `setActiveObjSlot` helper in `handlers_obj.go`.
5. `WorldVars.GetObj(level, x, z, objId, receiverUID int) ActiveObj` interface method.
6. `WorldVars.ZoneObjs(level, zoneX, zoneZ int) []ActiveObj` interface method.
7. `worldVarsView.GetObj` impl delegating to `Server.GetObj`.
8. `worldVarsView.ZoneObjs` impl reading `s.zoneMap.Get(level, x, z).Objs`.
9. Five handler functions (`handleObjFind`, `handleObjFindAllZone`, `handleObjFindNext`, `handleObjName`, `handleObjParam`) in `handlers_obj.go`.
10. Five dispatch entries in `handlers.go`.
11. Tests: iterator-level (new `obj_iterator_test.go`) + handler-level (`handlers_obj_test.go` additions).

### §3.3 Receiver convention (NAI-153-D2 inheritance)

NAI-153 chose `s.Self.UID()` (goscape player UID) over TS's `state.activePlayer.hash64` for `ActiveObj.IsValidFor`, tracked as `NAI-153-D2`. NAI-154's `OBJ_FIND` inherits this convention: `WorldVars.GetObj`'s `receiverUID` argument is `s.Self.UID()` at the handler call site. No new deviation surface; documented inline in the handler with a `(NAI-153-D2 inheritance)` doc-comment.

---

## §4. Architecture

### §4.1 `pkg/script/obj_iterator.go` (new file)

Structurally identical to `pkg/script/loc_iterator.go` — substitute `Loc`→`Obj`, `LocOps.AllLocsInZone`→`WorldVars.ZoneObjs`, `ActiveLoc`→`ActiveObj`. Difference from NAI-119: snapshot source is `WorldVars` (not a dedicated adapter), consistent with the existing OBJ-on-WorldVars convention.

```go
package script

// ObjIterator is the script-VM iterator state for the OBJ_FIND iterator
// family (currently OBJ_FINDALLZONE only — single-mode like LocIterator,
// unlike NpcIterator's DISTANCE/ZONE/HuntAll). Mirrors TS ObjIterator at
// ScriptIterators.ts:387-407.
//
// Lifetime: single-tick. Created by OBJ_FINDALLZONE; consumed by
// OBJ_FINDNEXT. Stale() check at FINDNEXT compares creationTick to
// World.CurrentTick(); on mismatch, handler returns an error mirroring
// the LOC family pattern (handlers_loc.go).
//
// Snapshot strategy: lazy on first Next() call via
// WorldVars.ZoneObjs(level, x, z). TS uses a generator over
// `getZone(...).getAllObjsSafe(true)` — equivalent because both produce a
// single point-in-time slice that the iterator drains independent of
// subsequent zone mutation.
//
// Ownership: held by ScriptState.objIterator. Nil = no active iterator.
type ObjIterator struct {
    creationTick int
    world        WorldVars
    level, x, z  int
    objs         []ActiveObj
    idx          int
    started      bool
}

// NewZoneObjIterator constructs a single-zone iterator for the zone
// containing (level, x, z). Snapshot deferred to first Next().
func NewZoneObjIterator(world WorldVars, tick, level, x, z int) *ObjIterator {
    return &ObjIterator{
        creationTick: tick,
        world:        world,
        level:        level,
        x:            x,
        z:            z,
    }
}

// Stale reports whether the iterator was created in a prior tick. The
// FINDNEXT handler MUST check this before calling Next. Mirrors TS
// strict-greater-than at ScriptIterators.ts:401
// (World.currentTick > this.tick).
func (it *ObjIterator) Stale(currentTick int) bool {
    return currentTick > it.creationTick
}

// Next returns the next obj in the zone snapshot, or (nil, false) on
// exhaustion. Lazy-initializes the snapshot on first call.
//
// Nil-world degrades to immediate exhaustion (test stub or pre-wiring).
func (it *ObjIterator) Next() (ActiveObj, bool) {
    if !it.started {
        it.started = true
        if it.world != nil {
            it.objs = it.world.ZoneObjs(it.level, it.x, it.z)
        }
    }
    if it.idx >= len(it.objs) {
        return nil, false
    }
    obj := it.objs[it.idx]
    it.idx++
    return obj, true
}
```

### §4.2 `pkg/script/state.go` — field adds

Add immediately after `ActiveObj ActiveObj` (line 306):

```go
// OtherActiveObj is the secondary Obj slot, parallel to OtherActiveLoc
// (NAI-119) and OtherActiveNpc (NAI-11). Set by OBJ_FIND / OBJ_FINDNEXT
// when the bytecode IntOperand is 1 (.obj2 syntax). NAI-154.
//
// NAI-154-D-NO-DOWNSTREAM-OBJ2-CONSUMERS: no existing OBJ_* read handler
// reads from this slot at HEAD — they all read s.ActiveObj only. Tracked
// deviation, mirrors NAI-119-D-NO-DOWNSTREAM-LOC2-CONSUMERS. Closure
// when a `.obj2` content-script consumer surfaces.
OtherActiveObj ActiveObj
```

Add adjacent to `locIterator` (state.go around line 324):

```go
// objIterator holds the active OBJ_FIND iterator state. Set by
// OBJ_FINDALLZONE; consumed by OBJ_FINDNEXT. Lifetime is single-tick —
// Stale() check enforced at FINDNEXT against s.World.CurrentTick().
// Nil = no active iterator. Mirrors TS ScriptState.objIterator. NAI-154.
objIterator *ObjIterator
```

Add to `WorldVars` interface (state.go after `EnqueueObjDelayed` around line 128):

```go
// GetObj returns the first ground obj at (level, x, z) whose type
// matches objId and is visible to the caller. receiverUID is the
// player UID gating private-receiver visibility (see NAI-153-D2 —
// goscape uses player UID where TS uses hash64). Returns nil on miss.
// Mirrors TS World.getObj at ServerOps.ts (consumed via OBJ_FIND).
// NAI-154.
GetObj(level, x, z, objId, receiverUID int) ActiveObj

// ZoneObjs returns every obj in the zone owning (level, zoneX, zoneZ),
// in storage order, without per-tile or per-receiver filtering. The
// caller (OBJ_FINDNEXT) applies its own validity gates as needed.
// Mirrors TS Zone.getAllObjsSafe(true) consumed by ObjIterator.generator
// (ScriptIterators.ts:400). Empty/nil slice on miss. NAI-154.
ZoneObjs(level, zoneX, zoneZ int) []ActiveObj
```

### §4.3 `modules/world/world.go` — `worldVarsView` impl

Add two methods on the existing `worldVarsView` adapter (the production-side `WorldVars` impl). Both delegate to existing infra.

```go
// GetObj delegates to Server.GetObj at modules/world/obj_lookup.go:13.
// Returns script.ActiveObj (via *entity.Obj which implements the
// interface from NAI-115).
func (v worldVarsView) GetObj(level, x, z, objId, receiverUID int) script.ActiveObj {
    o := v.s.GetObj(level, x, z, objId, receiverUID)
    if o == nil {
        return nil
    }
    return o
}

// ZoneObjs reads the zone's Objs slice directly via zoneMap.Get and
// adapts each *entity.Obj to script.ActiveObj. Mirrors serverLocOps.AllLocsInZone
// at modules/world/script_loc_ops.go:85-92.
func (v worldVarsView) ZoneObjs(level, zoneX, zoneZ int) []script.ActiveObj {
    z := v.s.zoneMap.Get(level, zoneX, zoneZ)
    if z == nil {
        return nil
    }
    out := make([]script.ActiveObj, 0, len(z.Objs))
    for _, o := range z.Objs {
        out = append(out, o)
    }
    return out
}
```

**Verification at plan-author time:** confirm (a) `v.s` field name on `worldVarsView` (or the actual receiver path to the Server), and (b) `*entity.Obj` already implements `script.ActiveObj` (it does as of NAI-153 — `pkg/entity/obj.go` has `ObjType()`, `Coords()`, `ObjCount()`, `IsValidFor(uid)`).

### §4.4 `pkg/script/handlers_obj.go` — helper

Add near the top of the file (after `requireActiveObj` at line 12):

```go
// setActiveObjSlot writes the obj to either ActiveObj (primary) or
// OtherActiveObj (secondary) based on the handler's IntOperand and sets
// the corresponding Pointer flag. Mirrors TS
// state.pointerAdd(ActiveObj[state.intOperand]) at ObjOps.ts:91, 181,
// 199, and the parallel setActiveLocSlot at handlers_loc.go:29-40.
//
// IntOperand==0 → ActiveObj/PtrActiveObj (.obj syntax).
// IntOperand==1 → OtherActiveObj/PtrActiveObj2 (.obj2 syntax).
// Any other value panics (compiler invariant — bytecode only emits 0/1).
func setActiveObjSlot(s *ScriptState, obj ActiveObj) {
    operand := s.Script.IntOperands[s.PC]
    switch operand {
    case 0:
        s.ActiveObj = obj
        s.Pointers |= PtrActiveObj
    case 1:
        s.OtherActiveObj = obj
        s.Pointers |= PtrActiveObj2
    default:
        panic(fmt.Sprintf("setActiveObjSlot: invalid IntOperand %d", operand))
    }
}
```

### §4.5 `pkg/script/handlers_obj.go` — five new handlers

```go
// handleObjFind (OBJ_FIND, opcode 3505) pops [coord, objId], resolves
// the obj via WorldVars.GetObj, and either slot-routes it via
// setActiveObjSlot + pushes 1 on hit, or pushes 0 on miss. Mirrors TS
// ObjOps.ts:168-183.
//
// Pop order: objId is at the top of the stack (last pushed); coord
// below it. Matches TS `[coord, objId] = state.popInts(2)` semantics
// (popInts(N) returns top-N in pushed order; objId was pushed last).
//
// Receiver UID is s.Self.UID() per NAI-153-D2 (goscape UID vs TS hash64).
// Requires ActivePlayer pointer per TS (state.activePlayer.hash64
// implies an active player); use requireActivePlayer for the gate.
func handleObjFind(s *ScriptState) error {
    if err := requireActivePlayer(s, "OBJ_FIND"); err != nil {
        return err
    }
    objId := s.PopInt()
    coord := s.PopInt()
    level, x, z, err := checkCoord(coord, "OBJ_FIND")
    if err != nil {
        return err
    }
    // TS also `check(objId, ObjTypeValid)` — preserved via Configs gate.
    if err := requireConfigs(s, "OBJ_FIND"); err != nil {
        return err
    }
    if s.Configs.ObjType(objId) == nil {
        return fmt.Errorf("OBJ_FIND: unknown obj id %d", objId)
    }
    if s.World == nil {
        s.PushInt(0)
        return nil
    }
    obj := s.World.GetObj(level, x, z, objId, s.Self.UID())
    if obj == nil {
        s.PushInt(0)
        return nil
    }
    setActiveObjSlot(s, obj)
    s.PushInt(1)
    return nil
}

// handleObjFindAllZone (OBJ_FINDALLZONE, opcode 3506) pops a coord and
// stores a single-zone ObjIterator targeting the zone containing that
// coord. Mirrors TS ObjOps.ts:185-189.
//
// Nil-World degrades silently (matches LOC_FINDALLZONE convention at
// handlers_loc.go).
func handleObjFindAllZone(s *ScriptState) error {
    coord := s.PopInt()
    level, x, z, err := checkCoord(coord, "OBJ_FINDALLZONE")
    if err != nil {
        return err
    }
    if s.World == nil {
        return nil
    }
    s.objIterator = NewZoneObjIterator(s.World, s.World.CurrentTick(), level, x, z)
    return nil
}

// handleObjFindNext (OBJ_FINDNEXT, opcode 3507) advances the active
// ObjIterator and either sets the active obj slot + pushes 1 on hit, or
// pushes 0 on miss / nil-iterator. Mirrors TS ObjOps.ts:191-201.
//
// Stale-iterator semantics mirror LOC_FINDNEXT — return error on stale;
// runtime path clears the active script.
//
// Pointer-set: setActiveObjSlot threads IntOperand 0/1 per TS
// state.pointerAdd(ActiveObj[intOperand]).
func handleObjFindNext(s *ScriptState) error {
    it := s.objIterator
    if it == nil {
        s.PushInt(0)
        return nil
    }
    if it.Stale(s.World.CurrentTick()) {
        return fmt.Errorf("OBJ_FINDNEXT: tried to use an old iterator. Create a new iterator instead.")
    }
    obj, ok := it.Next()
    if !ok {
        s.PushInt(0)
        return nil
    }
    setActiveObjSlot(s, obj)
    s.PushInt(1)
    return nil
}

// handleObjName (OBJ_NAME, opcode 3508) pushes the active obj's name
// (or debugname fallback; "null" when both are empty). Mirrors TS
// ObjOps.ts:106-110 and the existing handleOcName at handlers_config.go:429-446.
func handleObjName(s *ScriptState) error {
    if err := requireActiveObj(s, "OBJ_NAME"); err != nil {
        return err
    }
    if err := requireConfigs(s, "OBJ_NAME"); err != nil {
        return err
    }
    ot := s.Configs.ObjType(s.ActiveObj.ObjType())
    if ot == nil {
        return fmt.Errorf("OBJ_NAME: unknown obj id %d", s.ActiveObj.ObjType())
    }
    if ot.Name != "" {
        s.PushString(ot.Name)
    } else if ot.DebugName != "" {
        s.PushString(ot.DebugName)
    } else {
        s.PushString("null")
    }
    return nil
}

// handleObjParam (OBJ_PARAM, opcode 3509) pops a paramID and delegates
// to paramLookup using the active obj's type Params. Mirrors TS
// ObjOps.ts:95-104 and the existing handleOcParam at handlers_config.go:448-460.
func handleObjParam(s *ScriptState) error {
    if err := requireActiveObj(s, "OBJ_PARAM"); err != nil {
        return err
    }
    if err := requireConfigs(s, "OBJ_PARAM"); err != nil {
        return err
    }
    paramID := s.PopInt()
    ot := s.Configs.ObjType(s.ActiveObj.ObjType())
    if ot == nil {
        return fmt.Errorf("OBJ_PARAM: unknown obj id %d", s.ActiveObj.ObjType())
    }
    return paramLookup(s, ot.Params, paramID)
}
```

### §4.6 `pkg/script/handlers.go` — dispatch entries

Add to the existing handler map adjacent to the other `OpObj*` entries (currently `OpObjType`, `OpObjCount`, `OpObjTakeItem` from NAI-152/153, plus the older `OpObjAdd`/`OpObjDel`/`OpObjCoord`):

```go
OpObjFind:        handleObjFind,
OpObjFindAllZone: handleObjFindAllZone,
OpObjFindNext:    handleObjFindNext,
OpObjName:        handleObjName,
OpObjParam:       handleObjParam,
```

Plan-author: confirm dispatch-map insertion site and exact line at plan-write per `controller_preflight`; the existing map is in handlers.go.

---

## §5. Tests

### §5.1 `pkg/script/obj_iterator_test.go` (new file)

Mirror NAI-119's `loc_iterator_test.go` exactly — substitute Loc→Obj, locOps→worldVars, ActiveLoc→ActiveObj. Stub `WorldVars` via a minimal `fakeWorldVars` that implements only `ZoneObjs` (and embeds a no-op for the other methods, or use a test-helper composite).

| Test | Pin |
|---|---|
| `TestObjIteratorStaleAtSameTick` | Stale returns false when currentTick == creationTick (TS strict `>`). |
| `TestObjIteratorStaleNextTick` | Stale returns true when currentTick > creationTick. |
| `TestObjIteratorYieldsAllZoneObjs` | fakeWorldVars seeded with 3 objs in target zone; Next returns all 3 in slice order, then (nil, false). |
| `TestObjIteratorEmptyZone` | fakeWorldVars returns empty slice; first Next returns (nil, false). |
| `TestObjIteratorExhaustionDoesNotClear` | After exhaustion, repeat Next() calls keep returning (nil, false) without panic. |
| `TestObjIteratorNilWorldDegrades` | world=nil; first Next returns (nil, false). |
| `TestNewZoneObjIteratorStoresFields` | Constructor pins level/x/z/creationTick exactly. |

### §5.2 `pkg/script/handlers_obj_test.go` additions

| Test | Pin |
|---|---|
| `TestObjFindHitPrimarySlot` | fakeWorldVars returns mockActiveObj; IntOperand=0; OBJ_FIND pushes 1, sets `s.ActiveObj`, sets `PtrActiveObj`. |
| `TestObjFindHitSecondarySlot` | Same but `IntOperands[PC]=1`; OBJ_FIND sets `s.OtherActiveObj`, sets `PtrActiveObj2`. Pins dual-slot. |
| `TestObjFindMissPushesZero` | fakeWorldVars returns nil; OBJ_FIND pushes 0; does NOT touch ActiveObj. |
| `TestObjFindRequiresActivePlayer` | No Self pointer; OBJ_FIND returns "requires active player" error. |
| `TestObjFindUnknownObjId` | Configs returns nil for objId; OBJ_FIND returns "unknown obj id" error. |
| `TestObjFindInvalidCoord` | coord=-1; OBJ_FIND returns checkCoord error. |
| `TestObjFindUIDPropagation` | Asserts fakeWorldVars.GetObj receives `s.Self.UID()` as receiverUID (NAI-153-D2 pin). |
| `TestObjFindAllZoneStoresIterator` | Run `[push coord; OBJ_FINDALLZONE]`; assert `s.objIterator != nil`, `creationTick == World.CurrentTick()`, level/x/z match popped coord. |
| `TestObjFindAllZoneNilWorldDegrades` | s.World=nil; no panic, s.objIterator stays nil. |
| `TestObjFindAllZoneCoordValid` | coord=-1; handler returns checkCoord error. |
| `TestObjFindNextNoIterator` | s.objIterator=nil; FINDNEXT pushes 0; no error. |
| `TestObjFindNextHitPrimarySlot` | fakeWorldVars with one obj; iterator installed; IntOperand=0; FINDNEXT pushes 1, sets s.ActiveObj, sets PtrActiveObj. |
| `TestObjFindNextHitSecondarySlot` | Same but IntOperand=1; FINDNEXT sets s.OtherActiveObj, sets PtrActiveObj2. |
| `TestObjFindNextExhaustionPushesZero` | Iterator drained; FINDNEXT pushes 0, does NOT touch ActiveObj/OtherActiveObj. |
| `TestObjFindNextStaleErrors` | Iterator created at tick=0; World.CurrentTick advanced to 1; FINDNEXT returns "tried to use an old iterator" error. |
| `TestObjNameNamePresent` | ObjType.Name="rune sword"; OBJ_NAME pushes "rune sword". |
| `TestObjNameDebugFallback` | ObjType.Name=""; DebugName="sword_t1"; OBJ_NAME pushes "sword_t1". |
| `TestObjNameNullFallback` | Both empty; OBJ_NAME pushes "null". |
| `TestObjNameRequiresActiveObj` | s.ActiveObj=nil; OBJ_NAME returns requireActiveObj error. |
| `TestObjNameRequiresConfigs` | s.Configs=nil; OBJ_NAME returns requireConfigs error. |
| `TestObjNameUnknownType` | Configs.ObjType returns nil; OBJ_NAME returns "unknown obj id" error. |
| `TestObjParamIntBranch` | paramID resolves to int param with value=42; OBJ_PARAM pushes 42. |
| `TestObjParamStringBranch` | paramID resolves to string param with value="hello"; OBJ_PARAM pushes "hello". |
| `TestObjParamIntDefaultFallback` | obj.Params lacks paramID; ParamType.DefaultInt=-7; OBJ_PARAM pushes -7 (sign-extension preserved per NAI-125). |
| `TestObjParamStringDefaultFallback` | obj.Params lacks paramID; ParamType.DefaultString="def"; OBJ_PARAM pushes "def". |
| `TestObjParamRequiresActiveObj` | s.ActiveObj=nil; OBJ_PARAM returns requireActiveObj error. |
| `TestObjParamRequiresConfigs` | s.Configs=nil; OBJ_PARAM returns requireConfigs error. |
| `TestObjParamUnknownType` | Configs.ObjType returns nil; OBJ_PARAM returns "unknown obj id" error. |

Test fixtures: extend the existing `mockActiveObj` at `pkg/script/handlers_npc_test.go:2429+` if needed (already has `ObjType()`, `Coords()`, `ObjCount()`, `IsValidFor`). For OBJ_FIND/OBJ_FINDNEXT slot tests, reuse the `newObjTestState` builder pattern from NAI-153 if present, else inline-construct.

For `fakeWorldVars`, the iterator tests need only `ZoneObjs`. The handler tests for OBJ_FIND need both `ZoneObjs` and `GetObj` plus `CurrentTick` (for the iterator-installation tests). Cribbed from existing test scaffolding patterns — check `pkg/script/state_test.go` / `handlers_obj_test.go` for the existing fake-world test stub.

### §5.3 Test-coverage crosscheck (per `plan_test_coverage_crosscheck`)

Each T-task in the plan produces production code in tight one-to-one mapping with the test rows above. Plan-author must mentally execute each test fixture's setup against the production code shapes — especially the slot-routing tests (need `s.Script.IntOperands[s.PC]` set correctly in fixture) and the requireConfigs error tests (need Configs-nil setter on the test state builder).

---

## §6. Deviations

- **`NAI-154-D-NO-DOWNSTREAM-OBJ2-CONSUMERS`** — `OtherActiveObj` field added but no OBJ_* read handler at HEAD reads from it (OBJ_NAME / OBJ_PARAM / OBJ_COUNT / OBJ_TAKEITEM / OBJ_TYPE / OBJ_COORD / OBJ_DEL all read `s.ActiveObj` only). Mirrors NAI-119-D-NO-DOWNSTREAM-LOC2-CONSUMERS exactly. Tracked; closure when a `.obj2` content-script consumer surfaces.

**Inherited deviations (no new touch):**
- **NAI-153-D2** — receiverID convention (`s.Self.UID()` vs TS `hash64`). NAI-154 OBJ_FIND uses the same convention; documented inline.
- **NAI-115-D2** — `WorldVars.RemoveObj` no-duration. Not touched by NAI-154 (no OBJ_FIND family call site removes objs).
- **NAI-153-D1** — `wealthEvent` ledger gap. Not touched (no OBJ_FIND family call site emits wealth events).

**Iteration-order parity:** TS uses `getAllObjsSafe(true)` (reverse storage order); goscape `worldVarsView.ZoneObjs` returns forward storage order (matching NAI-119 `serverLocOps.AllLocsInZone` precedent). Not tracked as a new deviation — the LOC counterpart has been smoke-clean. If a content script that relies on reverse-order first-match surfaces a divergence in smoke, route forward as a separate sub-spec covering both LOC and OBJ iterators.

---

## §7. Verification

1. New test file (`obj_iterator_test.go`) and additions (`handlers_obj_test.go`) fail at HEAD `a47e9cf`.
2. Apply §4 production diffs.
3. New tests pass.
4. `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...` clean (no pre-existing failures introduced).
5. `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go vet ./...` clean.
6. Modern-Go style applied (per `use-modern-go` skill).

---

## §8. Smoke (user-launched, post-merge)

Per `smoke_test_server_handoff`: ask the user to launch the goscape server + Java client and verify no `OBJ_FIND` / `OBJ_FINDALLZONE` / `OBJ_FINDNEXT` / `OBJ_NAME` / `OBJ_PARAM` `no handler` WARN logs appear during typical play.

**Likely content path consumers** (plan-author: grep `LostCityRS/Content/scripts/**/*.rs2` for `obj_find`, `obj_findallzone`, `obj_findnext`, `obj_name`, `obj_param` at plan-write to enumerate concrete smoke targets):

- Tutorial Island pickup chains (already partially exercised by NAI-152/153 mindrune smoke).
- Combat-area ground-loot pickup procs.
- Any content script that iterates a tile or zone for ground items.

**PRIMARY bind:** No `no handler for OBJ_FIND|OBJ_FINDALLZONE|OBJ_FINDNEXT|OBJ_NAME|OBJ_PARAM` errors in server log during a normal play session.

**Secondary check:** NAI-152/153 mindrune pickup smoke unchanged (OBJ_COUNT/OBJ_TAKEITEM still bind cleanly; the new `OtherActiveObj` field doesn't perturb the single-slot existing reads).

**Cascade-tail update:** post-merge, re-run `missing_handler_audit.md`; expected count 34 → 29.

---

## §9. Out of scope

- **OBJ_* read-handler parameterization over slot.** Currently every OBJ read reads `s.ActiveObj`; making them honor `IntOperand` to route primary/secondary is the closure for `NAI-154-D-NO-DOWNSTREAM-OBJ2-CONSUMERS`. Deferred until an `.obj2` consumer surfaces (mirrors NAI-119 LOC parallel).
- **HuntAll/Distance modes for OBJ.** TS ObjIterator is single-zone only; no scope expansion needed.
- **Reverse-iteration parity.** Goscape iterators (LOC + OBJ) use forward storage order; TS uses reverse. Tracked at §6 if smoke surfaces a content-script divergence.
- **OBJ_SETVAR / OBJ_ADDDELAYED.** TS comments at `ObjOps.ts:203-204` mark these as future; goscape doesn't have opcode constants for them at HEAD; out of scope.
- **The remaining 29 undispatched opcodes** post-NAI-154 (varbit ops, NPC family, Player ops, INV family, LC/OC ops, LOS/Map). Continue cascade-tail in NAI-155+ per `runescript_cadence`.

---

## §10. Plan cadence

Cadence B (per `compressed_cadence` / `runescript_cadence` mid-band) — separate spec + plan; combined Sonnet reviewer at end (NOT per-task two-stage). Implementer subagents on Sonnet per `superpowers_code_reviewer_model`; reviewer also Sonnet.

The plan will lay out **5 sequential T-tasks**:

- **T1 — ScriptState surface + helper.** Field adds (`OtherActiveObj`, `objIterator`) in `pkg/script/state.go` + WorldVars method declarations (`GetObj`, `ZoneObjs`) + `setActiveObjSlot` helper in `pkg/script/handlers_obj.go`. No new tests yet — these surface changes are exercised by T2+. (Build-only; existing fake-world stubs in tests need parallel additions OR the WorldVars stub pattern needs an embeddable default — plan-author resolves at write.)

- **T2 — ObjIterator + WorldVars production impl.** New file `pkg/script/obj_iterator.go` per §4.1. New file `pkg/script/obj_iterator_test.go` per §5.1. `modules/world/world.go` (or equivalent location) `worldVarsView.GetObj` + `ZoneObjs` impls per §4.3. TDD red→green→commit.

- **T3 — OBJ_FIND handler + dispatch.** `handleObjFind` per §4.5 + dispatch entry per §4.6 + 7 OBJ_FIND tests per §5.2.

- **T4 — OBJ_FINDALLZONE + OBJ_FINDNEXT handlers + dispatch.** Both handlers per §4.5 + two dispatch entries per §4.6 + 8 iterator-handler tests per §5.2.

- **T5 — OBJ_NAME + OBJ_PARAM handlers + dispatch.** Both handlers per §4.5 + two dispatch entries per §4.6 + 12 OBJ_NAME/OBJ_PARAM tests per §5.2. Final commit triggers combined reviewer.

After review approves: handoff to user for §8 smoke. On smoke bind: close commit per `close_commit_memory_trailer` with `Closes memory:` trailer referencing this spec.

---

## §11. Pattern memories applied

- `iterator_state_pattern` — single-tick iterator template (custom struct + state field + Lookup-style snapshot + Stale check), cloned from NAI-119 LocIterator.
- `controller_preflight` — Pre-flight verified all anchor lines/symbols at HEAD `a47e9cf` (PtrActiveObj2 existence, ActiveObj interface shape, opcode constants 3505-3509, Server.GetObj signature, Zone.Objs field, LocIterator template, handleOcName/handleOcParam templates).
- `plan_helper_coverage` — `setActiveObjSlot` mirrors `setActiveLocSlot` exactly (same IntOperand 0/1 + same panic-on-other-operand invariant).
- `audit_full_method_against_ts` — TS ObjIterator + 5 handlers all read end-to-end from primary sources (ScriptIterators.ts:387-407, ObjOps.ts:95-201).
- `parallel_spatial_index_migration_pattern` — no migration in scope here; the new WorldVars.GetObj/ZoneObjs are pure read seams over existing Server.GetObj + Zone.Objs.
- `defensive_gate_doc_comment_label` — `s.World == nil` and `s.Configs == nil` guards labeled as goscape defensive checks.
- `compressed_cadence` — Cadence B (5 sequential tasks, single combined review).
- `smoke_test_server_handoff` — user-launched smoke binds.
- `close_commit_memory_trailer` — apply on close commit.
- `plan_test_coverage_crosscheck` — §5.3 crosscheck.
- `plan_grep_helper_patterns` — handlers reuse existing `requireActiveObj`, `requireActivePlayer`, `requireConfigs`, `paramLookup`, `checkCoord`.
- `flat_arg_signature_for_cross_lang_parity` — `WorldVars.GetObj(level, x, z, objId, receiverUID int)` kept as flat positional sig matching `Server.GetObj` and TS `World.getObj(x, z, level, type, hash64)`.
