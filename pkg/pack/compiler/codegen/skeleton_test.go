package codegen

import (
	"reflect"
	"testing"

	"github.com/zsrv/goscape/pkg/pack/compiler/symbol"
	"github.com/zsrv/goscape/pkg/pack/compiler/trigger"
)

// TestOpcode_AllSingletonsDeclared pins the 48-opcode surface against TS
// Opcode.ts. If a singleton is removed or renamed, this test fails.
func TestOpcode_AllSingletonsDeclared(t *testing.T) {
	want := []struct {
		op   Opcode
		name string
		kind OperandKind
	}{
		{PushConstantInt, "PushConstantInt", OperandInt},
		{PushConstantString, "PushConstantString", OperandString},
		{PushConstantLong, "PushConstantLong", OperandLong},
		{PushConstantSymbol, "PushConstantSymbol", OperandRuneScriptSym},
		{PushLocalVar, "PushLocalVar", OperandLocalVar},
		{PopLocalVar, "PopLocalVar", OperandLocalVar},
		{PushVar, "PushVar", OperandBasicVar},
		{PushVar2, "PushVar2", OperandBasicVar},
		{PopVar, "PopVar", OperandBasicVar},
		{PopVar2, "PopVar2", OperandBasicVar},
		{DefineArray, "DefineArray", OperandLocalVar},
		{Switch, "Switch", OperandSwitchTable},
		{Branch, "Branch", OperandLabel},
		{BranchNot, "BranchNot", OperandLabel},
		{BranchEquals, "BranchEquals", OperandLabel},
		{BranchLessThan, "BranchLessThan", OperandLabel},
		{BranchGreaterThan, "BranchGreaterThan", OperandLabel},
		{BranchLessThanOrEquals, "BranchLessThanOrEquals", OperandLabel},
		{BranchGreaterThanOrEquals, "BranchGreaterThanOrEquals", OperandLabel},
		{LongBranchNot, "LongBranchNot", OperandLabel},
		{LongBranchEquals, "LongBranchEquals", OperandLabel},
		{LongBranchLessThan, "LongBranchLessThan", OperandLabel},
		{LongBranchGreaterThan, "LongBranchGreaterThan", OperandLabel},
		{LongBranchLessThanOrEquals, "LongBranchLessThanOrEquals", OperandLabel},
		{LongBranchGreaterThanOrEquals, "LongBranchGreaterThanOrEquals", OperandLabel},
		{ObjBranchNot, "ObjBranchNot", OperandLabel},
		{ObjBranchEquals, "ObjBranchEquals", OperandLabel},
		{JoinString, "JoinString", OperandInt},
		{Discard, "Discard", OperandBaseVarType},
		{Gosub, "Gosub", OperandScriptSym},
		{Jump, "Jump", OperandScriptSym},
		{Command, "Command", OperandScriptSym},
		{Return, "Return", OperandNone},
		{Add, "Add", OperandNone},
		{Sub, "Sub", OperandNone},
		{Multiply, "Multiply", OperandNone},
		{Divide, "Divide", OperandNone},
		{Modulo, "Modulo", OperandNone},
		{Or, "Or", OperandNone},
		{And, "And", OperandNone},
		{LongAdd, "LongAdd", OperandNone},
		{LongSub, "LongSub", OperandNone},
		{LongMultiply, "LongMultiply", OperandNone},
		{LongDivide, "LongDivide", OperandNone},
		{LongModulo, "LongModulo", OperandNone},
		{LongOr, "LongOr", OperandNone},
		{LongAnd, "LongAnd", OperandNone},
		{LineNumber, "LineNumber", OperandInt},
	}

	for _, c := range want {
		if c.op.Name != c.name {
			t.Errorf("Opcode.Name: got %q, want %q", c.op.Name, c.name)
		}
		if c.op.Kind != c.kind {
			t.Errorf("Opcode %q Kind: got %v, want %v", c.name, c.op.Kind, c.kind)
		}
	}
}

func TestLabelGenerator_GeneratesUniqueNames(t *testing.T) {
	g := NewLabelGenerator()
	l1 := g.Generate("if_true")
	l2 := g.Generate("if_true")
	l3 := g.Generate("if_end")
	if l1.Name != "if_true_0" {
		t.Errorf("l1.Name: got %q, want %q", l1.Name, "if_true_0")
	}
	if l2.Name != "if_true_1" {
		t.Errorf("l2.Name: got %q, want %q", l2.Name, "if_true_1")
	}
	if l3.Name != "if_end_0" {
		t.Errorf("l3.Name: got %q, want %q", l3.Name, "if_end_0")
	}
	g.Reset()
	l4 := g.Generate("if_true")
	if l4.Name != "if_true_0" {
		t.Errorf("after Reset, l4.Name: got %q, want %q", l4.Name, "if_true_0")
	}
}

func TestBlock_AddsInstructions(t *testing.T) {
	lbl := &Label{Name: "entry"}
	b := NewBlock(lbl)
	b.Add(Instruction{Opcode: PushConstantInt, Operand: 42})
	b.Add(Instruction{Opcode: Return})
	if got := len(b.Instructions); got != 2 {
		t.Fatalf("Instructions len: got %d, want 2", got)
	}
	if b.Instructions[0].Opcode != PushConstantInt {
		t.Errorf("Instructions[0].Opcode: got %v, want PushConstantInt", b.Instructions[0].Opcode)
	}
	if b.Instructions[0].Operand.(int) != 42 {
		t.Errorf("Instructions[0].Operand: got %v, want 42", b.Instructions[0].Operand)
	}
}

func TestSwitchTable_AddsCases(t *testing.T) {
	st := NewSwitchTable(0)
	if st.ID != 0 {
		t.Fatalf("ID: got %d, want 0", st.ID)
	}
	lbl := &Label{Name: "case_a"}
	st.AddCase(SwitchCase{Label: lbl, Keys: []any{1, 2, 3}})
	cases := st.Cases()
	if len(cases) != 1 {
		t.Fatalf("Cases len: got %d, want 1", len(cases))
	}
	if !reflect.DeepEqual(cases[0].Keys, []any{1, 2, 3}) {
		t.Errorf("Cases[0].Keys: got %v, want [1 2 3]", cases[0].Keys)
	}

	// Mutating the returned slice must not affect internal state.
	cases[0] = SwitchCase{}
	fresh := st.Cases()
	if len(fresh) != 1 || fresh[0].Label != lbl {
		t.Error("Cases() copy-on-read violated: mutation of returned slice affected internal state")
	}
}

func TestRuneScript_FullNameAndSwitchTable(t *testing.T) {
	tr := &trigger.TriggerType{Identifier: "proc"}
	sym := &symbol.ServerScriptSymbol{
		ScriptSymbolFields: symbol.ScriptSymbolFields{Trigger: tr, Name: "foo"},
	}
	rs := NewRuneScript("foo.rs2", sym, tr, sym.Name, nil)
	if rs.FullName != "[proc,foo]" {
		t.Errorf("FullName: got %q, want %q", rs.FullName, "[proc,foo]")
	}
	st := rs.GenerateSwitchTable()
	if st.ID != 0 {
		t.Errorf("first switch table ID: got %d, want 0", st.ID)
	}
	st2 := rs.GenerateSwitchTable()
	if st2.ID != 1 {
		t.Errorf("second switch table ID: got %d, want 1", st2.ID)
	}
	if len(rs.SwitchTables) != 2 {
		t.Fatalf("SwitchTables len: got %d, want 2", len(rs.SwitchTables))
	}
}

func TestLocalTable_AppendsParametersAndAll(t *testing.T) {
	lt := &LocalTable{}
	p1 := &symbol.LocalVariableSymbol{}
	p2 := &symbol.LocalVariableSymbol{}
	lt.Parameters = append(lt.Parameters, p1, p2)
	lt.All = append(lt.All, p1, p2)
	if len(lt.Parameters) != 2 || len(lt.All) != 2 {
		t.Fatalf("Parameters/All len: got %d/%d, want 2/2", len(lt.Parameters), len(lt.All))
	}
}
