// pkg/pack/compiler/symbol/loader_test.go
package symbol

import (
	"testing"

	typ "github.com/zsrv/goscape/pkg/pack/compiler/type"
)

// TestAddConstant_HappyPath inserts a constant and verifies it can be looked up.
func TestAddConstant_HappyPath(t *testing.T) {
	tab := NewSymbolTable(nil)
	sym, err := AddConstant(tab, "MAX_LEVEL", "99")
	if err != nil {
		t.Fatalf("AddConstant: %v", err)
	}
	if sym.Name != "MAX_LEVEL" || sym.Value != "99" {
		t.Errorf("inserted ConstantSymbol: got %+v", sym)
	}
	got := tab.Find(SymbolTypeConstant(), "MAX_LEVEL")
	if got != sym {
		t.Errorf("Find after AddConstant: got %v, want %v", got, sym)
	}
}

// TestAddConstant_DuplicateReturnsError pins TS L26-28 (Go: returns error).
func TestAddConstant_DuplicateReturnsError(t *testing.T) {
	tab := NewSymbolTable(nil)
	if _, err := AddConstant(tab, "X", "1"); err != nil {
		t.Fatal(err)
	}
	if _, err := AddConstant(tab, "X", "2"); err == nil {
		t.Error("AddConstant duplicate: want error, got nil")
	}
}

// TestAddBasic_HappyPath inserts a basic symbol with type.
func TestAddBasic_HappyPath(t *testing.T) {
	tab := NewSymbolTable(nil)
	sym, err := AddBasic(tab, typ.PrimitiveInt, "foo", false)
	if err != nil {
		t.Fatalf("AddBasic: %v", err)
	}
	if sym.Name != "foo" || sym.Type != typ.PrimitiveInt || sym.IsProtected {
		t.Errorf("inserted BasicSymbol: got %+v", sym)
	}
}

// TestAddBasic_Protected pins isProtected propagation.
func TestAddBasic_Protected(t *testing.T) {
	tab := NewSymbolTable(nil)
	sym, err := AddBasic(tab, typ.PrimitiveInt, "secret", true)
	if err != nil {
		t.Fatal(err)
	}
	if !sym.IsProtected {
		t.Error("AddBasic isProtected=true: want IsProtected=true, got false")
	}
}
