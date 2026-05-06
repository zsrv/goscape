package script

import (
	"testing"
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
