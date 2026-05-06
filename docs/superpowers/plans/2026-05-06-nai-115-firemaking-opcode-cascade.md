# NAI-115 Firemaking Opcode-Cascade Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Port the 7 firemaking-cascade opcodes (OBJ_COORD, OBJ_DEL, OBJ_ADD, LINEOFWALK, INV_DROPSLOT, OBJ_ADDALL, P_OPOBJ) to unblock Tutorial Island firemaking ✅ smoke and broadcast-ashes non-Tutorial flow.

**Architecture:** Two-bundle subagent-driven TDD. Bundle 1 (T1-T5) lands the 5 Tutorial-essential opcodes; user-launched smoke between bundles binds Tutorial Island firemaking. Bundle 2 (T6-T7) lands the 2 non-Tutorial-only opcodes. New file `pkg/script/handlers_obj.go` for the Obj-family ports; INV_DROPSLOT lives in `pkg/script/handlers_inv.go`; LINEOFWALK in `pkg/script/handlers_map.go`; P_OPOBJ in `pkg/script/handlers_player.go`. P_OPOBJ adds a new `SetInteractionScriptObj` method to the `ActivePlayer` interface (mirroring `SetInteractionScriptLoc`/`SetInteractionScriptNpc`).

**Tech Stack:** Go 1.26+. Test runner: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test`.

---

## File Structure

| File | Status | Responsibility |
|------|--------|----------------|
| `pkg/script/handlers_obj.go` | **create** | OBJ_COORD, OBJ_DEL, OBJ_ADD, OBJ_ADDALL handlers + shared `objAddCommon` helper |
| `pkg/script/handlers_obj_test.go` | **create** | Unit tests for the 4 Obj-family handlers |
| `pkg/script/handlers_inv.go` | modify | append `handleInvDropSlot` |
| `pkg/script/handlers_inv_test.go` | modify | append `handleInvDropSlot` tests |
| `pkg/script/handlers_map.go` | modify | append `handleLineOfWalk` |
| `pkg/script/handlers_map_test.go` | modify | append `handleLineOfWalk` tests |
| `pkg/script/handlers_player.go` | modify | append `handleP_OpObj` (mirror P_OPLOC at line 801) |
| `pkg/script/handlers_player_test.go` | modify | append `handleP_OpObj` tests |
| `pkg/script/handlers.go` | modify | register 7 new opcodes in dispatch map |
| `pkg/script/active.go` | modify | append `SetInteractionScriptObj(obj ActiveObj, op int)` to `ActivePlayer` interface |
| `modules/world/player_script.go` | modify | implement `(p *Player) SetInteractionScriptObj` (mirror SetInteractionScriptLoc at line 921) |
| `modules/world/world_zone.go` | possibly modify | add `RemoveObjWithDuration` if respawn-aware delete is needed (see T2 D2 deviation — Bundle 1 defers; goscape `Server.RemoveObj` ignores duration for the smoke path) |

---

# Bundle 1 — Tutorial-essential opcodes (T1-T5)

## Task 1: OBJ_COORD (opcode 3502)

**TS reference:** `Engine-TS/src/engine/script/handlers/ObjOps.ts:163-166`

**Files:**
- Create: `pkg/script/handlers_obj.go`
- Create: `pkg/script/handlers_obj_test.go`
- Modify: `pkg/script/handlers.go` (register OpObjCoord)

- [ ] **Step 1: Write failing test**

Create `pkg/script/handlers_obj_test.go` with:

```go
package script

import (
	"testing"
)

// stubActiveObj is a minimal ActiveObj for handler unit tests.
type stubActiveObj struct {
	x, z, level, typeID int
}

func (s *stubActiveObj) ObjType() int              { return s.typeID }
func (s *stubActiveObj) Coords() (x, z, level int) { return s.x, s.z, s.level }

func TestHandleObjCoord(t *testing.T) {
	s := newTestState()
	s.ActiveObj = &stubActiveObj{x: 3200, z: 3200, level: 0, typeID: 590}

	if err := handleObjCoord(s); err != nil {
		t.Fatalf("handleObjCoord returned error: %v", err)
	}
	got := s.PopInt()
	want := (0 << 28) | (3200 << 14) | 3200
	if got != want {
		t.Errorf("OBJ_COORD: got %d, want %d (packed level=0 x=3200 z=3200)", got, want)
	}
}

func TestHandleObjCoordNilActive(t *testing.T) {
	s := newTestState()
	// ActiveObj nil — handler must error rather than panic.
	if err := handleObjCoord(s); err == nil {
		t.Errorf("OBJ_COORD: expected error on nil ActiveObj, got nil")
	}
}
```

If `newTestState` doesn't exist, search `pkg/script/*_test.go` for the existing test-state factory (e.g., `newScriptState`, `mkState`) and use that. Run `grep -n "func newTestState\|func newScriptState\|func mkState" pkg/script/*_test.go` to confirm.

- [ ] **Step 2: Verify test fails**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/ -run TestHandleObjCoord -v`
Expected: FAIL — `undefined: handleObjCoord`.

- [ ] **Step 3: Implement handler**

Create `pkg/script/handlers_obj.go`:

```go
// Package script — handlers for the Obj family of script opcodes.
package script

import (
	"fmt"

	"github.com/zsrv/goscape/pkg/coordgrid"
)

// handleObjCoord (OBJ_COORD, opcode 3502) packs the active obj's tile
// position into a single RS2 coord int and pushes it. Mirrors TS
// ObjOps.ts:163-166.
func handleObjCoord(s *ScriptState) error {
	if s.ActiveObj == nil {
		return fmt.Errorf("OBJ_COORD: no active obj")
	}
	x, z, level := s.ActiveObj.Coords()
	s.PushInt(coordgrid.PackCoord(level, x, z))
	return nil
}
```

Verify the `coordgrid.PackCoord` symbol — run `grep -n "func PackCoord" pkg/coordgrid/*.go`. If the function name is different (e.g., `Pack`), substitute. If the canonical pack expression is `(level<<28) | (x<<14) | z` inlined elsewhere, inline it here too.

- [ ] **Step 4: Register opcode**

Modify `pkg/script/handlers.go` — locate the dispatch map (starts at line 13) and append a new section after the existing Obj-related comments. Search for the section near `OpObjAdd:` — it doesn't exist yet, so add a new block. Find the `// LOC active-loc reads + mutations.` block and add after it (or in alphanumeric-opcode order; the file mostly groups by sub-spec).

Insertion (place near other reads, e.g., after the existing OpUID line ~124):

```go
	// NAI-115 Bundle 1: firemaking-cascade Obj/Inv/Server/Player ports.
	OpObjCoord: handleObjCoord,
```

- [ ] **Step 5: Verify test passes**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/ -run TestHandleObjCoord -v`
Expected: PASS (both subtests).

- [ ] **Step 6: Commit**

```bash
git add pkg/script/handlers_obj.go pkg/script/handlers_obj_test.go pkg/script/handlers.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(script): NAI-115 T1 — OBJ_COORD handler (opcode 3502)

Packs ActiveObj coord into RS2 int. Mirrors TS ObjOps.ts:163-166.
First task of NAI-115 Bundle 1 (Tutorial Island firemaking cascade).

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 2: OBJ_DEL (opcode 3504)

**TS reference:** `Engine-TS/src/engine/script/handlers/ObjOps.ts:112-119`

**Deviation D2 (defensive note):** TS calls `World.removeObj(obj, duration)` where duration = ObjType.respawnrate. goscape `Server.RemoveObj(obj)` does not accept a duration — RESPAWN-lifecycle obj respawn-after-delay is a foundation gap. For DESPAWN-lifecycle objs (e.g., the firemaking log being lit), duration is irrelevant; the obj is permanently removed. T2 ports the call without duration; the deviation is doc-commented and the foundation gap remains for a later sub-spec.

**Files:**
- Modify: `pkg/script/handlers_obj.go`
- Modify: `pkg/script/handlers_obj_test.go`
- Modify: `pkg/script/handlers.go`

- [ ] **Step 1: Add WorldVars surface for ObjDel**

Open `pkg/script/state.go` and locate the `WorldVars` interface (line 53). Append a new method:

```go
	// RemoveObj despawns / removes the given obj from its zone. Mirrors
	// TS World.removeObj. Goscape's Server.RemoveObj does not accept a
	// duration; respawn-aware delete (RESPAWN-lifecycle objs) is a
	// foundation gap (NAI-115-D2). Used by OBJ_DEL.
	RemoveObj(obj ActiveObj)
```

`ActiveObj` is the existing interface at `pkg/script/active.go:733`. The handler will type-assert to the concrete `*entity.Obj` inside the world-side adapter — so the script-side surface only carries `ActiveObj`.

Open `modules/world/script.go` (the `WorldVarsAdapter` or equivalent — find via `grep -n "AnimMap\|IsMapBlocked" modules/world/*.go | head` for an existing analog). Add a `RemoveObj` method on the same receiver:

```go
// RemoveObj implements script.WorldVars.RemoveObj. Mirrors TS
// World.removeObj. NAI-115-D2: ignores duration (RESPAWN-lifecycle
// respawn timing is a foundation gap; DESPAWN-lifecycle path — the
// firemaking smoke target — is unaffected).
func (w *worldAdapter) RemoveObj(obj script.ActiveObj) {
	realObj, ok := obj.(*entitypkg.Obj)
	if !ok {
		return
	}
	w.server.RemoveObj(realObj)
}
```

Substitute `worldAdapter` / `entitypkg` with whatever names the file uses — `grep -n "AnimMap" modules/world/*.go` should locate the right struct and import alias.

- [ ] **Step 2: Write failing test**

Append to `pkg/script/handlers_obj_test.go`:

```go
// fakeWorldRemoveObj is a minimal WorldVars stub recording the obj
// passed to RemoveObj.
type fakeWorldRemoveObj struct {
	stubWorld
	removed []ActiveObj
}

func (f *fakeWorldRemoveObj) RemoveObj(obj ActiveObj) {
	f.removed = append(f.removed, obj)
}

func TestHandleObjDel(t *testing.T) {
	s := newTestState()
	w := &fakeWorldRemoveObj{}
	s.World = w
	active := &stubActiveObj{x: 3200, z: 3200, level: 0, typeID: 590}
	s.ActiveObj = active

	if err := handleObjDel(s); err != nil {
		t.Fatalf("handleObjDel returned error: %v", err)
	}
	if len(w.removed) != 1 || w.removed[0] != active {
		t.Errorf("OBJ_DEL: expected 1 RemoveObj call with active, got %v", w.removed)
	}
}

func TestHandleObjDelNilActive(t *testing.T) {
	s := newTestState()
	s.World = &fakeWorldRemoveObj{}
	if err := handleObjDel(s); err == nil {
		t.Errorf("OBJ_DEL: expected error on nil ActiveObj, got nil")
	}
}
```

`stubWorld` is the existing test-helper for `WorldVars` — search via `grep -n "type stubWorld\|type fakeWorld" pkg/script/*_test.go`. If a different name exists (`mockWorld`, `worldStub`), use that. If no fake-world exists, define a minimal one inside this test file with no-op methods for every WorldVars method except `RemoveObj`.

- [ ] **Step 3: Verify test fails**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/ -run TestHandleObjDel -v`
Expected: FAIL — `undefined: handleObjDel`.

- [ ] **Step 4: Implement handler**

Append to `pkg/script/handlers_obj.go`:

```go
// handleObjDel (OBJ_DEL, opcode 3504) removes the active obj. Mirrors
// TS ObjOps.ts:112-119.
//
// NAI-115-D2 deviation: TS reads ObjType.respawnrate and passes it to
// World.removeObj as duration. goscape's Server.RemoveObj does not
// accept a duration; RESPAWN-lifecycle respawn-after-delay is a
// foundation gap. DESPAWN-lifecycle objs (the firemaking smoke target)
// are unaffected.
func handleObjDel(s *ScriptState) error {
	if s.ActiveObj == nil {
		return fmt.Errorf("OBJ_DEL: no active obj")
	}
	if s.World == nil {
		return fmt.Errorf("OBJ_DEL: no world surface")
	}
	s.World.RemoveObj(s.ActiveObj)
	return nil
}
```

- [ ] **Step 5: Register opcode**

In `pkg/script/handlers.go`, append to the NAI-115 block:

```go
	OpObjDel:   handleObjDel,
```

- [ ] **Step 6: Verify test passes**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/ -run "TestHandleObjDel|TestHandleObjCoord" -v`
Expected: PASS.

- [ ] **Step 7: Verify world adapter compiles**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./...`
Expected: clean build. If the world adapter is missing a method, the compiler will name it.

- [ ] **Step 8: Commit**

```bash
git add pkg/script/handlers_obj.go pkg/script/handlers_obj_test.go pkg/script/handlers.go pkg/script/state.go modules/world/script.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(script,world): NAI-115 T2 — OBJ_DEL handler (opcode 3504)

Removes the active obj via Server.RemoveObj. Mirrors TS ObjOps.ts:112-119.
NAI-115-D2 deviation: TS passes respawn-rate as duration; goscape's
RemoveObj has no duration arg (foundation gap for RESPAWN-lifecycle
respawn timing). DESPAWN path unaffected.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 3: OBJ_ADD (opcode 3500)

**TS reference:** `Engine-TS/src/engine/script/handlers/ObjOps.ts:20-55`

**Files:**
- Modify: `pkg/script/handlers_obj.go`
- Modify: `pkg/script/handlers_obj_test.go`
- Modify: `pkg/script/state.go` — add `AddObj` to `WorldVars`
- Modify: `modules/world/script.go` (or wherever `worldAdapter` lives) — implement `AddObj`
- Modify: `pkg/script/handlers.go`

- [ ] **Step 1: Extend WorldVars surface**

Open `pkg/script/state.go` and append to the `WorldVars` interface:

```go
	// AddObj routes a ground-item spawn. receiverID is the owning player's
	// UID for caller-only drops, or zone.PublicReceiver for broadcast.
	// Used by OBJ_ADD (per-caller), OBJ_ADDALL (broadcast), and
	// INV_DROPSLOT.
	AddObj(level, x, z, typeID, count, duration, receiverID int)
```

The handler ports stay engine-agnostic; the world adapter constructs the entity, picks the lifecycle (`LifecycleDespawn`), and calls `s.server.AddObj`. Implement in `modules/world/script.go`:

```go
// AddObj implements script.WorldVars.AddObj. Constructs a DESPAWN-lifecycle
// Obj and routes via Server.AddObj. Mirrors TS World.addObj for
// despawnable drops. NAI-115-D2: duration is accepted but currently
// ignored (foundation gap for despawn-timer arming).
func (w *worldAdapter) AddObj(level, x, z, typeID, count, duration, receiverID int) {
	obj := entitypkg.NewObj(level, x, z, entitypkg.LifecycleDespawn, typeID, count)
	obj.ReceiverID = receiverID
	w.server.AddObj(obj, receiverID)
	_ = duration // foundation gap; see NAI-115-D2 in handlers_obj.go
}
```

The despawn timer (TS schedules a delete after `duration` ticks) is a foundation gap — subsumed under D2. The smoke binds because Tutorial firemaking only checks that the obj appears at all (visible loot drop).

- [ ] **Step 2: Add value validators if missing**

Search for existing ObjType validator helpers: `grep -n "func checkObjType\|ObjTypeValid" pkg/script/*.go`. If `checkObjType` exists, reuse it. Otherwise add to `pkg/script/handlers_obj.go`:

```go
// checkObjType validates an ObjType id. Returns an error if the id is
// negative or unknown to Configs. Mirrors TS check(id, ObjTypeValid).
func checkObjType(s *ScriptState, id int, op string) error {
	if id < 0 {
		return fmt.Errorf("%s: invalid obj id (%d)", op, id)
	}
	if s.Configs == nil || s.Configs.ObjType(id) == nil {
		return fmt.Errorf("%s: invalid obj id (%d)", op, id)
	}
	return nil
}

// checkDuration validates a despawn-timer duration arg. NumberPositive
// in TS (DurationValid). Negative durations error.
func checkDuration(d int, op string) error {
	if d < 0 {
		return fmt.Errorf("%s: invalid duration (%d)", op, d)
	}
	return nil
}

// checkObjStack validates a stack count. NumberPositive in TS.
func checkObjStack(c int, op string) error {
	if c < 1 {
		return fmt.Errorf("%s: invalid count (%d)", op, c)
	}
	return nil
}
```

If `checkCoord` exists in `pkg/script/handlers_map.go`, reuse it directly. (It's referenced in handleMapPlayerCount.)

- [ ] **Step 3: Write failing test**

Append to `pkg/script/handlers_obj_test.go`:

```go
type fakeWorldAddObj struct {
	stubWorld
	addedCalls []addObjCall
	mapMembers int
}

type addObjCall struct {
	level, x, z, typeID, count, duration, receiverID int
}

func (f *fakeWorldAddObj) AddObj(level, x, z, typeID, count, duration, receiverID int) {
	f.addedCalls = append(f.addedCalls, addObjCall{level, x, z, typeID, count, duration, receiverID})
}

func (f *fakeWorldAddObj) MapMembers() int { return f.mapMembers }

func TestHandleObjAddStackable(t *testing.T) {
	s := newTestState()
	w := &fakeWorldAddObj{mapMembers: 1}
	s.World = w
	s.Configs = newTestConfigsWithObj(590, /*stackable*/ true, /*members*/ false, /*dummyitem*/ 0)
	s.Self = &stubPlayer{uid: 12345}

	// Push order: bottom-up matches TS popInts(4) destructuring [coord, objId, count, duration].
	s.PushInt(packCoord(0, 3200, 3200)) // coord
	s.PushInt(590)                      // objId
	s.PushInt(5)                        // count
	s.PushInt(100)                      // duration

	if err := handleObjAdd(s); err != nil {
		t.Fatalf("handleObjAdd returned error: %v", err)
	}
	if len(w.addedCalls) != 1 {
		t.Fatalf("OBJ_ADD stackable count=5: expected 1 AddObj call, got %d", len(w.addedCalls))
	}
	got := w.addedCalls[0]
	want := addObjCall{level: 0, x: 3200, z: 3200, typeID: 590, count: 5, duration: 100, receiverID: 12345}
	if got != want {
		t.Errorf("OBJ_ADD stackable: got %+v, want %+v", got, want)
	}
}

func TestHandleObjAddNonStackable(t *testing.T) {
	s := newTestState()
	w := &fakeWorldAddObj{mapMembers: 1}
	s.World = w
	s.Configs = newTestConfigsWithObj(590, /*stackable*/ false, /*members*/ false, /*dummyitem*/ 0)
	s.Self = &stubPlayer{uid: 12345}

	s.PushInt(packCoord(0, 3200, 3200))
	s.PushInt(590)
	s.PushInt(3)
	s.PushInt(100)

	if err := handleObjAdd(s); err != nil {
		t.Fatalf("handleObjAdd returned error: %v", err)
	}
	// Non-stackable count=3 must produce 3 separate AddObj calls each with count=1.
	if len(w.addedCalls) != 3 {
		t.Fatalf("OBJ_ADD non-stackable count=3: expected 3 AddObj calls, got %d", len(w.addedCalls))
	}
	for i, c := range w.addedCalls {
		if c.count != 1 {
			t.Errorf("OBJ_ADD non-stackable call[%d]: count=%d, want 1", i, c.count)
		}
	}
}

func TestHandleObjAddNegativeIdShortCircuits(t *testing.T) {
	s := newTestState()
	w := &fakeWorldAddObj{mapMembers: 1}
	s.World = w
	s.Configs = newTestConfigsWithObj(590, true, false, 0)
	s.Self = &stubPlayer{uid: 12345}
	s.PushInt(packCoord(0, 3200, 3200))
	s.PushInt(-1) // objId == -1 → short-circuit return
	s.PushInt(5)
	s.PushInt(100)

	if err := handleObjAdd(s); err != nil {
		t.Fatalf("handleObjAdd returned error: %v", err)
	}
	if len(w.addedCalls) != 0 {
		t.Errorf("OBJ_ADD with objId=-1: expected 0 AddObj calls, got %d", len(w.addedCalls))
	}
}

func TestHandleObjAddDummyItemErrors(t *testing.T) {
	s := newTestState()
	w := &fakeWorldAddObj{mapMembers: 1}
	s.World = w
	s.Configs = newTestConfigsWithObj(590, true, false, /*dummyitem*/ 1)
	s.Self = &stubPlayer{uid: 12345}
	s.PushInt(packCoord(0, 3200, 3200))
	s.PushInt(590)
	s.PushInt(1)
	s.PushInt(100)
	if err := handleObjAdd(s); err == nil {
		t.Errorf("OBJ_ADD: expected dummyitem error, got nil")
	}
}

func TestHandleObjAddMembersGate(t *testing.T) {
	s := newTestState()
	w := &fakeWorldAddObj{mapMembers: 0} // F2P world
	s.World = w
	s.Configs = newTestConfigsWithObj(590, true, /*members*/ true, 0)
	s.Self = &stubPlayer{uid: 12345}
	s.PushInt(packCoord(0, 3200, 3200))
	s.PushInt(590)
	s.PushInt(1)
	s.PushInt(100)
	if err := handleObjAdd(s); err != nil {
		t.Fatalf("handleObjAdd returned error: %v", err)
	}
	if len(w.addedCalls) != 0 {
		t.Errorf("OBJ_ADD members-gated F2P: expected 0 AddObj calls, got %d", len(w.addedCalls))
	}
}
```

`newTestConfigsWithObj`, `stubPlayer{uid:int}`, and `packCoord` are test-helpers. Search before authoring:
- `grep -n "func newTestConfigs\|newTestConfigs " pkg/script/*_test.go`
- `grep -n "type stubPlayer\|stubPlayer struct" pkg/script/*_test.go`
- `grep -n "func packCoord\b" pkg/script/*_test.go pkg/coordgrid/*.go`

If `newTestConfigsWithObj` does not exist, define a thin wrapper at the top of `handlers_obj_test.go`:

```go
func newTestConfigsWithObj(id int, stackable, members bool, dummyitem int) Configs {
	// Reuse existing stubConfigs (rg "type stubConfigs" pkg/script/*_test.go).
	// Pattern match on whatever ObjType-attaching helper already exists.
}
```

If `stubPlayer{uid: int}` does not exist on the existing stubPlayer, add a `uid` field and a `UID() int` method (used by ActivePlayer for the receiverID derivation in T3 Step 4 below). Search via `grep -n "func.*stubPlayer.*UID\|UID() int" pkg/script/*_test.go`. Also confirm the production ActivePlayer interface exposes a `UID()` method via `grep -n "UID() int" pkg/script/active.go`.

If `ActivePlayer` does not have `UID()` exposed, add it to `pkg/script/active.go`:

```go
	// UID returns the player's UID (composeUID per NAI-113). Used as the
	// receiverID for caller-only obj drops (OBJ_ADD, INV_DROPSLOT).
	UID() int
```

And implement on `modules/world/player_script.go`:

```go
// UID implements script.ActivePlayer.UID. Returns the player's runtime
// UID composed at login (NAI-113 composeUID).
func (p *Player) UID() int { return p.uid }
```

- [ ] **Step 4: Verify test fails**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/ -run TestHandleObjAdd -v`
Expected: FAIL — `undefined: handleObjAdd`.

- [ ] **Step 5: Implement handler**

Append to `pkg/script/handlers_obj.go`:

```go
// objAddCommon is the shared body of OBJ_ADD and OBJ_ADDALL. Differs
// only in receiverID: OBJ_ADD passes the active player's UID for a
// caller-only private drop; OBJ_ADDALL passes zone.PublicReceiver for
// a broadcast drop.
//
// Mirrors TS ObjOps.ts:20-92 (both opcodes share the validation chain
// + stackable branch). Pop order matches popInts(4): top-of-stack is
// duration, then count, then objId, then coord at the bottom.
func objAddCommon(s *ScriptState, op string, receiverID int) error {
	duration := s.PopInt()
	count := s.PopInt()
	objId := s.PopInt()
	coord := s.PopInt()

	if objId == -1 || count == -1 {
		return nil
	}
	if err := checkObjType(s, objId, op); err != nil {
		return err
	}
	if err := checkDuration(duration, op); err != nil {
		return err
	}
	level, x, z, err := checkCoord(coord, op)
	if err != nil {
		return err
	}
	if err := checkObjStack(count, op); err != nil {
		return err
	}

	objType := s.Configs.ObjType(objId)
	if objType.DummyItem != 0 {
		return fmt.Errorf("%s: attempted to add dummy item: id=%d", op, objId)
	}
	if objType.Members && s.World.MapMembers() == 0 {
		return nil
	}

	if !objType.Stackable || count == 1 {
		for range count {
			s.World.AddObj(level, x, z, objId, 1, duration, receiverID)
		}
	} else {
		s.World.AddObj(level, x, z, objId, count, duration, receiverID)
	}
	// pointerAdd / state.activeObj is set by the world adapter on the LAST
	// added obj — TS sets it once per Add call. goscape currently has no
	// world→state ActiveObj writeback path; the smoke binds without it
	// because the script flow does not read OBJ_COORD/OBJ_DEL on the
	// drop's active-pointer in the same handler call. Tracked as deviation
	// NAI-115-D3 (cross-ref in Risk §6 of the spec).
	return nil
}

// handleObjAdd (OBJ_ADD, opcode 3500) drops a private (caller-only) obj
// at the unpacked coord. Mirrors TS ObjOps.ts:20-55.
func handleObjAdd(s *ScriptState) error {
	if s.Self == nil {
		return fmt.Errorf("OBJ_ADD: no active player")
	}
	return objAddCommon(s, "OBJ_ADD", s.Self.UID())
}
```

If `s.Configs.ObjType(id)` returns a different concrete type with different field names than `DummyItem` / `Members` / `Stackable`, fix the access expressions to match. The grep result above (`pkg/objtype/objtype.go`) confirms these names match.

- [ ] **Step 6: Register opcode**

In `pkg/script/handlers.go`, append to the NAI-115 block:

```go
	OpObjAdd:   handleObjAdd,
```

- [ ] **Step 7: Verify tests pass**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/ -run "TestHandleObjAdd|TestHandleObjCoord|TestHandleObjDel" -v`
Expected: PASS (all 5 obj subtests).

Then full suite for guardrail:
Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/...`
Expected: PASS.

- [ ] **Step 8: Commit**

```bash
git add pkg/script/handlers_obj.go pkg/script/handlers_obj_test.go pkg/script/handlers.go pkg/script/state.go pkg/script/active.go modules/world/script.go modules/world/player_script.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(script,world): NAI-115 T3 — OBJ_ADD handler (opcode 3500)

Drops a caller-only obj at the unpacked coord. Pop order
[coord, objId, count, duration] mirrors TS popInts(4). Stackable branch
splits non-stackable drops into N separate count=1 AddObj calls.
Adds Player.UID() accessor for receiverID derivation. Adds
WorldVars.AddObj surface; world adapter routes via Server.AddObj.

NAI-115-D3 deviation: TS sets state.activeObj after each AddObj for
pointerAdd; goscape has no world→state writeback path yet — smoke
binds because the script does not consume the obj-pointer in the
same handler invocation.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 4: LINEOFWALK (opcode 1006)

**TS reference:** `Engine-TS/src/engine/script/handlers/ServerOps.ts:65-82`

The script-side `LineValidator` interface already exists at `pkg/script/state.go:28-31` with the right method shape. Production wiring goes through `s.LineValidator` (set by `modules/world/script.go`).

**Files:**
- Modify: `pkg/script/handlers_map.go`
- Modify: `pkg/script/handlers_map_test.go`
- Modify: `pkg/script/handlers.go`

- [ ] **Step 1: Write failing test**

Append to `pkg/script/handlers_map_test.go`:

```go
// fakeLineValidator records calls and returns configured booleans.
type fakeLineValidator struct {
	hasLineOfWalkResult  bool
	hasLineOfWalkCalls   int
	lastWalkArgs         [9]int
}

func (f *fakeLineValidator) HasLineOfWalk(level, srcX, srcZ, destX, destZ, srcSize, destWidth, destLength, extraFlag int) bool {
	f.hasLineOfWalkCalls++
	f.lastWalkArgs = [9]int{level, srcX, srcZ, destX, destZ, srcSize, destWidth, destLength, extraFlag}
	return f.hasLineOfWalkResult
}

func (f *fakeLineValidator) HasLineOfSight(level, srcX, srcZ, destX, destZ, srcSize, destWidth, destLength, extraFlag int) bool {
	return false
}

func TestHandleLineOfWalkSameLevelTrue(t *testing.T) {
	s := newTestState()
	w := &fakeWorldAddObj{mapMembers: 1} // members world; F2P gate inert
	s.World = w
	s.LineValidator = &fakeLineValidator{hasLineOfWalkResult: true}

	s.PushInt(packCoord(0, 3200, 3200)) // c1 (from)
	s.PushInt(packCoord(0, 3201, 3200)) // c2 (to)

	if err := handleLineOfWalk(s); err != nil {
		t.Fatalf("handleLineOfWalk returned error: %v", err)
	}
	if got := s.PopInt(); got != 1 {
		t.Errorf("LINEOFWALK same-level true: got %d, want 1", got)
	}
}

func TestHandleLineOfWalkDifferentLevels(t *testing.T) {
	s := newTestState()
	w := &fakeWorldAddObj{mapMembers: 1}
	s.World = w
	s.LineValidator = &fakeLineValidator{hasLineOfWalkResult: true}

	s.PushInt(packCoord(0, 3200, 3200))
	s.PushInt(packCoord(1, 3200, 3200)) // different level

	if err := handleLineOfWalk(s); err != nil {
		t.Fatalf("handleLineOfWalk returned error: %v", err)
	}
	if got := s.PopInt(); got != 0 {
		t.Errorf("LINEOFWALK different-level: got %d, want 0", got)
	}
}

func TestHandleLineOfWalkF2PShortCircuit(t *testing.T) {
	s := newTestState()
	w := &fakeWorldAddObj{mapMembers: 0} // F2P world
	s.World = w
	s.LineValidator = &fakeLineValidator{hasLineOfWalkResult: true}

	s.PushInt(packCoord(0, 3200, 3200))
	s.PushInt(packCoord(0, 3201, 3200))

	if err := handleLineOfWalk(s); err != nil {
		t.Fatalf("handleLineOfWalk returned error: %v", err)
	}
	if got := s.PopInt(); got != 0 {
		t.Errorf("LINEOFWALK F2P-blocked: got %d, want 0", got)
	}
}

func TestHandleLineOfWalkNilValidator(t *testing.T) {
	s := newTestState()
	w := &fakeWorldAddObj{mapMembers: 1}
	s.World = w
	s.LineValidator = nil // no validator wired
	s.PushInt(packCoord(0, 3200, 3200))
	s.PushInt(packCoord(0, 3201, 3200))
	if err := handleLineOfWalk(s); err != nil {
		t.Fatalf("handleLineOfWalk returned error: %v", err)
	}
	// No validator → handler must not panic. TS pessimistically pushes 1
	// when the underlying isLineOfWalk has no implementation; goscape's
	// existing pattern (see handleMapFindSquare) treats nil-validator as
	// "fail closed" → push 0.
	if got := s.PopInt(); got != 0 {
		t.Errorf("LINEOFWALK nil validator: got %d, want 0", got)
	}
}
```

If `fakeWorldAddObj` from T3 has not landed yet (e.g., parallel review state), define a minimal stub here. Otherwise reuse.

If `IsFreeToPlay` requires a tile-by-tile fake, add to `fakeWorldAddObj`:

```go
func (f *fakeWorldAddObj) IsFreeToPlay(x, z int) bool { return false }
```

- [ ] **Step 2: Verify test fails**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/ -run TestHandleLineOfWalk -v`
Expected: FAIL — `undefined: handleLineOfWalk`.

- [ ] **Step 3: Implement handler**

Append to `pkg/script/handlers_map.go`:

```go
// handleLineOfWalk (LINEOFWALK, opcode 1006) reports whether a player at
// c1 has line-of-walk to c2 on the same level. Pop order: top-of-stack
// is c2, c1 below. Pushes 1 on success, 0 on fail. F2P short-circuit
// when the destination tile is not in an F2P zone in a non-members
// world. Mirrors TS ServerOps.ts:65-82.
func handleLineOfWalk(s *ScriptState) error {
	c2 := s.PopInt()
	c1 := s.PopInt()

	fromLevel, fromX, fromZ, err := checkCoord(c1, "LINEOFWALK")
	if err != nil {
		return err
	}
	toLevel, toX, toZ, err := checkCoord(c2, "LINEOFWALK")
	if err != nil {
		return err
	}
	if fromLevel != toLevel {
		s.PushInt(0)
		return nil
	}
	if s.World.MapMembers() == 0 && !s.World.IsFreeToPlay(toX, toZ) {
		s.PushInt(0)
		return nil
	}
	if s.LineValidator == nil {
		s.PushInt(0)
		return nil
	}
	if s.LineValidator.HasLineOfWalk(fromLevel, fromX, fromZ, toX, toZ, 1, 0, 0, 0) {
		s.PushInt(1)
	} else {
		s.PushInt(0)
	}
	return nil
}
```

- [ ] **Step 4: Register opcode**

In `pkg/script/handlers.go`, append to the NAI-115 block:

```go
	OpLineOfWalk: handleLineOfWalk,
```

- [ ] **Step 5: Verify tests pass**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/ -run TestHandleLineOfWalk -v`
Expected: PASS (all 4 subtests).

- [ ] **Step 6: Commit**

```bash
git add pkg/script/handlers_map.go pkg/script/handlers_map_test.go pkg/script/handlers.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(script): NAI-115 T4 — LINEOFWALK handler (opcode 1006)

Pops 2 coords, level-equality short-circuit, F2P gate, validator
passthrough. Mirrors TS ServerOps.ts:65-82. Nil-LineValidator path
fails closed (push 0).

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 5: INV_DROPSLOT (opcode 4312)

**TS reference:** `Engine-TS/src/engine/script/handlers/InvOps.ts:213-260`

**Deviation D1 (wealth-event skip):** TS inlines `addWealthEvent` for SCOPE_PERM drops. goscape has separate `OpWealthEvent (2131)` already declared. Skip inline emission with a tracked deviation comment.

**Files:**
- Modify: `pkg/script/handlers_inv.go`
- Modify: `pkg/script/handlers_inv_test.go`
- Modify: `pkg/script/handlers.go`

- [ ] **Step 1: Write failing test**

Append to `pkg/script/handlers_inv_test.go`:

```go
func TestHandleInvDropSlotHappyPath(t *testing.T) {
	s := newTestState()
	w := &fakeWorldAddObj{mapMembers: 1}
	s.World = w
	s.Self = &stubPlayer{uid: 12345}
	s.Pointers = PtrActivePlayer | PtrProtectedActivePlayer

	// Set up an inv with a log at slot 2.
	inv := newTestInventory(28 /*capacity*/, /*stackable*/ false)
	inv.Set(2, &inventory.Item{Id: 1511 /*newbielogs*/, Count: 1})
	s.Inv = newTestInvLookup(map[int]*inventory.Inventory{93 /*inv_id*/: inv})
	s.Configs = newTestConfigsWithInv(93 /*invId*/, /*scope*/ objtype.InvTypeScopeTemp, /*protect*/ true).
		withObj(1511, /*stackable*/ false, /*members*/ false, 0)

	// Push order [inv, coord, slot, duration].
	s.PushInt(93)
	s.PushInt(packCoord(0, 3200, 3200))
	s.PushInt(2)
	s.PushInt(100)

	if err := handleInvDropSlot(s); err != nil {
		t.Fatalf("handleInvDropSlot returned error: %v", err)
	}
	if len(w.addedCalls) != 1 {
		t.Fatalf("expected 1 AddObj call, got %d", len(w.addedCalls))
	}
	if got := w.addedCalls[0].typeID; got != 1511 {
		t.Errorf("AddObj typeID: got %d, want 1511", got)
	}
	if got := w.addedCalls[0].receiverID; got != 12345 {
		t.Errorf("AddObj receiverID: got %d, want 12345 (player UID)", got)
	}
	if it := inv.Get(2); it != nil {
		t.Errorf("expected slot 2 cleared, got %+v", it)
	}
}

func TestHandleInvDropSlotEmptySlotErrors(t *testing.T) {
	s := newTestState()
	s.World = &fakeWorldAddObj{mapMembers: 1}
	s.Self = &stubPlayer{uid: 12345}
	s.Pointers = PtrActivePlayer | PtrProtectedActivePlayer

	inv := newTestInventory(28, false)
	s.Inv = newTestInvLookup(map[int]*inventory.Inventory{93: inv})
	s.Configs = newTestConfigsWithInv(93, objtype.InvTypeScopeTemp, true)

	s.PushInt(93)
	s.PushInt(packCoord(0, 3200, 3200))
	s.PushInt(2) // empty slot
	s.PushInt(100)

	if err := handleInvDropSlot(s); err == nil {
		t.Errorf("INV_DROPSLOT empty slot: expected error, got nil")
	}
}

func TestHandleInvDropSlotProtectedRequired(t *testing.T) {
	s := newTestState()
	s.World = &fakeWorldAddObj{mapMembers: 1}
	s.Self = &stubPlayer{uid: 12345}
	s.Pointers = PtrActivePlayer // NO PtrProtectedActivePlayer

	inv := newTestInventory(28, false)
	inv.Set(2, &inventory.Item{Id: 1511, Count: 1})
	s.Inv = newTestInvLookup(map[int]*inventory.Inventory{93: inv})
	// Inv has protect=true, scope=Temp → must require ProtectedActivePlayer.
	s.Configs = newTestConfigsWithInv(93, objtype.InvTypeScopeTemp, /*protect*/ true)

	s.PushInt(93)
	s.PushInt(packCoord(0, 3200, 3200))
	s.PushInt(2)
	s.PushInt(100)

	if err := handleInvDropSlot(s); err == nil {
		t.Errorf("INV_DROPSLOT protect-required without pointer: expected error, got nil")
	}
}

func TestHandleInvDropSlotSharedScopeBypassesProtect(t *testing.T) {
	s := newTestState()
	w := &fakeWorldAddObj{mapMembers: 1}
	s.World = w
	s.Self = &stubPlayer{uid: 12345}
	s.Pointers = PtrActivePlayer // NO PtrProtectedActivePlayer

	inv := newTestInventory(28, false)
	inv.Set(2, &inventory.Item{Id: 1511, Count: 1})
	s.Inv = newTestInvLookup(map[int]*inventory.Inventory{93: inv})
	// Shared scope → protect gate is bypassed.
	s.Configs = newTestConfigsWithInv(93, objtype.InvTypeScopeShared, /*protect*/ true).
		withObj(1511, false, false, 0)

	s.PushInt(93)
	s.PushInt(packCoord(0, 3200, 3200))
	s.PushInt(2)
	s.PushInt(100)

	if err := handleInvDropSlot(s); err != nil {
		t.Fatalf("INV_DROPSLOT scope=Shared: expected success, got error %v", err)
	}
	if len(w.addedCalls) != 1 {
		t.Errorf("expected 1 AddObj call, got %d", len(w.addedCalls))
	}
}
```

Helper authoring guidance — search before defining new ones:
- `grep -n "func newTestInventory\|func newTestInvLookup\|func newTestConfigsWithInv" pkg/script/*_test.go pkg/inventory/*_test.go`
- The methods `withObj`, `withInv` are presumed-builder-style; if the existing fixture uses a different pattern (e.g., `newTestConfigs(map[int]any{})`), adapt.

If `requireProtectedActivePlayer` exists in `pkg/script/handlers_player.go` (it's referenced by handleP_OpLoc), reuse it for the protect gate — but note that gate is binary (Pointer-only). The TS gate also conditions on `invType.protect && invType.scope !== SCOPE_SHARED`. Implementation must combine both.

- [ ] **Step 2: Verify test fails**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/ -run TestHandleInvDropSlot -v`
Expected: FAIL — `undefined: handleInvDropSlot`.

- [ ] **Step 3: Implement handler**

Append to `pkg/script/handlers_inv.go`:

```go
// handleInvDropSlot (INV_DROPSLOT, opcode 4312) drops the entire stack
// at slot from inv onto the ground at coord, with private (caller-only)
// visibility for `duration` ticks. Mirrors TS InvOps.ts:213-260.
//
// Pop order: [inv, coord, slot, duration]. Top-of-stack is duration.
//
// Protect gate: when invType.Protect is true AND invType.Scope is not
// Shared, require PtrProtectedActivePlayer. Otherwise bypass.
//
// NAI-115-D1 deviation: TS inlines addWealthEvent for SCOPE_PERM drops.
// goscape has a separate OpWealthEvent (2131); inline emission is
// skipped here and content layer can call OpWealthEvent explicitly.
func handleInvDropSlot(s *ScriptState) error {
	duration := s.PopInt()
	slot := s.PopInt()
	coord := s.PopInt()
	invID := s.PopInt()

	if s.Self == nil {
		return fmt.Errorf("INV_DROPSLOT: no active player")
	}
	if s.World == nil {
		return fmt.Errorf("INV_DROPSLOT: no world surface")
	}
	if s.Configs == nil {
		return fmt.Errorf("INV_DROPSLOT: no configs")
	}

	invType := s.Configs.InvType(invID)
	if invType == nil {
		return fmt.Errorf("INV_DROPSLOT: invalid inv id (%d)", invID)
	}
	if err := checkDuration(duration, "INV_DROPSLOT"); err != nil {
		return err
	}
	level, x, z, err := checkCoord(coord, "INV_DROPSLOT")
	if err != nil {
		return err
	}

	// Protect gate (TS InvOps.ts:220-222).
	if invType.Protect && invType.Scope != objtype.InvTypeScopeShared {
		if s.Pointers&PtrProtectedActivePlayer == 0 {
			return fmt.Errorf("INV_DROPSLOT: $inv requires protected access (id=%d)", invID)
		}
	}

	inv := resolveInv(s, invID)
	if inv == nil {
		return fmt.Errorf("INV_DROPSLOT: inv unresolved (id=%d)", invID)
	}
	it := inv.Get(slot)
	if it == nil {
		return fmt.Errorf("INV_DROPSLOT: $slot is empty (slot=%d)", slot)
	}

	objID := it.Id
	count := it.Count
	objType := s.Configs.ObjType(objID)
	if objType == nil {
		return fmt.Errorf("INV_DROPSLOT: invalid obj id at slot (id=%d)", objID)
	}

	// NAI-115-D1: TS calls addWealthEvent here for SCOPE_PERM. Skipped.
	// (goscape defensive; content can emit via OpWealthEvent.)

	// Slot-scoped removal: clear the specific slot. Mirrors TS
	// player.invDel(invType.id, obj.id, obj.count, slot) where the slot
	// arg constrains deletion to that exact slot. completed = count.
	completed := count
	inv.Delete(slot)
	if completed == 0 {
		return nil
	}

	receiverID := s.Self.UID()
	if !objType.Stackable || completed == 1 {
		for range completed {
			s.World.AddObj(level, x, z, objID, 1, duration, receiverID)
		}
	} else {
		s.World.AddObj(level, x, z, objID, completed, duration, receiverID)
	}
	return nil
}
```

If `s.Configs.InvType(id)` does not exist, search `pkg/script/configs.go` (or wherever the Configs interface lives) and either add the method or use the existing accessor. Run:
- `grep -n "InvType(id\|InvType(" pkg/script/*.go pkg/objtype/*.go`

If `resolveInv` returns nil because `s.Inv` is wired but the lookup fails, the handler errors — that's correct. The smoke needs the world adapter to wire `s.Inv` before script execution; this is already done in production wiring (per existing INV_TOTAL/INV_GETOBJ tests).

- [ ] **Step 4: Register opcode**

In `pkg/script/handlers.go`, append to the NAI-115 block:

```go
	OpInvDropSlot: handleInvDropSlot,
```

- [ ] **Step 5: Verify tests pass**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/ -run TestHandleInvDropSlot -v`
Expected: PASS (all 4 subtests).

Then run full pkg/script suite + build to catch any cross-package fallout:
```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/...
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./...
```
Expected: PASS / clean build.

- [ ] **Step 6: Commit**

```bash
git add pkg/script/handlers_inv.go pkg/script/handlers_inv_test.go pkg/script/handlers.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(script): NAI-115 T5 — INV_DROPSLOT handler (opcode 4312)

Drops the entire stack at slot onto the ground at coord with private
caller-only visibility. Pop order [inv, coord, slot, duration]
mirrors TS popInts(4). Protect gate: invType.Protect && Scope !=
Shared requires PtrProtectedActivePlayer. Stackable branch matches
TS OBJ_ADD shape.

NAI-115-D1 deviation: TS inlines addWealthEvent for SCOPE_PERM
drops; goscape skips inline (content layer can call OpWealthEvent
2131 explicitly).

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Bundle 1 smoke checkpoint (USER-LAUNCHED)

After T5, dispatch a smoke handoff message (per `smoke_test_server_handoff`):

> Bundle 1 (T1-T5) is complete. Please launch the server and run Tutorial Island firemaking:
> 1. Start a fresh tutorial-island character.
> 2. Reach the Survival Expert area (post-NAI-114 dispatch fix should let OPHELDU fire).
> 3. Use tinderbox on logs from inv.
>
> **Pass:**
> - human_createfire animation plays.
> - Inv loses 1 newbielogs (slot cleared).
> - Ground tile gains a fire loc.
> - Stat panel shows +400 firemaking XP (capped at base ≥3 → no XP per `tutorial_give_xp` proc).
> - After ~150 ticks: fire despawns, ashes appear at coord.
> - Server log: ZERO `no handler for OPCODE` warnings on `[label,tut_light_logs_inv]`, `[proc,tut_firemaking_success]`, `[proc,push_player]`.
>
> Report symptom shifts. Per `smoke_surfaces_adjacent_divergences`, ≤30 LOC fixes for adjacent gaps land in-bundle; larger residuals route to NAI-116.

---

# Bundle 2 — non-Tutorial-only opcodes (T6-T7)

## Task 6: OBJ_ADDALL (opcode 3501)

**TS reference:** `Engine-TS/src/engine/script/handlers/ObjOps.ts:58-93`

**Files:**
- Modify: `pkg/script/handlers_obj.go`
- Modify: `pkg/script/handlers_obj_test.go`
- Modify: `pkg/script/handlers.go`

- [ ] **Step 1: Write failing test**

Append to `pkg/script/handlers_obj_test.go`:

```go
func TestHandleObjAddAllUsesPublicReceiver(t *testing.T) {
	s := newTestState()
	w := &fakeWorldAddObj{mapMembers: 1}
	s.World = w
	s.Configs = newTestConfigsWithObj(590, true, false, 0)
	s.Self = &stubPlayer{uid: 12345}

	s.PushInt(packCoord(0, 3200, 3200))
	s.PushInt(590)
	s.PushInt(1)
	s.PushInt(100)

	if err := handleObjAddAll(s); err != nil {
		t.Fatalf("handleObjAddAll returned error: %v", err)
	}
	if len(w.addedCalls) != 1 {
		t.Fatalf("expected 1 AddObj call, got %d", len(w.addedCalls))
	}
	got := w.addedCalls[0].receiverID
	want := publicReceiverIDForTest() // see helper below
	if got != want {
		t.Errorf("OBJ_ADDALL receiverID: got %d, want %d (public)", got, want)
	}
}

// publicReceiverIDForTest returns the value the world adapter uses to
// signal "broadcast" routing. In production this is zone.PublicReceiver
// (=-1 — confirm via grep).
func publicReceiverIDForTest() int { return -1 }
```

Verify the actual sentinel value: `grep -n "PublicReceiver\s*=\s*\|PublicReceiver = " pkg/zone/*.go` and substitute the real constant.

- [ ] **Step 2: Verify test fails**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/ -run TestHandleObjAddAll -v`
Expected: FAIL — `undefined: handleObjAddAll`.

- [ ] **Step 3: Implement handler**

Append to `pkg/script/handlers_obj.go`:

```go
// objAddAllReceiverID is the receiverID sentinel passed to
// WorldVars.AddObj for broadcast (visible-to-all) drops. The world
// adapter resolves this to zone.PublicReceiver. Kept package-local so
// pkg/script does not depend on pkg/zone directly.
const objAddAllReceiverID = -1

// handleObjAddAll (OBJ_ADDALL, opcode 3501) drops a broadcast
// (visible-to-all) obj at the unpacked coord. Twin of handleObjAdd;
// passes objAddAllReceiverID as the receiver. Mirrors TS
// ObjOps.ts:58-93.
func handleObjAddAll(s *ScriptState) error {
	return objAddCommon(s, "OBJ_ADDALL", objAddAllReceiverID)
}
```

Confirm the production-side world adapter routes `receiverID == -1` to `zone.PublicReceiver`. Open `modules/world/script.go` `worldAdapter.AddObj` (added in T3 Step 1) and verify the receiverID passes through unchanged. Since `zone.PublicReceiver` is -1 (verify via grep), no translation is needed.

- [ ] **Step 4: Register opcode**

In `pkg/script/handlers.go`, append to the NAI-115 block:

```go
	OpObjAddAll: handleObjAddAll,
```

- [ ] **Step 5: Verify tests pass**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/ -run "TestHandleObjAdd|TestHandleObjAddAll" -v`
Expected: PASS (all subtests, including reused T3 cases).

- [ ] **Step 6: Commit**

```bash
git add pkg/script/handlers_obj.go pkg/script/handlers_obj_test.go pkg/script/handlers.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(script): NAI-115 T6 — OBJ_ADDALL handler (opcode 3501)

Twin of OBJ_ADD with broadcast receiverID (visible-to-all). Reuses
objAddCommon helper. Mirrors TS ObjOps.ts:58-93.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 7: P_OPOBJ (opcode 2080)

**TS reference:** `Engine-TS/src/engine/script/handlers/PlayerOps.ts:990-1006`

**Files:**
- Modify: `pkg/script/active.go` — add `SetInteractionScriptObj` to `ActivePlayer`
- Modify: `modules/world/player_script.go` — implement (mirror SetInteractionScriptLoc at line 921)
- Modify: `pkg/script/handlers_player.go` — add `handleP_OpObj`
- Modify: `pkg/script/handlers_player_test.go`
- Modify: `pkg/script/handlers.go`

- [ ] **Step 1: Extend ActivePlayer surface**

In `pkg/script/active.go`, locate the `SetInteractionScriptNpc` declaration (around line 425) and add immediately after:

```go
	// SetInteractionScriptObj anchors the player on `obj` with trigger
	// APOBJ<op>. Mirrors TS Player.setInteraction(Interaction.SCRIPT,
	// obj, ServerTriggerType.APOBJ1 + (op-1)). NAI-115 T7.
	SetInteractionScriptObj(obj ActiveObj, op int)
```

In `modules/world/player_script.go`, after `SetInteractionScriptNpc` (around line 947), add:

```go
// SetInteractionScriptObj implements script.ActivePlayer. Type-asserts
// the script-side ActiveObj surface to the world-side *entitypkg.Obj
// and routes via Player.SetInteraction with the SCRIPT interaction
// type. NAI-115 T7.
func (p *Player) SetInteractionScriptObj(obj script.ActiveObj, op int) {
	realObj, ok := obj.(*entitypkg.Obj)
	if !ok {
		return
	}
	p.SetInteraction(InteractionScript, realObj, op, -1)
}
```

The `SetInteraction(InteractionScript, target, op, -1)` 4-arg signature matches sister methods at line 926 (Loc) and 946 (Npc). The op parameter is the 1-based opnumber TS encodes as `APOBJ1 + (op-1)`; goscape's existing pattern (P_OPLOC) passes the raw 1-based op through and the SetInteraction layer maps it. Confirm by reading the receiving end of SetInteraction: `grep -n "func.*SetInteraction\b" modules/world/player.go modules/world/player_interaction.go 2>/dev/null`.

- [ ] **Step 2: Write failing test**

Append to `pkg/script/handlers_player_test.go`:

```go
// stubPlayerWithObjOp records SetInteractionScriptObj calls.
type stubPlayerWithObjOp struct {
	stubPlayer
	objOpCalls []objOpCall
}

type objOpCall struct {
	obj ActiveObj
	op  int
}

func (s *stubPlayerWithObjOp) SetInteractionScriptObj(obj ActiveObj, op int) {
	s.objOpCalls = append(s.objOpCalls, objOpCall{obj, op})
}

func TestHandleP_OpObjHappyPath(t *testing.T) {
	s := newTestState()
	pl := &stubPlayerWithObjOp{stubPlayer: stubPlayer{uid: 12345}}
	s.Self = pl
	s.Pointers = PtrActivePlayer | PtrProtectedActivePlayer
	active := &stubActiveObj{x: 3200, z: 3200, level: 0, typeID: 590}
	s.ActiveObj = active
	// Configs must report op[op-1] != "" so handler proceeds past the
	// objType.Op[op-1] gate. (Verify field name via grep; ObjType.Op is a
	// []string per pkg/objtype/objtype.go:108.)
	s.Configs = newTestConfigsWithObj(590, false, false, 0).withOp(0 /*op1 idx*/, "Light")

	s.PushInt(1) // op = 1

	if err := handleP_OpObj(s); err != nil {
		t.Fatalf("handleP_OpObj returned error: %v", err)
	}
	if !pl.stopActionCalled {
		t.Errorf("expected StopAction call")
	}
	if pl.lastQueueWaypoint != (queueWaypointArgs{x: 3200, z: 3200}) {
		t.Errorf("QueueWaypoint args: got %+v, want {3200, 3200}", pl.lastQueueWaypoint)
	}
	if len(pl.objOpCalls) != 1 || pl.objOpCalls[0].op != 1 {
		t.Errorf("SetInteractionScriptObj: got %+v, want 1 call op=1", pl.objOpCalls)
	}
}

func TestHandleP_OpObjOutOfRange(t *testing.T) {
	s := newTestState()
	pl := &stubPlayerWithObjOp{stubPlayer: stubPlayer{uid: 12345}}
	s.Self = pl
	s.Pointers = PtrActivePlayer | PtrProtectedActivePlayer
	s.ActiveObj = &stubActiveObj{x: 0, z: 0, level: 0, typeID: 590}
	s.Configs = newTestConfigsWithObj(590, false, false, 0)
	s.PushInt(6) // op = 6 → out of range (1..5)
	if err := handleP_OpObj(s); err == nil {
		t.Errorf("P_OPOBJ op=6: expected error, got nil")
	}
}

func TestHandleP_OpObjMissingOpEntryShortCircuits(t *testing.T) {
	s := newTestState()
	pl := &stubPlayerWithObjOp{stubPlayer: stubPlayer{uid: 12345}}
	s.Self = pl
	s.Pointers = PtrActivePlayer | PtrProtectedActivePlayer
	s.ActiveObj = &stubActiveObj{x: 0, z: 0, level: 0, typeID: 590}
	// Op[2] empty — TS returns silently.
	s.Configs = newTestConfigsWithObj(590, false, false, 0) // no withOp() call
	s.PushInt(3)                                            // op = 3 → Op[2]
	if err := handleP_OpObj(s); err != nil {
		t.Fatalf("P_OPOBJ missing op entry: expected nil-error short-circuit, got %v", err)
	}
	if len(pl.objOpCalls) != 0 {
		t.Errorf("P_OPOBJ missing op entry: expected 0 SetInteractionScriptObj calls, got %d", len(pl.objOpCalls))
	}
}

func TestHandleP_OpObjRequiresProtect(t *testing.T) {
	s := newTestState()
	pl := &stubPlayerWithObjOp{stubPlayer: stubPlayer{uid: 12345}}
	s.Self = pl
	s.Pointers = PtrActivePlayer // NO PtrProtectedActivePlayer
	s.ActiveObj = &stubActiveObj{x: 0, z: 0, level: 0, typeID: 590}
	s.Configs = newTestConfigsWithObj(590, false, false, 0).withOp(0, "Light")
	s.PushInt(1)
	if err := handleP_OpObj(s); err == nil {
		t.Errorf("P_OPOBJ without ProtectedActivePlayer: expected error, got nil")
	}
}
```

The `stubPlayer` already used in T3 needs `stopActionCalled` and `lastQueueWaypoint` fields for this test to compile. Search and extend:
- `grep -n "type stubPlayer\b\|stubPlayer struct" pkg/script/*_test.go` — locate the existing struct
- Add the recorder fields:

```go
type stubPlayer struct {
	uid                int
	stopActionCalled   bool
	lastQueueWaypoint  queueWaypointArgs
	// ... existing fields preserved
}

type queueWaypointArgs struct{ x, z int }

func (s *stubPlayer) UID() int                  { return s.uid }
func (s *stubPlayer) StopAction()               { s.stopActionCalled = true }
func (s *stubPlayer) QueueWaypoint(x, z int)    { s.lastQueueWaypoint = queueWaypointArgs{x, z} }
```

Verify per `mock_recorder_field_naming_check`: read the actual stubPlayer struct first; do not assume fields. If a sister recorder pattern already exists (e.g., a `mockPlayer` with `enqueueCalls`), follow that pattern's idiom.

- [ ] **Step 3: Verify test fails**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/ -run TestHandleP_OpObj -v`
Expected: FAIL — `undefined: handleP_OpObj`.

- [ ] **Step 4: Implement handler**

Append to `pkg/script/handlers_player.go` (after handleP_OpNpc at line 842):

```go
// handleP_OpObj (P_OPOBJ, opcode 2080) re-anchors the active player on
// the active obj with AP trigger APOBJ<op>. Mirrors TS
// PlayerOps.ts:990-1006.
func handleP_OpObj(s *ScriptState) error {
	if err := requireProtectedActivePlayer(s, "P_OPOBJ"); err != nil {
		return err
	}
	if s.ActiveObj == nil {
		return errors.New("P_OPOBJ: no active obj")
	}
	op := s.PopInt()
	if err := checkNotNull(op, "P_OPOBJ"); err != nil {
		return err
	}
	if op < 1 || op > 5 {
		return fmt.Errorf("P_OPOBJ: invalid op %d (must be 1..5)", op)
	}
	if s.Configs == nil {
		return errors.New("P_OPOBJ: no configs")
	}
	objType := s.Configs.ObjType(s.ActiveObj.ObjType())
	if objType == nil {
		return fmt.Errorf("P_OPOBJ: invalid active obj type (%d)", s.ActiveObj.ObjType())
	}
	// TS: type.op[op-1] === null → return (silent skip)
	if op-1 >= len(objType.Op) || objType.Op[op-1] == "" {
		return nil
	}
	x, z, _ := s.ActiveObj.Coords()
	s.Self.StopAction()
	s.Self.QueueWaypoint(x, z)
	s.Self.SetInteractionScriptObj(s.ActiveObj, op)
	return nil
}
```

If `errors` and `fmt` are not already imported in `handlers_player.go`, the file already imports them (per the P_OPLOC handler at line 801 using `errors.New` + `fmt.Errorf`).

- [ ] **Step 5: Register opcode**

In `pkg/script/handlers.go`, append to the NAI-115 block:

```go
	OpPOpObj: handleP_OpObj,
```

- [ ] **Step 6: Verify tests pass**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/ -run TestHandleP_OpObj -v`
Expected: PASS (all 4 subtests).

Then run full repo build + test guardrail:
```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./...
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...
```
Expected: clean / PASS.

- [ ] **Step 7: Commit**

```bash
git add pkg/script/active.go pkg/script/handlers_player.go pkg/script/handlers_player_test.go pkg/script/handlers.go modules/world/player_script.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(script,world): NAI-115 T7 — P_OPOBJ handler (opcode 2080)

Re-anchors active player on active obj with APOBJ<op> trigger.
Mirrors TS PlayerOps.ts:990-1006. Adds
ActivePlayer.SetInteractionScriptObj (twin of SetInteractionScriptLoc/
SetInteractionScriptNpc). Gates on ProtectedActivePlayer +
op ∈ [1,5] + ObjType.Op[op-1] non-empty.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Bundle 2 smoke checkpoint (OPTIONAL, USER-LAUNCHED)

After T7, dispatch a smoke handoff message:

> Bundle 2 (T6-T7) is complete. Optional smoke for non-Tutorial firemaking:
> 1. Walk to a non-Tutorial coord with `area_allow_loc_add` permitted (e.g., near Lumbridge, outside bank/duel/party-room zones).
> 2. Drop logs from inv (use ground-light path) or use tinderbox-on-logs from inv.
> 3. Expected: retry-loop completes after `stat_random` roll succeeds; ashes drop visible to all clients (broadcast).
>
> Pass: full firemaking-success in non-Tutorial zone with broadcast ashes. Skip if Bundle 1 smoke already exercises this content path.

---

## Close-out

- [ ] **Step 1: Update `nai_followups.md`**

Append a `## NAI-115 — CLOSED YYYY-MM-DD` section mirroring the NAI-114 close-memo style. Capture: scope (7 opcodes), bundle commits (T1-T7 SHAs), smoke result, deviation D1 (wealth-event skip), D2 (RemoveObj duration / Server.RemoveObj foundation gap), D3 (ActiveObj writeback after Add), and NAI-111 P_TELEJUMP carry-forward.

- [ ] **Step 2: Update `MEMORY.md` index**

Cross-reference any new memory entries (if any). If no new entry, leave the index untouched. Per `close_commit_memory_trailer`, end the close commit with a `Closes memory:` trailer enumerating any updated entries.

- [ ] **Step 3: Close commit**

```bash
git add /home/owner/.claude/projects/-home-owner-Code-github-com-zsrv-goscape/memory/nai_followups.md /home/owner/.claude/projects/-home-owner-Code-github-com-zsrv-goscape/memory/MEMORY.md
git commit --no-gpg-sign -m "$(cat <<'EOF'
chore(close): NAI-115 — firemaking opcode-cascade ports complete

7 opcodes ported in 2 bundles. Tutorial Island firemaking smoke
binds at Bundle 1 close (T5). Non-Tutorial broadcast-ashes at
Bundle 2 close (T7). Deviations tracked: D1 wealth-event skip
(INV_DROPSLOT), D2 Server.RemoveObj duration foundation gap
(OBJ_DEL/OBJ_ADD/OBJ_ADDALL), D3 ActiveObj writeback after Add.

Carry-forward: NAI-111 P_TELEJUMP investigation.

Closes memory: nai_followups.md (NAI-115 CLOSED section)

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Self-review checklist

- **Spec coverage:** §3 cadence, §4 Bundle 1, §5 Bundle 2, §6 risk register, §7 test strategy, §8 deviations all map to numbered tasks. D1, D2, D3 each tracked in commit messages and close-out.
- **Placeholder scan:** No "TBD", "TODO", "appropriate error handling" in any task. Every step has either a code block, command, or specific file/line citation.
- **Type consistency:** `objAddCommon` signature matches between T3 declaration and T6 usage. `SetInteractionScriptObj` signature consistent across T7 active.go declaration + player_script.go implementation + handler call. `WorldVars.AddObj` signature consistent between T3 declaration, T5 reuse, T6 reuse, and world adapter implementation.
- **Sibling-site grep guidance** (`plan_sibling_site_guard_audit`): T3 (Configs nil-check), T5 (resolveInv pattern), T7 (requireProtectedActivePlayer + checkNotNull) all reference the existing sibling pattern.
- **Helper-grep guidance** (`plan_grep_helper_patterns`): T3 reuses `checkCoord`; T5 reuses `requireProtectedActivePlayer` + `resolveInv`; T7 reuses `checkNotNull`.
- **mock recorder field naming** (`mock_recorder_field_naming_check`): T7 explicitly requires reading the actual `stubPlayer` struct before assuming `stopActionCalled` / `lastQueueWaypoint` field names.
- **Smoke handoff** (`smoke_test_server_handoff`): both smokes are explicitly user-launched.
