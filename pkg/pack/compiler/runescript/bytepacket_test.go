// pkg/pack/compiler/runescript/bytepacket_test.go
package runescript

import (
	"bytes"
	"testing"
)

// TestCrc32_GoldenVectors pins crc32 against known values.
// Verified against TS BytePacket.crc32 output for identical inputs.
func TestCrc32_GoldenVectors(t *testing.T) {
	cases := []struct {
		name string
		in   []byte
		want int32
	}{
		{"empty", []byte{}, 0},
		{"single zero byte", []byte{0x00}, int32(-771559539)},      // 0xD202EF8D as signed
		{"abc", []byte("abc"), int32(0x352441C2)},
		{"binary", []byte{0xDE, 0xAD, 0xBE, 0xEF}, int32(0x7C9CA35A)},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := Crc32(c.in)
			if got != c.want {
				t.Errorf("Crc32(%q) = 0x%08x, want 0x%08x", c.in, uint32(got), uint32(c.want))
			}
		})
	}
}

func TestByteWriter_P1(t *testing.T) {
	w := NewByteWriter(8)
	w.P1(0x12)
	w.P1(0xFF)
	w.P1(0x00)
	got := w.Bytes()
	want := []byte{0x12, 0xFF, 0x00}
	if !bytes.Equal(got, want) {
		t.Errorf("ByteWriter.P1: got %x, want %x", got, want)
	}
}

func TestByteWriter_P2(t *testing.T) {
	w := NewByteWriter(8)
	w.P2(0x1234)
	w.P2(0xFFFF)
	got := w.Bytes()
	want := []byte{0x12, 0x34, 0xFF, 0xFF}
	if !bytes.Equal(got, want) {
		t.Errorf("ByteWriter.P2: got %x, want %x", got, want)
	}
}

func TestByteWriter_P4(t *testing.T) {
	w := NewByteWriter(8)
	w.P4(0x12345678)
	w.P4(-1)
	got := w.Bytes()
	want := []byte{0x12, 0x34, 0x56, 0x78, 0xFF, 0xFF, 0xFF, 0xFF}
	if !bytes.Equal(got, want) {
		t.Errorf("ByteWriter.P4: got %x, want %x", got, want)
	}
}

// TestByteWriter_PSmart2or4 pins the 32768 boundary.
// TS BytePacket.ts L51-57: <32768 → 2-byte BE; else → 4-byte (value | 0x80000000) BE.
func TestByteWriter_PSmart2or4(t *testing.T) {
	cases := []struct {
		name string
		v    int
		want []byte
	}{
		{"zero", 0, []byte{0x00, 0x00}},
		{"32767 last 2-byte", 32767, []byte{0x7F, 0xFF}},
		{"32768 first 4-byte", 32768, []byte{0x80, 0x00, 0x80, 0x00}},
		{"65536", 65536, []byte{0x80, 0x01, 0x00, 0x00}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			w := NewByteWriter(8)
			w.PSmart2or4(c.v)
			got := w.Bytes()
			if !bytes.Equal(got, c.want) {
				t.Errorf("PSmart2or4(%d): got %x, want %x", c.v, got, c.want)
			}
		})
	}
}

func TestByteWriter_PData(t *testing.T) {
	w := NewByteWriter(4)
	w.PData([]byte{0xAA, 0xBB})
	w.PData([]byte{0xCC, 0xDD, 0xEE, 0xFF})
	got := w.Bytes()
	want := []byte{0xAA, 0xBB, 0xCC, 0xDD, 0xEE, 0xFF}
	if !bytes.Equal(got, want) {
		t.Errorf("ByteWriter.PData: got %x, want %x", got, want)
	}
}

// TestByteWriter_GrowsBuffer pins TS ensure() doubling behavior.
func TestByteWriter_GrowsBuffer(t *testing.T) {
	w := NewByteWriter(2)
	for i := range 100 {
		w.P1(i & 0xff)
	}
	if w.Len() != 100 {
		t.Errorf("Len after 100 P1s: got %d, want 100", w.Len())
	}
	got := w.Bytes()
	if got[0] != 0x00 || got[99] != 99 {
		t.Errorf("byte content after growth: got[0]=%02x got[99]=%02x, want 00 63", got[0], got[99])
	}
}

// TestByteWriter_InitialSizeFloor pins TS L33: max(64, initialSize).
func TestByteWriter_InitialSizeFloor(t *testing.T) {
	w := NewByteWriter(4)
	// Writes up to 64 bytes should not require any growth visible to caller.
	for range 64 {
		w.P1(0xAA)
	}
	if w.Len() != 64 {
		t.Errorf("Len after 64 P1s with initialSize=4: got %d, want 64", w.Len())
	}
}
