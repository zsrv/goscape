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
	s := newTestState(minimalScript(OpReturn))
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
	s := newTestState(minimalScript(OpReturn))
	if err := handleObjCoord(s); err == nil {
		t.Errorf("OBJ_COORD: expected error on nil ActiveObj, got nil")
	}
}
