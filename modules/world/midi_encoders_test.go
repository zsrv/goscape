package world

import (
	"bytes"
	"testing"

	"github.com/zsrv/goscape/pkg/io/packet"
)

func TestEncodeMidiSongFieldsDecodeInClientOrder(t *testing.T) {
	buf := packet.NewPacket(make([]byte, 0, 64))
	encodeMidiSong(buf, "adventure", 0xDEADBEEF, 2048)

	r := packet.NewPacket(buf.Bytes())
	r.Pos = 0
	if got := r.GJStrLF(); got != "adventure" {
		t.Errorf("GJStrLF = %q, want \"adventure\"", got)
	}
	if got := r.G4(); got != 0xDEADBEEF {
		t.Errorf("G4 (crc) = 0x%08x, want 0xDEADBEEF", got)
	}
	if got := r.G4(); got != 2048 {
		t.Errorf("G4 (length) = %d, want 2048", got)
	}
	if r.Pos != len(buf.Bytes()) {
		t.Errorf("not all bytes consumed: pos=%d, len=%d", r.Pos, len(buf.Bytes()))
	}
}

func TestEncodeMidiSongBytesExact(t *testing.T) {
	buf := packet.NewPacket(make([]byte, 0, 32))
	encodeMidiSong(buf, "a", 0x01020304, 0x05060708)
	want := []byte{
		0x61,                   // 'a'
		0x0A,                   // PJStrLF terminator
		0x01, 0x02, 0x03, 0x04, // P4(crc)
		0x05, 0x06, 0x07, 0x08, // P4(length)
	}
	if !bytes.Equal(buf.Bytes(), want) {
		t.Errorf("bytes mismatch:\n got: % 02x\nwant: % 02x", buf.Bytes(), want)
	}
}

func TestEncodeMidiSongEmptyNameValid(t *testing.T) {
	buf := packet.NewPacket(make([]byte, 0, 32))
	encodeMidiSong(buf, "", 0, 0)
	want := []byte{0x0A, 0, 0, 0, 0, 0, 0, 0, 0}
	if !bytes.Equal(buf.Bytes(), want) {
		t.Errorf("bytes mismatch:\n got: % 02x\nwant: % 02x", buf.Bytes(), want)
	}
}

func TestEncodeMidiJingleFieldsDecodeInClientOrder(t *testing.T) {
	buf := packet.NewPacket(make([]byte, 0, 32))
	data := []byte{0x01, 0x02, 0x03}
	encodeMidiJingle(buf, 500, data)

	r := packet.NewPacket(buf.Bytes())
	r.Pos = 0
	if got := r.G2(); got != 500 {
		t.Errorf("G2 (delay) = %d, want 500", got)
	}
	rest := buf.Bytes()[r.Pos:]
	if !bytes.Equal(rest, data) {
		t.Errorf("data tail mismatch: got %v, want %v", rest, data)
	}
}

func TestEncodeMidiJingleBytesExact(t *testing.T) {
	buf := packet.NewPacket(make([]byte, 0, 16))
	encodeMidiJingle(buf, 0x0102, []byte{0xFF})
	want := []byte{0x01, 0x02, 0xFF}
	if !bytes.Equal(buf.Bytes(), want) {
		t.Errorf("bytes mismatch:\n got: % 02x\nwant: % 02x", buf.Bytes(), want)
	}
}

func TestEncodeMidiJingleEmptyDataValid(t *testing.T) {
	buf := packet.NewPacket(make([]byte, 0, 8))
	encodeMidiJingle(buf, 0, []byte{})
	want := []byte{0x00, 0x00}
	if !bytes.Equal(buf.Bytes(), want) {
		t.Errorf("bytes mismatch:\n got: % 02x\nwant: % 02x", buf.Bytes(), want)
	}
}
