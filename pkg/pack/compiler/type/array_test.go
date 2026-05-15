// pkg/pack/compiler/type/array_test.go
package typ

import "testing"

func TestArrayType_WrapPrimitive(t *testing.T) {
	a, err := NewArrayType(PrimitiveInt)
	if err != nil {
		t.Fatalf("NewArrayType(int): %v", err)
	}
	if got, want := a.Representation(), "intarray"; got != want {
		t.Fatalf("rep = %q, want %q", got, want)
	}
	if a.Inner() != PrimitiveInt {
		t.Fatalf("inner = %v, want PrimitiveInt", a.Inner())
	}
}

func TestArrayType_RejectsNestedArray(t *testing.T) {
	inner, _ := NewArrayType(PrimitiveInt)
	if _, err := NewArrayType(inner); err == nil {
		t.Fatal("NewArrayType(intarray) = nil, want error")
	}
}

func TestArrayType_BaseType(t *testing.T) {
	a, _ := NewArrayType(PrimitiveInt)
	b, ok := a.BaseType()
	if !ok || b != BaseVarInteger {
		t.Fatalf("baseType = (%d, %v), want (Integer, true)", b, ok)
	}
}

func TestArrayType_NoCode_NoDefaultValue(t *testing.T) {
	a, _ := NewArrayType(PrimitiveInt)
	if _, ok := a.Code(); ok {
		t.Fatal("ArrayType.Code() ok=true; want false (TS throws)")
	}
	if a.DefaultValue() != nil {
		t.Fatal("ArrayType.DefaultValue() != nil; want nil (TS throws)")
	}
}

func TestArrayType_OptionsAllowSwitchYesAllowArrayNo(t *testing.T) {
	a, _ := NewArrayType(PrimitiveInt)
	o := a.Options()
	if !o.AllowSwitch || !o.AllowDeclaration || !o.AllowParameter {
		t.Fatalf("ArrayType options: %+v want switch/decl/param all-true", o)
	}
	if o.AllowArray {
		t.Fatalf("ArrayType.options.AllowArray = true; want false (no nested arrays)")
	}
}

func TestArrayType_SatisfiesAstTypeRef(t *testing.T) {
	a, _ := NewArrayType(PrimitiveInt)
	var _ astTypeRef = a
}
