package script

import (
	"errors"
	"slices"
	"strings"
	"testing"
)

func TestSetTimerCapturesArgs(t *testing.T) {
	// NAI-27 Bundle 2: real popScriptArgs payload pinned. Stack layout
	// at OpSetTimer (top → bottom):
	//   tags="ii"
	//   20 (intArgs[1] — popScriptArgs pops in tag-reverse order)
	//   10 (intArgs[0])
	//   5  (interval)
	//   0x12345678 (scriptID)
	sf := &ScriptFile{
		Name: "set_timer",
		Opcodes: []Opcode{
			OpPushConstantInt,    // scriptID
			OpPushConstantInt,    // interval
			OpPushConstantInt,    // intArgs[0] = 10
			OpPushConstantInt,    // intArgs[1] = 20
			OpPushConstantString, // type-tags "ii"
			OpSetTimer,
			OpReturn,
		},
		IntOperands:      []int32{0x12345678, 5, 10, 20, 0, 0, 0},
		StringOperands:   []string{"", "", "", "", "ii", "", ""},
		InstructionCount: 7,
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
	if got.scriptID != 0x12345678 || got.interval != 5 || got.ttype != TimerNormal {
		t.Errorf("scalars: got scriptID=%#x interval=%d ttype=%v, want 0x12345678 5 Normal", got.scriptID, got.interval, got.ttype)
	}
	if !slices.Equal(got.intArgs, []int{10, 20}) {
		t.Errorf("intArgs: got %v, want [10 20]", got.intArgs)
	}
	if len(got.stringArgs) != 0 {
		t.Errorf("stringArgs: got %v, want empty/nil", got.stringArgs)
	}
}

func TestSoftTimerSetsSoftType(t *testing.T) {
	// NAI-27 Bundle 2: empty popScriptArgs payload (tags="") to keep this
	// test focused on the type field; full args coverage in TestSoftTimerCapturesArgs.
	sf := &ScriptFile{
		Name: "soft_timer",
		Opcodes: []Opcode{
			OpPushConstantInt,    // scriptID
			OpPushConstantInt,    // interval
			OpPushConstantString, // type-tags "" (empty → nil/nil from popScriptArgs)
			OpSoftTimer,
			OpReturn,
		},
		IntOperands:      []int32{0x7BCDEF00, 3, 0, 0, 0},
		StringOperands:   []string{"", "", "", "", ""},
		InstructionCount: 5,
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
	// NAI-27 Bundle 2: handler-side script-missing check requires the
	// test provider to have a script registered at the queried ID. The
	// mockPlayer.getTimerValue is now interpreted as "the absolute clock
	// the entity returned" (TS-faithful per (*Player).GetTimer flip).
	// GetByID looks up by index in the scripts slice (not by LookupKey).
	// RegisterAt grows the slice and slots the script at the requested id;
	// a small id keeps the slice tiny without weakening the assertion (the
	// pushed value is mocked via getTimerValue and isn't tied to id magnitude).
	const queriedID = uint32(7)
	registered := &ScriptFile{
		Name:             "registered_timer",
		LookupKey:        queriedID,
		Opcodes:          []Opcode{OpReturn},
		IntOperands:      []int32{0},
		StringOperands:   []string{""},
		InstructionCount: 1,
	}
	provider := NewProvider()
	provider.RegisterAt(queriedID, registered)

	sf := &ScriptFile{
		Name: "get_timer",
		Opcodes: []Opcode{
			OpPushConstantInt, OpGetTimer, OpReturn,
		},
		IntOperands:      []int32{int32(queriedID), 0, 0},
		StringOperands:   []string{"", "", ""},
		InstructionCount: 3,
	}
	mp := &mockPlayer{getTimerValue: 99}
	state := Init(sf, mp, false, nil, nil)
	state.Provider = provider
	if err := Execute(state); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if got := state.PopInt(); got != 99 {
		t.Errorf("GETTIMER push: got %d, want 99", got)
	}
}

// TestSoftTimerCapturesArgs pins NAI-27 Bundle 2: SOFTTIMER pops a
// popScriptArgs payload (type-tag string + typed values) and forwards
// the resulting parallel slices to (*Player).SetTimer. Mirrors TS
// PlayerOps.ts:815-826 (popScriptArgs FIRST, then interval, then timerId).
func TestSoftTimerCapturesArgs(t *testing.T) {
	// Stack layout at OpSoftTimer (top → bottom):
	//   tags="ii"
	//   20 (intArgs[1] — popScriptArgs pops in tag-reverse order)
	//   10 (intArgs[0])
	//   3  (interval)
	//   0x12345678 (scriptID)
	sf := &ScriptFile{
		Name: "soft_timer_args",
		Opcodes: []Opcode{
			OpPushConstantInt,    // scriptID  (bottom of stack at OpSoftTimer)
			OpPushConstantInt,    // interval
			OpPushConstantInt,    // intArgs[0] = 10
			OpPushConstantInt,    // intArgs[1] = 20
			OpPushConstantString, // type-tags "ii"
			OpSoftTimer,
			OpReturn,
		},
		IntOperands:      []int32{0x12345678, 3, 10, 20, 0, 0, 0},
		StringOperands:   []string{"", "", "", "", "ii", "", ""},
		InstructionCount: 7,
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
	if got.scriptID != 0x12345678 || got.interval != 3 || got.ttype != TimerSoft {
		t.Errorf("scalars: got scriptID=%#x interval=%d ttype=%v, want 0x12345678 3 Soft", got.scriptID, got.interval, got.ttype)
	}
	if !slices.Equal(got.intArgs, []int{10, 20}) {
		t.Errorf("intArgs: got %v, want [10 20]", got.intArgs)
	}
	if len(got.stringArgs) != 0 {
		t.Errorf("stringArgs: got %v, want empty/nil", got.stringArgs)
	}
}

// TestSetTimerScriptMissing pins NAI-27 Bundle 2: SETTIMER returns a
// non-nil error when the scriptID does not resolve. Mirrors TS
// PlayerOps.ts:838-840 (Unable to find timer script: ${id}).
//
// Requires the mockPlayer to surface the entity-layer error returned
// from SetTimer; mockPlayer.SetTimer's signature gains an error return
// in Bundle 2 (Step 7) parallel to (*Player).EnqueueScriptArgs at
// modules/world/player_script.go:102-118.
func TestSetTimerScriptMissing(t *testing.T) {
	sf := &ScriptFile{
		Name: "set_timer_missing",
		Opcodes: []Opcode{
			OpPushConstantInt,    // scriptID (will not resolve)
			OpPushConstantInt,    // interval
			OpPushConstantString, // type-tag string for popScriptArgs (empty = no args)
			OpSetTimer,
			OpReturn,
		},
		// 0xDEADBEEF as a signed int32 bit pattern (overflows untyped int32; use signed form).
		IntOperands:      []int32{-559038737, 5, 0, 0, 0},
		StringOperands:   []string{"", "", "", "", ""},
		InstructionCount: 5,
	}
	mp := &mockPlayer{setTimerErr: errors.New("unable to find timer script: 3735928559")}
	state := Init(sf, mp, false, nil, nil)
	err := Execute(state)
	if err == nil {
		t.Fatalf("Execute: expected error, got nil")
	}
	if !strings.Contains(err.Error(), "unable to find timer script") {
		t.Errorf("error: got %v, want contains 'unable to find timer script'", err)
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

// TestGetTimerScriptMissing pins NAI-27 Bundle 2: GETTIMER returns a
// non-nil error when the scriptID does not resolve. Mirrors TS
// PlayerOps.ts:852-854 (Unable to find timer script: ${id}).
func TestGetTimerScriptMissing(t *testing.T) {
	// Empty provider — no scripts registered.
	provider := NewProvider()

	sf := &ScriptFile{
		Name: "get_timer_missing",
		Opcodes: []Opcode{
			OpPushConstantInt, OpGetTimer, OpReturn,
		},
		// 0xCAFEBABE as a signed int32 bit pattern (overflows untyped int32; use signed form).
		IntOperands:      []int32{-889275714, 0, 0},
		StringOperands:   []string{"", "", ""},
		InstructionCount: 3,
	}
	mp := &mockPlayer{getTimerValue: 99}
	state := Init(sf, mp, false, nil, nil)
	state.Provider = provider
	err := Execute(state)
	if err == nil {
		t.Fatalf("Execute: expected error, got nil")
	}
	if !strings.Contains(err.Error(), "unable to find timer script") {
		t.Errorf("error: got %v, want contains 'unable to find timer script'", err)
	}
}

// TestSoftTimerScriptMissing pins NAI-27 Bundle 2: SOFTTIMER returns
// the entity-layer script-missing error. Mirrors TS PlayerOps.ts:822-824.
func TestSoftTimerScriptMissing(t *testing.T) {
	sf := &ScriptFile{
		Name: "soft_timer_missing",
		Opcodes: []Opcode{
			OpPushConstantInt,    // scriptID (will not resolve)
			OpPushConstantInt,    // interval
			OpPushConstantString, // type-tag string for popScriptArgs (empty)
			OpSoftTimer,
			OpReturn,
		},
		// 0xCAFEF00D as a signed int32 bit pattern (overflows untyped int32; use signed form).
		IntOperands:      []int32{-889262067, 5, 0, 0, 0},
		StringOperands:   []string{"", "", "", "", ""},
		InstructionCount: 5,
	}
	mp := &mockPlayer{setTimerErr: errors.New("unable to find timer script: 3405705229")}
	state := Init(sf, mp, false, nil, nil)
	err := Execute(state)
	if err == nil {
		t.Fatalf("Execute: expected error, got nil")
	}
	if !strings.Contains(err.Error(), "unable to find timer script") {
		t.Errorf("error: got %v, want contains 'unable to find timer script'", err)
	}
}
