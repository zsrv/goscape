package world

import (
	"io"
	"testing"
	"time"

	gameserver "github.com/zsrv/goscape/pkg/io/protocol/game/server"
)

func TestSendRebuildNormalWireFormat(t *testing.T) {
	enc, _ := isaacPair([4]uint32{1, 2, 3, 4})
	wantEnc, _ := isaacPair([4]uint32{1, 2, 3, 4})

	p, clientConn := newTestPlayer(t)
	p.client.encryptor = enc
	p.client.server = newTestServer(t)
	p.x = 3094
	p.z = 3106

	mapsquares := []uint16{uint16((48 << 8) | 48)}

	received := make(chan []byte, 1)
	go func() {
		buf := make([]byte, 17)
		clientConn.SetReadDeadline(time.Now().Add(time.Second))
		if _, err := io.ReadFull(clientConn, buf); err == nil {
			received <- buf
		}
	}()

	sendRebuildNormal(p, mapsquares)
	p.client.flushWrite()

	expectedOpcode := byte((int(gameserver.OpRebuildNormal.Opcode) + int(wantEnc.GetNext())) & 0xff)

	select {
	case got := <-received:
		if got[0] != expectedOpcode {
			t.Errorf("opcode byte: got %d, want %d", got[0], expectedOpcode)
		}
		if got[1] != 0 || got[2] != 14 {
			t.Errorf("len prefix: got %v, want [0 14]", got[1:3])
		}
		if got[3] != 0x01 || got[4] != 0x82 {
			t.Errorf("zoneX: got %v, want [0x01 0x82]", got[3:5])
		}
		if got[5] != 0x01 || got[6] != 0x84 {
			t.Errorf("zoneZ: got %v, want [0x01 0x84]", got[5:7])
		}
		if got[7] != 48 || got[8] != 48 {
			t.Errorf("mapsquare: got (%d,%d), want (48,48)", got[7], got[8])
		}
	case <-time.After(time.Second):
		t.Error("timed out")
	}
}
