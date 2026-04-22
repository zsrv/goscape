# S6p — InvListener Keyed-Map Refactor + OpLocU/OpNpcU Item Validation Design

> **Sub-spec context:** Sixteenth sub-spec in the runescript-s* series. Closes S6m-D3 and S6o-D3 (security-relevant gaps — client can currently forge item-in-slot state). Refactors `Player.invListeners` from a slice to a keyed map, adds runtime `invListenOnCom`/`invStopListenOnCom` registration methods, and wires item-in-slot validation into `handleOpLocU` + `handleOpNpcU`.

> **TS-faithfulness gate:** User requires "true to TS." **Zero new deviations introduced.** Two prior deviations closed.

> **Scope:** Approach 2 (refactor + registration methods + handler validation). 3 tasks. Picks up the `invListenOnCom` runtime methods so that after S6p the deviation closure is practically usable, not just formally correct.

## 1. Goal

Close the OpLocU/OpNpcU item-validation gap by:

1. Restructuring `invListeners` as a keyed map for O(1) listener lookup by interface-component ID (`com`)
2. Providing `invListenOnCom` and `invStopListenOnCom` runtime registration methods (future UI-modal-open and script-opcode consumers)
3. Wiring listener-lookup + inventory-resolution + slot/item-match validation in both `handleOpLocU` and `handleOpNpcU`, matching TS `OpLocUHandler.ts:50-66` and `OpNpcUHandler.ts:35-50`

Observable gain: clients can no longer forge `(useObj, useSlot, useCom)` tuples that don't correspond to real inventory state.

## 2. Architecture

Three phases, one per task.

**Phase A — Data structure refactor (Task 1):** `Player.invListeners []InventoryListener` → `map[int]InventoryListener` keyed by `Com`. Migrate 2 production iteration sites (modal-close, updateInvs) + 6 test-fixture assignments.

**Phase B — Runtime registration API (Task 2):** Two new methods on `*Player`:
- `invListenOnCom(invType, com, source int)` — register (or replace) a listener at `com`, resetting `FirstSeen=true`
- `invStopListenOnCom(com int)` — unregister by `com`

`ActivePlayer` interface extension deferred to a future sub-spec that wires script-opcode consumers (YAGNI).

**Phase C — Handler validation (Task 3):** In `handleOpLocU` and `handleOpNpcU`, replace the `_ = int(r.G2()) // useCom — deliberately discarded` line with real validation: listener lookup, inventory resolution (world-shared vs another-player's-inv), `Inventory.HasAt` item+slot match. A shared `resolveListenerInv` helper captures the listener→inventory resolution for both handlers.

### Data flow

```
UI modal open (future, NOT in S6p):
  → UI code calls p.invListenOnCom(invType=93, com=149, source=-1)
  → p.invListeners[149] = {Type: 93, Com: 149, Source: -1, FirstSeen: true}

Item-on-Loc click (after S6p):
  → handleOpLocU decodes (x, z, locId, useObj=1511, useSlot=3, useCom=149)
  → existing 5 gates pass (delayed, payload, viewport, loc, locType)
  → NEW: listener, ok := p.invListeners[149] → hit
  → NEW: inv := s.invs[93] (Source=-1 → world-shared)
  → NEW: inv.HasAt(3, 1511) → item match OK
  → p.lastUseItem=1511, p.lastUseSlot=3
  → SetInteraction(Engine, loc, targetOpLocU, -1)
```

## 3. File Map

| File | Action | Purpose | Task |
|---|---|---|---|
| `modules/world/player.go` | Modify | Change `invListeners` field type slice → map | 1 |
| `modules/world/player.go` | Modify | 2 iteration-site updates (modal close, updateInvs) | 1 |
| `modules/world/inv_update_test.go` | Modify | 4 test fixtures: slice literal → map literal | 1 |
| `modules/world/modal_close_test.go` | Modify | 2 test fixtures: slice literal → map literal | 1 |
| `modules/world/player.go` | Modify | Add `invListenOnCom` + `invStopListenOnCom` methods | 2 |
| `modules/world/player_inv_test.go` | Create | 6 listener-lifecycle tests | 2 |
| `modules/world/handler_opnpc.go` | Modify | Add `resolveListenerInv` helper (used by both OP*U handlers) | 3 |
| `modules/world/handler_oploc.go` | Modify | Listener lookup + validation in `handleOpLocU`; update S6m-D3 comment | 3 |
| `modules/world/handler_opnpc.go` | Modify | Same validation in `handleOpNpcU`; update S6o-D3 comment | 3 |
| `modules/world/handler_oploc_test.go` | Modify | 6 fixture updates + 4 new validation tests | 3 |
| `modules/world/handler_opnpc_test.go` | Modify | 6 fixture updates + 4 new validation tests | 3 |

**Existing infrastructure leveraged (no changes needed):**
- `InventoryListener` struct (player.go:16-22, 4 fields: Type/Com/Source/FirstSeen) — unchanged
- `Player.invs map[int]*inventory.Inventory` (player.go:178) — unchanged
- `Server.invs`, `Server.players` — for world-shared + other-player inventory resolution
- `inventory.Inventory.HasAt(slot, id int) bool` — already bounds-checks and item-matches
- `sendUnsetMapFlag(p)` helper
- `makeOpLocFixture` / `makeOpNpcFixture` — existing test helpers
- Go 1.21+ `clear(map)` builtin for reset operations

## 4. TS Reference Map

- **InventoryListener struct:** `src/engine/Inventory.ts:38-43` — `{type, com, source, firstSeen}` matches goscape's 4 fields exactly
- **Registration methods:** `src/engine/entity/Player.ts:1441-1471` — `invListenOnCom(inv, com, source)` and `invStopListenOnCom(com)`
- **OpLocU validation:** `src/network/game/client/handler/OpLocUHandler.ts:50-66` — `.find(l => l.com === useComId)` + `validSlot` + `hasAt`
- **OpNpcU validation:** `src/network/game/client/handler/OpNpcUHandler.ts:35-50` — identical shape
- **`getInventoryFromListener`:** `src/engine/entity/Player.ts` — resolves `Source === -1` to world, else to player-by-uid

## 5. Component Details

### 5.1 `invListeners` field refactor

In `modules/world/player.go:179`:

```go
invListeners []InventoryListener
```

Becomes:

```go
// invListeners maps UI component ID (Com) to an InventoryListener.
// Registered via invListenOnCom (S6p); unregistered via
// invStopListenOnCom or cleared on modal close. Keyed structure
// enables O(1) lookup in handleOpLocU / handleOpNpcU's item-match
// validation (S6p closure of S6m-D3 / S6o-D3). Nil until first
// listener registers; safe to read, range, len-check while nil.
invListeners map[int]InventoryListener
```

### 5.2 Modal-close iteration migration

Current `player.go:~227`:

```go
for _, l := range p.invListeners {
    sendUpdateInvStopTransmit(p, l.Com)
}
p.invListeners = p.invListeners[:0]
```

Becomes:

```go
for _, l := range p.invListeners {
    sendUpdateInvStopTransmit(p, l.Com)
}
clear(p.invListeners) // Go 1.21+ map reset
```

### 5.3 `updateInvs` iteration migration

Current `player.go:~418-422` takes `&p.invListeners[i]` for mutation. Map elements can't be addressed; the migration uses read-modify-write:

```go
observed := make([]*inventory.Inventory, 0, len(p.invListeners))
for com, l := range p.invListeners {
    _ = com
    // ... use l.Type, l.Source, l.Com, l.FirstSeen ...
    // If mutation required (e.g., l.FirstSeen = false after first emit),
    // write back: l.FirstSeen = false; p.invListeners[com] = l
}
```

**Task 1 verification step:** implementer inspects the existing loop body. If mutation happens (likely `FirstSeen` flip), use the read-modify-write pattern. If not (field is read-only during iteration), a value-copy `for com, l := range` is sufficient.

### 5.4 `invListenOnCom` + `invStopListenOnCom` (Task 2)

In `modules/world/player.go` near other player-state methods (or a new `player_inv.go` if the file grows too large — implementer's call):

```go
// invListenOnCom registers an inventory listener at the given interface
// component ID. If a listener already exists at com, it's replaced
// (matches TS Player.ts:1441-1462: add-or-replace semantics; FirstSeen
// resets to true on replace).
//
// Source = -1 → world-shared inventory (Server.invs[Type]).
// Source >= 0 → another player's slot (Server.players[Source].invs[Type]).
//
// Lazy-initializes the invListeners map on first call.
func (p *Player) invListenOnCom(invType, com, source int) {
    if p.invListeners == nil {
        p.invListeners = make(map[int]InventoryListener)
    }
    p.invListeners[com] = InventoryListener{
        Type:      invType,
        Com:       com,
        Source:    source,
        FirstSeen: true,
    }
}

// invStopListenOnCom unregisters the listener at the given component
// ID. No-op if no listener exists there. Matches TS Player.ts:
// invStopListenOnCom. Safe on nil map (Go's delete-on-nil semantic).
func (p *Player) invStopListenOnCom(com int) {
    delete(p.invListeners, com)
}
```

### 5.5 `resolveListenerInv` helper (Task 3)

Placement: top of `modules/world/handler_opnpc.go` or adjacent to `sendUnsetMapFlag` in `modules/world/interaction.go` (implementer's judgment; both handlers live in modules/world so scope is identical).

```go
// resolveListenerInv returns the inventory the given listener observes,
// or nil if it can't be resolved. Source = -1 → world-shared inventory
// (Server.invs[Type]); otherwise the source is another player's slot,
// and the inventory is that player's local invs[Type].
func resolveListenerInv(s *Server, listener InventoryListener) *inventory.Inventory {
    if listener.Source == -1 {
        return s.invs[listener.Type]
    }
    if listener.Source < 0 || listener.Source >= len(s.players) {
        return nil
    }
    other := s.players[listener.Source]
    if other == nil {
        return nil
    }
    return other.invs[listener.Type]
}
```

### 5.6 `handleOpLocU` validation gate (Task 3)

In `modules/world/handler_oploc.go`, change the `useCom` decode line:

```go
_ = int(r.G2()) // useCom — deliberately discarded (S6m-D2/D3)
```

To:

```go
useCom := int(r.G2())
```

After the 5 existing validation gates and BEFORE `p.lastUseItem = useObj`, insert:

```go
// S6m-D3 closed in S6p: verify the player has an inv listener at
// useCom and that the claimed item lives at the claimed slot.
// Per TS OpLocUHandler.ts:50-66.
listener, ok := p.invListeners[useCom]
if !ok {
    sendUnsetMapFlag(p)
    return nil
}
inv := resolveListenerInv(s, listener)
if inv == nil {
    sendUnsetMapFlag(p)
    return nil
}
if !inv.HasAt(useSlot, useObj) {
    sendUnsetMapFlag(p)
    return nil
}
```

Also update the multi-line `DEVIATION (S6m-D3)` comment block at line ~194-202 — replace with a 2-line closure note:

```go
// S6m-D3 closed in S6p: per-op useCom listener lookup + slot/item
// validation gates added below, mirroring TS OpLocUHandler.ts:50-66.
```

Keep S6m-D2 (component-visibility) and S6m-D4 (members-only) deviation notes — still open.

### 5.7 `handleOpNpcU` validation gate (Task 3)

Identical transformation in `modules/world/handler_opnpc.go`. Change:

```go
_ = int(r.G2()) // useCom — deliberately discarded (S6o-D2/D3)
```

To:

```go
useCom := int(r.G2())
```

Insert the same listener + resolveListenerInv + HasAt gate after the 5 existing validation gates. Update the S6o-D3 deviation comment to the closure form.

### 5.8 Test fixture migration (Task 1)

Existing `inv_update_test.go` and `modal_close_test.go` assign `invListeners` as slice literals, e.g.:

```go
viewer.invListeners = []InventoryListener{
    {Type: 93, Com: 149, Source: 2, FirstSeen: true},
}
```

Become:

```go
viewer.invListeners = map[int]InventoryListener{
    149: {Type: 93, Com: 149, Source: 2, FirstSeen: true},
}
```

Index-based accesses (e.g., `viewer.invListeners[0].FirstSeen`) become key-based (`viewer.invListeners[149].FirstSeen`).

### 5.9 Handler-test fixture updates (Task 3)

Existing S6m/S6o happy-path tests (e.g., `TestHandleOpLocUSetsInteraction`, `TestHandleOpNpcUSetsInteraction`, and their 5 validation-rejection siblings per handler) currently pass `useCom` through without registering a listener. After S6p's validation gate, those tests would fail at the new listener-lookup gate.

Fix by extending each fixture's setup to:
1. Register a listener via `p.invListenOnCom(invType, useCom, -1)`
2. Populate the referenced inventory with the claimed item at the claimed slot — either via `s.invs[invType] = ...` for world-shared or `p.invs[invType] = ...` depending on Source

Specific migration: tests that assert "handler rejects because of validation gate X" (delayed, short-payload, invalid-slot, dead-npc, missing-NpcType) don't need listener registration — those gates fire before the listener check. Only happy-path tests need the full setup.

## 6. Test Plan

### 6.1 Task 1 — No new tests, 6 fixture migrations

| File | Sites |
|---|---|
| `modules/world/inv_update_test.go` | 4 slice → map conversions |
| `modules/world/modal_close_test.go` | 2 slice → map conversions |

Existing test coverage validates the refactor is behavior-preserving.

### 6.2 Task 2 — 6 new tests

`modules/world/player_inv_test.go`:

| # | Test | Asserts |
|---|---|---|
| 1 | `TestInvListenOnComRegistersNewListener` | Fresh call → map entry with expected Type/Com/Source/FirstSeen=true |
| 2 | `TestInvListenOnComReplacesExisting` | Pre-existing listener at same Com → overwritten; FirstSeen=true after replace |
| 3 | `TestInvListenOnComLazyInitializesMap` | Nil invListeners → first call creates the map |
| 4 | `TestInvStopListenOnComRemovesListener` | Listener present → `delete` succeeds, `len` decreases |
| 5 | `TestInvStopListenOnComNoopForMissingKey` | Missing Com → no panic |
| 6 | `TestInvStopListenOnComNoopForNilMap` | Nil map → no panic (Go's delete-on-nil is safe) |

### 6.3 Task 3 — 8 new tests + 12 existing fixture updates

**New validation tests (4 per handler):**

OpLocU (`modules/world/handler_oploc_test.go`):

| # | Test | Asserts |
|---|---|---|
| 7 | `TestHandleOpLocUMissingListenerRejected` | useCom not in invListeners → UnsetMapFlag, no state change |
| 8 | `TestHandleOpLocUInvalidSlotRejected` | useSlot ≥ inv.Capacity → UnsetMapFlag |
| 9 | `TestHandleOpLocUItemMismatchRejected` | Inv[useSlot].Id != useObj → UnsetMapFlag |
| 10 | `TestHandleOpLocUHappyPathWithRealInv` | Listener + item match → `p.target==loc`, state set |

OpNpcU (`modules/world/handler_opnpc_test.go`): same 4 shape for NPC.

**Existing fixtures to update (~12 tests):**
- 6 OpLocU tests from S6m (happy-path + 5 validation-rejection). Happy-path test needs listener + inv setup.
- 6 OpNpcU tests from S6o. Same.

Validation-rejection tests (delayed, short-payload, invalid-slot, dead-npc, missing-NpcType) DON'T need listener setup — those gates fire before the new listener check.

### 6.4 Totals

**14 new tests + 18 fixture migrations** across 6 test files.

## 7. Task Split

### Task 1 — Slice→map refactor + fixture migration

- `modules/world/player.go` — field type change; 2 iteration-site updates
- `modules/world/inv_update_test.go` — 4 fixture migrations
- `modules/world/modal_close_test.go` — 2 fixture migrations
- All existing tests continue passing
- Commit: `refactor(world): invListeners slice → keyed map (S6p-1)`

### Task 2 — `invListenOnCom` + `invStopListenOnCom`

- `modules/world/player.go` (or new `player_inv.go`) — 2 new methods
- `modules/world/player_inv_test.go` (new) — 6 lifecycle tests
- Depends on Task 1 (map structure)
- Build green; no consumer wires these yet
- Commit: `feat(world): invListenOnCom / invStopListenOnCom runtime registration (S6p-2)`

### Task 3 — Handler validation gates + S6m-D3 + S6o-D3 closure

- `modules/world/handler_opnpc.go` — `resolveListenerInv` helper
- `modules/world/handler_oploc.go` — listener lookup + validation in `handleOpLocU`; update S6m-D3 comment
- `modules/world/handler_opnpc.go` — same in `handleOpNpcU`; update S6o-D3 comment
- `modules/world/handler_oploc_test.go` — 6 fixture updates + 4 new tests
- `modules/world/handler_opnpc_test.go` — 6 fixture updates + 4 new tests
- Depends on Tasks 1 + 2 (map + registration API for fixture setup)
- After commit: S6m-D3 + S6o-D3 CLOSED
- Commit: `feat(world): OpLocU/OpNpcU item validation + S6m-D3/S6o-D3 closure (S6p-3)`

## 8. Deviations from TS

**S6p introduces ZERO new deviations.** Closes 2:

| ID | Status | Notes |
|---|---|---|
| **S6m-D3** | ✅ **CLOSED in S6p** | OpLocU listener lookup + slot/item validation wired |
| **S6o-D3** | ✅ **CLOSED in S6p** | OpNpcU same |

### Still-open deviations after S6p

| ID | Status |
|---|---|
| S6m-D1 / S6o-D1 | Still open — spellCom component-visibility (needs component registry) |
| S6m-D2 / S6o-D2 | Still open — useCom component-visibility (same) |
| S6m-D4 / S6o-D4 | Still open — members-only item gate (needs members-config surface) |
| S6l-D1 | Still open — apRange=-1 sentinel (pure optimization) |
| S6l-D3 | Still open — ProtectedActivePlayer gate |
| S6l-D4 | Still open — LOS/collision in distance checks |
| S6l-D5 | Still open — p_op* / nextTarget re-anchor opcodes |
| S6j-D3 / S6j-D4 / S6j-D6 focus | Still open — storage convention + defensive |

## 9. Scope Estimate

- **Implementation:** ~125 LOC across 4 production files
- **Tests:** ~295 LOC (14 new tests + 18 fixture migrations across 6 files)
- **Commits:** 3 (one per task)
- **Build/test green:** at every commit (Task 1 behavior-preserving; Task 2 additive; Task 3 adds consumers)
- **End-to-end gain:** clients can't forge item state on OpLocU/OpNpcU wire clicks; security hardening

## 10. Out-of-Scope Reminders

- `ActivePlayer` interface extension for script opcodes (e.g., `inv_listen_on_com`) — YAGNI until a script opcode consumer lands
- `invListenOnCom` call sites from UI-modal-open handlers — no UI-modal-open code yet; separate downstream sub-spec
- Component registry + ComActionTarget validation (S6m-D1/D2 + S6o-D1/D2) — different infra track
- Members-config surface (S6m-D4 / S6o-D4) — different infra track
- 4 fire-helpers DRY refactor — still deferred (shape fully matured but real maintenance pain hasn't hit)
