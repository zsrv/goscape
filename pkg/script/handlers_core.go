package script

import (
	"errors"
	"fmt"
)

// handleGosub pops the target script id from the int stack and gosub-
// calls it with no args. The frame is saved so RETURN can resume the
// caller. Counterpart to handleJump (tail-call).
func handleGosub(s *ScriptState) error {
	if s.Provider == nil {
		return errors.New("GOSUB: no provider")
	}
	scriptID := uint32(s.PopInt())
	target := s.Provider.GetByLookupKey(scriptID)
	if target == nil {
		return fmt.Errorf("GOSUB: unknown script id %d", scriptID)
	}
	s.GosubCall(target, nil, nil)
	return nil
}

// handleJump pops the target script id from the int stack and tail-
// calls it with no args. TS CoreOps.ts JUMP.
func handleJump(s *ScriptState) error {
	if s.Provider == nil {
		return errors.New("JUMP: no provider")
	}
	scriptID := uint32(s.PopInt())
	target := s.Provider.GetByLookupKey(scriptID)
	if target == nil {
		return fmt.Errorf("JUMP: unknown script id %d", scriptID)
	}
	s.JumpCall(target, nil, nil)
	return nil
}

// handleJumpWithParams reads target script id from the instruction's
// int operand and pops int/string args per the target's ParamTypes.
// Mirrors handleGosubWithParams.
func handleJumpWithParams(s *ScriptState) error {
	if s.Provider == nil {
		return errors.New("JUMP_WITH_PARAMS: no provider")
	}
	scriptID := uint32(s.Script.IntOperands[s.PC])
	target := s.Provider.GetByLookupKey(scriptID)
	if target == nil {
		return fmt.Errorf("JUMP_WITH_PARAMS: unknown script id %d", scriptID)
	}
	intArgs, stringArgs := popArgsForTarget(s, target)
	s.JumpCall(target, intArgs, stringArgs)
	return nil
}
