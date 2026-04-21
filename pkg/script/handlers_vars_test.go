package script

import (
	"strings"
	"testing"
)

// mockWorld implements WorldVars for tests.
type mockWorld struct {
	ints    map[int]int32
	strings map[int]string
	tick    int
	players int
}

func newMockWorld() *mockWorld {
	return &mockWorld{
		ints:    make(map[int]int32),
		strings: make(map[int]string),
	}
}

func (m *mockWorld) VarsInt(id int) int32             { return m.ints[id] }
func (m *mockWorld) SetVarsInt(id int, val int32)     { m.ints[id] = val }
func (m *mockWorld) VarsString(id int) string         { return m.strings[id] }
func (m *mockWorld) SetVarsString(id int, val string) { m.strings[id] = val }
func (m *mockWorld) CurrentTick() int                 { return m.tick }
func (m *mockWorld) PlayerCount() int                 { return m.players }

func TestPushVarp(t *testing.T) {
	sf := &ScriptFile{
		Name:             "push_varp",
		Opcodes:          []Opcode{OpPushVarp, OpReturn},
		IntOperands:      []int32{0x42, 0},
		StringOperands:   []string{"", ""},
		InstructionCount: 2,
	}
	mp := &mockPlayer{varps: map[int]int32{0x42: 99}}
	state := Init(sf, mp, false, nil, nil)
	if err := Execute(state); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if got := state.PopInt(); got != 99 {
		t.Errorf("PushVarp: got %d, want 99", got)
	}
}

func TestPushVarpIgnoresSecondaryBit(t *testing.T) {
	// Operand high bit = secondary flag; S5b masks it off.
	sf := &ScriptFile{
		Name:             "push_varp_secondary",
		Opcodes:          []Opcode{OpPushVarp, OpReturn},
		IntOperands:      []int32{0x10042, 0}, // secondary=1, id=0x42
		StringOperands:   []string{"", ""},
		InstructionCount: 2,
	}
	mp := &mockPlayer{varps: map[int]int32{0x42: 99}}
	state := Init(sf, mp, false, nil, nil)
	if err := Execute(state); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if got := state.PopInt(); got != 99 {
		t.Errorf("PushVarp(secondary masked): got %d, want 99", got)
	}
}

func TestPopVarpWritesToSelf(t *testing.T) {
	sf := &ScriptFile{
		Name: "pop_varp",
		Opcodes: []Opcode{
			OpPushConstantInt, // push 77
			OpPopVarp,         // write varp 5 = 77
			OpReturn,
		},
		IntOperands:      []int32{77, 5, 0},
		StringOperands:   []string{"", "", ""},
		InstructionCount: 3,
	}
	mp := &mockPlayer{}
	state := Init(sf, mp, false, nil, nil)
	if err := Execute(state); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if got := mp.Varp(5); got != 77 {
		t.Errorf("mp.Varp(5): got %d, want 77", got)
	}
}

func TestPushVarpRequiresActivePlayer(t *testing.T) {
	sf := &ScriptFile{
		Name:             "push_varp_noself",
		Opcodes:          []Opcode{OpPushVarp, OpReturn},
		IntOperands:      []int32{0, 0},
		StringOperands:   []string{"", ""},
		InstructionCount: 2,
	}
	state := Init(sf, nil, false, nil, nil)
	if err := Execute(state); err == nil {
		t.Fatal("Execute: want error")
	}
}

func TestPushVars(t *testing.T) {
	w := newMockWorld()
	w.SetVarsInt(7, 123)

	sf := &ScriptFile{
		Name:             "push_vars",
		Opcodes:          []Opcode{OpPushVars, OpReturn},
		IntOperands:      []int32{7, 0},
		StringOperands:   []string{"", ""},
		InstructionCount: 2,
	}
	state := Init(sf, nil, false, nil, nil)
	state.World = w
	if err := Execute(state); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if got := state.PopInt(); got != 123 {
		t.Errorf("PushVars: got %d, want 123", got)
	}
}

func TestPopVarsWritesToWorld(t *testing.T) {
	w := newMockWorld()
	sf := &ScriptFile{
		Name: "pop_vars",
		Opcodes: []Opcode{
			OpPushConstantInt,
			OpPopVars,
			OpReturn,
		},
		IntOperands:      []int32{55, 3, 0},
		StringOperands:   []string{"", "", ""},
		InstructionCount: 3,
	}
	state := Init(sf, nil, false, nil, nil)
	state.World = w
	if err := Execute(state); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if got := w.VarsInt(3); got != 55 {
		t.Errorf("w.VarsInt(3): got %d, want 55", got)
	}
}

func TestPushVarnReadsActiveNpc(t *testing.T) {
	sf := &ScriptFile{
		Name:             "push_varn",
		Opcodes:          []Opcode{OpPushVarn, OpReturn},
		IntOperands:      []int32{5, 0},
		StringOperands:   []string{"", ""},
		InstructionCount: 2,
	}
	npc := &mockNpc{varns: map[int]int32{5: 42}}
	state := Init(sf, nil, false, nil, nil)
	state.ActiveNpc = npc
	if err := Execute(state); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if got := state.PopInt(); got != 42 {
		t.Errorf("PushVarn: got %d, want 42", got)
	}
}

func TestPushVarnIgnoresSecondaryBit(t *testing.T) {
	// Operand high bit = secondary flag; S5b masks it off, same for VARN.
	sf := &ScriptFile{
		Name:             "push_varn_secondary",
		Opcodes:          []Opcode{OpPushVarn, OpReturn},
		IntOperands:      []int32{0x10005, 0}, // secondary=1, id=5
		StringOperands:   []string{"", ""},
		InstructionCount: 2,
	}
	npc := &mockNpc{varns: map[int]int32{5: 42}}
	state := Init(sf, nil, false, nil, nil)
	state.ActiveNpc = npc
	if err := Execute(state); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if got := state.PopInt(); got != 42 {
		t.Errorf("PushVarn(secondary masked): got %d, want 42", got)
	}
}

func TestPopVarnWritesActiveNpc(t *testing.T) {
	sf := &ScriptFile{
		Name: "pop_varn",
		Opcodes: []Opcode{
			OpPushConstantInt, // push 99
			OpPopVarn,         // write varn 7 = 99
			OpReturn,
		},
		IntOperands:      []int32{99, 7, 0},
		StringOperands:   []string{"", "", ""},
		InstructionCount: 3,
	}
	npc := &mockNpc{}
	state := Init(sf, nil, false, nil, nil)
	state.ActiveNpc = npc
	if err := Execute(state); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if got := npc.varns[7]; got != 99 {
		t.Errorf("npc.varns[7]: got %d, want 99", got)
	}
}

func TestVarnRequireActiveNpc(t *testing.T) {
	cases := []struct {
		name string
		op   Opcode
		want string
	}{
		{"PUSH_VARN", OpPushVarn, "PUSH_VARN: no active npc"},
		{"POP_VARN", OpPopVarn, "POP_VARN: no active npc"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			sf := &ScriptFile{
				Name:             "varn_noactive_" + tc.name,
				Opcodes:          []Opcode{OpPushConstantInt, tc.op, OpReturn},
				IntOperands:      []int32{0, 0, 0},
				StringOperands:   []string{"", "", ""},
				InstructionCount: 3,
			}
			state := Init(sf, nil, false, nil, nil)
			err := Execute(state)
			if err == nil {
				t.Fatalf("%s: expected error, got nil", tc.name)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("%s: error %q does not contain %q", tc.name, err.Error(), tc.want)
			}
		})
	}
}
