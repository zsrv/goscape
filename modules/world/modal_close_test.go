package world

import (
	"testing"

	io2 "github.com/zsrv/goscape/pkg/io/isaac"
)

func TestModalCloseEmitsStopTransmit(t *testing.T) {
	p, cc := newTestPlayer(t)
	p.client.encryptor = io2.New([4]uint32{1, 2, 3, 4})
	p.invListeners = []InventoryListener{
		{Type: 93, Com: 149, Source: 2, FirstSeen: false},
		{Type: 93, Com: 150, Source: -1, FirstSeen: false},
	}
	p.refreshModalClose = true

	received := drainConn(t, cc)
	p.encodeOut()
	p.client.flushWrite()

	got := <-received
	// Expected wire:
	//   1 byte IfClose (opcode, no payload)
	//   + 2 * 3 bytes UpdateInvStopTransmit (1 opcode + 2 payload)
	// Total = 1 + 6 = 7 bytes.
	if len(got) != 7 {
		t.Errorf("got %d bytes, want 7 (IfClose + 2× StopTransmit); bytes=%v", len(got), got)
	}
	if len(p.invListeners) != 0 {
		t.Errorf("invListeners should be cleared; got %d", len(p.invListeners))
	}
}

func TestNoStopTransmitWithoutModalClose(t *testing.T) {
	p, cc := newTestPlayer(t)
	p.client.encryptor = io2.New([4]uint32{1, 2, 3, 4})
	p.invListeners = []InventoryListener{
		{Type: 93, Com: 149, Source: 2, FirstSeen: false},
	}
	p.refreshModalClose = false

	received := drainConn(t, cc)
	p.encodeOut()
	p.client.flushWrite()

	got := <-received
	if len(got) != 0 {
		t.Errorf("no modal close → no stop-transmit; got %d bytes", len(got))
	}
	if len(p.invListeners) != 1 {
		t.Errorf("invListeners should be untouched; got %d", len(p.invListeners))
	}
}
