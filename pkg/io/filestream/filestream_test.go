package filestream

import (
	"bytes"
	"compress/gzip"
	"os"
	"path/filepath"
	"testing"
)

// TS FileStream.ts:131-196 (write) + 44-123 (read): single-sector round-trip.
func TestWriteReadRoundTripSingleSector(t *testing.T) {
	fs, err := New(t.TempDir(), true, false)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer fs.Close()

	data := []byte("hello cache")
	if !fs.Write(1, 0, data, 0) {
		t.Fatal("Write returned false")
	}
	got := fs.Read(1, 0, false)
	if !bytes.Equal(got, data) {
		t.Fatalf("Read = %q, want %q", got, data)
	}
}

// TS FileStream.ts:78-104: multi-sector chaining for payloads > 512 bytes.
func TestWriteReadRoundTripMultiSector(t *testing.T) {
	fs, err := New(t.TempDir(), true, false)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer fs.Close()

	data := make([]byte, 1300) // 3 sectors
	for i := range data {
		data[i] = byte(i % 251)
	}
	if !fs.Write(2, 5, data, 0) {
		t.Fatal("Write returned false")
	}
	if got := fs.Read(2, 5, false); !bytes.Equal(got, data) {
		t.Fatalf("multi-sector read mismatch: len=%d want %d", len(got), len(data))
	}
}

// TS FileStream.ts:140-147: version != 0 appends a 2-byte big-endian trailer.
func TestWriteVersionTrailer(t *testing.T) {
	fs, err := New(t.TempDir(), true, false)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer fs.Close()

	if !fs.Write(1, 0, []byte{0xAA}, 0x1234) {
		t.Fatal("Write returned false")
	}
	got := fs.Read(1, 0, false)
	want := []byte{0xAA, 0x12, 0x34}
	if !bytes.Equal(got, want) {
		t.Fatalf("Read = %x, want %x", got, want)
	}
}

// TS FileStream.ts:35-41: count = idx length / 6.
func TestCount(t *testing.T) {
	fs, err := New(t.TempDir(), true, false)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer fs.Close()

	if n := fs.Count(1); n != 0 {
		t.Fatalf("Count(1) = %d, want 0", n)
	}
	fs.Write(1, 0, []byte{1}, 0)
	fs.Write(1, 1, []byte{2}, 0)
	if n := fs.Count(1); n != 2 {
		t.Fatalf("Count(1) = %d, want 2", n)
	}
	// TS: out-of-range archive returns 0.
	if n := fs.Count(9); n != 0 {
		t.Fatalf("Count(9) = %d, want 0", n)
	}
}

// TS FileStream.ts:198-225 (has) + 49-59 (read bounds): bounds and missing files.
func TestHasAndReadBounds(t *testing.T) {
	fs, err := New(t.TempDir(), true, false)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer fs.Close()

	fs.Write(0, 0, []byte{7}, 0)
	if !fs.Has(0, 0) {
		t.Fatal("Has(0,0) = false after write")
	}
	if fs.Has(0, 1) || fs.Has(-1, 0) || fs.Has(5, 0) || fs.Has(0, -1) {
		t.Fatal("Has returned true for an out-of-range entry")
	}
	if fs.Read(0, 1, false) != nil || fs.Read(-1, 0, false) != nil {
		t.Fatal("Read returned data for an out-of-range entry")
	}
}

// TS FileStream.ts:135: write to an out-of-range archive returns false
// (TS: idx[5] is undefined -> !undefined -> false). Regression: this
// panicked when the guard used > instead of >=.
func TestWriteOutOfRangeArchive(t *testing.T) {
	fs, err := New(t.TempDir(), true, false)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer fs.Close()

	if fs.Write(5, 0, []byte{1}, 0) {
		t.Fatal("Write(5,...) must return false")
	}
	if fs.Write(-1, 0, []byte{1}, 0) {
		t.Fatal("Write(-1,...) must return false")
	}
}

// TS FileStream.ts:107-122: decompress=true gunzips for archive != 0,
// returns raw for archive 0.
func TestReadDecompress(t *testing.T) {
	fs, err := New(t.TempDir(), true, false)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer fs.Close()

	plain := []byte("the payload to be gzipped for archive one")
	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	zw.Write(plain)
	zw.Close()

	fs.Write(1, 0, buf.Bytes(), 0)
	if got := fs.Read(1, 0, true); !bytes.Equal(got, plain) {
		t.Fatalf("decompressed read mismatch: %q", got)
	}

	fs.Write(0, 0, buf.Bytes(), 0)
	if got := fs.Read(0, 0, true); !bytes.Equal(got, buf.Bytes()) {
		t.Fatal("archive 0 must return raw bytes even with decompress=true")
	}
}

// Entries written WITH a version store gzip + a 2-byte trailer (every 244
// packer archive 1-4 entry). node gunzipSync decompresses the member and
// ignores the trailing bytes; Read(decompress=true) must do the same
// (Multistream(false)), not fail on them. A corrupt member still returns nil
// (gunzipSync throws).
func TestReadDecompressVersionTrailer(t *testing.T) {
	fs, err := New(t.TempDir(), true, false)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer fs.Close()

	plain := []byte("model bytes behind a version trailer")
	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	zw.Write(plain)
	zw.Close()

	if !fs.Write(1, 0, buf.Bytes(), 1) {
		t.Fatal("versioned Write returned false")
	}
	if got := fs.Read(1, 0, true); !bytes.Equal(got, plain) {
		t.Fatalf("versioned decompressed read mismatch: %q", got)
	}

	// Truncated member (CRC/ISIZE cut off) — node gunzipSync throws → nil.
	trunc := buf.Bytes()[:buf.Len()-6]
	fs.Write(1, 1, trunc, 0)
	if got := fs.Read(1, 1, true); got != nil {
		t.Fatalf("truncated member must read nil, got %d bytes", len(got))
	}
}

// TS FileStream.ts:14-32: createNew=false on an existing dir preserves content;
// the constructor creates dat + idx0..idx4 when missing.
func TestPersistenceAcrossOpens(t *testing.T) {
	dir := t.TempDir()
	fs1, err := New(dir, true, false)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	fs1.Write(3, 2, []byte("persisted"), 0)
	fs1.Close()

	for i := 0; i <= 4; i++ {
		if _, err := os.Stat(filepath.Join(dir, "main_file_cache.idx"+string(rune('0'+i)))); err != nil {
			t.Fatalf("idx%d missing: %v", i, err)
		}
	}

	fs2, err := New(dir, false, true) // read-only reopen
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer fs2.Close()
	if got := fs2.Read(3, 2, false); !bytes.Equal(got, []byte("persisted")) {
		t.Fatalf("reopen read = %q", got)
	}
}

// TS FileStream.ts:56-58 + 106-112: packed[] caches raw reads unless
// DiscardPacked; a second Read returns the cached slice.
func TestPackedCache(t *testing.T) {
	dir := t.TempDir()
	fs, err := New(dir, true, false)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer fs.Close()

	fs.Write(1, 0, []byte("cache me"), 0)
	first := fs.Read(1, 0, false)
	// Overwrite the entry behind the cache's back; the cached copy must win.
	fs2, err := New(dir, false, false)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	fs2.Write(1, 0, []byte("OVERWROTE"), 0)
	fs2.Close()
	second := fs.Read(1, 0, false)
	if !bytes.Equal(first, second) {
		t.Fatal("second Read did not come from the packed cache")
	}
}
