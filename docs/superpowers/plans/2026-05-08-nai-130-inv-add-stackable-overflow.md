# NAI-130 — INV_ADD stackable + overflow-to-world TS-fidelity port (implementation plan)

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Bring `INV_ADD` and its underlying `pkg/inventory.Inventory.Add` to TS-fidelity per `Engine-TS/src/engine/Inventory.ts:158-225` and `Engine-TS/src/engine/script/handlers/InvOps.ts:57-83`. Two divergences close: (1) the stack predicate now reads `ObjType.Stackable` (was: ignored — bronze arrows fill 25 slots in Tutorial Combat Instructor smoke); (2) the `handleInvAdd` overflow-to-world drop now wires through `s.World.AddObj` (was: silently discarded with a "deferred limitation" doc comment).

**Architecture:** `AddOpts` extended with `Stackable` and `StockObj` bools (caller pre-computes; `pkg/inventory` stays decoupled from `pkg/objtype`). `Transaction` extended with `Added []SlotEntry` (mirrors TS `InventoryTransaction.added`). `Inventory.Add` rewritten 1:1 against TS branches. `handleInvAdd` adds Configs lookups, `requireActivePlayer` gate, and overflow-to-world drop. `handleInvMoveItem`/`handleInvMoveFromSlot` add the same stackable/stockObj lookups for the to-inv Add.

**Tech Stack:** Go 1.26+

**Spec:** `docs/superpowers/specs/2026-05-08-nai-130-inv-add-stackable-overflow-design.md` (commit `dd5acaf`).

**TS sources (canonical only — `Engine-TS` per `ts_source_canonical_path`):**
- `Engine-TS/src/engine/Inventory.ts:158-225` (`Inventory.add`)
- `Engine-TS/src/engine/script/handlers/InvOps.ts:57-83` (`INV_ADD` opcode)
- `Engine-TS/src/engine/entity/Player.ts:1496-1504` (`Player.invAdd` thin wrapper)

**Cadence note (per `runescript_cadence`, `execution_mode_default`):** subagent-driven-development; fresh implementer per task; final whole-impl Sonnet code review per `superpowers_code_reviewer_model`. Controller pre-flight per `controller_preflight` already complete; findings noted inline.

---

## Files

**Modified:**
- `pkg/inventory/inventory.go` — refactor `Inventory.Add` to TS-fidelity (lines 119-124 AddOpts extension; lines 157-215 Add rewrite).
- `pkg/inventory/transaction.go` — add `SlotEntry` struct + `Transaction.Added []SlotEntry` field.
- `pkg/inventory/inventory_test.go` — add 10 unit tests (existing 7 stay).
- `pkg/script/handlers_inv.go` — refactor `handleInvAdd` (lines 293-307: full TS port + Configs lookups + requireActivePlayer + overflow drop); update `handleInvMoveItem` (lines 367-386) and `handleInvMoveFromSlot` (lines 390-411) Configs lookups for stackable/stockObj.
- `pkg/script/handlers_inv_test.go` — add `testObjSword` fixture (Task 1); refactor `TestInvTotalParam` + `TestInvTotalCat` (Task 1); add 6 overflow-drop tests (Task 4).

**No new files.**

---

## Task 1: pkg/script test fixture migration (precondition for inventory.Add semantics change)

**Why first:** `TestInvTotalParam` (`handlers_inv_test.go:347`) and `TestInvTotalCat` (`handlers_inv_test.go:361`) currently exercise per-slot semantics by calling `OpInvAdd` against `testInvMain` (StackNormal) with `testObjCoin` (Stackable=true). Once Task 3's predicate fix lands, coins will stack into 1 slot instead of distributing 5 slots, and the assertions break. Pre-migrating these tests to a non-stackable obj (`testObjSword`) keeps the per-slot assertions valid while not blocking on Task 3. Per `controller_preflight`.

**Files:**
- Modify: `pkg/script/handlers_inv_test.go:23-83` (add `testObjSword` const + extend `newTestInvConfigs`)
- Modify: `pkg/script/handlers_inv_test.go:347-371` (refactor `TestInvTotalParam` + `TestInvTotalCat`)

**Step 1 — Add `testObjSword` fixture constant:**

- [ ] Add a new test obj id constant at `pkg/script/handlers_inv_test.go:23-28`. Replace the existing const block with:

```go
const (
    testInvMain  = 1 // stack-normal, capacity 28
    testInvBank  = 2 // stack-always, capacity 100
    testObjCoin  = 995
    testObjArr   = 2
    testObjSword = 3 // non-stackable scratch obj for per-slot semantics tests
)
```

**Step 2 — Extend `newTestInvConfigs` to register the sword:**

- [ ] In `pkg/script/handlers_inv_test.go:49-83` (function `newTestInvConfigs`), append before `return mc`:

```go
sword := objtype.NewObjType(testObjSword)
sword.Name = "Sword"
sword.DebugName = "sword"
sword.Stackable = false
sword.Category = 10
sword.Params = objtype.ParamMap{1: uint32(5)}
mc.objs[testObjSword] = sword
```

The sword shares `Category=10` and `params[1]=5` with the original coins fixture, so existing `TestInvTotalCat`/`TestInvTotalParam` arithmetic still works — only the obj id changes.

**Step 3 — Refactor `TestInvTotalParam` to use the sword:**

- [ ] At `pkg/script/handlers_inv_test.go:347-359`, replace the function body:

```go
func TestInvTotalParam(t *testing.T) {
    lookup := newTestInvLookup()
    mc := newTestInvConfigs()
    // testObjSword is non-stackable, so 5 swords distribute one-per-slot
    // in main (StackNormal): 5 slots × 1 count. INV_TOTALPARAM sums the
    // param value per non-empty slot (no count multiply — that's
    // TOTALPARAM_STACK). params[1]=5 × 5 slots = 25.
    runInvOp(t, OpInvAdd, []int{testInvMain, testObjSword, 5}, lookup, mc)
    state := runInvOp(t, OpInvTotalParam, []int{testInvMain, 1}, lookup, mc)
    if got := state.PopInt(); got != 25 {
        t.Errorf("INV_TOTALPARAM: got %d, want 25", got)
    }
}
```

**Step 4 — Refactor `TestInvTotalCat` to use the sword:**

- [ ] At `pkg/script/handlers_inv_test.go:361-372`, replace the function body:

```go
func TestInvTotalCat(t *testing.T) {
    lookup := newTestInvLookup()
    mc := newTestInvConfigs()
    // 3 swords (cat=10, non-stackable: 3 slots × 1 count) + 2 arrows
    // (cat=20, stackable: 1 slot — fixture aside; cat-mismatch means
    // count is irrelevant). TOTALCAT for cat=10 sums the count of sword
    // slots: 3 slots × 1 count = 3.
    runInvOp(t, OpInvAdd, []int{testInvMain, testObjSword, 3}, lookup, mc)
    runInvOp(t, OpInvAdd, []int{testInvMain, testObjArr, 2}, lookup, mc)
    state := runInvOp(t, OpInvTotalCat, []int{testInvMain, 10}, lookup, mc)
    if got := state.PopInt(); got != 3 {
        t.Errorf("INV_TOTALCAT(10 swords): got %d, want 3", got)
    }
}
```

**Step 5 — Verify all `pkg/script` tests still pass at HEAD:**

- [ ] Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/... -count=1 -run "TestInvTotalParam|TestInvTotalCat"`
- [ ] Expected: PASS for both. If FAIL, the fixture migration is incomplete — re-check the Stackable/Category/params consistency.

**Step 6 — Verify full repo tests still pass (regression sweep):**

- [ ] Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./... -count=1 2>&1 | grep -E "^(FAIL|ok)" | tail -20`
- [ ] Expected: all packages report `ok`. If anything FAILs, do not proceed — diagnose and fix the migration.

**Step 7 — Commit:**

- [ ] Run:

```bash
git add pkg/script/handlers_inv_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
test(nai-130): testObjSword fixture for per-slot Total* tests

Migrates TestInvTotalParam and TestInvTotalCat from testObjCoin
(stackable=true) to a new testObjSword=3 (stackable=false). The new
fixture preserves Category=10 and params[1]=5 so the per-slot
arithmetic (5 sword slots × 5 = 25; 3 sword slots = 3) is unchanged
under both pre- and post-NAI-130 inventory.Add semantics.

Precondition for Task 3: once inventory.Add honors ObjType.Stackable,
coins (stackable=true) into a StackNormal main inv would stack into
one slot, breaking the original assertions. Sword distributes
one-per-slot under both predicates.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 2: pkg/inventory type extensions + 10 RED unit tests

**Why combined:** the new tests reference `AddOpts.Stackable`, `AddOpts.StockObj`, and `Transaction.Added`. Splitting types from tests would leave a stranded "types-only" commit with no consumer. Per project pattern, types and their first consumer land together.

**Files:**
- Modify: `pkg/inventory/transaction.go:1-9` (add `SlotEntry` + `Added` field)
- Modify: `pkg/inventory/inventory.go:119-124` (extend `AddOpts`)
- Modify: `pkg/inventory/inventory_test.go` (append 10 new tests)

**Step 1 — Extend `Transaction`:**

- [ ] Replace `pkg/inventory/transaction.go` entirely with:

```go
package inventory

// Transaction is the result of an Add or Remove operation.
type Transaction struct {
    Requested int    // units the caller asked for
    Completed int    // units actually added/removed
    Items     []Item // items moved (used by Transfer)

    // Added lists the (slot, item) pairs actually written by Add.
    // Mirrors TS InventoryTransaction.added (Inventory.ts:194 etc.).
    // Populated on every Add path (stack and non-stack); empty for
    // dry-run, no-op, or Remove.
    Added []SlotEntry
}

// SlotEntry pairs a slot index with the Item value written there.
// Used by Transaction.Added to record per-slot writes during Add.
type SlotEntry struct {
    Slot int
    Item Item
}
```

**Step 2 — Extend `AddOpts`:**

- [ ] At `pkg/inventory/inventory.go:119-124`, replace the `AddOpts` struct with:

```go
type AddOpts struct {
    BeginSlot           int
    AssureFullInsertion bool
    ForceNoStack        bool
    DryRun              bool

    // Stackable signals whether the obj being added is stackable
    // (`ObjType.stackable` in TS). Caller pre-computes from
    // objtype.Configs.ObjType(id).Stackable. Drives the new TS-fidelity
    // stack predicate per Inventory.ts:161. Default zero-value (false)
    // means non-stackable.
    Stackable bool

    // StockObj signals whether the obj is in the inv's stock list
    // (`InvType.stockobj.includes(id)` in TS). Caller pre-computes from
    // InvType.StockObj. Drives the TS line 173 stockObj-aware free-slot
    // guard. Default zero-value (false) means not a stock obj.
    StockObj bool
}
```

**Step 3 — Verify the package compiles before adding tests:**

- [ ] Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./pkg/inventory/...`
- [ ] Expected: clean compile. (Existing tests will not yet reference the new fields.)

**Step 4 — Append 10 RED unit tests to `inventory_test.go`:**

- [ ] Append the following at the end of `pkg/inventory/inventory_test.go` (after `TestIsFullAndFreeSlotCount` at line 97):

```go
// -- NAI-130 TS-fidelity tests --

// (1) Bronze-arrow analogue: stackable obj into normal-stack inv stacks
// in a single slot. Pre-NAI-130 this distributed 25 slots × count=1.
func TestAdd_StackableObj_NormalStackInv_StacksInOneSlot(t *testing.T) {
    inv := New(1, 28, StackNormal)
    tx := inv.Add(10, 25, AddOpts{BeginSlot: -1, Stackable: true})
    if tx.Completed != 25 {
        t.Errorf("Completed: got %d, want 25", tx.Completed)
    }
    if inv.Items[0] == nil || inv.Items[0].Id != 10 || inv.Items[0].Count != 25 {
        t.Errorf("slot 0 after add: got %+v, want {Id:10 Count:25}", inv.Items[0])
    }
    if inv.Items[1] != nil {
        t.Errorf("slot 1 should remain empty, got %+v", inv.Items[1])
    }
}

// (2) Regression: non-stackable obj into normal-stack inv distributes
// one-per-slot. Same shape as TestAddNoStackFillsSlots (which uses
// StackNever) but exercises the stackable=false path through StackNormal.
func TestAdd_NonStackableObj_NormalStackInv_FillsSlots(t *testing.T) {
    inv := New(1, 28, StackNormal)
    tx := inv.Add(10, 3, AddOpts{BeginSlot: -1, Stackable: false})
    if tx.Completed != 3 {
        t.Errorf("Completed: got %d, want 3", tx.Completed)
    }
    for i := 0; i < 3; i++ {
        if inv.Items[i] == nil || inv.Items[i].Count != 1 {
            t.Errorf("slot %d: got %+v, want {Id:10 Count:1}", i, inv.Items[i])
        }
    }
    if inv.Items[3] != nil {
        t.Errorf("slot 3 should remain empty, got %+v", inv.Items[3])
    }
}

// (3) ALWAYS_STACK predicate's right-hand disjunct: even a non-stackable
// obj stacks when the inv is StackAlways (e.g., bank).
func TestAdd_AlwaysStackInv_IgnoresStackableFlag(t *testing.T) {
    inv := New(1, 28, StackAlways)
    tx := inv.Add(10, 10, AddOpts{BeginSlot: -1, Stackable: false})
    if tx.Completed != 10 {
        t.Errorf("Completed: got %d, want 10", tx.Completed)
    }
    if inv.Items[0] == nil || inv.Items[0].Count != 10 {
        t.Errorf("slot 0: got %+v, want {Id:10 Count:10}", inv.Items[0])
    }
    if inv.Items[1] != nil {
        t.Errorf("slot 1 should remain empty, got %+v", inv.Items[1])
    }
}

// (4) AssureFullInsertion + stack-overflow rolls back: previousCount=10,
// StackLimit-count would overflow → tx.Completed=0, slot unchanged.
func TestAdd_AssureFullInsertion_StackOverflow_RollsBack(t *testing.T) {
    inv := New(1, 28, StackAlways)
    inv.Items[0] = &Item{Id: 10, Count: StackLimit - 5}
    tx := inv.Add(10, 10, AddOpts{
        BeginSlot:           -1,
        AssureFullInsertion: true,
        Stackable:           true,
    })
    if tx.Completed != 0 {
        t.Errorf("Completed (should roll back): got %d, want 0", tx.Completed)
    }
    if inv.Items[0].Count != StackLimit-5 {
        t.Errorf("slot 0 should be unchanged: got Count=%d, want %d", inv.Items[0].Count, StackLimit-5)
    }
}

// (5) AssureFullInsertion + non-stack overflow rolls back: free=2,
// count=3 → tx.Completed=0, no slot mutation.
func TestAdd_AssureFullInsertion_NonStackOverflow_RollsBack(t *testing.T) {
    inv := New(1, 3, StackNormal)
    // Pre-fill 1 slot so free=2.
    inv.Items[0] = &Item{Id: 99, Count: 1}
    tx := inv.Add(10, 3, AddOpts{
        BeginSlot:           -1,
        AssureFullInsertion: true,
        Stackable:           false,
    })
    if tx.Completed != 0 {
        t.Errorf("Completed (should roll back): got %d, want 0", tx.Completed)
    }
    if inv.Items[1] != nil || inv.Items[2] != nil {
        t.Errorf("non-stack rollback should leave slots 1/2 empty; got %+v %+v", inv.Items[1], inv.Items[2])
    }
}

// (6) Free=0 + stack + previousCount=0 + !stockObj → fail. This is the
// TS line 173 early-return for invs with no slots and no existing stack
// for a non-stock obj.
func TestAdd_FreeZero_NoExistingStack_NoStockObj_Fails(t *testing.T) {
    inv := New(1, 2, StackAlways)
    // Fill both slots with OTHER objs so free=0 and obj 10 has no stack.
    inv.Items[0] = &Item{Id: 99, Count: 1}
    inv.Items[1] = &Item{Id: 88, Count: 1}
    tx := inv.Add(10, 5, AddOpts{
        BeginSlot: -1,
        Stackable: true,
        StockObj:  false,
    })
    if tx.Completed != 0 {
        t.Errorf("Completed (no slot, no stock): got %d, want 0", tx.Completed)
    }
    if inv.Items[0].Id != 99 || inv.Items[1].Id != 88 {
        t.Errorf("slots should be unchanged; got %+v %+v", inv.Items[0], inv.Items[1])
    }
}

// (7) Free=0 + stack + StockObj=true + existing depleted stock slot →
// the TS line 173 stockObj guard skips the early-return; getItemIndex
// finds the depleted slot; stack branch increments it.
func TestAdd_FreeZero_StockObj_ExistingDepletedStock_Succeeds(t *testing.T) {
    inv := New(1, 2, StackAlways)
    // Slot 0 holds the depleted stock slot for obj 10 (Count=0 but
    // non-nil so freeSlotCount() == 0). Slot 1 holds another obj.
    inv.Items[0] = &Item{Id: 10, Count: 0}
    inv.Items[1] = &Item{Id: 99, Count: 1}
    tx := inv.Add(10, 5, AddOpts{
        BeginSlot: -1,
        Stackable: true,
        StockObj:  true,
    })
    if tx.Completed != 5 {
        t.Errorf("Completed (stockObj depleted): got %d, want 5", tx.Completed)
    }
    if inv.Items[0].Count != 5 {
        t.Errorf("depleted stock slot: got Count=%d, want 5", inv.Items[0].Count)
    }
}

// (8) StackLimit clamp on stack add without AssureFullInsertion: adds
// up to the limit and reports clamped tx.Completed.
func TestAdd_StackLimitClamp_NonAssure(t *testing.T) {
    inv := New(1, 28, StackAlways)
    inv.Items[0] = &Item{Id: 10, Count: StackLimit - 3}
    tx := inv.Add(10, 10, AddOpts{
        BeginSlot:           -1,
        AssureFullInsertion: false,
        Stackable:           true,
    })
    if tx.Completed != 3 {
        t.Errorf("Completed (clamped): got %d, want 3", tx.Completed)
    }
    if inv.Items[0].Count != StackLimit {
        t.Errorf("clamped slot: got Count=%d, want %d", inv.Items[0].Count, StackLimit)
    }
}

// (9) Partial non-stack add without AssureFullInsertion: fills as many
// slots as available and reports partial tx.Completed.
func TestAdd_PartialNonStack_ReturnsCompletedCount(t *testing.T) {
    inv := New(1, 3, StackNormal)
    tx := inv.Add(10, 5, AddOpts{
        BeginSlot:           -1,
        AssureFullInsertion: false,
        Stackable:           false,
    })
    if tx.Completed != 3 {
        t.Errorf("Completed (partial): got %d, want 3", tx.Completed)
    }
    for i := 0; i < 3; i++ {
        if inv.Items[i] == nil || inv.Items[i].Count != 1 {
            t.Errorf("slot %d: got %+v, want {Id:10 Count:1}", i, inv.Items[i])
        }
    }
}

// (10) Transaction.Added populated for non-stack add: lists each (slot,
// item) actually written. Mirrors TS Inventory.add `added` array.
func TestAdd_TransactionAddedPopulated(t *testing.T) {
    inv := New(1, 28, StackNormal)
    tx := inv.Add(10, 2, AddOpts{BeginSlot: -1, Stackable: false})
    if len(tx.Added) != 2 {
        t.Fatalf("Added len: got %d, want 2", len(tx.Added))
    }
    if tx.Added[0] != (SlotEntry{Slot: 0, Item: Item{Id: 10, Count: 1}}) {
        t.Errorf("Added[0]: got %+v, want {Slot:0 Item:{Id:10 Count:1}}", tx.Added[0])
    }
    if tx.Added[1] != (SlotEntry{Slot: 1, Item: Item{Id: 10, Count: 1}}) {
        t.Errorf("Added[1]: got %+v, want {Slot:1 Item:{Id:10 Count:1}}", tx.Added[1])
    }
}
```

**Step 5 — Run the tests; expect failures (RED):**

- [ ] Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/inventory/... -count=1 -run "TestAdd_" -v 2>&1 | tail -60`
- [ ] Expected: tests (1), (3), (8) PASS at HEAD because the existing impl honors `StackAlways` and stacks correctly. Tests (2), (4), (5), (6), (7), (9), (10) FAIL at HEAD — no Stackable handling, no StockObj guard, no AssureFullInsertion shape, no `Transaction.Added` population. Specifically:
  - (2) FAILS: existing impl distributes correctly but the `Stackable: false` flag is ignored — sub-result still PASSES because StackNormal also distributes one-per-slot via the non-stack branch. Actually verify by running.
  - Run, then if any unexpected PASS appears, treat the test as a vacuously-true case and review whether the test exercises the right branch.

- [ ] If a test PASSES unexpectedly at HEAD, the test is not pinning the new behavior and must be redesigned. Stop, escalate.

**Step 6 — Run existing tests too; expect they still PASS:**

- [ ] Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/inventory/... -count=1 -run "TestNewHasCapacity|TestAddIntoEmptyInventory|TestAddStackingBehavior|TestAddNoStackFillsSlots|TestRemoveDecrementsCount|TestRemovePartialWhenInsufficient|TestSwapExchangesSlots|TestIsFullAndFreeSlotCount" 2>&1 | tail -10`
- [ ] Expected: all 7 existing tests PASS. If any FAIL, the type extension introduced an unexpected change — diagnose before proceeding.

**Step 7 — Commit:**

- [ ] Run:

```bash
git add pkg/inventory/transaction.go pkg/inventory/inventory.go pkg/inventory/inventory_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
test(nai-130): pkg/inventory.Add TS-fidelity types + 10 RED tests

Extends AddOpts with Stackable bool + StockObj bool (caller-precomputed
per Inventory.ts:159-161, 173). Extends Transaction with Added
[]SlotEntry mirroring TS InventoryTransaction.added.

Adds 10 unit tests pinning each TS branch:
  (1) Stackable obj in StackNormal inv stacks (bronze-arrow smoke pin)
  (2) Non-stackable obj in StackNormal inv distributes one-per-slot
  (3) StackAlways inv ignores the Stackable flag
  (4) AssureFullInsertion + stack overflow rolls back
  (5) AssureFullInsertion + non-stack overflow rolls back
  (6) Free=0 + stack + !stockObj fails (TS line 173)
  (7) Free=0 + stockObj + depleted stock slot succeeds
  (8) StackLimit clamp without AssureFullInsertion
  (9) Partial non-stack add reports actual Completed count
  (10) Transaction.Added populated for non-stack add

Tests at HEAD: (1)(3)(8) pass vacuously via StackAlways path; the rest
FAIL until Task 3 lands the predicate refactor. No production behavior
changes in this commit (types-only + tests).

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 3: pkg/inventory.Add TS-fidelity port (GREEN)

**Files:**
- Modify: `pkg/inventory/inventory.go:157-215` (`Inventory.Add` body refactor)

**Step 1 — Replace `Inventory.Add` with the TS-shape port:**

- [ ] At `pkg/inventory/inventory.go:157-215`, replace the entire function body:

```go
// Add inserts up to count units of obj id into the inv. Mirrors TS
// Inventory.add (Engine-TS/src/engine/Inventory.ts:158-225) 1:1.
//
// Stack predicate (TS line 161):
//   stack = !ForceNoStack && stackType != StackNever
//        && (Stackable || stackType == StackAlways)
//
// Returns Transaction with Completed (units written) and Added (per-slot
// SlotEntries actually written; empty for no-op or DryRun).
func (inv *Inventory) Add(id, count int, opts AddOpts) Transaction {
    tx := Transaction{Requested: count}
    if count <= 0 {
        return tx
    }

    // TS line 161: stack predicate.
    stack := !opts.ForceNoStack &&
        inv.StackType != StackNever &&
        (opts.Stackable || inv.StackType == StackAlways)

    // TS lines 163-166: previousCount is non-zero only on the stack path.
    var previousCount int
    if stack {
        previousCount = inv.GetItemCount(id)
    }

    // TS lines 168-170: stack already at limit — short-circuit.
    if previousCount == StackLimit {
        return tx
    }

    free := inv.FreeSlotCount()
    // TS lines 172-175: free=0 guard with stockObj exception.
    if free == 0 && (!stack || (stack && previousCount == 0 && !opts.StockObj)) {
        return tx
    }

    // TS lines 177-191: AssureFullInsertion gate.
    if opts.AssureFullInsertion {
        if stack && previousCount > StackLimit-count {
            return tx
        }
        if !stack && count > free {
            return tx
        }
    } else {
        if stack && previousCount == StackLimit {
            return tx
        }
        if !stack && free == 0 {
            return tx
        }
    }

    // TS lines 196-213: non-stack branch.
    if !stack {
        startSlot := opts.BeginSlot
        if startSlot < 0 {
            startSlot = 0
        }
        completed := 0
        for i := startSlot; i < inv.Capacity && completed < count; i++ {
            if inv.Items[i] != nil {
                continue
            }
            it := Item{Id: id, Count: 1}
            if !opts.DryRun {
                inv.Items[i] = &Item{Id: id, Count: 1}
            }
            tx.Added = append(tx.Added, SlotEntry{Slot: i, Item: it})
            completed++
        }
        if !opts.DryRun && completed > 0 {
            inv.Update = true
        }
        tx.Completed = completed
        return tx
    }

    // TS lines 214-? : stack branch — find or allocate the stack slot.
    stackIndex := inv.GetItemIndex(id)
    if stackIndex == -1 {
        if opts.BeginSlot == -1 {
            stackIndex = inv.NextFreeSlot()
        } else {
            stackIndex = opts.BeginSlot
        }
        if stackIndex < 0 || stackIndex >= inv.Capacity {
            return tx
        }
    }

    // Clamp at StackLimit.
    addCount := count
    if previousCount+addCount > StackLimit {
        addCount = StackLimit - previousCount
    }
    if addCount <= 0 {
        return tx
    }

    var written Item
    if !opts.DryRun {
        if inv.Items[stackIndex] == nil {
            inv.Items[stackIndex] = &Item{Id: id, Count: addCount}
        } else {
            inv.Items[stackIndex].Count += addCount
        }
        inv.Update = true
        written = *inv.Items[stackIndex]
    } else {
        written = Item{Id: id, Count: previousCount + addCount}
    }
    tx.Completed = addCount
    tx.Added = []SlotEntry{{Slot: stackIndex, Item: written}}
    return tx
}
```

**Step 2 — Run the new TS-fidelity tests; expect GREEN:**

- [ ] Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/inventory/... -count=1 -run "TestAdd_" -v 2>&1 | tail -60`
- [ ] Expected: all 10 tests PASS. If any FAIL, the refactor diverges from the test's pin — re-read the failing test against TS line numbers.

**Step 3 — Run all `pkg/inventory` tests; expect GREEN:**

- [ ] Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/inventory/... -count=1`
- [ ] Expected: PASS — no regression on the 7 existing tests.

**Step 4 — Run repo-wide tests; expect GREEN:**

- [ ] Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./... -count=1 2>&1 | grep -E "^(FAIL|ok)" | tail -25`
- [ ] Expected: all `ok`. **If FAIL appears, especially in `pkg/script` or `modules/world`, that's a sign that an existing test fixture relied on the old predicate.** Likely candidates: anything passing a stackable obj into a StackNormal inv via `inv.Add`. Audit and either:
  - Update the test fixture (preserve test intent with a different obj id), OR
  - Pass the new `AddOpts.Stackable` flag explicitly if the test is asserting stacking semantics.
- [ ] If `pkg/script/handlers_inv_test.go` tests fail, re-verify Task 1 landed correctly.

**Step 5 — Commit:**

- [ ] Run:

```bash
git add pkg/inventory/inventory.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(nai-130): pkg/inventory.Add TS-fidelity port

Refactors Inventory.Add 1:1 against TS Inventory.add (Engine-TS/src/
engine/Inventory.ts:158-225). The new stack predicate (TS line 161)
honors ObjType.Stackable, the StockObj guard (TS line 173) admits
depleted stock slots, the AssureFullInsertion semantics roll back
partial adds, and the StackLimit clamp matches TS exactly.

The 10 RED tests from the previous commit now pass.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 4: pkg/script overflow-drop tests (RED)

**Files:**
- Modify: `pkg/script/handlers_inv_test.go` (append 6 new tests)

**Step 1 — Append 6 RED tests:**

- [ ] At the end of `pkg/script/handlers_inv_test.go`, append:

```go
// -- NAI-130 overflow-to-world tests --

// helper: build a fakeWorldAddObj-backed state for INV_ADD overflow tests.
// Sets up: mc with stackable + non-stackable test objs, an InvType=1
// (StackNormal capacity 28), an mockPlayer at level=0, x=3200, z=3200,
// uid=12345. Returns the state, the world recorder, and the inv (so the
// caller can pre-fill it).
func newInvAddOverflowState(t *testing.T) (*ScriptState, *fakeWorldAddObj, *inventory.Inventory) {
    t.Helper()
    s := newTestState(minimalScript(OpReturn))
    w := newFakeWorldMembers()
    s.World = w

    mc := newTestInvConfigs()
    invType := objtype.NewInvType(testInvMain)
    invType.Size = 28
    mc.invs[testInvMain] = invType
    s.Configs = mc

    inv := inventory.New(testInvMain, 28, inventory.StackNormal)
    s.Inv = &mockInvLookup{invs: map[int]*inventory.Inventory{testInvMain: inv}}

    s.Self = &mockPlayer{
        uidValue:    12345,
        coordPacked: coordgrid.PackCoord(0, 3200, 3200),
        x:           3200,
        z:           3200,
    }
    s.Pointers |= PtrActivePlayer
    return s, w, inv
}

// (1) No-overflow regression: bag has space; no AddObj calls.
func TestInvAdd_NoOverflow_NoWorldAddObj(t *testing.T) {
    s, w, _ := newInvAddOverflowState(t)
    s.PushInt(testInvMain)
    s.PushInt(testObjCoin)
    s.PushInt(5)
    if err := handleInvAdd(s); err != nil {
        t.Fatalf("handleInvAdd: %v", err)
    }
    if len(w.addedCalls) != 0 {
        t.Errorf("no overflow → no AddObj calls, got %d", len(w.addedCalls))
    }
}

// (2) Stackable overflow > 1: full bag (no existing stack) + stackable
// obj + overflow=5 → 1 AddObj with count=5.
func TestInvAdd_StackableOverflow_GreaterThanOne_SingleDrop(t *testing.T) {
    s, w, inv := newInvAddOverflowState(t)
    // Fill all 28 slots with OTHER (non-stackable) objs so free=0 and
    // the stackable obj has no existing stack.
    for i := 0; i < 28; i++ {
        inv.Items[i] = &inventory.Item{Id: testObjSword, Count: 1}
    }
    s.PushInt(testInvMain)
    s.PushInt(testObjCoin) // Stackable=true per newTestInvConfigs
    s.PushInt(5)
    if err := handleInvAdd(s); err != nil {
        t.Fatalf("handleInvAdd: %v", err)
    }
    if len(w.addedCalls) != 1 {
        t.Fatalf("stackable overflow=5: expected 1 AddObj call, got %d", len(w.addedCalls))
    }
    got := w.addedCalls[0]
    want := addObjCall{level: 0, x: 3200, z: 3200, typeID: testObjCoin, count: 5, duration: 200, receiverID: 12345}
    if got != want {
        t.Errorf("AddObj: got %+v, want %+v", got, want)
    }
}

// (3) Stackable overflow == 1: TS line 75 special case — even stackable,
// overflow=1 emits 1 single-count drop.
func TestInvAdd_StackableOverflow_EqualsOne_SingleDrop(t *testing.T) {
    s, w, inv := newInvAddOverflowState(t)
    for i := 0; i < 28; i++ {
        inv.Items[i] = &inventory.Item{Id: testObjSword, Count: 1}
    }
    s.PushInt(testInvMain)
    s.PushInt(testObjCoin)
    s.PushInt(1)
    if err := handleInvAdd(s); err != nil {
        t.Fatalf("handleInvAdd: %v", err)
    }
    if len(w.addedCalls) != 1 {
        t.Fatalf("stackable overflow=1: expected 1 AddObj call, got %d", len(w.addedCalls))
    }
    if got := w.addedCalls[0].count; got != 1 {
        t.Errorf("AddObj count: got %d, want 1", got)
    }
}

// (4) Non-stackable overflow loops one-per-call.
func TestInvAdd_NonStackableOverflow_LoopsOnePerCall(t *testing.T) {
    s, w, inv := newInvAddOverflowState(t)
    // Fill 25 of 28 slots so free=3; non-stackable distribution puts 3
    // swords in slots 25-27, then overflow=3-3=0 wait let's recompute:
    // count=6 swords, free=3 → 3 fit, 3 overflow.
    for i := 0; i < 25; i++ {
        inv.Items[i] = &inventory.Item{Id: testObjSword, Count: 1}
    }
    s.PushInt(testInvMain)
    s.PushInt(testObjSword) // Stackable=false per newTestInvConfigs
    s.PushInt(6)
    if err := handleInvAdd(s); err != nil {
        t.Fatalf("handleInvAdd: %v", err)
    }
    if len(w.addedCalls) != 3 {
        t.Fatalf("non-stack overflow=3: expected 3 AddObj calls, got %d", len(w.addedCalls))
    }
    for i, c := range w.addedCalls {
        if c.count != 1 {
            t.Errorf("AddObj[%d] count: got %d, want 1", i, c.count)
        }
        if c.typeID != testObjSword {
            t.Errorf("AddObj[%d] typeID: got %d, want %d", i, c.typeID, testObjSword)
        }
    }
}

// (5) Overflow drop coords come from player.CoordPacked() / X / Z.
func TestInvAdd_OverflowDropUsesPlayerCoord(t *testing.T) {
    s, w, inv := newInvAddOverflowState(t)
    // Override the default coord to a recognizable level=2, x=2500, z=3000.
    s.Self.(*mockPlayer).coordPacked = coordgrid.PackCoord(2, 2500, 3000)
    s.Self.(*mockPlayer).x = 2500
    s.Self.(*mockPlayer).z = 3000
    for i := 0; i < 28; i++ {
        inv.Items[i] = &inventory.Item{Id: testObjSword, Count: 1}
    }
    s.PushInt(testInvMain)
    s.PushInt(testObjCoin)
    s.PushInt(7)
    if err := handleInvAdd(s); err != nil {
        t.Fatalf("handleInvAdd: %v", err)
    }
    if len(w.addedCalls) != 1 {
        t.Fatalf("expected 1 AddObj call, got %d", len(w.addedCalls))
    }
    got := w.addedCalls[0]
    if got.level != 2 || got.x != 2500 || got.z != 3000 {
        t.Errorf("AddObj coord: got level=%d x=%d z=%d, want level=2 x=2500 z=3000", got.level, got.x, got.z)
    }
}

// (6) Overflow drop receiverID is the player's UID.
func TestInvAdd_OverflowDropReceiverIsPlayerUID(t *testing.T) {
    s, w, inv := newInvAddOverflowState(t)
    s.Self.(*mockPlayer).uidValue = 99999
    for i := 0; i < 28; i++ {
        inv.Items[i] = &inventory.Item{Id: testObjSword, Count: 1}
    }
    s.PushInt(testInvMain)
    s.PushInt(testObjCoin)
    s.PushInt(3)
    if err := handleInvAdd(s); err != nil {
        t.Fatalf("handleInvAdd: %v", err)
    }
    if len(w.addedCalls) != 1 {
        t.Fatalf("expected 1 AddObj call, got %d", len(w.addedCalls))
    }
    if got := w.addedCalls[0].receiverID; got != 99999 {
        t.Errorf("AddObj receiverID: got %d, want 99999", got)
    }
}
```

**Step 2 — Verify mockPlayer fields exist:**

- [ ] Run: `grep -nE "uidValue|coordPacked|^func \(m \*mockPlayer\) (X|Z|UID|CoordPacked)" pkg/script/runner_test.go`
- [ ] Expected output (or equivalent): `uidValue` and `coordPacked` fields are present on `mockPlayer`; `X()`, `Z()`, `UID()`, `CoordPacked()` methods exist. If `mockPlayer` lacks `x int` / `z int` fields used by `X()` / `Z()`, the helper must set them via the mockPlayer constructor pattern. **If any field/method is missing, escalate — do not invent.**

**Step 3 — Run new tests; expect RED:**

- [ ] Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/... -count=1 -run "TestInvAdd_" -v 2>&1 | tail -40`
- [ ] Expected: all 6 tests FAIL — current `handleInvAdd` silently discards overflow. Specifically:
  - Tests (2), (3), (4) FAIL because no AddObj is called.
  - Tests (5), (6) FAIL for the same reason.
  - Test (1) MAY PASS at HEAD (no overflow → no AddObj calls is the current behavior). If it PASSES, that's expected — it's a regression-pin to ensure Task 5 doesn't introduce spurious AddObj calls.

**Step 4 — Run existing tests; expect GREEN:**

- [ ] Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/... -count=1 -run "TestInv" 2>&1 | grep -E "^(FAIL|ok|---)" | tail -10`
- [ ] Expected: existing tests pass; only the new 5 tests fail.

**Step 5 — Commit:**

- [ ] Run:

```bash
git add pkg/script/handlers_inv_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
test(nai-130): handleInvAdd overflow-to-world RED tests

Adds 6 RED tests pinning the TS InvOps.ts:73-82 overflow drop semantics:
  (1) No overflow → no AddObj calls (regression pin)
  (2) Stackable + overflow > 1 → single AddObj with count=overflow
  (3) Stackable + overflow == 1 → single AddObj with count=1
  (4) Non-stackable + overflow=3 → 3 AddObj calls each count=1
  (5) AddObj coord pulled from CoordPacked/X/Z
  (6) AddObj receiverID is player UID

newInvAddOverflowState helper centralizes the fixture: full-bag fills
via direct inv.Items writes, mockPlayer with PtrActivePlayer pointer
set, mc seeded via newTestInvConfigs (which now includes testObjSword).

All 6 fail at HEAD because handleInvAdd silently discards overflow.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 5: pkg/script handler ports (GREEN)

**Files:**
- Modify: `pkg/script/handlers_inv.go:293-307` (`handleInvAdd` full port)
- Modify: `pkg/script/handlers_inv.go:367-386` (`handleInvMoveItem` Configs lookups)
- Modify: `pkg/script/handlers_inv.go:390-411` (`handleInvMoveFromSlot` Configs lookups)

**Step 1 — Replace `handleInvAdd`:**

- [ ] At `pkg/script/handlers_inv.go:293-307`, replace the function body (including its 3-line doc comment) with:

```go
// handleInvAdd ports TS InvOps.ts:57-83 (INV_ADD, opcode 4302). Pops
// [inv, obj, count]; adds count units of obj to the inv via Inventory.Add
// with caller-precomputed Stackable/StockObj flags. Per TS, any overflow
// drops to the world at the player's tile via World.AddObj — branched on
// (!stackable || overflow == 1) for the per-unit-loop case vs the
// single-stack-drop case (TS InvOps.ts:73-82, duration=200).
//
// DEVIATION-NAI-130-D2: defensive nil-World guard skips the overflow
// drop when s.World is unset (goscape defensive; TS uses static World
// import which is never null). Per defensive_gate_doc_comment_label.
//
// DEVIATION-NAI-130-D3: defensive nil-Configs fallback (Stackable=false,
// StockObj=false) when s.Configs is unset (goscape defensive; TS
// `check(obj, ObjTypeValid)` would throw on missing config). Mirrors
// the invItemSpaceRemaining nil-Configs pattern at handlers_inv.go:113.
func handleInvAdd(s *ScriptState) error {
    if err := requireActivePlayer(s, "INV_ADD"); err != nil {
        return err
    }
    count := s.PopInt()
    obj := s.PopInt()
    typeID := s.PopInt()
    inv := resolveInv(s, typeID)
    if inv == nil {
        return fmt.Errorf("INV_ADD: no inv for type %d", typeID)
    }

    stackable, stockObj := lookupStackableStockObj(s, inv.Type, obj)

    tx := inv.Add(obj, count, inventory.AddOpts{
        BeginSlot:           -1,
        AssureFullInsertion: false,
        Stackable:           stackable,
        StockObj:            stockObj,
    })

    overflow := count - tx.Completed
    if overflow > 0 && s.World != nil {
        level := s.Self.CoordPacked() >> 28
        x := s.Self.X()
        z := s.Self.Z()
        receiverID := s.Self.UID()
        if !stackable || overflow == 1 {
            for i := 0; i < overflow; i++ {
                s.World.AddObj(level, x, z, obj, 1, 200, receiverID)
            }
        } else {
            s.World.AddObj(level, x, z, obj, overflow, 200, receiverID)
        }
    }

    return nil
}

// lookupStackableStockObj returns the (Stackable, StockObj) pair for the
// given (invType, objId), pre-computed from s.Configs for inventory.Add
// to consume. Returns (false, false) on nil-Configs / missing types
// (goscape defensive — see DEVIATION-NAI-130-D3).
func lookupStackableStockObj(s *ScriptState, invTypeID, objID int) (stackable, stockObj bool) {
    if s.Configs == nil {
        return false, false
    }
    if ot := s.Configs.ObjType(objID); ot != nil {
        stackable = ot.Stackable
    }
    if it := s.Configs.InvType(invTypeID); it != nil {
        for _, id := range it.StockObj {
            if int(id) == objID {
                stockObj = true
                break
            }
        }
    }
    return stackable, stockObj
}
```

**Step 2 — Update `handleInvMoveItem` to thread Stackable/StockObj into the to-inv Add:**

- [ ] At `pkg/script/handlers_inv.go:367-386`, replace the existing `toInv.Add(...)` call. The current function body looks like:

```go
func handleInvMoveItem(s *ScriptState) error {
    count := s.PopInt()
    obj := s.PopInt()
    toTypeID := s.PopInt()
    fromTypeID := s.PopInt()
    fromInv := resolveInv(s, fromTypeID)
    if fromInv == nil {
        return fmt.Errorf("INV_MOVEITEM: no inv for from-type %d", fromTypeID)
    }
    toInv := resolveInv(s, toTypeID)
    if toInv == nil {
        return fmt.Errorf("INV_MOVEITEM: no inv for to-type %d", toTypeID)
    }
    tx := fromInv.Remove(obj, count, inventory.RemoveOpts{BeginSlot: -1})
    if tx.Completed == 0 {
        return nil
    }
    toInv.Add(obj, tx.Completed, inventory.AddOpts{BeginSlot: -1})
    return nil
}
```

Replace the final `toInv.Add(...)` line with:

```go
    stackable, stockObj := lookupStackableStockObj(s, toInv.Type, obj)
    toInv.Add(obj, tx.Completed, inventory.AddOpts{
        BeginSlot: -1,
        Stackable: stackable,
        StockObj:  stockObj,
    })
```

**Step 3 — Update `handleInvMoveFromSlot` similarly:**

- [ ] At `pkg/script/handlers_inv.go:390-411`, the function ends with:

```go
    id, cnt := it.Id, it.Count
    fromInv.Delete(fromSlot)
    toInv.Add(id, cnt, inventory.AddOpts{BeginSlot: -1})
```

Replace the final `toInv.Add(...)` line with:

```go
    stackable, stockObj := lookupStackableStockObj(s, toInv.Type, id)
    toInv.Add(id, cnt, inventory.AddOpts{
        BeginSlot: -1,
        Stackable: stackable,
        StockObj:  stockObj,
    })
```

**Step 4 — Run the new overflow tests; expect GREEN:**

- [ ] Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/... -count=1 -run "TestInvAdd_" -v 2>&1 | tail -40`
- [ ] Expected: all 6 tests PASS.

**Step 5 — Run all `pkg/script` tests:**

- [ ] Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/... -count=1 2>&1 | grep -E "^(FAIL|ok|---)" | tail -10`
- [ ] Expected: PASS. Existing inv tests (`TestInvAddThenTotal`, `TestInvDel`, `TestInvMoveItem`, etc.) should remain green because:
  - Tests using `testInvBank` (StackAlways) are unaffected by the predicate change.
  - `TestInvTotalParam`/`TestInvTotalCat` were migrated to `testObjSword` in Task 1.
  - `TestInvLookupNilReturnsError` calls with `lookup=nil` and expects an error; with `requireActivePlayer` now first, the error message may shift. **Verify:** the test asserts `"no inv for type"`. The new `requireActivePlayer` check fires first when `s.Self == nil`. If the test setup has `Self=nil`, the error message becomes `"INV_ADD requires active player"` (or whatever `requireActivePlayer` emits) instead of `"no inv for type"`. Re-check: in `runInvOpExpectErr`, the test calls `Init(sf, nil, ...)` which sets `Self=nil` AND no `PtrActivePlayer` flag. **This will FAIL.** Mitigation in Step 6.

**Step 6 — Re-verify and patch `TestInvLookupNilReturnsError` if needed:**

- [ ] Read `pkg/script/handlers_inv_test.go:376-382` and `pkg/script/runner_test.go` for the `mockPlayer` and `requireActivePlayer` semantics.
- [ ] If `runInvOpExpectErr` calls `Init(sf, nil, false, nil, nil)` and the new `requireActivePlayer` rejects with a different error string than `"no inv for type"`, update `TestInvLookupNilReturnsError`:
  - For `OpInvAdd`, change the expected substring from `"no inv for type"` to whatever `requireActivePlayer` emits (likely `"requires active player"` or `"INV_ADD"` — check the helper in `pkg/script/handlers.go` near other `requireActivePlayer` calls).
  - Lines 379, 381 (`OpInvTotal`, `OpInvClear`) — these handlers don't have `requireActivePlayer` so leave them alone.
- [ ] Re-run the tests. If still failing, escalate before proceeding.

**Step 7 — Run repo-wide tests:**

- [ ] Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./... -count=1 2>&1 | grep -E "^(FAIL|ok)" | tail -25`
- [ ] Expected: all `ok`.

**Step 8 — Run vet:**

- [ ] Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go vet ./...`
- [ ] Expected: clean. Any new findings should be addressed inline.

**Step 9 — Commit:**

- [ ] Run:

```bash
git add pkg/script/handlers_inv.go pkg/script/handlers_inv_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(nai-130): handleInvAdd full TS port + stackable lookup for Move handlers

handleInvAdd now ports TS InvOps.ts:57-83 end-to-end:
  - requireActivePlayer gate (TS uses checkedHandler(ActivePlayer, ...))
  - Stackable/StockObj lookups via shared lookupStackableStockObj helper
  - Inventory.Add called with the new flags
  - Overflow-to-world drop via s.World.AddObj, branched on
    (!stackable || overflow == 1) per TS line 75

handleInvMoveItem and handleInvMoveFromSlot now thread Stackable/StockObj
into the to-inv Add call so cross-inv moves stack correctly when the
to-inv is StackNormal and the obj is stackable.

Retires the 'silently discarded' deferred-limitation doc comment at
handlers_inv.go:293-296. Bundle 2 of NAI-130 closes that gap.

DEVIATION-NAI-130-D2 (defensive nil-World guard) and DEVIATION-NAI-130-D3
(defensive nil-Configs fallback) labeled per defensive_gate_doc_comment_label.

The 6 RED overflow tests from the previous commit now pass.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 6: NAI-130 close — final verification + memory update + close commit

**Files:**
- Optional: `pkg/script/handlers_inv.go` (any reviewer-fix tweaks)
- Memory: `~/.claude/projects/-home-owner-Code-github-com-zsrv-goscape/memory/nai_followups.md` (close entry)
- Commit only: no further code edits unless reviewer flags critical issues.

**Step 1 — Final whole-impl Sonnet code review per `superpowers_code_reviewer_model`:**

- [ ] Dispatch a Sonnet code-reviewer subagent with the spec, plan, and Task 1-5 commit SHAs in scope. Ask for: TS-fidelity verification (line-by-line vs `Engine-TS/src/engine/Inventory.ts:158-225` and `Engine-TS/src/engine/script/handlers/InvOps.ts:57-83`), DEVIATION label correctness per `defensive_gate_doc_comment_label`, no YAGNI, mockPlayer satisfies all needed methods, no stale comments.
- [ ] If the reviewer flags critical issues, address inline as a reviewer-fix sub-commit before the close commit. Path matches NAI-123 reviewer-fix pattern (`ac966db`).

**Step 2 — Verification battery (per `verify_implementer_claims`):**

- [ ] Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./...`
- [ ] Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./... -count=1 -race 2>&1 | grep -E "^(FAIL|ok)" | tail -25`
- [ ] Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go vet ./...`
- [ ] Expected: all clean.

**Step 3 — User-launched smoke handoff per `smoke_test_server_handoff`:**

- [ ] STOP and ask the user to run the smoke. Do NOT launch the server in a Claude background task — Java client cannot reach a sandbox-bound listener.
- [ ] Provide the user with the exact command:
  ```bash
  CGO_ENABLED=0 GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go run -trimpath ./cmd/goscape --config.file config.yaml
  ```
- [ ] Smoke steps: login Tutorial Island fresh char → progress to Combat Instructor → receive 25 bronze arrows → open inventory tab.
- [ ] Pass criterion: bronze arrows occupy 1 slot with count `25`.
- [ ] Wait for user smoke result.

**Step 4 — Close commit (per `close_commit_memory_trailer`):**

- [ ] Once smoke passes, run:

```bash
git commit --allow-empty --no-gpg-sign -m "$(cat <<'EOF'
chore(nai-130): close — INV_ADD stackable + overflow-to-world TS-fidelity port

PRIMARY met (smoke 2026-05-08): bronze arrows from Tutorial Combat
Instructor stack into 1 inventory slot with count=25. Pre-fix:
25 slots × count=1 each.

Bundle 1: pkg/inventory.Inventory.Add ported 1:1 against TS
Inventory.ts:158-225. New stack predicate honors ObjType.Stackable,
StockObj guard admits depleted stock slots, AssureFullInsertion
semantics + StackLimit clamp match TS.

Bundle 2: handleInvAdd ports TS InvOps.ts:57-83. requireActivePlayer
gate added; Configs lookups for Stackable/StockObj plumbed via shared
lookupStackableStockObj helper. Overflow-to-world drop wired via
s.World.AddObj with the !stackable || overflow == 1 branch. Retires
the pre-existing 'silently discarded' deferred-limitation doc comment.
Move handlers updated to thread the same lookup.

DEVIATION tracker:
  D1: StackNever sentinel constant (existing, now activated in predicate)
  D2: nil-World defensive guard (goscape-defensive; TS skips)
  D3: nil-Configs defensive fallback (goscape-defensive; TS throws)
  D4: requireActivePlayer gate added inline (was missing on dispatch path)

5 implementation commits (Task 1 through Task 5), each TDD discipline.

Closes memory: nai_followups.md § "From NAI-127 → Smoke residual #1: bronze arrows fill 25 slots"

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

**Step 5 — Memory update per `post_task_handoff`:**

- [ ] Append a new NAI-130 section to `~/.claude/projects/-home-owner-Code-github-com-zsrv-goscape/memory/nai_followups.md` with the same shape as NAI-129's close entry (scope, spec/plan refs, commits, smoke result, deviations, memory pattern matches, carry-forward). Mark NAI-127's residual #1 as resolved with a `Resolved by NAI-130 (smoke 2026-05-08)` marker.
- [ ] No new memory entries this sub-spec unless the implementer surfaces a novel pattern not already captured.

**Step 6 — Resume prompt for next session:**

- [ ] Emit a fresh resume prompt for the user to use after `/clear`. Template:

```
Pick up the next NAI-N sub-spec. Pre-conditions: main is at <NEW SHA>
(NAI-130 closed; bronze-arrow smoke confirmed YYYY-MM-DD). Continue
with NAI-127 close residual #2 (line-of-walk for ranged across fence)
or residual #3 (arrows not consumed when firing ranged attacks), or
return to triage via the standard candidate surfacing flow.
```

---

## Self-review pass

**Spec coverage:**
- Spec §2 Bundle 1 (Inventory.Add port) → Tasks 2 + 3 ✅
- Spec §2 Bundle 2 (handleInvAdd overflow) → Task 5 ✅
- Spec §2 (Move handler stackable lookup) → Task 5 ✅
- Spec §3 Bundle 1 tests (10 unit tests) → Task 2 ✅
- Spec §3 Bundle 2 tests (6 overflow tests) → Task 4 ✅
- Spec §3 fixture audit (testObjCoin/testObjArr stackable bits) → Task 1 + pre-flight ✅
- Spec §3 mockWorld.addedCalls recorder verification → pre-flight (already exists) ✅
- Spec §4 smoke handoff → Task 6 Step 3 ✅
- Spec §5 risk register: nil-Configs guard (D3) → Task 5 Step 1 ✅; nil-World guard (D2) → Task 5 Step 1 ✅; requireActivePlayer gate (D4) → Task 5 Step 1 ✅; StackNever sentinel value → ✅ verified at HEAD (already 2)
- Spec §6 deviations → declared inline in commit and close-commit body ✅
- Spec §8 close criteria → Task 6 ✅

**Placeholder scan:** none found. All steps include concrete code, exact commands, expected output.

**Type consistency:** `lookupStackableStockObj(s, invTypeID, objID)` used identically in `handleInvAdd`, `handleInvMoveItem`, `handleInvMoveFromSlot`. `SlotEntry{Slot, Item}` literal shape matches across Task 2 (def) and Task 2 (test) and Task 3 (impl). `AddOpts.Stackable`/`AddOpts.StockObj` field names consistent.

**Risk: Step 5 of Task 5 flags a potential `TestInvLookupNilReturnsError` regression if `requireActivePlayer` fires before the `no inv for type` error.** Step 6 of Task 5 handles this with explicit re-check + patch instructions. Implementer must NOT skip Step 6.
