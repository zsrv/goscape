package objtype

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/zsrv/goscape/pkg/io/packet"
)

func TestNewSeqTypeDefaults(t *testing.T) {
	st := NewSeqType(7)
	if st.ID != 7 {
		t.Errorf("ID: got %d, want 7", st.ID)
	}
	if st.Loops != -1 {
		t.Errorf("Loops: got %d, want -1", st.Loops)
	}
	if st.Priority != 5 {
		t.Errorf("Priority: got %d, want 5", st.Priority)
	}
	if st.ReplaceHeldLeft != -1 {
		t.Errorf("ReplaceHeldLeft: got %d, want -1", st.ReplaceHeldLeft)
	}
	if st.ReplaceHeldRight != -1 {
		t.Errorf("ReplaceHeldRight: got %d, want -1", st.ReplaceHeldRight)
	}
	if st.MaxLoops != 99 {
		t.Errorf("MaxLoops: got %d, want 99", st.MaxLoops)
	}
	// 244: new default fields
	if st.PreanimMove != -1 {
		t.Errorf("PreanimMove: got %d, want -1", st.PreanimMove)
	}
	if st.PostanimMove != -1 {
		t.Errorf("PostanimMove: got %d, want -1", st.PostanimMove)
	}
	if st.DuplicateBehaviour != 0 {
		t.Errorf("DuplicateBehavior: got %d, want 0", st.DuplicateBehaviour)
	}
	if st.FrameCount != 0 {
		t.Errorf("FrameCount: got %d, want 0", st.FrameCount)
	}
	if st.Frames != nil || st.IFrames != nil || st.Delay != nil || st.WalkMerge != nil {
		t.Error("slice fields should be nil by default")
	}
	if st.Reachforward {
		t.Error("Stretches: got true, want false")
	}
	if st.Duration != 0 {
		t.Errorf("Duration: got %d, want 0", st.Duration)
	}
}

// decodeSeq builds a writer packet, appends a 0-terminator, flips to reader,
// and runs DecodeType on a fresh NewSeqType(0) with the optional AnimFrame
// back-reference. Mirrors idktype_test's decodeIdk pattern.
func decodeSeq(animFrames *AnimFrameConfigs, build func(*packet.Packet)) (*SeqType, error) {
	w := packet.NewPacket(nil)
	build(w)
	w.P1(0) // terminator
	r := packet.NewPacket(w.Bytes())
	st := NewSeqType(0)
	st.animFrames = animFrames
	err := DecodeType(r, st)
	return st, err
}

func TestSeqTypeDecode_Frames(t *testing.T) {
	st, err := decodeSeq(nil, func(p *packet.Packet) {
		p.P1(1)
		p.P1(2)      // count = 2
		p.P2(0x0010) // frames[0] = 16
		p.P2(0x0020) // iframes[0] = 32
		p.P2(0x0003) // delay[0] = 3
		p.P2(0x0011) // frames[1] = 17
		p.P2(0x0021) // iframes[1] = 33
		p.P2(0x0004) // delay[1] = 4
	})
	if err != nil {
		t.Fatalf("DecodeType: %v", err)
	}
	// 244: FrameCount is now stored on the type.
	// TS SeqType.ts:91 (Engine-TS 9aadcec4): this.frameCount = dat.g1()
	if st.FrameCount != 2 {
		t.Errorf("FrameCount: got %d, want 2", st.FrameCount)
	}
	if got := st.Frames; len(got) != 2 || got[0] != 16 || got[1] != 17 {
		t.Errorf("Frames: got %v, want [16 17]", got)
	}
	if got := st.IFrames; len(got) != 2 || got[0] != 32 || got[1] != 33 {
		t.Errorf("IFrames: got %v, want [32 33]", got)
	}
	if got := st.Delay; len(got) != 2 || got[0] != 3 || got[1] != 4 {
		t.Errorf("Delay: got %v, want [3 4]", got)
	}
	if st.Duration != 7 {
		t.Errorf("Duration: got %d, want 7 (3+4)", st.Duration)
	}
}

func TestSeqTypeDecode_IFrames65535ToMinusOne(t *testing.T) {
	st, err := decodeSeq(nil, func(p *packet.Packet) {
		p.P1(1)
		p.P1(1)      // count = 1
		p.P2(0x0001) // frames[0] = 1
		p.P2(0xFFFF) // iframes[0] = 65535 → normalised to -1
		p.P2(0x0005) // delay[0] = 5
	})
	if err != nil {
		t.Fatalf("DecodeType: %v", err)
	}
	if st.IFrames[0] != -1 {
		t.Errorf("IFrames[0]: got %d, want -1 (65535 normalisation)", st.IFrames[0])
	}
}

func TestSeqTypeDecode_DelayZeroFallbackToAnimFrameDelay(t *testing.T) {
	// 244: delay fallback now uses AnimFrameConfigs (was SeqFrameConfigs).
	// TS SeqType.ts:105-107 (Engine-TS 9aadcec4):
	//   if (this.delay[i] === 0) { this.delay[i] = AnimFrame.instances[this.frames[i]].delay; }
	animFrames := &AnimFrameConfigs{
		Instances: []*AnimFrame{
			{Delay: 0}, // frame 0
			{Delay: 7}, // frame 1
		},
	}
	st, err := decodeSeq(animFrames, func(p *packet.Packet) {
		p.P1(1)
		p.P1(1)      // count = 1
		p.P2(0x0001) // frames[0] = 1
		p.P2(0x0000) // iframes[0] = 0
		p.P2(0x0000) // delay[0] = 0 → fallback to animFrames.Instances[1].Delay = 7
	})
	if err != nil {
		t.Fatalf("DecodeType: %v", err)
	}
	if st.Delay[0] != 7 {
		t.Errorf("Delay[0]: got %d, want 7 (AnimFrame.delay fallback)", st.Delay[0])
	}
	if st.Duration != 7 {
		t.Errorf("Duration: got %d, want 7", st.Duration)
	}
}

func TestSeqTypeDecode_DelayZeroNoFallbackUsesOne(t *testing.T) {
	// nil animFrames back-ref → fallback to TS L109 default of 1
	st, err := decodeSeq(nil, func(p *packet.Packet) {
		p.P1(1)
		p.P1(1)
		p.P2(0x0001)
		p.P2(0x0000)
		p.P2(0x0000) // delay = 0; no fallback registry; final fallback = 1
	})
	if err != nil {
		t.Fatalf("DecodeType: %v", err)
	}
	if st.Delay[0] != 1 {
		t.Errorf("Delay[0]: got %d, want 1 (TS L109 default)", st.Delay[0])
	}
}

// TestSeqTypeDecode_DelayZeroEmptyAnimFrames verifies that an empty (no-cache)
// AnimFrameConfigs falls through to the TS L109 default of 1 rather than
// panicking. This covers the rev-244 scenario where main_file_cache.dat is
// absent (e.g. a 225-format test cache) and LoadAnimFrames returns an empty
// registry. Production callers always have a populated registry.
// (CONFIRMED-EXCEPTION: additive robustness for test fixtures using 225 caches.)
func TestSeqTypeDecode_DelayZeroEmptyAnimFrames_UsesDefault(t *testing.T) {
	emptyFrames := &AnimFrameConfigs{} // Instances == nil/empty
	st, err := decodeSeq(emptyFrames, func(p *packet.Packet) {
		p.P1(1)
		p.P1(1)
		p.P2(0x0005) // frames[0] = 5
		p.P2(0x0000) // iframes[0] = 0
		p.P2(0x0000) // delay[0] = 0 → empty Instances → falls through to d=1
	})
	if err != nil {
		t.Fatalf("DecodeType: %v", err)
	}
	if st.Delay[0] != 1 {
		t.Errorf("Delay[0]: got %d, want 1 (TS L109 default when Instances empty)", st.Delay[0])
	}
}

// cfg-media-1: TS SeqType.decode L105 derefs AnimFrame.instances[frames[i]].delay
// unconditionally — an OOR frames[i] throws TypeError, aborting the config
// parse. goscape pre-fix wrapped the deref in bounds guard; post-fix drops the
// upper-bound guard to match TS for POPULATED registries. The nil-animFrames
// and empty-Instances guards are preserved (CONFIRMED-EXCEPTION).
func TestSeqTypeDecode_DelayZeroOutOfRangeFramesIndex_Panics(t *testing.T) {
	animFrames := &AnimFrameConfigs{
		Instances: []*AnimFrame{{Delay: 7}}, // length 1
	}
	defer func() {
		if r := recover(); r == nil {
			t.Errorf("expected panic on OOR frames[i]=999 against Instances len 1 (TS L105 unguarded must abort parse), got no panic")
		}
	}()
	_, _ = decodeSeq(animFrames, func(p *packet.Packet) {
		p.P1(1)
		p.P1(1)
		p.P2(0x03e7) // frames[0] = 999 (OOR vs Instances len 1)
		p.P2(0x0000) // iframes[0] = 0
		p.P2(0x0000) // delay[0] = 0 → triggers L105 fallback → OOR access
	})
}

func TestSeqTypeDecode_Loops(t *testing.T) {
	st, err := decodeSeq(nil, func(p *packet.Packet) { p.P1(2); p.P2(0x0007) })
	if err != nil {
		t.Fatalf("DecodeType: %v", err)
	}
	if st.Loops != 7 {
		t.Errorf("Loops: got %d, want 7", st.Loops)
	}
}

func TestSeqTypeDecode_WalkMerge(t *testing.T) {
	st, err := decodeSeq(nil, func(p *packet.Packet) {
		p.P1(3)
		p.P1(2)  // count = 2
		p.P1(11) // walkmerge[0]
		p.P1(22) // walkmerge[1]
	})
	if err != nil {
		t.Fatalf("DecodeType: %v", err)
	}
	if got := st.WalkMerge; len(got) != 3 || got[0] != 11 || got[1] != 22 || got[2] != 9999999 {
		t.Errorf("WalkMerge: got %v, want [11 22 9999999]", got)
	}
}

func TestSeqTypeDecode_Stretches(t *testing.T) {
	st, err := decodeSeq(nil, func(p *packet.Packet) { p.P1(4) })
	if err != nil {
		t.Fatalf("DecodeType: %v", err)
	}
	if !st.Reachforward {
		t.Error("Stretches: got false, want true")
	}
}

func TestSeqTypeDecode_Priority(t *testing.T) {
	st, err := decodeSeq(nil, func(p *packet.Packet) { p.P1(5); p.P1(3) })
	if err != nil {
		t.Fatalf("DecodeType: %v", err)
	}
	if st.Priority != 3 {
		t.Errorf("Priority: got %d, want 3", st.Priority)
	}
}

func TestSeqTypeDecode_ReplaceHeldLeft(t *testing.T) {
	st, err := decodeSeq(nil, func(p *packet.Packet) { p.P1(6); p.P2(0x0102) })
	if err != nil {
		t.Fatalf("DecodeType: %v", err)
	}
	if st.ReplaceHeldLeft != 0x0102 {
		t.Errorf("ReplaceHeldLeft: got %d, want %d", st.ReplaceHeldLeft, 0x0102)
	}
}

func TestSeqTypeDecode_ReplaceHeldRight(t *testing.T) {
	st, err := decodeSeq(nil, func(p *packet.Packet) { p.P1(7); p.P2(0x0304) })
	if err != nil {
		t.Fatalf("DecodeType: %v", err)
	}
	if st.ReplaceHeldRight != 0x0304 {
		t.Errorf("ReplaceHeldRight: got %d, want %d", st.ReplaceHeldRight, 0x0304)
	}
}

func TestSeqTypeDecode_MaxLoops(t *testing.T) {
	st, err := decodeSeq(nil, func(p *packet.Packet) { p.P1(8); p.P1(42) })
	if err != nil {
		t.Fatalf("DecodeType: %v", err)
	}
	if st.MaxLoops != 42 {
		t.Errorf("MaxLoops: got %d, want 42", st.MaxLoops)
	}
}

// 244 new codes — TS SeqType.ts:137-145 (Engine-TS 9aadcec4)

// TestSeqTypeDecode_PreanimMove verifies code 9 reads g1 into PreanimMove.
// TS SeqType.ts:137-139: } else if (code === 9) { this.preanim_move = dat.g1(); }
func TestSeqTypeDecode_PreanimMove(t *testing.T) {
	st, err := decodeSeq(nil, func(p *packet.Packet) { p.P1(9); p.P1(3) })
	if err != nil {
		t.Fatalf("DecodeType: %v", err)
	}
	if st.PreanimMove != 3 {
		t.Errorf("PreanimMove: got %d, want 3 (code 9, g1)", st.PreanimMove)
	}
}

// TestSeqTypeDecode_PostanimMove verifies code 10 reads g1 into PostanimMove.
// TS SeqType.ts:140-142: } else if (code === 10) { this.postanim_move = dat.g1(); }
func TestSeqTypeDecode_PostanimMove(t *testing.T) {
	st, err := decodeSeq(nil, func(p *packet.Packet) { p.P1(10); p.P1(1) })
	if err != nil {
		t.Fatalf("DecodeType: %v", err)
	}
	if st.PostanimMove != 1 {
		t.Errorf("PostanimMove: got %d, want 1 (code 10, g1)", st.PostanimMove)
	}
}

// TestSeqTypeDecode_DuplicateBehavior verifies code 11 reads g1 into DuplicateBehavior.
// TS SeqType.ts:143-145: } else if (code === 11) { this.duplicatebehaviour = dat.g1(); }
func TestSeqTypeDecode_DuplicateBehavior(t *testing.T) {
	st, err := decodeSeq(nil, func(p *packet.Packet) { p.P1(11); p.P1(2) })
	if err != nil {
		t.Fatalf("DecodeType: %v", err)
	}
	if st.DuplicateBehaviour != 2 {
		t.Errorf("DuplicateBehavior: got %d, want 2 (code 11, g1)", st.DuplicateBehaviour)
	}
}

func TestSeqTypeDecode_DebugName(t *testing.T) {
	st, err := decodeSeq(nil, func(p *packet.Packet) { p.P1(250); p.PJStrLF("test_seq") })
	if err != nil {
		t.Fatalf("DecodeType: %v", err)
	}
	if st.DebugName != "test_seq" {
		t.Errorf("DebugName: got %q, want %q", st.DebugName, "test_seq")
	}
}

func TestSeqTypeDecode_UnknownCode(t *testing.T) {
	_, err := decodeSeq(nil, func(p *packet.Packet) { p.P1(99) })
	if err == nil {
		t.Error("want error for unknown code 99, got nil")
	}
}

// TestSeqTypePostDecode_NoFrames verifies that postDecode creates stub arrays
// when FrameCount==0, matching TS SeqType.postDecode() L149-157.
// TS SeqType.ts:148-157 (Engine-TS 9aadcec4)
func TestSeqTypePostDecode_NoFrames(t *testing.T) {
	st := NewSeqType(0)
	// No code-1 decoded — FrameCount stays 0.
	st.PostDecode()

	if st.FrameCount != 1 {
		t.Errorf("FrameCount after postDecode: got %d, want 1", st.FrameCount)
	}
	if len(st.Frames) != 1 || st.Frames[0] != -1 {
		t.Errorf("Frames after postDecode: got %v, want [-1]", st.Frames)
	}
	if len(st.IFrames) != 1 || st.IFrames[0] != -1 {
		t.Errorf("IFrames after postDecode: got %v, want [-1]", st.IFrames)
	}
	if len(st.Delay) != 1 || st.Delay[0] != -1 {
		t.Errorf("Delay after postDecode: got %v, want [-1]", st.Delay)
	}
}

// TestSeqTypePostDecode_PreanimMoveNoWalkmerge verifies preanim_move defaults
// to 0 when walkmerge is nil.
// TS SeqType.ts:159-165 (Engine-TS 9aadcec4)
func TestSeqTypePostDecode_PreanimMoveNoWalkmerge(t *testing.T) {
	st := NewSeqType(0)
	st.FrameCount = 1 // prevent stub-fill branch
	st.Frames = []int32{0}
	st.IFrames = []int32{0}
	st.Delay = []int32{1}
	// WalkMerge is nil (default)
	st.PostDecode()

	if st.PreanimMove != 0 {
		t.Errorf("PreanimMove (no walkmerge): got %d, want 0", st.PreanimMove)
	}
}

// TestSeqTypePostDecode_PreanimMoveWithWalkmerge verifies preanim_move defaults
// to 2 when walkmerge is set.
// TS SeqType.ts:162-165 (Engine-TS 9aadcec4)
func TestSeqTypePostDecode_PreanimMoveWithWalkmerge(t *testing.T) {
	st := NewSeqType(0)
	st.FrameCount = 1
	st.Frames = []int32{0}
	st.IFrames = []int32{0}
	st.Delay = []int32{1}
	st.WalkMerge = []int32{0, 9999999} // non-nil
	st.PostDecode()

	if st.PreanimMove != 2 {
		t.Errorf("PreanimMove (with walkmerge): got %d, want 2", st.PreanimMove)
	}
}

// TestSeqTypePostDecode_PostanimMoveNoWalkmerge verifies postanim_move defaults
// to 0 when walkmerge is nil.
// TS SeqType.ts:167-175 (Engine-TS 9aadcec4)
func TestSeqTypePostDecode_PostanimMoveNoWalkmerge(t *testing.T) {
	st := NewSeqType(0)
	st.FrameCount = 1
	st.Frames = []int32{0}
	st.IFrames = []int32{0}
	st.Delay = []int32{1}
	st.PostDecode()

	if st.PostanimMove != 0 {
		t.Errorf("PostanimMove (no walkmerge): got %d, want 0", st.PostanimMove)
	}
}

// TestSeqTypePostDecode_PostanimMoveWithWalkmerge verifies postanim_move defaults
// to 2 when walkmerge is set.
// TS SeqType.ts:172-175 (Engine-TS 9aadcec4)
func TestSeqTypePostDecode_PostanimMoveWithWalkmerge(t *testing.T) {
	st := NewSeqType(0)
	st.FrameCount = 1
	st.Frames = []int32{0}
	st.IFrames = []int32{0}
	st.Delay = []int32{1}
	st.WalkMerge = []int32{0, 9999999}
	st.PostDecode()

	if st.PostanimMove != 2 {
		t.Errorf("PostanimMove (with walkmerge): got %d, want 2", st.PostanimMove)
	}
}

// TestSeqTypePostDecode_ExplicitFieldsNotOverridden verifies that explicit
// wire values for preanim_move and postanim_move are NOT overridden by postDecode.
// TS SeqType.ts:159/167: only overrides when the value is still -1.
func TestSeqTypePostDecode_ExplicitFieldsNotOverridden(t *testing.T) {
	st := NewSeqType(0)
	st.FrameCount = 1
	st.Frames = []int32{0}
	st.IFrames = []int32{0}
	st.Delay = []int32{1}
	st.PreanimMove = 1  // explicitly set
	st.PostanimMove = 3 // explicitly set
	st.PostDecode()

	if st.PreanimMove != 1 {
		t.Errorf("PreanimMove: explicit value should not be overridden by postDecode; got %d, want 1", st.PreanimMove)
	}
	if st.PostanimMove != 3 {
		t.Errorf("PostanimMove: explicit value should not be overridden by postDecode; got %d, want 3", st.PostanimMove)
	}
}

// TestSeqTypePostDecode_DurationNotRecalculated verifies that postDecode does
// NOT recalculate Duration — Duration is accumulated in Decode code-1.
// The TS has no duration change in postDecode (SeqType.ts:148-175 9aadcec4).
func TestSeqTypePostDecode_DurationNotRecalculated(t *testing.T) {
	st, err := decodeSeq(nil, func(p *packet.Packet) {
		p.P1(1)
		p.P1(2) // count = 2
		p.P2(0) // frames[0]
		p.P2(0) // iframes[0]
		p.P2(3) // delay[0] = 3
		p.P2(0) // frames[1]
		p.P2(0) // iframes[1]
		p.P2(5) // delay[1] = 5
	})
	if err != nil {
		t.Fatalf("DecodeType: %v", err)
	}
	durationBeforePostDecode := st.Duration
	st.PostDecode()
	if st.Duration != durationBeforePostDecode {
		t.Errorf("Duration changed by postDecode: was %d, got %d", durationBeforePostDecode, st.Duration)
	}
	if st.Duration != 8 {
		t.Errorf("Duration: got %d, want 8 (3+5)", st.Duration)
	}
}

func TestSeqTypeConfigsCount_NilReceiver(t *testing.T) {
	var c *SeqTypeConfigs
	if got := c.Count(); got != 0 {
		t.Errorf("Count() on nil: got %d, want 0", got)
	}
}

func TestSeqTypeConfigsCount_Populated(t *testing.T) {
	c := &SeqTypeConfigs{Configs: make([]*SeqType, 5)}
	if got := c.Count(); got != 5 {
		t.Errorf("Count(): got %d, want 5", got)
	}
}

func TestLoadSeqTypes_MissingFile(t *testing.T) {
	dir := t.TempDir()
	// 244: LoadSeqTypes now takes *AnimFrameConfigs (was *SeqFrameConfigs).
	configs, err := LoadSeqTypes(dir, &AnimFrameConfigs{})
	if err != nil {
		t.Fatalf("LoadSeqTypes: want nil error on missing file, got %v", err)
	}
	if configs == nil {
		t.Fatal("configs: want non-nil registry, got nil")
	}
	if len(configs.Configs) != 0 {
		t.Errorf("Configs: want empty, got %d entries", len(configs.Configs))
	}
}

func TestLoadSeqTypes_FromPack(t *testing.T) {
	// Prefers the repo's data/pack (regenerate with goscape-cli pack); falls
	// back to the Server274-ref reference cache (the rev-274 branch pin —
	// never resolve revision-specific caches across branch boundaries), which
	// has the server/seq.dat and main_file_cache.dat (FileStream) required
	// for LoadAnimFrames. Skip when neither is available.
	var refPack string
	if ref := os.Getenv("GOSCAPE_REF274_DIR"); ref != "" {
		refPack = filepath.Join(ref, "data", "pack")
	}
	cacheDir := filepath.Join("..", "..", "data", "pack")
	if _, err := os.Stat(filepath.Join(cacheDir, "server", "seq.dat")); err != nil {
		if refPack == "" {
			t.Skipf("no pack data: %v; GOSCAPE_REF274_DIR not set for reference cache", err)
		}
		if _, err2 := os.Stat(filepath.Join(refPack, "server", "seq.dat")); err2 != nil {
			t.Skipf("no pack data: %v; reference cache also unavailable: %v", err, err2)
		}
		cacheDir = refPack
	}
	// 244: LoadAnimFrames replaces LoadSeqFrames.
	animFrames, err := LoadAnimFrames(cacheDir)
	if err != nil {
		t.Fatalf("LoadAnimFrames: %v", err)
	}
	configs, err := LoadSeqTypes(cacheDir, animFrames)
	if err != nil {
		t.Fatalf("LoadSeqTypes: %v", err)
	}
	if len(configs.Configs) == 0 {
		t.Fatal("expected at least one SeqType, got 0")
	}
}

func TestSeqTypeConfigs_ByName_HitViaConfigNames(t *testing.T) {
	c := &SeqTypeConfigs{
		Configs: []*SeqType{
			{ID: 0, DebugName: "first"},
			{ID: 1, DebugName: "second"},
		},
		ConfigNames: map[string]int{"first": 0, "second": 1},
	}
	got := c.ByName("second")
	if got == nil {
		t.Fatalf("ByName(second) = nil, want non-nil")
	}
	if got.ID != 1 || got.DebugName != "second" {
		t.Errorf("ByName(second) = {ID:%d, DebugName:%q}, want {ID:1, DebugName:\"second\"}", got.ID, got.DebugName)
	}
}

func TestSeqTypeConfigs_ByName_MissReturnsNil(t *testing.T) {
	c := &SeqTypeConfigs{
		Configs:     []*SeqType{{ID: 0, DebugName: "only"}},
		ConfigNames: map[string]int{"only": 0},
	}
	if got := c.ByName("absent"); got != nil {
		t.Errorf("ByName(absent) = %+v, want nil", got)
	}
}

func TestSeqTypeConfigs_ByName_NilReceiverReturnsNil(t *testing.T) {
	var c *SeqTypeConfigs
	if got := c.ByName("anything"); got != nil {
		t.Errorf("nil-receiver ByName = %+v, want nil", got)
	}
}

func TestSeqTypeConfigs_ByName_StaleIndexFallsThroughToLinearScan(t *testing.T) {
	c := &SeqTypeConfigs{
		Configs: []*SeqType{
			{ID: 0, DebugName: "other"},
			{ID: 1, DebugName: "fresh"},
		},
		ConfigNames: map[string]int{"fresh": 5},
	}
	got := c.ByName("fresh")
	if got == nil {
		t.Fatalf("stale-index ByName(fresh) = nil; want fallback hit at id=1")
	}
	if got.ID != 1 {
		t.Errorf("stale-index ByName(fresh).ID = %d, want 1", got.ID)
	}
}

func TestSeqTypeConfigs_ByName_LinearScanWhenConfigNamesEmpty(t *testing.T) {
	c := &SeqTypeConfigs{
		Configs:     []*SeqType{{ID: 0, DebugName: "scan_me"}},
		ConfigNames: nil,
	}
	got := c.ByName("scan_me")
	if got == nil || got.ID != 0 {
		t.Errorf("ByName(scan_me) with nil ConfigNames = %+v, want non-nil id=0", got)
	}
}

// TestSeqTypeReachforwardRename pins the reachforward→reachforward field rename
// from Engine-TS 8139461a (TS SeqType.ts:79,129 @1d25566c). Opcode 4 is
// unchanged; only the name moved.
func TestSeqTypeReachforwardRename(t *testing.T) {
	st := NewSeqType(0)
	if st.Reachforward {
		t.Error("Reachforward default: got true, want false")
	}

	pkt := packet.NewPacket(nil)
	pkt.P1(4)
	pkt.P1(0)

	if err := DecodeType(packet.NewPacket(pkt.Bytes()), st); err != nil {
		t.Fatalf("DecodeType: %v", err)
	}
	if !st.Reachforward {
		t.Error("Reachforward after opcode 4: got false, want true")
	}
}

// TestSeqTypeDuplicateBehaviourRename pins the duplicatebehaviour→
// duplicatebehaviour rename (TS SeqType.ts:86,143 @1d25566c). Opcode 11 is
// unchanged.
func TestSeqTypeDuplicateBehaviourRename(t *testing.T) {
	st := NewSeqType(0)
	if st.DuplicateBehaviour != 0 {
		t.Errorf("DuplicateBehaviour default: got %d, want 0", st.DuplicateBehaviour)
	}

	pkt := packet.NewPacket(nil)
	pkt.P1(11)
	pkt.P1(2)
	pkt.P1(0)

	if err := DecodeType(packet.NewPacket(pkt.Bytes()), st); err != nil {
		t.Fatalf("DecodeType: %v", err)
	}
	if st.DuplicateBehaviour != 2 {
		t.Errorf("DuplicateBehaviour: got %d, want 2", st.DuplicateBehaviour)
	}
}
