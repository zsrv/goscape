package login

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/zsrv/goscape/pkg/io/packet"
)

// makeValidSave builds a minimal SAV blob that passes verifySave: magic 0x2004,
// version 6, the int32 playtime at offset 24, and a trailing 4-byte CRC over
// the preceding body.
func makeValidSave(playtime int32) []byte {
	body := make([]byte, 28) // header through playtime (offset 24..27)
	body[0], body[1] = 0x20, 0x04 // magic 0x2004
	body[2], body[3] = 0x00, 0x06 // version 6
	body[24] = byte(playtime >> 24)
	body[25] = byte(playtime >> 16)
	body[26] = byte(playtime >> 8)
	body[27] = byte(playtime)
	crc := packet.GetCRC(body, 0, len(body))
	return append(body, byte(crc>>24), byte(crc>>16), byte(crc>>8), byte(crc))
}

func TestVerifySave(t *testing.T) {
	good := makeValidSave(123)
	if !verifySave(good) {
		t.Error("valid save must verify")
	}
	if pt, ok := savePlaytime(good); !ok || pt != 123 {
		t.Errorf("savePlaytime: got (%d,%v), want (123,true)", pt, ok)
	}

	// Bad magic.
	badMagic := makeValidSave(1)
	badMagic[0] = 0x00
	if verifySave(badMagic) {
		t.Error("bad magic must fail verify")
	}

	// Version above max.
	badVer := makeValidSave(1)
	badVer[3] = savMaxVersion + 1
	if verifySave(badVer) {
		t.Error("version > savMaxVersion must fail verify")
	}

	// Corrupt CRC.
	badCrc := makeValidSave(1)
	badCrc[len(badCrc)-1] ^= 0xFF
	if verifySave(badCrc) {
		t.Error("corrupt CRC must fail verify")
	}

	// Too short.
	if verifySave([]byte{0x20, 0x04}) {
		t.Error("too-short blob must fail verify")
	}
}

func TestWouldResetSaveFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "p.sav")

	// No existing save: never a rollback.
	reset, err := wouldResetSaveFile(path, makeValidSave(50))
	if err != nil {
		t.Fatalf("wouldResetSaveFile: %v", err)
	}
	if reset {
		t.Error("no existing save: must not be a rollback")
	}

	// Existing playtime 100; new 50 → rollback.
	if err := os.WriteFile(path, makeValidSave(100), 0o644); err != nil {
		t.Fatal(err)
	}
	reset, err = wouldResetSaveFile(path, makeValidSave(50))
	if err != nil {
		t.Fatalf("wouldResetSaveFile: %v", err)
	}
	if !reset {
		t.Error("existing(100) > new(50): must be a rollback")
	}

	// New playtime >= existing → not a rollback.
	reset, err = wouldResetSaveFile(path, makeValidSave(150))
	if err != nil {
		t.Fatalf("wouldResetSaveFile: %v", err)
	}
	if reset {
		t.Error("new(150) >= existing(100): must not be a rollback")
	}
}
