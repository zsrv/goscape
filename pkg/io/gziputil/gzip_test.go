package gziputil

import (
	"bytes"
	"testing"
)

// TS GZip.ts:3-18: round-trip + the data[9]=0 OS-byte pin.
func TestCompressGzZeroesOSByte(t *testing.T) {
	src := []byte("determinism matters for pack byte-parity")
	gz := CompressGz(src, 0, len(src))
	if gz == nil {
		t.Fatal("CompressGz returned nil")
	}
	if gz[9] != 0 {
		t.Fatalf("gz[9] = %d, want 0 (OS byte must be zeroed)", gz[9])
	}
	back := DecompressGz(gz, 0, len(gz))
	if !bytes.Equal(back, src) {
		t.Fatalf("round-trip = %q, want %q", back, src)
	}
}

// TS GZip.ts: offset/length subarray semantics.
func TestSubarrayOffsets(t *testing.T) {
	padded := append([]byte{0xFF, 0xFF}, []byte("payload")...)
	gz := CompressGz(padded, 2, 7)
	if back := DecompressGz(gz, 0, len(gz)); !bytes.Equal(back, []byte("payload")) {
		t.Fatalf("round-trip = %q", back)
	}
}

// TS GZip.ts:24-31: decompress error -> null.
func TestDecompressGzBadData(t *testing.T) {
	if DecompressGz([]byte{1, 2, 3}, 0, 3) != nil {
		t.Fatal("DecompressGz on garbage must return nil")
	}
}

// Node zlib.gzipSync writes MTIME=0 in the gzip header (bytes 4-7); pack
// byte-parity requires Go to match. compress/gzip writes zeros when
// Header.ModTime is the zero time — this pins that.
func TestHeaderMTimeZero(t *testing.T) {
	gz := CompressGz([]byte("x"), 0, 1)
	if gz[4] != 0 || gz[5] != 0 || gz[6] != 0 || gz[7] != 0 {
		t.Fatalf("MTIME bytes = %x, want 00000000", gz[4:8])
	}
}
