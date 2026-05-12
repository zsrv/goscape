package world

import (
	"bytes"
	"testing"

	io2 "github.com/zsrv/goscape/pkg/io/isaac"
	gameserver "github.com/zsrv/goscape/pkg/io/protocol/game/server"
)

// TestSendUpdatePid_EmitsExactByteSequence pins the wire bytes of sendUpdatePid. NAI-182 B1.
func TestSendUpdatePid_EmitsExactByteSequence(t *testing.T) {
	p, cc := newTestPlayer(t)
	enc, _ := isaacPair([4]uint32{1, 2, 3, 4})
	p.client.encryptor = io2.New([4]uint32{1, 2, 3, 4})

	want := []byte{
		byte((int(gameserver.OpUpdatePid.Opcode) + int(enc.GetNext())) & 0xff),
		0x12, 0x34,
	}

	received := drainConn(t, cc)
	p.slot = 0x1234
	sendUpdatePid(p, p.slot)
	p.client.flushWrite()

	got := <-received
	if !bytes.Equal(got, want) {
		t.Errorf("wire: got %v, want %v", got, want)
	}
}

// TestSendResetClientVarCache_EmitsOpcodeOnly pins the wire bytes of sendResetClientVarCache. NAI-182 B1.
func TestSendResetClientVarCache_EmitsOpcodeOnly(t *testing.T) {
	p, cc := newTestPlayer(t)
	enc, _ := isaacPair([4]uint32{1, 2, 3, 4})
	p.client.encryptor = io2.New([4]uint32{1, 2, 3, 4})

	want := []byte{
		byte((int(gameserver.OpResetClientVarCache.Opcode) + int(enc.GetNext())) & 0xff),
	}

	received := drainConn(t, cc)
	sendResetClientVarCache(p)
	p.client.flushWrite()

	got := <-received
	if !bytes.Equal(got, want) {
		t.Errorf("wire: got %v, want %v", got, want)
	}
}

// TestSendResetAnims_EmitsOpcodeOnly pins the wire bytes of sendResetAnims. NAI-182 B1.
func TestSendResetAnims_EmitsOpcodeOnly(t *testing.T) {
	p, cc := newTestPlayer(t)
	enc, _ := isaacPair([4]uint32{1, 2, 3, 4})
	p.client.encryptor = io2.New([4]uint32{1, 2, 3, 4})

	want := []byte{
		byte((int(gameserver.OpResetAnims.Opcode) + int(enc.GetNext())) & 0xff),
	}

	received := drainConn(t, cc)
	sendResetAnims(p)
	p.client.flushWrite()

	got := <-received
	if !bytes.Equal(got, want) {
		t.Errorf("wire: got %v, want %v", got, want)
	}
}
