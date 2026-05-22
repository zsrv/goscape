package jagfile

import (
	"bytes"
	"fmt"
	"io"

	"github.com/zsrv/goscape/pkg/io/bzip2"
)

// MaxBZip2DecompressedSize caps both the declared decompressed length read
// from an untrusted archive header and the actual byte count drained from
// the bzip2 stream. A malicious archive can declare a huge unpackedSize
// (e.g. 2GB) for a tiny payload to force a large allocation; bounding
// both sides keeps a single corrupt or hostile entry from exhausting RAM.
// 64 MiB is well above any single jagfile entry the client legitimately
// ships (full cache is ~10 MiB compressed).
const MaxBZip2DecompressedSize = 64 << 20

func BZip2Compress(decompressed []byte, prefixLength bool, removeHeader bool, blockSize int, compressedLength int) ([]byte, error) {
	if compressedLength == 0 {
		compressedLength = len(decompressed) + 1024
	}

	if compressedLength < 128 {
		compressedLength = 128
	}

	compressedBuf := bytes.NewBuffer(make([]byte, 0, compressedLength))
	bw, err := bzip2.NewWriter(compressedBuf, &bzip2.WriterConfig{
		Level: blockSize,
	})
	if err != nil {
		return nil, err
	}
	_, err = bw.Write(decompressed)
	if err != nil {
		return nil, err
	}
	bw.Close()

	compressed := compressedBuf.Bytes()
	if prefixLength {
		compressed[0] = byte((len(decompressed) >> 24) & 0xFF)
		compressed[1] = byte((len(decompressed) >> 16) & 0xFF)
		compressed[2] = byte((len(decompressed) >> 8) & 0xFF)
		compressed[3] = byte((len(decompressed)) & 0xFF)
	}

	if removeHeader {
		return compressed[4:], nil
	}

	return compressed, nil
}

func BZip2Decompress(compressed []byte, decompressedLength int, prependHeader bool, containsDecompressedLength bool) ([]byte, error) {
	if containsDecompressedLength {
		if len(compressed) < 4 {
			return nil, fmt.Errorf("bzip2: payload too short for embedded length prefix")
		}
		decompressedLength = (int(compressed[0]) << 24) | (int(compressed[1]) << 16) | (int(compressed[2]) << 8) | int(compressed[3])
		compressed[0] = 'B'
		compressed[1] = 'Z'
		compressed[2] = 'h'
		compressed[3] = '1'
		prependHeader = false
	}

	if decompressedLength < 0 || decompressedLength > MaxBZip2DecompressedSize {
		return nil, fmt.Errorf("bzip2: declared decompressed length %d out of range (max %d)", decompressedLength, MaxBZip2DecompressedSize)
	}

	if prependHeader {
		temp := make([]uint8, 0, len(compressed)+4)
		temp = append(temp, 'B', 'Z', 'h', '1')
		temp = append(temp, compressed...)
		compressed = temp
	}

	compressedBuf := bytes.NewBuffer(compressed)
	br, err := bzip2.NewReader(compressedBuf, nil)
	if err != nil {
		return nil, err
	}
	defer br.Close()

	decompressedBuf := bytes.NewBuffer(make([]byte, 0, decompressedLength))
	// Bound the actual decode in case the stream produces more bytes than the
	// header declared. CopyN reads at most max+1; if the +1 byte gets through
	// we know the stream exceeded the cap and reject.
	n, err := io.CopyN(decompressedBuf, br, int64(MaxBZip2DecompressedSize)+1)
	if err != nil && err != io.EOF {
		return nil, err
	}
	if n > int64(MaxBZip2DecompressedSize) {
		return nil, fmt.Errorf("bzip2: decompressed output exceeds max %d bytes", MaxBZip2DecompressedSize)
	}

	return decompressedBuf.Bytes(), nil
}
