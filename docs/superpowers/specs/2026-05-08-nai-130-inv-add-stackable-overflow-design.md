# NAI-130 — INV_ADD stackable + overflow-to-world TS-fidelity port

**Status:** spec
**Date:** 2026-05-08
**Predecessor:** NAI-127 close (`27090aa`) smoke residual #1 — bronze arrows from Combat Instructor fill 25 inventory slots instead of stacking. NAI-128/129 closed the cascading rat-loot residual; NAI-130 picks up the next NAI-127 residual on the active Tutorial Island content path.
**Tech stack:** Go 1.26+
**Cadence:** full (brainstorm → spec → plan → subagent-driven TDD with two-stage review).

## §1 — Problem & diagnosis

Tutorial Combat Instructor hands the player 25 bronze arrows. Client renders them as 25 separate inventory slots (count `1` each) instead of one stack of count `25`.

**Root cause:** `pkg/inventory.Inventory.Add` derives its stack predicate from `inv.StackType` only:

```go
// pkg/inventory/inventory.go:163
shouldStack := inv.StackType == StackAlways && !opts.ForceNoStack
```

TS `Inventory.add` derives it from BOTH the inv's stack type AND the obj's stackable flag:

```ts
// Engine-TS/src/engine/Inventory.ts:161
const stack = !forceNoStack
           && this.stackType != Inventory.NEVER_STACK
           && (type.stackable || this.stackType == Inventory.ALWAYS_STACK);
```

Bronze arrows have `ObjType.Stackable == true` but the player's main inv is `StackNormal` (not `StackAlways`). Goscape's predicate evaluates `false` → falls through to the per-slot loop at lines 189-202 → 25 single-count slots.

**Adjacent divergence (in scope):** The `handleInvAdd` doc-comment at `pkg/script/handlers_inv.go:295` reads:

> Overflow-to-world drop is NOT implemented; the overflow is silently discarded (documented limitation — needs active_obj plumbing).

TS `InvOps.ts:73-82` drops overflow to the world via `World.addObj`. Goscape now has the `s.World.AddObj(level, x, z, typeID, count, duration, receiverID)` plumbing (NAI-127, NAI-128 G6 probe, NAI-129 UID-space delivery), so the deferred limitation is unblocked. NAI-130 closes it alongside the stackable port — both touch the stackable-flag dependency, both are smoke-adjacent.

**Why no test caught the stackable bug:** `pkg/inventory/inventory_test.go` exercises `Add` only against fixtures with `StackType=StackAlways` (or single-slot fixtures where stack vs non-stack is unobservable). No test combines `Stackable=true` obj with `StackType=StackNormal` inv. `pkg/script/handlers_inv_test.go` uses `runInvOp` which routes through `handleInvAdd` → `inv.Add(obj, count, AddOpts{BeginSlot: -1})`; the test invs are `StackAlways` for stackable fixtures (coins) and `StackNormal` for non-stackable fixtures (arrows). The Stackable+Normal combination never appears.

## §2 — Architecture / fix

Two bundles, mirrored 1:1 against TS:

### Bundle 1 — `pkg/inventory.Inventory.Add` TS-fidelity port

**Reference:** `Engine-TS/src/engine/Inventory.ts:158-225`.

**`AddOpts` extension** (no new package dependencies; `pkg/inventory` stays decoupled from `pkg/objtype`):

```go
type AddOpts struct {
    BeginSlot           int
    DryRun              bool
    ForceNoStack        bool
    AssureFullInsertion bool   // existing
    Stackable           bool   // NEW: caller pre-computes from objtype.Configs.ObjType(id).Stackable
    StockObj            bool   // NEW: caller pre-computes from InvType.StockObj.includes(id)
}
```

**`Transaction` extension:**

```go
type Transaction struct {
    Requested int
    Completed int
    Added     []SlotEntry  // NEW: list of {slot, item} actually written; empty for dry-run / no-op
}

type SlotEntry struct {
    Slot int
    Item Item            // value copy — caller may not mutate
}
```

**`Inventory.Add` shape** — 1:1 with TS:

```go
func (inv *Inventory) Add(id, count int, opts AddOpts) Transaction {
    tx := Transaction{Requested: count}
    if count <= 0 {
        return tx
    }

    // TS line 161
    stack := !opts.ForceNoStack &&
             inv.StackType != StackNever &&
             (opts.Stackable || inv.StackType == StackAlways)

    // TS lines 163-166
    var previousCount int
    if stack {
        previousCount = inv.GetItemCount(id)
    }

    // TS lines 168-170
    if previousCount == StackLimit {
        return tx
    }

    free := inv.FreeSlotCount()
    // TS lines 172-175
    if free == 0 && (!stack || (stack && previousCount == 0 && !opts.StockObj)) {
        return tx
    }

    // TS lines 177-191
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

    // TS lines 196-213 (non-stack branch)
    if !stack {
        startSlot := opts.BeginSlot
        if startSlot < 0 {
            startSlot = 0
        }
        completed := 0
        added := []SlotEntry(nil)
        for i := startSlot; i < inv.Capacity; i++ {
            if inv.Items[i] != nil {
                continue
            }
            it := Item{Id: id, Count: 1}
            if !opts.DryRun {
                inv.Items[i] = &Item{Id: id, Count: 1}
            }
            added = append(added, SlotEntry{Slot: i, Item: it})
            completed++
            if completed >= count {
                break
            }
        }
        if !opts.DryRun && completed > 0 {
            inv.Update = true
        }
        tx.Completed = completed
        tx.Added = added
        return tx
    }

    // TS lines 214-? (stack branch)
    stackIndex := inv.GetItemIndex(id)
    if stackIndex == -1 {
        if opts.BeginSlot == -1 {
            stackIndex = inv.findFreeSlotFrom(0)
        } else {
            stackIndex = opts.BeginSlot
        }
        if stackIndex < 0 || stackIndex >= inv.Capacity {
            return tx
        }
    }
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

**`StackNever` constant:** add `StackNever = 2` (or whatever sentinel matches TS `Inventory.NEVER_STACK`). Currently goscape only has `StackNormal=0` and `StackAlways=1`. Verify TS sentinel value at plan-write time; if no goscape inv currently uses it, the constant exists but is dormant — track as `NAI-130-D1` (declared sentinel without consumer) per `consume_reserved_constant`.

### Bundle 2 — `handleInvAdd` overflow-to-world drop

**Reference:** `Engine-TS/src/engine/script/handlers/InvOps.ts:73-82`.

**Production diff** (`pkg/script/handlers_inv.go`):

```go
func handleInvAdd(s *ScriptState) error {
    count := s.PopInt()
    obj := s.PopInt()
    typeID := s.PopInt()
    inv := resolveInv(s, typeID)
    if inv == nil {
        return fmt.Errorf("INV_ADD: no inv for type %d", typeID)
    }

    // Pre-compute stackable + stockObj for inventory.Add.
    var stackable, stockObj bool
    if s.Configs != nil {
        if ot := s.Configs.ObjType(obj); ot != nil {
            stackable = ot.Stackable
        }
        if it := s.Configs.InvType(inv.Type); it != nil {
            for _, id := range it.StockObj {
                if int(id) == obj {
                    stockObj = true
                    break
                }
            }
        }
    }

    tx := inv.Add(obj, count, inventory.AddOpts{
        BeginSlot:           -1,
        AssureFullInsertion: false,
        Stackable:           stackable,
        StockObj:            stockObj,
    })

    // TS InvOps.ts:73-82 — overflow drops to world.
    overflow := count - tx.Completed
    if overflow > 0 {
        if s.World == nil {
            return nil // DEVIATION-NAI-130-D2: defensive nil-World guard (goscape defensive; TS skips this check).
        }
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
```

**Doc-comment retire:** delete the "Overflow-to-world drop is NOT implemented" lines at `pkg/script/handlers_inv.go:294-296`. Replace with a short `// handleInvAdd ports TS InvOps.ts:57-83.` reference.

**Two MoveItem callers** (`handleInvMoveItem:367`, `handleInvMoveFromSlot:388`) — also need `Stackable`/`StockObj` lookup for the to-inv Add call. TS `Player.invMoveFromSlot` and `invMoveItem` both forward through `Inventory.add` and rely on the same stackable predicate. Update both call sites symmetrically; no overflow-to-world for moves (TS doesn't drop on move — only INV_ADD's handler does).

## §3 — Test plan

### Bundle 1: `pkg/inventory/inventory_test.go`

Existing 7 tests stay. Add 10 unit tests pinning each TS branch:

| # | Name | Pin |
|---|------|-----|
| 1 | `TestAdd_StackableObj_NormalStackInv_StacksInOneSlot` | bronze-arrow analogue: `Stackable: true`, `StackType=StackNormal`, count=25 → 1 slot occupied, `Count=25`, `tx.Completed=25`. |
| 2 | `TestAdd_NonStackableObj_NormalStackInv_FillsSlots` | regression: `Stackable: false`, `StackType=StackNormal`, count=3 → 3 slots, each `Count=1`, `tx.Completed=3`. |
| 3 | `TestAdd_AlwaysStackInv_IgnoresStackableFlag` | `Stackable: false`, `StackType=StackAlways`, count=10 → 1 slot, `Count=10`. (Predicate's right-hand disjunct.) |
| 4 | `TestAdd_AssureFullInsertion_StackOverflow_RollsBack` | stack branch + `previousCount > StackLimit-count` → `tx.Completed=0`, no mutation. |
| 5 | `TestAdd_AssureFullInsertion_NonStackOverflow_RollsBack` | non-stack branch + `count > free` (free=2, count=3) → `tx.Completed=0`, no mutation. |
| 6 | `TestAdd_FreeZero_NoExistingStack_NoStockObj_Fails` | stack + `freeSlots==0 && previousCount==0 && !stockObj` → `tx.Completed=0`. |
| 7 | `TestAdd_FreeZero_StockObj_ExistingDepletedStock_Succeeds` | stockObj scenario: pre-populate a slot with `&Item{Id: testStockObj, Count: 0}` (depleted stock slot — a non-nil item makes `freeSlotCount() == 0`); call `Add(testStockObj, 5, AddOpts{Stackable: true, StockObj: true})`. Pin: `tx.Completed == 5`, the depleted slot now has `Count: 5`. This pins the TS line 173 `stockObj` guard skipping the early-return so `getItemIndex` locates the existing depleted stock slot and the stack-branch increments it. Without the guard, the early-return fires and `tx.Completed == 0`. |
| 8 | `TestAdd_StackLimitClamp_NonAssure` | `previousCount + count > StackLimit` && `!AssureFullInsertion` → adds `StackLimit - previousCount`; `tx.Completed = StackLimit - previousCount`. |
| 9 | `TestAdd_PartialNonStack_ReturnsCompletedCount` | `!AssureFullInsertion`, free=3, count=5 → completed=3; 3 slots filled. |
| 10 | `TestAdd_TransactionAddedPopulated` | non-stack + count=2 → `tx.Added` has 2 entries with the correct slot indices and items. |

### Bundle 2: `pkg/script/handlers_inv_test.go`

Existing tests (lines 141, 152, 354, 366, 367, 380) stay. Add 6 overflow-drop tests:

| # | Name | Pin |
|---|------|-----|
| 1 | `TestInvAdd_NoOverflow_NoWorldAddObj` | regression: bag has space → `mockWorld.addedCalls` is empty. |
| 2 | `TestInvAdd_StackableOverflow_GreaterThanOne_SingleDrop` | full bag (no existing stack) + stackable obj + overflow=5 → exactly 1 `AddObj` call with `count=5`. |
| 3 | `TestInvAdd_StackableOverflow_EqualsOne_SingleDrop` | full bag + stackable + overflow=1 → 1 `AddObj` with `count=1` (TS line 75 special case). |
| 4 | `TestInvAdd_NonStackableOverflow_LoopsOnePerCall` | full bag + non-stackable + overflow=3 → 3 `AddObj` calls each `count=1`. |
| 5 | `TestInvAdd_OverflowDropUsesPlayerCoord` | `mockPlayer.coordPacked = (level<<28)\|(x<<14)\|z` — verify level/x/z extracted correctly into `AddObj` args. |
| 6 | `TestInvAdd_OverflowDropReceiverIsPlayerUID` | `mockPlayer.uid = 12345` → `AddObj.receiverID == 12345`. |

### Fixture audit (plan-write precondition)

Per `plan_runnable_test_fixtures` and `plan_test_coverage_crosscheck`:

1. **`runInvOp` plumbing.** Verify `runInvOp` (in `handlers_inv_test.go`) initializes `s.Configs` with a mock `objtype.Configs` that the new `handleInvAdd` lookup can use. If `s.Configs` is nil in the existing helper, prepend a fixture-precondition T-task to wire it.
2. **Test fixture stackable bits.** Verify `testObjCoin` and `testObjArr` configs in the test setup actually set `Stackable` (true / false respectively). If they don't, the new `Stackable: ot.Stackable` lookup would always read `false` and miss the bug. Re-grep at plan-write.
3. **`mockWorld.addedCalls` recorder.** Verify it exists at `handlers_vars_test.go:69-75` (already referenced at `handlers_inv_test.go:721`). Confirm field name, slice type, and method signature match the new test plan; if absent, add as fixture-precondition T-task.
4. **`mockPlayer.coordPacked` field.** Verified to exist at `runner_test.go:426`. Tests will set it via `m.coordPacked = (level<<28)|(x<<14)|z`.

### Regression check

- `pkg/inventory/inventory_test.go` existing tests rely on the goscape-only `StackType` predicate. Updating `Inventory.Add` changes the predicate to also require `Stackable: true` for `StackNormal` invs. Audit each existing test fixture: confirm whether it implicitly relied on the old `StackAlways`-only behavior. Where fixtures use `StackAlways`, no change needed. Where they use `StackNormal` non-stackable, no change needed. Where they use `StackNormal` with the implicit-stack assumption, the new `AddOpts.Stackable: true` field must be added to the test call — enumerate at plan-write per `plan_enumerate_struct_literals`.
- `pkg/script/handlers_inv_test.go` existing INV_ADD tests at lines 141, 152, 354, 366, 367 must be re-verified post-fix. The change in stacking semantics may affect expected slot counts in tests using non-stackable objs in normal-stack invs. Pre-flight per `controller_preflight`.

## §4 — Smoke handoff

User-launched per `smoke_test_server_handoff`. Steps:

1. Build & run: `CGO_ENABLED=0 GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go run -trimpath ./cmd/goscape --config.file config.yaml`
2. Login Tutorial Island fresh char; progress to the Combat Instructor.
3. Receive 25 bronze arrows from the Combat Instructor.
4. Open the inventory tab.

**Pass criterion (PRIMARY):** bronze arrows occupy **1 slot** with the count `25` rendered (was: 25 slots, count `1` each).

**Secondary observations** (route forward if surfaced; don't block close):
- Drop the bronze-arrow stack via right-click → Drop. Verify single-stack obj appears at the player tile (no per-arrow loop on drop).
- If the player's inventory is otherwise full when receiving the arrows, verify overflow drops to the ground at the player's tile.

**Failure routing:** if arrows still fill 25 slots post-fix, the predicate bug is not the binding issue. Re-audit `Inventory.Add` against the new TS-faithful predicate at HEAD. Open NAI-131 with the new symptom; do not regress this fix.

## §5 — Risk register

Per `risk_register_premise_grep` — every load-bearing claim below requires fresh grep evidence at plan-write time, NOT inferred from this spec.

| Risk | Likelihood | Mitigation |
|---|---|---|
| `s.Configs` is nil in some `handleInvAdd` call paths (test fixtures, edge cases) | Medium — existing handlers have nil-check pattern at `invItemSpaceRemaining:113-117` | Same pattern: nil-Configs → `stackable=false`, `stockObj=false`. Documented as `NAI-130-D3` (goscape-defensive; TS dispatches through `check(...)` and would throw on nil). |
| `s.World` is nil at handler entry | Low (production); medium (some tests) | `NAI-130-D2` defensive nil-guard. Skip overflow drop on nil-World. Pin via test. |
| `s.Self` is nil (no ActivePlayer pointer) | Low — `INV_ADD` is `checkedHandler(ActivePlayer, ...)` per TS `InvOps.ts:57` | Verify at plan-write: confirm `requireActivePlayer` is on the dispatch path for `OpInvAdd` in `handlers.go`. If absent, add it; track as `NAI-130-D4`. |
| `InvType.StockObj` slice contains 0xFFFF sentinel that false-matches `obj=65535` | Very low (no live obj has id 65535) | Document at the lookup site; no defensive guard. |
| `Transaction.Added` is a NEW field — existing positional `Transaction{...}` literals break | Verify at plan-write — grep `Transaction{` across all files | If any positional literals exist, convert to keyed before adding the field. Per `plan_enumerate_struct_literals`. |
| `StackNever` constant value drift from TS sentinel | Low — verify at plan-write against `Engine-TS/src/engine/Inventory.ts` constants | Pin sentinel value with a test asserting `StackNever != StackNormal && StackNever != StackAlways`. |
| Existing `pkg/inventory/inventory_test.go` tests implicitly relied on the old predicate | Medium — must enumerate each test fixture at plan-write | Per `plan_enumerate_struct_literals`: list every `inv.Add(..., AddOpts{...})` call site; classify by expected stack outcome under new predicate; update fixtures. |
| Tutorial Combat Instructor `inv_add` source actually uses a different opcode (e.g., `inv_addslot`) | Very low — TS `~give_arrows` content path uses `inv_add` | Verify by grep `Content/scripts/skill_combat/.../combat_instructor*.rs2` for the give-arrows op. If different opcode, NAI-130 still fixes the underlying `Inventory.Add` predicate; smoke just needs a different content trigger. |
| `mockConfigs` at handler-test layer doesn't set `Stackable` on test objs | Medium — must re-grep at plan-write | Pre-flight T-task to extend mockConfigs / test fixture if needed. |

## §6 — TS-fidelity & deviations

**Closes** (this sub-spec retires):
- Pre-existing deferred limitation at `handlers_inv.go:295` doc-comment (silent overflow discard). Bundle 2 wires the World.AddObj drop.
- The implicit goscape-only divergence in `inventory.Add` predicate (no tracker entry pre-existed; surfaced by NAI-127 close smoke).

**New deviations declared:**

| ID | Site | Description | Rationale | Closure plan |
|---|---|---|---|---|
| `NAI-130-D1` | `pkg/inventory/inventory.go` `StackNever` const | New sentinel constant declared but no production inv currently sets `StackType=StackNever`. | TS-fidelity port preserves the predicate's `stackType != NEVER_STACK` guard. Future content/cache may instantiate it. | Retire when first inv with `StackType=StackNever` lands; verify predicate excludes it from stacking. |
| `NAI-130-D2` | `handleInvAdd` overflow branch | Defensive nil-World guard skips overflow drop. | Goscape-defensive; TS uses static `World` import which is never null. Mirrors NAI-125-D1, NAI-127-D1 pattern per `defensive_gate_doc_comment_label`. | Retire when goscape's nil-World invariant is documented as a top-level precondition. |
| `NAI-130-D3` | `handleInvAdd` Configs lookup | Defensive nil-Configs fallback (`stackable=false, stockObj=false`). | Goscape-defensive; TS `check(obj, ObjTypeValid)` would throw on missing config. Mirrors `invItemSpaceRemaining` pattern. | Retire when test fixtures consistently provide non-nil Configs and the production `runWithConfigs` invariant is enforced. |
| `NAI-130-D4` (conditional) | `handleInvAdd` ActivePlayer pointer | If pre-flight reveals that `OpInvAdd` does NOT route through `requireActivePlayer`, document the gap and add the gate. | TS uses `checkedHandler(ActivePlayer, ...)` per `InvOps.ts:57`. | Retire by wiring the pointer-gate at plan-write (declare the gap as a positive code change, not a deferred deviation). |

**Existing related deviations** (kept, unaffected):
- `NAI-115-D2` — `duration` accepted but not honored on private drops (no despawn-after-N-ticks scheduler). Orthogonal; bronze-arrow smoke does not exercise duration.

## §7 — Out of scope

- NAI-127 residual #2 (line-of-walk for ranged across fence). Engine-layer LOS port; separate sub-spec.
- NAI-127 residual #3 (arrows not consumed when firing ranged attacks). Missile lifecycle / ranged-attack inv-decrement; separate sub-spec.
- `NAI-115-D2` despawn scheduler.
- Beyond bronze-arrow smoke: any additional smoke-surfaced divergences route to NAI-131+ per `smoke_surfaces_adjacent_divergences`.
- Full audit of all `Inventory.Add` consumer call sites for stackable correctness — only the 3 production sites in `handlers_inv.go` are in scope. Other future consumers must follow the new `AddOpts.Stackable`/`StockObj` convention.

## §8 — Close criteria

1. `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./... -count=1` green.
2. Production diff is bounded to: `pkg/inventory/inventory.go` (Add refactor + AddOpts/Transaction extensions), `pkg/script/handlers_inv.go` (3 callers updated; doc-comment retire on handleInvAdd), test files for both packages. No incidental code motion outside these.
3. Smoke (§4) PRIMARY met: bronze arrows render in 1 inv slot.
4. Close commit body cites: pre-fix smoke screenshot or description, post-fix evidence, and `Closes memory:` trailer per `close_commit_memory_trailer` referencing the NAI-127 carry-forward entry for residual #1.
5. Updated `nai_followups.md` entry for NAI-130 documenting any smoke-surfaced adjacent residuals routed to NAI-131+.
