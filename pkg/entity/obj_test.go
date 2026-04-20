package entity

import "testing"

func TestObjDefaultsArePublicAndUnmodified(t *testing.T) {
	o := NewObj(0, 3094, 3106, LifecycleDespawn, 995, 100)
	if o.ReceiverID != -1 {
		t.Errorf("ReceiverID: got %d, want -1 (public)", o.ReceiverID)
	}
	if o.Reveal != -1 {
		t.Errorf("Reveal: got %d, want -1", o.Reveal)
	}
	if o.LastChange != -1 {
		t.Errorf("LastChange: got %d, want -1", o.LastChange)
	}
	if o.Type != 995 || o.Count != 100 {
		t.Errorf("Type/Count: got (%d,%d), want (995,100)", o.Type, o.Count)
	}
}

func TestObjIsAlways1x1(t *testing.T) {
	o := NewObj(0, 0, 0, LifecycleDespawn, 1, 1)
	if o.Width != 1 || o.Length != 1 {
		t.Errorf("Obj footprint: got (%d,%d), want (1,1)", o.Width, o.Length)
	}
}

func TestObjCarriesEntityFields(t *testing.T) {
	o := NewObj(3, 2000, 3000, LifecycleRespawn, 42, 7)
	if o.Level != 3 || o.X != 2000 || o.Z != 3000 {
		t.Errorf("position: got (%d,%d,%d)", o.Level, o.X, o.Z)
	}
	if o.Lifecycle != LifecycleRespawn {
		t.Errorf("lifecycle: got %v, want Respawn", o.Lifecycle)
	}
}

func TestObjRevealConstantValue(t *testing.T) {
	if ObjReveal != 100 {
		t.Errorf("ObjReveal: got %d, want 100", ObjReveal)
	}
}
