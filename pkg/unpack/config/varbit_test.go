package config

import (
	"testing"

	"github.com/zsrv/goscape/pkg/io/packet"
)

// buildVarbitConfigIdx builds a single-entry ConfigIdx from raw opcode bytes.
func buildVarbitConfigIdx(body []byte) *ConfigIdx {
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

// TestUnpackVarbit_Code1 pins the 4-line def for opcode 1:
// basevar resolved through the varp pack, then startbit/endbit.
//
// TS source: VarbitConfig.ts:19-27 @2e3bcf43.
func TestUnpackVarbit_Code1(t *testing.T) {
	// opcode 1, varpId g2 = 7, startbit g1 = 3, endbit g1 = 5, terminator
	body := []byte{1, 0x00, 7, 3, 5, 0}
	cfg := buildVarbitConfigIdx(body)
	varbitPack := makePackFile(0, "myvarbit")
	varpPack := makePackFile(7, "quest_progress")
	got := unpackVarbit(cfg, 0, varbitPack, varpPack, captureWarnings(new([]string)))
	want := []string{"[myvarbit]", "basevar=quest_progress", "startbit=3", "endbit=5"}
	assertLines(t, want, got)
}

// TestUnpackVarbit_Code1_VarpFallback pins the `varp_<id>` fallback when the
// varp id has no registered name.
//
// TS source: VarbitConfig.ts:23 (`VarpPack.getById(varpId) || `varp_${varpId}“).
func TestUnpackVarbit_Code1_VarpFallback(t *testing.T) {
	body := []byte{1, 0x01, 0x05, 0, 31, 0} // varpId=261, startbit=0, endbit=31
	cfg := buildVarbitConfigIdx(body)
	got := unpackVarbit(cfg, 0, nil, nil, captureWarnings(new([]string)))
	want := []string{"[]", "basevar=varp_261", "startbit=0", "endbit=31"}
	assertLines(t, want, got)
}

// TestUnpackVarbit_UnknownOpcodeWarning pins the unknown-code warning text.
//
// TS source: VarbitConfig.ts:28-30 (`unknown varbit code ${code}`).
func TestUnpackVarbit_UnknownOpcodeWarning(t *testing.T) {
	body := []byte{42, 0}
	cfg := buildVarbitConfigIdx(body)
	var warns []string
	unpackVarbit(cfg, 0, nil, nil, captureWarnings(&warns))
	if len(warns) != 1 || warns[0] != "unknown varbit code 42" {
		t.Errorf("want [\"unknown varbit code 42\"], got %v", warns)
	}
}

// TestUnpackVarbit_PositionMismatchWarning pins the incomplete-read warning.
//
// TS source: VarbitConfig.ts:33-35.
func TestUnpackVarbit_PositionMismatchWarning(t *testing.T) {
	// body = opcode 1 (5 bytes) + terminator = 6 bytes; idx lies len[0]=5.
	body := []byte{1, 0x00, 7, 3, 5, 0}
	dat := make([]byte, 2+len(body))
	copy(dat[2:], body)
	cfg := &ConfigIdx{
		Size: 1,
		Pos:  []int{2},
		Len:  []int{5}, // lie
		Dat:  packet.NewPacket(dat),
	}

	var warns []string
	unpackVarbit(cfg, 0, nil, nil, captureWarnings(&warns))

	if len(warns) == 0 {
		t.Fatal("expected position-mismatch warning, got none")
	}
	want := "incomplete read: 8 != 7"
	if warns[0] != want {
		t.Errorf("want %q got %q", want, warns[0])
	}
}
