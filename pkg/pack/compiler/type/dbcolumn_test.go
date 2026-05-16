// pkg/pack/compiler/type/dbcolumn_test.go
package typ

import "testing"

// TestNewDbColumnType_Representation verifies that the dbColumnType's
// Representation wraps the inner type's representation with "dbcolumn<...>".
func TestNewDbColumnType_Representation(t *testing.T) {
	col := NewDbColumnType(PrimitiveInt)
	want := "dbcolumn<int>"
	if got := col.Representation(); got != want {
		t.Fatalf("Representation() = %q, want %q", got, want)
	}
}

// TestIsDbColumnType_RoundTrip verifies that IsDbColumnType recovers the inner
// type from a dbColumnType and returns ok=true.
func TestIsDbColumnType_RoundTrip(t *testing.T) {
	col := NewDbColumnType(PrimitiveInt)
	inner, ok := IsDbColumnType(col)
	if !ok {
		t.Fatal("IsDbColumnType: want ok=true, got false")
	}
	if inner != PrimitiveInt {
		t.Fatalf("IsDbColumnType inner: want PrimitiveInt, got %v", inner)
	}
}

// TestIsDbColumnType_NonMatch verifies that IsDbColumnType returns ok=false for
// types that are not dbColumnType.
func TestIsDbColumnType_NonMatch(t *testing.T) {
	_, ok := IsDbColumnType(PrimitiveInt)
	if ok {
		t.Fatal("IsDbColumnType(PrimitiveInt): want ok=false, got true")
	}
	_, ok = IsDbColumnType(MetaAny)
	if ok {
		t.Fatal("IsDbColumnType(MetaAny): want ok=false, got true")
	}
}

// TestDbColumnType_BaseType verifies that BaseType() returns BaseVarInteger always
// (TS hardcoded, not delegated to inner). Uses a non-integer inner (PrimitiveString)
// to demonstrate the hardcoding rather than masking it.
func TestDbColumnType_BaseType(t *testing.T) {
	col := NewDbColumnType(PrimitiveString)
	base, ok := col.BaseType()
	if !ok {
		t.Fatal("BaseType() ok: want true, got false")
	}
	if base != BaseVarInteger {
		t.Fatalf("BaseType() base: want BaseVarInteger (%d), got %d", BaseVarInteger, base)
	}
}

// TestDbColumnType_DefaultValue verifies that DefaultValue() returns -1 always
// (TS hardcoded, L15 — not delegated to inner).
func TestDbColumnType_DefaultValue(t *testing.T) {
	col := NewDbColumnType(PrimitiveInt)
	dv := col.DefaultValue()
	if dv != -1 {
		t.Fatalf("DefaultValue(): want -1, got %v (%T)", dv, dv)
	}
}

// TestDbColumnType_AllowParameter verifies that AllowParameter is true.
// TS DbColumnType.ts L24: allowParameter: true.
func TestDbColumnType_AllowParameter(t *testing.T) {
	col := NewDbColumnType(PrimitiveInt)
	if got := col.Options().AllowParameter; !got {
		t.Fatal("AllowParameter: want true, got false")
	}
}

// TestDbColumnType_Code verifies that Code() returns ("", false).
// TS DbColumnType.code is declared but never assigned (always undefined).
func TestDbColumnType_Code(t *testing.T) {
	col := NewDbColumnType(PrimitiveInt)
	code, ok := col.Code()
	if ok {
		t.Fatalf("Code() ok: want false, got true (code=%q)", code)
	}
	if code != "" {
		t.Fatalf("Code() code: want \"\", got %q", code)
	}
}

// TestNewDbColumnType_NilPanics verifies that NewDbColumnType panics on nil inner.
func TestNewDbColumnType_NilPanics(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("NewDbColumnType(nil): want panic, got none")
		}
	}()
	NewDbColumnType(nil)
}

// TestDbColumnType_FullTypeInterface verifies that NewDbColumnType returns a
// value that satisfies the full Type interface without panicking.
func TestDbColumnType_FullTypeInterface(t *testing.T) {
	col := NewDbColumnType(PrimitiveString)
	_ = col.Representation()
	_, _ = col.Code()
	_, _ = col.BaseType()
	_ = col.DefaultValue()
	_ = col.Options()
	col.AsTypeRef()
}

// TestDbColumnType_AsTypeRef is a compile-time guard that dbColumnType
// satisfies ast.TypeRef via the Type interface's AsTypeRef() marker.
func TestDbColumnType_AsTypeRef(t *testing.T) {
	var _ Type = NewDbColumnType(PrimitiveInt)
}
