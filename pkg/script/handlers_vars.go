package script

import (
	"fmt"

	"github.com/zsrv/goscape/pkg/objtype"
)

// varOperandID returns the low 16 bits of the int operand at the
// current PC — that's the VAR id. Bit 16 (0x10000) flags the "secondary
// active player/npc" (_activePlayer2 / _activeNpc2) and is consumed by
// varSecondary; this masks it off to leave the id.
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

// varSecondary reports whether a VARP/VARN opcode targets the SECONDARY
// active player/npc. The flag lives in bit 16 of the int operand (TS
// CoreOps.ts:26/62 `(intOperand >> 16) & 0x1`) — distinct from the simple 0/1
// operand the `.`-prefix uses for ordinary player commands — so `.%var` (e.g.
// the combat scripts' `.%pk_predator1` / `.%lastcombat`) reads/writes the
// second entity. The var id is the low 16 bits (varOperandID).
func varSecondary(s *ScriptState) bool {
	return (s.intOperand()>>16)&0x1 == 1
}

// handlePushVarp reads per-player variable `id` from the active player
// (Self, or Self2 when the secondary bit is set) and pushes it. Dispatches on
// Configs.VarpType(id): STRING calls PushString, else PushInt.
func handlePushVarp(s *ScriptState) error {
	player := s.Self
	if varSecondary(s) {
		player = s.Self2
	}
	if player == nil {
		if varSecondary(s) {
			return fmt.Errorf("PUSH_VARP: %w", ErrNoActivePlayer2)
		}
		return fmt.Errorf("PUSH_VARP: %w", ErrNoActivePlayer)
	}
	id := varOperandID(s)
	typ, _ := s.varpType(id)
	if typ == objtype.ScriptVarTypeString {
		s.PushString(player.VarpString(id))
	} else {
		s.PushInt(int(player.Varp(id)))
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
	// Secondary-aware via bit 16 (see varSecondary): `.%var = x` writes the
	// second active player. The protect gate is likewise operand-aware
	// (TS ProtectedActivePlayer[secondary], CoreOps.ts:51).
	secondary := varSecondary(s)
	player := s.Self
	protectFlag := PtrProtectedActivePlayer
	if secondary {
		player = s.Self2
		protectFlag = PtrProtectedActivePlayer2
	}
	if player == nil {
		if secondary {
			return fmt.Errorf("POP_VARP: %w", ErrNoActivePlayer2)
		}
		return fmt.Errorf("POP_VARP: %w", ErrNoActivePlayer)
	}
	id := varOperandID(s)
	typ, protect := s.varpType(id)
	if protect && s.Pointers&protectFlag == 0 {
		return fmt.Errorf("POP_VARP: %%%d requires protected access", id)
	}
	if typ == objtype.ScriptVarTypeString {
		player.SetVarpString(id, s.PopString())
	} else {
		player.SetVarp(id, int32(s.PopInt()))
	}
	return nil
}

func handlePushVars(s *ScriptState) error {
	if s.World == nil {
		return fmt.Errorf("PUSH_VARS: %w", ErrNoWorld)
	}
	// MVP always pushes int. Real string VARS are rare; dispatch by
	// VarSharedType.Type if we see them in telemetry.
	s.PushInt(int(s.World.VarsInt(varOperandID(s))))
	return nil
}

func handlePopVars(s *ScriptState) error {
	if s.World == nil {
		return fmt.Errorf("POP_VARS: %w", ErrNoWorld)
	}
	val := int32(s.PopInt())
	s.World.SetVarsInt(varOperandID(s), val)
	return nil
}

// handlePushVarn reads per-NPC variable `id` from the active NPC (ActiveNpc,
// or OtherActiveNpc when the secondary bit is set — TS CoreOps.ts:62-63
// `.%npcvar` reads _activeNpc2) and pushes it.
func handlePushVarn(s *ScriptState) error {
	npc := s.ActiveNpc
	if varSecondary(s) {
		npc = s.OtherActiveNpc
	}
	if npc == nil {
		return fmt.Errorf("PUSH_VARN: %w", ErrNoActiveNpc)
	}
	id := varOperandID(s)
	if s.varnType(id) == objtype.ScriptVarTypeString {
		s.PushString(npc.NpcVarNString(id))
	} else {
		s.PushInt(int(npc.NpcVarN(id)))
	}
	return nil
}

// handlePopVarn pops the top of the appropriate stack and writes it to per-NPC
// variable `id` on the active NPC (ActiveNpc, or OtherActiveNpc when the
// secondary bit is set). Dispatches on Configs.VarnType(id).
func handlePopVarn(s *ScriptState) error {
	npc := s.ActiveNpc
	if varSecondary(s) {
		npc = s.OtherActiveNpc
	}
	if npc == nil {
		return fmt.Errorf("POP_VARN: %w", ErrNoActiveNpc)
	}
	id := varOperandID(s)
	if s.varnType(id) == objtype.ScriptVarTypeString {
		npc.SetNpcVarNString(id, s.PopString())
	} else {
		npc.SetNpcVarN(id, int32(s.PopInt()))
	}
	return nil
}
