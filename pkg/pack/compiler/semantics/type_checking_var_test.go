// pkg/pack/compiler/semantics/type_checking_var_test.go
//
// T16 tests: LocalVariableExpression, GameVariableExpression,
// ConstantVariableExpression walker arms.
// TS reference: visitLocalVariableExpression (L909-949),
// visitGameVariableExpression (L950-985),
// visitConstantVariableExpression (L987-1082),
// parseConstantExpression/parseConstantExpressionTree (L1333-1356)
// at HEAD b8c338801fbb72d294ff9576a58925a8d3f6de47.
package semantics

import (
	"strings"
	"testing"

	"github.com/zsrv/goscape/pkg/pack/compiler/ast"
	"github.com/zsrv/goscape/pkg/pack/compiler/diagnostics"
	"github.com/zsrv/goscape/pkg/pack/compiler/lexer"
	"github.com/zsrv/goscape/pkg/pack/compiler/symbol"
	typ "github.com/zsrv/goscape/pkg/pack/compiler/type"
)

func TestVisitLocalVariableExpression_Unresolved(t *testing.T) {
	tc := newBasicCheckingFixture(t)
	lv := &ast.LocalVariableExpression{Name: &ast.Identifier{Text: "x"}}
	tc.Visit(lv)
	found := false
	for _, d := range tc.diagnostics.List() {
		if strings.Contains(d.Message, "cannot be resolved") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected MessageLocalReferenceUnresolved; got %v", tc.diagnostics.List())
	}
}

func TestVisitLocalVariableExpression_HappyPath(t *testing.T) {
	tc := newBasicCheckingFixture(t)
	_ = tc.typeManager.RegisterByRepresentation(typ.PrimitiveInt) // ignore duplicate error
	// Insert a local symbol named "x" of type PrimitiveInt.
	sym := &symbol.LocalVariableSymbol{Name: "x", Type: typ.PrimitiveInt}
	tc.table.Insert(symbol.SymbolTypeLocalVariable(), sym)
	lv := &ast.LocalVariableExpression{Name: &ast.Identifier{Text: "x"}}
	tc.Visit(lv)
	if got := len(tc.diagnostics.List()); got != 0 {
		t.Errorf("emit count = %d, want 0; diags=%v", got, tc.diagnostics.List())
	}
	if lv.Reference != sym {
		t.Errorf("Reference = %v, want %v", lv.Reference, sym)
	}
	gotType, _ := lv.Type.(typ.Type)
	if gotType != typ.PrimitiveInt {
		t.Errorf("Type = %v, want PrimitiveInt", lv.Type)
	}
}

func TestVisitLocalVariableExpression_FeatureDisabledLocal(t *testing.T) {
	tc := newBasicCheckingFixture(t)
	tc.features.DisableProcs = true
	lv := &ast.LocalVariableExpression{Name: &ast.Identifier{Text: "x"}}
	tc.Visit(lv)
	found := false
	for _, d := range tc.diagnostics.List() {
		if strings.Contains(d.Message, "Local variables are disabled") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected MessageFeatureDisabledLocal; got %v", tc.diagnostics.List())
	}
}

func TestVisitLocalVariableExpression_NotArrayButIndexed(t *testing.T) {
	tc := newBasicCheckingFixture(t)
	sym := &symbol.LocalVariableSymbol{Name: "x", Type: typ.PrimitiveInt}
	tc.table.Insert(symbol.SymbolTypeLocalVariable(), sym)
	// Index present means IsArray() == true, but symbol type is scalar.
	lv := &ast.LocalVariableExpression{
		Name:  &ast.Identifier{Text: "x"},
		Index: &ast.IntegerLiteral{Value: 0},
	}
	tc.Visit(lv)
	found := false
	for _, d := range tc.diagnostics.List() {
		if strings.Contains(d.Message, "non-array type") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected MessageLocalReferenceNotArray; got %v", tc.diagnostics.List())
	}
}

func TestVisitLocalVariableExpression_ArrayButNoIndex(t *testing.T) {
	tc := newBasicCheckingFixture(t)
	arrType, err := typ.NewArrayType(typ.PrimitiveInt)
	if err != nil {
		t.Fatalf("NewArrayType: %v", err)
	}
	sym := &symbol.LocalVariableSymbol{Name: "arr", Type: arrType}
	tc.table.Insert(symbol.SymbolTypeLocalVariable(), sym)
	// No index — IsArray() == false, but symbol is array.
	lv := &ast.LocalVariableExpression{Name: &ast.Identifier{Text: "arr"}}
	tc.Visit(lv)
	found := false
	for _, d := range tc.diagnostics.List() {
		if strings.Contains(d.Message, "without specifying the index") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected MessageLocalArrayReferenceNoIndex; got %v", tc.diagnostics.List())
	}
}

func TestVisitGameVariableExpression_Unresolved(t *testing.T) {
	tc := newBasicCheckingFixture(t)
	gv := &ast.GameVariableExpression{Name: &ast.Identifier{Text: "x"}}
	tc.Visit(gv)
	found := false
	for _, d := range tc.diagnostics.List() {
		if strings.Contains(d.Message, "game variable") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected MessageGameReferenceUnresolved; got %v", tc.diagnostics.List())
	}
}

func TestVisitGameVariableExpression_HappyPath(t *testing.T) {
	tc := newBasicCheckingFixture(t)
	varType := typ.NewVarPlayerType(typ.PrimitiveInt)
	sym := &symbol.BasicSymbol{Name: "hp", Type: varType}
	tc.rootTable.Insert(symbol.SymbolTypeBasic(varType), sym)
	gv := &ast.GameVariableExpression{Name: &ast.Identifier{Text: "hp"}}
	tc.Visit(gv)
	if got := len(tc.diagnostics.List()); got != 0 {
		t.Errorf("emit count = %d, want 0; diags=%v", got, tc.diagnostics.List())
	}
	gotType, _ := gv.Type.(typ.Type)
	if gotType != typ.PrimitiveInt {
		t.Errorf("Type = %v, want PrimitiveInt (inner of VarPlayerType)", gv.Type)
	}
}

func TestVisitConstantVariableExpression_UnknownTypeHint(t *testing.T) {
	tc := newBasicCheckingFixture(t)
	cv := &ast.ConstantVariableExpression{Name: &ast.Identifier{Text: "FOO"}}
	// No TypeHint set => unknown type.
	tc.Visit(cv)
	found := false
	for _, d := range tc.diagnostics.List() {
		if strings.Contains(d.Message, "Unable to infer") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected MessageConstantUnknownType; got %v", tc.diagnostics.List())
	}
}

func TestVisitConstantVariableExpression_Unresolved(t *testing.T) {
	tc := newBasicCheckingFixture(t)
	cv := &ast.ConstantVariableExpression{Name: &ast.Identifier{Text: "FOO"}}
	cv.TypeHint = typ.PrimitiveInt
	tc.Visit(cv)
	found := false
	for _, d := range tc.diagnostics.List() {
		if strings.Contains(d.Message, "cannot be resolved") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected MessageConstantReferenceUnresolved; got %v", tc.diagnostics.List())
	}
}

// TestVisitConstantVariableExpression_HappyPath_Resolved verifies that
// visitConstantVariableExpression resolves a constant symbol, sets the
// SubExpression from the parsed value, and does NOT emit any ERROR diagnostics.
// Full type propagation (cv.Type == PrimitiveInt) requires visitIntegerLiteral
// (T17) to set the sub-expression type; that integration is exercised in T17.
//
// NOTE: INFO diagnostics from unhandled node fallbacks (pre-T17) are tolerated
// here because the TDD cycle gates on no ERROR diagnostics, not total silence.
func TestVisitConstantVariableExpression_HappyPath_Resolved(t *testing.T) {
	tc := newBasicCheckingFixture(t)
	_ = tc.typeManager.RegisterByRepresentation(typ.PrimitiveInt) // ignore duplicate error
	sym := &symbol.ConstantSymbol{Name: "FOO", Value: "42"}
	tc.rootTable.Insert(symbol.SymbolTypeConstant(), sym)
	cv := &ast.ConstantVariableExpression{Name: &ast.Identifier{Text: "FOO"}}
	cv.TypeHint = typ.PrimitiveInt
	tc.Visit(cv)
	// Verify no ERROR diagnostics (INFO from unhandled-node fallback tolerated).
	for _, d := range tc.diagnostics.List() {
		if d.Type == diagnostics.DiagnosticError {
			t.Errorf("unexpected ERROR diagnostic: %v", d)
		}
	}
	// SubExpression must be populated (constant was resolved and parsed).
	if cv.SubExpression == nil {
		t.Errorf("SubExpression is nil; diags=%v", tc.diagnostics.List())
	}
}

func TestVisitConstantVariableExpression_CyclicRef(t *testing.T) {
	tc := newBasicCheckingFixture(t)
	_ = tc.typeManager.RegisterByRepresentation(typ.PrimitiveInt)
	sym := &symbol.ConstantSymbol{Name: "FOO", Value: "42"}
	tc.rootTable.Insert(symbol.SymbolTypeConstant(), sym)
	// Manually inject the symbol as "being evaluated" to trigger the cycle check.
	tc.constantsBeingEvaluated[sym] = true
	cv := &ast.ConstantVariableExpression{Name: &ast.Identifier{Text: "FOO"}}
	cv.TypeHint = typ.PrimitiveInt
	tc.Visit(cv)
	found := false
	for _, d := range tc.diagnostics.List() {
		if strings.Contains(d.Message, "Cyclic constant") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected MessageConstantCyclicRef; got %v", tc.diagnostics.List())
	}
}

// TestParseConstantExpression_AppliesCallSiteOffset pins the per-call-site
// source offset behavior. TS parseConstantExpression constructs AstBuilder
// with (source.line - 1, source.column - 1) so the parsed inner expression's
// source maps back to the call site, not to line=1 of the value string.
//
// Regression: without offset application, an inlined `^const` push emits
// PushConstantInt with source.Line=1 (the value-string's intrinsic position),
// inflating the script's LineNumberTable by N extra entries — see
// memory codegen_branch_unsourced_fix for the prior family of this bug.
func TestParseConstantExpression_AppliesCallSiteOffset(t *testing.T) {
	tc := newBasicCheckingFixture(t)
	callSite := lexer.NodeSourceLocation{Name: "caller.rs2", Line: 70, Column: 25}
	expr := tc.parseConstantExpression("0", callSite)
	if expr == nil {
		t.Fatal("parseConstantExpression returned nil for value=\"0\"")
	}
	src := expr.Source()
	if src.Line != 70 {
		t.Errorf("Source.Line: got=%d want=70 (call-site line)", src.Line)
	}
	if src.Column != 25 {
		t.Errorf("Source.Column: got=%d want=25 (call-site column)", src.Column)
	}
	if src.Name != "caller.rs2" {
		t.Errorf("Source.Name: got=%q want=%q", src.Name, "caller.rs2")
	}
}

func TestVisitConstantVariableExpression_ParseError(t *testing.T) {
	tc := newBasicCheckingFixture(t)
	_ = tc.typeManager.RegisterByRepresentation(typ.PrimitiveInt)
	// Value that can't be parsed as an expression.
	sym := &symbol.ConstantSymbol{Name: "BAD", Value: "def_int $broken"}
	tc.rootTable.Insert(symbol.SymbolTypeConstant(), sym)
	cv := &ast.ConstantVariableExpression{Name: &ast.Identifier{Text: "BAD"}}
	cv.TypeHint = typ.PrimitiveInt
	tc.Visit(cv)
	found := false
	for _, d := range tc.diagnostics.List() {
		if strings.Contains(d.Message, "Unable to parse") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected MessageConstantParseError; got %v", tc.diagnostics.List())
	}
}
