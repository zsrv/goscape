package world

import (
	"io"
	"net"
	"testing"
	"time"

	io2 "github.com/zsrv/goscape/pkg/io/isaac"
)

// newPendingLoginClient builds a client that has completed sendLoginOK: the
// tick's buffer ref is taken and the Player is queued in s.newPlayers, but
// processLogins has not registered it yet (pid still -1).
func newPendingLoginClient(t *testing.T, s *Server) *client {
	t.Helper()
	serverConn, clientConn := net.Pipe()
	t.Cleanup(func() { serverConn.Close(); clientConn.Close() })
	go io.Copy(io.Discard, clientConn)

	c := newClient(serverConn, time.Second, discardLogger())
	c.server = s
	c.state = ClientStateGame
	c.encryptor = io2.New([4]uint32{1, 2, 3, 4})
	p := newPlayer(c)
	c.player = p

	c.teardownRefs.Add(1) // sendLoginOK: tick co-owns the buffers
	s.appendNewPlayer(p)
	return c
}

// TestRemovePlayerOnTick_PendingLoginIsNoOp pins the TS
// World.removePlayer guard (`if (player.pid === -1) return;`,
// World.ts:1629-1632 @3c16994c) for a Player that is still only queued in
// s.newPlayers.
//
// Regression: drainRemovals runs at the top of the tick body and
// processLogins later in the same body, so a socket dying in that window
// delivered the removal while the Player was still pending.
// removePlayerInternal's pid-identity guard made the cleanup a no-op but
// dropTickRef still returned c.bufw/c.bufr/c.in to their pools; processLogins
// then registered the player anyway and the same tick's processClientsOut
// wrote into and flushed a bufio.Writer whose underlying writer had been
// Reset(nil) — panicking with a nil pointer dereference inside
// bufio.(*Writer).Flush, which killed the whole tick loop.
func TestRemovePlayerOnTick_PendingLoginIsNoOp(t *testing.T) {
	s := newTestServer(t)
	c := newPendingLoginClient(t, s)
	p := c.player

	// Socket dies before the tick ever saw the player: handleTCPConn's
	// teardown enqueues the removal and drops the conn's ref.
	s.removePlayerOnDisconnect(p)
	c.dropConnRef()

	// One tick body: removals drain first, then logins, then client-out.
	s.drainRemovals()

	if got := c.teardownRefs.Load(); got != 1 {
		t.Fatalf("teardownRefs after removing a pending login: got %d, want 1 "+
			"(the tick still owns the buffers; processLogins has not run yet)", got)
	}

	s.processLogins()
	s.processClientsOut() // panicked here before the fix
}

// TestRemovePlayerOnTick_PendingLoginSkipsSave pins the rest of the TS
// no-op: a Player queued in newPlayers has not run LoadSave yet
// (processLogins does that), so removing it must not serialize its blank
// default state — that state would travel to the login server as the
// account's final save, and on to updateHiscores.
func TestRemovePlayerOnTick_PendingLoginSkipsSave(t *testing.T) {
	s := newTestServer(t)
	fake := newFakeLoginClient()
	s.loginClient = fake
	c := newPendingLoginClient(t, s)
	c.username = "pending"
	c.player.username = "pending"

	s.removePlayerOnTick(c.player)
	s.saveWg.Wait()

	select {
	case <-fake.playerLogoutFired:
		t.Error("PlayerLogout fired for a player that never entered the world; " +
			"its save is blank default state (LoadSave runs in processLogins)")
	default:
	}
}

// TestRemovePlayerOnTick_ReleasesTickRefOnce pins the other side of the
// guard: once processLogins has registered the player, the ordinary removal
// still runs in full and drops the tick's buffer ref, so retaining the ref
// through the pending window does not leak it.
func TestRemovePlayerOnTick_ReleasesTickRefOnce(t *testing.T) {
	s := newTestServer(t)
	c := newPendingLoginClient(t, s)
	p := c.player

	s.processLogins()
	if !s.registered(p) {
		t.Fatal("processLogins did not register the queued player")
	}

	c.dropConnRef()
	s.removePlayerOnTick(p)

	if got := c.teardownRefs.Load(); got != 0 {
		t.Errorf("teardownRefs after the registered player was removed: got %d, want 0", got)
	}
	// Second removal (idle-logout and disconnect can both land on the same
	// player) must not double-drop or double-save.
	s.removePlayerOnTick(p)
	if got := c.teardownRefs.Load(); got != 0 {
		t.Errorf("teardownRefs after a duplicate removal: got %d, want 0", got)
	}
}

// TestProcessLogins_WorldFullReleasesTickRef pins the one path where a
// queued player never enters the world: the world-full rejection. No
// removal will ever run for it (removePlayerOnTick no-ops on an
// unregistered player), so processLogins itself owes the ref drop —
// otherwise the pooled bufio buffers are never returned.
func TestProcessLogins_WorldFullReleasesTickRef(t *testing.T) {
	s := newTestServer(t)
	c := newPendingLoginClient(t, s)

	// Fill every slot so addPlayer returns errWorldFull.
	s.playersMu.Lock()
	s.players = newPlayerList(1)
	s.playersMu.Unlock()

	c.dropConnRef()
	s.processLogins()

	if got := c.teardownRefs.Load(); got != 0 {
		t.Errorf("teardownRefs after a world-full rejection: got %d, want 0", got)
	}
}

// TestClearLogins_ReleasesTickRef pins the third and last exit a queued
// player has without ever being registered: the RELAY_CLEARLOGINS admin
// drain. Same invariant as the world-full rejection — whoever discards a
// pending player owes the tick's buffer ref, because removePlayerOnTick
// no-ops on an unregistered player.
func TestClearLogins_ReleasesTickRef(t *testing.T) {
	s := newTestServer(t)
	c := newPendingLoginClient(t, s)
	c.dropConnRef()

	s.ClearLogins()
	s.drainRelayActions()

	if got := c.teardownRefs.Load(); got != 0 {
		t.Errorf("teardownRefs after ClearLogins dropped the queued player: got %d, want 0", got)
	}
}
