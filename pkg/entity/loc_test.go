package entity

import "testing"

func TestLocInfoRoundTrip(t *testing.T) {
	l := NewLoc(0, 3094, 3106, 1, 1, LifecycleForever, 5000, 10, 3)
	if l.Type() != 5000 {
		t.Errorf("Type: got %d, want 5000", l.Type())
	}
	if l.Shape() != 10 {
		t.Errorf("Shape: got %d, want 10", l.Shape())
	}
	if l.Angle() != 3 {
		t.Errorf("Angle: got %d, want 3", l.Angle())
	}
}

func TestLocInfoBoundaryValues(t *testing.T) {
	// shape=22 (ShapeGroundDecor) is the highest valid LocShape; shape=31
	// would be out-of-range and panics in LayerOf (matching TS locShapeLayer).
	l := NewLoc(0, 0, 0, 1, 1, LifecycleForever, 0x3FFF, 22, 0x3)
	if l.Type() != 0x3FFF {
		t.Errorf("Type at max: got %d, want %d", l.Type(), 0x3FFF)
	}
	if l.Shape() != 22 {
		t.Errorf("Shape at max valid: got %d, want 22 (ShapeGroundDecor)", l.Shape())
	}
	if l.Angle() != 0x3 {
		t.Errorf("Angle at max: got %d, want %d", l.Angle(), 0x3)
	}
}

func TestLocInfoOverflowSilentlyMasks(t *testing.T) {
	l := NewLoc(0, 0, 0, 1, 1, LifecycleForever, 0x4001, 0x20, 0x4)
	if l.Type() != 1 {
		t.Errorf("Type=0x4001 should mask to 1; got %d", l.Type())
	}
	if l.Shape() != 0 {
		t.Errorf("Shape=0x20 should mask to 0; got %d", l.Shape())
	}
	if l.Angle() != 0 {
		t.Errorf("Angle=0x4 should mask to 0; got %d", l.Angle())
	}
}

func TestLocCarriesEntityFields(t *testing.T) {
	l := NewLoc(2, 3094, 3106, 3, 2, LifecycleDespawn, 100, 0, 0)
	if l.Level != 2 || l.X != 3094 || l.Z != 3106 {
		t.Errorf("position: got (%d,%d,%d)", l.Level, l.X, l.Z)
	}
	if l.Width != 3 || l.Length != 2 {
		t.Errorf("size: got (%d,%d), want (3,2)", l.Width, l.Length)
	}
	if l.Lifecycle != LifecycleDespawn {
		t.Errorf("lifecycle: got %v, want Despawn", l.Lifecycle)
	}
}

func TestLocSlotReturnsMinusOne(t *testing.T) {
	l := NewLoc(0, 100, 100, 1, 1, LifecycleForever, 0, 10, 0)
	if got := l.Slot(); got != -1 {
		t.Errorf("Loc.Slot(): got %d, want -1", got)
	}
}

func TestLocCoordsReturnsXZLevel(t *testing.T) {
	l := NewLoc(2, 3245, 3198, 1, 1, LifecycleForever, 0, 10, 0)
	x, z, level := l.Coords()
	if x != 3245 || z != 3198 || level != 2 {
		t.Errorf("Loc.Coords(): got (%d, %d, %d), want (3245, 3198, 2)", x, z, level)
	}
}

func TestLocIsValid(t *testing.T) {
	l := NewLoc(0, 100, 100, 1, 1, LifecycleRespawn, 42, 10, 0)
	if !l.IsValid() {
		t.Error("fresh loc: IsValid = false, want true")
	}
}

func TestLocChangeMutatesCurrentInfoOnly(t *testing.T) {
	l := NewLoc(0, 100, 200, 1, 1, LifecycleRespawn, 42, 0, 0)
	baseBefore := l.BaseInfo
	l.Change(99, 0, 1)
	if l.BaseInfo != baseBefore {
		t.Errorf("BaseInfo mutated: got %d, want %d", l.BaseInfo, baseBefore)
	}
	if l.Type() != 99 {
		t.Errorf("Type after Change: got %d, want 99", l.Type())
	}
	if l.Angle() != 1 {
		t.Errorf("Angle after Change: got %d, want 1", l.Angle())
	}
}

func TestLocRevertRestoresBaseInfo(t *testing.T) {
	l := NewLoc(0, 100, 200, 1, 1, LifecycleRespawn, 42, 0, 0)
	l.Change(99, 0, 1)
	l.Revert()
	if l.CurrentInfo != l.BaseInfo {
		t.Errorf("Revert: CurrentInfo=%d BaseInfo=%d", l.CurrentInfo, l.BaseInfo)
	}
	if l.Type() != 42 {
		t.Errorf("Type after Revert: got %d, want 42", l.Type())
	}
}

func TestLocIsChangedReflectsBaseDelta(t *testing.T) {
	l := NewLoc(0, 100, 200, 1, 1, LifecycleRespawn, 42, 0, 0)
	if l.IsChanged() {
		t.Error("fresh loc must report IsChanged=false")
	}
	l.Change(99, 0, 0)
	if !l.IsChanged() {
		t.Error("after Change with new type, IsChanged must be true")
	}
	l.Revert()
	if l.IsChanged() {
		t.Error("after Revert, IsChanged must be false")
	}
}

func TestLocLayerReadsFromBaseInfo(t *testing.T) {
	// shape=0 (ShapeWallStraight) → LayerWall (0)
	l := NewLoc(0, 100, 200, 1, 1, LifecycleRespawn, 42, 0, 0)
	if l.Layer() != 0 {
		t.Errorf("Layer for shape=0: got %d, want 0 (LayerWall)", l.Layer())
	}
	// Change shape; Layer reads BaseInfo so must be unaffected
	l.Change(42, 22, 0) // ShapeGroundDecor (LayerGroundDecor=3)
	if l.Layer() != 0 {
		t.Errorf("Layer after Change of shape: got %d, want 0 (BaseInfo unchanged)", l.Layer())
	}
}

func TestLocIsActiveDefaultFalse(t *testing.T) {
	l := NewLoc(0, 100, 200, 1, 1, LifecycleDespawn, 42, 0, 0)
	if l.IsActive {
		t.Error("fresh loc must have IsActive=false")
	}
}
