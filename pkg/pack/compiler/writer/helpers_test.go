// pkg/pack/compiler/writer/helpers_test.go
package writer_test

import (
	"reflect"
	"testing"

	"github.com/zsrv/goscape/pkg/pack/compiler/codegen"
	"github.com/zsrv/goscape/pkg/pack/compiler/lexer"
	"github.com/zsrv/goscape/pkg/pack/compiler/symbol"
	"github.com/zsrv/goscape/pkg/pack/compiler/trigger"
	typ "github.com/zsrv/goscape/pkg/pack/compiler/type"
	"github.com/zsrv/goscape/pkg/pack/compiler/writer"
)

// TestGenerateJumpTable_TwoBlocks pins jump-table layout for a 2-block
// script: block A has 3 instructions, block B has 2. JumpTable[A.Label] = 0,
// JumpTable[B.Label] = 3.
func TestGenerateJumpTable_TwoBlocks(t *testing.T) {
	procTrig := &trigger.TriggerType{ID: 0, Identifier: "proc", SubjectMode: trigger.ModeName}
	ss := &symbol.ServerScriptSymbol{ScriptSymbolFields: symbol.ScriptSymbolFields{Trigger: procTrig, Name: "foo"}}
	script := codegen.NewRuneScript("smoke.rs2", ss, procTrig, "foo", nil)

	la := &codegen.Label{Name: "a"}
	lb := &codegen.Label{Name: "b"}
	ba := codegen.NewBlock(la)
	bb := codegen.NewBlock(lb)
	ba.Add(codegen.Instruction{Opcode: codegen.PushConstantInt, Operand: 1})
	ba.Add(codegen.Instruction{Opcode: codegen.PushConstantInt, Operand: 2})
	ba.Add(codegen.Instruction{Opcode: codegen.Return})
	bb.Add(codegen.Instruction{Opcode: codegen.PushConstantInt, Operand: 3})
	bb.Add(codegen.Instruction{Opcode: codegen.Return})
	script.Blocks = []*codegen.Block{ba, bb}

	jt := writer.GenerateJumpTable(script)
	if got := jt[la]; got != 0 {
		t.Errorf("JumpTable[a] = %d, want 0", got)
	}
	if got := jt[lb]; got != 3 {
		t.Errorf("JumpTable[b] = %d, want 3", got)
	}
}

// TestGenerateLineNumberTable_DistinctLines pins the table + the parallel
// insertion-order slice. NAI-209-D-LINENUMBER-ORDER-SLICE: Go maps are
// non-deterministic; consumers iterate via the slice.
func TestGenerateLineNumberTable_DistinctLines(t *testing.T) {
	procTrig := &trigger.TriggerType{ID: 0, Identifier: "proc", SubjectMode: trigger.ModeName}
	ss := &symbol.ServerScriptSymbol{ScriptSymbolFields: symbol.ScriptSymbolFields{Trigger: procTrig, Name: "foo"}}
	script := codegen.NewRuneScript("smoke.rs2", ss, procTrig, "foo", nil)

	mk := func(line int) codegen.Instruction {
		return codegen.Instruction{
			Opcode:  codegen.PushConstantInt,
			Operand: 0,
			Source:  lexer.NodeSourceLocation{Line: line},
		}
	}
	b := codegen.NewBlock(&codegen.Label{Name: "entry"})
	b.Add(mk(1)) // pc 0 → line 1
	b.Add(mk(1)) // pc 1 → same line, skipped
	b.Add(mk(2)) // pc 2 → line 2
	b.Add(mk(2)) // pc 3 → same line, skipped
	b.Add(mk(5)) // pc 4 → line 5 (gap is fine)
	script.Blocks = []*codegen.Block{b}

	tbl, order := writer.GenerateLineNumberTable(script)
	want := map[int]int{0: 1, 2: 2, 4: 5}
	if !reflect.DeepEqual(tbl, want) {
		t.Errorf("LineNumberTable = %v, want %v", tbl, want)
	}
	wantOrder := []int{0, 2, 4}
	if !reflect.DeepEqual(order, wantOrder) {
		t.Errorf("LineNumberPCs = %v, want %v", order, wantOrder)
	}
}

// TestGetVariableId_ParamsAndLocals pins the index returned by GetVariableId
// for each LocalVariableSymbol — both parameter int + parameter string +
// declared int local + an int array.
//
// Layout in LocalTable.All:
//
//	[intParam, strParam, intLocal, intArr]
//
// Expected indices (post-filter):
//
//	intParam  -> index 0 among ints (intParam, intLocal): 0
//	strParam  -> index 0 among strings: 0
//	intLocal  -> index 1 among ints: 1
//	intArr    -> index 0 among arrays: 0
func TestGetVariableId_ParamsAndLocals(t *testing.T) {
	intParam := &symbol.LocalVariableSymbol{Name: "$ip", Type: typ.PrimitiveInt}
	strParam := &symbol.LocalVariableSymbol{Name: "$sp", Type: typ.PrimitiveString}
	intLocal := &symbol.LocalVariableSymbol{Name: "$il", Type: typ.PrimitiveInt}
	arr, _ := typ.NewArrayType(typ.PrimitiveInt)
	intArr := &symbol.LocalVariableSymbol{Name: "$ia", Type: arr}

	locals := &codegen.LocalTable{
		Parameters: []*symbol.LocalVariableSymbol{intParam, strParam},
		All:        []*symbol.LocalVariableSymbol{intParam, strParam, intLocal, intArr},
	}

	if got := writer.GetVariableId(locals, intParam); got != 0 {
		t.Errorf("intParam id = %d, want 0", got)
	}
	if got := writer.GetVariableId(locals, strParam); got != 0 {
		t.Errorf("strParam id = %d, want 0", got)
	}
	if got := writer.GetVariableId(locals, intLocal); got != 1 {
		t.Errorf("intLocal id = %d, want 1", got)
	}
	if got := writer.GetVariableId(locals, intArr); got != 0 {
		t.Errorf("intArr id = %d, want 0", got)
	}
}

// TestGetVariableId_BlockScopeDistinctSlots pins the 254/TS rule: when two
// LocalVariableSymbols with the same name and type appear in All (declared
// in mutually-exclusive block scopes), each symbol object gets its OWN slot
// — reference-equality indexOf, RuneScriptTS b8c3388
// BaseScriptWriter.getVariableId L276-282 (Array.prototype.indexOf ===)
// + TypeChecking.ts:406 (fresh LocalVariableSymbol per declaration).
//
// History: the rev-244 port pinned RuneScriptKt's data-class indexOf
// "slot recycling" (same name+type → first slot). The 254 reference
// script.dat refutes it for the TS toolchain ([proc,duel_arena_coord]:
// else-branch $i is slot 1 upstream) — caught by the T23 full-tree gate.
//
// Fixture: two separate $i int locals as if in if/else branches.
// All = [$i_if, $i_else] — $i_else gets slot 1.
//
// int_locals header count = GetLocalCount = 2 (raw symbol count, unchanged).
func TestGetVariableId_BlockScopeDistinctSlots(t *testing.T) {
	// Two distinct pointer objects, same name+type — if/else $i siblings.
	iIf := &symbol.LocalVariableSymbol{Name: "$i", Type: typ.PrimitiveInt}
	iElse := &symbol.LocalVariableSymbol{Name: "$i", Type: typ.PrimitiveInt}

	locals := &codegen.LocalTable{
		Parameters: nil,
		All:        []*symbol.LocalVariableSymbol{iIf, iElse},
	}

	if got := writer.GetVariableId(locals, iIf); got != 0 {
		t.Errorf("iIf slot = %d, want 0", got)
	}
	if got := writer.GetVariableId(locals, iElse); got != 1 {
		t.Errorf("iElse slot = %d, want 1 (reference-equality indexOf; TS does NOT recycle sibling-scope slots)", got)
	}
	// int_locals count stays 2 — raw symbol count.
	if got := writer.GetLocalCount(locals, typ.BaseVarInteger); got != 2 {
		t.Errorf("LocalCount(Integer) = %d, want 2", got)
	}
}

// TestGetVariableId_DifferentNameNoRecycling verifies that two locals with
// DIFFERENT names in sequential scopes do NOT share a slot, even if they
// have the same type.
func TestGetVariableId_DifferentNameNoRecycling(t *testing.T) {
	aLocal := &symbol.LocalVariableSymbol{Name: "$a", Type: typ.PrimitiveInt}
	bLocal := &symbol.LocalVariableSymbol{Name: "$b", Type: typ.PrimitiveInt}

	locals := &codegen.LocalTable{
		Parameters: nil,
		All:        []*symbol.LocalVariableSymbol{aLocal, bLocal},
	}

	if got := writer.GetVariableId(locals, aLocal); got != 0 {
		t.Errorf("aLocal slot = %d, want 0", got)
	}
	if got := writer.GetVariableId(locals, bLocal); got != 1 {
		t.Errorf("bLocal slot = %d, want 1 (distinct name, no recycling)", got)
	}
}

// TestGetCounts pins GetParameterCount + GetLocalCount.
// GetLocalCount excludes arrays unless the array is a parameter.
func TestGetCounts(t *testing.T) {
	intParam := &symbol.LocalVariableSymbol{Name: "$ip", Type: typ.PrimitiveInt}
	strParam := &symbol.LocalVariableSymbol{Name: "$sp", Type: typ.PrimitiveString}
	intLocal := &symbol.LocalVariableSymbol{Name: "$il", Type: typ.PrimitiveInt}
	arr, _ := typ.NewArrayType(typ.PrimitiveInt)
	intArr := &symbol.LocalVariableSymbol{Name: "$ia", Type: arr}

	locals := &codegen.LocalTable{
		Parameters: []*symbol.LocalVariableSymbol{intParam, strParam},
		All:        []*symbol.LocalVariableSymbol{intParam, strParam, intLocal, intArr},
	}

	if got := writer.GetParameterCount(locals, typ.BaseVarInteger); got != 1 {
		t.Errorf("ParameterCount(Integer) = %d, want 1", got)
	}
	if got := writer.GetParameterCount(locals, typ.BaseVarString); got != 1 {
		t.Errorf("ParameterCount(String) = %d, want 1", got)
	}
	// Two int locals counted (intParam, intLocal); intArr excluded since it
	// is an ArrayType AND it is not a parameter.
	if got := writer.GetLocalCount(locals, typ.BaseVarInteger); got != 2 {
		t.Errorf("LocalCount(Integer) = %d, want 2", got)
	}
}
