package objtype

import (
	"testing"

	"github.com/zsrv/goscape/pkg/io/packet"
)

func newParamPacket(b0, b1, b2, b3 uint8) *packet.Packet {
	pkt := packet.NewPacket(nil)
	pkt.P1(b0)
	pkt.P1(b1)
	pkt.P1(b2)
	pkt.P1(b3)
	return pkt
}

func TestParamType_DecodeNegativeDefault(t *testing.T) {
	pt := NewParamType(0)
	pkt := newParamPacket(0xFF, 0xFF, 0xFF, 0xFF)
	if err := pt.Decode(2, pkt); err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if got, want := pt.DefaultInt, int32(-1); got != want {
		t.Errorf("DefaultInt: got %d, want %d", got, want)
	}
	if got, want := int(pt.DefaultInt), -1; got != want {
		t.Errorf("int(DefaultInt): got %d, want %d", got, want)
	}
}

func TestParamType_DecodePositiveDefault(t *testing.T) {
	pt := NewParamType(0)
	pkt := newParamPacket(0x00, 0x00, 0x00, 0x64)
	if err := pt.Decode(2, pkt); err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if got, want := pt.DefaultInt, int32(100); got != want {
		t.Errorf("DefaultInt: got %d, want %d", got, want)
	}
}

func TestParamType_DecodeMaxInt32(t *testing.T) {
	pt := NewParamType(0)
	pkt := newParamPacket(0x7F, 0xFF, 0xFF, 0xFF)
	if err := pt.Decode(2, pkt); err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if got, want := pt.DefaultInt, int32(2147483647); got != want {
		t.Errorf("DefaultInt: got %d, want %d", got, want)
	}
}
