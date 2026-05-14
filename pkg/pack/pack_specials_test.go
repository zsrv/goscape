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

// TestPackAndSaveFrameDel_BytePin asserts frame_del.dat byte layout
// for a 3-slot AnimPack (foo at id 0, gap at id 1, bar at id 2) with
// only foo.frame present on disk. Matches TS PackShared.ts:355-388.
//
// Synthetic foo.frame layout (32 bytes total):
//
//	bytes  0..5 : head segment (6 bytes)          — aa bb cc dd ee ff
//	bytes  6..9 : tran1 segment (4 bytes)         — 11 22 33 44
//	bytes 10..11: tran2 segment (2 bytes)         — 55 66
//	bytes 12..23: del segment (12 bytes)          — 42 99 99 99 99 99 99 99 99 99 99 99
//	bytes 24..31: trailer (4×u16 BE)              — 00 06 00 04 00 02 00 0c
//	               = head=6, tran1=4, tran2=2, del=12
//
// TS reads pos=len-8=24, three g2 → 6, 4, 2 (discards 4th g2). Then
// pos=0+6+4+2=12, reads g1() = 0x42 (del[0]). Expected output for foo.
//
// Expected frame_del.dat: 42 (foo id 0) 00 (id 1 gap) 00 (bar id 2,
// no .frame file). Total 3 bytes.
func TestPackAndSaveFrameDel_BytePin(t *testing.T) {
	srcDir := t.TempDir()
	serverOut := t.TempDir()

	writeFile(t, filepath.Join(srcDir, "pack", "anim.pack"),
		"0=foo\n2=bar\n")

	fooFrame := []byte{
		0xaa, 0xbb, 0xcc, 0xdd, 0xee, 0xff, // head[6]
		0x11, 0x22, 0x33, 0x44, // tran1[4]
		0x55, 0x66, // tran2[2]
		0x42, 0x99, 0x99, 0x99, 0x99, 0x99, 0x99, 0x99, 0x99, 0x99, 0x99, 0x99, // del[12]
		0x00, 0x06, 0x00, 0x04, 0x00, 0x02, 0x00, 0x0c, // trailer
	}
	modelsDir := filepath.Join(srcDir, "models")
	if err := os.MkdirAll(modelsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(modelsDir, "foo.frame"), fooFrame, 0o644); err != nil {
		t.Fatal(err)
	}
	ClearFsCache()

	pf, err := NewPackFile(srcDir, "anim", nil)
	if err != nil {
		t.Fatalf("NewPackFile: %v", err)
	}
	if err := packAndSaveFrameDel(srcDir, serverOut, pf); err != nil {
		t.Fatalf("packAndSaveFrameDel: %v", err)
	}

	got, err := os.ReadFile(filepath.Join(serverOut, "frame_del.dat"))
	if err != nil {
		t.Fatalf("read frame_del.dat: %v", err)
	}
	want := []byte{0x42, 0x00, 0x00}
	if !bytes.Equal(got, want) {
		t.Fatalf("frame_del.dat mismatch:\ngot  % x\nwant % x", got, want)
	}
}

// TestPackAndSaveFrameDel_EmptyAnimPack asserts that an AnimPack with
// no registered names (no <src>/pack/anim.pack file) produces a
// zero-byte frame_del.dat. Matches TS no-src behaviour: Packet.alloc(3)
// is empty, loop runs 0 times, save writes 0 bytes.
func TestPackAndSaveFrameDel_EmptyAnimPack(t *testing.T) {
	srcDir := t.TempDir()
	serverOut := t.TempDir()
	ClearFsCache()

	pf, err := NewPackFile(srcDir, "anim", nil)
	if err != nil {
		t.Fatalf("NewPackFile: %v", err)
	}
	if err := packAndSaveFrameDel(srcDir, serverOut, pf); err != nil {
		t.Fatalf("packAndSaveFrameDel: %v", err)
	}

	got, err := os.ReadFile(filepath.Join(serverOut, "frame_del.dat"))
	if err != nil {
		t.Fatalf("read frame_del.dat: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("frame_del.dat for empty AnimPack: got %d bytes (% x), want 0", len(got), got)
	}
}
