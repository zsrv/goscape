package world

import (
	"net"
	"testing"
	"time"
)

// TestShouldFlushOnTeardown pins arch-29.1's flush-skip decision (extracted
// from handleTCPConn's teardown defer into client.shouldFlushOnTeardown so
// it can be tested directly): a pre-login connection that never reached
// ClientStateOndemand should flush; a connection that has (regardless of
// whether c.player also somehow got set — the caller's c.player != nil
// branch takes priority in practice, but the helper's own contract must
// still hold in isolation) or that already has a player attached should
// not.
//
// Investigated driving a real op-15 handshake through HandleConn first
// (this task's report has the detail): flushWrite's underlying
// bufio.Writer.Flush is a no-op on an empty buffer, and every OnDemand
// write in production (op-15's own reply, every clientODAdapter.send)
// already self-flushes immediately — so c.bufw is always empty by the
// time real teardown runs in the OnDemand state, making "did the
// underlying conn.Write get called" unobservable regardless of whether the
// guard is present, deleted, or inverted. Testing the extracted decision
// directly is the only way to actually pin the branch.
func TestShouldFlushOnTeardown(t *testing.T) {
	newTestClientForTeardown := func(t *testing.T) *client {
		t.Helper()
		_, server := net.Pipe()
		t.Cleanup(func() { server.Close() })
		return newClient(server, time.Second, discardLogger())
	}

	tests := []struct {
		name       string
		state      ClientState
		attachPlyr bool
		want       bool
	}{
		{name: "pre-login, still in login state", state: ClientStateLogin, attachPlyr: false, want: true},
		{name: "pre-login, transitioned to OnDemand (op-15)", state: ClientStateOndemand, attachPlyr: false, want: false},
		{name: "post-login, in game state", state: ClientStateGame, attachPlyr: true, want: false},
		{name: "player attached despite OnDemand state (defensive)", state: ClientStateOndemand, attachPlyr: true, want: false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			c := newTestClientForTeardown(t)
			c.state = tc.state
			if tc.attachPlyr {
				c.player = newPlayer(c)
			}
			if got := c.shouldFlushOnTeardown(); got != tc.want {
				t.Errorf("shouldFlushOnTeardown() with state=%v player-attached=%v: got %v, want %v",
					tc.state, tc.attachPlyr, got, tc.want)
			}
		})
	}
}
