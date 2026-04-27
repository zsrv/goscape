package world

import (
	"github.com/zsrv/goscape/pkg/script"
)

// scriptLineValidator returns the LineValidator wired into all script
// state-init sites. Returns nil if the gamemap has not been initialized
// (only happens in unit-test fixtures that build a stripped-down
// Server). Production callers always have gamemap set via New().
// HuntAll-mode passesFilter pessimistically allows on a nil validator,
// so the test path degrades gracefully. NAI-35-T3.
func (s *Server) scriptLineValidator() script.LineValidator {
	if s.gamemap == nil {
		return nil
	}
	return s.gamemap.Pathfinder.LineValidator
}

// runScript initialises a ScriptState for a fresh invocation and routes
// the result via resumeOrFinish. Safe to call with a nil scriptFile
// (no-op) so callers don't have to nil-check the trigger lookup.
//
// If the script suspends (Execution == Suspended), the state is stored
// on the active player and the tick loop will resume it when the
// player's delay expires via processActiveScripts.
func (s *Server) runScript(sf *script.ScriptFile, self script.ActivePlayer, protect bool, intArgs []int, stringArgs []string) {
	if sf == nil {
		return
	}
	state := script.Init(sf, self, protect, intArgs, stringArgs)
	state.Provider = s.scriptProvider
	state.World = s.worldVars
	state.Configs = s.configsView
	state.Inv = s.invLookup
	state.Npcs = s.npcLookup
	state.PlayerLookup = s
	state.LineValidator = s.scriptLineValidator()
	s.resumeOrFinish(state, self)
}

// resumeOrFinish is the shared post-Execute handler for both fresh runs
// (from runScript) and resumed runs (from processActiveScripts). It
// drives the state-store / state-clear decision in one place so the
// tick loop doesn't need to type-assert back to *Player.
func (s *Server) resumeOrFinish(state *script.ScriptState, self script.ActivePlayer) {
	if err := script.Execute(state); err != nil {
		s.log.Warn("script execute error",
			"script", state.Script.Name, "err", err)
		self.ClearActiveScript()
		return
	}
	switch state.Execution {
	case script.Finished, script.Aborted:
		self.ClearActiveScript()
	case script.Suspended, script.PauseButton, script.CountDialog:
		self.StoreActiveScript(state)
	case script.WorldSuspended:
		// NAI-37 T10: player-bound script suspended to world queue.
		// Pop the wakeup-tick (which the script's bytecode pushed
		// before WORLD_DELAY — see handlers_server.go:87-108) and
		// enqueue. The player no longer owns this script; it now
		// belongs to the world queue. Mirrors TS Player.ts:2135-2136.
		delay := state.PopInt()
		s.EnqueueWorldScript(state, delay)
		self.ClearActiveScript()
	default:
		// NpcSuspended — future sub-spec (T11).
		s.log.Warn("script in unsupported execution state",
			"script", state.Script.Name, "execution", state.Execution)
		self.ClearActiveScript()
	}
}

// resumeOrFinishWorld dispatches the post-Execute state for a script
// run from the world-script queue (called by processWorldQueue after
// removing the entry). STUB at T9 — full dispatch table arrives in T12.
//
// At T9: just calls script.Execute and logs errors. The post-execute
// state is not yet dispatched on, so a script that returns
// WorldSuspended will not self-re-enqueue (T12 fixes this); a script
// that hits a cross-context state will not warn (T12 adds the warns).
// T9's scheduler tests exercise only Finished-returning scripts; the
// WorldSuspended self-loop test is deferred to T12 alongside the
// dispatch table.
func (s *Server) resumeOrFinishWorld(state *script.ScriptState) {
	if err := script.Execute(state); err != nil {
		s.log.Warn("world script execute error",
			"script", state.Script.Name, "err", err)
		return
	}
	// T12 will switch on state.Execution.
}
