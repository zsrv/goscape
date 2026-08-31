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
// Shape: TS CrcTable.ts:11-29 at dee467c8 — count = cache.count(0); loop
// i=0..count; Read(0, i, false) → nil for absent/zero-entry files → p4(0).
// CrcBuffer is a fixed 40-byte wire buffer (4*9+4): 36 CRC slots + a 4-byte
// rolling-hash trailer. CrcTable has exactly count entries.
//
// Fixture: archive 0 files 1, 2, 4 written (gap at 3; file 0 absent).
// → idx[0] spans entries 0..4 (30 bytes) → Count(0)=5.
// Slot 0: absent (zero idx entry, sector=0 → Read nil → p4(0)).
// Slot 1: data1 → p4(crc1).
// Slot 2: data2 → p4(crc2).
// Slot 3: gap (zero idx entry → Read nil → p4(0)).
// Slot 4: data4 → p4(crc4).
// Bytes: 40 bytes (5*4 = 20 content + 16 zero CRC padding + 4 rolling hash).
// (The rolling-hash trailer is pinned separately in
// TestMakeCRCsRollingHashTrailer; this test keeps the original slot/padding
// assertions over the first 36 bytes.)
func TestMakeCRCsFromFileStream(t *testing.T) {
	dir := t.TempDir()

	// Write archive 0 files 1, 2, 4 — leave 0 and 3 absent.
	data1 := []byte("title archive data")
	data2 := []byte("config archive data")
	data4 := []byte("media archive data")

	{
		fs, err := filestream.New(dir, true, false)
		if err != nil {
			t.Fatalf("filestream.New: %v", err)
		}
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

	if err := MakeCRCs(dir); err != nil {
		t.Fatalf("MakeCRCs: %v", err)
	}

	snap := CRC()
	if snap == nil {
		t.Fatal("CRC() is nil after MakeCRCs")
	}

	// Bytes must be exactly 40 (fixed wire size, TS CrcBuffer =
	// new Packet(new Uint8Array(4*9+4)) at rev-274: 36 CRC slots + 4 hash).
	if len(snap.Bytes) != 40 {
		t.Errorf("Bytes len = %d, want 40", len(snap.Bytes))
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

// rollingHash mirrors TS CrcTable.ts:25-28 at dee467c8 (rev-274):
//
//	let hash = 1234;
//	for (let i = 0; i < 9; i++) { hash = (hash << 1) + CrcTable[i]; }
//
// JS coerces to int32 at each `<<`; the faithful Go is int32 arithmetic with
// two's-complement wrap. Missing slots (table shorter than 9) → 0.
//
// This test helper is verified against an independent `node -e` computation in
// TestMakeCRCsRollingHashTrailer's hardcoded vectors below.
func rollingHash(table []uint32) uint32 {
	hash := int32(1234)
	for i := range 9 {
		var c uint32
		if i < len(table) {
			c = table[i]
		}
		hash = (hash << 1) + int32(c)
	}
	return uint32(hash)
}

// TestRollingHashMatchesNode pins the rolling-hash arithmetic against values
// independently computed with `node -e` (JS int32 semantics). These are the
// load-bearing pins that lock goscape's Go int32 wrap to the TS coercion:
//
//	node: hash=1234; for(i=0;i<9;i++) hash=(hash<<1)+(table[i]|0); hash>>>0
//
// Vector A (5 entries, indices 5-8 missing → 0): 0x555ef940 = [85 94 249 64].
// Vector B (full 9 entries, large CRCs exercise two's-complement wrap):
// 0x91526b0e = [145 82 107 14].
func TestRollingHashMatchesNode(t *testing.T) {
	vecA := []uint32{0x00000000, 0x11111111, 0x22222222, 0x00000000, 0x44444444}
	if got := rollingHash(vecA); got != 0x555ef940 {
		t.Errorf("rollingHash(A) = 0x%08x, want 0x555ef940 (node)", got)
	}

	vecB := []uint32{0xFFFFFFFF, 0x80000000, 0x7FFFFFFF, 0xDEADBEEF, 0xCAFEBABE, 0x12345678, 0x9ABCDEF0, 0x0F0F0F0F, 0xF0F0F0F0}
	if got := rollingHash(vecB); got != 0x91526b0e {
		t.Errorf("rollingHash(B) = 0x%08x, want 0x91526b0e (node)", got)
	}
}

// TestMakeCRCsRollingHashTrailer pins the rev-274 change (TS CrcTable.ts
// @dee467c8): CrcBuffer grows from 4*9 (36) to 4*9+4 (40) bytes, and a 4-byte
// big-endian rolling hash is appended after the 9 archive CRCs:
//
//	hash = 1234; for i in 0..9: hash = (hash << 1) + CrcTable[i]; p4(hash)
//
// Bytes must be 40: the 36 CRC bytes + the 4-byte hash trailer. Table is
// UNCHANGED (still the raw per-slot CRCs for login compare).
func TestMakeCRCsRollingHashTrailer(t *testing.T) {
	dir := t.TempDir()

	data1 := []byte("title archive data")
	data2 := []byte("config archive data")
	data4 := []byte("media archive data")

	{
		fs, err := filestream.New(dir, true, false)
		if err != nil {
			t.Fatalf("filestream.New: %v", err)
		}
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

	if err := MakeCRCs(dir); err != nil {
		t.Fatalf("MakeCRCs: %v", err)
	}

	snap := CRC()
	if snap == nil {
		t.Fatal("CRC() is nil after MakeCRCs")
	}

	// Bytes must now be 40: 36 CRC bytes + 4-byte rolling-hash trailer.
	if len(snap.Bytes) != 40 {
		t.Fatalf("Bytes len = %d, want 40 (36 CRCs + 4 rolling-hash bytes)", len(snap.Bytes))
	}

	// Table UNCHANGED: still the 5 raw per-slot CRCs (login compare path).
	crc1 := packet.GetCRC(data1, 0, len(data1))
	crc2 := packet.GetCRC(data2, 0, len(data2))
	crc4 := packet.GetCRC(data4, 0, len(data4))
	wantTable := []uint32{0, crc1, crc2, 0, crc4}
	if len(snap.Table) != len(wantTable) {
		t.Fatalf("Table len = %d, want %d (login-compare table unchanged)", len(snap.Table), len(wantTable))
	}
	for i, want := range wantTable {
		if snap.Table[i] != want {
			t.Errorf("Table[%d] = 0x%08x, want 0x%08x", i, snap.Table[i], want)
		}
	}

	// First 36 bytes are the CRC slots (unchanged from the 225/244 shape).
	if got := binary.BigEndian.Uint32(snap.Bytes[4:8]); got != crc1 {
		t.Errorf("Bytes[4:8] = 0x%08x, want 0x%08x (crc1)", got, crc1)
	}

	// Trailer = rolling hash over the 9-slot table (indices 5-8 → 0).
	want := rollingHash(wantTable)
	if got := binary.BigEndian.Uint32(snap.Bytes[36:40]); got != want {
		t.Errorf("Bytes[36:40] (rolling hash) = 0x%08x, want 0x%08x", got, want)
	}
}

// TestMakeCRCsEmptyCache pins behaviour when the cache has no files:
// Count(0)=0 → count=0 → Bytes = 40 bytes (36 zero CRC slots + the rolling
// hash of an all-zero table). TS: when count=0 the loop body never runs, so
// CrcTable is empty; the hash loop then sums 1234<<9 with all zeros.
// CrcBuffer.data is the fixed 4*9+4 pre-allocated buffer.
func TestMakeCRCsEmptyCache(t *testing.T) {
	ResetCRCForTest()
	t.Cleanup(ResetCRCForTest)

	if err := MakeCRCs(t.TempDir()); err != nil { // fresh cache, count=0
		t.Fatalf("MakeCRCs: %v", err)
	}

	snap := CRC()
	if snap == nil {
		t.Fatal("CRC() is nil after MakeCRCs on empty cache")
	}
	if len(snap.Bytes) != 40 {
		t.Errorf("Bytes len = %d, want 40 for empty cache (36 CRCs + 4 hash)", len(snap.Bytes))
	}
	if len(snap.Table) != 0 {
		t.Errorf("Table len = %d, want 0 for empty cache", len(snap.Table))
	}
	// First 36 bytes are zero (no CRC slots written).
	for i := range 36 {
		if snap.Bytes[i] != 0 {
			t.Errorf("Bytes[%d] = 0x%02x, want 0 for empty cache", i, snap.Bytes[i])
		}
	}
	// Trailer = rolling hash of an all-zero (empty) table = 1234 << 9 as int32.
	want := rollingHash(nil)
	if got := binary.BigEndian.Uint32(snap.Bytes[36:40]); got != want {
		t.Errorf("Bytes[36:40] (empty-cache hash) = 0x%08x, want 0x%08x", got, want)
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
	if err := MakeCRCs(dir); err != nil {
		t.Fatalf("MakeCRCs: %v", err)
	}
	old := CRC()
	oldBytesCopy := append([]byte(nil), old.Bytes...)
	oldTableCopy := append([]uint32(nil), old.Table...)

	if err := MakeCRCs(dir); err != nil {
		t.Fatalf("MakeCRCs: %v", err)
	}
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
// snapshot via the atomic.Pointer swap. The 274 shape always produces
// exactly 40 bytes (fixed CrcBuffer wire size, faithful to TS 4*9+4 pre-alloc).
func TestMakeCRCsPopulatesSnapshot(t *testing.T) {
	ResetCRCForTest()
	t.Cleanup(ResetCRCForTest)

	if err := MakeCRCs(t.TempDir()); err != nil {
		t.Fatalf("MakeCRCs: %v", err)
	}

	snap := CRC()
	if snap == nil {
		t.Fatal("CRC() is nil after MakeCRCs")
	}
	// 274: always 40 bytes (fixed wire buffer regardless of count).
	if len(snap.Bytes) != 40 {
		t.Errorf("CRC().Bytes len = %d, want 40", len(snap.Bytes))
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
