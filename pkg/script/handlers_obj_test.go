package script

import (
	"testing"

	"github.com/zsrv/goscape/pkg/coordgrid"
	"github.com/zsrv/goscape/pkg/objtype"
)

func TestHandleObjCoord(t *testing.T) {
	s := newTestState(minimalScript(OpReturn))
	s.ActiveObj = &mockActiveObj{objType: 590, x: 3200, z: 3200, level: 0}

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
	s := newTestState(minimalScript(OpReturn))
	if err := handleObjCoord(s); err == nil {
		t.Errorf("OBJ_COORD: expected error on nil ActiveObj, got nil")
	}
}

// fakeWorldRemoveObj records RemoveObj calls. Embeds *mockWorld so the
// full WorldVars surface is satisfied.
type fakeWorldRemoveObj struct {
	*mockWorld
	removed []ActiveObj
}

func (f *fakeWorldRemoveObj) RemoveObj(obj ActiveObj) {
	f.removed = append(f.removed, obj)
}

func TestHandleObjDel(t *testing.T) {
	s := newTestState(minimalScript(OpReturn))
	w := &fakeWorldRemoveObj{mockWorld: newMockWorld()}
	s.World = w
	active := &mockActiveObj{objType: 590, x: 3200, z: 3200, level: 0}
	s.ActiveObj = active

	if err := handleObjDel(s); err != nil {
		t.Fatalf("handleObjDel returned error: %v", err)
	}
	if len(w.removed) != 1 || w.removed[0] != active {
		t.Errorf("OBJ_DEL: expected 1 RemoveObj call with active, got %v", w.removed)
	}
}

func TestHandleObjDelNilActive(t *testing.T) {
	s := newTestState(minimalScript(OpReturn))
	s.World = &fakeWorldRemoveObj{mockWorld: newMockWorld()}
	if err := handleObjDel(s); err == nil {
		t.Errorf("OBJ_DEL: expected error on nil ActiveObj, got nil")
	}
}

// fakeWorldAddObj records AddObj calls. Embeds *mockWorld for full
// WorldVars satisfaction. Set mapMembers on the embedded mockWorld
// (e.g. fakeWorldAddObj{mockWorld: &mockWorld{mapMembers: 1}, ...}).
type fakeWorldAddObj struct {
	*mockWorld
	addedCalls             []addObjCall
	enqueueObjDelayedCalls []enqueueObjDelayedCall
}

type addObjCall struct {
	level, x, z, typeID, count, duration, receiverID int
}

// enqueueObjDelayedCall captures one EnqueueObjDelayed invocation for
// NAI-134 INV_DROPITEM_DELAYED tests. Field order mirrors the WorldVars
// signature exactly (level, x, z, typeID, count, duration, delay,
// receiverID).
type enqueueObjDelayedCall struct {
	level, x, z, typeID, count, duration, delay, receiverID int
}

func (f *fakeWorldAddObj) AddObj(level, x, z, typeID, count, duration, receiverID int) ActiveObj {
	f.addedCalls = append(f.addedCalls, addObjCall{level, x, z, typeID, count, duration, receiverID})
	return &mockActiveObj{objType: typeID, x: x, z: z, level: level}
}

func (f *fakeWorldAddObj) EnqueueObjDelayed(level, x, z, typeID, count, duration, delay, receiverID int) {
	f.enqueueObjDelayedCalls = append(f.enqueueObjDelayedCalls, enqueueObjDelayedCall{
		level: level, x: x, z: z,
		typeID: typeID, count: count,
		duration: duration, delay: delay,
		receiverID: receiverID,
	})
}

// withObjForObjAdd registers an ObjType in the mockConfigs with the
// given stackable/members/dummyitem flags. Returns the same mc for
// chaining. Avoids redefining a parallel newTestConfigsWithObj helper.
func withObjForObjAdd(mc *mockConfigs, id int, stackable, members bool, dummyitem int) *mockConfigs {
	ot := objtype.NewObjType(id)
	ot.Stackable = stackable
	ot.Members = members
	ot.DummyItem = dummyitem
	mc.objs[id] = ot
	return mc
}

func newFakeWorldMembers() *fakeWorldAddObj {
	mw := newMockWorld()
	mw.mapMembers = 1
	return &fakeWorldAddObj{mockWorld: mw}
}

func newFakeWorldF2P() *fakeWorldAddObj {
	mw := newMockWorld()
	mw.mapMembers = 0
	return &fakeWorldAddObj{mockWorld: mw}
}

func TestHandleObjAddStackable(t *testing.T) {
	s := newTestState(minimalScript(OpReturn))
	w := newFakeWorldMembers()
	s.World = w
	s.Configs = withObjForObjAdd(newTestConfigs(), 590, true, false, 0)
	s.Self = &mockPlayer{uidValue: 12345}

	// Push order: bottom-up matches TS popInts(4) destructuring [coord, objId, count, duration].
	s.PushInt(coordgrid.PackCoord(0, 3200, 3200))
	s.PushInt(590)
	s.PushInt(5)
	s.PushInt(100)

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
	s := newTestState(minimalScript(OpReturn))
	w := newFakeWorldMembers()
	s.World = w
	s.Configs = withObjForObjAdd(newTestConfigs(), 590, false, false, 0)
	s.Self = &mockPlayer{uidValue: 12345}

	s.PushInt(coordgrid.PackCoord(0, 3200, 3200))
	s.PushInt(590)
	s.PushInt(3)
	s.PushInt(100)

	if err := handleObjAdd(s); err != nil {
		t.Fatalf("handleObjAdd returned error: %v", err)
	}
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
	s := newTestState(minimalScript(OpReturn))
	w := newFakeWorldMembers()
	s.World = w
	s.Configs = withObjForObjAdd(newTestConfigs(), 590, true, false, 0)
	s.Self = &mockPlayer{uidValue: 12345}

	s.PushInt(coordgrid.PackCoord(0, 3200, 3200))
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
	s := newTestState(minimalScript(OpReturn))
	w := newFakeWorldMembers()
	s.World = w
	s.Configs = withObjForObjAdd(newTestConfigs(), 590, true, false, 1) // dummyitem=1
	s.Self = &mockPlayer{uidValue: 12345}

	s.PushInt(coordgrid.PackCoord(0, 3200, 3200))
	s.PushInt(590)
	s.PushInt(1)
	s.PushInt(100)

	if err := handleObjAdd(s); err == nil {
		t.Errorf("OBJ_ADD: expected dummyitem error, got nil")
	}
}

func TestHandleObjAddMembersGate(t *testing.T) {
	s := newTestState(minimalScript(OpReturn))
	w := newFakeWorldF2P() // F2P world
	s.World = w
	s.Configs = withObjForObjAdd(newTestConfigs(), 590, true, true, 0) // members=true
	s.Self = &mockPlayer{uidValue: 12345}

	s.PushInt(coordgrid.PackCoord(0, 3200, 3200))
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

func TestHandleObjAddNilWorldErrors(t *testing.T) {
	s := newTestState(minimalScript(OpReturn))
	s.World = nil
	s.Configs = withObjForObjAdd(newTestConfigs(), 590, true, false, 0)
	s.Self = &mockPlayer{uidValue: 12345}

	s.PushInt(coordgrid.PackCoord(0, 3200, 3200))
	s.PushInt(590)
	s.PushInt(1)
	s.PushInt(100)

	if err := handleObjAdd(s); err == nil {
		t.Errorf("OBJ_ADD nil world: expected error, got nil")
	}
}

func TestHandleObjAddAllUsesPublicReceiver(t *testing.T) {
	s := newTestState(minimalScript(OpReturn))
	w := newFakeWorldMembers()
	s.World = w
	s.Configs = withObjForObjAdd(newTestConfigs(), 590, true, false, 0)
	// No s.Self required — broadcast does not need an active player.

	s.PushInt(coordgrid.PackCoord(0, 3200, 3200))
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
	want := -1 // zone.PublicReceiver
	if got != want {
		t.Errorf("OBJ_ADDALL receiverID: got %d, want %d (broadcast/PublicReceiver)", got, want)
	}
}

func TestHandleObjAddAllNonStackableSplits(t *testing.T) {
	s := newTestState(minimalScript(OpReturn))
	w := newFakeWorldMembers()
	s.World = w
	s.Configs = withObjForObjAdd(newTestConfigs(), 590, false, false, 0)

	s.PushInt(coordgrid.PackCoord(0, 3200, 3200))
	s.PushInt(590)
	s.PushInt(3)
	s.PushInt(100)

	if err := handleObjAddAll(s); err != nil {
		t.Fatalf("handleObjAddAll returned error: %v", err)
	}
	if len(w.addedCalls) != 3 {
		t.Fatalf("expected 3 AddObj calls (non-stackable split), got %d", len(w.addedCalls))
	}
	for i, c := range w.addedCalls {
		if c.receiverID != -1 {
			t.Errorf("OBJ_ADDALL call[%d] receiverID: got %d, want -1", i, c.receiverID)
		}
		if c.count != 1 {
			t.Errorf("OBJ_ADDALL call[%d] count: got %d, want 1", i, c.count)
		}
	}
}

// TestHandleObjAddSetsActiveObjAndPointer pins NAI-115-D3 closure:
// OBJ_ADD must set s.ActiveObj and PtrActiveObj after the spawn.
func TestHandleObjAddSetsActiveObjAndPointer(t *testing.T) {
	s := newTestState(minimalScript(OpReturn))
	w := newFakeWorldMembers()
	s.World = w
	s.Configs = withObjForObjAdd(newTestConfigs(), 590, true, false, 0)
	s.Self = &mockPlayer{uidValue: 12345}

	s.PushInt(coordgrid.PackCoord(0, 3200, 3200))
	s.PushInt(590)
	s.PushInt(1)
	s.PushInt(100)

	if err := handleObjAdd(s); err != nil {
		t.Fatalf("handleObjAdd returned error: %v", err)
	}
	if s.ActiveObj == nil {
		t.Fatalf("OBJ_ADD: expected s.ActiveObj set, got nil")
	}
	if s.Pointers&PtrActiveObj == 0 {
		t.Errorf("OBJ_ADD: expected PtrActiveObj set in Pointers")
	}
	x, z, level := s.ActiveObj.Coords()
	if x != 3200 || z != 3200 || level != 0 {
		t.Errorf("OBJ_ADD: ActiveObj coords got (%d,%d,%d), want (3200,3200,0)", x, z, level)
	}
}
