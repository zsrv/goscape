// pkg/pack/compiler/type/meta_test.go
package typ

import "testing"

func TestMetaType_Singletons_DistinctRepresentations(t *testing.T) {
	cases := []struct {
		t   Type
		rep string
	}{
		{MetaAny, "any"},
		{MetaNothing, "nothing"},
		{MetaError, "error"},
		{MetaUnit, "unit"},
	}
	for _, c := range cases {
		if got := c.t.Representation(); got != c.rep {
			t.Fatalf("%v.Representation() = %q, want %q", c.t, got, c.rep)
		}
	}
}

func TestMetaType_AnyType_Wrapping(t *testing.T) {
	// TS MetaType.Type(MetaType.Any).representation == 'type'
	w := NewMetaWrapping(MetaAny)
	if got := w.Representation(); got != "type" {
		t.Fatalf("wrap(Any).rep = %q, want %q", got, "type")
	}
}

func TestMetaType_TypeWrapping_NonAny(t *testing.T) {
	// TS MetaType.Type(PrimitiveType.INT).representation == 'type<int>'
	w := NewMetaWrapping(PrimitiveInt)
	if got := w.Representation(); got != "type<int>" {
		t.Fatalf("wrap(Int).rep = %q, want %q", got, "type<int>")
	}
}

func TestMetaType_OptionsNoSwitchNoArrayNoDeclNoParam(t *testing.T) {
	// TS MetaType.ts L18-23: all four flags false on every MetaType instance.
	o := MetaAny.Options()
	if o.AllowSwitch || o.AllowArray || o.AllowDeclaration || o.AllowParameter {
		t.Fatalf("MetaAny.options = %+v, want all-false", o)
	}
}

func TestMetaType_BaseTypeInteger(t *testing.T) {
	b, ok := MetaAny.BaseType()
	if !ok || b != BaseVarInteger {
		t.Fatalf("MetaAny.baseType = (%d, %v), want (Integer, true)", b, ok)
	}
}

func TestMetaType_DefaultValueMinusOne(t *testing.T) {
	if got := MetaAny.DefaultValue(); got != -1 {
		t.Fatalf("MetaAny.defaultValue = %v, want -1", got)
	}
}

func TestMetaType_CodeIsAbsent(t *testing.T) {
	// TS L25-27: throws on `code` access. Goscape returns (_, false).
	if _, ok := MetaAny.Code(); ok {
		t.Fatalf("MetaAny.Code() ok=true, want false")
	}
}

func TestMetaType_SatisfiesAstTypeRef(t *testing.T) {
	var _ astTypeRef = MetaAny
	var _ astTypeRef = NewMetaWrapping(PrimitiveInt)
}
