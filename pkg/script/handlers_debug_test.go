package script

import (
	"strings"
	"testing"
)

func TestErrorAborts(t *testing.T) {
	sf := &ScriptFile{
		Name:             "err",
		Opcodes:          []Opcode{OpPushConstantString, OpError, OpReturn},
		IntOperands:      []int32{0, 0, 0},
		StringOperands:   []string{"bad thing", "", ""},
		InstructionCount: 3,
	}
	state := Init(sf, nil, false, nil, nil)
	err := Execute(state)
	if err == nil {
		t.Fatal("Execute: want error")
	}
	if !strings.Contains(err.Error(), "bad thing") {
		t.Errorf("err msg: got %v, want containing 'bad thing'", err)
	}
	if state.Execution != Aborted {
		t.Errorf("Execution: got %v, want Aborted", state.Execution)
	}
}

func TestTimeSpentReturnsPlaytime(t *testing.T) {
	sf := &ScriptFile{
		Name:             "ts",
		Opcodes:          []Opcode{OpTimeSpent, OpReturn},
		IntOperands:      []int32{0, 0},
		StringOperands:   []string{"", ""},
		InstructionCount: 2,
	}
	mp := &mockPlayer{username: "x", playtime: 42}
	state := Init(sf, mp, false, nil, nil)
	if err := Execute(state); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if got := state.PopInt(); got != 42 {
		t.Errorf("TimeSpent: got %d, want 42", got)
	}
}

func TestTimeSpentRequiresActivePlayer(t *testing.T) {
	sf := &ScriptFile{
		Name:             "ts_noself",
		Opcodes:          []Opcode{OpTimeSpent, OpReturn},
		IntOperands:      []int32{0, 0},
		StringOperands:   []string{"", ""},
		InstructionCount: 2,
	}
	state := Init(sf, nil, false, nil, nil)
	if err := Execute(state); err == nil {
		t.Fatal("Execute: want error with no active player")
	}
}
