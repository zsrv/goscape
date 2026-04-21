package script

import "errors"

// varOperandID returns the low 16 bits of the int operand at the
// current PC — that's the VAR id. The high bit (0x10000) flags the
// "secondary active player" (a.k.a. _activePlayer2) for future
// expansion; S5b ignores it.
func varOperandID(s *ScriptState) int {
	return int(uint32(s.Script.IntOperands[s.PC]) & 0xffff)
}

func handlePushVarp(s *ScriptState) error {
	if s.Pointers&PtrActivePlayer == 0 || s.Self == nil {
		return errors.New("PUSH_VARP: no active player")
	}
	s.PushInt(int(s.Self.Varp(varOperandID(s))))
	return nil
}

func handlePopVarp(s *ScriptState) error {
	if s.Pointers&PtrActivePlayer == 0 || s.Self == nil {
		return errors.New("POP_VARP: no active player")
	}
	val := int32(s.PopInt())
	s.Self.SetVarp(varOperandID(s), val)
	return nil
}

func handlePushVars(s *ScriptState) error {
	if s.World == nil {
		return errors.New("PUSH_VARS: no world")
	}
	// MVP always pushes int. Real string VARS are rare; dispatch by
	// VarSharedType.Type if we see them in telemetry.
	s.PushInt(int(s.World.VarsInt(varOperandID(s))))
	return nil
}

func handlePopVars(s *ScriptState) error {
	if s.World == nil {
		return errors.New("POP_VARS: no world")
	}
	val := int32(s.PopInt())
	s.World.SetVarsInt(varOperandID(s), val)
	return nil
}

// handlePushVarn is a stub until S6's active_npc lands.
func handlePushVarn(s *ScriptState) error {
	s.PushInt(0)
	return nil
}

// handlePopVarn is a stub until S6's active_npc lands.
func handlePopVarn(s *ScriptState) error {
	_ = s.PopInt()
	return nil
}
