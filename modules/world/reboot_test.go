package world

import (
	"bytes"
	"testing"

	io2 "github.com/zsrv/goscape/pkg/io/isaac"
	gameserver "github.com/zsrv/goscape/pkg/io/protocol/game/server"
)

// TestSendUpdateRebootTimer_EmitsExactByteSequence pins the wire bytes of sendUpdateRebootTimer. NAI-182 B1.
func TestSendUpdateRebootTimer_EmitsExactByteSequence(t *testing.T) {
	p, cc := newTestPlayer(t)
	enc, _ := isaacPair([4]uint32{1, 2, 3, 4})
	p.client.encryptor = io2.New([4]uint32{1, 2, 3, 4})

	want := []byte{
		byte((int(gameserver.OpUpdateRebootTimer.Opcode) + int(enc.GetNext())) & 0xff),
		0x00, 0x32,
	}

	received := drainConn(t, cc)
	sendUpdateRebootTimer(p, 50)
	p.client.flushWrite()

	got := <-received
	if !bytes.Equal(got, want) {
		t.Errorf("wire: got %v, want %v", got, want)
	}
}

// TestSendUpdateRebootTimer_ZeroTicks pins the wire bytes of sendUpdateRebootTimer with ticks=0. NAI-182 B1.
func TestSendUpdateRebootTimer_ZeroTicks(t *testing.T) {
	p, cc := newTestPlayer(t)
	enc, _ := isaacPair([4]uint32{1, 2, 3, 4})
	p.client.encryptor = io2.New([4]uint32{1, 2, 3, 4})

	want := []byte{
		byte((int(gameserver.OpUpdateRebootTimer.Opcode) + int(enc.GetNext())) & 0xff),
		0x00, 0x00,
	}

	received := drainConn(t, cc)
	sendUpdateRebootTimer(p, 0)
	p.client.flushWrite()

	got := <-received
	if !bytes.Equal(got, want) {
		t.Errorf("wire: got %v, want %v", got, want)
	}
}
