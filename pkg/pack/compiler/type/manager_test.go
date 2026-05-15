// pkg/pack/compiler/type/manager_test.go
package typ

import (
	"strings"
	"testing"
)

func TestTypeManager_Register_DoubleErrors(t *testing.T) {
	m := NewTypeManager()
	if err := m.Register("int", PrimitiveInt); err != nil {
		t.Fatalf("first Register int: %v", err)
	}
	if err := m.Register("int", PrimitiveInt); err == nil {
		t.Fatal("double Register int: nil err, want collision error")
	}
}

func TestTypeManager_RegisterByRepresentation(t *testing.T) {
	m := NewTypeManager()
	if err := m.RegisterByRepresentation(PrimitiveInt); err != nil {
		t.Fatalf("RegisterByRepresentation(int): %v", err)
	}
	got, err := m.Find("int", false)
	if err != nil {
		t.Fatalf("Find int: %v", err)
	}
	if got != PrimitiveInt {
		t.Fatalf("Find int = %v, want PrimitiveInt", got)
	}
}

func TestTypeManager_FindOrNil_Miss(t *testing.T) {
	m := NewTypeManager()
	if got := m.FindOrNil("doesnotexist", false); got != nil {
		t.Fatalf("FindOrNil miss = %v, want nil", got)
	}
}

func TestTypeManager_Find_ErrorOnMiss(t *testing.T) {
	m := NewTypeManager()
	if _, err := m.Find("doesnotexist", false); err == nil {
		t.Fatal("Find miss: nil err, want error")
	} else if !strings.Contains(err.Error(), "doesnotexist") {
		t.Fatalf("Find miss err = %v; want to mention 'doesnotexist'", err)
	}
}

func TestTypeManager_AllowArray_WrapsBaseType(t *testing.T) {
	m := NewTypeManager()
	_ = m.RegisterByRepresentation(PrimitiveInt)
	got := m.FindOrNil("intarray", true)
	if got == nil {
		t.Fatal("FindOrNil intarray (allowArray=true) = nil")
	}
	a, ok := got.(*ArrayType)
	if !ok {
		t.Fatalf("intarray result type = %T, want *ArrayType", got)
	}
	if a.Inner() != PrimitiveInt {
		t.Fatalf("intarray.Inner = %v, want PrimitiveInt", a.Inner())
	}
}

func TestTypeManager_AllowArray_RejectsForOptionsAllowArrayFalse(t *testing.T) {
	// STRING disables AllowArray in its TypeOptions (see TS PrimitiveType L46-48).
	m := NewTypeManager()
	_ = m.RegisterByRepresentation(PrimitiveString)
	got := m.FindOrNil("stringarray", true)
	if got != nil {
		t.Fatalf("FindOrNil stringarray = %v, want nil (string.allowArray=false)", got)
	}
}

func TestTypeManager_AllowArray_RejectsForMissingBase(t *testing.T) {
	m := NewTypeManager()
	got := m.FindOrNil("zarray", true)
	if got != nil {
		t.Fatalf("FindOrNil zarray with no base registered = %v, want nil", got)
	}
}

func TestTypeManager_ChangeOptions_MutatesInPlace(t *testing.T) {
	m := NewTypeManager()
	custom := newPrimitiveType("CUSTOM", "x", BaseVarInteger, -1)
	_ = m.RegisterByRepresentation(custom)
	if err := m.ChangeOptions("custom", func(o *TypeOptions) {
		o.AllowSwitch = false
	}); err != nil {
		t.Fatalf("ChangeOptions: %v", err)
	}
	got, _ := m.Find("custom", false)
	if got.Options().AllowSwitch {
		t.Fatalf("after ChangeOptions: AllowSwitch still true")
	}
}

func TestTypeManager_RegisterNew_RoundTrip(t *testing.T) {
	m := NewTypeManager()
	got, err := m.RegisterNew("widget", "w", BaseVarInteger, -1, func(o *TypeOptions) {
		o.AllowArray = false
	})
	if err != nil {
		t.Fatalf("RegisterNew: %v", err)
	}
	if got.Representation() != "widget" {
		t.Fatalf("rep = %q, want %q", got.Representation(), "widget")
	}
	c, ok := got.Code()
	if !ok || c != "w" {
		t.Fatalf("code = (%q, %v), want (\"w\", true)", c, ok)
	}
	if got.Options().AllowArray {
		t.Fatal("widget AllowArray = true, want false")
	}
	resolved, _ := m.Find("widget", false)
	if resolved.Representation() != "widget" {
		t.Fatalf("Find rep mismatch")
	}
}

func TestTypeManager_Check_RegisteredCheckerFires(t *testing.T) {
	m := NewTypeManager()
	m.AddTypeChecker(func(left, right Type) bool {
		return left == PrimitiveInt && right == PrimitiveBoolean
	})
	if !m.Check(PrimitiveInt, PrimitiveBoolean) {
		t.Fatal("Check(int, boolean): false, want true")
	}
	if m.Check(PrimitiveBoolean, PrimitiveInt) {
		t.Fatal("Check(boolean, int): true, want false")
	}
}

func TestTypeManager_Check_EmptyChain(t *testing.T) {
	m := NewTypeManager()
	if m.Check(PrimitiveInt, PrimitiveInt) {
		t.Fatal("Check on empty checker chain: true, want false")
	}
}
