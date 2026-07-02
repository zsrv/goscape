package packet

import (
	"math/big"
	"testing"
)

// TestRSADecTruncatedReturnsError pins arch-29.7: a declared RSA block
// length longer than what's actually buffered must return an error, not
// fall through to GData and panic on a slice-out-of-range. The modulus and
// exponent values are irrelevant here — the bounds check fires before any
// RSA arithmetic runs — so small placeholder big.Ints are enough.
func TestRSADecTruncatedReturnsError(t *testing.T) {
	// A single byte declaring a 50-byte RSA block with none of it present.
	p := NewPacket([]byte{50})
	_, err := p.RSADec(big.NewInt(1), big.NewInt(1))
	if err == nil {
		t.Fatal("truncated RSA block must return an error, not panic (arch-29.7)")
	}
}

// TestGDataShortDestPanics pins arch-29.7: GData must panic loudly when
// dest is shorter than length rather than silently under-copying via Go's
// copy() (which copies min(len(dst), len(src)) bytes) while still advancing
// Pos by the full length — a desync with no signal that fewer bytes were
// actually delivered.
func TestGDataShortDestPanics(t *testing.T) {
	p := NewPacket([]byte{1, 2, 3, 4})
	defer func() {
		if recover() == nil {
			t.Fatal("GData with short dest must panic loudly, not silently under-copy")
		}
	}()
	dest := make([]byte, 2)
	p.GData(dest, 4)
}
