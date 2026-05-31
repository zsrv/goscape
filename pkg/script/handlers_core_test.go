package script

import (
	"strings"
	"testing"
)

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
	prov.RegisterAt(0xAAAA, scriptA)
	prov.RegisterAt(0xBBBB, scriptB)
	prov.RegisterAt(0xCCCC, scriptC)

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

// TestGosubBasic verifies the no-params GOSUB pops a script id and
// invokes the target with a saved frame. After the target's RETURN,
// control resumes the caller's next instruction.
func TestGosubBasic(t *testing.T) {
	target := &ScriptFile{
		Name:             "[gosub_target]",
		LookupKey:        0x4321,
		Opcodes:          []Opcode{OpPushConstantString, OpMes, OpReturn},
		IntOperands:      []int32{0, 0, 0},
		StringOperands:   []string{"sub", "", ""},
		InstructionCount: 3,
	}
	caller := &ScriptFile{
		Name: "[gosub_caller]",
		Opcodes: []Opcode{
			OpPushConstantInt,    // push target id
			OpGosub,              // GOSUB target
			OpPushConstantString, // push "main"
			OpMes,                // emit "main" after return
			OpReturn,
		},
		IntOperands:      []int32{int32(0x4321), 0, 0, 0, 0},
		StringOperands:   []string{"", "", "main", "", ""},
		InstructionCount: 5,
	}
	prov := NewProvider()
	prov.RegisterAt(0x4321, target)

	mp := &mockPlayer{}
	state := Init(caller, mp, false, nil, nil)
	state.Provider = prov
	if err := Execute(state); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if len(mp.messages) != 2 || mp.messages[0] != "sub" || mp.messages[1] != "main" {
		t.Errorf("messages: got %v, want [sub main]", mp.messages)
	}
}

// TestHandleGosub_FrameStackOverflow_ReturnsErrorNotPanic pins
// script-core-2: TS `CoreOps.ts:194-214` gates `if (state.fp >= 50) throw
// 'stack overflow'` at the TOP of the handler, which TS Execute's catch
// converts to ScriptExecution.Aborted. goscape's pre-fix `GosubCall`
// (state.go:509-511) panicked when `FrameSP >= FrameCapacity`, so a
// pathological / miscompiled script that GOSUB'd past the cap crashed
// the host goroutine rather than aborting gracefully. The fix gates at
// the handler's top and returns an error; the runner's
// `if err := h(s); err != nil { s.Execution=Aborted; return err }` path
// then handles it like every other handler error.
func TestHandleGosub_FrameStackOverflow_ReturnsErrorNotPanic(t *testing.T) {
	target := &ScriptFile{
		Name:             "[gosub_target]",
		LookupKey:        0x4321,
		Opcodes:          []Opcode{OpReturn},
		IntOperands:      []int32{0},
		StringOperands:   []string{""},
		InstructionCount: 1,
	}
	caller := &ScriptFile{
		Name:             "[gosub_caller]",
		Opcodes:          []Opcode{OpGosub},
		IntOperands:      []int32{0},
		StringOperands:   []string{""},
		InstructionCount: 1,
	}
	prov := NewProvider()
	prov.RegisterAt(0x4321, target)

	state := Init(caller, nil, false, nil, nil)
	state.Provider = prov
	state.FrameSP = FrameCapacity // saturate the frame stack
	state.PushInt(0x4321)         // target id (consumed by handleGosub's PopInt)

	var panicked bool
	var panicVal any
	err := func() (e error) {
		defer func() {
			if r := recover(); r != nil {
				panicked = true
				panicVal = r
			}
		}()
		return handleGosub(state)
	}()

	if panicked {
		t.Fatalf("handleGosub at FrameSP=%d panicked with %v — TS CoreOps.ts:194-214 throws (caught → Aborted) gracefully; goscape must return error not panic (script-core-2)", state.FrameSP, panicVal)
	}
	if err == nil {
		t.Fatalf("handleGosub at FrameSP=FrameCapacity must return an error; got nil")
	}
	if !strings.Contains(err.Error(), "stack overflow") {
		t.Errorf("error must mention 'stack overflow' (mirrors TS throw); got %q", err.Error())
	}
}

// TestHandleGosubWithParams_FrameStackOverflow_ReturnsErrorNotPanic pins
// the same cap-gate behaviour on the GOSUB_WITH_PARAMS handler (handlers.go).
func TestHandleGosubWithParams_FrameStackOverflow_ReturnsErrorNotPanic(t *testing.T) {
	target := &ScriptFile{
		Name:             "[gwp_target]",
		LookupKey:        0x9876,
		Opcodes:          []Opcode{OpReturn},
		IntOperands:      []int32{0},
		StringOperands:   []string{""},
		InstructionCount: 1,
	}
	// Caller uses GOSUB_WITH_PARAMS — target id is in the operand, not the stack.
	caller := &ScriptFile{
		Name:             "[gwp_caller]",
		Opcodes:          []Opcode{OpGosubWithParams},
		IntOperands:      []int32{int32(0x9876)},
		StringOperands:   []string{""},
		InstructionCount: 1,
	}
	prov := NewProvider()
	prov.RegisterAt(0x9876, target)

	state := Init(caller, nil, false, nil, nil)
	state.Provider = prov
	state.FrameSP = FrameCapacity

	var panicked bool
	var panicVal any
	err := func() (e error) {
		defer func() {
			if r := recover(); r != nil {
				panicked = true
				panicVal = r
			}
		}()
		return handleGosubWithParams(state)
	}()

	if panicked {
		t.Fatalf("handleGosubWithParams at FrameSP=%d panicked with %v — must return error not panic (script-core-2)", state.FrameSP, panicVal)
	}
	if err == nil {
		t.Fatalf("handleGosubWithParams at FrameSP=FrameCapacity must return an error; got nil")
	}
	if !strings.Contains(err.Error(), "stack overflow") {
		t.Errorf("error must mention 'stack overflow'; got %q", err.Error())
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
	prov.RegisterAt(0x1234, target)

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
	prov.RegisterAt(0x5678, target)

	state := Init(caller, nil, false, nil, nil)
	state.Provider = prov
	if err := Execute(state); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if got := state.PopInt(); got != 77 {
		t.Errorf("PopInt after JUMP_WITH_PARAMS target returned: got %d, want 77", got)
	}
}

// TestGosubPlainPopsDeclaredArgs pins L27: plain GOSUB (dynamic dispatch,
// proc id on the stack) must pop the callee's declared args, mirroring TS
// setupNewScript which always pops intArgCount/stringArgCount.
func TestGosubPlainPopsDeclaredArgs(t *testing.T) {
	// Target declares 1 int arg; body pushes local 0 back so the caller
	// can observe the popped value.
	target := &ScriptFile{
		Name:             "[gosub_arg_target]",
		LookupKey:        0x7711,
		IntArgCount:      1,
		IntLocalCount:    1,
		Opcodes:          []Opcode{OpPushIntLocal, OpReturn},
		IntOperands:      []int32{0, 0},
		StringOperands:   []string{"", ""},
		InstructionCount: 2,
	}
	// Caller: push arg 88, push target id, plain GOSUB.
	caller := &ScriptFile{
		Name:             "[gosub_arg_caller]",
		LookupKey:        0xFFFFFFFF,
		Opcodes:          []Opcode{OpPushConstantInt, OpPushConstantInt, OpGosub, OpReturn},
		IntOperands:      []int32{88, int32(0x7711), 0, 0},
		StringOperands:   []string{"", "", "", ""},
		InstructionCount: 4,
	}
	prov := NewProvider()
	prov.RegisterAt(0x7711, target)

	state := Init(caller, nil, false, nil, nil)
	state.Provider = prov
	if err := Execute(state); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if got := state.PopInt(); got != 88 {
		t.Errorf("plain GOSUB declared-arg: got %d, want 88", got)
	}
}

// TestOpCountCapAllowsTSLimit pins L30: the runner aborts on
// OpCount > OpCountLimit (strict `>`), so OpCountLimit+1 opcodes execute
// before the abort — matching TS ScriptRunner.ts:144 (`opcount > 500_000`).
func TestOpCountCapAllowsTSLimit(t *testing.T) {
	// Self-jumping script: push own id, JUMP to self → infinite loop.
	const selfID = 0x9999
	loop := &ScriptFile{
		Name:             "[opcount_loop]",
		LookupKey:        selfID,
		Opcodes:          []Opcode{OpPushConstantInt, OpJump},
		IntOperands:      []int32{int32(selfID), 0},
		InstructionCount: 2,
	}
	prov := NewProvider()
	prov.RegisterAt(selfID, loop)

	state := Init(loop, nil, false, nil, nil)
	state.Provider = prov
	if err := Execute(state); err == nil {
		t.Fatal("expected opcount limit error, got nil")
	}
	if state.OpCount != OpCountLimit+1 {
		t.Errorf("OpCount at abort: got %d, want %d (TS executes limit+1 opcodes)", state.OpCount, OpCountLimit+1)
	}
}
