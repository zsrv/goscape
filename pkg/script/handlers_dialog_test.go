package script

import "testing"

func TestPPauseButtonSuspends(t *testing.T) {
	sf := &ScriptFile{
		Name:             "ppb",
		Opcodes:          []Opcode{OpPPauseButton, OpReturn},
		IntOperands:      []int32{0, 0},
		StringOperands:   []string{"", ""},
		InstructionCount: 2,
	}
	mp := &mockPlayer{}
	state := Init(sf, mp, false, nil, nil)
	if err := Execute(state); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if state.Execution != PauseButton {
		t.Errorf("Execution: got %v, want PauseButton", state.Execution)
	}
}

func TestPCountDialogSuspends(t *testing.T) {
	sf := &ScriptFile{
		Name:             "pcd",
		Opcodes:          []Opcode{OpPCountDialog, OpReturn},
		IntOperands:      []int32{0, 0},
		StringOperands:   []string{"", ""},
		InstructionCount: 2,
	}
	mp := &mockPlayer{}
	state := Init(sf, mp, false, nil, nil)
	if err := Execute(state); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if state.Execution != CountDialog {
		t.Errorf("Execution: got %v, want CountDialog", state.Execution)
	}
	if mp.sendCountDialogCalls != 1 {
		t.Errorf("sendCountDialogCalls: got %d, want 1", mp.sendCountDialogCalls)
	}
}

func TestLastCom(t *testing.T) {
	sf := &ScriptFile{
		Name:             "lc",
		Opcodes:          []Opcode{OpLastCom, OpReturn},
		IntOperands:      []int32{0, 0},
		StringOperands:   []string{"", ""},
		InstructionCount: 2,
	}
	mp := &mockPlayer{lastComValue: 42}
	state := Init(sf, mp, false, nil, nil)
	if err := Execute(state); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if got := state.PopInt(); got != 42 {
		t.Errorf("PopInt: got %d, want 42", got)
	}
}

func TestDialogOpsRequireActivePlayer(t *testing.T) {
	for _, op := range []Opcode{OpPPauseButton, OpPCountDialog, OpLastCom} {
		t.Run(op.String(), func(t *testing.T) {
			sf := &ScriptFile{
				Name:             "no_self",
				Opcodes:          []Opcode{op, OpReturn},
				IntOperands:      []int32{0, 0},
				StringOperands:   []string{"", ""},
				InstructionCount: 2,
			}
			state := Init(sf, nil, false, nil, nil)
			if err := Execute(state); err == nil {
				t.Errorf("%v: want error with nil Self", op)
			}
		})
	}
}
