# RuneScript S5e: Inventory Opcodes Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Register 17 core INV_* handlers. One new `InvLookup` interface on `ScriptState` resolves player-owned vs world-shared invs. `Configs` gains `InvType(id)`. Zero new wire code — reuses existing dirty-flag + `updateInvs()` path.

**Architecture:** Handlers call `s.Inv.Get(s.Self, typeID)` to resolve an inventory, then call existing `inventory` package methods. The resolver downcasts `*Player` internally. Mutations automatically dirty-flag and flow to `OpUpdateInvFull` each tick.

**Tech Stack:** Go 1.22+, existing `pkg/inventory`, existing `modules/world/updateInvs()` tick phase.

**Spec:** [`docs/superpowers/specs/2026-04-21-runescript-s5e-inventory-ops-design.md`](../specs/2026-04-21-runescript-s5e-inventory-ops-design.md)

---

## Task 1: Interface extensions + `Configs.InvType`

**Files:**
- Modify: `pkg/script/configs.go` (+InvType method)
- Modify: `pkg/script/state.go` (+Inv field, +InvLookup interface)
- Modify: `modules/world/server_configs.go` (+InvType impl)

- [ ] **Step 1: Add `InvType(id int) *objtype.InvType` to the `Configs` interface** in `pkg/script/configs.go`.

- [ ] **Step 2: Add `InvLookup` interface to `pkg/script/state.go`** (next to `WorldVars`):

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

Import `pkg/inventory`. Also add `Inv InvLookup` field to `ScriptState` next to `Configs`.

- [ ] **Step 3: Add `InvType` method to `serverConfigsView`** in `modules/world/server_configs.go`:

```go
func (c serverConfigsView) InvType(id int) *objtype.InvType {
    if c.s == nil || c.s.invTypes == nil {
        return nil
    }
    if id < 0 || id >= len(c.s.invTypes.Configs) {
        return nil
    }
    return c.s.invTypes.Configs[id]
}
```

- [ ] **Step 4: Build**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./...
```

Expected: clean. No existing code broken.

- [ ] **Step 5: Commit**

```bash
git add pkg/script/configs.go pkg/script/state.go modules/world/server_configs.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(script): InvLookup interface + Configs.InvType for S5e

Adds a single-method InvLookup interface on ScriptState for INV_*
handlers to resolve an inventory by type id. Extends Configs with
InvType(id) so handlers that need scope/param config can read it.
serverConfigsView gains InvType impl.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 2: `invLookupView` adapter + Server wiring

**Files:**
- Create: `modules/world/server_invs.go`
- Modify: `modules/world/server.go`
- Modify: `modules/world/script.go`

- [ ] **Step 1: Create `modules/world/server_invs.go`**

```go
package world

import (
    "github.com/zsrv/goscape/pkg/inventory"
    "github.com/zsrv/goscape/pkg/objtype"
    "github.com/zsrv/goscape/pkg/script"
)

// invLookupView adapts *Server to script.InvLookup. The single
// *Player downcast is contained here.
type invLookupView struct {
    s *Server
}

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
    p, ok := self.(*Player)
    if !ok {
        return nil
    }
    return p.invs[typeID]
}
```

Verify the exact field names on `InvType` by reading `pkg/objtype/invtype.go`:
- `Scope` — is it `int` (as invtype.go showed: `Scope int`) with constant `InvTypeScopeShared = 2`? Confirm.

- [ ] **Step 2: Add `invLookup invLookupView` field to `Server` struct** in `modules/world/server.go`, near `configsView`.

- [ ] **Step 3: Wire it in `NewServer`** after the configsView setup:

```go
s.invLookup = invLookupView{s: s}
```

- [ ] **Step 4: Wire `state.Inv` in `runScript`** at `modules/world/script.go`:

```go
state.Inv = s.invLookup
```

Place next to the existing `state.Configs = s.configsView`.

- [ ] **Step 5: Build + full test suite**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./...
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...
```

Expected: clean build, no regressions.

- [ ] **Step 6: Commit**

```bash
git add modules/world/server_invs.go modules/world/server.go modules/world/script.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(world): invLookupView + runScript wires script.Inv

Routes typeID → either server-shared inv (InvTypeScopeShared) or
player-owned inv via a contained *Player downcast. runScript now sets
state.Inv so INV_* handlers (Task 3) can resolve inventories.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 3: Handlers + tests + map registration

**Files:**
- Create: `pkg/script/handlers_inv.go`
- Create: `pkg/script/handlers_inv_test.go`
- Modify: `pkg/script/handlers.go`
- Possibly modify: `pkg/inventory/inventory.go` (if Clear missing)

- [ ] **Step 1: Investigate `pkg/inventory/inventory.go`** to confirm method signatures match what the spec assumes:

- `GetItemCount(id int) int` — confirm return type
- `Get(slot int) *Item` — confirm
- `Set(slot int, item *Item)` — confirm
- `Delete(slot int)` — confirm
- `FreeSlotCount() int` — confirm
- `Add(id, count int, opts AddOpts)` — check AddOpts shape; if it exists
- `Remove(id, count int, opts RemoveOpts)` — same
- **`Clear()`** — does it exist? If not, add it.

Also read `pkg/inventory/`'s Item struct to know the `ID` / `Count` field names.

- [ ] **Step 2: Read TS InvOps.ts** for exact pop order + formula per opcode. The spec lists guesses; verify the real shapes at `/home/owner/Code/github.com/LostCityRS/Engine-TS/src/engine/script/handlers/InvOps.ts`. Especially:
- INV_ITEMSPACE / INV_ITEMSPACE2 formula (size parameter semantics).
- INV_SETSLOT: does TS pop `(typeID, slot, objID, count)` or a different order?
- INV_TOTALPARAM: does it check ObjType.Params then fall back to ParamType.DefaultInt?

- [ ] **Step 3: Write `pkg/script/handlers_inv.go`** with all 17 handlers + a `resolveInv` helper:

```go
func resolveInv(s *ScriptState, typeID int) *inventory.Inventory {
    if s.Inv == nil {
        return nil
    }
    return s.Inv.Get(s.Self, typeID)
}
```

Every handler calls `resolveInv`, errors on nil, then delegates to `pkg/inventory` methods. For INV_TOTALPARAM / INV_TOTALCAT, iterate the inv's items and look up per-slot ObjType via `s.Configs.ObjType`.

- [ ] **Step 4: Register 17 handlers in `pkg/script/handlers.go`** at the end of the map with an `// S5e: inventory.` comment block.

- [ ] **Step 5: If `pkg/inventory.Inventory.Clear()` is missing**, add it:

```go
// Clear removes every item slot and dirty-flags for wire sync.
func (i *Inventory) Clear() {
    for j := range i.Items {
        i.Items[j] = nil
    }
    i.Update = true
}
```

- [ ] **Step 6: Write `pkg/script/handlers_inv_test.go`** — Use Edit/Write tool, not bash heredoc, to avoid `!=` corruption.

Required coverage:
- `mockInvLookup` struct seeded with 2 inventories: `main` (capacity 28, stack-normal) and `bank` (capacity 100, stack-always).
- `mockConfigs` seeded with 3 ObjTypes (id 0 "empty", id 995 "coins" stackable+category 10, id 2 "arrow" stackable+category 20) and 1 ParamType at id 1 (int, default 0).
- Tests:
  - `TestInvAddThenTotal`: ADD 42 coins, TOTAL returns 42.
  - `TestInvDel`: DEL 10 after ADD 50, TOTAL returns 40.
  - `TestInvDelSlot`: SETSLOT slot=0 (coins,5), DELSLOT 0, GETNUM returns 0.
  - `TestInvGetObjEmptySlot`: returns -1.
  - `TestInvGetObjFilled`: returns the obj id.
  - `TestInvSize`: returns 28.
  - `TestInvClear`: after ADDs, CLEAR; FREESPACE returns 28.
  - `TestInvFreeSpace`: zero after filling non-stackable to capacity.
  - `TestInvItemSpace`: boolean fits-check.
  - `TestInvItemSpace2`: overflow count.
  - `TestInvMoveItem`: between main and bank.
  - `TestInvMoveFromSlot` / `TestInvMoveToSlot`.
  - `TestInvTotalParam`: sums obj.Params[1] across two slot entries.
  - `TestInvTotalCat`: counts items in category 10.
  - `TestInvLookupNilReturnsError`.

- [ ] **Step 7: Run tests**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/...
```

Expected: all pass.

- [ ] **Step 8: Commit**

```bash
git add pkg/script/handlers_inv.go pkg/script/handlers_inv_test.go pkg/script/handlers.go pkg/inventory/inventory.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(script): S5e inventory opcodes (17 handlers)

Reads: INV_TOTAL, INV_GETOBJ, INV_GETNUM, INV_SIZE, INV_FREESPACE,
INV_ITEMSPACE, INV_ITEMSPACE2, INV_TOTALPARAM, INV_TOTALCAT.
Mutations: INV_ADD, INV_DEL, INV_DELSLOT, INV_SETSLOT, INV_CLEAR,
INV_MOVEITEM, INV_MOVEFROMSLOT, INV_MOVETOSLOT.

resolveInv helper dispatches via script.InvLookup.Get. Mutations use
pkg/inventory's existing Add/Remove/Set/Delete which already set
Update=true; wire sync flows through the existing updateInvs() tick
phase.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 4: E2E test

**Files:**
- Modify: `modules/world/script_test.go`

- [ ] **Step 1: Add `TestInvAddGrantsItemsViaScript`** to end of file using Edit tool.

```go
func TestInvAddGrantsItemsViaScript(t *testing.T) {
    s := newTestServer(t)
    s.scriptProvider = script.NewProvider()

    // Seed main inv type at id 0 and a small ObjType for "coins" at id 995.
    // (newTestServer already loaded real cache; we override ObjType[995].)
    mainTypeID := 0
    if s.invTypes.Configs[mainTypeID] == nil {
        // fallback: seed a minimal InvType
        ...
    }

    p, _ := newTestPlayer(t)
    p.client.server = s
    p.client.encryptor = io2.New([4]uint32{1, 2, 3, 4})
    p.invs = make(map[int]*inventory.Inventory)
    p.invs[mainTypeID] = inventory.FromType(s.invTypes.Configs[mainTypeID])

    // Script: push typeID, push 995, push 42, inv_add, return
    sf := &script.ScriptFile{
        Name: "[invadd,test]",
        Opcodes: []script.Opcode{
            script.OpPushConstantInt,
            script.OpPushConstantInt,
            script.OpPushConstantInt,
            script.OpInvAdd,
            script.OpReturn,
        },
        IntOperands:      []int32{int32(mainTypeID), 995, 42, 0, 0},
        StringOperands:   []string{"", "", "", "", ""},
        InstructionCount: 5,
    }
    s.runScript(sf, p, true, nil, nil)

    inv := p.invs[mainTypeID]
    if inv == nil {
        t.Fatal("main inv not allocated")
    }
    if got := inv.GetItemCount(995); got != 42 {
        t.Errorf("inv.GetItemCount(995): got %d, want 42", got)
    }
}
```

Verify the imports. `inventory.FromType` was mentioned in prior code; confirm it exists. Adapt if the signature is different.

- [ ] **Step 2: Run the test + full suite**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestInvAddGrants -v
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race ./...
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go vet ./...
```

All green.

Handler count check: `grep -cE "^\s+Op[A-Z].*handle" pkg/script/handlers.go` → **152**.

- [ ] **Step 3: Commit**

```bash
git add modules/world/script_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
test(world): end-to-end S5e inv_add grants real items via script

Runs a 4-instruction inv_add(main_inv, 995, 42) script against a
freshly-allocated main inventory and asserts GetItemCount(995) == 42,
verifying the full pipeline from handler → InvLookup → *Player
downcast → inventory.Add.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Self-Review

- [ ] `go build ./...` clean
- [ ] `go test ./...` passes
- [ ] `go test -race ./...` clean
- [ ] `go vet ./...` clean
- [ ] Handler count = 152 (135 after S5d + 17 S5e)
