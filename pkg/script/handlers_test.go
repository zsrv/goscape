package script

import (
	"bytes"
	"fmt"
	"log/slog"
	"slices"
	"strings"
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
	p.RegisterAt(subLookupKey, sub)

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
	// CONSOLE pops a string and discards it; without s.Log set, no output.
	ops := []Opcode{OpPushConstantString, OpConsole, OpReturn}
	intOps := []int32{0, 0, 0}
	strOps := []string{"debug msg", "", ""}
	s := runScript(t, ops, intOps, strOps, nil)
	if s.SSP != 0 {
		t.Errorf("SSP after CONSOLE: got %d want 0", s.SSP)
	}
}

func TestHandleConsole_EmitsToLogger(t *testing.T) {
	// CONSOLE forwards the popped string to s.Log if set, tagged with the
	// script name.
	f := &ScriptFile{
		Name:             "dbgScript",
		Opcodes:          []Opcode{OpPushConstantString, OpConsole, OpReturn},
		IntOperands:      []int32{0, 0, 0},
		StringOperands:   []string{"hello console", "", ""},
		InstructionCount: 3,
		IntLocalCount:    8,
		StringLocalCount: 8,
	}
	var buf bytes.Buffer
	s := Init(f, nil, true, nil, nil)
	s.Log = slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelInfo}))
	if err := Execute(s); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if s.SSP != 0 {
		t.Errorf("SSP after CONSOLE: got %d want 0", s.SSP)
	}
	out := buf.String()
	if !strings.Contains(out, "msg=") || !strings.Contains(out, "hello console") {
		t.Errorf("log missing msg: %q", out)
	}
	if !strings.Contains(out, "script=dbgScript") {
		t.Errorf("log missing script name: %q", out)
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
	state := Init(sf, mp, true, nil, nil)
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

// TestPDelayUnprotectedRejected verifies that a script started without
// protection gets the "script not protected" error. Closes S6l-D3 for
// P_DELAY (matches TS checkedHandler(ProtectedActivePlayer, ...)).
func TestPDelayUnprotectedRejected(t *testing.T) {
	mp := &mockPlayer{}
	sf := newSingleOp("p_delay_unprotected", OpPDelay)
	state := Init(sf, mp, false, nil, nil) // protect=false
	state.PushInt(1)

	err := Execute(state)
	if err == nil || err.Error() != "P_DELAY: script not protected" {
		t.Errorf("expected 'P_DELAY: script not protected', got %v", err)
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
	if got.ScriptID != 77 {
		t.Errorf("ScriptID: got %d, want 77", got.ScriptID)
	}
	if got.Delay != 3 {
		t.Errorf("Delay: got %d, want 3", got.Delay)
	}
	if !slices.Equal(got.IntArgs, []int{42}) {
		t.Errorf("IntArgs: got %v, want [42]", got.IntArgs)
	}
	if got.StringArgs != nil {
		t.Errorf("StringArgs: got %v, want nil", got.StringArgs)
	}
	if got.Type != QueueNormal {
		t.Errorf("Type: got %v, want QueueNormal", got.Type)
	}
}

func TestQueueVariants(t *testing.T) {
	cases := []struct {
		name  string
		op    Opcode
		qtype PlayerQueueType
	}{
		{"weak", OpWeakQueue, QueueWeak},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			sf := &ScriptFile{
				Name: "q_" + tc.name,
				Opcodes: []Opcode{
					OpPushConstantInt,
					OpPushConstantInt,
					OpPushConstantInt,
					tc.op,
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
			if len(mp.enqueueCalls) != 1 {
				t.Fatalf("enqueueCalls: got %d, want 1", len(mp.enqueueCalls))
			}
			got := mp.enqueueCalls[0]
			if got.ScriptID != 77 || got.Delay != 3 || got.Type != tc.qtype {
				t.Errorf("enqueue header: got ScriptID=%d Delay=%d Type=%v, want ScriptID=77 Delay=3 Type=%v",
					got.ScriptID, got.Delay, got.Type, tc.qtype)
			}
			if !slices.Equal(got.IntArgs, []int{42}) {
				t.Errorf("IntArgs: got %v, want [42]", got.IntArgs)
			}
			if got.StringArgs != nil {
				t.Errorf("StringArgs: got %v, want nil", got.StringArgs)
			}
		})
	}
}

// TestPopScriptArgs_Empty pins the empty-tags case: an empty type-tags
// string yields nil/nil. Mirrors TS PlayerOps.ts:1248-1263 with count=0.
func TestPopScriptArgs_Empty(t *testing.T) {
	state := Init(&ScriptFile{Name: "popscriptargs_empty"}, nil, false, nil, nil)
	state.PushString("")

	intArgs, stringArgs := popScriptArgs(state)

	if intArgs != nil {
		t.Errorf("intArgs: got %v, want nil", intArgs)
	}
	if stringArgs != nil {
		t.Errorf("stringArgs: got %v, want nil", stringArgs)
	}
	if state.SSP != 0 {
		t.Errorf("SSP after pop: got %d, want 0", state.SSP)
	}
}

// TestPopScriptArgs_AllInt pins the all-int case: tags="iii" with stack-
// pushed ints [1, 2, 3] (top of stack is 3) yields intArgs=[1, 2, 3],
// stringArgs=nil. Verifies tag-position order is preserved.
func TestPopScriptArgs_AllInt(t *testing.T) {
	state := Init(&ScriptFile{Name: "popscriptargs_allint"}, nil, false, nil, nil)
	state.PushInt(1)
	state.PushInt(2)
	state.PushInt(3)
	state.PushString("iii")

	intArgs, stringArgs := popScriptArgs(state)

	if !slices.Equal(intArgs, []int{1, 2, 3}) {
		t.Errorf("intArgs: got %v, want [1 2 3]", intArgs)
	}
	if stringArgs != nil {
		t.Errorf("stringArgs: got %v, want nil", stringArgs)
	}
	if state.ISP != 0 {
		t.Errorf("ISP after pop: got %d, want 0", state.ISP)
	}
}

// TestPopScriptArgs_AllString pins the all-string case: tags="sss" with
// stack-pushed strings ["a", "b", "c"] (top of stack is "c") yields
// stringArgs=["a", "b", "c"], intArgs=nil.
func TestPopScriptArgs_AllString(t *testing.T) {
	state := Init(&ScriptFile{Name: "popscriptargs_allstring"}, nil, false, nil, nil)
	state.PushString("a")
	state.PushString("b")
	state.PushString("c")
	state.PushString("sss")

	intArgs, stringArgs := popScriptArgs(state)

	if intArgs != nil {
		t.Errorf("intArgs: got %v, want nil", intArgs)
	}
	if !slices.Equal(stringArgs, []string{"a", "b", "c"}) {
		t.Errorf("stringArgs: got %v, want [a b c]", stringArgs)
	}
	if state.SSP != 0 {
		t.Errorf("SSP after pop: got %d, want 0", state.SSP)
	}
}

// TestPopScriptArgs_Mixed pins the mixed-type case from spec § Bundle 2
// § "Order semantics": tags="isi" with stack-pushed [1, "two", 3]
// yields intArgs=[1, 3] (tag-relative-int-order: i0 then i2), and
// stringArgs=["two"] (tag-relative-string-order: s1).
//
// Stack push order: PushInt(1), PushString("two"), PushInt(3),
// PushString("isi"). Top of int stack is 3; top of string stack is
// "isi". popScriptArgs first pops "isi" off the string stack, then
// loop i=2 (tag 'i') pops 3 off the int stack into intArgs[1], loop
// i=1 (tag 's') pops "two" off the string stack into stringArgs[0],
// loop i=0 (tag 'i') pops 1 off the int stack into intArgs[0].
func TestPopScriptArgs_Mixed(t *testing.T) {
	state := Init(&ScriptFile{Name: "popscriptargs_mixed"}, nil, false, nil, nil)
	state.PushInt(1)
	state.PushString("two")
	state.PushInt(3)
	state.PushString("isi")

	intArgs, stringArgs := popScriptArgs(state)

	if !slices.Equal(intArgs, []int{1, 3}) {
		t.Errorf("intArgs: got %v, want [1 3]", intArgs)
	}
	if !slices.Equal(stringArgs, []string{"two"}) {
		t.Errorf("stringArgs: got %v, want [two]", stringArgs)
	}
	if state.ISP != 0 {
		t.Errorf("ISP after pop: got %d, want 0", state.ISP)
	}
	if state.SSP != 0 {
		t.Errorf("SSP after pop: got %d, want 0", state.SSP)
	}
}

// TestPopScriptArgs_ReverseOrder pins the reverse-pop semantics that
// distinguish popScriptArgs from a naive forward-iteration: with
// tags="iii" and stack-pushed [10, 20, 30] (i.e. PushInt(10),
// PushInt(20), PushInt(30)), the result is intArgs=[10, 20, 30] —
// NOT [30, 20, 10]. The TS i=count-1→0 loop combined with the
// intIdx=intCount-1→0 decrementer preserves tag-position order even
// though pops are last-in-first-out. This test pins the inversion.
func TestPopScriptArgs_ReverseOrder(t *testing.T) {
	state := Init(&ScriptFile{Name: "popscriptargs_reverseorder"}, nil, false, nil, nil)
	state.PushInt(10)
	state.PushInt(20)
	state.PushInt(30)
	state.PushString("iii")

	intArgs, _ := popScriptArgs(state)

	if !slices.Equal(intArgs, []int{10, 20, 30}) {
		t.Errorf("intArgs: got %v, want [10 20 30] (tag-position order, not LIFO order)", intArgs)
	}
}

// TestStrongQueueDelayNullRejected pins divergence α: TS
// PlayerOps.ts:99 wraps the popped delay with check(..., NumberNotNull).
// goscape's pre-NAI-26 enqueueTyped helper missed this wrap. This test
// pushes tags="" (empty popScriptArgs), delay=-1 (NULL), scriptID=77
// and expects "STRONGQUEUE: input number was null(-1)".
func TestStrongQueueDelayNullRejected(t *testing.T) {
	sf := newSingleOp("strongqueue_delay_null", OpStrongQueue)
	mp := &mockPlayer{}
	state := Init(sf, mp, false, nil, nil)
	state.PushInt(77)    // scriptID
	state.PushInt(-1)    // delay (NULL)
	state.PushString("") // type-tags (no script args)

	err := Execute(state)
	if err == nil {
		t.Fatalf("Execute: want error for delay=-1, got nil")
	}
	want := "STRONGQUEUE: input number was null(-1)"
	if !strings.Contains(err.Error(), want) {
		t.Errorf("error: got %q, want substring %q", err.Error(), want)
	}
	if len(mp.enqueueCalls) != 0 {
		t.Errorf("enqueueCalls: got %d, want 0 (rejection should not enqueue)", len(mp.enqueueCalls))
	}
}

// TestStrongQueueEmptyScriptArgs pins divergence β with an empty
// type-tags string: STRONGQUEUE with tags="", delay=3, scriptID=77
// enqueues with IntArgs=nil, StringArgs=nil.
func TestStrongQueueEmptyScriptArgs(t *testing.T) {
	sf := newSingleOp("strongqueue_empty_args", OpStrongQueue)
	mp := &mockPlayer{}
	state := Init(sf, mp, false, nil, nil)
	state.PushInt(77)    // scriptID
	state.PushInt(3)     // delay
	state.PushString("") // type-tags (no script args)

	if err := Execute(state); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if len(mp.enqueueCalls) != 1 {
		t.Fatalf("enqueueCalls: got %d, want 1", len(mp.enqueueCalls))
	}
	got := mp.enqueueCalls[0]
	if got.ScriptID != 77 || got.Delay != 3 || got.Type != QueueStrong {
		t.Errorf("enqueue header: got ScriptID=%d Delay=%d Type=%v, want 77/3/QueueStrong",
			got.ScriptID, got.Delay, got.Type)
	}
	if got.IntArgs != nil {
		t.Errorf("IntArgs: got %v, want nil", got.IntArgs)
	}
	if got.StringArgs != nil {
		t.Errorf("StringArgs: got %v, want nil", got.StringArgs)
	}
}

// TestStrongQueueAllIntScriptArgs pins divergence β with an all-int
// type-tags string: STRONGQUEUE with tags="iii", three int args
// (10, 20, 30), delay=5, scriptID=77 enqueues with
// IntArgs=[10, 20, 30], StringArgs=nil.
func TestStrongQueueAllIntScriptArgs(t *testing.T) {
	sf := newSingleOp("strongqueue_allint_args", OpStrongQueue)
	mp := &mockPlayer{}
	state := Init(sf, mp, false, nil, nil)
	state.PushInt(77)       // scriptID (deepest int)
	state.PushInt(5)        // delay
	state.PushInt(10)       // arg0
	state.PushInt(20)       // arg1
	state.PushInt(30)       // arg2 (top of int stack)
	state.PushString("iii") // type-tags

	if err := Execute(state); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if len(mp.enqueueCalls) != 1 {
		t.Fatalf("enqueueCalls: got %d, want 1", len(mp.enqueueCalls))
	}
	got := mp.enqueueCalls[0]
	if got.ScriptID != 77 || got.Delay != 5 || got.Type != QueueStrong {
		t.Errorf("enqueue header: got ScriptID=%d Delay=%d Type=%v, want 77/5/QueueStrong",
			got.ScriptID, got.Delay, got.Type)
	}
	if !slices.Equal(got.IntArgs, []int{10, 20, 30}) {
		t.Errorf("IntArgs: got %v, want [10 20 30]", got.IntArgs)
	}
	if got.StringArgs != nil {
		t.Errorf("StringArgs: got %v, want nil", got.StringArgs)
	}
}

// TestStrongQueuePopsMixedScriptArgs pins divergence β with a mixed-
// type type-tags string: STRONGQUEUE with tags="is", arg-int=99,
// arg-string="hello", delay=2, scriptID=77 enqueues with IntArgs=[99],
// StringArgs=["hello"]. Pin shape lifts directly from spec § Bundle 2 §
// "Order semantics".
func TestStrongQueuePopsMixedScriptArgs(t *testing.T) {
	sf := newSingleOp("strongqueue_mixed_args", OpStrongQueue)
	mp := &mockPlayer{}
	state := Init(sf, mp, false, nil, nil)
	// Stack push order — caller pre-loads the typed-arg block in
	// tag-position order, so for tags="is": int arg first (deepest),
	// then string arg, then delay, then scriptID — but ordering on
	// the int and string stacks is independent.
	//
	// Int stack from bottom: [scriptID=77, delay=2, intArg=99].
	// String stack from bottom: [stringArg="hello", tags="is"].
	state.PushInt(77)
	state.PushInt(2)
	state.PushInt(99)
	state.PushString("hello")
	state.PushString("is")

	if err := Execute(state); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if len(mp.enqueueCalls) != 1 {
		t.Fatalf("enqueueCalls: got %d, want 1", len(mp.enqueueCalls))
	}
	got := mp.enqueueCalls[0]
	if got.ScriptID != 77 || got.Delay != 2 || got.Type != QueueStrong {
		t.Errorf("enqueue header: got ScriptID=%d Delay=%d Type=%v, want 77/2/QueueStrong",
			got.ScriptID, got.Delay, got.Type)
	}
	if !slices.Equal(got.IntArgs, []int{99}) {
		t.Errorf("IntArgs: got %v, want [99]", got.IntArgs)
	}
	if !slices.Equal(got.StringArgs, []string{"hello"}) {
		t.Errorf("StringArgs: got %v, want [hello]", got.StringArgs)
	}
}

// TestLongQueuePopsFourInts pins divergences ζ + η: LONGQUEUE pops 4
// ints (scriptID, delay, arg, logoutAction — the 4th distinguishes it
// from QUEUE/WEAKQUEUE/STRONGQUEUE) and enqueues with the 2-element
// args array [logoutAction, arg] (logoutAction-first per TS
// PlayerOps.ts:179, even though logoutAction is the last-pushed and
// first-popped int).
func TestLongQueuePopsFourInts(t *testing.T) {
	sf := newSingleOp("longqueue_4ints", OpLongQueue)
	mp := &mockPlayer{}
	state := Init(sf, mp, false, nil, nil)
	// Stack push order: scriptID first (deepest), then delay, then arg,
	// then logoutAction (top). PopInt order at handler entry:
	// logoutAction → arg → delay → scriptID.
	state.PushInt(77) // scriptID
	state.PushInt(3)  // delay
	state.PushInt(99) // arg
	state.PushInt(42) // logoutAction (top)

	if err := Execute(state); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if len(mp.enqueueCalls) != 1 {
		t.Fatalf("enqueueCalls: got %d, want 1", len(mp.enqueueCalls))
	}
	got := mp.enqueueCalls[0]
	if got.ScriptID != 77 || got.Delay != 3 || got.Type != QueueLong {
		t.Errorf("enqueue header: got ScriptID=%d Delay=%d Type=%v, want 77/3/QueueLong",
			got.ScriptID, got.Delay, got.Type)
	}
	if !slices.Equal(got.IntArgs, []int{42, 99}) {
		t.Errorf("IntArgs: got %v, want [42 99] (logoutAction, arg per TS PlayerOps.ts:179)",
			got.IntArgs)
	}
	if got.StringArgs != nil {
		t.Errorf("StringArgs: got %v, want nil", got.StringArgs)
	}
}

// TestPDelayNullRejected pins divergence κ: TS PlayerOps.ts:377 wraps
// the popped n with check(..., NumberNotNull). Pushes -1 (NULL) → the
// handler returns "P_DELAY: input number was null(-1)" without
// calling SetDelayed.
func TestPDelayNullRejected(t *testing.T) {
	mp := &mockPlayer{}
	sf := newSingleOp("p_delay_null", OpPDelay)
	state := Init(sf, mp, true, nil, nil) // protect=true (P_DELAY needs protection)
	state.PushInt(-1)

	err := Execute(state)
	if err == nil {
		t.Fatalf("Execute: want error, got nil")
	}
	want := "P_DELAY: input number was null(-1)"
	if !strings.Contains(err.Error(), want) {
		t.Errorf("error: got %q, want substring %q", err.Error(), want)
	}
	if len(mp.setDelayedCalls) != 0 {
		t.Errorf("setDelayedCalls: got %v, want [] (rejection should not call SetDelayed)",
			mp.setDelayedCalls)
	}
}

// TestQueueScriptNotFound pins divergences γ + δ + ε + θ:
// STRONGQUEUE / WEAKQUEUE / QUEUE / LONGQUEUE all propagate the
// EnqueueScriptArgs script-missing error to their caller via the
// handler's error return. Mirrors TS PlayerOps.ts:103-105 (STRONG),
// :127-129 (WEAK), :152-154 (NORMAL), :175-177 (LONG). The mock
// player is pre-configured to return the script-missing error;
// the handler must propagate it up.
func TestQueueScriptNotFound(t *testing.T) {
	cases := []struct {
		name  string
		op    Opcode
		setup func(state *ScriptState) // pushes scriptID/delay/[arg|tags...] in op-specific order
	}{
		{
			name: "STRONGQUEUE",
			op:   OpStrongQueue,
			setup: func(state *ScriptState) {
				state.PushInt(77)    // scriptID
				state.PushInt(3)     // delay
				state.PushString("") // tags=""
			},
		},
		{
			name: "WEAKQUEUE",
			op:   OpWeakQueue,
			setup: func(state *ScriptState) {
				state.PushInt(77) // scriptID
				state.PushInt(3)  // delay
				state.PushInt(42) // arg
			},
		},
		{
			name: "QUEUE",
			op:   OpQueue,
			setup: func(state *ScriptState) {
				state.PushInt(77) // scriptID
				state.PushInt(3)  // delay
				state.PushInt(42) // arg
			},
		},
		{
			name: "LONGQUEUE",
			op:   OpLongQueue,
			setup: func(state *ScriptState) {
				state.PushInt(77) // scriptID
				state.PushInt(3)  // delay
				state.PushInt(42) // arg
				state.PushInt(99) // logoutAction
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			mp := &mockPlayer{
				enqueueScriptArgsReturnErr: fmt.Errorf("unable to find queue script: 77"),
			}
			sf := newSingleOp(tc.name+"_notfound", tc.op)
			state := Init(sf, mp, false, nil, nil)
			tc.setup(state)

			err := Execute(state)
			if err == nil {
				t.Fatalf("Execute: want error, got nil")
			}
			want := "unable to find queue script: 77"
			if !strings.Contains(err.Error(), want) {
				t.Errorf("error: got %q, want substring %q", err.Error(), want)
			}
			// The mock records the call (the error happens inside the
			// real (*Player).EnqueueScriptArgs in production; the mock
			// records the call AND returns the configured error to the
			// handler).
			if len(mp.enqueueCalls) != 1 {
				t.Errorf("enqueueCalls: got %d, want 1 (mock should record before returning)",
					len(mp.enqueueCalls))
			}
		})
	}
}

// -- P_ARRIVEDELAY tests (NAI-82) ----------------------------------------
//
// TS PlayerOps.ts:357-366: if state.activePlayer.lastMovement < World.currentTick
// then return (no-op); else SetDelayed(0) + Suspended. The 2-tick window arises
// because lastMovement is written to currentTick + 1 after a moving tick.

// TestPArriveDelaySuspendsWhenMovedThisTick: lastMovement = currentTick + 1
// (the value written this tick by Player.resolveMovement).
// Gate condition: 101 < 100 is false ⇒ suspend.
func TestPArriveDelaySuspendsWhenMovedThisTick(t *testing.T) {
	mp := &mockPlayer{lastMovement: 101}
	w := &mockWorld{tick: 100}
	sf := newSingleOp("p_arrivedelay_moved_this_tick", OpPArriveDelay)
	state := Init(sf, mp, true, nil, nil)
	state.World = w

	if err := Execute(state); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if state.Execution != Suspended {
		t.Errorf("Execution: got %v, want Suspended", state.Execution)
	}
	if len(mp.setDelayedCalls) != 1 || mp.setDelayedCalls[0] != 0 {
		t.Errorf("setDelayedCalls: got %v, want [0]", mp.setDelayedCalls)
	}
}

// TestPArriveDelaySuspendsWhenMovedLastTick: lastMovement = currentTick (the
// boundary case — moved on tick T-1 means lastMovement was set to T-1+1 = T).
// Gate condition: 100 < 100 is false ⇒ suspend. Pins the inclusive boundary.
func TestPArriveDelaySuspendsWhenMovedLastTick(t *testing.T) {
	mp := &mockPlayer{lastMovement: 100}
	w := &mockWorld{tick: 100}
	sf := newSingleOp("p_arrivedelay_moved_last_tick", OpPArriveDelay)
	state := Init(sf, mp, true, nil, nil)
	state.World = w

	if err := Execute(state); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if state.Execution != Suspended {
		t.Errorf("Execution: got %v, want Suspended", state.Execution)
	}
	if len(mp.setDelayedCalls) != 1 || mp.setDelayedCalls[0] != 0 {
		t.Errorf("setDelayedCalls: got %v, want [0]", mp.setDelayedCalls)
	}
}

// TestPArriveDelayNoOpWhenMovedTwoTicksAgo: lastMovement = currentTick - 1
// (the first tick on which the gate becomes a no-op).
// Gate condition: 99 < 100 is true ⇒ return early.
func TestPArriveDelayNoOpWhenMovedTwoTicksAgo(t *testing.T) {
	mp := &mockPlayer{lastMovement: 99}
	w := &mockWorld{tick: 100}
	sf := newSingleOp("p_arrivedelay_moved_two_ticks_ago", OpPArriveDelay)
	state := Init(sf, mp, true, nil, nil)
	state.World = w

	if err := Execute(state); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if state.Execution != Finished {
		t.Errorf("Execution: got %v, want Finished (no-op should let OpReturn complete)", state.Execution)
	}
	if len(mp.setDelayedCalls) != 0 {
		t.Errorf("setDelayedCalls: got %v, want [] (no-op must not call SetDelayed)", mp.setDelayedCalls)
	}
}

// TestPArriveDelayNoOpWhenNeverMoved: lastMovement = 0 (zero-value, never
// moved). Gate condition: 0 < 100 is true ⇒ return early. Pins zero-value.
func TestPArriveDelayNoOpWhenNeverMoved(t *testing.T) {
	mp := &mockPlayer{lastMovement: 0}
	w := &mockWorld{tick: 100}
	sf := newSingleOp("p_arrivedelay_never_moved", OpPArriveDelay)
	state := Init(sf, mp, true, nil, nil)
	state.World = w

	if err := Execute(state); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if state.Execution != Finished {
		t.Errorf("Execution: got %v, want Finished", state.Execution)
	}
	if len(mp.setDelayedCalls) != 0 {
		t.Errorf("setDelayedCalls: got %v, want []", mp.setDelayedCalls)
	}
}

// TestPArriveDelayUnprotectedRejected: TS uses checkedHandler(ProtectedActivePlayer);
// scripts started with protect=false must reject. Mirrors TestPDelayUnprotectedRejected.
func TestPArriveDelayUnprotectedRejected(t *testing.T) {
	mp := &mockPlayer{}
	sf := newSingleOp("p_arrivedelay_unprotected", OpPArriveDelay)
	state := Init(sf, mp, false, nil, nil) // protect=false

	err := Execute(state)
	if err == nil || err.Error() != "P_ARRIVEDELAY: script not protected" {
		t.Errorf("expected 'P_ARRIVEDELAY: script not protected', got %v", err)
	}
	if len(mp.setDelayedCalls) != 0 {
		t.Errorf("setDelayedCalls: got %v, want [] (rejection must not mutate)", mp.setDelayedCalls)
	}
}

// TestPArriveDelayRequiresActivePlayer: no Self ⇒ requireProtectedActivePlayer
// chains through requireActivePlayer's "no active player" message.
// Mirrors TestPDelayRequiresActivePlayer.
func TestPArriveDelayRequiresActivePlayer(t *testing.T) {
	sf := newSingleOp("p_arrivedelay_no_self", OpPArriveDelay)
	state := Init(sf, nil, false, nil, nil)

	err := Execute(state)
	if err == nil || err.Error() != "P_ARRIVEDELAY: no active player" {
		t.Errorf("expected 'P_ARRIVEDELAY: no active player', got %v", err)
	}
}

// TestPArriveDelayRequiresWorld: handler reads s.World.CurrentTick() to
// evaluate its gate; missing world must return a clean error rather than
// nil-deref. Mirrors the established sibling-handler convention
// (handlePushVars / handleMapClock / handlePlayerCount etc.).
func TestPArriveDelayRequiresWorld(t *testing.T) {
	mp := &mockPlayer{lastMovement: 101}
	sf := newSingleOp("p_arrivedelay_no_world", OpPArriveDelay)
	state := Init(sf, mp, true, nil, nil)
	// state.World intentionally left nil

	err := Execute(state)
	if err == nil || err.Error() != "P_ARRIVEDELAY: no world" {
		t.Errorf("expected 'P_ARRIVEDELAY: no world', got %v", err)
	}
	if len(mp.setDelayedCalls) != 0 {
		t.Errorf("setDelayedCalls: got %v, want [] (rejection must not mutate)", mp.setDelayedCalls)
	}
}
