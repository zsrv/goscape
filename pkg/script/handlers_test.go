package script

import (
	"testing"
)

// runScript builds a ScriptFile from the given ops/operands, initialises a state
// with self, and executes it. Fails the test if Execute returns an error.
func runScript(t *testing.T, ops []Opcode, intOps []int32, strOps []string, self ActivePlayer) *ScriptState {
	t.Helper()
	n := len(ops)
	if intOps == nil {
		intOps = make([]int32, n)
	}
	if strOps == nil {
		strOps = make([]string, n)
	}
	f := &ScriptFile{
		Name:             "test",
		Opcodes:          ops,
		IntOperands:      intOps,
		StringOperands:   strOps,
		InstructionCount: uint32(n),
		IntLocalCount:    8,
		StringLocalCount: 8,
	}
	s := Init(f, self, true, nil, nil)
	if err := Execute(s); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	return s
}

func TestHandlePushConstantInt(t *testing.T) {
	s := runScript(t,
		[]Opcode{OpPushConstantInt, OpReturn},
		[]int32{42, 0},
		nil, nil)
	// After RETURN the int stack still has 42 on it.
	s.ISP-- // peek
	if s.IntStack[s.ISP] != 42 {
		t.Errorf("top of stack: got %d want 42", s.IntStack[s.ISP])
	}
}

func TestHandlePushConstantString(t *testing.T) {
	s := runScript(t,
		[]Opcode{OpPushConstantString, OpReturn},
		[]int32{0, 0},
		[]string{"hi", ""},
		nil)
	s.SSP--
	if s.StringStack[s.SSP] != "hi" {
		t.Errorf("top of string stack: got %q want %q", s.StringStack[s.SSP], "hi")
	}
}

func TestHandleReturn(t *testing.T) {
	s := runScript(t, []Opcode{OpReturn}, nil, nil, nil)
	if s.Execution != Finished {
		t.Errorf("Execution: got %v want Finished", s.Execution)
	}
}

func TestHandlePushPopIntLocal(t *testing.T) {
	// PUSH_CONSTANT_INT 99 → POP_INT_LOCAL 0 → PUSH_INT_LOCAL 0 → RETURN
	ops := []Opcode{OpPushConstantInt, OpPopIntLocal, OpPushIntLocal, OpReturn}
	intOps := []int32{99, 0, 0, 0}
	s := runScript(t, ops, intOps, nil, nil)
	s.ISP--
	if s.IntStack[s.ISP] != 99 {
		t.Errorf("top of stack: got %d want 99", s.IntStack[s.ISP])
	}
}

func TestHandlePushPopStringLocal(t *testing.T) {
	// PUSH_CONSTANT_STRING "abc" → POP_STRING_LOCAL 0 → PUSH_STRING_LOCAL 0 → RETURN
	ops := []Opcode{OpPushConstantString, OpPopStringLocal, OpPushStringLocal, OpReturn}
	intOps := []int32{0, 0, 0, 0}
	strOps := []string{"abc", "", "", ""}
	s := runScript(t, ops, intOps, strOps, nil)
	s.SSP--
	if s.StringStack[s.SSP] != "abc" {
		t.Errorf("top of string stack: got %q want %q", s.StringStack[s.SSP], "abc")
	}
}

func TestHandleBranchUnconditional(t *testing.T) {
	// PC0: BRANCH +1 → skips PC1 (PUSH_CONSTANT_INT 999) → PC2: PUSH_CONSTANT_INT 1 → PC3: RETURN
	// Branch operand +1 → PC += 1, then PC++ → next PC = 0+1+1 = 2 (skip PC1).
	ops := []Opcode{OpBranch, OpPushConstantInt, OpPushConstantInt, OpReturn}
	intOps := []int32{1, 999, 1, 0}
	s := runScript(t, ops, intOps, nil, nil)
	s.ISP--
	if s.IntStack[s.ISP] != 1 {
		t.Errorf("expected 1 on stack (skipped 999), got %d", s.IntStack[s.ISP])
	}
}

func TestHandleBranchEqualsTaken(t *testing.T) {
	// Push 5, 5; BRANCH_EQUALS +1 (should branch); PUSH 999 (skipped); PUSH 1; RETURN
	ops := []Opcode{
		OpPushConstantInt, OpPushConstantInt,
		OpBranchEquals,
		OpPushConstantInt, // skipped
		OpPushConstantInt,
		OpReturn,
	}
	intOps := []int32{5, 5, 1, 999, 1, 0}
	s := runScript(t, ops, intOps, nil, nil)
	s.ISP--
	if s.IntStack[s.ISP] != 1 {
		t.Errorf("branch taken: expected 1, got %d", s.IntStack[s.ISP])
	}
}

func TestHandleBranchEqualsNotTaken(t *testing.T) {
	// Push 5, 6; BRANCH_EQUALS +1 (not taken); PUSH 999 (executed); RETURN
	ops := []Opcode{
		OpPushConstantInt, OpPushConstantInt,
		OpBranchEquals,
		OpPushConstantInt,
		OpReturn,
	}
	intOps := []int32{5, 6, 1, 999, 0}
	s := runScript(t, ops, intOps, nil, nil)
	s.ISP--
	if s.IntStack[s.ISP] != 999 {
		t.Errorf("branch not taken: expected 999, got %d", s.IntStack[s.ISP])
	}
}

func TestHandleBranchNotTaken(t *testing.T) {
	// Push 5, 6; BRANCH_NOT +1 (taken, values differ); PUSH 999 (skipped); PUSH 1; RETURN
	ops := []Opcode{
		OpPushConstantInt, OpPushConstantInt,
		OpBranchNot,
		OpPushConstantInt,
		OpPushConstantInt,
		OpReturn,
	}
	intOps := []int32{5, 6, 1, 999, 1, 0}
	s := runScript(t, ops, intOps, nil, nil)
	s.ISP--
	if s.IntStack[s.ISP] != 1 {
		t.Errorf("BRANCH_NOT taken: expected 1, got %d", s.IntStack[s.ISP])
	}
}

func TestHandleBranchNotNotTaken(t *testing.T) {
	// Push 5, 5; BRANCH_NOT (not taken, equal); PUSH 999 (executed); RETURN
	ops := []Opcode{
		OpPushConstantInt, OpPushConstantInt,
		OpBranchNot,
		OpPushConstantInt,
		OpReturn,
	}
	intOps := []int32{5, 5, 1, 999, 0}
	s := runScript(t, ops, intOps, nil, nil)
	s.ISP--
	if s.IntStack[s.ISP] != 999 {
		t.Errorf("BRANCH_NOT not taken: expected 999, got %d", s.IntStack[s.ISP])
	}
}

func TestHandleAdd(t *testing.T) {
	ops := []Opcode{OpPushConstantInt, OpPushConstantInt, OpAdd, OpReturn}
	intOps := []int32{2, 3, 0, 0}
	s := runScript(t, ops, intOps, nil, nil)
	s.ISP--
	if s.IntStack[s.ISP] != 5 {
		t.Errorf("2+3: got %d want 5", s.IntStack[s.ISP])
	}
}

func TestHandleSub(t *testing.T) {
	ops := []Opcode{OpPushConstantInt, OpPushConstantInt, OpSub, OpReturn}
	intOps := []int32{5, 3, 0, 0}
	s := runScript(t, ops, intOps, nil, nil)
	s.ISP--
	if s.IntStack[s.ISP] != 2 {
		t.Errorf("5-3: got %d want 2", s.IntStack[s.ISP])
	}
}

func TestHandleJoinString(t *testing.T) {
	// Push "a", "b", "c" then JOIN_STRING 3 → "abc"
	ops := []Opcode{
		OpPushConstantString, OpPushConstantString, OpPushConstantString,
		OpJoinString, OpReturn,
	}
	intOps := []int32{0, 0, 0, 3, 0}
	strOps := []string{"a", "b", "c", "", ""}
	s := runScript(t, ops, intOps, strOps, nil)
	s.SSP--
	if s.StringStack[s.SSP] != "abc" {
		t.Errorf("join: got %q want %q", s.StringStack[s.SSP], "abc")
	}
}

func TestHandleToString(t *testing.T) {
	ops := []Opcode{OpPushConstantInt, OpToString, OpReturn}
	intOps := []int32{42, 0, 0}
	s := runScript(t, ops, intOps, nil, nil)
	s.SSP--
	if s.StringStack[s.SSP] != "42" {
		t.Errorf("tostring: got %q want %q", s.StringStack[s.SSP], "42")
	}
}

func TestHandlePopIntDiscard(t *testing.T) {
	ops := []Opcode{OpPushConstantInt, OpPopIntDiscard, OpReturn}
	intOps := []int32{42, 0, 0}
	s := runScript(t, ops, intOps, nil, nil)
	if s.ISP != 0 {
		t.Errorf("ISP after discard: got %d want 0", s.ISP)
	}
}

func TestHandlePopStringDiscard(t *testing.T) {
	ops := []Opcode{OpPushConstantString, OpPopStringDiscard, OpReturn}
	intOps := []int32{0, 0, 0}
	strOps := []string{"hello", "", ""}
	s := runScript(t, ops, intOps, strOps, nil)
	if s.SSP != 0 {
		t.Errorf("SSP after discard: got %d want 0", s.SSP)
	}
}

func TestHandleGosubWithParams(t *testing.T) {
	// Sub-script: PUSH_INT_LOCAL 0; RETURN (returns its first int arg)
	subLookupKey := uint32(0x00000001)
	sub := &ScriptFile{
		Name:             "[proc,sub]",
		LookupKey:        subLookupKey,
		IntArgCount:      1,
		IntLocalCount:    1,
		StringArgCount:   0,
		StringLocalCount: 0,
		Opcodes:          []Opcode{OpPushIntLocal, OpReturn},
		IntOperands:      []int32{0, 0},
		StringOperands:   []string{"", ""},
	}

	// Main script: PUSH_CONSTANT_INT 5; GOSUB_WITH_PARAMS sub; RETURN
	mainLookupKey := uint32(0xFFFFFFFF)
	main := &ScriptFile{
		Name:             "[proc,main]",
		LookupKey:        mainLookupKey,
		IntArgCount:      0,
		IntLocalCount:    0,
		StringArgCount:   0,
		StringLocalCount: 0,
		Opcodes:          []Opcode{OpPushConstantInt, OpGosubWithParams, OpReturn},
		IntOperands:      []int32{5, int32(subLookupKey), 0},
		StringOperands:   []string{"", "", ""},
	}

	p := NewProvider()
	p.byKey[subLookupKey] = sub

	s := Init(main, nil, false, nil, nil)
	s.Provider = p

	if err := Execute(s); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if s.Execution != Finished {
		t.Errorf("Execution: got %v want Finished", s.Execution)
	}
	// Sub pushed its int local (=5) and returned; now 5 is on the int stack.
	s.ISP--
	if s.IntStack[s.ISP] != 5 {
		t.Errorf("GOSUB result: got %d want 5", s.IntStack[s.ISP])
	}
}

func TestHandleMes(t *testing.T) {
	mp := &mockPlayer{username: "Bob"}
	ops := []Opcode{OpPushConstantString, OpMes, OpReturn}
	intOps := []int32{0, 0, 0}
	strOps := []string{"hello", "", ""}
	runScript(t, ops, intOps, strOps, mp)
	if len(mp.messages) != 1 || mp.messages[0] != "hello" {
		t.Errorf("MessageGame: got %v want [hello]", mp.messages)
	}
}

func TestHandleMesWithoutPlayerErrors(t *testing.T) {
	// No self → handler should return error → Execute returns error.
	f := &ScriptFile{
		Name:           "test",
		Opcodes:        []Opcode{OpPushConstantString, OpMes, OpReturn},
		IntOperands:    []int32{0, 0, 0},
		StringOperands: []string{"hello", "", ""},
	}
	s := Init(f, nil, false, nil, nil) // no self
	err := Execute(s)
	if err == nil {
		t.Fatal("expected error from MES without player, got nil")
	}
	if s.Execution != Aborted {
		t.Errorf("Execution: got %v want Aborted", s.Execution)
	}
}

func TestHandleName(t *testing.T) {
	mp := &mockPlayer{username: "Alice"}
	ops := []Opcode{OpName, OpReturn}
	s := runScript(t, ops, nil, nil, mp)
	s.SSP--
	if s.StringStack[s.SSP] != "Alice" {
		t.Errorf("NAME: got %q want %q", s.StringStack[s.SSP], "Alice")
	}
}

func TestHandleConsole(t *testing.T) {
	// CONSOLE pops a string and discards it.
	ops := []Opcode{OpPushConstantString, OpConsole, OpReturn}
	intOps := []int32{0, 0, 0}
	strOps := []string{"debug msg", "", ""}
	s := runScript(t, ops, intOps, strOps, nil)
	if s.SSP != 0 {
		t.Errorf("SSP after CONSOLE: got %d want 0", s.SSP)
	}
}

func TestExecuteOpcountLimitHit(t *testing.T) {
	// BRANCH -1 branches back to itself: PC += -1, then PC++, net = 0. Infinite loop.
	f := &ScriptFile{
		Name:           "loop",
		Opcodes:        []Opcode{OpBranch},
		IntOperands:    []int32{-1},
		StringOperands: []string{""},
	}
	s := Init(f, nil, false, nil, nil)
	err := Execute(s)
	if err == nil {
		t.Fatal("expected opcount limit error, got nil")
	}
	if s.Execution != Aborted {
		t.Errorf("Execution: got %v want Aborted", s.Execution)
	}
	if s.OpCount < OpCountLimit {
		t.Errorf("OpCount: got %d want >= %d", s.OpCount, OpCountLimit)
	}
}

func TestPDelaySuspends(t *testing.T) {
	sf := &ScriptFile{
		Name: "test_pdelay",
		Opcodes: []Opcode{
			OpPushConstantInt, // push 5
			OpPDelay,
			OpReturn,
		},
		IntOperands:      []int32{5, 0, 0},
		StringOperands:   []string{"", "", ""},
		InstructionCount: 3,
	}
	mp := &mockPlayer{}
	state := Init(sf, mp, false, nil, nil)
	if err := Execute(state); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if state.Execution != Suspended {
		t.Errorf("Execution: got %v, want Suspended", state.Execution)
	}
	if len(mp.setDelayedCalls) != 1 || mp.setDelayedCalls[0] != 5 {
		t.Errorf("setDelayedCalls: got %v, want [5]", mp.setDelayedCalls)
	}
}

func TestPDelayRequiresActivePlayer(t *testing.T) {
	sf := &ScriptFile{
		Name: "test_pdelay_noself",
		Opcodes: []Opcode{
			OpPushConstantInt,
			OpPDelay,
			OpReturn,
		},
		IntOperands:      []int32{0, 0, 0},
		StringOperands:   []string{"", "", ""},
		InstructionCount: 3,
	}
	state := Init(sf, nil, false, nil, nil)
	if err := Execute(state); err == nil {
		t.Fatal("Execute: want error, got nil")
	}
}

func TestQueueOpcode(t *testing.T) {
	sf := &ScriptFile{
		Name: "test_queue",
		Opcodes: []Opcode{
			OpPushConstantInt, // push scriptID 77
			OpPushConstantInt, // push delay 3
			OpPushConstantInt, // push arg 42
			OpQueue,
			OpReturn,
		},
		IntOperands:      []int32{77, 3, 42, 0, 0},
		StringOperands:   []string{"", "", "", "", ""},
		InstructionCount: 5,
	}
	mp := &mockPlayer{}
	state := Init(sf, mp, false, nil, nil)
	if err := Execute(state); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if state.Execution != Finished {
		t.Errorf("Execution: got %v, want Finished", state.Execution)
	}
	if len(mp.enqueueCalls) != 1 {
		t.Fatalf("enqueueCalls: got %d, want 1", len(mp.enqueueCalls))
	}
	got := mp.enqueueCalls[0]
	want := mockEnqueue{ScriptID: 77, Delay: 3, IntArg: 42}
	if got != want {
		t.Errorf("enqueue: got %+v, want %+v", got, want)
	}
}
