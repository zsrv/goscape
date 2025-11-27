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
