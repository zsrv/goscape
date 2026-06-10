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

// varsType returns the type of world-shared var id from Configs, falling
// back to ScriptVarTypeInt when Configs is nil (test paths). Mirrors
// DEVIATION-NAI-121-D3 — silent default rather than TS's check() throw.
func (s *ScriptState) varsType(id int) objtype.ScriptVarType {
	if s.Configs == nil {
		return objtype.ScriptVarTypeInt
	}
	return s.Configs.VarsType(id)
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

// checkVarBitType validates a VarBitType id is registered in s.Configs
// and returns it. Mirrors TS check(id, VarBitValid) (ScriptValidators.ts:130
// @43e02957 — config-type validator over 0 <= id < VarBitType.count).
// rev-254.
func checkVarBitType(s *ScriptState, id int, op string) (*objtype.VarBitType, error) {
	if s.Configs == nil {
		return nil, fmt.Errorf("%s: no VarBit with value (%d) found", op, id)
	}
	vb := s.Configs.VarBitType(id)
	if vb == nil {
		return nil, fmt.Errorf("%s: no VarBit with value (%d) found", op, id)
	}
	return vb, nil
}

// handlePushVarbit reads the varbit's bit-range out of the active
// player's base varp (Self, or Self2 when the secondary bit is set) and
// pushes it. Varbits are always int-typed — no STRING fork like
// PUSH_VARP. Mirrors TS CoreOps.ts:61-71 @43e02957. rev-254 (opcode 25
// restored; deleted in 244).
func handlePushVarbit(s *ScriptState) error {
	player := s.Self
	if varSecondary(s) {
		player = s.Self2
	}
	if player == nil {
		if varSecondary(s) {
			return fmt.Errorf("PUSH_VARBIT: %w", ErrNoActivePlayer2)
		}
		return fmt.Errorf("PUSH_VARBIT: %w", ErrNoActivePlayer)
	}
	id := varOperandID(s)
	vb, err := checkVarBitType(s, id, "PUSH_VARBIT")
	if err != nil {
		return err
	}
	s.PushInt(int(player.GetVarBit(vb.ID)))
	return nil
}

// handlePopVarbit pops an int and writes it into the varbit's bit-range
// of the active player's base varp. The protect gate reads the BASE
// varp's protect flag (TS CoreOps.ts:83-84 `VarPlayerType.get(varbit.basevar)`
// → `basevar.protect`), but the error carries the VARBIT's debugname
// (CoreOps.ts:85 `%${varbit.debugname} requires protected access`).
// Secondary-aware via bit 16 like POP_VARP; the gate is likewise
// operand-aware (ProtectedActivePlayer[secondary], CoreOps.ts:84).
// Mirrors TS CoreOps.ts:73-90 @43e02957. rev-254 (opcode 27 restored;
// deleted in 244).
func handlePopVarbit(s *ScriptState) error {
	secondary := varSecondary(s)
	player := s.Self
	protectFlag := PtrProtectedActivePlayer
	if secondary {
		player = s.Self2
		protectFlag = PtrProtectedActivePlayer2
	}
	if player == nil {
		if secondary {
			return fmt.Errorf("POP_VARBIT: %w", ErrNoActivePlayer2)
		}
		return fmt.Errorf("POP_VARBIT: %w", ErrNoActivePlayer)
	}
	id := varOperandID(s)
	vb, err := checkVarBitType(s, id, "POP_VARBIT")
	if err != nil {
		return err
	}
	// TS VarPlayerType.get(varbit.basevar).protect — goscape's VarpType
	// surfaces exactly the (type, protect) tuple; an OOB basevar degrades
	// to protect=false (DEVIATION-NAI-121-D3 convention).
	_, protect := s.varpType(vb.Basevar)
	if protect && s.Pointers&protectFlag == 0 {
		return fmt.Errorf("POP_VARBIT: %%%s requires protected access", vb.DebugName)
	}
	player.SetVarBit(vb.ID, int32(s.PopInt()))
	return nil
}

// handlePushVars reads world-shared variable `id` from the running World
// and pushes it. Dispatches on Configs.VarsType(id): STRING calls
// PushString backed by World.VarsString, else PushInt backed by
// World.VarsInt. Mirrors TS CoreOps.ts:257-265 — `state.intStack.push(...)`
// vs `state.stringStack.push(...)` branches on `varsType.type`.
//
// Pre-h-core-3 always treated shared vars as int regardless of the
// VarSharedType.Type bit, so a stock script reading a string-typed
// shared var (e.g. a debug name surfaced from World) pushed a junk int
// instead of the string and downstream PopString crashed the script.
// h-config-5 is the canonical dup row for this divergence at the same
// file/line.
//
// TS additionally gates with check(id, VarSharedValid) which throws on
// an unloaded id; goscape uses the same silent-default convention as
// VarpType/VarnType (out-of-range → ScriptVarTypeInt, underlying World
// accessor returns 0/""), so a malformed bytecode pushes the zero value
// rather than aborting. Tracking VarSharedValid as a separate strict-mode
// gate is part of the broader VarpValid/VarnValid carryover; not in
// h-core-3 scope.
func handlePushVars(s *ScriptState) error {
	if s.World == nil {
		return fmt.Errorf("PUSH_VARS: %w", ErrNoWorld)
	}
	id := varOperandID(s)
	if s.varsType(id) == objtype.ScriptVarTypeString {
		s.PushString(s.World.VarsString(id))
	} else {
		s.PushInt(int(s.World.VarsInt(id)))
	}
	return nil
}

// handlePopVars pops the top of the appropriate stack and writes it to
// world-shared variable `id` on the running World. Dispatches on
// Configs.VarsType(id): STRING calls PopString backed by SetVarsString,
// else PopInt backed by SetVarsInt. Mirrors TS CoreOps.ts:267-275.
// See handlePushVars for the pre-h-core-3 always-int divergence note.
func handlePopVars(s *ScriptState) error {
	if s.World == nil {
		return fmt.Errorf("POP_VARS: %w", ErrNoWorld)
	}
	id := varOperandID(s)
	if s.varsType(id) == objtype.ScriptVarTypeString {
		s.World.SetVarsString(id, s.PopString())
	} else {
		s.World.SetVarsInt(id, int32(s.PopInt()))
	}
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
