package pack

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestNewPackedData_WritesSizeHeader(t *testing.T) {
	pd := NewPackedData(7)

	// p2(size=7) in BE: 00 07
	if !bytes.Equal(pd.Dat.Data, []byte{0x00, 0x07}) {
		t.Fatalf("dat=% x, want 00 07", pd.Dat.Data)
	}
	if !bytes.Equal(pd.Idx.Data, []byte{0x00, 0x07}) {
		t.Fatalf("idx=% x, want 00 07", pd.Idx.Data)
	}
	if pd.Size != 7 {
		t.Fatalf("Size=%d, want 7", pd.Size)
	}
}

func TestPackedData_NextWritesTerminatorAndIdxOffset(t *testing.T) {
	pd := NewPackedData(2)
	pd.P1(1)
	pd.P1(105)
	// dat so far: 00 02 01 69  (4 bytes)
	pd.Next()
	// next() appends 0x00 to dat (5 bytes total) and writes p2(3) to idx
	// (3 = 5 - marker=2). Marker advances to 5.
	wantDat := []byte{0x00, 0x02, 0x01, 0x69, 0x00}
	wantIdx := []byte{0x00, 0x02, 0x00, 0x03}
	if !bytes.Equal(pd.Dat.Data, wantDat) {
		t.Fatalf("dat=% x, want % x", pd.Dat.Data, wantDat)
	}
	if !bytes.Equal(pd.Idx.Data, wantIdx) {
		t.Fatalf("idx=% x, want % x", pd.Idx.Data, wantIdx)
	}
}

func TestPackedData_NextTwiceTracksMarker(t *testing.T) {
	pd := NewPackedData(2)
	pd.P1(0xAA)
	pd.Next() // entry 0: 1-byte body + terminator = 2 bytes since marker
	pd.P1(0xBB)
	pd.P1(0xCC)
	pd.Next() // entry 1: 2-byte body + terminator = 3 bytes since marker

	wantDat := []byte{0x00, 0x02, 0xAA, 0x00, 0xBB, 0xCC, 0x00}
	wantIdx := []byte{0x00, 0x02, 0x00, 0x02, 0x00, 0x03}
	if !bytes.Equal(pd.Dat.Data, wantDat) {
		t.Fatalf("dat=% x, want % x", pd.Dat.Data, wantDat)
	}
	if !bytes.Equal(pd.Idx.Data, wantIdx) {
		t.Fatalf("idx=% x, want % x", pd.Idx.Data, wantIdx)
	}
}

func TestPackedData_PJStrUsesLFTerminator(t *testing.T) {
	// NAI-192 R2 pin: TS pjstr writes LF (0x0a), not NUL.
	pd := NewPackedData(1)
	pd.PJStr("hi")
	// dat: 00 01 'h' 'i' 0a
	want := []byte{0x00, 0x01, 0x68, 0x69, 0x0a}
	if !bytes.Equal(pd.Dat.Data, want) {
		t.Fatalf("dat=% x, want % x", pd.Dat.Data, want)
	}
}

func TestPackedData_SaveWritesBothFiles(t *testing.T) {
	dir := t.TempDir()
	pd := NewPackedData(1)
	pd.P1(1)
	pd.P1(105)
	pd.Next()
	datPath := filepath.Join(dir, "out.dat")
	idxPath := filepath.Join(dir, "out.idx")
	if err := pd.Save(datPath, idxPath); err != nil {
		t.Fatal(err)
	}
	gotDat, err := os.ReadFile(datPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(gotDat, []byte{0x00, 0x01, 0x01, 0x69, 0x00}) {
		t.Fatalf("dat file=% x", gotDat)
	}
	gotIdx, err := os.ReadFile(idxPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(gotIdx, []byte{0x00, 0x01, 0x00, 0x03}) {
		t.Fatalf("idx file=% x", gotIdx)
	}
}

func TestPackedData_SaveCreatesParentDir(t *testing.T) {
	dir := t.TempDir()
	pd := NewPackedData(0)
	// Empty: just the 2-byte header in both buffers.
	deepDat := filepath.Join(dir, "a", "b", "c.dat")
	deepIdx := filepath.Join(dir, "a", "b", "c.idx")
	if err := pd.Save(deepDat, deepIdx); err != nil {
		t.Fatalf("Save with missing parent dir: %v", err)
	}
	if _, err := os.Stat(deepDat); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(deepIdx); err != nil {
		t.Fatal(err)
	}
}
