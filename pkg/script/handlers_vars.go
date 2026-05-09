package script

import (
	"errors"
	"fmt"

	"github.com/zsrv/goscape/pkg/objtype"
)

// varOperandID returns the low 16 bits of the int operand at the
// current PC — that's the VAR id. The high bit (0x10000) flags the
// "secondary active player" (a.k.a. _activePlayer2) for future
// expansion; S5b ignores it.
func varOperandID(s *ScriptState) int {
	return int(uint32(s.Script.IntOperands[s.PC]) & 0xffff)
}

// varnType returns the type of NPC-var id from Configs, falling back
// to ScriptVarTypeInt when Configs is nil (test paths). Mirrors
// DEVIATION-NAI-121-D3.
func (s *ScriptState) varnType(id int) objtype.ScriptVarType {
	if s.Configs == nil {
		return objtype.ScriptVarTypeInt
	}
	return s.Configs.VarnType(id)
}

// varpType returns (type, protect) for player-var id from Configs,
// falling back to (ScriptVarTypeInt, false) when Configs is nil
// (test paths). Mirrors DEVIATION-NAI-121-D3.
func (s *ScriptState) varpType(id int) (objtype.ScriptVarType, bool) {
	if s.Configs == nil {
		return objtype.ScriptVarTypeInt, false
	}
	return s.Configs.VarpType(id)
}

// handlePushVarp reads per-player variable `id` from the active player
// and pushes it. Dispatches on Configs.VarpType(id): STRING calls
// PushString, else calls PushInt. Returns an error if no ActivePlayer
// is bound.
func handlePushVarp(s *ScriptState) error {
	if s.Pointers&PtrActivePlayer == 0 || s.Self == nil {
		return errors.New("PUSH_VARP: no active player")
	}
	id := varOperandID(s)
	typ, _ := s.varpType(id)
	if typ == objtype.ScriptVarTypeString {
		s.PushString(s.Self.VarpString(id))
	} else {
		s.PushInt(int(s.Self.Varp(id)))
	}
	return nil
}

// handlePopVarp pops the top of the appropriate stack and writes it
// to per-player variable `id` on the active player. Dispatches on
// Configs.VarpType(id): STRING calls PopString, else calls PopInt.
// Enforces TS CoreOps.ts:50-52 Protect gate (DEVIATION-NAI-121-D4):
// if the var's type is Protect=true, the script must hold protected
// access (PtrProtectedActivePlayer set) or the handler errors. Returns an error
// if no ActivePlayer is bound.
func handlePopVarp(s *ScriptState) error {
	if s.Pointers&PtrActivePlayer == 0 || s.Self == nil {
		return errors.New("POP_VARP: no active player")
	}
	id := varOperandID(s)
	typ, protect := s.varpType(id)
	if protect && s.Pointers&PtrProtectedActivePlayer == 0 {
		return fmt.Errorf("POP_VARP: %%%d requires protected access", id)
	}
	if typ == objtype.ScriptVarTypeString {
		s.Self.SetVarpString(id, s.PopString())
	} else {
		s.Self.SetVarp(id, int32(s.PopInt()))
	}
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
// pushes it. Dispatches on Configs.VarnType(id): STRING calls
// PushString, else calls PushInt. Returns an error if no ActiveNpc is
// bound. High operand bit (secondary-NPC flag) is ignored — same
// convention as VARP.
func handlePushVarn(s *ScriptState) error {
	if s.ActiveNpc == nil {
		return errors.New("PUSH_VARN: no active npc")
	}
	id := varOperandID(s)
	if s.varnType(id) == objtype.ScriptVarTypeString {
		s.PushString(s.ActiveNpc.NpcVarNString(id))
	} else {
		s.PushInt(int(s.ActiveNpc.NpcVarN(id)))
	}
	return nil
}

// handlePopVarn pops the top of the appropriate stack and writes it
// to per-NPC variable `id` on the active NPC. Dispatches on
// Configs.VarnType(id): STRING calls PopString, else calls PopInt.
// Returns an error if no ActiveNpc is bound.
func handlePopVarn(s *ScriptState) error {
	if s.ActiveNpc == nil {
		return errors.New("POP_VARN: no active npc")
	}
	id := varOperandID(s)
	if s.varnType(id) == objtype.ScriptVarTypeString {
		s.ActiveNpc.SetNpcVarNString(id, s.PopString())
	} else {
		s.ActiveNpc.SetNpcVarN(id, int32(s.PopInt()))
	}
	return nil
}
