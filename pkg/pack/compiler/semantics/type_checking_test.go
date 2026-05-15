package semantics

import (
	"testing"

	"github.com/zsrv/goscape/pkg/pack/compiler/ast"
	"github.com/zsrv/goscape/pkg/pack/compiler/diagnostics"
	"github.com/zsrv/goscape/pkg/pack/compiler/symbol"
	"github.com/zsrv/goscape/pkg/pack/compiler/trigger"
	typ "github.com/zsrv/goscape/pkg/pack/compiler/type"
)

// newBasicCheckingFixture builds a TypeChecker with empty triggers/symbols
// and a single identity type-checker (left == right ⇒ assignable).
// Helper for T7+ tests.
func newBasicCheckingFixture(t *testing.T) *TypeChecker {
	t.Helper()
	tm := typ.NewTypeManager()
	// Install an identity checker so that identical types are assignable.
	// TS TypeManager.check has the same blank-slate behaviour (no built-in
	// identity check); production code always adds at least one checker
	// via the compiler configuration. Tests need at minimum identity.
	tm.AddTypeChecker(func(left, right typ.Type) bool { return left == right })
	trm := trigger.NewTriggerManager()
	root := symbol.NewSymbolTable(nil)
	d := &diagnostics.Diagnostics{}
	return NewTypeChecker(tm, trm, root, map[string]DynamicCommandHandler{}, d, StrictFeatureLevel{})
}

func TestTypeChecker_ScopedSavesAndRestoresTable(t *testing.T) {
	tc := newBasicCheckingFixture(t)
	root := tc.table
	sub := root.CreateSubTable()
	called := false
	tc.scoped(sub, func() {
		called = true
		if tc.table != sub {
			t.Error("inside scoped(): expected tc.table == sub")
		}
	})
	if !called {
		t.Fatal("scoped() did not invoke the function")
	}
	if tc.table != root {
		t.Error("after scoped(): expected tc.table restored to root")
	}
}

func TestTypeChecker_IsDisabledTypeName(t *testing.T) {
	tc := newBasicCheckingFixture(t)
	tc.features.DisableBooleans = true
	if !tc.isDisabledTypeName("boolean") {
		t.Error("'boolean' should be disabled with DisableBooleans=true")
	}
	if tc.isDisabledTypeName("int") {
		t.Error("'int' is never disabled")
	}
	tc.features.DisableBooleans = false
	tc.features.DisableDBTables = true
	if !tc.isDisabledTypeName("dbtable") {
		t.Error("'dbtable' should be disabled with DisableDBTables=true")
	}
	if !tc.isDisabledTypeName("dbrowarray") {
		t.Error("'dbrowarray' should be disabled (array suffix stripped) with DisableDBTables=true")
	}
}

func TestTypeChecker_CheckTypeMatch_TupleSelfMatch(t *testing.T) {
	tc := newBasicCheckingFixture(t)
	expected, err := typ.NewTupleType(typ.PrimitiveInt, typ.PrimitiveString)
	if err != nil {
		t.Fatalf("NewTupleType: %v", err)
	}
	actual, err := typ.NewTupleType(typ.PrimitiveInt, typ.PrimitiveString)
	if err != nil {
		t.Fatalf("NewTupleType: %v", err)
	}
	node := &ast.IntegerLiteral{Value: 0}
	if !tc.checkTypeMatch(node, expected, actual, false) {
		t.Error("tuple matching itself should return true")
	}
}

func TestTypeChecker_CheckTypeMatch_LengthMismatchReportsError(t *testing.T) {
	tc := newBasicCheckingFixture(t)
	// Compare a single scalar (PrimitiveInt) against a 2-element tuple
	// (int, string) — lengths differ, so checkTypeMatch must return false
	// and emit exactly one error diagnostic.
	// NAI-206-D-TUPLE-SINGLE-SCALAR: NewTupleType requires ≥2 children;
	// for a 1-element "tuple" we use the scalar type directly (which
	// flattenTuple wraps as [PrimitiveInt]).
	expected := typ.PrimitiveInt // scalar → flattenTuple yields [int]
	actual, err := typ.NewTupleType(typ.PrimitiveInt, typ.PrimitiveString)
	if err != nil {
		t.Fatalf("NewTupleType: %v", err)
	}
	node := &ast.IntegerLiteral{Value: 0}
	if tc.checkTypeMatch(node, expected, actual, true) {
		t.Error("length-mismatched tuples should not match")
	}
	if got := len(tc.diagnostics.List()); got != 1 {
		t.Errorf("diagnostics emitted = %d, want 1", got)
	}
}

func TestTypeChecker_CheckTypeMatchAny(t *testing.T) {
	tc := newBasicCheckingFixture(t)
	expected := []typ.Type{typ.PrimitiveInt, typ.PrimitiveLong}
	if !tc.checkTypeMatchAny(&ast.IntegerLiteral{}, expected, typ.PrimitiveInt) {
		t.Error("int should match one of [int, long]")
	}
	if tc.checkTypeMatchAny(&ast.IntegerLiteral{}, expected, typ.PrimitiveString) {
		t.Error("string should not match any of [int, long]")
	}
}

func TestTypeChecker_GetSafeType_NilExpressionReturnsError(t *testing.T) {
	tc := newBasicCheckingFixture(t)
	if got := tc.getSafeType(nil); got != typ.MetaError {
		t.Errorf("getSafeType(nil) = %v, want MetaError", got)
	}
	il := &ast.IntegerLiteral{}
	il.Type = typ.PrimitiveInt
	if got := tc.getSafeType(il); got != typ.PrimitiveInt {
		t.Errorf("getSafeType(int-typed) = %v, want PrimitiveInt", got)
	}
}
