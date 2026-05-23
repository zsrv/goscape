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

// TestObjSatisfiesEntityInterface locks in the Slot() + Coords()
// methods required for *Obj to be used as a huntTarget in
// modules/world. The interface assertion is compile-time; the test
// itself just confirms values.
func TestObjSatisfiesEntityInterface(t *testing.T) {
	type entityLike interface {
		Slot() int
		Coords() (x, z, level int)
	}
	var _ entityLike = (*Obj)(nil) // compile-time assertion

	o := NewObj(2, 3094, 3106, LifecycleDespawn, 995, 100)
	if o.Slot() != -1 {
		t.Errorf("Slot: got %d, want -1 (objs are not slot-indexed)", o.Slot())
	}
	x, z, level := o.Coords()
	if x != 3094 || z != 3106 || level != 2 {
		t.Errorf("Coords: got (%d,%d,%d), want (3094,3106,2)", x, z, level)
	}
}

func TestObjIsValid(t *testing.T) {
	o := NewObj(0, 100, 100, LifecycleRespawn, 42, 1)
	if !o.IsValid() {
		t.Error("fresh obj: IsValid = false, want true")
	}
}

func TestObjIsActiveDefaultFalse(t *testing.T) {
	o := NewObj(0, 0, 0, LifecycleRespawn, 1, 1)
	if o.IsActive {
		t.Error("fresh obj must have IsActive=false")
	}
}

func TestObjIsActiveSettable(t *testing.T) {
	o := NewObj(0, 0, 0, LifecycleRespawn, 1, 1)
	o.IsActive = true
	if !o.IsActive {
		t.Error("IsActive must be settable to true")
	}
}

func TestObj_ObjCount(t *testing.T) {
	o := NewObj(0, 3200, 3200, LifecycleRespawn, 558, 7)
	if got := o.ObjCount(); got != 7 {
		t.Errorf("ObjCount: got %d, want 7", got)
	}
}

func TestObj_IsValidFor_Public(t *testing.T) {
	// Public obj (Reveal == -1): valid for any playerUID.
	o := NewObj(0, 3200, 3200, LifecycleRespawn, 558, 1)
	if !o.IsValidFor(12345) {
		t.Errorf("IsValidFor(public obj, any uid): got false, want true")
	}
	if !o.IsValidFor(-1) {
		t.Errorf("IsValidFor(public obj, uid -1): got false, want true")
	}
}

func TestObj_IsValidFor_PrivateSelf(t *testing.T) {
	// Private obj (Reveal > -1) where playerUID matches ReceiverID: valid.
	o := NewObj(0, 3200, 3200, LifecycleDespawn, 558, 1)
	o.Reveal = 50
	o.ReceiverID = 12345
	if !o.IsValidFor(12345) {
		t.Errorf("IsValidFor(private obj, matching uid): got false, want true")
	}
}

func TestObj_IsValidFor_PrivateOther(t *testing.T) {
	// Private obj where playerUID does NOT match: invalid.
	o := NewObj(0, 3200, 3200, LifecycleDespawn, 558, 1)
	o.Reveal = 50
	o.ReceiverID = 12345
	if o.IsValidFor(99999) {
		t.Errorf("IsValidFor(private obj, non-matching uid): got true, want false")
	}
}

func TestObj_IsValidFor_DepletedCount(t *testing.T) {
	// Count < 1: invalid regardless of receiver state.
	o := NewObj(0, 3200, 3200, LifecycleRespawn, 558, 0)
	if o.IsValidFor(12345) {
		t.Errorf("IsValidFor(public obj, count=0): got true, want false")
	}
}

func TestObj_IsValid_NoArg_StillTrue(t *testing.T) {
	// Regression guard: the existing no-arg IsValid() (intrinsic base)
	// must still return true. The polymorphic entity interface at
	// modules/world/movement_consts.go:45-49 depends on this method
	// signature — DO NOT remove or rename.
	o := NewObj(0, 3200, 3200, LifecycleRespawn, 558, 1)
	if !o.IsValid() {
		t.Errorf("IsValid() (no-arg): got false, want true (intrinsic base)")
	}
}

func TestObjDropperAccountIDZeroByDefault(t *testing.T) {
	o := &Obj{}
	if o.DropperAccountID() != 0 {
		t.Fatalf("zero-value DropperAccountID: got %d", o.DropperAccountID())
	}
}

func TestObjDropperAccountIDPersists(t *testing.T) {
	o := &Obj{}
	o.SetDropperAccountID(42)
	if o.DropperAccountID() != 42 {
		t.Fatalf("DropperAccountID: got %d, want 42", o.DropperAccountID())
	}
}
