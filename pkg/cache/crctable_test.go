package cache

import (
	"bytes"
	"encoding/binary"
	"log/slog"
	"strings"
	"testing"

	"github.com/zsrv/goscape/pkg/io/filestream"
	"github.com/zsrv/goscape/pkg/io/packet"
)

// TestResetCRCForTest_NilsSnapshot pins that ResetCRCForTest clears the
// stored snapshot so subsequent CRC() calls return the zero-value
// snapshot (the empty-but-non-nil sentinel that preserves the prior
// pre-init contract where the package-level CrcBytes/CrcTable were nil).
func TestResetCRCForTest_NilsSnapshot(t *testing.T) {
	SetCRCForTest(&CRCSnapshot{
		Bytes: []byte{0xDE, 0xAD, 0xBE, 0xEF},
		Table: []uint32{1, 2, 3},
	})

	ResetCRCForTest()

	got := CRC()
	if got == nil {
		t.Fatal("CRC() returned nil after reset; want zero-value sentinel")
	}
	if len(got.Bytes) != 0 {
		t.Errorf("CRC().Bytes len = %d, want 0", len(got.Bytes))
	}
	if len(got.Table) != 0 {
		t.Errorf("CRC().Table len = %d, want 0", len(got.Table))
	}
}

// TestMakeCRCsFromFileStream pins the 244 shape: MakeCRCs reads archive 0
// from a FileStream (dat/idx cache), not loose files.
//
// Shape: TS CrcTable.ts:11-27 at 9aadcec4 — count = cache.count(0); loop
// i=0..count; Read(0, i, false) → nil for absent/zero-entry files → p4(0).
// CrcBuffer is a fixed 36-byte wire buffer (4*9); count entries written,
// remainder zero-padded. CrcTable has exactly count entries.
//
// Fixture: archive 0 files 1, 2, 4 written (gap at 3; file 0 absent).
// → idx[0] spans entries 0..4 (30 bytes) → Count(0)=5.
// Slot 0: absent (zero idx entry, sector=0 → Read nil → p4(0)).
// Slot 1: data1 → p4(crc1).
// Slot 2: data2 → p4(crc2).
// Slot 3: gap (zero idx entry → Read nil → p4(0)).
// Slot 4: data4 → p4(crc4).
// Bytes: 36 bytes (5*4 = 20 content + 16 zero padding).
func TestMakeCRCsFromFileStream(t *testing.T) {
	dir := t.TempDir()

	// Write archive 0 files 1, 2, 4 — leave 0 and 3 absent.
	data1 := []byte("title archive data")
	data2 := []byte("config archive data")
	data4 := []byte("media archive data")

	{
		fs := filestream.New(dir, true, false)
		if !fs.Write(0, 1, data1, 0) {
			t.Fatal("Write(0,1) failed")
		}
		if !fs.Write(0, 2, data2, 0) {
			t.Fatal("Write(0,2) failed")
		}
		if !fs.Write(0, 4, data4, 0) {
			t.Fatal("Write(0,4) failed")
		}
		if err := fs.Close(); err != nil {
			t.Fatalf("Close (write) failed: %v", err)
		}
	}

	ResetCRCForTest()
	t.Cleanup(ResetCRCForTest)

	MakeCRCs(dir)

	snap := CRC()
	if snap == nil {
		t.Fatal("CRC() is nil after MakeCRCs")
	}

	// Bytes must be exactly 36 (fixed wire size, TS CrcBuffer = new Packet(new Uint8Array(4*9))).
	if len(snap.Bytes) != 36 {
		t.Errorf("Bytes len = %d, want 36", len(snap.Bytes))
	}

	// Table must have exactly count entries (= Count(0) = 5).
	if len(snap.Table) != 5 {
		t.Errorf("Table len = %d, want 5", len(snap.Table))
	}

	// Compute expected CRCs.
	crc1 := packet.GetCRC(data1, 0, len(data1))
	crc2 := packet.GetCRC(data2, 0, len(data2))
	crc4 := packet.GetCRC(data4, 0, len(data4))

	wantTable := []uint32{0, crc1, crc2, 0, crc4}
	for i, want := range wantTable {
		if snap.Table[i] != want {
			t.Errorf("Table[%d] = 0x%08x, want 0x%08x", i, snap.Table[i], want)
		}
	}

	// Bytes[0..3] = slot 0 = 0 (file 0 absent → natural leading zero, mirrors 225 p4(0) header).
	if got := binary.BigEndian.Uint32(snap.Bytes[0:4]); got != 0 {
		t.Errorf("Bytes[0:4] = 0x%08x, want 0 (file 0 absent → natural leading zero)", got)
	}
	// Bytes[4..7] = slot 1 = crc1.
	if got := binary.BigEndian.Uint32(snap.Bytes[4:8]); got != crc1 {
		t.Errorf("Bytes[4:8] = 0x%08x, want 0x%08x (crc1)", got, crc1)
	}
	// Bytes[8..11] = slot 2 = crc2.
	if got := binary.BigEndian.Uint32(snap.Bytes[8:12]); got != crc2 {
		t.Errorf("Bytes[8:12] = 0x%08x, want 0x%08x (crc2)", got, crc2)
	}
	// Bytes[12..15] = slot 3 = 0 (gap).
	if got := binary.BigEndian.Uint32(snap.Bytes[12:16]); got != 0 {
		t.Errorf("Bytes[12:16] = 0x%08x, want 0 (gap at file 3)", got)
	}
	// Bytes[16..19] = slot 4 = crc4.
	if got := binary.BigEndian.Uint32(snap.Bytes[16:20]); got != crc4 {
		t.Errorf("Bytes[16:20] = 0x%08x, want 0x%08x (crc4)", got, crc4)
	}
	// Bytes[20..35] = zero padding (count=5 < 9 → 4 zero slots).
	for i := 20; i < 36; i++ {
		if snap.Bytes[i] != 0 {
			t.Errorf("Bytes[%d] = 0x%02x, want 0 (zero padding)", i, snap.Bytes[i])
		}
	}
}

// TestMakeCRCsEmptyCache pins behaviour when the cache has no files:
// Count(0)=0 → count=0 → Bytes = 36 zero bytes, Table = empty.
// TS: when count=0 the loop body never runs; CrcBuffer.data is still 4*9
// zero bytes (the fixed pre-allocated buffer, never written into).
func TestMakeCRCsEmptyCache(t *testing.T) {
	ResetCRCForTest()
	t.Cleanup(ResetCRCForTest)

	MakeCRCs(t.TempDir()) // fresh cache, count=0

	snap := CRC()
	if snap == nil {
		t.Fatal("CRC() is nil after MakeCRCs on empty cache")
	}
	if len(snap.Bytes) != 36 {
		t.Errorf("Bytes len = %d, want 36 for empty cache", len(snap.Bytes))
	}
	if len(snap.Table) != 0 {
		t.Errorf("Table len = %d, want 0 for empty cache", len(snap.Table))
	}
	for i, b := range snap.Bytes {
		if b != 0 {
			t.Errorf("Bytes[%d] = 0x%02x, want 0 for empty cache", i, b)
		}
	}
}

// TestMakeCRCsSwapIsAtomic pins that consecutive MakeCRCs calls publish
// independent snapshots (no in-place mutation aliasing prior reads).
// Captures a snapshot, calls MakeCRCs again, verifies the captured
// pointer still observes its original Bytes/Table — i.e. the new
// snapshot did not retroactively edit the old one.
func TestMakeCRCsSwapIsAtomic(t *testing.T) {
	ResetCRCForTest()
	t.Cleanup(ResetCRCForTest)

	dir := t.TempDir()
	MakeCRCs(dir)
	old := CRC()
	oldBytesCopy := append([]byte(nil), old.Bytes...)
	oldTableCopy := append([]uint32(nil), old.Table...)

	MakeCRCs(dir)
	newSnap := CRC()
	if newSnap == old {
		t.Fatal("MakeCRCs reused the prior pointer; expected fresh snapshot")
	}
	if !bytes.Equal(old.Bytes, oldBytesCopy) {
		t.Error("captured snapshot's Bytes was mutated by subsequent MakeCRCs")
	}
	if len(old.Table) != len(oldTableCopy) {
		t.Error("captured snapshot's Table was mutated by subsequent MakeCRCs")
	}
}

// TestMakeCRCsPopulatesSnapshot pins that MakeCRCs publishes a non-nil
// snapshot via the atomic.Pointer swap. The 244 shape always produces
// exactly 36 bytes (fixed CrcBuffer wire size, faithful to TS 4*9 pre-alloc).
func TestMakeCRCsPopulatesSnapshot(t *testing.T) {
	ResetCRCForTest()
	t.Cleanup(ResetCRCForTest)

	MakeCRCs(t.TempDir())

	snap := CRC()
	if snap == nil {
		t.Fatal("CRC() is nil after MakeCRCs")
	}
	// 244: always 36 bytes (fixed wire buffer regardless of count).
	if len(snap.Bytes) != 36 {
		t.Errorf("CRC().Bytes len = %d, want 36", len(snap.Bytes))
	}
	// Table may be empty (empty cache → count=0 → no entries).
	// No table-length assertion here; TestMakeCRCsEmptyCache covers that path.
}

// TestLoadCrcWarnsOnMissingFile pins NAI-20 Task 3 C2 (carried forward
// from the old TestMakeCrcWarnsOnMissingFile): loadCrc emits a slog.Warn
// on os.Stat failure and returns 0.
//
// NOTE: loadCrc is the 225-era loose-file helper, retained for the
// transition period (B6-deferred format window). It is NOT called by the
// 244 MakeCRCs path; this test pins the helper's contract independently.
func TestLoadCrcWarnsOnMissingFile(t *testing.T) {
	prev := slog.Default()
	t.Cleanup(func() { slog.SetDefault(prev) })

	var buf bytes.Buffer
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf,
		&slog.HandlerOptions{Level: slog.LevelWarn})))

	got := loadCrc("/nonexistent/path/that/should/never/exist")
	if got != 0 {
		t.Errorf("loadCrc on missing file = 0x%08x, want 0", got)
	}

	out := buf.String()
	if out == "" {
		t.Fatalf("expected slog.Warn output, got empty buffer")
	}
	if !strings.Contains(out, "loadCrc Stat failed") {
		t.Errorf("expected 'loadCrc Stat failed' in output; got: %s", out)
	}
	if !strings.Contains(out, "/nonexistent/path/that/should/never/exist") {
		t.Errorf("expected path in output; got: %s", out)
	}
}
