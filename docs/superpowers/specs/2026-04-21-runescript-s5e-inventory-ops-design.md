# Sub-spec RuneScript S5e: Inventory Opcodes — Design

**Status:** Draft → ready for plan
**Scope:** 17 core `INV_*` handlers covering read (INV_TOTAL, INV_GETOBJ, INV_GETNUM, INV_SIZE, INV_FREESPACE, INV_ITEMSPACE, INV_ITEMSPACE2, INV_TOTALPARAM, INV_TOTALCAT) and mutation (INV_ADD, INV_DEL, INV_DELSLOT, INV_SETSLOT, INV_CLEAR, INV_MOVEITEM, INV_MOVEFROMSLOT, INV_MOVETOSLOT). A new `InvLookup` interface on `ScriptState` resolves InvType-scoped inventories (player-owned vs world-shared). Zero new wire code — `pkg/inventory`'s write methods already dirty-flag, and the existing `updateInvs()` tick phase already emits `OpUpdateInvFull` per listener.
**Out of scope:** INV_TRANSMIT / INV_STOPTRANSMIT (UI-component registration — bundle with dialog/interface sub-spec). INV_DROPSLOT (needs active_obj — S6). INV_MOVEITEM_CERT / _UNCERT (cert-variant plumbing — niche). INVOTHER_TRANSMIT / BOTH_* (dual-player interop — S6). INV_ALLSTOCK / INV_STOCKBASE / INV_DEBUGNAME (rarely-used config readers — add later on demand). INV_DROPITEM family.

---

## Goal

After S5e:

- Cache scripts that grant items (`inv_add main_inv, coins, 100`), check quantities (`inv_total main_inv, coins`), or move items between inventories (`inv_moveitem main, bank, coins, 100`) run against the player's real inventory state.
- Mutations automatically flow to the Java client via the existing `updateInvs()` tick phase that emits `OpUpdateInvFull` per registered listener.
- Scope dispatch is correct: writes to `SHARED`-scope invs (bank, shops) go to `Server.invs`; `TEMP`/`PERM` scope invs go to `Player.invs`.
- Demo: a LOGIN-trigger script that calls `inv_add main_inv, coins, 42` grants 42 coins to the player's main inventory; next tick, the client sees the update.

## Architecture

```
pkg/script/
├── state.go                  + Inv InvLookup field
├── configs.go                + InvType lookup on Configs interface
├── handlers_inv.go           (new) 17 handlers + inv-resolution helper
└── handlers_inv_test.go      (new)

modules/world/
├── server_configs.go         + InvType(id) impl
├── server_invs.go            (new) invLookupView implementing script.InvLookup
├── server.go                 + wire invLookupView in NewServer
├── script.go                 + state.Inv = s.invLookup in runScript
└── script_test.go            + E2E inv_add + inv_total round-trip
```

Reuses `pkg/inventory.Inventory` (Add/Remove/Set/Delete/Swap/GetItemCount/NextFreeSlot/FreeSlotCount/IsFull/IsEmpty) verbatim. Reuses `modules/world/updateInvs()` for wire sync verbatim.

## Components

### 1. `script.Configs` — add `InvType`

```go
type Configs interface {
    // ... existing 6 methods ...
    InvType(id int) *objtype.InvType
}
```

Scope dispatch in the invLookupView needs to check `InvType.Scope` — the handler can't do it because `InvType` lives in pkg/objtype and handlers don't need to know about scope directly. But the **lookup** needs the InvType to decide player vs world. Cleanest: give the Configs surface a way to return the InvType, and keep scope dispatch in `invLookupView`.

### 2. `script.InvLookup` — new interface

```go
// InvLookup is the inventory resolution surface for INV_* handlers.
// Implementations route between player-owned and world-shared inventories
// based on InvType.Scope.
type InvLookup interface {
    // Get returns the inventory at typeID for the given active player,
    // or nil if the type is invalid or the player has no such inv.
    Get(self ActivePlayer, typeID int) *inventory.Inventory
}
```

Added as `ScriptState.Inv InvLookup`, wired by `runScript` same as `Configs`.

### 3. `modules/world/server_invs.go`

```go
type invLookupView struct{ s *Server }

func (v invLookupView) Get(self script.ActivePlayer, typeID int) *inventory.Inventory {
    if v.s == nil || v.s.invTypes == nil {
        return nil
    }
    if typeID < 0 || typeID >= len(v.s.invTypes.Configs) {
        return nil
    }
    cfg := v.s.invTypes.Configs[typeID]
    if cfg == nil {
        return nil
    }
    if cfg.Scope == objtype.InvTypeScopeShared {
        return v.s.invs[typeID]
    }
    // Player-owned (TEMP or PERM).
    p, ok := self.(*Player)
    if !ok {
        return nil
    }
    return p.invs[typeID]
}
```

Single downcast contained to this file. Handlers stay pure.

### 4. Handlers — `pkg/script/handlers_inv.go`

Every handler follows the shape:

```go
func handleInvTotal(s *ScriptState) error {
    objID := s.PopInt()
    typeID := s.PopInt()
    inv := resolveInv(s, typeID)
    if inv == nil {
        return fmt.Errorf("INV_TOTAL: no inv for type %d", typeID)
    }
    s.PushInt(inv.GetItemCount(objID))
    return nil
}

// resolveInv is a small helper so we don't repeat the nil-check.
func resolveInv(s *ScriptState, typeID int) *inventory.Inventory {
    if s.Inv == nil {
        return nil
    }
    return s.Inv.Get(s.Self, typeID)
}
```

**Handler-specific shapes** the implementer verifies against TS InvOps.ts:

- **INV_TOTAL** `(typeID, objID) → count`: `inv.GetItemCount(objID)`.
- **INV_ADD** `(typeID, objID, count) → void`: `inv.Add(objID, count, AddOpts{...})`. TS does overflow-to-world drop; we skip that until active_obj — document.
- **INV_DEL** `(typeID, objID, count) → void`: `inv.Remove(objID, count, RemoveOpts{})`.
- **INV_DELSLOT** `(typeID, slot) → void`: `inv.Delete(slot)`.
- **INV_GETOBJ** `(typeID, slot) → objID`: returns `-1` if slot empty. Check `inv.Get(slot)` — if `nil` or id == 0, push -1.
- **INV_GETNUM** `(typeID, slot) → count`: 0 if empty.
- **INV_SETSLOT** `(typeID, slot, objID, count) → void`: `inv.Set(slot, &inventory.Item{ID: objID, Count: count})`.
- **INV_SIZE** `(typeID) → capacity`: `inv.Capacity`.
- **INV_CLEAR** `(typeID) → void`: reset all slots. If `pkg/inventory` has a `Clear()` method, use it; otherwise loop and call `Delete(i)`. Add the helper if missing.
- **INV_FREESPACE** `(typeID) → count`: `inv.FreeSlotCount()`.
- **INV_ITEMSPACE** `(typeID, objID, count, size) → 0/1`: returns 1 if the inv can fit `count` of `objID` given stacking + size constraints. Implementer verifies the exact formula against TS — likely checks if `count + existing <= inv.Capacity * size` or similar.
- **INV_ITEMSPACE2** `(typeID, objID, count, size) → remaining`: returns the count that would overflow.
- **INV_MOVEITEM** `(fromInv, toInv, objID, count) → void`: remove from source, add to dest. Verify both invs resolve before mutating.
- **INV_MOVEFROMSLOT** `(fromInv, toInv, fromSlot) → void`: move the entire slot contents.
- **INV_MOVETOSLOT** `(fromInv, toInv, fromSlot, toSlot) → void`: move the slot to a specific destination slot.
- **INV_TOTALPARAM** `(typeID, paramID) → total`: sum `ObjType.Params[paramID]` across non-empty slots (treating missing param as `ParamType.DefaultInt`). Must iterate slots, look up each item's ObjType via `s.Configs.ObjType`, read param.
- **INV_TOTALCAT** `(typeID, categoryID) → count`: sum counts of items whose `ObjType.Category == categoryID`.

### 5. `InvLookup` field on `ScriptState`

```go
type ScriptState struct {
    // ... existing fields ...
    Inv InvLookup
}
```

Wired in `runScript`:
```go
state.Inv = s.invLookup
```

### 6. Server wiring

```go
// In NewServer, after the configsView block:
s.invLookup = invLookupView{s: s}
```

Add `invLookup invLookupView` to the Server struct.

### 7. `Inventory.Clear()` helper (if missing)

If `pkg/inventory` doesn't already have a Clear method, add one:

```go
// Clear removes all items and sets Update = true.
func (i *Inventory) Clear() {
    for j := range i.Items {
        i.Items[j] = nil
    }
    i.Update = true
}
```

Check existing methods first — if there's an equivalent Reset/Empty, use that.

## Testing

**Handler tests** (`pkg/script/handlers_inv_test.go`) — a `mockInvLookup` seeded with a single stack-normal inventory. Cover:

- INV_ADD then INV_TOTAL: grants 42 items, count returns 42.
- INV_DEL: removes 10, count returns 32.
- INV_GETOBJ / INV_GETNUM: read-back after a SETSLOT.
- INV_SIZE: returns the seeded capacity.
- INV_FREESPACE: works before and after some Adds.
- INV_ITEMSPACE / INV_ITEMSPACE2: with an inv 1 slot from full, verify space=0 and overflow=count.
- INV_MOVEITEM between two invs.
- INV_CLEAR empties everything.
- INV_TOTALPARAM / INV_TOTALCAT using a mockConfigs with 2-3 ObjTypes carrying Category + Params.
- A "no-inv" negative test → error.

**E2E test** (`modules/world/script_test.go`):

`TestInvAddGrantsItemsViaScript`: seed `p.invs[mainInvID]` with an empty 28-slot inventory, seed an `ObjType` at id 995 "Coins", run `push_constant_int mainInvID, push_constant_int 995, push_constant_int 42, inv_add, return`. Assert `p.invs[mainInvID].GetItemCount(995) == 42`.

## LOC estimate

| File | LOC |
|---|---|
| `pkg/script/configs.go` (diff) | +10 |
| `pkg/script/state.go` (diff) | +4 |
| `pkg/script/handlers_inv.go` | ~350 |
| `pkg/script/handlers_inv_test.go` | ~280 |
| `pkg/script/handlers.go` (diff) | +22 |
| `pkg/inventory/inventory.go` (diff) | +10 (Clear if missing) |
| `modules/world/server.go` (diff) | +5 |
| `modules/world/server_configs.go` (diff) | +10 (InvType) |
| `modules/world/server_invs.go` | ~35 |
| `modules/world/script.go` (diff) | +1 |
| `modules/world/script_test.go` (diff) | +55 |
| **Total** | **~780** |

## Key design calls

- **`InvLookup` is a single-method interface**. Scope dispatch lives in the Go impl, not the handler. Keeps handlers ignorant of player-vs-world distinction.
- **`Configs.InvType()` added** — natural companion to the existing 6 lookup methods. Handlers that need per-item param lookups (INV_TOTALPARAM) use `s.Configs.ObjType + s.Configs.ParamType`, same pattern as S5d.
- **Zero new wire code.** `inventory.Add/Remove/Set/Delete` already set `Update = true`; existing `updateInvs()` already emits `OpUpdateInvFull` per listener. Adds reach the client via the dirty-flag + tick-flush path.
- **INV_TRANSMIT / INV_STOPTRANSMIT deferred.** They add/remove UI-component listeners — simple but tangles with modal/interface state we haven't modeled yet. Bundle with dialog sub-spec.
- **No active_obj for INV_DROPSLOT.** Deferred to S6 when world-obj spawning exists.
- **Overflow-to-world drop** (INV_ADD when inv is full) is silent for S5e. TS drops the overflow to the player's tile as a world obj with a despawn timer — needs active_obj. Document.

## Gotchas

- **`Inventory.Add` return type**: survey says it returns `Transaction{Requested, Completed}`. INV_ADD handler doesn't push anything — discard the transaction return. But `INV_ITEMSPACE2` should use the pre-computed overflow count, not actually run Add; verify TS logic.
- **ObjType.Stackable interaction with INV_ADD**: stackable items coalesce into one slot; non-stackable need N empty slots. `inventory.Add` already handles this via `ObjType.Stackable`. Handler doesn't need to re-check.
- **INV_TOTALPARAM missing-param behavior**: TS falls back to `ParamType.DefaultInt`. Handler must look up ParamType via `s.Configs.ParamType(paramID)`.
- **INV_TOTALCAT excludes empty slots**: iterate `inv.Items`, skip nil, check `ObjType.Category`.
- **INV_SETSLOT clamping**: TS drops count if > ObjType allows for non-stackable (max 1). Our `inv.Set` doesn't enforce this; decide whether handler clamps or we trust the caller. MVP: trust the caller (it's a cache-level responsibility).
- **Protect flag**: `InvType.Protect == true` + non-shared scope is TS's "protected inv" (main inv, worn). Scripts that don't hold the `protect` flag on the state can't mutate these. **For S5e we skip this gate** — the LOGIN trigger already passes `protect=true`. Document as a future hardening task.
- **Heredoc `!=` bug** applies to any test code using `!=` — use Edit/Write tool, not bash heredocs.
