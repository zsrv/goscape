package config

import (
	"testing"

	"github.com/zsrv/goscape/pkg/io/packet"
)

// buildSeqConfigIdx builds a single-entry ConfigIdx from raw opcode bytes.
func buildSeqConfigIdx(body []byte) *ConfigIdx {
	idx := packet.NewPacket([]byte{
		0, 1,
		byte(len(body) >> 8), byte(len(body)),
	})
	dat := make([]byte, 2+len(body))
	copy(dat[2:], body)
	cfg, err := ReadConfigIdx(idx, packet.NewPacket(dat))
	if err != nil {
		panic(err)
	}
	return cfg
}

// u2be encodes v as big-endian u16.
func u2be(v int) []byte { return []byte{byte(v >> 8), byte(v)} }

// u4be encodes v as big-endian u32.
func u4be(v uint32) []byte { return []byte{byte(v >> 24), byte(v >> 16), byte(v >> 8), byte(v)} }

func TestUnpackSeq_Opcode1_FrameTable_RegistryHit(t *testing.T) {
	// count=1, frame=2 (AnimPack has "run"), iframe=0xFFFF→-1 (not emitted), delay=0 (not emitted)
	body := []byte{1, 1}            // opcode=1, count=1
	body = append(body, u2be(2)...) // frame[0]=2
	body = append(body, 0xFF, 0xFF) // iframe[0]=65535→-1
	body = append(body, u2be(0)...) // delay[0]=0
	body = append(body, 0)          // terminator

	cfg := buildSeqConfigIdx(body)
	seqPack := makePackFile(0, "myseq")
	animPack := makePackFile(2, "run")
	got := unpackSeq(cfg, 0, seqPack, animPack, nil, captureWarnings(new([]string)))
	// expect: header, frame1=run (no delay since 0, no iframe since -1)
	want := []string{"[myseq]", "frame1=run"}
	assertLines(t, want, got)
}

func TestUnpackSeq_Opcode1_FrameTable_Fallback(t *testing.T) {
	// frame=5 with no AnimPack entry → "anim_5"
	body := []byte{1, 1}
	body = append(body, u2be(5)...) // frame[0]=5
	body = append(body, u2be(0)...) // iframe[0]=0 (not -1, so emitted)
	body = append(body, u2be(0)...) // delay[0]=0
	body = append(body, 0)

	cfg := buildSeqConfigIdx(body)
	got := unpackSeq(cfg, 0, nil, nil, nil, captureWarnings(new([]string)))
	// frame1=anim_5, no delay (0), iframe1=anim_0 (iframe[0]=0 != -1)
	want := []string{"[]", "frame1=anim_5", "iframe1=anim_0"}
	assertLines(t, want, got)
}

func TestUnpackSeq_Opcode1_FrameTable_Delay(t *testing.T) {
	// delay non-zero → emit delayN=
	body := []byte{1, 1}
	body = append(body, u2be(3)...)  // frame[0]=3
	body = append(body, 0xFF, 0xFF)  // iframe[0]=65535→-1
	body = append(body, u2be(20)...) // delay[0]=20
	body = append(body, 0)

	cfg := buildSeqConfigIdx(body)
	got := unpackSeq(cfg, 0, nil, nil, nil, captureWarnings(new([]string)))
	want := []string{"[]", "frame1=anim_3", "delay1=20"}
	assertLines(t, want, got)
}

func TestUnpackSeq_Opcode1_MultiFrames_OrderCheck(t *testing.T) {
	// count=2: frame[0]=1,iframe[0]=-1,delay[0]=5; frame[1]=2,iframe[1]=3,delay[1]=0
	// Expected output order: frame1, delay1, frame2 (no delay2), iframe2
	body := []byte{1, 2}
	body = append(body, u2be(1)...) // frame[0]=1
	body = append(body, 0xFF, 0xFF) // iframe[0]=-1
	body = append(body, u2be(5)...) // delay[0]=5
	body = append(body, u2be(2)...) // frame[1]=2
	body = append(body, u2be(3)...) // iframe[1]=3
	body = append(body, u2be(0)...) // delay[1]=0
	body = append(body, 0)

	cfg := buildSeqConfigIdx(body)
	animPack := makePackFile(1, "walk")
	got := unpackSeq(cfg, 0, nil, animPack, nil, captureWarnings(new([]string)))
	// frame pass: frame1=walk,delay1=5, frame2=anim_2 (no delay)
	// iframe pass: iframe[0]=-1 skip, iframe[1]=3→anim_3
	want := []string{"[]", "frame1=walk", "delay1=5", "frame2=anim_2", "iframe2=anim_3"}
	assertLines(t, want, got)
}

func TestUnpackSeq_Opcode2_Loops(t *testing.T) {
	body := append([]byte{2}, u2be(10)...)
	body = append(body, 0)
	cfg := buildSeqConfigIdx(body)
	got := unpackSeq(cfg, 0, nil, nil, nil, captureWarnings(new([]string)))
	want := []string{"[]", "loops=10"}
	assertLines(t, want, got)
}

func TestUnpackSeq_Opcode3_Walkmerge(t *testing.T) {
	// count=3, labels 1,2,3 → "walkmerge=label_1,label_2,label_3"
	body := []byte{3, 3, 1, 2, 3, 0}
	cfg := buildSeqConfigIdx(body)
	got := unpackSeq(cfg, 0, nil, nil, nil, captureWarnings(new([]string)))
	want := []string{"[]", "walkmerge=label_1,label_2,label_3"}
	assertLines(t, want, got)
}

func TestUnpackSeq_Opcode4_Stretches(t *testing.T) {
	body := []byte{4, 0}
	cfg := buildSeqConfigIdx(body)
	got := unpackSeq(cfg, 0, nil, nil, nil, captureWarnings(new([]string)))
	want := []string{"[]", "reachforward=yes"}
	assertLines(t, want, got)
}

func TestUnpackSeq_Opcode5_Priority(t *testing.T) {
	body := []byte{5, 7, 0}
	cfg := buildSeqConfigIdx(body)
	got := unpackSeq(cfg, 0, nil, nil, nil, captureWarnings(new([]string)))
	want := []string{"[]", "priority=7"}
	assertLines(t, want, got)
}

func TestUnpackSeq_Opcode6_ReplaceHeldLeft_Hide(t *testing.T) {
	// value=0 → "replaceheldleft=hide"
	body := append([]byte{6}, u2be(0)...)
	body = append(body, 0)
	cfg := buildSeqConfigIdx(body)
	got := unpackSeq(cfg, 0, nil, nil, nil, captureWarnings(new([]string)))
	want := []string{"[]", "replaceheldleft=hide"}
	assertLines(t, want, got)
}

func TestUnpackSeq_Opcode6_ReplaceHeldLeft_ObjLookup(t *testing.T) {
	// value=514 → ObjPack.getById(514-512) = ObjPack.getById(2)
	// TS: def.push(`replaceheldleft=${ObjPack.getById(replaceheldleft - 512)}`);
	body := append([]byte{6}, u2be(514)...)
	body = append(body, 0)
	cfg := buildSeqConfigIdx(body)
	objPack := makePackFile(2, "sword")
	got := unpackSeq(cfg, 0, nil, nil, objPack, captureWarnings(new([]string)))
	want := []string{"[]", "replaceheldleft=sword"}
	assertLines(t, want, got)
}

func TestUnpackSeq_Opcode7_ReplaceHeldRight_Hide(t *testing.T) {
	body := append([]byte{7}, u2be(0)...)
	body = append(body, 0)
	cfg := buildSeqConfigIdx(body)
	got := unpackSeq(cfg, 0, nil, nil, nil, captureWarnings(new([]string)))
	want := []string{"[]", "replaceheldright=hide"}
	assertLines(t, want, got)
}

func TestUnpackSeq_Opcode7_ReplaceHeldRight_ObjLookup(t *testing.T) {
	body := append([]byte{7}, u2be(513)...) // 513-512=1
	body = append(body, 0)
	cfg := buildSeqConfigIdx(body)
	objPack := makePackFile(1, "shield")
	got := unpackSeq(cfg, 0, nil, nil, objPack, captureWarnings(new([]string)))
	want := []string{"[]", "replaceheldright=shield"}
	assertLines(t, want, got)
}

func TestUnpackSeq_Opcode8_MaxLoops(t *testing.T) {
	body := []byte{8, 5, 0}
	cfg := buildSeqConfigIdx(body)
	got := unpackSeq(cfg, 0, nil, nil, nil, captureWarnings(new([]string)))
	want := []string{"[]", "maxloops=5"}
	assertLines(t, want, got)
}

func TestUnpackSeq_Opcode9_PreanimMove(t *testing.T) {
	tests := []struct {
		val  byte
		want string
	}{
		{0, "delaymove"},
		{1, "delayanim"},
		{2, "merge"},
		{3, "3"},
	}
	for _, tc := range tests {
		body := []byte{9, tc.val, 0}
		cfg := buildSeqConfigIdx(body)
		got := unpackSeq(cfg, 0, nil, nil, nil, captureWarnings(new([]string)))
		want := []string{"[]", "preanim_move=" + tc.want}
		assertLines(t, want, got)
	}
}

func TestUnpackSeq_Opcode10_PostanimMove(t *testing.T) {
	tests := []struct {
		val  byte
		want string
	}{
		{0, "delaymove"},
		{1, "abortanim"},
		{2, "merge"},
		{4, "4"},
	}
	for _, tc := range tests {
		body := []byte{10, tc.val, 0}
		cfg := buildSeqConfigIdx(body)
		got := unpackSeq(cfg, 0, nil, nil, nil, captureWarnings(new([]string)))
		want := []string{"[]", "postanim_move=" + tc.want}
		assertLines(t, want, got)
	}
}

func TestUnpackSeq_Opcode11_DuplicateBehavior(t *testing.T) {
	tests := []struct {
		val  byte
		want string
	}{
		{0, "0"},
		{1, "reset"},
		{2, "reset_loop"},
		{5, "5"},
	}
	for _, tc := range tests {
		body := []byte{11, tc.val, 0}
		cfg := buildSeqConfigIdx(body)
		got := unpackSeq(cfg, 0, nil, nil, nil, captureWarnings(new([]string)))
		want := []string{"[]", "duplicatebehaviour=" + tc.want}
		assertLines(t, want, got)
	}
}

func TestUnpackSeq_Opcode12_Code12_Positive(t *testing.T) {
	// g4s (signed 32-bit): value 0x000003E8 = 1000
	body := append([]byte{12}, u4be(1000)...)
	body = append(body, 0)
	cfg := buildSeqConfigIdx(body)
	got := unpackSeq(cfg, 0, nil, nil, nil, captureWarnings(new([]string)))
	want := []string{"[]", "code12=1000"}
	assertLines(t, want, got)
}

func TestUnpackSeq_Opcode12_Code12_Negative(t *testing.T) {
	// g4s signed: 0xFFFFFFFF = -1
	body := append([]byte{12}, u4be(0xFFFFFFFF)...)
	body = append(body, 0)
	cfg := buildSeqConfigIdx(body)
	got := unpackSeq(cfg, 0, nil, nil, nil, captureWarnings(new([]string)))
	want := []string{"[]", "code12=-1"}
	assertLines(t, want, got)
}

func TestUnpackSeq_UnknownOpcodeWarning(t *testing.T) {
	body := []byte{200, 0}
	cfg := buildSeqConfigIdx(body)
	var warns []string
	unpackSeq(cfg, 0, nil, nil, nil, captureWarnings(&warns))
	if len(warns) != 1 || warns[0] != "unknown seq code 200" {
		t.Errorf("want [\"unknown seq code 200\"], got %v", warns)
	}
}

func TestUnpackSeq_PositionMismatchWarning(t *testing.T) {
	// opcode 4 body=2 bytes, lie about len=1 → pos ends at 2+2=4, but pos[0]+len[0]=2+1=3
	body := []byte{4, 0}
	dat := make([]byte, 2+len(body))
	copy(dat[2:], body)
	cfg := &ConfigIdx{
		Size: 1,
		Pos:  []int{2},
		Len:  []int{1}, // lie: body is 2 bytes
		Dat:  packet.NewPacket(dat),
	}
	var warns []string
	unpackSeq(cfg, 0, nil, nil, nil, captureWarnings(&warns))
	if len(warns) == 0 {
		t.Fatal("expected position-mismatch warning, got none")
	}
	want := "incomplete read: 4 != 3"
	if warns[0] != want {
		t.Errorf("want %q got %q", want, warns[0])
	}
}

func TestUnpackSeq_HeaderNilPack(t *testing.T) {
	body := []byte{0}
	cfg := buildSeqConfigIdx(body)
	got := unpackSeq(cfg, 0, nil, nil, nil, captureWarnings(new([]string)))
	if got[0] != "[]" {
		t.Errorf("want [] header got %q", got[0])
	}
}
