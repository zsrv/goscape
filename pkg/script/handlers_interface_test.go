package script

import "testing"

// Mock-based tests for S5f interface opcodes. Each test assembles a
// minimal script that pushes operands in the order expected by the
// matching TS handler, runs it via Execute, and inspects the
// mockPlayer capture fields. Pop orders are documented in the
// per-handler doc comments in handlers_interface.go.

// -- Modal management ---------------------------------------------------

func TestIfClose(t *testing.T) {
	sf := &ScriptFile{
		Name:             "if_close",
		Opcodes:          []Opcode{OpIfClose, OpReturn},
		IntOperands:      []int32{0, 0},
		StringOperands:   []string{"", ""},
		InstructionCount: 2,
	}
	mp := &mockPlayer{}
	state := Init(sf, mp, false, nil, nil)
	if err := Execute(state); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if mp.lastCloseModalCalls != 1 {
		t.Errorf("CloseModal calls: got %d, want 1", mp.lastCloseModalCalls)
	}
}

func TestIfOpenMain(t *testing.T) {
	sf := &ScriptFile{
		Name:             "if_openmain",
		Opcodes:          []Opcode{OpPushConstantInt, OpIfOpenMain, OpReturn},
		IntOperands:      []int32{1234, 0, 0},
		StringOperands:   []string{"", "", ""},
		InstructionCount: 3,
	}
	mp := &mockPlayer{}
	state := Init(sf, mp, false, nil, nil)
	if err := Execute(state); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if mp.lastOpenMain != 1234 {
		t.Errorf("OpenMain: got %d, want 1234", mp.lastOpenMain)
	}
}

func TestIfOpenChat(t *testing.T) {
	sf := &ScriptFile{
		Name:             "if_openchat",
		Opcodes:          []Opcode{OpPushConstantInt, OpIfOpenChat, OpReturn},
		IntOperands:      []int32{77, 0, 0},
		StringOperands:   []string{"", "", ""},
		InstructionCount: 3,
	}
	mp := &mockPlayer{}
	state := Init(sf, mp, false, nil, nil)
	if err := Execute(state); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if mp.lastOpenChat != 77 {
		t.Errorf("OpenChat: got %d, want 77", mp.lastOpenChat)
	}
}

func TestIfOpenSide(t *testing.T) {
	sf := &ScriptFile{
		Name:             "if_openside",
		Opcodes:          []Opcode{OpPushConstantInt, OpIfOpenSide, OpReturn},
		IntOperands:      []int32{42, 0, 0},
		StringOperands:   []string{"", "", ""},
		InstructionCount: 3,
	}
	mp := &mockPlayer{}
	state := Init(sf, mp, false, nil, nil)
	if err := Execute(state); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if mp.lastOpenSide != 42 {
		t.Errorf("OpenSide: got %d, want 42", mp.lastOpenSide)
	}
}

func TestIfOpenMainSide(t *testing.T) {
	// TS: const [main, side] = state.popInts(2); side is on stack top,
	// so push main first, then side.
	sf := &ScriptFile{
		Name: "if_openmainside",
		Opcodes: []Opcode{
			OpPushConstantInt, // main
			OpPushConstantInt, // side
			OpIfOpenMainSide,
			OpReturn,
		},
		IntOperands:      []int32{100, 200, 0, 0},
		StringOperands:   []string{"", "", "", ""},
		InstructionCount: 4,
	}
	mp := &mockPlayer{}
	state := Init(sf, mp, false, nil, nil)
	if err := Execute(state); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if mp.lastOpenMainSide != (struct{ main, side int }{100, 200}) {
		t.Errorf("OpenMainSide: got %+v, want {100, 200}", mp.lastOpenMainSide)
	}
}

// -- Per-component setters ----------------------------------------------

func TestIfSetText(t *testing.T) {
	// TS: const text = state.popString(); const com = state.popInt();
	// The script pushes the int onto the int stack and the string onto
	// the string stack (separate stacks). Order between them is
	// interleavable; we push in the TS assembly order (int then string).
	sf := &ScriptFile{
		Name: "if_settext",
		Opcodes: []Opcode{
			OpPushConstantInt,    // com
			OpPushConstantString, // text
			OpIfSetText,
			OpReturn,
		},
		IntOperands:      []int32{555, 0, 0, 0},
		StringOperands:   []string{"", "hello world", "", ""},
		InstructionCount: 4,
	}
	mp := &mockPlayer{}
	state := Init(sf, mp, false, nil, nil)
	if err := Execute(state); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	want := struct {
		com  int
		text string
	}{555, "hello world"}
	if mp.lastIfSetText != want {
		t.Errorf("IfSetText: got %+v, want %+v", mp.lastIfSetText, want)
	}
}

func TestIfSetModel(t *testing.T) {
	sf := &ScriptFile{
		Name: "if_setmodel",
		Opcodes: []Opcode{
			OpPushConstantInt, // com
			OpPushConstantInt, // model
			OpIfSetModel,
			OpReturn,
		},
		IntOperands:      []int32{10, 20, 0, 0},
		StringOperands:   []string{"", "", "", ""},
		InstructionCount: 4,
	}
	mp := &mockPlayer{}
	state := Init(sf, mp, false, nil, nil)
	if err := Execute(state); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if mp.lastIfSetModel != (struct{ com, modelID int }{10, 20}) {
		t.Errorf("IfSetModel: got %+v, want {10, 20}", mp.lastIfSetModel)
	}
}

func TestIfSetNpcHead(t *testing.T) {
	sf := &ScriptFile{
		Name: "if_setnpchead",
		Opcodes: []Opcode{
			OpPushConstantInt, // com
			OpPushConstantInt, // npc
			OpIfSetNpcHead,
			OpReturn,
		},
		IntOperands:      []int32{11, 22, 0, 0},
		StringOperands:   []string{"", "", "", ""},
		InstructionCount: 4,
	}
	mp := &mockPlayer{}
	state := Init(sf, mp, false, nil, nil)
	if err := Execute(state); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if mp.lastIfSetNpcHead != (struct{ com, npcID int }{11, 22}) {
		t.Errorf("IfSetNpcHead: got %+v, want {11, 22}", mp.lastIfSetNpcHead)
	}
}

func TestIfSetPlayerHead(t *testing.T) {
	sf := &ScriptFile{
		Name:             "if_setplayerhead",
		Opcodes:          []Opcode{OpPushConstantInt, OpIfSetPlayerHead, OpReturn},
		IntOperands:      []int32{33, 0, 0},
		StringOperands:   []string{"", "", ""},
		InstructionCount: 3,
	}
	mp := &mockPlayer{}
	state := Init(sf, mp, false, nil, nil)
	if err := Execute(state); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if mp.lastIfSetPlayerHead != 33 {
		t.Errorf("IfSetPlayerHead: got %d, want 33", mp.lastIfSetPlayerHead)
	}
}

func TestIfSetAnim(t *testing.T) {
	sf := &ScriptFile{
		Name: "if_setanim",
		Opcodes: []Opcode{
			OpPushConstantInt, // com
			OpPushConstantInt, // seq
			OpIfSetAnim,
			OpReturn,
		},
		IntOperands:      []int32{5, 99, 0, 0},
		StringOperands:   []string{"", "", "", ""},
		InstructionCount: 4,
	}
	mp := &mockPlayer{}
	state := Init(sf, mp, false, nil, nil)
	if err := Execute(state); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if mp.lastIfSetAnim != (struct{ com, seqID int }{5, 99}) {
		t.Errorf("IfSetAnim: got %+v, want {5, 99}", mp.lastIfSetAnim)
	}
}

// TestIfSetAnimSuppressesOnMinusOne verifies the TS guard that skips
// the wire op when seq == -1 (client would crash).
func TestIfSetAnimSuppressesOnMinusOne(t *testing.T) {
	sf := &ScriptFile{
		Name: "if_setanim_skip",
		Opcodes: []Opcode{
			OpPushConstantInt, // com
			OpPushConstantInt, // seq = -1
			OpIfSetAnim,
			OpReturn,
		},
		IntOperands:      []int32{5, -1, 0, 0},
		StringOperands:   []string{"", "", "", ""},
		InstructionCount: 4,
	}
	mp := &mockPlayer{}
	state := Init(sf, mp, false, nil, nil)
	if err := Execute(state); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	// lastIfSetAnim should remain zero-valued because the handler
	// returned before calling IfSetAnim.
	if mp.lastIfSetAnim != (struct{ com, seqID int }{0, 0}) {
		t.Errorf("IfSetAnim seq=-1: expected no call, got %+v", mp.lastIfSetAnim)
	}
}

func TestIfSetHide(t *testing.T) {
	sf := &ScriptFile{
		Name: "if_sethide",
		Opcodes: []Opcode{
			OpPushConstantInt, // com
			OpPushConstantInt, // hide = 1
			OpIfSetHide,
			OpReturn,
		},
		IntOperands:      []int32{8, 1, 0, 0},
		StringOperands:   []string{"", "", "", ""},
		InstructionCount: 4,
	}
	mp := &mockPlayer{}
	state := Init(sf, mp, false, nil, nil)
	if err := Execute(state); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	want := struct {
		com  int
		hide bool
	}{8, true}
	if mp.lastIfSetHide != want {
		t.Errorf("IfSetHide: got %+v, want %+v", mp.lastIfSetHide, want)
	}
}

func TestIfSetHideZeroIsFalse(t *testing.T) {
	sf := &ScriptFile{
		Name: "if_sethide_zero",
		Opcodes: []Opcode{
			OpPushConstantInt, // com
			OpPushConstantInt, // hide = 0
			OpIfSetHide,
			OpReturn,
		},
		IntOperands:      []int32{8, 0, 0, 0},
		StringOperands:   []string{"", "", "", ""},
		InstructionCount: 4,
	}
	mp := &mockPlayer{}
	state := Init(sf, mp, false, nil, nil)
	if err := Execute(state); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	want := struct {
		com  int
		hide bool
	}{8, false}
	if mp.lastIfSetHide != want {
		t.Errorf("IfSetHide: got %+v, want %+v", mp.lastIfSetHide, want)
	}
}

func TestIfSetTab(t *testing.T) {
	sf := &ScriptFile{
		Name: "if_settab",
		Opcodes: []Opcode{
			OpPushConstantInt, // com
			OpPushConstantInt, // tab
			OpIfSetTab,
			OpReturn,
		},
		IntOperands:      []int32{100, 3, 0, 0},
		StringOperands:   []string{"", "", "", ""},
		InstructionCount: 4,
	}
	mp := &mockPlayer{}
	state := Init(sf, mp, false, nil, nil)
	if err := Execute(state); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if mp.lastIfSetTab != (struct{ com, tab int }{100, 3}) {
		t.Errorf("IfSetTab: got %+v, want {100, 3}", mp.lastIfSetTab)
	}
}

func TestIfSetObject(t *testing.T) {
	sf := &ScriptFile{
		Name: "if_setobject",
		Opcodes: []Opcode{
			OpPushConstantInt, // com
			OpPushConstantInt, // obj
			OpPushConstantInt, // scale
			OpIfSetObject,
			OpReturn,
		},
		IntOperands:      []int32{1, 2, 3, 0, 0},
		StringOperands:   []string{"", "", "", "", ""},
		InstructionCount: 5,
	}
	mp := &mockPlayer{}
	state := Init(sf, mp, false, nil, nil)
	if err := Execute(state); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if mp.lastIfSetObject != (struct{ com, objID, scale int }{1, 2, 3}) {
		t.Errorf("IfSetObject: got %+v, want {1, 2, 3}", mp.lastIfSetObject)
	}
}

func TestIfSetColour(t *testing.T) {
	sf := &ScriptFile{
		Name: "if_setcolour",
		Opcodes: []Opcode{
			OpPushConstantInt, // com
			OpPushConstantInt, // colour
			OpIfSetColour,
			OpReturn,
		},
		IntOperands:      []int32{12, 0xff0000, 0, 0},
		StringOperands:   []string{"", "", "", ""},
		InstructionCount: 4,
	}
	mp := &mockPlayer{}
	state := Init(sf, mp, false, nil, nil)
	if err := Execute(state); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if mp.lastIfSetColour != (struct{ com, colour int }{12, 0xff0000}) {
		t.Errorf("IfSetColour: got %+v, want {12, 0xff0000}", mp.lastIfSetColour)
	}
}

func TestIfSetPosition(t *testing.T) {
	sf := &ScriptFile{
		Name: "if_setposition",
		Opcodes: []Opcode{
			OpPushConstantInt, // com
			OpPushConstantInt, // x
			OpPushConstantInt, // y
			OpIfSetPosition,
			OpReturn,
		},
		IntOperands:      []int32{50, 10, 20, 0, 0},
		StringOperands:   []string{"", "", "", "", ""},
		InstructionCount: 5,
	}
	mp := &mockPlayer{}
	state := Init(sf, mp, false, nil, nil)
	if err := Execute(state); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if mp.lastIfSetPosition != (struct{ com, x, y int }{50, 10, 20}) {
		t.Errorf("IfSetPosition: got %+v, want {50, 10, 20}", mp.lastIfSetPosition)
	}
}

func TestIfSetRecol(t *testing.T) {
	sf := &ScriptFile{
		Name: "if_setrecol",
		Opcodes: []Opcode{
			OpPushConstantInt, // com
			OpPushConstantInt, // src
			OpPushConstantInt, // dst
			OpIfSetRecol,
			OpReturn,
		},
		IntOperands:      []int32{7, 0x123, 0x456, 0, 0},
		StringOperands:   []string{"", "", "", "", ""},
		InstructionCount: 5,
	}
	mp := &mockPlayer{}
	state := Init(sf, mp, false, nil, nil)
	if err := Execute(state); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if mp.lastIfSetRecol != (struct{ com, src, dst int }{7, 0x123, 0x456}) {
		t.Errorf("IfSetRecol: got %+v, want {7, 0x123, 0x456}", mp.lastIfSetRecol)
	}
}

// -- Misc ---------------------------------------------------------------

func TestIfSetTabActive(t *testing.T) {
	sf := &ScriptFile{
		Name:             "if_settabactive",
		Opcodes:          []Opcode{OpPushConstantInt, OpIfSetTabActive, OpReturn},
		IntOperands:      []int32{6, 0, 0},
		StringOperands:   []string{"", "", ""},
		InstructionCount: 3,
	}
	mp := &mockPlayer{}
	state := Init(sf, mp, false, nil, nil)
	if err := Execute(state); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if mp.lastIfSetTabActive != 6 {
		t.Errorf("IfSetTabActive: got %d, want 6", mp.lastIfSetTabActive)
	}
}

func TestIfSetResumeButtons(t *testing.T) {
	// TS: const [button1, button2, button3, button4, button5] =
	// state.popInts(5); b5 is on stack top, so push b1..b5 in order.
	sf := &ScriptFile{
		Name: "if_setresumebuttons",
		Opcodes: []Opcode{
			OpPushConstantInt, // b1
			OpPushConstantInt, // b2
			OpPushConstantInt, // b3
			OpPushConstantInt, // b4
			OpPushConstantInt, // b5
			OpIfSetResumeButtons,
			OpReturn,
		},
		IntOperands:      []int32{11, 22, 33, 44, 55, 0, 0},
		StringOperands:   []string{"", "", "", "", "", "", ""},
		InstructionCount: 7,
	}
	mp := &mockPlayer{}
	state := Init(sf, mp, false, nil, nil)
	if err := Execute(state); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if mp.lastSetResumeButtons != [5]int{11, 22, 33, 44, 55} {
		t.Errorf("SetResumeButtons: got %v, want [11 22 33 44 55]", mp.lastSetResumeButtons)
	}
}

// -- Negative tests -----------------------------------------------------

func TestIfCloseNoActivePlayer(t *testing.T) {
	sf := &ScriptFile{
		Name:             "if_close_nap",
		Opcodes:          []Opcode{OpIfClose, OpReturn},
		IntOperands:      []int32{0, 0},
		StringOperands:   []string{"", ""},
		InstructionCount: 2,
	}
	// nil Self → Pointers never get PtrActivePlayer set.
	state := Init(sf, nil, false, nil, nil)
	if err := Execute(state); err == nil {
		t.Fatal("expected error from IF_CLOSE with no active player, got nil")
	}
	if state.Execution != Aborted {
		t.Errorf("Execution: got %v, want Aborted", state.Execution)
	}
}

func TestIfOpenMainNoActivePlayer(t *testing.T) {
	sf := &ScriptFile{
		Name:             "if_openmain_nap",
		Opcodes:          []Opcode{OpPushConstantInt, OpIfOpenMain, OpReturn},
		IntOperands:      []int32{1, 0, 0},
		StringOperands:   []string{"", "", ""},
		InstructionCount: 3,
	}
	state := Init(sf, nil, false, nil, nil)
	if err := Execute(state); err == nil {
		t.Fatal("expected error from IF_OPENMAIN with no active player, got nil")
	}
	if state.Execution != Aborted {
		t.Errorf("Execution: got %v, want Aborted", state.Execution)
	}
}

func TestIfSetResumeButtonsNoActivePlayer(t *testing.T) {
	sf := &ScriptFile{
		Name: "if_setresumebuttons_nap",
		Opcodes: []Opcode{
			OpPushConstantInt, OpPushConstantInt, OpPushConstantInt,
			OpPushConstantInt, OpPushConstantInt,
			OpIfSetResumeButtons, OpReturn,
		},
		IntOperands:      []int32{1, 2, 3, 4, 5, 0, 0},
		StringOperands:   []string{"", "", "", "", "", "", ""},
		InstructionCount: 7,
	}
	state := Init(sf, nil, false, nil, nil)
	if err := Execute(state); err == nil {
		t.Fatal("expected error from IF_SETRESUMEBUTTONS with no active player, got nil")
	}
	if state.Execution != Aborted {
		t.Errorf("Execution: got %v, want Aborted", state.Execution)
	}
}
