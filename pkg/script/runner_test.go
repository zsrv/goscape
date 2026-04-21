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
