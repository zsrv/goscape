// Package audio tests for PackMidi at rev-244: per-file gzip writes to
// FileStream archive 3.
// The 225 tests (TestPackMidi_CompressesNew, TestPackMidi_SkipsExisting,
// TestPackMidi_MissingSrcReturnsNil) are deleted; they pinned the 225
// bzip2 client/jingles + client/songs dir contract removed upstream.
package audio

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/zsrv/goscape/pkg/io/filestream"
	"github.com/zsrv/goscape/pkg/io/gziputil"
	"github.com/zsrv/goscape/pkg/pack"
)

// newMidiCache creates a fresh FileStream in a temp subdir.
func newMidiCache(t *testing.T, dir string) *filestream.FileStream {
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

// seedMidiPack writes a minimal midi.pack file.
func seedMidiPack(t *testing.T, packDir, content string) {
	t.Helper()
	if err := os.MkdirAll(packDir, 0o755); err != nil {
		t.Fatalf("MkdirAll packDir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(packDir, "midi.pack"), []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile midi.pack: %v", err)
	}
}

// TestPackMidi_JingleWritesToArchive3 verifies that a non-empty .mid file in
// jingles/ is gzip-compressed and written to cache archive 3 at the midi id.
// TS midi/pack.ts:11-19 @ 9aadcec4.
func TestPackMidi_JingleWritesToArchive3(t *testing.T) {
	tmp := t.TempDir()
	src := filepath.Join(tmp, "src")
	jinglesDir := filepath.Join(src, "jingles")
	if err := os.MkdirAll(jinglesDir, 0o755); err != nil {
		t.Fatal(err)
	}

	seedMidiPack(t, filepath.Join(src, "pack"), "0=tune1\n")

	midiBytes := []byte{0x4D, 0x54, 0x68, 0x64} // "MThd" MIDI magic
	if err := os.WriteFile(filepath.Join(jinglesDir, "tune1.mid"), midiBytes, 0o644); err != nil {
		t.Fatal(err)
	}

	reg := &pack.Registry{SrcDir: src}
	cache := newMidiCache(t, tmp)
	defer cache.Close()

	if err := PackMidi(reg, src, cache); err != nil {
		t.Fatalf("PackMidi: %v", err)
	}

	if !cache.Has(3, 0) {
		t.Fatal("cache.Has(3, 0) = false, want true")
	}

	got := cache.Read(3, 0, false)
	if got == nil {
		t.Fatal("cache.Read(3, 0) = nil")
	}
	if len(got) < 2 {
		t.Fatal("stored data too short")
	}
	// Strip 2-byte version trailer then decompress.
	payload := got[:len(got)-2]
	back := gziputil.DecompressGz(payload, 0, len(payload))
	if back == nil {
		t.Fatal("DecompressGz returned nil")
	}
	if !bytes.Equal(back, midiBytes) {
		t.Errorf("round-trip = %v, want %v", back, midiBytes)
	}
}

// TestPackMidi_SongWritesToArchive3 verifies that a non-empty .mid file in
// songs/ is gzip-compressed and written to cache archive 3 at the midi id.
// TS midi/pack.ts:11 spread `[...jingles, ...songs]` @ 9aadcec4.
func TestPackMidi_SongWritesToArchive3(t *testing.T) {
	tmp := t.TempDir()
	src := filepath.Join(tmp, "src")
	songsDir := filepath.Join(src, "songs")
	if err := os.MkdirAll(songsDir, 0o755); err != nil {
		t.Fatal(err)
	}

	seedMidiPack(t, filepath.Join(src, "pack"), "0=theme\n")

	midiBytes := []byte{0x01, 0x02, 0x03}
	if err := os.WriteFile(filepath.Join(songsDir, "theme.mid"), midiBytes, 0o644); err != nil {
		t.Fatal(err)
	}

	reg := &pack.Registry{SrcDir: src}
	cache := newMidiCache(t, tmp)
	defer cache.Close()

	if err := PackMidi(reg, src, cache); err != nil {
		t.Fatalf("PackMidi: %v", err)
	}

	if !cache.Has(3, 0) {
		t.Fatal("cache.Has(3, 0) = false, want true")
	}
}

// TestPackMidi_EmptyFileSkipped verifies that a zero-length .mid file is NOT
// written to the cache. TS midi/pack.ts:16 `if (data.length)` guard.
func TestPackMidi_EmptyFileSkipped(t *testing.T) {
	tmp := t.TempDir()
	src := filepath.Join(tmp, "src")
	jinglesDir := filepath.Join(src, "jingles")
	if err := os.MkdirAll(jinglesDir, 0o755); err != nil {
		t.Fatal(err)
	}

	seedMidiPack(t, filepath.Join(src, "pack"), "0=tune1\n")

	// Zero-length file.
	if err := os.WriteFile(filepath.Join(jinglesDir, "tune1.mid"), []byte{}, 0o644); err != nil {
		t.Fatal(err)
	}

	reg := &pack.Registry{SrcDir: src}
	cache := newMidiCache(t, tmp)
	defer cache.Close()

	if err := PackMidi(reg, src, cache); err != nil {
		t.Fatalf("PackMidi: %v", err)
	}

	if cache.Has(3, 0) {
		t.Error("cache.Has(3, 0) = true for empty .mid, want false")
	}
}

// TestPackMidi_NilCacheNoOp verifies that a nil cache causes the stage to be
// a no-op (no error). T15 comment pattern.
func TestPackMidi_NilCacheNoOp(t *testing.T) {
	tmp := t.TempDir()
	src := filepath.Join(tmp, "src")
	if err := os.MkdirAll(filepath.Join(src, "pack"), 0o755); err != nil {
		t.Fatal(err)
	}
	seedMidiPack(t, filepath.Join(src, "pack"), "0=tune1\n")

	reg := &pack.Registry{SrcDir: src}
	if err := PackMidi(reg, src, nil); err != nil {
		t.Errorf("PackMidi with nil cache: %v, want nil", err)
	}
}

// TestPackMidi_MissingSrcNoOp verifies that missing jingles/songs dirs
// produce no error (NAI-192-D-NO-SRC-NO-OP mirror).
func TestPackMidi_MissingSrcNoOp(t *testing.T) {
	tmp := t.TempDir()
	src := filepath.Join(tmp, "src")
	if err := os.MkdirAll(filepath.Join(src, "pack"), 0o755); err != nil {
		t.Fatal(err)
	}
	seedMidiPack(t, filepath.Join(src, "pack"), "")

	reg := &pack.Registry{SrcDir: src}
	cache := newMidiCache(t, tmp)
	defer cache.Close()

	if err := PackMidi(reg, src, cache); err != nil {
		t.Errorf("PackMidi with missing src: %v, want nil", err)
	}
}

// TestPackMidi_No225BzipDirs verifies that no client/jingles or client/songs
// directories are produced (225 bzip2 format removed upstream at rev-244).
func TestPackMidi_No225BzipDirs(t *testing.T) {
	tmp := t.TempDir()
	src := filepath.Join(tmp, "src")
	jinglesDir := filepath.Join(src, "jingles")
	if err := os.MkdirAll(jinglesDir, 0o755); err != nil {
		t.Fatal(err)
	}
	seedMidiPack(t, filepath.Join(src, "pack"), "0=tune1\n")

	midiBytes := []byte{0x4D, 0x54, 0x68, 0x64}
	if err := os.WriteFile(filepath.Join(jinglesDir, "tune1.mid"), midiBytes, 0o644); err != nil {
		t.Fatal(err)
	}

	outDir := filepath.Join(tmp, "out")
	reg := &pack.Registry{SrcDir: src}
	cache := newMidiCache(t, tmp)
	defer cache.Close()

	if err := PackMidi(reg, src, cache); err != nil {
		t.Fatalf("PackMidi: %v", err)
	}

	// 225 artifacts must NOT exist.
	for _, sub := range []string{"jingles", "songs"} {
		p := filepath.Join(outDir, "client", sub)
		if _, err := os.Stat(p); err == nil {
			t.Errorf("client/%s dir exists; 225 bzip2 dirs should be removed at rev-244", sub)
		}
	}
}
