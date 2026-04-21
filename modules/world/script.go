package world

import (
	"github.com/zsrv/goscape/pkg/script"
)

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
	case script.Suspended:
		self.StoreActiveScript(state)
	default:
		// CountDialog, PauseButton, NpcSuspended, WorldSuspended are
		// handled by later sub-specs; drop the state for now.
		s.log.Warn("script in unsupported execution state",
			"script", state.Script.Name, "execution", state.Execution)
		self.ClearActiveScript()
	}
}
