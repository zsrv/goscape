package config

import (
	"bytes"
	"encoding/binary"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zsrv/goscape/pkg/io/filestream"
	"github.com/zsrv/goscape/pkg/io/jagfile"
	"github.com/zsrv/goscape/pkg/io/packet"
	"github.com/zsrv/goscape/pkg/unpack/unpacktest"
)

// ---- synthetic helpers ----

// buildConfigJag builds an in-memory jagfile containing <typ>.idx and <typ>.dat
// for the given entries.
// Each entry is a raw byte slice; idx encodes count+lengths, dat concatenates.
func buildConfigJag(typ string, entries [][]byte) (*jagfile.Jagfile, error) {
	// build idx packet: g2(count), then for each entry g2(len)
	idxBuf := make([]byte, 2+2*len(entries))
	binary.BigEndian.PutUint16(idxBuf[0:], uint16(len(entries)))
	for i, e := range entries {
		binary.BigEndian.PutUint16(idxBuf[2+2*i:], uint16(len(e)))
	}

	// build dat packet: g2(count) header + concatenated entries
	datBuf := make([]byte, 2)
	binary.BigEndian.PutUint16(datBuf, uint16(len(entries)))
	for _, e := range entries {
		datBuf = append(datBuf, e...)
	}

	jag := jagfile.NewEmptyJagfile(true)
	jag.Write(typ+".idx", packet.NewPacket(idxBuf))
	jag.Write(typ+".dat", packet.NewPacket(datBuf))
	return jag, nil
}

// writeJagToCache writes a jagfile's raw bytes into a fresh FileStream cache at
// cacheDir as archive 0, file fileID.
func writeJagToCache(t *testing.T, cacheDir string, fileID int, jag *jagfile.Jagfile) {
	t.Helper()
	dir := t.TempDir()
	// Save the jagfile to a temp path, read back bytes, write into filestream.
	tmpPath := filepath.Join(dir, "tmp.jag")
	if err := jag.Save(tmpPath); err != nil {
		t.Fatalf("jag.Save: %v", err)
	}
	data, err := os.ReadFile(tmpPath)
	if err != nil {
		t.Fatalf("read tmp jag: %v", err)
	}
	fs2, err := filestream.New(cacheDir, true, false)
	if err != nil {
		t.Fatalf("filestream.New: %v", err)
	}
	if !fs2.Write(0, fileID, data, 0) {
		t.Fatalf("filestream.Write failed")
	}
	fs2.Close()
}

// buildPackDir builds a packDir with client/config containing the given jag.
func buildPackDir(t *testing.T, jag *jagfile.Jagfile) string {
	t.Helper()
	packDir := t.TempDir()
	clientDir := filepath.Join(packDir, "client")
	if err := os.MkdirAll(clientDir, 0o755); err != nil {
		t.Fatalf("mkdir client: %v", err)
	}
	if err := jag.Save(filepath.Join(clientDir, "config")); err != nil {
		t.Fatalf("save pack config jag: %v", err)
	}
	return packDir
}

// ---- unit tests ----

// TestCompare_ExactMatch verifies that two identical config jags produce
// "npc" + "exact match" lines exactly, with no warnings.
func TestCompare_ExactMatch(t *testing.T) {
	entries := [][]byte{
		{0x01, 0x02, 0x03},
		{0xAA, 0xBB},
	}
	jag1, _ := buildConfigJag("npc", entries)
	jag2, _ := buildConfigJag("npc", entries)

	cacheDir := t.TempDir()
	writeJagToCache(t, cacheDir, 2, jag1)
	packDir := buildPackDir(t, jag2)

	var out bytes.Buffer
	if err := Compare(cacheDir, packDir, "npc", &out); err != nil {
		t.Fatalf("Compare: %v", err)
	}

	lines := strings.Split(strings.TrimRight(out.String(), "\n"), "\n")
	if len(lines) != 2 {
		t.Fatalf("want 2 lines, got %d: %q", len(lines), out.String())
	}
	if lines[0] != "npc" {
		t.Errorf("line 0: want %q got %q", "npc", lines[0])
	}
	if lines[1] != "exact match" {
		t.Errorf("line 1: want %q got %q", "exact match", lines[1])
	}
}

// TestCompare_SizeMismatch verifies the "different config sizes" warning.
func TestCompare_SizeMismatch(t *testing.T) {
	entries1 := [][]byte{{0x01}, {0x02}}
	entries2 := [][]byte{{0x01}, {0x02}, {0x03}}
	jag1, _ := buildConfigJag("npc", entries1)
	jag2, _ := buildConfigJag("npc", entries2)

	cacheDir := t.TempDir()
	writeJagToCache(t, cacheDir, 2, jag1)
	packDir := buildPackDir(t, jag2)

	var out bytes.Buffer
	if err := Compare(cacheDir, packDir, "npc", &out); err != nil {
		t.Fatalf("Compare: %v", err)
	}

	got := out.String()
	// TS Compare.ts:23: printWarning(`different config sizes, ${idx1.pos.length} != ${idx2.pos.length}`)
	if !strings.Contains(got, "different config sizes, 2 != 3") {
		t.Errorf("want size mismatch warning, got: %q", got)
	}
}

// TestCompare_LengthMismatch verifies the "length does not match" warning.
func TestCompare_LengthMismatch(t *testing.T) {
	// entry 0: same bytes; entry 1: different lengths
	entries1 := [][]byte{{0x01, 0x02}, {0xAA, 0xBB, 0xCC}}
	entries2 := [][]byte{{0x01, 0x02}, {0xAA, 0xBB}}
	jag1, _ := buildConfigJag("npc", entries1)
	jag2, _ := buildConfigJag("npc", entries2)

	cacheDir := t.TempDir()
	writeJagToCache(t, cacheDir, 2, jag1)
	packDir := buildPackDir(t, jag2)

	var out bytes.Buffer
	if err := Compare(cacheDir, packDir, "npc", &out); err != nil {
		t.Fatalf("Compare: %v", err)
	}

	got := out.String()
	// TS Compare.ts:33: printWarning(`${i}: length does not match, ${idx1.len[i]} != ${idx2.len[i]}`)
	if !strings.Contains(got, "1: length does not match, 3 != 2") {
		t.Errorf("want length mismatch warning for entry 1, got: %q", got)
	}
	// entry 0 should not produce a warning (same bytes → same CRC)
	if strings.Contains(got, "0:") {
		t.Errorf("unexpected warning for entry 0 in: %q", got)
	}
}

// TestCompare_CRCMismatch verifies the "crc does not match" warning.
func TestCompare_CRCMismatch(t *testing.T) {
	// same length, different bytes
	entries1 := [][]byte{{0x01, 0x02}, {0xAA, 0xBB}}
	entries2 := [][]byte{{0x01, 0x02}, {0xCC, 0xDD}} // entry 1 differs
	jag1, _ := buildConfigJag("npc", entries1)
	jag2, _ := buildConfigJag("npc", entries2)

	cacheDir := t.TempDir()
	writeJagToCache(t, cacheDir, 2, jag1)
	packDir := buildPackDir(t, jag2)

	var out bytes.Buffer
	if err := Compare(cacheDir, packDir, "npc", &out); err != nil {
		t.Fatalf("Compare: %v", err)
	}

	got := out.String()
	// TS Compare.ts:46: printWarning(`${i}: crc does not match`)
	if !strings.Contains(got, "1: crc does not match") {
		t.Errorf("want crc mismatch warning for entry 1, got: %q", got)
	}
	// entry 0 is identical — no warning expected
	if strings.Contains(got, "0: crc") {
		t.Errorf("unexpected crc warning for entry 0 in: %q", got)
	}
}

// ---- parity test ----

// TestCompareParity is the env-gated full parity test for Compare.
// It requires GOSCAPE_REF274_DIR to point at the engine directory of a
// Server274-ref checkout. Run with:
//
//	GOSCAPE_REF274_DIR=/path/to/Server274-ref/engine \
//	  GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache \
//	  go test ./pkg/unpack/config/ -run TestCompareParity -v -count=1 -timeout 600s
//
// Compare does not write any files — it is a read-only diff tool. The manifest
// therefore has no ADDED/MODIFIED/CACHE-* entries, only STDOUT-NORM.
func TestCompareParity(t *testing.T) {
	refRoot := unpacktest.RefDir(t)
	cacheDir := unpacktest.CacheDir(t)

	// Build a packDir containing data/pack/client/config from the reference engine.
	// We copy the file rather than referencing it directly (read-only source).
	srcConfig := filepath.Join(refRoot, "engine", "data", "pack", "client", "config")
	packDir := t.TempDir()
	clientDir := filepath.Join(packDir, "client")
	if err := os.MkdirAll(clientDir, 0o755); err != nil {
		t.Fatalf("mkdir packDir/client: %v", err)
	}
	dstConfig := filepath.Join(clientDir, "config")
	if err := copyRegFile(srcConfig, dstConfig); err != nil {
		t.Fatalf("copy client/config: %v", err)
	}

	var out bytes.Buffer
	if err := Compare(cacheDir, packDir, "npc", &out); err != nil {
		t.Fatalf("Compare: %v", err)
	}

	// Compare is read-only: no content or cache changes, no files written.
	cachePristine := filepath.Join(refRoot, "unpack-ref", "cache")
	r := unpacktest.Result{
		Content:      nil, // no content scratch involved
		Cache:        unpacktest.ChangedSet(t, cachePristine, cacheDir),
		Wrote:        nil, // no writes
		Stdout:       out.Bytes(),
		PostDir:      "",
		CachePostDir: cacheDir,
	}

	unpacktest.AssertManifest(t, refRoot, "compare", r)
}

// copyRegFile copies a regular file from src to dst, creating parent dirs.
func copyRegFile(src, dst string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(dst), fs.ModePerm); err != nil {
		return err
	}
	return os.WriteFile(dst, data, 0o644)
}
