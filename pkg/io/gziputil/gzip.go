// Package gziputil ports src/io/GZip.ts (Engine-TS 9aadcec4): gzip helpers
// with the byte-9 (OS field) zeroing that keeps pack output deterministic.
package gziputil

import (
	"bytes"
	"compress/gzip"
	"io"
)

// CompressGz mirrors TS compressGz (GZip.ts:3-18). Byte 9 of the gzip
// header (OS) is zeroed for deterministic output. Returns nil on error
// (TS catches and logs; goscape returns nil).
func CompressGz(src []byte, off, length int) []byte {
	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	if _, err := zw.Write(src[off : off+length]); err != nil {
		return nil
	}
	if err := zw.Close(); err != nil {
		return nil
	}
	data := buf.Bytes()
	data[9] = 0
	return data
}

// DecompressGz mirrors TS decompressGz (GZip.ts:20-31). Returns nil on error.
func DecompressGz(src []byte, off, length int) []byte {
	zr, err := gzip.NewReader(bytes.NewReader(src[off : off+length]))
	if err != nil {
		return nil
	}
	out, err := io.ReadAll(zr)
	if err != nil {
		return nil
	}
	if err := zr.Close(); err != nil {
		// Trailer CRC/ISIZE validation — TS gunzipSync validates the
		// whole stream; mirror that strictness.
		return nil
	}
	return out
}
