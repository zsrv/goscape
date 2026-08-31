package script

import (
	"testing"
)

func newTestState(f *ScriptFile) *ScriptState {
	return Init(f, nil, false, nil, nil)
}

func minimalScript(ops ...Opcode) *ScriptFile {
	n := len(ops)
	f := &ScriptFile{
		Name:             "test",
		Opcodes:          ops,
		IntOperands:      make([]int32, n),
		StringOperands:   make([]string, n),
		InstructionCount: uint32(n),
		IntLocalCount:    4,
		StringLocalCount: 4,
	}
	return f
}

func TestPushPopIntRoundTrip(t *testing.T) {
	s := newTestState(minimalScript(OpReturn))
	s.PushInt(42)
	s.PushInt(99)
	if got := s.PopInt(); got != 99 {
		t.Errorf("PopInt: got %d want 99", got)
	}
	if got := s.PopInt(); got != 42 {
		t.Errorf("PopInt: got %d want 42", got)
	}
}

func TestPushPopStringRoundTrip(t *testing.T) {
	s := newTestState(minimalScript(OpReturn))
	s.PushString("alpha")
	s.PushString("beta")
	if got := s.PopString(); got != "beta" {
		t.Errorf("PopString: got %q want %q", got, "beta")
	}
	if got := s.PopString(); got != "alpha" {
		t.Errorf("PopString: got %q want %q", got, "alpha")
	}
}

func TestPopEmptyIntStackReturnsZero(t *testing.T) {
	s := newTestState(minimalScript(OpReturn))
	if got := s.PopInt(); got != 0 {
		t.Errorf("PopInt on empty stack: got %d want 0", got)
	}
}

func TestPopEmptyStringStackReturnsEmpty(t *testing.T) {
	s := newTestState(minimalScript(OpReturn))
	if got := s.PopString(); got != "" {
		t.Errorf("PopString on empty stack: got %q want %q", got, "")
	}
}

// TestPushInt_GrowsBeyondStackCapacity pins script-core-3: PushInt must
// grow the int stack past StackCapacity rather than panic. Mirrors TS
// ScriptState.pushInt (ScriptState.ts:333-351) writing into a JS array
// that grows unbounded.
func TestPushInt_GrowsBeyondStackCapacity(t *testing.T) {
	s := newTestState(minimalScript(OpReturn))
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("PushInt past StackCapacity panicked: %v — TS ScriptState.pushInt (ScriptState.ts:333-351) grows JS array unbounded (script-core-3)", r)
		}
	}()
	for i := range StackCapacity + 5 {
		s.PushInt(i)
	}
	if s.ISP != StackCapacity+5 {
		t.Errorf("ISP after %d pushes: got %d, want %d", StackCapacity+5, s.ISP, StackCapacity+5)
	}
	if got := s.IntStack[StackCapacity+4]; got != StackCapacity+4 {
		t.Errorf("IntStack[%d]: got %d, want %d", StackCapacity+4, got, StackCapacity+4)
	}
}

// TestPushString_GrowsBeyondStackCapacity pins script-core-3 for the string
// stack: PushString must grow rather than panic.
func TestPushString_GrowsBeyondStackCapacity(t *testing.T) {
	s := newTestState(minimalScript(OpReturn))
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("PushString past StackCapacity panicked: %v — TS ScriptState.pushString grows JS array unbounded (script-core-3)", r)
		}
	}()
	for range StackCapacity + 5 {
		s.PushString("x")
	}
	if s.SSP != StackCapacity+5 {
		t.Errorf("SSP after %d pushes: got %d, want %d", StackCapacity+5, s.SSP, StackCapacity+5)
	}
	if got := s.StringStack[StackCapacity+4]; got != "x" {
		t.Errorf("StringStack[%d]: got %q, want %q", StackCapacity+4, got, "x")
	}
}

func TestGosubCallRestoresFrame(t *testing.T) {
	// Set up a state with some locals, then GosubCall into a target, then Return.
	main := minimalScript(OpReturn)
	main.IntLocalCount = 3

	target := minimalScript(OpReturn)
	target.IntLocalCount = 2

	s := Init(main, nil, false, []int{1, 2, 3}, nil)
	origIntLocals := []int{s.IntLocals[0], s.IntLocals[1], s.IntLocals[2]}

	// GosubCall with no int/string args.
	s.GosubCall(target, nil, nil)

	if s.PC != -1 {
		t.Errorf("GosubCall: PC should be -1, got %d", s.PC)
	}
	if s.Script != target {
		t.Error("GosubCall: Script should be target")
	}

	// Mutate new locals.
	s.IntLocals[0] = 99

	// Return should restore original locals.
	if err := s.Return(); err != nil {
		t.Fatalf("Return: %v", err)
	}
	if s.Script != main {
		t.Error("Return: Script should be main")
	}
	for i, want := range origIntLocals {
		if s.IntLocals[i] != want {
			t.Errorf("Return: IntLocals[%d] = %d, want %d", i, s.IntLocals[i], want)
		}
	}
}

func TestReturnEmptyFramesFinishes(t *testing.T) {
	s := newTestState(minimalScript(OpReturn))
	if err := s.Return(); err != nil {
		t.Fatalf("Return: %v", err)
	}
	if s.Execution != Finished {
		t.Errorf("Execution: got %v want Finished", s.Execution)
	}
}

func TestScriptStateSplitFieldsZeroValue(t *testing.T) {
	s := &ScriptState{}
	if s.SplitPages != nil {
		t.Errorf("fresh ScriptState.SplitPages: got %v, want nil", s.SplitPages)
	}
	if s.SplitMesanim != 0 {
		t.Errorf("fresh ScriptState.SplitMesanim: got %d, want 0", s.SplitMesanim)
	}
}
