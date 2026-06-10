package gziputil

import (
	"archive/zip"
	"bytes"
	"compress/gzip"
	"io"
	"os"
	"path/filepath"
	"testing"
)

// TestCompressGz_Fixtures verifies bit-exact output against the three pinned
// testdata pairs (tiny / mid / large) extracted from the reference corpus.
// These run without any environment variable and act as the permanent contract.
func TestCompressGz_Fixtures(t *testing.T) {
	fixtures := []struct {
		label   string
		rawFile string
		gzFile  string
	}{
		{"tiny", "testdata/tiny_1_2175.raw", "testdata/tiny_1_2175.gz"},
		{"mid", "testdata/mid_4_317.raw", "testdata/mid_4_317.gz"},
		{"large", "testdata/large_3_1.raw", "testdata/large_3_1.gz"},
	}
	for _, fx := range fixtures {
		t.Run(fx.label, func(t *testing.T) {
			raw, err := os.ReadFile(fx.rawFile)
			if err != nil {
				t.Fatalf("read raw: %v", err)
			}
			want, err := os.ReadFile(fx.gzFile)
			if err != nil {
				t.Fatalf("read gz: %v", err)
			}
			got := CompressGz(raw, 0, len(raw))
			if got == nil {
				t.Fatal("CompressGz returned nil")
			}
			if !bytes.Equal(got, want) {
				t.Errorf("got %d bytes, want %d", len(got), len(want))
				// Report first divergence
				for i := range min(len(got), len(want)) {
					if got[i] != want[i] {
						t.Errorf("first diff at byte %d: got 0x%02x want 0x%02x", i, got[i], want[i])
						break
					}
				}
			}
		})
	}
}

// TestCompressGz_EmptyInput pins the exact output for a zero-length input.
// Expected bytes were produced by the C oracle (cfztest /dev/null out 6) and
// then compared against CompressGz to confirm the Go implementation agrees
// (OS byte differs: C writes 0x03, Go zeroes it per TS GZip.ts — pinned as Go output).
// m4 — B6 quality review.
func TestCompressGz_EmptyInput(t *testing.T) {
	want := []byte{
		0x1f, 0x8b, // ID1, ID2
		0x08,                   // CM = Z_DEFLATED
		0x00,                   // FLG
		0x00, 0x00, 0x00, 0x00, // MTIME = 0
		0x00,       // XFL
		0x00,       // OS = 0 (zeroed per TS GZip.ts)
		0x03, 0x00, // deflate: BFINAL=1 empty block
		0x00, 0x00, 0x00, 0x00, // CRC32(empty) = 0
		0x00, 0x00, 0x00, 0x00, // ISIZE = 0
	}
	got := CompressGz([]byte{}, 0, 0)
	if len(got) != len(want) {
		t.Fatalf("empty input: got %d bytes, want %d: % x", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("empty input: byte[%d] = 0x%02x, want 0x%02x", i, got[i], want[i])
		}
	}
}

// TestCompressGz_RefCorpus is an env-gated full corpus check.
// Set GOSCAPE_REF245_DIR to the path of Server245.2-ref/engine to enable it.
// It iterates every entry in data/pack/ondemand.zip (strip last 2 bytes for gz)
// plus every file under data/pack/client/maps/, decompresses with stdlib,
// recompresses with CompressGz, and byte-compares.
func TestCompressGz_RefCorpus(t *testing.T) {
	refDir := os.Getenv("GOSCAPE_REF245_DIR")
	if refDir == "" {
		t.Skip("GOSCAPE_REF245_DIR not set; skipping corpus check")
	}

	totalOK := 0
	totalFail := 0
	var firstFails []string

	// --- ondemand.zip ---
	ondemandZip := filepath.Join(refDir, "data", "pack", "ondemand.zip")
	zr, err := zip.OpenReader(ondemandZip)
	if err != nil {
		t.Fatalf("open ondemand.zip: %v", err)
	}
	defer zr.Close()

	for _, entry := range zr.File {
		rc, err := entry.Open()
		if err != nil {
			t.Fatalf("open zip entry %s: %v", entry.Name, err)
		}
		entryData, err := io.ReadAll(rc)
		rc.Close()
		if err != nil {
			t.Fatalf("read zip entry %s: %v", entry.Name, err)
		}
		// The entry[:len-2] is the gzip stream; strip the 2-byte trailer.
		if len(entryData) < 2 {
			continue
		}
		refGz := entryData[:len(entryData)-2]

		raw, err := gzip.NewReader(bytes.NewReader(refGz))
		if err != nil {
			t.Fatalf("gzip.NewReader for %s: %v", entry.Name, err)
		}
		rawData, err := io.ReadAll(raw)
		raw.Close()
		if err != nil {
			t.Fatalf("decompress %s: %v", entry.Name, err)
		}

		got := CompressGz(rawData, 0, len(rawData))
		if bytes.Equal(got, refGz) {
			totalOK++
		} else {
			totalFail++
			if len(firstFails) < 10 {
				firstFails = append(firstFails, entry.Name)
			}
		}
	}

	// --- client/maps/ ---
	mapsDir := filepath.Join(refDir, "data", "pack", "client", "maps")
	mapEntries, err := os.ReadDir(mapsDir)
	if err != nil && !os.IsNotExist(err) {
		t.Fatalf("readdir maps: %v", err)
	}
	for _, me := range mapEntries {
		if me.IsDir() {
			continue
		}
		path := filepath.Join(mapsDir, me.Name())
		refGzData, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read map %s: %v", me.Name(), err)
		}
		raw, err := gzip.NewReader(bytes.NewReader(refGzData))
		if err != nil {
			continue // not a gz, skip
		}
		rawData, err := io.ReadAll(raw)
		raw.Close()
		if err != nil {
			continue
		}
		got := CompressGz(rawData, 0, len(rawData))
		if bytes.Equal(got, refGzData) {
			totalOK++
		} else {
			totalFail++
			if len(firstFails) < 10 {
				firstFails = append(firstFails, "maps/"+me.Name())
			}
		}
	}

	t.Logf("corpus: %d OK, %d FAIL", totalOK, totalFail)
	if totalFail > 0 {
		t.Errorf("corpus: %d byte-mismatches; first failures: %v", totalFail, firstFails)
	}
}
