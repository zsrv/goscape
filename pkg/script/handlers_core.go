package script

import "fmt"

// handleGosub pops the target script id from the int stack, then pops
// the callee's declared args off the stacks before gosub-calling it.
// The frame is saved so RETURN can resume the caller. Counterpart to
// handleJump (tail-call).
//
// TS plain GOSUB (CoreOps.ts:194) pops the proc id then calls
// gosubFrame → setupNewScript, which ALWAYS pops intArgCount/
// stringArgCount args regardless of the GOSUB vs GOSUB_WITH_PARAMS
// variant. Plain GOSUB is the dynamic-dispatch form (proc id computed
// onto the stack above its args), so it must consume the declared args
// exactly like GOSUB_WITH_PARAMS.
func handleGosub(s *ScriptState) error {
	// TS gates the frame-stack cap at the TOP of the handler (CoreOps.ts:194-214:
	// `if (state.fp >= 50) throw 'stack overflow'`), which the runner's
	// try/catch converts to ScriptExecution.Aborted. Goscape mirrors this by
	// returning the error before any stack mutation; the runner's
	// `if err := h(s); err != nil { s.Execution=Aborted; return err }` path
	// (script-core-2) then aborts gracefully where the pre-fix GosubCall's
	// raw panic would have crashed the host goroutine.
	if s.FrameSP >= FrameCapacity {
		return fmt.Errorf("GOSUB: stack overflow (FrameSP=%d, FrameCapacity=%d) in %q", s.FrameSP, FrameCapacity, s.Script.Name)
	}
	if s.Provider == nil {
		return fmt.Errorf("GOSUB: %w", ErrNoProvider)
	}
	scriptID := uint32(s.PopInt())
	target := s.Provider.GetByID(scriptID)
	if target == nil {
		return fmt.Errorf("GOSUB: no script with id %d", scriptID)
	}
	intArgs, stringArgs := popArgsForTarget(s, target)
	s.GosubCall(target, intArgs, stringArgs)
	return nil
}

// handleJump pops the target script id from the int stack, then pops the
// callee's declared args, and tail-calls it. TS CoreOps.ts:216 JUMP →
// gotoFrame → setupNewScript pops the same intArgCount/stringArgCount
// args as GOSUB.
func handleJump(s *ScriptState) error {
	if s.Provider == nil {
		return fmt.Errorf("JUMP: %w", ErrNoProvider)
	}
	scriptID := uint32(s.PopInt())
	target := s.Provider.GetByID(scriptID)
	if target == nil {
		return fmt.Errorf("JUMP: no script with id %d", scriptID)
	}
	intArgs, stringArgs := popArgsForTarget(s, target)
	s.JumpCall(target, intArgs, stringArgs)
	return nil
}

// handleJumpWithParams reads target script id from the instruction's
// int operand and pops int/string args per the target's ParamTypes.
// Mirrors handleGosubWithParams.
func handleJumpWithParams(s *ScriptState) error {
	if s.Provider == nil {
		return fmt.Errorf("JUMP_WITH_PARAMS: %w", ErrNoProvider)
	}
	scriptID := uint32(s.Script.IntOperands[s.PC])
	target := s.Provider.GetByID(scriptID)
	if target == nil {
		return fmt.Errorf("JUMP_WITH_PARAMS: no script with id %d", scriptID)
	}
	intArgs, stringArgs := popArgsForTarget(s, target)
	s.JumpCall(target, intArgs, stringArgs)
	return nil
}
