# NAI-132 — INV_* Handler Ports Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Port four missing INV_* handlers (CHANGESLOT, DROPITEM, MOVEITEM_CERT, MOVEITEM_UNCERT), backfill NAI-131 dual-protect/scope validators onto the already-implemented INV_MOVETOSLOT, and modernize `inventory.Remove` with min/max builtins.

**Architecture:** Each task is a self-contained RED→GREEN port that mirrors TS `InvOps.ts` line-by-line, reusing established helpers (`requireActivePlayer`, `checkInvType`, `checkObjType`, `checkObjStack`, `checkCoord`, `checkDuration`, `lookupStackableStockObj`, `resolveInv`, NAI-131 `runInvOpExpectErrAsPlayer`). Inherits NAI-131-D1 (TS dual-from-scope asymmetry), NAI-130-D2 (defensive nil-World), NAI-130-D3 (defensive nil-Configs).

**Tech Stack:** Go 1.26+. No new dependencies.

**Spec:** `docs/superpowers/specs/2026-05-08-nai-132-inv-handler-ports-design.md`

**Cadence:** All `go` invocations prefix with `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache`. All commits use `--no-gpg-sign`. Each task ends with `go test ./pkg/script/... ./pkg/inventory/... -count=1` GREEN before commit.

---

## File Structure

**Modify (single file across all tasks):**
- `pkg/script/handlers_inv.go` — add T1 validators; add T3-T6 handlers
- `pkg/script/handlers.go` — add T3-T6 dispatch entries (after `OpInvMoveToSlot` at line 314)
- `pkg/script/handlers_inv_test.go` — add RED+GREEN tests for T1-T6; patch existing T1 GREEN regression
- `pkg/script/test_helpers.go` — extend ObjType seeds for T4/T5 cert tests
- `pkg/inventory/inventory.go` — T2 minmax modernization

No new files.

---

## Task 1 — INV_MOVETOSLOT validator-backfill

**Files:**
- Modify: `pkg/script/handlers_inv.go:866-902` (existing `handleInvMoveToSlot`)
- Modify: `pkg/script/handlers_inv_test.go:390-410` (existing `TestInvMoveToSlot` GREEN regression patch)
- Test: `pkg/script/handlers_inv_test.go` (new RED tests appended near existing INV_MOVETOSLOT test)

**Goal:** Add NAI-131 dual-protect/scope gates to existing handler. Inherits **DEVIATION-NAI-131-D1** (both gates evaluate `fromInvType.Scope`, never `toInvType.Scope`).

- [ ] **Step 1.1 — Read existing handler and test for the patch site**

```bash
sed -n '866,905p' pkg/script/handlers_inv.go
sed -n '385,415p' pkg/script/handlers_inv_test.go
```

Note current handler has no validators; reads pop ints directly.

- [ ] **Step 1.2 — Write failing RED tests for the 5 new gates**

Append to `pkg/script/handlers_inv_test.go` after the existing `TestInvMoveToSlot`:

```go
func TestInvMoveToSlot_NoActivePlayer(t *testing.T) {
	mc := newTestConfigs()
	lookup := newTestInvLookup()
	runInvOpExpectErr(t, OpInvMoveToSlot, []int{testInvMain, testInvBank, 0, 0}, lookup, mc, "INV_MOVETOSLOT: no active player")
}

func TestInvMoveToSlot_FromInvTypeInvalid(t *testing.T) {
	mc := newTestConfigs()
	lookup := newTestInvLookup()
	runInvOpExpectErrAsPlayer(t, OpInvMoveToSlot, []int{9999, testInvBank, 0, 0}, lookup, mc, "no InvType with value (9999) found")
}

func TestInvMoveToSlot_ToInvTypeInvalid(t *testing.T) {
	mc := newTestConfigs()
	lookup := newTestInvLookup()
	runInvOpExpectErrAsPlayer(t, OpInvMoveToSlot, []int{testInvMain, 9999, 0, 0}, lookup, mc, "no InvType with value (9999) found")
}

func TestInvMoveToSlot_FromProtectedRejectsUnprotected(t *testing.T) {
	mc := newTestConfigs()
	// Make testInvMain Protect+Perm so the from-gate fires.
	mc.invs[testInvMain].Protect = true
	mc.invs[testInvMain].Scope = objtype.InvTypeScopePerm
	lookup := newTestInvLookup()
	runInvOpExpectErrAsPlayer(t, OpInvMoveToSlot, []int{testInvMain, testInvBank, 0, 0}, lookup, mc, "INV_MOVETOSLOT: $inv requires protected access")
}

func TestInvMoveToSlot_ToProtectedAsymmetricD1(t *testing.T) {
	// DEVIATION-NAI-131-D1: to-gate evaluates fromInvType.Scope, NOT toInvType.Scope.
	// fromInvType.Scope=Shared but toInvType.Scope=Perm → to-gate must NOT fire (gated by from's scope).
	mc := newTestConfigs()
	mc.invs[testInvMain].Scope = objtype.InvTypeScopeShared
	mc.invs[testInvBank].Protect = true
	mc.invs[testInvBank].Scope = objtype.InvTypeScopePerm
	lookup := newTestInvLookup()
	// Should NOT error on the to-gate because fromInvType.Scope=Shared escapes both.
	st := runInvOp(t, OpInvMoveToSlot, []int{testInvMain, testInvBank, 0, 0}, lookup, mc)
	if st == nil {
		t.Fatal("expected handler to complete without error")
	}
}

func TestInvMoveToSlot_ToProtectedSamefromScopePerm(t *testing.T) {
	// Inverse pin: fromInvType.Scope=Perm + toInvType.Protect=true → to-gate fires.
	mc := newTestConfigs()
	mc.invs[testInvMain].Scope = objtype.InvTypeScopePerm
	mc.invs[testInvBank].Protect = true
	mc.invs[testInvBank].Scope = objtype.InvTypeScopeShared // ignored — gate uses from's scope
	lookup := newTestInvLookup()
	runInvOpExpectErrAsPlayer(t, OpInvMoveToSlot, []int{testInvMain, testInvBank, 0, 0}, lookup, mc, "INV_MOVETOSLOT: $inv requires protected access")
}
```

- [ ] **Step 1.3 — Run RED tests, confirm fail**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/ -run TestInvMoveToSlot -count=1 -v
```

Expected: existing `TestInvMoveToSlot` PASS; new 6 tests FAIL (handler currently has no validators).

- [ ] **Step 1.4 — Apply T1 implementation (validator backfill)**

Replace `pkg/script/handlers_inv.go:866-902` (the entire existing `handleInvMoveToSlot` function) with:

```go
// handleInvMoveToSlot (INV_MOVETOSLOT) ports TS InvOps.ts:353-368. Pops
// [fromInv, toInv, fromSlot, toSlot] (popInts(4) — toSlot on top) and
// swaps the two slot contents (nil-safe both directions). Matches TS
// Player.invMoveToSlot.
//
// Validator chain (NAI-131): from-InvTypeValid → to-InvTypeValid →
// from-protect/scope → to-protect/scope.
//
// DEVIATION-NAI-131-D1: TS asymmetry — both protect/scope gates check
// fromInvType.Scope, never toInvType.Scope. Pinned per ts_asymmetry_dual_pin.md.
func handleInvMoveToSlot(s *ScriptState) error {
	if err := requireActivePlayer(s, "INV_MOVETOSLOT"); err != nil {
		return err
	}
	toSlot := s.PopInt()
	fromSlot := s.PopInt()
	toTypeID := s.PopInt()
	fromTypeID := s.PopInt()

	if err := checkInvType(s, fromTypeID, "INV_MOVETOSLOT"); err != nil {
		return err
	}
	if err := checkInvType(s, toTypeID, "INV_MOVETOSLOT"); err != nil {
		return err
	}

	fromInvType := s.Configs.InvType(fromTypeID)
	toInvType := s.Configs.InvType(toTypeID)

	// TS InvOps.ts:359-361 — from-protect gate uses fromInvType.Scope.
	if fromInvType.Protect && fromInvType.Scope != objtype.InvTypeScopeShared && !s.Protect {
		return fmt.Errorf("INV_MOVETOSLOT: $inv requires protected access: %s", fromInvType.DebugName)
	}
	// TS InvOps.ts:363-365 — to-protect gate ALSO uses fromInvType.Scope (DEVIATION-NAI-131-D1).
	if toInvType.Protect && fromInvType.Scope != objtype.InvTypeScopeShared && !s.Protect {
		return fmt.Errorf("INV_MOVETOSLOT: $inv requires protected access: %s", toInvType.DebugName)
	}

	fromInv := resolveInv(s, fromTypeID)
	if fromInv == nil {
		return fmt.Errorf("INV_MOVETOSLOT: no inv for from-type %d", fromTypeID)
	}
	toInv := resolveInv(s, toTypeID)
	if toInv == nil {
		return fmt.Errorf("INV_MOVETOSLOT: no inv for to-type %d", toTypeID)
	}
	// Snapshot both ends; Set/Delete may rewrite the original slot when
	// from == to, so copy the item fields out first.
	var fromCopy, toCopy *inventory.Item
	if src := fromInv.Get(fromSlot); src != nil {
		fromCopy = &inventory.Item{Id: src.Id, Count: src.Count}
	}
	if dst := toInv.Get(toSlot); dst != nil {
		toCopy = &inventory.Item{Id: dst.Id, Count: dst.Count}
	}
	if fromCopy != nil {
		toInv.Set(toSlot, fromCopy)
	} else {
		toInv.Delete(toSlot)
	}
	if toCopy != nil {
		fromInv.Set(fromSlot, toCopy)
	} else {
		fromInv.Delete(fromSlot)
	}
	return nil
}
```

- [ ] **Step 1.5 — Patch existing GREEN regression test**

The pre-existing `TestInvMoveToSlot` (handlers_inv_test.go:390) calls `runInvOp` (no active player set) and expects the swap to succeed. After T1, that path now hits `requireActivePlayer` and errors. Patch the test to use `runInvOp` with an active-player-injected state, OR ensure the inv types satisfy the gates. Read the existing test first:

```bash
sed -n '390,415p' pkg/script/handlers_inv_test.go
```

The test should still call `runInvOp` (which sets Self). If `runInvOp` doesn't set active-player pointers, switch to a helper that does (check `runInvOp` body — NAI-130 may have unified it).

```bash
sed -n '110,170p' pkg/script/handlers_inv_test.go
```

If the test fixture uses default test InvTypes (no `Protect=true`), the new gates are no-ops and the existing test passes. Verify the GREEN by running the existing test alone first.

- [ ] **Step 1.6 — Run all tests, confirm GREEN**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/ -run TestInvMoveToSlot -count=1 -v
```

Expected: existing GREEN + 6 new tests PASS.

- [ ] **Step 1.7 — Run full package**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/... ./pkg/inventory/... -count=1
```

Expected: ALL PASS.

- [ ] **Step 1.8 — Commit**

```bash
git add pkg/script/handlers_inv.go pkg/script/handlers_inv_test.go
git commit --no-gpg-sign -m "feat(nai-132): T1 — INV_MOVETOSLOT NAI-131 validator-backfill (GREEN)

Adds requireActivePlayer + InvTypeValid×2 + dual-protect/scope gates to
handleInvMoveToSlot, closing NAI-131 spec-error fixup. DEVIATION-NAI-131-D1
inherited (both gates evaluate fromInvType.Scope, dual-pinned via
TestInvMoveToSlot_ToProtectedAsymmetricD1)."
```

---

## Task 2 — `inventory.Remove` minmax modernization

**Files:**
- Modify: `pkg/inventory/inventory.go:291-321` (`Remove` body)

**Goal:** Mechanical refactor — replace C-style guards with Go 1.21+ `min`/`max` builtins. ~6 LOC delta. Existing INV_DEL test paths cover Remove behaviorally; no new tests.

- [ ] **Step 2.1 — Read current Remove**

```bash
sed -n '291,325p' pkg/inventory/inventory.go
```

- [ ] **Step 2.2 — Apply T2 modernization**

In `pkg/inventory/inventory.go`, edit `Remove`:

Replace:
```go
	removed := 0
	begin := opts.BeginSlot
	if begin < 0 {
		begin = 0
	}
```
With:
```go
	removed := 0
	begin := max(opts.BeginSlot, 0)
```

Replace:
```go
		take := count - removed
		if take > it.Count {
			take = it.Count
		}
```
With:
```go
		take := min(count-removed, it.Count)
```

- [ ] **Step 2.3 — Run inventory + script tests, confirm GREEN**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/inventory/... ./pkg/script/... -count=1
```

Expected: ALL PASS.

- [ ] **Step 2.4 — Commit**

```bash
git add pkg/inventory/inventory.go
git commit --no-gpg-sign -m "refactor(nai-132): T2 — inventory.Remove min/max modernization

Replaces C-style guards in Remove with Go 1.21 min/max builtins. Final
NAI-126 carryover. Mechanical refactor; existing INV_DEL test paths cover
Remove behaviorally."
```

---

## Task 3 — INV_CHANGESLOT (4304)

**Files:**
- Modify: `pkg/script/handlers_inv.go` (add `handleInvChangeSlot` near other write-ops)
- Modify: `pkg/script/handlers.go` (add dispatch entry after `OpInvMoveToSlot:` at line 314)
- Test: `pkg/script/handlers_inv_test.go` (append new tests)

**Goal:** Port TS InvOps.ts:86-113. Pops `[inv, find, replace, replaceCount]`. **TS does not validate replaceCount** — pop-without-validate is intentional, absence-pinned via test.

- [ ] **Step 3.1 — Verify TS source unchanged at spec-write time**

```bash
sed -n '86,113p' $HOME/Code/github.com/LostCityRS/Engine-TS/src/engine/script/handlers/InvOps.ts
```

Confirm: validators are InvTypeValid + protect/scope + ObjTypeValid×2 (no ObjStackValid on replaceCount).

- [ ] **Step 3.2 — Write failing RED tests**

Append to `pkg/script/handlers_inv_test.go`:

```go
func TestInvChangeSlot_NoActivePlayer(t *testing.T) {
	mc := newTestConfigs()
	lookup := newTestInvLookup()
	runInvOpExpectErr(t, OpInvChangeSlot, []int{testInvMain, testObjCoin, testObjArr, 1}, lookup, mc, "INV_CHANGESLOT: no active player")
}

func TestInvChangeSlot_InvTypeInvalid(t *testing.T) {
	mc := newTestConfigs()
	lookup := newTestInvLookup()
	runInvOpExpectErrAsPlayer(t, OpInvChangeSlot, []int{9999, testObjCoin, testObjArr, 1}, lookup, mc, "no InvType with value (9999) found")
}

func TestInvChangeSlot_ProtectedRejectsUnprotected(t *testing.T) {
	mc := newTestConfigs()
	mc.invs[testInvMain].Protect = true
	mc.invs[testInvMain].Scope = objtype.InvTypeScopePerm
	lookup := newTestInvLookup()
	runInvOpExpectErrAsPlayer(t, OpInvChangeSlot, []int{testInvMain, testObjCoin, testObjArr, 1}, lookup, mc, "INV_CHANGESLOT: $inv requires protected access")
}

func TestInvChangeSlot_FindObjTypeInvalid(t *testing.T) {
	mc := newTestConfigs()
	lookup := newTestInvLookup()
	runInvOpExpectErrAsPlayer(t, OpInvChangeSlot, []int{testInvMain, 9999, testObjArr, 1}, lookup, mc, "no ObjType with value (9999) found")
}

func TestInvChangeSlot_ReplaceObjTypeInvalid(t *testing.T) {
	mc := newTestConfigs()
	lookup := newTestInvLookup()
	runInvOpExpectErrAsPlayer(t, OpInvChangeSlot, []int{testInvMain, testObjCoin, 9999, 1}, lookup, mc, "no ObjType with value (9999) found")
}

func TestInvChangeSlot_HitOnFirstMatch(t *testing.T) {
	mc := newTestConfigs()
	lookup := newTestInvLookup()
	inv := lookup.Get(nil, testInvMain) // pre-populate
	inv.Set(0, &inventory.Item{Id: testObjCoin, Count: 100})
	inv.Set(1, &inventory.Item{Id: testObjCoin, Count: 50})
	runInvOp(t, OpInvChangeSlot, []int{testInvMain, testObjCoin, testObjArr, 7}, lookup, mc)
	// Slot 0 replaced; slot 1 untouched (early return on first hit).
	if got := inv.Get(0); got == nil || got.Id != testObjArr || got.Count != 7 {
		t.Errorf("slot 0: got %+v, want {Id=%d, Count=7}", got, testObjArr)
	}
	if got := inv.Get(1); got == nil || got.Id != testObjCoin || got.Count != 50 {
		t.Errorf("slot 1 should be unchanged: got %+v", got)
	}
}

func TestInvChangeSlot_NoMatchSilentNoOp(t *testing.T) {
	mc := newTestConfigs()
	lookup := newTestInvLookup()
	inv := lookup.Get(nil, testInvMain)
	inv.Set(0, &inventory.Item{Id: testObjCoin, Count: 100})
	runInvOp(t, OpInvChangeSlot, []int{testInvMain, testObjArr, testObjSword, 1}, lookup, mc)
	if got := inv.Get(0); got == nil || got.Id != testObjCoin || got.Count != 100 {
		t.Errorf("slot 0 should be unchanged: got %+v", got)
	}
}

func TestInvChangeSlot_ReplaceCountZeroAbsencePin(t *testing.T) {
	// Absence-pin: TS does NOT validate replaceCount via ObjStackValid (no `check(count, ObjStackValid)` at InvOps.ts:86-113).
	// replaceCount=0 must be accepted (pop-without-validate); inv.Set writes the zero-count item.
	mc := newTestConfigs()
	lookup := newTestInvLookup()
	inv := lookup.Get(nil, testInvMain)
	inv.Set(0, &inventory.Item{Id: testObjCoin, Count: 100})
	runInvOp(t, OpInvChangeSlot, []int{testInvMain, testObjCoin, testObjArr, 0}, lookup, mc)
	if got := inv.Get(0); got == nil || got.Id != testObjArr || got.Count != 0 {
		t.Errorf("slot 0: got %+v, want {Id=%d, Count=0}", got, testObjArr)
	}
}
```

- [ ] **Step 3.3 — Run RED tests, confirm fail**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/ -run TestInvChangeSlot -count=1 -v
```

Expected: all 8 tests fail (handler not yet wired; opcode dispatches to no entry).

- [ ] **Step 3.4 — Add handler to `handlers_inv.go`**

Append the following function to `pkg/script/handlers_inv.go` (after `handleInvMoveToSlot`):

```go
// handleInvChangeSlot (INV_CHANGESLOT) ports TS InvOps.ts:86-113. Pops
// [inv, find, replace, replaceCount]. Loops the inventory for the first
// slot whose item.Id == findObj.Id; on hit, replaces with replaceObj.Id
// at replaceCount. No-match is a silent no-op.
//
// Validator chain (NAI-131 shape, partial): InvTypeValid → protect/scope
// → ObjTypeValid(find) → ObjTypeValid(replace). NOTE: TS does NOT
// validate replaceCount (no `check(count, ObjStackValid)` at InvOps.ts:86-113).
// Goscape preserves this — pop-without-validate is intentional;
// absence-pinned via TestInvChangeSlot_ReplaceCountZeroAbsencePin.
func handleInvChangeSlot(s *ScriptState) error {
	if err := requireActivePlayer(s, "INV_CHANGESLOT"); err != nil {
		return err
	}
	replaceCount := s.PopInt()
	replace := s.PopInt()
	find := s.PopInt()
	typeID := s.PopInt()

	if err := checkInvType(s, typeID, "INV_CHANGESLOT"); err != nil {
		return err
	}

	invType := s.Configs.InvType(typeID)
	if invType.Protect && invType.Scope != objtype.InvTypeScopeShared && !s.Protect {
		return fmt.Errorf("INV_CHANGESLOT: $inv requires protected access: %s", invType.DebugName)
	}

	if err := checkObjType(s, find, "INV_CHANGESLOT"); err != nil {
		return err
	}
	if err := checkObjType(s, replace, "INV_CHANGESLOT"); err != nil {
		return err
	}

	inv := resolveInv(s, typeID)
	if inv == nil {
		return fmt.Errorf("INV_CHANGESLOT: no inv for type %d", typeID)
	}

	findObj := s.Configs.ObjType(find)
	replaceObj := s.Configs.ObjType(replace)
	for slot := 0; slot < inv.Capacity; slot++ {
		it := inv.Get(slot)
		if it == nil {
			continue
		}
		if it.Id == findObj.Id {
			inv.Set(slot, &inventory.Item{Id: replaceObj.Id, Count: replaceCount})
			return nil
		}
	}
	return nil
}
```

- [ ] **Step 3.5 — Wire dispatch entry**

In `pkg/script/handlers.go`, add after the `OpInvMoveToSlot:` line (around line 314):

```go
	OpInvChangeSlot:   handleInvChangeSlot,
```

- [ ] **Step 3.6 — Run RED tests, confirm GREEN**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/ -run TestInvChangeSlot -count=1 -v
```

Expected: all 8 tests PASS.

- [ ] **Step 3.7 — Run full package**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/... ./pkg/inventory/... -count=1
```

Expected: ALL PASS.

- [ ] **Step 3.8 — Commit**

```bash
git add pkg/script/handlers_inv.go pkg/script/handlers.go pkg/script/handlers_inv_test.go
git commit --no-gpg-sign -m "feat(nai-132): T3 — INV_CHANGESLOT handler (GREEN)

Ports TS InvOps.ts:86-113. Validators: InvTypeValid + protect/scope +
ObjTypeValid×2. TS does NOT validate replaceCount (pop-without-validate);
preserved with absence-pin test."
```

---

## Task 4 — INV_MOVEITEM_UNCERT (4320)

**Files:**
- Modify: `pkg/script/handlers_inv.go` (add `handleInvMoveItemUncert`)
- Modify: `pkg/script/handlers.go` (add dispatch entry)
- Modify: `pkg/script/test_helpers.go` (add cert ObjType seeds)
- Test: `pkg/script/handlers_inv_test.go` (append new tests)

**Goal:** Port TS InvOps.ts:570-597. invDel → cert-resolve via `CertTemplate>=0 && CertLink>=0` → invAdd. **No overflow drop.** Inherits **DEVIATION-NAI-131-D1** (dual-from-scope).

- [ ] **Step 4.1 — Verify ObjType field shape and current test seeds**

```bash
sed -n '120,135p' pkg/objtype/objtype.go
sed -n '20,100p' pkg/script/test_helpers.go
```

Confirm: `CertLink int` (line 124), `CertTemplate int` (line 125); existing seeds set neither (defaults to -1 per `pkg/objtype/objtype.go:300`).

- [ ] **Step 4.2 — Add cert ObjType test seeds**

Append to `pkg/script/test_helpers.go` near other obj seeds (after `testObjSword`):

```go
const (
	testObjCertNote = 4 // certificate-template item: CertTemplate=-1, CertLink=testObjCoin (the underlying obj)
)
```

In the configs builder (after the existing sword seed):

```go
	certNote := objtype.NewObjType(testObjCertNote)
	certNote.DebugName = "cert_note"
	certNote.CertTemplate = -1
	certNote.CertLink = testObjCoin
	certNote.Stackable = true
	mc.objs[testObjCertNote] = certNote
```

- [ ] **Step 4.3 — Write failing RED tests**

Append to `pkg/script/handlers_inv_test.go`:

```go
func TestInvMoveItemUncert_NoActivePlayer(t *testing.T) {
	mc := newTestConfigs()
	lookup := newTestInvLookup()
	runInvOpExpectErr(t, OpInvMoveItemUncert, []int{testInvMain, testInvBank, testObjCoin, 1}, lookup, mc, "INV_MOVEITEM_UNCERT: no active player")
}

func TestInvMoveItemUncert_FromInvTypeInvalid(t *testing.T) {
	mc := newTestConfigs()
	lookup := newTestInvLookup()
	runInvOpExpectErrAsPlayer(t, OpInvMoveItemUncert, []int{9999, testInvBank, testObjCoin, 1}, lookup, mc, "no InvType with value (9999) found")
}

func TestInvMoveItemUncert_ToInvTypeInvalid(t *testing.T) {
	mc := newTestConfigs()
	lookup := newTestInvLookup()
	runInvOpExpectErrAsPlayer(t, OpInvMoveItemUncert, []int{testInvMain, 9999, testObjCoin, 1}, lookup, mc, "no InvType with value (9999) found")
}

func TestInvMoveItemUncert_ObjTypeInvalid(t *testing.T) {
	mc := newTestConfigs()
	lookup := newTestInvLookup()
	runInvOpExpectErrAsPlayer(t, OpInvMoveItemUncert, []int{testInvMain, testInvBank, 9999, 1}, lookup, mc, "no ObjType with value (9999) found")
}

func TestInvMoveItemUncert_ObjStackInvalid(t *testing.T) {
	mc := newTestConfigs()
	lookup := newTestInvLookup()
	runInvOpExpectErrAsPlayer(t, OpInvMoveItemUncert, []int{testInvMain, testInvBank, testObjCoin, 0}, lookup, mc, "INV_MOVEITEM_UNCERT: invalid count (0)")
}

func TestInvMoveItemUncert_NonCertObjMovesAsIs(t *testing.T) {
	// Non-cert obj (CertTemplate=-1 default, CertLink=-1 default) → invAdd uses obj.Id unchanged.
	mc := newTestConfigs()
	lookup := newTestInvLookup()
	from := lookup.Get(nil, testInvMain)
	from.Set(0, &inventory.Item{Id: testObjCoin, Count: 50})
	runInvOp(t, OpInvMoveItemUncert, []int{testInvMain, testInvBank, testObjCoin, 50}, lookup, mc)
	if got := from.Get(0); got != nil {
		t.Errorf("from slot 0 should be empty: got %+v", got)
	}
	to := lookup.Get(nil, testInvBank)
	if got := to.GetItemCount(testObjCoin); got != 50 {
		t.Errorf("to inv: got %d coins, want 50", got)
	}
}

func TestInvMoveItemUncert_CertObjUncertifies(t *testing.T) {
	// Certificate obj (CertTemplate=-1 + CertLink>=0) → invAdd uses CertLink.
	mc := newTestConfigs()
	lookup := newTestInvLookup()
	from := lookup.Get(nil, testInvMain)
	from.Set(0, &inventory.Item{Id: testObjCertNote, Count: 5})
	runInvOp(t, OpInvMoveItemUncert, []int{testInvMain, testInvBank, testObjCertNote, 5}, lookup, mc)
	to := lookup.Get(nil, testInvBank)
	if got := to.GetItemCount(testObjCertNote); got != 0 {
		t.Errorf("to inv should not contain cert note: got %d", got)
	}
	// CertLink = testObjCoin → 5 coins added.
	if got := to.GetItemCount(testObjCoin); got != 5 {
		t.Errorf("to inv: got %d coins via cert→link, want 5", got)
	}
}

func TestInvMoveItemUncert_RemoveZeroCompletesNoOp(t *testing.T) {
	// from inv empty → tx.Completed=0 → return without invAdd.
	mc := newTestConfigs()
	lookup := newTestInvLookup()
	runInvOp(t, OpInvMoveItemUncert, []int{testInvMain, testInvBank, testObjCoin, 50}, lookup, mc)
	to := lookup.Get(nil, testInvBank)
	if got := to.GetItemCount(testObjCoin); got != 0 {
		t.Errorf("to inv: got %d coins, want 0 (Remove returned 0)", got)
	}
}
```

- [ ] **Step 4.4 — Run RED tests, confirm fail**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/ -run TestInvMoveItemUncert -count=1 -v
```

Expected: all 8 tests fail.

- [ ] **Step 4.5 — Add handler to `handlers_inv.go`**

Append after `handleInvChangeSlot`:

```go
// handleInvMoveItemUncert (INV_MOVEITEM_UNCERT) ports TS InvOps.ts:570-597.
// Pops [fromInv, toInv, obj, count]. invDel → if obj is a certificate
// (CertTemplate >= 0 && CertLink >= 0) add CertLink to toInv else add
// obj.Id. No overflow-to-world drop (TS InvOps.ts:593-595 just calls
// player.invAdd without overflow-handling).
//
// Validator chain (NAI-131): from-InvTypeValid → to-InvTypeValid →
// ObjTypeValid → ObjStackValid → from-protect/scope → to-protect/scope
// (DEVIATION-NAI-131-D1: both gates evaluate fromInvType.Scope).
func handleInvMoveItemUncert(s *ScriptState) error {
	if err := requireActivePlayer(s, "INV_MOVEITEM_UNCERT"); err != nil {
		return err
	}
	count := s.PopInt()
	obj := s.PopInt()
	toTypeID := s.PopInt()
	fromTypeID := s.PopInt()

	if err := checkInvType(s, fromTypeID, "INV_MOVEITEM_UNCERT"); err != nil {
		return err
	}
	if err := checkInvType(s, toTypeID, "INV_MOVEITEM_UNCERT"); err != nil {
		return err
	}
	if err := checkObjType(s, obj, "INV_MOVEITEM_UNCERT"); err != nil {
		return err
	}
	if err := checkObjStack(count, "INV_MOVEITEM_UNCERT"); err != nil {
		return err
	}

	fromInvType := s.Configs.InvType(fromTypeID)
	toInvType := s.Configs.InvType(toTypeID)

	if fromInvType.Protect && fromInvType.Scope != objtype.InvTypeScopeShared && !s.Protect {
		return fmt.Errorf("INV_MOVEITEM_UNCERT: $inv requires protected access: %s", fromInvType.DebugName)
	}
	// DEVIATION-NAI-131-D1: to-gate uses fromInvType.Scope.
	if toInvType.Protect && fromInvType.Scope != objtype.InvTypeScopeShared && !s.Protect {
		return fmt.Errorf("INV_MOVEITEM_UNCERT: $inv requires protected access: %s", toInvType.DebugName)
	}

	fromInv := resolveInv(s, fromTypeID)
	if fromInv == nil {
		return fmt.Errorf("INV_MOVEITEM_UNCERT: no inv for from-type %d", fromTypeID)
	}
	toInv := resolveInv(s, toTypeID)
	if toInv == nil {
		return fmt.Errorf("INV_MOVEITEM_UNCERT: no inv for to-type %d", toTypeID)
	}

	tx := fromInv.Remove(obj, count, inventory.RemoveOpts{BeginSlot: -1})
	if tx.Completed == 0 {
		return nil
	}

	objType := s.Configs.ObjType(obj)
	finalObj := obj
	if objType.CertTemplate >= 0 && objType.CertLink >= 0 {
		finalObj = objType.CertLink
	}
	stackable, stockObj := lookupStackableStockObj(s, toInv.Type, finalObj)
	toInv.Add(finalObj, tx.Completed, inventory.AddOpts{
		BeginSlot: -1,
		Stackable: stackable,
		StockObj:  stockObj,
	})
	return nil
}
```

- [ ] **Step 4.6 — Wire dispatch entry**

In `pkg/script/handlers.go`, add after `OpInvChangeSlot:`:

```go
	OpInvMoveItemUncert: handleInvMoveItemUncert,
```

- [ ] **Step 4.7 — Run RED tests, confirm GREEN**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/ -run TestInvMoveItemUncert -count=1 -v
```

Expected: all 8 tests PASS.

- [ ] **Step 4.8 — Run full package**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/... ./pkg/inventory/... -count=1
```

Expected: ALL PASS.

- [ ] **Step 4.9 — Commit**

```bash
git add pkg/script/handlers_inv.go pkg/script/handlers.go pkg/script/handlers_inv_test.go pkg/script/test_helpers.go
git commit --no-gpg-sign -m "feat(nai-132): T4 — INV_MOVEITEM_UNCERT handler (GREEN)

Ports TS InvOps.ts:570-597. invDel → cert-resolve (CertTemplate>=0 &&
CertLink>=0 → CertLink) → invAdd. No overflow drop. DEVIATION-NAI-131-D1
inherited (dual-from-scope). Adds testObjCertNote seed for cert-branch
fixture coverage."
```

---

## Task 5 — INV_MOVEITEM_CERT (4319)

**Files:**
- Modify: `pkg/script/handlers_inv.go` (add `handleInvMoveItemCert`)
- Modify: `pkg/script/handlers.go` (add dispatch entry)
- Test: `pkg/script/handlers_inv_test.go` (append new tests)

**Goal:** Port TS InvOps.ts:535-566. invDel → **inverted** cert-resolve `CertTemplate==-1 && CertLink>=0 → finalObj=CertLink` → invAdd → overflow → `World.AddObj(level, x, z, finalObj, overflow, 200, receiverID)` (single call, NOT per-item; TS comment: "should be a stackable cert already"). Inherits **DEVIATION-NAI-131-D1**, **DEVIATION-NAI-130-D2** (defensive nil-World).

- [ ] **Step 5.1 — Verify TS source**

```bash
sed -n '535,566p' $HOME/Code/github.com/LostCityRS/Engine-TS/src/engine/script/handlers/InvOps.ts
```

Note line 558: `if (objType.certtemplate === -1 && objType.certlink >= 0)` — the inverted condition vs UNCERT.

- [ ] **Step 5.2 — Verify mockWorld AddObj recorder shape**

```bash
grep -n "type fakeWorld\|type mockWorld\|AddObj" pkg/script/handlers_obj_test.go pkg/script/handlers_inv_test.go pkg/script/test_helpers.go 2>/dev/null | head -10
```

Confirm a `mockWorld` (or similar) test fixture exists with an AddObj recorder. If not, the existing INV_DROPSLOT tests (handlers_inv_test.go, search "TestInvDropSlot") use a fakeWorld pattern — reuse it.

- [ ] **Step 5.3 — Write failing RED tests**

Append to `pkg/script/handlers_inv_test.go`. Use the same mockWorld pattern as INV_DROPSLOT tests:

```go
func TestInvMoveItemCert_NoActivePlayer(t *testing.T) {
	mc := newTestConfigs()
	lookup := newTestInvLookup()
	runInvOpExpectErr(t, OpInvMoveItemCert, []int{testInvMain, testInvBank, testObjCoin, 1}, lookup, mc, "INV_MOVEITEM_CERT: no active player")
}

func TestInvMoveItemCert_FromInvTypeInvalid(t *testing.T) {
	mc := newTestConfigs()
	lookup := newTestInvLookup()
	runInvOpExpectErrAsPlayer(t, OpInvMoveItemCert, []int{9999, testInvBank, testObjCoin, 1}, lookup, mc, "no InvType with value (9999) found")
}

func TestInvMoveItemCert_ObjStackInvalid(t *testing.T) {
	mc := newTestConfigs()
	lookup := newTestInvLookup()
	runInvOpExpectErrAsPlayer(t, OpInvMoveItemCert, []int{testInvMain, testInvBank, testObjCoin, 0}, lookup, mc, "INV_MOVEITEM_CERT: invalid count (0)")
}

func TestInvMoveItemCert_NonCertObjMovesAsIs(t *testing.T) {
	// Non-cert obj (CertTemplate=-1, CertLink=-1) → finalObj=obj.Id unchanged.
	mc := newTestConfigs()
	lookup := newTestInvLookup()
	from := lookup.Get(nil, testInvMain)
	from.Set(0, &inventory.Item{Id: testObjCoin, Count: 50})
	runInvOp(t, OpInvMoveItemCert, []int{testInvMain, testInvBank, testObjCoin, 50}, lookup, mc)
	to := lookup.Get(nil, testInvBank)
	if got := to.GetItemCount(testObjCoin); got != 50 {
		t.Errorf("to inv: got %d coins, want 50", got)
	}
}

func TestInvMoveItemCert_CertableObjCertifies(t *testing.T) {
	// Certable obj (CertTemplate=-1 && CertLink>=0) → finalObj=CertLink.
	// testObjCertNote has CertTemplate=-1, CertLink=testObjCoin → finalObj=testObjCoin.
	mc := newTestConfigs()
	lookup := newTestInvLookup()
	from := lookup.Get(nil, testInvMain)
	from.Set(0, &inventory.Item{Id: testObjCertNote, Count: 5})
	runInvOp(t, OpInvMoveItemCert, []int{testInvMain, testInvBank, testObjCertNote, 5}, lookup, mc)
	to := lookup.Get(nil, testInvBank)
	if got := to.GetItemCount(testObjCertNote); got != 0 {
		t.Errorf("to inv should NOT contain cert note: got %d", got)
	}
	if got := to.GetItemCount(testObjCoin); got != 5 {
		t.Errorf("to inv: got %d coins via cert link, want 5", got)
	}
}

func TestInvMoveItemCert_OverflowDropsToWorld(t *testing.T) {
	mc := newTestConfigs()
	lookup := newTestInvLookup()
	from := lookup.Get(nil, testInvMain)
	from.Set(0, &inventory.Item{Id: testObjCoin, Count: 10})
	// to inv with capacity 1 already at full to force overflow.
	to := lookup.Get(nil, testInvBank)
	to.Capacity = 1
	to.Items = make([]*inventory.Item, 1)
	to.Set(0, &inventory.Item{Id: testObjArr, Count: 1}) // non-stackable obj blocks coin add
	world := newMockWorld()
	st := runInvOpWithWorld(t, OpInvMoveItemCert, []int{testInvMain, testInvBank, testObjCoin, 10}, lookup, mc, world)
	_ = st
	if len(world.addObjCalls) != 1 {
		t.Fatalf("want 1 World.AddObj call (single stacked overflow), got %d", len(world.addObjCalls))
	}
	got := world.addObjCalls[0]
	if got.objId != testObjCoin || got.count != 10 || got.duration != 200 {
		t.Errorf("AddObj args: got obj=%d count=%d duration=%d, want obj=%d count=10 duration=200",
			got.objId, got.count, got.duration, testObjCoin)
	}
}

func TestInvMoveItemCert_RemoveZeroCompletesNoOp(t *testing.T) {
	mc := newTestConfigs()
	lookup := newTestInvLookup()
	world := newMockWorld()
	runInvOpWithWorld(t, OpInvMoveItemCert, []int{testInvMain, testInvBank, testObjCoin, 50}, lookup, mc, world)
	if len(world.addObjCalls) != 0 {
		t.Errorf("want 0 AddObj calls when from-Remove completes 0; got %d", len(world.addObjCalls))
	}
}
```

**Note:** If `runInvOpWithWorld` doesn't exist in `handlers_inv_test.go`, study the INV_DROPSLOT test setup pattern and add an inline equivalent or extend the existing helper. Check first:

```bash
grep -n "runInvOpWith\|World:\|s.World =" pkg/script/handlers_inv_test.go | head
```

If absent, define a local helper in handlers_inv_test.go:

```go
func runInvOpWithWorld(t *testing.T, op Opcode, intInputs []int, lookup InvLookup, configs Configs, world WorldSurface) *ScriptState {
	state := runInvOp(t, op, intInputs, lookup, configs)
	state.World = world
	// Re-execute (tests prefer pre-set; if runInvOp already ran, you may need to refactor).
	return state
}
```

**Cleaner approach:** extend `runInvOp` to accept an optional WorldSurface, OR construct the state explicitly in the overflow tests. Implementer decides.

- [ ] **Step 5.4 — Run RED tests, confirm fail**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/ -run TestInvMoveItemCert -count=1 -v
```

Expected: all 7 tests fail.

- [ ] **Step 5.5 — Add handler to `handlers_inv.go`**

Append after `handleInvMoveItemUncert`:

```go
// handleInvMoveItemCert (INV_MOVEITEM_CERT) ports TS InvOps.ts:535-566.
// Pops [fromInv, toInv, obj, count]. invDel → if obj is certifiable
// (CertTemplate == -1 && CertLink >= 0) finalObj=CertLink; invAdd
// finalObj. Overflow drops to world as a single stacked Obj — TS
// comment "should be a stackable cert already" → no per-item branch.
//
// Validator chain (NAI-131): from-InvTypeValid → to-InvTypeValid →
// ObjTypeValid → ObjStackValid → from-protect/scope → to-protect/scope
// (DEVIATION-NAI-131-D1: both gates evaluate fromInvType.Scope).
//
// DEVIATION-NAI-130-D2: defensive nil-World guard skips overflow drop
// when s.World is unset (goscape defensive; TS uses static World import).
func handleInvMoveItemCert(s *ScriptState) error {
	if err := requireActivePlayer(s, "INV_MOVEITEM_CERT"); err != nil {
		return err
	}
	count := s.PopInt()
	obj := s.PopInt()
	toTypeID := s.PopInt()
	fromTypeID := s.PopInt()

	if err := checkInvType(s, fromTypeID, "INV_MOVEITEM_CERT"); err != nil {
		return err
	}
	if err := checkInvType(s, toTypeID, "INV_MOVEITEM_CERT"); err != nil {
		return err
	}
	if err := checkObjType(s, obj, "INV_MOVEITEM_CERT"); err != nil {
		return err
	}
	if err := checkObjStack(count, "INV_MOVEITEM_CERT"); err != nil {
		return err
	}

	fromInvType := s.Configs.InvType(fromTypeID)
	toInvType := s.Configs.InvType(toTypeID)

	if fromInvType.Protect && fromInvType.Scope != objtype.InvTypeScopeShared && !s.Protect {
		return fmt.Errorf("INV_MOVEITEM_CERT: $inv requires protected access: %s", fromInvType.DebugName)
	}
	// DEVIATION-NAI-131-D1.
	if toInvType.Protect && fromInvType.Scope != objtype.InvTypeScopeShared && !s.Protect {
		return fmt.Errorf("INV_MOVEITEM_CERT: $inv requires protected access: %s", toInvType.DebugName)
	}

	fromInv := resolveInv(s, fromTypeID)
	if fromInv == nil {
		return fmt.Errorf("INV_MOVEITEM_CERT: no inv for from-type %d", fromTypeID)
	}
	toInv := resolveInv(s, toTypeID)
	if toInv == nil {
		return fmt.Errorf("INV_MOVEITEM_CERT: no inv for to-type %d", toTypeID)
	}

	tx := fromInv.Remove(obj, count, inventory.RemoveOpts{BeginSlot: -1})
	if tx.Completed == 0 {
		return nil
	}

	objType := s.Configs.ObjType(obj)
	finalObj := obj
	if objType.CertTemplate == -1 && objType.CertLink >= 0 {
		finalObj = objType.CertLink
	}
	stackable, stockObj := lookupStackableStockObj(s, toInv.Type, finalObj)
	tx2 := toInv.Add(finalObj, tx.Completed, inventory.AddOpts{
		BeginSlot: -1,
		Stackable: stackable,
		StockObj:  stockObj,
	})

	overflow := count - tx2.Completed
	if overflow > 0 && s.World != nil {
		level := (s.Self.CoordPacked() >> 28) & 0x3
		receiverID := s.Self.UID()
		s.World.AddObj(level, s.Self.X(), s.Self.Z(), finalObj, overflow, 200, receiverID)
	}
	return nil
}
```

- [ ] **Step 5.6 — Wire dispatch entry**

In `pkg/script/handlers.go`, add after `OpInvMoveItemUncert:`:

```go
	OpInvMoveItemCert:   handleInvMoveItemCert,
```

- [ ] **Step 5.7 — Run RED tests, confirm GREEN**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/ -run TestInvMoveItemCert -count=1 -v
```

Expected: all 7 tests PASS.

- [ ] **Step 5.8 — Run full package**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/... ./pkg/inventory/... -count=1
```

Expected: ALL PASS.

- [ ] **Step 5.9 — Commit**

```bash
git add pkg/script/handlers_inv.go pkg/script/handlers.go pkg/script/handlers_inv_test.go
git commit --no-gpg-sign -m "feat(nai-132): T5 — INV_MOVEITEM_CERT handler (GREEN)

Ports TS InvOps.ts:535-566. invDel → cert-resolve (CertTemplate==-1 &&
CertLink>=0 → CertLink, INVERTED vs UNCERT) → invAdd → overflow drops to
world as single stacked Obj (per TS comment 'should be a stackable cert
already'). Inherits D1 (dual-from-scope) + D2 (defensive nil-World)."
```

---

## Task 6 — INV_DROPITEM (4311)

**Files:**
- Modify: `pkg/script/handlers_inv.go` (add `handleInvDropItem`)
- Modify: `pkg/script/handlers.go` (add dispatch entry)
- Test: `pkg/script/handlers_inv_test.go` (append new tests)

**Goal:** Port TS InvOps.ts:163-186. Pops `[inv, coord, obj, count, duration]`. Validators: InvTypeValid → CoordValid → ObjTypeValid → ObjStackValid → DurationValid → protect/scope. Logic mirrors INV_DROPSLOT (handlers_inv.go:771): `inv.Remove` → if `tx.Completed==0` return; if non-stackable OR completed==1 spawn per-item, else single stacked AddObj. Set `s.ActiveObj` + `s.Pointers |= PtrActiveObj` after each spawn (last-wins).

- [ ] **Step 6.1 — Verify checkCoord/checkDuration error literals**

```bash
sed -n '13,21p' pkg/script/handlers_npc.go
sed -n '275,285p' pkg/script/handlers_loc.go
```

Confirm: `checkCoord` returns `"<op>: coord out of range (N)"` (op-prefixed); `checkDuration` returns `"duration out of range [1, 2147483647]: N"` (NOT op-prefixed — handler must wrap via `fmt.Errorf("%s: %w", "INV_DROPITEM", err)`).

- [ ] **Step 6.2 — Re-read INV_DROPSLOT body for the canonical pattern**

```bash
sed -n '771,866p' pkg/script/handlers_inv.go
```

Note: `s.World.AddObj` is called inside `for range completed` for non-stackable/single-item, and once for stackable+multi.

- [ ] **Step 6.3 — Write failing RED tests**

Append to `pkg/script/handlers_inv_test.go`. The mockWorld pattern from INV_DROPSLOT tests is the model. Construct a valid coord int via `packCoord(level, x, z)` if available, else use a known-valid integer (search existing tests):

```bash
grep -n "packCoord\|coord.*<<\|0x[0-9a-f]*.*coord\|CoordPacked" pkg/script/handlers_inv_test.go | head
```

```go
func TestInvDropItem_NoActivePlayer(t *testing.T) {
	mc := newTestConfigs()
	lookup := newTestInvLookup()
	runInvOpExpectErr(t, OpInvDropItem, []int{testInvMain, validCoord(0, 3200, 3200), testObjCoin, 1, 100}, lookup, mc, "INV_DROPITEM: no active player")
}

func TestInvDropItem_InvTypeInvalid(t *testing.T) {
	mc := newTestConfigs()
	lookup := newTestInvLookup()
	runInvOpExpectErrAsPlayer(t, OpInvDropItem, []int{9999, validCoord(0, 3200, 3200), testObjCoin, 1, 100}, lookup, mc, "no InvType with value (9999) found")
}

func TestInvDropItem_CoordInvalid(t *testing.T) {
	mc := newTestConfigs()
	lookup := newTestInvLookup()
	runInvOpExpectErrAsPlayer(t, OpInvDropItem, []int{testInvMain, -1, testObjCoin, 1, 100}, lookup, mc, "INV_DROPITEM: coord out of range (-1)")
}

func TestInvDropItem_ObjTypeInvalid(t *testing.T) {
	mc := newTestConfigs()
	lookup := newTestInvLookup()
	runInvOpExpectErrAsPlayer(t, OpInvDropItem, []int{testInvMain, validCoord(0, 3200, 3200), 9999, 1, 100}, lookup, mc, "no ObjType with value (9999) found")
}

func TestInvDropItem_ObjStackInvalid(t *testing.T) {
	mc := newTestConfigs()
	lookup := newTestInvLookup()
	runInvOpExpectErrAsPlayer(t, OpInvDropItem, []int{testInvMain, validCoord(0, 3200, 3200), testObjCoin, 0, 100}, lookup, mc, "INV_DROPITEM: invalid count (0)")
}

func TestInvDropItem_DurationInvalid(t *testing.T) {
	mc := newTestConfigs()
	lookup := newTestInvLookup()
	runInvOpExpectErrAsPlayer(t, OpInvDropItem, []int{testInvMain, validCoord(0, 3200, 3200), testObjCoin, 1, 0}, lookup, mc, "INV_DROPITEM: duration out of range")
}

func TestInvDropItem_StackableSpawnsSingleStackedObj(t *testing.T) {
	mc := newTestConfigs()
	lookup := newTestInvLookup()
	from := lookup.Get(nil, testInvMain)
	from.Set(0, &inventory.Item{Id: testObjCoin, Count: 100}) // stackable
	world := newMockWorld()
	runInvOpWithWorld(t, OpInvDropItem, []int{testInvMain, validCoord(0, 3200, 3200), testObjCoin, 100, 200}, lookup, mc, world)
	if len(world.addObjCalls) != 1 {
		t.Fatalf("stackable: want 1 AddObj call, got %d", len(world.addObjCalls))
	}
	if world.addObjCalls[0].count != 100 {
		t.Errorf("AddObj count: got %d, want 100 (single stacked)", world.addObjCalls[0].count)
	}
}

func TestInvDropItem_NonStackableSpawnsPerItem(t *testing.T) {
	mc := newTestConfigs()
	lookup := newTestInvLookup()
	from := lookup.Get(nil, testInvMain)
	from.Set(0, &inventory.Item{Id: testObjSword, Count: 1})
	from.Set(1, &inventory.Item{Id: testObjSword, Count: 1})
	from.Set(2, &inventory.Item{Id: testObjSword, Count: 1})
	world := newMockWorld()
	runInvOpWithWorld(t, OpInvDropItem, []int{testInvMain, validCoord(0, 3200, 3200), testObjSword, 3, 200}, lookup, mc, world)
	if len(world.addObjCalls) != 3 {
		t.Fatalf("non-stackable count=3: want 3 AddObj calls, got %d", len(world.addObjCalls))
	}
	for i, c := range world.addObjCalls {
		if c.count != 1 {
			t.Errorf("call %d: got count=%d, want 1", i, c.count)
		}
	}
}

func TestInvDropItem_StackableCompletedOneSpawnsSingle(t *testing.T) {
	// Stackable + completed == 1 → falls through to per-item branch (`!Stackable || completed == 1`).
	mc := newTestConfigs()
	lookup := newTestInvLookup()
	from := lookup.Get(nil, testInvMain)
	from.Set(0, &inventory.Item{Id: testObjCoin, Count: 1})
	world := newMockWorld()
	runInvOpWithWorld(t, OpInvDropItem, []int{testInvMain, validCoord(0, 3200, 3200), testObjCoin, 1, 200}, lookup, mc, world)
	if len(world.addObjCalls) != 1 {
		t.Fatalf("want 1 AddObj call, got %d", len(world.addObjCalls))
	}
	if world.addObjCalls[0].count != 1 {
		t.Errorf("count: got %d, want 1", world.addObjCalls[0].count)
	}
}

func TestInvDropItem_RemoveZeroCompletedNoSpawn(t *testing.T) {
	mc := newTestConfigs()
	lookup := newTestInvLookup()
	world := newMockWorld()
	runInvOpWithWorld(t, OpInvDropItem, []int{testInvMain, validCoord(0, 3200, 3200), testObjCoin, 50, 200}, lookup, mc, world)
	if len(world.addObjCalls) != 0 {
		t.Errorf("empty inv: want 0 AddObj, got %d", len(world.addObjCalls))
	}
}

func TestInvDropItem_ActiveObjPointerSet(t *testing.T) {
	mc := newTestConfigs()
	lookup := newTestInvLookup()
	from := lookup.Get(nil, testInvMain)
	from.Set(0, &inventory.Item{Id: testObjCoin, Count: 5})
	world := newMockWorld()
	st := runInvOpWithWorld(t, OpInvDropItem, []int{testInvMain, validCoord(0, 3200, 3200), testObjCoin, 5, 200}, lookup, mc, world)
	if st.Pointers&PtrActiveObj == 0 {
		t.Error("PtrActiveObj should be set after successful drop")
	}
	if st.ActiveObj == nil {
		t.Error("ActiveObj should be set")
	}
}
```

**Note:** If `validCoord` / `runInvOpWithWorld` / `newMockWorld` don't exist, define them locally based on the INV_DROPSLOT test patterns (re-grep first; reuse where possible).

- [ ] **Step 6.4 — Run RED tests, confirm fail**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/ -run TestInvDropItem -count=1 -v
```

Expected: all 11 tests fail.

- [ ] **Step 6.5 — Add handler to `handlers_inv.go`**

Append after `handleInvMoveItemCert`:

```go
// handleInvDropItem (INV_DROPITEM) ports TS InvOps.ts:163-186. Pops
// [inv, coord, obj, count, duration]. Removes count of obj from inv,
// then drops the removed count to the world at coord. Stackable+completed>1
// spawns a single stacked Obj; non-stackable OR completed==1 spawns
// per-item. Sets ActiveObj + PtrActiveObj after each spawn (last-wins).
//
// Validator chain (NAI-131): InvTypeValid → CoordValid → ObjTypeValid
// → ObjStackValid → DurationValid → protect/scope.
//
// DEVIATION-NAI-130-D2: defensive nil-World guard returns clean error.
func handleInvDropItem(s *ScriptState) error {
	if err := requireActivePlayer(s, "INV_DROPITEM"); err != nil {
		return err
	}
	duration := s.PopInt()
	count := s.PopInt()
	obj := s.PopInt()
	coord := s.PopInt()
	invID := s.PopInt()

	if err := checkInvType(s, invID, "INV_DROPITEM"); err != nil {
		return err
	}
	level, x, z, err := checkCoord(coord, "INV_DROPITEM")
	if err != nil {
		return err
	}
	if err := checkObjType(s, obj, "INV_DROPITEM"); err != nil {
		return err
	}
	if err := checkObjStack(count, "INV_DROPITEM"); err != nil {
		return err
	}
	if err := checkDuration(duration); err != nil {
		return fmt.Errorf("INV_DROPITEM: %w", err)
	}

	invType := s.Configs.InvType(invID)
	if invType.Protect && invType.Scope != objtype.InvTypeScopeShared && !s.Protect {
		return fmt.Errorf("INV_DROPITEM: $inv requires protected access: %s", invType.DebugName)
	}

	inv := resolveInv(s, invID)
	if inv == nil {
		return fmt.Errorf("INV_DROPITEM: inv unresolved (id=%d)", invID)
	}
	tx := inv.Remove(obj, count, inventory.RemoveOpts{BeginSlot: -1})
	completed := tx.Completed
	if completed == 0 {
		return nil
	}
	if s.World == nil {
		return fmt.Errorf("INV_DROPITEM: no world surface")
	}
	objType := s.Configs.ObjType(obj)
	receiverID := s.Self.UID()
	if !objType.Stackable || completed == 1 {
		for range completed {
			o := s.World.AddObj(level, x, z, obj, 1, duration, receiverID)
			if o != nil {
				s.ActiveObj = o
				s.Pointers |= PtrActiveObj
			}
		}
	} else {
		o := s.World.AddObj(level, x, z, obj, completed, duration, receiverID)
		if o != nil {
			s.ActiveObj = o
			s.Pointers |= PtrActiveObj
		}
	}
	return nil
}
```

- [ ] **Step 6.6 — Wire dispatch entry**

In `pkg/script/handlers.go`, add after `OpInvMoveItemCert:`:

```go
	OpInvDropItem:       handleInvDropItem,
```

- [ ] **Step 6.7 — Run RED tests, confirm GREEN**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/ -run TestInvDropItem -count=1 -v
```

Expected: all 11 tests PASS.

- [ ] **Step 6.8 — Run full package + modules/world (full TC)**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/... ./pkg/inventory/... ./modules/world/... -count=1
```

Expected: ALL PASS.

- [ ] **Step 6.9 — Commit**

```bash
git add pkg/script/handlers_inv.go pkg/script/handlers.go pkg/script/handlers_inv_test.go
git commit --no-gpg-sign -m "feat(nai-132): T6 — INV_DROPITEM handler (GREEN)

Ports TS InvOps.ts:163-186. Mirrors INV_DROPSLOT shape: inv.Remove →
stackable single-stacked vs non-stackable per-item branch → ActiveObj
last-wins. Validators: InvType/Coord/ObjType/ObjStack/Duration/protect.
DEVIATION-NAI-130-D2 inherited (defensive nil-World)."
```

---

## Task 7 — Close commit + memory updates

**Files:**
- Modify: `~/.claude/projects/-home-owner-Code-github-com-zsrv-goscape/memory/nai_followups.md` (carry-forward updates — note: outside repo, not staged via `git add`)

**Goal:** NAI-132 close commit per `close_commit_memory_trailer`. Memory updates capture:
- Retire `inventory.Remove` minmax carryover (closed by T2).
- Add NAI-133+ candidate: BOTH_MOVEINV (Self2-protect infra prerequisite).
- Add NAI-134+ candidate: INV_DROPITEM_DELAYED (delayed-obj queue infra prerequisite).
- Surface any session-learned non-derivable facts (e.g., updated risk-register premises if implementer found new ones).

- [ ] **Step 7.1 — Update `nai_followups.md` carry-forward routing**

Edit `$HOME/.claude/projects/-home-owner-Code-github-com-zsrv-goscape/memory/nai_followups.md`. Replace the existing `**NAI-132+ candidate (style-cleanup, still queued):**` line with:

```markdown
- **NAI-133+ candidate (NAI-132 deferred):** `BOTH_MOVEINV` opcode 4301. TS InvOps.ts:373-495 indexes `ProtectedActivePlayer[secondary?1:0]` / `[secondary?0:1]` for per-pointer-slot protect tracking. Goscape's `s.Protect` is a single bool (state.go:315). Prerequisites: (1) new `Self2Protect bool` field on ScriptState OR `PtrProtectedActivePlayer2` Pointer flag, (2) P_PROTECT routing on `state.intOperand` to set the appropriate slot, (3) BOTH_MOVEINV handler with `!fromPlayerProtect` / `!toPlayerProtect` per gate. Plus DEVIATION-NAI-115-D1 wealth-event-tail skip (TS InvOps.ts:445-494; goscape skips per established pattern). ~140-180 LOC.
- **NAI-134+ candidate (NAI-132 deferred):** `INV_DROPITEM_DELAYED` opcode 4310. TS InvOps.ts:188-209 enqueues an `ObjDelayedRequest` onto `World.objDelayedQueue`. Goscape has no analog. Prerequisites: delayed-spawn queue infra (audit npc-respawn / obj-respawn for adjacent primitives). ~100+ LOC infra + 50 LOC handler.
```

If the spec-write also stale-retired the original `NAI-132+ candidate` for inventory.Remove minmax (now closed by T2), confirm that's been removed from the followups file or replace correspondingly.

- [ ] **Step 7.2 — Sanity-check entire test suite**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./... -count=1
```

Expected: ALL PASS (full repo).

- [ ] **Step 7.3 — Close commit**

```bash
git add pkg/script/ pkg/inventory/
git commit --no-gpg-sign --allow-empty -m "$(cat <<'EOF'
chore(nai-132): close — INV_* handler ports (CHANGESLOT/DROPITEM/CERT/UNCERT) + MOVETOSLOT validator-backfill + Remove minmax

T1: INV_MOVETOSLOT validator-backfill (NAI-131 spec-error fixup; 5 gates)
T2: inventory.Remove min/max modernization (final NAI-126 carryover)
T3: INV_CHANGESLOT — InvType + protect/scope + ObjType×2 (TS skips ObjStackValid; absence-pinned)
T4: INV_MOVEITEM_UNCERT — invDel + cert→link resolve + invAdd, no overflow
T5: INV_MOVEITEM_CERT — invDel + cert→link resolve (inverted) + invAdd + single-stacked overflow drop
T6: INV_DROPITEM — InvType/Coord/ObjType/ObjStack/Duration/protect + Remove + stackable-or-per-item AddObj + ActiveObj last-wins

DEVIATION-NAI-131-D1 (dual-from-scope) inherited by T1/T4/T5; D2 (defensive nil-World) by T5/T6; D3 (defensive nil-Configs) by T4/T5.

Defers BOTH_MOVEINV → NAI-133+ (Self2-protect infra prerequisite),
INV_DROPITEM_DELAYED → NAI-134+ (delayed-obj queue infra prerequisite).

Closes memory: nai_followups.md (NAI-132 carryover routing updated)
EOF
)"
```

---

## Self-Review Notes

- **Spec coverage:** T1 (§4 T1) ✓, T2 (§4 T2) ✓, T3 (§4 T3) ✓, T4 (§4 T4) ✓, T5 (§4 T5) ✓, T6 (§4 T6) ✓. Deferred BOTH_MOVEINV/DROPITEM_DELAYED tracked in §2 + plan T7.
- **Test strategy (§5):** every per-handler GREEN regression pin called out in spec is covered by at least one test in the plan.
- **Deviations (§6):** D1/D2/D3 inheritance correctly applied per task in handler doc-comments.
- **Risk register (§7):** ObjType seed shape verified via test_helpers.go grep; checkCoord/checkDuration literals verified; inv.Type field confirmed.
- **Helper reuse:** `runInvOpExpectErr`, `runInvOpExpectErrAsPlayer`, `runInvOp` (NAI-131 T0). `runInvOpWithWorld` may need to be added; flagged in T5/T6 as implementer judgment-call.
- **Validation literal pinning:** test substrings match `checkInvType` / `checkObjType` / `checkObjStack` / `checkCoord` / `checkDuration` actual error literals (verified via grep).
