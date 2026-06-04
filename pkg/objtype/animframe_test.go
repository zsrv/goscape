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
