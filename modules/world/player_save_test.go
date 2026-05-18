package world

import (
	"encoding/binary"
	"testing"

	"github.com/zsrv/goscape/pkg/io/packet"
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
