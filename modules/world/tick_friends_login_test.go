package world

import (
	"io"
	"testing"
	"time"

	io2 "github.com/zsrv/goscape/pkg/io/isaac"
)

func TestProcessLogins_FiresFriendsPlayerLogin(t *testing.T) {
	s := newTestServer(t)
	s.cfg.NodeID = 10
	s.cfg.NodeProfile = "main"
	fake := newFakeFriendsClient()
	s.friendsClient = fake

	c, conn := newTestClient(t)
	c.encryptor = io2.New([4]uint32{1, 2, 3, 4})
	go io.Copy(io.Discard, conn)
	p := newPlayer(c)
	p.username = "alice"
	p.username37 = 1234
	p.staffModLevel = 2
	p.privateChat = 1
	s.appendNewPlayer(p)

	s.processLogins()

	select {
	case got := <-fake.playerLoginReqs:
		if got.WorldId != 10 {
			t.Errorf("WorldId: got %d, want 10", got.WorldId)
		}
		if got.Username37 != 1234 {
			t.Errorf("Username37: got %d, want 1234", got.Username37)
		}
		if got.StaffLvl != 2 {
			t.Errorf("StaffLvl: got %d, want 2", got.StaffLvl)
		}
		if got.PrivateChat != 1 {
			t.Errorf("PrivateChat: got %d, want 1", got.PrivateChat)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for PlayerLogin RPC")
	}
}

func TestProcessLogins_NilFriendsClient_NoPanic(t *testing.T) {
	s := newTestServer(t)
	s.friendsClient = nil

	c, conn := newTestClient(t)
	c.encryptor = io2.New([4]uint32{1, 2, 3, 4})
	go io.Copy(io.Discard, conn)
	p := newPlayer(c)
	p.username = "alice"
	p.username37 = 1234
	s.appendNewPlayer(p)

	// Must not panic.
	s.processLogins()
}

func TestProcessLogins_EmptyUsername_NoFriendsRPC(t *testing.T) {
	s := newTestServer(t)
	fake := newFakeFriendsClient()
	s.friendsClient = fake

	c, conn := newTestClient(t)
	c.encryptor = io2.New([4]uint32{1, 2, 3, 4})
	go io.Copy(io.Discard, conn)
	p := newPlayer(c)
	p.username = "" // pathological: bypassed login auth
	p.username37 = 0
	s.appendNewPlayer(p)

	s.processLogins()

	select {
	case got := <-fake.playerLoginReqs:
		t.Errorf("unexpected PlayerLogin RPC fired for empty username: %+v", got)
	case <-time.After(50 * time.Millisecond):
		// expected: no RPC fired
	}
}
