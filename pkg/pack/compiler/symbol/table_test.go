// pkg/pack/compiler/symbol/table_test.go
package symbol

import (
	"testing"

	typ "github.com/zsrv/goscape/pkg/pack/compiler/type"
)

func TestSymbolTable_InsertAndFind(t *testing.T) {
	tg := makeTriggerStub("proc")
	st := NewSymbolTable(nil)
	sym := &ServerScriptSymbol{
		Trigger: tg, Name: "foo", Parameters: typ.MetaUnit, Returns: typ.MetaUnit}
	if !st.Insert(SymbolTypeServerScript(tg), sym) {
		t.Fatal("first Insert returned false")
	}
	got := st.Find(SymbolTypeServerScript(tg), "foo")
	if got != sym {
		t.Fatalf("Find returned different pointer: %v", got)
	}
}

func TestSymbolTable_Insert_DuplicateReturnsFalse(t *testing.T) {
	tg := makeTriggerStub("proc")
	st := NewSymbolTable(nil)
	first := &ServerScriptSymbol{
		Trigger: tg, Name: "foo"}
	second := &ServerScriptSymbol{
		Trigger: tg, Name: "foo"}
	if !st.Insert(SymbolTypeServerScript(tg), first) {
		t.Fatal("first Insert returned false")
	}
	if st.Insert(SymbolTypeServerScript(tg), second) {
		t.Fatal("second Insert returned true; want false (already-defined)")
	}
}

func TestSymbolTable_Find_Miss(t *testing.T) {
	st := NewSymbolTable(nil)
	tg := makeTriggerStub("proc")
	if got := st.Find(SymbolTypeServerScript(tg), "missing"); got != nil {
		t.Fatalf("Find miss = %v, want nil", got)
	}
}

func TestSymbolTable_ChildLookupWalksParent(t *testing.T) {
	tg := makeTriggerStub("proc")
	root := NewSymbolTable(nil)
	root.Insert(SymbolTypeServerScript(tg), &ServerScriptSymbol{
		Trigger: tg, Name: "foo",
	})
	child := root.CreateSubTable()
	got := child.Find(SymbolTypeServerScript(tg), "foo")
	if got == nil {
		t.Fatal("child.Find did not walk to parent")
	}
}

func TestSymbolTable_ParentDoesNotWalkChild(t *testing.T) {
	tg := makeTriggerStub("proc")
	root := NewSymbolTable(nil)
	child := root.CreateSubTable()
	child.Insert(SymbolTypeServerScript(tg), &ServerScriptSymbol{
		Trigger: tg, Name: "child_only",
	})
	if root.Find(SymbolTypeServerScript(tg), "child_only") != nil {
		t.Fatal("root.Find found child-table entry")
	}
}

func TestSymbolTable_ChildInsertBlocksOnParent(t *testing.T) {
	// Per TS L29-36: child Insert checks the parent chain. If parent already
	// has the same (type, name), child Insert returns false.
	tg := makeTriggerStub("proc")
	root := NewSymbolTable(nil)
	root.Insert(SymbolTypeServerScript(tg), &ServerScriptSymbol{
		Trigger: tg, Name: "foo",
	})
	child := root.CreateSubTable()
	if child.Insert(SymbolTypeServerScript(tg), &ServerScriptSymbol{
		Trigger: tg, Name: "foo",
	}) {
		t.Fatal("child.Insert succeeded despite parent having same entry")
	}
}

func TestSymbolTable_BasicNameNormalisation(t *testing.T) {
	// Per TS L18-22: Basic kind normalises name (lowercase + whitespace→_).
	st := NewSymbolTable(nil)
	st.Insert(SymbolTypeBasic(typ.PrimitiveInt), &BasicSymbol{
		Name: "Wooden Bowl", Type: typ.PrimitiveInt,
	})
	got := st.Find(SymbolTypeBasic(typ.PrimitiveInt), "wooden_bowl")
	if got == nil {
		t.Fatal("normalised lookup miss: 'Wooden Bowl' inserted; 'wooden_bowl' should hit")
	}
}

func TestSymbolTable_ServerScriptNameNotNormalised(t *testing.T) {
	tg := makeTriggerStub("proc")
	st := NewSymbolTable(nil)
	st.Insert(SymbolTypeServerScript(tg), &ServerScriptSymbol{
		Trigger: tg, Name: "PascalCase",
	})
	if got := st.Find(SymbolTypeServerScript(tg), "pascalcase"); got != nil {
		t.Fatal("server-script name lookup case-insensitive; want case-sensitive")
	}
	if got := st.Find(SymbolTypeServerScript(tg), "PascalCase"); got == nil {
		t.Fatal("server-script exact-case lookup missed")
	}
}

func TestSymbolTable_SatisfiesAstSymbolTableRef(t *testing.T) {
	st := NewSymbolTable(nil)
	var _ astSymbolTableRef = st
}

type astSymbolTableRef interface {
	AsSymbolTableRef()
}

func TestSymbolTable_FindAll_AcrossKinds(t *testing.T) {
	tg := makeTriggerStub("proc")
	st := NewSymbolTable(nil)
	// Two symbols share name "foo" across different kinds.
	st.Insert(SymbolTypeServerScript(tg), &ServerScriptSymbol{
		Trigger: tg, Name: "foo",
	})
	st.Insert(SymbolTypeLocalVariable(), &LocalVariableSymbol{Name: "foo"})

	got := st.FindAll("foo")
	if len(got) != 2 {
		t.Fatalf("FindAll(\"foo\") len = %d, want 2", len(got))
	}
}

func TestSymbolTable_FindAll_WalksParent(t *testing.T) {
	tg := makeTriggerStub("proc")
	root := NewSymbolTable(nil)
	root.Insert(SymbolTypeServerScript(tg), &ServerScriptSymbol{
		Trigger: tg, Name: "x",
	})
	child := root.CreateSubTable()
	child.Insert(SymbolTypeLocalVariable(), &LocalVariableSymbol{Name: "x"})

	got := child.FindAll("x")
	if len(got) != 2 {
		t.Fatalf("child.FindAll(\"x\") len = %d, want 2 (one from child + one from parent)", len(got))
	}
}

func TestSymbolTable_FindAll_Miss(t *testing.T) {
	st := NewSymbolTable(nil)
	got := st.FindAll("nope")
	if len(got) != 0 {
		t.Fatalf("FindAll miss len = %d, want 0", len(got))
	}
}

// TestSymbolTable_FindAll_DeterministicOrder pins FindAll's iteration order to
// be stable across repeated calls. Without this guarantee, callers like
// type_checking_expr.go (visitGameVariableExpression / resolveSymbol) that
// iterate FindAll and break on the first matching symbol will pick a different
// symbol between runs whenever cross-kind name collisions exist — corrupting
// bytecode determinism.
//
// The root cause is findAllInto iterating `st.symbols` (a Go map) without
// sorting. This test populates a table with several symbols of different kinds
// sharing the same name, calls FindAll many times, and asserts every call
// returns the same sequence of pointers.
func TestSymbolTable_FindAll_DeterministicOrder(t *testing.T) {
	const N = 200

	tg := makeTriggerStub("proc")
	st := NewSymbolTable(nil)

	// Insert symbols of multiple kinds all named "foo" so they all land in
	// different outer-map keys but match the same FindAll name. Goal: maximise
	// outer-map iteration sensitivity to Go's map randomisation.
	st.Insert(SymbolTypeServerScript(tg), &ServerScriptSymbol{
		Trigger: tg, Name: "foo",
	})
	st.Insert(SymbolTypeLocalVariable(), &LocalVariableSymbol{Name: "foo"})
	st.Insert(SymbolTypeConstant(), &ConstantSymbol{Name: "foo", Value: "1"})
	st.Insert(SymbolTypeBasic(typ.PrimitiveInt), &BasicSymbol{
		Name: "foo", Type: typ.PrimitiveInt,
	})
	st.Insert(SymbolTypeBasic(typ.PrimitiveBoolean), &BasicSymbol{
		Name: "foo", Type: typ.PrimitiveBoolean,
	})
	st.Insert(SymbolTypeBasic(typ.PrimitiveString), &BasicSymbol{
		Name: "foo", Type: typ.PrimitiveString,
	})
	st.Insert(SymbolTypeBasic(typ.PrimitiveCoord), &BasicSymbol{
		Name: "foo", Type: typ.PrimitiveCoord,
	})

	first := st.FindAll("foo")
	if len(first) < 2 {
		t.Fatalf("FindAll(\"foo\") returned %d symbols; need >=2 to test ordering", len(first))
	}

	for i := range N {
		got := st.FindAll("foo")
		if len(got) != len(first) {
			t.Fatalf("iteration %d: len mismatch: first=%d got=%d", i, len(first), len(got))
		}
		for j := range got {
			if got[j] != first[j] {
				t.Fatalf("iteration %d: order differs at index %d:\n  first[%d]=%T(%s)\n  got[%d]=%T(%s)\nfull first: %v\nfull got:   %v",
					i, j, j, first[j], first[j].SymbolName(), j, got[j], got[j].SymbolName(),
					symNames(first), symNames(got))
			}
		}
	}
}

// TestSymbolTable_FindAll_DeterministicOrder_AcrossParents pins iteration
// determinism across the parent-chain walk too. Same property as the
// single-table variant but additionally exercises findAllInto's recursive
// st.parent.findAllInto call.
func TestSymbolTable_FindAll_DeterministicOrder_AcrossParents(t *testing.T) {
	const N = 200

	tg := makeTriggerStub("proc")
	root := NewSymbolTable(nil)
	root.Insert(SymbolTypeServerScript(tg), &ServerScriptSymbol{
		Trigger: tg, Name: "x",
	})
	root.Insert(SymbolTypeBasic(typ.PrimitiveInt), &BasicSymbol{
		Name: "x", Type: typ.PrimitiveInt,
	})
	root.Insert(SymbolTypeBasic(typ.PrimitiveBoolean), &BasicSymbol{
		Name: "x", Type: typ.PrimitiveBoolean,
	})
	root.Insert(SymbolTypeConstant(), &ConstantSymbol{Name: "x", Value: "1"})

	child := root.CreateSubTable()
	child.Insert(SymbolTypeLocalVariable(), &LocalVariableSymbol{Name: "x"})
	child.Insert(SymbolTypeBasic(typ.PrimitiveString), &BasicSymbol{
		Name: "x", Type: typ.PrimitiveString,
	})
	child.Insert(SymbolTypeBasic(typ.PrimitiveCoord), &BasicSymbol{
		Name: "x", Type: typ.PrimitiveCoord,
	})

	first := child.FindAll("x")
	if len(first) < 2 {
		t.Fatalf("FindAll(\"x\") returned %d symbols; need >=2 to test ordering", len(first))
	}

	for i := range N {
		got := child.FindAll("x")
		if len(got) != len(first) {
			t.Fatalf("iteration %d: len mismatch: first=%d got=%d", i, len(first), len(got))
		}
		for j := range got {
			if got[j] != first[j] {
				t.Fatalf("iteration %d: order differs at index %d:\n  first=%v\n  got=  %v",
					i, j, symNames(first), symNames(got))
			}
		}
	}
}

func symNames(syms []Symbol) []string {
	out := make([]string, len(syms))
	for i, s := range syms {
		out[i] = s.SymbolName()
	}
	return out
}
