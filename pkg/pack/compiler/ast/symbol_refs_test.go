package ast

import (
	"os"
	"strings"
	"testing"
)

// astRefStubSymbol satisfies SymbolRef via its AsSymbolRef method.
// This is the cross-package satisfaction pattern that NAI-205-D-AST-REF-INTERFACES
// relies on: the concrete impl can live outside the ast package because
// the marker method is exported.
type astRefStubSymbol struct{}

func (*astRefStubSymbol) AsSymbolRef() {}

type astRefStubTrigger struct{}

func (*astRefStubTrigger) AsTriggerRef() {}

type astRefStubType struct{}

func (*astRefStubType) AsTypeRef() {}

type astRefStubTable struct{}

func (*astRefStubTable) AsSymbolTableRef() {}

// TestSymbolRef_StructuralSatisfaction pins that a concrete pointer type with
// an exported AsSymbolRef() method satisfies SymbolRef without importing ast
// or sharing a package. This mirrors how *symbol.ServerScriptSymbol etc. will
// flow into ast.Script.Symbol fields after T8.
func TestSymbolRef_StructuralSatisfaction(t *testing.T) {
	var s SymbolRef = (*astRefStubSymbol)(nil)
	_ = s

	var tr TriggerRef = (*astRefStubTrigger)(nil)
	_ = tr

	var ty TypeRef = (*astRefStubType)(nil)
	_ = ty

	var tb SymbolTableRef = (*astRefStubTable)(nil)
	_ = tb
}

// TestSymbolRef_DocCommentTagged pins that symbol_refs.go carries the
// NAI-205-D-AST-REF-INTERFACES deviation tag. Sister NAI-204 pin tests use
// the readAllGoFiles helper in parser/; we duplicate the inline read here
// to avoid a parser → ast import (which would be backwards).
func TestSymbolRef_DocCommentTagged(t *testing.T) {
	b := mustReadFileForTest(t, "symbol_refs.go")
	if !contains(b, "NAI-205-D-AST-REF-INTERFACES") {
		t.Fatal("symbol_refs.go missing deviation tag NAI-205-D-AST-REF-INTERFACES")
	}
}

func mustReadFileForTest(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(b)
}

func contains(s, sub string) bool { return strings.Contains(s, sub) }
