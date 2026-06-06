package config

import (
	"fmt"
	"testing"

	"github.com/zsrv/goscape/pkg/io/packet"
	"github.com/zsrv/goscape/pkg/pack"
)

// buildFloConfigIdx builds a single-entry ConfigIdx from raw opcode bytes.
// idx encodes: g2(count=1), g2(len=len(body)).
// dat has 2 leading bytes (the count slot) followed by body, so pos[0]=2.
func buildFloConfigIdx(body []byte) *ConfigIdx {
	idx := packet.NewPacket([]byte{
		0, 1, // count = 1
		byte(len(body) >> 8), byte(len(body)), // len[0]
	})
	dat := make([]byte, 2+len(body))
	copy(dat[2:], body)
	cfg, err := ReadConfigIdx(idx, packet.NewPacket(dat))
	if err != nil {
		panic(err)
	}
	return cfg
}

// makePackFile builds an in-memory PackFile with a single (id→name) entry.
func makePackFile(id int, name string) *pack.PackFile {
	return &pack.PackFile{
		Pack:     map[int]string{id: name},
		NameToID: map[string]int{name: id},
		Names:    map[string]struct{}{name: {}},
		Max:      id + 1,
	}
}

// captureWarnings returns a warnf func that appends formatted messages to a slice.
func captureWarnings(out *[]string) func(string, ...any) {
	return func(f string, a ...any) {
		*out = append(*out, fmt.Sprintf(f, a...))
	}
}

func TestUnpackFlo_Opcode1_Colour(t *testing.T) {
	// colour=0xABCDEF → "colour=0xABCDEF" (uppercase hex, 6 digits padded)
	body := []byte{1, 0xAB, 0xCD, 0xEF, 0}
	cfg := buildFloConfigIdx(body)
	got := unpackFlo(cfg, 0, makePackFile(0, "myfloor"), nil, captureWarnings(new([]string)))
	want := []string{"[myfloor]", "colour=0xABCDEF"}
	assertLines(t, want, got)
}

func TestUnpackFlo_Opcode1_ColourZeroPadded(t *testing.T) {
	// colour=0x000001 → "colour=0x000001" (padded to 6 hex digits uppercase)
	body := []byte{1, 0x00, 0x00, 0x01, 0}
	cfg := buildFloConfigIdx(body)
	got := unpackFlo(cfg, 0, nil, nil, captureWarnings(new([]string)))
	if got[1] != "colour=0x000001" {
		t.Errorf("want colour=0x000001 got %q", got[1])
	}
}

func TestUnpackFlo_Opcode2_TextureHit(t *testing.T) {
	// texture id 3 → TexturePack has "marble"
	body := []byte{2, 3, 0}
	cfg := buildFloConfigIdx(body)
	got := unpackFlo(cfg, 0, nil, makePackFile(3, "marble"), captureWarnings(new([]string)))
	if got[1] != "texture=marble" {
		t.Errorf("want texture=marble got %q", got[1])
	}
}

func TestUnpackFlo_Opcode2_TextureFallback(t *testing.T) {
	// texture id 7, no pack entry → "texture_7"
	body := []byte{2, 7, 0}
	cfg := buildFloConfigIdx(body)
	got := unpackFlo(cfg, 0, nil, nil, captureWarnings(new([]string)))
	if got[1] != "texture=texture_7" {
		t.Errorf("want texture=texture_7 got %q", got[1])
	}
}

func TestUnpackFlo_Opcode3_Overlay(t *testing.T) {
	body := []byte{3, 0}
	cfg := buildFloConfigIdx(body)
	got := unpackFlo(cfg, 0, nil, nil, captureWarnings(new([]string)))
	if got[1] != "overlay=yes" {
		t.Errorf("want overlay=yes got %q", got[1])
	}
}

func TestUnpackFlo_Opcode5_Occlude(t *testing.T) {
	body := []byte{5, 0}
	cfg := buildFloConfigIdx(body)
	got := unpackFlo(cfg, 0, nil, nil, captureWarnings(new([]string)))
	if got[1] != "occlude=no" {
		t.Errorf("want occlude=no got %q", got[1])
	}
}

func TestUnpackFlo_Opcode6_DebugNameNotEmitted(t *testing.T) {
	// opcode 6 reads a LF-terminated string (GJStrLF) but must NOT emit any line.
	// TS gjstr(10) reads until LF (byte 10). Write "hello\n" (0x0a = LF).
	body := append([]byte{6}, []byte("hello\x0a")...)
	body = append(body, 0) // terminator
	cfg := buildFloConfigIdx(body)
	got := unpackFlo(cfg, 0, nil, nil, captureWarnings(new([]string)))
	if len(got) != 1 {
		t.Errorf("want 1 line (header only), got %d: %v", len(got), got)
	}
}

func TestUnpackFlo_UnknownOpcodeWarning(t *testing.T) {
	body := []byte{77, 0}
	cfg := buildFloConfigIdx(body)
	var warns []string
	unpackFlo(cfg, 0, nil, nil, captureWarnings(&warns))
	if len(warns) != 1 || warns[0] != "unknown flo code 77" {
		t.Errorf("want [\"unknown flo code 77\"], got %v", warns)
	}
}

func TestUnpackFlo_HeaderNilPack(t *testing.T) {
	// nil FloPack → header should be "[]"
	body := []byte{0}
	cfg := buildFloConfigIdx(body)
	got := unpackFlo(cfg, 0, nil, nil, captureWarnings(new([]string)))
	if got[0] != "[]" {
		t.Errorf("want [] header got %q", got[0])
	}
}

func TestUnpackFlo_PositionMismatchWarning(t *testing.T) {
	// body has 5 bytes (1+3+1=terminator) but idx says len[0]=4 → mismatch.
	// After reading the full opcode sequence, pos ends at 2+5=7, but pos[0]+len[0]=2+4=6.
	body := []byte{1, 0x00, 0x11, 0x22, 0} // 5 bytes
	dat := make([]byte, 2+len(body))
	copy(dat[2:], body)
	cfg := &ConfigIdx{
		Size: 1,
		Pos:  []int{2},
		Len:  []int{4}, // lie: actual body is 5 bytes
		Dat:  packet.NewPacket(dat),
	}

	var warns []string
	unpackFlo(cfg, 0, nil, nil, captureWarnings(&warns))

	if len(warns) == 0 {
		t.Fatal("expected position-mismatch warning, got none")
	}
	want := "incomplete read: 7 != 6"
	if warns[0] != want {
		t.Errorf("want %q got %q", want, warns[0])
	}
}

// assertLines checks that got matches want exactly, reporting all mismatches.
func assertLines(t *testing.T, want, got []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("len: want %d got %d\n  want: %v\n  got:  %v", len(want), len(got), want, got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("line %d: want %q got %q", i, want[i], got[i])
		}
	}
}
