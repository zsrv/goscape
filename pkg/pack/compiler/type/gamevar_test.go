// pkg/pack/compiler/type/gamevar_test.go
package typ

import "testing"

func TestGameVarType_Representations(t *testing.T) {
	cases := []struct {
		ctor func(Type) Type
		want string
	}{
		{func(t Type) Type { return NewVarPlayerType(t) }, "varp<int>"},
		{func(t Type) Type { return NewVarBitType(t) }, "varbit<int>"},
		{func(t Type) Type { return NewVarNpcType(t) }, "varn<int>"},
		{func(t Type) Type { return NewVarSharedType(t) }, "vars<int>"},
	}
	for _, c := range cases {
		got := c.ctor(PrimitiveInt).Representation()
		if got != c.want {
			t.Fatalf("rep = %q, want %q", got, c.want)
		}
	}
}

func TestGameVarType_OptionsAllFalse(t *testing.T) {
	// Per TS GameVarType.ts L13-19: all four AllowX false.
	v := NewVarPlayerType(PrimitiveInt)
	o := v.Options()
	if o.AllowSwitch || o.AllowArray || o.AllowDeclaration || o.AllowParameter {
		t.Fatalf("varp options = %+v; want all-false", o)
	}
}

func TestGameVarType_BaseAndDefault(t *testing.T) {
	v := NewVarPlayerType(PrimitiveInt)
	if dv := v.DefaultValue(); dv != -1 {
		t.Fatalf("default = %v, want -1", dv)
	}
	b, ok := v.BaseType()
	if !ok || b != BaseVarInteger {
		t.Fatalf("baseType = (%d, %v), want (Integer, true)", b, ok)
	}
}

func TestGameVarType_SatisfiesAstTypeRef(t *testing.T) {
	var _ astTypeRef = NewVarPlayerType(PrimitiveInt)
	var _ astTypeRef = NewVarBitType(PrimitiveInt)
	var _ astTypeRef = NewVarNpcType(PrimitiveInt)
	var _ astTypeRef = NewVarSharedType(PrimitiveInt)
}
