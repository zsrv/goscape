package packet

import (
	"io"
	"testing"
)

// recoverBoundsExpected classifies a recovered panic value from a packet
// reader or PSize call.
//
// All intentional bounds panics in this package satisfy the error
// interface:
//   - G1..G8, IG2, IG4, GSmart, GSmartS: panic(io.EOF) on short buffer.
//   - PSize1/2/4: panic(fmt.Errorf("PSizeN: ...")) on invalid length.
//   - GData: no explicit guard; panics with a runtime slice-bounds error
//     that implements runtime.Error and therefore error.
//
// Any recovered value that does NOT satisfy error is an unexpected panic
// (a real bug) and is re-panicked so the fuzzer records a crash entry.
func recoverBoundsExpected(r any) {
	if r == nil {
		return
	}
	if _, ok := r.(error); ok {
		return
	}
	panic(r)
}

// callAndRecover calls fn and absorbs any expected bounds panic.
func callAndRecover(fn func()) {
	defer func() { recoverBoundsExpected(recover()) }()
	fn()
}

// captureRecovered calls fn and returns the recovered panic value, or nil
// if fn returned normally.
func captureRecovered(fn func()) (out any) {
	defer func() { out = recover() }()
	fn()
	return nil
}

// FuzzReaders populates a Packet from arbitrary fuzz bytes and drives
// every reader method unconditionally with the read pointer reset to
// zero before each call. Expected panics (io.EOF from bounds-guarded
// readers, runtime slice errors from GData) satisfy the error interface
// and are absorbed. Non-error panics are re-panicked as real bugs.
//
// GJStr / GJStrLF / GJStrNUL are called without a recovery wrapper
// because they handle empty and terminator-free buffers gracefully.
func FuzzReaders(f *testing.F) {
	f.Add([]byte{})
	f.Add([]byte{0x00})
	f.Add([]byte{0x7F})
	f.Add([]byte{0xFF})
	// G2/IG2 boundary: one byte (under), two bytes (exact).
	f.Add([]byte{0x01})
	f.Add([]byte{0x01, 0x02})
	// G3 boundary.
	f.Add([]byte{0x01, 0x02, 0x03})
	// G4/IG4 boundary.
	f.Add([]byte{0x01, 0x02, 0x03, 0x04})
	// G8 boundary: seven bytes (under), eight bytes (exact).
	f.Add([]byte{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07})
	f.Add([]byte{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08})
	// GSmart high branch: first byte >= 128, two bytes needed.
	f.Add([]byte{0x80})
	f.Add([]byte{0x80, 0xCA})
	// GSmartS one-byte path (first byte < 128).
	f.Add([]byte{0x40})
	// GJStrLF: newline-terminated.
	f.Add([]byte{'H', 'e', 'l', 'l', 'o', 10})
	// GJStrNUL: NUL-terminated.
	f.Add([]byte{'H', 'i', 0x00})
	// No terminator: exercises the scan-to-end fallback in GJStr.
	f.Add([]byte{'a', 'b', 'c'})
	// Multi-field seed for sequential-reader coverage.
	f.Add([]byte{0x80, 0xCA, 0x01, 0x02, 0x03, 0x04, 'H', 'i', 0x00})

	f.Fuzz(func(t *testing.T, data []byte) {
		p := NewPacket(data)

		callAndRecover(func() { p.Pos = 0; _ = p.G1() })
		callAndRecover(func() { p.Pos = 0; _ = p.G2() })
		callAndRecover(func() { p.Pos = 0; _ = p.G3() })
		callAndRecover(func() { p.Pos = 0; _ = p.G4() })
		callAndRecover(func() { p.Pos = 0; _ = p.G8() })
		callAndRecover(func() { p.Pos = 0; _ = p.IG2() })
		callAndRecover(func() { p.Pos = 0; _ = p.IG4() })
		callAndRecover(func() { p.Pos = 0; _ = p.GSmart() })
		callAndRecover(func() { p.Pos = 0; _ = p.GSmartS() })

		// GJStr family never panics.
		p.Pos = 0
		_ = p.GJStr(0)
		p.Pos = 0
		_ = p.GJStrLF()
		p.Pos = 0
		_ = p.GJStrNUL()

		n := len(data)

		// GData with an in-bounds length must succeed.
		callAndRecover(func() {
			p.Pos = 0
			if n > 0 {
				dest := make([]byte, n)
				p.GData(dest, n)
			}
		})

		// GData one past the end exercises the runtime slice-bounds panic.
		callAndRecover(func() {
			p.Pos = 0
			dest := make([]byte, n+1)
			p.GData(dest, n+1)
		})

		// Verify that G1 on an empty buffer panics with exactly io.EOF
		// (not some other error or a non-error value).
		if n == 0 {
			got := captureRecovered(func() { p.Pos = 0; _ = p.G1() })
			if got != nil && got != io.EOF {
				t.Errorf("G1 on empty buffer: want panic(io.EOF), got %T: %v", got, got)
			}
		}
	})
}

// FuzzPSize exercises PSize1, PSize2, and PSize4 with a buffer of fuzz
// bytes and a fuzzed length argument.
//
// Each PSize variant requires length >= 0 AND length+N <= len(p.Data)
// (N = 1, 2, or 4 respectively). Violations panic with a fmt.Errorf
// value that satisfies error and is absorbed. Valid calls write the
// length into the buffer and must not panic.
func FuzzPSize(f *testing.F) {
	f.Add([]byte{}, 0)
	f.Add([]byte{0x00}, 0)
	// Negative length — must panic.
	f.Add([]byte{0x01, 0x02, 0x03, 0x04}, -1)
	// length == len(data) — must panic (no room for size header byte).
	f.Add([]byte{0x01, 0x02, 0x03, 0x04}, 4)
	// length + 1 == len(data) — valid for PSize1 only.
	f.Add([]byte{0x01, 0x02, 0x03, 0x04}, 3)
	// length + 4 == len(data) — valid for all three.
	f.Add([]byte{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08}, 4)
	// length > len(data) — must panic.
	f.Add([]byte{0x01}, 100)
	// Large buffer, small valid lengths.
	f.Add(make([]byte, 64), 0)
	f.Add(make([]byte, 64), 1)
	f.Add(make([]byte, 64), 60)

	f.Fuzz(func(t *testing.T, data []byte, length int) {
		callAndRecover(func() {
			p := NewPacket(append([]byte(nil), data...))
			p.PSize1(length)
		})
		callAndRecover(func() {
			p := NewPacket(append([]byte(nil), data...))
			p.PSize2(length)
		})
		callAndRecover(func() {
			p := NewPacket(append([]byte(nil), data...))
			p.PSize4(length)
		})
	})
}
