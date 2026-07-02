package world

import (
	"testing"
	"time"
)

func TestRemovePlayerOnTick_FiresFriendsPlayerLogout(t *testing.T) {
	s := newTestServer(t)
	s.cfg.NodeID = 10
	s.cfg.NodeProfile = "main"
	fake := newFakeFriendsClient()
	s.friendsClient = fake
	s.invTypes = nil // PlayerLogout doesn't need it (login side does p.Save)

	c, _ := newTestClient(t)
	p := newPlayer(c)
	p.username = "alice"
	p.username37 = 1234
	if err := s.addPlayer(p); err != nil {
		t.Fatalf("addPlayer: %v", err)
	}

	s.removePlayerOnTick(p)

	select {
	case got := <-fake.playerLogoutReqs:
		if got.WorldId != 10 {
			t.Errorf("WorldId: got %d, want 10", got.WorldId)
		}
		if got.Username37 != 1234 {
			t.Errorf("Username37: got %d, want 1234", got.Username37)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for PlayerLogout RPC")
	}
}

func TestRemovePlayerOnDisconnect_FiresFriendsPlayerLogout(t *testing.T) {
	s := newTestServer(t)
	s.cfg.NodeID = 10
	s.cfg.NodeProfile = "main"
	fake := newFakeFriendsClient()
	s.friendsClient = fake

	c, _ := newTestClient(t)
	p := newPlayer(c)
	p.username = "alice"
	p.username37 = 1234
	if err := s.addPlayer(p); err != nil {
		t.Fatalf("addPlayer: %v", err)
	}

	s.removePlayerOnDisconnect(p) // enqueues removePlayerOnTick on the removal queue
	s.drainRemovals()             // run it on-tick → fires the friends PlayerLogout

	select {
	case got := <-fake.playerLogoutReqs:
		if got.WorldId != 10 {
			t.Errorf("WorldId: got %d, want 10", got.WorldId)
		}
		if got.Username37 != 1234 {
			t.Errorf("Username37: got %d, want 1234", got.Username37)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for PlayerLogout RPC")
	}
}

func TestRemovePlayerOnTick_NilFriendsClient_NoOp(t *testing.T) {
	s := newTestServer(t)
	s.friendsClient = nil // explicit
	s.invTypes = nil

	c, _ := newTestClient(t)
	p := newPlayer(c)
	p.username = "alice"
	if err := s.addPlayer(p); err != nil {
		t.Fatalf("addPlayer: %v", err)
	}
	// Must not panic.
	s.removePlayerOnTick(p)
}

func TestRemovePlayerOnDisconnect_EmptyUsername_NoOp(t *testing.T) {
	s := newTestServer(t)
	fake := newFakeFriendsClient()
	s.friendsClient = fake

	c, _ := newTestClient(t)
	p := newPlayer(c)
	p.username = "" // unauthenticated disconnect
	if err := s.addPlayer(p); err != nil {
		t.Fatalf("addPlayer: %v", err)
	}
	s.removePlayerOnDisconnect(p)

	select {
	case got := <-fake.playerLogoutReqs:
		t.Errorf("unexpected PlayerLogout RPC fired: %+v", got)
	case <-time.After(50 * time.Millisecond):
		// expected: no RPC fired
	}
}
