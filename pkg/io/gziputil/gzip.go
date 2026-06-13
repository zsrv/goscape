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
//
// rev-274: implemented via CompressSZGz — the bit-exact stock zlib 1.3.1
// level-6 deflate port — so output is byte-identical to the ORIGINAL r274
// cache, which was produced by stock zlib 1.3.1 level 6 with the gzip OS byte
// zeroed.  (python zlib 1.3.1 level-6 reproduces every original cache gzip
// member byte-for-byte: 6201/6201.)
//
// CompressCFGz (the Cloudflare-fork port) remains in the tree for rev<=254
// branches and its own unit tests; it is NOT used on this revision.
func CompressGz(src []byte, off, length int) []byte {
	return CompressSZGz(src, off, length)
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
