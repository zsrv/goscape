package pack

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

// TestPackAndSaveCategoryDat_BytePin asserts the exact byte layout
// of category.dat for a 3-entry dense CategoryPack: matches TS
// PackShared.ts:341-352 (p2(size); per id: p1(1) + pjstr(name) + p1(0)).
func TestPackAndSaveCategoryDat_BytePin(t *testing.T) {
	srcDir := t.TempDir()
	serverOut := t.TempDir()
	writeFile(t, filepath.Join(srcDir, "pack", "category.pack"),
		"0=alpha\n1=bravo\n2=charlie\n")
	ClearFsCache()

	pf, err := NewPackFile(srcDir, "category", nil)
	if err != nil {
		t.Fatalf("NewPackFile: %v", err)
	}
	if err := packAndSaveCategoryDat(serverOut, pf); err != nil {
		t.Fatalf("packAndSaveCategoryDat: %v", err)
	}

	got, err := os.ReadFile(filepath.Join(serverOut, "category.dat"))
	if err != nil {
		t.Fatalf("read category.dat: %v", err)
	}
	want := []byte{
		0x00, 0x03, // p2(3)
		0x01, 'a', 'l', 'p', 'h', 'a', 0x0a, 0x00, // record 0
		0x01, 'b', 'r', 'a', 'v', 'o', 0x0a, 0x00, // record 1
		0x01, 'c', 'h', 'a', 'r', 'l', 'i', 'e', 0x0a, 0x00, // record 2
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("category.dat mismatch:\ngot  % x\nwant % x", got, want)
	}
}

// TestPackAndSaveCategoryDat_EmptyRegistry asserts that an empty
// CategoryPack (no <src>/pack/category.pack file) produces a 2-byte
// category.dat containing just p2(0). Matches TS no-src behaviour.
func TestPackAndSaveCategoryDat_EmptyRegistry(t *testing.T) {
	srcDir := t.TempDir()
	serverOut := t.TempDir()
	ClearFsCache()

	pf, err := NewPackFile(srcDir, "category", nil)
	if err != nil {
		t.Fatalf("NewPackFile: %v", err)
	}
	if err := packAndSaveCategoryDat(serverOut, pf); err != nil {
		t.Fatalf("packAndSaveCategoryDat: %v", err)
	}

	got, err := os.ReadFile(filepath.Join(serverOut, "category.dat"))
	if err != nil {
		t.Fatalf("read category.dat: %v", err)
	}
	want := []byte{0x00, 0x00}
	if !bytes.Equal(got, want) {
		t.Fatalf("category.dat mismatch:\ngot  % x\nwant % x", got, want)
	}
}

// TestPackAndSaveFrameDel_Removed_Rev244 documents that frame_del packing
// was removed at rev-244 (TS PackShared.ts:355-388 deleted @ 9aadcec4).
// packAndSaveFrameDel no longer exists; this comment replaces the two
// byte-pin tests that covered the 225-era implementation so the history
// of the deletion decision is captured in-package.
//
// Consumer check at removal time: no Go runtime code in modules/ or cmd/
// reads frame_del.dat; pkg/io/jagfile/jagfile.go:513 retains the name
// in the known-name decode table for 225-era cache compat only. The TS
// runtime at 9aadcec4 likewise only retains it in Jagfile.ts:405.
//
// Rev-244 B6 removal: pack_specials.go packAndSaveFrameDel deleted;
// pack_configs.go frame_del branch replaced with comment. The two unit
// tests (BytePin + EmptyAnimPack) are also removed.
func TestPackAndSaveFrameDel_Removed_Rev244(t *testing.T) {
	// No-op documentation test; the production function no longer exists.
	t.Log("frame_del packing removed at rev-244; see pack_configs.go comment")
}
