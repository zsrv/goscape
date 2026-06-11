package script

import (
	"strings"
	"testing"
	"time"
)

func TestErrorAborts(t *testing.T) {
	sf := &ScriptFile{
		Name:             "err",
		Opcodes:          []Opcode{OpPushConstantString, OpError, OpReturn},
		IntOperands:      []int32{0, 0, 0},
		StringOperands:   []string{"bad thing", "", ""},
		InstructionCount: 3,
	}
	state := Init(sf, nil, false, nil, nil)
	err := Execute(state)
	if err == nil {
		t.Fatal("Execute: want error")
	}
	if !strings.Contains(err.Error(), "bad thing") {
		t.Errorf("err msg: got %v, want containing 'bad thing'", err)
	}
	if state.Execution != Aborted {
		t.Errorf("Execution: got %v, want Aborted", state.Execution)
	}
}

// TestTimeSpentSetsStartTime — TIMESPENT seeds state.Timespent with the
// current monotonic time and pushes nothing. Mirrors TS DebugOps.ts:13.
func TestTimeSpentSetsStartTime(t *testing.T) {
	sf := &ScriptFile{
		Name:             "ts_set",
		Opcodes:          []Opcode{OpTimeSpent, OpReturn},
		IntOperands:      []int32{0, 0},
		StringOperands:   []string{"", ""},
		InstructionCount: 2,
	}
	state := Init(sf, nil, false, nil, nil)
	before := time.Now()
	if err := Execute(state); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if state.Timespent.Before(before) {
		t.Errorf("Timespent: %v should be >= %v", state.Timespent, before)
	}
}

// TestGetTimeSpentReturnsElapsedMilliseconds — GETTIMESPENT with unit
// flag 0 pops the flag and pushes elapsed ms since TIMESPENT.
func TestGetTimeSpentReturnsElapsedMilliseconds(t *testing.T) {
	sf := &ScriptFile{
		Name: "ts_elapsed_ms",
		Opcodes: []Opcode{
			OpTimeSpent,
			OpPushConstantInt,
			OpGetTimeSpent,
			OpReturn,
		},
		IntOperands:      []int32{0, 0, 0, 0},
		StringOperands:   []string{"", "", "", ""},
		InstructionCount: 4,
	}
	state := Init(sf, nil, false, nil, nil)
	state.Timespent = time.Now().Add(-50 * time.Millisecond)
	// Don't re-set in TIMESPENT — short-circuit by skipping that opcode
	// would need an opcode replacement; easier to start with a known
	// offset and execute only the GETTIMESPENT branch.
	sf2 := &ScriptFile{
		Name: "ts_get_only",
		Opcodes: []Opcode{
			OpPushConstantInt,
			OpGetTimeSpent,
			OpReturn,
		},
		IntOperands:      []int32{0, 0, 0},
		StringOperands:   []string{"", "", ""},
		InstructionCount: 3,
	}
	state2 := Init(sf2, nil, false, nil, nil)
	state2.Timespent = time.Now().Add(-50 * time.Millisecond)
	if err := Execute(state2); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	got := state2.PopInt()
	if got < 50 || got > 5000 {
		t.Errorf("elapsed ms: got %d, want ~50 (in [50, 5000])", got)
	}
}

// TestGetTimeSpentReturnsElapsedMicroseconds — GETTIMESPENT with unit
// flag 1 returns microseconds instead of milliseconds.
func TestGetTimeSpentReturnsElapsedMicroseconds(t *testing.T) {
	sf := &ScriptFile{
		Name: "ts_us",
		Opcodes: []Opcode{
			OpPushConstantInt,
			OpGetTimeSpent,
			OpReturn,
		},
		IntOperands:      []int32{1, 0, 0},
		StringOperands:   []string{"", "", ""},
		InstructionCount: 3,
	}
	state := Init(sf, nil, false, nil, nil)
	state.Timespent = time.Now().Add(-50 * time.Millisecond)
	if err := Execute(state); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	got := state.PopInt()
	if got < 50_000 || got > 5_000_000 {
		t.Errorf("elapsed us: got %d, want ~50_000 (in [50_000, 5_000_000])", got)
	}
}

// TestGetTimeSpentNoActivePlayerOK — TS DebugOps doesn't gate on active
// player; the stopwatch is per-ScriptState, not per-entity.
func TestGetTimeSpentNoActivePlayerOK(t *testing.T) {
	sf := &ScriptFile{
		Name: "ts_noself_ok",
		Opcodes: []Opcode{
			OpTimeSpent,
			OpPushConstantInt,
			OpGetTimeSpent,
			OpReturn,
		},
		IntOperands:      []int32{0, 0, 0, 0},
		StringOperands:   []string{"", "", "", ""},
		InstructionCount: 4,
	}
	state := Init(sf, nil, false, nil, nil)
	if err := Execute(state); err != nil {
		t.Fatalf("Execute: %v (must not require active player per TS DebugOps)", err)
	}
}

// TestMapLive covers MAP_LIVE (ServerOps.ts @2e3bcf43 pushes
// Environment.NODE_PRODUCTION). The 244-era MAP_PRODUCTION + 12 MAP_LAST*
// debug ops were deleted upstream at the 254 pin-advance; their tests
// left with them.
func TestMapLive(t *testing.T) {
	w := newMockWorld()
	w.mapProduction = 1

	sf := &ScriptFile{
		Name:             "map_live",
		Opcodes:          []Opcode{OpMapLive, OpReturn},
		IntOperands:      []int32{0, 0},
		StringOperands:   []string{"", ""},
		InstructionCount: 2,
	}
	state := Init(sf, nil, false, nil, nil)
	state.World = w
	if err := Execute(state); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if got := state.PopInt(); got != 1 {
		t.Errorf("MAP_LIVE: got %d, want 1", got)
	}
}

// TestMapLive_NilWorldReject verifies MAP_LIVE returns ErrNoWorld when
// state.World is nil.
func TestMapLive_NilWorldReject(t *testing.T) {
	sf := &ScriptFile{
		Name:             "nil_world",
		Opcodes:          []Opcode{OpMapLive, OpReturn},
		IntOperands:      []int32{0, 0},
		StringOperands:   []string{"", ""},
		InstructionCount: 2,
	}
	state := Init(sf, nil, false, nil, nil)
	if err := Execute(state); err == nil {
		t.Fatal("Execute: want ErrNoWorld error, got nil")
	}
}

// TestMidiLength_A10Stub pins the MIDI_LENGTH stub posture: pops the
// track id, errors until Task A10 lands Midi.getTickLength.
func TestMidiLength_A10Stub(t *testing.T) {
	sf := &ScriptFile{
		Name:             "midi_length",
		Opcodes:          []Opcode{OpPushConstantInt, OpMidiLength, OpReturn},
		IntOperands:      []int32{3, 0, 0},
		StringOperands:   []string{"", "", ""},
		InstructionCount: 3,
	}
	state := Init(sf, nil, false, nil, nil)
	err := Execute(state)
	if err == nil {
		t.Fatal("Execute: want A10-stub error, got nil")
	}
	if got := err.Error(); !strings.Contains(got, "MIDI_LENGTH: unimplemented") {
		t.Errorf("error = %q, want it to contain %q", got, "MIDI_LENGTH: unimplemented")
	}
}
