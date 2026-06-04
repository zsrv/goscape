package world

import (
	"bytes"
	"io"
	"testing"
	"time"

	gameserver "github.com/zsrv/goscape/pkg/io/protocol/game/server"
)

// TestSendRebuildNormal244WireFormat pins the 244 wire shape:
// fixed-4 payload = p2(zoneX) p2(zoneZ), no length prefix, no mapsquare
// CRC loop. TS RebuildNormalEncoder.ts (244): encode writes p2(zoneX)
// p2(zoneZ) only; test() returns 4.
func TestSendRebuildNormal244WireFormat(t *testing.T) {
	enc, _ := isaacPair([4]uint32{1, 2, 3, 4})
	wantEnc, _ := isaacPair([4]uint32{1, 2, 3, 4})

	p, clientConn := newTestPlayer(t)
	p.client.encryptor = enc
	p.client.server = newTestServer(t)
	// p.x=3094, p.z=3106 → zoneX=3094>>3=386=0x0182, zoneZ=3106>>3=388=0x0184
	p.x = 3094
	p.z = 3106

	received := make(chan []byte, 1)
	go func() {
		buf := make([]byte, 5) // opcode(1) + p2(zoneX)(2) + p2(zoneZ)(2) = 5
		clientConn.SetReadDeadline(time.Now().Add(time.Second))
		if _, err := io.ReadFull(clientConn, buf); err == nil {
			received <- buf
		}
	}()

	sendRebuildNormal(p)
	p.client.flushWrite()

	expectedOpcode := byte((int(gameserver.OpRebuildNormal.Opcode) + int(wantEnc.GetNext())) & 0xff)

	select {
	case got := <-received:
		if got[0] != expectedOpcode {
			t.Errorf("opcode byte: got %d, want %d", got[0], expectedOpcode)
		}
		// No length prefix for fixed-4 payloads.
		// zoneX = 3094>>3 = 386 = 0x0182
		if got[1] != 0x01 || got[2] != 0x82 {
			t.Errorf("zoneX: got %v, want [0x01 0x82]", got[1:3])
		}
		// zoneZ = 3106>>3 = 388 = 0x0184
		if got[3] != 0x01 || got[4] != 0x84 {
			t.Errorf("zoneZ: got %v, want [0x01 0x84]", got[3:5])
		}
	case <-time.After(time.Second):
		t.Error("timed out")
	}
}

// TestSendRebuildNormal244ExactBytes pins the exact byte sequence using
// drainConn so the test does not depend on packet framing knowledge.
func TestSendRebuildNormal244ExactBytes(t *testing.T) {
	enc, _ := isaacPair([4]uint32{1, 2, 3, 4})
	wantEnc, _ := isaacPair([4]uint32{1, 2, 3, 4})

	p, cc := newTestPlayer(t)
	p.client.encryptor = enc
	p.client.server = newTestServer(t)
	p.x = 3094
	p.z = 3106

	// zoneX=386=0x0182, zoneZ=388=0x0184; total = opcode(1) + 4 = 5 bytes.
	want := []byte{
		byte((int(gameserver.OpRebuildNormal.Opcode) + int(wantEnc.GetNext())) & 0xff),
		0x01, 0x82, // zoneX big-endian
		0x01, 0x84, // zoneZ big-endian
	}

	received := drainConn(t, cc)
	sendRebuildNormal(p)
	p.client.flushWrite()

	got := <-received
	if !bytes.Equal(got, want) {
		t.Errorf("wire: got % 02x, want % 02x", got, want)
	}
}
