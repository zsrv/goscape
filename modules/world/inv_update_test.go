package world

import (
	"net"
	"testing"

	"github.com/zsrv/goscape/pkg/inventory"
	io2 "github.com/zsrv/goscape/pkg/io/isaac"
)

func newInvListenerTestPlayer(t *testing.T, s *Server, slot int) (*Player, net.Conn) {
	t.Helper()
	p, cc := newTestPlayer(t)
	p.client.server = s
	p.client.encryptor = io2.New([4]uint32{1, 2, 3, 4})
	p.slot = slot
	p.invs = map[int]*inventory.Inventory{}
	s.players[slot] = p
	return p, cc
}

func TestUpdateInvsFirstSeenFires(t *testing.T) {
	s := newTestServer(t)
	s.invs = make(map[int]*inventory.Inventory)

	owner, _ := newInvListenerTestPlayer(t, s, 2)
	inv := inventory.New(93, 28, inventory.StackNormal)
	inv.Update = false // first-seen listener should override dirty==false.
	owner.invs[93] = inv

	viewer, vcc := newInvListenerTestPlayer(t, s, 3)
	viewer.invListeners = []InventoryListener{
		{Type: 93, Com: 149, Source: 2, FirstSeen: true},
	}

	received := drainConn(t, vcc)
	viewer.updateInvs()
	viewer.client.flushWrite()
	got := <-received
	if len(got) == 0 {
		t.Fatal("FirstSeen should fire a packet; got none")
	}
	if viewer.invListeners[0].FirstSeen {
		t.Error("FirstSeen should flip to false after first send")
	}
}

func TestUpdateInvsRespectsDirty(t *testing.T) {
	s := newTestServer(t)
	s.invs = make(map[int]*inventory.Inventory)

	owner, _ := newInvListenerTestPlayer(t, s, 2)
	inv := inventory.New(93, 28, inventory.StackNormal)
	owner.invs[93] = inv

	viewer, vcc := newInvListenerTestPlayer(t, s, 3)
	viewer.invListeners = []InventoryListener{
		{Type: 93, Com: 149, Source: 2, FirstSeen: false},
	}
	inv.Update = false

	received := drainConn(t, vcc)
	viewer.updateInvs()
	viewer.client.flushWrite()
	quiet := <-received
	if len(quiet) != 0 {
		t.Errorf("clean listener should emit nothing; got %d bytes", len(quiet))
	}

	inv.Update = true
	received2 := drainConn(t, vcc)
	viewer.updateInvs()
	viewer.client.flushWrite()
	loud := <-received2
	if len(loud) == 0 {
		t.Error("dirty inv should fire a packet; got none")
	}
	if inv.Update {
		t.Error("inv.Update should be cleared after the tick")
	}
}

func TestUpdateInvsWorldSource(t *testing.T) {
	s := newTestServer(t)
	s.invs = make(map[int]*inventory.Inventory)
	s.invs[0] = inventory.New(0, 1, inventory.StackAlways)

	viewer, vcc := newInvListenerTestPlayer(t, s, 3)
	viewer.invListeners = []InventoryListener{
		{Type: 0, Com: 200, Source: -1, FirstSeen: true},
	}

	received := drainConn(t, vcc)
	viewer.updateInvs()
	viewer.client.flushWrite()
	got := <-received
	if len(got) == 0 {
		t.Error("world-source listener should fire on FirstSeen")
	}
}

func TestUpdateInvsSkipsMissingSource(t *testing.T) {
	s := newTestServer(t)
	s.invs = make(map[int]*inventory.Inventory)

	viewer, vcc := newInvListenerTestPlayer(t, s, 3)
	// source=99 doesn't exist in s.players.
	viewer.invListeners = []InventoryListener{
		{Type: 93, Com: 149, Source: 99, FirstSeen: true},
	}

	received := drainConn(t, vcc)
	viewer.updateInvs()
	viewer.client.flushWrite()
	got := <-received
	if len(got) != 0 {
		t.Errorf("missing source should be skipped silently; got %d bytes", len(got))
	}
}
