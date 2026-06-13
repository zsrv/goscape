package script

// handlers_player_vararg.go contains the four VARARG variant queue
// opcode handlers (STRONGQUEUEVARARG / WEAKQUEUEVARARG / QUEUEVARARG /
// LONGQUEUEVARARG). Separated from handlers_player.go (770 LOC at NAI-27
// Bundle 3 dispatch) per design-for-isolation; the file holds only
// VARARG-family handlers.
//
// All four handlers consume popScriptArgs (the top-of-stack type-tag
// string + typed values populated by the new-script bytecode prior to
// the queue opcode) and propagate script-missing errors via the
// (*Player).EnqueueScriptArgs entity-layer return. Per memory
// vararg_opcode_shapes_dont_share_with_fixed_arg_siblings, each
// handler has its own body — no shared helper. LONGQUEUEVARARG
// diverges from the other three by popping an extra logoutAction and
// prepending it to the args slice (TS PlayerOps.ts:191).

// handleStrongQueueVarArg implements STRONGQUEUEVARARG (opcode 2137):
// pop popScriptArgs (top), then delay, then scriptID, and enqueue a
// STRONG queue request. Mirrors TS PlayerOps.ts:110-120.
//
// Like the fixed-arg STRONGQUEUE (rev-274: TS @dee467c8 switched it to a
// plain popInts(3) with no check), STRONGQUEUEVARARG does NOT check
// NumberNotNull on delay — TS destructures scriptId and delay from
// popInts(2) without a check wrapper.
func handleStrongQueueVarArg(s *ScriptState) error {
	if err := requireActivePlayer(s, "STRONGQUEUEVARARG"); err != nil {
		return err
	}
	intArgs, stringArgs := popScriptArgs(s)
	delay := s.PopInt()
	scriptID := uint32(s.PopInt())
	return s.activePlayer().EnqueueScriptArgs(scriptID, delay, intArgs, stringArgs, QueueStrong)
}

// handleWeakQueueVarArg implements WEAKQUEUEVARARG (opcode 2136):
// identical structure to STRONGQUEUEVARARG with QueueWeak. Mirrors TS
// PlayerOps.ts:134-144.
func handleWeakQueueVarArg(s *ScriptState) error {
	if err := requireActivePlayer(s, "WEAKQUEUEVARARG"); err != nil {
		return err
	}
	intArgs, stringArgs := popScriptArgs(s)
	delay := s.PopInt()
	scriptID := uint32(s.PopInt())
	return s.activePlayer().EnqueueScriptArgs(scriptID, delay, intArgs, stringArgs, QueueWeak)
}

// handleQueueVarArg implements QUEUEVARARG (opcode 2134): identical
// structure to STRONGQUEUEVARARG with QueueNormal. Mirrors TS
// PlayerOps.ts:159-169. Does NOT check NumberNotNull on delay (TS
// asymmetry — only the fixed-arg STRONGQUEUE checks).
func handleQueueVarArg(s *ScriptState) error {
	if err := requireActivePlayer(s, "QUEUEVARARG"); err != nil {
		return err
	}
	intArgs, stringArgs := popScriptArgs(s)
	delay := s.PopInt()
	scriptID := uint32(s.PopInt())
	return s.activePlayer().EnqueueScriptArgs(scriptID, delay, intArgs, stringArgs, QueueNormal)
}

// handleLongQueueVarArg implements LONGQUEUEVARARG (opcode 2135):
// pops popScriptArgs (top), then logoutAction, then delay, then scriptID,
// and enqueues a LONG queue request with the args slice
// `[logoutAction, ...intArgs]` (logoutAction prepended). Mirrors TS
// PlayerOps.ts:182-192 line-by-line.
//
// Per memory vararg_opcode_shapes_dont_share_with_fixed_arg_siblings,
// the extra logoutAction popInt + prepended args slice diverges from
// the other 3 VARARG handlers; this body is intentionally not shared.
func handleLongQueueVarArg(s *ScriptState) error {
	if err := requireActivePlayer(s, "LONGQUEUEVARARG"); err != nil {
		return err
	}
	intArgs, stringArgs := popScriptArgs(s)
	logoutAction := s.PopInt()
	delay := s.PopInt()
	scriptID := uint32(s.PopInt())
	prepended := append([]int{logoutAction}, intArgs...)
	return s.activePlayer().EnqueueScriptArgs(scriptID, delay, prepended, stringArgs, QueueLong)
}
