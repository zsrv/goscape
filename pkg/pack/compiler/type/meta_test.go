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

func TestNewMetaScript_TSParity(t *testing.T) {
	// TS MetaType.Script (MetaType.ts L90-99):
	// - representation = trigger.identifier (NOT a synthesised "script(...)->(...)" form)
	// - options.allowParameter = true (override of MetaType's default false)
	m := NewMetaScript("proc", PrimitiveInt, PrimitiveInt)
	if got, want := m.Representation(), "proc"; got != want {
		t.Fatalf("Representation() = %q, want %q (TS reads trigger.identifier)", got, want)
	}
	if !m.Options().AllowParameter {
		t.Fatalf("Options().AllowParameter = false, want true (TS MetaType.ts L96)")
	}
}

func TestNewMetaScript_SatisfiesAstTypeRef(t *testing.T) {
	var _ astTypeRef = NewMetaScript("proc", PrimitiveInt, PrimitiveInt)
}

func TestIsMetaScript_DiscriminatesAndRecoversComponents(t *testing.T) {
	ms := NewMetaScript("proc", PrimitiveInt, PrimitiveString)
	params, returns, ok := IsMetaScript(ms)
	if !ok {
		t.Fatal("IsMetaScript(MetaScript) ok=false, want true")
	}
	if params != PrimitiveInt {
		t.Fatalf("params = %v, want PrimitiveInt", params)
	}
	if returns != PrimitiveString {
		t.Fatalf("returns = %v, want PrimitiveString", returns)
	}
}

func TestIsMetaScript_RejectsNonScriptTypes(t *testing.T) {
	cases := []Type{
		PrimitiveInt,
		MetaAny,
		MetaUnit,
		NewMetaWrapping(PrimitiveInt),
	}
	for _, c := range cases {
		if _, _, ok := IsMetaScript(c); ok {
			t.Fatalf("IsMetaScript(%v) ok=true, want false", c)
		}
	}
}

// TestMetaHook_Representation verifies the representation string matches
// TS MetaType.Hook constructor: `hook<${transmitListType.representation}>`.
// TS MetaType.ts L103-112, HEAD b8c338801fbb72d294ff9576a58925a8d3f6de47.
func TestMetaHook_Representation(t *testing.T) {
	h := NewMetaHook(PrimitiveInt)
	if got, want := h.Representation(), "hook<int>"; got != want {
		t.Fatalf("Representation() = %q, want %q", got, want)
	}
}

func TestMetaHook_TransmitListAccess(t *testing.T) {
	h := NewMetaHook(PrimitiveInt)
	transmit, ok := IsMetaHook(h)
	if !ok {
		t.Fatal("IsMetaHook(h) = false, want true")
	}
	if transmit != PrimitiveInt {
		t.Fatalf("transmit = %v, want PrimitiveInt", transmit)
	}
}

func TestIsMetaHook_NonHook(t *testing.T) {
	if _, ok := IsMetaHook(MetaAny); ok {
		t.Fatal("IsMetaHook(MetaAny) = true, want false")
	}
	if _, ok := IsMetaHook(PrimitiveInt); ok {
		t.Fatal("IsMetaHook(PrimitiveInt) = true, want false")
	}
}

func TestMetaHook_Options(t *testing.T) {
	h := NewMetaHook(MetaUnit)
	opts := h.Options()
	if opts.AllowSwitch {
		t.Error("AllowSwitch = true, want false")
	}
	if opts.AllowArray {
		t.Error("AllowArray = true, want false")
	}
	if opts.AllowDeclaration {
		t.Error("AllowDeclaration = true, want false")
	}
	if opts.AllowParameter {
		t.Error("AllowParameter = true, want false")
	}
}
