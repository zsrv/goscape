package world

import (
	"testing"

	entitypkg "github.com/zsrv/goscape/pkg/entity"
	"github.com/zsrv/goscape/pkg/gamemap"
	"github.com/zsrv/goscape/pkg/objtype"
	"github.com/zsrv/goscape/pkg/rsbuf"
	"github.com/zsrv/goscape/pkg/script"
)

func newNpcForScriptTest(t *testing.T) *Npc {
	t.Helper()
	typ := &objtype.NpcType{
		ConfigType: objtype.ConfigType{ID: 0, DebugName: "test_npc"},
		Size:       1, // match production NewNpcType default (NAI-18).
	}
	return NewNpc(1, 0, 3094, 3106, 0, typ)
}

func TestNpcStoreAndClearActiveScript(t *testing.T) {
	n := newNpcForScriptTest(t)
	state := &script.ScriptState{}

	n.StoreActiveScript(state)
	if n.activeScript != state {
		t.Errorf("StoreActiveScript: got %v, want %v", n.activeScript, state)
	}

	n.ClearActiveScript()
	if n.activeScript != nil {
		t.Errorf("ClearActiveScript: got %v, want nil", n.activeScript)
	}
}

func TestNpcSetDelayed(t *testing.T) {
	n := newNpcForScriptTest(t)
	s := &Server{}
	s.currentTick = 100
	n.server = s

	n.SetDelayed(5)

	if !n.delayed {
		t.Errorf("delayed: got false, want true")
	}
	want := 100 + 1 + 5
	if n.delayedUntil != want {
		t.Errorf("delayedUntil: got %d, want %d", n.delayedUntil, want)
	}
}

// newServerForScriptTest builds a minimal *Server wired for running
// NPC-anchored scripts. Reuses the pattern from script_test.go:939-952.
func newServerForScriptTest(t *testing.T) *Server {
	t.Helper()
	return &Server{
		log:   discardLogger(),
		rsbuf: rsbuf.New(),
	}
}

func TestRunNpcScriptFiresAndFinishes(t *testing.T) {
	s := newServerForScriptTest(t)
	s.currentTick = 100
	n := newNpcForScriptTest(t)
	n.server = s

	sf := &script.ScriptFile{
		Name:    "trivial_return",
		Opcodes: []script.Opcode{script.OpReturn},
	}

	s.runNpcScript(sf, n, nil, script.TriggerProc, nil, nil)

	if n.activeScript != nil {
		t.Errorf("activeScript: got %v, want nil (script finished)", n.activeScript)
	}
	if n.delayed {
		t.Errorf("delayed: got true, want false")
	}
}

func TestRunNpcScriptSuspendsOnNpcDelay(t *testing.T) {
	s := newServerForScriptTest(t)
	s.currentTick = 100
	n := newNpcForScriptTest(t)
	n.server = s

	sf := &script.ScriptFile{
		Name:        "npc_delay_3_return",
		Opcodes:     []script.Opcode{script.OpPushConstantInt, script.OpNpcDelay, script.OpReturn},
		IntOperands: []int32{3},
	}

	s.runNpcScript(sf, n, nil, script.TriggerProc, nil, nil)

	if n.activeScript == nil {
		t.Fatalf("activeScript: got nil, want stored state")
	}
	if n.activeScript.Execution != script.NpcSuspended {
		t.Errorf("Execution: got %v, want NpcSuspended", n.activeScript.Execution)
	}
	if !n.delayed {
		t.Errorf("delayed: got false, want true")
	}
	want := 100 + 1 + 3
	if n.delayedUntil != want {
		t.Errorf("delayedUntil: got %d, want %d", n.delayedUntil, want)
	}
}

func TestNpcTurnResumesSuspendedScriptAfterDelay(t *testing.T) {
	s := newServerForScriptTest(t)
	s.currentTick = 100
	n := newNpcForScriptTest(t)
	n.server = s

	sf := &script.ScriptFile{
		Name:        "npc_delay_3_return",
		Opcodes:     []script.Opcode{script.OpPushConstantInt, script.OpNpcDelay, script.OpReturn},
		IntOperands: []int32{3},
	}

	// Suspend: after this, delayedUntil = 104.
	s.runNpcScript(sf, n, nil, script.TriggerProc, nil, nil)
	if n.activeScript == nil || !n.delayed {
		t.Fatalf("setup: expected suspended state")
	}

	// Advance to delayedUntil and call turn.
	s.currentTick = 104
	n.turn(s)

	if n.activeScript != nil {
		t.Errorf("activeScript: got %v, want nil (resumed and finished)", n.activeScript)
	}
	if n.delayed {
		t.Errorf("delayed: got true, want false (delay expired)")
	}
}

func TestNpcTurnDoesNotResumeWhileDelayed(t *testing.T) {
	s := newServerForScriptTest(t)
	s.currentTick = 100
	n := newNpcForScriptTest(t)
	n.server = s

	sf := &script.ScriptFile{
		Name:        "npc_delay_3_return",
		Opcodes:     []script.Opcode{script.OpPushConstantInt, script.OpNpcDelay, script.OpReturn},
		IntOperands: []int32{3},
	}

	// Suspend: delayedUntil = 104.
	s.runNpcScript(sf, n, nil, script.TriggerProc, nil, nil)

	// Advance to one tick BEFORE delayedUntil.
	s.currentTick = 103
	n.turn(s)

	if n.activeScript == nil {
		t.Errorf("activeScript: got nil, want still-suspended state")
	}
	if n.activeScript != nil && n.activeScript.Execution != script.NpcSuspended {
		t.Errorf("Execution: got %v, want still NpcSuspended", n.activeScript.Execution)
	}
	if !n.delayed {
		t.Errorf("delayed: got false, want true (still within delay window)")
	}
}

func TestNpcEnqueueScriptForTrigger(t *testing.T) {
	n := newNpcForScriptTest(t)

	n.EnqueueScriptForTrigger(script.TriggerAiQueue3, 5, 42)

	if len(n.queue) != 1 {
		t.Fatalf("queue len: got %d, want 1", len(n.queue))
	}
	req := n.queue[0]
	if req.Trigger != script.TriggerAiQueue3 {
		t.Errorf("Trigger: got %v, want TriggerAiQueue3", req.Trigger)
	}
	if req.Delay != 5 {
		t.Errorf("Delay: got %d, want 5", req.Delay)
	}
	if req.LastInt != 42 {
		t.Errorf("LastInt: got %d, want 42", req.LastInt)
	}
}

func TestNpcTurnDeadNpcDoesNotResumeScript(t *testing.T) {
	s := newServerForScriptTest(t)
	s.currentTick = 100
	n := newNpcForScriptTest(t)
	n.server = s

	sf := &script.ScriptFile{
		Name:        "npc_delay_3_return",
		Opcodes:     []script.Opcode{script.OpPushConstantInt, script.OpNpcDelay, script.OpReturn},
		IntOperands: []int32{3},
	}

	// Suspend the script: delayedUntil = 104.
	s.runNpcScript(sf, n, nil, script.TriggerProc, nil, nil)
	if n.activeScript == nil || !n.delayed {
		t.Fatalf("setup: expected suspended state")
	}

	// NPC dies before the delay expires.
	n.dead = true

	// Advance past delayedUntil and call turn.
	s.currentTick = 105
	n.turn(s)

	// Script must NOT have resumed — it stays suspended on the dead
	// NPC per TS Npc.ts:112 isActive guard.
	if n.activeScript == nil {
		t.Errorf("activeScript: got nil, want stored (dead NPC should not resume)")
	}
	if n.activeScript != nil && n.activeScript.Execution != script.NpcSuspended {
		t.Errorf("Execution: got %v, want still NpcSuspended", n.activeScript.Execution)
	}
}

// buildNpcForIntegration builds an NPC wired to a server, with typ
// set so processNpcQueue can read n.typ.Category.
func buildNpcForIntegration(t *testing.T) (*Server, *Npc) {
	t.Helper()
	s := newServerForScriptTest(t)
	s.currentTick = 100
	typ := &objtype.NpcType{
		ConfigType: objtype.ConfigType{ID: 0, DebugName: "test_npc"},
		Category:   -1,
	}
	n := NewNpc(1, 0, 3094, 3106, 0, typ)
	n.server = s
	return s, n
}

// TestNpcTurnFiresQueuedEntryWhenDelayZero — enqueue at delay=1,
// advance one tick. TS Npc.ts:544-549: decrement THEN fire if delay<=0.
// So at delay=1, the same tick that decrements also fires. Queue empty.
func TestNpcTurnFiresQueuedEntryWhenDelayZero(t *testing.T) {
	s, n := buildNpcForIntegration(t)

	n.EnqueueScriptForTrigger(script.TriggerAiQueue1, 1, 0)
	if len(n.queue) != 1 {
		t.Fatalf("setup: queue should have 1 entry, got %d", len(n.queue))
	}

	n.turn(s)

	if len(n.queue) != 0 {
		t.Fatalf("after first turn: queue should be empty (delay went 1→0 and fired), got %d", len(n.queue))
	}
}

// TestNpcTurnDoesNotDecrementQueueWhileDelayed — NPC delayed; queue
// delay must not decrement. Matches TS Npc.ts:544-547.
func TestNpcTurnDoesNotDecrementQueueWhileDelayed(t *testing.T) {
	s, n := buildNpcForIntegration(t)

	n.EnqueueScriptForTrigger(script.TriggerAiQueue1, 3, 0)
	n.delayed = true
	n.delayedUntil = s.currentTick + 100 // far future

	n.turn(s)
	n.turn(s)
	n.turn(s)

	if len(n.queue) != 1 {
		t.Fatalf("queue len: got %d, want 1 (no fire while delayed)", len(n.queue))
	}
	if n.queue[0].Delay != 3 {
		t.Errorf("queue[0].Delay: got %d, want 3 (no decrement while delayed)", n.queue[0].Delay)
	}
}

// TestNpcTurnReentryQueueAppendDuringIteration — strong form: a
// script fired mid-iteration of processNpcQueue can append a new
// entry that is visible to the same iteration. Mirrors TS Npc.ts:538-560
// "speedup quirk" semantics.
//
// Setup: register an "amplifier" script for TriggerAiQueue1 whose
// bytecode (i) calls OpNpcQueue to enqueue a TriggerAiQueue2 entry,
// and (ii) calls OpNpcSetTimer with interval=42 as an observable
// side-effect proving the amplifier actually executed (distinguishes
// from a silent dispatch failure).
//
// Pre-enqueue ONE entry: TriggerAiQueue1 (delay=0). Call turn(). Assert:
//   - len(n.queue) == 0 — proves both the original AND the amplifier-
//     appended TriggerAiQueue2 entry drained in the same pass.
//   - n.timerInterval == 42 — proves the amplifier actually ran.
//
// Failure modes covered:
//
//	A: processNpcQueue switches to snapshot-len iteration → queue len = 1 after turn.
//	B: amplifier silently no-ops (dispatch wired wrong) → timerInterval unchanged.
func TestNpcTurnReentryQueueAppendDuringIteration(t *testing.T) {
	s, n := buildNpcForIntegration(t)

	// buildNpcForIntegration returns a server with scriptProvider == nil
	// (newServerForScriptTest only sets `log`). Seed an empty provider so
	// processNpcQueue can dispatch.
	s.scriptProvider = script.NewProvider()

	// Amplifier: bytecode = OpNpcQueue(TriggerAiQueue2, 0, 0) + OpNpcSetTimer(42) + OpReturn.
	// Pop order for OpNpcQueue: delay (top), arg, queueID (bottom).
	// Bytecode push order: queueID, arg, delay (matching handlers_npc_test.go:734).
	// queueID=2 maps to TriggerAiQueue2 via TriggerAiQueue1 + queueID - 1.
	amplifier := &script.ScriptFile{
		Name:      "nai21_amplifier_aiqueue1",
		LookupKey: uint32(script.TriggerAiQueue1),
		Opcodes: []script.Opcode{
			script.OpPushConstantInt, // push queueID (2 → TriggerAiQueue2)
			script.OpPushConstantInt, // push arg (0)
			script.OpPushConstantInt, // push delay (0)
			script.OpNpcQueue,
			script.OpPushConstantInt, // push interval (42)
			script.OpNpcSetTimer,
			script.OpReturn,
		},
		IntOperands:      []int32{2, 0, 0, 0, 42, 0, 0},
		StringOperands:   []string{"", "", "", "", "", "", ""},
		InstructionCount: 7,
	}
	s.scriptProvider.Register(amplifier)

	// Pre-flight wiring guard: ensure the lookup actually resolves to the amplifier.
	// Without this, a wrong LookupKey computation would silently fall through to
	// nil-script handling and the queue would still drain (Bundle 3 spec § failure
	// modes), masking the wiring bug.
	if got := s.scriptProvider.GetByTrigger(script.TriggerAiQueue1, n.typeId, n.typ.Category); got != amplifier {
		t.Fatalf("setup: GetByTrigger(TriggerAiQueue1, ...) = %v, want amplifier", got)
	}

	// Pre-enqueue ONE entry. Amplifier will append the second mid-iteration.
	n.EnqueueScriptForTrigger(script.TriggerAiQueue1, 0, 0)
	if len(n.queue) != 1 {
		t.Fatalf("setup: queue should have 1 entry, got %d", len(n.queue))
	}

	n.turn(s)

	// Assertion 1: queue fully drained — proves mid-iteration append visible.
	if len(n.queue) != 0 {
		t.Errorf("queue len: got %d, want 0 — amplifier-appended TriggerAiQueue2 "+
			"entry did not drain in same pass (regression to snapshot-len iteration?)",
			len(n.queue))
	}

	// Assertion 2: amplifier side-effect fired — proves amplifier actually ran
	// (not silent dispatch failure).
	if n.timerInterval != 42 {
		t.Errorf("n.timerInterval: got %d, want 42 — amplifier did not execute "+
			"(scriptProvider lookup or runNpcScript may be silently no-op'ing)",
			n.timerInterval)
	}
}

// TestResumeOrFinishNpcDefaultBranchClearsScript — NAI-2 follow-up.
// Synthetic: pre-set Execution to a value that hits the default:
// branch (not Finished/Aborted/NpcSuspended). Execute's hot loop
// exits immediately when Execution != Running, so the pre-set value
// survives untouched.
//
// This path is unreachable from authentic content (all non-
// NpcSuspended non-terminal Execution values require an ActivePlayer,
// and runNpcScript passes nil Self), but the test proves the
// defensive clear fires if future code accidentally drives an NPC-
// anchored script into one of these states.
func TestResumeOrFinishNpcDefaultBranchClearsScript(t *testing.T) {
	s, n := buildNpcForIntegration(t)

	n.activeScript = &script.ScriptState{}

	sf := &script.ScriptFile{
		Name:    "default_branch_script",
		Opcodes: []script.Opcode{script.OpReturn},
	}
	state := script.Init(sf, nil, false, nil, nil)
	state.ActiveNpc = n
	state.Execution = script.CountDialog // synthetic non-Running, non-terminal state

	s.resumeOrFinishNpc(state, n)

	if n.activeScript != nil {
		t.Errorf("activeScript: got %v, want nil (cleared on default branch)", n.activeScript)
	}
}

func TestNpcSetTimer(t *testing.T) {
	n := newNpcForScriptTest(t)

	n.SetTimer(5)
	if n.timerInterval != 5 {
		t.Errorf("timerInterval after SetTimer(5): got %d, want 5", n.timerInterval)
	}

	// -1 is a TS-faithful no-op: must leave timerInterval at 5.
	n.SetTimer(-1)
	if n.timerInterval != 5 {
		t.Errorf("timerInterval after SetTimer(-1): got %d, want 5 (no-op expected)", n.timerInterval)
	}
}

func TestNewNpcSeedsTimerIntervalFromType(t *testing.T) {
	typ := &objtype.NpcType{
		ConfigType: objtype.ConfigType{ID: 0, DebugName: "test_npc"},
		Timer:      7,
	}
	n := NewNpc(1, 0, 3094, 3106, 0, typ)

	if n.timerInterval != 7 {
		t.Errorf("timerInterval from NewNpc: got %d, want 7 (seeded from typ.Timer)", n.timerInterval)
	}
}

// TestNpcTurnDoesNotTickTimerWhileDelayed — timer must not increment
// while the NPC is delayed. TS gates via the isValid early-return in
// turn(); Go gates internally inside processNpcTimer. Matches TS
// Npc.ts:154 (isValid) + :527-536 (processTimers).
func TestNpcTurnDoesNotTickTimerWhileDelayed(t *testing.T) {
	s, n := buildNpcForIntegration(t)
	n.timerInterval = 3
	n.delayed = true
	n.delayedUntil = s.currentTick + 100 // far future

	n.turn(s)
	n.turn(s)
	n.turn(s)

	if n.timerClock != 0 {
		t.Errorf("timerClock after 3 turns while delayed: got %d, want 0 (no tick while delayed)", n.timerClock)
	}
}

// NAI-11 target type-dispatch tests: call buildNpcScriptState directly
// (the pure test seam) and assert that the correct ScriptState field
// and pointer flag are set for each target type.

// TestBuildNpcScriptStateDispatchesActivePlayer — a *Player target
// must land in state.Self with PtrActivePlayer set. Mirrors TS
// ScriptRunner.init: self=Npc, target=Player → _activePlayer=target.
func TestBuildNpcScriptStateDispatchesActivePlayer(t *testing.T) {
	s := newServerForScriptTest(t)
	n := newNpcForScriptTest(t)
	p := &Player{} // *Player satisfies script.ActivePlayer
	sf := &script.ScriptFile{Name: "noop"}

	state := s.buildNpcScriptState(sf, n, p, script.TriggerProc, nil, nil)

	if state.Self == nil {
		t.Error("Self: nil, want set (ActivePlayer target)")
	}
	if state.Pointers&script.PtrActivePlayer == 0 {
		t.Error("Pointers: PtrActivePlayer flag not set")
	}
	// Absence pin (NAI-43, ts_asymmetry_dual_pin):
	// Self2 / PtrActivePlayer2 must NOT be set when self=Npc, target=Player.
	// TS ScriptRunner.init:84-91 sets _activePlayer2 only when self and
	// target are both Player; the self=Npc branch falls into the else arm
	// at L89-90 and assigns _activePlayer (already pinned above).
	if state.Self2 != nil {
		t.Errorf("Self2: got %v, want nil (NPC-self → target.Player goes to Self, not Self2)", state.Self2)
	}
	if state.Pointers&script.PtrActivePlayer2 != 0 {
		t.Error("Pointers: PtrActivePlayer2 set, want unset")
	}
}

// TestBuildNpcScriptStateDispatchesActiveLoc — a *entity.Loc target
// must land in state.ActiveLoc with PtrActiveLoc set.
func TestBuildNpcScriptStateDispatchesActiveLoc(t *testing.T) {
	s := newServerForScriptTest(t)
	n := newNpcForScriptTest(t)
	loc := entitypkg.NewLoc(0, 100, 100, 1, 1, entitypkg.LifecycleRespawn, 42, 10, 0)
	sf := &script.ScriptFile{Name: "noop"}

	state := s.buildNpcScriptState(sf, n, loc, script.TriggerProc, nil, nil)

	if state.ActiveLoc == nil {
		t.Error("ActiveLoc: nil, want set")
	}
	if state.Pointers&script.PtrActiveLoc == 0 {
		t.Error("Pointers: PtrActiveLoc flag not set")
	}
}

// TestBuildNpcScriptStateDispatchesActiveObj — a *entity.Obj target
// must land in state.ActiveObj with PtrActiveObj set.
func TestBuildNpcScriptStateDispatchesActiveObj(t *testing.T) {
	s := newServerForScriptTest(t)
	n := newNpcForScriptTest(t)
	obj := entitypkg.NewObj(0, 100, 100, entitypkg.LifecycleRespawn, 42, 1)
	sf := &script.ScriptFile{Name: "noop"}

	state := s.buildNpcScriptState(sf, n, obj, script.TriggerProc, nil, nil)

	if state.ActiveObj == nil {
		t.Error("ActiveObj: nil, want set")
	}
	if state.Pointers&script.PtrActiveObj == 0 {
		t.Error("Pointers: PtrActiveObj flag not set")
	}
}

// TestBuildNpcScriptStateDispatchesOtherActiveNpc — a *Npc target
// must land in state.OtherActiveNpc with PtrOtherActiveNpc set.
// Mirrors TS ScriptRunner.init: self=Npc, target=Npc → _activeNpc2.
func TestBuildNpcScriptStateDispatchesOtherActiveNpc(t *testing.T) {
	s := newServerForScriptTest(t)
	n := newNpcForScriptTest(t)
	other := newNpcForScriptTest(t)
	sf := &script.ScriptFile{Name: "noop"}

	state := s.buildNpcScriptState(sf, n, other, script.TriggerProc, nil, nil)

	if state.OtherActiveNpc == nil {
		t.Error("OtherActiveNpc: nil, want set")
	}
	if state.Pointers&script.PtrOtherActiveNpc == 0 {
		t.Error("Pointers: PtrOtherActiveNpc flag not set")
	}
	// Absence pin (NAI-43, ts_asymmetry_dual_pin):
	// ActiveNpc (primary slot, set to self=n at npc_script.go:233) must
	// remain bound to self — the target's *Npc must land in OtherActiveNpc,
	// not overwrite the primary slot. TS ScriptRunner.init:92-95 sets
	// _activeNpc2 only when self is also Npc; the primary _activeNpc
	// (already set to self) must be untouched.
	if state.ActiveNpc != n {
		t.Errorf("ActiveNpc: got %v, want self n (target overwrote primary slot)", state.ActiveNpc)
	}
}

// TestNpcRetaliation_EndToEnd_EngineChain exercises the engine path
// from "TriggerAiQueue1 enqueued on NPC" through "NPC's targetOp/target
// set to attack the player" using a synthetic ai_queue1 script that
// mirrors the content npc_default_retaliate logic (finduid +
// npc_setmode(opplayer2)). If this passes, the engine wiring for NPC
// retaliation is complete and any in-game retaliation gap is content-
// side (proc/script not loaded, wrong varn value, etc.).
//
// Bytecode model:
//
//	PUSH_INT_CONST <player.uid>   // operand[0] = uid
//	FIND_UID                      // pops uid, binds s.Self, pushes 1
//	POP_INT_DISCARD               // discard the 1
//	PUSH_INT_CONST 8              // operand[3] = OPPLAYER2
//	NPC_SETMODE                   // pops 8, n.target=player, n.targetOp=8
//	RETURN
func TestNpcRetaliation_EndToEnd_EngineChain(t *testing.T) {
	s := newTestServer(t)
	s.scriptProvider = script.NewProvider()
	s.configsView = serverConfigsView{s: s}

	p, _ := newTestPlayer(t)
	p.client.server = s
	p.x, p.z, p.level = 100, 100, 0
	p.username37 = 42 // deterministic, drives composeUID inside addPlayer
	if err := s.addPlayer(p); err != nil {
		t.Fatalf("addPlayer: %v", err)
	}
	// p.uid is now set by addPlayer via composeUID(username37=42, slot=1).
	// Read it AFTER addPlayer (composeUID overrides any pre-set value).

	npcType := &objtype.NpcType{
		ConfigType: objtype.ConfigType{ID: 7, DebugName: "rat"},
		Category:   0,
	}
	npc := NewNpc(0, 7, 101, 100, 0, npcType)
	npc.server = s
	s.npcs[0] = npc
	s.npcLoop = append(s.npcLoop, npc)

	// Synthetic ai_queue1 script — mirrors npc_default_retaliate.
	s.scriptProvider.Register(&script.ScriptFile{
		Name:      "[ai_queue1,test-retaliate]",
		LookupKey: script.LookupKeyForGlobal(script.TriggerAiQueue1),
		Opcodes: []script.Opcode{
			script.OpPushConstantInt, // push p.uid
			script.OpFindUID,         // pops uid, sets s.Self, pushes 1
			script.OpPopIntDiscard,   // discard FindUID result
			script.OpPushConstantInt, // push OPPLAYER2 = 8
			script.OpNpcSetMode,      // pop 8, set n.target=s.Self, n.targetOp=8
			script.OpReturn,
		},
		IntOperands: []int32{
			int32(p.uid),
			0, // FindUID operand: 0=Self
			0,
			int32(objtype.NPCModeOpPlayer2),
			0, // NpcSetMode operand (unused for Player branch)
			0,
		},
		StringOperands:   []string{"", "", "", "", "", ""},
		InstructionCount: 6,
	})

	// Synthetic ai_opplayer2 script (noop) so fireAiOpTriggerPlayer's
	// GetByTrigger(TriggerAiOpPlayer2, 0, 0) lookup succeeds and doesn't
	// fall into clearInteraction. Without this, aiMode → tryInteract →
	// fireAiOpTriggerPlayer would find nil sf and clear the NPC's target.
	s.scriptProvider.Register(&script.ScriptFile{
		Name:             "[ai_opplayer2,test-attack]",
		LookupKey:        script.LookupKeyForGlobal(script.TriggerAiOpPlayer2),
		Opcodes:          []script.Opcode{script.OpReturn},
		IntOperands:      []int32{0},
		StringOperands:   []string{""},
		InstructionCount: 1,
	})

	// Pre-enqueue the trigger on the NPC. Delay=0 so processNpcQueue
	// fires it this tick.
	npc.queue = []script.NpcQueueRequest{{
		Trigger: script.TriggerAiQueue1,
		Delay:   0,
		LastInt: 0,
	}}

	// Drive the NPC's tick (calls processNpcQueue → fires the
	// retaliation script → npc_setmode binds player as target → aiMode
	// runs → tryInteract operable → fireAiOpTriggerPlayer fires
	// [ai_opplayer2,_] → npc_default_attack would deal damage in
	// production).
	npc.turn(s)

	// Post-tick assertions: retaliation chain must end with the NPC
	// bound to the player via OPPLAYER2 mode.
	if npc.targetOp != objtype.NPCModeOpPlayer2 {
		t.Errorf("npc.targetOp: got %d, want %d (OPPLAYER2) — npc_setmode did not run or s.Self was nil after FindUID",
			npc.targetOp, objtype.NPCModeOpPlayer2)
	}
	if npc.target == nil {
		t.Fatal("npc.target: nil after retaliation — SetInteractionScript failed to bind player")
	}
	if npc.target != p {
		t.Errorf("npc.target: got %v, want player (uid=%d)", npc.target, p.uid)
	}
}

// TestBuildNpcScriptStateSetsPlayerLookup pins the PlayerLookup wiring
// that NPC-anchored scripts depend on when they call `finduid` or
// `p_finduid` to rebind an active player. Without this, the entire
// content retaliation chain breaks:
//   - Player attacks NPC → ~npc_retaliate(0) writes %npc_aggressive_player
//     and queues TriggerAiQueue1 on the NPC.
//   - Next tick's processNpcQueue fires [ai_queue1,_] →
//     gosub(npc_default_retaliate).
//   - npc_default_retaliate calls finduid(%npc_aggressive_player) to
//     bind the attacking player as state.Self, so npc_setmode(opplayer2)
//     can route the NPC's interaction at that player.
//   - handleFindUID returns 0 early when state.PlayerLookup == nil, so
//     the retaliate proc falls into its `if (finduid(...) = false ...)`
//     branch and returns without setting npc mode. NPC never attacks
//     back.
//
// The PARALLEL wiring on the player path is at modules/world/script.go:62
// (`state.PlayerLookup = s` inside buildPlayerScriptState). The NPC path
// needs the same line so finduid resolves from any NPC-anchored script.
func TestBuildNpcScriptStateSetsPlayerLookup(t *testing.T) {
	s := newServerForScriptTest(t)
	n := newNpcForScriptTest(t)
	sf := &script.ScriptFile{Name: "noop"}

	state := s.buildNpcScriptState(sf, n, nil, script.TriggerAiQueue1, nil, nil)

	if state.PlayerLookup == nil {
		t.Error("state.PlayerLookup: nil, want non-nil (NPC scripts calling finduid/p_finduid depend on this — see npc_default_retaliate)")
	}
}

// TestBuildNpcScriptStateNilTargetSetsNoSecondaryPointer — nil target
// must leave all secondary pointer flags clear (only PtrActiveNpc is
// set by the primary NPC wiring). Covers AI_TIMER/DESPAWN/QUEUE* paths.
func TestBuildNpcScriptStateNilTargetSetsNoSecondaryPointer(t *testing.T) {
	s := newServerForScriptTest(t)
	n := newNpcForScriptTest(t)
	sf := &script.ScriptFile{Name: "noop"}

	state := s.buildNpcScriptState(sf, n, nil, script.TriggerProc, nil, nil)

	// ActiveNpc (primary) is set by buildNpcScriptState itself, not target-dispatch.
	if state.Pointers&script.PtrActiveNpc == 0 {
		t.Error("Pointers: PtrActiveNpc not set (primary NPC wiring missing)")
	}
	secondaryMask := script.PtrActivePlayer | script.PtrActiveLoc |
		script.PtrActiveObj | script.PtrOtherActiveNpc
	if state.Pointers&secondaryMask != 0 {
		t.Errorf("secondary Pointers: got %x, want 0", state.Pointers&secondaryMask)
	}
}

// TestNpcStatOutOfRange verifies defensive bounds checking on the
// production *Npc.NpcStat (NAI-17-D2 deviation). Calls with stat id
// < 0 or >= NpcStatCount return 0 instead of panicking on array
// out-of-bounds.
func TestNpcStatOutOfRange(t *testing.T) {
	typ := &objtype.NpcType{Stats: []uint16{7, 11, 13, 17, 19, 23}}
	n := NewNpc(1, 42, 100, 100, 0, typ)

	cases := []struct {
		name string
		id   int
	}{
		{"negative", -1},
		{"at-count", objtype.NpcStatCount},
		{"way-beyond", 100},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := n.NpcStat(tc.id); got != 0 {
				t.Errorf("NpcStat(%d): got %d, want 0 (out of range)", tc.id, got)
			}
		})
	}
}

// TestNpcBaseStatOutOfRange mirrors TestNpcStatOutOfRange for NpcBaseStat.
func TestNpcBaseStatOutOfRange(t *testing.T) {
	typ := &objtype.NpcType{Stats: []uint16{7, 11, 13, 17, 19, 23}}
	n := NewNpc(1, 42, 100, 100, 0, typ)

	cases := []struct {
		name string
		id   int
	}{
		{"negative", -1},
		{"at-count", objtype.NpcStatCount},
		{"way-beyond", 100},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := n.NpcBaseStat(tc.id); got != 0 {
				t.Errorf("NpcBaseStat(%d): got %d, want 0 (out of range)", tc.id, got)
			}
		})
	}
}

// TestNpcRegenIteratesAllSixStats verifies that the regen loop
// converges levels[i] toward baseLevels[i] for ALL 6 stats, not
// just HP. Mirrors TS Npc.ts:515-523.
func TestNpcRegenIteratesAllSixStats(t *testing.T) {
	t.Run("drain-converges-up", func(t *testing.T) {
		s := newServerForScriptTest(t)
		typ := &objtype.NpcType{
			ConfigType: objtype.ConfigType{ID: 0, DebugName: "test_npc"},
			Stats:      []uint16{10, 10, 10, 10, 10, 10},
			RegenRate:  1,
		}
		n := NewNpc(1, 0, 3094, 3106, 0, typ)
		n.server = s
		n.regenInterval = 1
		n.regenClock = 1 // will tick to 2 >= 1 → fires
		// Direct writes to non-HP slots: no production opcode currently
		// mutates levels[0..2,4,5]. This simulates what a future stat-boost
		// or stat-drain opcode would do, so we can assert the regen loop
		// iterates beyond HP per TS Npc.ts:515-523.
		// Seed drains on non-HP slots.
		n.levels[objtype.NpcStatStrength] = 5
		n.levels[objtype.NpcStatMagic] = 8

		s.processNpcRegen(n)

		if got := n.levels[objtype.NpcStatStrength]; got != 6 {
			t.Errorf("levels[STR]: got %d, want 6 (regen increment)", got)
		}
		if got := n.levels[objtype.NpcStatMagic]; got != 9 {
			t.Errorf("levels[MAG]: got %d, want 9 (regen increment)", got)
		}
	})

	t.Run("boost-converges-down", func(t *testing.T) {
		s := newServerForScriptTest(t)
		typ := &objtype.NpcType{
			ConfigType: objtype.ConfigType{ID: 0, DebugName: "test_npc"},
			Stats:      []uint16{10, 10, 10, 10, 10, 10},
			RegenRate:  1,
		}
		n := NewNpc(1, 0, 3094, 3106, 0, typ)
		n.server = s
		n.regenInterval = 1
		n.regenClock = 1
		// Seed boosts on non-HP slots.
		n.levels[objtype.NpcStatRanged] = 12
		n.levels[objtype.NpcStatDefence] = 15

		s.processNpcRegen(n)

		if got := n.levels[objtype.NpcStatRanged]; got != 11 {
			t.Errorf("levels[RNG]: got %d, want 11 (regen decrement)", got)
		}
		if got := n.levels[objtype.NpcStatDefence]; got != 14 {
			t.Errorf("levels[DEF]: got %d, want 14 (regen decrement)", got)
		}
	})
}

func TestNpcTeleport_SetsFieldsAndTeleFlag(t *testing.T) {
	s := newTestServer(t)
	n := &Npc{nid: 0, typeId: 0, x: 5000, z: 5000, level: 0, startX: 5000, startZ: 5000, startLevel: 0}
	if err := s.addNpc(n, -1, true); err != nil {
		t.Fatalf("addNpc: %v", err)
	}
	n.tele = false
	n.Teleport(3200, 3200, 1)
	if n.x != 3200 || n.z != 3200 || n.level != 1 {
		t.Errorf("post-Teleport coords: got (%d, %d, %d), want (3200, 3200, 1)", n.x, n.z, n.level)
	}
	if !n.tele {
		t.Error("post-Teleport tele flag: got false, want true")
	}
}

func TestNpcTeleport_CrossZoneRefreshSubscription(t *testing.T) {
	s := newTestServer(t)
	n := &Npc{nid: 0, typeId: 0, x: 3200, z: 3200, level: 0, startX: 3200, startZ: 3200, startLevel: 0}
	if err := s.addNpc(n, -1, true); err != nil {
		t.Fatalf("addNpc: %v", err)
	}
	prevZone := s.zoneMap.Get(0, 3200, 3200)
	n.Teleport(4000, 4000, 0)
	newZone := s.zoneMap.Get(0, 4000, 4000)
	if prevZone.NpcsCount() != 0 {
		t.Errorf("prev zone NpcsCount after Teleport: got %d, want 0", prevZone.NpcsCount())
	}
	if newZone.NpcsCount() != 1 {
		t.Errorf("new zone NpcsCount after Teleport: got %d, want 1", newZone.NpcsCount())
	}
}

func TestNpcTeleport_SameZoneNoRefresh(t *testing.T) {
	s := newTestServer(t)
	n := &Npc{nid: 0, typeId: 0, x: 3200, z: 3200, level: 0, startX: 3200, startZ: 3200, startLevel: 0}
	if err := s.addNpc(n, -1, true); err != nil {
		t.Fatalf("addNpc: %v", err)
	}
	z := s.zoneMap.Get(0, 3200, 3200)
	prevElement := n.zoneListElement
	n.Teleport(3201, 3201, 0) // same 8x8 zone (400, 400)
	if z.NpcsCount() != 1 {
		t.Errorf("same-zone Teleport NpcsCount: got %d, want 1", z.NpcsCount())
	}
	if n.zoneListElement != prevElement {
		t.Error("same-zone Teleport should preserve zoneListElement (no leave/enter)")
	}
}

func TestNpcTeleport_NilServerNoOp(t *testing.T) {
	n := &Npc{nid: 0, typeId: 0, x: 5000, z: 5000, level: 0}
	// n.server intentionally nil — refreshNpcZone has a nil-guard at zone_refresh.go:34.
	n.Teleport(3200, 3200, 0)
	if n.x != 3200 || n.z != 3200 || n.level != 0 {
		t.Errorf("post-Teleport coords (nil server): got (%d, %d, %d), want (3200, 3200, 0)", n.x, n.z, n.level)
	}
	if !n.tele {
		t.Error("post-Teleport tele flag (nil server): got false, want true")
	}
}

// TestNpcTeleport_FocusFromDirection pins NAI-65 D3-NPC. Teleport from
// (3200, 3200, 0) to (3300, 3300, 0) for a size=1 NPC: dir=NE, moveX=3301,
// moveZ=3301. faceAngleX = Fine(3301, 1) = 3301*2 + 1 = 6603.
// Mirrors TS PathingEntity.ts:286-289 + TS CoordGrid.ts:125-127.
func TestNpcTeleport_FocusFromDirection(t *testing.T) {
	s := newTestServer(t)
	n := &Npc{nid: 0, typeId: 0, x: 3200, z: 3200, level: 0, size: 1, startX: 3200, startZ: 3200, startLevel: 0, faceAngleX: -1, faceAngleZ: -1}
	if err := s.addNpc(n, -1, true); err != nil {
		t.Fatalf("addNpc: %v", err)
	}

	n.Teleport(3300, 3300, 0)

	wantX := 3301*2 + 1
	wantZ := 3301*2 + 1
	if n.faceAngleX != wantX {
		t.Errorf("faceAngleX after Teleport(NE): got %d, want %d (Fine(3301, 1))", n.faceAngleX, wantX)
	}
	if n.faceAngleZ != wantZ {
		t.Errorf("faceAngleZ after Teleport(NE): got %d, want %d (Fine(3301, 1))", n.faceAngleZ, wantZ)
	}
}

// TestNpcTeleport_FocusSize2 pins the size>1 path so a refactor that drops
// `n.size` to a literal `1` regresses. Fine(3301, 2) = 3301*2 + 2 = 6604
// (TS CoordGrid.fine = pos*2 + size).
func TestNpcTeleport_FocusSize2(t *testing.T) {
	s := newTestServer(t)
	n := &Npc{nid: 0, typeId: 0, x: 3200, z: 3200, level: 0, size: 2, startX: 3200, startZ: 3200, startLevel: 0, faceAngleX: -1, faceAngleZ: -1}
	if err := s.addNpc(n, -1, true); err != nil {
		t.Fatalf("addNpc: %v", err)
	}

	n.Teleport(3300, 3300, 0)

	wantX := 3301*2 + 2
	wantZ := 3301*2 + 2
	if n.faceAngleX != wantX {
		t.Errorf("size=2 faceAngleX: got %d, want %d (Fine(3301, 2))", n.faceAngleX, wantX)
	}
	if n.faceAngleZ != wantZ {
		t.Errorf("size=2 faceAngleZ: got %d, want %d (Fine(3301, 2))", n.faceAngleZ, wantZ)
	}
}

// TestNpcTeleport_InPlaceFocusUsesSelfCenter pins the in-place edge case.
// prev == new → Face returns -1 → MoveX/MoveZ no-op → focus uses
// self-center coords. tele still flags true.
func TestNpcTeleport_InPlaceFocusUsesSelfCenter(t *testing.T) {
	s := newTestServer(t)
	n := &Npc{nid: 0, typeId: 0, x: 3200, z: 3200, level: 0, size: 1, startX: 3200, startZ: 3200, startLevel: 0, faceAngleX: -1, faceAngleZ: -1}
	if err := s.addNpc(n, -1, true); err != nil {
		t.Fatalf("addNpc: %v", err)
	}
	n.tele = false

	n.Teleport(3200, 3200, 0)

	wantSelf := 3200*2 + 1
	if n.faceAngleX != wantSelf {
		t.Errorf("in-place faceAngleX: got %d, want %d (Fine(3200, 1) self-center)", n.faceAngleX, wantSelf)
	}
	if n.faceAngleZ != wantSelf {
		t.Errorf("in-place faceAngleZ: got %d, want %d (Fine(3200, 1) self-center)", n.faceAngleZ, wantSelf)
	}
	if !n.tele {
		t.Error("in-place tele flag: got false, want true")
	}
}

// --- NAI-36 Task 7 + NAI-65: Npc.Teleport parity status -----------------
//
// Closed: D1 (level clamp), D2 (unallocated-zone reject) — NAI-36-T7.
// Closed: D3-NPC (focus call) — NAI-65.
// Residual: D4-NPC (no lastStepX/Z fields), D5-NPC (no jump field).
// See DEVIATION block in npc_script.go for full tracker.

// TestNpcTeleport_LevelClampNegative pins D1: level=-1 clamps to 0
// per TS PathingEntity.ts:268-271.
func TestNpcTeleport_LevelClampNegative(t *testing.T) {
	s := newTestServer(t)
	n := &Npc{nid: 0, typeId: 0, x: 5000, z: 5000, level: 0, startX: 5000, startZ: 5000, startLevel: 0}
	if err := s.addNpc(n, -1, true); err != nil {
		t.Fatalf("addNpc: %v", err)
	}

	n.Teleport(3210, 3310, -1)

	if n.level != 0 {
		t.Errorf("level after Teleport(level=-1): got %d, want 0 (clamp)", n.level)
	}
	if n.x != 3210 || n.z != 3310 {
		t.Errorf("x/z after Teleport: got (%d, %d), want (3210, 3310)", n.x, n.z)
	}
}

// TestNpcTeleport_LevelClampHigh pins D1 upper bound: level=4 clamps to 3.
func TestNpcTeleport_LevelClampHigh(t *testing.T) {
	s := newTestServer(t)
	n := &Npc{nid: 0, typeId: 0, x: 5000, z: 5000, level: 0, startX: 5000, startZ: 5000, startLevel: 0}
	if err := s.addNpc(n, -1, true); err != nil {
		t.Fatalf("addNpc: %v", err)
	}

	n.Teleport(3210, 3310, 4)

	if n.level != 3 {
		t.Errorf("level after Teleport(level=4): got %d, want 3 (clamp)", n.level)
	}
}

// TestNpcTeleport_UnallocatedZoneRejects pins D2: silent reject when
// IsZoneAllocated returns false. Per TS PathingEntity.ts:273-278.
func TestNpcTeleport_UnallocatedZoneRejects(t *testing.T) {
	s := newTestServer(t)
	s.gamemap = gamemap.New(discardLogger())
	// Allocate the start zone so refreshNpcZone (which won't run on
	// reject) doesn't matter; allocate-status of the destination is the
	// gate.
	s.gamemap.Pathfinder.Flags.AllocateIfAbsent(3200, 3300, 0)
	n := &Npc{nid: 0, typeId: 0, x: 3200, z: 3300, level: 0, startX: 3200, startZ: 3300, startLevel: 0}
	if err := s.addNpc(n, -1, true); err != nil {
		t.Fatalf("addNpc: %v", err)
	}
	prevX, prevZ, prevLevel := n.x, n.z, n.level
	n.tele = false

	// (3210, 3310) is in an UNallocated zone → reject.
	n.Teleport(3210, 3310, 0)

	if n.x != prevX || n.z != prevZ || n.level != prevLevel {
		t.Errorf("Teleport to unallocated zone: state changed (%d,%d,%d) → (%d,%d,%d), want unchanged",
			prevX, prevZ, prevLevel, n.x, n.z, n.level)
	}
	if n.tele {
		t.Errorf("tele flag: got true, want false (rejected teleport must not set flag)")
	}
}

// TestNpcTeleport_AllocatedZoneAccepts pins D2 positive case as a
// regression-guard against an "always-reject" misimpl.
func TestNpcTeleport_AllocatedZoneAccepts(t *testing.T) {
	s := newTestServer(t)
	s.gamemap = gamemap.New(discardLogger())
	s.gamemap.Pathfinder.Flags.AllocateIfAbsent(3200, 3300, 0)
	s.gamemap.Pathfinder.Flags.AllocateIfAbsent(3210, 3310, 0)
	n := &Npc{nid: 0, typeId: 0, x: 3200, z: 3300, level: 0, startX: 3200, startZ: 3300, startLevel: 0}
	if err := s.addNpc(n, -1, true); err != nil {
		t.Fatalf("addNpc: %v", err)
	}

	n.Teleport(3210, 3310, 0)

	if n.x != 3210 || n.z != 3310 || n.level != 0 {
		t.Errorf("Teleport to allocated zone: got (%d,%d,%d), want (3210,3310,0)",
			n.x, n.z, n.level)
	}
	if !n.tele {
		t.Error("tele flag: got false, want true (accepted teleport must set flag)")
	}
}

// --- NAI-37 Task 3: walktrigger field-write round-trip ---------------------

func TestNpcSetWalkTriggerFieldWrites(t *testing.T) {
	n := NewNpc(0, 0, 3200, 3200, 0, &objtype.NpcType{})
	if got := n.walktrigger; got != -1 {
		t.Errorf("NewNpc walktrigger default: got %d, want -1 (unset sentinel)", got)
	}
	n.SetWalkTrigger(7)
	if got := n.walktrigger; got != 7 {
		t.Errorf("SetWalkTrigger(7): got walktrigger=%d, want 7", got)
	}
	n.SetWalkTriggerArg(42)
	if got := n.walktriggerArg; got != 42 {
		t.Errorf("SetWalkTriggerArg(42): got walktriggerArg=%d, want 42", got)
	}
}

// --- NAI-37 Task 11: npc-path WorldSuspended producer test ----------------

// TestResumeOrFinishNpc_WorldSuspended_EnqueuesAndClearsActiveScript pins
// the npc-path producer: an npc-bound script whose Execute returned
// Execution=WorldSuspended (with the wakeup-tick on the int stack)
// is dispatched by resumeOrFinishNpc to (a) pop the wakeup-tick,
// (b) enqueue to s.worldScriptQueue with that delay, and (c) CLEAR
// the npc's active script pointer. Mirrors TS Npc.ts:219-220
// (WORLD_SUSPENDED arm does NOT assign activeNpc.activeScript).
//
// NAI-156 inverts the NAI-44 T1 retention: symmetric to the player-path
// NAI-155 fix at script.go:148, the WorldSuspended arm now clears
// activeScript for TS-fidelity uniformity. NPCs have no CanAccess
// analog, so the change is non-behavioral but closes the asymmetry.
func TestResumeOrFinishNpc_WorldSuspended_EnqueuesAndClearsActiveScript(t *testing.T) {
	s, n := buildNpcForIntegration(t)

	state := &script.ScriptState{
		Script:      &script.ScriptFile{Name: "test_world_suspend"},
		Execution:   script.WorldSuspended,
		IntStack:    make([]int, script.StackCapacity),
		StringStack: make([]string, script.StackCapacity),
	}
	state.PushInt(7) // wakeup-tick value for resumer to pop

	// Pre-set a non-nil activeScript on the npc so we can verify it gets cleared.
	n.activeScript = state

	s.resumeOrFinishNpc(state, n)

	if got, want := len(s.worldScriptQueue), 1; got != want {
		t.Fatalf("worldScriptQueue length: got %d, want %d", got, want)
	}
	if got := s.worldScriptQueue[0].delay; got != 8 {
		t.Errorf("enqueued delay: got %d, want 8 (popped 7 from script stack, stored as 7+1=8 per TS World.enqueueScript)", got)
	}
	if got := s.worldScriptQueue[0].script; got != state {
		t.Errorf("enqueued script identity: got %p, want %p", got, state)
	}
	// NAI-156: the WorldSuspended arm now calls ClearActiveScript() to
	// mirror TS Npc.ts:219-220 (WORLD_SUSPENDED arm does not assign
	// activeNpc.activeScript). Symmetric to player-path NAI-155 fix at
	// script.go:148. The resume gate (tick.go) is doubly guarded so a
	// nil activeScript here produces no false-resume.
	if got := n.activeScript; got != nil {
		t.Errorf("npc.activeScript: got %p, want nil (WorldSuspended must clear; NAI-156)", got)
	}
}

// TestNpcOnScriptFinishedOrAborted_Match pins the npc-path Finished/Aborted
// tail where state matches activeScript: activeScript is nulled.
// Mirrors TS Npc.ts:226-228. NAI-54 T2.
func TestNpcOnScriptFinishedOrAborted_Match(t *testing.T) {
	n := &Npc{}
	state := &script.ScriptState{Script: &script.ScriptFile{Name: "match"}}
	n.activeScript = state

	n.OnScriptFinishedOrAborted(state)

	if n.activeScript != nil {
		t.Errorf("activeScript: got non-nil, want nil (match must clear)")
	}
}

// TestNpcOnScriptFinishedOrAborted_Mismatch pins the guard: when state
// is NOT n.activeScript, activeScript is preserved. Closes the silent
// NpcSuspended-clobber bug symmetric to the player path. NAI-54 T2.
func TestNpcOnScriptFinishedOrAborted_Mismatch(t *testing.T) {
	n := &Npc{}
	stored := &script.ScriptState{Script: &script.ScriptFile{Name: "stored"}}
	other := &script.ScriptState{Script: &script.ScriptFile{Name: "other"}}
	n.activeScript = stored

	n.OnScriptFinishedOrAborted(other)

	if n.activeScript != stored {
		t.Errorf("activeScript: got %p, want preserved %p", n.activeScript, stored)
	}
}

// TestResumeOrFinishNpcWorldSuspendedClearsActiveScript — NAI-156 inverts
// the NAI-44 T1 retention pin. TS Npc.ts:219-220 WORLD_SUSPENDED arm does
// NOT assign activeNpc.activeScript; symmetric to player-path NAI-155.
func TestResumeOrFinishNpcWorldSuspendedClearsActiveScript(t *testing.T) {
	s, n := buildNpcForIntegration(t)

	state := &script.ScriptState{
		Script:      &script.ScriptFile{Name: "test_world_suspend_nai44"},
		Execution:   script.WorldSuspended,
		IntStack:    make([]int, script.StackCapacity),
		StringStack: make([]string, script.StackCapacity),
	}
	state.PushInt(5) // wakeup-tick value for resumer to pop

	// Pre-set activeScript so the assertion is meaningful.
	n.activeScript = state

	s.resumeOrFinishNpc(state, n)

	if n.activeScript != nil {
		t.Errorf("activeScript: got %p, want nil (WorldSuspended must clear; NAI-156)", n.activeScript)
	}
	if len(s.worldScriptQueue) != 1 {
		t.Errorf("worldScriptQueue length: got %d, want 1", len(s.worldScriptQueue))
	}
}

// TestResumeOrFinishNpc_PreservesUnrelatedSuspendedScript pins the
// NAI-54 NpcSuspended-clobber bug fix end-to-end via resumeOrFinishNpc.
// Symmetric to the player-path test. Mirrors TS Npc.ts:226 guard.
func TestResumeOrFinishNpc_PreservesUnrelatedSuspendedScript(t *testing.T) {
	s, n := buildNpcForIntegration(t)

	// Pre-seed: an unrelated NpcSuspended X stored on the npc.
	stored := &script.ScriptState{
		Script:    &script.ScriptFile{Name: "stored-suspended"},
		Execution: script.NpcSuspended,
	}
	n.activeScript = stored

	// Y: a fresh npc-bound script that returns immediately.
	sf := &script.ScriptFile{
		Name: "[fresh-npc,test]",
		Opcodes: []script.Opcode{
			script.OpReturn,
		},
		IntOperands:      []int32{0},
		StringOperands:   []string{""},
		InstructionCount: 1,
	}
	state := s.buildNpcScriptState(sf, n, nil, script.TriggerProc, nil, nil)

	s.resumeOrFinishNpc(state, n)

	if n.activeScript != stored {
		t.Errorf("activeScript: got %p, want preserved %p (NAI-54 guard: fresh-Y finishing must not null unrelated stored X)",
			n.activeScript, stored)
	}
}

// TestResumeOrFinishNpc_ExecuteError_PreservesUnrelatedSuspendedScript
// pins the NAI-55 NPC-path error+mismatch match-guard: a fresh script Y
// that errors during script.Execute must NOT null an unrelated stored
// activeScript X on the NPC. Mirrors TS Npc.ts:226 guard reached after
// ScriptRunner.execute returned ABORTED (ScriptRunner.ts:228).
func TestResumeOrFinishNpc_ExecuteError_PreservesUnrelatedSuspendedScript(t *testing.T) {
	s, n := buildNpcForIntegration(t)

	stored := &script.ScriptState{
		Script:    &script.ScriptFile{Name: "stored-npc-suspended"},
		Execution: script.NpcSuspended,
	}
	n.activeScript = stored

	sf := &script.ScriptFile{
		Name:    "[err,npc-test]",
		Opcodes: []script.Opcode{script.Opcode(0xFFFF)},
	}
	errState := script.Init(sf, nil, false, nil, nil)
	errState.ActiveNpc = n

	s.resumeOrFinishNpc(errState, n)

	if n.activeScript != stored {
		t.Errorf("activeScript: got %p, want preserved %p (NAI-55 NPC error-path guard)",
			n.activeScript, stored)
	}
}

// TestProcessNpcQueue_SetsStateLastInt pins NAI-123 fix: processNpcQueue
// must copy req.LastInt into state.LastInt before executing the dispatched
// ai_queueN script. Mirrors TS Npc.ts:554-555 — without this line,
// [ai_queueN,_] ~proc(last_int) reads 0 and zero-damages the target.
//
// Observable: register a script at TriggerAiQueue2 whose bytecode pushes
// last_int and feeds it to NPC_SETTIMER. SetTimer writes n.timerInterval
// directly. Enqueue with lastIntArg=42; turn(); assert n.timerInterval == 42.
//
// Failure mode if state.LastInt is unset: OpLastInt pushes 0 → SetTimer(0)
// → n.timerInterval = 0 (got 0, want 42).
func TestProcessNpcQueue_SetsStateLastInt(t *testing.T) {
	s, n := buildNpcForIntegration(t)
	s.scriptProvider = script.NewProvider()

	// Bytecode: OpLastInt; OpNpcSetTimer; OpReturn.
	// OpLastInt pushes state.LastInt; OpNpcSetTimer pops it as the
	// interval and calls n.SetTimer(interval) → n.timerInterval=interval.
	probe := &script.ScriptFile{
		Name:             "nai123_lastint_probe_aiqueue2",
		LookupKey:        uint32(script.TriggerAiQueue2),
		Opcodes:          []script.Opcode{script.OpLastInt, script.OpNpcSetTimer, script.OpReturn},
		IntOperands:      []int32{0, 0, 0},
		StringOperands:   []string{"", "", ""},
		InstructionCount: 3,
	}
	s.scriptProvider.Register(probe)

	if got := s.scriptProvider.GetByTrigger(script.TriggerAiQueue2, n.typeId, n.typ.Category); got != probe {
		t.Fatalf("setup: GetByTrigger(TriggerAiQueue2, ...) = %v, want probe", got)
	}

	n.EnqueueScriptForTrigger(script.TriggerAiQueue2, 1, 42)
	if len(n.queue) != 1 {
		t.Fatalf("setup: queue len = %d, want 1", len(n.queue))
	}

	n.turn(s)

	if len(n.queue) != 0 {
		t.Fatalf("after turn: queue len = %d, want 0 (delay 1→0 fires the queue)", len(n.queue))
	}
	if n.timerInterval != 42 {
		t.Errorf("n.timerInterval: got %d, want 42 (state.LastInt was not propagated to dispatched script — TS Npc.ts:554-555)", n.timerInterval)
	}
}

// TestResumeOrFinishNpc_ExecuteError_ClearsMatchingActiveScript pins
// the NAI-55 NPC-path error+match arm: when the fresh state IS the NPC's
// activeScript and Execute errors, activeScript is nulled. Mirrors TS
// Npc.ts:226-228 tail reached after ScriptRunner.execute returned ABORTED.
func TestResumeOrFinishNpc_ExecuteError_ClearsMatchingActiveScript(t *testing.T) {
	s, n := buildNpcForIntegration(t)

	sf := &script.ScriptFile{
		Name:    "[err,npc-match]",
		Opcodes: []script.Opcode{script.Opcode(0xFFFF)},
	}
	state := script.Init(sf, nil, false, nil, nil)
	state.ActiveNpc = n
	n.activeScript = state // match-arm

	s.resumeOrFinishNpc(state, n)

	if n.activeScript != nil {
		t.Errorf("activeScript: got non-nil, want nil (NPC match-arm must clear on error)")
	}
}
