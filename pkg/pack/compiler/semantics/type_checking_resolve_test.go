package semantics

import (
	"strings"
	"testing"

	"github.com/zsrv/goscape/pkg/pack/compiler/ast"
	"github.com/zsrv/goscape/pkg/pack/compiler/symbol"
	typ "github.com/zsrv/goscape/pkg/pack/compiler/type"
)

func TestVisitIdentifier_Unresolved(t *testing.T) {
	tc := newBasicCheckingFixture(t)
	id := &ast.Identifier{Text: "missing"}
	tc.Visit(id)
	found := false
	for _, d := range tc.diagnostics.List() {
		if strings.Contains(d.Message, "cannot be resolved") || strings.Contains(d.Message, "resolved") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected MessageGenericUnresolvedSymbol; got %v", tc.diagnostics.List())
	}
	gotType, _ := id.Type.(typ.Type)
	if gotType != typ.MetaError {
		t.Errorf("id.Type = %v, want MetaError", id.Type)
	}
}

func TestSymbolToType_BasicSymbol(t *testing.T) {
	tc := newBasicCheckingFixture(t)
	bs := &symbol.BasicSymbol{Name: "x", Type: typ.PrimitiveInt}
	if got := tc.symbolToType(bs); got != typ.PrimitiveInt {
		t.Errorf("symbolToType(BasicSymbol) = %v, want PrimitiveInt", got)
	}
}

func TestSymbolToType_ConstantSymbolReturnsNil(t *testing.T) {
	tc := newBasicCheckingFixture(t)
	cs := &symbol.ConstantSymbol{Name: "x", Value: "1"}
	if got := tc.symbolToType(cs); got != nil {
		t.Errorf("symbolToType(ConstantSymbol) = %v, want nil", got)
	}
}

func TestSymbolToType_LocalArrayReturnsArray(t *testing.T) {
	tc := newBasicCheckingFixture(t)
	arrT, err := typ.NewArrayType(typ.PrimitiveInt)
	if err != nil {
		t.Fatalf("NewArrayType: %v", err)
	}
	lv := &symbol.LocalVariableSymbol{Name: "a", Type: arrT}
	if got := tc.symbolToType(lv); got != typ.Type(arrT) {
		t.Errorf("symbolToType(local-array) = %v, want %v", got, arrT)
	}
}

func TestSymbolToType_LocalScalarReturnsNil(t *testing.T) {
	tc := newBasicCheckingFixture(t)
	lv := &symbol.LocalVariableSymbol{Name: "x", Type: typ.PrimitiveInt}
	if got := tc.symbolToType(lv); got != nil {
		t.Errorf("symbolToType(local-scalar) = %v, want nil (only arrays are identifier-resolvable)", got)
	}
}

func TestVisitIdentifier_ResolvesBasicSymbol(t *testing.T) {
	tc := newBasicCheckingFixture(t)
	// Register the int type so SymbolTypeBasic uses its representation.
	if err := tc.typeManager.RegisterByRepresentation(typ.PrimitiveInt); err != nil {
		// already registered ok
	}
	bs := &symbol.BasicSymbol{Name: "foo", Type: typ.PrimitiveInt}
	tc.rootTable.Insert(symbol.SymbolTypeBasic(typ.PrimitiveInt), bs)
	id := &ast.Identifier{Text: "foo"}
	tc.Visit(id)
	if got := len(tc.diagnostics.List()); got != 0 {
		t.Errorf("emit count = %d, want 0; diags=%v", got, tc.diagnostics.List())
	}
	if id.Reference != bs {
		t.Errorf("Reference = %v, want %v", id.Reference, bs)
	}
	gotType, _ := id.Type.(typ.Type)
	if gotType != typ.PrimitiveInt {
		t.Errorf("Type = %v, want PrimitiveInt", id.Type)
	}
}
