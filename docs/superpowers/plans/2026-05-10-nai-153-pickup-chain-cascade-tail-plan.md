# NAI-153 — pickup-chain cascade-tail Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Wire OBJ_COUNT (3503) and OBJ_TAKEITEM (3510) script handlers so the Java-client mindrune-pickup smoke completes (item enters inventory; ground tile clears).

**Architecture:** Extract `performInvAdd` from existing `handleInvAdd` so OBJ_TAKEITEM can call into the validated invAdd path without going through PopInt-driven dispatch. Extend the `script.ActiveObj` interface with `ObjCount() int` + `IsValidFor(playerUID int) bool` and implement on `*entity.Obj` (using a distinct method name from the existing no-arg `IsValid()` to avoid breaking the polymorphic `entity` interface). Wire two new handlers in `pkg/script/handlers_obj.go` mirroring TS `ObjOps.ts:121-130` and `:137-161`. Wealth-event inlining skipped per NAI-115-D1 precedent.

**Tech Stack:** Go 1.26+; `pkg/script`, `pkg/entity`. No new deps.

**Spec:** `docs/superpowers/specs/2026-05-10-nai-153-pickup-chain-cascade-tail-design.md`

---

## File structure

| Path | Action | Responsibility |
|---|---|---|
| `pkg/script/handlers_inv.go` | Modify | Refactor `handleInvAdd` body into shared `performInvAdd` helper (T1) |
| `pkg/script/handlers_inv_test.go` | Modify | Add `TestPerformInvAdd_DirectCall` (T1) |
| `pkg/script/active.go` | Modify | Extend `ActiveObj` interface with `ObjCount`, `IsValidFor` (T2) |
| `pkg/entity/obj.go` | Modify | Add `ObjCount() int` and `IsValidFor(playerUID int) bool` methods on `*Obj`; leave existing no-arg `IsValid() bool` untouched (T2) |
| `pkg/entity/obj_test.go` | Create | Unit tests for `IsValidFor` and `ObjCount` (T2) |
| `pkg/script/handlers_npc_test.go` | Modify | Extend `mockActiveObj` struct with `count`, `receiverID`, `reveal` fields and `ObjCount()`, `IsValidFor()` methods (T2) |
| `pkg/script/handlers_obj.go` | Modify | Add `handleObjCount` (T3) and `handleObjTakeItem` (T4); extend NAI-115-D2 doc-comment on `handleObjDel` to mention TAKEITEM (T4) |
| `pkg/script/handlers.go` | Modify | Register `OpObjCount` (T3) and `OpObjTakeItem` (T4) in OBJ family map block at line 120-127 |
| `pkg/script/handlers_obj_test.go` | Modify | Add `handleObjCount` tests (T3) and `handleObjTakeItem` tests (T4) |

---

## Preflight (controller, before dispatching T1+T2 in parallel)

Per `controller_preflight.md` — verify HEAD state before each implementer dispatch:

```bash
# Confirm seed claims hold
grep -n "OpObjCount\|OpObjTakeItem" pkg/script/handlers.go    # must show NO matches
grep -n "ObjCount\|IsValidFor" pkg/entity/obj.go              # must show NO matches
grep -n "performInvAdd" pkg/script/handlers_inv.go            # must show NO matches
grep -n "handleObjType" pkg/script/handlers.go                # must show line 124 (B2 T1 still landed)

# R3 mitigation: enumerate mockActiveObj sites
grep -rn "mockActiveObj" pkg/script/                          # 1 def site, ~12 use sites

# R2 verification: existing no-arg IsValid call sites that constrain T2
grep -rn "\.IsValid()" pkg/zone/ modules/world/ pkg/script/ --include='*.go' | grep -v _test
# Expected: 6+ sites in npc_interaction.go, npc_interaction_trigger.go, pkg/zone/zone.go
# All consume the polymorphic entity interface; the T2 plan adds IsValidFor(uid)
# alongside, NOT replacing IsValid(). Zero rename pressure.
```

If any expectation fails, STOP and reframe before dispatch.

---

## Task 1: Extract `performInvAdd` (refactor; no behavior change)

**Files:**
- Modify: `pkg/script/handlers_inv.go:318-382` (`handleInvAdd` body)
- Test: `pkg/script/handlers_inv_test.go` (append new test)

T1 and T2 are independent and may be dispatched in parallel.

### Step 1.1: Read the existing `handleInvAdd` body

- [ ] **Step 1.1: Open `pkg/script/handlers_inv.go` and Read lines 293-404 in full**

Confirm the body matches the verbatim copy below (Step 1.3). If it diverges, STOP and re-derive from current HEAD.

### Step 1.2: Write the failing test

- [ ] **Step 1.2: Append the new test to `pkg/script/handlers_inv_test.go`**

```go
// TestPerformInvAdd_DirectCall pins the contract that performInvAdd
// can be called with already-typed args, bypassing the PopInt-driven
// handleInvAdd wrapper. Mirrors the same validation chain + Inventory.Add
// path that handleInvAdd takes; this test focuses on the direct-call
// happy path so OBJ_TAKEITEM (NAI-153 T4) can rely on the shared impl.
func TestPerformInvAdd_DirectCall(t *testing.T) {
	s := newTestState(minimalScript(OpReturn))
	s.Self = &mockPlayer{uidValue: 12345}
	s.Pointers |= PtrActivePlayer

	mc := newTestInvConfigs()
	invType := objtype.NewInvType(93)
	invType.Size = 28
	mc.invs[93] = invType
	mindrune := objtype.NewObjType(558)
	mindrune.Stackable = false
	mc.objs[558] = mindrune
	s.Configs = mc

	inv := inventory.New(93, 28, inventory.StackNormal)
	s.Inv = &mockInvLookup{invs: map[int]*inventory.Inventory{93: inv}}

	if err := performInvAdd(s, 93, 558, 1, "TEST"); err != nil {
		t.Fatalf("performInvAdd returned error: %v", err)
	}

	got := inv.Get(0)
	if got == nil || got.Id != 558 || got.Count != 1 {
		t.Errorf("performInvAdd: inv slot 0 got %+v, want {Id:558 Count:1}", got)
	}
}
```

### Step 1.3: Run the test to verify it fails

- [ ] **Step 1.3: Run the test (must fail with "performInvAdd undefined")**

```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/ -run TestPerformInvAdd_DirectCall -v
```

Expected: FAIL with `undefined: performInvAdd` compile error.

### Step 1.4: Refactor `handleInvAdd` into wrapper + `performInvAdd`

- [ ] **Step 1.4: Replace the body of `handleInvAdd` at `pkg/script/handlers_inv.go:318-382`**

The new file region (replacing the existing `handleInvAdd` from line 318 through its closing `}` at 382):

```go
func handleInvAdd(s *ScriptState) error {
	if err := requireActivePlayer(s, "INV_ADD"); err != nil {
		return err
	}
	count := s.PopInt()
	obj := s.PopInt()
	typeID := s.PopInt()
	return performInvAdd(s, typeID, obj, count, "INV_ADD")
}

// performInvAdd is the shared invAdd impl. Mirrors TS Player.invAdd —
// the method that both INV_ADD opcode and OBJ_TAKEITEM call. Validates
// invType + objType + count, enforces protect/scope + dummyitem gates,
// resolves the inv, routes via Inventory.Add, and drops overflow at
// the player's tile.
//
// Pre-conditions: caller has invoked requireActivePlayer (s.Self is
// dereferenced for the overflow drop). Inputs are raw script ints;
// performInvAdd does its own check chain so each call site stays minimal.
func performInvAdd(s *ScriptState, typeID, obj, count int, op string) error {
	// TS InvOps.ts:60-62 — InvTypeValid, ObjTypeValid, ObjStackValid.
	if err := checkInvType(s, typeID, op); err != nil {
		return err
	}
	if err := checkObjType(s, obj, op); err != nil {
		return err
	}
	if err := checkObjStack(count, op); err != nil {
		return err
	}

	invType := s.Configs.InvType(typeID)
	objType := s.Configs.ObjType(obj)

	// TS InvOps.ts:64-66 — protect/scope gate.
	if invType.Protect && invType.Scope != objtype.InvTypeScopeShared && s.Pointers&PtrProtectedActivePlayer == 0 {
		return fmt.Errorf("%s: $inv requires protected access: %s", op, invType.DebugName)
	}

	// TS InvOps.ts:68-70 — dummyitem-in-non-dummyinv gate.
	if !invType.DummyInv && objType.DummyItem != 0 {
		return fmt.Errorf("%s: dummyitem in non-dummyinv: %s -> %s", op, objType.DebugName, invType.DebugName)
	}

	inv := resolveInv(s, typeID)
	if inv == nil {
		// Defensive: unreachable post-checkInvType for valid configs;
		// retained for the InvLookup-unset case (s.Inv == nil → resolveInv returns nil).
		return fmt.Errorf("%s: no inv for type %d", op, typeID)
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

**Behavior preservation:** the body is verbatim modulo:
- Error literals use `%s` formatted with `op` (was hardcoded `"INV_ADD"`); INV_ADD callers get identical strings since `op = "INV_ADD"`.

### Step 1.5: Run the new test + the entire `handleInvAdd` regression suite

- [ ] **Step 1.5: Verify both pass**

```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/ -run "TestPerformInvAdd_DirectCall|TestHandleInvAdd" -v
```

Expected: all PASS.

### Step 1.6: Run the full pkg/script test suite + race detector

- [ ] **Step 1.6: Cross-bundle regression**

```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race ./pkg/script/...
```

Expected: PASS.

### Step 1.7: Commit

- [ ] **Step 1.7: Commit**

```bash
git add pkg/script/handlers_inv.go pkg/script/handlers_inv_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
refactor(script): NAI-153 T1 — extract performInvAdd helper

Split handleInvAdd into thin pop-wrapper + performInvAdd(s, typeID,
obj, count, op) so NAI-153 T4 (OBJ_TAKEITEM) can call into the
validated invAdd chain without going through PopInt-driven dispatch.

Body extracted verbatim from the existing handleInvAdd; error-literal
"INV_ADD" replaced with %s + op so other callers can reuse with their
own opcode tag. INV_ADD callers see identical error strings.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 2: ActiveObj surface + `*entity.Obj` methods

**Files:**
- Modify: `pkg/script/active.go:910-913` (`ActiveObj` interface)
- Modify: `pkg/entity/obj.go` (add methods on `*Obj`)
- Create: `pkg/entity/obj_test.go` (unit tests for `IsValidFor`)
- Modify: `pkg/script/handlers_npc_test.go:2412-2417` (`mockActiveObj` extension)

T2 is independent of T1; both can be dispatched in parallel.

### Step 2.1: Write failing tests for `*entity.Obj.IsValidFor` and `ObjCount`

- [ ] **Step 2.1: Create `pkg/entity/obj_test.go`**

```go
package entity

import "testing"

func TestObj_ObjCount(t *testing.T) {
	o := NewObj(0, 3200, 3200, LifecycleRespawn, 558, 7)
	if got := o.ObjCount(); got != 7 {
		t.Errorf("ObjCount: got %d, want 7", got)
	}
}

func TestObj_IsValidFor_Public(t *testing.T) {
	// Public obj (Reveal == -1): valid for any playerUID.
	o := NewObj(0, 3200, 3200, LifecycleRespawn, 558, 1)
	if !o.IsValidFor(12345) {
		t.Errorf("IsValidFor(public obj, any uid): got false, want true")
	}
	if !o.IsValidFor(-1) {
		t.Errorf("IsValidFor(public obj, uid -1): got false, want true")
	}
}

func TestObj_IsValidFor_PrivateSelf(t *testing.T) {
	// Private obj (Reveal > -1) where playerUID matches ReceiverID: valid.
	o := NewObj(0, 3200, 3200, LifecycleDespawn, 558, 1)
	o.Reveal = 50
	o.ReceiverID = 12345
	if !o.IsValidFor(12345) {
		t.Errorf("IsValidFor(private obj, matching uid): got false, want true")
	}
}

func TestObj_IsValidFor_PrivateOther(t *testing.T) {
	// Private obj where playerUID does NOT match: invalid.
	o := NewObj(0, 3200, 3200, LifecycleDespawn, 558, 1)
	o.Reveal = 50
	o.ReceiverID = 12345
	if o.IsValidFor(99999) {
		t.Errorf("IsValidFor(private obj, non-matching uid): got true, want false")
	}
}

func TestObj_IsValidFor_DepletedCount(t *testing.T) {
	// Count < 1: invalid regardless of receiver state.
	o := NewObj(0, 3200, 3200, LifecycleRespawn, 558, 0)
	if o.IsValidFor(12345) {
		t.Errorf("IsValidFor(public obj, count=0): got true, want false")
	}
}

func TestObj_IsValid_NoArg_StillTrue(t *testing.T) {
	// Regression guard: the existing no-arg IsValid() (intrinsic base)
	// must still return true. The polymorphic entity interface at
	// modules/world/movement_consts.go:45-49 depends on this method
	// signature — DO NOT remove or rename.
	o := NewObj(0, 3200, 3200, LifecycleRespawn, 558, 1)
	if !o.IsValid() {
		t.Errorf("IsValid() (no-arg): got false, want true (intrinsic base)")
	}
}
```

### Step 2.2: Run the tests to verify they fail

- [ ] **Step 2.2: Verify FAIL**

```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/entity/ -run "TestObj_" -v
```

Expected: FAIL with `o.ObjCount undefined` and `o.IsValidFor undefined`. The `TestObj_IsValid_NoArg_StillTrue` should PASS (existing method unchanged).

### Step 2.3: Add the new methods to `*entity.Obj`

- [ ] **Step 2.3: Append to `pkg/entity/obj.go` (after the existing `ObjType()` method at line 66)**

```go
// ObjCount returns the obj's current stack size. Method wrapper around
// the public Count field so *Obj satisfies script.ActiveObj. (Go
// disallows same-name field + method; same convention as ObjType().)
func (o *Obj) ObjCount() int { return o.Count }

// IsValidFor reports whether the obj is consumable by the given player
// UID. Mirrors TS Obj.ts:52-62 with goscape's UID-int receiver instead
// of TS bigint hash64. Reveal>-1 means private; non-receiver players
// see invalid. Count<1 means depleted.
//
// NAI-153-D2: TS uses hash64 (bigint username hash); goscape uses
// ReceiverID = composeUID(username37, slot) per
// modules/world/server_varp.go:169.
//
// Distinct from the no-arg IsValid() (intrinsic base, always true)
// which satisfies the polymorphic entity interface — Go disallows
// method overloading, so the player-aware variant gets its own name.
func (o *Obj) IsValidFor(playerUID int) bool {
	if o.Reveal > -1 && playerUID != o.ReceiverID {
		return false
	}
	if o.Count < 1 {
		return false
	}
	return true
}
```

### Step 2.4: Extend the `ActiveObj` interface

- [ ] **Step 2.4: Replace `pkg/script/active.go:910-913`**

Current:
```go
type ActiveObj interface {
	ObjType() int              // underlying ObjType id
	Coords() (x, z, level int) // world position
}
```

Replacement:
```go
type ActiveObj interface {
	ObjType() int                  // underlying ObjType id
	Coords() (x, z, level int)     // world position
	ObjCount() int                 // current stack size; consumed by OBJ_COUNT, OBJ_TAKEITEM (NAI-153)
	IsValidFor(playerUID int) bool // private-receiver + count>0 (NAI-153); see *entity.Obj.IsValidFor
}
```

### Step 2.5: Extend `mockActiveObj` test fixture

- [ ] **Step 2.5: Replace `pkg/script/handlers_npc_test.go:2411-2417`**

Current:
```go
// mockActiveObj is a minimal ActiveObj fixture for NPC_SETMODE OPOBJ tests.
type mockActiveObj struct {
	objType, x, z, level int
}

func (m *mockActiveObj) ObjType() int              { return m.objType }
func (m *mockActiveObj) Coords() (x, z, level int) { return m.x, m.z, m.level }
```

Replacement:
```go
// mockActiveObj is a minimal ActiveObj fixture used across the
// pkg/script test suite. Extended for NAI-153 with count, receiverID,
// reveal so OBJ_COUNT / OBJ_TAKEITEM tests can drive the IsValidFor
// branches. Existing call sites that construct mockActiveObj{objType,
// x, z, level} continue to work — the new fields default to their zero
// values, which by Reveal=0 (>-1) makes the obj "private to receiver
// 0" — but every such call site sets ActiveObj for handlers that don't
// inspect IsValidFor (OBJ_COORD, OBJ_TYPE, NPC_SETMODE OPOBJ), so the
// default is benign.
type mockActiveObj struct {
	objType, x, z, level   int
	count                  int
	receiverID             int
	reveal                 int
}

func (m *mockActiveObj) ObjType() int              { return m.objType }
func (m *mockActiveObj) Coords() (x, z, level int) { return m.x, m.z, m.level }
func (m *mockActiveObj) ObjCount() int             { return m.count }
func (m *mockActiveObj) IsValidFor(playerUID int) bool {
	if m.reveal > -1 && playerUID != m.receiverID {
		return false
	}
	if m.count < 1 {
		return false
	}
	return true
}
```

**Note on default Reveal=0:** Go's zero value for `int` is 0, which satisfies `Reveal > -1` (treated as private). For existing call sites that don't care about IsValidFor, this is fine — they don't call it. For NAI-153 T3/T4 tests that DO drive IsValidFor, set `reveal: -1` explicitly for "public" or `reveal: 50, receiverID: <uid>` for "private to UID".

### Step 2.6: Run all tests to verify the surface change compiles + passes

- [ ] **Step 2.6: Run regression**

```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/entity/ ./pkg/script/ -v
```

Expected: all PASS. If any existing `mockActiveObj{...}` call site fails to compile, that's a fixture-extension miss — review the grep output from Preflight (`grep -rn "mockActiveObj" pkg/script/`) and re-confirm every site continues to satisfy the extended interface (struct literals with named fields keep working since the new fields default to zero).

### Step 2.7: Cross-bundle regression with race detector

- [ ] **Step 2.7: Full repo regression**

```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race ./...
```

Expected: PASS.

### Step 2.8: Commit

- [ ] **Step 2.8: Commit**

```bash
git add pkg/entity/obj.go pkg/entity/obj_test.go pkg/script/active.go pkg/script/handlers_npc_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(entity): NAI-153 T2 — Obj.ObjCount + IsValidFor methods

Extend script.ActiveObj interface with ObjCount() int and
IsValidFor(playerUID int) bool. Implement on *entity.Obj using
distinct names from existing fields/methods (Count field, no-arg
IsValid()).

R2 resolution: the no-arg IsValid() (intrinsic base, always true) is
required by the polymorphic entity interface at
modules/world/movement_consts.go:45-49 with 6+ consumers. Player-aware
variant uses IsValidFor to avoid Go's no-method-overloading constraint
and a rename cascade. Regression test pins the no-arg IsValid()
behavior.

mockActiveObj fixture extended with count, receiverID, reveal fields;
existing call sites unaffected (named-field struct literals continue
to compile with zero values).

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 3: `handleObjCount` (depends on T2)

**Files:**
- Modify: `pkg/script/handlers_obj.go` (append `handleObjCount` after `handleObjType`)
- Modify: `pkg/script/handlers.go:120-127` (register `OpObjCount`)
- Modify: `pkg/script/handlers_obj_test.go` (append tests after the OBJ_TYPE tests)

### Step 3.1: Write failing tests

- [ ] **Step 3.1: Append to `pkg/script/handlers_obj_test.go`**

```go
// --- NAI-153 T3: OBJ_COUNT handler --------------------------------------

func TestHandleObjCount_PushesCount_WhenValid(t *testing.T) {
	s := newTestState(minimalScript(OpReturn))
	s.Self = &mockPlayer{uidValue: 12345}
	s.Pointers |= PtrActivePlayer
	// Public obj (reveal: -1): IsValidFor(any) returns true.
	s.ActiveObj = &mockActiveObj{objType: 558, x: 3200, z: 3200, level: 0, count: 7, reveal: -1}

	if err := handleObjCount(s); err != nil {
		t.Fatalf("handleObjCount returned error: %v", err)
	}
	if got := s.PopInt(); got != 7 {
		t.Errorf("OBJ_COUNT (valid public): got %d, want 7", got)
	}
}

func TestHandleObjCount_PushesCount_WhenPrivateSelf(t *testing.T) {
	s := newTestState(minimalScript(OpReturn))
	s.Self = &mockPlayer{uidValue: 12345}
	s.Pointers |= PtrActivePlayer
	// Private obj where receiverID matches Self.UID: IsValidFor(12345) = true.
	s.ActiveObj = &mockActiveObj{objType: 558, x: 3200, z: 3200, level: 0, count: 3, reveal: 50, receiverID: 12345}

	if err := handleObjCount(s); err != nil {
		t.Fatalf("handleObjCount returned error: %v", err)
	}
	if got := s.PopInt(); got != 3 {
		t.Errorf("OBJ_COUNT (valid private-to-self): got %d, want 3", got)
	}
}

func TestHandleObjCount_PushesZero_WhenPrivateOther(t *testing.T) {
	s := newTestState(minimalScript(OpReturn))
	s.Self = &mockPlayer{uidValue: 12345}
	s.Pointers |= PtrActivePlayer
	// Private obj with non-matching receiver: IsValidFor(12345) = false → push 0.
	s.ActiveObj = &mockActiveObj{objType: 558, x: 3200, z: 3200, level: 0, count: 7, reveal: 50, receiverID: 99999}

	if err := handleObjCount(s); err != nil {
		t.Fatalf("handleObjCount returned error: %v", err)
	}
	if got := s.PopInt(); got != 0 {
		t.Errorf("OBJ_COUNT (private-to-other): got %d, want 0", got)
	}
}

func TestHandleObjCount_PushesZero_WhenDepleted(t *testing.T) {
	s := newTestState(minimalScript(OpReturn))
	s.Self = &mockPlayer{uidValue: 12345}
	s.Pointers |= PtrActivePlayer
	// Public obj with count=0: IsValidFor returns false (count<1) → push 0.
	s.ActiveObj = &mockActiveObj{objType: 558, x: 3200, z: 3200, level: 0, count: 0, reveal: -1}

	if err := handleObjCount(s); err != nil {
		t.Fatalf("handleObjCount returned error: %v", err)
	}
	if got := s.PopInt(); got != 0 {
		t.Errorf("OBJ_COUNT (depleted): got %d, want 0", got)
	}
}

func TestHandleObjCount_NoActiveObj(t *testing.T) {
	s := newTestState(minimalScript(OpReturn))
	s.Self = &mockPlayer{uidValue: 12345}
	s.Pointers |= PtrActivePlayer
	// s.ActiveObj == nil

	if err := handleObjCount(s); err == nil {
		t.Errorf("OBJ_COUNT: expected error on nil ActiveObj, got nil")
	}
}

func TestHandleObjCount_NoActivePlayer(t *testing.T) {
	s := newTestState(minimalScript(OpReturn))
	s.ActiveObj = &mockActiveObj{objType: 558, x: 3200, z: 3200, level: 0, count: 7, reveal: -1}
	// s.Self == nil; PtrActivePlayer not set

	if err := handleObjCount(s); err == nil {
		t.Errorf("OBJ_COUNT: expected error on nil Self, got nil")
	}
}
```

### Step 3.2: Run tests to verify FAIL

- [ ] **Step 3.2: Verify FAIL**

```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/ -run "TestHandleObjCount" -v
```

Expected: FAIL with `undefined: handleObjCount`.

### Step 3.3: Implement `handleObjCount`

- [ ] **Step 3.3: Append to `pkg/script/handlers_obj.go` (after `handleObjType` at line 176)**

```go
// handleObjCount (OBJ_COUNT, opcode 3503) pushes the active obj's
// count if it's valid for the active player; else pushes 0. Mirrors
// TS ObjOps.ts:121-130:
//
//	const obj: Obj = state.activeObj;
//	if (obj.isValid(state.activePlayer.hash64)) {
//	    state.pushInt(state.activeObj.count);
//	    return;
//	}
//	state.pushInt(0);
//
// goscape uses Self.UID() (composeUID-shaped int) instead of TS bigint
// hash64. See NAI-153-D2 in the spec.
func handleObjCount(s *ScriptState) error {
	if err := requireActiveObj(s, "OBJ_COUNT"); err != nil {
		return err
	}
	if err := requireActivePlayer(s, "OBJ_COUNT"); err != nil {
		return err
	}
	if s.ActiveObj.IsValidFor(s.Self.UID()) {
		s.PushInt(s.ActiveObj.ObjCount())
		return nil
	}
	s.PushInt(0)
	return nil
}
```

### Step 3.4: Register the handler

- [ ] **Step 3.4: Edit `pkg/script/handlers.go:124`**

Replace:
```go
	OpObjType:     handleObjType, // NAI-152 B2 T1
```

With:
```go
	OpObjType:     handleObjType, // NAI-152 B2 T1
	OpObjCount:    handleObjCount, // NAI-153 T3
```

### Step 3.5: Run tests to verify PASS

- [ ] **Step 3.5: Verify PASS**

```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/ -run "TestHandleObjCount" -v
```

Expected: all 6 cases PASS.

### Step 3.6: Cross-bundle regression with race detector

- [ ] **Step 3.6: Full repo regression**

```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race ./...
```

Expected: PASS.

### Step 3.7: Commit

- [ ] **Step 3.7: Commit**

```bash
git add pkg/script/handlers_obj.go pkg/script/handlers_obj_test.go pkg/script/handlers.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(script): NAI-153 T3 — register OBJ_COUNT handler

Port TS ObjOps.ts:121-130. Pushes obj.count if obj is valid for the
active player (per Obj.IsValidFor — NAI-153-D2 substitutes UID-int
receiver for TS bigint hash64); else pushes 0.

Closes the [label,pickup_obj_*] crash on `OBJ_COUNT (opcode 3503) at
pc=1` surfaced by the NAI-152 B2 PRIMARY smoke. Pickup chain still
crashes at OBJ_TAKEITEM (3510) until T4.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 4: `handleObjTakeItem` + smoke handoff (depends on T1, T2, T3)

**Files:**
- Modify: `pkg/script/handlers_obj.go` (extend NAI-115-D2 doc-comment on `handleObjDel`; append `handleObjTakeItem`)
- Modify: `pkg/script/handlers.go:120-127` (register `OpObjTakeItem`)
- Modify: `pkg/script/handlers_obj_test.go` (append tests after the OBJ_COUNT tests)

### Step 4.1: Write failing tests

- [ ] **Step 4.1a: Add imports to `pkg/script/handlers_obj_test.go`**

Current imports (lines 3-8):
```go
import (
	"testing"

	"github.com/zsrv/goscape/pkg/coordgrid"
	"github.com/zsrv/goscape/pkg/objtype"
)
```

Replacement:
```go
import (
	"strings"
	"testing"

	"github.com/zsrv/goscape/pkg/coordgrid"
	"github.com/zsrv/goscape/pkg/inventory"
	"github.com/zsrv/goscape/pkg/objtype"
)
```

- [ ] **Step 4.1b: Append to `pkg/script/handlers_obj_test.go`**

```go
// --- NAI-153 T4: OBJ_TAKEITEM handler -----------------------------------

// fakeWorldTakeItem combines RemoveObj recording (for OBJ_TAKEITEM
// removal) and AddObj recording (for performInvAdd overflow drop —
// expected zero in the happy path). Embeds *mockWorld for the rest of
// the WorldVars surface.
type fakeWorldTakeItem struct {
	*mockWorld
	removed    []ActiveObj
	addedCalls []addObjCall
}

func (f *fakeWorldTakeItem) RemoveObj(obj ActiveObj) {
	f.removed = append(f.removed, obj)
}

func (f *fakeWorldTakeItem) AddObj(level, x, z, typeID, count, duration, receiverID int) ActiveObj {
	f.addedCalls = append(f.addedCalls, addObjCall{level, x, z, typeID, count, duration, receiverID})
	return &mockActiveObj{objType: typeID, x: x, z: z, level: level}
}

// newTakeItemFixture builds the standard TAKEITEM happy-path harness:
// player UID 12345 with PtrActivePlayer set, mindrune (id 558) ObjType
// registered, inventory 93 (28 slots) registered, world recording
// RemoveObj/AddObj. Caller sets s.ActiveObj and pushes the invType.
func newTakeItemFixture(t *testing.T) (*ScriptState, *fakeWorldTakeItem, *inventory.Inventory) {
	t.Helper()
	s := newTestState(minimalScript(OpReturn))
	s.Self = &mockPlayer{uidValue: 12345}
	s.Pointers |= PtrActivePlayer

	w := &fakeWorldTakeItem{mockWorld: newMockWorld()}
	s.World = w

	mc := newTestInvConfigs()
	invType := objtype.NewInvType(93)
	invType.Size = 28
	mc.invs[93] = invType
	mindrune := objtype.NewObjType(558)
	mindrune.Stackable = false
	mc.objs[558] = mindrune
	s.Configs = mc

	inv := inventory.New(93, 28, inventory.StackNormal)
	s.Inv = &mockInvLookup{invs: map[int]*inventory.Inventory{93: inv}}

	return s, w, inv
}

func TestHandleObjTakeItem_HappyPath(t *testing.T) {
	s, w, inv := newTakeItemFixture(t)
	active := &mockActiveObj{objType: 558, x: 3200, z: 3200, level: 0, count: 1, reveal: -1}
	s.ActiveObj = active

	s.PushInt(93) // invType id

	if err := handleObjTakeItem(s); err != nil {
		t.Fatalf("OBJ_TAKEITEM: returned error: %v", err)
	}

	got := inv.Get(0)
	if got == nil || got.Id != 558 || got.Count != 1 {
		t.Errorf("OBJ_TAKEITEM: inv slot 0 got %+v, want {Id:558 Count:1}", got)
	}
	if len(w.removed) != 1 || w.removed[0] != active {
		t.Errorf("OBJ_TAKEITEM: expected 1 RemoveObj call with active, got %v", w.removed)
	}
	if len(w.addedCalls) != 0 {
		t.Errorf("OBJ_TAKEITEM: expected 0 AddObj calls (no overflow), got %v", w.addedCalls)
	}
}

func TestHandleObjTakeItem_InvalidObj_Noop(t *testing.T) {
	s, w, inv := newTakeItemFixture(t)
	// Private obj with non-matching receiver: IsValidFor returns false.
	s.ActiveObj = &mockActiveObj{objType: 558, x: 3200, z: 3200, level: 0, count: 1, reveal: 50, receiverID: 99999}

	s.PushInt(93)

	if err := handleObjTakeItem(s); err != nil {
		t.Fatalf("OBJ_TAKEITEM (invalid obj): returned error: %v, want nil (no-op)", err)
	}
	if got := inv.Get(0); got != nil {
		t.Errorf("OBJ_TAKEITEM (invalid obj): expected empty inv, got %+v", got)
	}
	if len(w.removed) != 0 {
		t.Errorf("OBJ_TAKEITEM (invalid obj): expected 0 RemoveObj calls, got %v", w.removed)
	}
}

func TestHandleObjTakeItem_DepletedObj_Noop(t *testing.T) {
	s, w, inv := newTakeItemFixture(t)
	// Public obj with count=0: IsValidFor returns false (count<1).
	s.ActiveObj = &mockActiveObj{objType: 558, x: 3200, z: 3200, level: 0, count: 0, reveal: -1}

	s.PushInt(93)

	if err := handleObjTakeItem(s); err != nil {
		t.Fatalf("OBJ_TAKEITEM (depleted obj): returned error: %v, want nil (no-op)", err)
	}
	if got := inv.Get(0); got != nil {
		t.Errorf("OBJ_TAKEITEM (depleted obj): expected empty inv, got %+v", got)
	}
	if len(w.removed) != 0 {
		t.Errorf("OBJ_TAKEITEM (depleted obj): expected 0 RemoveObj calls, got %v", w.removed)
	}
}

func TestHandleObjTakeItem_BadInvType(t *testing.T) {
	s, _, _ := newTakeItemFixture(t)
	s.ActiveObj = &mockActiveObj{objType: 558, x: 3200, z: 3200, level: 0, count: 1, reveal: -1}

	s.PushInt(99999) // unregistered invType id → checkInvType errors

	err := handleObjTakeItem(s)
	if err == nil {
		t.Fatalf("OBJ_TAKEITEM (bad invType): expected error, got nil")
	}
	if !strings.Contains(err.Error(), "OBJ_TAKEITEM") {
		t.Errorf("OBJ_TAKEITEM (bad invType): error tag missing 'OBJ_TAKEITEM': %v", err)
	}
}

func TestHandleObjTakeItem_NoActiveObj(t *testing.T) {
	s, _, _ := newTakeItemFixture(t)
	// s.ActiveObj == nil
	s.PushInt(93)

	if err := handleObjTakeItem(s); err == nil {
		t.Errorf("OBJ_TAKEITEM: expected error on nil ActiveObj, got nil")
	}
}

func TestHandleObjTakeItem_NoActivePlayer(t *testing.T) {
	s := newTestState(minimalScript(OpReturn))
	w := &fakeWorldTakeItem{mockWorld: newMockWorld()}
	s.World = w
	s.ActiveObj = &mockActiveObj{objType: 558, x: 3200, z: 3200, level: 0, count: 1, reveal: -1}
	// s.Self == nil; PtrActivePlayer not set
	s.PushInt(93)

	if err := handleObjTakeItem(s); err == nil {
		t.Errorf("OBJ_TAKEITEM: expected error on nil Self, got nil")
	}
}

func TestHandleObjTakeItem_NoWorld(t *testing.T) {
	s := newTestState(minimalScript(OpReturn))
	s.Self = &mockPlayer{uidValue: 12345}
	s.Pointers |= PtrActivePlayer
	s.ActiveObj = &mockActiveObj{objType: 558, x: 3200, z: 3200, level: 0, count: 1, reveal: -1}
	// s.World == nil
	s.PushInt(93)

	if err := handleObjTakeItem(s); err == nil {
		t.Errorf("OBJ_TAKEITEM: expected error on nil World, got nil")
	}
}
```

Both `strings` and `inventory` imports were added in Step 4.1a above.

### Step 4.2: Run tests to verify FAIL

- [ ] **Step 4.2: Verify FAIL**

```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/ -run "TestHandleObjTakeItem" -v
```

Expected: FAIL with `undefined: handleObjTakeItem`.

### Step 4.3: Implement `handleObjTakeItem`

- [ ] **Step 4.3: Append to `pkg/script/handlers_obj.go` (after `handleObjCount`)**

```go
// handleObjTakeItem (OBJ_TAKEITEM, opcode 3510) pops invType, validates,
// guards on isValid, adds the obj to the player's inv via performInvAdd,
// and removes the obj from the world. Mirrors TS ObjOps.ts:137-161.
//
// NAI-153-D1: TS calls activePlayer.addWealthEvent(...) between invAdd
// and removeObj. Skipped per NAI-115-D1 precedent — content can emit
// via OpWealthEvent (2131). (goscape defensive skip; TS inlines.)
//
// NAI-115-D2 (extended to TAKEITEM): TS calls World.removeObj(obj,
// respawnrate) for RESPAWN-lifecycle and World.removeObj(obj, 0) for
// DESPAWN. goscape's WorldVars.RemoveObj has no duration arg — both
// branches collapse to a single zero-arg RemoveObj call.
// RESPAWN-lifecycle respawn-after-delay remains a foundation gap
// (shared with OBJ_DEL; see handleObjDel).
func handleObjTakeItem(s *ScriptState) error {
	if err := requireActiveObj(s, "OBJ_TAKEITEM"); err != nil {
		return err
	}
	if err := requireActivePlayer(s, "OBJ_TAKEITEM"); err != nil {
		return err
	}
	if s.World == nil {
		return fmt.Errorf("OBJ_TAKEITEM: no world surface")
	}

	invID := s.PopInt()
	if err := checkInvType(s, invID, "OBJ_TAKEITEM"); err != nil {
		return err
	}

	if !s.ActiveObj.IsValidFor(s.Self.UID()) {
		return nil // TS returns false; goscape no-op (matches OBJ_DEL idiom)
	}

	if err := performInvAdd(s, invID, s.ActiveObj.ObjType(), s.ActiveObj.ObjCount(), "OBJ_TAKEITEM"); err != nil {
		return err
	}
	s.World.RemoveObj(s.ActiveObj)
	return nil
}
```

### Step 4.4: Extend the NAI-115-D2 doc-comment on `handleObjDel`

- [ ] **Step 4.4: Edit `pkg/script/handlers_obj.go:117-121`**

Current:
```go
// NAI-115-D2 deviation: TS reads ObjType.respawnrate and passes it to
// World.removeObj as duration. goscape's Server.RemoveObj has no
// duration arg; RESPAWN-lifecycle respawn-after-delay is a foundation
// gap. DESPAWN-lifecycle objs (the firemaking smoke target) are
// unaffected.
```

Replacement:
```go
// NAI-115-D2 deviation: TS reads ObjType.respawnrate and passes it to
// World.removeObj as duration. goscape's Server.RemoveObj has no
// duration arg; RESPAWN-lifecycle respawn-after-delay is a foundation
// gap. DESPAWN-lifecycle objs (the firemaking smoke target) are
// unaffected. NAI-153 T4 extends the same gap to OBJ_TAKEITEM, which
// also collapses TS's lifecycle-branched removeObj-with-duration into
// a single zero-arg RemoveObj.
```

### Step 4.5: Register the handler

- [ ] **Step 4.5: Edit `pkg/script/handlers.go` near line 124**

Replace:
```go
	OpObjType:     handleObjType, // NAI-152 B2 T1
	OpObjCount:    handleObjCount, // NAI-153 T3
```

With:
```go
	OpObjType:     handleObjType,     // NAI-152 B2 T1
	OpObjCount:    handleObjCount,    // NAI-153 T3
	OpObjTakeItem: handleObjTakeItem, // NAI-153 T4
```

### Step 4.6: Run tests to verify PASS

- [ ] **Step 4.6: Verify PASS**

```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/ -run "TestHandleObjTakeItem" -v
```

Expected: all 7 cases PASS.

### Step 4.7: Cross-bundle regression with race detector

- [ ] **Step 4.7: Full repo regression**

```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race ./...
```

Expected: PASS.

### Step 4.8: Commit

- [ ] **Step 4.8: Commit**

```bash
git add pkg/script/handlers_obj.go pkg/script/handlers_obj_test.go pkg/script/handlers.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(script): NAI-153 T4 — register OBJ_TAKEITEM handler

Port TS ObjOps.ts:137-161. Pops invType, guards via Obj.IsValidFor,
calls performInvAdd (NAI-153 T1) with already-typed args, and removes
the obj via WorldVars.RemoveObj.

NAI-153-D1: addWealthEvent inlining skipped per NAI-115-D1 precedent
(content emits via OpWealthEvent 2131).

NAI-115-D2 extended: doc-comment on handleObjDel now lists OBJ_TAKEITEM
as a sibling consumer of the RemoveObj-no-duration foundation gap.
RESPAWN-lifecycle respawn-after-delay does not fire; DESPAWN-lifecycle
works correctly.

Smoke handoff: pickup chain (OBJ_TYPE → OBJ_COUNT → OBJ_TAKEITEM) now
fully wired. Java-client mindrune-pickup smoke should reach inventory
and clear the ground tile.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

### Step 4.9: Smoke handoff (USER-LAUNCHED)

- [ ] **Step 4.9: Hand off to user for Java-client smoke**

Per `smoke_test_server_handoff.md`, the Java-client smoke runs against a user-launched goscape server (sandbox can't bind the world TCP port). Emit a smoke handoff message:

```
NAI-153 T4 landed. Ready for the deferred B2 §10 smoke gate.

Please launch the goscape server + Java client and reproduce:

1. Drop mindrune (id=558) on player tile
2. Right-click → Take

Acceptance:
- No "no handler for OBJ_COUNT" or "no handler for OBJ_TAKEITEM" log
- mindrune appears in inventory at the expected slot
- ground item disappears from the tile
- Off-tile pickup (1 tile away) shows identical pass shape

Deferred (NOT in gate): RESPAWN-lifecycle respawn-after-delay
(NAI-115-D2 foundation gap; mindrune ground tile stays empty until
server restart).

Adjacent surprises: route per smoke_surfaces_adjacent_divergences —
≤30 LOC stretch-in, larger to NAI-154.

Paste the relevant log snippet (or "smoke pass") to close NAI-153.
```

---

## Close commit (after smoke pass)

- [ ] **Close NAI-153**

```bash
git commit --no-gpg-sign --allow-empty -m "$(cat <<'EOF'
chore(close): NAI-153 — pickup chain landed; cascade-tail closed

Java-client mindrune-pickup smoke confirmed: mindrune in inventory,
ground tile clears, no "no handler for OBJ_COUNT"/"OBJ_TAKEITEM" logs.

NAI-152 B2 PRIMARY (7376779) closed handler-crash + reach; NAI-153 T1
(performInvAdd extract) + T2 (ActiveObj surface + Obj.IsValidFor) +
T3 (OBJ_COUNT) + T4 (OBJ_TAKEITEM) finished the cascade. Pickup chain
fully wired through invAdd + RemoveObj.

Deferred:
- RESPAWN-lifecycle respawn-after-delay (NAI-115-D2 foundation gap;
  shared with OBJ_DEL)
- WealthEvent ledger (NAI-153-D1; content can emit via OpWealthEvent)

Closes memory: nai_followups.md NAI-152-B2-FOLLOWUP-NAI-153 (retire).

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

Then retire the NAI-152-B2-FOLLOWUP-NAI-153 entry from `~/.claude/projects/-home-owner-Code-github-com-zsrv-goscape/memory/nai_followups.md` and append a new "From NAI-153 (DATE)" section recording any cascade-tail surprises (or "no carry-forwards" if smoke was clean).
