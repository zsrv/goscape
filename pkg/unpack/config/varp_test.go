package config

import (
	"testing"

	"github.com/zsrv/goscape/pkg/io/packet"
)

// buildVarpConfigIdx builds a single-entry ConfigIdx from raw opcode bytes.
func buildVarpConfigIdx(body []byte) *ConfigIdx {
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

func TestUnpackVarp_Opcode5_Clientcode(t *testing.T) {
	// opcode 5, g2 = 0x0105 → clientcode=261
	body := []byte{5, 0x01, 0x05, 0}
	cfg := buildVarpConfigIdx(body)
	varpPack := makePackFile(0, "myvarp")
	got := unpackVarp(cfg, 0, varpPack, captureWarnings(new([]string)))
	want := []string{"[myvarp]", "clientcode=261"}
	assertLines(t, want, got)
}

func TestUnpackVarp_Opcode5_Zero(t *testing.T) {
	// clientcode=0 is a valid value (unsigned g2)
	body := []byte{5, 0x00, 0x00, 0}
	cfg := buildVarpConfigIdx(body)
	got := unpackVarp(cfg, 0, nil, captureWarnings(new([]string)))
	if got[1] != "clientcode=0" {
		t.Errorf("want clientcode=0 got %q", got[1])
	}
}

func TestUnpackVarp_UnknownOpcodeWarning(t *testing.T) {
	body := []byte{42, 0}
	cfg := buildVarpConfigIdx(body)
	var warns []string
	unpackVarp(cfg, 0, nil, captureWarnings(&warns))
	if len(warns) != 1 || warns[0] != "unknown varp code 42" {
		t.Errorf("want [\"unknown varp code 42\"], got %v", warns)
	}
}

func TestUnpackVarp_HeaderNilPack(t *testing.T) {
	body := []byte{0}
	cfg := buildVarpConfigIdx(body)
	got := unpackVarp(cfg, 0, nil, captureWarnings(new([]string)))
	if got[0] != "[]" {
		t.Errorf("want [] header got %q", got[0])
	}
}

func TestUnpackVarp_PositionMismatchWarning(t *testing.T) {
	// body has 4 bytes (opcode5 + g2 + terminator = 4), idx says len[0]=3
	body := []byte{5, 0x00, 0x01, 0} // 4 bytes
	dat := make([]byte, 2+len(body))
	copy(dat[2:], body)
	cfg := &ConfigIdx{
		Size: 1,
		Pos:  []int{2},
		Len:  []int{3}, // lie
		Dat:  packet.NewPacket(dat),
	}

	var warns []string
	unpackVarp(cfg, 0, nil, captureWarnings(&warns))

	if len(warns) == 0 {
		t.Fatal("expected position-mismatch warning, got none")
	}
	want := "incomplete read: 6 != 5"
	if warns[0] != want {
		t.Errorf("want %q got %q", want, warns[0])
	}
}

func TestUnpackVarp_MultipleOpcodes(t *testing.T) {
	// Two opcode-5 entries followed by terminator
	body := []byte{5, 0x00, 0x0A, 5, 0x00, 0x14, 0}
	cfg := buildVarpConfigIdx(body)
	got := unpackVarp(cfg, 0, makePackFile(0, "testvarp"), captureWarnings(new([]string)))
	want := []string{"[testvarp]", "clientcode=10", "clientcode=20"}
	assertLines(t, want, got)
}
