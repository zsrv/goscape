package world

import (
	"encoding/binary"
	"testing"

	"github.com/zsrv/goscape/pkg/io/packet"
	"github.com/zsrv/goscape/pkg/objtype"
)

func TestVerifySave_RejectsTooSmall(t *testing.T) {
	if VerifySave(nil) {
		t.Error("VerifySave(nil) should be false")
	}
	if VerifySave([]byte{0x20}) {
		t.Error("VerifySave([0x20]) should be false (too short for magic)")
	}
}

func TestVerifySave_AcceptsWellFormed(t *testing.T) {
	sav := buildValidSav(t, 6, []byte{0xAA, 0xBB})
	if !VerifySave(sav) {
		t.Error("VerifySave on well-formed v6 sav should be true")
	}
}

func TestVerifySave_RejectsBadMagic(t *testing.T) {
	sav := buildValidSav(t, 6, []byte{0x00})
	sav[0] = 0xFF
	if VerifySave(sav) {
		t.Error("VerifySave with corrupted magic should be false")
	}
}

func TestVerifySave_RejectsUnsupportedVer(t *testing.T) {
	sav := buildValidSav(t, 7, []byte{0x00})
	if VerifySave(sav) {
		t.Error("VerifySave with version=7 should be false")
	}
	sav = buildValidSav(t, 0, []byte{0x00})
	if VerifySave(sav) {
		t.Error("VerifySave with version=0 should be false")
	}
}

func TestVerifySave_RejectsCorruptCRC(t *testing.T) {
	sav := buildValidSav(t, 6, []byte{0xAA})
	sav[len(sav)-1] ^= 0xFF
	if VerifySave(sav) {
		t.Error("VerifySave with corrupted CRC should be false")
	}
}

func TestLoadSave_EmptyByteSliceBootstraps(t *testing.T) {
	p := &Player{}
	if err := LoadSave(p, []byte{}); err != nil {
		t.Fatalf("LoadSave(empty) returned err: %v", err)
	}
	for i := range objtype.PlayerStatCount {
		if i == objtype.PlayerStatHitpoints {
			continue
		}
		if p.stats[i] != 0 || p.baseLevels[i] != 1 || p.levels[i] != 1 {
			t.Errorf("stat %d: got (stats=%d, base=%d, lvl=%d), want (0, 1, 1)",
				i, p.stats[i], p.baseLevels[i], p.levels[i])
		}
	}
	wantHpExp := int32(objtype.GetExpByLevel(10))
	if p.stats[objtype.PlayerStatHitpoints] != wantHpExp {
		t.Errorf("hp stats: got %d, want %d", p.stats[objtype.PlayerStatHitpoints], wantHpExp)
	}
	if p.baseLevels[objtype.PlayerStatHitpoints] != 10 || p.levels[objtype.PlayerStatHitpoints] != 10 {
		t.Errorf("hp levels: got (base=%d, lvl=%d), want (10, 10)",
			p.baseLevels[objtype.PlayerStatHitpoints], p.levels[objtype.PlayerStatHitpoints])
	}
}

func TestLoadSave_NilSliceBootstraps(t *testing.T) {
	p := &Player{}
	if err := LoadSave(p, nil); err != nil {
		t.Fatalf("LoadSave(nil) returned err: %v", err)
	}
	if p.stats[objtype.PlayerStatHitpoints] != int32(objtype.GetExpByLevel(10)) {
		t.Errorf("nil-slice path didn't bootstrap hp like empty-slice path")
	}
}

// buildValidSav constructs a minimal SAV with the given version and
// payload bytes, including a trailing valid CRC. Used by Verify tests.
func buildValidSav(t *testing.T, version uint16, payload []byte) []byte {
	t.Helper()
	p := packet.NewPacket(make([]byte, 0, 16))
	p.P2(SavMagic)
	p.P2(version)
	for _, b := range payload {
		p.P1(b)
	}
	body := append([]byte{}, p.Data...)
	crc := packet.GetCRC(body, 0, len(body))
	out := append(body, 0, 0, 0, 0)
	binary.BigEndian.PutUint32(out[len(body):], crc)
	return out
}
