package script

import (
	"strings"
	"testing"

	"github.com/zsrv/goscape/pkg/objtype"
)

// mockWorld implements WorldVars for tests.
type mockWorld struct {
	ints       map[int]int32
	strings    map[int]string
	tick       int
	players    int
	mapMembers int
	mapLive    int
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
func (m *mockWorld) MapMembers() int                  { return m.mapMembers }
func (m *mockWorld) MapLive() int                     { return m.mapLive }

// NAI-35-T6: default no-op stubs for the WorldVars surface extension. Tests
// that exercise MAP_FINDSQUARE override these via mapFindSquareWorld
// (handlers_map_test.go).
func (m *mockWorld) IsMapBlocked(level, x, z int) bool { return false }
func (m *mockWorld) IsFreeToPlay(x, z int) bool        { return false }

// NAI-120 Bundle 2A: default no-op stub. Tests exercising MAP_MULTIWAY override
// via a recorder type that wraps mockWorld.
func (m *mockWorld) IsMulti(level, x, z int) bool { return false }

// NAI-36: default no-op stub for SPOTANIM_MAP test fixture. Real recording
// is layered on by handler-specific test types.
func (m *mockWorld) AnimMap(level, x, z, spotanim, height, delay int) {}

// NAI-115 T2: default no-op stub for OBJ_DEL test fixture. Tests
// exercising RemoveObj override via fakeWorldRemoveObj.
func (m *mockWorld) RemoveObj(obj ActiveObj) {}

// NAI-115 T3: default no-op stub for OBJ_ADD/OBJ_ADDALL/INV_DROPSLOT
// test fixture. Tests exercising AddObj override via fakeWorldAddObj.
func (m *mockWorld) AddObj(level, x, z, typeID, count, duration, receiverID int) ActiveObj {
	return nil
}

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

func TestPushVarn_StringType_PushesString(t *testing.T) {
	sf := &ScriptFile{
		Name:             "push_varn_str",
		Opcodes:          []Opcode{OpPushVarn, OpReturn},
		IntOperands:      []int32{3, 0},
		StringOperands:   []string{"", ""},
		InstructionCount: 2,
	}
	npc := &mockNpc{varnsString: map[int]string{3: "hello"}}
	state := Init(sf, nil, false, nil, nil)
	state.ActiveNpc = npc
	state.Configs = &mockConfigs{
		varns: map[int]*objtype.VarNpcType{
			3: {Type: objtype.ScriptVarTypeString},
		},
	}
	if err := Execute(state); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if got := state.PopString(); got != "hello" {
		t.Errorf("PushVarn(STRING): got %q, want %q", got, "hello")
	}
}

func TestPopVarn_StringType_PopsString(t *testing.T) {
	sf := &ScriptFile{
		Name: "pop_varn_str",
		Opcodes: []Opcode{
			OpPushConstantString, // push "abc"
			OpPopVarn,            // write varn 7 = "abc"
			OpReturn,
		},
		IntOperands:      []int32{0, 7, 0},
		StringOperands:   []string{"abc", "", ""},
		InstructionCount: 3,
	}
	npc := &mockNpc{}
	state := Init(sf, nil, false, nil, nil)
	state.ActiveNpc = npc
	state.Configs = &mockConfigs{
		varns: map[int]*objtype.VarNpcType{
			7: {Type: objtype.ScriptVarTypeString},
		},
	}
	if err := Execute(state); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if got := npc.NpcVarNString(7); got != "abc" {
		t.Errorf("NpcVarNString(7): got %q, want %q", got, "abc")
	}
}

func TestPushVarn_PlayerUidDefault_PushesMinusOne(t *testing.T) {
	// Smoke-bind unit pin. A fresh-spawn NPC's player_uid varn reads -1
	// (set by resetEntityForRespawn in T3) — combat gate skips.
	// Here we mock the seeded state directly: mockNpc.varns[N] = -1.
	sf := &ScriptFile{
		Name:             "push_varn_pid",
		Opcodes:          []Opcode{OpPushVarn, OpReturn},
		IntOperands:      []int32{0, 0},
		StringOperands:   []string{"", ""},
		InstructionCount: 2,
	}
	npc := &mockNpc{varns: map[int]int32{0: -1}}
	state := Init(sf, nil, false, nil, nil)
	state.ActiveNpc = npc
	state.Configs = &mockConfigs{
		varns: map[int]*objtype.VarNpcType{
			0: {Type: objtype.ScriptVarTypePlayerUid},
		},
	}
	if err := Execute(state); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if got := state.PopInt(); got != -1 {
		t.Errorf("PushVarn(PLAYER_UID, default-seeded -1): got %d, want -1", got)
	}
}

func TestPushVarn_NilConfigsFallsBackToInt(t *testing.T) {
	// DEVIATION-NAI-121-D3 pin: nil Configs → int dispatch.
	sf := &ScriptFile{
		Name:             "push_varn_nilconfigs",
		Opcodes:          []Opcode{OpPushVarn, OpReturn},
		IntOperands:      []int32{0, 0},
		StringOperands:   []string{"", ""},
		InstructionCount: 2,
	}
	npc := &mockNpc{varns: map[int]int32{0: 99}}
	state := Init(sf, nil, false, nil, nil)
	state.ActiveNpc = npc
	state.Configs = nil // explicit
	if err := Execute(state); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if got := state.PopInt(); got != 99 {
		t.Errorf("PushVarn(nil Configs fallback): got %d, want 99", got)
	}
}

func TestPushVarp_StringType_PushesString(t *testing.T) {
	sf := &ScriptFile{
		Name:             "push_varp_str",
		Opcodes:          []Opcode{OpPushVarp, OpReturn},
		IntOperands:      []int32{2, 0},
		StringOperands:   []string{"", ""},
		InstructionCount: 2,
	}
	mp := &mockPlayer{varpsString: map[int]string{2: "hello"}}
	state := Init(sf, mp, false, nil, nil)
	state.Configs = &mockConfigs{
		varps: map[int]*objtype.VarPlayerType{
			2: {Type: objtype.ScriptVarTypeString},
		},
	}
	if err := Execute(state); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if got := state.PopString(); got != "hello" {
		t.Errorf("PushVarp(STRING): got %q, want %q", got, "hello")
	}
}

func TestPopVarp_StringType_PopsString(t *testing.T) {
	sf := &ScriptFile{
		Name: "pop_varp_str",
		Opcodes: []Opcode{
			OpPushConstantString, // push "xyz"
			OpPopVarp,            // write varp 4 = "xyz"
			OpReturn,
		},
		IntOperands:      []int32{0, 4, 0},
		StringOperands:   []string{"xyz", "", ""},
		InstructionCount: 3,
	}
	mp := &mockPlayer{}
	state := Init(sf, mp, false, nil, nil)
	state.Configs = &mockConfigs{
		varps: map[int]*objtype.VarPlayerType{
			4: {Type: objtype.ScriptVarTypeString},
		},
	}
	if err := Execute(state); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if got := mp.VarpString(4); got != "xyz" {
		t.Errorf("VarpString(4): got %q, want %q", got, "xyz")
	}
}

func TestPopVarp_ProtectGate_DeniesUnprotected(t *testing.T) {
	sf := &ScriptFile{
		Name: "pop_varp_protected_unprot",
		Opcodes: []Opcode{
			OpPushConstantInt,
			OpPopVarp,
			OpReturn,
		},
		IntOperands:      []int32{77, 5, 0},
		StringOperands:   []string{"", "", ""},
		InstructionCount: 3,
	}
	mp := &mockPlayer{}
	state := Init(sf, mp, false /* protect=false */, nil, nil)
	state.Configs = &mockConfigs{
		varps: map[int]*objtype.VarPlayerType{
			5: {Type: objtype.ScriptVarTypeInt, Protect: true},
		},
	}
	err := Execute(state)
	if err == nil {
		t.Fatal("Execute: want Protect-gate error, got nil")
	}
	if !strings.Contains(err.Error(), "requires protected access") {
		t.Errorf("error: got %q, want substring 'requires protected access'", err.Error())
	}
}

func TestPopVarp_ProtectGate_AllowsProtected(t *testing.T) {
	sf := &ScriptFile{
		Name: "pop_varp_protected_prot",
		Opcodes: []Opcode{
			OpPushConstantInt,
			OpPopVarp,
			OpReturn,
		},
		IntOperands:      []int32{77, 5, 0},
		StringOperands:   []string{"", "", ""},
		InstructionCount: 3,
	}
	mp := &mockPlayer{}
	state := Init(sf, mp, true /* protect=true */, nil, nil)
	state.Configs = &mockConfigs{
		varps: map[int]*objtype.VarPlayerType{
			5: {Type: objtype.ScriptVarTypeInt, Protect: true},
		},
	}
	if err := Execute(state); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if got := mp.Varp(5); got != 77 {
		t.Errorf("mp.Varp(5): got %d, want 77", got)
	}
}

func TestPopVarp_NonProtect_NoGate(t *testing.T) {
	// Confirm Protect=false varps don't gate even when state.Protect=false.
	sf := &ScriptFile{
		Name: "pop_varp_unprot",
		Opcodes: []Opcode{
			OpPushConstantInt,
			OpPopVarp,
			OpReturn,
		},
		IntOperands:      []int32{42, 5, 0},
		StringOperands:   []string{"", "", ""},
		InstructionCount: 3,
	}
	mp := &mockPlayer{}
	state := Init(sf, mp, false, nil, nil)
	state.Configs = &mockConfigs{
		varps: map[int]*objtype.VarPlayerType{
			5: {Type: objtype.ScriptVarTypeInt, Protect: false},
		},
	}
	if err := Execute(state); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if got := mp.Varp(5); got != 42 {
		t.Errorf("mp.Varp(5): got %d, want 42", got)
	}
}
