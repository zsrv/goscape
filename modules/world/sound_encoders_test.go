package world

import (
	"bytes"
	"testing"

	"github.com/zsrv/goscape/pkg/io/packet"
)

// TestEncodeSynthSoundFieldsDecodeInClientOrder pins client-order
// field decode of an encodeSynthSound payload. Mirrors TS
// SynthSoundEncoder.ts:9-13 (p2 synth, p1 loops, p2 delay).
func TestEncodeSynthSoundFieldsDecodeInClientOrder(t *testing.T) {
	buf := packet.NewPacket(make([]byte, 0, 8))
	encodeSynthSound(buf, 0x1234, 0x56, 0x789A)

	r := packet.NewPacket(buf.Bytes())
	r.Pos = 0
	if got := r.G2(); got != 0x1234 {
		t.Errorf("G2 (synth) = 0x%04x, want 0x1234", got)
	}
	if got := r.G1(); got != 0x56 {
		t.Errorf("G1 (loops) = 0x%02x, want 0x56", got)
	}
	if got := r.G2(); got != 0x789A {
		t.Errorf("G2 (delay) = 0x%04x, want 0x789A", got)
	}
	if r.Pos != len(buf.Bytes()) {
		t.Errorf("not all bytes consumed: pos=%d, len=%d", r.Pos, len(buf.Bytes()))
	}
}

// TestEncodeSynthSoundBytesExact pins the exact 5-byte big-endian
// payload (synth=0x0102, loops=0x03, delay=0x0405).
func TestEncodeSynthSoundBytesExact(t *testing.T) {
	buf := packet.NewPacket(make([]byte, 0, 8))
	encodeSynthSound(buf, 0x0102, 0x03, 0x0405)
	want := []byte{0x01, 0x02, 0x03, 0x04, 0x05}
	if !bytes.Equal(buf.Bytes(), want) {
		t.Errorf("bytes mismatch:\n got: % 02x\nwant: % 02x", buf.Bytes(), want)
	}
}

// TestEncodeSynthSoundZeroValuesValid pins the all-zeros payload.
func TestEncodeSynthSoundZeroValuesValid(t *testing.T) {
	buf := packet.NewPacket(make([]byte, 0, 8))
	encodeSynthSound(buf, 0, 0, 0)
	want := []byte{0, 0, 0, 0, 0}
	if !bytes.Equal(buf.Bytes(), want) {
		t.Errorf("bytes mismatch:\n got: % 02x\nwant: % 02x", buf.Bytes(), want)
	}
}

// TestEncodeSynthSoundMaxValuesValid pins boundary values
// (uint16 max for synth/delay, uint8 max for loops).
func TestEncodeSynthSoundMaxValuesValid(t *testing.T) {
	buf := packet.NewPacket(make([]byte, 0, 8))
	encodeSynthSound(buf, 0xFFFF, 0xFF, 0xFFFF)
	want := []byte{0xFF, 0xFF, 0xFF, 0xFF, 0xFF}
	if !bytes.Equal(buf.Bytes(), want) {
		t.Errorf("bytes mismatch:\n got: % 02x\nwant: % 02x", buf.Bytes(), want)
	}
}
