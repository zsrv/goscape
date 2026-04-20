package world

import (
	"bytes"
	"net"
	"testing"
	"time"

	gameserver "github.com/zsrv/goscape/pkg/io/protocol/game/server"
	io2 "github.com/zsrv/goscape/pkg/io/isaac"
)

func TestSendUpdateStatWireFormat(t *testing.T) {
	p, cc := newTestPlayer(t)
	enc, _ := isaacPair([4]uint32{1, 2, 3, 4})
	p.client.encryptor = io2.New([4]uint32{1, 2, 3, 4})

	// Expected wire:
	//   encrypted opcode = (44 + enc.GetNext()) & 0xff
	//   payload = p1(3) p4(100/10=10) p1(10) = [3, 0, 0, 0, 10, 10]
	want := []byte{
		byte((int(gameserver.OpUpdateStat.Opcode) + int(enc.GetNext())) & 0xff),
		3,
		0, 0, 0, 10,
		10,
	}

	received := drainConn(t, cc)
	sendUpdateStat(p, 3, 100, 10)
	p.client.flushWrite()

	got := <-received
	if !bytes.Equal(got, want) {
		t.Errorf("wire: got %v, want %v", got, want)
	}
}

func TestSendUpdateRunEnergyWireFormat(t *testing.T) {
	p, cc := newTestPlayer(t)
	enc, _ := isaacPair([4]uint32{1, 2, 3, 4})
	p.client.encryptor = io2.New([4]uint32{1, 2, 3, 4})

	want := []byte{
		byte((int(gameserver.OpUpdateRunEnergy.Opcode) + int(enc.GetNext())) & 0xff),
		100, // 10000 / 100
	}

	received := drainConn(t, cc)
	sendUpdateRunEnergy(p, 10000)
	p.client.flushWrite()

	got := <-received
	if !bytes.Equal(got, want) {
		t.Errorf("wire: got %v, want %v", got, want)
	}
}

// drainConn reads everything currently in the pipe. Must be called BEFORE flush.
func drainConn(t *testing.T, c net.Conn) <-chan []byte {
	t.Helper()
	received := make(chan []byte, 1)
	go func() {
		buf := make([]byte, 4096)
		c.SetReadDeadline(time.Now().Add(time.Second))
		n, _ := c.Read(buf)
		received <- buf[:n]
	}()
	return received
}
