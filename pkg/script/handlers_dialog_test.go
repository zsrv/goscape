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
	state := Init(sf, mp, true, nil, nil)
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
	state := Init(sf, mp, true, nil, nil)
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

func TestCamReset(t *testing.T) {
	sf := &ScriptFile{
		Name:             "cam_reset",
		Opcodes:          []Opcode{OpCamReset, OpReturn},
		IntOperands:      []int32{0, 0},
		StringOperands:   []string{"", ""},
		InstructionCount: 2,
	}
	mp := &mockPlayer{}
	state := Init(sf, mp, false, nil, nil)
	if err := Execute(state); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if mp.camResetCalls != 1 {
		t.Errorf("camResetCalls: got %d, want 1", mp.camResetCalls)
	}
}

// TestPPauseButtonUnprotectedRejected verifies that a script started without
// protection gets the "script not protected" error. Closes S6l-D3 for
// P_PAUSEBUTTON (matches TS checkedHandler(ProtectedActivePlayer, ...)).
func TestPPauseButtonUnprotectedRejected(t *testing.T) {
	mp := &mockPlayer{}
	sf := newSingleOp("ppb_unprotected", OpPPauseButton)
	state := Init(sf, mp, false, nil, nil) // protect=false

	err := Execute(state)
	if err == nil || err.Error() != "P_PAUSEBUTTON: script not protected" {
		t.Errorf("expected 'P_PAUSEBUTTON: script not protected', got %v", err)
	}
}

// TestPCountDialogUnprotectedRejected verifies that a script started without
// protection gets the "script not protected" error. Closes S6l-D3 for
// P_COUNTDIALOG (matches TS checkedHandler(ProtectedActivePlayer, ...)).
func TestPCountDialogUnprotectedRejected(t *testing.T) {
	mp := &mockPlayer{}
	sf := newSingleOp("pcd_unprotected", OpPCountDialog)
	state := Init(sf, mp, false, nil, nil) // protect=false

	err := Execute(state)
	if err == nil || err.Error() != "P_COUNTDIALOG: script not protected" {
		t.Errorf("expected 'P_COUNTDIALOG: script not protected', got %v", err)
	}
}

func TestDialogOpsRequireActivePlayer(t *testing.T) {
	for _, op := range []Opcode{OpPPauseButton, OpPCountDialog, OpLastCom, OpCamReset} {
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
