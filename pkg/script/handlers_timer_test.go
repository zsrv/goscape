package script

import "testing"

func TestSetTimerCapturesArgs(t *testing.T) {
	// NAI-27 Bundle 1: placeholder shape pre-popScriptArgs. Bundle 2
	// re-pins this with real popScriptArgs args after activation.
	sf := &ScriptFile{
		Name: "set_timer",
		Opcodes: []Opcode{
			OpPushConstantInt, // scriptID
			OpPushConstantInt, // interval
			OpSetTimer,
			OpReturn,
		},
		IntOperands:      []int32{0x12345678, 5, 0, 0},
		StringOperands:   []string{"", "", "", ""},
		InstructionCount: 4,
	}
	mp := &mockPlayer{}
	state := Init(sf, mp, false, nil, nil)
	if err := Execute(state); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if mp.setTimerCalls != 1 {
		t.Fatalf("setTimerCalls: got %d, want 1", mp.setTimerCalls)
	}
	got := mp.lastSetTimer
	if got.scriptID != 0x12345678 || got.interval != 5 || got.intArgs != nil || got.stringArgs != nil || got.ttype != TimerNormal {
		t.Errorf("lastSetTimer: got %+v, want scriptID=0x12345678 interval=5 intArgs=nil stringArgs=nil type=Normal", got)
	}
}

func TestSoftTimerSetsSoftType(t *testing.T) {
	// NAI-27 Bundle 1: placeholder shape pre-popScriptArgs.
	sf := &ScriptFile{
		Name: "soft_timer",
		Opcodes: []Opcode{
			OpPushConstantInt, OpPushConstantInt,
			OpSoftTimer, OpReturn,
		},
		IntOperands:      []int32{0x7BCDEF00, 3, 0, 0},
		StringOperands:   []string{"", "", "", ""},
		InstructionCount: 4,
	}
	mp := &mockPlayer{}
	state := Init(sf, mp, false, nil, nil)
	if err := Execute(state); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if mp.lastSetTimer.ttype != TimerSoft {
		t.Errorf("ttype: got %v, want TimerSoft", mp.lastSetTimer.ttype)
	}
}

func TestClearTimerCapturesID(t *testing.T) {
	sf := &ScriptFile{
		Name: "clear_timer",
		Opcodes: []Opcode{
			OpPushConstantInt, OpClearTimer, OpReturn,
		},
		IntOperands:      []int32{0x11111111, 0, 0},
		StringOperands:   []string{"", "", ""},
		InstructionCount: 3,
	}
	mp := &mockPlayer{}
	state := Init(sf, mp, false, nil, nil)
	if err := Execute(state); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if mp.lastClearTimer != 0x11111111 || mp.clearTimerCalls != 1 {
		t.Errorf("ClearTimer: got %#x (x%d calls), want 0x11111111 (x1)", mp.lastClearTimer, mp.clearTimerCalls)
	}
}

func TestGetTimer(t *testing.T) {
	sf := &ScriptFile{
		Name: "get_timer",
		Opcodes: []Opcode{
			OpPushConstantInt, OpGetTimer, OpReturn,
		},
		IntOperands:      []int32{0x22222222, 0, 0},
		StringOperands:   []string{"", "", ""},
		InstructionCount: 3,
	}
	mp := &mockPlayer{getTimerValue: 99}
	state := Init(sf, mp, false, nil, nil)
	if err := Execute(state); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if got := state.PopInt(); got != 99 {
		t.Errorf("GETTIMER push: got %d, want 99", got)
	}
}

func TestTimerOpsRequireActivePlayer(t *testing.T) {
	for _, op := range []Opcode{OpSetTimer, OpSoftTimer, OpClearTimer, OpClearSoftTimer, OpGetTimer} {
		t.Run(op.String(), func(t *testing.T) {
			sf := &ScriptFile{
				Name: "no_self",
				Opcodes: []Opcode{
					OpPushConstantInt, OpPushConstantInt, OpPushConstantInt,
					op, OpReturn,
				},
				IntOperands:      []int32{0, 0, 0, 0, 0},
				StringOperands:   []string{"", "", "", "", ""},
				InstructionCount: 5,
			}
			state := Init(sf, nil, false, nil, nil)
			if err := Execute(state); err == nil {
				t.Errorf("%v: want error with nil Self", op)
			}
		})
	}
}
