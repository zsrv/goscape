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
		log:    discardLogger(),
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

	s.runNpcScript(sf, n, nil, nil, nil)

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

	s.runNpcScript(sf, n, nil, nil, nil)

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
	s.runNpcScript(sf, n, nil, nil, nil)
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
	s.runNpcScript(sf, n, nil, nil, nil)

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
	if req.IntArg != 42 {
		t.Errorf("IntArg: got %d, want 42", req.IntArg)
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
	s.runNpcScript(sf, n, nil, nil, nil)
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

// TestResumeOrFinishNpcErrorPathClearsScript — NAI-2 follow-up.
// When script.Execute returns an error, resumeOrFinishNpc must
// clear n.activeScript (matching the player-side resumeOrFinish
// error-path at modules/world/script.go:31-35).
func TestResumeOrFinishNpcErrorPathClearsScript(t *testing.T) {
	s, n := buildNpcForIntegration(t)

	// Pre-store a dummy script to prove it gets cleared.
	n.activeScript = &script.ScriptState{}

	// Build a state whose Execute will error. Opcode 0xFFFF has no
	// registered handler; Execute returns "no handler for ..." error.
	sf := &script.ScriptFile{
		Name:    "err_script",
		Opcodes: []script.Opcode{script.Opcode(0xFFFF)},
	}
	errState := script.Init(sf, nil, false, nil, nil)
	errState.ActiveNpc = n

	s.resumeOrFinishNpc(errState, n)

	if n.activeScript != nil {
		t.Errorf("activeScript: got %v, want nil (cleared on Execute error)", n.activeScript)
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

	state := s.buildNpcScriptState(sf, n, p, nil, nil)

	if state.Self == nil {
		t.Error("Self: nil, want set (ActivePlayer target)")
	}
	if state.Pointers&script.PtrActivePlayer == 0 {
		t.Error("Pointers: PtrActivePlayer flag not set")
	}
}

// TestBuildNpcScriptStateDispatchesActiveLoc — a *entity.Loc target
// must land in state.ActiveLoc with PtrActiveLoc set.
func TestBuildNpcScriptStateDispatchesActiveLoc(t *testing.T) {
	s := newServerForScriptTest(t)
	n := newNpcForScriptTest(t)
	loc := entitypkg.NewLoc(0, 100, 100, 1, 1, entitypkg.LifecycleRespawn, 42, 10, 0)
	sf := &script.ScriptFile{Name: "noop"}

	state := s.buildNpcScriptState(sf, n, loc, nil, nil)

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

	state := s.buildNpcScriptState(sf, n, obj, nil, nil)

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

	state := s.buildNpcScriptState(sf, n, other, nil, nil)

	if state.OtherActiveNpc == nil {
		t.Error("OtherActiveNpc: nil, want set")
	}
	if state.Pointers&script.PtrOtherActiveNpc == 0 {
		t.Error("Pointers: PtrOtherActiveNpc flag not set")
	}
}

// TestBuildNpcScriptStateNilTargetSetsNoSecondaryPointer — nil target
// must leave all secondary pointer flags clear (only PtrActiveNpc is
// set by the primary NPC wiring). Covers AI_TIMER/DESPAWN/QUEUE* paths.
func TestBuildNpcScriptStateNilTargetSetsNoSecondaryPointer(t *testing.T) {
	s := newServerForScriptTest(t)
	n := newNpcForScriptTest(t)
	sf := &script.ScriptFile{Name: "noop"}

	state := s.buildNpcScriptState(sf, n, nil, nil, nil)

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

// --- NAI-36 Task 7: Npc.Teleport partial parity (D1 + D2 only) ----------
//
// NPC closes only D1 (level clamp) and D2 (unallocated-zone reject).
// D3 (focus), D4 (lastStepX/Z), D5-NPC (jump field absent on Npc) remain
// residual per dead_api_polish.md — see DEVIATION block in npc_script.go.

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
