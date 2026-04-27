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
// Mirrors TS World.enqueueScript at World.ts:1238.
//
// Note: delay parameter is the wakeup-tick value popped by the caller,
// not the queue-internal "ticks remaining" counter. processWorldQueue
// decrements the entry's delay each tick; when it hits 0 (after
// being decremented from a positive starting value), the entry fires.
func (s *Server) EnqueueWorldScript(state *script.ScriptState, delay int) {
	s.worldScriptQueue = append(s.worldScriptQueue, worldScriptQueueEntry{
		script: state,
		delay:  delay,
	})
}
