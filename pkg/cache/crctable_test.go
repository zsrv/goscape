package cache

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"
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

// TestMakeCRCsPopulatesSnapshot pins that MakeCRCs publishes a non-empty
// snapshot via the atomic.Pointer swap. Missing files are silently
// skipped (loadCrc returns 0); the slot-0 header guarantees at least
// 4 bytes / 1 table entry.
func TestMakeCRCsPopulatesSnapshot(t *testing.T) {
	ResetCRCForTest()
	t.Cleanup(ResetCRCForTest)

	MakeCRCs()

	snap := CRC()
	if snap == nil {
		t.Fatal("CRC() is nil after MakeCRCs")
	}
	if len(snap.Bytes) < 4 {
		t.Errorf("CRC().Bytes len = %d, want >= 4 (header alone)", len(snap.Bytes))
	}
	if len(snap.Table) == 0 {
		t.Errorf("CRC().Table is empty after MakeCRCs")
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

	MakeCRCs()
	old := CRC()
	oldBytesCopy := append([]byte(nil), old.Bytes...)
	oldTableCopy := append([]uint32(nil), old.Table...)

	MakeCRCs()
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

// TestLoadCrcWarnsOnMissingFile pins NAI-20 Task 3 C2 (carried forward
// from the old TestMakeCrcWarnsOnMissingFile): loadCrc emits a slog.Warn
// on os.Stat failure and returns 0.
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
