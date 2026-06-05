package world

import (
	"bytes"
	"testing"

	"github.com/zsrv/goscape/pkg/io/packet"
)

// TestEncodeMidiSong244BytesExact pins the 244 wire shape: p2(id) only.
// TS MidiSongEncoder.ts (244): encode writes buf.p2(message.id); test() returns 2.
func TestEncodeMidiSong244BytesExact(t *testing.T) {
	buf := packet.NewPacket(make([]byte, 0, 8))
	encodeMidiSong(buf, 0x1234)
	want := []byte{0x12, 0x34}
	if !bytes.Equal(buf.Bytes(), want) {
		t.Errorf("bytes mismatch:\n got: % 02x\nwant: % 02x", buf.Bytes(), want)
	}
}

// TestEncodeMidiSong244ZeroId pins id=0 encodes as 0x0000.
func TestEncodeMidiSong244ZeroId(t *testing.T) {
	buf := packet.NewPacket(make([]byte, 0, 8))
	encodeMidiSong(buf, 0)
	want := []byte{0x00, 0x00}
	if !bytes.Equal(buf.Bytes(), want) {
		t.Errorf("bytes mismatch:\n got: % 02x\nwant: % 02x", buf.Bytes(), want)
	}
}

// TestEncodeMidiJingle244BytesExact pins the 244 wire shape: p2(id) p2(delay).
// TS MidiJingleEncoder.ts (244): encode writes buf.p2(message.id) buf.p2(message.delay);
// test() returns 4.
func TestEncodeMidiJingle244BytesExact(t *testing.T) {
	buf := packet.NewPacket(make([]byte, 0, 8))
	encodeMidiJingle(buf, 0x0042, 500)
	want := []byte{
		0x00, 0x42, // p2(id=0x42)
		0x01, 0xF4, // p2(delay=500)
	}
	if !bytes.Equal(buf.Bytes(), want) {
		t.Errorf("bytes mismatch:\n got: % 02x\nwant: % 02x", buf.Bytes(), want)
	}
}

// TestEncodeMidiJingle244ZeroFields pins id=0, delay=0 → 0x0000 0x0000.
func TestEncodeMidiJingle244ZeroFields(t *testing.T) {
	buf := packet.NewPacket(make([]byte, 0, 8))
	encodeMidiJingle(buf, 0, 0)
	want := []byte{0x00, 0x00, 0x00, 0x00}
	if !bytes.Equal(buf.Bytes(), want) {
		t.Errorf("bytes mismatch:\n got: % 02x\nwant: % 02x", buf.Bytes(), want)
	}
}

// TestServerMidiIDByName244NilRegistryReturnsMinusOne pins that a Server with
// nil midiPack returns -1 for all names (degrade posture when ContentPath is
// empty or midi.pack is absent). Replaces the B2 stub
// TestMidiIDByName244AlwaysReturnsMinusOne which tested the old free function.
func TestServerMidiIDByName244NilRegistryReturnsMinusOne(t *testing.T) {
	s := &Server{midiPack: nil}
	cases := []string{"adventure", "", "fanfare", "some_song"}
	for _, name := range cases {
		if got := s.midiIDByName(name); got != -1 {
			t.Errorf("midiIDByName(%q) with nil registry = %d; want -1", name, got)
		}
	}
}

// TestPlaySongNoServer244SilentNoOp pins that PlaySong is a silent no-op when
// p.client.server is nil (bare test player, no server). Mirrors TS
// Player.ts:1921 `if (id !== -1) this.write(...)` guard — nil server
// degrades to -1 posture.
func TestPlaySongNoServer244SilentNoOp(t *testing.T) {
	p, _ := newTestPlayer(t)
	enc, _ := isaacPair([4]uint32{1, 2, 3, 4})
	p.client.encryptor = enc
	// p.client.server is nil; PlaySong must not panic and must write nothing.
	p.PlaySong("adventure")
	if n := p.client.bufw.Buffered(); n != 0 {
		t.Errorf("PlaySong(nil server) wrote %d bytes; want 0", n)
	}
}

// TestPlayJingleNoServer244SilentNoOp pins that PlayJingle is a silent no-op
// when p.client.server is nil. Mirrors TS Player.ts:1929 guard.
func TestPlayJingleNoServer244SilentNoOp(t *testing.T) {
	p, _ := newTestPlayer(t)
	enc, _ := isaacPair([4]uint32{1, 2, 3, 4})
	p.client.encryptor = enc
	p.PlayJingle(3, "fanfare")
	if n := p.client.bufw.Buffered(); n != 0 {
		t.Errorf("PlayJingle(nil server) wrote %d bytes; want 0", n)
	}
}
