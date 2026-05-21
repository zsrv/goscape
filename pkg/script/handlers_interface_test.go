package script

import (
	"strings"
	"testing"

	"github.com/zsrv/goscape/pkg/objtype"
)

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
	state.Configs = &mockConfigs{npcs: map[int]*objtype.NpcType{22: {ConfigType: objtype.ConfigType{ID: 22}}}}
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
	state.Configs = &mockConfigs{objs: map[int]*objtype.ObjType{2: {ConfigType: objtype.ConfigType{ID: 2}}}}
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

// -- NAI-23 Bundle 4c: NumberNotNull null-pin tests ----------------------
//
// Each test below corresponds to a popInt site where the TS counterpart
// (PlayerOps.ts) wraps with check(..., NumberNotNull). A value of -1
// must be rejected before any side-effect occurs.

// TestHandleIfOpenMainNullRejected pins IF_OPENMAIN: TS wraps com with
// NumberNotNull (PlayerOps.ts:720).
func TestHandleIfOpenMainNullRejected(t *testing.T) {
	mp := &mockPlayer{}
	sf := &ScriptFile{
		Name: "if_openmain_null_com",
		Opcodes: []Opcode{
			OpPushConstantInt, // com = -1
			OpIfOpenMain,
			OpReturn,
		},
		IntOperands: []int32{-1, 0, 0},
	}
	state := Init(sf, mp, false, nil, nil)

	err := Execute(state)
	if err == nil {
		t.Fatalf("Execute: want error for com=-1, got nil")
	}
	want := "IF_OPENMAIN: input number was null(-1)"
	if !strings.Contains(err.Error(), want) {
		t.Errorf("error: got %q, want substring %q", err.Error(), want)
	}
	if mp.lastOpenMain != 0 {
		t.Errorf("OpenMain: should not have been called, got %d", mp.lastOpenMain)
	}
}

// TestHandleIfOpenChatNullRejected pins IF_OPENCHAT: TS wraps com with
// NumberNotNull (PlayerOps.ts:642).
func TestHandleIfOpenChatNullRejected(t *testing.T) {
	mp := &mockPlayer{}
	sf := &ScriptFile{
		Name: "if_openchat_null_com",
		Opcodes: []Opcode{
			OpPushConstantInt, // com = -1
			OpIfOpenChat,
			OpReturn,
		},
		IntOperands: []int32{-1, 0, 0},
	}
	state := Init(sf, mp, false, nil, nil)

	err := Execute(state)
	if err == nil {
		t.Fatalf("Execute: want error for com=-1, got nil")
	}
	want := "IF_OPENCHAT: input number was null(-1)"
	if !strings.Contains(err.Error(), want) {
		t.Errorf("error: got %q, want substring %q", err.Error(), want)
	}
	if mp.lastOpenChat != 0 {
		t.Errorf("OpenChat: should not have been called, got %d", mp.lastOpenChat)
	}
}

// TestHandleIfOpenSideNullRejected pins IF_OPENSIDE: TS wraps com with
// NumberNotNull (PlayerOps.ts:728).
func TestHandleIfOpenSideNullRejected(t *testing.T) {
	mp := &mockPlayer{}
	sf := &ScriptFile{
		Name: "if_openside_null_com",
		Opcodes: []Opcode{
			OpPushConstantInt, // com = -1
			OpIfOpenSide,
			OpReturn,
		},
		IntOperands: []int32{-1, 0, 0},
	}
	state := Init(sf, mp, false, nil, nil)

	err := Execute(state)
	if err == nil {
		t.Fatalf("Execute: want error for com=-1, got nil")
	}
	want := "IF_OPENSIDE: input number was null(-1)"
	if !strings.Contains(err.Error(), want) {
		t.Errorf("error: got %q, want substring %q", err.Error(), want)
	}
	if mp.lastOpenSide != 0 {
		t.Errorf("OpenSide: should not have been called, got %d", mp.lastOpenSide)
	}
}

// TestHandleIfOpenMainSideNullRejected pins IF_OPENMAIN_SIDE: TS wraps both
// main and side with NumberNotNull (PlayerOps.ts:648-649). Table-driven:
// one sub-test per null slot.
func TestHandleIfOpenMainSideNullRejected(t *testing.T) {
	tests := []struct {
		name       string
		main, side int
		wantSubstr string
	}{
		{
			name:       "null_main",
			main:       -1,
			side:       200,
			wantSubstr: "IF_OPENMAIN_SIDE: input number was null(-1)",
		},
		{
			name:       "null_side",
			main:       100,
			side:       -1,
			wantSubstr: "IF_OPENMAIN_SIDE: input number was null(-1)",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			mp := &mockPlayer{}
			sf := &ScriptFile{
				Name: "if_openmainside_" + tc.name,
				Opcodes: []Opcode{
					OpPushConstantInt, // push main (bottom)
					OpPushConstantInt, // push side (top)
					OpIfOpenMainSide,
					OpReturn,
				},
				IntOperands: []int32{int32(tc.main), int32(tc.side), 0, 0},
			}
			state := Init(sf, mp, false, nil, nil)

			err := Execute(state)
			if err == nil {
				t.Fatalf("Execute: want error for %s, got nil", tc.name)
			}
			if !strings.Contains(err.Error(), tc.wantSubstr) {
				t.Errorf("error: got %q, want substring %q", err.Error(), tc.wantSubstr)
			}
			if mp.lastOpenMainSide != (struct{ main, side int }{}) {
				t.Errorf("lastOpenMainSide: got %+v, want zero (mock should not have been called on null-input rejection)", mp.lastOpenMainSide)
			}
		})
	}
}

// TestHandleIfSetTextNullComRejected pins IF_SETTEXT: TS wraps com with
// NumberNotNull (PlayerOps.ts:737).
func TestHandleIfSetTextNullComRejected(t *testing.T) {
	mp := &mockPlayer{}
	sf := &ScriptFile{
		Name: "if_settext_null_com",
		Opcodes: []Opcode{
			OpPushConstantInt,    // com = -1
			OpPushConstantString, // text (valid)
			OpIfSetText,
			OpReturn,
		},
		IntOperands:    []int32{-1, 0, 0, 0},
		StringOperands: []string{"", "hello", "", ""},
	}
	state := Init(sf, mp, false, nil, nil)

	err := Execute(state)
	if err == nil {
		t.Fatalf("Execute: want error for com=-1, got nil")
	}
	want := "IF_SETTEXT: input number was null(-1)"
	if !strings.Contains(err.Error(), want) {
		t.Errorf("error: got %q, want substring %q", err.Error(), want)
	}
	if mp.lastIfSetText != (struct {
		com  int
		text string
	}{}) {
		t.Errorf("lastIfSetText: got %+v, want zero (mock should not have been called on null-input rejection)", mp.lastIfSetText)
	}
}

// TestHandleIfSetModelNullRejected pins IF_SETMODEL: TS wraps both com and
// model with NumberNotNull (PlayerOps.ts:680-681). Table-driven.
func TestHandleIfSetModelNullRejected(t *testing.T) {
	tests := []struct {
		name       string
		com, model int
		wantSubstr string
	}{
		{
			name:       "null_com",
			com:        -1,
			model:      20,
			wantSubstr: "IF_SETMODEL: input number was null(-1)",
		},
		{
			name:       "null_model",
			com:        10,
			model:      -1,
			wantSubstr: "IF_SETMODEL: input number was null(-1)",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			mp := &mockPlayer{}
			sf := &ScriptFile{
				Name: "if_setmodel_" + tc.name,
				Opcodes: []Opcode{
					OpPushConstantInt, // push com (bottom)
					OpPushConstantInt, // push model (top)
					OpIfSetModel,
					OpReturn,
				},
				IntOperands: []int32{int32(tc.com), int32(tc.model), 0, 0},
			}
			state := Init(sf, mp, false, nil, nil)

			err := Execute(state)
			if err == nil {
				t.Fatalf("Execute: want error for %s, got nil", tc.name)
			}
			if !strings.Contains(err.Error(), tc.wantSubstr) {
				t.Errorf("error: got %q, want substring %q", err.Error(), tc.wantSubstr)
			}
			if mp.lastIfSetModel != (struct{ com, modelID int }{}) {
				t.Errorf("lastIfSetModel: got %+v, want zero (mock should not have been called on null-input rejection)", mp.lastIfSetModel)
			}
		})
	}
}

// TestHandleIfSetNpcHeadNullComRejected pins IF_SETNPCHEAD: TS wraps com with
// NumberNotNull (PlayerOps.ts:745). npc uses NpcTypeValid (not NumberNotNull)
// so only com is covered here.
func TestHandleIfSetNpcHeadNullComRejected(t *testing.T) {
	mp := &mockPlayer{}
	sf := &ScriptFile{
		Name: "if_setnpchead_null_com",
		Opcodes: []Opcode{
			OpPushConstantInt, // push com = -1 (bottom)
			OpPushConstantInt, // push npc (top)
			OpIfSetNpcHead,
			OpReturn,
		},
		IntOperands: []int32{-1, 22, 0, 0},
	}
	state := Init(sf, mp, false, nil, nil)

	err := Execute(state)
	if err == nil {
		t.Fatalf("Execute: want error for com=-1, got nil")
	}
	want := "IF_SETNPCHEAD: input number was null(-1)"
	if !strings.Contains(err.Error(), want) {
		t.Errorf("error: got %q, want substring %q", err.Error(), want)
	}
	if mp.lastIfSetNpcHead != (struct{ com, npcID int }{0, 0}) {
		t.Errorf("IfSetNpcHead: should not have been called, got %+v", mp.lastIfSetNpcHead)
	}
}

// TestHandleIfSetPlayerHeadNullRejected pins IF_SETPLAYERHEAD: TS wraps com
// with NumberNotNull (PlayerOps.ts:732).
func TestHandleIfSetPlayerHeadNullRejected(t *testing.T) {
	mp := &mockPlayer{}
	sf := &ScriptFile{
		Name: "if_setplayerhead_null_com",
		Opcodes: []Opcode{
			OpPushConstantInt, // com = -1
			OpIfSetPlayerHead,
			OpReturn,
		},
		IntOperands: []int32{-1, 0, 0},
	}
	state := Init(sf, mp, false, nil, nil)

	err := Execute(state)
	if err == nil {
		t.Fatalf("Execute: want error for com=-1, got nil")
	}
	want := "IF_SETPLAYERHEAD: input number was null(-1)"
	if !strings.Contains(err.Error(), want) {
		t.Errorf("error: got %q, want substring %q", err.Error(), want)
	}
	if mp.lastIfSetPlayerHead != 0 {
		t.Errorf("IfSetPlayerHead: should not have been called, got %d", mp.lastIfSetPlayerHead)
	}
}

// TestHandleIfSetAnimNullComRejected pins IF_SETANIM: TS wraps com with
// NumberNotNull (PlayerOps.ts:701). seq=-1 is a valid sentinel (client-crash
// guard) and is NOT wrapped, so only com is covered here.
func TestHandleIfSetAnimNullComRejected(t *testing.T) {
	mp := &mockPlayer{}
	sf := &ScriptFile{
		Name: "if_setanim_null_com",
		Opcodes: []Opcode{
			OpPushConstantInt, // push com = -1 (bottom)
			OpPushConstantInt, // push seq (top)
			OpIfSetAnim,
			OpReturn,
		},
		IntOperands: []int32{-1, 5, 0, 0},
	}
	state := Init(sf, mp, false, nil, nil)

	err := Execute(state)
	if err == nil {
		t.Fatalf("Execute: want error for com=-1, got nil")
	}
	want := "IF_SETANIM: input number was null(-1)"
	if !strings.Contains(err.Error(), want) {
		t.Errorf("error: got %q, want substring %q", err.Error(), want)
	}
	if mp.lastIfSetAnim != (struct{ com, seqID int }{0, 0}) {
		t.Errorf("IfSetAnim: should not have been called, got %+v", mp.lastIfSetAnim)
	}
}

// TestHandleIfSetHideNullRejected pins IF_SETHIDE: TS wraps both com and hide
// with NumberNotNull (PlayerOps.ts:657-658). Table-driven.
func TestHandleIfSetHideNullRejected(t *testing.T) {
	tests := []struct {
		name       string
		com, hide  int
		wantSubstr string
	}{
		{
			name:       "null_com",
			com:        -1,
			hide:       1,
			wantSubstr: "IF_SETHIDE: input number was null(-1)",
		},
		{
			name:       "null_hide",
			com:        8,
			hide:       -1,
			wantSubstr: "IF_SETHIDE: input number was null(-1)",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			mp := &mockPlayer{}
			sf := &ScriptFile{
				Name: "if_sethide_" + tc.name,
				Opcodes: []Opcode{
					OpPushConstantInt, // push com (bottom)
					OpPushConstantInt, // push hide (top)
					OpIfSetHide,
					OpReturn,
				},
				IntOperands: []int32{int32(tc.com), int32(tc.hide), 0, 0},
			}
			state := Init(sf, mp, false, nil, nil)

			err := Execute(state)
			if err == nil {
				t.Fatalf("Execute: want error for %s, got nil", tc.name)
			}
			if !strings.Contains(err.Error(), tc.wantSubstr) {
				t.Errorf("error: got %q, want substring %q", err.Error(), tc.wantSubstr)
			}
			if mp.lastIfSetHide != (struct {
				com  int
				hide bool
			}{}) {
				t.Errorf("lastIfSetHide: got %+v, want zero (mock should not have been called on null-input rejection)", mp.lastIfSetHide)
			}
		})
	}
}

// TestHandleIfSetTabNullTabRejected pins IF_SETTAB: TS wraps tab with
// NumberNotNull (PlayerOps.ts:714). com is NOT wrapped in TS so only tab
// is covered here.
func TestHandleIfSetTabNullTabRejected(t *testing.T) {
	mp := &mockPlayer{}
	sf := &ScriptFile{
		Name: "if_settab_null_tab",
		Opcodes: []Opcode{
			OpPushConstantInt, // push com (bottom)
			OpPushConstantInt, // push tab = -1 (top)
			OpIfSetTab,
			OpReturn,
		},
		IntOperands: []int32{100, -1, 0, 0},
	}
	state := Init(sf, mp, false, nil, nil)

	err := Execute(state)
	if err == nil {
		t.Fatalf("Execute: want error for tab=-1, got nil")
	}
	want := "IF_SETTAB: input number was null(-1)"
	if !strings.Contains(err.Error(), want) {
		t.Errorf("error: got %q, want substring %q", err.Error(), want)
	}
	if mp.lastIfSetTab != (struct{ com, tab int }{0, 0}) {
		t.Errorf("IfSetTab: should not have been called, got %+v", mp.lastIfSetTab)
	}
}

// TestHandleIfSetObjectNullRejected pins IF_SETOBJECT: TS wraps com and scale
// with NumberNotNull (PlayerOps.ts:666, 668). obj uses ObjTypeValid (not
// NumberNotNull) and is not covered here. Table-driven: one sub-test per null slot.
func TestHandleIfSetObjectNullRejected(t *testing.T) {
	tests := []struct {
		name            string
		com, obj, scale int
		wantSubstr      string
	}{
		{
			name:       "null_com",
			com:        -1,
			obj:        2,
			scale:      3,
			wantSubstr: "IF_SETOBJECT: input number was null(-1)",
		},
		{
			name:       "null_scale",
			com:        1,
			obj:        2,
			scale:      -1,
			wantSubstr: "IF_SETOBJECT: input number was null(-1)",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			mp := &mockPlayer{}
			sf := &ScriptFile{
				Name: "if_setobject_" + tc.name,
				Opcodes: []Opcode{
					OpPushConstantInt, // push com (bottom)
					OpPushConstantInt, // push obj
					OpPushConstantInt, // push scale (top)
					OpIfSetObject,
					OpReturn,
				},
				IntOperands: []int32{int32(tc.com), int32(tc.obj), int32(tc.scale), 0, 0},
			}
			state := Init(sf, mp, false, nil, nil)
			state.Configs = &mockConfigs{objs: map[int]*objtype.ObjType{2: {ConfigType: objtype.ConfigType{ID: 2}}}}

			err := Execute(state)
			if err == nil {
				t.Fatalf("Execute: want error for %s, got nil", tc.name)
			}
			if !strings.Contains(err.Error(), tc.wantSubstr) {
				t.Errorf("error: got %q, want substring %q", err.Error(), tc.wantSubstr)
			}
			if mp.lastIfSetObject != (struct{ com, objID, scale int }{}) {
				t.Errorf("lastIfSetObject: got %+v, want zero (mock should not have been called on null-input rejection)", mp.lastIfSetObject)
			}
		})
	}
}

// TestHandleIfSetColourNullRejected pins IF_SETCOLOUR: TS wraps both com and
// colour with NumberNotNull (PlayerOps.ts:635-636). Table-driven.
func TestHandleIfSetColourNullRejected(t *testing.T) {
	tests := []struct {
		name        string
		com, colour int
		wantSubstr  string
	}{
		{
			name:       "null_com",
			com:        -1,
			colour:     0xff0000,
			wantSubstr: "IF_SETCOLOUR: input number was null(-1)",
		},
		{
			name:       "null_colour",
			com:        12,
			colour:     -1,
			wantSubstr: "IF_SETCOLOUR: input number was null(-1)",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			mp := &mockPlayer{}
			sf := &ScriptFile{
				Name: "if_setcolour_" + tc.name,
				Opcodes: []Opcode{
					OpPushConstantInt, // push com (bottom)
					OpPushConstantInt, // push colour (top)
					OpIfSetColour,
					OpReturn,
				},
				IntOperands: []int32{int32(tc.com), int32(tc.colour), 0, 0},
			}
			state := Init(sf, mp, false, nil, nil)

			err := Execute(state)
			if err == nil {
				t.Fatalf("Execute: want error for %s, got nil", tc.name)
			}
			if !strings.Contains(err.Error(), tc.wantSubstr) {
				t.Errorf("error: got %q, want substring %q", err.Error(), tc.wantSubstr)
			}
			if mp.lastIfSetColour != (struct{ com, colour int }{}) {
				t.Errorf("lastIfSetColour: got %+v, want zero (mock should not have been called on null-input rejection)", mp.lastIfSetColour)
			}
		})
	}
}

// TestHandleIfSetPositionNullComRejected pins IF_SETPOSITION: TS wraps com
// with NumberNotNull (PlayerOps.ts:754). x and y are NOT wrapped in TS so
// only com is covered here.
func TestHandleIfSetPositionNullComRejected(t *testing.T) {
	mp := &mockPlayer{}
	sf := &ScriptFile{
		Name: "if_setposition_null_com",
		Opcodes: []Opcode{
			OpPushConstantInt, // push com = -1 (bottom)
			OpPushConstantInt, // push x
			OpPushConstantInt, // push y (top)
			OpIfSetPosition,
			OpReturn,
		},
		IntOperands: []int32{-1, 10, 20, 0, 0},
	}
	state := Init(sf, mp, false, nil, nil)

	err := Execute(state)
	if err == nil {
		t.Fatalf("Execute: want error for com=-1, got nil")
	}
	want := "IF_SETPOSITION: input number was null(-1)"
	if !strings.Contains(err.Error(), want) {
		t.Errorf("error: got %q, want substring %q", err.Error(), want)
	}
	if mp.lastIfSetPosition != (struct{ com, x, y int }{0, 0, 0}) {
		t.Errorf("IfSetPosition: should not have been called, got %+v", mp.lastIfSetPosition)
	}
}

// TestHandleIfSetRecolNullComRejected pins IF_SETRECOL: TS wraps com with
// NumberNotNull (PlayerOps.ts:689). src and dest are NOT wrapped in TS so
// only com is covered here.
func TestHandleIfSetRecolNullComRejected(t *testing.T) {
	mp := &mockPlayer{}
	sf := &ScriptFile{
		Name: "if_setrecol_null_com",
		Opcodes: []Opcode{
			OpPushConstantInt, // push com = -1 (bottom)
			OpPushConstantInt, // push src
			OpPushConstantInt, // push dest (top)
			OpIfSetRecol,
			OpReturn,
		},
		IntOperands: []int32{-1, 0x123, 0x456, 0, 0},
	}
	state := Init(sf, mp, false, nil, nil)

	err := Execute(state)
	if err == nil {
		t.Fatalf("Execute: want error for com=-1, got nil")
	}
	want := "IF_SETRECOL: input number was null(-1)"
	if !strings.Contains(err.Error(), want) {
		t.Errorf("error: got %q, want substring %q", err.Error(), want)
	}
	if mp.lastIfSetRecol != (struct{ com, src, dst int }{0, 0, 0}) {
		t.Errorf("IfSetRecol: should not have been called, got %+v", mp.lastIfSetRecol)
	}
}

// TestHandleIfSetTabActiveNullRejected pins IF_SETTABACTIVE: TS wraps tab with
// NumberNotNull (PlayerOps.ts:674).
func TestHandleIfSetTabActiveNullRejected(t *testing.T) {
	mp := &mockPlayer{}
	sf := &ScriptFile{
		Name: "if_settabactive_null_tab",
		Opcodes: []Opcode{
			OpPushConstantInt, // tab = -1
			OpIfSetTabActive,
			OpReturn,
		},
		IntOperands: []int32{-1, 0, 0},
	}
	state := Init(sf, mp, false, nil, nil)

	err := Execute(state)
	if err == nil {
		t.Fatalf("Execute: want error for tab=-1, got nil")
	}
	want := "IF_SETTABACTIVE: input number was null(-1)"
	if !strings.Contains(err.Error(), want) {
		t.Errorf("error: got %q, want substring %q", err.Error(), want)
	}
	if mp.lastIfSetTabActive != 0 {
		t.Errorf("IfSetTabActive: should not have been called, got %d", mp.lastIfSetTabActive)
	}
}

// -- NAI-76: TUT_OPEN tests ------------------------------------------------

// TestTutOpen pins TUT_OPEN script-opcode dispatch:
// state.popInt() → ActivePlayer.OpenTutorial(com).
// Mirrors TS PlayerOps.ts:723-725.
func TestTutOpen(t *testing.T) {
	sf := &ScriptFile{
		Name:             "tut_open",
		Opcodes:          []Opcode{OpPushConstantInt, OpTutOpen, OpReturn},
		IntOperands:      []int32{42, 0, 0},
		StringOperands:   []string{"", "", ""},
		InstructionCount: 3,
	}
	mp := &mockPlayer{}
	state := Init(sf, mp, false, nil, nil)
	if err := Execute(state); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if mp.lastOpenTutorial != 42 {
		t.Errorf("OpenTutorial: got %d, want 42", mp.lastOpenTutorial)
	}
}

// TestHandleTutOpenNullRejected pins TUT_OPEN: TS wraps com with
// NumberNotNull (PlayerOps.ts:723-724). A com value of -1 must be
// rejected before any side-effect occurs.
func TestHandleTutOpenNullRejected(t *testing.T) {
	mp := &mockPlayer{}
	sf := &ScriptFile{
		Name: "tut_open_null_com",
		Opcodes: []Opcode{
			OpPushConstantInt, // com = -1
			OpTutOpen,
			OpReturn,
		},
		IntOperands: []int32{-1, 0, 0},
	}
	state := Init(sf, mp, false, nil, nil)

	err := Execute(state)
	if err == nil {
		t.Fatalf("Execute: want error for com=-1, got nil")
	}
	want := "TUT_OPEN: input number was null(-1)"
	if !strings.Contains(err.Error(), want) {
		t.Errorf("error: got %q, want substring %q", err.Error(), want)
	}
	if mp.lastOpenTutorial != 0 {
		t.Errorf("OpenTutorial: should not have been called, got %d", mp.lastOpenTutorial)
	}
}

func TestTutOpenNoActivePlayer(t *testing.T) {
	sf := &ScriptFile{
		Name:             "tut_open_nap",
		Opcodes:          []Opcode{OpPushConstantInt, OpTutOpen, OpReturn},
		IntOperands:      []int32{1, 0, 0},
		StringOperands:   []string{"", "", ""},
		InstructionCount: 3,
	}
	state := Init(sf, nil, false, nil, nil)
	if err := Execute(state); err == nil {
		t.Fatal("expected error from TUT_OPEN with no active player, got nil")
	}
	if state.Execution != Aborted {
		t.Errorf("Execution: got %v, want Aborted", state.Execution)
	}
}

// -- NAI-102: TUT_CLOSE tests ----------------------------------------------

// TestTutClose pins TUT_CLOSE script-opcode dispatch:
// no pops; just delegates to ActivePlayer.CloseTutorial().
// Mirrors TS PlayerOps.ts:877-879.
func TestTutClose(t *testing.T) {
	sf := &ScriptFile{
		Name:             "tut_close",
		Opcodes:          []Opcode{OpTutClose, OpReturn},
		IntOperands:      []int32{0, 0},
		StringOperands:   []string{"", ""},
		InstructionCount: 2,
	}
	mp := &mockPlayer{}
	state := Init(sf, mp, false, nil, nil)
	if err := Execute(state); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if mp.lastCloseTutorialCalls != 1 {
		t.Errorf("CloseTutorial calls: got %d, want 1", mp.lastCloseTutorialCalls)
	}
}

// TestTutCloseNoActivePlayer pins the no-active-player guard on TUT_CLOSE.
func TestTutCloseNoActivePlayer(t *testing.T) {
	sf := &ScriptFile{
		Name:             "tut_close_nap",
		Opcodes:          []Opcode{OpTutClose, OpReturn},
		IntOperands:      []int32{0, 0},
		StringOperands:   []string{"", ""},
		InstructionCount: 2,
	}
	mp := &mockPlayer{}
	state := Init(sf, nil, false, nil, nil)
	err := Execute(state)
	if err == nil {
		t.Fatal("expected error from TUT_CLOSE with no active player, got nil")
	}
	want := "TUT_CLOSE: no active player"
	if !strings.Contains(err.Error(), want) {
		t.Errorf("error: got %q, want substring %q", err.Error(), want)
	}
	if mp.lastCloseTutorialCalls != 0 {
		t.Errorf("CloseTutorial calls: got %d, want 0", mp.lastCloseTutorialCalls)
	}
	if state.Execution != Aborted {
		t.Errorf("Execution: got %v, want Aborted", state.Execution)
	}
}

// -- NAI-109: TUT_FLASH tests ----------------------------------------------

// TestTutFlash pins TUT_FLASH script-opcode dispatch:
// state.popInt() → ActivePlayer.FlashTutorial(tab).
// Mirrors TS PlayerOps.ts:694-696.
func TestTutFlash(t *testing.T) {
	sf := &ScriptFile{
		Name:             "tut_flash",
		Opcodes:          []Opcode{OpPushConstantInt, OpTutFlash, OpReturn},
		IntOperands:      []int32{42, 0, 0},
		StringOperands:   []string{"", "", ""},
		InstructionCount: 3,
	}
	mp := &mockPlayer{}
	state := Init(sf, mp, false, nil, nil)
	if err := Execute(state); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if mp.lastFlashTutorial != 42 {
		t.Errorf("FlashTutorial: got %d, want 42", mp.lastFlashTutorial)
	}
	if mp.lastFlashTutorialCalls != 1 {
		t.Errorf("FlashTutorial calls: got %d, want 1", mp.lastFlashTutorialCalls)
	}
}

// TestHandleTutFlashNullRejected pins TUT_FLASH: TS wraps tab with
// NumberNotNull (PlayerOps.ts:694-695). A tab value of -1 must be
// rejected before any side-effect occurs.
func TestHandleTutFlashNullRejected(t *testing.T) {
	mp := &mockPlayer{}
	sf := &ScriptFile{
		Name: "tut_flash_null_tab",
		Opcodes: []Opcode{
			OpPushConstantInt, // tab = -1
			OpTutFlash,
			OpReturn,
		},
		IntOperands: []int32{-1, 0, 0},
	}
	state := Init(sf, mp, false, nil, nil)

	err := Execute(state)
	if err == nil {
		t.Fatalf("Execute: want error for tab=-1, got nil")
	}
	want := "TUT_FLASH: input number was null(-1)"
	if !strings.Contains(err.Error(), want) {
		t.Errorf("error: got %q, want substring %q", err.Error(), want)
	}
	if mp.lastFlashTutorialCalls != 0 {
		t.Errorf("FlashTutorial: should not have been called, got %d calls", mp.lastFlashTutorialCalls)
	}
}

// TestTutFlashNoActivePlayer pins the no-active-player guard on TUT_FLASH.
func TestTutFlashNoActivePlayer(t *testing.T) {
	sf := &ScriptFile{
		Name:             "tut_flash_nap",
		Opcodes:          []Opcode{OpPushConstantInt, OpTutFlash, OpReturn},
		IntOperands:      []int32{1, 0, 0},
		StringOperands:   []string{"", "", ""},
		InstructionCount: 3,
	}
	state := Init(sf, nil, false, nil, nil)
	if err := Execute(state); err == nil {
		t.Fatal("expected error from TUT_FLASH with no active player, got nil")
	}
	if state.Execution != Aborted {
		t.Errorf("Execution: got %v, want Aborted", state.Execution)
	}
}
