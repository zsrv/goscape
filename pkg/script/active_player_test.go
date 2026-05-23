package script

import "testing"

// newOperandState builds a ScriptState for a single instruction `op` carrying
// the given int operand, with Self bound (+PtrActivePlayer via Init) and, when
// other != nil, Self2 bound (+PtrActivePlayer2). Ready to invoke a handler
// directly with PC=0.
func newOperandState(op Opcode, operand int32, self, other ActivePlayer) *ScriptState {
	f := &ScriptFile{
		Name:             "test",
		Opcodes:          []Opcode{op, OpReturn},
		IntOperands:      []int32{operand, 0},
		StringOperands:   []string{"", ""},
		InstructionCount: 2,
		IntLocalCount:    8,
		StringLocalCount: 8,
	}
	s := Init(f, self, true, nil, nil)
	if other != nil {
		s.Self2 = other
		s.Pointers |= PtrActivePlayer2
	}
	return s
}

// TestMes_SecondaryOperandTargetsSelf2 pins the trade-request fix: `.mes`
// (operand 1) sends to the SECONDARY active player. trade.rs2 uses
// `.mes("<displayname>:tradereq:")` to notify the target; pre-fix it went back
// to the sender ("User1 wishes to trade with you." received by User1).
func TestMes_SecondaryOperandTargetsSelf2(t *testing.T) {
	self := &mockPlayer{}
	other := &mockPlayer{}
	s := newOperandState(OpMes, 1, self, other)
	s.PushString("trade request")
	if err := handleMes(s); err != nil {
		t.Fatalf("handleMes: %v", err)
	}
	if len(self.messages) != 0 {
		t.Errorf("self must NOT receive .mes (operand 1), got %v", self.messages)
	}
	if len(other.messages) != 1 || other.messages[0] != "trade request" {
		t.Errorf("Self2 must receive .mes (operand 1), got %v", other.messages)
	}
}

// TestMes_PrimaryOperandTargetsSelf is the operand-0 regression guard.
func TestMes_PrimaryOperandTargetsSelf(t *testing.T) {
	self := &mockPlayer{}
	other := &mockPlayer{}
	s := newOperandState(OpMes, 0, self, other)
	s.PushString("hello")
	if err := handleMes(s); err != nil {
		t.Fatalf("handleMes: %v", err)
	}
	if len(self.messages) != 1 || self.messages[0] != "hello" {
		t.Errorf("Self must receive bare mes (operand 0), got %v", self.messages)
	}
	if len(other.messages) != 0 {
		t.Errorf("Self2 must NOT receive bare mes, got %v", other.messages)
	}
}

// TestUid_SecondaryOperandReturnsSelf2 pins the combat self-damage fix: `.uid`
// (operand 1) pushes the SECONDARY player's uid. PvP scripts do
// damage(.uid, ...); pre-fix .uid returned the attacker's own uid so damage
// landed on self.
func TestUid_SecondaryOperandReturnsSelf2(t *testing.T) {
	self := &mockPlayer{uidValue: 111}
	other := &mockPlayer{uidValue: 222}
	s := newOperandState(OpUID, 1, self, other)
	if err := handleUID(s); err != nil {
		t.Fatalf("handleUID: %v", err)
	}
	if got := s.PopInt(); got != 222 {
		t.Errorf(".uid (operand 1) must push Self2's uid 222, got %d", got)
	}
}

// TestUid_PrimaryOperandReturnsSelf is the operand-0 regression guard.
func TestUid_PrimaryOperandReturnsSelf(t *testing.T) {
	self := &mockPlayer{uidValue: 111}
	other := &mockPlayer{uidValue: 222}
	s := newOperandState(OpUID, 0, self, other)
	if err := handleUID(s); err != nil {
		t.Fatalf("handleUID: %v", err)
	}
	if got := s.PopInt(); got != 111 {
		t.Errorf("uid (operand 0) must push Self's uid 111, got %d", got)
	}
}
