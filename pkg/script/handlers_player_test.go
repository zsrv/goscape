package script

import (
	"fmt"
	"math"
	"slices"
	"strings"
	"testing"

	"github.com/zsrv/goscape/pkg/objtype"
)

// -- mock active entity stubs (S6v) -------------------------------------

type mockActiveLoc struct {
	locType     int
	x, z, level int
	angle       int
	shape       int
}

func (m *mockActiveLoc) LocType() int              { return m.locType }
func (m *mockActiveLoc) Coords() (x, z, level int) { return m.x, m.z, m.level }
func (m *mockActiveLoc) Angle() int                { return m.angle }
func (m *mockActiveLoc) Shape() int                { return m.shape }

type mockActiveNpc struct {
	typeId, x, z, level int
	stats               [8]int
}

func (m *mockActiveNpc) NpcType() int                            { return m.typeId }
func (m *mockActiveNpc) NpcX() int                               { return m.x }
func (m *mockActiveNpc) NpcZ() int                               { return m.z }
func (m *mockActiveNpc) NpcLevel() int                           { return m.level }
func (m *mockActiveNpc) NpcStat(stat int) int                    { return m.stats[stat] }
func (m *mockActiveNpc) NpcBaseStat(stat int) int                { return 0 }
func (m *mockActiveNpc) NpcCategory() int                        { return 0 }
func (m *mockActiveNpc) NpcUID() int                             { return 0 }
func (m *mockActiveNpc) Nid() int                                { return 0 }
func (m *mockActiveNpc) NpcVarN(id int) int32                    { return 0 }
func (m *mockActiveNpc) SetNpcVarN(id int, val int32)            {}
func (m *mockActiveNpc) Say(text []byte)                         {}
func (m *mockActiveNpc) Animate(id, delay int)                   {}
func (m *mockActiveNpc) FaceCoord(x, z int)                      {}
func (m *mockActiveNpc) ChangeType(newType, duration int)        {}
func (m *mockActiveNpc) ChangeTypeKeepAll(newType, duration int) {}
func (m *mockActiveNpc) Damage(amount, dmgType int)              {}

func (m *mockActiveNpc) StoreActiveScript(_ *ScriptState)                      {}
func (m *mockActiveNpc) ClearActiveScript()                                    {}
func (m *mockActiveNpc) OnScriptFinishedOrAborted(_ *ScriptState)              {}
func (m *mockActiveNpc) SetDelayed(_ int)                                      {}
func (m *mockActiveNpc) EnqueueScriptForTrigger(_ ServerTriggerType, _, _ int) {}
func (m *mockActiveNpc) SetTimer(_ int)                                        {}
func (m *mockActiveNpc) SetHuntRange(_ int)                                    {}
func (m *mockActiveNpc) SetHuntMode(_ int)                                     {}
func (m *mockActiveNpc) SetWalkTrigger(_ int)                                  {}
func (m *mockActiveNpc) SetWalkTriggerArg(_ int)                               {}
func (m *mockActiveNpc) Teleport(_, _, _ int)                                  {}
func (m *mockActiveNpc) QueueWaypoint(_, _ int)                                {}
func (m *mockActiveNpc) TargetOp() int                                         { return 0 }
func (m *mockActiveNpc) ClearInteraction()                                     {}
func (m *mockActiveNpc) ResetDefaults()                                        {}
func (m *mockActiveNpc) ClearPatrol()                                          {}
func (m *mockActiveNpc) SetTargetOp(_ int)                                     {}
func (m *mockActiveNpc) SetInteractionScript(_ any, _ int)                     {}

// newSingleOp builds a single-opcode script plus its trailing OpReturn,
// so handler tests can run a handler in isolation and observe the state
// after.
func newSingleOp(name string, op Opcode) *ScriptFile {
	return &ScriptFile{
		Name:             name,
		Opcodes:          []Opcode{op, OpReturn},
		IntOperands:      []int32{0, 0},
		StringOperands:   []string{"", ""},
		InstructionCount: 2,
	}
}

// -- Stat read tests -----------------------------------------------------

func TestStatReadsSeededLevel(t *testing.T) {
	mp := &mockPlayer{}
	mp.levels[3] = 50

	sf := &ScriptFile{
		Name: "stat",
		Opcodes: []Opcode{
			OpPushConstantInt, // push stat id = 3
			OpStat,
			OpReturn,
		},
		IntOperands:      []int32{3, 0, 0},
		StringOperands:   []string{"", "", ""},
		InstructionCount: 3,
	}
	state := Init(sf, mp, false, nil, nil)
	if err := Execute(state); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if got := state.PopInt(); got != 50 {
		t.Errorf("STAT: got %d, want 50", got)
	}
}

func TestStatBaseReadsSeededBase(t *testing.T) {
	mp := &mockPlayer{}
	mp.baseLevels[0] = 7

	sf := &ScriptFile{
		Name: "stat_base",
		Opcodes: []Opcode{
			OpPushConstantInt,
			OpStatBase,
			OpReturn,
		},
		IntOperands:      []int32{0, 0, 0},
		StringOperands:   []string{"", "", ""},
		InstructionCount: 3,
	}
	state := Init(sf, mp, false, nil, nil)
	if err := Execute(state); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if got := state.PopInt(); got != 7 {
		t.Errorf("STAT_BASE: got %d, want 7", got)
	}
}

func TestStatTotalSumsAllBases(t *testing.T) {
	mp := &mockPlayer{}
	for i := 0; i < NumStats; i++ {
		mp.baseLevels[i] = i + 1 // 1..21 → total 231
	}
	state := Init(newSingleOp("stat_total", OpStatTotal), mp, false, nil, nil)
	if err := Execute(state); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if got, want := state.PopInt(), 231; got != want {
		t.Errorf("STAT_TOTAL: got %d, want %d", got, want)
	}
}

// -- Stat mutation tests -------------------------------------------------

func TestStatAddFormula(t *testing.T) {
	// TS: added = current + constant + (base*percent)/100, capped at 255.
	// Seed: id=2, base=80, current=50, constant=10, percent=25
	// → 50 + (10 + 80*25/100) = 50 + (10 + 20) = 80
	mp := &mockPlayer{}
	mp.levels[2] = 50
	mp.baseLevels[2] = 80

	sf := &ScriptFile{
		Name: "stat_add",
		Opcodes: []Opcode{
			OpPushConstantInt, // stat id
			OpPushConstantInt, // constant
			OpPushConstantInt, // percent (top)
			OpStatAdd,
			OpReturn,
		},
		IntOperands:      []int32{2, 10, 25, 0, 0},
		StringOperands:   []string{"", "", "", "", ""},
		InstructionCount: 5,
	}
	state := Init(sf, mp, false, nil, nil)
	if err := Execute(state); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if len(mp.setCurLevelCalls) != 1 {
		t.Fatalf("setCurLevelCalls: got %d, want 1", len(mp.setCurLevelCalls))
	}
	if got := mp.setCurLevelCalls[0]; got.id != 2 || got.level != 80 {
		t.Errorf("STAT_ADD: got %+v, want {id:2,level:80}", got)
	}
}

func TestStatAddCapsAt255(t *testing.T) {
	mp := &mockPlayer{}
	mp.levels[1] = 250
	mp.baseLevels[1] = 250

	sf := &ScriptFile{
		Name: "stat_add_cap",
		Opcodes: []Opcode{
			OpPushConstantInt, OpPushConstantInt, OpPushConstantInt,
			OpStatAdd, OpReturn,
		},
		IntOperands:      []int32{1, 100, 0, 0, 0},
		StringOperands:   []string{"", "", "", "", ""},
		InstructionCount: 5,
	}
	state := Init(sf, mp, false, nil, nil)
	if err := Execute(state); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if got := mp.setCurLevelCalls[0].level; got != 255 {
		t.Errorf("STAT_ADD cap: got %d, want 255", got)
	}
}

func TestStatSubFormula(t *testing.T) {
	// subbed = current - (constant + (base*percent)/100), clamped >=0.
	// id=4, current=60, base=50, constant=5, percent=20 → 60 - (5 + 10) = 45.
	mp := &mockPlayer{}
	mp.levels[4] = 60
	mp.baseLevels[4] = 50

	sf := &ScriptFile{
		Name: "stat_sub",
		Opcodes: []Opcode{
			OpPushConstantInt, OpPushConstantInt, OpPushConstantInt,
			OpStatSub, OpReturn,
		},
		IntOperands:      []int32{4, 5, 20, 0, 0},
		StringOperands:   []string{"", "", "", "", ""},
		InstructionCount: 5,
	}
	state := Init(sf, mp, false, nil, nil)
	if err := Execute(state); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if got := mp.setCurLevelCalls[0]; got.id != 4 || got.level != 45 {
		t.Errorf("STAT_SUB: got %+v, want {id:4,level:45}", got)
	}
}

func TestStatSubFloorsAtZero(t *testing.T) {
	mp := &mockPlayer{}
	mp.levels[5] = 3
	mp.baseLevels[5] = 50

	sf := &ScriptFile{
		Name: "stat_sub_floor",
		Opcodes: []Opcode{
			OpPushConstantInt, OpPushConstantInt, OpPushConstantInt,
			OpStatSub, OpReturn,
		},
		IntOperands:      []int32{5, 100, 0, 0, 0},
		StringOperands:   []string{"", "", "", "", ""},
		InstructionCount: 5,
	}
	state := Init(sf, mp, false, nil, nil)
	if err := Execute(state); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if got := mp.setCurLevelCalls[0].level; got != 0 {
		t.Errorf("STAT_SUB floor: got %d, want 0", got)
	}
}

func TestStatBoostClampsToBasePlusBoost(t *testing.T) {
	// TS: boost=10, boosted = max(min(cur+boost, base+boost), cur).
	// id=0, cur=50, base=80, constant=10, percent=0 → boost=10.
	// cur+boost=60; base+boost=90; min=60; max(60, 50)=60.
	mp := &mockPlayer{}
	mp.levels[0] = 50
	mp.baseLevels[0] = 80

	sf := &ScriptFile{
		Name: "stat_boost",
		Opcodes: []Opcode{
			OpPushConstantInt, OpPushConstantInt, OpPushConstantInt,
			OpStatBoost, OpReturn,
		},
		IntOperands:      []int32{0, 10, 0, 0, 0},
		StringOperands:   []string{"", "", "", "", ""},
		InstructionCount: 5,
	}
	state := Init(sf, mp, false, nil, nil)
	if err := Execute(state); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if got := mp.setCurLevelCalls[0].level; got != 60 {
		t.Errorf("STAT_BOOST: got %d, want 60", got)
	}
}

func TestStatBoostNeverLowersCurrent(t *testing.T) {
	// If cur is already above base+boost, the max(cur,...) clamp keeps cur.
	// id=0, cur=120, base=80, boost=10 → cur+boost=130, base+boost=90,
	// min(130,90)=90, max(90, 120)=120.
	mp := &mockPlayer{}
	mp.levels[0] = 120
	mp.baseLevels[0] = 80

	sf := &ScriptFile{
		Name: "stat_boost_noop",
		Opcodes: []Opcode{
			OpPushConstantInt, OpPushConstantInt, OpPushConstantInt,
			OpStatBoost, OpReturn,
		},
		IntOperands:      []int32{0, 10, 0, 0, 0},
		StringOperands:   []string{"", "", "", "", ""},
		InstructionCount: 5,
	}
	state := Init(sf, mp, false, nil, nil)
	if err := Execute(state); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if got := mp.setCurLevelCalls[0].level; got != 120 {
		t.Errorf("STAT_BOOST noop: got %d, want 120", got)
	}
}

func TestStatDrainUsesCurrentNotBase(t *testing.T) {
	// TS: drain uses current, not base.
	// id=2, cur=80, base=20, constant=0, percent=25 → 80 - (0 + 80*25/100) = 80 - 20 = 60.
	mp := &mockPlayer{}
	mp.levels[2] = 80
	mp.baseLevels[2] = 20 // deliberately different from cur to catch the bug

	sf := &ScriptFile{
		Name: "stat_drain",
		Opcodes: []Opcode{
			OpPushConstantInt, OpPushConstantInt, OpPushConstantInt,
			OpStatDrain, OpReturn,
		},
		IntOperands:      []int32{2, 0, 25, 0, 0},
		StringOperands:   []string{"", "", "", "", ""},
		InstructionCount: 5,
	}
	state := Init(sf, mp, false, nil, nil)
	if err := Execute(state); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if got := mp.setCurLevelCalls[0].level; got != 60 {
		t.Errorf("STAT_DRAIN: got %d, want 60", got)
	}
}

func TestStatHealCapsAtBase(t *testing.T) {
	// healed = cur + (constant + (base*percent)/100), capped at base.
	// id=3, cur=10, base=50, constant=100, percent=0 → healed=110, capped to 50.
	mp := &mockPlayer{}
	mp.levels[3] = 10
	mp.baseLevels[3] = 50

	sf := &ScriptFile{
		Name: "stat_heal",
		Opcodes: []Opcode{
			OpPushConstantInt, OpPushConstantInt, OpPushConstantInt,
			OpStatHeal, OpReturn,
		},
		IntOperands:      []int32{3, 100, 0, 0, 0},
		StringOperands:   []string{"", "", "", "", ""},
		InstructionCount: 5,
	}
	state := Init(sf, mp, false, nil, nil)
	if err := Execute(state); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if got := mp.setCurLevelCalls[0].level; got != 50 {
		t.Errorf("STAT_HEAL cap: got %d, want 50", got)
	}
}

func TestStatHealNeverLowersCurrent(t *testing.T) {
	// If cur > base (boosted), max(min(healed, base), cur) keeps cur.
	// id=3, cur=99, base=50 → min(99+const, 50)=50, max(50, 99)=99.
	mp := &mockPlayer{}
	mp.levels[3] = 99
	mp.baseLevels[3] = 50

	sf := &ScriptFile{
		Name: "stat_heal_noop",
		Opcodes: []Opcode{
			OpPushConstantInt, OpPushConstantInt, OpPushConstantInt,
			OpStatHeal, OpReturn,
		},
		IntOperands:      []int32{3, 10, 0, 0, 0},
		StringOperands:   []string{"", "", "", "", ""},
		InstructionCount: 5,
	}
	state := Init(sf, mp, false, nil, nil)
	if err := Execute(state); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if got := mp.setCurLevelCalls[0].level; got != 99 {
		t.Errorf("STAT_HEAL noop: got %d, want 99", got)
	}
}

func TestStatAdvanceForwardsToAddXP(t *testing.T) {
	// TS popInts(2) = [stat, xp]; stack top = xp.
	mp := &mockPlayer{}

	sf := &ScriptFile{
		Name: "stat_advance",
		Opcodes: []Opcode{
			OpPushConstantInt, // stat
			OpPushConstantInt, // xp (top)
			OpStatAdvance,
			OpReturn,
		},
		IntOperands:      []int32{7, 250, 0, 0},
		StringOperands:   []string{"", "", "", ""},
		InstructionCount: 4,
	}
	state := Init(sf, mp, false, nil, nil)
	if err := Execute(state); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if len(mp.addXPCalls) != 1 {
		t.Fatalf("addXPCalls: got %d, want 1", len(mp.addXPCalls))
	}
	if got := mp.addXPCalls[0]; got.id != 7 || got.xp != 250 {
		t.Errorf("STAT_ADVANCE: got %+v, want {id:7,xp:250}", got)
	}
}

func TestStatRandomPushesZeroOrOne(t *testing.T) {
	// Can't assert the exact value without reseeding rand; just confirm
	// it's 0 or 1.
	mp := &mockPlayer{}
	mp.levels[6] = 50

	sf := &ScriptFile{
		Name: "stat_random",
		Opcodes: []Opcode{
			OpPushConstantInt, // stat
			OpPushConstantInt, // low
			OpPushConstantInt, // high (top)
			OpStatRandom,
			OpReturn,
		},
		IntOperands:      []int32{6, 10, 200, 0, 0},
		StringOperands:   []string{"", "", "", "", ""},
		InstructionCount: 5,
	}
	state := Init(sf, mp, false, nil, nil)
	if err := Execute(state); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if got := state.PopInt(); got != 0 && got != 1 {
		t.Errorf("STAT_RANDOM: got %d, want 0 or 1", got)
	}
}

// -- OOB stat id tests ---------------------------------------------------

// Covers STAT, STAT_BASE, STAT_ADD, STAT_SUB, STAT_BOOST, STAT_DRAIN,
// STAT_HEAL, STAT_ADVANCE, STAT_RANDOM with both -1 and NumStats=21.
func TestStatOpsRejectOOBStatID(t *testing.T) {
	type opCase struct {
		name      string
		op        Opcode
		intsBelow []int32 // additional ints pushed below the stat id
	}
	ops := []opCase{
		{"STAT", OpStat, nil},
		{"STAT_BASE", OpStatBase, nil},
		{"STAT_ADD", OpStatAdd, []int32{0, 0}}, // constant, percent
		{"STAT_SUB", OpStatSub, []int32{0, 0}},
		{"STAT_BOOST", OpStatBoost, []int32{0, 0}},
		{"STAT_DRAIN", OpStatDrain, []int32{0, 0}},
		{"STAT_HEAL", OpStatHeal, []int32{0, 0}},
		{"STAT_ADVANCE", OpStatAdvance, []int32{0}},  // xp
		{"STAT_RANDOM", OpStatRandom, []int32{0, 0}}, // low, high
	}
	badIDs := []int32{-1, int32(NumStats)} // 21 is OOB

	for _, tc := range ops {
		for _, badID := range badIDs {
			t.Run(tc.name+"/id="+itoa(int(badID)), func(t *testing.T) {
				// Build a script: push stat id, push the "below" ints, then the op.
				pushes := 1 + len(tc.intsBelow)
				opcodes := make([]Opcode, 0, pushes+2)
				operands := make([]int32, 0, pushes+2)
				opcodes = append(opcodes, OpPushConstantInt)
				operands = append(operands, badID)
				for _, v := range tc.intsBelow {
					opcodes = append(opcodes, OpPushConstantInt)
					operands = append(operands, v)
				}
				opcodes = append(opcodes, tc.op, OpReturn)
				operands = append(operands, 0, 0)

				sf := &ScriptFile{
					Name:             "oob_" + tc.name,
					Opcodes:          opcodes,
					IntOperands:      operands,
					StringOperands:   make([]string, len(opcodes)),
					InstructionCount: uint32(len(opcodes)),
				}
				state := Init(sf, &mockPlayer{}, false, nil, nil)
				if err := Execute(state); err == nil {
					t.Fatalf("%s id=%d: Execute returned nil, want error", tc.name, badID)
				}
				if state.Execution != Aborted {
					t.Errorf("%s id=%d: Execution = %v, want Aborted", tc.name, badID, state.Execution)
				}
			})
		}
	}
}

// -- Coord / facing / teleport tests ------------------------------------

func TestCoordPushesPacked(t *testing.T) {
	mp := &mockPlayer{coordPacked: 0x1234_5678}
	state := Init(newSingleOp("coord", OpCoord), mp, false, nil, nil)
	if err := Execute(state); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if got := state.PopInt(); got != 0x1234_5678 {
		t.Errorf("COORD: got %#x, want %#x", got, 0x1234_5678)
	}
}

func packCoord(level, x, z int) int {
	return ((level & 0x3) << 28) | ((x & 0x3fff) << 14) | (z & 0x3fff)
}

func TestPTeleJumpUnpacksCoord(t *testing.T) {
	// Lumbridge-style test: (3222, 3222, 0).
	mp := &mockPlayer{}
	packed := packCoord(0, 3222, 3222)
	sf := &ScriptFile{
		Name: "p_telejump",
		Opcodes: []Opcode{
			OpPushConstantInt, OpPTeleJump, OpReturn,
		},
		IntOperands:      []int32{int32(packed), 0, 0},
		StringOperands:   []string{"", "", ""},
		InstructionCount: 3,
	}
	state := Init(sf, mp, true, nil, nil)
	if err := Execute(state); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if mp.teleJumpCalls != 1 {
		t.Fatalf("teleJumpCalls: got %d, want 1", mp.teleJumpCalls)
	}
	if mp.lastTeleJump != (struct{ x, z, level int }{3222, 3222, 0}) {
		t.Errorf("P_TELEJUMP: got %+v, want {3222, 3222, 0}", mp.lastTeleJump)
	}
}

func TestPTeleJumpRoundTripsLevel(t *testing.T) {
	// Level 3 (the 2-bit max) exercises the level mask.
	mp := &mockPlayer{}
	packed := packCoord(3, 3222, 3222)
	sf := &ScriptFile{
		Name: "p_telejump_level3",
		Opcodes: []Opcode{
			OpPushConstantInt, OpPTeleJump, OpReturn,
		},
		IntOperands:      []int32{int32(packed), 0, 0},
		StringOperands:   []string{"", "", ""},
		InstructionCount: 3,
	}
	state := Init(sf, mp, true, nil, nil)
	if err := Execute(state); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if mp.lastTeleJump != (struct{ x, z, level int }{3222, 3222, 3}) {
		t.Errorf("P_TELEJUMP level=3: got %+v, want {3222, 3222, 3}", mp.lastTeleJump)
	}
}

func TestPTeleportUnpacksCoord(t *testing.T) {
	mp := &mockPlayer{}
	packed := packCoord(2, 1000, 2000)
	sf := &ScriptFile{
		Name: "p_teleport",
		Opcodes: []Opcode{
			OpPushConstantInt, OpPTeleport, OpReturn,
		},
		IntOperands:      []int32{int32(packed), 0, 0},
		StringOperands:   []string{"", "", ""},
		InstructionCount: 3,
	}
	state := Init(sf, mp, true, nil, nil)
	if err := Execute(state); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if mp.teleportCalls != 1 {
		t.Fatalf("teleportCalls: got %d, want 1", mp.teleportCalls)
	}
	if mp.lastTeleport != (struct{ x, z, level int }{1000, 2000, 2}) {
		t.Errorf("P_TELEPORT: got %+v, want {1000, 2000, 2}", mp.lastTeleport)
	}
}

func TestFaceSquareIgnoresLevelComponent(t *testing.T) {
	// FaceSquare takes (x, z) only — the level bits are discarded.
	mp := &mockPlayer{}
	packed := packCoord(2, 3200, 3250)
	sf := &ScriptFile{
		Name: "facesquare",
		Opcodes: []Opcode{
			OpPushConstantInt, OpFaceSquare, OpReturn,
		},
		IntOperands:      []int32{int32(packed), 0, 0},
		StringOperands:   []string{"", "", ""},
		InstructionCount: 3,
	}
	state := Init(sf, mp, false, nil, nil)
	if err := Execute(state); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if mp.faceSquareCalls != 1 {
		t.Fatalf("faceSquareCalls: got %d, want 1", mp.faceSquareCalls)
	}
	if mp.lastFaceSquare != (struct{ x, z int }{3200, 3250}) {
		t.Errorf("FACESQUARE: got %+v, want {3200, 3250}", mp.lastFaceSquare)
	}
}

func TestPWalkStubPopsAndLogs(t *testing.T) {
	// Stub: pop one int, log.Debug, return nil. No captured state.
	mp := &mockPlayer{}
	sf := &ScriptFile{
		Name: "p_walk",
		Opcodes: []Opcode{
			OpPushConstantInt, OpPWalk, OpReturn,
		},
		IntOperands:      []int32{0x12345, 0, 0},
		StringOperands:   []string{"", "", ""},
		InstructionCount: 3,
	}
	state := Init(sf, mp, false, nil, nil)
	if err := Execute(state); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if state.Execution != Finished {
		t.Errorf("P_WALK stub: Execution = %v, want Finished", state.Execution)
	}
	// Stack should be empty (arg was popped, nothing pushed).
	if state.ISP != 0 {
		t.Errorf("P_WALK stub: ISP = %d, want 0", state.ISP)
	}
}

// -- Animation tests -----------------------------------------------------

func TestAnimCapturesSeqAndDelay(t *testing.T) {
	// TS pops (seq, delay); stack top is delay.
	mp := &mockPlayer{}
	sf := &ScriptFile{
		Name: "anim",
		Opcodes: []Opcode{
			OpPushConstantInt, // seq
			OpPushConstantInt, // delay (top)
			OpAnim,
			OpReturn,
		},
		IntOperands:      []int32{808, 5, 0, 0},
		StringOperands:   []string{"", "", "", ""},
		InstructionCount: 4,
	}
	state := Init(sf, mp, false, nil, nil)
	if err := Execute(state); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if mp.playAnimCalls != 1 {
		t.Fatalf("playAnimCalls: got %d, want 1", mp.playAnimCalls)
	}
	if mp.lastPlayAnim != (struct{ seqID, delay int }{808, 5}) {
		t.Errorf("ANIM: got %+v, want {seqID:808, delay:5}", mp.lastPlayAnim)
	}
}

func TestSpotAnimPlCapturesTriple(t *testing.T) {
	// TS pops (spotanim, height, delay); stack top is delay.
	mp := &mockPlayer{}
	sf := &ScriptFile{
		Name: "spotanim_pl",
		Opcodes: []Opcode{
			OpPushConstantInt, // spotanim id
			OpPushConstantInt, // height
			OpPushConstantInt, // delay (top)
			OpSpotAnimPl,
			OpReturn,
		},
		IntOperands:      []int32{42, 100, 3, 0, 0},
		StringOperands:   []string{"", "", "", "", ""},
		InstructionCount: 5,
	}
	state := Init(sf, mp, false, nil, nil)
	if err := Execute(state); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if mp.playSpotAnimCalls != 1 {
		t.Fatalf("playSpotAnimCalls: got %d, want 1", mp.playSpotAnimCalls)
	}
	want := struct{ id, height, delay int }{42, 100, 3}
	if mp.lastPlaySpotAnim != want {
		t.Errorf("SPOTANIM_PL: got %+v, want %+v", mp.lastPlaySpotAnim, want)
	}
}

// Table-driven test covering every BAS setter. All seven take (seqID)
// and call the corresponding SetXxxAnim on mockPlayer.
func TestBASSetters(t *testing.T) {
	cases := []struct {
		name string
		op   Opcode
		get  func(*mockPlayer) int
	}{
		{"READYANIM", OpReadyAnim, func(m *mockPlayer) int { return m.lastReadyAnim }},
		{"TURNANIM", OpTurnAnim, func(m *mockPlayer) int { return m.lastTurnAnim }},
		{"WALKANIM", OpWalkAnim, func(m *mockPlayer) int { return m.lastWalkAnim }},
		{"WALKANIM_B", OpWalkAnimB, func(m *mockPlayer) int { return m.lastWalkAnimB }},
		{"WALKANIM_L", OpWalkAnimL, func(m *mockPlayer) int { return m.lastWalkAnimL }},
		{"WALKANIM_R", OpWalkAnimR, func(m *mockPlayer) int { return m.lastWalkAnimR }},
		{"RUNANIM", OpRunAnim, func(m *mockPlayer) int { return m.lastRunAnim }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			mp := &mockPlayer{}
			sf := &ScriptFile{
				Name: tc.name,
				Opcodes: []Opcode{
					OpPushConstantInt, tc.op, OpReturn,
				},
				IntOperands:      []int32{1234, 0, 0},
				StringOperands:   []string{"", "", ""},
				InstructionCount: 3,
			}
			state := Init(sf, mp, false, nil, nil)
			if err := Execute(state); err != nil {
				t.Fatalf("Execute: %v", err)
			}
			if got := tc.get(mp); got != 1234 {
				t.Errorf("%s: got %d, want 1234", tc.name, got)
			}
		})
	}
}

func TestRunAnimAcceptsMinusOne(t *testing.T) {
	// TS-behaviour check: -1 clears the run animation. The handler
	// forwards it unconditionally to SetRunAnim.
	mp := &mockPlayer{}
	sf := &ScriptFile{
		Name: "runanim_clear",
		Opcodes: []Opcode{
			OpPushConstantInt, OpRunAnim, OpReturn,
		},
		IntOperands:      []int32{-1, 0, 0},
		StringOperands:   []string{"", "", ""},
		InstructionCount: 3,
	}
	state := Init(sf, mp, false, nil, nil)
	if err := Execute(state); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if mp.lastRunAnim != -1 {
		t.Errorf("RUNANIM -1: got %d, want -1", mp.lastRunAnim)
	}
}

// -- Active-player-required negative tests -------------------------------

// Every handler that dereferences Self must return an error when
// Self == nil (no active player). Runs one representative handler from
// each category.
func TestHandlersRequireActivePlayer(t *testing.T) {
	cases := []struct {
		name string
		op   Opcode
	}{
		{"STAT", OpStat},
		{"STAT_BASE", OpStatBase},
		{"STAT_TOTAL", OpStatTotal},
		{"STAT_ADD", OpStatAdd},
		{"STAT_SUB", OpStatSub},
		{"STAT_BOOST", OpStatBoost},
		{"STAT_DRAIN", OpStatDrain},
		{"STAT_HEAL", OpStatHeal},
		{"STAT_ADVANCE", OpStatAdvance},
		{"STAT_RANDOM", OpStatRandom},
		{"COORD", OpCoord},
		{"FACESQUARE", OpFaceSquare},
		{"P_TELEPORT", OpPTeleport},
		{"P_TELEJUMP", OpPTeleJump},
		{"ANIM", OpAnim},
		{"SPOTANIM_PL", OpSpotAnimPl},
		{"READYANIM", OpReadyAnim},
		{"TURNANIM", OpTurnAnim},
		{"WALKANIM", OpWalkAnim},
		{"WALKANIM_B", OpWalkAnimB},
		{"WALKANIM_L", OpWalkAnimL},
		{"WALKANIM_R", OpWalkAnimR},
		{"RUNANIM", OpRunAnim},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			state := Init(newSingleOp(tc.name, tc.op), nil, false, nil, nil)
			if err := Execute(state); err == nil {
				t.Fatalf("%s with nil Self: Execute returned nil, want error", tc.name)
			}
		})
	}
}

// -- Small helpers -------------------------------------------------------

// itoa without importing strconv at test scope; just for sub-test names.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var buf [12]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}

func TestPStopAction(t *testing.T) {
	sf := &ScriptFile{
		Name:             "stop",
		Opcodes:          []Opcode{OpPStopAction, OpReturn},
		IntOperands:      []int32{0, 0},
		StringOperands:   []string{"", ""},
		InstructionCount: 2,
	}
	mp := &mockPlayer{}
	state := Init(sf, mp, true, nil, nil)
	if err := Execute(state); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if mp.stopActionCalls != 1 {
		t.Errorf("stopActionCalls: got %d, want 1", mp.stopActionCalls)
	}
}

func TestPClearPendingAction(t *testing.T) {
	sf := &ScriptFile{
		Name:             "clear",
		Opcodes:          []Opcode{OpPClearPendingAction, OpReturn},
		IntOperands:      []int32{0, 0},
		StringOperands:   []string{"", ""},
		InstructionCount: 2,
	}
	mp := &mockPlayer{}
	state := Init(sf, mp, true, nil, nil)
	if err := Execute(state); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if mp.clearPendingActionCalls != 1 {
		t.Errorf("clearPendingActionCalls: got %d, want 1", mp.clearPendingActionCalls)
	}
}

// -- P_APRANGE tests -----------------------------------------------------

func TestHandlePApRangeSetsBothFields(t *testing.T) {
	fake := &mockPlayer{}
	s := &ScriptState{
		IntStack: make([]int, StackCapacity),
		Self:     fake,
		Protect:  true,
	}
	s.Pointers |= PtrActivePlayer
	s.PushInt(5)

	if err := handlePApRange(s); err != nil {
		t.Fatalf("handlePApRange: %v", err)
	}

	if fake.lastApRange != 5 {
		t.Errorf("lastApRange: got %d, want 5", fake.lastApRange)
	}
	if !fake.lastApRangeCalled {
		t.Error("lastApRangeCalled: want true")
	}
	if fake.setApRangeCalls != 1 {
		t.Errorf("setApRangeCalls: got %d, want 1", fake.setApRangeCalls)
	}
}

func TestHandlePApRangeRequiresActivePlayer(t *testing.T) {
	s := &ScriptState{
		IntStack: make([]int, StackCapacity),
	}
	s.PushInt(5)

	err := handlePApRange(s)
	if err == nil {
		t.Fatal("handlePApRange: expected error, got nil")
	}
	if got := err.Error(); got != "P_APRANGE: no active player" {
		t.Errorf("error: got %q, want \"P_APRANGE: no active player\"", got)
	}
}

func TestHandlePApRangeAcceptsNegative(t *testing.T) {
	// NAI-24 Bundle 1: TS NumberNotNull only rejects -1; other negatives
	// are accepted. Use -2 to verify negative-but-not-null still passes.
	fake := &mockPlayer{}
	s := &ScriptState{
		IntStack: make([]int, StackCapacity),
		Self:     fake,
		Protect:  true,
	}
	s.Pointers |= PtrActivePlayer
	s.PushInt(-2)

	if err := handlePApRange(s); err != nil {
		t.Fatalf("handlePApRange: %v", err)
	}
	if fake.lastApRange != -2 {
		t.Errorf("lastApRange: got %d, want -2", fake.lastApRange)
	}
	if !fake.lastApRangeCalled {
		t.Error("lastApRangeCalled: want true even for negative apRange")
	}
}

func TestHandlePApRangeAcceptsZero(t *testing.T) {
	fake := &mockPlayer{}
	s := &ScriptState{
		IntStack: make([]int, StackCapacity),
		Self:     fake,
		Protect:  true,
	}
	s.Pointers |= PtrActivePlayer
	s.PushInt(0)

	if err := handlePApRange(s); err != nil {
		t.Fatalf("handlePApRange: %v", err)
	}
	if fake.lastApRange != 0 {
		t.Errorf("lastApRange: got %d, want 0", fake.lastApRange)
	}
	if !fake.lastApRangeCalled {
		t.Error("lastApRangeCalled: want true for zero apRange")
	}
}

// -- S6v: p_op* tests ----------------------------------------------------

// TestPOpLocAnchorsOnActiveLoc — happy path for P_OPLOC.
func TestPOpLocAnchorsOnActiveLoc(t *testing.T) {
	mp := &mockPlayer{}
	loc := &mockActiveLoc{locType: 42}

	sf := &ScriptFile{
		Name:             "p_op_loc",
		Opcodes:          []Opcode{OpPushConstantInt, OpPOpLoc, OpReturn},
		IntOperands:      []int32{3, 0, 0},
		StringOperands:   []string{"", "", ""},
		InstructionCount: 3,
	}
	state := Init(sf, mp, true, nil, nil)
	state.ActiveLoc = loc
	state.Pointers |= PtrActiveLoc
	if err := Execute(state); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if len(mp.lastSetInteractionScriptLoc) != 1 {
		t.Fatalf("expected 1 SetInteractionScriptLoc call, got %d", len(mp.lastSetInteractionScriptLoc))
	}
	got := mp.lastSetInteractionScriptLoc[0]
	if got.Loc != loc || got.Op != 3 {
		t.Errorf("args: got %+v, want {Loc:%p, Op:3}", got, loc)
	}
}

// TestPOpLocNoActivePlayerErrors — requireActivePlayer gate fires.
func TestPOpLocNoActivePlayerErrors(t *testing.T) {
	sf := newSingleOp("p_op_loc_no_player", OpPOpLoc)
	state := Init(sf, nil, false, nil, nil)
	state.PushInt(3)

	err := Execute(state)
	if err == nil || err.Error() != "P_OPLOC: no active player" {
		t.Errorf("expected 'P_OPLOC: no active player', got %v", err)
	}
}

// TestPOpLocNoActiveLocErrors — nil ActiveLoc.
func TestPOpLocNoActiveLocErrors(t *testing.T) {
	mp := &mockPlayer{}

	sf := newSingleOp("p_op_loc_no_loc", OpPOpLoc)
	state := Init(sf, mp, true, nil, nil)
	state.PushInt(3)

	err := Execute(state)
	if err == nil || err.Error() != "P_OPLOC: no active loc" {
		t.Errorf("expected 'P_OPLOC: no active loc', got %v", err)
	}
}

// TestPOpLocInvalidOpErrors — op out of [1,5] range.
func TestPOpLocInvalidOpErrors(t *testing.T) {
	// NAI-24 Bundle 1: -1 is now caught by the NumberNotNull wrap (TS
	// PlayerOps.ts:387) before reaching the [1..5] range check; covered
	// separately by TestHandlePOpLocNullRejected. Other out-of-range
	// values still produce "invalid op".
	for _, op := range []int32{0, 6, 100} {
		mp := &mockPlayer{}
		loc := &mockActiveLoc{locType: 42}

		sf := &ScriptFile{
			Name:             "p_op_loc_invalid",
			Opcodes:          []Opcode{OpPushConstantInt, OpPOpLoc, OpReturn},
			IntOperands:      []int32{op, 0, 0},
			StringOperands:   []string{"", "", ""},
			InstructionCount: 3,
		}
		state := Init(sf, mp, true, nil, nil)
		state.ActiveLoc = loc
		state.Pointers |= PtrActiveLoc

		err := Execute(state)
		if err == nil {
			t.Errorf("op=%d: expected error, got nil", op)
			continue
		}
		wantPrefix := "P_OPLOC: invalid op"
		if len(err.Error()) < len(wantPrefix) || err.Error()[:len(wantPrefix)] != wantPrefix {
			t.Errorf("op=%d: expected error starting with %q, got %v", op, wantPrefix, err)
		}
	}
}

// TestPOpNpcAnchorsOnActiveNpc — happy path for P_OPNPC.
func TestPOpNpcAnchorsOnActiveNpc(t *testing.T) {
	mp := &mockPlayer{}
	npc := &mockActiveNpc{typeId: 7}

	sf := &ScriptFile{
		Name:             "p_op_npc",
		Opcodes:          []Opcode{OpPushConstantInt, OpPOpNpc, OpReturn},
		IntOperands:      []int32{2, 0, 0},
		StringOperands:   []string{"", "", ""},
		InstructionCount: 3,
	}
	state := Init(sf, mp, true, nil, nil)
	state.ActiveNpc = npc
	state.Pointers |= PtrActiveNpc
	if err := Execute(state); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if len(mp.lastSetInteractionScriptNpc) != 1 {
		t.Fatalf("expected 1 SetInteractionScriptNpc call, got %d", len(mp.lastSetInteractionScriptNpc))
	}
	got := mp.lastSetInteractionScriptNpc[0]
	if got.Npc != npc || got.Op != 2 {
		t.Errorf("args: got %+v, want {Npc:%p, Op:2}", got, npc)
	}
}

// TestPOpNpcInvalidOpErrors — op out of range.
func TestPOpNpcInvalidOpErrors(t *testing.T) {
	for _, op := range []int32{0, 6} {
		mp := &mockPlayer{}
		npc := &mockActiveNpc{typeId: 7}

		sf := &ScriptFile{
			Name:             "p_op_npc_invalid",
			Opcodes:          []Opcode{OpPushConstantInt, OpPOpNpc, OpReturn},
			IntOperands:      []int32{op, 0, 0},
			StringOperands:   []string{"", "", ""},
			InstructionCount: 3,
		}
		state := Init(sf, mp, true, nil, nil)
		state.ActiveNpc = npc
		state.Pointers |= PtrActiveNpc

		err := Execute(state)
		if err == nil {
			t.Errorf("op=%d: expected error, got nil", op)
		}
	}
}

// TestPOpLocUnprotectedRejected verifies that a script started without
// protection (protect=false) gets an error from P_OPLOC. Matches TS
// checkedHandler(ProtectedActivePlayer, ...) semantics. Closes S6v-D1.
func TestPOpLocUnprotectedRejected(t *testing.T) {
	mp := &mockPlayer{}
	loc := &mockActiveLoc{locType: 42}
	sf := newSingleOp("p_op_loc_unprotected", OpPOpLoc)
	state := Init(sf, mp, false, nil, nil) // protect=false
	state.ActiveLoc = loc
	state.Pointers |= PtrActiveLoc
	state.PushInt(3)

	err := Execute(state)
	if err == nil || err.Error() != "P_OPLOC: script not protected" {
		t.Errorf("expected 'P_OPLOC: script not protected', got %v", err)
	}
}

// TestPOpNpcUnprotectedRejected — symmetric.
func TestPOpNpcUnprotectedRejected(t *testing.T) {
	mp := &mockPlayer{}
	npc := &mockActiveNpc{typeId: 7}
	sf := newSingleOp("p_op_npc_unprotected", OpPOpNpc)
	state := Init(sf, mp, false, nil, nil) // protect=false
	state.ActiveNpc = npc
	state.Pointers |= PtrActiveNpc
	state.PushInt(2)

	err := Execute(state)
	if err == nil || err.Error() != "P_OPNPC: script not protected" {
		t.Errorf("expected 'P_OPNPC: script not protected', got %v", err)
	}
}

// TestPTeleportUnprotectedRejected verifies that a script started without
// protection gets the "script not protected" error. Closes S6l-D3 for
// P_TELEPORT (matches TS checkedHandler(ProtectedActivePlayer, ...)).
func TestPTeleportUnprotectedRejected(t *testing.T) {
	mp := &mockPlayer{}
	sf := newSingleOp("p_teleport_unprotected", OpPTeleport)
	state := Init(sf, mp, false, nil, nil) // protect=false
	state.PushInt(123)

	err := Execute(state)
	if err == nil || err.Error() != "P_TELEPORT: script not protected" {
		t.Errorf("expected 'P_TELEPORT: script not protected', got %v", err)
	}
}

// TestPTeleJumpUnprotectedRejected verifies that a script started without
// protection gets the "script not protected" error. Closes S6l-D3 for
// P_TELEJUMP (matches TS checkedHandler(ProtectedActivePlayer, ...)).
func TestPTeleJumpUnprotectedRejected(t *testing.T) {
	mp := &mockPlayer{}
	sf := newSingleOp("p_telejump_unprotected", OpPTeleJump)
	state := Init(sf, mp, false, nil, nil) // protect=false
	state.PushInt(123)

	err := Execute(state)
	if err == nil || err.Error() != "P_TELEJUMP: script not protected" {
		t.Errorf("expected 'P_TELEJUMP: script not protected', got %v", err)
	}
}

// TestPApRangeUnprotectedRejected verifies that a script started without
// protection gets the "script not protected" error. Closes S6l-D3 for
// P_APRANGE (matches TS checkedHandler(ProtectedActivePlayer, ...)).
func TestPApRangeUnprotectedRejected(t *testing.T) {
	mp := &mockPlayer{}
	sf := newSingleOp("p_aprange_unprotected", OpPApRange)
	state := Init(sf, mp, false, nil, nil) // protect=false
	state.PushInt(5)

	err := Execute(state)
	if err == nil || err.Error() != "P_APRANGE: script not protected" {
		t.Errorf("expected 'P_APRANGE: script not protected', got %v", err)
	}
}

// TestPStopActionUnprotectedRejected verifies that a script started without
// protection gets the "script not protected" error. Closes S6l-D3 for
// P_STOPACTION (matches TS checkedHandler(ProtectedActivePlayer, ...)).
func TestPStopActionUnprotectedRejected(t *testing.T) {
	mp := &mockPlayer{}
	sf := newSingleOp("p_stopaction_unprotected", OpPStopAction)
	state := Init(sf, mp, false, nil, nil) // protect=false

	err := Execute(state)
	if err == nil || err.Error() != "P_STOPACTION: script not protected" {
		t.Errorf("expected 'P_STOPACTION: script not protected', got %v", err)
	}
}

// TestPClearPendingActionUnprotectedRejected verifies that a script started
// without protection gets the "script not protected" error. Closes S6l-D3
// for P_CLEARPENDINGACTION (matches TS checkedHandler(ProtectedActivePlayer, ...)).
func TestPClearPendingActionUnprotectedRejected(t *testing.T) {
	mp := &mockPlayer{}
	sf := newSingleOp("p_clearpendingaction_unprotected", OpPClearPendingAction)
	state := Init(sf, mp, false, nil, nil) // protect=false

	err := Execute(state)
	if err == nil || err.Error() != "P_CLEARPENDINGACTION: script not protected" {
		t.Errorf("expected 'P_CLEARPENDINGACTION: script not protected', got %v", err)
	}
}

// -- S7a FINDUID tests ---------------------------------------------------

// zoneKey indexes the byZone fixture below by (level, zoneX, zoneZ),
// matching the production-side ZonePlayers parameter shape (world coords,
// not zone indices). NAI-35-T2.
type zoneKey struct{ level, zoneX, zoneZ int }

// mockPlayerLookup resolves UIDs via a pre-seeded map. Introduced in S7a.
// NAI-35-T2 extends with byZone for the new ZonePlayers method.
type mockPlayerLookup struct {
	byUID  map[int]ActivePlayer
	byZone map[zoneKey][]ActivePlayer
	calls  int
}

func (m *mockPlayerLookup) LookupPlayerByUID(uid int) ActivePlayer {
	m.calls++
	return m.byUID[uid]
}

// ZonePlayers satisfies the NAI-35-T2 PlayerLookup.ZonePlayers extension.
// Returns the slice keyed by (level, zoneX, zoneZ); nil/zero-value if
// unseeded. Mirrors the production semantics of "empty/nil slice on miss".
func (m *mockPlayerLookup) ZonePlayers(level, zoneX, zoneZ int) []ActivePlayer {
	return m.byZone[zoneKey{level, zoneX, zoneZ}]
}

// TestFindUIDFound: lookup returns a target → push 1, Self rebinds,
// PtrActivePlayer set, Protect stays false (FINDUID is unprotected).
func TestFindUIDFound(t *testing.T) {
	target := &mockPlayer{username: "Target", uidValue: 99}
	origSelf := &mockPlayer{username: "Orig", uidValue: 1}
	lookup := &mockPlayerLookup{byUID: map[int]ActivePlayer{99: target}}

	sf := newSingleOp("finduid_found", OpFindUID)
	state := Init(sf, origSelf, false, nil, nil)
	state.PlayerLookup = lookup
	state.PushInt(99)

	if err := Execute(state); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if state.ISP != 1 || state.IntStack[0] != 1 {
		t.Errorf("stack: got [%v], want [1]", state.IntStack[:state.ISP])
	}
	if state.Self != target {
		t.Errorf("Self: got %v, want target", state.Self)
	}
	if state.Pointers&PtrActivePlayer == 0 {
		t.Errorf("PtrActivePlayer should be set, pointers=%b", state.Pointers)
	}
	if state.Protect {
		t.Errorf("Protect should remain false for FINDUID")
	}
}

// TestFindUIDNotFound: lookup returns nil → push 0, Self unchanged.
func TestFindUIDNotFound(t *testing.T) {
	origSelf := &mockPlayer{username: "Orig", uidValue: 1}
	lookup := &mockPlayerLookup{byUID: map[int]ActivePlayer{}}

	sf := newSingleOp("finduid_notfound", OpFindUID)
	state := Init(sf, origSelf, false, nil, nil)
	state.PlayerLookup = lookup
	state.PushInt(999)

	if err := Execute(state); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if state.ISP != 1 || state.IntStack[0] != 0 {
		t.Errorf("stack: got [%v], want [0]", state.IntStack[:state.ISP])
	}
	if state.Self != origSelf {
		t.Errorf("Self should be unchanged, got %v", state.Self)
	}
}

// TestFindUIDNoLookupConfigured: PlayerLookup nil → push 0.
// Host configurations that don't wire a lookup degrade to "not found"
// rather than erroring, matching the LAST_INT / LAST_COM precedent.
func TestFindUIDNoLookupConfigured(t *testing.T) {
	origSelf := &mockPlayer{username: "Orig"}

	sf := newSingleOp("finduid_nolookup", OpFindUID)
	state := Init(sf, origSelf, false, nil, nil)
	// state.PlayerLookup left nil
	state.PushInt(1)

	if err := Execute(state); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if state.ISP != 1 || state.IntStack[0] != 0 {
		t.Errorf("stack: got [%v], want [0]", state.IntStack[:state.ISP])
	}
	if state.Self != origSelf {
		t.Errorf("Self should be unchanged")
	}
}

// TestPFindUIDSelfReacquire: script already runs protected on the target
// uid → push 1 with no state mutation, no lookup call (fast-path).
// Mirrors TS PlayerOps.ts:79-83.
func TestPFindUIDSelfReacquire(t *testing.T) {
	self := &mockPlayer{username: "Self", uidValue: 42}
	lookup := &mockPlayerLookup{byUID: map[int]ActivePlayer{}}

	sf := newSingleOp("pfinduid_self", OpPFindUID)
	state := Init(sf, self, true, nil, nil) // protect=true
	state.PlayerLookup = lookup
	state.PushInt(42)

	if err := Execute(state); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if state.ISP != 1 || state.IntStack[0] != 1 {
		t.Errorf("stack: got [%v], want [1]", state.IntStack[:state.ISP])
	}
	if state.Self != self {
		t.Errorf("Self should be unchanged on self-reacquire")
	}
	if lookup.calls != 0 {
		t.Errorf("fast-path should skip lookup, calls=%d", lookup.calls)
	}
	if !state.Protect {
		t.Errorf("Protect should remain true")
	}
}

// TestPFindUIDFoundCanAccess: target is reachable and CanAccess=true →
// push 1, Self rebinds, Protect=true, PtrActivePlayer set.
func TestPFindUIDFoundCanAccess(t *testing.T) {
	target := &mockPlayer{username: "Target", uidValue: 99, canAccessValue: true}
	origSelf := &mockPlayer{username: "Orig", uidValue: 1}
	lookup := &mockPlayerLookup{byUID: map[int]ActivePlayer{99: target}}

	sf := newSingleOp("pfinduid_ok", OpPFindUID)
	state := Init(sf, origSelf, false, nil, nil) // protect=false initially
	state.PlayerLookup = lookup
	state.PushInt(99)

	if err := Execute(state); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if state.ISP != 1 || state.IntStack[0] != 1 {
		t.Errorf("stack: got [%v], want [1]", state.IntStack[:state.ISP])
	}
	if state.Self != target {
		t.Errorf("Self: got %v, want target", state.Self)
	}
	if state.Pointers&PtrActivePlayer == 0 {
		t.Errorf("PtrActivePlayer should be set")
	}
	if !state.Protect {
		t.Errorf("Protect should be true after successful P_FINDUID")
	}
}

// TestPFindUIDFoundCannotAccess: target exists but CanAccess=false →
// push 0, Self unchanged, Protect unchanged.
func TestPFindUIDFoundCannotAccess(t *testing.T) {
	target := &mockPlayer{username: "Target", uidValue: 99, canAccessValue: false}
	origSelf := &mockPlayer{username: "Orig", uidValue: 1}
	lookup := &mockPlayerLookup{byUID: map[int]ActivePlayer{99: target}}

	sf := newSingleOp("pfinduid_busy", OpPFindUID)
	state := Init(sf, origSelf, false, nil, nil)
	state.PlayerLookup = lookup
	state.PushInt(99)

	if err := Execute(state); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if state.ISP != 1 || state.IntStack[0] != 0 {
		t.Errorf("stack: got [%v], want [0]", state.IntStack[:state.ISP])
	}
	if state.Self != origSelf {
		t.Errorf("Self should be unchanged when CanAccess=false")
	}
	if state.Protect {
		t.Errorf("Protect should remain false")
	}
}

// TestPFindUIDNotFound: lookup returns nil → push 0, Self unchanged.
func TestPFindUIDNotFound(t *testing.T) {
	origSelf := &mockPlayer{username: "Orig", uidValue: 1}
	lookup := &mockPlayerLookup{byUID: map[int]ActivePlayer{}}

	sf := newSingleOp("pfinduid_notfound", OpPFindUID)
	state := Init(sf, origSelf, false, nil, nil)
	state.PlayerLookup = lookup
	state.PushInt(999)

	if err := Execute(state); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if state.ISP != 1 || state.IntStack[0] != 0 {
		t.Errorf("stack: got [%v], want [0]", state.IntStack[:state.ISP])
	}
	if state.Self != origSelf {
		t.Errorf("Self should be unchanged")
	}
}

// -- S7b: checkNotNull + handlePAnimProtect tests -------------------------

// TestCheckNotNull validates the shared NumberNotNull helper.
// Mirrors TS ScriptValidators.ts:36-41.
func TestCheckNotNull(t *testing.T) {
	cases := []struct {
		name    string
		v       int
		wantErr bool
	}{
		{"null sentinel", -1, true},
		{"zero", 0, false},
		{"positive", 1, false},
		{"min int32", math.MinInt32, false},
		{"max int32", math.MaxInt32, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := checkNotNull(tc.v, "OP")
			if tc.wantErr {
				if err == nil {
					t.Fatalf("checkNotNull(%d): want error, got nil", tc.v)
				}
				if !strings.Contains(err.Error(), "OP: input number was null(-1)") {
					t.Errorf("error message: got %q, want contains %q", err.Error(), "OP: input number was null(-1)")
				}
			} else {
				if err != nil {
					t.Fatalf("checkNotNull(%d): want nil, got %v", tc.v, err)
				}
			}
		})
	}
}

// TestPAnimProtectHappyPathZero — protect=true, push 0 → no error,
// animProtectValue set to 0.
func TestPAnimProtectHappyPathZero(t *testing.T) {
	player := &mockPlayer{animProtectValue: -2} // sentinel
	sf := newSingleOp("panimprotect_zero", OpPAnimProtect)
	state := Init(sf, player, true, nil, nil)
	state.PushInt(0)

	if err := Execute(state); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if player.animProtectValue != 0 {
		t.Errorf("animProtectValue: got %d, want 0", player.animProtectValue)
	}
	if state.Self != player {
		t.Errorf("Self should be unchanged")
	}
}

// TestPAnimProtectHappyPathNonzero — protect=true, push 1 → no error,
// animProtectValue set to 1.
func TestPAnimProtectHappyPathNonzero(t *testing.T) {
	player := &mockPlayer{animProtectValue: -2} // sentinel
	sf := newSingleOp("panimprotect_nonzero", OpPAnimProtect)
	state := Init(sf, player, true, nil, nil)
	state.PushInt(1)

	if err := Execute(state); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if player.animProtectValue != 1 {
		t.Errorf("animProtectValue: got %d, want 1", player.animProtectValue)
	}
}

// TestPAnimProtectNullRejected — protect=true, push -1 → error containing
// "P_ANIMPROTECT: input number was null(-1)"; animProtectValue unchanged.
func TestPAnimProtectNullRejected(t *testing.T) {
	player := &mockPlayer{animProtectValue: -2} // sentinel
	sf := newSingleOp("panimprotect_null", OpPAnimProtect)
	state := Init(sf, player, true, nil, nil)
	state.PushInt(-1)

	err := Execute(state)
	if err == nil {
		t.Fatalf("Execute: want error, got nil")
	}
	want := "P_ANIMPROTECT: input number was null(-1)"
	if !strings.Contains(err.Error(), want) {
		t.Errorf("error: got %q, want contains %q", err.Error(), want)
	}
	if player.animProtectValue != -2 {
		t.Errorf("animProtectValue should be unchanged sentinel -2, got %d", player.animProtectValue)
	}
}

// TestPAnimProtectNotProtected — protect=false, push 0 → error containing
// "P_ANIMPROTECT: script not protected"; animProtectValue unchanged.
func TestPAnimProtectNotProtected(t *testing.T) {
	player := &mockPlayer{animProtectValue: -2} // sentinel
	sf := newSingleOp("panimprotect_notprotected", OpPAnimProtect)
	state := Init(sf, player, false, nil, nil) // protect=false
	state.PushInt(0)

	err := Execute(state)
	if err == nil {
		t.Fatalf("Execute: want error, got nil")
	}
	want := "P_ANIMPROTECT: script not protected"
	if !strings.Contains(err.Error(), want) {
		t.Errorf("error: got %q, want contains %q", err.Error(), want)
	}
	if player.animProtectValue != -2 {
		t.Errorf("animProtectValue should be unchanged sentinel -2, got %d", player.animProtectValue)
	}
}

// TestPAnimProtectNoActivePlayer — Self=nil → error from requireActivePlayer
// chain containing "P_ANIMPROTECT"; animProtectValue unchanged.
func TestPAnimProtectNoActivePlayer(t *testing.T) {
	player := &mockPlayer{animProtectValue: -2} // sentinel (not wired into state)
	sf := newSingleOp("panimprotect_noactive", OpPAnimProtect)
	state := Init(sf, nil, true, nil, nil) // Self=nil
	state.PushInt(0)

	err := Execute(state)
	if err == nil {
		t.Fatalf("Execute: want error, got nil")
	}
	if !strings.Contains(err.Error(), "P_ANIMPROTECT") {
		t.Errorf("error: got %q, want contains %q", err.Error(), "P_ANIMPROTECT")
	}
	if player.animProtectValue != -2 {
		t.Errorf("animProtectValue should be unchanged sentinel -2, got %d", player.animProtectValue)
	}
}

// -- S7c: checkInvType + handleBuildAppearance tests ----------------------

// TestCheckInvType validates the state-aware InvType validator.
// Mirrors TS InvTypeValid (ScriptValidators.ts:122). Both the range check
// and the registry-present check collapse into a single Configs.InvType
// lookup per the Configs interface contract.
func TestCheckInvType(t *testing.T) {
	tests := []struct {
		name      string
		id        int
		setup     func() *mockConfigs
		wantErr   bool
		wantSubst string
	}{
		{
			name:    "valid id",
			id:      5,
			setup:   func() *mockConfigs { return &mockConfigs{invs: map[int]*objtype.InvType{5: {}}} },
			wantErr: false,
		},
		{
			name:      "unknown id",
			id:        100,
			setup:     func() *mockConfigs { return &mockConfigs{invs: map[int]*objtype.InvType{}} },
			wantErr:   true,
			wantSubst: "OP: no InvType with value (100) found",
		},
		{
			name:      "negative id",
			id:        -1,
			setup:     func() *mockConfigs { return &mockConfigs{invs: map[int]*objtype.InvType{}} },
			wantErr:   true,
			wantSubst: "OP: no InvType with value (-1) found",
		},
		{
			name:      "nil Configs",
			id:        0,
			setup:     func() *mockConfigs { return nil },
			wantErr:   true,
			wantSubst: "OP: no InvType with value (0) found",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			s := &ScriptState{}
			if cfg := tc.setup(); cfg != nil {
				s.Configs = cfg
			}
			err := checkInvType(s, tc.id, "OP")
			if tc.wantErr {
				if err == nil {
					t.Fatalf("checkInvType(%d): want error, got nil", tc.id)
				}
				if !strings.Contains(err.Error(), tc.wantSubst) {
					t.Errorf("error message: got %q, want contains %q", err.Error(), tc.wantSubst)
				}
			} else {
				if err != nil {
					t.Fatalf("checkInvType(%d): want nil, got %v", tc.id, err)
				}
			}
		})
	}
}

// TestBuildAppearanceHappyPath — Self != nil, Configs.invs has id=5,
// push 5 → no error; lastAppearanceInv == 5, appearanceInvCalls == 1,
// appearanceMaskSet == true.
func TestBuildAppearanceHappyPath(t *testing.T) {
	player := &mockPlayer{}
	sf := newSingleOp("buildappearance_happy", OpBuildAppearance)
	state := Init(sf, player, false, nil, nil)
	state.Configs = &mockConfigs{invs: map[int]*objtype.InvType{5: {}}}
	state.PushInt(5)

	if err := Execute(state); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if player.lastAppearanceInv != 5 {
		t.Errorf("lastAppearanceInv: got %d, want 5", player.lastAppearanceInv)
	}
	if player.appearanceInvCalls != 1 {
		t.Errorf("appearanceInvCalls: got %d, want 1", player.appearanceInvCalls)
	}
	if !player.appearanceMaskSet {
		t.Errorf("appearanceMaskSet: got false, want true")
	}
}

// TestBuildAppearanceInvalidInvRejected — Self != nil, Configs.invs empty,
// push 999 → error message contains "BUILDAPPEARANCE: no InvType with
// value (999) found"; appearanceInvCalls == 0, appearanceMaskSet == false.
func TestBuildAppearanceInvalidInvRejected(t *testing.T) {
	player := &mockPlayer{}
	sf := newSingleOp("buildappearance_invalid", OpBuildAppearance)
	state := Init(sf, player, false, nil, nil)
	state.Configs = &mockConfigs{invs: map[int]*objtype.InvType{}}
	state.PushInt(999)

	err := Execute(state)
	if err == nil {
		t.Fatalf("Execute: want error, got nil")
	}
	want := "BUILDAPPEARANCE: no InvType with value (999) found"
	if !strings.Contains(err.Error(), want) {
		t.Errorf("error: got %q, want contains %q", err.Error(), want)
	}
	if player.appearanceInvCalls != 0 {
		t.Errorf("appearanceInvCalls: got %d, want 0", player.appearanceInvCalls)
	}
	if player.appearanceMaskSet {
		t.Errorf("appearanceMaskSet: got true, want false")
	}
}

// TestBuildAppearanceNegativeIdRejected — Self != nil, Configs.invs empty,
// push -1 → error; setter not called. Covers the TS `input >= 0` branch
// via nil lookup since goscape collapses both checks.
func TestBuildAppearanceNegativeIdRejected(t *testing.T) {
	player := &mockPlayer{}
	sf := newSingleOp("buildappearance_negative", OpBuildAppearance)
	state := Init(sf, player, false, nil, nil)
	state.Configs = &mockConfigs{invs: map[int]*objtype.InvType{}}
	state.PushInt(-1)

	err := Execute(state)
	if err == nil {
		t.Fatalf("Execute: want error, got nil")
	}
	want := "BUILDAPPEARANCE: no InvType with value (-1) found"
	if !strings.Contains(err.Error(), want) {
		t.Errorf("error: got %q, want contains %q", err.Error(), want)
	}
	if player.appearanceInvCalls != 0 {
		t.Errorf("appearanceInvCalls: got %d, want 0", player.appearanceInvCalls)
	}
	if player.appearanceMaskSet {
		t.Errorf("appearanceMaskSet: got true, want false")
	}
}

// TestBuildAppearanceNoActivePlayer — Self=nil → error from
// requireActivePlayer chain containing "BUILDAPPEARANCE". The gate runs
// before PopInt so the int stack should retain the pushed value.
func TestBuildAppearanceNoActivePlayer(t *testing.T) {
	player := &mockPlayer{}
	sf := newSingleOp("buildappearance_noactive", OpBuildAppearance)
	state := Init(sf, nil, false, nil, nil) // Self=nil
	state.Configs = &mockConfigs{invs: map[int]*objtype.InvType{5: {}}}
	state.PushInt(5)

	err := Execute(state)
	if err == nil {
		t.Fatalf("Execute: want error, got nil")
	}
	if !strings.Contains(err.Error(), "BUILDAPPEARANCE") {
		t.Errorf("error: got %q, want contains %q", err.Error(), "BUILDAPPEARANCE")
	}
	if player.appearanceInvCalls != 0 {
		t.Errorf("appearanceInvCalls: got %d, want 0", player.appearanceInvCalls)
	}
	// Gate runs before PopInt — the pushed value should still be on the stack.
	if got := state.PopInt(); got != 5 {
		t.Errorf("int stack top: got %d, want 5 (gate should run before PopInt)", got)
	}
}

// TestBuildAppearanceNotProtectedOK — Protect=false, Self != nil,
// Configs.invs has id=3, push 3 → no error. BUILDAPPEARANCE uses
// ActivePlayer (not ProtectedActivePlayer). Gate-regression guard:
// catches a future edit that copy-pastes requireProtectedActivePlayer.
func TestBuildAppearanceNotProtectedOK(t *testing.T) {
	player := &mockPlayer{}
	sf := newSingleOp("buildappearance_unprotected", OpBuildAppearance)
	state := Init(sf, player, false, nil, nil) // protect=false
	state.Configs = &mockConfigs{invs: map[int]*objtype.InvType{3: {}}}
	state.PushInt(3)

	if err := Execute(state); err != nil {
		t.Fatalf("Execute: unexpected error %v (BUILDAPPEARANCE should not require Protect)", err)
	}
	if player.lastAppearanceInv != 3 {
		t.Errorf("lastAppearanceInv: got %d, want 3", player.lastAppearanceInv)
	}
	if player.appearanceInvCalls != 1 {
		t.Errorf("appearanceInvCalls: got %d, want 1", player.appearanceInvCalls)
	}
	if !player.appearanceMaskSet {
		t.Errorf("appearanceMaskSet: got false, want true")
	}
}

// -- S7e: handleAllowDesign tests ------------------------------------------

// TestAllowDesign is a table-driven test covering the three value-coercion
// paths (5.1 true, 5.2 false, 5.3 non-one coerces to false). All three
// exercise the happy path: ActivePlayer set, valid int (not -1), setter
// called exactly once. Pins the exact v==1 coercion shape — a truthy
// v!=0 mistake would fail the 5.3 sub-case.
func TestAllowDesign(t *testing.T) {
	cases := []struct {
		name    string
		push    int
		wantVal bool
	}{
		{"True", 1, true},
		{"False", 0, false},
		{"NonOneCoercesToFalse_2", 2, false},
		{"NonOneCoercesToFalse_neg2", -2, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			player := &mockPlayer{}
			sf := newSingleOp("allowdesign_"+tc.name, OpAllowDesign)
			state := Init(sf, player, false, nil, nil)
			state.PushInt(tc.push)

			if err := Execute(state); err != nil {
				t.Fatalf("Execute: unexpected error %v", err)
			}
			if player.allowDesignValue != tc.wantVal {
				t.Errorf("allowDesignValue: got %v, want %v", player.allowDesignValue, tc.wantVal)
			}
			if player.allowDesignCalls != 1 {
				t.Errorf("allowDesignCalls: got %d, want 1", player.allowDesignCalls)
			}
		})
	}
}

// TestAllowDesignNullInput — push -1 → checkNotNull rejects with
// "input number was null(-1)". Setter must NOT be called (S7e §5.4).
func TestAllowDesignNullInput(t *testing.T) {
	player := &mockPlayer{}
	sf := newSingleOp("allowdesign_null", OpAllowDesign)
	state := Init(sf, player, false, nil, nil)
	state.PushInt(-1)

	err := Execute(state)
	if err == nil {
		t.Fatal("Execute: want error for null input, got nil")
	}
	if !strings.Contains(err.Error(), "input number was null(-1)") {
		t.Errorf("error: got %q, want contains %q", err.Error(), "input number was null(-1)")
	}
	if player.allowDesignCalls != 0 {
		t.Errorf("allowDesignCalls: got %d, want 0 (setter must not be called on validator failure)", player.allowDesignCalls)
	}
}

// TestAllowDesignRequiresActivePlayer — Self=nil → error from
// requireActivePlayer containing "no active player". Setter must NOT
// be called (S7e §5.5). Gate is ActivePlayer (not Protected) — mirrors
// TestBuildAppearanceNoActivePlayer structure.
func TestAllowDesignRequiresActivePlayer(t *testing.T) {
	player := &mockPlayer{}
	sf := newSingleOp("allowdesign_noactive", OpAllowDesign)
	state := Init(sf, nil, false, nil, nil) // Self=nil
	state.PushInt(1)

	err := Execute(state)
	if err == nil {
		t.Fatal("Execute: want error for missing active player, got nil")
	}
	if !strings.Contains(err.Error(), "no active player") {
		t.Errorf("error: got %q, want contains %q", err.Error(), "no active player")
	}
	if player.allowDesignCalls != 0 {
		t.Errorf("allowDesignCalls: got %d, want 0 (setter must not be called when gate fails)", player.allowDesignCalls)
	}
}

func TestCheckStringNotNullEmpty(t *testing.T) {
	err := checkStringNotNull("", "MIDI_SONG")
	if err == nil {
		t.Fatal("empty string: want error, got nil")
	}
	if !strings.Contains(err.Error(), "MIDI_SONG: input string was null") {
		t.Errorf("error message %q does not contain %q", err.Error(), "MIDI_SONG: input string was null")
	}
}

func TestCheckStringNotNullNonEmpty(t *testing.T) {
	if err := checkStringNotNull("harmony1", "MIDI_SONG"); err != nil {
		t.Errorf("non-empty string: want nil, got %v", err)
	}
}

func TestMidiSongHappyPath(t *testing.T) {
	s := &ScriptState{
		IntStack:    make([]int, StackCapacity),
		StringStack: make([]string, StackCapacity),
		Self:        &mockPlayer{},
		Pointers:    PtrActivePlayer,
	}
	s.PushString("harmony1")
	mp := s.Self.(*mockPlayer)

	if err := handleMidiSong(s); err != nil {
		t.Fatalf("handleMidiSong: %v", err)
	}
	if len(mp.playSongCalls) != 1 {
		t.Fatalf("playSongCalls: got %d, want 1", len(mp.playSongCalls))
	}
	if mp.playSongCalls[0].name != "harmony1" {
		t.Errorf("playSongCalls[0].name: got %q, want %q", mp.playSongCalls[0].name, "harmony1")
	}
}

func TestMidiSongLowMemoryBails(t *testing.T) {
	s := &ScriptState{
		IntStack:    make([]int, StackCapacity),
		StringStack: make([]string, StackCapacity),
		Self:        &mockPlayer{lowMemoryValue: true},
		Pointers:    PtrActivePlayer,
	}
	s.PushString("harmony1")
	mp := s.Self.(*mockPlayer)

	if err := handleMidiSong(s); err != nil {
		t.Fatalf("handleMidiSong: %v", err)
	}
	if len(mp.playSongCalls) != 0 {
		t.Errorf("lowMemory=true: playSongCalls=%d, want 0", len(mp.playSongCalls))
	}
}

func TestMidiSongNullStringRejects(t *testing.T) {
	s := &ScriptState{
		IntStack:    make([]int, StackCapacity),
		StringStack: make([]string, StackCapacity),
		Self:        &mockPlayer{},
		Pointers:    PtrActivePlayer,
	}
	s.PushString("")

	err := handleMidiSong(s)
	if err == nil {
		t.Fatal("empty name: want error, got nil")
	}
	if !strings.Contains(err.Error(), "MIDI_SONG: input string was null") {
		t.Errorf("error %q does not contain %q", err.Error(), "MIDI_SONG: input string was null")
	}
}

func TestMidiSongNoActivePlayerRejects(t *testing.T) {
	s := &ScriptState{
		IntStack:    make([]int, StackCapacity),
		StringStack: make([]string, StackCapacity),
		Self:        nil,
		Pointers:    0, // PtrActivePlayer unset
	}
	s.PushString("harmony1")

	err := handleMidiSong(s)
	if err == nil {
		t.Fatal("no active player: want error, got nil")
	}
	if !strings.Contains(err.Error(), "MIDI_SONG: no active player") {
		t.Errorf("error %q does not contain %q", err.Error(), "MIDI_SONG: no active player")
	}
}

func TestMidiJingleHappyPath(t *testing.T) {
	s := &ScriptState{
		IntStack:    make([]int, StackCapacity),
		StringStack: make([]string, StackCapacity),
		Self:        &mockPlayer{},
		Pointers:    PtrActivePlayer,
	}
	// Pop order in handler: delay first (top-of-stack), then name.
	// Push order: name (deepest), delay (topmost).
	s.PushString("fanfare")
	s.PushInt(3)
	mp := s.Self.(*mockPlayer)

	if err := handleMidiJingle(s); err != nil {
		t.Fatalf("handleMidiJingle: %v", err)
	}
	if len(mp.playJingleCalls) != 1 {
		t.Fatalf("playJingleCalls: got %d, want 1", len(mp.playJingleCalls))
	}
	if mp.playJingleCalls[0].delay != 3 || mp.playJingleCalls[0].name != "fanfare" {
		t.Errorf("playJingleCalls[0]: got {delay:%d, name:%q}, want {delay:3, name:\"fanfare\"}",
			mp.playJingleCalls[0].delay, mp.playJingleCalls[0].name)
	}
}

func TestMidiJingleLowMemoryBails(t *testing.T) {
	s := &ScriptState{
		IntStack:    make([]int, StackCapacity),
		StringStack: make([]string, StackCapacity),
		Self:        &mockPlayer{lowMemoryValue: true},
		Pointers:    PtrActivePlayer,
	}
	s.PushString("fanfare")
	s.PushInt(3)
	mp := s.Self.(*mockPlayer)

	if err := handleMidiJingle(s); err != nil {
		t.Fatalf("handleMidiJingle: %v", err)
	}
	if len(mp.playJingleCalls) != 0 {
		t.Errorf("lowMemory=true: playJingleCalls=%d, want 0", len(mp.playJingleCalls))
	}
}

func TestMidiJingleNullStringRejects(t *testing.T) {
	s := &ScriptState{
		IntStack:    make([]int, StackCapacity),
		StringStack: make([]string, StackCapacity),
		Self:        &mockPlayer{},
		Pointers:    PtrActivePlayer,
	}
	s.PushString("")
	s.PushInt(3)

	err := handleMidiJingle(s)
	if err == nil {
		t.Fatal("empty name: want error, got nil")
	}
	if !strings.Contains(err.Error(), "MIDI_JINGLE: input string was null") {
		t.Errorf("error %q does not contain %q", err.Error(), "MIDI_JINGLE: input string was null")
	}
}

func TestMidiJingleNullDelayRejects(t *testing.T) {
	s := &ScriptState{
		IntStack:    make([]int, StackCapacity),
		StringStack: make([]string, StackCapacity),
		Self:        &mockPlayer{},
		Pointers:    PtrActivePlayer,
	}
	s.PushString("fanfare")
	s.PushInt(-1)

	err := handleMidiJingle(s)
	if err == nil {
		t.Fatal("delay=-1: want error, got nil")
	}
	if !strings.Contains(err.Error(), "MIDI_JINGLE: input number was null(-1)") {
		t.Errorf("error %q does not contain %q", err.Error(), "MIDI_JINGLE: input number was null(-1)")
	}
}

func TestMidiJingleNoActivePlayerRejects(t *testing.T) {
	s := &ScriptState{
		IntStack:    make([]int, StackCapacity),
		StringStack: make([]string, StackCapacity),
		Self:        nil,
		Pointers:    0,
	}
	s.PushString("fanfare")
	s.PushInt(3)

	err := handleMidiJingle(s)
	if err == nil {
		t.Fatal("no active player: want error, got nil")
	}
	if !strings.Contains(err.Error(), "MIDI_JINGLE: no active player") {
		t.Errorf("error %q does not contain %q", err.Error(), "MIDI_JINGLE: no active player")
	}
}

// -- NAI-24 Bundle 1: NumberNotNull audit null-pin tests -----------------
//
// Each test below corresponds to a popInt site in handlers_player.go where
// the TS counterpart (PlayerOps.ts) wraps with check(..., NumberNotNull).
// A value of -1 must be rejected before any side-effect occurs. Tests
// follow the TestHandle<OpName>NullRejected naming convention from
// handlers_interface_test.go.

// TestHandleStatAddNullRejected pins STAT_ADD: TS wraps both constant and
// percent with NumberNotNull (PlayerOps.ts:505-506). Stat id is wrapped
// with PlayerStatValid (separate gate via checkStatID); only constant and
// percent get the NumberNotNull pin here.
func TestHandleStatAddNullRejected(t *testing.T) {
	tests := []struct {
		name                      string
		statID, constant, percent int32
		wantSubstr                string
	}{
		{
			name:       "null_constant",
			statID:     2,
			constant:   -1,
			percent:    0,
			wantSubstr: "STAT_ADD: input number was null(-1)",
		},
		{
			name:       "null_percent",
			statID:     2,
			constant:   0,
			percent:    -1,
			wantSubstr: "STAT_ADD: input number was null(-1)",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			mp := &mockPlayer{}
			sf := &ScriptFile{
				Name: "stat_add_" + tc.name,
				Opcodes: []Opcode{
					OpPushConstantInt, // stat id (bottom)
					OpPushConstantInt, // constant
					OpPushConstantInt, // percent (top)
					OpStatAdd,
					OpReturn,
				},
				IntOperands: []int32{tc.statID, tc.constant, tc.percent, 0, 0},
			}
			state := Init(sf, mp, false, nil, nil)

			err := Execute(state)
			if err == nil {
				t.Fatalf("Execute: want error for %s, got nil", tc.name)
			}
			if !strings.Contains(err.Error(), tc.wantSubstr) {
				t.Errorf("error: got %q, want substring %q", err.Error(), tc.wantSubstr)
			}
			if len(mp.setCurLevelCalls) != 0 {
				t.Errorf("setCurLevelCalls: should not have been called, got %d", len(mp.setCurLevelCalls))
			}
		})
	}
}

// TestHandleStatSubNullRejected pins STAT_SUB: TS wraps both constant and
// percent with NumberNotNull (PlayerOps.ts:525-526).
func TestHandleStatSubNullRejected(t *testing.T) {
	tests := []struct {
		name                      string
		statID, constant, percent int32
		wantSubstr                string
	}{
		{
			name:       "null_constant",
			statID:     2,
			constant:   -1,
			percent:    0,
			wantSubstr: "STAT_SUB: input number was null(-1)",
		},
		{
			name:       "null_percent",
			statID:     2,
			constant:   0,
			percent:    -1,
			wantSubstr: "STAT_SUB: input number was null(-1)",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			mp := &mockPlayer{}
			sf := &ScriptFile{
				Name: "stat_sub_" + tc.name,
				Opcodes: []Opcode{
					OpPushConstantInt, OpPushConstantInt, OpPushConstantInt,
					OpStatSub, OpReturn,
				},
				IntOperands: []int32{tc.statID, tc.constant, tc.percent, 0, 0},
			}
			state := Init(sf, mp, false, nil, nil)

			err := Execute(state)
			if err == nil {
				t.Fatalf("Execute: want error for %s, got nil", tc.name)
			}
			if !strings.Contains(err.Error(), tc.wantSubstr) {
				t.Errorf("error: got %q, want substring %q", err.Error(), tc.wantSubstr)
			}
			if len(mp.setCurLevelCalls) != 0 {
				t.Errorf("setCurLevelCalls: should not have been called, got %d", len(mp.setCurLevelCalls))
			}
		})
	}
}

// TestHandleStatBoostNullRejected pins STAT_BOOST: TS wraps both constant
// and percent with NumberNotNull (PlayerOps.ts:542-543).
func TestHandleStatBoostNullRejected(t *testing.T) {
	tests := []struct {
		name                      string
		statID, constant, percent int32
		wantSubstr                string
	}{
		{
			name:       "null_constant",
			statID:     2,
			constant:   -1,
			percent:    0,
			wantSubstr: "STAT_BOOST: input number was null(-1)",
		},
		{
			name:       "null_percent",
			statID:     2,
			constant:   0,
			percent:    -1,
			wantSubstr: "STAT_BOOST: input number was null(-1)",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			mp := &mockPlayer{}
			sf := &ScriptFile{
				Name: "stat_boost_" + tc.name,
				Opcodes: []Opcode{
					OpPushConstantInt, OpPushConstantInt, OpPushConstantInt,
					OpStatBoost, OpReturn,
				},
				IntOperands: []int32{tc.statID, tc.constant, tc.percent, 0, 0},
			}
			state := Init(sf, mp, false, nil, nil)

			err := Execute(state)
			if err == nil {
				t.Fatalf("Execute: want error for %s, got nil", tc.name)
			}
			if !strings.Contains(err.Error(), tc.wantSubstr) {
				t.Errorf("error: got %q, want substring %q", err.Error(), tc.wantSubstr)
			}
			if len(mp.setCurLevelCalls) != 0 {
				t.Errorf("setCurLevelCalls: should not have been called, got %d", len(mp.setCurLevelCalls))
			}
		})
	}
}

// TestHandleStatDrainNullRejected pins STAT_DRAIN: TS wraps both constant
// and percent with NumberNotNull (PlayerOps.ts:565-566).
func TestHandleStatDrainNullRejected(t *testing.T) {
	tests := []struct {
		name                      string
		statID, constant, percent int32
		wantSubstr                string
	}{
		{
			name:       "null_constant",
			statID:     2,
			constant:   -1,
			percent:    0,
			wantSubstr: "STAT_DRAIN: input number was null(-1)",
		},
		{
			name:       "null_percent",
			statID:     2,
			constant:   0,
			percent:    -1,
			wantSubstr: "STAT_DRAIN: input number was null(-1)",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			mp := &mockPlayer{}
			sf := &ScriptFile{
				Name: "stat_drain_" + tc.name,
				Opcodes: []Opcode{
					OpPushConstantInt, OpPushConstantInt, OpPushConstantInt,
					OpStatDrain, OpReturn,
				},
				IntOperands: []int32{tc.statID, tc.constant, tc.percent, 0, 0},
			}
			state := Init(sf, mp, false, nil, nil)

			err := Execute(state)
			if err == nil {
				t.Fatalf("Execute: want error for %s, got nil", tc.name)
			}
			if !strings.Contains(err.Error(), tc.wantSubstr) {
				t.Errorf("error: got %q, want substring %q", err.Error(), tc.wantSubstr)
			}
			if len(mp.setCurLevelCalls) != 0 {
				t.Errorf("setCurLevelCalls: should not have been called, got %d", len(mp.setCurLevelCalls))
			}
		})
	}
}

// TestHandleStatHealNullRejected pins STAT_HEAL: TS wraps both constant
// and percent with NumberNotNull (PlayerOps.ts:600-601).
func TestHandleStatHealNullRejected(t *testing.T) {
	tests := []struct {
		name                      string
		statID, constant, percent int32
		wantSubstr                string
	}{
		{
			name:       "null_constant",
			statID:     2,
			constant:   -1,
			percent:    0,
			wantSubstr: "STAT_HEAL: input number was null(-1)",
		},
		{
			name:       "null_percent",
			statID:     2,
			constant:   0,
			percent:    -1,
			wantSubstr: "STAT_HEAL: input number was null(-1)",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			mp := &mockPlayer{}
			sf := &ScriptFile{
				Name: "stat_heal_" + tc.name,
				Opcodes: []Opcode{
					OpPushConstantInt, OpPushConstantInt, OpPushConstantInt,
					OpStatHeal, OpReturn,
				},
				IntOperands: []int32{tc.statID, tc.constant, tc.percent, 0, 0},
			}
			state := Init(sf, mp, false, nil, nil)

			err := Execute(state)
			if err == nil {
				t.Fatalf("Execute: want error for %s, got nil", tc.name)
			}
			if !strings.Contains(err.Error(), tc.wantSubstr) {
				t.Errorf("error: got %q, want substring %q", err.Error(), tc.wantSubstr)
			}
			if len(mp.setCurLevelCalls) != 0 {
				t.Errorf("setCurLevelCalls: should not have been called, got %d", len(mp.setCurLevelCalls))
			}
		})
	}
}

// TestHandleStatAdvanceNullRejected pins STAT_ADVANCE: TS wraps BOTH stat
// and xp with NumberNotNull (PlayerOps.ts:762-763) — NOT PlayerStatValid
// for stat (this is a TS asymmetry vs. sibling stat ops). Both ints are
// pinned here.
func TestHandleStatAdvanceNullRejected(t *testing.T) {
	tests := []struct {
		name       string
		statID, xp int32
		wantSubstr string
	}{
		{
			name:       "null_stat",
			statID:     -1,
			xp:         100,
			wantSubstr: "STAT_ADVANCE: input number was null(-1)",
		},
		{
			name:       "null_xp",
			statID:     2,
			xp:         -1,
			wantSubstr: "STAT_ADVANCE: input number was null(-1)",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			mp := &mockPlayer{}
			sf := &ScriptFile{
				Name: "stat_advance_" + tc.name,
				Opcodes: []Opcode{
					OpPushConstantInt, // stat id (bottom)
					OpPushConstantInt, // xp (top)
					OpStatAdvance,
					OpReturn,
				},
				IntOperands: []int32{tc.statID, tc.xp, 0, 0},
			}
			state := Init(sf, mp, false, nil, nil)

			err := Execute(state)
			if err == nil {
				t.Fatalf("Execute: want error for %s, got nil", tc.name)
			}
			if !strings.Contains(err.Error(), tc.wantSubstr) {
				t.Errorf("error: got %q, want substring %q", err.Error(), tc.wantSubstr)
			}
			if len(mp.addXPCalls) != 0 {
				t.Errorf("addXPCalls: should not have been called, got %d", len(mp.addXPCalls))
			}
		})
	}
}

// TestHandleSpotAnimPlNullRejected pins SPOTANIM_PL: TS wraps delay (top
// of stack) with NumberNotNull (PlayerOps.ts:589). height and spotanim
// are NOT wrapped; only delay is pinned here.
func TestHandleSpotAnimPlNullRejected(t *testing.T) {
	mp := &mockPlayer{}
	sf := &ScriptFile{
		Name: "spotanim_pl_null_delay",
		Opcodes: []Opcode{
			OpPushConstantInt, // spotanim (bottom)
			OpPushConstantInt, // height
			OpPushConstantInt, // delay (top) = -1
			OpSpotAnimPl,
			OpReturn,
		},
		IntOperands: []int32{100, 0, -1, 0, 0},
	}
	state := Init(sf, mp, false, nil, nil)

	err := Execute(state)
	if err == nil {
		t.Fatalf("Execute: want error for delay=-1, got nil")
	}
	want := "SPOTANIM_PL: input number was null(-1)"
	if !strings.Contains(err.Error(), want) {
		t.Errorf("error: got %q, want substring %q", err.Error(), want)
	}
	if mp.playSpotAnimCalls != 0 {
		t.Errorf("playSpotAnimCalls: should not have been called, got %d", mp.playSpotAnimCalls)
	}
}

// TestHandlePApRangeNullRejected pins P_APRANGE: TS wraps with
// NumberNotNull (PlayerOps.ts:353).
func TestHandlePApRangeNullRejected(t *testing.T) {
	fake := &mockPlayer{}
	s := &ScriptState{
		IntStack: make([]int, StackCapacity),
		Self:     fake,
		Protect:  true,
	}
	s.Pointers |= PtrActivePlayer
	s.PushInt(-1)

	err := handlePApRange(s)
	if err == nil {
		t.Fatal("handlePApRange: want error for n=-1, got nil")
	}
	want := "P_APRANGE: input number was null(-1)"
	if !strings.Contains(err.Error(), want) {
		t.Errorf("error: got %q, want substring %q", err.Error(), want)
	}
	if fake.setApRangeCalls != 0 {
		t.Errorf("setApRangeCalls: should not have been called, got %d", fake.setApRangeCalls)
	}
}

// TestHandlePOpLocNullRejected pins P_OPLOC: TS wraps op with
// NumberNotNull (PlayerOps.ts:387). The wrap fires before the [1..5]
// range check, so -1 produces the NumberNotNull error.
func TestHandlePOpLocNullRejected(t *testing.T) {
	mp := &mockPlayer{}
	loc := &mockActiveLoc{locType: 42}
	sf := &ScriptFile{
		Name:        "p_op_loc_null",
		Opcodes:     []Opcode{OpPushConstantInt, OpPOpLoc, OpReturn},
		IntOperands: []int32{-1, 0, 0},
	}
	state := Init(sf, mp, true, nil, nil)
	state.ActiveLoc = loc
	state.Pointers |= PtrActiveLoc

	err := Execute(state)
	if err == nil {
		t.Fatal("Execute: want error for op=-1, got nil")
	}
	want := "P_OPLOC: input number was null(-1)"
	if !strings.Contains(err.Error(), want) {
		t.Errorf("error: got %q, want substring %q", err.Error(), want)
	}
	if len(mp.lastSetInteractionScriptLoc) != 0 {
		t.Errorf("lastSetInteractionScriptLoc: should not have been called, got %d", len(mp.lastSetInteractionScriptLoc))
	}
}

// TestHandlePOpNpcNullRejected pins P_OPNPC: TS wraps op with
// NumberNotNull (PlayerOps.ts:404).
func TestHandlePOpNpcNullRejected(t *testing.T) {
	mp := &mockPlayer{}
	npc := &mockActiveNpc{typeId: 7}
	sf := &ScriptFile{
		Name:        "p_op_npc_null",
		Opcodes:     []Opcode{OpPushConstantInt, OpPOpNpc, OpReturn},
		IntOperands: []int32{-1, 0, 0},
	}
	state := Init(sf, mp, true, nil, nil)
	state.ActiveNpc = npc
	state.Pointers |= PtrActiveNpc

	err := Execute(state)
	if err == nil {
		t.Fatal("Execute: want error for op=-1, got nil")
	}
	want := "P_OPNPC: input number was null(-1)"
	if !strings.Contains(err.Error(), want) {
		t.Errorf("error: got %q, want substring %q", err.Error(), want)
	}
	if len(mp.lastSetInteractionScriptNpc) != 0 {
		t.Errorf("lastSetInteractionScriptNpc: should not have been called, got %d", len(mp.lastSetInteractionScriptNpc))
	}
}

// --- NAI-35-T4: HUNTALL handler tests ----------------------------------

// newHuntAllState pushes (coord, distance, huntvis) — popInts(3) order
// matching TS PlayerOps.ts:1215-1223. Mirrors handlers_npc_test.go's
// newNpcHuntAllState convention.
func newHuntAllState(t *testing.T, coord, distance, huntvis int, lookup *mockPlayerLookup) *ScriptState {
	t.Helper()
	mw := newMockWorld()
	mw.tick = 100
	s := &ScriptState{
		Script:      &ScriptFile{IntOperands: []int32{0}},
		PC:          0,
		World:       mw,
		IntStack:    make([]int, StackCapacity),
		StringStack: make([]string, StackCapacity),
	}
	if lookup != nil {
		s.PlayerLookup = lookup
	}
	s.PushInt(coord)
	s.PushInt(distance)
	s.PushInt(huntvis)
	return s
}

func TestHandleHuntAll_StoresHuntAllPlayerIterator(t *testing.T) {
	coord := (2 << 28) | (3200 << 14) | 3300
	s := newHuntAllState(t, coord, 10, objtype.HuntVisLineOfSight, &mockPlayerLookup{})
	if err := handleHuntAll(s); err != nil {
		t.Fatalf("handleHuntAll: %v", err)
	}
	if s.playerIterator == nil {
		t.Fatal("playerIterator should be non-nil after HUNTALL")
	}
	if s.playerIterator.mode != PlayerIteratorHuntAll {
		t.Errorf("mode: got %v, want PlayerIteratorHuntAll", s.playerIterator.mode)
	}
	if s.playerIterator.huntvis != objtype.HuntVisLineOfSight {
		t.Errorf("huntvis: got %d, want HuntVisLineOfSight (%d)", s.playerIterator.huntvis, objtype.HuntVisLineOfSight)
	}
	if s.playerIterator.creationTick != 100 {
		t.Errorf("creationTick: got %d, want 100 (from World.CurrentTick)", s.playerIterator.creationTick)
	}
	if s.playerIterator.level != 2 || s.playerIterator.x != 3200 || s.playerIterator.z != 3300 {
		t.Errorf("center: got (level=%d, x=%d, z=%d), want (2, 3200, 3300)",
			s.playerIterator.level, s.playerIterator.x, s.playerIterator.z)
	}
	if s.playerIterator.distance != 10 {
		t.Errorf("distance: got %d, want 10", s.playerIterator.distance)
	}
	if s.ISP != 0 {
		t.Errorf("HUNTALL should not push; ISP=%d", s.ISP)
	}
}

func TestHandleHuntAll_NilLookupDegrades(t *testing.T) {
	coord := (0 << 28) | (3200 << 14) | 3300
	s := newHuntAllState(t, coord, 10, objtype.HuntVisOff, nil)
	// PlayerLookup left nil.
	if err := handleHuntAll(s); err != nil {
		t.Fatalf("handleHuntAll with nil PlayerLookup: %v", err)
	}
	if s.playerIterator != nil {
		t.Error("playerIterator should remain nil when PlayerLookup is nil (degrades to HUNTNEXT push-0)")
	}
}

func TestHandleHuntAll_InvalidHuntVisRejected(t *testing.T) {
	coord := (0 << 28) | (3200 << 14) | 3300
	s := newHuntAllState(t, coord, 10, 99, &mockPlayerLookup{})
	if err := handleHuntAll(s); err == nil {
		t.Fatal("expected validator error for invalid huntvis=99")
	} else if !strings.Contains(err.Error(), "HUNTALL") {
		t.Errorf("error should be tagged HUNTALL: %v", err)
	}
	if s.playerIterator != nil {
		t.Error("playerIterator should remain nil after validation error")
	}
}

// --- NAI-35-T5: HUNTNEXT handler tests ---------------------------------

// newHuntNextState mirrors newNpcFindNextState (handlers_npc_test.go:1860):
// builds a ScriptState with a pre-set playerIterator and configurable
// World tick. Tests use this for direct handler-level coverage.
func newHuntNextState(t *testing.T, tick int, iter *PlayerIterator) *ScriptState {
	t.Helper()
	mw := newMockWorld()
	mw.tick = tick
	s := &ScriptState{
		Script:      &ScriptFile{IntOperands: []int32{0}},
		PC:          0,
		World:       mw,
		IntStack:    make([]int, StackCapacity),
		StringStack: make([]string, StackCapacity),
	}
	s.playerIterator = iter
	return s
}

func TestHandleHuntNext_NilIteratorPushesZero(t *testing.T) {
	s := newHuntNextState(t, 0, nil)
	if err := handleHuntNext(s); err != nil {
		t.Fatalf("handleHuntNext: %v", err)
	}
	if got := s.PopInt(); got != 0 {
		t.Errorf("nil iterator: got push %d, want 0", got)
	}
	if s.Self != nil {
		t.Error("Self should remain nil on nil iterator")
	}
	if s.Pointers&PtrActivePlayer != 0 {
		t.Error("PtrActivePlayer should NOT be set on nil iterator")
	}
}

func TestHandleHuntNext_StaleIteratorReturnsError(t *testing.T) {
	// Iterator created at tick=3, World now at tick=5 → stale.
	iter := NewHuntAllPlayerIterator(
		&mockPlayerLookup{}, nil, 3, 0, 3200, 3200, 8, objtype.HuntVisOff,
	)
	s := newHuntNextState(t, 5, iter)

	err := handleHuntNext(s)
	if err == nil {
		t.Fatal("stale iterator should return error")
	}
	if !strings.Contains(err.Error(), "HUNTNEXT") {
		t.Errorf("error should be tagged HUNTNEXT: %v", err)
	}
	if !strings.Contains(err.Error(), "tried to use an old iterator") {
		t.Errorf("error message should mention old iterator: %v", err)
	}
}

func TestHandleHuntNext_HitSetsSelfAndPushesOne(t *testing.T) {
	// HuntAll cursor for (level=0, x=3200, z=3200, distance=8):
	//   centerX = 3200>>3 = 400; radius = 1+8/8 = 2.
	//   curZoneX=curZoneZ=402 (max corner). First ZonePlayers lookup at
	//   world coords (402*8, 402*8) = (3216, 3216).
	// Player at (3204, 3204): DistanceToSW(3200,3200,3204,3204) = max(4,4) = 4 ≤ 8 → hit.
	target := &mockPlayer{username: "Hit", x: 3204, z: 3204}
	lookup := &mockPlayerLookup{
		byZone: map[zoneKey][]ActivePlayer{
			{0, 3216, 3216}: {target},
		},
	}
	iter := NewHuntAllPlayerIterator(
		lookup, nil, 100, 0, 3200, 3200, 8, objtype.HuntVisOff,
	)
	s := newHuntNextState(t, 100, iter)

	if err := handleHuntNext(s); err != nil {
		t.Fatalf("handleHuntNext: %v", err)
	}
	if s.ISP != 1 || s.IntStack[0] != 1 {
		t.Errorf("stack: got [%v], want [1]", s.IntStack[:s.ISP])
	}
	if s.Self != target {
		t.Errorf("Self: got %v, want target %v", s.Self, target)
	}
	if s.Pointers&PtrActivePlayer == 0 {
		t.Error("PtrActivePlayer should be set on hit")
	}
}

func TestHandleHuntNext_ExhaustionPushesZero(t *testing.T) {
	// Empty PlayerLookup → iterator walks all zones in the radius and
	// finds nothing.
	lookup := &mockPlayerLookup{}
	iter := NewHuntAllPlayerIterator(
		lookup, nil, 100, 0, 3200, 3200, 8, objtype.HuntVisOff,
	)
	s := newHuntNextState(t, 100, iter)

	if err := handleHuntNext(s); err != nil {
		t.Fatalf("handleHuntNext: %v", err)
	}
	if s.ISP != 1 || s.IntStack[0] != 0 {
		t.Errorf("stack: got [%v], want [0]", s.IntStack[:s.ISP])
	}
	if s.Self != nil {
		t.Error("Self should remain nil on exhaustion (no hit to bind)")
	}
	if s.Pointers&PtrActivePlayer != 0 {
		t.Error("PtrActivePlayer should NOT be set on exhaustion")
	}
}

// TestHandleHuntNext_ExhaustionDoesNotClearIterator pins
// iterator_state_pattern.md element 7: exhaustion does NOT nil out
// s.playerIterator. Mirrors NPC parity at handlers_npc_test.go:1926.
func TestHandleHuntNext_ExhaustionDoesNotClearIterator(t *testing.T) {
	lookup := &mockPlayerLookup{}
	iter := NewHuntAllPlayerIterator(
		lookup, nil, 100, 0, 3200, 3200, 8, objtype.HuntVisOff,
	)
	s := newHuntNextState(t, 100, iter)

	if err := handleHuntNext(s); err != nil {
		t.Fatalf("first handleHuntNext: %v", err)
	}
	_ = s.PopInt() // discard first push
	if s.playerIterator == nil {
		t.Fatal("playerIterator should NOT be cleared on exhaustion (TS parity)")
	}

	// Second call on the now-exhausted iterator must also push 0
	// without erroring (Stale check still passes — same tick).
	if err := handleHuntNext(s); err != nil {
		t.Fatalf("second handleHuntNext: %v", err)
	}
	if got := s.PopInt(); got != 0 {
		t.Errorf("second exhaustion: got push %d, want 0", got)
	}
	if s.playerIterator == nil {
		t.Error("playerIterator should still be non-nil after second call")
	}
}

// --- NAI-37 Task 6: HINT_NPC handler unit tests ---------------------------

func TestHintNpc_NoActivePlayer_Errors(t *testing.T) {
	npc := &mockNpc{nid: 42}
	s := &ScriptState{
		IntStack:    make([]int, StackCapacity),
		StringStack: make([]string, StackCapacity),
		ActiveNpc:   npc,
	} // no Self
	if err := handleHintNpc(s); err == nil {
		t.Fatalf("expected error for no active player")
	}
}

func TestHintNpc_NoActiveNpc_Errors(t *testing.T) {
	pl := &mockPlayer{}
	s := &ScriptState{
		IntStack:    make([]int, StackCapacity),
		StringStack: make([]string, StackCapacity),
		Self:        pl,
		Pointers:    PtrActivePlayer,
	} // no ActiveNpc
	if err := handleHintNpc(s); err == nil {
		t.Fatalf("expected error for no active npc")
	}
	if len(pl.hintNpcCalls) != 0 {
		t.Errorf("hintNpcCalls: got %d, want 0 on validation failure",
			len(pl.hintNpcCalls))
	}
}

func TestHintNpc_Success_RecordsNid(t *testing.T) {
	pl := &mockPlayer{}
	npc := &mockNpc{nid: 4242}
	s := &ScriptState{
		IntStack:    make([]int, StackCapacity),
		StringStack: make([]string, StackCapacity),
		Self:        pl,
		Pointers:    PtrActivePlayer,
		ActiveNpc:   npc,
	}
	if err := handleHintNpc(s); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if want := []int{4242}; !slices.Equal(pl.hintNpcCalls, want) {
		t.Errorf("hintNpcCalls: got %v, want %v", pl.hintNpcCalls, want)
	}
}

// --- NAI-39 Task 1: requireActivePlayer2 unit tests ----------------------

// TestRequireActivePlayer2_NoBit_Errors pins the pointer-bit check:
// Self2 is set but PtrActivePlayer2 is unset → error. Without this direct
// helper test, a bug that drops the bit-mask check could pass the
// handler-level "Self2 set" path silently (per test_passes_for_wrong_reason.md).
func TestRequireActivePlayer2_NoBit_Errors(t *testing.T) {
	s := &ScriptState{
		IntStack:    make([]int, StackCapacity),
		StringStack: make([]string, StackCapacity),
		Self2:       &mockPlayer{},
		Pointers:    PtrActivePlayer, // PtrActivePlayer2 NOT set
	}
	if err := requireActivePlayer2(s, "TEST"); err == nil {
		t.Fatal("expected error when PtrActivePlayer2 unset")
	}
}

// TestRequireActivePlayer2_NilSelf2_Errors pins the nil-receiver check:
// PtrActivePlayer2 is set but Self2 is nil → error. Defends against the
// flag/state mismatch case that buildPlayerScriptState's atomic seeding
// is supposed to prevent.
func TestRequireActivePlayer2_NilSelf2_Errors(t *testing.T) {
	s := &ScriptState{
		IntStack:    make([]int, StackCapacity),
		StringStack: make([]string, StackCapacity),
		Pointers:    PtrActivePlayer | PtrActivePlayer2,
		// Self2 nil
	}
	if err := requireActivePlayer2(s, "TEST"); err == nil {
		t.Fatal("expected error when Self2 nil")
	}
}

// TestRequireActivePlayer2_Both_OK pins the both-present happy path.
func TestRequireActivePlayer2_Both_OK(t *testing.T) {
	s := &ScriptState{
		IntStack:    make([]int, StackCapacity),
		StringStack: make([]string, StackCapacity),
		Self2:       &mockPlayer{},
		Pointers:    PtrActivePlayer | PtrActivePlayer2,
	}
	if err := requireActivePlayer2(s, "TEST"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// --- NAI-39 Task 4: HINT_COORD / HINT_PL / HINT_STOP handler unit tests ---

func TestHintCoord_NoActivePlayer_Errors(t *testing.T) {
	s := &ScriptState{
		IntStack:    make([]int, StackCapacity),
		StringStack: make([]string, StackCapacity),
	} // no Self
	if err := handleHintCoord(s); err == nil {
		t.Fatal("expected error for no active player")
	}
}

func TestHintCoord_InvalidCoord_Errors(t *testing.T) {
	pl := &mockPlayer{}
	s := &ScriptState{
		IntStack:    make([]int, StackCapacity),
		StringStack: make([]string, StackCapacity),
		Self:        pl,
		Pointers:    PtrActivePlayer,
	}
	// Push offset=3, coord=-1 (invalid), height=0. Pop order is height,
	// coord, offset — so push offset FIRST.
	s.PushInt(3)
	s.PushInt(-1)
	s.PushInt(0)
	if err := handleHintCoord(s); err == nil {
		t.Fatal("expected error for invalid coord")
	}
	if len(pl.hintCoordCalls) != 0 {
		t.Errorf("hintCoordCalls: got %d, want 0 on validation failure", len(pl.hintCoordCalls))
	}
}

func TestHintCoord_Success_RecordsArgs(t *testing.T) {
	pl := &mockPlayer{}
	s := &ScriptState{
		IntStack:    make([]int, StackCapacity),
		StringStack: make([]string, StackCapacity),
		Self:        pl,
		Pointers:    PtrActivePlayer,
	}
	// coord = pack(level=0, x=100, z=200) = (0<<28)|(100<<14)|200
	coord := (100 << 14) | 200
	s.PushInt(3)     // offset
	s.PushInt(coord) // coord
	s.PushInt(42)    // height
	if err := handleHintCoord(s); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := []mockHintCoord{{offset: 3, x: 100, z: 200, height: 42}}
	if !slices.Equal(pl.hintCoordCalls, want) {
		t.Errorf("hintCoordCalls: got %v, want %v", pl.hintCoordCalls, want)
	}
}

// TestHintCoord_PopOrderDistinctValues pins which popped value lands in
// which dispatch arg. Distinct values rule out symmetric off-by-one.
func TestHintCoord_PopOrderDistinctValues(t *testing.T) {
	pl := &mockPlayer{}
	s := &ScriptState{
		IntStack:    make([]int, StackCapacity),
		StringStack: make([]string, StackCapacity),
		Self:        pl,
		Pointers:    PtrActivePlayer,
	}
	// coord = pack(0, 1, 2)
	coord := (1 << 14) | 2
	s.PushInt(2) // offset (push first, popped last)
	s.PushInt(coord)
	s.PushInt(99) // height (push last, popped first)
	if err := handleHintCoord(s); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := []mockHintCoord{{offset: 2, x: 1, z: 2, height: 99}}
	if !slices.Equal(pl.hintCoordCalls, want) {
		t.Errorf("hintCoordCalls: got %v, want %v", pl.hintCoordCalls, want)
	}
}

func TestHintPl_NoActivePlayer_Errors(t *testing.T) {
	s := &ScriptState{
		IntStack:    make([]int, StackCapacity),
		StringStack: make([]string, StackCapacity),
	} // no Self, no Self2
	if err := handleHintPl(s); err == nil {
		t.Fatal("expected error for no active player")
	}
}

// TestHintPl_NoActivePlayer2_Errors pins the second guard: Self set +
// PtrActivePlayer set, but Self2 nil + PtrActivePlayer2 unset.
func TestHintPl_NoActivePlayer2_Errors(t *testing.T) {
	pl := &mockPlayer{}
	s := &ScriptState{
		IntStack:    make([]int, StackCapacity),
		StringStack: make([]string, StackCapacity),
		Self:        pl,
		Pointers:    PtrActivePlayer, // PtrActivePlayer2 NOT set
	}
	if err := handleHintPl(s); err == nil {
		t.Fatal("expected error for no active player2")
	}
	if len(pl.hintPlayerCalls) != 0 {
		t.Errorf("hintPlayerCalls: got %d, want 0 on validation failure", len(pl.hintPlayerCalls))
	}
}

func TestHintPl_Success_RecordsSlot(t *testing.T) {
	pl := &mockPlayer{}
	pl2 := &mockPlayer{slot: 7}
	s := &ScriptState{
		IntStack:    make([]int, StackCapacity),
		StringStack: make([]string, StackCapacity),
		Self:        pl,
		Self2:       pl2,
		Pointers:    PtrActivePlayer | PtrActivePlayer2,
	}
	if err := handleHintPl(s); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if want := []int{7}; !slices.Equal(pl.hintPlayerCalls, want) {
		t.Errorf("hintPlayerCalls: got %v, want %v", pl.hintPlayerCalls, want)
	}
}

func TestHintStop_NoActivePlayer_Errors(t *testing.T) {
	s := &ScriptState{
		IntStack:    make([]int, StackCapacity),
		StringStack: make([]string, StackCapacity),
	} // no Self
	if err := handleHintStop(s); err == nil {
		t.Fatal("expected error for no active player")
	}
}

func TestHintStop_Success_IncrementsCounter(t *testing.T) {
	pl := &mockPlayer{}
	s := &ScriptState{
		IntStack:    make([]int, StackCapacity),
		StringStack: make([]string, StackCapacity),
		Self:        pl,
		Pointers:    PtrActivePlayer,
	}
	if err := handleHintStop(s); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if pl.hintStopCalls != 1 {
		t.Errorf("hintStopCalls: got %d, want 1", pl.hintStopCalls)
	}
}

// --- NAI-47: handleSetIdKit ---

func buildIdkTypeConfig(id, typ int) *objtype.IdkType {
	c := objtype.NewIdkType(id)
	c.Type = typ
	return c
}

func TestHandleSetIdKitRequiresActivePlayer(t *testing.T) {
	s := &ScriptState{IntStack: make([]int, StackCapacity)}
	s.PushInt(0)
	s.PushInt(0)
	if err := handleSetIdKit(s); err == nil {
		t.Error("want error for no active player, got nil")
	}
}

func TestHandleSetIdKitNilConfigs(t *testing.T) {
	s := &ScriptState{Pointers: PtrActivePlayer, Self: &mockPlayer{}, IntStack: make([]int, StackCapacity)}
	s.PushInt(0) // idkit (pushed first = below)
	s.PushInt(0) // color (pushed last = top)
	if err := handleSetIdKit(s); err == nil {
		t.Error("want error for nil Configs, got nil")
	}
}

func TestHandleSetIdKitInvalidIdkit(t *testing.T) {
	mc := &mockConfigs{idks: map[int]*objtype.IdkType{}}
	s := &ScriptState{Pointers: PtrActivePlayer, Self: &mockPlayer{}, Configs: mc, IntStack: make([]int, StackCapacity)}
	s.PushInt(5) // idkit=5 — not in registry (pushed first = below)
	s.PushInt(0) // color (pushed last = top)
	if err := handleSetIdKit(s); err == nil {
		t.Error("want error for invalid idkit, got nil")
	}
}

// TestHandleSetIdKitMaleHair: gender=0, idkType.Type=0 (hair) → body[0]=idkit,
// colors[0]=color (hair colorSlot).
func TestHandleSetIdKitMaleHair(t *testing.T) {
	mc := &mockConfigs{idks: map[int]*objtype.IdkType{3: buildIdkTypeConfig(3, 0)}}
	mp := &mockPlayer{genderValue: 0}
	s := &ScriptState{Pointers: PtrActivePlayer, Self: mp, Configs: mc, IntStack: make([]int, StackCapacity)}
	s.PushInt(3) // idkit=3 (Type=0, male hair) — pushed first = below
	s.PushInt(7) // color — pushed last = top
	if err := handleSetIdKit(s); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if mp.bodyParts[0] != 3 {
		t.Errorf("bodyParts[0]: got %d, want 3 (idkit id)", mp.bodyParts[0])
	}
	if mp.colorParts[0] != 7 {
		t.Errorf("colorParts[0]: got %d, want 7 (hair color)", mp.colorParts[0])
	}
}

// TestHandleSetIdKitFemaleSlotAdjust: gender=1, idkType.Type=7 (female hair).
// slot = 7 − 7 = 0, adjustedType = 0 → colorSlot=0.
func TestHandleSetIdKitFemaleSlotAdjust(t *testing.T) {
	mc := &mockConfigs{idks: map[int]*objtype.IdkType{9: buildIdkTypeConfig(9, 7)}}
	mp := &mockPlayer{genderValue: 1}
	s := &ScriptState{Pointers: PtrActivePlayer, Self: mp, Configs: mc, IntStack: make([]int, StackCapacity)}
	s.PushInt(9) // idkit=9 (Type=7 → female hair, slot=0) — pushed first = below
	s.PushInt(2) // color — pushed last = top
	if err := handleSetIdKit(s); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if mp.bodyParts[0] != 9 {
		t.Errorf("bodyParts[0]: got %d, want 9", mp.bodyParts[0])
	}
	if mp.colorParts[0] != 2 {
		t.Errorf("colorParts[0]: got %d, want 2", mp.colorParts[0])
	}
}

// TestHandleSetIdKitSkinNoColorWrite: Type=4 (hands) has no color slot.
// colorParts must stay at zero defaults.
func TestHandleSetIdKitSkinNoColorWrite(t *testing.T) {
	mc := &mockConfigs{idks: map[int]*objtype.IdkType{4: buildIdkTypeConfig(4, 4)}}
	mp := &mockPlayer{genderValue: 0}
	s := &ScriptState{Pointers: PtrActivePlayer, Self: mp, Configs: mc, IntStack: make([]int, StackCapacity)}
	s.PushInt(4)  // idkit=4 (Type=4, hands/skin) — pushed first = below
	s.PushInt(99) // color (should not be written) — pushed last = top
	if err := handleSetIdKit(s); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if mp.bodyParts[4] != 4 {
		t.Errorf("bodyParts[4]: got %d, want 4", mp.bodyParts[4])
	}
	for i, v := range mp.colorParts {
		if v != 0 {
			t.Errorf("colorParts[%d]: got %d, want 0 (no color write for Type=4)", i, v)
		}
	}
}

// TestHandleSetIdKitLegs: Type=5 → colorSlot=2.
func TestHandleSetIdKitLegs(t *testing.T) {
	mc := &mockConfigs{idks: map[int]*objtype.IdkType{5: buildIdkTypeConfig(5, 5)}}
	mp := &mockPlayer{genderValue: 0}
	s := &ScriptState{Pointers: PtrActivePlayer, Self: mp, Configs: mc, IntStack: make([]int, StackCapacity)}
	s.PushInt(5)  // idkit=5 (Type=5, legs) — pushed first = below
	s.PushInt(11) // color — pushed last = top
	if err := handleSetIdKit(s); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if mp.colorParts[2] != 11 {
		t.Errorf("colorParts[2]: got %d, want 11 (legs colorSlot=2)", mp.colorParts[2])
	}
}

// TestHandleWalkTrigger_PopsAndWrites verifies P_WALKTRIGGER (opcode
// 2128) pops one int and writes it via SetWalkTrigger on the active
// player. Mirrors TS PlayerOps.ts:1035-1037.
func TestHandleWalkTrigger_PopsAndWrites(t *testing.T) {
	mp := &mockPlayer{}
	sf := &ScriptFile{
		Name:             "[walktrigger,test]",
		Opcodes:          []Opcode{OpPushConstantInt, OpWalkTrigger, OpReturn},
		IntOperands:      []int32{42, 0, 0},
		StringOperands:   []string{"", "", ""},
		InstructionCount: 3,
	}
	state := Init(sf, mp, false, nil, nil)
	if err := Execute(state); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if mp.walkTriggerSetCalls != 1 {
		t.Errorf("SetWalkTrigger calls: got %d, want 1", mp.walkTriggerSetCalls)
	}
	if mp.lastWalkTriggerSet != 42 {
		t.Errorf("SetWalkTrigger arg: got %d, want 42", mp.lastWalkTriggerSet)
	}
}

// TestHandleWalkTrigger_NoActivePlayer asserts the handler errors when
// the active-player pointer is unset, matching the requireActivePlayer
// contract.
func TestHandleWalkTrigger_NoActivePlayer(t *testing.T) {
	state := &ScriptState{IntStack: make([]int, StackCapacity)}
	state.PushInt(42)
	err := handleWalkTrigger(state)
	if err == nil {
		t.Fatal("handleWalkTrigger: got nil, want no-active-player error")
	}
}

// TestHandleGetWalkTrigger_ReadsAndPushes verifies GETWALKTRIGGER (opcode
// 2023) reads p.walktrigger via WalkTrigger() and pushes the value.
// Mirrors TS PlayerOps.ts:1039-1042.
func TestHandleGetWalkTrigger_ReadsAndPushes(t *testing.T) {
	mp := &mockPlayer{walkTriggerValue: 99}
	sf := &ScriptFile{
		Name:             "[getwalktrigger,test]",
		Opcodes:          []Opcode{OpGetWalkTrigger, OpReturn},
		IntOperands:      []int32{0, 0},
		StringOperands:   []string{"", ""},
		InstructionCount: 2,
	}
	state := Init(sf, mp, false, nil, nil)
	if err := Execute(state); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if state.ISP != 1 {
		t.Fatalf("ISP after GETWALKTRIGGER: got %d, want 1", state.ISP)
	}
	if got := state.PopInt(); got != 99 {
		t.Errorf("popped: got %d, want 99", got)
	}
}

// TestHandleGetWalkTrigger_DefaultUnsetReturnsMinusOne pins the unset
// sentinel propagation through the handler.
func TestHandleGetWalkTrigger_DefaultUnsetReturnsMinusOne(t *testing.T) {
	mp := &mockPlayer{walkTriggerValue: -1}
	sf := &ScriptFile{
		Name:             "[getwalktrigger,test]",
		Opcodes:          []Opcode{OpGetWalkTrigger, OpReturn},
		IntOperands:      []int32{0, 0},
		StringOperands:   []string{"", ""},
		InstructionCount: 2,
	}
	state := Init(sf, mp, false, nil, nil)
	if err := Execute(state); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if got := state.PopInt(); got != -1 {
		t.Errorf("popped: got %d, want -1", got)
	}
}

// TestHandleGetWalkTrigger_NoActivePlayer asserts the handler errors when
// the active-player pointer is unset.
func TestHandleGetWalkTrigger_NoActivePlayer(t *testing.T) {
	state := &ScriptState{IntStack: make([]int, StackCapacity)}
	err := handleGetWalkTrigger(state)
	if err == nil {
		t.Fatal("handleGetWalkTrigger: got nil, want no-active-player error")
	}
}

// TestHandleSessionLog pins the SESSION_LOG opcode (TS PlayerOps.ts:1184-1189).
// Stack convention: pushString(event); pushInt(eventType_unshifted) →
// handler pops eventType+2, pops event, calls Self.AddSessionLog(eventType+2, event).
func TestHandleSessionLog(t *testing.T) {
	mp := &mockPlayer{}
	s := &ScriptState{
		IntStack:     make([]int, StackCapacity),
		StringStack:  make([]string, StackCapacity),
		IntLocals:    []int{},
		StringLocals: []string{},
		Pointers:     PtrActivePlayer,
		Self:         mp,
	}
	// Push string first (deeper), then int (top of int stack).
	s.PushString("hello")
	s.PushInt(0) // script-side 0 → engine-side MODERATOR (2)

	if err := handleSessionLog(s); err != nil {
		t.Fatalf("handleSessionLog: %v", err)
	}
	if got := len(mp.addSessionLogCalls); got != 1 {
		t.Fatalf("AddSessionLog calls: got %d, want 1", got)
	}
	call := mp.addSessionLogCalls[0]
	if call.eventType != 2 {
		t.Errorf("eventType: got %d, want 2 (script 0 → MODERATOR via +2 shift)", call.eventType)
	}
	if call.message != "hello" {
		t.Errorf("message: got %q, want %q", call.message, "hello")
	}
	if len(call.args) != 0 {
		t.Errorf("args: got %v, want empty", call.args)
	}
}

// TestHandleSessionLogModeratorAdventureMapping pins both script-side
// values: 0 → 2 (MODERATOR), 1 → 3 (ADVENTURE).
func TestHandleSessionLogModeratorAdventureMapping(t *testing.T) {
	cases := []struct {
		scriptVal int
		wantType  int
	}{
		{0, 2}, // MODERATOR
		{1, 3}, // ADVENTURE
	}
	for _, tc := range cases {
		t.Run(fmt.Sprintf("script%d_eng%d", tc.scriptVal, tc.wantType), func(t *testing.T) {
			mp := &mockPlayer{}
			s := &ScriptState{
				IntStack:    make([]int, StackCapacity),
				StringStack: make([]string, StackCapacity),
				Pointers:    PtrActivePlayer,
				Self:        mp,
			}
			s.PushString("evt")
			s.PushInt(tc.scriptVal)

			if err := handleSessionLog(s); err != nil {
				t.Fatalf("handleSessionLog: %v", err)
			}
			if mp.addSessionLogCalls[0].eventType != tc.wantType {
				t.Errorf("eventType: got %d, want %d", mp.addSessionLogCalls[0].eventType, tc.wantType)
			}
		})
	}
}

// TestHandleSessionLogRequiresActivePlayer pins the gate.
func TestHandleSessionLogRequiresActivePlayer(t *testing.T) {
	s := &ScriptState{
		IntStack:    make([]int, StackCapacity),
		StringStack: make([]string, StackCapacity),
		Pointers:    0, // no PtrActivePlayer
		Self:        nil,
	}
	if err := handleSessionLog(s); err == nil {
		t.Fatal("handleSessionLog: want error on missing ActivePlayer, got nil")
	}
}
