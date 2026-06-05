// Package gziputil ports src/io/GZip.ts (Engine-TS 9aadcec4): gzip helpers
// with the byte-9 (OS field) zeroing that keeps pack output deterministic.
package gziputil

import (
	"bytes"
	"compress/gzip"
	"io"
)

// CompressGz mirrors TS compressGz (GZip.ts:3-18). Byte 9 of the gzip
// header (OS) is zeroed for deterministic output.
// Implemented via CompressCFGz — the bit-exact cf-zlib deflate port —
// so output is byte-identical to the reference corpus produced by
// bun 1.2.20 node:zlib.gzipSync (Cloudflare zlib fork, commit 886098f3).
func CompressGz(src []byte, off, length int) []byte {
	return CompressCFGz(src, off, length)
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
