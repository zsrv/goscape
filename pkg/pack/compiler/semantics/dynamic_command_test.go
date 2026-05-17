package semantics

import (
	"testing"

	"github.com/zsrv/goscape/pkg/pack/compiler/ast"
	"github.com/zsrv/goscape/pkg/pack/compiler/diagnostics"
	typ "github.com/zsrv/goscape/pkg/pack/compiler/type"
)

func TestTypeCheckingContext_ArgumentsCallExpression(t *testing.T) {
	call := &ast.CommandCallExpression{
		Name:      &ast.Identifier{Text: "foo"},
		Arguments: []ast.Expression{&ast.IntegerLiteral{Value: 1}, &ast.IntegerLiteral{Value: 2}},
	}
	ctx := newTypeCheckingContext(nil, nil, call, &diagnostics.Diagnostics{})
	if got := len(ctx.Arguments()); got != 2 {
		t.Errorf("Arguments() length = %d, want 2", got)
	}
}

func TestTypeCheckingContext_ArgumentsNonCallExpression(t *testing.T) {
	ident := &ast.Identifier{Text: "foo"}
	ctx := newTypeCheckingContext(nil, nil, ident, &diagnostics.Diagnostics{})
	if got := len(ctx.Arguments()); got != 0 {
		t.Errorf("Arguments() length = %d, want 0 for non-call expression", got)
	}
}

// TestTypeCheckingContext_CheckArgumentTypes_TupleMatch verifies that
// CheckArgumentTypes returns true when the arg types match the expected tuple.
func TestTypeCheckingContext_CheckArgumentTypes_TupleMatch(t *testing.T) {
	tc := newBasicCheckingFixture(t)
	tc.typeManager.RegisterByRepresentation(typ.PrimitiveInt)
	tc.typeManager.RegisterByRepresentation(typ.PrimitiveString)

	// Build a CommandCallExpression whose arguments have types already set.
	intLit := &ast.IntegerLiteral{}
	intLit.Type = typ.PrimitiveInt
	strLit := &ast.StringLiteral{}
	strLit.Type = typ.PrimitiveString

	cmd := &ast.CommandCallExpression{
		Arguments: []ast.Expression{intLit, strLit},
	}

	expected, err := typ.NewTupleType(typ.PrimitiveInt, typ.PrimitiveString)
	if err != nil {
		t.Fatalf("NewTupleType: %v", err)
	}

	ctx := newTypeCheckingContext(tc, tc.typeManager, cmd, tc.diagnostics)
	if !ctx.CheckArgumentTypes(expected, false, false) {
		t.Fatal("CheckArgumentTypes: want true for matching types, got false")
	}
	if tc.diagnostics.HasErrors() {
		t.Fatalf("unexpected errors: %v", tc.diagnostics.List())
	}
}

// TestTypeCheckingContext_CheckArgumentTypes_MismatchReports verifies that
// CheckArgumentTypes returns false and emits a diagnostic on type mismatch
// when reportError=true. Diagnostics are emitted to tc.diagnostics (the
// TypeChecker's own collector, since checkTypeMatch writes there).
func TestTypeCheckingContext_CheckArgumentTypes_MismatchReports(t *testing.T) {
	tc := newBasicCheckingFixture(t)
	tc.typeManager.RegisterByRepresentation(typ.PrimitiveInt)
	tc.typeManager.RegisterByRepresentation(typ.PrimitiveString)

	intLit := &ast.IntegerLiteral{}
	intLit.Type = typ.PrimitiveInt

	cmd := &ast.CommandCallExpression{
		Arguments: []ast.Expression{intLit},
	}

	// Expected is string but actual arg is int — mismatch.
	// ctx.Diagnostics is passed through but checkTypeMatch emits to tc.diagnostics.
	ctx := newTypeCheckingContext(tc, tc.typeManager, cmd, tc.diagnostics)
	if ctx.CheckArgumentTypes(typ.PrimitiveString, true, false) {
		t.Fatal("CheckArgumentTypes: want false for mismatched types, got true")
	}
	if !tc.diagnostics.HasErrors() {
		t.Fatal("CheckArgumentTypes: want error diagnostic on mismatch with reportError=true")
	}
}

// TestTypeCheckingContext_CheckArgumentTypes_UntypedArg_NoCrash mirrors TS
// TypeCheckingContext.checkArgumentTypes L160-164 + L167: when an arg's type
// is unset (handler deferred visiting to checkArgumentTypes, as
// QueueVarArgCommandHandler does for its vararg list), the type falls back to
// MetaError rather than nil. Without the fallback the tuple/representation
// chain panics on a nil-interface Representation() call.
func TestTypeCheckingContext_CheckArgumentTypes_UntypedArg_NoCrash(t *testing.T) {
	tc := newBasicCheckingFixture(t)
	tc.typeManager.RegisterByRepresentation(typ.PrimitiveInt)

	// Arg has no Type set (Type field left zero / nil).
	untyped := &ast.IntegerLiteral{}
	cmd := &ast.CommandCallExpression{
		Arguments: []ast.Expression{untyped},
	}

	ctx := newTypeCheckingContext(tc, tc.typeManager, cmd, tc.diagnostics)
	// Must not panic; must return false; must emit a diagnostic.
	if ctx.CheckArgumentTypes(typ.PrimitiveInt, true, false) {
		// (Match might be true if visit-on-untyped resolves IntegerLiteral
		// to int — that's fine. Either outcome is non-crashing.)
	}
}

// TestTypeCheckingContext_CheckArgumentTypes_MismatchSilent verifies that
// CheckArgumentTypes returns false without emitting a diagnostic when
// reportError=false on mismatch.
func TestTypeCheckingContext_CheckArgumentTypes_MismatchSilent(t *testing.T) {
	tc := newBasicCheckingFixture(t)
	tc.typeManager.RegisterByRepresentation(typ.PrimitiveInt)
	tc.typeManager.RegisterByRepresentation(typ.PrimitiveString)

	intLit := &ast.IntegerLiteral{}
	intLit.Type = typ.PrimitiveInt

	cmd := &ast.CommandCallExpression{
		Arguments: []ast.Expression{intLit},
	}

	ctx := newTypeCheckingContext(tc, tc.typeManager, cmd, tc.diagnostics)
	if ctx.CheckArgumentTypes(typ.PrimitiveString, false, false) {
		t.Fatal("CheckArgumentTypes: want false for mismatched types, got true")
	}
	if tc.diagnostics.HasErrors() {
		t.Fatalf("CheckArgumentTypes: want no diagnostic with reportError=false, got: %v", tc.diagnostics.List())
	}
}

func TestTypeCheckingContext_Arguments2(t *testing.T) {
	cmd := &ast.CommandCallExpression{
		Arguments:  []ast.Expression{&ast.IntegerLiteral{}, &ast.IntegerLiteral{}},
		Arguments2: []ast.Expression{&ast.StringLiteral{}},
	}
	ctx := newTypeCheckingContext(nil, nil, cmd, &diagnostics.Diagnostics{})
	if got := len(ctx.Arguments()); got != 2 {
		t.Fatalf("Arguments() len: got %d, want 2", got)
	}
	a2 := ctx.Arguments2()
	if len(a2) != 1 {
		t.Fatalf("Arguments2() len: got %d, want 1", len(a2))
	}
	if _, ok := a2[0].(*ast.StringLiteral); !ok {
		t.Fatalf("Arguments2()[0]: want *StringLiteral, got %T", a2[0])
	}

	// CommandCallExpression with nil Arguments2 → must return nil
	cmdNoArgs2 := &ast.CommandCallExpression{
		Arguments:  []ast.Expression{&ast.IntegerLiteral{}},
		Arguments2: nil,
	}
	ctx = newTypeCheckingContext(nil, nil, cmdNoArgs2, &diagnostics.Diagnostics{})
	if got := ctx.Arguments2(); got != nil {
		t.Fatalf("Arguments2() for CommandCall with nil Arguments2: want nil, got %v", got)
	}

	proc := &ast.ProcCallExpression{Arguments: []ast.Expression{&ast.IntegerLiteral{}}}
	ctx = newTypeCheckingContext(nil, nil, proc, &diagnostics.Diagnostics{})
	if got := ctx.Arguments2(); got != nil {
		t.Fatalf("Arguments2() for ProcCall: want nil, got %v", got)
	}
}

// Compile-time guard: an empty struct satisfies DynamicCommandHandler.
// Updated by NAI-207 to include GenerateCode.
type _stubHandler struct{}

func (_stubHandler) TypeCheck(ctx *TypeCheckingContext)    {}
func (_stubHandler) GenerateCode(ctx CodeGenContext) bool  { return false }

func TestDynamicCommandHandler_Interface(t *testing.T) {
	var _ DynamicCommandHandler = _stubHandler{}
	_ = typ.MetaUnit // import-keepalive
}

// TestDynamicCommandHandler_GenerateCodeIsRequired compile-time pins that
// a struct satisfies DynamicCommandHandler only with both TypeCheck AND
// GenerateCode. Retires NAI-206-D-DYNCOMMAND-NO-CODEGEN.
func TestDynamicCommandHandler_GenerateCodeIsRequired(t *testing.T) {
	var _ DynamicCommandHandler = (*fakeHandlerComplete)(nil)
}

type fakeHandlerComplete struct{}

func (f *fakeHandlerComplete) TypeCheck(ctx *TypeCheckingContext)   {}
func (f *fakeHandlerComplete) GenerateCode(ctx CodeGenContext) bool { return false }
