package pack

import (
	"bytes"
	"strings"
	"testing"
)

func TestParseMesAnimConfig_LenKey(t *testing.T) {
	seqPF := newTestPF("seq", map[int]string{0: "idle", 1: "walk", 2: "death"})
	parse := parseMesAnimConfigFor(seqPF)

	v, ok, err := parse("len0", "walk")
	if err != nil || !ok {
		t.Fatalf("len0: ok=%v err=%v", ok, err)
	}
	if v.(int) != 1 {
		t.Fatalf("len0: got %d, want 1", v.(int))
	}
}

func TestParseMesAnimConfig_UnknownSeqName(t *testing.T) {
	seqPF := newTestPF("seq", map[int]string{0: "idle"})
	parse := parseMesAnimConfigFor(seqPF)

	_, ok, err := parse("len3", "doesnotexist")
	if !ok {
		t.Fatalf("len3: ok=false, want true (recognized key)")
	}
	if err == nil || !strings.Contains(err.Error(), "doesnotexist") {
		t.Fatalf("len3 unknown seq: err=%v, want containing 'doesnotexist'", err)
	}
}

func TestParseMesAnimConfig_UnknownKey(t *testing.T) {
	seqPF := newTestPF("seq", map[int]string{0: "idle"})
	parse := parseMesAnimConfigFor(seqPF)

	_, ok, err := parse("foo", "bar")
	if ok || err != nil {
		t.Fatalf("unknown key: ok=%v err=%v, want (false, nil)", ok, err)
	}
}

func TestPackMesAnimConfigs_ByteExact_SingleLen(t *testing.T) {
	pf := newTestPF("mesanim", map[int]string{0: "test_anim"})
	cfgs := map[string][]ConfigLine{
		"test_anim": {
			// len0 → opcode max(0, 0-1)+1 = 1
			{Key: "len0", Value: 7},
		},
	}
	pd := packMesAnimConfigs(cfgs, pf)
	// dat header (p2 size=1) + opcode 1 + p2(7) + 250 + pjstr("test_anim\n") + 0x00 terminator
	want := []byte{
		0x00, 0x01, // size=1
		0x01,       // opcode 1
		0x00, 0x07, // seq idx 7
		0xfa,                                              // 250
		't', 'e', 's', 't', '_', 'a', 'n', 'i', 'm', 0x0a, // pjstr LF
		0x00, // Next() terminator
	}
	if !bytes.Equal(pd.Dat.Data, want) {
		t.Fatalf("dat:\n got % x\nwant % x", pd.Dat.Data, want)
	}
}

func TestPackMesAnimConfigs_OpcodeFormula(t *testing.T) {
	pf := newTestPF("mesanim", map[int]string{0: "x"})
	for _, tc := range []struct {
		key    string
		wantOp byte
	}{
		{"len0", 1}, // max(0, -1)+1 = 1
		{"len1", 1}, // max(0,  0)+1 = 1
		{"len2", 2}, // max(0,  1)+1 = 2
		{"len5", 5},
	} {
		cfgs := map[string][]ConfigLine{"x": {{Key: tc.key, Value: 0}}}
		pd := packMesAnimConfigs(cfgs, pf)
		// dat is: [00 01 OP 00 00 fa 'x' 0a 00]
		got := pd.Dat.Data[2]
		if got != tc.wantOp {
			t.Errorf("%s: opcode = %d, want %d", tc.key, got, tc.wantOp)
		}
	}
}

func TestPackMesAnimConfigs_NonNumericLenSkipped(t *testing.T) {
	pf := newTestPF("mesanim", map[int]string{0: "x"})
	cfgs := map[string][]ConfigLine{
		"x": {{Key: "lenABC", Value: 0}},
	}
	pd := packMesAnimConfigs(cfgs, pf)
	// dat = [00 01 fa 'x' 0a 00] — no len opcode emitted
	want := []byte{0x00, 0x01, 0xfa, 'x', 0x0a, 0x00}
	if !bytes.Equal(pd.Dat.Data, want) {
		t.Fatalf("non-numeric len: got % x, want % x", pd.Dat.Data, want)
	}
}

func TestPackMesAnimConfigs_NoConfigEmitsOnlyTrailer(t *testing.T) {
	pf := newTestPF("mesanim", map[int]string{0: "named", 1: ""}) // slot 1 unnamed
	cfgs := map[string][]ConfigLine{}                             // no config for either
	pd := packMesAnimConfigs(cfgs, pf)
	want := []byte{
		0x00, 0x02, // size=2
		0xfa, 'n', 'a', 'm', 'e', 'd', 0x0a, 0x00, // slot 0: trailer + Next
		0x00, // slot 1: empty name → no trailer; just Next
	}
	if !bytes.Equal(pd.Dat.Data, want) {
		t.Fatalf("got % x, want % x", pd.Dat.Data, want)
	}
}
