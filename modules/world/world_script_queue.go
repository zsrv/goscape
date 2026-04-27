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

// processWorldQueue drains ready entries from s.worldScriptQueue,
// firing each by calling script.Execute (via resumeOrFinishWorld) and
// dispatching the post-execute state.
//
// Iteration uses index-based slice walk with mid-pass append visibility
// (re-reads len(s.worldScriptQueue) each loop iteration) — this
// preserves the same TS-authentic "speedup quirk" already present
// in processPlayerQueue (tick.go:222) where a script that re-enqueues
// itself or another script during Execute will see the new entry
// processed in the same tick.
//
// Removal happens BEFORE firing (matching processPlayerQueue:243-249)
// so a re-entrant Execute that calls EnqueueWorldScript doesn't
// collide with the index pointer.
//
// Mirrors TS World.processWorld world-queue iteration at World.ts:534-559.
//
// DEVIATION NAI-37-D-WORLDQUEUE-NO-PANIC-RECOVERY: TS wraps the
// world-queue iteration body in try/catch (World.ts:557-559) to
// swallow per-script panics. goscape leaves panics to propagate up
// the tick goroutine — closure when the project adopts a tick-wide
// panic-recovery convention.
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
		s.worldScriptQueue = append(s.worldScriptQueue[:i], s.worldScriptQueue[i+1:]...)
		// Reset Execution=Running so script.Execute resumes the loop
		// from the post-WORLD_DELAY PC. Mirrors the player-path resume
		// convention at tick.go:211. TS ScriptRunner.execute resets
		// internally (ScriptRunner.ts:130); goscape leaves the reset to
		// callers, matching processActiveScripts.
		state.Execution = script.Running
		s.resumeOrFinishWorld(state)
		// Don't advance i: we just removed the current element, so i
		// now points to what was the next element (or past end).
	}
}
