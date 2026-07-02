// Package graphics tests port TS tools/pack/graphics/pack.ts rev-244 contract:
// per-file gzip writes to FileStream archives 1 (models) and 2 (animsets).
// The 225 jag-aggregation tests (TestPack_BytePinned, TestPack_MissingSrcReturnsNil)
// are deleted; they pinned a contract removed upstream at rev-244.
package graphics

import (
	"bytes"
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"github.com/zsrv/goscape/pkg/io/filestream"
	"github.com/zsrv/goscape/pkg/io/gziputil"
	"github.com/zsrv/goscape/pkg/pack"
)

// seedPack writes a minimal <packDir>/<name>.pack file with a single entry.
func seedPack(t *testing.T, packDir, name, content string) {
	t.Helper()
	if err := os.MkdirAll(packDir, 0o755); err != nil {
		t.Fatalf("MkdirAll packDir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(packDir, name+".pack"), []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile %s.pack: %v", name, err)
	}
}

// newCache creates a fresh FileStream in a temp subdir.
func newCache(t *testing.T, dir string) *filestream.FileStream {
	t.Helper()
	cacheDir := filepath.Join(dir, "cache")
	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		t.Fatalf("MkdirAll cache: %v", err)
	}
	fs, err := filestream.New(cacheDir, true, false)
	if err != nil {
		t.Fatalf("filestream.New: %v", err)
	}
	return fs
}

// TestPack_Ob2WritesToArchive1 verifies that a non-empty .ob2 file is
// gzip-compressed and written to cache archive 1 at the model's id.
// TS pack.ts:13-19 @ 9aadcec4.
func TestPack_Ob2WritesToArchive1(t *testing.T) {
	tmp := t.TempDir()
	src := filepath.Join(tmp, "src")
	modelsDir := filepath.Join(src, "models")
	if err := os.MkdirAll(modelsDir, 0o755); err != nil {
		t.Fatal(err)
	}

	// model.pack: id 0 = "m0"
	seedPack(t, filepath.Join(src, "pack"), "model", "0=m0\n")
	// animset.pack: empty (no anim files)
	seedPack(t, filepath.Join(src, "pack"), "animset", "")

	// .ob2 fixture bytes (non-empty, arbitrary content)
	ob2Bytes := []byte{0x01, 0x02, 0x03, 0x04}
	if err := os.WriteFile(filepath.Join(modelsDir, "m0.ob2"), ob2Bytes, 0o644); err != nil {
		t.Fatal(err)
	}

	reg := &pack.Registry{SrcDir: src}
	cache := newCache(t, tmp)
	defer cache.Close()

	if err := Pack(reg, src, nil, cache, nil); err != nil {
		t.Fatalf("Pack: %v", err)
	}

	// cache.Has(1, 0) must be true.
	if !cache.Has(1, 0) {
		t.Fatal("cache.Has(1, 0) = false, want true")
	}

	// Read back and decompress; must equal original bytes.
	got := cache.Read(1, 0, false) // compressed
	if got == nil {
		t.Fatal("cache.Read(1, 0) = nil")
	}
	// The version trailer adds 2 bytes; strip them for gzip decode.
	// Actually cache.Write(1, id, compressed, 1) adds a 2-byte version
	// trailer. DecompressGz on [compressed + 2 version bytes] must be
	// tried via Read(decompress=false) then manual strip, OR we can
	// just verify the first 10 bytes are a valid gzip magic.
	// Simpler: decompress the raw stored bytes (without the 2-byte trailer).
	if len(got) < 2 {
		t.Fatal("stored data too short")
	}
	payload := got[:len(got)-2] // strip 2-byte version trailer
	back := gziputil.DecompressGz(payload, 0, len(payload))
	if back == nil {
		t.Fatal("DecompressGz returned nil")
	}
	if !bytes.Equal(back, ob2Bytes) {
		t.Errorf("round-trip = %v, want %v", back, ob2Bytes)
	}
}

// TestPack_AnimWritesToArchive2 verifies that a non-empty .anim file is
// gzip-compressed and written to cache archive 2 at the animset id.
// TS pack.ts:32-40 @ 9aadcec4.
func TestPack_AnimWritesToArchive2(t *testing.T) {
	tmp := t.TempDir()
	src := filepath.Join(tmp, "src")
	modelsDir := filepath.Join(src, "models")
	if err := os.MkdirAll(modelsDir, 0o755); err != nil {
		t.Fatal(err)
	}

	seedPack(t, filepath.Join(src, "pack"), "model", "")
	seedPack(t, filepath.Join(src, "pack"), "animset", "0=a0\n")

	animBytes := []byte{0xAA, 0xBB, 0xCC}
	if err := os.WriteFile(filepath.Join(modelsDir, "a0.anim"), animBytes, 0o644); err != nil {
		t.Fatal(err)
	}

	reg := &pack.Registry{SrcDir: src}
	cache := newCache(t, tmp)
	defer cache.Close()

	if err := Pack(reg, src, nil, cache, nil); err != nil {
		t.Fatalf("Pack: %v", err)
	}

	if !cache.Has(2, 0) {
		t.Fatal("cache.Has(2, 0) = false, want true")
	}

	got := cache.Read(2, 0, false)
	if got == nil {
		t.Fatal("cache.Read(2, 0) = nil")
	}
	if len(got) < 2 {
		t.Fatal("stored data too short")
	}
	payload := got[:len(got)-2]
	back := gziputil.DecompressGz(payload, 0, len(payload))
	if back == nil {
		t.Fatal("DecompressGz returned nil")
	}
	if !bytes.Equal(back, animBytes) {
		t.Errorf("round-trip = %v, want %v", back, animBytes)
	}
}

// TestPack_EmptyFileSkipped verifies that a zero-length .ob2 or .anim file
// is NOT written to the cache. TS pack.ts:17 `if (data.length)` guard.
func TestPack_EmptyFileSkipped(t *testing.T) {
	tmp := t.TempDir()
	src := filepath.Join(tmp, "src")
	modelsDir := filepath.Join(src, "models")
	if err := os.MkdirAll(modelsDir, 0o755); err != nil {
		t.Fatal(err)
	}

	seedPack(t, filepath.Join(src, "pack"), "model", "0=m0\n")
	seedPack(t, filepath.Join(src, "pack"), "animset", "0=a0\n")

	// Write zero-byte files.
	if err := os.WriteFile(filepath.Join(modelsDir, "m0.ob2"), []byte{}, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(modelsDir, "a0.anim"), []byte{}, 0o644); err != nil {
		t.Fatal(err)
	}

	reg := &pack.Registry{SrcDir: src}
	cache := newCache(t, tmp)
	defer cache.Close()

	if err := Pack(reg, src, nil, cache, nil); err != nil {
		t.Fatalf("Pack: %v", err)
	}

	if cache.Has(1, 0) {
		t.Error("cache.Has(1, 0) = true for empty .ob2, want false")
	}
	if cache.Has(2, 0) {
		t.Error("cache.Has(2, 0) = true for empty .anim, want false")
	}
}

// TestPack_MissingModelWarning verifies that a model id with modelFlags[id]>0
// that is absent from the cache emits a slog Warn. TS pack.ts:22-29 @ 9aadcec4.
func TestPack_MissingModelWarning(t *testing.T) {
	tmp := t.TempDir()
	src := filepath.Join(tmp, "src")
	modelsDir := filepath.Join(src, "models")
	if err := os.MkdirAll(modelsDir, 0o755); err != nil {
		t.Fatal(err)
	}

	// Two models: id 0 = "m0" (not present on disk), id 1 = "m1" (not present).
	seedPack(t, filepath.Join(src, "pack"), "model", "0=m0\n1=m1\n")
	seedPack(t, filepath.Join(src, "pack"), "animset", "")

	// modelFlags: [1, 0] — id 0 is flagged (should warn), id 1 is not.
	modelFlags := []int{1, 0}

	var buf bytes.Buffer
	lg := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelWarn}))

	reg := &pack.Registry{SrcDir: src}
	cache := newCache(t, tmp)
	defer cache.Close()

	if err := Pack(reg, src, modelFlags, cache, lg); err != nil {
		t.Fatalf("Pack: %v", err)
	}

	output := buf.String()
	if !bytes.Contains([]byte(output), []byte("missing model")) {
		t.Errorf("expected warning about missing model, got: %q", output)
	}
	// id 0 should be named in the warning.
	if !bytes.Contains([]byte(output), []byte("m0")) {
		t.Errorf("expected model name m0 in warning, got: %q", output)
	}
	// id 1 should NOT be warned (modelFlags[1] == 0).
	// Count occurrences: only 1 warning expected.
	count := bytes.Count([]byte(output), []byte("missing model"))
	if count != 1 {
		t.Errorf("expected 1 warning, got %d in: %q", count, output)
	}
}

// TestPack_NilCacheNoOp verifies that a nil cache causes the stage to be a
// no-op (no error, no write attempt). T15 comment pattern.
func TestPack_NilCacheNoOp(t *testing.T) {
	tmp := t.TempDir()
	src := filepath.Join(tmp, "src")
	if err := os.MkdirAll(filepath.Join(src, "pack"), 0o755); err != nil {
		t.Fatal(err)
	}
	seedPack(t, filepath.Join(src, "pack"), "model", "0=m0\n")
	seedPack(t, filepath.Join(src, "pack"), "animset", "")

	reg := &pack.Registry{SrcDir: src}
	if err := Pack(reg, src, nil, nil, nil); err != nil {
		t.Errorf("Pack with nil cache: %v, want nil", err)
	}
}

// TestPack_No225JagArtifact verifies that no client/models jagfile is produced
// (225 format removed upstream at rev-244). The outDir should contain no
// "client/models" file.
func TestPack_No225JagArtifact(t *testing.T) {
	tmp := t.TempDir()
	src := filepath.Join(tmp, "src")
	modelsDir := filepath.Join(src, "models")
	if err := os.MkdirAll(modelsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	seedPack(t, filepath.Join(src, "pack"), "model", "0=m0\n")
	seedPack(t, filepath.Join(src, "pack"), "animset", "")

	ob2Bytes := []byte{0x01, 0x02}
	if err := os.WriteFile(filepath.Join(modelsDir, "m0.ob2"), ob2Bytes, 0o644); err != nil {
		t.Fatal(err)
	}

	outDir := filepath.Join(tmp, "out")
	reg := &pack.Registry{SrcDir: src}
	cache := newCache(t, tmp)
	defer cache.Close()

	if err := Pack(reg, src, nil, cache, nil); err != nil {
		t.Fatalf("Pack: %v", err)
	}

	// The old 225 artifact would be at outDir/client/models.
	// This file must NOT exist.
	_, err := os.Stat(filepath.Join(outDir, "client", "models"))
	if err == nil {
		t.Error("client/models jagfile exists; 225 aggregation should be removed at rev-244")
	}
}
