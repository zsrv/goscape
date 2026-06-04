# NAI-131 INV_* write-handler TS validator sweep — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Port the TS validator chain (`InvTypeValid` / `ObjTypeValid` / `ObjStackValid` / protect-scope / dummyitem gates plus `requireActivePlayer`) across all 7 goscape INV write handlers to match `Engine-TS/src/engine/script/handlers/InvOps.ts`.

**Architecture:** Each handler gets the TS gate chain inserted between the existing `PopInt` calls and `inv.Add/Del/Set/Move/Clear` body. All needed validator helpers (`checkInvType`, `checkObjType`, `checkObjStack`, `requireActivePlayer`) already exist in `pkg/script/`. No new helpers; all edits land in `pkg/script/handlers_inv.go` (gates), `pkg/script/handlers_obj.go` (one-line `checkObjStack` upper-bound tighten), and matching `_test.go` files. TDD throughout.

**Tech Stack:** Go 1.26+

---

## Spec reference

`docs/superpowers/specs/2026-05-08-nai-131-inv-write-validators-design.md` (commit `26674f6`).

## File structure

| File | Role |
|---|---|
| `pkg/script/handlers_inv.go` | Add gates to the 7 write handlers (INV_ADD, INV_SETSLOT, INV_DEL, INV_DELSLOT, INV_CLEAR, INV_MOVEITEM, INV_MOVEFROMSLOT) |
| `pkg/script/handlers_obj.go` | Tighten `checkObjStack` to `1..StackLimit`; correct doc-comment |
| `pkg/script/handlers_inv_test.go` | New gate-coverage RED tests; `newTestInvConfigs()` fixture update; per-task fixture patches |

No new files.

## Pre-flight (controller, before dispatching T1)

The plan-author has verified against HEAD:

- `pkg/objtype/invtype.go:13`: `InvTypeScopeShared = 2` constant.
- `pkg/objtype/invtype.go:18,26,28`: `Scope int`, `Protect bool`, `DummyInv bool` fields.
- `pkg/objtype/objtype.go:133`: `DummyItem int` field on `ObjType`.
- `pkg/objtype/objtype.go` ConfigType embeds `DebugName string` — accessible as `invType.DebugName` / `objType.DebugName`.
- `pkg/inventory/inventory.go:13`: `StackLimit = 0x7fffffff`.
- `pkg/script/handlers_player.go:35,58,129`: `requireActivePlayer`, `requireProtectedActivePlayer`, `checkInvType`.
- `pkg/script/handlers_obj.go:20,29`: `checkObjType`, `checkObjStack`.
- `pkg/script/handlers_inv.go:14`: `resolveInv` signature and behavior.
- `pkg/script/handlers_inv_test.go:50` `newTestInvConfigs()` does **not** seed any InvType entries (only Obj/Param). All existing `runInvOp(...)` callers pass `mc` without InvTypes; T0 below adds the InvType seeds.
- `pkg/script/handlers_inv_test.go:97` `runInvOp` sets `Pointers |= PtrActivePlayer` and `Self = mockPlayer{}`.
- `pkg/script/handlers_inv_test.go:122` `runInvOpExpectErr` does NOT set `PtrActivePlayer` (Self is nil). Used by `TestInvLookupNilReturnsError` (line 389).

**Pop order** in goscape: lowest-stack item is popped LAST. TS `popInts(N)` returns left-to-right matching the source list. Goscape pops in reverse. Example: TS `[inv, obj, count] = popInts(3)` → goscape `count := PopInt(); obj := PopInt(); inv := PopInt()`.

**Direct-check protect/scope pattern** (used in all 7 handlers):

```go
if invType.Protect && invType.Scope != objtype.InvTypeScopeShared && !s.Protect {
    return fmt.Errorf("OP_NAME: $inv requires protected access: %s", invType.DebugName)
}
```

Reasoning: `requireActivePlayer` runs first in every handler (so `s.Self != nil`); the protect-gate then checks `s.Protect` directly. This matches TS literal `"$inv requires protected access: <debugname>"` (the inner `requireProtectedActivePlayer` would emit `"<op>: script not protected"`, divergent literal). Single-line, no error wrapping.

---

## Task 0: Test fixture prep — InvType seeds + gate-test helper

**Files:**
- Modify: `pkg/script/handlers_inv_test.go:50-92` (helper update)
- Add: new `runInvOpExpectErrAsPlayer` helper in `pkg/script/handlers_inv_test.go`

**Why this comes first:**

1. T1's new `checkInvType` gate would RED-fail every existing GREEN test that uses `runInvOp(t, OpInv*, ...)` because `newTestInvConfigs()` returns a `mockConfigs` with `invs: make(map[int]*objtype.InvType)` — empty. Adding the InvType seeds upfront keeps existing GREEN tests GREEN once the gate lands. Behavior-neutral pre-T1 (no code reads the InvType yet).

2. The existing `runInvOpExpectErr` helper calls `Init(sf, nil, false, nil, nil)` — nil Self, no `PtrActivePlayer`. After T1-T4 add `requireActivePlayer` gates, every test using this helper will fail at the first gate emitting `"<op>: no active player"` regardless of which deeper gate the test intends to exercise. We need a sibling helper that sets up an active player so subsequent gate tests reach the gates they target.

- [ ] **Step 1: Read the helper at handlers_inv_test.go:50-92.** Confirm it returns `mc *mockConfigs` with empty `invs` map.

- [ ] **Step 2: Add InvType seeds for `testInvMain` and `testInvBank`.**

Edit `pkg/script/handlers_inv_test.go` — inside `newTestInvConfigs()`, after the existing `sword` seed (around line 89, before `return mc`):

```go
	mainInv := objtype.NewInvType(testInvMain)
	mainInv.DebugName = "main"
	mainInv.Size = 28
	mainInv.Scope = objtype.InvTypeScopeTemp
	mainInv.Protect = false // turn off the NewInvType default so existing tests don't trip the protect/scope gate
	mc.invs[testInvMain] = mainInv

	bankInv := objtype.NewInvType(testInvBank)
	bankInv.DebugName = "bank"
	bankInv.Size = 100
	bankInv.Scope = objtype.InvTypeScopeShared
	bankInv.Protect = false
	mc.invs[testInvBank] = bankInv

	return mc
```

Note: `objtype.NewInvType(id)` defaults `Protect = true` (per pkg/objtype/invtype.go:83). We override to `false` so existing tests using `runInvOp` (which doesn't set `s.Protect`) don't trip the new protect/scope gate landing in T1-T4. Tests that intentionally exercise the protect/scope gate will set `Protect = true` locally on a per-test basis.

- [ ] **Step 3: Update the helper doc-comment (line 45-49) to list the new InvType seeds.**

```go
// newTestInvConfigs builds a mockConfigs seeded with the inventory
// fixture types used across this file:
//   - obj 995 "coins": stackable, category 10, params[1] = 5
//   - obj 2   "arrow": stackable, category 20, params[1] = 0
//   - obj 3   "sword": non-stackable, category 10
//   - param 1: int, default 0
//   - inv 1 "main": size 28, scope TEMP, protect=false
//   - inv 2 "bank": size 100, scope SHARED, protect=false
func newTestInvConfigs() *mockConfigs {
```

- [ ] **Step 4: Add the gate-test helper `runInvOpExpectErrAsPlayer`.**

Append to `pkg/script/handlers_inv_test.go` immediately after the existing `runInvOpExpectErr` (around line 145):

```go
// runInvOpExpectErrAsPlayer is the active-player variant of
// runInvOpExpectErr. Sets up a zero-value mockPlayer + PtrActivePlayer
// so the requireActivePlayer gate is satisfied; tests targeting deeper
// gates (InvTypeValid / ObjTypeValid / ObjStackValid / protect-scope /
// dummyitem) use this helper. s.Protect remains false (Init's third
// arg) — tests that need a protected script set state.Protect = true
// before pushing inputs.
func runInvOpExpectErrAsPlayer(t *testing.T, op Opcode, intInputs []int, lookup InvLookup, configs Configs, substr string) {
	t.Helper()
	sf := &ScriptFile{
		Name:             "test_" + op.String(),
		Opcodes:          []Opcode{op, OpReturn},
		IntOperands:      []int32{0, 0},
		StringOperands:   []string{"", ""},
		InstructionCount: 2,
	}
	mp := &mockPlayer{}
	state := Init(sf, mp, false, nil, nil)
	state.Pointers |= PtrActivePlayer
	state.Inv = lookup
	state.Configs = configs
	for _, v := range intInputs {
		state.PushInt(v)
	}
	err := Execute(state)
	if err == nil {
		t.Fatalf("%s: expected error containing %q, got nil", op.String(), substr)
	}
	if !strings.Contains(err.Error(), substr) {
		t.Fatalf("%s: expected error containing %q, got %q", op.String(), substr, err.Error())
	}
}
```

- [ ] **Step 5: Run the package suite — must remain GREEN.**

```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/... -count=1
```

Expected: PASS. No behavior change yet (no code reads the new InvType entries; new helper has no callers).

- [ ] **Step 6: Commit.**

```
git add pkg/script/handlers_inv_test.go
git commit --no-gpg-sign -m "test(nai-131): T0 — InvType seeds + runInvOpExpectErrAsPlayer test helper"
```

---

## Task 1: `handleInvAdd` 5 gates

**Files:**
- Modify: `pkg/script/handlers_inv.go:308-345` (`handleInvAdd` body)
- Modify: `pkg/script/handlers_inv_test.go` (add 7 new tests; patch `TestInvLookupNilReturnsError` line 389-394)

**TS reference:** `Engine-TS/src/engine/script/handlers/InvOps.ts:57-83`.

**Gates added (TS order):** `InvTypeValid` → `ObjTypeValid` → `ObjStackValid` → protect/scope → dummyitem.

- [ ] **Step 1: Write 7 RED tests for the new INV_ADD gates.**

Append to `pkg/script/handlers_inv_test.go` (after the existing NAI-130 INV_ADD overflow tests):

```go
// -- NAI-131 INV_ADD validator tests --

// (T1.1) InvTypeValid: passing an inv id not registered in s.Configs
// triggers checkInvType before any inv lookup, mirroring TS check(inv,
// InvTypeValid). Asserts TS-shaped error literal.
func TestInvAdd_InvTypeValid_UnregisteredID(t *testing.T) {
	lookup := newTestInvLookup()
	mc := newTestInvConfigs()
	delete(mc.invs, testInvMain)
	runInvOpExpectErrAsPlayer(t, OpInvAdd, []int{testInvMain, testObjCoin, 1}, lookup, mc, "no InvType with value (1) found")
}

// (T1.2) ObjTypeValid: passing an obj id not registered triggers
// checkObjType. Mirrors TS check(objId, ObjTypeValid).
func TestInvAdd_ObjTypeValid_UnregisteredID(t *testing.T) {
	lookup := newTestInvLookup()
	mc := newTestInvConfigs()
	delete(mc.objs, testObjCoin)
	runInvOpExpectErrAsPlayer(t, OpInvAdd, []int{testInvMain, testObjCoin, 1}, lookup, mc, "no ObjType with value (995) found")
}

// (T1.3) ObjStackValid: count == 0 (TS: ScriptInputRangeValidator min=1).
func TestInvAdd_ObjStackValid_CountZero(t *testing.T) {
	lookup := newTestInvLookup()
	mc := newTestInvConfigs()
	runInvOpExpectErrAsPlayer(t, OpInvAdd, []int{testInvMain, testObjCoin, 0}, lookup, mc, "invalid count (0)")
}

// (T1.4) ObjStackValid: count == -1 (TS rejects below min=1).
func TestInvAdd_ObjStackValid_CountNegative(t *testing.T) {
	lookup := newTestInvLookup()
	mc := newTestInvConfigs()
	runInvOpExpectErrAsPlayer(t, OpInvAdd, []int{testInvMain, testObjCoin, -1}, lookup, mc, "invalid count (-1)")
}

// (T1.5) Protect+TEMP scope rejects unprotected script. The fixture
// invType "main" defaults Scope=TEMP so a Protect=true override here
// triggers the gate.
func TestInvAdd_ProtectGate_RejectsUnprotected(t *testing.T) {
	lookup := newTestInvLookup()
	mc := newTestInvConfigs()
	mc.invs[testInvMain].Protect = true // scope is TEMP from T0 default
	runInvOpExpectErrAsPlayer(t, OpInvAdd, []int{testInvMain, testObjCoin, 1}, lookup, mc, "$inv requires protected access: main")
}

// (T1.6) Protect+SHARED scope is the TS escape hatch — no protected
// access required even when Protect=true.
func TestInvAdd_ProtectGate_SharedScopeEscapeHatch(t *testing.T) {
	lookup := newTestInvLookup()
	mc := newTestInvConfigs()
	mc.invs[testInvBank].Protect = true // bank is SHARED scope per T0
	// No error expected: SHARED scope skips the protect gate.
	runInvOp(t, OpInvAdd, []int{testInvBank, testObjCoin, 1}, lookup, mc)
}

// (T1.7) Dummy-item gate: non-DummyInv inv + ObjType.DummyItem != 0
// rejects with TS-shaped literal.
func TestInvAdd_DummyItemGate_RejectsDummyItemInRegularInv(t *testing.T) {
	lookup := newTestInvLookup()
	mc := newTestInvConfigs()
	mc.objs[testObjCoin].DummyItem = 1 // make coins a dummy item; main is not a dummy inv
	runInvOpExpectErrAsPlayer(t, OpInvAdd, []int{testInvMain, testObjCoin, 1}, lookup, mc, "dummyitem in non-dummyinv: coins -> main")
}
```

- [ ] **Step 2: Run the new tests — expect RED.**

```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/ -run "TestInvAdd_(InvTypeValid|ObjTypeValid|ObjStackValid|ProtectGate|DummyItemGate)" -count=1 -v
```

Expected: All 7 tests FAIL — handler currently silently passes through bad inputs (or returns the existing "no inv for type N" literal which doesn't match the new substring).

- [ ] **Step 3: Implement the 5 gates in `handleInvAdd`.**

Replace `pkg/script/handlers_inv.go:308-345` (`handleInvAdd` function body) with:

```go
func handleInvAdd(s *ScriptState) error {
	if err := requireActivePlayer(s, "INV_ADD"); err != nil {
		return err
	}
	count := s.PopInt()
	obj := s.PopInt()
	typeID := s.PopInt()

	// TS InvOps.ts:60-62 — InvTypeValid, ObjTypeValid, ObjStackValid.
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

	// TS InvOps.ts:64-66 — protect/scope gate.
	if invType.Protect && invType.Scope != objtype.InvTypeScopeShared && !s.Protect {
		return fmt.Errorf("INV_ADD: $inv requires protected access: %s", invType.DebugName)
	}

	// TS InvOps.ts:68-70 — dummyitem-in-non-dummyinv gate.
	if !invType.DummyInv && objType.DummyItem != 0 {
		return fmt.Errorf("INV_ADD: dummyitem in non-dummyinv: %s -> %s", objType.DebugName, invType.DebugName)
	}

	inv := resolveInv(s, typeID)
	if inv == nil {
		// Defensive: unreachable post-checkInvType for valid configs;
		// retained for the InvLookup-unset case (s.Inv == nil → resolveInv returns nil).
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
		level := (s.Self.CoordPacked() >> 28) & 0x3
		x := s.Self.X()
		z := s.Self.Z()
		receiverID := s.Self.UID()
		if !stackable || overflow == 1 {
			for range overflow {
				s.World.AddObj(level, x, z, obj, 1, 200, receiverID)
			}
		} else {
			s.World.AddObj(level, x, z, obj, overflow, 200, receiverID)
		}
	}

	return nil
}
```

- [ ] **Step 4: Update the doc-comment above `handleInvAdd` (handlers_inv.go:293-307) to list the 5 new gates and reference NAI-131.**

Replace the existing doc-comment block:

```go
// handleInvAdd ports TS InvOps.ts:57-83 (INV_ADD, opcode 4302). Pops
// [inv, obj, count]; validates each via TS check chain (InvTypeValid,
// ObjTypeValid, ObjStackValid), enforces the protect/scope gate, and
// rejects dummy items in non-dummy invs. Adds count units of obj to
// the inv via Inventory.Add with caller-precomputed Stackable/StockObj
// flags. Per TS, any overflow drops to the world at the player's tile
// via World.AddObj — branched on (!stackable || overflow == 1) for the
// per-unit-loop case vs the single-stack-drop case (TS InvOps.ts:73-82,
// duration=200).
//
// Validator chain (NAI-131): InvTypeValid → ObjTypeValid → ObjStackValid
// → protect/scope (rejects unprotected scripts when invType.Protect &&
// scope != SHARED) → dummyitem (rejects ObjType.DummyItem != 0 when
// invType.DummyInv == false). All 5 gates throw in TS; goscape returns
// errors with TS-shaped literals.
//
// DEVIATION-NAI-130-D2: defensive nil-World guard skips the overflow
// drop when s.World is unset (goscape defensive; TS uses static World
// import which is never null). Per defensive_gate_doc_comment_label.
//
// DEVIATION-NAI-130-D3: defensive nil-Configs fallback in
// lookupStackableStockObj retained for sibling callers (handleInvMoveItem
// etc.); INV_ADD itself is now ObjType-validated before the helper
// runs, making the fallback unreachable on this path. The defensive
// fallback stays for the sibling Move handlers (NAI-131 T4).
```

- [ ] **Step 5: Run the new tests — expect GREEN. Run the full pkg/script suite.**

```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/ -run "TestInvAdd_" -count=1 -v
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/... -count=1
```

Expected: All TestInvAdd_* GREEN. Full suite GREEN. If any pre-existing test goes RED, it indicates a fixture in this file or a sibling _test.go that constructs a `mockConfigs` without InvType seeds and uses INV_ADD; patch it inline (the audit shows none should exist after T0).

- [ ] **Step 6: Patch `TestInvLookupNilReturnsError` (handlers_inv_test.go:389-394).**

The test currently asserts `OpInvAdd` errors with `"no active player"` (already satisfied by NAI-130 D4 — keep). The `OpInvClear` and `OpInvTotal` lines are unchanged in T1. No edit needed in T1; T3 patches `OpInvClear`.

- [ ] **Step 7: Commit.**

```
git add pkg/script/handlers_inv.go pkg/script/handlers_inv_test.go
git commit --no-gpg-sign -m "feat(nai-131): T1 — handleInvAdd 5 TS validators (GREEN)"
```

---

## Task 2: `handleInvSetSlot` 5 gates

**Files:**
- Modify: `pkg/script/handlers_inv.go:399-410` (`handleInvSetSlot` body)
- Modify: `pkg/script/handlers_inv_test.go` (add 7 new tests)

**TS reference:** `InvOps.ts:600-616`. Same 5 gates as INV_ADD including dummyitem.

**Pop order:** TS `popInts(4)` returns `[inv, slot, objId, count]`. Goscape pops in reverse: `count, objID, slot, typeID`.

- [ ] **Step 1: Write 7 RED tests, mirroring T1's pattern with INV_SETSLOT.**

Append to `pkg/script/handlers_inv_test.go`:

```go
// -- NAI-131 INV_SETSLOT validator tests --

func TestInvSetSlot_InvTypeValid_UnregisteredID(t *testing.T) {
	lookup := newTestInvLookup()
	mc := newTestInvConfigs()
	delete(mc.invs, testInvMain)
	runInvOpExpectErrAsPlayer(t, OpInvSetSlot, []int{testInvMain, 0, testObjCoin, 1}, lookup, mc, "no InvType with value (1) found")
}

func TestInvSetSlot_ObjTypeValid_UnregisteredID(t *testing.T) {
	lookup := newTestInvLookup()
	mc := newTestInvConfigs()
	delete(mc.objs, testObjCoin)
	runInvOpExpectErrAsPlayer(t, OpInvSetSlot, []int{testInvMain, 0, testObjCoin, 1}, lookup, mc, "no ObjType with value (995) found")
}

func TestInvSetSlot_ObjStackValid_CountZero(t *testing.T) {
	lookup := newTestInvLookup()
	mc := newTestInvConfigs()
	runInvOpExpectErrAsPlayer(t, OpInvSetSlot, []int{testInvMain, 0, testObjCoin, 0}, lookup, mc, "invalid count (0)")
}

func TestInvSetSlot_ObjStackValid_CountNegative(t *testing.T) {
	lookup := newTestInvLookup()
	mc := newTestInvConfigs()
	runInvOpExpectErrAsPlayer(t, OpInvSetSlot, []int{testInvMain, 0, testObjCoin, -1}, lookup, mc, "invalid count (-1)")
}

func TestInvSetSlot_ProtectGate_RejectsUnprotected(t *testing.T) {
	lookup := newTestInvLookup()
	mc := newTestInvConfigs()
	mc.invs[testInvMain].Protect = true
	runInvOpExpectErrAsPlayer(t, OpInvSetSlot, []int{testInvMain, 0, testObjCoin, 1}, lookup, mc, "$inv requires protected access: main")
}

func TestInvSetSlot_ProtectGate_SharedScopeEscapeHatch(t *testing.T) {
	lookup := newTestInvLookup()
	mc := newTestInvConfigs()
	mc.invs[testInvBank].Protect = true
	runInvOp(t, OpInvSetSlot, []int{testInvBank, 0, testObjCoin, 1}, lookup, mc)
}

func TestInvSetSlot_DummyItemGate_RejectsDummyItemInRegularInv(t *testing.T) {
	lookup := newTestInvLookup()
	mc := newTestInvConfigs()
	mc.objs[testObjCoin].DummyItem = 1
	runInvOpExpectErrAsPlayer(t, OpInvSetSlot, []int{testInvMain, 0, testObjCoin, 1}, lookup, mc, "dummyitem in non-dummyinv: coins -> main")
}
```

- [ ] **Step 2: Run the new tests — expect RED.**

```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/ -run "TestInvSetSlot_" -count=1 -v
```

Expected: All 7 FAIL.

- [ ] **Step 3: Replace `handleInvSetSlot` (handlers_inv.go:399-410) with the gated version.**

```go
// handleInvSetSlot (INV_SETSLOT) ports TS InvOps.ts:600-616. Pops
// [inv, slot, obj, count] (popInts(4) order — count on top). Validates
// via TS check chain, enforces protect/scope and dummyitem gates, then
// replaces the slot with {obj, count}. Out-of-range slot is silently
// ignored by inv.Set (matches TS Inventory.set behavior).
//
// Validator chain (NAI-131): InvTypeValid → ObjTypeValid → ObjStackValid
// → protect/scope → dummyitem.
func handleInvSetSlot(s *ScriptState) error {
	if err := requireActivePlayer(s, "INV_SETSLOT"); err != nil {
		return err
	}
	count := s.PopInt()
	obj := s.PopInt()
	slot := s.PopInt()
	typeID := s.PopInt()

	if err := checkInvType(s, typeID, "INV_SETSLOT"); err != nil {
		return err
	}
	if err := checkObjType(s, obj, "INV_SETSLOT"); err != nil {
		return err
	}
	if err := checkObjStack(count, "INV_SETSLOT"); err != nil {
		return err
	}

	invType := s.Configs.InvType(typeID)
	objType := s.Configs.ObjType(obj)

	if invType.Protect && invType.Scope != objtype.InvTypeScopeShared && !s.Protect {
		return fmt.Errorf("INV_SETSLOT: $inv requires protected access: %s", invType.DebugName)
	}

	if !invType.DummyInv && objType.DummyItem != 0 {
		return fmt.Errorf("INV_SETSLOT: dummyitem in non-dummyinv: %s -> %s", objType.DebugName, invType.DebugName)
	}

	inv := resolveInv(s, typeID)
	if inv == nil {
		return fmt.Errorf("INV_SETSLOT: no inv for type %d", typeID)
	}
	inv.Set(slot, &inventory.Item{Id: obj, Count: count})
	return nil
}
```

- [ ] **Step 4: Run the new tests — expect GREEN. Run the full pkg/script suite.**

```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/ -run "TestInvSetSlot_" -count=1 -v
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/... -count=1
```

Expected: All TestInvSetSlot_* GREEN. Full suite GREEN.

- [ ] **Step 5: Commit.**

```
git add pkg/script/handlers_inv.go pkg/script/handlers_inv_test.go
git commit --no-gpg-sign -m "feat(nai-131): T2 — handleInvSetSlot 5 TS validators (GREEN)"
```

---

## Task 3: `handleInvDel` / `handleInvDelSlot` / `handleInvClear`

**Files:**
- Modify: `pkg/script/handlers_inv.go:371-394` (Del + DelSlot), `:413-421` (Clear)
- Modify: `pkg/script/handlers_inv_test.go` (new tests + patch line 392-394 `TestInvLookupNilReturnsError`)

**TS references:** `InvOps.ts:129-141` (DEL), `:144-159` (DELSLOT), `:116-124` (CLEAR).

**Gate sets:**
- INV_DEL: `requireActivePlayer` + InvTypeValid + ObjTypeValid + ObjStackValid + protect/scope. No dummyitem.
- INV_DELSLOT: `requireActivePlayer` + InvTypeValid + protect/scope. No Obj/Stack.
- INV_CLEAR: `requireActivePlayer` + InvTypeValid + protect/scope. No Obj/Stack.

**Pop orders:**
- INV_DEL: TS `popInts(3)` = `[inv, obj, count]`. Goscape: `count, obj, typeID`.
- INV_DELSLOT: TS `popInts(2)` = `[inv, slot]`. Goscape: `slot, typeID`.
- INV_CLEAR: TS `popInt()` = inv. Goscape: `typeID`.

- [ ] **Step 1: Write RED tests for the 3 handlers (one per gate, plus shared-scope escape).**

Append to `pkg/script/handlers_inv_test.go`:

```go
// -- NAI-131 INV_DEL / INV_DELSLOT / INV_CLEAR validator tests --

// (T3.A) INV_DEL — full Obj-gate set without dummyitem.
func TestInvDel_InvTypeValid_UnregisteredID(t *testing.T) {
	lookup := newTestInvLookup()
	mc := newTestInvConfigs()
	delete(mc.invs, testInvMain)
	runInvOpExpectErrAsPlayer(t, OpInvDel, []int{testInvMain, testObjCoin, 1}, lookup, mc, "no InvType with value (1) found")
}

func TestInvDel_ObjTypeValid_UnregisteredID(t *testing.T) {
	lookup := newTestInvLookup()
	mc := newTestInvConfigs()
	delete(mc.objs, testObjCoin)
	runInvOpExpectErrAsPlayer(t, OpInvDel, []int{testInvMain, testObjCoin, 1}, lookup, mc, "no ObjType with value (995) found")
}

func TestInvDel_ObjStackValid_CountZero(t *testing.T) {
	lookup := newTestInvLookup()
	mc := newTestInvConfigs()
	runInvOpExpectErrAsPlayer(t, OpInvDel, []int{testInvMain, testObjCoin, 0}, lookup, mc, "invalid count (0)")
}

func TestInvDel_ProtectGate_RejectsUnprotected(t *testing.T) {
	lookup := newTestInvLookup()
	mc := newTestInvConfigs()
	mc.invs[testInvMain].Protect = true
	runInvOpExpectErrAsPlayer(t, OpInvDel, []int{testInvMain, testObjCoin, 1}, lookup, mc, "$inv requires protected access: main")
}

func TestInvDel_NoActivePlayerErrors(t *testing.T) {
	mc := newTestInvConfigs()
	runInvOpExpectErr(t, OpInvDel, []int{testInvMain, testObjCoin, 1}, nil, mc, "INV_DEL: no active player")
}

// (T3.B) INV_DELSLOT — InvTypeValid + protect/scope only.
func TestInvDelSlot_InvTypeValid_UnregisteredID(t *testing.T) {
	lookup := newTestInvLookup()
	mc := newTestInvConfigs()
	delete(mc.invs, testInvMain)
	runInvOpExpectErrAsPlayer(t, OpInvDelSlot, []int{testInvMain, 0}, lookup, mc, "no InvType with value (1) found")
}

func TestInvDelSlot_ProtectGate_RejectsUnprotected(t *testing.T) {
	lookup := newTestInvLookup()
	mc := newTestInvConfigs()
	mc.invs[testInvMain].Protect = true
	runInvOpExpectErrAsPlayer(t, OpInvDelSlot, []int{testInvMain, 0}, lookup, mc, "$inv requires protected access: main")
}

func TestInvDelSlot_NoActivePlayerErrors(t *testing.T) {
	mc := newTestInvConfigs()
	runInvOpExpectErr(t, OpInvDelSlot, []int{testInvMain, 0}, nil, mc, "INV_DELSLOT: no active player")
}

// (T3.C) INV_CLEAR — InvTypeValid + protect/scope only.
func TestInvClear_InvTypeValid_UnregisteredID(t *testing.T) {
	lookup := newTestInvLookup()
	mc := newTestInvConfigs()
	delete(mc.invs, testInvMain)
	runInvOpExpectErrAsPlayer(t, OpInvClear, []int{testInvMain}, lookup, mc, "no InvType with value (1) found")
}

func TestInvClear_ProtectGate_RejectsUnprotected(t *testing.T) {
	lookup := newTestInvLookup()
	mc := newTestInvConfigs()
	mc.invs[testInvMain].Protect = true
	runInvOpExpectErrAsPlayer(t, OpInvClear, []int{testInvMain}, lookup, mc, "$inv requires protected access: main")
}

func TestInvClear_NoActivePlayerErrors(t *testing.T) {
	mc := newTestInvConfigs()
	runInvOpExpectErr(t, OpInvClear, []int{testInvMain}, nil, mc, "INV_CLEAR: no active player")
}
```

- [ ] **Step 2: Patch the existing `TestInvLookupNilReturnsError` (handlers_inv_test.go:389-394).**

The existing test asserts `OpInvClear` errors with "no inv for type" and `OpInvTotal` with "no inv for type". After T3, INV_CLEAR will error earlier with "no active player". Update only the OpInvClear assertion:

Replace `pkg/script/handlers_inv_test.go:394`:

```go
	runInvOpExpectErr(t, OpInvClear, []int{testInvMain}, nil, mc, "no active player")
```

(Was: `... "no inv for type"`.) Reason: post-T3 the `requireActivePlayer` gate fires before `resolveInv`. Test purpose ("no Inv lookup → error") is satisfied; the literal asserts the new gate.

OpInvTotal (line 392) is a read handler — out of scope, not modified. Keep its assertion as-is.

- [ ] **Step 3: Run the new tests — expect RED.**

```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/ -run "TestInv(Del|DelSlot|Clear)_" -count=1 -v
```

Expected: All 11 new tests FAIL. `TestInvLookupNilReturnsError` will also fail until T3 implementation lands.

- [ ] **Step 4: Implement gates in all 3 handlers.**

Replace `pkg/script/handlers_inv.go:371-394` (handleInvDel + handleInvDelSlot) with:

```go
// handleInvDel (INV_DEL) ports TS InvOps.ts:129-141. Pops [inv, obj,
// count] and removes count units of obj from the inv. Validates via
// TS check chain (no dummyitem gate — TS doesn't apply it on DEL).
func handleInvDel(s *ScriptState) error {
	if err := requireActivePlayer(s, "INV_DEL"); err != nil {
		return err
	}
	count := s.PopInt()
	obj := s.PopInt()
	typeID := s.PopInt()

	if err := checkInvType(s, typeID, "INV_DEL"); err != nil {
		return err
	}
	if err := checkObjType(s, obj, "INV_DEL"); err != nil {
		return err
	}
	if err := checkObjStack(count, "INV_DEL"); err != nil {
		return err
	}

	invType := s.Configs.InvType(typeID)
	if invType.Protect && invType.Scope != objtype.InvTypeScopeShared && !s.Protect {
		return fmt.Errorf("INV_DEL: $inv requires protected access: %s", invType.DebugName)
	}

	inv := resolveInv(s, typeID)
	if inv == nil {
		return fmt.Errorf("INV_DEL: no inv for type %d", typeID)
	}
	inv.Remove(obj, count, inventory.RemoveOpts{BeginSlot: -1})
	return nil
}

// handleInvDelSlot (INV_DELSLOT) ports TS InvOps.ts:144-159. Pops
// [inv, slot] and clears that slot. Out-of-range slots are silently
// ignored by inventory.Delete (matches TS).
//
// Validator chain (NAI-131): InvTypeValid → protect/scope.
func handleInvDelSlot(s *ScriptState) error {
	if err := requireActivePlayer(s, "INV_DELSLOT"); err != nil {
		return err
	}
	slot := s.PopInt()
	typeID := s.PopInt()

	if err := checkInvType(s, typeID, "INV_DELSLOT"); err != nil {
		return err
	}

	invType := s.Configs.InvType(typeID)
	if invType.Protect && invType.Scope != objtype.InvTypeScopeShared && !s.Protect {
		return fmt.Errorf("INV_DELSLOT: $inv requires protected access: %s", invType.DebugName)
	}

	inv := resolveInv(s, typeID)
	if inv == nil {
		return fmt.Errorf("INV_DELSLOT: no inv for type %d", typeID)
	}
	inv.Delete(slot)
	return nil
}
```

Replace `pkg/script/handlers_inv.go:413-421` (handleInvClear) with:

```go
// handleInvClear (INV_CLEAR) ports TS InvOps.ts:116-124. Pops an inv
// id and empties every slot.
//
// Validator chain (NAI-131): InvTypeValid → protect/scope.
func handleInvClear(s *ScriptState) error {
	if err := requireActivePlayer(s, "INV_CLEAR"); err != nil {
		return err
	}
	typeID := s.PopInt()

	if err := checkInvType(s, typeID, "INV_CLEAR"); err != nil {
		return err
	}

	invType := s.Configs.InvType(typeID)
	if invType.Protect && invType.Scope != objtype.InvTypeScopeShared && !s.Protect {
		return fmt.Errorf("INV_CLEAR: $inv requires protected access: %s", invType.DebugName)
	}

	inv := resolveInv(s, typeID)
	if inv == nil {
		return fmt.Errorf("INV_CLEAR: no inv for type %d", typeID)
	}
	inv.Clear()
	return nil
}
```

- [ ] **Step 5: Run new + patched tests — expect GREEN. Run the full pkg/script suite.**

```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/ -run "TestInv(Del|DelSlot|Clear|LookupNil)" -count=1 -v
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/... -count=1
```

Expected: All targeted tests GREEN; full suite GREEN.

- [ ] **Step 6: Commit.**

```
git add pkg/script/handlers_inv.go pkg/script/handlers_inv_test.go
git commit --no-gpg-sign -m "feat(nai-131): T3 — INV_DEL/DELSLOT/CLEAR validator gates (GREEN)"
```

---

## Task 4: `handleInvMoveItem` / `handleInvMoveFromSlot` (dual-inv gates)

**Files:**
- Modify: `pkg/script/handlers_inv.go:427-481` (both handlers)
- Modify: `pkg/script/handlers_inv_test.go` (new tests)

**TS references:** `InvOps.ts:499-531` (MOVEITEM), `:323-349` (MOVEFROMSLOT).

**Gate sets:**
- INV_MOVEITEM: `requireActivePlayer` + 2× InvTypeValid + ObjTypeValid + ObjStackValid + 2× protect/scope. No dummyitem.
- INV_MOVEFROMSLOT: `requireActivePlayer` + 2× InvTypeValid + 2× protect/scope. No Obj/Stack/dummyitem.

**Pop orders:**
- INV_MOVEITEM: TS `popInts(4)` = `[fromInv, toInv, obj, count]`. Goscape: `count, obj, toTypeID, fromTypeID`.
- INV_MOVEFROMSLOT: TS `popInts(3)` = `[fromInv, toInv, fromSlot]`. Goscape: `fromSlot, toTypeID, fromTypeID`.

**TS asymmetry to preserve (DEVIATION-NAI-131-D1):** Both protect/scope checks in TS use `fromInvType.scope !== SCOPE_SHARED` — TS never consults `toInvType.scope`. Goscape mirrors this. Pin both presence and absence per `ts_asymmetry_dual_pin.md`.

- [ ] **Step 1: Write RED tests for both handlers.**

Append to `pkg/script/handlers_inv_test.go`:

```go
// -- NAI-131 INV_MOVEITEM / INV_MOVEFROMSLOT validator tests --

// (T4.A) INV_MOVEITEM — full Obj-gate set + 2 inv gates.
func TestInvMoveItem_FromInvTypeValid_UnregisteredID(t *testing.T) {
	lookup := newTestInvLookup()
	mc := newTestInvConfigs()
	delete(mc.invs, testInvMain)
	runInvOpExpectErrAsPlayer(t, OpInvMoveItem, []int{testInvMain, testInvBank, testObjCoin, 1}, lookup, mc, "no InvType with value (1) found")
}

func TestInvMoveItem_ToInvTypeValid_UnregisteredID(t *testing.T) {
	lookup := newTestInvLookup()
	mc := newTestInvConfigs()
	delete(mc.invs, testInvBank)
	runInvOpExpectErrAsPlayer(t, OpInvMoveItem, []int{testInvMain, testInvBank, testObjCoin, 1}, lookup, mc, "no InvType with value (2) found")
}

func TestInvMoveItem_ObjTypeValid_UnregisteredID(t *testing.T) {
	lookup := newTestInvLookup()
	mc := newTestInvConfigs()
	delete(mc.objs, testObjCoin)
	runInvOpExpectErrAsPlayer(t, OpInvMoveItem, []int{testInvMain, testInvBank, testObjCoin, 1}, lookup, mc, "no ObjType with value (995) found")
}

func TestInvMoveItem_ObjStackValid_CountZero(t *testing.T) {
	lookup := newTestInvLookup()
	mc := newTestInvConfigs()
	runInvOpExpectErrAsPlayer(t, OpInvMoveItem, []int{testInvMain, testInvBank, testObjCoin, 0}, lookup, mc, "invalid count (0)")
}

// Protect gate fires on the FROM inv (TS InvOps.ts:507-509).
func TestInvMoveItem_FromProtectGate_RejectsUnprotected(t *testing.T) {
	lookup := newTestInvLookup()
	mc := newTestInvConfigs()
	mc.invs[testInvMain].Protect = true // from=main, scope=TEMP
	runInvOpExpectErrAsPlayer(t, OpInvMoveItem, []int{testInvMain, testInvBank, testObjCoin, 1}, lookup, mc, "$inv requires protected access: main")
}

// Protect gate fires on the TO inv (TS InvOps.ts:511-513) BUT TS
// preserves the asymmetry: the SHARED-escape check uses fromInv's
// scope, not toInv's. So when from=SHARED, the TO gate's escape hatch
// fires regardless of toInv's scope — even if toInv.Protect=true.
// DEVIATION-NAI-131-D1.
func TestInvMoveItem_ToProtectGate_RejectsUnprotected(t *testing.T) {
	lookup := newTestInvLookup()
	mc := newTestInvConfigs()
	// from=main (TEMP, Protect=false default), to=bank (SHARED, Protect=true override)
	mc.invs[testInvBank].Protect = true
	// from is TEMP (not SHARED) — no TS escape hatch — both gates check fromInv.scope=TEMP.
	// So TO gate fires.
	runInvOpExpectErrAsPlayer(t, OpInvMoveItem, []int{testInvMain, testInvBank, testObjCoin, 1}, lookup, mc, "$inv requires protected access: bank")
}

// TS asymmetry pin (DEVIATION-NAI-131-D1): when fromInv.scope == SHARED,
// BOTH gates' escape hatches fire because both check fromInv.scope.
// toInv's own scope is irrelevant.
func TestInvMoveItem_TSAsymmetry_FromSharedSkipsBothGates(t *testing.T) {
	lookup := newTestInvLookup()
	mc := newTestInvConfigs()
	// Swap roles: pretend bank is "from" (SHARED) and main is "to" (TEMP).
	mc.invs[testInvMain].Protect = true // would normally trigger TO gate
	// But fromInv (bank) is SHARED → TS asymmetry skips both gates.
	runInvOp(t, OpInvMoveItem, []int{testInvBank, testInvMain, testObjCoin, 1}, lookup, mc)
}

func TestInvMoveItem_NoActivePlayerErrors(t *testing.T) {
	mc := newTestInvConfigs()
	runInvOpExpectErr(t, OpInvMoveItem, []int{testInvMain, testInvBank, testObjCoin, 1}, nil, mc, "INV_MOVEITEM: no active player")
}

// (T4.B) INV_MOVEFROMSLOT — 2 inv gates + 2 protect gates only.
func TestInvMoveFromSlot_FromInvTypeValid_UnregisteredID(t *testing.T) {
	lookup := newTestInvLookup()
	mc := newTestInvConfigs()
	delete(mc.invs, testInvMain)
	runInvOpExpectErrAsPlayer(t, OpInvMoveFromSlot, []int{testInvMain, testInvBank, 0}, lookup, mc, "no InvType with value (1) found")
}

func TestInvMoveFromSlot_ToInvTypeValid_UnregisteredID(t *testing.T) {
	lookup := newTestInvLookup()
	mc := newTestInvConfigs()
	delete(mc.invs, testInvBank)
	runInvOpExpectErrAsPlayer(t, OpInvMoveFromSlot, []int{testInvMain, testInvBank, 0}, lookup, mc, "no InvType with value (2) found")
}

func TestInvMoveFromSlot_FromProtectGate_RejectsUnprotected(t *testing.T) {
	lookup := newTestInvLookup()
	mc := newTestInvConfigs()
	mc.invs[testInvMain].Protect = true
	// Pre-fill source slot so the test reaches the gate before the empty-slot error.
	lookup.invs[testInvMain].Items[0] = &inventory.Item{Id: testObjCoin, Count: 1}
	runInvOpExpectErrAsPlayer(t, OpInvMoveFromSlot, []int{testInvMain, testInvBank, 0}, lookup, mc, "$inv requires protected access: main")
}

func TestInvMoveFromSlot_NoActivePlayerErrors(t *testing.T) {
	mc := newTestInvConfigs()
	runInvOpExpectErr(t, OpInvMoveFromSlot, []int{testInvMain, testInvBank, 0}, nil, mc, "INV_MOVEFROMSLOT: no active player")
}
```

- [ ] **Step 2: Run the new tests — expect RED.**

```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/ -run "TestInvMove(Item|FromSlot)_" -count=1 -v
```

Expected: All 12 FAIL.

- [ ] **Step 3: Implement gates in both handlers.**

Replace `pkg/script/handlers_inv.go:427-481` (`handleInvMoveItem` + `handleInvMoveFromSlot`) with:

```go
// handleInvMoveItem (INV_MOVEITEM) ports TS InvOps.ts:499-531. Pops
// [fromInv, toInv, obj, count] and moves up to count of obj from
// fromInv to toInv. Remove first, then Add with the removed count
// (matches TS).
//
// Validator chain (NAI-131): from-InvTypeValid → to-InvTypeValid →
// ObjTypeValid → ObjStackValid → from-protect/scope → to-protect/scope.
//
// DEVIATION-NAI-131-D1: TS asymmetry — both protect/scope gates check
// fromInvType.scope !== SCOPE_SHARED (toInv's own scope is never
// consulted). Pinned per ts_asymmetry_dual_pin.md (positive presence
// + absence-pin in tests). Escalates if upstream TS fixes.
func handleInvMoveItem(s *ScriptState) error {
	if err := requireActivePlayer(s, "INV_MOVEITEM"); err != nil {
		return err
	}
	count := s.PopInt()
	obj := s.PopInt()
	toTypeID := s.PopInt()
	fromTypeID := s.PopInt()

	if err := checkInvType(s, fromTypeID, "INV_MOVEITEM"); err != nil {
		return err
	}
	if err := checkInvType(s, toTypeID, "INV_MOVEITEM"); err != nil {
		return err
	}
	if err := checkObjType(s, obj, "INV_MOVEITEM"); err != nil {
		return err
	}
	if err := checkObjStack(count, "INV_MOVEITEM"); err != nil {
		return err
	}

	fromInvType := s.Configs.InvType(fromTypeID)
	toInvType := s.Configs.InvType(toTypeID)

	// TS InvOps.ts:507-509 — from-protect gate uses fromInv.scope.
	if fromInvType.Protect && fromInvType.Scope != objtype.InvTypeScopeShared && !s.Protect {
		return fmt.Errorf("INV_MOVEITEM: $inv requires protected access: %s", fromInvType.DebugName)
	}
	// TS InvOps.ts:511-513 — to-protect gate ALSO uses fromInv.scope (DEVIATION-NAI-131-D1).
	if toInvType.Protect && fromInvType.Scope != objtype.InvTypeScopeShared && !s.Protect {
		return fmt.Errorf("INV_MOVEITEM: $inv requires protected access: %s", toInvType.DebugName)
	}

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
	stackable, stockObj := lookupStackableStockObj(s, toInv.Type, obj)
	toInv.Add(obj, tx.Completed, inventory.AddOpts{
		BeginSlot: -1,
		Stackable: stackable,
		StockObj:  stockObj,
	})
	return nil
}

// handleInvMoveFromSlot (INV_MOVEFROMSLOT) ports TS InvOps.ts:323-349.
// Pops [fromInv, toInv, fromSlot] and moves the entire slot contents
// from fromInv to toInv.
//
// Validator chain (NAI-131): from-InvTypeValid → to-InvTypeValid →
// from-protect/scope → to-protect/scope. No Obj-gates (the obj id
// comes from the source slot, not a stack-pushed input).
//
// DEVIATION-NAI-131-D1 applies (see handleInvMoveItem above).
func handleInvMoveFromSlot(s *ScriptState) error {
	if err := requireActivePlayer(s, "INV_MOVEFROMSLOT"); err != nil {
		return err
	}
	fromSlot := s.PopInt()
	toTypeID := s.PopInt()
	fromTypeID := s.PopInt()

	if err := checkInvType(s, fromTypeID, "INV_MOVEFROMSLOT"); err != nil {
		return err
	}
	if err := checkInvType(s, toTypeID, "INV_MOVEFROMSLOT"); err != nil {
		return err
	}

	fromInvType := s.Configs.InvType(fromTypeID)
	toInvType := s.Configs.InvType(toTypeID)

	if fromInvType.Protect && fromInvType.Scope != objtype.InvTypeScopeShared && !s.Protect {
		return fmt.Errorf("INV_MOVEFROMSLOT: $inv requires protected access: %s", fromInvType.DebugName)
	}
	if toInvType.Protect && fromInvType.Scope != objtype.InvTypeScopeShared && !s.Protect {
		return fmt.Errorf("INV_MOVEFROMSLOT: $inv requires protected access: %s", toInvType.DebugName)
	}

	fromInv := resolveInv(s, fromTypeID)
	if fromInv == nil {
		return fmt.Errorf("INV_MOVEFROMSLOT: no inv for from-type %d", fromTypeID)
	}
	toInv := resolveInv(s, toTypeID)
	if toInv == nil {
		return fmt.Errorf("INV_MOVEFROMSLOT: no inv for to-type %d", toTypeID)
	}
	it := fromInv.Get(fromSlot)
	if it == nil {
		return fmt.Errorf("INV_MOVEFROMSLOT: from slot %d empty", fromSlot)
	}
	id, cnt := it.Id, it.Count
	fromInv.Delete(fromSlot)
	stackable, stockObj := lookupStackableStockObj(s, toInv.Type, id)
	toInv.Add(id, cnt, inventory.AddOpts{
		BeginSlot: -1,
		Stackable: stackable,
		StockObj:  stockObj,
	})
	return nil
}
```

- [ ] **Step 4: Run the new tests — expect GREEN. Run the full pkg/script suite.**

```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/ -run "TestInvMove" -count=1 -v
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/... -count=1
```

Expected: All 12 new tests GREEN, plus pre-existing `TestInvMoveItem*` and `TestInvMoveFromSlot*` GREEN. Full suite GREEN.

- [ ] **Step 5: Commit.**

```
git add pkg/script/handlers_inv.go pkg/script/handlers_inv_test.go
git commit --no-gpg-sign -m "feat(nai-131): T4 — INV_MOVEITEM/MOVEFROMSLOT dual-inv validator gates (GREEN)

DEVIATION-NAI-131-D1: TS asymmetry preserved — both protect/scope
gates check fromInv.scope (toInv's scope never consulted). Dual-pin
per ts_asymmetry_dual_pin.md."
```

---

## Task 5: `checkObjStack` upper-bound tighten + doc-comment correction

**Files:**
- Modify: `pkg/script/handlers_obj.go:27-34` (helper body + doc-comment + import)
- Modify: `pkg/script/handlers_obj_test.go` OR `pkg/script/handlers_inv_test.go` (add test for upper bound)

**TS reference:** `Engine-TS/src/engine/script/ScriptValidators.ts:121` — `ObjStackValid: ScriptInputRangeValidator(1, Inventory.STACK_LIMIT, 'ObjStack')`.

- [ ] **Step 1: Write a RED test for the upper bound.**

Find an existing `handlers_obj_test.go`; if not present, append to `pkg/script/handlers_inv_test.go` (since the call site is `objAddCommon` and INV_ADD/SETSLOT/DEL also exercise `checkObjStack` post-T1):

```go
// (T5) checkObjStack upper-bound: count > Inventory.StackLimit
// (0x7fffffff) is rejected. TS-fidelity per ScriptValidators.ts:121.
func TestInvAdd_ObjStackValid_CountAboveStackLimit(t *testing.T) {
	lookup := newTestInvLookup()
	mc := newTestInvConfigs()
	// 0x7fffffff + 1 in int — represents one above StackLimit.
	overLimit := int(inventory.StackLimit) + 1
	runInvOpExpectErrAsPlayer(t, OpInvAdd, []int{testInvMain, testObjCoin, overLimit}, lookup, mc, fmt.Sprintf("invalid count (%d)", overLimit))
}
```

Note the import additions at top of the test file: `"fmt"` and `"github.com/zsrv/goscape/pkg/inventory"` are already imported in handlers_inv_test.go. No new imports.

- [ ] **Step 2: Run the test — expect RED.**

```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/ -run "TestInvAdd_ObjStackValid_CountAboveStackLimit" -count=1 -v
```

Expected: FAIL — `checkObjStack` currently only enforces `c >= 1`.

- [ ] **Step 3: Tighten `checkObjStack` and update the doc-comment.**

Edit `pkg/script/handlers_obj.go`:

Add to imports (line 4-8):

```go
import (
	"fmt"

	"github.com/zsrv/goscape/pkg/coordgrid"
	"github.com/zsrv/goscape/pkg/inventory"
)
```

Replace lines 27-34 (`checkObjStack` doc-comment + body):

```go
// checkObjStack mirrors TS ObjStackValid (ScriptValidators.ts:121) — a
// ScriptInputRangeValidator over [1, Inventory.STACK_LIMIT=0x7fffffff].
// Rejects 0, negatives, and counts above StackLimit.
func checkObjStack(c int, op string) error {
	if c < 1 || c > inventory.StackLimit {
		return fmt.Errorf("%s: invalid count (%d)", op, c)
	}
	return nil
}
```

- [ ] **Step 4: Run the test — expect GREEN. Run full suite.**

```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/ -run "TestInvAdd_ObjStackValid" -count=1 -v
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/... -count=1
```

Expected: GREEN. The other call site (`objAddCommon` for OBJ_ADD/OBJ_ADDALL) gets the upper bound for free; if any pre-existing OBJ_ADD test pushes a count > StackLimit, it RED-fails — patch it inline (audit: no such tests known to exist; OBJ_ADD tests typically use small counts).

- [ ] **Step 5: Commit.**

```
git add pkg/script/handlers_obj.go pkg/script/handlers_inv_test.go
git commit --no-gpg-sign -m "fix(nai-131): T5 — checkObjStack tighten to TS ObjStackValid range [1, StackLimit]"
```

---

## Task 6: Final verification + memory entry hygiene

**Files:**
- Modify: `pkg/script/handlers_inv_test.go` (only if T1-T5 missed any fixture patches — final audit)

This task is a verification+cleanup pass. Most of its work happens through inline fixes during T1-T5; this step formally confirms nothing is broken and writes the close commit.

- [ ] **Step 1: Run the entire test suite.**

```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./... -count=1
```

Expected: All packages PASS.

- [ ] **Step 2: Run with `-race`.**

```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race ./pkg/script/... ./pkg/inventory/... -count=1
```

Expected: PASS.

- [ ] **Step 3: Run `vet`.**

```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go vet ./...
```

Expected: clean output.

- [ ] **Step 4: Verify all 7 INV write handlers in `pkg/script/handlers_inv.go` now satisfy the §1 audit table — every cell ✓ (or ✱ for INV_TOTAL/INV_GETOBJ/etc. read handlers, which are out of scope).**

Manual audit:

```bash
rg -n "func handleInv(Add|Del|DelSlot|SetSlot|Clear|MoveItem|MoveFromSlot|DropSlot)" pkg/script/handlers_inv.go
```

Expected: every listed function starts with `requireActivePlayer` AND calls `checkInvType` AND has the protect/scope gate. (handleInvDropSlot was already TS-faithful and is unchanged.)

- [ ] **Step 5: Grep for any remaining stale `(false, false)` defensive returns in `lookupStackableStockObj` that might still be reachable.**

```bash
rg -n "lookupStackableStockObj" pkg/script/
```

Expected: 3 call sites (handleInvAdd, handleInvMoveItem, handleInvMoveFromSlot — INV_MOVEFROMSLOT looks up the obj id from the slot, not from a popped int, so the helper still serves a defensive role for the slot-derived id; keep DEVIATION-NAI-130-D3).

- [ ] **Step 6: Smoke check — start the server and confirm INV_ADD on Tutorial Combat Instructor still works (regression-pin against NAI-130's bronze-arrow stacking smoke).**

This step is a manual-spot regression, NOT required for close. Document the spot-check status in the close commit body.

If the user is willing, ask them to run:

```
CGO_ENABLED=0 GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go run -trimpath ./cmd/goscape --config.file config.yaml
```

…then complete Tutorial Island's combat-stage bronze-arrow handover. Stack should still arrive in 1 slot (regression-clean). If RED, fall back to NAI-130 fix path; otherwise GREEN-spot is the close criterion.

(If the user declines or defers, mark spot-check status `deferred` in the close commit; the unit-test gate is the binding criterion.)

- [ ] **Step 7: Close commit.**

```
git commit --no-gpg-sign --allow-empty -m "$(cat <<'EOF'
chore(nai-131): close — INV_* write-handler TS validator sweep

PRIMARY met. All 7 goscape INV write handlers in
pkg/script/handlers_inv.go now match the TS InvOps.ts validator chain:
requireActivePlayer + InvTypeValid + (per-handler) ObjTypeValid +
ObjStackValid + protect/scope + dummyitem. checkObjStack tightened
to the TS-faithful 1..StackLimit range.

Tasks: T0 fixture seeds, T1 INV_ADD (5 gates), T2 INV_SETSLOT
(5 gates), T3 INV_DEL/DELSLOT/CLEAR (3 handlers), T4 INV_MOVEITEM/
MOVEFROMSLOT (2 handlers, dual-inv DEVIATION-NAI-131-D1 preserving
TS asymmetry on fromInv.scope check), T5 checkObjStack tighten +
doc-comment correction, T6 close.

Out-of-scope (NAI-132+ candidates): missing-handler ports for
INV_CHANGESLOT, INV_DROPITEM, INV_DROPITEM_DELAYED, INV_MOVETOSLOT,
BOTH_MOVEINV, INV_MOVEITEM_CERT, INV_MOVEITEM_UNCERT.

Closes memory: feedback_followup_grep_path,
plan_grep_helper_patterns, ts_asymmetry_dual_pin,
defensive_gate_doc_comment_label.
EOF
)"
```

---

## Self-review checklist (executed by plan-author)

- [x] **Spec coverage.** Each spec §2 task (T1-T6) maps to a numbered task above. T1=spec T1; T2=spec T2; T3=spec T3 (3 sub-handlers in one task); T4=spec T4 (2 sub-handlers in one task); T5=spec T5; T6=spec T6 + final verification.
- [x] **Placeholder scan.** No "TBD", "TODO", "implement later". Every step has runnable code/commands.
- [x] **Type consistency.** Field names verified against pkg/objtype HEAD: `Protect`, `Scope`, `DummyInv`, `DummyItem`, `DebugName`, `InvTypeScopeShared`. Helper signatures verified: `checkInvType(s, id, op)`, `checkObjType(s, id, op)`, `checkObjStack(c, op)`, `requireActivePlayer(s, op)`. Mock fixtures verified: `mockConfigs.invs map`, `runInvOp` signature, `runInvOpExpectErr` signature.
- [x] **Test fixture runnability** (per `plan_runnable_test_fixtures.md`). Each test mentally executed — `runInvOp(t, OpX, intInputs, lookup, mc)` pushes inputs in the order given, opcode pops in reverse. Verified each test case against pop order in handler. T4's `TestInvMoveFromSlot_FromProtectGate_RejectsUnprotected` pre-fills the source slot to avoid hitting "from slot empty" before the gate fires.
- [x] **Helper-set audit** (per `plan_grep_helper_patterns.md`). All 5 validators are existing helpers; no inline reimplementation; doc-comments cite TS source line numbers.
- [x] **Hex literal sanity** (per `int32_hex_literal_overflow.md`). No 0xDEADBEEF-style fixtures. T5's `int(inventory.StackLimit) + 1` overflows to a negative value on 32-bit-only platforms but goscape's int is 64-bit; verified.
- [x] **TS asymmetry dual-pin** (per `ts_asymmetry_dual_pin.md`). T4 includes `TestInvMoveItem_TSAsymmetry_FromSharedSkipsBothGates` AND the deviation comment in handler doc-comment. Both presence and absence pinned.

---

## Deferred / out-of-scope

Tracked in spec §3 — handler ports for INV_CHANGESLOT, INV_DROPITEM, INV_DROPITEM_DELAYED, INV_MOVETOSLOT, BOTH_MOVEINV, INV_MOVEITEM_CERT, INV_MOVEITEM_UNCERT. Each is a full handler port (pop chain + Inventory call + protect/scope gate + tests), separate concern from this gate sweep.

Memory hygiene: `nai_followups.md` index lines 6415-6417 are stale (NAI-126 already shipped NPC_DEL handler, paramtype, and style cleanup). Cleanup in a separate session — not part of NAI-131 implementation work.
