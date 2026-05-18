package world

import (
	"testing"
	"time"
)

// TestRemovePlayerOnTick_FiresPlayerLogoutWithSave pins server.go:897-917 —
// the graceful-logout RPC site. The captured PlayerLogoutRequest must
// carry NodeID + Profile from s.cfg, the player's username, and a
// VerifySave-able Save payload produced by p.Save(s.invTypes).
func TestRemovePlayerOnTick_FiresPlayerLogoutWithSave(t *testing.T) {
	fake := newFakeLoginClient()
	s := newTestServer(t)
	s.cfg.NodeID = 42
	s.cfg.NodeProfile = "main"
	s.loginClient = fake
	_, invTypes := newTestPlayerForLoadSave(t)
	s.invTypes = invTypes

	c, _ := newTestClient(t)
	c.server = s
	p := newPlayer(c)
	p.username = "alice"
	if err := s.addPlayer(p); err != nil {
		t.Fatalf("addPlayer: %v", err)
	}

	s.removePlayerOnTick(p)

	select {
	case <-fake.playerLogoutFired:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for PlayerLogout RPC")
	}

	got := fake.snapshotPlayerLogoutReq()
	if got == nil {
		t.Fatal("no PlayerLogoutRequest captured")
	}
	if got.NodeId != 42 || got.Profile != "main" {
		t.Errorf("server-cfg fields: got NodeId=%d Profile=%q; want 42 main",
			got.NodeId, got.Profile)
	}
	if got.Username != "alice" {
		t.Errorf("Username: got %q, want alice", got.Username)
	}
	if len(got.Save) == 0 {
		t.Error("Save must be non-empty (Player.Save bytes)")
	}
	// Verify the captured Save bytes are a structurally-valid SAV.
	// Plan said Verify; the actual API is VerifySave bool (player_load.go:43).
	if !VerifySave(got.Save) {
		t.Errorf("captured Save fails VerifySave; bytes=%d", len(got.Save))
	}
}

// TestRemovePlayerOnTick_NoLoginClient_NoRPC pins the s.loginClient==nil
// guard at server.go:898. removePlayerInternal must still run.
func TestRemovePlayerOnTick_NoLoginClient_NoRPC(t *testing.T) {
	s := newTestServer(t)
	// loginClient stays nil.

	c, _ := newTestClient(t)
	c.server = s
	p := newPlayer(c)
	p.username = "alice"
	if err := s.addPlayer(p); err != nil {
		t.Fatalf("addPlayer: %v", err)
	}

	s.removePlayerOnTick(p) // must not panic
	// player slot cleared (removePlayerInternal still runs).
	if s.players[p.slot] != nil {
		t.Error("removePlayerInternal must still run when loginClient is nil")
	}
}

// TestRemovePlayerOnTick_EmptyUsername_NoRPC pins the p.username==""
// guard at server.go:898. Empty username short-circuits before
// p.Save and before the RPC goroutine.
func TestRemovePlayerOnTick_EmptyUsername_NoRPC(t *testing.T) {
	fake := newFakeLoginClient()
	s := newTestServer(t)
	s.loginClient = fake

	c, _ := newTestClient(t)
	c.server = s
	p := newPlayer(c)
	// p.username stays empty.
	if err := s.addPlayer(p); err != nil {
		t.Fatalf("addPlayer: %v", err)
	}

	s.removePlayerOnTick(p)

	// No PlayerLogout should fire.
	select {
	case <-fake.playerLogoutFired:
		t.Fatal("PlayerLogout fired despite empty username")
	case <-time.After(100 * time.Millisecond):
		// expected — no RPC
	}
	if got := fake.snapshotPlayerLogoutReq(); got != nil {
		t.Errorf("PlayerLogoutRequest captured despite empty username: %+v", got)
	}
}
