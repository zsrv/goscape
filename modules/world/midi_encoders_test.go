package world

import (
	"bytes"
	"testing"

	"github.com/zsrv/goscape/pkg/io/packet"
	"github.com/zsrv/goscape/pkg/midi"
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

// A10 @2e3bcf43: the 244-era midiIDByName/loadMidiPack registry pins are
// retired with the registry itself (names resolve at compile time;
// playSong/playJingle are id-based — TS Player.ts:1985-1991).

// TestPlaySongIDWritesUnconditionally pins the rev-254 contract: PlaySong
// takes a TRACK ID and writes MidiSong even with no server attached (TS
// playSong has no guard — `this.write(new MidiSong(id))`). Supersedes
// TestPlaySongNoServer244SilentNoOp (the nil-server silence came from the
// retired registry lookup).
func TestPlaySongIDWritesUnconditionally(t *testing.T) {
	p, _ := newTestPlayer(t)
	enc, _ := isaacPair([4]uint32{1, 2, 3, 4})
	p.client.encryptor = enc
	p.PlaySong(7)
	// 1-byte encrypted opcode + p2(id) = 3 bytes buffered.
	if n := p.client.bufw.Buffered(); n != 3 {
		t.Errorf("PlaySong(7) buffered %d bytes; want 3 (opcode + p2 id)", n)
	}
}

// TestPlayJingleIDNilServerZeroLength pins the rev-254 contract: PlayJingle
// takes a TRACK ID; the delay field is Midi.getLength(id) (TS
// Player.ts:1989-1991). With no server (no Midi cache) the length degrades
// to 0 — the packet is still written. Supersedes
// TestPlayJingleNoServer244SilentNoOp.
func TestPlayJingleIDNilServerZeroLength(t *testing.T) {
	p, _ := newTestPlayer(t)
	enc, _ := isaacPair([4]uint32{1, 2, 3, 4})
	p.client.encryptor = enc
	p.PlayJingle(7)
	// 1-byte encrypted opcode + p2(id) + p2(length=0) = 5 bytes buffered.
	if n := p.client.bufw.Buffered(); n != 5 {
		t.Errorf("PlayJingle(7) buffered %d bytes; want 5 (opcode + p2 id + p2 len)", n)
	}
}

// TestPlayJingleUsesMidiCacheLength pins that the delay field is the
// track length in MILLISECONDS from the server's Midi cache — TS
// Player.ts:1989-1991 @2e3bcf43 `new MidiJingle(id, Midi.getLength(id))`
// with MidiJingleEncoder p2(id) p2(delay).
func TestPlayJingleUsesMidiCacheLength(t *testing.T) {
	s := newTestServer(t)
	p, cc := newTestPlayer(t)
	p.client.server = s
	enc, _ := isaacPair([4]uint32{1, 2, 3, 4})
	p.client.encryptor = enc
	s.midi = midi.NewCache([]int{0, 0, 1234}) // track 2 → 1234ms

	received := drainConn(t, cc)
	p.PlayJingle(2)
	p.client.flushWrite()
	emitted := <-received

	if len(emitted) != 5 {
		t.Fatalf("emitted %d bytes; want 5 (opcode + p2 id + p2 len)", len(emitted))
	}
	// Payload is plaintext (only the opcode byte is ISAAC-shifted):
	// p2(id=2) p2(length=1234ms=0x04D2).
	want := []byte{0x00, 0x02, 0x04, 0xd2}
	if !bytes.Equal(emitted[1:], want) {
		t.Errorf("payload: got % 02x, want % 02x", emitted[1:], want)
	}
}
