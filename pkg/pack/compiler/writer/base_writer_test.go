// pkg/pack/compiler/writer/base_writer_test.go
package writer_test

import (
	"reflect"
	"testing"

	"github.com/zsrv/goscape/pkg/pack/compiler/codegen"
	"github.com/zsrv/goscape/pkg/pack/compiler/symbol"
	"github.com/zsrv/goscape/pkg/pack/compiler/trigger"
	typ "github.com/zsrv/goscape/pkg/pack/compiler/type"
	"github.com/zsrv/goscape/pkg/pack/compiler/writer"
)

// recorderWriter records the method names called on it in order, so the
// dispatch tests can pin (block-enter, per-instruction call, advance-after)
// ordering.
type recorderWriter struct {
	calls    []string
	curIndex []int
	ctx      *writer.BaseContext
}

func (r *recorderWriter) note(name string) {
	r.calls = append(r.calls, name)
	r.curIndex = append(r.curIndex, r.ctx.CurIndex)
}

func (r *recorderWriter) EnterBlock(b *codegen.Block)                                 { r.note("EnterBlock") }
func (r *recorderWriter) WritePushConstantInt(int32)                                  { r.note("WritePushConstantInt") }
func (r *recorderWriter) WritePushConstantString(string)                              { r.note("WritePushConstantString") }
func (r *recorderWriter) WritePushConstantLong(int64)                                 { r.note("WritePushConstantLong") }
func (r *recorderWriter) WritePushConstantSymbol(symbol.Symbol)                       { r.note("WritePushConstantSymbol") }
func (r *recorderWriter) WritePushLocalVar(*symbol.LocalVariableSymbol)               { r.note("WritePushLocalVar") }
func (r *recorderWriter) WritePopLocalVar(*symbol.LocalVariableSymbol)                { r.note("WritePopLocalVar") }
func (r *recorderWriter) WritePushVar(*symbol.BasicSymbol, bool)                      { r.note("WritePushVar") }
func (r *recorderWriter) WritePopVar(*symbol.BasicSymbol, bool)                       { r.note("WritePopVar") }
func (r *recorderWriter) WriteDefineArray(*symbol.LocalVariableSymbol)                { r.note("WriteDefineArray") }
func (r *recorderWriter) WriteSwitch(*codegen.SwitchTable)                            { r.note("WriteSwitch") }
func (r *recorderWriter) WriteBranch(codegen.Opcode, *codegen.Label)                  { r.note("WriteBranch") }
func (r *recorderWriter) WriteJoinString(int)                                         { r.note("WriteJoinString") }
func (r *recorderWriter) WriteDiscard(typ.BaseVarType)                                { r.note("WriteDiscard") }
func (r *recorderWriter) WriteJump(symbol.Symbol)                                     { r.note("WriteJump") }
func (r *recorderWriter) WriteGosub(symbol.Symbol)                                    { r.note("WriteGosub") }
func (r *recorderWriter) WriteCommand(symbol.Symbol)                                  { r.note("WriteCommand") }
func (r *recorderWriter) WriteReturn()                                                { r.note("WriteReturn") }
func (r *recorderWriter) WriteMath(codegen.Opcode)                                    { r.note("WriteMath") }

// TestWriteScript_DispatchOrder pins the per-instruction dispatch + the
// CurIndex post-increment.
func TestWriteScript_DispatchOrder(t *testing.T) {
	procTrig := &trigger.TriggerType{ID: 0, Identifier: "proc", SubjectMode: trigger.ModeName}
	ss := &symbol.ServerScriptSymbol{ScriptSymbolFields: symbol.ScriptSymbolFields{Trigger: procTrig, Name: "foo"}}
	script := codegen.NewRuneScript("smoke.rs2", ss, procTrig, "foo", nil)

	la := &codegen.Label{Name: "a"}
	lb := &codegen.Label{Name: "b"}
	ba := codegen.NewBlock(la)
	bb := codegen.NewBlock(lb)
	ba.Add(codegen.Instruction{Opcode: codegen.PushConstantInt, Operand: 42})
	ba.Add(codegen.Instruction{Opcode: codegen.Branch, Operand: lb})
	bb.Add(codegen.Instruction{Opcode: codegen.Return})
	script.Blocks = []*codegen.Block{ba, bb}

	ctx := writer.NewBaseContext(script)
	r := &recorderWriter{ctx: ctx}
	writer.WriteScript(r, ctx, script)

	wantCalls := []string{
		"EnterBlock",
		"WritePushConstantInt",
		"WriteBranch",
		"EnterBlock",
		"WriteReturn",
	}
	if !reflect.DeepEqual(r.calls, wantCalls) {
		t.Errorf("calls = %v\nwant %v", r.calls, wantCalls)
	}
	// CurIndex at the time each method ran (Enter sees pre-increment of its
	// block's first instruction):
	//   EnterBlock(a)            CurIndex=0
	//   WritePushConstantInt(42) CurIndex=0
	//   WriteBranch              CurIndex=1
	//   EnterBlock(b)            CurIndex=2
	//   WriteReturn              CurIndex=2
	wantIdx := []int{0, 0, 1, 2, 2}
	if !reflect.DeepEqual(r.curIndex, wantIdx) {
		t.Errorf("curIndex per call = %v\nwant %v", r.curIndex, wantIdx)
	}
	// Post-loop CurIndex equals total instruction count.
	if ctx.CurIndex != 3 {
		t.Errorf("post-loop CurIndex = %d, want 3", ctx.CurIndex)
	}
}

// TestDispatch_LineNumberPanics pins that the codegen author can't smuggle
// LineNumber instructions to the writer.
func TestDispatch_LineNumberPanics(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Errorf("dispatch did not panic on LineNumber opcode")
		}
	}()
	procTrig := &trigger.TriggerType{ID: 0, Identifier: "proc", SubjectMode: trigger.ModeName}
	ss := &symbol.ServerScriptSymbol{ScriptSymbolFields: symbol.ScriptSymbolFields{Trigger: procTrig, Name: "foo"}}
	script := codegen.NewRuneScript("smoke.rs2", ss, procTrig, "foo", nil)
	b := codegen.NewBlock(&codegen.Label{Name: "e"})
	b.Add(codegen.Instruction{Opcode: codegen.LineNumber, Operand: 1})
	script.Blocks = []*codegen.Block{b}
	ctx := writer.NewBaseContext(script)
	writer.WriteScript(&recorderWriter{ctx: ctx}, ctx, script)
}
