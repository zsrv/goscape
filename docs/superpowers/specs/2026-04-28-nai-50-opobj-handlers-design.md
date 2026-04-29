# NAI-50: OPOBJ1-5 + OPOBJT + OPOBJU handlers + trigger dispatch

**Date:** 2026-04-28
**Status:** Spec — pending plan

## 1. Objective

Port the seven client-game OPOBJ packet handlers (OPOBJ1-5, OPOBJT, OPOBJU) and their
server-side trigger dispatch (fireOpTriggerObj, fireApTriggerObj). After this sub-spec,
all five entity-target op families (Npc, Loc, Player, Obj, Held) have full OPXXX + T + U
coverage. Clicking a ground item will route to [opobj<N>,<objType>] / [apobjN>,<objType>]
/ [apobjt,<spellCom>] / [apobju,<objType>] trigger scripts.

## 2. Background

OPOBJ1-5, OPOBJT, OPOBJU are declared in `pkg/io/protocol/game/client/prot.go:52-58`
but are not registered in `modules/world/handlers_game.go`. Clicking any ground item
silently discards the packet. All 14 trigger constants (TriggerApObj1-5/T/U,
TriggerOpObj1-5/T/U) already exist at `pkg/script/trigger.go:38-51`. `objStillValid`
already exists at `modules/world/interaction_trigger.go:236-243`. The AP/OP dispatch
switch-arms in `tryFireApTrigger` and `tryFireOpTrigger` fall through to a default that
explicitly marks this sub-spec as the follow-up (interaction_trigger.go:269, :47).

**TS reference:** `src/network/game/client/handler/OpObjHandler.ts`,
`OpObjTHandler.ts`, `OpObjUHandler.ts`, and `src/engine/entity/Player.ts:966-1031`
(getOpTrigger / getApTrigger dispatch).

## 3. Tech Stack

Go 1.26+. No new dependencies.

## 4. Scope

### In scope

- `Server.GetObj` lookup helper (new `obj_lookup.go`)
- `handleOpObj` shared + 5 shims + `handleOpObjT` + `handleOpObjU` (new `handler_opobj.go`)
- 7 `gameHandlers[N]` registrations in `handlers_game.go`
- `targetOpObjT = 12`, `targetOpObjU = 13` sentinel consts in `interaction.go`
- `fireOpTriggerObj`, `fireApTriggerObj`, `apObjTriggerForOp` + `*entitypkg.Obj` cases
  wired in both `tryFireOpTrigger` and `tryFireApTrigger` in `interaction_trigger.go`
- Unit tests for all of the above

### Out of scope

- Component registry validation (actionTarget, usable, isComponentVisible) — deferred
  to the component-registry sub-spec; tracked as NAI-50-D1 / NAI-50-D2
- `reachedObj` rsmod collision shape logic — pre-existing S6l-D4 deviation (Chebyshev
  used for all target types)

## 5. Files

| File | Change |
|---|---|
| `modules/world/obj_lookup.go` | new |
| `modules/world/obj_lookup_test.go` | new |
| `modules/world/handler_opobj.go` | new |
| `modules/world/handler_opobj_test.go` | new |
| `modules/world/handlers_game.go` | +7 registrations |
| `modules/world/interaction.go` | +2 sentinel consts |
| `modules/world/interaction_trigger.go` | +2 fire helpers + 2 case arms |
| `pkg/objtype/objtype.go` | +`"hidden"` → `""` coercion in Op decoder (cases 30-34) |

## 6. Design

### 6.1 Server.GetObj

File: `modules/world/obj_lookup.go`

```go
// GetObj returns the obj at (level, x, z) whose type matches objId and is
// visible to receiverID, or nil. Mirrors TS World.getObj / Zone.getObj
// (Zone.ts:353-360): matches public objs (ReceiverID == -1) OR objs
// privately owned by this player (ReceiverID == receiverID).
// Callers pass p.slot as receiverID.
func (s *Server) GetObj(level, x, z, objId, receiverID int) *entitypkg.Obj {
    zn := s.zoneMap.Get(level, x, z)
    for _, o := range zn.Objs {
        if o.X == x && o.Z == z && o.Type == objId &&
            (o.ReceiverID == zone.PublicReceiver || o.ReceiverID == receiverID) {
            return o
        }
    }
    return nil
}
```

### 6.2 targetOp sentinel additions

File: `modules/world/interaction.go` (append after `targetOpPlayerU = 11`):

```go
targetOpObjT = 12 // APOBJT / OPOBJT dispatch marker
targetOpObjU = 13 // APOBJU / OPOBJU dispatch marker
```

### 6.3 handleOpObj (shared, OPOBJ1-5)

File: `modules/world/handler_opobj.go`. Payload: 6 bytes `(x G2, z G2, objId G2)`.

Validation gates (mirrors TS OpObjHandler.ts:14-42):
1. `p.client == nil || p.client.server == nil` → return nil
2. `p.delayed && s.currentTick < p.delayedUntil` → UnsetMapFlag
3. `len(payload) < 6` → UnsetMapFlag
4. Viewport gate: `|x - p.originX| > 52 || |z - p.originZ| > 52` → UnsetMapFlag
5. `s.GetObj(p.level, x, z, objId, p.slot) == nil` → UnsetMapFlag
6. `s.objTypes == nil || objId < 0 || objId >= len(s.objTypes.Configs)` → UnsetMapFlag
7. `s.objTypes.Configs[objId] == nil` → UnsetMapFlag
8. Per-op gate: `len(objType.Op) < op || objType.Op[op-1] == ""` → UnsetMapFlag
   (mirrors TS OpObjHandler.ts:38-42; `"hidden"` is coerced to `""` in the ObjType
   decoder at `pkg/objtype/objtype.go` cases 30-34, matching the LocType/NpcType
   decoder pattern — added as part of T2)

On success:
```go
p.ClearPendingAction()
p.opcalled = true
p.SetInteraction(InteractionEngine, obj, op, -1)
p.targetSubject.typ = obj.Type
p.targetSubject.x = obj.X
p.targetSubject.z = obj.Z
p.targetSubject.level = obj.Level
```

Shims:
```go
func handleOpObj1(p *Player, payload []byte) error { return handleOpObj(p, payload, 1) }
// … handleOpObj2..5
```

### 6.4 handleOpObjT (OPOBJT)

Payload: 8 bytes `(x G2, z G2, objId G2, spellCom G2)`.

Gates 1-7 from §6.3 (no per-op check). **DEVIATION NAI-50-D1** (see §8): spellCom
component actionTarget + visibility check skipped.

On success:
```go
p.ClearPendingAction()
p.opcalled = true
p.SetInteraction(InteractionEngine, obj, targetOpObjT, spellCom)
p.targetSubject.typ = obj.Type
p.targetSubject.x   = obj.X
p.targetSubject.z   = obj.Z
p.targetSubject.level = obj.Level
```

### 6.5 handleOpObjU (OPOBJU)

Payload: 12 bytes `(x G2, z G2, objId G2, useObj G2, useSlot G2, useCom G2)`.

Gates 1-7 from §6.3 (no per-op check). **DEVIATION NAI-50-D2** (see §8): useCom
component usable + visibility check skipped.

Additional gates (mirrors TS OpObjUHandler.ts:50-66):
- `p.invListeners[useCom]` missing → UnsetMapFlag
- resolved inv nil → UnsetMapFlag
- `!inv.HasAt(useSlot, useObj)` → UnsetMapFlag
- members-only item on free world → MessageGame + UnsetMapFlag

On success:
```go
p.lastUseItem = useObj
p.lastUseSlot = useSlot
p.ClearPendingAction()
p.opcalled = true
p.SetInteraction(InteractionEngine, obj, targetOpObjU, -1)
p.targetSubject.typ   = obj.Type
p.targetSubject.x     = obj.X
p.targetSubject.z     = obj.Z
p.targetSubject.level = obj.Level
```

### 6.6 handlers_game.go registrations

```go
gameHandlers[140] = handleOpObj1 // OPOBJ1
gameHandlers[40]  = handleOpObj2 // OPOBJ2
gameHandlers[200] = handleOpObj3 // OPOBJ3
gameHandlers[178] = handleOpObj4 // OPOBJ4
gameHandlers[247] = handleOpObj5 // OPOBJ5
gameHandlers[138] = handleOpObjT // OPOBJT
gameHandlers[239] = handleOpObjU // OPOBJU
```

Opcodes from `pkg/io/protocol/game/client/prot.go:52-58`.

### 6.7 apObjTriggerForOp

```go
// apObjTriggerForOp returns the APOBJ trigger for p.targetOp. Returns
// ok=false for unrecognised sentinels. fireOpTriggerObj derives OPOBJ
// by adding 7 (TS Player.ts:997 offset convention):
//   APOBJ1..5 (31..35) + 7 → OPOBJ1..5 (38..42)
//   APOBJT    (37)     + 7 → OPOBJT    (44)
//   APOBJU    (36)     + 7 → OPOBJU    (43)
func apObjTriggerForOp(op int) (script.ServerTriggerType, bool) {
    switch {
    case op >= 1 && op <= 5:
        return script.TriggerApObj1 + script.ServerTriggerType(op-1), true
    case op == targetOpObjT:
        return script.TriggerApObjT, true
    case op == targetOpObjU:
        return script.TriggerApObjU, true
    default:
        return 0, false
    }
}
```

### 6.8 fireOpTriggerObj

Mirrors `fireOpTriggerLoc` with three substitutions:
- Lifecycle gate: `objStillValid(srv, obj, p.targetSubject.x, p.targetSubject.z, p.targetSubject.level)`
- ScriptState: `state.ActiveObj = obj`, `state.Pointers |= script.PtrActiveObj`
- No-script path: `p.MessageGame("Nothing interesting happens.")` (TS Player.ts:1095)

ObjType category lookup (parallel to fireOpTriggerLoc):
```go
category := 0
if obj.Type >= 0 && obj.Type < len(srv.objTypes.Configs) {
    if ot := srv.objTypes.Configs[obj.Type]; ot != nil {
        category = ot.Category
    }
}
sf := srv.scriptProvider.GetByTrigger(trigger, obj.Type, category)
```

### 6.9 fireApTriggerObj

Mirrors `fireApTriggerLoc` with the same three substitutions as §6.8.
Full apRangeCalled persistence contract preserved (same as Loc).
When no AP script: `p.apRange = -1` sentinel (same as fireApTriggerLoc) rather than
"Nothing interesting happens." — OP trigger takes over on a later tick.

### 6.10 tryFireOpTrigger + tryFireApTrigger wiring

In `tryFireOpTrigger`, replace the default arm comment with:
```go
case *entitypkg.Obj:
    fireOpTriggerObj(p, srv, tgt)
```

In `tryFireApTrigger`, replace the default arm comment with:
```go
case *entitypkg.Obj:
    fireApTriggerObj(p, srv, tgt)
```

The default arms revert to the standard silent skip + `p.interactionFired = true`
for any remaining unhandled types.

## 7. Test Strategy

### obj_lookup_test.go

- Public obj at correct tile + type → returned
- Public obj at wrong tile → nil
- Public obj wrong type → nil
- Private obj (ReceiverID == slot) → returned for matching receiver
- Private obj (ReceiverID == slot) → nil for non-matching receiver
- Empty zone → nil

### handler_opobj_test.go (handleOpObj / OPOBJ1-5)

- Delayed player → UnsetMapFlag, no SetInteraction
- Payload too short → UnsetMapFlag
- Coords outside viewport (> 52 tiles) → UnsetMapFlag
- GetObj nil → UnsetMapFlag
- ObjType nil / out-of-range → UnsetMapFlag
- `objType.Op[op-1] == ""` (hidden slot) → UnsetMapFlag
- Success: opcalled=true, SetInteraction(op, -1), targetSubject snapshot
- Op variant: op=1 and op=5 produce distinct SetInteraction calls

### handler_opobj_test.go (handleOpObjT)

- Delayed → UnsetMapFlag
- Payload < 8 bytes → UnsetMapFlag
- Out-of-viewport → UnsetMapFlag
- GetObj nil → UnsetMapFlag
- Success: opcalled=true, SetInteraction(targetOpObjT, spellCom), snapshot

### handler_opobj_test.go (handleOpObjU)

- Delayed → UnsetMapFlag
- Payload < 12 bytes → UnsetMapFlag
- Out-of-viewport → UnsetMapFlag
- GetObj nil → UnsetMapFlag
- invListener missing → UnsetMapFlag
- Slot/item mismatch → UnsetMapFlag
- Members-only item on free world → MessageGame + UnsetMapFlag
- Success: lastUseItem/lastUseSlot set, opcalled=true, SetInteraction(targetOpObjU, -1), snapshot

### handler_opobj_test.go (fireOpTriggerObj / fireApTriggerObj via tryFire*)

- Delayed → no script fired
- objStillValid fails → ClearInteraction + interactionFired=true
- No script registered (OP path) → "Nothing interesting happens." + clear
- No script registered (AP path) → apRange=-1, interactionFired=true (no message)
- Script fires → ActiveObj set, PtrActiveObj pointer, interactionFired=true
- apObjTriggerForOp with T sentinel → TriggerApObjT
- apObjTriggerForOp with U sentinel → TriggerApObjU
- AP persistence: apRangeCalled=true keeps interaction anchored
- tryFireOpTrigger with *entitypkg.Obj target reaches fireOpTriggerObj (not default)
- tryFireApTrigger with *entitypkg.Obj target reaches fireApTriggerObj (not default)

## 8. Deviations

### NAI-50-D1 (active): OPOBJT spellCom component validation skipped

TS OpObjTHandler.ts:20-29 validates `spellCom` references a component with
`ComActionTarget.OBJ` flag AND that the component is visible in the player's interface
stack. Skipped because goscape has no component registry. Same cluster as S6m-D1,
NAI-45-D1, NAI-48-D1.

**Production tag:** `modules/world/handler_opobj.go` (handleOpObjT).
**Closure:** component-registry sub-spec.

### NAI-50-D2 (active): OPOBJU useCom component validation skipped

TS OpObjUHandler.ts:39-48 validates `useCom` references a usable, visible component.
Skipped for the same reason as NAI-50-D1.

**Production tag:** `modules/world/handler_opobj.go` (handleOpObjU).
**Closure:** component-registry sub-spec.

### Pre-existing: S6l-D4 (Chebyshev distance for all target types)

TS `Player.inOperableDistance` for Obj calls `reachedEntity(...) || reachedObj(...)`
(Player.ts:1109-1110) where `reachedObj` uses rsmod collision shape logic. goscape uses
Chebyshev ≤ 1 for all target types. NAI-50 inherits this deviation; does not introduce
it. Behaviorally negligible for 1×1 ground items.

## 9. Deviation Tally

- Pre-NAI-50: 20
- Closes: 0
- Opens: +2 (NAI-50-D1, NAI-50-D2)
- Post-NAI-50: **22**

## 10. Proposed Task Split

| Task | Files | Est. LOC (prod) |
|---|---|---|
| T1 | `obj_lookup.go` + `obj_lookup_test.go` | ~30 + ~50 |
| T2 | `pkg/objtype/objtype.go` hidden→"" coercion + `handler_opobj.go` + `handler_opobj_test.go` (handleOpObj + 5 shims + T + U) | ~165 + ~200 |
| T3 | `interaction.go` sentinels + `interaction_trigger.go` (apObjTriggerForOp + fireOpTriggerObj + fireApTriggerObj + 2 case arms) + tests | ~80 + ~80 |
| T4 | `handlers_game.go` registrations + NAI-50-D1/D2 doc-comment tags + close commit | ~10 |

T1 is a prerequisite for T2 (handleOpObjT/U call GetObj). T2 and T3 are independent;
T4 follows both.
