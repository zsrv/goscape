package script

import (
	"reflect"
	"strings"
	"testing"

	"github.com/zsrv/goscape/pkg/objtype"
)

type mockEnqueueCall struct {
	trigger ServerTriggerType
	delay   int
	intArg  int
}

// mockNpc is a test fixture implementing ActiveNpc. Pre-seed fields then
// assign to state.ActiveNpc before Execute.
type mockNpc struct {
	typeID, x, z, level, uid, category int
	curHP, baseHP                      int
	varns                              map[int]int32
	sayCalls                           []string
	animCalls                          []struct{ id, delay int }
	faceCoordCalls                     []struct{ x, z int }
	changeTypeCalls                    []struct{ newType, duration int }
	damageCalls                        []struct{ amount, dmgType int }
	enqueueCalls                       []mockEnqueueCall
	setTimerCalls                      []int
	setHuntRangeCalls                  []int
	setHuntModeCalls                   []int
}

func (m *mockNpc) NpcType() int     { return m.typeID }
func (m *mockNpc) NpcX() int        { return m.x }
func (m *mockNpc) NpcZ() int        { return m.z }
func (m *mockNpc) NpcLevel() int    { return m.level }
func (m *mockNpc) NpcUID() int      { return m.uid }
func (m *mockNpc) NpcCategory() int { return m.category }

func (m *mockNpc) NpcStat(stat int) int {
	if stat == 0 {
		return m.curHP
	}
	return 0
}

func (m *mockNpc) NpcBaseStat(stat int) int {
	if stat == 0 {
		return m.baseHP
	}
	return 0
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

func (m *mockNpc) Damage(amount, dmgType int) {
	m.damageCalls = append(m.damageCalls, struct{ amount, dmgType int }{amount, dmgType})
}

func (m *mockNpc) StoreActiveScript(_ *ScriptState) {}
func (m *mockNpc) ClearActiveScript()               {}
func (m *mockNpc) SetDelayed(_ int)                 {}

func (m *mockNpc) EnqueueScriptForTrigger(trigger ServerTriggerType, delay, intArg int) {
	m.enqueueCalls = append(m.enqueueCalls, mockEnqueueCall{
		trigger: trigger,
		delay:   delay,
		intArg:  intArg,
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

func TestNpcMutatingOpsRequireActiveNpc_S6c(t *testing.T) {
	cases := []struct {
		op   Opcode
		name string
	}{
		{OpNpcAnim, "NPC_ANIM"},
		{OpNpcFaceSquare, "NPC_FACESQUARE"},
		{OpNpcChangeType, "NPC_CHANGETYPE"},
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
	if call.intArg != 42 {
		t.Errorf("intArg: got %d, want 42", call.intArg)
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

