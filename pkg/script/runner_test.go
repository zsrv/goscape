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
}

type mockEnqueue struct {
	ScriptID uint32
	Delay    int
	IntArg   int
}

func (m *mockPlayer) MessageGame(msg string) { m.messages = append(m.messages, msg) }
func (m *mockPlayer) Username() string       { return m.username }

func (m *mockPlayer) SetDelayed(ticks int) {
	m.setDelayedCalls = append(m.setDelayedCalls, ticks)
}
func (m *mockPlayer) EnqueueScript(id uint32, delay, arg int) {
	m.enqueueCalls = append(m.enqueueCalls, mockEnqueue{ScriptID: id, Delay: delay, IntArg: arg})
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
