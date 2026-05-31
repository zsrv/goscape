package script

import (
	"errors"
	"path/filepath"

	"github.com/zsrv/goscape/pkg/io/packet"
)

// SwitchTable maps a signed integer key to a signed jump offset.
type SwitchTable map[int32]int32

// ScriptFile is the decoded representation of one compiled RuneScript blob.
type ScriptFile struct {
	Name       string
	SourceFile string
	FileName   string // base name of SourceFile, for diagnostics
	LookupKey  uint32 // 0xFFFFFFFF means no trigger hook

	ParamTypes []byte

	Opcodes        []Opcode
	IntOperands    []int32
	StringOperands []string
	PCs            []uint32 // instruction indices in the line table
	Lines          []uint32 // source line at the corresponding PC

	InstructionCount uint32
	IntLocalCount    uint16
	StringLocalCount uint16
	IntArgCount      uint16
	StringArgCount   uint16

	SwitchTables []SwitchTable
}

// Decode parses the raw bytes of one script blob.
//
// Critical format details (TS ScriptFile.ts is the canonical reference):
//   - lookupKey is u32 (per Engine-TS/src/engine/script/ScriptFile.ts).
//   - Trailer position: fileLen - trailerLen - 12 - 2.
//   - Operand encoding: PUSH_CONSTANT_STRING → NUL-terminated string;
//     isLargeOperand(op) → u32; else → u8.
func Decode(data []byte) (*ScriptFile, error) {
	if len(data) < 16 {
		return nil, errors.New("script: data too short (minimum 16 bytes)")
	}

	pkt := packet.NewPacket(data)
	length := len(data)

	// Last 2 bytes = u16 trailer byte length.
	pkt.Pos = length - 2
	trailerLen := int(pkt.G2())
	trailerPos := length - trailerLen - 12 - 2

	if trailerPos < 0 || trailerPos >= length {
		return nil, errors.New("script: bad trailer position")
	}

	// Parse trailer.
	pkt.Pos = trailerPos
	f := &ScriptFile{}
	f.InstructionCount = pkt.G4()
	f.IntLocalCount = pkt.G2()
	f.StringLocalCount = pkt.G2()
	f.IntArgCount = pkt.G2()
	f.StringArgCount = pkt.G2()

	switchCount := int(pkt.G1())
	f.SwitchTables = make([]SwitchTable, switchCount)
	for i := range switchCount {
		count := int(pkt.G2())
		table := make(SwitchTable, count)
		for range count {
			key := int32(pkt.G4())
			offset := int32(pkt.G4())
			table[key] = offset
		}
		f.SwitchTables[i] = table
	}

	// Parse header at the start of the blob.
	pkt.Pos = 0
	f.Name = pkt.GJStrNUL()
	f.SourceFile = pkt.GJStrNUL()
	f.FileName = filepath.Base(f.SourceFile)
	f.LookupKey = pkt.G4()

	paramCount := int(pkt.G1())
	f.ParamTypes = make([]byte, paramCount)
	for i := range paramCount {
		f.ParamTypes[i] = pkt.G1()
	}

	lineTableLen := int(pkt.G2())
	f.PCs = make([]uint32, lineTableLen)
	f.Lines = make([]uint32, lineTableLen)
	for i := range lineTableLen {
		f.PCs[i] = pkt.G4()
		f.Lines[i] = pkt.G4()
	}

	// Parse instruction stream from current pos up to trailerPos.
	instrCount := int(f.InstructionCount)
	f.Opcodes = make([]Opcode, instrCount)
	f.IntOperands = make([]int32, instrCount)
	f.StringOperands = make([]string, instrCount)

	instr := 0
	for pkt.Pos < trailerPos {
		op := Opcode(pkt.G2())

		if op == OpPushConstantString {
			f.StringOperands[instr] = pkt.GJStrNUL()
		} else if isLargeOperand(op) {
			f.IntOperands[instr] = int32(pkt.G4())
		} else {
			f.IntOperands[instr] = int32(pkt.G1())
		}

		f.Opcodes[instr] = op
		instr++
	}

	return f, nil
}

// LineNumber returns the source line for the instruction at pc by walking
// the PCs / Lines tables decoded from the script's line table. Mirrors
// TS ScriptFile.lineNumber (Engine-TS/.../ScriptFile.ts:141-149) — scan
// PCs for the first threshold strictly greater than pc and return the
// preceding line. When pc is at or past the last recorded PC, the final
// line is returned (TS's terminal fall-through).
//
// Defensive deviations from TS:
//   - Empty line table returns 0 (TS would index undefined).
//   - pc preceding the first recorded PC (PCs[0] > pc with i==0) returns
//     0 rather than TS's lines[-1] = undefined; in practice the compiler
//     always emits PCs[0] = 0, so this branch only fires on malformed
//     blobs or pc < 0.
//
// Used by the script-error backtrace path (script-core-1) to map a frame
// PC back to a source line. Closes script-core-5 (the audit-ledger flag
// "PCs/Lines decoded but never read").
func (f *ScriptFile) LineNumber(pc int) int {
	if len(f.PCs) == 0 || len(f.Lines) == 0 {
		return 0
	}
	for i, threshold := range f.PCs {
		if int(threshold) > pc {
			if i == 0 {
				return 0
			}
			return int(f.Lines[i-1])
		}
	}
	return int(f.Lines[len(f.Lines)-1])
}
