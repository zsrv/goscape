package midi

import (
	"bytes"
	"compress/gzip"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zsrv/goscape/pkg/io/filestream"
	"github.com/zsrv/goscape/pkg/io/jagfile"
	"github.com/zsrv/goscape/pkg/io/packet"
)

// gzipBytes compresses data using gzip and returns the compressed bytes.
// This is needed because Unpack calls cache.Read(3, i, true) — the
// decompress=true path — so test data must be pre-compressed.
func gzipBytes(t *testing.T, data []byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	w := gzip.NewWriter(&buf)
	if _, err := w.Write(data); err != nil {
		t.Fatalf("gzip write: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("gzip close: %v", err)
	}
	return buf.Bytes()
}

// buildTestCache creates a minimal FileStream cache at cacheDir containing:
//   - archive 0 / file 5: a versionlist jagfile with a midi_index member
//     whose bytes are midiIndexBytes (one g1 per midi).
//   - archive 3 / file i: gzip(midiData[i]) for each i (nil entries are skipped,
//     leaving those ids with no data — triggers Missing warning).
//
// Data is gzip-compressed because Unpack calls cache.Read(3, i, true) which
// decompresses archive 3 entries via gunzip.
func buildTestCache(t *testing.T, cacheDir string, midiIndexBytes []byte, midiData [][]byte) {
	t.Helper()

	// Build the versionlist jagfile with a midi_index member.
	vl := jagfile.NewEmptyJagfile(false)
	vl.Write("midi_index", packet.NewPacket(midiIndexBytes))

	// Save the jagfile to a temp file, read back bytes, inject into cache.
	tmp := filepath.Join(t.TempDir(), "vl.jag")
	if err := vl.Save(tmp); err != nil {
		t.Fatalf("vl.Save: %v", err)
	}
	vlBytes, err := os.ReadFile(tmp)
	if err != nil {
		t.Fatalf("readFile vl: %v", err)
	}

	fs2 := filestream.New(cacheDir, true, false)
	if !fs2.Write(0, 5, vlBytes, 0) {
		t.Fatal("write versionlist to cache failed")
	}

	// Write midi data into archive 3 (gzip-compressed — matches decompress=true).
	for i, data := range midiData {
		if data == nil {
			continue
		}
		compressed := gzipBytes(t, data)
		if !fs2.Write(3, i, compressed, 0) {
			t.Fatalf("write midi %d to cache failed", i)
		}
	}
	fs2.Close()
}

// TestUnpack_SongsAndJingles verifies that files are routed correctly by the
// jingle flag: flag=0 → songs/, flag=1 → jingles/.
func TestUnpack_SongsAndJingles(t *testing.T) {
	cacheDir := t.TempDir()
	srcDir := t.TempDir()

	// Two midis: id=0 song (flag=0), id=1 jingle (flag=1).
	midiIndex := []byte{0, 1}
	data0 := []byte("MThd-song")
	data1 := []byte("MThd-jingle")
	buildTestCache(t, cacheDir, midiIndex, [][]byte{data0, data1})

	var out bytes.Buffer
	if err := Unpack(Options{CacheDir: cacheDir, SrcDir: srcDir, Out: &out}); err != nil {
		t.Fatalf("Unpack: %v", err)
	}

	// Song should be in songs/, jingle in jingles/.
	songPath := filepath.Join(srcDir, "songs", "midi_0.mid")
	jinglePath := filepath.Join(srcDir, "jingles", "midi_1.mid")

	if got, err := os.ReadFile(songPath); err != nil || !bytes.Equal(got, data0) {
		t.Errorf("songs/midi_0.mid: got %q err %v, want %q", got, err, data0)
	}
	if got, err := os.ReadFile(jinglePath); err != nil || !bytes.Equal(got, data1) {
		t.Errorf("jingles/midi_1.mid: got %q err %v, want %q", got, data1, data1)
	}

	// No warnings expected.
	if out.Len() != 0 {
		t.Errorf("unexpected stdout: %q", out.String())
	}
}

// TestUnpack_MissingDataWarning verifies that a missing cache entry emits the
// exact warning "Missing midi id=<i>" and that g1() is consumed before the check
// (i.e., the loop does not desync).
func TestUnpack_MissingDataWarning(t *testing.T) {
	cacheDir := t.TempDir()
	srcDir := t.TempDir()

	// Three midis: id=0 present, id=1 missing, id=2 present.
	// jingle flags all 0 (songs).
	midiIndex := []byte{0, 0, 0}
	data0 := []byte("MThd-0")
	data2 := []byte("MThd-2")
	// Pass nil for id=1 to leave it absent.
	buildTestCache(t, cacheDir, midiIndex, [][]byte{data0, nil, data2})

	var out bytes.Buffer
	if err := Unpack(Options{CacheDir: cacheDir, SrcDir: srcDir, Out: &out}); err != nil {
		t.Fatalf("Unpack: %v", err)
	}

	// Exactly one warning for id=1.
	warnLines := strings.Split(strings.TrimRight(out.String(), "\n"), "\n")
	if len(warnLines) != 1 || warnLines[0] != "Missing midi id=1" {
		t.Errorf("warnings: got %v, want [Missing midi id=1]", warnLines)
	}

	// id=0 and id=2 must have been written correctly.
	if got, err := os.ReadFile(filepath.Join(srcDir, "songs", "midi_0.mid")); err != nil || !bytes.Equal(got, data0) {
		t.Errorf("songs/midi_0.mid: got %q err %v", got, err)
	}
	if got, err := os.ReadFile(filepath.Join(srcDir, "songs", "midi_2.mid")); err != nil || !bytes.Equal(got, data2) {
		t.Errorf("songs/midi_2.mid: got %q err %v", got, err)
	}
}

// TestUnpack_NameFallback verifies the "midi_<i>" fallback name when the pack
// registry has no entry for the id, and that a pre-registered name is used when
// available (populated via midi.pack written before the run).
func TestUnpack_NameFallback(t *testing.T) {
	cacheDir := t.TempDir()
	srcDir := t.TempDir()

	// Seed midi.pack with id=0 named "my_song".
	packDir := filepath.Join(srcDir, "pack")
	if err := os.MkdirAll(packDir, 0o755); err != nil {
		t.Fatalf("mkdir pack: %v", err)
	}
	if err := os.WriteFile(filepath.Join(packDir, "midi.pack"), []byte("0=my_song\n"), 0o644); err != nil {
		t.Fatalf("write midi.pack: %v", err)
	}

	// id=0 has a name; id=1 does not (fallback).
	midiIndex := []byte{0, 0}
	buildTestCache(t, cacheDir, midiIndex, [][]byte{[]byte("data0"), []byte("data1")})

	var out bytes.Buffer
	if err := Unpack(Options{CacheDir: cacheDir, SrcDir: srcDir, Out: &out}); err != nil {
		t.Fatalf("Unpack: %v", err)
	}

	// id=0 uses registered name "my_song".
	if _, err := os.Stat(filepath.Join(srcDir, "songs", "my_song.mid")); err != nil {
		t.Errorf("songs/my_song.mid not found: %v", err)
	}
	// id=1 falls back to "midi_1".
	if _, err := os.Stat(filepath.Join(srcDir, "songs", "midi_1.mid")); err != nil {
		t.Errorf("songs/midi_1.mid not found: %v", err)
	}
}

// TestUnpack_NilOut verifies that passing Out=nil does not panic when a
// warning would otherwise be emitted.
func TestUnpack_NilOut(t *testing.T) {
	cacheDir := t.TempDir()
	srcDir := t.TempDir()

	// id=0 missing.
	midiIndex := []byte{0}
	buildTestCache(t, cacheDir, midiIndex, [][]byte{nil})

	if err := Unpack(Options{CacheDir: cacheDir, SrcDir: srcDir, Out: nil}); err != nil {
		t.Fatalf("Unpack with nil Out: %v", err)
	}
}
