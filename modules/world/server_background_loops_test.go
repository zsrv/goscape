package world

import (
	"testing"
	"time"
)

// The tick loop is tracked by tickWg, which Shutdown waits on so it is not
// still running once Shutdown returns (server.go tickWg doc comment; Arc 18
// R2: "without this, Shutdown could return while runTickLoop was still
// executing tick phases").
//
// Registering it inside Run() could not deliver that. world.go's startingFn
// spawns run() in a goroutine and returns immediately, so stoppingFn — and
// therefore Shutdown — can reach tickWg.Wait() before that goroutine has been
// scheduled as far as its .Go() call. Wait then sees a zero counter, returns
// at once, and the tick loop starts AFTER Shutdown reported it finished, with
// tick phases free to touch s.players and s.sessionLogs during cleanup. The
// race detector reports it as a write/read race on tickWg.
//
// So the registration belongs to the starting phase, which happens-before any
// possible Shutdown — the same reasoning arch-29.8/29.13 used to move Listen,
// startWorldEventsSubscriber and the friends dispatcher out of NewServer.
//
// This pins the half that makes it safe: once startBackgroundLoops has
// returned, tickWg is already non-zero, so a later Wait blocks rather than
// sailing through.
//
// rev-225 has no OnDemand pump goroutine (and so no odWg) — that arrived with
// rev-274's signal-driven pump — so only the tick loop is covered here.
func TestStartBackgroundLoopsRegistersBeforeReturning(t *testing.T) {
	s := newTestServer(t)

	s.startBackgroundLoops()

	waited := make(chan struct{})
	go func() {
		defer close(waited)
		s.tickWg.Wait()
	}()

	// The tick loop runs until quit closes, so Wait must still be blocked.
	select {
	case <-waited:
		t.Fatal("tickWg.Wait returned while the tick loop was still running: " +
			"the counter was zero, so Shutdown would not have waited for it")
	case <-time.After(100 * time.Millisecond):
	}

	close(s.quit)

	select {
	case <-waited:
	case <-time.After(10 * time.Second):
		t.Fatal("tick loop did not exit after quit closed")
	}
}

// Shutdown must stay usable on a Server whose starting phase never ran, or the
// tests that construct a bare Server and call Shutdown directly
// (conn_handler_test.go, server_shutdown_test.go) would wedge.
func TestTickWgWaitDoesNotBlockWhenStartingPhaseNeverRan(t *testing.T) {
	s := newTestServer(t)

	done := make(chan struct{})
	go func() {
		defer close(done)
		s.tickWg.Wait()
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Wait blocked on a server whose starting phase never ran")
	}
}
