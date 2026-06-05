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

// TestDebugWorldStatOps covers MAP_PRODUCTION (DebugOps.ts:16-18) and all
// 12 MAP_LAST* ops (DebugOps.ts:20-66). Each op reads from the mockWorld's
// mapProduction / lastCycleStats fields via the WorldVars interface.
//
// Index order follows the TS WorldStat enum (WorldStat.ts:1-14):
//
//	CYCLE=0 MAP_LASTCLOCK, WORLD=1, CLIENT_IN=2, NPC=3, PLAYER=4,
//	LOGOUT=5, LOGIN=6, ZONE=7, CLIENT_OUT=8, CLEANUP=9,
//	BANDWIDTH_IN=10, BANDWIDTH_OUT=11.
func TestDebugWorldStatOps(t *testing.T) {
	const (
		wantProduction = 1
		wantCycle      = 10
		wantWorld      = 11
		wantClientIn   = 12
		wantNpc        = 13
		wantPlayer     = 14
		wantLogout     = 15
		wantLogin      = 16
		wantZone       = 17
		wantClientOut  = 18
		wantCleanup    = 19
		wantBwIn       = 20
		wantBwOut      = 21
	)

	w := newMockWorld()
	w.mapProduction = wantProduction
	w.lastCycleStats = [12]int{
		wantCycle, wantWorld, wantClientIn, wantNpc, wantPlayer, wantLogout,
		wantLogin, wantZone, wantClientOut, wantCleanup, wantBwIn, wantBwOut,
	}

	tests := []struct {
		name   string
		opcode Opcode
		want   int
	}{
		// TS DebugOps.ts:16-18
		{"MAP_PRODUCTION", OpMapProduction, wantProduction},
		// TS DebugOps.ts:20-22
		{"MAP_LASTCLOCK", OpMapLastClock, wantCycle},
		// TS DebugOps.ts:24-26
		{"MAP_LASTWORLD", OpMapLastWorld, wantWorld},
		// TS DebugOps.ts:28-30
		{"MAP_LASTCLIENTIN", OpMapLastClientIn, wantClientIn},
		// TS DebugOps.ts:32-34
		{"MAP_LASTNPC", OpMapLastNpc, wantNpc},
		// TS DebugOps.ts:36-38
		{"MAP_LASTPLAYER", OpMapLastPlayer, wantPlayer},
		// TS DebugOps.ts:40-42
		{"MAP_LASTLOGOUT", OpMapLastLogout, wantLogout},
		// TS DebugOps.ts:44-46
		{"MAP_LASTLOGIN", OpMapLastLogin, wantLogin},
		// TS DebugOps.ts:48-50
		{"MAP_LASTZONE", OpMapLastZone, wantZone},
		// TS DebugOps.ts:52-54
		{"MAP_LASTCLIENTOUT", OpMapLastClientOut, wantClientOut},
		// TS DebugOps.ts:56-58
		{"MAP_LASTCLEANUP", OpMapLastCleanup, wantCleanup},
		// TS DebugOps.ts:60-62
		{"MAP_LASTBANDWIDTHIN", OpMapLastBandwidthIn, wantBwIn},
		// TS DebugOps.ts:64-66
		{"MAP_LASTBANDWIDTHOUT", OpMapLastBandwidthOut, wantBwOut},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			sf := &ScriptFile{
				Name:             tc.name,
				Opcodes:          []Opcode{tc.opcode, OpReturn},
				IntOperands:      []int32{0, 0},
				StringOperands:   []string{"", ""},
				InstructionCount: 2,
			}
			state := Init(sf, nil, false, nil, nil)
			state.World = w
			if err := Execute(state); err != nil {
				t.Fatalf("Execute: %v", err)
			}
			got := state.PopInt()
			if got != tc.want {
				t.Errorf("%s: got %d, want %d", tc.name, got, tc.want)
			}
		})
	}
}

// TestDebugWorldStatOps_NilWorldReject verifies that each of the 13 world-stat
// ops returns ErrNoWorld when state.World is nil.
func TestDebugWorldStatOps_NilWorldReject(t *testing.T) {
	ops := []Opcode{
		OpMapProduction,
		OpMapLastClock,
		OpMapLastWorld,
		OpMapLastClientIn,
		OpMapLastNpc,
		OpMapLastPlayer,
		OpMapLastLogout,
		OpMapLastLogin,
		OpMapLastZone,
		OpMapLastClientOut,
		OpMapLastCleanup,
		OpMapLastBandwidthIn,
		OpMapLastBandwidthOut,
	}
	for _, op := range ops {
		op := op
		t.Run(op.String(), func(t *testing.T) {
			sf := &ScriptFile{
				Name:             "nil_world",
				Opcodes:          []Opcode{op, OpReturn},
				IntOperands:      []int32{0, 0},
				StringOperands:   []string{"", ""},
				InstructionCount: 2,
			}
			state := Init(sf, nil, false, nil, nil)
			// state.World is nil — handler must return ErrNoWorld.
			err := Execute(state)
			if err == nil {
				t.Fatal("Execute: want ErrNoWorld error, got nil")
			}
		})
	}
}
