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

// TestMoveCoord_AppliesPackingMasks pins h-server-1 (2026-05-28 audit):
// TS ServerOps.ts:106 packs via CoordGrid.packCoord (CoordGrid.ts:136-138)
// with the 0x3fff/0x3 masks. Pre-fix goscape OR'd raw shifted values, so
// a cx (or cz) delta that pushed the result above 0x3fff bled into the
// level field. This test pushes cx from 0x3fff to 0x4000 with an x=+1
// delta. TS masks 0x4000 → 0x0000 (and cx silently wraps); pre-fix
// goscape leaves the 0x4000 bit set in (cx<<14), which lands at bit 28
// — the lowest level bit — so the unpacked level reads as 1, not 0.
func TestMoveCoord_AppliesPackingMasks(t *testing.T) {
	// Start at (level=0, x=0x3fff, z=0). Packed: (0x3fff << 14) =
	// 268419072 — well within checkCoord's [0, 2147483647] range.
	start := (0 << 28) | (0x3fff << 14) | 0

	sf := &ScriptFile{
		Name: "movecoord_mask",
		Opcodes: []Opcode{
			OpPushConstantInt, // coord
			OpPushConstantInt, // x  delta = +1 → cx = 0x4000 (out of 0x3fff range)
			OpPushConstantInt, // y  delta = 0
			OpPushConstantInt, // z  delta = 0
			OpMoveCoord,
			OpReturn,
		},
		IntOperands:      []int32{int32(start), 1, 0, 0, 0, 0},
		StringOperands:   []string{"", "", "", "", "", ""},
		InstructionCount: 6,
	}
	state := Init(sf, nil, false, nil, nil)
	if err := Execute(state); err != nil {
		t.Fatalf("Execute: %v", err)
	}

	got := state.PopInt()
	gotLevel := (got >> 28) & 0x3
	gotX := (got >> 14) & 0x3fff
	gotZ := got & 0x3fff

	// TS-faithful packCoord(level=0, cx=0x4000, cz=0):
	//   (0 & 0x3) << 28      = 0
	//   (0x4000 & 0x3fff) << 14 = 0   (the 0x4000 bit is dropped)
	//   0 & 0x3fff           = 0
	//   → 0
	if gotLevel != 0 {
		t.Errorf("level bits: got %d, want 0 (cx=0x4000 overflow must NOT bleed into level; TS CoordGrid.packCoord applies (cx & 0x3fff) << 14)", gotLevel)
	}
	if gotX != 0 {
		t.Errorf("cx bits: got 0x%x, want 0x0000 (cx=0x4000 masked by 0x3fff)", gotX)
	}
	if gotZ != 0 {
		t.Errorf("cz bits: got 0x%x, want 0x0000", gotZ)
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
		ID:       42,
		Duration: 180, // ticks
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

// TestHandleMapIndoors_True pins TS ServerOps.ts MAP_INDOORS path that
// pushes 1 when IsIndoors returns true.
func TestHandleMapIndoors_True(t *testing.T) {
	coord := (0 << 28) | (3200 << 14) | 3200
	s := &ScriptState{
		IntStack:    make([]int, StackCapacity),
		StringStack: make([]string, StackCapacity),
		World:       &mockWorld{isIndoorsReturn: true},
	}
	s.PushInt(coord)
	if err := handleMapIndoors(s); err != nil {
		t.Fatalf("handleMapIndoors: %v", err)
	}
	if got := s.PopInt(); got != 1 {
		t.Errorf("MAP_INDOORS true: got %d, want 1", got)
	}
}

// TestHandleMapIndoors_False pins the false path.
func TestHandleMapIndoors_False(t *testing.T) {
	coord := (0 << 28) | (3200 << 14) | 3200
	s := &ScriptState{
		IntStack:    make([]int, StackCapacity),
		StringStack: make([]string, StackCapacity),
		World:       &mockWorld{isIndoorsReturn: false},
	}
	s.PushInt(coord)
	if err := handleMapIndoors(s); err != nil {
		t.Fatalf("handleMapIndoors: %v", err)
	}
	if got := s.PopInt(); got != 0 {
		t.Errorf("MAP_INDOORS false: got %d, want 0", got)
	}
}

// TestHandleMapIndoors_InvalidCoord pins the checkCoord guard — negative
// packed value encodes a negative coord component and must return an error.
func TestHandleMapIndoors_InvalidCoord(t *testing.T) {
	s := &ScriptState{
		IntStack:    make([]int, StackCapacity),
		StringStack: make([]string, StackCapacity),
		World:       &mockWorld{},
	}
	s.PushInt(-1) // negative coord → checkCoord must reject
	err := handleMapIndoors(s)
	if err == nil {
		t.Fatal("MAP_INDOORS: expected error for invalid coord, got nil")
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

// TestCountOps pins the 244 NPCCOUNT/ZONECOUNT/LOCCOUNT/OBJCOUNT handlers.
// Mirrors TS ServerOps.ts:403-417 (verified against 9aadcec4).
func TestCountOps(t *testing.T) {
	type countCase struct {
		name    string
		op      Opcode
		handler func(*ScriptState) error
		field   func(*mockWorld, int)
		errTag  string
	}
	cases := []countCase{
		{
			name:    "NPCCOUNT",
			op:      OpNpcCount,
			handler: handleNpcCount,
			field:   func(w *mockWorld, v int) { w.totalNpcs = v },
			errTag:  "NPCCOUNT",
		},
		{
			name:    "ZONECOUNT",
			op:      OpZoneCount,
			handler: handleZoneCount,
			field:   func(w *mockWorld, v int) { w.totalZones = v },
			errTag:  "ZONECOUNT",
		},
		{
			name:    "LOCCOUNT",
			op:      OpLocCount,
			handler: handleLocCount,
			field:   func(w *mockWorld, v int) { w.totalLocs = v },
			errTag:  "LOCCOUNT",
		},
		{
			name:    "OBJCOUNT",
			op:      OpZoneObjCount,
			handler: handleZoneObjCount,
			field:   func(w *mockWorld, v int) { w.totalObjs = v },
			errTag:  "OBJCOUNT",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name+"_pushesTotal", func(t *testing.T) {
			const canned = 42
			w := &mockWorld{}
			tc.field(w, canned)
			s := &ScriptState{
				IntStack:    make([]int, StackCapacity),
				StringStack: make([]string, StackCapacity),
				World:       w,
			}
			if err := tc.handler(s); err != nil {
				t.Fatalf("%s handler: %v", tc.name, err)
			}
			if got := s.PopInt(); got != canned {
				t.Errorf("%s: got %d, want %d", tc.name, got, canned)
			}
		})

		t.Run(tc.name+"_nilWorldRejects", func(t *testing.T) {
			s := &ScriptState{
				IntStack:    make([]int, StackCapacity),
				StringStack: make([]string, StackCapacity),
			}
			err := tc.handler(s)
			if err == nil {
				t.Fatalf("%s with nil World: want error, got nil", tc.name)
			}
			if !strings.Contains(err.Error(), tc.errTag) {
				t.Errorf("%s nil-world error: got %q, want substring %q",
					tc.name, err.Error(), tc.errTag)
			}
		})

		// Integration path: Execute via opcode dispatch.
		t.Run(tc.name+"_execute", func(t *testing.T) {
			const canned = 77
			w := &mockWorld{}
			tc.field(w, canned)
			sf := &ScriptFile{
				Name:             strings.ToLower(tc.name),
				Opcodes:          []Opcode{tc.op, OpReturn},
				IntOperands:      []int32{0, 0},
				StringOperands:   []string{"", ""},
				InstructionCount: 2,
			}
			state := Init(sf, nil, false, nil, nil)
			state.World = w
			if err := Execute(state); err != nil {
				t.Fatalf("Execute %s: %v", tc.name, err)
			}
			if got := state.PopInt(); got != canned {
				t.Errorf("%s Execute: got %d, want %d", tc.name, got, canned)
			}
		})
	}
}
