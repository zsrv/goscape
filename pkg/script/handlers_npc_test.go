package script

import (
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/zsrv/goscape/pkg/objtype"
)

// --- S7f: validator unit tests -----------------------------------------

func TestCheckCoord(t *testing.T) {
	cases := []struct {
		name    string
		in      int
		wantErr bool
		wantL   int
		wantX   int
		wantZ   int
	}{
		{"zero", 0, false, 0, 0, 0},
		{"valid packed", (2 << 28) | (3200 << 14) | 3300, false, 2, 3200, 3300},
		{"max valid", 2147483647, false, 3, 0x3fff, 0x3fff},
		{"negative", -1, true, 0, 0, 0},
		{"beyond max", 2147483648, true, 0, 0, 0}, // one past upper bound (requires int >= 64-bit)
		{"very negative", -2147483648, true, 0, 0, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			level, x, z, err := checkCoord(tc.in, "TEST")
			if tc.wantErr {
				if err == nil {
					t.Fatalf("checkCoord(%d): want error, got nil", tc.in)
				}
				if !strings.Contains(err.Error(), "TEST:") {
					t.Errorf("error should carry op prefix: %v", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("checkCoord(%d): unexpected error: %v", tc.in, err)
			}
			if level != tc.wantL || x != tc.wantX || z != tc.wantZ {
				t.Errorf("checkCoord(%d) = (%d, %d, %d), want (%d, %d, %d)",
					tc.in, level, x, z, tc.wantL, tc.wantX, tc.wantZ)
			}
		})
	}
}

func TestCheckNpcType(t *testing.T) {
	// Build a minimal ScriptState with a Configs that reports NpcType 7 as present.
	s := &ScriptState{Configs: newTestConfigsWithNpcTypes(map[int]bool{7: true})}

	if err := checkNpcType(s, 7, "TEST"); err != nil {
		t.Errorf("checkNpcType(7) with loaded type: unexpected error %v", err)
	}
	if err := checkNpcType(s, 8, "TEST"); err == nil {
		t.Errorf("checkNpcType(8) with unloaded type: want error")
	} else if !strings.Contains(err.Error(), "TEST:") || !strings.Contains(err.Error(), "8") {
		t.Errorf("error should carry op prefix and offending id: %v", err)
	}
	if err := checkNpcType(s, -1, "TEST"); err == nil {
		t.Errorf("checkNpcType(-1): want error")
	}

	// Nil Configs: always errors.
	s2 := &ScriptState{}
	if err := checkNpcType(s2, 7, "TEST"); err == nil {
		t.Errorf("checkNpcType with nil Configs: want error")
	}
}

func TestCheckHuntVis(t *testing.T) {
	for _, v := range []int{0, 1, 2} {
		if err := checkHuntVis(v, "TEST"); err != nil {
			t.Errorf("checkHuntVis(%d): unexpected error %v", v, err)
		}
	}
	for _, v := range []int{-1, 3, 99} {
		if err := checkHuntVis(v, "TEST"); err == nil {
			t.Errorf("checkHuntVis(%d): want error", v)
		}
	}
}

func TestCheckCategoryType(t *testing.T) {
	// Partial validator: only -1 rejected (S7f-D3).
	if err := checkCategoryType(-1, "TEST"); err == nil {
		t.Errorf("checkCategoryType(-1): want error (null sentinel)")
	}
	for _, v := range []int{0, 1, 100, 999999} {
		if err := checkCategoryType(v, "TEST"); err != nil {
			t.Errorf("checkCategoryType(%d): partial validator should accept; got %v", v, err)
		}
	}
}

// newTestConfigsWithNpcTypes builds a Configs that reports any id in present
// as a valid NpcType. Uses the shared mockConfigs type from handlers_config_test.go.
func newTestConfigsWithNpcTypes(present map[int]bool) Configs {
	mc := &mockConfigs{
		npcs: make(map[int]*objtype.NpcType),
	}
	for id := range present {
		mc.npcs[id] = &objtype.NpcType{ConfigType: objtype.ConfigType{ID: id}}
	}
	return mc
}

// newTestConfigsWithSpotAnims builds a Configs that reports any id in present
// as a valid SpotanimType. Uses the shared mockConfigs type from handlers_config_test.go.
// NAI-120 Bundle 2C.
func newTestConfigsWithSpotAnims(present map[int]bool) Configs {
	mc := &mockConfigs{
		spotAnimTypes: make(map[int]*objtype.SpotanimType),
	}
	for id := range present {
		mc.spotAnimTypes[id] = objtype.NewSpotanimType(id)
	}
	return mc
}

func TestSetActiveNpcSlot_OperandZero(t *testing.T) {
	s := &ScriptState{
		Script: &ScriptFile{IntOperands: []int32{0}},
		PC:     0,
	}
	npc := &mockNpc{typeID: 42}
	setActiveNpcSlot(s, npc)
	if s.ActiveNpc != npc {
		t.Errorf("ActiveNpc: got %v, want %v", s.ActiveNpc, npc)
	}
	if s.OtherActiveNpc != nil {
		t.Errorf("OtherActiveNpc: got %v, want nil", s.OtherActiveNpc)
	}
	if s.Pointers&PtrActiveNpc == 0 {
		t.Error("PtrActiveNpc should be set")
	}
	if s.Pointers&PtrActiveNpc2 != 0 {
		t.Error("PtrActiveNpc2 should NOT be set")
	}
}

func TestSetActiveNpcSlot_OperandOne(t *testing.T) {
	s := &ScriptState{
		Script: &ScriptFile{IntOperands: []int32{1}},
		PC:     0,
	}
	npc := &mockNpc{typeID: 42}
	setActiveNpcSlot(s, npc)
	if s.OtherActiveNpc != npc {
		t.Errorf("OtherActiveNpc: got %v, want %v", s.OtherActiveNpc, npc)
	}
	if s.ActiveNpc != nil {
		t.Errorf("ActiveNpc: got %v, want nil", s.ActiveNpc)
	}
	if s.Pointers&PtrActiveNpc2 == 0 {
		t.Error("PtrActiveNpc2 should be set")
	}
	if s.Pointers&PtrActiveNpc != 0 {
		t.Error("PtrActiveNpc should NOT be set")
	}
}

func TestSetActiveNpcSlot_InvalidOperand(t *testing.T) {
	s := &ScriptState{
		Script: &ScriptFile{IntOperands: []int32{2}},
		PC:     0,
	}
	defer func() {
		if r := recover(); r == nil {
			t.Error("setActiveNpcSlot with operand=2 should panic")
		}
	}()
	setActiveNpcSlot(s, &mockNpc{typeID: 42})
}

// TestNpcLookupInterfaceShape is a compile-time assertion that the
// NpcLookup interface has the three expected methods. If this test
// compiles, the interface is correctly defined.
func TestNpcLookupInterfaceShape(t *testing.T) {
	var _ NpcLookup = (*mockNpcLookup)(nil)
	s := &ScriptState{}
	s.Npcs = &mockNpcLookup{}
	_ = s.Npcs
}

type mockEnqueueCall struct {
	trigger    ServerTriggerType
	delay      int
	lastIntArg int
}

// mockNpc is a test fixture implementing ActiveNpc. Pre-seed fields then
// assign to state.ActiveNpc before Execute.
type mockNpc struct {
	typeID, x, z, level, uid, category int
	nid                                int
	respawnrate                        int
	curHP, baseHP                      int
	// NAI-120 Bundle 2C: NPC_STATADD/SUB capture. Sparse maps let tests seed
	// specific stat slots without seeding all 6. curHP/baseHP remain as
	// fallback for stat=0 when the maps are unset.
	levels          map[int]int
	baseLevels      map[int]int
	setNpcStatCalls []struct{ stat, level int }
	// NAI-120 Bundle 2C: SPOTANIM_NPC capture.
	playSpotAnimCalls []struct{ id, height, delay int }
	// NAI-120 Bundle 2D: NPC_HEROPOINTS capture.
	addHeroPointsCalls []struct{ playerUID, amount int }
	// NAI-127 Bundle 1: NPC_FINDHERO ledger-top getter.
	topContributor int
	varns          map[int]int32
	varnsString       map[int]string
	sayCalls                           []string
	animCalls                          []struct{ id, delay int }
	faceCoordCalls                     []struct{ x, z int }
	changeTypeCalls                    []struct{ newType, duration int }
	changeTypeKeepAllCalls             []struct{ newType, duration int }
	damageCalls                        []struct{ amount, dmgType int }
	enqueueCalls                       []mockEnqueueCall
	// NAI-125: mirrors PathingEntity.lastMovement reader for
	// NPC_ARRIVEDELAY. Default 0 = "never moved".
	lastMovement                       int
	setDelayedCalls                    []int
	setTimerCalls                      []int
	setHuntRangeCalls                  []int
	setHuntModeCalls                   []int
	walkTriggerCalls                   []int
	walkTriggerArgCalls                []int
	teleportCalls                      []struct{ x, z, level int }
	queueWaypointCalls                 []struct{ x, z int }
	targetOpField                      int

	// NAI-36 Task 6: NPC_SETMODE recorder fields.
	clearInteractionCalls     int
	resetDefaultsCalls        int
	clearPatrolCalls          int
	setTargetOpCalls          []int
	setInteractionScriptCalls []struct {
		target any
		mode   int
	}
	// NAI-160 T7: NPC_INRANGE seeded value.
	targetWithinMaxRangeValue bool
}

func (m *mockNpc) NpcType() int     { return m.typeID }
func (m *mockNpc) NpcX() int        { return m.x }
func (m *mockNpc) NpcZ() int        { return m.z }
func (m *mockNpc) NpcLevel() int    { return m.level }
func (m *mockNpc) NpcUID() int      { return m.uid }
func (m *mockNpc) Nid() int         { return m.nid }
func (m *mockNpc) LastMovement() int { return m.lastMovement }
func (m *mockNpc) Respawnrate() int  { return m.respawnrate }
func (m *mockNpc) TopContributor() int { return m.topContributor }
func (m *mockNpc) NpcCategory() int { return m.category }

// TargetWithinMaxRange returns the seeded value. NAI-160 T7.
func (m *mockNpc) TargetWithinMaxRange() bool { return m.targetWithinMaxRangeValue }

func (m *mockNpc) NpcStat(stat int) int {
	if m.levels != nil {
		if v, ok := m.levels[stat]; ok {
			return v
		}
	}
	if stat == 0 {
		return m.curHP
	}
	return 0
}

func (m *mockNpc) NpcBaseStat(stat int) int {
	if m.baseLevels != nil {
		if v, ok := m.baseLevels[stat]; ok {
			return v
		}
	}
	if stat == 0 {
		return m.baseHP
	}
	return 0
}

func (m *mockNpc) SetNpcStat(stat, level int) {
	m.setNpcStatCalls = append(m.setNpcStatCalls, struct{ stat, level int }{stat, level})
	if m.levels == nil {
		m.levels = make(map[int]int)
	}
	m.levels[stat] = level
}

func (m *mockNpc) PlaySpotAnim(id, height, delay int) {
	m.playSpotAnimCalls = append(m.playSpotAnimCalls, struct{ id, height, delay int }{id, height, delay})
}

func (m *mockNpc) AddHeroPoints(playerUID, amount int) {
	m.addHeroPointsCalls = append(m.addHeroPointsCalls, struct{ playerUID, amount int }{playerUID, amount})
}

func (m *mockNpc) NpcVarN(id int) int32 {
	if m.varns == nil {
		return 0
	}
	return m.varns[id]
}

func (m *mockNpc) SetNpcVarN(id int, val int32) {
	if m.varns == nil {
		m.varns = make(map[int]int32)
	}
	m.varns[id] = val
}

func (m *mockNpc) NpcVarNString(id int) string {
	if m.varnsString == nil {
		return ""
	}
	return m.varnsString[id]
}

func (m *mockNpc) SetNpcVarNString(id int, val string) {
	if m.varnsString == nil {
		m.varnsString = make(map[int]string)
	}
	m.varnsString[id] = val
}

func (m *mockNpc) Say(text []byte) {
	m.sayCalls = append(m.sayCalls, string(text))
}

func (m *mockNpc) Animate(id, delay int) {
	m.animCalls = append(m.animCalls, struct{ id, delay int }{id, delay})
}
func (m *mockNpc) FaceCoord(x, z int) {
	m.faceCoordCalls = append(m.faceCoordCalls, struct{ x, z int }{x, z})
}
func (m *mockNpc) ChangeType(newType, duration int) {
	m.changeTypeCalls = append(m.changeTypeCalls, struct{ newType, duration int }{newType, duration})
}

func (m *mockNpc) ChangeTypeKeepAll(newType, duration int) {
	m.changeTypeKeepAllCalls = append(m.changeTypeKeepAllCalls, struct{ newType, duration int }{newType, duration})
}

func (m *mockNpc) Damage(amount, dmgType int) {
	m.damageCalls = append(m.damageCalls, struct{ amount, dmgType int }{amount, dmgType})
}

func (m *mockNpc) StoreActiveScript(_ *ScriptState)         {}
func (m *mockNpc) ClearActiveScript()                       {}
func (m *mockNpc) OnScriptFinishedOrAborted(_ *ScriptState) {}
func (m *mockNpc) SetDelayed(d int) {
	m.setDelayedCalls = append(m.setDelayedCalls, d)
}

func (m *mockNpc) EnqueueScriptForTrigger(trigger ServerTriggerType, delay, lastIntArg int) {
	m.enqueueCalls = append(m.enqueueCalls, mockEnqueueCall{
		trigger:    trigger,
		delay:      delay,
		lastIntArg: lastIntArg,
	})
}

func (m *mockNpc) SetTimer(interval int) {
	m.setTimerCalls = append(m.setTimerCalls, interval)
}

func (m *mockNpc) SetHuntRange(r int) {
	m.setHuntRangeCalls = append(m.setHuntRangeCalls, r)
}

func (m *mockNpc) SetHuntMode(mode int) {
	m.setHuntModeCalls = append(m.setHuntModeCalls, mode)
}

func (m *mockNpc) SetWalkTrigger(queueID int) {
	m.walkTriggerCalls = append(m.walkTriggerCalls, queueID)
}

func (m *mockNpc) SetWalkTriggerArg(arg int) {
	m.walkTriggerArgCalls = append(m.walkTriggerArgCalls, arg)
}

func (m *mockNpc) Teleport(x, z, level int) {
	m.teleportCalls = append(m.teleportCalls, struct{ x, z, level int }{x, z, level})
}

func (m *mockNpc) QueueWaypoint(x, z int) {
	m.queueWaypointCalls = append(m.queueWaypointCalls, struct{ x, z int }{x, z})
}

func (m *mockNpc) TargetOp() int { return m.targetOpField }

func (m *mockNpc) ClearInteraction() { m.clearInteractionCalls++ }
func (m *mockNpc) ResetDefaults()    { m.resetDefaultsCalls++ }
func (m *mockNpc) ClearPatrol()      { m.clearPatrolCalls++ }
func (m *mockNpc) SetTargetOp(mode int) {
	m.targetOpField = mode
	m.setTargetOpCalls = append(m.setTargetOpCalls, mode)
}
func (m *mockNpc) SetInteractionScript(target any, mode int) {
	m.setInteractionScriptCalls = append(m.setInteractionScriptCalls, struct {
		target any
		mode   int
	}{target, mode})
}

// runNpcOp executes a single-opcode script against npc + optional mc,
// with pre-pushed int inputs, and returns the resulting state.
func runNpcOp(t *testing.T, npc ActiveNpc, mc *mockConfigs, op Opcode, intInputs []int) *ScriptState {
	t.Helper()
	sf := &ScriptFile{
		Name:             "test_" + op.String(),
		Opcodes:          []Opcode{op, OpReturn},
		IntOperands:      []int32{0, 0},
		StringOperands:   []string{"", ""},
		InstructionCount: 2,
	}
	state := Init(sf, nil, false, nil, nil)
	state.ActiveNpc = npc
	if mc != nil {
		state.Configs = mc
	}
	for _, v := range intInputs {
		state.PushInt(v)
	}
	if err := Execute(state); err != nil {
		t.Fatalf("%s: unexpected error: %v", op.String(), err)
	}
	return state
}

func TestNpcType(t *testing.T) {
	npc := &mockNpc{typeID: 42}
	state := runNpcOp(t, npc, nil, OpNpcType, nil)
	if got := state.PopInt(); got != 42 {
		t.Errorf("NPC_TYPE: got %d, want 42", got)
	}
}

func TestNpcCoord(t *testing.T) {
	// level=1, x=3222, z=3222 → (1<<28) | (3222<<14) | 3222
	npc := &mockNpc{x: 3222, z: 3222, level: 1}
	state := runNpcOp(t, npc, nil, OpNpcCoord, nil)
	want := (1 << 28) | (3222 << 14) | 3222
	if got := state.PopInt(); got != want {
		t.Errorf("NPC_COORD: got %d, want %d", got, want)
	}
}

func TestNpcCoordLevelZero(t *testing.T) {
	npc := &mockNpc{x: 3222, z: 3222, level: 0}
	state := runNpcOp(t, npc, nil, OpNpcCoord, nil)
	want := (3222 << 14) | 3222
	if got := state.PopInt(); got != want {
		t.Errorf("NPC_COORD(level 0): got %d, want %d", got, want)
	}
}

func TestNpcStatHP(t *testing.T) {
	npc := &mockNpc{curHP: 99}
	state := runNpcOp(t, npc, nil, OpNpcStat, []int{0})
	if got := state.PopInt(); got != 99 {
		t.Errorf("NPC_STAT(0): got %d, want 99", got)
	}
}

func TestNpcStatOtherReturnsZero(t *testing.T) {
	npc := &mockNpc{curHP: 99}
	state := runNpcOp(t, npc, nil, OpNpcStat, []int{5})
	if got := state.PopInt(); got != 0 {
		t.Errorf("NPC_STAT(5): got %d, want 0", got)
	}
}

func TestNpcBaseStat(t *testing.T) {
	npc := &mockNpc{baseHP: 75}
	state := runNpcOp(t, npc, nil, OpNpcBaseStat, []int{0})
	if got := state.PopInt(); got != 75 {
		t.Errorf("NPC_BASESTAT(0): got %d, want 75", got)
	}
}

func TestNpcUID(t *testing.T) {
	// (7 << 16) | 3 = 458755
	npc := &mockNpc{uid: (7 << 16) | 3}
	state := runNpcOp(t, npc, nil, OpNpcUID, nil)
	want := (7 << 16) | 3
	if got := state.PopInt(); got != want {
		t.Errorf("NPC_UID: got %d, want %d", got, want)
	}
}

func TestNpcCategory(t *testing.T) {
	mc := newTestConfigs()
	mc.npcs[7] = &objtype.NpcType{
		ConfigType: objtype.ConfigType{ID: 7},
		Name:       "Hans",
		Category:   99,
		Op:         []string{"Talk-to", "", ""},
	}
	npc := &mockNpc{typeID: 7}
	state := runNpcOp(t, npc, mc, OpNpcCategory, nil)
	if got := state.PopInt(); got != 99 {
		t.Errorf("NPC_CATEGORY: got %d, want 99", got)
	}
}

func TestNpcCategoryUnknownTypeReturnsMinusOne(t *testing.T) {
	mc := newTestConfigs()
	npc := &mockNpc{typeID: 9999}
	state := runNpcOp(t, npc, mc, OpNpcCategory, nil)
	if got := state.PopInt(); got != -1 {
		t.Errorf("NPC_CATEGORY(unknown): got %d, want -1", got)
	}
}

func TestNpcName(t *testing.T) {
	mc := newTestConfigs()
	mc.npcs[7] = &objtype.NpcType{
		ConfigType: objtype.ConfigType{ID: 7},
		Name:       "Hans",
		Category:   99,
		Op:         []string{"Talk-to", "", ""},
	}
	npc := &mockNpc{typeID: 7}
	state := runNpcOp(t, npc, mc, OpNpcName, nil)
	if got := state.PopString(); got != "Hans" {
		t.Errorf("NPC_NAME: got %q, want %q", got, "Hans")
	}
}

func TestNpcNameFallsBackToDebugName(t *testing.T) {
	mc := newTestConfigs()
	// mc.npcs[1] has only DebugName = "unnamed_npc"
	npc := &mockNpc{typeID: 1}
	state := runNpcOp(t, npc, mc, OpNpcName, nil)
	if got := state.PopString(); got != "unnamed_npc" {
		t.Errorf("NPC_NAME(debugname fallback): got %q, want %q", got, "unnamed_npc")
	}
}

func TestNpcNameUnknownTypeReturnsNull(t *testing.T) {
	mc := newTestConfigs()
	npc := &mockNpc{typeID: 9999}
	state := runNpcOp(t, npc, mc, OpNpcName, nil)
	if got := state.PopString(); got != "null" {
		t.Errorf("NPC_NAME(unknown): got %q, want %q", got, "null")
	}
}

func TestNpcHasOpExisting(t *testing.T) {
	mc := newTestConfigs()
	mc.npcs[7] = &objtype.NpcType{
		ConfigType: objtype.ConfigType{ID: 7},
		Name:       "Hans",
		Category:   99,
		Op:         []string{"Talk-to", "", ""},
	}
	npc := &mockNpc{typeID: 7}
	state := runNpcOp(t, npc, mc, OpNpcHasOp, []int{1})
	if got := state.PopInt(); got != 1 {
		t.Errorf("NPC_HASOP(1 existing): got %d, want 1", got)
	}
}

func TestNpcHasOpMissing(t *testing.T) {
	mc := newTestConfigs()
	mc.npcs[7] = &objtype.NpcType{
		ConfigType: objtype.ConfigType{ID: 7},
		Name:       "Hans",
		Category:   99,
		Op:         []string{"Talk-to", "", ""},
	}
	npc := &mockNpc{typeID: 7}
	state := runNpcOp(t, npc, mc, OpNpcHasOp, []int{2})
	if got := state.PopInt(); got != 0 {
		t.Errorf("NPC_HASOP(2 empty): got %d, want 0", got)
	}
}

func TestNpcHasOpOutOfRange(t *testing.T) {
	mc := newTestConfigs()
	mc.npcs[7] = &objtype.NpcType{
		ConfigType: objtype.ConfigType{ID: 7},
		Op:         []string{"Talk-to", "", ""},
	}
	npc := &mockNpc{typeID: 7}
	// op=0 is below 1-based range → 0.
	state := runNpcOp(t, npc, mc, OpNpcHasOp, []int{0})
	if got := state.PopInt(); got != 0 {
		t.Errorf("NPC_HASOP(0 OOB low): got %d, want 0", got)
	}
	// op=99 is far above range → 0.
	state = runNpcOp(t, npc, mc, OpNpcHasOp, []int{99})
	if got := state.PopInt(); got != 0 {
		t.Errorf("NPC_HASOP(99 OOB high): got %d, want 0", got)
	}
}

func TestNpcSay(t *testing.T) {
	npc := &mockNpc{typeID: 7}
	sf := &ScriptFile{
		Name:             "[npcsay,test]",
		Opcodes:          []Opcode{OpPushConstantString, OpNpcSay, OpReturn},
		IntOperands:      []int32{0, 0, 0},
		StringOperands:   []string{"hello", "", ""},
		InstructionCount: 3,
	}
	state := Init(sf, nil, false, nil, nil)
	state.ActiveNpc = npc
	state.Pointers |= PtrActiveNpc
	if err := Execute(state); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if got, want := npc.sayCalls, []string{"hello"}; len(got) != 1 || got[0] != want[0] {
		t.Errorf("sayCalls: got %v, want %v", got, want)
	}
}

func TestNpcSayRequiresActiveNpc(t *testing.T) {
	sf := &ScriptFile{
		Name:             "[npcsay_noactive,test]",
		Opcodes:          []Opcode{OpPushConstantString, OpNpcSay, OpReturn},
		IntOperands:      []int32{0, 0, 0},
		StringOperands:   []string{"hello", "", ""},
		InstructionCount: 3,
	}
	state := Init(sf, nil, false, nil, nil)
	// state.ActiveNpc intentionally left nil.
	err := Execute(state)
	if err == nil {
		t.Fatalf("Execute: got nil err, want NPC_SAY: no active npc")
	}
	if got := err.Error(); !strings.Contains(got, "NPC_SAY: no active npc") {
		t.Errorf("err: got %q, want substring 'NPC_SAY: no active npc'", got)
	}
}

func TestNpcOpsRequireActiveNpc(t *testing.T) {
	cases := []struct {
		name    string
		op      Opcode
		inputs  []int // pre-pushed int inputs
		wantMsg string
	}{
		{"NPC_TYPE", OpNpcType, nil, "NPC_TYPE: no active npc"},
		{"NPC_COORD", OpNpcCoord, nil, "NPC_COORD: no active npc"},
		{"NPC_STAT", OpNpcStat, []int{0}, "NPC_STAT: no active npc"},
		{"NPC_BASESTAT", OpNpcBaseStat, []int{0}, "NPC_BASESTAT: no active npc"},
		{"NPC_NAME", OpNpcName, nil, "NPC_NAME: no active npc"},
		{"NPC_HASOP", OpNpcHasOp, []int{1}, "NPC_HASOP: no active npc"},
		{"NPC_UID", OpNpcUID, nil, "NPC_UID: no active npc"},
		{"NPC_CATEGORY", OpNpcCategory, nil, "NPC_CATEGORY: no active npc"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			sf := &ScriptFile{
				Name:             "test_noactive_" + tc.name,
				Opcodes:          []Opcode{tc.op, OpReturn},
				IntOperands:      []int32{0, 0},
				StringOperands:   []string{"", ""},
				InstructionCount: 2,
			}
			state := Init(sf, nil, false, nil, nil)
			for _, v := range tc.inputs {
				state.PushInt(v)
			}
			err := Execute(state)
			if err == nil {
				t.Fatalf("%s: expected error, got nil", tc.name)
			}
			if !strings.Contains(err.Error(), tc.wantMsg) {
				t.Errorf("%s: error %q does not contain %q", tc.name, err.Error(), tc.wantMsg)
			}
		})
	}
}

func TestNpcAnim(t *testing.T) {
	npc := &mockNpc{typeID: 7}
	// Push seq=42, push delay=3, NPC_ANIM. delay is on top per TS.
	sf := &ScriptFile{
		Name:             "[npcanim,test]",
		Opcodes:          []Opcode{OpPushConstantInt, OpPushConstantInt, OpNpcAnim, OpReturn},
		IntOperands:      []int32{42, 3, 0, 0},
		StringOperands:   []string{"", "", "", ""},
		InstructionCount: 4,
	}
	state := Init(sf, nil, false, nil, nil)
	state.ActiveNpc = npc
	state.Pointers |= PtrActiveNpc
	if err := Execute(state); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	want := []struct{ id, delay int }{{42, 3}}
	if !reflect.DeepEqual(npc.animCalls, want) {
		t.Errorf("animCalls: got %v, want %v", npc.animCalls, want)
	}
}

func TestNpcFaceSquare(t *testing.T) {
	npc := &mockNpc{typeID: 7}
	// Packed coord: level=0, x=3222, z=3218 → (0<<28)|(3222<<14)|3218
	coord := int32((3222 << 14) | 3218)
	sf := &ScriptFile{
		Name:             "[npcfacesquare,test]",
		Opcodes:          []Opcode{OpPushConstantInt, OpNpcFaceSquare, OpReturn},
		IntOperands:      []int32{coord, 0, 0},
		StringOperands:   []string{"", "", ""},
		InstructionCount: 3,
	}
	state := Init(sf, nil, false, nil, nil)
	state.ActiveNpc = npc
	state.Pointers |= PtrActiveNpc
	if err := Execute(state); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	want := []struct{ x, z int }{{3222, 3218}}
	if !reflect.DeepEqual(npc.faceCoordCalls, want) {
		t.Errorf("faceCoordCalls: got %v, want %v", npc.faceCoordCalls, want)
	}
}

func TestHandleNpcChangeTypePassesDuration(t *testing.T) {
	npc := &mockNpc{typeID: 7}
	// Push newType=42, push duration=100, NPC_CHANGETYPE. duration on top
	// (TS order: popInts(2) returns [id, duration]).
	sf := &ScriptFile{
		Name:             "[npcchangetype,test]",
		Opcodes:          []Opcode{OpPushConstantInt, OpPushConstantInt, OpNpcChangeType, OpReturn},
		IntOperands:      []int32{42, 100, 0, 0},
		StringOperands:   []string{"", "", "", ""},
		InstructionCount: 4,
	}
	state := Init(sf, nil, false, nil, nil)
	state.ActiveNpc = npc
	state.Pointers |= PtrActiveNpc
	if err := Execute(state); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if len(npc.changeTypeCalls) != 1 {
		t.Fatalf("changeTypeCalls: got %d, want 1", len(npc.changeTypeCalls))
	}
	if got := npc.changeTypeCalls[0]; got.newType != 42 || got.duration != 100 {
		t.Errorf("changeTypeCalls[0]: got (newType=%d, duration=%d), want (42, 100)",
			got.newType, got.duration)
	}
}

// TestHandleNpcChangeTypeKeepAllDispatch verifies that opcode 2506
// pops (newType, duration) in TS order and calls ChangeTypeKeepAll.
func TestHandleNpcChangeTypeKeepAllDispatch(t *testing.T) {
	npc := &mockNpc{typeID: 7}
	state := runNpcOp(t, npc, nil, OpNpcChangeTypeKeepAll, []int{42, 100}) // id=42, duration=100
	_ = state

	if len(npc.changeTypeKeepAllCalls) != 1 {
		t.Fatalf("changeTypeKeepAllCalls: got %d entries, want 1", len(npc.changeTypeKeepAllCalls))
	}
	got := npc.changeTypeKeepAllCalls[0]
	if got.newType != 42 || got.duration != 100 {
		t.Errorf("call: got {newType=%d, duration=%d}, want {42, 100}", got.newType, got.duration)
	}
	if len(npc.changeTypeCalls) != 0 {
		t.Errorf("changeTypeCalls: got %d, want 0 (KEEPALL should not dispatch through ChangeType)",
			len(npc.changeTypeCalls))
	}
}

func TestNpcMutatingOpsRequireActiveNpc_S6c(t *testing.T) {
	cases := []struct {
		op   Opcode
		name string
	}{
		{OpNpcAnim, "NPC_ANIM"},
		{OpNpcFaceSquare, "NPC_FACESQUARE"},
		{OpNpcChangeType, "NPC_CHANGETYPE"},
		{OpNpcChangeTypeKeepAll, "NPC_CHANGETYPE_KEEPALL"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			sf := &ScriptFile{
				Name: "[noactive," + tc.name + "]",
				// Push enough ints so the handler's pops don't underflow:
				// NPC_FACESQUARE needs 1, NPC_ANIM/NPC_CHANGETYPE need 2.
				Opcodes:          []Opcode{OpPushConstantInt, OpPushConstantInt, tc.op, OpReturn},
				IntOperands:      []int32{0, 0, 0, 0},
				StringOperands:   []string{"", "", "", ""},
				InstructionCount: 4,
			}
			state := Init(sf, nil, false, nil, nil)
			// ActiveNpc intentionally nil.
			err := Execute(state)
			if err == nil {
				t.Fatalf("Execute: got nil err, want %q: no active npc", tc.name)
			}
			if !strings.Contains(err.Error(), tc.name+": no active npc") {
				t.Errorf("err: got %q, want substring %q", err, tc.name+": no active npc")
			}
		})
	}
}

func TestNpcDamage(t *testing.T) {
	npc := &mockNpc{typeID: 7}
	// Push type=1, push amount=3, NPC_DAMAGE. amount is on top per TS.
	sf := &ScriptFile{
		Name:             "[npcdamage,test]",
		Opcodes:          []Opcode{OpPushConstantInt, OpPushConstantInt, OpNpcDamage, OpReturn},
		IntOperands:      []int32{1, 3, 0, 0},
		StringOperands:   []string{"", "", "", ""},
		InstructionCount: 4,
	}
	state := Init(sf, nil, false, nil, nil)
	state.ActiveNpc = npc
	state.Pointers |= PtrActiveNpc
	if err := Execute(state); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	want := []struct{ amount, dmgType int }{{3, 1}}
	if !reflect.DeepEqual(npc.damageCalls, want) {
		t.Errorf("damageCalls: got %v, want %v", npc.damageCalls, want)
	}
}

func TestNpcDamageRequiresActiveNpc(t *testing.T) {
	sf := &ScriptFile{
		Name:             "[npcdamage_noactive,test]",
		Opcodes:          []Opcode{OpPushConstantInt, OpPushConstantInt, OpNpcDamage, OpReturn},
		IntOperands:      []int32{0, 0, 0, 0},
		StringOperands:   []string{"", "", "", ""},
		InstructionCount: 4,
	}
	state := Init(sf, nil, false, nil, nil)
	err := Execute(state)
	if err == nil {
		t.Fatalf("Execute: got nil err, want NPC_DAMAGE no-active-npc error")
	}
	if !strings.Contains(err.Error(), "NPC_DAMAGE: no active npc") {
		t.Errorf("err: got %q, want substring 'NPC_DAMAGE: no active npc'", err)
	}
}

// TestHandleNpcQueueEnqueues — NPC_QUEUE pops (delay, arg, queueID)
// in that order (top of stack = delay) and maps queueID (1-20) to
// TriggerAiQueue1 + queueID - 1.
func TestHandleNpcQueueEnqueues(t *testing.T) {
	npc := &mockNpc{}
	sf := &ScriptFile{
		Name: "test_npc_queue",
		Opcodes: []Opcode{
			OpPushConstantInt, // push queueID (3)
			OpPushConstantInt, // push arg (42)
			OpPushConstantInt, // push delay (5)
			OpNpcQueue,
			OpReturn,
		},
		IntOperands: []int32{3, 42, 5, 0, 0},
	}
	state := Init(sf, nil, false, nil, nil)
	state.ActiveNpc = npc
	state.Pointers |= PtrActiveNpc

	if err := Execute(state); err != nil {
		t.Fatalf("Execute: %v", err)
	}

	if len(npc.enqueueCalls) != 1 {
		t.Fatalf("enqueueCalls: got %d, want 1", len(npc.enqueueCalls))
	}
	call := npc.enqueueCalls[0]
	if call.trigger != TriggerAiQueue3 {
		t.Errorf("trigger: got %v, want TriggerAiQueue3", call.trigger)
	}
	if call.delay != 5 {
		t.Errorf("delay: got %d, want 5", call.delay)
	}
	if call.lastIntArg != 42 {
		t.Errorf("lastIntArg: got %d, want 42", call.lastIntArg)
	}
}

// TestHandleNpcQueueWithoutActiveNpcErrors — defensive nil check.
func TestHandleNpcQueueWithoutActiveNpcErrors(t *testing.T) {
	sf := &ScriptFile{
		Name: "npc_queue_no_npc",
		Opcodes: []Opcode{
			OpPushConstantInt, OpPushConstantInt, OpPushConstantInt,
			OpNpcQueue, OpReturn,
		},
		IntOperands: []int32{1, 0, 0, 0, 0},
	}
	state := Init(sf, nil, false, nil, nil)
	// state.ActiveNpc intentionally nil.

	err := Execute(state)
	if err == nil {
		t.Fatalf("Execute: want error, got nil")
	}
	want := "NPC_QUEUE: no active npc"
	if got := err.Error(); got != want {
		t.Errorf("error: got %q, want %q", got, want)
	}
}

// TestHandleNpcQueueInvalidQueueIDErrors — queueID out of [1,20].
func TestHandleNpcQueueInvalidQueueIDErrors(t *testing.T) {
	cases := []struct {
		name    string
		queueID int32
		wantErr string
	}{
		{"zero", 0, "NPC_QUEUE: invalid queueId 0 (want 1..20)"},
		{"twentyone", 21, "NPC_QUEUE: invalid queueId 21 (want 1..20)"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			npc := &mockNpc{}
			sf := &ScriptFile{
				Name: "npc_queue_invalid_id",
				Opcodes: []Opcode{
					OpPushConstantInt, // queueID
					OpPushConstantInt, // arg
					OpPushConstantInt, // delay
					OpNpcQueue,
					OpReturn,
				},
				IntOperands: []int32{tc.queueID, 0, 0, 0, 0},
			}
			state := Init(sf, nil, false, nil, nil)
			state.ActiveNpc = npc
			state.Pointers |= PtrActiveNpc

			err := Execute(state)
			if err == nil {
				t.Fatalf("Execute: want error, got nil")
			}
			if got := err.Error(); got != tc.wantErr {
				t.Errorf("error: got %q, want %q", got, tc.wantErr)
			}
			if len(npc.enqueueCalls) != 0 {
				t.Errorf("enqueueCalls: got %d, want 0 (enqueue must not fire on invalid id)", len(npc.enqueueCalls))
			}
		})
	}
}

// TestHandleNpcDelayWithoutActiveNpcErrors — defensive check when
// NPC_DELAY runs with no active npc anchored.
func TestHandleNpcDelayWithoutActiveNpcErrors(t *testing.T) {
	sf := &ScriptFile{
		Name:        "npc_delay_no_npc",
		Opcodes:     []Opcode{OpPushConstantInt, OpNpcDelay, OpReturn},
		IntOperands: []int32{3},
	}

	state := Init(sf, nil, false, nil, nil)
	// state.ActiveNpc intentionally left nil.

	err := Execute(state)
	if err == nil {
		t.Fatalf("Execute: want error, got nil")
	}
	want := "NPC_DELAY: no active npc"
	if got := err.Error(); got != want {
		t.Errorf("error: got %q, want %q", got, want)
	}
}

// TestHandleNpcSetTimer — NPC_SETTIMER pops interval and calls
// ActiveNpc.SetTimer. Mirrors TS NpcOps.ts:278-280.
func TestHandleNpcSetTimer(t *testing.T) {
	npc := &mockNpc{}
	sf := &ScriptFile{
		Name: "test_npc_settimer",
		Opcodes: []Opcode{
			OpPushConstantInt, // push interval (42)
			OpNpcSetTimer,
			OpReturn,
		},
		IntOperands: []int32{42, 0, 0},
	}
	state := Init(sf, nil, false, nil, nil)
	state.ActiveNpc = npc
	state.Pointers |= PtrActiveNpc

	if err := Execute(state); err != nil {
		t.Fatalf("Execute: %v", err)
	}

	if len(npc.setTimerCalls) != 1 {
		t.Fatalf("setTimerCalls: got %d, want 1", len(npc.setTimerCalls))
	}
	if npc.setTimerCalls[0] != 42 {
		t.Errorf("setTimerCalls[0]: got %d, want 42", npc.setTimerCalls[0])
	}
}

// TestHandleNpcSetTimerWithoutActiveNpcErrors — defensive nil check.
func TestHandleNpcSetTimerWithoutActiveNpcErrors(t *testing.T) {
	sf := &ScriptFile{
		Name: "npc_settimer_no_npc",
		Opcodes: []Opcode{
			OpPushConstantInt, OpNpcSetTimer, OpReturn,
		},
		IntOperands: []int32{5, 0, 0},
	}
	state := Init(sf, nil, false, nil, nil)
	// state.ActiveNpc intentionally nil.

	err := Execute(state)
	if err == nil {
		t.Fatalf("Execute: want error, got nil")
	}
	want := "NPC_SETTIMER: no active npc"
	if got := err.Error(); got != want {
		t.Errorf("error: got %q, want %q", got, want)
	}
}

// TestHandleNpcSetTimerNullRejected — interval=-1 must return error
// (S7b back-fill of NumberNotNull check). mockNpc.SetTimer must NOT be
// called. Mirrors TS NpcOps.ts:278-280 with check(..., NumberNotNull).
func TestHandleNpcSetTimerNullRejected(t *testing.T) {
	npc := &mockNpc{}
	sf := &ScriptFile{
		Name: "npc_settimer_null",
		Opcodes: []Opcode{
			OpPushConstantInt, // push interval (-1)
			OpNpcSetTimer,
			OpReturn,
		},
		IntOperands: []int32{-1, 0, 0},
	}
	state := Init(sf, nil, false, nil, nil)
	state.ActiveNpc = npc
	state.Pointers |= PtrActiveNpc

	err := Execute(state)
	if err == nil {
		t.Fatalf("Execute: want error for interval=-1, got nil")
	}
	want := "NPC_SETTIMER: input number was null(-1)"
	if !strings.Contains(err.Error(), want) {
		t.Errorf("error: got %q, want contains %q", err.Error(), want)
	}
	if len(npc.setTimerCalls) != 0 {
		t.Errorf("SetTimer should not be called on null input, got %d calls", len(npc.setTimerCalls))
	}
}

// TestHandleNpcSetHunt — NPC_SETHUNT pops range and calls
// ActiveNpc.SetHuntRange. Mirrors TS NpcOps.ts:174-176.
func TestHandleNpcSetHunt(t *testing.T) {
	npc := &mockNpc{}
	sf := &ScriptFile{
		Name: "test_npc_sethunt",
		Opcodes: []Opcode{
			OpPushConstantInt, // push range (15)
			OpNpcSetHunt,
			OpReturn,
		},
		IntOperands: []int32{15, 0, 0},
	}
	state := Init(sf, nil, false, nil, nil)
	state.ActiveNpc = npc
	state.Pointers |= PtrActiveNpc

	if err := Execute(state); err != nil {
		t.Fatalf("Execute: %v", err)
	}

	if len(npc.setHuntRangeCalls) != 1 {
		t.Fatalf("setHuntRangeCalls: got %d, want 1", len(npc.setHuntRangeCalls))
	}
	if npc.setHuntRangeCalls[0] != 15 {
		t.Errorf("setHuntRangeCalls[0]: got %d, want 15", npc.setHuntRangeCalls[0])
	}
}

// TestHandleNpcSetHuntWithoutActiveNpcErrors — defensive nil check.
func TestHandleNpcSetHuntWithoutActiveNpcErrors(t *testing.T) {
	sf := &ScriptFile{
		Name: "npc_sethunt_no_npc",
		Opcodes: []Opcode{
			OpPushConstantInt, OpNpcSetHunt, OpReturn,
		},
		IntOperands: []int32{5, 0, 0},
	}
	state := Init(sf, nil, false, nil, nil)
	// state.ActiveNpc intentionally nil.

	err := Execute(state)
	if err == nil {
		t.Fatalf("Execute: want error, got nil")
	}
	want := "NPC_SETHUNT: no active npc"
	if got := err.Error(); got != want {
		t.Errorf("error: got %q, want %q", got, want)
	}
}

// TestHandleNpcSetHuntMode — NPC_SETHUNTMODE with both positive and
// -1 (clear) values. Mirrors TS NpcOps.ts:178-185.
func TestHandleNpcSetHuntMode(t *testing.T) {
	npc := &mockNpc{}
	sf := &ScriptFile{
		Name: "test_npc_sethuntmode",
		Opcodes: []Opcode{
			OpPushConstantInt, // push mode (3)
			OpNpcSetHuntMode,
			OpPushConstantInt, // push mode (-1, clear)
			OpNpcSetHuntMode,
			OpReturn,
		},
		IntOperands: []int32{3, 0, -1, 0, 0},
	}
	state := Init(sf, nil, false, nil, nil)
	state.ActiveNpc = npc
	state.Pointers |= PtrActiveNpc

	if err := Execute(state); err != nil {
		t.Fatalf("Execute: %v", err)
	}

	if len(npc.setHuntModeCalls) != 2 {
		t.Fatalf("setHuntModeCalls: got %d, want 2", len(npc.setHuntModeCalls))
	}
	if npc.setHuntModeCalls[0] != 3 {
		t.Errorf("setHuntModeCalls[0]: got %d, want 3", npc.setHuntModeCalls[0])
	}
	if npc.setHuntModeCalls[1] != -1 {
		t.Errorf("setHuntModeCalls[1]: got %d, want -1 (clear)", npc.setHuntModeCalls[1])
	}
}

// TestHandleNpcSetHuntModeWithoutActiveNpcErrors — defensive nil check.
func TestHandleNpcSetHuntModeWithoutActiveNpcErrors(t *testing.T) {
	sf := &ScriptFile{
		Name: "npc_sethuntmode_no_npc",
		Opcodes: []Opcode{
			OpPushConstantInt, OpNpcSetHuntMode, OpReturn,
		},
		IntOperands: []int32{3, 0, 0},
	}
	state := Init(sf, nil, false, nil, nil)

	err := Execute(state)
	if err == nil {
		t.Fatalf("Execute: want error, got nil")
	}
	want := "NPC_SETHUNTMODE: no active npc"
	if got := err.Error(); got != want {
		t.Errorf("error: got %q, want %q", got, want)
	}
}

// --- S7f Task 2: NPC_FIND handler tests --------------------------------

// newNpcFindState constructs a ScriptState with IntOperands[PC]=operand,
// pushes (coord, npcTypeID, distance, huntvis) onto the int stack in
// the order the handler expects, wires a Configs that treats every id
// in loaded as a valid NpcType, and binds a mockNpcLookup as state.Npcs.
func newNpcFindState(t *testing.T, operand int32, coord, npcTypeID, distance, huntvis int, loaded map[int]bool, lookup *mockNpcLookup) *ScriptState {
	t.Helper()
	s := &ScriptState{
		Script:      &ScriptFile{IntOperands: []int32{operand}},
		PC:          0,
		Configs:     newTestConfigsWithNpcTypes(loaded),
		Npcs:        lookup,
		Pointers:    0,
		IntStack:    make([]int, StackCapacity),
		StringStack: make([]string, StackCapacity),
	}
	// Push in the order: coord, npcType, distance, huntvis (matches
	// TS popInts(4) = [coord, npc, distance, checkVis] where coord
	// was pushed first).
	s.PushInt(coord)
	s.PushInt(npcTypeID)
	s.PushInt(distance)
	s.PushInt(huntvis)
	return s
}

func TestNpcFind_SingleMatch(t *testing.T) {
	foundNpc := &mockNpc{typeID: 7}
	lookup := &mockNpcLookup{byType: foundNpc}
	coord := (2 << 28) | (3200 << 14) | 3300
	s := newNpcFindState(t, 0, coord, 7, 10, 0, map[int]bool{7: true}, lookup)

	if err := handleNpcFind(s); err != nil {
		t.Fatalf("handleNpcFind: %v", err)
	}
	if got := s.PopInt(); got != 1 {
		t.Errorf("push: got %d, want 1", got)
	}
	if s.ActiveNpc != foundNpc {
		t.Errorf("ActiveNpc: got %v, want %v", s.ActiveNpc, foundNpc)
	}
	if s.Pointers&PtrActiveNpc == 0 {
		t.Error("PtrActiveNpc should be set")
	}
	if lookup.byTypeCalls != 1 {
		t.Errorf("byTypeCalls: got %d, want 1", lookup.byTypeCalls)
	}
	// Cross-check: handler passed (level, x, z, dist, typeID, huntvis).
	wantArgs := []int{2, 3200, 3300, 10, 7, 0}
	if !slices.Equal(lookup.lastArgs, wantArgs) {
		t.Errorf("lastArgs: got %v, want %v", lookup.lastArgs, wantArgs)
	}
}

func TestNpcFind_NoMatch(t *testing.T) {
	lookup := &mockNpcLookup{byType: nil} // no match
	coord := (0 << 28) | (50 << 14) | 50
	s := newNpcFindState(t, 0, coord, 7, 10, 0, map[int]bool{7: true}, lookup)

	if err := handleNpcFind(s); err != nil {
		t.Fatalf("handleNpcFind: %v", err)
	}
	if got := s.PopInt(); got != 0 {
		t.Errorf("push: got %d, want 0", got)
	}
	if s.ActiveNpc != nil {
		t.Errorf("ActiveNpc should be nil, got %v", s.ActiveNpc)
	}
	if s.Pointers&PtrActiveNpc != 0 {
		t.Error("PtrActiveNpc should NOT be set on miss")
	}
}

func TestNpcFind_NilNpcLookup(t *testing.T) {
	coord := (0 << 28) | (50 << 14) | 50
	s := newNpcFindState(t, 0, coord, 7, 10, 0, map[int]bool{7: true}, nil)
	s.Npcs = nil // explicit

	if err := handleNpcFind(s); err != nil {
		t.Fatalf("handleNpcFind with nil Npcs: %v", err)
	}
	if got := s.PopInt(); got != 0 {
		t.Errorf("nil Npcs should degrade to not-found (push 0); got %d", got)
	}
	if s.Pointers&PtrActiveNpc != 0 {
		t.Error("PtrActiveNpc should NOT be set when Npcs is nil")
	}
}

// TestNpcFind_NilNpcLookupStillValidates pins the invariant that
// validators run BEFORE the nil-Npcs short-circuit. A regression that
// checked s.Npcs first and pushed 0 without validating would pass
// TestNpcFind_NilNpcLookup but fail here (passes invalid coord with
// nil Npcs and expects the validator error, not a silent push-0).
func TestNpcFind_NilNpcLookupStillValidates(t *testing.T) {
	s := newNpcFindState(t, 0, -1, 7, 10, 0, map[int]bool{7: true}, nil)
	s.Npcs = nil

	if err := handleNpcFind(s); err == nil {
		t.Fatal("expected validator error for coord=-1 even with nil Npcs")
	} else if !strings.Contains(err.Error(), "NPC_FIND: coord out of range") {
		t.Errorf("wrong error: %v", err)
	}
}

func TestNpcFind_IntOperandZero(t *testing.T) {
	foundNpc := &mockNpc{typeID: 7}
	lookup := &mockNpcLookup{byType: foundNpc}
	s := newNpcFindState(t, 0, 0, 7, 10, 0, map[int]bool{7: true}, lookup)

	if err := handleNpcFind(s); err != nil {
		t.Fatal(err)
	}
	if s.ActiveNpc != foundNpc {
		t.Errorf("operand=0 should set ActiveNpc, got %v", s.ActiveNpc)
	}
	if s.OtherActiveNpc != nil {
		t.Errorf("operand=0 should leave OtherActiveNpc nil, got %v", s.OtherActiveNpc)
	}
	if s.Pointers&PtrActiveNpc == 0 {
		t.Error("operand=0 should set PtrActiveNpc")
	}
	if s.Pointers&PtrActiveNpc2 != 0 {
		t.Error("operand=0 should NOT set PtrActiveNpc2")
	}
}

func TestNpcFind_IntOperandOne(t *testing.T) {
	foundNpc := &mockNpc{typeID: 7}
	lookup := &mockNpcLookup{byType: foundNpc}
	s := newNpcFindState(t, 1, 0, 7, 10, 0, map[int]bool{7: true}, lookup)

	if err := handleNpcFind(s); err != nil {
		t.Fatal(err)
	}
	if s.OtherActiveNpc != foundNpc {
		t.Errorf("operand=1 should set OtherActiveNpc, got %v", s.OtherActiveNpc)
	}
	if s.ActiveNpc != nil {
		t.Errorf("operand=1 should leave ActiveNpc nil, got %v", s.ActiveNpc)
	}
	if s.Pointers&PtrActiveNpc2 == 0 {
		t.Error("operand=1 should set PtrActiveNpc2")
	}
	if s.Pointers&PtrActiveNpc != 0 {
		t.Error("operand=1 should NOT set PtrActiveNpc")
	}
}

func TestNpcFind_InvalidCoord(t *testing.T) {
	lookup := &mockNpcLookup{}
	s := newNpcFindState(t, 0, -1, 7, 10, 0, map[int]bool{7: true}, lookup)
	if err := handleNpcFind(s); err == nil {
		t.Fatal("expected error for coord=-1")
	} else if !strings.Contains(err.Error(), "NPC_FIND: coord out of range") {
		t.Errorf("wrong error: %v", err)
	}
	if lookup.byTypeCalls != 0 {
		t.Errorf("lookup should NOT be called on validator failure; calls=%d", lookup.byTypeCalls)
	}
}

func TestNpcFind_InvalidNpcType(t *testing.T) {
	lookup := &mockNpcLookup{}
	s := newNpcFindState(t, 0, 0, 999, 10, 0, map[int]bool{7: true}, lookup) // 999 not loaded
	if err := handleNpcFind(s); err == nil {
		t.Fatal("expected error for unloaded npcType")
	} else if !strings.Contains(err.Error(), "NPC_FIND: no NpcType") {
		t.Errorf("wrong error: %v", err)
	}
	if lookup.byTypeCalls != 0 {
		t.Errorf("lookup should NOT be called; calls=%d", lookup.byTypeCalls)
	}
}

func TestNpcFind_NullDistance(t *testing.T) {
	lookup := &mockNpcLookup{}
	s := newNpcFindState(t, 0, 0, 7, -1, 0, map[int]bool{7: true}, lookup)
	if err := handleNpcFind(s); err == nil {
		t.Fatal("expected error for distance=-1 (NumberNotNull)")
	} else if !strings.Contains(err.Error(), "NPC_FIND") {
		t.Errorf("error should carry op prefix: %v", err)
	}
	if lookup.byTypeCalls != 0 {
		t.Errorf("lookup should NOT be called; calls=%d", lookup.byTypeCalls)
	}
}

func TestNpcFind_InvalidHuntVis(t *testing.T) {
	lookup := &mockNpcLookup{}
	s := newNpcFindState(t, 0, 0, 7, 10, 3, map[int]bool{7: true}, lookup) // 3 out of range
	if err := handleNpcFind(s); err == nil {
		t.Fatal("expected error for huntvis=3")
	} else if !strings.Contains(err.Error(), "NPC_FIND: huntvis out of range") {
		t.Errorf("wrong error: %v", err)
	}
	if lookup.byTypeCalls != 0 {
		t.Errorf("lookup should NOT be called; calls=%d", lookup.byTypeCalls)
	}
}

// --- S7f Task 2: NPC_FINDCAT handler tests -----------------------------

// newNpcFindCatState is the NPC_FINDCAT analogue of newNpcFindState.
// Pushes (coord, category, distance, huntvis). Loaded is the NpcType map
// — NPC_FINDCAT does NOT validate NpcType (it validates CategoryType)
// but the ScriptState still needs a Configs field.
func newNpcFindCatState(t *testing.T, operand int32, coord, category, distance, huntvis int, loaded map[int]bool, lookup *mockNpcLookup) *ScriptState {
	t.Helper()
	s := &ScriptState{
		Script:      &ScriptFile{IntOperands: []int32{operand}},
		PC:          0,
		Configs:     newTestConfigsWithNpcTypes(loaded),
		Npcs:        lookup,
		Pointers:    0,
		IntStack:    make([]int, StackCapacity),
		StringStack: make([]string, StackCapacity),
	}
	s.PushInt(coord)
	s.PushInt(category)
	s.PushInt(distance)
	s.PushInt(huntvis)
	return s
}

func TestNpcFindCat_SingleMatch(t *testing.T) {
	foundNpc := &mockNpc{typeID: 12}
	lookup := &mockNpcLookup{byCategory: foundNpc}
	coord := (1 << 28) | (1000 << 14) | 1000
	s := newNpcFindCatState(t, 0, coord, 5, 15, 1, nil, lookup)

	if err := handleNpcFindCat(s); err != nil {
		t.Fatalf("handleNpcFindCat: %v", err)
	}
	if got := s.PopInt(); got != 1 {
		t.Errorf("push: got %d, want 1", got)
	}
	if s.ActiveNpc != foundNpc {
		t.Error("ActiveNpc should be the found NPC")
	}
	if s.Pointers&PtrActiveNpc == 0 {
		t.Error("PtrActiveNpc should be set on hit")
	}
	if lookup.byCategoryCalls != 1 {
		t.Errorf("byCategoryCalls: got %d, want 1", lookup.byCategoryCalls)
	}
	wantArgs := []int{1, 1000, 1000, 15, 5, 1} // level, x, z, dist, cat, huntvis
	if !slices.Equal(lookup.lastArgs, wantArgs) {
		t.Errorf("lastArgs: got %v, want %v", lookup.lastArgs, wantArgs)
	}
}

func TestNpcFindCat_NoMatch(t *testing.T) {
	lookup := &mockNpcLookup{byCategory: nil}
	s := newNpcFindCatState(t, 0, 0, 5, 10, 0, nil, lookup)

	if err := handleNpcFindCat(s); err != nil {
		t.Fatal(err)
	}
	if got := s.PopInt(); got != 0 {
		t.Errorf("push: got %d, want 0", got)
	}
}

func TestNpcFindCat_NullCategory(t *testing.T) {
	lookup := &mockNpcLookup{}
	s := newNpcFindCatState(t, 0, 0, -1, 10, 0, nil, lookup)

	if err := handleNpcFindCat(s); err == nil {
		t.Fatal("expected error for category=-1")
	} else if !strings.Contains(err.Error(), "NPC_FINDCAT: category null(-1)") {
		t.Errorf("wrong error: %v", err)
	}
	if lookup.byCategoryCalls != 0 {
		t.Errorf("lookup should NOT be called; calls=%d", lookup.byCategoryCalls)
	}
}

// TestNpcFindCat_PartialValidatorAcceptsNonNegative pins S7f-D3:
// checkCategoryType accepts any non-(-1) value even if no CategoryType
// count is loaded. The handler MUST call the lookup with the raw cat.
func TestNpcFindCat_PartialValidatorAcceptsNonNegative(t *testing.T) {
	foundNpc := &mockNpc{typeID: 12}
	lookup := &mockNpcLookup{byCategory: foundNpc}
	s := newNpcFindCatState(t, 0, 0, 999999, 10, 0, nil, lookup)

	if err := handleNpcFindCat(s); err != nil {
		t.Fatalf("partial validator should accept 999999 (S7f-D3): %v", err)
	}
	if lookup.byCategoryCalls != 1 {
		t.Errorf("byCategoryCalls: got %d, want 1", lookup.byCategoryCalls)
	}
}

// --- S7f Task 2: NPC_FINDEXACT handler tests ---------------------------

// newNpcFindExactState pushes (coord, npcTypeID) — only 2 args.
func newNpcFindExactState(t *testing.T, operand int32, coord, npcTypeID int, loaded map[int]bool, lookup *mockNpcLookup) *ScriptState {
	t.Helper()
	s := &ScriptState{
		Script:      &ScriptFile{IntOperands: []int32{operand}},
		PC:          0,
		Configs:     newTestConfigsWithNpcTypes(loaded),
		Npcs:        lookup,
		Pointers:    0,
		IntStack:    make([]int, StackCapacity),
		StringStack: make([]string, StackCapacity),
	}
	s.PushInt(coord)
	s.PushInt(npcTypeID)
	return s
}

func TestNpcFindExact_Match(t *testing.T) {
	foundNpc := &mockNpc{typeID: 7}
	lookup := &mockNpcLookup{atCoord: foundNpc}
	coord := (0 << 28) | (3200 << 14) | 3300
	s := newNpcFindExactState(t, 0, coord, 7, map[int]bool{7: true}, lookup)

	if err := handleNpcFindExact(s); err != nil {
		t.Fatalf("handleNpcFindExact: %v", err)
	}
	if got := s.PopInt(); got != 1 {
		t.Errorf("push: got %d, want 1", got)
	}
	if s.ActiveNpc != foundNpc {
		t.Error("ActiveNpc should be the found NPC")
	}
	if s.Pointers&PtrActiveNpc == 0 {
		t.Error("PtrActiveNpc should be set on hit")
	}
	if lookup.atCoordCalls != 1 {
		t.Errorf("atCoordCalls: got %d, want 1", lookup.atCoordCalls)
	}
	wantArgs := []int{0, 3200, 3300, 7}
	if !slices.Equal(lookup.lastArgs, wantArgs) {
		t.Errorf("lastArgs: got %v, want %v", lookup.lastArgs, wantArgs)
	}
}

func TestNpcFindExact_NoNpcAtCoord(t *testing.T) {
	lookup := &mockNpcLookup{atCoord: nil}
	s := newNpcFindExactState(t, 0, 0, 7, map[int]bool{7: true}, lookup)

	if err := handleNpcFindExact(s); err != nil {
		t.Fatal(err)
	}
	if got := s.PopInt(); got != 0 {
		t.Errorf("push: got %d, want 0", got)
	}
}

func TestNpcFindExact_InvalidCoord(t *testing.T) {
	lookup := &mockNpcLookup{}
	s := newNpcFindExactState(t, 0, -1, 7, map[int]bool{7: true}, lookup)

	if err := handleNpcFindExact(s); err == nil {
		t.Fatal("expected error for coord=-1")
	} else if !strings.Contains(err.Error(), "NPC_FINDEXACT: coord out of range") {
		t.Errorf("wrong error: %v", err)
	}
	if lookup.atCoordCalls != 0 {
		t.Errorf("lookup should NOT be called; calls=%d", lookup.atCoordCalls)
	}
}

// TestHandleNpcDelayNullRejected pins NAI-20 Task 4: NPC_DELAY rejects
// ticks=-1 via checkNotNull (TS NumberNotNull). Mirrors TS NpcOps.ts
// NPC_DELAY shape and the S7b back-fill on NPC_SETTIMER.
func TestHandleNpcDelayNullRejected(t *testing.T) {
	npc := &mockNpc{}
	sf := &ScriptFile{
		Name: "npc_delay_null",
		Opcodes: []Opcode{
			OpPushConstantInt, // push ticks (-1)
			OpNpcDelay,
			OpReturn,
		},
		IntOperands: []int32{-1, 0, 0},
	}
	state := Init(sf, nil, false, nil, nil)
	state.ActiveNpc = npc
	state.Pointers |= PtrActiveNpc

	err := Execute(state)
	if err == nil {
		t.Fatalf("Execute: want error for ticks=-1, got nil")
	}
	want := "NPC_DELAY: input number was null(-1)"
	if !strings.Contains(err.Error(), want) {
		t.Errorf("error: got %q, want substring %q", err.Error(), want)
	}
	if len(npc.setDelayedCalls) != 0 {
		t.Errorf("setDelayedCalls: got %d, want 0 (must not call on rejection)",
			len(npc.setDelayedCalls))
	}
}

// TestHandleNpcQueueNullDelayRejected pins NAI-20 Task 4: NPC_QUEUE
// rejects delay=-1 via checkNotNull. The queueId 1..20 range check is
// orthogonal (covered by TestHandleNpcQueueInvalidQueueIDErrors).
func TestHandleNpcQueueNullDelayRejected(t *testing.T) {
	npc := &mockNpc{}
	// Pop order: delay (top), arg, queueID (bottom). IntOperands push
	// in left-to-right order; the rightmost int is on top of the stack
	// when the opcode runs. So we want delay on top → IntOperands ends
	// with -1.
	sf := &ScriptFile{
		Name: "npc_queue_null_delay",
		Opcodes: []Opcode{
			OpPushConstantInt, // push queueID (5)
			OpPushConstantInt, // push arg (0)
			OpPushConstantInt, // push delay (-1)
			OpNpcQueue,
			OpReturn,
		},
		IntOperands: []int32{5, 0, -1, 0, 0},
	}
	state := Init(sf, nil, false, nil, nil)
	state.ActiveNpc = npc
	state.Pointers |= PtrActiveNpc

	err := Execute(state)
	if err == nil {
		t.Fatalf("Execute: want error for delay=-1, got nil")
	}
	want := "NPC_QUEUE: input number was null(-1)"
	if !strings.Contains(err.Error(), want) {
		t.Errorf("error: got %q, want substring %q", err.Error(), want)
	}
	if len(npc.enqueueCalls) != 0 {
		t.Errorf("enqueueCalls: got %d, want 0 (must not enqueue on rejection)",
			len(npc.enqueueCalls))
	}
}

// TestHandleNpcSetHuntNullRejected pins NAI-20 Task 4: NPC_SETHUNT
// rejects range=-1 via checkNotNull (TS NumberNotNull). Mirrors TS
// NpcOps.ts:174-176.
func TestHandleNpcSetHuntNullRejected(t *testing.T) {
	npc := &mockNpc{}
	sf := &ScriptFile{
		Name: "npc_sethunt_null",
		Opcodes: []Opcode{
			OpPushConstantInt, // push range (-1)
			OpNpcSetHunt,
			OpReturn,
		},
		IntOperands: []int32{-1, 0, 0},
	}
	state := Init(sf, nil, false, nil, nil)
	state.ActiveNpc = npc
	state.Pointers |= PtrActiveNpc

	err := Execute(state)
	if err == nil {
		t.Fatalf("Execute: want error for range=-1, got nil")
	}
	want := "NPC_SETHUNT: input number was null(-1)"
	if !strings.Contains(err.Error(), want) {
		t.Errorf("error: got %q, want substring %q", err.Error(), want)
	}
	if len(npc.setHuntRangeCalls) != 0 {
		t.Errorf("setHuntRangeCalls: got %d, want 0 (must not call on rejection)",
			len(npc.setHuntRangeCalls))
	}
}

func TestNpcFindExact_InvalidNpcType(t *testing.T) {
	lookup := &mockNpcLookup{}
	s := newNpcFindExactState(t, 0, 0, 999, map[int]bool{7: true}, lookup)

	if err := handleNpcFindExact(s); err == nil {
		t.Fatal("expected error for unloaded npcType")
	} else if !strings.Contains(err.Error(), "NPC_FINDEXACT: no NpcType") {
		t.Errorf("wrong error: %v", err)
	}
	if lookup.atCoordCalls != 0 {
		t.Errorf("lookup should NOT be called; calls=%d", lookup.atCoordCalls)
	}
}

// TestHandleNpcHasOpNullRejected pins NAI-23 Bundle 4a: NPC_HASOP rejects
// op=-1 via checkNotNull (TS NpcOps.ts NPC_HASOP: check(op, NumberNotNull)).
// The TS handler explicitly wraps op with NumberNotNull before the op-slot
// lookup, so -1 must be rejected before any side-effects.
func TestHandleNpcHasOpNullRejected(t *testing.T) {
	mc := newTestConfigs()
	mc.npcs[7] = &objtype.NpcType{
		ConfigType: objtype.ConfigType{ID: 7},
		Op:         []string{"Attack", "", "", "", ""},
	}
	sf := &ScriptFile{
		Name: "npc_hasop_null",
		Opcodes: []Opcode{
			OpPushConstantInt, // push op (-1)
			OpNpcHasOp,
			OpReturn,
		},
		IntOperands: []int32{-1, 0, 0},
	}
	state := Init(sf, nil, false, nil, nil)
	state.Configs = mc
	state.ActiveNpc = &mockNpc{typeID: 7}
	state.Pointers |= PtrActiveNpc

	err := Execute(state)
	if err == nil {
		t.Fatalf("Execute: want error for input=-1, got nil")
	}
	want := "NPC_HASOP: input number was null(-1)"
	if !strings.Contains(err.Error(), want) {
		t.Errorf("error: got %q, want substring %q", err.Error(), want)
	}
}

// TestHandleNpcAnimNullDelayRejected pins NAI-23 Bundle 4a: NPC_ANIM rejects
// delay=-1 via checkNotNull (TS NpcOps.ts NPC_ANIM: check(delay, NumberNotNull)).
// seq is NOT wrapped per TS; only delay must be rejected. The Animate side-
// effect must NOT occur when delay is null.
func TestHandleNpcAnimNullDelayRejected(t *testing.T) {
	npc := &mockNpc{}
	// Pop order: delay (top), id. Push id=42 first, then delay=-1 on top.
	sf := &ScriptFile{
		Name: "npc_anim_null_delay",
		Opcodes: []Opcode{
			OpPushConstantInt, // push id (42)
			OpPushConstantInt, // push delay (-1)
			OpNpcAnim,
			OpReturn,
		},
		IntOperands: []int32{42, -1, 0, 0},
	}
	state := Init(sf, nil, false, nil, nil)
	state.ActiveNpc = npc
	state.Pointers |= PtrActiveNpc

	err := Execute(state)
	if err == nil {
		t.Fatalf("Execute: want error for delay=-1, got nil")
	}
	want := "NPC_ANIM: input number was null(-1)"
	if !strings.Contains(err.Error(), want) {
		t.Errorf("error: got %q, want substring %q", err.Error(), want)
	}
	if len(npc.animCalls) != 0 {
		t.Errorf("animCalls: got %d, want 0 (must not animate on rejection)", len(npc.animCalls))
	}
}

// TestHandleNpcDamageNullAmountRejected pins NAI-23 Bundle 4a: NPC_DAMAGE
// rejects amount=-1 via checkNotNull (TS NpcOps.ts NPC_DAMAGE:
// check(amount, NumberNotNull)). dmgType is wrapped with HitTypeValid (not
// NumberNotNull) and stays raw here. The Damage side-effect must NOT occur.
func TestHandleNpcDamageNullAmountRejected(t *testing.T) {
	npc := &mockNpc{}
	// Pop order: amount (top), dmgType. Push dmgType=1 first, then amount=-1.
	sf := &ScriptFile{
		Name: "npc_damage_null_amount",
		Opcodes: []Opcode{
			OpPushConstantInt, // push dmgType (1)
			OpPushConstantInt, // push amount (-1)
			OpNpcDamage,
			OpReturn,
		},
		IntOperands: []int32{1, -1, 0, 0},
	}
	state := Init(sf, nil, false, nil, nil)
	state.ActiveNpc = npc
	state.Pointers |= PtrActiveNpc

	err := Execute(state)
	if err == nil {
		t.Fatalf("Execute: want error for amount=-1, got nil")
	}
	want := "NPC_DAMAGE: input number was null(-1)"
	if !strings.Contains(err.Error(), want) {
		t.Errorf("error: got %q, want substring %q", err.Error(), want)
	}
	if len(npc.damageCalls) != 0 {
		t.Errorf("damageCalls: got %d, want 0 (must not damage on rejection)", len(npc.damageCalls))
	}
}

// --- NAI-33 Task 9: NPC_FINDALLANY handler tests -----------------------

// newNpcFindAllAnyState pushes (coord, distance, checkVis) — TS popInts(3) order.
func newNpcFindAllAnyState(t *testing.T, coord, distance, huntvis int, lookup *mockNpcLookup) *ScriptState {
	t.Helper()
	mw := newMockWorld()
	mw.tick = 100
	s := &ScriptState{
		Script:      &ScriptFile{IntOperands: []int32{0}},
		PC:          0,
		Configs:     newTestConfigsWithNpcTypes(map[int]bool{}),
		Npcs:        lookup,
		World:       mw,
		IntStack:    make([]int, StackCapacity),
		StringStack: make([]string, StackCapacity),
	}
	s.PushInt(coord)
	s.PushInt(distance)
	s.PushInt(huntvis)
	return s
}

func TestNpcFindAllAny_SetsIterator(t *testing.T) {
	lookup := &mockNpcLookup{}
	coord := (2 << 28) | (3200 << 14) | 3300
	s := newNpcFindAllAnyState(t, coord, 10, 0, lookup)

	if err := handleNpcFindAllAny(s); err != nil {
		t.Fatalf("handleNpcFindAllAny: %v", err)
	}
	if s.npcIterator == nil {
		t.Fatal("npcIterator should be non-nil after FINDALLANY")
	}
	if s.npcIterator.mode != NpcIteratorDistance {
		t.Errorf("mode: got %v, want NpcIteratorDistance", s.npcIterator.mode)
	}
	if s.npcIterator.typeID != -1 {
		t.Errorf("typeID: got %d, want -1 (FINDALLANY = no type filter)", s.npcIterator.typeID)
	}
	if s.npcIterator.creationTick != 100 {
		t.Errorf("creationTick: got %d, want 100", s.npcIterator.creationTick)
	}
	if s.npcIterator.level != 2 || s.npcIterator.x != 3200 || s.npcIterator.z != 3300 {
		t.Errorf("center: got (level=%d, x=%d, z=%d)", s.npcIterator.level, s.npcIterator.x, s.npcIterator.z)
	}
	if s.npcIterator.distance != 10 {
		t.Errorf("distance: got %d, want 10", s.npcIterator.distance)
	}
	if s.ISP != 0 {
		t.Errorf("FINDALLANY should not push; ISP=%d", s.ISP)
	}
}

func TestNpcFindAllAny_PopOrder(t *testing.T) {
	// distinguishable values — if pop order is wrong, the iterator stores wrong distance.
	lookup := &mockNpcLookup{}
	coord := (0 << 28) | (3200 << 14) | 3300
	s := newNpcFindAllAnyState(t, coord, 99, 0, lookup)

	if err := handleNpcFindAllAny(s); err != nil {
		t.Fatalf("handleNpcFindAllAny: %v", err)
	}
	if s.npcIterator.distance != 99 {
		t.Errorf("distance pop order wrong: got %d, want 99", s.npcIterator.distance)
	}
	if s.npcIterator.huntvis != 0 {
		t.Errorf("huntvis pop order wrong: got %d, want 0", s.npcIterator.huntvis)
	}
}

func TestNpcFindAllAny_InvalidCoord(t *testing.T) {
	s := newNpcFindAllAnyState(t, -1, 10, 0, &mockNpcLookup{})
	if err := handleNpcFindAllAny(s); err == nil {
		t.Fatal("expected validator error for coord=-1")
	} else if !strings.Contains(err.Error(), "NPC_FINDALLANY: coord out of range") {
		t.Errorf("wrong error: %v", err)
	}
}

func TestNpcFindAllAny_NullDistance(t *testing.T) {
	coord := (0 << 28) | (3200 << 14) | 3300
	s := newNpcFindAllAnyState(t, coord, -1, 0, &mockNpcLookup{})
	if err := handleNpcFindAllAny(s); err == nil {
		t.Fatal("expected validator error for null distance")
	} else if !strings.Contains(err.Error(), "NPC_FINDALLANY") {
		t.Errorf("wrong error: %v", err)
	}
}

func TestNpcFindAllAny_InvalidHuntVis(t *testing.T) {
	coord := (0 << 28) | (3200 << 14) | 3300
	s := newNpcFindAllAnyState(t, coord, 10, 99, &mockNpcLookup{})
	if err := handleNpcFindAllAny(s); err == nil {
		t.Fatal("expected validator error for invalid huntvis")
	}
}

func TestNpcFindAllAny_NilNpcLookup(t *testing.T) {
	coord := (0 << 28) | (3200 << 14) | 3300
	s := newNpcFindAllAnyState(t, coord, 10, 0, nil)
	s.Npcs = nil
	if err := handleNpcFindAllAny(s); err != nil {
		t.Fatalf("handleNpcFindAllAny with nil Npcs: %v", err)
	}
	if s.npcIterator != nil {
		t.Error("npcIterator should remain nil when Npcs is nil (degrades to FINDNEXT push-0)")
	}
}

// --- NAI-33 Task 10: NPC_FINDALL handler tests -------------------------

// newNpcFindAllState pushes (coord, npcType, distance, huntvis) — popInts(4) order.
func newNpcFindAllState(t *testing.T, coord, npcTypeID, distance, huntvis int, loaded map[int]bool, lookup *mockNpcLookup) *ScriptState {
	t.Helper()
	mw := newMockWorld()
	mw.tick = 100
	s := &ScriptState{
		Script:      &ScriptFile{IntOperands: []int32{0}},
		PC:          0,
		Configs:     newTestConfigsWithNpcTypes(loaded),
		Npcs:        lookup,
		World:       mw,
		IntStack:    make([]int, StackCapacity),
		StringStack: make([]string, StackCapacity),
	}
	s.PushInt(coord)
	s.PushInt(npcTypeID)
	s.PushInt(distance)
	s.PushInt(huntvis)
	return s
}

func TestNpcFindAll_SetsIteratorWithTypeFilter(t *testing.T) {
	coord := (2 << 28) | (3200 << 14) | 3300
	s := newNpcFindAllState(t, coord, 7, 10, 0, map[int]bool{7: true}, &mockNpcLookup{})
	if err := handleNpcFindAll(s); err != nil {
		t.Fatalf("handleNpcFindAll: %v", err)
	}
	if s.npcIterator == nil {
		t.Fatal("npcIterator should be non-nil")
	}
	if s.npcIterator.typeID != 7 {
		t.Errorf("typeID: got %d, want 7 (FINDALL = type filter set)", s.npcIterator.typeID)
	}
	if s.npcIterator.mode != NpcIteratorDistance {
		t.Errorf("mode: got %v, want NpcIteratorDistance", s.npcIterator.mode)
	}
}

func TestNpcFindAll_InvalidNpcType(t *testing.T) {
	coord := (0 << 28) | (3200 << 14) | 3300
	s := newNpcFindAllState(t, coord, 99, 10, 0, map[int]bool{7: true}, &mockNpcLookup{})
	if err := handleNpcFindAll(s); err == nil {
		t.Fatal("expected NpcType validator error for unloaded npcTypeID")
	} else if !strings.Contains(err.Error(), "NPC_FINDALL") {
		t.Errorf("wrong error: %v", err)
	}
}

func TestNpcFindAll_NilNpcLookup(t *testing.T) {
	coord := (0 << 28) | (3200 << 14) | 3300
	s := newNpcFindAllState(t, coord, 7, 10, 0, map[int]bool{7: true}, nil)
	s.Npcs = nil
	if err := handleNpcFindAll(s); err != nil {
		t.Fatalf("handleNpcFindAll: %v", err)
	}
	if s.npcIterator != nil {
		t.Error("nil Npcs → no iterator")
	}
}

// --- NAI-33 Task 11: NPC_FINDALLZONE handler tests ---------------------

func newNpcFindAllZoneState(t *testing.T, coord int, lookup *mockNpcLookup) *ScriptState {
	t.Helper()
	mw := newMockWorld()
	mw.tick = 100
	s := &ScriptState{
		Script:      &ScriptFile{IntOperands: []int32{0}},
		PC:          0,
		Configs:     newTestConfigsWithNpcTypes(map[int]bool{}),
		Npcs:        lookup,
		World:       mw,
		IntStack:    make([]int, StackCapacity),
		StringStack: make([]string, StackCapacity),
	}
	s.PushInt(coord)
	return s
}

func TestNpcFindAllZone_SetsZoneIterator(t *testing.T) {
	coord := (2 << 28) | (3200 << 14) | 3300
	s := newNpcFindAllZoneState(t, coord, &mockNpcLookup{})
	if err := handleNpcFindAllZone(s); err != nil {
		t.Fatalf("handleNpcFindAllZone: %v", err)
	}
	if s.npcIterator == nil {
		t.Fatal("npcIterator should be non-nil")
	}
	if s.npcIterator.mode != NpcIteratorZone {
		t.Errorf("mode: got %v, want NpcIteratorZone", s.npcIterator.mode)
	}
	if s.npcIterator.level != 2 || s.npcIterator.x != 3200 || s.npcIterator.z != 3300 {
		t.Errorf("center: got (level=%d, x=%d, z=%d)", s.npcIterator.level, s.npcIterator.x, s.npcIterator.z)
	}
	if s.npcIterator.typeID != -1 {
		t.Errorf("typeID: got %d, want -1 (no filter in ZONE mode)", s.npcIterator.typeID)
	}
}

func TestNpcFindAllZone_InvalidCoord(t *testing.T) {
	s := newNpcFindAllZoneState(t, -1, &mockNpcLookup{})
	if err := handleNpcFindAllZone(s); err == nil {
		t.Fatal("expected coord validator error")
	} else if !strings.Contains(err.Error(), "NPC_FINDALLZONE: coord out of range") {
		t.Errorf("wrong error: %v", err)
	}
}

func TestNpcFindAllZone_NilNpcLookup(t *testing.T) {
	coord := (0 << 28) | (3200 << 14) | 3300
	s := newNpcFindAllZoneState(t, coord, nil)
	s.Npcs = nil
	if err := handleNpcFindAllZone(s); err != nil {
		t.Fatalf("handleNpcFindAllZone: %v", err)
	}
	if s.npcIterator != nil {
		t.Error("nil Npcs → no iterator")
	}
}

// --- NAI-33 Task 12: NPC_FINDNEXT handler tests ------------------------

func newNpcFindNextState(t *testing.T, tick int, iter *NpcIterator) *ScriptState {
	t.Helper()
	mw := newMockWorld()
	mw.tick = tick
	s := &ScriptState{
		Script:      &ScriptFile{IntOperands: []int32{0}},
		PC:          0,
		Configs:     newTestConfigsWithNpcTypes(map[int]bool{}),
		World:       mw,
		IntStack:    make([]int, StackCapacity),
		StringStack: make([]string, StackCapacity),
	}
	s.npcIterator = iter
	return s
}

func TestNpcFindNext_NilIterator(t *testing.T) {
	s := newNpcFindNextState(t, 100, nil)
	if err := handleNpcFindNext(s); err != nil {
		t.Fatalf("handleNpcFindNext: %v", err)
	}
	if got := s.PopInt(); got != 0 {
		t.Errorf("nil iterator: got push %d, want 0", got)
	}
	if s.ActiveNpc != nil {
		t.Error("ActiveNpc should not be set on nil iterator")
	}
}

func TestNpcFindNext_StaleIterator(t *testing.T) {
	npc := &mockNpc{typeID: 1, x: 3200, z: 3300, level: 0}
	zoneKey := mockZoneKey(0, 3200, 3296)
	lookup := &mockNpcLookup{byZone: map[uint64][]ActiveNpc{zoneKey: {npc}}}
	iter := NewZoneNpcIterator(lookup, 99, 0, 3200, 3300) // creationTick=99
	s := newNpcFindNextState(t, 100, iter)                // currentTick=100 (advanced)

	err := handleNpcFindNext(s)
	if err == nil {
		t.Fatal("stale iterator should return error")
	}
	if !strings.Contains(err.Error(), "tried to use an old iterator") {
		t.Errorf("wrong error message: %v", err)
	}
}

func TestNpcFindNext_HitSetsActiveNpcAndPushes1(t *testing.T) {
	npc := &mockNpc{typeID: 1, x: 3200, z: 3300, level: 0}
	zoneKey := mockZoneKey(0, 3200, 3296)
	lookup := &mockNpcLookup{byZone: map[uint64][]ActiveNpc{zoneKey: {npc}}}
	iter := NewZoneNpcIterator(lookup, 100, 0, 3200, 3300)
	s := newNpcFindNextState(t, 100, iter)

	if err := handleNpcFindNext(s); err != nil {
		t.Fatalf("handleNpcFindNext: %v", err)
	}
	if got := s.PopInt(); got != 1 {
		t.Errorf("hit: got push %d, want 1", got)
	}
	if s.ActiveNpc != npc {
		t.Errorf("ActiveNpc: got %v, want %v", s.ActiveNpc, npc)
	}
	if s.Pointers&PtrActiveNpc == 0 {
		t.Error("PtrActiveNpc should be set on hit")
	}
}

func TestNpcFindNext_ExhaustionPushes0AndDoesNotClearIterator(t *testing.T) {
	// Empty zone — first Next exhausts immediately.
	lookup := &mockNpcLookup{}
	iter := NewZoneNpcIterator(lookup, 100, 0, 3200, 3300)
	s := newNpcFindNextState(t, 100, iter)

	if err := handleNpcFindNext(s); err != nil {
		t.Fatalf("handleNpcFindNext: %v", err)
	}
	if got := s.PopInt(); got != 0 {
		t.Errorf("exhaustion: got push %d, want 0", got)
	}
	// Critical TS-fidelity quirk: iterator NOT cleared on exhaustion
	// (matches TS state.npcIterator?.next() returning {done:true} without nulling).
	if s.npcIterator == nil {
		t.Error("npcIterator should NOT be cleared on exhaustion (TS parity)")
	}
}

func TestNpcFindNext_ExhaustionThenSecondCallStillPushes0(t *testing.T) {
	// Subsequent FINDNEXT calls on exhausted iterator continue to push 0.
	lookup := &mockNpcLookup{}
	iter := NewZoneNpcIterator(lookup, 100, 0, 3200, 3300)
	s := newNpcFindNextState(t, 100, iter)

	_ = handleNpcFindNext(s)
	_ = s.PopInt() // discard first

	if err := handleNpcFindNext(s); err != nil {
		t.Fatalf("second handleNpcFindNext: %v", err)
	}
	if got := s.PopInt(); got != 0 {
		t.Errorf("second exhaustion: got push %d, want 0", got)
	}
}

// --- NAI-33 Task 14: integration test (Layer 4) ------------------------

// TestIteratorFamily_Integration_FindAllAnyThenLoopFindNext exercises the
// end-to-end binding: FINDALLANY sets s.npcIterator; subsequent FINDNEXT
// calls visit-and-yield matching NPCs from the same iterator state.
// Mirrors the [proc,check_fishing_spot_empty] use pattern.
func TestIteratorFamily_Integration_FindAllAnyThenLoopFindNext(t *testing.T) {
	npc1 := &mockNpc{typeID: 1, x: 3200, z: 3300, level: 0}
	npc2 := &mockNpc{typeID: 2, x: 3201, z: 3300, level: 0}
	zoneKey := mockZoneKey(0, 3200, 3296)
	lookup := &mockNpcLookup{byZone: map[uint64][]ActiveNpc{zoneKey: {npc1, npc2}}}

	mw := newMockWorld()
	mw.tick = 100
	s := &ScriptState{
		Script:      &ScriptFile{IntOperands: []int32{0}},
		PC:          0,
		Configs:     newTestConfigsWithNpcTypes(map[int]bool{}),
		Npcs:        lookup,
		World:       mw,
		IntStack:    make([]int, StackCapacity),
		StringStack: make([]string, StackCapacity),
	}

	// Stage 1: FINDALLANY (3 args: coord, distance, huntvis)
	coord := (0 << 28) | (3200 << 14) | 3300
	s.PushInt(coord)
	s.PushInt(5)
	s.PushInt(0)
	if err := handleNpcFindAllAny(s); err != nil {
		t.Fatalf("FINDALLANY: %v", err)
	}
	if s.npcIterator == nil {
		t.Fatal("FINDALLANY did not set iterator")
	}

	// Stage 2: FINDNEXT loop
	yielded := []ActiveNpc{}
	for i := range 5 { // bounded loop — guards against infinite-loop bugs
		if err := handleNpcFindNext(s); err != nil {
			t.Fatalf("FINDNEXT iter %d: %v", i, err)
		}
		got := s.PopInt()
		if got == 0 {
			break
		}
		yielded = append(yielded, s.ActiveNpc)
	}

	if len(yielded) != 2 {
		t.Errorf("yielded count: got %d, want 2 (npc1, npc2)", len(yielded))
	}
	// Iterator persists across FINDNEXT calls (TS-parity).
	if s.npcIterator == nil {
		t.Error("iterator should persist after exhaustion")
	}
}

// --- NAI-34 Task 3: NPC_TELE Layer 1 unit tests --------------------------

func TestNpcTele_PopsCoordValidatesAndDelegates(t *testing.T) {
	// Pack (level=2, x=3200, z=3200) into a single RS2 coord int.
	packed := (2 << 28) | (3200 << 14) | 3200
	npc := &mockNpc{}
	s := &ScriptState{
		ActiveNpc:   npc,
		IntStack:    make([]int, StackCapacity),
		StringStack: make([]string, StackCapacity),
	}
	s.PushInt(packed)
	if err := handleNpcTele(s); err != nil {
		t.Fatalf("handleNpcTele: unexpected err %v", err)
	}
	if len(npc.teleportCalls) != 1 {
		t.Fatalf("teleportCalls: got %d, want 1", len(npc.teleportCalls))
	}
	got := npc.teleportCalls[0]
	if got.x != 3200 || got.z != 3200 || got.level != 2 {
		t.Errorf("teleportCalls[0]: got (x=%d, z=%d, level=%d), want (3200, 3200, 2)", got.x, got.z, got.level)
	}
}

func TestNpcTele_NoActiveNpcErrors(t *testing.T) {
	s := &ScriptState{
		ActiveNpc:   nil,
		IntStack:    make([]int, StackCapacity),
		StringStack: make([]string, StackCapacity),
	}
	s.PushInt(0)
	err := handleNpcTele(s)
	if err == nil {
		t.Fatal("handleNpcTele: expected error for nil ActiveNpc, got nil")
	}
	if !strings.Contains(err.Error(), "NPC_TELE: no active npc") {
		t.Errorf("err: got %q, want substring %q", err.Error(), "NPC_TELE: no active npc")
	}
}

// --- NAI-36 Task 2: NPC_WALK Layer 1 unit tests --------------------------

func TestNpcWalk_PopsCoordValidatesAndDelegates(t *testing.T) {
	npc := &mockNpc{}
	mc := &mockConfigs{}

	// coord pack(level=2, x=3200, z=3300)
	const level, x, z = 2, 3200, 3300
	coord := (level << 28) | (x << 14) | z

	state := runNpcOp(t, npc, mc, OpNpcWalk, []int{coord})
	_ = state

	if len(npc.queueWaypointCalls) != 1 {
		t.Fatalf("queueWaypointCalls: got %d, want 1", len(npc.queueWaypointCalls))
	}
	got := npc.queueWaypointCalls[0]
	if got.x != x || got.z != z {
		t.Errorf("queueWaypointCalls[0]: got (x=%d, z=%d), want (x=%d, z=%d)",
			got.x, got.z, x, z)
	}
}

// TS-asymmetry pin per ts_asymmetry_dual_pin.md — pin presence (QueueWaypoint
// called with x/z) AND conspicuous absence (level discarded TS-faithfully —
// no Teleport call, no level path). Escalates if upstream TS adds a level
// argument to NPC_WALK in a future fix.
func TestNpcWalk_DiscardsLevelTSFaithfully(t *testing.T) {
	npc := &mockNpc{}
	mc := &mockConfigs{}

	// coord pack(level=3, x=3200, z=3300) — non-zero level
	const x, z = 3200, 3300
	coord := (3 << 28) | (x << 14) | z

	_ = runNpcOp(t, npc, mc, OpNpcWalk, []int{coord})

	// Presence: QueueWaypoint called.
	if len(npc.queueWaypointCalls) != 1 {
		t.Fatalf("queueWaypointCalls: got %d, want 1", len(npc.queueWaypointCalls))
	}
	// Conspicuous absence: Teleport NOT called (no 3-arg level path).
	if len(npc.teleportCalls) != 0 {
		t.Errorf("teleportCalls: got %d, want 0 (NPC_WALK must not Teleport — level is dropped TS-faithfully)",
			len(npc.teleportCalls))
	}
}

func TestNpcWalk_NoActiveNpcErrors(t *testing.T) {
	state := &ScriptState{
		IntStack:    make([]int, StackCapacity),
		StringStack: make([]string, StackCapacity),
	}
	state.PushInt((1 << 28) | (3200 << 14) | 3300)

	err := handleNpcWalk(state)
	if err == nil || !strings.Contains(err.Error(), "no active npc") {
		t.Errorf("handleNpcWalk with no active npc: got %v, want error containing 'no active npc'", err)
	}
}

func TestNpcTele_InvalidCoordErrors(t *testing.T) {
	npc := &mockNpc{}
	s := &ScriptState{
		ActiveNpc:   npc,
		IntStack:    make([]int, StackCapacity),
		StringStack: make([]string, StackCapacity),
	}
	s.PushInt(-1)
	err := handleNpcTele(s)
	if err == nil {
		t.Fatal("handleNpcTele: expected error for coord=-1, got nil")
	}
	if !strings.Contains(err.Error(), "NPC_TELE: coord out of range") {
		t.Errorf("err: got %q, want substring %q", err.Error(), "NPC_TELE: coord out of range")
	}
	if len(npc.teleportCalls) != 0 {
		t.Errorf("teleportCalls on error path: got %d, want 0 (handler must reject before delegating)", len(npc.teleportCalls))
	}
}

func TestNpcTele_PopOrderIsSinglePopInt(t *testing.T) {
	// Push two ints; verify the handler pops exactly 1 (the top one — packed coord).
	npc := &mockNpc{}
	s := &ScriptState{
		ActiveNpc:   npc,
		IntStack:    make([]int, StackCapacity),
		StringStack: make([]string, StackCapacity),
	}
	s.PushInt(0xCAFE)                          // bottom — should remain after handler
	s.PushInt((0 << 28) | (3200 << 14) | 3200) // top — packed coord, gets popped
	if err := handleNpcTele(s); err != nil {
		t.Fatalf("handleNpcTele: unexpected err %v", err)
	}
	// Verify remaining stack depth — exactly 1 int left (the 0xCAFE sentinel).
	if got := s.PopInt(); got != 0xCAFE {
		t.Errorf("residual stack top: got %d, want 0xCAFE — handler popped wrong number of ints", got)
	}
}

func TestNpcTele_DispatchRoutes(t *testing.T) {
	// Integration test — exercises the dispatch table to confirm
	// OpNpcTele is routed to handleNpcTele. If dispatch is unset,
	// runNpcOp's internal t.Fatalf fires on the unknown-opcode error
	// returned by Execute (per runner_test.go:8 convention) before
	// reaching the assertions below.
	npc := &mockNpc{}
	packed := (0 << 28) | (3200 << 14) | 3200
	state := runNpcOp(t, npc, nil, OpNpcTele, []int{packed})
	if len(npc.teleportCalls) != 1 {
		t.Fatalf("teleportCalls after dispatch: got %d, want 1 (handler ran but didn't delegate to mock)", len(npc.teleportCalls))
	}
	got := npc.teleportCalls[0]
	if got.x != 3200 || got.z != 3200 || got.level != 0 {
		t.Errorf("teleportCalls[0]: got (x=%d, z=%d, level=%d), want (3200, 3200, 0)", got.x, got.z, got.level)
	}
	// Confirm the script reached normal completion (Finished), not Aborted.
	if state.Execution != Finished {
		t.Errorf("state.Execution after NPC_TELE: got %v, want Finished", state.Execution)
	}
}

// --- NAI-35-T3: NPC_HUNTALL handler tests ------------------------------

// newNpcHuntAllState pushes (coord, distance, huntvis) — popInts(3) order
// matching TS NpcOps.ts:325-333.
func newNpcHuntAllState(t *testing.T, coord, distance, huntvis int, lookup *mockNpcLookup) *ScriptState {
	t.Helper()
	mw := newMockWorld()
	mw.tick = 100
	s := &ScriptState{
		Script:      &ScriptFile{IntOperands: []int32{0}},
		PC:          0,
		Configs:     newTestConfigsWithNpcTypes(map[int]bool{}),
		Npcs:        lookup,
		World:       mw,
		IntStack:    make([]int, StackCapacity),
		StringStack: make([]string, StackCapacity),
	}
	s.PushInt(coord)
	s.PushInt(distance)
	s.PushInt(huntvis)
	return s
}

func TestHandleNpcHuntAll_StoresHuntAllIterator(t *testing.T) {
	coord := (2 << 28) | (3200 << 14) | 3300
	s := newNpcHuntAllState(t, coord, 10, objtype.HuntVisLineOfSight, &mockNpcLookup{})
	if err := handleNpcHuntAll(s); err != nil {
		t.Fatalf("handleNpcHuntAll: %v", err)
	}
	if s.npcIterator == nil {
		t.Fatal("npcIterator should be non-nil after NPC_HUNTALL")
	}
	if s.npcIterator.mode != NpcIteratorHuntAll {
		t.Errorf("mode: got %v, want NpcIteratorHuntAll", s.npcIterator.mode)
	}
	if s.npcIterator.huntvis != objtype.HuntVisLineOfSight {
		t.Errorf("huntvis: got %d, want HuntVisLineOfSight (%d)", s.npcIterator.huntvis, objtype.HuntVisLineOfSight)
	}
	if s.npcIterator.creationTick != 100 {
		t.Errorf("creationTick: got %d, want 100 (from World.CurrentTick)", s.npcIterator.creationTick)
	}
	if s.npcIterator.level != 2 || s.npcIterator.x != 3200 || s.npcIterator.z != 3300 {
		t.Errorf("center: got (level=%d, x=%d, z=%d), want (2, 3200, 3300)",
			s.npcIterator.level, s.npcIterator.x, s.npcIterator.z)
	}
	if s.npcIterator.distance != 10 {
		t.Errorf("distance: got %d, want 10", s.npcIterator.distance)
	}
	if s.npcIterator.typeID != -1 {
		t.Errorf("typeID: got %d, want -1 (HuntAll has no type filter)", s.npcIterator.typeID)
	}
	if s.ISP != 0 {
		t.Errorf("NPC_HUNTALL should not push; ISP=%d", s.ISP)
	}
}

func TestHandleNpcHuntAll_NilNpcsDegrades(t *testing.T) {
	coord := (0 << 28) | (3200 << 14) | 3300
	s := newNpcHuntAllState(t, coord, 10, objtype.HuntVisOff, nil)
	s.Npcs = nil
	if err := handleNpcHuntAll(s); err != nil {
		t.Fatalf("handleNpcHuntAll with nil Npcs: %v", err)
	}
	if s.npcIterator != nil {
		t.Error("npcIterator should remain nil when Npcs is nil (degrades to FINDNEXT push-0)")
	}
}

func TestHandleNpcHuntAll_InvalidHuntVisRejected(t *testing.T) {
	coord := (0 << 28) | (3200 << 14) | 3300
	s := newNpcHuntAllState(t, coord, 10, 99, &mockNpcLookup{})
	if err := handleNpcHuntAll(s); err == nil {
		t.Fatal("expected validator error for invalid huntvis=99")
	} else if !strings.Contains(err.Error(), "NPC_HUNTALL") {
		t.Errorf("error should be tagged NPC_HUNTALL: %v", err)
	}
	if s.npcIterator != nil {
		t.Error("npcIterator should remain nil after validation error")
	}
}

// --- NAI-36 Task 3: NPC_GETMODE Layer 1 unit tests -----------------------

func TestNpcGetMode_PushesTargetOp(t *testing.T) {
	npc := &mockNpc{targetOpField: 5} // NPCModePlayerFace per pkg/objtype constants
	mc := &mockConfigs{}

	state := runNpcOp(t, npc, mc, OpNpcGetMode, nil)

	if state.ISP != 1 {
		t.Fatalf("ISP after NPC_GETMODE: got %d, want 1 (one push)", state.ISP)
	}
	got := state.IntStack[0]
	if got != 5 {
		t.Errorf("pushed value: got %d, want 5", got)
	}
}

func TestNpcGetMode_NoActiveNpcErrors(t *testing.T) {
	state := &ScriptState{
		IntStack:    make([]int, StackCapacity),
		StringStack: make([]string, StackCapacity),
	}
	err := handleNpcGetMode(state)
	if err == nil || !strings.Contains(err.Error(), "no active npc") {
		t.Errorf("handleNpcGetMode with no active npc: got %v, want error containing 'no active npc'", err)
	}
}

// --- NAI-36 Task 6: NPC_SETMODE Layer 1 unit tests -----------------------

// mockActiveObj is a minimal ActiveObj fixture used across the
// pkg/script test suite. Extended for NAI-153 with count, receiverID,
// reveal so OBJ_COUNT / OBJ_TAKEITEM tests can drive the IsValidFor
// branches. Existing call sites that construct mockActiveObj{objType,
// x, z, level} continue to work — the new fields default to their zero
// values, which by Reveal=0 (>-1) makes the obj "private to receiver
// 0" — but every such call site sets ActiveObj for handlers that don't
// inspect IsValidFor (OBJ_COORD, OBJ_TYPE, NPC_SETMODE OPOBJ), so the
// default is benign.
// T3/T4 tests driving IsValidFor must set reveal: -1 for public
// or reveal: N, receiverID: uid for private — the zero value is not neutral.
type mockActiveObj struct {
	objType, x, z, level int
	count                int
	receiverID           int
	reveal               int
}

func (m *mockActiveObj) ObjType() int              { return m.objType }
func (m *mockActiveObj) Coords() (x, z, level int) { return m.x, m.z, m.level }
func (m *mockActiveObj) ObjCount() int             { return m.count }
func (m *mockActiveObj) IsValidFor(playerUID int) bool {
	if m.reveal > -1 && playerUID != m.receiverID {
		return false
	}
	if m.count < 1 {
		return false
	}
	return true
}

func TestNpcSetMode_ModeNoneClearsInteractionAndSetsOp(t *testing.T) {
	npc := &mockNpc{}
	mc := &mockConfigs{}

	_ = runNpcOp(t, npc, mc, OpNpcSetMode, []int{int(objtype.NPCModeNone)})

	if npc.clearInteractionCalls != 1 {
		t.Errorf("clearInteractionCalls: got %d, want 1", npc.clearInteractionCalls)
	}
	if len(npc.setTargetOpCalls) != 1 || npc.setTargetOpCalls[0] != int(objtype.NPCModeNone) {
		t.Errorf("setTargetOpCalls: got %v, want [NPCModeNone]", npc.setTargetOpCalls)
	}
	if npc.clearPatrolCalls != 0 {
		t.Errorf("clearPatrolCalls: got %d, want 0 (only PATROL mode triggers clearPatrol)", npc.clearPatrolCalls)
	}
	if len(npc.setInteractionScriptCalls) != 0 {
		t.Errorf("setInteractionScriptCalls: got %d, want 0 (clear-target branch must not bind)",
			len(npc.setInteractionScriptCalls))
	}
}

func TestNpcSetMode_ModeWanderClearsInteractionAndSetsOp(t *testing.T) {
	npc := &mockNpc{}
	mc := &mockConfigs{}

	_ = runNpcOp(t, npc, mc, OpNpcSetMode, []int{int(objtype.NPCModeWander)})

	if npc.clearInteractionCalls != 1 {
		t.Errorf("clearInteractionCalls: got %d, want 1", npc.clearInteractionCalls)
	}
	if len(npc.setTargetOpCalls) != 1 || npc.setTargetOpCalls[0] != int(objtype.NPCModeWander) {
		t.Errorf("setTargetOpCalls: got %v, want [NPCModeWander]", npc.setTargetOpCalls)
	}
	if npc.clearPatrolCalls != 0 {
		t.Errorf("clearPatrolCalls: got %d, want 0", npc.clearPatrolCalls)
	}
}

func TestNpcSetMode_ModePatrolAlsoClearsPatrol(t *testing.T) {
	npc := &mockNpc{}
	mc := &mockConfigs{}

	_ = runNpcOp(t, npc, mc, OpNpcSetMode, []int{int(objtype.NPCModePatrol)})

	if npc.clearInteractionCalls != 1 {
		t.Errorf("clearInteractionCalls: got %d, want 1", npc.clearInteractionCalls)
	}
	if len(npc.setTargetOpCalls) != 1 || npc.setTargetOpCalls[0] != int(objtype.NPCModePatrol) {
		t.Errorf("setTargetOpCalls: got %v, want [NPCModePatrol]", npc.setTargetOpCalls)
	}
	if npc.clearPatrolCalls != 1 {
		t.Errorf("clearPatrolCalls: got %d, want 1 (PATROL must reset patrol-tick)", npc.clearPatrolCalls)
	}
}

func TestNpcSetMode_ModeNullCallsResetDefaults(t *testing.T) {
	npc := &mockNpc{}
	mc := &mockConfigs{}

	_ = runNpcOp(t, npc, mc, OpNpcSetMode, []int{int(objtype.NPCModeNull)})

	if npc.resetDefaultsCalls != 1 {
		t.Errorf("resetDefaultsCalls: got %d, want 1", npc.resetDefaultsCalls)
	}
	if npc.clearInteractionCalls != 0 {
		t.Errorf("clearInteractionCalls: got %d, want 0 (NULL goes through resetDefaults, not direct clear)",
			npc.clearInteractionCalls)
	}
	if len(npc.setInteractionScriptCalls) != 0 {
		t.Errorf("setInteractionScriptCalls: got %d, want 0", len(npc.setInteractionScriptCalls))
	}
}

func TestNpcSetMode_OpPlayerWithSelfTargetBindsToActivePlayer(t *testing.T) {
	npc := &mockNpc{}
	player := &mockPlayer{}
	mc := &mockConfigs{}

	state := &ScriptState{
		Script: &ScriptFile{
			Name:             "test_npc_setmode",
			Opcodes:          []Opcode{OpNpcSetMode, OpReturn},
			IntOperands:      []int32{0, 0},
			StringOperands:   []string{"", ""},
			InstructionCount: 2,
		},
		ActiveNpc:   npc,
		Self:        player,
		Configs:     mc,
		IntStack:    make([]int, StackCapacity),
		StringStack: make([]string, StackCapacity),
	}
	state.PushInt(int(objtype.NPCModeOpPlayer1))

	if err := Execute(state); err != nil {
		t.Fatalf("Execute: %v", err)
	}

	if len(npc.setInteractionScriptCalls) != 1 {
		t.Fatalf("setInteractionScriptCalls: got %d, want 1", len(npc.setInteractionScriptCalls))
	}
	got := npc.setInteractionScriptCalls[0]
	if got.mode != int(objtype.NPCModeOpPlayer1) {
		t.Errorf("mode: got %d, want NPCModeOpPlayer1", got.mode)
	}
	if got.target != ActivePlayer(player) {
		t.Errorf("target: got %v, want player (%v)", got.target, player)
	}
}

func TestNpcSetMode_OpNpcWithIntOperandZeroBindsToOtherActiveNpc(t *testing.T) {
	npc := &mockNpc{}
	otherNpc := &mockNpc{}
	mc := &mockConfigs{}

	state := &ScriptState{
		Script: &ScriptFile{
			Name:             "test_npc_setmode",
			Opcodes:          []Opcode{OpNpcSetMode, OpReturn},
			IntOperands:      []int32{0, 0},
			StringOperands:   []string{"", ""},
			InstructionCount: 2,
		},
		ActiveNpc:      npc,
		OtherActiveNpc: otherNpc,
		Configs:        mc,
		IntStack:       make([]int, StackCapacity),
		StringStack:    make([]string, StackCapacity),
	}
	state.PushInt(int(objtype.NPCModeOpNpc1))

	if err := Execute(state); err != nil {
		t.Fatalf("Execute: %v", err)
	}

	if len(npc.setInteractionScriptCalls) != 1 {
		t.Fatalf("setInteractionScriptCalls: got %d, want 1", len(npc.setInteractionScriptCalls))
	}
	if npc.setInteractionScriptCalls[0].target != ActiveNpc(otherNpc) {
		t.Errorf("target: got %v, want otherNpc (%v) — operand=0 selects OtherActiveNpc",
			npc.setInteractionScriptCalls[0].target, otherNpc)
	}
}

func TestNpcSetMode_OpNpcWithIntOperandNonZeroBindsToActiveNpc(t *testing.T) {
	npc := &mockNpc{}
	otherNpc := &mockNpc{}
	mc := &mockConfigs{}

	state := &ScriptState{
		Script: &ScriptFile{
			Name:             "test_npc_setmode",
			Opcodes:          []Opcode{OpNpcSetMode, OpReturn},
			IntOperands:      []int32{1, 0},
			StringOperands:   []string{"", ""},
			InstructionCount: 2,
		},
		ActiveNpc:      npc,
		OtherActiveNpc: otherNpc,
		Configs:        mc,
		IntStack:       make([]int, StackCapacity),
		StringStack:    make([]string, StackCapacity),
	}
	state.PushInt(int(objtype.NPCModeOpNpc1))

	if err := Execute(state); err != nil {
		t.Fatalf("Execute: %v", err)
	}

	if len(npc.setInteractionScriptCalls) != 1 {
		t.Fatalf("setInteractionScriptCalls: got %d, want 1", len(npc.setInteractionScriptCalls))
	}
	if npc.setInteractionScriptCalls[0].target != ActiveNpc(npc) {
		t.Errorf("target: got %v, want npc (self) — operand!=0 selects ActiveNpc",
			npc.setInteractionScriptCalls[0].target)
	}
}

func TestNpcSetMode_OpObjBindsToActiveObj(t *testing.T) {
	npc := &mockNpc{}
	obj := &mockActiveObj{}
	mc := &mockConfigs{}

	state := &ScriptState{
		Script: &ScriptFile{
			Name:             "test_npc_setmode",
			Opcodes:          []Opcode{OpNpcSetMode, OpReturn},
			IntOperands:      []int32{0, 0},
			StringOperands:   []string{"", ""},
			InstructionCount: 2,
		},
		ActiveNpc:   npc,
		ActiveObj:   obj,
		Configs:     mc,
		IntStack:    make([]int, StackCapacity),
		StringStack: make([]string, StackCapacity),
	}
	state.PushInt(int(objtype.NPCModeOpObj1))

	if err := Execute(state); err != nil {
		t.Fatalf("Execute: %v", err)
	}

	if len(npc.setInteractionScriptCalls) != 1 {
		t.Fatalf("setInteractionScriptCalls: got %d, want 1", len(npc.setInteractionScriptCalls))
	}
	if npc.setInteractionScriptCalls[0].target != ActiveObj(obj) {
		t.Errorf("target: got %v, want obj (%v)", npc.setInteractionScriptCalls[0].target, obj)
	}
}

func TestNpcSetMode_OpLocBindsToActiveLoc(t *testing.T) {
	npc := &mockNpc{}
	loc := &mockActiveLoc{locType: 42}
	mc := &mockConfigs{}

	state := &ScriptState{
		Script: &ScriptFile{
			Name:             "test_npc_setmode",
			Opcodes:          []Opcode{OpNpcSetMode, OpReturn},
			IntOperands:      []int32{0, 0},
			StringOperands:   []string{"", ""},
			InstructionCount: 2,
		},
		ActiveNpc:   npc,
		ActiveLoc:   loc,
		Configs:     mc,
		IntStack:    make([]int, StackCapacity),
		StringStack: make([]string, StackCapacity),
	}
	state.PushInt(int(objtype.NPCModeOpLoc1))

	if err := Execute(state); err != nil {
		t.Fatalf("Execute: %v", err)
	}

	if len(npc.setInteractionScriptCalls) != 1 {
		t.Fatalf("setInteractionScriptCalls: got %d, want 1", len(npc.setInteractionScriptCalls))
	}
	if npc.setInteractionScriptCalls[0].target != ActiveLoc(loc) {
		t.Errorf("target: got %v, want loc (%v)", npc.setInteractionScriptCalls[0].target, loc)
	}
}

func TestNpcSetMode_OpPlayerWithNoSelfFallsThroughToResetDefaults(t *testing.T) {
	npc := &mockNpc{}
	mc := &mockConfigs{}

	state := &ScriptState{
		Script: &ScriptFile{
			Name:             "test_npc_setmode",
			Opcodes:          []Opcode{OpNpcSetMode, OpReturn},
			IntOperands:      []int32{0, 0},
			StringOperands:   []string{"", ""},
			InstructionCount: 2,
		},
		ActiveNpc:   npc,
		Self:        nil,
		Configs:     mc,
		IntStack:    make([]int, StackCapacity),
		StringStack: make([]string, StackCapacity),
	}
	state.PushInt(int(objtype.NPCModeOpPlayer1))

	if err := Execute(state); err != nil {
		t.Fatalf("Execute: %v", err)
	}

	if npc.resetDefaultsCalls != 1 {
		t.Errorf("resetDefaultsCalls: got %d, want 1 (no-target fallthrough)", npc.resetDefaultsCalls)
	}
	if len(npc.setInteractionScriptCalls) != 0 {
		t.Errorf("setInteractionScriptCalls: got %d, want 0 (no target → no bind)",
			len(npc.setInteractionScriptCalls))
	}
}

func TestNpcSetMode_NoActiveNpcErrors(t *testing.T) {
	state := &ScriptState{
		IntStack:    make([]int, StackCapacity),
		StringStack: make([]string, StackCapacity),
	}
	state.PushInt(int(objtype.NPCModeNone))

	err := handleNpcSetMode(state)
	if err == nil || !strings.Contains(err.Error(), "no active npc") {
		t.Errorf("handleNpcSetMode with no active npc: got %v, want error", err)
	}
}

// equalIntSlice reports whether two int slices are element-wise equal.
// Local helper for the NAI-37 NPC_WALKTRIGGER tests; the rest of this
// file uses reflect.DeepEqual, but the plan prescribes this helper for
// the walktrigger pop-order/transform assertions.
func equalIntSlice(a, b []int) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// --- NAI-37 Task 2: NPC_WALKTRIGGER handler unit tests ---------------------

func TestNpcWalkTrigger_NoActiveNpc_Errors(t *testing.T) {
	s := &ScriptState{
		IntStack:    make([]int, StackCapacity),
		StringStack: make([]string, StackCapacity),
	} // no ActiveNpc set
	// Push order: queueID (first → bottom), arg (second → top).
	// Handler pops arg first, queueID second.
	s.PushInt(3) // queueID
	s.PushInt(5) // arg
	if err := handleNpcWalkTrigger(s); err == nil {
		t.Fatalf("expected error for no active npc")
	}
}

func TestNpcWalkTrigger_QueueIDBelowOne_Errors(t *testing.T) {
	npc := &mockNpc{}
	s := &ScriptState{
		ActiveNpc:   npc,
		IntStack:    make([]int, StackCapacity),
		StringStack: make([]string, StackCapacity),
	}
	// Push order: queueID (first → bottom), arg (second → top).
	s.PushInt(0) // queueID = 0 → invalid
	s.PushInt(5) // arg
	if err := handleNpcWalkTrigger(s); err == nil {
		t.Fatalf("expected error for queueID=0")
	}
	if len(npc.walkTriggerCalls) != 0 {
		t.Errorf("walkTriggerCalls: got %d writes, want 0 on validation failure",
			len(npc.walkTriggerCalls))
	}
}

func TestNpcWalkTrigger_QueueIDAboveTwenty_Errors(t *testing.T) {
	npc := &mockNpc{}
	s := &ScriptState{
		ActiveNpc:   npc,
		IntStack:    make([]int, StackCapacity),
		StringStack: make([]string, StackCapacity),
	}
	// Push order: queueID (first → bottom), arg (second → top).
	s.PushInt(21) // queueID = 21 → invalid
	s.PushInt(5)  // arg
	if err := handleNpcWalkTrigger(s); err == nil {
		t.Fatalf("expected error for queueID=21")
	}
}

func TestNpcWalkTrigger_PopOrderAndTransform(t *testing.T) {
	npc := &mockNpc{}
	s := &ScriptState{
		ActiveNpc:   npc,
		IntStack:    make([]int, StackCapacity),
		StringStack: make([]string, StackCapacity),
	}
	s.PushInt(7)  // queueID (pushed first → bottom of stack)
	s.PushInt(42) // arg (pushed second → top of stack)
	if err := handleNpcWalkTrigger(s); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Pop order: arg (top) first, queueID (next) second.
	// Then queueID-1 transform: 7 → 6.
	if want := []int{6}; !equalIntSlice(npc.walkTriggerCalls, want) {
		t.Errorf("walkTriggerCalls: got %v, want %v", npc.walkTriggerCalls, want)
	}
	if want := []int{42}; !equalIntSlice(npc.walkTriggerArgCalls, want) {
		t.Errorf("walkTriggerArgCalls: got %v, want %v", npc.walkTriggerArgCalls, want)
	}
}

func TestNpcWalkTrigger_BoundaryQueueIDs(t *testing.T) {
	t.Run("queueID=1", func(t *testing.T) {
		npc := &mockNpc{}
		s := &ScriptState{
			ActiveNpc:   npc,
			IntStack:    make([]int, StackCapacity),
			StringStack: make([]string, StackCapacity),
		}
		s.PushInt(1) // queueID
		s.PushInt(0) // arg
		if err := handleNpcWalkTrigger(s); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if want := []int{0}; !equalIntSlice(npc.walkTriggerCalls, want) {
			t.Errorf("queueID=1 → walktrigger=0 (queueID-1); got %v", npc.walkTriggerCalls)
		}
	})
	t.Run("queueID=20", func(t *testing.T) {
		npc := &mockNpc{}
		s := &ScriptState{
			ActiveNpc:   npc,
			IntStack:    make([]int, StackCapacity),
			StringStack: make([]string, StackCapacity),
		}
		s.PushInt(20) // queueID
		s.PushInt(0)  // arg
		if err := handleNpcWalkTrigger(s); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if want := []int{19}; !equalIntSlice(npc.walkTriggerCalls, want) {
			t.Errorf("queueID=20 → walktrigger=19 (queueID-1); got %v", npc.walkTriggerCalls)
		}
	})
}

// NPC_FINDUID (opcode 2521) — NAI-120 Bundle 2A.
// Mirrors TS NpcOps.ts:26-40.

func TestNpcFindUID_Hit_PrimarySlot(t *testing.T) {
	npc := &mockNpc{typeID: 42, nid: 7, uid: (42 << 16) | 7}
	lookup := &mockNpcLookup{byUID: map[int]ActiveNpc{npc.uid: npc}}
	s := &ScriptState{
		Npcs:        lookup,
		Script:      &ScriptFile{IntOperands: []int32{0}},
		PC:          0,
		IntStack:    make([]int, StackCapacity),
		StringStack: make([]string, StackCapacity),
	}
	s.PushInt(npc.uid)
	if err := handleNpcFindUID(s); err !=nil {
		t.Fatalf("NPC_FINDUID hit: unexpected error %v", err)
	}
	if got := s.PopInt(); got !=1 {
		t.Errorf("NPC_FINDUID hit: pushed %d, want 1", got)
	}
	if s.ActiveNpc !=npc {
		t.Errorf("NPC_FINDUID hit: ActiveNpc not bound (got %v, want %v)", s.ActiveNpc, npc)
	}
	if s.Pointers&PtrActiveNpc == 0 {
		t.Error("NPC_FINDUID hit: PtrActiveNpc not set")
	}
}

func TestNpcFindUID_Hit_SecondarySlot(t *testing.T) {
	npc := &mockNpc{typeID: 42, nid: 7, uid: (42 << 16) | 7}
	lookup := &mockNpcLookup{byUID: map[int]ActiveNpc{npc.uid: npc}}
	s := &ScriptState{
		Npcs:        lookup,
		Script:      &ScriptFile{IntOperands: []int32{1}},
		PC:          0,
		IntStack:    make([]int, StackCapacity),
		StringStack: make([]string, StackCapacity),
	}
	s.PushInt(npc.uid)
	if err := handleNpcFindUID(s); err !=nil {
		t.Fatalf("NPC_FINDUID secondary hit: unexpected error %v", err)
	}
	if got := s.PopInt(); got !=1 {
		t.Errorf("NPC_FINDUID secondary hit: pushed %d, want 1", got)
	}
	if s.OtherActiveNpc !=npc {
		t.Errorf("NPC_FINDUID secondary hit: OtherActiveNpc not bound (got %v, want %v)", s.OtherActiveNpc, npc)
	}
	if s.Pointers&PtrActiveNpc2 == 0 {
		t.Error("NPC_FINDUID secondary hit: PtrActiveNpc2 not set")
	}
}

func TestNpcFindUID_Miss(t *testing.T) {
	lookup := &mockNpcLookup{byUID: nil}
	s := &ScriptState{
		Npcs:        lookup,
		Script:      &ScriptFile{IntOperands: []int32{0}},
		PC:          0,
		IntStack:    make([]int, StackCapacity),
		StringStack: make([]string, StackCapacity),
	}
	s.PushInt((42 << 16) | 999)
	if err := handleNpcFindUID(s); err !=nil {
		t.Fatalf("NPC_FINDUID miss: unexpected error %v", err)
	}
	if got := s.PopInt(); got !=0 {
		t.Errorf("NPC_FINDUID miss: pushed %d, want 0", got)
	}
	if s.ActiveNpc !=nil {
		t.Errorf("NPC_FINDUID miss: ActiveNpc should remain nil (got %v)", s.ActiveNpc)
	}
	if s.Pointers&PtrActiveNpc !=0 {
		t.Error("NPC_FINDUID miss: PtrActiveNpc should NOT be set")
	}
}

func TestNpcFindUID_NilNpcs(t *testing.T) {
	s := &ScriptState{
		Npcs:        nil,
		Script:      &ScriptFile{IntOperands: []int32{0}},
		PC:          0,
		IntStack:    make([]int, StackCapacity),
		StringStack: make([]string, StackCapacity),
	}
	s.PushInt((42 << 16) | 7)
	if err := handleNpcFindUID(s); err !=nil {
		t.Fatalf("NPC_FINDUID nil Npcs: unexpected error %v", err)
	}
	if got := s.PopInt(); got !=0 {
		t.Errorf("NPC_FINDUID nil Npcs: pushed %d, want 0", got)
	}
}

// NPC_RANGE (opcode 2531) — NAI-120 Bundle 2A.
// Mirrors TS NpcOps.ts:152-168.

func TestNpcRange_SameLevel_Adjacent(t *testing.T) {
	npc := &mockNpc{x: 3222, z: 3218, level: 0}
	s := &ScriptState{
		ActiveNpc:   npc,
		Pointers:    PtrActiveNpc,
		IntStack:    make([]int, StackCapacity),
		StringStack: make([]string, StackCapacity),
	}
	// Pop a packed coord at (3223, 3218, level 0): chebyshev distance 1.
	s.PushInt((0 << 28) | (3223 << 14) | 3218)
	if err := handleNpcRange(s); err != nil {
		t.Fatalf("NPC_RANGE same-level adjacent: unexpected error %v", err)
	}
	if got := s.PopInt(); got != 1 {
		t.Errorf("NPC_RANGE same-level adjacent: got %d, want 1", got)
	}
}

func TestNpcRange_SameLevel_Diagonal(t *testing.T) {
	npc := &mockNpc{x: 3222, z: 3218, level: 0}
	s := &ScriptState{
		ActiveNpc:   npc,
		Pointers:    PtrActiveNpc,
		IntStack:    make([]int, StackCapacity),
		StringStack: make([]string, StackCapacity),
	}
	// Pop a packed coord at (3223, 3219, 0): chebyshev distance 1.
	s.PushInt((0 << 28) | (3223 << 14) | 3219)
	if err := handleNpcRange(s); err != nil {
		t.Fatalf("NPC_RANGE same-level diagonal: unexpected error %v", err)
	}
	if got := s.PopInt(); got != 1 {
		t.Errorf("NPC_RANGE same-level diagonal: got %d, want 1", got)
	}
}

func TestNpcRange_DifferentLevel_Sentinel(t *testing.T) {
	npc := &mockNpc{x: 3222, z: 3218, level: 0}
	s := &ScriptState{
		ActiveNpc:   npc,
		Pointers:    PtrActiveNpc,
		IntStack:    make([]int, StackCapacity),
		StringStack: make([]string, StackCapacity),
	}
	// Pop a packed coord at (3222, 3218, level 1): different level -> -1.
	s.PushInt((1 << 28) | (3222 << 14) | 3218)
	if err := handleNpcRange(s); err != nil {
		t.Fatalf("NPC_RANGE diff-level: unexpected error %v", err)
	}
	if got := s.PopInt(); got != -1 {
		t.Errorf("NPC_RANGE diff-level: got %d, want -1", got)
	}
}

func TestNpcRange_NoActiveNpc(t *testing.T) {
	s := &ScriptState{
		IntStack:    make([]int, StackCapacity),
		StringStack: make([]string, StackCapacity),
	}
	s.PushInt(0)
	if err := handleNpcRange(s); err == nil {
		t.Error("NPC_RANGE with no ActiveNpc: want error")
	}
}

func TestNpcRange_InvalidCoord(t *testing.T) {
	npc := &mockNpc{x: 3222, z: 3218, level: 0}
	s := &ScriptState{
		ActiveNpc:   npc,
		Pointers:    PtrActiveNpc,
		IntStack:    make([]int, StackCapacity),
		StringStack: make([]string, StackCapacity),
	}
	// Negative coord fails CoordValid.
	s.PushInt(-1)
	if err := handleNpcRange(s); err == nil {
		t.Error("NPC_RANGE invalid coord: want error")
	}
}

// --- NAI-120 Bundle 2C: NPC_STATADD tests ---

func TestNpcStatAdd_HappyPath(t *testing.T) {
	npc := &mockNpc{
		baseLevels: map[int]int{0: 70},
		levels:     map[int]int{0: 50},
	}
	s := &ScriptState{
		ActiveNpc:   npc,
		Pointers:    PtrActiveNpc,
		IntStack:    make([]int, StackCapacity),
		StringStack: make([]string, StackCapacity),
	}
	// Pop order: percent (top), constant, stat (bottom).
	s.PushInt(0)  // stat
	s.PushInt(5)  // constant
	s.PushInt(10) // percent
	if err := handleNpcStatAdd(s); err != nil {
		t.Fatalf("NPC_STATADD happy: unexpected error %v", err)
	}
	// 50 + (5 + 70*10/100) = 50 + (5 + 7) = 62
	if got := len(npc.setNpcStatCalls); got != 1 {
		t.Fatalf("NPC_STATADD happy: SetNpcStat calls = %d, want 1", got)
	}
	call := npc.setNpcStatCalls[0]
	if call.stat != 0 || call.level != 62 {
		t.Errorf("NPC_STATADD happy: SetNpcStat(%d,%d), want (0,62)", call.stat, call.level)
	}
}

func TestNpcStatAdd_CapAt255(t *testing.T) {
	npc := &mockNpc{
		baseLevels: map[int]int{0: 100},
		levels:     map[int]int{0: 250},
	}
	s := &ScriptState{
		ActiveNpc:   npc,
		Pointers:    PtrActiveNpc,
		IntStack:    make([]int, StackCapacity),
		StringStack: make([]string, StackCapacity),
	}
	s.PushInt(0)
	s.PushInt(10)
	s.PushInt(100)
	if err := handleNpcStatAdd(s); err != nil {
		t.Fatalf("NPC_STATADD cap: unexpected error %v", err)
	}
	// 250 + (10 + 100*100/100) = 250 + 110 = 360 → clamp 255.
	if got := npc.setNpcStatCalls[0].level; got != 255 {
		t.Errorf("NPC_STATADD cap: level = %d, want 255", got)
	}
}

func TestNpcStatAdd_NoActiveNpc(t *testing.T) {
	s := &ScriptState{
		IntStack:    make([]int, StackCapacity),
		StringStack: make([]string, StackCapacity),
	}
	s.PushInt(0)
	s.PushInt(5)
	s.PushInt(10)
	if err := handleNpcStatAdd(s); err == nil {
		t.Error("NPC_STATADD no active npc: want error")
	}
}

func TestNpcStatAdd_StatOOB(t *testing.T) {
	npc := &mockNpc{}
	s := &ScriptState{
		ActiveNpc:   npc,
		Pointers:    PtrActiveNpc,
		IntStack:    make([]int, StackCapacity),
		StringStack: make([]string, StackCapacity),
	}
	s.PushInt(6) // OOB
	s.PushInt(5)
	s.PushInt(10)
	if err := handleNpcStatAdd(s); err == nil {
		t.Error("NPC_STATADD stat=6: want range error")
	}
}

func TestNpcStatAdd_ConstantNull(t *testing.T) {
	npc := &mockNpc{baseLevels: map[int]int{0: 10}, levels: map[int]int{0: 5}}
	s := &ScriptState{
		ActiveNpc:   npc,
		Pointers:    PtrActiveNpc,
		IntStack:    make([]int, StackCapacity),
		StringStack: make([]string, StackCapacity),
	}
	s.PushInt(0)
	s.PushInt(-1) // null
	s.PushInt(10)
	if err := handleNpcStatAdd(s); err == nil {
		t.Error("NPC_STATADD constant=-1: want NumberNotNull error")
	}
}

func TestNpcStatAdd_PercentNull(t *testing.T) {
	npc := &mockNpc{baseLevels: map[int]int{0: 10}, levels: map[int]int{0: 5}}
	s := &ScriptState{
		ActiveNpc:   npc,
		Pointers:    PtrActiveNpc,
		IntStack:    make([]int, StackCapacity),
		StringStack: make([]string, StackCapacity),
	}
	s.PushInt(0)
	s.PushInt(5)
	s.PushInt(-1) // null
	if err := handleNpcStatAdd(s); err == nil {
		t.Error("NPC_STATADD percent=-1: want NumberNotNull error")
	}
}

// --- NAI-120 Bundle 2C: NPC_STATSUB tests ---

func TestNpcStatSub_HappyPath(t *testing.T) {
	npc := &mockNpc{
		baseLevels: map[int]int{0: 70},
		levels:     map[int]int{0: 50},
	}
	s := &ScriptState{
		ActiveNpc:   npc,
		Pointers:    PtrActiveNpc,
		IntStack:    make([]int, StackCapacity),
		StringStack: make([]string, StackCapacity),
	}
	s.PushInt(0)
	s.PushInt(5)
	s.PushInt(10)
	if err := handleNpcStatSub(s); err != nil {
		t.Fatalf("NPC_STATSUB happy: unexpected error %v", err)
	}
	// 50 - (5 + 70*10/100) = 50 - (5 + 7) = 38
	if got := npc.setNpcStatCalls[0].level; got != 38 {
		t.Errorf("NPC_STATSUB happy: level = %d, want 38", got)
	}
}

func TestNpcStatSub_FloorAtZero(t *testing.T) {
	npc := &mockNpc{
		baseLevels: map[int]int{0: 70},
		levels:     map[int]int{0: 5},
	}
	s := &ScriptState{
		ActiveNpc:   npc,
		Pointers:    PtrActiveNpc,
		IntStack:    make([]int, StackCapacity),
		StringStack: make([]string, StackCapacity),
	}
	s.PushInt(0)
	s.PushInt(100)
	s.PushInt(100)
	if err := handleNpcStatSub(s); err != nil {
		t.Fatalf("NPC_STATSUB floor: unexpected error %v", err)
	}
	// 5 - (100 + 70) = -165 → clamp 0.
	if got := npc.setNpcStatCalls[0].level; got != 0 {
		t.Errorf("NPC_STATSUB floor: level = %d, want 0", got)
	}
}

func TestNpcStatSub_NoActiveNpc(t *testing.T) {
	s := &ScriptState{
		IntStack:    make([]int, StackCapacity),
		StringStack: make([]string, StackCapacity),
	}
	s.PushInt(0)
	s.PushInt(5)
	s.PushInt(10)
	if err := handleNpcStatSub(s); err == nil {
		t.Error("NPC_STATSUB no active npc: want error")
	}
}

func TestNpcStatSub_StatOOB(t *testing.T) {
	npc := &mockNpc{}
	s := &ScriptState{
		ActiveNpc:   npc,
		Pointers:    PtrActiveNpc,
		IntStack:    make([]int, StackCapacity),
		StringStack: make([]string, StackCapacity),
	}
	s.PushInt(-1)
	s.PushInt(5)
	s.PushInt(10)
	if err := handleNpcStatSub(s); err == nil {
		t.Error("NPC_STATSUB stat=-1: want range error")
	}
}

func TestNpcStatSub_ConstantNull(t *testing.T) {
	npc := &mockNpc{baseLevels: map[int]int{0: 10}, levels: map[int]int{0: 5}}
	s := &ScriptState{
		ActiveNpc:   npc,
		Pointers:    PtrActiveNpc,
		IntStack:    make([]int, StackCapacity),
		StringStack: make([]string, StackCapacity),
	}
	s.PushInt(0)
	s.PushInt(-1)
	s.PushInt(10)
	if err := handleNpcStatSub(s); err == nil {
		t.Error("NPC_STATSUB constant=-1: want NumberNotNull error")
	}
}

func TestNpcStatSub_PercentNull(t *testing.T) {
	npc := &mockNpc{baseLevels: map[int]int{0: 10}, levels: map[int]int{0: 5}}
	s := &ScriptState{
		ActiveNpc:   npc,
		Pointers:    PtrActiveNpc,
		IntStack:    make([]int, StackCapacity),
		StringStack: make([]string, StackCapacity),
	}
	s.PushInt(0)
	s.PushInt(5)
	s.PushInt(-1) // null
	if err := handleNpcStatSub(s); err == nil {
		t.Error("NPC_STATSUB percent=-1: want NumberNotNull error")
	}
}

// --- NAI-120 Bundle 2C: SPOTANIM_NPC tests ---

func TestSpotAnimNpc_HappyPath(t *testing.T) {
	npc := &mockNpc{}
	cfg := newTestConfigsWithSpotAnims(map[int]bool{5: true})
	s := &ScriptState{
		ActiveNpc:   npc,
		Configs:     cfg,
		Pointers:    PtrActiveNpc,
		IntStack:    make([]int, StackCapacity),
		StringStack: make([]string, StackCapacity),
	}
	// Pop order: delay (top), height, spotanim id (bottom).
	s.PushInt(5)  // spotanim id
	s.PushInt(92) // height
	s.PushInt(30) // delay
	if err := handleSpotAnimNpc(s); err != nil {
		t.Fatalf("SPOTANIM_NPC happy: unexpected error %v", err)
	}
	if got := len(npc.playSpotAnimCalls); got != 1 {
		t.Fatalf("SPOTANIM_NPC happy: PlaySpotAnim calls = %d, want 1", got)
	}
	call := npc.playSpotAnimCalls[0]
	if call.id != 5 || call.height != 92 || call.delay != 30 {
		t.Errorf("SPOTANIM_NPC happy: PlaySpotAnim(%d,%d,%d), want (5,92,30)", call.id, call.height, call.delay)
	}
}

func TestSpotAnimNpc_NoActiveNpc(t *testing.T) {
	cfg := newTestConfigsWithSpotAnims(map[int]bool{5: true})
	s := &ScriptState{
		Configs:     cfg,
		IntStack:    make([]int, StackCapacity),
		StringStack: make([]string, StackCapacity),
	}
	s.PushInt(5)
	s.PushInt(92)
	s.PushInt(30)
	if err := handleSpotAnimNpc(s); err == nil {
		t.Error("SPOTANIM_NPC no active npc: want error")
	}
}

func TestSpotAnimNpc_DelayNull(t *testing.T) {
	npc := &mockNpc{}
	cfg := newTestConfigsWithSpotAnims(map[int]bool{5: true})
	s := &ScriptState{
		ActiveNpc:   npc,
		Configs:     cfg,
		Pointers:    PtrActiveNpc,
		IntStack:    make([]int, StackCapacity),
		StringStack: make([]string, StackCapacity),
	}
	s.PushInt(5)
	s.PushInt(92)
	s.PushInt(-1)
	if err := handleSpotAnimNpc(s); err == nil {
		t.Error("SPOTANIM_NPC delay=-1: want NumberNotNull error")
	}
}

func TestSpotAnimNpc_HeightNull(t *testing.T) {
	npc := &mockNpc{}
	cfg := newTestConfigsWithSpotAnims(map[int]bool{5: true})
	s := &ScriptState{
		ActiveNpc:   npc,
		Configs:     cfg,
		Pointers:    PtrActiveNpc,
		IntStack:    make([]int, StackCapacity),
		StringStack: make([]string, StackCapacity),
	}
	s.PushInt(5)
	s.PushInt(-1)
	s.PushInt(30)
	if err := handleSpotAnimNpc(s); err == nil {
		t.Error("SPOTANIM_NPC height=-1: want NumberNotNull error")
	}
}

func TestSpotAnimNpc_InvalidSpotAnim(t *testing.T) {
	npc := &mockNpc{}
	cfg := newTestConfigsWithSpotAnims(map[int]bool{5: true})
	s := &ScriptState{
		ActiveNpc:   npc,
		Configs:     cfg,
		Pointers:    PtrActiveNpc,
		IntStack:    make([]int, StackCapacity),
		StringStack: make([]string, StackCapacity),
	}
	s.PushInt(999) // not in registry
	s.PushInt(92)
	s.PushInt(30)
	if err := handleSpotAnimNpc(s); err == nil {
		t.Error("SPOTANIM_NPC invalid id: want SpotAnimTypeValid error")
	}
}

// --- NAI-120 Bundle 2D: NPC_HEROPOINTS handler tests -----------------------

func TestNpcHeroPoints_HappyPath(t *testing.T) {
	mp := &mockPlayer{uidValue: 12345}
	npc := &mockNpc{}
	s := &ScriptState{
		Self:        mp,
		ActiveNpc:   npc,
		Pointers:    PtrActivePlayer | PtrActiveNpc,
		IntStack:    make([]int, StackCapacity),
		StringStack: make([]string, StackCapacity),
	}
	s.PushInt(42)
	if err := handleNpcHeroPoints(s); err != nil {
		t.Fatalf("NPC_HEROPOINTS happy: unexpected error %v", err)
	}
	if got := len(npc.addHeroPointsCalls); got != 1 {
		t.Fatalf("NPC_HEROPOINTS happy: AddHeroPoints calls = %d, want 1", got)
	}
	call := npc.addHeroPointsCalls[0]
	if call.playerUID != 12345 || call.amount != 42 {
		t.Errorf("NPC_HEROPOINTS happy: AddHeroPoints(%d,%d), want (12345,42)", call.playerUID, call.amount)
	}
}

func TestNpcHeroPoints_NoActivePlayer(t *testing.T) {
	npc := &mockNpc{}
	s := &ScriptState{
		ActiveNpc:   npc,
		Pointers:    PtrActiveNpc,
		IntStack:    make([]int, StackCapacity),
		StringStack: make([]string, StackCapacity),
	}
	s.PushInt(42)
	if err := handleNpcHeroPoints(s); err == nil {
		t.Error("NPC_HEROPOINTS no active player: want error")
	}
}

func TestNpcHeroPoints_NoActiveNpc(t *testing.T) {
	mp := &mockPlayer{uidValue: 1}
	s := &ScriptState{
		Self:        mp,
		Pointers:    PtrActivePlayer,
		IntStack:    make([]int, StackCapacity),
		StringStack: make([]string, StackCapacity),
	}
	s.PushInt(42)
	if err := handleNpcHeroPoints(s); err == nil {
		t.Error("NPC_HEROPOINTS no active npc: want error")
	}
}

func TestNpcHeroPoints_AmountNull(t *testing.T) {
	mp := &mockPlayer{uidValue: 1}
	npc := &mockNpc{}
	s := &ScriptState{
		Self:        mp,
		ActiveNpc:   npc,
		Pointers:    PtrActivePlayer | PtrActiveNpc,
		IntStack:    make([]int, StackCapacity),
		StringStack: make([]string, StackCapacity),
	}
	s.PushInt(-1)
	if err := handleNpcHeroPoints(s); err == nil {
		t.Error("NPC_HEROPOINTS amount=-1: want NumberNotNull error")
	}
}

func TestNpcHeroPoints_AmountZero(t *testing.T) {
	mp := &mockPlayer{uidValue: 1}
	npc := &mockNpc{}
	s := &ScriptState{
		Self:        mp,
		ActiveNpc:   npc,
		Pointers:    PtrActivePlayer | PtrActiveNpc,
		IntStack:    make([]int, StackCapacity),
		StringStack: make([]string, StackCapacity),
	}
	s.PushInt(0)
	if err := handleNpcHeroPoints(s); err != nil {
		t.Fatalf("NPC_HEROPOINTS amount=0: unexpected error %v (NumberNotNull only rejects -1)", err)
	}
	// Handler still calls AddHeroPoints(uid, 0); the ledger no-ops on amount=0.
	if got := len(npc.addHeroPointsCalls); got != 1 {
		t.Errorf("NPC_HEROPOINTS amount=0: AddHeroPoints calls = %d, want 1", got)
	}
}

// -- NPC_ARRIVEDELAY tests (NAI-125) ---------------------------------------
//
// TS NpcOps.ts:542-555: 3-tick acceptance window with recency-dependent
// suspend duration. Asymmetric vs P_ARRIVEDELAY (which has a 2-tick
// window and always SetDelayed(0)).
//
// lastMovement is written by (*Npc).updateMovement to currentTick + 1
// after any tick the NPC stepped (npc_interaction.go:334). The
// SetDelayed(ticks) primitive at npc.go:323-326 writes both
// n.delayed = true and n.delayedUntil = currentTick + 1 + ticks, so:
//   TS delayedUntil = T+2 ⇒ goscape SetDelayed(1)
//   TS delayedUntil = T+1 ⇒ goscape SetDelayed(0)

// TestNpcArriveDelaySuspendsWhenMovedThisTick: lastMovement = currentTick + 1.
// Gate condition: 101 < 99 is false ⇒ continue. Branch: 101 == 99 is false
// ⇒ SetDelayed(1) (delayedUntil = T+2).
func TestNpcArriveDelaySuspendsWhenMovedThisTick(t *testing.T) {
	mn := &mockNpc{lastMovement: 101}
	w := &mockWorld{tick: 100}
	sf := &ScriptFile{
		Name:    "npc_arrivedelay_moved_this_tick",
		Opcodes: []Opcode{OpNpcArriveDelay, OpReturn},
	}
	state := Init(sf, nil, false, nil, nil)
	state.ActiveNpc = mn
	state.World = w

	if err := Execute(state); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if state.Execution != NpcSuspended {
		t.Errorf("Execution: got %v, want NpcSuspended", state.Execution)
	}
	if len(mn.setDelayedCalls) != 1 || mn.setDelayedCalls[0] != 1 {
		t.Errorf("setDelayedCalls: got %v, want [1]", mn.setDelayedCalls)
	}
}

// TestNpcArriveDelaySuspendsWhenMovedLastTick: lastMovement = currentTick (the
// boundary case — moved on tick T-1 means lastMovement was set to T-1+1 = T).
// Gate condition: 100 < 99 is false ⇒ continue. Branch: 100 == 99 is false
// ⇒ SetDelayed(1) (delayedUntil = T+2). Mid-of-window.
func TestNpcArriveDelaySuspendsWhenMovedLastTick(t *testing.T) {
	mn := &mockNpc{lastMovement: 100}
	w := &mockWorld{tick: 100}
	sf := &ScriptFile{
		Name:    "npc_arrivedelay_moved_last_tick",
		Opcodes: []Opcode{OpNpcArriveDelay, OpReturn},
	}
	state := Init(sf, nil, false, nil, nil)
	state.ActiveNpc = mn
	state.World = w

	if err := Execute(state); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if state.Execution != NpcSuspended {
		t.Errorf("Execution: got %v, want NpcSuspended", state.Execution)
	}
	if len(mn.setDelayedCalls) != 1 || mn.setDelayedCalls[0] != 1 {
		t.Errorf("setDelayedCalls: got %v, want [1]", mn.setDelayedCalls)
	}
}

// TestNpcArriveDelaySuspendsWhenMovedTwoTicksAgo: lastMovement = currentTick - 1.
// Gate condition: 99 < 99 is false ⇒ continue. Branch: 99 == 99 is true
// ⇒ SetDelayed(0) (delayedUntil = T+1). NPC-unique branch (no equivalent
// in P_ARRIVEDELAY which always SetDelayed(0) regardless).
func TestNpcArriveDelaySuspendsWhenMovedTwoTicksAgo(t *testing.T) {
	mn := &mockNpc{lastMovement: 99}
	w := &mockWorld{tick: 100}
	sf := &ScriptFile{
		Name:    "npc_arrivedelay_moved_two_ticks_ago",
		Opcodes: []Opcode{OpNpcArriveDelay, OpReturn},
	}
	state := Init(sf, nil, false, nil, nil)
	state.ActiveNpc = mn
	state.World = w

	if err := Execute(state); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if state.Execution != NpcSuspended {
		t.Errorf("Execution: got %v, want NpcSuspended", state.Execution)
	}
	if len(mn.setDelayedCalls) != 1 || mn.setDelayedCalls[0] != 0 {
		t.Errorf("setDelayedCalls: got %v, want [0]", mn.setDelayedCalls)
	}
}

// TestNpcArriveDelayNoOpWhenMovedThreeTicksAgo: lastMovement = currentTick - 2
// (the first tick on which the gate becomes a no-op).
// Gate condition: 98 < 99 is true ⇒ return early.
func TestNpcArriveDelayNoOpWhenMovedThreeTicksAgo(t *testing.T) {
	mn := &mockNpc{lastMovement: 98}
	w := &mockWorld{tick: 100}
	sf := &ScriptFile{
		Name:    "npc_arrivedelay_moved_three_ticks_ago",
		Opcodes: []Opcode{OpNpcArriveDelay, OpReturn},
	}
	state := Init(sf, nil, false, nil, nil)
	state.ActiveNpc = mn
	state.World = w

	if err := Execute(state); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if state.Execution != Finished {
		t.Errorf("Execution: got %v, want Finished (no-op should let OpReturn complete)", state.Execution)
	}
	if len(mn.setDelayedCalls) != 0 {
		t.Errorf("setDelayedCalls: got %v, want [] (no-op must not call SetDelayed)", mn.setDelayedCalls)
	}
}

// TestNpcArriveDelayNoOpWhenNeverMoved: lastMovement = 0 (zero-value, never
// moved). Gate condition: 0 < 99 is true ⇒ return early. Pins zero-value.
func TestNpcArriveDelayNoOpWhenNeverMoved(t *testing.T) {
	mn := &mockNpc{lastMovement: 0}
	w := &mockWorld{tick: 100}
	sf := &ScriptFile{
		Name:    "npc_arrivedelay_never_moved",
		Opcodes: []Opcode{OpNpcArriveDelay, OpReturn},
	}
	state := Init(sf, nil, false, nil, nil)
	state.ActiveNpc = mn
	state.World = w

	if err := Execute(state); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if state.Execution != Finished {
		t.Errorf("Execution: got %v, want Finished", state.Execution)
	}
	if len(mn.setDelayedCalls) != 0 {
		t.Errorf("setDelayedCalls: got %v, want []", mn.setDelayedCalls)
	}
}

// TestNpcArriveDelayRequiresActiveNpc: handler must reject when no
// ActiveNpc bound. Mirrors requireActiveNpc semantics shared by every
// NPC_* reader handler.
func TestNpcArriveDelayRequiresActiveNpc(t *testing.T) {
	sf := &ScriptFile{
		Name:    "npc_arrivedelay_no_npc",
		Opcodes: []Opcode{OpNpcArriveDelay, OpReturn},
	}
	state := Init(sf, nil, false, nil, nil)
	// state.ActiveNpc intentionally left nil.
	state.World = &mockWorld{tick: 100}

	err := Execute(state)
	if err == nil || err.Error() != "NPC_ARRIVEDELAY: no active npc" {
		t.Errorf("expected 'NPC_ARRIVEDELAY: no active npc', got %v", err)
	}
}

// TestNpcArriveDelayRequiresWorld: handler reads s.World.CurrentTick() to
// evaluate its gate; missing world must return a clean error rather than
// nil-deref. Defensive guard mirrors P_ARRIVEDELAY's sibling-handler
// convention (DEVIATION-NAI-125-D1; goscape defensive; TS skips this check).
func TestNpcArriveDelayRequiresWorld(t *testing.T) {
	mn := &mockNpc{lastMovement: 101}
	sf := &ScriptFile{
		Name:    "npc_arrivedelay_no_world",
		Opcodes: []Opcode{OpNpcArriveDelay, OpReturn},
	}
	state := Init(sf, nil, false, nil, nil)
	state.ActiveNpc = mn
	// state.World intentionally left nil.

	err := Execute(state)
	if err == nil || err.Error() != "NPC_ARRIVEDELAY: no world" {
		t.Errorf("expected 'NPC_ARRIVEDELAY: no world', got %v", err)
	}
	if len(mn.setDelayedCalls) != 0 {
		t.Errorf("setDelayedCalls: got %v, want [] (rejection must not mutate)", mn.setDelayedCalls)
	}
}

// fakeWorldRemoveNpc records RemoveNpc calls. Embeds *mockWorld so the
// rest of the WorldVars surface stays no-op. Mirrors fakeWorldRemoveObj
// at handlers_obj_test.go:33.
type fakeWorldRemoveNpc struct {
	*mockWorld
	calls []struct {
		npc      ActiveNpc
		duration int
	}
}

func (f *fakeWorldRemoveNpc) RemoveNpc(npc ActiveNpc, duration int) {
	f.calls = append(f.calls, struct {
		npc      ActiveNpc
		duration int
	}{npc, duration})
}

func newNpcDelState(t *testing.T, npc ActiveNpc, world WorldVars) *ScriptState {
	t.Helper()
	sf := &ScriptFile{
		Name:             "npc_del",
		Opcodes:          []Opcode{OpNpcDel, OpReturn},
		IntOperands:      []int32{0, 0},
		StringOperands:   []string{"", ""},
		InstructionCount: 2,
	}
	state := Init(sf, nil, false, nil, nil)
	state.ActiveNpc = npc
	state.World = world
	state.Pointers |= PtrActiveNpc
	return state
}

func TestHandleNpcDel_CallsRemoveNpc(t *testing.T) {
	w := &fakeWorldRemoveNpc{mockWorld: newMockWorld()}
	npc := &mockNpc{typeID: 5, respawnrate: 50}
	state := newNpcDelState(t, npc, w)
	if err := Execute(state); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if len(w.calls) != 1 {
		t.Fatalf("RemoveNpc calls: got %d, want 1", len(w.calls))
	}
	if got, want := w.calls[0].duration, 50; got != want {
		t.Errorf("duration: got %d, want %d", got, want)
	}
}

func TestHandleNpcDel_PassesActiveNpcInstance(t *testing.T) {
	w := &fakeWorldRemoveNpc{mockWorld: newMockWorld()}
	npc := &mockNpc{typeID: 5, respawnrate: 50}
	state := newNpcDelState(t, npc, w)
	if err := Execute(state); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if len(w.calls) != 1 {
		t.Fatalf("RemoveNpc calls: got %d, want 1", len(w.calls))
	}
	if w.calls[0].npc != ActiveNpc(npc) {
		t.Errorf("npc identity mismatch: got %v, want %v", w.calls[0].npc, npc)
	}
}

func TestHandleNpcDel_NoActiveNpcErrors(t *testing.T) {
	w := &fakeWorldRemoveNpc{mockWorld: newMockWorld()}
	sf := &ScriptFile{
		Name:             "npc_del_no_active",
		Opcodes:          []Opcode{OpNpcDel, OpReturn},
		IntOperands:      []int32{0, 0},
		StringOperands:   []string{"", ""},
		InstructionCount: 2,
	}
	state := Init(sf, nil, false, nil, nil)
	state.World = w
	// Pointers flag NOT set → requireActiveNpc gate fires.
	err := Execute(state)
	if err == nil {
		t.Fatal("Execute: expected error, got nil")
	}
	if !strings.Contains(err.Error(), "NPC_DEL") {
		t.Errorf("error: %v, want containing \"NPC_DEL\"", err)
	}
	if len(w.calls) != 0 {
		t.Errorf("RemoveNpc calls: got %d, want 0", len(w.calls))
	}
}

func TestHandleNpcDel_NilWorldErrors(t *testing.T) {
	npc := &mockNpc{typeID: 5, respawnrate: 50}
	sf := &ScriptFile{
		Name:             "npc_del_nil_world",
		Opcodes:          []Opcode{OpNpcDel, OpReturn},
		IntOperands:      []int32{0, 0},
		StringOperands:   []string{"", ""},
		InstructionCount: 2,
	}
	state := Init(sf, nil, false, nil, nil)
	state.ActiveNpc = npc
	state.World = nil
	state.Pointers |= PtrActiveNpc
	err := Execute(state)
	if err == nil {
		t.Fatal("Execute: expected error, got nil")
	}
	if !strings.Contains(err.Error(), "no world surface") {
		t.Errorf("error: %v, want containing \"no world surface\"", err)
	}
}

func TestHandleNpcDel_ZeroRespawnrate(t *testing.T) {
	w := &fakeWorldRemoveNpc{mockWorld: newMockWorld()}
	npc := &mockNpc{typeID: 5, respawnrate: 0}
	state := newNpcDelState(t, npc, w)
	if err := Execute(state); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if len(w.calls) != 1 {
		t.Fatalf("RemoveNpc calls: got %d, want 1", len(w.calls))
	}
	if got, want := w.calls[0].duration, 0; got != want {
		t.Errorf("duration: got %d, want %d", got, want)
	}
}

// --- NAI-127 Bundle 1: NPC_FINDHERO (opcode 2519) ---

// newNpcFindHeroState builds a ScriptState with PtrActiveNpc set,
// ActiveNpc=npc, and World=mw, IntOperand-driven by intOperand.
func newNpcFindHeroState(npc ActiveNpc, mw WorldVars, intOperand int) *ScriptState {
	s := &ScriptState{
		World:       mw,
		ActiveNpc:   npc,
		Pointers:    PtrActiveNpc,
		IntStack:    make([]int, StackCapacity),
		StringStack: make([]string, StackCapacity),
	}
	s.Script = &ScriptFile{IntOperands: []int32{int32(intOperand)}}
	return s
}

func TestNpcFindHero_EmptyLedger(t *testing.T) {
	npc := &mockNpc{topContributor: 0}
	mw := &mockWorld{}
	s := newNpcFindHeroState(npc, mw, 0)
	if err := handleNpcFindHero(s); err != nil {
		t.Fatalf("NPC_FINDHERO empty: err=%v", err)
	}
	if got := s.PopInt(); got != 0 {
		t.Errorf("NPC_FINDHERO empty: pushed %d, want 0", got)
	}
	if s.Self != nil || s.Self2 != nil {
		t.Errorf("NPC_FINDHERO empty: Self/Self2 should remain nil")
	}
	if s.Pointers&(PtrActivePlayer|PtrActivePlayer2) != 0 {
		t.Errorf("NPC_FINDHERO empty: ActivePlayer pointer flags must not be set")
	}
}

func TestNpcFindHero_PrimarySlot(t *testing.T) {
	p := &mockPlayer{uidValue: 42}
	npc := &mockNpc{topContributor: 42}
	mw := &mockWorld{playersByUID: map[int]ActivePlayer{42: p}}
	s := newNpcFindHeroState(npc, mw, 0)
	if err := handleNpcFindHero(s); err != nil {
		t.Fatalf("NPC_FINDHERO primary: err=%v", err)
	}
	if got := s.PopInt(); got != 1 {
		t.Errorf("NPC_FINDHERO primary: pushed %d, want 1", got)
	}
	if s.Self != p {
		t.Errorf("NPC_FINDHERO primary: Self=%v, want %v", s.Self, p)
	}
	if s.Pointers&PtrActivePlayer == 0 {
		t.Errorf("NPC_FINDHERO primary: PtrActivePlayer flag must be set")
	}
}

func TestNpcFindHero_SecondarySlot(t *testing.T) {
	p := &mockPlayer{uidValue: 42}
	npc := &mockNpc{topContributor: 42}
	mw := &mockWorld{playersByUID: map[int]ActivePlayer{42: p}}
	s := newNpcFindHeroState(npc, mw, 1)
	if err := handleNpcFindHero(s); err != nil {
		t.Fatalf("NPC_FINDHERO secondary: err=%v", err)
	}
	if got := s.PopInt(); got != 1 {
		t.Errorf("NPC_FINDHERO secondary: pushed %d, want 1", got)
	}
	if s.Self2 != p {
		t.Errorf("NPC_FINDHERO secondary: Self2=%v, want %v", s.Self2, p)
	}
	if s.Pointers&PtrActivePlayer2 == 0 {
		t.Errorf("NPC_FINDHERO secondary: PtrActivePlayer2 flag must be set")
	}
}

func TestNpcFindHero_LookupReturnsNil(t *testing.T) {
	npc := &mockNpc{topContributor: 99}
	mw := &mockWorld{} // empty playersByUID
	s := newNpcFindHeroState(npc, mw, 0)
	if err := handleNpcFindHero(s); err != nil {
		t.Fatalf("NPC_FINDHERO loggedout: err=%v", err)
	}
	if got := s.PopInt(); got != 0 {
		t.Errorf("NPC_FINDHERO loggedout: pushed %d, want 0", got)
	}
}

func TestNpcFindHero_RequiresActiveNpc(t *testing.T) {
	mw := &mockWorld{}
	s := &ScriptState{
		World:       mw,
		IntStack:    make([]int, StackCapacity),
		StringStack: make([]string, StackCapacity),
	} // no PtrActiveNpc
	if err := handleNpcFindHero(s); err == nil {
		t.Fatalf("NPC_FINDHERO no-active-npc: want error, got nil")
	}
}

// --- NAI-160 T6: NPC_ATTACKRANGE (opcode 2503) ----------------------------

// TestNpcAttackRange pins OpNpcAttackRange's body: read activeNpc.type,
// checkNpcType, push NpcType.AttackRange (widened to int). Mirrors TS
// NpcOps.ts:521-523. NAI-160 T6.
func TestNpcAttackRange(t *testing.T) {
	mc := newTestConfigs()
	mc.npcs[7] = &objtype.NpcType{
		ConfigType:  objtype.ConfigType{ID: 7},
		AttackRange: 5,
	}
	npc := &mockNpc{typeID: 7}
	state := runNpcOp(t, npc, mc, OpNpcAttackRange, nil)
	if got := state.PopInt(); got != 5 {
		t.Errorf("NPC_ATTACKRANGE: got %d, want 5", got)
	}
}

// TestNpcAttackRangeRequiresActiveNpc pins the ActiveNpc gate. NAI-160 T6.
func TestNpcAttackRangeRequiresActiveNpc(t *testing.T) {
	sf := &ScriptFile{
		Name:             "[npc_attackrange_noactive,test]",
		Opcodes:          []Opcode{OpNpcAttackRange, OpReturn},
		IntOperands:      []int32{0, 0},
		StringOperands:   []string{"", ""},
		InstructionCount: 2,
	}
	state := Init(sf, nil, false, nil, nil)
	err := Execute(state)
	if err == nil {
		t.Fatalf("Execute: got nil err, want NPC_ATTACKRANGE: no active npc")
	}
	if got := err.Error(); !strings.Contains(got, "NPC_ATTACKRANGE") || !strings.Contains(got, "no active npc") {
		t.Errorf("err: got %q, want contains 'NPC_ATTACKRANGE' and 'no active npc'", got)
	}
}

// TestNpcAttackRangeInvalidType pins checkNpcType rejection. NAI-160 T6.
func TestNpcAttackRangeInvalidType(t *testing.T) {
	mc := newTestConfigs()
	npc := &mockNpc{typeID: 99}
	sf := &ScriptFile{
		Name:             "[npc_attackrange_badtype,test]",
		Opcodes:          []Opcode{OpNpcAttackRange, OpReturn},
		IntOperands:      []int32{0, 0},
		StringOperands:   []string{"", ""},
		InstructionCount: 2,
	}
	state := Init(sf, nil, false, nil, nil)
	state.ActiveNpc = npc
	state.Pointers |= PtrActiveNpc
	state.Configs = mc
	err := Execute(state)
	if err == nil {
		t.Fatalf("Execute: got nil err, want NPC_ATTACKRANGE: no NpcType")
	}
	if got := err.Error(); !strings.Contains(got, "NPC_ATTACKRANGE") {
		t.Errorf("err: got %q, want 'NPC_ATTACKRANGE' substring", got)
	}
}

// --- NAI-160 T7: NPC_INRANGE (opcode 2527) --------------------------------

// TestNpcInRangeTrue pins OpNpcInRange's body when the NPC's target is
// within max range: push 1. Mirrors TS NpcOps.ts:556-558. NAI-160 T7.
func TestNpcInRangeTrue(t *testing.T) {
	npc := &mockNpc{targetWithinMaxRangeValue: true}
	state := runNpcOp(t, npc, nil, OpNpcInRange, nil)
	if got := state.PopInt(); got != 1 {
		t.Errorf("NPC_INRANGE(true): got %d, want 1", got)
	}
}

// TestNpcInRangeFalse pins the false path (default zero-value). NAI-160 T7.
func TestNpcInRangeFalse(t *testing.T) {
	npc := &mockNpc{targetWithinMaxRangeValue: false}
	state := runNpcOp(t, npc, nil, OpNpcInRange, nil)
	if got := state.PopInt(); got != 0 {
		t.Errorf("NPC_INRANGE(false): got %d, want 0", got)
	}
}

// TestNpcInRangeRequiresActiveNpc pins the ActiveNpc gate. NAI-160 T7.
func TestNpcInRangeRequiresActiveNpc(t *testing.T) {
	sf := &ScriptFile{
		Name:             "[npc_inrange_noactive,test]",
		Opcodes:          []Opcode{OpNpcInRange, OpReturn},
		IntOperands:      []int32{0, 0},
		StringOperands:   []string{"", ""},
		InstructionCount: 2,
	}
	state := Init(sf, nil, false, nil, nil)
	err := Execute(state)
	if err == nil {
		t.Fatalf("Execute: got nil err, want NPC_INRANGE: no active npc")
	}
	if got := err.Error(); !strings.Contains(got, "NPC_INRANGE") || !strings.Contains(got, "no active npc") {
		t.Errorf("err: got %q, want contains 'NPC_INRANGE' and 'no active npc'", got)
	}
}
