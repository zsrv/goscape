package login

import (
	"os"
	"path/filepath"
	"testing"

	world "github.com/zsrv/goscape/modules/world"
	"github.com/zsrv/goscape/pkg/io/packet"
	"github.com/zsrv/goscape/pkg/objtype"
)

// makeValidSave builds a minimal SAV blob that passes verifySave: magic 0x2004,
// version 6, the int32 playtime at offset 24, and a trailing 4-byte CRC over
// the preceding body.
func makeValidSave(playtime int32) []byte {
	body := make([]byte, 28)      // header through playtime (offset 24..27)
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

	// Version 0 (zeroed header) — lower bound mirrors the world decoder.
	badVer0 := makeValidSave(1)
	badVer0[2], badVer0[3] = 0x00, 0x00
	if verifySave(badVer0) {
		t.Error("version 0 must fail verify")
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

// makeSaveWithStats builds a version-6 SAV blob carrying the 21 stat XP values
// (each i32 XP + u8 level, level byte left 0). Extends makeValidSave's header
// (offset 0..27) with the 105-byte stat block at offset 28, then a trailing CRC.
func makeSaveWithStats(playtime int32, xps [objtype.PlayerStatCount]int32) []byte {
	const stride = 5
	body := make([]byte, 28+objtype.PlayerStatCount*stride)
	body[0], body[1] = 0x20, 0x04 // magic 0x2004
	body[2], body[3] = 0x00, 0x06 // version 6
	body[24] = byte(playtime >> 24)
	body[25] = byte(playtime >> 16)
	body[26] = byte(playtime >> 8)
	body[27] = byte(playtime)
	for i := range objtype.PlayerStatCount {
		o := 28 + i*stride
		x := uint32(xps[i])
		body[o] = byte(x >> 24)
		body[o+1] = byte(x >> 16)
		body[o+2] = byte(x >> 8)
		body[o+3] = byte(x)
		// body[o+4] (current-level byte) intentionally left 0 — saveStats ignores it.
	}
	crc := packet.GetCRC(body, 0, len(body))
	return append(body, byte(crc>>24), byte(crc>>16), byte(crc>>8), byte(crc))
}

func TestSaveStats(t *testing.T) {
	var xps [objtype.PlayerStatCount]int32
	for i := range xps {
		xps[i] = int32((i + 1) * 1000)
	}
	got, ok := saveStats(makeSaveWithStats(42, xps))
	if !ok {
		t.Fatal("saveStats: ok=false for a valid stat-carrying blob")
	}
	if got != xps {
		t.Errorf("saveStats: got %v, want %v", got, xps)
	}

	// A header-only blob (no stat block) is too short → ok=false.
	if _, ok := saveStats(makeValidSave(0)); ok {
		t.Error("saveStats: ok=true for a blob with no stat block")
	}

	// Version 1: playtime is u16, so the stat block begins at offset 26 (not 28).
	v1Body := make([]byte, 26+objtype.PlayerStatCount*5)
	v1Body[0], v1Body[1] = 0x20, 0x04 // magic 0x2004
	v1Body[2], v1Body[3] = 0x00, 0x01 // version 1
	for i := range objtype.PlayerStatCount {
		o := 26 + i*5
		x := uint32(xps[i])
		v1Body[o] = byte(x >> 24)
		v1Body[o+1] = byte(x >> 16)
		v1Body[o+2] = byte(x >> 8)
		v1Body[o+3] = byte(x)
	}
	crc := packet.GetCRC(v1Body, 0, len(v1Body))
	v1Blob := append(v1Body, byte(crc>>24), byte(crc>>16), byte(crc>>8), byte(crc))
	if got, ok := saveStats(v1Blob); !ok || got != xps {
		t.Errorf("saveStats v1: ok=%v got=%v want=%v", ok, got, xps)
	}
}

// TestWouldResetSaveFile_CorruptExistingSaveReturnsError pins login-server-6:
// TS PlayerLoading.load (PlayerLoading.ts:55-68) throws on bad
// magic/version/CRC of the existing save; the throw propagates out of
// LoginServer.wouldResetSaveFile (LoginServer.ts:126-141) and aborts the
// persistSave path so the new save is NOT written over a corrupt existing
// one. Go must mirror that fail-safe by returning an error rather than
// silently treating garbage playtime as authoritative.
func TestWouldResetSaveFile_CorruptExistingSaveReturnsError(t *testing.T) {
	cases := []struct {
		name string
		mut  func([]byte)
	}{
		{"bad magic", func(b []byte) { b[0] = 0x00 }},
		{"unsupported version", func(b []byte) { b[3] = savMaxVersion + 1 }},
		{"corrupt crc", func(b []byte) { b[len(b)-1] ^= 0xFF }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "p.sav")
			corrupt := makeValidSave(100)
			tc.mut(corrupt)
			if err := os.WriteFile(path, corrupt, 0o644); err != nil {
				t.Fatal(err)
			}
			_, err := wouldResetSaveFile(path, makeValidSave(50))
			if err == nil {
				t.Fatal("TS PlayerLoading.load throws on bad magic/version/CRC (login-server-6) but wouldResetSaveFile returned nil error")
			}
		})
	}
}

// TestSavMaxVersionMatchesWorld pins the duplicated login-side save
// version gate to the world module's authoritative SavVersion. The
// constants are duplicated to avoid a login→world production dependency
// (save.go:12-21); this test-only import is the drift guard — rev-254 A6
// bumped the world to 7 and the stale login gate silently discarded
// every logout/autosave until caught in review.
func TestSavMaxVersionMatchesWorld(t *testing.T) {
	if savMaxVersion != int(world.SavVersion) {
		t.Fatalf("savMaxVersion = %d, want world.SavVersion = %d — bump modules/login/save.go in the same commit as any world save-format change",
			savMaxVersion, world.SavVersion)
	}
	if savMagic != int(world.SavMagic) {
		t.Fatalf("savMagic = %#x, want world.SavMagic = %#x", savMagic, world.SavMagic)
	}
}
