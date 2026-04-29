# NAI-50: OPOBJ Handlers Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Port OPOBJ1-5 + OPOBJT + OPOBJU client-game packet handlers and their AP/OP trigger dispatch so clicking a ground item routes to the correct script trigger.

**Architecture:** Four independent tasks: T1 adds `Server.GetObj` lookup + ObjType hidden-coercion; T2 adds packet decode handlers (handleOpObj/T/U); T3 adds the trigger-dispatch helpers and wires the `*entitypkg.Obj` case into both `tryFireOpTrigger` / `tryFireApTrigger`; T4 registers the seven opcodes and commits deviation doc-comments. T1 is a prerequisite for T2. T2 and T3 are independent; T4 follows both.

**Tech Stack:** Go 1.26+. No new dependencies. Test runner: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/... ./pkg/objtype/...`

---

## File Map

| File | Change |
|---|---|
| `pkg/objtype/objtype.go` | Add `"hidden"` → `""` coercion for Op slots (cases 30-34), matching LocType/NpcType decoders |
| `modules/world/obj_lookup.go` | New — `Server.GetObj(level, x, z, objId, receiverID int)` |
| `modules/world/obj_lookup_test.go` | New — unit tests for GetObj |
| `modules/world/handler_opobj.go` | New — `handleOpObj` (shared) + 5 shims + `handleOpObjT` + `handleOpObjU` |
| `modules/world/handler_opobj_test.go` | New — handler tests |
| `modules/world/interaction.go` | +2 sentinel consts: `targetOpObjT = 12`, `targetOpObjU = 13` |
| `modules/world/interaction_trigger.go` | +`apObjTriggerForOp` + `fireOpTriggerObj` + `fireApTriggerObj` + wire `*entitypkg.Obj` case arms |
| `modules/world/handler_opobj_test.go` | Trigger dispatch tests appended (same file) |
| `modules/world/handlers_game.go` | +7 `gameHandlers[N]` registrations |

---

## Task 1: GetObj lookup + ObjType hidden coercion

**Files:**
- Modify: `pkg/objtype/objtype.go` (cases 30-34 in the switch, around line 213-217)
- Create: `modules/world/obj_lookup.go`
- Create: `modules/world/obj_lookup_test.go`

### Step 1.1 — Write failing tests for GetObj

Create `modules/world/obj_lookup_test.go`:

```go
package world

import (
	"testing"

	entitypkg "github.com/zsrv/goscape/pkg/entity"
	"github.com/zsrv/goscape/pkg/zone"
)

func TestServerGetObjReturnsPublicObjWhenPresent(t *testing.T) {
	s := newTestServer(t)
	s.zoneMap = zone.NewZoneMap()
	obj := entitypkg.NewObj(0, 3200, 3200, entitypkg.LifecycleDespawn, 42, 1)
	// ReceiverID -1 == public
	z := s.zoneMap.Get(0, 3200, 3200)
	z.Objs = append(z.Objs, obj)

	got := s.GetObj(0, 3200, 3200, 42, 99)
	if got != obj {
		t.Errorf("GetObj: got %v, want obj", got)
	}
}

func TestServerGetObjReturnsPrivateObjForMatchingReceiver(t *testing.T) {
	s := newTestServer(t)
	s.zoneMap = zone.NewZoneMap()
	obj := entitypkg.NewObj(0, 3200, 3200, entitypkg.LifecycleDespawn, 42, 1)
	obj.ReceiverID = 5 // privately owned by player slot 5
	z := s.zoneMap.Get(0, 3200, 3200)
	z.Objs = append(z.Objs, obj)

	got := s.GetObj(0, 3200, 3200, 42, 5)
	if got != obj {
		t.Errorf("GetObj: got %v, want obj (matching receiver)", got)
	}
}

func TestServerGetObjRejectsPrivateObjForNonMatchingReceiver(t *testing.T) {
	s := newTestServer(t)
	s.zoneMap = zone.NewZoneMap()
	obj := entitypkg.NewObj(0, 3200, 3200, entitypkg.LifecycleDespawn, 42, 1)
	obj.ReceiverID = 5 // owned by slot 5
	z := s.zoneMap.Get(0, 3200, 3200)
	z.Objs = append(z.Objs, obj)

	got := s.GetObj(0, 3200, 3200, 42, 9) // different receiver
	if got != nil {
		t.Errorf("GetObj: got %v, want nil (receiver mismatch)", got)
	}
}

func TestServerGetObjReturnsNilWhenAbsent(t *testing.T) {
	s := newTestServer(t)
	s.zoneMap = zone.NewZoneMap()
	if got := s.GetObj(0, 3200, 3200, 42, -1); got != nil {
		t.Errorf("GetObj: got %v, want nil (empty zone)", got)
	}
}

func TestServerGetObjFiltersByTypeID(t *testing.T) {
	s := newTestServer(t)
	s.zoneMap = zone.NewZoneMap()
	obj := entitypkg.NewObj(0, 3200, 3200, entitypkg.LifecycleDespawn, 99, 1)
	z := s.zoneMap.Get(0, 3200, 3200)
	z.Objs = append(z.Objs, obj)

	got := s.GetObj(0, 3200, 3200, 42, -1) // looking for type 42, only 99 present
	if got != nil {
		t.Errorf("GetObj: got %v, want nil (wrong typeID)", got)
	}
}

func TestServerGetObjFiltersByCoords(t *testing.T) {
	s := newTestServer(t)
	s.zoneMap = zone.NewZoneMap()
	// obj at (3200, 3208) — same zone, different tile
	obj := entitypkg.NewObj(0, 3200, 3208, entitypkg.LifecycleDespawn, 42, 1)
	z := s.zoneMap.Get(0, 3200, 3208)
	z.Objs = append(z.Objs, obj)

	got := s.GetObj(0, 3200, 3200, 42, -1) // asking for (3200,3200)
	if got != nil {
		t.Errorf("GetObj: got %v, want nil (wrong tile)", got)
	}
}
```

- [ ] **Step 1.2 — Run tests to verify they fail**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/... -run "TestServerGetObj" -v 2>&1 | tail -20
```

Expected: FAIL — `s.GetObj undefined`

- [ ] **Step 1.3 — Add hidden→"" coercion to ObjType decoder**

In `pkg/objtype/objtype.go`, find cases 30-34 (around line 213-217). They currently read:

```go
case 30, 31, 32, 33, 34:
    if ot.Op == nil {
        ot.Op = make([]string, 5)
    }
    ot.Op[code-30] = dat.GJStrLF()
```

Change to:

```go
case 30, 31, 32, 33, 34:
    if ot.Op == nil {
        ot.Op = make([]string, 5)
    }
    ot.Op[code-30] = dat.GJStrLF()
    if ot.Op[code-30] == "hidden" {
        ot.Op[code-30] = ""
    }
```

- [ ] **Step 1.4 — Implement Server.GetObj**

Create `modules/world/obj_lookup.go`:

```go
package world

import (
	entitypkg "github.com/zsrv/goscape/pkg/entity"
	"github.com/zsrv/goscape/pkg/zone"
)

// GetObj returns the first obj at (level, x, z) whose type matches objId and
// is visible to receiverID, or nil. Mirrors TS World.getObj / Zone.getObj
// (Zone.ts:353-360): matches public objs (ReceiverID == zone.PublicReceiver)
// OR objs privately owned by this player (ReceiverID == receiverID).
// Callers pass p.slot as receiverID.
func (s *Server) GetObj(level, x, z, objId, receiverID int) *entitypkg.Obj {
	zn := s.zoneMap.Get(level, x, z)
	for _, o := range zn.Objs {
		if o.X == x && o.Z == z && o.Type == objId &&
			(o.ReceiverID == zone.PublicReceiver || o.ReceiverID == receiverID) {
			return o
		}
	}
	return nil
}
```

- [ ] **Step 1.5 — Run tests to verify they pass**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/... ./pkg/objtype/... -run "TestServerGetObj" -v 2>&1 | tail -20
```

Expected: all 6 PASS.

- [ ] **Step 1.6 — Full suite green check**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/... ./pkg/objtype/... -count=1 2>&1 | tail -5
```

Expected: ok, no failures.

- [ ] **Step 1.7 — Commit**

```bash
git add pkg/objtype/objtype.go modules/world/obj_lookup.go modules/world/obj_lookup_test.go
git commit --no-gpg-sign -m "feat(world): NAI-50 T1 — Server.GetObj + ObjType hidden-coercion"
```

---

## Task 2: OPOBJ1-5 + OPOBJT + OPOBJU packet handlers

**Files:**
- Create: `modules/world/handler_opobj.go`
- Create: `modules/world/handler_opobj_test.go`

### Step 2.1 — Write failing tests

Create `modules/world/handler_opobj_test.go`:

```go
package world

import (
	"net"
	"testing"

	entitypkg "github.com/zsrv/goscape/pkg/entity"
	"github.com/zsrv/goscape/pkg/inventory"
	io2 "github.com/zsrv/goscape/pkg/io/isaac"
	"github.com/zsrv/goscape/pkg/objtype"
	"github.com/zsrv/goscape/pkg/zone"
)

// makeOpObjFixture creates a server + player + obj adjacent to the player,
// with an ObjType registered, ready for handleOpObj tests.
// Player at (99, 100, 0); obj at (100, 100, 0) — Chebyshev=1.
// Player originX/originZ = (100, 100) so viewport gate accepts coords
// within [-52, +52] of (100, 100).
// ObjType 42, Op = ["op1","op2","op3","op4","op5"].
// Returns (server, player, obj, clientConn).
func makeOpObjFixture(t *testing.T) (*Server, *Player, *entitypkg.Obj, net.Conn) {
	t.Helper()
	s := newTestServer(t)
	s.zoneMap = zone.NewZoneMap()

	s.objTypes = &objtype.ObjTypeConfigs{
		Configs: make([]*objtype.ObjType, 43),
	}
	s.objTypes.Configs[42] = &objtype.ObjType{
		ConfigType: objtype.ConfigType{ID: 42, DebugName: "test_obj"},
		Op:         []string{"op1", "op2", "op3", "op4", "op5"},
	}

	obj := entitypkg.NewObj(0, 100, 100, entitypkg.LifecycleDespawn, 42, 1)
	zn := s.zoneMap.Get(0, 100, 100)
	zn.Objs = append(zn.Objs, obj)

	p, cc := newTestPlayer(t)
	p.client.server = s
	p.client.encryptor = io2.New([4]uint32{1, 2, 3, 4})
	p.x, p.z, p.level = 99, 100, 0
	p.originX, p.originZ = 100, 100

	return s, p, obj, cc
}

// p2x3ObjPayload encodes (x: u16, z: u16, objId: u16) into 6 bytes big-endian.
func p2x3ObjPayload(x, z, objId int) []byte {
	return []byte{
		byte(x >> 8), byte(x),
		byte(z >> 8), byte(z),
		byte(objId >> 8), byte(objId),
	}
}

// p2x4ObjPayload encodes (x: u16, z: u16, objId: u16, com: u16) into 8 bytes.
func p2x4ObjPayload(x, z, objId, com int) []byte {
	return []byte{
		byte(x >> 8), byte(x),
		byte(z >> 8), byte(z),
		byte(objId >> 8), byte(objId),
		byte(com >> 8), byte(com),
	}
}

// p2x6ObjPayload encodes (x, z, objId, useObj, useSlot, useCom) into 12 bytes.
func p2x6ObjPayload(x, z, objId, useObj, useSlot, useCom int) []byte {
	return []byte{
		byte(x >> 8), byte(x),
		byte(z >> 8), byte(z),
		byte(objId >> 8), byte(objId),
		byte(useObj >> 8), byte(useObj),
		byte(useSlot >> 8), byte(useSlot),
		byte(useCom >> 8), byte(useCom),
	}
}

// --- handleOpObj (OPOBJ1-5) ---

func TestHandleOpObj1SetsInteraction(t *testing.T) {
	_, p, obj, _ := makeOpObjFixture(t)

	if err := handleOpObj1(p, p2x3ObjPayload(100, 100, 42)); err != nil {
		t.Fatalf("handleOpObj1: %v", err)
	}

	if p.target != obj {
		t.Errorf("target: got %v, want obj", p.target)
	}
	if p.targetOp != 1 {
		t.Errorf("targetOp: got %d, want 1", p.targetOp)
	}
	if p.interactionKind != InteractionEngine {
		t.Errorf("interactionKind: got %v, want InteractionEngine", p.interactionKind)
	}
	if !p.opcalled {
		t.Error("opcalled: want true")
	}
	if p.targetSubject.typ != 42 {
		t.Errorf("targetSubject.typ: got %d, want 42", p.targetSubject.typ)
	}
	if p.targetSubject.x != 100 || p.targetSubject.z != 100 || p.targetSubject.level != 0 {
		t.Errorf("targetSubject coords: got (%d,%d,%d), want (100,100,0)",
			p.targetSubject.x, p.targetSubject.z, p.targetSubject.level)
	}
}

func TestHandleOpObjAllFiveOpsRouteIndependently(t *testing.T) {
	type opCase struct {
		op int
		fn func(*Player, []byte) error
	}
	cases := []opCase{
		{1, handleOpObj1}, {2, handleOpObj2}, {3, handleOpObj3},
		{4, handleOpObj4}, {5, handleOpObj5},
	}
	for _, c := range cases {
		t.Run("op"+string(rune('0'+c.op)), func(t *testing.T) {
			_, p, _, _ := makeOpObjFixture(t)
			if err := c.fn(p, p2x3ObjPayload(100, 100, 42)); err != nil {
				t.Fatalf("op%d: %v", c.op, err)
			}
			if p.targetOp != c.op {
				t.Errorf("targetOp: got %d, want %d", p.targetOp, c.op)
			}
		})
	}
}

func TestHandleOpObjDelayedPlayerRejected(t *testing.T) {
	s, p, _, cc := makeOpObjFixture(t)
	p.delayed = true
	p.delayedUntil = 999
	s.currentTick = 0

	received := drainConn(t, cc)
	_ = handleOpObj1(p, p2x3ObjPayload(100, 100, 42))
	p.client.flushWrite()
	got := <-received

	if len(got) == 0 {
		t.Fatal("expected UnsetMapFlag for delayed player")
	}
	if p.target != nil {
		t.Error("target should remain nil for delayed player")
	}
}

func TestHandleOpObjShortPayloadRejected(t *testing.T) {
	_, p, _, cc := makeOpObjFixture(t)

	received := drainConn(t, cc)
	_ = handleOpObj1(p, []byte{0x00, 0x64, 0x00}) // 3 bytes
	p.client.flushWrite()
	got := <-received

	if len(got) == 0 {
		t.Fatal("expected UnsetMapFlag for short payload")
	}
	if p.target != nil {
		t.Error("target should remain nil for short payload")
	}
}

func TestHandleOpObjOutOfViewportRejected(t *testing.T) {
	_, p, _, cc := makeOpObjFixture(t)

	received := drainConn(t, cc)
	_ = handleOpObj1(p, p2x3ObjPayload(250, 100, 42)) // dx = 150 > 52
	p.client.flushWrite()
	got := <-received

	if len(got) == 0 {
		t.Fatal("expected UnsetMapFlag for out-of-viewport click")
	}
	if p.target != nil {
		t.Error("target should remain nil for out-of-viewport click")
	}
}

func TestHandleOpObjMissingObjRejected(t *testing.T) {
	_, p, _, cc := makeOpObjFixture(t)

	received := drainConn(t, cc)
	_ = handleOpObj1(p, p2x3ObjPayload(100, 100, 999)) // wrong objId
	p.client.flushWrite()
	got := <-received

	if len(got) == 0 {
		t.Fatal("expected UnsetMapFlag for missing obj")
	}
	if p.target != nil {
		t.Error("target should remain nil for missing obj")
	}
}

func TestHandleOpObjMissingObjTypeRejected(t *testing.T) {
	s, p, _, cc := makeOpObjFixture(t)
	// Place an obj with typeID 77 but no registered ObjType.
	extra := entitypkg.NewObj(0, 100, 100, entitypkg.LifecycleDespawn, 77, 1)
	s.zoneMap.Get(0, 100, 100).Objs = append(s.zoneMap.Get(0, 100, 100).Objs, extra)

	received := drainConn(t, cc)
	_ = handleOpObj1(p, p2x3ObjPayload(100, 100, 77)) // ObjType 77 not registered
	p.client.flushWrite()
	got := <-received

	if len(got) == 0 {
		t.Fatal("expected UnsetMapFlag for missing ObjType")
	}
	if p.target != nil {
		t.Error("target should remain nil for missing ObjType")
	}
}

func TestHandleOpObjRejectsEmptyOpSlot(t *testing.T) {
	s, p, _, cc := makeOpObjFixture(t)
	s.objTypes.Configs[42].Op[0] = "" // op=1 slot cleared

	received := drainConn(t, cc)
	_ = handleOpObj1(p, p2x3ObjPayload(100, 100, 42))
	p.client.flushWrite()
	got := <-received

	if len(got) == 0 {
		t.Fatal("expected UnsetMapFlag for empty Op slot")
	}
	if p.target != nil {
		t.Error("target should remain nil when Op slot is empty")
	}
}

// --- handleOpObjT ---

func TestHandleOpObjTSetsInteraction(t *testing.T) {
	_, p, obj, _ := makeOpObjFixture(t)

	if err := handleOpObjT(p, p2x4ObjPayload(100, 100, 42, 7777)); err != nil {
		t.Fatalf("handleOpObjT: %v", err)
	}

	if p.target != obj {
		t.Errorf("target: got %v, want obj", p.target)
	}
	if p.targetOp != targetOpObjT {
		t.Errorf("targetOp: got %d, want targetOpObjT (%d)", p.targetOp, targetOpObjT)
	}
	if p.targetSubject.com != 7777 {
		t.Errorf("targetSubject.com: got %d, want 7777 (spellCom)", p.targetSubject.com)
	}
	if p.targetSubject.typ != 42 || p.targetSubject.x != 100 || p.targetSubject.z != 100 {
		t.Errorf("targetSubject snapshot: got (typ=%d,x=%d,z=%d), want (42,100,100)",
			p.targetSubject.typ, p.targetSubject.x, p.targetSubject.z)
	}
	if !p.opcalled {
		t.Error("opcalled: want true")
	}
}

func TestHandleOpObjTDelayedPlayerRejected(t *testing.T) {
	s, p, _, cc := makeOpObjFixture(t)
	p.delayed = true
	p.delayedUntil = 999
	s.currentTick = 0

	received := drainConn(t, cc)
	_ = handleOpObjT(p, p2x4ObjPayload(100, 100, 42, 7777))
	p.client.flushWrite()
	got := <-received

	if len(got) == 0 {
		t.Fatal("expected UnsetMapFlag for delayed player")
	}
	if p.target != nil {
		t.Error("target should remain nil for delayed player")
	}
}

func TestHandleOpObjTShortPayloadRejected(t *testing.T) {
	_, p, _, cc := makeOpObjFixture(t)

	received := drainConn(t, cc)
	_ = handleOpObjT(p, []byte{0x00, 0x64, 0x00, 0x64}) // 4 bytes, need 8
	p.client.flushWrite()
	got := <-received

	if len(got) == 0 {
		t.Fatal("expected UnsetMapFlag for short payload")
	}
	if p.target != nil {
		t.Error("target should remain nil for short payload")
	}
}

func TestHandleOpObjTOutOfViewportRejected(t *testing.T) {
	_, p, _, cc := makeOpObjFixture(t)

	received := drainConn(t, cc)
	_ = handleOpObjT(p, p2x4ObjPayload(250, 100, 42, 7777)) // dx = 150
	p.client.flushWrite()
	got := <-received

	if len(got) == 0 {
		t.Fatal("expected UnsetMapFlag for out-of-viewport")
	}
	if p.target != nil {
		t.Error("target should remain nil for out-of-viewport")
	}
}

func TestHandleOpObjTMissingObjRejected(t *testing.T) {
	_, p, _, cc := makeOpObjFixture(t)

	received := drainConn(t, cc)
	_ = handleOpObjT(p, p2x4ObjPayload(100, 100, 999, 7777))
	p.client.flushWrite()
	got := <-received

	if len(got) == 0 {
		t.Fatal("expected UnsetMapFlag for missing obj")
	}
	if p.target != nil {
		t.Error("target should remain nil for missing obj")
	}
}

// --- handleOpObjU ---

func TestHandleOpObjUSetsInteraction(t *testing.T) {
	s, p, obj, _ := makeOpObjFixture(t)

	if s.invs == nil {
		s.invs = make(map[int]*inventory.Inventory)
	}
	inv := inventory.New(93, 28, inventory.StackNormal)
	inv.Items[3] = &inventory.Item{Id: 1511, Count: 1}
	s.invs[93] = inv
	p.invListenOnCom(93, 149, -1)

	if err := handleOpObjU(p, p2x6ObjPayload(100, 100, 42, 1511, 3, 149)); err != nil {
		t.Fatalf("handleOpObjU: %v", err)
	}

	if p.target != obj {
		t.Errorf("target: got %v, want obj", p.target)
	}
	if p.targetOp != targetOpObjU {
		t.Errorf("targetOp: got %d, want targetOpObjU (%d)", p.targetOp, targetOpObjU)
	}
	if p.lastUseItem != 1511 {
		t.Errorf("lastUseItem: got %d, want 1511", p.lastUseItem)
	}
	if p.lastUseSlot != 3 {
		t.Errorf("lastUseSlot: got %d, want 3", p.lastUseSlot)
	}
	if p.targetSubject.com != -1 {
		t.Errorf("targetSubject.com: got %d, want -1 (OPOBJU passes -1)", p.targetSubject.com)
	}
	if !p.opcalled {
		t.Error("opcalled: want true")
	}
}

func TestHandleOpObjUDelayedPlayerRejected(t *testing.T) {
	s, p, _, cc := makeOpObjFixture(t)
	p.delayed = true
	p.delayedUntil = 999
	s.currentTick = 0
	p.lastUseItem = 42 // sentinel: must stay unchanged

	received := drainConn(t, cc)
	_ = handleOpObjU(p, p2x6ObjPayload(100, 100, 42, 1511, 3, 149))
	p.client.flushWrite()
	got := <-received

	if len(got) == 0 {
		t.Fatal("expected UnsetMapFlag for delayed player")
	}
	if p.target != nil {
		t.Error("target should remain nil for delayed player")
	}
	if p.lastUseItem != 42 {
		t.Errorf("lastUseItem leaked through rejected handler: got %d, want 42", p.lastUseItem)
	}
}

func TestHandleOpObjUShortPayloadRejected(t *testing.T) {
	_, p, _, cc := makeOpObjFixture(t)

	received := drainConn(t, cc)
	_ = handleOpObjU(p, []byte{0x00, 0x64, 0x00, 0x64, 0x00, 0x2a, 0x05, 0xe7}) // 8 bytes
	p.client.flushWrite()
	got := <-received

	if len(got) == 0 {
		t.Fatal("expected UnsetMapFlag for short payload")
	}
	if p.target != nil {
		t.Error("target should remain nil for short payload")
	}
}

func TestHandleOpObjUOutOfViewportRejected(t *testing.T) {
	_, p, _, cc := makeOpObjFixture(t)

	received := drainConn(t, cc)
	_ = handleOpObjU(p, p2x6ObjPayload(250, 100, 42, 1511, 3, 149)) // dx=150
	p.client.flushWrite()
	got := <-received

	if len(got) == 0 {
		t.Fatal("expected UnsetMapFlag for out-of-viewport")
	}
	if p.target != nil {
		t.Error("target should remain nil for out-of-viewport")
	}
}

func TestHandleOpObjUMissingObjRejected(t *testing.T) {
	_, p, _, cc := makeOpObjFixture(t)

	received := drainConn(t, cc)
	_ = handleOpObjU(p, p2x6ObjPayload(100, 100, 999, 1511, 3, 149))
	p.client.flushWrite()
	got := <-received

	if len(got) == 0 {
		t.Fatal("expected UnsetMapFlag for missing obj")
	}
	if p.target != nil {
		t.Error("target should remain nil for missing obj")
	}
}

func TestHandleOpObjUMissingListenerRejected(t *testing.T) {
	s, p, _, cc := makeOpObjFixture(t)
	if s.invs == nil {
		s.invs = make(map[int]*inventory.Inventory)
	}
	s.invs[93] = inventory.New(93, 28, inventory.StackNormal)
	// No invListenOnCom call — listener absent.
	p.lastUseItem = 77 // sentinel

	received := drainConn(t, cc)
	_ = handleOpObjU(p, p2x6ObjPayload(100, 100, 42, 1511, 3, 149))
	p.client.flushWrite()
	got := <-received

	if len(got) == 0 {
		t.Fatal("expected UnsetMapFlag for missing listener")
	}
	if p.target != nil {
		t.Error("target should remain nil for missing listener")
	}
	if p.lastUseItem != 77 {
		t.Errorf("lastUseItem leaked: got %d, want 77", p.lastUseItem)
	}
}

func TestHandleOpObjUItemMismatchRejected(t *testing.T) {
	s, p, _, cc := makeOpObjFixture(t)
	if s.invs == nil {
		s.invs = make(map[int]*inventory.Inventory)
	}
	inv := inventory.New(93, 28, inventory.StackNormal)
	inv.Items[3] = &inventory.Item{Id: 9999, Count: 1} // NOT 1511
	s.invs[93] = inv
	p.invListenOnCom(93, 149, -1)
	p.lastUseItem = 77 // sentinel

	received := drainConn(t, cc)
	_ = handleOpObjU(p, p2x6ObjPayload(100, 100, 42, 1511, 3, 149)) // claims 1511
	p.client.flushWrite()
	got := <-received

	if len(got) == 0 {
		t.Fatal("expected UnsetMapFlag for item mismatch")
	}
	if p.target != nil {
		t.Error("target should remain nil for item mismatch")
	}
	if p.lastUseItem != 77 {
		t.Errorf("lastUseItem leaked: got %d, want 77", p.lastUseItem)
	}
}
```

- [ ] **Step 2.2 — Run tests to verify they fail**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/... -run "TestHandleOpObj" -v 2>&1 | tail -10
```

Expected: FAIL — `handleOpObj1 undefined`

- [ ] **Step 2.3 — Implement handler_opobj.go**

Create `modules/world/handler_opobj.go`:

```go
package world

import (
	"github.com/zsrv/goscape/pkg/io/packet"
)

// handleOpObj is the shared implementation for OPOBJ1..OPOBJ5.
// op is 1..5. Payload = 6 bytes: (x: G2, z: G2, objId: G2).
//
// Validation gates (mirrors TS OpObjHandler.ts:14-42):
//  1. nil client/server guard
//  2. p.delayed → UnsetMapFlag
//  3. payload < 6 bytes → UnsetMapFlag
//  4. viewport gate: |x-originX| > 52 || |z-originZ| > 52 → UnsetMapFlag
//  5. Server.GetObj returns nil → UnsetMapFlag
//  6. ObjType not registered → UnsetMapFlag
//  7. per-op gate: Op[op-1] == "" → UnsetMapFlag
//
// On success: ClearPendingAction → opcalled=true →
// SetInteraction(Engine, obj, op, -1) → targetSubject snapshot.
func handleOpObj(p *Player, payload []byte, op int) error {
	if p.client == nil || p.client.server == nil {
		return nil
	}
	s := p.client.server

	if p.delayed && s.currentTick < p.delayedUntil {
		sendUnsetMapFlag(p)
		return nil
	}

	if len(payload) < 6 {
		sendUnsetMapFlag(p)
		return nil
	}

	r := packet.NewPacket(payload)
	x := int(r.G2())
	z := int(r.G2())
	objId := int(r.G2())

	dx := x - p.originX
	if dx < 0 {
		dx = -dx
	}
	dz := z - p.originZ
	if dz < 0 {
		dz = -dz
	}
	if dx > 52 || dz > 52 {
		sendUnsetMapFlag(p)
		return nil
	}

	obj := s.GetObj(p.level, x, z, objId, p.slot)
	if obj == nil {
		sendUnsetMapFlag(p)
		return nil
	}

	if s.objTypes == nil || objId < 0 || objId >= len(s.objTypes.Configs) {
		sendUnsetMapFlag(p)
		return nil
	}
	objType := s.objTypes.Configs[objId]
	if objType == nil {
		sendUnsetMapFlag(p)
		return nil
	}
	if len(objType.Op) < op || objType.Op[op-1] == "" {
		sendUnsetMapFlag(p)
		return nil
	}

	p.ClearPendingAction()
	p.opcalled = true
	p.SetInteraction(InteractionEngine, obj, op, -1)
	p.targetSubject.typ = obj.Type
	p.targetSubject.x = obj.X
	p.targetSubject.z = obj.Z
	p.targetSubject.level = obj.Level
	return nil
}

func handleOpObj1(p *Player, payload []byte) error { return handleOpObj(p, payload, 1) }
func handleOpObj2(p *Player, payload []byte) error { return handleOpObj(p, payload, 2) }
func handleOpObj3(p *Player, payload []byte) error { return handleOpObj(p, payload, 3) }
func handleOpObj4(p *Player, payload []byte) error { return handleOpObj(p, payload, 4) }
func handleOpObj5(p *Player, payload []byte) error { return handleOpObj(p, payload, 5) }

// handleOpObjT is the handler for OPOBJT (opcode 138, 8-byte payload).
// Spell-on-obj: player casts a spell onto a ground item.
// Payload = (x:G2, z:G2, objId:G2, spellCom:G2).
//
// DEVIATION NAI-50-D1: TS OpObjTHandler.ts:20-29 validates spellCom
// references a component with ComActionTarget.OBJ AND that the component
// is visible. Skipped — no component registry. Same cluster as S6m-D1,
// NAI-45-D1, NAI-48-D1. Closure: component-registry sub-spec.
func handleOpObjT(p *Player, payload []byte) error {
	if p.client == nil || p.client.server == nil {
		return nil
	}
	s := p.client.server

	if p.delayed && s.currentTick < p.delayedUntil {
		sendUnsetMapFlag(p)
		return nil
	}

	if len(payload) < 8 {
		sendUnsetMapFlag(p)
		return nil
	}

	r := packet.NewPacket(payload)
	x := int(r.G2())
	z := int(r.G2())
	objId := int(r.G2())
	spellCom := int(r.G2())

	dx := x - p.originX
	if dx < 0 {
		dx = -dx
	}
	dz := z - p.originZ
	if dz < 0 {
		dz = -dz
	}
	if dx > 52 || dz > 52 {
		sendUnsetMapFlag(p)
		return nil
	}

	obj := s.GetObj(p.level, x, z, objId, p.slot)
	if obj == nil {
		sendUnsetMapFlag(p)
		return nil
	}

	if s.objTypes == nil || objId < 0 || objId >= len(s.objTypes.Configs) || s.objTypes.Configs[objId] == nil {
		sendUnsetMapFlag(p)
		return nil
	}

	p.ClearPendingAction()
	p.opcalled = true
	p.SetInteraction(InteractionEngine, obj, targetOpObjT, spellCom)
	p.targetSubject.typ = obj.Type
	p.targetSubject.x = obj.X
	p.targetSubject.z = obj.Z
	p.targetSubject.level = obj.Level
	return nil
}

// handleOpObjU is the handler for OPOBJU (opcode 239, 12-byte payload).
// Item-on-obj: player drags an inventory item onto a ground item.
// Payload = (x:G2, z:G2, objId:G2, useObj:G2, useSlot:G2, useCom:G2).
//
// DEVIATION NAI-50-D2: TS OpObjUHandler.ts:39-48 validates useCom
// references a usable, visible component. Skipped — no component registry.
// Same cluster as S6m-D2, NAI-45-D2, NAI-48-D1. Closure: component-registry sub-spec.
func handleOpObjU(p *Player, payload []byte) error {
	if p.client == nil || p.client.server == nil {
		return nil
	}
	s := p.client.server

	if p.delayed && s.currentTick < p.delayedUntil {
		sendUnsetMapFlag(p)
		return nil
	}

	if len(payload) < 12 {
		sendUnsetMapFlag(p)
		return nil
	}

	r := packet.NewPacket(payload)
	x := int(r.G2())
	z := int(r.G2())
	objId := int(r.G2())
	useObj := int(r.G2())
	useSlot := int(r.G2())
	useCom := int(r.G2())

	dx := x - p.originX
	if dx < 0 {
		dx = -dx
	}
	dz := z - p.originZ
	if dz < 0 {
		dz = -dz
	}
	if dx > 52 || dz > 52 {
		sendUnsetMapFlag(p)
		return nil
	}

	obj := s.GetObj(p.level, x, z, objId, p.slot)
	if obj == nil {
		sendUnsetMapFlag(p)
		return nil
	}

	if s.objTypes == nil || objId < 0 || objId >= len(s.objTypes.Configs) || s.objTypes.Configs[objId] == nil {
		sendUnsetMapFlag(p)
		return nil
	}

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

	if s.objTypes != nil && useObj >= 0 && useObj < len(s.objTypes.Configs) {
		if useObjType := s.objTypes.Configs[useObj]; useObjType != nil && useObjType.Members && !s.cfg.NodeMembers {
			p.MessageGame("To use this item please login to a members' server.")
			sendUnsetMapFlag(p)
			return nil
		}
	}

	p.lastUseItem = useObj
	p.lastUseSlot = useSlot

	p.ClearPendingAction()
	p.opcalled = true
	p.SetInteraction(InteractionEngine, obj, targetOpObjU, -1)
	p.targetSubject.typ = obj.Type
	p.targetSubject.x = obj.X
	p.targetSubject.z = obj.Z
	p.targetSubject.level = obj.Level
	return nil
}
```

- [ ] **Step 2.4 — Run tests to verify they pass**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/... -run "TestHandleOpObj" -v 2>&1 | tail -30
```

Expected: all PASS. If `targetOpObjT`/`targetOpObjU` are undefined, proceed to Task 3 Step 3.1 first (add constants), then re-run.

- [ ] **Step 2.5 — Full suite green check**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/... ./pkg/objtype/... -count=1 2>&1 | tail -5
```

Expected: ok, no failures.

- [ ] **Step 2.6 — Commit**

```bash
git add modules/world/handler_opobj.go modules/world/handler_opobj_test.go
git commit --no-gpg-sign -m "feat(world): NAI-50 T2 — handleOpObj/T/U packet handlers"
```

---

## Task 3: targetOp sentinels + trigger dispatch (fireOpTriggerObj, fireApTriggerObj)

**Files:**
- Modify: `modules/world/interaction.go` (sentinel const block)
- Modify: `modules/world/interaction_trigger.go` (add helpers + wire case arms)
- Append to: `modules/world/handler_opobj_test.go` (trigger dispatch tests)

### Step 3.1 — Write failing trigger dispatch tests

Append to `modules/world/handler_opobj_test.go` (after the existing tests):

```go
// --- Trigger dispatch tests (fireOpTriggerObj / fireApTriggerObj) ---

// makeOpObjTriggerFixture creates a fixture for tryFireOpTrigger Obj-branch
// tests: server + player anchored on an obj with valid targetSubject,
// positioned at contact distance (player at (99,100), obj at (100,100)).
func makeOpObjTriggerFixture(t *testing.T) (*Server, *Player, *entitypkg.Obj, net.Conn) {
	t.Helper()
	s, p, obj, cc := makeOpObjFixture(t)
	s.scriptProvider = script.NewProvider()
	p.SetInteraction(InteractionEngine, obj, 1, -1)
	p.targetSubject.typ = obj.Type
	p.targetSubject.x = obj.X
	p.targetSubject.z = obj.Z
	p.targetSubject.level = obj.Level
	return s, p, obj, cc
}

// TestTryFireOpTriggerObjNoScript verifies a *Obj target with no registered
// trigger emits "Nothing interesting happens." and clears the interaction.
func TestTryFireOpTriggerObjNoScript(t *testing.T) {
	_, p, _, cc := makeOpObjTriggerFixture(t)

	received := drainConn(t, cc)
	tryFireOpTrigger(p)
	p.client.flushWrite()
	got := <-received

	if len(got) == 0 {
		t.Fatal("expected MessageGame packet for default-op, got nothing")
	}
	if p.target != nil {
		t.Errorf("target: got %v, want nil after default-op clear", p.target)
	}
	if !p.interactionFired {
		t.Error("interactionFired: want true after default-op clear")
	}
	if !bytes.Contains(got, []byte("Nothing interesting happens.")) {
		t.Errorf("expected \"Nothing interesting happens.\" in drained bytes, got %x", got)
	}
}

// TestTryFireOpTriggerObjScriptFires verifies a registered [opobj1,<typeID>]
// script fires, ActiveObj is set, and ClearInteraction runs after Finished.
func TestTryFireOpTriggerObjScriptFires(t *testing.T) {
	s, p, obj, _ := makeOpObjTriggerFixture(t)

	sf := newNoopScriptFile(t, script.TriggerOpObj1, obj.Type, -1)
	s.scriptProvider.Register(sf)

	tryFireOpTrigger(p)

	if p.target != nil {
		t.Errorf("target: got %v, want nil after Finished", p.target)
	}
	if !p.interactionFired {
		t.Error("interactionFired: want true after script fire")
	}
}

// TestTryFireOpTriggerObjDeferredOnDelay verifies delayed player defers fire.
func TestTryFireOpTriggerObjDeferredOnDelay(t *testing.T) {
	s, p, obj, _ := makeOpObjTriggerFixture(t)
	p.delayed = true
	p.delayedUntil = 999
	s.currentTick = 0

	tryFireOpTrigger(p)

	if p.target != obj {
		t.Errorf("target: got %v, want obj (deferred)", p.target)
	}
	if p.interactionFired {
		t.Error("interactionFired: want false (deferred)")
	}
}

// TestTryFireOpTriggerObjRemoved verifies removing the obj from its zone
// clears interaction silently.
func TestTryFireOpTriggerObjRemoved(t *testing.T) {
	s, p, _, _ := makeOpObjTriggerFixture(t)

	// Remove all objs from the zone.
	zn := s.zoneMap.Get(0, 100, 100)
	zn.Objs = nil

	tryFireOpTrigger(p)

	if p.target != nil {
		t.Errorf("target: got %v, want nil (obj removed)", p.target)
	}
	if !p.interactionFired {
		t.Error("interactionFired: want true after removal clear")
	}
}

// TestTryFireOpTriggerObjFiresObjTTrigger verifies targetOpObjT → OPOBJT dispatch.
func TestTryFireOpTriggerObjFiresObjTTrigger(t *testing.T) {
	s, p, obj, _ := makeOpObjFixture(t)
	s.scriptProvider = script.NewProvider()
	p.SetInteraction(InteractionEngine, obj, targetOpObjT, 7777)
	p.targetSubject.typ = obj.Type
	p.targetSubject.x = obj.X
	p.targetSubject.z = obj.Z
	p.targetSubject.level = obj.Level

	sf := newNoopScriptFile(t, script.TriggerOpObjT, obj.Type, -1)
	s.scriptProvider.Register(sf)

	tryFireOpTrigger(p)

	if p.target != nil {
		t.Errorf("target: got %v, want nil after Finished", p.target)
	}
	if !p.interactionFired {
		t.Error("interactionFired: want true after OPOBJT fire")
	}
}

// TestTryFireOpTriggerObjFiresObjUTrigger verifies targetOpObjU → OPOBJU dispatch.
func TestTryFireOpTriggerObjFiresObjUTrigger(t *testing.T) {
	s, p, obj, _ := makeOpObjFixture(t)
	s.scriptProvider = script.NewProvider()
	p.SetInteraction(InteractionEngine, obj, targetOpObjU, -1)
	p.targetSubject.typ = obj.Type
	p.targetSubject.x = obj.X
	p.targetSubject.z = obj.Z
	p.targetSubject.level = obj.Level

	sf := newNoopScriptFile(t, script.TriggerOpObjU, obj.Type, -1)
	s.scriptProvider.Register(sf)

	tryFireOpTrigger(p)

	if p.target != nil {
		t.Errorf("target: got %v, want nil after Finished", p.target)
	}
	if !p.interactionFired {
		t.Error("interactionFired: want true after OPOBJU fire")
	}
}

// makeApObjTriggerFixture creates a fixture for tryFireApTrigger Obj-branch tests:
// player at (95, 100) — 5 tiles from obj at (100, 100), within apRange=10.
func makeApObjTriggerFixture(t *testing.T) (*Server, *Player, *entitypkg.Obj, net.Conn) {
	t.Helper()
	s, p, obj, cc := makeOpObjTriggerFixture(t)
	p.x, p.z = 95, 100 // move out of contact, within approach range
	return s, p, obj, cc
}

// TestTryFireApTriggerObjNoScript verifies a *Obj target with no APOBJ
// trigger leaves the interaction anchored, sets apRange=-1, interactionFired=true.
func TestTryFireApTriggerObjNoScript(t *testing.T) {
	_, p, obj, _ := makeApObjTriggerFixture(t)

	tryFireApTrigger(p)

	if p.target != obj {
		t.Errorf("target: got %v, want obj (no-AP-script should not clear)", p.target)
	}
	if !p.interactionFired {
		t.Error("interactionFired: want true after no-AP-script mark")
	}
	if p.apRange != -1 {
		t.Errorf("apRange: got %d, want -1 (sentinel for no-AP-script)", p.apRange)
	}
}

// TestTryFireApTriggerObjScriptFiresNoApRangeCalled verifies an APOBJ script
// that runs but doesn't call p_aprange causes ClearInteraction.
func TestTryFireApTriggerObjScriptFiresNoApRangeCalled(t *testing.T) {
	s, p, obj, _ := makeApObjTriggerFixture(t)

	sf := newNoopScriptFile(t, script.TriggerApObj1, obj.Type, -1)
	s.scriptProvider.Register(sf)

	tryFireApTrigger(p)

	if p.target != nil {
		t.Errorf("target: got %v, want nil after no-p_aprange clear", p.target)
	}
	if !p.interactionFired {
		t.Error("interactionFired: want true after clear")
	}
}
```

- [ ] **Step 3.2 — Run tests to verify they fail**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/... -run "TestTryFireOpTriggerObj|TestTryFireApTriggerObj" -v 2>&1 | tail -15
```

Expected: FAIL — `targetOpObjT undefined` or `fireOpTriggerObj undefined`.

- [ ] **Step 3.3 — Add targetOp sentinel constants**

In `modules/world/interaction.go`, find the sentinel block (around line 30-35):

```go
	targetOpLocT    = 6  // APLOCT / OPLOCT dispatch marker
	targetOpLocU    = 7  // APLOCU / OPLOCU dispatch marker
	targetOpNpcT    = 8  // APNPCT / OPNPCT dispatch marker (S6o)
	targetOpNpcU    = 9  // APNPCU / OPNPCU dispatch marker (S6o)
	targetOpPlayerT = 10 // APPLAYERT / OPPLAYERT dispatch marker (NAI-40)
	targetOpPlayerU = 11 // APPLAYERU / OPPLAYERU dispatch marker (NAI-40)
```

Append two new lines after `targetOpPlayerU`:

```go
	targetOpObjT = 12 // APOBJT / OPOBJT dispatch marker (NAI-50)
	targetOpObjU = 13 // APOBJU / OPOBJU dispatch marker (NAI-50)
```

- [ ] **Step 3.4 — Add apObjTriggerForOp + fireOpTriggerObj + fireApTriggerObj, wire case arms**

In `modules/world/interaction_trigger.go`:

**3.4a** — Replace the default arm in `tryFireOpTrigger` (around line 46-50):

Find:
```go
	default:
		// Target type not handled by any branch: skip; mark fired so we
		// don't retry every tick.
		p.interactionFired = true
	}
```

Replace with:
```go
	case *entitypkg.Obj:
		fireOpTriggerObj(p, srv, tgt)
	default:
		p.interactionFired = true
	}
```

**3.4b** — Replace the default arm in `tryFireApTrigger` (around line 269-272):

Find:
```go
	default:
		// *Obj, etc. — AP branch not yet wired. Mark fired to prevent
		// same-tick retry. Follow-up: APOBJ sub-spec.
		p.interactionFired = true
	}
```

Replace with:
```go
	case *entitypkg.Obj:
		fireApTriggerObj(p, srv, tgt)
	default:
		p.interactionFired = true
	}
```

**3.4c** — Append the three new functions at the end of `interaction_trigger.go`:

```go
// apObjTriggerForOp returns the APOBJ trigger for p.targetOp. Returns
// ok=false for unrecognised sentinels. fireOpTriggerObj derives OPOBJ
// by adding 7 (TS Player.ts:997 offset convention):
//
//	APOBJ1..5 (31..35) + 7 → OPOBJ1..5 (38..42)
//	APOBJT    (37)     + 7 → OPOBJT    (44)
//	APOBJU    (36)     + 7 → OPOBJU    (43)
func apObjTriggerForOp(op int) (script.ServerTriggerType, bool) {
	switch {
	case op >= 1 && op <= 5:
		return script.TriggerApObj1 + script.ServerTriggerType(op-1), true
	case op == targetOpObjT:
		return script.TriggerApObjT, true
	case op == targetOpObjU:
		return script.TriggerApObjU, true
	default:
		return 0, false
	}
}

// fireOpTriggerObj fires the [opobj<op>,<objType>] trigger for the player's
// anchored Obj target when the player has reached operable distance.
// Mirrors fireOpTriggerLoc with three substitutions:
//  1. Lifecycle gate: objStillValid (zone-membership check).
//  2. ScriptState: ActiveObj + PtrActiveObj.
//  3. No-script fallback: "Nothing interesting happens." (TS Player.ts:1095).
func fireOpTriggerObj(p *Player, srv *Server, obj *entitypkg.Obj) {
	if p.delayed && srv.currentTick < p.delayedUntil {
		return
	}

	if !objStillValid(srv, obj, p.targetSubject.x, p.targetSubject.z, p.targetSubject.level) {
		p.ClearInteraction()
		p.interactionFired = true
		return
	}

	apTrigger, ok := apObjTriggerForOp(p.targetOp)
	if !ok {
		p.ClearInteraction()
		p.interactionFired = true
		return
	}
	trigger := apTrigger + 7 // APOBJ→OPOBJ offset per TS Player.ts:997

	category := 0
	if obj.Type >= 0 && obj.Type < len(srv.objTypes.Configs) {
		if ot := srv.objTypes.Configs[obj.Type]; ot != nil {
			category = ot.Category
		}
	}

	sf := srv.scriptProvider.GetByTrigger(trigger, obj.Type, category)
	if sf == nil {
		p.MessageGame("Nothing interesting happens.")
		p.ClearInteraction()
		p.interactionFired = true
		return
	}

	state := script.Init(sf, p, true, nil, nil)
	state.ActiveObj = obj
	state.Pointers |= script.PtrActiveObj
	state.Provider = srv.scriptProvider
	state.World = srv.worldVars
	state.Configs = srv.configsView
	state.Inv = srv.invLookup
	state.Npcs = srv.npcLookup
	state.LineValidator = srv.scriptLineValidator()

	srv.resumeOrFinish(state, p)

	if state.Execution == script.Finished || state.Execution == script.Aborted {
		p.ClearInteraction()
	}
	p.interactionFired = true
}

// fireApTriggerObj fires the [apobj<op>,<objType>] approach-trigger for the
// player's anchored Obj target. Mirrors fireApTriggerLoc with three
// substitutions:
//  1. Lifecycle gate: objStillValid.
//  2. ScriptState: ActiveObj + PtrActiveObj.
//  3. No-script path: apRange=-1 sentinel (OP trigger takes over on contact).
func fireApTriggerObj(p *Player, srv *Server, obj *entitypkg.Obj) {
	if p.delayed && srv.currentTick < p.delayedUntil {
		return
	}

	if !objStillValid(srv, obj, p.targetSubject.x, p.targetSubject.z, p.targetSubject.level) {
		p.ClearInteraction()
		p.interactionFired = true
		return
	}

	trigger, ok := apObjTriggerForOp(p.targetOp)
	if !ok {
		p.ClearInteraction()
		p.interactionFired = true
		return
	}

	category := 0
	if obj.Type >= 0 && obj.Type < len(srv.objTypes.Configs) {
		if ot := srv.objTypes.Configs[obj.Type]; ot != nil {
			category = ot.Category
		}
	}

	sf := srv.scriptProvider.GetByTrigger(trigger, obj.Type, category)
	if sf == nil {
		p.apRange = -1
		p.interactionFired = true
		return
	}

	p.apRangeCalled = false

	state := script.Init(sf, p, true, nil, nil)
	state.ActiveObj = obj
	state.Pointers |= script.PtrActiveObj
	state.Provider = srv.scriptProvider
	state.World = srv.worldVars
	state.Configs = srv.configsView
	state.Inv = srv.invLookup
	state.Npcs = srv.npcLookup
	state.LineValidator = srv.scriptLineValidator()

	srv.resumeOrFinish(state, p)

	if state.Execution == script.Finished || state.Execution == script.Aborted {
		if p.apRangeCalled {
			p.repathed = false
			return
		}
		p.ClearInteraction()
	}
	p.interactionFired = true
}
```

- [ ] **Step 3.5 — Run trigger dispatch tests to verify they pass**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/... -run "TestTryFireOpTriggerObj|TestTryFireApTriggerObj" -v 2>&1 | tail -25
```

Expected: all PASS.

- [ ] **Step 3.6 — Full suite green check**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/... ./pkg/objtype/... -count=1 2>&1 | tail -5
```

Expected: ok, no failures.

- [ ] **Step 3.7 — Commit**

```bash
git add modules/world/interaction.go modules/world/interaction_trigger.go modules/world/handler_opobj_test.go
git commit --no-gpg-sign -m "feat(world): NAI-50 T3 — fireOpTriggerObj + fireApTriggerObj + Obj case arms"
```

---

## Task 4: Register opcodes + close commit

**Files:**
- Modify: `modules/world/handlers_game.go`

### Step 4.1 — Add gameHandlers registrations

In `modules/world/handlers_game.go`, in the `init()` function, add after the last OPPLAYER registration block (after `gameHandlers[248] = handleOpPlayerU`):

```go
	gameHandlers[140] = handleOpObj1 // OPOBJ1
	gameHandlers[40]  = handleOpObj2 // OPOBJ2
	gameHandlers[200] = handleOpObj3 // OPOBJ3
	gameHandlers[178] = handleOpObj4 // OPOBJ4
	gameHandlers[247] = handleOpObj5 // OPOBJ5
	gameHandlers[138] = handleOpObjT // OPOBJT
	gameHandlers[239] = handleOpObjU // OPOBJU
```

Opcodes match `pkg/io/protocol/game/client/prot.go:52-58`.

- [ ] **Step 4.2 — Run full suite**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/... ./pkg/objtype/... -count=1 -race 2>&1 | tail -5
```

Expected: ok, no failures, no races.

- [ ] **Step 4.3 — Verify deviation doc-comments are present**

```bash
grep -rn "NAI-50-D1\|NAI-50-D2" modules/world/ pkg/
```

Expected: exactly 2 hits — one in `handler_opobj.go` (handleOpObjT) and one in `handler_opobj.go` (handleOpObjU). Both were added in Task 2.

- [ ] **Step 4.4 — Verify all seven opcodes are wired**

```bash
grep "OPOBJ" modules/world/handlers_game.go
```

Expected: 7 lines (OPOBJ1..5, OPOBJT, OPOBJU).

- [ ] **Step 4.5 — Close commit**

```bash
git add modules/world/handlers_game.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
chore(close): NAI-50 — OPOBJ1-5 + OPOBJT + OPOBJU handlers + trigger dispatch

Opens: NAI-50-D1 (OPOBJT spellCom component validation skipped),
       NAI-50-D2 (OPOBJU useCom component validation skipped).
Deviation tally: 20 → 22.

Closes memory: NAI-50 close.
EOF
)"
```
