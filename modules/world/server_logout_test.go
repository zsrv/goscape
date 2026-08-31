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

// TestRemovePlayerOnDisconnect_SavesViaTickLogout pins the disconnect-save
// fix: an ungraceful socket close defers removal to the tick (relay queue) so
// the player is SAVED on-tick via PlayerLogout, instead of the old
// PlayerForceLogout that discarded all progress since the last autosave.
func TestRemovePlayerOnDisconnect_SavesViaTickLogout(t *testing.T) {
	fake := newFakeLoginClient()
	s := newTestServer(t)
	s.cfg.NodeID = 7
	s.cfg.NodeProfile = "dev"
	s.loginClient = fake
	_, invTypes := newTestPlayerForLoadSave(t)
	s.invTypes = invTypes

	c, _ := newTestClient(t)
	c.server = s
	p := newPlayer(c)
	p.username = "bob"
	if err := s.addPlayer(p); err != nil {
		t.Fatalf("addPlayer: %v", err)
	}

	s.removePlayerOnDisconnect(p) // enqueues removePlayerOnTick on the removal queue
	s.drainRemovals()             // run it on-tick (race-free p.Save())

	select {
	case <-fake.playerLogoutFired:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for PlayerLogout RPC")
	}
	got := fake.snapshotPlayerLogoutReq()
	if got == nil {
		t.Fatal("no PlayerLogoutRequest captured — disconnect must save")
	}
	if got.NodeId != 7 || got.Profile != "dev" || got.Username != "bob" {
		t.Errorf("logout fields: got NodeId=%d Profile=%q Username=%q; want 7 dev bob",
			got.NodeId, got.Profile, got.Username)
	}
	if len(got.Save) == 0 {
		t.Error("disconnect PlayerLogout must carry a save blob — state must persist on disconnect")
	}
	// PlayerForceLogout (no save) must NOT fire on the disconnect path anymore.
	select {
	case <-fake.forceLogoutReqs:
		t.Error("PlayerForceLogout fired on disconnect; should use saving PlayerLogout")
	default:
	}
}

// TestSaveAllOnShutdown_SavesEveryOnlinePlayer pins symptom-3: when the server
// is stopped while players are still online, the tick's final save-all logs
// each of them out WITH a save (PlayerLogout), instead of dropping their
// progress. Each save is tracked by saveWg, which Shutdown waits on before
// cancelling bridgesCtx.
func TestSaveAllOnShutdown_SavesEveryOnlinePlayer(t *testing.T) {
	fake := newFakeLoginClient()
	s := newTestServer(t)
	s.loginClient = fake
	_, invTypes := newTestPlayerForLoadSave(t)
	s.invTypes = invTypes

	for _, name := range []string{"alice", "bob"} {
		c, _ := newTestClient(t)
		c.server = s
		p := newPlayer(c)
		p.username = name
		if err := s.addPlayer(p); err != nil {
			t.Fatalf("addPlayer(%s): %v", name, err)
		}
	}

	s.saveAllOnShutdown()

	if got := s.getTotalPlayers(); got != 0 {
		t.Errorf("players remaining after saveAllOnShutdown: %d, want 0", got)
	}
	// Two PlayerLogout (with-save) RPCs must fire — one per online player.
	for got := range 2 {
		select {
		case <-fake.playerLogoutFired:
		case <-time.After(2 * time.Second):
			t.Fatalf("only %d PlayerLogout RPCs fired, want 2", got)
		}
	}
	// saveWg must be fully drained (no leaked Add) once the RPCs complete.
	s.waitForSaveFlush() // returns promptly if drained; bounded otherwise
}

func TestRemovePlayerOnDisconnect_NoLoginClient_NoRPC(t *testing.T) {
	s := newTestServer(t)
	c, _ := newTestClient(t)
	c.server = s
	p := newPlayer(c)
	p.username = "bob"
	if err := s.addPlayer(p); err != nil {
		t.Fatalf("addPlayer: %v", err)
	}

	s.removePlayerOnDisconnect(p) // enqueues; must not panic
	s.drainRemovals()             // runs the removed player's removePlayerOnTick on-tick
	if s.players[p.slot] != nil {
		t.Error("removePlayerInternal must run (via the relayed tick logout) when loginClient is nil")
	}
}
