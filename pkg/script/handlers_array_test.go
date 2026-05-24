package script

import "testing"

func TestDefineAndReadArray(t *testing.T) {
	// Script: define slot=0 length=5, write arrays[0][2]=42, read it back.
	sf := &ScriptFile{
		Name: "arr",
		Opcodes: []Opcode{
			OpPushConstantInt, // 0: push length 5
			OpDefineArray,     // 1: slot 0 = new [5]int32
			OpPushConstantInt, // 2: push idx 2
			OpPushConstantInt, // 3: push val 42
			OpPopArrayInt,     // 4: arrays[0][2] = 42
			OpPushConstantInt, // 5: push idx 2
			OpPushArrayInt,    // 6: push arrays[0][2]
			OpReturn,
		},
		IntOperands:      []int32{5, 0, 2, 42, 0, 2, 0, 0},
		StringOperands:   []string{"", "", "", "", "", "", "", ""},
		InstructionCount: 8,
	}
	state := Init(sf, nil, false, nil, nil)
	if err := Execute(state); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if got := state.PopInt(); got != 42 {
		t.Errorf("arr[2]: got %d, want 42", got)
	}
}

func TestPushArrayIntOutOfBoundsReturnsZero(t *testing.T) {
	sf := &ScriptFile{
		Name: "oob",
		Opcodes: []Opcode{
			OpPushConstantInt, // push length 3
			OpDefineArray,     // slot 0
			OpPushConstantInt, // push idx 99 (OOB)
			OpPushArrayInt,
			OpReturn,
		},
		IntOperands:      []int32{3, 0, 99, 0, 0},
		StringOperands:   []string{"", "", "", "", ""},
		InstructionCount: 5,
	}
	state := Init(sf, nil, false, nil, nil)
	if err := Execute(state); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if got := state.PopInt(); got != 0 {
		t.Errorf("oob read: got %d, want 0", got)
	}
}

func TestDefineArrayBadSlotErrors(t *testing.T) {
	sf := &ScriptFile{
		Name: "badslot",
		Opcodes: []Opcode{
			OpPushConstantInt, // push length 3
			OpDefineArray,     // operand = slot 99 (OOB)
			OpReturn,
		},
		IntOperands:      []int32{3, 99, 0},
		StringOperands:   []string{"", "", ""},
		InstructionCount: 3,
	}
	state := Init(sf, nil, false, nil, nil)
	if err := Execute(state); err == nil {
		t.Fatal("Execute: want error for OOB slot")
	}
}

func TestSwitchHitAndMiss(t *testing.T) {
	// table[7] = +3 offset. PC starts at 1 after PUSH (instr 0). After
	// OpSwitch's handler adds +3, PC becomes 4; dispatch loop's PC++
	// advances to 5 → lands at instr 5? Actually we want the taken
	// branch to skip over [2,3] (the fall-through PUSH 111 + RETURN)
	// and land at instr 4 (PUSH 222).
	//
	// At OpSwitch's PC=1: handler sets PC += offset. Offset must be 3
	// so PC becomes 4, then dispatch's PC++ makes it 5. Instr 5 is the
	// branch-taken PUSH 222 pushed there. Wait — let me lay this out.
	//
	// Actually: dispatch increments PC AFTER running the handler, so
	// PC=1 → handler sets PC += offset=3 → PC=4 → loop PC++ → PC=5.
	// So instr at index 5 runs next. Design the layout with that.
	sf := &ScriptFile{
		Name: "sw",
		Opcodes: []Opcode{
			OpPushConstantInt, // 0: push key
			OpSwitch,          // 1: switch table[0]
			OpPushConstantInt, // 2: push 111 (fall-through path)
			OpReturn,          // 3
			OpPushConstantInt, // 4
			OpPushConstantInt, // 5: push 222 (taken path — lands here after PC+=3 then PC++)
			OpReturn,          // 6
		},
		IntOperands:      []int32{7, 0, 111, 0, 0, 222, 0},
		StringOperands:   []string{"", "", "", "", "", "", ""},
		InstructionCount: 7,
		SwitchTables: []SwitchTable{
			{7: 3}, // key 7 → PC += 3
		},
	}
	state := Init(sf, nil, false, nil, nil)
	if err := Execute(state); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if got := state.PopInt(); got != 222 {
		t.Errorf("switch hit: got %d, want 222", got)
	}

	// Miss: key 99 not in table; falls through to PUSH 111.
	sf.IntOperands[0] = 99
	state2 := Init(sf, nil, false, nil, nil)
	if err := Execute(state2); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if got := state2.PopInt(); got != 111 {
		t.Errorf("switch miss: got %d, want 111", got)
	}
}

// TestSwitchZeroOffsetFallsThrough pins L28: a present key whose offset
// is 0 falls through, matching TS `if (result)` (truthy) rather than
// branching on key presence. PC stays put so the next instruction is the
// fall-through PUSH 111.
func TestSwitchZeroOffsetFallsThrough(t *testing.T) {
	sf := &ScriptFile{
		Name: "sw0",
		Opcodes: []Opcode{
			OpPushConstantInt, // 0: push key 7
			OpSwitch,          // 1: switch table[0] — key 7 present with offset 0
			OpPushConstantInt, // 2: push 111 (fall-through path)
			OpReturn,          // 3
		},
		IntOperands:      []int32{7, 0, 111, 0},
		StringOperands:   []string{"", "", "", ""},
		InstructionCount: 4,
		SwitchTables: []SwitchTable{
			{7: 0}, // key 7 present, offset 0 → must NOT branch
		},
	}
	state := Init(sf, nil, false, nil, nil)
	if err := Execute(state); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if got := state.PopInt(); got != 111 {
		t.Errorf("switch zero-offset: got %d, want 111 (fall-through)", got)
	}
}
