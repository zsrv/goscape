# NAI-131 — INV_* write-handler TS validator sweep

**Status:** spec
**Date:** 2026-05-08
**Predecessor:** NAI-130 close (`88318d3`) deferred-omissions §6 — five TS validators omitted from `handleInvAdd` per NAI-130 scope decision. Audit-first determined the same gates are missing across most goscape `INV_*` write handlers, so NAI-131 sweeps the family.
**Tech stack:** Go 1.26+
**Cadence:** full (brainstorm → spec → plan → subagent-driven TDD with two-stage review).

## §1 — Problem & diagnosis

TS `InvOps.ts` wraps every inventory-write opcode in a uniform validation chain:

1. `checkedHandler(ActivePlayer, ...)` — reject if no active player pointer.
2. `check(invID, InvTypeValid)` — reject out-of-range/missing InvType (literal: `"An input for a Inv type was not valid to use. Input was N."`).
3. `check(objID, ObjTypeValid)` — same shape, for ObjType (where applicable).
4. `check(count, ObjStackValid)` — `1 ≤ count ≤ Inventory.STACK_LIMIT` (`0x7fffffff`).
5. **Protect/scope gate:** `if (!state.pointerGet(ProtectedActivePlayer[state.intOperand]) && invType.protect && invType.scope !== InvType.SCOPE_SHARED) throw "$inv requires protected access: ..."`.
6. **Dummy-item gate (INV_ADD, INV_SETSLOT):** `if (!invType.dummyinv && objType.dummyitem !== 0) throw "dummyitem in non-dummyinv: ..."`.

Goscape ports most write handlers without these gates. Today's behavior on bad input is either silent pass-through (e.g. `lookupStackableStockObj` returns `(false, false)` on missing ObjType) or a different-shaped error (`"INV_ADD: no inv for type N"` vs TS `"... was not valid to use. Input was N."`). For nil-active-player dispatches, six of seven write handlers would dereference `s.Self` and crash rather than emitting a clean script abort.

This is a TS-fidelity gap. None of the gaps is a known smoke-blocker (NAI-130 closed PRIMARY without them), but they leave the engine quietly tolerant of malformed scripts that TS would surface as runtime errors.

### Audit summary (per existing handler)

Handlers in `pkg/script/handlers_inv.go`. ✱ = literal-shape mismatch on InvType error, ✗ = missing.

| Handler | requireActivePlayer | InvTypeValid (literal) | ObjTypeValid | ObjStackValid | protect/scope | dummyitem |
|---|:---:|:---:|:---:|:---:|:---:|:---:|
| handleInvAdd (308) | ✓ | ✱ | ✗ | ✗ | ✗ | ✗ |
| handleInvDel (371) | ✗ | ✱ | ✗ | ✗ | ✗ | n/a |
| handleInvDelSlot (385) | ✗ | ✱ | n/a | n/a | ✗ | n/a |
| handleInvSetSlot (399) | ✗ | ✱ | ✗ | ✗ | ✗ | ✗ |
| handleInvClear (413) | ✗ | ✱ | n/a | n/a | ✗ | n/a |
| handleInvMoveItem (427) | ✗ | ✱✱ | ✗ | ✗ | ✗✗ | n/a |
| handleInvMoveFromSlot (455) | ✗ | ✱✱ | n/a | n/a | ✗✗ | n/a |
| handleInvDropSlot (585) | — already TS-faithful (NAI port complete) — | | | | | |

**Helper inventory (every needed validator already exists):**

| TS validator | goscape helper | Location |
|---|---|---|
| `InvTypeValid` | `checkInvType(s, id, op)` | `handlers_player.go:129` |
| `ObjTypeValid` | `checkObjType(s, id, op)` | `handlers_obj.go:20` |
| `ObjStackValid` | `checkObjStack(c, op)` | `handlers_obj.go:29` *(only enforces ≥1; needs upper bound)* |
| `ActivePlayer` gate | `requireActivePlayer(s, op)` | `handlers_player.go:35` |
| `ProtectedActivePlayer` gate | `requireProtectedActivePlayer(s, op)` | `handlers_player.go:58` |
| Protect+scope conditional | inline pattern at `handleInvDropSlot:612-621` | reused as-is |

No new helpers are introduced.

## §2 — Architecture / fix

A single sub-spec, six tasks. All edits land in `pkg/script/handlers_inv.go` (gates), `pkg/script/handlers_obj.go` (ObjStack upper bound), and matching `_test.go` files (RED tests + fixture patches).

### Task 1 — `handleInvAdd` (5 gates, original NAI-130 deferred set)

**Reference:** TS `InvOps.ts:57-83`.

**Validation order in ported handler:**

```go
func handleInvAdd(s *ScriptState) error {
    if err := requireActivePlayer(s, "INV_ADD"); err != nil {
        return err
    }
    count := s.PopInt()
    obj := s.PopInt()
    typeID := s.PopInt()

    if err := checkInvType(s, typeID, "INV_ADD"); err != nil {
        return err
    }
    if err := checkObjType(s, obj, "INV_ADD"); err != nil {
        return err
    }
    if err := checkObjStack(count, "INV_ADD"); err != nil {
        return err
    }

    invType := s.Configs.InvType(typeID)
    objType := s.Configs.ObjType(obj)

    // Protect/scope gate (TS InvOps.ts:64-66)
    if invType.Protect && invType.Scope != objtype.InvTypeScopeShared {
        if err := requireProtectedActivePlayer(s, "INV_ADD"); err != nil {
            return fmt.Errorf("INV_ADD: $inv requires protected access: %s", invType.DebugName)
        }
    }

    // Dummy-item gate (TS InvOps.ts:68-70)
    if !invType.DummyInv && objType.DummyItem != 0 {
        return fmt.Errorf("INV_ADD: dummyitem in non-dummyinv: %s -> %s",
            objType.DebugName, invType.DebugName)
    }

    inv := resolveInv(s, typeID)
    if inv == nil {
        return fmt.Errorf("INV_ADD: no inv for type %d", typeID)  // post-validator: should be unreachable
    }
    // ... existing inv.Add + overflow path ...
}
```

**Key shape decisions:**

- `checkInvType` runs BEFORE `resolveInv` so the literal matches TS-shape (`"INV_ADD: no InvType with value (N) found"`). `resolveInv` retains its existing `nil → "no inv for type N"` error as a post-validator safety net (defensive; should be unreachable after `checkInvType`).
- `lookupStackableStockObj` is no longer relied on for ObjType validation — `checkObjType` runs first. The helper retains its `(false, false)` defensive fallback (DEVIATION-NAI-130-D3) but is now unreachable for missing ObjType in INV_ADD specifically.
- The protect/scope gate wraps `requireProtectedActivePlayer`'s "script not protected" error in a TS-shaped `"$inv requires protected access: <debugname>"` message via `fmt.Errorf`. The wrapped error preserves the goscape error surface for tests that check the existing literal but matches TS for new tests.

### Task 2 — `handleInvSetSlot` (5 gates, same shape as T1)

**Reference:** TS `InvOps.ts:600-616`. Same 5 gates as INV_ADD including dummyitem. Same insertion pattern. Pop order: `count, obj, slot, typeID` (TS `popInts(4)` returns `[inv, slot, objId, count]` — count on top).

### Task 3 — Single-inv handlers without dummyitem

Three handlers share the same minimal gate set (no ObjType, no ObjStack, no dummyitem):

- **handleInvDelSlot** (`InvOps.ts:144-159`) — InvTypeValid + protect/scope. Pop: `slot, typeID`.
- **handleInvClear** (`InvOps.ts:116-124`) — InvTypeValid + protect/scope. Pop: `typeID`.

And one with full Obj gates but no dummyitem:

- **handleInvDel** (`InvOps.ts:129-141`) — InvTypeValid + ObjTypeValid + ObjStackValid + protect/scope. Pop: `count, obj, typeID`.

All three currently lack `requireActivePlayer`. Add it as the first gate in each.

### Task 4 — Dual-inv handlers (`MoveItem`, `MoveFromSlot`)

- **handleInvMoveItem** (`InvOps.ts:499-531`): two `InvTypeValid` checks, plus `ObjTypeValid`, `ObjStackValid`, plus TWO protect/scope gates (one per inv). Pop order: `count, obj, toTypeID, fromTypeID`.
- **handleInvMoveFromSlot** (`InvOps.ts:323-349`): two `InvTypeValid` checks, two protect/scope gates. No Obj gates (the obj id comes from the source slot, not a stack-pushed input). Pop: `fromSlot, toTypeID, fromTypeID`.

**TS quirk preserved:** Both TS gates check `fromInvType.scope !== SCOPE_SHARED` for both `from` and `to` (line 333: `... && toInvType.protect && fromInvType.scope !== InvType.SCOPE_SHARED`). The `to`-inv's scope is **not** consulted; only the `from`-inv's scope acts as the SHARED escape hatch. This appears to be a TS bug-or-quirk — pinned with a deviation tag (`DEVIATION-NAI-131-D1`) and a TS-asymmetry dual-pin (`ts_asymmetry_dual_pin.md`): pin both the presence (matches TS) and the conspicuous absence of `toInvType.scope` (escalates if upstream fixes).

### Task 5 — `checkObjStack` upper bound + doc-comment correction

`pkg/script/handlers_obj.go:29` currently:

```go
// checkObjStack validates a stack count is positive. Mirrors TS
// NumberPositive (ScriptValidators.ts).
func checkObjStack(c int, op string) error {
    if c < 1 {
        return fmt.Errorf("%s: invalid count (%d)", op, c)
    }
    return nil
}
```

After:

```go
// checkObjStack mirrors TS ObjStackValid (ScriptValidators.ts:121) — a
// ScriptInputRangeValidator over [1, Inventory.STACK_LIMIT=0x7fffffff].
// TS-faithful: rejects 0, negatives, and counts above StackLimit.
func checkObjStack(c int, op string) error {
    if c < 1 || c > inventory.StackLimit {
        return fmt.Errorf("%s: invalid count (%d)", op, c)
    }
    return nil
}
```

The single existing call site is `objAddCommon` (handlers_obj.go:67); upper-bound enforcement there is also TS-faithful (`OBJ_ADD`/`OBJ_ADDALL` use the same validator). Imports: `pkg/inventory` is not yet imported in `handlers_obj.go` — T5 adds the import. No cycle (sibling files `handlers_inv.go` and `state.go` already import `pkg/inventory`).

### Task 6 — Existing-test fixture patches

Adding `requireActivePlayer` to handlers that previously dispatched without it will RED-fail any existing test that initializes `ScriptState` without `Pointers |= PtrActivePlayer` and `Self != nil`.

**Affected fixtures (audit during implementation):** Any test in `handlers_inv_test.go` that calls one of T3/T4 handlers and uses a bare `&ScriptState{}` or `Init(...)` without setting `Pointers` and `Self`. The NAI-130 `runInvOp` helper already wires `mockPlayer{} + PtrActivePlayer` for INV_ADD; sibling helpers may not.

**Strategy:** Bundle 1 leaves T6 as a final cleanup pass — implementer audits failing tests under each task and patches fixtures locally. Each fix preserves the test's intent (mockPlayer is a no-op stub for these tests; gating only changes whether the gate-emit fires before the handler body). Document patched fixtures in T6 commit body.

## §3 — Out of scope

These TS handlers have no goscape implementation yet; porting them is a separate concern, NOT part of NAI-131:

- `INV_CHANGESLOT` (TS `InvOps.ts:86-113`)
- `INV_DROPITEM` (`InvOps.ts:163-186`)
- `INV_DROPITEM_DELAYED` (`InvOps.ts:188-209`)
- `INV_MOVETOSLOT` (`InvOps.ts:353-368`)
- `BOTH_MOVEINV` (`InvOps.ts:373-495`) — secondary-active-player intOperand-swap pattern
- `INV_MOVEITEM_CERT` (`InvOps.ts:535-566`)
- `INV_MOVEITEM_UNCERT` (`InvOps.ts:570-597`)

Track as NAI-132+ candidates — each is a full handler port, not a gate sweep.

Sibling Op*Ops files (`PlayerOps`, `NpcOps`, etc.) likely have analogous gate gaps but are out of scope. NAI-131's sweep is bounded to the InvOps family.

## §4 — Test strategy

Each handler under T1-T4 gets RED tests for each newly-added gate before the gate is wired (TDD per project cadence). Each test:

1. Constructs a `ScriptState` initialized to violate exactly one gate (or to satisfy all gates as a regression-pin GREEN).
2. Runs `Execute` on a single-opcode `ScriptFile` for the handler.
3. Asserts the error literal contains the gate-specific TS-shape substring.

**Per-gate assertion patterns:**

- **No active player:** `Pointers` lacks `PtrActivePlayer`; assert error contains `"INV_X: no active player"`.
- **InvTypeValid:** Pass an `invID` not registered in `s.Configs`; assert `"no InvType with value (N) found"`.
- **ObjTypeValid:** Pass an `objID` not registered; assert `"no ObjType with value (N) found"`.
- **ObjStackValid:** Pass `count = 0`, `count = -1`, `count = StackLimit + 1`; assert `"invalid count (N)"`.
- **Protect/scope:** Configure `InvType.Protect = true`, `Scope = InvTypeScopePerm`, dispatch with `s.Protect = false`; assert error contains `"$inv requires protected access: <debugname>"`.
- **Protect/scope SHARED escape:** Same setup but `Scope = InvTypeScopeShared`; assert no error.
- **Dummy-item gate (T1, T2 only):** Configure `InvType.DummyInv = false`, `ObjType.DummyItem = 1`; assert `"dummyitem in non-dummyinv: <objname> -> <invname>"`.

**Regression pins:**

- Existing GREEN tests in handlers_inv_test.go must remain green after T6 fixture patches. Implementer runs `go test ./pkg/script/... ./pkg/inventory/... -count=1` after every task.
- `runInvOp` helper (NAI-130) is the canonical path for INV_ADD and is already gate-aware. T1's tests reuse it directly. T2-T4 may need a similar helper or use `Init` + manual pointer setup.

**Estimated test count:** ~30-40 tests across all 7 handlers (3-7 RED tests per handler depending on gate count).

## §5 — Anticipated DEVIATIONs

- **DEVIATION-NAI-131-D1:** TS asymmetry in `INV_MOVEITEM`/`INV_MOVEFROMSLOT`/`INV_MOVETOSLOT`/`BOTH_MOVEINV` — both protect/scope gates check `fromInvType.scope !== SCOPE_SHARED`, never `toInvType.scope`. Goscape preserves the asymmetry. Dual-pinned per `ts_asymmetry_dual_pin.md`.

- **DEVIATION-NAI-131-D2 (potential):** Goscape's `requireProtectedActivePlayer` returns `"<op>: script not protected"`. The wrapping `fmt.Errorf("%s: $inv requires protected access: %s", op, invType.DebugName)` preserves the TS message but discards the inner `script not protected` literal. Acceptable: tests assert only the TS-shaped substring.

No other deviations anticipated; every other gate is a 1:1 TS port using existing helpers.

## §6 — Risk register (for plan author / controller pre-flight)

| Risk | Mitigation |
|---|---|
| Test fixture churn surprises during T1-T4 | T6 collects patches at the end; each task implementer runs the package test suite to surface failures |
| Unimported `objtype` package in handlers_inv.go | Already imported (used by NAI-130). No new imports expected. |
| `inventory.StackLimit` import in handlers_obj.go for T5 | Add `"github.com/zsrv/goscape/pkg/inventory"` to handlers_obj.go imports. Sibling files (handlers_inv.go, state.go) already import it; no cycle. |
| Real-content scripts dispatching INV_DEL etc. without active player | Pre-NAI-131 these would crash on nil-Self deref. New behavior: clean script abort. Strict improvement. |
| Adding `checkInvType` BEFORE `resolveInv` introduces a double-lookup of `s.Configs.InvType(typeID)` | Cosmetic — both lookups are O(1) array index. Don't optimize. |
| Wrapping `requireProtectedActivePlayer` error masks its inner literal | Tests only assert TS-shape substring. Preserved diagnostic value via `op` prefix. |
| TS `BOTH_MOVEINV` uses `state.intOperand`-driven secondary-player swap | Out of scope (no goscape impl); not a NAI-131 risk. |

## §7 — Plan-author notes (future writing-plans pass)

- Plan tasks should be ordered T1, T2, T3, T4, T5, T6 — sequential dependency only between T6 and the others.
- Each gate-adding task must be RED-then-GREEN: implementer writes failing tests for the new gate first, then adds the gate. Reusable pattern from NAI-130 T5.
- Pre-flight grep of handlers_inv_test.go for `&ScriptState{}` literal and `Init(` calls in INV_DEL/CLEAR/SETSLOT/MOVE* tests — these are the candidates for T6 fixture patches.
- Verify `objtype.InvTypeScopeShared` and `InvType.Protect/Scope/DummyInv` field names against HEAD before codifying (per `controller_preflight.md`). Confirmed at spec-write time (pkg/objtype/invtype.go:13, 18, 26, 28).
- Verify `ObjType.DebugName` field name (per `mock_recorder_field_naming_check.md`) — `pkg/objtype/objtype.go` uses `DebugName` (inherited from `ConfigType`).

## §8 — Closing criteria

PRIMARY met when:

1. All 7 handlers pass `go test ./pkg/script/... -race` with the new gate tests.
2. `vet` clean (`GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go vet ./...`).
3. Existing INV_* tests all still GREEN (T6 fixture patches landed where needed).
4. Goscape's `INV_ADD` error literals match TS for the 5 gates from NAI-130 §6.

No smoke required: this is a TS-fidelity polish sub-spec. Scripts that currently pass through to silent no-op behavior will now emit clean errors; any content that genuinely depended on the silent pass-through would surface in a future smoke as a script abort, not as silent corruption.
