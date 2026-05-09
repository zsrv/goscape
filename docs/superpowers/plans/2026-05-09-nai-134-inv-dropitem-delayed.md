# NAI-134 — INV_DROPITEM_DELAYED + objDelayedQueue Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Port TS `INV_DROPITEM_DELAYED` (opcode 4310) and the `World.objDelayedQueue` per-tick spawn-delay queue to goscape, with full TS fidelity. Leaves `NAI-115-D2` (duration-despawn scheduler) untouched.

**Architecture:** New slice-based queue on `Server` (mirrors `worldScriptQueue`), drained per-tick between `processActiveScripts` and `processPlayerTimers`. Handler in `pkg/script` calls a new `WorldVars.EnqueueObjDelayed` surface, which `worldVarsView` constructs an `Obj` for and enqueues. Operand-aware protect gate per NAI-133.

**Tech Stack:** Go 1.26+; existing `pkg/script` (VM) ↔ `modules/world` (server) split; existing test fixtures `newTestServer` / `newZoneTestServer` / `mockWorld` / `fakeWorldAddObj`.

**Spec:** `docs/superpowers/specs/2026-05-09-nai-134-inv-dropitem-delayed-design.md`

**TS source anchors:**
- `Engine-TS/src/engine/script/handlers/InvOps.ts:188-209` — handler.
- `Engine-TS/src/engine/entity/ObjDelayedRequest.ts:1-22` — request type.
- `Engine-TS/src/engine/World.ts:157` — queue declaration.
- `Engine-TS/src/engine/World.ts:563-573` — drain loop.

**Pre-flight (verified at HEAD `b386dec`):**
- Helpers: `requireActivePlayer` (handlers_player.go:35), `checkInvType` (handlers_player.go:142), `checkCoord` (handlers_npc.go:13), `checkObjType` (handlers_obj.go:21), `checkObjStack` (handlers_obj.go:31), `checkDuration` (handlers_loc.go:278), `resolveInv` (handlers_inv.go:14).
- Pointer flags: `PtrProtectedActivePlayer = 1<<9`, `PtrProtectedActivePlayer2 = 1<<10` (pointer.go:22-28).
- Operand: `s.Script.IntOperands[s.PC]` is `int32`; comparisons against untyped `0`/`1` work as in `handleBothMoveInv` (handlers_inv.go:1230).
- `mockWorld` (handlers_vars_test.go:11) has default no-op stubs for every `WorldVars` method; `fakeWorldAddObj` (handlers_obj_test.go:68) embeds `*mockWorld` so it inherits stubs.
- `newTestServer` (server_test.go:311) sets `zoneMap` but **NOT** `zonesTracking`. Use `newZoneTestServer` (world_zone_test.go:13) for any test that triggers `s.AddObj` → `s.TrackZone`.
- `Obj.Type` is the field name (not `TypeID`) — pkg/entity/obj.go:12.
- Server has `s.log *slog.Logger` (server.go ~line 110).
- All commits use `--no-gpg-sign` (global CLAUDE.md).
- Run go via: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go ...`

---

## File Structure

| File | Status | Responsibility |
|---|---|---|
| `modules/world/obj_delayed_queue.go` | **Create** | `objDelayedRequest` type, `Server.enqueueObjDelayed`, `Server.processObjDelayedQueue`. ~35 LOC. |
| `modules/world/obj_delayed_queue_test.go` | **Create** | Queue-mechanics tests (post-decrement, drain ordering, panic recovery). ~120 LOC. |
| `modules/world/server.go` | Modify | Add `objDelayedQueue []objDelayedRequest` field on `Server` next to `worldScriptQueue`. ~1 LOC. |
| `modules/world/tick_recovery.go` | Modify | Add `recoverObjDelayed(req objDelayedRequest, log *slog.Logger)` next to `recoverWorldScript`. ~15 LOC. |
| `modules/world/tick.go` | Modify | Insert `s.processObjDelayedQueue()` between `processActiveScripts()` and `processPlayerTimers()`. ~1 LOC + comment. |
| `modules/world/server_varp.go` | Modify | Add `worldVarsView.EnqueueObjDelayed`. ~25 LOC. |
| `pkg/script/state.go` | Modify | Add `EnqueueObjDelayed(level,x,z,typeID,count,duration,delay,receiverID int)` method to `WorldVars` interface. ~5 LOC. |
| `pkg/script/handlers_vars_test.go` | Modify | Add default `EnqueueObjDelayed` stub on `mockWorld` (test fixture). ~5 LOC. |
| `pkg/script/handlers_obj_test.go` | Modify | Add `enqueueObjDelayedCalls` recorder + override on `fakeWorldAddObj`. ~12 LOC. |
| `pkg/script/handlers_inv.go` | Modify | Add `handleInvDropItemDelayed` (~60 LOC). |
| `pkg/script/handlers.go` | Modify | One-line dispatch wire `OpInvDropItemDelayed: handleInvDropItemDelayed,`. |
| `pkg/script/handlers_inv_test.go` | Modify | 14 tests for the handler. ~280 LOC. |

**LOC budget:** ~560 LOC total (production ~95, test ~430, plumbing ~35).

---

## Task 1: Queue infrastructure (type, enqueue, drain, recovery, tests)

**Files:**
- Create: `modules/world/obj_delayed_queue.go`
- Create: `modules/world/obj_delayed_queue_test.go`
- Modify: `modules/world/server.go` — add `objDelayedQueue` field
- Modify: `modules/world/tick_recovery.go` — add `recoverObjDelayed`

- [ ] **Step 1.1: Add `objDelayedQueue` field on Server**

Open `modules/world/server.go`, find the field `worldScriptQueue   []worldScriptQueueEntry`, and add immediately below it:

```go
	objDelayedQueue []objDelayedRequest // NAI-134
```

- [ ] **Step 1.2: Write the failing queue-mechanics tests**

Create `modules/world/obj_delayed_queue_test.go`:

```go
package world

import (
	"testing"

	entitypkg "github.com/zsrv/goscape/pkg/entity"
	"github.com/zsrv/goscape/pkg/zone"
)

// TestObjDelayedQueue_DelayZeroFiresImmediately pins TS post-decrement
// semantics: stored delay=0, captured 0, decrement to -1, 0>0 false → fires
// on the very first drain after enqueue. Mirrors TS World.ts:564.
func TestObjDelayedQueue_DelayZeroFiresImmediately(t *testing.T) {
	s := newZoneTestServer(t)
	obj := entitypkg.NewObj(0, 3094, 3106, entitypkg.LifecycleDespawn, 995, 10)
	s.enqueueObjDelayed(obj, zone.PublicReceiver, 200, 0)

	if got := len(s.objDelayedQueue); got != 1 {
		t.Fatalf("post-enqueue queue len: got %d, want 1", got)
	}

	s.processObjDelayedQueue()

	if got := len(s.objDelayedQueue); got != 0 {
		t.Errorf("delay=0: queue should drain on first call, got len %d", got)
	}
	if got := len(s.zonesTracking); got != 1 {
		t.Errorf("delay=0: drain should call s.AddObj → TrackZone; zonesTracking len got %d, want 1", got)
	}
}

// TestObjDelayedQueue_FiresAfterDelayTicks pins user delay=2 → first 2
// drain calls skip; the 3rd fires (captured 2 → skip; captured 1 → skip;
// captured 0 → fire).
func TestObjDelayedQueue_FiresAfterDelayTicks(t *testing.T) {
	s := newZoneTestServer(t)
	obj := entitypkg.NewObj(0, 3094, 3106, entitypkg.LifecycleDespawn, 995, 10)
	s.enqueueObjDelayed(obj, zone.PublicReceiver, 200, 2)

	s.processObjDelayedQueue()
	if got := len(s.objDelayedQueue); got != 1 {
		t.Errorf("after drain 1 (captured 2): queue len got %d, want 1", got)
	}
	s.processObjDelayedQueue()
	if got := len(s.objDelayedQueue); got != 1 {
		t.Errorf("after drain 2 (captured 1): queue len got %d, want 1", got)
	}
	s.processObjDelayedQueue()
	if got := len(s.objDelayedQueue); got != 0 {
		t.Errorf("after drain 3 (captured 0): queue len got %d, want 0", got)
	}
}

// TestObjDelayedQueue_DrainCallsServerAddObj pins drain → s.AddObj routing.
// After fire: zoneMap-resolved zone has the obj, receiverID propagated.
func TestObjDelayedQueue_DrainCallsServerAddObj(t *testing.T) {
	s := newZoneTestServer(t)
	const receiverUID = 0x1234
	obj := entitypkg.NewObj(0, 3094, 3106, entitypkg.LifecycleDespawn, 995, 10)
	obj.ReceiverID = receiverUID
	s.enqueueObjDelayed(obj, receiverUID, 200, 0)

	s.processObjDelayedQueue()

	z := s.zoneMap.Get(0, 3094, 3106)
	if z == nil {
		t.Fatalf("expected zone at (0,3094,3106) to exist after drain")
	}
	if got := obj.ReceiverID; got != receiverUID {
		t.Errorf("obj.ReceiverID after drain: got %d, want %d", got, receiverUID)
	}
}

// TestObjDelayedQueue_MultipleEntriesIndependentDelays pins per-entry
// delay independence: enqueue {0,1,2} → drain 1 fires entry-0 only, drain
// 2 fires entry-1 only, drain 3 fires entry-2 only.
func TestObjDelayedQueue_MultipleEntriesIndependentDelays(t *testing.T) {
	s := newZoneTestServer(t)
	objA := entitypkg.NewObj(0, 3094, 3106, entitypkg.LifecycleDespawn, 995, 1)
	objB := entitypkg.NewObj(0, 3200, 3200, entitypkg.LifecycleDespawn, 995, 2)
	objC := entitypkg.NewObj(0, 3300, 3300, entitypkg.LifecycleDespawn, 995, 3)
	s.enqueueObjDelayed(objA, zone.PublicReceiver, 200, 0)
	s.enqueueObjDelayed(objB, zone.PublicReceiver, 200, 1)
	s.enqueueObjDelayed(objC, zone.PublicReceiver, 200, 2)

	s.processObjDelayedQueue()
	if got := len(s.objDelayedQueue); got != 2 {
		t.Errorf("after drain 1: queue len got %d, want 2 (objA fired)", got)
	}
	s.processObjDelayedQueue()
	if got := len(s.objDelayedQueue); got != 1 {
		t.Errorf("after drain 2: queue len got %d, want 1 (objB fired)", got)
	}
	s.processObjDelayedQueue()
	if got := len(s.objDelayedQueue); got != 0 {
		t.Errorf("after drain 3: queue len got %d, want 0 (objC fired)", got)
	}
}

// TestObjDelayedQueue_DurationStoredAtEnqueueDroppedAtDrain pins NAI-115-D2
// parity: duration is stored on the queue entry verbatim at enqueue, then
// silently discarded at drain (Server.AddObj does not yet accept duration).
func TestObjDelayedQueue_DurationStoredAtEnqueueDroppedAtDrain(t *testing.T) {
	s := newZoneTestServer(t)
	obj := entitypkg.NewObj(0, 3094, 3106, entitypkg.LifecycleDespawn, 995, 10)
	const wantDuration = 17
	s.enqueueObjDelayed(obj, zone.PublicReceiver, wantDuration, 0)

	if got := s.objDelayedQueue[0].duration; got != wantDuration {
		t.Errorf("enqueue: duration field got %d, want %d", got, wantDuration)
	}

	s.processObjDelayedQueue()
	// No observable side-effect from duration today (NAI-115-D2 open).
	// This test pins the *plumbing* — that the field round-trips through
	// the queue intact — so a future NAI-115-D2 closure has the entry to
	// read at drain time.
	if got := len(s.objDelayedQueue); got != 0 {
		t.Errorf("post-drain queue len got %d, want 0", got)
	}
}

// TestObjDelayedQueue_RemoveBeforeFire_PanicRecovery pins recover-then-
// log-then-continue semantics: if AddObj panics inside fire,
// recoverObjDelayed swallows the panic, the entry is already removed
// from the queue (remove-before-fire), and the next iteration sees the
// next entry. Mirrors recoverWorldScript pattern at world_script_queue.go:75.
//
// Trigger panic by enqueuing a nil Obj — Server.AddObj nil-derefs at
// obj.Level on the first line. recoverObjDelayed must handle the nil
// case in its log-field extraction.
func TestObjDelayedQueue_RemoveBeforeFire_PanicRecovery(t *testing.T) {
	s := newZoneTestServer(t)
	good := entitypkg.NewObj(0, 3094, 3106, entitypkg.LifecycleDespawn, 995, 10)
	s.enqueueObjDelayed(nil, zone.PublicReceiver, 200, 0) // nil-Obj triggers panic on AddObj
	s.enqueueObjDelayed(good, zone.PublicReceiver, 200, 0)

	// Should not panic the test goroutine — recoverObjDelayed swallows.
	s.processObjDelayedQueue()

	if got := len(s.objDelayedQueue); got != 0 {
		t.Errorf("post-drain queue len got %d, want 0 (both entries removed even with panic)", got)
	}
}
```

- [ ] **Step 1.3: Run the queue-mechanics tests to verify they fail**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestObjDelayedQueue -v`
Expected: FAIL with build errors — `enqueueObjDelayed` and `processObjDelayedQueue` undefined.

- [ ] **Step 1.4: Add `recoverObjDelayed` in tick_recovery.go**

Open `modules/world/tick_recovery.go`. It currently imports `"log/slog"`, `"runtime/debug"`, and `"github.com/zsrv/goscape/pkg/script"`. The recover function for the new queue does NOT need `pkg/script` (uses no script types) — append at end of file:

```go
// recoverObjDelayed recovers from panics during objDelayedQueue fire
// (NAI-134). Mirrors recoverWorldScript: structured log + swallow. The
// offending request was already removed before fire (per
// processObjDelayedQueue's remove-before-fire ordering), so recovery
// only logs.
//
// Mirrors TS World.ts:566-572 catch action.
func recoverObjDelayed(req objDelayedRequest, log *slog.Logger) {
	r := recover()
	if r == nil {
		return
	}
	typeID := -1
	if req.obj != nil {
		typeID = req.obj.Type
	}
	log.Error("panic in objDelayedQueue fire",
		"typeID", typeID,
		"receiverID", req.receiverID,
		"duration", req.duration,
		"err", r,
		"stack", string(debug.Stack()))
}
```

- [ ] **Step 1.5: Create `obj_delayed_queue.go` with type + enqueue + drain**

Create `modules/world/obj_delayed_queue.go`:

```go
package world

import (
	entitypkg "github.com/zsrv/goscape/pkg/entity"
)

// objDelayedRequest is one INV_DROPITEM_DELAYED request awaiting drain.
// Mirrors TS ObjDelayedRequest (Engine-TS/src/engine/entity/ObjDelayedRequest.ts).
//
// DEVIATION-NAI-134-D1: TS uses LinkList<ObjDelayedRequest> (Linkable mixin).
// Goscape uses a slice on Server, mirroring worldScriptQueue. Behavior identical.
//
// DEVIATION-NAI-115-D2 (sibling): duration is plumbed into the entry at
// enqueue but discarded at drain because Server.AddObj does not yet accept
// a duration param (no despawn-after-N-ticks scheduler). The discard
// mirrors worldVarsView.AddObj's existing `_ = duration` at server_varp.go.
// Single-point retire when NAI-115-D2 closes.
type objDelayedRequest struct {
	obj        *entitypkg.Obj
	receiverID int
	duration   int // see DEVIATION-NAI-115-D2 above
	delay      int // ticks remaining; post-decrement per TS World.ts:564
}

// enqueueObjDelayed appends a request to s.objDelayedQueue. Called by
// worldVarsView.EnqueueObjDelayed (server_varp.go) which is in turn
// driven by handleInvDropItemDelayed (pkg/script/handlers_inv.go).
//
// Mirrors TS World.objDelayedQueue.addTail at InvOps.ts:208. No `+1`
// offset — TS stores delay verbatim (unlike worldScriptQueue which
// stores delay+1 per TS World.ts:1239).
func (s *Server) enqueueObjDelayed(obj *entitypkg.Obj, receiverID, duration, delay int) {
	s.objDelayedQueue = append(s.objDelayedQueue, objDelayedRequest{
		obj:        obj,
		receiverID: receiverID,
		duration:   duration,
		delay:      delay,
	})
}

// processObjDelayedQueue drains ready entries from s.objDelayedQueue,
// firing each by calling s.AddObj (zone routing).
//
// Index-based slice walk with mid-pass append visibility (re-reads
// len(s.objDelayedQueue) each iteration), mirroring processWorldQueue
// (world_script_queue.go:59). Removal happens BEFORE fire so a panicking
// fire path doesn't leave a dead entry in the queue (recoverObjDelayed
// in tick_recovery.go).
//
// Mirrors TS World.cycle objDelayedQueue iteration at World.ts:563-573,
// including the per-iteration try/catch (mirrors NAI-42 pattern).
func (s *Server) processObjDelayedQueue() {
	i := 0
	for i < len(s.objDelayedQueue) {
		e := &s.objDelayedQueue[i]
		// POST-decrement: capture current, then decrement. Mirrors TS
		// World.ts:564 (`const delay = request.delay--;`).
		delay := e.delay
		e.delay--
		if delay > 0 {
			i++
			continue
		}
		req := *e
		s.objDelayedQueue = append(s.objDelayedQueue[:i], s.objDelayedQueue[i+1:]...)
		func(req objDelayedRequest) {
			defer recoverObjDelayed(req, s.log)
			s.AddObj(req.obj, req.receiverID)
			_ = req.duration // DEVIATION-NAI-115-D2 sibling — duration not honored here
		}(req)
		// Don't advance i — slice contracted under us (mirrors processWorldQueue).
	}
}
```

- [ ] **Step 1.6: Run the queue-mechanics tests to verify they pass**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestObjDelayedQueue -v`
Expected: PASS — all 6 tests green.

- [ ] **Step 1.7: Run the full module to catch regressions**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -count=1`
Expected: PASS — no regressions in adjacent tests (worldScriptQueue, npcEventQueue, etc.).

- [ ] **Step 1.8: Commit**

```bash
git add modules/world/obj_delayed_queue.go modules/world/obj_delayed_queue_test.go modules/world/server.go modules/world/tick_recovery.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(nai-134): T1 — objDelayedQueue infra (type, enqueue, drain, recovery)

Per docs/superpowers/specs/2026-05-09-nai-134-inv-dropitem-delayed-design.md
§4-§6.1. Adds slice-based queue on Server mirroring worldScriptQueue:
post-decrement drain semantics (TS World.ts:564), remove-before-fire
ordering, panic recovery via recoverObjDelayed (tick_recovery.go).

Drain not yet wired into runTickLoop — that lands in T2.

Tests: 6 cases pin delay=0 fire-immediately, delay=N fires-after-N+1
drains, multi-entry independence, AddObj routing, duration
round-trip-through-entry (NAI-115-D2 parity), panic recovery.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 2: Tick-loop integration

**Files:**
- Modify: `modules/world/tick.go`

- [ ] **Step 2.1: Insert drain step into runTickLoop**

Open `modules/world/tick.go` at the `runTickLoopWithRate` body. Find:

```go
		s.processNpcEventQueue()
		s.processActiveScripts()
		s.processPlayerTimers()
```

Replace with:

```go
		s.processNpcEventQueue()
		s.processActiveScripts()
		// NAI-134: drain the obj-delayed-spawn queue. Mirrors TS
		// World.cycle ordering at World.ts:563 — runs after script-firing
		// (so same-tick INV_DROPITEM_DELAYED with delay=0 spawns the obj
		// before processNpcs / processInfo reads zone state).
		s.processObjDelayedQueue()
		s.processPlayerTimers()
```

- [ ] **Step 2.2: Run module tests to verify no regression**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -count=1`
Expected: PASS — adjacent tick-order tests (NAI-122 npc-event, NAI-37 world-queue, NAI-77 walk-trigger) unaffected.

- [ ] **Step 2.3: Commit**

```bash
git add modules/world/tick.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(nai-134): T2 — wire processObjDelayedQueue into runTickLoop

Inserts drain step between processActiveScripts and processPlayerTimers,
matching TS World.cycle ordering at World.ts:563 (objDelayedQueue drained
after script-firing, before npc loops).

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 3: WorldVars interface widening + mock fixtures

**Files:**
- Modify: `pkg/script/state.go` — widen `WorldVars`
- Modify: `pkg/script/handlers_vars_test.go` — add default stub on `mockWorld`
- Modify: `pkg/script/handlers_obj_test.go` — add recorder on `fakeWorldAddObj`

- [ ] **Step 3.1: Widen WorldVars interface**

Open `pkg/script/state.go`. Find the `AddObj` declaration in the `WorldVars` interface (line ~109):

```go
	// Used by OBJ_ADD, OBJ_ADDALL, INV_DROPSLOT.
	AddObj(level, x, z, typeID, count, duration, receiverID int) ActiveObj
```

Add immediately below it:

```go
	// EnqueueObjDelayed appends an INV_DROPITEM_DELAYED request to the
	// world's per-tick spawn-delay queue. The Obj is constructed at the
	// implementation side (worldVarsView in modules/world). Mirrors TS
	// World.objDelayedQueue.addTail at InvOps.ts:208.
	//
	// duration is plumbed through but currently discarded at drain
	// (NAI-115-D2 foundation gap; mirrors worldVarsView.AddObj's
	// existing `_ = duration`). Used by INV_DROPITEM_DELAYED.
	EnqueueObjDelayed(level, x, z, typeID, count, duration, delay, receiverID int)
```

- [ ] **Step 3.2: Add default stub on `mockWorld`**

Open `pkg/script/handlers_vars_test.go`. Find the `AddObj` stub at line 70:

```go
// NAI-115 T3: default no-op stub for OBJ_ADD/OBJ_ADDALL/INV_DROPSLOT
// test fixture. Tests exercising AddObj override via fakeWorldAddObj.
func (m *mockWorld) AddObj(level, x, z, typeID, count, duration, receiverID int) ActiveObj {
	return nil
}
```

Add immediately below:

```go
// NAI-134: default no-op stub for INV_DROPITEM_DELAYED test fixture.
// Tests exercising EnqueueObjDelayed override via fakeWorldAddObj
// (handlers_obj_test.go).
func (m *mockWorld) EnqueueObjDelayed(level, x, z, typeID, count, duration, delay, receiverID int) {
}
```

- [ ] **Step 3.3: Add recorder on `fakeWorldAddObj`**

Open `pkg/script/handlers_obj_test.go`. Find the `fakeWorldAddObj` type and its `AddObj` method (around line 68-78):

```go
type fakeWorldAddObj struct {
	*mockWorld
	addedCalls []addObjCall
}
```

Locate the `addObjCall` struct (just above `fakeWorldAddObj`) and add an analogous `enqueueObjDelayedCall` plus a recorder field. After the `addedCalls []addObjCall` field, add:

```go
	enqueueObjDelayedCalls []enqueueObjDelayedCall
```

Above the `fakeWorldAddObj` type declaration, add the call-record type:

```go
// enqueueObjDelayedCall captures one EnqueueObjDelayed invocation for
// NAI-134 INV_DROPITEM_DELAYED tests. Field order mirrors the WorldVars
// signature exactly (level, x, z, typeID, count, duration, delay,
// receiverID).
type enqueueObjDelayedCall struct {
	level, x, z, typeID, count, duration, delay, receiverID int
}
```

After the existing `(f *fakeWorldAddObj) AddObj(...)` method (around line 77), add:

```go
func (f *fakeWorldAddObj) EnqueueObjDelayed(level, x, z, typeID, count, duration, delay, receiverID int) {
	f.enqueueObjDelayedCalls = append(f.enqueueObjDelayedCalls, enqueueObjDelayedCall{
		level: level, x: x, z: z,
		typeID: typeID, count: count,
		duration: duration, delay: delay,
		receiverID: receiverID,
	})
}
```

- [ ] **Step 3.4: Verify build and existing tests still green**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./...`
Expected: clean build.

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/ -count=1`
Expected: PASS — no regression. The `mockWorld` stub satisfies the widened interface; `fakeWorldAddObj` inherits the stub via its embedded `*mockWorld` and overrides it.

- [ ] **Step 3.5: Commit**

```bash
git add pkg/script/state.go pkg/script/handlers_vars_test.go pkg/script/handlers_obj_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(nai-134): T3 — widen WorldVars with EnqueueObjDelayed + test mocks

Adds the script-side surface for INV_DROPITEM_DELAYED: WorldVars gains
EnqueueObjDelayed(level,x,z,typeID,count,duration,delay,receiverID).
mockWorld gets a default no-op stub; fakeWorldAddObj gains a recorder
(enqueueObjDelayedCalls []enqueueObjDelayedCall) for handler-side tests.

No production wiring yet — worldVarsView impl lands in T4, handler in T5.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 4: worldVarsView.EnqueueObjDelayed implementation

**Files:**
- Modify: `modules/world/server_varp.go`

- [ ] **Step 4.1: Add EnqueueObjDelayed impl**

Open `modules/world/server_varp.go`. Find the `worldVarsView.AddObj` method (around line 164-184). Add immediately below it:

```go
// EnqueueObjDelayed implements script.WorldVars.EnqueueObjDelayed
// (NAI-134). Constructs a DESPAWN-lifecycle Obj at (level,x,z) with
// typeID/count, sets ReceiverID, and appends to s.objDelayedQueue via
// s.enqueueObjDelayed.
//
// The Obj is constructed at enqueue time (not drain time), mirroring TS
// InvOps.ts:207-208 where `new Obj(...)` is the call-site argument to
// `objDelayedQueue.addTail`.
//
// NAI-115-D2 sibling: duration is plumbed onto the queue entry but the
// drain (processObjDelayedQueue, obj_delayed_queue.go) discards it
// because Server.AddObj does not yet accept a duration param. Single-point
// retire when NAI-115-D2 closes.
func (w worldVarsView) EnqueueObjDelayed(level, x, z, typeID, count, duration, delay, receiverID int) {
	if w.s == nil {
		return
	}
	obj := entitypkg.NewObj(level, x, z, entitypkg.LifecycleDespawn, typeID, count)
	obj.ReceiverID = receiverID
	w.s.enqueueObjDelayed(obj, receiverID, duration, delay)
	if w.s.cfg.NodeDebug && w.s.log != nil {
		w.s.log.Info("nai134.obj.delayed.enqueue",
			"level", level, "x", x, "z", z,
			"typeID", typeID, "count", count,
			"duration", duration, "delay", delay,
			"receiverID", receiverID,
		)
	}
}
```

- [ ] **Step 4.2: Build to verify worldVarsView still satisfies WorldVars**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./...`
Expected: clean build.

- [ ] **Step 4.3: Run module tests to verify no regression**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -count=1`
Expected: PASS.

- [ ] **Step 4.4: Commit**

```bash
git add modules/world/server_varp.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(nai-134): T4 — worldVarsView.EnqueueObjDelayed impl

Constructs DESPAWN-lifecycle Obj at handler-supplied coords, propagates
receiverID, calls Server.enqueueObjDelayed. Mirrors TS InvOps.ts:207-208
where `new Obj(...)` is the call-site argument to addTail.

Includes NodeDebug gateway log at "nai134.obj.delayed.enqueue" for the
production-residual diagnostic pattern (per nodedebug_gateway_probe_pattern
memory).

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 5: handleInvDropItemDelayed — handler core + dispatch + 3 baseline tests

**Files:**
- Modify: `pkg/script/handlers_inv.go` — add handler
- Modify: `pkg/script/handlers.go` — dispatch wire
- Modify: `pkg/script/handlers_inv_test.go` — 3 baseline tests

- [ ] **Step 5.1: Write the 3 baseline failing tests**

Open `pkg/script/handlers_inv_test.go`. At end of file, append:

```go
// -- NAI-134 INV_DROPITEM_DELAYED tests --

// makeDropItemDelayedState builds a direct-call test fixture matching
// the existing newInvAddOverflowState pattern (handlers_inv_test.go:1038).
// Sets up: configs with one InvType (Protect/Scope per args) and one
// stackable ObjType, an inventory pre-loaded with `count` of the obj, an
// mockPlayer at (0,3200,3200) with uid=12345, PtrActivePlayer set, and
// a fakeWorldAddObj recorder wired into state.World.
//
// Returns (state, inv, world). Caller pushes int args in TS pop-order
// before calling handleInvDropItemDelayed directly.
func makeDropItemDelayedState(t *testing.T, protect bool, scope objtype.InvTypeScope, count int) (*ScriptState, *inventory.Inventory, *fakeWorldAddObj) {
	t.Helper()
	s := newTestState(minimalScript(OpReturn))

	mc := newTestInvConfigs()
	invType := objtype.NewInvType(testInvMain)
	invType.DebugName = "test_inv"
	invType.Size = 28
	invType.Protect = protect
	invType.Scope = scope
	mc.invs[testInvMain] = invType
	s.Configs = mc

	inv := inventory.New(testInvMain, 28, inventory.StackNormal)
	if count > 0 {
		inv.Items[0] = &inventory.Item{Id: testObjCoin, Count: count}
	}
	s.Inv = &mockInvLookup{invs: map[int]*inventory.Inventory{testInvMain: inv}}

	s.Self = &mockPlayer{
		uidValue:    12345,
		coordPacked: coordgrid.PackCoord(0, 3200, 3200),
		x:           3200,
		z:           3200,
	}
	s.Pointers |= PtrActivePlayer

	world := &fakeWorldAddObj{mockWorld: newMockWorld()}
	s.World = world
	return s, inv, world
}

// pushDropItemDelayedArgs pushes the 6 args in TS pop-order
// (invID at bottom, delay on top): handler PopInts in reverse.
func pushDropItemDelayedArgs(s *ScriptState, invID, coord, obj, count, duration, delay int) {
	s.PushInt(invID)
	s.PushInt(coord)
	s.PushInt(obj)
	s.PushInt(count)
	s.PushInt(duration)
	s.PushInt(delay)
}

// TestInvDropItemDelayed_NoActivePlayer_Errors pins the requireActivePlayer
// guard. Without PtrActivePlayer set, handler returns an error.
func TestInvDropItemDelayed_NoActivePlayer_Errors(t *testing.T) {
	s, _, world := makeDropItemDelayedState(t, false, objtype.InvTypeScopeTemp, 5)
	s.Pointers &^= PtrActivePlayer

	pushDropItemDelayedArgs(s, testInvMain, coordgrid.PackCoord(0, 3200, 3200), testObjCoin, 1, 100, 5)
	err := handleInvDropItemDelayed(s)

	if err == nil {
		t.Fatalf("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "no active player") {
		t.Errorf("err: got %q, want substring \"no active player\"", err)
	}
	if got := len(world.enqueueObjDelayedCalls); got != 0 {
		t.Errorf("error path: expected 0 enqueue calls, got %d", got)
	}
}

// TestInvDropItemDelayed_HappyPath_EnqueueArgs pins the success path:
// validators pass, protect-gate skipped (Protect=false), inv.Remove
// succeeds with completed=count, EnqueueObjDelayed receives every arg
// verbatim including delay.
func TestInvDropItemDelayed_HappyPath_EnqueueArgs(t *testing.T) {
	s, _, world := makeDropItemDelayedState(t, false, objtype.InvTypeScopeTemp, 5)
	const wantDelay = 7
	const wantDuration = 100

	pushDropItemDelayedArgs(s, testInvMain, coordgrid.PackCoord(0, 3200, 3200), testObjCoin, 3, wantDuration, wantDelay)
	err := handleInvDropItemDelayed(s)

	if err != nil {
		t.Fatalf("happy path: unexpected error: %v", err)
	}
	if got := len(world.enqueueObjDelayedCalls); got != 1 {
		t.Fatalf("expected 1 enqueue call, got %d", got)
	}
	c := world.enqueueObjDelayedCalls[0]
	if c.level != 0 || c.x != 3200 || c.z != 3200 {
		t.Errorf("enqueue coord: got level=%d x=%d z=%d, want 0/3200/3200", c.level, c.x, c.z)
	}
	if c.typeID != testObjCoin {
		t.Errorf("enqueue typeID: got %d, want %d", c.typeID, testObjCoin)
	}
	if c.count != 3 {
		t.Errorf("enqueue count: got %d, want 3 (TS uses Remove.completed)", c.count)
	}
	if c.duration != wantDuration {
		t.Errorf("enqueue duration: got %d, want %d", c.duration, wantDuration)
	}
	if c.delay != wantDelay {
		t.Errorf("enqueue delay: got %d, want %d", c.delay, wantDelay)
	}
	if c.receiverID != s.Self.UID() {
		t.Errorf("enqueue receiverID: got %d, want %d (Self.UID)", c.receiverID, s.Self.UID())
	}
	// TS-asymmetry vs INV_DROPITEM: ActiveObj NOT set (obj does not yet exist).
	if s.ActiveObj != nil {
		t.Errorf("DoesNotSetActiveObj: state.ActiveObj got %v, want nil", s.ActiveObj)
	}
	if s.Pointers&PtrActiveObj != 0 {
		t.Errorf("DoesNotSetActiveObj: PtrActiveObj should not be set, pointers=%b", s.Pointers)
	}
}

// TestInvDropItemDelayed_RemoveCompletedZero_NoEnqueue pins TS
// InvOps.ts:203-205: when inv.Remove returns completed=0 (empty inv),
// handler returns nil and does NOT enqueue.
func TestInvDropItemDelayed_RemoveCompletedZero_NoEnqueue(t *testing.T) {
	// Empty inv (count=0 → Items[0] not seeded → Remove returns completed=0).
	s, _, world := makeDropItemDelayedState(t, false, objtype.InvTypeScopeTemp, 0)

	pushDropItemDelayedArgs(s, testInvMain, coordgrid.PackCoord(0, 3200, 3200), testObjCoin, 1, 100, 0)
	err := handleInvDropItemDelayed(s)

	if err != nil {
		t.Fatalf("completed=0 path: unexpected error: %v", err)
	}
	if got := len(world.enqueueObjDelayedCalls); got != 0 {
		t.Errorf("completed=0: expected 0 enqueue calls, got %d", got)
	}
}
```

- [ ] **Step 5.2: Run tests to verify they fail**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/ -run TestInvDropItemDelayed -v`
Expected: FAIL with "handleInvDropItemDelayed undefined" build error.

- [ ] **Step 5.3: Add handleInvDropItemDelayed**

Open `pkg/script/handlers_inv.go`. Append at end of file:

```go
// handleInvDropItemDelayed (INV_DROPITEM_DELAYED, opcode 4310) ports
// TS InvOps.ts:188-209. Pops [inv, coord, obj, count, duration, delay].
// Removes count of obj from inv; if completed > 0, enqueues an
// ObjDelayedRequest onto World.objDelayedQueue (drained per-tick by
// Server.processObjDelayedQueue at modules/world/obj_delayed_queue.go).
//
// Validator chain (mirrors handleInvDropItem at handlers_inv.go:1142):
// InvTypeValid → CoordValid → ObjTypeValid → ObjStackValid → DurationValid
// → operand-aware protect-gate.
//
// `delay` is unvalidated — TS InvOps.ts:188-195 lacks DelayValid.
//
// Operand-aware protect gate (NAI-133 slot routing): operand=0 selects
// PtrProtectedActivePlayer; operand=1 selects PtrProtectedActivePlayer2.
// Out-of-range operand returns an error.
//
// TS-asymmetry vs INV_DROPITEM (handlers_inv.go:1142-1203): does NOT set
// state.ActiveObj or PtrActiveObj — the obj does not yet exist as a
// tracked world entity at enqueue time. TS verbatim at InvOps.ts:206-208.
//
// DEVIATION-NAI-130-D2 sibling: defensive nil-World guard returns clean
// error rather than nil-deref, matching handleInvDropItem.
func handleInvDropItemDelayed(s *ScriptState) error {
	if err := requireActivePlayer(s, "INV_DROPITEM_DELAYED"); err != nil {
		return err
	}
	delay := s.PopInt()
	duration := s.PopInt()
	count := s.PopInt()
	obj := s.PopInt()
	coord := s.PopInt()
	invID := s.PopInt()

	if err := checkInvType(s, invID, "INV_DROPITEM_DELAYED"); err != nil {
		return err
	}
	level, x, z, err := checkCoord(coord, "INV_DROPITEM_DELAYED")
	if err != nil {
		return err
	}
	if err := checkObjType(s, obj, "INV_DROPITEM_DELAYED"); err != nil {
		return err
	}
	if err := checkObjStack(count, "INV_DROPITEM_DELAYED"); err != nil {
		return err
	}
	if err := checkDuration(duration); err != nil {
		return fmt.Errorf("INV_DROPITEM_DELAYED: %w", err)
	}

	// Operand-aware protect gate (NAI-133 slot routing).
	operand := s.Script.IntOperands[s.PC]
	if operand != 0 && operand != 1 {
		return fmt.Errorf("INV_DROPITEM_DELAYED: invalid intOperand %d", operand)
	}
	protectFlag := PtrProtectedActivePlayer
	if operand == 1 {
		protectFlag = PtrProtectedActivePlayer2
	}
	invType := s.Configs.InvType(invID)
	if invType.Protect && invType.Scope != objtype.InvTypeScopeShared && s.Pointers&protectFlag == 0 {
		return fmt.Errorf("INV_DROPITEM_DELAYED: $inv requires protected access: %s", invType.DebugName)
	}

	inv := resolveInv(s, invID)
	if inv == nil {
		return fmt.Errorf("INV_DROPITEM_DELAYED: inv unresolved (id=%d)", invID)
	}
	tx := inv.Remove(obj, count, inventory.RemoveOpts{BeginSlot: -1})
	completed := tx.Completed
	if completed == 0 {
		return nil
	}
	if s.World == nil {
		return fmt.Errorf("INV_DROPITEM_DELAYED: no world surface")
	}
	s.World.EnqueueObjDelayed(level, x, z, obj, completed, duration, delay, s.Self.UID())
	return nil
}
```

- [ ] **Step 5.4: Wire into dispatch table**

Open `pkg/script/handlers.go`. Find the `OpInvDropItem: handleInvDropItem,` line. Add immediately below:

```go
	OpInvDropItemDelayed: handleInvDropItemDelayed,
```

- [ ] **Step 5.5: Run baseline tests to verify GREEN**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/ -run TestInvDropItemDelayed -v`
Expected: PASS — all 3 baseline tests green.

- [ ] **Step 5.6: Run pkg/script full to catch regressions**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/ -count=1`
Expected: PASS.

- [ ] **Step 5.7: Commit**

```bash
git add pkg/script/handlers_inv.go pkg/script/handlers.go pkg/script/handlers_inv_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(nai-134): T5 — handleInvDropItemDelayed core + dispatch (GREEN)

Per spec §5.1, §6.6, §6.7. Ports TS InvOps.ts:188-209 with full validator
chain (InvType → Coord → ObjType → ObjStack → Duration), operand-aware
protect gate (NAI-133 slot routing for PtrProtectedActivePlayer{,2}), and
TS-asymmetry vs INV_DROPITEM (NO ActiveObj/PtrActiveObj writeback at
enqueue, since the obj does not yet exist).

Tests cover happy path (full enqueue-arg pinning + DoesNotSetActiveObj
verification), no-active-player guard, completed=0 no-op. Validator-chain
+ protect-gate matrix lands in T6.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 6: Validator-chain tests + nil-World defensive

**Files:**
- Modify: `pkg/script/handlers_inv_test.go`

- [ ] **Step 6.1: Add validator-chain + nil-World tests**

Append to `pkg/script/handlers_inv_test.go`:

```go
// TestInvDropItemDelayed_BadInv_Errors pins InvTypeValid: invID=-1 fails.
func TestInvDropItemDelayed_BadInv_Errors(t *testing.T) {
	s, _, world := makeDropItemDelayedState(t, false, objtype.InvTypeScopeTemp, 5)

	pushDropItemDelayedArgs(s, -1, coordgrid.PackCoord(0, 3200, 3200), testObjCoin, 1, 100, 0)
	err := handleInvDropItemDelayed(s)

	if err == nil {
		t.Fatalf("expected error for invID=-1")
	}
	if !strings.Contains(err.Error(), "INV_DROPITEM_DELAYED") {
		t.Errorf("err: got %q, want INV_DROPITEM_DELAYED prefix", err)
	}
	if got := len(world.enqueueObjDelayedCalls); got != 0 {
		t.Errorf("error path: expected 0 enqueue, got %d", got)
	}
}

// TestInvDropItemDelayed_BadCoord_Errors pins CoordValid: coord=-1 fails.
func TestInvDropItemDelayed_BadCoord_Errors(t *testing.T) {
	s, _, _ := makeDropItemDelayedState(t, false, objtype.InvTypeScopeTemp, 5)

	pushDropItemDelayedArgs(s, testInvMain, -1, testObjCoin, 1, 100, 0)
	err := handleInvDropItemDelayed(s)

	if err == nil {
		t.Fatalf("expected error for coord=-1")
	}
	if !strings.Contains(err.Error(), "coord") {
		t.Errorf("err: got %q, want substring \"coord\"", err)
	}
}

// TestInvDropItemDelayed_BadObj_Errors pins ObjTypeValid: obj=-1 fails.
func TestInvDropItemDelayed_BadObj_Errors(t *testing.T) {
	s, _, _ := makeDropItemDelayedState(t, false, objtype.InvTypeScopeTemp, 5)

	pushDropItemDelayedArgs(s, testInvMain, coordgrid.PackCoord(0, 3200, 3200), -1, 1, 100, 0)
	err := handleInvDropItemDelayed(s)

	if err == nil {
		t.Fatalf("expected error for obj=-1")
	}
}

// TestInvDropItemDelayed_BadCount_Errors pins ObjStackValid: count=0 fails.
func TestInvDropItemDelayed_BadCount_Errors(t *testing.T) {
	s, _, _ := makeDropItemDelayedState(t, false, objtype.InvTypeScopeTemp, 5)

	pushDropItemDelayedArgs(s, testInvMain, coordgrid.PackCoord(0, 3200, 3200), testObjCoin, 0, 100, 0)
	err := handleInvDropItemDelayed(s)

	if err == nil {
		t.Fatalf("expected error for count=0")
	}
}

// TestInvDropItemDelayed_BadDuration_Errors pins DurationValid:
// duration=0 fails (DurationValid rejects <=0).
func TestInvDropItemDelayed_BadDuration_Errors(t *testing.T) {
	s, _, _ := makeDropItemDelayedState(t, false, objtype.InvTypeScopeTemp, 5)

	pushDropItemDelayedArgs(s, testInvMain, coordgrid.PackCoord(0, 3200, 3200), testObjCoin, 1, 0, 0)
	err := handleInvDropItemDelayed(s)

	if err == nil {
		t.Fatalf("expected error for duration=0")
	}
}

// TestInvDropItemDelayed_NilWorld_DefensiveError pins
// DEVIATION-NAI-130-D2 sibling: nil World surface returns a clean error
// rather than nil-deref. Only fires AFTER all validators + protect gate
// + Remove succeed.
func TestInvDropItemDelayed_NilWorld_DefensiveError(t *testing.T) {
	s, _, _ := makeDropItemDelayedState(t, false, objtype.InvTypeScopeTemp, 5)
	s.World = nil

	pushDropItemDelayedArgs(s, testInvMain, coordgrid.PackCoord(0, 3200, 3200), testObjCoin, 1, 100, 0)
	err := handleInvDropItemDelayed(s)

	if err == nil {
		t.Fatalf("nil World: expected error, got nil")
	}
	if !strings.Contains(err.Error(), "no world surface") {
		t.Errorf("err: got %q, want substring \"no world surface\"", err)
	}
}
```

- [ ] **Step 6.2: Run validator + nil-World tests to verify GREEN**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/ -run TestInvDropItemDelayed -v`
Expected: PASS — all baseline + 6 new tests green.

- [ ] **Step 6.3: Commit**

```bash
git add pkg/script/handlers_inv_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
test(nai-134): T6 — validator-chain + nil-World defensive coverage

Pins InvTypeValid / CoordValid / ObjTypeValid / ObjStackValid /
DurationValid each fire on the expected bad-arg input, and the
DEVIATION-NAI-130-D2 sibling nil-World guard returns a clean error
rather than nil-deref. All assertions check zero-enqueue side-effects
on the error paths.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 7: Protect-gate matrix + operand-aware coverage

**Files:**
- Modify: `pkg/script/handlers_inv_test.go`

- [ ] **Step 7.1: Add protect-gate + operand-aware tests**

Append to `pkg/script/handlers_inv_test.go`:

```go
// TestInvDropItemDelayed_ProtectGate_Operand0_Errors pins operand=0 +
// Protect=true + Scope!=Shared + no PtrProtectedActivePlayer → error.
func TestInvDropItemDelayed_ProtectGate_Operand0_Errors(t *testing.T) {
	s, _, world := makeDropItemDelayedState(t, true, objtype.InvTypeScopeTemp, 5)
	// Pointers does NOT include PtrProtectedActivePlayer.

	pushDropItemDelayedArgs(s, testInvMain, coordgrid.PackCoord(0, 3200, 3200), testObjCoin, 1, 100, 0)
	err := handleInvDropItemDelayed(s)

	if err == nil {
		t.Fatalf("expected protect-gate error")
	}
	if !strings.Contains(err.Error(), "protected access") {
		t.Errorf("err: got %q, want substring \"protected access\"", err)
	}
	if !strings.Contains(err.Error(), "test_inv") {
		t.Errorf("err: got %q, want substring \"test_inv\" (debugname)", err)
	}
	if got := len(world.enqueueObjDelayedCalls); got != 0 {
		t.Errorf("protect-error path: expected 0 enqueue, got %d", got)
	}
}

// TestInvDropItemDelayed_ProtectGate_Operand0_PassesWithFlag pins
// operand=0 + Protect=true + PtrProtectedActivePlayer set → success.
func TestInvDropItemDelayed_ProtectGate_Operand0_PassesWithFlag(t *testing.T) {
	s, _, world := makeDropItemDelayedState(t, true, objtype.InvTypeScopeTemp, 5)
	s.Pointers |= PtrProtectedActivePlayer

	pushDropItemDelayedArgs(s, testInvMain, coordgrid.PackCoord(0, 3200, 3200), testObjCoin, 1, 100, 0)
	err := handleInvDropItemDelayed(s)

	if err != nil {
		t.Fatalf("protect-flag set: unexpected error: %v", err)
	}
	if got := len(world.enqueueObjDelayedCalls); got != 1 {
		t.Errorf("protect-flag set: expected 1 enqueue, got %d", got)
	}
}

// TestInvDropItemDelayed_ProtectGate_Operand1_RequiresPtr2 pins NAI-133
// slot routing: operand=1 must check PtrProtectedActivePlayer2, not
// PtrProtectedActivePlayer.
//
// Sub-case A: only PtrProtectedActivePlayer set (not …2) → error.
// Sub-case B: PtrProtectedActivePlayer2 set → success.
func TestInvDropItemDelayed_ProtectGate_Operand1_RequiresPtr2(t *testing.T) {
	t.Run("PtrProtectedActivePlayer_only_errors", func(t *testing.T) {
		s, _, _ := makeDropItemDelayedState(t, true, objtype.InvTypeScopeTemp, 5)
		s.Script.IntOperands[s.PC] = 1
		s.Pointers |= PtrProtectedActivePlayer // wrong slot

		pushDropItemDelayedArgs(s, testInvMain, coordgrid.PackCoord(0, 3200, 3200), testObjCoin, 1, 100, 0)
		err := handleInvDropItemDelayed(s)

		if err == nil {
			t.Fatalf("operand=1 with only PtrProtectedActivePlayer (not …2): expected error")
		}
		if !strings.Contains(err.Error(), "protected access") {
			t.Errorf("err: got %q, want substring \"protected access\"", err)
		}
	})

	t.Run("PtrProtectedActivePlayer2_passes", func(t *testing.T) {
		s, _, world := makeDropItemDelayedState(t, true, objtype.InvTypeScopeTemp, 5)
		s.Script.IntOperands[s.PC] = 1
		s.Pointers |= PtrProtectedActivePlayer2

		pushDropItemDelayedArgs(s, testInvMain, coordgrid.PackCoord(0, 3200, 3200), testObjCoin, 1, 100, 0)
		err := handleInvDropItemDelayed(s)

		if err != nil {
			t.Fatalf("operand=1 with PtrProtectedActivePlayer2: unexpected error: %v", err)
		}
		if got := len(world.enqueueObjDelayedCalls); got != 1 {
			t.Errorf("expected 1 enqueue, got %d", got)
		}
	})
}

// TestInvDropItemDelayed_BadOperand_Errors pins operand=2 → "invalid
// intOperand". Mirrors handleBothMoveInv at handlers_inv.go:1230-1233.
func TestInvDropItemDelayed_BadOperand_Errors(t *testing.T) {
	s, _, _ := makeDropItemDelayedState(t, false, objtype.InvTypeScopeTemp, 5)
	s.Script.IntOperands[s.PC] = 2

	pushDropItemDelayedArgs(s, testInvMain, coordgrid.PackCoord(0, 3200, 3200), testObjCoin, 1, 100, 0)
	err := handleInvDropItemDelayed(s)

	if err == nil {
		t.Fatalf("operand=2: expected error")
	}
	if !strings.Contains(err.Error(), "invalid intOperand") {
		t.Errorf("err: got %q, want substring \"invalid intOperand\"", err)
	}
}

// TestInvDropItemDelayed_SharedScopeBypassesProtect pins TS InvOps.ts:197:
// when invType.Scope == InvTypeScopeShared, the protect gate is skipped
// even if invType.Protect=true and no PtrProtectedActivePlayer flag.
func TestInvDropItemDelayed_SharedScopeBypassesProtect(t *testing.T) {
	s, _, world := makeDropItemDelayedState(t, true, objtype.InvTypeScopeShared, 5)
	// Pointers does NOT include PtrProtectedActivePlayer.

	pushDropItemDelayedArgs(s, testInvMain, coordgrid.PackCoord(0, 3200, 3200), testObjCoin, 1, 100, 0)
	err := handleInvDropItemDelayed(s)

	if err != nil {
		t.Fatalf("Scope=Shared: unexpected error: %v", err)
	}
	if got := len(world.enqueueObjDelayedCalls); got != 1 {
		t.Errorf("Scope=Shared: expected 1 enqueue, got %d", got)
	}
}
```

- [ ] **Step 7.2: Run all NAI-134 tests to verify GREEN**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/ -run TestInvDropItemDelayed -v`
Expected: PASS — full 14-test matrix green.

- [ ] **Step 7.3: Run full repo test suite**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./... -count=1`
Expected: PASS — no regressions across pkg/* and modules/*.

- [ ] **Step 7.4: Run race detector**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race ./modules/world/ ./pkg/script/`
Expected: PASS — no races introduced. (Race-detector run scoped to modified packages — full-repo race run remains optional.)

- [ ] **Step 7.5: Commit**

```bash
git add pkg/script/handlers_inv_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
test(nai-134): T7 — protect-gate matrix + operand-aware coverage

Pins TS InvOps.ts:197 protect gate via operand-aware slot routing
(NAI-133 PtrProtectedActivePlayer{,2}):
  - operand=0 + no flag → error (debugname-bearing)
  - operand=0 + flag → success
  - operand=1 + only PtrProtectedActivePlayer (wrong slot) → error
  - operand=1 + PtrProtectedActivePlayer2 → success
  - operand=2 → "invalid intOperand"
  - Scope=Shared → bypasses protect gate even with Protect=true

Closes NAI-134. Memory-trailer follow-up below.

Closes memory:
  - true_to_ts_gate (handler 1:1 to TS InvOps.ts:188-209;
    DEVIATION-NAI-134-D1 = slice-vs-LinkList, behavior identical)
  - DEVIATION-NAI-115-D2 (sibling reuse — duration plumbed but discarded
    at drain; single-point retire at NAI-115-D2 closure)
  - DEVIATION-NAI-130-D2 (sibling reuse — defensive nil-World guard)

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Self-Review

Spec coverage check (each spec §):
- §1 Goal — covered by T1+T4+T5 production code; §2 anchors cited in commit messages.
- §3 Non-goals — explicitly preserved (no NAI-115-D2 / NAI-86 / smoke / other handlers).
- §4 Architecture — T1 (queue type), T2 (tick wiring), T3 (interface), T4 (worldVarsView), T5 (handler).
- §5.1 enqueue path — T5.3 handler body steps 1-9 mirrored.
- §5.2 drain path — T1.5 `processObjDelayedQueue`.
- §5.3 post-decrement semantics — T1.2 tests `DelayZeroFiresImmediately` + `FiresAfterDelayTicks`.
- §6.1 obj_delayed_queue.go — T1.5.
- §6.2 server.go field — T1.1.
- §6.3 worldVarsView.EnqueueObjDelayed — T4.1.
- §6.4 tick.go integration — T2.1.
- §6.5 WorldVars surface — T3.1.
- §6.6 handleInvDropItemDelayed — T5.3.
- §6.7 dispatch wire — T5.4.
- §7 error handling — T6 (validators + nil-World), T7 (protect-gate, bad-operand). T1.2 panic-recovery test.
- §8.1 queue tests — T1.2 (6 tests). `TestObjDelayedQueue_DurationStoredAtEnqueueDroppedAtDrain` codified.
- §8.2 handler tests — T5/T6/T7 (3+6+5 = 14 tests). All spec table rows mapped.
- §9 risk register — race+full-repo runs in T7.3-T7.4.
- §11 acceptance — last steps run `go test ./...`, `go test -race`, `go build ./...` (build implicit in passing test compile).

Placeholder scan: no TODO/TBD/"add appropriate" patterns; every step has either exact code or exact command + expected output.

Type consistency:
- `enqueueObjDelayedCall` field order verified consistent across T3.3 (struct decl) and T5.1 (recorder reads).
- `PtrProtectedActivePlayer{,2}` constants cross-checked against pointer.go:22-28.
- `s.Script.IntOperands[s.PC]` is `[]int32`; comparison against untyped 0/1/2 OK.
- `objDelayedRequest` field names (`obj`, `receiverID`, `duration`, `delay`) consistent across T1.5 (decl), T1.2 (tests), T1.4 (recoverObjDelayed reads).
- Spec §6.1 `recoverObjDelayed` body matches plan T1.4 verbatim.
