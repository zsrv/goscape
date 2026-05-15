// pkg/pack/compiler/symbol/table_test.go
package symbol

import (
	"testing"

	typ "github.com/zsrv/goscape/pkg/pack/compiler/type"
)

func TestSymbolTable_InsertAndFind(t *testing.T) {
	tg := makeTriggerStub("proc")
	st := NewSymbolTable(nil)
	sym := &ServerScriptSymbol{ScriptSymbolFields: ScriptSymbolFields{
		Trigger: tg, Name: "foo", Parameters: typ.MetaUnit, Returns: typ.MetaUnit,
	}}
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
	first := &ServerScriptSymbol{ScriptSymbolFields: ScriptSymbolFields{
		Trigger: tg, Name: "foo",
	}}
	second := &ServerScriptSymbol{ScriptSymbolFields: ScriptSymbolFields{
		Trigger: tg, Name: "foo",
	}}
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
		ScriptSymbolFields: ScriptSymbolFields{Trigger: tg, Name: "foo"},
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
		ScriptSymbolFields: ScriptSymbolFields{Trigger: tg, Name: "child_only"},
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
		ScriptSymbolFields: ScriptSymbolFields{Trigger: tg, Name: "foo"},
	})
	child := root.CreateSubTable()
	if child.Insert(SymbolTypeServerScript(tg), &ServerScriptSymbol{
		ScriptSymbolFields: ScriptSymbolFields{Trigger: tg, Name: "foo"},
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
		ScriptSymbolFields: ScriptSymbolFields{Trigger: tg, Name: "PascalCase"},
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
