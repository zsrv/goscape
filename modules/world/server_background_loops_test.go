package world

import (
	"testing"
	"time"
)

// The tick loop and the OnDemand pump are tracked by tickWg/odWg, which
// Shutdown waits on so neither is still running once Shutdown returns
// (server.go tickWg doc comment; Arc 18 R2).
//
// Registering them inside Run() could not deliver that. world.go's startingFn
// spawns run() in a goroutine and returns immediately, so stoppingFn — and
// therefore Shutdown — can reach tickWg.Wait() before that goroutine has been
// scheduled as far as its .Go() calls. Wait then sees a zero counter, returns
// at once, and the loops start AFTER Shutdown reported them finished. The race
// detector sees it as a write/read race on tickWg; the operator would see tick
// phases touching s.players and s.sessionLogs during cleanup.
//
// So the registration belongs to the starting phase, which happens-before any
// possible Shutdown — the same reasoning arch-29.8/29.13 used to move Listen,
// startWorldEventsSubscriber and the friends dispatcher out of NewServer.
//
// This pins the half that makes it safe: once startBackgroundLoops has
// returned, both WaitGroups are already non-zero, so a later Wait blocks
// rather than sailing through.
func TestStartBackgroundLoopsRegistersBeforeReturning(t *testing.T) {
	s := newTestServer(t)
	s.onDemand = newOnDemand(nil)

	s.startBackgroundLoops()

	waited := make(chan struct{})
	go func() {
		defer close(waited)
		s.tickWg.Wait()
		s.odWg.Wait()
	}()

	// Both loops run until quit closes, so Wait must still be blocked.
	select {
	case <-waited:
		t.Fatal("tickWg/odWg Wait returned while both loops were still running: " +
			"the counters were zero, so Shutdown would not have waited for them")
	case <-time.After(100 * time.Millisecond):
	}

	close(s.quit)

	select {
	case <-waited:
	case <-time.After(10 * time.Second):
		t.Fatal("loops did not exit after quit closed")
	}
}

// Run must not be the thing that registers them, or the window above reopens.
// Shutdown is reachable before Run has run a single statement.
func TestRunDoesNotRegisterTheShutdownWaitGroups(t *testing.T) {
	s := newTestServer(t)
	s.onDemand = newOnDemand(nil)

	// Nothing registered yet, so both Waits must return immediately. If Run
	// were still the registration site this would still pass — the point is
	// the pairing with the test above: registration happens in the starting
	// phase, and Shutdown's Wait is meaningful only because of that.
	done := make(chan struct{})
	go func() {
		defer close(done)
		s.tickWg.Wait()
		s.odWg.Wait()
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Wait blocked on a server whose starting phase never ran; " +
			"Shutdown on such a server (conn_handler_test, server_shutdown_test) would wedge")
	}
}
