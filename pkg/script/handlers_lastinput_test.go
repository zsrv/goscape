package script

import (
	"strings"
	"testing"
)

// runLastOp builds a 1-instruction script that runs op, executes with
// the given mockPlayer and trigger, and returns the top of the int stack.
// trigger must be in the opcode's allowlist (see handlers_dialog.go) for
// the op to succeed. Pass TriggerProc (zero) to exercise the rejection
// path.
func runLastOp(t *testing.T, op Opcode, mp *mockPlayer, trigger ServerTriggerType) int {
	t.Helper()
	sf := &ScriptFile{
		Name:             "last_" + op.String(),
		Opcodes:          []Opcode{op, OpReturn},
		IntOperands:      []int32{0, 0},
		StringOperands:   []string{"", ""},
		InstructionCount: 2,
	}
	state := Init(sf, mp, false, nil, nil)
	state.Trigger = trigger
	if err := Execute(state); err != nil {
		t.Fatalf("%s: %v", op.String(), err)
	}
	return state.PopInt()
}

// runLastOpErr is the rejection-path counterpart to runLastOp: returns
// the error rather than fatal-ing on it. Used by the per-opcode "wrong
// trigger" tests.
func runLastOpErr(t *testing.T, op Opcode, mp *mockPlayer, trigger ServerTriggerType) error {
	t.Helper()
	sf := &ScriptFile{
		Name:             "last_err_" + op.String(),
		Opcodes:          []Opcode{op, OpReturn},
		IntOperands:      []int32{0, 0},
		StringOperands:   []string{"", ""},
		InstructionCount: 2,
	}
	state := Init(sf, mp, false, nil, nil)
	state.Trigger = trigger
	return Execute(state)
}

func TestLastItem(t *testing.T) {
	mp := &mockPlayer{lastItemValue: 995}
	if got := runLastOp(t, OpLastItem, mp, TriggerOpHeld1); got != 995 {
		t.Errorf("LAST_ITEM: got %d, want 995", got)
	}
}

func TestLastSlot(t *testing.T) {
	mp := &mockPlayer{lastSlotValue: 3}
	if got := runLastOp(t, OpLastSlot, mp, TriggerOpHeld1); got != 3 {
		t.Errorf("LAST_SLOT: got %d, want 3", got)
	}
}

func TestLastUseItem(t *testing.T) {
	mp := &mockPlayer{lastUseItemValue: 1042}
	if got := runLastOp(t, OpLastUseItem, mp, TriggerOpHeldU); got != 1042 {
		t.Errorf("LAST_USEITEM: got %d, want 1042", got)
	}
}

func TestLastUseSlot(t *testing.T) {
	mp := &mockPlayer{lastUseSlotValue: 7}
	if got := runLastOp(t, OpLastUseSlot, mp, TriggerOpHeldU); got != 7 {
		t.Errorf("LAST_USESLOT: got %d, want 7", got)
	}
}

func TestLastTargetSlot(t *testing.T) {
	mp := &mockPlayer{lastTargetSlotValue: 11}
	if got := runLastOp(t, OpLastTargetSlot, mp, TriggerInvButtonD); got != 11 {
		t.Errorf("LAST_TARGETSLOT: got %d, want 11", got)
	}
}

// TestLastOpsTriggerAllowlistRejects verifies that the per-opcode trigger
// allowlist (PlayerOps.ts:259-340, :1026-1033) rejects out-of-allowlist
// callers, including the zero-value TriggerProc default. One row per
// opcode + one row per LAST_* showing the cross-opcode allowlist boundary
// (LAST_USEITEM rejects under TriggerOpHeldT even though OPHELDT is in
// LAST_ITEM's allowlist).
func TestLastOpsTriggerAllowlistRejects(t *testing.T) {
	tests := []struct {
		name    string
		op      Opcode
		trigger ServerTriggerType
		errSub  string
	}{
		{"item_proc_default", OpLastItem, TriggerProc, "LAST_ITEM"},
		{"item_inv_buttond_outside", OpLastItem, TriggerInvButtonD, "LAST_ITEM"},
		{"slot_proc_default", OpLastSlot, TriggerProc, "LAST_SLOT"},
		{"slot_apnpcu_outside", OpLastSlot, TriggerApNpcU, "LAST_SLOT"},
		{"useitem_proc_default", OpLastUseItem, TriggerProc, "LAST_USEITEM"},
		{"useitem_opheldt_outside", OpLastUseItem, TriggerOpHeldT, "LAST_USEITEM"},
		{"useitem_invbutton1_outside", OpLastUseItem, TriggerInvButton1, "LAST_USEITEM"},
		{"useslot_proc_default", OpLastUseSlot, TriggerProc, "LAST_USESLOT"},
		{"useslot_opheld1_outside", OpLastUseSlot, TriggerOpHeld1, "LAST_USESLOT"},
		{"targetslot_proc_default", OpLastTargetSlot, TriggerProc, "LAST_TARGETSLOT"},
		{"targetslot_invbutton1_outside", OpLastTargetSlot, TriggerInvButton1, "LAST_TARGETSLOT"},
		{"targetslot_opheldu_outside", OpLastTargetSlot, TriggerOpHeldU, "LAST_TARGETSLOT"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			mp := &mockPlayer{}
			err := runLastOpErr(t, tc.op, mp, tc.trigger)
			if err == nil {
				t.Fatalf("want allowlist rejection, got nil")
			}
			if !strings.Contains(err.Error(), tc.errSub) ||
				!strings.Contains(err.Error(), "not safe to use in this trigger") {
				t.Errorf("err = %q, want substring %q + 'not safe to use in this trigger'",
					err.Error(), tc.errSub)
			}
		})
	}
}

// TestLastOpsTriggerAllowlistAccepts spot-checks every other allowlisted
// trigger per opcode (TS PlayerOps.ts:259-340 + :1026-1033). Pairs with
// TestLast{Item,Slot,UseItem,UseSlot,TargetSlot} which cover one allowed
// trigger each. The cross-boundary case (OPHELDU passes LAST_USEITEM but
// fails LAST_ITEM) is exercised in TestLastOpsTriggerAllowlistRejects
// via opheldt_outside.
func TestLastOpsTriggerAllowlistAccepts(t *testing.T) {
	tests := []struct {
		name    string
		op      Opcode
		trigger ServerTriggerType
		want    int
		setup   func(*mockPlayer)
	}{
		{"item_opheld5", OpLastItem, TriggerOpHeld5, 11, func(m *mockPlayer) { m.lastItemValue = 11 }},
		{"item_opheldt", OpLastItem, TriggerOpHeldT, 12, func(m *mockPlayer) { m.lastItemValue = 12 }},
		{"item_opheldu", OpLastItem, TriggerOpHeldU, 13, func(m *mockPlayer) { m.lastItemValue = 13 }},
		{"item_invbutton3", OpLastItem, TriggerInvButton3, 14, func(m *mockPlayer) { m.lastItemValue = 14 }},
		{"slot_invbuttond", OpLastSlot, TriggerInvButtonD, 5, func(m *mockPlayer) { m.lastSlotValue = 5 }},
		{"slot_invbutton5", OpLastSlot, TriggerInvButton5, 6, func(m *mockPlayer) { m.lastSlotValue = 6 }},
		{"useitem_apobju", OpLastUseItem, TriggerApObjU, 21, func(m *mockPlayer) { m.lastUseItemValue = 21 }},
		{"useitem_aplocu", OpLastUseItem, TriggerApLocU, 22, func(m *mockPlayer) { m.lastUseItemValue = 22 }},
		{"useitem_apnpcu", OpLastUseItem, TriggerApNpcU, 23, func(m *mockPlayer) { m.lastUseItemValue = 23 }},
		{"useitem_applayeru", OpLastUseItem, TriggerApPlayerU, 24, func(m *mockPlayer) { m.lastUseItemValue = 24 }},
		{"useitem_opobju", OpLastUseItem, TriggerOpObjU, 25, func(m *mockPlayer) { m.lastUseItemValue = 25 }},
		{"useitem_oplocu", OpLastUseItem, TriggerOpLocU, 26, func(m *mockPlayer) { m.lastUseItemValue = 26 }},
		{"useitem_opnpcu", OpLastUseItem, TriggerOpNpcU, 27, func(m *mockPlayer) { m.lastUseItemValue = 27 }},
		{"useitem_opplayeru", OpLastUseItem, TriggerOpPlayerU, 28, func(m *mockPlayer) { m.lastUseItemValue = 28 }},
		{"useslot_opheldu", OpLastUseSlot, TriggerOpHeldU, 31, func(m *mockPlayer) { m.lastUseSlotValue = 31 }},
		{"useslot_apobju", OpLastUseSlot, TriggerApObjU, 32, func(m *mockPlayer) { m.lastUseSlotValue = 32 }},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			mp := &mockPlayer{}
			tc.setup(mp)
			if got := runLastOp(t, tc.op, mp, tc.trigger); got != tc.want {
				t.Errorf("%s: got %d, want %d", tc.name, got, tc.want)
			}
		})
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
