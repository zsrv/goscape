package world

import (
	"bytes"
	"testing"

	io2 "github.com/zsrv/goscape/pkg/io/isaac"
	gameserver "github.com/zsrv/goscape/pkg/io/protocol/game/server"
)

func TestMessageGameWireFormat(t *testing.T) {
	p, cc := newTestPlayer(t)
	enc, _ := isaacPair([4]uint32{1, 2, 3, 4})
	p.client.encryptor = io2.New([4]uint32{1, 2, 3, 4})

	// MessageGame("hello") payload = PJStrNUL("hello") = 'h','e','l','l','o',0x00 (6 bytes).
	// Wire: [encrypted_opcode, len=6, 'h','e','l','l','o', 0x00].
	want := []byte{
		byte((int(gameserver.OpMessageGame.Opcode) + int(enc.GetNext())) & 0xff),
		6,
		'h', 'e', 'l', 'l', 'o', 0x00,
	}

	received := drainConn(t, cc)
	p.MessageGame("hello")
	p.client.flushWrite()
	got := <-received
	if !bytes.Equal(got, want) {
		t.Errorf("wire: got %v, want %v", got, want)
	}
}

func TestUsernameReturnsField(t *testing.T) {
	p, _ := newTestPlayer(t)
	p.username = "alice"
	if got := p.Username(); got != "alice" {
		t.Errorf("Username: got %q, want %q", got, "alice")
	}
}
