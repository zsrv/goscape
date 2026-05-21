package script

import (
	"strings"
	"testing"

	"github.com/zsrv/goscape/pkg/coordgrid"
	"github.com/zsrv/goscape/pkg/inventory"
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

// TestCheckObjType validates the state-aware ObjType validator at
// handlers_obj.go:44. Mirrors TestCheckNpcType.
func TestCheckObjType(t *testing.T) {
	// Build a minimal ScriptState with a Configs that reports ObjType 42 as present.
	s := &ScriptState{Configs: newTestConfigsWithObjTypes(map[int]bool{42: true})}

	if err := checkObjType(s, 42, "TEST"); err != nil {
		t.Errorf("checkObjType(42) with loaded type: unexpected error %v", err)
	}
	if err := checkObjType(s, 999, "TEST"); err == nil {
		t.Errorf("checkObjType(999) with unloaded type: want error")
	} else if !strings.Contains(err.Error(), "TEST:") || !strings.Contains(err.Error(), "999") {
		t.Errorf("error should carry op prefix and offending id: %v", err)
	}
	if err := checkObjType(s, -1, "TEST"); err == nil {
		t.Errorf("checkObjType(-1): want error")
	}

	// Nil Configs: always errors.
	s2 := &ScriptState{}
	if err := checkObjType(s2, 42, "TEST"); err == nil {
		t.Errorf("checkObjType with nil Configs: want error")
	}
}

// newTestConfigsWithObjTypes builds a Configs that reports any id in present
// as a valid ObjType. Uses the shared mockConfigs type from handlers_config_test.go.
func newTestConfigsWithObjTypes(present map[int]bool) Configs {
	mc := &mockConfigs{
		objs: make(map[int]*objtype.ObjType),
	}
	for id := range present {
		mc.objs[id] = objtype.NewObjType(id)
	}
	return mc
}

// removeObjCall captures one RemoveObj invocation: the ActiveObj
// argument and the duration plumbed through to the world adapter.
// Used by fakeWorldRemoveObj and fakeWorldTakeItem so handler tests
// can assert on duration values (NAI-178 B3).
type removeObjCall struct {
	obj      ActiveObj
	duration int
}

// fakeWorldRemoveObj records RemoveObj calls. Embeds *mockWorld so the
// full WorldVars surface is satisfied.
type fakeWorldRemoveObj struct {
	*mockWorld
	removed []removeObjCall
}

func (f *fakeWorldRemoveObj) RemoveObj(obj ActiveObj, duration int) {
	f.removed = append(f.removed, removeObjCall{obj: obj, duration: duration})
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
	if len(w.removed) != 1 || w.removed[0].obj != active {
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

// TestHandleObjDel_PassesRespawnRateFromObjType pins that OBJ_DEL reads
// ObjType.RespawnRate from Configs and plumbs it into WorldVars.RemoveObj
// as duration. Mirrors TS ObjOps.ts:113. NAI-178 B3.
func TestHandleObjDel_PassesRespawnRateFromObjType(t *testing.T) {
	s := newTestState(minimalScript(OpReturn))
	w := &fakeWorldRemoveObj{mockWorld: newMockWorld()}
	s.World = w
	mc := newTestConfigs()
	ot := objtype.NewObjType(590)
	ot.RespawnRate = 200
	mc.objs[590] = ot
	s.Configs = mc
	s.ActiveObj = &mockActiveObj{objType: 590, x: 3200, z: 3200, level: 0}

	if err := handleObjDel(s); err != nil {
		t.Fatalf("handleObjDel returned error: %v", err)
	}
	if len(w.removed) != 1 || w.removed[0].duration != 200 {
		t.Errorf("OBJ_DEL: expected duration=200, got %v", w.removed)
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

// --- NAI-152 B2 T1: OBJ_TYPE handler ------------------------------------

func TestHandleObjType(t *testing.T) {
	s := newTestState(minimalScript(OpReturn))
	s.ActiveObj = &mockActiveObj{objType: 558, x: 3200, z: 3200, level: 0}

	if err := handleObjType(s); err != nil {
		t.Fatalf("handleObjType returned error: %v", err)
	}
	got := s.PopInt()
	if got != 558 {
		t.Errorf("OBJ_TYPE: got %d, want 558 (mindrune id)", got)
	}
}

func TestHandleObjTypeNilActive(t *testing.T) {
	s := newTestState(minimalScript(OpReturn))
	if err := handleObjType(s); err == nil {
		t.Errorf("OBJ_TYPE: expected error on nil ActiveObj, got nil")
	}
}

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

// --- NAI-153 T4: OBJ_TAKEITEM handler -----------------------------------

// fakeWorldTakeItem combines RemoveObj recording (for OBJ_TAKEITEM
// removal) and AddObj recording (for performInvAdd overflow drop —
// expected zero in the happy path). Embeds *mockWorld for the rest of
// the WorldVars surface.
type fakeWorldTakeItem struct {
	*mockWorld
	removed    []removeObjCall
	addedCalls []addObjCall
}

func (f *fakeWorldTakeItem) RemoveObj(obj ActiveObj, duration int) {
	f.removed = append(f.removed, removeObjCall{obj: obj, duration: duration})
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
	invType.Protect = false // NewInvType defaults true; clear so performInvAdd's protect/scope gate doesn't fire.
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
	if len(w.removed) != 1 || w.removed[0].obj != active {
		t.Errorf("OBJ_TAKEITEM: expected 1 RemoveObj call with active, got %v", w.removed)
	}
	if len(w.addedCalls) != 0 {
		t.Errorf("OBJ_TAKEITEM: expected 0 AddObj calls (no overflow), got %v", w.addedCalls)
	}
}

// TestHandleObjTakeItem_RespawnLifecyclePassesRespawnRate pins that the
// RESPAWN-lifecycle arm reads ObjType.RespawnRate and forwards it as
// duration to WorldVars.RemoveObj. Mirrors TS ObjOps.ts:156-157.
// NAI-178 B3.
func TestHandleObjTakeItem_RespawnLifecyclePassesRespawnRate(t *testing.T) {
	s, w, _ := newTakeItemFixture(t)
	mc := s.Configs.(*mockConfigs)
	mc.objs[558].RespawnRate = 300
	active := &mockActiveObj{objType: 558, x: 3200, z: 3200, level: 0, count: 1, reveal: -1, respawnLifecycle: true}
	s.ActiveObj = active

	s.PushInt(93)

	if err := handleObjTakeItem(s); err != nil {
		t.Fatalf("OBJ_TAKEITEM: returned error: %v", err)
	}
	if len(w.removed) != 1 || w.removed[0].duration != 300 {
		t.Errorf("OBJ_TAKEITEM (RESPAWN): expected duration=300, got %v", w.removed)
	}
}

// TestHandleObjTakeItem_DespawnLifecyclePassesZero pins that the
// DESPAWN-lifecycle arm forwards duration=0 to WorldVars.RemoveObj even
// when ObjType.RespawnRate is non-zero. Mirrors TS ObjOps.ts:158-160.
// NAI-178 B3.
func TestHandleObjTakeItem_DespawnLifecyclePassesZero(t *testing.T) {
	s, w, _ := newTakeItemFixture(t)
	mc := s.Configs.(*mockConfigs)
	mc.objs[558].RespawnRate = 300
	active := &mockActiveObj{objType: 558, x: 3200, z: 3200, level: 0, count: 1, reveal: -1, respawnLifecycle: false}
	s.ActiveObj = active

	s.PushInt(93)

	if err := handleObjTakeItem(s); err != nil {
		t.Fatalf("OBJ_TAKEITEM: returned error: %v", err)
	}
	if len(w.removed) != 1 || w.removed[0].duration != 0 {
		t.Errorf("OBJ_TAKEITEM (DESPAWN): expected duration=0, got %v", w.removed)
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

// --- NAI-154: OBJ_FIND handler tests ---------------------------------

// objFindCall records a single WorldVars.GetObj invocation.
type objFindCall struct {
	level, x, z, objId, receiverUID int
}

// fakeWorldObjFind extends mockWorld to record GetObj calls and return a
// preset result. Mirrors fakeWorldAddObj.
type fakeWorldObjFind struct {
	*mockWorld
	result ActiveObj
	calls  []objFindCall
}

func (w *fakeWorldObjFind) GetObj(level, x, z, objId, receiverUID int) ActiveObj {
	w.calls = append(w.calls, objFindCall{level, x, z, objId, receiverUID})
	return w.result
}

func newFakeWorldObjFind(result ActiveObj) *fakeWorldObjFind {
	return &fakeWorldObjFind{mockWorld: newMockWorld(), result: result}
}

// newObjFindState builds a ScriptState ready for handleObjFind: World
// wired, Configs wired with the given objId registered, Self+UID
// populated, IntOperand set for slot routing, and the coord+objId
// pre-pushed onto the int stack (bottom-up matches popInts semantics:
// coord pushed first, objId pushed second).
func newObjFindState(t *testing.T, w WorldVars, mc *mockConfigs, intOperand int32, coord, objId, uid int) *ScriptState {
	t.Helper()
	s := &ScriptState{
		Script:      &ScriptFile{IntOperands: []int32{intOperand}},
		PC:          0,
		World:       w,
		Configs:     mc,
		IntStack:    make([]int, StackCapacity),
		StringStack: make([]string, StackCapacity),
	}
	s.Self = &mockPlayer{uidValue: uid}
	s.Pointers |= PtrActivePlayer
	s.PushInt(coord)
	s.PushInt(objId)
	return s
}

// TestObjFindHitPrimarySlot pins OBJ_FIND IntOperand=0: hit sets
// s.ActiveObj, sets PtrActiveObj, pushes 1.
func TestObjFindHitPrimarySlot(t *testing.T) {
	obj := &mockActiveObj{objType: 590, x: 3200, z: 3300, level: 0, count: 1}
	w := newFakeWorldObjFind(obj)
	mc := withObjForObjAdd(newTestConfigs(), 590, true, false, 0)
	coord := coordgrid.PackCoord(0, 3200, 3300)
	s := newObjFindState(t, w, mc, 0 /*intOperand*/, coord, 590, 12345)

	if err := handleObjFind(s); err != nil {
		t.Fatalf("handleObjFind: %v", err)
	}
	if got := s.PopInt(); got != 1 {
		t.Errorf("push: got %d, want 1 (hit)", got)
	}
	if s.ActiveObj != obj {
		t.Errorf("ActiveObj: got %v, want %v", s.ActiveObj, obj)
	}
	if s.Pointers&PtrActiveObj == 0 {
		t.Error("PtrActiveObj not set")
	}
	if s.OtherActiveObj != nil {
		t.Errorf("OtherActiveObj: got %v, want nil (primary slot)", s.OtherActiveObj)
	}
}

// TestObjFindHitSecondarySlot pins OBJ_FIND IntOperand=1: hit sets
// s.OtherActiveObj, sets PtrActiveObj2, pushes 1.
func TestObjFindHitSecondarySlot(t *testing.T) {
	obj := &mockActiveObj{objType: 590, x: 3200, z: 3300, level: 0, count: 1}
	w := newFakeWorldObjFind(obj)
	mc := withObjForObjAdd(newTestConfigs(), 590, true, false, 0)
	coord := coordgrid.PackCoord(0, 3200, 3300)
	s := newObjFindState(t, w, mc, 1 /*intOperand*/, coord, 590, 12345)

	if err := handleObjFind(s); err != nil {
		t.Fatalf("handleObjFind: %v", err)
	}
	if got := s.PopInt(); got != 1 {
		t.Errorf("push: got %d, want 1", got)
	}
	if s.OtherActiveObj != obj {
		t.Errorf("OtherActiveObj: got %v, want %v", s.OtherActiveObj, obj)
	}
	if s.Pointers&PtrActiveObj2 == 0 {
		t.Error("PtrActiveObj2 not set")
	}
	if s.ActiveObj != nil {
		t.Errorf("ActiveObj: got %v, want nil (secondary slot)", s.ActiveObj)
	}
}

// TestObjFindMissPushesZero pins the nil-result branch: pushes 0, leaves
// ActiveObj/OtherActiveObj untouched.
func TestObjFindMissPushesZero(t *testing.T) {
	w := newFakeWorldObjFind(nil)
	mc := withObjForObjAdd(newTestConfigs(), 590, true, false, 0)
	coord := coordgrid.PackCoord(0, 3200, 3300)
	s := newObjFindState(t, w, mc, 0, coord, 590, 12345)

	if err := handleObjFind(s); err != nil {
		t.Fatalf("handleObjFind: %v", err)
	}
	if got := s.PopInt(); got != 0 {
		t.Errorf("push: got %d, want 0 (miss)", got)
	}
	if s.ActiveObj != nil {
		t.Errorf("ActiveObj: got %v, want nil on miss", s.ActiveObj)
	}
}

// TestObjFindRequiresActivePlayer pins the requireActivePlayer guard:
// no Self → error.
func TestObjFindRequiresActivePlayer(t *testing.T) {
	w := newFakeWorldObjFind(nil)
	mc := withObjForObjAdd(newTestConfigs(), 590, true, false, 0)
	coord := coordgrid.PackCoord(0, 3200, 3300)
	s := newObjFindState(t, w, mc, 0, coord, 590, 12345)
	// Clear the player.
	s.Self = nil
	s.Pointers &^= PtrActivePlayer

	err := handleObjFind(s)
	if err == nil {
		t.Fatal("handleObjFind: want error (no active player), got nil")
	}
	if !strings.Contains(err.Error(), "OBJ_FIND") {
		t.Errorf("err: got %q, want substring %q", err.Error(), "OBJ_FIND")
	}
}

// TestObjFindUnknownObjId pins the Configs lookup guard: unknown objId
// → error.
func TestObjFindUnknownObjId(t *testing.T) {
	w := newFakeWorldObjFind(nil)
	mc := newTestConfigs() // no 590 registered
	coord := coordgrid.PackCoord(0, 3200, 3300)
	s := newObjFindState(t, w, mc, 0, coord, 590, 12345)

	err := handleObjFind(s)
	if err == nil {
		t.Fatal("handleObjFind: want error (no ObjType with value), got nil")
	}
	if !strings.Contains(err.Error(), "no ObjType with value") {
		t.Errorf("err: got %q, want substring %q", err.Error(), "no ObjType with value")
	}
}

// TestObjFindInvalidCoord pins the checkCoord guard: -1 → error.
func TestObjFindInvalidCoord(t *testing.T) {
	w := newFakeWorldObjFind(nil)
	mc := withObjForObjAdd(newTestConfigs(), 590, true, false, 0)
	s := newObjFindState(t, w, mc, 0, -1 /*coord*/, 590, 12345)

	err := handleObjFind(s)
	if err == nil {
		t.Fatal("handleObjFind: want error (invalid coord), got nil")
	}
	if !strings.Contains(err.Error(), "OBJ_FIND") {
		t.Errorf("err: got %q, want substring %q", err.Error(), "OBJ_FIND")
	}
}

// TestObjFindUIDPropagation pins NAI-153-D2: receiverUID passed to
// WorldVars.GetObj is s.Self.UID() (goscape player UID, not TS hash64).
func TestObjFindUIDPropagation(t *testing.T) {
	obj := &mockActiveObj{objType: 590, count: 1}
	w := newFakeWorldObjFind(obj)
	mc := withObjForObjAdd(newTestConfigs(), 590, true, false, 0)
	coord := coordgrid.PackCoord(0, 3200, 3300)
	s := newObjFindState(t, w, mc, 0, coord, 590, 98765 /*uid*/)

	if err := handleObjFind(s); err != nil {
		t.Fatalf("handleObjFind: %v", err)
	}
	if len(w.calls) != 1 {
		t.Fatalf("GetObj calls: got %d, want 1", len(w.calls))
	}
	got := w.calls[0]
	want := objFindCall{level: 0, x: 3200, z: 3300, objId: 590, receiverUID: 98765}
	if got != want {
		t.Errorf("GetObj call: got %+v, want %+v", got, want)
	}
}

// --- NAI-154: OBJ_FINDALLZONE + OBJ_FINDNEXT handler tests -----------

// newObjFindAllZoneState builds a ScriptState with a coord on the int
// stack, World wired (for CurrentTick), and IntOperands sized for the
// handler. Mirror of newLocFindAllZoneState (handlers_loc_test.go).
func newObjFindAllZoneState(t *testing.T, w WorldVars, coord int) *ScriptState {
	t.Helper()
	s := &ScriptState{
		Script:      &ScriptFile{IntOperands: []int32{0}},
		PC:          0,
		World:       w,
		IntStack:    make([]int, StackCapacity),
		StringStack: make([]string, StackCapacity),
	}
	s.PushInt(coord)
	return s
}

// newObjFindNextState builds a ScriptState with World wired (for
// CurrentTick), an optional objIterator pre-installed, and IntOperands
// supplied for setActiveObjSlot. Mirror of newLocFindNextState.
func newObjFindNextState(t *testing.T, tick int, iter *ObjIterator, intOperand int32) *ScriptState {
	t.Helper()
	mw := newMockWorld()
	mw.tick = tick
	s := &ScriptState{
		Script:      &ScriptFile{IntOperands: []int32{intOperand}},
		PC:          0,
		World:       mw,
		IntStack:    make([]int, StackCapacity),
		StringStack: make([]string, StackCapacity),
	}
	s.objIterator = iter
	return s
}

// TestObjFindAllZoneStoresIterator pins OBJ_FINDALLZONE: pop coord →
// store iterator with creationTick from World.CurrentTick + level/x/z
// from coord.
func TestObjFindAllZoneStoresIterator(t *testing.T) {
	w := newObjIterTestWorld(nil)
	w.tick = 100
	coord := coordgrid.PackCoord(0, 3200, 3300)
	s := newObjFindAllZoneState(t, w, coord)

	if err := handleObjFindAllZone(s); err != nil {
		t.Fatalf("handleObjFindAllZone: %v", err)
	}
	if s.objIterator == nil {
		t.Fatal("objIterator: got nil, want set")
	}
	if s.objIterator.creationTick != 100 {
		t.Errorf("creationTick: got %d, want 100 (from World.CurrentTick)",
			s.objIterator.creationTick)
	}
	if s.objIterator.level != 0 || s.objIterator.x != 3200 || s.objIterator.z != 3300 {
		t.Errorf("coord: got (%d, %d, %d), want (0, 3200, 3300)",
			s.objIterator.level, s.objIterator.x, s.objIterator.z)
	}
}

// TestObjFindAllZoneNilWorldDegrades pins the LocFindAllZone parallel:
// nil World → handler returns nil, objIterator stays nil.
func TestObjFindAllZoneNilWorldDegrades(t *testing.T) {
	coord := coordgrid.PackCoord(0, 3200, 3300)
	s := newObjFindAllZoneState(t, nil, coord)
	s.World = nil

	if err := handleObjFindAllZone(s); err != nil {
		t.Fatalf("handleObjFindAllZone: got err %v, want nil (degrade silently)", err)
	}
	if s.objIterator != nil {
		t.Errorf("objIterator: got %v, want nil (no iterator on nil-world)", s.objIterator)
	}
}

// TestObjFindAllZoneCoordValid pins the checkCoord error path.
func TestObjFindAllZoneCoordValid(t *testing.T) {
	w := newObjIterTestWorld(nil)
	s := newObjFindAllZoneState(t, w, -1)

	err := handleObjFindAllZone(s)
	if err == nil {
		t.Fatal("handleObjFindAllZone(-1): want error, got nil")
	}
	if !strings.Contains(err.Error(), "OBJ_FINDALLZONE") {
		t.Errorf("err: got %q, want substring %q", err.Error(), "OBJ_FINDALLZONE")
	}
}

// TestObjFindNextNoIterator pins the nil-iterator branch.
func TestObjFindNextNoIterator(t *testing.T) {
	s := newObjFindNextState(t, 100, nil, 0)

	if err := handleObjFindNext(s); err != nil {
		t.Fatalf("handleObjFindNext: %v", err)
	}
	if got := s.PopInt(); got != 0 {
		t.Errorf("nil iterator: got push %d, want 0", got)
	}
	if s.ActiveObj != nil {
		t.Error("ActiveObj should remain nil")
	}
	if s.OtherActiveObj != nil {
		t.Error("OtherActiveObj should remain nil")
	}
}

// TestObjFindNextHitPrimarySlot pins OBJ_FINDNEXT IntOperand=0: hit
// sets s.ActiveObj, sets PtrActiveObj, pushes 1.
func TestObjFindNextHitPrimarySlot(t *testing.T) {
	obj := &mockActiveObj{objType: 590, count: 1}
	w := newObjIterTestWorld([]ActiveObj{obj})
	iter := NewZoneObjIterator(w, 100, 0, 3200, 3300)
	s := newObjFindNextState(t, 100, iter, 0)

	if err := handleObjFindNext(s); err != nil {
		t.Fatalf("handleObjFindNext: %v", err)
	}
	if got := s.PopInt(); got != 1 {
		t.Errorf("push: got %d, want 1 (hit)", got)
	}
	if s.ActiveObj != obj {
		t.Errorf("ActiveObj: got %v, want %v", s.ActiveObj, obj)
	}
	if s.Pointers&PtrActiveObj == 0 {
		t.Error("PtrActiveObj not set")
	}
}

// TestObjFindNextHitSecondarySlot pins OBJ_FINDNEXT IntOperand=1: hit
// sets s.OtherActiveObj, sets PtrActiveObj2, pushes 1.
func TestObjFindNextHitSecondarySlot(t *testing.T) {
	obj := &mockActiveObj{objType: 590, count: 1}
	w := newObjIterTestWorld([]ActiveObj{obj})
	iter := NewZoneObjIterator(w, 100, 0, 3200, 3300)
	s := newObjFindNextState(t, 100, iter, 1)

	if err := handleObjFindNext(s); err != nil {
		t.Fatalf("handleObjFindNext: %v", err)
	}
	if got := s.PopInt(); got != 1 {
		t.Errorf("push: got %d, want 1", got)
	}
	if s.OtherActiveObj != obj {
		t.Errorf("OtherActiveObj: got %v, want %v", s.OtherActiveObj, obj)
	}
	if s.Pointers&PtrActiveObj2 == 0 {
		t.Error("PtrActiveObj2 not set")
	}
	if s.ActiveObj != nil {
		t.Error("ActiveObj should remain nil (secondary slot)")
	}
}

// TestObjFindNextExhaustionPushesZero pins the exhaustion path.
func TestObjFindNextExhaustionPushesZero(t *testing.T) {
	obj := &mockActiveObj{objType: 590, count: 1}
	w := newObjIterTestWorld([]ActiveObj{obj})
	iter := NewZoneObjIterator(w, 100, 0, 3200, 3300)
	// Drain.
	if _, ok := iter.Next(); !ok {
		t.Fatal("setup: iterator should yield once")
	}
	s := newObjFindNextState(t, 100, iter, 0)

	if err := handleObjFindNext(s); err != nil {
		t.Fatalf("handleObjFindNext: %v", err)
	}
	if got := s.PopInt(); got != 0 {
		t.Errorf("exhausted: got push %d, want 0", got)
	}
	if s.ActiveObj != nil {
		t.Error("ActiveObj should remain nil on exhaustion")
	}
}

// TestObjFindNextStaleErrors pins the stale-iterator guard: iterator
// created at tick=0, World.CurrentTick advanced to 1 → error.
func TestObjFindNextStaleErrors(t *testing.T) {
	obj := &mockActiveObj{objType: 590, count: 1}
	w := newObjIterTestWorld([]ActiveObj{obj})
	iter := NewZoneObjIterator(w, 0, 0, 3200, 3300) // tick=0
	s := newObjFindNextState(t, 1, iter, 0)         // World.tick=1

	err := handleObjFindNext(s)
	if err == nil {
		t.Fatal("handleObjFindNext on stale iterator: want error, got nil")
	}
	if !strings.Contains(err.Error(), "old iterator") {
		t.Errorf("err: got %q, want substring %q", err.Error(), "old iterator")
	}
}

// --- NAI-154: OBJ_NAME + OBJ_PARAM handler tests ---------------------

// newObjNameOrParamState builds a state with s.ActiveObj installed,
// Configs wired, and (for OBJ_PARAM) paramID pre-pushed by the caller.
func newObjNameOrParamState(t *testing.T, obj ActiveObj, mc *mockConfigs) *ScriptState {
	t.Helper()
	s := &ScriptState{
		Script:      &ScriptFile{IntOperands: []int32{0}},
		PC:          0,
		Configs:     mc,
		IntStack:    make([]int, StackCapacity),
		StringStack: make([]string, StackCapacity),
	}
	s.ActiveObj = obj
	return s
}

// TestObjNameNamePresent pins the Name branch.
func TestObjNameNamePresent(t *testing.T) {
	obj := &mockActiveObj{objType: 590, count: 1}
	mc := newTestConfigs()
	ot := objtype.NewObjType(590)
	ot.Name = "rune sword"
	mc.objs[590] = ot
	s := newObjNameOrParamState(t, obj, mc)

	if err := handleObjName(s); err != nil {
		t.Fatalf("handleObjName: %v", err)
	}
	if got := s.PopString(); got != "rune sword" {
		t.Errorf("name: got %q, want %q", got, "rune sword")
	}
}

// TestObjNameDebugFallback pins the DebugName fallback (Name empty).
func TestObjNameDebugFallback(t *testing.T) {
	obj := &mockActiveObj{objType: 590, count: 1}
	mc := newTestConfigs()
	ot := objtype.NewObjType(590)
	ot.Name = ""
	ot.DebugName = "sword_t1"
	mc.objs[590] = ot
	s := newObjNameOrParamState(t, obj, mc)

	if err := handleObjName(s); err != nil {
		t.Fatalf("handleObjName: %v", err)
	}
	if got := s.PopString(); got != "sword_t1" {
		t.Errorf("debugname fallback: got %q, want %q", got, "sword_t1")
	}
}

// TestObjNameNullFallback pins the "null" fallback (both empty).
func TestObjNameNullFallback(t *testing.T) {
	obj := &mockActiveObj{objType: 590, count: 1}
	mc := newTestConfigs()
	ot := objtype.NewObjType(590)
	ot.Name = ""
	ot.DebugName = ""
	mc.objs[590] = ot
	s := newObjNameOrParamState(t, obj, mc)

	if err := handleObjName(s); err != nil {
		t.Fatalf("handleObjName: %v", err)
	}
	if got := s.PopString(); got != "null" {
		t.Errorf("null fallback: got %q, want %q", got, "null")
	}
}

// TestObjNameRequiresActiveObj pins the requireActiveObj guard.
func TestObjNameRequiresActiveObj(t *testing.T) {
	s := newObjNameOrParamState(t, nil, newTestConfigs())

	err := handleObjName(s)
	if err == nil {
		t.Fatal("handleObjName(nil ActiveObj): want error, got nil")
	}
	if !strings.Contains(err.Error(), "OBJ_NAME") {
		t.Errorf("err: got %q, want substring %q", err.Error(), "OBJ_NAME")
	}
}

// TestObjNameRequiresConfigs pins the requireConfigs guard.
func TestObjNameRequiresConfigs(t *testing.T) {
	obj := &mockActiveObj{objType: 590, count: 1}
	s := newObjNameOrParamState(t, obj, nil)
	s.Configs = nil

	err := handleObjName(s)
	if err == nil {
		t.Fatal("handleObjName(nil Configs): want error, got nil")
	}
	if !strings.Contains(err.Error(), "OBJ_NAME") {
		t.Errorf("err: got %q, want substring %q", err.Error(), "OBJ_NAME")
	}
}

// TestObjNameUnknownType pins the unknown-objId guard.
func TestObjNameUnknownType(t *testing.T) {
	obj := &mockActiveObj{objType: 999, count: 1}
	s := newObjNameOrParamState(t, obj, newTestConfigs()) // 999 not registered

	err := handleObjName(s)
	if err == nil {
		t.Fatal("handleObjName(unknown objId): want error, got nil")
	}
	if !strings.Contains(err.Error(), "unknown obj id") {
		t.Errorf("err: got %q, want substring %q", err.Error(), "unknown obj id")
	}
}

// TestObjParamIntBranch pins the int-param branch.
// ParamMap stores int param values as uint32 wire bytes (per
// NAI-122 stretch in paramLookup). 42 is stored as uint32(42).
func TestObjParamIntBranch(t *testing.T) {
	obj := &mockActiveObj{objType: 590, count: 1}
	mc := newTestConfigs()
	ot := objtype.NewObjType(590)
	ot.Params = objtype.ParamMap{7: uint32(42)}
	mc.objs[590] = ot
	paramID := 7
	pt := objtype.NewParamType(paramID)
	pt.Type = objtype.ScriptVarTypeInt
	pt.DefaultInt = 0
	mc.params[paramID] = pt
	s := newObjNameOrParamState(t, obj, mc)
	s.PushInt(paramID)

	if err := handleObjParam(s); err != nil {
		t.Fatalf("handleObjParam: %v", err)
	}
	if got := s.PopInt(); got != 42 {
		t.Errorf("int param: got %d, want 42", got)
	}
}

// TestObjParamStringBranch pins the string-param branch.
func TestObjParamStringBranch(t *testing.T) {
	obj := &mockActiveObj{objType: 590, count: 1}
	mc := newTestConfigs()
	ot := objtype.NewObjType(590)
	ot.Params = objtype.ParamMap{8: "hello"}
	mc.objs[590] = ot
	paramID := 8
	pt := objtype.NewParamType(paramID)
	pt.Type = objtype.ScriptVarTypeString
	pt.DefaultString = ""
	mc.params[paramID] = pt
	s := newObjNameOrParamState(t, obj, mc)
	s.PushInt(paramID)

	if err := handleObjParam(s); err != nil {
		t.Fatalf("handleObjParam: %v", err)
	}
	if got := s.PopString(); got != "hello" {
		t.Errorf("string param: got %q, want %q", got, "hello")
	}
}

// TestObjParamIntDefaultFallback pins the int-default branch with
// negative-default sign preservation. Empty Params falls back to
// pt.DefaultInt; paramLookup pushes int(pt.DefaultInt) which preserves
// the int32 sign directly (no uint32 round-trip on the default path).
func TestObjParamIntDefaultFallback(t *testing.T) {
	obj := &mockActiveObj{objType: 590, count: 1}
	mc := newTestConfigs()
	ot := objtype.NewObjType(590)
	ot.Params = objtype.ParamMap{} // empty — fallback path
	mc.objs[590] = ot
	paramID := 9
	pt := objtype.NewParamType(paramID)
	pt.Type = objtype.ScriptVarTypeInt
	pt.DefaultInt = int32(-7)
	mc.params[paramID] = pt
	s := newObjNameOrParamState(t, obj, mc)
	s.PushInt(paramID)

	if err := handleObjParam(s); err != nil {
		t.Fatalf("handleObjParam: %v", err)
	}
	if got := s.PopInt(); got != -7 {
		t.Errorf("int default: got %d, want -7 (sign-extended)", got)
	}
}

// TestObjParamStringDefaultFallback pins the string-default branch.
func TestObjParamStringDefaultFallback(t *testing.T) {
	obj := &mockActiveObj{objType: 590, count: 1}
	mc := newTestConfigs()
	ot := objtype.NewObjType(590)
	ot.Params = objtype.ParamMap{} // empty
	mc.objs[590] = ot
	paramID := 10
	pt := objtype.NewParamType(paramID)
	pt.Type = objtype.ScriptVarTypeString
	pt.DefaultString = "def"
	mc.params[paramID] = pt
	s := newObjNameOrParamState(t, obj, mc)
	s.PushInt(paramID)

	if err := handleObjParam(s); err != nil {
		t.Fatalf("handleObjParam: %v", err)
	}
	if got := s.PopString(); got != "def" {
		t.Errorf("string default: got %q, want %q", got, "def")
	}
}

// TestObjParamRequiresActiveObj pins the requireActiveObj guard.
func TestObjParamRequiresActiveObj(t *testing.T) {
	s := newObjNameOrParamState(t, nil, newTestConfigs())
	s.PushInt(7)

	err := handleObjParam(s)
	if err == nil {
		t.Fatal("handleObjParam(nil ActiveObj): want error, got nil")
	}
	if !strings.Contains(err.Error(), "OBJ_PARAM") {
		t.Errorf("err: got %q, want substring %q", err.Error(), "OBJ_PARAM")
	}
}

// TestObjParamRequiresConfigs pins the requireConfigs guard.
func TestObjParamRequiresConfigs(t *testing.T) {
	obj := &mockActiveObj{objType: 590, count: 1}
	s := newObjNameOrParamState(t, obj, nil)
	s.Configs = nil
	s.PushInt(7)

	err := handleObjParam(s)
	if err == nil {
		t.Fatal("handleObjParam(nil Configs): want error, got nil")
	}
	if !strings.Contains(err.Error(), "OBJ_PARAM") {
		t.Errorf("err: got %q, want substring %q", err.Error(), "OBJ_PARAM")
	}
}

// TestObjParamUnknownType pins the unknown-objId guard.
func TestObjParamUnknownType(t *testing.T) {
	obj := &mockActiveObj{objType: 999, count: 1}
	s := newObjNameOrParamState(t, obj, newTestConfigs()) // 999 not registered
	s.PushInt(7)

	err := handleObjParam(s)
	if err == nil {
		t.Fatal("handleObjParam(unknown objId): want error, got nil")
	}
	if !strings.Contains(err.Error(), "unknown obj id") {
		t.Errorf("err: got %q, want substring %q", err.Error(), "unknown obj id")
	}
}

// TestNAI115D1Retirement_ObjTakeItemEmitsPickupWealthEvent pins that
// OBJ_TAKEITEM emits a PICKUP wealth event between invAdd and removeObj.
// Pre-NAI-162 this was skipped (NAI-115-D1 deviation). Mirrors TS
// ObjOps.ts:149-154.
func TestNAI115D1Retirement_ObjTakeItemEmitsPickupWealthEvent(t *testing.T) {
	s := newTestState(minimalScript(OpReturn))
	mp := &mockPlayer{uidValue: 12345}
	s.Self = mp
	s.Pointers |= PtrActivePlayer

	w := &fakeWorldTakeItem{mockWorld: newMockWorld()}
	s.World = w

	mc := newTestInvConfigs()
	invType := objtype.NewInvType(93)
	invType.Size = 28
	invType.Protect = false
	mc.invs[93] = invType
	// Obj with cost so we can verify AccountValue.
	swordGem := objtype.NewObjType(1623)
	swordGem.Name = "Sapphire"
	swordGem.DebugName = "sapphire"
	swordGem.Cost = 150
	mc.objs[1623] = swordGem
	s.Configs = mc

	inv := inventory.New(93, 28, inventory.StackNormal)
	s.Inv = &mockInvLookup{invs: map[int]*inventory.Inventory{93: inv}}

	active := &mockActiveObj{objType: 1623, x: 3200, z: 3200, level: 0, count: 3, reveal: -1}
	s.ActiveObj = active

	s.PushInt(93)

	if err := handleObjTakeItem(s); err != nil {
		t.Fatalf("OBJ_TAKEITEM: unexpected error: %v", err)
	}
	if len(mp.addWealthEventCalls) != 1 {
		t.Fatalf("AddWealthEvent: got %d calls, want 1 (retirement)", len(mp.addWealthEventCalls))
	}
	evt := mp.addWealthEventCalls[0]
	if evt.EventType != WealthEventTypePickup {
		t.Errorf("EventType: got %d, want %d (PICKUP)", evt.EventType, WealthEventTypePickup)
	}
	if evt.AccountValue != 450 { // 3 * 150
		t.Errorf("AccountValue: got %d, want 450 (3*150)", evt.AccountValue)
	}
	if len(evt.AccountItems) != 1 || evt.AccountItems[0].ID != 1623 || evt.AccountItems[0].Count != 3 {
		t.Errorf("AccountItems: got %+v, want [{ID:1623 Count:3 ...}]", evt.AccountItems)
	}
}
