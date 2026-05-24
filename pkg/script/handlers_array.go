package script

import "errors"

// ArrayCap is the maximum number of arrays per script (matches TS's
// compile-time limit).
const ArrayCap = 5

// handleDefineArray allocates a fresh []int32 of popped length at the
// operand-indexed slot. Re-defining the same slot overwrites.
func handleDefineArray(s *ScriptState) error {
	length := s.PopInt()
	slot := int(s.Script.IntOperands[s.PC])
	if slot < 0 || slot >= ArrayCap {
		return errors.New("DEFINE_ARRAY: slot out of range")
	}
	if length < 0 {
		length = 0
	}
	s.Arrays[slot] = make([]int32, length)
	return nil
}

// handlePushArrayInt reads Arrays[slot][idx]. Pushes 0 on OOB.
func handlePushArrayInt(s *ScriptState) error {
	idx := s.PopInt()
	slot := int(s.Script.IntOperands[s.PC])
	if slot < 0 || slot >= ArrayCap || idx < 0 || idx >= len(s.Arrays[slot]) {
		s.PushInt(0)
		return nil
	}
	s.PushInt(int(s.Arrays[slot][idx]))
	return nil
}

// handlePopArrayInt writes Arrays[slot][idx] = value. Silently drops on
// OOB.
func handlePopArrayInt(s *ScriptState) error {
	val := s.PopInt()
	idx := s.PopInt()
	slot := int(s.Script.IntOperands[s.PC])
	if slot < 0 || slot >= ArrayCap || idx < 0 || idx >= len(s.Arrays[slot]) {
		return nil
	}
	s.Arrays[slot][idx] = int32(val)
	return nil
}

// handleSwitch looks up the popped key in the per-instruction switch
// table and advances PC by the table's offset when that offset is
// non-zero. Falls through on miss.
//
// TS (CoreOps.ts:244) reads `const result = table[key]` and branches on
// `if (result)` — truthy, not key-presence. A missing key yields
// undefined (falsy → fall through); a present key with offset 0 is also
// falsy → fall through. Go's map zero-value is 0, so reading the offset
// directly and testing `!= 0` reproduces both cases exactly (a present
// 0-offset would have been a no-op `PC += 0` under the old key-presence
// test, so behaviour is unchanged in practice — this just mirrors TS).
func handleSwitch(s *ScriptState) error {
	key := int32(s.PopInt())
	tableIdx := int(s.Script.IntOperands[s.PC])
	if tableIdx < 0 || tableIdx >= len(s.Script.SwitchTables) {
		return nil
	}
	if offset := s.Script.SwitchTables[tableIdx][key]; offset != 0 {
		s.PC += int(offset)
	}
	return nil
}
