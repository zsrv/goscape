package jagfile

import (
	"bytes"
	"io"

	"github.com/zsrv/goscape/pkg/io/bzip2"
)

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
		decompressedLength = (int(compressed[0]) << 24) | (int(compressed[1]) << 16) | (int(compressed[2]) << 8) | int(compressed[3])
		compressed[0] = 'B'
		compressed[1] = 'Z'
		compressed[2] = 'h'
		compressed[3] = '1'
		prependHeader = false
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
	_, err = io.Copy(decompressedBuf, br)
	if err != nil {
		return nil, err
	}

	return decompressedBuf.Bytes(), nil
}
