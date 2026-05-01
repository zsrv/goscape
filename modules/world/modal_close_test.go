package world

import (
	"testing"

	io2 "github.com/zsrv/goscape/pkg/io/isaac"
	"github.com/zsrv/goscape/pkg/script"
)

func TestModalCloseEmitsStopTransmit(t *testing.T) {
	p, cc := newTestPlayer(t)
	p.client.encryptor = io2.New([4]uint32{1, 2, 3, 4})
	p.invListeners = map[int]InventoryListener{
		149: {Type: 93, Com: 149, Source: 2, FirstSeen: false},
		150: {Type: 93, Com: 150, Source: -1, FirstSeen: false},
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
	p.invListeners = map[int]InventoryListener{
		149: {Type: 93, Com: 149, Source: 2, FirstSeen: false},
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

// TestCloseModalClearsWeakQueueWhenTrue pins CloseModal(true) drops weak
// queue entries. Mirrors TS Player.closeModal default arg path
// (Player.ts:742-744).
func TestCloseModalClearsWeakQueueWhenTrue(t *testing.T) {
	p, _ := newTestPlayer(t)
	sf := &script.ScriptFile{Name: "stub"}
	p.queue = []playerQueueRequest{
		{Script: sf, Type: script.QueueStrong},
		{Script: sf, Type: script.QueueWeak},
	}

	p.CloseModal(true)

	if got, want := len(p.queue), 1; got != want {
		t.Fatalf("queue len: got %d, want %d (weak should be dropped)", got, want)
	}
	if p.queue[0].Type != script.QueueStrong {
		t.Errorf("queue[0].Type: got %v, want QueueStrong", p.queue[0].Type)
	}
}

// TestCloseModalPreservesWeakQueueWhenFalse pins CloseModal(false)
// preserves weak queue entries. Mirrors TS Player.closeModal(false)
// path (Player.ts:2148 caller).
func TestCloseModalPreservesWeakQueueWhenFalse(t *testing.T) {
	p, _ := newTestPlayer(t)
	sf := &script.ScriptFile{Name: "stub"}
	p.queue = []playerQueueRequest{
		{Script: sf, Type: script.QueueStrong},
		{Script: sf, Type: script.QueueWeak},
	}

	p.CloseModal(false)

	if got, want := len(p.queue), 2; got != want {
		t.Fatalf("queue len: got %d, want %d (weak should be preserved)", got, want)
	}
}
