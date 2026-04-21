package script

import "testing"

// runLastOp builds a 1-instruction script that runs op, executes with
// the given mockPlayer, and returns the top of the int stack.
func runLastOp(t *testing.T, op Opcode, mp *mockPlayer) int {
	t.Helper()
	sf := &ScriptFile{
		Name:             "last_" + op.String(),
		Opcodes:          []Opcode{op, OpReturn},
		IntOperands:      []int32{0, 0},
		StringOperands:   []string{"", ""},
		InstructionCount: 2,
	}
	state := Init(sf, mp, false, nil, nil)
	if err := Execute(state); err != nil {
		t.Fatalf("%s: %v", op.String(), err)
	}
	return state.PopInt()
}

func TestLastItem(t *testing.T) {
	mp := &mockPlayer{lastItemValue: 995}
	if got := runLastOp(t, OpLastItem, mp); got != 995 {
		t.Errorf("LAST_ITEM: got %d, want 995", got)
	}
}

func TestLastSlot(t *testing.T) {
	mp := &mockPlayer{lastSlotValue: 3}
	if got := runLastOp(t, OpLastSlot, mp); got != 3 {
		t.Errorf("LAST_SLOT: got %d, want 3", got)
	}
}

func TestLastUseItem(t *testing.T) {
	mp := &mockPlayer{lastUseItemValue: 1042}
	if got := runLastOp(t, OpLastUseItem, mp); got != 1042 {
		t.Errorf("LAST_USEITEM: got %d, want 1042", got)
	}
}

func TestLastUseSlot(t *testing.T) {
	mp := &mockPlayer{lastUseSlotValue: 7}
	if got := runLastOp(t, OpLastUseSlot, mp); got != 7 {
		t.Errorf("LAST_USESLOT: got %d, want 7", got)
	}
}

func TestLastTargetSlot(t *testing.T) {
	mp := &mockPlayer{lastTargetSlotValue: 11}
	if got := runLastOp(t, OpLastTargetSlot, mp); got != 11 {
		t.Errorf("LAST_TARGETSLOT: got %d, want 11", got)
	}
}

func TestLastInt(t *testing.T) {
	sf := &ScriptFile{
		Name:             "last_int",
		Opcodes:          []Opcode{OpLastInt, OpReturn},
		IntOperands:      []int32{0, 0},
		StringOperands:   []string{"", ""},
		InstructionCount: 2,
	}
	state := Init(sf, nil, false, nil, nil)
	state.LastInt = 42
	if err := Execute(state); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if got := state.PopInt(); got != 42 {
		t.Errorf("LAST_INT: got %d, want 42", got)
	}
}

func TestLastInputOpsRequireActivePlayer(t *testing.T) {
	for _, op := range []Opcode{OpLastItem, OpLastSlot, OpLastUseItem, OpLastUseSlot, OpLastTargetSlot} {
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
