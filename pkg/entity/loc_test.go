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
	l := NewLoc(0, 0, 0, 1, 1, LifecycleForever, 0x3FFF, 0x1F, 0x3)
	if l.Type() != 0x3FFF {
		t.Errorf("Type at max: got %d, want %d", l.Type(), 0x3FFF)
	}
	if l.Shape() != 0x1F {
		t.Errorf("Shape at max: got %d, want %d", l.Shape(), 0x1F)
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
