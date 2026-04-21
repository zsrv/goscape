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

// handlePushVarn reads per-NPC variable `id` from the active NPC and
// pushes it. Returns an error if no ActiveNpc is bound. High operand bit
// (secondary-NPC flag) is ignored — same convention as VARP.
func handlePushVarn(s *ScriptState) error {
	if s.ActiveNpc == nil {
		return errors.New("PUSH_VARN: no active npc")
	}
	s.PushInt(int(s.ActiveNpc.NpcVarN(varOperandID(s))))
	return nil
}

// handlePopVarn pops an int and writes it to per-NPC variable `id` on
// the active NPC. Returns an error if no ActiveNpc is bound.
func handlePopVarn(s *ScriptState) error {
	if s.ActiveNpc == nil {
		return errors.New("POP_VARN: no active npc")
	}
	val := int32(s.PopInt())
	s.ActiveNpc.SetNpcVarN(varOperandID(s), val)
	return nil
}
