package script

import "testing"

// TestJumpClearsFrameStack verifies that JUMP discards the frame stack
// — a GOSUB caller's frame should not be restorable after the callee
// performs a JUMP elsewhere.
func TestJumpClearsFrameStack(t *testing.T) {
	// Script C: push "done", mes, return.
	scriptC := &ScriptFile{
		Name:             "[c]",
		LookupKey:        0xCCCC,
		Opcodes:          []Opcode{OpPushConstantString, OpMes, OpReturn},
		IntOperands:      []int32{0, 0, 0},
		StringOperands:   []string{"done", "", ""},
		InstructionCount: 3,
	}

	// Script B: jump to C. No args; jump pops target from stack.
	scriptB := &ScriptFile{
		Name:             "[b]",
		LookupKey:        0xBBBB,
		Opcodes:          []Opcode{OpPushConstantInt, OpJump, OpReturn},
		IntOperands:      []int32{int32(0xCCCC), 0, 0},
		StringOperands:   []string{"", "", ""},
		InstructionCount: 3,
	}

	// Script A: gosub B, return.
	scriptA := &ScriptFile{
		Name:             "[a]",
		LookupKey:        0xAAAA,
		Opcodes:          []Opcode{OpGosubWithParams, OpReturn},
		IntOperands:      []int32{int32(0xBBBB), 0},
		StringOperands:   []string{"", ""},
		InstructionCount: 2,
	}

	prov := NewProvider()
	prov.Register(scriptA)
	prov.Register(scriptB)
	prov.Register(scriptC)

	mp := &mockPlayer{}
	state := Init(scriptA, mp, false, nil, nil)
	state.Provider = prov
	if err := Execute(state); err != nil {
		t.Fatalf("Execute: %v", err)
	}

	if state.Execution != Finished {
		t.Errorf("Execution: got %v, want Finished", state.Execution)
	}
	if state.FrameSP != 0 {
		t.Errorf("FrameSP after JUMP+return: got %d, want 0 (frame cleared)", state.FrameSP)
	}
	if len(mp.messages) != 1 || mp.messages[0] != "done" {
		t.Errorf("messages: got %v, want [done]", mp.messages)
	}
}

// TestJumpBasic verifies JUMP works without GOSUB wrapping.
func TestJumpBasic(t *testing.T) {
	target := &ScriptFile{
		Name:             "[target]",
		LookupKey:        0x1234,
		Opcodes:          []Opcode{OpPushConstantString, OpMes, OpReturn},
		IntOperands:      []int32{0, 0, 0},
		StringOperands:   []string{"hi", "", ""},
		InstructionCount: 3,
	}
	caller := &ScriptFile{
		Name:             "[caller]",
		Opcodes:          []Opcode{OpPushConstantInt, OpJump, OpReturn},
		IntOperands:      []int32{int32(0x1234), 0, 0},
		StringOperands:   []string{"", "", ""},
		InstructionCount: 3,
	}
	prov := NewProvider()
	prov.Register(target)

	mp := &mockPlayer{}
	state := Init(caller, mp, false, nil, nil)
	state.Provider = prov
	if err := Execute(state); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if len(mp.messages) != 1 || mp.messages[0] != "hi" {
		t.Errorf("messages: got %v, want [hi]", mp.messages)
	}
}

// TestJumpWithParams verifies JUMP_WITH_PARAMS pops int args per
// target.IntArgCount and places them into the callee's locals.
func TestJumpWithParams(t *testing.T) {
	// Target: single int param. Body: push_int_local 0, return.
	// This pushes the arg back on the int stack so the caller can inspect it.
	target := &ScriptFile{
		Name:             "[target]",
		LookupKey:        0x5678,
		IntArgCount:      1,
		IntLocalCount:    1,
		Opcodes:          []Opcode{OpPushIntLocal, OpReturn},
		IntOperands:      []int32{0, 0},
		StringOperands:   []string{"", ""},
		InstructionCount: 2,
	}
	// Caller: push 77, jump_with_params target.
	caller := &ScriptFile{
		Name:             "[caller]",
		LookupKey:        0xFFFFFFFF,
		Opcodes:          []Opcode{OpPushConstantInt, OpJumpWithParams, OpReturn},
		IntOperands:      []int32{77, int32(0x5678), 0},
		StringOperands:   []string{"", "", ""},
		InstructionCount: 3,
	}

	prov := NewProvider()
	prov.Register(target)

	state := Init(caller, nil, false, nil, nil)
	state.Provider = prov
	if err := Execute(state); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if got := state.PopInt(); got != 77 {
		t.Errorf("PopInt after JUMP_WITH_PARAMS target returned: got %d, want 77", got)
	}
}
