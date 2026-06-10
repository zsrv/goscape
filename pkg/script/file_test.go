package script

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"
)

// buildScript constructs a minimal script blob for testing. It accepts NUL-terminated
// name/source strings, a u32 lookupKey, and a list of (opcode, operand/string) pairs.
func buildScript(name, source string, lookupKey uint32, intArgCount, stringArgCount, intLocalCount, stringLocalCount uint16, instructions []testInstr) []byte {
	// --- Build instruction stream ---
	var instrStream []byte
	for _, ins := range instructions {
		// u16 opcode
		instrStream = append(instrStream, byte(ins.op>>8), byte(ins.op))
		if ins.op == OpPushConstantString {
			instrStream = append(instrStream, []byte(ins.str)...)
			instrStream = append(instrStream, 0) // NUL terminator
		} else if isLargeOperand(ins.op) {
			instrStream = binary.BigEndian.AppendUint32(instrStream, uint32(ins.intOp))
		} else {
			instrStream = append(instrStream, byte(ins.intOp))
		}
	}

	// --- Build trailer ---
	// The format is: [fixed 12 bytes][variable switchData][u16 varLen]
	// where fixed = u32 instructionCount + 4×u16 (local/arg counts) = 12 bytes.
	// The u16 at the very end (trailerLen field) stores only the variable switch
	// data length, NOT the fixed 12 bytes. The decoder formula is:
	//   trailerPos = fileLen - trailerLen - 12 - 2
	// which backs over the final u16 (2), the fixed section (12), and the variable
	// switch data (trailerLen) to find the start of the trailer.

	// variable switch data: just the count byte when there are no tables
	var switchData []byte
	switchData = append(switchData, 0) // switchTableCount = 0
	varLen := len(switchData)          // = 1 (count only)

	var trailer []byte
	// Fixed 12 bytes
	trailer = binary.BigEndian.AppendUint32(trailer, uint32(len(instructions)))
	trailer = binary.BigEndian.AppendUint16(trailer, intLocalCount)
	trailer = binary.BigEndian.AppendUint16(trailer, stringLocalCount)
	trailer = binary.BigEndian.AppendUint16(trailer, intArgCount)
	trailer = binary.BigEndian.AppendUint16(trailer, stringArgCount)
	// Variable switch data
	trailer = append(trailer, switchData...)
	// trailerLen is the variable portion only
	trailerLen := varLen

	// --- Build header ---
	var header []byte
	header = append(header, []byte(name)...)
	header = append(header, 0)
	header = append(header, []byte(source)...)
	header = append(header, 0)
	header = binary.BigEndian.AppendUint32(header, lookupKey)
	header = append(header, 0)                        // parameterTypeCount = 0
	header = binary.BigEndian.AppendUint16(header, 0) // lineNumberTableLength = 0

	// full blob = header + instrStream + trailer + u16(trailerLen)
	var blob []byte
	blob = append(blob, header...)
	blob = append(blob, instrStream...)
	blob = append(blob, trailer...)
	blob = binary.BigEndian.AppendUint16(blob, uint16(trailerLen))
	return blob
}

type testInstr struct {
	op    Opcode
	intOp int32
	str   string
}

func TestDecodeMinimalScript(t *testing.T) {
	// [PUSH_CONSTANT_STRING "hi", MES, RETURN]
	instrs := []testInstr{
		{op: OpPushConstantString, str: "hi"},
		{op: OpMes, intOp: 0},
		{op: OpReturn, intOp: 0},
	}
	blob := buildScript("[proc,test]", "test.rs2", 0xDEADBEEF, 0, 0, 2, 1, instrs)

	f, err := Decode(blob)
	if err != nil {
		t.Fatalf("Decode error: %v", err)
	}

	if f.Name != "[proc,test]" {
		t.Errorf("Name: got %q want %q", f.Name, "[proc,test]")
	}
	if f.SourceFile != "test.rs2" {
		t.Errorf("SourceFile: got %q want %q", f.SourceFile, "test.rs2")
	}
	if f.LookupKey != 0xDEADBEEF {
		t.Errorf("LookupKey: got %#x want %#x", f.LookupKey, uint32(0xDEADBEEF))
	}
	if len(f.Opcodes) != 3 {
		t.Fatalf("Opcodes len: got %d want 3", len(f.Opcodes))
	}
	if f.Opcodes[0] != OpPushConstantString {
		t.Errorf("Opcodes[0]: got %v want PUSH_CONSTANT_STRING", f.Opcodes[0])
	}
	if f.StringOperands[0] != "hi" {
		t.Errorf("StringOperands[0]: got %q want %q", f.StringOperands[0], "hi")
	}
	if f.Opcodes[1] != OpMes {
		t.Errorf("Opcodes[1]: got %v want MES", f.Opcodes[1])
	}
	if f.Opcodes[2] != OpReturn {
		t.Errorf("Opcodes[2]: got %v want RETURN", f.Opcodes[2])
	}
	if f.IntLocalCount != 2 {
		t.Errorf("IntLocalCount: got %d want 2", f.IntLocalCount)
	}
	if f.StringLocalCount != 1 {
		t.Errorf("StringLocalCount: got %d want 1", f.StringLocalCount)
	}
}

func TestDecodeLargeOperandOpcode(t *testing.T) {
	// PUSH_CONSTANT_INT is large-operand (opcode=0, <=100 and not in small set): u32
	instrs := []testInstr{
		{op: OpPushConstantInt, intOp: 123456},
		{op: OpReturn, intOp: 0},
	}
	blob := buildScript("test", "test.rs2", 0xFFFFFFFF, 0, 0, 0, 0, instrs)
	f, err := Decode(blob)
	if err != nil {
		t.Fatalf("Decode error: %v", err)
	}
	if f.IntOperands[0] != 123456 {
		t.Errorf("IntOperands[0]: got %d want 123456", f.IntOperands[0])
	}
}

func TestDecodeSmallOperandOpcode(t *testing.T) {
	// RETURN is small-operand: u8 (value capped at 255)
	instrs := []testInstr{
		{op: OpReturn, intOp: 42},
	}
	blob := buildScript("test", "test.rs2", 0xFFFFFFFF, 0, 0, 0, 0, instrs)
	f, err := Decode(blob)
	if err != nil {
		t.Fatalf("Decode error: %v", err)
	}
	if f.IntOperands[0] != 42 {
		t.Errorf("IntOperands[0]: got %d want 42", f.IntOperands[0])
	}
}

func TestDecodeOneSwitchTable(t *testing.T) {
	// Build a blob with one switch table containing 2 cases.
	name := "[proc,sw]"
	source := "sw.rs2"
	lookupKey := uint32(0)

	// Minimal header
	var header []byte
	header = append(header, []byte(name)...)
	header = append(header, 0)
	header = append(header, []byte(source)...)
	header = append(header, 0)
	header = binary.BigEndian.AppendUint32(header, lookupKey)
	header = append(header, 0)                        // paramCount
	header = binary.BigEndian.AppendUint16(header, 0) // lineTableLen

	// One SWITCH instruction (small operand, opcode=24)
	var instrStream []byte
	instrStream = append(instrStream, 0x00, byte(OpSwitch)) // u16 opcode
	instrStream = append(instrStream, 0)                    // u8 operand (small)

	// Trailer: fixed 12 bytes + variable switch data + u16 varLen.
	// varLen = switchTableCount(1) + caseCount(2) + 2 cases×8 bytes = 1+2+16 = 19.
	var fixedTrailer []byte
	fixedTrailer = binary.BigEndian.AppendUint32(fixedTrailer, 1) // instructionCount
	fixedTrailer = binary.BigEndian.AppendUint16(fixedTrailer, 0) // intLocalCount
	fixedTrailer = binary.BigEndian.AppendUint16(fixedTrailer, 0) // stringLocalCount
	fixedTrailer = binary.BigEndian.AppendUint16(fixedTrailer, 0) // intArgCount
	fixedTrailer = binary.BigEndian.AppendUint16(fixedTrailer, 0) // stringArgCount

	var switchData []byte
	switchData = append(switchData, 1)                         // switchTableCount=1
	switchData = binary.BigEndian.AppendUint16(switchData, 2)  // caseCount=2
	switchData = binary.BigEndian.AppendUint32(switchData, 10) // key
	switchData = binary.BigEndian.AppendUint32(switchData, 5)  // offset
	switchData = binary.BigEndian.AppendUint32(switchData, 20) // key
	switchData = binary.BigEndian.AppendUint32(switchData, 3)  // offset
	trailerLen := len(switchData)

	var trailer []byte
	trailer = append(trailer, fixedTrailer...)
	trailer = append(trailer, switchData...)
	var blob []byte
	blob = append(blob, header...)
	blob = append(blob, instrStream...)
	blob = append(blob, trailer...)
	blob = binary.BigEndian.AppendUint16(blob, uint16(trailerLen))

	f, err := Decode(blob)
	if err != nil {
		t.Fatalf("Decode error: %v", err)
	}
	if len(f.SwitchTables) != 1 {
		t.Fatalf("SwitchTables len: got %d want 1", len(f.SwitchTables))
	}
	tbl := f.SwitchTables[0]
	if tbl[10] != 5 {
		t.Errorf("SwitchTables[0][10]: got %d want 5", tbl[10])
	}
	if tbl[20] != 3 {
		t.Errorf("SwitchTables[0][20]: got %d want 3", tbl[20])
	}
}

func TestDecodeRealCacheBlob(t *testing.T) {
	datPath := filepath.Join("..", "..", "data", "pack", "server", "script.dat")
	idxPath := filepath.Join("..", "..", "data", "pack", "server", "script.idx")

	if _, err := os.Stat(datPath); os.IsNotExist(err) {
		t.Skip("real cache not present; skipping")
	}

	dat, err := os.ReadFile(datPath)
	if err != nil {
		t.Fatalf("read dat: %v", err)
	}
	idx, err := os.ReadFile(idxPath)
	if err != nil {
		t.Fatalf("read idx: %v", err)
	}

	// Header: dat = u32 entryCount + u32 version (8 bytes); idx = u32
	// entryCount only (4 bytes). The original version of this walk assumed
	// an 8-byte idx header, sliced a Frankenblob across two scripts, and
	// Decode rejected it with "bad trailer position" — that test bug was
	// fixed at 0a068e40 but lived on in the porting trackers as a phantom
	// "decoder residual" until retired at rev-245.2 (decoder verified
	// against 32,826 blobs across four era-caches, zero failures).
	if len(dat) < 8 {
		t.Fatal("dat file too short")
	}
	entryCount := int(binary.BigEndian.Uint32(dat[0:4]))
	_ = binary.BigEndian.Uint32(dat[4:8]) // version
	idxOffset := 4
	datOffset := 8

	// Decode EVERY entry (first-blob-only coverage let the walk bug above
	// masquerade as a decoder bug; the full sweep costs ~10ms).
	decoded := 0
	for id := range entryCount {
		if idxOffset+4 > len(idx) {
			t.Fatalf("idx truncated at entry %d", id)
		}
		size := int(binary.BigEndian.Uint32(idx[idxOffset : idxOffset+4]))
		idxOffset += 4
		if size == 0 {
			continue
		}
		if datOffset+size > len(dat) {
			t.Fatalf("dat truncated: id %d needs %d bytes at offset %d", id, size, datOffset)
		}
		blob := dat[datOffset : datOffset+size]
		datOffset += size
		f, err := Decode(blob)
		if err != nil {
			t.Fatalf("Decode real cache blob id %d: %v", id, err)
		}
		if f.Name == "" {
			t.Errorf("script id %d decoded with empty name", id)
		}
		decoded++
	}
	if decoded == 0 {
		t.Fatal("no non-zero entries found in idx")
	}
	// Every dat byte must be accounted for — a walk misalignment that
	// happens to produce decodable slices would still trip this.
	if datOffset != len(dat) {
		t.Errorf("dat not fully consumed: walked %d of %d bytes", datOffset, len(dat))
	}
	t.Logf("decoded %d scripts, %d bytes", decoded, len(dat))
}

// TestScriptFileLineNumber pins the LineNumber(pc) accessor (script-core-5,
// the consequence-of-script-core-1 line-mapping closure). Mirrors TS
// ScriptFile.lineNumber semantics: scan PCs for the first threshold strictly
// greater than pc and return the preceding line; pc past the last threshold
// returns the final line. Edge cases (empty table, pc before first PC)
// degrade defensively to 0 — TS would return undefined.
func TestScriptFileLineNumber(t *testing.T) {
	// Synthetic line table: PCs={0, 5, 12, 20}, Lines={10, 11, 12, 13}.
	// PC 0..4 → line 10; PC 5..11 → line 11; PC 12..19 → line 12; PC 20+ → line 13.
	f := &ScriptFile{
		PCs:   []uint32{0, 5, 12, 20},
		Lines: []uint32{10, 11, 12, 13},
	}

	tests := []struct {
		pc   int
		want int
	}{
		{0, 10},   // first instruction
		{4, 10},   // last instruction at threshold 0
		{5, 11},   // jump to threshold 5
		{11, 11},  // last instruction at threshold 5
		{12, 12},  // jump to threshold 12
		{19, 12},  // last instruction at threshold 12
		{20, 13},  // jump to threshold 20
		{999, 13}, // past last threshold → final line
		{-1, 0},   // before first threshold → defensive 0 (TS undefined)
	}
	for _, tc := range tests {
		if got := f.LineNumber(tc.pc); got != tc.want {
			t.Errorf("LineNumber(%d) = %d, want %d", tc.pc, got, tc.want)
		}
	}

	// Empty table degrades defensively.
	empty := &ScriptFile{}
	if got := empty.LineNumber(0); got != 0 {
		t.Errorf("LineNumber on empty table = %d, want 0", got)
	}
}
