// pkg/pack/compiler/runescript/binary_writer_test.go
package runescript_test

import (
	"bytes"
	"encoding/binary"
	"testing"

	"github.com/zsrv/goscape/pkg/pack/compiler/codegen"
	"github.com/zsrv/goscape/pkg/pack/compiler/runescript"
	"github.com/zsrv/goscape/pkg/pack/compiler/symbol"
	"github.com/zsrv/goscape/pkg/pack/compiler/trigger"
	typ "github.com/zsrv/goscape/pkg/pack/compiler/type"
	"github.com/zsrv/goscape/pkg/pack/compiler/writer"
)

// stubIdProvider returns a fixed ID for every symbol.
type stubIdProvider struct{ id int }

func (s stubIdProvider) Get(symbol.Symbol) int { return s.id }

// recOutput captures the data passed to OutputScript so tests can re-decode.
type recOutput struct {
	script *codegen.RuneScript
	data   []byte
}

func (r *recOutput) OutputScript(s *codegen.RuneScript, data []byte) {
	r.script = s
	d := make([]byte, len(data))
	copy(d, data)
	r.data = d
}

func minimalScript(t *testing.T, name string, blocks ...*codegen.Block) *codegen.RuneScript {
	t.Helper()
	procTrig := &trigger.TriggerType{ID: 5, Identifier: "proc", SubjectMode: trigger.ModeName, AllowParameters: true, AllowReturns: true}
	ss := &symbol.ServerScriptSymbol{ScriptSymbolFields: symbol.ScriptSymbolFields{
		Trigger:    procTrig,
		Name:       name,
		Parameters: typ.MetaUnit,
		Returns:    typ.MetaUnit,
	}}
	s := codegen.NewRuneScript("smoke.rs2", ss, procTrig, name, nil)
	if len(blocks) == 0 {
		blocks = []*codegen.Block{codegen.NewBlock(&codegen.Label{Name: "e"})}
	}
	s.Blocks = blocks
	return s
}

// runOne drives a 1-block script with one Instruction through the writer and
// returns the instructionBuffer prefix the writer emitted, *plus* the
// recOutput-captured full buffer for full-stack tests.
func runOne(t *testing.T, idp writer.IdProvider, ins codegen.Instruction) []byte {
	t.Helper()
	b := codegen.NewBlock(&codegen.Label{Name: "e"})
	b.Add(ins)
	s := minimalScript(t, "x", b)
	out := &recOutput{}
	w := runescript.NewBinaryScriptWriter(idp, out)
	w.Write(s)
	// The first 2 bytes of the instruction stream within the Finish() blob
	// are the opcode; the test pulls them out by re-decoding the header.
	off := len(s.FullName) + 1 + len(s.SourceName) + 1 + 4 + 1
	off += 2 // lineNumberCount
	return out.data[off:]
}

// TestWritePushConstantInt pins the opcode + operand bytes.
func TestWritePushConstantInt(t *testing.T) {
	got := runOne(t, stubIdProvider{}, codegen.Instruction{Opcode: codegen.PushConstantInt, Operand: int(42)})
	want := []byte{0x00, 0x00, 0x00, 0x00, 0x00, 0x2A}
	if !bytes.Equal(got[:len(want)], want) {
		t.Errorf("got %x, want %x", got[:len(want)], want)
	}
}

// TestWritePushConstantString pins opcode + bytes + NUL.
func TestWritePushConstantString(t *testing.T) {
	got := runOne(t, stubIdProvider{}, codegen.Instruction{Opcode: codegen.PushConstantString, Operand: "hi"})
	want := []byte{0x00, 0x03, 'h', 'i', 0x00}
	if !bytes.Equal(got[:len(want)], want) {
		t.Errorf("got %x, want %x", got[:len(want)], want)
	}
}

// TestWritePushLocalVar_IntParam pins PushIntLocal (id=33) + variable-id.
func TestWritePushLocalVar_IntParam(t *testing.T) {
	intParam := &symbol.LocalVariableSymbol{Name: "$p", Type: typ.PrimitiveInt}
	b := codegen.NewBlock(&codegen.Label{Name: "e"})
	b.Add(codegen.Instruction{Opcode: codegen.PushLocalVar, Operand: intParam})
	s := minimalScript(t, "x", b)
	s.Locals.Parameters = []*symbol.LocalVariableSymbol{intParam}
	s.Locals.All = []*symbol.LocalVariableSymbol{intParam}

	out := &recOutput{}
	w := runescript.NewBinaryScriptWriter(stubIdProvider{}, out)
	w.Write(s)
	off := len(s.FullName) + 1 + len(s.SourceName) + 1 + 4 + 1 + 2
	want := []byte{0x00, 0x21, 0x00, 0x00, 0x00, 0x00}
	if !bytes.Equal(out.data[off:off+len(want)], want) {
		t.Errorf("got %x, want %x", out.data[off:off+len(want)], want)
	}
}

// TestWritePushVar_VarpDot pins the dot-bit encoding (+1<<16).
func TestWritePushVar_VarpDot(t *testing.T) {
	v := &symbol.BasicSymbol{Name: "vp", Type: typ.NewVarPlayerType(typ.PrimitiveInt)}
	b := codegen.NewBlock(&codegen.Label{Name: "e"})
	b.Add(codegen.Instruction{Opcode: codegen.PushVar2, Operand: v}) // PushVar2 → dot=true
	s := minimalScript(t, "x", b)

	out := &recOutput{}
	w := runescript.NewBinaryScriptWriter(stubIdProvider{id: 7}, out)
	w.Write(s)
	off := len(s.FullName) + 1 + len(s.SourceName) + 1 + 4 + 1 + 2
	want := []byte{0x00, 0x01, 0x00, 0x01, 0x00, 0x07}
	if !bytes.Equal(out.data[off:off+len(want)], want) {
		t.Errorf("got %x, want %x", out.data[off:off+len(want)], want)
	}
}

// TestWriteDefineArray pins (id<<16) | charCode encoding.
func TestWriteDefineArray(t *testing.T) {
	arr, _ := typ.NewArrayType(typ.PrimitiveInt) // PrimitiveInt code = 'i'
	local := &symbol.LocalVariableSymbol{Name: "$a", Type: arr}
	b := codegen.NewBlock(&codegen.Label{Name: "e"})
	b.Add(codegen.Instruction{Opcode: codegen.DefineArray, Operand: local})
	s := minimalScript(t, "x", b)
	s.Locals.All = []*symbol.LocalVariableSymbol{local}

	out := &recOutput{}
	w := runescript.NewBinaryScriptWriter(stubIdProvider{}, out)
	w.Write(s)
	off := len(s.FullName) + 1 + len(s.SourceName) + 1 + 4 + 1 + 2
	want := []byte{0x00, 0x2C, 0x00, 0x00, 0x00, 0x69}
	if !bytes.Equal(out.data[off:off+len(want)], want) {
		t.Errorf("got %x, want %x", out.data[off:off+len(want)], want)
	}
}

// TestWriteBranch pins the `jumpLocation - curIndex - 1` arithmetic.
func TestWriteBranch(t *testing.T) {
	la := &codegen.Label{Name: "a"}
	lb := &codegen.Label{Name: "b"}
	ba := codegen.NewBlock(la)
	bb := codegen.NewBlock(lb)
	ba.Add(codegen.Instruction{Opcode: codegen.PushConstantInt, Operand: 0})
	ba.Add(codegen.Instruction{Opcode: codegen.Branch, Operand: lb})
	bb.Add(codegen.Instruction{Opcode: codegen.Return})
	s := minimalScript(t, "x", ba, bb)

	out := &recOutput{}
	w := runescript.NewBinaryScriptWriter(stubIdProvider{}, out)
	w.Write(s)
	off := len(s.FullName) + 1 + len(s.SourceName) + 1 + 4 + 1 + 2
	off += 6
	want := []byte{0x00, 0x06, 0x00, 0x00, 0x00, 0x00}
	if !bytes.Equal(out.data[off:off+len(want)], want) {
		t.Errorf("got %x, want %x", out.data[off:off+len(want)], want)
	}
}

// TestWriteCommand pins InstructionRaw with secondary=1 for dot-prefixed names.
func TestWriteCommand(t *testing.T) {
	cmd := &symbol.ServerScriptSymbol{ScriptSymbolFields: symbol.ScriptSymbolFields{
		Trigger: trigger.CommandTrigger,
		Name:    ".mes",
	}}
	b := codegen.NewBlock(&codegen.Label{Name: "e"})
	b.Add(codegen.Instruction{Opcode: codegen.Command, Operand: cmd})
	s := minimalScript(t, "x", b)

	out := &recOutput{}
	w := runescript.NewBinaryScriptWriter(stubIdProvider{id: 0x1234}, out)
	w.Write(s)
	off := len(s.FullName) + 1 + len(s.SourceName) + 1 + 4 + 1 + 2
	want := []byte{0x12, 0x34, 0x01}
	if !bytes.Equal(out.data[off:off+len(want)], want) {
		t.Errorf("got %x, want %x", out.data[off:off+len(want)], want)
	}
}

// TestWritePushConstantLong_Panics pins NAI-209-D-PUSHLONG-PANIC.
func TestWritePushConstantLong_Panics(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Errorf("WritePushConstantLong did not panic")
		}
	}()
	b := codegen.NewBlock(&codegen.Label{Name: "e"})
	b.Add(codegen.Instruction{Opcode: codegen.PushConstantLong, Operand: int64(7)})
	s := minimalScript(t, "x", b)

	out := &recOutput{}
	w := runescript.NewBinaryScriptWriter(stubIdProvider{}, out)
	w.Write(s)
}

// TestWriteSwitch_OneCase pins the OpSwitch instruction + key/jump pair.
func TestWriteSwitch_OneCase(t *testing.T) {
	la := &codegen.Label{Name: "a"}
	lb := &codegen.Label{Name: "b"}
	ba := codegen.NewBlock(la)
	bb := codegen.NewBlock(lb)

	procTrig := &trigger.TriggerType{ID: 5, Identifier: "proc", SubjectMode: trigger.ModeName}
	ss := &symbol.ServerScriptSymbol{ScriptSymbolFields: symbol.ScriptSymbolFields{Trigger: procTrig, Name: "x", Parameters: typ.MetaUnit, Returns: typ.MetaUnit}}
	s := codegen.NewRuneScript("smoke.rs2", ss, procTrig, "x", nil)
	st := s.GenerateSwitchTable()
	st.AddCase(codegen.SwitchCase{Label: lb, Keys: []any{int(1)}})
	ba.Add(codegen.Instruction{Opcode: codegen.Switch, Operand: st})
	bb.Add(codegen.Instruction{Opcode: codegen.Return})
	s.Blocks = []*codegen.Block{ba, bb}

	out := &recOutput{}
	w := runescript.NewBinaryScriptWriter(stubIdProvider{}, out)
	w.Write(s)

	off := len(s.FullName) + 1 + len(s.SourceName) + 1 + 4 + 1 + 2
	wantIns := []byte{0x00, 0x18, 0x00, 0x00, 0x00, 0x00}
	if !bytes.Equal(out.data[off:off+len(wantIns)], wantIns) {
		t.Errorf("instruction stream = %x, want %x", out.data[off:off+len(wantIns)], wantIns)
	}

	off += 6 + 3
	off += 4
	off += 8
	off += 1
	wantSw := []byte{0x00, 0x01, 0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x00}
	if !bytes.Equal(out.data[off:off+len(wantSw)], wantSw) {
		t.Errorf("switch buffer = %x, want %x", out.data[off:off+len(wantSw)], wantSw)
	}
}

// TestWriteMath pins one math arm + the operand=0 convention.
func TestWriteMath(t *testing.T) {
	b := codegen.NewBlock(&codegen.Label{Name: "e"})
	b.Add(codegen.Instruction{Opcode: codegen.Add})
	s := minimalScript(t, "x", b)
	out := &recOutput{}
	w := runescript.NewBinaryScriptWriter(stubIdProvider{}, out)
	w.Write(s)
	off := len(s.FullName) + 1 + len(s.SourceName) + 1 + 4 + 1 + 2
	want := []byte{0x11, 0xF8, 0x00}
	if !bytes.Equal(out.data[off:off+len(want)], want) {
		t.Errorf("got %x, want %x", out.data[off:off+len(want)], want)
	}
}

// TestWrite_LookupKey_ModeName pins that the T6 stub generateLookupKey
// returns -1 for SubjectMode.Name. T7 grows the stub to handle the other
// cases; this test stays valid since ModeName always returns -1 in both
// the stub and the final implementation.
func TestWrite_LookupKey_ModeName(t *testing.T) {
	s := minimalScript(t, "x")
	s.Blocks[0].Add(codegen.Instruction{Opcode: codegen.Return})
	out := &recOutput{}
	w := runescript.NewBinaryScriptWriter(stubIdProvider{}, out)
	w.Write(s)
	off := len(s.FullName) + 1 + len(s.SourceName) + 1
	if got := int32(binary.BigEndian.Uint32(out.data[off:])); got != -1 {
		t.Errorf("lookupKey = %d, want -1 (ModeName)", got)
	}
}
