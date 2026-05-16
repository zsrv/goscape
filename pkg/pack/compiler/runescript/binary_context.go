// pkg/pack/compiler/runescript/binary_context.go
package runescript

import (
	"encoding/binary"

	"github.com/zsrv/goscape/pkg/pack/compiler/codegen"
	"github.com/zsrv/goscape/pkg/pack/compiler/symbol"
	typ "github.com/zsrv/goscape/pkg/pack/compiler/type"
	"github.com/zsrv/goscape/pkg/pack/compiler/writer"
)

// binaryCtxInitialCapacity matches TS BinaryScriptWriterContext.INITIAL_CAPACITY
// (the average OSRS script size rounded to the next power of two).
const binaryCtxInitialCapacity = 512

// BinaryScriptWriterContext is the concrete writer context for the binary
// on-disk script format. Embeds *writer.BaseContext for CurIndex + line/jump
// tables. Mirrors TS BinaryScriptWriterContext.ts.
type BinaryScriptWriterContext struct {
	*writer.BaseContext

	LookupKey         int32
	instructionBuffer []byte
	switchBuffer      []byte
	instructionCount  int
	instructionOffset int
	switchOffset      int
}

// NewBinaryScriptWriterContext allocates instruction + switch buffers at the
// initial capacity. lookupKey is computed by the caller (binary_writer.go)
// before construction.
func NewBinaryScriptWriterContext(script *codegen.RuneScript, lookupKey int32) *BinaryScriptWriterContext {
	return &BinaryScriptWriterContext{
		BaseContext:       writer.NewBaseContext(script),
		LookupKey:         lookupKey,
		instructionBuffer: make([]byte, binaryCtxInitialCapacity),
		switchBuffer:      make([]byte, binaryCtxInitialCapacity),
	}
}

func (c *BinaryScriptWriterContext) ensureInstruction(extra int) {
	for c.instructionOffset+extra > len(c.instructionBuffer) {
		c.instructionBuffer = append(c.instructionBuffer, make([]byte, len(c.instructionBuffer))...)
	}
}

func (c *BinaryScriptWriterContext) ensureSwitch(extra int) {
	for c.switchOffset+extra > len(c.switchBuffer) {
		c.switchBuffer = append(c.switchBuffer, make([]byte, len(c.switchBuffer))...)
	}
}

// Instruction emits opcode.ID (2 BE bytes) + operand (4 BE bytes if
// opcode.LargeOperand, else 1 byte). Mirrors TS L63-78.
func (c *BinaryScriptWriterContext) Instruction(op *writer.ServerScriptOpcode, operand int32) {
	c.instructionCount++
	size := 4
	if op.LargeOperand {
		size = 6
	}
	c.ensureInstruction(size)
	binary.BigEndian.PutUint16(c.instructionBuffer[c.instructionOffset:], op.ID)
	c.instructionOffset += 2
	if op.LargeOperand {
		binary.BigEndian.PutUint32(c.instructionBuffer[c.instructionOffset:], uint32(operand))
		c.instructionOffset += 4
	} else {
		c.instructionBuffer[c.instructionOffset] = byte(operand & 0xff)
		c.instructionOffset++
	}
}

// InstructionRaw emits a 2-BE-byte opcode + 1-byte operand (no
// LargeOperand-aware sizing). Used by WriteCommand which carries the
// numeric command-id directly. Mirrors TS L80-89.
func (c *BinaryScriptWriterContext) InstructionRaw(opcode, operand int) {
	c.instructionCount++
	c.ensureInstruction(3)
	binary.BigEndian.PutUint16(c.instructionBuffer[c.instructionOffset:], uint16(opcode))
	c.instructionOffset += 2
	c.instructionBuffer[c.instructionOffset] = byte(operand & 0xff)
	c.instructionOffset++
}

// InstructionString emits a 2-BE-byte opcode followed by the operand string,
// each byte being `charCodeAt(i) & 0xff` (TS), terminated by 0x00.
// Mirrors TS L91-101.
func (c *BinaryScriptWriterContext) InstructionString(op *writer.ServerScriptOpcode, operand string) {
	c.instructionCount++
	c.ensureInstruction(2 + len(operand) + 1)
	binary.BigEndian.PutUint16(c.instructionBuffer[c.instructionOffset:], op.ID)
	c.instructionOffset += 2
	for i := 0; i < len(operand); i++ {
		c.instructionBuffer[c.instructionOffset] = operand[i] & 0xff
		c.instructionOffset++
	}
	c.instructionBuffer[c.instructionOffset] = 0
	c.instructionOffset++
}

// Switch emits OpSwitch (with the switch table ID as operand) into the
// instruction stream, then sets up a placeholder for the total key count in
// the switch buffer, invokes block (which writes SwitchCase entries), and
// finally back-patches the placeholder. Mirrors TS L103-112.
func (c *BinaryScriptWriterContext) Switch(id int, block func() int) {
	c.Instruction(writer.OpSwitch, int32(id))
	sizePos := c.switchOffset
	c.ensureSwitch(2)
	c.switchOffset += 2 // placeholder
	total := block()
	binary.BigEndian.PutUint16(c.switchBuffer[sizePos:], uint16(total))
}

// SwitchCase emits one (key int32 BE, jump int32 BE) pair. Mirrors TS L114-121.
func (c *BinaryScriptWriterContext) SwitchCase(key, jump int32) {
	c.ensureSwitch(8)
	binary.BigEndian.PutUint32(c.switchBuffer[c.switchOffset:], uint32(key))
	c.switchOffset += 4
	binary.BigEndian.PutUint32(c.switchBuffer[c.switchOffset:], uint32(jump))
	c.switchOffset += 4
}

// Finish assembles the final binary blob in TS BinaryScriptWriterContext.finish
// header layout (L123-179). Returns a freshly allocated []byte.
//
// NAI-209-D-DEBUGPROC-TRIGGER-STRING-CHECK: the DEBUGPROC trigger singleton
// is not yet ported to goscape; comparison uses `Trigger.Identifier ==
// "debugproc"` for parity.
func (c *BinaryScriptWriterContext) Finish() []byte {
	script := c.Script
	var buf []byte

	buf = appendNULString(buf, script.FullName)
	buf = appendNULString(buf, script.SourceName)
	buf = binary.BigEndian.AppendUint32(buf, uint32(c.LookupKey))

	if script.Trigger != nil && script.Trigger.Identifier == "debugproc" {
		params := paramCodes(script)
		buf = append(buf, byte(len(params)))
		for _, code := range params {
			buf = append(buf, byte(int8(code)))
		}
	} else {
		buf = append(buf, 0)
	}

	buf = binary.BigEndian.AppendUint16(buf, uint16(len(c.LineNumberPCs)))
	for _, pc := range c.LineNumberPCs {
		line := c.LineNumberTable[pc]
		buf = binary.BigEndian.AppendUint32(buf, uint32(int32(pc)))
		buf = binary.BigEndian.AppendUint32(buf, uint32(int32(line)))
	}

	buf = append(buf, c.instructionBuffer[:c.instructionOffset]...)
	buf = binary.BigEndian.AppendUint32(buf, uint32(int32(c.instructionCount)))

	locals := script.Locals
	buf = binary.BigEndian.AppendUint16(buf, uint16(writer.GetLocalCount(locals, typ.BaseVarInteger)))
	buf = binary.BigEndian.AppendUint16(buf, uint16(writer.GetLocalCount(locals, typ.BaseVarString)))
	buf = binary.BigEndian.AppendUint16(buf, uint16(writer.GetParameterCount(locals, typ.BaseVarInteger)))
	buf = binary.BigEndian.AppendUint16(buf, uint16(writer.GetParameterCount(locals, typ.BaseVarString)))

	buf = append(buf, byte(len(script.SwitchTables)))
	buf = append(buf, c.switchBuffer[:c.switchOffset]...)
	buf = binary.BigEndian.AppendUint16(buf, uint16(c.switchOffset+1))

	return buf
}

// paramCodes returns the per-parameter char-code list for a DEBUGPROC's
// Parameters field. Mirrors TS BinaryScriptWriterContext.finish L136-141
// (TupleType.toList + code?.charCodeAt(0) ?? -1).
func paramCodes(script *codegen.RuneScript) []int {
	ss, ok := script.Symbol.(*symbol.ServerScriptSymbol)
	if !ok || ss.Parameters == nil {
		return nil
	}
	params := typ.TupleToList(ss.Parameters)
	out := make([]int, len(params))
	for i, p := range params {
		code, ok := p.Code()
		if !ok || code == "" {
			out[i] = -1
		} else {
			out[i] = int(code[0])
		}
	}
	return out
}

// appendNULString appends each byte of s (low 8 bits, per TS L207-213) plus
// a trailing 0x00.
func appendNULString(buf []byte, s string) []byte {
	for i := 0; i < len(s); i++ {
		buf = append(buf, s[i]&0xff)
	}
	return append(buf, 0)
}

// InstructionBytesForTest returns a copy of the instruction buffer's used
// prefix. Test-only inspector; consumers in production code use Finish().
// Naming follows the `*ForTest` convention from
// pkg/gamemap.(*GameMap).SetLandBytesForTest (NAI-151).
func (c *BinaryScriptWriterContext) InstructionBytesForTest() []byte {
	out := make([]byte, c.instructionOffset)
	copy(out, c.instructionBuffer)
	return out
}

// SwitchBytesForTest returns a copy of the switch buffer's used prefix.
func (c *BinaryScriptWriterContext) SwitchBytesForTest() []byte {
	out := make([]byte, c.switchOffset)
	copy(out, c.switchBuffer)
	return out
}
