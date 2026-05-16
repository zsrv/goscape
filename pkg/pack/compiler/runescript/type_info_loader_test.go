// pkg/pack/compiler/runescript/type_info_loader_test.go
package runescript

import (
	"testing"

	"github.com/zsrv/goscape/pkg/pack/compiler/symbol"
	typ "github.com/zsrv/goscape/pkg/pack/compiler/type"
)

// stubCompilerContext wraps a TypeManager pre-registered with named types.
type stubCompilerContext struct {
	tm *typ.TypeManager
}

func newStubCompilerContext(entries map[string]typ.Type) *stubCompilerContext {
	tm := typ.NewTypeManager()
	for name, t := range entries {
		if err := tm.Register(name, t); err != nil {
			panic(err)
		}
	}
	return &stubCompilerContext{tm: tm}
}

func (s *stubCompilerContext) Types() *typ.TypeManager { return s.tm }

// TestCompilerTypeInfoConstantLoader_InsertsAll pins TS L13-15.
func TestCompilerTypeInfoConstantLoader_InsertsAll(t *testing.T) {
	tab := symbol.NewSymbolTable(nil)
	info := &CompilerTypeInfo{
		Map: map[string]string{
			"MAX_LEVEL": "99",
			"MIN_LEVEL": "1",
		},
	}
	loader := &CompilerTypeInfoConstantLoader{Symbols: info}
	if err := loader.Load(tab, newStubCompilerContext(nil)); err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got := tab.Find(symbol.SymbolTypeConstant(), "MAX_LEVEL"); got == nil {
		t.Error("MAX_LEVEL not inserted")
	}
	if got := tab.Find(symbol.SymbolTypeConstant(), "MIN_LEVEL"); got == nil {
		t.Error("MIN_LEVEL not inserted")
	}
}

// TestCompilerTypeInfoLoader_Vartype_TupleFromList pins TS L21-24 — comma-
// separated type list becomes a TupleType.
func TestCompilerTypeInfoLoader_Vartype_TupleFromList(t *testing.T) {
	mapper := NewSymbolMapper(nil)
	info := &CompilerTypeInfo{
		Map:     map[string]string{"0": "foo"},
		Vartype: map[string]string{"0": "int,string"},
	}
	ctx := newStubCompilerContext(map[string]typ.Type{
		"int":    typ.PrimitiveInt,
		"string": typ.PrimitiveString,
	})
	loader := &CompilerTypeInfoLoader{
		Mapper:       mapper,
		Symbols:      info,
		TypeSupplier: func(sub typ.Type) typ.Type { return sub },
	}
	tab := symbol.NewSymbolTable(nil)
	if err := loader.Load(tab, ctx); err != nil {
		t.Fatalf("Load: %v", err)
	}
	syms := tab.FindAll("foo")
	if len(syms) == 0 {
		t.Fatal("symbol 'foo' not found")
	}
}

// TestCompilerTypeInfoLoader_NoVartype_DefaultsToUnit pins TS L19 — when
// vartype is missing, subTypes defaults to MetaType.Unit.
func TestCompilerTypeInfoLoader_NoVartype_DefaultsToUnit(t *testing.T) {
	mapper := NewSymbolMapper(nil)
	info := &CompilerTypeInfo{Map: map[string]string{"5": "bar"}}
	var capturedSub typ.Type
	loader := &CompilerTypeInfoLoader{
		Mapper:  mapper,
		Symbols: info,
		TypeSupplier: func(sub typ.Type) typ.Type {
			capturedSub = sub
			return typ.PrimitiveInt
		},
	}
	if err := loader.Load(symbol.NewSymbolTable(nil), newStubCompilerContext(nil)); err != nil {
		t.Fatal(err)
	}
	if capturedSub != typ.MetaUnit {
		t.Errorf("TypeSupplier subtype: got %v, want MetaUnit", capturedSub)
	}
}

// TestCompilerTypeInfoLoader_UnknownTypeNameMapsToError pins TS L23 —
// type lookup that fails resolves to MetaType.Error. With a single-element
// vartype, TupleFromList unwraps to the element directly, so capturedSub is
// MetaError.
func TestCompilerTypeInfoLoader_UnknownTypeNameMapsToError(t *testing.T) {
	mapper := NewSymbolMapper(nil)
	info := &CompilerTypeInfo{
		Map:     map[string]string{"0": "baz"},
		Vartype: map[string]string{"0": "no_such_type"},
	}
	var capturedSub typ.Type
	loader := &CompilerTypeInfoLoader{
		Mapper:  mapper,
		Symbols: info,
		TypeSupplier: func(sub typ.Type) typ.Type {
			capturedSub = sub
			return typ.PrimitiveInt
		},
	}
	if err := loader.Load(symbol.NewSymbolTable(nil), newStubCompilerContext(nil)); err != nil {
		t.Fatal(err)
	}
	if capturedSub != typ.MetaError {
		t.Errorf("unknown type: got %v, want MetaError", capturedSub)
	}
}

// TestCompilerTypeInfoProtectedLoader_PropagatesProtect pins TS L24-29.
func TestCompilerTypeInfoProtectedLoader_PropagatesProtect(t *testing.T) {
	mapper := NewSymbolMapper(nil)
	info := &CompilerTypeInfo{
		Map:     map[string]string{"0": "secret"},
		Protect: map[string]bool{"0": true},
	}
	loader := &CompilerTypeInfoProtectedLoader{
		Mapper:       mapper,
		Symbols:      info,
		TypeSupplier: func(sub typ.Type) typ.Type { return typ.PrimitiveInt },
	}
	tab := symbol.NewSymbolTable(nil)
	if err := loader.Load(tab, newStubCompilerContext(nil)); err != nil {
		t.Fatal(err)
	}
	syms := tab.FindAll("secret")
	if len(syms) == 0 {
		t.Fatal("'secret' not inserted")
	}
	bs, ok := syms[0].(*symbol.BasicSymbol)
	if !ok {
		t.Fatalf("'secret' not a *BasicSymbol: %T", syms[0])
	}
	if !bs.IsProtected {
		t.Error("'secret' IsProtected = false, want true")
	}
}

// TestCompilerTypeInfoLoader_SortedIteration_ByID pins
// NAI-210-D-LOADER-SORTED-ITERATION: loader visits ids in ascending numeric
// order. Verifies the id→symbol round-trip via SymbolMapper.Get after Load
// (every name resolves to the right id regardless of map insertion order).
func TestCompilerTypeInfoLoader_SortedIteration_ByID(t *testing.T) {
	mapper := NewSymbolMapper(nil)
	info := &CompilerTypeInfo{
		Map: map[string]string{
			"10": "ten",
			"2":  "two",
			"5":  "five",
		},
	}
	loader := &CompilerTypeInfoLoader{
		Mapper:  mapper,
		Symbols: info,
		TypeSupplier: func(sub typ.Type) typ.Type {
			return typ.PrimitiveInt
		},
	}
	tab := symbol.NewSymbolTable(nil)
	if err := loader.Load(tab, newStubCompilerContext(nil)); err != nil {
		t.Fatal(err)
	}
	want := map[string]int{"ten": 10, "two": 2, "five": 5}
	for name, expectedID := range want {
		sym := tab.Find(symbol.SymbolTypeBasic(typ.PrimitiveInt), name)
		if sym == nil {
			t.Fatalf("symbol %q not inserted", name)
		}
		got := mapper.Get(sym)
		if got != expectedID {
			t.Errorf("symbol %q mapper.Get = %d, want %d", name, got, expectedID)
		}
	}
}
