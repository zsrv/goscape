package script

import "testing"

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
