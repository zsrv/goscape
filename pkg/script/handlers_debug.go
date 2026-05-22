package script

import (
	"fmt"
	"time"
)

// handleError aborts the script with a scripted error message. The
// error propagates up to runScript which logs it; Execution is set to
// Aborted by the dispatch loop.
func handleError(s *ScriptState) error {
	msg := s.PopString()
	return fmt.Errorf("ERROR: %s", msg)
}

// handleTimeSpent starts the script-side stopwatch by recording the
// current monotonic time in state.Timespent. Mirrors TS DebugOps.ts:13
// `state.timespent = performance.now()`. No active-player gate — TS
// doesn't require one, and the stopwatch is per-ScriptState not per-player.
func handleTimeSpent(s *ScriptState) error {
	s.Timespent = time.Now()
	return nil
}

// handleGetTimeSpent pops a unit flag and pushes elapsed time since the
// last TIMESPENT call: milliseconds when flag != 1, microseconds when
// flag == 1. Mirrors TS DebugOps.ts:16-26.
func handleGetTimeSpent(s *ScriptState) error {
	unit := s.PopInt()
	elapsed := time.Since(s.Timespent)
	if unit == 1 {
		s.PushInt(int(elapsed.Microseconds()))
	} else {
		s.PushInt(int(elapsed.Milliseconds()))
	}
	return nil
}
