package world

import (
	"github.com/zsrv/goscape/pkg/script"
)

// worldScriptQueueEntry is one suspended script awaiting its
// world-tick wakeup. delay decrements each tick by processWorldQueue;
// when it reaches 0 the script is removed and ScriptRunner.Execute
// is called via resumeOrFinishWorld, which dispatches the post-execute
// state.
type worldScriptQueueEntry struct {
	script *script.ScriptState
	delay  int
}

// EnqueueWorldScript appends a script to the world-script queue with
// the given wakeup-tick countdown. Called by:
//   - resumeOrFinish (player path, T10) when a player-bound script
//     returned WorldSuspended; the caller pops the wakeup-tick from
//     the script's int stack and passes it as delay.
//   - resumeOrFinishNpc (npc path, T11) — symmetric.
//   - processWorldQueue (world self-loop via resumeOrFinishWorld, T12)
//     when a world-queued script re-suspends with WorldSuspended.
//
// Mirrors TS World.enqueueScript at World.ts:1238-1240 — stored delay
// is user-delay + 1 so that processWorldQueue's post-decrement (capture
// current, decrement, check captured>0) fires at the TS-canonical tick.
// For user world_delay N this means the entry fires on the (N+2)-th
// processWorldQueue call after enqueue (delay=0 fires on the 2nd call).
func (s *Server) EnqueueWorldScript(state *script.ScriptState, delay int) {
	s.worldScriptQueue = append(s.worldScriptQueue, worldScriptQueueEntry{
		script: state,
		delay:  delay + 1, // mirror TS World.enqueueScript at World.ts:1239
	})
}

// processWorldQueue drains ready entries from s.worldScriptQueue, firing each
// via fireWorldScript (script.Execute through resumeOrFinishWorld) and
// dispatching the post-execute state.
//
// Iteration uses an index-based slice walk with mid-pass append visibility
// (re-reads len(s.worldScriptQueue) each iteration) — the TS-authentic
// "speedup quirk" (also in processPlayerQueue, tick.go:222) where a script
// that re-enqueues during Execute is processed the same tick.
//
// Fire-then-remove-on-clean: an entry is removed only after a non-panicking
// fire. On a panic the entry is LEFT queued so it retries next tick. This
// mirrors TS World.ts:542-558, where request.unlink() runs AFTER
// ScriptRunner.execute and a throw skips the unlink. Normal script errors are
// caught INSIDE execute (→ ABORTED → resumeOrFinishWorld returns cleanly →
// removed here); only a genuine Go panic takes the retry path. ARCH-1
// (closes NAI-37-D-WORLDQUEUE-NO-PANIC-RECOVERY).
//
// Re-entrancy: resumeOrFinishWorld's WorldSuspended branch appends a new entry
// via EnqueueWorldScript during a clean fire; the caller then removes the OLD
// entry at index i, and the new entry (at the tail) is visited later in the
// same pass. Re-entrant appends never collide with the [:i]/[i+1:] splice.
// state is captured before the fire (the slice may reallocate); the entry
// pointer is not used after the fire.
func (s *Server) processWorldQueue() {
	i := 0
	for i < len(s.worldScriptQueue) {
		entry := &s.worldScriptQueue[i]
		// POST-decrement: capture current, then decrement. Mirrors TS
		// World.processWorld at World.ts:535 (`const delay = request.delay--`).
		// With delay=delay+1 stored at enqueue, this means user world_delay N
		// fires on the (N+2)-th processWorldQueue call after suspend (matching TS).
		delay := entry.delay
		entry.delay--
		if delay > 0 {
			i++
			continue
		}
		state := entry.script
		// Reset Execution=Running so script.Execute resumes the loop from the
		// post-WORLD_DELAY PC. Mirrors the player-path resume convention at
		// tick.go:211. TS ScriptRunner.execute resets internally
		// (ScriptRunner.ts:130); goscape leaves the reset to callers.
		state.Execution = script.Running

		if s.fireWorldScript(state) {
			// Panicked: leave the entry queued so it retries next tick;
			// advance past it for the remainder of this pass.
			i++
			continue
		}
		// Clean return (incl. an inline-handled script error): remove.
		s.worldScriptQueue = append(s.worldScriptQueue[:i], s.worldScriptQueue[i+1:]...)
		// Don't advance i: we removed the current element, so i now points
		// to what was the next element (or past end).
	}
}

// fireWorldScript resumes one world-queued script under a recover. Returns
// panicked=true when execution panicked (the caller leaves the entry queued
// for a next-tick retry, mirroring TS World.ts where a throw skips unlink).
// A normal return — including an inline-handled script error that
// resumeOrFinishWorld logged and routed — yields panicked=false (caller
// removes the entry). ARCH-1.
func (s *Server) fireWorldScript(state *script.ScriptState) (panicked bool) {
	defer func() {
		if r := recover(); r != nil {
			panicked = true
			logWorldScriptPanic(state, r, s.log)
		}
	}()
	s.resumeOrFinishWorld(state)
	return false
}
