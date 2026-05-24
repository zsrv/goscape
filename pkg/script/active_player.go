package script

// Operand-aware active-player resolution.
//
// In RuneScript a player command may be written bare (`mes`, `uid`) or with a
// leading `.` (`.mes`, `.uid`). The compiler encodes that prefix as the
// instruction's int operand: 0 → primary active player, 1 → secondary. TS
// resolves this through the ScriptState.activePlayer getter
// (ScriptState.ts:214-241); every PlayerOps handler that operates on "the
// active player" reads that getter rather than a fixed field.
//
// goscape stores the two players in Self (primary) and Self2 (secondary). The
// helpers below resolve the operand so handlers can mirror TS by calling
// s.activePlayer() instead of touching s.Self directly. For operand 0 (the
// overwhelmingly common, non-`.`-prefixed case) activePlayer() == Self, so
// converting a handler from s.Self to s.activePlayer() is behaviourally a
// no-op there and only changes the previously-broken operand-1 path.

// intOperand returns the int operand of the instruction at the current PC.
// Mirrors TS ScriptState.intOperand (ScriptState.ts:309-311). Defaults to 0
// (primary active player) when the script/PC are not in a dispatchable state
// — production handlers always run with a valid Script and in-bounds PC, but
// unit tests that invoke a handler directly may construct a bare ScriptState;
// operand 0 keeps those on the primary slot, matching the pre-operand-aware
// behaviour.
func (s *ScriptState) intOperand() int32 {
	if s.Script == nil || s.PC < 0 || s.PC >= len(s.Script.IntOperands) {
		return 0
	}
	return s.Script.IntOperands[s.PC]
}

// activePlayer returns the operand-resolved primary active player: Self for
// operand 0, Self2 for operand 1. Mirrors TS ScriptState.activePlayer getter
// (ScriptState.ts:214-221).
func (s *ScriptState) activePlayer() ActivePlayer {
	if s.intOperand() == 0 {
		return s.Self
	}
	return s.Self2
}

// activePlayer2 returns the operand-resolved secondary active player: Self2
// for operand 0, Self for operand 1 (the inverse of activePlayer). Mirrors TS
// ScriptState.activePlayer2 getter (ScriptState.ts:223-230).
func (s *ScriptState) activePlayer2() ActivePlayer {
	if s.intOperand() == 0 {
		return s.Self2
	}
	return s.Self
}

// activePlayerPointer returns the pointer-flag bit for the operand-resolved
// primary active-player slot (PtrActivePlayer for operand 0, PtrActivePlayer2
// for operand 1).
func (s *ScriptState) activePlayerPointer() Pointer {
	if s.intOperand() == 0 {
		return PtrActivePlayer
	}
	return PtrActivePlayer2
}

// setActivePlayer writes p into the operand-resolved primary slot and sets the
// matching pointer flag. Mirrors TS ScriptState.activePlayer setter
// (ScriptState.ts:235-241). Used by FINDUID/P_FINDUID.
func (s *ScriptState) setActivePlayer(p ActivePlayer) {
	if s.intOperand() == 0 {
		s.Self = p
		s.Pointers |= PtrActivePlayer
	} else {
		s.Self2 = p
		s.Pointers |= PtrActivePlayer2
	}
}

// activeNpc returns the operand-resolved active npc: ActiveNpc for operand 0,
// OtherActiveNpc for operand 1. Mirrors TS ScriptState.activeNpc getter
// (ScriptState.ts:246-252) — the `.npc` / `.npc2` selector that every NpcOps
// read/mutate handler resolves via checkedHandler(ActiveNpc[intOperand]).
// Returns nil when the resolved slot is unset; callers gate on
// requireActiveNpc first.
func (s *ScriptState) activeNpc() ActiveNpc {
	if s.intOperand() == 0 {
		return s.ActiveNpc
	}
	return s.OtherActiveNpc
}
