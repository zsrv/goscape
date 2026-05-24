package packet

import (
	"testing"
)

func TestPacketBit(t *testing.T) {
	expected := Alloc(0)
	expected.AccessBits()
	expected.PBit(1, 0)
	expected.PBit(4, 3)
	expected.PBit(7, 13)
	expected.AccessBytes()

	result := NewPacket(expected.Data)
	result.AccessBits()

	if res := result.GBit(1); res != 0 {
		t.Fatalf("GBit(1) = %v, want 0", res)
	}
	if res := result.GBit(4); res != 3 {
		t.Fatalf("GBit(4) = %v, want 3", res)
	}
	if res := result.GBit(7); res != 13 {
		t.Fatalf("GBit(7) = %v, want 13", res)
	}
}

// TestGBitWideValue pins that GBit returns reads wider than 8 bits without
// truncation, matching TS Packet.gBit which returns a number (Packet.ts:384).
// The old uint8 return silently truncated reads > 8 bits to their low byte. L36.
func TestGBitWideValue(t *testing.T) {
	tests := []struct {
		n     int
		value int
	}{
		{16, 0xABCD},     // low byte 0xCD; truncation would drop 0xAB00
		{12, 0x0FA5},     // truncation would drop the high nibble
		{32, 0x12345678}, // full-width read
	}
	for _, tt := range tests {
		p := NewPacket(nil)
		p.AccessBits()
		p.PBit(tt.n, tt.value)
		p.AccessBytes()

		r := NewPacket(p.Data)
		r.AccessBits()
		if got := r.GBit(tt.n); got != tt.value {
			t.Errorf("GBit(%d) = %#x, want %#x", tt.n, got, tt.value)
		}
	}
}

// TestPacketBitMSBFirst audits that PBit writes MSB-first, matching
// rsbuf (github.com/2004scape/rsbuf branch 225) pbit behavior.
//
// Sequence: PBit(3, 5); PBit(11, 1500); PBit(1, 0)
// Bits: 101 10111011100 0 = 101101110111000 (15 bits)
// Padded to 16 bits: 1011011101110000
// Expected bytes: 0xB7 0x70
func TestPacketBitMSBFirst(t *testing.T) {
	p := NewPacket(nil)
	p.AccessBits()
	p.PBit(3, 5)
	p.PBit(11, 1500)
	p.PBit(1, 0)
	p.AccessBytes()

	got := p.Data[:p.Pos]
	if len(got) != 2 || got[0] != 0xB7 || got[1] != 0x70 {
		t.Errorf("PBit MSB-first: got % x, want [b7 70]", got)
	}
}
