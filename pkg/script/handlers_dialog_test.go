package script

import (
	"strings"
	"testing"
)

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

func TestCamShake(t *testing.T) {
	// Script call: cam_shake(axis=4, random=0, amplitude=20, rate=5).
	// engine.rs2 declares cam_shake(int $axis, int $random, int $amplitude, int $rate);
	// args are pushed left-to-right, so on the int stack at OpCamShake (top → bottom):
	//   rate(5), amplitude(20), random(0), axis(4)
	// Wire encoder (TS CamShakeEncoder.ts): p1(axis), p1(random), p1(amplitude), p1(rate).
	sf := &ScriptFile{
		Name: "cam_shake",
		Opcodes: []Opcode{
			OpPushConstantInt, // axis = 4
			OpPushConstantInt, // random = 0
			OpPushConstantInt, // amplitude = 20
			OpPushConstantInt, // rate = 5
			OpCamShake,
			OpReturn,
		},
		IntOperands:      []int32{4, 0, 20, 5, 0, 0},
		StringOperands:   []string{"", "", "", "", "", ""},
		InstructionCount: 6,
	}
	mp := &mockPlayer{}
	state := Init(sf, mp, false, nil, nil)
	if err := Execute(state); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if mp.lastCamShake == nil {
		t.Fatal("CamShake was not called")
	}
	got := *mp.lastCamShake
	want := struct{ axis, random, amplitude, rate int }{axis: 4, random: 0, amplitude: 20, rate: 5}
	if got != want {
		t.Errorf("CamShake args: got %+v, want %+v", got, want)
	}
	if len(mp.cameraPackets) != 0 {
		t.Errorf("cameraPackets must NOT be populated for cam_shake (direct-write); got %d entries", len(mp.cameraPackets))
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
	for _, op := range []Opcode{OpPPauseButton, OpPCountDialog, OpLastCom, OpCamReset, OpCamShake, OpCamMoveTo, OpCamLookAt} {
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

func TestCamMoveTo(t *testing.T) {
	const packedCoord = int32(0x0000_1000)
	level, x, z := unpackCoord(int(packedCoord))

	sf := &ScriptFile{
		Name: "cam_moveto",
		Opcodes: []Opcode{
			OpPushConstantInt, // coord
			OpPushConstantInt, // height
			OpPushConstantInt, // rate
			OpPushConstantInt, // rate2
			OpCamMoveTo,
			OpReturn,
		},
		IntOperands:      []int32{packedCoord, 550, 100, 80, 0, 0},
		StringOperands:   []string{"", "", "", "", "", ""},
		InstructionCount: 6,
	}
	_ = level
	mp := &mockPlayer{}
	state := Init(sf, mp, false, nil, nil)
	if err := Execute(state); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if len(mp.cameraPackets) != 1 {
		t.Fatalf("cameraPackets: got %d entries, want 1", len(mp.cameraPackets))
	}
	got := mp.cameraPackets[0]
	if got.kind != 0 {
		t.Errorf("kind: got %d, want 0 (moveto)", got.kind)
	}
	if got.camX != x || got.camZ != z {
		t.Errorf("(camX, camZ): got (%d, %d), want (%d, %d) from unpackCoord", got.camX, got.camZ, x, z)
	}
	if got.height != 550 || got.rotationSpeed != 100 || got.rotationMultiplier != 80 {
		t.Errorf("scalars: got height=%d rate=%d rate2=%d, want 550 100 80",
			got.height, got.rotationSpeed, got.rotationMultiplier)
	}
}

func TestCamLookAt(t *testing.T) {
	const packedCoord = int32(0x0000_1000)
	_, x, z := unpackCoord(int(packedCoord))

	sf := &ScriptFile{
		Name: "cam_lookat",
		Opcodes: []Opcode{
			OpPushConstantInt, OpPushConstantInt, OpPushConstantInt, OpPushConstantInt,
			OpCamLookAt, OpReturn,
		},
		IntOperands:      []int32{packedCoord, 200, 60, 30, 0, 0},
		StringOperands:   []string{"", "", "", "", "", ""},
		InstructionCount: 6,
	}
	mp := &mockPlayer{}
	state := Init(sf, mp, false, nil, nil)
	if err := Execute(state); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if len(mp.cameraPackets) != 1 {
		t.Fatalf("cameraPackets: got %d entries, want 1", len(mp.cameraPackets))
	}
	got := mp.cameraPackets[0]
	if got.kind != 1 {
		t.Errorf("kind: got %d, want 1 (lookat)", got.kind)
	}
	if got.camX != x || got.camZ != z {
		t.Errorf("(camX, camZ): got (%d, %d), want (%d, %d)", got.camX, got.camZ, x, z)
	}
	if got.height != 200 || got.rotationSpeed != 60 || got.rotationMultiplier != 30 {
		t.Errorf("scalars: got height=%d rate=%d rate2=%d, want 200 60 30",
			got.height, got.rotationSpeed, got.rotationMultiplier)
	}
}

func TestCamMoveToHandler_invalidCoord(t *testing.T) {
	const invalidCoord = int32(-1)
	sf := &ScriptFile{
		Name: "cam_moveto_bad",
		Opcodes: []Opcode{
			OpPushConstantInt, OpPushConstantInt, OpPushConstantInt, OpPushConstantInt,
			OpCamMoveTo, OpReturn,
		},
		IntOperands:      []int32{invalidCoord, 100, 1, 1, 0, 0},
		StringOperands:   []string{"", "", "", "", "", ""},
		InstructionCount: 6,
	}
	mp := &mockPlayer{}
	state := Init(sf, mp, false, nil, nil)
	err := Execute(state)
	if err == nil {
		t.Fatal("expected error from CAM_MOVETO with invalid coord, got nil")
	}
	if !strings.Contains(err.Error(), "CAM_MOVETO") || !strings.Contains(err.Error(), "coord out of range") {
		t.Errorf("error shape: got %q, want substrings 'CAM_MOVETO' and 'coord out of range'", err.Error())
	}
	if len(mp.cameraPackets) != 0 {
		t.Errorf("cameraPackets must remain empty on error; got %d entries", len(mp.cameraPackets))
	}
}

// TestUIDOpcodePushesComposedUID pins NAI-113 cascade closure for the
// UID opcode (handlers_dialog.go:115-121): pushes the active player's
// composed uid to the stack. Pre-fix Player.uid was always -1 because
// Server.addPlayer never composed it; runescript callers branching on
// UID short-circuited dead. Post-NAI-113 production composes uid via
// composeUID(username37, slot); this test pins the handler propagating
// that value to the int stack.
func TestUIDOpcodePushesComposedUID(t *testing.T) {
	self := &mockPlayer{username: "Self", uidValue: 0xABCDEF}

	sf := newSingleOp("uid_push", OpUID)
	state := Init(sf, self, false, nil, nil)

	if err := Execute(state); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if state.ISP != 1 || state.IntStack[0] != 0xABCDEF {
		t.Errorf("stack: got [%v], want [%#x]", state.IntStack[:state.ISP], 0xABCDEF)
	}
}

// TestUIDOpcodeNoActivePlayerErrors pins the negative case: handleUID
// returns an error when Pointers&PtrActivePlayer is unset (no active
// player). This branch is unaffected by NAI-113 but documents the
// guard for completeness.
func TestUIDOpcodeNoActivePlayerErrors(t *testing.T) {
	sf := newSingleOp("uid_no_player", OpUID)
	state := Init(sf, nil, false, nil, nil)

	err := Execute(state)
	if err == nil {
		t.Fatal("expected error on UID with no active player")
	}
}
