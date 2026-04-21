package script

import (
	"testing"
)

func TestExecuteUnknownOpcodeAborts(t *testing.T) {
	// Opcode 9999 has no handler → Aborted + non-nil error.
	f := &ScriptFile{
		Name:           "test",
		Opcodes:        []Opcode{9999},
		IntOperands:    []int32{0},
		StringOperands: []string{""},
	}
	s := Init(f, nil, false, nil, nil)
	err := Execute(s)
	if err == nil {
		t.Fatal("expected error for unknown opcode, got nil")
	}
	if s.Execution != Aborted {
		t.Errorf("Execution: got %v want Aborted", s.Execution)
	}
}

func TestExecutePcOutOfRangeAborts(t *testing.T) {
	// Empty Opcodes slice: PC=0 is immediately out of range.
	f := &ScriptFile{
		Name:    "test",
		Opcodes: []Opcode{},
	}
	s := Init(f, nil, false, nil, nil)
	err := Execute(s)
	if err == nil {
		t.Fatal("expected error for pc out of range, got nil")
	}
	if s.Execution != Aborted {
		t.Errorf("Execution: got %v want Aborted", s.Execution)
	}
}

func TestInitSetsFields(t *testing.T) {
	f := minimalScript(OpReturn)
	f.IntArgCount = 2
	f.StringArgCount = 1
	f.IntLocalCount = 3
	f.StringLocalCount = 2

	mp := &mockPlayer{username: "Alice"}
	s := Init(f, mp, true, []int{10, 20}, []string{"hello"})

	if s.Self != mp {
		t.Error("Self not set")
	}
	if s.Pointers&PtrActivePlayer == 0 {
		t.Error("PtrActivePlayer not set")
	}
	if s.Protect != true {
		t.Error("Protect not set")
	}
	if s.PC != 0 {
		t.Errorf("PC: got %d want 0", s.PC)
	}
	if s.Execution != Running {
		t.Errorf("Execution: got %v want Running", s.Execution)
	}
	if s.IntLocals[0] != 10 || s.IntLocals[1] != 20 {
		t.Errorf("IntLocals: got %v want [10,20,...]", s.IntLocals)
	}
	if s.StringLocals[0] != "hello" {
		t.Errorf("StringLocals[0]: got %q want %q", s.StringLocals[0], "hello")
	}
	if cap(s.IntStack) != StackCapacity {
		t.Errorf("IntStack cap: got %d want %d", cap(s.IntStack), StackCapacity)
	}
}

// mockPlayer is defined here for use in runner_test and handlers_test.
// It is also used in handlers_test.go in the same package.
type mockPlayer struct {
	messages []string
	username string

	// S4: captured calls from the suspension + queue methods.
	setDelayedCalls []int
	enqueueCalls    []mockEnqueue
	stored          *ScriptState
	cleared         int
}

type mockEnqueue struct {
	ScriptID uint32
	Delay    int
	IntArg   int
}

func (m *mockPlayer) MessageGame(msg string) { m.messages = append(m.messages, msg) }
func (m *mockPlayer) Username() string       { return m.username }

func (m *mockPlayer) SetDelayed(ticks int) {
	m.setDelayedCalls = append(m.setDelayedCalls, ticks)
}
func (m *mockPlayer) EnqueueScript(id uint32, delay, arg int) {
	m.enqueueCalls = append(m.enqueueCalls, mockEnqueue{ScriptID: id, Delay: delay, IntArg: arg})
}
func (m *mockPlayer) StoreActiveScript(s *ScriptState) { m.stored = s }
func (m *mockPlayer) ClearActiveScript()               { m.cleared++ }
