// pkg/pack/compiler/runescript/binary_context_test.go
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

func newEmptyScript(t *testing.T) *codegen.RuneScript {
	t.Helper()
	procTrig := &trigger.TriggerType{ID: 0, Identifier: "proc", SubjectMode: trigger.ModeName, AllowParameters: true, AllowReturns: true}
	ss := &symbol.ServerScriptSymbol{ScriptSymbolFields: symbol.ScriptSymbolFields{
		Trigger:    procTrig,
		Name:       "foo",
		Parameters: typ.MetaUnit,
		Returns:    typ.MetaUnit,
	}}
	s := codegen.NewRuneScript("smoke.rs2", ss, procTrig, "foo", nil)
	s.Blocks = []*codegen.Block{codegen.NewBlock(&codegen.Label{Name: "e"})}
	return s
}

// TestBinaryContext_InstructionLargeOperand pins that Instruction writes
// opcode (2 BE bytes) + operand (4 BE bytes) for a LargeOperand opcode.
func TestBinaryContext_InstructionLargeOperand(t *testing.T) {
	s := newEmptyScript(t)
	ctx := runescript.NewBinaryScriptWriterContext(s, 0)
	ctx.Instruction(writer.OpPushConstantInt, 42)

	want := []byte{0x00, 0x00, 0x00, 0x00, 0x00, 0x2A} // opcode=0, operand=0x2A
	if got := ctx.InstructionBytesForTest(); !bytes.Equal(got, want) {
		t.Errorf("instruction bytes = %x, want %x", got, want)
	}
}

// TestBinaryContext_InstructionSmallOperand pins 2 BE bytes + 1 byte.
func TestBinaryContext_InstructionSmallOperand(t *testing.T) {
	s := newEmptyScript(t)
	ctx := runescript.NewBinaryScriptWriterContext(s, 0)
	ctx.Instruction(writer.OpReturn, 0)

	want := []byte{0x00, 0x15, 0x00} // opcode=21, operand=0
	if got := ctx.InstructionBytesForTest(); !bytes.Equal(got, want) {
		t.Errorf("instruction bytes = %x, want %x", got, want)
	}
}

// TestBinaryContext_InstructionString pins opcode (2 BE) + string-bytes +
// NUL terminator. TS uses charCodeAt & 0xff per byte.
func TestBinaryContext_InstructionString(t *testing.T) {
	s := newEmptyScript(t)
	ctx := runescript.NewBinaryScriptWriterContext(s, 0)
	ctx.InstructionString(writer.OpPushConstantString, "hi")

	want := []byte{0x00, 0x03, 'h', 'i', 0x00}
	if got := ctx.InstructionBytesForTest(); !bytes.Equal(got, want) {
		t.Errorf("instruction bytes = %x, want %x", got, want)
	}
}

// TestBinaryContext_InstructionString_BmpCharCodeTruncation pins the TS
// charCodeAt(i) & 0xff semantics for non-ASCII BMP characters. U+2019
// (right single quotation mark, e.g. in "you're") is the canonical case
// in Content: JS yields one UTF-16 code unit 0x2019, truncated to 0x19.
// Go must iterate by rune (not byte) and take the low byte; iterating
// over the UTF-8 byte slice would write 0xE2 0x80 0x99.
func TestBinaryContext_InstructionString_BmpCharCodeTruncation(t *testing.T) {
	s := newEmptyScript(t)
	ctx := runescript.NewBinaryScriptWriterContext(s, 0)
	ctx.InstructionString(writer.OpPushConstantString, "a’b")

	want := []byte{0x00, 0x03, 'a', 0x19, 'b', 0x00}
	if got := ctx.InstructionBytesForTest(); !bytes.Equal(got, want) {
		t.Errorf("instruction bytes = %x, want %x", got, want)
	}
}

// TestBinaryContext_InstructionRaw pins opcode + 1-byte operand for raw form.
func TestBinaryContext_InstructionRaw(t *testing.T) {
	s := newEmptyScript(t)
	ctx := runescript.NewBinaryScriptWriterContext(s, 0)
	ctx.InstructionRaw(0x1234, 0x7F)

	want := []byte{0x12, 0x34, 0x7F}
	if got := ctx.InstructionBytesForTest(); !bytes.Equal(got, want) {
		t.Errorf("instruction bytes = %x, want %x", got, want)
	}
}

// TestBinaryContext_SwitchPlaceholderBackpatch pins the random-access fix-up
// of the 2-byte placeholder at sizePos. Two cases → key-count of 2.
func TestBinaryContext_SwitchPlaceholderBackpatch(t *testing.T) {
	s := newEmptyScript(t)
	ctx := runescript.NewBinaryScriptWriterContext(s, 0)
	ctx.Switch(0, func() int {
		ctx.SwitchCase(1, 10)
		ctx.SwitchCase(2, 20)
		return 2
	})

	sw := ctx.SwitchBytesForTest()
	// Layout: 2 BE bytes placeholder (now =2), then 2× (4 BE key, 4 BE jump).
	want := []byte{
		0x00, 0x02, // total key count
		0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x0A,
		0x00, 0x00, 0x00, 0x02, 0x00, 0x00, 0x00, 0x14,
	}
	if !bytes.Equal(sw, want) {
		t.Errorf("switch bytes = %x, want %x", sw, want)
	}
}

// TestBinaryContext_FinishHeaderLayout pins the header bytes emitted by
// Finish() for a 1-instruction script (PushConstantInt 42; Return).
//
// Layout (TS BinaryScriptWriterContext.finish L123-179):
//
//	fullName  null-terminated string
//	sourceName null-terminated string
//	lookupKey       int32 BE
//	debugproc-zero  uint8 (0 because trigger != DEBUGPROC)
//	lineNumberCount uint16 BE
//	instructionBuffer (variable)
//	instructionCount  int32 BE
//	intLocals uint16 BE
//	strLocals uint16 BE
//	intParams uint16 BE
//	strParams uint16 BE
//	switchTableCount  uint8
//	switchBuffer (variable)
//	switchEnd uint16 BE (= switchOffset + 1)
func TestBinaryContext_FinishHeaderLayout(t *testing.T) {
	s := newEmptyScript(t)
	s.Blocks[0].Add(codegen.Instruction{Opcode: codegen.PushConstantInt, Operand: 42})
	s.Blocks[0].Add(codegen.Instruction{Opcode: codegen.Return})

	ctx := runescript.NewBinaryScriptWriterContext(s, 0x12345678)
	// Manually emit two instructions to populate ctx.
	ctx.Instruction(writer.OpPushConstantInt, 42)
	ctx.Instruction(writer.OpReturn, 0)

	buf := ctx.Finish()

	var off int
	// fullName: "[proc,foo]\x00"
	expFull := "[proc,foo]"
	if string(buf[off:off+len(expFull)]) != expFull || buf[off+len(expFull)] != 0 {
		t.Fatalf("fullName mismatch at offset 0: %q", buf[:len(expFull)+1])
	}
	off += len(expFull) + 1
	// sourceName: "smoke.rs2\x00"
	expSrc := "smoke.rs2"
	if string(buf[off:off+len(expSrc)]) != expSrc || buf[off+len(expSrc)] != 0 {
		t.Fatalf("sourceName mismatch at offset %d", off)
	}
	off += len(expSrc) + 1
	// lookupKey
	if got := int32(binary.BigEndian.Uint32(buf[off:])); got != 0x12345678 {
		t.Errorf("lookupKey = %#x, want %#x", uint32(got), uint32(0x12345678))
	}
	off += 4
	// debugproc-zero
	if buf[off] != 0 {
		t.Errorf("debugproc-zero = %#x, want 0", buf[off])
	}
	off++
	// lineNumberCount = 0 (no line info on synthesised instructions)
	if got := binary.BigEndian.Uint16(buf[off:]); got != 0 {
		t.Errorf("lineNumberCount = %d, want 0", got)
	}
	off += 2
	// instructionBuffer: opcode 0x0000 + operand 0x0000002A (push 42)
	//                  + opcode 0x0015 + operand 0x00 (return)
	wantIns := []byte{0x00, 0x00, 0x00, 0x00, 0x00, 0x2A, 0x00, 0x15, 0x00}
	if !bytes.Equal(buf[off:off+len(wantIns)], wantIns) {
		t.Errorf("instructionBuffer = %x, want %x", buf[off:off+len(wantIns)], wantIns)
	}
	off += len(wantIns)
	// instructionCount = 2
	if got := int32(binary.BigEndian.Uint32(buf[off:])); got != 2 {
		t.Errorf("instructionCount = %d, want 2", got)
	}
	off += 4
	// intLocals=0, strLocals=0, intParams=0, strParams=0
	for i, label := range []string{"intLocals", "strLocals", "intParams", "strParams"} {
		if got := binary.BigEndian.Uint16(buf[off:]); got != 0 {
			t.Errorf("%s (idx %d) = %d, want 0", label, i, got)
		}
		off += 2
	}
	// switchTableCount = 0
	if buf[off] != 0 {
		t.Errorf("switchTableCount = %d, want 0", buf[off])
	}
	off++
	// switchEnd = switchOffset+1 = 0+1 = 1
	if got := binary.BigEndian.Uint16(buf[off:]); got != 1 {
		t.Errorf("switchEnd = %d, want 1", got)
	}
}

// TestBinaryContext_FinishDebugproc pins the debugproc-parameter-codes
// path: trigger.Identifier == "debugproc" → emit param-count byte + param
// type-code bytes (signed int8). NAI-209-D-DEBUGPROC-TRIGGER-STRING-CHECK.
func TestBinaryContext_FinishDebugproc(t *testing.T) {
	debugproc := &trigger.TriggerType{ID: 1, Identifier: "debugproc", SubjectMode: trigger.ModeName, AllowParameters: true}
	tup, _ := typ.NewTupleType(typ.PrimitiveInt, typ.PrimitiveString)
	ss := &symbol.ServerScriptSymbol{ScriptSymbolFields: symbol.ScriptSymbolFields{
		Trigger:    debugproc,
		Name:       "x",
		Parameters: tup,
		Returns:    typ.MetaUnit,
	}}
	s := codegen.NewRuneScript("smoke.rs2", ss, debugproc, "x", nil)
	s.Blocks = []*codegen.Block{codegen.NewBlock(&codegen.Label{Name: "e"})}
	ctx := runescript.NewBinaryScriptWriterContext(s, 0)
	buf := ctx.Finish()

	// fullName "[debugproc,x]\x00" + sourceName "smoke.rs2\x00" + 4 BE lookupKey
	off := len("[debugproc,x]") + 1 + len("smoke.rs2") + 1 + 4
	if buf[off] != 2 {
		t.Errorf("debugproc paramCount = %d, want 2", buf[off])
	}
	off++
	// param[0] is PrimitiveInt: code='i' (0x69); param[1] is PrimitiveString: code='s' (0x73)
	if buf[off] != 'i' {
		t.Errorf("param[0] code = %#x, want 'i'", buf[off])
	}
	if buf[off+1] != 's' {
		t.Errorf("param[1] code = %#x, want 's'", buf[off+1])
	}
}
