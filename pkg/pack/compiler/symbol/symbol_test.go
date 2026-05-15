// pkg/pack/compiler/symbol/symbol_test.go
package symbol

import (
	"testing"

	typ "github.com/zsrv/goscape/pkg/pack/compiler/type"
)

func TestSymbol_LocalVariableShape(t *testing.T) {
	s := &LocalVariableSymbol{Name: "x", Type: typ.PrimitiveInt}
	if s.SymbolName() != "x" {
		t.Fatalf("SymbolName = %q, want \"x\"", s.SymbolName())
	}
}

func TestSymbol_BasicShape(t *testing.T) {
	s := &BasicSymbol{Name: "Goblin Mail", Type: typ.PrimitiveInt, IsProtected: true}
	if s.SymbolName() != "Goblin Mail" {
		t.Fatalf("SymbolName = %q", s.SymbolName())
	}
	if !s.IsProtected {
		t.Fatal("IsProtected = false, want true")
	}
}

func TestSymbol_ConstantShape(t *testing.T) {
	s := &ConstantSymbol{Name: "MAX_LEVEL", Value: "99"}
	if s.SymbolName() != "MAX_LEVEL" || s.Value != "99" {
		t.Fatalf("ConstantSymbol shape: %+v", s)
	}
}

func TestServerScriptSymbol_Shape(t *testing.T) {
	tg := makeTriggerStub("proc")
	s := &ServerScriptSymbol{
		ScriptSymbolFields: ScriptSymbolFields{
			Trigger:    tg,
			Name:       "foo",
			Parameters: typ.MetaUnit,
			Returns:    typ.MetaUnit,
		},
	}
	if s.SymbolName() != "foo" {
		t.Fatalf("SymbolName = %q", s.SymbolName())
	}
	if !s.IsServerScript() {
		t.Fatal("ServerScriptSymbol.IsServerScript() = false")
	}
}

func TestClientScriptSymbol_NotServer(t *testing.T) {
	s := &ClientScriptSymbol{}
	if s.IsServerScript() {
		t.Fatal("ClientScriptSymbol.IsServerScript() = true")
	}
}

func TestAllSymbols_SatisfyAstSymbolRef(t *testing.T) {
	var _ astSymbolRef = (*LocalVariableSymbol)(nil)
	var _ astSymbolRef = (*BasicSymbol)(nil)
	var _ astSymbolRef = (*ConstantSymbol)(nil)
	var _ astSymbolRef = (*ServerScriptSymbol)(nil)
	var _ astSymbolRef = (*ClientScriptSymbol)(nil)
}

type astSymbolRef interface {
	AsSymbolRef()
}
