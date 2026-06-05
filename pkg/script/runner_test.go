package script

import (
	"errors"
	"strings"
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

// TestExecuteHandlerErrorPrependsOpcodeMnemonic pins the script-core-1
// closure: the handler-error fault path in Execute prepends the lowercased
// opcode mnemonic to err.Error(). Mirrors TS ScriptRunner.ts:182 prefix.
//
// Strategy: register a temporary handler for an opcode unused by other
// tests (Opcode 8888, well above any real opcode value) that returns a
// known error. Verify the returned error wraps that error with the
// mnemonic prefix.
func TestExecuteHandlerErrorPrependsOpcodeMnemonic(t *testing.T) {
	const testOp Opcode = 8888
	sentinel := errors.New("boom")
	handlers[testOp] = func(*ScriptState) error { return sentinel }
	t.Cleanup(func() { delete(handlers, testOp) })

	f := &ScriptFile{
		Name:           "test",
		Opcodes:        []Opcode{testOp},
		IntOperands:    []int32{0},
		StringOperands: []string{""},
	}
	s := Init(f, nil, false, nil, nil)
	err := Execute(s)
	if err == nil {
		t.Fatal("expected handler error to surface, got nil")
	}
	if !errors.Is(err, sentinel) {
		t.Errorf("error chain lost sentinel: %v", err)
	}
	// testOp's String() returns "opcode_8888" via opcode.go:1283-1285
	// (Opcode.String fallback for undefined opcodes). Lowercased prefix
	// should land before the wrapped message.
	wantPrefix := "opcode_8888 "
	if !strings.HasPrefix(err.Error(), wantPrefix) {
		t.Errorf("expected error to start with %q, got %q", wantPrefix, err.Error())
	}
	if s.Execution != Aborted {
		t.Errorf("Execution: got %v want Aborted", s.Execution)
	}
}

// TestScriptOpcodePrefixVarpSecondaryDotFlag pins the '.' prefix for the
// VARP/VARN protected-variant marker. Mirrors TS ScriptRunner.ts:175-186:
// when the operand's bit 16 is set, the prefix is ".<opname> " — the
// RuneScript source-form notation for protected accesses.
func TestScriptOpcodePrefixVarpSecondaryDotFlag(t *testing.T) {
	// bit 16 set → secondary == 1 → "." prefix.
	const protectedOperand int32 = 1 << 16
	f := &ScriptFile{
		Name:        "test",
		Opcodes:     []Opcode{OpPopVarp},
		IntOperands: []int32{protectedOperand},
	}
	s := &ScriptState{Script: f, PC: 0}
	got := scriptOpcodePrefix(s, "")
	if !strings.HasPrefix(got, ".") {
		t.Errorf("expected '.' prefix for protected POP_VARP, got %q", got)
	}

	// bit 16 cleared → no '.' prefix.
	f.IntOperands[0] = 0
	got = scriptOpcodePrefix(s, "")
	if strings.HasPrefix(got, ".") {
		t.Errorf("unexpected '.' prefix for unprotected POP_VARP, got %q", got)
	}

	// PC out-of-range → empty.
	s.PC = 99
	if got := scriptOpcodePrefix(s, ""); got != "" {
		t.Errorf("expected empty prefix on PC out-of-range, got %q", got)
	}

	// Existing message already starts with the opcode name → suppressed
	// (goscape-convention dedup against handlers that self-embed the
	// opcode name in their error string).
	s.PC = 0
	if got := scriptOpcodePrefix(s, "POP_VARP: something bad"); got != "" {
		t.Errorf("expected empty prefix when existingMsg embeds opcode name, got %q", got)
	}
}

// TestBacktrace pins the per-frame format and ordering of Backtrace.
// Mirrors TS ScriptRunner.ts:194-201 — first frame is the current
// state.Script @ state.PC; subsequent frames are the GOSUB stack from
// most-recent (Frames[FrameSP-1]) down to oldest (Frames[0]).
func TestBacktrace(t *testing.T) {
	caller := &ScriptFile{
		Name:     "caller_script",
		FileName: "caller.rs2",
		PCs:      []uint32{0, 10},
		Lines:    []uint32{42, 50},
	}
	callee := &ScriptFile{
		Name:     "callee_script",
		FileName: "callee.rs2",
		PCs:      []uint32{0, 5},
		Lines:    []uint32{100, 110},
	}
	s := &ScriptState{
		Script: callee,
		PC:     5, // line 110 per the table above (threshold 5+ → final line)
		Frames: []Frame{
			{Script: caller, PC: 7}, // line 42 (threshold 10 > 7 → preceding line)
		},
		FrameSP: 1,
	}

	got := Backtrace(s)
	want := []string{
		"stack backtrace:",
		"    1: callee_script - callee.rs2:110",
		"    2: caller_script - caller.rs2:42",
	}
	if len(got) != len(want) {
		t.Fatalf("Backtrace lines: got %d want %d (got=%q)", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("Backtrace line %d: got %q want %q", i, got[i], want[i])
		}
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
	if s.Pointers&PtrProtectedActivePlayer == 0 {
		t.Errorf("Protect: PtrProtectedActivePlayer should be set, pointers=%b", s.Pointers)
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

// mockHintCoord captures the 4 args of a single HintCoord call for
// handler-test inspection. NAI-39.
type mockHintCoord struct{ offset, x, z, height int }

// mockPlayer is defined here for use in runner_test and handlers_test.
// It is also used in handlers_test.go in the same package.
type mockPlayer struct {
	messages    []string
	username    string
	displayName string
	playtime    int

	// NAI-35-T2: absolute world coords for ActivePlayer.X/Z, consumed
	// by MAP_PLAYERCOUNT's rect filter. Default zero is safe.
	x, z int

	// NAI-82: seeded by handler tests to drive P_ARRIVEDELAY's gate.
	lastMovement int

	// S4: captured calls from the suspension + queue methods.
	setDelayedCalls []int
	enqueueCalls    []mockEnqueue
	stored          *ScriptState
	cleared         int

	// NAI-26 Bundle 2: opt-in error return for EnqueueScriptArgs,
	// configured by tests that pin script-missing error propagation.
	// Default zero-value (nil error) preserves Bundle-1 mock behavior.
	enqueueScriptArgsReturnErr error

	// S5b: per-player varp storage for tests.
	varps       map[int]int32
	varpsString map[int]string

	// S5c: read-side storage. Tests pre-seed these; the getter methods
	// return the corresponding slot. 21 is the authentic stat count.
	levels      [21]int
	baseLevels  [21]int
	statXP      [21]int
	coordPacked int

	// S5c: captured calls from the mutation methods. Tests inspect these
	// to verify a handler made the expected call.
	lastTeleJump    struct{ x, z, level int }
	teleJumpCalls   int
	lastTeleport    struct{ x, z, level int }
	teleportCalls   int
	lastFaceSquare  struct{ x, z int }
	faceSquareCalls int

	setCurLevelCalls []struct{ id, level int }
	addXPCalls       []struct{ id, xp int }

	lastPlayAnim      struct{ seqID, delay int }
	playAnimCalls     int
	lastPlaySpotAnim  struct{ id, height, delay int }
	playSpotAnimCalls int

	lastReadyAnim int
	lastTurnAnim  int
	lastWalkAnim  int
	lastWalkAnimB int
	lastWalkAnimL int
	lastWalkAnimR int
	lastRunAnim   int

	// NAI-117 P_RUN: most recent value passed to SetRun(v); -1 sentinel
	// distinguishes "never called" from a legitimate v=0 walk-mode write.
	lastSetRun int

	// NAI-137: seeded by tests; mockPlayer.RunVarpID returns this. Default
	// 0 matches the TS VarPlayerType.RUN placeholder default.
	runVarpID int

	// NAI-117 RUNENERGY: configurable return for RunEnergy(); zero default
	// is fine for tests that don't pin a specific value.
	runenergyValue int

	// NAI-149: trivial-handler-sweep cohort backing fields.
	membersValue          bool
	runweightValue        int
	afkEventReadyValue    bool
	setAfkEventReadyCalls []bool // captures every SetAfkEventReady arg in order

	// S5f: captured calls from the interface / modal-control methods.
	lastCloseModalCalls    int
	lastOpenMain           int
	lastOpenChat           int
	lastOpenSide           int
	lastOpenMainSide       struct{ main, side int }
	lastOpenTutorial       int
	lastCloseTutorialCalls int
	lastFlashTutorial      int
	lastFlashTutorialCalls int

	lastIfSetText struct {
		com  int
		text string
	}
	lastIfSetModel      struct{ com, modelID int }
	lastIfSetNpcHead    struct{ com, npcID int }
	lastIfSetPlayerHead int // just com
	lastIfSetAnim       struct{ com, seqID int }
	lastIfSetHide       struct {
		com  int
		hide bool
	}
	lastIfSetTab       struct{ com, tab int }
	lastIfSetObject    struct{ com, objID, scale int }
	lastIfSetColour    struct{ com, colour int }
	lastIfSetPosition  struct{ com, x, y int }
	lastIfSetTabActive int // just tab

	lastSetResumeButtons [5]int

	lastComValue         int
	sendCountDialogCalls int

	// S5h: action-clear capture counters.
	stopActionCalls         int
	clearPendingActionCalls int
	requestLogoutCalls      int

	// S5i capture fields
	lastSetTimer struct {
		scriptID   uint32
		interval   int
		intArgs    []int
		stringArgs []string
		ttype      PlayerTimerType
	}
	setTimerCalls   int
	setTimerErr     error // NAI-27 Bundle 2: pre-seed for SetTimer error return
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

	// cameraPackets mirrors the production Player.cameraPackets accumulator
	// for handler-layer tests. CamMoveTo / CamLookAt append to this slice;
	// CamShake does NOT touch it (direct-write).
	cameraPackets []struct {
		kind                                                  uint8
		camX, camZ, height, rotationSpeed, rotationMultiplier int
	}
	lastCamShake *struct{ axis, random, amplitude, rate int }

	// NAI-37 T5: HINT_NPC capture. Each entry records the nid passed to
	// HintNpc; tests inspect this slice to verify a handler made the
	// expected call.
	hintNpcCalls []int

	// NAI-39: HintCoord / HintPlayer / HintStop captures (mirrors hintNpcCalls
	// shape). hintCoordCalls captures all 4 args via a struct slice;
	// hintPlayerCalls captures the slot int; hintStopCalls counts invocations.
	hintCoordCalls  []mockHintCoord
	hintPlayerCalls []int
	hintStopCalls   int

	// slot is the value returned by mockPlayer.Slot(); tests pre-seed it.
	slot int

	// NAI-120 Bundle 2B: BUSY2 read-side seeds.
	hasInteractionValue bool
	hasWaypointsValue   bool

	// NAI-163 B0: BUSY read-side seeds.
	busyValue       bool
	loggingOutValue bool

	// Staff-mod level (pre-seed for STAFFMODLEVEL query).
	staffModLevelValue int
	uidValue           int
	accountIDValue          int64
	recipientSessionValue   string

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

	// NAI-120 Bundle 2B: P_OPNPCT capture.
	lastSetInteractionScriptNpcT []struct {
		npc      ActiveNpc
		spellCom int
	}

	// NAI-120 Bundle 2B: P_OPPLAYER capture.
	lastSetInteractionScriptPlayer []struct {
		player2 ActivePlayer
		op      int
	}

	// NAI-115 T7: P_OPOBJ-side captures.
	queueWaypointCalls []struct{ x, z int }
	objOpCalls         []struct {
		obj ActiveObj
		op  int
	}

	// P_OPLOC InOperableDistance gate (TS PlayerOps.ts:396-398).
	// Defaults to false (out of range) so P_OPLOC tests that don't
	// pre-seed exercise the queueWaypoint branch by default. Tests
	// that want the "already in range, no waypoint" path set this to
	// true explicitly.
	inOperableDistanceValue bool
	inOperableDistanceCalls []ActiveLoc

	// S7a: canAccess return value. Defaults to false; tests that exercise
	// P_FINDUID positive paths set this to true explicitly.
	canAccessValue bool

	// S7b: anim-protect flag. Tests pre-seed to a sentinel (e.g. -2) so
	// they can assert "unchanged" vs. "set to 0".
	animProtectValue int

	// NAI-51: SetWalkTrigger captures. lastWalkTriggerSet is the last
	// scriptID passed to SetWalkTrigger; walkTriggerSetCalls counts
	// invocations (so error-path tests can assert the setter was NOT
	// reached). walkTriggerValue is pre-seeded by tests that exercise
	// GETWALKTRIGGER's read path.
	walkTriggerValue    int
	lastWalkTriggerSet  int
	walkTriggerSetCalls int

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

	// S7h: lowMemory pre-seed for MIDI_SONG / MIDI_JINGLE lowMemory-gate tests.
	lowMemoryValue bool

	// NAI-47: SETIDKIT gender + appearance captures.
	genderValue    int
	bodyParts      [7]int
	colorParts     [5]int
	setGenderCalls []int

	// NAI-127 Bundle 1: FINDHERO ledger-top getter.
	topContributor int

	// NAI-127 Bundle 1: BOTH_HEROPOINTS recipient recorder. Mirrors
	// mockNpc.addHeroPointsCalls.
	addHeroPointsCalls []struct{ playerUID, amount int }

	// NAI-120 Bundle 2D follow-up: HeroPointsClear() call counter.
	// Mirrors mockNpc.heroPointsClearCalls.
	heroPointsClearCalls int

	// changestat trigger fix: ChangeStat(id) call recorder. One entry per
	// invocation; STAT_ADD / STAT_SUB / STAT_BOOST / STAT_DRAIN / STAT_HEAL
	// fire ChangeStat after SetCurLevel when pre-clamp value != current.
	changeStatCalls []int

	// NAI-127 Bundle 2: DAMAGE recorder.
	applyDamageCalls []struct{ amount, dmgType int }

	// NAI-127 Bundle 2: P_PREVENTLOGOUT recorders.
	preventLogoutMessage string
	preventLogoutUntil   int

	// S7h: captured MIDI_SONG plays. Each entry records the normalized-name
	// argument as seen by the mock; the mock does not perform TS
	// normalization (that's (*Player).PlaySong's responsibility).
	playSongCalls []struct{ name string }

	// S7h: captured MIDI_JINGLE plays. Each entry records the delay and
	// the normalized-name argument as seen by the mock.
	playJingleCalls []struct {
		delay int
		name  string
	}

	// NAI-87: captured SOUND_SYNTH plays. Each entry records the three
	// int arguments passed to PlaySynth in TS argument order
	// (synth, loops, delay).
	playSynthCalls []struct {
		synth, loops, delay int
	}

	// NAI-74: SESSION_LOG opcode + Player.AddSessionLog capture.
	addSessionLogCalls []mockSessionLogCall

	// NAI-160 T1: SAY recorder. Defensive-copies the byte slice on Say()
	// to immunize from caller-mutates-buffer after the call.
	sayCalls [][]byte

	// NAI-160 T2/T3: HEADICONS_GET / HEADICONS_SET recorders.
	headiconsValue    int
	setHeadIconsCalls []int

	// NAI-160 T4: P_EXACTMOVE / UnsetMapFlag recorders.
	exactMoveCalls    []struct{ sX, sZ, eX, eZ, begin, finish, dir int }
	unsetMapFlagCalls int

	// P_WALK port T1: Walk recorder.
	walkCalls []walkCall

	// NAI-161 T1/T2: queue-introspection recorders.
	unlinkScriptCalls  []int       // every UnlinkQueuedScript call's scriptID
	queueCountByScript map[int]int // scriptID → return value; unset entries return 0

	// NAI-162 B1: trivial-handler sweep recorders.
	lastLoginInfoCalls       int
	invTotalParamStackReturn int // configurable for tests
	invTotalParamStackArgs   []invTotalParamStackArg
	addWealthEventCalls      []WealthEvent
}

type mockEnqueue struct {
	ScriptID   uint32
	Delay      int
	IntArgs    []int
	StringArgs []string
	Type       PlayerQueueType
}

type mockSessionLogCall struct {
	eventType int
	message   string
	args      []string
}

type walkCall struct {
	destX, destZ int
}

func (m *mockPlayer) MessageGame(msg string) { m.messages = append(m.messages, msg) }
func (m *mockPlayer) Username() string       { return m.username }
func (m *mockPlayer) DisplayName() string    { return m.displayName }

func (m *mockPlayer) SetDelayed(ticks int) {
	m.setDelayedCalls = append(m.setDelayedCalls, ticks)
}

func (m *mockPlayer) LastMovement() int { return m.lastMovement }
func (m *mockPlayer) EnqueueScriptArgs(id uint32, delay int, intArgs []int, stringArgs []string, qtype PlayerQueueType) error {
	m.enqueueCalls = append(m.enqueueCalls, mockEnqueue{ScriptID: id, Delay: delay, IntArgs: intArgs, StringArgs: stringArgs, Type: qtype})
	return m.enqueueScriptArgsReturnErr
}
func (m *mockPlayer) StoreActiveScript(s *ScriptState)         { m.stored = s }
func (m *mockPlayer) ClearActiveScript()                       { m.cleared++ }
func (m *mockPlayer) OnScriptFinishedOrAborted(_ *ScriptState) {}
func (m *mockPlayer) Playtime() int                            { return m.playtime }

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

func (m *mockPlayer) VarpString(id int) string {
	if m.varpsString == nil {
		return ""
	}
	return m.varpsString[id]
}
func (m *mockPlayer) SetVarpString(id int, val string) {
	if m.varpsString == nil {
		m.varpsString = make(map[int]string)
	}
	m.varpsString[id] = val
}
func (m *mockPlayer) RunVarpID() int { return m.runVarpID }

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

// ExactMove records the 7-arg call. NAI-160 T4.
func (m *mockPlayer) ExactMove(sX, sZ, eX, eZ, begin, finish, dir int) {
	m.exactMoveCalls = append(m.exactMoveCalls,
		struct{ sX, sZ, eX, eZ, begin, finish, dir int }{sX, sZ, eX, eZ, begin, finish, dir})
}

// UnsetMapFlag counts invocations. NAI-160 T4.
func (m *mockPlayer) UnsetMapFlag() { m.unsetMapFlagCalls++ }

func (m *mockPlayer) Walk(destX, destZ int) {
	m.walkCalls = append(m.walkCalls, walkCall{destX, destZ})
}

// NAI-161 T3: queue-introspection adapters.
func (m *mockPlayer) UnlinkQueuedScript(scriptID int) {
	m.unlinkScriptCalls = append(m.unlinkScriptCalls, scriptID)
}

func (m *mockPlayer) QueueCount(scriptID int) int {
	return m.queueCountByScript[scriptID]
}

// NAI-162 B1: trivial-handler sweep mock methods.

type invTotalParamStackArg struct {
	InvID, ParamID int
}

func (m *mockPlayer) LastLoginInfo() { m.lastLoginInfoCalls++ }

func (m *mockPlayer) InvTotalParamStack(inv, p int) int {
	m.invTotalParamStackArgs = append(m.invTotalParamStackArgs, invTotalParamStackArg{InvID: inv, ParamID: p})
	return m.invTotalParamStackReturn
}

func (m *mockPlayer) AddWealthEvent(evt WealthEvent) {
	m.addWealthEventCalls = append(m.addWealthEventCalls, evt)
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

func (m *mockPlayer) AddXP(id int, xp int, allowMulti bool) {
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

// NAI-117 P_RUN.
func (m *mockPlayer) SetRun(v int) { m.lastSetRun = v }

// NAI-117 RUNENERGY.
func (m *mockPlayer) RunEnergy() int { return m.runenergyValue }

// NAI-149.
func (m *mockPlayer) Members() bool       { return m.membersValue }
func (m *mockPlayer) RunWeight() int      { return m.runweightValue }
func (m *mockPlayer) AfkEventReady() bool { return m.afkEventReadyValue }
func (m *mockPlayer) SetAfkEventReady(v bool) {
	m.setAfkEventReadyCalls = append(m.setAfkEventReadyCalls, v)
	m.afkEventReadyValue = v
}
func (m *mockPlayer) SetRunEnergy(v int) { m.runenergyValue = v }

// S5f: interface / modal control.

func (m *mockPlayer) CloseModal(bool)  { m.lastCloseModalCalls++ }
func (m *mockPlayer) OpenMain(com int) { m.lastOpenMain = com }
func (m *mockPlayer) OpenChat(com int) { m.lastOpenChat = com }
func (m *mockPlayer) OpenSide(com int) { m.lastOpenSide = com }
func (m *mockPlayer) OpenMainModalSide(mainCom, sideCom int) {
	m.lastOpenMainSide = struct{ main, side int }{mainCom, sideCom}
}

func (m *mockPlayer) OpenTutorial(com int) { m.lastOpenTutorial = com }
func (m *mockPlayer) CloseTutorial()       { m.lastCloseTutorialCalls++ }
func (m *mockPlayer) FlashTutorial(tab int) {
	m.lastFlashTutorial = tab
	m.lastFlashTutorialCalls++
}

func (m *mockPlayer) IfSetText(com int, text string) {
	m.lastIfSetText = struct {
		com  int
		text string
	}{com, text}
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
	m.lastIfSetHide = struct {
		com  int
		hide bool
	}{com, hide}
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

// IfSetRecol deleted in 244 (IfSetRecolEncoder.ts removed upstream); mock method removed in B4 Task 2.
func (m *mockPlayer) IfSetTabActive(tab int) { m.lastIfSetTabActive = tab }

func (m *mockPlayer) SetResumeButtons(b1, b2, b3, b4, b5 int) {
	m.lastSetResumeButtons = [5]int{b1, b2, b3, b4, b5}
}

func (m *mockPlayer) LastCom() int     { return m.lastComValue }
func (m *mockPlayer) SendCountDialog() { m.sendCountDialogCalls++ }

// S5h: action-clear.

func (m *mockPlayer) StopAction()         { m.stopActionCalls++ }
func (m *mockPlayer) ClearPendingAction() { m.clearPendingActionCalls++ }
func (m *mockPlayer) RequestLogout()      { m.requestLogoutCalls++ }

// S5i: timer ops.

func (m *mockPlayer) SetTimer(scriptID uint32, interval int, intArgs []int, stringArgs []string, ttype PlayerTimerType) error {
	m.lastSetTimer = struct {
		scriptID   uint32
		interval   int
		intArgs    []int
		stringArgs []string
		ttype      PlayerTimerType
	}{scriptID, interval, intArgs, stringArgs, ttype}
	m.setTimerCalls++
	return m.setTimerErr
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

func (m *mockPlayer) CamShake(axis, random, amplitude, rate int) {
	m.lastCamShake = &struct{ axis, random, amplitude, rate int }{axis, random, amplitude, rate}
}

func (m *mockPlayer) CamMoveTo(camX, camZ, height, rate, rate2 int) {
	m.cameraPackets = append(m.cameraPackets, struct {
		kind                                                  uint8
		camX, camZ, height, rotationSpeed, rotationMultiplier int
	}{kind: 0, camX: camX, camZ: camZ, height: height, rotationSpeed: rate, rotationMultiplier: rate2})
}

func (m *mockPlayer) CamLookAt(camX, camZ, height, rate, rate2 int) {
	m.cameraPackets = append(m.cameraPackets, struct {
		kind                                                  uint8
		camX, camZ, height, rotationSpeed, rotationMultiplier int
	}{kind: 1, camX: camX, camZ: camZ, height: height, rotationSpeed: rate, rotationMultiplier: rate2})
}

// HintNpc capture for handler tests (NAI-37 T5).
func (m *mockPlayer) HintNpc(nid int) { m.hintNpcCalls = append(m.hintNpcCalls, nid) }

// NAI-39: HintCoord / HintPlayer / HintStop / Slot capture impls.
func (m *mockPlayer) HintCoord(offset, x, z, height int) {
	m.hintCoordCalls = append(m.hintCoordCalls, mockHintCoord{offset, x, z, height})
}
func (m *mockPlayer) HintPlayer(s int) { m.hintPlayerCalls = append(m.hintPlayerCalls, s) }
func (m *mockPlayer) HintStop()        { m.hintStopCalls++ }
func (m *mockPlayer) Slot() int        { return m.slot }

// StaffModLevel returns the seeded staff level for tests.
func (m *mockPlayer) StaffModLevel() int32 { return int32(m.staffModLevelValue) }
func (m *mockPlayer) UID() int             { return m.uidValue }
func (m *mockPlayer) AccountID() int64     { return m.accountIDValue }
func (m *mockPlayer) RecipientSession() string { return m.recipientSessionValue }

// NAI-35-T2: ActivePlayer.X/Z used by MAP_PLAYERCOUNT rect filter and
// future PlayerIterator passes-filter check.
func (m *mockPlayer) X() int { return m.x }
func (m *mockPlayer) Z() int { return m.z }

// CanAccess returns the seeded accessibility flag for P_FINDUID tests.
func (m *mockPlayer) CanAccess() bool { return m.canAccessValue }

// S7b: SetAnimProtect stores the anim-protect flag for P_ANIMPROTECT tests.
func (m *mockPlayer) SetAnimProtect(v int) { m.animProtectValue = v }

func (m *mockPlayer) WalkTrigger() int { return m.walkTriggerValue }

func (m *mockPlayer) SetWalkTrigger(scriptID int) {
	m.lastWalkTriggerSet = scriptID
	m.walkTriggerSetCalls++
}

// S7c: SetAppearanceInv stores the id + mask-set intent for BUILDAPPEARANCE
// tests. The bool captures the TS two-side-effects guarantee without porting
// real mask semantics into the mock.
func (m *mockPlayer) SetAppearanceInv(id int) {
	m.lastAppearanceInv = id
	m.appearanceInvCalls++
	m.appearanceMaskSet = true
}

// SetAllowDesign — see mockPlayer struct (runner_test.go:224) for field semantics.
func (m *mockPlayer) SetAllowDesign(v bool) {
	m.allowDesignValue = v
	m.allowDesignCalls++
}

// S7h: LowMemory returns the seeded value for MIDI_SONG / MIDI_JINGLE
// handler tests that exercise the lowMemory bail path.
func (m *mockPlayer) LowMemory() bool { return m.lowMemoryValue }

// NAI-47: SETIDKIT appearance-mutation captures.
func (m *mockPlayer) Gender() int                  { return m.genderValue }
func (m *mockPlayer) SetBodyPart(slot, idkit int)  { m.bodyParts[slot] = idkit }
func (m *mockPlayer) SetColorPart(slot, color int) { m.colorParts[slot] = color }

// SetGender captures SETGENDER dispatches for handler tests. The setter's
// real body-rewriting logic lives on modules/world.Player.SetGender; the
// mock only records the gender argument so handler-level tests can pin
// the popInt + checkGender + dispatch flow.
func (m *mockPlayer) SetGender(gender int) { m.setGenderCalls = append(m.setGenderCalls, gender) }

func (m *mockPlayer) TopContributor() int { return m.topContributor }

func (m *mockPlayer) AddHeroPoints(playerUID, amount int) {
	m.addHeroPointsCalls = append(m.addHeroPointsCalls, struct{ playerUID, amount int }{playerUID, amount})
}

// HeroPointsClear increments the call counter. NAI-120 Bundle 2D follow-up.
func (m *mockPlayer) HeroPointsClear() { m.heroPointsClearCalls++ }

// ChangeStat records each [changestat,<skill>] trigger fire-attempt by stat id.
func (m *mockPlayer) ChangeStat(id int) { m.changeStatCalls = append(m.changeStatCalls, id) }

func (m *mockPlayer) SetPreventLogout(message string, untilTick int) {
	m.preventLogoutMessage = message
	m.preventLogoutUntil = untilTick
}

func (m *mockPlayer) ApplyDamage(amount, dmgType int) {
	m.applyDamageCalls = append(m.applyDamageCalls, struct{ amount, dmgType int }{amount, dmgType})
}

// S7h: PlaySong captures the MIDI_SONG name for handler tests.
func (m *mockPlayer) PlaySong(name string) {
	m.playSongCalls = append(m.playSongCalls, struct{ name string }{name})
}

// S7h: PlayJingle captures the MIDI_JINGLE delay + name for handler tests.
func (m *mockPlayer) PlayJingle(delay int, name string) {
	m.playJingleCalls = append(m.playJingleCalls, struct {
		delay int
		name  string
	}{delay, name})
}

// NAI-87: PlaySynth captures the SOUND_SYNTH (synth, loops, delay)
// triple for handler tests. The mock does not encode anything;
// wire-format coverage lives in modules/world/sound_encoders_test.go.
func (m *mockPlayer) PlaySynth(synth, loops, delay int) {
	m.playSynthCalls = append(m.playSynthCalls, struct {
		synth, loops, delay int
	}{synth, loops, delay})
}

// NAI-74: AddSessionLog captures SESSION_LOG dispatch for handler tests.
func (m *mockPlayer) AddSessionLog(eventType int, message string, args ...string) {
	// Defensive copy of args (variadic slice may alias caller storage).
	cp := make([]string, len(args))
	copy(cp, args)
	m.addSessionLogCalls = append(m.addSessionLogCalls, mockSessionLogCall{
		eventType: eventType,
		message:   message,
		args:      cp,
	})
}

// Say records the byte slice passed by handleSay. Defensive byte-copy
// to avoid caller-mutates-buffer aliasing. NAI-160 T1.
func (m *mockPlayer) Say(text []byte) {
	m.sayCalls = append(m.sayCalls, append([]byte(nil), text...))
}

// HeadIcons returns the seeded headiconsValue. NAI-160 T2.
func (m *mockPlayer) HeadIcons() int { return m.headiconsValue }

// SetHeadIcons records the write AND updates headiconsValue so a
// subsequent HeadIcons() read returns the new value (mirrors TS direct
// field assignment). NAI-160 T3.
func (m *mockPlayer) SetHeadIcons(v int) {
	m.setHeadIconsCalls = append(m.setHeadIconsCalls, v)
	m.headiconsValue = v
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

func (m *mockPlayer) SetInteractionScriptNpcT(npc ActiveNpc, spellCom int) {
	m.lastSetInteractionScriptNpcT = append(m.lastSetInteractionScriptNpcT, struct {
		npc      ActiveNpc
		spellCom int
	}{npc, spellCom})
}

func (m *mockPlayer) SetInteractionScriptPlayer(player2 ActivePlayer, op int) {
	m.lastSetInteractionScriptPlayer = append(m.lastSetInteractionScriptPlayer, struct {
		player2 ActivePlayer
		op      int
	}{player2, op})
}

func (m *mockPlayer) HasInteraction() bool { return m.hasInteractionValue }
func (m *mockPlayer) HasWaypoints() bool   { return m.hasWaypointsValue }
func (m *mockPlayer) Busy() bool           { return m.busyValue }
func (m *mockPlayer) LoggingOut() bool     { return m.loggingOutValue }

func (m *mockPlayer) QueueWaypoint(x, z int) {
	m.queueWaypointCalls = append(m.queueWaypointCalls, struct{ x, z int }{x, z})
}

func (m *mockPlayer) InOperableDistance(loc ActiveLoc) bool {
	m.inOperableDistanceCalls = append(m.inOperableDistanceCalls, loc)
	return m.inOperableDistanceValue
}

func (m *mockPlayer) SetInteractionScriptObj(obj ActiveObj, op int) {
	m.objOpCalls = append(m.objOpCalls, struct {
		obj ActiveObj
		op  int
	}{obj, op})
}

// mockNpcLookup is a test double for script.NpcLookup. Tests set the
// per-method return fields and assert call-capture afterwards. Mirrors
// the mockPlayer "value + counter" pattern (runner_test.go:224-228,
// S7e precedent). lastArgs captures the most recent call's args as an
// []int so handler tests can cross-check arg ordering.
type mockNpcLookup struct {
	byType     ActiveNpc
	byCategory ActiveNpc
	atCoord    ActiveNpc
	// byZone returns the NPC slice keyed by (level, zoneX, zoneZ) tuple
	// packed via mockZoneKey(level, zoneX, zoneZ). nil entry = empty.
	byZone map[uint64][]ActiveNpc
	// byUID returns the NPC keyed by uid. nil entry = miss. NAI-120 Bundle 2A.
	byUID map[int]ActiveNpc

	byTypeCalls     int
	byCategoryCalls int
	atCoordCalls    int
	zoneNpcsCalls   int
	byUIDCalls      int

	lastArgs []int
	// zoneNpcsCallArgs records each ZoneNpcs call's (level, zoneX, zoneZ)
	// in call order — used by iterator-cursor-order tests to assert
	// the iterator visits zones in TS line 337-340 order.
	zoneNpcsCallArgs [][3]int
}

func mockZoneKey(level, zoneX, zoneZ int) uint64 {
	return uint64(level&0x3)<<28 | uint64(zoneX&0x3FFF)<<14 | uint64(zoneZ&0x3FFF)
}

func (m *mockNpcLookup) FindClosestNpcByType(level, x, z, dist, typeID, huntvis int) ActiveNpc {
	m.byTypeCalls++
	m.lastArgs = []int{level, x, z, dist, typeID, huntvis}
	return m.byType
}

func (m *mockNpcLookup) FindClosestNpcByCategory(level, x, z, dist, cat, huntvis int) ActiveNpc {
	m.byCategoryCalls++
	m.lastArgs = []int{level, x, z, dist, cat, huntvis}
	return m.byCategory
}

func (m *mockNpcLookup) FindNpcAtExactCoord(level, x, z, typeID int) ActiveNpc {
	m.atCoordCalls++
	m.lastArgs = []int{level, x, z, typeID}
	return m.atCoord
}

func (m *mockNpcLookup) ZoneNpcs(level, zoneX, zoneZ int) []ActiveNpc {
	m.zoneNpcsCalls++
	m.zoneNpcsCallArgs = append(m.zoneNpcsCallArgs, [3]int{level, zoneX, zoneZ})
	if m.byZone == nil {
		return nil
	}
	return m.byZone[mockZoneKey(level, zoneX, zoneZ)]
}

func (m *mockNpcLookup) FindNpcByUID(uid int) ActiveNpc {
	m.byUIDCalls++
	m.lastArgs = []int{uid}
	if m.byUID == nil {
		return nil
	}
	return m.byUID[uid]
}
