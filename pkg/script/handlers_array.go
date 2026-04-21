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
// table and advances PC by the table's offset on hit. Falls through on
// miss.
func handleSwitch(s *ScriptState) error {
	key := int32(s.PopInt())
	tableIdx := int(s.Script.IntOperands[s.PC])
	if tableIdx < 0 || tableIdx >= len(s.Script.SwitchTables) {
		return nil
	}
	table := s.Script.SwitchTables[tableIdx]
	if offset, ok := table[key]; ok {
		s.PC += int(offset)
	}
	return nil
}
