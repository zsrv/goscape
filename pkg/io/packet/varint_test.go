package packet

import (
	"bytes"
	"math"
	"testing"
)

// TestPVarIntEncoding pins the exact byte layout of PVarInt against the
// TS pVarInt algorithm (Packet.ts:388-404 @Engine-TS 2e3bcf43):
// big-endian base-128 groups, MSB continuation bit, group-count chosen
// by the 0xFFFFFF80 / 0xFFFFC000 / 0xFFE00000 / 0xF0000000 masks over
// the int32 bit pattern. Negative values always take 5 bytes (the TS
// bitwise checks coerce to int32, so every high-group mask is non-zero).
func TestPVarIntEncoding(t *testing.T) {
	cases := []struct {
		name string
		v    int32
		want []byte
	}{
		{"zero", 0, []byte{0x00}},
		{"one", 1, []byte{0x01}},
		{"max-1-byte", 0x7F, []byte{0x7F}},
		{"min-2-byte", 0x80, []byte{0x81, 0x00}},
		{"max-2-byte", 0x3FFF, []byte{0xFF, 0x7F}},
		{"min-3-byte", 0x4000, []byte{0x81, 0x80, 0x00}},
		{"max-3-byte", 0x1FFFFF, []byte{0xFF, 0xFF, 0x7F}},
		{"min-4-byte", 0x200000, []byte{0x81, 0x80, 0x80, 0x00}},
		{"max-4-byte", 0xFFFFFFF, []byte{0xFF, 0xFF, 0xFF, 0x7F}},
		{"min-5-byte", 0x10000000, []byte{0x81, 0x80, 0x80, 0x80, 0x00}},
		{"max-int32", math.MaxInt32, []byte{0x87, 0xFF, 0xFF, 0xFF, 0x7F}},
		{"neg-one", -1, []byte{0x8F, 0xFF, 0xFF, 0xFF, 0x7F}},
		{"min-int32", math.MinInt32, []byte{0x88, 0x80, 0x80, 0x80, 0x00}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := NewPacket(nil)
			p.PVarInt(tc.v)
			if got := p.Data; !bytes.Equal(got, tc.want) {
				t.Errorf("PVarInt(%d): got % X, want % X", tc.v, got, tc.want)
			}
		})
	}
}

// TestVarIntRoundTrip pins PVarInt→GVarInt identity over the full int32
// range edges, including negatives. TS semantics: gVarInt returns an
// unsigned number (`>>> 0`, Packet.ts:299) that callers store into an
// Int32Array, wrapping back to the original int32 — Go's GVarInt
// returns that int32 bit pattern directly.
func TestVarIntRoundTrip(t *testing.T) {
	values := []int32{
		0, 1, 2, 63, 64, 127, 128, 129,
		16383, 16384, 16385,
		0x1FFFFF, 0x200000, 0xFFFFFFF, 0x10000000,
		math.MaxInt32, math.MaxInt32 - 1,
		-1, -2, -127, -128, -129, -16384, -65536,
		math.MinInt32, math.MinInt32 + 1,
		42, 1000, 1000000, -1000000,
	}
	for _, v := range values {
		p := NewPacket(nil)
		p.PVarInt(v)
		if got := p.GVarInt(); got != v {
			t.Errorf("round-trip %d: got %d (encoded % X)", v, got, p.Data)
		}
		if p.Len() != 0 {
			t.Errorf("round-trip %d: %d unread bytes left", v, p.Len())
		}
	}
}

// TestGVarIntDecodesHandBuiltStreams pins GVarInt against hand-built
// byte streams (independent of PVarInt) and confirms multiple values
// decode sequentially from one buffer.
func TestGVarIntDecodesHandBuiltStreams(t *testing.T) {
	p := NewPacket([]byte{
		0x07,             // 7
		0x81, 0x00,       // 128
		0xFF, 0x7F,       // 16383
		0x8F, 0xFF, 0xFF, 0xFF, 0x7F, // -1 (unsigned 0xFFFFFFFF wraps to int32 -1)
	})
	for i, want := range []int32{7, 128, 16383, -1} {
		if got := p.GVarInt(); got != want {
			t.Errorf("value %d: got %d, want %d", i, got, want)
		}
	}
	if p.Len() != 0 {
		t.Errorf("%d unread bytes left", p.Len())
	}
}
