// pkg/pack/compiler/semantics/type_checking_decl_test.go
//
// T10 tests: DeclarationStatement + ArrayDeclarationStatement walker arms.
// TS reference: visitDeclarationStatement (L380-422) and
// visitArrayDeclarationStatement (L423-472) at HEAD
// b8c338801fbb72d294ff9576a58925a8d3f6de47.
package semantics

import (
	"strings"
	"testing"

	"github.com/zsrv/goscape/pkg/pack/compiler/ast"
	"github.com/zsrv/goscape/pkg/pack/compiler/diagnostics"
	"github.com/zsrv/goscape/pkg/pack/compiler/symbol"
	typ "github.com/zsrv/goscape/pkg/pack/compiler/type"
)

// registerIntForDecl registers PrimitiveInt so typeManager.FindOrNil("int")
// returns it. RegisterByRepresentation stores under Representation() == "int".
func registerIntForDecl(t *testing.T, tc *TypeChecker) {
	t.Helper()
	_ = tc.typeManager.RegisterByRepresentation(typ.PrimitiveInt) // ignore duplicate error
}

func TestVisitDeclaration_FeatureDisabledLocal(t *testing.T) {
	tc := newBasicCheckingFixture(t)
	registerIntForDecl(t, tc)
	tc.features.DisableProcs = true
	tc.currentScript = &ast.Script{}
	tc.atScriptTopLevel = true
	d := &ast.DeclarationStatement{
		TypeToken: &ast.Token{Text: "def_int"},
		Name:      &ast.Identifier{Text: "x"},
	}
	tc.Visit(d)
	found := false
	for _, diag := range tc.diagnostics.List() {
		if diag.Message == diagnostics.MessageFeatureDisabledLocal {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected MessageFeatureDisabledLocal; got %v", tc.diagnostics.List())
	}
}

func TestVisitDeclaration_TopLevelDefOnly_NestedDeclRejected(t *testing.T) {
	tc := newBasicCheckingFixture(t)
	registerIntForDecl(t, tc)
	tc.features.TopLevelDefOnly = true
	tc.currentScript = &ast.Script{}
	tc.atScriptTopLevel = false
	d := &ast.DeclarationStatement{
		TypeToken: &ast.Token{Text: "def_int"},
		Name:      &ast.Identifier{Text: "x"},
	}
	tc.Visit(d)
	found := false
	for _, diag := range tc.diagnostics.List() {
		if strings.Contains(diag.Message, "Local variables may only") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected MessageLocalDeclarationNotTopLevel; got %v", tc.diagnostics.List())
	}
}

func TestVisitDeclaration_HappyPath(t *testing.T) {
	tc := newBasicCheckingFixture(t)
	registerIntForDecl(t, tc)
	tc.currentScript = &ast.Script{}
	tc.atScriptTopLevel = true
	// No Initializer — avoids visiting an IntegerLiteral whose arm lands in
	// T17. The symbol-registration and zero-diagnostic invariants are the
	// focus of this test.
	d := &ast.DeclarationStatement{
		TypeToken: &ast.Token{Text: "def_int"},
		Name:      &ast.Identifier{Text: "x"},
	}
	tc.Visit(d)
	if got := len(tc.diagnostics.List()); got != 0 {
		t.Fatalf("emit count = %d, want 0; diags=%v", got, tc.diagnostics.List())
	}
	if d.Symbol == nil {
		t.Fatal("DeclarationStatement.Symbol should be populated")
	}
	sym, ok := d.Symbol.(*symbol.LocalVariableSymbol)
	if !ok {
		t.Fatalf("Symbol type = %T, want *symbol.LocalVariableSymbol", d.Symbol)
	}
	if sym.Name != "x" {
		t.Errorf("symbol name = %q, want %q", sym.Name, "x")
	}
}

func TestVisitArrayDeclaration_HappyPath(t *testing.T) {
	tc := newBasicCheckingFixture(t)
	registerIntForDecl(t, tc)
	tc.currentScript = &ast.Script{}
	tc.atScriptTopLevel = true
	// No Initializer — the size-expression arm lands in T17 (IntegerLiteral).
	// Focus here is symbol-registration and ArrayType wrapping.
	d := &ast.ArrayDeclarationStatement{
		TypeToken: &ast.Token{Text: "def_int"},
		Name:      &ast.Identifier{Text: "arr"},
	}
	tc.Visit(d)
	if got := len(tc.diagnostics.List()); got != 0 {
		t.Fatalf("emit count = %d, want 0; diags=%v", got, tc.diagnostics.List())
	}
	if d.Symbol == nil {
		t.Fatal("ArrayDeclarationStatement.Symbol should be populated")
	}
	sym, ok := d.Symbol.(*symbol.LocalVariableSymbol)
	if !ok {
		t.Fatalf("Symbol type = %T, want *symbol.LocalVariableSymbol", d.Symbol)
	}
	if _, isArr := sym.Type.(*typ.ArrayType); !isArr {
		t.Errorf("symbol.Type = %T, want *typ.ArrayType", sym.Type)
	}
}

func TestVisitDeclaration_Redeclaration(t *testing.T) {
	tc := newBasicCheckingFixture(t)
	registerIntForDecl(t, tc)
	tc.currentScript = &ast.Script{}
	tc.atScriptTopLevel = true

	d1 := &ast.DeclarationStatement{
		TypeToken: &ast.Token{Text: "def_int"},
		Name:      &ast.Identifier{Text: "x"},
	}
	tc.Visit(d1)
	d2 := &ast.DeclarationStatement{
		TypeToken: &ast.Token{Text: "def_int"},
		Name:      &ast.Identifier{Text: "x"}, // duplicate
	}
	tc.Visit(d2)
	found := false
	for _, diag := range tc.diagnostics.List() {
		if strings.Contains(diag.Message, "already defined") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected redeclaration diagnostic; got %v", tc.diagnostics.List())
	}
}
