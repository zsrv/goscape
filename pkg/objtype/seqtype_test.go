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
	if st.Frames != nil || st.IFrames != nil || st.Delay != nil || st.WalkMerge != nil {
		t.Error("slice fields should be nil by default")
	}
	if st.Stretches {
		t.Error("Stretches: got true, want false")
	}
	if st.Duration != 0 {
		t.Errorf("Duration: got %d, want 0", st.Duration)
	}
}

// decodeSeq builds a writer packet, appends a 0-terminator, flips to reader,
// and runs DecodeType on a fresh NewSeqType(0) with the optional SeqFrame
// back-reference. Mirrors idktype_test's decodeIdk pattern.
func decodeSeq(frames *SeqFrameConfigs, build func(*packet.Packet)) (*SeqType, error) {
	w := packet.NewPacket(nil)
	build(w)
	w.P1(0) // terminator
	r := packet.NewPacket(w.Bytes())
	st := NewSeqType(0)
	st.frames = frames
	err := DecodeType(r, st)
	return st, err
}

func TestSeqTypeDecode_Frames(t *testing.T) {
	st, err := decodeSeq(nil, func(p *packet.Packet) {
		p.P1(1)
		p.P1(2)       // count = 2
		p.P2(0x0010)  // frames[0] = 16
		p.P2(0x0020)  // iframes[0] = 32
		p.P2(0x0003)  // delay[0] = 3
		p.P2(0x0011)  // frames[1] = 17
		p.P2(0x0021)  // iframes[1] = 33
		p.P2(0x0004)  // delay[1] = 4
	})
	if err != nil {
		t.Fatalf("DecodeType: %v", err)
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
		p.P1(1)       // count = 1
		p.P2(0x0001)  // frames[0] = 1
		p.P2(0xFFFF)  // iframes[0] = 65535 → normalised to -1
		p.P2(0x0005)  // delay[0] = 5
	})
	if err != nil {
		t.Fatalf("DecodeType: %v", err)
	}
	if st.IFrames[0] != -1 {
		t.Errorf("IFrames[0]: got %d, want -1 (65535 normalisation)", st.IFrames[0])
	}
}

func TestSeqTypeDecode_DelayZeroFallbackToFrameDelay(t *testing.T) {
	frames := &SeqFrameConfigs{
		Instances: []*SeqFrame{
			{Delay: 0}, // frame 0
			{Delay: 7}, // frame 1
		},
	}
	st, err := decodeSeq(frames, func(p *packet.Packet) {
		p.P1(1)
		p.P1(1)       // count = 1
		p.P2(0x0001)  // frames[0] = 1
		p.P2(0x0000)  // iframes[0] = 0
		p.P2(0x0000)  // delay[0] = 0 → fallback to frames.Instances[1].Delay = 7
	})
	if err != nil {
		t.Fatalf("DecodeType: %v", err)
	}
	if st.Delay[0] != 7 {
		t.Errorf("Delay[0]: got %d, want 7 (SeqFrame.delay fallback)", st.Delay[0])
	}
	if st.Duration != 7 {
		t.Errorf("Duration: got %d, want 7", st.Duration)
	}
}

func TestSeqTypeDecode_DelayZeroNoFallbackUsesOne(t *testing.T) {
	// nil frames back-ref → fallback to TS L101 default of 1
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
		t.Errorf("Delay[0]: got %d, want 1 (TS L101 default)", st.Delay[0])
	}
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
	if !st.Stretches {
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
	configs, err := LoadSeqTypes(dir, &SeqFrameConfigs{})
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
	cacheDir := filepath.Join("..", "..", "data", "pack")
	if _, err := os.Stat(filepath.Join(cacheDir, "server", "seq.dat")); err != nil {
		t.Skipf("no pack data: %v", err)
	}
	frames, err := LoadSeqFrames(cacheDir)
	if err != nil {
		t.Fatalf("LoadSeqFrames: %v", err)
	}
	configs, err := LoadSeqTypes(cacheDir, frames)
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
			{ConfigType: ConfigType{ID: 0, DebugName: "first"}},
			{ConfigType: ConfigType{ID: 1, DebugName: "second"}},
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
		Configs:     []*SeqType{{ConfigType: ConfigType{ID: 0, DebugName: "only"}}},
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
			{ConfigType: ConfigType{ID: 0, DebugName: "other"}},
			{ConfigType: ConfigType{ID: 1, DebugName: "fresh"}},
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
		Configs:     []*SeqType{{ConfigType: ConfigType{ID: 0, DebugName: "scan_me"}}},
		ConfigNames: nil,
	}
	got := c.ByName("scan_me")
	if got == nil || got.ID != 0 {
		t.Errorf("ByName(scan_me) with nil ConfigNames = %+v, want non-nil id=0", got)
	}
}
