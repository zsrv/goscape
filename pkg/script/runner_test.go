package script

import (
	"testing"
)

func TestExecuteUnknownOpcodeAborts(t *testing.T) {
	// Opcode 9999 has no handler → Aborted + non-nil error.
	f := &ScriptFile{
		Name:           "test",
		Opcodes:        []Opcode{9999},
		IntOperands:    []int32{0},
		StringOperands: []string{""},
	}
	s := Init(f, nil, false, nil, nil)
	err := Execute(s)
	if err == nil {
		t.Fatal("expected error for unknown opcode, got nil")
	}
	if s.Execution != Aborted {
		t.Errorf("Execution: got %v want Aborted", s.Execution)
	}
}

func TestExecutePcOutOfRangeAborts(t *testing.T) {
	// Empty Opcodes slice: PC=0 is immediately out of range.
	f := &ScriptFile{
		Name:    "test",
		Opcodes: []Opcode{},
	}
	s := Init(f, nil, false, nil, nil)
	err := Execute(s)
	if err == nil {
		t.Fatal("expected error for pc out of range, got nil")
	}
	if s.Execution != Aborted {
		t.Errorf("Execution: got %v want Aborted", s.Execution)
	}
}

func TestInitSetsFields(t *testing.T) {
	f := minimalScript(OpReturn)
	f.IntArgCount = 2
	f.StringArgCount = 1
	f.IntLocalCount = 3
	f.StringLocalCount = 2

	mp := &mockPlayer{username: "Alice"}
	s := Init(f, mp, true, []int{10, 20}, []string{"hello"})

	if s.Self != mp {
		t.Error("Self not set")
	}
	if s.Pointers&PtrActivePlayer == 0 {
		t.Error("PtrActivePlayer not set")
	}
	if s.Protect != true {
		t.Error("Protect not set")
	}
	if s.PC != 0 {
		t.Errorf("PC: got %d want 0", s.PC)
	}
	if s.Execution != Running {
		t.Errorf("Execution: got %v want Running", s.Execution)
	}
	if s.IntLocals[0] != 10 || s.IntLocals[1] != 20 {
		t.Errorf("IntLocals: got %v want [10,20,...]", s.IntLocals)
	}
	if s.StringLocals[0] != "hello" {
		t.Errorf("StringLocals[0]: got %q want %q", s.StringLocals[0], "hello")
	}
	if cap(s.IntStack) != StackCapacity {
		t.Errorf("IntStack cap: got %d want %d", cap(s.IntStack), StackCapacity)
	}
}

type mockInvListen struct {
	InvType int
	Com     int
	Source  int
}

type mockLocOp struct {
	Loc ActiveLoc
	Op  int
}

type mockNpcOp struct {
	Npc ActiveNpc
	Op  int
}

// mockPlayer is defined here for use in runner_test and handlers_test.
// It is also used in handlers_test.go in the same package.
type mockPlayer struct {
	messages []string
	username string
	playtime int

	// S4: captured calls from the suspension + queue methods.
	setDelayedCalls []int
	enqueueCalls    []mockEnqueue
	stored          *ScriptState
	cleared         int

	// S5b: per-player varp storage for tests.
	varps map[int]int32

	// S5c: read-side storage. Tests pre-seed these; the getter methods
	// return the corresponding slot. 21 is the authentic stat count.
	levels      [21]int
	baseLevels  [21]int
	statXP      [21]int
	coordPacked int

	// S5c: captured calls from the mutation methods. Tests inspect these
	// to verify a handler made the expected call.
	lastTeleJump  struct{ x, z, level int }
	teleJumpCalls int
	lastTeleport  struct{ x, z, level int }
	teleportCalls int
	lastFaceSquare struct{ x, z int }
	faceSquareCalls int

	setCurLevelCalls []struct{ id, level int }
	addXPCalls       []struct{ id, xp int }

	lastPlayAnim     struct{ seqID, delay int }
	playAnimCalls    int
	lastPlaySpotAnim struct{ id, height, delay int }
	playSpotAnimCalls int

	lastReadyAnim  int
	lastTurnAnim   int
	lastWalkAnim   int
	lastWalkAnimB  int
	lastWalkAnimL  int
	lastWalkAnimR  int
	lastRunAnim    int

	// S5f: captured calls from the interface / modal-control methods.
	lastCloseModalCalls int
	lastOpenMain        int
	lastOpenChat        int
	lastOpenSide        int
	lastOpenMainSide    struct{ main, side int }

	lastIfSetText       struct{ com int; text string }
	lastIfSetModel      struct{ com, modelID int }
	lastIfSetNpcHead    struct{ com, npcID int }
	lastIfSetPlayerHead int // just com
	lastIfSetAnim       struct{ com, seqID int }
	lastIfSetHide       struct{ com int; hide bool }
	lastIfSetTab        struct{ com, tab int }
	lastIfSetObject     struct{ com, objID, scale int }
	lastIfSetColour     struct{ com, colour int }
	lastIfSetPosition   struct{ com, x, y int }
	lastIfSetRecol      struct{ com, src, dst int }
	lastIfSetTabActive  int // just tab

	lastSetResumeButtons [5]int

	lastComValue         int
	sendCountDialogCalls int

	// S5h: action-clear capture counters.
	stopActionCalls         int
	clearPendingActionCalls int

	// S5i capture fields
	lastSetTimer    struct{ scriptID uint32; interval, intArg int; ttype PlayerTimerType }
	setTimerCalls   int
	lastClearTimer  uint32
	clearTimerCalls int
	getTimerValue   int // pre-seed for GetTimer return

	// S5m: last-input captures (pre-seed these for the Last* queries).
	lastItemValue       int
	lastSlotValue       int
	lastUseItemValue    int
	lastUseSlotValue    int
	lastTargetSlotValue int

	// Camera control capture.
	camResetCalls int

	// Staff-mod level (pre-seed for STAFFMODLEVEL query).
	staffModLevelValue int
	uidValue           int

	// S6l: p_aprange capture fields.
	lastApRange       int
	lastApRangeCalled bool
	setApRangeCalls   int

	// S6m: spellCom pre-seed for TargetSubjectCom query.
	targetSubjectComValue int

	// S6u: inv listener registration captures.
	lastInvListenOnCom     []mockInvListen
	lastInvStopListenOnCom []int

	// S6v: p_op* script-queued interaction captures.
	lastSetInteractionScriptLoc []mockLocOp
	lastSetInteractionScriptNpc []mockNpcOp

	// S7a: canAccess return value. Defaults to false; tests that exercise
	// P_FINDUID positive paths set this to true explicitly.
	canAccessValue bool

	// S7b: anim-protect flag. Tests pre-seed to a sentinel (e.g. -2) so
	// they can assert "unchanged" vs. "set to 0".
	animProtectValue int

	// S7c: BUILDAPPEARANCE captures. lastAppearanceInv is the last id passed
	// to SetAppearanceInv; appearanceInvCalls counts invocations (0 verifies
	// the setter was NOT reached for error paths); appearanceMaskSet tracks
	// whether the setter flipped the mask side-effect (mockPlayer has no real
	// masks field, so we capture intent as a bool).
	lastAppearanceInv  int
	appearanceInvCalls int
	appearanceMaskSet  bool

	// S7e: SetAllowDesign stores the coerced-bool flag for ALLOWDESIGN tests.
	// allowDesignCalls counts invocations so error-path tests can assert the
	// setter was NOT called.
	allowDesignValue bool
	allowDesignCalls int
}

type mockEnqueue struct {
	ScriptID uint32
	Delay    int
	IntArg   int
	Type     PlayerQueueType
}

func (m *mockPlayer) MessageGame(msg string) { m.messages = append(m.messages, msg) }
func (m *mockPlayer) Username() string       { return m.username }

func (m *mockPlayer) SetDelayed(ticks int) {
	m.setDelayedCalls = append(m.setDelayedCalls, ticks)
}
func (m *mockPlayer) EnqueueScriptTyped(id uint32, delay, arg int, qtype PlayerQueueType) {
	m.enqueueCalls = append(m.enqueueCalls, mockEnqueue{ScriptID: id, Delay: delay, IntArg: arg, Type: qtype})
}
func (m *mockPlayer) StoreActiveScript(s *ScriptState) { m.stored = s }
func (m *mockPlayer) ClearActiveScript()               { m.cleared++ }
func (m *mockPlayer) Playtime() int                    { return m.playtime }

func (m *mockPlayer) Varp(id int) int32 {
	if m.varps == nil {
		return 0
	}
	return m.varps[id]
}
func (m *mockPlayer) SetVarp(id int, val int32) {
	if m.varps == nil {
		m.varps = make(map[int]int32)
	}
	m.varps[id] = val
}

// S5c: position / facing / teleport.

func (m *mockPlayer) CoordPacked() int { return m.coordPacked }

func (m *mockPlayer) TeleJump(x, z, level int) {
	m.lastTeleJump = struct{ x, z, level int }{x, z, level}
	m.teleJumpCalls++
}

func (m *mockPlayer) Teleport(x, z, level int) {
	m.lastTeleport = struct{ x, z, level int }{x, z, level}
	m.teleportCalls++
}

func (m *mockPlayer) FaceSquare(x, z int) {
	m.lastFaceSquare = struct{ x, z int }{x, z}
	m.faceSquareCalls++
}

// S5c: stats.

func (m *mockPlayer) Stat(id int) int {
	if id < 0 || id >= len(m.levels) {
		return 0
	}
	return m.levels[id]
}

func (m *mockPlayer) StatBase(id int) int {
	if id < 0 || id >= len(m.baseLevels) {
		return 0
	}
	return m.baseLevels[id]
}

func (m *mockPlayer) StatXP(id int) int {
	if id < 0 || id >= len(m.statXP) {
		return 0
	}
	return m.statXP[id]
}

func (m *mockPlayer) SetCurLevel(id int, level int) {
	m.setCurLevelCalls = append(m.setCurLevelCalls, struct{ id, level int }{id, level})
}

func (m *mockPlayer) AddXP(id int, xp int) {
	m.addXPCalls = append(m.addXPCalls, struct{ id, xp int }{id, xp})
}

// S5c: animation.

func (m *mockPlayer) PlayAnim(seqID, delay int) {
	m.lastPlayAnim = struct{ seqID, delay int }{seqID, delay}
	m.playAnimCalls++
}

func (m *mockPlayer) PlaySpotAnim(id, height, delay int) {
	m.lastPlaySpotAnim = struct{ id, height, delay int }{id, height, delay}
	m.playSpotAnimCalls++
}

func (m *mockPlayer) SetReadyAnim(seqID int) { m.lastReadyAnim = seqID }
func (m *mockPlayer) SetTurnAnim(seqID int)  { m.lastTurnAnim = seqID }
func (m *mockPlayer) SetWalkAnim(seqID int)  { m.lastWalkAnim = seqID }
func (m *mockPlayer) SetWalkAnimB(seqID int) { m.lastWalkAnimB = seqID }
func (m *mockPlayer) SetWalkAnimL(seqID int) { m.lastWalkAnimL = seqID }
func (m *mockPlayer) SetWalkAnimR(seqID int) { m.lastWalkAnimR = seqID }
func (m *mockPlayer) SetRunAnim(seqID int)   { m.lastRunAnim = seqID }

// S5f: interface / modal control.

func (m *mockPlayer) CloseModal()            { m.lastCloseModalCalls++ }
func (m *mockPlayer) OpenMain(com int)       { m.lastOpenMain = com }
func (m *mockPlayer) OpenChat(com int)       { m.lastOpenChat = com }
func (m *mockPlayer) OpenSide(com int)       { m.lastOpenSide = com }
func (m *mockPlayer) OpenMainSide(mainCom, sideCom int) {
	m.lastOpenMainSide = struct{ main, side int }{mainCom, sideCom}
}

func (m *mockPlayer) IfSetText(com int, text string) {
	m.lastIfSetText = struct{ com int; text string }{com, text}
}
func (m *mockPlayer) IfSetModel(com, modelID int) {
	m.lastIfSetModel = struct{ com, modelID int }{com, modelID}
}
func (m *mockPlayer) IfSetNpcHead(com, npcID int) {
	m.lastIfSetNpcHead = struct{ com, npcID int }{com, npcID}
}
func (m *mockPlayer) IfSetPlayerHead(com int) { m.lastIfSetPlayerHead = com }
func (m *mockPlayer) IfSetAnim(com, seqID int) {
	m.lastIfSetAnim = struct{ com, seqID int }{com, seqID}
}
func (m *mockPlayer) IfSetHide(com int, hide bool) {
	m.lastIfSetHide = struct{ com int; hide bool }{com, hide}
}
func (m *mockPlayer) IfSetTab(com, tab int) {
	m.lastIfSetTab = struct{ com, tab int }{com, tab}
}
func (m *mockPlayer) IfSetObject(com, objID, scale int) {
	m.lastIfSetObject = struct{ com, objID, scale int }{com, objID, scale}
}
func (m *mockPlayer) IfSetColour(com, colour int) {
	m.lastIfSetColour = struct{ com, colour int }{com, colour}
}
func (m *mockPlayer) IfSetPosition(com, x, y int) {
	m.lastIfSetPosition = struct{ com, x, y int }{com, x, y}
}
func (m *mockPlayer) IfSetRecol(com, srcColour, dstColour int) {
	m.lastIfSetRecol = struct{ com, src, dst int }{com, srcColour, dstColour}
}
func (m *mockPlayer) IfSetTabActive(tab int) { m.lastIfSetTabActive = tab }

func (m *mockPlayer) SetResumeButtons(b1, b2, b3, b4, b5 int) {
	m.lastSetResumeButtons = [5]int{b1, b2, b3, b4, b5}
}

func (m *mockPlayer) LastCom() int     { return m.lastComValue }
func (m *mockPlayer) SendCountDialog() { m.sendCountDialogCalls++ }

// S5h: action-clear.

func (m *mockPlayer) StopAction()         { m.stopActionCalls++ }
func (m *mockPlayer) ClearPendingAction() { m.clearPendingActionCalls++ }

// S5i: timer ops.

func (m *mockPlayer) SetTimer(scriptID uint32, interval, intArg int, ttype PlayerTimerType) {
	m.lastSetTimer = struct{ scriptID uint32; interval, intArg int; ttype PlayerTimerType }{scriptID, interval, intArg, ttype}
	m.setTimerCalls++
}
func (m *mockPlayer) ClearTimer(scriptID uint32) {
	m.lastClearTimer = scriptID
	m.clearTimerCalls++
}
func (m *mockPlayer) GetTimer(scriptID uint32) int { return m.getTimerValue }

// S5m: last-input query captures.
func (m *mockPlayer) LastItem() int       { return m.lastItemValue }
func (m *mockPlayer) LastSlot() int       { return m.lastSlotValue }
func (m *mockPlayer) LastUseItem() int    { return m.lastUseItemValue }
func (m *mockPlayer) LastUseSlot() int    { return m.lastUseSlotValue }
func (m *mockPlayer) LastTargetSlot() int { return m.lastTargetSlotValue }

// CamReset capture for handler tests.
func (m *mockPlayer) CamReset() { m.camResetCalls++ }

// StaffModLevel returns the seeded staff level for tests.
func (m *mockPlayer) StaffModLevel() int32 { return int32(m.staffModLevelValue) }
func (m *mockPlayer) UID() int              { return m.uidValue }

// CanAccess returns the seeded accessibility flag for P_FINDUID tests.
func (m *mockPlayer) CanAccess() bool { return m.canAccessValue }

// S7b: SetAnimProtect stores the anim-protect flag for P_ANIMPROTECT tests.
func (m *mockPlayer) SetAnimProtect(v int) { m.animProtectValue = v }

// S7c: SetAppearanceInv stores the id + mask-set intent for BUILDAPPEARANCE
// tests. The bool captures the TS two-side-effects guarantee without porting
// real mask semantics into the mock.
func (m *mockPlayer) SetAppearanceInv(id int) {
	m.lastAppearanceInv = id
	m.appearanceInvCalls++
	m.appearanceMaskSet = true
}

// S7e: SetAllowDesign stores the coerced-bool flag for ALLOWDESIGN tests.
// allowDesignCalls counts invocations so error-path tests can assert the
// setter was NOT called.
func (m *mockPlayer) SetAllowDesign(v bool) {
	m.allowDesignValue = v
	m.allowDesignCalls++
}

// S6l: p_aprange.
func (m *mockPlayer) SetApRange(n int) {
	m.lastApRange = n
	m.lastApRangeCalled = true
	m.setApRangeCalls++
}

// S6m: spellCom slot read.
func (m *mockPlayer) TargetSubjectCom() int { return m.targetSubjectComValue }

// S6u: inv listener registration.
func (m *mockPlayer) InvListenOnCom(invType, com, source int) {
	m.lastInvListenOnCom = append(m.lastInvListenOnCom, mockInvListen{InvType: invType, Com: com, Source: source})
}

func (m *mockPlayer) InvStopListenOnCom(com int) {
	m.lastInvStopListenOnCom = append(m.lastInvStopListenOnCom, com)
}

// S6v: p_op* script-queued interaction captures.
func (m *mockPlayer) SetInteractionScriptLoc(loc ActiveLoc, op int) {
	m.lastSetInteractionScriptLoc = append(m.lastSetInteractionScriptLoc, mockLocOp{Loc: loc, Op: op})
}

func (m *mockPlayer) SetInteractionScriptNpc(npc ActiveNpc, op int) {
	m.lastSetInteractionScriptNpc = append(m.lastSetInteractionScriptNpc, mockNpcOp{Npc: npc, Op: op})
}
