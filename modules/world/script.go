package world

import (
	"github.com/zsrv/goscape/pkg/script"
)

// runScript initializes a ScriptState for the given script and invocation
// context, executes it, and handles errors / suspension uniformly. Safe to
// call with a nil scriptFile (no-op) so callers don't have to nil-check the
// trigger lookup.
//
// Suspension: until sub-spec S4 implements per-entity active-script storage
// + tick-loop resumption, any script that pauses (delay, queue, dialog) is
// dropped after a warning. LOGIN scripts in the cache typically don't
// suspend, so this is acceptable for S3.
func (s *Server) runScript(sf *script.ScriptFile, self script.ActivePlayer, protect bool, intArgs []int, stringArgs []string) {
	if sf == nil {
		return
	}
	state := script.Init(sf, self, protect, intArgs, stringArgs)
	state.Provider = s.scriptProvider
	if err := script.Execute(state); err != nil {
		s.log.Warn("script execute error",
			"script", sf.Name, "err", err)
		return
	}
	if state.Execution != script.Finished {
		s.log.Warn("script suspended; suspension support pending sub-spec S4",
			"script", sf.Name, "execution", state.Execution)
	}
}
