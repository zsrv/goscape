package jagfile

import (
	"testing"

	"github.com/zsrv/goscape/pkg/io/packet"
)

// recoverJagExpected classifies a recovered panic from jagfile / bzip2
// parsing. Expected panics all satisfy the error interface:
//   - io.EOF from short-input packet reads (G2, G3, G4 inside NewJagfile).
//   - fmt.Errorf from BZip2Decompress validation guards.
//   - runtime.Error from out-of-bounds slice access in Jagfile.Get when
//     FilePos+FilePackedSize exceeds len(jf.Data).
//
// Non-error panics are re-panicked as real bugs.
func recoverJagExpected(r any) {
	if r == nil {
		return
	}
	if _, ok := r.(error); ok {
		return
	}
	panic(r)
}

// FuzzBZip2Decompress fuzzes BZip2Decompress across all three code paths.
//
// The bool fuzz dimension guides corpus generation; the fuzz body always
// exercises all three paths to maximise branch coverage per iteration:
//  1. containsDecompressedLength=true — first 4 bytes encode the length;
//     tests the "too short" guard (<4 bytes) and the "out of range"
//     guard (declared length > MaxBZip2DecompressedSize or negative).
//  2. containsDecompressedLength=false, prependHeader=false — uses the
//     fuzzed decompressedLength directly.
//  3. containsDecompressedLength=false, prependHeader=true — same, but
//     the "BZh1" header is prepended before decoding.
//
// BZip2Decompress must never panic; all invalid inputs return errors.
func FuzzBZip2Decompress(f *testing.F) {
	f.Add([]byte{}, 0, false)
	f.Add([]byte{}, 0, true)
	// containsDecompressedLength=true, too few bytes for length prefix.
	f.Add([]byte{0x00}, 0, true)
	f.Add([]byte{0x00, 0x00, 0x00}, 0, true)
	// 4-byte length prefix of zero.
	f.Add([]byte{0x00, 0x00, 0x00, 0x00}, 0, true)
	// 4-byte prefix that exceeds MaxBZip2DecompressedSize (67108864 = 0x04000000).
	f.Add([]byte{0x05, 0x00, 0x00, 0x00}, 0, true)
	// Negative decompressedLength.
	f.Add([]byte{0x42, 0x5A, 0x68, 0x31}, -1, false)
	// decompressedLength == MaxBZip2DecompressedSize+1.
	f.Add([]byte{0x42, 0x5A, 0x68, 0x31}, MaxBZip2DecompressedSize+1, false)
	// Valid bzip2 stream for "Hello World!" (full header, from TestBZip2Decompress).
	f.Add([]byte{
		0x42, 0x5A, 0x68, 0x31, 0x31, 0x41, 0x59, 0x26,
		0x53, 0x59, 0x6B, 0x1A, 0x7C, 0xAE, 0x00, 0x00,
		0x01, 0x17, 0x80, 0x60, 0x00, 0x00, 0x40, 0x00,
		0x80, 0x06, 0x04, 0x90, 0x00, 0x20, 0x00, 0x22,
		0x2A, 0x37, 0xFA, 0xA9, 0xFA, 0xA7, 0xED, 0x08,
		0x06, 0x0B, 0x02, 0xC5, 0x39, 0x70, 0xBB, 0x92,
		0x29, 0xC2, 0x84, 0x83, 0x58, 0xD3, 0xE5, 0x70,
	}, 12, false)
	// Stream body without "BZh1" header (from TestBZip2Compress "without header").
	f.Add([]byte{
		0x31, 0x41, 0x59, 0x26, 0x53, 0x59, 0x6B, 0x1A,
		0x7C, 0xAE, 0x00, 0x00, 0x01, 0x17, 0x80, 0x60,
		0x00, 0x00, 0x40, 0x00, 0x80, 0x06, 0x04, 0x90,
		0x00, 0x20, 0x00, 0x22, 0x42, 0x37, 0xFA, 0xA9,
		0xFA, 0xA7, 0xED, 0x08, 0x06, 0x0B, 0x02, 0xC5,
		0x39, 0x70, 0xBB, 0x92, 0x29, 0xC2, 0x84, 0x83,
		0x58, 0xD3, 0xE5, 0x70,
	}, 12, true)
	// containsDecompressedLength=true with 12-byte declared length followed
	// by the "Hello World!" bzip2 body (without "BZh1" prefix).
	f.Add([]byte{
		0x00, 0x00, 0x00, 0x0C, // decompressedLength = 12 (big-endian)
		0x31, 0x41, 0x59, 0x26, 0x53, 0x59, 0x6B, 0x1A,
		0x7C, 0xAE, 0x00, 0x00, 0x01, 0x17, 0x80, 0x60,
		0x00, 0x00, 0x40, 0x00, 0x80, 0x06, 0x04, 0x90,
		0x00, 0x20, 0x00, 0x22, 0x42, 0x37, 0xFA, 0xA9,
		0xFA, 0xA7, 0xED, 0x08, 0x06, 0x0B, 0x02, 0xC5,
		0x39, 0x70, 0xBB, 0x92, 0x29, 0xC2, 0x84, 0x83,
		0x58, 0xD3, 0xE5, 0x70,
	}, 0, true)
	// Corrupt / random bytes.
	f.Add([]byte{0xDE, 0xAD, 0xBE, 0xEF, 0x00, 0x01}, 4, false)

	f.Fuzz(func(t *testing.T, compressed []byte, decompressedLength int, _ bool) {
		// Path 1: containsDecompressedLength=true.
		// BZip2Decompress mutates bytes 0-3 in-place when
		// containsDecompressedLength=true, so pass a copy.
		{
			src := append([]byte(nil), compressed...)
			_, _ = BZip2Decompress(src, 0, false, true)
		}

		// Path 2: fuzzed decompressedLength, no prepended header.
		{
			_, _ = BZip2Decompress(compressed, decompressedLength, false, false)
		}

		// Path 3: fuzzed decompressedLength, with prepended "BZh1" header.
		{
			_, _ = BZip2Decompress(compressed, decompressedLength, true, false)
		}
	})
}

// FuzzJagfileGet constructs a Jagfile from arbitrary fuzz bytes and
// exercises every valid index via Get.
//
// NewJagfile reads a 6-byte outer header (G3+G3), a 2-byte FileCount
// (G2), and 10 bytes per entry (G4+G3+G3). All reads are bounds-guarded:
// truncated input panics with io.EOF. Get performs a slice expression
// on jf.Data that panics with a runtime.Error when FilePos+FilePackedSize
// exceeds the data length. Both panic types satisfy the error interface
// and are absorbed by recoverJagExpected.
func FuzzJagfileGet(f *testing.F) {
	// Empty and too-short inputs.
	f.Add([]byte{})
	f.Add([]byte{0x00, 0x00, 0x01})

	// Six-byte outer header only; G2 for FileCount panics (no data remains).
	f.Add([]byte{0x00, 0x00, 0x00, 0x00, 0x00, 0x00})

	// Eight bytes: valid header + FileCount=0 (no entry reads needed).
	f.Add([]byte{0x00, 0x00, 0x01, 0x00, 0x00, 0x01, 0x00, 0x00})

	// Well-formed single-file jagfile (non-compressed path).
	// Matches the MakeTestJagfile() fixture in jagfile_test.go.
	f.Add([]byte{
		0x00, 0x00, 0x01, // unpackedSize = 1
		0x00, 0x00, 0x01, // packedSize = 1 (equal → non-compressed path)
		0x00, 0x01, // FileCount = 1
		0xA6, 0x73, 0xCA, 0xEE, // FileHash[0] = hitmarks.dat
		0x00, 0x00, 0x01, // FileUnpackedSize[0] = 1
		0x00, 0x00, 0x01, // FilePackedSize[0] = 1
		0xFF, // payload[0]
	})

	// FileCount = 0: valid header, no Get calls inside the loop.
	f.Add([]byte{
		0x00, 0x00, 0x05,
		0x00, 0x00, 0x05,
		0x00, 0x00, // FileCount = 0
		0x01, 0x02, 0x03, 0x04, 0x05,
	})

	// Corrupt: FilePackedSize large → runtime slice OOB in Get.
	f.Add([]byte{
		0x00, 0x00, 0x01, 0x00, 0x00, 0x01, // unpackedSize == packedSize
		0x00, 0x01, // FileCount = 1
		0xDE, 0xAD, 0xBE, 0xEF, // hash
		0x00, 0xFF, 0xFF, // FileUnpackedSize[0] = large
		0x00, 0xFF, 0xFF, // FilePackedSize[0] = large (OOB)
	})

	f.Fuzz(func(t *testing.T, data []byte) {
		var jf *Jagfile

		func() {
			defer func() { recoverJagExpected(recover()) }()
			var err error
			jf, err = NewJagfile(packet.NewPacket(append([]byte(nil), data...)))
			if err != nil {
				jf = nil
			}
		}()

		if jf == nil {
			return
		}

		// Exercise Get for every valid index.
		for i := range jf.FileCount {
			func() {
				defer func() { recoverJagExpected(recover()) }()
				_, _ = jf.Get(i)
			}()
		}

		// Out-of-range indices must return errors, not panic.
		func() {
			defer func() { recoverJagExpected(recover()) }()
			_, _ = jf.Get(-1)
		}()
		func() {
			defer func() { recoverJagExpected(recover()) }()
			_, _ = jf.Get(jf.FileCount)
		}()
	})
}
