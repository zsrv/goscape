package objtype

import (
	"testing"

	"github.com/zsrv/goscape/pkg/io/packet"
)

// buildAnimFrameBlob constructs a minimal FileStream archive-2 blob that
// unpackAnimFrames can decode. The blob layout (per AnimFrame.ts:27-44) is:
//
//	[head section][tran1 section][tran2 section][del section][base data]
//	[8-byte trailer: g2=headLen, g2=tran1Len, g2=tran2Len, g2=delLen]
//
// We build the simplest possible frame: 1 frame, 0 transform groups,
// so tran1/tran2 are empty. The AnimBase has 0 types (length=0).
func buildAnimFrameBlob(frameID int, delay int) []byte {
	// head section: g2(total=1) + g2(frameID) + g1(groupCount=0)
	headBuf := packet.NewPacket(nil)
	headBuf.P2(1)               // total frames in blob
	headBuf.P2(uint16(frameID)) // frame id
	headBuf.P1(0)               // groupCount = 0 (no transforms)
	headBytes := headBuf.Bytes()

	// del section: g1(delay)
	delBuf := packet.NewPacket(nil)
	delBuf.P1(uint8(delay))
	delBytes := delBuf.Bytes()

	// base data: AnimBase with length=0 (g1=0)
	baseBuf := packet.NewPacket(nil)
	baseBuf.P1(0) // length = 0
	baseBytes := baseBuf.Bytes()

	// tran1 and tran2 are empty (no groups)
	// The head section length in the trailer is len(headBytes)-2 (excludes the
	// leading g2(total) which is not in the "section length" but IS included in
	// the head buffer for reading). Per AnimFrame.ts:31-32:
	//   offset += meta.g2() + 2
	// so meta.g2() = len(headBytes) - 2.
	headLen := len(headBytes) - 2
	tran1Len := 0
	tran2Len := 0
	delLen := len(delBytes)

	// Assemble: [head][tran1][tran2][del][base][trailer]
	trailer := packet.NewPacket(nil)
	trailer.P2(uint16(headLen))
	trailer.P2(uint16(tran1Len))
	trailer.P2(uint16(tran2Len))
	trailer.P2(uint16(delLen))
	trailerBytes := trailer.Bytes()

	total := len(headBytes) + len(delBytes) + len(baseBytes) + len(trailerBytes)
	blob := make([]byte, total)
	pos := 0
	copy(blob[pos:], headBytes)
	pos += len(headBytes)
	// tran1 and tran2 are empty (0 bytes)
	copy(blob[pos:], delBytes)
	pos += len(delBytes)
	copy(blob[pos:], baseBytes)
	pos += len(baseBytes)
	copy(blob[pos:], trailerBytes)
	return blob
}

// TestAnimFrame_UnpackSimple verifies that a minimal blob with one frame
// is decoded into AnimFrameConfigs with the correct delay.
// TS AnimFrame.ts:26-130 (Engine-TS 9aadcec4)
func TestAnimFrame_UnpackSimple(t *testing.T) {
	blob := buildAnimFrameBlob(3, 5) // id=3, delay=5
	cfg := &AnimFrameConfigs{}
	unpackAnimFrames(cfg, blob)

	if len(cfg.Instances) < 4 {
		t.Fatalf("Instances len: got %d, want >=4 (frame id 3 requires index 3)", len(cfg.Instances))
	}
	frame := cfg.Instances[3]
	if frame == nil {
		t.Fatalf("Instances[3]: got nil, want non-nil")
	}
	if frame.Delay != 5 {
		t.Errorf("Instances[3].Delay: got %d, want 5", frame.Delay)
	}
	if frame.Length != 0 {
		t.Errorf("Instances[3].Length: got %d, want 0 (no transforms)", frame.Length)
	}
}

// TestAnimFrame_UnpackOrder verifies that Order is populated correctly.
// TS AnimFrame.ts:50-51: AnimFrame.order.push(id)
func TestAnimFrame_UnpackOrder(t *testing.T) {
	blob := buildAnimFrameBlob(7, 3) // id=7
	cfg := &AnimFrameConfigs{}
	unpackAnimFrames(cfg, blob)

	if len(cfg.Order) != 1 || cfg.Order[0] != 7 {
		t.Errorf("Order: got %v, want [7]", cfg.Order)
	}
}

// TestAnimFrame_NilInstancesForMissingIDs verifies that sparse ids result in
// nil slots in Instances (matching TS AnimFrame.instances[id] = frame where
// absent ids are undefined/nil).
func TestAnimFrame_NilInstancesForMissingIDs(t *testing.T) {
	blob := buildAnimFrameBlob(5, 2) // id=5, so [0..4] must be nil
	cfg := &AnimFrameConfigs{}
	unpackAnimFrames(cfg, blob)

	if len(cfg.Instances) < 6 {
		t.Fatalf("Instances len: got %d, want >=6", len(cfg.Instances))
	}
	for i := range 5 {
		if cfg.Instances[i] != nil {
			t.Errorf("Instances[%d]: got non-nil, want nil (sparse)", i)
		}
	}
}

// buildAnimFrameTransformBlob constructs a FileStream archive-2 blob that
// exercises the groupCount>0 inner loop in unpackAnimFrames — specifically:
//   - OP_BASE filler-insertion path (TS AnimFrame.ts:65-72)
//   - gsmart 1-byte form (byte < 128: value = byte - 64)
//   - gsmart 2-byte form (byte >= 128: value = g2 - 49152)
//   - OP_SCALE default 128 (TS AnimFrame.ts:76-79)
//   - flags==0 skip/continue path (TS AnimFrame.ts:61-63)
//
// AnimBase layout (4 groups):
//
//	group 0: type=0 (OP_BASE)
//	group 1: type=1 (OP_TRANSLATE)
//	group 2: type=3 (OP_SCALE)
//	group 3: type=1 (OP_TRANSLATE)
//
// Frame layout (1 frame, id=2, delay=4, groupCount=4):
//
//	group=0: flags=0 → skip (continue)
//	group=1: flags=0x7 → NOT OP_BASE; search back: cur=0 → types[0]=OP_BASE → filler at length=0
//	           then slot 1 (bases=1): x=gsmart(5,1-byte), y=gsmart(200,2-byte), z=gsmart(-3,1-byte)
//	group=2: flags=0x1 → NOT OP_BASE; search back: cur=1 types[1]=1≠OP_BASE, cur=0 types[0]=OP_BASE,
//	           lastGroup=1 so cur=1 > 1 is false? No: lastGroup after group=1 is 1, so cur starts at
//	           group-1=1. 1 > 1 is false, loop exits immediately — no filler.
//	           Wait — that is wrong. Let me re-derive: after group=1 ran, lastGroup=1, length=2.
//	           For group=2: search from cur=group-1=1 down while cur>lastGroup=1: 1>1 is false → no filler.
//	           slot 2 (bases=2): OP_SCALE → defaultValue=128; flags=0x1: x=gsmart(10,1-byte), y=128, z=128.
//	group=3: flags=0x2 → NOT OP_BASE; search from cur=2: 2>lastGroup=2 is false → no filler.
//	           slot 3 (bases=3): OP_TRANSLATE → defaultValue=0; flags=0x2: x=0, y=gsmart(7,1-byte), z=0.
//
// Hand-derived expected output (TS AnimFrame.ts algorithm, Engine-TS 9aadcec4):
//
//	Length = 4
//	Groups = [0,  1,   2,   3  ]   (slot 0: filler OP_BASE, slots 1-3: real groups)
//	X      = [0,  5,  10,   0  ]
//	Y      = [0, 200, 128,  7  ]
//	Z      = [0,  -3, 128,  0  ]
//
// NOTE: For group=2, the filler search (cur=group-1=1, condition cur>lastGroup=1) evaluates
// 1>1=false immediately, so NO filler is emitted — there are only 4 output slots not 5.
// The OP_BASE filler for group=1 accounts for the only filler slot (slot 0).
func buildAnimFrameTransformBlob() []byte {
	// ---- AnimBase section ----
	// TS AnimBase.ts:18-34 (Engine-TS 9aadcec4)
	// length=4, types=[0,1,3,1], labels=[{0 labels},{0 labels},{0 labels},{0 labels}]
	baseBuf := packet.NewPacket(nil)
	baseBuf.P1(4) // length = 4 groups
	baseBuf.P1(0) // types[0] = OP_BASE (0)
	baseBuf.P1(1) // types[1] = OP_TRANSLATE (1)
	baseBuf.P1(3) // types[2] = OP_SCALE (3)
	baseBuf.P1(1) // types[3] = OP_TRANSLATE (1)
	// labelCount for each group = 0 (no label bytes)
	baseBuf.P1(0)
	baseBuf.P1(0)
	baseBuf.P1(0)
	baseBuf.P1(0)
	baseBytes := baseBuf.Bytes()

	// ---- tran2 section: gsmart values for the non-default slots ----
	// group=1 flags=0x7: x=5(1-byte), y=200(2-byte), z=-3(1-byte)
	// group=2 flags=0x1: x=10(1-byte)  [y,z use default 128 — not read]
	// group=3 flags=0x2: y=7(1-byte)   [x,z use default 0 — not read]
	//
	// GSmart 1-byte encoding: write byte(v+64). Range [-64,63].
	//   v=5:   5+64=69   (< 128 ✓)
	//   v=-3: -3+64=61   (< 128 ✓)
	//   v=10: 10+64=74   (< 128 ✓)
	//   v=7:   7+64=71   (< 128 ✓)
	// GSmart 2-byte encoding: write uint16(v+49152). First byte must be >= 128.
	//   v=200: 200+49152=49352=0xC0C8. First byte=0xC0=192 (>= 128 ✓). Decode: 49352-49152=200 ✓
	tran2Buf := packet.NewPacket(nil)
	tran2Buf.P1(69)     // gsmart(5): group=1 x, 1-byte form
	tran2Buf.P2(0xC0C8) // gsmart(200): group=1 y, 2-byte form (49352=200+49152)
	tran2Buf.P1(61)     // gsmart(-3): group=1 z, 1-byte form
	tran2Buf.P1(74)     // gsmart(10): group=2 x, 1-byte form
	tran2Buf.P1(71)     // gsmart(7): group=3 y, 1-byte form
	tran2Bytes := tran2Buf.Bytes()

	// ---- tran1 section: flags per group (groupCount=4) ----
	// group=0: flags=0  (skip)
	// group=1: flags=7  (0x1|0x2|0x4, read x+y+z)
	// group=2: flags=1  (0x1, read x; y,z get OP_SCALE default 128)
	// group=3: flags=2  (0x2, read y; x,z get OP_TRANSLATE default 0)
	tran1Buf := packet.NewPacket(nil)
	tran1Buf.P1(0)
	tran1Buf.P1(7)
	tran1Buf.P1(1)
	tran1Buf.P1(2)
	tran1Bytes := tran1Buf.Bytes()

	// ---- head section: g2(total=1) + g2(frameID=2) + g1(groupCount=4) ----
	headBuf := packet.NewPacket(nil)
	headBuf.P2(1) // total = 1 frame
	headBuf.P2(2) // frame id = 2
	headBuf.P1(4) // groupCount = 4
	headBytes := headBuf.Bytes()

	// ---- del section: g1(delay=4) ----
	delBuf := packet.NewPacket(nil)
	delBuf.P1(4)
	delBytes := delBuf.Bytes()

	// ---- Trailer: 8 bytes = g2(headLen) g2(tran1Len) g2(tran2Len) g2(delLen) ----
	// headLen = len(headBytes)-2 per AnimFrame.ts:31-32 (offset += meta.g2() + 2)
	headLen := len(headBytes) - 2
	tran1Len := len(tran1Bytes)
	tran2Len := len(tran2Bytes)
	delLen := len(delBytes)

	trailer := packet.NewPacket(nil)
	trailer.P2(uint16(headLen))
	trailer.P2(uint16(tran1Len))
	trailer.P2(uint16(tran2Len))
	trailer.P2(uint16(delLen))
	trailerBytes := trailer.Bytes()

	// Assemble: [head][tran1][tran2][del][base][trailer]
	var blob []byte
	blob = append(blob, headBytes...)
	blob = append(blob, tran1Bytes...)
	blob = append(blob, tran2Bytes...)
	blob = append(blob, delBytes...)
	blob = append(blob, baseBytes...)
	blob = append(blob, trailerBytes...)
	return blob
}

// TestAnimFrame_UnpackTransformGroups exercises the groupCount>0 inner loop in
// unpackAnimFrames: OP_BASE filler insertion, gsmart 1-byte and 2-byte forms,
// OP_SCALE defaultValue=128, and the flags==0 skip path.
//
// Expected values are derived by hand from TS AnimFrame.ts:58-116 (Engine-TS
// 9aadcec4) — see buildAnimFrameTransformBlob for full derivation comments.
//
//	Length = 4
//	Groups = [0, 1, 2, 3]
//	X      = [0, 5, 10, 0]
//	Y      = [0, 200, 128, 7]
//	Z      = [0, -3, 128, 0]
func TestAnimFrame_UnpackTransformGroups(t *testing.T) {
	blob := buildAnimFrameTransformBlob()
	cfg := &AnimFrameConfigs{}
	unpackAnimFrames(cfg, blob)

	if len(cfg.Instances) < 3 {
		t.Fatalf("Instances len: got %d, want >=3 (frame id 2 requires index 2)", len(cfg.Instances))
	}
	frame := cfg.Instances[2]
	if frame == nil {
		t.Fatalf("Instances[2]: got nil, want non-nil")
	}

	// Verify delay and base.
	if frame.Delay != 4 {
		t.Errorf("Delay: got %d, want 4", frame.Delay)
	}
	if frame.Base != 0 {
		t.Errorf("Base: got %d, want 0 (first base in local slice)", frame.Base)
	}

	// Hand-derived expected arrays (see derivation in buildAnimFrameTransformBlob).
	wantLength := 4
	wantGroups := []int32{0, 1, 2, 3}
	wantX := []int32{0, 5, 10, 0}
	wantY := []int32{0, 200, 128, 7}
	wantZ := []int32{0, -3, 128, 0}

	if frame.Length != wantLength {
		t.Errorf("Length: got %d, want %d", frame.Length, wantLength)
	}
	if len(frame.Groups) != wantLength {
		t.Fatalf("len(Groups): got %d, want %d", len(frame.Groups), wantLength)
	}
	for i := range wantLength {
		if frame.Groups[i] != wantGroups[i] {
			t.Errorf("Groups[%d]: got %d, want %d", i, frame.Groups[i], wantGroups[i])
		}
		if frame.X[i] != wantX[i] {
			t.Errorf("X[%d]: got %d, want %d", i, frame.X[i], wantX[i])
		}
		if frame.Y[i] != wantY[i] {
			t.Errorf("Y[%d]: got %d, want %d", i, frame.Y[i], wantY[i])
		}
		if frame.Z[i] != wantZ[i] {
			t.Errorf("Z[%d]: got %d, want %d", i, frame.Z[i], wantZ[i])
		}
	}
}

// TestLoadAnimFrames_EmptyCache verifies that an empty cache dir (no
// main_file_cache.dat / .idx2) returns an empty registry with no error.
// TS AnimFrame.load() — Engine-TS 9aadcec4 AnimFrame.ts:17-24.
func TestLoadAnimFrames_EmptyCache(t *testing.T) {
	dir := t.TempDir()
	cfg, err := LoadAnimFrames(dir)
	if err != nil {
		t.Fatalf("LoadAnimFrames: want nil error on empty dir, got %v", err)
	}
	if cfg == nil {
		t.Fatal("cfg: want non-nil registry, got nil")
	}
	if len(cfg.Instances) != 0 {
		t.Errorf("Instances: want empty, got %d entries", len(cfg.Instances))
	}
}
