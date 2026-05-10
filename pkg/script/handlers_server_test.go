package script

import (
	"strings"
	"testing"

	"github.com/zsrv/goscape/pkg/objtype"
)

func TestMapClock(t *testing.T) {
	sf := &ScriptFile{
		Name:             "map_clock",
		Opcodes:          []Opcode{OpMapClock, OpReturn},
		IntOperands:      []int32{0, 0},
		StringOperands:   []string{"", ""},
		InstructionCount: 2,
	}
	w := &mockWorld{tick: 1234}
	state := Init(sf, nil, false, nil, nil)
	state.World = w
	if err := Execute(state); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if got := state.PopInt(); got != 1234 {
		t.Errorf("MAP_CLOCK: got %d, want 1234", got)
	}
}

func TestPlayerCount(t *testing.T) {
	sf := &ScriptFile{
		Name:             "playercount",
		Opcodes:          []Opcode{OpPlayerCount, OpReturn},
		IntOperands:      []int32{0, 0},
		StringOperands:   []string{"", ""},
		InstructionCount: 2,
	}
	w := &mockWorld{players: 7}
	state := Init(sf, nil, false, nil, nil)
	state.World = w
	if err := Execute(state); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if got := state.PopInt(); got != 7 {
		t.Errorf("PLAYERCOUNT: got %d, want 7", got)
	}
}

func TestMoveCoord(t *testing.T) {
	// Start at (level=0, x=3222, z=3222), offset by (x=+1, y=+0, z=-2).
	// Pop order: coord, x, y, z (z on top).
	start := (0 << 28) | (3222 << 14) | 3222
	want := (0 << 28) | (3223 << 14) | 3220

	sf := &ScriptFile{
		Name: "movecoord",
		Opcodes: []Opcode{
			OpPushConstantInt, // coord
			OpPushConstantInt, // x
			OpPushConstantInt, // y
			OpPushConstantInt, // z
			OpMoveCoord,
			OpReturn,
		},
		IntOperands:      []int32{int32(start), 1, 0, -2, 0, 0},
		StringOperands:   []string{"", "", "", "", "", ""},
		InstructionCount: 6,
	}
	state := Init(sf, nil, false, nil, nil)
	if err := Execute(state); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if got := state.PopInt(); got != want {
		t.Errorf("MOVECOORD: got %d, want %d", got, want)
	}
}

// --- NAI-37 Task 7: WORLD_DELAY handler unit test --------------------------

func TestWorldDelay_SetsExecutionWorldSuspendedAndDoesNotPop(t *testing.T) {
	s := &ScriptState{
		IntStack:    make([]int, StackCapacity),
		StringStack: make([]string, StackCapacity),
	}
	s.PushInt(999)
	s.PushInt(42)

	startISP := s.ISP
	if err := handleWorldDelay(s); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if got, want := s.Execution, WorldSuspended; got != want {
		t.Errorf("Execution: got %v, want %v", got, want)
	}
	if got, want := s.ISP, startISP; got != want {
		t.Errorf("ISP after handler: got %d, want %d (handler must not pop)", got, want)
	}
	// Verify exact stack contents — top-of-stack is at index ISP-1.
	if got := s.IntStack[s.ISP-1]; got != 42 {
		t.Errorf("top of stack: got %d, want 42", got)
	}
	if got := s.IntStack[s.ISP-2]; got != 999 {
		t.Errorf("next of stack: got %d, want 999", got)
	}
}

func TestServerOpsRequireWorld(t *testing.T) {
	for _, op := range []Opcode{OpMapClock, OpPlayerCount} {
		t.Run(op.String(), func(t *testing.T) {
			sf := &ScriptFile{
				Name:             "no_world",
				Opcodes:          []Opcode{op, OpReturn},
				IntOperands:      []int32{0, 0},
				StringOperands:   []string{"", ""},
				InstructionCount: 2,
			}
			state := Init(sf, nil, false, nil, nil)
			if err := Execute(state); err == nil {
				t.Errorf("%v: want error with nil World", op)
			}
		})
	}
}

// TestHandleSeqLength_PushesDuration pins TS ServerOps.ts:109-111
// (NAI-149). state.pushInt(check(popInt(), SeqTypeValid).duration).
func TestHandleSeqLength_PushesDuration(t *testing.T) {
	seq := &objtype.SeqType{
		ConfigType: objtype.ConfigType{ID: 42},
		Duration:   180, // ticks
	}
	mc := &mockConfigs{
		seqs: map[int]*objtype.SeqType{42: seq},
	}
	s := &ScriptState{
		IntStack:    make([]int, StackCapacity),
		StringStack: make([]string, StackCapacity),
		Configs:     mc,
	}
	s.PushInt(42)
	if err := handleSeqLength(s); err != nil {
		t.Fatalf("handleSeqLength: %v", err)
	}
	if got := s.IntStack[0]; got != 180 {
		t.Errorf("top: got %d, want 180", got)
	}
}

// TestHandleSeqLength_RejectsUnknownID pins TS check(id, SeqTypeValid)
// — unknown id throws.
func TestHandleSeqLength_RejectsUnknownID(t *testing.T) {
	mc := &mockConfigs{
		seqs: map[int]*objtype.SeqType{}, // empty
	}
	s := &ScriptState{
		IntStack:    make([]int, StackCapacity),
		StringStack: make([]string, StackCapacity),
		Configs:     mc,
	}
	s.PushInt(99)
	err := handleSeqLength(s)
	if err == nil {
		t.Fatalf("handleSeqLength: expected error for unknown id")
	}
	if !strings.Contains(err.Error(), "SEQ_LENGTH") {
		t.Errorf("error: got %q, want to contain \"SEQ_LENGTH\"", err.Error())
	}
}
